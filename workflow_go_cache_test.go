package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Guards for the Go cache every CI job restores (.github/actions/setup-go).
//
// Restoring GOCACHE across runs is safe for COMPILED objects, which Go keys by
// content. It is not safe for cached TEST RESULTS: a result is keyed by the
// test binary, its flags, and the files and environment variables the binary
// itself opened. This repo's tests start browsers, run npm, scaffold whole
// projects and call the network, and none of what those subprocesses read is in
// the key. So a restored cache can replay a `--- PASS` for inputs that changed —
// measured for the npm guard at `pins-vs-published` in ci.yml, where the second
// run of an unchanged command printed `(cached)` and contacted npm zero times.
//
// `-count=1` is the documented way to bypass the result cache. These tests make
// it a property of every step CI runs rather than a habit of whoever wrote it.
//
// WHAT THE GUARDS READ: every workflow under .github/workflows, every composite
// action under .github/actions, and every repo script one of those runs in
// command position (followed recursively). A `go test` reached any other way —
// a script named through a variable, a Makefile target (refused, see makeRe),
// a Go program that execs `go test` — is not seen.

// minGoTestInvocations is the positive control. A matcher that finds nothing
// passes exactly like a tree where every invocation complies. Twelve were
// found when this was written: four in ci.yml, four in bump-scaffold-pins.yml,
// two in revendor-canonical-schema.yml, one each in docs-links.yml and
// readme-links.yml (the scripts those workflows run contain none). The floor
// sits below that so moving one does not trip it; it exists to catch "matched
// nothing", not to track a count.
const minGoTestInvocations = 10

// cmdPos is COMMAND POSITION: start of a line, or after `&&`, `||`, `;`, `|`,
// `(`, `then` or `do`, followed by any of the prefixes that still leave the
// next word a command (`time`, `if`, `while`, `until`, `!`, `exec`, `command`)
// and any `VAR=value` assignments. The workflows also QUOTE commands in prose
// (issue bodies, `echo` hints), and a command named mid-sentence is not one the
// step runs; requiring command position is how those are told apart (the same
// rule, for the same reason, as npmInstallRe in workflow_node_version_test.go).
const cmdPos = `(?m)(?:^|&&|\|\||;|\||\(|\bthen\b|\bdo\b)[ \t]*` +
	`(?:(?:time|if|while|until|!|exec|command)[ \t]+)*` +
	`(?:[A-Za-z_][A-Za-z0-9_]*=\S*[ \t]+)*`

// goTestRe finds `go test`, including `go -C <dir> test` and `go -C=<dir> test`.
var goTestRe = regexp.MustCompile(cmdPos + `go(?:[ \t]+-C(?:=|[ \t]+)\S+)?[ \t]+test\b`)

// countOneRe accepts `-count=1` and `--count=1`, and the two-word spelling.
var countOneRe = regexp.MustCompile(`(?:^|\s)--?count(?:=|\s+)1(?:\s|$)`)

// makeRe finds a `make` invocation in command position. The rule cannot see
// inside a make target, so a step that calls one has to be refused rather than
// passed.
var makeRe = regexp.MustCompile(cmdPos + `make\b`)

// repoScriptRe finds a repo-relative script run in command position — as
// `./path`, `path`, or behind `bash`/`sh` — so its contents can be checked like
// the step's own. Only paths that exist as files in this repo are followed.
var repoScriptRe = regexp.MustCompile(cmdPos + `(?:(?:bash|sh)[ \t]+)?(?:\./)?((?:scripts|tools|\.github)/[A-Za-z0-9_./-]+)`)

// goTestInvocation is one `go test` command found in a script, as written.
type goTestInvocation struct {
	where   string // file → job → step [→ script]
	command string // continuation lines joined, up to the first UNQUOTED `|`, `&`, `;`, `)` or newline
}

// goTestInvocations returns every `go test` command in a run script. Backslash
// continuations are joined first, so flags on a later line belong to the
// command they continue, and the command ends at the first separator OUTSIDE
// quotes — `-run 'TestA|TestB'` is one argument, not the end of the command.
func goTestInvocations(where, script string) []goTestInvocation {
	script = joinShellContinuations(script)
	var out []goTestInvocation
	for _, loc := range goTestRe.FindAllStringIndex(script, -1) {
		// The match may begin with the separator that put it in command position.
		rest := strings.TrimLeft(script[loc[0]:], "&|;( \t\n")
		out = append(out, goTestInvocation{where: where, command: strings.TrimSpace(rest[:commandEnd(rest)])})
	}
	return out
}

func joinShellContinuations(script string) string {
	return strings.ReplaceAll(stripProse(script), "\\\n", " ")
}

