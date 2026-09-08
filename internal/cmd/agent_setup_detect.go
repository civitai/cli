package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Agent identity, detection, and the per-agent MCP config table.
//
// 🔴 DETECTION IS A PURE FUNCTION OVER AN INJECTED ENVIRONMENT. `detectAgent`
// reads nothing global: no os.Getenv, no os.Stat, no os.UserHomeDir. Everything
// it consults arrives in an agentEnv, so the ORDER of the signals — which is the
// only part that can be wrong — is table-testable without a machine that has any
// of these agents installed. `liveAgentEnv` is the one place the real process
// environment is read, and it is not called from any pure helper.
//
// 🔴 THE ORDER IS THE WHOLE ALGORITHM, NOT A STYLE CHOICE. Cursor, Windsurf and
// several others are VS Code FORKS and inherit its environment: a Cursor session
// sets `TERM_PROGRAM=vscode` too. So a table walked in the wrong order reports
// `vscode` for every fork, writes `.vscode/mcp.json` under the key `servers`,
// and the agent that is actually running never sees a server. Fork-specific
// signals therefore come FIRST and the generic VS Code signals last.

// The agent ids `--agent` accepts. `other` is a real answer, not a failure: it
// means "this CLI does not know where your agent keeps MCP config", and the
// command then prints the JSON for the human to paste rather than writing a file
// it guessed at.
const (
	agentClaude   = "claude"
	agentCursor   = "cursor"
	agentCodex    = "codex"
	agentOpencode = "opencode"
	agentVSCode   = "vscode"
	agentWindsurf = "windsurf"
	agentZed      = "zed"
	agentOther    = "other"
)

// agentEnv is everything detection is allowed to look at. Injected so the
// detection table can be exercised over fixtures instead of over the machine the
// tests happen to run on — including in CI, where none of these agents exist and
// a detection function reading the real environment would be untestable in the
// only direction that matters.
type agentEnv struct {
	// Vars is the process environment, already read into a map.
	Vars map[string]string
	// Home is the user's home directory, or "" when it could not be resolved.
	// A user-scoped target (codex, windsurf, zed) is unreachable without it, and
	// the command says so rather than writing into a path built from "".
	Home string
	// Dir is the project directory the run was pointed at.
	Dir string
	// Exists reports whether an absolute path exists. Injected for the same
	// reason Vars is: a marker-file table is only testable if the filesystem is.
	Exists func(path string) bool
}

// liveAgentEnv builds an agentEnv from the real process environment. It is the
// ONE place any of this is read; every other function in this file takes the
// struct. A home directory that cannot be resolved is not an error here — it
// becomes an unreachable user-scoped target, reported by name at the point of
// use.
func liveAgentEnv(dir string) agentEnv {
	vars := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			vars[kv[:i]] = kv[i+1:]
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return agentEnv{
		Vars:   vars,
		Home:   home,
		Dir:    dir,
		Exists: func(path string) bool { _, statErr := os.Stat(path); return statErr == nil },
	}
}

// envSignal is one environment-variable tell. Value == "" means "the variable
// being set to anything non-empty is the signal"; otherwise the comparison is a
// case-insensitive equality, because `TERM_PROGRAM` is spelled inconsistently
// across the editors that set it.
type envSignal struct {
	Agent string
	Var   string
	Value string
}

// agentEnvSignals is the ORDERED environment table.
//
// 🔴 FORK-SPECIFIC SIGNALS COME BEFORE THE SIGNAL THEY INHERIT. Cursor and
// Windsurf are VS Code forks and both set `TERM_PROGRAM=vscode`; Zed sets
// `TERM_PROGRAM=zed` but also `ZED_TERM`. Sorting this table, or appending a new
// editor at the end, is how every fork silently starts being reported as
// `vscode` — which writes the right JSON into a file the running agent never
// reads. TestAgentDetectionPrefersTheForkOverVSCode pins the order.
//
// 🔴 THIS IS A HEURISTIC, AND `--agent` IS THE AUTHORITY. A missing signal costs
// nothing — detection falls through to the marker files and then to `other`,
// both of which are honest answers. What it must never do is answer CONFIDENTLY
// wrong, which is what an out-of-order table does.
var agentEnvSignals = []envSignal{
	// Claude Code exports CLAUDECODE=1 and an entrypoint tag into the shell it
	// runs commands in.
	{agentClaude, "CLAUDECODE", ""},
	{agentClaude, "CLAUDE_CODE", ""},
	{agentClaude, "CLAUDE_CODE_ENTRYPOINT", ""},

	// Cursor — before the VS Code rows below, which it also satisfies.
	{agentCursor, "CURSOR_TRACE_ID", ""},
	{agentCursor, "CURSOR_AGENT", ""},

	// Windsurf — same reason, same position.
	{agentWindsurf, "WINDSURF_USER_ID", ""},
	{agentWindsurf, "TERM_PROGRAM", "windsurf"},

	// Codex CLI.
	{agentCodex, "CODEX_SANDBOX", ""},
	{agentCodex, "CODEX_SANDBOX_NETWORK_DISABLED", ""},

	// opencode.
	{agentOpencode, "OPENCODE", ""},
	{agentOpencode, "OPENCODE_BIN_PATH", ""},

	// Zed's integrated terminal.
	{agentZed, "ZED_TERM", ""},
	{agentZed, "TERM_PROGRAM", "zed"},

	// Plain VS Code, LAST — every fork above inherits these.
	{agentVSCode, "VSCODE_GIT_ASKPASS_MAIN", ""},
	{agentVSCode, "VSCODE_PID", ""},
	{agentVSCode, "TERM_PROGRAM", "vscode"},
}

