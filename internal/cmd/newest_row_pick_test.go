package cmd

// THE NEWEST-FIRST ROW PICK, PINNED AT EVERY READER OF THE SUBMISSIONS ROUTE —
// issue #390.
//
// GET /api/v1/blocks/submissions returns the caller's submissions NEWEST FIRST,
// and six non-test files read it. Four of them picked ONE row out of that list
// and pinned nothing about WHICH end they read, because every fixture they had
// was a single row — where `subs[0]` and `subs[len(subs)-1]` are the same row, so
// the choice cannot be observed. Measured by reversing each pick ALONE:
//
//	site                                         mutated to           result
//	internal/cmd/apps.go ownedSubmission         last matching row    3786 RUN, 0 FAIL
//	internal/cmd/app_metrics.go appBlockId scan  last non-null row    3786 RUN, 0 FAIL
//	internal/appapi/appblocks.go latestMatching  last matching row    3786 RUN, 0 FAIL
//	internal/appapi/appblocks.go GetSubmission   Submissions[len-1]   3812 RUN, 0 FAIL
//
// The first three were measured on d9f9c67; the fourth on the merged tree that
// had already fixed them, which is the point — it was found by the adversarial
// audit of the PR that fixed the other three, and it is why this ledger's
// predicate is what it is. The first version of this file keyed on a
// `.ListSubmissions` reference: the predicate that was easy to grep, not the
// property that matters. `GetSubmission` reaches the SAME route and picks
// `Submissions[0]` out of the SAME newest-first list while making no
// `ListSubmissions` reference at all, so the ledger was blind to it BY
// CONSTRUCTION — and `internal/cmd/app_listing.go`, which consumes that row to
// resolve the listing every `app listing` subcommand mutates, was in no ledger at
// all.
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
// THE ROW — a reference to any accessor of the submissions ROUTE. #384 rejected a
// scanner over the row-pick SHAPE, and that judgement still stands: any pattern
// matching `subs[0]` turns on a variable name a rename defeats. The accessors are
// exported method names, and the scanner also catches one taken as a FUNCTION
// VALUE (`listSubmissions: client.ListSubmissions`), which is how `app pull` and
// `app metrics` actually reach the list.
//
// 🔴 AND THE ACCESSOR SET IS DERIVED FROM THE CODE, NOT HAND-PICKED — that is the
// repair for the blind spot above, and what stops this from recurring a third
// time. A hand-written list of names has exactly the failure the audit found: it
// covers the accessors somebody thought of. Instead:
//
//   - Every accessor must build its URL through `submissionsURL`, and
//     TestSubmissionsRouteHasOneGateway asserts `SubmissionsPath` is referenced
//     nowhere else — so a new accessor cannot reach the route around the side.
//   - TestSubmissionsRouteAccessorsAreLedgered DERIVES the accessor set as "every
//     appapi func referencing submissionsURL" and fails if it differs from
//     submissionsRouteAccessors. A third accessor therefore forces a ledger
//     decision on the day it is written, rather than waiting for an audit.
//   - Only then does the reader walk run, over that derived-and-checked set.
//
// 🔴 WHAT IT STILL CANNOT SEE — MEASURED, NOT ESTIMATED. An earlier draft claimed
// the gateway assertion narrows the blind spot to "a new route". It does not, and
// an audit found three SAME-ROUTE evasions; two are now closed and the rest are
// named here rather than implied away.
//
// Closed since: the gateway taken as a METHOD VALUE (`mk := c.submissionsURL`) —
// the derivation matches the bare identifier, not `submissionsURL(`; a helper
// between the gateway and the accessor — the derivation follows unexported chains
// and reports the exported boundary; a CMD-package file using the exported
// `appapi.SubmissionsPath`, or the path as a whole string literal, with raw http —
// the gateway scan covers both ledgered packages and matches the quoted literal.
//
// STILL OPEN, and cheap to reach on purpose: a path assembled from FRAGMENTS
// (`"/api/v1/blocks/" + "submissions"`), reflection, or a submission obtained
// without touching this route at all — handed over by another function, cached, or
// read from a new route. The last is the visible, deliberate act; the first is
// not, and no line-based scanner closes it. What bounds the damage is that all of
// them still have to PICK a row, and the four behavioural cases above pin the
// picks that exist today.
//
// The two ledgers therefore overlap by design and answer different questions:
// #384's asks "does every caller of the shared advice have a case proving it
// describes the newest row?", this one asks "does every reader of a newest-first
// list declare, and pin, which end it reads?". app_pull.go and app_metrics.go
// appear in both.
//
// 🔴 WHAT NEITHER LEDGER CHECKS, AND — PRECISELY — WHY. Every guard here asserts
// what the CLI does with a list the TEST SERVER hands back in newest-first order.
// Nothing client-side verifies that order today: `ListSubmissions` does not sort
// and no caller inspects the sequence, so if the server's ordering changed, every
// test in this file would stay green while every message went wrong together.
//
// That is a statement about what the CLI DOES, not about what it COULD do, and an
// earlier version of this comment got that wrong — it claimed "the route sends no
// field these guards could check an ordering against" and then, six lines later,
// proposed sorting on `submittedAt`. Both cannot be true. Every row carries
// `SubmittedAt` (plus `CreatedAt`/`UpdatedAt`), so a client-side sort, or an
// assertion that the returned list IS ordered, is entirely possible. It is
// UNDONE, not impossible — a behaviour change needing its own decision, because
// `app status --limit` and the cap caveat both inherit the server's order today,
// and because it would move the CLI from reporting the server's answer to
// overriding it.
//
// The genuinely unverifiable half is narrower and worth keeping separate: whether
// the SERVER's intended recency order is the one `submittedAt` expresses. A
// resubmission's `submittedAt` is the CLI's inference about which row is newer,
// not a guarantee the route makes. Conflating "we have not checked" with "we
// cannot check" is how a closeable residual stays open forever, so: the CLI's own
// half is pinned here; the ordering check is unwritten and could be written; the
// server's intent behind the order is the part no client can settle.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
//
// Scope of the guard, stated so it is not read as wider than it is: it covers the
// five fields the caller supplies. `blockId` is shared by construction (both rows
// describe the SAME app — that is the point) and `liveUrl` is hard-coded null
// here, so neither discriminates in a fixture from this builder. The
// ownedSubmissionBody const does differ on liveUrl; this builder does not.
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
func metricsRowsBody(t *testing.T, slug string, rows ...[3]string) string {
	t.Helper()
	// Fixture control, matching twoRowOwnedBody's: the NON-NULL appBlockIds are
	// what every assertion turns on, so two rows carrying the same one would
	// collapse the ends exactly as a single row does. Versions are checked too,
	// since they are what a reader uses to see which row is which.
	seenID, seenVer := map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		if r[2] != "" && seenID[r[2]] {
			t.Fatalf("two rows carry appBlockId %q — the ends of this fixture are indistinguishable "+
				"on the field this test asserts (#390)", r[2])
		}
		if seenVer[r[0]] {
			t.Fatalf("two rows carry version %q — the rows are not separable", r[0])
		}
		seenID[r[2]], seenVer[r[0]] = r[2] != "", true
	}
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
			body:    metricsRowsBody(t, slug, [3]string{"0.2.0", "approved", "apb_newest"}, [3]string{"0.1.0", "approved", "apb_oldest"}),
			want:    "apb_newest",
			mustNot: []string{"apb_oldest"},
		},
		{
			// A null row BETWEEN two non-null ones: "the newest non-null" and
			// "the last non-null" differ, and so do "the row after the null" and
			// either of them.
			name: "a null row between two non-null rows",
			body: metricsRowsBody(t, slug,
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
			body: metricsRowsBody(t, slug,
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

// submissionsPin binds one behavioural test to the site it proves.
type submissionsPin struct {
	// test is the behavioural test's name.
	test string
	// exercises is a fragment that must appear LITERALLY in that test's own
	// body, naming the command or API the site implements. Without it the
	// binding is satisfied by any test in the package that merely exists, and a
	// stale re-bind stays green — measured as a live hole in #384.
	//
	// 🔴 It is per-PIN, not per-file, and that is not cosmetic: a file with two
	// independent picks has two tests driving two DIFFERENT surfaces, so one
	// shared fragment could only be something weak enough to match both — which
	// is precisely the "satisfied by any test that exists" hole this field
	// exists to close.
	exercises string
}

// submissionsListReader is one non-test file that reads the submissions route.
type submissionsListReader struct {
	// reads states, in words, WHICH row(s) this file consumes and what it does
	// with them. It is documentation for the next reader — the CHECKED part is
	// pinnedBy — so it must name EVERY independent read in the file, not just
	// the one that prompted the entry.
	reads string
	// pinnedBy names the behavioural test(s) proving that claim: one per
	// independent pick, each bound to its own surface.
	pinnedBy []submissionsPin
}

// submissionsListReaders is the ledger. Its key is `<package>/<file>.go`, since
// the walk spans two packages: internal/appapi holds readers too, and #384's
// ledger walked only internal/cmd, which is why those sites were outside it
// entirely.
var submissionsListReaders = map[string]submissionsListReader{
	"cmd/apps.go": {
		reads: "the newest row whose blockId matches the slug (ownedSubmission); its status and deployState are printed verbatim",
		pinnedBy: []submissionsPin{
			{"TestAppViewOwnedAdviceNamesTheNewestSubmission", `run(t, "app", "view"`},
		},
	},
	"cmd/app_metrics.go": {
		reads: "TWO independent picks: the newest non-null appBlockId, and the newest row's status for the not-approved advice",
		pinnedBy: []submissionsPin{
			{"TestAppMetricsUsesTheNewestNonNullAppBlockID", `run(t, "app", "metrics"`},
			{"TestAppMetricsAdviceNamesTheNewestSubmission", `run(t, "app", "metrics"`},
		},
	},
	"cmd/app_pull.go": {
		reads: "the newest row's status for the not-approved advice (explainMissingApp)",
		pinnedBy: []submissionsPin{
			{"TestAppPullAdviceNamesTheNewestSubmission", `run(t, "app", "pull"`},
		},
	},
	"cmd/app_status.go": {
		reads: "TWO independent reads: the LIST view keeps every row and `--limit N` trims the HEAD (the newest N, which is what the flag help promises); " +
			"the DETAIL view takes GetSubmission's single row and prints its version, status and deploy state",
		pinnedBy: []submissionsPin{
			// `--limit` was already pinned against five rows with distinct
			// blockIds; ledgered so that coverage is visible from here rather
			// than rediscovered. The detail view had no multi-row case until the
			// audit of #390 found GetSubmission unpinned.
			{"TestAppStatusLimitTrimsTheTable", `run(t, "app", "status", "--limit", "2"`},
			{"TestAppStatusDetailShowsTheNewestSubmission", `run(t, "app", "status", slug)`},
		},
	},
	"cmd/app_listing.go": {
		reads: "GetSubmission's single row, for the appBlockId resolveListing hands to GetMyListingForApp — i.e. WHICH listing every `app listing` subcommand reads and mutates",
		pinnedBy: []submissionsPin{
			// Found by the adversarial audit of the PR that fixed the other
			// three sites: the previous predicate (a `.ListSubmissions`
			// reference) could not see this file at all, because it reaches the
			// route through GetSubmission.
			{"TestAppListingResolvesTheNewestSubmissionsBlock", `run(t, "app", "listing", "status"`},
		},
	},
	"appapi/appblocks.go": {
		reads: "THREE independent picks: latestMatchingSubmission takes the newest slug+version match preferring a non-terminal row (the id `app submit` reports), " +
			"GetSubmission's `?blockId=` fallback takes Submissions[0] out of the narrowed list, " +
			"and GetSubmissionRows hands that WHOLE narrowed list back to its caller in the order it was read — the order itself is the value returned",
		pinnedBy: []submissionsPin{
			{"TestRecoverTimedOutSubmitReadsTheNewestMatchingRow", `SubmitVersion(context.Background()`},
			{"TestGetSubmissionByBlockIDTakesTheNewestRow", `c.GetSubmission(context.Background()`},
			// #413 delta-audit finding 1: the entry above already named
			// GetSubmissionRows as an ACCESSOR, but nothing pinned the order it
			// promises — reversing the returned slice left the suite green,
			// because its only consumer (highestApprovedVersion) is a maximum
			// and does not care which end it starts from.
			{"TestGetSubmissionRowsHandsBackTheRowsInServedOrder", `c.GetSubmissionRows(context.Background()`},
		},
	},
}

// submissionsRouteGateway is the ONE method every accessor of the submissions
// route must build its URL through. It is the anchor the accessor set is derived
// from, so an accessor cannot exist without being discoverable.
const submissionsRouteGateway = "submissionsURL"

// submissionsRouteAccessors is the ledgered set of appapi methods that talk to
// the submissions route. It is ASSERTED against the set derived from the code
// (TestSubmissionsRouteAccessorsAreLedgered), never trusted on its own — a
// hand-written name list is exactly what let GetSubmission hide from the first
// version of this ledger.
// 🔴 THREE NOW. GetSubmissionRows is the same route read as GetSubmission, with
// the narrowed listing handed back instead of discarded — `app status <slug>`
// takes the rows so the drift check does not re-issue the identical GET (#413).
// Both exported spellings are ledgered because both hand a caller a row out of a
// list documented newest-first: GetSubmission picks Submissions[0], and
// GetSubmissionRows exposes the ordering itself to its caller. Their shared body
// is deliberately UNEXPORTED so the derivation below reports both boundaries
// rather than only whichever one still touches the gateway directly.
var submissionsRouteAccessors = []string{"GetSubmission", "GetSubmissionRows", "ListSubmissions"}

// funcDecl matches a top-level func declaration, with or without a receiver, and
// captures the func's own name.
var funcDecl = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

// enclosingFuncsReferencing returns the names of the top-level funcs whose
// bodies mention needle, ignoring anything after a `//`. A reference outside any
// func (a const or var initialiser) is reported as "" so the caller can see it
// rather than silently attributing it to the preceding func.
//
// It is line-based, not AST-based, and its assumptions are gofmt's: a top-level
// declaration starts at column 0. That is enough here because both scanned
// packages are gofmt-clean (CI's build-test job runs `gofmt -s -l .`), and the
// calibration battery below pins the shapes that would otherwise mis-attribute.
func enclosingFuncsReferencing(src, needle string) []string {
	var out []string
	seen := map[string]bool{}
	cur := ""
	for _, line := range strings.Split(src, "\n") {
		if m := funcDecl.FindStringSubmatch(line); m != nil {
			cur = m[1]
		} else if len(line) > 0 && !isSpaceByte(line[0]) && !strings.HasPrefix(line, "}") && !strings.HasPrefix(line, "//") {
			// A top-level declaration that is NOT a func (const/var/type) ends
			// the previous func's scope. Without this a `const X = SubmissionsPath`
			// after a func would be attributed to that func.
			cur = ""
		}
		code := line
		if i := strings.Index(code, "//"); i >= 0 {
			code = code[:i]
		}
		if strings.Contains(code, needle) && !seen[cur] {
			seen[cur] = true
			out = append(out, cur)
		}
	}
	sort.Strings(out)
	return out
}

func isSpaceByte(b byte) bool { return b == ' ' || b == '\t' }

// referencesIdent reports whether code mentions name as a WHOLE identifier —
// with a non-identifier byte on each side — so `submissionsURL` does not match
// `mySubmissionsURLThing`, and a METHOD VALUE (`mk := c.submissionsURL`, no
// parenthesis) matches just as a call does. Matching the bare identifier rather
// than `name(` is deliberate: requiring the parenthesis let a method value slip
// past the derivation entirely.
func referencesIdent(code, name string) bool {
	for i := 0; ; {
		j := strings.Index(code[i:], name)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(name)
		beforeOK := start == 0 || !isIdentByte(code[start-1])
		afterOK := end >= len(code) || !isIdentByte(code[end])
		if beforeOK && afterOK {
			return true
		}
		i = end
	}
}

// topLevelFuncBodies maps each top-level func in src to its body, comments
// stripped and its own declaration line excluded. Line-based, on gofmt's
// guarantee that a top-level declaration starts at column 0.
func topLevelFuncBodies(src string) map[string]string {
	out := map[string]string{}
	cur := ""
	var b strings.Builder
	flush := func() {
		if cur != "" {
			out[cur] += b.String()
		}
		b.Reset()
	}
	for _, line := range strings.Split(src, "\n") {
		if m := funcDecl.FindStringSubmatch(line); m != nil {
			flush()
			cur = m[1]
			if _, ok := out[cur]; !ok {
				out[cur] = ""
			}
			continue // the decl line is not part of the body
		}
		if len(line) > 0 && !isSpaceByte(line[0]) && !strings.HasPrefix(line, "}") && !strings.HasPrefix(line, "//") {
			flush()
			cur = ""
			continue
		}
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	flush()
	return out
}

func isExportedName(s string) bool { return s != "" && s[0] >= 'A' && s[0] <= 'Z' }

// deriveSubmissionsRouteAccessors returns the EXPORTED boundary funcs that reach
// the route gateway, following chains of UNEXPORTED helpers and stopping at the
// first exported func.
//
// 🔴 "THE FUNCS THAT CALL THE GATEWAY" IS THE WRONG RULE, AND IT FAILED IN THE
// DIRECTION THAT DISARMS THE GUARD. Put one private helper between the gateway
// and an accessor — a one-step refactor —
//
//	func (c *Client) subsEndpoint(id, b string) string { return c.submissionsURL(id, b) }
//	func (c *Client) LatestSubmission(...)             { u := c.subsEndpoint("", slug) … }
//
// and the direct-call rule derives {subsEndpoint}: it fails loudly, but names the
// HELPER. A maintainer doing exactly what the message said — ledger subsEndpoint —
// went green with LatestSubmission unledgered and cmd files picking Submissions[0]
// off it, because the reader walk only looks for names in the accessor set. The
// correct remediation was the one the guard rejected. Following the chain and
// reporting the exported boundary names LatestSubmission instead, which is both
// the accessor and the name the reader walk needs.
//
// Stopping AT the first exported func is what keeps the set meaningful: without
// it, recoverTimedOutSubmit → SubmitVersion and everything above them would join,
// and an "accessor set" containing half the package pins nothing.
func deriveSubmissionsRouteAccessors(bodies map[string]string) []string {
	reached := map[string]bool{submissionsRouteGateway: true}
	frontier := []string{submissionsRouteGateway}
	for len(frontier) > 0 {
		g := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		// Only the gateway and UNEXPORTED links propagate; an exported func is a
		// boundary, not a conduit.
		if g != submissionsRouteGateway && isExportedName(g) {
			continue
		}
		for fn, body := range bodies {
			if reached[fn] || fn == g {
				continue
			}
			if referencesIdent(body, g) {
				reached[fn] = true
				frontier = append(frontier, fn)
			}
		}
	}
	var out []string
	for fn := range reached {
		if fn != submissionsRouteGateway && isExportedName(fn) {
			out = append(out, fn)
		}
	}
	sort.Strings(out)
	return out
}

// TestDeriveSubmissionsRouteAccessorsIsCalibrated is the instrument control for
// the derivation, including the indirection that defeated its first version.
func TestDeriveSubmissionsRouteAccessorsIsCalibrated(t *testing.T) {
	direct := topLevelFuncBodies("package appapi\n\n" +
		"func (c *Client) submissionsURL(id, b string) string {\n\treturn c.BaseURL + SubmissionsPath\n}\n\n" +
		"func (c *Client) ListSubmissions(ctx context.Context, b string) ([]Submission, error) {\n\tu := c.submissionsURL(\"\", b)\n\treturn nil, nil\n}\n\n" +
		"func (c *Client) Unrelated() error {\n\treturn nil\n}\n")
	if got := deriveSubmissionsRouteAccessors(direct); !equalStrings(got, []string{"ListSubmissions"}) {
		t.Errorf("direct call: derived %v, want [ListSubmissions]", got)
	}

	// 🔴 THE CASE THAT DISARMED THE GUARD: one private helper in between. The
	// derivation must name the EXPORTED accessor, never the helper.
	indirect := topLevelFuncBodies("package appapi\n\n" +
		"func (c *Client) submissionsURL(id, b string) string {\n\treturn c.BaseURL + SubmissionsPath\n}\n\n" +
		"func (c *Client) subsEndpoint(id, b string) string {\n\treturn c.submissionsURL(id, b)\n}\n\n" +
		"func (c *Client) LatestSubmission(ctx context.Context, s string) (*Submission, error) {\n\tu := c.subsEndpoint(\"\", s)\n\treturn nil, nil\n}\n")
	got := deriveSubmissionsRouteAccessors(indirect)
	if !equalStrings(got, []string{"LatestSubmission"}) {
		t.Errorf("through a private helper: derived %v, want [LatestSubmission] — the guard must name the ACCESSOR, "+
			"not the gateway-adjacent helper, or its own remediation advice disarms it", got)
	}

	// A METHOD VALUE is a reference too — requiring `name(` let this slip past.
	methodValue := topLevelFuncBodies("package appapi\n\n" +
		"func (c *Client) submissionsURL(id, b string) string {\n\treturn \"\"\n}\n\n" +
		"func (c *Client) PeekSubmissions(ctx context.Context) error {\n\tmk := c.submissionsURL\n\t_ = mk\n\treturn nil\n}\n")
	if got := deriveSubmissionsRouteAccessors(methodValue); !equalStrings(got, []string{"PeekSubmissions"}) {
		t.Errorf("method value: derived %v, want [PeekSubmissions]", got)
	}

	// STOPPING AT THE EXPORTED BOUNDARY: a caller of an exported accessor is not
	// itself an accessor, or the set swells until it pins nothing.
	chain := topLevelFuncBodies("package appapi\n\n" +
		"func (c *Client) submissionsURL(id, b string) string {\n\treturn \"\"\n}\n\n" +
		"func (c *Client) ListSubmissions(ctx context.Context, b string) error {\n\tu := c.submissionsURL(\"\", b)\n\treturn nil\n}\n\n" +
		"func (c *Client) SubmitVersion(ctx context.Context) error {\n\treturn c.ListSubmissions(ctx, \"\")\n}\n")
	if got := deriveSubmissionsRouteAccessors(chain); !equalStrings(got, []string{"ListSubmissions"}) {
		t.Errorf("chain: derived %v, want [ListSubmissions] — an exported accessor is a boundary, not a conduit", got)
	}

	// Positive control: the derivation can come back empty.
	if got := deriveSubmissionsRouteAccessors(topLevelFuncBodies("package appapi\n\nfunc lonely() {}\n")); len(got) != 0 {
		t.Errorf("a package that never reaches the gateway derived %v", got)
	}
}

// countSubmissionsRouteRefs counts references to any of the route's accessors in
// one Go source: a `.`-qualified use of the name, whether CALLED
// (`c.ListSubmissions(ctx, slug)`) or taken as a function value
// (`listSubmissions: client.ListSubmissions,`). Anything after a `//` is ignored.
//
// The `.` qualifier is what makes it a use rather than a declaration: it excludes
// both `func (c *Client) ListSubmissions(` and the interface's method lines,
// which declare the API instead of consuming it. The trailing-character test
// excludes `appapi.ListSubmissionsCap`, a different exported name that shares a
// prefix and is `.`-qualified.
//
// Stated blind spot, in the under-reporting direction: like #384's scanner it
// strips from the first `//` regardless of string context, so a reference written
// inside a string literal after a `//` is invisible. No such shape exists in
// either package today, and the walk below asserts a floor so an
// everything-invisible scanner cannot read as "no drift".
func countSubmissionsRouteRefs(src string, accessors []string) int {
	n := 0
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		for _, a := range accessors {
			name := "." + a
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
	}
	return n
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// TestSubmissionsRouteRefScannerIsCalibrated validates the INSTRUMENT before the
// ledger reads a verdict from it, in both directions: a scanner that can never
// count and one that counts declarations too are each indistinguishable from a
// green ledger.
func TestSubmissionsRouteRefScannerIsCalibrated(t *testing.T) {
	acc := []string{"GetSubmission", "ListSubmissions"}
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		// Positive controls: it can observe the shapes that actually occur.
		{"a call", "subs, err := c.ListSubmissions(ctx, slug)\n", 1},
		{"a function value", "listSubmissions: client.ListSubmissions,\n", 1},
		{"the OTHER accessor", "sub, err := client.GetSubmission(ctx, \"\", slug)\n", 1},
		{"two uses on separate lines", "a := c.ListSubmissions(ctx, \"\")\nb := d.GetSubmission(ctx, i, s)\n", 2},
		// Negative controls: declarations are not uses.
		{"the method declaration", "func (c *Client) ListSubmissions(ctx context.Context, blockID string) ([]Submission, error) {\n", 0},
		{"an interface method line", "\tGetSubmission(ctx context.Context, id, blockID string) (*Submission, error)\n", 0},
		// A different exported name that shares a prefix AND the dot.
		{"the row cap constant", "if n >= appapi.ListSubmissionsCap {\n", 0},
		{"a comment mentioning a call", "// c.GetSubmission(ctx, id, slug) takes the first row\n", 0},
		{"a trailing comment beside code", "y := 1 // c.ListSubmissions(ctx, slug)\n", 0},
		{"an unrelated file", "package cmd\n\nfunc other() {}\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := countSubmissionsRouteRefs(tc.src, acc); got != tc.want {
				t.Errorf("countSubmissionsRouteRefs(%q) = %d, want %d", tc.src, got, tc.want)
			}
		})
	}
}

// TestEnclosingFuncsReferencingIsCalibrated is the instrument control for the
// accessor DERIVATION. An extractor that attributed a reference to the wrong func
// would ledger the wrong accessor set, and one that found nothing would make both
// derivation tests below pass vacuously.
func TestEnclosingFuncsReferencingIsCalibrated(t *testing.T) {
	const src = "package appapi\n\n" +
		"func (c *Client) submissionsURL(id, blockID string) string {\n\tu := c.BaseURL + SubmissionsPath\n\treturn u\n}\n\n" +
		"func (c *Client) ListSubmissions(ctx context.Context, b string) ([]Submission, error) {\n\turl := c.submissionsURL(\"\", b)\n\treturn nil, nil\n}\n\n" +
		"func unrelated() string {\n\treturn \"x\"\n}\n"

	if got := enclosingFuncsReferencing(src, "submissionsURL"); !equalStrings(got, []string{"ListSubmissions", "submissionsURL"}) {
		// submissionsURL matches its own declaration line; the caller filters it.
		t.Errorf("derivation = %v, want [ListSubmissions submissionsURL]", got)
	}
	// A func that does NOT reference the needle must not be attributed.
	for _, bad := range enclosingFuncsReferencing(src, "submissionsURL") {
		if bad == "unrelated" {
			t.Error("a func with no reference was attributed the needle")
		}
	}
	// Positive control: the extractor can come back empty for a real absence.
	if got := enclosingFuncsReferencing(src, "NoSuchIdentifier"); len(got) != 0 {
		t.Errorf("a needle that does not occur returned %v", got)
	}
	// 🔴 The mis-attribution shape: a top-level declaration AFTER a func must end
	// that func's scope, or a const holding the needle is credited to the
	// preceding func and the derived set silently gains a member.
	const trailing = "package appapi\n\nfunc alpha() {\n\tx := 1\n}\n\nconst other = SubmissionsPath\n"
	got := enclosingFuncsReferencing(trailing, "SubmissionsPath")
	for _, g := range got {
		if g == "alpha" {
			t.Errorf("a top-level const referencing the needle was attributed to the preceding func: %v", got)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// submissionsPkgSources returns every non-test .go source in one of the ledgered
// package directories, keyed by file name, with the CHECKED floor so an empty or
// partial read cannot pass as "nothing found".
func submissionsPkgSources(t *testing.T, prefix string) map[string]string {
	t.Helper()
	dir, ok := submissionsLedgerDirs[prefix]
	if !ok {
		t.Fatalf("no ledgered directory for %q", prefix)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Clean(filepath.Join(dir, name)))
		if err != nil {
			t.Fatalf("reading %s/%s: %v", dir, name, err)
		}
		out[name] = string(b)
	}
	// 🔴 THE CHECKED FLOOR, NOT A RAW MAP READ. A raw `floors[prefix]` yields 0 on
	// a missing key, so `len(out) < 0` never fires and this positive control is
	// silently off — the same shape as the walk's floor, surviving at a SECOND
	// call site. Measured before the fix: with the key dropped and the directory
	// pointed at a .go-free path, TestSubmissionsRouteHasOneGateway PASSED over a
	// completely empty scan, and that test is the chokepoint with no backup.
	if floor := submissionsLedgerFloorFor(t, prefix); len(out) < floor {
		t.Fatalf("only %d non-test .go files found in %s (floor %d) — the read is wrong or partial, "+
			"so an empty result means nothing", len(out), dir, floor)
	}
	return out
}

// submissionsRoutePathLiteral is the route path written as a WHOLE string
// literal, quotes included. The gateway check matches both the SubmissionsPath
// identifier and this, because grepping only the identifier let a func spell the
// path out instead.
//
// 🔴 THE QUOTES ARE LOAD-BEARING, AND MATCHING THE BARE PATH DOES NOT WORK. The
// route path appears inside longer strings all over both packages for entirely
// correct reasons — `"unexpected /api/v1/blocks/submissions response: %s"` in
// both accessors, and the `GET /api/v1/blocks/submissions` in a Cobra Long
// description — so a bare-substring match flags three legitimate sites and the
// guard becomes noise. Requiring the path to BE the whole literal separates
// "names the route in a message" from "builds a URL to the route".
const submissionsRoutePathLiteral = `"/api/v1/blocks/submissions"`

// TestSubmissionsRouteHasOneGateway asserts that the submissions route is reached
// ONLY through submissionsURL — scanning BOTH packages, and matching the path
// identifier AND the literal.
//
// 🔴 THIS IS WHAT MAKES THE DERIVED ACCESSOR SET COMPLETE. Deriving accessors as
// "funcs that reach submissionsURL" is only sound while submissionsURL is the sole
// way to the route; a func that assembled the URL itself would be an accessor the
// derivation cannot see — the same blind spot as before, one level down.
//
// Its scope was originally narrower than its claim, so both gaps are closed here:
// scanning only internal/appapi missed a cmd-package file using the EXPORTED
// appapi.SubmissionsPath with raw http, and matching only the identifier missed a
// path written as a string literal.
func TestSubmissionsRouteHasOneGateway(t *testing.T) {
	var offenders []string
	for prefix := range submissionsLedgerDirs {
		for name, src := range submissionsPkgSources(t, prefix) {
			for _, needle := range []string{"SubmissionsPath", submissionsRoutePathLiteral} {
				for _, fn := range enclosingFuncsReferencing(src, needle) {
					if fn == submissionsRouteGateway {
						continue
					}
					// The const's own declaration sits outside any func, and it
					// necessarily holds both needles.
					if fn == "" && strings.Contains(src, "const SubmissionsPath") {
						continue
					}
					offenders = append(offenders, prefix+"/"+name+":"+fn)
				}
			}
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("the submissions route is reached outside %s by %v.\n"+
			"The accessor set this ledger derives is 'every exported func that reaches %s', so code reaching the route "+
			"another way is an accessor the derivation CANNOT see — which is how GetSubmission hid from the first "+
			"version of this ledger. Route it through the gateway, or widen the derivation deliberately and say so here.",
			submissionsRouteGateway, offenders, submissionsRouteGateway)
	}
}

// TestSubmissionsRouteAccessorsAreLedgered derives the accessor set from the code
// and fails when it differs from submissionsRouteAccessors, in BOTH directions. A
// new accessor therefore forces a ledger decision on the day it is written
// instead of waiting for an audit to find it.
func TestSubmissionsRouteAccessorsAreLedgered(t *testing.T) {
	bodies := map[string]string{}
	for name, src := range submissionsPkgSources(t, "appapi") {
		for fn, body := range topLevelFuncBodies(src) {
			bodies[fn] += body
			_ = name
		}
	}
	derived := deriveSubmissionsRouteAccessors(bodies)
	// Positive control before comparing: a derivation wired to nothing would
	// otherwise agree with any ledger that happened to be empty.
	if len(derived) == 0 {
		t.Fatalf("derived NO accessors of the submissions route — the derivation is wired to nothing, "+
			"so agreement with %v would mean nothing", submissionsRouteAccessors)
	}
	inDerived := map[string]bool{}
	for _, fn := range derived {
		inDerived[fn] = true
	}
	ledgered := map[string]bool{}
	for _, a := range submissionsRouteAccessors {
		ledgered[a] = true
	}
	for _, fn := range derived {
		if !ledgered[fn] {
			t.Errorf("appapi.%s reaches the submissions route and is NOT in submissionsRouteAccessors.\n"+
				"It is the EXPORTED boundary that hands a caller a row out of a list documented newest-first — add "+
				"THAT name here (not any private helper it goes through, which the reader walk cannot search for) "+
				"AND re-run the reader walk, because a new accessor turns previously-invisible files into readers (#390).", fn)
		}
	}
	for fn := range ledgered {
		if !inDerived[fn] {
			t.Errorf("submissionsRouteAccessors names appapi.%s, which no longer reaches the submissions route "+
				"through %s. If it was renamed, rename it here; if it went away, drop it.", fn, submissionsRouteGateway)
		}
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
//
// 🔴 A MISSING KEY IS A FAILURE, NOT A ZERO. Read as `floors[prefix]` a directory
// with no entry yields 0, and `scanned < 0` never fires — so adding a directory
// to submissionsLedgerDirs and forgetting its floor silently turns that
// directory's positive control OFF, in exactly the direction the floor exists to
// prevent. submissionsLedgerFloorFor fails instead.
var submissionsLedgerFloors = map[string]int{"cmd": 40, "appapi": 4}

func submissionsLedgerFloorFor(t *testing.T, prefix string) int {
	t.Helper()
	floor, ok := submissionsLedgerFloors[prefix]
	if !ok {
		t.Fatalf("submissionsLedgerDirs has %q but submissionsLedgerFloors does not — without a floor its "+
			"positive control is off, and an empty or partial scan of that directory would read as 'no drift'", prefix)
	}
	if floor < 1 {
		t.Fatalf("the floor for %q is %d — a floor below 1 cannot fail, so it is not a control", prefix, floor)
	}
	return floor
}

// TestSubmissionsListReadersAreLedgered fails when the set of files reading the
// submissions route grows or shrinks, and when a ledgered file's pinning test is
// missing or does not actually drive that file's surface.
//
// The grow direction is the one that matters: a new command reading this route
// inherits an assumption it cannot see, which is how `app metrics`, then
// `app view`, then `app listing` each acquired it.
func TestSubmissionsListReadersAreLedgered(t *testing.T) {
	found := map[string]int{}
	var tests strings.Builder
	for prefix, dir := range submissionsLedgerDirs {
		floor := submissionsLedgerFloorFor(t, prefix)
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
			if n := countSubmissionsRouteRefs(string(b), submissionsRouteAccessors); n > 0 {
				found[prefix+"/"+name] = n
			}
		}
		if scanned < floor {
			t.Fatalf("only %d non-test .go files scanned in %s (floor %d) — the walk is wrong or partial, so an empty result means nothing",
				scanned, dir, floor)
		}
	}
	// Positive control on the walk itself, separate from the file floor: a
	// scanner wired to nothing yields an empty set from a complete walk.
	if len(found) == 0 {
		t.Fatal("the scan found NO references to any submissions-route accessor — the ledger is wired to nothing")
	}

	for name := range found {
		if _, ok := submissionsListReaders[name]; !ok {
			t.Errorf("internal/%s reads the submissions route and is NOT in submissionsListReaders.\n"+
				"That route answers with a list documented newest-first, and nothing client-side verifies the order, so any "+
				"single row this file takes out of it is an untested assumption (#378, #390). Add it to the ledger together "+
				"with a behavioural case over a MULTI-ROW fixture whose rows differ on every field the case asserts "+
				"(see twoRowOwnedBody).", name)
		}
	}
	for name, site := range submissionsListReaders {
		if _, ok := found[name]; !ok {
			t.Errorf("submissionsListReaders names internal/%s, which no longer references a submissions-route accessor. "+
				"If the read moved, move the ledger entry with it; if it went away, drop the entry and its pinning tests.", name)
			continue
		}
		if len(site.pinnedBy) == 0 {
			t.Errorf("internal/%s is ledgered with NO pinning test — the entry documents a row pick and proves nothing", name)
			continue
		}
		for _, pin := range site.pinnedBy {
			body := testFuncBody(tests.String(), pin.test)
			if body == "" {
				t.Errorf("internal/%s is ledgered as pinned by %s, which is not a test in either scanned package — "+
					"the ledger names coverage that does not exist", name, pin.test)
				continue
			}
			// THE BINDING: the pinning test must drive THIS file's surface.
			// Existence alone leaves entries swappable, and a swapped ledger
			// actively misdirects — its shrink-direction message then names the
			// wrong test to delete.
			if !strings.Contains(body, pin.exercises) {
				t.Errorf("internal/%s is ledgered as pinned by %s, but that test's body does not contain %s.\n"+
					"This binding requires the fragment LITERALLY in the pinning test's own body, so the ledger cannot be "+
					"satisfied by an unrelated test that merely exists. Either the entry names the wrong test (re-point it), "+
					"or the invocation moved into a helper (inline it, or update this pin's `exercises`).",
					name, pin.test, pin.exercises)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Site 5 + 6: GetSubmission's single row — internal/cmd/app_status.go (detail
// view) and internal/cmd/app_listing.go (resolveListing)
// ---------------------------------------------------------------------------

// detailField reads one `Label: value` line out of the tabwriter-aligned detail
// block, so assertions name the FIELD rather than searching the whole page for a
// word. `app status <slug>` prints the version, status and deploy state of ONE
// row, and a bare Contains would be satisfied by any of them appearing anywhere.
func detailField(t *testing.T, out, label string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(label) + `:\s+(.*?)\s*$`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no %q line in the detail output — there is nothing to check the row pick against:\n%s", label, out)
	}
	return m[1]
}

// twoRowDetailBody is a newest-first, `?blockId=`-narrowed listing whose two rows
// disagree on every field the detail view prints.
func twoRowDetailBody(slug string) map[string]any {
	return map[string]any{
		"submissions": []map[string]any{
			{"id": "pubreq_02H", "blockId": slug, "version": "0.2.0", "status": "pending",
				"deployState": "building", "submittedAt": "2026-07-29T10:00:00.000Z", "liveUrl": nil},
			{"id": "pubreq_01H", "blockId": slug, "version": "0.1.1", "status": "approved",
				"deployState": "live", "submittedAt": "2026-07-28T10:00:00.000Z",
				"liveUrl": "https://" + slug + ".civit.ai/"},
		},
	}
}

// TestAppStatusDetailShowsTheNewestSubmission pins GetSubmission's row through
// the surface that PRINTS it.
//
// 🔴 THIS IS THE SAME HARM AS `app view`, ON A FOURTH SURFACE, AND IT WAS FOUND
// ONLY BY THE ADVERSARIAL AUDIT OF THE PR THAT FIXED THE OTHER THREE. Reversing
// GetSubmission's `Submissions[len-1]` pick left the whole merged-tree suite
// green (3812 RUN, 0 FAIL) while `app status <slug>` reported an OLDER row's
// version, status and deploy state as plain fact — "0.1.1 / approved / live" for
// an app whose newest submission is 0.2.0 and still building.
func TestAppStatusDetailShowsTheNewestSubmission(t *testing.T) {
	const slug = "buzz-generator"
	var rec struct{ auth, path, id, blockID string }
	srv := statusServer(t, twoRowDetailBody(slug), http.StatusOK, &rec)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, _, err := run(t, "app", "status", slug)
	if err != nil {
		t.Fatalf("app status <slug>: %v", err)
	}
	// REACHABILITY: this must be the narrowed DETAIL lookup, not the list view —
	// they render differently and only one of them goes through GetSubmission.
	if rec.blockID != slug {
		t.Fatalf("blockId query = %q, want %q — the detail lookup did not run, so nothing below is about its row pick", rec.blockID, slug)
	}
	// Expected values written by hand from twoRowDetailBody's NEWEST row.
	for _, f := range []struct{ label, want string }{
		{"Version", "0.2.0"},
		{"Publish request", "pubreq_02H"},
		{"Status", "pending"},
		{"Deploy state", "building"},
	} {
		if got := detailField(t, out, f.label); got != f.want {
			t.Errorf("%s: = %q, want %q — the NEWEST row's value.\n"+
				"An older row's deploy state printed as fact is #390's harm; here it reaches `app status` itself.\n%s",
				f.label, got, f.want, out)
		}
	}
}

// TestAppListingResolvesTheNewestSubmissionsBlock pins the OTHER consumer of
// GetSubmission's row: resolveListing takes its AppBlockID and hands it to
// GetMyListingForApp, so the row picked here decides WHICH listing every
// `app listing` subcommand reads and mutates. That is the "real data for a
// DIFFERENT version" harm, on the write path.
func TestAppListingResolvesTheNewestSubmissionsBlock(t *testing.T) {
	const slug = "my-app"
	var listingInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			// Newest first, two APPROVED rows carrying DIFFERENT block ids, so
			// only recency separates them.
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []map[string]any{
				{"id": "pubreq_02H", "blockId": slug, "version": "0.2.0", "status": "approved",
					"appBlockId": "block_newest", "submittedAt": "2026-07-29T10:00:00.000Z"},
				{"id": "pubreq_01H", "blockId": slug, "version": "0.1.0", "status": "approved",
					"appBlockId": "block_oldest", "submittedAt": "2026-07-28T10:00:00.000Z"},
			}})
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			listingInput = r.URL.RawQuery
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft", "contentRating": "g", "hasPendingRevision": false})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId": "listing_1", "slug": slug, "status": "draft", "hasPendingRevision": false, "shadowId": nil,
				"assets": map[string]any{
					"icon":        map[string]any{"imageId": 11, "url": "http://x/i.png"},
					"cover":       map[string]any{"imageId": nil, "url": nil},
					"screenshots": []any{},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	if _, _, err := run(t, "app", "listing", "status", "--slug", slug); err != nil {
		t.Fatalf("app listing status: %v", err)
	}
	// REACHABILITY: the listing resolve must have happened, or "the older id is
	// absent" is trivially true of an empty string.
	if listingInput == "" {
		t.Fatal("getMyListingForApp was never called, so no appBlockId was resolved — nothing below is about the row pick")
	}
	// Hand-written from the fixture's newest row, not read back from it.
	if !strings.Contains(listingInput, "block_newest") {
		t.Errorf("the listing was resolved with the wrong block id.\nwant %q in the query; got: %s", "block_newest", listingInput)
	}
	if strings.Contains(listingInput, "block_oldest") {
		t.Errorf("`app listing` resolved the OLDER version's block — every subcommand would then read and MUTATE "+
			"the wrong listing (#390).\ngot: %s", listingInput)
	}
}
