package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The round-1 audit guards for `civitai agent-setup`.
//
// 🔴 EVERY TEST BELOW *AS ROUND 1 WROTE IT* WAS WATCHED RED ON THE PRE-FIX TREE
// (6340db8) AND GREEN AFTER. They are regression tests, not invariant guards:
// each one names a behaviour that was MEASURED wrong by running the binary, not
// a property that was merely unasserted. The red-then-green matrix is in the PR.
//
// 🔴 TWO OF THEM HAVE SINCE BEEN REWRITTEN, AND THE BLANKET ABOVE DOES NOT COVER
// A REWRITE — a header that keeps asserting a matrix measured against a
// DIFFERENT body is the description-wider-than-body defect this PR has now hit
// four times. `TestTOMLMergePreservesKeysOnOurOwnTable` (rewritten in round 2)
// and `TestRepeatedRunsAreIdempotent` (rewritten in rounds 2 and 3) each carry
// their OWN, narrower matrix at their own docstring, measured against the tree
// the rewrite was made on. Read those, not this paragraph, for either of them.
// In particular `TestRepeatedRunsAreIdempotent` is NOT wholly a regression
// guard: six of its seven per-agent subtests PASS on `c801ab8`.

// ---------------------------------------------------------------------------
// 1 — the two servers do not both work anonymously
// ---------------------------------------------------------------------------

// TestTheTwoServersDisagreeAboutAnonymousAccess pins the measured fact the whole
// header-less design rests on.
//
// 🔴 THIS IS A PIN ON A LIVE MEASUREMENT, AND IT IS DELIBERATELY NOT A NETWORK
// TEST. Probed with no credential, POST `initialize`:
//
//	https://mcp.civitai.com/mcp            -> 200
//	https://orchestration.civitai.com/mcp  -> 401, empty body, no WWW-Authenticate
//
// The shipped code claimed BOTH were anonymous, in seven places, and used that
// claim to justify writing no Authorization header. If the orchestration server
// later opens up, change the table AND re-probe — do not change this test to
// match a table someone edited without measuring.
func TestTheTwoServersDisagreeAboutAnonymousAccess(t *testing.T) {
	want := map[string]bool{
		"civitai":               true,
		"civitai-orchestration": false,
	}
	if len(civitaiMCPServers) != len(want) {
		t.Fatalf("the server table has %d entries and this guard knows %d — a new server needs a MEASURED "+
			"anonymity value here, not a guessed one", len(civitaiMCPServers), len(want))
	}
	for _, srv := range civitaiMCPServers {
		expected, known := want[srv.Name]
		if !known {
			t.Fatalf("server %q is not in this guard's measured table", srv.Name)
		}
		if srv.Anonymous != expected {
			t.Errorf("%s Anonymous = %t, measured %t", srv.Name, srv.Anonymous, expected)
		}
	}
}

// TestAnonymityNoteNamesBothGroups: the sentence every header-less surface
// prints must name the server that answers AND the one that does not. A note
// that named only one group is how "both servers work anonymously" read as true.
func TestAnonymityNoteNamesBothGroups(t *testing.T) {
	note := mcpAnonymityNote()
	for _, srv := range civitaiMCPServers {
		if !strings.Contains(note, srv.Name) {
			t.Errorf("the anonymity note does not name %s:\n%s", srv.Name, note)
		}
	}
	if !strings.Contains(note, "401") {
		t.Errorf("the anonymity note does not say what happens without a credential:\n%s", note)
	}
}

