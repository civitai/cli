package scaffold

import (
	"path/filepath"
	"testing"

	"github.com/civitai/cli/internal/antipattern"
)

// TestScaffoldedTemplatesHaveNoAntipatterns renders EVERY template to disk and
// asserts the scaffold-currency anti-pattern gate finds nothing. This is the
// offline half of the gate (runs in the default `go test ./...`): it proves the
// SHIPPED templates never teach a dead/superseded platform surface, and it fires
// the moment a future template edit reintroduces one — before it reaches an
// author. The network-dependent scaffold-of-record scan runs in CI against the
// `civitai app init` output; this covers all templates hermetically.
func TestScaffoldedTemplatesHaveNoAntipatterns(t *testing.T) {
	for _, tmpl := range AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), string(tmpl))
			if _, err := Render(tmpl, dest, Data{Slug: "sample-block", Name: "Sample Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
			}
			findings, err := antipattern.ScanDir(dest)
			if err != nil {
				t.Fatalf("scan %s: %v", tmpl, err)
			}
			if len(findings) > 0 {
				t.Fatalf("template %q ships an anti-pattern:\n%s", tmpl, antipattern.Format(findings))
			}
		})
	}
}
