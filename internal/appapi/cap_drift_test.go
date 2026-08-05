package appapi

// Vendored-cap drift guard for ListSubmissionsCap.
//
// ListSubmissionsCap mirrors MAX_ROWS in civitai/civitai
// src/pages/api/v1/blocks/submissions.ts. TestListSubmissionsCapMirrorsServer
// pins the CLI's own literal, but it cannot SEE the server — so on its own the
// only thing holding the two together is a comment, and both drift directions
// fail silently (see the constant's doc comment; the LOWERING direction
// reintroduces the silent-truncation bug the caveat exists to prevent).
//
// The obvious live check — "fetch the listing, assert len == cap" — is unsound
// and would pass for the wrong reason: an account holding fewer submissions
// than the cap returns a short page and proves nothing, and a full page is
// equally consistent with "the cap is 100" and "the cap is 250 and this account
// happens to hold exactly 100". A green from that check would be noise.
//
// So the probe below establishes truncation POSITIVELY before it reads a cap
// off anything, using the one asymmetry the route offers: `?blockId=<slug>` is
// narrowed by `where.slug` BEFORE `take`, so a per-app lookup can see rows the
// unfiltered listing dropped. If any app in the page holds a submission
// STRICTLY OLDER than the oldest row the unfiltered listing returned, that
// listing demonstrably withheld rows — truncation is proven, and the size of
// that page IS the server's cap, whatever the vendored constant says. This
// catches drift in BOTH directions. With no such evidence the probe answers
// "cannot observe" and the live test SKIPS; it never turns an absence of
// evidence into a pass.
//
// Strictly-older is also what makes the probe safe against a submission landing
// between the two calls: a new row is NEWER than the page's oldest, so it can
// never forge the signal.
//
// Two ways the probe legitimately declines, both conservative (skip, never a
// wrong answer): an app that itself holds more rows than the cap has its own
// view truncated too, and a page boundary that falls exactly between two apps
// leaves no straddling app to cross-check.
//
// The probe is validated offline against fixtures with KNOWN caps — including
// the two controls that matter: a page that is exactly full but hides nothing
// must come back "cannot observe", and a lowered or raised server cap must come
// back "confirmed" at the server's number so the live assertion goes red.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

// capVerdict is the probe's conclusion. There are deliberately only two: either
// we PROVED the listing was truncated (and therefore measured the server's real
// cap) or we could not tell. "Looks fine" is not a verdict.
type capVerdict string

const (
	capCannotObserve capVerdict = "cannot-observe"
	capConfirmed     capVerdict = "confirmed"
)

type capProbe struct {
	Verdict capVerdict
	// ObservedCap is the server's actual page size, meaningful ONLY when
	// Verdict is capConfirmed.
	ObservedCap int
	// Evidence names the row that proved the listing withheld history.
	Evidence string
	// Probed counts the per-app lookups spent.
	Probed int
}

// listFn is the seam: the live test passes Client.ListSubmissions, the offline
// tests pass a fixture server.
type listFn func(ctx context.Context, blockID string) ([]Submission, error)

// liveCapProbeBudget bounds the per-app lookups. The route allows 60 requests
// per minute per user, so 1 + 8 stays comfortably inside one window.
const liveCapProbeBudget = 8

// probeSubmissionsCap measures the server's unfiltered page size, or reports
// that it could not. See the file comment for why a plain length check is not
// sound. It returns an error only when an API call itself fails.
func probeSubmissionsCap(ctx context.Context, list listFn, maxProbes int) (capProbe, error) {
	page, err := list(ctx, "")
	if err != nil {
		return capProbe{}, err
	}
	if len(page) == 0 {
		return capProbe{Verdict: capCannotObserve}, nil
	}
	// The oldest timestamp actually PRESENT in the page, computed as a minimum
	// rather than read off the last row: trusting the server's `orderBy` would
	// make this probe silently wrong if that ordering ever changed.
	oldest, ok := oldestSubmittedAt(page)
	if !ok {
		return capProbe{Verdict: capCannotObserve}, nil
	}
	probed := 0
	for _, slug := range slugsOldestFirst(page) {
		if probed >= maxProbes {
			break
		}
		probed++
		rows, err := list(ctx, slug)
		if err != nil {
			return capProbe{}, err
		}
		for _, r := range rows {
			at, perr := time.Parse(time.RFC3339, r.SubmittedAt)
			if perr != nil {
				continue
			}
			// STRICTLY older: a row that landed mid-probe is newer than the
			// page's oldest and therefore cannot forge this.
			if at.Before(oldest) {
				return capProbe{
					Verdict:     capConfirmed,
					ObservedCap: len(page),
					Probed:      probed,
					Evidence: fmt.Sprintf(
						"app %q holds submission %s at %s, older than the oldest row the unfiltered listing returned (%s) — the listing withheld it",
						slug, r.ID, r.SubmittedAt, oldest.Format(time.RFC3339)),
				}, nil
			}
		}
	}
	return capProbe{Verdict: capCannotObserve, Probed: probed}, nil
}

