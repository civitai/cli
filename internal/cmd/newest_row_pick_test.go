package cmd

// THE NEWEST-FIRST ROW PICK, PINNED AT EVERY READER OF THE LIST — issue #390.
//
// GET /api/v1/blocks/submissions returns the caller's submissions NEWEST FIRST,
// and five non-test files read that list. Three of them picked ONE row out of it
// and pinned nothing about WHICH end they read, because every fixture they had
// was a single row — where `subs[0]` and `subs[len(subs)-1]` are the same row, so
// the choice cannot be observed. Measured on this branch's parent (d9f9c67),
// reversing each pick ALONE left the entire suite green:
//
//	site                                        mutated to            result
//	internal/cmd/apps.go ownedSubmission        last matching row     3786 RUN, 0 FAIL
//	internal/cmd/app_metrics.go appBlockId scan last non-null row     3786 RUN, 0 FAIL
//	internal/appapi/appblocks.go latestMatching last matching row     3786 RUN, 0 FAIL
//
// `ownedSubmission` is the one that matters: `appViewOwnedAdvice` prints its
// row's Status and DeployState VERBATIM ("submission status: %s, deploy: %s").
// #384's defect at least printed a self-contradictory sentence a reader could
// notice; here the oldest row's state reads as plain fact about the newest
// submission, and an author whose newest version is `pending` over an older
// `approved`/`live` one is told the wrong deploy state of their own app.
//
// 🔴 WHY THIS IS A SECOND LEDGER AND NOT A WIDENING OF #384's.
// #384's ledger (app_newest_submission_test.go) is keyed on CALL SITES OF
// pullReviewAdvice. That predicate structurally cannot reach these three: apps.go
// and appblocks.go never call that helper at all, and app_metrics.go is already
// in that ledger while carrying a SECOND, unpinned pick five lines above the call
// it covers. Adding these files to that map would fail its own shrink-direction
// check ("names a file which no longer calls pullReviewAdvice") — a ledger keyed
// on the wrong predicate is not the same rule.
//
// So this ledger is keyed on a different, and structural, thing: PROVENANCE OF
// THE LIST — a reference to appapi's ListSubmissions, the only way to obtain it.
// That is a package-level exported name, not a local spelling: #384 rejected a
// scanner over the row-pick SHAPE precisely because any pattern matching `subs[0]`
// turns on a variable name a rename defeats. `ListSubmissions` cannot be renamed
// without renaming it here too, and it catches a reference taken as a FUNCTION
// VALUE (`listSubmissions: client.ListSubmissions`) — which is how both
// `app pull` and `app metrics` actually reach the list.
//
// The two ledgers therefore overlap by design and answer different questions:
// #384's asks "does every caller of the shared advice have a case proving it
// describes the newest row?", this one asks "does every reader of a newest-first
// list declare, and pin, which end it reads?". app_pull.go and app_metrics.go
// appear in both.
//
// 🔴 WHAT NEITHER LEDGER CAN CHECK, RESTATED RATHER THAN CLOSED. Every guard here
// asserts what the CLI does with a list the TEST SERVER hands back in
// newest-first order. Nothing client-side verifies that the real server orders it
// that way: `ListSubmissions` does not sort, and the route sends no field these
// guards could check an ordering against. So if that server-side contract ever
// changed, every test in this file would stay green while every message went
// wrong together. That residual is NOT closeable from here — closing it means
// either sorting client-side on `submittedAt` (a behaviour change, and one that
// would need its own decision because `app status --limit` and the cap caveat
// both inherit the server's order) or a server-side guarantee. Neither is in
// scope for #390; what is in scope is that the CLI's OWN half is now pinned, so a
// future breakage is attributable to the server rather than hidden by an untested
// client.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Site 1: internal/cmd/apps.go — ownedSubmission
// ---------------------------------------------------------------------------

// ownedStatePair extracts the `(submission status: X, deploy: Y)` pair that
// appViewOwnedAdvice prints, so assertions compare the two FIELDS the row
// supplied rather than searching for bare words.
//
// 🔴 The word search is not an option here: the same message says "the deploy
// state and the live URL", so `Contains(msg, "live")` is satisfied by prose that
// never read deployState at all — a guard spelled rather than structural. The
// pair is positional, so a message that prints the right words in the wrong
// slots still fails.
var ownedStatePair = regexp.MustCompile(`\(submission status: ([^,)]*), deploy: ([^)]*)\)`)

