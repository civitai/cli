package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// THE MULTI-SHELL PORTABILITY TABLE FOR THE EMITTED PATH BLOCK.
//
// 🔴 WHY IT IS COMMITTED, AND WHY THAT WAS THE WHOLE FINDING. Round 1 of cli#777
// ran this matrix from an UNCOMMITTED harness and decision 39 quoted its total
// ("109 assertions, 0 failures"). A number from a harness nobody can re-run is
// not re-derivable: it cannot be checked after the next edit to the block, and
// the figure did not even follow from its own stated dimensions (5 shells × 5
// directory shapes × 4 properties is 100, and nothing accounted for the other 9).
// Committing the harness replaces the quoted total with a number any run prints.
//
// 🔴 IT DRIVES THE BLOCK THE REAL BINARY WROTE, NOT A GO STRING LITERAL. For each
// directory shape it copies the built CLI into a directory of that shape, runs
// `agent-setup --fix-path` from there against a throwaway HOME, and extracts the
// managed block back out of the `~/.zshenv` it wrote. A block built by calling
// pathFixBlock in-process would not be evidence about what ships.
//
// 🔴 A SHELL THAT IS NOT INSTALLED SKIPS, VISIBLY, AND NEVER PASSES. Round 1's
// first harness could not find the shells at all under `env -i` and scored the
// uninstall arm PASS for every shell it never ran — a reassuring zero from a
// probe wired to nothing. Here an absent shell produces a `t.Skip` on its own
// subtest and is counted in the SKIPPED line of the summary, so a run that
// measured two shells cannot be read as a run that measured five.
//
// 🔴 AND THE HARNESS VALIDATES ITSELF ON AN UNGUARDED CONTROL BLOCK BEFORE
// TRUSTING ANY VERDICT. The control is a plain unconditional prepend — no `case`,
// no `[ -x … ]` — which MUST score "1 entry after one source / 2 after two / 1
// with the binary moved away" in every shell that runs. If the control does not
// behave that way the harness is measuring nothing, and that is a FAILURE, not a
// skip: it is the negative control for the three assertions that follow it.
//
// To measure the shells this host does not ship:
//
//	nix-shell -p dash mksh busybox --run 'go test . -run MultiShell -v'

// multiShellRunner is one shell, as an argv prefix, because `busybox ash` is two
// words and a `shell string` field cannot hold it.
type multiShellRunner struct {
	name string
	argv []string // e.g. {"bash", "-c"} or {"busybox", "ash", "-c"}
}

var multiShellRunners = []multiShellRunner{
	{"bash", []string{"bash", "-c"}},
	{"dash", []string{"dash", "-c"}},
	{"zsh", []string{"zsh", "-c"}},
	{"mksh", []string{"mksh", "-c"}},
	{"busybox ash", []string{"busybox", "ash", "-c"}},
}

// multiShellDirShapes are the directory names the block has to survive quoting
// for. Each becomes the LAST path segment, so the metacharacter is in the PATH
// entry itself rather than only in a parent.
var multiShellDirShapes = []string{
	"plain",
	"with space",
	"with'quote",
	"glob*[a-z]",
	"dollar$HOME",
}

// resolve returns the absolute program for this runner, or "" when it is absent.
func (r multiShellRunner) resolve() (string, []string, bool) {
	p, err := exec.LookPath(r.argv[0])
	if err != nil {
		return "", nil, false
	}
	return p, r.argv[1:], true
}

