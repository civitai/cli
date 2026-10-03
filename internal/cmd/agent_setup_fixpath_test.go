package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests for `civitai agent-setup --fix-path` (cli#665).
//
// 🔴 THE OBSERVABLE cli#665 NAMES IS MEASURED IN A DIFFERENT FILE, ON PURPOSE.
// `zsh -lic 'civitai --version'` and `bash -lc 'command -v civitai'` resolving
// the CLI after `--fix-path` ran needs the REAL binary — in a `go test` process
// os.Executable() is `cmd.test`, so everything here injects a seam over it, and a
// seam cannot answer "does the unseamed path work". That test builds
// ./cmd/civitai and runs both shells; it lives at the module root in
// agent_setup_fix_path_shell_test.go and references no symbol this change adds,
// which is what lets it be RUN (not merely fail to compile) at origin/main.
//
// What is here is everything a seam CAN answer: the file choice, the merge rule,
// the refusals, idempotence at the row level, the `--check` interaction, the
// footprint, and the cross-surface rule from decision 36.

// fakeCLIBinDir makes a directory holding a real, executable file named exactly
// what cliBinDirForPATH requires, and points cliExecutable at it for the test.
//
// It returns the directory, which is what the written block must name. The file
// prints a recognisable version string so an end-to-end shell probe can prove it
// ran THIS binary rather than some civitai on the developer's own PATH.
func fakeCLIBinDir(t *testing.T) (dir, marker string) {
	t.Helper()
	dir = t.TempDir()
	marker = "civitai 0.0.0-fixpath-fixture"
	exe := filepath.Join(dir, cliBinaryBaseName())
	script := "#!/bin/sh\necho '" + marker + "'\n"
	if runtime.GOOS == "windows" {
		script = "@echo " + marker + "\r\n"
	}
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := cliExecutable
	cliExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { cliExecutable = prev })
	return dir, marker
}

// agentSetupJSONPayload runs the command with --json and decodes the payload.
func agentSetupJSONPayload(t *testing.T, args ...string) (agentSetupJSON, string, error) {
	t.Helper()
	out, _, err := run(t, append([]string{"agent-setup", "--json"}, args...)...)
	var payload agentSetupJSON
	if out != "" {
		if jerr := json.Unmarshal([]byte(out), &payload); jerr != nil {
			t.Fatalf("decoding --json payload: %v\n%s", jerr, out)
		}
	}
	return payload, out, err
}

// rowFor returns the change row for one path.
func rowFor(payload agentSetupJSON, path string) (agentChangeJSON, bool) {
	for _, c := range payload.Changes {
		if c.Path == path {
			return c, true
		}
	}
	return agentChangeJSON{}, false
}

// agentSetupProjectWithFakeCLI is agentSetupProject plus the fake binary, in the
// order that matters: agentSetupProject sets HOME, so the fixture must be built
// after it or `~` would point at the developer's real home.
func agentSetupProjectWithFakeCLI(t *testing.T) (projectDir, marker string) {
	t.Helper()
	projectDir, _ = agentSetupProject(t)
	_, marker = fakeCLIBinDir(t)
	return projectDir, marker
}

// ---------------------------------------------------------------------------
// Opt-in: a default run is unchanged
// ---------------------------------------------------------------------------