// oldestSubmittedAt returns the minimum parseable submittedAt in the page.
func oldestSubmittedAt(page []Submission) (time.Time, bool) {
	var oldest time.Time
	found := false
	for _, s := range page {
		at, err := time.Parse(time.RFC3339, s.SubmittedAt)
		if err != nil {
			continue
		}
		if !found || at.Before(oldest) {
			oldest, found = at, true
		}
	}
	return oldest, found
}

// slugsOldestFirst lists the page's distinct apps, the one whose visible
// history reaches furthest back first — an app sitting at the page's old
// boundary is the likeliest to have had rows cut off, so it is the
// highest-yield probe.
func slugsOldestFirst(page []Submission) []string {
	minAt := map[string]time.Time{}
	var order []string
	for _, s := range page {
		at, err := time.Parse(time.RFC3339, s.SubmittedAt)
		if err != nil {
			continue
		}
		cur, seen := minAt[s.BlockID]
		if !seen {
			order = append(order, s.BlockID)
		}
		if !seen || at.Before(cur) {
			minAt[s.BlockID] = at
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return minAt[order[i]].Before(minAt[order[j]]) })
	return order
}

// ── offline harness validation ───────────────────────────────────────────────

// fakeListing models the route: the unfiltered listing is the newest serverCap
// rows across every app, while a per-app lookup is narrowed BEFORE the cap
// applies — and is then capped in its own right, exactly like the server.
type fakeListing struct {
	all       []Submission
	serverCap int
	calls     int
	// extra is appended to a per-app lookup only, modelling a submission that
	// lands between the unfiltered read and the cross-check.
	extra map[string][]Submission
}

func (f *fakeListing) list(_ context.Context, blockID string) ([]Submission, error) {
	f.calls++
	var rows []Submission
	for _, s := range f.all {
		if blockID == "" || s.BlockID == blockID {
			rows = append(rows, s)
		}
	}
	if blockID != "" {
		rows = append(rows, f.extra[blockID]...)
	}
	// RFC3339 in a fixed zone sorts lexicographically; newest first, like the
	// server's `orderBy: { submittedAt: 'desc' }`.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].SubmittedAt > rows[j].SubmittedAt })
	if len(rows) > f.serverCap {
		rows = rows[:f.serverCap]
	}
	return rows, nil
}

// sub builds one submission for slug, dated `day` days after an epoch.
func sub(slug string, day int) Submission {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return Submission{
		ID:          fmt.Sprintf("pubreq_%s_%04d", slug, day),
		BlockID:     slug,
		Version:     "0.1.0",
		Status:      "approved",
		SubmittedAt: base.AddDate(0, 0, day).Format(time.RFC3339),
	}
}

// contiguousApps builds numApps apps of perApp submissions each, laid out so
// that EVERY day from 1 to numApps*perApp carries exactly one submission and
// each app occupies one contiguous block of days. App 0 is the newest.
//
// The contiguity matters: a page boundary then always falls INSIDE some app's
// block unless the cap is an exact multiple of perApp, which is what gives the
// probe a straddling app to cross-check. Tests pick caps that are not multiples
// of perApp on purpose — see TestCapProbeDeclinesOnAlignedBoundary for the
// aligned case, which the probe conservatively declines.
func contiguousApps(numApps, perApp int) []Submission {
	total := numApps * perApp
	var all []Submission
	for i := 0; i < numApps; i++ {
		slug := fmt.Sprintf("app-%03d", i)
		top := total - i*perApp // newest day in this app's block
		for k := 0; k < perApp; k++ {
			all = append(all, sub(slug, top-k))
		}
	}
	return all
}

// soloApps builds n apps holding exactly one submission each — an account with
// no hidden per-app history, so nothing can ever prove truncation.
func soloApps(n int) []Submission { return contiguousApps(n, 1) }

// TestCapProbeConfirmsTrueServerCap is the POSITIVE control: against a server
// whose cap matches the vendored constant, the probe must actually OBSERVE it.
// A probe that could only ever answer "cannot observe" would make every other
// result here meaningless.
func TestCapProbeConfirmsTrueServerCap(t *testing.T) {
	f := &fakeListing{all: contiguousApps(60, 3), serverCap: ListSubmissionsCap}
	got, err := probeSubmissionsCap(context.Background(), f.list, liveCapProbeBudget)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.Verdict != capConfirmed {
		t.Fatalf("verdict = %q, want %q — the probe must be able to observe a real cap", got.Verdict, capConfirmed)
	}
	if got.ObservedCap != ListSubmissionsCap {
		t.Errorf("ObservedCap = %d, want %d", got.ObservedCap, ListSubmissionsCap)
	}
	if got.Evidence == "" {
		t.Error("a confirmed verdict must carry the row that proved it")
	}
}

