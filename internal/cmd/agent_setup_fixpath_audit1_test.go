package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/ui"
)

// lookPathSh, runShell and sourceAndPrintPATH are the three-line harness the
// block's behavioural tests share. They exist so a test that wants to know what a
// real shell does with the emitted block cannot accidentally assert on the
// STRUCTURE instead — the mistake the M1b mutant walked through.
func lookPathSh() (string, error) { return exec.LookPath("sh") }

func runShell(t *testing.T, sh, script string) (string, error) {
	t.Helper()
	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	return string(out), err
}

// sourceAndPrintPATH sources blockFile in a fresh shell and returns the resulting
// PATH. The file is single-quoted, so a temp directory with a space or a glob
// character in it does not become two words.
func sourceAndPrintPATH(t *testing.T, sh, blockFile string) string {
	t.Helper()
	out, err := runShell(t, sh, ". "+shellSingleQuote(blockFile)+"; printf %s \"$PATH\"")
	if err != nil {
		t.Fatalf("sourcing %s failed: %v\n%s", blockFile, err, out)
	}
	return out
}

// Round-1 audit findings on `--fix-path` (cli#777). Each test here was shown to
// FAIL at 166ffd4 before the fix it guards; the matrix is in
// claudedocs/decisions/39-agent-setup-fix-path.md.
//
// They live in their own file rather than beside the originals because the
// originals are organised by REQUIREMENT and these are organised by DEFECT —
// mixing them loses which assertion was shown to fail on pre-change code.

// pathFootprintLineFor returns the line of the `PATH:` footprint that names path.
//
// 🔴 SCOPING TO THE FOOTPRINT IS LOAD-BEARING. The `changes` rows above it also
// name every path, with their own verb, so an assertion over the whole output
// cannot tell the footprint's claim from the row's. The footprint is the surface
// under test: it is the one-line summary a reader skims.
func pathFootprintLineFor(t *testing.T, out, path string) string {
	t.Helper()
	i := strings.Index(out, "\nPATH:\n")
	if i < 0 {
		t.Fatalf("the output has no `PATH:` footprint section, so there is nothing to read a verb "+
			"out of:\n%s", out)
	}
	for _, line := range strings.Split(out[i+len("\nPATH:\n"):], "\n") {
		if strings.Contains(line, path) {
			return line
		}
	}
	t.Fatalf("the `PATH:` footprint does not name %s:\n%s", path, out)
	return ""
}

// ---------------------------------------------------------------------------
// R1 — the footprint's verb must be read out of EVERY row, not just `blocked`
// ---------------------------------------------------------------------------

