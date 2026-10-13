package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The `@civitai/cli` npm wrapper (npm/) ships the binary for some platforms as
// a separate "platform package" — `@civitai/cli-linux-x64` — that the main
// package names under optionalDependencies, so that `npm install @civitai/cli`
// works where only the npm registry is reachable. Every other platform still
// downloads from the GitHub Release in postinstall.
//
// Three places have to agree on which platform packages exist, and nothing but
// this file reads all three:
//
//	npm/lib/platforms.json             the table; the resolver reads it at run time
//	npm/package.json                   optionalDependencies — what npm installs
//	.github/workflows/release-npm.yml  what gets published
//
// A row with no optionalDependencies entry is a published package nobody
// installs; an entry with no row is a dependency that is never published, which
// npm tolerates SILENTLY because it is optional — every install would quietly
// take the GitHub download again. Neither produces an error anywhere.
//
// # What this deliberately does NOT claim
//
// It is a static read of JSON and YAML. It cannot run the publish, and it
// cannot see npm: whether each package name exists on the registry and has a
// trusted publisher configured is an operator step (see the comment at the top
// of release-npm.yml).

const (
	npmPlatformsFile   = "npm/lib/platforms.json"
	npmPackageFile     = "npm/package.json"
	npmReleaseWorkflow = ".github/workflows/release-npm.yml"
	npmReleaseScript   = "scripts/release.js"
)

type npmPlatformRow struct {
	Package string `json:"package"`
	OS      string `json:"os"`
	CPU     string `json:"cpu"`
}

type npmPackageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Files                []string          `json:"files"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

func npmReadJSON(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func npmSortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestNpmPlatformTableMatchesOptionalDependencies(t *testing.T) {
	var rows []npmPlatformRow
	npmReadJSON(t, npmPlatformsFile, &rows)
	var pkg npmPackageJSON
	npmReadJSON(t, npmPackageFile, &pkg)

	// Positive control: an empty table makes every comparison below pass.
	if len(rows) == 0 {
		t.Fatalf("%s lists no platform packages — the comparison would be vacuous", npmPlatformsFile)
	}

	fromTable := map[string]string{}
	hosts := map[string]string{}
	for _, r := range rows {
		if !strings.HasPrefix(r.Package, pkg.Name+"-") {
			t.Errorf("%s: package %q must be named %s-<platform>", npmPlatformsFile, r.Package, pkg.Name)
		}
		if _, dup := fromTable[r.Package]; dup {
			t.Errorf("%s: package %q is listed twice", npmPlatformsFile, r.Package)
		}
		if prev, dup := hosts[r.OS+"/"+r.CPU]; dup {
			t.Errorf("%s: %s/%s is claimed by both %q and %q", npmPlatformsFile, r.OS, r.CPU, prev, r.Package)
		}
		hosts[r.OS+"/"+r.CPU] = r.Package
		// A row outside the main package's own os/cpu could never be installed.
		if !npmContains(pkg.OS, r.OS) || !npmContains(pkg.CPU, r.CPU) {
			t.Errorf("%s: %q targets %s/%s, which %s does not list under os/cpu", npmPlatformsFile, r.Package, r.OS, r.CPU, npmPackageFile)
		}
		fromTable[r.Package] = pkg.Version
	}

	want, got := npmSortedKeys(fromTable), npmSortedKeys(pkg.OptionalDependencies)
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Fatalf("platform packages drifted apart:\n  %s lists:            %v\n  %s optionalDependencies: %v\n"+
			"Adding or removing a platform is one row in the table AND its optionalDependencies entry.",
			npmPlatformsFile, want, npmPackageFile, got)
	}
	// The pin is exact and equal to the package's own version. The release
	// workflow stamps both; in the repo both are the unstamped placeholder.
	for name, v := range pkg.OptionalDependencies {
		if v != pkg.Version {
			t.Errorf("%s: optionalDependencies[%q] is %q, want the package's own version %q (an exact pin, no range)", npmPackageFile, name, v, pkg.Version)
		}
	}

	// The table is read at run time, so it has to be in the published package.
	if !npmContains(pkg.Files, "lib/") {
		t.Errorf("%s: files no longer includes lib/, so %s would not be published", npmPackageFile, npmPlatformsFile)
	}
}

func npmContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// The workflow carries NO list of its own: it asks scripts/release.js, which
// reads the table. This pins that, and the ordering the design depends on —
// platform packages are published by a step that can fail the job, BEFORE the
// main package that pins them.
func TestNpmReleaseWorkflowPublishesPlatformPackagesFromTheTableFirst(t *testing.T) {
	var rows []npmPlatformRow
	npmReadJSON(t, npmPlatformsFile, &rows)

	raw, err := os.ReadFile(npmReleaseWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			ContinueOnError any `yaml:"continue-on-error"`
			Steps           []struct {
				Name            string `yaml:"name"`
				If              string `yaml:"if"`
				Run             string `yaml:"run"`
				WorkingDir      string `yaml:"working-directory"`
				ContinueOnError any    `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", npmReleaseWorkflow, err)
	}
	job, ok := wf.Jobs["publish"]
	if !ok {
		t.Fatalf("%s has no `publish` job", npmReleaseWorkflow)
	}
	if job.ContinueOnError != nil {
		t.Errorf("the publish job sets continue-on-error; a failed platform publish must fail the release")
	}

	publishRe := regexp.MustCompile(`(?m)\bnpm publish\b`)
	pin, platforms, mainPublish := -1, -1, -1
	for i, s := range job.Steps {
		if s.ContinueOnError != nil {
			t.Errorf("step %q sets continue-on-error; every step before the main publish must be able to stop it", s.Name)
		}
		// The workflow must not name a platform package itself — that would be
		// a second list, free to drift from the table.
		for _, r := range rows {
			if strings.Contains(s.Run, r.Package) {
				t.Errorf("step %q hardcodes %q; the platform list comes from %s via %s", s.Name, r.Package, npmPlatformsFile, npmReleaseScript)
			}
		}
		if strings.Contains(s.Run, npmReleaseScript+" pin ") {
			pin = i
		}
		if !publishRe.MatchString(s.Run) {
			continue
		}
		switch {
		case strings.Contains(s.Run, npmReleaseScript+" platforms ") && strings.Contains(s.Run, npmReleaseScript+" build-platform "):
			if platforms != -1 {
				t.Errorf("more than one step publishes platform packages")
			}
			platforms = i
			if s.If != "" {
				t.Errorf("step %q is conditional (`if: %s`); it must run on every release", s.Name, s.If)
			}
			if strings.Contains(s.Run, "|| true") {
				t.Errorf("step %q swallows a failure with `|| true`", s.Name)
			}
			if !strings.Contains(s.Run, "set -euo pipefail") {
				t.Errorf("step %q must run under `set -euo pipefail` so a failed publish inside the loop fails the step", s.Name)
			}
		default:
			if mainPublish != -1 {
				t.Errorf("more than one step publishes the main package")
			}
			mainPublish = i
			if regexp.MustCompile(`always\(\)|failure\(\)|cancelled\(\)`).MatchString(s.If) {
				t.Errorf("step %q runs even after a failure (`if: %s`); it must not publish when the platform packages did not", s.Name, s.If)
			}
		}
	}

	if platforms == -1 {
		t.Fatalf("no step in %s publishes the platform packages from `%s platforms` + `%s build-platform`", npmReleaseWorkflow, npmReleaseScript, npmReleaseScript)
	}
	if mainPublish == -1 {
		t.Fatalf("no step in %s publishes the main package", npmReleaseWorkflow)
	}
	if pin == -1 {
		t.Fatalf("no step in %s runs `%s pin <version>`; the main package would publish with the placeholder pin", npmReleaseWorkflow, npmReleaseScript)
	}
	if platforms > mainPublish {
		t.Errorf("the main package is published (step %d) BEFORE its platform packages (step %d): it would pin a version that does not exist yet", mainPublish+1, platforms+1)
	}
	if pin > mainPublish {
		t.Errorf("optionalDependencies are pinned (step %d) AFTER the main package is published (step %d)", pin+1, mainPublish+1)
	}
}

