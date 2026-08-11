package cmd

// THE NEWEST-FIRST ASSUMPTION, PINNED — issue #378.
//
// `app pull` (explainMissingApp) and `app metrics` (resolveAppBlockID) reach the
// same precondition — submissions exist for the slug, none carries an
// appBlockId — and both answer it by reading ONE row out of a list the server
// returns newest-first, then handing that row's status to pullReviewAdvice.
//
// 🔴 THAT ROW CHOICE WAS UNPINNED AT BOTH SITES, AND NOT BECAUSE NOBODY TESTED
// IT. Every fixture the two commands' own tests used had a SINGLE row, where
// `subs[0]` and `subs[len(subs)-1]` are the same row — so mutating both call
// sites to the last row left the entire suite green (measured on this branch's
// parent: 3653 RUN, 0 FAIL, 0 SKIP-of-these). The seam guard added for the
// shared predicate (TestPullAndMetricsGiveTheSameNextStepForTheSameState) used
// the same one-row body, so it agreed with itself about the wrong row: both
// sites move together, so a shared-predicate check stays green while both are
// wrong. That is the isolation-seam shape — every fixture was scoped to one
// surface, and the surface none of them loaded was "a list with a history".
//
// Under that mutant `app pull` prints a self-contradictory sentence, because the
// parenthetical and the advice come from opposite ends of the list:
//
//	(latest submission: 0.1.1 pending); that submission was WITHDRAWN, so
//	nothing is in review
//
// So the guards here are (a) behavioural, per call site, over a MULTI-ROW list
// whose ends disagree, asserting the advice matches the NEWEST row, plus for
// `app pull` the relationship the mutant breaks — the state named in the
// parenthetical and the state the advice describes are the same state — and
// (b) a LEDGER of the files that CALL pullReviewAdvice, which fails when that
// set grows or shrinks, because a structural count is the only thing that
// notices a third CALLER OF THAT HELPER arriving with no behavioural case of its
// own. Its scope is the helper's callers and NOT every site that reads one row
// from a newest-first list — three such sites exist that it cannot see. They are
// enumerated on newestFirstAdviceSites and covered since #390 by a SECOND ledger
// (submissionsListReaders, newest_row_pick_test.go) keyed on references to
// appapi's ListSubmissions. Read both before treating a green run here as "the
// row-pick assumption is covered"; neither verifies that the SERVER really
// returns the list newest-first, which nothing client-side can.

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// twoRowNotApprovedBody is a never-approved (appBlockId null) submissions list
// whose ENDS DISAGREE: the newest row carries newest, the oldest carries oldest.
// One row is what made the mutation invisible, so a fixture here is only useful
// if first != last AND the two produce different advice.
func twoRowNotApprovedBody(t *testing.T, slug, newest, oldest string) string {
	t.Helper()
	if pullReviewAdvice(slug, newest) == pullReviewAdvice(slug, oldest) {
		t.Fatalf("statuses %q and %q produce the SAME advice, so this fixture cannot tell "+
			"the newest row from the oldest — pick states in different pullReviewAdvice branches", newest, oldest)
	}
	// Every field that could identify a row differs between the two — id,
	// version, status AND submittedAt. Identical timestamps would leave the rows
	// indistinguishable on the one axis that separates "trusts the server's
	// order" from "sorts client-side", so a future client-side sort could not be
	// told from the current behaviour.
	return `{"submissions":[
		{"id":"p2","blockId":"` + slug + `","appBlockId":null,"version":"0.1.1","status":"` + newest + `","submittedAt":"2026-06-21T08:00:00.000Z"},
		{"id":"p1","blockId":"` + slug + `","appBlockId":null,"version":"0.1.0","status":"` + oldest + `","submittedAt":"2026-06-20T08:00:00.000Z"}
	]}`
}

// latestParenthetical extracts the `(latest submission: <version> <status>)`
// `app pull` prints, so the assertion below can compare the state the CLI
// REPORTED against the state its advice DESCRIBES rather than against a literal
// the test also chose.
var latestParenthetical = regexp.MustCompile(`\(latest submission: ([^)]*)\)`)

