package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// renderRootHelp returns what a user actually sees from `civitai --help`.
//
// It renders through cobra's help path (template + UsageString), NOT cmd.Long,
// because the exit-code section deliberately no longer lives in Long — see
// rootHelpTemplate in root.go. Reading Long would silently stop observing it.
func renderRootHelp(t *testing.T) string {
	t.Helper()
	root := NewRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := root.Help(); err != nil {
		t.Fatalf("render root help: %v", err)
	}
	got := buf.String()
	// Positive control: an empty (or usage-less) render would make every
	// ordering assertion below vacuously true.
	if !strings.Contains(got, "Available Commands:") {
		t.Fatalf("root help render produced no command list — the harness is reading the wrong text:\n%s", got)
	}
	return got
}

// TestRootHelpShowsUsageBeforeExitCodes is the ordering guard for issue #260.
//
// `civitai --help` used to open with the ~60-line exit-code taxonomy (it was
// appended to Long, and cobra prints Long before the usage block), so the list
// of things the binary can DO sat below the fold for a first-time reader. The
// taxonomy is still published in full — it just comes last now.
//
// The assertion is on INDICES, not on presence: "the help contains both" is
// satisfied by the broken order too.
func TestRootHelpShowsUsageBeforeExitCodes(t *testing.T) {
	help := renderRootHelp(t)

	usage := strings.Index(help, "Usage:")
	commands := strings.Index(help, "Available Commands:")
	exits := strings.Index(help, "Exit codes:")

	if usage < 0 {
		t.Fatalf("root help has no %q block:\n%s", "Usage:", help)
	}
	if exits < 0 {
		t.Fatalf("root help no longer publishes the exit-code contract:\n%s", help)
	}
	if usage > exits {
		t.Errorf("`Usage:` (index %d) comes AFTER the exit-code section (index %d) — "+
			"the taxonomy is back above the fold", usage, exits)
	}
	if commands > exits {
		t.Errorf("`Available Commands:` (index %d) comes AFTER the exit-code section (index %d) — "+
			"a first-time reader has to scroll past a reference table to find the commands", commands, exits)
	}
	// The section must still be the GENERATED one, byte for byte. Without this a
	// "fix" that simply deleted it from Long would pass the ordering checks by
	// having nothing left to order.
	if !strings.Contains(help, rootExitCodeHelp()) {
		t.Errorf("root help does not carry the rendered exit-code section verbatim:\n%s", help)
	}
}

// TestSubcommandHelpDoesNotRepeatTheExitCodeSection pins the `{{if not
// .HasParent}}` gate in rootHelpTemplate.
//
// Command.HelpTemplate() walks UP to the parent when a command sets none of its
// own, so a template set on the root is inherited by every subcommand. Without
// the gate, moving the section into the template would have stapled the whole
// ~60-line taxonomy onto `civitai app validate --help` and every other
// subcommand help — strictly worse than the problem it fixes.
func TestSubcommandHelpDoesNotRepeatTheExitCodeSection(t *testing.T) {
	// Positive control first: the ROOT must carry it, so a green here cannot
	// mean "the section does not exist anywhere".
	if !strings.Contains(renderRootHelp(t), "Exit codes:") {
		t.Fatal("the root help does not carry the exit-code section — the control is broken")
	}

	for _, path := range [][]string{{"app"}, {"app", "validate"}, {"login"}, {"generate"}} {
		name := strings.Join(path, " ")
		t.Run(name, func(t *testing.T) {
			out, _, err := run(t, append(path, "--help")...)
			if err != nil {
				t.Fatalf("%s --help: %v", name, err)
			}
			if !strings.Contains(out, "Usage:") {
				t.Fatalf("%s --help produced no usage block — the probe is reading the wrong text:\n%s", name, out)
			}
			if strings.Contains(out, "Exit codes:") {
				t.Errorf("`civitai %s --help` inherited the ROOT's exit-code section — "+
					"the {{if not .HasParent}} gate in rootHelpTemplate is gone:\n%s", name, out)
			}
		})
	}
}

// TestUnknownFlagNamesTheHelpCommandForThatCommand is the issue-#260 guard for
// the unknown-FLAG path.
//
// A mistyped flag used to print a bare `unknown flag: --stict` with no next
// step, while a mistyped COMMAND already got `Run 'civitai app --help' …`. The
// hint must name the command whose flags were being parsed — pointing a user at
// the ROOT's flag list for a subcommand's typo is the wrong answer, so the test
// asserts the FULL command path rather than merely "some --help".
//
// The exit code is pinned with errors.Is, never by message text (AGENTS item 7):
// stripping the ErrUsage tag leaves every character of the message intact.
func TestUnknownFlagNamesTheHelpCommandForThatCommand(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"app subcommand", []string{"app", "validate", "--stict"}, "Run 'civitai app validate --help'"},
		{"top-level command", []string{"models", "search", "--quiry", "x"}, "Run 'civitai models search --help'"},
		{"root", []string{"--nocolour"}, "Run 'civitai --help'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("%v: expected a flag-parse error", tc.args)
			}
			// Exit code 2 (usage). Asserted structurally, not from the text.
			if !errors.Is(err, ErrUsage) {
				t.Errorf("a bad flag must classify as ErrUsage (exit 2), got %T: %v", err, err)
			}
			// Cobra's own diagnosis is APPENDED to, never replaced.
			if !strings.Contains(err.Error(), "unknown flag") {
				t.Errorf("cobra's own wording was lost: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error must name the next step %q, got: %v", tc.want, err)
			}
		})
	}
}

// TestUnknownFlagHintDoesNotNameTheWrongCommand is the discriminating half of
// the test above: a hint hard-coded to the root would satisfy every "contains
// --help" assertion while sending a user to the wrong flag list.
func TestUnknownFlagHintDoesNotNameTheWrongCommand(t *testing.T) {
	_, _, err := run(t, "app", "validate", "--stict")
	if err == nil {
		t.Fatal("expected a flag-parse error")
	}
	if strings.Contains(err.Error(), "Run 'civitai --help'") {
		t.Errorf("a subcommand's bad flag was answered with the ROOT's help command: %v", err)
	}
}
