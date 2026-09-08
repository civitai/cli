package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The round-3 audit guards for `civitai agent-setup`.
//
// 🔴 THREE KINDS OF TEST LIVE HERE AND EACH ONE SAYS WHICH IT IS, because a
// blanket "every test below was watched red" is how the round-2 file came to
// claim ten regression guards over seven. Count them from the labels, not from
// the file:
//
//   - REGRESSION (watched red on 897c1cc, green after): all six subtests of
//     TestJSONNeverExitsSilently — four measured by hand as recorded at its
//     docstring, and all six replayed by copying this file into a worktree at
//     897c1cc — plus TestAPreservedHeaderIsNotReportedAsAuthenticating and
//     TestACodexEnvVarReferenceIsNotCalledAHeader.
//   - MUTATION GUARD (897c1cc is not the mutant; it passes there by construction):
//     TestAFirstWriteFailureDoesNotStopTheLaterOnes. Its red observation is
//     against a named source mutation, recorded at its docstring.
//   - OVER-WIDENING CONTROL, i.e. an invariant guard: TestAUsageErrorStillEmitsNoPayload
//     and TestAnAbsentHeaderStillGetsThe401Row. Both pass on 897c1cc. They pin the
//     direction each fix could over-shoot in; they are NOT regression coverage.
//
// 🔴 THEY ARE BLACK-BOX ON PURPOSE, for the reason the round-2 file gives: a
// guard that references a symbol the fix introduced cannot be watched red, it
// fails to compile, which is not the same observation.

// ---------------------------------------------------------------------------
// F4 — the three writes are independent, and only a FIRST failure can show it
// ---------------------------------------------------------------------------

// TestAFirstWriteFailureDoesNotStopTheLaterOnes is a MUTATION GUARD, not a
// regression test: 897c1cc already has the independent-writes behaviour, so this
// test passes there. What it was watched red against is the named source
// mutation below — the shape the behaviour is one edit away from losing, and
// which the whole package failed to notice.
//
// 🔴 MEASURED: inserting `if writeFailed { return }` at the top of
// runAgentSetupWrite's `attempt` — i.e. restoring abort-on-first-failure —
// SURVIVED `go test ./internal/cmd` at 897c1cc. `assertBlockedWritePayload`
// carried the right assertion and all three of its fixtures failed the LAST
// write, so an abort-on-first mutant never skipped anything. Built and run
// against a broken `AGENTS.md`, that mutant emitted `blocked` / `create` /
// `create` while only the first file had been touched — a payload claiming
// `create` for two files nothing attempted.
//
// The fixture therefore fails the FIRST write, at WRITE time rather than at plan
// time: `AGENTS.md` is a symlink whose target EXISTS (so the destination check
// resolves and the plan succeeds) in a directory that is not writable (so the
// atomic write's temp file cannot be created).
func TestAFirstWriteFailureDoesNotStopTheLaterOnes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — a 0500 directory is still writable")
	}
	dir, home := agentSetupProject(t)
	readOnly := filepath.Join(home, "dotfiles")
	target := filepath.Join(readOnly, agentsFilename)
	writeFile(t, target, "# theirs\n")
	if err := os.Symlink(target, filepath.Join(dir, agentsFilename)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	// PREMISE: the failure really is at WRITE time. A plan-time refusal would
	// exercise a different branch and the mutant would survive this fixture too.
	dryOut, _, _ := run(t, "agent-setup", "--dry-run", "--json", "--dir", dir, "--agent", agentCodex)
	var dry agentSetupJSON
	if err := json.Unmarshal([]byte(dryOut), &dry); err != nil {
		t.Fatalf("bad --dry-run json: %v\n%q", err, dryOut)
	}
	if len(dry.Changes) == 0 || dry.Changes[0].Action == actionBlocked {
		t.Fatalf("PREMISE BROKEN: the plan already refused %s, so this fixture does not exercise a "+
			"write-time failure:\n%s", agentsFilename, dryOut)
	}

	// 🔴 THE DIRECT RUN COMES FIRST, AND THE ORDER IS THE ASSERTION. This block
	// used to sit AFTER assertBlockedWritePayload, which performs a complete
	// `agent-setup` against this same dir/home — so the closing `os.Stat(mcpPath)`
	// passed on THAT run's artefact no matter what the second run did, while the
	// comment beside it claimed it was catching the write an abort-on-first mutant
	// never reaches. A guard whose subject was already created by an earlier run
	// is not observing the run it names.
	mcpPath, ok := agentConfigPath(liveAgentEnv(dir), agentCodex)
	if !ok {
		t.Fatal("no codex config path")
	}
	for _, p := range []string{mcpPath, filepath.Join(dir, claudeFilename)} {
		if _, statErr := os.Lstat(p); statErr == nil {
			t.Fatalf("PREMISE BROKEN: %s exists before the run under test, so its existence afterwards "+
				"proves nothing", p)
		}
	}

	out, _, err := run(t, "agent-setup", "--json", "--dir", dir, "--agent", agentCodex)
	if err == nil {
		t.Fatal("a run whose first write failed exited 0")
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("bad json: %v\n%q", jsonErr, out)
	}
	if payload.Changes[0].Action != actionBlocked {
		t.Fatalf("PREMISE BROKEN: the FIRST row is not the blocked one, so an abort-on-first mutant "+
			"would skip nothing:\n%s", out)
	}
	for _, c := range payload.Changes[1:] {
		if c.Action == actionBlocked {
			t.Errorf("%s was blocked too, so this fixture cannot distinguish independence from an "+
				"abort: %s", c.Path, c.Reason)
		}
	}
	// Both later writes must have HAPPENED, in this run: CLAUDE.md is the second
	// and the MCP config is the last, the one an abort-on-first mutant never
	// reaches at all.
	if _, statErr := os.Stat(filepath.Join(dir, claudeFilename)); statErr != nil {
		t.Errorf("%s was never written, though its row claims %q: %v",
			claudeFilename, payload.Changes[1].Action, statErr)
	}
	if _, statErr := os.Stat(mcpPath); statErr != nil {
		t.Errorf("the MCP config was never written, though its row claims %q: %v",
			payload.Changes[len(payload.Changes)-1].Action, statErr)
	}

	// And the shared payload contract on a SECOND run over the same tree: the
	// first write still fails, the rows still describe what happened.
	assertBlockedWritePayload(t, dir, agentCodex, "permission denied", claudeFilename)
}

