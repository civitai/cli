package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lockfileNames are the three lockfiles the platform build recipe installs
// strictly from. Ignoring any of them in a scaffolded .gitignore would produce
// an app that cannot be built: the author installs, the lockfile is never
// committed, and the build hard-fails on the missing file.
var lockfileNames = []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock"}

// TestScaffoldGitignoreDoesNotIgnoreLockfiles guards the scaffold against
// generating a born-unbuildable app. A `*.lock` / `package-lock.json` line
// sneaking into a gitignore template is a silent, one-line regression whose only
// symptom is an opaque server-side build failure weeks later.
func TestScaffoldGitignoreDoesNotIgnoreLockfiles(t *testing.T) {
	for _, tmpl := range AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "block")
			if _, err := Render(tmpl, dir, Data{Slug: "lock-guard", Name: "Lock Guard"}); err != nil {
				t.Fatalf("render: %v", err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
			if err != nil {
				if os.IsNotExist(err) {
					return // no .gitignore is fine — nothing can be ignored
				}
				t.Fatalf("read .gitignore: %v", err)
			}
			for _, line := range strings.Split(string(raw), "\n") {
				pattern := strings.TrimSpace(line)
				if pattern == "" || strings.HasPrefix(pattern, "#") || strings.HasPrefix(pattern, "!") {
					continue
				}
				// Strip a leading "/" (anchored pattern) and a trailing "/"
				// (directory-only) before matching the base name.
				pattern = strings.TrimSuffix(strings.TrimPrefix(pattern, "/"), "/")
				for _, lock := range lockfileNames {
					if ok, _ := filepath.Match(pattern, lock); ok {
						t.Errorf("%s/.gitignore line %q ignores %s — the platform build installs strictly from the committed lockfile and will hard-fail without it", tmpl, line, lock)
					}
				}
			}
		})
	}
}

// TestBuildTemplatesNeedInstall pins NeedsInstall to "the template scaffolds a
// package.json", which is exactly the condition the platform build recipe keys
// its install step off (`if [ -f package.json ]`). The scaffold's
// commit-your-lockfile guidance is gated on it.
func TestBuildTemplatesNeedInstall(t *testing.T) {
	for _, tmpl := range AllTemplates() {
		dir := filepath.Join(t.TempDir(), "block")
		if _, err := Render(tmpl, dir, Data{Slug: "lock-guard", Name: "Lock Guard"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		_, statErr := os.Stat(filepath.Join(dir, "package.json"))
		hasPackageJSON := statErr == nil
		if hasPackageJSON != tmpl.NeedsInstall() {
			t.Errorf("%s: NeedsInstall()=%v but package.json present=%v", tmpl, tmpl.NeedsInstall(), hasPackageJSON)
		}
	}
}
