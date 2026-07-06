package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/devtunnel"
)

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
		probeLocal: func(int) error { return nil }, // no-op: local dev server "up"
		keygen:     goodKeygen,
		dialer:     dialer,
		blockID:    "my-block",
		port:       5186,
		endpoint:   "sish.example:2224",
		baseURL:    "https://civitai.com",
		idle:       30 * time.Minute,
		newTimer:   func(time.Duration) devtunnel.Timer { return timer },
		signals:    sigs,
		out:        &out,
		errw:       &errw,
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
	deps.probeLocal = func(port int) error {
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
