package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The `@civitai/cli` npm wrapper (npm/) ships the binary for some platforms as
// a separate "platform package" — `@civitai/cli-linux-x64` — that the published
// main package names under optionalDependencies, so that `npm install
// @civitai/cli` works where only the npm registry is reachable. The list of
// platform packages is npm/lib/platforms.json; `npm/scripts/release.js pin`
// generates optionalDependencies from it at publish time (the committed
// package.json carries none), and npm/test/wrapper.test.js covers that.
//
// What is pinned HERE is one thing: the ORDER of three steps in
// release-npm.yml — pin, then publish the platform packages, then publish the
// main package. npm tolerates a missing optional dependency silently, so a main
// package published first, or unpinned, would not error anywhere; every install
// would just take the GitHub download again.
//
// It does NOT check that a failed platform publish stops the job, that the
// checksum is verified, or anything else about what the steps do — it reads
// YAML and cannot run the publish.

const (
	npmReleaseWorkflow = ".github/workflows/release-npm.yml"
	npmReleaseScript   = "scripts/release.js"
)

func TestNpmReleaseWorkflowPinsThenPublishesPlatformPackagesThenMain(t *testing.T) {
	raw, err := os.ReadFile(npmReleaseWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", npmReleaseWorkflow, err)
	}

	publishRe := regexp.MustCompile(`\bnpm publish\b`)
	pin, platforms, mainPublish := -1, -1, -1
	for i, s := range wf.Jobs["publish"].Steps {
		switch {
		case strings.Contains(s.Run, npmReleaseScript+" pin "):
			pin = i
		case publishRe.MatchString(s.Run) && strings.Contains(s.Run, npmReleaseScript+" build-platform "):
			platforms = i
		case publishRe.MatchString(s.Run) && mainPublish == -1:
			// The FIRST main publish: a later one must not hide an earlier one.
			mainPublish = i
		}
	}
	if pin == -1 || platforms == -1 || mainPublish == -1 {
		t.Fatalf("%s `publish` job: could not find all three steps (pin=%d, platform publish=%d, main publish=%d; -1 = missing)",
			npmReleaseWorkflow, pin, platforms, mainPublish)
	}
	if !(pin <= platforms && platforms < mainPublish) {
		t.Fatalf("%s step order is pin=%d, platform publish=%d, main publish=%d; want pin, then platform packages, then the main package that pins them",
			npmReleaseWorkflow, pin+1, platforms+1, mainPublish+1)
	}
}

// minNpmWrapperTests is the positive control on the node run below: a glob that
// matches no file, or a file that registers no test, exits 0. There were eleven
// tests in npm/test/wrapper.test.js when this was written. If you remove one,
// lower this in the same commit.
const minNpmWrapperTests = 11

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
