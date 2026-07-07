package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/devtunnel"
)

// syncBuffer is a goroutine-safe io.Writer the concurrent runTunnelSession tests
// use for stdout/stderr, so the test goroutine can read what the session goroutine
// wrote without racing on a strings.Builder.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// ── test doubles ─────────────────────────────────────────────────────────────

// fakeTunnelAPI records the start/stop calls and returns canned results.
type fakeTunnelAPI struct {
	mu sync.Mutex

	startResult *api.DevTunnelSession
	startErr    error
	startCalls  []struct{ blockID, pubKey string }

	stopErr   error
	stopCalls []struct{ sessionID, blockID string }

	// whoami / whoamiErr are the canned WhoAmI result used to enrich a 403 mint
	// refusal; whoamiCalls counts invocations so a test can assert WhoAmI is NOT
	// called on a non-forbidden error.
	whoami     *api.Identity
	whoamiErr  error
	whoamiCall int
}

func (f *fakeTunnelAPI) WhoAmI(_ context.Context) (*api.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whoamiCall++
	if f.whoamiErr != nil {
		return nil, f.whoamiErr
	}
	return f.whoami, nil
}

func (f *fakeTunnelAPI) whoamiCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.whoamiCall
}

func (f *fakeTunnelAPI) StartDevTunnel(_ context.Context, blockID, pubKey string) (*api.DevTunnelSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls = append(f.startCalls, struct{ blockID, pubKey string }{blockID, pubKey})
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.startResult, nil
}

func (f *fakeTunnelAPI) StopDevTunnel(_ context.Context, sessionID, blockID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls = append(f.stopCalls, struct{ sessionID, blockID string }{sessionID, blockID})
	if f.stopErr != nil {
		return false, f.stopErr
	}
	return true, nil
}

func (f *fakeTunnelAPI) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stopCalls)
}

func (f *fakeTunnelAPI) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.startCalls)
}

// fakeTunnel is a controllable devtunnel.Tunnel.
type fakeTunnel struct {
	done      chan struct{}
	activity  chan struct{}
	closed    bool
	closeOnce sync.Once
}

func newFakeTunnel() *fakeTunnel {
	return &fakeTunnel{done: make(chan struct{}), activity: make(chan struct{}, 1)}
}
func (t *fakeTunnel) Done() <-chan struct{}     { return t.done }
func (t *fakeTunnel) Activity() <-chan struct{} { return t.activity }
func (t *fakeTunnel) Close() error {
	t.closeOnce.Do(func() { t.closed = true })
	return nil
}

// fakeDialer returns a preset tunnel (or an error) and records the options.
type fakeDialer struct {
	tunnel  devtunnel.Tunnel
	err     error
	dialed  bool
	lastOpt devtunnel.DialOptions
}

func (d *fakeDialer) Dial(_ context.Context, opts devtunnel.DialOptions) (devtunnel.Tunnel, error) {
	d.dialed = true
	d.lastOpt = opts
	if d.err != nil {
		return nil, d.err
	}
	return d.tunnel, nil
}

// fakeTimer is a Timer whose channel the test fires by hand.
// fakeTimer is shared between the session goroutine (Reset/Stop/C) and the test
// goroutine (reads), so its counters are mutex-guarded. Stop() reports the timer
// as already-stopped (returns false) so runTunnelSession exercises the drain
// path on Reset.
type fakeTimer struct {
	ch      chan time.Time
	mu      sync.Mutex
	resets  int
	stopped int
}

func newFakeTimer() *fakeTimer           { return &fakeTimer{ch: make(chan time.Time, 1)} }
func (t *fakeTimer) C() <-chan time.Time { return t.ch }
func (t *fakeTimer) Reset(time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resets++
	return true
}
func (t *fakeTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped++
	return false // force the drain branch (already fired / not active)
}
func (t *fakeTimer) fire() { t.ch <- time.Now() }
func (t *fakeTimer) resetCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resets
}

// goodKeygen mints a real ephemeral key (cheap, keeps the pubkey plumbing real).
func goodKeygen() (*devtunnel.EphemeralKey, error) { return devtunnel.GenerateEphemeralKey() }

// sampleHostKey is a real OpenSSH ed25519 public-key line the tests use as the
// mint's sshHostPublicKey (the value the CLI pins). Generated once so the
// happy-path session carries a valid, pinnable host key.
var sampleHostKey = func() string {
	k, err := devtunnel.GenerateEphemeralKey()
	if err != nil {
		panic(err)
	}
	return k.AuthorizedKey
}()

func sampleSession() *api.DevTunnelSession {
	return &api.DevTunnelSession{
		SessionID:        "bki_test",
		Host:             "dev-0123456789abcdef.civit.ai",
		URL:              "https://civitai.com/apps/dev/my-block",
		ExpiresAt:        time.Now().Add(8 * time.Hour).Unix(),
		SpendCapBuzz:     5000,
		SSHHostPublicKey: sampleHostKey,
	}
}

// baseDeps wires a happy-path deps set; callers override fields per test.
func baseDeps(t *testing.T, apiStub *fakeTunnelAPI, dialer *fakeDialer, timer *fakeTimer, sigs <-chan os.Signal) tunnelSessionDeps {
	t.Helper()
	var out, errw strings.Builder
	return tunnelSessionDeps{
		api:        apiStub,
		probeLocal: func(string, int) error { return nil }, // no-op: local dev server "up"
		// probePublic left nil by default → the readiness wait treats the host as
		// immediately reachable, so lifecycle tests that don't care about the wait are
		// unaffected. Tests exercising the wait set it explicitly.
		probePublic: nil,
		keygen:      goodKeygen,
		dialer:      dialer,
		blockID:     "my-block",
		port:        5186,
		endpoint:    "sish.example:2224",
		baseURL:     "https://civitai.com",
		idle:        30 * time.Minute,
		// Short readiness knobs so any wait-exercising test completes fast (the probe
		// is injected and returns instantly — no real network). nudgeAfter/quietInterval
		// default LARGE so they don't fire in tests that don't exercise them; tests
		// that do override them to a few ms.
		readyTimeout:      100 * time.Millisecond,
		readyPollInterval: time.Millisecond,
		nudgeAfter:        time.Hour,
		spinnerInterval:   10 * time.Millisecond,
		quietInterval:     time.Hour,
		newTimer:          func(time.Duration) devtunnel.Timer { return timer },
		signals:           sigs,
		out:               &out,
		errw:              &errw,
	}
}

// runInBackground runs runTunnelSession and returns a channel with its error.
func runInBackground(deps tunnelSessionDeps) <-chan error {
	errc := make(chan error, 1)
	go func() { errc <- runTunnelSession(context.Background(), deps) }()
	return errc
}

