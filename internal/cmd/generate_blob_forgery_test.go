package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// civitai/cli#574 — generate's BLOB DOWNLOAD PATH printed the server-derived
// name raw in six error strings, while its own `Saved` line was gated.
//
// `downloadBlobTo`'s `name` is `filepath.Base(target)`, and `target` comes from
// `planOutputTarget(o.outName, …)` whose `{workflow}` is read off the wire —
// `renderOutName`'s own doc comment says so. So every one of those strings is a
// terminal surface carrying uploader-influenced text, on the ONE path in this
// CLI that spends the user's money.
//
// 🔴 THE INCONSISTENCY IS WHAT MAKES IT A BUG RATHER THAN A GAP: the same
// function's success line already read `safeTerm(target)`. One value, two
// treatments, decided by whether the download worked.
//
// # blobStatusError IS downloadStatusError'S STRUCTURAL TWIN
//
// Same signature, same `defer civitai.TagStatus`, same arms — and civitai/cli#572
// gated that one ONCE at the top rather than at each return, because #566 exists
// precisely because two spellings of one rule drifted apart. This gates its twin
// the same way. If the two ever diverge, that is the defect, not the style.
//
// # THE TRANSFORM IS safeTermSingle, NOT safeTerm
//
// `internal/saferune` keeps `\n` and `\t` by design, so `safeTerm` alone leaves
// the server able to forge a whole line — civitai/cli#577, measured one command
// over, where a file name forged a `(SHA256 verified)` line on a transfer that
// had not happened. Every string here is one line, so every one collapses.
//
// # AND BOTH HALVES OF EACH `%s: %w` PAIR
//
// #577's first pass gated the `%s` operands and left `safeTermErr` on `safeTerm`,
// so the WRAPPED CAUSE still forged the line: `*fs.PathError` renders its Path
// unquoted and that Path carries the hostile name. Fixed there by moving
// `safeTermErr` onto `safeTermSingle`; this path inherits that, and the two
// `%s: %w` sites here wrap their causes through it.

// gbfHostileName is the fixture: a plausible complete success line, so a
// survivor reads as the lie it is rather than as mangled text. It carries a
// newline AND a tab — the two runes saferune deliberately retains.
const gbfHostileName = "out-1.png\nSaved /home/u/real.png (2.0 MiB)\tDONE"

// 🔴 THE ASSERTION IS assertOneLine, NOT A SECOND COPY OF IT. A first draft of
// this file defined gbfAssertOneLine — byte-identical to
// download_newline_test.go's assertOneLine but for the issue number in its
// message, in the SAME package. Two spellings of one rule is the mechanism
// civitai/cli#566 exists about, and writing a second one inside a PR applying
// that lesson is how it recurs. Found by this PR's round-0 audit.

// TestBlobStatusErrorCannotForgeALine drives all FOUR arms.
//
// Four, not three: 401/403, 404 and `default` are the error arms and the 2xx
// arm returns nil. The sibling test for `downloadStatusError` shipped covering
// three and an audit found the omitted one was `default` — precisely the arm
// that would ship raw if someone ever pushed the single top-of-function gate
// down into the arms, which is the drift this shape exists to prevent.
func TestBlobStatusErrorCannotForgeALine(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError, // the default arm
	} {
		err := blobStatusError(status, gbfHostileName)
		if err == nil {
			t.Fatalf("CONTROL failure, not a finding: status %d produced no error", status)
		}
		assertOneLine(t, "blobStatusError (HTTP "+http.StatusText(status)+")", err.Error())
		// POSITIVE CONTROL: the name must actually reach the message, or the
		// assertion above passes vacuously.
		if !strings.Contains(err.Error(), "out-1.png") {
			t.Fatalf("CONTROL failure, not a finding: the name never reached the HTTP %d message:\n%s",
				status, err.Error())
		}
	}
	// The gate must not have broken the classification it sits in front of.
	if err := blobStatusError(http.StatusOK, gbfHostileName); err != nil {
		t.Errorf("CONTROL failure, not a finding: a 2xx produced an error: %v", err)
	}
}

