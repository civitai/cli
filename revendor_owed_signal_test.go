package cli_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The re-vendor bot's GLOSS-DEBT SIGNAL, driven as shell rather than read as
// prose.
//
// # The defect these tests were written for
//
// `revendor-canonical-schema.yml`'s `owed` step is the only producer of the
// gloss-debt signal: it reads `internal/validate/pattern_gloss_owed.json` with
// `jq` and publishes `count`, `list` and `prnote`. `jq` does not always signal
// failure by its exit status — on a ZERO-BYTE or whitespace-only ledger,
// `jq '.owed | length'` exits **0** and prints **nothing** (measured, jq 1.8.1).
// So `count` came back EMPTY with the step GREEN. `classify` then read
// `${R_OWED_COUNT:-0}`, and `:-` substitutes on EMPTY as well as on unset, so an
// UNPRODUCED count became the string "0":
//
//	state=green  →  failing=false  →  failure-issue.yml CLOSES the open
//	`gloss-owed` issue while the debt row is still committed in the repo
//
// and `prnote` was empty too, so the bot's PR carried no note either. Every
// check green, the debt permanently silent — the exact outcome the ledger exists
// to prevent. The `-update-gloss-ledger` step immediately above is what writes
// that file, so a truncated or interrupted write produces exactly this input.
//
// ⚠ NOT the mechanism this was first diagnosed as, and the difference matters
// because the wrong one is wider and sends a reader hunting a bug that is not
// there. The first reading was "the step runs `set -uo pipefail` with no `-e`,
// so ANY failed `jq` leaves count empty and the step still exits 0". GitHub runs
// every `run:` block as `bash -e {0}`, so errexit is already on and
// `set -uo pipefail` does not clear it — which is why the `probe` step in the
// same file needs its `|| true`. Driving the pre-fix script under `bash -e`:
// the absent-file (rc 2) and not-JSON (rc 5) cases DO fail the step and publish
// no count; only the empty-input cases slip through. The fix spells out
// `set -euo pipefail` anyway, so the script's contract stops depending on the
// platform's default shell flags, but the guard that closes the hole is the
// `case` assertion on `count`.
//
// Compounding it, `owed` was ALSO the one step whose `outcome` was absent from
// `classify`'s `env:` (`R_PROBE`, `R_REVENDOR`, `R_LEDGER`, `R_CHANGED`,
// `R_VALIDATE` and `R_PR` were all there). A failed `owed` skips `detect
// changes`, so `R_CHANGED` came back `skipped` and the `detect-failed` arm fired
// — a verdict whose own text reads "which is unusual — it only runs
// `git status`". The report named the wrong step.
//
// # Why shell-driving rather than regex-over-YAML
//
// `workflow_node_version_test.go` asserts over workflow TEXT, deliberately, and
// says so. That is the right shape for "this job must pin node >= 24", and the
// wrong shape here: the defect was not a missing token, it was the VALUE a
// correct-looking chain of `[ ... ]` tests computes from an empty string. A grep
// for `set -euo pipefail` would pass over a script whose assertion is
// unreachable, and a grep for `owed-failed` would pass over an arm placed below
// `green`. So these tests extract each step's `run:` script verbatim and EXECUTE
// it with the env GitHub would set, then assert the `state=` it writes to
// `$GITHUB_OUTPUT` — the same channel the `signal` job consumes.
//
// What that buys and what it does not: it proves the shell computes the right
// verdict from a given set of step outcomes. It cannot prove GitHub produces
// those outcomes, that `jq` is present on the runner, or that
// `failure-issue.yml` acts on `failing` as documented. Those need a real run.

const revendorWorkflow = "revendor-canonical-schema.yml"

// revendorJob is the job whose steps carry the signal. If it is renamed, every
// lookup below fails by name rather than silently inspecting nothing.
const revendorJob = "revendor"

