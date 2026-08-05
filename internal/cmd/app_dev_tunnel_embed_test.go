package cmd

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/devtunnel"
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
