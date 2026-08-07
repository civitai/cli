package cmd

// AN UNREADABLE `--input` FILE IS A FILESYSTEM FAILURE, NOT A USAGE ERROR.
//
// `readGraphInput` used to wrap EVERY os.ReadFile failure in asUsageError, so
// `civitai generate --input <mode-000 file>` exited 2 — while the published
// contract in exitcodes_doc.go says, under code 2 and in as many words, that
// "a file that exists and cannot be read (permissions, an I/O error) is a
// filesystem failure rather than a mistake about the invocation, and exits 1,
// not 2", and while `--image <mode-000 png>` already exited 1. Measured on the
// binary at 592a8a9: `--input <unreadable>` rc=2, `--image <unreadable>` rc=1.
//
// The tell that it was an oversight rather than a decision is inside the same
// function: the `--input -` stdin branch returns its read failure UNTAGGED and
// always exited 1, so one command answered the same question two ways
// depending only on whether the graph arrived by file or by pipe.
//
// 🔴 CLASSIFICATION IS ASSERTED WITH errors.Is(err, ErrUsage), NEVER WITH
// MESSAGE TEXT (AGENTS.md item 7). The sentinels carry no visible text, so a
// test that reads the message says nothing at all about the exit code — that
// exact mistake left the entire suite green while the 403 → exit 3 promise was
// unpinned.
//
// Two batteries, and neither subsumes the other:
//
//   - TestReadGraphInputClassification is the table over the real error
//     SHAPES. Each row asserts its own premise (the fixture really produces
//     the errno the row is about) before asserting the classification, so a
//     fixture that stopped exercising the case cannot pass quietly.
//   - The ErrUsage rows are the CONTROL that stops "just delete the tagging"
//     from being a passing fix: untag everything and `missing` / `a directory`
//     redden.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// inputFixture is one `--input` path plus the errno reading it must produce and
// the classification the published contract requires.
type inputFixture struct {
	name string
	path string
	// errno is what a direct os.ReadFile of path must fail with. It is asserted
	// per row so a fixture the OS stopped rejecting (a filesystem that ignores
	// mode 000, say) fails loudly instead of proving nothing.
	errno syscall.Errno
	// wantUsage is the published contract: true => exit 2, false => exit 1.
	wantUsage bool
	// why records the REASON the row expects that code, derived from what the
	// failure MEANS rather than copied from the implementation.
	why string
}

// TestReadGraphInputClassification pins which `--input` read failures are a
// mistake about the invocation (exit 2) and which are filesystem failures
// (exit 1).
//
// RED AT BASE (3d95b05) for the `unreadable` row: it was ErrUsage-tagged.
// The `missing` and `a directory` rows are GREEN AT BASE by construction —
// they are the controls that make "untag everything" a failing fix rather than
// a passing one, so they are invariant guards, not regression coverage.
func TestReadGraphInputClassification(t *testing.T) {
	dir := t.TempDir()

	const graph = `{"workflow":"txt2img","prompt":"a cat"}`

	readable := filepath.Join(dir, "readable.json")
	if err := os.WriteFile(readable, []byte(graph), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(dir, "unreadable.json")
	if err := os.WriteFile(unreadable, []byte(graph), 0o000); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}

	// POSITIVE CONTROL. The same bytes at 0600 must be read and parsed, so a
	// failure below is about the path's state and not about the fixture being a
	// bad graph — and so a green table cannot mean "readGraphInput rejects
	// everything".
	t.Run("CONTROL readable file is read and parses", func(t *testing.T) {
		b, err := readGraphInput(nil, readable)
		if err != nil {
			t.Fatalf("a readable graph file must be read, got: %v", err)
		}
		if string(b) != graph {
			t.Fatalf("readGraphInput returned %q, want %q", b, graph)
		}
		if _, err := parseGraphInput(b); err != nil {
			t.Fatalf("the fixture must be a valid graph, got: %v", err)
		}
	})

	fixtures := []inputFixture{
		{
			name:      "missing (ENOENT)",
			path:      filepath.Join(dir, "definitely-absent.json"),
			errno:     syscall.ENOENT,
			wantUsage: true,
			why: "a path that is not there is a mistake about the INVOCATION — the user named a file that " +
				"does not exist, and no amount of filesystem cooperation would have helped",
		},
		{
			// 🔴 DELIBERATE, and it is the one row that is a judgement call.
			// exit 2. A directory is not "a file that exists and cannot be
			// read" — it is not a file at all, so the code-1 rule does not
			// reach it, and the CLI already publishes "a directory" as an
			// exit-2 refusal for the image flags (imageUsageRefusals). Sending
			// it to 1 would make `generate --input <dir>` and
			// `generate --image <dir>` answer the same mistake two different
			// ways, which is precisely the inconsistency this change exists to
			// remove — reintroduced one flag over. It also preserves the
			// behaviour measured at base (rc=2), so nothing that scripts
			// against a directory typo moves.
			name:      "a directory (EISDIR on read)",
			path:      subdir,
			errno:     syscall.EISDIR,
			wantUsage: true,
			why: "a directory is not a file the read could ever have succeeded on, so the mistake is the " +
				"invocation — and `--image <dir>` is already published as exit 2",
		},
		{
			name:      "unreadable but present (EACCES)",
			path:      unreadable,
			errno:     syscall.EACCES,
			wantUsage: false,
			why: "the file IS there and the invocation named it correctly; the read failed for a reason " +
				"outside the invocation, which the published contract puts on exit 1",
		},
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			if f.errno == syscall.EACCES && os.Geteuid() == 0 {
				t.Skip("running as root: a mode-000 file is still readable, so the unreadable case is not observable here")
			}

			// The row asserts its own premise first. A fixture that no longer
			// produces the errno it is named for cannot say anything about the
			// classification below.
			_, probeErr := os.ReadFile(f.path)
			if probeErr == nil {
				t.Fatalf("os.ReadFile(%s) succeeded — this row no longer exercises %v", f.path, f.errno)
			}
			if !errors.Is(probeErr, f.errno) {
				t.Fatalf("os.ReadFile(%s) failed with %v, want %v — the row is testing something else", f.path, probeErr, f.errno)
			}

			_, err := readGraphInput(nil, f.path)
			if err == nil {
				t.Fatalf("readGraphInput accepted %s", f.path)
			}

			got := errors.Is(err, ErrUsage)
			if got != f.wantUsage {
				want, gotName := "exit 1 (not ErrUsage)", "ErrUsage (exit 2)"
				if f.wantUsage {
					want, gotName = "exit 2 (ErrUsage)", "untagged (exit 1)"
				}
				t.Errorf("readGraphInput(%s) classified as %s, want %s.\nWhy: %s\nerror: %v",
					f.name, gotName, want, f.why, err)
			}

			// The error must stay actionable whichever way it went — an
			// exit-code fix that turned a helpful message into a bare errno
			// would be a regression the classification assertion cannot see.
			if strings.TrimSpace(err.Error()) == "" {
				t.Errorf("readGraphInput(%s) returned an empty message", f.name)
			}
		})
	}
}

