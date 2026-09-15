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
// safeterm.go for what a hostile field does without it). It was called from 152
// sites in 60 functions when this file was written; at civitai/cli#575 R1 the
// harness reports 199 sites in 72. 🔴 THE FIGURES IN THIS HEADER ARE DATED, NOT
// LIVE — nothing checks them, and they are kept only because the #398 audit below
// is a claim about the tree it ran on. Read the harness's own log line for today's
// numbers. An adversarial audit of #398 deleted `safeTerm(...)`
// at each of 25 sampled sites in turn: 20 of them left the WHOLE SUITE GREEN.
// A refactor could drop the call and CI would say nothing.
//
// 🔴 THE PROBLEM WAS NOT A MISSING TEST, IT WAS A MISSING NUMBER. Nobody could
// answer "which of these sites is pinned?" without re-running that audit by
// hand, so the answer was never written down and never re-checked. A table of
// table of rows beats a test per site: this is the answer, in a form a compiler
// keeps honest.
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
//     for every row, notCovered ones included. That is the whole ledgered set —
//     60 functions when #399 measured it — and it is what its headline
//     measurement was about.
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
	// --- generate: the blob download path (civitai/cli#574) -----------------
	"blobStatusError": {"TestBlobStatusErrorCannotForgeALine",
		"generate's blob 401/403/404/default arms: a server-derived output name, gated once at the top like its twin downloadStatusError"},
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

	// --- the SECOND composition row (civitai/cli#605, civitai/cli#624) --------
	// safeTermBounded is safeTermSingle plus a LENGTH cap. Its own safeTerm call
	// is the delegation, not a render, so — like safeTermSingle's row above — the
	// named test is one that puts this function's OUTPUT on a screen.
	// 🔴 WATCHED, NOT READ: rewriting `s = safeTermSingle(s)` inside
	// safeTermBounded to `s = s` reddens TestProgressWriterLineSanitizesServerName
	// (3 subtests) and TestProgressLineCannotForgeALine's newline and tab
	// subtests. The LENGTH half of this function is pinned separately — by
	// TestProgressLineCannotForgeALine/soft-wrap and
	// TestGenerateBuzzBalanceWarningIsLengthBounded — because deleting the cap
	// leaves every strip assertion green, which is the whole reason #605 and #624
	// exist.
	"safeTermBounded": {"TestProgressWriterLineSanitizesServerName",
		"the shared LENGTH cap on a single-line server operand, applied at exactly two sites — " +
			"(*progressWriter).line and runGenerate's Buzz-balance warning — because those are the two " +
			"surfaces with a MEASURED soft-wrap forgery. It is not inside safeTermSingle on purpose: that " +
			"would truncate EVERY other safeTermSingle operand in this package — 148 call sites when " +
			"this was measured, against a draft of this row that said \"~16\" and was wrong by an order " +
			"of magnitude — several of which (a SHA256 mismatch, an `unusable filename %q` refusal) " +
			"exist to show the user exactly what the server sent"},

	// --- download path ------------------------------------------------------
	"printDownloadPlan": {"TestDownloadPlanSanitizesControlChars",
		"the plan a user reads before fetching: file name, sha256, target and notes. An escape here " +
			"can overwrite the `SHA256 verified` line the plan is there to establish"},
	"reportBaseModel": {"TestDownloadPlanSanitizesControlChars",
		"the base-model line and the mismatch warnings above the plan"},
	"formatFileList": {"TestReadRenderersStripTheInvisibleClass",
		"the ambiguity list naming each candidate file by server-supplied name and type"},

	// --- download path: the five surfaces civitai/cli#566 found ---------------
	//
	// 🔴 THESE FIVE ARE WHY THE LEDGER'S KEY IS A BLIND SPOT WORTH NAMING. GREW
	// only fires for a function that ALREADY calls safeTerm at least once, so
	// while these called it ZERO times no row was ever demanded — their absence
	// was not merely unrecorded, it was UNRECORDABLE. Gating them is what made
	// them ledgerable. The instrument that would have FOUND them (a row demanded
	// of any function rendering a server-supplied struct field, gated or not) is
	// deliberately still open; #566 scopes it out rather than half-building it.
	"checkTargetCollisions": {"TestCheckTargetCollisionsSanitizesServerFields",
		"the same-target refusal: each colliding file's name and type, plus the mixed-origin target. The " +
			"LITERAL TWIN of formatFileList's line, which was gated while this was not — and " +
			"this one is the ONLY thing between the user and a silent overwrite"},
	"(*progressWriter).line": {"TestProgressWriterLineSanitizesServerName",
		"the server's files[].name inside a \\r-REWRITTEN line — both callers rewrite in place (Write's TTY " +
			"branch 10×/s, done() once), so the CLI is already moving the cursor and an ESC in the name " +
			"extends that reach up into the pickle/archive EXECUTION WARNING printed just above it. " +
			"🔴 THE GATE IS safeTermBounded SINCE civitai/cli#605, AND THIS ROW ONCE CARRIED THE OPPOSITE " +
			"CLAIM: it said nothing about length while the surface was length-unbounded, so a name padded " +
			"to the terminal width SOFT-WRAPPED and stranded `(SHA256 verified)` at column zero with no " +
			"\\n, no \\t and no \\x1b — the one forgery on this path that misleads about whether the bytes " +
			"are the bytes, rendered before any bytes are verified. The named test covers the STRIP half " +
			"(watched red: rewriting safeTermBounded's delegation to `s = s` fails its three subtests); the " +
			"LENGTH half is covered by TestProgressLineCannotForgeALine/soft-wrap, which asserts the " +
			"DISPLAY-ROW COUNT at 80/100/120/132 and carries the issue's negative control. Two tests, two " +
			"claims — a row naming only the first would read as coverage of both"},
	"downloadOne": {"TestDownloadOneErrorsSanitizeTheServerName",
		"the download / SHA256-mismatch / install errors AND the `Saved <target>` line. main.go " +
			"prints err.Error() unfiltered, so the mismatch string is the CLI ASSERTING AN INTEGRITY " +
			"FAILURE with an uploader-controlled prefix. MEASURED, 6 calls, ALL SIX killed individually. " +
			"🔴 THE SIXTH — the safeTermErr on `download %s: %w` — was recorded here as a SURVIVOR " +
			"\"because the cause is a *url.Error and Go's own %q already escapes the class\". That " +
			"sentence was false and is retracted (#572 round 2): %q is strconv.Quote, which escapes what " +
			"is not unicode.IsPrint, and IsPrint admits U+2800 and U+034F; and url.URL.String() writes " +
			"RawQuery back VERBATIM, so a hostile query rides into *url.Error raw. 🔴 THAT RETRACTION'S " +
			"OWN THIRD CLAUSE — \"the https/parse refusals on this path are not *url.Error at all\" — is " +
			"retracted in turn (#572 round 4): only the http:// SCHEME refusal is not one (plain " +
			"fmt.Errorf, no %w, exitCode 1); the PARSE refusal wraps url.Parse's *url.Error{Op:\"parse\"} " +
			"with %w, so errors.As matches and transport_error.go classifies it exit 5. Two refusals, two " +
			"published exit codes — do not lump them. It survived for want of a driving test, " +
			"which the two `*url.Error carries the runes %q does not escape` / `real client's https " +
			"refusal` subtests now supply. The `create output directory` error is deliberately NOT here: " +
			"its `dir` is user-typed only (#572)"},
	"writePart": {"TestWritePartErrorsSanitizeTheServerName",
		"the frame BETWEEN downloadOne and the progress writer, gated so the sibling-renderer split #566 " +
			"is about cannot reappear one call frame down. MEASURED, 6 calls: `streaming <name>`, " +
			"`create <partPath>` and its cause are killed; the two `finalize` calls and `streaming`'s " +
			"cause SURVIVE — a failing Close and a hostile-bytes stream error are not driven (#566 PR body)"},
	// 🔴 THE SIXTH, FOUND BY #572's AUDIT OF #566 — AND IT IS THE SAME BLIND SPOT
	// ONE LAYER DEEPER. targetPath also called safeTerm zero times, so no row was
	// demandable; worse, its refusal LOOKED gated because it used `%q`. It is not:
	// strconv escapes what is not unicode.IsPrint, and IsPrint admits U+2800 (So)
	// and U+034F (Mn) — both in saferune's class, both emitted raw. Measured, not
	// argued, and the fixture that shows it (dlHostileQuoted) has its own control.
	"targetPath": {"TestTargetPathRefusalSanitizesTheServerName",
		"the unusable-filename refusal, which echoes files[].name with an ARBITRARY prefix — " +
			"filepath.Base(\"<payload>/..\") is \"..\", so the degenerate basename the guard tests for says " +
			"nothing about the bytes in front of it. Reaches BOTH streams: stderr via downloadSelected and " +
			"STDOUT via printDownloadPlan's `target: (unresolved)`, one line under a name that IS sanitised. " +
			"`%q` is kept alongside — it delimits a name that is often blank, and it escapes the \\n saferune " +
			"deliberately keeps",
	},
	"downloadStatusError": {"TestDownloadOneErrorsSanitizeTheServerName",
		"the four HTTP-status errors, gated ONCE at the top rather than at each return — four spellings of " +
			"one rule is how #566 happened. The 401 arm is the most exposed: an anonymous download of a " +
			"gated file reaches it on the FIRST run, and the message it forges tells the user to log in"},
	"safeTermErr": {"TestDownloadOneErrorsSanitizeTheServerName",
		"THE WRAPPED CAUSE, which #566's own first fix left raw: *fs.PathError and *os.LinkError render " +
			"their paths with no quoting, so `%s` sanitised + `%w` raw emitted the hostile bytes one colon " +
			"later. Pinned by the `install` subtest, writePart's `create`, and — since #572 round 2 — the " +
			"two download-URL subtests, which cover the case `%q` does NOT neutralise; errors.Is/As still " +
			"reach through"},
	"emitPreDownloadNotes": {"TestSafeTermIsNeverAppliedToUserTypedInput",
		"INCIDENTAL, NOT BEHAVIOURAL: the published file name in the `no SHA256 published` warning. " +
			"The red comes from bareIdentArgs noticing `name` stopped being passed, not from any " +
			"assertion about what reaches the terminal"},

	// --- generate path ------------------------------------------------------
	// 🔴 THIS ROW IS THE POINT OF civitai/cli#612, NOT A BY-PRODUCT OF IT. Until
	// it existed, runGenerate called safeTerm ZERO times — so GREW could never
	// demand a row for it, and the absence of a gate on the Buzz-balance warning
	// was not merely unrecorded, it was UNRECORDABLE. That is the same blind spot
	// the five download-path rows below are annotated with, reappearing on the
	// money-spending path. The instrument that FOUND it was not a grep for this
	// helper's name (that can only enumerate operands which already have a gate)
	// but an enumeration of every value interpolated into a writer or an
	// fmt.Errorf on generate.go and generate_wait.go, each traced to its origin.
	"runGenerate": {"TestGenerateBuzzBalanceWarningCannotForgeALine",
		"the Buzz-balance read FAILURE warning — appapi.GetBuzzAccount's error, whose text is " +
			"server-chosen on EVERY route (serverMessage ends `return strings.TrimSpace(string(raw))`, so a " +
			"body with no message/error key yields the WHOLE body; the non-envelope-200 path interpolates " +
			"string(raw) directly), and which had NO gate at all, so " +
			"raw ANSI passed through. It is printed directly above confirmGenerate's real `Cost: … Buzz` " +
			"line and `Generate? [y/N]:`, and the measured payload erased its own line and left a " +
			"counterfeit `Cost:` line the SERVER wrote on the last screen before an IRREVERSIBLE spend. " +
			"🔴 THE NAMED TEST ASSERTS GEOMETRY AND THE ESCAPE CLASS, NOT THE ABSENCE OF A WORD: one " +
			"stderr line opens AND closes the CLI's own sentence, the line count matches a benign render, " +
			"and no ESC byte survives. It DELIBERATELY ASSERTS NOTHING ABOUT TABS — this surface goes " +
			"through ui.Warn, and lipgloss expands \\t to four spaces before the bytes reach the writer, " +
			"so such a guard could never fire. Not reached under --dry-run. This row covers runGenerate's " +
			"ONE safeTerm call; the function's many other surfaces answer to their own callees' rows. " +
			"🔴 THE SENTENCE THAT STOOD HERE — \"THE GATE BOUNDS THE RUNE CLASS, NOT THE LENGTH — " +
			"RESIDUAL, MEASURED, STILL LIVE\" — IS RETRACTED (civitai/cli#624, taken with #605 because one " +
			"decision governs both). Its measurement stands and is why the fix exists: a soft wrap has no " +
			"line-break rune, so safeTermSingle could not see it, and a 5,120-char balance error rendered " +
			"as ONE logical line of 5,224 runes with zero ESC and zero TAB — PASSING EVERY ASSERTION THE " +
			"NAMED TEST MAKES — which an 80-column terminal laid out as ~66 rows mostly beginning at " +
			"column zero with a counterfeit `Cost:` line. The gate is safeTermBounded now, capping the " +
			"operand HERE rather than at one appapi arm, since it is uncapped on EVERY route (see the call " +
			"site). 🔴 THE NAMED TEST STILL DOES NOT ASSERT LENGTH, AND THAT IS DELIBERATE: " +
			"TestGenerateBuzzBalanceWarningIsLengthBounded does, on the same surface, by asserting the " +
			"DISPLAY-ROW COUNT at 80/100/120/132 — and it re-runs the named test's own predicates on the " +
			"5,120-char payload to demonstrate, in the same run, that they stay green. A guard that could " +
			"not see the defect is kept and labelled rather than widened, so the pair reads as two claims",
	},
	// 🔴 REPOINTED BY civitai/cli#575 R1, AND THE OLD NAME IS WHY THIS FIELD EXISTS.
	// This row read TestGenerate_SanitisesServerStrings, which was TRUE until this
	// same PR gave printGenerateQuote its own cell gate — that gate sanitises the
	// payload whatever describeVersion did, so the named test went green on a
	// broken upstream. MEASURED: deleting both calls here reddens 3 tests at
	// f0cb748 and, without this repoint, would have left the row naming one that
	// no longer moves. A row naming a test that cannot go red is the "resolves a
	// NAME, not a relationship" defect this PR deletes a ledger state for,
	// regenerated one file over.
	"describeVersion": {"TestDescribeVersionIsSanitisedAtItsOwnOutput",
		"the resolved checkpoint/LoRA name and type. It reaches a tabwriter CELL on the quote screen and " +
			"a plain label line on the approval screen, and BOTH now gate at their own render site — so the " +
			"named test asserts this function's RETURN VALUE, which no downstream gate can mask"},
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
	"presentTargetSatisfies": {notCovered,
		"the `already present (SHA256 verified)` lines, which assert an integrity result"},
	"pickleArchiveNote": {notCovered,
		"the pickle/archive EXECUTION WARNING, whose text embeds the server's file name"},
	"printOutputURLs": {"TestPrintOutputURLsRowCountIsNotServerChosen",
		"the --no-download URL listing, the one #604 surface on STDOUT. The named test drives the renderer " +
			"with four outputs — one of them URL-LESS and one of the rest hostile — and asserts the ROW COUNT " +
			"plus exactly two tab-separated fields per row, each numbered by the CLI. 🔴 THE INVARIANT IS ONE " +
			"ROW PER KEPT OUTPUT THAT HAS A URL, NUMBERED BY ITS INDEX IN kept, NOT `one per kept output` — " +
			"which is what this row and the assertion both said until #604 round 0, and it is FALSE: the " +
			"loop numbers by index but skips a nil URL, and genapi.Deliverable (Available && " +
			"!hasBlockedReason && !Hidden) does not require one, so a legitimate `url: null` in the middle " +
			"yields rows numbered 1 and 3 and the old assertion would have reported the CLI forging its own " +
			"output. Gaps are legitimate; server-chosen, duplicated or displaced numbers are not, which is " +
			"what still catches a URL carrying \\n2\\t<other>. The fixture's nil URL drives the gap case so " +
			"the widened assertion cannot pass vacuously. The TAB half is live here: this is bare " +
			"fmt.Fprintf, not ui-styled. The `These URLs are presigned` trailer on stderr carries no server " +
			"text and is not asserted"},
	"printSubmitted": {"TestPrintSubmittedCannotForgeALine",
		"the submitted-workflow receipt. The named test calls the renderer with cost nil and asserts the " +
			"block occupies the 2 lines the function writes, differentially against a benign id. 🔴 IT " +
			"DELIBERATELY ASSERTS NOTHING ABOUT TABS: both lines go through ui (Success/Dim) and lipgloss " +
			"expands \\t to four spaces before the bytes reach the writer (re-measured), so such a guard " +
			"could never fire. The `Charged N Buzz` line is not driven — cost is nil — and it carries a " +
			"number, not a server string"},
	"printReattach": {"TestPrintReattachBlockGeometryIsNotServerChosen",
		"the recovery block a user reads after a wait ended without a result. The named test calls the " +
			"renderer with a hostile id AND a hostile status and asserts the block is the 6 lines it writes " +
			"(⚠ header + 4 rows + trailer), that no TAB survives, and that each of the four labelled rows — " +
			"`Re-attach:` included — begins exactly ONE line. #604 measured two counterfeit `Re-attach:` " +
			"lines, the first ABOVE the genuine one. The `External ID:` row is the CLI's own value and is " +
			"deliberately ungated, so nothing here pins it"},
	"printSubmitResult": {"TestTabwriterRenderersCannotBeForged",
		"the submit result's id and status — the receipt for money already spent"},
	"printCostMap": {"TestTabwriterRenderersCannotBeForged",
		"the cost-map KEYS on the quote screen — server-named factors beside Buzz amounts, written " +
			"into the PRE-SPEND table through a tabwriter its caller owns"},
	"printGenerateQuote": {"TestTabwriterRenderersCannotBeForged",
		"the Checkpoint cell and each LoRA cell on the PRE-SPEND quote screen, both filled by " +
			"describeVersion, plus checkpointNote beside them. 🔴 checkpointNote is NOT from " +
			"describeVersion and an earlier draft of this row said it was: substitutionCheckpointNote " +
			"(generate_substitution.go) builds a CLI-owned literal around a %d, so it carries no server " +
			"text today — it is gated here because the next author to extend it to name the SUBSTITUTED " +
			"model would otherwise inherit a row claiming it was already handled. The cells are gated at " +
			"the cell, not upstream, because a ledger row naming an upstream function resolved only to a " +
			"NAME and was satisfiable by any unrelated sanitising renderer (civitai/cli#575 R1). The " +
			"named test drives this renderer with the fields set DIRECTLY, bypassing describeVersion, so " +
			"it goes red when the cell gate is deleted — watched, not read: deleting safeTermSingle from " +
			"the Checkpoint cell fails it on gqckpt and gqnote plus two column-zero findings"},
	"confirmGenerate": {"TestGatedRenderersDoNotForgeOutsideTheirTable",
		"the `Checkpoint:` and `LoRA:` label lines on the APPROVAL screen — plain Fprintf, four lines " +
			"above `Generate? [y/N]:`, so a `\\n` here forges the last thing a user reads before an " +
			"IRREVERSIBLE spend, Cost line included. They were ungated and safe only because " +
			"describeVersion gates on the way in; #575 R1's round 0 measured that once printGenerateQuote " +
			"had its own cell gate, deleting describeVersion's stopped reddening ANY behavioural test. " +
			"Watched red, not read: deleting the Checkpoint gate here fails the named test with two " +
			"column-zero findings. Everything the USER typed on this screen stays unsanitised on purpose " +
			"(civitai/cli#393) — this row is about the two SERVER-derived labels only"},
	"classifyGenerateError": {"TestClassifyGenerateErrorCannotForgeALine",
		"the server's own message, sanitised ONCE at the top and interpolated into the error each matching " +
			"arm rebuilds. The named test drives ALL FIVE matching arms through a real tRPC error server and " +
			"asserts one line each — tabs included, since these are plain fmt.Errorf with no ui styling in " +
			"front of them — with a control that the arm MATCHED rather than falling through. 🔴 THIS ROW " +
			"ENDED IN A DISCLAIMER — \"the fall-through returns the transport's own error unchanged, which " +
			"is not this function's surface\" — AND THAT SENTENCE IS RETRACTED (civitai/cli#612). It was " +
			"the thing that kept the defect invisible: the fall-through is not an edge, it is the DOMINANT " +
			"path (the set is whatever the five arms do not take, and TWO drafts that tried to restate it as " +
			"a status list or a message list were each false on a different axis — the arms above " +
			"classifyGenerateError's own comment are the only authority), the error it returned " +
			"embedded genapi.serverMessage(raw) with no strip of ANY kind, and main.go prints it as " +
			"`Error: <it>` — so raw ESC deleted that line and left a counterfeit `✓ Generation submitted` " +
			"banner in its place. A ledger row that scopes a surface OUT is a claim, and this one was " +
			"false. The fall-through is safeTermErr'd now and is driven by " +
			"TestClassifyGenerateErrorFallThroughCannotForgeALine, which uses a 503/429/500 — never one of " +
			"the five arms, since an arm-matching case passes without executing the defective path at all. " +
			"TestClassifyGenerateErrorFallThroughPreservesClassification pins that errors.Is/As still " +
			"resolve through the wrapper, so the published exit codes are untouched. 🔴 THE OTHER RETURN — " +
			"the !errors.As early exit — IS STILL UNGATED AND THAT IS NOT A CLAIM THAT IT IS SAFE: it is " +
			"MEASURED to carry raw ANSI, because genapi interpolates the unparsed HTTP body into " +
			"`unexpected %s response: %s` (civitai/cli#620). It is left alone because that same raw body " +
			"reaches SEVEN " +
			"genapi sites of which only some come back through here — so a gate here would close a subset " +
			"of one class — and because this return is a pass-through pinned by identity. Read the " +
			"comment at that return, not this sentence, before acting on it"},
	"buildGenerateGraph": {"TestImageDisclosureLineIsGatedAtComposition",
		"the uploaded image URL echoed back into the img2img disclosure line. It composes the USER's own " +
			"--image path (echoed byte-for-byte, civitai/cli#393) with the SERVER's blob URL, and only the " +
			"server half is gated — which is why the gate is HERE and not at printImageDisclosure, the one " +
			"place this PR did not move a gate to its render site (civitai/cli#575 R1). The named test drives " +
			"the real upload seam and is watched red both ways: deleting the gate, and flattening the whole " +
			"composed line (the #393 direction, which its ZWNJ-bearing fixture path is what makes visible)"},
	"waitAndCollect": {"TestWaitAndCollectReReadHintCannotForgeALine",
		"THIS FUNCTION'S OWN call sites, which is the scope this row has and the scope it keeps — a " +
			"surface reached through a CALLEE answers to that callee's row, and three successive drafts " +
			"that tried to enumerate the reachable set here were each incomplete in the reassuring " +
			"direction. The named test drives the `Output URLs expire — re-read the workflow for fresh " +
			"links` hint, printed after a transfer FAILED: civitai/cli#596 upgraded it to safeTermSingle " +
			"and shipped it UNPINNED, and round 2's delta audit reverted that one expression with the " +
			"whole package staying green. The other two surfaces are driven by " +
			"TestWaitAndCollectTerminalStatusErrorCannotForgeALine (the non-succeeded terminal-status " +
			"error, generate.go:1514) and TestWaitAndCollectNoDeliverablesErrorCannotForgeALine (the " +
			"succeeded-but-no-deliverables error, :1558) — civitai/cli#604, which moved all three off " +
			"plain safeTerm. 🔴 THE TERMINAL-STATUS ERROR HAS TWO OPERANDS AND ONLY ONE OF THEM IS " +
			"LINE-FORGEABLE, so they take two different assertions and neither is a claim about the " +
			"other: safeTermSingle(workflowID) renders through %s and assertOneLine sees a forged line " +
			"there, while safeTermSingle(wf.Status) renders through %q, which ESCAPES \\n and \\t — that " +
			"operand is pinned by assertNoControlEscapes instead, and its gate buys independence from " +
			"the verb rather than closing a reachable forgery (genapi.IsTerminalStatus TrimSpace-bounds " +
			"the value to a whitespace-padded known status, so the server cannot put words of its own in " +
			"it and still exit the poll loop)"},
	"substitutionRefusal": {notCovered,
		"the server's reason inside the refusal that ABORTS a spend"},
	"joinQuoted": {notCovered,
		"the quoted key list in an --input parse error"},
	"downloadBlobTo": {"TestDownloadBlobToErrorsCannotForgeALine",
		"generate's blob transfer — all four surfaces DRIVEN, not merely listed: the overwrite " +
			"refusal, both `%s: %w` pairs (operand AND wrapped " +
			"cause) and the `Saved` line, carrying the server-derived leaf and the mixed-origin " +
			"target (civitai/cli#574). Its `create output directory` line is deliberately UNGATED " +
			"and pinned the OTHER way by TestCreateOutputDirectoryStaysUngated, because that value " +
			"is the user's own --out-dir"},
	"downloadOutputs": {"TestDownloadOutputsErrorsCannotForgeALine",
		"all THREE of this function's error surfaces, one subtest each, each mutation-verified alone. " +
			"They do NOT carry the same value, and saying they did is what let two of them go " +
			"unmeasured: (a) the overwrite refusal names `j.target` — planOutputTarget's output, so the " +
			"server's {workflow} placeholder expands into the LEAF while the directory is the user's " +
			"own --out-dir, a MIXED origin no ledger in this package can key (see the selector-blind-spot " +
			"note beside bareIdentArgs); (b) the no-URL error names `o.ID`, the server's blob id, and no " +
			"path at all; (c) the duplicate-target hint renders the target through %q, which Go escapes, " +
			"so the workflow id is its only unquoted operand. 🔴 The overwrite refusal was FULLY RAW " +
			"until civitai/cli#574's round-0 audit — raw ESC, not merely an ungated newline — and it is " +
			"the branch users actually reach, since it pre-checks every job and leaves downloadBlobTo's " +
			"own refusal TOCTOU-only. 🔴 THIS ROW HAS NOW BEEN WRONG TWICE IN THE REASSURING DIRECTION: " +
			"it once named a progress line this function does not have, and civitai/cli#596 round 1 " +
			"rewrote it to claim all three surfaces as one target while the named test drove ONE — the " +
			"no-URL error was still on plain safeTerm and forging a line, and the duplicate hint's gate " +
			"could be reverted with no behavioural test going red. Both are driven now, and the test was " +
			"renamed off …RefusalCannotForgeALine because that name is what made a one-surface test read " +
			"as a three-surface one"},
	"printAppMetrics": {"TestTabwriterRenderersCannotBeForged",
		"the scope and endpoint tokens — kept RAW on purpose (AGENTS.md item 8), which makes the " +
			"strip the ONLY thing between an uploader-shaped token and the terminal — plus the window " +
			"timestamps and granularity, which had no gate at all until #552"},
	"newUsersGetCmd": {notCovered,
		"the `closest matches` usernames printed when a user lookup misses"},
	"(*quietPollReporter).tick": {"TestPollReportersCannotForgeALine",
		"TWO server operands on the retryable-error branch's single Fprintf — the status echoed on every " +
			"poll of a running generation, AND the failed check's own error, whose text is the server's " +
			"`message` verbatim (genapi's generateError interpolates it into every arm; APIError.Error() " +
			"returns it unchanged). 🔴 THE ERROR OPERAND WAS UNGATED, AND THIS ROW ONCE DESCRIBED THE " +
			"BRANCH AS IF IT WERE NOT: #604 round 0 measured a 500 whose message was " +
			"\"boom\\n  status succeeded\\tSaved out.png (2.0 MiB)\" printing THREE forged lines, each " +
			"opening `  status ` — the prefix (*quietPollReporter).finish writes for the line that reports " +
			"how a paid-for generation ended. The named test's `quiet` subtest drives a NON-TERMINAL status " +
			"through the real pollWorkflow — non-terminal because that is what a waiting generate displays " +
			"— and reaches BOTH of tick's gated branches: the no-error one on attempt 1 and the " +
			"retryable-error one on attempt 2 (a scripted 500 carrying that message). It asserts the line " +
			"count matches a benign render, that no TAB survives (bare fmt.Fprintf, so that half is live), " +
			"and — instead of the permitted-prefix SET it asserted first, which allowed `  status ` and so " +
			"could not tell a counterfeit from finish()'s own line — the SEQUENCE: waiting… lines, then " +
			"exactly one `  status ` line, LAST. tick's 429 branch renders no server text and is not driven"},
	"(*quietPollReporter).finish": {"TestPollReportersCannotForgeALine",
		"the final status line of a quiet poll. The same `quiet` subtest reaches it through a TIMEOUT, so " +
			"finish() is called with the last NON-TERMINAL status rather than a terminal one, and the same " +
			"line-count, row-prefix and TAB assertions cover it"},
	"(*ttyPollReporter).tick": {"TestPollReportersCannotForgeALine",
		"the same status inside a \\r-rewritten spinner line, where a cursor escape is worth most. The `tty` " +
			"subtest constructs this reporter directly — newPollReporter picks it only for a real TTY, so a " +
			"buffer-driven test never gets it otherwise — drives the same non-terminal status through " +
			"pollWorkflow, and asserts the whole run emits exactly ONE newline and no TAB, so a forged " +
			"newline cannot strand attacker text above the rewrite point. It also routes the SAME hostile " +
			"server error message down this path, which is what makes \"this tick drops e.err, so its " +
			"suffix is a fixed string\" a measured claim rather than one read off the source: rendering that " +
			"message here ungated is red in this subtest"},
	"(*ttyPollReporter).finish": {"TestPollReportersCannotForgeALine",
		"the spinner's final status line, reached by the same `tty` subtest through the timeout path with " +
			"the last non-terminal status; the one-newline and no-TAB assertions cover it"},
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
	// 🔴 LOWERED 25 -> 21 BY TWO INDEPENDENT EFFORTS THAT LANDED TOGETHER, AND
	// THE MERGED VALUE IS NEITHER SIDE'S. This is the ratchet working as
	// designed, twice over, on disjoint sets of functions:
	//
	//   25 -> 22, civitai/cli#552 (app path). printAppMetrics, printCostMap and
	//   printSubmitResult moved to covered when
	//   TestTabwriterRenderersCannotBeForged started driving them with a forgery
	//   payload — measured by deleting each one's gate and watching that test go
	//   red, not by reading it. The three renderers #552 added to this ledger
	//   (printSubmissionTable, printSubmissionDetail, printListingStatus) arrive
	//   COVERED by the same test, so they do not spend headroom either.
	//
	//   25 -> 24, civitai/cli#566 (download path). downloadOne moved from
	//   notCovered to covered, which is exactly the "unbanked progress that must
	//   be spent in the same commit" case the paragraph above describes. The
	//   SIX NEW rows the #566 work adds — checkTargetCollisions,
	//   (*progressWriter).line, downloadStatusError, safeTermErr, writePart and
	//   targetPath — are all covered, so they do not move this number in either
	//   direction.
	//   ⚠ THIS SAID "seven", AND IT WAS WRONG THE MOMENT IT WAS WRITTEN. The
	//   pre-merge text said "five" and was right for ITS tree; the merge
	//   resolution changed it to "seven" without re-measuring. Re-derived here by
	//   diffing this map's KEYS between refs rather than by counting prose: six
	//   against the merge base (which already carried #552/#573's three app-path
	//   rows), and nine against this branch's fork point c4ed077 — of which the
	//   same three arrived via main, not from this work. Neither reading is
	//   seven. A count in a comment is a claim; re-measure it, do not carry it
	//   through a merge.
	//
	// The two sets are disjoint — four distinct functions moved, so the merged
	// count WAS 21, not 22 and not 24. 🔴 THAT 21 WAS MEASURED, NOT DERIVED: taking
	// either branch's constant through the merge would have left the equality
	// asserting a number no tree ever held. It is what this test reported for
	// the MERGED tree, whose notCovered set it also enumerates on failure.
	//
	// 🔴 LOWERED 21 -> 20 BY civitai/cli#575 R1, AND THE PARAGRAPH ABOVE IS LEFT
	// IN THE PAST TENSE ON PURPOSE. That PR made buildGenerateGraph covered
	// (TestImageDisclosureLineIsGatedAtComposition reddens when its gate goes) and
	// banked the unit in the same commit, as the RATCHET HEADROOM message demands.
	// 20 is what this test reports for the current tree: 52 covered, 20 not.
	// 🔴 The three sentences above asserting 21 went on asserting it for one round
	// after the constant moved, directly above the retraction that says "a count in
	// a comment is a claim; re-measure it, do not carry it through a merge" — found
	// by an audit round, not by any check. A reader hitting a RATCHET failure reads
	// this paragraph as the authority on what the number should be, so it is
	// amended rather than search-and-replaced: each bullet now says which tree its
	// figure belonged to.
	// 🔴 LOWERED 18 -> 17 BY civitai/cli#596 ROUND 2, AND BANKED IN THE SAME
	// COMMIT, which is what the RATCHET HEADROOM paragraph above demands.
	// waitAndCollect moved from notCovered to covered when
	// TestWaitAndCollectReReadHintCannotForgeALine started driving its re-read
	// hint through the real command. MEASURED, not derived: this test reported
	// "RATCHET HEADROOM: 17 … but maxUncoveredSafeTermFuncs is 18" on the tree
	// that changed the row, and 17 is the number it printed. Its `why` states
	// which ONE of that function's four surfaces is driven and which three are
	// not — a row does not become a claim about every call inside it.
	// 🔴 LOWERED 17 -> 9 BY civitai/cli#604, BANKED IN THE SAME COMMIT. Eight
	// functions moved from notCovered to covered at once — printOutputURLs,
	// printSubmitted, printReattach, classifyGenerateError and all four poll
	// reporter methods — because #604 is one CLASS (server text on a generate
	// surface, rendered through plain safeTerm, which keeps \n and \t) and
	// gating a subset of a class is the mechanism that regenerates the defect at
	// the ungated members. 9 is what this test PRINTED for the tree this commit
	// ships ("RATCHET HEADROOM: 9 … but maxUncoveredSafeTermFuncs is 17"), not a
	// number derived by subtracting eight from the old one. Each of those rows
	// says which surfaces its named test drives and which it does not; four of
	// them also say which assertions are deliberately absent because ui's
	// lipgloss styles expand \t before the bytes reach the writer, so a tab
	// guard on a styled surface could not fire.
	maxUncoveredSafeTermFuncs = 9
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
