package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ci.yml decides, inside the `dogfood-browser` job, whether a pull request
// needs the browser suite at all, and it cancels a pull request's superseded
// CI run. The decision itself is table-tested in
// dogfood_browser_relevance_test.go; THIS file guards the YAML around it,
// because every way of breaking that half fails in a quiet direction:
//
//   - split the classifier back into its own job, and `dogfood-browser`
//     waits for a SECOND runner in series on the run's slowest path again;
//   - drop the step's `continue-on-error`, or spell a step condition as
//     `run == 'true'`, and a classifier that crashed or published nothing
//     now skips the suite (fail CLOSED) with the required context green;
//   - drop `fetch-depth: 2` or `--no-renames`, and the diff is empty or blind
//     to a moved harness file;
//   - widen the concurrency group to every event, and GitHub's one-pending-
//     run-per-group rule cancels queued push-to-main runs.
//
// # What it does NOT claim
//
// It is a static read of YAML. It cannot run the job, so it says nothing
// about what GitHub does with the expressions — only that they are the
// expressions the comment in ci.yml describes.

const (
	dgWorkflow = "ci.yml"
	dgJob      = "dogfood-browser"
	dgScript   = "./scripts/dogfood/browser-suite-relevant.sh"
	dgFailOpen = "steps.decide.outcome != 'success' || steps.decide.outputs.run != 'false'"
	dgGroup    = "ci-${{ github.event_name == 'pull_request' && github.ref || github.run_id }}"
	dgCancel   = "${{ github.event_name == 'pull_request' }}"
)

// The steps after `decide` that do the suite's work. Each must carry exactly
// dgFailOpen; the suite step is also the positive control that the guard is
// looking at the job it was written for.
var dgGatedRuns = []string{"./scripts/dogfood/ci-browser.sh", "./scripts/dogfood/ci-browser-tests.sh"}

const dgGatedUses = "./.github/actions/setup-go"

type dgStep struct {
	ID              string         `yaml:"id"`
	Name            string         `yaml:"name"`
	Uses            string         `yaml:"uses"`
	Run             string         `yaml:"run"`
	If              string         `yaml:"if"`
	ContinueOnError yaml.Node      `yaml:"continue-on-error"`
	With            map[string]any `yaml:"with"`
}

type dgJobT struct {
	Needs           yaml.Node `yaml:"needs"`
	If              yaml.Node `yaml:"if"`
	ContinueOnError yaml.Node `yaml:"continue-on-error"`
	Steps           []dgStep  `yaml:"steps"`
}

