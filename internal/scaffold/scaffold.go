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

	"github.com/civitai/cli/internal/blockproto"
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
	// for the money path (estimate → consent → submit → poll → Buzz spend). It
	// is the React alternative to PageElements.
	PageMoney Template = "page-money"
	// PageElements is a vite+TS page block with no UI framework: `@civitai/sdk`
	// for the host bridge and `@civitai/components` `<civitai-*>` custom
	// elements (themed by `@civitai/theme`) for the UI. It mirrors the
	// `civitai-block-starter` starter in civitai/civitai-app-starters,
	// declared as a page rather than a model slot like every template here.
	PageElements Template = "page-elements"
)

// DefaultTemplate is what `civitai app init` and `civitai app create` scaffold
// when no --template is given, and what their interactive prompt pre-selects.
//
// 🔴 ONE CONSTANT FOR BOTH COMMANDS. They used to default differently (init to
// `static`, create to `page-money`), each spelled at its own flag definition;
// web components became the default for both at once, and a single constant is
// what keeps a later change from moving one and not the other.
const DefaultTemplate = PageElements

// AllTemplates lists the templates available to `civitai app init`, default
// first — the same order the --template usage string and the interactive prompt
// list them in.
func AllTemplates() []Template { return []Template{PageElements, PageMoney, PageVite, Static} }

// NeedsHarness reports whether the template's dev loop runs against a local mock
// host (`npm run dev:harness`) rather than a plain `dev` (which renders blank
// without a host).
func (t Template) NeedsHarness() bool { return t == PageMoney || t == PageElements }

// ReadyAckPath is where this template's rendered tree carries the canonical
// block -> host ready-ack emitter (blockproto.ReadyAckSource), or "" when the
// template does not need one.
//
// Every template declares a `page` surface, and the host will not reveal a
// page app until it posts `BLOCK_READY`. page-money and page-elements get that
// for free — `@civitai/blocks-react`'s and `@civitai/sdk`'s iframe transports
// ack internally — so shipping a second emitter there would double-post into a
// rate-limited inbound channel for no gain. The two SDK-free templates have to
// say hello themselves.
//
// The path is relative to the project root and uses forward slashes.
// `internal/scaffold/ready_ack_contract_test.go` enumerates AllTemplates() and
// fails when a page template that carries no SDK returns "" here, so a future
// template cannot quietly opt out.
func (t Template) ReadyAckPath() string {
	switch t {
	case Static:
		return blockproto.ReadyAckFilename
	case PageVite:
		return "src/" + blockproto.ReadyAckFilename
	default:
		return ""
	}
}

// NeedsInstall reports whether the template scaffolds a package.json — i.e.
// whether the platform build will run an install step for it, and therefore
// whether the author MUST commit a lockfile. Static blocks are served as-is and
// never install.
func (t Template) NeedsInstall() bool { return t != Static }

