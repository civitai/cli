package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// THE REGRESSION TEST FOR cli#665, RUN AGAINST THE REAL BINARY AND REAL SHELLS.
//
// 🔴 THE DEFECT. On a machine where npm's global prefix is not writable, the
// documented install remedy puts the CLI on PATH for the INSTALLING SHELL only.
// `civitai agent-setup` then writes an AGENTS.md whose every row starts with
// `civitai`, and every later agent session gets a NEW shell — so the durable
// artifact the setup exists to produce names a binary the setup left unreachable.
// Measured 6 of 6 failing containers (complete enumeration, cli#663):
// `~/.npm-global/bin/civitai` present, `zsh -lic 'civitai --version'` and
// `bash -lc 'command -v civitai'` both `command not found`.
//
// 🔴 WHY IT IS HERE AND NOT IN internal/cmd. The thing under test is what
// `os.Executable()` reports for a REAL `civitai` process and whether a REAL login
// shell then resolves the name. internal/cmd's version of this has to inject a
// seam over `os.Executable` (a `go test` binary is called `cmd.test`), and a seam
// is the one thing that cannot answer "does the unseamed path work". This file
// builds ./cmd/civitai, runs it, and asks two shells. It follows
// dogfood_submission_cap_seam_test.go, which builds the binary for the same
// reason: only the binary can answer the question.
//
// 🔴 AND IT COMPILES AT origin/main, WHICH IS THE POINT. A regression test that
// only fails to BUILD on pre-change code has not been shown to fail; it has been
// shown not to compile. This one references no new symbol, so at origin/main it
// runs and goes red on the behaviour: `unknown flag: --fix-path`.
//
// 🔴 WHAT IT DOES NOT ESTABLISH. cli#665's closing condition arm 1 is graded by
// scripts/dogfood/grade.sh against a container image, by a blind model trial.
// This test is not that: it is the same OBSERVABLE, on this host, with the real
// binary. A green here is not a closed issue.

// fixPathShellProbe is one of the two commands cli#665's arm 1 names, verbatim.
type fixPathShellProbe struct {
	shell string
	args  []string
}

var fixPathShellProbes = []fixPathShellProbe{
	{"zsh", []string{"-lic", "civitai --version"}},
	{"bash", []string{"-lc", "command -v civitai"}},
}

