package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// R-F2/F1 — `--fix-path` REFUSES Windows, and the refusal is BEHAVIOURAL
// ---------------------------------------------------------------------------
//
// 🔴 WHY THIS FILE EXISTS AT ALL: THE GUARD IT REPLACES PINNED A SPELLING. Round
// 2 banned a `runtime.GOOS != "windows"` gate with a source scan asserting
// `strings.Count(code, "runtime.GOOS") == 1`. A gate written `env.GOOS ==
// "windows"` was then measured to survive that scan AND the whole module, while
// the `runtime.GOOS` spelling was caught — so the guard was a claim about a word,
// not about the gate. Counting a second spelling cannot repair that; the only
// repair is to assert what the command DOES. Everything here is behavioural.
//
// 🔴 AND THE REFUSAL IS ONLY TESTABLE BECAUSE IT READS THE INJECTED SEAM.
// pathFixWindowsRefused keys on `agentEnv.GOOS`, which this package injects
// everywhere for exactly this reason, so `windows` is reachable from a linux
// host. Keyed on `runtime.GOOS` the gate would be unreachable here and could only
// be asserted by reading the source for a spelling again.
//
// 🔴 NOTHING IN THIS FILE REFERENCES A SYMBOL THE CHANGE INTRODUCES, DELIBERATELY
// — the same rule the round-2 file states. The assertions are on the command's
// observable behaviour and on substrings of the user-facing reason, so the file
// COMPILES at the pre-change tree and the matrix below is a test shown to fail
// rather than a build error.
//
// MEASURED RED AT `eae4935` (the pre-change tree), and the base does NOT merely
// refuse for the wrong reason — on a POSIX host it does not refuse at all.
// Neither of the two refusals that exist there can see an injected `GOOS`:
// cliBinaryBaseName reads `runtime.GOOS` (so the fixture binary is `civitai`, not
// `civitai.exe`, and the base-name check passes) and the colon predicate is
// unconditional (and a linux temp dir holds no colon). So at the base the
// `windows` arm plans, writes BOTH startup files, and reports `ok:true` at exit
// 0. The native-Windows build is the case that reaches the colon refusal with an
// unactionable reason; that case remains UNVERIFIED for want of a Windows host
// and is derived in agent_setup_fixpath.go's posture note, not asserted here.

// fixPathStartupFileNames is every startup file a reader might reasonably fear
// this command touched. It is deliberately WIDER than the two rows pathFixTargets
// builds: the claim under test is "no startup file was written", and a guard that
// only looks at the two files the implementation names cannot see a third one
// being added.
var fixPathStartupFileNames = []string{
	".zshenv", ".zprofile", ".zshrc", ".zlogin",
	".bash_profile", ".bash_login", ".profile", ".bashrc",
}

// fixPathWriteRun drives the write path with an INJECTED GOOS and returns the
// `--json` payload, the raw stdout and the error the command would exit on.
//
// It calls runAgentSetupWrite rather than the Cobra command because that is the
// only place `agentEnv` can be supplied: the command builds its own from
// liveAgentEnv, whose GOOS is this process's. `--json` is used so the assertions
// read the same rows a scripted consumer does.
func fixPathWriteRun(t *testing.T, goos, home, project string) (agentSetupJSON, string, error) {
	t.Helper()
	var buf bytes.Buffer
	emit := &agentSetupEmitter{w: &buf, json: true}
	env := agentEnv{Vars: map[string]string{}, Home: home, Dir: project, GOOS: goos}
	err := runAgentSetupWrite(emit, env, trackApp, agentClaude, "", false, true)
	var payload agentSetupJSON
	if buf.Len() > 0 {
		if jerr := json.Unmarshal(buf.Bytes(), &payload); jerr != nil {
			t.Fatalf("decoding the --json payload for GOOS=%s: %v\n%s", goos, jerr, buf.String())
		}
	}
	return payload, buf.String(), err
}

// assertNoStartupFileUnder fails naming the file, because "a startup file was
// written" is only actionable if you know which one.
func assertNoStartupFileUnder(t *testing.T, home, why string) {
	t.Helper()
	for _, name := range fixPathStartupFileNames {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			body, _ := os.ReadFile(p)
			t.Errorf("%s, yet %s exists:\n%s", why, p, body)
		}
	}
}

