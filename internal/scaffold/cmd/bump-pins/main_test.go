package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// TestDesiredPin locks the "pin the minor" contract shared with the guard.
func TestDesiredPin(t *testing.T) {
	cases := []struct {
		published string
		want      string
		wantErr   bool
	}{
		{"0.25.3", "^0.25.0", false},
		{"0.25.0", "^0.25.0", false},
		{"1.4.0", "^1.4.0", false},
		{"1.4.9", "^1.4.0", false},
		{"0.0.7", "^0.0.7", false}, // 0.0.z: caret locks the PATCH, so pin the patch (self-consistent with CaretAdmits)
		{"", "", true},
		{"1.2", "", true},
		{"vNope", "", true},
	}
	for _, c := range cases {
		got, err := scaffold.DesiredPin(c.published)
		if c.wantErr {
			if err == nil {
				t.Errorf("DesiredPin(%q): expected error, got %q", c.published, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("DesiredPin(%q): unexpected error: %v", c.published, err)
			continue
		}
		if got != c.want {
			t.Errorf("DesiredPin(%q) = %q, want %q", c.published, got, c.want)
		}
	}
}

// rewritePin is the load-bearing surgical replace — cover both literal shapes,
// the already-current no-op, and the absent-pin no-op.
func TestRewritePin(t *testing.T) {
	t.Run("json bumps only the matching pin", func(t *testing.T) {
		in := `{
  "dependencies": {
    "@civitai/app-sdk": "^0.24.0",
    "@civitai/blocks-react": "^0.29.0",
    "react": "^19.0.0"
  }
}`
		out, old, did := rewritePin(in, "@civitai/app-sdk", "0.25.0", jsonPin)
		if !did || old != "0.24.0" {
			t.Fatalf("expected change from 0.24.0, got did=%v old=%q", did, old)
		}
		if !strings.Contains(out, `"@civitai/app-sdk": "^0.25.0"`) {
			t.Errorf("app-sdk not bumped:\n%s", out)
		}
		// unrelated versions untouched
		if !strings.Contains(out, `"@civitai/blocks-react": "^0.29.0"`) ||
			!strings.Contains(out, `"react": "^19.0.0"`) {
			t.Errorf("blanket-replaced an unrelated version:\n%s", out)
		}
	})

	t.Run("prose bumps the @pkg@^ver form", func(t *testing.T) {
		in := "requires `@civitai/app-sdk@^0.24.0` + `@civitai/blocks-react@^0.29.0` (pinned)."
		out, old, did := rewritePin(in, "@civitai/blocks-react", "0.30.0", prosePin)
		if !did || old != "0.29.0" {
			t.Fatalf("expected change from 0.29.0, got did=%v old=%q", did, old)
		}
		if !strings.Contains(out, "@civitai/blocks-react@^0.30.0") {
			t.Errorf("blocks-react prose not bumped:\n%s", out)
		}
		if !strings.Contains(out, "@civitai/app-sdk@^0.24.0") {
			t.Errorf("app-sdk prose should be untouched:\n%s", out)
		}
	})

	t.Run("prose does not match a subpath import", func(t *testing.T) {
		in := "import from `@civitai/blocks-react/ui` and `@civitai/blocks-react/testing`."
		_, _, did := rewritePin(in, "@civitai/blocks-react", "0.30.0", prosePin)
		if did {
			t.Errorf("should not have matched a /ui or /testing subpath import")
		}
	})

	t.Run("already current is a no-op", func(t *testing.T) {
		in := `"@civitai/app-sdk": "^0.25.0"`
		out, old, did := rewritePin(in, "@civitai/app-sdk", "0.25.0", jsonPin)
		if did {
			t.Errorf("expected no-op when already current")
		}
		if old != "0.25.0" || out != in {
			t.Errorf("no-op mangled content: old=%q out=%q", old, out)
		}
	})

	t.Run("absent pin is a no-op", func(t *testing.T) {
		in := `"react": "^19.0.0"`
		out, old, did := rewritePin(in, "@civitai/app-sdk", "0.25.0", jsonPin)
		if did || old != "" || out != in {
			t.Errorf("expected clean no-op for absent pin: did=%v old=%q", did, old)
		}
	})
}

// fakeTokens is the injected design-token read (no network). It returns a small
// but non-empty set — non-empty matters, because run() refuses to write an empty
// ledger and an empty set would make the design-token guard vacuous.
func fakeTokens(pkg, version string) ([]string, error) {
	if pkg != scaffold.DesignTokenPkg {
		return nil, fmt.Errorf("unexpected token fetch for %s", pkg)
	}
	return []string{"--civitai-color-text", "--civitai-color-surface", "--civitai-radius"}, nil
}

// TestRunRewritesAllThreeFilesAndIsIdempotent exercises the full run() against a
// temp-dir fixture mirroring the three literal sites, with an injected
// "published" version (no network).
func TestRunRewritesAllThreeFilesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)

	// Inject npm: app-sdk latest 0.25.4, blocks-react latest 0.30.1.
	fetch := func(pkg string) (string, error) {
		switch pkg {
		case "@civitai/app-sdk":
			return "0.25.4", nil
		case "@civitai/blocks-react":
			return "0.30.1", nil
		}
		return "", scaffold.ErrPkgNotFound
	}

	var buf bytes.Buffer
	changes, err := run(dir, fetch, fakeTokens, false, &buf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// 2 packages × 3 pin sites = 6 rewrites, + the design-token ledger.
	if len(changes) != 7 {
		t.Fatalf("expected 7 changes (6 pin sites + the ledger), got %d:\n%s", len(changes), buf.String())
	}

	// The ledger must have been written, and must record the pin this run set.
	ledger := readFile(t, dir, scaffold.DesignTokenLedgerFile)
	l, err := scaffold.ParseDesignTokenLedger([]byte(ledger))
	if err != nil {
		t.Fatalf("bump-pins wrote an unparseable ledger: %v\n%s", err, ledger)
	}
	if l.Pkg != scaffold.DesignTokenPkg || l.Pin != "^0.30.0" || l.Version != "0.30.1" {
		t.Errorf("ledger provenance = %+v, want pkg=%s pin=^0.30.0 version=0.30.1", l, scaffold.DesignTokenPkg)
	}
	if len(l.Tokens) != 3 {
		t.Errorf("ledger holds %d token(s), want 3: %v", len(l.Tokens), l.Tokens)
	}

	pkgJSON := readFile(t, dir, pkgJSONFile)
	readme := readFile(t, dir, readmeFile)
	test := readFile(t, dir, testFile)

	assertContains(t, pkgJSON, `"@civitai/app-sdk": "^0.25.0"`)
	assertContains(t, pkgJSON, `"@civitai/blocks-react": "^0.30.0"`)
	assertContains(t, readme, "@civitai/app-sdk@^0.25.0")
	assertContains(t, readme, "@civitai/blocks-react@^0.30.0")
	assertContains(t, test, `mustContain(t, pkg, `+"`"+`"@civitai/app-sdk": "^0.25.0"`+"`"+`)`)
	assertContains(t, test, `mustContain(t, pkg, `+"`"+`"@civitai/blocks-react": "^0.30.0"`+"`"+`)`)
	// unrelated deps untouched
	assertContains(t, pkgJSON, `"react": "^19.0.0"`)

	// Second run is a no-op (idempotent).
	buf.Reset()
	changes2, err := run(dir, fetch, fakeTokens, false, &buf)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(changes2) != 0 {
		t.Fatalf("expected idempotent no-op, got %d changes:\n%s", len(changes2), buf.String())
	}
	if !strings.Contains(buf.String(), "nothing to do") {
		t.Errorf("expected 'nothing to do' on the idempotent run, got:\n%s", buf.String())
	}
}

// TestRunCheckWritesNothing verifies --check reports needed bumps but leaves the
// files untouched.
func TestRunCheckWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	before := readFile(t, dir, pkgJSONFile)

	fetch := func(pkg string) (string, error) {
		switch pkg {
		case "@civitai/app-sdk":
			return "0.25.0", nil
		case "@civitai/blocks-react":
			return "0.30.0", nil
		}
		return "", scaffold.ErrPkgNotFound
	}

	var buf bytes.Buffer
	changes, err := run(dir, fetch, fakeTokens, true /*check*/, &buf)
	if err != nil {
		t.Fatalf("run --check: %v", err)
	}
	if len(changes) == 0 {
		t.Fatalf("expected --check to report needed bumps")
	}
	if got := readFile(t, dir, pkgJSONFile); got != before {
		t.Errorf("--check must not modify files, but package.json.tmpl changed")
	}
	if !strings.Contains(buf.String(), "would bump") {
		t.Errorf("expected 'would bump' phrasing under --check, got:\n%s", buf.String())
	}
}

