package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// THE LISTING GATE'S SCOPE-403 WARNING (the first GitHub Actions OIDC submit,
// CLI v0.1.145, with a personal API key scoped to UserRead).
//
// That submit printed the generic fail-open line with the listing client's
// INTERACTIVE remedy inside it — "re-run `civitai login` (an old token may
// predate the scope) …" — from a CI job that cannot log in, about a credential
// that can never carry the scope anyway. These tests drive the real command
// against a fake server whose listing read answers the server's real scope
// refusal, and pin the whole warning literally, in both arms.
//
// Both expected strings are written out by hand rather than built from the
// helper that produces them, so a reword is a visible test change.

// scopeRefusalMessage is the server's words for a token-scope refusal
// (civitai/civitai enforceTokenScope), verbatim.
const scopeRefusalMessage = "Your API key does not have the required scope for this action"

// scopeGateServer is gateServer with the listing read refused for SCOPE. The
// rest of the routes answer as gateServer's do, so the submit can proceed.
func scopeGateServer(t *testing.T, message string) (*httptest.Server, *bool, *int) {
	t.Helper()
	submitted := false
	listMineCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
		case strings.HasPrefix(r.URL.Path, listMineTestPath):
			listMineCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"` + message + `","code":-32603}}}`))
		default:
			submitted = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishRequestId": "pubreq_gate", "slug": gateSlug, "version": "0.1.0", "status": "pending",
			})
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &submitted, &listMineCalls
}

// clearUnattendedSignals empties every environment signal submitIsUnattended
// reads, so the test's own CI runner (which sets CI and GITHUB_ACTIONS) cannot
// decide the arm.
func clearUnattendedSignals(t *testing.T) {
	t.Helper()
	for _, k := range ciEnvVars {
		t.Setenv(k, "")
	}
	t.Setenv(GitHubOIDCTokenEnv, "")
}

const wantUnattendedScopeWarning = "skipped the store-listing check for gate-subject-app: this credential " +
	"cannot read listing media (403, it lacks the Apps submit scope). A personal API key never carries " +
	"that scope and passes only with Full Access, so with a scoped key this check is skipped on every " +
	"unattended submit. Submitting without the listing-completeness gate. To check the listing, run " +
	"`civitai app doctor gate-subject-app` from a `civitai login` session, or open the listing in your browser."

const wantAttendedScopeWarning = "skipped the store-listing check for gate-subject-app: this credential " +
	"cannot read listing media (403, it lacks the Apps submit scope). A personal API key never carries " +
	"that scope and passes only with Full Access; a `civitai login` token carries it (re-run `civitai login` " +
	"if yours predates the scope). Submitting without the listing-completeness gate."

// unwrapStderr joins the warning back onto one line: the ui layer may wrap a
// long warning for the terminal, and the assertion is about the words.
func unwrapStderr(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestSubmitListingScopeRefusalInCINamesTheCauseAndAnUnattendedCheck(t *testing.T) {
	withStdinTTY(t, false)
	clearUnattendedSignals(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	srv, submitted, listMineCalls := scopeGateServer(t, scopeRefusalMessage)
	withGateEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("a scope-refused listing read must not block a submit; got %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout, stderr)
	}
	if *listMineCalls == 0 {
		t.Fatal("CONTROL failure: the listing read was never attempted")
	}
	if !*submitted {
		t.Error("the submit route was not reached: the warning must not change whether the submit proceeds")
	}
	got := unwrapStderr(stderr)
	if !strings.Contains(got, wantUnattendedScopeWarning) {
		t.Errorf("the CI scope warning is not the expected sentence.\nwant (substring):\n  %s\nstderr:\n%s",
			wantUnattendedScopeWarning, stderr)
	}
	if strings.Contains(got, "re-run `civitai login`") {
		t.Errorf("an unattended submit was told to re-run `civitai login`, which a CI job cannot do:\n%s", stderr)
	}
	if strings.Contains(got, "could not check") {
		t.Errorf("the scope refusal fell to the generic fail-open line:\n%s", stderr)
	}
}

func TestSubmitListingScopeRefusalAtATerminalKeepsTheLoginRemedy(t *testing.T) {
	withStdinTTY(t, true)
	clearUnattendedSignals(t)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	srv, submitted, _ := scopeGateServer(t, scopeRefusalMessage)
	withGateEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("a scope-refused listing read must not block a submit; got %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout, stderr)
	}
	if !*submitted {
		t.Error("the submit route was not reached")
	}
	got := unwrapStderr(stderr)
	if !strings.Contains(got, wantAttendedScopeWarning) {
		t.Errorf("the attended scope warning is not the expected sentence.\nwant (substring):\n  %s\nstderr:\n%s",
			wantAttendedScopeWarning, stderr)
	}
	if strings.Contains(got, "civitai app doctor") {
		t.Errorf("the attended arm carries the unattended remedy:\n%s", stderr)
	}
}

// TestSubmitListingNonScope403KeepsTheGenericLine is an INVARIANT guard, not a
// regression test: a 403 that is NOT a scope refusal (here an access refusal)
// must keep the generic fail-open line and its own words, so the scope branch is
// shown to be selected by the error's KIND and not by "any 403".
func TestSubmitListingNonScope403KeepsTheGenericLine(t *testing.T) {
	withStdinTTY(t, false)
	clearUnattendedSignals(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	srv, submitted, _ := scopeGateServer(t, "You are not an Apps author")
	withGateEnv(t, srv)

	_, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("a refused listing read must not block a submit; got %v\nstderr:\n%s", err, stderr)
	}
	if !*submitted {
		t.Error("the submit route was not reached")
	}
	got := unwrapStderr(stderr)
	if !strings.Contains(got, "could not check gate-subject-app's store listing (") ||
		!strings.Contains(got, "You are not an Apps author") {
		t.Errorf("a non-scope 403 lost the generic fail-open line or the server's words:\n%s", stderr)
	}
	if strings.Contains(got, "skipped the store-listing check") {
		t.Errorf("a non-scope 403 took the scope branch:\n%s", stderr)
	}
}