// assertOwnedStatePair fails unless the advice reports exactly wantStatus and
// wantDeploy, which callers write out BY HAND from their fixture's newest row.
// Deriving them from the row the implementation selected would make the
// assertion agree with any implementation, including the reversed one.
func assertOwnedStatePair(t *testing.T, msg, wantStatus, wantDeploy string) {
	t.Helper()
	m := ownedStatePair.FindStringSubmatch(msg)
	if m == nil {
		t.Fatalf("the owned-slug advice printed no `(submission status: …, deploy: …)` pair, "+
			"so there is nothing to check the row pick against; got: %s", msg)
	}
	if m[1] != wantStatus || m[2] != wantDeploy {
		t.Errorf("the advice reports (status %q, deploy %q), want (%q, %q) — the NEWEST row's state.\n"+
			"Reading any other row means `app view` states a stale deploy state as fact about the newest submission (#390).\ngot: %s",
			m[1], m[2], wantStatus, wantDeploy, msg)
	}
}

// ownedRow is one row of a submissions listing, spelled out field by field so a
// fixture cannot accidentally share one between its two rows.
type ownedRow struct {
	id, version, status, deploy, submittedAt string
}

// twoRowOwnedBody builds a newest-first, two-row listing for slug whose ENDS
// DISAGREE, and refuses to build one whose ends could be confused.
//
// The guard is not decoration: one row is what made this pick invisible in the
// first place, and two rows sharing the field under assertion is the same hole
// with an extra row. #384's own fixture was flagged in audit for carrying an
// identical submittedAt across its rows, so every identifying field is checked
// here, not just the ones a given case asserts on.
func twoRowOwnedBody(t *testing.T, slug string, newest, oldest ownedRow) string {
	t.Helper()
	for _, f := range []struct{ name, a, b string }{
		{"id", newest.id, oldest.id},
		{"version", newest.version, oldest.version},
		{"status", newest.status, oldest.status},
		{"deployState", newest.deploy, oldest.deploy},
		{"submittedAt", newest.submittedAt, oldest.submittedAt},
	} {
		if f.a == f.b {
			t.Fatalf("both rows carry %s=%q — a shared field cannot tell the newest row from the oldest, "+
				"which is exactly how this pick went untested (#390)", f.name, f.a)
		}
	}
	row := func(r ownedRow) string {
		deploy := "null"
		if r.deploy != "" {
			deploy = `"` + r.deploy + `"`
		}
		return fmt.Sprintf(`{"id":%q,"blockId":%q,"version":%q,"status":%q,"deployState":%s,`+
			`"submittedAt":%q,"updatedAt":%q,"createdAt":%q,"liveUrl":null}`,
			r.id, slug, r.version, r.status, deploy, r.submittedAt, r.submittedAt, r.submittedAt)
	}
	return `{"submissions":[` + row(newest) + `,` + row(oldest) + `]}`
}

