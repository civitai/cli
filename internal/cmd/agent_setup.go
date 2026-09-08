package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// `civitai agent-setup` — configure the developer's CODING AGENT to build
// Civitai Apps: an AGENTS.md the agent reads, and the two Civitai MCP servers
// registered in whichever config file that particular agent looks in.
//
// 🔴 IT IS A TOP-LEVEL COMMAND, NOT `civitai app agent-setup`. It configures the
// developer's environment, not an app: it can be run in a directory that holds
// no manifest, before any app exists, and it never talks to the App API.
//
// 🔴 IT STOPS BEFORE AUTH, ON PURPOSE. Nothing here runs `civitai login`, opens
// a browser or prompts for a credential — the last thing it prints is the
// instruction for the human to authenticate themselves. A token-less
// registration is still useful because the SITE server answers anonymously, and
// registering a server you cannot yet reach costs nothing and is reversible. So
// an unauthenticated setup is a SUCCESS rather than a half-finished one, and
// that is why `authenticated` is reported by `--check` and deliberately excluded
// from its verdict; see agentSetupVerdict.
//
// 🔴 IT IS NOT "BOTH SERVERS WORK ANONYMOUSLY", WHICH IS WHAT THIS COMMENT AND
// SIX OTHER SURFACES USED TO SAY. The orchestration server 401s an anonymous
// handshake. See mcpServer.Anonymous; every surface here reads mcpAnonymityNote
// rather than repeating a claim.
//
// 🔴 IT PINS NO PACKAGE VERSION, ANYWHERE — not in the template, not in help
// text, not as an example in a comment. See agent_setup_files.go.

// The tracks. `api` is a RECOGNISED value that refuses, not an unknown flag
// value: an agent told "unknown --track" concludes the flag is wrong, while an
// agent told "that track is not built yet, here is what does work" does the
// right thing. See agentSetupTrackAPIRefusal.
const (
	trackApp = "app"
	trackAPI = "api"
)

// The `--check` row names. 🔴 PUBLISHED: `developer.civitai.com`'s setup prompt
// runs `civitai agent-setup --check --json` and reads these strings, so renaming
// one silently breaks a hosted document this repo does not own.
const (
	checkCLIVersion    = "cli-version"
	checkAgentsMD      = "agents-md"
	checkClaudeMD      = "claude-md"
	checkAuthenticated = "authenticated"
)

// ErrAgentSetupIncomplete is the sentinel `agent-setup --check` returns when at
// least one check failed. It carries the non-zero exit and nothing else — the
// rows have already been printed by the time it is returned.
//
// 🔴 EXIT 1, AND THAT IS A DECISION RATHER THAN A FALLTHROUGH — the same one
// `app doctor`'s ErrListingBlocked records. Exit 2 is documented as a mistake
// about the INVOCATION, and every flag and argument is well-formed when this
// fires; what is incomplete is the SETUP. It is deliberately left UNTAGGED for
// the exit mapper (tagging it civitai.ErrBadRequest — the only route to exit 2 —
// would move it), which TestAgentSetupCheckFailureExitsGeneric in cmd/civitai is
// what makes deliberate.
var ErrAgentSetupIncomplete = errors.New("agent setup incomplete")

// The two exit-2 refusals for `--dir`, as named constants.
//
// 🔴 THEY ARE CONSTANTS BECAUSE A SWAP IS OTHERWISE INVISIBLE (AGENTS.md item
// 26's recorded shape): both arms carry the same ErrUsage sentinel, so no
// errors.Is assertion can tell them apart, and a message test that only asks for
// the path is satisfied by either spelling — while the user whose directory is
// simply missing is told to "pass the project ROOT, not a file".
// TestAgentSetupDirRemediesMatchTheirArm requires each arm to carry its own and
// NOT the other's.
const (
	agentSetupNoSuchDir = "%s: no such directory — --dir takes an existing project directory; create it first, or scaffold one with `civitai app create <name>`"
	agentSetupNotADir   = "%s is not a directory — --dir takes the project ROOT that receives AGENTS.md, not a file"
)

// resolveAgentSetupDir classifies the directory the user named, BEFORE anything
// is read or written.
//
// 🔴 IT IS NOT `resolveProjectDir`, AND THAT IS DELIBERATE. That helper is
// bound by an asserted ledger to the two commands that call `validate.Dir`
// (TestEveryValidateDirCallerGatesOnResolveProjectDir), because its job is to
// classify an APP PROJECT root; this command takes any directory at all and its
// remedies name different next steps. Sharing the helper would either break that
// ledger or drag `agent-setup` into a manifest check it has no business doing.
//
// The three-way branch is the same one item 26 settled on, for the same reason:
//
//	does not exist     -> ErrUsage (exit 2)
//	exists, not a dir  -> ErrUsage (exit 2)
//	a directory        -> nil
//
// Any OTHER stat failure (EACCES on a parent, ENOTDIR partway down) is returned
// UNTAGGED and exits 1 — the generic code item 24 assigns to exactly those
// shapes. os.Stat's *fs.PathError already reads `stat <path>: <reason>`, so it
// is returned bare rather than wrapped into a message that says `stat` twice.
func resolveAgentSetupDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return asUsageError(fmt.Errorf(agentSetupNoSuchDir, dir))
		}
		return err
	}
	if !info.IsDir() {
		return asUsageError(fmt.Errorf(agentSetupNotADir, dir))
	}
	return nil
}

