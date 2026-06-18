package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	cli "github.com/civitai/cli"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/validate"
)

// TestExampleManifestsValidateClean asserts that the real example manifests
// shipped under examples/ (copied from the live civitai-block-* apps) pass the
// CLI's `validate` — making the README/PR claim "the example manifests validate
// clean" TRUE rather than aspirational. If a future server-validator change
// makes one of these stricter, this test fails and forces a reconciliation
// (fix the example, or fix the ported check, and document it).
func TestExampleManifestsValidateClean(t *testing.T) {
	entries, err := cli.ExamplesFS.ReadDir("examples")
	if err != nil {
		t.Fatalf("read embedded examples: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no example manifests embedded — examples/*.block.manifest.json missing")
	}

	for _, e := range entries {
		e := e
		t.Run(e.Name(), func(t *testing.T) {
			data, err := cli.ExamplesFS.ReadFile(filepath.Join("examples", e.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			dir := t.TempDir()
			// validate.Dir expects the manifest at the project root under the
			// canonical filename.
			if err := os.WriteFile(filepath.Join(dir, manifest.Filename), data, 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			res, err := validate.Dir(dir)
			if err != nil {
				t.Fatalf("validate.Dir: %v", err)
			}
			if !res.OK() {
				t.Fatalf("example %s should validate clean, got errors: %v", e.Name(), res.Errors)
			}
		})
	}
}
