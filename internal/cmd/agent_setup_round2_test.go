package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The round-2 audit guards for `civitai agent-setup`.
//
// 🔴 EVERY TEST BELOW WAS WATCHED RED ON c801ab8 — the tip of the round-1 fix —
// AND GREEN AFTER. They are regression tests, not invariant guards: each names a
// behaviour that was MEASURED wrong by running the binary against a file a real
// install has. The red-then-green matrix is in the PR.
//
// 🔴 THEY ARE ALL BLACK-BOX ON PURPOSE. Every assertion below drives the command
// through `run` and reads the file or the payload it produced, so the same test
// source compiles and runs against the PRE-FIX tree. A guard that references a
// symbol the fix introduced cannot be watched red — it fails to compile, which
// is not the same observation.

// ---------------------------------------------------------------------------
// 1 — the published merge claim, over EVERY agent
// ---------------------------------------------------------------------------

// TestThePublishedMergeClaimHoldsForEveryAgent is the LEDGER behind round 2's
// first finding rather than a second copy of the one case that was wrong.
//
// 🔴 README.md publishes "merged into **key by key** (a header you added to a
// Civitai entry survives)". Round 1 made that true of the JSON agents and left
// it FALSE for Codex: `mergeTOMLBlock` skipped a FIXED set of owned keys while
// the renderer emits `http_headers` never and `bearer_token_env_var` only with a
// token, so a key that was not re-rendered was dropped with nothing put back.
// Measured on c801ab8, rc 0, `--check` still `ok: true`.
//
// So the claim is asserted over the whole table, in each agent's OWN spelling,
// and a new agent is covered the moment it is added — which is what matters,
// since the defect was per-format and the guard aimed at one format.
func TestThePublishedMergeClaimHoldsForEveryAgent(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			target := agentTargets[agent]
			path, ok := agentConfigPath(liveAgentEnv(dir), agent)
			if !ok {
				t.Fatalf("no config path for %s", agent)
			}
			const theirs = "Bearer sk-hand-added-to-our-entry-6620"
			// The header goes on OUR entry, in the shape that agent's own file uses.
			// `theirs` is a synthetic fixture, never a real credential.
			var body string
			if target.Format == formatTOML {
				body = "[" + target.ServersKey + "." + strconv.Quote(civitaiMCPServers[0].Name) + "]\n" +
					target.URLKey + " = " + strconv.Quote(civitaiMCPServers[0].URL) + "\n" +
					target.HeadersKey + " = { Authorization = " + strconv.Quote(theirs) + " }\n"
			} else {
				raw, err := json.Marshal(map[string]any{
					target.ServersKey: map[string]any{
						civitaiMCPServers[0].Name: map[string]any{
							target.URLKey:     civitaiMCPServers[0].URL,
							target.HeadersKey: map[string]any{"Authorization": theirs},
						},
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				body = string(raw)
			}
			writeFile(t, path, body)

			// No token: this run renders no header of its own, so nothing it writes
			// can legitimately replace theirs.
			t.Setenv("CIVITAI_TOKEN", "")
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup: %v", err)
			}
			got := readFile(t, path)
			if !strings.Contains(got, theirs) {
				t.Errorf("the header the user added to OUR entry was deleted, and this command wrote "+
					"nothing in its place:\n%s", got)
			}
			// PREMISE: the run really did register both servers, so the assertion
			// above is not passing on a run that did nothing at all.
			for _, srv := range civitaiMCPServers {
				if !strings.Contains(got, srv.URL) {
					t.Fatalf("PREMISE BROKEN: %s was not registered, so nothing was merged:\n%s", srv.Name, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4 — the run must not describe a header it just preserved as absent
// ---------------------------------------------------------------------------

// TestARunWithoutATokenDoesNotCallAPreservedHeaderAbsent.
//
// 🔴 MEASURED RED: run once with a token, then re-run in a shell without one —
// CI, a second machine, an expired login. The merge correctly KEEPS
// `"Authorization": "Bearer ${CIVITAI_TOKEN}"` (that is round 1's own fix
// working) while the run printed "No token is configured, so no Authorization
// header was written" and "civitai-orchestration returns 401 until an
// Authorization header is present", in the terminal output and in `--json`'s
// `reason` alike. The first sentence is true of the RUN and misleading about the
// FILE; the second is a claim about what the REGISTRATION REACHES, and it is
// wrong about the file that run had just written.
func TestARunWithoutATokenDoesNotCallAPreservedHeaderAbsent(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, ".mcp.json")

	// Run 1: a token is configured, so the env-var reference is written.
	t.Setenv("CIVITAI_TOKEN", credFixtureToken)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("agent-setup with a token: %v", err)
	}

	// Run 2: the same project, a shell with no token at all.
	t.Setenv("CIVITAI_TOKEN", "")
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup without a token: %v", err)
	}

	// PREMISE: the header really did survive, so the assertions below are about
	// the wording and not about a merge that quietly deleted it.
	var root map[string]any
	if jsonErr := json.Unmarshal([]byte(readFile(t, path)), &root); jsonErr != nil {
		t.Fatalf("merged config is not valid JSON: %v", jsonErr)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	for _, srv := range civitaiMCPServers {
		entry, _ := servers[srv.Name].(map[string]any)
		headers, _ := entry["headers"].(map[string]any)
		if got, _ := headers["Authorization"].(string); got == "" {
			t.Fatalf("PREMISE BROKEN: %s lost its Authorization header, so this run does not exercise "+
				"the preserved-header case:\n%s", srv.Name, readFile(t, path))
		}
	}

	if strings.Contains(out, "so no Authorization header was written") {
		t.Errorf("the run says no header was written about a file it just wrote one into:\n%s", out)
	}
	if strings.Contains(out, "returns 401 until an Authorization header is present") {
		t.Errorf("the run claims the registration 401s while every entry in it carries a header:\n%s", out)
	}
	if !strings.Contains(out, "already carries an Authorization header") {
		t.Errorf("the run never says the header that IS there is there:\n%s", out)
	}

	// `--json`'s reason is the same claim through the other channel, and it was
	// wrong in the same words.
	jsonOut, _, err := run(t, "agent-setup", "--json", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup --json: %v", err)
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(jsonOut), &payload); jsonErr != nil {
		t.Fatalf("bad json: %v\n%s", jsonErr, jsonOut)
	}
	var reason string
	for _, c := range payload.Changes {
		if c.Path == path {
			reason = c.Reason
		}
	}
	if reason == "" {
		t.Fatalf("no changes row for %s:\n%s", path, jsonOut)
	}
	if strings.Contains(reason, "returns 401 until an Authorization header is present") {
		t.Errorf("--json's reason claims the registration 401s while it carries a header: %s", reason)
	}
	if !strings.Contains(reason, "already carries an Authorization header") {
		t.Errorf("--json's reason does not say the header that IS there is there: %s", reason)
	}
}

// TestARunWithoutATokenAndWithoutAHeaderStillSaysSo is the OTHER direction, so
// the guard above cannot be satisfied by a command that simply stopped saying
// anything about what a header-less registration reaches. This is the case item
// 34 exists for and its wording must survive unchanged.
func TestARunWithoutATokenAndWithoutAHeaderStillSaysSo(t *testing.T) {
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	// PREMISE: nothing wrote a header, so this really is the header-less case.
	if strings.Contains(strings.ToLower(readFile(t, filepath.Join(dir, ".mcp.json"))), "authorization") {
		t.Fatal("PREMISE BROKEN: a header was written on a run with no token")
	}
	if !strings.Contains(out, "no Authorization header was written") {
		t.Errorf("a genuinely header-less run stopped saying so:\n%s", out)
	}
	if !strings.Contains(out, "returns 401 until an Authorization header is present") {
		t.Errorf("a genuinely header-less run no longer names the server it cannot reach:\n%s", out)
	}
}

// TestZedIsToldAboutTheHeaderItWasAskedToAddByHand covers the arm that is most
// likely to be describing a file that already has a header: Zed gets no
// interpolation, so the run TELLS the user to add one themselves — and the next
// run then described that file as unreachable.
func TestZedIsToldAboutTheHeaderItWasAskedToAddByHand(t *testing.T) {
	dir, home := agentSetupProject(t)
	settings := filepath.Join(home, ".config", "zed", "settings.json")
	entries := map[string]any{}
	for _, srv := range civitaiMCPServers {
		entries[srv.Name] = map[string]any{
			"url":     srv.URL,
			"headers": map[string]any{"Authorization": "Bearer sk-zed-user-added-by-hand-4417"},
		}
	}
	body, err := json.Marshal(map[string]any{"context_servers": entries})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, settings, string(body))

	t.Setenv("CIVITAI_TOKEN", credFixtureToken)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed)
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if !strings.Contains(readFile(t, settings), "sk-zed-user-added-by-hand-4417") {
		t.Fatalf("PREMISE BROKEN: the hand-added header was not preserved:\n%s", readFile(t, settings))
	}
	if strings.Contains(out, "Access without one:") {
		t.Errorf("the run describes a file carrying the very header it told the user to add as having "+
			"none:\n%s", out)
	}
	if !strings.Contains(out, "already carries an Authorization header") {
		t.Errorf("the run never acknowledges the header that is there:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// 5 — a trailing comma is JSONC too
// ---------------------------------------------------------------------------

// TestATrailingCommaIsToleratedLikeAComment.
//
// 🔴 MEASURED RED: `{ "theme": "One Dark", }` as Zed's settings.json exited 1
// with "does not parse (invalid character '}' …) — fix or move that file". That
// is this command's own stated failure mode — refusing while blaming the user —
// on a file Zed reads happily: Zed parses settings through
// `serde_json_lenient`, which accepts comments AND trailing commas by default,
// and VS Code strips both in `src/vs/base/common/jsonc.ts` before `JSON.parse`.
// Neither editor was RUN; the claim rests on those two sources.
func TestATrailingCommaIsToleratedLikeAComment(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		// dropsFormatting is whether this fixture actually LOSES something in the
		// round trip, and it is per case rather than asserted for all of them: the
		// last fixture is the CONTROL, a file with no comment and no trailing comma
		// at all, and a run that announced a loss there would be announcing it
		// unconditionally — which is indistinguishable from announcing it correctly.
		dropsFormatting bool
		// wantValue, when set, is a key whose value must survive byte-for-byte.
		wantKey, wantValue string
	}{
		{name: "an object's trailing comma", src: "{\n  \"theme\": \"One Dark\",\n}\n", dropsFormatting: true},
		{name: "an array's trailing comma", src: "{\n  \"a\": [1, 2,],\n  \"theme\": \"One Dark\"\n}\n",
			dropsFormatting: true},
		{name: "nested closers", src: "{\n  \"a\": {\"b\": [1,],},\n  \"theme\": \"One Dark\"\n}\n",
			dropsFormatting: true},
		{name: "a comment between the comma and the brace", src: "{\n  \"theme\": \"One Dark\", // mine\n}\n",
			dropsFormatting: true},
		{name: "a comma before a closer INSIDE a string is not one",
			src: "{\n  \"theme\": \"One Dark\",\n  \"x\": \"a,]\",\n}\n", dropsFormatting: true,
			wantKey: "x", wantValue: "a,]"},
		{name: "CONTROL: no comment, no trailing comma", src: "{\n  \"theme\": \"One Dark\"\n}\n",
			dropsFormatting: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			settings := filepath.Join(home, ".config", "zed", "settings.json")
			writeFile(t, settings, tc.src)

			out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed)
			if err != nil {
				t.Fatalf("a JSONC file both Zed and VS Code accept was refused: %v", err)
			}
			var root map[string]any
			if jsonErr := json.Unmarshal([]byte(readFile(t, settings)), &root); jsonErr != nil {
				t.Fatalf("the merged settings.json is not valid JSON: %v", jsonErr)
			}
			if root["theme"] != "One Dark" {
				t.Errorf("the user's key was lost: %v", root["theme"])
			}
			if tc.wantKey != "" && root[tc.wantKey] != tc.wantValue {
				t.Errorf("a comma inside a STRING was blanked: %s = %v, want %q",
					tc.wantKey, root[tc.wantKey], tc.wantValue)
			}
			section, _ := root["context_servers"].(map[string]any)
			for _, srv := range civitaiMCPServers {
				if _, ok := section[srv.Name]; !ok {
					t.Errorf("%s was not registered:\n%s", srv.Name, readFile(t, settings))
				}
			}
			// 🔴 THE LOSS IS STATED, AND ONLY WHEN THERE IS ONE. The re-encode drops
			// the trailing comma along with the comments, and a change reason that
			// named only comments would say nothing at all about a file whose only
			// JSONC feature is a comma. The CONTROL row is the other half: a file
			// that loses nothing must not be told it lost something.
			said := strings.Contains(out, "trailing commas")
			if tc.dropsFormatting && !said {
				t.Errorf("the run does not say the trailing comma is not preserved:\n%s", out)
			}
			if !tc.dropsFormatting && said {
				t.Errorf("the run announces a formatting loss for a file that has none:\n%s", out)
			}
		})
	}
}

// TestATrailingCommaDoesNotMakeRubbishAcceptable is the direction a leniency fix
// is most likely to break. Refuse-rather-than-repair is still the rule, and a
// parser loosened for one JSONC form must not start accepting broken JSON.
func TestATrailingCommaDoesNotMakeRubbishAcceptable(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a doubled comma", "{\n  \"a\": [1,,],\n}\n"},
		{"a leading comma", "{\n  ,\"a\": 1\n}\n"},
		{"truncated after the comma", "{\n  \"a\": 1,\n"},
		{"a comma with no container", "{\n  \"a\": 1\n},\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			settings := filepath.Join(home, ".config", "zed", "settings.json")
			writeFile(t, settings, tc.src)

			_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentZed)
			if err == nil {
				t.Fatalf("broken JSON was accepted:\n%s", tc.src)
			}
			if !strings.Contains(err.Error(), "does not parse") {
				t.Errorf("the refusal does not name the parse failure: %v", err)
			}
			if got := readFile(t, settings); got != tc.src {
				t.Errorf("the refused file was rewritten:\n%s", got)
			}
		})
	}
}