// ---------------------------------------------------------------------------
// F3 — `--json` has THREE shapes and no fourth, silent one
// ---------------------------------------------------------------------------

// TestJSONNeverExitsSilently is the SHAPE CLASS rather than a fifth instance.
//
// 🔴 MEASURED RED at 897c1cc, all rc 1 with ZERO BYTES on stdout: an `AGENTS.md`
// that is a directory, under `--json`, under `--check --json` and under
// `--dry-run --json`; and a run with neither $HOME nor $XDG_CONFIG_HOME. Round 2
// closed four enumerated instances and asserted an absolute — "never as an empty
// stdout" — that three of these falsify, INCLUDING `--check --json`, the exact
// surface `developer.civitai.com`'s hosted prompt reads.
//
// Every case below is one where the command reaches a RUN. Exit 2 — a mistake
// about the invocation — is the stated exception and has its own test.
func TestJSONNeverExitsSilently(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setUp func(t *testing.T, dir, home string)
		args  []string
	}{
		{"a write run whose AGENTS.md cannot be read", mkdirOverAgentsMD, nil},
		{"--check whose AGENTS.md cannot be read", mkdirOverAgentsMD, []string{"--check"}},
		{"--dry-run whose AGENTS.md cannot be read", mkdirOverAgentsMD, []string{"--dry-run"}},
		// A SECOND plan-time refusal, reached by an ordinary copy-paste rather
		// than by a permission trick: the merge refuses a file carrying two
		// managed blocks, and that refusal took the same silent exit.
		{"a write run over a duplicated managed block", duplicateManagedBlock, nil},
		{"--dry-run over a duplicated managed block", duplicateManagedBlock, []string{"--dry-run"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			tc.setUp(t, dir, home)
			args := append([]string{"agent-setup", "--json", "--dir", dir, "--agent", agentClaude}, tc.args...)
			out, _, _ := run(t, args...)
			var payload agentSetupJSON
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatalf("--json emitted no readable payload (%v):\n%q", err, out)
			}
			if payload.Agent == "" || payload.Track == "" {
				t.Errorf("the payload names neither the run's agent nor its track:\n%s", out)
			}
		})
	}

	// The fourth measured shape: nothing to build a payload FROM, which is what
	// the third shape (`error`, no arrays) exists for. `--dir` is passed so the
	// exit-2 path-validation arm is not what answers.
	t.Run("no resolvable config root", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		t.Setenv("HOME", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		out, _, err := run(t, "agent-setup", "--json", "--dir", dir, "--agent", agentCodex)
		if err == nil {
			t.Skip("this platform resolves a config root without $HOME — the silent-exit case is unreachable")
		}
		// Decoded into a bare map on purpose: naming a Go field the fix introduced
		// is what stops a guard being watchable red on the pre-fix tree.
		var payload map[string]any
		if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
			t.Fatalf("--json emitted no readable payload (%v):\n%q", jsonErr, out)
		}
		if ok, _ := payload["ok"].(bool); ok {
			t.Error("ok = true on a run that could not start")
		}
		if reason, _ := payload["error"].(string); strings.TrimSpace(reason) == "" {
			t.Errorf("the envelope carries no reason:\n%s", out)
		}
		if _, has := payload["checks"]; has {
			t.Errorf("the envelope carries `checks`, so a consumer cannot discriminate on it:\n%s", out)
		}
		if _, has := payload["changes"]; has {
			t.Errorf("the envelope carries `changes`, so a consumer cannot discriminate on it:\n%s", out)
		}
	})
}

