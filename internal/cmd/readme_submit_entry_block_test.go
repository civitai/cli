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
// with what the README is obliged to say about it.
//
// 🔴 THIS IS A LEDGER, NOT A COUNT. Membership is the assertion: adding a
// sentinel without a row here is red, and so is deleting one. A count would let
// add-one/delete-one swap a case silently — the exact defeat
// symptomAttributionsFloor was converted away from in #605.
type entryBlockSentinel struct {
	name string // the identifier inside errors.Is(err, …)
	// readmeSays is text the paragraph MUST contain while this case exists.
	// Empty means the case is deliberately not named in the README.
	readmeSays string
	why        string
}

var entryBlockSentinels = []entryBlockSentinel{
	{
		name:       "ErrBundleTooLarge",
		readmeSays: "The ceiling refusal above is the one refusal with an entry list of its own",
		why: "it takes the FIRST case and diverts to printSubmitSizeRefusal, so it is an " +
			"exception to the past-tense block rather than an instance of it. Round 0 of #635 " +
			"found the README claiming the block printed here",
	},
	{
		name:       "ErrUnauthorized",
		readmeSays: "except a `401`/`403`",
		why:        "401 and 403 both map to this sentinel in pkg/civitai/errkind.go, and both suppress the block",
	},
	{
		name:       "ErrRateLimited",
		readmeSays: "or a `429`",
		why:        "429 maps to this sentinel, and it suppresses the block",
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
// which entry block prints, bounded by the two printer names so it cannot drift
// onto an unrelated switch.
func doUploadSwitch(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("app_submit.go"))
	if err != nil {
		t.Fatalf("CONTROL failure: cannot read app_submit.go: %v", err)
	}
	src := string(b)
	// Bound it at the refusal printer's call site and close at the following
	// `return err`, which is the switch's own terminator.
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

var errorsIsRe = regexp.MustCompile(`errors\.Is\(err,\s*(?:appapi|civitai)\.(\w+)\)`)

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
				"The README currently tells readers %q. If the branch is genuinely gone, delete that "+
				"claim from BOTH the paragraph and the Troubleshooting row in the same commit, then "+
				"drop this row.", name, s.why, s.readmeSays)
		}
	}

	// And the README must actually carry what each live sentinel obliges.
	para := collapseWS(readmeEntryBlockParagraph(t))
	for _, s := range entryBlockSentinels {
		if !found[s.name] || s.readmeSays == "" {
			continue
		}
		if !strings.Contains(para, s.readmeSays) {
			t.Errorf("doUpload branches on %s, but the entry-table paragraph no longer says %q.\n"+
				"Why that matters: %s.\n"+
				"Do not satisfy this by pasting the string back — check the paragraph still describes "+
				"the branch correctly, then update the expectation here if the wording moved.",
				s.name, s.readmeSays, s.why)
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
		claim:    "R1 — the block is about the UPLOAD CALL, not about any submit failure",
		want:     "That block prints under **any error the upload call reports**",
		verifyAt: "internal/cmd/app_submit.go — the switch is inside the `if err != nil` for client.SubmitVersion, so nine earlier `return err` sites never reach it",
	},
	{
		claim:    "R1b — the pre-upload refusals print nothing, named so a reader can tell which case they are in",
		want:     "A refusal that stops the submit **before** the upload step prints nothing at all: no `--yes`, a dirty work tree, the version guard, a validation failure.",
		verifyAt: "internal/cmd/app_submit.go — those four all return before doUpload",
	},
	{
		claim:    "R3 — keyed on CLASSIFICATION, not on whether bytes left the machine",
		want:     "It is keyed on how the error *classifies*, not on whether bytes left the machine",
		verifyAt: "internal/appapi/appblocks.go — authedDoWith returns on Tokens.Token() BEFORE doOnceWith builds a request, and internal/auth/source.go returns that error untagged; measured in issue #637 as 0 requests received with the past-tense block printed",
	},
	{
		claim:    "R3b — and the past tense is therefore an account, not a guarantee",
		want:     "read `sent` as the CLI's account and not a guarantee",
		verifyAt: "issue #637 — the CODE fix that would let this claim be strengthened again",
	},
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
			"It is too short to carry these claims — the locator is probably reading the wrong "+
			"region, not the paragraph having been trimmed.", len(para))
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

