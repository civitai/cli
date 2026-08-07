package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/civitai/cli/internal/devtunnel"
	"github.com/civitai/cli/internal/dnsprobe"
)

// The embeddability warnings only do their job if they land where the developer
// is looking: `dev-tunnel` printing "Ready" and a URL while the app silently
// fails to load is the entire complaint. A warning emitted before the readiness
// wait would scroll away behind it and reproduce the bug in a new form, so
// PLACEMENT is asserted here, not just presence.

const (
	corsSummary = "CORS-BLOCKED-DEV-SERVER"
	envSummary  = "MISSING-PARENT-ORIGINS"
)

func corsFinding() devtunnel.Finding {
	return devtunnel.Finding{
		Kind:     devtunnel.FindingCORS,
		Summary:  corsSummary,
		Evidence: []string{"  evidence line"},
		Fix:      []string{"Fix — do the thing"},
	}
}

func envFinding() devtunnel.Finding {
	return devtunnel.Finding{
		Kind:     devtunnel.FindingParentOrigins,
		Summary:  envSummary,
		Evidence: []string{"  evidence line"},
		Fix:      []string{"Fix — do the other thing"},
	}
}

// countFindingSummaries counts how many of these findings' summaries appear in s.
// Used instead of a boolean "did anything warn" so the assertions carry a NUMBER
// that has to move with the fixture — a zero that cannot be distinguished from a
// harness wired to nothing is the failure mode this whole check exists to avoid.
func countFindingSummaries(s string, findings []devtunnel.Finding) int {
	n := 0
	for _, f := range findings {
		n += strings.Count(s, f.Summary)
	}
	return n
}

// probeSnapshotter is a probePublic that records the output stream's contents at
// the FIRST probe — i.e. the instant the readiness wait starts doing work — and
// then reports ready so the session finishes. Snapshotting inside the injected
// seam is what makes "printed before the wait" an ORDERING assertion rather than
// a wall-clock one: there is no sleep, no threshold and no real clock anywhere.
type probeSnapshotter struct {
	mu    sync.Mutex
	buf   *syncBuffer
	calls int
	first string
}

func (p *probeSnapshotter) probe(context.Context, string) (bool, string, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	if n == 1 {
		p.first = p.buf.String()
	}
	p.mu.Unlock()
	if n == 1 {
		// Not ready on the first probe, so the wait actually loops at least once —
		// a wait that returns instantly is not the wait an author Ctrl-Cs.
		return false, "http 404", nil
	}
	return true, "http 401", nil
}

func (p *probeSnapshotter) snapshot() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.first
}

// runToReadyThenInterrupt drives a session until want appears, then interrupts it
// and waits for a clean exit. Returns false if waitForOutput already failed.
func runToReadyThenInterrupt(t *testing.T, deps tunnelSessionDeps, buf *syncBuffer, sigs chan os.Signal, want string) bool {
	t.Helper()
	errc := runInBackground(deps)
	if !waitForOutput(t, buf, want, errc) {
		return false
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}
	return true
}

// jointDeps wires a deps set whose stdout AND stderr are the SAME buffer. The
// findings go to out and the readiness wait's banner/heartbeat go to errw, so the
// ordering BETWEEN them is only observable on a merged stream — which is also
// what a real terminal is.
func jointDeps(t *testing.T, apiStub *fakeTunnelAPI, sigs <-chan os.Signal, joint *syncBuffer) tunnelSessionDeps {
	t.Helper()
	deps := baseDeps(t, apiStub, &fakeDialer{tunnel: newFakeTunnel()}, newFakeTimer(), sigs)
	deps.out = joint
	deps.errw = joint
	deps.readyPollInterval = time.Millisecond
	return deps
}

// assertOrder requires each marker to appear, in the given order, in s.
func assertOrder(t *testing.T, s string, markers ...string) {
	t.Helper()
	prev := -1
	for _, m := range markers {
		idx := strings.Index(s, m)
		if idx < 0 {
			t.Fatalf("expected %q in output:\n%s", m, s)
		}
		if idx < prev {
			t.Fatalf("%q appeared out of order (must follow the previous marker):\n%s", m, s)
		}
		prev = idx
	}
}

