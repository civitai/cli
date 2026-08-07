package validate

// readyack_gaps_test.go covers issue #258: the presence tier used to GUESS at
// why it had fallen back, and the guess was wrong in the commonest case.
//
// 🔴 THE DEFECT WAS THE MESSAGE, NOT THE TIERING. AGENTS.md item 20 records, as
// a deliberate trade, that a reference to a file which is not there is a GAP
// rather than a decided absence — so the canonical #206 shape (a `static`
// scaffold whose `civitai-host.js` was deleted) is CORRECTLY checked on presence
// only. What was wrong is that the advisory then offered "there is no index.html
// at the project root, or it holds a reference this CLI cannot follow — a
// bundler alias, a generated file, an off-project URL", none of which is true of
// a five-file no-build app whose index.html plainly references a file that has
// been deleted. The resolver had recorded exactly that in `EntryGraph.Gaps` and
// the check threw it away.
//
// So every case here asserts on the GAP REPORT — the section spliced between the
// two fixed halves of the presence message — never on the whole message. A
// `strings.Contains(msg, "index.html")` is vacuous: the shared remedy names
// index.html twice in every tier, and would pass for a message that still
// guessed.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/scaffold"
)

// gapReportFor drives the real pipeline and returns the presence tier's gap
// report, failing if the project did not reach that tier.
func gapReportFor(t *testing.T, dir string) string {
	t.Helper()
	res := wantAckKind(t, dir, "presence-only")
	for _, w := range res.Warnings {
		if isPresenceOnlyAdvice(w.Message) {
			return presenceOnlyGapReport(t, w.Message)
		}
	}
	t.Fatalf("no presence-tier advisory in %v", res.Warnings)
	return ""
}

// wantGapReport asserts the report names every literal in want, and fails
// naming the ones it does not.
func wantGapReport(t *testing.T, report string, want ...string) {
	t.Helper()
	if report == "" {
		t.Fatalf("the presence advisory carries NO gap report — the resolver's reasons were discarded, "+
			"which is issue #258 exactly; wanted %q", want)
	}
	for _, w := range want {
		if !strings.Contains(report, w) {
			t.Errorf("the gap report does not name %q — an author cannot act on a reason that omits it\ngot: %s", w, report)
		}
	}
}

// ---------------------------------------------------------------------------
// THE DEFECT, REPRODUCED — both SDK-free templates.
// ---------------------------------------------------------------------------

// TestDanglingEmitterReferenceIsNamed is issue #258 as filed.
//
// 🔴 THE POSITIVE CONTROL AGAINST THE OLD TEXT IS THE ABSENCE ASSERTION, not the
// presence one. A report that named the file AND still said "a bundler alias" is
// the message that shipped; the fix is only real if the guess is gone.
func TestDanglingEmitterReferenceIsNamed(t *testing.T) {
	t.Run("static: index.html references the deleted emitter", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.Static)
		wantAckKind(t, dir, "") // the untouched scaffold is silent
		if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		report := gapReportFor(t, dir)
		wantGapReport(t, report,
			"index.html",                     // WHERE the reference lives
			"./"+blockproto.ReadyAckFilename, // the specifier as the author wrote it
			"does not exist",                 // WHY the resolver stopped
			"fix the reference",              // what to do about it
		)
	})

	t.Run("page-vite: the entry module imports the deleted emitter", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.PageVite)
		wantAckKind(t, dir, "")
		if err := os.Remove(filepath.Join(dir, "src", blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		// The referencing file here is the entry MODULE, not index.html — which
		// is the point of running this template too: the report has to name the
		// file the author must edit, and for page-vite that is src/main.jsx.
		wantGapReport(t, gapReportFor(t, dir),
			"src/main.jsx",
			"./"+blockproto.ReadyAckFilename,
			"does not exist",
		)
	})
}