// runProbe executes one probe with HOME pointed at a hermetic directory and a
// PATH from which every entry already holding a `civitai` has been removed.
//
// 🔴 ZDOTDIR IS REMOVED. With it set, zsh reads its startup files from THERE and
// never looks at $HOME, so every assertion below would be vacuous while the test
// still reported green.
//
// 🔴 THE PATH FILTER IS DETERMINISM, NOT THE GUARD. A maintainer running this has
// the real CLI installed, and a system profile may legitimately re-add its
// directory. So the assertions name the BUILT binary's directory explicitly
// rather than trusting the filter.
func runProbe(t *testing.T, shellBin string, p fixPathShellProbe, home string) (string, error) {
	t.Helper()
	var keep []string
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(entry, "civitai")); err == nil {
			continue
		}
		keep = append(keep, entry)
	}
	env := []string{"HOME=" + home, "PATH=" + strings.Join(keep, string(filepath.ListSeparator))}
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "HOME="), strings.HasPrefix(kv, "PATH="),
			strings.HasPrefix(kv, "ZDOTDIR="):
			continue
		}
		env = append(env, kv)
	}
	cmd := exec.Command(shellBin, p.args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestAfterFixPathANewLoginShellResolvesTheCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the two probes cli#665 names are POSIX login shells")
	}

	// The binary goes in its OWN directory, so the directory `--fix-path` has to
	// resolve and prepend is one nothing else put on PATH.
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "civitai")
	// 🔴 THE VERSION IS STAMPED WITH A UNIQUE MARKER, AND IT IS LOAD-BEARING. The
	// two probes answer in different currencies: `bash -lc 'command -v civitai'`
	// prints the PATH it resolved, while `zsh -lic 'civitai --version'` RUNS the
	// binary and prints a version. Without a marker the zsh arm is satisfied by any
	// `civitai` on the machine — measured: it printed `civitai dev` from a
	// different build and the assertion on the directory could not see it.
	const marker = "0.0.0-fixpath-regression-fixture"
	if out, err := exec.Command("go", "build", "-ldflags", "-X main.version="+marker,
		"-o", bin, "./cmd/civitai").CombinedOutput(); err != nil {
		t.Fatalf("building ./cmd/civitai: %v\n%s", err, out)
	}

	type live struct {
		probe fixPathShellProbe
		bin   string
	}
	var found []live
	var missing []string
	for _, p := range fixPathShellProbes {
		sh, err := exec.LookPath(p.shell)
		if err != nil {
			missing = append(missing, p.shell)
			continue
		}
		found = append(found, live{p, sh})
	}
	if len(found) == 0 {
		t.Skipf("NOT A PASS — NOTHING WAS MEASURED: neither %s is installed, so the observable "+
			"cli#665 names (a NEW login shell resolving `civitai`) could not be checked here.",
			strings.Join(missing, " nor "))
	}

	home := t.TempDir()
	project := t.TempDir()

	// NEGATIVE CONTROL: before the fix, no probe may name the built binary.
	//
	// 🔴 "THE PROBE FAILS" WOULD BE THE WRONG CONTROL, AND IT WAS TRIED FIRST. A
	// maintainer with the CLI installed system-wide gets a probe that SUCCEEDS
	// before the fix (observed: `civitai 0.1.111` from the developer's own
	// install), which reads as "the environment already works" and aborts a test
	// whose subject is unrelated. The claim that matters is that the BUILT binary
	// is what answers afterwards and did not answer before.
	for _, l := range found {
		out, err := runProbe(t, l.bin, l.probe, home)
		if err == nil && (strings.Contains(out, binDir) || strings.Contains(out, marker)) {
			t.Fatalf("CONTROL failure, not a finding: `%s %s` already resolves %s before --fix-path "+
				"ran, so this test cannot tell the written block from the environment.\n%s",
				l.probe.shell, strings.Join(l.probe.args, " "), bin, out)
		}
	}

	cmd := exec.Command(bin, "agent-setup", "--dir", project, "--agent", "claude", "--fix-path")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"CIVITAI_TOKEN=",
		"CIVITAI_NO_UPDATE_CHECK=1",
	)
	setupOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("`civitai agent-setup --fix-path` failed: %v\n%s\n\n"+
			"THIS IS THE RED AT origin/main: the flag does not exist there, so the remedy for "+
			"cli#665 cannot be applied and both probes below stay `command not found`.",
			err, setupOut)
	}

	// Requirement: it prints the full path of every file it wrote, because the
	// hosted setup prompt's step 3 requires the agent to relay that list.
	wroteSomething := false
	for _, name := range []string{".zshenv", ".profile", ".bash_profile", ".bash_login"} {
		p := filepath.Join(home, name)
		if _, serr := os.Stat(p); serr != nil {
			continue
		}
		wroteSomething = true
		if !strings.Contains(string(setupOut), p) {
			t.Errorf("--fix-path wrote %s without naming it in its output — a silent write outside "+
				"the project is a defect, not a tidiness question:\n%s", p, setupOut)
		}
	}
	if !wroteSomething {
		t.Fatalf("CONTROL failure: --fix-path exited 0 and created no startup file at all, so the "+
			"probes below would be measuring nothing.\n%s", setupOut)
	}

	for _, l := range found {
		out, err := runProbe(t, l.bin, l.probe, home)
		if err != nil {
			t.Errorf("`%s %s` cannot resolve `civitai` after --fix-path: %v\n%s",
				l.probe.shell, strings.Join(l.probe.args, " "), err, out)
			continue
		}
		if !strings.Contains(out, binDir) && !strings.Contains(out, marker) {
			t.Errorf("`%s %s` resolved a `civitai`, but not the one --fix-path was run from.\n"+
				"want output naming %s or the version marker %s\ngot: %s",
				l.probe.shell, strings.Join(l.probe.args, " "), binDir, marker, strings.TrimSpace(out))
		}
	}

	// Idempotence, end to end: a second run must not add a second block.
	cmd2 := exec.Command(bin, "agent-setup", "--dir", project, "--agent", "claude", "--fix-path")
	cmd2.Env = cmd.Env
	if out, err := cmd2.CombinedOutput(); err != nil {
		t.Fatalf("second `--fix-path` run failed: %v\n%s", err, out)
	}
	for _, name := range []string{".zshenv", ".profile", ".bash_profile", ".bash_login"} {
		p := filepath.Join(home, name)
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			continue
		}
		if n := strings.Count(string(b), "# BEGIN civitai cli PATH"); n > 1 {
			t.Errorf("%s holds %d managed PATH blocks after two runs. Only the first is ever "+
				"refreshed, so the rest keep exporting a stale directory forever:\n%s", p, n, b)
		}
	}
	for _, l := range found {
		out, err := runProbe(t, l.bin, l.probe, home)
		if err != nil || (!strings.Contains(out, binDir) && !strings.Contains(out, marker)) {
			t.Errorf("after the SECOND --fix-path run, `%s %s` no longer resolves the built CLI: "+
				"%v\n%s", l.probe.shell, strings.Join(l.probe.args, " "), err, out)
		}
	}

	var measured []string
	for _, l := range found {
		measured = append(measured, l.probe.shell+" "+strings.Join(l.probe.args, " "))
	}
	t.Logf("measured %d of %d probes: %s", len(found), len(fixPathShellProbes), strings.Join(measured, " | "))
	if len(missing) > 0 {
		t.Logf("NOT measured here (shell not installed): %s", strings.Join(missing, ", "))
	}
}
