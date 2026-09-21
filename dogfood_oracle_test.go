package cli_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The RENDER ORACLE of the blind dogfood harness (scripts/dogfood/oracle.sh +
// serve-block.mjs + briefs/*.assert.mjs).
//
// The arc's frozen closing condition is a headless browser observing a running
// block. `civitai app validate` is a cheap offline gate that is REPORTED and
// decides nothing — because an untouched `civitai app init` scaffold passes it,
// so a validate-keyed grade returns a confident `yes` to a question it never
// asked.
//
// These tests pin the three things that would make a matrix silently vacuous:
//
//   - the gate deciding the verdict (a red validate turning a working block into
//     a `no`, or a green one carrying a broken block to `yes`);
//   - a harness failure — no browser, a container that went away, a server the
//     host cannot reach — being reported as `RENDER=no`, which reads as "the
//     model did not build the app";
//   - the `observed` string not reaching the cell, which collapses a near-miss
//     (`212 °F`, a working converter with a unit suffix) into a bare `no`.
//
// Docker is STUBBED (see stubDockerScript): the stub rewrites container paths
// onto a host fixture tree and runs the command there, so the real oracle, the
// real server and the real browser all execute — only the container is fake.
// That keeps these runnable on a machine with no daemon while still exercising
// the code under test rather than a mock of it.

const oracleDir = "scripts/dogfood"

// 🔴 A SKIP IS A GREEN THAT CHECKED NOTHING. Same rule, and the same shape, as
// dogfoodTool: on a contributor's machine a missing browser skips (with the
// reason, under -v); under CI it FAILS, because there a skip and a pass are
// indistinguishable in the log the merge gate is read from. ci.yml's build-test
// job resolves a browser into CIVITAI_CHROME before `go test ./...` for exactly
// this reason.
func oracleBrowser(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("CIVITAI_CHROME"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, n := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	msg := "no Chromium on PATH and CIVITAI_CHROME unset — these tests drive a real " +
		"browser against a real served block, so without one THIS SUITE CHECKED NOTHING. " +
		"That is a statement about this machine, not about the oracle."
	if os.Getenv("CI") != "" {
		t.Fatal(msg + " Under CI that is a defect: build-test would report `ok` having run nothing.")
	}
	t.Skip(msg)
	return ""
}

// ── fixtures ─────────────────────────────────────────────────────────────────

// A correct converter, plain DOM. Deliberately NOT React and NOT built with the
// SDK: this fixture exists to exercise the ORACLE, and pulling a node_modules
// tree in would make the oracle's test depend on a registry fetch.
const fxGood = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="celsius" type="number">
<button id="go">Convert</button>
<div data-testid="fahrenheit"></div>
<script>
document.getElementById('go').onclick = function () {
  var c = parseFloat(document.querySelector('[data-testid="celsius"]').value);
  document.querySelector('[data-testid="fahrenheit"]').textContent =
    String(Math.round(((c * 9) / 5 + 32) * 100) / 100);
};
</script></body>`

// The strictness hazard in one file: the arithmetic is RIGHT and the brief still
// says no, because it asked for "the converted number and nothing else". The
// point of the fixture is the `observed` field, not the verdict.
const fxNearMiss = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="celsius" type="number">
<button id="go">Convert</button>
<div data-testid="fahrenheit"></div>
<script>
document.getElementById('go').onclick = function () {
  var c = parseFloat(document.querySelector('[data-testid="celsius"]').value);
  document.querySelector('[data-testid="fahrenheit"]').textContent =
    String(Math.round(((c * 9) / 5 + 32) * 100) / 100) + ' \u00b0F';
};
</script></body>`

// Stands in for an untouched scaffold: a perfectly valid app that does not do
// the thing the brief named.
const fxScaffold = `<!doctype html><meta charset="utf-8"><body>
<main id="app"><h1>Hello</h1><button id="ping">Click me</button></main>
</body>`

// 🔴 THE HOST-GATE FIXTURE, and the reason the oracle injects anything at all.
// A block built from the `page-money` template renders nothing but
// "Connecting to host…" until a host delivers its runtime context — measured on
// a freshly built scaffold, 2026-09-20. This reproduces that BRANCH (the SDK's
// transport detector keys on `window.__CIVITAI_BLOCK_CONTEXT__`) without pulling
// the SDK in, so the guard runs offline. The real-SDK measurement is recorded in
// briefs/celsius.md; this is the regression test for it.
const fxHostGated = `<!doctype html><meta charset="utf-8"><body>
<div id="root">Connecting to host&hellip;</div>
<script>
if (window.__CIVITAI_BLOCK_CONTEXT__) {
  document.getElementById('root').innerHTML =
    '<input data-testid="celsius" type="number">' +
    '<button id="go">Convert</button><div data-testid="fahrenheit"></div>';
  document.getElementById('go').onclick = function () {
    var c = parseFloat(document.querySelector('[data-testid="celsius"]').value);
    document.querySelector('[data-testid="fahrenheit"]').textContent =
      String(Math.round(((c * 9) / 5 + 32) * 100) / 100);
  };
}
</script></body>`

const fxManifestBuilt = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",
"buildCommand":"npm run build","outputDir":"dist"}`

const fxManifestStatic = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block"}`

// ── the stub docker ──────────────────────────────────────────────────────────

// Rewrites the container's paths onto the fixture tree and runs the command on
// this machine. Everything downstream of it — serve-block.mjs, the HTTP fetch,
// the browser, the assertion — is the real thing.
//
// STUB_STATE      what `docker inspect` reports (empty => no such container)
// STUB_ROOT       host dir standing in for the container's /work
// STUB_TMP        host dir standing in for the container's /tmp
// STUB_NODE       "0" => the container has no node
// STUB_CIVITAI_RC "-" => no civitai on PATH; otherwise the exit code of validate
const stubDockerScript = `#!/usr/bin/env bash
set -uo pipefail
# Container path -> host path on the way IN, and host path -> container path on
# the way OUT. 🔴 BOTH DIRECTIONS ARE REQUIRED. Rewriting only inbound leaks host
# paths back to the caller, which then hands them to the next exec -- where the
# inbound rule rewrites the /work inside the host path a SECOND time and every
# later command addresses a directory that does not exist. The oracle must only
# ever see container paths, exactly as it would with a real daemon.
rw()  { printf '%s' "$1" | sed "s|/tmp/dogfood-|$STUB_TMP/dogfood-|g; s|/work|$STUB_ROOT|g"; }
unrw() { sed "s|$STUB_ROOT|/work|g; s|$STUB_TMP|/tmp|g"; }
case "${1:-}" in
  inspect)
    for a in "$@"; do case "$a" in
      *State.Status*) printf '%s' "$STUB_STATE"; [ -n "$STUB_STATE" ] || exit 1; echo; exit 0 ;;
      *NetworkSettings*) echo "127.0.0.1 "; exit 0 ;;
    esac; done
    exit 1 ;;
  cp)
    src="$2"; dst="${3#*:}"; cp "$src" "$(rw "$dst")"; exit $? ;;
  exec)
    shift; det=0
    while [ $# -gt 0 ]; do case "$1" in
      -u|-w) shift 2 ;;
      -d) det=1; shift ;;
      -i) shift ;;
      *) break ;;
    esac; done
    shift              # the container name
    shift              # bash | sh
    shift              # -lc | -c
    raw="${1:-}"
    # The container's own node. Answered here rather than by hiding the real
    # node from PATH, which the stub itself needs to carry commands out.
    case "$raw" in
      *"command -v node"*) [ "${STUB_NODE:-1}" = "1" ] || exit 1 ;;
    esac
    cmd="$(rw "$raw")"
    if [ "$det" = "1" ]; then bash -c "$cmd" & exit 0; fi
    bash -c "$cmd" | unrw; exit "${PIPESTATUS[0]}" ;;
esac
exit 1
`

type stubEnv struct {
	state     string // "running", "exited", "" (absent)
	root      string // the host dir standing in for /work
	civitaiRC string // "-" for absent, else the exit code validate returns
	appHTML   string
	manifest  string
	// outputDir: when non-empty the HTML is written there, mimicking a built app.
	outputDir string
	noNode    bool
}

// Builds a stub PATH and a fixture tree, and returns the env for running
// oracle.sh / grade.sh against it.
func stubOracleEnv(t *testing.T, s stubEnv) []string {
	t.Helper()
	dir := t.TempDir()
	stub := filepath.Join(dir, "bin")
	work := filepath.Join(dir, "work")
	tmp := filepath.Join(dir, "tmp")
	for _, d := range []string{stub, work, tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, body string, mode os.FileMode) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(stub, "docker"), stubDockerScript, 0o755)
	if s.civitaiRC != "-" && s.civitaiRC != "" {
		write(filepath.Join(stub, "civitai"), fmt.Sprintf(
			"#!/bin/sh\necho 'stub validate says %s'\nexit %s\n", s.civitaiRC, s.civitaiRC), 0o755)
	}
	if s.manifest != "" {
		app := filepath.Join(work, "app")
		write(filepath.Join(app, "block.manifest.json"), s.manifest, 0o644)
		if s.appHTML != "" {
			out := app
			if s.outputDir != "" {
				out = filepath.Join(app, s.outputDir)
			}
			write(filepath.Join(out, "index.html"), s.appHTML, 0o644)
		}
	}
	// 🔴 The stub dir goes FIRST on PATH but the rest of PATH is kept: the stub
	// rewrites container paths and then needs the real `find`, `cat`, `node` and
	// the browser to carry the command out.
	node := "1"
	if s.noNode {
		node = "0"
	}
	return []string{
		"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STUB_STATE=" + s.state,
		"STUB_ROOT=" + work,
		"STUB_TMP=" + tmp,
		"STUB_NODE=" + node,
	}
}