// TestPresenceAdviceNoLongerSpeculates is the OTHER half of issue #258, and it
// has to read the WHOLE emitted message rather than the gap report.
//
// 🔴 A REPORT-SCOPED ABSENCE CHECK CANNOT SEE THE DEFECT, AND A MUTATION PROVED
// IT. The first version of this assertion lived inside
// TestDanglingEmitterReferenceIsNamed and asked only whether the GAP REPORT
// speculates. Restoring the shipped guess to `readyAckAdvicePresenceOnlyHead` —
// i.e. adding the true reasons and keeping the false ones, the most likely way
// this regresses — reddened NOTHING: 0 failures across internal/validate and
// internal/cmd. Naming the real cause is only half the fix; the speculation has
// to be gone, and "gone" is a claim about the message a user reads.
//
// The fixture is chosen so every phrase below is PROVABLY false of it: a
// rendered `static` scaffold is five files with no build step, so it has no
// bundler and therefore no alias, nothing generates a file, every reference is
// local, and index.html is right there at the root. A message that offers any of
// them is manufacturing advice, which is the failure AGENTS.md item 10 spent
// four measured corrections avoiding.
func TestPresenceAdviceNoLongerSpeculates(t *testing.T) {
	dir := renderTemplate(t, scaffold.Static)
	if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
		t.Fatal(err)
	}
	res := wantAckKind(t, dir, "presence-only")

	msg := ""
	for _, w := range res.Warnings {
		if isPresenceOnlyAdvice(w.Message) {
			msg = w.Message
		}
	}
	if msg == "" {
		t.Fatal("no presence-tier message to inspect")
	}
	// The positive control: the message DOES name the real cause. Without it,
	// "says none of the wrong things" is satisfied by a message that says nothing.
	if !strings.Contains(msg, "which does not exist") {
		t.Fatalf("the advisory does not name the real cause, so the absence assertions below are vacuous:\n%s", msg)
	}
	// Quoted from the wording that shipped, so a revert is caught verbatim.
	for _, gone := range []string{
		"a bundler alias",
		"a generated file",
		"an off-project URL",
		"there is no index.html at the project root",
	} {
		if strings.Contains(msg, gone) {
			t.Errorf("the advisory still offers %q at a five-file no-build scaffold where it is impossible — "+
				"naming the real cause does not close #258 while the speculation is still printed alongside "+
				"it:\n%s", gone, msg)
		}
	}
}

// ---------------------------------------------------------------------------
// THE GENERAL CASE. The fix surfaces `EntryGraph.Gaps` wholesale rather than
// special-casing the dangling reference, so the other gap kinds must arrive too
// — that is what distinguishes "issue #258 is closed" from "one message got a
// better sentence".
// ---------------------------------------------------------------------------

func TestEveryGapKindReachesTheAuthor(t *testing.T) {
	t.Run("a bare specifier that is not a declared dependency", func(t *testing.T) {
		// `import '@/civitai-host.js'` is a real file behind a resolve.alias this
		// CLI does not read. Here the guess the old message made was RIGHT — and
		// it has to keep being said, from the resolver, naming the specifier.
		dir := ackProject(t, ackManifest(false), map[string]string{
			"package.json": `{"dependencies": {}}`,
			"index.html":   `<!doctype html><script type="module" src="./main.js"></script>`,
			"main.js":      `import '@/civitai-host.js';` + "\n",
		})
		wantGapReport(t, gapReportFor(t, dir),
			"main.js",           // the file holding the reference
			"@/civitai-host.js", // the specifier
			"bundler alias",     // the reason, now earned rather than guessed
		)
	})

	t.Run("a file the resolver could not read", func(t *testing.T) {
		// A stylesheet over the per-file size cap. It has to be a NON-source
		// extension: the whole-tree scan would hit the same cap on a `.js` and
		// report UNOBSERVABLE, which gates both tiers and emits nothing at all.
		dir := ackProject(t, ackManifest(false), map[string]string{
			"index.html": `<!doctype html><script type="module" src="./main.js"></script>`,
			"main.js":    `import './huge.css';` + "\n",
		})
		big := strings.Repeat("/* pad */\n.a{color:red}\n", (maxAckFileBytes/22)+64)
		if err := os.WriteFile(filepath.Join(dir, "huge.css"), []byte(big), 0o600); err != nil {
			t.Fatal(err)
		}
		wantGapReport(t, gapReportFor(t, dir), "could not read", "huge.css")
	})

	t.Run("no index.html at the project root", func(t *testing.T) {
		dir := ackProject(t, ackManifest(true), map[string]string{
			"package.json": `{"dependencies": {"next": "^15.0.0"}}`,
			"app/page.tsx": `export default function Page() { return null; }`,
		})
		wantGapReport(t, gapReportFor(t, dir), "index.html")
	})

	t.Run("an off-project URL in a script src", func(t *testing.T) {
		dir := ackProject(t, ackManifest(false), map[string]string{
			"index.html": `<!doctype html><script src="https://cdn.example.com/x.js"></script>` +
				`<script src="./app.js"></script>`,
			"app.js": `document.title = 'hi';` + "\n",
		})
		wantGapReport(t, gapReportFor(t, dir), "https://cdn.example.com/x.js", "could not resolve")
	})
}