// TestThePathFootprintVerbIsReadOutOfEveryFilesOwnRow is the guard for round 1's
// R1, and it fails in BOTH directions.
//
// 🔴 THE DOCSTRING ON printPathFixFootprint CLAIMED A RELATIONSHIP THE BODY
// IMPLEMENTED FOR ONE ARM. "THE VERB PER FILE IS READ OUT OF THAT FILE'S ROW"
// was true of `blocked` and of nothing else, so a second `--fix-path` run — whose
// plan is `unchanged`, whose write is skipped at agent_setup.go's
// `plan.Action == actionUnchanged`, and whose file mtime does not move — printed
// `Wrote <path>` three lines under a row reading `unchanged`. Mutant M14 fixed the
// `blocked` arm; nobody checked the others, and the auditor measured the lie
// UNGUARDED IN BOTH DIRECTIONS (patching the verb to be honest left the package
// green).
//
// So this table asserts the verb for every action a path row can carry, and it
// asserts both what the line MUST say and what it MUST NOT. A test that only
// checked `unchanged` would be walkable by hardcoding `unchanged` everywhere.
func TestThePathFootprintVerbIsReadOutOfEveryFilesOwnRow(t *testing.T) {
	const dir = "/opt/some/bin"
	// 🔴 THE FIXTURE PATHS DELIBERATELY SPELL NONE OF THE VERBS. A first draft
	// named them `.profile-<action>`, which made the `unchanged` row's positive
	// assertion satisfied by the PATH rather than by the verb — green while the
	// line read `Wrote /home/u/.profile-unchanged`. A fixture that can only
	// produce the asserted literal cannot see the mutant.
	rows := []struct {
		path       string
		action     string
		wantVerb   string
		wantDryRun string
	}{
		{".s1", string(actionCreate), "Wrote", "WOULD write"},
		{".s2", string(actionAppend), "Wrote", "WOULD write"},
		{".s3", string(actionReplace), "Wrote", "WOULD write"},
		{".s4", string(actionUnchanged), "unchanged", "unchanged"},
		{".s5", actionBlocked, "REFUSED", "REFUSED"},
	}

	for _, dryRun := range []bool{false, true} {
		var changes []agentChangeJSON
		pf := pathFixReport{Requested: true, Dir: dir, Block: pathFixBlock(dir)}
		for _, r := range rows {
			p := filepath.Join("/home/u", r.path)
			changes = append(changes, agentChangeJSON{Path: p, Action: r.action, Reason: "fixture"})
			pf.Paths = append(pf.Paths, p)
		}

		var buf bytes.Buffer
		printPathFixFootprint(&buf, ui.For(&buf), changes, pf, dryRun)
		out := buf.String()

		for _, r := range rows {
			p := filepath.Join("/home/u", r.path)
			line := pathFootprintLineFor(t, out, p)
			want := r.wantVerb
			if dryRun {
				want = r.wantDryRun
			}
			if !strings.Contains(line, want) {
				t.Errorf("dryRun=%v: the footprint line for a %q row is %q, which does not carry the "+
					"verb %q. The verb is a CLAIM about what happened to THAT file, and it must be "+
					"read out of that file's row — a `%s` row means nothing was written to it.",
					dryRun, r.action, strings.TrimSpace(line), want, r.action)
			}
			// The other direction: no line may carry a verb belonging to a
			// different action. This is what a "hardcode `unchanged` everywhere"
			// fix would trip over.
			for _, other := range []string{"Wrote", "WOULD write", "unchanged", "REFUSED"} {
				if other == want || strings.Contains(want, other) || strings.Contains(other, want) {
					continue
				}
				if strings.Contains(line, other) {
					t.Errorf("dryRun=%v: the footprint line for a %q row is %q, which carries %q — "+
						"a verb that belongs to a different action. Want %q and nothing else.",
						dryRun, r.action, strings.TrimSpace(line), other, want)
				}
			}
		}
	}
}

// TestASecondFixPathRunDoesNotClaimAWriteItDidNotMake is the same defect measured
// end to end, through the real command, with the file's mtime as the witness.
//
// A second `--fix-path` run plans `actionUnchanged`, and agent_setup.go's write
// closure returns nil without touching the file. The footprint must say so.
func TestASecondFixPathRunDoesNotClaimAWriteItDidNotMake(t *testing.T) {
	dir, _ := agentSetupProjectWithFakeCLI(t)

	if out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path"); err != nil {
		t.Fatalf("first --fix-path run: %v\n%s", err, out)
	}
	targets, terr := pathFixTargets(liveAgentEnv(dir))
	if terr != nil {
		t.Fatalf("pathFixTargets: %v", terr)
	}
	before := map[string]os.FileInfo{}
	for _, tgt := range targets {
		fi, serr := os.Stat(tgt.Path)
		if serr != nil {
			t.Fatalf("CONTROL failure: the first run did not create %s, so the second run's verb is "+
				"not the thing under test: %v", tgt.Path, serr)
		}
		before[tgt.Path] = fi
	}

	payload, jout, err := agentSetupJSONPayload(t, "--dir", dir, "--agent", "claude", "--fix-path")
	if err != nil {
		t.Fatalf("second --fix-path run: %v\n%s", err, jout)
	}
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--fix-path")
	if err != nil {
		t.Fatalf("third --fix-path run: %v\n%s", err, out)
	}

	for _, tgt := range targets {
		row, ok := rowFor(payload, tgt.Path)
		if !ok {
			t.Fatalf("CONTROL failure: no change row for %s on a repeat run:\n%s", tgt.Path, jout)
		}
		if row.Action != string(actionUnchanged) {
			t.Fatalf("CONTROL failure: a repeat run reported %q for %s, not %q — this test is about "+
				"what the footprint says on an `unchanged` row, and the fixture no longer builds one",
				row.Action, tgt.Path, actionUnchanged)
		}
		after, serr := os.Stat(tgt.Path)
		if serr != nil {
			t.Fatalf("stat %s: %v", tgt.Path, serr)
		}
		if !after.ModTime().Equal(before[tgt.Path].ModTime()) {
			t.Errorf("%s was rewritten by a repeat run (mtime %v -> %v). The plan said `unchanged`; "+
				"the write is supposed to be skipped.", tgt.Path, before[tgt.Path].ModTime(), after.ModTime())
		}

		line := pathFootprintLineFor(t, out, tgt.Path)
		if strings.Contains(line, "Wrote") {
			t.Errorf("the PATH footprint says %q for %s on a repeat run. The file was NOT written — "+
				"its mtime did not move and the row three lines above reads `unchanged`. A footprint "+
				"claiming a write the report denies is the defect agentSetupHeadline exists for, one "+
				"surface over.", strings.TrimSpace(line), tgt.Path)
		}
		if !strings.Contains(line, string(actionUnchanged)) {
			t.Errorf("the PATH footprint line for %s is %q, which does not say the file was left "+
				"alone — a reader cannot tell a refreshed file from an untouched one",
				tgt.Path, strings.TrimSpace(line))
		}
	}
}