func newestFirstCases() []struct {
	name           string
	newest, oldest string
} {
	return []struct {
		name           string
		newest, oldest string
	}{
		// The issue's prescribed shape: a resubmission still in review over a
		// withdrawn predecessor. The mutant answers "that submission was
		// WITHDRAWN, so nothing is in review" for an app that IS in review.
		{"newest pending over an older withdrawn", "pending", "withdrawn"},
		// The mirror, so "always say check-where-it-is-in-review" cannot pass:
		// the newest row is terminal and the older one is not.
		{"newest withdrawn over an older pending", "withdrawn", "pending"},
		{"newest rejected over an older pending", "rejected", "pending"},
		// Two terminal states that are not the same terminal state.
		{"newest rejected over an older withdrawn", "rejected", "withdrawn"},
	}
}

// TestAppPullAdviceNamesTheNewestSubmission is call site 1 of the ledger:
// explainMissingApp (internal/cmd/app_pull.go).
func TestAppPullAdviceNamesTheNewestSubmission(t *testing.T) {
	const slug = "my-block"
	for _, tc := range newestFirstCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv := pullDisambiguationServer(t, twoRowNotApprovedBody(t, slug, tc.newest, tc.oldest), http.StatusOK)
			defer srv.Close()
			pullEnv(t, srv)

			_, _, err := run(t, "app", "pull", "--app", slug)
			if err == nil {
				t.Fatal("expected an error when nothing is approved yet")
			}
			msg := err.Error()

			// REACHABILITY, asserted before anything else is read out of msg: an
			// earlier check bailing out (the lookup declining, a row with an
			// appBlockId, the len==0 branch) would leave the server's own
			// message here, and every assertion below would then be about a
			// sentence the disambiguation never wrote.
			if !strings.Contains(msg, "no approved version yet") {
				t.Fatalf("the disambiguation did not run — nothing below is about the newest row; got: %s", msg)
			}

			// THE REPORTED STATE: the parenthetical must name the newest row.
			m := latestParenthetical.FindStringSubmatch(msg)
			if m == nil {
				t.Fatalf("no `(latest submission: …)` to compare the advice against; got: %s", msg)
			}
			reported := strings.Fields(m[1])
			if len(reported) != 2 {
				t.Fatalf("the parenthetical is not `<version> <status>`: %q", m[1])
			}
			if reported[0] != "0.1.1" || reported[1] != tc.newest {
				t.Errorf("`latest submission:` names %q, not the newest row (0.1.1 %s)", m[1], tc.newest)
			}

			// THE RELATIONSHIP the mutant breaks: whatever state the CLI just
			// reported, the advice must be the advice FOR THAT STATE. Derived
			// from the parenthetical, not from tc, so a message that is
			// internally consistent about the wrong row still fails the check
			// above rather than passing both by accident.
			if want := pullReviewAdvice(slug, reported[1]); !strings.Contains(msg, want) {
				t.Errorf("the message reports %q and then gives the advice for a different state.\nwant substring: %s\ngot: %s",
					m[1], want, msg)
			}
			// And explicitly not the OLDEST row's advice — the mutant's output.
			if other := pullReviewAdvice(slug, tc.oldest); strings.Contains(msg, other) {
				t.Errorf("the advice is the OLDEST row's (%s), not the newest's (%s) — subs[0] became subs[len(subs)-1].\ngot: %s",
					tc.oldest, tc.newest, msg)
			}

			// AGENTS.md item 7: this message replaces a not-found error and its
			// exit code (4) is pinned by the sentinel, never by the text.
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("want the not-found classification (exit 4), got %T: %v", err, err)
			}
		})
	}
}

