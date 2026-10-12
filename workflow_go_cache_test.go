package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
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
// it a property of every workflow rather than a habit of whoever wrote the step.

// minGoTestInvocations is the positive control. A matcher that finds nothing
// passes exactly like a tree where every invocation complies. Twelve were
// found when this was written: four in ci.yml, four in bump-scaffold-pins.yml,
// two in revendor-canonical-schema.yml, one each in docs-links.yml and
// readme-links.yml. The floor sits below that so moving one does not trip it;
// it exists to catch "matched nothing", not to track a count.
const minGoTestInvocations = 10

// goTestRe finds `go test` in COMMAND POSITION — start of a line, or after
// `&&`, `||`, `;`, `|` or `(` — optionally behind `VAR=value` assignments. The
// workflows also QUOTE `go test` in prose (issue bodies, `echo` hints), and a
// command named mid-sentence is not one this job runs; requiring command
// position is how those are told apart (the same rule, for the same reason, as
// npmInstallRe in workflow_node_version_test.go).
var goTestRe = regexp.MustCompile(`(?m)(?:^|&&|\|\||;|\||\()[ \t]*(?:[A-Za-z_][A-Za-z0-9_]*=\S*[ \t]+)*go[ \t]+test\b`)

// countOneRe accepts `-count=1` and `--count=1`, and the two-word spelling.
var countOneRe = regexp.MustCompile(`(?:^|\s)--?count(?:=|\s+)1(?:\s|$)`)

// makeRe finds a `make` invocation in command position. The rule cannot see
// inside a make target, so a workflow that calls one has to be refused rather
// than passed.
var makeRe = regexp.MustCompile(`(?m)(?:^|&&|\|\||;|\||\()[ \t]*make\b`)

// goTestInvocation is one `go test` command found in a script, as written.
type goTestInvocation struct {
	where   string // file → job → step
	command string // continuation lines joined, up to the first UNQUOTED `|`, `&`, `;` or newline
}

// goTestInvocations returns every `go test` command in a run script. Backslash
// continuations are joined first, so flags on a later line belong to the
// command they continue, and the command ends at the first separator OUTSIDE
// quotes — `-run 'TestA|TestB'` is one argument, not the end of the command.
func goTestInvocations(where, script string) []goTestInvocation {
	script = strings.ReplaceAll(stripProse(script), "\\\n", " ")
	var out []goTestInvocation
	for _, loc := range goTestRe.FindAllStringIndex(script, -1) {
		// The match may begin with the separator that put it in command position.
		rest := strings.TrimLeft(script[loc[0]:], "&|;( \t\n")
		out = append(out, goTestInvocation{where: where, command: strings.TrimSpace(rest[:commandEnd(rest)])})
	}
	return out
}

// commandEnd returns the index of the first shell separator in s that is not
// inside single or double quotes, or len(s).
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
		case c == '|' || c == '&' || c == ';' || c == '\n':
			return i
		}
	}
	return len(s)
}

// workflowRunScripts returns every step's run script in every workflow, keyed
// by a printable location, in a stable order.
func workflowRunScripts(t *testing.T) (locations []string, scripts map[string]string) {
	t.Helper()
	scripts = map[string]string{}
	for file, wf := range loadWorkflows(t) {
		for jobName, job := range wf.Jobs {
			for i, s := range job.Steps {
				if s.Run == "" {
					continue
				}
				name := s.Name
				if name == "" {
					name = fmt.Sprintf("step %d", i+1)
				}
				loc := fmt.Sprintf("%s → %s → %s", file, jobName, name)
				locations = append(locations, loc)
				scripts[loc] = s.Run
			}
		}
	}
	sort.Strings(locations)
	return locations, scripts
}