// TestEmbedWarningsPrecedeReadyBlock covers the confirmed-ready path: both
// findings must be on screen BEFORE the ✓ line and the URL.
func TestEmbedWarningsPrecedeReadyBlock(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)
	var out, errw syncBuffer
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out
	deps.errw = &errw
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 5 * time.Second
	deps.probePublic = func(context.Context, string) (bool, string, error) { return true, "http 401", nil }
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding {
		return []devtunnel.Finding{corsFinding()}
	}
	deps.checkParentOrigins = func(string) []devtunnel.Finding {
		return []devtunnel.Finding{envFinding()}
	}

	errc := runInBackground(deps)
	if !waitForOutput(t, &out, "✓ Ready", errc) {
		return
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}

	assertOrder(t, out.String(), corsSummary, envSummary, "✓ Ready", sampleSession().URL)
}

// TestEmbedWarningsPrecedeReadyBlockNoWait: --no-wait skips the readiness wait
// entirely, a separate print path that must not lose the warnings.
func TestEmbedWarningsPrecedeReadyBlockNoWait(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)
	var out, errw syncBuffer
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out
	deps.errw = &errw
	deps.noWait = true
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding {
		return []devtunnel.Finding{corsFinding()}
	}

	errc := runInBackground(deps)
	if !waitForOutput(t, &out, sampleSession().URL, errc) {
		return
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}

	assertOrder(t, out.String(), corsSummary, "Dev tunnel ready", sampleSession().URL)
}

// TestEmbedWarningsPrecedeReadyTimeout: the readiness-timeout path prints its own
// warning + URL block. An app that is ALSO un-embeddable must say so here too —
// this is the path where a developer is most likely to blame the tunnel.
func TestEmbedWarningsPrecedeReadyTimeout(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)
	var out, errw syncBuffer
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out
	deps.errw = &errw
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 30 * time.Millisecond
	deps.probePublic = func(context.Context, string) (bool, string, error) { return false, "http 502", nil }
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding {
		return []devtunnel.Finding{corsFinding()}
	}

	errc := runInBackground(deps)
	if !waitForOutput(t, &out, sampleSession().URL, errc) {
		return
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}

	assertOrder(t, out.String(), corsSummary, sampleSession().URL)
}

// TestEmbedWarningsSilentWhenClean is the counterpart control: a healthy dev
// server must add NO noise. Without this, a renderer that printed a banner
// unconditionally would satisfy every test above.
func TestEmbedWarningsSilentWhenClean(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)
	var out, errw syncBuffer
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out
	deps.errw = &errw
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 5 * time.Second
	deps.probePublic = func(context.Context, string) (bool, string, error) { return true, "http 401", nil }

	called := false
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding { called = true; return nil }
	deps.checkParentOrigins = func(string) []devtunnel.Finding { return nil }

	errc := runInBackground(deps)
	if !waitForOutput(t, &out, "✓ Ready", errc) {
		return
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}

	if !called {
		t.Error("probeEmbeddable must be invoked on the happy path — a check that never runs cannot warn")
	}
	if strings.Contains(out.String(), "⚠") {
		t.Errorf("a clean dev server must produce no warning:\n%s", out.String())
	}
}

// TestEmbedChecksRunBeforeMint pins the ordering that keeps a broken setup from
// burning a rate-limited server session: both checks must have run by the time
// StartDevTunnel is called.
func TestEmbedChecksRunBeforeMint(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)
	var out, errw syncBuffer
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out
	deps.errw = &errw
	deps.noWait = true

	var order []string
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding {
		order = append(order, "embeddable")
		return nil
	}
	deps.checkParentOrigins = func(string) []devtunnel.Finding {
		order = append(order, "parent-origins")
		return nil
	}
	apiStub.onStart = func() { order = append(order, "mint") }

	errc := runInBackground(deps)
	if !waitForOutput(t, &out, sampleSession().URL, errc) {
		return
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}

	want := []string{"embeddable", "parent-origins", "mint"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("checks must run before the mint: got %v want %v", order, want)
	}
}

// TestEmbedChecksSkippedWhenDepsNil: both fields are optional (nil disables the
// check), matching probePublic/probeLocalHop. A nil must not panic.
func TestEmbedChecksSkippedWhenDepsNil(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)
	var out, errw syncBuffer
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out
	deps.errw = &errw
	deps.noWait = true
	deps.probeEmbeddable = nil
	deps.checkParentOrigins = nil

	errc := runInBackground(deps)
	if !waitForOutput(t, &out, sampleSession().URL, errc) {
		return
	}
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}
}

