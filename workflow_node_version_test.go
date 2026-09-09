package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Two invariants about `.github/workflows/`, both written after issue #530, in
// which the repo froze itself for three days and nobody could land the fix.
//
// # What happened
//
// `ci.yml` moved its scaffold-and-install jobs from node 22 to node 24 in
// aebd2fb (PR #520). npm 10 — the npm that ships with node 22 — crashes with
// `Cannot read properties of null (reading 'edgesOut')` while its arborist walks
// the dependency tree of a scaffolded `page-money` app. The nightly
// `bump-scaffold-pins.yml` does exactly the same scaffold-and-install, in a job
// nobody thought of as "a CI job", and it was left on node 22.
//
// So every night the bumper computed the right pins, died at
// `validate — scaffold builds against the bumped SDK`, and — because the PR step
// was gated on that step's success — never proposed them. `pins-vs-published` is
// a REQUIRED check with `enforce_admins: true`, so `main` stayed red and every
// open PR stayed blocked until a human bumped by hand.
//
// Measured control pair on the runner's exact versions, on a fresh
// `civitai app init … --template page-money` at the pins of the day
// (`@civitai/app-sdk ^0.39.0`, `@civitai/blocks-react ^0.49.0`), a separate
// clean directory per arm:
//
//   - node 22.23.2 / npm 10.9.8 → `npm install --no-audit --no-fund` exits 1 with
//     `npm error Cannot read properties of null (reading 'edgesOut')`.
//   - node 24.19.0 / npm 11.17.0 → install + `npm run typecheck` + `npm run build`
//     all exit 0, 116 modules transformed.
//
// # Why a test rather than a comment
//
// The comment already existed — four times, verbatim, at every `node-version`
// site in `ci.yml`, ending "Do not drop back to 22". It did not help, because
// the file that was wrong was the one WITHOUT the comment. Sibling drift is not
// fixed by writing the reason in the sibling that is already right; it is fixed
// by something that reads every workflow at once. That is this file.
//
// # What these guards deliberately do NOT claim
//
// They say nothing about whether the nightly works. They are static assertions
// over YAML: they cannot run a workflow, install anything, or observe npm. The
// closing condition for #530 is a real scheduled run, and only the operator can
// see it.

const workflowDir = ".github/workflows"

// minWorkflowFiles is a positive control on the glob itself. If the directory
// moves or the extension convention changes, every assertion below would inspect
// zero files and pass — "found nothing wrong" and "looked at nothing" print the
// same. There were nine workflows when this was written.
const minWorkflowFiles = 9

// minScaffoldInstallJobs is the positive control that matters most here: the
// jobs the node-major rule actually governs are found by REGEX over shell
// scripts, and a regex that matches nothing is indistinguishable from a repo
// where every job already complies.
//
// Four jobs matched when this was written, and they are named in the failure
// message so a drop is diagnosable rather than merely red:
//
//	ci.yml                   → template-page-vite
//	ci.yml                   → template-page-money
//	ci.yml                   → scaffold-currency
//	bump-scaffold-pins.yml   → bump
//
// If you legitimately remove one, lower this number in the same commit and say
// which job went and why. Do not lower it to make a red test green.
const minScaffoldInstallJobs = 4

// minNodeMajor is the floor, not a pin. Newer is fine — the measurement says
// npm 10 (node 22) is broken for this workload and npm 11 (node 24) is not, so
// the invariant is ">= 24", not "== 24".
const minNodeMajor = 24

// scaffoldsAppRe matches a run script that scaffolds an app with this CLI, and
// npmInstallRe one that installs a node dependency tree. The rule below applies
// only to a job doing BOTH: scaffolding alone needs no npm, and installing
// alone (e.g. `release-npm.yml`'s `npm install -g npm@11.18.0`) is not the
// workload that crashes.
//
// 🔴 THESE READ SHELL, AND THIS FILE ALSO CONTAINS PROSE ABOUT SHELL. The
// `classify` job composes an issue body that quotes the very commands the
// scaffold job runs — including a copy-pasteable repro block. A naive matcher
// classifies that job as a scaffold-and-install workload and demands a
// `setup-node` in a job that runs no node at all; that is not hypothetical, it
// is what the first draft of this guard did. Two narrowings, both stated so the
// coverage they cost is visible rather than silent:
//
//  1. `stripProse` drops markdown fenced blocks and `>` blockquote lines before
//     either regex sees the script.
//  2. `npmInstallRe` requires COMMAND POSITION — start of line, or after `&&`,
//     `||`, `;`, `|`, `(`. An `npm install` named mid-sentence is prose.
//
// Residual, named rather than papered over: a job that puts its real install
// inside a markdown fence would not be seen. Nothing does, and nothing has a
// reason to. The scaffold side stays deliberately loose (it only has to name the
// binary anywhere in the script) because BOTH signals are required to match, so
// the tight one carries the discrimination.
var (
	scaffoldsAppRe = regexp.MustCompile(`\bcivitai\s+app\s+(init|create)\b`)
	npmInstallRe   = regexp.MustCompile(`(?m)(?:^|&&|\|\||;|\||\()\s*npm\s+(?:install|ci)\b`)
	setupNodeRe    = regexp.MustCompile(`^actions/setup-node@`)
	nodeMajorRe    = regexp.MustCompile(`^v?(\d+)`)
)