// ---------------------------------------------------------------------------
// R2 — a PATH separator in the directory is a confident false fix
// ---------------------------------------------------------------------------

// TestFixPathRefusesADirectoryHoldingThePATHSeparator is round 1's R2.
//
// 🔴 THE SEPARATOR IS THE ONE CHARACTER THAT STRUCTURALLY CANNOT APPEAR IN A PATH
// ENTRY. `cliBinDirForPATH` refused `\n` and `\r` and nothing else, so a binary in
// `…/a:b/` wrote the block, reported `Wrote` for both files, told the reader to
// open a NEW shell and exited 0 with `ok: true` — while the block it wrote put TWO
// directories on PATH, neither of which exists. A false fix in a file the user
// then trusts is the exact failure mode the base-name refusal exists to prevent,
// one character over.
func TestFixPathRefusesADirectoryHoldingThePATHSeparator(t *testing.T) {
	sep := string(os.PathListSeparator)
	project, home := agentSetupProject(t)

	bad := filepath.Join(t.TempDir(), "a"+sep+"b")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Skipf("NOT A PASS: this filesystem will not hold a directory named %q, so the refusal was "+
			"not measured: %v", "a"+sep+"b", err)
	}
	exe := filepath.Join(bad, cliBinaryBaseName())
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho civitai 0.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := cliExecutable
	cliExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { cliExecutable = prev })

	_, err := cliBinDirForPATH()
	if err == nil {
		t.Fatalf("cliBinDirForPATH accepted %q. The block it would write prepends "+
			"%q to PATH, which the shell splits on %q into entries that do not exist — so "+
			"`civitai` still does not resolve and the command reports success.", bad, bad, sep)
	}
	if !strings.Contains(err.Error(), sep) || !strings.Contains(err.Error(), "PATH") {
		t.Errorf("the refusal does not name the character it found or say why PATH cannot hold it: %v", err)
	}

	payload, out, rerr := agentSetupJSONPayload(t, "--dir", project, "--agent", "claude", "--fix-path")
	if rerr == nil {
		t.Fatalf("the run exited 0 with a directory PATH cannot express:\n%s", out)
	}
	if payload.OK {
		t.Errorf("ok:true for a run whose PATH block could never work:\n%s", out)
	}
	blocked := 0
	for _, c := range payload.Changes {
		if c.Action == actionBlocked && strings.Contains(c.Reason, "PATH") {
			blocked++
		}
	}
	if blocked == 0 {
		t.Errorf("no blocked row explains why nothing happened:\n%s", out)
	}
	for _, name := range []string{".zshenv", ".profile", ".bash_profile"} {
		if _, serr := os.Stat(filepath.Join(home, name)); serr == nil {
			t.Errorf("a refused run still wrote %s", filepath.Join(home, name))
		}
	}
}

// ---------------------------------------------------------------------------
// R4 — the block must not shadow a later install from another prefix
// ---------------------------------------------------------------------------

