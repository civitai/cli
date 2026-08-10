package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
)

// blobFetcher issues a GET for one presigned output URL and returns the open
// response for streaming; the caller closes the body.
//
// 🔴 It is wired to civitai.Client.DownloadPresigned, NOT DownloadFile. Blob
// URLs are presigned — they carry their own authorization and need no bearer
// token — and they are served from a *.civitai.com host, which
// isTrustedDownloadHost treats as trusted. So DownloadFile would attach the
// user's FULL-SCOPE personal API key to a request that does not need it. See
// the PresignedDownloader doc in pkg/civitai for why this is a separate seam
// rather than a weakened predicate.
type blobFetcher func(ctx context.Context, fileURL string) (*http.Response, error)

// safeExtension bounds an extension taken from a SERVER-supplied URL: a leading
// dot and 1-8 alphanumerics. Anything else is replaced with .bin rather than
// pasted into a filename.
var safeExtension = regexp.MustCompile(`^\.[A-Za-z0-9]{1,8}$`)

// blobExtension derives an output file's extension from its URL path.
//
// It is deliberately derived from the URL and not from the response's
// Content-Type: the target path has to be known BEFORE the request, so that a
// collision with an existing file is refused without any bytes moving (and
// without burning a presigned URL that is already counting down to expiry).
func blobExtension(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ".bin"
	}
	ext := path.Ext(u.Path)
	if !safeExtension.MatchString(ext) {
		return ".bin"
	}
	return strings.ToLower(ext)
}

// blobFileName builds `<workflowId>-<n>.<ext>`.
//
// The workflow id is SERVER-supplied, so it is reduced to a bare basename and
// rejected if it degenerates — the same rule `civitai download` applies to
// server-supplied file names, for the same reason: a value like "../../.bashrc"
// must never be able to write outside the chosen directory.
func blobFileName(workflowID string, n int, rawURL string) (string, error) {
	base := filepath.Base(strings.TrimSpace(workflowID))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) || base == "/" {
		return "", fmt.Errorf("the server returned an unusable workflow id %q — cannot build an output filename", workflowID)
	}
	return fmt.Sprintf("%s-%d%s", base, n, blobExtension(rawURL)), nil
}

// reportExcludedOutputs prints, per excluded output, WHY it is not being saved.
//
// 🔴 This is not decoration. A `succeeded` workflow can carry outputs that are
// moderation-blocked, never landed, or user-hidden, and the CLI must not filter
// them out silently: the visible symptom of doing so is "the command said it
// worked and I got two files instead of the four I paid for", with nothing on
// screen explaining the gap.
func reportExcludedOutputs(errw io.Writer, excluded []genapi.Output) {
	if len(excluded) == 0 {
		return
	}
	st := ui.For(errw)
	fmt.Fprintln(errw, st.Warn(fmt.Sprintf(
		"%d output(s) will NOT be saved — they are not deliverable results:", len(excluded))))
	for _, o := range excluded {
		fmt.Fprintf(errw, "    - %s: %s\n", safeTerm(dashIfEmpty(o.ID)), safeTerm(genapi.ExclusionReason(o)))
	}
	// 🔴 The second half of this line used to read "a blocked or missing output
	// is not refunded". That is true of a HARD block and false of the commonest
	// soft one: a mature-content output is `canUpgrade`, where the server's own
	// wording is "your Buzz is held, not lost" and the non-yellow debits are
	// credited back when the user unlocks it
	// (`civitai/civitai → src/server/services/orchestrator/poll-iteration.ts`,
	// `src/components/ImageGeneration/QueueItem.tsx`). The workflow HAS been
	// charged — that half is not in doubt, and stays — but the CLI cannot see
	// the Buzz ledger and cannot tell the two exclusion kinds apart from the
	// payload, so it must not settle the refund question in either direction.
	// See AGENTS.md item 28 and #278.
	fmt.Fprintln(errw, st.Dim(
		"These were part of the workflow you were charged for. Whether any of that charge comes back depends on why each output was excluded and is decided server-side; "+
			buzzLedgerUnknownNote+"."))
}