// TestPrintEmbedWarnings covers the renderer directly: the summary AND every
// detail line must reach the writer. A renderer that dropped Detail would still
// satisfy the placement tests above, which only look for summaries.
func TestPrintEmbedWarnings(t *testing.T) {
	var b strings.Builder
	printEmbedWarnings(&b, []devtunnel.Finding{corsFinding(), envFinding()})
	got := b.String()

	for _, want := range []string{
		corsSummary, "  evidence line", "Fix — do the thing",
		envSummary, "Fix — do the other thing",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	var empty strings.Builder
	printEmbedWarnings(&empty, nil)
	if empty.String() != "" {
		t.Errorf("no findings must render nothing, got %q", empty.String())
	}
}

// TestPrintEmbedWarningsDedupesSharedFix: CORS, framing and allowedHosts all
// come from one missing vite.config.ts `server` block, so in the real output
// they arrive as three findings carrying an IDENTICAL eight-line remediation.
// Printing it per finding buries the evidence and reads as three unrelated
// problems — measured against a live Vite server before this dedupe existed.
func TestPrintEmbedWarningsDedupesSharedFix(t *testing.T) {
	shared := []string{"Fix — in vite.config.ts:", "    server: { headers: {...} }"}
	var b strings.Builder
	printEmbedWarnings(&b, []devtunnel.Finding{
		{Kind: devtunnel.FindingCORS, Summary: "cors summary", Evidence: []string{"  cors evidence"}, Fix: shared},
		{Kind: devtunnel.FindingAllowedHosts, Summary: "hosts summary", Evidence: []string{"  hosts evidence"}, Fix: shared},
		{Kind: devtunnel.FindingParentOrigins, Summary: "env summary", Evidence: []string{"  env evidence"}, Fix: []string{"Fix — in .env.development:"}},
	})
	got := b.String()

	if n := strings.Count(got, "Fix — in vite.config.ts:"); n != 1 {
		t.Errorf("a shared remediation must print exactly once, got %d:\n%s", n, got)
	}
	if n := strings.Count(got, "Fix — in .env.development:"); n != 1 {
		t.Errorf("a distinct remediation must still print, got %d:\n%s", n, got)
	}
	// Every finding keeps its own summary + evidence.
	for _, want := range []string{"cors summary", "  cors evidence", "hosts summary", "  hosts evidence", "env summary", "  env evidence"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Evidence must come before remediation, not be interleaved with it.
	assertOrder(t, got, "cors summary", "hosts summary", "env summary", "Fix — in vite.config.ts:")
}

// ── #226: the findings must survive a Ctrl-C during the readiness wait ───────
//
// The late print (above) is necessary and NOT sufficient. Measured against the
// live endpoint, the DNS wait ran >60 s, >3:00 and ~2:30–3:00 over three runs; a
// 45-second run produced ZERO preflight output because the author killed an
// apparently-hung command before the late print was reached. So the findings are
// printed at BOTH placements, and both are pinned here.

// TestEmbedWarningsPrintedBeforeReadinessWait is the #226 guard. It asserts on
// output ORDERING against the injected readiness probe — no wall clock, no
// threshold. The table is also the assertion's POSITIVE CONTROL: the count it
// observes has to move 0 → 1 → 2 with the fixture, so a green on the clean row
// cannot mean "the snapshot was wired to nothing".
func TestEmbedWarningsPrintedBeforeReadinessWait(t *testing.T) {
	for _, tc := range []struct {
		name     string
		findings []devtunnel.Finding
		want     int
	}{
		{"clean server warns about nothing", nil, 0},
		{"one finding", []devtunnel.Finding{corsFinding()}, 1},
		{"two findings", []devtunnel.Finding{corsFinding(), envFinding()}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiStub := &fakeTunnelAPI{startResult: sampleSession()}
			sigs := make(chan os.Signal, 1)
			var joint syncBuffer
			deps := jointDeps(t, apiStub, sigs, &joint)
			deps.readyTimeout = 5 * time.Second
			snap := &probeSnapshotter{buf: &joint}
			deps.probePublic = snap.probe
			deps.probeEmbeddable = func(string, int) []devtunnel.Finding { return tc.findings }
			deps.checkParentOrigins = func(string) []devtunnel.Finding { return nil }

			if !runToReadyThenInterrupt(t, deps, &joint, sigs, "✓ Ready") {
				return
			}

			// (1) The findings were ALREADY on screen when the readiness wait ran its
			// first probe — so an author who Ctrl-Cs the wait has seen them.
			if got := countFindingSummaries(snap.snapshot(), tc.findings); got != tc.want {
				t.Fatalf("findings on screen at the first readiness probe = %d, want %d\nsnapshot:\n%s",
					got, tc.want, snap.snapshot())
			}
			// (2) And the merged stream orders them ahead of the wait's own banner.
			for _, f := range tc.findings {
				assertOrder(t, joint.String(), f.Summary, "Dev tunnel established", "✓ Ready")
			}
			// (3) A clean dev server adds NO noise anywhere — the false-warning
			// direction item 10 spent four measured corrections avoiding.
			if tc.want == 0 && strings.Contains(joint.String(), "⚠") {
				t.Errorf("a clean dev server must print no warning at all:\n%s", joint.String())
			}
		})
	}
}

// TestEmbedWarningsPrintedTwiceOnReadyPath: the early print must not have
// REPLACED the late one. Both placements answer different failure modes, so the
// count is pinned at exactly 2 and the second occurrence is required to sit
// between the wait's banner and the ✓ Ready block — where the dev acts on it.
func TestEmbedWarningsPrintedTwiceOnReadyPath(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	sigs := make(chan os.Signal, 1)
	var joint syncBuffer
	deps := jointDeps(t, apiStub, sigs, &joint)
	deps.readyTimeout = 5 * time.Second
	deps.probePublic = func(context.Context, string) (bool, string, error) { return true, "http 401", nil }
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding { return []devtunnel.Finding{corsFinding()} }

	if !runToReadyThenInterrupt(t, deps, &joint, sigs, "✓ Ready") {
		return
	}

	s := joint.String()
	if n := strings.Count(s, corsSummary); n != 2 {
		t.Fatalf("the finding must print twice (early + at the ready block), got %d:\n%s", n, s)
	}
	estab := strings.Index(s, "Dev tunnel established")
	ready := strings.Index(s, "✓ Ready")
	last := strings.LastIndex(s, corsSummary)
	if estab < 0 || ready < 0 {
		t.Fatalf("missing the wait banner or the ready block:\n%s", s)
	}
	if last < estab || last > ready {
		t.Fatalf("the SECOND print must sit between the wait banner (%d) and ✓ Ready (%d), got %d:\n%s",
			estab, ready, last, s)
	}
}

// TestEmbedWarningsPrintedTwiceOnReadyTimeoutPath: the --ready-timeout expiry
// block is its own print site. This is the path where a dev is most likely to
// blame the tunnel for what is really an un-embeddable dev server, so it must
// keep BOTH prints too.
func TestEmbedWarningsPrintedTwiceOnReadyTimeoutPath(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	sigs := make(chan os.Signal, 1)
	var joint syncBuffer
	deps := jointDeps(t, apiStub, sigs, &joint)
	deps.readyTimeout = 30 * time.Millisecond
	deps.probePublic = func(context.Context, string) (bool, string, error) { return false, "http 502", nil }
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding { return []devtunnel.Finding{corsFinding()} }

	if !runToReadyThenInterrupt(t, deps, &joint, sigs, sampleSession().URL) {
		return
	}

	s := joint.String()
	if n := strings.Count(s, corsSummary); n != 2 {
		t.Fatalf("the finding must print twice (early + at the timeout block), got %d:\n%s", n, s)
	}
	estab := strings.Index(s, "Dev tunnel established")
	url := strings.Index(s, sampleSession().URL)
	last := strings.LastIndex(s, corsSummary)
	if estab < 0 || url < 0 {
		t.Fatalf("missing the wait banner or the URL block:\n%s", s)
	}
	if last < estab || last > url {
		t.Fatalf("the SECOND print must sit between the wait banner (%d) and the URL (%d), got %d:\n%s",
			estab, url, last, s)
	}
}

// TestEmbedWarningsPrintedOnceOnNoWaitPath pins the ONE deliberate asymmetry:
// --no-wait has no readiness wait to scroll behind, so the early print is
// skipped and only the late one — directly above the URL — survives. Without
// this, the two prints land a couple of seconds apart and duplicate an
// eight-line vite.config block for no reason.
func TestEmbedWarningsPrintedOnceOnNoWaitPath(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	sigs := make(chan os.Signal, 1)
	var joint syncBuffer
	deps := jointDeps(t, apiStub, sigs, &joint)
	deps.noWait = true
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding { return []devtunnel.Finding{corsFinding()} }

	if !runToReadyThenInterrupt(t, deps, &joint, sigs, sampleSession().URL) {
		return
	}

	s := joint.String()
	if n := strings.Count(s, corsSummary); n != 1 {
		t.Fatalf("--no-wait must print the finding exactly once, got %d:\n%s", n, s)
	}
	// It must be the LATE print: the early one would land before the
	// "Establishing dev tunnel…" status line, which is written after the checks.
	assertOrder(t, s, "Establishing dev tunnel", corsSummary, "Dev tunnel ready", sampleSession().URL)
}

// TestEmbedWarningsSilentWhenCleanNoWait is the --no-wait half of the
// zero-findings control. TestEmbedWarningsPrintedBeforeReadinessWait covers the
// wait path; a renderer that printed an unconditional banner would satisfy every
// ordering assertion above and only these two catch it.
func TestEmbedWarningsSilentWhenCleanNoWait(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	sigs := make(chan os.Signal, 1)
	var joint syncBuffer
	deps := jointDeps(t, apiStub, sigs, &joint)
	deps.noWait = true
	called := 0
	deps.probeEmbeddable = func(string, int) []devtunnel.Finding { called++; return nil }
	deps.checkParentOrigins = func(string) []devtunnel.Finding { return nil }

	if !runToReadyThenInterrupt(t, deps, &joint, sigs, sampleSession().URL) {
		return
	}

	if called != 1 {
		t.Errorf("probeEmbeddable ran %d times, want 1 — a check that never runs cannot warn", called)
	}
	if strings.Contains(joint.String(), "⚠") {
		t.Errorf("a clean dev server must produce no warning under --no-wait:\n%s", joint.String())
	}
}

// ── #226: the DNS estimate ───────────────────────────────────────────────────

// TestDNSPublishNoteSharedByBothRenderers: the DNS-pending state is rendered by
// TWO code paths — the non-TTY heartbeat and the bubbletea spinner — which used
// to hold two hand-copied copies of the estimate, so a fix to one would silently
// leave the other saying something different. One constant, asserted on both.
// The retired "usually <1 min" claim is pinned absent: it was measured wrong
// (0/3 runs met it) and an estimate the wait routinely blows past is what makes
// a working command read as a hang.
func TestDNSPublishNoteSharedByBothRenderers(t *testing.T) {
	const retired = "<1 min"
	if strings.Contains(dnsPublishNote, retired) {
		t.Fatalf("dnsPublishNote still claims %q: %q", retired, dnsPublishNote)
	}

	// TTY renderer.
	m := &tunnelWaitModel{
		host:       testTunnelHost,
		sp:         spinner.New(),
		start:      time.Now().Add(-time.Minute),
		dnsGrace:   time.Millisecond,
		dnsPending: true,
	}
	if view := m.View(); !strings.Contains(view, dnsPublishNote) {
		t.Errorf("TTY spinner view must carry dnsPublishNote, got:\n%s", view)
	} else if strings.Contains(view, retired) {
		t.Errorf("TTY spinner view still claims %q:\n%s", retired, view)
	}

	// Non-TTY heartbeat: drive the real wait with a DNS-pending probe.
	var errw syncBuffer
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	deps.errw = &errw // syncBuffer → non-TTY path
	deps.readyPollInterval = 2 * time.Millisecond
	deps.quietInterval = 3 * time.Millisecond
	deps.dnsPendingGrace = time.Millisecond
	// A positive cap so the wait ends on its own (non-fatal expiry) instead of
	// polling forever — this test only cares about the heartbeat's wording.
	deps.readyTimeout = 300 * time.Millisecond
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return false, dnsPendingDetail, fmt.Errorf("x: %w", dnsprobe.ErrNotPublished)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
	}()
	deadline := time.After(2 * time.Second)
	for !strings.Contains(errw.String(), "waiting for DNS to publish") {
		select {
		case <-deadline:
			t.Fatalf("no DNS-pending heartbeat appeared:\n%s", errw.String())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := errw.String(); !strings.Contains(got, dnsPublishNote) {
		t.Errorf("non-TTY heartbeat must carry dnsPublishNote, got:\n%s", got)
	} else if strings.Contains(got, retired) {
		t.Errorf("non-TTY heartbeat still claims %q:\n%s", retired, got)
	}
	<-done
}
