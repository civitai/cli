package cmd

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// probeEnv builds an agentEnv from a variable map and a set of marker paths that
// "exist", so detection can be exercised without any of these agents being
// installed and without touching the process environment.
func probeEnv(vars map[string]string, dir string, present ...string) agentEnv {
	set := map[string]bool{}
	for _, p := range present {
		set[filepath.Join(dir, p)] = true
	}
	return agentEnv{
		Vars:   vars,
		Dir:    dir,
		Exists: func(path string) bool { return set[path] },
	}
}

// TestAgentDetectionEnvironmentTable drives every environment signal.
func TestAgentDetectionEnvironmentTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{"claude code", map[string]string{"CLAUDECODE": "1"}, agentClaude},
		{"claude code entrypoint", map[string]string{"CLAUDE_CODE_ENTRYPOINT": "cli"}, agentClaude},
		{"cursor", map[string]string{"CURSOR_TRACE_ID": "abc"}, agentCursor},
		{"codex", map[string]string{"CODEX_SANDBOX": "seatbelt"}, agentCodex},
		{"opencode", map[string]string{"OPENCODE": "1"}, agentOpencode},
		{"zed terminal", map[string]string{"ZED_TERM": "true"}, agentZed},
		{"zed term_program", map[string]string{"TERM_PROGRAM": "zed"}, agentZed},
		{"vscode", map[string]string{"TERM_PROGRAM": "vscode"}, agentVSCode},
		{"vscode askpass", map[string]string{"VSCODE_GIT_ASKPASS_MAIN": "/x"}, agentVSCode},
		{"windsurf", map[string]string{"TERM_PROGRAM": "windsurf"}, agentWindsurf},
		{"nothing at all", map[string]string{}, agentOther},
		// An EMPTY value is not a signal. Exported-but-empty is a real shape (a
		// shell that unsets by assigning ""), and reading it as "set" would
		// report an agent that is not running.
		{"empty value is not a signal", map[string]string{"CLAUDECODE": ""}, agentOther},
		// TERM_PROGRAM is spelled inconsistently across editors.
		{"case-insensitive value", map[string]string{"TERM_PROGRAM": "vsCode"}, agentVSCode},
		// An unrelated value on a value-matched variable must not fire.
		{"unmatched TERM_PROGRAM value", map[string]string{"TERM_PROGRAM": "iTerm.app"}, agentOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectAgent(probeEnv(tc.vars, t.TempDir())); got != tc.want {
				t.Errorf("detectAgent(%v) = %q, want %q", tc.vars, got, tc.want)
			}
		})
	}
}

// TestAgentDetectionPrefersTheForkOverVSCode is the ORDER guard, and it is the
// only part of detection that can be silently wrong.
//
// Cursor and Windsurf are VS Code forks: a Cursor session sets `TERM_PROGRAM=
// vscode` in addition to its own variable. A table walked in the wrong order
// therefore answers `vscode` for a Cursor user, writes `.vscode/mcp.json` under
// the key `servers`, exits 0, and Cursor never sees a server. Every row here
// carries BOTH the fork's signal and the inherited one, so a reordering that
// puts the generic row first fails by name.
func TestAgentDetectionPrefersTheForkOverVSCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		vars map[string]string
		want string
	}{
		{
			"cursor also sets TERM_PROGRAM=vscode",
			map[string]string{"CURSOR_TRACE_ID": "t", "TERM_PROGRAM": "vscode", "VSCODE_PID": "9"},
			agentCursor,
		},
		{
			"windsurf also sets the vscode variables",
			map[string]string{"TERM_PROGRAM": "windsurf", "VSCODE_PID": "9"},
			agentWindsurf,
		},
		{
			"claude code inside a vscode terminal is still claude code",
			map[string]string{"CLAUDECODE": "1", "TERM_PROGRAM": "vscode"},
			agentClaude,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// PREMISE: the inherited signal really is present, or this row is
			// testing an ordering hazard that does not exist in it.
			if tc.vars["TERM_PROGRAM"] == "" && tc.vars["VSCODE_PID"] == "" {
				t.Fatal("fixture carries no VS Code signal — it cannot observe the ordering")
			}
			if got := detectAgent(probeEnv(tc.vars, t.TempDir())); got != tc.want {
				t.Errorf("detectAgent = %q, want %q — a fork-specific signal must beat the VS Code "+
					"signal it inherits, or every fork is configured through the wrong file", got, tc.want)
			}
		})
	}
}