// commandEnd returns the index of the first shell separator in s that is not
// inside single or double quotes, or len(s). An unquoted `)` closes the
// subshell the command ran in, so it ends the command too.
func commandEnd(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' {
				i++
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '\\':
			i++
		case c == '&' && ((i > 0 && s[i-1] == '>') || (i+1 < len(s) && s[i+1] == '>')):
			// `2>&1` and `&>` are redirections, not separators.
		case c == '|' || c == '&' || c == ';' || c == ')' || c == '\n':
			return i
		}
	}
	return len(s)
}

// ciStep is one step of a workflow job or of a composite action.
type ciStep struct {
	where string // printable location
	file  string // workflow base name, or the action's directory
	job   string // job name; "" for a composite action's step
	step  wfStep
}

type compositeAction struct {
	Runs struct {
		Using string   `yaml:"using"`
		Steps []wfStep `yaml:"steps"`
	} `yaml:"runs"`
}

// actionDir is where this repo's composite actions live.
const actionDir = ".github/actions"

// loadCISteps returns every step CI can run: each job's steps in every
// workflow, and each step of every composite action under .github/actions. A
// composite's steps run inside whichever job uses it, so a save hidden in one
// saves from every such job — reading only the workflows misses exactly that.
func loadCISteps(t *testing.T) []ciStep {
	t.Helper()
	var out []ciStep
	for file, wf := range loadWorkflows(t) {
		for jobName, job := range wf.Jobs {
			for i, s := range job.Steps {
				name := s.Name
				if name == "" {
					name = fmt.Sprintf("step %d", i+1)
				}
				out = append(out, ciStep{where: fmt.Sprintf("%s → %s → %s", file, jobName, name), file: file, job: jobName, step: s})
			}
		}
	}

	paths, err := filepath.Glob(filepath.Join(actionDir, "*", "action.y*ml"))
	if err != nil {
		t.Fatalf("glob %s: %v", actionDir, err)
	}
	if len(paths) == 0 {
		t.Fatalf("positive control failed: no composite action found under %s — every CI job uses one, so this guard "+
			"is not looking where the steps are", actionDir)
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var a compositeAction
		if err := yaml.Unmarshal(raw, &a); err != nil {
			t.Fatalf("parse %s as an action: %v", p, err)
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		for i, s := range a.Runs.Steps {
			name := s.Name
			if name == "" {
				name = s.ID
			}
			if name == "" {
				name = fmt.Sprintf("step %d", i+1)
			}
			out = append(out, ciStep{where: fmt.Sprintf("%s → %s", dir, name), file: dir, step: s})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].where < out[j].where })
	return out
}

// scriptFinding is a refusal raised while reading a script, not a `go test`.
type scriptFinding struct{ where, msg string }

