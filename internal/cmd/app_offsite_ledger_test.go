package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// civitai/cli#427 residual 2 — THE REQUEST LEDGER OF THE NOT-FOUND ERROR PATH.
//
// 🔴 WHAT THIS FILE PINS IS THE SET OF REQUESTS, NOT THEIR TIMING. The drift it
// exists to catch already happened once and nobody saw it: the diagnostic probe
// cost ONE extra request when #422 outcome 2 shipped, resolveListing's by-slug
// fallback (#422 outcome 1) silently made it two, and app_offsite.go's own
// comment went on saying "one" for the length of a whole PR. Every test on this
// path asserted a per-route COUNT for the one route it cared about
// (`ps.appCalls == 1`, `ps.listingCalls == 1`), which is structurally blind to a
// route nobody thought to count — a new request added tomorrow is invisible to
// all of them. This asserts the WHOLE ordered ledger, so it fails when the set
// GROWS (the drift that happened) and when it SHRINKS (a diagnosis silently
// dropped).
//
// 🔴 NO TIMING ASSERTION LIVES HERE, DELIBERATELY. Latency was measured for
// #427 and is reported in the PR, not asserted: this repo already carries one
// elapsed-time assertion it tolerates for a reason no other test can borrow
// (TestOffsiteProbeDoesNotHoldTheCommandOpen — a context deadline is not
// observable from the server side), and a second one would be a flake with no
// such justification. A request ledger is deterministic; a duration is not.
//
// 🔴 IT IS AN INVARIANT GUARD, NOT REGRESSION COVERAGE, AND SAYING SO IS THE
// POINT. The 1→2→3 growth predates this file's base commit, so this cannot be
// shown red at base for the defect it is named after — it pins the invariant
// that the growth violated, going forward. What IS red at base is the
// documentation half below: the residual-2 paragraph in app_offsite.go said
// THREE requests for "every `no such submission`", and `app status <slug>` has
// only ever paid TWO (it does not resolve through resolveListing, so there is no
// by-slug fallback in its chain at all).

// ledgerLabel is the canonical name this file gives one outbound request. It is
// the METHOD plus the PATH — a structural identity, not a word a future feature
// could also spell — with the submissions route's selector appended because
// `?id=` and `?blockId=` are different lookups reached by different flags and
// the ledger must not read them as one.
func ledgerLabel(r *http.Request) string {
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/v1/blocks/submissions") {
		switch {
		case r.URL.Query().Get("id") != "":
			return r.Method + " " + p + "?id"
		case r.URL.Query().Get("blockId") != "":
			return r.Method + " " + p + "?blockId"
		}
	}
	return r.Method + " " + p
}

// ledgerServer is a fake civitai.com that RECORDS every request it receives, in
// order, rather than counting the handful of routes a test remembered to count.
//
// subStatus selects which failure the submissions route reports: 0 means the
// #422 wall (200 with an empty list, which appapi maps to ErrNotFound), and any
// other value is returned verbatim — 403 is the not-a-not-found case, which must
// buy no diagnosis at all.
type ledgerServer struct {
	*httptest.Server
	mu        sync.Mutex
	requests  []string
	apps      map[string]string
	subStatus int
}