// TestGapReportIsCappedAndSaysSo pins the truncation disclosure.
//
// 🔴 A SILENTLY TRUNCATED LIST READS AS "THAT WAS ALL OF THEM" — the same class
// of lie as the guess this report replaced. An author fixes the three references
// they were shown, re-runs, and is told about three more that were there the
// whole time.
func TestGapReportIsCappedAndSaysSo(t *testing.T) {
	const refs = readyAckGapCap + 4
	var tags strings.Builder
	for i := 0; i < refs; i++ {
		fmt.Fprintf(&tags, `<script src="./missing%d.js"></script>`, i)
	}
	dir := ackProject(t, ackManifest(false), map[string]string{
		"index.html": `<!doctype html>` + tags.String(),
	})
	report := gapReportFor(t, dir)

	// Exactly readyAckGapCap reasons are numbered, and the numbering stops there.
	for i := 1; i <= readyAckGapCap; i++ {
		if !strings.Contains(report, fmt.Sprintf("(%d)", i)) {
			t.Errorf("the gap report is missing reason (%d):\n%s", i, report)
		}
	}
	if strings.Contains(report, fmt.Sprintf("(%d)", readyAckGapCap+1)) {
		t.Errorf("the gap report rendered more than readyAckGapCap=%d reasons:\n%s", readyAckGapCap, report)
	}
	// And the overflow is COUNTED, not merely hinted at. `refs` references all
	// gap, so the count is exact.
	want := fmt.Sprintf("and %d more", refs-readyAckGapCap)
	if !strings.Contains(report, want) {
		t.Errorf("the gap report truncated silently — it must say %q, or an author reads three reasons "+
			"as the complete list:\n%s", want, report)
	}
}

// TestGapReportUnitCap is the same rule at the function, where the input can be
// varied freely — the fixture above can only produce the counts a project shape
// happens to yield.
func TestGapReportUnitCap(t *testing.T) {
	if got := readyAckGapReport(nil); got != "" {
		t.Errorf("no gaps must render nothing, got %q", got)
	}
	for _, n := range []int{1, readyAckGapCap, readyAckGapCap + 1, 47} {
		gaps := make([]string, n)
		for i := range gaps {
			gaps[i] = fmt.Sprintf("reason-%d", i)
		}
		got := readyAckGapReport(gaps)
		shown := min(n, readyAckGapCap)
		for i := 0; i < shown; i++ {
			if !strings.Contains(got, fmt.Sprintf("reason-%d", i)) {
				t.Errorf("n=%d: missing reason-%d in %q", n, i, got)
			}
		}
		if shown < n {
			if !strings.Contains(got, fmt.Sprintf("and %d more", n-shown)) {
				t.Errorf("n=%d: overflow of %d not disclosed: %q", n, n-shown, got)
			}
			if strings.Contains(got, fmt.Sprintf("reason-%d", readyAckGapCap)) {
				t.Errorf("n=%d: rendered past the cap: %q", n, got)
			}
		} else if strings.Contains(got, "more") {
			t.Errorf("n=%d: claimed an overflow with nothing withheld: %q", n, got)
		}
	}
}

// TestGapReportIsOneLine pins the wire contract. `Finding.Message` is a `--json`
// string field; the human layout happens at the printer (internal/cmd), and a
// newline here would break a consumer without breaking any assertion about the
// text. A gap interpolates `%v` of an OS error, which is not guaranteed
// newline-free.
func TestGapReportIsOneLine(t *testing.T) {
	got := readyAckGapReport([]string{"a reason\nsplit over\r\ntwo lines", "and\ta tab"})
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("the gap report carries a line break: %q", got)
	}
	for _, want := range []string{"a reason split over two lines", "and a tab"} {
		if !strings.Contains(got, want) {
			t.Errorf("collapsing whitespace lost content: want %q in %q", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// CONTROLS. Without these, "the presence tier names its reasons" is satisfied by
// a check that reports the presence tier at every project.
// ---------------------------------------------------------------------------

// TestGapReportDoesNotLeakIntoTheStrongTiers is the structural half of item 20's
// disclosure rule, aimed at THIS change: the reachability tiers resolved the
// graph completely, so they have no gaps to report and must not acquire the
// weak tier's apparatus.
func TestGapReportDoesNotLeakIntoTheStrongTiers(t *testing.T) {
	cases := []struct {
		name, kind string
		build      func(*testing.T) string
	}{
		{"unwired", "unwired", func(t *testing.T) string {
			dir := renderTemplate(t, scaffold.Static)
			editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
			return dir
		}},
		{"missing", "missing", func(t *testing.T) string {
			dir := renderTemplate(t, scaffold.Static)
			editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
			if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := wantAckKind(t, c.build(t), c.kind)
			for _, w := range res.Warnings {
				if isPresenceOnlyAdvice(w.Message) {
					t.Fatalf("the %s tier emitted a presence-tier message", c.name)
				}
				if strings.Contains(w.Message, readyAckGapLead) {
					t.Fatalf("the %s tier carries the gap report's lead-in — it resolved the graph completely, "+
						"so it has nothing it could not follow, and saying otherwise blurs the two tiers:\n%s",
						c.name, w.Message)
				}
			}
		})
	}
}

// TestGapReportNeverAppearsAtACorrectProject is the other control: the shipped
// templates resolve completely and must stay silent, gap report or not.
func TestGapReportNeverAppearsAtACorrectProject(t *testing.T) {
	examined := 0
	for _, tmpl := range scaffold.AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			wantAckKind(t, renderTemplate(t, tmpl), "")
		})
		examined++
	}
	if examined < 3 {
		t.Fatalf("examined only %d template(s) — the enumeration stopped OBSERVING", examined)
	}
}

