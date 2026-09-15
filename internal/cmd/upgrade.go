package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/civitai/cli/internal/ui"
	"github.com/minio/selfupdate"
	"github.com/spf13/cobra"
)

// upgradeTimeout bounds the whole self-update (release lookup + downloads).
// Downloads are a few MB so this is generous.
const upgradeTimeout = 60 * time.Second

// maxDownloadBytes caps each download so a misbehaving/hostile endpoint can't
// make us read forever. The largest current asset is ~5MB; 64MB is ample.
const maxDownloadBytes = 64 << 20

// assetHostAllowlist is the set of hosts GitHub serves release assets (and the
// hosts it redirects browser_download_url through). A release's asset URLs are
// attacker-influenced data (they come straight out of the release JSON), so we
// pin both the scheme (https) AND the host before fetching either the tarball
// or checksums.txt. Without this, a release that advertised an http:// pair
// (checksums.txt + tarball) would make the SHA-256 gate self-referential —
// both halves attacker-controlled — and http.DefaultClient would happily follow
// an https->http downgrade redirect.
//
//   - github.com                        — the canonical browser_download_url host
//   - objects.githubusercontent.com     — the S3-backed CDN github.com 302s to
//   - release-assets.githubusercontent.com — newer release-asset redirect target
//
// It is a var (not a const map) so tests can inject the httptest loopback host
// via withAssetHost without weakening the production default.
var assetHostAllowlist = map[string]bool{
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

// validateAssetURL rejects any asset URL that is not https on an allowlisted
// GitHub host. It returns a non-nil error (aborting the upgrade, before any
// bytes are read) for anything else.
func validateAssetURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid asset URL %q: %w", rawURL, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing asset URL %q: scheme %q is not https", rawURL, u.Scheme)
	}
	if !assetHostAllowlist[u.Hostname()] {
		return fmt.Errorf("refusing asset URL %q: host %q is not an allowed GitHub release host", rawURL, u.Hostname())
	}
	return nil
}

// assetDownloadClient is the HTTP client used for the tarball + checksums.txt
// downloads. Unlike http.DefaultClient it refuses to follow a redirect to a
// non-https URL or to a host outside assetHostAllowlist — closing the
// https->http downgrade (and host-pivot) foot-gun on every hop, not just the
// initial URL. Timeouts/body caps stay on the request context + LimitReader.
var assetDownloadClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if err := validateAssetURL(req.URL.String()); err != nil {
			return fmt.Errorf("insecure redirect: %w", err)
		}
		return nil
	},
}

// releaseClient fetches the release JSON. It is http.DefaultClient in
// production; tests override its transport to trust an httptest TLS server (the
// asset URLs are now scheme-pinned to https, so the test fixtures serve TLS).
var releaseClient = http.DefaultClient

// releaseAsset is a single downloadable asset.
type releaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// fullRelease is the release JSON shape with assets we actually unmarshal.
type fullRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// The two archive formats this repo's release publishes. They are the goreleaser
// `formats:` spellings AND the filename extensions, which is why one constant
// serves both: `archives[civitai]` has no explicit `formats:` (goreleaser's
// default is tar.gz) and one `format_overrides` entry pinning `goos: windows` to
// zip.
const (
	archiveFormatTarGz = "tar.gz"
	archiveFormatZip   = "zip"
)

// archiveFormatForGOOS returns the archive format `.goreleaser.yaml` publishes
// for goos — which is also the extension of the release asset `civitai upgrade`
// must ask for.
//
// 🔴 THIS IS A VENDORED MIRROR OF `.goreleaser.yaml`, AND IT IS THE FIX FOR
// #613. Until this existed, runUpgrade built the asset name with a hardcoded
// `.tar.gz`, so on Windows — where `format_overrides` publishes a `.zip` — the
// lookup could never match and every `civitai upgrade` died with
// `no release asset "civitai_…_windows_amd64.tar.gz"`. Neither file was wrong on
// its own; the defect lived in the seam between them, which is why the parity
// guard (TestUpgradeArchiveFormatsMatchTheReleaseConfig) drives THIS function
// for every GOOS `.goreleaser.yaml` builds rather than grepping either file.
//
// Adding a `format_overrides` entry to `.goreleaser.yaml` without teaching this
// function about it reddens that guard. Do not "simplify" it back to a constant.
func archiveFormatForGOOS(goos string) string {
	if goos == "windows" {
		return archiveFormatZip
	}
	return archiveFormatTarGz
}

