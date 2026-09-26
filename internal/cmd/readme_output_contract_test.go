package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/pkg/civitai"
)

// This file guards two published claims in `## Global flags` that had NO guard
// at all, and each was measurably wrong when it was written:
//
//   - the colour precedence list published THREE tiers while resolveMode has
//     FOUR — `TERM` appeared zero times in the whole README — and said nothing
//     about auto being resolved per writer;
//   - the user-typed echo promise was published unqualified ("echoed
//     byte-for-byte and deliberately not rewritten") while `internal/saferune`
//     documents two exceptions and this package's own ledger MEASURES two more.
//
// Both are pinned to the code that decides them rather than to a fixed sentence,
// because a sentence-shaped pin is walkable by rewording — and, in the second
// case, with a behavioural leg, because the README now makes a positive claim
// about `--root` and `--for-base` that has to be TRUE.

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

// TestAssertsUnquotedClaimPredicate validates the instrument before either guard
// below reads its verdict, in BOTH directions — the predicate it is modelled on
// was wrong in both.
func TestAssertsUnquotedClaimPredicate(t *testing.T) {
	const claim = "is echoed byte-for-byte"
	for _, tc := range []struct {
		name  string
		in    string
		asrts bool
	}{
		{"absent", "the paths you name are echoed exactly on the confirmation screen", false},
		{"quoted retraction", "the old \"is echoed byte-for-byte\" wording was unqualified", false},
		{"backticked retraction", "we dropped the `is echoed byte-for-byte` promise", false},
		{"bare assertion", "what you typed is echoed byte-for-byte and never rewritten", true},
		{"capitalised", "What you typed Is Echoed Byte-For-Byte here", true},
		{"one-sided quote", "we said \"is echoed byte-for-byte and meant it", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := assertsUnquotedClaim(flattenWS(tc.in), claim)
			if got != tc.asrts {
				t.Errorf("assertsUnquotedClaim(%q) = %v, want %v", tc.in, got, tc.asrts)
			}
		})
	}
}

// resolveModeSource returns the body of internal/ui/resolveMode, which is the
// ONE place the colour precedence is decided.
func resolveModeSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRootDir(t), "internal", "ui", "ui.go"))
	if err != nil {
		t.Fatalf("read internal/ui/ui.go: %v", err)
	}
	s := string(src)
	i := strings.Index(s, "func resolveMode(o Options) colorMode {")
	if i < 0 {
		t.Fatalf("internal/ui/ui.go no longer declares resolveMode(o Options) colorMode. The README's " +
			"colour-precedence list is a claim about that function; find where the decision moved and " +
			"re-point this guard at it rather than deleting the guard.")
	}
	body := s[i:]
	if j := strings.Index(body, "\n}\n"); j >= 0 {
		body = body[:j]
	}
	return body
}

