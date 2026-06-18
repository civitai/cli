// Package validate checks a block.manifest.json against the vendored JSON
// Schema plus structural project checks (manifest at root, build coherence).
package validate

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	cli "github.com/civitai/cli"
	"github.com/civitai/cli/internal/manifest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// printer renders jsonschema ErrorKind messages. It must be non-nil — the
// library's LocalizedString implementations dereference it.
var printer = message.NewPrinter(language.English)

// Result is the outcome of validating a project directory.
type Result struct {
	Errors []string
}

// OK reports whether validation passed.
func (r Result) OK() bool { return len(r.Errors) == 0 }

var compiled *jsonschema.Schema

func schema() (*jsonschema.Schema, error) {
	if compiled != nil {
		return compiled, nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(cli.SchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("vendored schema is not valid JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	const url = "https://civitai.com/schemas/app-block/v1.json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile vendored schema: %w", err)
	}
	compiled = s
	return s, nil
}

// Dir validates the App Block project in dir.
func Dir(dir string) (Result, error) {
	var res Result

	// Structural: manifest must exist at the project root.
	if _, err := os.Stat(manifest.Path(dir)); err != nil {
		if os.IsNotExist(err) {
			return Result{Errors: []string{
				fmt.Sprintf("%s not found at project root %s", manifest.Filename, dir),
			}}, nil
		}
		return res, err
	}

	generic, m, err := manifest.LoadRaw(dir)
	if err != nil {
		// A parse error is a validation failure, not a CLI error.
		return Result{Errors: []string{err.Error()}}, nil
	}

	// Schema validation.
	s, err := schema()
	if err != nil {
		return res, err
	}
	if err := s.Validate(generic); err != nil {
		res.Errors = append(res.Errors, schemaErrors(err)...)
	}

	// Structural: server-owned fields must not be set by the developer. The
	// JSON Schema can express this with `not`, but its error message ("not
	// failed", no field path) is useless — do it in Go for a clear message.
	res.Errors = append(res.Errors, serverOwnedFieldChecks(generic)...)

	// Structural: buildCommand + outputDir coherence beyond what the schema
	// can express (the schema enforces "outputDir required when buildCommand
	// set"; here we also check the directory is declared/exists sanely).
	res.Errors = append(res.Errors, buildCoherence(dir, m)...)

	sort.Strings(res.Errors)
	res.Errors = dedupe(res.Errors)
	return res, nil
}

// serverOwnedFieldChecks rejects manifest fields the platform assigns. Devs
// must not set them; doing so is a hard error with a clear, actionable message.
func serverOwnedFieldChecks(generic any) []string {
	var errs []string
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	if _, set := m["trustTier"]; set {
		errs = append(errs, "trustTier is server-owned — remove it from your manifest; the platform assigns the trust tier during review")
	}
	if iframe, ok := m["iframe"].(map[string]any); ok {
		if _, set := iframe["src"]; set {
			errs = append(errs, "iframe.src is server-owned — remove it; the platform stamps the canonical bundle URL during build/approve")
		}
	}
	return errs
}

func buildCoherence(dir string, m *manifest.Manifest) []string {
	var errs []string
	hasBuild := strings.TrimSpace(m.BuildCommand) != ""
	hasOut := strings.TrimSpace(m.OutputDir) != ""

	switch {
	case hasBuild && !hasOut:
		errs = append(errs, "buildCommand is set but outputDir is missing — the platform needs to know where the build output lands (e.g. \"dist\")")
	case !hasBuild && hasOut:
		// Not fatal, but almost always a mistake: an outputDir with no build
		// to produce it. Warn as an error so it surfaces.
		errs = append(errs, "outputDir is set but buildCommand is missing — a no-build (static) app should omit outputDir; a built app must set buildCommand")
	}
	return errs
}

// schemaErrors flattens a jsonschema validation error into clear, per-field
// messages keyed by the offending JSON path.
func schemaErrors(err error) []string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	var out []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		// Only emit leaf errors (those with no children carry the concrete
		// reason); intermediate nodes just describe the path.
		if len(e.Causes) == 0 {
			loc := strings.Join(e.InstanceLocation, "/")
			if loc == "" {
				loc = "(root)"
			} else {
				loc = "/" + loc
			}
			out = append(out, fmt.Sprintf("%s: %s", loc, e.ErrorKind.LocalizedString(printer)))
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(out) == 0 {
		out = append(out, ve.Error())
	}
	return out
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
