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
// (b) a LEDGER of the call sites that consume the ordering, which fails when
// that set grows or shrinks, because a structural count is the only thing that
// notices a THIRD caller arriving with no behavioural case of its own.

import (
	"errors"
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
	return `{"submissions":[
		{"id":"p2","blockId":"` + slug + `","appBlockId":null,"version":"0.1.1","status":"` + newest + `"},
		{"id":"p1","blockId":"` + slug + `","appBlockId":null,"version":"0.1.0","status":"` + oldest + `"}
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
			srv := pullDisambiguationServer(t, twoRowNotApprovedBody(t, slug, tc.newest, tc.oldest), 0)
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
			srv := metricsServer(t, twoRowNotApprovedBody(t, slug, tc.newest, tc.oldest), 0,
				trpcEnvelope(verifiedAnalyticsPayload), 0, &rec)
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

// newestFirstAdviceSites is the LEDGER: every non-test file that turns a
// newest-first submissions list into the "nothing is approved yet" next step,
// mapped to the behavioural test that proves it reads the NEWEST row.
//
// It is asserted in BOTH directions on purpose. A shrunk set means a call site
// lost its coverage; a GROWN set is the case a behavioural test cannot see at
// all — a third command open-coding the same row pick, arriving green because
// no fixture of its own ever had two rows. That is exactly how this defect got
// in: `app metrics` was the second caller, and it inherited the untested
// assumption along with the sentence.
var newestFirstAdviceSites = map[string]string{
	"app_pull.go":    "TestAppPullAdviceNamesTheNewestSubmission",
	"app_metrics.go": "TestAppMetricsAdviceNamesTheNewestSubmission",
}

// countPullReviewAdviceCalls counts CALL sites of pullReviewAdvice in one Go
// source, ignoring its own declaration and anything after a `//`.
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

// TestNewestFirstAdviceCallSitesAreLedgered fails when the set of call sites
// grows or shrinks, and when a ledgered site names a behavioural test that is
// not in the package.
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
	if scanned < 10 {
		t.Fatalf("only %d non-test .go files scanned in internal/cmd — the walk is wrong, so an empty result means nothing", scanned)
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
	for name, pinnedBy := range newestFirstAdviceSites {
		if _, ok := found[name]; !ok {
			t.Errorf("newestFirstAdviceSites names internal/cmd/%s, which no longer calls pullReviewAdvice. "+
				"If the call moved, move the ledger entry with it; if it went away, drop the entry and %s.",
				name, pinnedBy)
			continue
		}
		if !strings.Contains(tests.String(), "func "+pinnedBy+"(") {
			t.Errorf("internal/cmd/%s is ledgered as pinned by %s, which is not a test in this package — "+
				"the ledger names coverage that does not exist", name, pinnedBy)
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
