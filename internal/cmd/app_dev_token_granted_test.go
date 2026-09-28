package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/appapi"
)

// makeJWTClaims builds a syntactically-valid JWT carrying an arbitrary payload,
// so a test can state a buzzBudget / exp that makeJWT (scopes only) cannot. The
// signature segment is the literal "sig" — nothing here is a credential, and
// decodeDevTokenClaims never verifies one.
func makeJWTClaims(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}

// writeManifestWithBudget writes a block.manifest.json carrying
// page.buzzBudgetPerGen (and the scaffold's scopes) into a temp dir, then chdirs
// there — dev-token reads the CWD.
func writeManifestWithBudget(t *testing.T, pageJSON string) {
	t.Helper()
	dir := t.TempDir()
	body := `{"blockId":"my-block","version":"1.0.0","name":"My Block","scopes":["ai:write:budgeted"]`
	if pageJSON != "" {
		body += `,"page":` + pageJSON
	}
	body += "}"
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

// devTokenStderr runs `app dev-token` against a mint returning jwt and returns
// (stdout, stderr).
func devTokenStderr(t *testing.T, jwt string, args ...string) (string, string) {
	t.Helper()
	srv := devTokenServer(t, map[string]any{"token": jwt}, http.StatusOK, nil)
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	out, errOut, err := run(t, append([]string{"app", "dev-token", "my-block"}, args...)...)
	if err != nil {
		t.Fatalf("app dev-token: %v (stderr: %s)", err, errOut)
	}
	return out, errOut
}

// TestAppDevTokenPrintsGrantedClaims is the end-to-end reason this exists: a mint
// must state what it granted, on stderr, without being asked. The measured
// failure was a developer hand-decoding the JWT payload with a base64 pipeline to
// discover a budget of 50, then burning two more mints.
func TestAppDevTokenPrintsGrantedClaims(t *testing.T) {
	writeManifestWithBudget(t, "")
	jwt := makeJWTClaims(t, map[string]any{
		"scopes":     []string{"ai:write:budgeted", "user:read:self"},
		"buzzBudget": 50,
		"exp":        time.Now().Add(4 * time.Hour).Unix(),
	})
	out, errOut := devTokenStderr(t, jwt, "--spend")

	if strings.TrimSpace(out) != jwt {
		t.Errorf("stdout must be exactly the token (it is piped into .env), got %q", out)
	}
	for _, want := range []string{"Granted:", "ai:write:budgeted", "user:read:self", "buzz budget 50/generation", "expires "} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q; got:\n%s", want, errOut)
		}
	}
}

// 🔴 TestAppDevTokenNeverPrintsTokenMaterialOnStderr is the constraint that
// outranks the feature. `civitai/cli` is PUBLIC and this output is pasted into
// issues: the claims may be echoed, the token may not.
//
// It is deliberately MECHANICAL rather than a check for the whole token string —
// a future notice that printed a truncated token, a "…3f9a" tail or a digest
// would pass an equality check and fail this. Every 8-character window of every
// JWT segment must be absent from stderr.
func TestAppDevTokenNeverPrintsTokenMaterialOnStderr(t *testing.T) {
	writeManifestWithBudget(t, `{"buzzBudgetPerGen":300}`)
	jwt := makeJWTClaims(t, map[string]any{
		"scopes":     []string{"ai:write:budgeted"},
		"buzzBudget": 50,
		"exp":        time.Now().Add(4 * time.Hour).Unix(),
		"sub":        "user:4242",
	})
	_, errOut := devTokenStderr(t, jwt, "--spend")

	// Positive control: prove the window scan CAN fire, using a string that IS in
	// stderr. Without this, a scan wired to the wrong text reports a clean zero.
	if !leaksWindow(t, "Granted: scopes", errOut) {
		t.Fatal("the leak scan cannot see a string that IS present — it is wired to nothing")
	}

	for i, seg := range strings.Split(jwt, ".") {
		if leaksWindow(t, seg, errOut) {
			t.Errorf("JWT segment %d leaked into stderr:\n%s", i, errOut)
		}
	}
	// The identity claim is not ours to print either.
	if strings.Contains(errOut, "user:4242") {
		t.Errorf("the token's `sub` must never be echoed; got:\n%s", errOut)
	}
}

