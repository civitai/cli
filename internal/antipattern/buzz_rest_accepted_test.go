package antipattern

import (
	"os"
	"path/filepath"
	"testing"
)

// buzzRestLines are reads of `GET /api/v1/blocks/buzz` in the shapes an app
// writes them. The route is LIVE (it returns the same `{ blue, green, yellow }`
// projection as the bridge's balance read), so none of them may be flagged.
//
// The first four are exactly the shapes the removed `buzz-rest` rule matched:
// each ends the `/buzz` segment with a different character of its terminating
// class (quote, backtick, `?`, end of line).
var buzzRestLines = []string{
	`const bal = await fetch('/api/v1/blocks/buzz');`,
	"const res = await fetch(`${base}/api/v1/blocks/buzz`, { headers });",
	`return fetch("/api/v1/blocks/buzz?fresh=1");`,
	`const BUZZ_URL = origin + /api/v1/blocks/buzz`,
	// The prefix-less SDK call site, and prose naming the route.
	`return app.site.get<{ blue: number; green: number; yellow: number }>('blocks/buzz');`,
	"Read the balance with `GET /api/v1/blocks/buzz`.",
}

// TestBuzzRestIsAccepted is the regression guard for the removed `buzz-rest`
// rule: a REST read of the buzz balance must produce NO finding, through the
// scanner's own entrypoint and in every file type it reads.
//
// Both halves carry a positive control, because "no finding" is also what a
// scanner that read nothing reports.
func TestBuzzRestIsAccepted(t *testing.T) {
	for _, r := range Rules() {
		for _, line := range buzzRestLines {
			if r.Pattern.MatchString(line) {
				t.Errorf("rule %q flags a read of the live /api/v1/blocks/buzz route:\n  rule:    %s\n  matched: %s",
					r.ID, r.Pattern, line)
			}
		}
	}

	root := t.TempDir()
	body := ""
	for _, line := range buzzRestLines {
		body += line + "\n"
	}
	// One file per corpus family: code, data, prose.
	for _, rel := range []string{"src/buzz.ts", "src/routes.json", "README.md"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// Control: a line a remaining rule matches, in the same tree.
	writeFinding(t, root, "src/control.ts")

	findings, err := ScanDir(root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	control := 0
	for _, f := range findings {
		if filepath.ToSlash(f.File) == "src/control.ts" {
			control++
			continue
		}
		t.Errorf("%s:%d — rule %q fired on a read of the live buzz route: %s", f.File, f.Line, f.Rule.ID, f.Text)
	}
	if control != 1 {
		t.Fatalf("the control file produced %d finding(s), want 1 — the scan observed nothing, so the "+
			"absences above are vacuous", control)
	}
}
