package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// The R3 arc closes cobra-layer exit-code gaps: usage mistakes at the cobra
// dispatch/arg-validation layer (unknown subcommand, unknown top-level command,
// missing required positional) used to leak exit 0 (silent success) or exit 1
// (generic) instead of the usage code (2). These tests pin the classification
// (errors.Is(err, ErrUsage), which the entrypoint maps to exit 2) WITHOUT
// reclassifying any genuine runtime failure (a real 404 stays not-found → 4).

// TestUnknownSubcommandIsUsageTagged covers bug 1: an unrecognized subcommand on
// a group-only parent (models/users/app …) used to print the parent's help and
// exit 0 — a silent success a script would read as "it worked". It must now be a
// usage error (exit 2).
func TestUnknownSubcommandIsUsageTagged(t *testing.T) {
	cases := [][]string{
		{"models", "frobnicate"},
		{"users", "show", "Lykon"}, // `show` is not a users subcommand (`get` is)
		{"app", "dev"},             // `dev` is not an app subcommand (`dev-token`/`dev-tunnel` are)
		{"images", "serch"},        // typo'd `search`
	}
	for _, args := range cases {
		_, _, err := run(t, args...)
		if err == nil {
			t.Fatalf("%v: expected an error for an unknown subcommand, got nil (silent exit 0)", args)
		}
		if !errors.Is(err, ErrUsage) {
			t.Errorf("%v: unknown subcommand should classify as usage (exit 2), got %T: %v", args, err, err)
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("%v: message should name the unknown command, got: %v", args, err)
		}
	}
}

// TestUnknownSubcommandSuggestsClosest proves the message keeps cobra's
// newcomer-friendly "Did you mean this?" hint — a near-miss typo of a real
// subcommand surfaces the intended one.
func TestUnknownSubcommandSuggestsClosest(t *testing.T) {
	_, _, err := run(t, "models", "serch") // near-miss of `search`
	if err == nil {
		t.Fatal("expected an error for `models serch`")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("near-miss subcommand should classify as usage, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Did you mean this?") || !strings.Contains(err.Error(), "search") {
		t.Errorf("message should suggest the closest subcommand (search), got: %v", err)
	}
}

// TestMissingRequiredArgIsUsageTagged covers bug 2: the `get`/`by-hash` commands
// use cobra.ExactArgs, whose "accepts 1 arg(s), received 0" error used to map to
// the generic code (1). Every one must now be a usage error (exit 2), matching
// `download` (which already returned 2).
func TestMissingRequiredArgIsUsageTagged(t *testing.T) {
	cases := [][]string{
		{"models", "get"},
		{"images", "get"},
		{"articles", "get"},
		{"collections", "get"},
		{"model-versions", "get"},
		{"model-versions", "by-hash"},
		{"users", "get"},
	}
	for _, args := range cases {
		_, _, err := run(t, args...)
		if err == nil {
			t.Fatalf("%v: expected an error for a missing required positional, got nil", args)
		}
		if !errors.Is(err, ErrUsage) {
			t.Errorf("%v: missing required arg should classify as usage (exit 2), got %T: %v", args, err, err)
		}
	}
}

// TestTopLevelUnknownCommandIsUsageTagged covers bug 3: an unknown TOP-LEVEL
// command used to exit 1 (generic) while an unknown FLAG already exited 2. Unify
// onto usage (exit 2).
func TestTopLevelUnknownCommandIsUsageTagged(t *testing.T) {
	_, _, err := run(t, "nonsensecmd")
	if err == nil {
		t.Fatal("expected an error for an unknown top-level command")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("unknown top-level command should classify as usage (exit 2), got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("message should name the unknown command, got: %v", err)
	}
}

// TestBareParentPrintsHelpAndExitsZero proves the fix does NOT over-reach: a
// bare `civitai <parent>` with no subcommand must still print help and succeed
// (exit 0), not be treated as a usage error.
func TestBareParentPrintsHelpAndExitsZero(t *testing.T) {
	for _, parent := range []string{"models", "images", "users", "app", "model-versions"} {
		out, _, err := run(t, parent)
		if err != nil {
			t.Errorf("bare `civitai %s` should print help and exit 0, got error: %v", parent, err)
		}
		// Help output lists the child commands (e.g. the "Available Commands:"
		// section), so it should be non-trivial.
		if !strings.Contains(out, "Available Commands") && !strings.Contains(out, "Usage:") {
			t.Errorf("bare `civitai %s` should emit help, got: %q", parent, out)
		}
	}
}

// TestValidSubcommandUnaffected proves a genuinely valid subcommand invocation
// still parses and runs (its own success/RunE path), unchanged by the walk.
func TestValidSubcommandUnaffected(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	// `models search` (a valid subcommand with no required positional) succeeds.
	if _, _, err := run(t, "models", "search", "--limit", "1"); err != nil {
		t.Errorf("a valid subcommand should still run cleanly, got: %v", err)
	}
}

// TestRealNotFoundNotReclassified is the load-bearing guard: a genuine 404 from
// the API (a valid invocation whose RESOURCE is missing) must keep its not-found
// classification (exit 4). The usage-tagging walk must never reclassify a real
// runtime error as a usage error.
func TestRealNotFoundNotReclassified(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	_, _, err := run(t, "models", "get", "999999999")
	if err == nil {
		t.Fatal("expected a not-found error for a missing model id")
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("a real 404 must NOT be reclassified as a usage error, got: %v", err)
	}
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a real 404 should stay not-found (exit 4), got %T: %v", err, err)
	}
}