// TestAgentDetectionFallsBackToMarkerFiles pins the second tier: run from an
// ordinary shell, with no agent variables at all, the project's own files decide.
func TestAgentDetectionFallsBackToMarkerFiles(t *testing.T) {
	for _, tc := range []struct {
		name    string
		present []string
		want    string
	}{
		{"claude project dir", []string{".claude"}, agentClaude},
		{"claude project mcp file", []string{".mcp.json"}, agentClaude},
		{"cursor", []string{".cursor"}, agentCursor},
		{"opencode", []string{"opencode.json"}, agentOpencode},
		{"zed", []string{".zed"}, agentZed},
		{"vscode", []string{".vscode"}, agentVSCode},
		{"none", nil, agentOther},
		// 🔴 `.vscode/` is the most common of these by a wide margin and says
		// almost nothing. A Claude Code project with a `.vscode/` directory must
		// still resolve to claude.
		{"a diagnostic marker beats .vscode", []string{".claude", ".vscode"}, agentClaude},
		{"cursor beats .vscode", []string{".cursor", ".vscode"}, agentCursor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if got := detectAgent(probeEnv(map[string]string{}, dir, tc.present...)); got != tc.want {
				t.Errorf("detectAgent(markers %v) = %q, want %q", tc.present, got, tc.want)
			}
		})
	}
}

// TestAgentDetectionEnvironmentBeatsMarkers pins the TIER order. The environment
// describes the process running RIGHT NOW; a marker file describes what was used
// in this directory at some point. When they disagree, the live one wins.
func TestAgentDetectionEnvironmentBeatsMarkers(t *testing.T) {
	dir := t.TempDir()
	env := probeEnv(map[string]string{"CURSOR_TRACE_ID": "t"}, dir, ".claude", ".vscode")
	if got := detectAgent(env); got != agentCursor {
		t.Errorf("detectAgent = %q, want %q — an environment signal must beat a stale marker file", got, agentCursor)
	}
	// CONTROL: with the variable gone, the SAME markers resolve to claude — so
	// the assertion above is about precedence, not about a marker table that
	// never fires.
	if got := detectAgent(probeEnv(map[string]string{}, dir, ".claude", ".vscode")); got != agentClaude {
		t.Fatalf("control: markers alone resolve to %q, want %q — the fixture proves nothing", got, agentClaude)
	}
}

// TestAgentTargetTable is the §2 table, pinned literally.
//
// 🔴 EVERY ROW HERE IS A TRAP, AND EVERY ONE FAILS SILENTLY. A wrong top-level
// key or a wrong URL key produces a file that parses, an agent that starts
// cleanly, and a server that is simply not there. The expected values are
// written out rather than derived from agentTargets, so an edit to the table
// fails here instead of agreeing with itself.
func TestAgentTargetTable(t *testing.T) {
	for _, tc := range []struct {
		agent      string
		userScoped bool
		wantPath   string
		serversKey string
		urlKey     string
	}{
		{agentClaude, false, ".mcp.json", "mcpServers", "url"},
		{agentCursor, false, ".cursor/mcp.json", "mcpServers", "url"},
		// 🔴 `servers`, not `mcpServers`.
		{agentVSCode, false, ".vscode/mcp.json", "servers", "url"},
		{agentCodex, true, ".codex/config.toml", "mcp_servers", "url"},
		// 🔴 `mcp`.
		{agentOpencode, false, "opencode.json", "mcp", "url"},
		// 🔴 `serverUrl`, not `url`.
		{agentWindsurf, true, ".codeium/windsurf/mcp_config.json", "mcpServers", "serverUrl"},
		// 🔴 `context_servers`.
		{agentZed, true, ".config/zed/settings.json", "context_servers", "url"},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			target, ok := agentTargets[tc.agent]
			if !ok {
				t.Fatalf("no target registered for %q", tc.agent)
			}
			if target.ServersKey != tc.serversKey {
				t.Errorf("%s top-level key = %q, want %q — the wrong key writes a file that parses and "+
					"registers nothing", tc.agent, target.ServersKey, tc.serversKey)
			}
			if target.URLKey != tc.urlKey {
				t.Errorf("%s url key = %q, want %q", tc.agent, target.URLKey, tc.urlKey)
			}
			if target.UserScoped != tc.userScoped {
				t.Errorf("%s UserScoped = %t, want %t — a user-scoped file written under the project "+
					"directory (or the reverse) lands somewhere the agent never reads",
					tc.agent, target.UserScoped, tc.userScoped)
			}

			home, dir := t.TempDir(), t.TempDir()
			base := dir
			if tc.userScoped {
				base = home
			}
			env := agentEnv{Vars: map[string]string{}, Home: home, Dir: dir}
			got, ok := agentConfigPath(env, tc.agent)
			if !ok {
				t.Fatalf("agentConfigPath(%s) reported no path", tc.agent)
			}
			if want := filepath.Join(base, filepath.FromSlash(tc.wantPath)); got != want {
				t.Errorf("agentConfigPath(%s) = %q, want %q", tc.agent, got, want)
			}
		})
	}
}

