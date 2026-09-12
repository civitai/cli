package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// civitai/cli#399 — safeTerm WAS UNPINNED AT MOST OF ITS CALL SITES, AND
// NOTHING COULD SAY WHICH.
//
// safeTerm is the CLI's ONE gate on server-supplied text reaching a terminal
// (see internal/saferune's package doc for the class it removes, and
// safeterm.go for what a hostile field does without it). It is called from 152
// sites in 60 functions. An adversarial audit of #398 deleted `safeTerm(...)`
// at each of 25 sampled sites in turn: 20 of them left the WHOLE SUITE GREEN.
// A refactor could drop the call and CI would say nothing.
//
// 🔴 THE PROBLEM WAS NOT A MISSING TEST, IT WAS A MISSING NUMBER. Nobody could
// answer "which of these sites is pinned?" without re-running that audit by
// hand, so the answer was never written down and never re-checked. A table of
// 60 rows beats 152 tests: this is the answer, in a form a compiler keeps
// honest.
//
// It is deliberately NOT a claim that every site is covered. Most rows say
// notCovered, and that is the point — the absence is now IMPOSSIBLE NOT TO
// WRITE DOWN, counted by the test, and capped by maxUncoveredSafeTermFuncs so
// it can only shrink. A ledger that could only record coverage would have been
// satisfied by deleting the rows nobody had got to.
//
// Why the ENCLOSING FUNCTION is the key, and not the call site's argument
// expression: #398's bareIdentArgs is keyed by argument name and its own
// comment concedes the weakness — "the same name holds a server-returned id at
// other call sites". A function is the unit a test actually drives, the unit a
// refactor moves, and the unit whose row goes stale the moment its last
// safeTerm call is deleted.
//
// 🔴 TWO DIFFERENT CLAIMS LIVE HERE, AND CONFUSING THEM WOULD OVERSTATE WHAT
// THIS BUYS. Say plainly what each half covers:
//
//   - The LEDGER pins that a function still calls safeTerm AT ALL. Deleting a
//     function's LAST safeTerm call empties its row and fails SHRANK below —
//     for every row, notCovered ones included. That is the whole set of 60
//     functions, and it is what #399's headline measurement was about.
//   - A NAMED TEST pins that the class does not reach the SCREEN. That is
//     strictly more, and it is the only thing that catches deleting ONE of a
//     function's twelve calls, or a new field printed without the gate.
//
// So a notCovered row is not "unguarded"; it is "guarded against wholesale
// removal, not against a per-field regression". Do not read the covered count
// as a coverage percentage of the 152 call sites — it is a count of FUNCTIONS
// whose output a test actually inspects.
//
// The walk is scanSafeTermCallSites in safeterm_userinput_test.go — the same
// one #398 built for the opposite property (that no site sanitises what the
// USER typed). #399's own body asked for it there: "the same walk is the
// natural place to hang a coverage ledger."

// safeTermCoverage is one row: what pins the safeTerm call(s) in one function.
type safeTermCoverage struct {
	// test is the name of the test that goes RED when the safeTerm call(s) in
	// this function are deleted — measured by deleting them, not by reading the
	// test and believing it. notCovered means no test in this package does.
	//
	// 🔴 IT IS RESOLVED TO A DECLARATION, not used as a label. A ledger naming a
	// test that does not exist is worse than no ledger: it reads as coverage and
	// stops anyone looking. checkCoveringTestsResolve is what makes the name a
	// binding, so renaming or deleting the test is red HERE.
	test string
	// why is what that test asserts — or, for a notCovered row, what a hostile
	// server field could do at this surface today. Every row must say something;
	// an empty why is a row nobody thought about.
	why string
}

// notCovered is a FIRST-CLASS LEDGER VALUE, not a hole in the table.
//
// Writing it down is the whole mechanism. The alternative — leaving a function
// out until someone writes its test — makes "unmeasured" and "covered"
// indistinguishable, which is the state #399 found the repo in.
const notCovered = ""