// TestAppViewOwnedAdviceNamesTheNewestSubmission is the behavioural case for
// internal/cmd/apps.go's row pick.
//
// The cases run in BOTH directions, so neither "always report the last row" nor
// a constant answer can pass: whichever pair a mutant prints is the expected
// answer of at most one case.
func TestAppViewOwnedAdviceNamesTheNewestSubmission(t *testing.T) {
	const slug = "buzz-generator"
	for _, tc := range []struct {
		name                   string
		newest, oldest         ownedRow
		wantStatus, wantDeploy string
	}{
		{
			// The issue's case: a resubmission in review over the approved/live
			// predecessor. Reading the oldest row answers "approved, live" for an
			// app whose newest version is still building.
			name:       "newest pending/building over an older approved/live",
			newest:     ownedRow{"pubreq_02H", "0.2.0", "pending", "building", "2026-07-29T10:00:00Z"},
			oldest:     ownedRow{"pubreq_01H", "0.1.1", "approved", "live", "2026-07-28T10:00:00Z"},
			wantStatus: "pending",
			wantDeploy: "building",
		},
		{
			// The mirror: the newest row is the healthy one. A mutant that always
			// reports the oldest tells this author their live app was REJECTED.
			name:       "newest approved/live over an older rejected/failed",
			newest:     ownedRow{"pubreq_04H", "0.4.0", "approved", "live", "2026-08-02T10:00:00Z"},
			oldest:     ownedRow{"pubreq_03H", "0.3.0", "rejected", "failed", "2026-08-01T10:00:00Z"},
			wantStatus: "approved",
			wantDeploy: "live",
		},
		{
			// The newest row has NO deployState (never deployed), so the advice
			// must print deployLabel's dash. This is the pair a wrong row pick
			// most easily hides: "live" is a plausible-looking answer, and it is
			// the one the oldest row supplies.
			name:       "newest withdrawn with no deploy state over an older approved/live",
			newest:     ownedRow{"pubreq_06H", "0.6.0", "withdrawn", "", "2026-08-04T10:00:00Z"},
			oldest:     ownedRow{"pubreq_05H", "0.5.0", "approved", "live", "2026-08-03T10:00:00Z"},
			wantStatus: "withdrawn",
			wantDeploy: "-",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := twoRowOwnedBody(t, slug, tc.newest, tc.oldest)
			probes := appViewServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			})

			_, _, err := run(t, "app", "view", slug)
			if err == nil {
				t.Fatal("expected a not-found error from the store 404")
			}
			msg := err.Error()
			// REACHABILITY, before anything is read out of msg: an ownership probe
			// that did not run, or ran and declined, leaves the plain 404 here and
			// every assertion below would be about a sentence nothing wrote.
			if n := atomic.LoadInt32(probes); n != 1 {
				t.Fatalf("the ownership probe ran %d times, want 1 — nothing below is about the row pick", n)
			}
			if !strings.Contains(msg, "one of your own apps") {
				t.Fatalf("the owned-slug advice did not fire, so the row pick was never reached; got: %s", msg)
			}
			assertOwnedStatePair(t, msg, tc.wantStatus, tc.wantDeploy)
		})
	}
}

// ---------------------------------------------------------------------------
// Site 2: internal/cmd/app_metrics.go — the first-non-null appBlockId pick
// ---------------------------------------------------------------------------

// metricsRowsBody builds a newest-first submissions listing from
// (version, status, appBlockId) triples; an empty id emits a null appBlockId.
// Every row gets its own id and submittedAt derived from its position, so no two
// rows of a fixture can collide on an identifying field.
func metricsRowsBody(slug string, rows ...[3]string) string {
	out := make([]string, 0, len(rows))
	for i, r := range rows {
		blockID := "null"
		if r[2] != "" {
			blockID = `"` + r[2] + `"`
		}
		out = append(out, fmt.Sprintf(
			`{"id":"pubreq_%02d","blockId":%q,"version":%q,"status":%q,"appBlockId":%s,"submittedAt":"2026-06-%02dT08:00:00.000Z"}`,
			len(rows)-i, slug, r[0], r[1], blockID, 20+len(rows)-i))
	}
	return `{"submissions":[` + strings.Join(out, ",") + `]}`
}

