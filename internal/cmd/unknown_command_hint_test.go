package cmd

import (
	"errors"
	"testing"
)

// An unknown command may be a command NEWER than this binary: docs and agent
// onboarding prompts name commands from the latest release. These tests pin the
// WHOLE error string (not a keyword), so the upgrade hint's wording, its
// position after cobra's suggestions and the --help pointer, and the running
// version it names are all part of the contract a reword must update here.

// The tests pin the version (pinVersion, agent_setup_onboarding_test.go) to a
// value that is neither "dev" (the source-build default) nor the 0.1.125 the
// binary-level test stamps (cmd/civitai/unknown_command_hint_exitcode_test.go),
// so a hint that hardcoded either version fails one of the two.
const hintTail = "is not in this CLI (civitai 9.8.7); if a doc or prompt told you to run it, " +
	"this CLI may be older than that command.\n" +
	"Upgrade with 'civitai upgrade', or get the latest release at " +
	"https://github.com/civitai/cli/releases/latest"

func TestUnknownCommandUpgradeHint(t *testing.T) {
	pinVersion(t, "9.8.7")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			// The measured case: a command an older binary has never heard of.
			name: "top-level unknown, no suggestion",
			args: []string{"feedbackxyz"},
			want: `unknown command "feedbackxyz" for "civitai"` + "\n" +
				"Run 'civitai --help' for the available subcommands.\n" +
				`"civitai feedbackxyz" ` + hintTail,
		},
		{
			name: "unknown subcommand under a group",
			args: []string{"models", "frobnicate"},
			want: `unknown command "frobnicate" for "civitai models"` + "\n" +
				"Run 'civitai models --help' for the available subcommands.\n" +
				`"civitai models frobnicate" ` + hintTail,
		},
		{
			// A near-miss keeps cobra's suggestion FIRST; the hint comes last.
			name: "near-miss keeps the suggestion first",
			args: []string{"models", "serch"},
			want: `unknown command "serch" for "civitai models"` + "\n\n" +
				"Did you mean this?\n\tsearch\n\n" +
				"Run 'civitai models --help' for the available subcommands.\n" +
				`"civitai models serch" ` + hintTail,
		},
		{
			// The realistic agent invocation carries flags the old binary does not
			// know either. Cobra parses them against the ROOT before any Args/RunE,
			// so without unknownGroupSubcommand this reported `unknown flag: --about`
			// and the hint never appeared.
			name: "top-level unknown followed by a flag",
			args: []string{"feedbackxyz", "send", "--about", "x"},
			want: `unknown command "feedbackxyz" for "civitai"` + "\n" +
				"Run 'civitai --help' for the available subcommands.\n" +
				`"civitai feedbackxyz" ` + hintTail,
		},
		{
			name: "unknown subcommand under a group followed by a flag",
			args: []string{"models", "frobnicate", "--nope"},
			want: `unknown command "frobnicate" for "civitai models"` + "\n" +
				"Run 'civitai models --help' for the available subcommands.\n" +
				`"civitai models frobnicate" ` + hintTail,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errb, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("%v: expected an unknown-command error, got nil", tc.args)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("%v: error text mismatch\n got: %q\nwant: %q", tc.args, got, tc.want)
			}
			// The exit code is unchanged: still a usage error (exit 2).
			if !errors.Is(err, ErrUsage) {
				t.Errorf("%v: should stay usage-tagged (exit 2), got %T", tc.args, err)
			}
			if errb != "" {
				t.Errorf("%v: cobra must print nothing itself (SilenceErrors; main renders the error), got %q", tc.args, errb)
			}
		})
	}
}

// TestKnownCommandErrorsHaveNoUpgradeHint: the hint is about a command this
// binary does not know. A KNOWN command's usage errors — a stray positional on
// a leaf (cobra.NoArgs, which also says "unknown command"), a missing required
// positional, an unknown flag — must not gain it.
func TestKnownCommandErrorsHaveNoUpgradeHint(t *testing.T) {
	pinVersion(t, "9.8.7")
	cases := []struct {
		args []string
		want string
	}{
		{
			args: []string{"whoami", "extra"},
			want: `unknown command "extra" for "civitai whoami"`,
		},
		{
			args: []string{"models", "get"},
			want: "accepts 1 arg(s), received 0",
		},
		{
			args: []string{"models", "search", "--nope"},
			want: "unknown flag: --nope\nRun 'civitai models search --help' for the available flags",
		},
		{
			// A bad flag on a group with NO positional is still a flag error.
			args: []string{"models", "--nope"},
			want: "unknown flag: --nope\nRun 'civitai models --help' for the available flags",
		},
		{
			args: []string{"--nope"},
			want: "unknown flag: --nope\nRun 'civitai --help' for the available flags",
		},
		{
			// A KNOWN subcommand after a bad root flag. Cobra's Find reads `--nope`
			// as taking a value and swallows `models`, so the flag fails on the
			// root (measured) — and must not become an "unknown command".
			args: []string{"--nope", "models"},
			want: "unknown flag: --nope\nRun 'civitai --help' for the available flags",
		},
		{
			// A known subcommand collected BEFORE the bad flag never reads as unknown.
			args: []string{"models", "search", "frob", "--nope"},
			want: "unknown flag: --nope\nRun 'civitai models search --help' for the available flags",
		},
	}
	for _, tc := range cases {
		_, _, err := run(t, tc.args...)
		if err == nil {
			t.Fatalf("%v: expected an error, got nil", tc.args)
		}
		if got := err.Error(); got != tc.want {
			t.Errorf("%v: a known command's error must be unchanged\n got: %q\nwant: %q", tc.args, got, tc.want)
		}
	}
}