// TestHeaderLessRunNamesTheServerThatNeedsAHeader is the BEHAVIOURAL half: a
// real run for an agent that gets no header must tell the user, in its own
// output, that one of the two servers it just registered will 401.
//
// 🔴 THIS IS THE CASE THE OLD OUTPUT GOT WRONG OUT LOUD. It printed "The read
// tools work anonymously, so this setup is usable as it stands" — for a
// registration that is half unreachable.
func TestHeaderLessRunNamesTheServerThatNeedsAHeader(t *testing.T) {
	// Every agent/token combination that produces no Authorization header.
	for _, agent := range agentsWithConfigFiles() {
		target := agentTargets[agent]
		for _, withToken := range []bool{false, true} {
			if withToken && (target.EnvHeaderSyntax != "" || target.EnvBearerKey != "") {
				continue // this one DOES get a credential reference
			}
			name := agent + "/no-token"
			if withToken {
				name = agent + "/token-but-no-syntax"
			}
			t.Run(name, func(t *testing.T) {
				dir, _ := agentSetupProject(t)
				if withToken {
					t.Setenv("CIVITAI_TOKEN", credFixtureToken)
				}
				out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent)
				if err != nil {
					t.Fatalf("agent-setup --agent %s: %v", agent, err)
				}
				// Premise: this run really did write no Authorization header.
				path, ok := agentConfigPath(liveAgentEnv(dir), agent)
				if !ok {
					t.Fatalf("no config path for %s", agent)
				}
				if strings.Contains(strings.ToLower(readFile(t, path)), "authorization") {
					t.Fatalf("PREMISE BROKEN: %s got an Authorization header, so this is not the "+
						"header-less case", agent)
				}
				for _, srv := range civitaiMCPServers {
					if srv.Anonymous {
						continue
					}
					if !strings.Contains(out, srv.Name) {
						t.Errorf("the output never names %s, the server this config cannot reach:\n%s",
							srv.Name, out)
					}
				}
				if !strings.Contains(out, "401") {
					t.Errorf("the output does not say a registered server will 401 without a header:\n%s", out)
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// 2 — a JSONC config must not abort the run
// ---------------------------------------------------------------------------

// zedCommentedSettings is what Zed actually ships in ~/.config/zed/settings.json:
// a comment block, then the object. Reproduced from a stock install.
const zedCommentedSettings = `// Zed settings
//
// For information on how to configure Zed, see the Zed
// documentation: https://zed.dev/docs/configuring-zed
{
  "theme": "One Dark",
  "buffer_font_size": 15
}
`

// TestZedCommentedSettingsDoNotAbortTheRun.
//
// 🔴 MEASURED RED: `--agent zed` against the file above exited 1 with
// "settings.json does not parse (invalid character '/' …) — fix or move that
// file", and the project directory was EMPTY: no AGENTS.md, no CLAUDE.md. Two
// independent defects — a JSONC file refused as malformed, and an MCP refusal
// taking the instruction files down with it — and this guard covers the first.
func TestZedCommentedSettingsDoNotAbortTheRun(t *testing.T) {
	dir, home := agentSetupProject(t)
	settings := filepath.Join(home, ".config", "zed", "settings.json")
	writeFile(t, settings, zedCommentedSettings)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed); err != nil {
		t.Fatalf("a stock Zed settings.json was refused: %v", err)
	}
	for _, name := range []string{agentsFilename, claudeFilename} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was not written: %v", name, err)
		}
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(readFile(t, settings)), &root); err != nil {
		t.Fatalf("the merged settings.json is not valid JSON: %v", err)
	}
	// The user's own keys survived…
	if root["theme"] != "One Dark" {
		t.Errorf("the user's theme was lost: %v", root["theme"])
	}
	// …and both servers landed.
	section, _ := root["context_servers"].(map[string]any)
	for _, srv := range civitaiMCPServers {
		if _, ok := section[srv.Name]; !ok {
			t.Errorf("%s was not registered", srv.Name)
		}
	}
}

// TestBlankJSONCommentsLeavesStringsAlone is the unit under the guard above: a
// `//` or `/*` INSIDE a string is data, not a comment, and blanking it would
// corrupt exactly the values these files carry — URLs.
func TestBlankJSONCommentsLeavesStringsAlone(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"url in a string survives",
			`{"url": "https://mcp.civitai.com/mcp"}`,
			`{"url": "https://mcp.civitai.com/mcp"}`},
		{"block-comment marker in a string survives",
			`{"a": "/* not a comment */"}`,
			`{"a": "/* not a comment */"}`},
		{"an escaped quote does not end the string",
			`{"a": "say \" // still in string"}`,
			`{"a": "say \" // still in string"}`},
		{"a line comment is blanked, the newline kept",
			"// hi\n{}",
			"     \n{}"},
		{"a block comment is blanked, newlines kept",
			"/* a\nb */{}",
			"    \n    {}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := string(blankJSONComments([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("blankJSONComments(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) != len(tc.in) {
				t.Errorf("length changed: %d -> %d; offsets into the original are no longer valid",
					len(tc.in), len(got))
			}
		})
	}
}

// TestAGenuinelyMalformedConfigStillRefuses is the OTHER direction, and it is
// the one a JSONC fix is most likely to break: tolerating comments must not
// tolerate a broken file. Refuse-rather-than-repair is still the rule.
func TestAGenuinelyMalformedConfigStillRefuses(t *testing.T) {
	dir, home := agentSetupProject(t)
	settings := filepath.Join(home, ".config", "zed", "settings.json")
	writeFile(t, settings, "// a comment, then rubbish\n{ \"context_servers\": \n")

	_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed)
	if err == nil {
		t.Fatal("a truncated settings.json was accepted — the never-repair rule is gone")
	}
	if !strings.Contains(err.Error(), "does not parse") {
		t.Errorf("the refusal does not name the parse failure: %v", err)
	}
	// The file is untouched.
	if got := readFile(t, settings); !strings.Contains(got, "rubbish") {
		t.Errorf("the malformed file was rewritten:\n%s", got)
	}
}

// TestAnMCPRefusalStillWritesTheInstructionFiles is the second half of finding 2.
//
// 🔴 MEASURED RED: the MCP plan runs before ANY write, so an unreadable config
// file — one this command was never going to touch — left the project directory
// empty. The instruction files have nothing to do with the MCP config.
func TestAnMCPRefusalStillWritesTheInstructionFiles(t *testing.T) {
	dir, home := agentSetupProject(t)
	settings := filepath.Join(home, ".config", "zed", "settings.json")
	writeFile(t, settings, "{ this is not json")

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed, "--json")
	if err == nil {
		t.Fatal("a run that could not write the MCP config exited 0 — a partial run must not report success")
	}
	for _, name := range []string{agentsFilename, claudeFilename} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Errorf("%s was not written despite the MCP refusal being unrelated to it: %v", name, statErr)
		}
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("a degraded run emitted no readable payload (%v):\n%s", jsonErr, out)
	}
	if payload.OK {
		t.Error("ok = true on a run that wrote no MCP config")
	}
	var blocked *agentChangeJSON
	for i := range payload.Changes {
		if payload.Changes[i].Action == actionBlocked {
			blocked = &payload.Changes[i]
		}
	}
	if blocked == nil {
		t.Fatalf("no %q change row explaining what did not happen:\n%s", actionBlocked, out)
	}
	if !strings.Contains(blocked.Reason, "does not parse") {
		t.Errorf("the blocked row does not carry the refusal: %s", blocked.Reason)
	}
}

// ---------------------------------------------------------------------------
// 3 — the merge preserves keys on OUR entries too
// ---------------------------------------------------------------------------