// TestDownloadBlobToErrorsCannotForgeALine drives all four surfaces — the
// overwrite refusal, both `%s: %w` pairs and the `Saved` line — each as its own
// subtest, each mutation-verified alone.
//
// 🔴 IT CLAIMED ALL FOUR AND DROVE TWO. An audit measured it: ungating `install
// %s`'s operand alone SURVIVED the whole suite, and ungating the `Saved` line
// entirely SURVIVED it too, while this doc and the safeTermCoveredBy row both
// listed them. `Saved` had been honestly ledgered notCovered BEFORE this PR, so
// the row made its coverage look better than the tree's. Both are driven now.
func TestDownloadBlobToErrorsCannotForgeALine(t *testing.T) {
	t.Run("overwrite refusal", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatalf("CONTROL failure, not a finding: could not create the existing file: %v", err)
		}
		err := downloadBlobTo(context.Background(), nil, io.Discard, io.Discard,
			"https://example.invalid/blob", target, false)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: an existing target produced no refusal")
		}
		if !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		assertOneLine(t, "downloadBlobTo (refusing to overwrite)", err.Error())
	})

	// 🔴 THESE TWO SUBTESTS EXIST BECAUSE THE DOC ABOVE CLAIMED THEM AND THE
	// TEST DID NOT DRIVE THEM. Measured by this PR's nine-axis audit: ungating
	// `install %s`'s operand alone SURVIVED the whole suite, and ungating the
	// `Saved` line entirely SURVIVED it too — while this file's header and the
	// safeTermCoveredBy row both listed them as covered. A row that reads as
	// coverage while providing none is the class this repo treats as worse than
	// no row, because it stops the next reader looking.
	t.Run("install %s: %w — both halves", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		// os.Rename fails when the destination is a DIRECTORY, and the error is
		// an *os.LinkError carrying both paths — so the cause repeats the
		// hostile bytes the operand already holds.
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		fetch := func(context.Context, string) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(strings.NewReader("payload")),
				ContentLength: 7,
			}, nil
		}
		err := downloadBlobTo(context.Background(), fetch, io.Discard, io.Discard,
			"https://example.invalid/blob", target, true)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: renaming onto a directory succeeded")
		}
		if !strings.Contains(err.Error(), "install ") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		assertOneLine(t, "downloadBlobTo (install %s: %w)", err.Error())
	})

	t.Run("the Saved line", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		fetch := func(context.Context, string) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(strings.NewReader("payload")),
				ContentLength: 7,
			}, nil
		}
		var out bytes.Buffer
		if err := downloadBlobTo(context.Background(), fetch, &out, io.Discard,
			"https://example.invalid/blob", target, true); err != nil {
			t.Fatalf("CONTROL failure, not a finding: the transfer failed: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "Saved ") {
			t.Fatalf("CONTROL failure, not a finding: no Saved line was written:\n%s", got)
		}
		// The success line is ONE line. A forged newline here is the worst of
		// the set: it is the line the user reads as "this finished".
		if n := strings.Count(strings.TrimRight(got, "\n"), "\n"); n != 0 {
			t.Errorf("#574 FORGERY in downloadBlobTo (Saved): %d forged line(s) on the success "+
				"line:\n%s", n, got)
		}
		if strings.Contains(got, "\t") {
			t.Errorf("#574 FORGERY in downloadBlobTo (Saved): a TAB survived:\n%s", got)
		}
	})

	t.Run("download %s: %w — both halves", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		// A fetcher that fails with a cause carrying the hostile bytes a second
		// time: gating only the %s operand leaves this half forging the line.
		fetch := func(context.Context, string) (*http.Response, error) {
			return nil, &os.PathError{Op: "open", Path: target, Err: errors.New("no such file or directory")}
		}
		err := downloadBlobTo(context.Background(), fetch, io.Discard, io.Discard,
			"https://example.invalid/blob", target, true)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: a failing fetcher produced no error")
		}
		if !strings.Contains(err.Error(), "download ") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		assertOneLine(t, "downloadBlobTo (download %s: %w)", err.Error())
		// The strip must not break the chain the exit-code classifier reads.
		var pe *os.PathError
		if !errors.As(err, &pe) {
			t.Errorf("safeTermErr broke the error chain: errors.As(*os.PathError) no longer matches, so "+
				"the published exit code can no longer see what kind of failure this is:\n  %#v", err)
		}
	})
}