// agentSetupTrackAPIRefusal is the `--track api` answer.
//
// 🔴 IT NAMES WHAT DOES WORK, AND SAYS THE TRACK IS COMING. A bare "not
// supported" would let a reading agent conclude the Civitai API is unsupported
// and stop — which is the opposite of true: both MCP servers are live today and
// the API is documented. So the refusal is an exit 2 that hands over three
// working routes rather than a dead end.
func agentSetupTrackAPIRefusal() error {
	var b strings.Builder
	b.WriteString("--track api is not implemented yet — this release sets up the APP track only.\n\n")
	b.WriteString("What works today for calling the Civitai API from your own service:\n")
	for _, srv := range civitaiMCPServers {
		fmt.Fprintf(&b, "  %-22s %s\n", srv.Name, srv.URL)
	}
	b.WriteString("\nRegister both with `civitai agent-setup --track app` (they are the same two servers) and read:\n")
	b.WriteString("  https://developer.civitai.com/site/\n")
	b.WriteString("  https://developer.civitai.com/orchestration/\n")
	b.WriteString("\nA dedicated API track is coming; until then nothing about the API is unsupported, only this command's setup for it")
	return asUsageError(errors.New(b.String()))
}

// validateTrack accepts `app`, refuses `api` with the pointer above, and treats
// anything else as the usage error it is.
func validateTrack(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case trackApp:
		return trackApp, nil
	case trackAPI:
		return "", agentSetupTrackAPIRefusal()
	default:
		return "", asUsageError(fmt.Errorf("unknown --track %q — expected %s or %s", v, trackApp, trackAPI))
	}
}

// agentCheckJSON is one `--check` row. PUBLISHED CONTRACT.
type agentCheckJSON struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// The MCP-config `changes[].action` values. Named because three of the four are
// reported identically in `--json` and rendered differently on the terminal, and
// a bare string literal at each site is how they drift.
const (
	// actionManual: there is no file to write — `--agent other`, or a
	// user-scoped target with no resolvable home. The human does it.
	actionManual = "manual"
	// actionMerge: an existing config gained the two entries.
	actionMerge = "merge"
	// actionBlocked: the MCP config was REFUSED and nothing was written to it,
	// while the instruction files were still written. See runAgentSetupWrite —
	// this is the degraded run, and it is the one action that comes with
	// `ok: false` and a non-zero exit.
	actionBlocked = "blocked"
)

// agentChangeJSON is one file a write run touched (or, under --dry-run, would).
type agentChangeJSON struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// agentSetupJSON is the `--json` payload for BOTH modes.
//
// 🔴 `--check --json` EMITS EXACTLY `track` / `agent` / `ok` / `checks`, and the
// write path exactly `track` / `agent` / `ok` / `changes` (plus `dryRun`). The
// two are discriminated by which array is present — `omitempty` on both — so the
// check payload is byte-for-byte the shape `developer.civitai.com`'s setup prompt
// was written against, with no key it does not expect.
//
// 🔴 `ok` MEANS DIFFERENT THINGS IN THE TWO MODES, AND SAYING SO IS THE POINT.
// On `--check` it is the verdict, and the structured form of the exit code
// (agentSetupVerdict). On a write run it means the run did what it set out to
// do. A write run for `--agent other` leaves a MANUAL step, and that is reported
// as a `changes` row with the action `manual` rather than by moving `ok` — an
// `ok: false` beside exit 0 is a contradiction a script cannot act on.
//
// 🔴 A WRITE RUN CAN NOW EMIT `ok: false`, AND IT ALWAYS COMES WITH A NON-ZERO
// EXIT. An MCP config that cannot be written no longer aborts the run before the
// instruction files (runAgentSetupWrite), so there is a partial outcome to
// report: the `changes` row carries the action `blocked` with the refusal as its
// reason, `ok` is false, and the command exits 1. The invariant a script needs is
// preserved in the form that matters — `ok: true` still means every step
// happened — and the one it used to rely on ("a payload implies success") is
// gone deliberately, because the alternative was writing nothing at all.
type agentSetupJSON struct {
	Track   string            `json:"track"`
	Agent   string            `json:"agent"`
	OK      bool              `json:"ok"`
	Checks  []agentCheckJSON  `json:"checks,omitempty"`
	Changes []agentChangeJSON `json:"changes,omitempty"`
	DryRun  bool              `json:"dryRun,omitempty"`
}

// agentSetupVerdict is the ONE place `ok` is computed for `--check`.
//
// 🔴 `authenticated` IS REPORTED AND MUST NOT COUNT. We stop before auth on
// purpose (see the file header), so a fresh, correct, unauthenticated setup is a
// SUCCESS — folding that row into the AND would make the command exit 1 for
// every user who has not logged in yet, i.e. exactly the state the setup flow
// leaves them in, and the hosted prompt would report a failure for a run that
// did everything right. TestAuthenticatedIsReportedButNeverFailsTheVerdict pins
// it in both directions.
//
// 🔴 `claude-md` MUST NOT COUNT FOR AN AGENT THAT DOES NOT READ IT, AND THAT IS
// THE SAME SHAPE AGAIN. CLAUDE.md exists only because Claude Code does not read
// AGENTS.md; every other agent reads AGENTS.md directly and the shim is inert
// for them. A Cursor user who deletes the file they were never going to use got
// `ok: false`, exit 1, with every other row green — and `developer.civitai.com`'s
// hosted prompt reports that as a failed setup. The row STAYS (it is true, and a
// user who later opens Claude Code wants it) and it is excluded from the AND, the
// same way the absent Authorization header already is under item 34.
func agentSetupVerdict(checks []agentCheckJSON, agent string) bool {
	for _, c := range checks {
		if checkCountsTowardVerdict(c.Name, agent) && !c.OK {
			return false
		}
	}
	return true
}

