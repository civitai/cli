package cmd

import (
	"errors"
	"strings"
	"testing"
)

// wantNoSubmissionMsg is the exact text a slug with no submission gets. Written
// out here, not rebuilt from the implementation, so an accidental edit to either
// side is visible (AGENTS.md: never derive an expectation from the code under
// test).
func wantNoSubmissionMsg(slug string) string {
	return "no such submission for app \"" + slug + "\" — run `civitai app submit` first; " +
		"the submission and its draft store listing are created at submit time " +
		"(list what you have submitted with `civitai app status`)"
}

// TestNoSubmissionNamesSubmit is civitai/cli#363 §1: `app listing status` right
// after `app create` answered `Error: no such submission` — three words, no next
// step — in a tool whose house style always names the next command. Both
// commands that resolve a slug through the empty-list path are pinned.
func TestNoSubmissionNamesSubmit(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"app listing status --slug", []string{"app", "listing", "status", "--slug", "f6-probe-app"}},
		{"app status <slug>", []string{"app", "status", "f6-probe-app"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			emptySubmissionsServer(t)
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected an error before anything is submitted")
			}
			msg := err.Error()
			if msg == "no such submission" {
				t.Fatalf("the bare three-word error is the regression: %q", msg)
			}
			for _, want := range []string{"civitai app submit", "civitai app status"} {
				if !strings.Contains(msg, want) {
					t.Errorf("missing next command %q; got: %s", want, msg)
				}
			}
			if !strings.Contains(msg, "f6-probe-app") {
				t.Errorf("the error must name the slug it looked up; got: %s", msg)
			}
		})
	}
}

// TestAppMetricsNoArgNamesTheSlug is civitai/cli#363 §4: a bare `app metrics`
// hit cobra's own "accepts 1 arg(s), received 0", while `app dev-token` and
// `app dev-tunnel` in the same situation name the argument AND how to find one.
func TestAppMetricsNoArgNamesTheSlug(t *testing.T) {
	_, _, err := run(t, "app", "metrics")
	if err == nil {
		t.Fatal("expected an error with no slug")
	}
	msg := err.Error()
	if strings.Contains(msg, "accepts 1 arg(s), received 0") {
		t.Errorf("the raw cobra message is the regression: %q", msg)
	}
	for _, want := range []string{"an app slug is required", "civitai app metrics my-block", "civitai app status"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got: %s", want, msg)
		}
	}
	// AGENTS.md item 7 — a missing argument stays a usage error (exit 2).
	if !errors.Is(err, ErrUsage) {
		t.Errorf("a missing required argument must classify as a usage error (exit 2), got %T: %v", err, err)
	}
}

// TestAppMetricsTooManyArgsStillRefuses is the negative control for the custom
// Args validator: replacing cobra.ExactArgs(1) must not accidentally accept a
// second argument (which would silently report on only the first app).
func TestAppMetricsTooManyArgsStillRefuses(t *testing.T) {
	_, _, err := run(t, "app", "metrics", "a", "b")
	if err == nil {
		t.Fatal("expected an error with two slugs")
	}
	if !strings.Contains(err.Error(), "civitai app metrics <slug>") {
		t.Errorf("the too-many-args error must name the shape; got: %v", err)
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("want a usage error (exit 2), got %T: %v", err, err)
	}
}

// TestAppPullPositionalSlugNamesTheTwoSlots is civitai/cli#363 §2: `app pull` is
// the one `app` subcommand whose slug is a FLAG and whose positional is the
// DIRECTORY, and every natural first attempt answered with a message that never
// says so — including a bare cobra "accepts at most 1 arg(s), received 2".
func TestAppPullPositionalSlugNamesTheTwoSlots(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"slug typed positionally", []string{"app", "pull", "my-block"}},
		{"slug then dir", []string{"app", "pull", "my-block", "./pulled"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok-1")
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected a usage error")
			}
			msg := err.Error()
			if strings.Contains(msg, "accepts at most 1 arg(s), received") {
				t.Errorf("the raw cobra message is the regression: %q", msg)
			}
			for _, want := range []string{"--app", "DIRECTORY", "civitai app pull [dir] --app <slug>"} {
				if !strings.Contains(msg, want) {
					t.Errorf("missing %q; got: %s", want, msg)
				}
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("want a usage error (exit 2), got %T: %v", err, err)
			}
		})
	}
}

// TestAppPullBareInvocationNamesTheTwoSlots: `civitai app pull` with nothing at
// all is the most likely FIRST-CONTACT invocation, and it was the one shape the
// new validator did not cover — cobra's required-flag check answered it with
// `required flag(s) "app" not set`, which never says the positional is the
// directory.
func TestAppPullBareInvocationNamesTheTwoSlots(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	_, _, err := run(t, "app", "pull")
	if err == nil {
		t.Fatal("expected a usage error")
	}
	msg := err.Error()
	if strings.Contains(msg, `required flag(s) "app" not set`) {
		t.Errorf("the raw cobra message is the regression: %q", msg)
	}
	for _, want := range []string{"--app", "DIRECTORY", "civitai app pull [dir] --app <slug>"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got: %s", want, msg)
		}
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("want a usage error (exit 2), got %T: %v", err, err)
	}
}

// TestAppPullTooManyArgsWithTheFlagSetDoesNotBlameTheFlag: with `--app` already
// present, "the app goes in --app" is a non sequitur — the app IS in --app, and
// the only thing wrong is the extra positional.
func TestAppPullTooManyArgsWithTheFlagSetDoesNotBlameTheFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	_, _, err := run(t, "app", "pull", "a", "b", "--app", "my-block")
	if err == nil {
		t.Fatal("expected a usage error")
	}
	msg := err.Error()
	if strings.Contains(msg, "the app goes in --app") {
		t.Errorf("--app was passed; got: %s", msg)
	}
	for _, want := range []string{"at most 1 arg", "DIRECTORY", "civitai app pull [dir] --app <slug>"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got: %s", want, msg)
		}
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("want a usage error (exit 2), got %T: %v", err, err)
	}

	// Control: WITHOUT --app the pointer to the flag is the whole point.
	_, _, err = run(t, "app", "pull", "a", "b")
	if err == nil || !strings.Contains(err.Error(), "the app goes in --app") {
		t.Errorf("with no --app the message must still name where the app goes; got: %v", err)
	}
}

// TestAppPullWithDirAndFlagIsAccepted is the negative control: the correct
// invocation must still parse (the Args validator only rejects the shapes that
// mean the user swapped the two slots).
func TestAppPullWithDirAndFlagIsAccepted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "pull", "./pulled", "--app", "my-block")
	if err == nil {
		t.Fatal("expected the no-token error (the command ran past argument validation)")
	}
	if !strings.Contains(err.Error(), "no token") {
		t.Errorf("`app pull [dir] --app <slug>` must reach RunE, got: %v", err)
	}
}