func runScript(t *testing.T, script string, env []string, args ...string) (string, int) {
	t.Helper()
	bash := dogfoodTool(t, "bash")
	cmd := exec.Command(bash, append([]string{filepath.Join(oracleDir, script)}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v\n%s", script, err, out)
	}
	return string(out), code
}

// Reads a `key=value` off the oracle's single-line summary.
func summaryField(t *testing.T, out, key string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "brief=") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, key+"=") {
				return strings.TrimPrefix(f, key+"=")
			}
		}
	}
	t.Fatalf("no %q field on any summary line of:\n%s", key, out)
	return ""
}

// ── nothing-was-measured: exit 2, and never a verdict ────────────────────────

// 🔴 THE SINGLE MOST IMPORTANT PROPERTY. A harness that could not measure must
// not emit the verdict a broken block emits. Every case below is reachable on an
// ordinary matrix run: a typo'd trial id, a daemon restart, a cgroup OOM kill.
func TestOracleRefusesWhenNothingCanBeMeasured(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     stubEnv
		brief   string
		wantMsg string
	}{
		{"no such container", stubEnv{state: "", root: "x"}, "celsius", "no such container"},
		{"container not running", stubEnv{state: "exited"}, "celsius", "not running"},
		{"unknown brief", stubEnv{state: "running"}, "no-such-brief", "no assertion for brief"},
		{"no node in the container", stubEnv{state: "running", noNode: true}, "celsius", "no node inside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := stubOracleEnv(t, tc.env)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root", tc.brief)
			if code != 2 {
				t.Fatalf("exit %d, want 2 (nothing measured)\n%s", code, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("the refusal does not say %q:\n%s", tc.wantMsg, out)
			}
			// It must not also print a verdict — a reader grepping RENDER= would
			// otherwise find one attached to a run that measured nothing.
			if strings.Contains(out, "RENDER=") {
				t.Fatalf("a run that measured nothing still printed a RENDER verdict:\n%s", out)
			}
		})
	}
}