func newLedgerServer(t *testing.T, apps map[string]string, subStatus int) *ledgerServer {
	t.Helper()
	ls := &ledgerServer{apps: apps, subStatus: subStatus}
	ls.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.mu.Lock()
		ls.requests = append(ls.requests, ledgerLabel(r))
		ls.mu.Unlock()

		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			if ls.subStatus != 0 {
				w.WriteHeader(ls.subStatus)
				_, _ = w.Write([]byte(`{"error":"refused"}`))
				return
			}
			if r.URL.Query().Get("id") != "" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"submissions":[]}`))
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			// A server that predates civitai/civitai#3989, or an app with no
			// listing row: the by-slug arm misses, which is the only shape that
			// still reaches the refusal.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"No listing found for this app","code":-32004}}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			body, ok := ls.apps[strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			// Any route this file does not model is itself ledger drift — record
			// it (above) and fail loudly rather than answering it.
			t.Errorf("unexpected path %s — a request this ledger does not model", r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(ls.Close)
	listingEnv(t, ls.URL)
	return ls
}

func (ls *ledgerServer) ledger() []string {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return append([]string(nil), ls.requests...)
}

const (
	ledgerSubmissionsBySlug = "GET /api/v1/blocks/submissions?blockId"
	ledgerSubmissionsByID   = "GET /api/v1/blocks/submissions?id"
	ledgerListingBySlug     = "GET /api/trpc/appListings.getMyListingForApp"
)

func ledgerStoreProbe(slug string) string { return "GET /api/v1/apps/" + slug }

// errorPathLedgerCase is one command driven to its not-found (or refused) end,
// with the EXACT ordered request ledger it may make. The `key` is the string the
// residual-2 paragraph in app_offsite.go must use for the same case — see
// TestDocumentedRequestCountsAreTheMeasuredOnes, which is what stops the prose
// and the code drifting apart again.
type errorPathLedgerCase struct {
	key       string
	args      []string
	subStatus int
	want      []string
}

func errorPathLedgerCases() []errorPathLedgerCase {
	return []errorPathLedgerCase{
		{
			// The `app listing` chain, the expensive one: the submissions lookup
			// that failed, resolveListing's by-slug fallback, then the kind probe.
			key:  "app listing <sub> not-found",
			args: []string{"app", "listing", "status", "--slug", offsiteSlugA},
			want: []string{ledgerSubmissionsBySlug, ledgerListingBySlug, ledgerStoreProbe(offsiteSlugA)},
		},
		{
			// 🔴 THE ONSITE, NEVER-SUBMITTED CASE PAYS THE SAME THREE. This is
			// the ordinary "I have not submitted yet" path — far more common
			// than an offsite app — and the probe runs on it too: `kind` is not
			// known until the store answers. Pinned so a claim that the cost is
			// paid only by offsite apps cannot be made.
			key:  "app listing <sub> not-found (onsite)",
			args: []string{"app", "listing", "status", "--slug", onsiteSlug},
			want: []string{ledgerSubmissionsBySlug, ledgerListingBySlug, ledgerStoreProbe(onsiteSlug)},
		},
		{
			// 🔴 `app status <slug>` PAYS TWO, NOT THREE. It calls
			// explainOffsiteMiss directly off GetSubmissionRows and never
			// touches resolveListing, so the by-slug fallback is not in its
			// chain. The residual-2 comment said three for BOTH commands until
			// civitai/cli#427 measured it.
			key:  "app status <slug> not-found",
			args: []string{"app", "status", offsiteSlugB},
			want: []string{ledgerSubmissionsBySlug, ledgerStoreProbe(offsiteSlugB)},
		},
		{
			// `--id` names a publish request, not an app, so there is no slug to
			// ask the store about and the probe is a no-op with no request.
			key:  "app status --id not-found",
			args: []string{"app", "status", "--id", "pr_zzz_missing"},
			want: []string{ledgerSubmissionsByID},
		},
		{
			// Not a not-found: a 403 from the invite-gated submissions route is
			// no evidence about the app's kind, so neither the fallback nor the
			// probe may run. One request, and the diagnosis is not bought.
			key:       "app listing <sub> non-not-found",
			args:      []string{"app", "listing", "status", "--slug", offsiteSlugA},
			subStatus: http.StatusForbidden,
			want:      []string{ledgerSubmissionsBySlug},
		},
	}
}

// TestNotFoundErrorPathRequestLedger drives each case through the REAL command
// path (NewRootCmd + SetArgs, via run) and asserts the whole ordered ledger.
//
// INVARIANT GUARD — see the file header. It is green at base by construction;
// what it buys is that the next request added to this path fails a test instead
// of being noticed a PR later.
func TestNotFoundErrorPathRequestLedger(t *testing.T) {
	for _, tc := range errorPathLedgerCases() {
		t.Run(tc.key, func(t *testing.T) {
			ls := newLedgerServer(t, bothOffsiteAndOnsite(), tc.subStatus)
			_, _, err := run(t, tc.args...)
			// PREMISE: every case here is an ERROR path. A case that started
			// succeeding would otherwise pin the ledger of a different command.
			if err == nil {
				t.Fatalf("PREMISE BROKEN — %v must fail; the ledger below is not the error path's", tc.args)
			}
			got := ls.ledger()
			if len(got) != len(tc.want) || !equalLedger(got, tc.want) {
				t.Errorf("the not-found error path's request ledger changed — this fails on GROWTH and on SHRINK.\n"+
					"A request added here is paid by every user who has simply not submitted yet; a request removed here\n"+
					"is a diagnosis silently dropped. If the change is deliberate, update this table AND the LEDGER lines\n"+
					"in app_offsite.go's residual 2 (they are compared against this measurement).\n"+
					"want (%d): %v\ngot  (%d): %v",
					len(tc.want), tc.want, len(got), got)
			}
		})
	}
}

func equalLedger(got, want []string) bool {
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// ledgerDocLine matches the machine-readable rows residual 2 carries:
//
//	//      LEDGER app status <slug> not-found = 2
var ledgerDocLine = regexp.MustCompile(`(?m)^//\s*LEDGER (.+?) = (\d+)$`)

