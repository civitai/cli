package antipattern

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScanDir is the two-directional correctness proof for the anti-pattern
// gate: a fixture that uses a dead/superseded surface FAILS (naming the file,
// pattern, and replacement), a clean fixture PASSES, and — critically — a
// fixture full of legitimately-REST endpoints that have no bridge does NOT
// false-flag.
func TestScanDir(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		wantRule  string // "" means expect zero findings
		wantFile  string // substring the offending file path must contain
		wantInMsg string // substring the formatted message / replacement must contain
	}{
		{
			name: "clean scaffold passes",
			dir:  "clean",
		},
		{
			name: "legit unbridged REST routes are not flagged",
			dir:  "legit-rest",
		},
		{
			name: "node_modules is not descended into",
			dir:  "skips-deps",
		},
		{
			name:      "removed buzz REST route fails",
			dir:       "buzz-rest",
			wantRule:  "buzz-rest",
			wantFile:  "buzz.ts",
			wantInMsg: "useBuzzBalance",
		},
		{
			name:      "superseded shared-storage REST route fails",
			dir:       "shared-storage-rest",
			wantRule:  "shared-storage-rest",
			wantFile:  "store.ts",
			wantInMsg: "SHARED_APPEND",
		},
		{
			name:      "deprecated blocks-cli reference fails",
			dir:       "blocks-cli",
			wantRule:  "deprecated-blocks-cli",
			wantFile:  "README.md",
			wantInMsg: "civitai app",
		},
		{
			// The pre-#206 page template shape: an app that sizes itself to
			// content on a surface that does not size to content.
			name:      "RESIZE_IFRAME posted from a page app fails",
			dir:       "resize-iframe",
			wantRule:  "resize-iframe-page",
			wantFile:  "App.jsx",
			wantInMsg: "BLOCK_READY",
		},
		{
			// The false-positive direction, which matters more. This fixture
			// carries the message name in every NON-CODE shape at once: markdown
			// backticks in a README, prose with straight DOUBLE quotes in a .txt,
			// and a double-quoted JSON key in a handler table. None of them is a
			// call — the first is literally documentation telling the author not
			// to post it — and flagging any one fails a correct project.
			name: "RESIZE_IFRAME named in docs, prose and JSON data is not flagged",
			dir:  "resize-iframe-doc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings, err := ScanDir(filepath.Join("testdata", tt.dir))
			if err != nil {
				t.Fatalf("ScanDir(%s): %v", tt.dir, err)
			}

			if tt.wantRule == "" {
				if len(findings) != 0 {
					t.Fatalf("expected a clean scan, got %d finding(s):\n%s", len(findings), Format(findings))
				}
				return
			}

			if len(findings) == 0 {
				t.Fatalf("expected rule %q to fire, got zero findings", tt.wantRule)
			}
			// The offending finding must exist with the right rule + file.
			var hit *Finding
			for i := range findings {
				if findings[i].Rule.ID == tt.wantRule {
					hit = &findings[i]
					break
				}
			}
			if hit == nil {
				t.Fatalf("expected rule %q, got findings:\n%s", tt.wantRule, Format(findings))
			}
			if !strings.Contains(hit.File, tt.wantFile) {
				t.Errorf("finding file = %q, want it to contain %q", hit.File, tt.wantFile)
			}
			if hit.Line == 0 {
				t.Errorf("finding has no line number")
			}
			if hit.Rule.Replacement == "" {
				t.Errorf("finding rule %q has an empty Replacement — the message must tell the author what to use instead", hit.Rule.ID)
			}
			if strings.TrimSpace(hit.Rule.What) == "" {
				t.Errorf("finding rule %q has an empty What — the report would name the file and the fix "+
					"but never say what was wrong", hit.Rule.ID)
			}
			// The formatted report must name the file, the pattern, and the fix.
			msg := Format(findings)
			for _, want := range []string{hit.File, tt.wantInMsg, tt.wantRule} {
				if !strings.Contains(msg, want) {
					t.Errorf("Format() output missing %q; got:\n%s", want, msg)
				}
			}
		})
	}
}

// TestLegitRoutesNeverMatchAnyRule is a focused regression guard: the exact
// unbridged routes the real scaffolds rely on must never match ANY rule, so the
// gate can never false-fail a correct scaffold.
func TestLegitRoutesNeverMatchAnyRule(t *testing.T) {
	legit := []string{
		"await fetch('/api/v1/blocks/models?limit=20')",
		"await fetch('/api/v1/blocks/images')",
		"fetch(`${base}/api/v1/blocks/me`)",
		"fetch(`${origin}/api/v1/blocks/dev-token`, { method: 'POST' })",
		"fetch(`${base}/api/trpc/buzz.getBuzzAccount`)",
	}
	for _, r := range Rules() {
		for _, line := range legit {
			if r.Pattern.MatchString(line) {
				t.Errorf("rule %q FALSE-FLAGGED a legit line:\n  rule:    %s\n  matched: %s", r.ID, r.Pattern, line)
			}
		}
	}
}

