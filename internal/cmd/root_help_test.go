package cmd

import (
	"strings"
	"testing"
)

// TestRootHelpPresentsBothPaths proves the root command foregrounds BOTH the
// browse/download path and the Apps path (dogfood: the README #138 now leads
// with browse/download, but the binary's root help still presented the CLI as
// Apps-only). Short + Long + Example must name both, and the browse examples
// must use REAL subcommands.
func TestRootHelpPresentsBothPaths(t *testing.T) {
	cmd := NewRootCmd()
	short := strings.ToLower(cmd.Short)
	blob := cmd.Short + "\n" + cmd.Long + "\n" + cmd.Example
	low := strings.ToLower(blob)

	// Short must name both paths — browse/download AND Apps.
	if !strings.Contains(short, "download") && !strings.Contains(short, "browse") {
		t.Errorf("root Short should mention browse/download, got %q", cmd.Short)
	}
	if !strings.Contains(short, "app") {
		t.Errorf("root Short should still mention Apps, got %q", cmd.Short)
	}

	// Long + Example must present both paths.
	for _, want := range []string{"browse", "download", "app"} {
		if !strings.Contains(low, want) {
			t.Errorf("root help should mention %q:\n%s", want, blob)
		}
	}

	// The browse/download examples must use REAL subcommand invocations.
	for _, want := range []string{"civitai models search", "civitai download"} {
		if !strings.Contains(blob, want) {
			t.Errorf("root help should show a real browse/download example %q:\n%s", want, blob)
		}
	}
	// …and still an app example.
	if !strings.Contains(blob, "civitai app create") {
		t.Errorf("root help should still show an app example (civitai app create):\n%s", blob)
	}
}

// TestRootHelpExamplesResolveToRealCommands proves the commands named in the
// root help (models search, download, app create) actually exist in the tree —
// so the examples can't drift into naming a command that isn't wired up.
func TestRootHelpExamplesResolveToRealCommands(t *testing.T) {
	root := NewRootCmd()
	cases := [][]string{
		{"models", "search"},
		{"download"},
		{"app", "create"},
	}
	for _, path := range cases {
		c, _, err := root.Find(path)
		if err != nil {
			t.Errorf("root help example command %v does not resolve: %v", path, err)
			continue
		}
		if c.Name() != path[len(path)-1] {
			t.Errorf("root help example %v resolved to %q, want %q", path, c.Name(), path[len(path)-1])
		}
	}
}
