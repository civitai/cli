package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"gopkg.in/yaml.v3"
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

// makeZip builds an in-memory zip containing a single file `name` with the given
// content — the shape `.goreleaser.yaml` publishes for `goos: windows`.
func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// makeReleaseArchive builds the archive kind named by format.
func makeReleaseArchive(t *testing.T, format, name string, content []byte) []byte {
	t.Helper()
	switch format {
	case "zip":
		return makeZip(t, name, content)
	case "tar.gz":
		return makeTarGz(t, name, content)
	default:
		t.Fatalf("test fixture cannot build a %q archive", format)
		return nil
	}
}

// wantArchiveFormat and wantBinaryName are the TEST's own statement of what the
// release publishes for a GOOS, written out literally here rather than read back
// from `archiveFormatForGOOS` / `releaseBinaryName`.
//
// 🔴 That duplication is deliberate. A fixture built from the production
// functions agrees with them by construction: a hardcoded `.tar.gz` in
// `releaseAssetName` would make the fixture serve a `.tar.gz` too and the
// end-to-end Windows test would pass while #613 was fully reopened. Stating it
// independently is what makes "the server offered a zip and the CLI asked for
// one" an assertion rather than a tautology. `.goreleaser.yaml` remains the
// authority: TestUpgradeArchiveFormatsMatchTheReleaseConfig reads it and fails
// if EITHER of these two copies drifts from it.
func wantArchiveFormat(goos string) string {
	if goos == "windows" {
		return "zip"
	}
	return "tar.gz"
}

func wantBinaryName(goos string) string {
	if goos == "windows" {
		return "civitai.exe"
	}
	return "civitai"
}