// TestDocumentedRequestCountsAreTheMeasuredOnes is the prose half, and it is the
// half that would have caught the drift this issue is about.
//
// 🔴 A COMMENT IS A CLAIM, AND THIS ONE IS NOW PINNED TO A MEASUREMENT RATHER
// THAN TO A WORD. app_offsite.go's residual 2 states what the error path costs;
// that sentence was wrong about `app status <slug>` (it said three, the measured
// answer is two) and was wrong about `app listing` for the length of a whole PR
// (it said one, the answer was three). A guard on the WORDS — "the comment must
// contain the word three" — is walked by rewording and is satisfied by a comment
// nobody re-measured. This instead re-derives the numbers by driving the real
// commands and compares them with the documented ones, so:
//
//   - editing the prose alone fails (the number no longer matches the code),
//   - adding a request to the path alone fails (the code no longer matches the
//     prose), and
//   - the ledger is BIDIRECTIONAL: a documented case with no measurement, or a
//     measured case with no documented line, is itself a failure.
func TestDocumentedRequestCountsAreTheMeasuredOnes(t *testing.T) {
	src, err := os.ReadFile("app_offsite.go")
	if err != nil {
		t.Fatalf("read app_offsite.go: %v", err)
	}
	matches := ledgerDocLine.FindAllStringSubmatch(string(src), -1)
	// POSITIVE CONTROL on the extractor itself: a regexp that matches nothing
	// reports a perfectly clean bidirectional ledger of zero rows against zero
	// rows, which is the reassuring shape of a guard wired to nothing.
	if len(matches) == 0 {
		t.Fatalf("no `// LEDGER <case> = <n>` lines in app_offsite.go — the extractor is wired to nothing, "+
			"so every comparison below is vacuous. Pattern: %s", ledgerDocLine)
	}
	documented := map[string]int{}
	for _, m := range matches {
		n, cerr := strconv.Atoi(m[2])
		if cerr != nil {
			t.Fatalf("undecodable count %q on LEDGER line %q: %v", m[2], m[0], cerr)
		}
		if _, dup := documented[m[1]]; dup {
			t.Errorf("duplicate LEDGER line for %q — two claims about one case", m[1])
		}
		documented[m[1]] = n
	}

	measured := map[string]int{}
	for _, tc := range errorPathLedgerCases() {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			ls := newLedgerServer(t, bothOffsiteAndOnsite(), tc.subStatus)
			if _, _, rerr := run(t, tc.args...); rerr == nil {
				t.Fatalf("PREMISE BROKEN — %v must fail", tc.args)
			}
			n := len(ls.ledger())
			measured[tc.key] = n
			want, ok := documented[tc.key]
			if !ok {
				t.Errorf("app_offsite.go's residual 2 documents no request count for %q, which the CLI really pays %d "+
					"requests for. Add a `// LEDGER %s = %d` line there.", tc.key, n, tc.key, n)
				return
			}
			if want != n {
				t.Errorf("app_offsite.go claims %q costs %d request(s); driving the real command measured %d.\n"+
					"The comment and the code disagree — re-measure before editing either.", tc.key, want, n)
			}
		})
	}

	for key := range documented {
		if _, ok := measured[key]; !ok {
			t.Errorf("app_offsite.go documents a LEDGER case %q that no measurement covers — a claim with no witness. "+
				"Known cases: %s", key, strings.Join(sortedLedgerKeys(measured), ", "))
		}
	}
}

func sortedLedgerKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestLedgerLabelSeparatesTheSubmissionsSelectors is the control ON the label
// function. `?id=` and `?blockId=` are two different lookups on one path, and a
// label that collapsed them would make the `--id` row below indistinguishable
// from the slug row — a ledger that cannot tell two cases apart pins neither.
func TestLedgerLabelSeparatesTheSubmissionsSelectors(t *testing.T) {
	mk := func(raw string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, "http://x"+raw, nil)
		if err != nil {
			t.Fatalf("build %q: %v", raw, err)
		}
		return r
	}
	bySlug := ledgerLabel(mk("/api/v1/blocks/submissions?blockId=a"))
	byID := ledgerLabel(mk("/api/v1/blocks/submissions?id=b"))
	if bySlug == byID {
		t.Fatalf("the two submissions selectors must not share a label; both rendered %q", bySlug)
	}
	if bySlug != ledgerSubmissionsBySlug || byID != ledgerSubmissionsByID {
		t.Errorf("label drift: blockId=%q (want %q), id=%q (want %q)", bySlug, ledgerSubmissionsBySlug, byID, ledgerSubmissionsByID)
	}
	if got, want := ledgerLabel(mk("/api/v1/apps/"+offsiteSlugA)), ledgerStoreProbe(offsiteSlugA); got != want {
		t.Errorf("store probe label: got %q want %q", got, want)
	}
	// The store probe's label carries the SLUG, so a probe for the wrong app is
	// a different ledger row rather than an equal one.
	if ledgerStoreProbe(offsiteSlugA) == ledgerStoreProbe(offsiteSlugB) {
		t.Error("the store probe label must distinguish two slugs")
	}
}
