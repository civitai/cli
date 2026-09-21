package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The brief-injection path of the blind dogfood harness (scripts/dogfood).
//
// A trial's ENTIRE task is the user message runner.py sends: a hosted URL, and
// — since the app-build arc — one operator-typed line of brief. Two things can
// go wrong silently and neither shows up in a grade:
//
//   - the brief stops being delivered, and every cell of an app matrix comes
//     back red for a reason that looks exactly like "the model cannot build an
//     app";
//   - the no-brief task drifts, and the setup matrix already measured stops
//     being comparable to anything run afterwards.
//
// So these tests assert the BYTES of the model-facing message and the contents
// of the transcript, not the presence of a flag in a source file. The literals
// below are pinned deliberately: deriving them from runner.py would make the
// test agree with whatever runner.py says, which is not a contract.
const (
	dogfoodHostedPrompt = "https://developer.civitai.com/agent-setup/prompt.md"
	dogfoodDir          = "scripts/dogfood"
)

// python3 is what runner.py is written in. A machine without it cannot exercise
// these paths at all.
//
// 🔴 A SKIP IS A GREEN THAT CHECKED NOTHING, AND `go test ./...` PRINTS NO SKIP
// REASON WITHOUT -v. So on a contributor's machine this skips (with the reason,
// under -v) rather than failing for an unrelated missing tool — but under CI it
// FAILS, because there the skip and the pass are indistinguishable in the log
// the merge gate is read from, and the runner image is not something this repo
// gets to be uncertain about. Same reasoning as ready-ack-runtime's refusal to
// skip when node is missing.
func dogfoodTool(t *testing.T, names ...string) string {
	t.Helper()
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	msg := "none of " + strings.Join(names, "/") + " on PATH — these tests exercise " +
		"scripts/dogfood, so without it THIS SUITE CHECKED NOTHING. That is a " +
		"statement about this machine, not about the harness."
	if os.Getenv("CI") != "" {
		t.Fatal(msg + " Under CI that is a defect: build-test would report `ok` having run nothing.")
	}
	t.Skip(msg)
	return ""
}

func dogfoodPython(t *testing.T) string {
	t.Helper()
	return dogfoodTool(t, "python3", "python")
}

func printTask(t *testing.T, args ...string) (string, int) {
	t.Helper()
	py := dogfoodPython(t)
	cmd := exec.Command(py, append([]string{filepath.Join(dogfoodDir, "runner.py"), "--print-task"}, args...)...)
	out, err := cmd.Output()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running runner.py --print-task: %v", err)
	}
	return string(out), code
}

// The no-brief task must be the hosted URL and nothing else — no trailing
// newline, no framing, no separator. An app-build trial is a setup trial plus
// one paragraph; that is what keeps the two grids comparable.
func TestDogfoodTaskDefaultIsByteIdentical(t *testing.T) {
	got, code := printTask(t)
	if code != 0 {
		t.Fatalf("--print-task exited %d, want 0", code)
	}
	if got != dogfoodHostedPrompt {
		t.Fatalf("default task changed.\n got: %q\nwant: %q\n\nEvery setup trial already measured was run against the "+
			"`want` bytes. Changing them silently re-bases the grid.", got, dogfoodHostedPrompt)
	}
}

// An empty or whitespace-only --brief is the same as no brief. Without this,
// a driver that always passes --brief "$BRIEF" would ship a task with two
// trailing newlines on every setup run.
func TestDogfoodEmptyBriefIsTheDefaultTask(t *testing.T) {
	for _, b := range []string{"", "   ", "\t "} {
		got, code := printTask(t, "--brief", b)
		if code != 0 {
			t.Fatalf("--brief %q exited %d, want 0", b, code)
		}
		if got != dogfoodHostedPrompt {
			t.Fatalf("--brief %q produced %q, want the default task %q", b, got, dogfoodHostedPrompt)
		}
	}
}

