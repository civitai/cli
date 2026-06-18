package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

func scaffoldGood(t *testing.T, tmpl scaffold.Template) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "block")
	if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "good-block", Name: "Good Block"}); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return dir
}

func TestValidateAcceptsStaticTemplate(t *testing.T) {
	res, err := Dir(scaffoldGood(t, scaffold.Static))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("static template should be valid, got: %v", res.Errors)
	}
}

func TestValidateAcceptsPageViteTemplate(t *testing.T) {
	res, err := Dir(scaffoldGood(t, scaffold.PageVite))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("page-vite template should be valid, got: %v", res.Errors)
	}
}

// writeManifest writes a manifest-only project dir and validates it.
func validateManifest(t *testing.T, json string) Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	return res
}

func mustReject(t *testing.T, name, json, wantSubstr string) {
	t.Helper()
	res := validateManifest(t, json)
	if res.OK() {
		t.Fatalf("%s: expected rejection, got valid", name)
	}
	if wantSubstr != "" {
		found := false
		for _, e := range res.Errors {
			if strings.Contains(e, wantSubstr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: errors %v did not contain %q", name, res.Errors, wantSubstr)
		}
	}
}

const baseGood = `{
  "blockId": "ok-block",
  "version": "0.1.0",
  "name": "OK",
  "contentRating": "g",
  "scopes": [],
  "page": {"path": "/", "title": "OK"},
  "iframe": {"minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms"}
}`

func TestValidateAcceptsMinimalGood(t *testing.T) {
	res := validateManifest(t, baseGood)
	if !res.OK() {
		t.Fatalf("baseGood should validate, got: %v", res.Errors)
	}
}

func TestValidateRejectsMissingRequired(t *testing.T) {
	mustReject(t, "missing blockId", `{
		"version": "0.1.0", "name": "X", "contentRating": "g", "scopes": []
	}`, "blockId")
}

func TestValidateRejectsBadScope(t *testing.T) {
	mustReject(t, "bad scope", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["NotAScope"]
	}`, "scopes")
}

func TestValidateRejectsBadContentRating(t *testing.T) {
	mustReject(t, "bad contentRating", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "banana", "scopes": []
	}`, "contentRating")
}

func TestValidateRejectsDevSetIframeSrc(t *testing.T) {
	mustReject(t, "dev-set iframe.src", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"src": "https://evil.example.com", "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts"}
	}`, "iframe.src is server-owned")
}

func TestValidateRejectsDevSetTrustTier(t *testing.T) {
	mustReject(t, "dev-set trustTier", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "trustTier": "internal"
	}`, "trustTier is server-owned")
}

func TestValidateRejectsNonIntegerBuzzBudget(t *testing.T) {
	mustReject(t, "non-integer buzzBudgetPerGen", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"page": {"path": "/", "title": "X", "buzzBudgetPerGen": 1.5}
	}`, "buzzBudgetPerGen")
}

func TestValidateRejectsNonPositiveBuzzBudget(t *testing.T) {
	mustReject(t, "non-positive buzzBudgetPerGen", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"page": {"path": "/", "title": "X", "buzzBudgetPerGen": 0}
	}`, "buzzBudgetPerGen")
}

func TestValidateRejectsBuildCommandWithoutOutputDir(t *testing.T) {
	mustReject(t, "buildCommand without outputDir", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "buildCommand": "npm run build"
	}`, "outputDir")
}

func TestValidateRejectsBadSemver(t *testing.T) {
	mustReject(t, "bad version", `{
		"blockId": "ok-block", "version": "v1", "name": "X",
		"contentRating": "g", "scopes": []
	}`, "version")
}

func TestValidateRejectsPagePathNotSlash(t *testing.T) {
	mustReject(t, "page.path no slash", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"page": {"path": "home", "title": "X"}
	}`, "path")
}

func TestValidateMissingManifest(t *testing.T) {
	res, err := Dir(t.TempDir())
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if res.OK() {
		t.Fatal("empty dir should fail validation")
	}
}
