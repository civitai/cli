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
// instruction for the human to authenticate themselves. Both MCP servers' read
// tools work anonymously, so a token-less registration is genuinely useful, and
// an unauthenticated setup is a SUCCESS rather than a half-finished one. That is
// why `authenticated` is reported by `--check` and deliberately excluded from its
// verdict; see agentSetupVerdict.
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
// do; a write run that fails returns an error and emits no payload at all, so
// `ok` is true whenever there is one to read. A write run for `--agent other`
// leaves a MANUAL step, and that is reported as a `changes` row with the action
// `manual` rather than by moving `ok` — an `ok: false` beside exit 0 is a
// contradiction a script cannot act on.
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
func agentSetupVerdict(checks []agentCheckJSON) bool {
	ok := true
	for _, c := range checks {
		if c.Name == checkAuthenticated {
			continue
		}
		if !c.OK {
			ok = false
		}
	}
	return ok
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
an existing MCP config is MERGED into, preserving every other server and key. A
config file that does not parse is refused by name rather than repaired.

The agent is detected from the environment and then from marker files in the
project; --agent overrides it. --agent other prints the config for you to paste
in yourself and writes nothing.

NO CREDENTIAL IS EVER WRITTEN INTO A CONFIG FILE. Most of these files are
project-scoped — .mcp.json, .cursor/mcp.json, .vscode/mcp.json and opencode.json
sit in the repo root and get committed — so an Authorization header holding your
actual token is a secret headed for version control. Instead, for agents whose
vendors document environment-variable interpolation, the header REFERENCES
CIVITAI_TOKEN in that vendor's own spelling (${CIVITAI_TOKEN} for Claude Code,
${env:CIVITAI_TOKEN} for Cursor and Windsurf, {env:CIVITAI_TOKEN} for opencode,
and bearer_token_env_var for Codex); export CIVITAI_TOKEN so the agent resolves
it. For agents that document none — VS Code and Zed today — no header is written
at all and the output names the exact header to add yourself. Both servers' read
tools work anonymously, so a header-less config browses models, images and
articles as it stands.

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

			env := liveAgentEnv(dir)
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
		OK:     agentSetupVerdict(checks),
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
		"what it can write", ErrAgentSetupIncomplete, countFailedChecks(checks))
}

