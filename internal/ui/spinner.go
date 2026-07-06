package ui

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// WithSpinner runs work while showing a spinner + message, for simple
// "spin while doing X" call sites (e.g. an upload with a network wait).
//
// Behavior by destination:
//   - TTY (w is a real terminal): a bubbletea program renders a live spinner
//     next to message while work runs on its own goroutine. The program tears
//     down cleanly the instant work returns (or ctx is canceled). Ctrl-C cancels
//     the context passed to work and returns promptly.
//   - non-TTY (piped / CI / a bytes.Buffer in tests): prints a single plain
//     "message…" line, runs work, and returns. NO bubbletea, NO animation.
//
// It always returns work's error (or the context error if work was interrupted
// and returned nil). work MUST honor its context for Ctrl-C to be prompt.
func WithSpinner(ctx context.Context, w io.Writer, message string, work func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Non-TTY: one quiet line, run inline, done. Keeps piped/CI output greppable
	// and fully deterministic (and test-drivable without a real terminal).
	if !isTerminal(w) {
		fmt.Fprintf(w, "%s…\n", message)
		return work(ctx)
	}

	// TTY: derive a cancelable context so Ctrl-C (or an external cancel) stops
	// work; run work off the render goroutine and feed its result into the model.
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resCh := make(chan error, 1)
	go func() { resCh <- work(cctx) }()

	m := spinnerRunModel{
		msg:    message,
		sp:     Spinner(),
		resCh:  resCh,
		cancel: cancel,
	}
	// tea.WithContext(cctx): an external ctx cancel (or our own Ctrl-C cancel)
	// also stops the program, so it never outlives the work.
	fm, runErr := tea.NewProgram(m, tea.WithOutput(w), tea.WithContext(cctx)).Run()

	if final, ok := fm.(spinnerRunModel); ok {
		if final.err != nil {
			return final.err
		}
		if final.interrupted && ctx.Err() != nil {
			return ctx.Err()
		}
	}
	// If the program itself was killed (e.g. external ctx cancel) before a
	// doneMsg landed, surface the context error rather than tea's internal one.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return runErr
}

// spinnerRunModel is the bubbletea model behind WithSpinner: a spinner + message
// that quits when work reports its result (doneMsg) or the user hits Ctrl-C.
type spinnerRunModel struct {
	msg         string
	sp          spinner.Model
	resCh       chan error
	cancel      context.CancelFunc
	err         error
	interrupted bool
}

// doneMsg carries work's result into the model.
type doneMsg struct{ err error }

// waitForResult blocks on resCh and delivers work's result as a doneMsg. Exactly
// one is ever produced (resCh is written once).
func waitForResult(resCh chan error) tea.Cmd {
	return func() tea.Msg { return doneMsg{err: <-resCh} }
}

func (m spinnerRunModel) Init() tea.Cmd {
	return tea.Batch(m.sp.Tick, waitForResult(m.resCh))
}

func (m spinnerRunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case doneMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			// Cancel work and mark interrupted; do NOT quit yet — wait for work to
			// observe the cancellation and send its doneMsg, so we never leak the
			// goroutine and we surface work's actual (likely context.Canceled) error.
			m.interrupted = true
			m.cancel()
			return m, nil
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m spinnerRunModel) View() string {
	return fmt.Sprintf("%s %s", m.sp.View(), m.msg)
}