// reportOutputCountMismatch warns when fewer deliverable outputs came back than
// were asked for. requested <= 0 means the user did not pass --quantity, so
// there is no number to compare against and nothing is claimed.
func reportOutputCountMismatch(errw io.Writer, requested, delivered int) {
	if requested <= 0 || delivered == requested {
		return
	}
	fmt.Fprintln(errw, ui.For(errw).Warn(fmt.Sprintf(
		"you asked for %d output(s) and %d are deliverable — the difference was charged for either way",
		requested, delivered)))
}

// blobStatusError maps a non-2xx blob fetch to an actionable error.
//
// It is separate from downloadStatusError (the model-file one) because the
// remedies are different: a blob URL is PRESIGNED and short-lived, so a 401/403
// here means the signature expired or was never valid — `civitai login` is not
// the answer, re-reading the workflow for a fresh URL is.
func blobStatusError(status int, name string) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("the download link for %s was refused (HTTP %d) — output URLs are presigned and expire, so this usually means the link went stale; re-read the workflow with `civitai workflows get <workflow-id>` to get fresh links", name, status)
	case status == http.StatusNotFound:
		return fmt.Errorf("the download link for %s returned 404 — the output may have expired or been removed", name)
	default:
		return fmt.Errorf("download of %s failed (HTTP %d)", name, status)
	}
}

// downloadBlobTo streams one presigned URL to target.
//
// It deliberately reuses the existing machinery rather than reimplementing it:
// the transport (SSRF dial guard, https-per-redirect-hop, hop cap) comes from
// pkg/civitai via the fetcher, and the on-disk half — stale-.part self-heal,
// progress meter, cleanup-on-failure, atomic rename — is writePart, the same
// function `civitai download` uses. A fresh downloader would have discarded all
// of it.
func downloadBlobTo(ctx context.Context, fetch blobFetcher, out, errw io.Writer, fileURL, target string, force bool) error {
	name := filepath.Base(target)
	if !force {
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			return fmt.Errorf("refusing to overwrite the existing file %s — pass --force to replace it, or --out-dir <dir> to write somewhere else", target)
		}
	}
	if dir := filepath.Dir(target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory %s: %w", dir, err)
		}
	}

	resp, err := fetch(ctx, fileURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	defer resp.Body.Close()
	if err := blobStatusError(resp.StatusCode, name); err != nil {
		return err
	}

	partPath := target + ".part"
	written, _, err := writePart(resp.Body, partPath, errw, name, resp.ContentLength, false)
	if err != nil {
		return err
	}
	if err := os.Rename(partPath, target); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("install %s: %w", target, err)
	}
	fmt.Fprintf(out, "Saved %s (%s)\n", safeTerm(target), humanBytes(written))
	return nil
}

// downloadOutputs writes every deliverable output into outDir as
// `<workflowId>-<n>.<ext>` and returns the paths written.
//
// Target collisions are checked for EVERY file before the first byte moves, the
// same fail-safe ordering `civitai download` uses: a run that would overwrite an
// existing result must not first spend half the presigned URLs' remaining life
// downloading the ones that happened to come earlier.
func downloadOutputs(ctx context.Context, fetch blobFetcher, out, errw io.Writer, workflowID string, outputs []genapi.Output, outDir string, force bool) ([]string, error) {
	type job struct {
		url    string
		target string
	}
	jobs := make([]job, 0, len(outputs))
	for i, o := range outputs {
		if o.URL == nil || strings.TrimSpace(*o.URL) == "" {
			return nil, fmt.Errorf("output %s is marked available but carries no URL — nothing to download", safeTerm(dashIfEmpty(o.ID)))
		}
		name, err := blobFileName(workflowID, i+1, *o.URL)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job{url: *o.URL, target: filepath.Join(outDir, name)})
	}
	if !force {
		var clashes []string
		for _, j := range jobs {
			if info, err := os.Stat(j.target); err == nil && !info.IsDir() {
				clashes = append(clashes, j.target)
			}
		}
		if len(clashes) > 0 {
			return nil, fmt.Errorf("refusing to overwrite %d existing file(s):\n  %s\nPass --force to replace them, or --out-dir <dir> to write somewhere else",
				len(clashes), strings.Join(clashes, "\n  "))
		}
	}

	written := make([]string, 0, len(jobs))
	for _, j := range jobs {
		if err := downloadBlobTo(ctx, fetch, out, errw, j.url, j.target, force); err != nil {
			return written, err
		}
		written = append(written, j.target)
	}
	return written, nil
}
