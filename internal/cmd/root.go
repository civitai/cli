// Package cmd wires the cobra command tree for the civitai CLI.
package cmd

import (
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
		Short: "Civitai CLI — author and ship Apps",
		Long: `civitai is the command-line interface for Civitai (https://civitai.com).

Its first feature group is Apps authoring — Apps are small,
sandboxed web apps that run inside Civitai surfaces. The CLI scaffolds a
correct project, validates it against the platform contract, and packages it
for submission, so you don't have to hand-format a ZIP.

Get started:

  civitai login                    store your API token
  civitai app create my-app        scaffold a ready-to-build App
  civitai app validate             check the manifest before you submit
  civitai app submit               package + submit for review`,
		Example: `  # First time: authenticate, then scaffold and submit an app.
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

	root.AddCommand(newAppCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newWhoAmICmd())
	root.AddCommand(newBuzzCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newUpgradeCmd())
	root.AddCommand(newCompletionCmd())
	root.AddCommand(newUpdateCheckCmd())

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

	return root
}