// TestPresenceAdviceHalvesBracketTheReport pins the assumption every matcher in
// this package now rests on: the head and the tail are non-empty and neither is
// a prefix of the other tiers' messages, so bracketing really identifies the
// presence tier.
//
// 🔴 An empty half silently disarms `isPresenceOnlyAdvice` —
// `strings.HasPrefix(x, "")` is always true — which would classify BOTH
// reachability tiers as presence-only and make every wantAckKind in this package
// assert the wrong thing while staying green.
func TestPresenceAdviceHalvesBracketTheReport(t *testing.T) {
	if readyAckAdvicePresenceOnlyHead == "" || readyAckAdvicePresenceOnlyTail == "" {
		t.Fatal("a half of the presence advisory is empty; isPresenceOnlyAdvice matches everything")
	}
	if got := readyAckAdvicePresenceOnlyHead + readyAckAdvicePresenceOnlyTail; got != readyAckAdvicePresenceOnly {
		t.Fatal("readyAckAdvicePresenceOnly is not the concatenation of its two halves — the ledger entry " +
			"and the emitted message have drifted apart")
	}
	if !isPresenceOnlyAdvice(readyAckAdvicePresenceOnly) {
		t.Fatal("the no-gaps presence message is not recognised as one")
	}
	if !isPresenceOnlyAdvice(presenceOnlyAdvice([]string{"x"})) {
		t.Fatal("a presence message WITH a gap report is not recognised as one")
	}
	for name, other := range map[string]string{"unwired": readyAckAdviceUnwired, "missing": readyAckAdviceMissing} {
		if isPresenceOnlyAdvice(other) {
			t.Errorf("the %s advisory is bracketed by the presence tier's halves — the tiers are no longer "+
				"distinguishable, and readyAckKind reports the wrong one", name)
		}
	}
}

// TestGapReportCannotSatisfyAnotherTiersStrengthAssertion is the case
// TestReadyAckAdvisoriesStateTheirOwnStrength cannot make, because that test
// operates on the FIXED bases and this change adds text at runtime.
//
// 🔴 The inverse assertions there are what stop the tiers blurring together, and
// they would keep passing if the appended report happened to carry another
// tier's own literal — the assertion never sees an emitted message. So the
// literals are re-checked against a REAL rendered advisory here.
func TestGapReportCannotSatisfyAnotherTiersStrengthAssertion(t *testing.T) {
	dir := renderTemplate(t, scaffold.Static)
	if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
		t.Fatal(err)
	}
	report := gapReportFor(t, dir)
	// These are TestReadyAckAdvisoriesStateTheirOwnStrength's `own` literals for
	// the two reachability tiers. None may appear in the presence tier's report.
	for _, lit := range []string{
		"DOES contain",
		"nothing index.html loads reaches it",
		"orphan",
		"nothing index.html loads posts it either",
	} {
		if strings.Contains(report, lit) {
			t.Errorf("the gap report carries %q, which is another tier's whole diagnosis — a reader can no "+
				"longer tell which check ran:\n%s", lit, report)
		}
	}
	// And the weak tier's own disclosure survives the splice, in the emitted
	// message rather than only in the constant.
	res := wantAckKind(t, dir, "presence-only")
	for _, w := range res.Warnings {
		if !isPresenceOnlyAdvice(w.Message) {
			continue
		}
		for _, want := range []string{"did NOT check that the file is loaded", "will silence this warning"} {
			if !strings.Contains(w.Message, want) {
				t.Errorf("the emitted presence advisory lost %q — the disclosure is the fix, not a nicety", want)
			}
		}
	}
}
