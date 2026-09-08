package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	// GOOS is runtime.GOOS, injected rather than read.
	//
	// 🔴 A PER-OS CONFIG PATH IS OTHERWISE UNTESTABLE ON ONE MACHINE. Zed keeps
	// its settings under `%APPDATA%\Zed` on Windows and `~/.config/zed`
	// everywhere else, and the release cross-compiles windows/amd64 and
	// windows/arm64 — so the Windows path is shipped to users on a build no
	// maintainer runs. Injecting it lets the table be exercised for both.
	GOOS string
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
		GOOS:   runtime.GOOS,
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
	Parts []string
	// Roots are config-root OVERRIDES, tried in order BEFORE Parts.
	//
	// 🔴 $HOME IS NOT ALWAYS THE ROOT, AND GETTING IT WRONG IS SILENT. Codex
	// documents `CODEX_HOME` as its config directory; a user who has set it gets
	// a `~/.codex/config.toml` written that Codex never reads, and `--check`
	// then reports "not registered" forever with no way to tell why. Zed on
	// Windows reads `%APPDATA%\Zed\settings.json`, and the release ships
	// windows/amd64 and windows/arm64. See agentRoot.
	Roots []agentRoot
	// PreferExisting are alternative Parts that WIN over Parts when the file is
	// already there.
	//
	// 🔴 A DETECTION MARKER THAT IS NEVER A WRITE TARGET CREATES A SECOND FILE.
	// `opencode.jsonc` is a marker that detects opencode; without this the run
	// then writes `opencode.json` BESIDE it, and the config the user actually
	// maintains is not the one the servers landed in.
	PreferExisting [][]string
	Format         configFormat
	// AllowsComments is true when the agent's own parser accepts JSONC — `//`
	// and `/* */` comments in a nominally-JSON file.
	//
	// 🔴 WITHOUT IT, THE COMMON CASE IS A REFUSAL. Zed SHIPS
	// `~/.config/zed/settings.json` with a leading comment block, and VS Code
	// writes commented JSON throughout `.vscode/`. A strict `encoding/json`
	// decode of either fails with `invalid character '/'`, and the refusal blames
	// the user for a file their editor authored. See blankJSONComments for what
	// this does and does NOT preserve.
	AllowsComments bool
	// Caveat, when set, is one sentence printed beside this agent's config path
	// saying something true about it that the path alone does not convey.
	Caveat string
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
	// HeadersKey is the entry key an Authorization header would live under.
	//
	// 🔴 IT DOES NOT MEAN "WRITE A HEADER". A header is written ONLY when
	// EnvHeaderSyntax is also set; see mcpEntry and the block comment above
	// agentTargets.
	HeadersKey string
	// EnvHeaderSyntax is this agent's DOCUMENTED environment-variable
	// interpolation for a header VALUE, already spelled for CIVITAI_TOKEN —
	// e.g. `${CIVITAI_TOKEN}`. Empty means the vendor documents none, and an
	// empty value is the SAFE default: see the block comment above agentTargets
	// for why an unsupported syntax is worse than no header at all.
	EnvHeaderSyntax string
	// EnvBearerKey is an entry key taking the NAME of an environment variable
	// holding the bearer token, with the agent doing the `Bearer ` prefixing
	// itself. Codex's `bearer_token_env_var` is the only one; it is a DIFFERENT
	// mechanism from EnvHeaderSyntax, not a spelling of it, and the two are
	// mutually exclusive (TestEveryTargetPicksOneCredentialMechanism).
	EnvBearerKey string
	// EnabledKey, where set, is written `true`. opencode requires it.
	EnabledKey string
}

// agentRoot is one config-root override for a user-scoped target: an
// environment variable the vendor documents as its config directory, optionally
// restricted to one GOOS.
//
// 🔴 IT REPLACES THE WHOLE `$HOME + Parts` PATH, not just its first segment —
// `CODEX_HOME=/etc/codex` means `/etc/codex/config.toml`, not
// `/etc/codex/.codex/config.toml`. That is why Parts is stated here again
// rather than reused from the target.
type agentRoot struct {
	// EnvVar holds the root directory. Empty or unset ⇒ this root does not apply.
	EnvVar string
	// GOOS restricts the root to one operating system; "" means any.
	GOOS string
	// Parts are joined onto EnvVar's value.
	Parts []string
}

