package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/dnsprobe"
)

// fakeDoHResolver is an injectable dnsprobe.Resolver for the probe tests.
type fakeDoHResolver struct {
	addrs []netip.Addr
	err   error
}

func (f fakeDoHResolver) Resolve(context.Context, string) ([]netip.Addr, error) {
	return f.addrs, f.err
}

// withResolver swaps the package devTunnelResolver for the duration of a test.
func withResolver(t *testing.T, r dnsprobe.Resolver) {
	t.Helper()
	prev := devTunnelResolver
	devTunnelResolver = r
	t.Cleanup(func() { devTunnelResolver = prev })
}

// TestProbePublicTunnelDNSPending: when DoH authoritatively reports the tunnel host
// isn't published yet, probePublicTunnel reports the DISTINCT dns-pending signal
// (not-ready + dnsPendingDetail + the ErrNotPublished sentinel), NOT a hard error.
func TestProbePublicTunnelDNSPending(t *testing.T) {
	withResolver(t, fakeDoHResolver{err: fmt.Errorf("x: %w", dnsprobe.ErrNotPublished)})

	ready, detail, err := probePublicTunnel(context.Background(), testTunnelHost)
	if ready {
		t.Fatal("a not-yet-published host must be NOT ready")
	}
	if detail != dnsPendingDetail {
		t.Fatalf("detail = %q, want %q", detail, dnsPendingDetail)
	}
	if !errors.Is(err, dnsprobe.ErrNotPublished) {
		t.Fatalf("err should carry ErrNotPublished, got %v", err)
	}
}

// TestIsDNSPending: the mapping helper accepts either the detail marker or the
// sentinel error, and rejects other not-ready reasons.
func TestIsDNSPending(t *testing.T) {
	if !isDNSPending(dnsPendingDetail, nil) {
		t.Error("dnsPendingDetail should be DNS-pending")
	}
	if !isDNSPending("", fmt.Errorf("wrap: %w", dnsprobe.ErrNotPublished)) {
		t.Error("ErrNotPublished sentinel should be DNS-pending")
	}
	if isDNSPending("http 404", nil) {
		t.Error("a 404 (route not built) is NOT DNS-pending")
	}
	if isDNSPending("unreachable", errors.New("connection refused")) {
		t.Error("a connection error is NOT DNS-pending")
	}
}

// TestWaitForTunnelReachableDNSPendingLine: when the gate probe reports the
// dns-pending state, the non-TTY heartbeat surfaces the DNS-specific line
// ("waiting for DNS to publish for <host>") rather than the generic "still
// waiting" — then, once DoH resolves, the wait completes ready.
func TestWaitForTunnelReachableDNSPendingLine(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var errw syncBuffer
	deps.errw = &errw // syncBuffer → non-TTY path
	deps.readyPollInterval = 2 * time.Millisecond
	deps.readyTimeout = 0 // indefinite
	deps.quietInterval = 3 * time.Millisecond
	deps.dnsPendingGrace = time.Millisecond // show the DNS line almost immediately

	release := make(chan struct{})
	deps.probePublic = func(_ context.Context, _ string) (bool, string, error) {
		select {
		case <-release:
			return true, "http 401", nil
		default:
			// DoH says not published yet.
			return false, dnsPendingDetail, fmt.Errorf("x: %w", dnsprobe.ErrNotPublished)
		}
	}

	resc := make(chan bool, 1)
	go func() {
		ready, _ := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
		resc <- ready
	}()

	deadline := time.After(2 * time.Second)
	for !strings.Contains(errw.String(), "waiting for DNS to publish for "+testTunnelHost) {
		select {
		case <-deadline:
			t.Fatalf("no DNS-pending line appeared:\n%s", errw.String())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	// It must be the DNS-specific line, not the generic one.
	if strings.Contains(errw.String(), "still waiting for "+testTunnelHost) {
		t.Errorf("should show the DNS-specific line, not the generic 'still waiting':\n%s", errw.String())
	}

	close(release)
	select {
	case ready := <-resc:
		if !ready {
			t.Fatal("wait should end ready once DoH resolves")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not finish after the host published")
	}
}

// TestWaitForTunnelReachableDNSPendingIsNotFatal: the dns-pending result (a
// not-ready + ErrNotPublished error) must NOT abort the wait — it keeps polling.
// Distinct from a real abort (Ctrl-C / tunnel close). Here a positive readyTimeout
// caps it non-fatally, proving dns-pending alone never returns an abort reason.
func TestWaitForTunnelReachableDNSPendingIsNotFatal(t *testing.T) {
	deps := baseDeps(t, &fakeTunnelAPI{}, &fakeDialer{}, newFakeTimer(), make(chan os.Signal))
	var errw syncBuffer
	deps.errw = &errw
	deps.readyPollInterval = time.Millisecond
	deps.readyTimeout = 40 * time.Millisecond // positive cap → non-fatal expiry
	deps.dnsPendingGrace = time.Millisecond

	deps.probePublic = func(context.Context, string) (bool, string, error) {
		return false, dnsPendingDetail, fmt.Errorf("x: %w", dnsprobe.ErrNotPublished)
	}

	ready, abort := waitForTunnelReachable(context.Background(), deps, newFakeTunnel(), testTunnelHost, testTunnelURL)
	if ready {
		t.Fatal("never-published host should not be ready")
	}
	if abort != "" {
		t.Fatalf("dns-pending must be non-fatal (no abort), got abort=%q", abort)
	}
}
