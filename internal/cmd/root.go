// Package cmd wires the cobra command tree for the civitai CLI.
package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Build metadata, overridden at build time via -ldflags (see cmd/civitai/main.go
// and .goreleaser.yaml). They default to dev values for source builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// readBuildInfo is a seam so tests can stub runtime/debug.ReadBuildInfo.
var readBuildInfo = debug.ReadBuildInfo

// SetBuildInfo lets main inject the build version, commit, and date. Goreleaser
// release binaries pass real ldflag values, which are authoritative. For a plain
// `go install github.com/civitai/cli/cmd/civitai@latest` (or any source build),
// the ldflags are absent and the injected values are still the "dev" defaults —
// in that case we fall back to runtime/debug.ReadBuildInfo so `civitai version`
// reports the module version (e.g. v0.1.1 or a pseudo-version) and the embedded
// VCS revision/time instead of dev/none/unknown.
func SetBuildInfo(v, c, d string) {
	if v != "" {
		version = v
	}
	if c != "" {
		commit = c
	}
	if d != "" {
		date = d
	}
	applyBuildInfoFallback()
}

// applyBuildInfoFallback fills version/commit/date from the embedded Go build
// info when (and only when) the corresponding value is still its dev default —
// i.e. no goreleaser ldflag overrode it. Each field falls back independently.
func applyBuildInfoFallback() {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return
	}
	var mainVersion, rev, vcsTime, modified string
	mainVersion = info.Main.Version
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			vcsTime = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	version, commit, date = mergeBuildInfo(version, commit, date, mainVersion, rev, vcsTime, modified)
}

// mergeBuildInfo merges the (possibly ldflag-stamped) version/commit/date with
// the values read from runtime/debug.ReadBuildInfo. The ldflag values are
// authoritative: a field is only filled from build info when it's still its
// dev/none/unknown default. Pure function so the merge logic is unit-testable
// without mocking ReadBuildInfo.
//
//   - version: if dev/empty, use mainVersion (the module version from
//     `go install module@vX`), unless it's "" or "(devel)".
//   - commit:  if "none", use vcs.revision shortened to 12 chars, with "+dirty"
//     appended when vcs.modified == "true".
//   - date:    if "unknown", use vcs.time.
//
// Precedence per field: ldflags > vcs.* > Go pseudo-version. The vcs.* settings
// are absent for `go install module@version` builds (the module proxy cache has
// no VCS checkout), so as a last resort we recover commit/date from a Go
// pseudo-version mainVersion (the `vX.Y.Z-<timestamp>-<sha>` form used for
// untagged/branch installs, which embeds both). A clean release tag (e.g.
// `v0.1.12`) carries no commit/date, so those stay at their defaults here.
func mergeBuildInfo(version, commit, date, mainVersion, rev, vcsTime, modified string) (string, string, string) {
	if isDevDefault(version) {
		if mainVersion != "" && mainVersion != "(devel)" {
			version = mainVersion
		}
	}
	if commit == "none" && rev != "" {
		commit = shortenRevision(rev)
		if modified == "true" {
			commit += "+dirty"
		}
	}
	if date == "unknown" && vcsTime != "" {
		date = vcsTime
	}

	// Last-resort recovery from a Go pseudo-version (no vcs.* present): fills
	// only the fields still at their defaults; vcs.*/ldflags above already won.
	if commit == "none" || date == "unknown" {
		if psha, pdate, ok := parsePseudoVersion(mainVersion); ok {
			if commit == "none" && psha != "" {
				commit = shortenRevision(psha)
			}
			if date == "unknown" && pdate != "" {
				date = pdate
			}
		}
	}
	return version, commit, date
}

