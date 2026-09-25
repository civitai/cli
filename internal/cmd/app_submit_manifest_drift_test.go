package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/validate"
)

// ---------------------------------------------------------------------------
// THE MANIFEST-DRIFT WARNING
//
// The hazard: the website edits the manifest (its form commits through
// blocks.updateManifest), the author submits from a checkout that predates the
// edit, and the bundle — which IS the submission — silently reverts it.
// ---------------------------------------------------------------------------

const mfDriftSlug = "custom-generators"
const mfDriftBlockID = "apb_01TESTBLOCK"

// storedManifest builds a platform-shaped manifest: the author's fields PLUS the
// two the platform stamps.
//
// 🔴 THE SERVER-OWNED FIELDS ARE IN EVERY FIXTURE ON PURPOSE. The platform
// really does add them to the copy it stores (`iframe.src` stamped,
// `trustTier` forced), so a fixture without them is the textbook-vs-realistic
// trap: the comparison would look correct while never exercising the ignore
// list that stops it firing on every app forever.
func storedManifest(overrides map[string]any) map[string]any {
	m := map[string]any{
		"$schema":       "https://civitai.com/schemas/app-block/v1.json",
		"blockId":       mfDriftSlug,
		"version":       "0.6.1",
		"name":          "Custom Generators",
		"type":          "block",
		"scopes":        []any{},
		"page":          map[string]any{"path": "/", "title": "Custom Generators", "icon": "bolt"},
		"iframe":        map[string]any{"minHeight": float64(400), "maxHeight": float64(4000), "resizable": true, "sandbox": "allow-scripts allow-forms", "src": "https://custom-generators.civit.ai/"},
		"contentRating": "g",
		"minApiVersion": "1.0",
		"trustTier":     "unverified",
	}
	for k, v := range overrides {
		m[k] = v
	}
	return m
}