// TestReadGraphInputPreservesTheUnderlyingErrno pins that the exit-1 path wraps
// with %w rather than re-phrasing, so cmd/civitai's classifier (and anything
// else reading the cause) still sees the real filesystem error.
//
// Without this, "untagged" could be satisfied by an error that threw the cause
// away — which would silently break the exit-5-vs-exit-1 walk in
// pkg/civitai's transportError, since it has nothing left to walk.
func TestReadGraphInputPreservesTheUnderlyingErrno(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a mode-000 file is still readable, so the unreadable case is not observable here")
	}
	p := filepath.Join(t.TempDir(), "unreadable.json")
	if err := os.WriteFile(p, []byte(`{"workflow":"txt2img"}`), 0o000); err != nil {
		t.Fatal(err)
	}
	_, err := readGraphInput(nil, p)
	if err == nil {
		t.Skip("this filesystem ignores mode 000 — the unreadable case is not observable here")
	}
	if !errors.Is(err, syscall.EACCES) {
		t.Errorf("the exit-1 path must wrap the OS error with %%w so the cause survives; got %v (%T)", err, err)
	}
}

// failingReader is a stdin that fails mid-read.
type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

// TestReadGraphInputStdinFailureIsNotAUsageError is a CONTROL, not regression
// coverage — the stdin branch was already untagged at base and passes there.
//
// Its job is the opposite direction: it pins the sibling that made the file
// branch's tagging visibly wrong, so a future "make --input classification
// consistent" that harmonises the two by tagging BOTH fails here instead of
// quietly re-publishing exit 2 for a read failure.
func TestReadGraphInputStdinFailureIsNotAUsageError(t *testing.T) {
	_, err := readGraphInput(failingReader{err: fmt.Errorf("pipe: %w", syscall.EIO)}, "-")
	if err == nil {
		t.Fatal("a failing stdin must produce an error")
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("a stdin read failure is a filesystem/IO failure (exit 1), not a usage error: %v", err)
	}
	if !errors.Is(err, syscall.EIO) {
		t.Errorf("the stdin branch must wrap its cause with %%w; got %v", err)
	}

	// CONTROL for the other stdin branch: `--input -` with no stdin attached IS
	// a usage error, and must stay one. Without this row, "the stdin branch
	// never tags" would also be satisfied by deleting that guard.
	if _, err := readGraphInput(nil, "-"); !errors.Is(err, ErrUsage) {
		t.Errorf("`--input -` with no stdin attached is a usage error (exit 2), got: %v", err)
	}

	// And the happy stdin path still reads.
	b, err := readGraphInput(strings.NewReader(`{"workflow":"txt2img"}`), "-")
	if err != nil {
		t.Fatalf("a working stdin must be read: %v", err)
	}
	if string(b) != `{"workflow":"txt2img"}` {
		t.Errorf("stdin graph came back as %q", b)
	}
}

// Ensure failingReader really is an io.Reader (a nil interface would make every
// row above take the no-stdin branch instead).
var _ io.Reader = failingReader{}