// TestBlocksCliBoundary guards the deprecated-blocks-cli rule against matching a
// wider package name: `@civitai/blocks-cli` (and a versioned/subpath form) must
// fire, but a hypothetical sibling `@civitai/blocks-client` must NOT.
func TestBlocksCliBoundary(t *testing.T) {
	var rule Rule
	for _, r := range Rules() {
		if r.ID == "deprecated-blocks-cli" {
			rule = r
		}
	}
	if rule.Pattern == nil {
		t.Fatal("deprecated-blocks-cli rule not found")
	}

	cases := []struct {
		line string
		want bool
	}{
		{`import x from '@civitai/blocks-cli';`, true},
		{`npx @civitai/blocks-cli submit`, true},
		{`"@civitai/blocks-cli": "^1.2.3"`, true},
		{`npm i @civitai/blocks-cli@1.2.3`, true},
		{`require('@civitai/blocks-cli/bin')`, true},
		{`@civitai/blocks-cli`, true}, // bare, end-of-line
		// Must NOT false-flag a wider package name.
		{`import x from '@civitai/blocks-client';`, false},
		{`"@civitai/blocks-client": "^1.0.0"`, false},
		{`@civitai/blocks-client-utils`, false},
	}
	for _, c := range cases {
		if got := rule.Pattern.MatchString(c.line); got != c.want {
			t.Errorf("match(%q) = %v, want %v (pattern %s)", c.line, got, c.want, rule.Pattern)
		}
	}
}

// TestResizeIframeBoundary is the resize-iframe-page rule's control pair. The
// rule exists to catch a DEAD HOST MESSAGE, and the population it must never
// touch is prose: this repo's own README and both scaffold READMEs discuss
// `RESIZE_IFRAME` by name precisely to tell authors not to post it, and a gate
// that fails the documentation of its own rule is a false positive at a correct
// project.
func TestResizeIframeBoundary(t *testing.T) {
	var rule Rule
	for _, r := range Rules() {
		if r.ID == "resize-iframe-page" {
			rule = r
		}
	}
	if rule.Pattern == nil {
		t.Fatal("resize-iframe-page rule not found")
	}

	cases := []struct {
		line string
		want bool
	}{
		// The real emitting shapes, single- and double-quoted.
		{`window.parent.postMessage({ type: 'RESIZE_IFRAME', height: h }, '*');`, true},
		{`window.parent.postMessage({ type: "RESIZE_IFRAME", height: h }, "*");`, true},
		{`if (msg.type === 'RESIZE_IFRAME') return;`, true},
		{`const RESIZE = 'RESIZE_IFRAME';`, true},
		// Prose — the direction that must stay silent.
		{"`RESIZE_IFRAME` is **not** part of a page app's protocol.", false},
		{"Do not post RESIZE_IFRAME from a page app.", false},
		{`// RESIZE_IFRAME is N/A for PageBlockHost — do not send it.`, false},
		// MISMATCHED quote pairs are not string literals in any language. This
		// pins the "matching pair" claim: a relaxed `['"]RESIZE_IFRAME['"]`
		// accepts both of these and passes every other case above.
		{`var x = 'RESIZE_IFRAME";`, false},
		{`var x = "RESIZE_IFRAME';`, false},
		// Neighbouring identifiers must not be dragged in.
		{`useBlockResize();`, false},
		{`"description": "Whether the host honours RESIZE_IFRAME messages."`, false},
	}
	for _, c := range cases {
		if got := rule.Pattern.MatchString(c.line); got != c.want {
			t.Errorf("match(%q) = %v, want %v (pattern %s)", c.line, got, c.want, rule.Pattern)
		}
	}

	// The rule must be scoped to CODE. Without Exts it fires on prose and on a
	// JSON handler table, which is how it came to flag its own documentation.
	if rule.Exts == nil {
		t.Fatal("the resize-iframe-page rule has no Exts — a message NAME is quotable in prose and " +
			"usable as a JSON key, so an unscoped rule fails a correct project's README")
	}
	for _, ext := range []string{".md", ".txt", ".json", ".css"} {
		if rule.appliesTo(ext) {
			t.Errorf("the rule applies to %s — that corpus is prose/data, not calls", ext)
		}
	}
	// Positive control on the same predicate: it must still reach real code, or
	// "does not apply to .md" is satisfied by a rule that applies to nothing.
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx", ".html"} {
		if !rule.appliesTo(ext) {
			t.Errorf("the rule does NOT apply to %s — that is where the dead post lives", ext)
		}
	}
}

// TestRuleExtScopeDefaultsToEverything pins that adding Exts to one rule did not
// silently narrow the others: a nil Exts must still mean "the whole corpus", or
// the package-name and route rules stop catching a documented dead workflow.
func TestRuleExtScopeDefaultsToEverything(t *testing.T) {
	unscoped := 0
	for _, r := range Rules() {
		if r.Exts != nil {
			continue
		}
		unscoped++
		for _, ext := range []string{".md", ".txt", ".json", ".js", ".html"} {
			if !r.appliesTo(ext) {
				t.Errorf("rule %q has no Exts but does not apply to %s — nil must mean every extension", r.ID, ext)
			}
		}
	}
	if unscoped == 0 {
		t.Fatal("no rule is unscoped — this control observed nothing")
	}
}

// TestScanDirFromEnv is the CI entrypoint. The scaffold-currency job scaffolds a
// template to disk, sets CIVITAI_ANTIPATTERN_SCAN_DIR to it, and runs this test;
// any finding fails the job. It SKIPS when the env var is unset so the default
// `go test ./...` stays hermetic.
func TestScanDirFromEnv(t *testing.T) {
	dir := os.Getenv("CIVITAI_ANTIPATTERN_SCAN_DIR")
	if dir == "" {
		t.Skip("set CIVITAI_ANTIPATTERN_SCAN_DIR=<scaffolded app dir> to scan it (the CI scaffold-currency job does)")
	}
	findings, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("scanning %s: %v", dir, err)
	}
	if len(findings) > 0 {
		t.Fatalf("%s", Format(findings))
	}
	t.Logf("scaffold-currency: no anti-patterns found in %s", dir)
}