// classifyEnvByStepID is a TOTAL LEDGER of `revendor`'s identified steps and the
// `classify` env var that carries each one's `outcome`.
//
// 🔴 THIS IS THE SEAM GUARD, and it is the reason the arm ladder alone is not
// enough. The original defect was not a wrong branch — it was a step whose
// outcome `classify` could not see at all, so the ladder mis-attributed its
// failure to the next step down. A guard on the ladder cannot notice that,
// because the ladder it drives is internally consistent either way.
//
// The assertion fails when this set GROWS (a step gains an `id` and no env row,
// i.e. the next instance of the same defect) and when it SHRINKS (a step named
// here disappears, so a row here is describing nothing). `classify` itself is
// the one exception and is listed so the exception is data rather than a
// hardcoded skip.
//
// Residual, named rather than papered over: this asserts the env var EXISTS and
// interpolates that step's `outcome`. It does not assert the ladder BRANCHES on
// it — `TestRevendorClassifyArmLadder` is what does that, and a new step needs a
// row in both.
var classifyEnvByStepID = map[string]string{
	"probe":    "R_PROBE",
	"revendor": "R_REVENDOR",
	"ledger":   "R_LEDGER",
	"owed":     "R_OWED",
	"changed":  "R_CHANGED",
	"validate": "R_VALIDATE",
	"pr":       "R_PR",
	"classify": "", // the classifier cannot classify its own outcome
}

// revendorWorkflowFile is the slice of the workflow file these tests read.
type revendorWorkflowFile struct {
	Jobs map[string]struct {
		Steps []struct {
			Name string            `yaml:"name"`
			ID   string            `yaml:"id"`
			Run  string            `yaml:"run"`
			Env  map[string]string `yaml:"env"`
		} `yaml:"steps"`
		With map[string]string `yaml:"with"`
	} `yaml:"jobs"`
}

func loadRevendorWorkflow(t *testing.T) revendorWorkflowFile {
	t.Helper()
	path := filepath.Join(workflowDir, revendorWorkflow)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var wf revendorWorkflowFile
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if _, ok := wf.Jobs[revendorJob]; !ok {
		got := make([]string, 0, len(wf.Jobs))
		for j := range wf.Jobs {
			got = append(got, j)
		}
		sort.Strings(got)
		t.Fatalf("%s has no job %q (jobs: %v) — these tests look every step up by "+
			"name, so a rename must be followed here rather than leaving them inspecting nothing",
			path, revendorJob, got)
	}
	return wf
}