// No manifest anywhere is a real finding about the trial (the agent created no
// app), so it IS a verdict — `no`, exit 0 — and the reason has to say which.
func TestOracleReportsNoAppAsAVerdictNotAHarnessError(t *testing.T) {
	env := stubOracleEnv(t, stubEnv{state: "running", civitaiRC: "-"})
	out, code := runScript(t, "oracle.sh", env, "ctl", "root", "celsius")
	if code != 0 {
		t.Fatalf("exit %d, want 0 — an app that was never created is a measured `no`\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no\n%s", got, out)
	}
	if !strings.Contains(out, "no block.manifest.json") {
		t.Fatalf("the reason does not name the missing manifest:\n%s", out)
	}
}

// A manifest declaring a buildCommand whose outputDir was never produced is the
// shape a model that wrote the app and never ran the build leaves behind. That
// must be legible as such and not as "the block rendered nothing".
func TestOracleSaysTheAppWasNeverBuilt(t *testing.T) {
	env := stubOracleEnv(t, stubEnv{state: "running", civitaiRC: "-", manifest: fxManifestBuilt})
	out, code := runScript(t, "oracle.sh", env, "ctl", "root", "celsius")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no\n%s", got, out)
	}
	if !strings.Contains(out, "never built") {
		t.Fatalf("the reason does not distinguish an unbuilt app:\n%s", out)
	}
}

// ── the verdict comes from the browser ───────────────────────────────────────