// 🔴 EVERY ROW'S VERDICT WAS MEASURED, NOT READ OFF THE TESTS.
//
// Method, so it can be repeated: for each function below, every `safeTerm(x)`
// inside it was rewritten to `x` by byte splice (nothing else in the file
// moved), then `go test ./internal/cmd/ -count=1` was run against a tree frozen
// at cf5e4a8. A named test is one that went red; notCovered means the suite
// stayed green. Controls on that sweep: the unmutated tree was green first, the
// mutated tree was diffed to prove the mutant was actually on disk, and
// neutering safeTerm itself reddened 20 tests — so a SURVIVED verdict is not a
// harness that cannot see red.
//
// The baseline that produced, at cf5e4a8 and BEFORE this change: 23 of the 59
// functions were killed by some test, 36 SURVIVED losing every safeTerm call
// they had. That is #399's headline, re-measured at the level of functions
// rather than of sampled sites. The rows below were then re-measured against
// the tree this commit ships, which is what they describe.
//
// 🔴 TWO ROWS NAME AN INCIDENTAL KILLER AND SAY SO. Deleting the safeTerm in
// emitPreDownloadNotes or parseGraphInput reddens
// TestSafeTermIsNeverAppliedToUserTypedInput — but only because its
// bareIdentArgs ledger notices a classified NAME has stopped appearing. That is
// bookkeeping noticing a deletion, not an assertion that the class stays off
// the screen, and writing it down as ordinary coverage would overstate what
// those two surfaces have.
var safeTermCoveredBy = map[string]safeTermCoverage{
	// --- read path: models / versions -------------------------------------
	"printModelList": {"TestModelsSearchSanitizesControlChars",
		"`models search` rows: an uploader's model name, type and creator username"},
	"printModelDetail": {"TestModelsGetSanitizesControlChars",
		"`models get`: name, type, creator, and each version's name and base model"},
	"printModelVersionDetail": {"TestModelVersionsGetSanitizesControlChars",
		"`model-versions get`: version name, parent model, AIR, download URL, file names"},
	"joinTags": {"TestModelsGetSanitizesControlChars",
		"the shared tag / trained-word list — trigger words are uploader free text"},
	"nonModelFileMarker": {"TestReadRenderersStripTheInvisibleClass",
		"the [Archive] / [Training Data] marker: `files[].type` INTERPOLATED INTO A HEADER LINE. " +
			"It was raw until #399 — the same field is safeTerm'd in the files table below it"},

	// --- read path: users / tags / creators --------------------------------
	"printUser": {"TestUsersGetSanitizesControlChars",
		"`users get`: the username and avatar URL"},
	"printTagList": {"TestReadRenderersStripTheInvisibleClass",
		"`tags search` rows: tag name and link"},
	"printCreatorList": {"TestReadRenderersStripTheInvisibleClass",
		"`creators search` rows: username and link"},

	// --- read path: collections / articles ---------------------------------
	"printCollectionList": {"TestReadRenderersStripTheInvisibleClass",
		"`collections search` rows: name, type and owner username"},
	"printCollectionDetail": {"TestReadRenderersStripTheInvisibleClass",
		"`collections get`: name, type, read scope, description and owner"},
	"joinCollectionTags": {"TestReadRenderersStripTheInvisibleClass",
		"a collection's tag list"},
	"printArticleList": {"TestReadRenderersStripTheInvisibleClass",
		"`articles search` rows: title and author username"},
	"printArticleDetail": {"TestArticlesGetSanitizesControlChars",
		"`articles get`: title, author and tags"},
	"joinArticleTags": {"TestArticlesGetSanitizesControlChars",
		"an article's tag list"},
	"htmlToText": {"TestHTMLToTextStripsControlChars",
		"`articles get --content`: the article BODY, the largest free-text surface the CLI prints"},

	// --- app path: the tabwriter renderers that had NO gate at all ----------
	// 🔴 THESE THREE WERE INVISIBLE TO THIS LEDGER UNTIL civitai/cli#552, AND
	// NOT BECAUSE THEY WERE SAFE. Its rows are keyed by "functions that call
	// safeTerm", so a renderer that sanitises NOTHING has no row, no count and no
	// signal — the absence reads exactly like "no such surface exists". #552
	// named printSubmissionTable specifically for that reason. They are here now
	// because they acquired a gate, and tabwriterRenderers
	// (tabwriter_ledger_test.go) is the ledger that could have seen them without
	// one.
	"printSubmissionTable": {"TestTabwriterRenderersCannotBeForged",
		"`app status` rows: block id, version, status, deploy state, source commit, date and live URL"},
	"printSubmissionDetail": {"TestTabwriterRenderersCannotBeForged",
		"`app status --id`: the same fields plus the publish-request id and the deploy detail. The named " +
			"test drives its CELLS. Its four NON-cell surfaces — rejection reason, approval notes, live " +
			"URL and the block id in the not-live sentence — are killed by " +
			"TestGatedRenderersDoNotForgeOutsideTheirTable instead; they were ungated while this row read " +
			"as coverage of the whole function, which is why both tests are named here"},
	"printListingStatus": {"TestTabwriterRenderersCannotBeForged",
		"`app listing status`: the server's listing status, beside the slug the USER typed. The screenshot " +
			"id and caption printed below the flushed table are NOT cells and are killed by " +
			"TestGatedRenderersDoNotForgeOutsideTheirTable"},

	// --- read path: apps ----------------------------------------------------
	"printAppList": {"TestReadRenderersStripTheInvisibleClass",
		"`app list` rows: name, slug, kind, category and author"},
	"printAppDetail": {"TestReadRenderersStripTheInvisibleClass",
		"`app view`: name, slug, tagline, category, content rating, description, and BOTH arms " +
			"of the kind switch (onsite live URL; offsite subKind / external URL / client id)"},
	"appCardAuthor": {"TestReadRenderersStripTheInvisibleClass",
		"the creator chip shared by the app list and detail"},
	"appViewOwnedAdvice": {"TestReadRenderersStripTheInvisibleClass",
		"the 404-but-owned advice, which quotes the submission's server-supplied status"},
	"offsiteRegisteredAt": {"TestOffsiteRefusalSanitizesControlChars",
		"the registration date quoted in an offsite app's refusal"},

	// --- read path: images --------------------------------------------------
	"printImageList": {"TestImagesSearchBaseModelSanitized",
		"`images search` rows: uploader, base model, rating and URL"},
	"printImageMetaBlock": {"TestImagesSearchMetaHumanOutput",
		"`images search --meta`: the generation prompt — attacker-controlled free text by design"},
	"printImageResources": {"TestImagesSearchMetaResourceSanitized",
		"the meta.resources recipe: resource type, name, weight and hash"},

	// --- the one COMPOSITION row ---------------------------------------------
	// safeTermSingle is a sanitizer, not a renderer: its safeTerm call is
	// composition (see sanitizerComposers in safeterm_userinput_test.go). The row
	// names a renderer test because that is where its output reaches a screen.
	// 🔴 WATCHED, NOT READ: deleting `s = safeTerm(s)` inside safeTermSingle
	// reddened TestReadRenderersStripTheInvisibleClass/`images_search`_table,
	// TestImagesSearchMetaResourceSanitized and TestImagesSearchBaseModelSanitized.
	"safeTermSingle": {"TestReadRenderersStripTheInvisibleClass",
		"collapses \\n for single-line/tabwriter fields, delegating to safeTerm first — so the invisible class must still be stripped"},

	// --- download path ------------------------------------------------------
	"printDownloadPlan": {"TestDownloadPlanSanitizesControlChars",
		"the plan a user reads before fetching: file name, sha256, target and notes. An escape here " +
			"can overwrite the `SHA256 verified` line the plan is there to establish"},
	"reportBaseModel": {"TestDownloadPlanSanitizesControlChars",
		"the base-model line and the mismatch warnings above the plan"},
	"formatFileList": {"TestReadRenderersStripTheInvisibleClass",
		"the ambiguity list naming each candidate file by server-supplied name and type"},
	"emitPreDownloadNotes": {"TestSafeTermIsNeverAppliedToUserTypedInput",
		"INCIDENTAL, NOT BEHAVIOURAL: the published file name in the `no SHA256 published` warning. " +
			"The red comes from bareIdentArgs noticing `name` stopped being passed, not from any " +
			"assertion about what reaches the terminal"},

	// --- generate path ------------------------------------------------------
	"describeVersion": {"TestGenerate_SanitisesServerStrings",
		"the resolved checkpoint/LoRA name and type on the PRE-SPEND quote screen"},
	"serverReasonSuffix": {"TestGenerate_ServerReasonIsSanitizedForTheTerminal",
		"the orchestrator's failure reason appended to a generate error"},
	"printWorkflow": {"TestWorkflowsGet_ReasonBlockIsSanitized",
		"`workflows get`: id, status, timestamps, the failure-reason block and each output"},
	"printWorkflowList": {"TestWorkflowsList_ReasonCannotCarryTerminalControlBytes",
		"`workflows list`: each row's id/status/date, the wrapped reason lines and the next cursor"},
	"reportExcludedOutputs": {"TestReportExcludedOutputs_ReasonIsSanitized",
		"the excluded-output line: output id and the server's exclusion reason"},
	"reportModelSubstitutions": {"TestReportModelSubstitutions_SanitizesTheServerReason",
		"the server's reason for swapping the model the user asked to spend on"},
	"reportWorkflowSettlement": {"TestReportWorkflowSettlement_SanitisesTheServerType",
		"the settlement table's transaction TYPE, printed next to a Buzz amount"},
	"parseGraphInput": {"TestSafeTermIsNeverAppliedToUserTypedInput",
		"INCIDENTAL, NOT BEHAVIOURAL: the `workflow` value out of the user's own --input FILE — " +
			"saferune's documented file-content exception. The red comes from bareIdentArgs noticing " +
			"`workflow` stopped being passed"},

	// --- NOT COVERED --------------------------------------------------------
	// Each of these lost every safeTerm call it had and the whole suite stayed
	// green (measured at cf5e4a8). They are still pinned against WHOLESALE
	// removal by the SHRANK check above; what is missing is an assertion that
	// the class does not reach the screen.
	"ambiguousYesNote": {notCovered,
		"the --yes ambiguity note quotes the server's model and parent name"},
	"ambiguousStopError": {notCovered,
		"the same two names in the refusal that stops an ambiguous download"},
	"warnMixedTypes": {notCovered,
		"the mixed-file-type warnings above a download"},
	"downloadSelected": {notCovered,
		"the per-file routing note built from the server's file name"},
	"downloadOne": {notCovered,
		"the `Saved <target>` line — the one line that says where bytes landed"},
	"presentTargetSatisfies": {notCovered,
		"the `already present (SHA256 verified)` lines, which assert an integrity result"},
	"pickleArchiveNote": {notCovered,
		"the pickle/archive EXECUTION WARNING, whose text embeds the server's file name"},
	"printOutputURLs": {notCovered,
		"the generated output URLs printed for piping"},
	"printSubmitted": {notCovered,
		"the submitted-workflow id and its civitai.com link"},
	"printReattach": {notCovered,
		"the re-attach block: workflow id and last status"},
	"printSubmitResult": {"TestTabwriterRenderersCannotBeForged",
		"the submit result's id and status — the receipt for money already spent"},
	"printCostMap": {"TestTabwriterRenderersCannotBeForged",
		"the cost-map KEYS on the quote screen — server-named factors beside Buzz amounts, written " +
			"into the PRE-SPEND table through a tabwriter its caller owns"},
	"classifyGenerateError": {notCovered,
		"the server's own error message, shown verbatim when generation is refused"},
	"buildGenerateGraph": {notCovered,
		"the uploaded image URL echoed back into the img2img disclosure"},
	"waitAndCollect": {notCovered,
		"the terminal-status and dead-end lines naming the workflow and its status"},
	"substitutionRefusal": {notCovered,
		"the server's reason inside the refusal that ABORTS a spend"},
	"joinQuoted": {notCovered,
		"the quoted key list in an --input parse error"},
	"downloadBlobTo": {notCovered,
		"the `Saved <target>` line on the generate output path"},
	"downloadOutputs": {notCovered,
		"the multi-output progress line and the no-URL error, both naming server ids"},
	"printAppMetrics": {"TestTabwriterRenderersCannotBeForged",
		"the scope and endpoint tokens — kept RAW on purpose (AGENTS.md item 8), which makes the " +
			"strip the ONLY thing between an uploader-shaped token and the terminal — plus the window " +
			"timestamps and granularity, which had no gate at all until #552"},
	"newUsersGetCmd": {notCovered,
		"the `closest matches` usernames printed when a user lookup misses"},
	"(*quietPollReporter).tick": {notCovered,
		"the server status echoed on every poll of a running generation"},
	"(*quietPollReporter).finish": {notCovered,
		"the final status line of a quiet poll"},
	"(*ttyPollReporter).tick": {notCovered,
		"the same status inside a \\r-rewritten spinner line, where a cursor escape is worth most"},
	"(*ttyPollReporter).finish": {notCovered,
		"the spinner's final status line"},
}