type dgWorkflowT struct {
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress string `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]dgJobT `yaml:"jobs"`
}

// dogfoodGateProblems returns every way the workflow in raw breaks the
// contract above. It is a pure function of the bytes so the broken variants
// below can be fed to it without touching the real file.
func dogfoodGateProblems(raw []byte) []string {
	var wf dgWorkflowT
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return []string{fmt.Sprintf("cannot parse the workflow: %v", err)}
	}
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	// ── concurrency ──
	if wf.Concurrency.Group != dgGroup {
		add("concurrency.group is %q, want exactly %q: only pull requests may share a group — a push to main sharing one is cancelled while pending", wf.Concurrency.Group, dgGroup)
	}
	if wf.Concurrency.CancelInProgress != dgCancel {
		add("concurrency.cancel-in-progress is %q, want exactly %q", wf.Concurrency.CancelInProgress, dgCancel)
	}

	// ── no other job runs the classifier ──
	for name, j := range wf.Jobs {
		if name == dgJob {
			continue
		}
		for _, st := range j.Steps {
			if strings.Contains(st.Run, dgScript) {
				add("job `%s` runs %s: the classifier must be a step of `%s`, not a job it waits for — a separate job is a second runner wait in series on the slowest path", name, dgScript, dgJob)
			}
		}
	}

	job, ok := wf.Jobs[dgJob]
	if !ok {
		add("there is no `%s` job — it is a required status context", dgJob)
		return out
	}
	if !job.Needs.IsZero() {
		add("`%s` has `needs:` — it must not wait for another job before it can be queued", dgJob)
	}
	if !job.If.IsZero() {
		add("`%s` has a job-level `if:` — the skip decision belongs to its `decide` step", dgJob)
	}
	if !job.ContinueOnError.IsZero() {
		add("`%s` sets job-level `continue-on-error` — a failing suite must fail the required context", dgJob)
	}

	decide := -1
	deep := false
	for i, st := range job.Steps {
		if strings.HasPrefix(st.Uses, "actions/checkout@") && decide < 0 {
			if d, ok := st.With["fetch-depth"].(int); ok && (d == 0 || d >= 2) {
				deep = true
			}
		}
		if st.ID == "decide" {
			decide = i
		}
	}
	if decide < 0 {
		add("`%s` has no step with `id: decide`", dgJob)
		return out
	}
	d := job.Steps[decide]
	if !strings.Contains(d.Run, dgScript) {
		add("the `decide` step does not run %s", dgScript)
	}
	if !strings.Contains(d.Run, "--no-renames") {
		add("the `decide` step's diff lacks `--no-renames` — a file moved out of scripts/dogfood would be classified under its new path only")
	}
	if d.ContinueOnError.Value != "true" {
		add("the `decide` step is not `continue-on-error: true` — a crashing classifier would fail the job instead of failing OPEN to running the suite")
	}
	if !deep {
		add("no checkout before `decide` has `fetch-depth: 2` (or 0) — HEAD^1 is absent, the diff fails, and every PR runs the suite")
	}

	sawSuite := false
	for i, st := range job.Steps[decide+1:] {
		gated := st.Uses == dgGatedUses
		for _, r := range dgGatedRuns {
			if strings.TrimSpace(st.Run) == r {
				gated = true
				sawSuite = sawSuite || r == dgGatedRuns[len(dgGatedRuns)-1]
			}
		}
		label := st.Name
		if label == "" {
			label = st.Uses
		}
		if gated && st.If != dgFailOpen {
			add("step %d (%s) of `%s` has\n  if: %s\nwant exactly\n  if: %s\n(anything else can skip the suite when the classifier failed)", decide+2+i, label, dgJob, st.If, dgFailOpen)
		}
		if !gated && st.If != "" {
			add("step %d (%s) of `%s` has an unexpected `if: %s`", decide+2+i, label, dgJob, st.If)
		}
	}
	if !sawSuite {
		add("positive control: no step after `decide` runs %s — this guard is not looking at the job it was written for", dgGatedRuns[len(dgGatedRuns)-1])
	}
	return out
}

func readDGWorkflow(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(".github", "workflows", dgWorkflow))
	if err != nil {
		t.Fatalf("read %s: %v", dgWorkflow, err)
	}
	return raw
}

func TestDogfoodGateHoldsInCI(t *testing.T) {
	for _, p := range dogfoodGateProblems(readDGWorkflow(t)) {
		t.Error(p)
	}
}

// Each variant breaks the real file in ONE way and must be reported with ITS
// message — a variant rejected for some other reason proves nothing about the
// check it names.
func TestDogfoodGateRejectsBrokenVariants(t *testing.T) {
	real := string(readDGWorkflow(t))
	jobHead := "  dogfood-browser:\n    runs-on: ubuntu-latest\n"
	suiteIf := "        if: " + dgFailOpen + "\n        env:\n          CIVITAI_ASSERT_WAIT_LOG"
	cases := []struct {
		name, old, new, want string
	}{
		{"needs another job", jobHead, jobHead + "    needs: [build-test]\n", "has `needs:`"},
		{"job-level if", jobHead, jobHead + "    if: github.event_name == 'push'\n", "job-level `if:`"},
		{"decide fails closed", "        id: decide\n        continue-on-error: true\n", "        id: decide\n", "not `continue-on-error: true`"},
		{"suite gated fail-closed", suiteIf, "        if: steps.decide.outputs.run == 'true'\n        env:\n          CIVITAI_ASSERT_WAIT_LOG", "want exactly\n  if: " + dgFailOpen},
		{"shallow checkout", "      - uses: actions/checkout@v4\n        with:\n          fetch-depth: 2\n\n      - name: classify the change", "      - uses: actions/checkout@v4\n\n      - name: classify the change", "`fetch-depth: 2`"},
		{"rename detection", "--name-only --no-renames HEAD^1", "--name-only HEAD^1", "`--no-renames`"},
		{"cancel on push too", "  cancel-in-progress: " + dgCancel + "\n", "  cancel-in-progress: true\n", "concurrency.cancel-in-progress"},
		{"group shared by push", "  group: " + dgGroup + "\n", "  group: ci-${{ github.ref }}\n", "concurrency.group"},
		{"classifier split into its own job", "  lint:\n", "  dogfood-browser-changes:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + dgScript + " x y 0\n\n  lint:\n", "the classifier must be a step"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if strings.Count(real, c.old) != 1 {
				t.Fatalf("variant anchor occurs %d times in %s, want exactly 1 — the variant would not be the break it names", strings.Count(real, c.old), dgWorkflow)
			}
			got := dogfoodGateProblems([]byte(strings.Replace(real, c.old, c.new, 1)))
			for _, p := range got {
				if strings.Contains(p, c.want) {
					return
				}
			}
			t.Errorf("variant %q not reported with %q; got %q", c.name, c.want, got)
		})
	}
}