// 🔴 THE GATE IS REPORTED AND DECIDES NOTHING, IN BOTH DIRECTIONS. One direction
// alone is not enough: a gate wired as `verdict = gate && render` passes the
// "red gate, good block" case only if it is also checked, and a gate wired as
// `verdict = gate` passes the "green gate, bad block" case. This is exactly the
// substitution the frozen closing condition forbids — an untouched scaffold
// validates clean (`✓ <dir> is valid`, measured), so a validate-keyed grade
// scores every cell green.
func TestOracleGateNeverDecidesTheVerdict(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		gateRC     string
		html       string
		wantRender string
		wantGate   string
	}{
		{"red gate over a working block", "1", fxGood, "yes", "fail"},
		{"green gate over a scaffold", "0", fxScaffold, "no", "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: tc.gateRC,
				manifest: fxManifestBuilt, outputDir: "dist", appHTML: tc.html,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root", "celsius")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "gate"); got != tc.wantGate {
				t.Fatalf("gate=%s, want %s (it must still be REPORTED)\n%s", got, tc.wantGate, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s — the verdict followed the gate, not the browser\n%s",
					got, tc.wantRender, out)
			}
		})
	}
}

// 🔴 THE `observed` STRING REACHES THE CELL. The assertion is strict on purpose
// — `212 °F` fails where `212` passes — so without this field a working
// converter with a unit suffix is indistinguishable from a block that rendered
// nothing, and briefs/celsius.md says in as many words that any oracle built on
// it must carry the value through.
func TestOracleCarriesTheObservedValueToTheCell(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		html       string
		wantRender string
		wantObs    string
	}{
		{"exact", fxGood, "yes", "212"},
		{"near miss", fxNearMiss, "no", "212 °F"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0",
				manifest: fxManifestBuilt, outputDir: "dist", appHTML: tc.html,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root", "celsius")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
			// %q-quoted on the summary line, so read the assertion's own JSON for
			// the exact bytes — that is what a reader of the transcript sees too.
			var obs string
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, `{"assertion"`) {
					var j struct {
						Observed string `json:"observed"`
					}
					if err := json.Unmarshal([]byte(line), &j); err != nil {
						t.Fatalf("assertion line is not JSON: %q", line)
					}
					obs = j.Observed
				}
			}
			if obs != tc.wantObs {
				t.Fatalf("observed = %q, want %q — a near-miss must not collapse into a bare `no`\n%s",
					obs, tc.wantObs, out)
			}
			// And on the SUMMARY LINE, which is what grade.sh reads and what a
			// person scanning a matrix sees. Shell-quoted there, so match on the
			// digits rather than on the exact quoting.
			if got := summaryField(t, out, "observed"); !strings.Contains(got, "212") {
				t.Fatalf("observed=%q on the summary line does not carry the value — the cell loses it\n%s", got, out)
			}
		})
	}
}

// 🔴 THE HOST HANDSHAKE, AND ITS CONTROL ARM. A block built from the page-money
// template renders only "Connecting to host…" until a host delivers its runtime
// context, so an oracle that does not emulate one times out against a perfectly
// good app and reports a false `no` — the capability confound arriving through
// the instrument. Both arms are asserted: without the injection this same
// fixture must FAIL, or the injection is not the thing making it pass.
func TestOracleSendsTheHostHandshake(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		noHost     bool
		wantRender string
	}{
		{"with the handshake", false, "yes"},
		{"without it (control)", true, "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0",
				manifest: fxManifestBuilt, outputDir: "dist", appHTML: fxHostGated,
			}), "CIVITAI_CHROME="+browser)
			if tc.noHost {
				env = append(env, "CIVITAI_ASSERT_NO_HOST=1")
			}
			out, code := runScript(t, "oracle.sh", env, "ctl", "root", "celsius")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
		})
	}
}

// A no-build app (the `static` template) is its own served root. Without this
// the oracle would only ever be able to grade apps that declare an outputDir.
func TestOracleServesANoBuildApp(t *testing.T) {
	browser := oracleBrowser(t)
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0", manifest: fxManifestStatic, appHTML: fxGood,
	}), "CIVITAI_CHROME="+browser)
	out, code := runScript(t, "oracle.sh", env, "ctl", "root", "celsius")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "yes" {
		t.Fatalf("RENDER=%s, want yes\n%s", got, out)
	}
}

// ── grade.sh wiring ──────────────────────────────────────────────────────────

// Without a brief, grade.sh must be what it was: the setup grid of 2026-09-18
// was measured by its two arms and nothing else, and an app-build `no` appended
// to those cells would read as a regression in a grid that never asked.
func TestGradeWithoutABriefEmitsNoRenderFields(t *testing.T) {
	env := stubOracleEnv(t, stubEnv{state: "running", civitaiRC: "-"})
	out, _ := runScript(t, "grade.sh", env, "ctl", "root")
	if strings.Contains(out, "RENDER=") || strings.Contains(out, "render_brief=") {
		t.Fatalf("a no-brief grade carried render fields:\n%s", out)
	}
}

