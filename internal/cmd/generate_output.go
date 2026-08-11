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
	"strconv"
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

// defaultOutNameTemplate is the historical scheme `<workflowId>-<n>.<ext>`,
// written as a template on purpose: --out-name added a SECOND way to name an
// output, and a separate hard-coded default path would be a second naming rule
// to keep in step with the containment guard below. There is one renderer.
const defaultOutNameTemplate = "{workflow}-{n}{ext}"

// outNamePlaceholder matches ANY {...} span, including ones this CLI does not
// know. Matching only the three supported names would let `{workflowid}` fall
// through as literal text and land in a filename, which is how a typo becomes a
// mystery file instead of an error.
var outNamePlaceholder = regexp.MustCompile(`\{[^{}]*\}`)

// renderOutName expands an --out-name template for output n. An empty template
// means the default scheme.
//
// 🔴 BOTH INPUTS ARE UNTRUSTED. The workflow id is SERVER-supplied and the
// template is USER-supplied, and the rendered value is the only thing that
// decides where bytes land — a value like "../../.bashrc" must never be able to
// write outside the chosen directory. This function bounds what each
// placeholder can expand to (the id to a bare basename, {ext} to blobExtension's
// bounded value, {n} to a decimal integer); outputTarget is what enforces
// containment on the whole rendered name, and planOutputTarget is what keeps the
// two from ever being called apart.
func renderOutName(tmpl, workflowID string, n int, rawURL string) (string, error) {
	// Only a TRULY empty template means "use the default" — the flag's own zero
	// value. A whitespace-only template is a mistake the user typed, and both
	// available silent answers are wrong: writing a file named "   ", or quietly
	// substituting the default scheme they were trying to replace. It falls
	// through to the empty-render refusal below.
	if tmpl == "" {
		tmpl = defaultOutNameTemplate
	}
	base := filepath.Base(strings.TrimSpace(workflowID))
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) || base == "/" {
		return "", fmt.Errorf("the server returned an unusable workflow id %q — cannot build an output filename", workflowID)
	}

	var unknown string
	rendered := outNamePlaceholder.ReplaceAllStringFunc(tmpl, func(ph string) string {
		switch ph {
		case "{workflow}":
			return base
		case "{n}":
			return strconv.Itoa(n)
		case "{ext}":
			// The BOUNDED extension, never raw URL text: blobExtension replaces
			// anything that is not a dot plus 1-8 alphanumerics with .bin.
			return blobExtension(rawURL)
		}
		if unknown == "" {
			unknown = ph
		}
		return ph
	})
	if unknown != "" {
		return "", fmt.Errorf(
			"unknown placeholder %s in --out-name %q — the supported placeholders are {workflow}, {n} and {ext}; {ext} carries its own leading dot, so the default is %q",
			unknown, tmpl, defaultOutNameTemplate)
	}
	if strings.TrimSpace(rendered) == "" {
		return "", fmt.Errorf(
			"--out-name %q renders an empty file name — the template must produce a name, e.g. 'img-{n}{ext}'", tmpl)
	}
	return rendered, nil
}

// outputTarget resolves one rendered file name against outDir.
//
// 🔴 THE CONTAINMENT GUARD, and it REFUSES rather than sanitising. Silently
// stripping a traversal writes a file the user did not ask for, under a name
// they did not choose, with nothing on screen saying so — the traversal becomes
// invisible instead of impossible.
//
// It is deliberately STRUCTURAL rather than spelled: the last check does not
// hunt for separators in the text, it resolves the join and refuses unless the
// result sits DIRECTLY inside outDir. A spelling check has to enumerate every
// way a separator can be written; comparing the resolved parent cannot be
// spelled around. The two literal checks in front of it exist because
// filepath.Join CLEANS its result, so "." and ".." resolve to real directories
// that can satisfy a parent comparison (outDir "/" plus name "." is the case
// that gets through). One guard, one error — they are alternative ways for the
// same rule to be violated, not three separate rules.
func outputTarget(outDir, name string) (string, error) {
	target := filepath.Join(outDir, name)
	if name == "." || name == ".." || name != filepath.Base(name) || filepath.Dir(target) != filepath.Clean(outDir) {
		return "", fmt.Errorf(
			"the output file name %q would not land directly in the output directory %q — it resolves to %q. An output name must be a plain file name: a path separator or \"..\" is refused, not stripped. Fix the --out-name template, and use --out-dir to choose the directory",
			name, outDir, target)
	}
	return target, nil
}