// TestFixPathRefusesWindowsAndWritesNoStartupFile is round 3's regression guard
// for F2, and the behavioural replacement for round 2's deleted source scan.
//
// The matrix is the whole point: `windows` must refuse with the named reason and
// write nothing, and `linux`/`darwin` must proceed and write both files. A guard
// on the refusal alone would be satisfied by a command that refused everywhere.
func TestFixPathRefusesWindowsAndWritesNoStartupFile(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			project, _ := agentSetupProject(t)
			fakeCLIBinDir(t)
			home := t.TempDir()

			payload, out, err := fixPathWriteRun(t, goos, home, project)

			if goos != "windows" {
				// ---- the POSIX arms PROCEED. Without this half the refusal
				// assertion below is satisfied by a command refusing everywhere.
				if err != nil {
					t.Fatalf("GOOS=%s: `--fix-path` failed: %v\n%s", goos, err, out)
				}
				if !payload.OK {
					t.Errorf("GOOS=%s: ok:false on a run that should have written both startup "+
						"files\n%s", goos, out)
				}
				for _, name := range []string{".zshenv", ".profile"} {
					p := filepath.Join(home, name)
					body, rerr := os.ReadFile(p)
					if rerr != nil {
						t.Errorf("GOOS=%s: %s was not written: %v", goos, p, rerr)
						continue
					}
					for _, marker := range []string{pathFixBeginMarker, pathFixEndMarker} {
						if !strings.Contains(string(body), marker) {
							t.Errorf("GOOS=%s: %s holds no %q:\n%s", goos, p, marker, body)
						}
					}
				}
				return
			}

			// ---- the windows arm REFUSES, in all four channels at once.
			if err == nil {
				t.Fatalf("GOOS=windows: `--fix-path` returned no error. This command writes POSIX "+
					"shell startup files; on Windows there is no such file to write, so a run that "+
					"exits 0 tells a script the PATH was fixed when nothing happened.\n%s", out)
			}
			if !errors.Is(err, ErrAgentSetupIncomplete) {
				t.Errorf("GOOS=windows: the refusal does not carry ErrAgentSetupIncomplete, so it "+
					"would not get the documented non-zero exit: %v", err)
			}
			if payload.OK {
				t.Errorf("GOOS=windows: ok:true beside a refusal — `ok` and the exit code read the "+
					"same rows, so this is unreachable unless the row is missing\n%s", out)
			}

			var blocked []agentChangeJSON
			for _, c := range payload.Changes {
				if c.Action == actionBlocked {
					blocked = append(blocked, c)
				}
			}
			if len(blocked) != 1 {
				t.Fatalf("GOOS=windows: %d blocked row(s), want exactly 1 (the platform refusal, "+
					"with an empty path, the convention the unresolvable-HOME refusal already uses)"+
					"\n%s", len(blocked), out)
			}
			if blocked[0].Path != "" {
				t.Errorf("GOOS=windows: the refusal row names the file %q. There is no file: the "+
					"refusal is about the platform, and an empty path is what says so.", blocked[0].Path)
			}
			// The REASON, not merely the refusal. A refusal whose reason does not
			// name the platform sends a Windows user looking for a directory to move.
			reason := strings.ToLower(blocked[0].Reason)
			for _, want := range []string{"windows", "posix"} {
				if !strings.Contains(reason, want) {
					t.Errorf("GOOS=windows: the refusal reason does not contain %q, so it does not "+
						"tell the user which of the two facts stopped the run — that this is Windows, "+
						"and that the only startup files this command knows are POSIX ones. Reason: %s",
						want, blocked[0].Reason)
				}
			}

			assertNoStartupFileUnder(t, home,
				"GOOS=windows refused `--fix-path`, so no startup file may have been written")

			// The other three rows are the ordinary project files and must be
			// untouched by the refusal — same count, same order, none blocked.
			if len(payload.Changes) != 4 {
				t.Fatalf("GOOS=windows: %d change row(s), want 4 — the 3 project files every run "+
					"reports plus the one platform refusal\n%s", len(payload.Changes), out)
			}
			for i, c := range payload.Changes[:3] {
				if c.Action == actionBlocked {
					t.Errorf("GOOS=windows: project row %d (%s) is blocked (%s). The Windows refusal "+
						"is about the user's shell startup files; it has nothing to do with the files "+
						"this command writes INTO THE PROJECT, and degrading those would be a second "+
						"defect wearing the first one's reason.", i, c.Path, c.Reason)
				}
				if _, serr := os.Stat(c.Path); serr != nil {
					t.Errorf("GOOS=windows: row %d reports %q as %q, but the file is not there: %v",
						i, c.Path, c.Action, serr)
				}
			}
		})
	}
}