// ── tests ────────────────────────────────────────────────────────────────────

// TestValidateMintResponse: the mint response's host must match the P1
// dev-<16hex>.<domain> shape and its URL must be same-origin as the configured
// base — a compromised/malicious response can't steer the bind name or the URL.
func TestValidateMintResponse(t *testing.T) {
	const base = "https://civitai.com"
	cases := []struct {
		name    string
		host    string
		url     string
		hostKey string
		wantErr string // "" = should pass
	}{
		{"valid", "dev-0123456789abcdef.civit.ai", "https://civitai.com/apps/dev/my-block", sampleHostKey, ""},
		{"bad host prefix", "evil-0123456789abcdef.civit.ai", "https://civitai.com/apps/dev/x", sampleHostKey, "unexpected tunnel host"},
		{"bad host not-hex", "dev-zzzzzzzzzzzzzzzz.civit.ai", "https://civitai.com/apps/dev/x", sampleHostKey, "unexpected tunnel host"},
		{"bad host too-short", "dev-abc.civit.ai", "https://civitai.com/apps/dev/x", sampleHostKey, "unexpected tunnel host"},
		{"cross-origin host", "dev-0123456789abcdef.civit.ai", "https://evil.example/apps/dev/x", sampleHostKey, "cross-origin"},
		{"cross-origin scheme", "dev-0123456789abcdef.civit.ai", "http://civitai.com/apps/dev/x", sampleHostKey, "cross-origin"},
		{"unparseable url", "dev-0123456789abcdef.civit.ai", "://nope", sampleHostKey, "unparseable"},
		// FAIL CLOSED: an absent host key to pin is a hard rejection (no
		// InsecureIgnoreHostKey fallback anywhere).
		{"missing host key", "dev-0123456789abcdef.civit.ai", "https://civitai.com/apps/dev/x", "", "sish host key to pin"},
		{"blank host key", "dev-0123456789abcdef.civit.ai", "https://civitai.com/apps/dev/x", "   ", "sish host key to pin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMintResponse(&api.DevTunnelSession{Host: tc.host, URL: tc.url, SSHHostPublicKey: tc.hostKey}, base)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected pass, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestRunTunnelSessionRejectsBadMint: a mint response whose host fails validation
// is refused AND the just-minted session is revoked (no orphan), with no tunnel
// dial.
func TestRunTunnelSessionRejectsBadMint(t *testing.T) {
	bad := sampleSession()
	bad.Host = "evil-0123456789abcdef.civit.ai" // wrong prefix
	apiStub := &fakeTunnelAPI{startResult: bad}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "unexpected tunnel host") {
		t.Fatalf("expected a bad-host rejection, got %v", err)
	}
	if dialer.dialed {
		t.Error("must NOT dial the tunnel on a rejected mint response")
	}
	if apiStub.stopCount() != 1 || apiStub.stopCalls[0].sessionID != "bki_test" {
		t.Errorf("a rejected mint must be revoked (avoid orphan), stop calls=%+v", apiStub.stopCalls)
	}
}

// TestRunTunnelSessionRejectsCrossOriginURL: a same-shape host but a
// cross-origin URL is refused (the dev must not be handed an off-origin link).
func TestRunTunnelSessionRejectsCrossOriginURL(t *testing.T) {
	bad := sampleSession()
	bad.URL = "https://evil.example/apps/dev/my-block"
	apiStub := &fakeTunnelAPI{startResult: bad}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("expected a cross-origin URL rejection, got %v", err)
	}
	if apiStub.stopCount() != 1 {
		t.Errorf("a rejected mint must be revoked, stop calls=%d", apiStub.stopCount())
	}
}

// TestRunTunnelSessionSignalTeardown: SIGINT tears down — closes the tunnel and
// calls StopDevTunnel with the minted sessionId.
func TestRunTunnelSessionSignalTeardown(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)

	var out strings.Builder
	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.out = &out

	errc := runInBackground(deps)
	sigs <- os.Interrupt

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runTunnelSession did not return after signal")
	}

	// Started with the ephemeral pubkey.
	if len(apiStub.startCalls) != 1 || apiStub.startCalls[0].blockID != "my-block" {
		t.Fatalf("start calls = %+v", apiStub.startCalls)
	}
	if !strings.HasPrefix(apiStub.startCalls[0].pubKey, "ssh-ed25519 ") {
		t.Errorf("start should send an ed25519 public key, got %q", apiStub.startCalls[0].pubKey)
	}
	// Torn down.
	if !tunnel.closed {
		t.Error("tunnel was not closed on signal")
	}
	if apiStub.stopCount() != 1 || apiStub.stopCalls[0].sessionID != "bki_test" {
		t.Errorf("stop calls = %+v, want one stop for bki_test", apiStub.stopCalls)
	}
	// The /apps/dev URL was printed prominently.
	if !strings.Contains(out.String(), "https://civitai.com/apps/dev/my-block") {
		t.Errorf("stdout should print the /apps/dev URL:\n%s", out.String())
	}
}

// TestRunTunnelSessionIdleTeardown: the idle timer firing tears the tunnel down
// and revokes the session (client's part of the reaper contract).
func TestRunTunnelSessionIdleTeardown(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()

	deps := baseDeps(t, apiStub, dialer, timer, make(chan os.Signal))
	errc := runInBackground(deps)

	timer.fire()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runTunnelSession did not return after idle timeout")
	}
	if !tunnel.closed {
		t.Error("tunnel was not closed on idle timeout")
	}
	if apiStub.stopCount() != 1 {
		t.Errorf("idle timeout should call StopDevTunnel once, got %d", apiStub.stopCount())
	}
}

// TestRunTunnelSessionActivityResetsIdle: an activity event resets the idle
// timer (rather than tearing down) — proving the tunnel isn't reaped while used.
func TestRunTunnelSessionActivityResetsIdle(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()

	deps := baseDeps(t, apiStub, dialer, timer, make(chan os.Signal))
	errc := runInBackground(deps)

	// A connection arrives → the loop should Reset the idle timer, not tear down.
	tunnel.activity <- struct{}{}

	// Give the loop a moment, then confirm it's still running (no teardown yet).
	deadline := time.After(500 * time.Millisecond)
	for {
		if timer.resetCount() >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("activity did not reset the idle timer (resets=%d)", timer.resetCount())
		case err := <-errc:
			t.Fatalf("session exited early on activity (err=%v)", err)
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
	if apiStub.stopCount() != 0 {
		t.Errorf("activity must NOT tear down; stop calls=%d", apiStub.stopCount())
	}

	// Now really stop it (tunnel closes on its own) so the goroutine exits.
	close(tunnel.done)
	select {
	case <-errc:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after tunnel closed")
	}
}

// TestRunTunnelSessionTunnelClosedTeardown: the tunnel terminating on its own
// exits the loop and revokes the session.
func TestRunTunnelSessionTunnelClosedTeardown(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()

	deps := baseDeps(t, apiStub, dialer, timer, make(chan os.Signal))
	errc := runInBackground(deps)

	close(tunnel.done)
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after tunnel closed")
	}
	if apiStub.stopCount() != 1 {
		t.Errorf("tunnel-close should revoke the session, stop calls=%d", apiStub.stopCount())
	}
}