// tokenEnvVar is the environment variable this CLI already publishes as its
// token override (`config.Load` reads it). It is the ONE name an interpolated
// MCP header may reference: a second spelling would be a variable the user has
// never been told to set, so the header would resolve to empty and the failure
// would look like a bad token rather than a missing one.
const tokenEnvVar = "CIVITAI_TOKEN"

// agentTargets is the table. Keyed by agent id; `other` is absent on purpose —
// its absence IS the "we do not know where your agent keeps this" answer, so a
// lookup miss and `other` are the same code path.
//
// 🔴 NO ENTRY IN THIS TABLE MAY EVER CARRY A LITERAL CREDENTIAL, AND THAT IS A
// FIX FOR A SHIPPED BUG RATHER THAN A PREFERENCE. The first cut of this command
// wrote `"Authorization": "Bearer <the user's live token>"` into whichever file
// the agent uses — and four of those files (`.mcp.json`, `.cursor/mcp.json`,
// `.vscode/mcp.json`, `opencode.json`) are PROJECT-scoped, live in the repo root
// and get committed. This repo ships `internal/credscan` to warn about exactly
// that in a packaged bundle; writing one here was the same failure with the
// polarity reversed. So the rule is now absolute, project- and user-scoped
// alike, with no flag to opt in.
//
// 🔴 THE SYNTAXES BELOW COME FROM THE VENDOR DOCS AND FOUR OF THEM DISAGREE.
// `${VAR}` (Claude Code) vs `${env:VAR}` (Cursor, Windsurf, VS Code) vs
// `{env:VAR}` — single brace, no `$` — (opencode) vs a TOML key taking a bare
// variable NAME (Codex) vs nothing at all (Zed). Getting one wrong is WORSE than
// omitting the header: an unsupported syntax produces a config that looks
// configured and sends the literal string `${CIVITAI_TOKEN}` as a bearer token,
// which fails at request time and looks like a bad credential rather than a bad
// config. So an agent whose vendor doc does not document interpolation gets NO
// header key at all, and its next-step block names the header to add by hand.
//
// 🔴 A HEADER-LESS ENTRY IS NOT A COMPLETE SETUP, AND SAYING IT IS WAS THE BUG.
// The two servers differ: the SITE server serves an anonymous `initialize`, the
// ORCHESTRATION server answers 401 to one. So a header-less config reaches half
// of what it registered. See mcpServer.Anonymous — every surface reads it rather
// than repeating a claim about "both servers".
//
// 🔴 A HEADER IS ONLY WRITTEN WHEN A TOKEN IS CONFIGURED — AND "CONFIGURED" IS
// NOT "EXPORTED", WHICH IS THE PART THIS COMMENT USED TO GET WRONG. The gate is
// `config.Load().Token()`, satisfied by `~/.config/civitai/config.yaml` (i.e. by
// `civitai login`) OR by the CIVITAI_TOKEN environment variable. The AGENT
// resolves the header from the PROCESS environment and cannot read this CLI's
// config file, so the two are independent and the gate covers only the first.
//
// What the gate does buy: with NO token at all, an unconditional header would
// send `Bearer ` (Cursor/Windsurf/opencode/VS Code resolve an unset variable to
// the empty string) or the literal `Bearer ${CIVITAI_TOKEN}` (Claude Code passes
// it through) on every request, turning a working anonymous SITE setup into a
// 401 for a user who never asked for auth.
//
// What it does NOT cover, and what the output must therefore say: a user who ran
// `civitai login` but never exported CIVITAI_TOKEN passes this gate, gets the
// header written, and the agent still resolves it to empty. That case is
// detected separately (tokenIsExported) and named in the next-step block — it is
// a missing export, not a bad credential, and it reads as the latter if nobody
// says so.
var agentTargets = map[string]agentTarget{
	agentClaude: {
		Agent: agentClaude, Parts: []string{".mcp.json"}, Format: formatJSON,
		ServersKey: "mcpServers", URLKey: "url",
		TypeKey: "type", TypeValue: "http", HeadersKey: "headers",
		// code.claude.com/docs/en/mcp: "Claude Code supports environment
		// variable expansion in .mcp.json files"; the listed locations include
		// "headers: for HTTP server authentication", with the documented example
		// `"Authorization": "Bearer ${API_KEY}"`. Bare `${VAR}` — NOT `${env:…}`.
		EnvHeaderSyntax: "${" + tokenEnvVar + "}",
	},
	agentCursor: {
		Agent: agentCursor, Parts: []string{".cursor", "mcp.json"}, Format: formatJSON,
		ServersKey: "mcpServers", URLKey: "url", HeadersKey: "headers",
		// cursor.com/docs/context/mcp: "Cursor resolves variables in these
		// fields: command, args, env, url, and headers", documented example
		// `"Authorization": "Bearer ${env:MY_SERVICE_TOKEN}"`. The `env:` prefix
		// is required — Claude Code's bare `${VAR}` is not Cursor's syntax.
		EnvHeaderSyntax: "${env:" + tokenEnvVar + "}",
	},
	agentVSCode: {
		Agent: agentVSCode, Parts: []string{".vscode", "mcp.json"}, Format: formatJSON,
		// 🔴 `servers`, NOT `mcpServers`. VS Code is the one JSON agent here that
		// does not spell it the Claude way.
		ServersKey: "servers", URLKey: "url",
		TypeKey: "type", TypeValue: "http", HeadersKey: "headers",
		// `.vscode/mcp.json` and VS Code's own user `mcp.json` are JSONC — VS
		// Code writes commented JSON throughout `.vscode/`.
		AllowsComments: true,
		// 🔴 THIS ROW USED TO SAY "NO INTERPOLATION", AND THAT WAS WRONG. It was
		// derived from the docs alone — the MCP configuration reference's only
		// `headers` example is `"Bearer ${input:api-token}"` (prompted input),
		// and the variables reference does not name `mcp.json` — so the table
		// shipped the conservative reading. The IMPLEMENTATION says otherwise,
		// and it was checked rather than inferred:
		//
		//   - microsoft/vscode#245237 "Support ${env:VARIABLE_NAME} in mcp.json",
		//     closed COMPLETED 2025-04-01; the one comment, from connor4312
		//     (MEMBER, who owns the MCP implementation): "This is supported."
		//   - microsoft/vscode#264448, closed completed; connor4312 again: "The
		//     format is ${env:VARIABLE_NAME}" and "It works using the same logic
		//     as tasks.json/launch.json do".
		//   - mcpRegistry.ts `_replaceVariablesInLaunch` parses
		//     `McpServerLaunch.toSerialized(launch)` — which is the IDENTITY
		//     function (mcpTypes.ts) — and hands it to `resolveAsync`;
		//     `ConfigurationResolverExpression.parseObject` recurses through
		//     arrays and objects; an HTTP launch's `headers` is
		//     `[string, string][]` built from `configuration.headers`; and
		//     `variableReplacement` is set for EVERY mcp.json server, not only
		//     stdio ones (installedMcpServersDiscovery.ts).
		//
		// 🔴 THE RESIDUAL, STATED: VS Code resolves `${env:X}` against ITS OWN
		// process environment, and a variable it cannot see becomes the EMPTY
		// STRING, not the literal (variableResolver.ts `case 'env'` returns
		// ''). So a GUI-launched VS Code that never read your shell profile
		// sends `Authorization: Bearer ` and 401s. That failure is a missing
		// export, not a bad token, and the next-step block says so.
		EnvHeaderSyntax: "${env:" + tokenEnvVar + "}",
	},
	agentCodex: {
		Agent: agentCodex, UserScoped: true, Parts: []string{".codex", "config.toml"},
		// 🔴 `CODEX_HOME` IS THE ROOT WHEN IT IS SET. Codex documents it as its
		// configuration directory; writing `$HOME/.codex/config.toml` for a user
		// who has moved it registers the servers in a file Codex never opens.
		Roots:  []agentRoot{{EnvVar: "CODEX_HOME", Parts: []string{"config.toml"}}},
		Format: formatTOML, ServersKey: "mcp_servers", URLKey: "url",
		HeadersKey: "http_headers",
		// 🔴 A DIFFERENT MECHANISM, NOT A DIFFERENT SPELLING. Codex documents no
		// `${…}` interpolation anywhere in config.toml. It instead has keys that
		// take a variable NAME: `bearer_token_env_var` — "Environment variable
		// name for a bearer token to send in Authorization" — beside the
		// literal-only `http_headers` ("Map of header names to static values").
		// learn.chatgpt.com/docs/extend/mcp?surface=cli.
		//
		// `bearer_token_env_var` is chosen over the sibling `env_http_headers`
		// ("Map of header names to environment variable names") because that one
		// sends the variable's value VERBATIM as the header: an `Authorization`
		// built that way would carry the bare token with no `Bearer ` prefix.
		EnvBearerKey: "bearer_token_env_var",
	},
	agentOpencode: {
		Agent: agentOpencode, Parts: []string{"opencode.json"},
		// 🔴 AN EXISTING `opencode.jsonc` IS THE TARGET, NOT A SECOND FILE.
		// `opencode.jsonc` is one of the markers that DETECTS opencode
		// (agentMarkers), so a project carrying only that file was detected
		// correctly and then had `opencode.json` created beside it — two config
		// files, the servers in the one the user does not maintain.
		PreferExisting: [][]string{{"opencode.jsonc"}},
		Format:         formatJSON, AllowsComments: true,
		// 🔴 `mcp`, and the entry carries `type: "remote"` plus an explicit
		// `enabled` — opencode's schema requires both.
		ServersKey: "mcp", URLKey: "url",
		TypeKey: "type", TypeValue: "remote", HeadersKey: "headers",
		EnabledKey: "enabled",
		// 🔴 SINGLE BRACE AND NO `$` — the odd one out in this whole table.
		// opencode.ai/docs/mcp-servers/ shows `"Authorization": "Bearer
		// {env:MY_API_KEY}"` inside a remote server's headers, and
		// opencode.ai/docs/config/ states the rule: "Use {env:VARIABLE_NAME} to
		// substitute environment variables". Writing `${env:…}` here would be
		// sent literally.
		EnvHeaderSyntax: "{env:" + tokenEnvVar + "}",
	},
	agentWindsurf: {
		Agent: agentWindsurf, UserScoped: true,
		Parts: []string{".codeium", "windsurf", "mcp_config.json"}, Format: formatJSON,
		// 🔴 `serverUrl`, NOT `url`. Windsurf is the only agent in this table that
		// spells the URL key differently, and a `url` key here parses fine and
		// registers nothing.
		ServersKey: "mcpServers", URLKey: "serverUrl", HeadersKey: "headers",
		// docs.windsurf.com/windsurf/cascade/mcp (307 → docs.devin.ai/desktop/
		// cascade/mcp): "supports variable interpolation in the following fields:
		// command, args, env, serverUrl, url, and headers", with `${env:VAR_NAME}`
		// and the documented example `"API_KEY": "Bearer ${env:AUTH_TOKEN}"`.
		EnvHeaderSyntax: "${env:" + tokenEnvVar + "}",
		// 🔴 THAT PAGE NOW SCOPES ITSELF TO THE LEGACY AGENT, AND THE PATH IS
		// STILL THE ONE IT DOCUMENTS. Verbatim, at the top of it: "The MCP
		// configuration on this page applies to the legacy Cascade agent only.
		// The Devin Local agent — the default agent for new tabs — configures MCP
		// servers in the Devin CLI config files instead." Those files are
		// `~/.config/devin/mcp_config.json` (`%APPDATA%\devin\mcp_config.json` on
		// Windows) and the project-scoped `.devin/mcp_config.json`.
		//
		// This CLI still writes only the Cascade path, because that is the one
		// this row was built and tested against — but a user on the current
		// default agent would otherwise get a silently-ignored file, so the row
		// SAYS SO rather than pretending the write is the whole answer.
		Caveat: "that file is the LEGACY Cascade agent's; the Devin Local agent (the default for new " +
			"tabs) reads ~/.config/devin/mcp_config.json instead — add the servers there too if you use it",
	},
	agentZed: {
		Agent: agentZed, UserScoped: true, Parts: []string{".config", "zed", "settings.json"},
		// 🔴 `%APPDATA%\Zed\settings.json` ON WINDOWS, and the release ships
		// windows/amd64 and windows/arm64. `$HOME/.config/zed` there is a
		// directory Zed does not read.
		Roots:  []agentRoot{{EnvVar: "APPDATA", GOOS: "windows", Parts: []string{"Zed", "settings.json"}}},
		Format: formatJSON,
		// 🔴 ZED SHIPS THIS FILE WITH A COMMENT BLOCK IN IT. A fresh
		// `~/.config/zed/settings.json` opens with four `//` lines pointing at
		// Zed's docs, so a strict JSON decode fails on the FIRST BYTE of the
		// FIRST LINE for every real Zed install. Measured; it took the whole run
		// down with it.
		AllowsComments: true,
		// 🔴 `context_servers`, which is Zed's own name for the same concept, and
		// NO transport discriminator. zed.dev/docs/ai/mcp's remote example is
		// `{"url": …, "headers": {"Authorization": "Bearer …"}}` and nothing else;
		// a `"source": "custom"` key was real in older Zed and appears in NO
		// current doc, so it is not written. An invented key in someone else's
		// settings file is the failure this whole table exists to avoid.
		ServersKey: "context_servers", URLKey: "url", HeadersKey: "headers",
		// 🔴 NO INTERPOLATION. zed.dev/docs/ai/mcp documents no substitution
		// syntax at all and its remote example hard-codes `"Bearer <token>"`, so
		// there is nothing this command can write here that is both
		// authenticated and credential-free.
		//
		// 🔴 THE OAUTH RATIONALE THAT USED TO SIT HERE IS RETRACTED, AND IT HAS
		// NO REPLACEMENT. It said a header-less entry "lets Zed offer the user
		// its own auth flow" via Zed's MCP OAuth prompt. That flow needs the
		// server to advertise its authorization server, and the Civitai
		// orchestration server does not: an anonymous `initialize` returns a
		// bare 401 with NO `WWW-Authenticate` header, and
		// `/.well-known/oauth-protected-resource`,
		// `/.well-known/oauth-authorization-server` and the `/mcp`-scoped
		// variants all 404. Measured against the live host. So the header-less
		// entry is the SAFE option and nothing more — Zed users authenticate by
		// adding the header themselves, which the next-step block tells them.
		// Do not restore a benefit clause here without a live probe behind it.
		EnvHeaderSyntax: "",
		Caveat: "Zed documents no environment-variable interpolation, so the entries carry no Authorization " +
			"header; the orchestration server needs one, and Zed's MCP OAuth flow cannot supply it (that " +
			"server advertises no authorization server)",
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
	parts := t.Parts
	if t.UserScoped {
		// A documented config-root override wins over $HOME, and is checked
		// FIRST — a user who set CODEX_HOME has moved the file, not added a
		// second copy of it.
		if root, rootParts, found := agentRootOverride(env, t); found {
			base, parts = root, rootParts
		} else if strings.TrimSpace(env.Home) == "" {
			return "", false
		} else {
			base = env.Home
		}
	}
	// An alternative spelling that ALREADY EXISTS is the file the user
	// maintains, so it beats the default name. Only consulted when the
	// filesystem is injected; detection-only callers pass no Exists.
	if env.Exists != nil {
		for _, alt := range t.PreferExisting {
			candidate := filepath.Join(append([]string{base}, alt...)...)
			if env.Exists(candidate) {
				return candidate, true
			}
		}
	}
	return filepath.Join(append([]string{base}, parts...)...), true
}

// agentRootOverride returns the first applicable config-root override for a
// target: an environment variable the vendor documents as its config directory,
// set to something non-empty, on a matching GOOS.
func agentRootOverride(env agentEnv, t agentTarget) (root string, parts []string, found bool) {
	for _, r := range t.Roots {
		if r.GOOS != "" && r.GOOS != env.GOOS {
			continue
		}
		v := strings.TrimSpace(env.Vars[r.EnvVar])
		if v == "" {
			continue
		}
		return v, r.Parts, true
	}
	return "", nil, false
}