// TestREADMEColorPrecedenceListHasEveryTier ties the README's numbered colour
// precedence list to resolveMode, which is where the order actually lives.
//
// 🔴 THE COUNT IS DERIVED FROM THE CODE, NOT ASSERTED AS A LITERAL. resolveMode
// answers with one `return` per tier, so the number of rungs the README must
// publish is the number of returns that function has. A tier added to the
// resolver without a rung in the README reddens here — which is exactly the
// defect this guard was written for: `TERM=dumb` was tier 3 in the code and
// appeared ZERO times in the whole README, so the published list was a
// three-tier claim about a four-tier resolver.
//
// The per-writer sentence is on the same list because it is the same rung: tier
// 4 is EnabledFor's question about one stream, not a process-wide answer, and a
// reader who takes it as process-wide is wrong about what a piped stdout and a
// TTY stderr do in one run.
func TestREADMEColorPrecedenceListHasEveryTier(t *testing.T) {
	body := resolveModeSource(t)

	// --- Leg 1: the code, and the ORDER within it. ---
	tiers := []struct{ name, needle string }{
		{"force off", `if o.NoColor || envSet("NO_COLOR") {`},
		{"force on", `if o.ForceColor || envTrue("CLICOLOR_FORCE") {`},
		{"TERM=dumb", `if os.Getenv("TERM") == "dumb" {`},
	}
	at := make([]int, len(tiers))
	for i, tier := range tiers {
		at[i] = strings.Index(body, tier.needle)
		if at[i] < 0 {
			t.Fatalf("resolveMode no longer contains the %s tier (%s). The README publishes the precedence "+
				"as a numbered list derived from this function; re-derive both.", tier.name, tier.needle)
		}
	}
	for i := 1; i < len(at); i++ {
		if at[i] < at[i-1] {
			t.Errorf("resolveMode tests the %s tier BEFORE the %s tier. The README publishes them in the "+
				"opposite order, and states that force-on beats TERM=dumb — an ordering that holds only "+
				"because the force-on branch returns first. Re-derive the README, or restore the order.",
				tiers[i].name, tiers[i-1].name)
		}
	}
	wantRungs := strings.Count(body, "return mode")
	// CONTROL: a resolver with one or zero answers would make the rung count
	// below meaningless.
	if wantRungs < 3 {
		t.Fatalf("CONTROL failure: resolveMode has only %d `return mode` statement(s) — the rung count this "+
			"guard derives from it cannot be right", wantRungs)
	}

	// --- Leg 2: the document. ---
	md := readREADME(t)
	section := readmeSectionByAnchor(t, md, "global-flags")
	if len(strings.TrimSpace(section)) < 800 {
		t.Fatalf("CONTROL failure: the README's `## Global flags` section is only %d byte(s) long — the "+
			"extractor is reading the wrong block", len(strings.TrimSpace(section)))
	}
	rungRe := regexp.MustCompile(`(?m)^(\d+)\. `)
	var rungs []int
	for _, m := range rungRe.FindAllStringSubmatch(section, -1) {
		n, _ := strconv.Atoi(m[1])
		rungs = append(rungs, n)
	}
	if len(rungs) != wantRungs {
		t.Errorf("the README's `## Global flags` section publishes %d precedence rung(s) (%v) but resolveMode "+
			"decides with %d. A tier that exists in the resolver and not in the list is a setting a reader "+
			"cannot find: `TERM=dumb` was tier 3 for months and appeared ZERO times in this document. "+
			"Add the rung; do not adjust this count.", len(rungs), rungs, wantRungs)
	}
	for i, n := range rungs {
		if n != i+1 {
			t.Errorf("the precedence list's rung %d is numbered %d — the list is the published ORDER, so a "+
				"gap or repeat in the numbering changes which tier a reader thinks wins", i+1, n)
		}
	}

	flat := flattenWS(section)
	for _, want := range []struct{ claim, why string }{
		{"`TERM=dumb`", "the third tier of resolveMode, and the one the published list omitted entirely"},
		{"per writer", "tier 4 is resolved for EACH stream (ui.EnabledFor), not once per process"},
	} {
		if !strings.Contains(flat, want.claim) {
			t.Errorf("the README's `## Global flags` section does not state %s — %s.", want.claim, want.why)
		}
	}
	// The ordering claim itself, because the list alone does not say which way a
	// COMBINATION resolves, and this is the combination the resolver's block
	// order decides. internal/ui's TestTermDumbOffIsNotVacuous is the behavioural
	// half of the same claim.
	for _, want := range []string{"CLICOLOR_FORCE=1", "still styles"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the README's `## Global flags` section does not state %q. Force-on sits ABOVE "+
				"TERM=dumb, and a reader on a dumb terminal has no other way to learn that `--color` "+
				"still works there.", want)
		}
	}
}

// retractedEchoClaim is the sentence the README published for months: an
// UNQUALIFIED promise that what the user typed is never rewritten. It is wrong
// for at least four values — `--input` file content, `download`'s target path
// (including under `--out`), `--root` and `--for-base` — and it is the sentence a
// reader acts on when deciding whether a path on screen is the real path.
const retractedEchoClaim = "is echoed byte-for-byte and is deliberately *not* rewritten"

// TestREADMEUserTypedEchoSplitsByValue pins the repaired promise.
//
// 🔴 THE SHAPE OF THE REPAIR IS LOAD-BEARING, AND FOUR EARLIER DRAFTS FAILED THE
// SAME WAY: each claimed a sharper scope than the code supports — an unqualified
// universal, then "two documented exceptions" as a CLOSED set, then an
// attribution to one screen, then a false claim about which screen echoes a
// particular flag. The wording that survives splits the promise by VALUE, not by
// screen, and names nothing as complete. So this guard asserts three things, and
// the third is the one the earlier drafts would have failed:
//
//  1. the retracted unqualified sentence is GONE (and may be quoted only to
//     retract it);
//  2. the split is stated — the values that never go through the gate, and the
//     paths that do on some screens;
//  3. the set is NOT presented as closed, and the two extra measured cases are
//     named. A "two documented exceptions" phrasing is a closed-set claim and
//     reddens here.
func TestREADMEUserTypedEchoSplitsByValue(t *testing.T) {
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

	// --- Leg 2: the split, by value. ---
	for _, want := range []struct{ claim, why string }{
		{"splits by **value**", "the promise is per-value, not per-screen; four drafts got that wrong"},
		{"`--ecosystem`", "one of the values that never goes through the gate, beside the two prompts"},
		{"confirmation screen before a spend", "the screen where a path IS exact, and why"},
		{"`--input`", "the first documented exception: a graph FILE is not what the user typed"},
		{"`--out`", "the second: download filters the target path even when the user set it"},
	} {
		if !strings.Contains(flat, want.claim) {
			t.Errorf("the README's user-typed echo promise does not state %s — %s.", want.claim, want.why)
		}
	}

	// --- Leg 3: the set is OPEN, and the extra measured cases are named. ---
	for _, want := range []string{"`--root`", "`--for-base`"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the README does not name %s as a user-typed value that IS filtered. It is MEASURED "+
				"(see TestDownloadFiltersRootAndForBase in this package), and naming it is what stops the "+
				"two documented exceptions being read as the whole set.", want)
		}
	}
	// 🔴 THE BAN NAMES CLOSED-SET PHRASINGS OF *THIS* SET, NOT THE WORD
	// "exhaustive". Measured: banning that word outright reddens on the section's
	// OWN correct sentence about a different list — "The list above is
	// **illustrative, not exhaustive**" — i.e. the ban would forbid a document
	// saying exactly what this guard wants it to say, three bullets down. A
	// keyword ban over a section that discusses several sets cannot tell which set
	// a word is about; the phrasings below can only be about this one.
	for _, closed := range []string{
		"two documented exceptions", "the only two", "these two exceptions are the only",
		"the two exceptions are", "only these two values",
	} {
		if strings.Contains(strings.ToLower(flat), strings.ToLower(closed)) {
			t.Errorf("the README presents the filtered set as CLOSED (%q). It is not: nothing enumerates it, "+
				"and the second draft of this sentence failed for exactly this reason. State the two "+
				"documented cases and say the set has no boundary.", closed)
		}
	}
	if !strings.Contains(flat, "do not read those two as its boundary") {
		t.Error("the README does not tell the reader the two documented cases are not the boundary of the " +
			"set. Without that sentence a reader takes the enumeration as complete, which is the second " +
			"failed draft of this promise.")
	}
}

