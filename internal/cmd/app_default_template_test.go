package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The DEFAULT scaffold template: web components (`page-elements`).
//
// 🔴 THE EXPECTATION IS A LITERAL, NOT scaffold.DefaultTemplate. A test that
// compared the flag's default against the constant that sets it would pass
// whatever the constant said — including `page-money`, the value this change
// moved away from. "page-elements" is spelled here so pointing the constant (or
// either flag) anywhere else goes red.
//
// Matrix: these tests are red at origin/main — `init` defaulted to `static` and
// `create` to `page-money`, and no `page-elements` template existed — and green
// on the branch that introduced them. The mutation `DefaultTemplate =
// PageMoney` turns every test below red on its own assertion.
const wantDefaultTemplate = "page-elements"

// TestScaffoldCommandsDefaultToPageElements pins the flag default of BOTH
// scaffolders. They share one RunE and used to default differently; a default
// that moves on one command and not the other is the regression this catches.
func TestScaffoldCommandsDefaultToPageElements(t *testing.T) {
	for _, sub := range []string{"init", "create"} {
		c, _, err := NewRootCmd().Find([]string{"app", sub})
		if err != nil {
			t.Fatalf("app %s: %v", sub, err)
		}
		f := c.Flags().Lookup("template")
		if f == nil {
			t.Fatalf("`app %s` has no --template flag", sub)
		}
		if f.DefValue != wantDefaultTemplate {
			t.Errorf("`civitai app %s` --template defaults to %q, want %q", sub, f.DefValue, wantDefaultTemplate)
		}
		if !strings.HasPrefix(f.Usage, "project template: "+wantDefaultTemplate+" |") {
			t.Errorf("`civitai app %s` --template usage should list the default first, got %q", sub, f.Usage)
		}
	}
}

// TestPlainAppInitScaffoldsPageElements runs the user's actual command — no
// --template, --yes so no prompt — and checks what lands on disk, not just the
// summary line: a summary naming the template is a claim; the dependencies, the
// entry point and the absence of the vendored emitter are the template.
func TestPlainAppInitScaffoldsPageElements(t *testing.T) {
	for _, sub := range []string{"init", "create"} {
		t.Run(sub, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "my-block")
			stdout, _, err := run(t, "app", sub, "my-block", dest, "--yes")
			if err != nil {
				t.Fatalf("app %s: %v\n%s", sub, err, stdout)
			}
			if !strings.Contains(stdout, "("+wantDefaultTemplate+")") {
				t.Errorf("plain `civitai app %s` should scaffold %s:\n%s", sub, wantDefaultTemplate, stdout)
			}

			// What makes it the web-components template: the bridge, the elements
			// and the theme, and no UI framework.
			raw, err := os.ReadFile(filepath.Join(dest, "package.json"))
			if err != nil {
				t.Fatalf("the default template should ship a package.json: %v", err)
			}
			var pkg struct {
				Dependencies    map[string]string `json:"dependencies"`
				DevDependencies map[string]string `json:"devDependencies"`
				Scripts         map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal(raw, &pkg); err != nil {
				t.Fatalf("package.json: %v", err)
			}
			for _, dep := range []string{"@civitai/sdk", "@civitai/components", "@civitai/theme", "@civitai/app-sdk"} {
				if _, ok := pkg.Dependencies[dep]; !ok {
					t.Errorf("default scaffold is missing dependency %s: %v", dep, pkg.Dependencies)
				}
			}
			for _, framework := range []string{"react", "react-dom", "@civitai/blocks-react"} {
				if _, ok := pkg.Dependencies[framework]; ok {
					t.Errorf("default scaffold depends on %s — the web-components template has no UI framework", framework)
				}
			}
			for _, script := range []string{"dev:harness", "test", "typecheck", "build"} {
				if _, ok := pkg.Scripts[script]; !ok {
					t.Errorf("default scaffold has no %q script: %v", script, pkg.Scripts)
				}
			}
			for _, f := range []string{"src/block.ts", "src/main.ts", "test/block.test.ts", "index.html"} {
				if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err != nil {
					t.Errorf("default scaffold is missing %s: %v", f, err)
				}
			}
			// @civitai/sdk performs the ready-ack, so a vendored emitter would be a
			// second handshake racing it.
			for _, f := range []string{"civitai-host.js", "src/civitai-host.js"} {
				if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err == nil {
					t.Errorf("default scaffold ships %s beside @civitai/sdk — two handshakes race", f)
				}
			}

			// The next steps are the elements ones: the harness, the file to edit,
			// and the pointer to the React alternative.
			for _, want := range []string{"npm run dev:harness", "src/block.ts", "--template page-money"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("plain `civitai app %s` next steps should mention %q:\n%s", sub, want, stdout)
				}
			}
			assertScaffoldValid(t, dest)
		})
	}
}

// TestScaffoldPromptPreselectsAndListsPageElementsFirst covers the interactive
// path: the form is handed the default to pre-select, and that default is the
// FIRST option a reader sees.
func TestScaffoldPromptPreselectsAndListsPageElementsFirst(t *testing.T) {
	for _, sub := range []string{"init", "create"} {
		t.Run(sub, func(t *testing.T) {
			var rec promptRecorder
			// Reply with no template, so what gets built is the pre-selected one.
			stubPrompt(t, &rec, scaffoldInputs{name: "Prompted Block", template: ""})
			tmp := t.TempDir()
			chdir(t, tmp)
			stdout, _, err := run(t, "app", sub)
			if err != nil {
				t.Fatalf("app %s on a TTY: %v\n%s", sub, err, stdout)
			}
			if rec.calls != 1 {
				t.Fatalf("the prompt should run once with no name given, ran %d time(s)", rec.calls)
			}
			if rec.defaultTemplate != wantDefaultTemplate {
				t.Errorf("`civitai app %s`'s prompt pre-selects %q, want %q", sub, rec.defaultTemplate, wantDefaultTemplate)
			}
			if !strings.Contains(stdout, "("+wantDefaultTemplate+")") {
				t.Errorf("accepting the prompt's pre-selection should scaffold %s:\n%s", wantDefaultTemplate, stdout)
			}
		})
	}

	opts := scaffoldTemplateOptions()
	if len(opts) < 4 {
		t.Fatalf("CONTROL failure, not a finding: the prompt lists %d template option(s), want 4", len(opts))
	}
	if opts[0].Value != wantDefaultTemplate {
		t.Errorf("the prompt's first option is %q, want the default %q", opts[0].Value, wantDefaultTemplate)
	}
	if !strings.Contains(opts[1].Key, "React alternative") || opts[1].Value != "page-money" {
		t.Errorf("the prompt's second option should be page-money, described as the React alternative: %q=%q",
			opts[1].Key, opts[1].Value)
	}
}