// revendorStepScript returns one step's `run:` script, and fails loudly when the
// step is gone or its script no longer mentions a token the step is defined by.
//
// `mustContain` is a positive control on the extraction, not a style rule: an
// empty or wrong script would make every assertion below pass over nothing, and
// "the script computes the right answer" and "there is no script" are otherwise
// the same green.
func revendorStepScript(t *testing.T, id string, mustContain ...string) string {
	t.Helper()
	wf := loadRevendorWorkflow(t)
	var hits []string
	for _, s := range wf.Jobs[revendorJob].Steps {
		if s.ID == id {
			hits = append(hits, s.Run)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("job %q: want exactly 1 step with id=%q, found %d", revendorJob, id, len(hits))
	}
	script := hits[0]
	if strings.TrimSpace(script) == "" {
		t.Fatalf("step %q has an empty `run:` script", id)
	}
	for _, want := range mustContain {
		if !strings.Contains(script, want) {
			t.Fatalf("step %q's script no longer contains %q — the extraction may be "+
				"reading the wrong step, or the step was rewritten and these assertions "+
				"need re-deriving rather than trusting.\n--- script ---\n%s", id, want, script)
		}
	}
	return script
}

// runShell writes a script to disk and runs it under bash in `dir`, with a
// hermetic env plus `extra`. It returns the exit code, combined output, and the
// lines the script appended to `$GITHUB_OUTPUT`.
func runShell(t *testing.T, script, dir string, extra map[string]string) (int, string, string) {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "step.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "github_output")
	if err := os.WriteFile(outPath, nil, 0o600); err != nil {
		t.Fatalf("seed GITHUB_OUTPUT: %v", err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = dir
	// Hermetic: PATH only, so the script cannot read anything from the test
	// runner's environment and a missing var surfaces as `set -u` would surface
	// it on the runner.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GITHUB_OUTPUT=" + outPath}
	for k, v := range extra {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	combined, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run script: %v\n%s", err, combined)
		}
		code = ee.ExitCode()
	}
	written, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("read GITHUB_OUTPUT: %v", readErr)
	}
	return code, string(combined), string(written)
}

// writeLedger seeds a working dir with the ledger file at the relative path the
// `owed` step reads. A body equal to `absentLedger` leaves the file out
// entirely — distinct from a zero-byte file, which is its own case and the one
// `set -e` cannot see.
func writeLedger(t *testing.T, body, absentLedger string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "internal", "validate"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if body != absentLedger {
		p := filepath.Join(dir, "internal", "validate", "pattern_gloss_owed.json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("write ledger: %v", err)
		}
	}
	return dir
}

var countLineRe = regexp.MustCompile(`(?m)^count=(.*)$`)
var stateLineRe = regexp.MustCompile(`(?m)^state=(.*)$`)

// TestRevendorOwedStepRefusesAnUnreadableCount pins the PRODUCER half.
//
// The step must publish a count when it can read one, and must FAIL — not go
// green with an empty value — when it cannot. `jq` is what it reads the ledger
// with, and `jq` is an unpinned dependency of this path (pre-installed on
// GitHub-hosted runners, vendored and version-checked nowhere), so "jq could not
// produce a number" is a case that has to be loud.
//
// 🔴 THE `zero-byte` ROW IS THE ONE THE `case` GUARD EXISTS FOR, and the reason
// `set -euo pipefail` alone does not close this. Measured with jq 1.8.1:
//
//	ledger body        jq '.owed | length'      caught by
//	{"owed":[]}        rc 0, "0"                — (valid)
//	{"owed":["a"]}     rc 0, "1"                — (valid)
//	(absent file)      rc 5                     set -e
//	not json           rc 5                     set -e
//	{"_why":"x"}       rc 0, "0"; `list` rc 5   set -e, on the `list` line
//	(zero bytes)       rc 0, ""                 ONLY the `case` guard
//	"   " (spaces)     rc 0, ""                 ONLY the `case` guard
//
// A truncated or interrupted write by the `-update-gloss-ledger` step above
// produces exactly that zero-byte file, and the `list` expression is
// empty-and-clean on the same input, so nothing else in the step or downstream
// of it notices.
func TestRevendorOwedStepRefusesAnUnreadableCount(t *testing.T) {
	script := revendorStepScript(t, "owed", "jq", "pattern_gloss_owed.json", "count=")

	const absent = "\x00ABSENT\x00" // distinct from a zero-byte file, which is a case of its own

	cases := []struct {
		name      string
		ledger    string
		wantOK    bool
		wantCount string
	}{
		{
			name:      "no debt",
			ledger:    `{"_why":"x","owed":[]}`,
			wantOK:    true,
			wantCount: "0",
		},
		{
			name:      "two rows owed",
			ledger:    `{"_why":"x","owed":["^a$","^b$"]}`,
			wantOK:    true,
			wantCount: "2",
		},
		{
			name:      "seven rows owed (not a power of two, and not 0/1/2)",
			ledger:    `{"_why":"x","owed":["^a$","^b$","^c$","^d$","^e$","^f$","^g$"]}`,
			wantOK:    true,
			wantCount: "7",
		},
		{
			name:   "ledger absent — jq exits 5",
			ledger: absent,
			wantOK: false,
		},
		{
			name:   "ledger is not JSON — jq exits 5",
			ledger: "not json at all",
			wantOK: false,
		},
		{
			name:   "ledger has no `owed` key — the `list` expression exits 5",
			ledger: `{"_why":"x"}`,
			wantOK: false,
		},
		{
			// 🔴 The case `set -e` cannot see. jq exits 0 and prints nothing.
			name:   "ledger is zero bytes — jq exits 0 printing nothing",
			ledger: "",
			wantOK: false,
		},
		{
			name:   "ledger is whitespace only — jq exits 0 printing nothing",
			ledger: "   \n  ",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeLedger(t, tc.ledger, absent)
			code, combined, written := runShell(t, script, dir, nil)
			m := countLineRe.FindStringSubmatch(written)

			if !tc.wantOK {
				if code == 0 {
					t.Fatalf("the `owed` step exited 0 on a count it could not read. That is the "+
						"defect: the step is the ONLY producer of the gloss-debt signal, and "+
						"`classify` reads an empty count as \"0 owed\", which CLOSES the "+
						"gloss-owed issue while the debt row is still in the repo.\n"+
						"GITHUB_OUTPUT:\n%s\noutput:\n%s", written, combined)
				}
				if m != nil {
					t.Fatalf("the `owed` step exited %d but still published count=%q to "+
						"GITHUB_OUTPUT. A published count is read downstream whatever the exit "+
						"code, so it must not be written at all when it is not a number.\n%s",
						code, m[1], written)
				}
				return
			}

			if code != 0 {
				t.Fatalf("the `owed` step exited %d on a readable ledger (%s).\noutput:\n%s",
					code, tc.ledger, combined)
			}
			if m == nil {
				t.Fatalf("the `owed` step exited 0 without publishing a count.\nGITHUB_OUTPUT:\n%s", written)
			}
			if m[1] != tc.wantCount {
				t.Fatalf("count=%q, want %q (ledger %s)", m[1], tc.wantCount, tc.ledger)
			}
		})
	}
}

