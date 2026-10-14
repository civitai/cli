package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The `pins-vs-published` job in ci.yml is a REQUIRED status check whose
// blocking behaviour was deliberately narrowed: `internal/scaffold/cmd/pins-gate`
// now decides, per run, whether a stale pin blocks (a pull request that touches
// no pin input is warned, everything else gates). The decision is table-tested
// beside the command. THIS file guards the other half — the YAML around it —
// because every way of breaking that half fails in the REASSURING direction:
//
//   - rename or remove the job, and the required context is never reported;
//   - add `continue-on-error` or an `if:`, and the context goes green without
//     the job being able to block anything, main included;
//   - drop `fetch-depth: 2`, and nothing looks wrong — pins-gate fails closed,
//     every PR gates again, and the change this guards is silently inert;
//   - lose `pipefail`, and `pins-gate | tee` reports tee's exit status.
//
// # What it does NOT claim
//
// It is a static read of YAML. It cannot observe branch protection (which
// contexts are actually required is an API fact — AGENTS.md item 11), and it
// cannot run the job. That a pull_request run really does downgrade is a claim
// only a live run can make.

const (
	pinsGateWorkflow = "ci.yml"
	pinsGateJob      = "pins-vs-published"
	pinsGateCmd      = "go run ./internal/scaffold/cmd/pins-gate"
)

