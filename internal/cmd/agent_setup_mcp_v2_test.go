package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// The orchestration MCP server is registered at `/mcp/v2`, not the bare `/mcp`
// route, which serves the FROZEN v1 tool set. These tests pin the literal URL
// (never derived from civitaiMCPServers, so a revert to `/mcp` goes red) and the
// migration claim the PR makes: an install registered with the v1 URL moves to v2
// when `civitai agent-setup` is re-run, while every key the user added survives.

const (
	orchV1URL = "https://orchestration.civitai.com/mcp"
	orchV2URL = "https://orchestration.civitai.com/mcp/v2"
)

func TestOrchestrationMCPServerIsV2(t *testing.T) {
	var got []string
	for _, srv := range civitaiMCPServers {
		if srv.Name == "civitai-orchestration" {
			got = append(got, srv.URL)
		}
	}
	if len(got) != 1 || got[0] != orchV2URL {
		t.Fatalf("civitai-orchestration URL(s) = %q, want exactly [%q]", got, orchV2URL)
	}
}

// TestRerunMovesAV1OrchestrationEntryToV2JSON is the JSON (Claude Code) path.
func TestRerunMovesAV1OrchestrationEntryToV2JSON(t *testing.T) {
	dir, _ := agentSetupProject(t)
	path := filepath.Join(dir, ".mcp.json")
	writeFile(t, path, `{
  "mcpServers": {
    "civitai-orchestration": {
      "type": "http",
      "url": "`+orchV1URL+`",
      "timeout": 45
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
	entry, _ := servers["civitai-orchestration"].(map[string]any)
	if got := entry["url"]; got != orchV2URL {
		t.Errorf("url = %v after a re-run, want %q", got, orchV2URL)
	}
	if got := entry["timeout"]; got != float64(45) {
		t.Errorf("a key the user added to the entry was dropped: timeout = %v, want 45", got)
	}
}

// TestRerunMovesAV1OrchestrationEntryToV2TOML is the line-based Codex path.
func TestRerunMovesAV1OrchestrationEntryToV2TOML(t *testing.T) {
	dir, home := agentSetupProject(t)
	path := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, path, `model = "gpt-5"

[mcp_servers."civitai-orchestration"]
url = "`+orchV1URL+`"
tool_timeout_sec = 120
`)
	t.Setenv("CIVITAI_TOKEN", "")

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentCodex); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	got := readFile(t, path)
	if n := strings.Count(got, `url = "`+orchV2URL+`"`); n != 1 {
		t.Errorf("the v2 url line appears %d times, want 1:\n%s", n, got)
	}
	if strings.Contains(got, `url = "`+orchV1URL+`"`) {
		t.Errorf("the v1 url line survived the re-run:\n%s", got)
	}
	for _, want := range []string{"tool_timeout_sec = 120", `model = "gpt-5"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the merge dropped %q:\n%s", want, got)
		}
	}
}