func mkdirOverAgentsMD(t *testing.T, dir, _ string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, agentsFilename), 0o755); err != nil {
		t.Fatal(err)
	}
}

func duplicateManagedBlock(t *testing.T, dir, _ string) {
	t.Helper()
	one := agentsBeginMarker + "\nmine\n" + agentsEndMarker + "\n"
	writeFile(t, filepath.Join(dir, agentsFilename), one+"\n"+one)
}

// TestAUsageErrorStillEmitsNoPayload is the OVER-WIDENING CONTROL for the guard
// above, and it is an INVARIANT GUARD, not a regression test: it passes on
// 897c1cc. It exists because "close the shape class" is satisfiable by emitting
// a payload for everything, including a command line that never named a run —
// and the exit-2 arms are the documented exception, so widening into them would
// be a silent contract change rather than a fix.
//
// 🔴 IT ASSERTS THE CLASS, NOT JUST "IT FAILED AND SAID NOTHING". The first draft
// checked only `err != nil` plus an empty stdout — which is satisfied by exactly
// the defect the guard above exists to prevent: an input that stopped being a
// usage error and became an ordinary runtime failure with zero bytes on stdout
// would pass here and be certified correct. `errors.Is(err, ErrUsage)` is what
// makes "silence is allowed HERE" a claim about the exit-2 CLASS rather than
// about these three particular strings.
func TestAUsageErrorStillEmitsNoPayload(t *testing.T) {
	dir, _ := agentSetupProject(t)
	for _, args := range [][]string{
		{"agent-setup", "--json", "--dir", dir, "--agent", "notanagent"},
		{"agent-setup", "--json", "--dir", filepath.Join(dir, "nope"), "--agent", agentClaude},
		{"agent-setup", "--json", "--dir", dir, "--track", "api"},
	} {
		out, _, err := run(t, args...)
		if err == nil {
			t.Errorf("%v exited 0", args)
			continue
		}
		if !errors.Is(err, ErrUsage) {
			t.Errorf("%v is not a usage error (%v), so an empty stdout here is the silent-exit defect "+
				"rather than the documented exception", args, err)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("%v put a payload on stdout for a usage error:\n%s", args, out)
		}
	}
}

// ---------------------------------------------------------------------------
// F2 — a preserved value is PRESENT, not proven to authenticate
// ---------------------------------------------------------------------------

// TestAPreservedHeaderIsNotReportedAsAuthenticating.
//
// 🔴 MEASURED RED at 897c1cc, and the case is this CLI's own next-step text: the
// Zed branch tells the user to paste `"headers": {"Authorization": "Bearer <your
// token>"}`. With that literal on disk the run said the entry "already carries an
// Authorization header … and that is what they authenticate with" AND dropped the
// `needs an Authorization header — this one returns 401 without a credential`
// row that c801ab8 printed. `mcpAuthCoverageOf` treated any non-empty string as a
// credential. Items 28 and 34: the command does not claim what it cannot observe.
func TestAPreservedHeaderIsNotReportedAsAuthenticating(t *testing.T) {
	dir, home := agentSetupProject(t)
	settings := filepath.Join(home, ".config", "zed", "settings.json")
	entries := map[string]any{}
	for _, srv := range civitaiMCPServers {
		entries[srv.Name] = map[string]any{
			"url": srv.URL,
			// Byte-for-byte what printAgentSetupAuthNote tells a Zed user to paste.
			"headers": map[string]any{"Authorization": "Bearer <your token>"},
		}
	}
	body, err := json.Marshal(map[string]any{"context_servers": entries})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, settings, string(body))

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	// PREMISE: the placeholder is still there, so this is the preserved case.
	// Read back through a decoder — `<` and `>` come out of encoding/json as
	// `<` / `>`, so a byte-wise Contains would fail on a file that is
	// in fact correct.
	var merged map[string]any
	if jsonErr := json.Unmarshal([]byte(readFile(t, settings)), &merged); jsonErr != nil {
		t.Fatalf("merged settings.json is not valid JSON: %v", jsonErr)
	}
	section, _ := merged["context_servers"].(map[string]any)
	for _, srv := range civitaiMCPServers {
		entry, _ := section[srv.Name].(map[string]any)
		headers, _ := entry["headers"].(map[string]any)
		if got, _ := headers["Authorization"].(string); got != "Bearer <your token>" {
			t.Fatalf("PREMISE BROKEN: %s's placeholder was not preserved (%q):\n%s",
				srv.Name, got, readFile(t, settings))
		}
	}
	if strings.Contains(out, "that is what they authenticate with") {
		t.Errorf("the run asserts a placeholder authenticates:\n%s", out)
	}
	if !strings.Contains(out, "cannot tell whether it resolves") {
		t.Errorf("the run never says it cannot evaluate the value it found:\n%s", out)
	}
	// 🔴 THE 401 ROW IS NOT SUPPRESSED BY A STRING THE COMMAND CANNOT EVALUATE.
	if !strings.Contains(out, "returns 401 unless that value resolves to a credential") {
		t.Errorf("the orchestration server's 401 warning was dropped for an unevaluable header:\n%s", out)
	}
	// 🔴 AND THE RUN DOES NOT CONTRADICT ITSELF. Zed's static caveat used to say
	// "the entries carry no Authorization header" in the same output that had just
	// reported one.
	if strings.Contains(out, "the entries carry no Authorization header") {
		t.Errorf("the caveat denies the header the same run just reported:\n%s", out)
	}
}

