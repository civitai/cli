package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// agentSetupProject builds a hermetic project + home for one `agent-setup` run.
//
// 🔴 IT STRIPS EVERY AGENT SIGNAL FROM THE ENVIRONMENT, AND THAT IS LOAD-BEARING
// RATHER THAN TIDY. These tests are themselves usually run from inside one of the
// agents this command detects — a Claude Code session exports `CLAUDECODE=1` —
// so a test that did not clear them would exercise whatever agent the developer
// happened to be using and answer differently on CI. Cleared, `--agent` and the
// marker files are the only inputs, which is what makes the rows below mean
// anything.
func agentSetupProject(t *testing.T) (dir string, home string) {
	t.Helper()
	dir, home = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	for _, s := range agentEnvSignals {
		t.Setenv(s.Var, "")
	}
	return dir, home
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// AGENTS.md — the three cases
// ---------------------------------------------------------------------------

// TestAgentSetupWritesAGENTSMDWhenAbsent is case 1.
func TestAgentSetupWritesAGENTSMDWhenAbsent(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	got := readFile(t, filepath.Join(dir, agentsFilename))
	if !strings.Contains(got, agentsBeginMarker) || !strings.Contains(got, agentsEndMarker) {
		t.Fatalf("the written AGENTS.md carries no managed block:\n%s", got)
	}
	if !strings.Contains(got, "civitai app dev-tunnel") {
		t.Errorf("the template's own content did not reach the file:\n%s", got)
	}
}

// TestAgentSetupAppendsToExistingAGENTSMD is case 2, and the assertion is that
// EVERY EXISTING BYTE SURVIVES — not merely that the block arrived.
//
// This is the case that can destroy work: an author's AGENTS.md holds their
// build commands and their warnings, and no version of this CLI can give those
// back. So the guard is a prefix comparison against the original bytes, which a
// rewrite-the-file implementation fails even when it re-emits equivalent prose.
func TestAgentSetupAppendsToExistingAGENTSMD(t *testing.T) {
	dir, _ := agentSetupProject(t)
	const original = "# My project\n\nRun `make everything` before committing.\n\n## Conventions\n\n- tabs\n"
	path := filepath.Join(dir, agentsFilename)
	writeFile(t, path, original)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	got := readFile(t, path)
	if !strings.HasPrefix(got, strings.TrimRight(original, "\n")) {
		t.Errorf("the author's existing AGENTS.md was not preserved verbatim at the head of the file.\n"+
			"--- want prefix ---\n%s\n--- got ---\n%s", original, got)
	}
	if !strings.Contains(got, agentsBeginMarker) {
		t.Errorf("the managed block was not appended:\n%s", got)
	}
	if strings.Count(got, agentsBeginMarker) != 1 {
		t.Errorf("the managed block appears %d times, want 1", strings.Count(got, agentsBeginMarker))
	}
	if !strings.Contains(got, "make everything") {
		t.Errorf("the author's own instruction is gone:\n%s", got)
	}
}

// TestAgentSetupReplacesOnlyTheManagedBlock is case 3. The bytes OUTSIDE the
// markers must be identical, including the whitespace: a normalising rewrite of
// someone's document is a change they did not ask for and cannot review.
func TestAgentSetupReplacesOnlyTheManagedBlock(t *testing.T) {
	dir, _ := agentSetupProject(t)
	const head = "# Mine\n\nKeep   this    spacing.\n\n"
	const tail = "\n\n## After\n\nAlso mine.\n"
	path := filepath.Join(dir, agentsFilename)
	writeFile(t, path, head+agentsBeginMarker+"\nSTALE CONTENT THAT MUST GO\n"+agentsEndMarker+tail)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "STALE CONTENT") {
		t.Error("the managed block's old contents survived a refresh")
	}
	if !strings.HasPrefix(got, head) {
		t.Errorf("the bytes before the block changed:\n%q", got)
	}
	if !strings.HasSuffix(got, tail) {
		t.Errorf("the bytes after the block changed:\n%q", got)
	}
	if strings.Count(got, agentsBeginMarker) != 1 || strings.Count(got, agentsEndMarker) != 1 {
		t.Errorf("a second managed block was appended instead of the first being replaced:\n%s", got)
	}
}

// TestAgentSetupRefusesHalfAManagedBlock: one marker without the other means the
// file was hand-edited across the boundary. Appending would produce a nested,
// self-overlapping block that every later run gets more wrong, so it refuses —
// and the file is left exactly as it was.
func TestAgentSetupRefusesHalfAManagedBlock(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"BEGIN without END", "# Mine\n\n" + agentsBeginMarker + "\nsomething\n"},
		{"END without BEGIN", "# Mine\n\nsomething\n" + agentsEndMarker + "\n"},
		{"END before BEGIN", agentsEndMarker + "\nmiddle\n" + agentsBeginMarker + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			path := filepath.Join(dir, agentsFilename)
			writeFile(t, path, tc.body)

			_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude")
			if err == nil {
				t.Fatal("expected a refusal for a half-written managed block")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the refusal does not name the file: %v", err)
			}
			if got := readFile(t, path); got != tc.body {
				t.Errorf("the file was modified by a run that refused:\n%q", got)
			}
			// A verdict about the file, not a mistake about the invocation.
			if errors.Is(err, ErrUsage) {
				t.Error("a half-written managed block is not a usage error (exit 2)")
			}
		})
	}
}

