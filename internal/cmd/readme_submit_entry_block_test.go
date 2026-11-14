package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ⚠ RETIRED IN PART (README trim toward npm's 64 KB cap). The `## Submit & auth`
// entry-table paragraph this file pinned was cut from README.md; that detail now
// lives on the docs site's packaging guide. TestREADMESubmitEntryBlock-
// SentinelLedger and TestREADMEEntryBlockClaimsArePinned went with it. What is
// left is the BEHAVIOURAL half below (a pre-upload refusal prints no entry
// block), which asserts the code, not the README. The history below is kept as
// the record of why the claims were pinned.
//
// This file pinned the four contract claims that #635's FOUR audit rounds
// repaired, and it existed because nothing pinned them while they were wrong.
//
// 🔴 THE MEASURED GAP. Every behavioural sentence in the `## Submit & auth`
// entry-table paragraph, and in its troubleshooting row, was unasserted.
// TestPublishedTroubleshootingSymptomsExistInTheSource guards only the LEFT-HAND
// symptom column; app_submit_oversize_test.go names those rows in a comment and
// never opens README.md.
//
// ⚠ THE TWO SURFACES ARE NO LONGER BOTH IN README.md. The paragraph still is;
// the troubleshooting row is published at
// developer.civitai.com/site/guide/cli-troubleshooting and VENDORED here by
// publishedTroubleshootingRows. Where a message below says "README.md is the only
// place either appears", read it as the history it is — see the re-pointing note
// above troubleshootingEntryBlockCause. So each of the four rounds shipped a false claim into
// the published user contract with the full suite green, and the next round
// found it by reading rather than by running anything.
//
// The four repairs, in the order the rounds made them:
//
//	R1  the block prints for a failure of the UPLOAD CALL — not for "any submit
//	    failure". Nine earlier `return err` sites never reach it.
//	R2  the ceiling refusal diverts to a DIFFERENT block, so it is an exception
//	    and not an instance.
//	R3  the branch is keyed on how the error CLASSIFIES, not on whether bytes
//	    left the machine — so the past tense is an account, not a guarantee.
//	    ⚠ SUPERSEDED BY #637, which made the code compute the wire fact. Both of
//	    R3's sentences are gone from README.md and neither is pinned here any
//	    more; the appapi.ErrNothingSent row below carries what replaced them.
//	    Kept in this list because it is the history that explains the row.
//	R4  both surfaces that state this contract move together. README.md is the
//	    only place either appears, and they disagreed for one commit.
//
// 🔴 A PROSE PIN ALONE WOULD BE THE WRONG GUARD, so the ledger below is the
// primary one. A string pin catches prose drift and goes STALE when the code
// legitimately changes — it would keep asserting a sentence about a switch that
// no longer has those cases. The ledger pins the RELATIONSHIP instead: it reads
// the sentinel set out of app_submit.go and fails when that set GROWS or
// SHRINKS, which is the moment the prose needs rewriting. RULES.md: "A seam
// guard must pin a RELATIONSHIP, not a component."

// 🔴 WHY EVERY PROSE COMPARISON BELOW GOES THROUGH collapseWS (declared in
// generate_dryrun_label_test.go — one rule, one place; do not add a second).
//
// README.md is hard wrapped, so a RAW whole-sentence pin breaks the moment a
// sentence reflows. That is a COSMETIC change, and a guard that fails on it
// trains maintainers to "fix" these tests by pasting whatever the file now says
// — which is precisely the habit that would have waved through all four of the
// false claims this file exists to pin. Normalised, a reflow passes and a
// REWORD fails, and the reword is where a claim changes meaning.

// R4 — "the two surfaces stating this contract must not drift apart" — used to
// be TestREADMEEntryBlockSurfacesAgree, and it is now the LEDGER's job.
//
// 🔴 THAT TEST WAS DELETED BECAUSE IT HAD BECOME THE SHAPE IT ITSELF CONDEMNED.
// Its last assertion was a substring shared by both surfaces; by then
// entryBlockSentinels carried paragraphSays AND rowSays as whole sentences, and
// every string the shared check looked for was a substring of one of those. The
// PR's own mutation matrix showed it: no mutant was ever killed by SurfacesAgree
// alone — the ledger killed each one first. Its own closing comment said "the
// weaker of two overlapping assertions is the one that teaches a maintainer the
// wrong bar", which by then described the test it was written in.
//
// What replaced it is not less coverage: the ledger asserts, per live sentinel,
// that the paragraph carries its paragraphSays and the cause cell carries its
// rowSays. Two surfaces, checked together, per branch. The drift #635 shipped —
// the row keeping wording the paragraph had just called wrong — is red under
// that, and is the mutation matrix's M6.

// TestREADMEPreUploadRefusalPrintsNoEntryBlock is the BEHAVIOURAL half of R1.
//
// The name carries the README prefix deliberately: this asserts a claim the
// README makes, and the gate command in the handoff is
// -run 'Attribution|Troubleshooting|README|Readme|readme'. A behaviour-shaped
// name would sit OUTSIDE that filter and run only under a full ./... — which is
// exactly how the narrow filter used to miss the Attribution guards.
//
// 🔴 A STRUCTURAL GUARD TYPE-CHECKS PAST A WRONG REALITY. The ledger above
// proves the README's exception list matches doUpload's switch; it cannot prove
// that a pre-upload refusal never reaches that switch in the first place. That
// is the claim the README makes to a CI author — "prints nothing at all" — and
// it is the one the arc got wrong: the wording said "any submit failure" while
// nine earlier `return err` sites exit without printing anything.
//
// So this drives the real command down the no-`--yes` path and asserts BOTH
// block headers are absent from stderr. RULES.md: a seam guard needs "a
// behavioural case, since a structural check type-checks past a wrong argument."
func TestREADMEPreUploadRefusalPrintsNoEntryBlock(t *testing.T) {
	stderr := nonTTYSubmitRefusal(t)

	// The two block headers, taken from app_submit.go rather than retyped, so a
	// reword of either moves this assertion with it.
	for _, header := range []string{"What this CLI sent", "What this CLI would have sent"} {
		if strings.Contains(stderr, header) {
			t.Errorf("a pre-upload refusal printed %q.\n"+
				"README.md tells a reader that a refusal stopping the submit before the upload step "+
				"\"prints nothing at all\", naming no --yes as one of four such cases. If this block "+
				"is now meant to print here, that sentence and its Troubleshooting row both need "+
				"rewriting — they are the only two places the contract appears.\nstderr:\n%s",
				header, stderr)
		}
	}
}

// nonTTYSubmitRefusal drives `app submit` down the no-TTY, no-`--yes` path
// against a recorder server, asserts that the refusal happened AND that the
// endpoint was not reached, and returns the run's stderr for a caller to assert
// something further about.
//
// It does NOT cover the rest of that refusal's contract: that no `Packaged …`
// line is printed and no .zip is left on disk is
// TestAppSubmit_NonTTYRefuseHappensBeforePackaging, which does not come through
// here and keeps its own setup — no recorder server, and CIVITAI_BASE_URL set to
// the real https://civitai.com. No intent is claimed for that: git dates the
// test to #168, years before this driver existed, so "never consolidated" fits
// the evidence as well as "chosen". ⚠ It has a measured cost — under a mutant
// that removes the --yes gate, that test issues a LIVE request to civitai.com
// and dies on `unauthorized (401)`. Anyone moving it behind a recorder is fixing
// something, not breaking a decision.
//
// It is shared because #639 found ~30 lines of this setup duplicated verbatim
// between here and TestAppSubmit_NonTTYRefusesWithoutYes_NoNetworkCall, down to
// the literal token string. Both callers exercise the same path and differ only
// in what they read off it — one rule, one place.
//
// 🔴 THE "NO NETWORK CALL" ASSERTION IS THE DRIVER'S, NOT A CALLER'S, AND THAT
// IS A REPAIR TO HOW #639 LEFT IT. That PR returned the recorder's hit flag and
// let each caller assert `!hit` — which cannot fire. Reaching the endpoint means
// the run got past the refusal, so one of the two controls below has already
// fataled on it: both `if hit` branches were dead code.
//
// Measured by removing confirmSubmit's non-TTY refusal, whole-package, and
// stated with the mutant that produced it — three drafts of this paragraph
// quoted a count without naming one, and every draft was wrong in a different
// way:
//
//	ISOLATED — arm returns nil, message literal kept in the file:  4 red
//	WIDE     — the whole `if !stdinIsTTY()` block deleted:         5 red
//
// The fifth is TestPublishedTroubleshootingSymptomsExistInTheSource, and it is a
// fact about a STRING, not about the gate: the published index carries `refusing
// to submit without --yes` as a symptom and the vendored ledger requires it to
// exist in non-test source, so deleting the literal reddens it whether or not the
// gate still works. The isolated mutant is the one that measures this gate.
//
// ⚠ AND THE EARLIER "A FILTERED SWEEP MISSED THE FIFTH" DIAGNOSIS IS RETRACTED,
// not softened. Whole-package under ISOLATED also returns four, so the first
// count was the right answer for its own mutant — the filter was a latent hazard
// and not the cause. The repo's own gate filter at the head of this file even
// matches the symptom guard by name. What was actually wrong, every time, was
// quoting a number as if it were mutant-independent.
//
// Under ISOLATED, two of the red tests name production plainly:
// TestConfirmSubmit_NonTTYRefusesWithoutYes ("non-TTY without --yes must
// refuse", in app_submit_r2_test.go) and
// TestAppSubmit_NonTTYRefuseHappensBeforePackaging. Under WIDE the symptom guard
// is a third. So what #639's shape got wrong is
// narrower than "the reader is misdirected": the ONE test whose whole subject
// is the network call died with "the caller is measuring a different path than
// it claims" — a sentence about the harness, in the guard that is supposed to
// name the gate — while the endpoint had in fact been hit. Before #639 that
// same mutation printed "submit endpoint was hit — the gate did NOT prevent the
// submission". So the hit check runs FIRST and keeps that wording: it is the
// most specific fact available about a failed run.
func nonTTYSubmitRefusal(t *testing.T) (stderr string) {
	t.Helper()
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)

	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-should-not-be-used")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	out, errOut, err := run(t, "app", "submit", tmp)
	if hit {
		t.Fatalf("submit endpoint was hit — the gate did NOT prevent the submission.\n"+
			"A bare `app submit` in a non-interactive shell must refuse before contacting the "+
			"server; this run reached it. The refusal error, if any, was: %v\nstdout:\n%s\nstderr:\n%s",
			err, out, errOut)
	}
	if err == nil {
		t.Fatalf("CONTROL failure: the bare non-TTY submit did not refuse, so the caller never "+
			"reached the path it is asserting about.\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
	if !strings.Contains(err.Error(), "refusing to submit without --yes") {
		t.Fatalf("CONTROL failure: refused, but not by the --yes gate — the caller is measuring a "+
			"different path than it claims: %v", err)
	}
	return errOut
}