// TestUserScopedTargetsNeedAHomeDirectory pins the second return of
// agentConfigPath. Collapsing it into a bare path would build a config path out
// of "" and write into the filesystem root — and report success.
func TestUserScopedTargetsNeedAHomeDirectory(t *testing.T) {
	env := agentEnv{Vars: map[string]string{}, Home: "", Dir: t.TempDir()}
	for _, agent := range []string{agentCodex, agentWindsurf, agentZed} {
		if path, ok := agentConfigPath(env, agent); ok {
			t.Errorf("%s resolved to %q with no home directory — that path is built from an empty string", agent, path)
		}
	}
	// CONTROL: a project-scoped target still resolves without a home directory.
	if _, ok := agentConfigPath(env, agentClaude); !ok {
		t.Error("claude is project-scoped and must resolve with no home directory")
	}
	// CONTROL: with a home directory the user-scoped ones DO resolve, so the
	// negatives above are about the home directory and not about a function that
	// always says no.
	env.Home = t.TempDir()
	for _, agent := range []string{agentCodex, agentWindsurf, agentZed} {
		if _, ok := agentConfigPath(env, agent); !ok {
			t.Errorf("%s did not resolve even with a home directory", agent)
		}
	}
}

// TestKnownAgentsIsTheTablePlusOther keeps the accepted `--agent` set derived
// from the target table, so a new target cannot be added without becoming
// accepted, and an accepted id cannot exist with nowhere to write.
func TestKnownAgentsIsTheTablePlusOther(t *testing.T) {
	got := knownAgents()
	want := []string{agentOther}
	for id := range agentTargets {
		want = append(want, id)
	}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("knownAgents() = %v, want %v", got, want)
	}
	if len(got) != 8 {
		t.Errorf("knownAgents() has %d entries (%v), want the 7 targets plus `other`", len(got), got)
	}
}

// TestValidateAgentFlagRefusesUnknown pins the exit-2 classification and the
// content of the refusal: a user who typed `--agent vs-code` needs the spelling.
func TestValidateAgentFlagRefusesUnknown(t *testing.T) {
	for _, in := range []string{"vs-code", "copilot", "Claude Code", ""} {
		_, err := validateAgentFlag(in)
		if err == nil {
			t.Errorf("validateAgentFlag(%q) accepted an unknown agent", in)
			continue
		}
		if !strings.Contains(err.Error(), "windsurf") || !strings.Contains(err.Error(), "other") {
			t.Errorf("the refusal for %q does not list the accepted ids: %v", in, err)
		}
	}
	// CONTROL: every accepted id really is accepted, and case/whitespace are
	// forgiven — an agent piping a value in should not be refused over padding.
	for _, id := range knownAgents() {
		if got, err := validateAgentFlag("  " + strings.ToUpper(id) + " "); err != nil || got != id {
			t.Errorf("validateAgentFlag(%q) = (%q, %v), want (%q, nil)", id, got, err, id)
		}
	}
}