// TestTheWindowsRefusalWinsOverTheHomeRefusal pins the ORDER, because the order
// is a claim this change wrote into prose in two places and a comment is a claim
// like any other.
//
// 🔴 A WINDOWS BOX WITH NO RESOLVABLE HOME HAS TWO TRUE REFUSALS AND ONLY ONE IS
// ACTIONABLE. "Set HOME and re-run" invites a second run that then refuses for a
// different reason; the platform answer ends the matter in one.
func TestTheWindowsRefusalWinsOverTheHomeRefusal(t *testing.T) {
	_, _, err := planPathFix(agentEnv{Home: "   ", Dir: t.TempDir(), GOOS: "windows"})
	if err == nil {
		t.Fatal("planPathFix accepted GOOS=windows with an unresolvable home")
	}
	got := strings.ToLower(err.Error())
	if !strings.Contains(got, "windows") {
		t.Errorf("the refusal does not name the platform: %v", err)
	}
	if strings.Contains(got, "set home") {
		t.Errorf("GOOS=windows with an empty HOME refused for the HOME reason, not the platform "+
			"one. Both are true, but only one is actionable: setting HOME would produce a second "+
			"run that refuses again, for Windows. Reason: %v", err)
	}
}

// TestTheWindowsRefusalLeavesTheProjectFilesByteIdentical is the other half of
// "the project files land exactly as they do without the flag", and it is a
// different claim from the row assertions above: a row saying `create` and a file
// holding the right bytes are not the same fact.
//
// It compares BYTES across two runs that differ only in the flag and the platform,
// which is the only form of that claim a reader cannot talk themselves out of.
func TestTheWindowsRefusalLeavesTheProjectFilesByteIdentical(t *testing.T) {
	read := func(dir string) map[string][]byte {
		t.Helper()
		got := map[string][]byte{}
		for _, name := range []string{"AGENTS.md", "CLAUDE.md", ".mcp.json"} {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("%s/%s: %v", dir, name, err)
			}
			got[name] = b
		}
		return got
	}

	// Arm 1: no flag at all, on this host's own GOOS — the baseline shape.
	plain, _ := agentSetupProject(t)
	fakeCLIBinDir(t)
	var buf bytes.Buffer
	plainEnv := agentEnv{Vars: map[string]string{}, Home: t.TempDir(), Dir: plain, GOOS: "linux"}
	if err := runAgentSetupWrite(&agentSetupEmitter{w: &buf, json: true},
		plainEnv, trackApp, agentClaude, "", false, false); err != nil {
		t.Fatalf("the no-flag run failed: %v\n%s", err, buf.String())
	}

	// Arm 2: `--fix-path` on windows, which must refuse the startup files and
	// nothing else.
	refused, _ := agentSetupProject(t)
	fakeCLIBinDir(t)
	home := t.TempDir()
	if _, out, err := fixPathWriteRun(t, "windows", home, refused); err == nil {
		t.Fatalf("GOOS=windows accepted `--fix-path`\n%s", out)
	}

	want, got := read(plain), read(refused)
	for name := range want {
		if !bytes.Equal(want[name], got[name]) {
			t.Errorf("%s differs between a no-flag run and a Windows-refused `--fix-path` run.\n"+
				"The refusal must cost the project nothing.\n--- no flag ---\n%s\n--- refused ---\n%s",
				name, want[name], got[name])
		}
	}
	assertNoStartupFileUnder(t, home, "the Windows arm refused")
}