type pgStep struct {
	Name            string            `yaml:"name"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	If              yaml.Node         `yaml:"if"`
	ContinueOnError yaml.Node         `yaml:"continue-on-error"`
	With            map[string]any    `yaml:"with"`
	Env             map[string]string `yaml:"env"`
}

type pgJob struct {
	Name            string    `yaml:"name"`
	If              yaml.Node `yaml:"if"`
	ContinueOnError yaml.Node `yaml:"continue-on-error"`
	Steps           []pgStep  `yaml:"steps"`
}

type pgWorkflow struct {
	On   map[string]yaml.Node `yaml:"on"`
	Jobs map[string]pgJob     `yaml:"jobs"`
}

var (
	pgEventFlagRe = regexp.MustCompile(`(?m)(^|\s)-event(\s|=)`)
	pgErrexitOff  = regexp.MustCompile(`(?m)^\s*set\s+(\+e\b|\+o\s+errexit|[+][a-z]*e[a-z]*\b)`)
	pgPipefailOn  = regexp.MustCompile(`(?m)^\s*set\s+-[a-z]*o\s+pipefail\b`)
)

// pinsGateJobProblems returns every way the workflow in raw fails to hold the
// `pins-vs-published` contract. It is a pure function of the bytes so the
// broken variants below can be fed to it without touching the real file.
func pinsGateJobProblems(raw []byte) []string {
	var wf pgWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return []string{fmt.Sprintf("cannot parse the workflow: %v", err)}
	}
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	// ── the triggers ──
	if wf.On == nil {
		return []string{"the workflow has no readable `on:` block, so nothing below can be trusted"}
	}
	push, ok := wf.On["push"]
	if !ok {
		add("`on:` has no `push` trigger — a red main is how stale pins stay loud, and without this trigger the job never runs on main at all")
	} else {
		var p struct {
			Branches []string `yaml:"branches"`
		}
		_ = push.Decode(&p)
		onMain := len(p.Branches) == 0 // no filter = every branch
		for _, b := range p.Branches {
			onMain = onMain || b == "main"
		}
		if !onMain {
			add("`on.push.branches` is %q, which does not include `main` — the job would no longer run on a push to main", p.Branches)
		}
	}
	if _, ok := wf.On["pull_request"]; !ok {
		add("`on:` has no `pull_request` trigger — the required context would never be reported on a pull request")
	}
	if _, ok := wf.On["workflow_dispatch"]; !ok {
		add("`on:` has no `workflow_dispatch` trigger — the manual recovery path the file's own header says not to remove")
	}

	// ── the job ──
	job, ok := wf.Jobs[pinsGateJob]
	if !ok {
		add("there is no job with the key `%s`. Branch protection requires that context BY NAME: renamed or removed, it is never reported and every pull request waits on a check that cannot arrive", pinsGateJob)
		return out
	}
	if job.Name != "" && job.Name != pinsGateJob {
		add("the `%s` job sets `name: %s`. The status context is the job's NAME when one is set, so this renames the required check as surely as changing the key", pinsGateJob, job.Name)
	}
	if !job.ContinueOnError.IsZero() {
		add("the `%s` job sets `continue-on-error` at JOB level. That makes the job unable to fail the run ANYWHERE — on main and on a pull request that changed a pin alike — while still reporting the required context as passed. The downgrade belongs inside pins-gate, which decides per run", pinsGateJob)
	}
	if !job.If.IsZero() {
		add("the `%s` job has a job-level `if:`. A skipped job reports its required context as satisfied, so a condition here is a way for the gate to pass without running", pinsGateJob)
	}

	// ── its steps ──
	var gate []pgStep
	checkoutDeep := false
	sawCheckout := false
	for i, st := range job.Steps {
		label := st.Name
		if label == "" {
			label = st.Uses
		}
		if !st.ContinueOnError.IsZero() {
			add("step %d (%s) of `%s` sets `continue-on-error` — a failing step in this job must fail the job", i+1, label, pinsGateJob)
		}
		if strings.HasPrefix(st.Uses, "actions/checkout@") {
			sawCheckout = true
			switch d := st.With["fetch-depth"].(type) {
			case int:
				checkoutDeep = d == 0 || d >= 2
			}
		}
		if strings.Contains(st.Run, pinsGateCmd) {
			gate = append(gate, st)
		}
	}
	if !sawCheckout || !checkoutDeep {
		add("the `%s` job's checkout does not set `fetch-depth: 2` (or 0). pins-gate reads the pull request's diff off the merge commit's two parents; at the default depth 1 they are not there, it cannot tell what the PR changed, and — failing closed — it gates every pull request. Nothing would look broken; the downgrade would just never happen", pinsGateJob)
	}
	if len(gate) != 1 {
		add("%d step(s) of `%s` run `%s`, want exactly 1 — with none the job reports the required context without checking a pin, and with several its verdict is whichever ran last", len(gate), pinsGateJob, pinsGateCmd)
		return out
	}
	st := gate[0]
	if !st.If.IsZero() {
		add("the pins-gate step has an `if:` — when it is false the step is skipped and the job is green without a verdict")
	}

	// The script, with comment lines removed: this file's comments quote the
	// very constructs being rejected.
	var code strings.Builder
	for _, line := range strings.Split(st.Run, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			code.WriteString(line + "\n")
		}
	}
	script := code.String()
	at := strings.Index(script, pinsGateCmd)
	before, invocation := script[:at], script[at:]
	if end := strings.Index(invocation, "\n\n"); end >= 0 {
		invocation = invocation[:end]
	}

	if n := len(pgEventFlagRe.FindAllString(script, -1)); n != 1 || !strings.Contains(invocation, `-event "$GITHUB_EVENT_NAME"`) {
		add("the pins-gate step must pass `-event \"$GITHUB_EVENT_NAME\"` exactly once (found %d `-event` flag(s)). The event name has to come from the RUNNER: a literal here lets this file choose which runs gate — `-event pull_request` on a push to main would downgrade main", n)
	}
	if !strings.Contains(invocation, `-pr-head "$PR_HEAD_SHA"`) || st.Env["PR_HEAD_SHA"] != "${{ github.event.pull_request.head.sha }}" {
		add("the pins-gate step must pass `-pr-head \"$PR_HEAD_SHA\"` with `PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}` (env has %q). pins-gate checks the checkout really is THAT pull request's merge commit before trusting its diff", st.Env["PR_HEAD_SHA"])
	}
	if strings.Contains(script, "-registry") {
		add("the pins-gate step passes `-registry`. That flag exists for tests; in CI it would let the workflow point the required check at any server")
	}
	if rest := strings.ReplaceAll(invocation, "2>&1", ""); strings.Contains(rest, "||") || strings.Contains(rest, "&") {
		add("the pins-gate invocation is followed by `||` or backgrounded — its exit code IS the gate and must reach the shell")
	}
	if !pgPipefailOn.MatchString(before) {
		add("the pins-gate step does not `set -o pipefail` before running it. The command is piped into `tee`; without pipefail the pipeline's status is tee's, and a blocking verdict exits 0")
	}
	if pgErrexitOff.MatchString(before) {
		add("the pins-gate step turns errexit off before running it. GitHub's `bash -e` is what makes a non-zero pins-gate kill the step at the pipe")
	}

	// ── the verdict-line control ──
	//
	// 🔴 This is the ONLY thing standing between a green job and a green over
	// ZERO pins. pins-gate refuses zero pins itself, but that refusal sits behind
	// `go:embed` — no Go test can hand it an empty template tree — and it survives
	// the mutation battery. So the guard is behavioural, not textual: the regex
	// the step greps with is run against the lines it must accept and reject, and
	// a rewording that keeps it equally strict passes.
	after := script[at:]
	m := pgVerdictCheckRe.FindStringSubmatchIndex(after)
	if m == nil {
		add("the pins-gate step no longer checks pins-gate's own verdict line (`if ! grep -Eq -- '<regex>' \"$RUNNER_TEMP/pins.log\"; then … exit 1`). Without it, an exit 0 over zero pins — a green that checked nothing — reaches the required context")
		return out
	}
	re, err := regexp.Compile(after[m[2]:m[3]])
	if err != nil {
		add("the verdict-line regex %q does not compile: %v", after[m[2]:m[3]], err)
		return out
	}
	if body := after[m[1]:]; !pgThenExit1Re.MatchString(body) {
		add("the verdict-line check does not `exit 1` before its `fi` — a missing verdict line would print and pass")
	}
	for _, ok := range []string{
		"pins-gate: mode=advise verdict=stale pins=6 blocking=0 exit=0",
		"pins-gate: mode=gate verdict=ok pins=6 blocking=0 exit=0",
		"pins-gate: mode=advise verdict=unreachable pins=12 blocking=0 exit=0",
	} {
		if !re.MatchString(ok) {
			add("the verdict-line regex rejects a passing verdict %q — every green run would go red", ok)
		}
	}
	for _, bad := range []struct{ line, why string }{
		{"pins-gate: mode=gate verdict=ok pins=0 blocking=0 exit=0", "a verdict over ZERO pins"},
		{"pins-gate: mode=gate verdict=stale pins=6 blocking=3 exit=1", "a blocking verdict"},
		{"pins-gate: mode=advise verdict=broken pins=6 blocking=0 exit=0", "a broken verdict"},
		{"echo pins-gate: mode=gate verdict=ok pins=6 blocking=0 exit=0", "a line that merely CONTAINS the verdict (the regex is not anchored)"},
	} {
		if re.MatchString(bad.line) {
			add("the verdict-line regex accepts %s: %q. It is the only control that rejects that green", bad.why, bad.line)
		}
	}
	return out
}