// TestAStrictAgentStaysStrictAboutTrailingCommas: tolerating a trailing comma is
// gated on the agent's own parser, exactly as tolerating a comment is. Claude
// Code's `.mcp.json` is plain JSON.
func TestAStrictAgentStaysStrictAboutTrailingCommas(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, ".mcp.json")
	const src = "{\n  \"mcpServers\": {},\n}\n"
	writeFile(t, path, src)

	_, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err == nil {
		t.Fatal("a trailing comma was tolerated for an agent whose parser is strict JSON")
	}
	if got := readFile(t, path); got != src {
		t.Errorf("the refused file was rewritten:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// 6 — `--json` emits a payload for EVERY outcome, not only the plan-time one
// ---------------------------------------------------------------------------

// TestJSONIsEmittedForEveryFailureShape.
//
// 🔴 MEASURED RED, FOUR TRIGGERS AND THREE DIFFERENT CONTRACTS ON ONE FLAG. A
// plan-time refusal emitted the full payload with a `blocked` row; a write-time
// refusal, a write-time FAILURE and `--check --json` against an unparseable
// config each emitted ZERO BYTES on stdout at exit 1. A consumer facing the
// silent ones cannot tell a partial run from a usage error — and the fourth is
// what `developer.civitai.com`'s hosted prompt reads.
func TestJSONIsEmittedForEveryFailureShape(t *testing.T) {
	t.Run("plan-time refusal", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		writeFile(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers\nx = 1\n")
		assertBlockedWritePayload(t, dir, agentCodex, "does not parse")
	})

	t.Run("write-time refusal: a broken symlink destination", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		link := filepath.Join(home, ".codex", "config.toml")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, "gone", "codex.toml"), link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		assertBlockedWritePayload(t, dir, agentCodex, "symlink")
	})

	t.Run("write-time failure: an unwritable directory", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root — a 0500 directory is still writable")
		}
		dir, home := agentSetupProject(t)
		codexDir := filepath.Join(home, ".codex")
		if err := os.MkdirAll(codexDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(codexDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(codexDir, 0o700) })
		assertBlockedWritePayload(t, dir, agentCodex, "permission denied")
	})

	t.Run("--check --json on an unparseable config", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		writeFile(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers\nx = 1\n")

		out, _, err := run(t, "agent-setup", "--check", "--json", "--dir", dir, "--agent", agentCodex)
		if err == nil {
			t.Error("--check reported a config it could not read as complete")
		}
		var payload agentSetupJSON
		if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
			t.Fatalf("--check --json emitted no readable payload (%v):\n%q", jsonErr, out)
		}
		if payload.OK {
			t.Error("ok = true for a config that could not be read")
		}
		// 🔴 "COULD NOT LOOK" IS NOT "NOT REGISTERED", AND THE DETAIL SAYS WHICH —
		// the same rule the other two could-not-look cases already follow.
		for _, srv := range civitaiMCPServers {
			var row *agentCheckJSON
			for i := range payload.Checks {
				if payload.Checks[i].Name == srv.Check {
					row = &payload.Checks[i]
				}
			}
			if row == nil {
				t.Fatalf("the %s row is missing entirely:\n%s", srv.Check, out)
			}
			if row.OK {
				t.Errorf("%s is ok against a config that does not parse", srv.Check)
			}
			if !strings.Contains(row.Detail, "does not parse") {
				t.Errorf("%s does not say WHY it could not answer: %s", srv.Check, row.Detail)
			}
			if strings.Contains(row.Detail, "not registered in") {
				t.Errorf("%s reports 'not registered' for a file nothing could read: %s", srv.Check, row.Detail)
			}
		}
	})
}

