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

// ── the brief's NAME ─────────────────────────────────────────────────────────

// The committed prose of a brief. Read rather than pinned: the point of
// `--brief-name` is that the name and the text agree, so the test has to use
// whatever text a run today would actually send.
func dogfoodBriefText(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", name+".brief.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// 🔴 THE TRANSCRIPT RECORDS WHICH BRIEF, NOT ONLY ITS PROSE. The render oracle
// has to map a graded cell back to `briefs/<name>.assert.mjs`, and from prose
// alone the only route is an exact match against `briefs/*.brief.txt` — which
// stops resolving every already-run trial the moment a brief file is reworded,
// and cannot resolve an ad-hoc brief at all. The field is emitted
// UNCONDITIONALLY (empty string included) on the same contract `brief` has, so
// "this run named no brief" is a positive assertion rather than an absence
// indistinguishable from an older runner's transcript.
func TestDogfoodRunnerRecordsTheBriefName(t *testing.T) {
	py := dogfoodPython(t)
	genpost := dogfoodBriefText(t, "genpost")
	for _, tc := range []struct {
		name  string
		brief string
		extra []string
		want  string
	}{
		{"named", genpost, []string{"--brief-name", "genpost"}, "genpost"},
		{"an ad-hoc brief names nothing", `Build a thing with data-testid="x".`, nil, ""},
		{"a setup trial names nothing", "", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := []string{
				filepath.Join(dogfoodDir, "testdata", "fake_trial.py"),
				filepath.Join(dogfoodDir, "runner.py"),
				filepath.Join(dir, "runs"), filepath.Join(dir, "capture.json"),
			}
			if tc.brief != "" {
				args = append(args, tc.brief)
			}
			if len(tc.extra) > 0 {
				args = append(args, "--")
				args = append(args, tc.extra...)
			}
			cmd := exec.Command(py, args...)
			// Same reason as TestDogfoodRunnerDeliversAndRecordsTheBrief: a
			// __pycache__ hit would run bytecode predating the edit under test.
			cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fake trial failed: %v\n%s", err, out)
			}
			tr, err := os.ReadFile(filepath.Join(dir, "runs", "faketrial", "transcript.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			var saw bool
			for _, line := range strings.Split(strings.TrimSpace(string(tr)), "\n") {
				var r struct {
					Kind      string  `json:"kind"`
					BriefName *string `json:"brief_name"`
				}
				if err := json.Unmarshal([]byte(line), &r); err != nil {
					t.Fatalf("transcript line is not JSON: %q", line)
				}
				if r.Kind != "start" {
					continue
				}
				saw = true
				if r.BriefName == nil {
					t.Fatal(`the "start" record carries no "brief_name" field — a grader can only ` +
						`resolve this cell's assertion by matching prose, which a reworded brief breaks`)
				}
				if *r.BriefName != tc.want {
					t.Fatalf("start.brief_name = %q, want %q", *r.BriefName, tc.want)
				}
			}
			if !saw {
				t.Fatalf("no start record in:\n%s", tr)
			}
		})
	}
}

// 🔴 AND A MISLABELLED ONE IS REFUSED BEFORE A CONTAINER OR AN API CALL EXISTS.
// A `--brief-name` is recorded as a FACT and then believed by every later
// grade, so the cheapest moment to catch a wrong one is before the matrix runs
// and the most expensive is after it has been paid for. Both directions: a name
// with no assertion behind it, and a name whose committed text is not the text
// being sent.
func TestDogfoodRunnerRefusesAMislabelledBriefName(t *testing.T) {
	genpost := dogfoodBriefText(t, "genpost")
	for _, tc := range []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "a name with no assertion behind it",
			args:    []string{"--brief", genpost, "--brief-name", "no-such-brief"},
			wantMsg: "has no assertion",
		},
		{
			name:    "a name whose committed text is not the text being sent",
			args:    []string{"--brief", genpost, "--brief-name", "celsius"},
			wantMsg: "disagrees with --brief",
		},
		{
			// The empty-brief version of the same mistake: naming a brief while
			// sending a setup trial's task.
			name:    "a name with no brief text at all",
			args:    []string{"--brief-name", "genpost"},
			wantMsg: "disagrees with --brief",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := printTask(t, tc.args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2 (argparse usage error)\noutput: %q", code, out)
			}
			// printTask captures stdout only; argparse writes to stderr, so
			// re-run for the message rather than asserting on an empty string.
			py := dogfoodPython(t)
			full, _ := exec.Command(py, append([]string{
				filepath.Join(dogfoodDir, "runner.py"), "--print-task"}, tc.args...)...).CombinedOutput()
			if !strings.Contains(string(full), tc.wantMsg) {
				t.Fatalf("the refusal does not say %q:\n%s", tc.wantMsg, full)
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
	// The committed brief TEXTS come along, because driver.sh resolves a brief's
	// NAME by matching its text against them. Without these the name-derivation
	// branch would be unreachable here and its test would pass vacuously.
	briefs, gerr := filepath.Glob(filepath.Join(dogfoodDir, "briefs", "*.brief.txt"))
	if gerr != nil {
		t.Fatal(gerr)
	}
	if len(briefs) < 2 {
		t.Fatalf("found %d brief text(s) — the stubbed driver would exercise no name derivation", len(briefs))
	}
	if err := os.MkdirAll(filepath.Join(dir, "briefs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, b := range briefs {
		raw, err := os.ReadFile(b)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "briefs", filepath.Base(b)), raw, 0o644); err != nil {
			t.Fatal(err)
		}
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

// 🔴 THE NAME REACHES THE RUNNER BY BOTH ROUTES, AND THEY CANNOT DISAGREE. An
// operator names the brief (`DOGFOOD_BRIEF_NAME`) or pastes its text
// (`DOGFOOD_BRIEF`); either way the runner is handed both, so a graded cell can
// be resolved to `briefs/<name>.assert.mjs` without matching prose. The third
// row is the one that keeps this honest: an ad-hoc brief matching no committed
// text must still RUN, and must record no name rather than a guessed one.
//
// ⚠ That third row is an INVARIANT guard, not regression coverage: it passes on
// the pre-change driver too, which had no derivation to get wrong. The first two
// rows are the regression ones.
func TestDogfoodDriverPassesTheBriefName(t *testing.T) {
	genpost := dogfoodBriefText(t, "genpost")
	for _, tc := range []struct {
		name      string
		env       []string
		wantBrief string
		wantName  string // "" => --brief-name must NOT appear
	}{
		{
			name:      "named: the text is read from the brief file",
			env:       []string{"DOGFOOD_BRIEF_NAME=genpost"},
			wantBrief: genpost,
			wantName:  "genpost",
		},
		{
			name:      "pasted: the name is derived from the text",
			env:       []string{"DOGFOOD_BRIEF=" + genpost},
			wantBrief: genpost,
			wantName:  "genpost",
		},
		{
			name:      "an ad-hoc brief still runs, and names nothing",
			env:       []string{`DOGFOOD_BRIEF=Build a thing nobody committed a brief for.`},
			wantBrief: `Build a thing nobody committed a brief for.`,
			wantName:  "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argv, code, out := runStubbedDriver(t, append(append([]string{},
				append(tc.env, "DOGFOOD_TRIAL_PREFIX=ta")...), oneCellEnv...))
			if code != 0 {
				t.Fatalf("driver.sh exited %d, want 0\n%s", code, out)
			}
			valueOf := func(flag string) (string, bool) {
				for i, a := range argv {
					if a == flag {
						if i+1 >= len(argv) {
							t.Fatalf("%s has no value: %v", flag, argv)
						}
						return argv[i+1], true
					}
				}
				return "", false
			}
			got, ok := valueOf("--brief")
			if !ok || got != tc.wantBrief {
				t.Fatalf("--brief = %q (present=%v), want %q: %v", got, ok, tc.wantBrief, argv)
			}
			gotName, hasName := valueOf("--brief-name")
			if tc.wantName == "" {
				if hasName {
					t.Fatalf("an ad-hoc brief was labelled --brief-name %q — a guessed name is "+
						"worse than none, because every later grade believes it", gotName)
				}
				return
			}
			if !hasName || gotName != tc.wantName {
				t.Fatalf("--brief-name = %q (present=%v), want %q: %v", gotName, hasName, tc.wantName, argv)
			}
		})
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

// The same relationship for `genpost`. Its hooks are the two test ids, the two
// button labels and the two status words — and the STATUS WORDS are the half
// that is easy to reword in prose and forget in code, because the brief spells
// them in English ("holding exactly ready") while the assertion holds them as
// string constants.
func TestGenpostBriefAndAssertionAgree(t *testing.T) {
	briefRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "genpost.brief.txt"))
	if err != nil {
		t.Fatal(err)
	}
	brief := strings.TrimSpace(string(briefRaw))
	if brief == "" || strings.Contains(brief, "\n") {
		t.Fatalf("the brief must be exactly one non-empty line, got %q", brief)
	}
	assertRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "genpost.assert.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	assertion := string(assertRaw)

	for _, hook := range []string{
		`data-testid="prompt"`, `data-testid="status"`, "Generate", "Post", "ready", "generating",
	} {
		if !strings.Contains(brief, hook) {
			t.Fatalf("the brief no longer names %q — reword the assertion with it", hook)
		}
	}
	for _, hook := range []string{
		`[data-testid="prompt"]`, `[data-testid="status"]`,
		"'generate'", "'post'", "STATUS_IDLE = 'ready'", "STATUS_BUSY = 'generating'",
	} {
		if !strings.Contains(assertion, hook) {
			t.Fatalf("the assertion no longer looks for %q — the brief still asks for it", hook)
		}
	}
	// 🔴 THE POST GATE IS THE LOAD-BEARING STEP, AND IT IS THE ONE A
	// "simplification" would drop. The page-money scaffold already ships a
	// prompt field and a Generate button wired to a real submitWorkflow, so an
	// assertion keyed on generation alone is satisfied by an untouched scaffold
	// and measures nothing. Both halves must survive: the control must EXIST and
	// it must be DISABLED at rest.
	if !strings.Contains(brief, "disabled") {
		t.Fatal("the brief no longer asks for the Post control to start disabled — without that " +
			"clause the brief is satisfiable by a generate-only app, which the page-money " +
			"scaffold already is (see briefs/genpost.md)")
	}
	if !strings.Contains(assertion, "postDisabled") {
		t.Fatal("the assertion no longer checks whether the Post control is disabled — see the " +
			"neg-postenabled control in briefs/genpost.md")
	}
}