// agentMarker is one project-directory tell, consulted only when no environment
// signal fired — i.e. the command is being run from an ordinary shell.
type agentMarker struct {
	Agent string
	// Rel is relative to the project directory.
	Rel string
}

// agentMarkers is the ORDERED marker table.
//
// 🔴 `.vscode/` IS LAST, AND DELIBERATELY. It is the most common directory in
// this list by a wide margin and says almost nothing about which agent is
// driving — plenty of Cursor and Claude Code projects carry one. Every marker
// that is actually diagnostic is checked before it.
var agentMarkers = []agentMarker{
	{agentClaude, ".claude"},
	{agentClaude, ".mcp.json"},
	{agentCursor, ".cursor"},
	{agentOpencode, "opencode.json"},
	{agentOpencode, "opencode.jsonc"},
	{agentZed, ".zed"},
	{agentVSCode, ".vscode"},
}

// detectAgent answers which coding agent this run is configuring.
//
// Order, per the contract: environment first (it describes the process that is
// running RIGHT NOW), then the project's marker files (which describe what has
// been used here before), then `other`. It never returns "" — `other` is the
// floor, and it is a real answer.
func detectAgent(env agentEnv) string {
	for _, s := range agentEnvSignals {
		v, ok := env.Vars[s.Var]
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		if s.Value == "" || strings.EqualFold(strings.TrimSpace(v), s.Value) {
			return s.Agent
		}
	}
	if env.Exists != nil {
		for _, m := range agentMarkers {
			if env.Exists(filepath.Join(env.Dir, m.Rel)) {
				return m.Agent
			}
		}
	}
	return agentOther
}

// configFormat is how a target's config file is encoded. It decides which merge
// routine runs, and nothing else.
type configFormat int

// The zero value is deliberately NEITHER of these: an agent with no entry in
// agentTargets has no config file this CLI knows, and that case never reaches a
// merge — it is answered by the print-and-paste path instead.
const (
	formatJSON configFormat = iota + 1
	formatTOML
)

// agentTarget is where and how ONE agent stores its MCP servers.
//
// 🔴 SIX SPELLINGS FOR ONE CONCEPT, AND THAT IS THE ENTIRE REASON THIS COMMAND
// EXISTS. The top-level key is `mcpServers` for three agents, `servers` for VS
// Code, `mcp` for opencode, `context_servers` for Zed and a TOML table
// `[mcp_servers.<name>]` for Codex; the URL key is `url` everywhere except
// Windsurf, which spells it `serverUrl`. Every one of those is a silent failure
// when wrong: the file parses, the agent starts, and the server is simply not
// there. TestAgentTargetTable pins all of it.
type agentTarget struct {
	Agent string
	// UserScoped is true when the file lives under $HOME rather than under the
	// project directory. A user-scoped target cannot be written when the home
	// directory is unresolvable, and the command says which target that was.
	UserScoped bool
	// Parts are path segments joined onto the project dir (or $HOME).
	Parts  []string
	Format configFormat
	// ServersKey is the top-level object key (JSON) or the table prefix (TOML)
	// the server entries live under.
	ServersKey string
	// URLKey is the entry key carrying the server URL.
	URLKey string
	// TypeKey/TypeValue is the transport discriminator, where the agent has one.
	// An empty TypeKey means the agent does not use one and writing it would be
	// an unknown key in someone else's config.
	TypeKey   string
	TypeValue string
	// HeadersKey carries the `Authorization: Bearer …` header when — and ONLY
	// when — a token is configured. See mcpEntry.
	HeadersKey string
	// EnabledKey, where set, is written `true`. opencode requires it.
	EnabledKey string
}

