package scaffold_test

// The most likely way the bootSkeleton change breaks something is NOT the
// markup — it is the CLI refusing its own scaffold output.
//
// `civitai app validate` compiles the go:embedded mirror of the platform's
// canonical manifest schema and validates block.manifest.json against it. Adding
// a key the mirror does not know is only safe because the canonical schema's
// ROOT object does not set `additionalProperties: false` (it does set it on the
// `iframe` and `page` sub-objects, which is why this is worth pinning rather
// than assuming). If that ever changes — or if the mirror is re-vendored from a
// canonical that tightens the root — every scaffolded app starts failing the
// CLI's own validate the moment it is created, and the schema re-vendor stops
// being optional.
//
// This test is the tripwire for that. It is deliberately an END-TO-END read
// through validate.Dir rather than an inspection of the schema file: the
// question is whether the CLI accepts the manifest, and only running the
// validator answers it.

import (
	"path/filepath"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/validate"
)

func TestScaffoldedManifestsWithBootSkeletonStillValidate(t *testing.T) {
	examined := 0
	for _, tmpl := range scaffold.AllTemplates() {
		examined++
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), string(tmpl))
			if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "boot-block", Name: "Boot Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
			}

			res, err := validate.ManifestOnly(dest)
			if err != nil {
				t.Fatalf("validate.ManifestOnly(%s): %v", tmpl, err)
			}
			if !res.OK() {
				t.Fatalf("the scaffolded %q manifest FAILS the CLI's own validate:\n  %v\n\n"+
					"  If the failure names `bootSkeleton`, the vendored schema mirror\n"+
					"  (schema/app-block.manifest.schema.json) does not know the key AND the canonical\n"+
					"  root has been tightened to additionalProperties:false. The re-vendor is then\n"+
					"  NOT optional — run ./scripts/revendor-canonical-schema.sh (or wait for the\n"+
					"  revendor-canonical-schema workflow's PR) before this can ship.",
					tmpl, res.Errors)
			}
		})
	}
	// COUNT POSITIVE CONTROL — an empty AllTemplates() makes this pass serenely.
	if examined < len(scaffold.AllTemplates()) || examined < 3 {
		t.Fatalf("examined only %d template(s), expected at least 3 — the enumeration stopped OBSERVING", examined)
	}
}