// TestMergePreservesKeysOnOurOwnEntries.
//
// 🔴 MEASURED RED, AND IT DELETED WHAT THE COMMAND ITSELF TOLD PEOPLE TO ADD.
// The no-interpolation branch instructs Zed users to add an `Authorization`
// header by hand to the Civitai entries; the next run assigned each entry
// wholesale and the header was gone, rc 0, with `--check` still `ok: true`.
//
// The pre-existing guard (TestAgentSetupDoesNotTouchAUserWrittenLiteralToken)
// could not see this: it is scoped to a DIFFERENT server (`their-server`), and
// other servers were never the broken case.
func TestMergePreservesKeysOnOurOwnEntries(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, ".mcp.json")
	const theirHeader = "Bearer their-own-hand-added-reference"
	writeFile(t, path, `{
  "mcpServers": {
    "civitai": {
      "type": "http",
      "url": "https://mcp.civitai.com/mcp",
      "headers": {"Authorization": "`+theirHeader+`", "X-Their-Trace": "keep-me"},
      "timeout": 30
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
	entry, ok := servers["civitai"].(map[string]any)
	if !ok {
		t.Fatal("our own entry disappeared")
	}
	if got := entry["timeout"]; got != float64(30) {
		t.Errorf("a key the user added to OUR entry was dropped: timeout = %v, want 30", got)
	}
	headers, _ := entry["headers"].(map[string]any)
	if got := headers["Authorization"]; got != theirHeader {
		t.Errorf("the user's hand-added Authorization on OUR entry was dropped: %v, want %q", got, theirHeader)
	}
	if got := headers["X-Their-Trace"]; got != "keep-me" {
		t.Errorf("an unrelated header on OUR entry was dropped: %v", got)
	}
	// …and this command's own keys are still correct.
	if got := entry["url"]; got != "https://mcp.civitai.com/mcp" {
		t.Errorf("url = %v — the merge stopped writing our own key", got)
	}
	if got := entry["type"]; got != "http" {
		t.Errorf("type = %v — the merge stopped writing our own key", got)
	}
}

// TestTOMLMergePreservesKeysOnOurOwnTable is the same defect on the Codex path,
// where a server table legitimately carries timeouts and an enabled flag.
//
// 🔴 THE FIXTURE CARRIES BOTH AUTH KEYS, AND THAT IS THE HALF ROUND 1 MISSED.
// This guard's docstring claimed "the same defect on the Codex path" while its
// fixture held only `startup_timeout_sec` / `tool_timeout_sec` — two keys the
// TOML renderer never emits under ANY condition. The keys that were actually
// being deleted are the ones the renderer emits SOMETIMES: `http_headers` (never
// emitted — Codex's EnvHeaderSyntax is "") and `bearer_token_env_var` (emitted
// only with a token configured). `mergeTOMLBlock` skipped both unconditionally,
// so a hand-added `http_headers` — which is how a Codex user authenticates
// today — was deleted with nothing put back, at rc 0. Measured on c801ab8.
// A fixture that avoids the sometimes-written keys cannot see that class.
func TestTOMLMergePreservesKeysOnOurOwnTable(t *testing.T) {
	// theirHeaderLine is the line a Codex user adds by hand to authenticate:
	// `http_headers` is the ONLY static-header key Codex documents, and this
	// command never writes it. The value is a synthetic fixture.
	const theirHeaderLine = `http_headers = { Authorization = "Bearer sk-their-own-literal-secret-8891" }`
	const theirBearerLine = `bearer_token_env_var = "MY_OWN_CIVITAI_VAR"`
	fixture := `model = "gpt-5"

[mcp_servers."civitai"]
url = "https://mcp.civitai.com/mcp"
# I added this by hand so it works:
` + theirHeaderLine + `
` + theirBearerLine + `
startup_timeout_sec = 30
tool_timeout_sec = 120
`

	t.Run("no token configured", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		path := filepath.Join(home, ".codex", "config.toml")
		writeFile(t, path, fixture)
		t.Setenv("CIVITAI_TOKEN", "")

		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex); err != nil {
			t.Fatalf("agent-setup: %v", err)
		}
		got := readFile(t, path)
		for _, want := range []string{
			"startup_timeout_sec = 30",
			"tool_timeout_sec = 120",
			`model = "gpt-5"`,
			`[mcp_servers."civitai"]`,
			`url = "https://mcp.civitai.com/mcp"`,
			"# I added this by hand so it works:",
			// 🔴 THE TWO THE RENDERER DID NOT PUT BACK. With no token this run
			// renders neither key, so skipping their lines deletes them.
			theirHeaderLine,
			theirBearerLine,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("the merge dropped %q:\n%s", want, got)
			}
		}
		// The url line must appear ONCE — a merge that appends ours beside the old
		// one produces a table TOML rejects.
		if n := strings.Count(got, `url = "https://mcp.civitai.com/mcp"`); n != 1 {
			t.Errorf("the civitai url appears %d times, want 1:\n%s", n, got)
		}
	})

	t.Run("token configured replaces only the key we render", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		path := filepath.Join(home, ".codex", "config.toml")
		writeFile(t, path, fixture)
		t.Setenv("CIVITAI_TOKEN", credFixtureToken)

		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex); err != nil {
			t.Fatalf("agent-setup: %v", err)
		}
		got := readFile(t, path)
		// `bearer_token_env_var` IS re-rendered with a token, so ours wins — the
		// precedence rule in item 35: on our own entries this command owns the
		// keys it writes, and nothing else.
		if !strings.Contains(got, `bearer_token_env_var = "`+tokenEnvVar+`"`) {
			t.Errorf("our own bearer_token_env_var was not written:\n%s", got)
		}
		if strings.Contains(got, theirBearerLine) {
			t.Errorf("a key this run DOES render was not replaced by ours:\n%s", got)
		}
		// Exactly one per server table — a merge that appends ours beside theirs
		// defines the key twice in one table, which TOML rejects.
		if n, want := strings.Count(got, "bearer_token_env_var"), len(civitaiMCPServers); n != want {
			t.Errorf("bearer_token_env_var appears %d times, want %d (one per table):\n%s", n, want, got)
		}
		// `http_headers` is NEVER rendered, token or not, so it is still theirs.
		if !strings.Contains(got, theirHeaderLine) {
			t.Errorf("the merge dropped %q, a key it never re-renders:\n%s", theirHeaderLine, got)
		}
		for _, want := range []string{"startup_timeout_sec = 30", "tool_timeout_sec = 120", `model = "gpt-5"`} {
			if !strings.Contains(got, want) {
				t.Errorf("the merge dropped %q:\n%s", want, got)
			}
		}
	})
}

// TestRepeatedRunsAreIdempotent: the per-key merge must not accumulate. Two runs
// and three runs produce the same bytes.
//
// 🔴 THE FIRST RUN STARTS FROM A FILE THIS COMMAND DID NOT WRITE, AND THAT IS
// THE POINT — round 2 found this guard testing the RENDERER's idempotence and
// calling it the MERGE's. Started from an empty project, every run after the
// first merges over this command's OWN output, which is the one input item 35's
// thesis says cannot break a merge routine ("a merge routine tested only against
// its own output is tested against the one input that cannot break it"). The
// pre-existing keys below are what make the per-key path run at all, and the
// second assertion is that they are STILL THERE after three runs — accumulating
// is one failure, eroding is the other.
//
// 🔴 AND THE PRE-FILE CARRIES A KEY THE MERGE *OWNS*, NOT ONLY KEYS IT HAS NEVER
// HEARD OF — WHICH IS THE SAME BLIND CLASS ONE LEVEL DOWN. The rewritten fixture
// used `startup_timeout_sec` (TOML) and `theirEntryKey` (JSON), both of which
// `renderTOMLServer`/`mcpEntry` emit under NO condition. Item 35 §1 names exactly
// that: "a fixture built only from never-written keys cannot distinguish 'skip
// what we re-render' from 'skip a fixed list'." Consequence, measured: the whole
// test passed on c801ab8, the tree round 2 found the bug in — an invariant guard
// wearing a regression guard's docstring. `owned` below is the discriminating
// key: it is in the merge's owned set and is NOT re-rendered on this run, which
// is the combination c801ab8 dropped.
//
// SCOPE, measured rather than assumed: with `owned` added, the CODEX subtest goes
// red on c801ab8 and the six JSON subtests still pass — round 1's JSON merge was
// already per-key, and c801ab8's defect was in `mergeTOMLBlock` alone. So the
// JSON arms remain invariant guards; the TOML arm is the regression one.
//
// 🔴 THE ASSERTIONS NAME THE KEYS. `strings.Contains(after3, "45")` matched the
// digits of a port, a timeout, or any other number anywhere in the file.
func TestRepeatedRunsAreIdempotent(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			t.Setenv("CIVITAI_TOKEN", credFixtureToken)
			target := agentTargets[agent]
			path, _ := agentConfigPath(liveAgentEnv(dir), agent)
			// A file the USER wrote, in the shape their agent uses, carrying TWO
			// keys on OUR entry: one this command never renders, and one it owns
			// but does not re-render on a run shaped like this one.
			//
			// `owned` is per format because "owned but not re-rendered" is: with a
			// token Codex re-renders `bearer_token_env_var` and never renders
			// `http_headers`; the JSON targets re-render `headers` only where the
			// vendor documents interpolation, so Zed's is owned-and-never-rendered.
			var pre, owned, unowned string
			if target.Format == formatTOML {
				owned, unowned = target.HeadersKey, "startup_timeout_sec"
				pre = "model = \"gpt-5\"\n\n[mcp_servers.\"civitai\"]\nurl = \"https://mcp.civitai.com/mcp\"\n" +
					unowned + " = 45\n" +
					owned + " = { X-Civitai-Fixture = \"round3\" }\n"
			} else {
				owned, unowned = target.HeadersKey, "theirEntryKey"
				pre = "{\n  \"theirTopLevelKey\": true,\n  " + strconv.Quote(target.ServersKey) + ": {\n" +
					"    \"civitai\": {" + strconv.Quote(target.URLKey) + ": \"https://mcp.civitai.com/mcp\", " +
					strconv.Quote(unowned) + ": 45, " +
					strconv.Quote(owned) + ": {\"X-Civitai-Fixture\": \"round3\"}}\n  }\n}\n"
			}
			writeFile(t, path, pre)

			for i := 0; i < 2; i++ {
				if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
					t.Fatalf("run %d: %v", i, err)
				}
			}
			after2 := readFile(t, path)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("run 3: %v", err)
			}
			after3 := readFile(t, path)
			if after3 != after2 {
				t.Errorf("a third run changed the file:\n--- after 2 ---\n%s\n--- after 3 ---\n%s", after2, after3)
			}
			// PREMISE + the erosion half: the user's keys are what make this a MERGE,
			// and a run that had deleted them would be idempotent about nothing. The
			// KEY NAMES are asserted, plus the value that identifies the owned key as
			// theirs rather than one this run happened to re-render.
			for _, want := range []string{unowned, owned, "X-Civitai-Fixture", "round3"} {
				if !strings.Contains(after3, want) {
					t.Errorf("%q the user put on OUR entry was gone by run 3:\n--- before ---\n%s\n"+
						"--- after ---\n%s", want, pre, after3)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4 — "configured" is not "exported"
// ---------------------------------------------------------------------------

// TestAConfigOnlyTokenIsNotReportedAsAuthenticated.
//
// 🔴 MEASURED RED: a token in ~/.config/civitai/config.yaml with CIVITAI_TOKEN
// unset produced a written `${CIVITAI_TOKEN}` header AND the line "3. Already
// authenticated" — while the variable the header references was empty, so every
// request the agent makes 401s. That reads as a bad token and sends the user to
// re-mint one.
func TestAConfigOnlyTokenIsNotReportedAsAuthenticated(t *testing.T) {
	dir, home := agentSetupProject(t)
	writeFile(t, filepath.Join(home, ".config", "civitai", "config.yaml"),
		"token: "+credFixtureToken+"\n")
	t.Setenv("CIVITAI_TOKEN", "")

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	// Premise: the header really was written, i.e. the gate really did pass on a
	// config-file-only token. Without this the assertions below could pass on a
	// run that never reached the branch under test.
	cfg := readFile(t, filepath.Join(dir, ".mcp.json"))
	if !strings.Contains(cfg, "${"+tokenEnvVar+"}") {
		t.Fatalf("PREMISE BROKEN: no interpolated header was written, so this run does not "+
			"exercise the config-only-token case:\n%s", cfg)
	}
	if strings.Contains(out, "Already authenticated") {
		t.Errorf("the run reported 'Already authenticated' while %s is unset — the agent cannot "+
			"resolve the header it just wrote:\n%s", tokenEnvVar, out)
	}
	if !strings.Contains(out, "NOT set") && !strings.Contains(out, "cannot see it") {
		t.Errorf("the run does not say %s is unexported:\n%s", tokenEnvVar, out)
	}
}

// TestAnExportedTokenIsReportedAsAuthenticated is the other direction, so the
// guard above cannot be satisfied by a command that simply stopped saying
// anything reassuring.
func TestAnExportedTokenIsReportedAsAuthenticated(t *testing.T) {
	dir, _ := agentSetupProject(t)
	t.Setenv("CIVITAI_TOKEN", credFixtureToken)

	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if !strings.Contains(out, "Already authenticated") {
		t.Errorf("an exported token was not reported as authenticated:\n%s", out)
	}
	if strings.Contains(out, "NOT set") {
		t.Errorf("an exported token was reported as unexported:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// 5 — the hand-rolled TOML parser must not refuse valid TOML
// ---------------------------------------------------------------------------

// TestTOMLParserAcceptsValidDocuments. Each case below was MEASURED as a hard
// refusal on the pre-fix tree, telling the user their valid config was broken.
func TestTOMLParserAcceptsValidDocuments(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a multi-line array", "matrix = [\n  [1, 2],\n]\n"},
		{"a nested multi-line array", "a = [\n  [\n    [1],\n  ],\n]\n"},
		{"an array of tables twice", "[[profiles]]\nname = \"a\"\n\n[[profiles]]\nname = \"b\"\n"},
		{"a header with a trailing comment", "[tools] # mine\nx = 1\n"},
		{"a bracket inside a string", "a = \"[not a header]\"\n"},
		{"a bracket inside a comment", "# [not a header]\na = 1\n"},
		{"an array spanning lines then a real table", "a = [\n  1,\n]\n[tools]\nx = 1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tc.src)

			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex); err != nil {
				t.Fatalf("valid TOML was refused: %v", err)
			}
			got := readFile(t, path)
			// The original document survives verbatim…
			if !strings.Contains(got, strings.TrimRight(tc.src, "\n")) {
				t.Errorf("the original document was not preserved:\n--- want to contain ---\n%s\n--- got ---\n%s",
					tc.src, got)
			}
			// …and both servers were appended.
			for _, srv := range civitaiMCPServers {
				if !strings.Contains(got, `[mcp_servers."`+srv.Name+`"]`) {
					t.Errorf("%s was not registered:\n%s", srv.Name, got)
				}
			}
		})
	}
}

// TestTOMLParserStillRefusesBrokenDocuments is the other direction. A parser
// loosened to accept valid TOML must not start accepting broken TOML — the
// refuse-rather-than-repair rule is what keeps a "fix" from eating a config.
func TestTOMLParserStillRefusesBrokenDocuments(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a genuinely unterminated header", "[mcp_servers\nx = 1\n", "unterminated table header"},
		{"the same table defined twice", "[tools]\na = 1\n\n[tools]\nb = 2\n", "defined twice"},
		{"our own table defined twice", "[mcp_servers.civitai]\nurl = \"a\"\n\n[mcp_servers.\"civitai\"]\nurl = \"b\"\n", "defined twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			path := filepath.Join(home, ".codex", "config.toml")
			writeFile(t, path, tc.src)

			_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex)
			if err == nil {
				t.Fatalf("broken TOML was accepted:\n%s", tc.src)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say %q: %v", tc.want, err)
			}
			if got := readFile(t, path); got != tc.src {
				t.Errorf("the refused file was modified:\n%s", got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6 — a symlinked config is followed, not replaced
// ---------------------------------------------------------------------------

// TestASymlinkedConfigIsFollowed.
//
// 🔴 MEASURED RED: `~/.codex/config.toml -> ~/dotfiles/codex.toml` came back as
// a REGULAR FILE at exit 0, with the dotfiles copy orphaned and untouched — so
// every later edit to the dotfiles repo was invisible to Codex and the user's
// version control no longer tracked the live file. Dotfile repos are the normal
// way these files are managed.
func TestASymlinkedConfigIsFollowed(t *testing.T) {
	dir, home := agentSetupProject(t)
	real := filepath.Join(home, "dotfiles", "codex.toml")
	writeFile(t, real, "model = \"gpt-5\"\n")
	link := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is no longer a symlink — the write replaced the link with a regular file and "+
			"orphaned %s", link, real)
	}
	body := readFile(t, real)
	if !strings.Contains(body, `model = "gpt-5"`) {
		t.Errorf("the dotfiles copy lost its content:\n%s", body)
	}
	for _, srv := range civitaiMCPServers {
		if !strings.Contains(body, `[mcp_servers."`+srv.Name+`"]`) {
			t.Errorf("%s did not reach the real file:\n%s", srv.Name, body)
		}
	}
}

// TestABrokenSymlinkIsRefusedByName: following a link must not degrade into
// materialising a file where the user pointed one somewhere else.
func TestABrokenSymlinkIsRefusedByName(t *testing.T) {
	dir, home := agentSetupProject(t)
	link := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone", "codex.toml"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex)
	if err == nil {
		t.Fatal("a broken symlink was silently replaced with a regular file")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("the refusal does not name the symlink: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 7/8 — config roots the vendor documents
// ---------------------------------------------------------------------------

// TestCodexHomeIsTheConfigRoot.
//
// 🔴 MEASURED RED: the path was `$HOME/.codex/config.toml` unconditionally, so a
// user who set CODEX_HOME got the servers written into a file Codex never opens
// and `--check` reported "not registered" forever with nothing to explain it.
func TestCodexHomeIsTheConfigRoot(t *testing.T) {
	dir, home := agentSetupProject(t)
	codexHome := filepath.Join(home, "elsewhere", "codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	want := filepath.Join(codexHome, "config.toml")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("nothing was written to CODEX_HOME (%s): %v", want, err)
	}
	// 🔴 AND NOTHING WAS WRITTEN TO THE DEFAULT. A run that wrote BOTH would pass
	// the assertion above while still leaving a stray file — and, worse, would
	// keep the default path "working" so nobody noticed the override was ignored.
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); err == nil {
		t.Error("the default $HOME/.codex/config.toml was written too — CODEX_HOME replaces it, " +
			"it does not add to it")
	}
	// --check agrees, which is the half the user actually sees.
	if _, _, err := run(t, "agent-setup", "--check", "--dir", dir, "--agent", agentCodex); err != nil {
		if !strings.Contains(err.Error(), "agent setup incomplete") {
			t.Fatalf("--check: %v", err)
		}
		// agents-md/claude-md are present from the write above, so any failure
		// here means the MCP rows did not find the file.
		out, _, _ := run(t, "agent-setup", "--check", "--json", "--dir", dir, "--agent", agentCodex)
		t.Errorf("--check did not find the servers in CODEX_HOME:\n%s", out)
	}
}

// TestZedWindowsPathUsesAppData covers a path that ships in every release
// (windows/amd64 and windows/arm64) and that no maintainer's own run exercises.
// agentConfigPath is pure over agentEnv, which is what makes it testable here.
func TestZedWindowsPathUsesAppData(t *testing.T) {
	env := agentEnv{
		Vars:   map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`},
		Home:   `C:\Users\u`,
		Dir:    `C:\proj`,
		GOOS:   "windows",
		Exists: func(string) bool { return false },
	}
	got, ok := agentConfigPath(env, agentZed)
	if !ok {
		t.Fatal("no Zed config path on windows")
	}
	want := filepath.Join(`C:\Users\u\AppData\Roaming`, "Zed", "settings.json")
	if got != want {
		t.Errorf("zed windows path = %q, want %q", got, want)
	}

	// The same APPDATA on Linux must NOT move the path: a Linux user with APPDATA
	// set (wine, a CI image) keeps ~/.config/zed.
	env.GOOS = "linux"
	got, _ = agentConfigPath(env, agentZed)
	if !strings.Contains(got, filepath.Join(".config", "zed")) {
		t.Errorf("zed linux path = %q — a windows-only root leaked onto linux", got)
	}
}

