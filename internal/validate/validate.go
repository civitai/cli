// Package validate checks a block.manifest.json against the vendored JSON
// Schema plus structural project checks (manifest at root, build coherence).
package validate

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	cli "github.com/civitai/cli"
	"github.com/civitai/cli/internal/manifest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// buildCommandRe is the allowlist of permitted build invocations, kept in
// lockstep with BUILD_COMMAND_RE in the server's block-manifest-validator
// (src/server/services/block-manifest-validator.service.ts). It admits only
// "npm/pnpm/yarn run <script>", "vite build", and "npx vite build" — a
// defense-in-depth guard against shell injection via buildCommand. The vendored
// JSON Schema carries the same `pattern`; this Go copy exists only to emit a
// clear, actionable message instead of a raw regex-mismatch error.
var buildCommandRe = regexp.MustCompile(`^(?:(?:npm|pnpm|yarn) run [a-zA-Z0-9:_-]+|(?:npx )?vite build)$`)

// buildCommandMaxLength mirrors BUILD_COMMAND_MAX_LENGTH on the server.
const buildCommandMaxLength = 128

// printer renders jsonschema ErrorKind messages. It must be non-nil — the
// library's LocalizedString implementations dereference it.
var printer = message.NewPrinter(language.English)

// Result is the outcome of validating a project directory.
//
// Both slices hold Findings — a field PLUS a message — rather than bare
// strings. See finding.go for why (issue #225): the field has to be carried
// from where a finding is produced, because it cannot be recovered from prose
// afterwards.
type Result struct {
	// Errors are hard failures: the server will reject the manifest. Non-empty
	// Errors means validation failed.
	Errors []Finding
	// Warnings are non-fatal money-path / footgun advisories the schema can't
	// express as hard errors (e.g. a budgeted page with no per-gen budget).
	// They do not fail validation unless --strict is requested.
	Warnings []Finding
}

// OK reports whether validation passed (no hard errors).
func (r Result) OK() bool { return len(r.Errors) == 0 }

// HasWarnings reports whether any non-fatal advisories were emitted.
func (r Result) HasWarnings() bool { return len(r.Warnings) > 0 }

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

// Dir validates the App project in dir: the manifest checks PLUS the
// project-state checks that depend on files the AUTHOR maintains (currently the
// lockfile ↔ buildCommand consistency check — see lockfile.go).
func Dir(dir string) (Result, error) { return validateDir(dir, true) }

// ManifestOnly validates just the manifest of the project in dir, skipping the
// project-state checks.
//
// `civitai app init` / `app create` use it to self-check the template they just
// wrote. That self-check exists to catch a bug in OUR templates; a scaffold
// legitimately has no lockfile yet (the author's first `npm install` writes it),
// so failing there would turn an author-state issue into "internal error:
// scaffolded manifest failed validation". `civitai app validate` on that same
// directory still reports it — which is correct, because the app is not
// submittable until the lockfile is committed.
func ManifestOnly(dir string) (Result, error) { return validateDir(dir, false) }