// TestREADMEEntryBlockSurfacesAgree is R4: the two surfaces stating this
// contract must not drift apart again.
//
// 🔴 THIS IS THE ONE THAT WAS ACTUALLY BROKEN BY A FIX. Round 0's repair moved
// the paragraph and left the Troubleshooting row carrying the wording it had
// just called wrong. AGENTS.md names the class: the command section, the
// exit-code table and the Troubleshooting index each state the contract and each
// goes stale ALONE.
func TestREADMEEntryBlockSurfacesAgree(t *testing.T) {
	para := collapseWS(readmeEntryBlockParagraph(t))
	cause := collapseWS(troubleshootingEntryBlockCause(t))

	// Both surfaces must key on classification, and neither may key on bytes.
	for _, s := range []struct{ where, text string }{
		{"the `## Submit & auth` paragraph", para},
		{"the Troubleshooting row's cause cell", cause},
	} {
		if !strings.Contains(s.text, "classifies") {
			t.Errorf("%s no longer says the branch is keyed on how the error *classifies*.\n"+
				"That word is load-bearing: doUpload switches on errors.Is, never on a wire fact, "+
				"and a bytes-shaped claim here was measured false in issue #637 (0 requests received, "+
				"past-tense block printed). Round 2 of #635 introduced exactly that regression while "+
				"fixing round 1's.\n"+
				"🔴 IF YOU ARE HERE BECAUSE YOU FIXED #637: that is the one change this guard must NOT "+
				"block. Once printSubmitSizeDiagnosis can no longer print for a pre-contact error, the "+
				"stronger wording becomes TRUE and both surfaces should say so. Rewrite this expectation "+
				"in the same commit as the code fix — do not keep the weakened sentence to keep a test "+
				"green.", s.where)
		}
	}
	// The ceiling refusal must be named as the exception in the row's CAUSE
	// cell, or a reader who lands there gets a rule the paragraph contradicts.
	//
	// 🔴 THIS ASSERTION WAS VACUOUS AND SHIPPED AS LIVE — round 0 of #639 caught
	// it. It ran against the WHOLE table line, whose left-hand symptom column
	// literally reads `What this CLI would have sent`, so strings.Contains was
	// satisfied no matter what the cause cell said. Worse, that symptom column
	// is frozen independently: TestREADMETroubleshootingSymptomsExistInTheSource
	// requires it to exist verbatim in non-test source, and app_submit.go emits
	// it — so the string could never leave the line and the check could never
	// fail. A cause cell reading "The block never appears before an upload" —
	// verbatim what the error below calls false — passed.
	//
	// The original mutation missed it because it deleted BOTH words at once, so
	// the sibling assertion went red first and scored this one as killed.
	// RULES.md: isolate the mutation, and confirm the failure is THIS guard's.
	if !strings.Contains(cause, "would have sent") {
		t.Errorf("the Troubleshooting row's cause cell no longer names the `What this CLI would have sent` case.\n" +
			"It is the one pre-upload refusal that DOES print an entry list, so a cause cell that " +
			"omits it tells a reader the block never appears before an upload — which is false, " +
			"and was the round 0 finding on #635.\n" +
			"Note this asserts on the CAUSE cell only: the symptom column quotes the same string and " +
			"is frozen by TestREADMETroubleshootingSymptomsExistInTheSource, so matching the whole " +
			"row here would pass unconditionally.")
	}
}

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
		"so TestREADMEEntryBlockSurfacesAgree is asserting against an empty string. The row is on " +
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
// nonTTYSubmitRefusal drives `app submit` down the no-TTY, no-`--yes` path
// against a recorder server and returns its streams.
//
// 🔴 EXTRACTED, NOT COPIED — round 0 of #639 found ~30 lines of this setup
// duplicated verbatim between here and TestAppSubmit_NonTTYRefusesWithoutYes_
// NoNetworkCall, down to the literal token string. AGENTS.md: "One rule, one
// place." The two tests assert DIFFERENT things about the same path (that one:
// no network call; this one: no entry block), which is exactly the case a shared
// driver serves rather than a second copy.
//
// It fails the test itself if the path it claims to exercise was not the one
// taken, so a caller cannot assert about a refusal that never happened.
func nonTTYSubmitRefusal(t *testing.T) (stdout, stderr string, serverHit bool) {
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
	if err == nil {
		t.Fatalf("CONTROL failure: the bare non-TTY submit did not refuse, so the caller never "+
			"reached the path it is asserting about.\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
	if !strings.Contains(err.Error(), "refusing to submit without --yes") {
		t.Fatalf("CONTROL failure: refused, but not by the --yes gate — the caller is measuring a "+
			"different path than it claims: %v", err)
	}
	return out, errOut, hit
}

func TestREADMEPreUploadRefusalPrintsNoEntryBlock(t *testing.T) {
	_, stderr, hit := nonTTYSubmitRefusal(t)
	if hit {
		t.Error("the submit endpoint was hit on a pre-upload refusal")
	}

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