// TestCodexHomeIsNotConsultedOnATargetThatDoesNotDeclareIt guards the direction
// a shared helper makes easy to get wrong: roots are per target, not global.
func TestCodexHomeIsNotConsultedOnATargetThatDoesNotDeclareIt(t *testing.T) {
	env := agentEnv{
		Vars:   map[string]string{"CODEX_HOME": "/elsewhere"},
		Home:   "/home/u",
		Dir:    "/proj",
		GOOS:   "linux",
		Exists: func(string) bool { return false },
	}
	got, _ := agentConfigPath(env, agentWindsurf)
	if strings.Contains(got, "elsewhere") {
		t.Errorf("windsurf's path honoured CODEX_HOME: %q", got)
	}
}

// TestWindsurfRowNamesTheDevinSplit: the vendor page this row cites now opens
// "The MCP configuration on this page applies to the legacy Cascade agent only.
// The Devin Local agent — the default agent for new tabs — configures MCP servers
// in the Devin CLI config files instead." We still write the Cascade path; a user
// on the current default would otherwise get a silently-ignored file.
func TestWindsurfRowNamesTheDevinSplit(t *testing.T) {
	caveat := agentTargets[agentWindsurf].Caveat
	if caveat == "" {
		t.Fatal("the windsurf row carries no caveat — the file it writes is the LEGACY agent's")
	}
	for _, want := range []string{"legacy", "devin", "mcp_config.json"} {
		if !strings.Contains(strings.ToLower(caveat), want) {
			t.Errorf("the windsurf caveat does not mention %q: %s", want, caveat)
		}
	}
	// And it reaches the user, not just the source.
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentWindsurf)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if !strings.Contains(out, "Devin") {
		t.Errorf("the run never tells a Windsurf user about the Devin Local agent:\n%s", out)
	}
}

