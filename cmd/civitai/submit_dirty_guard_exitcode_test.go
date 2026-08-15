package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// THE EXIT CODE OF THE DIRTY-WORK-TREE REFUSAL (issue #411).
//
// The classification and the process contract are two different claims, and the
// second is the one scripts read: `cmd` can tag an error perfectly while
// `exitCode` routes it somewhere else.
//
// The verdict is 1, not 2, and it is the same DECISION #412's refusal made for
// the same reason — exit 2 is documented as a mistake about the INVOCATION, and
// every flag, argument and path in `civitai app submit --yes` is well-formed
// when this fires. What is wrong is the PROJECT: its working tree against its
// own history, the same shape exitCodeDocs publishes under code 1 as a
// validation verdict.
//
// 🔴 The refusal is left UNTAGGED for the exit mapper on purpose, so this test
// is what makes that silence deliberate rather than accidental: tagging it with
// civitai.ErrBadRequest (the only route from a command error to exit 2) would
// move it, and nothing else in the suite would notice.

func TestDirtyWorkTreeExitsGeneric(t *testing.T) {
	err := civitai.Tag(cmd.ErrDirtyWorkTree, errors.New("refusing to submit demo@0.6.1 from a dirty git work tree"))

	if !errors.Is(err, cmd.ErrDirtyWorkTree) {
		t.Fatal("fixture is not tagged with ErrDirtyWorkTree — the test would prove nothing")
	}
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exitCode(dirty work tree) = %d, want %d (generic — a verdict about the project).\n"+
			"2 is a mistake about the INVOCATION; the flags and paths here are all well-formed.", got, exitGeneric)
	}

	// Wrapped by an outer message (cobra/RunE chains do this) it must keep its
	// code — an errors.Is walk, never a top-level type check.
	wrapped := fmt.Errorf("app submit: %w", err)
	if got := exitCode(wrapped); got != exitGeneric {
		t.Errorf("wrapped: exitCode = %d, want %d", got, exitGeneric)
	}

	// NEGATIVE CONTROL: exitCode CAN return something other than exitGeneric, so
	// the assertion above is not a fact about a function that always says 1.
	if got := exitCode(civitai.Tag(civitai.ErrBadRequest, errors.New("bad enum"))); got != exitUsage {
		t.Fatalf("negative control: exitCode(ErrBadRequest) = %d, want %d — the instrument is not discriminating", got, exitUsage)
	}
}

// TestDirtyWorkTreeSentinelIsNotAnAPIKind pins the other half: the sentinel must
// not accidentally satisfy an API classification kind, which would silently move
// the refusal onto 2/3/4/5/6.
func TestDirtyWorkTreeSentinelIsNotAnAPIKind(t *testing.T) {
	err := civitai.Tag(cmd.ErrDirtyWorkTree, errors.New("refusing to submit demo@0.6.1"))
	kinds := map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	for name, kind := range kinds {
		if errors.Is(err, kind) {
			t.Errorf("a dirty-work-tree error must not match civitai.%s — that would move its exit code", name)
		}
	}
	// POSITIVE CONTROL on the loop.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), kinds["ErrNotFound"]) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives above prove nothing")
	}
}

// TestDirtyWorkTreeAndVersionRegressionAreDistinctSentinels — the two submit
// refusals must be separately identifiable. A single shared sentinel (or one
// wrapping the other) would make `errors.Is` unable to tell a release script
// which escape hatch applies: --allow-dirty and --allow-downgrade are not
// interchangeable.
func TestDirtyWorkTreeAndVersionRegressionAreDistinctSentinels(t *testing.T) {
	dirty := civitai.Tag(cmd.ErrDirtyWorkTree, errors.New("dirty"))
	regression := civitai.Tag(cmd.ErrVersionRegression, errors.New("regression"))

	if errors.Is(dirty, cmd.ErrVersionRegression) {
		t.Error("a dirty-tree refusal must not read as a version regression")
	}
	if errors.Is(regression, cmd.ErrDirtyWorkTree) {
		t.Error("a version regression must not read as a dirty-tree refusal")
	}
	// Both still land on the same exit code, and that is deliberate: they are
	// the same KIND of verdict, distinguished by sentinel rather than by code.
	if exitCode(dirty) != exitCode(regression) {
		t.Errorf("both submit refusals are project verdicts and share exit %d; got dirty=%d regression=%d",
			exitGeneric, exitCode(dirty), exitCode(regression))
	}
}
