package main

// THE PUBLISHED EXIT CODE FOR AN UNREADABLE `--input` FILE.
//
// internal/cmd's TestReadGraphInputClassification pins the CLASSIFICATION
// (ErrUsage or not). That is the internal contract; this file pins the thing
// the README actually promises a script — the PROCESS EXIT STATUS — by building
// the real binary and reading what it returns.
//
// The two are not the same claim and neither subsumes the other: a
// classification can be correct while exitCode routes it somewhere else (that
// is exactly what issue #241 was, from the other side — every untagged
// filesystem error WAS correctly untagged and still exited 5), and an exit
// status can be right for a reason that has nothing to do with the change.
//
// Measured on the binary at 592a8a9, before this change:
//
//	generate --input <mode-000 file>   rc=2   ← WRONG; exitCodeDocs says 1
//	generate --input <missing file>    rc=2   ← correct
//	generate --image <mode-000 png>    rc=1   ← the documented rule
//
// The harness is the one fs_not_network_test.go already uses — buildCLI +
// runCLI — deliberately, rather than a second in-process seam: a subprocess is
// the only thing that measures the actual process exit status, which is what
// the contract is about. No network is reachable from any row (readGraphInput
// runs before the graph is built and before any request), and CIVITAI_BASE_URL
// points at a refused loopback port so a row that somehow got past the read
// fails as exit 5 rather than dialling civitai.com.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateInputExitCodes is the end-to-end half of the fix.
//
// RED AT BASE (3d95b05) for the `unreadable` row: rc=2.
// GREEN AT HEAD: rc=1. The `missing` and `a directory` rows are CONTROLS —
// green at base and at HEAD — and they are what makes "delete the asUsageError
// tagging entirely" a FAILING fix rather than a passing one.
func TestGenerateInputExitCodes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a mode-000 file is still readable, so the headline row is not observable here")
	}
	bin := buildCLI(t)

	dir := t.TempDir()
	const graph = `{"workflow":"txt2img","prompt":"a cat"}`

	unreadable := filepath.Join(dir, "unreadable.json")
	if err := os.WriteFile(unreadable, []byte(graph), 0o000); err != nil {
		t.Fatal(err)
	}
	readable := filepath.Join(dir, "readable.json")
	if err := os.WriteFile(readable, []byte(graph), 0o600); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "adir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Fixture premise: the OS really is refusing the read. A filesystem that
	// ignores mode 000 would make the headline row measure nothing.
	if _, err := os.ReadFile(unreadable); err == nil {
		t.Skip("this filesystem ignores mode 000 — the unreadable case is not observable here")
	}

	deadURL := "http://" + closedLoopbackAddr(t)
	env := []string{
		"HOME=" + dir,
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "config"),
		"CIVITAI_TOKEN=abc123",
		"CIVITAI_BASE_URL=" + deadURL,
		"NO_COLOR=1",
	}

	for _, tc := range []struct {
		name    string
		path    string
		want    int
		control bool
		// reason is a fragment of the OS's own message that must survive
		// classification — the fix moves an exit code, never the text.
		reason string
		why    string
	}{
		{
			name:   "unreadable but present",
			path:   unreadable,
			want:   exitGeneric,
			reason: "permission denied",
			why: "the file exists and the invocation named it correctly — a permissions failure is a " +
				"filesystem failure, which exitCodeDocs publishes under code 2 as exiting 1, not 2",
		},
		{
			name:    "missing",
			path:    filepath.Join(dir, "definitely-absent.json"),
			want:    exitUsage,
			control: true,
			reason:  "no such file",
			why:     "a path that is not there is a mistake about the invocation",
		},
		{
			name:    "a directory",
			path:    subdir,
			want:    exitUsage,
			control: true,
			reason:  "is a directory",
			why:     "a directory is already published as an exit-2 refusal for the image flags; --input must agree",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc, stderr := runCLI(t, bin, env, "generate", "--input", tc.path, "--dry-run", "--yes")

			label := "REGRESSION ROW"
			if tc.control {
				label = "CONTROL ROW"
			}
			if rc != tc.want {
				t.Errorf("%s: `generate --input <%s>` exited %d, want %d.\nWhy: %s\nstderr: %s",
					label, tc.name, rc, tc.want, tc.why, stderr)
			}
			if !strings.Contains(stderr, tc.reason) {
				t.Errorf("%s: the message no longer names %q — an exit-code change must not alter what the user reads.\nstderr: %s",
					label, tc.reason, stderr)
			}
		})
	}

	// 🔴 POSITIVE CONTROLS. Without these, a binary that failed every
	// `--input` invocation the same way would pass every row above.
	t.Run("control: a readable graph gets PAST the read", func(t *testing.T) {
		rc, stderr := runCLI(t, bin, env, "generate", "--input", readable, "--dry-run", "--yes")
		if rc != exitNetwork {
			t.Errorf("a readable --input reached exit %d, want %d (the refused dial to %s).\n"+
				"If this is not 5 the run never got past readGraphInput, and the rows above are\n"+
				"measuring a shared early failure rather than the classification.\nstderr: %s",
				rc, exitNetwork, deadURL, stderr)
		}
	})

	t.Run("control: --image with the same unreadable-file shape also exits 1", func(t *testing.T) {
		png := filepath.Join(dir, "i.png")
		if err := os.WriteFile(png, []byte("\x89PNG\r\n\x1a\n"), 0o000); err != nil {
			t.Fatal(err)
		}
		rc, stderr := runCLI(t, bin, env,
			"generate", "a cat", "--ecosystem", "Qwen", "--image", png, "--dry-run", "--yes")
		if rc != exitGeneric {
			t.Errorf("`generate --image <unreadable>` exited %d, want %d — the two local path flags\n"+
				"must answer an unreadable file the same way, or the fix merely moved which one\n"+
				"disagrees.\nstderr: %s", rc, exitGeneric, stderr)
		}
	})
}