// TestAgentSetupIsIdempotent: running twice must leave the same bytes, or every
// re-run grows the file.
func TestAgentSetupIsIdempotent(t *testing.T) {
	dir, _ := agentSetupProject(t)
	for i := 0; i < 2; i++ {
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	agents := readFile(t, filepath.Join(dir, agentsFilename))
	if n := strings.Count(agents, agentsBeginMarker); n != 1 {
		t.Errorf("AGENTS.md carries %d managed blocks after two runs, want 1", n)
	}
	var cfg map[string]map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".mcp.json"))), &cfg); err != nil {
		t.Fatalf("the MCP config is not valid JSON after two runs: %v", err)
	}
	if n := len(cfg["mcpServers"]); n != 2 {
		t.Errorf("two runs registered %d servers, want 2", n)
	}
}

// ---------------------------------------------------------------------------
// CLAUDE.md — only if absent
// ---------------------------------------------------------------------------

func TestAgentSetupWritesTheCLAUDEShimWhenAbsent(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if got := readFile(t, filepath.Join(dir, claudeFilename)); got != claudeShim {
		t.Errorf("CLAUDE.md = %q, want the one-line import %q", got, claudeShim)
	}
}

// TestAgentSetupNeverModifiesAnExistingCLAUDEMD is the "only if absent" rule.
//
// CLAUDE.md is the file an author is most likely to have filled with project
// instructions, and this command has nothing to contribute to one that exists.
// The assertion is byte equality, so an implementation that "helpfully" prepends
// the import fails.
func TestAgentSetupNeverModifiesAnExistingCLAUDEMD(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"a real instruction file", "# House rules\n\nNever run the deploy script.\n"},
		{"one that does not mention AGENTS.md at all", "just some notes\n"},
		{"an empty file", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			path := filepath.Join(dir, claudeFilename)
			writeFile(t, path, tc.body)

			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
				t.Fatalf("agent-setup: %v", err)
			}
			if got := readFile(t, path); got != tc.body {
				t.Errorf("an existing CLAUDE.md was modified.\n want: %q\n  got: %q", tc.body, got)
			}
		})
	}
	// CONTROL: with no CLAUDE.md the same run DOES write one, so the byte
	// equality above is a fact about the "exists" branch and not about a command
	// that never writes this file.
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("control run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, claudeFilename)); err != nil {
		t.Fatalf("control: no CLAUDE.md was written when none existed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MCP registration
// ---------------------------------------------------------------------------

// TestAgentSetupMergesIntoAnExistingMCPConfig is the merge contract: every
// unknown key and every existing server survives.
func TestAgentSetupMergesIntoAnExistingMCPConfig(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, ".mcp.json")
	writeFile(t, path, `{
  "someUnknownTopLevelKey": {"keep": "me"},
  "mcpServers": {
    "their-server": {"command": "node", "args": ["server.js"]}
  }
}`)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatalf("merged config is not valid JSON: %v", err)
	}
	if _, ok := got["someUnknownTopLevelKey"]; !ok {
		t.Error("an unknown top-level key was dropped by the merge")
	}
	servers, _ := got["mcpServers"].(map[string]any)
	if _, ok := servers["their-server"]; !ok {
		t.Error("the user's own MCP server was dropped by the merge")
	}
	for _, srv := range civitaiMCPServers {
		entry, ok := servers[srv.Name].(map[string]any)
		if !ok {
			t.Errorf("%s was not registered", srv.Name)
			continue
		}
		if entry["url"] != srv.URL {
			t.Errorf("%s url = %v, want %s", srv.Name, entry["url"], srv.URL)
		}
	}
}

