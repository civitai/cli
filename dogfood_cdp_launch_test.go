package cli_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The BROWSER LAUNCH half of the render oracle (`launch()` in
// scripts/dogfood/briefs/_cdp.mjs), and the reason it has its own file.
//
// # What happened
//
// `build-test` went red intermittently on every PR and on `main`, with NO
// failing test name anywhere in the log. Measured 2026-09-21 across four runs —
// 35557830290, 35563154222, 35567753483, 35616180491 — one of them a docs-only
// PR changing a single markdown file:
//
//	{"assertion":"celsius","pass":false,
//	 "reason":"harness error: browser never printed a DevTools endpoint:
//	   [ERROR:dbus/bus.cc:405] Failed to connect to the bus:
//	    Could not parse server address: Unknown address type …"}
//	oracle: the assertion could not run (exit 2) — nothing was measured
//	FAIL github.com/civitai/cli
//
// In a 31 KB log that produced zero `--- FAIL:` lines and `ok` for every other
// package. Everything downstream behaved CORRECTLY: the assertion reported a
// harness error rather than a verdict, oracle.sh turned that into exit 2 /
// "nothing was measured" rather than a false `RENDER=no`, and the oracle tests
// refused to skip under CI. The defect was upstream of all of it — the browser
// never came up, and nothing was built to notice that as its own event.
//
// # What was and was not established
//
//   - ESTABLISHED, reproducible: the runner's `DBUS_SESSION_BUS_ADDRESS` holds
//     a transport type libdbus cannot parse. Setting it to `bogus:path=/nope`
//     on a developer box reproduces that stderr byte for byte.
//   - ESTABLISHED, measured in the failing jobs themselves: a GitHub runner is
//     100–300× slower than a warm dev box at the cheapest possible browser
//     invocation (`chromium --version`: 1.9–6.5 s on CI, 0.02 s locally) with a
//     3.4× spread run to run. ⚠ It does not separate pass from fail (2.2 s
//     passed, 2.8 s failed), so it is evidence about the environment's speed
//     class, not a per-run predictor.
//   - ESTABLISHED: `--password-store=basic` removes exactly one session-bus
//     round trip from startup (4 `dbus/bus.cc` errors → 3), the last one before
//     the DevTools line. Three of the four failures stalled after exactly three
//     such errors; the fourth stalled after one, so this is not the whole
//     mechanism and is not claimed to be.
//   - ESTABLISHED from the runner image's own installer: `/usr/bin/chromium` on
//     `ubuntu-latest` is a raw `chromium-browser-snapshots` build, not a
//     release, while a release `google-chrome` sits on the same disk.
//   - NOT ESTABLISHED: the stall was never reproduced locally. Everything below
//     is therefore an INVARIANT GUARD on the launch's behaviour, not a
//     regression test — none of it was watched to fail on pre-change code,
//     because pre-change code passes on any machine that can start a browser.
//     The one thing that IS a regression test is the orphan below, which was
//     visible in all four CI logs as the runner's own
//     `Terminate orphan process: pid (…) (chrome)`.
//
// # Why a fake browser
//
// The properties worth pinning — bounded attempts, a retry that announces
// itself, the process being killed, the flags being passed — are properties of
// `launch()`, not of Chromium. A fake browser makes each of them deterministic
// and keeps them runnable on a machine with no browser at all. The one test
// that needs a real browser says so and uses `oracleBrowser`, which skips
// locally and FAILS under CI like everything else here.

const cdpModule = "scripts/dogfood/briefs/_cdp.mjs"

// fakeBrowser writes an executable that stands in for Chromium.
//
// Each invocation appends its own argv to `<dir>/calls.log` (one line per call,
// so the test can COUNT attempts rather than infer them) and then behaves
// according to `okFrom`: invocations before it print junk and hang, invocations
// from it on print a DevTools endpoint and hang. `okFrom = 0` never succeeds.
//
// It hangs rather than exits on the failing path ON PURPOSE: an immediate exit
// takes the `browser exited N` branch, and the branch this arc is about is the
// browser that is ALIVE and silent.
func fakeBrowser(t *testing.T, dir string, okFrom int) string {
	t.Helper()
	path := filepath.Join(dir, "fake-browser")
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %q
n=$(wc -l < %q)
echo "fake browser call $n, argv: $@" >&2
if [ %d -gt 0 ] && [ "$n" -ge %d ]; then
  echo "DevTools listening on ws://127.0.0.1:1/devtools/browser/fake-$n" >&2
fi
exec sleep 120
`, filepath.Join(dir, "calls.log"), filepath.Join(dir, "calls.log"), okFrom, okFrom)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// cdpLaunchDriver writes a node program that calls `launch()` once and reports
// what happened on stdout, so a Go test can assert on it.
func cdpLaunchDriver(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(cdpModule)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "driver.mjs")
	body := fmt.Sprintf(`import { launch } from %q;
try {
  const { proc, ws } = await launch();
  console.log('LAUNCHED ' + ws + ' pid=' + proc.pid);
  proc.kill('SIGKILL');
  process.exit(0);
} catch (e) {
  console.log('LAUNCH-FAILED');
  console.log(e.message);
  process.exit(1);
}
`, abs)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type launchRun struct {
	stdout string
	stderr string
	code   int
	calls  []string
}

// runCdpLaunch drives `launch()` against a fake browser and returns everything
// observable about the run, including one entry per browser invocation.
func runCdpLaunch(t *testing.T, okFrom int, env ...string) launchRun {
	t.Helper()
	node := dogfoodTool(t, "node")

	// Not t.TempDir(): its path is built from the test NAME and this directory
	// is substituted into a `/bin/sh` script. A subtest name carrying a shell
	// metacharacter would make every fake-browser invocation a syntax error and
	// every assertion below pass for the wrong reason — the exact vacuous green
	// stubOracleEnv documents.
	dir, err := os.MkdirTemp("", "cdp-launch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	bin := fakeBrowser(t, dir, okFrom)
	driver := cdpLaunchDriver(t, dir)

	cmd := exec.Command(node, driver)
	cmd.Env = append(os.Environ(),
		append([]string{
			"CIVITAI_CHROME=" + bin,
			// Small enough that a two-attempt failure costs ~1.6s, large
			// enough that the succeeding arm is not a race against process
			// startup on a loaded machine.
			"CIVITAI_ASSERT_LAUNCH_MS=800",
		}, env...)...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running the launch driver: %v\nstdout:\n%s\nstderr:\n%s", err, outBuf.String(), errBuf.String())
	}

	var calls []string
	if raw, rerr := os.ReadFile(filepath.Join(dir, "calls.log")); rerr == nil {
		for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
			if l != "" {
				calls = append(calls, l)
			}
		}
	}
	return launchRun{stdout: outBuf.String(), stderr: errBuf.String(), code: code, calls: calls}
}

// ── the retry: bounded, and never silent ─────────────────────────────────────

// 🔴 A RETRY IS AN EXCELLENT WAY TO STOP NOTICING A BROKEN BROWSER. The reason
// this one is acceptable is that its SUCCESS is as loud as its failure: a run
// whose browser needed a second attempt prints `BROWSER LAUNCH RETRY` with the
// first attempt's diagnostics, and ci.yml's smoke step turns that string into a
// `::warning`. Without that line a browser degrading from "always works" to
// "works half the time" would be absorbed into a green and nobody would learn
// about it until it degraded again.
//
// This test is what makes that claim checkable: it is the ONLY thing standing
// between the retry and being exactly the anti-pattern the previous author
// declined to add.
func TestCdpLaunchRetriesOnceAndAnnouncesItEvenThoughItSucceeded(t *testing.T) {
	r := runCdpLaunch(t, 2) // fails on call 1, succeeds on call 2

	if r.code != 0 {
		t.Fatalf("exit %d, want 0 — the second attempt printed an endpoint\nstdout:\n%s\nstderr:\n%s",
			r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stdout, "LAUNCHED ws://") {
		t.Fatalf("the driver did not report a launch:\n%s", r.stdout)
	}
	if len(r.calls) != 2 {
		t.Fatalf("the browser was invoked %d time(s), want exactly 2 (one failure, one success): %v",
			len(r.calls), r.calls)
	}
	if !strings.Contains(r.stderr, "BROWSER LAUNCH RETRY") {
		t.Fatalf("a launch that needed a retry did not say so. ci.yml's browser-smoke step greps for "+
			"`BROWSER LAUNCH RETRY` to raise a ::warning; without it a browser that fails half the "+
			"time is absorbed into a green run.\nstderr:\n%s", r.stderr)
	}
	// The warning must carry the FAILED attempt's evidence, not just the fact
	// of a retry — "it retried" is not actionable, "it retried because the
	// browser said X" is.
	if !strings.Contains(r.stderr, "attempt 1/2 failed") {
		t.Errorf("the retry warning does not carry attempt 1's diagnostics:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "fake browser call 1") {
		t.Errorf("the retry warning does not carry what the browser itself printed on the failed "+
			"attempt:\n%s", r.stderr)
	}
}

// The bound, and the loudness of a genuine failure. A browser that never works
// must not be retried forever and must not produce a quiet error: every attempt
// has to be in the message the oracle turns into `nothing was measured`.
func TestCdpLaunchIsBoundedAndStillFailsLoudly(t *testing.T) {
	r := runCdpLaunch(t, 0) // never succeeds

	if r.code == 0 {
		t.Fatalf("exit 0 for a browser that never printed an endpoint — the retry swallowed a real "+
			"failure, which is the precise thing it must never do\nstdout:\n%s", r.stdout)
	}
	if len(r.calls) != 2 {
		t.Fatalf("the browser was invoked %d time(s), want exactly 2 (the default bound). An unbounded "+
			"retry burns the job's whole budget and reports a timeout instead of a browser failure: %v",
			len(r.calls), r.calls)
	}
	// 🔴 THE PHRASE IS LOAD-BEARING. The assertion wraps this message as its
	// `harness error:` reason, oracle.sh reports it verbatim under exit 2, and
	// it is what a reader greps for when a job goes red with no failing test.
	if !strings.Contains(r.stdout, "browser never printed a DevTools endpoint") {
		t.Errorf("the failure message lost the phrase every log, doc and grep for this class uses:\n%s", r.stdout)
	}
	for _, want := range []string{"attempt 1/2 failed", "attempt 2/2 failed", "fake browser call 1", "fake browser call 2"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the failure message does not contain %q — a reader cannot tell one attempt's "+
				"failure from the other's:\n%s", want, r.stdout)
		}
	}
}

// The bound is a DEFAULT, not a constant: ci.yml's negative control sets it to
// 1 so the failing path costs one budget rather than two. If the env var stops
// being honoured that control silently doubles in cost and stops being the
// single-attempt measurement it claims to be.
func TestCdpLaunchAttemptsAreTunable(t *testing.T) {
	r := runCdpLaunch(t, 0, "CIVITAI_ASSERT_LAUNCH_ATTEMPTS=1")
	if len(r.calls) != 1 {
		t.Fatalf("with CIVITAI_ASSERT_LAUNCH_ATTEMPTS=1 the browser was invoked %d time(s), want 1: %v",
			len(r.calls), r.calls)
	}
	if r.code == 0 {
		t.Fatalf("exit 0 for a browser that never printed an endpoint:\n%s", r.stdout)
	}
}

// ── the orphan: a regression test, and the only one here ─────────────────────

// 🔴 THIS ONE WAS MEASURED IN PRODUCTION. All four failing CI runs end with the
// runner's own cleanup line — `Terminate orphan process: pid (7798) (chrome)`,
// plus two crashpad handlers — because the old `launch()` rejected its promise
// and walked away from the process it had started. That was merely untidy while
// nothing retried. With a retry it is a correctness problem: a hung browser left
// holding a profile and CPU makes attempt 2 an experiment attempt 1 spoiled.
func TestCdpLaunchKillsABrowserItGaveUpOn(t *testing.T) {
	node := dogfoodTool(t, "node")

	dir, err := os.MkdirTemp("", "cdp-orphan-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	pidFile := filepath.Join(dir, "pids")
	bin := filepath.Join(dir, "fake-browser")
	// `exec sleep` so the recorded pid IS the process that survives — a shell
	// that forks a child would make this test pass by killing the wrapper while
	// the real sleep lived on, which is the failure it exists to catch.
	script := fmt.Sprintf("#!/bin/sh\necho $$ >> %q\necho 'silent browser' >&2\nexec sleep 120\n", pidFile)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	driver := cdpLaunchDriver(t, dir)
	cmd := exec.Command(node, driver)
	cmd.Env = append(os.Environ(),
		"CIVITAI_CHROME="+bin,
		"CIVITAI_ASSERT_LAUNCH_MS=600",
		"CIVITAI_ASSERT_LAUNCH_ATTEMPTS=2",
	)
	out, _ := cmd.CombinedOutput()

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the fake browser never ran, so this test measured nothing: %v\n%s", err, out)
	}
	pids := strings.Fields(string(raw))
	if len(pids) != 2 {
		t.Fatalf("expected 2 browser invocations, got %d (%v)\n%s", len(pids), pids, out)
	}

	for _, p := range pids {
		pid, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("unreadable pid %q", p)
		}
		// The kill is a signal, not a join: give the process a moment to go.
		gone := false
		for i := 0; i < 50; i++ {
			if err := syscall.Kill(pid, 0); err != nil {
				gone = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !gone {
			_ = syscall.Kill(pid, syscall.SIGKILL) // do not leak out of the test either
			t.Errorf("browser pid %d was still alive after launch() gave up on it. The old code did "+
				"exactly this — every failing CI run ended with the runner's own `Terminate orphan "+
				"process: pid (…) (chrome)`.", pid)
		}
	}
}