// 🔴 `ship` IS `genpost` PLUS A SUBMIT INSTRUCTION, AND THAT RELATIONSHIP IS
// WHAT THIS PINS — not two independent word lists.
//
// The render half of a ship cell is graded by the GENPOST assertion (ship.assert.mjs
// delegates to it, deliberately: one predicate, one place). That delegation is
// only correct while the ship brief asks for the same renderable behaviour, so
// the guard is the whole normalised string — `ship` must literally BEGIN with
// `genpost`'s text — rather than a set of keywords that a reworded brief could
// still spell while asking for something different. A cosmetic reword of
// `genpost` fails this test; that is the price of a machine-readable claim, and
// the remedy is to move both files together.
//
// 🔴 AND THE SUBMIT CLAUSE IS THE HALF THE WHOLE BRIEF EXISTS FOR. Four
// credentialed trials ran with `--max-submissions 1` and all four submitted
// nothing, because the genpost brief never asks. A cap that ALLOWS a submit does
// not produce one. If the trailing clause is ever dropped, `ship` becomes
// `genpost` under a second name and every ship cell grades `SHIP=no` for a reason
// nobody typed.
func TestShipBriefAndAssertionAgree(t *testing.T) {
	briefRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "ship.brief.txt"))
	if err != nil {
		t.Fatal(err)
	}
	brief := strings.TrimSpace(string(briefRaw))
	if brief == "" || strings.Contains(brief, "\n") {
		t.Fatalf("the brief must be exactly one non-empty line, got %q", brief)
	}
	genpost := dogfoodBriefText(t, "genpost")

	// (a) The render half, pinned as the WHOLE string rather than as keywords.
	if !strings.HasPrefix(brief, genpost) {
		t.Fatalf("the ship brief no longer begins with the genpost brief verbatim.\n"+
			" ship: %q\ngenpost: %q\n\nship.assert.mjs delegates to genpost.assert.mjs, so the two briefs' "+
			"renderable half must be the same text. Move both files together.", brief, genpost)
	}
	// (b) The submit half exists and is not empty whitespace.
	tail := strings.TrimSpace(strings.TrimPrefix(brief, genpost))
	if tail == "" {
		t.Fatal("the ship brief is byte-identical to the genpost brief — it asks for no submit, " +
			"which is the exact defect it exists to fix: four credentialed trials with a submission " +
			"PERMITTED submitted nothing because no brief asked")
	}
	if !strings.Contains(strings.ToLower(tail), "submit") {
		t.Fatalf("the ship brief's trailing clause does not ask for a submit: %q", tail)
	}
	// 🔴 (c) AND IT MUST NOT NAME A CLI FLAG. The brief is appended RAW, with no
	// framing of the harness's own, so the only Civitai knowledge a trial gets is
	// the hosted URL and what the operator typed. Naming `--yes` (which a
	// headless submit genuinely needs — `confirmSubmit` refuses a non-TTY without
	// it) would hand the trial a fact it is supposed to discover, and would make
	// a green cell a statement about the brief rather than about the onboarding.
	if strings.Contains(brief, "--") {
		t.Fatalf("the brief names a command-line flag: %q. The brief is what the OPERATOR typed; "+
			"a flag in it is Civitai knowledge injected by the rig", brief)
	}

	// (d) The assertion is the genpost one, by delegation, and the constants it
	// grades against still live in the file that owns them.
	assertRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "ship.assert.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	assertion := string(assertRaw)
	if !strings.Contains(assertion, "genpost.assert.mjs") {
		t.Fatal("ship.assert.mjs no longer delegates to genpost.assert.mjs. If it has been forked, " +
			"the same predicate is now open-coded at two sites and briefs/genpost.md's scaffold and " +
			"generate-only controls are no longer controls for THIS assertion — give it its own " +
			"constants here and its own controls in ship.md, or restore the delegation")
	}
	// The delegate's constants, checked THROUGH the delegation: this is the
	// relationship the ship brief actually depends on, and asserting it here is
	// what stops a genpost refactor silently emptying the ship cell's render half.
	delegateRaw, err := os.ReadFile(filepath.Join(dogfoodDir, "briefs", "genpost.assert.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	delegate := string(delegateRaw)
	for _, hook := range []string{
		`[data-testid="prompt"]`, `[data-testid="status"]`,
		"'generate'", "'post'", "STATUS_IDLE = 'ready'", "STATUS_BUSY = 'generating'",
		"postDisabled",
	} {
		if !strings.Contains(delegate, hook) {
			t.Fatalf("genpost.assert.mjs no longer looks for %q, and ship.assert.mjs delegates to it — "+
				"the ship brief still asks for it", hook)
		}
	}

	// (e) The SHIP half has a verdict path at all. The assertion above grades the
	// DOM and CANNOT see a submission (the oracle's host stub rejects every
	// request and `token.raw` is empty), so a ship brief whose only grader is
	// browser-shaped measures exactly the genpost brief under a new name.
	if _, err := os.Stat(filepath.Join(dogfoodDir, "ship.verdict.sh")); err != nil {
		t.Fatalf("no %s/ship.verdict.sh: %v — the ship brief's submit half would be ungraded, and "+
			"an ungraded half is indistinguishable from a passing one", dogfoodDir, err)
	}
}

// 🔴 THE LEDGER. The tests above pin one pair each; nothing pins that a
// FOURTH brief gets a guard at all. A brief with no drift guard is the failure
// this whole pair exists to prevent, arriving by addition instead of by edit:
// every cell of its matrix would fail for a reason nobody typed, and no test
// would be red. Adding briefs/<name>.brief.txt therefore means adding
// <name>.assert.mjs, <name>.md, and a Test<Name>BriefAndAssertionAgree here.
func TestEveryBriefHasASiblingGuard(t *testing.T) {
	briefs, err := filepath.Glob(filepath.Join(dogfoodDir, "briefs", "*.brief.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// POSITIVE CONTROL: a glob that matched nothing would make every assertion
	// below vacuous, and "no briefs found" is exactly what a moved directory
	// looks like.
	if len(briefs) < 2 {
		t.Fatalf("found %d brief(s) under %s/briefs — this ledger cannot have checked anything",
			len(briefs), dogfoodDir)
	}
	self, err := os.ReadFile("dogfood_brief_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range briefs {
		name := strings.TrimSuffix(filepath.Base(b), ".brief.txt")
		for _, sibling := range []string{name + ".assert.mjs", name + ".md"} {
			if _, err := os.Stat(filepath.Join(dogfoodDir, "briefs", sibling)); err != nil {
				t.Errorf("brief %q has no %s: %v", name, sibling, err)
			}
		}
		want := "func Test" + strings.ToUpper(name[:1]) + name[1:] + "BriefAndAssertionAgree("
		if !strings.Contains(string(self), want) {
			t.Errorf("brief %q has no drift guard — add `%s…}` to this file. Without one, a "+
				"reworded brief that leaves its assertion behind produces a matrix in which "+
				"every cell fails for a reason nobody typed.", name, want)
		}
	}
}