// classifyArm is one row of the verdict ladder: a set of step outcomes, and the
// `state` `classify` must reach from them.
type classifyArm struct {
	name  string
	env   map[string]string
	state string
}

// classifyDefaults is the all-green baseline every arm perturbs. Keeping the
// baseline in one place is what makes each arm's `env` read as "the ONE thing
// that is different".
func classifyDefaults() map[string]string {
	return map[string]string{
		"R_PROBE":      "success",
		"R_PROBE_CODE": "200",
		"R_REVENDOR":   "success",
		"R_LEDGER":     "success",
		"R_OWED":       "success",
		"R_OWED_COUNT": "0",
		"R_OWED_LIST":  "",
		"R_CHANGED":    "success",
		"R_VALIDATE":   "success",
		"R_PR":         "success",
		"DRILL":        "false",
	}
}

// TestRevendorClassifyArmLadder drives the CONSUMER half over every verdict the
// ladder can reach, and is total: `wantStates` below is the full set, so an arm
// that becomes unreachable (shadowed by a branch placed above it) shows up as a
// state nothing produced rather than as a test nobody wrote.
//
// 🔴 The two `unproduced count` rows are the regression cases. Before the fix
// both produced `state=green`.
func TestRevendorClassifyArmLadder(t *testing.T) {
	script := revendorStepScript(t, "classify", "state=", "R_OWED")

	arms := []classifyArm{
		{"probe died", map[string]string{"R_PROBE": "failure"}, "probe-failed"},
		{"re-vendor died", map[string]string{"R_REVENDOR": "failure"}, "revendor-failed"},
		{"ledger reconcile died", map[string]string{"R_LEDGER": "failure"}, "ledger-failed"},
		{
			// 🔴 The count here is deliberately VALID and deliberately not "0".
			// An arm that set both `R_OWED=failure` and an empty count would be
			// caught by either half of the `owed-failed` condition, so it could
			// not tell them apart — and a mutant deleting the `$R_OWED` half
			// would survive it. With a good count, only the `$R_OWED` half can
			// reach this verdict; without it the run would report `gloss-owed`
			// from a step that failed.
			"owed step died", map[string]string{"R_OWED": "failure", "R_OWED_COUNT": "3"}, "owed-failed",
		},
		{
			// The shape a guard keyed only on `$R_OWED` would miss: the step
			// reports success and the count is still unproduced.
			"unproduced count, step green",
			map[string]string{"R_OWED_COUNT": ""},
			"owed-failed",
		},
		{
			// A non-empty, non-numeric count. `!= "0"` is true for it, so a guard
			// that only tested for emptiness would file a gloss-owed issue over a
			// count nobody measured — and the issue body would render the garbage
			// as the number of patterns owed.
			"unproduced count, non-numeric",
			map[string]string{"R_OWED_COUNT": "null"},
			"owed-failed",
		},
		{"canonical unreachable", map[string]string{"R_PROBE_CODE": "503"}, "canonical-unreachable"},
		{"detect changes died", map[string]string{"R_CHANGED": "failure"}, "detect-failed"},
		{"validate died", map[string]string{"R_VALIDATE": "failure"}, "validate-failed"},
		{"PR step died", map[string]string{"R_PR": "failure"}, "pr-failed"},
		{
			"a gloss is owed",
			map[string]string{"R_OWED_COUNT": "2", "R_OWED_LIST": "`^a$`, `^b$`"},
			"gloss-owed",
		},
		{"nothing to report", nil, "green"},
	}

	// Totality both ways: every state the ladder can emit is exercised, and every
	// state exercised is one the ladder can emit. A row added to `arms` without a
	// row here (or the reverse) fails rather than quietly narrowing the drive.
	wantStates := map[string]bool{
		"probe-failed": true, "revendor-failed": true, "ledger-failed": true,
		"owed-failed": true, "canonical-unreachable": true, "detect-failed": true,
		"validate-failed": true, "pr-failed": true, "gloss-owed": true, "green": true,
	}

	seen := map[string]bool{}
	for _, arm := range arms {
		t.Run(arm.name, func(t *testing.T) {
			env := classifyDefaults()
			for k, v := range arm.env {
				env[k] = v
			}
			code, combined, written := runShell(t, script, t.TempDir(), env)
			if code != 0 {
				t.Fatalf("classify exited %d — it runs on `if: always()` and must always "+
					"reach a verdict.\noutput:\n%s", code, combined)
			}
			m := stateLineRe.FindStringSubmatch(written)
			if m == nil {
				t.Fatalf("classify published no state= to GITHUB_OUTPUT.\n%s", written)
			}
			if m[1] != arm.state {
				extra := ""
				if arm.state == "owed-failed" && m[1] == "green" {
					extra = "\n\n🔴 This is the original defect: an unproduced owed count read " +
						"as \"0 owed\", so the run went green and `failure-issue.yml` closed the " +
						"`gloss-owed` issue over a debt row still committed in the repo."
				}
				t.Fatalf("state=%q, want %q%s\noutput:\n%s", m[1], arm.state, extra, combined)
			}
			seen[m[1]] = true
		})
	}

	for s := range wantStates {
		if !seen[s] {
			t.Errorf("no arm reached state %q — either the drive stopped covering it or the "+
				"branch is shadowed by one placed above it", s)
		}
	}
	for s := range seen {
		if !wantStates[s] {
			t.Errorf("an arm reached state %q, which is not in wantStates — add it there in the "+
				"same commit so this drive stays total", s)
		}
	}
}