// TestRunSkipsOnFetchError confirms a transient/missing npm lookup skips the
// package without failing the command or touching files.
func TestRunSkipsOnFetchError(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)
	before := readFile(t, dir, pkgJSONFile)

	fetch := func(pkg string) (string, error) {
		return "", scaffold.ErrPkgNotFound // both packages unresolved
	}

	var buf bytes.Buffer
	changes, err := run(dir, fetch, fakeTokens, false, &buf)
	if err != nil {
		t.Fatalf("run must not fail on a skipped package: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes when every lookup is skipped, got %d", len(changes))
	}
	if got := readFile(t, dir, pkgJSONFile); got != before {
		t.Errorf("skipped run must not modify files")
	}
}

// TestRunSkipsThePinBumpWhenTheTokenReadFails is the coupling this wiring
// exists for. Writing the pack's PIN without regenerating the design-token
// ledger leaves the offline guard checking template references against the token
// set of a version the scaffold no longer installs — the exact drift state the
// ledger prevents. So a failed token read must skip that package's pin bump
// too, not write it alone.
//
// The other package must still bump: the failure is scoped to the token package.
func TestRunSkipsThePinBumpWhenTheTokenReadFails(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir)

	fetch := func(pkg string) (string, error) {
		switch pkg {
		case "@civitai/app-sdk":
			return "0.25.4", nil
		case "@civitai/blocks-react":
			return "0.30.1", nil
		}
		return "", scaffold.ErrPkgNotFound
	}
	failTokens := func(pkg, version string) ([]string, error) {
		return nil, errors.New("registry unreachable")
	}

	var buf bytes.Buffer
	changes, err := run(dir, fetch, failTokens, false, &buf)
	if err != nil {
		t.Fatalf("a failed token read must skip, not fail the command: %v", err)
	}
	// Only app-sdk's 3 pin sites — blocks-react is skipped entirely, ledger
	// included.
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes (app-sdk only), got %d:\n%s", len(changes), buf.String())
	}
	pkgJSON := readFile(t, dir, pkgJSONFile)
	assertContains(t, pkgJSON, `"@civitai/app-sdk": "^0.25.0"`)
	// 🔴 The load-bearing assertion: blocks-react's pin is UNCHANGED at the
	// fixture's value. A bumped pin here would be the half-applied state.
	assertContains(t, pkgJSON, `"@civitai/blocks-react": "^0.29.0"`)
	if _, err := os.Stat(filepath.Join(dir, scaffold.DesignTokenLedgerFile)); !os.IsNotExist(err) {
		t.Errorf("no ledger should have been written when the token read failed (stat err: %v)", err)
	}
}