// TestRunTunnelSessionStartError: a failed mint returns the error and NEVER
// dials the tunnel or calls stop (nothing to tear down).
func TestRunTunnelSessionStartError(t *testing.T) {
	apiStub := &fakeTunnelAPI{startErr: errors.New("FORBIDDEN: dev tunnels not available")}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}
	timer := newFakeTimer()

	deps := baseDeps(t, apiStub, dialer, timer, make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("expected the start error to propagate, got %v", err)
	}
	if dialer.dialed {
		t.Error("should NOT dial the tunnel when the mint failed")
	}
	if apiStub.stopCount() != 0 {
		t.Error("should NOT call stop when nothing was started")
	}
}

// TestRunTunnelSessionForbiddenEnrichedWithIdentity: a 403 mint refusal is
// enriched with the signed-in account + account-switch guidance, still errors.As
// to *api.DevTunnelForbiddenError, and NEVER dials/stops (it's on the mint-error
// path, before any tunnel dial).
func TestRunTunnelSessionForbiddenEnrichedWithIdentity(t *testing.T) {
	apiStub := &fakeTunnelAPI{
		startErr: &api.DevTunnelForbiddenError{ServerMsg: "Dev tunnels are not available"},
		whoami:   &api.Identity{Username: "zach", ID: 42},
	}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil {
		t.Fatal("expected the forbidden mint to error")
	}
	// (a) still unwraps to the typed forbidden error.
	var forbidden *api.DevTunnelForbiddenError
	if !errors.As(err, &forbidden) {
		t.Fatalf("enriched error must still errors.As to *api.DevTunnelForbiddenError, got %v", err)
	}
	// (b) names the account + carries the switch/check guidance.
	msg := err.Error()
	for _, want := range []string{"signed in as zach", "id 42", "civitai login", "civitai whoami", "not available", "403"} {
		if !strings.Contains(msg, want) {
			t.Errorf("enriched error missing %q: %v", want, msg)
		}
	}
	// (c) never dialed and never stopped (nothing was minted).
	if dialer.dialed {
		t.Error("a forbidden mint must NOT dial the tunnel")
	}
	if apiStub.stopCount() != 0 {
		t.Errorf("a forbidden mint must NOT stop (nothing minted), stop calls=%d", apiStub.stopCount())
	}
	if apiStub.whoamiCount() != 1 {
		t.Errorf("WhoAmI should be consulted once to name the account, got %d", apiStub.whoamiCount())
	}
}

// TestRunTunnelSessionForbiddenScopeGuidance: a token-SCOPE 403 gives full-scope-key
// guidance (NOT account-switching), and does NOT consult WhoAmI (the account is
// fine — the credential's scope is the problem).
func TestRunTunnelSessionForbiddenScopeGuidance(t *testing.T) {
	apiStub := &fakeTunnelAPI{
		startErr: &api.DevTunnelForbiddenError{
			ServerMsg:         "Your API key does not have the required scope for this action",
			InsufficientScope: true,
		},
		whoami: &api.Identity{Username: "zach", ID: 42},
	}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil {
		t.Fatal("expected the forbidden mint to error")
	}
	var forbidden *api.DevTunnelForbiddenError
	if !errors.As(err, &forbidden) {
		t.Fatalf("enriched error must still errors.As to *api.DevTunnelForbiddenError, got %v", err)
	}
	msg := err.Error()
	// Scope-specific guidance — full-scope key + whoami, NOT account-switching.
	for _, want := range []string{"full-scope personal API key", "civitai login --token", "civitai whoami", "403"} {
		if !strings.Contains(msg, want) {
			t.Errorf("scope-403 guidance missing %q: %v", want, msg)
		}
	}
	if strings.Contains(msg, "Switch accounts") || strings.Contains(msg, "signed in as") {
		t.Errorf("a scope-403 must NOT misdirect to account-switching: %v", msg)
	}
	// The account is not the problem, so WhoAmI is not consulted; nothing dialed.
	if apiStub.whoamiCount() != 0 {
		t.Errorf("a scope-403 must NOT consult WhoAmI, got %d", apiStub.whoamiCount())
	}
	if dialer.dialed {
		t.Error("a forbidden mint must NOT dial the tunnel")
	}
}

