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
	// for the money path (estimate → consent → submit → poll → Buzz spend).
	PageMoney Template = "page-money"
)

// AllTemplates lists the templates available to `civitai app init`.
func AllTemplates() []Template { return []Template{Static, PageVite, PageMoney} }

// SDKTemplates are templates whose dev loop needs a mock host (`dev:harness`)
// rather than a plain `dev` (which renders blank without a host).
func (t Template) NeedsHarness() bool { return t == PageMoney }

// ReadyAckPath is where this template's rendered tree carries the canonical
// block -> host ready-ack emitter (blockproto.ReadyAckSource), or "" when the
// template does not need one.
//
// Every template declares a `page` surface, and the host will not reveal a
// page app until it posts `BLOCK_READY`. page-money gets that for free —
// `@civitai/blocks-react`'s IframeTransport acks internally — so shipping a
// second emitter there would double-post into a rate-limited inbound channel
// for no gain. The two SDK-free templates have to say hello themselves.
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
// not recoverable — but it named no way forward, while the README's
// Troubleshooting section carried the remedy the CLI did not print. AGENTS.md's
// house rule is that an error names the next command to run, so the remedy
// moved into the message and the README now agrees with the binary.
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