// ── the flags ────────────────────────────────────────────────────────────────

// The flag whose effect was actually measured (4 `dbus/bus.cc` startup errors →
// 3, removing the keyring probe that is the last session-bus call before the
// DevTools line). Asserted by reading the browser's OWN argv rather than the
// source, so a refactor that stops passing it fails here even if the constant
// survives.
//
// ⚠ AN INVARIANT GUARD, NOT A REGRESSION TEST. The stall was never reproduced
// locally, so this was never watched to fail on pre-change code; what it pins
// is that a deliberate, measured flag does not quietly go away.
func TestCdpLaunchAsksForNoSessionBusServices(t *testing.T) {
	r := runCdpLaunch(t, 1) // succeeds first time; we only want its argv
	if len(r.calls) != 1 {
		t.Fatalf("expected one browser invocation, got %d: %v", len(r.calls), r.calls)
	}
	argv := r.calls[0]
	for _, want := range []string{
		"--password-store=basic", // the measured one
		"--disable-background-networking",
		"--disable-component-update",
		"--headless=new",
		"--remote-debugging-port=0",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("the browser was not started with %s.\nargv: %s\n"+
				"  WHY --password-store=basic: the runner's DBUS_SESSION_BUS_ADDRESS is unparseable "+
				"(reproduced with `bogus:path=/nope`), and this flag removes the keyring probe — the "+
				"last session-bus round trip before the DevTools endpoint is printed. Measured on "+
				"chromium 153: 4 `dbus/bus.cc` errors without it, 3 with it.", want, argv)
		}
	}
	// A profile per attempt, not a shared one: a stalled attempt leaves a
	// half-written profile and reusing it makes the retry a worse experiment
	// than the attempt it follows.
	if !strings.Contains(argv, "--user-data-dir=") {
		t.Errorf("the browser was started without a private profile:\n%s", argv)
	}
}