// TestWithoutFixPathNoShellProfileIsEverWritten pins requirement 1 of the
// design: `agent-setup` with no `--fix-path` behaves as it did before the flag
// existed.
//
// ⚠ THIS IS AN INVARIANT GUARD, NOT REGRESSION COVERAGE. The defect cli#665
// records never violated it — nothing before this change wrote a startup file,
// so this test is GREEN at origin/main and cannot be shown to fail there. It is
// counted as what it is: the guard that makes "opt-in" a property rather than a
// claim, and the thing that goes red if anyone later makes the write implicit.
func TestWithoutFixPathNoShellProfileIsEverWritten(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	home := os.Getenv("HOME")

	payload, out, err := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude")
	if err != nil {
		t.Fatalf("agent-setup: %v\n%s", err, out)
	}
	if len(payload.Changes) != 3 {
		var paths []string
		for _, c := range payload.Changes {
			paths = append(paths, c.Path+" ("+c.Action+")")
		}
		t.Errorf("a default run reported %d change rows, want the 3 it has always reported "+
			"(AGENTS.md, CLAUDE.md, the MCP config): %v", len(payload.Changes), paths)
	}
	for _, name := range []string{".zshenv", ".zprofile", ".zshrc", ".bash_profile", ".bash_login", ".profile", ".bashrc"} {
		p := filepath.Join(home, name)
		if _, serr := os.Stat(p); serr == nil {
			t.Errorf("a run WITHOUT --fix-path created %s. The flag is opt-in precisely because this "+
				"command is the first thing a new user runs, and editing their login files "+
				"unasked is a surprise, not a setup step.", p)
		}
	}
	if strings.Contains(out, "PATH:") {
		t.Errorf("a default run printed the PATH footprint section:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Idempotence and the no-clobber rule
// ---------------------------------------------------------------------------

// TestFixPathIsIdempotent is the property that buys the right to skip the
// detection three earlier drafts got wrong: because applying the block twice is
// a no-op, nothing has to ask whether it is needed.
func TestFixPathIsIdempotent(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	home := os.Getenv("HOME")

	first, out, err := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude", "--fix-path")
	if err != nil {
		t.Fatalf("first --fix-path run: %v\n%s", err, out)
	}
	targets, terr := pathFixTargets(liveAgentEnv(dir))
	if terr != nil {
		t.Fatalf("pathFixTargets: %v", terr)
	}
	if len(targets) < 2 {
		t.Fatalf("PREMISE BROKEN: %d startup target(s). This test exists to prove BOTH shells' "+
			"files are idempotent; one target proves half of it", len(targets))
	}
	afterFirst := map[string]string{}
	for _, tgt := range targets {
		row, ok := rowFor(first, tgt.Path)
		if !ok {
			t.Fatalf("no change row for %s in:\n%s", tgt.Path, out)
		}
		if row.Action != string(actionCreate) {
			t.Errorf("first run reported %q for %s, want %q", row.Action, tgt.Path, actionCreate)
		}
		afterFirst[tgt.Path] = readFile(t, tgt.Path)
	}

	second, out2, err := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude", "--fix-path")
	if err != nil {
		t.Fatalf("second --fix-path run: %v\n%s", err, out2)
	}
	for _, tgt := range targets {
		got := readFile(t, tgt.Path)
		if n := strings.Count(got, pathFixBeginMarker); n != 1 {
			t.Errorf("%s holds %d BEGIN markers after two runs, want exactly 1 — a second block "+
				"would keep exporting a stale directory forever, because only the first is ever "+
				"refreshed:\n%s", tgt.Path, n, got)
		}
		if n := strings.Count(got, pathFixEndMarker); n != 1 {
			t.Errorf("%s holds %d END markers after two runs, want exactly 1", tgt.Path, n)
		}
		if got != afterFirst[tgt.Path] {
			t.Errorf("%s changed between two identical runs.\nfirst:\n%s\nsecond:\n%s",
				tgt.Path, afterFirst[tgt.Path], got)
		}
		if row, ok := rowFor(second, tgt.Path); !ok {
			t.Errorf("no change row for %s on the second run", tgt.Path)
		} else if row.Action != string(actionUnchanged) {
			t.Errorf("second run reported %q for %s, want %q — the run has nothing to do and must "+
				"say so", row.Action, tgt.Path, actionUnchanged)
		}
	}
	_ = home
}

// TestFixPathKeepsEveryByteOutsideItsMarkers pins the no-clobber rule on files
// that decide what every future shell can run.
func TestFixPathKeepsEveryByteOutsideItsMarkers(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	env := liveAgentEnv(dir)

	const userContent = "# my own profile\nexport EDITOR=vi\nalias ll='ls -al'\n"
	targets, err := pathFixTargets(env)
	if err != nil {
		t.Fatalf("pathFixTargets: %v", err)
	}
	for _, tgt := range targets {
		writeFile(t, tgt.Path, userContent)
	}
	// Re-plan: seeding `.profile` does not change the bash precedence (it is the
	// fallback), but re-reading keeps this test honest if that ever changes.
	targets, err = pathFixTargets(liveAgentEnv(dir))
	if err != nil {
		t.Fatalf("pathFixTargets (second read): %v", err)
	}

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path"); err != nil {
		t.Fatalf("agent-setup --fix-path: %v", err)
	}
	for _, tgt := range targets {
		got := readFile(t, tgt.Path)
		if !strings.HasPrefix(got, userContent) {
			t.Errorf("%s no longer opens with the user's own content.\nwant prefix:\n%s\ngot:\n%s",
				tgt.Path, userContent, got)
		}
		if !strings.Contains(got, pathFixBeginMarker) {
			t.Errorf("%s did not gain the managed block:\n%s", tgt.Path, got)
		}
	}
}

// countPathEntries counts how many of PATH's entries are exactly dir.
//
// 🔴 strings.Count IS THE WRONG TOOL HERE, AND A MUTANT PROVED IT. The obvious
// `strings.Count(":"+path+":", ":"+dir+":")` is NON-OVERLAPPING, so two ADJACENT
// duplicates — `dir:dir:rest`, which is exactly what a broken guard produces —
// share the colon between them and count as ONE. Measured: a mutant that
// prepended in BOTH arms of the `case` (so the guard was present and inert)
// SURVIVED the whole package until this function replaced that expression.
func countPathEntries(pathVar, dir string) int {
	n := 0
	for _, e := range filepath.SplitList(pathVar) {
		if e == dir {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// The emitted snippet decides at SHELL STARTUP, not at write time
// ---------------------------------------------------------------------------

// TestTheEmittedBlockDecidesAtShellStartupNotAtWriteTime is the structural half
// of the decision recorded in claudedocs/decisions/39-agent-setup-fix-path.md:
// the "is it already on PATH?" question is answered by the user's shell, every
// time it starts, so this command never has to run one.
//
// It then checks the BEHAVIOUR with a real `sh`, because a structural check
// type-checks past a conditional that is spelled right and tests the wrong
// thing: sourcing the block twice must leave the directory on PATH exactly once.
func TestTheEmittedBlockDecidesAtShellStartupNotAtWriteTime(t *testing.T) {
	const dir = "/opt/some/bin"
	block := pathFixBlock(dir)
	for _, want := range []string{`case ":$PATH:" in`, `*":$civitai_cli_dir:"*)`, `PATH="$civitai_cli_dir:$PATH"`} {
		if !strings.Contains(block, want) {
			t.Errorf("the emitted block does not contain %q.\nThe runtime conditional is what replaces "+
				"the login-shell probe decision 36 abandoned — without it this command would have to "+
				"decide reachability itself, which is the thing that was measured wrong three times.\n%s",
				want, block)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("NOT A PASS: no `sh` on PATH, so the runtime behaviour of the block was not measured: %v", err)
	}
	f := filepath.Join(t.TempDir(), "block.sh")
	if werr := os.WriteFile(f, []byte(block), 0o644); werr != nil {
		t.Fatal(werr)
	}
	out, err := exec.Command(sh, "-c", ". "+f+"; . "+f+"; printf %s \"$PATH\"").CombinedOutput()
	if err != nil {
		t.Fatalf("sourcing the block twice failed: %v\n%s", err, out)
	}
	got := string(out)
	if n := countPathEntries(got, dir); n != 1 {
		t.Errorf("sourcing the block twice put %s on PATH %d time(s), want exactly 1. The `case` guard "+
			"is what stops a profile growing a duplicate entry on every shell start.\nPATH=%s", dir, n, got)
	}
}

// TestPathFixBlockSurvivesShellMetacharactersInTheDirectory drives the quoting,
// because a path is not an identifier: a space makes two words, and `*` or `[`
// in an unquoted `case` pattern is a glob.
func TestPathFixBlockSurvivesShellMetacharactersInTheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("NOT A PASS: no `sh` on PATH, so the quoting was not measured: %v", err)
	}
	for _, dir := range []string{
		"/opt/My Apps/bin",
		"/opt/weird'quote/bin",
		"/opt/glob*[a-z]/bin",
		`/opt/dollar$HOME/bin`,
	} {
		t.Run(dir, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "block.sh")
			if werr := os.WriteFile(f, []byte(pathFixBlock(dir)), 0o644); werr != nil {
				t.Fatal(werr)
			}
			out, cerr := exec.Command(sh, "-c", ". "+shellSingleQuote(f)+"; printf %s \"$PATH\"").CombinedOutput()
			if cerr != nil {
				t.Fatalf("sourcing the block failed: %v\n%s", cerr, out)
			}
			if !strings.Contains(string(out), dir) {
				t.Errorf("after sourcing, PATH does not contain %q literally.\nPATH=%s", dir, out)
			}
			// And twice is still once — the quoting must not break the guard.
			out2, cerr := exec.Command(sh, "-c", ". "+shellSingleQuote(f)+"; . "+shellSingleQuote(f)+
				"; printf %s \"$PATH\"").CombinedOutput()
			if cerr != nil {
				t.Fatalf("sourcing twice failed: %v\n%s", cerr, out2)
			}
			if n := countPathEntries(string(out2), dir); n != 1 {
				t.Errorf("sourcing twice put %q on PATH %d time(s), want 1.\nPATH=%s", dir, n, out2)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// --dry-run and --check
// ---------------------------------------------------------------------------

// TestFixPathDryRunWritesNothingAndPrintsTheBlock pins requirement 5.
func TestFixPathDryRunWritesNothingAndPrintsTheBlock(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	home := os.Getenv("HOME")

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path", "--dry-run")
	if err != nil {
		t.Fatalf("--fix-path --dry-run: %v\n%s", err, out)
	}
	targets, terr := pathFixTargets(liveAgentEnv(dir))
	if terr != nil {
		t.Fatalf("pathFixTargets: %v", terr)
	}
	for _, tgt := range targets {
		if _, serr := os.Stat(tgt.Path); serr == nil {
			t.Errorf("--dry-run created %s. It reports a plan; it writes nothing.", tgt.Path)
		}
		if !strings.Contains(out, tgt.Path) {
			t.Errorf("--dry-run did not name %s in its report:\n%s", tgt.Path, out)
		}
	}
	// It prints what WOULD be written: the conditional, and the resolved dir.
	cliDir, derr := cliBinDirForPATH()
	if derr != nil {
		t.Fatalf("cliBinDirForPATH: %v", derr)
	}
	for _, want := range []string{`case ":$PATH:" in`, cliDir, "WOULD write"} {
		if !strings.Contains(out, want) {
			t.Errorf("--fix-path --dry-run output does not contain %q:\n%s", want, out)
		}
	}
	_ = home
}

// TestFixPathIsRefusedWithCheck pins the interaction, which is a DECISION and not
// an omission: `--check` writes nothing and its `--json` shape is a published
// contract with ledgered consumers, so the flag that writes is refused there
// rather than quietly reinterpreted.
func TestFixPathIsRefusedWithCheck(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	_, stderr, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check", "--fix-path")
	if err == nil {
		t.Fatalf("`--check --fix-path` succeeded; it must be refused")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("`--check --fix-path` returned %v, which is not tagged ErrUsage — a mistake about the "+
			"INVOCATION exits 2 in this command, like every other usage error", err)
	}
	msg := err.Error() + stderr
	for _, want := range []string{"--check", "--fix-path", "--dry-run"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not mention %q, so it does not name the next command to run: %q", want, msg)
		}
	}
	// It must also have written nothing at all.
	home := os.Getenv("HOME")
	for _, name := range []string{".zshenv", ".profile", ".bash_profile"} {
		if _, serr := os.Stat(filepath.Join(home, name)); serr == nil {
			t.Errorf("the refused run still wrote %s", filepath.Join(home, name))
		}
	}
}

// TestFixPathAddsNoRowToTheCheckContract is the other half of that decision, and
// it is the one with a ledgered consumer: `--check --json` emits exactly the row
// set `developer.civitai.com`'s setup prompt was written against, and a
// machine-dependent row there is read by a consumer that only looks at `ok`.
func TestFixPathAddsNoRowToTheCheckContract(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	payload, out, _ := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude", "--check")
	if len(payload.Checks) == 0 {
		t.Fatalf("PREMISE BROKEN: `--check --json` emitted no rows, so this guard checks nothing:\n%s", out)
	}
	for _, c := range payload.Checks {
		switch {
		case strings.Contains(c.Name, "path"), strings.Contains(c.Name, "shell"), strings.Contains(c.Name, "profile"):
			t.Errorf("`--check` grew the row %q. A row about this machine's PATH is a MACHINE-dependent "+
				"verdict in a contract whose consumers read `ok` — exactly the axis decision 36 records "+
				"as abandoned. Detail: %q", c.Name, c.Detail)
		}
	}
}

// ---------------------------------------------------------------------------
// The refusals
// ---------------------------------------------------------------------------

// TestFixPathRefusesAnExecutableNotNamedCivitai is the guard that keeps the whole
// claim true: adding a directory to PATH only makes the WORD `civitai` resolve if
// a file by that name is in it.
func TestFixPathRefusesAnExecutableNotNamedCivitai(t *testing.T) {
	dir, _ := agentSetupProject(t)
	bin := t.TempDir()
	odd := filepath.Join(bin, "civitai-0.1.2")
	if err := os.WriteFile(odd, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := cliExecutable
	cliExecutable = func() (string, error) { return odd, nil }
	t.Cleanup(func() { cliExecutable = prev })

	if _, err := cliBinDirForPATH(); err == nil {
		t.Fatalf("cliBinDirForPATH accepted an executable named %q; a PATH entry for its directory "+
			"would not make `civitai` resolve", filepath.Base(odd))
	} else if !strings.Contains(err.Error(), "civitai-0.1.2") || !strings.Contains(err.Error(), "would not make") {
		t.Errorf("the refusal does not name what it found or why it matters: %v", err)
	}

	payload, out, err := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude", "--fix-path")
	if err == nil {
		t.Fatalf("the run exited 0 with an unusable binary name:\n%s", out)
	}
	if payload.OK {
		t.Errorf("ok:true for a run that wrote no PATH block:\n%s", out)
	}
	blocked := 0
	for _, c := range payload.Changes {
		if c.Action == actionBlocked && strings.Contains(c.Reason, "civitai-0.1.2") {
			blocked++
		}
	}
	if blocked == 0 {
		t.Errorf("no blocked row carries the refusal, so the report does not say why nothing "+
			"happened:\n%s", out)
	}
	home := os.Getenv("HOME")
	for _, name := range []string{".zshenv", ".profile"} {
		if _, serr := os.Stat(filepath.Join(home, name)); serr == nil {
			t.Errorf("a refused run still wrote %s", filepath.Join(home, name))
		}
	}
}

// TestMergePathFixBlockRefusesWhatItCannotRewrite drives the merge rule directly,
// in both directions: the three cases it handles and the two it refuses.
func TestMergePathFixBlockRefusesWhatItCannotRewrite(t *testing.T) {
	block := pathFixBlock("/opt/bin")
	for _, tc := range []struct {
		name     string
		existing string
		want     fileAction
		wantErr  string
	}{
		{"empty file", "", actionCreate, ""},
		{"no markers", "export EDITOR=vi\n", actionAppend, ""},
		{"already current", block, actionUnchanged, ""},
		{"stale block", pathFixBlock("/old/bin"), actionReplace, ""},
		{"two blocks", block + "\n" + block, "", "contains 2 civitai PATH blocks"},
		{"begin only", pathFixBeginMarker + "\nPATH=x\n", "", "only one half"},
		{"end only", "PATH=x\n" + pathFixEndMarker + "\n", "", "only one half"},
		{"out of order", pathFixEndMarker + "\nPATH=x\n" + pathFixBeginMarker + "\n", "", "only one half"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, action, err := mergePathFixBlock("/home/u/.profile", tc.existing, block)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want a refusal containing %q, got action %q and:\n%s", tc.wantErr, action, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("refusal %q does not contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if action != tc.want {
				t.Errorf("action %q, want %q", action, tc.want)
			}
			if n := strings.Count(got, pathFixBeginMarker); n != 1 {
				t.Errorf("result holds %d BEGIN markers, want 1:\n%s", n, got)
			}
			if !strings.Contains(got, "/opt/bin") {
				t.Errorf("result does not carry the new directory:\n%s", got)
			}
		})
	}
}

// TestFixPathWithNoResolvableHomeIsBlockedNotSilentlyOK: the user asked for a
// write in as many words, so "there was nowhere to write it" must not exit 0.
func TestFixPathWithNoResolvableHomeIsBlockedNotSilentlyOK(t *testing.T) {
	env := agentEnv{Home: "   ", Dir: t.TempDir(), GOOS: runtime.GOOS}
	if _, _, err := planPathFix(env); err == nil {
		t.Fatalf("planPathFix accepted an unresolvable home")
	} else if !strings.Contains(err.Error(), "HOME") {
		t.Errorf("the refusal does not name the variable to set: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Which startup files, and why those
// ---------------------------------------------------------------------------

// TestPathFixTargetsFollowTheShellsOwnPrecedence is the discriminating test for
// requirement 2's "choose the files that actually achieve this".
//
// 🔴 THE BASH ROW IS THE ONE THAT CAN BE WRONG WHILE LOOKING RIGHT. bash in login
// mode reads the FIRST that exists of ~/.bash_profile, ~/.bash_login, ~/.profile
// and STOPS. So always writing ~/.profile — the obvious choice, and the one a
// "write the portable file" instinct produces — is silently ineffective on every
// machine that has a ~/.bash_profile, which is most macOS setups and anything
// touched by nvm, rbenv or pyenv.
func TestPathFixTargetsFollowTheShellsOwnPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seed     []string
		wantBash string
	}{
		{"nothing exists", nil, ".profile"},
		{"only .profile", []string{".profile"}, ".profile"},
		{"a .bash_profile exists", []string{".bash_profile"}, ".bash_profile"},
		{"a .bash_login exists", []string{".bash_login"}, ".bash_login"},
		{"both bash files exist", []string{".bash_profile", ".bash_login"}, ".bash_profile"},
		{".bash_profile beside .profile", []string{".bash_profile", ".profile"}, ".bash_profile"},
		{".bashrc does not count", []string{".bashrc"}, ".profile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			for _, n := range tc.seed {
				writeFile(t, filepath.Join(home, n), "# seeded\n")
			}
			env := agentEnv{
				Home: home,
				GOOS: runtime.GOOS,
				Exists: func(p string) bool {
					_, err := os.Stat(p)
					return err == nil
				},
			}
			targets, err := pathFixTargets(env)
			if err != nil {
				t.Fatalf("pathFixTargets: %v", err)
			}
			if len(targets) != 2 {
				t.Fatalf("got %d target(s), want 2 (one per shell family): %+v", len(targets), targets)
			}
			if want := filepath.Join(home, ".zshenv"); targets[0].Path != want {
				t.Errorf("zsh target %s, want %s — zsh never reads ~/.profile, and .zshenv is the one "+
					"file EVERY zsh reads (login or not, interactive or not)", targets[0].Path, want)
			}
			if want := filepath.Join(home, tc.wantBash); targets[1].Path != want {
				t.Errorf("bash target %s, want %s — bash login reads the first of "+
					".bash_profile/.bash_login/.profile that exists and stops there",
					targets[1].Path, want)
			}
			for _, tgt := range targets {
				if strings.TrimSpace(tgt.Why) == "" {
					t.Errorf("target %s carries no reason; the row's `reason` is how a reader finds out "+
						"why THAT file", tgt.Path)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The cross-surface rule from decision 36
// ---------------------------------------------------------------------------

// TestFixPathWritesNoMachinePathIntoAGENTSMD is the guard that keeps this feature
// on the right side of decision 36: `--fix-path` is a per-MACHINE action, and
// AGENTS.md is normally COMMITTED, so not one byte of what this flag resolves may
// end up in that file.
//
// agent_setup_path_note_test.go already forbids a fixed list of machine-specific
// spellings in the rendered block. This one is the RELATIONSHIP: whatever
// directory THIS run resolved must not appear in the AGENTS.md it wrote — which
// holds for a directory nobody has thought of yet.
func TestFixPathWritesNoMachinePathIntoAGENTSMD(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	cliDir, err := cliBinDirForPATH()
	if err != nil {
		t.Fatalf("cliBinDirForPATH: %v", err)
	}
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path"); err != nil {
		t.Fatalf("agent-setup --fix-path: %v", err)
	}
	agents := readFile(t, filepath.Join(dir, agentsFilename))
	if strings.Contains(agents, cliDir) {
		t.Errorf("the AGENTS.md this run wrote names %s — the directory --fix-path resolved on THIS "+
			"machine. That file is normally committed, so a machine path in it is wrong for everyone "+
			"who pulls; see claudedocs/decisions/36-agents-block-per-project.md.", cliDir)
	}
	if strings.Contains(agents, os.Getenv("HOME")) {
		t.Errorf("the AGENTS.md this run wrote names the home directory %q", os.Getenv("HOME"))
	}
	// And the startup files DID get it — otherwise the assertion above is
	// satisfied by a run that did nothing.
	targets, terr := pathFixTargets(liveAgentEnv(dir))
	if terr != nil {
		t.Fatalf("pathFixTargets: %v", terr)
	}
	for _, tgt := range targets {
		if got := readFile(t, tgt.Path); !strings.Contains(got, cliDir) {
			t.Fatalf("CONTROL failure: %s does not name %s, so the assertions above passed over a run "+
				"that wrote no PATH block:\n%s", tgt.Path, cliDir, got)
		}
	}
}

// TestFixPathPrintsEveryFileItWrote pins requirement 4: the hosted setup prompt's
// step 3 requires the agent to relay what was written, so a file the output does
// not name is a file the operator never hears about.
func TestFixPathPrintsEveryFileItWrote(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path")
	if err != nil {
		t.Fatalf("agent-setup --fix-path: %v\n%s", err, out)
	}
	targets, terr := pathFixTargets(liveAgentEnv(dir))
	if terr != nil {
		t.Fatalf("pathFixTargets: %v", terr)
	}
	for _, tgt := range targets {
		if !strings.Contains(out, tgt.Path) {
			t.Errorf("the output does not name %s, which it wrote:\n%s", tgt.Path, out)
		}
	}
	for _, want := range []string{"PATH:", "Wrote ", "Open a NEW shell"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output is missing %q — a reader who does not know that THIS shell is "+
				"unchanged re-runs the same failing command and concludes the flag did nothing:\n%s",
				want, out)
		}
	}
	if !strings.Contains(out, fmt.Sprintf("lives in %s", mustCLIDir(t))) {
		t.Errorf("the output does not say where the CLI lives:\n%s", out)
	}
}

// TestAFullyRefusedFixPathRunNeverClaimsAWriteOrANewShell is the guard for a
// defect an adversarial read of this change's own output found, not for one the
// issue records.
//
// 🔴 THE FIRST DRAFT PRINTED `Wrote <path>` FOR EVERY PATH IN THE REPORT, and
// told the reader to open a new shell, on a run whose destinations were all
// REFUSED and whose rows said so three lines above. That is the same class
// agentSetupHeadline exists for: a summary line contradicting the report it
// summarises. Both the footprint's verb and the next-step line now read the rows.
func TestAFullyRefusedFixPathRunNeverClaimsAWriteOrANewShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	dir, _ := agentSetupProjectWithFakeCLI(t)
	home := os.Getenv("HOME")

	// A broken symlink is refused by checkWriteTargetResolvable at PLAN time, which
	// is the one refusal reachable for both targets without changing permissions.
	targets, terr := pathFixTargets(liveAgentEnv(dir))
	if terr != nil {
		t.Fatalf("pathFixTargets: %v", terr)
	}
	for _, tgt := range targets {
		if err := os.Symlink(filepath.Join(home, "nowhere", "at", "all"), tgt.Path); err != nil {
			t.Fatalf("seeding a broken symlink at %s: %v", tgt.Path, err)
		}
	}

	payload, out, err := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude", "--fix-path")
	if err == nil {
		t.Fatalf("the run exited 0 with every startup file refused:\n%s", out)
	}
	if payload.OK {
		t.Errorf("ok:true for a run that wrote no PATH block:\n%s", out)
	}
	for _, tgt := range targets {
		row, ok := rowFor(payload, tgt.Path)
		if !ok {
			t.Fatalf("CONTROL failure: no row for %s, so the fixture did not produce the refusal "+
				"this test is about:\n%s", tgt.Path, out)
		}
		if row.Action != actionBlocked {
			t.Fatalf("CONTROL failure: %s reported %q, not %q — a broken symlink is supposed to be "+
				"refused at plan time, so this fixture no longer builds the state under test",
				tgt.Path, row.Action, actionBlocked)
		}
	}

	// The human output must not claim either thing.
	human, _, _ := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path")
	if strings.Contains(human, "Wrote ") {
		t.Errorf("the output says `Wrote ` on a run where every startup file was refused:\n%s", human)
	}
	if strings.Contains(human, "Open a NEW shell") {
		t.Errorf("the output tells the reader to open a new shell, which would pick up nothing — "+
			"every startup file was refused:\n%s", human)
	}
	if !strings.Contains(human, "REFUSED") {
		t.Errorf("the PATH footprint does not mark the refused files:\n%s", human)
	}
}

func mustCLIDir(t *testing.T) string {
	t.Helper()
	d, err := cliBinDirForPATH()
	if err != nil {
		t.Fatalf("cliBinDirForPATH: %v", err)
	}
	return d
}