// TestAgentSetupRefusesAMalformedMCPConfig: refuse by name, do not "repair".
//
// A repair here is indistinguishable from deleting whatever the user had, so the
// guard asserts BOTH halves — the error names the path, and the file is byte-
// identical afterwards.
func TestAgentSetupRefusesAMalformedMCPConfig(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"truncated json", `{"mcpServers": {`},
		{"jsonc comments", "{\n  // my server\n  \"mcpServers\": {}\n}"},
		{"not an object at the servers key", `{"mcpServers": ["nope"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			path := filepath.Join(dir, ".mcp.json")
			writeFile(t, path, tc.body)

			_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude")
			if err == nil {
				t.Fatal("expected a refusal for a config that does not parse")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the refusal must name the path; got: %v", err)
			}
			if got := readFile(t, path); got != tc.body {
				t.Errorf("a refused run rewrote the file anyway:\n%q", got)
			}
		})
	}
	// CONTROL: a WELL-FORMED file at the same path is merged, so the refusals
	// above are about the malformed content and not about a path that is always
	// rejected.
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, ".mcp.json")
	writeFile(t, path, `{"mcpServers": {}}`)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("control: a well-formed config was refused: %v", err)
	}
}

// TestAgentSetupTOMLOnAFreshConfigOpensWithItsTable pins the empty-source arm.
//
// `strings.Split("", "\n")` returns a one-element slice holding a blank line, so
// a config file that does not exist yet rendered a document OPENING with a blank
// line — measured on a real fresh `~/.codex/config.toml`. Harmless to TOML and
// wrong in a file a human reads. The CONTROL below is the half that matters: a
// leading blank line an existing file legitimately has must SURVIVE, so the fix
// could not be a trim.
func TestAgentSetupTOMLOnAFreshConfigOpensWithItsTable(t *testing.T) {
	dir, home := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "codex"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	got := readFile(t, filepath.Join(home, ".codex", "config.toml"))
	if !strings.HasPrefix(got, "[mcp_servers.") {
		t.Errorf("a fresh config.toml does not open with its first table:\n%q", got)
	}

	// CONTROL: an EXISTING file's own leading blank line is a byte this command
	// has no business touching, so a trim-based fix must fail here.
	dir2, home2 := agentSetupProject(t)
	path2 := filepath.Join(home2, ".codex", "config.toml")
	writeFile(t, path2, "\n# a comment after a deliberate blank line\nmodel = \"gpt-5\"\n")
	if _, _, err := run(t, "agent-setup", "--dir", dir2, "--agent", "codex"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if got := readFile(t, path2); !strings.HasPrefix(got, "\n# a comment after a deliberate blank line") {
		t.Errorf("an existing file's leading blank line was stripped:\n%q", got)
	}
}

// TestAgentSetupTOMLMergePreservesEverythingElse is the Codex path. The
// line-based merge exists so a hand-maintained config.toml keeps its comments
// and its key order — properties a decode/encode round trip destroys while
// still "preserving every key".
func TestAgentSetupTOMLMergePreservesEverythingElse(t *testing.T) {
	dir, home := agentSetupProject(t)
	path := filepath.Join(home, ".codex", "config.toml")
	const original = `# my codex config
model = "gpt-5"
approval_policy = "on-request"

[mcp_servers.theirs]
command = "node"
args = ["their-server.js"]

[tui]
theme = "dark"
`
	writeFile(t, path, original)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "codex"); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{
		"# my codex config",
		`model = "gpt-5"`,
		`approval_policy = "on-request"`,
		"[mcp_servers.theirs]",
		`args = ["their-server.js"]`,
		"[tui]",
		`theme = "dark"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the merge lost %q:\n%s", want, got)
		}
	}
	for _, srv := range civitaiMCPServers {
		if !strings.Contains(got, `[mcp_servers."`+srv.Name+`"]`) {
			t.Errorf("%s has no table in the merged config:\n%s", srv.Name, got)
		}
		if !strings.Contains(got, `url = "`+srv.URL+`"`) {
			t.Errorf("%s has no url line:\n%s", srv.Name, got)
		}
	}
	// Idempotent: a second run must not append a duplicate table (which is
	// invalid TOML and would make Codex refuse to start).
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "codex"); err != nil {
		t.Fatalf("second agent-setup: %v", err)
	}
	again := readFile(t, path)
	for _, srv := range civitaiMCPServers {
		if n := strings.Count(again, `[mcp_servers."`+srv.Name+`"]`); n != 1 {
			t.Errorf("%s has %d tables after two runs, want 1:\n%s", srv.Name, n, again)
		}
	}
}

// TestAgentSetupWritesNoPlaceholderToken pins the credential rule in both
// directions: without a token the entry carries NO Authorization header, and
// with one it carries a REFERENCE to CIVITAI_TOKEN in that agent's own
// documented spelling — never the token itself.
//
// 🔴 THIS SECOND SUBTEST USED TO ASSERT `Bearer tok-abc`, AND THAT ASSERTION WAS
// THE BUG RATHER THAN A WITNESS TO IT. It pinned a live credential into
// `.mcp.json` — a PROJECT-scoped file that lives in the repo root and gets
// committed — and a green suite around it is why the leak shipped. The guards
// that now read every byte this command writes are in
// agent_setup_credential_test.go; the rule itself is the block comment above
// agentTargets.
//
// 🔴 A PLACEHOLDER WOULD BE WORSE THAN NOTHING. `Bearer <your-token>` produces a
// config that looks complete in every file the user can read and 401s at request
// time; a header-less entry is honest and still works, because both servers'
// read tools are anonymous.
func TestAgentSetupWritesNoPlaceholderToken(t *testing.T) {
	t.Run("no token", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
			t.Fatalf("agent-setup: %v", err)
		}
		raw := readFile(t, filepath.Join(dir, ".mcp.json"))
		if strings.Contains(raw, "Authorization") || strings.Contains(strings.ToLower(raw), "bearer") {
			t.Errorf("a token-less run wrote an Authorization header:\n%s", raw)
		}
		// And nothing that merely LOOKS like a credential.
		for _, ghost := range []string{"<token>", "YOUR_TOKEN", "your-token", "xxx"} {
			if strings.Contains(raw, ghost) {
				t.Errorf("the config carries the placeholder %q:\n%s", ghost, raw)
			}
		}
	})
	t.Run("with a token", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		const secret = "tok-abc-do-not-write-me"
		t.Setenv("CIVITAI_TOKEN", secret)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
			t.Fatalf("agent-setup: %v", err)
		}
		raw := readFile(t, filepath.Join(dir, ".mcp.json"))
		if strings.Contains(raw, secret) {
			t.Fatalf("the config carries the LITERAL token — see agent_setup_credential_test.go")
		}
		var cfg map[string]map[string]map[string]any
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		headers, _ := cfg["mcpServers"]["civitai"]["headers"].(map[string]any)
		// Claude Code's documented spelling is the BARE `${VAR}` form —
		// code.claude.com/docs/en/mcp lists `headers` among the fields it expands
		// and shows `"Authorization": "Bearer ${API_KEY}"`. `${env:…}` is
		// Cursor's and Windsurf's; writing it here would be sent literally.
		if got := headers["Authorization"]; got != "Bearer ${CIVITAI_TOKEN}" {
			t.Errorf("Authorization = %v, want %q", got, "Bearer ${CIVITAI_TOKEN}")
		}
	})
}