// TestCreateOutputDirectoryStaysUngated is an INVARIANT guard, labelled as one:
// it passes before this change too. Its value is the mutant.
//
// 🔴 `dir` IS filepath.Dir(target) — THE USER'S OWN `--out-dir`, NEVER THE
// SERVER'S LEAF, because outputTarget refuses any name that is not its own
// filepath.Base. Sanitising it would strip USER-TYPED bytes, which
// internal/saferune's rule forbids (civitai/cli#393), and download.go carries
// the byte-identical line ungated for the same reason — civitai/cli#572's round
// 0 REMOVED a gate someone had added there, after it had already survived a
// 20-mutant sweep, because breakable is not reachable.
//
// So a reader now sees one gated twin and one ungated one and will try to make
// them agree. This is the test that tells them which direction is wrong.
func TestCreateOutputDirectoryStaysUngated(t *testing.T) {
	// A directory component the user typed, carrying a rune saferune strips.
	// The escape is required, not stylistic: staticcheck's ST1018 rejects a raw
	// U+200B in a literal, and it is the ONE linter that sees it — `make ci` does
	// not run lint, which is why AGENTS.md says to run both.
	const userDir = "my\u200bout dir"
	dir := filepath.Join(t.TempDir(), userDir)
	// Make MkdirAll fail by putting a FILE where the directory must go.
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %v", err)
	}
	target := filepath.Join(dir, "out-1.png")
	err := downloadBlobTo(context.Background(), nil, io.Discard, io.Discard,
		"https://example.invalid/blob", target, true)
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: creating a directory over a file succeeded")
	}
	if !strings.Contains(err.Error(), "create output directory") {
		t.Fatalf("CONTROL failure, not a finding: reached a different error, so this test does not "+
			"drive the line it names: %v", err)
	}
	// 🔴 COUNT, DO NOT `Contains` — AND THE FIRST DRAFT OF THIS ASSERTION WAS
	// VACUOUS FOR EXACTLY THE REASON THIS WHOLE ARC IS ABOUT. `Contains` was
	// satisfied by the WRAPPED CAUSE: the message is
	// `create output directory <dir>: mkdir <dir>: not a directory`, so the path
	// appears TWICE and `%w` renders its half raw whatever the `%s` half does.
	// Gating the operand left the cause carrying the fixture, `Contains` stayed
	// true, and the mutant SURVIVED — one value, printed twice, asserted on one
	// half. Measured, then fixed.
	//
	// The count is the discriminator: ungated renders it twice, gated once.
	if n := strings.Count(err.Error(), userDir); n != 2 {
		t.Errorf("civitai/cli#393 REGRESSION in downloadBlobTo (create output directory %%s): the "+
			"user's own --out-dir appears %d time(s) in the message, want 2 (the `%%s` operand and "+
			"the `%%w` cause). `dir` is filepath.Dir(target), which outputTarget guarantees holds no "+
			"server bytes — gating it strips what the USER typed, and someone copying that path "+
			"gets one that does not exist.\ngot: %s", n, err.Error())
	}
}