// TestCapProbeCatchesDriftBothDirections is the NEGATIVE control, and the whole
// point of the guard: a server cap that no longer matches the vendored constant
// must be OBSERVED as the server's number, so the live assertion goes red. The
// lowered case is the serious one — that drift silently reintroduces the
// truncation bug.
func TestCapProbeCatchesDriftBothDirections(t *testing.T) {
	cases := []struct {
		name      string
		serverCap int
	}{
		{"server LOWERED its cap (silently reintroduces truncation)", 50},
		{"server RAISED its cap", 130},
	}
	for _, tc := range cases {
		f := &fakeListing{all: contiguousApps(60, 3), serverCap: tc.serverCap}
		got, err := probeSubmissionsCap(context.Background(), f.list, liveCapProbeBudget)
		if err != nil {
			t.Fatalf("%s: probe: %v", tc.name, err)
		}
		if got.Verdict != capConfirmed {
			t.Errorf("%s: verdict = %q, want %q — drift must be OBSERVED, not skipped past",
				tc.name, got.Verdict, capConfirmed)
			continue
		}
		if got.ObservedCap != tc.serverCap {
			t.Errorf("%s: ObservedCap = %d, want %d", tc.name, got.ObservedCap, tc.serverCap)
		}
		// The live test's own assertion, applied to the probe's answer.
		if got.ObservedCap == ListSubmissionsCap {
			t.Errorf("%s: probe reported the vendored value %d for a server capped at %d — the drift would go UNDETECTED",
				tc.name, got.ObservedCap, tc.serverCap)
		}
	}
}

// TestCapProbeCannotObserve covers every way the probe must decline to answer.
// The last case is what makes this guard sound: a page that is exactly full but
// hides nothing must NOT be read as confirmation — that is precisely the
// wrong-reason pass a naive `len == cap` live check would report.
func TestCapProbeCannotObserve(t *testing.T) {
	cases := []struct {
		name string
		all  []Submission
	}{
		{"account holds nothing", nil},
		{"account is far under the cap", soloApps(5)},
		{"page is exactly full but no app hides older history", soloApps(ListSubmissionsCap)},
	}
	for _, tc := range cases {
		f := &fakeListing{all: tc.all, serverCap: ListSubmissionsCap}
		got, err := probeSubmissionsCap(context.Background(), f.list, liveCapProbeBudget)
		if err != nil {
			t.Fatalf("%s: probe: %v", tc.name, err)
		}
		if got.Verdict != capCannotObserve {
			t.Errorf("%s: verdict = %q (cap %d), want %q — an absence of evidence must not become a confirmation",
				tc.name, got.Verdict, got.ObservedCap, capCannotObserve)
		}
	}
}

// TestCapProbeDeclinesOnAlignedBoundary pins the known conservative blind spot:
// when the page boundary lands exactly between two apps, no app straddles it and
// the probe declines rather than guessing. 150 rows / 3-per-app is such a case.
func TestCapProbeDeclinesOnAlignedBoundary(t *testing.T) {
	f := &fakeListing{all: contiguousApps(60, 3), serverCap: 150}
	got, err := probeSubmissionsCap(context.Background(), f.list, liveCapProbeBudget)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.Verdict != capCannotObserve {
		t.Errorf("verdict = %q, want %q — an aligned boundary offers no straddling app, so the probe must decline",
			got.Verdict, capCannotObserve)
	}
}

// TestCapProbeIgnoresRowsLandingMidProbe: a submission created between the
// unfiltered read and the per-app read appears only in the per-app view. It is
// NEWER than the page's oldest row, so it must not be mistaken for withheld
// history — otherwise this guard would fail at random on an active account.
func TestCapProbeIgnoresRowsLandingMidProbe(t *testing.T) {
	f := &fakeListing{
		all:       soloApps(ListSubmissionsCap), // exactly a full page, nothing hidden
		serverCap: ListSubmissionsCap,
		// Far NEWER than anything in the page, on the app the probe checks first.
		extra: map[string][]Submission{"app-099": {sub("app-099", 9000)}},
	}
	got, err := probeSubmissionsCap(context.Background(), f.list, liveCapProbeBudget)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.Verdict != capCannotObserve {
		t.Errorf("verdict = %q, want %q — a newly-landed row is not evidence of truncation",
			got.Verdict, capCannotObserve)
	}
}