// agentTargets is the table. Keyed by agent id; `other` is absent on purpose —
// its absence IS the "we do not know where your agent keeps this" answer, so a
// lookup miss and `other` are the same code path.
var agentTargets = map[string]agentTarget{
	agentClaude: {
		Agent: agentClaude, Parts: []string{".mcp.json"}, Format: formatJSON,
		ServersKey: "mcpServers", URLKey: "url",
		TypeKey: "type", TypeValue: "http", HeadersKey: "headers",
	},
	agentCursor: {
		Agent: agentCursor, Parts: []string{".cursor", "mcp.json"}, Format: formatJSON,
		ServersKey: "mcpServers", URLKey: "url", HeadersKey: "headers",
	},
	agentVSCode: {
		Agent: agentVSCode, Parts: []string{".vscode", "mcp.json"}, Format: formatJSON,
		// 🔴 `servers`, NOT `mcpServers`. VS Code is the one JSON agent here that
		// does not spell it the Claude way.
		ServersKey: "servers", URLKey: "url",
		TypeKey: "type", TypeValue: "http", HeadersKey: "headers",
	},
	agentCodex: {
		Agent: agentCodex, UserScoped: true, Parts: []string{".codex", "config.toml"},
		Format: formatTOML, ServersKey: "mcp_servers", URLKey: "url",
		HeadersKey: "http_headers",
	},
	agentOpencode: {
		Agent: agentOpencode, Parts: []string{"opencode.json"}, Format: formatJSON,
		// 🔴 `mcp`, and the entry carries `type: "remote"` plus an explicit
		// `enabled` — opencode's schema requires both.
		ServersKey: "mcp", URLKey: "url",
		TypeKey: "type", TypeValue: "remote", HeadersKey: "headers",
		EnabledKey: "enabled",
	},
	agentWindsurf: {
		Agent: agentWindsurf, UserScoped: true,
		Parts: []string{".codeium", "windsurf", "mcp_config.json"}, Format: formatJSON,
		// 🔴 `serverUrl`, NOT `url`. Windsurf is the only agent in this table that
		// spells the URL key differently, and a `url` key here parses fine and
		// registers nothing.
		ServersKey: "mcpServers", URLKey: "serverUrl", HeadersKey: "headers",
	},
	agentZed: {
		Agent: agentZed, UserScoped: true, Parts: []string{".config", "zed", "settings.json"},
		Format: formatJSON,
		// 🔴 `context_servers`, which is Zed's own name for the same concept, and
		// NO transport discriminator. zed.dev/docs/ai/mcp's remote example is
		// `{"url": …, "headers": {"Authorization": "Bearer …"}}` and nothing else;
		// a `"source": "custom"` key was real in older Zed and appears in NO
		// current doc, so it is not written. An invented key in someone else's
		// settings file is the failure this whole table exists to avoid.
		ServersKey: "context_servers", URLKey: "url", HeadersKey: "headers",
	},
}

// knownAgents returns every id `--agent` accepts, sorted, for the flag's help
// text and for the unknown-value refusal. Derived from the table plus `other`,
// so a new target cannot be added without becoming acceptable and documented in
// one move.
func knownAgents() []string {
	out := make([]string, 0, len(agentTargets)+1)
	for id := range agentTargets {
		out = append(out, id)
	}
	out = append(out, agentOther)
	sort.Strings(out)
	return out
}

// validateAgentFlag classifies an explicit --agent value.
//
// An unknown value is a mistake about the INVOCATION — exit 2 — and the refusal
// names every accepted id, because a user who typed `--agent vs-code` needs the
// spelling, not a category.
func validateAgentFlag(v string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(v))
	if id == agentOther {
		return agentOther, nil
	}
	if _, ok := agentTargets[id]; ok {
		return id, nil
	}
	return "", asUsageError(fmt.Errorf(
		"unknown --agent %q — expected one of: %s. Omit --agent to detect it, or pass `--agent other` "+
			"to print the MCP config for you to paste in yourself",
		v, strings.Join(knownAgents(), ", ")))
}

// agentConfigPath renders the absolute config path for an agent, or ("", false) when
// there is none to render: `other` has no file, and a user-scoped target has no
// path when the home directory could not be resolved.
//
// 🔴 THE SECOND RETURN IS NOT "NOT REGISTERED". A caller that collapses the two
// reports "not registered in " with an empty path — the shape of an answer with
// none of the content. Every call site branches on it.
func agentConfigPath(env agentEnv, agent string) (string, bool) {
	t, ok := agentTargets[agent]
	if !ok {
		return "", false
	}
	base := env.Dir
	if t.UserScoped {
		if strings.TrimSpace(env.Home) == "" {
			return "", false
		}
		base = env.Home
	}
	return filepath.Join(append([]string{base}, t.Parts...)...), true
}