// TestDownloadOutputsErrorsCannotForgeALine drives ALL THREE of downloadOutputs'
// own error surfaces, each as its own subtest, each mutation-verified alone:
// the overwrite refusal, the no-URL error and the duplicate-target hint.
//
// 🔴 IT WAS NAMED …RefusalCannotForgeALine AND DROVE ONE OF THE THREE, WHILE THE
// LEDGER ROW CLAIMED ALL THREE. civitai/cli#596 round 2's delta audit measured
// it: the no-URL error at generate_output.go was still on plain `safeTerm` (so
// it forged a line with the whole suite green), and the duplicate-target hint's
// gate could be reverted with no BEHAVIOURAL test going red — only
// bareIdentArgs' shrank-direction bookkeeping noticed, which is a deletion
// detector, not an assertion about the screen. Renamed rather than re-scoped,
// because the old name is what made a one-surface test look like a three-surface
// one.
//
// 🔴 THE OVERWRITE REFUSAL WAS FULLY RAW, NOT MERELY UNGATED FOR `\n`.
// `j.target` is planOutputTarget's output, whose `{workflow}` placeholder is
// filled from the wire, and it was joined into the message with no sanitiser at
// all — so that one is the #566 class (raw ANSI) rather than the #577 one.
//
// RE-MEASURED AT ROUND 2, because this header and the call site both carried a
// number nobody had re-derived: reverting that one expression to `j.target` and
// running the overwrite-refusal subtest reports `esc=true`, THREE newlines where
// the gated message has two, and 1 tab. The message writes both of its own
// newlines (header, then the trailer after the row), so the server forged
// exactly ONE line, not the "two forged newlines" this comment claimed for two
// rounds.
//
// 🔴 AND THE OVERWRITE REFUSAL IS THE REACHABLE BRANCH. downloadOutputs
// pre-checks every job here before any bytes move, so downloadBlobTo's own
// `!force` refusal (which #574 DID name) fires only in a TOCTOU window. The
// requirement's table was built by reading two functions; it hardened the branch
// users almost never hit and left this one raw. Found by this PR's round-0
// audit, which is the round that asks whether the requirement itself is right.
func TestDownloadOutputsErrorsCannotForgeALine(t *testing.T) {
	// A cursor-control payload, not merely a newline: these surfaces had NO gate
	// or a \n-keeping one, so a `\n`-only fixture could not distinguish "gated
	// for lines" from "gated".
	const hostileID = "wf1\x1b[1A\x1b[2K\nSaved out.png (2.0 MiB)\tDONE"

	t.Run("overwrite refusal", func(t *testing.T) {
		// 🔴 DRIVE THE REAL FUNCTION. A first draft of this test rebuilt the
		// message inline with safeTermSingle(target) hardcoded — so it asserted
		// on the HELPER, not on the production line, and ungating that line left
		// it GREEN. Caught by its own mutation check. A guard that reconstructs
		// the thing it is guarding is testing its own arithmetic.
		dir := t.TempDir()
		url := "https://example.invalid/blob.png"
		outs := []genapi.Output{{Blob: genapi.Blob{URL: &url}}}

		// Pre-create what planOutputTarget will choose, so the refusal fires.
		target, err := planOutputTarget("", dir, hostileID, 1, url)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: planOutputTarget refused the fixture: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		// POSITIVE CONTROL on the fixture: the hostile id must actually reach the
		// target, or the refusal below cannot carry it.
		if !strings.Contains(target, "wf1") {
			t.Fatalf("CONTROL failure, not a finding: planOutputTarget dropped the id; target=%q", target)
		}

		_, err = downloadOutputs(context.Background(), nil, io.Discard, io.Discard,
			hostileID, outs, dir, "", false)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: an existing target produced no refusal")
		}
		got := err.Error()
		if !strings.Contains(got, "refusing to overwrite") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		if strings.Contains(got, "\x1b") {
			t.Errorf("#566 FORGERY in downloadOutputs (refusing to overwrite): a raw ESC survived into "+
				"the one error standing between the user and an overwrite:\n%q", got)
		}
		// Header + one line per clash. A forged newline breaks that relationship.
		if n := strings.Count(strings.TrimRight(got, "\n"), "\n"); n != 2 {
			t.Errorf("#574 FORGERY in downloadOutputs (refusing to overwrite): %d newline(s) for ONE "+
				"clash, want 2 (header, the row, the trailer) — the server chose the geometry:\n%s", n, got)
		}
		if strings.Contains(got, "\t") {
			t.Errorf("#574 FORGERY in downloadOutputs (refusing to overwrite): a TAB survived:\n%s", got)
		}
	})

	// 🔴 THE SURFACE ROUND 1 CLAIMED AND DID NOT GATE. This error names `o.ID` —
	// the SERVER's own blob id — not a target, and it was on plain `safeTerm`,
	// which keeps \n and \t by design. Measured on the unfixed line with this
	// fixture: esc=false (safeTerm did strip the escape), 1 forged newline and 1
	// surviving tab, printed by cmd/civitai/main.go as `Error: <err>` with no
	// renderer in front of it.
	//
	// 🔴 REACHABLE WITHOUT ANY TOCTOU WINDOW, which is why it is not a
	// theoretical arm: genapi.Deliverable is `Available && !blocked && !hidden`
	// and says nothing about a URL, so `"available": true` with `"url": null`
	// is partitioned into `kept` and handed straight to this loop. The fixture
	// sets Available so it is the payload the server can actually send, not a
	// value only this test can construct.
	t.Run("no-URL error", func(t *testing.T) {
		outs := []genapi.Output{{Blob: genapi.Blob{ID: hostileID, Available: true}}}
		_, err := downloadOutputs(context.Background(), nil, io.Discard, io.Discard,
			"wf-clean", outs, t.TempDir(), "", false)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: an available output with no URL produced no error")
		}
		got := err.Error()
		if !strings.Contains(got, "carries no URL") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		// POSITIVE CONTROL: the id must actually reach the message, or the
		// assertions below pass on a string that never carried the payload.
		if !strings.Contains(got, "wf1") {
			t.Fatalf("CONTROL failure, not a finding: the output id never reached the message:\n%q", got)
		}
		assertOneLine(t, "downloadOutputs (marked available but carries no URL)", got)
		if strings.Contains(got, "\x1b") {
			t.Errorf("#566 FORGERY in downloadOutputs (no URL): a raw ESC survived:\n%q", got)
		}
	})

	// 🔴 THE THIRD SURFACE THE ROW CLAIMED. Reverting its gate reddened NO
	// behavioural test at round 2 — only bareIdentArgs noticed the identifier had
	// stopped appearing, which is bookkeeping seeing a deletion, not an assertion
	// that the class stays off the screen. The target is rendered with `%q`, so
	// Go escapes whatever it holds; `workflowID` is the operand that reaches the
	// terminal unquoted, and it is the one this drives.
	t.Run("duplicate-target hint", func(t *testing.T) {
		url := "https://example.invalid/blob.png"
		// An --out-name with no {n}: both outputs render the SAME name, which is
		// the collision this hint exists for. {workflow} is deliberately absent
		// so the hostile bytes reach the message ONLY through the `%s` operand.
		outs := []genapi.Output{
			{Blob: genapi.Blob{ID: "a", URL: &url}},
			{Blob: genapi.Blob{ID: "b", URL: &url}},
		}
		_, err := downloadOutputs(context.Background(), nil, io.Discard, io.Discard,
			hostileID, outs, t.TempDir(), "img{ext}", false)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: two outputs sharing one target produced no error")
		}
		got := err.Error()
		if !strings.Contains(got, "would both be written to") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		// POSITIVE CONTROL on the operand: the workflow id must reach the hint.
		if !strings.Contains(got, "wf1") {
			t.Fatalf("CONTROL failure, not a finding: the workflow id never reached the hint:\n%q", got)
		}
		assertOneLine(t, "downloadOutputs (duplicate target hint)", got)
		if strings.Contains(got, "\x1b") {
			t.Errorf("#566 FORGERY in downloadOutputs (duplicate target hint): a raw ESC survived:\n%q", got)
		}
	})
}

