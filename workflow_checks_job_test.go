package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ci.yml runs six small gates as steps of ONE job, `checks`, instead of six
// jobs that each waited for their own GitHub-hosted runner (see the comment
// above `checks:` in ci.yml). This file guards that layout, because each way of
// undoing it fails quietly:
//
//   - split a gate back out into its own job, and every run pays another
//     runner wait — and the required context that job would need does not
//     exist in branch protection, so its red blocks nothing;
//   - drop a gate's steps while folding, and `checks` stays green having
//     never run it — the LEDGER below fails when the set of gates shrinks or
//     grows, so adding one is a deliberate edit here too;
//   - put `continue-on-error` on a step, or an `if:` on the job, and `checks`
//     can go green with a gate red or never run.
//
// # What it does NOT claim
//
// It is a static read of YAML. It cannot see branch protection: whether
// `checks` is actually REQUIRED is an API fact (AGENTS.md item 11). The
// pins-vs-published step's own contract is in workflow_pins_gate_test.go.

const ckJob = "checks"

// The ledger: exactly these former jobs are steps of `checks`, in this order,
// each step named `<former job> · <step>`.
var ckGates = []string{
	"pins-vs-published",
	"ready-ack-runtime",
	"template-page-vite",
	"template-page-elements",
	"template-page-money",
	"scaffold-currency",
}

const ckSep = " · "

type ckStep struct {
	Name            string    `yaml:"name"`
	Uses            string    `yaml:"uses"`
	Run             string    `yaml:"run"`
	ContinueOnError yaml.Node `yaml:"continue-on-error"`
}

type ckJobT struct {
	Needs           yaml.Node `yaml:"needs"`
	If              yaml.Node `yaml:"if"`
	ContinueOnError yaml.Node `yaml:"continue-on-error"`
	Steps           []ckStep  `yaml:"steps"`
}

// checksJobProblems returns every way the workflow in raw breaks the layout
// above. A pure function of the bytes, so broken variants can be fed to it.
func checksJobProblems(raw []byte) []string {
	var wf struct {
		Jobs map[string]ckJobT `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		return []string{fmt.Sprintf("cannot parse the workflow: %v", err)}
	}
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	for _, g := range ckGates {
		if _, ok := wf.Jobs[g]; ok {
			add("`%s` is its own job again — it belongs in `%s` as `%s%s<step>` steps: a separate job waits for its own runner, and its context is not one branch protection requires", g, ckJob, g, ckSep)
		}
	}

	job, ok := wf.Jobs[ckJob]
	if !ok {
		add("there is no `%s` job — the six small gates and the required context live there", ckJob)
		return out
	}
	if !job.Needs.IsZero() {
		add("`%s` has `needs:` — it would queue only after another job finished", ckJob)
	}
	if !job.If.IsZero() {
		add("`%s` has a job-level `if:` — a skipped required job is reported as satisfied", ckJob)
	}
	if !job.ContinueOnError.IsZero() {
		add("`%s` sets job-level `continue-on-error` — a red gate would leave the context green", ckJob)
	}

	var order []string
	seen := map[string]bool{}
	for i, st := range job.Steps {
		if !st.ContinueOnError.IsZero() {
			add("step %d (%s) of `%s` sets `continue-on-error` — one gate's red would no longer fail the job", i+1, st.Name, ckJob)
		}
		if st.Name == "" {
			continue // the shared checkout / Go / node setup
		}
		g, _, found := strings.Cut(st.Name, ckSep)
		if !found {
			add("step %d of `%s` is named %q, without the `<former job>%s` prefix — a red run must say which gate failed", i+1, ckJob, st.Name, ckSep)
			continue
		}
		if !seen[g] {
			seen[g] = true
			order = append(order, g)
		}
	}
	if got, want := strings.Join(order, ","), strings.Join(ckGates, ","); got != want {
		add("the gates run by `%s` are [%s], want exactly [%s] in that order — a gate missing here is a check that silently stopped running; a new one must be added to this ledger on purpose", ckJob, got, want)
	}
	return out
}

func readCKWorkflow(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	return raw
}

func TestChecksJobHoldsTheSmallGates(t *testing.T) {
	for _, p := range checksJobProblems(readCKWorkflow(t)) {
		t.Error(p)
	}
}

// One variant per check, each required to be reported with ITS message.
func TestChecksJobGuardRejectsBrokenVariants(t *testing.T) {
	real := string(readCKWorkflow(t))
	if p := checksJobProblems([]byte(real)); len(p) != 0 {
		t.Fatalf("control: the unmodified workflow must be clean first, got %q", p)
	}
	jobHead := "  checks:\n    runs-on: ubuntu-latest\n"
	cases := []struct{ name, old, new, want string }{
		{"a gate split back out", "  lint:\n", "  scaffold-currency:\n    runs-on: ubuntu-latest\n    steps:\n      - run: true\n\n  lint:\n", "`scaffold-currency` is its own job again"},
		{"checks job removed", jobHead, "  checks-renamed:\n    runs-on: ubuntu-latest\n", "there is no `checks` job"},
		{"needs another job", jobHead, jobHead + "    needs: [build-test]\n", "has `needs:`"},
		{"job-level if", jobHead, jobHead + "    if: github.event_name == 'push'\n", "job-level `if:`"},
		{"job-level continue-on-error", jobHead, jobHead + "    continue-on-error: true\n", "job-level `continue-on-error`"},
		{"step continue-on-error", "      - name: template-page-money · build\n", "      - name: template-page-money · build\n        continue-on-error: true\n", "sets `continue-on-error`"},
		{"unprefixed step name", "      - name: template-page-vite · build\n", "      - name: build\n", "without the `<former job> · ` prefix"},
		{"a gate dropped", "      - name: ready-ack-runtime · scaffolded ready-ack actually fires\n", "      - name: template-page-vite · scaffolded ready-ack actually fires\n", "want exactly [pins-vs-published,ready-ack-runtime"},
		{"an unledgered gate added", "      - name: template-page-money · validate\n", "      - name: template-page-sdk · validate\n", "want exactly [pins-vs-published"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if n := strings.Count(real, c.old); n != 1 {
				t.Fatalf("variant anchor occurs %d times, want exactly 1:\n%s", n, c.old)
			}
			got := checksJobProblems([]byte(strings.Replace(real, c.old, c.new, 1)))
			for _, p := range got {
				if strings.Contains(p, c.want) {
					return
				}
			}
			t.Errorf("variant %q not reported with %q; got %q", c.name, c.want, got)
		})
	}
}