func countFailedChecks(checks []agentCheckJSON) int {
	n := 0
	for _, c := range checks {
		if c.Name == checkAuthenticated {
			continue
		}
		if !c.OK {
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
	switch {
	case !found:
		checks = append(checks, agentCheckJSON{Name: checkAgentsMD, OK: false,
			Detail: "missing at " + agentsPath + " — run `civitai agent-setup`"})
	case !agentsMDHasManagedBlock(string(raw)):
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
	claudePath := filepath.Join(env.Dir, claudeFilename)
	claudeRaw, claudeFound, err := readIfExists(claudePath)
	if err != nil {
		return nil, err
	}
	switch {
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
		if c.Name == checkAuthenticated {
			// 🔴 A WARNING, NOT A FAILURE — the glyph has to agree with the
			// verdict. Rendering this row like the others tells a reader the run
			// failed on the one row that deliberately cannot fail it.
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

	mcpPath, mcpData, mcpChange, err := planMCPConfig(env, agent, token)
	if err != nil {
		return err
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
		return writeJSON(out, agentSetupJSON{
			Track: track, Agent: agent, OK: true, Changes: changes, DryRun: dryRun,
		})
	}
	printAgentSetupWrite(out, env, agent, token, changes, dryRun)
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
			Action: "manual",
			Reason: "agent " + agent + " has no MCP config file this CLI knows — the servers are printed below for you to paste in",
		}, nil
	}
	path, ok := agentConfigPath(env, agent)
	if !ok {
		return "", nil, agentChangeJSON{
			Path:   "",
			Action: "manual",
			Reason: agent + "'s MCP config is user-scoped and no home directory could be resolved — set HOME and re-run",
		}, nil
	}
	data, err := renderMCPConfig(path, t, token)
	if err != nil {
		return path, nil, agentChangeJSON{}, err
	}
	_, existed, err := readIfExists(path)
	if err != nil {
		return path, nil, agentChangeJSON{}, err
	}
	action := string(actionCreate)
	reason := "registers both Civitai MCP servers"
	if existed {
		action = "merge"
		reason = "merges both Civitai MCP servers in, preserving every other server and key"
	}
	reason += "; " + mcpAuthReason(t, token != "")
	return path, data, agentChangeJSON{Path: path, Action: action, Reason: reason}, nil
}

// mcpAuthReason states, in one clause, what this run did about authentication —
// and it is the same sentence in `--dry-run`, in `--json` and on the terminal.
//
// 🔴 IT NEVER SAYS "THE TOKEN WAS WRITTEN", BECAUSE IT NEVER IS. The three
// outcomes are: a reference to CIVITAI_TOKEN in the agent's own documented
// spelling, Codex's variable-NAME key, or no header at all — and the last one is
// reported as the working anonymous setup it is, not as something missing.
func mcpAuthReason(t agentTarget, hasToken bool) string {
	if !hasToken {
		return "no Authorization header — no token is configured (read tools still work anonymously)"
	}
	if t.EnvBearerKey != "" {
		return "auth reads " + tokenEnvVar + " via `" + t.EnvBearerKey + "` — no credential is written to disk"
	}
	if t.EnvHeaderSyntax != "" {
		return "the Authorization header references " + tokenEnvVar + " as `" + t.EnvHeaderSyntax +
			"` — no credential is written to disk; export " + tokenEnvVar + " for the agent to resolve it"
	}
	return "no Authorization header — " + t.Agent + " documents no way to read one from the environment, " +
		"and this command never writes a credential to a config file (read tools still work anonymously)"
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
		case c.Action == "manual":
			fmt.Fprintln(w, "  "+st.Warn("manual   "+c.Reason))
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
		}
	}

	printAgentSetupAuthNote(w, st, agent, target, known, token != "")

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
	if token != "" && known && (target.EnvHeaderSyntax != "" || target.EnvBearerKey != "") {
		n++
		fmt.Fprintf(w, "  %d. Export the token where %s can see it — the config above references it by name:\n", n, agent)
		fmt.Fprintf(w, "       %s\n", st.Code("export "+tokenEnvVar+"=<your token>"))
		fmt.Fprintf(w, "     %s\n", st.Dim("Put it in your shell profile so the agent inherits it. "+
			"Browsing and other read tools work without it."))
	}
	// 🔴 THE LAST LINE NAMES `civitai login`, AND THIS COMMAND NEVER RUNS IT.
	// Authentication is the human's step: nothing here opens a browser, prompts
	// for a credential, or stores one.
	n++
	if token == "" {
		fmt.Fprintf(w, "  %d. Run %s to authenticate, then re-run %s so the MCP entries pick up the reference.\n",
			n, st.Code("civitai login"), st.Code("civitai agent-setup"))
	} else {
		fmt.Fprintf(w, "  %d. Already authenticated — run %s again only if you need different scopes.\n",
			n, st.Code("civitai login"))
	}
}

// printAgentSetupAuthNote says what was — and was not — written about auth.
//
// 🔴 A HEADER-LESS CONFIG IS PRESENTED AS WORKING, NOT AS DEGRADED. Both
// servers' read tools are anonymous, so "no Authorization header" is a setup
// that browses models, images and articles today; wording it as a shortfall
// invites the reader to paste their token in by hand, which is precisely the
// leak this command was fixed to stop causing.
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
			"} (Cursor, Windsurf), {env:"+tokenEnvVar+"} (opencode)."))
	case !hasToken:
		fmt.Fprintf(w, "  No token is configured, so no Authorization header was written. The read tools work\n")
		fmt.Fprintf(w, "  anonymously, so this setup is usable as it stands.\n")
	case t.EnvBearerKey != "":
		fmt.Fprintf(w, "  Each entry carries %s, so %s reads the token from your environment\n",
			st.Code(t.EnvBearerKey+" = \""+tokenEnvVar+"\""), agent)
		fmt.Fprintf(w, "  at request time rather than from the file.\n")
	case t.EnvHeaderSyntax != "":
		fmt.Fprintf(w, "  Each entry carries %s, which %s resolves from your\n",
			st.Code(`"Authorization": "Bearer `+t.EnvHeaderSyntax+`"`), agent)
		fmt.Fprintf(w, "  environment at request time rather than from the file.\n")
	default:
		// 🔴 NAMED, NOT SILENT. VS Code and Zed document no way to read a header
		// from the environment, so there is nothing this command can write that
		// is both authenticated and credential-free. Saying so — with the exact
		// file and the exact header — is the whole remedy; a silent omission
		// would read as a bug in this command.
		fmt.Fprintf(w, "  %s documents no way to read a header from the environment, so no Authorization header\n", agent)
		fmt.Fprintf(w, "  was written. Read tools still work anonymously. To authenticate, add this yourself to\n")
		fmt.Fprintf(w, "  each Civitai entry in that file:\n")
		fmt.Fprintf(w, "       %s\n", st.Code(`"`+t.HeadersKey+`": {"Authorization": "Bearer <your token>"}`))
		fmt.Fprintf(w, "  %s\n", st.Dim("That file is yours; note that a literal token in it is a credential on disk, "+
			"so keep it out of version control."))
	}
}
