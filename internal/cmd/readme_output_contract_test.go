package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/pkg/civitai"
)

// This file covers the user-typed echo promise that used to live in README's
// `## Global flags` and is now published at
// developer.civitai.com/site/guide/cli-output. It was published UNQUALIFIED
// ("echoed byte-for-byte and deliberately not rewritten") while
// `internal/saferune` documents two exceptions and this package's own ledger
// MEASURES two more (`--root`, `--for-base`).
//
// 🔴 NEITHER HALF OF THIS FILE MOVED WITH THE SECTION, AND THE REASON DIFFERS.
// The BAN was always over the whole document, so it still guards README.md
// against the retracted sentence reappearing anywhere — on this surface it is now
// an INVARIANT guard rather than a regression one, since the prose it was red
// against is gone; it is kept because restoring that sentence would be just as
// wrong today. The BEHAVIOURAL test never read the README at all. What DID have
// to be rebuilt is the ban's control — see the test's own comment.
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

	// 🔴 THE BAN SURVIVED THE CUT; ITS CONTROL DID NOT — AND THAT IS THE EXACT
	// SHAPE OF A GREEN-VACUOUS GUARD. Leg 1 has always read the WHOLE document,
	// so deleting `## Global flags` (now a pointer to
	// developer.civitai.com/site/guide/cli-output) did not touch it. But the
	// control that proved the ban was reading anything real was a lookup of THAT
	// section, and it would have gone on passing only because it fatals — a
	// maintainer who "fixed" it by deleting the lookup would have been left with
	// a ban nothing proves can fire.
	//
	// So the control is replaced with a STRICTLY STRONGER pair, and neither half
	// depends on any section existing:
	//
	//  1. POSITIVE CONTROL — the ban is shown to FIRE, on a synthetic document
	//     carrying the retracted sentence. The old control only proxied this by
	//     checking that some text was found; this measures the thing itself.
	//  2. NEGATIVE CONTROL — the ban is shown NOT to fire on the sentence that
	//     merely QUOTES the claim in order to retract it, which is prose the
	//     document is allowed to contain.
	//
	// Both were previously covered only indirectly, via
	// TestReadmeAssertsFlagWinsRulePredicate on the sibling predicate.
	const bait = "and the value you typed " + retractedEchoClaim + " by the CLI"
	if asserted, _ := assertsUnquotedClaim(flattenWS(bait), retractedEchoClaim); !asserted {
		t.Fatalf("CONTROL failure: assertsUnquotedClaim did not fire on a document that plainly ASSERTS "+
			"the retracted claim, so a green verdict below would be a fact about the checker rather "+
			"than about README.md.\nbait: %s", bait)
	}
	const quoted = "the retracted sentence was `" + retractedEchoClaim + "` and it is wrong"
	if asserted, ctx := assertsUnquotedClaim(flattenWS(quoted), retractedEchoClaim); asserted {
		t.Fatalf("CONTROL failure: assertsUnquotedClaim fired on a MENTION of the claim (backticked, in a "+
			"sentence retracting it). It would forbid the prose the document wants, and a guard that "+
			"fails on correct input is one somebody deletes.\ncontext: %s", ctx)
	}
	// And the document under test must be big enough to be the README at all.
	if len(md) < 50_000 {
		t.Fatalf("CONTROL failure: README.md is only %d bytes — readREADME is reading the wrong file, and "+
			"the ban below would be a verdict about nothing", len(md))
	}

	// --- Leg 1: the retraction, over the WHOLE document. ---
	// Restoring the sentence anywhere else would be just as wrong, so the ban is
	// not scoped to any section — which is what let it survive the deletion of the
	// one it used to be controlled against.
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
// 🔴 THE `--for-base` SUBTEST WAS DELETED ONCE AND IS BACK, because "a
// pre-existing test also reddens" was the WRONG test of redundancy here. Removing
// the gate at reportBaseModel leaves exactly one red module-wide,
// TestSafeTermIsNeverAppliedToUserTypedInput, and its message is:
//
//	bareIdentArgs classifies "reportBaseModel::w", which is no longer a bare
//	safeTerm argument in that function. A stale note reads as coverage; DELETE IT,
//	or fix the key if the function was renamed.
//
// The first remedy it offers is to delete the LEDGER ROW. Follow it and the suite
// is green with the gate gone and the README's `--for-base` claim false. A guard
// whose only co-killer instructs the maintainer to remove the coverage is not a
// co-killer — so this subtest's kill IS unique in the only sense that matters.
// Measured both ways; the matrix is in the PR body.
//
// TestBaseModelWarningMentionsBothModels cannot substitute: it calls
// baseModelWarning directly with ASCII fixtures and never reaches this gate.
//
// 🔴 WHERE THE PROBE RUNE SITS, which cost a false measurement to learn:
// baseModelFamily classifies by prefix/substring over the RAW label, so an
// invisible rune INSIDE a token ("SD<U+2800>1.5") defeats the classifier — the
// family resolves to "", baseModelMismatch is conservatively false, and
// reportBaseModel prints NO warning at all. A probe placed there measures nothing
// while looking like a pass. Append the rune instead. That silence is the
// conservative direction the classifier documents, not a defect.
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
		// 🔴 THIS IS A FINDING, NOT A CONTROL FAILURE, and it used to be labelled the
		// other way. A note that EXISTS but no longer names `--root` is precisely the
		// mutation this subtest uniquely kills: the README's sentence becomes false
		// and every structural ledger stays green. Labelled "CONTROL failure, not a
		// finding" it told CI's reader to go fix the test — the opposite of the
		// remedy. The empty-note case above stays a control, because an absent note
		// means the FIXTURE stopped reaching the routing path.
		if !strings.Contains(note, root) {
			t.Fatalf("FINDING: routeDir emits a note but no longer interpolates the user's --root, so the "+
				"line whose job is to say WHERE the file goes does not report it.\n  note: %q\n\n"+
				"The published output contract (developer.civitai.com/site/guide/cli-output) states that "+
				"`civitai download` filters the value given to --root in the very line reporting it. With "+
				"--root absent from that line the claim is unfalsifiable and should be removed from that "+
				"page — do not relax this assertion.", note)
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
			t.Errorf("the plan's note echoed --root byte-for-byte. The published output contract says "+
				"download FILTERS --root; if that stopped being true, that page's sentence naming "+
				"--root is wrong.\n%s", plan)
		}
		if !strings.Contains(plan, "my"+"models") {
			t.Errorf("the plan's note did not render --root with the invisible rune removed, so it is neither "+
				"verbatim nor filtered — re-derive what this path does:\n%s", plan)
		}
	})

	t.Run("--for-base, in the mismatch warning", func(t *testing.T) {
		// Appended, not embedded — see the header: a rune inside the token defeats
		// baseModelFamily and produces no warning at all.
		forBase := "SDXL 1.0" + invisible
		const have = "SD 1.5"

		// CONTROL: these two bases really are a confident mismatch, so a warning
		// exists to inspect at all.
		w := baseModelWarning(have, forBase)
		if w == "" {
			t.Fatalf("CONTROL failure, not a finding: baseModelWarning(%q, %q) is empty, so reportBaseModel "+
				"prints nothing and this subtest asserts nothing. The classifier stopped seeing these two "+
				"as a confident mismatch — pick fixtures that still are.", have, forBase)
		}
		// FINDING, by the same argument as the --root case above: a warning that no
		// longer names --for-base makes the README's claim unfalsifiable.
		if !strings.Contains(w, forBase) {
			t.Fatalf("FINDING: baseModelWarning no longer interpolates the user's --for-base, so no line "+
				"reports it.\n  warning: %q\n\nThe published output contract names --for-base as a value "+
				"download filters; "+
				"with it absent that claim should be removed rather than this assertion relaxed.", w)
		}

		var out, errb bytes.Buffer
		reportBaseModel(&out, &errb, have, forBase)
		warn := errb.String()
		if warn == "" {
			t.Fatal("CONTROL failure, not a finding: reportBaseModel wrote nothing to stderr")
		}
		if strings.Contains(warn, forBase) {
			t.Errorf("the mismatch warning echoed --for-base BYTE-FOR-BYTE, invisible rune included. This "+
				"is the gate at reportBaseModel being gone.\n  warning: %q\n\nIf a ledger row was deleted "+
				"to make TestSafeTermIsNeverAppliedToUserTypedInput green, that was the wrong remedy — "+
				"restore the safeTermSingle call, not the row.", warn)
		}
		if !strings.Contains(warn, "SDXL 1.0\"") {
			t.Errorf("the warning did not render --for-base with the invisible rune removed, so it is "+
				"neither verbatim nor filtered:\n  warning: %q", warn)
		}
	})
}
