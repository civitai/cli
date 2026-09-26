package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/pkg/civitai"
)

// This file covers the user-typed echo promise in README's `## Global flags`,
// which was published UNQUALIFIED ("echoed byte-for-byte and deliberately not
// rewritten") while `internal/saferune` documents two exceptions and this
// package's own ledger MEASURES two more (`--root`, `--for-base`).
//
// It holds exactly two things: a BAN on the retracted sentence, and a
// BEHAVIOURAL test that the two extra measured cases really are filtered. Both
// are falsifiable. What this file deliberately does NOT hold is any check that
// the repaired prose is present — see the ban's comment for the measurement that
// removed those, and why keyword pinning fails in the worse direction.
//
// 🔴 A COMPANION GUARD FOR THE COLOUR-PRECEDENCE PROSE WAS WRITTEN AND DELETED,
// and it should not be rewritten from scratch. It derived the number of published
// precedence rungs from `strings.Count(resolveMode, "return mode")`. MEASURED:
// adding an ordinary `if o.Writer == nil { return modeAuto }` guard clause to
// resolveMode made it demand a FIFTH user-facing precedence tier in the README —
// "Add the rung; do not adjust this count." — i.e. it instructed a maintainer to
// publish a tier that does not exist. Returns are not tiers, and `resolveMode`
// exposes no machine-readable tier list to count instead. Its presence-check legs
// were walkable the same way this file's were, and its ordering leg is already
// killed by internal/ui's TestTermDumbOffIsNotVacuous. So the colour prose ships
// with no document guard, on purpose: the behaviour is guarded in internal/ui,
// and no available document guard here was both sound and non-redundant.

// assertsUnquotedClaim reports whether `flat` — a flattenWS-normalised document —
// STATES `claim` rather than quoting it in order to retract it.
//
// 🔴 The mention exemption is the same structural rule readmeAssertsFlagWinsRule
// uses, and it reuses that function's mentionDelims rather than a second copy:
// two tables of quotation characters that disagree is how one of them starts
// admitting a claim the other rejects. See that function's comment for why the
// ban must be scoped at all — a bare substring ban forbids the sentence
// retracting the claim, which is prose the document wants.
func assertsUnquotedClaim(flat, claim string) (bool, string) {
	re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(claim))
	for _, loc := range re.FindAllStringIndex(flat, -1) {
		before, _ := utf8.DecodeLastRuneInString(flat[:loc[0]])
		after, _ := utf8.DecodeRuneInString(flat[loc[1]:])
		if mentionDelims[before] && mentionDelims[after] {
			continue // quoted: the document is retracting the claim, not making it
		}
		lo := max(0, loc[0]-90)
		hi := min(len(flat), loc[1]+90)
		return true, strings.ToValidUTF8(flat[lo:hi], "")
	}
	return false, ""
}

// retractedEchoClaim is the sentence the README published for months: an
// UNQUALIFIED promise that what the user typed is never rewritten. It is wrong
// for at least four values — `--input` file content, `download`'s target path
// (including under `--out`), `--root` and `--for-base` — and it is the sentence a
// reader acts on when deciding whether a path on screen is the real path.
const retractedEchoClaim = "is echoed byte-for-byte and is deliberately *not* rewritten"

// TestREADMEDoesNotAssertTheUnqualifiedEchoPromise bans ONE retracted sentence
// over the whole document. That is all it does, and the narrowing is the point.
//
// 🔴 IT USED TO ALSO CHECK THAT THE REPAIRED PROSE WAS PRESENT, AND THOSE LEGS
// WERE MEASURED WORTHLESS. All 1,031 bytes of the repaired paragraph were
// replaced with one keyword-stuffed line carrying every literal the legs demanded
// and ending "Nothing above is a promise about anything; every path is printed
// raw." — the contract inverted — and the guard returned ok. So did the entire
// repository: nothing anywhere caught it. A presence check over prose is
// satisfied by a document that contains the words and states the opposite, which
// is green-while-false, and a guard that reads as coverage while providing none is
// worse than no guard because it stops anyone looking.
//
// A BAN is structurally different and is why this leg survives: it fires on the
// exact bytes that were published, and filler cannot satisfy it — filler can only
// avoid it, which is the direction that is safe to be wrong in. Verified red
// against the README at origin/main.
//
// 🔴 WHAT IS THEREFORE NOT GUARDED: that the README still SAYS anything correct
// about the split. Nothing mechanical can hold prose to a claim it does not
// compute; the behavioural half of the claim is pinned instead by
// TestDownloadFiltersRootAndForBase below, which exercises the two measured cases
// the retracted sentence denied. Do not "restore coverage" here with more
// literals.
func TestREADMEDoesNotAssertTheUnqualifiedEchoPromise(t *testing.T) {
	md := readREADME(t)
	section := readmeSectionByAnchor(t, md, "global-flags")
	flat := flattenWS(section)
	// CONTROL: the whole-README ban below must be able to see text, and the
	// section must be the one carrying the gate's promises.
	if !strings.Contains(flat, "what a table cell can contain") && !strings.Contains(flat, "Terminal escapes are removed") {
		t.Fatalf("CONTROL failure: the extracted `## Global flags` section does not contain the terminal-gate "+
			"promises, so this guard is reading the wrong block (%d bytes)", len(flat))
	}

	// --- Leg 1: the retraction, over the WHOLE document. ---
	// Restoring the sentence anywhere else would be just as wrong, so the ban is
	// not scoped to the section.
	if asserted, ctx := assertsUnquotedClaim(flattenWS(md), retractedEchoClaim); asserted {
		t.Errorf("the README ASSERTS the retracted echo promise — %q — here:\n  …%s…\n\n"+
			"internal/saferune documents two exceptions to it and internal/cmd's own ledger "+
			"(safeterm_userinput_test.go, rows printDownloadPlan::note, downloadSelected::note and "+
			"reportBaseModel::w) MEASURES two more. An unqualified promise tells a reader a path on screen "+
			"is the path on disk when it may not be.", retractedEchoClaim, ctx)
	}

}