// run executes script under this runner and returns stdout, stderr and the error.
//
// The environment is minimal and deterministic: a PATH that holds the shell's own
// directory plus the usual system ones, and nothing that could already carry a
// `civitai`. HOME points at a throwaway directory so no real dotfile is read.
//
// 🔴 AN EXTERNAL `printf` IS ON THAT PATH DELIBERATELY, AND THE HARNESS CONTROL IS
// WHAT FOUND OUT WHY. `printf %s "$PATH"` is how every probe below reads its
// answer, and it is a BUILTIN in bash, dash, zsh and busybox ash — but NOT in
// mksh, which execs `/usr/bin/printf`. On a distribution whose `/usr/bin` and
// `/bin` are near-empty (NixOS: `/bin/sh` and `/usr/bin/env` and little else) a
// PATH built only from those two directories leaves mksh with no `printf` at all,
// and the shell exits 127 with `printf: inaccessible or not found`. That is a
// defect in the HARNESS, not in the block — and it presented as the control
// block failing, which is the one thing that cannot be mistaken for a finding.
// Resolving `printf` from the ambient PATH and adding its directory is what lets
// mksh be measured instead of silently skipped.
func (r multiShellRunner) run(t *testing.T, prog string, rest []string, home, script string) (string, string, error) {
	t.Helper()
	pathEntries := []string{filepath.Dir(prog)}
	if pf, err := exec.LookPath("printf"); err == nil {
		pathEntries = append(pathEntries, filepath.Dir(pf))
	}
	for _, d := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			pathEntries = append(pathEntries, d)
		}
	}
	args := append(append([]string{}, rest...), script)
	cmd := exec.Command(prog, args...)
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + strings.Join(pathEntries, string(filepath.ListSeparator)),
		"TERM=dumb",
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// countEntries counts EXACT occurrences of dir as a PATH entry.
//
// 🔴 NOT strings.Count ON A COLON-WRAPPED PATH. That expression is
// non-overlapping, so two ADJACENT duplicates — exactly what a broken guard
// produces — share the colon between them and count as ONE. That defect let a
// mutant survive a whole sweep in round 1 (decision 39, M1b).
func countEntries(pathVar, dir string) int {
	n := 0
	for _, e := range filepath.SplitList(pathVar) {
		if e == dir {
			n++
		}
	}
	return n
}

// buildCLIForMultiShell builds ./cmd/civitai once and returns the binary path.
func buildCLIForMultiShell(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "civitai-built")
	out, err := exec.Command("go", "build", "-o", bin, "./cmd/civitai").CombinedOutput()
	if err != nil {
		t.Fatalf("building ./cmd/civitai: %v\n%s", err, out)
	}
	return bin
}

// blockFromRealBinary copies built into a directory of the given shape, runs
// `agent-setup --fix-path` from that copy, and returns the managed block the run
// wrote plus the directory the block names.
func blockFromRealBinary(t *testing.T, built, shape string) (block, dir string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), shape)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("NOT A PASS: this filesystem will not hold a directory named %q, so the %q shape "+
			"was not measured: %v", shape, shape, err)
	}
	raw, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "civitai")
	if werr := os.WriteFile(exe, raw, 0o755); werr != nil {
		t.Fatal(werr)
	}

	home := t.TempDir()
	project := t.TempDir()
	cmd := exec.Command(exe, "agent-setup", "--dir", project, "--agent", "claude", "--fix-path")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"CIVITAI_TOKEN=",
		"CIVITAI_NO_UPDATE_CHECK=1",
	)
	if out, rerr := cmd.CombinedOutput(); rerr != nil {
		t.Fatalf("`civitai agent-setup --fix-path` failed for the %q shape: %v\n%s", shape, rerr, out)
	}
	b, err := os.ReadFile(filepath.Join(home, ".zshenv"))
	if err != nil {
		t.Fatalf("--fix-path wrote no ~/.zshenv for the %q shape, so there is no block to drive: %v",
			shape, err)
	}
	const begin = "# BEGIN civitai cli PATH"
	const end = "# END civitai cli PATH"
	i := strings.Index(string(b), begin)
	j := strings.Index(string(b), end)
	if i < 0 || j < i {
		t.Fatalf("the ~/.zshenv written for the %q shape holds no complete managed block:\n%s", shape, b)
	}
	block = string(b)[i:j+len(end)] + "\n"
	// 🔴 COMPARE THE SHELL-QUOTED FORM, NOT THE RAW PATH. The block renders the
	// directory as a POSIX single-quoted word, so for the `with'quote` shape the
	// raw string does NOT appear literally — a `'` is closed, backslash-escaped
	// and reopened. A `strings.Contains(block, dir)` check therefore fails on
	// exactly the shape whose quoting matters most; that is this harness's own
	// first finding, against itself.
	if !strings.Contains(block, shQuote(dir)) {
		t.Fatalf("the block written for the %q shape does not assign %s — the harness would be "+
			"driving a block for some other directory:\n%s", shape, shQuote(dir), block)
	}
	return block, dir
}