// assertBlockedWritePayload is the shared half of the three write-run cases: a
// payload on stdout, `ok: false`, a non-zero exit, a `blocked` row carrying the
// reason — and the instruction files written anyway, because they have nothing
// to do with the MCP config.
func assertBlockedWritePayload(t *testing.T, dir, agent, wantReason string) {
	t.Helper()
	out, _, err := run(t, "agent-setup", "--json", "--dir", dir, "--agent", agent)
	if err == nil {
		t.Fatal("a run that wrote no MCP config exited 0")
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("--json emitted no readable payload (%v):\n%q", jsonErr, out)
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
		t.Fatalf("no %q row explaining what did not happen:\n%s", actionBlocked, out)
	}
	if !strings.Contains(blocked.Reason, wantReason) {
		t.Errorf("the blocked row does not carry the reason %q: %s", wantReason, blocked.Reason)
	}
	if blocked.Path == "" {
		t.Error("the blocked row names no path")
	}
	for _, name := range []string{agentsFilename, claudeFilename} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Errorf("%s was not written despite the MCP failure being unrelated to it: %v", name, statErr)
		}
	}
	// 🔴 EVERY ROW IS TRUE OF WHAT HAPPENED. The old code returned on the FIRST
	// write failure, so any payload emitted after one would have claimed actions
	// for files nothing ever tried to write.
	for _, c := range payload.Changes {
		if c.Action == actionBlocked || c.Path == "" {
			continue
		}
		if _, statErr := os.Stat(c.Path); statErr != nil {
			t.Errorf("changes[] claims %q for %s, which does not exist: %v", c.Action, c.Path, statErr)
		}
	}
}