// TestSyncLedgerRewritesOnPinMoveAndOnTokenChange covers the two states the
// guard can be wrong about, and the one that must NOT churn a PR.
func TestSyncLedgerRewritesOnPinMoveAndOnTokenChange(t *testing.T) {
	toks := []string{"--civitai-color-text", "--civitai-color-surface"}

	seed := func(t *testing.T, pin, version string, tokens []string) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, scaffold.DesignTokenLedgerFile)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		raw := scaffold.RenderDesignTokenLedger(scaffold.DesignTokenPkg, pin, version, tokens)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("pin moved -> rewrite", func(t *testing.T) {
		dir := seed(t, "^0.43.0", "0.43.1", toks)
		c, err := syncLedger(dir, scaffold.DesignTokenPkg, "^0.44.0", "0.44.2", toks, false)
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			t.Fatal("a moved pin must rewrite the ledger — otherwise the guard checks the wrong version's token set")
		}
		l, err := scaffold.ParseDesignTokenLedger([]byte(readFile(t, dir, scaffold.DesignTokenLedgerFile)))
		if err != nil {
			t.Fatal(err)
		}
		if l.Pin != "^0.44.0" || l.Version != "0.44.2" {
			t.Errorf("ledger not updated: %+v", l)
		}
	})

	t.Run("token set changed -> rewrite", func(t *testing.T) {
		dir := seed(t, "^0.43.0", "0.43.1", toks)
		next := []string{"--civitai-color-text", "--civitai-color-surface", "--civitai-color-brand-new"}
		c, err := syncLedger(dir, scaffold.DesignTokenPkg, "^0.43.0", "0.43.2", next, false)
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			t.Fatal("a changed token set must rewrite the ledger even within one caret range")
		}
		l, err := scaffold.ParseDesignTokenLedger([]byte(readFile(t, dir, scaffold.DesignTokenLedgerFile)))
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Tokens) != 3 {
			t.Errorf("ledger tokens = %v, want the new 3", l.Tokens)
		}
	})

	t.Run("only the version moved -> no churn", func(t *testing.T) {
		dir := seed(t, "^0.43.0", "0.43.1", toks)
		before := readFile(t, dir, scaffold.DesignTokenLedgerFile)
		c, err := syncLedger(dir, scaffold.DesignTokenPkg, "^0.43.0", "0.43.2", toks, false)
		if err != nil {
			t.Fatal(err)
		}
		if c != nil {
			t.Errorf("same pin + same tokens must not rewrite (it would churn a PR for no behavioural change), got %+v", c)
		}
		if got := readFile(t, dir, scaffold.DesignTokenLedgerFile); got != before {
			t.Error("ledger was rewritten despite no behavioural change")
		}
	})

	t.Run("missing ledger is regenerated", func(t *testing.T) {
		dir := t.TempDir()
		c, err := syncLedger(dir, scaffold.DesignTokenPkg, "^0.43.0", "0.43.1", toks, false)
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			t.Fatal("a missing ledger must be regenerated — the guard fails closed on one, so leaving it absent leaves the repo permanently red")
		}
		if _, err := os.Stat(filepath.Join(dir, scaffold.DesignTokenLedgerFile)); err != nil {
			t.Errorf("ledger not created: %v", err)
		}
	})

	t.Run("refuses to write an empty ledger", func(t *testing.T) {
		dir := seed(t, "^0.43.0", "0.43.1", toks)
		before := readFile(t, dir, scaffold.DesignTokenLedgerFile)
		if _, err := syncLedger(dir, scaffold.DesignTokenPkg, "^0.44.0", "0.44.0", nil, false); err == nil {
			t.Fatal("writing an EMPTY ledger must be an error — it would make the membership guard pass over everything")
		}
		if got := readFile(t, dir, scaffold.DesignTokenLedgerFile); got != before {
			t.Error("the refused write still modified the ledger")
		}
	})

	t.Run("check writes nothing", func(t *testing.T) {
		dir := seed(t, "^0.43.0", "0.43.1", toks)
		before := readFile(t, dir, scaffold.DesignTokenLedgerFile)
		c, err := syncLedger(dir, scaffold.DesignTokenPkg, "^0.44.0", "0.44.2", toks, true /*check*/)
		if err != nil {
			t.Fatal(err)
		}
		if c == nil {
			t.Fatal("--check must still REPORT the needed rewrite")
		}
		if got := readFile(t, dir, scaffold.DesignTokenLedgerFile); got != before {
			t.Error("--check must not write")
		}
	})
}