// TestAppMetricsAdviceNamesTheNewestSubmission is call site 2 of the ledger:
// resolveAppBlockID (internal/cmd/app_metrics.go).
//
// `app metrics` prints no `(latest submission: …)` parenthetical, so the
// self-contradiction is INVISIBLE here — the wrong advice simply reads as the
// truth about a state the user is never shown. That is why this site needs its
// own case rather than riding on `app pull`'s.
func TestAppMetricsAdviceNamesTheNewestSubmission(t *testing.T) {
	const slug = "stuck-app"
	for _, tc := range newestFirstCases() {
		t.Run(tc.name, func(t *testing.T) {
			var rec metricsRec
			srv := metricsServer(t, twoRowNotApprovedBody(t, slug, tc.newest, tc.oldest), http.StatusOK,
				trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
			defer srv.Close()
			setupMetricsEnv(t, srv.URL)

			_, _, err := run(t, "app", "metrics", slug)
			if err == nil {
				t.Fatal("expected an error when every submission has a null appBlockId")
			}
			msg := err.Error()
			// REACHABILITY: the resolver's own precondition, not a transport or
			// envelope failure that would carry no advice at all.
			if !strings.Contains(msg, "no approved App Block yet") {
				t.Fatalf("the resolver did not reach the not-approved branch; got: %s", msg)
			}
			if rec.trpcReached {
				t.Error("a null appBlockId must not reach the analytics query")
			}
			// 🔴 WHAT THE README MAY CLAIM ABOUT THIS MESSAGE, AND NO MORE.
			// `app metrics` prints NO `(latest submission: …)` pair — so for a
			// PENDING app it names no state at all, and prose saying it "names
			// the latest submission's state" is false for the commonest case.
			// The section says "the next step FOR the latest submission's own
			// state" instead, and its parenthetical is scoped to THIS pair,
			// because the status word itself IS printed (uppercased) in the
			// rejected and withdrawn branches. Widening that sentence past what
			// this assertion measures is how it went wrong the first time.
			if m := latestParenthetical.FindString(msg); m != "" {
				t.Errorf("`app metrics` printed %q — if it now reports the state itself, the README "+
					"`App metrics` section says it does not, and moves with this", m)
			}

			if want := pullReviewAdvice(slug, tc.newest); !strings.Contains(msg, want) {
				t.Errorf("the advice is not the NEWEST row's.\nwant substring: %s\ngot: %s", want, msg)
			}
			if other := pullReviewAdvice(slug, tc.oldest); strings.Contains(msg, other) {
				t.Errorf("the advice is the OLDEST row's (%s), not the newest's (%s) — subs[0] became subs[len(subs)-1].\ngot: %s",
					tc.oldest, tc.newest, msg)
			}

			// AGENTS.md item 7, the other direction: `app metrics` publishes this
			// state as exit 1 (the app EXISTS, its analytics do not), so it must
			// not have acquired `app pull`'s not-found classification.
			if errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("a never-approved app still EXISTS — exit 4 promises it does not; got %T: %v", err, err)
			}
			if errors.Is(err, ErrUsage) {
				t.Errorf("this is not a mistake about the invocation (exit 2); got %T: %v", err, err)
			}
		})
	}
}

// newestFirstAdviceSites is the LEDGER of every non-test file that CALLS
// pullReviewAdvice, mapped to the behavioural test proving that call reads the
// NEWEST row, plus the argv fragment that test must actually drive.
//
// 🔴 ITS SCOPE IS "CALLERS OF pullReviewAdvice", NOT "READS ONE ROW OUT OF A
// NEWEST-FIRST LIST", AND THE DIFFERENCE IS NOT HYPOTHETICAL. The scanner is
// keyed on the helper's name, so it sees a new caller of THAT helper and nothing
// else. An earlier version of this comment claimed it catches "a third command
// open-coding the same row pick"; it does not. Three sites read one row from a
// newest-first list without calling this helper, each of which survived reversal
// with the whole suite green, and #390 fixed all three:
//
//   - internal/cmd/apps.go, ownedSubmission — the first slug-matching row, whose
//     Status/DeployState are printed verbatim.
//   - internal/cmd/app_metrics.go, the first-non-null appBlockId pick five lines
//     above the call this ledger covers.
//   - internal/appapi/appblocks.go, latestMatchingSubmission — outside this
//     package, so outside this walk entirely.
//
// Widening THIS scanner to the row-pick SHAPE was rejected and still is: any
// pattern over `subs[0]` is spelled rather than structural (it turns on a
// variable name). #390 did not widen it either — it added a SECOND ledger,
// submissionsListReaders in newest_row_pick_test.go, keyed on a different and
// structural predicate: a reference to appapi's ListSubmissions, which is the
// only way to obtain the list and is an exported name rather than a local
// spelling. That ledger spans internal/cmd AND internal/appapi and covers all
// five readers, so app_pull.go and app_metrics.go appear in both maps on purpose.
// The two answer different questions — this one asks whether every caller of the
// SHARED ADVICE proves it describes the newest row; that one asks whether every
// reader of the LIST declares which end it reads. Do not merge them: adding the
// other three files here would fail this ledger's own shrink-direction check,
// since they do not call pullReviewAdvice at all.
//
// Within that scope it is asserted in BOTH directions: a shrunk set means a call
// site lost its coverage, a grown one means a new caller arrived with no
// behavioural case of its own — which is how `app metrics` inherited the
// untested assumption along with the sentence.
type newestFirstSite struct {
	// pinnedBy is the behavioural test that proves this file reads subs[0].
	pinnedBy string
	// exercises is a fragment of that test's OWN BODY, naming the command this
	// file implements. Without it the binding is satisfied by any test in the
	// package, so swapping two pinnedBy values stayed green — and the
	// shrink-direction message then tells a maintainer to delete the wrong test.
	exercises string
}

