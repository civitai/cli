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
//
// 🔴 THERE IS A THIRD SHAPE, AND IT EXISTS SO THE FLAG HAS NO SILENT OUTCOME.
// `--json` used to emit ZERO BYTES for any failure that happened before a row
// could be built — an AGENTS.md that is a directory, a duplicated managed block,
// a config root that cannot be resolved — while the enumerated failures emitted a
// full payload. Four instances of that were closed one at a time and the class
// stayed open, so it is now closed at the ONE place stdout is written
// (agentSetupEmitter): any error that escapes without a payload is emitted as
// `track` / `agent` / `ok: false` / `error`, with NEITHER array. So a consumer
// discriminates on which of the three is present — `checks`, `changes`, or
// `error` — and never on stdout being empty. `error` is present ONLY on that
// envelope: a run that produced rows says what went wrong IN the rows.
//
// 🔴 THE ONE EXCEPTION IS EXIT 2, AND IT IS STATED RATHER THAN ABSOLUTE. A
// mistake about the INVOCATION — an unknown `--agent`, a `--dir` that is not a
// directory, `--track api` — is reported on stderr like every other command's
// usage error, because there is no run to describe. `--json` promises a payload
// for every run that STARTED, not for a command line that never named one.
type agentSetupJSON struct {
	Track   string            `json:"track"`
	Agent   string            `json:"agent"`
	OK      bool              `json:"ok"`
	Checks  []agentCheckJSON  `json:"checks,omitempty"`
	Changes []agentChangeJSON `json:"changes,omitempty"`
	DryRun  bool              `json:"dryRun,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// agentSetupEmitter is the ONE place `agent-setup` writes stdout, and the only
// thing that knows whether a payload got there. Both modes emit through it, so
// the "no silent outcome" rule above is a property of the code path rather than
// of an enumeration somebody has to keep complete.
type agentSetupEmitter struct {
	w       io.Writer
	json    bool
	emitted bool
}

// emit writes the payload in whichever channel the flags asked for and records
// that a payload happened. render is the human view of the SAME payload.
func (e *agentSetupEmitter) emit(payload agentSetupJSON, render func(io.Writer, agentSetupJSON)) error {
	e.emitted = true
	if e.json {
		// 🔴 The JSON path carries ZERO styling — internal/ui/CONVENTION.md rule
		// 1 — and goes through the same writeJSON every other command uses.
		return writeJSON(e.w, payload)
	}
	render(e.w, payload)
	return nil
}

// envelope is the fallback: an error reached the top with no rows behind it. It
// is a no-op unless `--json` was asked for and nothing was emitted, so it cannot
// double-print or invent a second report of a run that already described itself.
func (e *agentSetupEmitter) envelope(track, agent string, err error) {
	if !e.json || e.emitted || err == nil {
		return
	}
	e.emitted = true
	_ = writeJSON(e.w, agentSetupJSON{Track: track, Agent: agent, OK: false, Error: err.Error()})
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
.vscode/mcp.json, opencode.jsonc) is re-encoded, so its comments, its trailing
commas and its key order are not preserved. The run says so when it happens,
rather than refusing the file -- which is what it used to do, and Zed ships
settings.json with comments in it and reads a trailing comma back happily.

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
0 when every step happened and 1 when one did not -- a config that does not
parse, a destination it will not write (for ANY of the three files), a file it
could not write -- and each of those is a 'blocked' row in the report with 'ok'
false, never a silent success. --dry-run reports the same rows and the same exit
code without writing. THE ONE STEP THAT DOES NOT HAPPEN AND STILL EXITS 0 is the
'manual' row: --agent other, or a user-scoped agent with no resolvable home,
where there is no file for this CLI to write and the config is printed for you to
paste instead. A bad --agent, a --dir that does not exist or is not a directory,
and --track api all exit 2. 'authenticated' is REPORTED by --check and never
fails it: an unauthenticated setup is a success, not a failure.

--json HAS THREE SHAPES AND NO SILENT ONE: 'checks' for --check, 'changes' for a
write or dry run, and -- when a run failed before either could be built -- an
'error' string with neither array. Discriminate on which is present. The one
exception is exit 2, a mistake about the invocation rather than a run, which is
reported on stderr like every other command's usage error.`,
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
			// Everything above is a mistake about the INVOCATION and exits 2 on
			// stderr; everything below is a RUN, and a run always describes itself
			// on stdout — see agentSetupJSON's third shape.
			emit := &agentSetupEmitter{w: cmd.OutOrStdout(), json: jsonOut}
			runErr := runAgentSetup(emit, &resolvedAgent, resolvedTrack, dir, check, dryRun)
			emit.envelope(resolvedTrack, resolvedAgent, runErr)
			return runErr
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

// runAgentSetup is the body of the command, split out so the ONE stdout emitter
// wraps every failure it can reach. resolvedAgent is a pointer because detection
// happens in here and the envelope wants the answer even when the run then
// failed.
func runAgentSetup(emit *agentSetupEmitter, resolvedAgent *string, track, dir string, check, dryRun bool) error {
	// 🔴 ABSOLUTE, BECAUSE EVERY PATH IN THE PAYLOAD IS BUILT FROM IT.
	// With the default `--dir .` the `--json` `path` fields came out relative
	// (`AGENTS.md`, `.mcp.json`) while the README's documented example shows
	// absolute ones — so a script resolving them against anything but the CLI's
	// own cwd got the wrong file. A path a consumer cannot resolve without also
	// knowing the working directory is not a path.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	env := liveAgentEnv(absDir)
	if *resolvedAgent == "" {
		*resolvedAgent = detectAgent(env)
	}

	// config.Load is the only "network-shaped" thing here and it touches no
	// network: it reads ~/.config/civitai/config.yaml plus the CIVITAI_*
	// environment. The token decides whether the MCP entries carry an
	// Authorization header, and nothing else.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if check {
		return runAgentSetupCheck(emit, env, track, *resolvedAgent, cfg.Token())
	}
	return runAgentSetupWrite(emit, env, track, *resolvedAgent, cfg.Token(), dryRun)
}