// The end-to-end version of the same claim, against a REAL browser under the
// exact environment the CI failures reported: a session-bus address no dbus
// implementation can parse. Skips locally without a browser, FAILS under CI —
// the same rule as every other test in this suite.
//
// ⚠ Also an invariant guard. It passes on pre-change code too, because a
// developer box starts a browser in ~0.1 s whatever its bus looks like. What it
// buys is that the CI environment's defining property is exercised at all.
func TestCdpLaunchSurvivesAnUnparseableSessionBus(t *testing.T) {
	browser := oracleBrowser(t)
	node := dogfoodTool(t, "node")

	dir, err := os.MkdirTemp("", "cdp-dbus-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	driver := cdpLaunchDriver(t, dir)
	cmd := exec.Command(node, driver)
	cmd.Env = append(os.Environ(),
		"CIVITAI_CHROME="+browser,
		// The exact shape behind `Could not parse server address: Unknown
		// address type` in every one of the four failing runs.
		"DBUS_SESSION_BUS_ADDRESS=bogus:path=/nope",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s could not be launched with an unparseable DBUS_SESSION_BUS_ADDRESS: %v\n%s",
			browser, err, out)
	}
	if !strings.Contains(string(out), "LAUNCHED ws://") {
		t.Fatalf("no DevTools endpoint under an unparseable session bus:\n%s", out)
	}
}

