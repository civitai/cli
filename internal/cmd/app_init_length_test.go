package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #291, through the real command. The unit-level pins live in
// internal/scaffold/slug_length_test.go; what is asserted HERE is the half the
// scaffold package cannot see — the exit code the user gets, and that nothing is
// written to disk when the name is refused.
//
// 🔴 THE EXIT CODE IS PINNED WITH errors.Is, NEVER MESSAGE TEXT (AGENTS item 7).
// The classification sentinel carries no visible text, so a message assertion
// says nothing about whether this exits 2 — measured in this repo: stripping a
// classification while leaving every message identical left the whole suite
// green. The message substring below identifies WHICH guard fired; `ErrUsage` is
// what pins the published exit code.

// oneNameShortOfForty and its siblings are the same fixtures the unit tests use,
// restated here so the command-level rows are readable on their own.
const (
	cmdLongNameA = "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee"
	cmdLongNameB = "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd ZZZZZZZZZZ"
	cmdCollided  = "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd"

	cmdFortyName = "aaaaaaaaaa bbbbbbbbbb cccccccccc ddddddd"
	cmdFortySlug = "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd"
)

// TestScaffoldRefusesANameThatWouldTruncateTheBlockID: `app create` with a name
// whose derived blockId runs past 40 chars exits 2 and writes nothing, instead of
// scaffolding a project around a truncated permanent id.
func TestScaffoldRefusesANameThatWouldTruncateTheBlockID(t *testing.T) {
	for i, name := range []string{cmdLongNameA, cmdLongNameB} {
		t.Run(map[int]string{0: "first", 1: "second"}[i], func(t *testing.T) {
			tmp := t.TempDir()
			dest := filepath.Join(tmp, "out")
			stdout, _, err := run(t, "app", "create", name, "--dir", dest, "--template", "static")
			if err == nil {
				t.Fatalf("app create %q should be refused; it scaffolded instead:\n%s", name, stdout)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("an over-length derived blockId is a bad VALUE, i.e. a usage error (exit 2): "+
					"errors.Is(err, ErrUsage) = false (%v)", err)
			}
			if !strings.Contains(err.Error(), "the limit is 40") {
				t.Errorf("the refusal must be the LENGTH guard's, not another guard that happens to fire: %v", err)
			}
			if _, statErr := os.Stat(dest); statErr == nil {
				t.Error("the name was refused but a project was still written")
			}
			// The pre-fix id must not appear anywhere on stdout either: the
			// echo line is what made #291 visible, and re-printing the
			// truncation would be the same silent claim in a new place.
			if strings.Contains(stdout, cmdCollided) {
				t.Errorf("output still names the truncated blockId %q:\n%s", cmdCollided, stdout)
			}
		})
	}
}

// TestScaffoldAtExactlyFortyCharsStillScaffolds is the BOUNDARY CONTROL through
// the command: 40 is at the cap, not over it. It asserts the manifest, not just
// the printed line — the manifest is the authority on the app's identity.
func TestScaffoldAtExactlyFortyCharsStillScaffolds(t *testing.T) {
	if len(cmdFortySlug) != 40 {
		t.Fatalf("CONTROL failure: fixture slug is %d chars, want 40", len(cmdFortySlug))
	}
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out")
	stdout, _, err := run(t, "app", "create", cmdFortyName, "--dir", dest, "--template", "static")
	if err != nil {
		t.Fatalf("app create %q (derives exactly 40 chars): %v\n%s", cmdFortyName, err, stdout)
	}
	if got := manifestBlockID(t, dest); got != cmdFortySlug {
		t.Errorf("blockId = %q, want %q", got, cmdFortySlug)
	}
	if !strings.Contains(stdout, cmdFortySlug) {
		t.Errorf("output must echo the 40-char blockId:\n%s", stdout)
	}
}

// TestOverLengthExplicitSlugIsUnchanged is the CONTROL for the behaviour #291
// makes the derived path agree with. It must not move: same exit class, same
// message. (TestSlugFlagRejectsAnInvalidValue already carries a 41-char row for
// the exit code; this one additionally pins the WORDING, because "make the two
// paths consistent" is exactly the change that tempts someone to rewrite it.)
func TestOverLengthExplicitSlugIsUnchanged(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out")
	over := strings.Repeat("a", 41)
	_, _, err := run(t, "app", "create", "My Block", "--slug", over, "--dir", dest, "--template", "static")
	if err == nil {
		t.Fatal("an over-length explicit --slug must still be refused")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("an over-length --slug must stay a usage error: %v", err)
	}
	if !strings.Contains(err.Error(), "must be 3-40 chars") {
		t.Errorf("the explicit --slug message moved: %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("--slug was refused but a project was still written")
	}
}