// releaseBinaryName returns the name the civitai executable has INSIDE the
// release archive. goreleaser appends `.exe` on Windows, so extracting by the
// bare name would fail on exactly the platform the zip path exists for.
func releaseBinaryName(goos string) string {
	if goos == "windows" {
		return "civitai.exe"
	}
	return "civitai"
}

// releaseAssetName builds the ONE release-asset name runUpgrade asks for. It is
// the single writer of that string: an inlined Sprintf elsewhere would reopen
// #613 on whichever GOOS it got wrong.
func releaseAssetName(version, goos, goarch string) string {
	return fmt.Sprintf("civitai_%s_%s_%s.%s", version, goos, goarch, archiveFormatForGOOS(goos))
}

// Seams for tests.
var (
	// runtimeGOOS is runtime.GOOS behind a seam. runtime.GOOS is fixed at compile
	// time, so without this the Windows zip path could not be exercised at all on
	// the machine CI runs on — which is exactly how #613 shipped unnoticed.
	runtimeGOOS = runtime.GOOS
	// osExecutable resolves the running binary path; overridable in tests.
	osExecutable = os.Executable
	// evalSymlinks resolves symlinks; overridable in tests.
	evalSymlinks = filepath.EvalSymlinks
	// applyUpdate performs the atomic binary replacement. Default uses
	// minio/selfupdate; tests override it to write to a temp path.
	applyUpdate = func(newBinary io.Reader, targetPath string) error {
		return selfupdate.Apply(newBinary, selfupdate.Options{TargetPath: targetPath})
	}
)

// brewPathMarkers are path fragments that indicate a Homebrew-managed install.
// If the resolved executable lives under any of these, we delegate to brew
// instead of self-replacing (unless --force).
var brewPathMarkers = []string{
	"/Cellar/",
	"/Caskroom/",
	"/opt/homebrew",
	"/usr/local/Homebrew",
	"/usr/local/Cellar",
	"/home/linuxbrew/.linuxbrew",
}

func newUpgradeCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Update the civitai CLI to the latest release",
		Long: `Download and install the latest civitai release, replacing this binary.

The latest release is resolved from the public GitHub releases API (no token is
ever sent). The downloaded archive — a .zip on Windows, a .tar.gz everywhere
else — is verified against its SHA-256 checksum before anything is replaced; a
mismatch aborts the upgrade and leaves the current binary untouched.

If this binary was installed via Homebrew, upgrade delegates to:
    brew upgrade civitai/tap/civitai
(use --force to self-replace anyway). The release publishes a Homebrew CASK,
which is macOS-only, so that delegation only leads anywhere on macOS.`,
		Example: `  civitai upgrade
  civitai upgrade --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			noUpdateCheck, _ := cmd.Flags().GetBool("no-update-check")
			return runUpgrade(cmd.OutOrStdout(), force, noUpdateCheck)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"reinstall even if already up to date, and self-replace a Homebrew install")
	return cmd
}

// runUpgrade orchestrates the self-update. out is stdout for user messages.
func runUpgrade(out io.Writer, force, noUpdateCheck bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), upgradeTimeout)
	defer cancel()

	rel, err := fetchRelease(ctx, latestReleaseURL)
	if err != nil {
		return fmt.Errorf("look up latest release: %w", err)
	}
	latest := rel.TagName
	if !isParseableVersion(latest) {
		return fmt.Errorf("latest release tag %q is not a recognizable version", latest)
	}

	// Already up to date? (current >= latest). --force overrides.
	if !force && compareVersions(version, latest) >= 0 && isParseableVersion(version) {
		fmt.Fprintln(out, ui.Success(fmt.Sprintf("civitai is already up to date (%s).", version)))
		return nil
	}

	// Resolve the running executable, following symlinks (Homebrew installs a
	// symlink in the bin dir pointing into the Cellar).
	exe, err := osExecutable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	resolved := exe
	if r, rerr := evalSymlinks(exe); rerr == nil {
		resolved = r
	}

	// Homebrew delegation (unless --force).
	if !force && isHomebrewPath(resolved) {
		fmt.Fprintln(out, ui.Info("civitai was installed via Homebrew. Upgrade with:"))
		fmt.Fprintf(out, "  %s\n", ui.Code("brew upgrade civitai/tap/civitai"))
		return nil
	}

	// Build the asset names for this platform. The EXTENSION is derived per GOOS
	// (#613): Windows is published as a .zip, everything else as a .tar.gz.
	verNoV := strings.TrimPrefix(latest, "v")
	archiveFormat := archiveFormatForGOOS(runtimeGOOS)
	assetName := releaseAssetName(verNoV, runtimeGOOS, runtime.GOARCH)

	assetURL := findAssetURL(rel, assetName)
	if assetURL == "" {
		return fmt.Errorf("no release asset %q for %s/%s — upgrade manually from %s",
			assetName, runtimeGOOS, runtime.GOARCH, "https://github.com/civitai/cli/releases/latest")
	}
	sumsURL := findAssetURL(rel, "checksums.txt")
	if sumsURL == "" {
		return errors.New("release is missing checksums.txt — refusing to upgrade without integrity verification")
	}

	// Defense-in-depth on the transport: the asset URLs come from the release
	// JSON, so pin scheme+host BEFORE fetching anything. An http:// or
	// non-GitHub asset URL aborts the upgrade here — the binary is never touched.
	if err := validateAssetURL(assetURL); err != nil {
		return err
	}
	if err := validateAssetURL(sumsURL); err != nil {
		return err
	}

	// Download the checksums and the archive. checksums.txt covers BOTH archive
	// kinds (goreleaser hashes every uploaded artifact), so the zip path needs no
	// weakening of the integrity gate below.
	sums, err := download(ctx, sumsURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	wantSum, ok := checksumFor(string(sums), assetName)
	if !ok {
		return fmt.Errorf("checksums.txt has no entry for %s — aborting", assetName)
	}

	archiveBytes, err := download(ctx, assetURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetName, err)
	}

	// MANDATORY: verify SHA-256 BEFORE we touch the binary on disk.
	gotSum := sha256Hex(archiveBytes)
	if gotSum != wantSum {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s — aborting upgrade (binary NOT replaced)",
			assetName, gotSum, wantSum)
	}

	// Extract the civitai binary from the verified archive. A corrupt or
	// truncated archive fails HERE, before applyUpdate is reached, so the running
	// binary is left untouched rather than half-written.
	bin, err := extractBinaryFromArchive(archiveBytes, archiveFormat, releaseBinaryName(runtimeGOOS))
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	// Atomically replace the running executable.
	if err := applyUpdate(bytes.NewReader(bin), resolved); err != nil {
		if isPermissionError(err) {
			return fmt.Errorf("cannot replace %s: permission denied.\n"+
				"Try one of:\n"+
				"  sudo civitai upgrade\n"+
				"  reinstall via Homebrew (brew upgrade civitai/tap/civitai)\n"+
				"  or: go install github.com/civitai/cli/cmd/civitai@latest\noriginal error: %w",
				resolved, err)
		}
		return fmt.Errorf("install update: %w", err)
	}

	fmt.Fprintln(out, ui.Success(fmt.Sprintf("Upgraded civitai %s → %s.", version, latest)))
	return nil
}

// isHomebrewPath reports whether path lives under a known Homebrew location.
func isHomebrewPath(path string) bool {
	for _, m := range brewPathMarkers {
		if strings.Contains(path, m) {
			return true
		}
	}
	return false
}

// fetchRelease does the unauthenticated GitHub call and returns the full release
// (tag + assets). No Authorization header is ever attached.
func fetchRelease(ctx context.Context, url string) (fullRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fullRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// NOTE: intentionally NO Authorization header — public, token-free.
	resp, err := releaseClient.Do(req)
	if err != nil {
		return fullRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fullRelease{}, fmt.Errorf("github releases: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fullRelease{}, err
	}
	var rel fullRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return fullRelease{}, err
	}
	return rel, nil
}

// findAssetURL returns the download URL for the named asset, or "".
func findAssetURL(rel fullRelease, name string) string {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.DownloadURL
		}
	}
	return ""
}

// download GETs url and returns the body (capped). It explicitly attaches NO
// Authorization header — release downloads are unauthenticated HTTPS.
func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := assetDownloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
}

// checksumFor parses a `sha256  filename` checksums.txt body and returns the
// hex digest for the given filename.
func checksumFor(sums, filename string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// goreleaser writes "<sha256>  <name>" (filename may carry a leading *).
		name := strings.TrimPrefix(fields[1], "*")
		if name == filename {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// sha256Hex returns the lowercase hex SHA-256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// extractBinaryFromArchive pulls the named file out of a verified release
// archive, dispatching on the archive FORMAT rather than on runtime.GOOS.
//
// The format is an explicit argument so both branches are reachable from a test
// on any host: runtime.GOOS is compile-time, and a zip path only Windows could
// ever execute is a path nobody would ever have seen run (#613).
func extractBinaryFromArchive(data []byte, format, name string) ([]byte, error) {
	switch format {
	case archiveFormatZip:
		return extractBinaryFromZip(data, name)
	case archiveFormatTarGz:
		return extractBinaryFromTarGz(data, name)
	default:
		return nil, fmt.Errorf("unsupported release archive format %q — upgrade manually from %s",
			format, "https://github.com/civitai/cli/releases/latest")
	}
}

// readArchiveMember reads one archive member, REFUSING anything larger than
// limit instead of silently truncating it.
//
// 🔴 io.LimitReader ALONE CANNOT TELL A FULL READ FROM A TRUNCATED ONE: it
// returns exactly `limit` bytes with a nil error when the member is bigger, and
// what this function returns is written over the RUNNING BINARY. A truncated
// write would leave a corrupt executable where a working one used to be, with no
// error anywhere. Reading limit+1 is what makes the overflow observable.
//
// limit is a parameter rather than maxDownloadBytes inline so the refusal can be
// exercised without building a 64 MiB fixture.
func readArchiveMember(r io.Reader, name string, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%q in the release archive is larger than the %d-byte limit this "+
			"upgrader reads — upgrade manually from %s",
			name, limit, "https://github.com/civitai/cli/releases/latest")
	}
	return b, nil
}

// extractBinaryFromZip pulls the named file out of a zip archive — the format
// `.goreleaser.yaml` publishes for `goos: windows`.
func extractBinaryFromZip(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Zip entry names are always slash-separated (APPNOTE 4.4.17), so
		// path.Base — not filepath.Base, which would not split on a non-Windows
		// host. Matching the base name means a directory prefix does not matter.
		if path.Base(f.Name) != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := readArchiveMember(rc, name, maxDownloadBytes)
		// 🔴 THE CRC IS CHECKED BY THE READ ABOVE, NOT BY Close. Measured against
		// Go 1.25's archive/zip (three corruption shapes, including through the
		// io.LimitReader this uses): a member whose deflate stream is corrupt
		// returns `zip: checksum error` from io.ReadAll, and rc.Close() then
		// returns nil. So the `err` check below is what catches a corrupt archive;
		// Close has nothing left to report and its error is deliberately dropped.
		// A previous version of this code returned Close's error with a comment
		// claiming it was the CRC site — that branch was dead and the comment was
		// wrong, which invited deleting the check that actually works.
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

// extractBinaryFromTarGz pulls the named file out of a gzip'd tar archive. The
// goreleaser archive contains the binary at the top level (plus README/LICENSE).
func extractBinaryFromTarGz(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Match by base name so a possible directory prefix doesn't matter.
		if filepath.Base(hdr.Name) == name {
			return readArchiveMember(tr, name, maxDownloadBytes)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

// isPermissionError reports whether err is (or wraps) a permission-denied error.
func isPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission)
}