// TestVSCodeGetsTheDocumentedInterpolation reverses what item 34 originally
// recorded, and it is recorded as a reversal rather than a correction.
//
// 🔴 THE EVIDENCE IS IMPLEMENTATION, NOT DOCS, AND THAT IS WHY IT OVERRULES THE
// DOCS-BASED READING THAT SHIPPED. microsoft/vscode#245237 ("Support
// ${env:VARIABLE_NAME} in mcp.json") is closed COMPLETED with connor4312 (MEMBER)
// commenting "This is supported."; #264448 adds "The format is
// ${env:VARIABLE_NAME}" and "It works using the same logic as
// tasks.json/launch.json do". In the source, `_replaceVariablesInLaunch` parses
// `McpServerLaunch.toSerialized(launch)` — the identity function — and resolves
// it whole; `ConfigurationResolverExpression.parseObject` recurses into arrays
// and objects; an HTTP launch's `headers` is `[string, string][]` built from
// `configuration.headers`; and `variableReplacement` is set for every mcp.json
// server, http included.
func TestVSCodeGetsTheDocumentedInterpolation(t *testing.T) {
	if got := agentTargets[agentVSCode].EnvHeaderSyntax; got != "${env:"+tokenEnvVar+"}" {
		t.Fatalf("vscode EnvHeaderSyntax = %q, want ${env:%s}", got, tokenEnvVar)
	}
	dir, _ := agentSetupProject(t)
	t.Setenv("CIVITAI_TOKEN", credFixtureToken)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentVSCode); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".vscode", "mcp.json"))), &root); err != nil {
		t.Fatalf("bad vscode config: %v", err)
	}
	section, _ := root["servers"].(map[string]any)
	for _, srv := range civitaiMCPServers {
		entry, _ := section[srv.Name].(map[string]any)
		headers, _ := entry["headers"].(map[string]any)
		if got := headers["Authorization"]; got != "Bearer ${env:"+tokenEnvVar+"}" {
			t.Errorf("%s Authorization = %v", srv.Name, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 9 — claude-md is not a gate for an agent that never reads it
// ---------------------------------------------------------------------------

// TestClaudeMDDoesNotFailTheVerdictForANonClaudeAgent.
//
// 🔴 MEASURED RED: a Cursor user who deleted the shim they were never going to
// use got `ok: false` and exit 1 with everything else green — and that is what
// `developer.civitai.com`'s hosted prompt reports as a failed setup. Same shape
// as `authenticated` and as the absent Authorization header: a gate nobody can
// clear is a gate everyone learns to ignore.
// `other` is deliberately NOT in this loop: its two MCP rows fail by design (the
// CLI has no config file for it), so the verdict is false either way and the
// case could not discriminate. TestAgentSetupVerdictUnit covers it directly.
func TestClaudeMDDoesNotFailTheVerdictForANonClaudeAgent(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup: %v", err)
			}
			// The user deletes the shim their agent does not read.
			if err := os.Remove(filepath.Join(dir, claudeFilename)); err != nil {
				t.Fatal(err)
			}
			out, _, err := run(t, "agent-setup", "--check", "--json", "--dir", dir, "--agent", agent)
			var payload agentSetupJSON
			if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
				t.Fatalf("bad --check json: %v\n%s", jsonErr, out)
			}
			// Premise: the row really is red, so this test is not passing because
			// the row silently went green.
			var claudeRow *agentCheckJSON
			for i := range payload.Checks {
				if payload.Checks[i].Name == checkClaudeMD {
					claudeRow = &payload.Checks[i]
				}
			}
			if claudeRow == nil {
				t.Fatalf("the %s row was dropped from the report — it must still be REPORTED", checkClaudeMD)
			}
			if claudeRow.OK {
				t.Fatalf("PREMISE BROKEN: %s is ok despite the file being deleted", checkClaudeMD)
			}
			if agent == agentClaude {
				if payload.OK || err == nil {
					t.Error("a missing CLAUDE.md must still fail the verdict for Claude Code")
				}
				return
			}
			if !payload.OK {
				t.Errorf("ok = false for %s, whose only failing row is the inert CLAUDE.md shim:\n%s", agent, out)
			}
			if err != nil {
				t.Errorf("exit non-zero for %s on an otherwise complete setup: %v", agent, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 10 — a detection marker must be a write target
// ---------------------------------------------------------------------------

// TestOpencodeJSONCIsTheWriteTargetWhenItExists.
//
// 🔴 MEASURED RED: `opencode.jsonc` DETECTS opencode (it is in agentMarkers) and
// was never a write target, so a project carrying only that file was detected
// correctly and then had a SECOND file, `opencode.json`, created beside it — with
// the servers in the one the user does not maintain.
func TestOpencodeJSONCIsTheWriteTargetWhenItExists(t *testing.T) {
	dir, _ := agentSetupProject(t)
	jsonc := filepath.Join(dir, "opencode.jsonc")
	writeFile(t, jsonc, "// my opencode config\n{ \"theme\": \"dark\" }\n")

	// No --agent: the marker is what detects opencode, which is the whole setup.
	if _, _, err := run(t, "agent-setup", "--dir", dir); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); err == nil {
		t.Error("a second config file opencode.json was created beside the opencode.jsonc the user maintains")
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(readFile(t, jsonc)), &root); err != nil {
		t.Fatalf("the merged opencode.jsonc is not valid JSON: %v", err)
	}
	if root["theme"] != "dark" {
		t.Errorf("the user's key was lost: %v", root["theme"])
	}
	section, _ := root["mcp"].(map[string]any)
	for _, srv := range civitaiMCPServers {
		if _, ok := section[srv.Name]; !ok {
			t.Errorf("%s was not registered in opencode.jsonc", srv.Name)
		}
	}
}

// TestOpencodeJSONIsStillTheDefaultTarget: preferring an existing `.jsonc` must
// not stop a fresh project getting `opencode.json`, the documented default.
func TestOpencodeJSONIsStillTheDefaultTarget(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentOpencode); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opencode.json")); err != nil {
		t.Errorf("a fresh project did not get opencode.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opencode.jsonc")); err == nil {
		t.Error("a .jsonc file was invented for a project that had none")
	}
}

// TestEveryDetectionMarkerIsReachableAsAWriteTarget is the LEDGER behind finding
// 10, rather than a second copy of the one case that was wrong.
//
// 🔴 THE DEFECT WAS STRUCTURAL: a filename could DETECT an agent without ever
// being a file that agent's target would write. Any marker that names a config
// file must therefore be either the target's Parts or one of its PreferExisting
// spellings. Directory markers (`.claude`, `.cursor`, `.vscode`, `.zed`) are
// exempt — they are not files anything writes to.
func TestEveryDetectionMarkerIsReachableAsAWriteTarget(t *testing.T) {
	for _, m := range agentMarkers {
		if !strings.Contains(m.Rel, ".") || strings.HasPrefix(m.Rel, ".") && !strings.Contains(m.Rel[1:], ".") {
			continue // a directory marker like `.claude`, `.cursor`, `.zed`
		}
		target, known := agentTargets[m.Agent]
		if !known {
			t.Errorf("marker %q detects %s, which has no target at all", m.Rel, m.Agent)
			continue
		}
		candidates := append([][]string{target.Parts}, target.PreferExisting...)
		found := false
		for _, c := range candidates {
			if filepath.Join(c...) == m.Rel {
				found = true
			}
		}
		if !found {
			t.Errorf("marker %q detects %s but is never a write target for it (Parts=%v, PreferExisting=%v) — "+
				"a project carrying only that file gets a SECOND config file created beside it",
				m.Rel, m.Agent, target.Parts, target.PreferExisting)
		}
	}
}

// ---------------------------------------------------------------------------
// 12 / 13 — the managed-block refusals
// ---------------------------------------------------------------------------

// TestMarkersInTheWrongOrderSayThatAndNotSomethingFalse.
//
// 🔴 MEASURED RED: with END above BEGIN, both markers ARE present, so the shared
// refusal read "BEGIN present: true, END present: true — restore both marker
// lines" and sent the reader after a marker sitting right in front of them.
func TestMarkersInTheWrongOrderSayThatAndNotSomethingFalse(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, agentsFilename)
	writeFile(t, path, "# Mine\n\n"+agentsEndMarker+"\nstuff\n"+agentsBeginMarker+"\n")

	_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err == nil {
		t.Fatal("an out-of-order pair of markers was accepted")
	}
	msg := err.Error()
	if !strings.Contains(msg, "wrong order") {
		t.Errorf("the refusal does not say the markers are out of order: %v", err)
	}
	if strings.Contains(msg, "restore both marker lines") {
		t.Errorf("the refusal still tells the reader to restore markers that are both present: %v", err)
	}
	// The half-block refusals must keep saying what they say.
	writeFile(t, path, "# Mine\n\n"+agentsBeginMarker+"\nstuff\n")
	_, _, err = run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err == nil {
		t.Fatal("a BEGIN with no END was accepted")
	}
	if !strings.Contains(err.Error(), "only one half") {
		t.Errorf("the half-block refusal changed shape: %v", err)
	}
}