// 🔴 AND WHEN THE ORACLE COULD NOT MEASURE, THE CELL SAYS SO. Folding an exit-2
// oracle into `RENDER=no` would report "the model did not build the app" about a
// run in which no block was ever loaded.
func TestGradeReportsUnmeasuredRatherThanFailing(t *testing.T) {
	// A running container (so grade.sh's own guards pass) whose brief has no
	// assertion — the oracle exits 2. grade.sh must not translate that into a
	// statement about the block.
	env := stubOracleEnv(t, stubEnv{state: "running", civitaiRC: "-"})
	out, _ := runScript(t, "grade.sh", env, "ctl", "root", "no-such-brief")
	if !strings.Contains(out, "RENDER=unmeasured") {
		t.Fatalf("an oracle that measured nothing did not surface as unmeasured:\n%s", out)
	}
	if strings.Contains(out, "RENDER=no") {
		t.Fatalf("an unmeasurable run was graded `no`:\n%s", out)
	}
}

// The end-to-end wiring claim: a graded cell carries the render verdict, the
// observed value and the gate, on the same line as the setup fields.
func TestGradeCarriesTheRenderResultOntoTheVerdictLine(t *testing.T) {
	browser := oracleBrowser(t)
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0",
		manifest: fxManifestBuilt, outputDir: "dist", appHTML: fxGood,
	}), "CIVITAI_CHROME="+browser)
	out, _ := runScript(t, "grade.sh", env, "ctl", "root", "celsius")
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "agent=") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no verdict line:\n%s", out)
	}
	for _, want := range []string{"render_brief=celsius", "validate_gate=pass", "observed=212", "RENDER=yes"} {
		if !strings.Contains(line, want) {
			t.Fatalf("the verdict line is missing %q:\n%s", want, line)
		}
	}
	// The setup arms are still there and still first — this is an ADDITIONAL
	// arm, not a replacement for the frozen two.
	if !strings.Contains(line, "CLOSING_CONDITION=") {
		t.Fatalf("the setup verdict was dropped from the line:\n%s", line)
	}
}

// ── serve-block.mjs ──────────────────────────────────────────────────────────

// The server refuses to walk out of the root it was given. A block under test is
// model-authored and so is every path it asks for.
func TestServeBlockRefusesToEscapeItsRoot(t *testing.T) {
	node := dogfoodTool(t, "node")
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A symlink INSIDE the served tree pointing outside it: the case a
	// normalize()-only containment check cannot see.
	if err := os.Symlink(secret, filepath.Join(root, "leak.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cmd := exec.Command(node, filepath.Join(oracleDir, "serve-block.mjs"), root)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	buf := make([]byte, 128)
	n, _ := stdout.Read(buf)
	var port string
	for _, l := range strings.Split(string(buf[:n]), "\n") {
		if strings.HasPrefix(l, "LISTENING ") {
			port = strings.TrimPrefix(l, "LISTENING ")
		}
	}
	if port == "" {
		t.Fatalf("the server never reported a port: %q", string(buf[:n]))
	}

	base := "http://127.0.0.1:" + port
	body := func(path string) string {
		out, err := exec.Command(node, "-e",
			`fetch(process.argv[1]).then(async r=>process.stdout.write(await r.text()))`,
			base+path).Output()
		if err != nil {
			t.Fatalf("probe %s: %v", path, err)
		}
		return string(out)
	}
	// POSITIVE CONTROL FIRST: a probe that can never see anything proves nothing
	// about what it does not see. Without this, a server bound to the wrong
	// address would make every leak assertion below pass vacuously.
	if got := body("/"); got != "ok" {
		t.Fatalf("the server does not serve its own root — this test could not have caught a leak either. got %q", got)
	}
	// The symlink case. `normalize()` cannot see through one, so a containment
	// check written against the un-resolved path passes here while serving the
	// file it points at.
	if got := body("/leak.txt"); strings.Contains(got, "SECRET") {
		t.Fatal("a symlink inside the served tree leaked a file from outside it")
	}
	// ⚠ `fetch` collapses `..` in the URL before it sends, so this is an
	// end-to-end claim ("no route reaches it") rather than a test of the `..`
	// branch specifically. The symlink case above is the one that reaches the
	// containment code.
	if got := body("/../secret.txt"); strings.Contains(got, "SECRET") {
		t.Fatal("a `..` path escaped the served root")
	}
}