func validateDir(dir string, projectState bool) (Result, error) {
	var res Result

	// Structural: manifest must exist at the project root. There is no manifest
	// to name a field IN, so this is a document-level finding.
	if _, err := os.Stat(manifest.Path(dir)); err != nil {
		if os.IsNotExist(err) {
			return Result{Errors: []Finding{newFinding(FieldDocument,
				fmt.Sprintf("%s not found at project root %s", manifest.Filename, dir),
			)}}, nil
		}
		return res, err
	}

	generic, m, err := manifest.LoadRaw(dir)
	if err != nil {
		// A parse error is a validation failure, not a CLI error. The document
		// did not decode, so no field inside it can be named.
		return Result{Errors: []Finding{newFinding(FieldDocument, err.Error())}}, nil
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

	// Semantic: the cross-field / tier-gated rules the server runs at approve
	// time (BlockManifestValidator) that the JSON Schema cannot express —
	// sandbox trust-tier allowlist, page⇒iframe, iframe required sub-fields,
	// renderMode tier gate, target slotId registry. Without these, validate
	// green-lights manifests the server rejects. See semantic.go / targets.go.
	res.Errors = append(res.Errors, semanticChecks(generic)...)
	res.Errors = append(res.Errors, targetChecks(generic)...)

	// Structural: buildCommand + outputDir coherence beyond what the schema
	// can express (the schema enforces "outputDir required when buildCommand
	// set"; here we also check the directory is declared/exists sanely).
	res.Errors = append(res.Errors, buildCoherence(dir, m)...)

	// Project state: the committed lockfile must match the package manager the
	// platform build derives from buildCommand. The build recipe installs
	// STRICTLY from that lockfile (no registry re-resolve fallback), so a
	// mismatch is a guaranteed build failure. Skipped for a freshly scaffolded
	// project (see ManifestOnly) and, inside the check, for static blocks.
	if projectState {
		lockErrs, lockWarns := lockfileChecks(dir, m)
		res.Errors = append(res.Errors, lockErrs...)
		res.Warnings = append(res.Warnings, lockWarns...)

		// Project state: a `page` app whose source never posts BLOCK_READY is
		// invisible in the real host (issue #206). ADVISORY, not fatal — it
		// infers runtime behaviour from static text and can be wrong. It lives
		// HERE rather than in warningChecks because it reads src/, and
		// warningChecks also runs under ManifestOnly, where `civitai app init`
		// self-checks a template it just wrote. See readyack.go.
		res.Warnings = append(res.Warnings, readyAckChecks(dir, generic)...)
	}

	// Non-fatal advisories: real money-path footguns the schema can't catch as
	// hard errors (see warnings.go). These never fail validation unless the
	// caller opts into --strict.
	res.Warnings = append(res.Warnings, warningChecks(generic)...)

	sortFindings(res.Errors)
	res.Errors = dedupeFindings(res.Errors)
	sortFindings(res.Warnings)
	res.Warnings = dedupeFindings(res.Warnings)
	return res, nil
}

// serverOwnedFieldChecks rejects manifest fields the platform assigns. Devs
// must not set them; doing so is a hard error with a clear, actionable message.
func serverOwnedFieldChecks(generic any) []Finding {
	var errs []Finding
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	if _, set := m["trustTier"]; set {
		errs = append(errs, newFinding("trustTier",
			"trustTier is server-owned — remove it from your manifest; the platform assigns the trust tier during review"))
	}
	if iframe, ok := m["iframe"].(map[string]any); ok {
		if _, set := iframe["src"]; set {
			errs = append(errs, newFinding(childField("iframe", "src"),
				"iframe.src is server-owned — remove it; the platform stamps the canonical bundle URL during build/approve"))
		}
	}
	return errs
}

// buildCoherence checks buildCommand/outputDir.
//
// FIELD ASSIGNMENT for the two PAIR rules: the finding names the field the
// message reports as MISSING, not the one whose presence triggered the rule.
// "buildCommand is set but outputDir is missing" is a finding about
// `outputDir`; its mirror is a finding about `buildCommand`. That is mechanical
// (it reads straight off the sentence), symmetric across the pair, and it puts
// the finding on the field a consumer would have to edit.
func buildCoherence(dir string, m *manifest.Manifest) []Finding {
	var errs []Finding
	build := strings.TrimSpace(m.BuildCommand)
	hasBuild := build != ""
	out := strings.TrimSpace(m.OutputDir)
	hasOut := out != ""

	// buildCommand must be one of the allowlisted invocations (defense in depth
	// against shell injection). The vendored JSON Schema also enforces the same
	// pattern + max length, but its error ("does not match pattern ^(?:...)$")
	// is opaque — emit a clear, actionable message instead. Mirrors the server's
	// BUILD_COMMAND_RE / BUILD_COMMAND_MAX_LENGTH exactly.
	if hasBuild {
		if len(build) > buildCommandMaxLength {
			errs = append(errs, newFinding("buildCommand", fmt.Sprintf("buildCommand is too long (%d chars) — it must be at most %d characters", len(build), buildCommandMaxLength)))
		} else if !buildCommandRe.MatchString(build) {
			errs = append(errs, newFinding("buildCommand", fmt.Sprintf("buildCommand %q is not an allowed build invocation — use \"npm run <script>\", \"pnpm run <script>\", \"yarn run <script>\", \"vite build\", or \"npx vite build\"", build)))
		}
	}

	switch {
	case hasBuild && !hasOut:
		errs = append(errs, newFinding("outputDir", "buildCommand is set but outputDir is missing — the platform needs to know where the build output lands (e.g. \"dist\")"))
	case !hasBuild && hasOut:
		// Not fatal, but almost always a mistake: an outputDir with no build
		// to produce it. Warn as an error so it surfaces.
		errs = append(errs, newFinding("buildCommand", "outputDir is set but buildCommand is missing — a no-build (static) app should omit outputDir; a built app must set buildCommand"))
	}

	// outputDir must be a SAFE RELATIVE path. The server companion validates
	// outputDir as a relative path under the project root (no absolute paths,
	// no `..` traversal); the CLI matching that is correct. The JSON Schema's
	// `^[^/]` pattern catches the leading-slash case but with a cryptic regex
	// message and misses `..` traversal — do it in Go for a clear, aligned
	// message.
	if hasOut {
		if strings.HasPrefix(out, "/") {
			errs = append(errs, newFinding("outputDir", fmt.Sprintf("outputDir %q must be a relative path under the project root — remove the leading \"/\"", out)))
		} else if out == ".." || strings.HasPrefix(out, "../") || strings.Contains(out, "/../") || strings.HasSuffix(out, "/..") {
			errs = append(errs, newFinding("outputDir", fmt.Sprintf("outputDir %q must not escape the project root — remove the \"..\" path traversal", out)))
		}
	}
	return errs
}

// schemaErrors flattens a jsonschema validation error into clear, per-field
// findings keyed by the offending JSON path.
//
// The path used to be rendered as a JSON Pointer ("/scopes/1"), which was one
// of the three notations issue #225 was about. It is now dotted
// ("scopes[1]") — the same notation the ported semantic checks and the human
// output use. The MESSAGE still leads with the path, so the text output reads
// the same way it always did; only the notation inside it changed.
func schemaErrors(err error) []Finding {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		// Not a jsonschema error at all — we have no location to report, so the
		// finding is about the document.
		return []Finding{newFinding(FieldDocument, err.Error())}
	}
	var out []Finding
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		// Only emit leaf errors (those with no children carry the concrete
		// reason); intermediate nodes just describe the path.
		if len(e.Causes) == 0 {
			field := fieldPath(e.InstanceLocation)
			reason := e.ErrorKind.LocalizedString(printer)
			// A `pattern` violation is the one schema kind whose base message
			// is a raw regex with no rule and no example, while the enum
			// violations beside it read perfectly. Append the gloss; never
			// replace the library's text, which carries the offending value
			// and the authoritative regex. See pattern.go.
			if pk, ok := e.ErrorKind.(*kind.Pattern); ok {
				reason += patternAdvice(pk)
			}
			out = append(out, newFinding(field,
				fmt.Sprintf("%s: %s", field, reason)))
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(out) == 0 {
		out = append(out, newFinding(fieldPath(ve.InstanceLocation), ve.Error()))
	}
	return out
}