// TestASecondManagedBlockIsDetected.
//
// 🔴 MEASURED RED: only the FIRST block is ever refreshed, so a duplicate keeps
// instructing the agent with whatever the template said when it was written —
// forever — while `--check` reported `ok: true` because a block was found. A
// stale instruction file that reports healthy is the worst of the three states.
func TestASecondManagedBlockIsDetected(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, agentsFilename)
	block := agentsBeginMarker + "\nold text\n" + agentsEndMarker
	writeFile(t, path, "# Mine\n\n"+block+"\n\nmore prose\n\n"+block+"\n")

	// --check must NOT call this healthy.
	out, _, err := run(t, "agent-setup", "--check", "--json", "--dir", dir, "--agent", agentClaude)
	if err == nil {
		t.Error("--check reported a file with two managed blocks as complete")
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("bad --check json: %v\n%s", jsonErr, out)
	}
	if payload.OK {
		t.Error("ok = true for a file with two managed blocks")
	}
	var row *agentCheckJSON
	for i := range payload.Checks {
		if payload.Checks[i].Name == checkAgentsMD {
			row = &payload.Checks[i]
		}
	}
	if row == nil || row.OK {
		t.Fatalf("the %s row did not fail:\n%s", checkAgentsMD, out)
	}
	if !strings.Contains(row.Detail, "MORE THAN ONE") {
		t.Errorf("the detail does not name the duplicate: %s", row.Detail)
	}

	// …and a write run refuses rather than silently refreshing only the first.
	_, _, err = run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err == nil {
		t.Fatal("a write run silently accepted a file with two managed blocks")
	}
	if !strings.Contains(err.Error(), "managed blocks") {
		t.Errorf("the refusal does not name the duplicate: %v", err)
	}
	if got := readFile(t, path); strings.Count(got, agentsBeginMarker) != 2 {
		t.Errorf("the refused file was modified:\n%s", got)
	}
}