// checkCountsTowardVerdict is the ONE predicate both the verdict and the failed
// count consult, so `ok: true` beside "1 check(s) failed" is unreachable.
func checkCountsTowardVerdict(name, agent string) bool {
	switch name {
	case checkAuthenticated:
		return false
	case checkClaudeMD:
		return agent == agentClaude
	default:
		return true
	}
}

func newAgentSetupCmd() *cobra.Command {
	var (
		track   string
		agent   string
		dir     string
		check   bool
		jsonOut bool
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "agent-setup",
		Short: "Set up your coding agent (Claude Code, Cursor, Codex, …) to build Civitai Apps",
		Long: `Configure the coding agent you are using so it can build Civitai Apps.

It does three things, and it never authenticates:

  1. Writes an AGENTS.md managed block into the project — the commands and the
     gotchas an agent cannot infer by reading your code.
  2. Writes a one-line CLAUDE.md containing '@AGENTS.md', ONLY when there is no
     CLAUDE.md already. Claude Code does not read AGENTS.md on its own.
  3. Registers the two Civitai MCP servers in the detected agent's OWN config
     file. The path and the key name differ per agent — .mcp.json/mcpServers for
     Claude Code, .vscode/mcp.json/servers for VS Code, opencode.json/mcp for
     opencode, context_servers for Zed, serverUrl-not-url for Windsurf, a TOML
     [mcp_servers.<name>] table for Codex — which is why this is a command
     rather than a paragraph telling you to hand-write JSON.

NOTHING IS CLOBBERED. An existing AGENTS.md is appended to, or has only its
managed block replaced; an existing CLAUDE.md is left exactly as you wrote it;
an existing MCP config is MERGED into, preserving every other server, every
unknown key, and every key you added to the Civitai entries themselves. A config
file that does not parse is refused by name rather than repaired -- and that
refusal no longer stops AGENTS.md and CLAUDE.md from being written.

The agent is detected from the environment and then from marker files in the
project; --agent overrides it. --agent other prints the config for you to paste
in yourself and writes nothing.

ONE STATED EXCEPTION TO THAT: a JSONC config (Zed's settings.json,
.vscode/mcp.json, opencode.jsonc) is re-encoded, so its comments and key order
are not preserved. The run says so when it happens, rather than refusing the
file -- which is what it used to do, and Zed ships settings.json with comments
in it.

NO CREDENTIAL IS EVER WRITTEN INTO A CONFIG FILE. Most of these files are
project-scoped — .mcp.json, .cursor/mcp.json, .vscode/mcp.json and opencode.json
sit in the repo root and get committed — so an Authorization header holding your
actual token is a secret headed for version control. Instead, for agents whose
vendors document environment-variable interpolation, the header REFERENCES
CIVITAI_TOKEN in that vendor's own spelling (${CIVITAI_TOKEN} for Claude Code,
${env:CIVITAI_TOKEN} for Cursor, VS Code and Windsurf, {env:CIVITAI_TOKEN} for
opencode, and bearer_token_env_var for Codex); export CIVITAI_TOKEN so the agent
resolves it — a token stored only by 'civitai login' is NOT visible to your
agent. For agents that document none — Zed today — no header is written at all
and the output names the exact header to add yourself.

THE TWO SERVERS DIFFER ON ANONYMOUS ACCESS. https://mcp.civitai.com/mcp answers
without a credential, so a header-less config browses models, images and
articles as it stands. https://orchestration.civitai.com/mcp returns 401 until
an Authorization header is present, so generation tools need one either way.

AUTHENTICATION IS YOURS TO RUN. The servers are registered before login on
purpose, and 'civitai login' is a separate store from CIVITAI_TOKEN: it writes
this CLI's own config, which your coding agent does not read.

EXIT CODES: --check exits 1 when a check failed, 0 otherwise. A write run exits
0 on success. A bad --agent, a --dir that does not exist or is not a directory,
and --track api all exit 2. 'authenticated' is REPORTED by --check and never
fails it: an unauthenticated setup is a success, not a failure.`,
		Example: `  civitai agent-setup                       # detect the agent and set it up
  civitai agent-setup --agent cursor        # override the detection
  civitai agent-setup --dir ./my-app        # a project other than the cwd
  civitai agent-setup --dry-run             # print every path, write nothing
  civitai agent-setup --check --json        # verify a setup (scriptable)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Flags first, filesystem second: `--track api` and a mistyped
			// `--agent` are answers about the invocation and must not depend on
			// whether some directory happens to exist.
			resolvedTrack, err := validateTrack(track)
			if err != nil {
				return err
			}
			resolvedAgent := ""
			if strings.TrimSpace(agent) != "" {
				if resolvedAgent, err = validateAgentFlag(agent); err != nil {
					return err
				}
			}
			if err := resolveAgentSetupDir(dir); err != nil {
				return err
			}
			// 🔴 ABSOLUTE, BECAUSE EVERY PATH IN THE PAYLOAD IS BUILT FROM IT.
			// With the default `--dir .` the `--json` `path` fields came out
			// relative (`AGENTS.md`, `.mcp.json`) while the README's documented
			// example shows absolute ones — so a script resolving them against
			// anything but the CLI's own cwd got the wrong file. A path a consumer
			// cannot resolve without also knowing the working directory is not a
			// path.
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return err
			}

			env := liveAgentEnv(absDir)
			if resolvedAgent == "" {
				resolvedAgent = detectAgent(env)
			}

			// config.Load is the only "network-shaped" thing here and it touches
			// no network: it reads ~/.config/civitai/config.yaml plus the
			// CIVITAI_* environment. The token decides whether the MCP entries
			// carry an Authorization header, and nothing else.
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if check {
				return runAgentSetupCheck(cmd.OutOrStdout(), env, resolvedTrack, resolvedAgent, cfg.Token(), jsonOut)
			}
			return runAgentSetupWrite(cmd.OutOrStdout(), env, resolvedTrack, resolvedAgent, cfg.Token(), jsonOut, dryRun)
		},
	}
	cmd.Flags().StringVar(&track, "track", trackApp,
		"which onboarding track: app (the only one implemented) or api")
	cmd.Flags().StringVar(&agent, "agent", "",
		"the coding agent to configure ("+strings.Join(knownAgents(), ", ")+"); detected when omitted")
	cmd.Flags().StringVar(&dir, "dir", ".",
		"the project directory that receives AGENTS.md")
	cmd.Flags().BoolVar(&check, "check", false,
		"verify an existing setup and report each check; writes nothing, exits 1 when a check failed")
	cmd.Flags().BoolVar(&jsonOut, "json", false,
		"emit the result as JSON (scriptable); the exit code is unchanged")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"print every path that would be written and why; write nothing")
	return cmd
}

// ---------------------------------------------------------------------------
// --check
// ---------------------------------------------------------------------------

// runAgentSetupCheck verifies a setup and writes NOTHING. Every filesystem call
// below is a read.
func runAgentSetupCheck(out io.Writer, env agentEnv, track, agent, token string, jsonOut bool) error {
	checks, err := agentSetupChecks(env, agent, token)
	if err != nil {
		return err
	}
	payload := agentSetupJSON{
		Track:  track,
		Agent:  agent,
		OK:     agentSetupVerdict(checks, agent),
		Checks: checks,
	}
	if jsonOut {
		// 🔴 The JSON path carries ZERO styling — internal/ui/CONVENTION.md rule
		// 1 — and goes through the same writeJSON every other command uses.
		if err := writeJSON(out, payload); err != nil {
			return err
		}
	} else {
		printAgentSetupChecks(out, payload)
	}
	if payload.OK {
		return nil
	}
	// The rows are already printed; this error exists only to carry the exit
	// code, so it must not read as a second, competing report of the same facts.
	return fmt.Errorf("%w: %d check(s) failed — the report above lists them; re-run `civitai agent-setup` to fix "+
		"what it can write", ErrAgentSetupIncomplete, countFailedChecks(checks, agent))
}

func countFailedChecks(checks []agentCheckJSON, agent string) int {
	n := 0
	for _, c := range checks {
		if checkCountsTowardVerdict(c.Name, agent) && !c.OK {
			n++
		}
	}
	return n
}

// agentSetupChecks builds the rows, in the published order.
func agentSetupChecks(env agentEnv, agent, token string) ([]agentCheckJSON, error) {
	checks := []agentCheckJSON{
		{Name: checkCLIVersion, OK: true, Detail: version},
	}

	agentsPath := filepath.Join(env.Dir, agentsFilename)
	raw, found, err := readIfExists(agentsPath)
	if err != nil {
		return nil, err
	}
	switch state := agentsMDBlockState(string(raw)); {
	case !found:
		checks = append(checks, agentCheckJSON{Name: checkAgentsMD, OK: false,
			Detail: "missing at " + agentsPath + " — run `civitai agent-setup`"})
	case state == blockDuplicated:
		// 🔴 A SECOND MANAGED BLOCK IS A FAILED ROW, NOT A GREEN ONE. Only the
		// first is ever refreshed, so the stale copy keeps telling the agent
		// whatever it said when it was written, forever, while `--check` reported
		// `ok: true` because a block was found.
		checks = append(checks, agentCheckJSON{Name: checkAgentsMD, OK: false,
			Detail: agentsPath + " has MORE THAN ONE civitai managed block — only the first is ever refreshed, " +
				"so the others go stale silently; delete all but one and re-run `civitai agent-setup`"})
	case state != blockPresent:
		checks = append(checks, agentCheckJSON{Name: checkAgentsMD, OK: false,
			Detail: agentsPath + " has no civitai managed block — run `civitai agent-setup` (it appends, it does not overwrite)"})
	default:
		checks = append(checks, agentCheckJSON{Name: checkAgentsMD, OK: true, Detail: agentsPath})
	}

	// 🔴 THE CLAUDE.md ROW IS SATISFIED BY THE FILE EXISTING, NOT BY ITS
	// CONTENT. This command never modifies an existing CLAUDE.md, so a row that
	// demanded the '@AGENTS.md' import would be permanently red for every author
	// who wrote their own — a gate nobody can clear is a gate everyone learns to
	// ignore. The detail says which case it is; the verdict does not move.
	//
	// 🔴 AND FOR A NON-CLAUDE AGENT IT DOES NOT MOVE THE VERDICT EITHER — see
	// checkCountsTowardVerdict. The detail below says so, because a red row whose
	// absence from the verdict is invisible reads as a bug in the verdict.
	claudePath := filepath.Join(env.Dir, claudeFilename)
	claudeRaw, claudeFound, err := readIfExists(claudePath)
	if err != nil {
		return nil, err
	}
	switch {
	case !claudeFound && agent != agentClaude:
		checks = append(checks, agentCheckJSON{Name: checkClaudeMD, OK: false,
			Detail: "missing at " + claudePath + " — " + agent + " reads " + agentsFilename +
				" directly, so this shim is inert for it and does NOT fail the verdict; " +
				"`civitai agent-setup` writes it for whoever opens Claude Code here later"})
	case !claudeFound:
		checks = append(checks, agentCheckJSON{Name: checkClaudeMD, OK: false,
			Detail: "missing at " + claudePath + " — `civitai agent-setup` writes a one-line `@" + agentsFilename + "` shim"})
	case !strings.Contains(string(claudeRaw), agentsFilename):
		checks = append(checks, agentCheckJSON{Name: checkClaudeMD, OK: true,
			Detail: claudePath + " (yours — it does not reference " + agentsFilename + ", and is left alone)"})
	default:
		checks = append(checks, agentCheckJSON{Name: checkClaudeMD, OK: true, Detail: claudePath})
	}

	mcpRows, err := agentSetupMCPChecks(env, agent)
	if err != nil {
		return nil, err
	}
	checks = append(checks, mcpRows...)

	if token != "" {
		checks = append(checks, agentCheckJSON{Name: checkAuthenticated, OK: true,
			Detail: "token configured — `civitai whoami` verifies it"})
	} else {
		checks = append(checks, agentCheckJSON{Name: checkAuthenticated, OK: false,
			Detail: "no token — run `civitai login`"})
	}
	return checks, nil
}

// agentSetupMCPChecks reports one row per server.
//
// 🔴 "COULD NOT LOOK" IS NOT "NOT REGISTERED", and the details say which. An
// agent this CLI has no target for, and a user-scoped target with no resolvable
// home directory, are both genuinely unfinished setups — but a row reading "not
// registered in " with an empty path is an answer with none of the content.
func agentSetupMCPChecks(env agentEnv, agent string) ([]agentCheckJSON, error) {
	t, known := agentTargets[agent]
	if !known {
		rows := make([]agentCheckJSON, 0, len(civitaiMCPServers))
		for _, srv := range civitaiMCPServers {
			rows = append(rows, agentCheckJSON{Name: srv.Check, OK: false,
				Detail: "agent " + agent + " has no config file this CLI knows — register " + srv.URL +
					" by hand (`civitai agent-setup --agent other` prints the JSON)"})
		}
		return rows, nil
	}
	path, ok := agentConfigPath(env, agent)
	if !ok {
		rows := make([]agentCheckJSON, 0, len(civitaiMCPServers))
		for _, srv := range civitaiMCPServers {
			rows = append(rows, agentCheckJSON{Name: srv.Check, OK: false,
				Detail: agent + "'s MCP config is user-scoped and this CLI could not resolve a home directory — set HOME and re-run"})
		}
		return rows, nil
	}
	registered, err := mcpRegisteredServers(path, t)
	if err != nil {
		return nil, err
	}
	rows := make([]agentCheckJSON, 0, len(civitaiMCPServers))
	for _, srv := range civitaiMCPServers {
		if registered[srv.Name] {
			rows = append(rows, agentCheckJSON{Name: srv.Check, OK: true, Detail: path})
			continue
		}
		rows = append(rows, agentCheckJSON{Name: srv.Check, OK: false,
			Detail: "not registered in " + path})
	}
	return rows, nil
}

// printAgentSetupChecks renders the human view from the SAME payload `--json`
// emits, so the two cannot disagree about a row or the verdict.
func printAgentSetupChecks(w io.Writer, payload agentSetupJSON) {
	st := ui.For(w)
	fmt.Fprintf(w, "Checking the %s track setup for %s\n\n", payload.Track, st.Bold(payload.Agent))
	for _, c := range payload.Checks {
		label := fmt.Sprintf("%-15s", c.Name)
		if c.OK {
			fmt.Fprintln(w, "  "+st.Success(label+c.Detail))
			continue
		}
		if !checkCountsTowardVerdict(c.Name, payload.Agent) {
			// 🔴 A WARNING, NOT A FAILURE — the glyph has to agree with the
			// verdict, and it is derived from the SAME predicate rather than
			// re-listing which rows are exempt. Rendering one of these rows like
			// the others tells a reader the run failed on a row that deliberately
			// cannot fail it.
			fmt.Fprintln(w, "  "+st.Warn(label+c.Detail))
			continue
		}
		fmt.Fprintln(w, "  "+st.ErrorMsg(label+c.Detail))
	}
	fmt.Fprintln(w)
	if payload.OK {
		fmt.Fprintln(w, st.Success("Setup is complete."))
	} else {
		fmt.Fprintln(w, st.ErrorMsg("Setup is incomplete — re-run `civitai agent-setup`."))
	}
	fmt.Fprintln(w, st.Dim("`authenticated` is reported but never fails this check — setup stops before login on purpose."))
	if payload.Agent != agentClaude {
		fmt.Fprintln(w, st.Dim("`claude-md` likewise: "+payload.Agent+" reads "+agentsFilename+" directly, so the shim is inert for it."))
	}
}

// ---------------------------------------------------------------------------
// the write run
// ---------------------------------------------------------------------------

// runAgentSetupWrite performs (or, under dryRun, describes) the whole setup.
//
// 🔴 THE PLAN IS BUILT THE SAME WAY IN BOTH MODES. `--dry-run` runs exactly the
// planning code the real run does and then skips the writes, so it cannot report
// a path or an action the write path would not take. A separate dry-run renderer
// is how a `--dry-run` starts lying.
// 🔴 AN MCP REFUSAL DEGRADES, IT DOES NOT ABORT — AND THAT IS A FIX FOR A
// SHIPPED BUG. `planMCPConfig` runs before ANY write, so a config file this
// command would not touch took the whole run down with it: `--agent zed` on a
// stock Zed install (its settings.json ships with comments) exited 1 having
// written NOTHING, leaving the project dir empty — no AGENTS.md, no CLAUDE.md —
// with a message blaming the user's file. The instruction files have nothing to
// do with the MCP config, so they are written regardless and the MCP failure is
// reported as its own row.
//
// The run still exits NON-ZERO, and `ok` is false. Degrading is not pretending:
// a script that reads `ok: true` and exit 0 must be able to conclude every step
// happened, so a partial run says so in both channels at once.
func runAgentSetupWrite(out io.Writer, env agentEnv, track, agent, token string, jsonOut, dryRun bool) error {
	var changes []agentChangeJSON

	agentsPath, agentsContent, agentsAction, err := planAgentsMD(env.Dir)
	if err != nil {
		return err
	}
	changes = append(changes, agentChangeJSON{
		Path: agentsPath, Action: string(agentsAction), Reason: agentsMDReason(agentsAction)})

	claudePath, claudeContent, claudeAction, err := planClaudeMD(env.Dir)
	if err != nil {
		return err
	}
	changes = append(changes, agentChangeJSON{
		Path: claudePath, Action: string(claudeAction), Reason: claudeMDReason(claudeAction)})

	mcpPath, mcpData, mcpChange, mcpErr := planMCPConfig(env, agent, token)
	if mcpErr != nil {
		mcpData = nil
		mcpChange = agentChangeJSON{
			Path:   mcpPath,
			Action: actionBlocked,
			Reason: mcpErr.Error(),
		}
	}
	changes = append(changes, mcpChange)

	if !dryRun {
		if agentsAction != actionUnchanged {
			if err := writeProjectFile(agentsPath, agentsContent); err != nil {
				return err
			}
		}
		if claudeAction == actionCreate {
			if err := writeProjectFile(claudePath, claudeContent); err != nil {
				return err
			}
		}
		if mcpData != nil {
			if err := writeMCPConfig(mcpPath, mcpData); err != nil {
				return err
			}
		}
	}

	if jsonOut {
		if err := writeJSON(out, agentSetupJSON{
			Track: track, Agent: agent, OK: mcpErr == nil, Changes: changes, DryRun: dryRun,
		}); err != nil {
			return err
		}
	} else {
		printAgentSetupWrite(out, env, agent, token, changes, dryRun)
	}
	if mcpErr != nil {
		return fmt.Errorf("%w: the instruction files were written; the MCP config was not — %v",
			ErrAgentSetupIncomplete, mcpErr)
	}
	return nil
}

func agentsMDReason(a fileAction) string {
	switch a {
	case actionCreate:
		return "no " + agentsFilename + " here yet"
	case actionAppend:
		return "an " + agentsFilename + " exists without the managed block — appending, every existing byte is kept"
	case actionReplace:
		return "the managed block is refreshed; everything outside the markers is untouched"
	default:
		return "the managed block is already current"
	}
}

func claudeMDReason(a fileAction) string {
	if a == actionCreate {
		return "Claude Code does not read " + agentsFilename + " — this is a one-line `@" + agentsFilename + "` import"
	}
	return "you already have a " + claudeFilename + " — it is never modified"
}

// planMCPConfig renders the MCP config write, or the row explaining why there is
// none. It returns nil data when nothing is to be written, which is how the
// caller distinguishes "write this" from "tell the human".
func planMCPConfig(env agentEnv, agent, token string) (string, []byte, agentChangeJSON, error) {
	t, known := agentTargets[agent]
	if !known {
		return "", nil, agentChangeJSON{
			Path:   "",
			Action: actionManual,
			Reason: "agent " + agent + " has no MCP config file this CLI knows — the servers are printed below for you to paste in",
		}, nil
	}
	path, ok := agentConfigPath(env, agent)
	if !ok {
		return "", nil, agentChangeJSON{
			Path:   "",
			Action: actionManual,
			Reason: agent + "'s MCP config is user-scoped and no home directory could be resolved — set HOME and re-run",
		}, nil
	}
	data, err := renderMCPConfig(path, t, token)
	if err != nil {
		return path, nil, agentChangeJSON{}, err
	}
	raw, existed, err := readIfExists(path)
	if err != nil {
		return path, nil, agentChangeJSON{}, err
	}
	action := string(actionCreate)
	reason := "registers both Civitai MCP servers"
	if existed {
		action = actionMerge
		reason = "merges both Civitai MCP servers in, preserving every other server and key"
	}
	// 🔴 THE ONE THING THE MERGE DOES NOT PRESERVE IS NAMED WHERE IT HAPPENS. A
	// JSONC config is decoded and re-encoded, so its comments and key order are
	// gone from the file the user opens next. The alternative that shipped —
	// refusing every commented file — made `--agent zed` unusable on a stock Zed
	// install. Losing a comment while saying so beats refusing while blaming the
	// user, but only if it is actually said.
	if jsonMergeDropsComments(raw, t) {
		reason += "; comments and key order in that file are NOT preserved by this merge (it is decoded and re-encoded)"
	}
	reason += "; " + mcpAuthReason(t, token != "")
	if t.Caveat != "" {
		reason += "; note: " + t.Caveat
	}
	return path, data, agentChangeJSON{Path: path, Action: action, Reason: reason}, nil
}

// mcpAuthReason states, in one clause, what this run did about authentication —
// and it is the same sentence in `--dry-run`, in `--json` and on the terminal.
//
// 🔴 IT NEVER SAYS "THE TOKEN WAS WRITTEN", BECAUSE IT NEVER IS. The three
// outcomes are: a reference to CIVITAI_TOKEN in the agent's own documented
// spelling, Codex's variable-NAME key, or no header at all — and the last one is
// reported for exactly what it reaches (mcpAnonymityNote), not as "anonymous
// read tools work", which was true of one of the two servers.
func mcpAuthReason(t agentTarget, hasToken bool) string {
	if !hasToken {
		return "no Authorization header — no token is configured (" + mcpAnonymityNote() + ")"
	}
	if t.EnvBearerKey != "" {
		return "auth reads " + tokenEnvVar + " via `" + t.EnvBearerKey + "` — no credential is written to disk"
	}
	if t.EnvHeaderSyntax != "" {
		return "the Authorization header references " + tokenEnvVar + " as `" + t.EnvHeaderSyntax +
			"` — no credential is written to disk; export " + tokenEnvVar + " for the agent to resolve it"
	}
	return "no Authorization header — " + t.Agent + " documents no way to read one from the environment, " +
		"and this command never writes a credential to a config file (" + mcpAnonymityNote() + ")"
}