// pseudoVersionRE matches the three Go pseudo-version forms and captures the
// 14-digit `yyyymmddhhmmss` timestamp and the trailing 12-hex commit prefix:
//
//	vX.Y.Z-yyyymmddhhmmss-sha            (no known base tag below the commit)
//	vX.Y.Z-0.yyyymmddhhmmss-sha          (base release tag)
//	vX.Y.Z-pre.0.yyyymmddhhmmss-sha      (base pre-release tag, e.g. -pre / -beta.1)
//
// See https://go.dev/ref/mod#pseudo-versions. The timestamp is preceded by a
// `-` (form 1, no base tag) or a `.` (forms 2/3, where it's the final dotted
// segment of a `0` / `pre.0` pre-release), so we anchor on the unambiguous
// `[.-]<14 digits>-<12 hex>$` tail. The optional pre-release run before it is
// non-greedy so it can't swallow the timestamp.
var pseudoVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+?)?[.-](\d{14})-([0-9a-f]{12})$`)

// parsePseudoVersion extracts the commit SHA and build date embedded in a Go
// pseudo-version string. It returns (sha, rfc3339Date, true) on a match, or
// ("", "", false) for any non-pseudo or malformed value (a clean release tag,
// "dev", "", a git-describe string, etc.). Pure + offline.
//
// The date is the pseudo-version's `yyyymmddhhmmss` timestamp (always UTC),
// formatted as RFC3339 (e.g. "2026-06-25T19:47:26Z") to match the shape of the
// vcs.time value the rest of the command prints.
func parsePseudoVersion(v string) (sha, date string, ok bool) {
	m := pseudoVersionRE.FindStringSubmatch(v)
	if m == nil {
		return "", "", false
	}
	ts, err := time.Parse("20060102150405", m[1])
	if err != nil {
		return "", "", false
	}
	return m[2], ts.UTC().Format(time.RFC3339), true
}

// shortenRevision trims a full 40-char git SHA to 12 chars; shorter values are
// returned unchanged.
func shortenRevision(rev string) string {
	const n = 12
	if len(rev) > n {
		return rev[:n]
	}
	return rev
}

// isDevDefault reports whether v is one of the placeholder values that mean
// "no real version was stamped in" (so a build-info fallback should apply).
func isDevDefault(v string) bool {
	return v == "" || v == "dev"
}

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	var noUpdateCheck bool
	var noColor, forceColor bool
	// A dedicated viper instance (not the global) binds the color flags + their
	// CIVITAI_* env fallbacks, scoped to this root so repeated NewRootCmd calls
	// (tests) don't cross-wire flag pointers.
	colorViper := viper.New()
	root := &cobra.Command{
		Use:   "civitai",
		Short: "Civitai CLI — browse & download models, and build Apps",
		Long: `civitai is the command-line interface for Civitai (https://civitai.com).

It does three things from one static binary:

  • Browse & download the public catalog — search models, list images and
    articles, and fetch model files. Reading is anonymous; downloading a
    model file needs a token (civitai login). --layout routes each file into
    the right folder for ComfyUI / A1111.
  • Build & submit Apps — small, sandboxed web apps that run inside Civitai
    surfaces. Scaffold a correct project, validate it against the platform
    contract, and package it for submission.
  • Generate images — civitai generate "<prompt>". This SPENDS REAL BUZZ and
    needs the AI Services scopes (civitai login --scopes generate, or a
    full-scope personal API key); price a job with --dry-run first.

Get started:

  # Browse & download
  civitai models search --query "pony" --limit 5
  civitai images search --sort "Most Reactions" --period Week
  civitai download 128713 --layout comfyui --root ~/ComfyUI

  # Build an App
  civitai login                    store your API token
  civitai app create my-app        scaffold a ready-to-build App
  civitai app submit               package + submit for review

Exit codes:

  Every command returns a differentiated exit code so scripts can branch on the
  KIND of failure without parsing stderr (the error message itself is unchanged):

    0  success
    1  generic / unclassified error
    2  usage error — a bad flag, a bad flag value, or a request the API rejected
       as malformed (HTTP 400)
    3  authentication/authorization — login required, token invalid/expired, or
       the credential lacks the needed scope (HTTP 401/403)
    4  not found — the requested resource does not exist (HTTP 404)
    5  network/transport failure or service unavailable (dial/timeout, HTTP
       502/503/504 after retries)
    6  rate limited — throttled by the API (HTTP 429)`,
		Example: `  # Browse & download the public catalog (no account needed to read).
  civitai models search --query "dreamshaper" --limit 5
  civitai images search --sort "Most Reactions" --period Week
  civitai download 128713 --layout comfyui --root ~/ComfyUI

  # Browse the App store.
  civitai app list
  civitai app view <slug>

  # Build & ship an App.
  civitai login
  civitai app create my-first-app
  cd my-first-app
  civitai app validate
  civitai app submit`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		// PersistentPreRunE runs before ANY subcommand's RunE. It configures the
		// shared ui presentation layer ONCE — resolving color enablement from the
		// --no-color/--color flags (bound via viper, so CIVITAI_NO_COLOR /
		// CIVITAI_COLOR env work too), the standard NO_COLOR/CLICOLOR_FORCE env,
		// and whether stdout is a real TTY. Piping stdout (or NO_COLOR) fully
		// disables ANSI. cobra runs the nearest PersistentPreRunE, and no
		// subcommand defines its own, so this always runs.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ui.Configure(ui.Options{
				NoColor:    colorViper.GetBool("no_color"),
				ForceColor: colorViper.GetBool("color"),
				// Auto-detect against stdout — where the user-facing status output
				// (login/whoami/submit success lines) goes; piping it disables color.
				Writer: cmd.OutOrStdout(),
			})
			return nil
		},
		// PersistentPostRun fires after ANY subcommand's RunE. It prints at most
		// one cached "new version available" line to stderr and (when the cache
		// is stale) kicks off a detached background refresh. It is best-effort
		// and NEVER blocks the command — see maybeNotifyUpdate.
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			maybeNotifyUpdate(cmd.ErrOrStderr(), cmd.Name(), noUpdateCheck)
		},
	}
	root.SetVersionTemplate("civitai {{.Version}}\n")

	// A persistent flag so every command honours --no-update-check, and the
	// post-run hook can read its resolved value.
	root.PersistentFlags().BoolVar(&noUpdateCheck, "no-update-check", false,
		"skip the background check for a newer release (also via CIVITAI_NO_UPDATE_CHECK)")

	// Global color controls (honored by the shared ui package). --no-color wins
	// over --color; the standard NO_COLOR / CLICOLOR_FORCE env are also honored.
	root.PersistentFlags().BoolVar(&noColor, "no-color", false,
		"disable colored/styled output (also via NO_COLOR or CIVITAI_NO_COLOR)")
	root.PersistentFlags().BoolVar(&forceColor, "color", false,
		"force colored output even when stdout is not a TTY (also via CLICOLOR_FORCE)")
	_ = colorViper.BindPFlag("no_color", root.PersistentFlags().Lookup("no-color"))
	_ = colorViper.BindPFlag("color", root.PersistentFlags().Lookup("color"))
	_ = colorViper.BindEnv("no_color", "CIVITAI_NO_COLOR")
	_ = colorViper.BindEnv("color", "CIVITAI_COLOR")

	// Classify Cobra's own flag-parsing failures as usage errors (message left
	// untouched) so the entrypoint can map them to a dedicated exit code. Applies
	// to every subcommand via cobra's flag-error propagation.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return asUsageError(err)
	})

	root.AddCommand(newAppCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newWhoAmICmd())
	root.AddCommand(newBuzzCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newUpgradeCmd())
	root.AddCommand(newCompletionCmd())
	root.AddCommand(newUpdateCheckCmd())

	// 🔴 A money-spending command, and deliberately a LEAF: it must never grow
	// subcommands. enforceUsageExitCodes only installs the unknown-subcommand
	// guard on non-runnable parents, so on a runnable one a typo'd subcommand
	// would be swallowed as the positional PROMPT and billed.
	root.AddCommand(newGenerateCmd())

	// The read side of generation. A GROUP (no Run/RunE of its own) so
	// enforceUsageExitCodes below installs the unknown-subcommand guard on it —
	// the guard `generate` deliberately cannot have.
	root.AddCommand(newWorkflowsCmd())

	// Read subcommands — public REST API (`/api/v1/**`) browsing.
	root.AddCommand(newModelsCmd())
	root.AddCommand(newModelVersionsCmd())
	root.AddCommand(newDownloadCmd())
	root.AddCommand(newImagesCmd())
	root.AddCommand(newTagsCmd())
	root.AddCommand(newCreatorsCmd())
	root.AddCommand(newUsersCmd())
	root.AddCommand(newArticlesCmd())
	root.AddCommand(newCollectionsCmd())

	// Close the cobra-layer exit-code gaps: an unknown subcommand, an unknown
	// top-level command, and a missing/bad-count positional must all classify as
	// USAGE errors (exit 2) rather than leaking success (0) or the generic code
	// (1). Done once here, over the fully-assembled tree.
	enforceUsageExitCodes(root)

	return root
}

// enforceUsageExitCodes walks the assembled command tree and routes cobra's own
// argument- and subcommand-validation failures through the ErrUsage tag, so the
// entrypoint maps them to the usage exit code (2). It closes three leaks that
// the hand-written validators (which already tag their own errors) don't cover:
//
//  1. Cobra's built-in count validators (ExactArgs, MinimumNArgs, …) return
//     plain errors ("accepts 1 arg(s), received 0") that map to the generic exit
//     code (1). We wrap each command's existing Args validator so its failures
//     are tagged ErrUsage too, unifying e.g. `civitai models get` (missing id)
//     and `civitai model-versions by-hash` (missing hash) onto exit 2. Setting a
//     (non-nil) Args on every command also disables cobra's Find-time legacyArgs
//     fallback, so unknown-command handling flows through the single tagged path
//     below rather than an untagged generic error.
//  2. Unknown SUBCOMMAND on a group-only parent. A command that only groups
//     subcommands (has children, defines no Run/RunE of its own) is treated by
//     cobra as "not runnable", and cobra's execute() short-circuits a
//     not-runnable command straight to printing help and returning nil — BEFORE
//     argument validation runs. So `civitai models frobnicate` or the top-level
//     `civitai nonsensecmd` prints help and exits 0, a silent success. We give
//     each such parent a RunE: a bare `civitai <parent>` (no args) still prints
//     help and exits 0, but any leftover argument becomes a usage-tagged
//     "unknown command" error (exit 2). Defining RunE makes the parent runnable,
//     so cobra runs it (and our Args) instead of the help short-circuit.
//  3. REQUIRED FLAGS and flag GROUPS. Cobra validates these inside execute()
//     *after* Args and after the PreRun hooks, so they reach neither
//     SetFlagErrorFunc (which only sees PARSE errors) nor the wrapper in (1) —
//     `required flag(s) "app" not set` exited 1 while an unknown flag NAME
//     exited 2, for the same category of mistake. Running cobra's own
//     validators from inside our Args wrapper is the only interception point
//     ahead of cobra's call; the message is cobra's, unchanged, and when they
//     pass cobra's own later call is a no-op repeat. `--help` is unaffected:
//     execute() returns flag.ErrHelp before it ever reaches Args.
//
// It only reclassifies ARGUMENT/subcommand validation. A command's real RunE
// failures — a 404 not-found, a network error, an auth failure — are never
// touched, so their differentiated exit codes are preserved.
func enforceUsageExitCodes(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		enforceUsageExitCodes(child)
	}

	// (1) Tag the built-in count validators, and make Args non-nil everywhere so
	// cobra's Find no longer synthesizes an untagged legacyArgs "unknown command".
	existing := cmd.Args
	cmd.Args = func(c *cobra.Command, args []string) error {
		if existing != nil {
			if err := existing(c, args); err != nil {
				return asUsageError(err)
			}
		}
		// (3) Required flags / flag groups — see the doc comment. Run cobra's own
		// validators here so their errors can be tagged; cobra repeats them later
		// and finds nothing left to report.
		if err := c.ValidateRequiredFlags(); err != nil {
			return asUsageError(err)
		}
		if err := c.ValidateFlagGroups(); err != nil {
			return asUsageError(err)
		}
		return nil
	}

	// (2) A group-only parent (subcommands, no direct action) must reject an
	// unknown subcommand as a usage error instead of silently printing help.
	// Runnable() reflects the ORIGINAL Run/RunE (the Args set above doesn't
	// affect it), so this correctly targets only the pure command groups.
	if cmd.HasSubCommands() && !cmd.Runnable() {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				// Bare `civitai <parent>` — help, exit 0 (unchanged).
				return c.Help()
			}
			return asUsageError(unknownSubcommandError(c, args[0]))
		}
	}
}

// unknownSubcommandError builds the error for an unrecognized subcommand on a
// group-only parent, matching cobra's own "unknown command" wording (and its
// Levenshtein "Did you mean this?" suggestions, via the exported SuggestionsFor)
// so the message the user sees is unchanged from cobra's native root-level
// handling — only the classification (and thus the exit code) differs.
func unknownSubcommandError(c *cobra.Command, name string) error {
	msg := fmt.Sprintf("unknown command %q for %q", name, c.CommandPath())
	if !c.DisableSuggestions {
		// Mirror cobra's own findSuggestions default: the exported SuggestionsFor
		// reads SuggestionsMinimumDistance verbatim (0 → Levenshtein suggestions
		// off), whereas cobra's native unknown-command path defaults it to 2.
		if c.SuggestionsMinimumDistance <= 0 {
			c.SuggestionsMinimumDistance = 2
		}
		if suggestions := c.SuggestionsFor(name); len(suggestions) > 0 {
			msg += "\n\nDid you mean this?\n"
			for _, s := range suggestions {
				msg += fmt.Sprintf("\t%v\n", s)
			}
		}
	}
	msg += fmt.Sprintf("\nRun '%s --help' for the available subcommands.", c.CommandPath())
	return errors.New(msg)
}