// TestDownloadFiltersRootAndForBase is the BEHAVIOURAL half of the README's
// positive claim that `civitai download` rewrites the user's own `--root` bytes.
// safeterm_userinput_test.go's ledger recorded that as MEASURED — in a row that
// first said SERVER and was corrected — but nothing in the tree ran it.
//
// The probe rune is U+2800 BRAILLE PATTERN BLANK: it is in saferune's class, it is
// NOT `Cf` (so staticcheck's ST1018 is blind to it), and it occupies a cell — so a
// path carrying it renders as a DIFFERENT path with no visible difference.
//
// 🔴 WHAT IT UNIQUELY KILLS, AND WHAT IT DOES NOT. Measured against the whole
// internal/cmd suite. UNIQUE: routeDir's note stops naming `--root` at all — the
// line whose job is to say WHERE the file goes no longer reports the user's value,
// so the README's sentence silently becomes false while every structural
// call-site ledger stays green, because the sanitiser call is untouched. NOT
// unique: deleting the safeTermSingle call (TestSafeTermIsNeverAppliedToUserTypedInput
// also reddens) or removing U+2800 from saferune's class (ten other tests redden).
//
// 🔴 A `--for-base` SUBTEST WAS HERE AND WAS DELETED for having no unique kill:
// the analogous mutant is caught by TestBaseModelWarningMentionsBothModels, and
// the call-site removal by the structural ledger. Its one durable finding is kept
// here because it cost a false measurement to learn: baseModelFamily classifies by
// prefix/substring over the RAW label, so an invisible rune INSIDE a token
// ("SD<U+2800>1.5") defeats the classifier — the family resolves to "",
// baseModelMismatch is conservatively false, and reportBaseModel prints NO warning
// at all. A probe placed there measures nothing while looking like a pass; append
// the rune instead. That silence is the conservative direction the classifier
// documents, not a defect.
func TestDownloadFiltersRootAndForBase(t *testing.T) {
	const invisible = "⠀"

	t.Run("--root, in the routing note", func(t *testing.T) {
		root := "my" + invisible + "models"
		f := civitai.ModelVersionFile{
			Name: "weights.safetensors",
			Type: "SomethingUnrouted", // no layout folder → routeDir emits the note
		}
		o := &downloadOpts{layout: "a1111", root: root, modelType: "SomethingUnrouted"}

		// CONTROL: the note really is produced, and really does carry the user's
		// --root — otherwise the assertion below passes by absence.
		_, note := routeDir(o.layout, o.root, f.Type, o.modelType, f.Name)
		if note == "" {
			t.Fatal("CONTROL failure, not a finding: routeDir emitted no note, so no line reports --root " +
				"and this subtest asserts nothing")
		}
		if !strings.Contains(note, root) {
			t.Fatalf("CONTROL failure, not a finding: routeDir's note does not interpolate the user's "+
				"--root at all:\n  %q", note)
		}

		var out bytes.Buffer
		if err := printDownloadPlan(&out, []civitai.ModelVersionFile{f}, o, "https://civitai.com"); err != nil {
			t.Fatalf("printDownloadPlan: %v", err)
		}
		plan := out.String()
		if !strings.Contains(plan, "note:") {
			t.Fatalf("CONTROL failure, not a finding: the plan printed no note line:\n%s", plan)
		}
		if strings.Contains(plan, root) {
			t.Errorf("the plan's note echoed --root byte-for-byte. The README now says download FILTERS "+
				"--root; if that stopped being true, the README's sentence naming --root is wrong.\n%s", plan)
		}
		if !strings.Contains(plan, "my"+"models") {
			t.Errorf("the plan's note did not render --root with the invisible rune removed, so it is neither "+
				"verbatim nor filtered — re-derive what this path does:\n%s", plan)
		}
	})
}
