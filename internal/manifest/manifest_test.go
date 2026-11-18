package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	cli "github.com/civitai/cli"
)

func write(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPath(t *testing.T) {
	if got := Path("foo"); got != filepath.Join("foo", Filename) {
		t.Errorf("Path = %q", got)
	}
}

func TestLoadReadsFields(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{
		"blockId": "demo",
		"version": "1.2.3",
		"name": "Demo",
		"buildCommand": "npm run build",
		"outputDir": "dist"
	}`)
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.BlockID != "demo" || m.Version != "1.2.3" || m.Name != "Demo" {
		t.Errorf("unexpected manifest: %+v", m)
	}
	if m.BuildCommand != "npm run build" || m.OutputDir != "dist" {
		t.Errorf("build fields wrong: %+v", m)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	// 🔴 `app create`, NOT `app init`. Both run the same scaffolder, but they
	// default to different templates and `civitai app --help` calls `init` "a
	// back-compat alias" — so a reader who has NO app is sent to `create`, the
	// command `civitai --help`'s "Get started" block also names. The set is held
	// to one across every such surface by
	// internal/cmd.TestEveryNewAppRemedyNamesOneScaffolder.
	if !contains(err.Error(), "civitai app create") {
		t.Errorf("error should name the scaffolder a reader with no app should run: %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{not json`)
	if _, err := Load(dir); err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestLoadRaw(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"blockId":"x","version":"0.1.0","extra":true}`)
	generic, m, err := LoadRaw(dir)
	if err != nil {
		t.Fatalf("LoadRaw: %v", err)
	}
	if m.BlockID != "x" {
		t.Errorf("struct blockId = %q", m.BlockID)
	}
	gm, ok := generic.(map[string]any)
	if !ok {
		t.Fatalf("generic is %T, want map", generic)
	}
	if gm["extra"] != true {
		t.Errorf("generic should preserve unknown fields: %v", gm)
	}
}

func TestLoadRawMissingAndInvalid(t *testing.T) {
	if _, _, err := LoadRaw(t.TempDir()); err == nil {
		t.Error("expected error for missing manifest")
	}
	dir := t.TempDir()
	write(t, dir, `nope`)
	if _, _, err := LoadRaw(dir); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadScopes(t *testing.T) {
	// Present scopes are returned.
	dir := t.TempDir()
	write(t, dir, `{"blockId":"x","scopes":["ai:write:budgeted","identity:read"]}`)
	got := LoadScopes(dir)
	if len(got) != 2 || got[0] != "ai:write:budgeted" || got[1] != "identity:read" {
		t.Errorf("LoadScopes = %v", got)
	}

	// No scopes field → nil.
	dir2 := t.TempDir()
	write(t, dir2, `{"blockId":"x"}`)
	if got := LoadScopes(dir2); got != nil {
		t.Errorf("LoadScopes (no scopes) = %v, want nil", got)
	}

	// Missing manifest → nil, no error (graceful degrade).
	if got := LoadScopes(t.TempDir()); got != nil {
		t.Errorf("LoadScopes (missing) = %v, want nil", got)
	}

	// Malformed JSON → nil, no error (graceful degrade).
	dir3 := t.TempDir()
	write(t, dir3, `{ not json `)
	if got := LoadScopes(dir3); got != nil {
		t.Errorf("LoadScopes (malformed) = %v, want nil", got)
	}
}

// TestLoadAuth covers EVERY value the vendored schema admits, not just the one
// this field was added for: a suite that asserts only `oauth` lets a mutant
// dropping `block-token` from the accepted set pass, which silently deletes that
// app's `Declaring auth:` transparency line. It also pins the second return,
// which is the whole difference between "declared nothing" (say nothing) and
// "declared something we dropped" (warn).
func TestLoadAuth(t *testing.T) {
	cases := []struct {
		name           string
		manifest       string // "" = write no manifest at all
		wantAuth       string
		wantUnrecognzd bool
	}{
		{name: "oauth", manifest: `{"blockId":"x","auth":"oauth"}`, wantAuth: "oauth"},
		{name: "block-token", manifest: `{"blockId":"x","auth":"block-token"}`, wantAuth: "block-token"},
		{name: "unknown value warns", manifest: `{"blockId":"x","auth":"basic"}`, wantUnrecognzd: true},
		{name: "case mismatch is unknown", manifest: `{"blockId":"x","auth":"OAuth"}`, wantUnrecognzd: true},
		{name: "empty string is silence", manifest: `{"blockId":"x","auth":""}`},
		{name: "no auth key is silence", manifest: `{"blockId":"x"}`},
		{name: "malformed json is silence", manifest: `{"blockId":`},
		{name: "missing manifest is silence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.manifest != "" {
				write(t, dir, tc.manifest)
			}
			auth, unrecognized := LoadAuth(dir)
			if auth != tc.wantAuth {
				t.Errorf("LoadAuth auth = %q, want %q", auth, tc.wantAuth)
			}
			if unrecognized != tc.wantUnrecognzd {
				t.Errorf("LoadAuth unrecognized = %v, want %v", unrecognized, tc.wantUnrecognzd)
			}
		})
	}
}

// TestAuthKindsComeFromTheVendoredSchema pins the SINGLE-SOURCING: AuthKinds()
// must return exactly what the embedded schema's `auth` enum declares, in order.
//
// 🔴 THE `want` LIST IS DERIVED, AND IT USED TO BE HAND-TYPED — which made this a
// SELF-BLOCK on the most volatile enum in the schema. It read
// `want := []string{"block-token", "oauth"}`, so the canonical adding an auth kind
// reddened it, which reds `go test ./...`, which is what
// `revendor-canonical-schema.yml` runs BEFORE opening its resync PR. So the bot
// would have opened no PR, the vendored mirror would have stayed stale, and
// `schema-drift` would have gone red until a human hand-edited this list. `auth`
// landed 2026-09-24 and its description was already revised once by 2026-09-26; it
// is exactly the field where that was most likely to fire.
//
// The comment here used to say "A RED HERE IS NOT A BUG, IT IS THE SIGNAL THIS TEST
// EXISTS FOR", asking the reader to update this list AND the README's "What the
// tunnel declares" paragraph. The documentation intent was right; gating the
// re-vendor bot's own suite on it was not, because the cost of the signal is paid by
// every unrelated PR in the repo and the bot cannot deliver it to anyone.
//
// ⚠ WHAT IS NO LONGER ENFORCED, so it is not silently lost: nothing now reds when a
// new auth kind lands and the README's prose does not mention it. `LoadAuth` forwards
// the new kind correctly either way — this was always a docs-freshness signal, never
// a correctness one. If it is worth enforcing, it must live OUTSIDE `go test` (a
// scheduled job, or a line in the re-vendor PR's body), for the reason above.
func TestAuthKindsComeFromTheVendoredSchema(t *testing.T) {
	// The schema, read independently of the production derivation. This is the
	// authority; AuthKinds() is the thing under test.
	var doc struct {
		Properties struct {
			Auth struct {
				Enum []string `json:"enum"`
			} `json:"auth"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		t.Fatalf("the embedded schema must parse: %v", err)
	}

	want := append([]string(nil), doc.Properties.Auth.Enum...)
	sort.Strings(want) // AuthKinds() sorts; compare like for like

	// 🔴 POSITIVE CONTROL FIRST. Deriving both sides makes the comparison vacuous if
	// the enum is missing or empty — `[] == []` agrees and the test passes while
	// LoadAuth admits nothing and the tunnel silently stops declaring auth. That is
	// the failure this guard exists for, so the floor is asserted before the match.
	if len(want) < 2 {
		t.Fatalf("schema /properties/auth/enum = %v (%d entries) — want at least 2. An empty or "+
			"missing enum makes this test vacuous AND makes LoadAuth reject everything, so the "+
			"tunnel stops declaring auth with nothing reporting it.", want, len(want))
	}

	got := AuthKinds()
	if !slices.Equal(got, want) {
		t.Errorf("AuthKinds() = %v, want %v (the schema's own enum, sorted).\n"+
			"AuthKinds() must be single-sourced from the embedded schema — a difference here means "+
			"the derivation broke or a literal crept back in.", got, want)
	}

	// And every kind the schema admits must actually be accepted by LoadAuth, or the
	// derivation is right while the behaviour is not.
	for _, kind := range want {
		dir := t.TempDir()
		write(t, dir, `{"blockId":"x","auth":"`+kind+`"}`)
		auth, unrecognised := LoadAuth(dir)
		if auth != kind || unrecognised {
			t.Errorf("LoadAuth with auth=%q = (%q, unrecognised=%v), want (%q, false) — the schema "+
				"admits this kind but LoadAuth does not forward it.", kind, auth, unrecognised, kind)
		}
	}
}

func TestSetBlockIDPreservesOrderAndFields(t *testing.T) {
	dir := t.TempDir()
	src := `{
  "blockId": "my-block",
  "version": "0.1.0",
  "name": "My Block",
  "scopes": ["identity:read"]
}`
	write(t, dir, src)
	if err := SetBlockID(dir, "my-block-abc12"); err != nil {
		t.Fatalf("SetBlockID: %v", err)
	}
	raw, _ := os.ReadFile(Path(dir))
	out := string(raw)
	if !contains(out, `"blockId": "my-block-abc12"`) {
		t.Errorf("blockId not updated:\n%s", out)
	}
	// Other fields + their order preserved (surgical value replace).
	if !contains(out, `"name": "My Block"`) || !contains(out, `"identity:read"`) {
		t.Errorf("other fields not preserved:\n%s", out)
	}
	if indexOf(out, "blockId") >= indexOf(out, "version") {
		t.Errorf("field order not preserved:\n%s", out)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.BlockID != "my-block-abc12" {
		t.Errorf("reloaded blockId = %q", m.BlockID)
	}
	// Mode preserved at 0600.
	info, _ := os.Stat(Path(dir))
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestSetBlockIDMissingKeyFallsBack(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{"version":"0.1.0","name":"X"}`)
	if err := SetBlockID(dir, "new-slug"); err != nil {
		t.Fatalf("SetBlockID (no key): %v", err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.BlockID != "new-slug" {
		t.Errorf("blockId = %q, want new-slug", m.BlockID)
	}
	if m.Name != "X" {
		t.Errorf("other fields lost: name = %q", m.Name)
	}
}

func TestSetBlockIDMissingFileErrors(t *testing.T) {
	if err := SetBlockID(t.TempDir(), "x-slug"); err == nil {
		t.Error("expected an error when no manifest exists")
	}
}

// TestSetBlockIDRefusesInvalidResultDoesNotClobber: a blockId key inside otherwise
// malformed JSON is value-replaced, but the result fails the json.Valid guard, so
// SetBlockID errors WITHOUT writing (the original file is left intact).
func TestSetBlockIDRefusesInvalidResultDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	broken := `{ "blockId": "old-slug" this is not valid json`
	write(t, dir, broken)
	if err := SetBlockID(dir, "new-slug"); err == nil {
		t.Fatal("expected an error when the rewrite would produce invalid JSON")
	}
	raw, _ := os.ReadFile(Path(dir))
	if string(raw) != broken {
		t.Errorf("file must be untouched on a refused write:\n%s", raw)
	}
}

// TestSetBlockIDInvalidJSONNoKeyErrors: with no blockId key AND invalid JSON, the
// fallback structural rewrite fails to parse and returns an error (no write).
func TestSetBlockIDInvalidJSONNoKeyErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `{ not valid and no key `)
	if err := SetBlockID(dir, "x-slug"); err == nil {
		t.Error("expected an error rewriting invalid JSON with no blockId key")
	}
}

// TestLoadPageBuzzBudget: `civitai app dev-token` compares this figure against
// the budget the mint granted, so the PRESENT/ABSENT bit matters as much as the
// value — a declared 0 and a declared nothing have different remedies, and
// collapsing them would make the command warn about a gap nobody declared.
func TestLoadPageBuzzBudget(t *testing.T) {
	cases := []struct {
		name        string
		body        string // "" = write no manifest at all
		wantVal     int
		wantPresent bool
	}{
		{
			name:        "the page-money scaffold's own declaration",
			body:        `{"blockId":"x","page":{"path":"/","buzzBudgetPerGen":300}}`,
			wantVal:     300,
			wantPresent: true,
		},
		{
			// 🔴 Distinguishable from absent, on purpose.
			name:        "explicit zero is a declaration",
			body:        `{"blockId":"x","page":{"buzzBudgetPerGen":0}}`,
			wantVal:     0,
			wantPresent: true,
		},
		{
			name:        "negative is returned as declared — the caller judges it",
			body:        `{"blockId":"x","page":{"buzzBudgetPerGen":-5}}`,
			wantVal:     -5,
			wantPresent: true,
		},
		{"page object without the member", `{"blockId":"x","page":{"path":"/"}}`, 0, false},
		{"no page object", `{"blockId":"x"}`, 0, false},
		{"malformed json degrades, never errors", `{ not json`, 0, false},
		{"wrong type degrades", `{"blockId":"x","page":{"buzzBudgetPerGen":"300"}}`, 0, false},
		{"missing manifest degrades", "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.body != "" {
				write(t, dir, tc.body)
			}
			got, present := LoadPageBuzzBudget(dir)
			if present != tc.wantPresent {
				t.Fatalf("present = %v, want %v", present, tc.wantPresent)
			}
			if got != tc.wantVal {
				t.Errorf("value = %d, want %d", got, tc.wantVal)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
