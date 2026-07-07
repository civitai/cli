package cmd

import (
	"context"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestTunnelWaitWatchersReleaseSignalsChan is the regression test for the TTY
// teardown bug: the bubbletea readiness wait must NOT leave a goroutine parked
// reading the send-based, single-receiver, never-closed d.signals channel after
// it ends. If it did, the next SIGINT/SIGTERM would be stolen from
// runTunnelSession's serving loop (~50% of Ctrl-Cs dropped; a lone SIGTERM could
// leave the process lingering).
//
// Proof: after stop() (which close(quit)s and joins every watcher), a buffered
// send on d.signals followed by a receive must round-trip. A leaked reader would
// steal the value and the receive would block → timeout.
func TestTunnelWaitWatchersReleaseSignalsChan(t *testing.T) {
	sigs := make(chan os.Signal, 1) // shape mirrors signal.Notify: buffered, send-based, never closed
	d := tunnelSessionDeps{signals: sigs}
	tunnel := newFakeTunnel()

	sent := make(chan tea.Msg, 4)
	stop := startTunnelWaitWatchers(context.Background(), d, tunnel, func(m tea.Msg) { sent <- m })

	// Ready path: nothing aborted; the wait ends and joins the watchers.
	stop()

	// No watcher may still be reading d.signals.
	sigs <- os.Interrupt
	select {
	case got := <-sigs:
		if got != os.Interrupt {
			t.Fatalf("round-tripped the wrong signal: %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a watcher goroutine is STILL reading d.signals after stop() — the signal was stolen (teardown regression)")
	}

	// The clean-stop path must not have emitted any abort.
	select {
	case m := <-sent:
		t.Fatalf("no abort should be sent on a clean stop, got %#v", m)
	default:
	}
}

// TestTunnelWaitWatchersSignalAborts: a signal delivered DURING the wait produces
// the "interrupt" abort (SIGTERM path preserved; Ctrl-C is handled separately as
// a KeyCtrlC message under bubbletea raw mode).
func TestTunnelWaitWatchersSignalAborts(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	d := tunnelSessionDeps{signals: sigs}
	tunnel := newFakeTunnel()

	sent := make(chan tea.Msg, 4)
	stop := startTunnelWaitWatchers(context.Background(), d, tunnel, func(m tea.Msg) { sent <- m })
	defer stop()

	sigs <- os.Interrupt
	assertAbortReason(t, sent, "interrupt")
}

// TestTunnelWaitWatchersCtxAborts: ctx cancellation → "canceled".
func TestTunnelWaitWatchersCtxAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := tunnelSessionDeps{signals: make(chan os.Signal, 1)}
	tunnel := newFakeTunnel()

	sent := make(chan tea.Msg, 4)
	stop := startTunnelWaitWatchers(ctx, d, tunnel, func(m tea.Msg) { sent <- m })
	defer stop()

	cancel()
	assertAbortReason(t, sent, "canceled")
}

// TestTunnelWaitWatchersTunnelAborts: the tunnel dropping → "tunnel closed".
func TestTunnelWaitWatchersTunnelAborts(t *testing.T) {
	d := tunnelSessionDeps{signals: make(chan os.Signal, 1)}
	tunnel := newFakeTunnel()

	sent := make(chan tea.Msg, 4)
	stop := startTunnelWaitWatchers(context.Background(), d, tunnel, func(m tea.Msg) { sent <- m })
	defer stop()

	close(tunnel.done)
	assertAbortReason(t, sent, "tunnel closed")
}

// TestTunnelWaitWatchersNilSignals: a nil signals channel is tolerated (no
// signals watcher) and ctx still aborts.
func TestTunnelWaitWatchersNilSignals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := tunnelSessionDeps{signals: nil}
	tunnel := newFakeTunnel()

	sent := make(chan tea.Msg, 4)
	stop := startTunnelWaitWatchers(ctx, d, tunnel, func(m tea.Msg) { sent <- m })
	defer stop()

	cancel()
	assertAbortReason(t, sent, "canceled")
}

func assertAbortReason(t *testing.T, sent <-chan tea.Msg, want string) {
	t.Helper()
	select {
	case m := <-sent:
		ab, ok := m.(tunnelAbortMsg)
		if !ok {
			t.Fatalf("expected a tunnelAbortMsg, got %#v", m)
		}
		if ab.reason != want {
			t.Fatalf("abort reason = %q, want %q", ab.reason, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no abort msg produced (want reason %q)", want)
	}
}