// ParseTemplate validates a template name.
func ParseTemplate(s string) (Template, error) {
	switch Template(s) {
	case Static:
		return Static, nil
	case PageVite:
		return PageVite, nil
	case PageMoney:
		return PageMoney, nil
	case PageElements:
		return PageElements, nil
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
	// Tagline is the manifest's store tagline. Leave it EMPTY and Render derives
	// one from Name (TaglineFromName) — that is the normal path and the only one
	// `civitai app init` takes, so a caller never has to know this field exists.
	// Setting it explicitly overrides the derivation; it is not validated here,
	// because `app init` validates the manifest it writes.
	Tagline string
}

// outputName maps an embedded template filename to its on-disk name.
// `.tmpl` is stripped; some names are rewritten because go:embed cannot embed
// files whose names begin with a dot:
//   - `gitignore`  -> `.gitignore`
//   - `env.<any>`  -> `.env.<any>` (e.g. `env.development`, `env.production`,
//     `env.example`)
//
// 🔴 THE DOTENV REWRITE IS A RULE, NOT THE THREE NAMES IT HAPPENS TO MATCH
// TODAY. It enumerated `env.development` / `env.production` / `env.example`
// until #380, and the enumeration was a live hazard rather than a tidiness
// point: `internal/pkgzip` decides what to upload from a base name starting
// with `.env`, so an `env.sample.tmpl` would have rendered as the UNDOTTED
// `env.sample`, sailed past `isExcludedFile`, and shipped to the platform and
// to a human moderator reviewer — while `.env.sample` sits on that package's
// keptEnvFiles allow-list for a name the scaffold could not even emit.
// `TestUploadedDotenvNeverPointsASecretAtAnUploadedFile` fails on any UNDOTTED
// `env.*` file that reaches the package, so the two ends stay tied.
func outputName(rel string) string {
	rel = strings.TrimSuffix(rel, ".tmpl")
	base := filepath.Base(rel)
	switch {
	case base == "gitignore":
		rel = filepath.Join(filepath.Dir(rel), ".gitignore")
	case strings.HasPrefix(base, "env."):
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

	// 🔴 THE TEMPLATES SEE `Data` AND NOTHING ELSE, and that is still the whole
	// story. There used to be an unexported `renderData` WRAPPER carrying a
	// derived `HookIndex` — a 36-row markdown table of every
	// `@civitai/blocks-react` hook, generated from a Go table (`hooks.go`) and
	// rendered into the page-money README. It was retired: the hosted developer
	// reference (spelled once, in `README.md.tmpl`) is canonical, the README now
	// points at it, and keeping a second copy in this repo was a
	// drift surface for a route no measured trial took (two graded dogfood runs
	// read the scaffolded README zero times and fetched the hosted page four).
	// `Data` is a struct, so a template still referencing `{{ .HookIndex }}`
	// fails execution with `can't evaluate field HookIndex` rather than rendering
	// a blank — the removal cannot be half-done.
	//
	// 🔴 `Tagline` IS DERIVED HERE AND IS NOT A RETURN OF THAT WRAPPER. It fills a
	// FIELD ON `Data` rather than a wrapper around it, so the sentence above stays
	// literally true and every caller keeps working unchanged. And it is not the
	// thing the wrapper was retired for: `HookIndex` duplicated a document that
	// lives somewhere else, so the two could drift; nothing anywhere else holds a
	// copy of a scaffolded app's tagline. Deriving it in Render rather than at the
	// call site is what makes it ONE rule — every entry point, including the tests
	// that render a template directly, gets a schema-valid tagline without
	// knowing it has to ask for one.
	view := data
	if strings.TrimSpace(view.Tagline) == "" {
		view.Tagline = TaglineFromName(view.Name)
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
		if err := t.Execute(&buf, view); err != nil {
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

	// The ready-ack emitter is NOT a template file. It is copied verbatim from
	// blockproto so the repo holds one authority for the handshake and every
	// scaffolded app ships byte-identical bytes — no `text/template` pass, so
	// nothing in it can be reinterpreted as a template action.
	if rel := tmpl.ReadyAckPath(); rel != "" {
		out := filepath.Join(destDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, blockproto.ReadyAckSource(), 0o644); err != nil {
			return nil, err
		}
		written = append(written, out)
	}

	sort.Strings(written)
	return written, nil
}

// NotEmptyRemedy is the actionable half of the refusal to scaffold into a
// directory that already holds files (issue #260, item 7).
//
// The refusal itself was already right — clobbering an author's directory is
// not recoverable — but it named no way forward, while the troubleshooting index
// carried the remedy the CLI did not print. AGENTS.md's house rule is that an
// error names the next command to run, so the remedy moved into the message and
// the docs agree with the binary. (The index is now published at
// developer.civitai.com/site/guide/cli-troubleshooting rather than being a README
// section; this message is the surface that has to be right either way, which was
// the point of moving the remedy into it.)
//
// 🔴 IT ENDS BY SAYING THERE IS NO `--force`, and that sentence is the point
// rather than a hedge. Without it the natural next move on reading "refusing to
// overwrite" is to go looking for the override flag — through `--help`, then
// the README — and find nothing, which reads as a missing feature rather than
// as a decision. There is no `--force` because overwriting a directory the user
// already has is destructive and unrecoverable; if one is ever added, this
// constant is the one place that claim has to change.
//
// It is a named constant so the message and the tests that pin it cannot
// drift: the guard derives what it expects FROM this value rather than
// spelling a copy of it.
const NotEmptyRemedy = "refusing to overwrite. Scaffold somewhere else (`--dir <new path>`, or a different name), " +
	"or remove the directory first — there is no --force"

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
		return fmt.Errorf("%s is not empty — %s", dir, NotEmptyRemedy)
	}
	return nil
}