// stripProse removes markdown fenced code blocks and blockquote lines from a
// run script. The fence markers in this repo's workflows are backslash-escaped
// (they live inside double-quoted shell strings), so backslashes are dropped
// before the prefix test.
func stripProse(run string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(run, "\n") {
		t := strings.ReplaceAll(strings.TrimSpace(line), `\`, "")
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(t, ">") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

type wfStep struct {
	Name string         `yaml:"name"`
	ID   string         `yaml:"id"`
	Uses string         `yaml:"uses"`
	If   string         `yaml:"if"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

type wfJob struct {
	Steps []wfStep `yaml:"steps"`
}

type workflowFile struct {
	Jobs map[string]wfJob `yaml:"jobs"`
}

// loadWorkflows parses every workflow file and returns them keyed by base name.
// It fails the test on a parse error rather than skipping the file: a workflow
// this guard cannot read is a workflow this guard is not checking, and that must
// be loud.
func loadWorkflows(t *testing.T) map[string]workflowFile {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(workflowDir, "*.yml"))
	if err != nil {
		t.Fatalf("glob %s: %v", workflowDir, err)
	}
	more, err := filepath.Glob(filepath.Join(workflowDir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob %s: %v", workflowDir, err)
	}
	paths = append(paths, more...)
	sort.Strings(paths)

	if len(paths) < minWorkflowFiles {
		t.Fatalf("positive control failed: found %d workflow files under %s, expected at least %d.\n"+
			"Either the directory moved, the extension convention changed, or this test is running from the wrong "+
			"working directory. Every assertion in this file would have passed vacuously.",
			len(paths), workflowDir, minWorkflowFiles)
	}

	out := make(map[string]workflowFile, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var wf workflowFile
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("parse %s as a GitHub Actions workflow: %v", p, err)
		}
		out[filepath.Base(p)] = wf
	}
	return out
}

// nodeMajorOf extracts the major from a `node-version` value. It returns an
// error rather than a zero for anything it cannot read, because a `lts/*` or a
// `node-version-file:` is a value this guard genuinely cannot evaluate — and
// silently treating it as compliant is how a guard stops guarding.
func nodeMajorOf(v string) (int, error) {
	m := nodeMajorRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, fmt.Errorf("cannot read a node major out of %q", v)
	}
	return strconv.Atoi(m[1])
}

// TestScaffoldAndInstallJobsUseNode24OrNewer is the sibling-drift guard.
//
// It walks every job in every workflow and, for those that BOTH scaffold an app
// and run an npm install, asserts each `actions/setup-node` in that job pins a
// node major >= 24. A matching job with no `setup-node` at all fails too: it
// would run on the runner's default node, which is not a version anyone chose.
func TestScaffoldAndInstallJobsUseNode24OrNewer(t *testing.T) {
	workflows := loadWorkflows(t)

	type jobRef struct{ file, job string }
	var matched []jobRef

	files := make([]string, 0, len(workflows))
	for f := range workflows {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, file := range files {
		jobs := make([]string, 0, len(workflows[file].Jobs))
		for j := range workflows[file].Jobs {
			jobs = append(jobs, j)
		}
		sort.Strings(jobs)

		for _, jobName := range jobs {
			job := workflows[file].Jobs[jobName]

			scaffolds, installs := false, false
			for _, s := range job.Steps {
				script := stripProse(s.Run)
				if scaffoldsAppRe.MatchString(script) {
					scaffolds = true
				}
				if npmInstallRe.MatchString(script) {
					installs = true
				}
			}
			if !scaffolds || !installs {
				continue
			}
			matched = append(matched, jobRef{file, jobName})

			seenSetupNode := false
			for _, s := range job.Steps {
				if !setupNodeRe.MatchString(s.Uses) {
					continue
				}
				seenSetupNode = true

				raw, ok := s.With["node-version"]
				if !ok {
					t.Errorf("%s → job %q: an `actions/setup-node` step has no `node-version:`.\n%s",
						file, jobName, nodeMajorRationale())
					continue
				}
				major, err := nodeMajorOf(fmt.Sprint(raw))
				if err != nil {
					t.Errorf("%s → job %q: %v — this guard cannot evaluate that form, so state the major explicitly.\n%s",
						file, jobName, err, nodeMajorRationale())
					continue
				}
				if major < minNodeMajor {
					t.Errorf("%s → job %q pins node %d for a job that scaffolds an app AND runs `npm install`; the floor is %d.\n%s",
						file, jobName, major, minNodeMajor, nodeMajorRationale())
				}
			}

			if !seenSetupNode {
				t.Errorf("%s → job %q scaffolds an app and runs `npm install` but never runs `actions/setup-node`, "+
					"so it takes whatever node the runner image happens to ship.\n%s",
					file, jobName, nodeMajorRationale())
			}
		}
	}

	if len(matched) < minScaffoldInstallJobs {
		names := make([]string, 0, len(matched))
		for _, m := range matched {
			names = append(names, m.file+" → "+m.job)
		}
		t.Fatalf("positive control failed: this guard found %d job(s) that scaffold an app AND run `npm install`, "+
			"expected at least %d.\nFound: %v\n"+
			"A guard that matches nothing reports the same green as a guard that matched everything and found no "+
			"violation. Either a job was removed (lower `minScaffoldInstallJobs` in the same commit and say which), "+
			"or the shell spelling moved out from under `scaffoldsAppRe`/`npmInstallRe`.",
			len(matched), minScaffoldInstallJobs, names)
	}
}

// nodeMajorRationale is the measured reason, printed with every failure so the
// person who hits this test does not have to go find out why the number is 24.
func nodeMajorRationale() string {
	return "  WHY (measured, issue #530 / PR #520): npm 10 — the npm shipped with node 22 — crashes with\n" +
		"  `npm error Cannot read properties of null (reading 'edgesOut')` while installing a freshly\n" +
		"  scaffolded page-money app. Control pair on a clean directory per arm, at\n" +
		"  `@civitai/app-sdk ^0.39.0` / `@civitai/blocks-react ^0.49.0`:\n" +
		"    node 22.23.2 / npm 10.9.8  → `npm install` exits 1 with the edgesOut crash\n" +
		"    node 24.19.0 / npm 11.17.0 → install + typecheck + build all exit 0\n" +
		"  `ci.yml` moved to 24 for this; `bump-scaffold-pins.yml` was not, and the nightly bumper then\n" +
		"  failed every run for three days while `pins-vs-published` (REQUIRED, enforce_admins) held the\n" +
		"  whole repo red. Do not drop back to 22."
}

// nodeMajorsIn returns the node major of every `actions/setup-node` step in a
// job, in order.
func nodeMajorsIn(t *testing.T, file, jobName string, job wfJob) []int {
	t.Helper()

	var out []int
	for _, s := range job.Steps {
		if !setupNodeRe.MatchString(s.Uses) {
			continue
		}
		raw, ok := s.With["node-version"]
		if !ok {
			t.Fatalf("%s → job %q: an `actions/setup-node` step has no `node-version:`", file, jobName)
		}
		major, err := nodeMajorOf(fmt.Sprint(raw))
		if err != nil {
			t.Fatalf("%s → job %q: %v", file, jobName, err)
		}
		out = append(out, major)
	}
	return out
}

// TestReadyAckDriftRunnerMatchesTheGatesNodeMajor pins the OTHER half of the
// #530 drift, the half the scaffold-and-install rule above structurally cannot
// see: `ready-ack-runtime` exists TWICE under the same name — as the merge GATE
// in `ci.yml`, and as the daily DRIFT runner in `bump-scaffold-pins.yml`. That
// job installs nothing, so the rule above never looks at it.
//
// Two tiers of the same check running on different node majors is a suite that
// can disagree with itself, and a failure visible in one tier can be
// structurally invisible in the other. The drift runner exists precisely to see
// what the gate cannot; it can only do that from the same environment. So this
// asserts they are EQUAL, not merely that each is above the floor — a floor
// would let the gate move to 26 and the runner sit on 24, which is the same
// class of drift #530 was.
func TestReadyAckDriftRunnerMatchesTheGatesNodeMajor(t *testing.T) {
	const (
		gateFile  = "ci.yml"
		driftFile = "bump-scaffold-pins.yml"
		jobName   = "ready-ack-runtime"
	)

	workflows := loadWorkflows(t)

	majors := map[string][]int{}
	for _, file := range []string{gateFile, driftFile} {
		wf, ok := workflows[file]
		if !ok {
			t.Fatalf("%s not found under %s — this guard checked nothing", file, workflowDir)
		}
		job, ok := wf.Jobs[jobName]
		if !ok {
			t.Fatalf("%s has no job %q. The gate and its daily drift runner are matched BY NAME; if one was "+
				"renamed, re-point this guard at the pair rather than deleting it.", file, jobName)
		}
		m := nodeMajorsIn(t, file, jobName, job)
		if len(m) == 0 {
			t.Fatalf("%s → job %q runs no `actions/setup-node`, so this guard has nothing to compare and would "+
				"have passed vacuously.", file, jobName)
		}
		majors[file] = m
	}

	for _, file := range []string{gateFile, driftFile} {
		for _, m := range majors[file] {
			if m < minNodeMajor {
				t.Errorf("%s → job %q pins node %d, below the floor of %d.\n%s",
					file, jobName, m, minNodeMajor, nodeMajorRationale())
			}
		}
	}

	if fmt.Sprint(majors[gateFile]) != fmt.Sprint(majors[driftFile]) {
		t.Errorf("the `%s` GATE in %s runs node %v, but its daily DRIFT runner in %s runs node %v.\n"+
			"  They must match. The drift runner's whole job is to catch, on `main` and between PRs, what the "+
			"gate cannot see; running it on a different node major makes the two tiers able to disagree, so a "+
			"regression can be red in one and structurally invisible in the other. That is exactly the shape of "+
			"#530: ci.yml moved to node 24 and this sibling was left on 22 for three days.\n%s",
			jobName, gateFile, majors[gateFile], driftFile, majors[driftFile], nodeMajorRationale())
	}
}

// TestBumpPRStepStillRequiresChangedOutput pins the fire-drill guarantee.
//
// `workflow_dispatch`'s `drill` input promises, in its own description, that a
// drill "cannot open a PR or rewrite a pin". The mechanism behind that promise
// is narrow: `drill=bump` fails the `bump` step, so `detect changes` — which
// carries no `if:` and therefore defaults to `success()` — is SKIPPED and never
// writes its output, leaving `steps.changed.outputs.changed` empty rather than
// `'true'`.
//
// #530's fix put `always()` on the PR step so a failed scaffold build downgrades
// the PR to a draft instead of suppressing it. `always()` defeats the implicit
// `success()`, which is exactly the mechanism the drill promise leans on — so
// the `changed == 'true'` conjunct is now the ONLY thing standing between a fire
// drill and a real PR. This test is what keeps it there.
//
// It asserts the condition's STATE, not a spelling: the conjunct must be
// present, and the whole expression must contain no `||`, which is what makes
// "present" mean "required" rather than "mentioned in one branch of an or".
func TestBumpPRStepStillRequiresChangedOutput(t *testing.T) {
	const (
		file     = "bump-scaffold-pins.yml"
		jobName  = "bump"
		stepID   = "pr"
		conjunct = "steps.changed.outputs.changed == 'true'"
	)

	workflows := loadWorkflows(t)

	wf, ok := workflows[file]
	if !ok {
		t.Fatalf("%s not found under %s — this guard checked nothing", file, workflowDir)
	}
	job, ok := wf.Jobs[jobName]
	if !ok {
		t.Fatalf("%s has no job %q — this guard checked nothing", file, jobName)
	}

	var step *wfStep
	for i := range job.Steps {
		if job.Steps[i].ID == stepID {
			step = &job.Steps[i]
			break
		}
	}
	if step == nil {
		t.Fatalf("%s → job %q has no step with `id: %s` — the PR step was renamed or removed, so this guard "+
			"checked nothing. Re-point it at the step that opens the bump PR.", file, jobName, stepID)
	}

	// Collapse the folded-scalar line breaks so the assertion is about the
	// expression, not about how it happens to be wrapped in YAML.
	cond := strings.Join(strings.Fields(step.If), " ")

	if !strings.Contains(cond, conjunct) {
		t.Errorf("%s → job %q step %q must require `%s`, and its condition is:\n  %s\n%s",
			file, jobName, stepID, conjunct, cond, drillRationale())
	}
	if strings.Contains(cond, "||") {
		t.Errorf("%s → job %q step %q has an `||` in its condition:\n  %s\n"+
			"An or-branch means `%s` is no longer a REQUIREMENT, only a mention — a fire drill could reach the "+
			"PR step through the other branch. Keep this condition a pure conjunction.\n%s",
			file, jobName, stepID, cond, conjunct, drillRationale())
	}
}

func drillRationale() string {
	return "  WHY: `workflow_dispatch`'s `drill` input tells the operator a drill \"cannot open a PR or rewrite a\n" +
		"  pin\". That promise rests on one thing: `drill=bump` fails the `bump` step, `detect changes` is\n" +
		"  skipped, its output is never written, and this condition sees an empty string instead of 'true'.\n" +
		"  The step's `always()` (added for #530, so a failed scaffold build opens a DRAFT rather than nothing)\n" +
		"  removed the implicit `success()` that used to be a second line of defence. If you change this\n" +
		"  condition, fix the drill's description in the same commit — or, better, do not change it."
}
