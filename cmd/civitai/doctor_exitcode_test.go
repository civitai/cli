package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// THE EXIT CODE OF `civitai app doctor`'s BLOCKING VERDICT.
//
// The classification and the PROCESS contract are two different claims, and the
// second is the one a release script reads. `internal/cmd` can tag the verdict
// perfectly while `exitCode` routes it somewhere else — the same gap
// submit_version_guard_exitcode_test.go in this package exists to close for a
// different path.
//
// The verdict is 1, not 2, and that is a DECISION rather than a fallthrough:
// exit 2 is documented as a mistake about the INVOCATION, and every flag and
// argument in `civitai app doctor my-app` is well-formed when this fires. What
// is wrong is the LISTING — a verdict about the subject, which exitCodeDocs
// already publishes under code 1 (`app validate` lands there for the same
// reason).
//
// 🔴 The verdict is left UNTAGGED for the exit mapper on purpose, so this test
// is what makes that silence deliberate rather than accidental: tagging it
// civitai.ErrBadRequest — the only route from a command error to exit 2 — would
// move it, and nothing else in the suite would notice.

func TestDoctorBlockingExitsGeneric(t *testing.T) {
	err := fmt.Errorf("%w: 2 blocking problem(s) across 1 app(s) — see the report above", cmd.ErrListingBlocked)

	if !errors.Is(err, cmd.ErrListingBlocked) {
		t.Fatal("fixture is not tagged with ErrListingBlocked — the test would prove nothing")
	}
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exitCode(doctor blocking verdict) = %d, want %d (generic — a verdict about the listing).\n"+
			"2 is a mistake about the INVOCATION; the flags and arguments here are all well-formed.", got, exitGeneric)
	}

	// Wrapped by an outer message (cobra/RunE chains do this) it must keep its
	// code — an errors.Is walk, never a top-level type check.
	wrapped := fmt.Errorf("app doctor: %w", err)
	if got := exitCode(wrapped); got != exitGeneric {
		t.Errorf("wrapped: exitCode = %d, want %d", got, exitGeneric)
	}

	// NEGATIVE CONTROL: exitCode CAN return something other than exitGeneric, so
	// the assertion above is not a fact about a function that always says 1.
	if got := exitCode(civitai.Tag(civitai.ErrBadRequest, errors.New("bad enum"))); got != exitUsage {
		t.Fatalf("negative control: exitCode(ErrBadRequest) = %d, want %d — the instrument is not discriminating", got, exitUsage)
	}
}

// TestDoctorBlockingSentinelIsNotAnAPIKind pins the OTHER half: the sentinel
// must not accidentally satisfy one of the API classification kinds, which would
// silently move the verdict onto 2/3/4/5/6 — and 4 in particular, because
// `app doctor <unknown-slug>` deliberately DOES exit 4 and the two answers must
// stay separately actionable.
func TestDoctorBlockingSentinelIsNotAnAPIKind(t *testing.T) {
	err := fmt.Errorf("%w: 1 blocking problem(s)", cmd.ErrListingBlocked)
	kinds := map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	for name, kind := range kinds {
		if errors.Is(err, kind) {
			t.Errorf("a doctor blocking verdict must not match civitai.%s — that would move its exit code", name)
		}
	}
	// POSITIVE CONTROL on the loop: an error that IS one of these kinds must be
	// seen, or the five negatives above are a fact about a walk that matches
	// nothing.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), kinds["ErrNotFound"]) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives above prove nothing")
	}
	// And the sibling verdict `app doctor` really does emit for an unknown slug
	// must land on 4, so a script can tell "not ready" from "no such app".
	if got := exitCode(civitai.Tag(civitai.ErrNotFound, errors.New(`no listing of yours is called "nope"`))); got != exitNotFound {
		t.Errorf("exitCode(unknown-slug refusal) = %d, want %d", got, exitNotFound)
	}
}

// TestDoctorCleanRunExitsZero is the third arm of the exit contract at the
// PROCESS level: `runCLI` must map a nil error to 0. Without it, the two
// assertions above are equally true of a binary that never returns 0 at all.
func TestDoctorCleanRunExitsZero(t *testing.T) {
	if got := exitCode(nil); got != 0 {
		t.Errorf("exitCode(nil) = %d, want 0 — a clean `app doctor` run must not fail a release script", got)
	}
}