var (
	pgVerdictCheckRe = regexp.MustCompile(`(?m)^\s*if ! grep -Eq -- '([^']*)' "\$RUNNER_TEMP/pins\.log"; then\n`)
	pgThenExit1Re    = regexp.MustCompile(`\A(?:[^\n]*\n){0,3}?\s*exit 1\n\s*fi\n`)
)

func readPinsGateWorkflow(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workflowDir, pinsGateWorkflow))
	if err != nil {
		t.Fatalf("read %s: %v", pinsGateWorkflow, err)
	}
	return raw
}

// TestPinsVsPublishedJobStillGates is the guard on the real file.
func TestPinsVsPublishedJobStillGates(t *testing.T) {
	problems := pinsGateJobProblems(readPinsGateWorkflow(t))
	for _, p := range problems {
		t.Errorf("%s/%s: %s", workflowDir, pinsGateWorkflow, p)
	}
}

// TestPinsGateWorkflowGuardCanGoRed feeds the guard one broken variant of the
// REAL ci.yml per assertion and requires that assertion's own message. A guard
// that has only ever been run against a correct file has never been seen to
// work; and requiring the specific message (not merely "some problem") is what
// shows each assertion is reachable rather than shadowed by an earlier one.
func TestPinsGateWorkflowGuardCanGoRed(t *testing.T) {
	real := string(readPinsGateWorkflow(t))
	if p := pinsGateJobProblems([]byte(real)); len(p) != 0 {
		t.Fatalf("control: the unmodified workflow must be clean before its mutants mean anything, got %q", p)
	}

	const jobHead = "  pins-vs-published:\n    runs-on: ubuntu-latest\n"
	const invocation = "-event \"$GITHUB_EVENT_NAME\" -pr-head \"$PR_HEAD_SHA\" 2>&1 \\\n            | tee \"$RUNNER_TEMP/pins.log\"\n"
	const verdictRe = `'^pins-gate: mode=(gate|advise) verdict=(ok|stale|unreachable) pins=[1-9][0-9]* blocking=0 exit=0$'`
	const stepHead = "      - name: check scaffold pins vs npm latest\n"

	mutants := []struct {
		name     string
		old, new string
		want     string // a substring only THIS assertion's message carries
	}{
		{"job renamed", "  pins-vs-published:\n", "  pins-vs-npm:\n", "there is no job with the key `pins-vs-published`"},
		{"job removed (key commented out)", "  pins-vs-published:\n", "  # pins-vs-published:\n  some-other-job:\n", "there is no job with the key `pins-vs-published`"},
		{"job given a different display name", jobHead, jobHead + "    name: scaffold pins\n", "sets `name: scaffold pins`"},
		{"job-level continue-on-error: true", jobHead, jobHead + "    continue-on-error: true\n", "sets `continue-on-error` at JOB level"},
		{"job-level continue-on-error as an expression", jobHead, jobHead + "    continue-on-error: ${{ github.event_name == 'pull_request' }}\n", "sets `continue-on-error` at JOB level"},
		{"job-level continue-on-error: false is still a knob nobody should be holding", jobHead, jobHead + "    continue-on-error: false\n", "sets `continue-on-error` at JOB level"},
		{"job-level if", jobHead, jobHead + "    if: github.event_name == 'pull_request'\n", "has a job-level `if:`"},
		{"step-level continue-on-error on the gate", stepHead, stepHead + "        continue-on-error: true\n", "sets `continue-on-error` — a failing step in this job must fail the job"},
		{"step-level if on the gate", stepHead, stepHead + "        if: github.event_name != 'push'\n", "the pins-gate step has an `if:`"},
		{"fetch-depth dropped", "        with:\n          fetch-depth: 2\n", "", "does not set `fetch-depth: 2`"},
		{"fetch-depth: 1", "          fetch-depth: 2\n", "          fetch-depth: 1\n", "does not set `fetch-depth: 2`"},
		{"the command no longer runs", pinsGateCmd + " \\\n", "go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished \\\n", "0 step(s) of `pins-vs-published` run"},
		{"event hardcoded to pull_request", `-event "$GITHUB_EVENT_NAME"`, `-event pull_request`, "must pass `-event \"$GITHUB_EVENT_NAME\"` exactly once"},
		{"a second -event flag appended", invocation, strings.Replace(invocation, " 2>&1", " -event pull_request 2>&1", 1), "must pass `-event \"$GITHUB_EVENT_NAME\"` exactly once (found 2"},
		{"pr head no longer the event's", "PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}", "PR_HEAD_SHA: ${{ github.sha }}", "must pass `-pr-head \"$PR_HEAD_SHA\"` with"},
		{"registry overridden", `-pr-head "$PR_HEAD_SHA" 2>&1`, `-pr-head "$PR_HEAD_SHA" -registry "$NPM_MIRROR" 2>&1`, "passes `-registry`"},
		{"exit code swallowed", "| tee \"$RUNNER_TEMP/pins.log\"\n\n          # POSITIVE CONTROL — the remaining", "| tee \"$RUNNER_TEMP/pins.log\" || true\n\n          # POSITIVE CONTROL — the remaining", "is followed by `||`"},
		{"pipefail dropped", "          set -uo pipefail\n\n          # `go run`", "          set -u\n\n          # `go run`", "does not `set -o pipefail`"},
		{"errexit turned off", "          set -uo pipefail\n\n          # `go run`", "          set -uo pipefail\n          set +e\n\n          # `go run`", "turns errexit off"},
		// The verdict-line control: the coverage for the zero-pins refusal no Go
		// test can reach.
		{"verdict-line check deleted", "          if ! grep -Eq -- " + verdictRe + " \"$RUNNER_TEMP/pins.log\"; then\n            echo \"::error::pins-gate exited 0 without printing a passing verdict line over at least one pin. A green here would have been a claim about nothing.\"\n            exit 1\n          fi\n", "", "no longer checks pins-gate's own verdict line"},
		{"verdict-line check loosened to accept pins=0", "pins=[1-9][0-9]*", "pins=[0-9]+", "accepts a verdict over ZERO pins"},
		{"verdict-line check unanchored", verdictRe, strings.Replace(verdictRe, "'^", "'", 1), "is not anchored"},
		{"verdict-line check accepts blocking runs", "blocking=0 exit=0$'", "blocking=[0-9]+ exit=[0-9]$'", "accepts a blocking verdict"},
		{"verdict-line check no longer exits", "line over at least one pin. A green here would have been a claim about nothing.\"\n            exit 1\n", "line over at least one pin. A green here would have been a claim about nothing.\"\n", "does not `exit 1` before its `fi`"},
		{"push trigger removed", "  push:\n    branches: [main]\n  pull_request:\n", "  pull_request:\n", "`on:` has no `push` trigger"},
		{"push no longer covers main", "  push:\n    branches: [main]\n", "  push:\n    branches: [release]\n", "does not include `main`"},
		{"pull_request trigger removed", "    branches: [main]\n  pull_request:\n", "    branches: [main]\n", "`on:` has no `pull_request` trigger"},
	}

	for _, m := range mutants {
		t.Run(m.name, func(t *testing.T) {
			if n := strings.Count(real, m.old); n != 1 {
				t.Fatalf("this mutant's anchor occurs %d time(s) in %s, want exactly 1 — the mutation would not be the one this row describes. Anchor:\n%s", n, pinsGateWorkflow, m.old)
			}
			mutated := strings.Replace(real, m.old, m.new, 1)
			problems := pinsGateJobProblems([]byte(mutated))
			for _, p := range problems {
				if strings.Contains(p, m.want) {
					return
				}
			}
			t.Fatalf("the guard did not report this broken variant with its own message.\n  want a problem containing: %s\n  got: %q", m.want, problems)
		})
	}
}