var newestFirstAdviceSites = map[string]newestFirstSite{
	"app_pull.go": {
		pinnedBy:  "TestAppPullAdviceNamesTheNewestSubmission",
		exercises: `run(t, "app", "pull"`,
	},
	"app_metrics.go": {
		pinnedBy:  "TestAppMetricsAdviceNamesTheNewestSubmission",
		exercises: `run(t, "app", "metrics"`,
	},
}

// countPullReviewAdviceCalls counts CALL sites of pullReviewAdvice in one Go
// source, ignoring its own declaration and anything after a `//`.
//
// Its two known blind spots both fail SILENT-GREEN, so they are stated rather
// than implied: it strips from the first `//` regardless of string context (a
// call written after a `//` inside a string literal is invisible), and it counts
// the identifier followed by `(`, so a call reached through a function value
// (`f := pullReviewAdvice; f(app, s)`) is not counted. Neither shape exists in
// this package today; if one appears, the ledger under-reports rather than
// erroring, which is why the walk below also asserts a floor.
func countPullReviewAdviceCalls(src string) int {
	n := 0
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		if strings.Contains(line, "func pullReviewAdvice(") {
			continue
		}
		n += strings.Count(line, "pullReviewAdvice(")
	}
	return n
}

// TestPullReviewAdviceCallScannerIsCalibrated validates the INSTRUMENT the
// ledger below reads its verdict from, in both directions — a scanner that can
// never count and one that counts everything are indistinguishable from a green
// ledger otherwise.
func TestPullReviewAdviceCallScannerIsCalibrated(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		// Positive control: it can observe a call at all.
		{"a call", "x := pullReviewAdvice(app, subs[0].Status)\n", 1},
		{"two calls on separate lines", "a := pullReviewAdvice(s, x)\nb := pullReviewAdvice(s, y)\n", 2},
		// Negative controls: it must reject the things that are not call sites.
		{"the declaration itself", "func pullReviewAdvice(app, status string) string {\n", 0},
		{"a comment mentioning the call", "// both call pullReviewAdvice(slug, subs[0].Status) here\n", 0},
		{"a trailing comment beside code", "y := 1 // pullReviewAdvice(a, b)\n", 0},
		{"an unrelated file", "package cmd\n\nfunc other() {}\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := countPullReviewAdviceCalls(tc.src); got != tc.want {
				t.Errorf("countPullReviewAdviceCalls(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

// testFuncBody returns the source of the top-level func named name — from its
// declaration to its own closing brace — or "" when it is not declared. Bodies
// are compared, not just names, so a ledger entry cannot be satisfied by an
// unrelated test that merely exists.
//
// 🔴 IT ENDS AT THE CLOSING BRACE, NOT AT THE NEXT `func`, AND THE DIFFERENCE
// WAS A LIVE HOLE. Ending at the next top-level func swallowed everything
// between them — for TestAppMetricsAdviceNamesTheNewestSubmission that was 6013
// bytes reaching past `var newestFirstAdviceSites`, i.e. THE LEDGER'S OWN
// `exercises` LITERALS. So the metrics test's "body" contained the string
// `run(t, "app", "pull"` (the ledger data, not the test), and repointing
// app_pull.go's entry at the metrics test stayed green: a stale re-bind, exactly
// what this binding exists to catch. Every future `exercises` value would have
// landed in that same swallowed literal, so the guard weakened as the ledger
// grew. Measured before the fix: pull body 3062 bytes / metrics body 6013.
//
// Truncating on a column-0 `}` assumes gofmt, which nests braces under indent.
// 🔴 THAT IS NOT ENFORCED BY `make ci` — it runs `tidy vet test build` and no
// formatter. The gofmt gate is CI's `build-test` job (`gofmt -s -l .`) and
// `make lint` (golangci-lint `formatters: gofmt`). AGENTS.md carries a 🔴 about
// exactly this misattribution and #287 removed the same false claim from
// AGENTS.md itself; naming the wrong enforcer here would just re-introduce it.
//
// Two shapes defeat the column-0 brace, in OPPOSITE directions, and only one of
// them is safe:
//
//   - A raw string literal holding a line that is exactly `}` ends the body
//     EARLY. Measured: the func's own argv drops out, so the ledger fails loudly.
//     Safe — it cannot match something it should not see.
//   - `} // trailing comment` on the closing brace over-includes SILENTLY, which
//     is the NEW-1 direction, and it is gofmt-clean, so no gate catches it.
//     Measured on this file: the metrics body grows 2727 → 5260 bytes, running
//     through the `newestFirstSite` type declaration but stopping BEFORE
//     `var newestFirstAdviceSites` — so the binding still holds today and the
//     re-bind mutant still fails.
//
// 🔴 But it holds on the DECLARATION ORDER between these tests and the ledger
// var, and nothing pins that order. Moving `var newestFirstAdviceSites` above
// the pinning tests, or adding a trailing comment plus a reorder, is what would
// reopen NEW-1. If you reorder this file, re-run the re-bind mutant (repoint one
// entry's pinnedBy at the other's test, keep its `exercises`) and confirm it
// still reddens.
func testFuncBody(src, name string) string {
	decl := "\nfunc " + name + "("
	i := strings.Index("\n"+src, decl)
	if i < 0 {
		return ""
	}
	body := ("\n" + src)[i+1:]
	// Upper bound, and REDUNDANT under gofmt — stated rather than sold as a
	// second guard. gofmt always ends a file with a newline, so the column-0
	// brace below always matches first. Measured: deleting this line alone
	// leaves the suite green (an equivalent mutant on gofmt'd input); deleting
	// BOTH truncations goes red on the calibration battery. It is kept only as a
	// bound for input with no column-0 closing brace at all.
	if j := strings.Index(body[1:], "\nfunc "); j >= 0 {
		body = body[:j+1]
	}
	// The real end: this func's own closing brace, at column 0.
	if j := strings.Index(body, "\n}\n"); j >= 0 {
		body = body[:j+2]
	}
	return body
}

// TestTestFuncBodyIsCalibrated is the instrument control for the binding below:
// an extractor that returned "" for everything would make every `exercises`
// check fail loudly, but one that returned the WHOLE FILE would make every one
// of them pass — the silent-green direction, and the one that matters.
func TestTestFuncBodyIsCalibrated(t *testing.T) {
	const src = "package cmd\n\n" +
		"func TestAlpha(t *testing.T) {\n\trun(t, \"app\", \"pull\")\n}\n\n" +
		"func TestBeta(t *testing.T) {\n\trun(t, \"app\", \"metrics\")\n}\n"

	alpha := testFuncBody(src, "TestAlpha")
	if !strings.Contains(alpha, `run(t, "app", "pull")`) {
		t.Errorf("the extractor lost TestAlpha's own body: %q", alpha)
	}
	// THE POINT: it must STOP at the next func, or the binding is satisfied by
	// any test anywhere in the file.
	if strings.Contains(alpha, `run(t, "app", "metrics")`) {
		t.Errorf("the extractor ran past the end of TestAlpha into TestBeta: %q", alpha)
	}
	if got := testFuncBody(src, "TestGamma"); got != "" {
		t.Errorf("a func that is not declared must extract to \"\", got %q", got)
	}
	// A prefix must not match a longer name (TestAlphaExtra != TestAlpha).
	if got := testFuncBody("func TestAlphaExtra(t *testing.T) {}\n", "TestAlpha"); got != "" {
		t.Errorf("a name prefix must not bind, got %q", got)
	}

	// 🔴 THE CASE THAT HID THE REAL BUG: a func followed by a top-level
	// declaration that is NOT a func. The battery above is func→func only, so an
	// extractor ending at "the next func" passed every case here while swallowing
	// `var newestFirstAdviceSites` in the real file — and that var holds the very
	// `exercises` literals the binding searches for, making it self-satisfying.
	for _, decl := range []struct{ name, src string }{
		{"var", "var newestFirstAdviceSites = map[string]newestFirstSite{\n\t\"x\": {exercises: `run(t, \"app\", \"metrics\"`},\n}\n"},
		{"type", "type other struct {\n\tf string // run(t, \"app\", \"metrics\"\n}\n"},
		{"const", "const other = `run(t, \"app\", \"metrics\"`\n"},
	} {
		t.Run("stops before a top-level "+decl.name, func(t *testing.T) {
			src := "package cmd\n\nfunc TestOnly(t *testing.T) {\n\trun(t, \"app\", \"pull\")\n}\n\n" + decl.src
			body := testFuncBody(src, "TestOnly")
			if !strings.Contains(body, `run(t, "app", "pull")`) {
				t.Fatalf("the extractor lost TestOnly's own body: %q", body)
			}
			if strings.Contains(body, `run(t, "app", "metrics"`) {
				t.Errorf("the extractor ran past TestOnly's closing brace into the following %s, "+
					"so a ledger literal declared there would satisfy the binding it is supposed to check: %q", decl.name, body)
			}
		})
	}

	// The LAST func in a file has no following declaration to stop at — it must
	// still extract, or the binding silently depends on file order.
	last := testFuncBody("package cmd\n\nfunc TestLast(t *testing.T) {\n\trun(t, \"app\", \"pull\")\n}\n", "TestLast")
	if !strings.Contains(last, `run(t, "app", "pull")`) {
		t.Errorf("the last func in a file must still extract, got %q", last)
	}

	// Nested braces are indented under gofmt, so they must not end the body.
	nested := testFuncBody("package cmd\n\nfunc TestNested(t *testing.T) {\n\tif x {\n\t\ty()\n\t}\n\trun(t, \"app\", \"pull\")\n}\n", "TestNested")
	if !strings.Contains(nested, `run(t, "app", "pull")`) {
		t.Errorf("an indented closing brace must not end the body, got %q", nested)
	}
}

// TestNewestFirstAdviceCallSitesAreLedgered fails when the set of call sites
// grows or shrinks, and when a ledgered site's pinning test is missing or does
// not actually drive that site's command.
func TestNewestFirstAdviceCallSitesAreLedgered(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	found := map[string]int{}
	var tests strings.Builder
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if strings.HasSuffix(name, "_test.go") {
			tests.Write(b)
			continue
		}
		scanned++
		if n := countPullReviewAdviceCalls(string(b)); n > 0 {
			found[name] = n
		}
	}
	// Positive control on the walk: a mis-set working directory, or a filter
	// that matched nothing, would otherwise report an empty set as "no drift".
	// The floor is near the real count (55 non-test files at the time of
	// writing), because a floor far below it passes a PARTIAL walk — which is
	// the same silent green with extra steps.
	if scanned < 40 {
		t.Fatalf("only %d non-test .go files scanned in internal/cmd — the walk is wrong or partial, so an empty result means nothing", scanned)
	}
	if len(found) == 0 {
		t.Fatal("the scan found NO pullReviewAdvice call sites at all — the ledger is wired to nothing")
	}

	for name := range found {
		if _, ok := newestFirstAdviceSites[name]; !ok {
			t.Errorf("internal/cmd/%s calls pullReviewAdvice and is NOT in newestFirstAdviceSites.\n"+
				"It selects one row out of a newest-first list, which is the assumption issue #378 found "+
				"unpinned at BOTH existing sites. Add it to the ledger together with a behavioural case "+
				"over a multi-row fixture whose ends disagree (see twoRowNotApprovedBody).", name)
		}
	}
	for name, site := range newestFirstAdviceSites {
		if _, ok := found[name]; !ok {
			t.Errorf("newestFirstAdviceSites names internal/cmd/%s, which no longer calls pullReviewAdvice. "+
				"If the call moved, move the ledger entry with it; if it went away, drop the entry and %s.",
				name, site.pinnedBy)
			continue
		}
		body := testFuncBody(tests.String(), site.pinnedBy)
		if body == "" {
			t.Errorf("internal/cmd/%s is ledgered as pinned by %s, which is not a test in this package — "+
				"the ledger names coverage that does not exist", name, site.pinnedBy)
			continue
		}
		// THE BINDING, not merely the name: the pinning test must drive THIS
		// file's command. Existence alone left the two entries swappable, and a
		// swapped ledger actively misdirects — its shrink-direction message
		// names the wrong test to delete.
		if !strings.Contains(body, site.exercises) {
			// The message states the REQUIREMENT, not a diagnosis. Two very
			// different things reach here: a stale re-bind (the hazard), and a
			// legitimate refactor that moved the argv into a helper (not a
			// hazard at all). Asserting "bound to the wrong test" is false in
			// the second case and sends the reader hunting for a mistake nobody
			// made, so name what the guard needs instead of guessing why.
			t.Errorf("internal/cmd/%s is ledgered as pinned by %s, but that test's body does not contain %s.\n"+
				"This binding requires the argv fragment LITERALLY in the pinning test's own body, so the "+
				"ledger cannot be satisfied by an unrelated test that merely exists. Either the entry now names "+
				"the wrong test (re-point it), or the invocation moved into a helper (inline it in the test, or "+
				"update this entry's `exercises` to a fragment that is still literally there).",
				name, site.pinnedBy, site.exercises)
		}
	}
}

// TestREADMEAppMetricsSectionNamesTheTerminalStates is the OTHER half of #378:
// "README" is not one place. The `app metrics` behaviour reached the exit-code
// bullet and the errors table but not the command's own section, which still
// described the whole state as "an app still in review" — the narrow reading the
// shared advice exists to correct, and the one a reader of that section gets.
//
// It asserts the load-bearing nouns rather than a sentence, so a rewrite that
// keeps the meaning keeps this green.
func TestREADMEAppMetricsSectionNamesTheTerminalStates(t *testing.T) {
	section := readmeSectionSlice(t, readREADME(t), "\n## App metrics\n")
	// Positive control on the extractor before reading anything out of it.
	if len(section) < 400 {
		t.Fatalf("the `App metrics` section is only %d bytes — the extractor is reading the wrong block:\n%s", len(section), section)
	}
	for _, want := range []string{"rejected", "withdrawn", "civitai app submit"} {
		if !strings.Contains(section, want) {
			t.Errorf("the README `App metrics` section does not name %q.\n"+
				"`app metrics` gives a rejected or withdrawn app a NEW-SUBMISSION next step, not a review to wait for; "+
				"the exit-code bullet and the errors table both say so and this section is the third surface (#378).", want)
		}
	}
	// The parallel `app pull` section is where this wording came from — if it
	// loses it, the two sections have drifted again in the other direction.
	pull := readmeSectionSlice(t, readREADME(t), "\n## Pull your app's repository (`app pull`)\n")
	if len(pull) < 400 {
		t.Fatalf("the `app pull` section is only %d bytes — the extractor is reading the wrong block:\n%s", len(pull), pull)
	}
	for _, want := range []string{"rejected", "withdrawn"} {
		if !strings.Contains(pull, want) {
			t.Errorf("the README `app pull` section no longer names %q — the two sections describe ONE shared next step", want)
		}
	}
}
