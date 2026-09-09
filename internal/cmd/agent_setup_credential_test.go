package cmd

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The credential guards.
//
// 🔴 THESE ASSERT ON THE BYTES ON DISK, WHICH IS THE THING NOTHING ELSE ASSERTED
// ON. `agent-setup` shipped writing a live `Authorization: Bearer <token>` into
// `.mcp.json` — a PROJECT-scoped file that people commit — and it survived a
// green suite and a 24-mutant matrix because every guard checked the entry's
// SHAPE (which key, which URL) and none checked its CONTENT. So the guard below
// walks every file the run created, in the project directory AND under $HOME,
// and fails on the literal token appearing anywhere in any of them.

// credFixtureToken is the fixture credential. 🔴 IT IS BUILT SO IT CANNOT BE
// READ AS A PLACEHOLDER: it carries the real `civitai_` prefix and 32 hex
// characters, exactly the shape `civitai login` stores, and it contains no
// angle brackets, no "your", no "example", no "xxx". A guard whose fixture looks
// like a placeholder cannot distinguish "the command wrote a placeholder" — which
// is a separate, already-guarded bug — from "the command wrote the credential".
const credFixtureToken = "civitai_9f31c0a7d24e4b8fa5c6071e3d8b2a4c"

// credTokenBody is the token WITHOUT its prefix. Asserted separately so a future
// entry that writes `"Authorization": "Bearer " + strings.TrimPrefix(token, …)`,
// or splits the token across a key and a value, still trips the guard.
const credTokenBody = "9f31c0a7d24e4b8fa5c6071e3d8b2a4c"

// agentsWithConfigFiles is every agent this CLI writes a config file for, sorted
// so the subtest order is stable. Derived from the table rather than listed, so
// a NEW agent is covered by these guards the moment it is added — the whole
// point, since the leak was a per-agent write.
func agentsWithConfigFiles() []string {
	ids := make([]string, 0, len(agentTargets))
	for id := range agentTargets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// filesUnder returns every regular file below root, with its contents.
func filesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[path] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// TestNoFileAgentSetupWritesEverCarriesALiteralToken is THE regression test for
// the credential leak.
//
// It runs a real write for EVERY agent with a config file, with a token
// configured, and then reads every byte the run left behind — in the project
// directory (`.mcp.json`, `.cursor/`, `.vscode/`, `opencode.json`) and under
// $HOME (Codex's `config.toml`, Windsurf's `mcp_config.json`, Zed's
// `settings.json`). Any occurrence of the fixture token, in any of them, fails.
func TestNoFileAgentSetupWritesEverCarriesALiteralToken(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			t.Setenv("CIVITAI_TOKEN", credFixtureToken)

			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup --agent %s: %v", agent, err)
			}

			// Positive control: the run must actually have written a config file
			// for this agent. Without it a guard that finds no token is
			// indistinguishable from a guard that found no files.
			cfgPath, ok := agentConfigPath(liveAgentEnv(dir), agent)
			if !ok {
				t.Fatalf("no config path for %s", agent)
			}
			if _, err := os.Stat(cfgPath); err != nil {
				t.Fatalf("PREMISE BROKEN: %s wrote no config at %s (%v) — this guard "+
					"would then pass without inspecting anything", agent, cfgPath, err)
			}

			for _, root := range []string{dir, home} {
				for path, body := range filesUnder(t, root) {
					if strings.Contains(body, credFixtureToken) {
						t.Errorf("%s carries the LITERAL TOKEN — this file is a credential leak "+
							"(project-scoped config files get committed):\n%s", path, redactToken(body))
					}
					if strings.Contains(body, credTokenBody) {
						t.Errorf("%s carries the token body without its prefix:\n%s", path, redactToken(body))
					}
				}
			}
		})
	}
}

// TestOtherAgentPasteBlockCarriesNoLiteralToken covers the one config block this
// command emits that is NOT a file: `--agent other` prints JSON for the human to
// paste into a config this CLI does not know. A literal token there lands in the
// same place by hand, so it is the same leak with an extra step — and it is
// WORSE than the file case, because the destination agent is unknown, so no
// interpolation syntax can be assumed correct for it either.
func TestOtherAgentPasteBlockCarriesNoLiteralToken(t *testing.T) {
	dir, _ := agentSetupProject(t)
	t.Setenv("CIVITAI_TOKEN", credFixtureToken)

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentOther)
	if err != nil {
		t.Fatalf("agent-setup --agent other: %v", err)
	}
	// Positive control: the paste block really was printed.
	if !strings.Contains(out, "mcpServers") {
		t.Fatalf("PREMISE BROKEN: no paste block in the output:\n%s", out)
	}
	if strings.Contains(out, credFixtureToken) || strings.Contains(out, credTokenBody) {
		t.Errorf("the --agent other paste block carries the literal token:\n%s", redactToken(out))
	}
}

// redactToken keeps a failure message readable without reprinting a credential
// in test output (which is captured by CI and pasted into issues).
func redactToken(s string) string {
	s = strings.ReplaceAll(s, credFixtureToken, "<REDACTED-TOKEN>")
	return strings.ReplaceAll(s, credTokenBody, "<REDACTED-TOKEN-BODY>")
}