// TestCapProbeRespectsBudget keeps the live guard inside the route's 60/min
// per-user rate limit, and asserts the budget is actually SPENT (a probe that
// bailed after one call would satisfy an upper bound while proving nothing).
func TestCapProbeRespectsBudget(t *testing.T) {
	f := &fakeListing{all: soloApps(ListSubmissionsCap), serverCap: ListSubmissionsCap}
	got, err := probeSubmissionsCap(context.Background(), f.list, liveCapProbeBudget)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if want := 1 + liveCapProbeBudget; f.calls != want {
		t.Errorf("probe made %d API calls, want exactly %d (1 listing + %d cross-checks)",
			f.calls, want, liveCapProbeBudget)
	}
	if got.Probed != liveCapProbeBudget {
		t.Errorf("Probed = %d, want %d", got.Probed, liveCapProbeBudget)
	}
}

// ── the live guard ───────────────────────────────────────────────────────────

// TestSubmissionsCapDriftAgainstLiveAPI checks the VENDORED ListSubmissionsCap
// against the real server, in the style of the pins-vs-published guard
// (internal/scaffold/pins_guard_test.go). It is gated on
// CIVITAI_CHECK_SUBMISSIONS_CAP=1 plus a credential, so the default
// `go test ./...` stays offline.
//
// It fails ONLY on proven drift. Every other outcome — no credential, an
// unreachable server, or an account whose submissions cannot demonstrate
// truncation — is a SKIP carrying its reason, never a pass: a green here has to
// mean "the server's cap was measured and it is 100", and nothing else.
//
// Feasibility, stated plainly: this needs an account that (a) has Apps access
// and (b) holds more submissions than the server's cap, with at least one app
// whose older rows fall off the page. That is a real developer account, not a
// CI fixture — so the guard is useful to a maintainer on demand, and running it
// in CI would need a dedicated credential secret. Wiring that job touches
// .github/workflows/*, which is ask-first, so it is PROPOSED in the PR, not
// wired here.
func TestSubmissionsCapDriftAgainstLiveAPI(t *testing.T) {
	if os.Getenv("CIVITAI_CHECK_SUBMISSIONS_CAP") != "1" {
		t.Skip("live guard; set CIVITAI_CHECK_SUBMISSIONS_CAP=1 (plus CIVITAI_TOKEN) to run")
	}
	token := os.Getenv("CIVITAI_TOKEN")
	if token == "" {
		t.Skip("CIVITAI_CHECK_SUBMISSIONS_CAP=1 but CIVITAI_TOKEN is empty — the server cannot be observed, so this is a SKIP, not a pass")
	}
	base := os.Getenv("CIVITAI_BASE_URL")
	if base == "" {
		base = "https://civitai.com"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := probeSubmissionsCap(ctx, New(base, token, "").ListSubmissions, liveCapProbeBudget)
	if err != nil {
		// Unreachable / rate-limited / not entitled: nothing was learned about
		// the cap. Never a false failure, and never a pass.
		t.Skipf("could not read %s%s (%v) — cap NOT observed, so this is a SKIP, not a pass", base, SubmissionsPath, err)
	}
	if res.Verdict != capConfirmed {
		t.Skipf("CANNOT OBSERVE the server cap from this account after %d per-app cross-checks: "+
			"no app showed a submission older than the oldest row the listing returned, so the listing was "+
			"probably not truncated — which proves nothing about MAX_ROWS. Re-run against an account holding "+
			"more submissions than the cap. SKIP, not a pass.", res.Probed)
	}
	if res.ObservedCap != ListSubmissionsCap {
		t.Fatalf("VENDORED CAP DRIFT — the server's listing page size is %d, ListSubmissionsCap says %d.\n"+
			"  evidence: %s\n"+
			"  impact:   %s\n"+
			"  fix:      set ListSubmissionsCap (internal/appapi/appblocks.go) AND the literal in "+
			"TestListSubmissionsCapMirrorsServer to %d, after confirming MAX_ROWS in "+
			"civitai/civitai src/pages/api/v1/blocks/submissions.ts.",
			res.ObservedCap, ListSubmissionsCap, res.Evidence, driftImpact(res.ObservedCap), res.ObservedCap)
	}
	t.Logf("server cap CONFIRMED at %d after %d cross-check(s) — %s", res.ObservedCap, res.Probed, res.Evidence)
}

// driftImpact spells out which of the two failure modes a measured drift is, so
// whoever reads the failure knows whether users are being over-warned or
// silently under-warned.
func driftImpact(observed int) string {
	if observed < ListSubmissionsCap {
		return "the server's cap is LOWER than the vendored value, so `civitai app status` prints NO truncation " +
			"caveat on a truncated listing — the silent-truncation bug is live right now"
	}
	return "the server's cap is HIGHER than the vendored value, so `civitai app status` warns about truncation " +
		"for callers whose listing was not truncated"
}