// leaksWindow reports whether any 8-character window of needle appears in hay.
func leaksWindow(t *testing.T, needle, hay string) bool {
	t.Helper()
	const w = 8
	if len(needle) < w {
		return strings.Contains(hay, needle)
	}
	for i := 0; i+w <= len(needle); i++ {
		if strings.Contains(hay, needle[i:i+w]) {
			return true
		}
	}
	return false
}

// 🔴 TestAppDevTokenUndecodableTokenStillMints is the NEGATIVE CONTROL for the
// whole feature: a cosmetic read of the token must never be able to break a mint
// that already succeeded. If decoding becomes fatal, this goes red while every
// happy-path test above stays green.
func TestAppDevTokenUndecodableTokenStillMints(t *testing.T) {
	writeManifestWithBudget(t, `{"buzzBudgetPerGen":300}`)
	// Not a JWT at all — the shape a token-format change would produce.
	const opaque = "opaque-token-not-a-jwt"
	out, errOut := devTokenStderr(t, opaque, "--spend")

	if strings.TrimSpace(out) != opaque {
		t.Errorf("the token must still reach stdout verbatim, got %q", out)
	}
	if !strings.Contains(errOut, "VITE_LIVE_BLOCK_TOKEN") {
		t.Errorf("the paste hint must still print; got:\n%s", errOut)
	}
	// No claims line, and no budget warning derived from claims that do not exist.
	if strings.Contains(errOut, "Granted:") {
		t.Errorf("must not print a claims line for a token it could not decode; got:\n%s", errOut)
	}
	if strings.Contains(errOut, "page.buzzBudgetPerGen") {
		t.Errorf("must not warn about a budget gap it cannot measure; got:\n%s", errOut)
	}
}

// TestAppDevTokenWarnsOnBudgetShortfall: the scaffold declares 300, the default
// mint grants 50, and nothing used to say so — the developer found out from
// "insufficient buzz budget: recipe ceiling 90 exceeds budget 50" later.
func TestAppDevTokenWarnsOnBudgetShortfall(t *testing.T) {
	writeManifestWithBudget(t, `{"buzzBudgetPerGen":300}`)
	jwt := makeJWTClaims(t, map[string]any{
		"scopes":     []string{"ai:write:budgeted"},
		"buzzBudget": 50,
		"exp":        time.Now().Add(4 * time.Hour).Unix(),
	})
	_, errOut := devTokenStderr(t, jwt, "--spend")

	for _, want := range []string{
		"page.buzzBudgetPerGen 300",
		"granted 50/generation",
		fmt.Sprintf("--budget %d", appapi.DevBuzzBudgetCap),
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr missing %q; got:\n%s", want, errOut)
		}
	}
}

// The mirror image, and the half that keeps the warning worth reading: no gap, no
// warning. A notice that fires on every mint is one nobody reads.
func TestAppDevTokenSilentWhenBudgetIsEnough(t *testing.T) {
	cases := []struct {
		name string
		page string
		jwt  map[string]any
	}{
		{
			name: "granted equals declared",
			page: `{"buzzBudgetPerGen":50}`,
			jwt:  map[string]any{"scopes": []string{"ai:write:budgeted"}, "buzzBudget": 50},
		},
		{
			name: "granted exceeds declared",
			page: `{"buzzBudgetPerGen":50}`,
			jwt:  map[string]any{"scopes": []string{"ai:write:budgeted"}, "buzzBudget": 250},
		},
		{
			name: "manifest declares no budget — nothing to compare",
			page: "",
			jwt:  map[string]any{"scopes": []string{"ai:write:budgeted"}, "buzzBudget": 50},
		},
		{
			name: "no page object at all",
			page: `{"path":"/"}`,
			jwt:  map[string]any{"scopes": []string{"ai:write:budgeted"}, "buzzBudget": 50},
		},
		{
			// The token states no budget, so the gap is unmeasurable. Guessing the
			// server default here would warn on a token that may well be fine.
			name: "token states no budget",
			page: `{"buzzBudgetPerGen":300}`,
			jwt:  map[string]any{"scopes": []string{"ai:write:budgeted"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeManifestWithBudget(t, tc.page)
			_, errOut := devTokenStderr(t, makeJWTClaims(t, tc.jwt), "--spend")
			if strings.Contains(errOut, "page.buzzBudgetPerGen") {
				t.Errorf("must not warn when there is no measurable shortfall; got:\n%s", errOut)
			}
		})
	}
}
