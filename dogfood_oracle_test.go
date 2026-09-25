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
//
// 🔴 `oracleBrowserNames` IS ONE OF FOUR COPIES OF THE SAME PREFERENCE ORDER —
// the others are in `scripts/dogfood/briefs/_cdp.mjs` (`BROWSER_NAMES`),
// `scripts/dogfood/oracle.sh` and `.github/workflows/ci.yml`. They must agree,
// or this suite resolves a different browser from the one the assertions it
// drives will pick, and `TestEveryBrowserResolverAgreesOnTheSameOrder` in
// dogfood_cdp_launch_test.go is what enforces that.
var oracleBrowserNames = []string{
	"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
}

func oracleBrowser(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("CIVITAI_CHROME"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, n := range oracleBrowserNames {
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

// ── the `genpost` brief's fixtures ───────────────────────────────────────────
// A generate-then-post app as the brief specifies it: the Post control exists
// and is CLOSED until something has been generated, and the Generate click
// drives the status machine through `generating`. Plain DOM, for the same reason
// fxGood is: this exercises the ORACLE, not the SDK.
const fxGenpost = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post" disabled>Post</button>
<div data-testid="status">ready</div>
<script>
var s = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () {
  s.textContent = 'generating';
  setTimeout(function () {
    s.textContent = 'generated';
    document.getElementById('post').disabled = false;
  }, 30);
};
</script></body>`

// 🔴 THE CONTROL THAT MATTERS MORE THAN THE SCAFFOLD ONE. A complete, working
// generate-only app carrying the exact test ids the brief names — what a model
// that read the first half of the brief would build — and the `page-money`
// scaffold ALREADY ships that shape. If the oracle grades this `yes`, the brief
// measures the test ids and nothing about posting.
const fxGenpostNoPost = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<div data-testid="status">ready</div>
<script>
var s = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () { s.textContent = 'generating'; };
</script></body>`

// 🔴 THE FIXTURE THAT REACHES THE DISABLED CHECK. Without it, mutating the
// `postDisabled !== true` branch away SURVIVES — the no-Post fixture above
// throws one line earlier, so the disabled guard never executes and its
// assertion is unreachable. Measured: M9 of the mutation battery survived until
// this row existed.
const fxGenpostPostEnabled = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post">Post</button>
<div data-testid="status">ready</div>
<script>
var s = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () { s.textContent = 'generating'; };
</script></body>`

// 🔴 AND THE ONE THAT REACHES THE STATUS PREDICATE. Everything the brief names
// is present and the Generate button does nothing — the shape of an app wired
// up to the eye and not to anything. Same story: mutating `pass =
// seq.includes(STATUS_BUSY)` to `pass = true` survived until this row existed,
// because every other failing fixture throws before the predicate is evaluated.
const fxGenpostInertGenerate = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post" disabled>Post</button>
<div data-testid="status">ready</div>
<script>document.getElementById('gen').onclick = function () {};</script></body>`

// 🔴 THE NEAR-MISS THE PREDICATE ITSELF HAS TO CATCH. A working app whose state
// machine uses a different word — it moves, it just never says `generating`.
// Without this row, mutating the predicate to `pass = true` SURVIVES: every
// other failing fixture THROWS before the predicate is evaluated, so the
// comparison is unreachable. Measured: M10 survived until this row existed.
const fxGenpostWrongWord = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post" disabled>Post</button>
<div data-testid="status">ready</div>
<script>
var s = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () { s.textContent = 'submitting'; };
</script></body>`

// Same story one step earlier: the resting word is wrong. Without this the
// `initialStatus !== STATUS_IDLE` guard is unreachable (M11).
const fxGenpostWrongIdle = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post" disabled>Post</button>
<div data-testid="status">idle</div>
<script>
var s = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () { s.textContent = 'generating'; };
</script></body>`

// 🔴 THE AUTH-GATED APP, AND IT IS NOT A HYPOTHETICAL. This is the shape of
// `ab-genpost-mimo-01` (`const anon = ready && !viewer` → a sign-in CTA), which
// is the shape the CLI's own `page-money` scaffold ships — sign-in CTA, `pm-
// signin` test id and all. Under the oracle's old `viewer: null` bootstrap it
// rendered `<div data-testid="status">ready</div><p>Please sign in…</p>` and
// graded `RENDER=no`, a verdict about the harness's viewer rather than about
// the model. Both arms are required: without the anonymous control the seeded
// viewer is not shown to be what makes this pass.
const fxGenpostAuthGated = `<!doctype html><meta charset="utf-8"><body>
<div id="root"></div>
<script>
var b = window.__CIVITAI_BLOCK_CONTEXT__;
var r = document.getElementById('root');
if (!b || !b.viewer) {
  r.innerHTML = '<div data-testid="status">ready</div><p>Please sign in to generate images.</p>';
} else {
  r.innerHTML = '<input data-testid="prompt" type="text">' +
    '<button id="gen">Generate</button><button id="post" disabled>Post</button>' +
    '<div data-testid="status">ready</div>';
  document.getElementById('gen').onclick = function () {
    document.querySelector('[data-testid="status"]').textContent = 'generating';
  };
}
</script></body>`

// 🔴 THE SEAM GUARD: what the oracle actually PUTS on the window, asserted from
// inside the page rather than by reading _cdp.mjs's source. It drives the
// status machine to `generating` only when the bootstrap is exactly what a real
// host sends, and otherwise names the discrepancy — which the genpost assertion
// then carries out as `observed`, so a failure says WHICH field is wrong.
//
// The four things it pins, and why each is a hazard rather than a nicety:
//
//   - `viewer` is a present object with a NUMERIC id. `isValidBlockInitPayload`,
//     compiled into every already-deployed block bundle, rejects a viewer that
//     is neither null nor an object with a numeric `id` — a rejected BLOCK_INIT
//     is silently re-sent, not surfaced.
//   - its key set is EXACTLY production's. civitai.com funnels both hosts
//     through `withSignedInFlag()`, which PICKS `{ id, username, signedIn }` and
//     deliberately omits `status` (civitai #2521). A wider fake lets a block
//     read a field production never sends and still grade green here.
//   - `token.scopes` is EXACTLY what this fixture's manifest declares. It is run
//     with fxManifestScoped, so the list below is that manifest's, spelled out
//     rather than computed — a probe that recomputed it from the same source the
//     oracle reads could agree with a broken oracle. This crosses a process
//     boundary (oracle.sh -> `node <brief>.assert.mjs <url> <csv>` -> the
//     bootstrap), and nothing else in this suite can see that plumbing break:
//     the cell's own `scopes=` field is read straight off the manifest and stays
//     right whatever the block was shown.
//   - the token's `raw` stays EMPTY. Scopes must buy the block a BRANCH and
//     never a CAPABILITY: if a green cell could be earned with a credential the
//     oracle handed over, the brief's "no generation and no post can complete
//     here" premise is gone.
const fxGenpostBootstrapProbe = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post" disabled>Post</button>
<div data-testid="status">ready</div>
<script>
document.getElementById('gen').onclick = function () {
  var b = window.__CIVITAI_BLOCK_CONTEXT__ || {};
  var v = b.viewer, t = b.token || {}, c = b.context || {};
  var bad = [];
  if (!v) { bad.push('viewer-null'); }
  else {
    if (typeof v.id !== 'number') bad.push('viewer-id-not-number');
    var k = Object.keys(v).sort().join('+');
    if (k !== 'id+signedIn+username') bad.push('viewer-keys:' + k);
  }
  if (t.raw !== '') bad.push('token-raw-nonempty');
  if (!t.scopes || t.scopes.join(',') !== 'ai:write:budgeted,posts:write:self') {
    bad.push('token-scopes:' + JSON.stringify(t.scopes));
  }
  if (c.viewerUserId !== (v ? v.id : null)) bad.push('context-viewer-mismatch');
  document.querySelector('[data-testid="status"]').textContent =
    bad.length ? bad.join(',') : 'generating';
};
</script></body>`

// 🔴 THE CONSENT-FIRST APP, AND IT IS NOT A HYPOTHETICAL EITHER. This is the
// shape of `ab-genpost-dsv4-01` (2026-09-21): `const granted =
// hasBudgetedScope(token.scopes)`, and a `handleGenerate` that asks the host for
// consent and RETURNS when it is false, so `setStatus('generating')` never runs
// and the DOM produces no mutation at all after the click. Under the oracle's
// old unconditional `scopes: []` bootstrap that graded `RENDER=no
// observed=ready` — a verdict about the harness's scope list, which rewarded
// flipping a status optimistically and penalised checking consent first.
//
// The un-granted branch does NOTHING on purpose: the real app's `requestConsent`
// is a host call the stub transport swallows, so the measured signature is a
// TIMEOUT at `observed=ready`, not a wrong word.
const fxGenpostScopeGated = `<!doctype html><meta charset="utf-8"><body>
<input data-testid="prompt" type="text">
<button id="gen">Generate</button>
<button id="post" disabled>Post</button>
<div data-testid="status">ready</div>
<script>
var t = (window.__CIVITAI_BLOCK_CONTEXT__ || {}).token || {};
var granted = (t.scopes || []).indexOf('ai:write:budgeted') !== -1;
var s = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () {
  if (!granted) return;
  s.textContent = 'generating';
};
</script></body>`

const fxManifestBuilt = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",
"buildCommand":"npm run build","outputDir":"dist"}`

const fxManifestStatic = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block"}`

// A manifest declaring the two scopes a generate-then-post app needs. `scopes`
// is REPORTED on the cell and decides nothing — see briefs/genpost.md.
const fxManifestScoped = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",
"scopes":["ai:write:budgeted","posts:write:self"],
"buildCommand":"npm run build","outputDir":"dist"}`

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
    #
    # 🔴 THE SAME ANSWER FOR THE CONTAINER'S OWN CLI, AND IT IS NOT COSMETIC. The
    # stub keeps the host PATH behind it (it needs the real find/cat/node), so a
    # test that omits its civitai stub in order to say "the container has no CLI"
    # gets the OPERATOR'S INSTALLED civitai instead. Measured while writing
    # dogfood_ship_verdict_test.go: the "no civitai in the container" arm ran a
    # REAL, credentialed "civitai app status --json" against the operator's
    # account and came back with 100 rows. It is a read, so nothing was changed --
    # but the arm was measuring the operator's machine, and a verdict-shaped test
    # must never be able to reach a live account by accident.
    case "$raw" in
      *"command -v node"*)    [ "${STUB_NODE:-1}" = "1" ]    || exit 1 ;;
      *"command -v civitai"*) [ "${STUB_CIVITAI:-1}" = "1" ] || exit 1 ;;
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
	// Extra files written ALONGSIDE index.html in the served directory, keyed by
	// relative path. 🔴 It exists because an inline `<script>` and an external
	// `.js` are two different resources to a browser, and the oracle's picker
	// patch reads a RESPONSE: a fixture that only ever inlines its script would
	// leave the Script path — which is the one every real built bundle takes —
	// untested. See fxPickerGatedApp in dogfood_oracle_picker_test.go.
	appFiles map[string]string
	noNode   bool
	// The trial's transcript, written to <runs>/ctl/transcript.jsonl. Empty =>
	// no transcript at all, which is its own case: the oracle then has nothing
	// to derive the brief from.
	transcript string
}

// One `start` record as runner.py writes it. `brief` is the prose the model was
// sent; `briefName` is the name runner.py records alongside it.
func startRecord(brief, briefName string) string {
	b, err := json.Marshal(map[string]any{
		"t": 1.0, "kind": "start", "trial": "ctl", "model": "fake/model",
		"image": "fake-image", "user": "root", "agent_env": "",
		"brief": brief, "brief_name": briefName, "container": "dogfood-ctl",
	})
	if err != nil {
		panic(err)
	}
	// Plus an `end` record, so the fixture is a transcript rather than one line.
	return string(b) + "\n" + `{"t":2.0,"kind":"end","stop":"finished"}` + "\n"
}

// The committed prose of a brief, read rather than pinned: these tests are
// about a transcript's recorded brief resolving to the right NAME, so they must
// exercise whatever text a trial run today would actually carry.
func briefText(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(oracleDir, "briefs", name+".brief.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// Builds a stub PATH and a fixture tree, and returns the env for running
// oracle.sh / grade.sh against it.
func stubOracleEnv(t *testing.T, s stubEnv) []string {
	t.Helper()
	// 🔴 NOT t.TempDir(), AND THE REASON IS A VACUOUS GREEN THAT WAS ALREADY
	// LIVE. t.TempDir() builds its path out of the TEST NAME, the stub docker
	// substitutes that path into the oracle's `find /work …` command text, and
	// the oracle writes `/work` unquoted (correctly — on a real daemon it is a
	// fixed path). So a subtest whose NAME contains a shell metacharacter makes
	// every in-"container" command a bash syntax error, `manifests=0`, and
	// `RENDER=no` — which is the expected value of every negative case here.
	// Measured 2026-09-21: `TestOracleSendsTheHostHandshake/without_it_(control)`
	// — the control arm that proves the host injection does anything — was
	// passing for that reason and not for its own. A name-independent directory
	// removes the class rather than the one instance.
	dir, err := os.MkdirTemp("", "dogfood-oracle-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
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
			for rel, body := range s.appFiles {
				write(filepath.Join(out, rel), body, 0o644)
			}
		}
	}
	// 🔴 The stub dir goes FIRST on PATH but the rest of PATH is kept: the stub
	// rewrites container paths and then needs the real `find`, `cat`, `node` and
	// the browser to carry the command out.
	// 🔴 DOGFOOD_RUNS IS ALWAYS SET, EVEN WITH NO TRANSCRIPT. The oracle now
	// derives the brief from `<runs>/<trial>/transcript.jsonl`, whose default is
	// `scripts/dogfood/runs` — a directory a contributor who has driven a real
	// matrix HAS, and whose contents would then decide these tests' verdicts.
	// Pointing it at a temp dir is what keeps them a claim about the oracle.
	runs := filepath.Join(dir, "runs")
	if s.transcript != "" {
		write(filepath.Join(runs, "ctl", "transcript.jsonl"), s.transcript, 0o644)
	} else if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
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
		"DOGFOOD_RUNS=" + runs,
		// Never inherit an operator's shell default for the brief: it would
		// silently become the `requested` arm of every case below.
		"DOGFOOD_ASSERT=",
		// Same for the viewer control arm, which two tests below set explicitly.
		"CIVITAI_ASSERT_ANON_VIEWER=",
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

// ── which brief: derived from the trial, never guessed ───────────────────────

// 🔴 THE BRIEF USED TO BE AN OPTIONAL POSITIONAL DEFAULTING TO `celsius`, AND
// THE COST WAS MEASURED, NOT IMAGINED. `oracle.sh ab-genpost-mimo-01 root` —
// against a trial built from the `genpost` brief — ran the CELSIUS assertion,
// timed out waiting for `[data-testid="celsius"]` and printed `RENDER=no`,
// which is byte-identical to the verdict a model that built nothing earns.
// runner.py records the brief in the trial's own `start` record, so the oracle
// reads it from there.
//
// These cases never LAUNCH a browser — no manifest, so the verdict is "no app
// was created" and no block is ever served. They still call oracleBrowser
// because oracle.sh RESOLVES a browser up front, deliberately, so that an
// unreachable one is reported before a server is started inside someone's
// container. Same skip-locally / fail-under-CI rule as everywhere else here: a
// skip is a green that checked nothing.
func TestOracleDerivesTheBriefFromTheTrial(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		transcript string
		args       []string
		wantBrief  string
		wantSource string
		wantNote   string
	}{
		{
			name:       "no argument at all: the trial's recorded prose decides",
			transcript: startRecord(briefText(t, "genpost"), ""),
			args:       []string{"ctl", "root"},
			wantBrief:  "genpost",
			wantSource: "transcript-text",
		},
		{
			// The field runner.py gained with this change. Preferred over the
			// prose because prose is not an identifier: rewording a brief file
			// un-resolves every trial already run against it.
			name:       "a recorded brief_name is preferred over matching prose",
			transcript: startRecord("", "genpost"),
			args:       []string{"ctl", "root"},
			wantBrief:  "genpost",
			wantSource: "transcript-name",
		},
		{
			name:       "an argument that AGREES is accepted",
			transcript: startRecord(briefText(t, "genpost"), ""),
			args:       []string{"ctl", "root", "genpost"},
			wantBrief:  "genpost",
			wantSource: "transcript-text",
		},
		{
			// A setup cell: the transcript POSITIVELY records an empty brief, so
			// the trial built no app and every brief grades it identically. The
			// default is defensible here — and it has to SAY so, or a reader
			// cannot tell this cell from one graded against a derived brief.
			name:       "a setup trial defaults, and says so out loud",
			transcript: startRecord("", ""),
			args:       []string{"ctl", "root"},
			wantBrief:  "celsius",
			wantSource: "default-trial-recorded-no-brief",
			wantNote:   "records an EMPTY brief",
		},
		{
			// No transcript to check against: the argument is used, and the run
			// states that nothing verified it.
			name:       "no transcript: the argument is used and marked unverified",
			args:       []string{"ctl", "root", "genpost"},
			wantBrief:  "genpost",
			wantSource: "argument-unverified",
			wantNote:   "nothing verified 'genpost'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "-", transcript: tc.transcript,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "brief"); got != tc.wantBrief {
				t.Fatalf("brief=%s, want %s — the oracle graded against the wrong question\n%s",
					got, tc.wantBrief, out)
			}
			if got := summaryField(t, out, "brief_source"); got != tc.wantSource {
				t.Fatalf("brief_source=%s, want %s\n%s", got, tc.wantSource, out)
			}
			if tc.wantNote != "" && !strings.Contains(out, tc.wantNote) {
				t.Fatalf("the run did not state %q — an unverified or defaulted brief must not "+
					"be silent:\n%s", tc.wantNote, out)
			}
		})
	}
}

// 🔴 AND WHEN IT CANNOT BE SURE, IT REFUSES — it does not pick. Every case here
// is one in which a verdict would be a confident statement about the wrong
// question, and exit 2 is what grade.sh renders as `RENDER=unmeasured`.
func TestOracleRefusesRatherThanGradeTheWrongBrief(t *testing.T) {
	for _, tc := range []struct {
		name       string
		transcript string
		args       []string
		wantMsg    string
	}{
		{
			// THE ORIGINAL DEFECT, with the operator now wrong instead of the
			// default. Either way, one of the two beliefs is false.
			name:       "the argument disagrees with the trial",
			transcript: startRecord(briefText(t, "genpost"), ""),
			args:       []string{"ctl", "root", "celsius"},
			wantMsg:    "brief disagreement",
		},
		{
			name:       "the recorded brief matches no brief file",
			transcript: startRecord("Build something nobody committed a brief for.", ""),
			args:       []string{"ctl", "root"},
			wantMsg:    "matches none of",
		},
		{
			// A transcript that names one brief and quotes another was written
			// by hand somewhere. Picking either half is guessing which is the
			// lie.
			name:       "the transcript names one brief and quotes another",
			transcript: startRecord(briefText(t, "celsius"), "genpost"),
			args:       []string{"ctl", "root"},
			wantMsg:    "self-inconsistent",
		},
		{
			// Nothing passed, nothing to derive from. This is exactly where the
			// old default fired, and it is the case with the least information.
			name:    "nothing passed and no transcript to derive from",
			args:    []string{"ctl", "root"},
			wantMsg: "no brief and no way to derive one",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "-", transcript: tc.transcript,
			})
			out, code := runScript(t, "oracle.sh", env, tc.args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2 (nothing measured)\n%s", code, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("the refusal does not say %q:\n%s", tc.wantMsg, out)
			}
			// The property that matters more than the message: a run that could
			// not establish WHICH question to ask must not answer one.
			if strings.Contains(out, "RENDER=") {
				t.Fatalf("a run that could not resolve the brief still printed a verdict:\n%s", out)
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
			// 🔴 POSITIVE CONTROL ON THE FIXTURE TREE, because the expected value
			// of the control arm is `no` and so is the value a run that never
			// found the app produces. Measured 2026-09-21: this subtest's own
			// name once made that happen (see stubOracleEnv). A `no` earned by
			// an empty /work is not evidence about the handshake.
			if got := summaryField(t, out, "app_dirs"); got != "1" {
				t.Fatalf("app_dirs=%s, want 1 — the fixture app was never found, so this arm's "+
					"verdict is about the harness, not the handshake\n%s", got, out)
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

// ── the `genpost` brief ──────────────────────────────────────────────────────

// 🔴 THE ORACLE GRADES THE SECOND BRIEF, AND THE POST GATE IS WHAT IT GRADES ON.
// Both arms are required. A `yes` on the first alone is satisfied by an oracle
// that can reach a block at all; it is the SECOND row — a complete generate-only
// app, with the right test ids, graded `no` — that separates this brief from one
// the `page-money` scaffold already satisfies.
func TestOracleGradesTheGenpostBrief(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		html       string
		wantRender string
		wantReason string
	}{
		{"a generate-then-post app", fxGenpost, "yes", ""},
		{"generate only, no Post control", fxGenpostNoPost, "no", `labelled "post"`},
		{"Post open before anything exists to post", fxGenpostPostEnabled, "no", "is enabled before any generation"},
		// ⚠ The wording moved with the picker change: the wait used to be "the
		// status to leave \"ready\"" and is now "the status to move after clicking
		// generate", because the predicate is now scoped to what follows the
		// Generate click (see genpost.assert.mjs step 7). The VERDICT is unchanged.
		{"Generate does nothing", fxGenpostInertGenerate, "no", "waiting for the status to move after clicking"},
		{"the machine moves but never says generating", fxGenpostWrongWord, "no", `never read "generating"`},
		{"the resting word is wrong", fxGenpostWrongIdle, "no", "at rest, expected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0",
				manifest: fxManifestScoped, outputDir: "dist", appHTML: tc.html,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root", "genpost")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
			if tc.wantReason != "" && !strings.Contains(out, tc.wantReason) {
				t.Fatalf("the reason does not say %q:\n%s", tc.wantReason, out)
			}
		})
	}
}

// 🔴 `observed` IS THE STATUS SEQUENCE, NOT THE FINAL VALUE — and it has to
// reach the cell, for the same reason celsius's `212 °F` does. With a stub host
// every correct app ends in its own failure state a moment after `generating`,
// so a cell carrying only where it ENDED would make every correct app look
// broken. This pins the transition.
func TestOracleCarriesTheGenpostStatusSequence(t *testing.T) {
	browser := oracleBrowser(t)
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0",
		manifest: fxManifestScoped, outputDir: "dist", appHTML: fxGenpost,
	}), "CIVITAI_CHROME="+browser)
	out, code := runScript(t, "oracle.sh", env, "ctl", "root", "genpost")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
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
	if !strings.Contains(obs, "ready") || !strings.Contains(obs, "generating") {
		t.Fatalf("observed = %q, want the transition it recorded — a bare final value cannot "+
			"tell a working app from one whose button does nothing\n%s", obs, out)
	}
	if got := summaryField(t, out, "observed"); !strings.Contains(got, "generating") {
		t.Fatalf("observed=%q on the summary line does not carry the sequence — the cell loses it\n%s", got, out)
	}
}

// 🔴 SCOPES DECIDE NO VERDICT — the same standing as the validate gate, and
// pinned in both directions for the same reason that one is: a field wired as
// part of the verdict passes a one-sided test. A manifest declaring no scopes
// must still be able to grade `yes`, and one declaring both must still be able
// to grade `no`.
//
// ⚠ "Decides no verdict" is NOT "is inert". Since the scope seed landed, the
// declared list is also an INPUT to the host emulation — it picks which branch a
// consent-gated block takes, exactly as a real host's grant would. Those are
// different claims and this test only makes the first;
// TestOracleSeedsTheBlocksDeclaredScopes makes the second, and its own control
// arm is what keeps the two from collapsing into "scopes make things pass".
func TestOracleReportsScopesWithoutDeciding(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		manifest   string
		html       string
		wantScopes string
		wantRender string
	}{
		{"declared, app works", fxManifestScoped, fxGenpost, "ai:write:budgeted,posts:write:self", "yes"},
		{"none declared, app works", fxManifestBuilt, fxGenpost, "none", "yes"},
		{"declared, app does not", fxManifestScoped, fxGenpostNoPost, "ai:write:budgeted,posts:write:self", "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0",
				manifest: tc.manifest, outputDir: "dist", appHTML: tc.html,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root", "genpost")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "scopes"); got != tc.wantScopes {
				t.Fatalf("scopes=%s, want %s (it must be REPORTED)\n%s", got, tc.wantScopes, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s — the verdict followed the manifest, not the browser\n%s",
					got, tc.wantRender, out)
			}
		})
	}
}

// 🔴 THE ORACLE SHOWS THE BLOCK THE SCOPES ITS OWN MANIFEST DECLARES — AND THE
// SECOND HALF IS THE ONE THAT MAKES THIS COVERAGE RATHER THAN A CELEBRATION.
//
// Row 1 is the regression test: a consent-first generate-then-post app graded
// `yes`. It is RED on pre-change code — with `scopes: []` seeded unconditionally
// the Generate click does nothing, the assertion times out and the cell reads
// `RENDER=no observed=ready`, which is byte-identical to the verdict a model
// that built nothing earns. That was measured on `ab-genpost-dsv4-01`.
//
// Row 2 is the control that proves the value comes from the MANIFEST rather than
// from a list somebody baked into the harness: the SAME fixture, a manifest
// declaring nothing, still `no`. Without it, hardcoding `ai:write:budgeted` in
// _cdp.mjs would pass row 1 — and would then grade every block as though it had
// been granted a scope it never asked for.
//
// Rows 3 and 4 are the arm that decides whether the change is shippable at all:
// seeding scopes must not make the oracle PERMISSIVE. A generate-only app with
// the right test ids and an untouched scaffold both still grade `no` with both
// scopes seeded — the same result `ab-genpost-glm-01` (an unmodified
// `page-money` scaffold) gave on the live containers.
func TestOracleSeedsTheBlocksDeclaredScopes(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		manifest   string
		html       string
		wantRender string
		wantObs    string
	}{
		{
			name:     "a consent-gated app is shown its declared scopes and can generate",
			manifest: fxManifestScoped, html: fxGenpostScopeGated,
			wantRender: "yes", wantObs: "ready>generating",
		},
		{
			name:     "the same app, manifest declaring nothing (control)",
			manifest: fxManifestBuilt, html: fxGenpostScopeGated,
			wantRender: "no", wantObs: "ready",
		},
		{
			name:     "scopes do not buy a generate-only app a Post control",
			manifest: fxManifestScoped, html: fxGenpostNoPost,
			wantRender: "no",
		},
		{
			name:     "scopes do not buy an untouched scaffold a pass",
			manifest: fxManifestScoped, html: fxScaffold,
			wantRender: "no",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
				manifest: tc.manifest, outputDir: "dist", appHTML: tc.html,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			// 🔴 POSITIVE CONTROL ON THE FIXTURE TREE. Three of the four rows
			// expect `no`, and so does a run that never found the app — so
			// without this a broken fixture tree would make them pass for a
			// reason that has nothing to do with scopes.
			if got := summaryField(t, out, "app_dirs"); got != "1" {
				t.Fatalf("app_dirs=%s, want 1 — the fixture app was never found, so this arm's "+
					"verdict is about the harness, not the scope seed\n%s", got, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
			// `observed` separates "the click drove the machine" from "the click
			// did nothing", which is the whole difference the seed makes. A bare
			// verdict cannot tell them apart.
			if tc.wantObs != "" {
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
					t.Fatalf("observed = %q, want %q\n%s", obs, tc.wantObs, out)
				}
			}
		})
	}
}

// 🔴 AND THE TWO SIDES OF THE PROCESS BOUNDARY AGREE. `scopes=` on the cell is
// read off the manifest by oracle.sh; `hostScopes` is reported back by the
// assertion that actually seeded the bootstrap. An assertion that ignored its
// scope argument would still produce a cell whose `scopes=` field named a list
// the block never received — a confident, wrong, unnoticeable cell. The oracle
// refuses (exit 2, nothing measured) on a disagreement; this pins that the two
// are in fact wired to each other on the happy path.
func TestOracleAgreesWithTheAssertionOnWhichScopesWereSeeded(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		manifest string
		want     string
	}{
		{fxManifestScoped, "ai:write:budgeted,posts:write:self"},
		{fxManifestBuilt, "none"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
				manifest: tc.manifest, outputDir: "dist", appHTML: fxGenpost,
			}), "CIVITAI_CHROME="+browser)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — a scope seam mismatch exits 2\n%s", code, out)
			}
			if got := summaryField(t, out, "scopes"); got != tc.want {
				t.Fatalf("scopes=%s, want %s\n%s", got, tc.want, out)
			}
			var seeded string
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, `{"assertion"`) {
					var j struct {
						HostScopes string `json:"hostScopes"`
					}
					if err := json.Unmarshal([]byte(line), &j); err != nil {
						t.Fatalf("assertion line is not JSON: %q", line)
					}
					seeded = j.HostScopes
				}
			}
			if seeded != tc.want {
				t.Fatalf("the assertion reported hostScopes=%q, want %q — the cell's scopes= field "+
					"and the list the block was shown are not the same fact\n%s", seeded, tc.want, out)
			}
		})
	}
}

// ── the viewer the oracle presents ───────────────────────────────────────────

// 🔴 THE ORACLE PRESENTS A SIGNED-IN VIEWER, AND THE CONTROL ARM PROVES IT IS
// THE VIEWER DOING THE WORK. Measured on `ab-genpost-mimo-01` (2026-09-21): a
// generate-then-post app that gates its UI on the viewer — the shape this
// repo's own `page-money` scaffold ships — graded `RENDER=no observed=ready`
// under the old `viewer: null` bootstrap, with `Please sign in to generate
// images.` in its body. That verdict was about the harness.
//
// Why signed-in is the RIGHT default rather than the convenient one: the
// platform refuses this brief's behaviour to an anonymous subject (a block
// token minted for an anonymous viewer carries `sub: "anon"`, and civitai's
// block-scope middleware hard-rejects `posts:write:self` for it), so an
// anonymous-only oracle grades a branch in which the thing the brief asks for
// cannot exist. Every other host emulation in the ecosystem agrees: the SDK's
// `createMockHost`, `createLiveHost`'s anon FALLBACK, and this repo's own
// page-money harness (`?viewer=anon` is the opt-IN) all default to signed in.
func TestOracleShowsTheBlockASignedInViewer(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		anon       bool
		wantRender string
		wantViewer string
	}{
		{"signed in (the default)", false, "yes", "signed-in"},
		{"anonymous (the control arm)", true, "no", "anonymous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
				manifest: fxManifestScoped, outputDir: "dist", appHTML: fxGenpostAuthGated,
			}), "CIVITAI_CHROME="+browser)
			if tc.anon {
				env = append(env, "CIVITAI_ASSERT_ANON_VIEWER=1")
			}
			out, code := runScript(t, "oracle.sh", env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s — an auth-gated app is graded on the branch the "+
					"harness's viewer selects\n%s", got, tc.wantRender, out)
			}
			// 🔴 The cell has to SAY which viewer it graded against. Without it a
			// `no` cannot be told from "the model built a sign-in CTA and the
			// harness was anonymous" — which is the whole defect, one layer up.
			if got := summaryField(t, out, "viewer"); got != tc.wantViewer {
				t.Fatalf("viewer=%s, want %s\n%s", got, tc.wantViewer, out)
			}
		})
	}
}