// TestTheBlockAddsNothingWhenTheDirectoryNoLongerHoldsTheCLI is round 1's R4.
//
// 🔴 THE BLOCK PINS ONE ABSOLUTE DIRECTORY AND PREPENDS IT. Within one npm prefix
// that is right — a re-install lands in the same directory. Across prefixes it is
// not: after an nvm node switch or a `--prefix` change the global install lands
// elsewhere, and a startup file that still puts the OLD directory FIRST makes
// `civitai --version` report the stale build indefinitely while `npm update -g`
// looks inert. (How often users change prefix was not measured; the
// prepend-wins mechanism is certain from the block itself.) The `[ -x … ]` guard
// makes the stale entry drop out the moment the file it names is gone, which is
// also the uninstall case.
func TestTheBlockAddsNothingWhenTheDirectoryNoLongerHoldsTheCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted block is POSIX sh")
	}
	sh, err := lookPathSh()
	if err != nil {
		t.Skipf("NOT A PASS: no `sh` on PATH, so the block's runtime behaviour was not measured: %v", err)
	}

	dir := t.TempDir()
	block := filepath.Join(t.TempDir(), "block.sh")
	if werr := os.WriteFile(block, []byte(pathFixBlock(dir)), 0o644); werr != nil {
		t.Fatal(werr)
	}

	// NEGATIVE ARM: the directory holds no `civitai`, so the block must add nothing.
	got := sourceAndPrintPATH(t, sh, block)
	if n := countPathEntries(got, dir); n != 0 {
		t.Errorf("the block put %s on PATH %d time(s) while that directory holds no %q. A startup "+
			"file that prepends a directory unconditionally SHADOWS every later install elsewhere — "+
			"`civitai --version` then reports the stale build forever and an uninstall leaves a dead "+
			"entry first on PATH.\nPATH=%s", dir, n, cliBinaryBaseName(), got)
	}

	// POSITIVE ARM: with the file there, the block must still work. Without this
	// the assertion above is satisfied by a block that does nothing at all.
	if werr := os.WriteFile(filepath.Join(dir, cliBinaryBaseName()),
		[]byte("#!/bin/sh\necho civitai 0.0.0\n"), 0o755); werr != nil {
		t.Fatal(werr)
	}
	got = sourceAndPrintPATH(t, sh, block)
	if n := countPathEntries(got, dir); n != 1 {
		t.Errorf("CONTROL failure: with an executable %q in it, the block put %s on PATH %d time(s), "+
			"want 1 — the negative arm above proved nothing.\nPATH=%s",
			cliBinaryBaseName(), dir, n, got)
	}

	// And a non-executable file is not a CLI either.
	bare := t.TempDir()
	if werr := os.WriteFile(filepath.Join(bare, cliBinaryBaseName()), []byte("not executable\n"), 0o644); werr != nil {
		t.Fatal(werr)
	}
	block2 := filepath.Join(t.TempDir(), "block2.sh")
	if werr := os.WriteFile(block2, []byte(pathFixBlock(bare)), 0o644); werr != nil {
		t.Fatal(werr)
	}
	if n := countPathEntries(sourceAndPrintPATH(t, sh, block2), bare); n != 0 {
		t.Errorf("the block prepended %s, which holds a NON-executable %q — no shell can run it, so "+
			"the entry buys nothing and shadows whatever comes after", bare, cliBinaryBaseName())
	}
}

// ---------------------------------------------------------------------------
// R5 — a CRLF-saved block must not leave a lone CR line behind
// ---------------------------------------------------------------------------