// TestRunTunnelSessionForbiddenWhoAmIFails: a 403 mint with a failing WhoAmI still
// carries the switch guidance but omits the account name (best effort).
func TestRunTunnelSessionForbiddenWhoAmIFails(t *testing.T) {
	apiStub := &fakeTunnelAPI{
		startErr:  &api.DevTunnelForbiddenError{ServerMsg: "Dev tunnels are not available"},
		whoamiErr: errors.New("token expired"),
	}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil {
		t.Fatal("expected the forbidden mint to error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "civitai login") || !strings.Contains(msg, "civitai whoami") {
		t.Errorf("guidance must survive a failed WhoAmI: %v", msg)
	}
	if strings.Contains(msg, "signed in as") {
		t.Errorf("a failed WhoAmI must NOT claim an account name: %v", msg)
	}
	if dialer.dialed {
		t.Error("a forbidden mint must NOT dial the tunnel")
	}
}

// TestRunTunnelSessionNonForbiddenErrorPassesThrough: a NON-forbidden mint error
// passes through unchanged and does NOT consult WhoAmI.
func TestRunTunnelSessionNonForbiddenErrorPassesThrough(t *testing.T) {
	apiStub := &fakeTunnelAPI{
		startErr: errors.New("boom: transient server error"),
		whoami:   &api.Identity{Username: "zach", ID: 42},
	}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || err.Error() != "boom: transient server error" {
		t.Fatalf("a non-forbidden error must pass through unchanged, got %v", err)
	}
	if apiStub.whoamiCount() != 0 {
		t.Errorf("a non-forbidden error must NOT consult WhoAmI, got %d", apiStub.whoamiCount())
	}
	if dialer.dialed {
		t.Error("a failed mint must NOT dial the tunnel")
	}
}

// TestRunTunnelSessionDialError: the mint succeeds but the tunnel dial fails —
// the session is revoked (so no orphaned route) and the error propagates.
func TestRunTunnelSessionDialError(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	dialer := &fakeDialer{err: errors.New("connection refused")}
	timer := newFakeTimer()

	deps := baseDeps(t, apiStub, dialer, timer, make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "reverse tunnel") {
		t.Fatalf("expected a tunnel-dial error, got %v", err)
	}
	if apiStub.stopCount() != 1 {
		t.Errorf("a failed dial must revoke the minted session (avoid orphan), stop calls=%d", apiStub.stopCount())
	}
}

// TestRunTunnelSessionProbeError: the pre-flight local-dev-server probe runs
// FIRST — a probe failure returns that error verbatim and NEVER mints (no
// keygen-dependent StartDevTunnel, no dial), so a not-running dev server can't
// burn a rate-limited/reaper-tracked server session.
func TestRunTunnelSessionProbeError(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}
	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	probeErr := errors.New("no local dev server is listening on 127.0.0.1:5186")
	deps.probeLocal = func(host string, port int) error {
		if port != 5186 {
			t.Errorf("probe got port %d, want the session port 5186", port)
		}
		return probeErr
	}

	err := runTunnelSession(context.Background(), deps)
	if err == nil || !errors.Is(err, probeErr) {
		t.Fatalf("expected the probe error to propagate verbatim, got %v", err)
	}
	if apiStub.startCount() != 0 {
		t.Errorf("a failed pre-flight probe must NOT mint (StartDevTunnel calls=%d)", apiStub.startCount())
	}
	if dialer.dialed {
		t.Error("a failed pre-flight probe must NOT dial the tunnel")
	}
	if apiStub.stopCount() != 0 {
		t.Error("nothing was minted, so there is nothing to stop")
	}
}

// TestRunTunnelSessionKeygenError: a keygen failure aborts before any API call.
func TestRunTunnelSessionKeygenError(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	deps := baseDeps(t, apiStub, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	deps.keygen = func() (*devtunnel.EphemeralKey, error) { return nil, errors.New("no entropy") }

	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "ephemeral") {
		t.Fatalf("expected a keygen error, got %v", err)
	}
	if len(apiStub.startCalls) != 0 {
		t.Error("must not call StartDevTunnel when keygen failed")
	}
}

// TestRunTunnelSessionDialOptions: the dialer receives the SERVER-derived host +
// the configured endpoint/port (host is never client-chosen).
func TestRunTunnelSessionDialOptions(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)

	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	errc := runInBackground(deps)
	sigs <- os.Interrupt
	<-errc

	if dialer.lastOpt.RemoteHost != "dev-0123456789abcdef.civit.ai" {
		t.Errorf("dial RemoteHost = %q, want the server-assigned host", dialer.lastOpt.RemoteHost)
	}
	if dialer.lastOpt.Endpoint != "sish.example:2224" || dialer.lastOpt.LocalPort != 5186 {
		t.Errorf("dial opts endpoint/port = %q/%d", dialer.lastOpt.Endpoint, dialer.lastOpt.LocalPort)
	}
	if dialer.lastOpt.Signer == nil {
		t.Error("dial must carry the ephemeral signer")
	}
	// R1: the mint's sish host key is plumbed through to the dialer so it can be
	// PINNED (never InsecureIgnoreHostKey).
	if dialer.lastOpt.SSHHostPublicKey != sampleHostKey {
		t.Errorf("dial SSHHostPublicKey = %q, want the mint-provided host key %q", dialer.lastOpt.SSHHostPublicKey, sampleHostKey)
	}
}

// TestRunTunnelSessionRejectsMissingHostKey: FAIL CLOSED — a mint response that
// omits sshHostPublicKey is refused (clear message), the just-minted session is
// revoked (no orphan), and the tunnel is NEVER dialed (so no unverified/
// InsecureIgnoreHostKey connection is ever attempted).
func TestRunTunnelSessionRejectsMissingHostKey(t *testing.T) {
	bad := sampleSession()
	bad.SSHHostPublicKey = "" // mint did not provide a host key to pin
	apiStub := &fakeTunnelAPI{startResult: bad}
	dialer := &fakeDialer{tunnel: newFakeTunnel()}

	deps := baseDeps(t, apiStub, dialer, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "sish host key to pin") {
		t.Fatalf("expected a fail-closed missing-host-key rejection, got %v", err)
	}
	if dialer.dialed {
		t.Error("must NOT dial the tunnel when no host key was provided to pin")
	}
	if apiStub.stopCount() != 1 || apiStub.stopCalls[0].sessionID != "bki_test" {
		t.Errorf("a rejected mint must be revoked (avoid orphan), stop calls=%+v", apiStub.stopCalls)
	}
}

// ── readiness-wait tests ─────────────────────────────────────────────────────

const testTunnelHost = "dev-0123456789abcdef.civit.ai"
const testTunnelURL = "https://civitai.com/apps/dev/my-block"

// TestWaitForTunnelReachableReadyAfterRetries: the wait re-probes until the public
// host reports ready (200/401/403), then returns (true, ""). Crucially it does NOT
// emit the old one-line-per-attempt spam (that was replaced by the spinner /
// heartbeat indicator).
func TestWaitForTunnelReachableReadyAfterRetries(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var out, errw syncBuffer
	deps.out = &out
	deps.errw = &errw // syncBuffer → non-TTY path
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 5 * time.Second

	// Probe runs in a goroutine now, so guard the counter.
	var mu sync.Mutex
	calls := 0
	deps.probePublic = func(_ context.Context, host string) (bool, string, error) {
		if host != testTunnelHost {
			t.Errorf("probe got host %q, want %q", host, testTunnelHost)
		}
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n < 4 {
			return false, "http 404", nil
		}
		return true, "http 401", nil
	}

	ready, abort := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
	if !ready || abort != "" {
		t.Fatalf("want (true, \"\"), got (%v, %q)", ready, abort)
	}
	mu.Lock()
	got := calls
	mu.Unlock()
	if got < 4 {
		t.Fatalf("expected at least 4 probes (3 not-ready + 1 ready), got %d", got)
	}
	// The old per-attempt "waiting for … to come up (http 404)" spam is gone.
	if strings.Contains(errw.String(), "to come up (http 404)") {
		t.Errorf("must NOT emit one-line-per-attempt spam:\n%s", errw.String())
	}
}