// 🔴 AND THE SEEDED VIEWER AND SCOPES BUY THE BLOCK NOTHING. A host emulation
// that hands a block a viewer AND a usable credential would make the `genpost`
// brief's premise false — briefs/genpost.md says in as many words that no
// generation and no post can complete here, on any machine, and the assertion
// grades the state machine only on that basis. Scopes are a BRANCH, `token.raw`
// is the CAPABILITY, and only the first of those is seeded. This reads the
// bootstrap from INSIDE the page, so it is a claim about what the block receives
// rather than about what _cdp.mjs says.
func TestOracleSeedsTheProductionViewerAndNoCredential(t *testing.T) {
	browser := oracleBrowser(t)
	run := func(t *testing.T, anon bool) (string, string) {
		t.Helper()
		env := append(stubOracleEnv(t, stubEnv{
			state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
			manifest: fxManifestScoped, outputDir: "dist", appHTML: fxGenpostBootstrapProbe,
		}), "CIVITAI_CHROME="+browser)
		if anon {
			env = append(env, "CIVITAI_ASSERT_ANON_VIEWER=1")
		}
		out, code := runScript(t, "oracle.sh", env, "ctl", "root")
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, out)
		}
		return out, summaryField(t, out, "RENDER")
	}

	// POSITIVE CONTROL FIRST: the probe must be able to SEE the bootstrap at
	// all. Without this a probe wired to nothing — one whose `bad` list is
	// always empty because it reads an object that is never there — would make
	// the green arm below vacuous.
	anonOut, anonRender := run(t, true)
	if anonRender != "no" {
		t.Fatalf("the bootstrap probe passed with an anonymous viewer — it cannot be reading the "+
			"viewer at all, so its green arm proves nothing\n%s", anonOut)
	}
	if !strings.Contains(anonOut, "viewer-null") {
		t.Fatalf("the probe failed for some reason other than the anonymous viewer — it is not "+
			"measuring what this test claims:\n%s", anonOut)
	}

	out, render := run(t, false)
	if render != "yes" {
		t.Fatalf("RENDER=%s, want yes. The probe drives the status machine only when the seeded "+
			"bootstrap is exactly what a real host sends; `observed` names the field that is "+
			"wrong — a `viewer-keys:` value means the fake is wider or narrower than "+
			"civitai.com's own withSignedInFlag(), a `token-scopes:` value means the manifest's "+
			"declared scopes did not reach the block (the oracle.sh -> assert.mjs argv -> "+
			"hostBootstrap plumbing), and `token-raw-nonempty` means the oracle handed the block "+
			"a credential and briefs/genpost.md's premise is void.\n%s", render, out)
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
	// `brief_source=` and `viewer=` are on the cell for the same reason
	// `observed=` is: without them a reader cannot tell a verdict measured
	// against a brief the TRIAL recorded from one measured against a brief
	// somebody typed, nor a `no` earned signed-in from a `no` earned anonymous.
	for _, want := range []string{
		"render_brief=celsius", "brief_source=", "validate_gate=pass",
		"viewer=signed-in", "observed=212", "RENDER=yes",
	} {
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