// planOutputTarget is the ONE place an output's path is decided.
//
// Both callers go through it: validateGenerateOpts renders a PROBE before
// anything is submitted, so a broken template is a usage error instead of a
// post-charge dead end, and downloadOutputs renders the real ones. A second path
// would be a second rule, and the rule here is a security boundary.
func planOutputTarget(tmpl, outDir, workflowID string, n int, rawURL string) (string, error) {
	name, err := renderOutName(tmpl, workflowID, n, rawURL)
	if err != nil {
		return "", err
	}
	return outputTarget(outDir, name)
}

// reportExcludedOutputs prints, per excluded output, WHY it is not being saved.
//
// 🔴 This is not decoration. A `succeeded` workflow can carry outputs that are
// moderation-blocked, never landed, or user-hidden, and the CLI must not filter
// them out silently: the visible symptom of doing so is "the command said it
// worked and I got two files instead of the four I paid for", with nothing on
// screen explaining the gap.
//
// `settled` reports that the caller has ALREADY printed this workflow's own
// transaction record immediately above (see reportWorkflowSettlement). It comes
// in as the return value of that print rather than being re-derived here, so the
// "printed above" pointer cannot claim a block that was never rendered.
//
// 🔴 THE REPETITION RULE, AND THE ATTRIBUTION IT MUST NOT COST
// (civitai/cli#367, review rounds 2 and 3). The server records its failure
// reason on the STEP, so every output of a failed step carries the identical
// string; a `--quantity 10` run that produced nothing printed it once per
// excluded output plus once in the caller's own error — eleven copies of one
// sentence, burying anything that differed.
//
// 🔴 ROUND 2 FIXED THAT BY OVER-SUPPRESSING, AND THE FIX WAS WORSE THAN THE BUG.
// `reportedElsewhere` was fed the WHOLE workflow's reason set, and every output's
// step reasons are by construction a subset of it — so the fallback fired for
// every output on every multi-step failure and the list read
//
//   - out_1: not available (the job finished without producing a usable file)
//   - out_2: not available (the job finished without producing a usable file)
//
// on a workflow whose two steps died of two DIFFERENT causes. Which paid output
// died of what is the one thing this list knows and the workflow-level block
// does not; trading it for one line is not a de-duplication.
//
// So the rule is keyed on whether attribution carries INFORMATION, not on
// whether a string has been seen:
//
//   - Several DISTINCT reasons among the excluded outputs -> every output states
//     its own, always. A reason shared by two differing outputs is therefore
//     printed twice; that is deliberate, because the alternative is a list that
//     cannot be read per-row.
//   - Exactly ONE distinct reason -> attribution is vacuous (they all died of
//     the same thing), so it is printed once: on the first output, or not at all
//     when `reportedElsewhere` says the caller has already rendered it.
//
// `reportedElsewhere` is therefore ONLY for a caller writing to THIS SAME
// STREAM. printWorkflow passes nil deliberately: its block goes to stdout and
// this list to stderr, so `civitai workflows get X >file` would otherwise leave
// the terminal holding nothing but the generic sentence.
//
// The fallback wording is not spelled here: the output is copied with its step
// reasons cleared and handed back to genapi.ExclusionReason, so the two
// renderings cannot drift into two different sentences.
func reportExcludedOutputs(errw io.Writer, excluded []genapi.Output, settled bool, reportedElsewhere []string) {
	if len(excluded) == 0 {
		return
	}
	st := ui.For(errw)
	fmt.Fprintln(errw, st.Warn(fmt.Sprintf(
		"%d output(s) will NOT be saved — they are not deliverable results:", len(excluded))))

	// Does naming a reason per output tell the reader anything they could not
	// get from one line? Only if the outputs disagree about it.
	attributionDiscriminates := distinctReasonSets(excluded) > 1

	seen := make(map[string]bool, len(reportedElsewhere))
	for _, r := range reportedElsewhere {
		seen[r] = true
	}
	for _, o := range excluded {
		if !attributionDiscriminates && allSeen(o.StepErrors, seen) {
			o.StepErrors = nil
		} else {
			for _, r := range o.StepErrors {
				seen[r] = true
			}
		}
		// The reason is SERVER free text and safeTerm keeps newlines, so a
		// multi-line one is indented to stay visibly inside this list item —
		// see indentContinuation. The id is server-origin too.
		fmt.Fprintf(errw, "    - %s: %s\n", safeTerm(dashIfEmpty(o.ID)),
			indentContinuation(safeTerm(genapi.ExclusionReason(o)), "      "))
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
	//
	// 🔴 #346 SPLIT THIS SENTENCE IN TWO, AND THE SPLIT IS A SUBTRACTION. When the
	// caller has just printed the workflow's own debits and credits, telling the
	// user that this CLI cannot see the ledger and pointing them at
	// /user/transactions is false in context — the workflow-level record is on
	// screen. The replacement still answers nothing: it states what this CLI does
	// not do (map a charge onto individual outputs — a claim about this program,
	// which is checkable) and points at the entries. It does NOT say the server
	// has no per-output record; that would be a claim about a payload this build
	// has only seen one shape of. Where no settlement was printed, the original
	// sentence is unchanged.
	fate := "Whether any of that charge comes back depends on why each output was excluded and is decided server-side; " +
		buzzLedgerUnknownNote + "."
	if settled {
		fate = "This CLI does not map a charge onto individual outputs; " + workflowSettlementPrintedNote + "."
	}
	fmt.Fprintln(errw, st.Dim("These were part of the workflow you were charged for. "+fate))
}

// distinctReasonSets counts how many DIFFERENT step-reason sets the excluded
// outputs carry. Outputs with no reason at all are not counted: a
// moderation-blocked output alongside one that never landed does not make the
// server's account ambiguous, because only one of them has an account.
//
// The set, not the individual reason, is the unit — two outputs disagree when
// their lists differ, even if they share an entry.
func distinctReasonSets(excluded []genapi.Output) int {
	seen := make(map[string]bool, len(excluded))
	for _, o := range excluded {
		if len(o.StepErrors) == 0 {
			continue
		}
		// \x00 cannot appear in the key material: safeTerm's caller aside, these
		// strings reach us from JSON, and a NUL would make the joined key
		// ambiguous rather than merely ugly.
		seen[strings.Join(o.StepErrors, "\x00")] = true
	}
	return len(seen)
}

// allSeen reports whether EVERY reason in rs has already been rendered on this
// stream.
//
// 🔴 TWO OF ITS THREE BRANCHES ARE UNOBSERVABLE, AND SAYING SO IS THE POINT —
// an earlier draft of this comment claimed a test pinned the ALL/ANY choice, and
// it does not.
//
//   - ALL vs ANY cannot be observed from any caller. The only call site is
//     guarded by `!attributionDiscriminates`, which means every excluded output
//     carries an IDENTICAL reason set — so the set is either wholly seen or
//     wholly unseen and the two quantifiers agree on every reachable input. ALL
//     is written because it is what the name says; a mutation to ANY is
//     genuinely equivalent, not an uncovered gap. If the gate is ever removed,
//     this becomes live and needs its own test.
//   - The empty-rs branch is documentation. With `rs` empty the caller's
//     else-branch is a no-op loop and `o.StepErrors = nil` a no-op assignment,
//     so both answers produce byte-identical output. It is written out because
//     "an output with no reason is not 'already reported'" is the sentence a
//     reader needs.
func allSeen(rs []string, seen map[string]bool) bool {
	if len(rs) == 0 {
		return false
	}
	for _, r := range rs {
		if !seen[r] {
			return false
		}
	}
	return true
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

// downloadOutputs writes every deliverable output into outDir under the
// outName template (empty = `<workflowId>-<n>.<ext>`) and returns the paths
// written.
//
// Target collisions are checked for EVERY file before the first byte moves, the
// same fail-safe ordering `civitai download` uses: a run that would overwrite an
// existing result must not first spend half the presigned URLs' remaining life
// downloading the ones that happened to come earlier.
//
// 🔴 There are TWO kinds of collision and both are checked in that same
// before-any-bytes window. One is a target that already exists on disk (--force
// overrides it). The other is a template that renders the SAME name for two
// outputs — an --out-name with no {n} — which --force does NOT override, because
// the run would silently overwrite its OWN outputs and the presigned URLs it
// spent getting them are not re-issuable. Discovered halfway through, that is
// unrecoverable; discovered here, it costs nothing.
func downloadOutputs(ctx context.Context, fetch blobFetcher, out, errw io.Writer, workflowID string, outputs []genapi.Output, outDir, outName string, force bool) ([]string, error) {
	type job struct {
		url    string
		target string
	}
	jobs := make([]job, 0, len(outputs))
	byTarget := make(map[string]int, len(outputs))
	for i, o := range outputs {
		if o.URL == nil || strings.TrimSpace(*o.URL) == "" {
			return nil, fmt.Errorf("output %s is marked available but carries no URL — nothing to download", safeTerm(dashIfEmpty(o.ID)))
		}
		target, err := planOutputTarget(outName, outDir, workflowID, i+1, *o.URL)
		if err != nil {
			return nil, err
		}
		if first, dup := byTarget[target]; dup {
			return nil, fmt.Errorf(
				"outputs %d and %d would both be written to %q — this run produced %d outputs and the name is the same for each, so it would overwrite its own results. Include {n} in --out-name (e.g. 'img-{n}{ext}'); nothing was downloaded, and the outputs are still readable with `civitai workflows get %s`",
				first, i+1, target, len(outputs), safeTerm(workflowID))
		}
		byTarget[target] = i + 1
		jobs = append(jobs, job{url: *o.URL, target: target})
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