// TestRevendorClassifyEnvNamesEveryStepOutcome is the SEAM guard: the defect
// above was half a missing branch and half a step `classify` could not see.
//
// It fails when `revendor` gains an identified step with no env row (the next
// instance), and when a row here names a step that no longer exists (a ledger
// describing nothing). It asserts the env value interpolates THAT step's
// `outcome`, not merely that some var of that name exists.
func TestRevendorClassifyEnvNamesEveryStepOutcome(t *testing.T) {
	wf := loadRevendorWorkflow(t)

	var classifyEnv map[string]string
	ids := map[string]bool{}
	for _, s := range wf.Jobs[revendorJob].Steps {
		if s.ID == "" {
			continue
		}
		ids[s.ID] = true
		if s.ID == "classify" {
			classifyEnv = s.Env
		}
	}
	if classifyEnv == nil {
		t.Fatal("no step id=classify, or it has no `env:` — this test would otherwise " +
			"compare every step against an empty map and pass")
	}
	// Positive control on the ledger itself: a map that silently went empty would
	// make both loops below iterate over nothing.
	if len(classifyEnvByStepID) < 8 {
		t.Fatalf("classifyEnvByStepID has %d rows, want at least 8 — it is meant to be "+
			"total over the job's identified steps", len(classifyEnvByStepID))
	}

	for id := range ids {
		envVar, listed := classifyEnvByStepID[id]
		if !listed {
			t.Errorf("step id=%q has no row in classifyEnvByStepID. Either add its outcome to "+
				"`classify`'s env: and a row here, or record it as deliberately unclassified "+
				"with an empty row — a step whose outcome classify cannot see is mis-attributed "+
				"to whichever arm fires instead (that is what happened to `owed`, which fell "+
				"through to `detect-failed`)", id)
			continue
		}
		if envVar == "" {
			continue // deliberately unclassified, e.g. classify itself
		}
		got, ok := classifyEnv[envVar]
		if !ok {
			t.Errorf("classify's env: has no %s, so step %q's outcome is invisible to the "+
				"verdict ladder", envVar, id)
			continue
		}
		want := fmt.Sprintf("steps.%s.outcome", id)
		if !strings.Contains(got, want) {
			t.Errorf("classify's env: %s = %q, want it to interpolate %q — an env var carrying "+
				"the WRONG step's outcome reads as coverage and provides none", envVar, got, want)
		}
	}

	for id := range classifyEnvByStepID {
		if !ids[id] {
			t.Errorf("classifyEnvByStepID names step %q, which job %q no longer has. Remove the "+
				"row; a ledger describing a step that is gone stops anyone looking at the ones "+
				"that are left", id, revendorJob)
		}
	}
}