// ---------------------------------------------------------------------------
// The per-agent header rule
// ---------------------------------------------------------------------------

// TestPerAgentHeaderRule pins the two branches of the rule, per agent, against
// the table's own OWN declaration — so an agent whose vendor doc does not
// document interpolation gets NO `headers` key at all, and one whose doc does
// gets that vendor's EXACT documented syntax.
//
// 🔴 AN EMPTY `headers` OBJECT IS NOT THE SAME AS NO `headers` KEY, and the
// guard distinguishes them: `"headers": {}` reads to a human as "a header was
// configured and is blank", and to several agents as a header block to send.
func TestPerAgentHeaderRule(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			target := agentTargets[agent]
			dir, _ := agentSetupProject(t)
			t.Setenv("CIVITAI_TOKEN", credFixtureToken)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup --agent %s: %v", agent, err)
			}
			path, ok := agentConfigPath(liveAgentEnv(dir), agent)
			if !ok {
				t.Fatalf("no config path for %s", agent)
			}
			raw := readFile(t, path)

			// Codex: a distinct KEY taking the variable's NAME, not an
			// interpolated header. It must carry that key and NOT a header line.
			if target.EnvBearerKey != "" {
				want := target.EnvBearerKey + ` = "` + tokenEnvVar + `"`
				if !strings.Contains(raw, want) {
					t.Errorf("%s must carry %s; got:\n%s", agent, want, raw)
				}
				if strings.Contains(raw, target.HeadersKey) {
					t.Errorf("%s must not carry a %q line — it is documented as STATIC values, "+
						"i.e. the literal credential; got:\n%s", agent, target.HeadersKey, raw)
				}
				return
			}

			if target.EnvHeaderSyntax == "" {
				// Branch 2: no interpolation documented ⇒ NO header key at all.
				if strings.Contains(raw, target.HeadersKey) {
					t.Errorf("%s documents no env-var interpolation, so its entries must carry NO %q key; got:\n%s",
						agent, target.HeadersKey, raw)
				}
				if strings.Contains(strings.ToLower(raw), "authorization") {
					t.Errorf("%s carries an Authorization header it cannot fill from the environment:\n%s", agent, raw)
				}
				return
			}

			// Branch 1: the vendor's EXACT documented syntax, referencing the env
			// var this CLI already publishes as its token override.
			want := "Bearer " + target.EnvHeaderSyntax
			if !strings.Contains(raw, want) {
				t.Errorf("%s must carry the documented interpolation %q; got:\n%s", agent, want, raw)
			}
			if !strings.Contains(target.EnvHeaderSyntax, tokenEnvVar) {
				t.Errorf("%s's interpolation %q does not reference %s", agent, target.EnvHeaderSyntax, tokenEnvVar)
			}
			if target.Format == formatTOML {
				return
			}
			var root map[string]any
			if err := json.Unmarshal([]byte(raw), &root); err != nil {
				t.Fatalf("%s config is not valid JSON: %v", agent, err)
			}
			section, _ := root[target.ServersKey].(map[string]any)
			for _, srv := range civitaiMCPServers {
				entry, _ := section[srv.Name].(map[string]any)
				headers, hasHeaders := entry[target.HeadersKey].(map[string]any)
				if !hasHeaders {
					t.Errorf("%s/%s has no %q object", agent, srv.Name, target.HeadersKey)
					continue
				}
				if got := headers["Authorization"]; got != want {
					t.Errorf("%s/%s Authorization = %v, want %q", agent, srv.Name, got, want)
				}
			}
		})
	}
}

// TestEveryTargetPicksOneCredentialMechanism is the ledger over the table: an
// entry may interpolate into a header, or name an environment variable in a key
// of its own, or do neither — never both. A target carrying both would emit two
// competing Authorization sources whose precedence is the vendor's business and
// not documented the same way twice.
func TestEveryTargetPicksOneCredentialMechanism(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		target := agentTargets[agent]
		if target.EnvHeaderSyntax != "" && target.EnvBearerKey != "" {
			t.Errorf("%s declares both an interpolation (%q) and an env-var key (%q)",
				agent, target.EnvHeaderSyntax, target.EnvBearerKey)
		}
		if target.EnvHeaderSyntax != "" && target.HeadersKey == "" {
			t.Errorf("%s declares an interpolation but no headers key to put it in", agent)
		}
		// The syntax must name the ONE variable this CLI publishes. A second
		// spelling would be a variable the user was never told to export, so the
		// header would resolve to empty and read as a bad token.
		if s := target.EnvHeaderSyntax; s != "" && !strings.Contains(s, tokenEnvVar) {
			t.Errorf("%s interpolation %q does not reference %s", agent, s, tokenEnvVar)
		}
		// 🔴 AND IT MUST NOT BE A BARE VARIABLE NAME. Every documented syntax
		// here wraps the name in braces; an unwrapped `CIVITAI_TOKEN` would be
		// sent as that literal string.
		if s := target.EnvHeaderSyntax; s != "" && !strings.Contains(s, "{") {
			t.Errorf("%s interpolation %q has no braces — it would be sent literally", agent, s)
		}
	}
}