// unguardedControlBlock is the block as it shipped BEFORE round 1: a plain
// unconditional prepend, with no `case` dedupe and no `[ -x … ]` existence test.
//
// It is the negative control for all three behavioural properties at once. In any
// shell that runs it, it must put the directory on PATH once after one source,
// TWICE after two, and still once when the binary has been moved away. A shell in
// which it does not do that is a shell this harness cannot measure.
func unguardedControlBlock(dir string) string {
	q := "'" + strings.ReplaceAll(dir, "'", `'\''`) + "'"
	return "civitai_cli_dir=" + q + "\n" +
		"PATH=\"$civitai_cli_dir:$PATH\" ; export PATH\n" +
		"unset civitai_cli_dir\n"
}

func writeScriptFile(t *testing.T, body string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "block.sh")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// shQuote renders s as a POSIX single-quoted word, for interpolating a path into
// a script this harness builds.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// TestTheEmittedBlockIsPortableAcrossPOSIXShells is the committed replacement for
// round 1's uncommitted multi-shell sweep.
//
// It prints its own totals, so decision 39 can point at a command instead of
// quoting a number: MEASURED (assertions that ran), SKIPPED (shells absent) and
// the per-shell/per-shape verdicts under `-v`.
func TestTheEmittedBlockIsPortableAcrossPOSIXShells(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted block is POSIX `sh`; a native-Windows run of it was never measured")
	}

	built := buildCLIForMultiShell(t)

	type liveShell struct {
		runner multiShellRunner
		prog   string
		rest   []string
	}
	var live []liveShell
	var absent []string
	for _, r := range multiShellRunners {
		prog, rest, ok := r.resolve()
		if !ok {
			absent = append(absent, r.name)
			continue
		}
		live = append(live, liveShell{r, prog, rest})
	}

	// 🔴 AN ABSENT SHELL GETS ITS OWN SKIPPING SUBTEST, NOT JUST A LOG LINE. A
	// `--- SKIP: …/mksh` row in the output is what makes "this shell was not
	// measured" as visible as a pass or a failure; a summary line at the bottom is
	// read far less often, and an absent row is read not at all. The round-1
	// harness's defect was precisely that a shell it never ran was indistinguishable
	// from a shell that passed.
	for _, name := range absent {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Skipf("NOT A PASS — NOTHING WAS MEASURED for %s: it is not on PATH, so none of the four "+
				"properties were checked in it. Run `nix-shell -p dash mksh busybox coreutils --run "+
				"'go test . -run TheEmittedBlockIsPortable -v'` to cover it.", name)
		})
	}
	if len(live) == 0 {
		t.Skipf("NOT A PASS — NOTHING WAS MEASURED: none of %s is installed, so the block's "+
			"portability was not checked here.", strings.Join(absent, ", "))
	}

	// Pre-build one block per directory shape, from the real binary.
	type shapeCase struct {
		shape string
		block string
		dir   string
	}
	var shapes []shapeCase
	for _, shape := range multiShellDirShapes {
		block, dir := blockFromRealBinary(t, built, shape)
		shapes = append(shapes, shapeCase{shape, block, dir})
	}
	if len(shapes) == 0 {
		t.Fatal("no directory shape could be built, so nothing was measured")
	}

	assertions := 0
	skippedShapes := 0

	for _, ls := range live {
		ls := ls
		t.Run(ls.runner.name, func(t *testing.T) {
			home := t.TempDir()

			// ---- CONTROL FIRST. A failure here is a broken harness, not a finding.
			ctlDir := filepath.Join(t.TempDir(), "control")
			if err := os.MkdirAll(ctlDir, 0o755); err != nil {
				t.Fatal(err)
			}
			ctlExe := filepath.Join(ctlDir, "civitai")
			if err := os.WriteFile(ctlExe, []byte("#!/bin/sh\necho control\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			ctl := writeScriptFile(t, unguardedControlBlock(ctlDir))
			one := ". " + shQuote(ctl) + "; printf %s \"$PATH\""
			two := ". " + shQuote(ctl) + "; . " + shQuote(ctl) + "; printf %s \"$PATH\""

			out, errOut, err := ls.runner.run(t, ls.prog, ls.rest, home, one)
			if err != nil {
				t.Fatalf("HARNESS BROKEN, not a finding: %s could not source the unguarded control "+
					"block: %v\nstderr: %s", ls.runner.name, err, errOut)
			}
			if n := countEntries(out, ctlDir); n != 1 {
				t.Fatalf("HARNESS BROKEN, not a finding: the unguarded control block put the "+
					"directory on %s's PATH %d time(s), want 1. This harness cannot measure the real "+
					"block in this shell.\nPATH=%s", ls.runner.name, n, out)
			}
			out, errOut, err = ls.runner.run(t, ls.prog, ls.rest, home, two)
			if err != nil {
				t.Fatalf("HARNESS BROKEN: %s failed sourcing the control twice: %v\nstderr: %s",
					ls.runner.name, err, errOut)
			}
			if n := countEntries(out, ctlDir); n != 2 {
				t.Fatalf("HARNESS BROKEN, not a finding: the unguarded control block put the "+
					"directory on %s's PATH %d time(s) after TWO sources, want 2. A control that does "+
					"not duplicate cannot show that the real block's `case` arm is what prevents a "+
					"duplicate — every dedupe assertion below would pass for the wrong reason."+
					"\nPATH=%s", ls.runner.name, n, out)
			}
			if err := os.Remove(ctlExe); err != nil {
				t.Fatal(err)
			}
			out, _, err = ls.runner.run(t, ls.prog, ls.rest, home, one)
			if err != nil {
				t.Fatalf("HARNESS BROKEN: %s failed sourcing the control after the binary was "+
					"removed: %v", ls.runner.name, err)
			}
			if n := countEntries(out, ctlDir); n != 1 {
				t.Fatalf("HARNESS BROKEN, not a finding: with the binary moved away the unguarded "+
					"control block put the directory on %s's PATH %d time(s), want 1. A control that "+
					"ALSO adds nothing cannot show that the real block's `[ -x … ]` test is what "+
					"removes the entry — the uninstall assertion below would pass for the wrong "+
					"reason, which is exactly how round 1's first harness scored PASS for shells it "+
					"never ran.\nPATH=%s", ls.runner.name, n, out)
			}
			t.Logf("control validated in %s (1 / 2 / 1 on the three arms)", ls.runner.name)

			// ---- Now the real block, four properties per directory shape.
			for _, sc := range shapes {
				sc := sc
				t.Run(sc.shape, func(t *testing.T) {
					f := writeScriptFile(t, sc.block)
					src1 := ". " + shQuote(f) + "; printf %s \"$PATH\""
					src2 := ". " + shQuote(f) + "; . " + shQuote(f) + "; printf %s \"$PATH\""

					// P1 — one source puts the directory on PATH exactly once.
					out, errOut, err := ls.runner.run(t, ls.prog, ls.rest, home, src1)
					if err != nil {
						t.Fatalf("%s could not source the block for %q: %v\nstderr: %s",
							ls.runner.name, sc.dir, err, errOut)
					}
					if n := countEntries(out, sc.dir); n != 1 {
						t.Errorf("P1: %s put %q on PATH %d time(s) after ONE source, want 1.\nPATH=%s",
							ls.runner.name, sc.dir, n, out)
					}
					assertions++

					// P2 — two sources still leave exactly one (the `case` arm).
					out, errOut, err = ls.runner.run(t, ls.prog, ls.rest, home, src2)
					if err != nil {
						t.Fatalf("%s could not source the block twice for %q: %v\nstderr: %s",
							ls.runner.name, sc.dir, err, errOut)
					}
					if n := countEntries(out, sc.dir); n != 1 {
						t.Errorf("P2: %s put %q on PATH %d time(s) after TWO sources, want 1. The "+
							"`case` arm is what stops a profile growing an entry per shell start."+
							"\nPATH=%s", ls.runner.name, sc.dir, n, out)
					}
					assertions++

					// P3 — clean under `set -eu`, with EMPTY stderr. A startup file
					// that writes to stderr on every shell start is a defect in a file
					// this command wrote.
					// `out` is deliberately discarded here: P3 is about the EXIT
					// STATUS and an EMPTY stderr, not about the resulting PATH, which
					// P1 above already asserted on the same script.
					_, errOut, err = ls.runner.run(t, ls.prog, ls.rest, home,
						"set -eu; "+src1)
					if err != nil {
						t.Errorf("P3: %s failed sourcing the block for %q under `set -eu`: %v"+
							"\nstderr: %s", ls.runner.name, sc.dir, err, errOut)
					} else if strings.TrimSpace(errOut) != "" {
						t.Errorf("P3: %s wrote to stderr sourcing the block for %q under `set -eu`: "+
							"%q. Every shell start would print that, forever, out of a file this "+
							"command wrote.", ls.runner.name, sc.dir, errOut)
					}
					assertions++

					// P4 — with the binary moved away the block adds NOTHING (the
					// `[ -x … ]` test). This is the uninstall case, and the arm round
					// 1's first harness scored PASS without running.
					gone := filepath.Join(sc.dir, "civitai")
					saved, rerr := os.ReadFile(gone)
					if rerr != nil {
						t.Skipf("NOT A PASS: %q no longer holds a `civitai` to move away, so the "+
							"uninstall property was not measured: %v", sc.dir, rerr)
					}
					if err := os.Remove(gone); err != nil {
						t.Fatal(err)
					}
					out, _, err = ls.runner.run(t, ls.prog, ls.rest, home, src1)
					if werr := os.WriteFile(gone, saved, 0o755); werr != nil {
						t.Fatal(werr)
					}
					if err != nil {
						t.Fatalf("%s could not source the block for %q with the binary removed: %v",
							ls.runner.name, sc.dir, err)
					}
					if n := countEntries(out, sc.dir); n != 0 {
						t.Errorf("P4: %s put %q on PATH %d time(s) while that directory holds no "+
							"`civitai`, want 0. The `[ -x … ]` test is what makes the pin "+
							"self-expiring on an uninstall or a moved prefix.\nPATH=%s",
							ls.runner.name, sc.dir, n, out)
					}
					assertions++
				})
			}
		})
	}

	t.Logf("MEASURED %d assertions: %d shell(s) x %d directory shape(s) x 4 properties",
		assertions, len(live), len(shapes))
	var names []string
	for _, ls := range live {
		names = append(names, ls.runner.name+" ("+ls.prog+")")
	}
	t.Logf("shells measured: %s", strings.Join(names, ", "))
	if len(absent) > 0 {
		t.Logf("SKIPPED — NOT MEASURED HERE (shell not installed): %s. Run "+
			"`nix-shell -p dash mksh busybox --run 'go test . -run MultiShell -v'` to cover them.",
			strings.Join(absent, ", "))
	}
	if skippedShapes > 0 {
		t.Logf("SKIPPED %d directory shape(s) this filesystem would not hold", skippedShapes)
	}
}