// TestAgentSetupPerAgentEntryShape drives a real write for every agent and reads
// the URL back out through THAT agent's own key names — the end-to-end form of
// the §2 table. A run that wrote `url` into a Windsurf config, or `mcpServers`
// into a VS Code one, produces a file that parses and registers nothing; only
// reading it back under the right key can see that.
func TestAgentSetupPerAgentEntryShape(t *testing.T) {
	for _, agent := range []string{agentClaude, agentCursor, agentVSCode, agentOpencode, agentWindsurf, agentZed} {
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup --agent %s: %v", agent, err)
			}
			target := agentTargets[agent]
			env := liveAgentEnv(dir)
			path, ok := agentConfigPath(env, agent)
			if !ok {
				t.Fatalf("no config path for %s", agent)
			}
			var root map[string]any
			if err := json.Unmarshal([]byte(readFile(t, path)), &root); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			servers, ok := root[target.ServersKey].(map[string]any)
			if !ok {
				t.Fatalf("%s has no %q object — the top-level key is wrong:\n%v", path, target.ServersKey, root)
			}
			for _, srv := range civitaiMCPServers {
				entry, ok := servers[srv.Name].(map[string]any)
				if !ok {
					t.Fatalf("%s is not registered in %s", srv.Name, path)
				}
				if entry[target.URLKey] != srv.URL {
					t.Errorf("%s: %s[%q] = %v, want %s", agent, srv.Name, target.URLKey, entry[target.URLKey], srv.URL)
				}
				if target.TypeKey != "" && entry[target.TypeKey] != target.TypeValue {
					t.Errorf("%s: %s[%q] = %v, want %q", agent, srv.Name, target.TypeKey, entry[target.TypeKey], target.TypeValue)
				}
				// 🔴 The keys this agent does NOT use must be absent. An extra
				// `type` or a duplicate `url` beside `serverUrl` is an unknown key
				// in somebody else's config, which is what the per-agent table
				// exists to avoid.
				if target.TypeKey == "" {
					if _, bad := entry["type"]; bad {
						t.Errorf("%s: %s carries a `type` key its docs do not define: %v", agent, srv.Name, entry)
					}
				}
				if target.URLKey != "url" {
					if _, bad := entry["url"]; bad {
						t.Errorf("%s: %s carries both `url` and %q", agent, srv.Name, target.URLKey)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// --dry-run
// ---------------------------------------------------------------------------

// TestAgentSetupDryRunWritesNothing: every path is named, no byte is written.
func TestAgentSetupDryRunWritesNothing(t *testing.T) {
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("agent-setup --dry-run: %v", err)
	}
	for _, want := range []string{agentsFilename, claudeFilename, ".mcp.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("--dry-run did not name %s:\n%s", want, out)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("--dry-run wrote %v", names)
	}
	// CONTROL: the same invocation WITHOUT --dry-run writes all three, so the
	// empty directory above is about the flag and not about a command that never
	// writes anything.
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("control run: %v", err)
	}
	for _, name := range []string{agentsFilename, claudeFilename, ".mcp.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("control: %s was not written by a real run: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// usage errors (exit 2)
// ---------------------------------------------------------------------------

// TestAgentSetupTrackAPIRefuses is §4: a RECOGNISED value that refuses with a
// pointer, never a silent fall-back to the app track.
func TestAgentSetupTrackAPIRefuses(t *testing.T) {
	dir, _ := agentSetupProject(t)
	_, _, err := run(t, "agent-setup", "--dir", dir, "--track", "api")
	if err == nil {
		t.Fatal("--track api must refuse, not fall back to the app track")
	}
	msg := err.Error()
	for _, want := range []string{
		"https://mcp.civitai.com/mcp",
		"https://orchestration.civitai.com/mcp",
		"https://developer.civitai.com/site/",
		"https://developer.civitai.com/orchestration/",
		"coming",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the --track api refusal does not carry %q:\n%s", want, msg)
		}
	}
	// AGENTS.md item 7 — the exit code is pinned structurally, never from text.
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--track api must classify as a usage error (exit 2), got %T: %v", err, err)
	}
	// It must NOT have silently done the work anyway.
	if _, statErr := os.Stat(filepath.Join(dir, agentsFilename)); statErr == nil {
		t.Error("--track api wrote AGENTS.md — it fell back to the app track")
	}
	// CONTROL: an unrecognised track is a different, plainer refusal.
	_, _, err = run(t, "agent-setup", "--dir", dir, "--track", "nonsense")
	if err == nil || !strings.Contains(err.Error(), "unknown --track") {
		t.Errorf("an unknown --track must be refused as such; got: %v", err)
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("an unknown --track must be a usage error, got %T: %v", err, err)
	}
}

func TestAgentSetupUnknownAgentIsUsageError(t *testing.T) {
	dir, _ := agentSetupProject(t)
	_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "vs-code")
	if err == nil {
		t.Fatal("expected a refusal for an unknown --agent")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("an unknown --agent must classify as a usage error (exit 2), got %T: %v", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, agentsFilename)); statErr == nil {
		t.Error("a run with an unknown --agent wrote files anyway")
	}
}

// TestAgentSetupDirClassification is item 26's three-way branch, applied to this
// command's own --dir. Classification is asserted with errors.Is, never text.
func TestAgentSetupDirClassification(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notadir.txt"), "hi\n")

	for _, tc := range []struct {
		name      string
		dir       string
		wantUsage bool
		wantErr   bool
	}{
		{"nonexistent", filepath.Join(root, "nope"), true, true},
		{"a regular file", filepath.Join(root, "notadir.txt"), true, true},
		{"a real directory", root, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := resolveAgentSetupDir(tc.dir)
			if tc.wantErr != (err != nil) {
				t.Fatalf("resolveAgentSetupDir(%s) err = %v, wantErr %t", tc.dir, err, tc.wantErr)
			}
			if got := errors.Is(err, ErrUsage); got != tc.wantUsage {
				t.Errorf("errors.Is(err, ErrUsage) = %t, want %t (err: %v)", got, tc.wantUsage, err)
			}
		})
	}

	// End to end, through the real command tree.
	for _, tc := range []struct{ name, dir string }{
		{"nonexistent", filepath.Join(root, "nope")},
		{"a regular file", filepath.Join(root, "notadir.txt")},
	} {
		t.Run("command/"+tc.name, func(t *testing.T) {
			_, _, err := run(t, "agent-setup", "--dir", tc.dir)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("want ErrUsage (exit 2), got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tc.dir) {
				t.Errorf("the refusal must name the path the user typed; got: %v", err)
			}
		})
	}
}