// TestWaitForTunnelReachableQuietHeartbeat: on the non-TTY path the wait emits the
// quiet, greppable "still waiting" heartbeat at most every quietInterval (NOT one
// per probe attempt), then the host becomes ready.
func TestWaitForTunnelReachableQuietHeartbeat(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var errw syncBuffer
	deps.errw = &errw // syncBuffer → non-TTY path
	deps.readyPollInterval = 2 * time.Millisecond
	deps.readyTimeout = 0 // indefinite
	deps.quietInterval = 3 * time.Millisecond

	// Stay not-ready until the test has observed at least one heartbeat line, then
	// flip to ready so the wait finishes.
	release := make(chan struct{})
	deps.probePublic = func(_ context.Context, _ string) (bool, string, error) {
		select {
		case <-release:
			return true, "http 401", nil
		default:
			return false, "http 404", nil
		}
	}

	resc := make(chan bool, 1)
	go func() {
		ready, _ := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- ready
	}()

	// Wait for a heartbeat line to appear.
	deadline := time.After(2 * time.Second)
	for !strings.Contains(errw.String(), "still waiting for "+testTunnelHost) {
		select {
		case <-deadline:
			t.Fatalf("no quiet heartbeat line appeared:\n%s", errw.String())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(release) // let the next probe report ready
	select {
	case ready := <-resc:
		if !ready {
			t.Fatal("wait should end ready once the probe flips")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not finish after the probe flipped ready")
	}
	// No \r animation on the non-TTY path.
	if strings.Contains(errw.String(), "\r") {
		t.Errorf("non-TTY path must not emit carriage-return animation:\n%q", errw.String())
	}
}

// TestWaitForTunnelReachableAbortOnSignal: Ctrl-C during the wait returns
// (false, "interrupt") so the caller tears the session down instead of proceeding.
func TestWaitForTunnelReachableAbortOnSignal(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), sigs)
	var errw syncBuffer
	deps.errw = &errw
	deps.readyPollInterval = 20 * time.Millisecond
	deps.readyTimeout = 5 * time.Second
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return false, "http 502", nil // never ready
	}

	type res struct {
		ready bool
		abort string
	}
	resc := make(chan res, 1)
	go func() {
		r, a := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- res{r, a}
	}()
	sigs <- os.Interrupt

	select {
	case got := <-resc:
		if got.ready || got.abort != "interrupt" {
			t.Fatalf("want (false, \"interrupt\"), got (%v, %q)", got.ready, got.abort)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not abort on signal")
	}
}

// TestWaitForTunnelReachableAbortOnTunnelClose: a tunnel that drops mid-wait ends
// the wait with the "tunnel closed" reason.
func TestWaitForTunnelReachableAbortOnTunnelClose(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var errw syncBuffer
	deps.errw = &errw
	deps.readyPollInterval = 20 * time.Millisecond
	deps.readyTimeout = 5 * time.Second
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return false, "http 404", nil
	}

	tun := newFakeTunnel()
	resc := make(chan string, 1)
	go func() {
		_, a := waitForTunnelReachable(context.Background(), deps, tun, testTunnelHost, testTunnelURL)
		resc <- a
	}()
	close(tun.done)

	select {
	case a := <-resc:
		if a != "tunnel closed" {
			t.Fatalf("want abort reason \"tunnel closed\", got %q", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not end when the tunnel closed")
	}
}

// TestWaitForTunnelReachableNilProbeSkips: a nil probePublic disables the wait
// (returns ready immediately) — so lifecycle tests that don't wire a probe are
// unaffected.
func TestWaitForTunnelReachableNilProbeSkips(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	deps.probePublic = nil
	ready, abort := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
	if !ready || abort != "" {
		t.Fatalf("nil probe should return (true, \"\"), got (%v, %q)", ready, abort)
	}
}

// TestWaitForTunnelReachableIndefiniteDefault: with readyTimeout == 0 (the new
// default) a host that never becomes reachable must NOT make the wait return —
// it keeps polling until an abort (signal / tunnel-close / ctx). It must NEVER
// return (false, "") (that's the positive-cap path, which the default disables).
func TestWaitForTunnelReachableIndefiniteDefault(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), sigs)
	var errw syncBuffer
	deps.errw = &errw
	deps.readyTimeout = 0 // indefinite
	deps.readyPollInterval = time.Millisecond
	deps.quietInterval = time.Hour // keep stderr quiet

	var probes int
	var mu sync.Mutex
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		mu.Lock()
		probes++
		mu.Unlock()
		return false, "http 404", nil // never ready
	}

	type res struct {
		ready bool
		abort string
	}
	resc := make(chan res, 1)
	go func() {
		r, a := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- res{r, a}
	}()

	// Give it long enough to have polled many times and — critically — long past any
	// legacy 120s default would be irrelevant; the point is it must still be running.
	select {
	case got := <-resc:
		t.Fatalf("indefinite wait returned prematurely: (%v, %q)", got.ready, got.abort)
	case <-time.After(60 * time.Millisecond):
	}
	mu.Lock()
	if probes < 2 {
		t.Errorf("expected the wait to keep re-probing, got %d probes", probes)
	}
	mu.Unlock()

	// Now abort it — proves it was still alive and responsive.
	sigs <- os.Interrupt
	select {
	case got := <-resc:
		if got.ready || got.abort != "interrupt" {
			t.Fatalf("want (false, \"interrupt\"), got (%v, %q)", got.ready, got.abort)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("indefinite wait did not abort on signal")
	}
}