// ── one rule, four places ────────────────────────────────────────────────────

var (
	shellBrowserLoopRe = regexp.MustCompile(`(?m)^\s*for b in ([a-z0-9 \-]+); do`)
	cdpBrowserNamesRe  = regexp.MustCompile(`(?s)export const BROWSER_NAMES = \[(.*?)\];`)
	quotedNameRe       = regexp.MustCompile(`'([a-z0-9\-]+)'`)
)

// 🔴 THE BROWSER PREFERENCE ORDER EXISTS FOUR TIMES, IN FOUR LANGUAGES, AND A
// DISAGREEMENT IS INVISIBLE. If ci.yml resolves Chrome into CIVITAI_CHROME but
// `oracleBrowser` looks up Chromium first, a contributor's local run and CI
// drive DIFFERENT browsers while every log says the same thing. That is not
// hypothetical: the order CHANGED in this commit, in four files, and three of
// the four are not Go and none of the four is checked by anything else.
//
// It pins the whole ordered list, not merely set membership: order is the
// entire content of a preference.
func TestEveryBrowserResolverAgreesOnTheSameOrder(t *testing.T) {
	want := strings.Join(oracleBrowserNames, " ")

	read := func(p string) string {
		t.Helper()
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v — this guard checked nothing", p, err)
		}
		return string(raw)
	}

	// The two shell copies, matched by the loop that walks the list.
	for _, p := range []string{
		filepath.Join(oracleDir, "oracle.sh"),
		filepath.Join(workflowDir, "ci.yml"),
	} {
		body := read(p)
		m := shellBrowserLoopRe.FindAllStringSubmatch(body, -1)
		if len(m) != 1 {
			t.Fatalf("%s: found %d `for b in …; do` browser loops, want exactly 1. Either the resolver "+
				"moved or a second one appeared; a guard that matches nothing here passes vacuously.",
				p, len(m))
		}
		if got := strings.Join(strings.Fields(m[0][1]), " "); got != want {
			t.Errorf("%s resolves browsers in the order\n  %s\nbut dogfood_oracle_test.go's "+
				"oracleBrowserNames says\n  %s\n%s", p, got, want, browserOrderRationale())
		}
	}

	// The JS copy.
	body := read(cdpModule)
	m := cdpBrowserNamesRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("%s: no `export const BROWSER_NAMES = [ … ];` — the resolver was renamed or moved, so "+
			"this guard checked nothing. Re-point it rather than deleting it.", cdpModule)
	}
	var names []string
	for _, q := range quotedNameRe.FindAllStringSubmatch(m[1], -1) {
		names = append(names, q[1])
	}
	if got := strings.Join(names, " "); got != want {
		t.Errorf("%s resolves browsers in the order\n  %s\nbut dogfood_oracle_test.go's "+
			"oracleBrowserNames says\n  %s\n%s", cdpModule, got, want, browserOrderRationale())
	}

	// Positive control on the list itself: an empty or one-entry ledger would
	// make every comparison above trivially true.
	if len(oracleBrowserNames) < 3 {
		t.Fatalf("oracleBrowserNames holds %d name(s); with that few the comparisons above assert almost "+
			"nothing.", len(oracleBrowserNames))
	}
	if oracleBrowserNames[0] != "google-chrome" {
		t.Errorf("the first-preference browser is %q, not google-chrome.\n%s",
			oracleBrowserNames[0], browserOrderRationale())
	}
}

func browserOrderRationale() string {
	return "  WHY release-builds-first (measured 2026-09-21 from the runner image's own installer,\n" +
		"  actions/runner-images images/ubuntu/scripts/build/install-google-chrome.sh):\n" +
		"  `ubuntu-latest` carries BOTH. `/usr/bin/google-chrome` is a google-chrome-stable .deb —\n" +
		"  a release, 152.0.7977.82. `/usr/bin/chromium` is a symlink into /usr/local/share/chromium/,\n" +
		"  unzipped from the chromium-browser-snapshots bucket at whatever per-commit revision sits\n" +
		"  nearest Chrome's — 152.0.7977.0, never release-qualified. The old order named chromium first.\n" +
		"  ⚠ This is a determinism argument, not a measurement: no run has shown the snapshot launching\n" +
		"  less reliably than the release.\n" +
		"  AND WHY ALL FOUR MUST AGREE: ci.yml resolves the browser into CIVITAI_CHROME, but oracle.sh,\n" +
		"  _cdp.mjs and oracleBrowser each resolve independently when it is unset. A disagreement means a\n" +
		"  contributor's local run and CI drive different browsers while every log reads identically."
}