// TestRevendorSignalTreatsOwedFailedAsFailing closes the last link.
//
// `classify` reaching `owed-failed` changes nothing on its own: `failure-issue.yml`
// acts on the `failing` input, and `failing` is NOT just `result != 'success'` —
// three verdicts are reachable on a job that SUCCEEDED. `owed-failed` is one of
// them (the count came back empty or non-numeric while the step itself reported
// success), so omitting it here would compute the right state and still CLOSE the
// debt's own issue.
func TestRevendorSignalTreatsOwedFailedAsFailing(t *testing.T) {
	path := filepath.Join(workflowDir, revendorWorkflow)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var wf struct {
		Jobs map[string]struct {
			With map[string]string `yaml:"with"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	signal, ok := wf.Jobs["signal"]
	if !ok {
		t.Fatal("no `signal` job — it is what files and closes the bot's issue")
	}
	failing, ok := signal.With["failing"]
	if !ok {
		t.Fatal("the `signal` job passes no `failing:` input")
	}
	// Positive control: the expression must already name the two states that are
	// reachable on a green job for reasons OTHER than this fix, or the assertion
	// below is being made against something that is not the expression.
	for _, state := range []string{"canonical-unreachable", "gloss-owed", "owed-failed"} {
		if !strings.Contains(failing, state) {
			t.Errorf("the `signal` job's `failing:` expression does not name %q, so a run "+
				"reaching that verdict on a GREEN job resolves to failing=false and "+
				"failure-issue.yml CLOSES the issue instead of filing it.\nfailing: %s",
				state, failing)
		}
	}
}