// TestAppMetricsUsesTheNewestNonNullAppBlockID is the behavioural case for
// app_metrics.go's OTHER row pick — the appBlockId scan, five lines above the
// pullReviewAdvice call #384 pinned.
//
// TestAppMetricsPicksNewestNonNullAppBlockID already existed and says "newest" in
// its NAME, but its fixture holds exactly ONE non-null row, so first and last
// non-null are the same row: it pins "skips the null row", which is real, and
// says nothing about newest. It is renamed to what it measures and this test
// takes the claim its name was making.
//
// This is the pick whose wrong answer is the most expensive of the three: the
// appBlockId chosen here is the key the analytics query is issued for, so an
// older block's id returns a real, plausible dashboard belonging to a DIFFERENT
// version — wrong numbers with nothing on screen to mark them as wrong.
func TestAppMetricsUsesTheNewestNonNullAppBlockID(t *testing.T) {
	const slug = "mixed"
	for _, tc := range []struct {
		name    string
		body    string
		want    string
		mustNot []string
	}{
		{
			// Two approved versions, both carrying a block id. Only recency
			// separates them, which is the property under test.
			name:    "two non-null rows",
			body:    metricsRowsBody(slug, [3]string{"0.2.0", "approved", "apb_newest"}, [3]string{"0.1.0", "approved", "apb_oldest"}),
			want:    "apb_newest",
			mustNot: []string{"apb_oldest"},
		},
		{
			// A null row BETWEEN two non-null ones: "the newest non-null" and
			// "the last non-null" differ, and so do "the row after the null" and
			// either of them.
			name: "a null row between two non-null rows",
			body: metricsRowsBody(slug,
				[3]string{"0.3.0", "approved", "apb_head"},
				[3]string{"0.2.0", "pending", ""},
				[3]string{"0.1.0", "approved", "apb_tail"}),
			want:    "apb_head",
			mustNot: []string{"apb_tail"},
		},
		{
			// The newest row is un-approved, so the scan must fall THROUGH it and
			// then still take the newer of the two remaining ids. This is the one
			// case the old fixture nearly covered — it stopped at the first
			// non-null and had no second one to get wrong.
			name: "a null newest row above two non-null rows",
			body: metricsRowsBody(slug,
				[3]string{"0.3.0", "pending", ""},
				[3]string{"0.2.0", "approved", "apb_second"},
				[3]string{"0.1.0", "approved", "apb_third"}),
			want:    "apb_second",
			mustNot: []string{"apb_third"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var rec metricsRec
			srv := metricsServer(t, tc.body, http.StatusOK,
				trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
			defer srv.Close()
			setupMetricsEnv(t, srv.URL)

			if _, _, err := run(t, "app", "metrics", slug); err != nil {
				t.Fatalf("app metrics: %v", err)
			}
			// REACHABILITY: the resolver must have reached the analytics query at
			// all, or "the input does not contain the older id" is trivially true.
			if !rec.trpcReached {
				t.Fatalf("the analytics query never ran, so the resolved appBlockId was never used; input=%q", rec.trpcInput)
			}
			// The id is written out by hand, not read back from the fixture row
			// the implementation happened to select.
			if want := `"appBlockId":"` + tc.want + `"`; !strings.Contains(rec.trpcInput, want) {
				t.Errorf("the analytics query was issued for the wrong block.\nwant substring: %s\ngot input: %s", want, rec.trpcInput)
			}
			for _, bad := range tc.mustNot {
				if strings.Contains(rec.trpcInput, `"appBlockId":"`+bad+`"`) {
					t.Errorf("the query used %q — an OLDER version's block id, so the dashboard would be real numbers "+
						"for a different version (#390).\ngot input: %s", bad, rec.trpcInput)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The ledger
// ---------------------------------------------------------------------------

// submissionsListReader is one non-test file that obtains the newest-first
// submissions list.
type submissionsListReader struct {
	// reads states, in words, WHICH part of the list this file consumes. It is
	// documentation for the next reader — the checked part is pinnedBy.
	reads string
	// pinnedBy names the behavioural test(s) proving that claim. More than one
	// when a file makes more than one independent pick.
	pinnedBy []string
	// exercises is a fragment that must appear LITERALLY in each pinning test's
	// own body, naming the command or API this file implements. Without it the
	// binding is satisfied by any test in the package that merely exists, and a
	// stale re-bind stays green — measured as a live hole in #384.
	exercises string
}

// submissionsListReaders is the ledger. Its key is `<package>/<file>.go`, since
// the walk spans two packages: internal/appapi holds a reader too, and #384's
// ledger walked only internal/cmd, which is why that site was outside it
// entirely.
var submissionsListReaders = map[string]submissionsListReader{
	"cmd/apps.go": {
		reads:     "the newest row whose blockId matches the slug (ownedSubmission); its status and deployState are printed verbatim",
		pinnedBy:  []string{"TestAppViewOwnedAdviceNamesTheNewestSubmission"},
		exercises: `run(t, "app", "view"`,
	},
	"cmd/app_metrics.go": {
		reads: "TWO independent picks: the newest non-null appBlockId, and the newest row's status for the not-approved advice",
		pinnedBy: []string{
			"TestAppMetricsUsesTheNewestNonNullAppBlockID",
			"TestAppMetricsAdviceNamesTheNewestSubmission",
		},
		exercises: `run(t, "app", "metrics"`,
	},
	"cmd/app_pull.go": {
		reads:     "the newest row's status for the not-approved advice (explainMissingApp)",
		pinnedBy:  []string{"TestAppPullAdviceNamesTheNewestSubmission"},
		exercises: `run(t, "app", "pull"`,
	},
	"cmd/app_status.go": {
		reads: "every row, and `--limit N` keeps the HEAD of the list — i.e. the newest N, which is what the flag's help promises",
		// Not a new test: `--limit` was already pinned against five rows with
		// distinct blockIds, asserting the two newest survive and the three
		// oldest do not. Ledgered so the coverage is visible from here rather
		// than rediscovered.
		pinnedBy:  []string{"TestAppStatusLimitTrimsTheTable"},
		exercises: `run(t, "app", "status", "--limit", "2"`,
	},
	"appapi/appblocks.go": {
		reads:     "the newest slug+version match, preferring a non-terminal row (latestMatchingSubmission), for the id `app submit` reports",
		pinnedBy:  []string{"TestRecoverTimedOutSubmitReadsTheNewestMatchingRow"},
		exercises: `SubmitVersion(context.Background()`,
	},
}

// countListSubmissionsRefs counts references to appapi's ListSubmissions in one
// Go source: a `.`-qualified use of the name, whether CALLED (`c.ListSubmissions(
// ctx, slug)`) or taken as a function value (`listSubmissions: client.ListSubmissions,`).
// Anything after a `//` is ignored.
//
// The `.` qualifier is what makes it a use rather than a declaration: it excludes
// both `func (c *Client) ListSubmissions(` and the interface's method line, which
// declare the API instead of consuming it. The trailing-character test excludes
// `appapi.ListSubmissionsCap`, a different exported name that shares the prefix
// and is `.`-qualified.
//
// Stated blind spot, in the under-reporting direction: like #384's scanner it
// strips from the first `//` regardless of string context, so a reference written
// inside a string literal after a `//` is invisible. No such shape exists in
// either package today, and the walk below asserts a floor so an
// everything-invisible scanner cannot read as "no drift".
func countListSubmissionsRefs(src string) int {
	const name = ".ListSubmissions"
	n := 0
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		for i := 0; ; {
			j := strings.Index(line[i:], name)
			if j < 0 {
				break
			}
			end := i + j + len(name)
			if end >= len(line) || !isIdentByte(line[end]) {
				n++
			}
			i = end
		}
	}
	return n
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// TestListSubmissionsRefScannerIsCalibrated validates the INSTRUMENT before the
// ledger reads a verdict from it, in both directions: a scanner that can never
// count and one that counts declarations too are each indistinguishable from a
// green ledger.
func TestListSubmissionsRefScannerIsCalibrated(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		// Positive controls: it can observe the two shapes that actually occur.
		{"a call", "subs, err := c.ListSubmissions(ctx, slug)\n", 1},
		{"a function value", "listSubmissions: client.ListSubmissions,\n", 1},
		{"two uses on separate lines", "a := c.ListSubmissions(ctx, \"\")\nb := d.ListSubmissions(ctx, s)\n", 2},
		// Negative controls: declarations are not uses.
		{"the method declaration", "func (c *Client) ListSubmissions(ctx context.Context, blockID string) ([]Submission, error) {\n", 0},
		{"an interface method line", "\tListSubmissions(ctx context.Context, blockID string) ([]Submission, error)\n", 0},
		// A different exported name that shares the prefix AND the dot.
		{"the row cap constant", "if n >= appapi.ListSubmissionsCap {\n", 0},
		{"the cap in a comparison chain", "return n >= appapi.ListSubmissionsCap\n", 0},
		{"a comment mentioning a call", "// c.ListSubmissions(ctx, slug) is newest-first\n", 0},
		{"a trailing comment beside code", "y := 1 // c.ListSubmissions(ctx, slug)\n", 0},
		{"an unrelated file", "package cmd\n\nfunc other() {}\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := countListSubmissionsRefs(tc.src); got != tc.want {
				t.Errorf("countListSubmissionsRefs(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

// submissionsLedgerDirs are the package directories the walk covers, keyed by the
// prefix used in the ledger. Both are needed: the reader in internal/appapi is
// the one #384's single-package walk could not see.
var submissionsLedgerDirs = map[string]string{
	"cmd":    ".",
	"appapi": "../appapi",
}

// submissionsLedgerFloors is the minimum number of non-test .go files each
// directory must yield. A mis-set working directory or a filter that matched
// nothing would otherwise report an empty scan as "no drift"; a floor near the
// real count (55 and 6 at the time of writing) is what makes a PARTIAL walk fail
// too. appapi's floor is loose because that package is small and actively
// changing.
var submissionsLedgerFloors = map[string]int{"cmd": 40, "appapi": 4}

// TestSubmissionsListReadersAreLedgered fails when the set of files reading the
// newest-first list grows or shrinks, and when a ledgered file's pinning test is
// missing or does not actually drive that file's surface.
//
// The grow direction is the one that matters: a new command reading this list
// inherits an assumption it cannot see, which is exactly how `app metrics` and
// then `app view` each acquired it.
func TestSubmissionsListReadersAreLedgered(t *testing.T) {
	found := map[string]int{}
	var tests strings.Builder
	for prefix, dir := range submissionsLedgerDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		scanned := 0
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") {
				continue
			}
			b, err := os.ReadFile(filepath.Clean(filepath.Join(dir, name)))
			if err != nil {
				t.Fatalf("reading %s/%s: %v", dir, name, err)
			}
			if strings.HasSuffix(name, "_test.go") {
				tests.Write(b)
				continue
			}
			scanned++
			if n := countListSubmissionsRefs(string(b)); n > 0 {
				found[prefix+"/"+name] = n
			}
		}
		if scanned < submissionsLedgerFloors[prefix] {
			t.Fatalf("only %d non-test .go files scanned in %s — the walk is wrong or partial, so an empty result means nothing",
				scanned, dir)
		}
	}
	// Positive control on the walk itself, separate from the file floor: a
	// scanner wired to nothing yields an empty set from a complete walk.
	if len(found) == 0 {
		t.Fatal("the scan found NO references to ListSubmissions at all — the ledger is wired to nothing")
	}

	for name := range found {
		if _, ok := submissionsListReaders[name]; !ok {
			t.Errorf("internal/%s reads the submissions list and is NOT in submissionsListReaders.\n"+
				"That list is documented newest-first and nothing client-side verifies it, so any single row this file "+
				"picks out of it is an untested assumption (#378, #390). Add it to the ledger together with a behavioural "+
				"case over a MULTI-ROW fixture whose rows differ on every field the case asserts (see twoRowOwnedBody).", name)
		}
	}
	for name, site := range submissionsListReaders {
		if _, ok := found[name]; !ok {
			t.Errorf("submissionsListReaders names internal/%s, which no longer references ListSubmissions. "+
				"If the read moved, move the ledger entry with it; if it went away, drop the entry and %s.",
				name, strings.Join(site.pinnedBy, " + "))
			continue
		}
		if len(site.pinnedBy) == 0 {
			t.Errorf("internal/%s is ledgered with NO pinning test — the entry documents a row pick and proves nothing", name)
			continue
		}
		for _, pin := range site.pinnedBy {
			body := testFuncBody(tests.String(), pin)
			if body == "" {
				t.Errorf("internal/%s is ledgered as pinned by %s, which is not a test in either scanned package — "+
					"the ledger names coverage that does not exist", name, pin)
				continue
			}
			// THE BINDING: the pinning test must drive THIS file's surface.
			// Existence alone leaves entries swappable, and a swapped ledger
			// actively misdirects — its shrink-direction message then names the
			// wrong test to delete.
			if !strings.Contains(body, site.exercises) {
				t.Errorf("internal/%s is ledgered as pinned by %s, but that test's body does not contain %s.\n"+
					"This binding requires the fragment LITERALLY in the pinning test's own body, so the ledger cannot be "+
					"satisfied by an unrelated test that merely exists. Either the entry names the wrong test (re-point it), "+
					"or the invocation moved into a helper (inline it, or update this entry's `exercises`).",
					name, pin, site.exercises)
			}
		}
	}
}
