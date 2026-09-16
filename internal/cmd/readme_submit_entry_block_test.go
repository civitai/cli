package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This file pins the four contract claims that #635's FOUR audit rounds
// repaired, and it exists because nothing pinned them while they were wrong.
//
// 🔴 THE MEASURED GAP. Every behavioural sentence in the `## Submit & auth`
// entry-table paragraph, and in its Troubleshooting row, was unasserted.
// TestREADMETroubleshootingSymptomsExistInTheSource guards only the LEFT-HAND
// symptom column; app_submit_oversize_test.go names those rows in a comment and
// never opens README.md. So each of the four rounds shipped a false claim into
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

// entryBlockSentinel is one `errors.Is` case in doUpload's switch, together
// with what the README is obliged to say about it. `name` is the sentinel
// expression AS WRITTEN in the switch, qualifier included — that is what
// errorsIsRe captures, and matching on the qualified form is what stops a
// same-named sentinel from a different package reading as covered.
//
// 🔴 THIS IS A LEDGER, NOT A COUNT. Membership is the assertion: adding a
// sentinel without a row here is red, and so is deleting one. A count would let
// add-one/delete-one swap a case silently — the exact defeat
// symptomAttributionsFloor was converted away from in #605.
type entryBlockSentinel struct {
	name string // the sentinel expression as written in the switch
	// paragraphSays and rowSays are text the `## Submit & auth` paragraph and
	// the Troubleshooting row's CAUSE CELL must each contain while this case
	// exists. Empty means that surface deliberately does not name the case.
	//
	// 🔴 BOTH SURFACES, BECAUSE ONE OF THEM WAS UNPINNED AND ROUND 1 INVERTED
	// IT. This struct carried only the paragraph. The row's exception list —
	// the half that actually drifted in #635 round 1 — was reachable only by
	// two keyword checks, so rewriting the cell to "including a 401/403/429",
	// the exact inverse of doUpload's second case, left the whole package
	// green. A ledger that covers one of the two surfaces it exists to keep in
	// sync is the narrower-than-its-docstring defect this file was written to
	// catch, committed inside the file itself.
	paragraphSays string
	rowSays       string
	why           string
}

var entryBlockSentinels = []entryBlockSentinel{
	{
		name:          "appapi.ErrBundleTooLarge",
		paragraphSays: "The ceiling refusal above is the one refusal with an entry list of its own",
		// No `**` here: troubleshootingEntryBlockCause strips bold markers before
		// comparing, so the expectation is about the cell's WORDS and not about
		// which of them an author bolded.
		rowSays: "`What this CLI would have sent` is the ceiling refusal alone — it sends nothing " +
			"either, and says so — and that one is exact too: nothing was uploaded.",
		why: "it takes the FIRST case and diverts to printSubmitSizeRefusal, so it is an " +
			"exception to the past-tense block rather than an instance of it. Round 0 of #635 " +
			"found the README claiming the block printed here",
	},
	{
		name: "appapi.ErrNothingSent",
		// 🔴 TWO FALSE VERSIONS OF THIS SENTENCE SHIPPED FOR ONE COMMIT EACH, IN
		// OPPOSITE DIRECTIONS, AND BOTH ARE WHY THE WORDING IS THIS EXACT.
		//
		// "prints nothing" — false for the CEILING REFUSAL, which sends nothing
		// and prints a full would-have-sent block, named three lines later in the
		// same paragraph. A universal with its counterexample already on the page.
		// Hence "never prints the PAST TENSE".
		//
		// "so `sent` is a fact about bytes that left this machine" — false for a
		// write CUT SHORT. A submit timing out mid-body had delivered ~200 KB on
		// every measured run while the CLI called it nothing sent; the fix keys the
		// tag on the request reaching the connection, and the printed byte count is
		// then an upper bound rather than a receipt. Hence "a request that really
		// went out, not one the CLI only built".
		//
		// The #635 ladder's own lesson, in claudedocs/handoff-readme-reduction.md:
		// a fix that makes a sentence more specific can make it false, and it reads
		// as an improvement because specificity looks like rigour.
		paragraphSays: "A failure that never reached the connection never prints the past tense — " +
			"no usable credential, an unwritable config, a connection that never opened — so `sent` " +
			"means a request that really went out, not one the CLI only built.",
		rowSays: "A failure that never reached the connection never prints the past tense: no " +
			"usable credential, an unwritable config and a connection that never opened print " +
			"neither block.",
		why: "issue #637 — appapi.SubmitVersion sets this from httptrace's WroteRequest, so the arm " +
			"is a WIRE FACT and not a classification. It is what lets both surfaces state the past " +
			"tense as true instead of hedging it, and #635 spent four audit rounds on that hedge",
	},
	{
		name:          "civitai.ErrUnauthorized",
		paragraphSays: "except a `401`/`403`",
		rowSays:       "and not on a `401`/`403`/`429`.",
		why: "401 and 403 both map to this sentinel in pkg/civitai/errkind.go, and both suppress " +
			"the block. Since #637 this arm is reached ONLY when the server answered, so those " +
			"numbers mean on the page what they mean on the wire — a credential refused LOCALLY " +
			"(internal/auth tags ErrUnauthorized onto \"no refresh token stored\") lands on the " +
			"ErrNothingSent arm above it",
	},
	{
		name:          "civitai.ErrRateLimited",
		paragraphSays: "or a `429`",
		rowSays:       "and not on a `401`/`403`/`429`.",
		why:           "429 maps to this sentinel, and it suppresses the block",
	},
}