// tokenIsExported reports whether the AGENT will be able to resolve the header
// this command wrote: the token has to be in the PROCESS environment, not merely
// in this CLI's config file.
//
// 🔴 `civitai login` DOES NOT SATISFY IT, AND THAT IS THE POINT. `config.Load()`
// reads `~/.config/civitai/config.yaml`, which no coding agent opens. A user who
// logged in and never exported CIVITAI_TOKEN gets the header written and every
// request 401s with an EMPTY bearer — VS Code, Cursor, Windsurf and opencode all
// resolve an unset variable to "" (verified in VS Code's variableResolver.ts).
// Measured on this command: token in config.yaml, CIVITAI_TOKEN unset, output
// said "Already authenticated". It is a missing export, and it reads as a bad
// token unless the output says which.
func tokenIsExported(env agentEnv) bool {
	return strings.TrimSpace(env.Vars[tokenEnvVar]) != ""
}

// printAgentSetupWrite renders the human report and the next-step block.
func printAgentSetupWrite(w io.Writer, env agentEnv, agent, token string, changes []agentChangeJSON, dryRun bool) {
	st := ui.For(w)
	verb := "Configured"
	if dryRun {
		verb = "Would configure"
	}
	fmt.Fprintf(w, "%s %s for Civitai App development in %s\n\n", verb, st.Bold(agent), env.Dir)

	for _, c := range changes {
		switch {
		case c.Action == actionManual:
			fmt.Fprintln(w, "  "+st.Warn("manual   "+c.Reason))
		case c.Action == actionBlocked:
			// 🔴 RENDERED AS A FAILURE, BECAUSE IT IS ONE. The instruction files
			// above it succeeded; this row is the part that did not, and the run
			// exits non-zero. A dim or warn glyph here would read as "handled".
			fmt.Fprintf(w, "  %s %s\n", st.ErrorMsg(fmt.Sprintf("%-9s", c.Action)), c.Path)
			fmt.Fprintf(w, "  %s\n", st.ErrorMsg("          "+c.Reason))
		case c.Action == string(actionUnchanged), c.Action == string(actionKeep):
			fmt.Fprintf(w, "  %s %s\n", st.Dim(fmt.Sprintf("%-9s", c.Action)), c.Path)
			fmt.Fprintf(w, "  %s\n", st.Dim("          "+c.Reason))
		default:
			fmt.Fprintf(w, "  %s %s\n", st.Success(fmt.Sprintf("%-9s", c.Action)), c.Path)
			fmt.Fprintf(w, "  %s\n", st.Dim("          "+c.Reason))
		}
	}

	target, known := agentTargets[agent]
	if !known {
		fmt.Fprintln(w, "\nPaste this into your agent's MCP config:")
		fmt.Fprintln(w, mcpPasteBlock())
		fmt.Fprintln(w, "The file and the top-level key differ per agent:")
		for _, line := range mcpKeyDifferences() {
			fmt.Fprintln(w, "  "+st.Dim(line))
		}
	} else {
		fmt.Fprintln(w, "\nMCP servers:")
		for _, srv := range civitaiMCPServers {
			fmt.Fprintf(w, "  %-22s %s\n", srv.Name, srv.URL)
			fmt.Fprintf(w, "  %s\n", st.Dim(strings.Repeat(" ", 22)+" "+srv.What))
			// 🔴 THE PER-SERVER ACCESS FACT, ON THE SERVER'S OWN ROW. Stating it
			// once at the bottom is how "both servers work anonymously" survived
			// in seven places: a reader matches the sentence to whichever server
			// they were looking at.
			if !srv.Anonymous {
				fmt.Fprintf(w, "  %s%s\n", strings.Repeat(" ", 23), st.Warn("needs an Authorization header — "+
					"this one returns 401 without a credential"))
			}
		}
	}

	printAgentSetupAuthNote(w, st, agent, target, known, token != "")

	if known && target.Caveat != "" {
		fmt.Fprintf(w, "\n  %s\n", st.Warn("Note: "+target.Caveat))
	}

	fmt.Fprintln(w, "\nNext:")
	n := 1
	if dryRun {
		fmt.Fprintf(w, "  %d. Re-run without --dry-run — nothing above was written.\n", n)
	} else {
		fmt.Fprintf(w, "  %d. Restart %s so it loads the MCP servers.\n", n, agent)
	}
	// 🔴 EXPORTING CIVITAI_TOKEN IS A SEPARATE STEP FROM `civitai login`, AND
	// BOTH LINES STAY. `civitai login` writes ~/.config/civitai/config.yaml,
	// which this CLI reads and the coding agent does not; the agent resolves the
	// header from the PROCESS environment. Collapsing the two would leave the
	// user with a token the agent cannot see, which reads as a broken credential.
	exported := tokenIsExported(env)
	if token != "" && known && (target.EnvHeaderSyntax != "" || target.EnvBearerKey != "") {
		n++
		fmt.Fprintf(w, "  %d. Export the token where %s can see it — the config above references it by name:\n", n, agent)
		fmt.Fprintf(w, "       %s\n", st.Code("export "+tokenEnvVar+"=<your token>"))
		if exported {
			fmt.Fprintf(w, "     %s\n", st.Dim(tokenEnvVar+" is already set in this shell. Put it in your shell "+
				"profile so the agent inherits it too."))
		} else {
			// 🔴 THE MEASURED CASE, NAMED. A token in ~/.config/civitai/config.yaml
			// satisfies this command's own gate and is invisible to the agent, so
			// the header it just wrote resolves to the empty string and every
			// request 401s. Saying nothing here is what makes that read as a bad
			// credential and sends the user to re-mint a token that was fine.
			fmt.Fprintf(w, "     %s\n", st.Warn(tokenEnvVar+" is NOT set in this environment. The header above "+
				"references it by name, so until you export it "+agent+" resolves it to an empty string and the "+
				"requests 401 — that is a missing export, not a bad token."))
		}
	}
	// 🔴 THE LAST LINE NAMES `civitai login`, AND THIS COMMAND NEVER RUNS IT.
	// Authentication is the human's step: nothing here opens a browser, prompts
	// for a credential, or stores one.
	n++
	switch {
	case token == "":
		fmt.Fprintf(w, "  %d. Run %s to authenticate, then re-run %s so the MCP entries pick up the reference.\n",
			n, st.Code("civitai login"), st.Code("civitai agent-setup"))
	case !exported:
		// 🔴 NOT "ALREADY AUTHENTICATED". This CLI is; the agent is not, and this
		// is the line that used to say otherwise.
		fmt.Fprintf(w, "  %d. This CLI has a token, but %s cannot see it — %s is unset. Export it (step above); "+
			"%s alone does not reach your agent.\n", n, agent, tokenEnvVar, st.Code("civitai login"))
	default:
		fmt.Fprintf(w, "  %d. Already authenticated — run %s again only if you need different scopes.\n",
			n, st.Code("civitai login"))
	}
}