// withGOOS pins the runtimeGOOS seam for a test. runtime.GOOS is fixed at
// compile time, so this is the only way the Windows branch runs on CI.
func withGOOS(t *testing.T, goos string) {
	t.Helper()
	orig := runtimeGOOS
	runtimeGOOS = goos
	t.Cleanup(func() { runtimeGOOS = orig })
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

// upgradeFixture describes the release a test server should publish. Every field
// is stated by the test, never read back from the production name/format
// helpers, so a wrong helper cannot make a fixture agree with itself.
type upgradeFixture struct {
	tag              string
	goos, goarch     string
	format           string // "tar.gz" or "zip" — what the RELEASE uploads
	binName          string // the executable's name INSIDE the archive
	binContent       []byte
	checksumOverride string              // non-empty => written into checksums.txt instead of the real digest
	corrupt          func([]byte) []byte // applied to the archive BEFORE the checksum is computed
}

func newUpgradeServer(t *testing.T, tag string, binContent []byte, tarChecksumOverride string) *upgradeServer {
	t.Helper()
	// runtimeGOOS, not runtime.GOOS: a test that pinned the seam gets a server
	// publishing for the platform it pinned.
	return newUpgradeServerFor(t, upgradeFixture{
		tag:              tag,
		goos:             runtimeGOOS,
		goarch:           runtime.GOARCH,
		format:           wantArchiveFormat(runtimeGOOS),
		binName:          wantBinaryName(runtimeGOOS),
		binContent:       binContent,
		checksumOverride: tarChecksumOverride,
	})
}

func newUpgradeServerFor(t *testing.T, fx upgradeFixture) *upgradeServer {
	t.Helper()
	us := &upgradeServer{}
	verNoV := strings.TrimPrefix(fx.tag, "v")
	tag := fx.tag
	us.tarName = fmt.Sprintf("civitai_%s_%s_%s.%s", verNoV, fx.goos, fx.goarch, fx.format)
	us.tarBytes = makeReleaseArchive(t, fx.format, fx.binName, fx.binContent)
	if fx.corrupt != nil {
		us.tarBytes = fx.corrupt(us.tarBytes)
	}

	sum := sha256Of(us.tarBytes)
	if fx.checksumOverride != "" {
		sum = fx.checksumOverride
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

// ---------------------------------------------------------------------------
// #613: the archive-format seam between `.goreleaser.yaml` and upgrade.go.
// ---------------------------------------------------------------------------

// goreleaserDefaultArchiveFormat is what goreleaser publishes for an `archives:`
// entry that declares no `formats:` key. It is `tar.gz` (goreleaser v2's
// documented default), and `archives[civitai]` relies on it for every GOOS that
// has no `format_overrides` entry.
const goreleaserDefaultArchiveFormat = "tar.gz"

// releaseArchivePlan is `.goreleaser.yaml`, read as the question this seam turns
// on: for each GOOS the release BUILDS, which archive format does it PUBLISH?
type releaseArchivePlan struct {
	goos          []string          // every GOOS in `builds:`
	formatFor     map[string]string // goos -> published archive format
	overrideCount int               // how many `format_overrides` entries were read
}

// readReleaseArchivePlan parses `.goreleaser.yaml` into that plan, Fataling on
// anything that would make the comparison vacuous.
func readReleaseArchivePlan(t *testing.T) releaseArchivePlan {
	t.Helper()
	_, raw := readGoreleaserConfig(t)

	var cfg struct {
		Builds []struct {
			ID   string   `yaml:"id"`
			Goos []string `yaml:"goos"`
		} `yaml:"builds"`
		Archives []struct {
			ID              string   `yaml:"id"`
			Formats         []string `yaml:"formats"`
			FormatOverrides []struct {
				Goos    string   `yaml:"goos"`
				Formats []string `yaml:"formats"`
			} `yaml:"format_overrides"`
		} `yaml:"archives"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml builds/archives: %v", err)
	}

	// POSITIVE CONTROL on the builds half. An empty or single-entry goos list
	// would make the per-GOOS loop below run zero or one times and report a
	// confident green about a comparison it never made.
	var goos []string
	for _, b := range cfg.Builds {
		if b.ID == "civitai" {
			goos = b.Goos
		}
	}
	if len(goos) < 3 {
		t.Fatalf("CONTROL failure: .goreleaser.yaml's `civitai` build declares %d goos value(s) (%v), want at least 3 "+
			"(linux, darwin, windows). Either the build id moved or the parse is wrong — every per-GOOS verdict "+
			"below would be about a list this guard never read", len(goos), goos)
	}
	for _, want := range []string{"linux", "darwin", "windows"} {
		if !slices.Contains(goos, want) {
			t.Fatalf("CONTROL failure: .goreleaser.yaml's `civitai` build does not list goos %q (got %v). "+
				"If a platform really was dropped from the release, drop it from upgrade.go too and "+
				"re-derive this control", want, goos)
		}
	}

	// POSITIVE CONTROL on the archives half: the archive `civitai upgrade`
	// downloads is id `civitai` (not `civitai-raw`, the bare binary).
	if len(cfg.Archives) < 2 {
		t.Fatalf("CONTROL failure: parsed %d archive(s) from .goreleaser.yaml, want at least 2 "+
			"(the tar.gz/zip archive and the raw binary) — the `archives:` key moved", len(cfg.Archives))
	}
	plan := releaseArchivePlan{goos: goos, formatFor: map[string]string{}}
	found := false
	for _, a := range cfg.Archives {
		if a.ID != "civitai" {
			continue
		}
		found = true
		def := goreleaserDefaultArchiveFormat
		if len(a.Formats) > 0 {
			def = a.Formats[0]
		}
		for _, g := range goos {
			plan.formatFor[g] = def
		}
		for _, fo := range a.FormatOverrides {
			if len(fo.Formats) == 0 {
				t.Fatalf("CONTROL failure: .goreleaser.yaml has a `format_overrides` entry for goos %q with an "+
					"empty `formats:` list — this guard cannot say what that platform publishes", fo.Goos)
			}
			plan.overrideCount++
			plan.formatFor[fo.Goos] = fo.Formats[0]
		}
	}
	if !found {
		t.Fatal("CONTROL failure: .goreleaser.yaml has no archive with id `civitai` — that is the archive " +
			"`civitai upgrade` downloads, so without it this guard is comparing nothing")
	}
	// POSITIVE CONTROL on the override list. With none, every GOOS resolves to the
	// same default and the comparison below could not tell a per-GOOS function
	// from a hardcoded constant — the exact mutant #613 was.
	if plan.overrideCount == 0 {
		t.Fatalf("CONTROL failure: `archives[civitai]` declares no `format_overrides` at all, so every one of "+
			"%v publishes %q. This guard exists to catch a per-GOOS format the CLI does not honour; with a "+
			"uniform release it cannot distinguish `archiveFormatForGOOS` from `return %q`. If the override "+
			"really was removed, re-derive this guard rather than deleting the control",
			plan.goos, goreleaserDefaultArchiveFormat, goreleaserDefaultArchiveFormat)
	}
	// And the formats really must differ across the platform set, for the same
	// reason: a uniform map is a fixture that cannot see the mutant.
	distinct := map[string]bool{}
	for _, g := range plan.goos {
		distinct[plan.formatFor[g]] = true
	}
	if len(distinct) < 2 {
		t.Fatalf("CONTROL failure: all %d built platforms publish the same archive format (%v) — "+
			"a uniform expectation cannot see a hardcoded extension in upgrade.go", len(plan.goos), sortedKeys(distinct))
	}
	return plan
}

// TestUpgradeArchiveFormatsMatchTheReleaseConfig is the #613 parity guard.
//
// 🔴 IT PINS A RELATIONSHIP, NOT A COMPONENT. `.goreleaser.yaml` was right on
// its own (Windows ships a `.zip`, by `format_overrides`) and upgrade.go was
// self-consistent on its own (build a name, look it up, fail loudly if absent).
// The defect lived in the seam nobody owned: the name carried a hardcoded
// `.tar.gz`, so `civitai upgrade` on Windows could only ever print
// `no release asset "civitai_…_windows_amd64.tar.gz"`. No component test could
// see it, and `runtime.GOOS` being compile-time meant no test ON CI could
// execute the branch at all.
//
// For EVERY GOOS `.goreleaser.yaml` builds, this asserts four things — three
// about the production code, through the real functions rather than the source
// text, and one about the prose that PUBLISHES the same claim to users:
//  1. `archiveFormatForGOOS(goos)` equals the format the release publishes.
//  2. `releaseAssetName(...)` ends in that extension — so an extension inlined
//     somewhere other than archiveFormatForGOOS is still caught.
//  3. `extractBinaryFromArchive` can actually OPEN that format — so an override
//     naming a format the CLI has no reader for fails here rather than on a
//     user's machine after a verified download.
//  4. The three surfaces that STATE the format in prose — the README's
//     `## Upgrading` section, the README's `civitai upgrade` command-reference
//     row, and the command's own `Long` — name the same format this GOOS
//     really publishes. Measured before this leg existed: inverting all three
//     to "a .tar.gz on Windows, a .zip everywhere else" left the whole suite
//     green (21 packages, 0 FAIL), because every guard read code and none read
//     the prose.
//
// 🔴 ATTRIBUTION, AS MEASURED ON THIS TREE — not as assumed. Each mutant below
// reddens exactly the legs named, no more:
//   - upgrade.go reverted to a hardcoded `tar.gz` extension → leg (1) ALONE, on
//     windows. Leg (1)'s `continue` short-circuits (2) and (3) for the very
//     GOOS that failed, so they never run; the prose still matches the release,
//     so (4) is silent. This is a CODE-ONLY defect.
//   - a `format_overrides` entry upgrade.go does not honour (e.g. adding
//     `darwin: [zip]`) → legs (1) AND (4), on darwin: the same `continue` hides
//     (2) and (3), but the RELEASE moved, so all three prose surfaces are now
//     wrong about darwin too. A release change is a code AND a prose defect.
//   - `releaseAssetName` dropping the extension → leg (2), on every GOOS.
//   - the zip case deleted from `extractBinaryFromArchive`'s switch → leg (3),
//     on windows.
//   - any of the three prose surfaces inverted, release and code untouched →
//     leg (4) alone. It runs in its OWN pass, after the loop below, so leg (1)'s
//     `continue` cannot suppress it.
//
// So legs (2), (3) and (4) are all reachable, each by a different mutant — they
// are simply not all the legs a format DISAGREEMENT fires.
func TestUpgradeArchiveFormatsMatchTheReleaseConfig(t *testing.T) {
	plan := readReleaseArchivePlan(t)

	for _, goos := range plan.goos {
		want := plan.formatFor[goos]

		// (1) The format the CLI derives.
		if got := archiveFormatForGOOS(goos); got != want {
			t.Errorf("archiveFormatForGOOS(%q) = %q, but .goreleaser.yaml publishes %q for that platform.\n\n"+
				"That disagreement IS issue #613: the asset lookup asks for a name the release never uploaded, "+
				"so `civitai upgrade` fails on %s with `no release asset …` and replaces nothing. "+
				"Teach archiveFormatForGOOS about the format, and give extractBinaryFromArchive a reader for it.",
				goos, got, want, goos)
			continue
		}

		// (2) The name the CLI actually asks the GitHub API for.
		name := releaseAssetName("9.9.9", goos, "amd64")
		wantName := "civitai_9.9.9_" + goos + "_amd64." + want
		if name != wantName {
			t.Errorf("releaseAssetName(\"9.9.9\", %q, \"amd64\") = %q, want %q.\n\n"+
				"The extension is derived per GOOS precisely so this cannot be hardcoded again (#613); "+
				"a name built anywhere other than releaseAssetName reopens it.", goos, name, wantName)
		}

		// (3) Reachability: the extractor must handle the format end-to-end. A
		// format the CLI can name but not open fails AFTER a verified download,
		// which is the worst place to find out.
		const marker = "CIVITAI-TEST-BINARY-BYTES"
		archive := makeReleaseArchive(t, want, wantBinaryName(goos), []byte(marker))
		got, err := extractBinaryFromArchive(archive, want, wantBinaryName(goos))
		if err != nil {
			t.Errorf("extractBinaryFromArchive cannot read the %q archive .goreleaser.yaml publishes for %s: %v\n\n"+
				"The format is named by archiveFormatForGOOS but has no reader, so an upgrade on %s would "+
				"download and verify the archive and then fail at the last step.", want, goos, err, goos)
			continue
		}
		if string(got) != marker {
			t.Errorf("extractBinaryFromArchive(%s, %q) returned %q, want %q — the reader found the entry but "+
				"returned the wrong bytes", goos, want, got, marker)
		}
	}

	// (4) The PUBLISHED prose. Its own pass, deliberately: leg (1) `continue`s on
	// a format disagreement, and the prose claim is a separate artifact that must
	// be judged even when the code is already wrong.
	assertArchiveFormatProseMatchesPlan(t, plan)

	// NEGATIVE CONTROL on the extractor's dispatch: an unknown format must be
	// refused, not silently treated as one of the two known kinds.
	//
	// 🔴 IT RUNS ONCE PER KNOWN READER, AND THE PAYLOAD IS WELL-FORMED FOR THAT
	// READER ON PURPOSE. A `default:` that falls through to one of the readers is
	// only visible when the payload that reader would ACCEPT is the one being
	// mislabelled — otherwise the fall-through errors for a reason unrelated to
	// the dispatch and the control goes green for the wrong reason. Measured:
	// with a tar.gz payload alone, a `default:` falling through to
	// extractBinaryFromZip SURVIVED (76 pass / 0 fail, identical to baseline) —
	// the zip reader rejected the tarball. One case per reader is what makes this
	// control bidirectional.
	const wellFormed = "WELL-FORMED-BUT-WRONGLY-LABELLED"
	for _, payload := range []struct {
		realFormat string
		data       []byte
	}{
		{archiveFormatTarGz, makeTarGz(t, "civitai", []byte(wellFormed))},
		{archiveFormatZip, makeZip(t, "civitai", []byte(wellFormed))},
	} {
		if got, err := extractBinaryFromArchive(payload.data, "7z", "civitai"); err == nil {
			t.Errorf("extractBinaryFromArchive accepted the unknown format \"7z\" for a well-formed %s payload "+
				"and returned %q — it must refuse a format it has no reader for. A `default:` that falls "+
				"through to extractBinaryFrom%s makes leg (3) above vacuous: every format would "+
				"'have a reader'.", payload.realFormat, got, map[string]string{
				archiveFormatTarGz: "TarGz", archiveFormatZip: "Zip",
			}[payload.realFormat])
		}
	}
}

// ---------------------------------------------------------------------------
// Leg (4): the prose that PUBLISHES the archive-format claim.
// ---------------------------------------------------------------------------

// archiveFormatClaim is what a piece of PROSE claims about archive formats,
// PARSED OUT OF THAT PROSE rather than compared against a sentence written here.
//
// 🔴 THE PARSE IS THE POINT. A guard that hardcoded "the README must say zip on
// Windows" would pass on the day `.goreleaser.yaml` adds an override — which is
// the failure this leg exists for, one level up from #613: the parity guard
// reddens, someone teaches archiveFormatForGOOS the new format, the suite goes
// green, and three published surfaces now state the opposite of what the release
// does. Deriving the expectation from the same plan the code legs use is what
// makes that impossible.
type archiveFormatClaim struct {
	named    map[string]string // lowercased platform word -> format claimed for it
	fallback string            // the format claimed for "everywhere else" / "elsewhere"
}

// formatFor reports the format this prose claims for goos.
func (c archiveFormatClaim) formatFor(goos string) string {
	if f, ok := c.named[goos]; ok {
		return f
	}
	return c.fallback
}

var (
	// "a `.zip` on Windows" / "a .zip on Windows", after the markup strip.
	proseNamedFormatRe = regexp.MustCompile(`\ba \.([A-Za-z0-9.]+) on ([A-Za-z]+)\b`)
	// "a `.tar.gz` everywhere else" / "a .tar.gz elsewhere".
	proseFallbackFormatRe = regexp.MustCompile(`\ba \.([A-Za-z0-9.]+) (?:everywhere else|elsewhere)\b`)
)

// parseArchiveFormatClaim reads one surface's archive-format claim, Fataling
// rather than returning a zero value: an unparsed surface would compare an empty
// map against the plan and report a confident verdict about prose it never read.
//
// A reword that this cannot parse FAILS here, on purpose. That is the cost of a
// machine-readable claim, and the alternative — a looser pattern — is a guard a
// reword walks straight past.
func parseArchiveFormatClaim(t *testing.T, surface, text string) archiveFormatClaim {
	t.Helper()
	flat := flattenWS(strings.NewReplacer("`", "", "*", "").Replace(text))
	claim := archiveFormatClaim{named: map[string]string{}}
	for _, m := range proseNamedFormatRe.FindAllStringSubmatch(flat, -1) {
		claim.named[strings.ToLower(m[2])] = m[1]
	}
	if m := proseFallbackFormatRe.FindStringSubmatch(flat); m != nil {
		claim.fallback = m[1]
	}
	if len(claim.named) == 0 || claim.fallback == "" {
		t.Fatalf("CONTROL failure: could not parse an archive-format claim out of %s "+
			"(named=%v, fallback=%q). That surface is supposed to say which archive format each "+
			"platform gets — e.g. \"a `.zip` on Windows, a `.tar.gz` everywhere else\". Either the "+
			"sentence was reworded past this parser (widen proseNamedFormatRe / "+
			"proseFallbackFormatRe and re-measure) or the claim was dropped, in which case this leg "+
			"has nothing to check.\n\nSurface text:\n%s", surface, claim.named, claim.fallback, flat)
	}
	return claim
}

// readmeUpgradingSection returns the body of the README's `## Upgrading`
// section, Fataling on a miss so the leg cannot pass by reading "".
func readmeUpgradingSection(t *testing.T, readme string) string {
	t.Helper()
	const heading = "\n## Upgrading\n"
	i := strings.Index(readme, heading)
	if i < 0 {
		t.Fatalf("CONTROL failure: README.md has no `## Upgrading` heading — leg (4) would be " +
			"reading an empty string and reporting green about it")
	}
	body := readme[i+len(heading):]
	if j := strings.Index(body, "\n## "); j >= 0 {
		body = body[:j]
	}
	return body
}

// readmeUpgradeCommandRow returns the command-reference table row for
// `civitai upgrade`, Fataling on a miss for the same reason.
func readmeUpgradeCommandRow(t *testing.T, readme string) string {
	t.Helper()
	const prefix = "| `civitai upgrade [--force]`"
	for _, line := range strings.Split(readme, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("CONTROL failure: README.md has no command-reference row starting %q. Either the row "+
		"moved, the flag list changed, or the command was renamed — leg (4) cannot check a row it "+
		"cannot find", prefix)
	return ""
}

// assertArchiveFormatProseMatchesPlan is leg (4): every surface that STATES the
// archive format in prose must name, for every GOOS the release builds, the
// format `.goreleaser.yaml` really publishes for it.
func assertArchiveFormatProseMatchesPlan(t *testing.T, plan releaseArchivePlan) {
	t.Helper()
	readme := readREADME(t)

	for _, s := range []struct{ name, text string }{
		{"README.md's `## Upgrading` section", readmeUpgradingSection(t, readme)},
		{"README.md's `civitai upgrade` command-reference row", readmeUpgradeCommandRow(t, readme)},
		{"`civitai upgrade --help` (upgrade.go's Long)", newUpgradeCmd().Long},
	} {
		claim := parseArchiveFormatClaim(t, s.name, s.text)

		// POSITIVE CONTROL on the platform vocabulary. The prose names platforms
		// in English ("Windows"); the plan names them as GOOS ("windows"). If the
		// two sets do not intersect, every GOOS silently falls through to the
		// fallback and this surface is being judged on half its sentence. A prose
		// "macOS" would land here — map it to `darwin` and re-measure rather than
		// loosening the comparison.
		intersects := false
		for platform := range claim.named {
			if slices.Contains(plan.goos, platform) {
				intersects = true
				break
			}
		}
		if !intersects {
			t.Fatalf("CONTROL failure: %s names platform(s) %v, none of which is a GOOS the release "+
				"builds (%v). Every platform would fall through to the %q fallback, so the explicit "+
				"half of the claim would never be compared with anything",
				s.name, sortedKeys(toSet(claim.named)), plan.goos, claim.fallback)
		}

		for _, goos := range plan.goos {
			want := plan.formatFor[goos]
			if got := claim.formatFor(goos); got != want {
				t.Errorf("%s claims %s downloads a `.%s`, but .goreleaser.yaml publishes `.%s` for that "+
					"platform.\n\nThat is a PUBLISHED factual claim about what `civitai upgrade` does — the "+
					"prose is the only thing a user reads before running it, and nothing in the code legs "+
					"above can see it drift. Fix the sentence (or the release config); do not relax this.",
					s.name, goos, got, want)
			}
		}
	}
}

// toSet turns a map's keys into the set shape sortedKeys wants.
func toSet[V any](m map[string]V) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// TestReadArchiveMemberRefusesAnOversizeMember pins the boundary of the cap on
// what an archive member may be, in BOTH directions.
//
// The hazard it closes: `io.ReadAll(io.LimitReader(rc, limit))` returns exactly
// `limit` bytes with a NIL error when the member is bigger, and those bytes get
// written over the running binary. Silent truncation, no error, working
// executable replaced by a corrupt one.
//
// The bounds deliberately straddle the cap rather than sitting on it — limit-1,
// limit and limit+1 — so a mutant that compares with `>=` instead of `>`, or
// reads `limit` instead of `limit+1`, has a case that separates it.
func TestReadArchiveMemberRefusesAnOversizeMember(t *testing.T) {
	const limit = 1000

	for _, tc := range []struct {
		name      string
		size      int
		wantErr   bool
		wantBytes int
	}{
		{"well under the cap", 1, false, 1},
		{"one byte under the cap", limit - 1, false, limit - 1},
		{"exactly the cap", limit, false, limit},
		{"one byte over the cap", limit + 1, true, 0},
		{"far over the cap", limit * 4, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := bytes.Repeat([]byte{'x'}, tc.size)
			got, err := readArchiveMember(bytes.NewReader(src), "civitai", limit)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("a %d-byte member under a %d-byte cap returned %d bytes and no error — "+
						"that is the silent truncation this guard exists to prevent; those bytes would be "+
						"written over the running binary", tc.size, limit, len(got))
				}
				if !strings.Contains(err.Error(), "larger than") || !strings.Contains(err.Error(), "civitai") {
					t.Errorf("refusal should name the member and say it is too large, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("a %d-byte member under a %d-byte cap must be read in full, got error: %v",
					tc.size, limit, err)
			}
			if len(got) != tc.wantBytes || !bytes.Equal(got, src) {
				t.Errorf("read %d bytes, want %d (and byte-identical)", len(got), tc.wantBytes)
			}
		})
	}

	// A read error is propagated, not swallowed into a short-but-nil-error read.
	if _, err := readArchiveMember(iotest.ErrReader(errors.New("boom")), "civitai", limit); err == nil {
		t.Error("a reader error must propagate — a nil error here would hand a partial binary to selfupdate")
	}
}

// TestExtractBinaryFromZip is the component half of the new code path: a real
// zip, built by archive/zip, containing the binary under the name goreleaser
// gives it on Windows.
func TestExtractBinaryFromZip(t *testing.T) {
	want := []byte("MZ\x00\x00fake-windows-civitai-binary")
	data := makeZip(t, "civitai.exe", want)

	got, err := extractBinaryFromZip(data, "civitai.exe")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}
	// Through the dispatcher too — that is the function production calls.
	got, err = extractBinaryFromArchive(data, "zip", "civitai.exe")
	if err != nil {
		t.Fatalf("extractBinaryFromArchive(zip): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("dispatcher extracted %q, want %q", got, want)
	}
	// A directory prefix must not matter (the goreleaser archive is flat, but
	// matching on the base name is what makes that true rather than lucky).
	nested := makeZip(t, "civitai_0.1.11_windows_amd64/civitai.exe", want)
	if got, err := extractBinaryFromZip(nested, "civitai.exe"); err != nil || !bytes.Equal(got, want) {
		t.Errorf("nested entry: got %q, err %v", got, err)
	}
	// A missing member is an error, not empty bytes.
	if _, err := extractBinaryFromZip(data, "civitai"); err == nil {
		t.Error("expected an error extracting a name the zip does not contain")
	}
}

// TestExtractBinaryFromArchive_CorruptInput proves BOTH readers fail on a
// truncated archive instead of returning a partial binary. The bytes handed back
// on success are what gets written over the user's executable, so "returns
// something" and "returns the whole thing" are not the same claim.
func TestExtractBinaryFromArchive_CorruptInput(t *testing.T) {
	payload := bytes.Repeat([]byte("CIVITAI-BINARY-PAYLOAD-"), 512)

	for _, tc := range []struct {
		format, binName string
	}{
		{"tar.gz", "civitai"},
		{"zip", "civitai.exe"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			full := makeReleaseArchive(t, tc.format, tc.binName, payload)
			// POSITIVE CONTROL: the intact archive must extract, or "the truncated
			// one failed" says nothing about truncation.
			got, err := extractBinaryFromArchive(full, tc.format, tc.binName)
			if err != nil {
				t.Fatalf("CONTROL failure: the INTACT %s archive did not extract: %v", tc.format, err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("CONTROL failure: the intact %s archive extracted %d byte(s), want %d",
					tc.format, len(got), len(payload))
			}

			for _, cut := range []struct {
				name string
				data []byte
			}{
				{"truncated to half", full[:len(full)/2]},
				{"truncated to 10 bytes", full[:10]},
				{"empty", nil},
				{"header bytes replaced with noise", append([]byte("NOT-AN-ARCHIVE-AT-ALL"), full[21:]...)},
			} {
				t.Run(cut.name, func(t *testing.T) {
					got, err := extractBinaryFromArchive(cut.data, tc.format, tc.binName)
					if err == nil {
						t.Fatalf("a %s %s archive extracted cleanly and returned %d byte(s) — a corrupt "+
							"archive must fail, because these bytes are what overwrites the user's binary",
							cut.name, tc.format, len(got))
					}
					if bytes.Equal(got, payload) {
						t.Errorf("a %s %s archive returned the FULL payload alongside its error", cut.name, tc.format)
					}
				})
			}
		})
	}
}

// TestUpgrade_WindowsUsesTheZipAsset is the end-to-end #613 regression test: the
// whole runUpgrade path with GOOS pinned to windows, against a release server
// that publishes ONLY what the real release publishes there — a `.zip` holding
// `civitai.exe`.
//
// Measured RED before the fix: runUpgrade asked for
// `civitai_0.1.11_windows_<arch>.tar.gz`, the server (like the real release)
// offered only the `.zip`, and the run died with `no release asset` having
// replaced nothing.
func TestUpgrade_WindowsUsesTheZipAsset(t *testing.T) {
	withParseableVersion(t, "v0.1.10")
	withGOOS(t, "windows")
	newBin := []byte("MZ\x00\x00BRAND-NEW-WINDOWS-CIVITAI")
	us := newUpgradeServerFor(t, upgradeFixture{
		tag: "v0.1.11", goos: "windows", goarch: runtime.GOARCH,
		format: "zip", binName: "civitai.exe", binContent: newBin,
	})
	pointAtServer(t, us.releaseURL())
	target := captureApply(t)

	exe := filepath.Join(t.TempDir(), "civitai.exe")
	if err := os.WriteFile(exe, []byte("OLD-WINDOWS-BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	withExecutable(t, exe)

	var out bytes.Buffer
	if err := runUpgrade(&out, false, false); err != nil {
		t.Fatalf("upgrade on windows: %v", err)
	}
	// The server only ever answers for the .zip name, so a hit proves the CLI
	// asked for the zip — not merely that it did not crash.
	if !strings.HasSuffix(us.tarName, ".zip") {
		t.Fatalf("CONTROL failure: the fixture published %q, which is not a zip — this test cannot "+
			"say anything about the Windows asset", us.tarName)
	}
	if us.tarHits != 1 || us.checksumHits != 1 {
		t.Errorf("expected exactly one download of %s and of checksums.txt, got asset=%d sums=%d",
			us.tarName, us.tarHits, us.checksumHits)
	}
	if *target != exe {
		t.Errorf("applyUpdate target = %q, want the resolved exe %q", *target, exe)
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, newBin) {
		t.Errorf("the binary was not swapped to the zip's civitai.exe bytes, got %q", got)
	}
	if !strings.Contains(out.String(), "Upgraded civitai v0.1.10 → v0.1.11") {
		t.Errorf("expected the success line, got %q", out.String())
	}
}

// TestUpgrade_ChecksumMismatchAbortsForBothFormats keeps the integrity gate
// pinned on the path the fix added. The zip half is the new claim: a verified
// download is a precondition of extraction for BOTH archive kinds, and nothing
// about adding a second reader may let an unverified archive through.
func TestUpgrade_ChecksumMismatchAbortsForBothFormats(t *testing.T) {
	for _, tc := range []struct{ goos, format, binName, exeName string }{
		{"linux", "tar.gz", "civitai", "civitai"},
		{"windows", "zip", "civitai.exe", "civitai.exe"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			withParseableVersion(t, "v0.1.10")
			withGOOS(t, tc.goos)
			us := newUpgradeServerFor(t, upgradeFixture{
				tag: "v0.1.11", goos: tc.goos, goarch: runtime.GOARCH,
				format: tc.format, binName: tc.binName,
				binContent:       []byte("REPLACEMENT-THAT-MUST-NOT-LAND"),
				checksumOverride: strings.Repeat("0", 64),
			})
			pointAtServer(t, us.releaseURL())
			target := captureApply(t)

			exe := filepath.Join(t.TempDir(), tc.exeName)
			original := []byte("ORIGINAL-UNTOUCHED")
			if err := os.WriteFile(exe, original, 0o755); err != nil {
				t.Fatal(err)
			}
			withExecutable(t, exe)

			var out bytes.Buffer
			err := runUpgrade(&out, false, false)
			if err == nil {
				t.Fatalf("a %s archive with a bogus checksums.txt entry must abort the upgrade", tc.format)
			}
			if !strings.Contains(err.Error(), "checksum mismatch") {
				t.Errorf("error should name the checksum mismatch, got: %v", err)
			}
			// POSITIVE CONTROL: the archive really was fetched, so the refusal is the
			// checksum gate firing and not an earlier asset-lookup failure that would
			// pass this test for the wrong reason.
			if us.tarHits != 1 {
				t.Errorf("expected the %s archive to be downloaded once before the checksum gate, got %d hits",
					tc.format, us.tarHits)
			}
			if *target != "" {
				t.Errorf("checksum mismatch must NOT call applyUpdate, got target %q", *target)
			}
			if got, _ := os.ReadFile(exe); !bytes.Equal(got, original) {
				t.Errorf("the original binary must be untouched on mismatch, got %q", got)
			}
		})
	}
}

// TestUpgrade_CorruptArchiveLeavesTheBinaryAlone drives a corrupt archive whose
// checksums.txt entry MATCHES — so the integrity gate passes and extraction is
// what fails. That is the only way to reach the extractor's error path through
// runUpgrade, and the claim under test is that it still leaves the user with a
// working binary.
func TestUpgrade_CorruptArchiveLeavesTheBinaryAlone(t *testing.T) {
	for _, tc := range []struct{ goos, format, binName string }{
		{"linux", "tar.gz", "civitai"},
		{"windows", "zip", "civitai.exe"},
	} {
		t.Run(tc.goos, func(t *testing.T) {
			withParseableVersion(t, "v0.1.10")
			withGOOS(t, tc.goos)
			us := newUpgradeServerFor(t, upgradeFixture{
				tag: "v0.1.11", goos: tc.goos, goarch: runtime.GOARCH,
				format: tc.format, binName: tc.binName,
				binContent: bytes.Repeat([]byte("NEW-BINARY-"), 256),
				// Truncate, THEN hash: checksums.txt agrees with what is served, so
				// the run gets past the integrity gate and dies in the extractor.
				corrupt: func(b []byte) []byte { return b[:len(b)/3] },
			})
			pointAtServer(t, us.releaseURL())
			target := captureApply(t)

			exe := filepath.Join(t.TempDir(), tc.binName)
			original := []byte("ORIGINAL-UNTOUCHED")
			if err := os.WriteFile(exe, original, 0o755); err != nil {
				t.Fatal(err)
			}
			withExecutable(t, exe)

			var out bytes.Buffer
			err := runUpgrade(&out, false, false)
			if err == nil {
				t.Fatalf("a truncated %s archive must abort the upgrade", tc.format)
			}
			if !strings.Contains(err.Error(), "extract binary") {
				t.Errorf("the failure should come from the extractor (it got past the verified checksum), got: %v", err)
			}
			if *target != "" {
				t.Errorf("a corrupt archive must NOT call applyUpdate, got target %q", *target)
			}
			if got, _ := os.ReadFile(exe); !bytes.Equal(got, original) {
				t.Errorf("the original binary must survive a corrupt archive, got %q", got)
			}
		})
	}
}