// TestAgentSetupDirRemediesMatchTheirArm is item 26's operand-order guard,
// regenerated for this command: both arms carry the same ErrUsage sentinel and
// the same path, so swapping the two format strings passes every classification
// assertion while telling a user whose directory is missing to "pass the project
// ROOT, not a file".
//
// 🔴 IT ASSERTS A LITERAL PHRASE PER ARM, NOT ONLY THE CONSTANT, AND THAT IS THE
// WHOLE DIFFERENCE. Measured on this branch: a version of this test that derived
// BOTH expectations from `agentSetupNoSuchDir` / `agentSetupNotADir` SURVIVED a
// clean exchange of the two constants' values — because the test reads the same
// constant NAME the code does, so both sides moved together and the guard agreed
// with itself. That is item 26's own trap arriving from a third direction: the
// mutants recorded there all made the two constants DUPLICATE (which the
// precondition below catches), and a pure SWAP is invisible to a
// constant-derived expectation. The literal anchors are the live protection.
//
// The cost is stated: rewording either remedy past its anchor phrase fails this
// test. That is the price of a machine-readable claim about which advice each
// arm gives, and it is the right price — the alternative shipped backwards advice
// fully green.
func TestAgentSetupDirRemediesMatchTheirArm(t *testing.T) {
	const probe = "/probe/path"
	noSuch := func(p string) string { return strings.ReplaceAll(agentSetupNoSuchDir, "%s", p) }
	notDir := func(p string) string { return strings.ReplaceAll(agentSetupNotADir, "%s", p) }

	// The literal anchors, written here rather than read from the code.
	const (
		anchorNoSuch = "no such directory"
		anchorNotDir = "is not a directory"
	)
	// PRECONDITION on the anchors themselves: each must be in its own constant
	// and absent from the other, or the rows below cannot discriminate.
	if !strings.Contains(agentSetupNoSuchDir, anchorNoSuch) || strings.Contains(agentSetupNoSuchDir, anchorNotDir) {
		t.Fatalf("the no-such-directory remedy no longer carries only its own anchor: %q", agentSetupNoSuchDir)
	}
	if !strings.Contains(agentSetupNotADir, anchorNotDir) || strings.Contains(agentSetupNotADir, anchorNoSuch) {
		t.Fatalf("the not-a-directory remedy no longer carries only its own anchor: %q", agentSetupNotADir)
	}

	// PRECONDITION: both remedies are non-empty and distinct RENDERED WITH THE
	// SAME PATH. `strings.Contains(x, "")` is always true, and two constants that
	// differ only by their interpolated path would make the `deny` half of every
	// row below vacuous.
	if strings.TrimSpace(noSuch(probe)) == "" || strings.TrimSpace(notDir(probe)) == "" {
		t.Fatal("a remedy constant is empty — every assertion below would be vacuously true")
	}
	if noSuch(probe) == notDir(probe) {
		t.Fatal("the two remedies are identical — the deny half of this guard cannot fail")
	}

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "notadir.txt"), "hi\n")
	for _, tc := range []struct {
		name         string
		dir          string
		want, deny   func(string) string
		wantA, denyA string
	}{
		{"nonexistent takes the no-such-directory arm", filepath.Join(root, "nope"),
			noSuch, notDir, anchorNoSuch, anchorNotDir},
		{"a file takes the not-a-directory arm", filepath.Join(root, "notadir.txt"),
			notDir, noSuch, anchorNotDir, anchorNoSuch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := resolveAgentSetupDir(tc.dir)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			// Derived from the constant: catches a DUPLICATED remedy.
			if !strings.Contains(err.Error(), tc.want(tc.dir)) {
				t.Errorf("the refusal does not carry its own remedy.\n want: %s\n  got: %s", tc.want(tc.dir), err)
			}
			if strings.Contains(err.Error(), tc.deny(tc.dir)) {
				t.Errorf("the refusal carries the OTHER arm's remedy.\n got: %s", err)
			}
			// Literal: catches a SWAP, which the two above move with and cannot
			// see. Measured — without these the swap SURVIVED.
			if !strings.Contains(err.Error(), tc.wantA) {
				t.Errorf("this arm must say %q — the two remedy constants are swapped, so a user whose "+
					"directory is simply missing is told to pass the project ROOT rather than a file.\n got: %s",
					tc.wantA, err)
			}
			if strings.Contains(err.Error(), tc.denyA) {
				t.Errorf("this arm says %q, which belongs to the OTHER arm — the constants are swapped.\n got: %s",
					tc.denyA, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// --check
// ---------------------------------------------------------------------------

// TestAgentSetupCheckJSONShape pins the payload `developer.civitai.com`'s setup
// prompt was written against: exactly four keys, and the six row names in order.
func TestAgentSetupCheckJSONShape(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	stdout, _, _ := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check", "--json")

	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &keys); err != nil {
		t.Fatalf("--check --json is not valid JSON: %v\n%s", err, stdout)
	}
	got := make([]string, 0, len(keys))
	for k := range keys {
		got = append(got, k)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != "agent,checks,ok,track" {
		t.Errorf("`--check --json` keys = %v, want exactly [agent checks ok track].\n"+
			"This shape is PUBLISHED — developer.civitai.com's setup prompt parses it.", got)
	}

	var payload agentSetupJSON
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(payload.Checks))
	for _, c := range payload.Checks {
		names = append(names, c.Name)
	}
	want := []string{checkCLIVersion, checkAgentsMD, checkClaudeMD, "mcp-site", "mcp-orch", checkAuthenticated}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("check rows = %v, want %v (the names and their order are published)", names, want)
	}
	if payload.Track != trackApp || payload.Agent != agentClaude {
		t.Errorf("track/agent = %q/%q, want %q/%q", payload.Track, payload.Agent, trackApp, agentClaude)
	}
}

// TestAuthenticatedIsReportedButNeverFailsTheVerdict is THE inversion guard.
//
// 🔴 THIS IS THE SINGLE EASIEST THING IN THE FEATURE TO GET BACKWARDS. Setup
// stops before auth on purpose, so a fresh, correct, unauthenticated setup is a
// SUCCESS. Folding `authenticated` into the AND would make `--check` exit 1 for
// every user in exactly the state the setup flow leaves them in, and the hosted
// prompt — which is instructed not to report success if any check fails — would
// report a failure for a run that did everything right.
//
// Three rows, because two of them are individually satisfiable by a broken
// implementation: "verdict true while authenticated is false" is the inversion,
// and "verdict false when a REAL check fails" is what stops the fix being
// `return true`.
func TestAuthenticatedIsReportedButNeverFailsTheVerdict(t *testing.T) {
	t.Run("a complete but unauthenticated setup is ok and exits 0", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		stdout, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check", "--json")
		if err != nil {
			t.Fatalf("an unauthenticated but complete setup must exit 0, got: %v\n%s", err, stdout)
		}
		var payload agentSetupJSON
		if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
			t.Fatal(jerr)
		}
		// PREMISE: the `authenticated` row really did fail, or this row observes
		// nothing at all.
		var authRow *agentCheckJSON
		for i := range payload.Checks {
			if payload.Checks[i].Name == checkAuthenticated {
				authRow = &payload.Checks[i]
			}
		}
		if authRow == nil {
			t.Fatal("PREMISE BROKEN: there is no `authenticated` row — it must be REPORTED, not omitted")
		}
		if authRow.OK {
			t.Fatalf("PREMISE BROKEN: `authenticated` passed with no token configured (%q) — "+
				"this row cannot see the inversion it exists to catch", authRow.Detail)
		}
		if !strings.Contains(authRow.Detail, "civitai login") {
			t.Errorf("the `authenticated` row must name the next command; got %q", authRow.Detail)
		}
		if !payload.OK {
			t.Error("ok = false with only `authenticated` failing — the auth row is being counted, and a " +
				"fresh unauthenticated setup is a SUCCESS. This is the inversion this test exists for.")
		}
	})

	t.Run("a real failed check DOES set ok false and exit non-zero", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		// Nothing written at all: agents-md, claude-md and both MCP rows fail.
		stdout, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check", "--json")
		if err == nil {
			t.Fatal("an empty directory must not check clean")
		}
		if !errors.Is(err, ErrAgentSetupIncomplete) {
			t.Errorf("want ErrAgentSetupIncomplete, got %T: %v", err, err)
		}
		var payload agentSetupJSON
		if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
			t.Fatal(jerr)
		}
		if payload.OK {
			t.Error("ok = true with four real checks failing — the verdict is not reading them")
		}
	})

	t.Run("authenticated passing does not paper over a real failure", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		t.Setenv("CIVITAI_TOKEN", "tok-1")
		stdout, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check", "--json")
		if err == nil {
			t.Fatal("an empty directory must not check clean just because a token exists")
		}
		var payload agentSetupJSON
		if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
			t.Fatal(jerr)
		}
		if payload.OK {
			t.Error("ok = true — a configured token must not satisfy the other checks")
		}
	})
}