// printAgentSetupAuthNote says what was — and was not — written about auth.
//
// 🔴 A HEADER-LESS CONFIG IS PRESENTED AS WHAT IT REACHES, NOT AS "WORKING" AND
// NOT AS "BROKEN". It browses the SITE server today, and the ORCHESTRATION
// server answers it 401. Overstating it in the first direction — which this
// function did, in the words "the read tools work anonymously, so this setup is
// usable as it stands" — leaves a VS Code / Zed / no-token user believing half a
// registration is a finished one. Overstating it in the second invites them to
// paste a literal token into a file that gets committed, which is the leak item
// 34 exists to prevent. Both halves of the sentence are load-bearing.
func printAgentSetupAuthNote(w io.Writer, st ui.Styler, agent string, t agentTarget, known, hasToken bool) {
	fmt.Fprintln(w, "\nAuthentication:")
	fmt.Fprintf(w, "  %s\n", st.Dim("No credential is ever written into an agent config file — "+
		".mcp.json, .cursor/mcp.json, .vscode/mcp.json and opencode.json live in the repo and get committed."))

	switch {
	case !known:
		fmt.Fprintf(w, "  The block above carries no Authorization header, because the header value has to be\n")
		fmt.Fprintf(w, "  spelled the way YOUR agent reads environment variables, and this CLI does not know which\n")
		fmt.Fprintf(w, "  agent that is. Add it yourself if you want authenticated tools:\n")
		fmt.Fprintf(w, "       %s\n", st.Code(`"headers": {"Authorization": "Bearer <env-var reference>"}`))
		fmt.Fprintf(w, "  %s\n", st.Dim("Known spellings: ${"+tokenEnvVar+"} (Claude Code), ${env:"+tokenEnvVar+
			"} (Cursor, VS Code, Windsurf), {env:"+tokenEnvVar+"} (opencode)."))
		fmt.Fprintf(w, "  %s\n", st.Warn("Without it: "+mcpAnonymityNote()+"."))
	case !hasToken:
		fmt.Fprintf(w, "  No token is configured, so no Authorization header was written.\n")
		// 🔴 THE NOTE IS PRINTED VERBATIM. It opens with a SERVER NAME, and a
		// sentence-casing pass would silently rename `civitai` to `Civitai` — a
		// key the user has to type.
		fmt.Fprintf(w, "  %s\n", st.Warn("Access without one: "+mcpAnonymityNote()+"."))
		fmt.Fprintf(w, "  Run %s and re-run this command to reference it.\n", st.Code("civitai login"))
	case t.EnvBearerKey != "":
		fmt.Fprintf(w, "  Each entry carries %s, so %s reads the token from your environment\n",
			st.Code(t.EnvBearerKey+" = \""+tokenEnvVar+"\""), agent)
		fmt.Fprintf(w, "  at request time rather than from the file.\n")
	case t.EnvHeaderSyntax != "":
		fmt.Fprintf(w, "  Each entry carries %s, which %s resolves from your\n",
			st.Code(`"Authorization": "Bearer `+t.EnvHeaderSyntax+`"`), agent)
		fmt.Fprintf(w, "  environment at request time rather than from the file.\n")
	default:
		// 🔴 NAMED, NOT SILENT. Zed documents no way to read a header from the
		// environment, so there is nothing this command can write that is both
		// authenticated and credential-free. Saying so — with the exact file and
		// the exact header — is the whole remedy; a silent omission would read as
		// a bug in this command.
		fmt.Fprintf(w, "  %s documents no way to read a header from the environment, so no Authorization header\n", agent)
		fmt.Fprintf(w, "  was written. %s\n", st.Warn("Access without one: "+mcpAnonymityNote()+"."))
		fmt.Fprintf(w, "  To authenticate, add this yourself to each Civitai entry in that file:\n")
		fmt.Fprintf(w, "       %s\n", st.Code(`"`+t.HeadersKey+`": {"Authorization": "Bearer <your token>"}`))
		fmt.Fprintf(w, "  %s\n", st.Dim("That file is yours; note that a literal token in it is a credential on disk, "+
			"so keep it out of version control."))
	}
}