// TestACodexEnvVarReferenceIsNotCalledAHeader.
//
// 🔴 MEASURED RED at 897c1cc: a leftover `bearer_token_env_var = "CIVITAI_TOKEN"`
// in `~/.codex/config.toml`, run with CIVITAI_TOKEN unset, produced "every
// Civitai entry already carries an Authorization header … and that is what they
// authenticate with" — from the `!hasToken` branch, which is reached ONLY when
// that variable is unset in this process. Two false claims in one sentence: the
// file holds no header (Codex builds one from the variable NAME), and the
// reference resolves to nothing.
func TestACodexEnvVarReferenceIsNotCalledAHeader(t *testing.T) {
	dir, home := agentSetupProject(t)
	cfg := filepath.Join(home, ".codex", "config.toml")
	var b strings.Builder
	for _, srv := range civitaiMCPServers {
		b.WriteString("[mcp_servers.\"" + srv.Name + "\"]\n")
		b.WriteString("url = \"" + srv.URL + "\"\n")
		b.WriteString("bearer_token_env_var = \"" + tokenEnvVar + "\"\n\n")
	}
	writeFile(t, cfg, b.String())
	t.Setenv("CIVITAI_TOKEN", "")

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	// PREMISE: the reference survived the merge, so this is the preserved case.
	if !strings.Contains(readFile(t, cfg), "bearer_token_env_var") {
		t.Fatalf("PREMISE BROKEN: the reference was dropped:\n%s", readFile(t, cfg))
	}
	if strings.Contains(out, "that is what they authenticate with") {
		t.Errorf("the run asserts an unset variable authenticates:\n%s", out)
	}
	if strings.Contains(out, "already carries an Authorization header") {
		t.Errorf("the run calls `bearer_token_env_var` an Authorization header; that file has none:\n%s", out)
	}
	if !strings.Contains(out, "bearer_token_env_var") {
		t.Errorf("the run never names what it actually found:\n%s", out)
	}
	// The variable really is unset, and the run says so rather than the opposite.
	if !strings.Contains(out, "NOT set") {
		t.Errorf("the run does not say %s is unset while describing a reference to it:\n%s", tokenEnvVar, out)
	}
}

// TestAnAbsentHeaderStillGetsThe401Row is the OVER-WIDENING CONTROL for the two
// guards above, and it is an INVARIANT GUARD, not a regression test: it passes on
// 897c1cc. Un-suppressing the 401 warning is satisfiable by never suppressing
// anything, and it is equally satisfiable by dropping the row altogether; this
// pins the genuinely header-less case, which is the one item 34 exists for.
func TestAnAbsentHeaderStillGetsThe401Row(t *testing.T) {
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if strings.Contains(strings.ToLower(readFile(t, filepath.Join(dir, ".mcp.json"))), "authorization") {
		t.Fatal("PREMISE BROKEN: a header was written on a run with no token")
	}
	if !strings.Contains(out, "needs an Authorization header — this one returns 401 without a credential") {
		t.Errorf("a genuinely header-less entry lost its 401 row:\n%s", out)
	}
}
