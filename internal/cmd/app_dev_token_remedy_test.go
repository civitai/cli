package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
)

// spendCapableDevTokenServer answers /api/v1/me with a credential that CAN spend
// and mints whatever token is passed. That combination — "your credential can
// spend, the token still can't" — is the only one that reaches
// readOnlyTokenWarning's canSpend branch, which is the branch civitai/cli#362 is
// about.
func spendCapableDevTokenServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "u", "id": 1,
				"tokenScope": appapi.ScopeAIServicesWrite | appapi.ScopeUserRead,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": token})
	}))
}

// devTokenCommandLines returns every copy-pasteable `civitai app dev-token …`
// line in s.
//
// Structural on purpose: the defect in civitai/cli#362 was not a WORD, it was a
// suggested COMMAND that omits `--spend`. A `Contains(s, "--spend")` guard is
// satisfied by the sentence four lines above it and would stay green with the
// bad command still there.
func devTokenCommandLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "civitai app dev-token ") {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

// TestReadOnlyWarningCanSpendSuggestsOnlySpendingRemints is the direct guard on
// defect 1: in the branch whose whole thesis is "you needed --spend", EVERY
// suggested re-mint must carry --spend. The final line used to be the one
// without it.
func TestReadOnlyWarningCanSpendSuggestsOnlySpendingRemints(t *testing.T) {
	got := readOnlyTokenWarning(ui.For(io.Discard), true, config.AuthKindToken, "my-block", false)

	lines := devTokenCommandLines(got)
	if len(lines) == 0 {
		t.Fatalf("the credential-can-spend branch must still offer a re-mint command; got:\n%s", got)
	}
	for _, l := range lines {
		if !strings.Contains(l, "--spend") {
			t.Errorf("suggested command %q omits --spend — copying it reproduces the read-only token this warning complains about; full warning:\n%s", l, got)
		}
	}
}

// TestReadOnlyWarningCredentialBranchesKeepThePlainRemint is the negative
// control for the guard above: the two branches where the fix is the CREDENTIAL
// genuinely do end with a plain re-mint (change the credential, then re-mint),
// so the fix must not have stripped it everywhere.
func TestReadOnlyWarningCredentialBranchesKeepThePlainRemint(t *testing.T) {
	for _, authKind := range []string{config.AuthKindOAuth, config.AuthKindToken} {
		got := readOnlyTokenWarning(ui.For(io.Discard), false, authKind, "my-block", false)
		if !strings.Contains(got, "civitai app dev-token my-block --env >> .env.development.local") {
			t.Errorf("authKind %q: the credential-fix branch must still name the follow-up re-mint; got:\n%s", authKind, got)
		}
	}
}

// TestReadOnlyWarningDropsSatisfiedManifestAdvice is defect 2: the parenthetical
// "(or add it to `scopes` in block.manifest.json)" is a dead end when the
// manifest ALREADY declares the scope — that declaration is why the CLI filtered
// it out and warned four lines earlier.
func TestReadOnlyWarningDropsSatisfiedManifestAdvice(t *testing.T) {
	declared := readOnlyTokenWarning(ui.For(io.Discard), true, config.AuthKindToken, "my-block", true)
	if strings.Contains(declared, "add it to `scopes`") {
		t.Errorf("manifest already declares the scope — telling the user to add it is a step they have taken; got:\n%s", declared)
	}
	if !strings.Contains(declared, "--spend") {
		t.Errorf("the real remedy (--spend) must survive; got:\n%s", declared)
	}

	// Negative control: with no such declaration the advice is live and must stay.
	undeclared := readOnlyTokenWarning(ui.For(io.Discard), true, config.AuthKindToken, "my-block", false)
	if !strings.Contains(undeclared, "add it to `scopes` in block.manifest.json") {
		t.Errorf("with an undeclaring manifest the scopes route is a real fix and must be offered; got:\n%s", undeclared)
	}
}

// TestDevTokenSaysTheSameThingOnceAroundTheToken is defect 3, end-to-end: the
// manifest declares the scope, --spend is absent and the credential CAN spend,
// so the pre-mint notice and the post-token warning had the identical cause and
// the identical remedy — printed either side of the token on stdout.
func TestDevTokenSaysTheSameThingOnceAroundTheToken(t *testing.T) {
	writeManifestWithScopes(t, `["user:read:self","ai:write:budgeted"]`)
	srv := spendCapableDevTokenServer(t, makeJWT(t, []string{"user:read:self"}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block", "--env")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}

	if n := strings.Count(errOut, "civitai app dev-token my-block --spend --env >> .env.development.local"); n != 1 {
		t.Errorf("the --spend re-mint must be printed exactly once, got %d times:\n%s", n, errOut)
	}
	if !strings.Contains(errOut, "does NOT request budgeted spend") {
		t.Errorf("the surviving notice must still be the pre-mint one that names the manifest; got:\n%s", errOut)
	}
	if strings.Contains(errOut, "READ-ONLY") {
		t.Errorf("the post-token READ-ONLY warning repeats the pre-mint notice's cause and remedy here; got:\n%s", errOut)
	}
	for _, l := range devTokenCommandLines(errOut) {
		if !strings.Contains(l, "--spend") {
			t.Errorf("suggested command %q omits --spend and re-creates this exact state; full stderr:\n%s", l, errOut)
		}
	}
}

// devTokenSuggestedSlugs returns the slug argument of every suggested
// `civitai app dev-token <slug> …` line in s.
//
// Structural, like devTokenCommandLines: the defect below is not a word but a
// suggested command that names the WRONG slug, and a substring check on the
// slug cannot see it (the renamed slug CONTAINS the stale one, "my-block" ⊂
// "my-block-abc12").
func devTokenSuggestedSlugs(s string) []string {
	var out []string
	for _, l := range devTokenCommandLines(s) {
		f := strings.Fields(l)
		for i, w := range f {
			if w == "dev-token" && i+1 < len(f) {
				out = append(out, f[i+1])
			}
		}
	}
	return out
}

// TestDevTokenSuppressionNeverLeavesAStaleSlugCommand is the repair to defect 3's
// suppression: it silences the post-mint warning, which is the ONLY line printed
// after a rename — so on a rename the surviving advice named the PRE-rename slug,
// which block.manifest.json no longer contains. Copying it re-collides and mints
// yet another suffixed slug, i.e. the remedy does not leave the state it
// complains about — the thing readOnlyTokenWarning's own 🔴 comment forbids.
//
// The assertion is the RELATIONSHIP (every suggested slug is the one on disk),
// not a spelling, so it holds however the rename suffix comes out.
func TestDevTokenSuppressionNeverLeavesAStaleSlugCommand(t *testing.T) {
	prev := devTokenSuffixGen
	devTokenSuffixGen = func() string { return "abc12" }
	defer func() { devTokenSuffixGen = prev }()

	// The credential CAN spend (so the suppression's other condition holds) and
	// the first mint collides, forcing mintDevTokenWithRename.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"username": "u", "id": 1,
				"tokenScope": appapi.ScopeAIServicesWrite | appapi.ScopeUserRead,
			})
			return
		}
		var body struct {
			Slug string `json:"slug"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if body.Slug == "my-block" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "App not found"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"token": makeJWT(t, []string{"user:read:self"})})
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeDevTokenManifest(t, dir, `{
  "blockId": "my-block",
  "version": "0.1.0",
  "name": "My Block",
  "scopes": ["ai:write:budgeted"]
}`)
	chdir(t, dir)

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block", "--env")
	if err != nil {
		t.Fatalf("app dev-token: %v\n%s", err, errOut)
	}

	onDisk := readBlockID(t, dir)
	if onDisk == "my-block" {
		t.Fatalf("the mint must have renamed the slug for this test to mean anything; manifest still says %q", onDisk)
	}
	suggested := devTokenSuggestedSlugs(errOut)
	// Positive control: a run that suggests NOTHING would satisfy the loop below.
	if len(suggested) == 0 {
		t.Fatalf("no `civitai app dev-token …` command was suggested at all; stderr:\n%s", errOut)
	}
	for _, slug := range suggested {
		if slug != onDisk {
			t.Errorf("suggested command names slug %q but block.manifest.json now says %q — copying it re-collides and mints ANOTHER renamed slug; stderr:\n%s", slug, onDisk, errOut)
		}
	}
}

// TestDevTokenReadOnlyWarningSurvivesWhenTheCauseDiffers is the negative control
// for the suppression: when the credential CANNOT spend, the post-token warning
// names a cause the pre-mint notice never mentions, so suppressing it would lose
// the only line that says "fix your credential".
func TestDevTokenReadOnlyWarningSurvivesWhenTheCauseDiffers(t *testing.T) {
	writeManifestWithScopes(t, `["user:read:self","ai:write:budgeted"]`)
	// Default devTokenServer's /me reports tokenScope 0 → the credential cannot spend.
	srv := devTokenServer(t, map[string]any{"token": makeJWT(t, []string{"user:read:self"})}, http.StatusOK, nil)
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	_, errOut, err := run(t, "app", "dev-token", "my-block", "--env")
	if err != nil {
		t.Fatalf("app dev-token: %v", err)
	}
	if !strings.Contains(errOut, "READ-ONLY") {
		t.Errorf("a credential that cannot spend is a DIFFERENT cause and must still be reported; got:\n%s", errOut)
	}
}