func TestDogfoodTaskCarriesTheBrief(t *testing.T) {
	const brief = `Build a thing with data-testid="x".`
	got, code := printTask(t, "--brief", brief)
	if code != 0 {
		t.Fatalf("--print-task --brief exited %d, want 0", code)
	}
	want := dogfoodHostedPrompt + "\n\n" + brief
	if got != want {
		t.Fatalf("brief not delivered verbatim.\n got: %q\nwant: %q", got, want)
	}
	// The brief is appended RAW. A framing sentence would be Civitai knowledge
	// the harness injected into a trial whose premise is that the only such
	// knowledge is the URL and what the operator typed.
	if strings.Count(got, "\n") != 2 {
		t.Fatalf("the task gained lines beyond the URL and the brief: %q", got)
	}
}

// A multi-line brief is refused. The brief is operator-typed text; a value with
// newlines in it is almost always a file that got splatted onto the command
// line, which is the one route by which repo content could reach a trial whose
// blindness is otherwise a mount namespace.
func TestDogfoodRejectsMultilineBrief(t *testing.T) {
	_, code := printTask(t, "--brief", "line one\nline two")
	if code != 2 {
		t.Fatalf("multi-line --brief exited %d, want 2 (argparse usage error)", code)
	}
}

// --model/--image/--trial stopped being argparse-`required` so that
// --print-task could run without them. A real run must still refuse, with the
// same exit code as before.
func TestDogfoodRunStillRequiresModelImageTrial(t *testing.T) {
	py := dogfoodPython(t)
	for _, omit := range []string{"--model", "--image", "--trial"} {
		// --out points at a temp dir so that a REGRESSION here (the runner
		// proceeding past the check) writes its transcript there instead of
		// littering the repo root with a `x/` directory.
		args := []string{filepath.Join(dogfoodDir, "runner.py"), "--out", t.TempDir()}
		for _, f := range []string{"--model", "--image", "--trial"} {
			if f != omit {
				args = append(args, f, "x")
			}
		}
		out, err := exec.Command(py, args...).CombinedOutput()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 2 {
			t.Fatalf("omitting %s: exit %v, want 2\n%s", omit, err, out)
		}
		if !strings.Contains(string(out), omit) {
			t.Fatalf("omitting %s: the error does not name it:\n%s", omit, out)
		}
	}
}

// The end-to-end claim: the brief reaches the JSON that leaves for the model,
// AND it is recorded in the transcript, so a graded cell can be traced back to
// the brief it was run with. Driven offline by testdata/fake_trial.py — no
// Docker, no OpenRouter, no money.
func TestDogfoodRunnerDeliversAndRecordsTheBrief(t *testing.T) {
	py := dogfoodPython(t)
	const brief = `Build a Celsius converter with data-testid="celsius".`

	for _, tc := range []struct {
		name  string
		brief string
		want  string
	}{
		{"with a brief", brief, dogfoodHostedPrompt + "\n\n" + brief},
		{"without one", "", dogfoodHostedPrompt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "capture.json")
			args := []string{
				filepath.Join(dogfoodDir, "testdata", "fake_trial.py"),
				filepath.Join(dogfoodDir, "runner.py"),
				filepath.Join(dir, "runs"), capture,
			}
			if tc.brief != "" {
				args = append(args, tc.brief)
			}
			cmd := exec.Command(py, args...)
			// 🔴 fake_trial.py IMPORTS runner.py, and an import consults
			// __pycache__. CPython validates that cache on the source's mtime
			// in whole SECONDS plus its size, so a same-length edit landing in
			// the same second as the last import runs the ORIGINAL bytecode —
			// which would score a mutation SURVIVED without the mutant ever
			// executing. Nothing here may be cached.
			cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fake trial failed: %v\n%s", err, out)
			}

			// (a) what the model would have been sent
			raw, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			var cap struct {
				Payload struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(raw, &cap); err != nil {
				t.Fatal(err)
			}
			var user string
			for _, m := range cap.Payload.Messages {
				if m.Role == "user" {
					user = m.Content
				}
			}
			if user != tc.want {
				t.Fatalf("user message sent to the model:\n got: %q\nwant: %q", user, tc.want)
			}

			// (b) what the transcript records
			tr, err := os.ReadFile(filepath.Join(dir, "runs", "faketrial", "transcript.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			var sawStart, sawUser bool
			for _, line := range strings.Split(strings.TrimSpace(string(tr)), "\n") {
				var r struct {
					Kind    string  `json:"kind"`
					Brief   *string `json:"brief"`
					Content string  `json:"content"`
				}
				if err := json.Unmarshal([]byte(line), &r); err != nil {
					t.Fatalf("transcript line is not JSON: %q", line)
				}
				switch r.Kind {
				case "start":
					sawStart = true
					// Recorded UNCONDITIONALLY, empty string included: a missing
					// key is indistinguishable from a transcript written by an
					// older runner, so absence would not be evidence of a
					// setup trial.
					if r.Brief == nil {
						t.Fatal(`the "start" record carries no "brief" field — a graded cell cannot be traced to its brief`)
					}
					if *r.Brief != tc.brief {
						t.Fatalf("start.brief = %q, want %q", *r.Brief, tc.brief)
					}
				case "user":
					sawUser = true
					if r.Content != tc.want {
						t.Fatalf("transcript user.content = %q, want %q", r.Content, tc.want)
					}
				}
			}
			if !sawStart || !sawUser {
				t.Fatalf("transcript is missing records (start=%v user=%v):\n%s", sawStart, sawUser, tr)
			}
		})
	}
}

