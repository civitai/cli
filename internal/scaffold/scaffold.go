// Package scaffold renders embedded App project templates onto disk.
package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:embed all:templates
var templatesFS embed.FS

// Template is a named project template the CLI can scaffold.
type Template string

const (
	// Static is a no-build page block (index.html + a tiny JS).
	Static Template = "static"
	// PageVite is a vite+React page block with config-as-code build fields.
	PageVite Template = "page-vite"
	// PageMoney is a vite+React+TS page block wired to the published App SDK
	// for the money path (estimate → consent → submit → poll → Buzz spend).
	PageMoney Template = "page-money"
)

// AllTemplates lists the templates available to `civitai app init`.
func AllTemplates() []Template { return []Template{Static, PageVite, PageMoney} }

// SDKTemplates are templates whose dev loop needs a mock host (`dev:harness`)
// rather than a plain `dev` (which renders blank without a host).
func (t Template) NeedsHarness() bool { return t == PageMoney }

// ParseTemplate validates a template name.
func ParseTemplate(s string) (Template, error) {
	switch Template(s) {
	case Static:
		return Static, nil
	case PageVite:
		return PageVite, nil
	case PageMoney:
		return PageMoney, nil
	default:
		names := make([]string, 0, len(AllTemplates()))
		for _, t := range AllTemplates() {
			names = append(names, string(t))
		}
		return "", fmt.Errorf("unknown template %q (valid: %s)", s, strings.Join(names, ", "))
	}
}

// Data is the values passed to every template.
type Data struct {
	// Slug is the block slug (blockId): lowercase, hyphen-separated.
	Slug string
	// Name is the human-readable display name.
	Name string
}

// outputName maps an embedded template filename to its on-disk name.
// `.tmpl` is stripped; some names are rewritten because go:embed cannot embed
// files whose names begin with a dot:
//   - `gitignore`        -> `.gitignore`
//   - `env.development`  -> `.env.development`
//   - `env.production`   -> `.env.production`
//   - `env.example`      -> `.env.example`
func outputName(rel string) string {
	rel = strings.TrimSuffix(rel, ".tmpl")
	base := filepath.Base(rel)
	switch {
	case base == "gitignore":
		rel = filepath.Join(filepath.Dir(rel), ".gitignore")
	case base == "env.development" || base == "env.production" || base == "env.example":
		rel = filepath.Join(filepath.Dir(rel), "."+base)
	}
	return rel
}

// Render writes the chosen template into destDir, creating it if needed.
// It refuses to overwrite an existing non-empty directory.
func Render(tmpl Template, destDir string, data Data) ([]string, error) {
	root := "templates/" + string(tmpl)

	if err := ensureEmptyDir(destDir); err != nil {
		return nil, err
	}

	var written []string
	err := fs.WalkDir(templatesFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out := filepath.Join(destDir, outputName(rel))

		raw, err := templatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		t, err := template.New(rel).Option("missingkey=error").Parse(string(raw))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", rel, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return fmt.Errorf("render template %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
			return err
		}
		written = append(written, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(written)
	return written, nil
}

func ensureEmptyDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty — refusing to overwrite", dir)
	}
	return nil
}