// ---------------------------------------------------------------------------
// 8 — --dry-run must not report an action the real run refuses
// ---------------------------------------------------------------------------

// TestDryRunReportsADestinationTheRealRunRefuses.
//
// 🔴 MEASURED RED: with a broken-symlink `~/.codex/config.toml`, `--dry-run`
// reported action `create`, `ok: true` and exit 0 for a destination the real run
// refuses by name. The file header claims dry-run "cannot report a path or an
// action the write path would not take" — true for the refusals that sat in the
// planner, blind to the one that sat in the writer. Resolving the destination is
// a pure read, so it belongs in the planner where the claim can hold.
func TestDryRunReportsADestinationTheRealRunRefuses(t *testing.T) {
	dir, home := agentSetupProject(t)
	link := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone", "codex.toml"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	out, _, err := run(t, "agent-setup", "--dry-run", "--json", "--dir", dir, "--agent", agentCodex)
	if err == nil {
		t.Error("--dry-run exited 0 for a destination the real run refuses")
	}
	var payload agentSetupJSON
	if jsonErr := json.Unmarshal([]byte(out), &payload); jsonErr != nil {
		t.Fatalf("--dry-run --json emitted no readable payload (%v):\n%q", jsonErr, out)
	}
	if !payload.DryRun {
		t.Error("dryRun is not set on a --dry-run payload")
	}
	if payload.OK {
		t.Error("ok = true for a destination the real run refuses")
	}
	var blocked *agentChangeJSON
	for i := range payload.Changes {
		if payload.Changes[i].Action == actionBlocked {
			blocked = &payload.Changes[i]
		}
	}
	if blocked == nil {
		t.Fatalf("--dry-run reports no %q row for a destination the real run refuses:\n%s", actionBlocked, out)
	}
	if !strings.Contains(blocked.Reason, "symlink") {
		t.Errorf("the blocked row does not name the symlink: %s", blocked.Reason)
	}
	// A dry run still writes NOTHING, including the instruction files.
	for _, name := range []string{agentsFilename, claudeFilename} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil {
			t.Errorf("--dry-run wrote %s", name)
		}
	}
}

