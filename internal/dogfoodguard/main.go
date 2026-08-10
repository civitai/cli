// Command dogfoodguard classifies a `civitai` invocation for the dogfood
// sandbox (scripts/dogfood-sandbox.sh) by asking the REAL command tree what the
// argv means, and prints a machine-readable verdict.
//
// 🔴 WHY THIS EXISTS. The sandbox shim used to maintain its own model of how
// the CLI parses argv, and that model disagreed with pflag three times, each
// time letting a real spend or a real mutation past the gate:
//
//   - `--no-color generate … --yes` — Cobra strips the root persistent bools
//     before resolving the command, so a positional `$1` classifier saw
//     `--no-color` as the command and skipped every branch.
//   - `--negative-prompt --help` — pflag takes the token after a value-taking
//     flag as its VALUE even when it starts with `--`, so an "is --help
//     present?" scan reported a help request for a real submit.
//   - `--print-input=false` — a boolean set to FALSE is still "present", so a
//     presence test read a spending run as a free preview.
//
// Patching spellings fixed instances, not the class. This program removes the
// second parser: it builds the actual tree with cmd.NewRootCmd(), resolves the
// command with Cobra's own Find (which handles root persistent flags, `--`,
// aliases and abbreviation exactly as the binary does, because it IS what the
// binary does), parses with ParseFlags, and reads values through
// Flags().GetBool / GetInt / GetString. `--print-input=false` therefore reads
// as false, which is the entire point.
//
// 🔴 IT NEVER EXECUTES ANYTHING. Find + ParseFlags only — no Execute, no RunE,
// no PersistentPreRun. It performs no I/O beyond writing its verdict, and it
// holds no credential.
//
// This is a DEV/TEST tool. It is deliberately a separate main package rather
// than a subcommand, so it cannot appear in `civitai --help`, and it lives
// under internal/ rather than cmd/ so release tooling globbing ./cmd/* can
// never ship it. .goreleaser.yaml builds exactly one binary
// (main: ./cmd/civitai) and needs no change.
//
// Output is TAB-separated key/value lines on stdout, values escaped so a
// prompt containing a newline or a tab cannot forge a record:
//
//	ok       1
//	path     app listing set-icon
//	bool.dry-run     false
//	int.max-cost     5
//	changed.max-cost true
//	str.slug         my-app
//	argc     1
//	arg.0    ./icon.png
//
// On any failure it prints `ok\t0` plus `err\t<message>` and exits non-zero.
// The shim treats a non-zero exit, empty output, or an unrecognised field as
// DENY.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clicmd "github.com/civitai/cli/internal/cmd"
)

// Booleans the sandbox policy branches on.
var reportedBools = []string{
	"dry-run", "print-input", "package-only", "help", "no-wait", "yes",
	"json", "force", "skip-validate", "strict",
}

// Ints the sandbox policy branches on.
var reportedInts = []string{"max-cost", "quantity"}

// Strings the sandbox policy branches on (identity of the thing being mutated).
var reportedStrings = []string{"slug", "dir", "id", "out", "external-id", "input", "app"}

// Flags reported as their raw string value, whatever their type.
var reportedRaw = []string{"timeout"}

// escape keeps one record on one line and one field in one column.
func escape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t")
	return r.Replace(s)
}

func emit(w *os.File, k, v string) {
	fmt.Fprintf(w, "%s\t%s\n", k, escape(v))
}

func fail(msg string) {
	emit(os.Stdout, "ok", "0")
	emit(os.Stdout, "err", msg)
	os.Exit(1)
}