// TestDownloadFiltersRootAndForBase is the BEHAVIOURAL half of leg 3 above: the
// README now makes a positive claim that `civitai download` rewrites the user's
// own `--root` and `--for-base` bytes, and a claim in a document that no test
// exercises is a claim about nothing.
//
// 🔴 MEASURED AT THE PRODUCTION PRINT SITES, not by composing safeTermSingle
// here. safeterm_userinput_test.go's ledger records these two as MEASURED —
// twice with a row that first said SERVER and was wrong — but nothing in the tree
// ran them. This does.
//
// The probe rune is U+2800 BRAILLE PATTERN BLANK: it is in saferune's class, it is
// NOT `Cf` (so staticcheck's ST1018 is blind to it), and it occupies a cell — so a
// path carrying it renders as a DIFFERENT path with no visible difference.
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
				"--root; if that stopped being true, the README's leg-3 sentence is wrong.\n%s", plan)
		}
		if !strings.Contains(plan, "my"+"models") {
			t.Errorf("the plan's note did not render --root with the invisible rune removed, so it is neither "+
				"verbatim nor filtered — re-derive what this path does:\n%s", plan)
		}
	})

	// 🔴 WHERE THE PROBE RUNE SITS MATTERS HERE, AND THE OBVIOUS PLACEMENT MEASURES
	// NOTHING. baseModelFamily classifies by prefix/substring over the RAW label,
	// so an invisible rune INSIDE a token ("SD<U+2800>1.5") defeats the classifier:
	// the family comes back "", baseModelMismatch is conservatively false, and
	// reportBaseModel prints no warning at all — a silent pass that reads as "the
	// value was echoed verbatim". Measured. The rune is therefore appended, where
	// classification still succeeds and the warning exists to inspect.
	//
	// That is also a real, if benign, property of the command: a `--for-base` with
	// an invisible rune mid-token simply does not warn. It is the conservative
	// direction the classifier documents ("never warns on unknown"), so it is
	// recorded here rather than treated as a defect.
	t.Run("--for-base, in the mismatch warning", func(t *testing.T) {
		forBase := "SDXL 1.0" + invisible
		const have = "SD 1.5"

		// CONTROL: these two bases really are a confident mismatch, so a warning
		// exists to inspect.
		w := baseModelWarning(have, forBase)
		if w == "" {
			t.Fatalf("CONTROL failure, not a finding: baseModelWarning(%q, %q) is empty, so reportBaseModel "+
				"prints nothing and this subtest asserts nothing", have, forBase)
		}
		if !strings.Contains(w, forBase) {
			t.Fatalf("CONTROL failure, not a finding: the warning does not interpolate the user's "+
				"--for-base:\n  %q", w)
		}

		var out, errb bytes.Buffer
		reportBaseModel(&out, &errb, have, forBase)
		warn := errb.String()
		if warn == "" {
			t.Fatal("CONTROL failure, not a finding: reportBaseModel wrote nothing to stderr")
		}
		if strings.Contains(warn, forBase) {
			t.Errorf("the mismatch warning echoed --for-base byte-for-byte. The README now says download "+
				"FILTERS --for-base:\n  %q", warn)
		}
		if !strings.Contains(warn, "SDXL 1.0\"") {
			t.Errorf("the warning did not render --for-base with the invisible rune removed, so it is "+
				"neither verbatim nor filtered:\n  %q", warn)
		}
	})
}
