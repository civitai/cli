package scaffold

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Two defects an App Assistant eval run measured in a fresh page-elements
// scaffold, and the template changes that answer them.
//
// A9 — happy-dom (20.14.6 and 21.0.1 alike) has no `attachInternals`, and
// `<civitai-button>` plus the nine `<civitai-*>` form controls (switch included —
// it inherits form association from checkbox) call it while they are
// constructed. The stock template used none of the ten, so its own suite
// was green, and the first test an author wrote around a button or an input
// failed to load with `TypeError: this.attachInternals is not a function`. The
// template now ships a test-only stand-in (test/setup.ts, loaded through
// vitest's `setupFiles`) AND a text input plus a button in the stock view, so
// the scaffold's own suite exercises one of each — the `template-page-elements`
// CI job runs that suite. Measured on a scaffold of this tree: without
// test/setup.ts the suite reports `Test Files 2 failed | 1 passed` with
// `this.attachInternals is not a function`; with it, 20/20 pass.
//
// B6 — probes the agent wrote into `test/` became the only `tsc` errors and a
// test file that failed to load. The template now ships a git-ignored
// `scratch/` outside both the tsconfig program and the vitest collection.

func TestPageElementsLoadsTheAttachInternalsStandIn(t *testing.T) {
	dir := renderPageElements(t)

	vitest := renderedText(t, dir, "vitest.config.ts")
	if !regexp.MustCompile(`setupFiles:\s*\[\s*'test/setup\.ts'\s*\]`).MatchString(vitest) {
		t.Errorf("vitest.config.ts does not load test/setup.ts through setupFiles — without it any test "+
			"that renders <civitai-button> or a <civitai-*> field fails to load under happy-dom:\n%s", vitest)
	}

	setup := renderedText(t, dir, "test/setup.ts")
	// Installed only when absent, so it never replaces a real implementation.
	if !strings.Contains(setup, "typeof HTMLElement.prototype.attachInternals !== 'function'") {
		t.Errorf("test/setup.ts must install the stand-in ONLY when attachInternals is missing:\n%s", setup)
	}
	// Every member the elements call (dist/elements/civitai-button.js and
	// field-base.js in @civitai/components 0.9.x). Removing setFormValue was
	// measured to fail 10 of the 20 stock tests with
	// `this.internals.setFormValue is not a function`.
	for _, member := range []string{
		"get form()", "get validity()", "setFormValue(", "setValidity(", "checkValidity(", "reportValidity(",
	} {
		if !strings.Contains(setup, member) {
			t.Errorf("test/setup.ts's stand-in lacks %q, which @civitai/components calls", member)
		}
	}
	// Test-only: nothing the app ships may import it.
	for _, f := range []string{"src/block.ts", "src/main.ts", "index.html"} {
		if src := renderedText(t, dir, f); strings.Contains(src, "test/setup") || strings.Contains(src, "/setup.js") {
			t.Errorf("%s references the test setup — the stand-in must never reach the bundle", f)
		}
	}

	// The stock view uses one form field and the button, and the stock suite
	// drives both, so the scaffold's own `npm test` covers the path.
	block := renderedText(t, dir, "src/block.ts")
	for _, tag := range []string{"<civitai-text-input ", "<civitai-button "} {
		if !strings.Contains(block, tag) {
			t.Errorf("src/block.ts's stock view no longer uses %s, so the scaffold's own suite no longer "+
				"exercises an attachInternals element", strings.TrimSpace(tag))
		}
	}
	test := renderedText(t, dir, "test/block.test.ts")
	for _, want := range []string{"customElements.get('civitai-text-input')", "customElements.get('civitai-button')", "echo.click()"} {
		if !strings.Contains(test, want) {
			t.Errorf("test/block.test.ts no longer drives the stock controls (%q missing)", want)
		}
	}
}

func TestPageElementsScratchDirIsIgnoredAndOutsideTheProgram(t *testing.T) {
	dir := renderPageElements(t)

	if _, err := os.Stat(filepath.Join(dir, "scratch", "README.md")); err != nil {
		t.Fatalf("page-elements ships no scratch/README.md: %v", err)
	}

	// Git: everything in scratch/ is ignored except the README that keeps the
	// directory in every clone.
	ignore := strings.Split(renderedText(t, dir, ".gitignore"), "\n")
	for _, want := range []string{"scratch/*", "!scratch/README.md"} {
		if !containsLine(ignore, want) {
			t.Errorf(".gitignore lacks the line %q", want)
		}
	}

	// tsc: excluded explicitly, and no include pattern reaches it anyway.
	var ts struct {
		Include []string `json:"include"`
		Exclude []string `json:"exclude"`
	}
	if err := json.Unmarshal([]byte(renderedText(t, dir, "tsconfig.json")), &ts); err != nil {
		t.Fatalf("tsconfig.json: %v", err)
	}
	if len(ts.Include) == 0 {
		t.Fatal("CONTROL failure, not a finding: tsconfig.json's include parsed empty")
	}
	if !containsLine(ts.Exclude, "scratch") {
		t.Errorf("tsconfig.json exclude = %v, want it to name scratch", ts.Exclude)
	}
	for _, inc := range ts.Include {
		if strings.HasPrefix(inc, "scratch") || strings.HasPrefix(inc, "**") || inc == "*" {
			t.Errorf("tsconfig.json include %q reaches scratch/", inc)
		}
	}

	// vitest: collects only test/**, and excludes scratch/** on top of the defaults.
	vitest := renderedText(t, dir, "vitest.config.ts")
	if !strings.Contains(vitest, "include: ['test/**/*.test.ts']") {
		t.Errorf("vitest.config.ts include changed:\n%s", vitest)
	}
	if !regexp.MustCompile(`exclude:\s*\[\s*\.\.\.configDefaults\.exclude,\s*'scratch/\*\*'\s*\]`).MatchString(vitest) {
		t.Errorf("vitest.config.ts does not exclude scratch/** (on top of configDefaults.exclude):\n%s", vitest)
	}
}

