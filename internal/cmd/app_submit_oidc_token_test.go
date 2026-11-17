package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// `civitai app submit` and CIVITAI_GITHUB_OIDC_TOKEN, end to end over a real
// git repository and a recording server.
//
// What must hold: the token reaches the server as `githubOidcToken` when the
// variable is set and not otherwise; it is NEVER printed, on success or when the
// server refuses it; and the server's refusal reaches the user.

const oidcFixtureToken = "eyJhbGciOiJSUzI1NiJ9.eyJyZXBvIjoiaGFyYm9yLXRvb2xzL2tub3QifQ.c2lnLWNsaS04ODI3"

type oidcSubmitServer struct {
	body map[string]json.RawMessage
}

// newOIDCSubmitServer answers the submit with status and reply; every other
// route (the version guard's listing read) gets an empty list.
func newOIDCSubmitServer(t *testing.T, status int, reply any) (*oidcSubmitServer, *httptest.Server) {
	t.Helper()
	rec := &oidcSubmitServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "submit-version") {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r.Body)
			if err := json.Unmarshal(buf.Bytes(), &rec.body); err != nil {
				t.Errorf("decode submit body: %v", err)
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(reply)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
	}))
	return rec, srv
}

func runSubmitWithOIDC(t *testing.T, dir, srvURL, token string) (string, string, error) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-oidc")
	t.Setenv("CIVITAI_BASE_URL", srvURL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "")
	t.Setenv(GitHubOIDCTokenEnv, token)
	return run(t, "app", "submit", dir, "--yes")
}

var acceptedReply = map[string]any{
	"publishRequestId": "pubreq_oidc", "slug": dirtySlug, "version": "0.6.1", "status": "pending",
	"actionsRunSourceRecorded": true,
}

func TestAppSubmitSendsTheOIDCTokenFromTheEnvironment(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.publishHEAD()
	rec, srv := newOIDCSubmitServer(t, http.StatusOK, acceptedReply)
	defer srv.Close()

	// Surrounding whitespace (a trailing newline from `echo` into $GITHUB_ENV) is trimmed.
	stdout, stderr, err := runSubmitWithOIDC(t, f.root, srv.URL, oidcFixtureToken+"\n")
	if err != nil {
		t.Fatalf("submit: %v\n%s\n%s", err, stdout, stderr)
	}
	if got := string(rec.body["githubOidcToken"]); got != `"`+oidcFixtureToken+`"` {
		t.Errorf("the server received githubOidcToken=%s, want %q", got, oidcFixtureToken)
	}
	if !strings.Contains(stderr, "CIVITAI_GITHUB_OIDC_TOKEN") {
		t.Errorf("stderr does not say a token is being sent:\n%s", stderr)
	}
	if !strings.Contains(stdout, "GitHub Actions run verified and recorded with this submission.") {
		t.Errorf("stdout does not report the recorded run:\n%s", stdout)
	}
	for _, s := range []string{stdout, stderr} {
		if strings.Contains(s, oidcFixtureToken) || strings.Contains(s, "c2lnLWNsaS04ODI3") {
			t.Fatalf("the OIDC token was printed:\n%s", s)
		}
	}
}

func TestAppSubmitWithoutTheVariableSendsNoToken(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.publishHEAD()
	rec, srv := newOIDCSubmitServer(t, http.StatusOK, map[string]any{
		"publishRequestId": "pubreq_plain", "slug": dirtySlug, "version": "0.6.1", "status": "pending",
	})
	defer srv.Close()

	stdout, stderr, err := runSubmitWithOIDC(t, f.root, srv.URL, "  ")
	if err != nil {
		t.Fatalf("submit: %v\n%s\n%s", err, stdout, stderr)
	}
	if rec.body == nil {
		t.Fatal("CONTROL failure: no submit body was recorded")
	}
	if raw, ok := rec.body["githubOidcToken"]; ok {
		t.Errorf("a blank variable sent githubOidcToken=%s; it must send no key", raw)
	}
	if strings.Contains(stdout+stderr, "GitHub Actions") {
		t.Errorf("a submit without a token mentions GitHub Actions:\n%s\n%s", stdout, stderr)
	}
}

func TestAppSubmitReportsTheServersTokenRefusalWithoutPrintingTheToken(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.publishHEAD()
	_, srv := newOIDCSubmitServer(t, http.StatusBadRequest, map[string]any{
		"message": "GitHub Actions OIDC token rejected: tag v0.6.2 does not match the manifest version 0.6.1 (expected v0.6.1 or 0.6.1)",
		"code":    "github_oidc_token_rejected",
		"reason":  "tag-version-mismatch",
	})
	defer srv.Close()

	stdout, stderr, err := runSubmitWithOIDC(t, f.root, srv.URL, oidcFixtureToken)
	if err == nil {
		t.Fatalf("a refused token must fail the submit\n%s\n%s", stdout, stderr)
	}
	all := stdout + stderr + err.Error()
	if !strings.Contains(all, "tag v0.6.2 does not match the manifest version 0.6.1") {
		t.Errorf("the server's reason did not reach the user:\n%s", all)
	}
	if strings.Contains(all, oidcFixtureToken) || strings.Contains(all, "c2lnLWNsaS04ODI3") {
		t.Fatalf("the OIDC token was printed on a refusal:\n%s", all)
	}
}