// TestWaitForTunnelReachableNudgeFiresOnce: after nudgeAfter elapses the one-time
// "taking longer than usual" nudge (with the URL) is printed EXACTLY once, and the
// wait keeps going.
func TestWaitForTunnelReachableNudgeFiresOnce(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), sigs)
	var errw syncBuffer
	deps.errw = &errw
	deps.readyTimeout = 0
	deps.readyPollInterval = time.Millisecond
	deps.quietInterval = time.Hour
	deps.nudgeAfter = 5 * time.Millisecond
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return false, "http 404", nil // never ready → wait keeps running
	}

	resc := make(chan struct{}, 1)
	go func() {
		waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- struct{}{}
	}()

	// Wait for the nudge to appear.
	deadline := time.After(2 * time.Second)
	for !strings.Contains(errw.String(), "Taking longer than usual") {
		select {
		case <-deadline:
			t.Fatalf("nudge never printed:\n%s", errw.String())
		case <-resc:
			t.Fatalf("wait exited before nudging:\n%s", errw.String())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	// Let it run well past nudgeAfter again to prove the nudge does NOT repeat.
	time.Sleep(50 * time.Millisecond)
	sigs <- os.Interrupt
	select {
	case <-resc:
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not abort on signal")
	}

	if n := strings.Count(errw.String(), "Taking longer than usual"); n != 1 {
		t.Errorf("nudge must fire exactly once, fired %d times:\n%s", n, errw.String())
	}
	if !strings.Contains(errw.String(), testTunnelURL) {
		t.Errorf("nudge must include the URL so the dev can act:\n%s", errw.String())
	}
}

// TestWaitForTunnelReachableCancelsInFlightProbe: a signal arriving WHILE a probe
// is in flight must make the wait return PROMPTLY, canceling the probe's context
// (not riding out its ~8s HTTP timeout). Proven by a probe that blocks until its
// context is canceled.
func TestWaitForTunnelReachableCancelsInFlightProbe(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), sigs)
	var errw syncBuffer
	deps.errw = &errw
	deps.readyTimeout = 0
	deps.quietInterval = time.Hour
	deps.nudgeAfter = time.Hour

	probeStarted := make(chan struct{}, 1)
	probeCanceled := make(chan struct{})
	deps.probePublic = func(pctx context.Context, _ string) (bool, string, error) {
		select {
		case probeStarted <- struct{}{}:
		default:
		}
		<-pctx.Done() // block until the wait cancels us
		close(probeCanceled)
		return false, "canceled", pctx.Err()
	}

	type res struct {
		ready bool
		abort string
	}
	resc := make(chan res, 1)
	go func() {
		r, a := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- res{r, a}
	}()

	// Wait until the probe is actually in flight, THEN signal.
	select {
	case <-probeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("probe never started")
	}
	sigs <- os.Interrupt

	select {
	case got := <-resc:
		if got.ready || got.abort != "interrupt" {
			t.Fatalf("want (false, \"interrupt\"), got (%v, %q)", got.ready, got.abort)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not return promptly on signal during an in-flight probe")
	}
	// The in-flight probe's context WAS canceled (snappy Ctrl-C, not an 8s wait).
	select {
	case <-probeCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("the in-flight probe's context was not canceled")
	}
}

// TestRunTunnelSessionReadyPrintsConfirmedURL: on the ready path runTunnelSession
// prints the ✓ Ready confirmation + the /apps/dev URL (only after the wait
// succeeds), with the "waiting…" progress on stderr.
func TestRunTunnelSessionReadyPrintsConfirmedURL(t *testing.T) {
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
	// Not-ready twice, then ready. Only the session goroutine touches `calls`.
	calls := 0
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		calls++
		if calls < 3 {
			return false, "http 502", nil
		}
		return true, "http 401", nil
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
	if !strings.Contains(out.String(), sampleSession().URL) {
		t.Errorf("ready path must print the /apps/dev URL:\n%s", out.String())
	}
	if !strings.Contains(errw.String(), "Waiting for it to become reachable") {
		t.Errorf("expected the 'established, waiting' status on stderr:\n%s", errw.String())
	}
	// Change 2: the immediate startup status fills the previously-silent mint+dial
	// window (printed BEFORE keygen/mint, so the terminal never looks hung).
	if !strings.Contains(errw.String(), "Establishing dev tunnel for my-block") {
		t.Errorf("expected the immediate 'establishing' status on stderr:\n%s", errw.String())
	}
	// Change 3: the redundant "Make sure your dev server is running" line is gone —
	// probeLocal already guaranteed the dev server was listening before the mint.
	if strings.Contains(out.String(), "Make sure your dev server is running") {
		t.Errorf("ready block must NOT print the redundant dev-server-running noise:\n%s", out.String())
	}
}

// TestRunTunnelSessionReadinessTimeoutNonFatal: when the host never becomes
// reachable, the readiness deadline elapses, the WARNING + URL are printed, and
// the tunnel is NOT torn down (it may still come up) — it stays alive in the idle
// loop until an actual signal.
func TestRunTunnelSessionReadinessTimeoutNonFatal(t *testing.T) {
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
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return false, "http 404", nil // never ready → deadline elapses
	}

	errc := runInBackground(deps)
	// Wait for the URL — the last-relevant line of the timeout block. The block is
	// several sequential Fprintf calls, so waiting on the FIRST line (the warning)
	// then asserting the LATER URL line races the writes (flaky under CI scheduling).
	if !waitForOutput(t, &out, sampleSession().URL, errc) {
		return
	}
	if !strings.Contains(out.String(), "isn't resolving/serving yet") {
		t.Errorf("timeout path must print the warning:\n%s", out.String())
	}
	// Change 3: the timeout block also drops the redundant dev-server-running noise.
	if strings.Contains(out.String(), "Make sure your dev server is running") {
		t.Errorf("timeout block must NOT print the redundant dev-server-running noise:\n%s", out.String())
	}
	// The readiness timeout is non-fatal: nothing torn down, session still alive.
	if apiStub.stopCount() != 0 {
		t.Errorf("readiness timeout must NOT tear the tunnel down; stop calls=%d", apiStub.stopCount())
	}
	// Now really end it.
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}
	if !tunnel.closed || apiStub.stopCount() != 1 {
		t.Errorf("expected exactly one teardown on the follow-up signal (closed=%v, stops=%d)", tunnel.closed, apiStub.stopCount())
	}
}

// TestRunTunnelSessionNoWaitSkipsProbe: --no-wait prints the URL immediately and
// NEVER calls probePublic.
func TestRunTunnelSessionNoWaitSkipsProbe(t *testing.T) {
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
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		t.Error("probePublic must NOT be called when --no-wait is set")
		return false, "", nil
	}

	errc := runInBackground(deps)
	sigs <- os.Interrupt
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runTunnelSession: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after signal")
	}
	if !strings.Contains(out.String(), sampleSession().URL) {
		t.Errorf("--no-wait must print the URL immediately:\n%s", out.String())
	}
	if strings.Contains(out.String(), "✓ Ready") || strings.Contains(out.String(), "isn't resolving") {
		t.Errorf("--no-wait must use the plain ready block (no wait outcome):\n%s", out.String())
	}
	// Change 2: even on --no-wait (which pre-probes then skips the reachability
	// wait), the immediate startup status still fills the mint+dial window.
	if !strings.Contains(errw.String(), "Establishing dev tunnel for my-block") {
		t.Errorf("--no-wait must still print the immediate 'establishing' status:\n%s", errw.String())
	}
	// Change 3: the plain ready block drops the redundant dev-server-running noise.
	if strings.Contains(out.String(), "Make sure your dev server is running") {
		t.Errorf("--no-wait ready block must NOT print the redundant dev-server-running noise:\n%s", out.String())
	}
}