// ---------------------------------------------------------------------------
// --check
// ---------------------------------------------------------------------------

// runAgentSetupCheck verifies a setup and writes NOTHING. Every filesystem call
// below is a read.
func runAgentSetupCheck(emit *agentSetupEmitter, env agentEnv, track, agent, token string) error {
	checks := agentSetupChecks(env, agent, token)
	payload := agentSetupJSON{
		Track:  track,
		Agent:  agent,
		OK:     agentSetupVerdict(checks, agent),
		Checks: checks,
	}
	if err := emit.emit(payload, printAgentSetupChecks); err != nil {
		return err
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
//
// 🔴 IT RETURNS NO ERROR, AND THAT IS THE SHAPE RATHER THAN A SIMPLIFICATION. An
// `--check` run that cannot READ one of the three files used to return the read
// error up to `main`, which printed it on stderr and left `--json`'s stdout
// EMPTY at exit 1 — the same defect the MCP rows already fixed one case of, with
// `AGENTS.md`/`CLAUDE.md` still open. Measured on a project where `AGENTS.md` is
// a directory. A function that CAN return an error here is a function that will
// grow another silent exit, so it cannot: every failure is a row saying which
// file it was and why it could not be read.
func agentSetupChecks(env agentEnv, agent, token string) []agentCheckJSON {
	checks := []agentCheckJSON{
		{Name: checkCLIVersion, OK: true, Detail: version},
	}

	agentsPath := filepath.Join(env.Dir, agentsFilename)
	raw, found, err := readIfExists(agentsPath)
	switch state := agentsMDBlockState(string(raw)); {
	case err != nil:
		checks = append(checks, agentCheckJSON{Name: checkAgentsMD, OK: false,
			Detail: "could not read " + agentsPath + ": " + err.Error()})
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
	claudeRaw, claudeFound, claudeErr := readIfExists(claudePath)
	switch {
	case claudeErr != nil:
		checks = append(checks, agentCheckJSON{Name: checkClaudeMD, OK: false,
			Detail: "could not read " + claudePath + ": " + claudeErr.Error()})
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

	checks = append(checks, agentSetupMCPChecks(env, agent)...)

	if token != "" {
		checks = append(checks, agentCheckJSON{Name: checkAuthenticated, OK: true,
			Detail: "token configured — `civitai whoami` verifies it"})
	} else {
		checks = append(checks, agentCheckJSON{Name: checkAuthenticated, OK: false,
			Detail: "no token — run `civitai login`"})
	}
	return checks
}

// agentSetupMCPChecks reports one row per server.
//
// 🔴 "COULD NOT LOOK" IS NOT "NOT REGISTERED", and the details say which. An
// agent this CLI has no target for, and a user-scoped target with no resolvable
// home directory, are both genuinely unfinished setups — but a row reading "not
// registered in " with an empty path is an answer with none of the content.
func agentSetupMCPChecks(env agentEnv, agent string) []agentCheckJSON {
	t, known := agentTargets[agent]
	if !known {
		rows := make([]agentCheckJSON, 0, len(civitaiMCPServers))
		for _, srv := range civitaiMCPServers {
			rows = append(rows, agentCheckJSON{Name: srv.Check, OK: false,
				Detail: "agent " + agent + " has no config file this CLI knows — register " + srv.URL +
					" by hand (`civitai agent-setup --agent other` prints the JSON)"})
		}
		return rows
	}
	path, ok := agentConfigPath(env, agent)
	if !ok {
		rows := make([]agentCheckJSON, 0, len(civitaiMCPServers))
		for _, srv := range civitaiMCPServers {
			rows = append(rows, agentCheckJSON{Name: srv.Check, OK: false,
				Detail: agent + "'s MCP config is user-scoped and this CLI could not resolve a home directory — set HOME and re-run"})
		}
		return rows
	}
	registered, err := mcpRegisteredServers(path, t)
	if err != nil {
		// 🔴 A CONFIG THIS COMMAND CANNOT READ IS A THIRD "COULD NOT LOOK", AND IT
		// USED TO BE THE ONE THAT EMITTED NOTHING. `--check --json` against an
		// unparseable config returned this error up to `main`, which printed
		// `Error: …` on stderr and left stdout EMPTY at exit 1 — while the two
		// could-not-look cases above it returned rows. `developer.civitai.com`'s
		// hosted prompt reads this payload, so an empty stdout is indistinguishable
		// to it from a usage error. The refusal is carried in the rows' detail
		// instead, where the other two cases already put theirs.
		rows := make([]agentCheckJSON, 0, len(civitaiMCPServers))
		for _, srv := range civitaiMCPServers {
			rows = append(rows, agentCheckJSON{Name: srv.Check, OK: false,
				Detail: "could not read " + agent + "'s MCP config: " + err.Error()})
		}
		return rows
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
	return rows
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
func runAgentSetupWrite(emit *agentSetupEmitter, env agentEnv, track, agent, token string, dryRun bool) error {
	var changes []agentChangeJSON

	// 🔴 A PLAN-TIME REFUSAL IS A `blocked` ROW FOR *EVERY* FILE, NOT JUST THE
	// MCP CONFIG. Round 2 moved the destination check into `planMCPConfig` and
	// left the two instruction files reaching it only at WRITE time, so the exact
	// defect it fixed survived one file over: with `AGENTS.md` a broken symlink,
	// `--dry-run --json` reported `create` / `ok: true` / exit 0 while the real
	// run reported `blocked` / `ok: false` / exit 1. Measured. And a plan error
	// here used to be `return err`, which is the OTHER defect — `--json` emitting
	// zero bytes. Both are closed by giving all three files the same shape: plan,
	// and if the plan refuses, that file's row carries the refusal.
	planRow := func(path string, action fileAction, reason string, err error) agentChangeJSON {
		if err != nil {
			return agentChangeJSON{Path: path, Action: actionBlocked, Reason: err.Error()}
		}
		return agentChangeJSON{Path: path, Action: string(action), Reason: reason}
	}

	agentsPath, agentsContent, agentsAction, agentsErr := planAgentsMD(env.Dir)
	agentsRow := len(changes)
	changes = append(changes, planRow(agentsPath, agentsAction, agentsMDReason(agentsAction), agentsErr))

	claudePath, claudeContent, claudeAction, claudeErr := planClaudeMD(env.Dir)
	claudeRow := len(changes)
	changes = append(changes, planRow(claudePath, claudeAction, claudeMDReason(claudeAction), claudeErr))

	mcpPath, mcpData, mcpChange, mcpAuth, mcpErr := planMCPConfig(env, agent, token)
	if mcpErr != nil {
		mcpData = nil
		mcpAuth = mcpAuthCoverage{}
		mcpChange = agentChangeJSON{
			Path:   mcpPath,
			Action: actionBlocked,
			Reason: mcpErr.Error(),
		}
	}
	mcpRow := len(changes)
	changes = append(changes, mcpChange)

	// 🔴 A WRITE THAT FAILS IS A `blocked` ROW, NOT AN EMPTY STDOUT — AND EACH
	// FILE IS ATTEMPTED INDEPENDENTLY. Both halves are fixes for a measured
	// defect. `--json` used to emit ZERO BYTES whenever a write failed (an
	// unwritable directory, a full disk), so a consumer could not tell a partial
	// run from a usage error, while the plan-time refusal one line above it
	// emitted a full payload — three contracts on one flag. And the old code
	// returned on the FIRST failure, so a payload emitted after it would have
	// claimed actions for files nothing ever tried to write. The three files are
	// independent of each other, exactly as the MCP refusal is independent of the
	// instruction files, so each is attempted and each reports its own outcome.
	if !dryRun {
		// 🔴 `attempt` NEVER SHORT-CIRCUITS, AND THAT IS THE INVARIANT. Restoring
		// an abort-on-first-failure here (`if anyBlocked(changes) { return }`)
		// emits a payload claiming `create` for files nothing ever tried to write.
		// TestAFirstWriteFailureDoesNotStopTheLaterOnes is the guard: its fixture
		// fails the FIRST write, which is the only arrangement in which a
		// short-circuit is observable at all.
		attempt := func(i int, do func() error) {
			if changes[i].Action == actionBlocked {
				// The plan already refused this file; there is nothing to try, and
				// re-reporting it would overwrite the plan's own reason.
				return
			}
			if err := do(); err != nil {
				changes[i].Action = actionBlocked
				changes[i].Reason = err.Error()
			}
		}
		attempt(agentsRow, func() error {
			if agentsAction == actionUnchanged {
				return nil
			}
			return writeProjectFile(agentsPath, agentsContent)
		})
		attempt(claudeRow, func() error {
			if claudeAction != actionCreate {
				return nil
			}
			return writeProjectFile(claudePath, claudeContent)
		})
		attempt(mcpRow, func() error {
			if mcpData == nil {
				return nil
			}
			return writeMCPConfig(mcpPath, mcpData)
		})
	}

	// 🔴 `ok`, THE EXIT CODE AND THE ERROR ALL READ THE ROWS. One predicate, so
	// `ok: true` beside a `blocked` row is unreachable — and so is the case the
	// three-way version missed: a dry run whose PLAN refused a file exited 0
	// because no write had failed and `mcpErr` was nil.
	blocked := blockedRowSummary(changes)
	if err := emit.emit(agentSetupJSON{
		Track: track, Agent: agent, OK: blocked == "", Changes: changes, DryRun: dryRun,
	}, func(w io.Writer, p agentSetupJSON) {
		printAgentSetupWrite(w, env, agent, token, p.Changes, mcpAuth, dryRun)
	}); err != nil {
		return err
	}
	if blocked == "" {
		return nil
	}
	// 🔴 A DRY RUN DID NOT WRITE THE OTHER FILES, AND MUST NOT SAY IT DID. The
	// sentence that shipped read "the instruction files were written; the MCP
	// config was not" unconditionally, and round 2's fix ROUTED A NEW CASE INTO
	// IT: a broken-symlink destination used to exit 0 under `--dry-run` and now
	// exits 1, with that claim attached to a run that wrote nothing at all.
	if dryRun {
		return fmt.Errorf("%w: %s — this is a dry run, so nothing was written either way; "+
			"the report above is what the real run would do", ErrAgentSetupIncomplete, blocked)
	}
	return fmt.Errorf("%w: %s — the report above lists every step and whether it happened",
		ErrAgentSetupIncomplete, blocked)
}

// blockedRowSummary names what did not happen, from the rows themselves, so the
// error cannot claim a different set of failures from the report above it. It
// returns "" when nothing is blocked, which is what `ok` and the exit code both
// branch on — see runAgentSetupWrite.
func blockedRowSummary(changes []agentChangeJSON) string {
	var parts []string
	for _, c := range changes {
		if c.Action == actionBlocked {
			parts = append(parts, c.Path+": "+c.Reason)
		}
	}
	return strings.Join(parts, "; ")
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
func planMCPConfig(env agentEnv, agent, token string) (string, []byte, agentChangeJSON, mcpAuthCoverage, error) {
	t, known := agentTargets[agent]
	if !known {
		return "", nil, agentChangeJSON{
			Path:   "",
			Action: actionManual,
			Reason: "agent " + agent + " has no MCP config file this CLI knows — the servers are printed below for you to paste in",
		}, mcpAuthCoverage{}, nil
	}
	path, ok := agentConfigPath(env, agent)
	if !ok {
		return "", nil, agentChangeJSON{
			Path:   "",
			Action: actionManual,
			Reason: agent + "'s MCP config is user-scoped and no home directory could be resolved — set HOME and re-run",
		}, mcpAuthCoverage{}, nil
	}
	// 🔴 THE DESTINATION IS CLASSIFIED DURING PLANNING, NOT AT WRITE TIME. A
	// broken symlink is refused by name (see resolveWriteTarget), and that refusal
	// used to live only in the write path — so `--dry-run` reported `create`,
	// `ok: true`, exit 0 for a destination the real run refuses. It is a pure
	// read, so it belongs where every other plan-time refusal is.
	if err := checkWriteTargetResolvable(path); err != nil {
		return path, nil, agentChangeJSON{}, mcpAuthCoverage{}, err
	}
	data, err := renderMCPConfig(path, t, token)
	if err != nil {
		return path, nil, agentChangeJSON{}, mcpAuthCoverage{}, err
	}
	raw, existed, err := readIfExists(path)
	if err != nil {
		return path, nil, agentChangeJSON{}, mcpAuthCoverage{}, err
	}
	// The coverage is read back out of the bytes this run will write, so every
	// sentence about what the registration carries describes the same artefact
	// the agent will load — and `--dry-run` describes it identically, because it
	// renders through this same call and only skips the write.
	cov := mcpAuthCoverageOf(data, t)
	action := string(actionCreate)
	reason := "registers both Civitai MCP servers"
	if existed {
		action = actionMerge
		reason = "merges both Civitai MCP servers in, preserving every other server and key"
	}
	// 🔴 THE ONE THING THE MERGE DOES NOT PRESERVE IS NAMED WHERE IT HAPPENS. A
	// JSONC config is decoded and re-encoded, so its comments, its trailing commas
	// and its key order are gone from the file the user opens next. The
	// alternative that shipped — refusing every commented file — made `--agent
	// zed` unusable on a stock Zed install, and the narrower version of it
	// (accepting comments but still refusing a trailing comma) refused a file both
	// Zed and VS Code read happily. Losing formatting while saying so beats
	// refusing while blaming the user, but only if it is actually said — and the
	// sentence has to name every form that is lost, not just the first one found.
	if jsonMergeDropsFormatting(raw, t) {
		reason += "; comments, trailing commas and key order in that file are NOT preserved by this merge " +
			"(it is decoded and re-encoded)"
	}
	reason += "; " + mcpAuthReason(t, token != "", cov)
	if t.Caveat != "" {
		reason += "; note: " + t.Caveat
	}
	return path, data, agentChangeJSON{Path: path, Action: action, Reason: reason}, cov, nil
}

// mcpAuthReason states, in one clause, what this run did about authentication —
// and it is the same sentence in `--dry-run`, in `--json` and on the terminal.
//
// 🔴 IT NEVER SAYS "THE TOKEN WAS WRITTEN", BECAUSE IT NEVER IS. The three
// outcomes are: a reference to CIVITAI_TOKEN in the agent's own documented
// spelling, Codex's variable-NAME key, or no header at all — and the last one is
// reported for exactly what it reaches (mcpAnonymityNote), not as "anonymous
// read tools work", which was true of one of the two servers.
//
// 🔴 AND THE NO-HEADER CLAUSE IS ABOUT THE FILE, NOT ABOUT THIS RUN. It used to
// read the gate only, so a re-run without a token described a registration whose
// preserved `Authorization` it had just written back as one that 401s. See
// mcpAuthCoverage.
func mcpAuthReason(t agentTarget, hasToken bool, cov mcpAuthCoverage) string {
	if !hasToken {
		if preserved := mcpPreservedAuthNote(cov); preserved != "" {
			return "this run wrote no Authorization header — no token is configured — but " + preserved
		}
		return "no Authorization header — no token is configured (" + mcpAnonymityNote() + ")"
	}
	if t.EnvBearerKey != "" {
		return "auth reads " + tokenEnvVar + " via `" + t.EnvBearerKey + "` — no credential is written to disk"
	}
	if t.EnvHeaderSyntax != "" {
		return "the Authorization header references " + tokenEnvVar + " as `" + t.EnvHeaderSyntax +
			"` — no credential is written to disk; export " + tokenEnvVar + " for the agent to resolve it"
	}
	if preserved := mcpPreservedAuthNote(cov); preserved != "" {
		return "no Authorization header was written — " + t.Agent + " documents no way to read one from the " +
			"environment, and this command never writes a credential to a config file — but " + preserved
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
func printAgentSetupWrite(w io.Writer, env agentEnv, agent, token string, changes []agentChangeJSON, cov mcpAuthCoverage, dryRun bool) {
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
			//
			// 🔴 AND THE ROW IS NEVER SUPPRESSED BY A VALUE THIS COMMAND CANNOT
			// EVALUATE. Round 2 dropped it whenever the entry carried ANY non-empty
			// Authorization, which silenced the 401 warning for a Zed config
			// holding the literal `Bearer <your token>` — the placeholder this
			// command's own next-step block tells the user to paste. Presence is
			// not resolution: each kind gets its own honest row, and only this
			// command's OWN reference, with the variable actually exported, gets
			// none. See mcpAuthKind.
			if !srv.Anonymous {
				pad := strings.Repeat(" ", 23)
				switch cov.kindOf(srv.Name) {
				case authManaged:
					if !tokenIsExported(env) {
						fmt.Fprintf(w, "  %s%s\n", pad, st.Warn(cov.artefactOf(srv.Name)+" is present and "+
							"references "+tokenEnvVar+", which is NOT set here — this one returns 401 until "+
							"you export it"))
					}
				case authOpaque:
					fmt.Fprintf(w, "  %s%s\n", pad, st.Warn("carries "+cov.artefactOf(srv.Name)+" this "+
						"command did not write — this one returns 401 unless that value resolves to a credential"))
				default:
					fmt.Fprintf(w, "  %s%s\n", pad, st.Warn("needs an Authorization header — "+
						"this one returns 401 without a credential"))
				}
			}
		}
	}

	printAgentSetupAuthNote(w, st, agent, target, known, token != "", cov)

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
func printAgentSetupAuthNote(w io.Writer, st ui.Styler, agent string, t agentTarget, known, hasToken bool, cov mcpAuthCoverage) {
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
		// 🔴 "THIS RUN WROTE NONE" AND "THE FILE HAS NONE" ARE DIFFERENT CLAIMS,
		// AND ONLY THE FIRST FOLLOWS FROM hasToken. This branch used to print "No
		// token is configured, so no Authorization header was written" followed by
		// the anonymity note — a statement about what the registration REACHES —
		// for a merge that had just preserved the user's header verbatim. Measured
		// by running with a token and re-running in a shell without one, which is
		// what CI, a second machine and an expired login all look like.
		if preserved := mcpPreservedAuthNote(cov); preserved != "" {
			fmt.Fprintf(w, "  No token is configured, so this run wrote no Authorization header.\n")
			// Printed VERBATIM for the same reason the anonymity note below is: it
			// opens with a SERVER NAME the user has to type, and a sentence-casing
			// pass would silently rename `civitai` to `Civitai`.
			fmt.Fprintf(w, "  %s\n", st.Warn("What is in that file: "+preserved+"."))
			fmt.Fprintf(w, "  Run %s if you want this command to manage that header instead.\n",
				st.Code("civitai login"))
			break
		}
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
		// 🔴 SAME SPLIT AS THE no-token BRANCH ABOVE. This is the arm that TELLS a
		// Zed user to add the header by hand, so it is the arm most likely to be
		// describing a file that already has one — and describing that file as
		// unreachable is how the next run reads as having undone their work.
		if preserved := mcpPreservedAuthNote(cov); preserved != "" {
			fmt.Fprintf(w, "  was written by this run. %s\n", st.Warn("What is in that file: "+preserved+"."))
		} else {
			fmt.Fprintf(w, "  was written. %s\n", st.Warn("Access without one: "+mcpAnonymityNote()+"."))
		}
		fmt.Fprintf(w, "  To authenticate, add this yourself to each Civitai entry in that file:\n")
		fmt.Fprintf(w, "       %s\n", st.Code(`"`+t.HeadersKey+`": {"Authorization": "Bearer <your token>"}`))
		fmt.Fprintf(w, "  %s\n", st.Dim("That file is yours; note that a literal token in it is a credential on disk, "+
			"so keep it out of version control."))
	}
}
