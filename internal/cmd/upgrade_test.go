package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeTarGz builds an in-memory gzip'd tar containing a single regular file
// `name` with the given content.
func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Of(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// upgradeServer stands up an httptest server emulating the GitHub release API +
// the asset downloads (tarball + checksums.txt). tarChecksumOverride, if
// non-empty, is written into checksums.txt instead of the real digest (to test
// the mismatch-abort path). It records whether any download carried an
// Authorization header.
type upgradeServer struct {
	srv          *httptest.Server
	tarName      string
	sawAuth      bool
	tarBytes     []byte
	tarHits      int
	checksumHits int
}

func newUpgradeServer(t *testing.T, tag string, binContent []byte, tarChecksumOverride string) *upgradeServer {
	t.Helper()
	us := &upgradeServer{}
	verNoV := strings.TrimPrefix(tag, "v")
	us.tarName = fmt.Sprintf("civitai_%s_%s_%s.tar.gz", verNoV, runtime.GOOS, runtime.GOARCH)
	us.tarBytes = makeTarGz(t, "civitai", binContent)

	sum := sha256Of(us.tarBytes)
	if tarChecksumOverride != "" {
		sum = tarChecksumOverride
	}
	checksums := fmt.Sprintf("%s  %s\n", sum, us.tarName)

	mux := http.NewServeMux()
	// The release JSON is served at "/" (latestReleaseURL is pointed here).
	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("release lookup sent an Authorization header")
		}
		base := us.srv.URL
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[
			{"name":%q,"browser_download_url":%q},
			{"name":"checksums.txt","browser_download_url":%q}
		]}`, tag, us.tarName, base+"/dl/"+us.tarName, base+"/dl/checksums.txt")
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			us.sawAuth = true
		}
		us.checksumHits++
		_, _ = io.WriteString(w, checksums)
	})
	mux.HandleFunc("/dl/"+us.tarName, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			us.sawAuth = true
		}
		us.tarHits++
		_, _ = w.Write(us.tarBytes)
	})
	// Serve over TLS: production now scheme-pins asset URLs to https, so the
	// asset URLs (and the release JSON they're embedded in) must be https.
	us.srv = httptest.NewTLSServer(mux)
	t.Cleanup(us.srv.Close)
	// The httptest server listens on 127.0.0.1, which is not a real GitHub
	// release host. Inject it into the asset-host allowlist for this test so the
	// production allowlist stays untouched while the integration path still runs.
	if u, err := url.Parse(us.srv.URL); err == nil {
		withAssetHost(t, u.Hostname())
	}
	// Point both the release-lookup and asset-download clients at a transport
	// that trusts the test server's self-signed cert, for this test only.
	withTrustedClients(t, us.srv.Client())
	return us
}

// withTrustedClients swaps the transports of releaseClient + assetDownloadClient
// to trust the given test client's TLS cert, restoring them after the test. The
// asset client keeps its production CheckRedirect (the security control under
// test); only the transport (cert trust) is borrowed.
func withTrustedClients(t *testing.T, trusted *http.Client) {
	t.Helper()
	origRelease := releaseClient
	origAssetTransport := assetDownloadClient.Transport
	releaseClient = trusted
	assetDownloadClient.Transport = trusted.Transport
	t.Cleanup(func() {
		releaseClient = origRelease
		assetDownloadClient.Transport = origAssetTransport
	})
}

// withAssetHost temporarily adds host to assetHostAllowlist for the duration of
// a test (restoring the original state afterward). This is the seam that lets
// the httptest loopback host pass validateAssetURL without weakening the
// production allowlist.
func withAssetHost(t *testing.T, host string) {
	t.Helper()
	if assetHostAllowlist[host] {
		return
	}
	assetHostAllowlist[host] = true
	t.Cleanup(func() { delete(assetHostAllowlist, host) })
}

func (us *upgradeServer) releaseURL() string { return us.srv.URL + "/release" }

// withExecutable pins osExecutable + evalSymlinks to a fixed path for a test.
func withExecutable(t *testing.T, path string) {
	t.Helper()
	oe, es := osExecutable, evalSymlinks
	osExecutable = func() (string, error) { return path, nil }
	evalSymlinks = func(p string) (string, error) { return p, nil }
	t.Cleanup(func() { osExecutable, evalSymlinks = oe, es })
}

// captureApply replaces applyUpdate with a recorder writing the new bytes to the
// target path (emulating the atomic replace without the selfupdate machinery,
// so the test runs as non-root on any FS). Returns the recorded target.
func captureApply(t *testing.T) *string {
	t.Helper()
	orig := applyUpdate
	var target string
	applyUpdate = func(r io.Reader, tp string) error {
		target = tp
		b, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		return os.WriteFile(tp, b, 0o755)
	}
	t.Cleanup(func() { applyUpdate = orig })
	return &target
}

func TestIsHomebrewPath(t *testing.T) {
	brew := []string{
		"/opt/homebrew/Cellar/civitai/0.1.11/bin/civitai",
		"/usr/local/Cellar/civitai/0.1.11/bin/civitai",
		"/home/linuxbrew/.linuxbrew/Cellar/civitai/0.1.11/bin/civitai",
		"/opt/homebrew/bin/civitai",
		"/Users/x/Library/Caskroom/civitai/0.1.11/civitai",
	}
	for _, p := range brew {
		if !isHomebrewPath(p) {
			t.Errorf("expected %q to be detected as a Homebrew path", p)
		}
	}
	notBrew := []string{
		"/usr/local/bin/civitai",
		"/home/zach/go/bin/civitai",
		"/tmp/civitai",
	}
	for _, p := range notBrew {
		if isHomebrewPath(p) {
			t.Errorf("expected %q NOT to be a Homebrew path", p)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	body := "abc123  civitai_0.1.11_linux_amd64.tar.gz\ndef456  other.zip\n"
	got, ok := checksumFor(body, "civitai_0.1.11_linux_amd64.tar.gz")
	if !ok || got != "abc123" {
		t.Errorf("checksumFor = %q,%v want abc123,true", got, ok)
	}
	if _, ok := checksumFor(body, "missing.tar.gz"); ok {
		t.Error("checksumFor should return false for a missing filename")
	}
	// BSD-style leading * marker.
	star := "deadbeef  *civitai_x.tar.gz\n"
	if g, ok := checksumFor(star, "civitai_x.tar.gz"); !ok || g != "deadbeef" {
		t.Errorf("checksumFor should strip a leading *: got %q,%v", g, ok)
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	want := []byte("#!/fake/civitai\x00binary")
	data := makeTarGz(t, "civitai", want)
	got, err := extractBinaryFromTarGz(data, "civitai")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted bytes mismatch")
	}
	if _, err := extractBinaryFromTarGz(data, "nope"); err == nil {
		t.Error("expected error extracting a missing file")
	}
}

func TestUpgrade_AlreadyLatestNoOp(t *testing.T) {
	withParseableVersion(t, "v0.1.11")
	us := newUpgradeServer(t, "v0.1.11", []byte("new"), "")
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)

	var out bytes.Buffer
	if err := runUpgrade(&out, false, false); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("expected up-to-date message: %q", out.String())
	}
	if *target != "" {
		t.Error("up-to-date should NOT call applyUpdate")
	}
	if us.tarHits != 0 {
		t.Errorf("up-to-date should not download the tarball, got %d hits", us.tarHits)
	}
}

func TestUpgrade_ForceProceedsWhenLatest(t *testing.T) {
	withParseableVersion(t, "v0.1.11")
	binContent := []byte("FORCED-NEW-BINARY")
	us := newUpgradeServer(t, "v0.1.11", binContent, "")
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)

	exe := filepath.Join(t.TempDir(), "civitai")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	if err := runUpgrade(&out, true /*force*/, false); err != nil {
		t.Fatalf("upgrade --force: %v", err)
	}
	if *target != exe {
		t.Errorf("force should replace the resolved exe %q, got %q", exe, *target)
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, binContent) {
		t.Errorf("force should have swapped the binary content, got %q", got)
	}
}

func TestUpgrade_BrewDetectionDelegates(t *testing.T) {
	withParseableVersion(t, "v0.1.10") // behind, so it would normally proceed
	us := newUpgradeServer(t, "v0.1.11", []byte("new"), "")
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)
	withExecutable(t, "/opt/homebrew/Cellar/civitai/0.1.10/bin/civitai")

	var out bytes.Buffer
	if err := runUpgrade(&out, false, false); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if !strings.Contains(out.String(), "brew upgrade civitai/tap/civitai") {
		t.Errorf("brew install should print the brew command: %q", out.String())
	}
	if *target != "" {
		t.Error("brew path should NOT self-replace")
	}
	if us.tarHits != 0 || us.checksumHits != 0 {
		t.Errorf("brew path should not download anything, tar=%d sums=%d", us.tarHits, us.checksumHits)
	}
}

func TestUpgrade_BrewForceOverrides(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	binContent := []byte("BREW-FORCE-REPLACED")
	us := newUpgradeServer(t, "v0.1.11", binContent, "")
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)

	// A brew path, but --force should self-replace anyway.
	exe := filepath.Join(t.TempDir(), "Cellar", "civitai")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	if err := runUpgrade(&out, true, false); err != nil {
		t.Fatalf("upgrade --force on brew: %v", err)
	}
	if *target != exe {
		t.Errorf("force should override brew delegation and replace %q, got %q", exe, *target)
	}
}

func TestUpgrade_ChecksumMismatchAborts(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	// Override the checksum to a bogus digest => must abort BEFORE replacing.
	us := newUpgradeServer(t, "v0.1.11", []byte("new"), strings.Repeat("0", 64))
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)

	exe := filepath.Join(t.TempDir(), "civitai")
	original := []byte("ORIGINAL-UNTOUCHED")
	if err := os.WriteFile(exe, original, 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	err := runUpgrade(&out, false, false)
	if err == nil {
		t.Fatal("expected checksum mismatch to abort with an error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error should mention checksum mismatch: %v", err)
	}
	if *target != "" {
		t.Error("checksum mismatch must NOT call applyUpdate")
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, original) {
		t.Errorf("original binary must be untouched on mismatch, got %q", got)
	}
}

func TestUpgrade_HappyPathReplacesBinary(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	newBin := []byte("BRAND-NEW-CIVITAI-BINARY-BYTES")
	us := newUpgradeServer(t, "v0.1.11", newBin, "")
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)

	exe := filepath.Join(t.TempDir(), "civitai")
	if err := os.WriteFile(exe, []byte("OLD-BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	if err := runUpgrade(&out, false, false); err != nil {
		t.Fatalf("upgrade happy path: %v", err)
	}
	if !strings.Contains(out.String(), "Upgraded civitai v0.1.10 → v0.1.11") {
		t.Errorf("expected success message: %q", out.String())
	}
	if *target != exe {
		t.Errorf("applyUpdate target should be the resolved exe %q, got %q", exe, *target)
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, newBin) {
		t.Errorf("binary should be swapped to the new bytes, got %q", got)
	}
	// Verified-then-applied: both assets downloaded exactly once.
	if us.tarHits != 1 || us.checksumHits != 1 {
		t.Errorf("happy path should download tar+checksums once each, got tar=%d sums=%d", us.tarHits, us.checksumHits)
	}
	// SECURITY: no Authorization header on any download.
	if us.sawAuth {
		t.Error("upgrade must NOT send an Authorization header to the download host")
	}
}

func TestUpgrade_PermissionDeniedClearError(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	us := newUpgradeServer(t, "v0.1.11", []byte("new"), "")
	pointAtServer(t, us.releaseURL())

	// Force applyUpdate to return a permission error.
	orig := applyUpdate
	applyUpdate = func(r io.Reader, tp string) error { return os.ErrPermission }
	t.Cleanup(func() { applyUpdate = orig })

	exe := filepath.Join(t.TempDir(), "civitai")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	err := runUpgrade(&out, false, false)
	if err == nil {
		t.Fatal("expected a permission error")
	}
	if !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "sudo") {
		t.Errorf("permission error should suggest sudo: %v", err)
	}
}

func TestValidateAssetURL(t *testing.T) {
	allowed := []string{
		"https://github.com/civitai/cli/releases/download/v0.1.11/civitai_0.1.11_linux_amd64.tar.gz",
		"https://objects.githubusercontent.com/foo/bar",
		"https://release-assets.githubusercontent.com/foo/bar",
		"https://github.com/checksums.txt",
	}
	for _, u := range allowed {
		if err := validateAssetURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", u, err)
		}
	}
	rejected := []string{
		"http://github.com/civitai/cli/releases/download/x.tar.gz", // https downgrade
		"http://objects.githubusercontent.com/foo",                 // http CDN
		"https://evil.example.com/civitai.tar.gz",                  // off-host
		"https://github.com.evil.com/x",                            // host-suffix spoof
		"ftp://github.com/x",                                       // wrong scheme
		"https://raw.githubusercontent.com/x",                      // GitHub but not a release host
		"://nonsense",                                              // unparseable
	}
	for _, u := range rejected {
		if err := validateAssetURL(u); err == nil {
			t.Errorf("expected %q to be REJECTED, but it passed", u)
		}
	}
}

// TestUpgrade_RejectsHTTPAssetURL proves an http:// tarball asset URL aborts the
// upgrade BEFORE any download, leaving the binary untouched. This closes the
// self-referential-checksum foot-gun (an attacker-served http checksums+tarball
// pair).
func TestUpgrade_RejectsHTTPAssetURL(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	// Release JSON advertising an http:// tarball + an http:// checksums.txt.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tarName := fmt.Sprintf("civitai_0.1.11_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(w, `{"tag_name":"v0.1.11","assets":[
			{"name":%q,"browser_download_url":"http://github.com/dl/%s"},
			{"name":"checksums.txt","browser_download_url":"http://github.com/dl/checksums.txt"}]}`,
			tarName, tarName)
	}))
	t.Cleanup(srv.Close)
	pointAtServer(t, srv.URL)
	target := captureApply(t)

	exe := filepath.Join(t.TempDir(), "civitai")
	original := []byte("ORIGINAL-UNTOUCHED")
	if err := os.WriteFile(exe, original, 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	err := runUpgrade(&out, false, false)
	if err == nil {
		t.Fatal("expected an http:// asset URL to abort the upgrade")
	}
	if !strings.Contains(err.Error(), "not https") {
		t.Errorf("error should mention the non-https scheme: %v", err)
	}
	if *target != "" {
		t.Error("an http:// asset URL must NOT call applyUpdate")
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, original) {
		t.Errorf("binary must be untouched when the asset URL is rejected, got %q", got)
	}
}

// TestUpgrade_RejectsNonGitHubAssetHost proves an https asset URL on a host
// outside the allowlist aborts before download.
func TestUpgrade_RejectsNonGitHubAssetHost(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tarName := fmt.Sprintf("civitai_0.1.11_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
		fmt.Fprintf(w, `{"tag_name":"v0.1.11","assets":[
			{"name":%q,"browser_download_url":"https://evil.example.com/%s"},
			{"name":"checksums.txt","browser_download_url":"https://evil.example.com/checksums.txt"}]}`,
			tarName, tarName)
	}))
	t.Cleanup(srv.Close)
	pointAtServer(t, srv.URL)
	target := captureApply(t)
	withExecutable(t, filepath.Join(t.TempDir(), "civitai"))

	var out bytes.Buffer
	err := runUpgrade(&out, false, false)
	if err == nil || !strings.Contains(err.Error(), "not an allowed GitHub release host") {
		t.Errorf("expected an off-host rejection, got: %v", err)
	}
	if *target != "" {
		t.Error("an off-host asset URL must NOT call applyUpdate")
	}
}

// TestDownload_RejectsInsecureRedirect proves the asset download client refuses
// to follow an https->http downgrade redirect.
func TestDownload_RejectsInsecureRedirect(t *testing.T) {
	// Stand up an http target the redirect would point at (must not be reached).
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ATTACKER-PAYLOAD")
	}))
	t.Cleanup(plain.Close)

	// A redirector that 302s to the http:// target. We allowlist its loopback
	// host so the INITIAL URL passes validation and the redirect (not the first
	// hop) is what gets rejected.
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/payload", http.StatusFound)
	}))
	t.Cleanup(redir.Close)
	if u, err := url.Parse(redir.URL); err == nil {
		withAssetHost(t, u.Hostname())
	}

	_, err := download(t.Context(), redir.URL+"/start")
	if err == nil {
		t.Fatal("expected an insecure-redirect rejection")
	}
	if !strings.Contains(err.Error(), "insecure redirect") {
		t.Errorf("error should flag the insecure redirect: %v", err)
	}
}

func TestUpgrade_MissingAssetForPlatform(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	// Serve a release whose only asset is checksums.txt — no platform tarball.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.1.11","assets":[
			{"name":"checksums.txt","browser_download_url":"%s/sums"}]}`, "http://example.invalid")
	}))
	t.Cleanup(srv.Close)
	pointAtServer(t, srv.URL)
	withExecutable(t, filepath.Join(t.TempDir(), "civitai"))

	var out bytes.Buffer
	err := runUpgrade(&out, false, false)
	if err == nil || !strings.Contains(err.Error(), "no release asset") {
		t.Errorf("expected a missing-asset error, got: %v", err)
	}
}
