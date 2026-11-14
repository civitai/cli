package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ⚠ RETIRED IN PART (README trim toward npm's 64 KB cap). The `--no-download`
// bullet was cut from README.md's `## Generate` (it lives on
// the docs site's generate guide), and its string+behaviour pin,
// TestREADMENoDownloadListingClaimMatchesTheRenderer, went with it. Only the
// wait-path test below remains.
//
// readme_generate_wait_claims_test.go pinned the two README sentences
// civitai/cli#604 added, because both of them shipped FALSE and nothing could
// say so.
//
// 🔴 THE FAILURE WAS NOT A TYPO IN EITHER CASE — IT WAS PROSE CLAIMING MORE
// THAN THE CODE DOES, IN THE REASSURING DIRECTION.
//
//   - "One row per output" described printOutputURLs' loop as if it printed
//     every kept output. It numbers by index into `kept` and SKIPS an output
//     with no URL, and genapi.Deliverable does not require one — so a server
//     returning `url: null` legitimately produces a listing with a gap, and the
//     README promised a row that is not there.
//   - "every line `generate` writes while it waits … flattened the same way" was
//     false at the commit that wrote it: the quiet poll reporter rendered the
//     failed status check's error with a plain `%v`, and that error's text is
//     the server's own `message` verbatim.
//
// 🔴 TWO HALVES PER CLAIM, AND NEITHER ALONE IS THE GUARD. The STRING half pins
// the whole normalised bullet, because a guard on individual WORDS is walkable
// by rewording — this repo's rule for a prose artifact is to pin the normalised
// string and pay the cosmetic-reword cost for a machine-readable claim. The
// BEHAVIOURAL half drives the renderer the sentence is about, so the pair fails
// in both directions: reword the README and the string half goes red, change
// the renderer and the behavioural half does. A string pin alone would freeze a
// sentence without ever asking whether it is TRUE, which is the state both of
// these were in.

// 🔴 THE `wantFlattenedFieldsSentence` CONSTANT WAS DELETED HERE, NOT KEPT AS A
// RECORD. It held the `label: value` bullet's enumeration sentence, compared by
// regex against README.md's `### What a table cell can contain` — a `###` under
// `## Global flags`, both now a pointer to
// developer.civitai.com/site/guide/cli-output.
//
// Re-pointing it left the constant with exactly ONE non-message consumer: a
// `strings.Contains(wantFlattenedFieldsSentence, "…")` control — a check that a
// constant contains a substring of ITSELF. Zero coupling to the code and zero to
// the page, and the mutation that "covered" it only edited the constant. Keeping
// it would have read as coverage while providing none, which this repo holds to
// be worse than none. The residual below already conceded the anti-widening
// property was lost; deleting the constant finishes that retirement honestly
// instead of leaving a monument to it.
//
// The sentence is verbatim on the published page, so it is recorded HERE, where a
// future editor of that page can still diff against it and nothing can mistake it
// for an assertion:
//
//	The single-line metadata fields — `images … --meta`'s model / sampler / seed /
//	resources, `app status --id`'s live URL and block id, `app listing status`'s
//	screenshot ids and captions, the `--no-wait` re-attach hint, and `generate`'s
//	wait-path lines (the submit receipt, the status line the poll prints or
//	redraws, the server's own message when a status check fails and is retried,
//	and the re-attach block printed when a wait ends without a result) — are
//	flattened the same way.
//
// ⚠ DECLARED RESIDUAL: nothing detects a WIDENING of that published sentence —
// in particular, growing it to cover `generate`'s deliberately MULTI-LINE error
// reason (serverReasonSuffix, indentContinuation), which the old regex plus its
// `multiLineClaim` contradiction check used to forbid. Both are gone with the
// section they read. What survives is the only half that was ever about the CODE.

// TestREADMEFlattenedWaitPathClaimIsNotWiderThanTheCode measures the one surface
// the published enumeration's narrowest clause is about: the poll's retry line,
// which renders the SERVER's own status message and must stay on one line and one
// tab-separated field.
func TestREADMEFlattenedWaitPathClaimIsNotWiderThanTheCode(t *testing.T) {
	// The behavioural half, on the surface the sentence's narrowest clause is about:
	// the poll's retry line, which renders the SERVER's own message.
	var b bytes.Buffer
	r := &quietPollReporter{w: &b, now: newFakeClock().Now, heartbeat: 0}
	r.tick(pollEvent{
		attempt: 2,
		status:  gfsHostileStatus,
		wait:    5 * time.Second,
		err:     apiErrorWithMessage(t, 500, gfsHostileServerMessage),
	})
	line := b.String()
	// POSITIVE CONTROL: both server operands must be in the rendered line, or the
	// flattening assertion is measured on a line that carried neither.
	if !strings.Contains(line, "processing") || !strings.Contains(line, "boom") {
		t.Fatalf("CONTROL failure, not a finding: the status and the server's message did not both reach "+
			"the poll's retry line:\n%q", line)
	}
	if n := strings.Count(strings.TrimSuffix(line, "\n"), "\n"); n != 0 {
		t.Errorf("the published output contract (developer.civitai.com/site/guide/cli-output; the sentence "+
			"is quoted in the comment above this test) says the status line the poll prints — the "+
			"server's own message included — is flattened. It emitted %d extra line(s):\n%q", n, line)
	}
	if strings.Contains(line, "\t") {
		t.Errorf("the published output contract says that line is flattened to one line AND one "+
			"tab-separated field. A TAB survived:\n%q", line)
	}
}