// waitForOutput polls buf until it contains want, failing if the session exits
// (via errc) first or the deadline passes. Returns true on success. Uses the
// race-safe syncBuffer so reading buf while the session goroutine writes is safe.
func waitForOutput(t *testing.T, buf *syncBuffer, want string, errc <-chan error) bool {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(buf.String(), want) {
			return true
		}
		select {
		case <-deadline:
			t.Fatalf("output never contained %q:\n%s", want, buf.String())
			return false
		case err := <-errc:
			t.Fatalf("session exited before printing %q (err=%v):\n%s", want, err, buf.String())
			return false
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// ── --local-host + local-hop readiness tests ─────────────────────────────────

// TestRunTunnelSessionThreadsLocalHost: a set localHost reaches BOTH the pre-flight
// probe AND the tunnel proxy (the DialOptions), so the two agree on where the
// local dev server lives.
func TestRunTunnelSessionThreadsLocalHost(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	tunnel := newFakeTunnel()
	dialer := &fakeDialer{tunnel: tunnel}
	timer := newFakeTimer()
	sigs := make(chan os.Signal, 1)

	deps := baseDeps(t, apiStub, dialer, timer, sigs)
	deps.localHost = "10.42.0.100"
	var gotProbeHost string
	deps.probeLocal = func(host string, port int) error {
		gotProbeHost = host
		if port != 5186 {
			t.Errorf("probe port = %d, want 5186", port)
		}
		return nil
	}

	errc := runInBackground(deps)
	sigs <- os.Interrupt
	<-errc

	if gotProbeHost != "10.42.0.100" {
		t.Errorf("pre-flight probe got host %q, want the --local-host value", gotProbeHost)
	}
	if dialer.lastOpt.LocalHost != "10.42.0.100" {
		t.Errorf("dial LocalHost = %q, want the --local-host value threaded to the proxy", dialer.lastOpt.LocalHost)
	}
}

// TestRunTunnelSessionDefaultLocalHostViaCommand: the --local-host flag defaults to
// "localhost" (loopback), preserving the pre-flag behavior, and threads that
// default all the way to the dialer.
func TestRunTunnelSessionDefaultLocalHostViaCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == startDevTunnelRoute {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": "Dev tunnels are not available"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"username": "tester", "id": 7})
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	port := listenLocal(t)
	// No --local-host given → the pre-flight probe must still pass against the
	// loopback listener (default "localhost"), so we get PAST the probe to the
	// (expected pre-GA) forbidden mint — never a "no local dev server" error.
	_, _, err := run(t, "app", "dev-tunnel", "my-block", "--port", fmt.Sprint(port))
	if err == nil {
		t.Fatal("expected a forbidden mint error (flag dark)")
	}
	if strings.Contains(err.Error(), "no local dev server") {
		t.Fatalf("default localhost probe must reach the loopback listener, got: %v", err)
	}
}

// TestRunTunnelSessionLocalHostFlagPreflightUsesHost: an explicit --local-host that
// points nowhere fails the pre-flight probe with a message naming the host + the
// --local-host flag (so a wrong host is actionable), and NEVER mints.
func TestRunTunnelSessionLocalHostFlagPreflightUsesHost(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")

	// 192.0.2.1 is TEST-NET-1 (RFC 5737) — guaranteed unreachable, so the probe
	// fails deterministically without minting. A short dial timeout keeps it fast
	// (the production probe uses 2s).
	_, _, err := run(t, "app", "dev-tunnel", "my-block", "--local-host", "192.0.2.1", "--port", "5186")
	if err == nil {
		t.Fatal("expected a pre-flight failure against an unreachable --local-host")
	}
	if !strings.Contains(err.Error(), "192.0.2.1") || !strings.Contains(err.Error(), "--local-host") {
		t.Errorf("pre-flight error should name the host + --local-host: %v", err)
	}
}

// TestLocalHopReachable: only the bad-gateway family (502/503/504) means the tunnel
// is up but the local dev server is unreachable; every other status means the local
// server responded (the hop works).
func TestLocalHopReachable(t *testing.T) {
	unreachable := []int{502, 503, 504}
	reachable := []int{200, 201, 204, 301, 400, 401, 403, 404, 429, 500}
	for _, c := range unreachable {
		if localHopReachable(c) {
			t.Errorf("status %d should mean LOCAL UNREACHABLE (bad gateway)", c)
		}
	}
	for _, c := range reachable {
		if !localHopReachable(c) {
			t.Errorf("status %d should mean the local hop RESPONDED (reachable)", c)
		}
	}
}

// TestProbeLocalHopURLClassification: exercise probeLocalHopURL against a real
// httptest server — 502/503/504 → not reachable, others → reachable — and assert
// the subresource-shaped Sec-Fetch-Dest header is sent (so the gate forwards it to
// the backend rather than demanding a token).
func TestProbeLocalHopURLClassification(t *testing.T) {
	cases := []struct {
		code      int
		reachable bool
	}{
		{200, true}, {404, true}, {401, true}, {403, true}, {500, true},
		{502, false}, {503, false}, {504, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("http_%d", tc.code), func(t *testing.T) {
			var gotDest string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotDest = r.Header.Get("Sec-Fetch-Dest")
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()

			reachable, detail, err := probeLocalHopURL(context.Background(), srv.Client(), srv.URL+"/")
			if err != nil {
				t.Fatalf("unexpected transport error for %d: %v", tc.code, err)
			}
			if reachable != tc.reachable {
				t.Errorf("status %d: reachable=%v, want %v", tc.code, reachable, tc.reachable)
			}
			if want := fmt.Sprintf("http %d", tc.code); detail != want {
				t.Errorf("detail = %q, want %q", detail, want)
			}
			// The gate forwards NON-entry Sec-Fetch-Dest to the backend; "image" is a
			// subresource dest (NOT document/iframe/frame/nested-document/absent).
			if gotDest != "image" {
				t.Errorf("local-hop probe must send a subresource Sec-Fetch-Dest (image), got %q", gotDest)
			}
		})
	}
}

// TestProbeLocalHopURLUnreachable: a transport error (closed port) is returned as
// an error (not a false "reachable") so the wait treats it as inconclusive, not as
// a local-server-down signal.
func TestProbeLocalHopURLUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	client := &http.Client{Timeout: time.Second}
	reachable, _, err := probeLocalHopURL(context.Background(), client, closedURL+"/")
	if err == nil {
		t.Fatal("expected a transport error against a closed port")
	}
	if reachable {
		t.Error("a transport error must not report the local hop as reachable")
	}
}

// TestWaitForTunnelReachableReadyRequiresLocalHop: with a probeLocalHop wired, the
// wait declares ready ONLY when the gate is reachable AND the local hop is not a
// bad-gateway — the core fix (gate-only readiness masked a dead local server).
func TestWaitForTunnelReachableReadyRequiresLocalHop(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var errw syncBuffer
	deps.errw = &errw
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 5 * time.Second
	deps.localHost = "10.42.0.100"
	deps.localHopNoticeInterval = time.Millisecond

	// Gate is reachable from the first probe. The local hop 502s for the first few
	// attempts (dev server not up), then responds — only THEN is the wait ready.
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return true, "http 401", nil // gate always up
	}
	var mu sync.Mutex
	localCalls := 0
	deps.probeLocalHop = func(context.Context, string) (bool, string, error) {
		mu.Lock()
		localCalls++
		n := localCalls
		mu.Unlock()
		if n < 3 {
			return false, "http 502", nil // tunnel up, local server down
		}
		return true, "http 404", nil // local server now responds
	}

	ready, abort := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
	if !ready || abort != "" {
		t.Fatalf("want (true, \"\") once the local hop responds, got (%v, %q)", ready, abort)
	}
	mu.Lock()
	got := localCalls
	mu.Unlock()
	if got < 3 {
		t.Fatalf("expected the wait to re-probe the local hop until it responded, got %d local probes", got)
	}
	// The clear local-unreachable guidance was surfaced while the hop was 502-ing.
	if !strings.Contains(errw.String(), "local dev server on 10.42.0.100:5186 is unreachable") {
		t.Errorf("expected the clear local-unreachable notice naming host:port:\n%s", errw.String())
	}
	if !strings.Contains(errw.String(), "--local-host") {
		t.Errorf("the notice must reference --local-host:\n%s", errw.String())
	}
}

