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

// Ensure the interface assertion for net.Error stays valid.
var _ net.Error = fakeNetErr{}