// localManifestFile writes an author-shaped manifest (no server-owned fields)
// into dir and returns dir.
func localManifestFile(t *testing.T, dir string, overrides map[string]any) string {
	t.Helper()
	m := map[string]any{
		"$schema":       "https://civitai.com/schemas/app-block/v1.json",
		"blockId":       mfDriftSlug,
		"version":       "0.6.2",
		"name":          "Custom Generators",
		"type":          "block",
		"scopes":        []any{},
		"page":          map[string]any{"path": "/", "title": "Custom Generators", "icon": "bolt"},
		"iframe":        map[string]any{"minHeight": float64(400), "maxHeight": float64(4000), "resizable": true, "sandbox": "allow-scripts allow-forms"},
		"contentRating": "g",
		"minApiVersion": "1.0",
	}
	for k, v := range overrides {
		m[k] = v
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fetcherReturning(m map[string]any) storedManifestFetcher {
	return func(ctx context.Context, appBlockID string) (*appapi.StoredAppManifest, error) {
		return &appapi.StoredAppManifest{ID: appBlockID, BlockID: mfDriftSlug, Status: "approved", Version: "0.6.1", Manifest: m}, nil
	}
}

// --- THE CASE THE FEATURE EXISTS FOR -----------------------------------

// TestDriftWarnsWhenTheWebsiteEditedAFieldTheLocalCopyStillHasOld.
func TestDriftWarnsWhenTheWebsiteChangedAField(t *testing.T) {
	dir := localManifestFile(t, t.TempDir(), map[string]any{"tagline": "old pitch"})
	stored := storedManifest(map[string]any{"tagline": "pitch edited on the website"})

	var warn bytes.Buffer
	diffs := warnOnManifestDrift(context.Background(), fetcherReturning(stored), &warn, dir, mfDriftBlockID)

	if len(diffs) != 1 || diffs[0].Field != "tagline" {
		t.Fatalf("want exactly one difference on `tagline`, got %+v", diffs)
	}
	out := warn.String()
	for _, want := range []string{"OVERWRITE", "tagline", "pitch edited on the website", "old pitch"} {
		if !strings.Contains(out, want) {
			t.Errorf("the warning must contain %q:\n%s", want, out)
		}
	}
}

// TestDriftReportsAFieldTheWebsiteADDED — the case a local-keys-only loop
// cannot see, because there is nothing local to iterate to.
func TestDriftReportsAFieldTheWebsiteAdded(t *testing.T) {
	dir := localManifestFile(t, t.TempDir(), nil) // no tagline at all
	stored := storedManifest(map[string]any{"tagline": "set in the web form"})

	var warn bytes.Buffer
	diffs := warnOnManifestDrift(context.Background(), fetcherReturning(stored), &warn, dir, mfDriftBlockID)

	if len(diffs) != 1 || diffs[0].Field != "tagline" {
		t.Fatalf("a field present ONLY on the website must be reported, got %+v", diffs)
	}
	if !strings.Contains(warn.String(), "(not set)") {
		t.Errorf("the local side must render as absent, not as empty:\n%s", warn.String())
	}
}

// TestDriftReportsANestedLeafByItsFullPath.
func TestDriftReportsANestedLeafByItsFullPath(t *testing.T) {
	dir := localManifestFile(t, t.TempDir(), nil)
	stored := storedManifest(map[string]any{
		"page": map[string]any{"path": "/", "title": "Renamed On The Website", "icon": "bolt"},
	})

	var warn bytes.Buffer
	diffs := warnOnManifestDrift(context.Background(), fetcherReturning(stored), &warn, dir, mfDriftBlockID)

	if len(diffs) != 1 || diffs[0].Field != "page.title" {
		t.Fatalf("want one difference on `page.title` (the leaf, not the subtree), got %+v", diffs)
	}
}

// --- THE FIELDS THAT MUST NEVER FIRE -----------------------------------

// TestDriftIgnoresServerOwnedFieldsAndVersion.
//
// 🔴 THIS IS THE ONE THAT KEEPS THE FEATURE USABLE. The platform stamps
// `iframe.src` and forces `trustTier` into the stored copy, and a submit exists
// to raise `version` — so all three differ on every correct run, for every app,
// forever. If they were reported, the warning would fire on every submit and
// nobody would read the one that mattered.
func TestDriftIgnoresServerOwnedFieldsAndVersion(t *testing.T) {
	// Identical except the three fields that always differ.
	dir := localManifestFile(t, t.TempDir(), nil) // version 0.6.2, no src, no trustTier
	stored := storedManifest(nil)                 // version 0.6.1, src + trustTier set

	var warn bytes.Buffer
	diffs := warnOnManifestDrift(context.Background(), fetcherReturning(stored), &warn, dir, mfDriftBlockID)

	if len(diffs) != 0 {
		t.Errorf("no field may be reported here — every difference is structural: %+v\n%s", diffs, warn.String())
	}
	if warn.Len() != 0 {
		t.Errorf("a routine submit must print nothing:\n%s", warn.String())
	}
}

// TestDriftIgnoreListIsDerivedFromTheValidator pins the DERIVATION, not the
// values: adding a server-owned field to the validator must extend the ignore
// set without anyone editing this package.
func TestDriftIgnoreListIsDerivedFromTheValidator(t *testing.T) {
	ignored := serverOwnedIgnored()
	if len(validate.ServerOwnedFields) == 0 {
		t.Fatal("validate.ServerOwnedFields is empty — this guard would assert nothing")
	}
	for _, f := range validate.ServerOwnedFields {
		if _, ok := ignored[f]; !ok {
			t.Errorf("validate.ServerOwnedFields names %q but the drift comparison does not ignore it — "+
				"it would then differ on every app forever", f)
		}
	}
	if _, ok := ignored["version"]; !ok {
		t.Error("`version` must be ignored — a submit exists to raise it, so it differs on every correct run")
	}
}

// --- DEGRADE, NEVER ENFORCE --------------------------------------------

func TestDriftIsSilentWithoutAnAppBlockID(t *testing.T) {
	dir := localManifestFile(t, t.TempDir(), map[string]any{"tagline": "x"})
	called := false
	fetch := func(ctx context.Context, id string) (*appapi.StoredAppManifest, error) {
		called = true
		return nil, nil
	}
	var warn bytes.Buffer
	if diffs := warnOnManifestDrift(context.Background(), fetch, &warn, dir, ""); diffs != nil {
		t.Errorf("no appBlockId (a first submit) must report nothing, got %+v", diffs)
	}
	if called {
		t.Error("it must not make the API call at all — a first submit has no stored copy to compare against")
	}
	if warn.Len() != 0 {
		t.Errorf("and it must be silent:\n%s", warn.String())
	}
}

// TestDriftIsSilentWhenTheReadFails.
//
// 🔴 DELIBERATELY SILENT, AND THIS IS A REVERSAL. The earlier git-based attempt
// warned "the check was skipped, your bundle may overwrite the website's
// changes" whenever it could not check — a sentence that is true on every
// offline run and every first submit, so it fired constantly and named nothing
// actionable. A warning nobody can act on is what makes the real one invisible.
func TestDriftIsSilentWhenTheReadFails(t *testing.T) {
	dir := localManifestFile(t, t.TempDir(), map[string]any{"tagline": "x"})
	fetch := func(ctx context.Context, id string) (*appapi.StoredAppManifest, error) {
		return nil, context.DeadlineExceeded
	}
	var warn bytes.Buffer
	if diffs := warnOnManifestDrift(context.Background(), fetch, &warn, dir, mfDriftBlockID); diffs != nil {
		t.Errorf("a failed read must report nothing, got %+v", diffs)
	}
	if warn.Len() != 0 {
		t.Errorf("a failed read must be silent — it is unactionable:\n%s", warn.String())
	}
}

func TestDriftIsSilentWhenTheLocalManifestIsUnreadable(t *testing.T) {
	var warn bytes.Buffer
	diffs := warnOnManifestDrift(context.Background(), fetcherReturning(storedManifest(nil)), &warn, t.TempDir(), mfDriftBlockID)
	if diffs != nil || warn.Len() != 0 {
		t.Errorf("validation already reports a missing manifest; this must add nothing. diffs=%+v warn=%s", diffs, warn.String())
	}
}

// --- THE API CLIENT ----------------------------------------------------

// TestGetMyAppManifestDecodesTheTRPCEnvelope, including that UNKNOWN keys
// survive — a typed decode would drop exactly the fields the website might have
// added.
func TestGetMyAppManifestDecodesTheTRPCEnvelope(t *testing.T) {
	var gotInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, appapi.MyAppManifestPath) {
			http.Error(w, "unexpected route: "+r.URL.Path, http.StatusNotFound)
			return
		}
		gotInput = r.URL.Query().Get("input")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"data": map[string]any{"json": map[string]any{
				"id": mfDriftBlockID, "blockId": mfDriftSlug, "status": "approved", "version": "0.6.1",
				"manifest": map[string]any{"name": "N", "aFieldTheCLIDoesNotKnow": "kept"},
			}}},
		})
	}))
	t.Cleanup(srv.Close)

	c := appapi.New(srv.URL, "tok-test", "")
	got, err := c.GetMyAppManifest(context.Background(), mfDriftBlockID)
	if err != nil {
		t.Fatalf("GetMyAppManifest: %v", err)
	}
	if !strings.Contains(gotInput, mfDriftBlockID) {
		t.Errorf("the appBlockId must be sent in the tRPC input, got %q", gotInput)
	}
	if got.Version != "0.6.1" || got.BlockID != mfDriftSlug {
		t.Errorf("row fields decoded wrong: %+v", got)
	}
	if got.Manifest["aFieldTheCLIDoesNotKnow"] != "kept" {
		t.Errorf("an unknown manifest key was DROPPED — the comparison would be blind to exactly the "+
			"fields the website might have added: %+v", got.Manifest)
	}
}

func TestGetMyAppManifestSurfacesAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "Apps are not available", "code": -32003}},
		})
	}))
	t.Cleanup(srv.Close)

	c := appapi.New(srv.URL, "tok-test", "")
	_, err := c.GetMyAppManifest(context.Background(), mfDriftBlockID)
	if err == nil {
		t.Fatal("a 403 must surface as an error")
	}
	if !strings.Contains(err.Error(), "Apps are not available") {
		t.Errorf("the server's message must reach the caller, got %v", err)
	}
}
