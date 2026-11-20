package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// Issue #933: the `agent-token` row described MCP entries it never read.
//
// A Claude setup run with no credential configured writes both Civitai entries
// WITHOUT a header (decision 34: a header is written only when a token is
// configured, so a working anonymous setup is not turned into a 401). The row
// then said "claude's MCP entries reference it by name" about a file holding no
// reference at all — and setting CIVITAI_TOKEN alone flipped it to `ok: true`
// although nothing in .mcp.json reads that variable. Both details below are
// pinned as WHOLE strings: a guard on a word ("reference") is walkable by a
// reword that keeps the false claim.
//
// 🔴 THE VERDICT EXEMPTION IS NOT WHAT CHANGED. The row stays out of `ok`
// (checkCountsTowardVerdict); each subtest asserts the overall verdict is still
// true so a fix that made the headerless anonymous setup FAIL `--check` — the
// exit-1-forever shape decision 35 removed — goes red here too.

const issue933OrchURL = "https://orchestration.civitai.com/mcp/v2"

func headerlessClaudeSetup(t *testing.T) (dir, mcpPath string) {
	t.Helper()
	dir, _ = agentSetupProject(t) // no CLI config, CIVITAI_TOKEN=""
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("PREMISE BROKEN: headerless setup: %v", err)
	}
	mcpPath = filepath.Join(dir, ".mcp.json")
	// PREMISE, asserted on the FILE: this is the headerless state the issue
	// measured. If setup ever starts writing a header here, the rest of this test
	// would be about a different file.
	raw := readFile(t, mcpPath)
	if !strings.Contains(raw, "mcp.civitai.com") || strings.Contains(raw, "Authorization") ||
		strings.Contains(raw, "headers") {
		t.Fatalf("PREMISE BROKEN: expected both entries registered WITHOUT headers:\n%s", raw)
	}
	return dir, mcpPath
}

func TestAgentTokenRowReadsTheWrittenEntries(t *testing.T) {
	t.Run("no reference written, variable unset", func(t *testing.T) {
		dir, mcpPath := headerlessClaudeSetup(t)
		rows, ok, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkAgentToken)
		want := "claude's MCP entries in " + mcpPath + " carry no CIVITAI_TOKEN reference — " +
			"`civitai agent-setup` writes one only when a token is configured — so they connect " +
			"anonymously and " + issue933OrchURL + " 401s. CIVITAI_TOKEN is NOT set in this environment " +
			"either. To authenticate them: run `civitai login` (or export CIVITAI_TOKEN), re-run " +
			"`civitai agent-setup` so the entries gain the reference, and export CIVITAI_TOKEN where " +
			"claude can see it. " + appTrackTokenNote
		if row.OK {
			t.Errorf("agent-token is ok:true over entries that carry no reference: %s", row.Detail)
		}
		if row.Detail != want {
			t.Errorf("agent-token detail:\n got: %s\nwant: %s", row.Detail, want)
		}
		if !ok {
			t.Errorf("the overall verdict went false — agent-token must stay exempt from it")
		}
	})

	t.Run("variable exported after a headerless setup", func(t *testing.T) {
		dir, mcpPath := headerlessClaudeSetup(t)
		t.Setenv(tokenEnvVar, "synthetic-env-only")
		rows, ok, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkAgentToken)
		want := "claude's MCP entries in " + mcpPath + " carry no CIVITAI_TOKEN reference — " +
			"`civitai agent-setup` writes one only when a token is configured — so they connect " +
			"anonymously and " + issue933OrchURL + " 401s. CIVITAI_TOKEN IS set in this environment, " +
			"but nothing in that file reads it: re-run `civitai agent-setup` with it set so the entries " +
			"gain the reference. " + appTrackTokenNote
		if row.OK {
			t.Errorf("exporting CIVITAI_TOKEN alone turned agent-token green while %s still holds "+
				"no reference: %s", mcpPath, row.Detail)
		}
		if row.Detail != want {
			t.Errorf("agent-token detail:\n got: %s\nwant: %s", row.Detail, want)
		}
		if !ok {
			t.Errorf("the overall verdict went false — agent-token must stay exempt from it")
		}
		// The env fixture must never be what the file holds either.
		if strings.Contains(readFile(t, mcpPath), "synthetic-env-only") {
			t.Errorf("the env value reached %s", mcpPath)
		}
	})

	// The positive control for the two above: with the reference really written
	// and the variable set, the row is green and still says so. Without this a fix
	// returning `false` unconditionally would pass both subtests.
	t.Run("reference written and variable set", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		t.Setenv(tokenEnvVar, "synthetic-env-only")
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		if raw := readFile(t, filepath.Join(dir, ".mcp.json")); !strings.Contains(raw, `"Bearer ${CIVITAI_TOKEN}"`) {
			t.Fatalf("PREMISE BROKEN: no reference written:\n%s", raw)
		}
		rows, _, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkAgentToken)
		want := "CIVITAI_TOKEN is set in this environment, which is where claude resolves the reference " +
			"in its MCP config from"
		if !row.OK || row.Detail != want {
			t.Errorf("agent-token = ok:%v %q, want ok:true %q", row.OK, row.Detail, want)
		}
	})
}

// TestHeaderlessSetupNextStepsCarryTheExportGuidance: the no-token `Next:` block
// told the reader to log in and re-run, and never that Claude reads the token
// from its OWN environment — the step the token-present branch already prints.
func TestHeaderlessSetupNextStepsCarryTheExportGuidance(t *testing.T) {
	dir, _ := agentSetupProject(t)
	stdout, stderr, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
	if err != nil {
		t.Fatalf("agent-setup: %v\n%s\n%s", err, stdout, stderr)
	}
	for _, want := range []string{
		"Run civitai login to authenticate, then re-run civitai agent-setup so the MCP entries pick up the reference.",
		"     claude reads the token from CIVITAI_TOKEN in its own environment, never from civitai login's " +
			"config file, so after the re-run also export it where claude can see it:\n" +
			"       export CIVITAI_TOKEN=<a personal API key>\n",
		appTrackTokenNote,
		"Mint one at https://civitai.com/user/account",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("headerless Next: block is missing %q\n---\n%s", want, stdout)
		}
	}
}