// TestWaitForTunnelReachableLocalHopDownKeepsWaiting: gate reachable but the local
// hop 502s forever → the wait must NOT declare ready and must NOT return the
// positive-cap (false, "") until the deadline; it keeps waiting, surfacing the
// notice, until an abort. Proven by an indefinite wait that we abort by signal.
func TestWaitForTunnelReachableLocalHopDownKeepsWaiting(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), sigs)
	var errw syncBuffer
	deps.errw = &errw
	deps.readyTimeout = 0 // indefinite
	deps.readyPollInterval = time.Millisecond
	deps.quietInterval = time.Hour
	deps.localHopNoticeInterval = time.Millisecond
	deps.localHost = "10.42.0.100"

	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return true, "http 401", nil // gate up
	}
	var mu sync.Mutex
	localProbes := 0
	deps.probeLocalHop = func(context.Context, string) (bool, string, error) {
		mu.Lock()
		localProbes++
		mu.Unlock()
		return false, "http 502", nil // local server never comes up
	}

	type res struct {
		ready bool
		abort string
	}
	resc := make(chan res, 1)
	go func() {
		r, a := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- res{r, a}
	}()

	// It must NOT return while the local hop is down.
	select {
	case got := <-resc:
		t.Fatalf("wait returned while the local hop was still down: (%v, %q)", got.ready, got.abort)
	case <-time.After(60 * time.Millisecond):
	}
	mu.Lock()
	if localProbes < 2 {
		t.Errorf("expected repeated local-hop probes, got %d", localProbes)
	}
	mu.Unlock()
	// The clear notice was surfaced (persistent).
	if !strings.Contains(errw.String(), "is unreachable through the tunnel") {
		t.Errorf("expected the persistent local-unreachable notice:\n%s", errw.String())
	}

	// Abort proves it was still alive and responsive.
	sigs <- os.Interrupt
	select {
	case got := <-resc:
		if got.ready || got.abort != "interrupt" {
			t.Fatalf("want (false, \"interrupt\"), got (%v, %q)", got.ready, got.abort)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not abort on signal while the local hop was down")
	}
}

// TestWaitForTunnelReachableNilLocalHopIsGateOnly: a nil probeLocalHop preserves
// the old gate-only readiness (so lifecycle tests that don't wire it are
// unaffected) — gate reachable → ready immediately.
func TestWaitForTunnelReachableNilLocalHopIsGateOnly(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var errw syncBuffer
	deps.errw = &errw
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 5 * time.Second
	deps.probeLocalHop = nil // gate-only
	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return true, "http 401", nil
	}
	ready, abort := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
	if !ready || abort != "" {
		t.Fatalf("nil probeLocalHop should be gate-only ready, got (%v, %q)", ready, abort)
	}
}

// TestClassifyReadyStatus: only 200/401/403 mean the full server path is live; a
// naked no-token request being DENIED with 401/403 proves DNS→Traefik→forwardAuth
// are all up. 404 (no route yet), 5xx, and Cloudflare 520–526 are NOT ready.
func TestClassifyReadyStatus(t *testing.T) {
	ready := []int{200, 401, 403}
	notReady := []int{404, 500, 502, 503, 504, 520, 521, 522, 523, 524, 525, 526, 301, 400, 429}
	for _, c := range ready {
		if !classifyReadyStatus(c) {
			t.Errorf("status %d should be READY", c)
		}
	}
	for _, c := range notReady {
		if classifyReadyStatus(c) {
			t.Errorf("status %d should be NOT ready", c)
		}
	}
}

// TestProbePublicURLClassification: exercise probePublicURL against a real
// httptest server returning each status — 200/401/403 → ready, others → not ready,
// with a "http <code>" detail.
func TestProbePublicURLClassification(t *testing.T) {
	cases := []struct {
		code  int
		ready bool
	}{
		{200, true}, {401, true}, {403, true},
		{404, false}, {500, false}, {502, false}, {503, false}, {504, false},
		{520, false}, {526, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("http_%d", tc.code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()

			ready, detail, err := probePublicURL(context.Background(), srv.Client(), srv.URL+"/")
			if err != nil {
				t.Fatalf("unexpected transport error for %d: %v", tc.code, err)
			}
			if ready != tc.ready {
				t.Errorf("status %d: ready=%v, want %v", tc.code, ready, tc.ready)
			}
			if want := fmt.Sprintf("http %d", tc.code); detail != want {
				t.Errorf("detail = %q, want %q", detail, want)
			}
		})
	}
}

// TestProbePublicURLUnreachable: a GET against a closed port is a transport error
// → not ready (mirrors DNS-failure/connection-refused during propagation).
func TestProbePublicURLUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := srv.URL // capture, then close so the port refuses connections
	srv.Close()

	client := &http.Client{Timeout: time.Second}
	ready, detail, err := probePublicURL(context.Background(), client, closedURL+"/")
	if err == nil {
		t.Fatal("expected a transport error against a closed port")
	}
	if ready {
		t.Errorf("a connection-refused host must be NOT ready (detail %q)", detail)
	}
}