// collectGoTests returns the `go test` invocations in a run script and in every
// repo script it runs, recursively, plus any `make` refusals.
func collectGoTests(where, script string, seen map[string]bool) ([]goTestInvocation, []scriptFinding) {
	invs := goTestInvocations(where, script)
	var refusals []scriptFinding
	joined := joinShellContinuations(script)
	if makeRe.MatchString(joined) {
		refusals = append(refusals, scriptFinding{where, "runs `make`. This guard cannot see the `go test` inside a " +
			"make target, so it cannot vouch for this step: either call the go commands directly, or teach this test " +
			"to read the Makefile target the step runs."})
	}
	for _, m := range repoScriptRe.FindAllStringSubmatch(joined, -1) {
		path := filepath.Clean(m[1])
		if seen[path] {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		seen[path] = true
		body, err := os.ReadFile(path)
		if err != nil {
			refusals = append(refusals, scriptFinding{where, fmt.Sprintf("runs %s, which this guard could not read: %v", path, err)})
			continue
		}
		i, r := collectGoTests(where+" → "+path, string(body), seen)
		invs = append(invs, i...)
		refusals = append(refusals, r...)
	}
	return invs, refusals
}

// TestEveryWorkflowGoTestDisablesTheResultCache fails when any `go test` CI
// runs could be answered from a restored result cache.
func TestEveryWorkflowGoTestDisablesTheResultCache(t *testing.T) {
	steps := loadCISteps(t)

	var found []goTestInvocation
	scripts := 0
	for _, s := range steps {
		if s.step.Run == "" {
			continue
		}
		scripts++
		invs, refusals := collectGoTests(s.where, s.step.Run, map[string]bool{})
		for _, r := range refusals {
			t.Errorf("%s %s", r.where, r.msg)
		}
		found = append(found, invs...)
	}

	for _, inv := range found {
		if !countOneRe.MatchString(inv.command) {
			t.Errorf("%s runs `%s` without `-count=1`.\n"+
				"  Every CI job restores GOCACHE from an earlier run (.github/actions/setup-go), and a cached test "+
				"result does not record what a test's subprocesses read — the browser, npm, a scaffolded project, "+
				"the network. Without `-count=1` this step can replay an old PASS.", inv.where, inv.command)
		}
	}

	t.Logf("inspected %d `go test` invocation(s) across %d run script(s)", len(found), scripts)
	if len(found) < minGoTestInvocations {
		var names []string
		for _, inv := range found {
			names = append(names, inv.where)
		}
		t.Fatalf("positive control failed: found %d `go test` invocation(s) in CI, expected at least %d.\n"+
			"Found: %q\nA matcher that finds nothing passes exactly like a compliant tree. Either invocations were "+
			"removed (lower minGoTestInvocations in the same commit and say which) or the shell spelling moved out "+
			"from under goTestRe.", len(found), minGoTestInvocations, names)
	}
}

// TestGoTestMatcherSeesWhatTheWorkflowsSpell is the negative control for the
// guard above, over spellings this repo's workflows use or plausibly will.
// Each case would have to be MISSED for the guard to pass a non-compliant step.
func TestGoTestMatcherSeesWhatTheWorkflowsSpell(t *testing.T) {
	cases := []struct {
		name, script string
		want         []string // commands found, in order
		compliant    []bool
	}{
		{
			name:      "piped into tee",
			script:    "set -uo pipefail\ngo test ./... 2>&1 | tee \"$RUNNER_TEMP/gotest.log\"\n",
			want:      []string{"go test ./... 2>&1"},
			compliant: []bool{false},
		},
		{
			name:      "flags on a continuation line, a quoted pipe in -run",
			script:    "go test ./internal/cmd \\\n  -run 'TestA|TestB' \\\n  -count=1 -v 2>&1 | tee x.log || rc=$?\n",
			want:      []string{"go test ./internal/cmd    -run 'TestA|TestB'    -count=1 -v 2>&1"},
			compliant: []bool{true},
		},
		{
			name:      "-count=1 after the pipe belongs to tee, not to go test",
			script:    "go test ./... | tee -count=1\n",
			want:      []string{"go test ./..."},
			compliant: []bool{false},
		},
		{
			name:      "env assignment in front",
			script:    "CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold -count=1\n",
			want:      []string{"CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold -count=1"},
			compliant: []bool{true},
		},
		{
			name:      "after &&",
			script:    "cd x && go test ./...\n",
			want:      []string{"go test ./..."},
			compliant: []bool{false},
		},
		{
			name:      "behind time",
			script:    "time go test ./...\n",
			want:      []string{"time go test ./..."},
			compliant: []bool{false},
		},
		{
			name:      "as an if condition",
			script:    "if go test ./...; then echo ok; fi\n",
			want:      []string{"if go test ./..."},
			compliant: []bool{false},
		},
		{
			name:      "negated",
			script:    "! go test ./internal/x -count=1\n",
			want:      []string{"! go test ./internal/x -count=1"},
			compliant: []bool{true},
		},
		{
			name:      "go -C <dir> test",
			script:    "go -C . test ./...\ngo -C=sub test -count=1 ./...\n",
			want:      []string{"go -C . test ./...", "go -C=sub test -count=1 ./..."},
			compliant: []bool{false, true},
		},
		{
			name:      "inside a subshell",
			script:    "(cd \"$clone_dir\" && go test ./...) 2>&1 | tee log\n",
			want:      []string{"go test ./..."},
			compliant: []bool{false},
		},
		{
			name:   "prose is not a command",
			script: "echo \"CIVITAI_CHECK_DOCS_LINKS=1 go test ./internal/cmd \\\\\"\nbody=\"then \\`go test ./...\\` failed\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := goTestInvocations("case", tc.script)
			if len(got) != len(tc.want) {
				t.Fatalf("found %d invocation(s) %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i, inv := range got {
				if inv.command != tc.want[i] {
					t.Errorf("invocation %d = %q, want %q", i, inv.command, tc.want[i])
				}
				if c := countOneRe.MatchString(inv.command); c != tc.compliant[i] {
					t.Errorf("invocation %q: -count=1 detected = %v, want %v", inv.command, c, tc.compliant[i])
				}
			}
		})
	}
}

// TestGoTestGuardFollowsRepoScripts pins that a step which runs a repo script
// is judged by the script's contents. scripts/ci-shallow.sh is the realistic
// case: it runs a bare `go test ./...` in a subshell, and no workflow runs it
// today — so its invocation is outside the guard's verdict until one does, and
// then it must be caught.
func TestGoTestGuardFollowsRepoScripts(t *testing.T) {
	for _, run := range []string{"./scripts/ci-shallow.sh", "bash scripts/ci-shallow.sh", "set -e\nif ./scripts/ci-shallow.sh; then :; fi"} {
		invs, refusals := collectGoTests("case", run, map[string]bool{})
		if len(refusals) != 0 {
			t.Errorf("%q: unexpected refusals %v", run, refusals)
		}
		var bare []string
		for _, inv := range invs {
			if !countOneRe.MatchString(inv.command) {
				bare = append(bare, inv.where+": "+inv.command)
			}
		}
		if len(bare) == 0 {
			t.Errorf("%q: following scripts/ci-shallow.sh found no `go test` without -count=1 (found %d invocation(s)); "+
				"the guard would pass a workflow step that runs it", run, len(invs))
		}
	}
}

// TestCIRestoresTheGoCacheAndSavesItOnce pins what stops the go-cache-* entry
// freezing again, and keeps pull requests out of it.
//
//   - No job in ci.yml uses actions/setup-go's own cache. It keys on go.sum
//     alone and only saves on a miss, which is what froze a near-empty entry for
//     seven weeks. Its `cache` input DEFAULTS to true, so a job that omits it is
//     the hazard, not just one that sets it.
//   - The composite action every job uses RESTORES with actions/cache/restore
//     and nothing else. `actions/cache@` would save from every job, pull
//     requests included, in its post step.
//   - Exactly one step — workflow or composite — saves through actions/cache,
//     and it is ci.yml → build-test, only on a push to main. That is the one
//     step that saves a go-cache-* key.
//
// Out of scope, deliberately: six `actions/setup-go` steps with `cache: true`
// in OTHER workflows (bump-scaffold-pins ×3, docs-links, readme-links,
// revendor-canonical-schema, release, release-homebrew — counted when this was
// written) save setup-go-* keys from inside that action. Those keys are never
// restored by ci.yml, and a save hidden inside a third-party action is not a
// step this guard can see.
func TestCIRestoresTheGoCacheAndSavesItOnce(t *testing.T) {
	const (
		composite    = "./.github/actions/setup-go"
		compositeDir = ".github/actions/setup-go"
		saveIf       = "github.event_name == 'push' && github.ref == 'refs/heads/main' && steps.go.outputs.exact-hit != 'true'"
		// Eight jobs set up Go when this was written; see the positive control.
		minGoJobs = 8
	)

	steps := loadCISteps(t)

	goJobs, restores := 0, 0
	var saves []string
	for _, s := range steps {
		uses := s.step.Uses
		if s.file == "ci.yml" {
			switch {
			case uses == composite:
				goJobs++
			case strings.HasPrefix(uses, "actions/setup-go@"):
				goJobs++
				if fmt.Sprint(s.step.With["cache"]) != "false" {
					t.Errorf("%s uses actions/setup-go with its own cache enabled (cache: %v; the default is "+
						"true). Use %s, which restores the shared Go cache without saving it.",
						s.where, s.step.With["cache"], composite)
				}
			}
		}

		if s.file == compositeDir && strings.HasPrefix(uses, "actions/cache") {
			if strings.HasPrefix(uses, "actions/cache/restore@") {
				restores++
			} else {
				t.Errorf("%s uses %s. The composite runs in EVERY job, pull requests included, so it must only "+
					"restore: use actions/cache/restore@ (actions/cache@ saves in its post step).", s.where, uses)
			}
		}

		if !strings.HasPrefix(uses, "actions/cache/save@") && !strings.HasPrefix(uses, "actions/cache@") {
			continue
		}
		saves = append(saves, s.where)
		if s.file != "ci.yml" || s.job != "build-test" {
			t.Errorf("%s saves a cache. Only ci.yml → build-test may: its cache is the superset the other jobs "+
				"compile, and one saver is what stops a race deciding the contents. (actions/cache without /restore "+
				"also saves, in its post step.)", s.where)
			continue
		}
		if s.step.If != saveIf {
			t.Errorf("%s saves the Go cache under\n  if: %s\nwant exactly\n  if: %s\n"+
				"A save reachable from a pull_request lets a PR write the entry main restores.", s.where, s.step.If, saveIf)
		}
	}

	if goJobs < minGoJobs {
		t.Errorf("positive control failed: %d job(s) in ci.yml set up Go, expected at least %d — this guard is not "+
			"looking at the jobs it was written for", goJobs, minGoJobs)
	}
	if restores != 1 {
		t.Errorf("%s has %d actions/cache/restore@ step(s), want exactly 1 — without it no job restores anything", compositeDir, restores)
	}
	if len(saves) != 1 {
		t.Errorf("found %d cache save step(s) %q, want exactly 1 (ci.yml → build-test)", len(saves), saves)
	}
}
