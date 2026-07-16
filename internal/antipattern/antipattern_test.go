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