const (
	// minSafeTermFuncsScanned is the POSITIVE CONTROL on the attribution. A walk
	// that attributed nothing would find no unledgered function and report a
	// serene pass; 60 functions hold a call today.
	minSafeTermFuncsScanned = 40

	// minTestDeclsScanned is the POSITIVE CONTROL on the name resolver. An empty
	// declaration set would report every named test as a ghost — the same red for
	// the opposite cause. There are ~1580 today.
	minTestDeclsScanned = 500

	// maxUncoveredSafeTermFuncs is the RATCHET, and it is asserted as an
	// EQUALITY, not as a ceiling.
	//
	// 🔴 A CEILING SILENTLY ACQUIRES HEADROOM, WHICH IS THE ONE FAILURE A
	// RATCHET EXISTS TO PREVENT. `uncovered > max` alone is satisfied by
	// covering a row and leaving the constant where it was; the next commit may
	// then add a brand-new safeTerm-calling surface with a notCovered row and
	// stay green, spending headroom nobody knew was there. Measured on this
	// file: cover one row at cap 25 -> green at 24, then add a new uncovered
	// function -> green at 25, and a new unguarded server-text surface has
	// landed with no signal. So BOTH directions are checked below: too many is
	// the ratchet slipping, too few is unbanked progress that must be spent by
	// lowering this constant in the same commit.
	//
	// Say what it still does NOT do, because "ratchet" would otherwise
	// oversell it: it constrains the COUNT, so a commit that covers one row and
	// uncovers another nets out and passes. GREW is what makes a new surface
	// impossible to add SILENTLY — it must get a row, and whether that row is
	// honest is then a review question with the evidence written next to it.
	// That is the split #399 asked for: the count is mechanical, the content is
	// reviewed.
	// 🔴 LOWERED 25 -> 22 BY civitai/cli#552, WHICH IS THE RATCHET WORKING AS
	// DESIGNED. printAppMetrics, printCostMap and printSubmitResult moved to
	// covered when TestTabwriterRenderersCannotBeForged started driving them
	// with a forgery payload — measured by deleting each one's gate and watching
	// that test go red, not by reading it. The three renderers #552 added to this
	// ledger (printSubmissionTable, printSubmissionDetail, printListingStatus)
	// arrive COVERED by the same test, so they do not spend headroom either.
	maxUncoveredSafeTermFuncs = 22
)

