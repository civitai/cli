package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/cmd"
	"github.com/civitai/cli/pkg/civitai"
)

// fakeNetErr is a minimal net.Error for exercising the transport-error branch.
type fakeNetErr struct{ timeout bool }

func (e fakeNetErr) Error() string   { return "fake net error" }
func (e fakeNetErr) Timeout() bool   { return e.timeout }
func (e fakeNetErr) Temporary() bool { return false }

// usageErr mirrors internal/cmd's (unexported) usageError: a flag error tagged
// with cmd.ErrUsage while preserving its message. It lets this external test
// verify the same message-preserving classification the FlagErrorFunc produces.
type usageErr struct{ err error }

func (u usageErr) Error() string   { return u.err.Error() }
func (u usageErr) Unwrap() []error { return []error{u.err, cmd.ErrUsage} }

func TestExitCode(t *testing.T) {
	// A message-preserving check: every classified error must map to its code
	// AND keep its original text (only the exit code differs, per the scheme).
	cases := []struct {
		name string
		err  error
		want int
		// msg, when non-empty, is the exact message the tagged error must still
		// print — proving classification does not alter user-facing output.
		msg string
	}{
		{"nil is success", nil, exitOK, ""},

		{"usage error", usageErr{errors.New("unknown flag: --nope")}, exitUsage, "unknown flag: --nope"},
		{"usage sentinel", cmd.ErrUsage, exitUsage, ""},
		{"bad request sentinel", civitai.ErrBadRequest, exitUsage, ""},
		{"tagged 400 (real readError shape)", civitai.Tag(civitai.ErrBadRequest,
			fmt.Errorf("invalid request parameter (400): period — Invalid option")), exitUsage,
			"invalid request parameter (400): period — Invalid option"},

		{"unauthorized sentinel", civitai.ErrUnauthorized, exitAuth, ""},
		{"tagged 401 (real readError shape)", civitai.Tag(civitai.ErrUnauthorized,
			fmt.Errorf("unauthorized (401): token expired — run `civitai login`")), exitAuth,
			"unauthorized (401): token expired — run `civitai login`"},
		{"buzz scope", appapi.ErrBuzzScope, exitAuth, ""},
		{"device flow error", &appapi.DeviceFlowError{Code: "access_denied"}, exitAuth, ""},
		{"wrapped device flow error", fmt.Errorf("login: %w", &appapi.DeviceFlowError{Code: "expired_token"}), exitAuth, ""},

		{"not found sentinel", civitai.ErrNotFound, exitNotFound, ""},
		{"tagged 404 (real readError shape)", civitai.Tag(civitai.ErrNotFound,
			fmt.Errorf("not found (404): No model with id 999")), exitNotFound,
			"not found (404): No model with id 999"},

		{"rate limited sentinel", civitai.ErrRateLimited, exitRateLimited, ""},
		{"tagged 429", civitai.Tag(civitai.ErrRateLimited, fmt.Errorf("rate limited (429): slow down")), exitRateLimited, "rate limited (429): slow down"},

		{"network sentinel", civitai.ErrNetwork, exitNetwork, ""},
		{"tagged 503", civitai.Tag(civitai.ErrNetwork, fmt.Errorf("service unavailable (503): retry shortly")), exitNetwork, "service unavailable (503): retry shortly"},
		{"net.Error timeout", fakeNetErr{timeout: true}, exitNetwork, ""},
		{"wrapped dial refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), exitNetwork, ""},
		{"deadline exceeded", fmt.Errorf("request: %w", context.DeadlineExceeded), exitNetwork, ""},

		// 🔴 The generate sentinels must land on GENERIC 1 — deliberately NOT 3
		// (auth) or 2 (usage), because a caller who is out of Buzz, muted, or was
		// given a different model must never be told to re-run `civitai login`.
		// generate.go asserts this in a comment; nothing tested it until an audit
		// found ErrModelSubstituted's exit code unpinned.
		{"insufficient buzz sentinel", cmd.ErrInsufficientBuzz, exitGeneric, ""},
		{"generation disabled sentinel", cmd.ErrGenerationDisabled, exitGeneric, ""},
		{"account restricted sentinel", cmd.ErrAccountRestricted, exitGeneric, ""},
		{"prompt blocked sentinel", cmd.ErrPromptBlocked, exitGeneric, ""},
		{"model substituted sentinel", cmd.ErrModelSubstituted, exitGeneric, ""},
		{"tagged model substitution keeps its message", civitai.Tag(cmd.ErrModelSubstituted,
			errors.New("refusing to submit: the server reports it would run version 2 instead of the version 1 you asked for")),
			exitGeneric,
			"refusing to submit: the server reports it would run version 2 instead of the version 1 you asked for"},

		{"generic fallback", errors.New("something unexpected"), exitGeneric, ""},
		{"wrapped generic", fmt.Errorf("outer: %w", errors.New("inner")), exitGeneric, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCode(tc.err); got != tc.want {
				t.Errorf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
			if tc.msg != "" && tc.err.Error() != tc.msg {
				t.Errorf("message changed by classification: got %q, want %q", tc.err.Error(), tc.msg)
			}
		})
	}
}

// TestExitCodesAreDistinct guards against two kinds accidentally sharing a code.
func TestExitCodesAreDistinct(t *testing.T) {
	codes := map[int]string{
		exitOK: "ok", exitGeneric: "generic", exitUsage: "usage",
		exitAuth: "auth", exitNotFound: "notFound", exitNetwork: "network",
		exitRateLimited: "rateLimited",
	}
	if len(codes) != 7 {
		t.Fatalf("expected 7 distinct exit codes, got %d", len(codes))
	}
}

// TestExitCodeConstantsMatchDocs is the seam guard between the codes this
// binary can RETURN and the codes the CLI PUBLISHES (root `--help` + the
// README table, both rendered from cmd.ExitCodeDocs()). Neither side can move
// alone: the assertion is set EQUALITY, so it fails when a constant is added
// without documentation and when a documented code has no constant.
//
// It replaces the per-constant comments that used to restate the contract here
// and drifted from it — a comment cannot be asserted, a set can.
func TestExitCodeConstantsMatchDocs(t *testing.T) {
	constants := map[int]string{
		exitOK: "exitOK", exitGeneric: "exitGeneric", exitUsage: "exitUsage",
		exitAuth: "exitAuth", exitNotFound: "exitNotFound",
		exitNetwork: "exitNetwork", exitRateLimited: "exitRateLimited",
	}
	docs := cmd.ExitCodeDocs()
	if len(docs) == 0 {
		t.Fatal("cmd.ExitCodeDocs() is empty — the guard would compare against nothing")
	}

	documented := make(map[int]bool, len(docs))
	for _, d := range docs {
		documented[d.Code] = true
		if _, ok := constants[d.Code]; !ok {
			t.Errorf("exit code %d is documented in internal/cmd/exitcodes_doc.go but this binary has no constant for it", d.Code)
		}
	}
	for code, name := range constants {
		if !documented[code] {
			t.Errorf("%s = %d is returned by this binary but documented nowhere — add it to exitCodeDocs (internal/cmd/exitcodes_doc.go)", name, code)
		}
	}
}

// Ensure the interface assertion for net.Error stays valid.
var _ net.Error = fakeNetErr{}
