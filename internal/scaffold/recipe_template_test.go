package scaffold

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// repoRoot returns the repository root, resolved from this test file's location
// (internal/scaffold) so the test is cwd-independent.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// …/internal/scaffold/recipe_template_test.go -> repo root is two dirs up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestRequestRecipeIssueTemplateExists guards finding #3: the README template's
// recipe-request link must point at a real GitHub issue template. This asserts
// the dedicated request-recipe.yml template exists at the repo root and parses
// as a valid GitHub issue form (name + body). It is a REPO-LEVEL file, not
// scaffold output, so it must NOT be embedded under templates/ (guarded below).
func TestRequestRecipeIssueTemplateExists(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".github", "ISSUE_TEMPLATE", "request-recipe.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("request-recipe.yml issue template missing (%s): %v", path, err)
	}
	var form struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Body        []struct {
			Type string `yaml:"type"`
		} `yaml:"body"`
	}
	if err := yaml.Unmarshal(raw, &form); err != nil {
		t.Fatalf("request-recipe.yml is not valid YAML: %v", err)
	}
	if form.Name == "" {
		t.Error("request-recipe.yml must set a name")
	}
	if len(form.Body) == 0 {
		t.Error("request-recipe.yml must declare a body (form fields)")
	}
	if !strings.Contains(strings.ToLower(form.Name+form.Description), "recipe") {
		t.Errorf("request-recipe.yml should be about a recipe request, got name=%q", form.Name)
	}
}

// TestReadmeTemplateRecipeLinkPointsAtRealTemplate guards finding #3: the
// page-money README template's "Requesting a new recipe" link must target the
// real request-recipe.yml issue template, not the dead bare-repo URL.
func TestReadmeTemplateRecipeLinkPointsAtRealTemplate(t *testing.T) {
	path := filepath.Join(repoRoot(t), "internal", "scaffold", "templates", "page-money", "README.md.tmpl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read README template: %v", err)
	}
	readme := string(raw)
	if !strings.Contains(readme, "issues/new?template=request-recipe.yml") {
		t.Errorf("README template recipe-request link must point at request-recipe.yml issue template:\n%s", readme)
	}
	// The old dead link (bare repo root, no template) must be gone.
	if strings.Contains(readme, "issue form at <https://github.com/civitai/cli>") {
		t.Error("README template still uses the dead bare-repo recipe-request link")
	}
}

// TestRecipeIssueTemplateNotEmbeddedInScaffold guards that the new repo-level
// .github file did not leak into the embedded scaffold template set (it must not
// be shipped into a scaffolded app). The scaffold embeds `all:templates`, so a
// file outside templates/ can't leak — this asserts that invariant explicitly.
func TestRecipeIssueTemplateNotEmbeddedInScaffold(t *testing.T) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("read embedded templates: %v", err)
	}
	for _, e := range entries {
		if e.Name() == ".github" || strings.Contains(e.Name(), "request-recipe") {
			t.Errorf("repo-level .github file leaked into embedded scaffold templates: %s", e.Name())
		}
	}
}