// blockIDOf answers "which app would the CLI act on?" using encoding/json —
// the same decoder the CLI uses.
//
// 🔴 THE SHELL VERSION GREPPED THE FIRST `"blockId"`, AND encoding/json TAKES
// THE LAST. Measured on a manifest carrying the key twice: the gate resolved
// `sanctioned-ok` and allowed the submit, while the CLI would have submitted
// `someones-real-app`. That is the same gate-vs-binary disagreement class the
// classifier exists to remove, so the answer comes from the same library the
// binary uses rather than from a regex.
func blockIDOf(dir string) {
	var m struct {
		BlockID string `json:"blockId"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		emit(os.Stdout, "ok", "0")
		emit(os.Stdout, "err", err.Error())
		os.Exit(1)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		emit(os.Stdout, "ok", "0")
		emit(os.Stdout, "err", "manifest is not valid JSON: "+err.Error())
		os.Exit(1)
	}
	if m.BlockID == "" {
		emit(os.Stdout, "ok", "0")
		emit(os.Stdout, "err", "manifest declares no blockId")
		os.Exit(1)
	}
	emit(os.Stdout, "ok", "1")
	emit(os.Stdout, "blockid", m.BlockID)
}

func main() {
	args := os.Args[1:]

	// `blockid <dir>` resolves the app a project directory would submit.
	if len(args) >= 2 && args[0] == "blockid" {
		blockIDOf(args[1])
		return
	}

	// A leading `--` separates the guard's own (absent) flags from the argv
	// under test, so an invocation starting with a flag is unambiguous.
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	root := clicmd.NewRootCmd()
	// Silence anything Cobra might want to print; we only ever inspect.
	root.SetOut(os.Stderr)
	root.SetErr(os.Stderr)

	// 🔴 `help` IS ADDED BY Execute(), WHICH WE NEVER CALL. Without this,
	// `root.Find` never sees a `help` command, so `civitai help generate` — the
	// first thing a blind agent reaches for — resolved to the root with a
	// leftover positional and was REFUSED. Fail-closed, but a pointless
	// distortion. InitDefaultHelpCmd registers it exactly as Execute would.
	root.InitDefaultHelpCmd()

	target, remaining, err := root.Find(args)
	if err != nil {
		// An unknown command is a real answer, not an internal failure: report
		// it so the shim can refuse by name.
		emit(os.Stdout, "ok", "0")
		emit(os.Stdout, "err", err.Error())
		emit(os.Stdout, "kind", "unknown-command")
		os.Exit(2)
	}
	if target == nil {
		fail("cobra resolved no command")
	}

	// Cobra adds these during Execute, which we never call.
	target.InitDefaultHelpFlag()
	target.InitDefaultVersionFlag()

	parseErr := ""
	if err := target.ParseFlags(remaining); err != nil {
		parseErr = err.Error()
	}

	out := os.Stdout
	if parseErr != "" {
		// A flag the CLI itself would reject. The shim denies rather than
		// guessing what the user meant — this is how `--max-cost 08` and
		// `--dry-run=maybe` become refusals instead of shell arithmetic
		// crashes or silent misreads.
		emit(out, "ok", "0")
		emit(out, "err", parseErr)
		emit(out, "kind", "flag-parse-error")
		emit(out, "path", commandPath(root, target))
		os.Exit(3)
	}

	emit(out, "ok", "1")
	emit(out, "path", commandPath(root, target))

	// 🔴 ASK COBRA WHETHER THE CLI WOULD ACCEPT THESE ARGUMENTS. The shell gate
	// carried a hand-written list of "group" commands to spot an unknown
	// subcommand — the kind of table this classifier exists to delete — and it
	// was wrong: an unknown subcommand under `models`, `images`, `users` or
	// `tags` was ALLOWED because those parents were not in the list. Measured,
	// the binary exits 2 for all of them.
	//
	// `Runnable()` is NOT the discriminator: every one of those parents is
	// runnable (they print their own help). The two that are:
	//   * HasSubCommands + a leftover positional  => that positional is a
	//     subcommand that does not exist;
	//   * ValidateArgs   => the command's own Args validator, i.e. exactly what
	//     the binary would enforce.
	emit(out, "runnable", boolStr(target.Runnable()))
	emit(out, "hassubcommands", boolStr(target.HasSubCommands()))

	flags := target.Flags()

	// Raw values for flags whose type the shell does not need to understand —
	// it only needs to know they were set. `--timeout` is here because it is an
	// unblocked `--no-wait`: it stops the CLI waiting, not the charge.
	for _, name := range reportedRaw {
		f := flags.Lookup(name)
		if f == nil {
			continue
		}
		emit(out, "raw."+name, f.Value.String())
		emit(out, "changed."+name, boolStr(flags.Changed(name)))
	}
	for _, name := range reportedBools {
		if flags.Lookup(name) == nil {
			continue
		}
		v, err := flags.GetBool(name)
		if err != nil {
			// Declared but not a bool on this command: report it as unknown
			// rather than assuming either way.
			emit(out, "bool."+name, "unreadable")
			continue
		}
		emit(out, "bool."+name, boolStr(v))
		emit(out, "changed."+name, boolStr(flags.Changed(name)))
	}
	for _, name := range reportedInts {
		if flags.Lookup(name) == nil {
			continue
		}
		v, err := flags.GetInt(name)
		if err != nil {
			emit(out, "int."+name, "unreadable")
			continue
		}
		emit(out, "int."+name, fmt.Sprintf("%d", v))
		emit(out, "changed."+name, boolStr(flags.Changed(name)))
	}
	for _, name := range reportedStrings {
		if flags.Lookup(name) == nil {
			continue
		}
		v, err := flags.GetString(name)
		if err != nil {
			emit(out, "str."+name, "unreadable")
			continue
		}
		emit(out, "str."+name, v)
		emit(out, "changed."+name, boolStr(flags.Changed(name)))
	}

	positional := flags.Args()
	emit(out, "argc", fmt.Sprintf("%d", len(positional)))
	for i, a := range positional {
		emit(out, fmt.Sprintf("arg.%d", i), a)
	}
	if err := target.ValidateArgs(positional); err != nil {
		emit(out, "argserr", err.Error())
	} else {
		emit(out, "argserr", "")
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// commandPath returns the resolved command path WITHOUT the root's name, so
// the shim compares against "app listing set-icon" rather than
// "civitai app listing set-icon" (the binary is renamed in the sandbox).
func commandPath(root, target *cobra.Command) string {
	full := target.CommandPath()
	rootName := root.Name()
	if full == rootName {
		return ""
	}
	return strings.TrimPrefix(full, rootName+" ")
}