// TestReplacingACRLFSavedBlockLeavesNoLoneCarriageReturn is round 1's R5.
//
// 🔴 A LONE-CR LINE ERRORS AT EVERY SHELL START. `mergePathFixBlock` dropped a
// single leading `"\n"` from the tail so the block does not grow a blank line per
// run; on a file saved with CRLF the tail begins `"\r\n"`, so the `"\r"` survives
// as a line of its own and bash/dash/zsh answer `$'\r': command not found` on
// every new shell. Reachable on the shipped Windows targets: `.goreleaser.yaml`
// builds windows amd64 and arm64 and nothing gates `--fix-path` on GOOS.
//
// ⚠ THE CLAIM IS "NO *NEW* LONE-CR LINE", AND THE FIXTURE ISOLATES THAT. A
// startup file saved wholly in CRLF is already degraded for a POSIX shell before
// this command touches it — a blank `\r\n` line in it is already a CR-only line,
// and preserving it is the no-clobber rule working as intended. What this guards
// is the line the REPLACE creates out of the old block's own terminator, which
// did not exist in the file beforehand.
func TestReplacingACRLFSavedBlockLeavesNoLoneCarriageReturn(t *testing.T) {
	crlf := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	}
	existing := crlf("# my own profile\nexport EDITOR=vi\n" + pathFixBlock("/old/bin") + "alias ll='ls -al'\n")
	if !strings.Contains(existing, "\r\n") {
		t.Fatalf("PREMISE BROKEN: the fixture is not CRLF")
	}

	updated, action, err := mergePathFixBlock("/home/u/.profile", existing, pathFixBlock("/new/bin"))
	if err != nil {
		t.Fatalf("merging into a CRLF file: %v", err)
	}
	if action != actionReplace {
		t.Fatalf("CONTROL failure: action %q, want %q — the fixture does not exercise the replace "+
			"arm, which is the only one that trims the tail", action, actionReplace)
	}
	if strings.Contains(updated, "\n\r\n") {
		t.Errorf("the merged file holds a line that is a lone carriage return. bash, dash and zsh "+
			"each answer `$'\\r': command not found` for it, at EVERY shell start, which is a "+
			"permanent error message in a file this command wrote.\n%q", updated)
	}
	if !strings.Contains(updated, "/new/bin") || strings.Contains(updated, "/old/bin") {
		t.Errorf("the replace did not swap the directory:\n%s", updated)
	}
	if !strings.HasPrefix(updated, crlf("# my own profile\nexport EDITOR=vi\n")) {
		t.Errorf("the user's own CRLF lines were not kept byte for byte:\n%q", updated)
	}

	// Behavioural: a real shell must source it without complaining.
	if runtime.GOOS == "windows" {
		return
	}
	sh, lerr := lookPathSh()
	if lerr != nil {
		t.Skipf("NOT A PASS: no `sh` on PATH, so the lone-CR line's runtime effect was not measured: %v", lerr)
	}
	f := filepath.Join(t.TempDir(), "profile")
	if werr := os.WriteFile(f, []byte(updated), 0o644); werr != nil {
		t.Fatal(werr)
	}
	out, cerr := runShell(t, sh, ". "+shellSingleQuote(f)+"; printf done")
	if cerr != nil || strings.Contains(out, "not found") {
		t.Errorf("sourcing the merged profile errored: %v\n%s", cerr, out)
	}
}

// ---------------------------------------------------------------------------
// R6 — every path in --json is absolute, including the startup files
// ---------------------------------------------------------------------------

// TestPathFixTargetsAreAbsoluteEvenWhenHOMEIsRelative is round 1's R6.
//
// `README.md` publishes "A path in `--json` is always **absolute**, whatever
// `--dir` you passed". The project rows earn that with one `filepath.Abs` in
// runAgentSetup; the startup-file rows joined `env.Home` raw, so a relative
// `HOME` produced relative rows AND writes into the process's working directory
// rather than a home directory. (An UNSET home is refused earlier and by name —
// that arm is correct and is not what this covers.)
func TestPathFixTargetsAreAbsoluteEvenWhenHOMEIsRelative(t *testing.T) {
	env := agentEnv{
		Home:   "junkhome",
		Dir:    t.TempDir(),
		GOOS:   runtime.GOOS,
		Exists: func(string) bool { return false },
	}
	targets, err := pathFixTargets(env)
	if err != nil {
		t.Fatalf("pathFixTargets with a relative HOME: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d target(s), want 2", len(targets))
	}
	for _, tgt := range targets {
		if !filepath.IsAbs(tgt.Path) {
			t.Errorf("target %q is not absolute. README.md states every path in `--json` is "+
				"absolute whatever `--dir` you passed, and a relative one also means the file is "+
				"created under the CLI's working directory instead of a home directory.", tgt.Path)
		}
	}
	// And an unset home is still refused, not turned into the cwd by the fix.
	if _, _, err := planPathFix(agentEnv{Home: "  ", Dir: t.TempDir(), GOOS: runtime.GOOS}); err == nil {
		t.Errorf("an empty HOME was accepted. Making a relative home absolute must not make an " +
			"ABSENT one resolve to the working directory")
	}
}