// 🔴 WHY EVERY PROSE COMPARISON BELOW GOES THROUGH collapseWS (declared in
// generate_dryrun_label_test.go — one rule, one place; do not add a second).
//
// README.md is hard wrapped, so a RAW whole-sentence pin breaks the moment a
// sentence reflows. That is a COSMETIC change, and a guard that fails on it
// trains maintainers to "fix" these tests by pasting whatever the file now says
// — which is precisely the habit that would have waved through all four of the
// false claims this file exists to pin. Normalised, a reflow passes and a
// REWORD fails, and the reword is where a claim changes meaning.

// readmeEntryBlockParagraph returns the paragraph that documents the entry
// table, located by a heading-independent anchor so a reorder does not silently
// retarget it at prose that happens to match. The result is NOT normalised —
// callers that compare prose should wrap it in collapseWS.
func readmeEntryBlockParagraph(t *testing.T) string {
	t.Helper()
	md := readREADME(t)
	const anchor = "Entries are ranked by **compressed** size"
	i := strings.Index(md, anchor)
	if i < 0 {
		t.Fatalf("CONTROL failure: the entry-table paragraph's anchor %q is not in README.md, "+
			"so every assertion in this file is vacuous. If the paragraph was deliberately "+
			"reworded, move this anchor to a sentence it still contains — do not delete the anchor.", anchor)
	}
	rest := md[i:]
	if j := strings.Index(rest, "\n#### "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// doUploadSwitch returns the source text of the switch in doUpload that decides
// which entry block prints. It is anchored on the literal `switch {` + first
// case, which is specific enough not to drift onto an unrelated switch.
func doUploadSwitch(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("app_submit.go"))
	if err != nil {
		t.Fatalf("CONTROL failure: cannot read app_submit.go: %v", err)
	}
	src := string(b)
	// Bound it at the `switch {` plus its first case — NOT at either printer's
	// call site — and close at the first line that is exactly three tabs and a
	// brace, which is the switch's own closing brace. A nested block inside a
	// case closes deeper than that and cannot match.
	//
	// (This comment previously described both bounds wrongly, naming the printer
	// call site and a following `return err`. Round 1 of #639 caught it. A
	// comment is a claim like any other.)
	start := strings.Index(src, "switch {\n\t\tcase errors.Is(err, appapi.")
	if start < 0 {
		t.Fatalf("CONTROL failure: could not locate doUpload's entry-block switch in app_submit.go.\n" +
			"This guard reads the sentinel set out of that switch; if the switch was restructured, " +
			"retarget this locator rather than deleting the ledger — the ledger is what keeps the " +
			"README's exception list honest.")
	}
	rest := src[start:]
	end := strings.Index(rest, "\n\t\t}")
	if end < 0 {
		t.Fatal("CONTROL failure: found the switch but not its closing brace")
	}
	return rest[:end]
}

// errorsIsRe matches ANY errors.Is sentinel, qualified or bare.
//
// 🔴 IT WAS `(?:appapi|civitai)\.(\w+)` AND THAT MADE THE LEDGER SPELLED
// RATHER THAN STRUCTURAL. Round 1 of #639 added
// `case errors.Is(err, os.ErrDeadlineExceeded)` to the switch — `os` is already
// imported by app_submit.go — and the whole package stayed green. The doc above
// said flatly that a new sentinel is red; for any package outside that
// two-name allowlist it was invisible, and the `len(found) >= 2` control could
// not see it either because the three known sentinels still matched.
//
// The switch is package-local code, so a bare `errFoo` or a `context.` /
// `pkgzip.` sentinel are all realistic. Matching the whole expression and
// ledgering the QUALIFIED name means a new branch is red whatever it is
// spelled.
var errorsIsRe = regexp.MustCompile(`errors\.Is\(err,\s*([\w.]+)\)`)

// TestREADMESubmitEntryBlockSentinelLedger is the primary guard: it pins the
// RELATIONSHIP between doUpload's switch and what the README says about it.
//
// 🔴 It fails in BOTH directions on purpose. A sentinel added to the switch
// without a ledger row is red (the README is now silently incomplete); a ledger
// row whose sentinel left the switch is red (the README is now describing a
// branch that does not exist). Either way the prose needs rewriting, and this is
// the only thing in the tree that says so.
func TestREADMESubmitEntryBlockSentinelLedger(t *testing.T) {
	sw := doUploadSwitch(t)

	found := map[string]bool{}
	for _, m := range errorsIsRe.FindAllStringSubmatch(sw, -1) {
		found[m[1]] = true
	}
	// POSITIVE CONTROL: an extractor that has stopped seeing errors.Is calls
	// would make every comparison below trivially "matching" at zero. The
	// switch has three sentinel references today; fewer than two means the
	// regex is reading the wrong text, not that the code got simpler.
	if len(found) < 2 {
		t.Fatalf("CONTROL failure: extracted %d sentinel(s) from doUpload's switch (want >= 2). "+
			"The assertions below are vacuous at this count — the regex %q is not matching the "+
			"switch source:\n%s", len(found), errorsIsRe, sw)
	}

	ledgered := map[string]entryBlockSentinel{}
	for _, s := range entryBlockSentinels {
		ledgered[s.name] = s
	}

	for name := range found {
		if _, ok := ledgered[name]; !ok {
			t.Errorf("doUpload's switch branches on %s, but no row in entryBlockSentinels covers it.\n"+
				"A new case changes WHICH failures print the entry block, which is exactly what the "+
				"README paragraph and its Troubleshooting row promise a reader. Add a row here naming "+
				"what the README must now say — and update both surfaces, because README.md is the "+
				"only place either appears and they went out of sync for a commit in #635.", name)
		}
	}
	for name, s := range ledgered {
		if !found[name] {
			t.Errorf("entryBlockSentinels still carries %s, but doUpload's switch no longer branches on it.\n"+
				"Reason it was ledgered: %s.\n"+
				"The README currently tells readers %q (paragraph) and %q (Troubleshooting row). If the "+
				"branch is genuinely gone, delete both claims in the same commit, then drop this row.",
				name, s.why, s.paragraphSays, s.rowSays)
		}
	}

	// And BOTH README surfaces must carry what each live sentinel obliges.
	para := collapseWS(readmeEntryBlockParagraph(t))
	cause := collapseWS(troubleshootingEntryBlockCause(t))
	for _, s := range entryBlockSentinels {
		if !found[s.name] {
			continue
		}
		for _, surface := range []struct{ where, text, want string }{
			{"the `## Submit & auth` paragraph", para, s.paragraphSays},
			{"the Troubleshooting row's cause cell", cause, s.rowSays},
		} {
			if surface.want == "" || strings.Contains(surface.text, surface.want) {
				continue
			}
			t.Errorf("doUpload branches on %s, but %s no longer says %q.\n"+
				"Why that matters: %s.\n"+
				"Do not satisfy this by pasting the string back — check the surface still describes "+
				"the branch correctly, then update the expectation here if the wording moved. Both "+
				"surfaces are ledgered because the row's exception list is the half that drifted in "+
				"#635 round 1.", s.name, surface.where, surface.want, s.why)
		}
	}
}

// readmeEntryBlockClaims are the four repairs, pinned as WHOLE normalised
// sentences.
//
// 🔴 PINNED WHOLE, NOT BY KEYWORD, and that is deliberate. A guard on words is
// walkable by rewording — the exact failure mode RULES.md names — and each of
// these claims was replaced by a DIFFERENTLY WRONG one at least once during the
// ladder, which a keyword pin would have waved through. The cost is real: a
// cosmetic reword fails this test. Pay it. The repo already pays it for the
// #389 shadow-write bullet and for row 20's nine spans.
var readmeEntryBlockClaims = []struct {
	claim, want, verifyAt string
}{
	{
		claim:    "R1 — the block is about the UPLOAD CALL, and about one that actually went out",
		want:     "That block prints under **any error the upload call reports once the request has gone out**",
		verifyAt: "internal/cmd/app_submit.go — the switch is inside the `if err != nil` for client.SubmitVersion, so nine earlier `return err` sites never reach it, and its appapi.ErrNothingSent arm drops every failure that wrote no bytes",
	},
	{
		claim:    "R1b — the pre-upload refusals print nothing, named so a reader can tell which case they are in",
		want:     "A refusal that stops the submit **before** the upload step prints nothing at all: no `--yes`, a dirty work tree, the version guard, a validation failure.",
		verifyAt: "internal/cmd/app_submit.go — those four all return before doUpload",
	},
	// 🔴 THERE IS NO R3 OR R3b HERE, AND THE ABSENCE IS THE DECISION.
	//
	// #635's pair was the HEDGE that ladder had to publish because the code could
	// not support the strong version: "keyed on how the error *classifies*, not on
	// whether bytes left the machine" and "read `sent` as the CLI's account and
	// not a guarantee". #637 made the code compute the property, so the hedge is
	// now itself false and both strings are gone from README.md.
	//
	// The claim that REPLACED them is pinned by the ledger above, not here —
	// entryBlockSentinels' appapi.ErrNothingSent row carries the paragraph's whole
	// sentence AND the row's whole sentence, and it is CONDITIONAL on that arm
	// still existing in doUpload's switch, which a copy here would not be. A
	// second pin on the same string in the same surface adds no failure mode and
	// teaches the next maintainer that duplicating a claim is how this file works.
	// The weaker of two overlapping assertions is the one to delete; here they
	// were byte-identical, so the one that also checks the code is the keeper.
}

// TestREADMEEntryBlockClaimsArePinned pins the four repaired claims themselves.
//
// The ledger above pins the relationship; this pins the DISCLOSURES, which no
// amount of source-reading can derive — R3 in particular is a statement about
// what the CLI does NOT promise, and nothing in the code says that.
func TestREADMEEntryBlockClaimsArePinned(t *testing.T) {
	para := collapseWS(readmeEntryBlockParagraph(t))
	// POSITIVE CONTROL: a paragraph that shrank to nothing would satisfy no
	// Contains check and every failure below would read as four separate
	// regressions rather than one lost section.
	if len(para) < 400 {
		t.Fatalf("CONTROL failure: the entry-table paragraph is %d bytes (want >= 400). "+
			"It is too short to carry these claims. Two causes, and check which: the locator may be "+
			"reading the wrong region (retarget the anchor), OR the paragraph really was trimmed — "+
			"this repo is mid-way through a documented README-reduction effort, so that is a live "+
			"possibility and not a remote one. If it was trimmed, the four claim-specific failures "+
			"below are the ones to read.", len(para))
	}
	for _, c := range readmeEntryBlockClaims {
		if !strings.Contains(para, c.want) {
			t.Errorf("the README no longer makes this claim: %s\n\nwant verbatim:\n%q\n\n"+
				"🔴 Before you re-paste it: this claim was WRONG on main and was repaired across four "+
				"audit rounds of #635, and three of those rounds were fixing the previous round's fix. "+
				"Re-verify it at: %s\n"+
				"If it is genuinely being reworded, change the expectation here in the SAME commit, "+
				"and change the Troubleshooting row too — README.md is the only place either surface "+
				"appears and they disagreed for one commit during that ladder.",
				c.claim, c.want, c.verifyAt)
		}
	}
}

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

// troubleshootingEntryBlockCause returns the CAUSE cell — column two — of the
// Troubleshooting row documenting the entry block.
//
// 🔴 COLUMN TWO, NOT THE WHOLE ROW, AND THAT IS THE POINT. Column one is the
// symptom index and is pinned verbatim against non-test source by a sibling
// guard, so any assertion made against the full line can be satisfied by text
// that cannot change. The cause cell is the half that carries the claim.
func troubleshootingEntryBlockCause(t *testing.T) string {
	t.Helper()
	section := readmeTroubleshootingSection(t)
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "|") || !strings.Contains(line, "largest entries in the bundle") {
			continue
		}
		cols := strings.Split(line, " | ")
		if len(cols) < 3 {
			t.Fatalf("CONTROL failure: the entry-block row does not split into >= 3 columns, so the "+
				"cause cell cannot be isolated and every assertion on it is vacuous. Row:\n%s", line)
		}
		// Strip markdown emphasis before returning.
		//
		// 🔴 NOT COSMETIC — WITHOUT THIS THE ASSERTION IS WRONG, AND IT WAS.
		// The cell writes the tense as `What this CLI **would have** sent`, so
		// the literal substring "would have sent" does not occur: the bold
		// markers sit INSIDE the phrase. The first reachable run of this guard
		// failed on exactly that — which is also the cleanest evidence that
		// making it reachable changed something. Stripping `**` makes the check
		// about the WORDS the cell uses rather than which of them an author
		// bolded, for the same reason prose here goes through collapseWS.
		cause := strings.ReplaceAll(cols[1], "**", "")
		// POSITIVE CONTROL: a cell this short is not a cause cell — it means the
		// split landed on the wrong column, which is exactly the failure that
		// made this guard vacuous the first time.
		if len(cause) < 80 {
			t.Fatalf("CONTROL failure: the extracted cause cell is %d bytes (want >= 80), so the "+
				"column split is reading the wrong field. Extracted:\n%q", len(cause), cause)
		}
		return cause
	}
	t.Fatal("CONTROL failure: no Troubleshooting row quotes `largest entries in the bundle`, " +
		"so every rowSays check in the ledger is asserting against an empty string. The row is on " +
		"no floor, so it CAN be deleted — but deleting it silently un-documents the block, and this " +
		"guard is what makes that visible.")
	return ""
}

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
// The fifth is TestREADMETroubleshootingSymptomsExistInTheSource, and it is a
// fact about a STRING, not about the gate: the README indexes `refusing to
// submit without --yes` as a symptom and requires it to exist in non-test
// source, so deleting the literal reddens it whether or not the gate still
// works. The isolated mutant is the one that measures this gate.
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