// TestCheckDoesNotFailOnAnAbsentHeader.
//
// 🔴 AN ABSENT AUTHORIZATION HEADER IS NOW THE CORRECT STATE FOR VS CODE AND
// ZED, so `--check` must not report it as a problem — the same shape as the
// `authenticated` row, which is reported and deliberately excluded from the
// verdict. A check that went red here would be permanently red for every VS Code
// and Zed user who did everything right, and a gate nobody can clear is a gate
// everyone learns to ignore.
func TestCheckDoesNotFailOnAnAbsentHeader(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		if agentTargets[agent].EnvHeaderSyntax != "" || agentTargets[agent].EnvBearerKey != "" {
			continue
		}
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			t.Setenv("CIVITAI_TOKEN", credFixtureToken)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup --agent %s: %v", agent, err)
			}
			path, _ := agentConfigPath(liveAgentEnv(dir), agent)
			// Positive control: the premise of this test is that the config it is
			// checking really does lack a header.
			if strings.Contains(strings.ToLower(readFile(t, path)), "authorization") {
				t.Fatalf("PREMISE BROKEN: %s's config carries an Authorization header, so this "+
					"test is not exercising the absent-header case", agent)
			}
			out, _, err := run(t, "agent-setup", "--check", "--json", "--dir", dir, "--agent", agent)
			if err != nil {
				t.Fatalf("--check failed on a correctly header-less setup: %v\n%s", err, out)
			}
			var payload agentSetupJSON
			if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
				t.Fatalf("bad --check json: %v\n%s", jsonErr, out)
			}
			if !payload.OK {
				t.Errorf("ok = false for a setup whose only 'gap' is the absent header:\n%s", out)
			}
			for _, c := range payload.Checks {
				if c.Name == checkAuthenticated {
					continue
				}
				if !c.OK {
					t.Errorf("check %q failed: %s", c.Name, c.Detail)
				}
			}
		})
	}
}

// TestNoAgentGetsAnEmptyHeadersObject is the mirror of the branch-2 assertion
// above, stated as its own guard because "no headers key" and "an empty headers
// key" are one `if` apart and only one of them is correct.
func TestNoAgentGetsAnEmptyHeadersObject(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			target := agentTargets[agent]
			if target.Format == formatTOML {
				t.Skip("TOML entries render header lines, not an object")
			}
			dir, _ := agentSetupProject(t)
			t.Setenv("CIVITAI_TOKEN", credFixtureToken)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup --agent %s: %v", agent, err)
			}
			path, _ := agentConfigPath(liveAgentEnv(dir), agent)
			var root map[string]any
			if err := json.Unmarshal([]byte(readFile(t, path)), &root); err != nil {
				t.Fatalf("%s config is not valid JSON: %v", agent, err)
			}
			section, _ := root[target.ServersKey].(map[string]any)
			for _, srv := range civitaiMCPServers {
				entry, _ := section[srv.Name].(map[string]any)
				h, present := entry[target.HeadersKey]
				if !present {
					continue
				}
				m, isMap := h.(map[string]any)
				if !isMap || len(m) == 0 {
					t.Errorf("%s/%s has a present-but-empty %q (%v) — write no key at all instead",
						agent, srv.Name, target.HeadersKey, h)
				}
			}
		})
	}
}

// TestAgentSetupDoesNotTouchAUserWrittenLiteralToken: the user's own file is
// theirs. If they put a literal `Authorization` header on THEIR OWN server entry,
// this command must leave it exactly as they wrote it — it is not our credential,
// not our file, and "fixing" it is the same class of overreach as "repairing" a
// config that does not parse.
func TestAgentSetupDoesNotTouchAUserWrittenLiteralToken(t *testing.T) {
	dir, _ := agentSetupProject(t)
	t.Setenv("CIVITAI_TOKEN", credFixtureToken)
	path := filepath.Join(dir, ".mcp.json")
	const theirs = "Bearer sk-their-own-literal-secret-8891"
	writeFile(t, path, `{
  "mcpServers": {
    "their-server": {
      "type": "http",
      "url": "https://example.invalid/mcp",
      "headers": {"Authorization": "`+theirs+`"}
    }
  }
}`)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &root); err != nil {
		t.Fatalf("merged config is not valid JSON: %v", err)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	entry, ok := servers["their-server"].(map[string]any)
	if !ok {
		t.Fatal("the user's own server was dropped by the merge")
	}
	headers, _ := entry["headers"].(map[string]any)
	if got := headers["Authorization"]; got != theirs {
		t.Errorf("the user's own Authorization header was rewritten: %v, want %q", got, theirs)
	}
	// And ours were still registered beside it.
	for _, srv := range civitaiMCPServers {
		if _, ok := servers[srv.Name]; !ok {
			t.Errorf("%s was not registered", srv.Name)
		}
	}
}