// minNpmWrapperTests is the positive control on the node run below: a glob that
// matches no file, or a file that registers no test, exits 0. There were nine
// tests in npm/test/wrapper.test.js when this was written. If you remove one,
// lower this in the same commit.
const minNpmWrapperTests = 9

// The wrapper is JavaScript, so its behaviour is tested in JavaScript
// (npm/test/*.test.js, node's built-in runner — the wrapper stays
// dependency-free). This runs those tests from `go test ./...` so they gate
// wherever the Go suite does (the `build-test` job in ci.yml) instead of
// needing a job of their own that nothing requires.
func TestNpmWrapperNodeTests(t *testing.T) {
	inCI := os.Getenv("CI") != ""
	// The tests exec shell-script stand-ins for the binary and pin linux/x64
	// asset names; CI is ubuntu x64, where they must run.
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skipf("npm wrapper tests only run on linux/amd64 (this is %s/%s)", runtime.GOOS, runtime.GOARCH)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		if inCI {
			t.Fatalf("node is not on PATH under CI; the npm wrapper tests must run here, not skip")
		}
		t.Skip("node is not on PATH")
	}
	files, err := filepath.Glob("npm/test/*.test.js")
	if err != nil || len(files) == 0 {
		t.Fatalf("no npm/test/*.test.js files found (err=%v)", err)
	}

	args := append([]string{"--test", "--test-reporter=tap"}, files...)
	cmd := exec.Command(node, args...)
	// Scrub anything that could point the wrapper at a real binary or make the
	// child node preload something.
	cmd.Env = append(os.Environ(), "CIVITAI_CLI_BINARY=", "NODE_OPTIONS=")
	out, runErr := cmd.CombinedOutput()

	count := func(key string) int {
		m := regexp.MustCompile(`(?m)^# ` + key + ` (\d+)$`).FindSubmatch(out)
		if m == nil {
			t.Fatalf("node --test printed no `# %s N` summary line; cannot tell what ran.\n%s", key, out)
		}
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}
	pass, fail, skipped := count("pass"), count("fail"), count("skipped")
	if runErr != nil || fail != 0 {
		t.Fatalf("npm wrapper tests failed (%v): pass=%d fail=%d\n%s", runErr, pass, fail, out)
	}
	if skipped != 0 || pass < minNpmWrapperTests {
		t.Fatalf("npm wrapper tests: pass=%d skipped=%d, want at least %d passing and none skipped\n%s", pass, skipped, minNpmWrapperTests, out)
	}
}
