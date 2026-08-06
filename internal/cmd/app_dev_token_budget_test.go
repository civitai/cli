package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// devTokenBudgetKey decodes rec.rawBody as a generic object and reports the
// `buzzBudget` member and whether the KEY was present at all.
//
// Reading the raw body is the whole point: a `struct{BuzzBudget int}` decodes an
// absent key and an explicit `0` to the same value, so a test written against a
// decoded struct cannot tell "the CLI sent nothing" (server default applies)
// from "the CLI sent 0" (server 400s). Only the serialized bytes distinguish
// them, and that distinction is the flag's central requirement.
func devTokenBudgetKey(t *testing.T, rec *devTokenRec) (any, bool) {
	t.Helper()
	var generic map[string]any
	if err := json.Unmarshal(rec.rawBody, &generic); err != nil {
		t.Fatalf("request body is not a JSON object (%v): %q", err, string(rec.rawBody))
	}
	v, ok := generic["buzzBudget"]
	return v, ok
}

// TestAppDevTokenOmitsBudgetWhenFlagUnset is the no-regression pin, and it is an
// INVARIANT guard rather than a regression test: it passes both before and after
// --budget exists. That is deliberate — the risk the flag introduces is that a
// zero-valued Go variable starts being marshalled on EVERY mint, silently
// pinning every token at a budget of 0 (or 50) and making the server's own
// resolution unreachable. This pins the pre-flag wire shape so that regression
// cannot land unseen.
func TestAppDevTokenOmitsBudgetWhenFlagUnset(t *testing.T) {
	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	chdir(t, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	if _, _, err := run(t, "app", "dev-token", "my-block"); err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if v, present := devTokenBudgetKey(t, &rec); present {
		t.Errorf("body must carry NO buzzBudget key when --budget is unset, got %v (body %q) — "+
			"sending one makes the server's default resolution unreachable", v, string(rec.rawBody))
	}
}

// TestAppDevTokenSendsBudget: --budget N puts an integer buzzBudget on the wire.
func TestAppDevTokenSendsBudget(t *testing.T) {
	cases := []struct {
		name   string
		budget string
		want   float64 // encoding/json decodes every JSON number into float64
	}{
		{"the server cap", "250", 250},
		{"clears the seamless-pano-360 ceiling of 90", "120", 120},
		{"the smallest value the server accepts", "1", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec devTokenRec
			srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
			defer srv.Close()

			chdir(t, t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			if _, _, err := run(t, "app", "dev-token", "my-block", "--budget", tc.budget); err != nil {
				t.Fatalf("app dev-token --budget %s: %v", tc.budget, err)
			}
			v, present := devTokenBudgetKey(t, &rec)
			if !present {
				t.Fatalf("body must carry buzzBudget when --budget is set, got %q", string(rec.rawBody))
			}
			if v != tc.want {
				t.Errorf("buzzBudget = %v (%T), want %v — body %q", v, v, tc.want, string(rec.rawBody))
			}
			// The value must be a JSON NUMBER, not a string: the server's zod
			// schema is z.number().int().positive() and rejects "120" outright.
			if !strings.Contains(string(rec.rawBody), fmt.Sprintf(`"buzzBudget":%s`, tc.budget)) {
				t.Errorf("buzzBudget must serialize as a bare JSON number, got %q", string(rec.rawBody))
			}
		})
	}
}

// TestAppDevTokenRejectsOutOfRangeBudget: a budget outside the server's accepted
// range fails LOCALLY, names the bound, and sends NO request — shipping a call
// you already know the server refuses (or silently clamps) wastes a round trip
// and, in the clamp case, hands back a token whose budget is not what was asked
// for with no signal at all.
func TestAppDevTokenRejectsOutOfRangeBudget(t *testing.T) {
	cases := []struct {
		name   string
		budget string
		want   []string
	}{
		// The server's zod schema is .positive(), so 0 and negatives are a 400.
		{"zero", "0", []string{"--budget", "1", "250"}},
		{"negative", "-5", []string{"--budget", "1", "250"}},
		// Over the cap the server does NOT refuse — it silently clamps to 250,
		// which is the worse failure, so the CLI has to be the one to speak up.
		{"over the cap", "251", []string{"--budget", "250", "clamp"}},
		{"far over the cap", "100000", []string{"--budget", "250", "clamp"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec devTokenRec
			srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
			defer srv.Close()

			chdir(t, t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			out, _, err := run(t, "app", "dev-token", "my-block", "--budget", tc.budget)
			if err == nil {
				t.Fatalf("--budget %s must be rejected client-side", tc.budget)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("--budget %s error %q should mention %q", tc.budget, err.Error(), want)
				}
			}
			// No token may reach stdout, and no request may reach the server.
			if strings.TrimSpace(out) != "" {
				t.Errorf("a rejected --budget must print nothing to stdout, got %q", out)
			}
			if rec.method != "" {
				t.Errorf("a rejected --budget must not send a request, but the server saw %s %s",
					rec.method, rec.path)
			}
			// Usage-class failure -> the CLI's usage exit code, not a network one.
			if !errors.Is(err, ErrUsage) {
				t.Errorf("out-of-range --budget should classify as ErrUsage, got %T: %v", err, err)
			}
		})
	}
}

// TestAppDevTokenSurfacesServerBudgetRejection: when the server DOES refuse a
// budget it answers 400 with {"message","details"}, where the actionable text
// lives in details.fieldErrors.buzzBudget (zod's flatten()). Surfacing only the
// generic top-level "Invalid request body" tells the developer nothing about
// WHICH field was wrong, so the field detail has to make it into the error.
func TestAppDevTokenSurfacesServerBudgetRejection(t *testing.T) {
	body := map[string]any{
		"message": "Invalid request body",
		"details": map[string]any{
			"formErrors": []any{},
			"fieldErrors": map[string]any{
				"buzzBudget": []any{"Too small: expected number to be >0"},
			},
		},
	}
	srv := devTokenServer(t, body, http.StatusBadRequest, nil)
	defer srv.Close()

	chdir(t, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, _, err := run(t, "app", "dev-token", "my-block")
	if err == nil {
		t.Fatal("expected an error for a 400")
	}
	for _, want := range []string{"400", "Invalid request body", "buzzBudget", "Too small"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("400 error %q should surface %q — a bare status code is not actionable", err.Error(), want)
		}
	}
	if !errors.Is(err, civitai.ErrBadRequest) {
		t.Errorf("a 400 should classify as ErrBadRequest, got %T: %v", err, err)
	}
}

// TestAppDevTokenBudgetWithEnvKeepsStreamSplit: --budget must not disturb the
// stdout/stderr contract --env depends on (token on stdout, hints on stderr) —
// `dev-token --env >> .env.development.local` has to stay clean.
func TestAppDevTokenBudgetWithEnvKeepsStreamSplit(t *testing.T) {
	var rec devTokenRec
	srv := devTokenServer(t, map[string]any{"token": "jwt-x"}, http.StatusOK, &rec)
	defer srv.Close()

	chdir(t, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, "app", "dev-token", "my-block", "--budget", "120", "--env")
	if err != nil {
		t.Fatalf("app dev-token --budget 120 --env: %v", err)
	}
	if strings.TrimSpace(out) != "VITE_LIVE_BLOCK_TOKEN=jwt-x" {
		t.Errorf("stdout = %q, want exactly VITE_LIVE_BLOCK_TOKEN=jwt-x", out)
	}
	if !strings.Contains(errOut, "Paste") {
		t.Errorf("stderr should still carry the paste hint: %q", errOut)
	}
}

// TestAppDevTokenBudgetHelpTeachesTheTrap: the two ways an under-set budget
// bites are both invisible from the error the developer sees, so help has to
// name them — a recipe ceiling above the budget REFUSES the generation, and on
// an inline customComfy graph the same number is also the step timeout in
// seconds, so being thrifty shows up as `expired`, never as a billing error.
func TestAppDevTokenBudgetHelpTeachesTheTrap(t *testing.T) {
	out, _, err := run(t, "app", "dev-token", "--help")
	if err != nil {
		t.Fatalf("app dev-token --help: %v", err)
	}
	for _, want := range []string{
		"--budget",
		"ceiling",
		"customComfy",
		"seconds",
		"expired",
		// The two server constants, written as LITERALS on purpose: this test
		// must compile against a tree that does not yet vendor them, so the
		// red-before-green run is a real runtime failure and not a build break.
		// appapi's own drift guard is what pins these to the server.
		"250",
		"50",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("`app dev-token --help` should mention %q:\n%s", want, out)
		}
	}
}