// TestSafeTermCallSitesAreCoveredByANamedTest is civitai/cli#399.
//
// Four independent assertions, each with its own message, so a failure says
// which thing went wrong rather than that "the ledger is out of date":
//
//	GREW    — a function now calls safeTerm and no row covers it.
//	SHRANK  — a row names a function that no longer calls safeTerm.
//	GHOST   — a row names a test that does not exist.
//	RATCHET — the notCovered count is not EXACTLY maxUncoveredSafeTermFuncs:
//	          above it the ratchet slipped, below it there is unbanked headroom
//	          a later commit could spend in silence.
func TestSafeTermCallSitesAreCoveredByANamedTest(t *testing.T) {
	sites := scanSafeTermCallSites(t)

	calls := map[string]int{}
	firstPos := map[string]string{}
	for _, s := range sites {
		calls[s.enclosing]++
		if _, seen := firstPos[s.enclosing]; !seen {
			firstPos[s.enclosing] = s.pos
		}
	}
	if len(calls) < minSafeTermFuncsScanned {
		t.Fatalf("CONTROL failure, not a finding: %d safeTerm call(s) attributed to only %d function(s), "+
			"want >= %d. The enclosing-function attribution is broken, and every verdict below would be "+
			"about nothing.", len(sites), len(calls), minSafeTermFuncsScanned)
	}

	// --- GREW ---------------------------------------------------------------
	var unledgered []string
	for fn, n := range calls {
		if _, ok := safeTermCoveredBy[fn]; !ok {
			unledgered = append(unledgered, fmt.Sprintf("%s — %d call(s), first at %s", fn, n, firstPos[fn]))
		}
	}
	sort.Strings(unledgered)
	if len(unledgered) > 0 {
		t.Errorf("%d function(s) put SERVER text through safeTerm with no row in safeTermCoveredBy:\n  %s\n\n"+
			"Add a row. Either name the test that goes RED when you delete the safeTerm call there — delete it "+
			"and watch, do not read the test and believe it — or write notCovered and say in `why` what a "+
			"hostile field could do at that surface. notCovered is an honest row, but it counts against "+
			"maxUncoveredSafeTermFuncs (%d), which does not go up (civitai/cli#399).",
			len(unledgered), strings.Join(unledgered, "\n  "), maxUncoveredSafeTermFuncs)
	}

	// --- SHRANK -------------------------------------------------------------
	var stale []string
	for fn, cov := range safeTermCoveredBy {
		if calls[fn] == 0 {
			stale = append(stale, fmt.Sprintf("%s (recorded as %s)", fn, describeCoverage(cov)))
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("%d ledger row(s) name a function that no longer calls safeTerm:\n  %s\n\n"+
			"RENAMED or MOVED: move the row with it, in the same commit. DELETED: delete the row. "+
			"CALL REMOVED: that is the #399 defect happening — a server-text surface lost its only gate, and "+
			"a stale row would have gone on reading as coverage.", len(stale), strings.Join(stale, "\n  "))
	}

	// --- GHOST --------------------------------------------------------------
	checkCoveringTestsResolve(t)

	// --- RATCHET ------------------------------------------------------------
	covered, uncovered := 0, 0
	var missing []string
	for fn, cov := range safeTermCoveredBy {
		if cov.why == "" {
			t.Errorf("safeTermCoveredBy[%q] has an empty `why`. A row with no reason is a row nobody thought "+
				"about; say what the test asserts, or what a hostile field could do there.", fn)
		}
		if cov.test == notCovered {
			uncovered++
			missing = append(missing, fn)
			continue
		}
		covered++
	}
	sort.Strings(missing)
	switch {
	case uncovered > maxUncoveredSafeTermFuncs:
		t.Errorf("%d function(s) are ledgered notCovered, and the cap is %d:\n  %s\n\n"+
			"This number only goes down. Cover one by adding a case to TestReadRenderersStripTheInvisibleClass "+
			"— drive the renderer with hostileField and assert the paired predicate it uses (the class is gone "+
			"AND every named field arrived) — then lower maxUncoveredSafeTermFuncs to match.",
			uncovered, maxUncoveredSafeTermFuncs, strings.Join(missing, "\n  "))
	case uncovered < maxUncoveredSafeTermFuncs:
		t.Errorf("RATCHET HEADROOM: %d function(s) are ledgered notCovered but maxUncoveredSafeTermFuncs is %d. "+
			"Lower it to %d in this commit.\n\nThis is not a nit. A cap above the real count is headroom a LATER "+
			"commit can spend in silence: it adds a new safeTerm-calling surface, writes it a notCovered row, and "+
			"the count climbs back to the stale cap while the suite stays green — a new unguarded server-text "+
			"surface with no signal. The ratchet is an EQUALITY so progress is banked the moment it is made.",
			uncovered, maxUncoveredSafeTermFuncs, uncovered)
	}

	t.Logf("%d safeTerm call site(s) in %d function(s): %d function(s) COVERED by a named test, "+
		"%d NOT COVERED (cap %d)", len(sites), len(calls), covered, uncovered, maxUncoveredSafeTermFuncs)
	if len(missing) > 0 {
		t.Logf("NOT COVERED: %s", strings.Join(missing, ", "))
	}
}

// describeCoverage renders a row for a failure message.
func describeCoverage(c safeTermCoverage) string {
	if c.test == notCovered {
		return "notCovered"
	}
	return "covered by " + c.test
}

// checkCoveringTestsResolve requires every named covering test to be a test
// function DECLARED in this package.
//
// 🔴 THIS IS WHAT SEPARATES A LEDGER FROM A LIST OF WORDS, and the pattern is
// lifted from the module root's saferune_callers_ledger_test.go, where the
// same check had to be added after a rename left two prose statements naming a
// function that no longer existed while the suite stayed green.
//
// It resolves top-level funcs named Test… taking one parameter, which is what
// every covering test is. A row naming a SUBTEST (`TestX/sub`) does not resolve
// and is meant not to: a subtest name is not a declaration, and `-run` filters
// are not a contract.
func checkCoveringTestsResolve(t *testing.T) {
	t.Helper()
	decls := packageTestDecls(t)
	if len(decls) < minTestDeclsScanned {
		t.Fatalf("CONTROL failure, not a finding: resolved only %d test declaration(s) in this package, "+
			"want >= %d. The resolver is not reading the tests, and it would report every named test as "+
			"a ghost.", len(decls), minTestDeclsScanned)
	}
	var ghosts []string
	for fn, cov := range safeTermCoveredBy {
		if cov.test == notCovered || decls[cov.test] {
			continue
		}
		ghosts = append(ghosts, fmt.Sprintf("%s -> %s", fn, cov.test))
	}
	sort.Strings(ghosts)
	if len(ghosts) > 0 {
		t.Errorf("%d ledger row(s) name a test that this package does not declare:\n  %s\n\n"+
			"RENAMED: move the new name here in the same commit. DELETED: the surface lost its only pin — "+
			"either restore it or change the row to notCovered and lower nothing. A row naming a test that "+
			"does not exist reads as coverage and stops anyone looking, which is worse than no row at all.",
			len(ghosts), strings.Join(ghosts, "\n  "))
	}
}

// packageTestDecls returns the set of top-level `func TestXxx(t *testing.T)`
// names declared in this package's _test.go files.
func packageTestDecls(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, perr)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !strings.HasPrefix(fd.Name.Name, "Test") {
				continue
			}
			if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
				continue
			}
			out[fd.Name.Name] = true
		}
	}
	return out
}