// ── driver.sh ────────────────────────────────────────────────────────────────

// Run driver.sh in a throwaway copy of scripts/dogfood with a STUB python3
// first on PATH, so one cell's argv is captured instead of a container being
// started. Returns the stub's recorded argv, the exit code and the combined
// output.
func runStubbedDriver(t *testing.T, env []string) (argv []string, code int, out string) {
	t.Helper()
	bash := dogfoodTool(t, "bash")
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(dogfoodDir, "driver.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "driver.sh"), src, 0o755); err != nil {
		t.Fatal(err)
	}
	stubDir := filepath.Join(dir, "stub")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Prints its own argv, one per line. driver.sh redirects it to
	// logs/<trial>.out, which is where we read it back from.
	stub := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"
	if err := os.WriteFile(filepath.Join(stubDir, "python3"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bash, filepath.Join(dir, "driver.sh"))
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	b, err := cmd.CombinedOutput()
	out = string(b)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("driver.sh: %v\n%s", err, out)
	}

	logs, _ := filepath.Glob(filepath.Join(dir, "logs", "*.out"))
	for _, l := range logs {
		raw, err := os.ReadFile(l)
		if err != nil {
			t.Fatal(err)
		}
		argv = append(argv, strings.TrimSuffix(filepath.Base(l), ".out"))
		argv = append(argv, strings.Split(strings.TrimSpace(string(raw)), "\n")...)
	}
	return argv, code, out
}

// One model x one env x one identity, so a stubbed matrix is a single cell.
var oneCellEnv = []string{
	"DOGFOOD_MODELS=fake/model|mshort",
	"DOGFOOD_ENVS=fake-image|eshort|root",
	"DOGFOOD_IDENTITIES=idshort|",
	"FULL_CROSS=0",
}

// The default matrix is untouched: no --brief reaches the runner, and the trial
// id keeps the `t-` namespace every already-graded setup cell lives in.
func TestDogfoodDriverDefaultPassesNoBrief(t *testing.T) {
	argv, code, out := runStubbedDriver(t, oneCellEnv)
	if code != 0 {
		t.Fatalf("driver.sh exited %d, want 0\n%s", code, out)
	}
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, "--brief") {
		t.Fatalf("the default matrix passed --brief: %v", argv)
	}
	if !strings.Contains(joined, "t-mshort-eshort-idshort") {
		t.Fatalf("trial id is not the default `t-` namespace: %v", argv)
	}
}