// TestWaitAndCollectReReadHintCannotForgeALine pins the ONE gate civitai/cli#596
// moved inside waitAndCollect, and it exists because that gate was UNPINNED when
// it shipped: round 2's delta audit reverted generate.go's
// `safeTermSingle(workflowID)` on the re-read hint to plain `safeTerm` and the
// whole internal/cmd suite stayed green.
//
// The hint is printed on the path where a transfer FAILED after a spend — so it
// is the last thing a user reads about outputs they have already paid for and
// can still fetch, and `workflowID` is the server's own id in it.
//
// 🔴 WHAT THIS TEST DELIBERATELY DOES NOT CLAIM. Inside waitAndCollect itself
// there are THREE other safeTerm-family sites, all keeping \n and \t:
// safeTerm(wf.Status) and safeTerm(workflowID) in the terminal-status error
// (generate.go:1514), and safeTerm(workflowID) in the succeeded-but-no-
// deliverables error (:1558). Note the FIRST of those carries the server's
// STATUS, not the id.
//
// 🔴 MORE ARE REACHABLE FROM HERE, THROUGH CALLEES — AND THIS COMMENT NO LONGER
// TRIES TO LIST THEM. waitAndCollect reaches printReattach (generate.go:1447,
// :1451), printOutputURLs (:1562), classifyGenerateError (:1455) and both poll
// reporters (via newPollReporter, :1441); every one renders server text through
// plain safeTerm, which keeps \n and \t. The authoritative list is the
// notCovered rows in safeTermCoveredBy plus civitai/cli#604 — NOT this
// paragraph.
//
// 🔴 THREE SUCCESSIVE DRAFTS TRIED TO ENUMERATE THAT SET AND ALL THREE WERE
// INCOMPLETE IN THE REASSURING DIRECTION. Draft 1 credited printSubmitted's two
// lines to waitAndCollect and omitted wf.Status (caught round 3). Draft 2 fixed
// that and asserted both printSubmitted and printReattach are "different
// functions this one never calls" — true of the former, FALSE of the latter —
// pointing a reader closing #604 away from the exact path that issue measured a
// forgery on, and again saying only "the id" so safeTerm(status) went unnamed
// (caught round 4). Draft 3 named six sites and called that "the reachable-
// through set"; it missed printOutputURLs, whose safeTerm(*o.URL) on the
// --no-download path writes a TAB-separated numbered row to STDOUT, so a URL
// carrying "\n2\t<other>" forges a whole extra row on the piping surface — and
// it missed the poll reporters, which run on EVERY waiting generate rather than
// only the timeout path (caught round 5).
//
// The lesson is the one the repo already records about name blocklists: an
// enumeration like this cannot be completed by thinking harder, and each attempt
// reads as exhaustive to the next reader. State the PROPERTY and point at the
// ledger. Do not add a fourth list.
//
// All of them are pre-existing, are honestly ledgered notCovered in
// safeTermCoveredBy, and are outside #574's closing condition — they are
// civitai/cli#604 rather than a silent sweep. The slice below starts at the
// hint's own marker precisely so this test cannot accidentally pass or fail on
// their behaviour.
//
// It drives the REAL command — runGenerate down through waitAndCollect — rather
// than calling the print, because the value under test is one the poll path
// threads through several frames.
func TestWaitAndCollectReReadHintCannotForgeALine(t *testing.T) {
	withStdinTTY(t, false)
	// The server's workflow id, carrying the two runes saferune deliberately
	// keeps. No path separator: the id also lands in the output FILE NAME via
	// renderOutName, and a slash there would make outputTarget refuse before the
	// transfer seam is ever reached — a different error, printed by a different
	// line.
	const hostileWF = "wf1\nSaved out.png (2.0 MiB)\tDONE"

	payload := `{"id":"wf_123","status":"succeeded","steps":[{
	  "$type":"textToImage","name":"s","status":"succeeded","metadata":{},
	  "output":{"images":[{"id":"a","type":"image","available":true,"url":"https://example.invalid/o/a.jpeg"}]}}]}`

	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	s.submitReply = &genapi.SubmitResult{ID: hostileWF, Status: "queued"}
	// The transfer must FAIL: the hint is only printed when downloadOutputs
	// returns an error. Counted, so "the seam was reached" is measured rather
	// than assumed.
	fetched := 0
	s.downloadBlob = func(context.Context, string) (*http.Response, error) {
		fetched++
		return nil, errors.New("FIXTURE transfer failure")
	}

	c, _, errb := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a failing transfer reported success")
	}
	if fetched == 0 {
		t.Fatalf("CONTROL failure, not a finding: the blob seam was never reached, so the hint under "+
			"test was printed for some other reason (or not at all): %v", err)
	}

	all := errb.String()
	const marker = "Output URLs expire"
	i := strings.Index(all, marker)
	if i < 0 {
		t.Fatalf("CONTROL failure, not a finding: the re-read hint was never printed:\n%s", all)
	}
	// The hint is the LAST thing waitAndCollect writes before returning the
	// transfer error, so everything from the marker on is exactly one Fprintln.
	tail := all[i:]
	// POSITIVE CONTROL: the id must actually reach the hint, or the counts below
	// are being taken on a string that never carried the payload.
	if !strings.Contains(tail, "wf1") {
		t.Fatalf("CONTROL failure, not a finding: the workflow id never reached the hint:\n%q", tail)
	}
	if n := strings.Count(tail, "\n"); n != 1 {
		t.Errorf("#574 FORGERY in waitAndCollect (re-read hint): the hint occupies %d line(s), want 1 — "+
			"a server-chosen workflow id forged %d extra flush-left line(s) on the last advice a user "+
			"gets about outputs they have already been charged for:\n%q", n, n-1, tail)
	}
	// 🔴 NO TAB ASSERTION HERE, AND ITS ABSENCE IS MEASURED RATHER THAN AN
	// OVERSIGHT. This hint goes through ui's Dim style, and lipgloss EXPANDS \t
	// to four spaces before the bytes reach the buffer — measured on this tree:
	// ui.For(w).Dim("a\tb") == "a    b", same for Info and Warn. So a
	// `Contains(tail, "\t")` guard here could never fire whether or not the gate
	// exists, which is the unreachable-guard shape this repo counts as worse
	// than no guard. The sibling assertions on the UNSTYLED error strings
	// (downloadOutputs, downloadBlobTo, reportExcludedOutputs) are the ones that
	// can see a tab, and they are watched red. The \n above is what
	// discriminates on THIS surface: lipgloss does not eat newlines.
}