// TestEveryWorkflowGoTestDisablesTheResultCache fails when any `go test` a
// workflow runs could be answered from a restored result cache.
func TestEveryWorkflowGoTestDisablesTheResultCache(t *testing.T) {
	locations, scripts := workflowRunScripts(t)

	var found []goTestInvocation
	for _, loc := range locations {
		if makeRe.MatchString(stripProse(scripts[loc])) {
			t.Errorf("%s runs `make`. This guard cannot see the `go test` inside a make target, so it cannot vouch "+
				"for this step: either call the go commands directly, or teach this test to read the Makefile "+
				"target the step runs.", loc)
		}
		found = append(found, goTestInvocations(loc, scripts[loc])...)
	}

	for _, inv := range found {
		if !countOneRe.MatchString(inv.command) {
			t.Errorf("%s runs `%s` without `-count=1`.\n"+
				"  Every CI job restores GOCACHE from an earlier run (.github/actions/setup-go), and a cached test "+
				"result does not record what a test's subprocesses read — the browser, npm, a scaffolded project, "+
				"the network. Without `-count=1` this step can replay an old PASS.", inv.where, inv.command)
		}
	}

	t.Logf("inspected %d `go test` invocation(s) across %d run script(s)", len(found), len(locations))
	if len(found) < minGoTestInvocations {
		var names []string
		for _, inv := range found {
			names = append(names, inv.where)
		}
		t.Fatalf("positive control failed: found %d `go test` invocation(s) in the workflows, expected at least %d.\n"+
			"Found: %q\nA matcher that finds nothing passes exactly like a compliant tree. Either invocations were "+
			"removed (lower minGoTestInvocations in the same commit and say which) or the shell spelling moved out "+
			"from under goTestRe.", len(found), minGoTestInvocations, names)
	}
}

// TestGoTestMatcherSeesWhatTheWorkflowsSpell is the negative control for the
// guard above, over spellings this repo's workflows actually use. Each case
// would have to be MISSED for the guard to pass a non-compliant step.
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

// TestCIRestoresTheGoCacheAndSavesItOnce pins the two properties that stop the
// cache freezing again, and keep pull requests out of it.
//
//   - No job in ci.yml uses actions/setup-go's own cache. It keys on go.sum
//     alone and only saves on a miss, which is what froze a near-empty entry for
//     seven weeks. Its `cache` input DEFAULTS to true, so a job that omits it is
//     the hazard, not just one that sets it.
//   - Exactly one step in any workflow saves a Go cache, and only on a push to
//     main. A second saver reintroduces the race over whose contents win; a save
//     reachable from a pull request lets a PR write the entry main restores.
func TestCIRestoresTheGoCacheAndSavesItOnce(t *testing.T) {
	const (
		composite = "./.github/actions/setup-go"
		saveIf    = "github.event_name == 'push' && github.ref == 'refs/heads/main' && steps.go.outputs.exact-hit != 'true'"
		// Eight jobs set up Go when this was written; see the positive control.
		minGoJobs = 8
	)

	if _, err := os.Stat(filepath.Join(filepath.FromSlash(composite), "action.yml")); err != nil {
		t.Fatalf("the composite action every CI job uses is missing: %v", err)
	}

	workflows := loadWorkflows(t)
	ci, ok := workflows["ci.yml"]
	if !ok {
		t.Fatal("ci.yml not found — this guard checked nothing")
	}

	goJobs := 0
	for jobName, job := range ci.Jobs {
		for _, s := range job.Steps {
			switch {
			case s.Uses == composite:
				goJobs++
			case strings.HasPrefix(s.Uses, "actions/setup-go@"):
				goJobs++
				if fmt.Sprint(s.With["cache"]) != "false" {
					t.Errorf("ci.yml → %s uses actions/setup-go with its own cache enabled (cache: %v; the default is "+
						"true). Use %s, which restores the shared Go cache without saving it.",
						jobName, s.With["cache"], composite)
				}
			}
		}
	}
	if goJobs < minGoJobs {
		t.Errorf("positive control failed: %d job(s) in ci.yml set up Go, expected at least %d — this guard is not "+
			"looking at the jobs it was written for", goJobs, minGoJobs)
	}

	var saves []string
	for file, wf := range workflows {
		for jobName, job := range wf.Jobs {
			for _, s := range job.Steps {
				if !strings.HasPrefix(s.Uses, "actions/cache/save@") && !strings.HasPrefix(s.Uses, "actions/cache@") {
					continue
				}
				saves = append(saves, file+" → "+jobName)
				if file != "ci.yml" || jobName != "build-test" {
					t.Errorf("%s → %s saves a cache. Only ci.yml → build-test may: its cache is the superset the "+
						"other jobs compile, and one saver is what stops a race deciding the contents. (actions/cache "+
						"without /restore also saves, in its post step.)", file, jobName)
					continue
				}
				if s.If != saveIf {
					t.Errorf("ci.yml → build-test saves the Go cache under\n  if: %s\nwant exactly\n  if: %s\n"+
						"A save reachable from a pull_request lets a PR write the entry main restores.", s.If, saveIf)
				}
			}
		}
	}
	if len(saves) != 1 {
		t.Errorf("found %d cache save step(s) %q, want exactly 1 (ci.yml → build-test)", len(saves), saves)
	}
}