// With a brief, every cell carries it verbatim and lands in the namespace the
// operator named.
func TestDogfoodDriverPassesTheBrief(t *testing.T) {
	const brief = `Build a converter with data-testid="celsius".`
	argv, code, out := runStubbedDriver(t, append([]string{
		"DOGFOOD_BRIEF=" + brief,
		"DOGFOOD_TRIAL_PREFIX=ta",
	}, oneCellEnv...))
	if code != 0 {
		t.Fatalf("driver.sh exited %d, want 0\n%s", code, out)
	}
	var sawBrief bool
	for i, a := range argv {
		if a == "--brief" {
			if i+1 >= len(argv) || argv[i+1] != brief {
				t.Fatalf("--brief value is not the brief: %v", argv)
			}
			sawBrief = true
		}
	}
	if !sawBrief {
		t.Fatalf("DOGFOOD_BRIEF did not reach the runner: %v", argv)
	}
	if !strings.Contains(strings.Join(argv, " "), "ta-mshort-eshort-idshort") {
		t.Fatalf("DOGFOOD_TRIAL_PREFIX did not reach the trial id: %v", argv)
	}
}

// 🔴 The guard that fails CLOSED. An app matrix under the setup matrix's `t-`
// namespace would be skipped cell by cell by the resume guard — every trial
// "complete" from a DIFFERENT task — while driver.sh printed MATRIX COMPLETE.
func TestDogfoodDriverRefusesDefaultPrefixWithBrief(t *testing.T) {
	argv, code, out := runStubbedDriver(t, append([]string{
		"DOGFOOD_BRIEF=Build something.",
	}, oneCellEnv...))
	if code == 0 {
		t.Fatalf("driver.sh accepted a brief under the default `t-` prefix\n%s", out)
	}
	if len(argv) != 0 {
		t.Fatalf("driver.sh refused but still launched a trial: %v", argv)
	}
	if !strings.Contains(out, "DOGFOOD_TRIAL_PREFIX") {
		t.Fatalf("the refusal does not say how to fix it:\n%s", out)
	}
}

// A multi-line DOGFOOD_BRIEF is refused before anything runs — the same rule
// runner.py enforces, applied one layer up so the matrix does not launch a
// cell just to have it die.
func TestDogfoodDriverRefusesMultilineBrief(t *testing.T) {
	argv, code, out := runStubbedDriver(t, append([]string{
		"DOGFOOD_BRIEF=one\ntwo",
		"DOGFOOD_TRIAL_PREFIX=ta",
	}, oneCellEnv...))
	if code == 0 {
		t.Fatalf("driver.sh accepted a multi-line brief\n%s", out)
	}
	if len(argv) != 0 {
		t.Fatalf("driver.sh refused but still launched a trial: %v", argv)
	}
}

// ── the brief and its assertion must not drift apart ─────────────────────────

// The brief names the hooks; the assertion looks for them. They are two files
// and nothing else ties them together, so a reworded brief that leaves the
// assertion behind would produce a matrix in which every cell fails for a
// reason nobody typed. This pins the RELATIONSHIP, not either side.
func TestCelsiusBriefAndAssertionAgree(t *testing.T) {
	briefRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "celsius.brief.txt"))
	if err != nil {
		t.Fatal(err)
	}
	brief := strings.TrimSpace(string(briefRaw))
	if brief == "" || strings.Contains(brief, "\n") {
		t.Fatalf("the brief must be exactly one non-empty line, got %q", brief)
	}
	assertRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "celsius.assert.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	assertion := string(assertRaw)

	// Each hook the brief promises must be the hook the assertion looks for.
	for _, hook := range []string{`data-testid="celsius"`, `data-testid="fahrenheit"`, "Convert"} {
		if !strings.Contains(brief, hook) {
			t.Fatalf("the brief no longer names %q — reword the assertion with it", hook)
		}
	}
	for _, hook := range []string{`[data-testid="celsius"]`, `[data-testid="fahrenheit"]`, "'convert'"} {
		if !strings.Contains(assertion, hook) {
			t.Fatalf("the assertion no longer looks for %q — the brief still asks for it", hook)
		}
	}
	// 100 C is 212 F. If either constant moves, the other has to.
	if !strings.Contains(assertion, `INPUT_C = '100'`) || !strings.Contains(assertion, `EXPECT_F = '212'`) {
		t.Fatal("the assertion's 100 -> 212 constants moved; celsius.md records the control results for those values")
	}
}