// TestDryRunAndTheRealRunAgreeOnEveryAction is the ledger behind that finding
// rather than a second copy of the one case that was wrong: for each fixture,
// the dry run's `changes` and the real run's must carry the same path and the
// same action, row for row.
//
// 🔴 THE ACTIONS ARE COMPARED, NOT JUST THE PATHS. A dry run reporting `create`
// where the real run reports `blocked` is exactly the defect, and a path-only
// comparison is satisfied by both.
func TestDryRunAndTheRealRunAgreeOnEveryAction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setUp func(t *testing.T, dir, home string)
	}{
		{"a fresh project", func(*testing.T, string, string) {}},
		{"an existing config", func(t *testing.T, _, home string) {
			writeFile(t, filepath.Join(home, ".codex", "config.toml"), "model = \"gpt-5\"\n")
		}},
		{"a config that does not parse", func(t *testing.T, _, home string) {
			writeFile(t, filepath.Join(home, ".codex", "config.toml"), "[mcp_servers\nx = 1\n")
		}},
		{"a broken symlink destination", func(t *testing.T, _, home string) {
			link := filepath.Join(home, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(home, "gone", "codex.toml"), link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			tc.setUp(t, dir, home)

			dryOut, _, _ := run(t, "agent-setup", "--dry-run", "--json", "--dir", dir, "--agent", agentCodex)
			realOut, _, _ := run(t, "agent-setup", "--json", "--dir", dir, "--agent", agentCodex)

			var dry, real agentSetupJSON
			if err := json.Unmarshal([]byte(dryOut), &dry); err != nil {
				t.Fatalf("bad --dry-run json: %v\n%q", err, dryOut)
			}
			if err := json.Unmarshal([]byte(realOut), &real); err != nil {
				t.Fatalf("bad write-run json: %v\n%q", err, realOut)
			}
			if len(dry.Changes) != len(real.Changes) {
				t.Fatalf("--dry-run reports %d rows, the real run %d:\n%s\n%s",
					len(dry.Changes), len(real.Changes), dryOut, realOut)
			}
			if dry.OK != real.OK {
				t.Errorf("--dry-run ok = %t, the real run ok = %t", dry.OK, real.OK)
			}
			for i := range dry.Changes {
				if dry.Changes[i].Path != real.Changes[i].Path {
					t.Errorf("row %d path: dry %q, real %q", i, dry.Changes[i].Path, real.Changes[i].Path)
				}
				if dry.Changes[i].Action != real.Changes[i].Action {
					t.Errorf("row %d action: dry %q, real %q — a dry run that reports an action the write "+
						"path would not take is the whole defect",
						i, dry.Changes[i].Action, real.Changes[i].Action)
				}
			}
		})
	}
}
