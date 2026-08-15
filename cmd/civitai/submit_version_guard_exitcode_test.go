package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// THE EXIT CODE OF THE MONOTONIC-VERSION REFUSAL (issue #412).
//
// The classification and the process contract are two different claims, and the
// second is the one scripts read. `cmd` can tag an error perfectly while
// `exitCode` routes it somewhere else — which is exactly what
// generate_input_exitcode_test.go in this package exists to catch for a
// different path.
//
// The verdict is 1, not 2, and that is a DECISION rather than a fallthrough:
// exit 2 is documented as a mistake about the INVOCATION, and every flag,
// argument and path in `civitai app submit --yes` is well-formed when this fires.
// What is wrong is the PROJECT relative to what is published — the same shape as
// an invalid manifest, which exitCodeDocs already publishes under code 1 as a
// validation verdict.
//
// 🔴 The refusal is left UNTAGGED for the exit mapper on purpose, so this test
// is what makes that silence deliberate rather than accidental: tagging it with
// civitai.ErrBadRequest (the only route from a command error to exit 2) would
// move it, and nothing else in the suite would notice.

func TestVersionRegressionExitsGeneric(t *testing.T) {
	err := civitai.Tag(cmd.ErrVersionRegression, errors.New("refusing to submit demo@0.4.1 — 0.5.2 is already approved and live."))

	if !errors.Is(err, cmd.ErrVersionRegression) {
		t.Fatal("fixture is not tagged with ErrVersionRegression — the test would prove nothing")
	}
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exitCode(version regression) = %d, want %d (generic — a validation verdict).\n"+
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

// TestVersionRegressionSentinelIsNotAnAPIKind pins the OTHER half: the new
// sentinel must not accidentally satisfy one of the API classification kinds,
// which would silently move the refusal onto 2/3/4/5/6.
func TestVersionRegressionSentinelIsNotAnAPIKind(t *testing.T) {
	err := civitai.Tag(cmd.ErrVersionRegression, errors.New("refusing to submit demo@0.4.1"))
	kinds := map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	for name, kind := range kinds {
		if errors.Is(err, kind) {
			t.Errorf("a version-regression error must not match civitai.%s — that would move its exit code", name)
		}
	}
	// POSITIVE CONTROL on the loop: an error that IS one of these kinds must be
	// seen, or the five negatives above are a fact about a walk that matches
	// nothing.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), kinds["ErrNotFound"]) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives above prove nothing")
	}
}