// --- fixture helpers ---

func writeFixture(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, pkgJSONFile), fixturePkgJSON)
	mustWrite(t, filepath.Join(dir, readmeFile), fixtureReadme)
	mustWrite(t, filepath.Join(dir, testFile), fixtureTest)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected to contain %q, in:\n%s", needle, haystack)
	}
}

// Fixtures start pinned at the pre-bump 0.24 / 0.29 minors.
const fixturePkgJSON = `{
  "name": "{{ .Slug }}",
  "dependencies": {
    "@civitai/app-sdk": "^0.24.0",
    "@civitai/blocks-react": "^0.29.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  }
}
`

const fixtureReadme = `# {{ .Name }}

Wired to ` + "`@civitai/blocks-react/ui`" + ` — a subpath import.

` + "`accountType` / `useBuzzBalance` require `@civitai/app-sdk@^0.24.0` + `@civitai/blocks-react@^0.29.0` (already pinned)." + `
`

const fixtureTest = `package scaffold

func TestPins(t *testing.T) {
	mustContain(t, pkg, ` + "`" + `"@civitai/blocks-react": "^0.29.0"` + "`" + `)
	mustContain(t, pkg, ` + "`" + `"@civitai/app-sdk": "^0.24.0"` + "`" + `)
}
`