// TestAgentSetupVerdictUnit drives the verdict function directly, over the two
// rows that decide it. A unit here catches an inversion the end-to-end rows
// above could only see through a whole run.
func TestAgentSetupVerdictUnit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		checks []agentCheckJSON
		want   bool
	}{
		{"only authenticated fails", []agentCheckJSON{
			{Name: checkAgentsMD, OK: true}, {Name: checkAuthenticated, OK: false}}, true},
		{"a real check fails", []agentCheckJSON{
			{Name: checkAgentsMD, OK: false}, {Name: checkAuthenticated, OK: true}}, false},
		{"everything passes", []agentCheckJSON{
			{Name: checkAgentsMD, OK: true}, {Name: checkAuthenticated, OK: true}}, true},
		{"everything fails", []agentCheckJSON{
			{Name: checkAgentsMD, OK: false}, {Name: checkAuthenticated, OK: false}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentSetupVerdict(tc.checks); got != tc.want {
				t.Errorf("agentSetupVerdict = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestAgentSetupCheckWritesNothing: --check is a read. A verify step that
// repaired the thing it was verifying would make the hosted prompt's "report the
// raw output" instruction meaningless.
func TestAgentSetupCheckWritesNothing(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check"); err == nil {
		t.Fatal("an empty directory must not check clean")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("--check wrote into the project directory: %v", entries)
	}
}

// TestAgentSetupCheckJSONCarriesNoStyling is internal/ui/CONVENTION.md rule 1.
func TestAgentSetupCheckJSONCarriesNoStyling(t *testing.T) {
	dir, _ := agentSetupProject(t)
	stdout, _, _ := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check", "--json")
	if strings.ContainsAny(stdout, "\x1b") {
		t.Errorf("--json stdout carries an ANSI escape:\n%q", stdout)
	}
	for _, glyph := range []string{"✓", "⚠", "✗"} {
		if strings.Contains(stdout, glyph) {
			t.Errorf("--json stdout carries the ui glyph %q — machine-readable output must be raw:\n%s", glyph, stdout)
		}
	}
	// POSITIVE CONTROL: the HUMAN rendering of the same run DOES carry a glyph,
	// so the absence above is a property of the --json path and not of a
	// renderer that never emits one.
	human, _, _ := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--check")
	if !strings.ContainsAny(human, "✓⚠✗") {
		t.Fatalf("control: the human --check rendering emits no ui glyph at all, so the --json "+
			"assertion above proves nothing:\n%s", human)
	}
}

// TestAgentSetupOtherWritesNothingAndPrintsThePasteBlock is the `other` arm.
func TestAgentSetupOtherWritesNothingAndPrintsThePasteBlock(t *testing.T) {
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "other")
	if err != nil {
		t.Fatalf("agent-setup --agent other: %v", err)
	}
	// The two instruction files ARE still written — only the MCP config is not,
	// because there is no file to write.
	for _, name := range []string{agentsFilename, claudeFilename} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Errorf("--agent other should still write %s: %v", name, statErr)
		}
	}
	for _, srv := range civitaiMCPServers {
		if !strings.Contains(out, srv.URL) {
			t.Errorf("the paste block does not name %s:\n%s", srv.URL, out)
		}
	}
	// The one thing a paste cannot carry is which key name the reader's agent
	// wants, so the differences have to be printed too.
	for _, want := range []string{"context_servers", "serverUrl", "mcp_servers", "servers"} {
		if !strings.Contains(out, want) {
			t.Errorf("the `other` output does not name the differing key %q:\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------------
// the embedded template
// ---------------------------------------------------------------------------

// TestAgentsMarkersAreInTheTemplate pins the marker constants against the
// embedded file. If they drift apart, run 2 appends a SECOND managed block
// instead of replacing the first — and every run after that appends another.
func TestAgentsMarkersAreInTheTemplate(t *testing.T) {
	if !strings.Contains(agentsAppTemplate, agentsBeginMarker) {
		t.Errorf("the embedded template does not open with the BEGIN marker constant.\n"+
			"want: %q\ntemplate head: %q", agentsBeginMarker, firstLine(agentsAppTemplate))
	}
	if !strings.Contains(agentsAppTemplate, agentsEndMarker) {
		t.Errorf("the embedded template does not carry the END marker constant: %q", agentsEndMarker)
	}
	// The em dash is part of the BEGIN marker and a hyphen would not match.
	if !strings.Contains(agentsBeginMarker, "—") {
		t.Error("the BEGIN marker lost its em dash — an existing file's marker would no longer match")
	}
	block := agentsManagedBlock()
	if !strings.HasPrefix(block, agentsBeginMarker) || !strings.HasSuffix(block, agentsEndMarker) {
		t.Errorf("the managed block does not start and end with its markers:\n%q", block)
	}
}

// TestAgentsTemplatePinsNoVersion is the anti-rot rule, asserted mechanically.
//
// 🔴 PINS LIVE IN `civitai app init`, WHICH `pins-vs-published` HOLDS AGAINST
// npm. A version literal in this embedded template rots in silence: it keeps
// rendering, keeps looking authoritative, and points an author at a package set
// that no longer works together — the money path breaking quietly, which is the
// exact failure the template's own text warns about.
func TestAgentsTemplatePinsNoVersion(t *testing.T) {
	// A semver-ish literal anywhere near an `@civitai/` package name.
	pinRe := regexp.MustCompile(`@civitai/[a-z-]+@?\s*[\^~>=<]*\s*\d+\.\d+`)
	if m := pinRe.FindString(agentsAppTemplate); m != "" {
		t.Errorf("the embedded template pins a package version (%q). Pins live in `civitai app init`, "+
			"which CI holds against npm; a literal here rots in silence.", m)
	}
	// POSITIVE CONTROL: the pattern CAN match, so the clean verdict above is not
	// a fact about a regex wired to nothing.
	if !pinRe.MatchString("install @civitai/app-sdk@^1.2.3") {
		t.Fatal("the pin pattern cannot match a real pin — this guard observes nothing")
	}
	// And the template really does mention the packages, so the scan has
	// something to be about.
	if !strings.Contains(agentsAppTemplate, "@civitai/") {
		t.Fatal("the template never mentions an @civitai/ package — the scan above is vacuous")
	}
}

// templateCommandRe matches a `civitai …` invocation inside a markdown code
// span, which is how every command in the template is written.
var templateCommandRe = regexp.MustCompile("`(civitai [^`]+)`")

// commandWordRe matches a word that can be a command NAME. The walk stops at the
// first word that is not one — `<name>`, `--flag`, a value — because everything
// after it is an argument, not a path through the tree.
var commandWordRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// templateCommandPaths extracts every command path named in a markdown document.
func templateCommandPaths(md string) [][]string {
	var out [][]string
	for _, m := range templateCommandRe.FindAllStringSubmatch(md, -1) {
		fields := strings.Fields(m[1])
		var path []string
		for _, f := range fields[1:] {
			if !commandWordRe.MatchString(f) {
				break
			}
			path = append(path, f)
		}
		if len(path) > 0 {
			out = append(out, path)
		}
	}
	return out
}

// TestAgentsTemplateCommandsResolveInTheTree walks the embedded template and
// requires every `civitai …` it names to exist in the real Cobra tree.
//
// 🔴 THIS IS THE FAILURE THE WHOLE FEATURE EXISTS TO PREVENT, TURNED ON ITSELF.
// The template is a document that tells somebody else's agent which commands to
// run; a command named there that the binary does not have is a hosted claim
// about a surface that is not real — precisely the rot `agent-setup` was built
// to replace. A rename in the tree must fail here rather than at an author's
// terminal.
func TestAgentsTemplateCommandsResolveInTheTree(t *testing.T) {
	root := NewRootCmd()
	paths := templateCommandPaths(agentsAppTemplate)

	// Positive control: an extractor reading the wrong text finds nothing and
	// every assertion below becomes vacuous.
	if len(paths) < 5 {
		t.Fatalf("extracted only %d `civitai …` invocations from the template (%v) — the extractor is "+
			"reading the wrong text", len(paths), paths)
	}
	for _, path := range paths {
		c, _, err := root.Find(path)
		if err != nil {
			t.Errorf("the template names `civitai %s`, which does not resolve: %v", strings.Join(path, " "), err)
			continue
		}
		// 🔴 THE NAME CHECK IS THE DISCRIMINATING HALF. cobra's Find returns the
		// deepest command it COULD match plus the leftovers, and does not error
		// on an unknown leaf — so `err == nil` alone is satisfied by a command
		// that does not exist.
		if c.Name() != path[len(path)-1] {
			t.Errorf("the template names `civitai %s`, but that resolves to %q — the command does not exist",
				strings.Join(path, " "), c.CommandPath())
		}
	}
}

// TestTemplateCommandWalkerCanFail is the NEGATIVE CONTROL for the walker above.
// Fed a document naming a command the binary does not have, the same extraction
// and the same resolution must report it — otherwise the green run above is a
// fact about a walk that accepts everything.
func TestTemplateCommandWalkerCanFail(t *testing.T) {
	const bad = "Run `civitai app frobnicate` and then `civitai nonsense thing`.\n"
	paths := templateCommandPaths(bad)
	if len(paths) != 2 {
		t.Fatalf("extracted %d paths from the control document, want 2: %v", len(paths), paths)
	}
	root := NewRootCmd()
	for _, path := range paths {
		c, _, err := root.Find(path)
		if err == nil && c.Name() == path[len(path)-1] {
			t.Errorf("`civitai %s` resolved — the walker cannot reject a command that does not exist",
				strings.Join(path, " "))
		}
	}
	// And the extractor must handle the shapes the real template uses.
	got := templateCommandPaths("`civitai app init <name>` and `civitai app validate` and `civitai app dev-tunnel`")
	want := "app init|app validate|app dev-tunnel"
	var joined []string
	for _, p := range got {
		joined = append(joined, strings.Join(p, " "))
	}
	if strings.Join(joined, "|") != want {
		t.Errorf("extractor over the real spellings = %v, want %s", joined, want)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