// TestAgentsMDBlockStateClassifies is the unit under both guards, so a wrong
// classification is caught where it is decided rather than only through a run.
func TestAgentsMDBlockStateClassifies(t *testing.T) {
	b, e := agentsBeginMarker, agentsEndMarker
	for _, tc := range []struct {
		name string
		in   string
		want agentsBlockState
	}{
		{"empty", "", blockAbsent},
		{"prose only", "# hi\n", blockAbsent},
		{"one well-ordered block", "x\n" + b + "\ny\n" + e + "\nz\n", blockPresent},
		{"begin only", b + "\ny\n", blockPartial},
		{"end only", "y\n" + e + "\n", blockPartial},
		{"end before begin", e + "\ny\n" + b + "\n", blockPartial},
		{"two blocks", b + "\n1\n" + e + "\n" + b + "\n2\n" + e + "\n", blockDuplicated},
		{"two begins one end", b + "\n1\n" + b + "\n2\n" + e + "\n", blockDuplicated},
		{"one begin two ends", b + "\n1\n" + e + "\n2\n" + e + "\n", blockDuplicated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentsMDBlockState(tc.in); got != tc.want {
				t.Errorf("agentsMDBlockState = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 15 — --json paths are absolute
// ---------------------------------------------------------------------------

// TestJSONPathsAreAbsoluteUnderTheDefaultDir.
//
// 🔴 MEASURED RED: with the default `--dir .` the payload's `path` fields came
// out relative (`AGENTS.md`, `.mcp.json`) while the README's documented example
// shows absolute ones. A path a consumer cannot resolve without also knowing the
// CLI's working directory is not a path.
func TestJSONPathsAreAbsoluteUnderTheDefaultDir(t *testing.T) {
	dir, _ := agentSetupProject(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	out, _, err := run(t, "agent-setup", "--json", "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("bad json: %v\n%s", jsonErr, out)
	}
	if len(payload.Changes) == 0 {
		t.Fatalf("PREMISE BROKEN: no changes to inspect:\n%s", out)
	}
	for _, c := range payload.Changes {
		if c.Path == "" {
			continue // `manual` rows have no path, by design
		}
		if !filepath.IsAbs(c.Path) {
			t.Errorf("changes[].path is relative: %q", c.Path)
		}
	}

	out, _, err = run(t, "agent-setup", "--check", "--json", "--agent", agentClaude)
	if err != nil {
		t.Fatalf("--check: %v", err)
	}
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("bad json: %v\n%s", jsonErr, out)
	}
	for _, c := range payload.Checks {
		if c.Name != checkAgentsMD && c.Name != checkClaudeMD {
			continue
		}
		if !c.OK {
			continue
		}
		if !filepath.IsAbs(strings.TrimSpace(c.Detail)) {
			t.Errorf("checks[%s].detail is not an absolute path: %q", c.Name, c.Detail)
		}
	}
}