// TestPageElementsScratchProbesCannotFailTypecheckOrTests RUNS the scaffold's own
// toolchain, so it is gated on CIVITAI_SCAFFOLD_TYPECHECK=1 (network, ~1 min),
// like TestPageMoneyScaffoldTypechecksItsOwnShippedTests.
//
// It plants a file with a type error and a failing test in scratch/ and requires
// `tsc` and `vitest run` to stay green; then, as the POSITIVE CONTROL, plants the
// same two files in test/ and requires both to go red — without that half a
// green would also be what a toolchain that checks nothing reports.
func TestPageElementsScratchProbesCannotFailTypecheckOrTests(t *testing.T) {
	if os.Getenv(typecheckEnv) != "1" {
		t.Skipf("set %s=1 to render page-elements, npm install and run its tsc + vitest against "+
			"probes in scratch/ (network, ~1 min). NOTHING about scratch/ was verified by this run.", typecheckEnv)
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("%s=1 was set and npm is not on PATH: %v", typecheckEnv, err)
	}
	dir := renderPageElements(t)
	install := exec.Command(npm, "install", "--no-audit", "--no-fund")
	install.Dir = dir
	install.Env = append(os.Environ(), "npm_config_update_notifier=false")
	if out, err := runWithTimeout(install, 10*time.Minute); err != nil {
		t.Fatalf("npm install failed in the rendered scaffold: %v\n%s", err, tail(out, 4000))
	}

	const typeError = "const n: number = 'not a number';\nexport default n;\n"
	const failingTest = "import { expect, test } from 'vitest';\ntest('probe', () => expect(1).toBe(2));\n"
	tsc := filepath.Join(dir, "node_modules", ".bin", "tsc")
	vitest := filepath.Join(dir, "node_modules", ".bin", "vitest")
	runBoth := func() (tscOut, testOut string, tscErr, testErr error) {
		c := exec.Command(tsc, "-p", "tsconfig.json", "--noEmit")
		c.Dir = dir
		tscOut, tscErr = runWithTimeout(c, 5*time.Minute)
		v := exec.Command(vitest, "run")
		v.Dir = dir
		v.Env = append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0")
		testOut, testErr = runWithTimeout(v, 5*time.Minute)
		return
	}
	plant := func(sub string) func() {
		a := filepath.Join(dir, sub, "zzscratchprobe.ts")
		b := filepath.Join(dir, sub, "zzscratchprobe.test.ts")
		for p, body := range map[string]string{a: typeError, b: failingTest} {
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return func() { _ = os.Remove(a); _ = os.Remove(b) }
	}

	cleanup := plant("scratch")
	tscOut, testOut, tscErr, testErr := runBoth()
	cleanup()
	if tscErr != nil {
		t.Errorf("a type error in scratch/ failed tsc: %v\n%s", tscErr, tail(tscOut, 3000))
	}
	if testErr != nil {
		t.Errorf("a failing test in scratch/ failed vitest: %v\n%s", testErr, tail(testOut, 3000))
	}
	// The stock suite really ran (a vitest that collected nothing also exits 0).
	if !regexp.MustCompile(`Tests\s+\d+ passed`).MatchString(testOut) || strings.Contains(testOut, "zzscratchprobe") {
		t.Errorf("vitest did not run the stock suite, or collected the scratch probe:\n%s", tail(testOut, 3000))
	}

	cleanup = plant("test")
	tscOut, testOut, tscErr, testErr = runBoth()
	cleanup()
	if tscErr == nil || !strings.Contains(tscOut, "zzscratchprobe.ts") {
		t.Errorf("CONTROL failure: the same type error in test/ did not fail tsc, so the green above "+
			"proves nothing:\n%s", tail(tscOut, 3000))
	}
	if testErr == nil || !strings.Contains(testOut, "zzscratchprobe") {
		t.Errorf("CONTROL failure: the same failing test in test/ did not fail vitest:\n%s", tail(testOut, 3000))
	}
}

func renderedText(t *testing.T, dir, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}
