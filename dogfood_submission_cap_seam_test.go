package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// THE SEAM between the dogfood submission cap and the CLI it caps.
//
// 🔴 WHY THESE EXIST. The cap in scripts/dogfood/runner.py charges every `civitai
// app submit` attempt and gives the charge back when the attempt's own result
// proves nothing was contacted (see the SUBMIT_PREFLIGHT_REFUSAL block there, and
// dogfood_submission_cap_test.go for the measurement that forced it). That refund
// rests on TWO facts that live in Go, in internal/cmd/app_submit.go, with no
// import between the two languages:
//
//	(1) a non-interactive `app submit` WITHOUT --yes refuses BEFORE any network
//	    call, and the sentence it refuses with is the one the refund matches;
//	(2) `--package-only` writes the .zip and contacts nothing, which is the whole
//	    justification for exempting it from the cap.
//
// A hardcoded string on both sides is two copies of one assumption. So these tests
// RUN THE REAL CLI against a recording server, measure both facts, and feed the
// CLI's ACTUAL BYTES back through runner.py — the python side is never handed a
// string this file typed. When the CLI changes, the refund stops firing and the
// harness over-refuses, which is the safe direction; these turn that silent drift
// into a red test.
//
// 🔴 AND THE THIRD FACT, WHICH IS THIS REPO'S OWN: fact (1) holds only on a
// NON-TTY stdin. `sh()` runs every model command through `docker exec -i`, i.e. a
// pipe, so stdin is never a terminal — TestDogfoodRunnerRunsEveryCommandOnANonTTYStdin
// pins that rather than leaving it as a comment.
//
// No account, no real endpoint, no money: the only server contacted is an
// httptest one on 127.0.0.1, and every arm asserts how many times it was hit.

var (
	seamCLIOnce sync.Once
	seamCLIPath string
	seamCLIErr  error
)

// builtCLI builds ./cmd/civitai once per test binary and returns the path.
//
// 🔴 THE REAL BINARY, NOT A CALL INTO THE PACKAGE. internal/cmd already unit-tests
// confirmSubmit (TestConfirmSubmit_NonTTYRefusesWithoutYes) and --package-only
// (TestAppSubmit_PackageOnlyBypassesGate); what is unpinned is whether the BYTES a
// shelled-out `civitai app submit` writes are the bytes runner.py matches on. Only
// the binary can answer that — the dogfood harness shells out, and so does this.
func builtCLI(t *testing.T) string {
	t.Helper()
	seamCLIOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dogfood-seam-cli-")
		if err != nil {
			seamCLIErr = err
			return
		}
		seamCLIPath = filepath.Join(dir, "civitai")
		out, err := exec.Command("go", "build", "-o", seamCLIPath, "./cmd/civitai").CombinedOutput()
		if err != nil {
			seamCLIErr = fmt.Errorf("building ./cmd/civitai: %v\n%s", err, out)
		}
	})
	if seamCLIErr != nil {
		t.Fatalf("%v", seamCLIErr)
	}
	return seamCLIPath
}

// A minimal submittable app tree.
func seamAppDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "seam-block",
  "version": "0.1.0",
  "name": "Seam Block",
  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "Seam Block", "icon": "bolt" },
  "iframe": { "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "minApiVersion": "1.0"
}`
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runSeamCLI runs the built CLI with a token configured and EVERY endpoint pointed
// at `base`, and returns what the dogfood harness's `sh()` would have handed the
// model: `exit code: N\n<stdout><stderr>`.
//
// 🔴 THE RESULT IS ASSEMBLED THE WAY sh() ASSEMBLES IT, or this test would be
// measuring a string shape the runner never sees. sh() prefixes the exit code and
// appends stderr under a `[stderr]` header when it is non-empty.
func runSeamCLI(t *testing.T, base string, args ...string) (result string, code int) {
	t.Helper()
	bin := builtCLI(t)
	home := t.TempDir()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"CIVITAI_TOKEN=seam-token-not-a-real-credential",
		"CIVITAI_BASE_URL="+base,
		"CIVITAI_SUBMIT_PATH=/api/blocks/submit-version",
		// Stdin is inherited as /dev/null here, which is not a terminal — the same
		// condition `docker exec -i` creates in a real trial.
		"NO_COLOR=1",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code = 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running the CLI: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	body := stdout.String()
	if strings.TrimSpace(stderr.String()) != "" {
		body += "\n[stderr]\n" + stderr.String()
	}
	return fmt.Sprintf("exit code: %d\n%s", code, body), code
}

// runnerSaysContactedNothing asks runner.py's own predicate whether a result
// proves nothing was contacted. Read from the module rather than restated: a copy
// of the rule in Go would be the second assumption this file exists to delete.
func runnerSaysContactedNothing(t *testing.T, result string) bool {
	t.Helper()
	py := dogfoodPython(t)
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("r", sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
print(json.dumps(bool(m.submit_contacted_nothing(sys.stdin.read()))))
`
	cmd := exec.Command(py, "-c", script, filepath.Join(dogfoodDir, "runner.py"))
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	cmd.Stdin = strings.NewReader(result)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("asking runner.py about a result: %v", err)
	}
	var got bool
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("submit_contacted_nothing did not return a bool: %v\n%s", err, out)
	}
	return got
}

// ── seam guard 1: the refusal the refund matches is the one the CLI emits ─────

// 🔴 THE REFUND'S TEXT, MEASURED RATHER THAN ASSERTED. Runs the real CLI the way
// the harness does — shelled out, non-TTY stdin, a token configured — and hands
// its actual output to runner.py's predicate. Every control is in the same test:
// the endpoint counts its hits, and the `--yes` arm proves that counter can move.
func TestDogfoodRefundIsTheRefusalTheCLIActuallyEmits(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"publishRequestId":"pubreq_seam","slug":"seam-block","version":"0.1.0","status":"pending"}`))
	}))
	defer srv.Close()
	hitCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return hits
	}

	// (1) A bare non-interactive submit. It must refuse, and it must have
	// contacted nothing.
	bareResult, bareCode := runSeamCLI(t, srv.URL, "app", "submit", seamAppDir(t))
	if bareCode == 0 {
		t.Fatalf("a bare non-interactive `app submit` exited 0 — the pre-flight refusal the refund "+
			"rests on is gone. The refund must now be removed or re-derived, because the harness "+
			"can no longer tell a refused attempt from a real submission.\n%s", bareResult)
	}
	if n := hitCount(); n != 0 {
		t.Fatalf("the bare submit reached the endpoint %d time(s). The refund's entire claim is that "+
			"THIS invocation contacts nothing; if it does, refunding it gives back a submission that "+
			"was really sent.\n%s", n, bareResult)
	}
	if !runnerSaysContactedNothing(t, bareResult) {
		t.Fatalf("runner.py does NOT recognise the refusal the CLI actually emitted, so the dogfood "+
			"submission cap will charge a refused attempt and block the `--yes` retry — exactly the "+
			"ab-ship-mimo-01 defect, re-opened by a change to this message.\n"+
			"  fix: update SUBMIT_PREFLIGHT_REFUSAL in scripts/dogfood/runner.py to a substring of "+
			"what the CLI now says.\nthe CLI said:\n%s", bareResult)
	}

	// (2) POSITIVE CONTROL for the endpoint counter. A `--yes` submit on the same
	// server MUST move it — otherwise the zero above is a fact about the wiring
	// and proves nothing about the refusal.
	yesResult, yesCode := runSeamCLI(t, srv.URL, "app", "submit", seamAppDir(t), "--yes")
	if yesCode != 0 {
		t.Fatalf("CONTROL failure, not a finding: `app submit --yes` against the recorder failed, so "+
			"the hit counter was never exercised and the zero above is unvalidated.\n%s", yesResult)
	}
	if n := hitCount(); n < 1 {
		t.Fatalf("CONTROL failure, not a finding: `--yes` submitted successfully and the recorder "+
			"counted %d hit(s). The counter is not wired to the submit path, so \"the bare submit "+
			"contacted nothing\" is unmeasured.", n)
	}

	// (3) NEGATIVE CONTROL for the predicate: a submission that really happened
	// must not be refundable.
	if runnerSaysContactedNothing(t, yesResult) {
		t.Fatalf("runner.py would refund a submit that REACHED the endpoint. The cap then never "+
			"decrements and unlimited real moderator-review requests are permitted.\n%s", yesResult)
	}

	// (4) The seam closed end to end: the CLI's real bytes, driven through the real
	// harness. No string in this arm is typed by this file on the runner's side.
	credPath, _ := credentialFile(t)
	const bare = "civitai app submit"
	const withYes = "civitai app submit --yes"
	body := strings.TrimPrefix(bareResult, fmt.Sprintf("exit code: %d\n", bareCode))
	tr := runFakeTrialEnv(t,
		[]string{plantedResults(t, map[string]plantedResult{
			bare: {RC: bareCode, Out: body},
		})},
		[]string{bare, withYes}, "",
		"--credential-file", credPath, "--app-prefix", "seam-", "--max-submissions", "1")
	got := stepVerdicts(t, readFile(t, tr.transcript))
	want := []string{"run", "run"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("replaying the CLI's REAL refusal through the harness gave verdicts %v, want %v.\n%s",
			got, want, readFile(t, tr.transcript))
	}
}

// ── seam guard 2: --package-only still means "contact nothing" ────────────────

// 🔴 THE EXEMPTION'S JUSTIFICATION, MEASURED. `--package-only` is exempt from the
// submission cap because it cannot reach the API. That is a fact about
// app_submit.go (`canUpload := !packageOnly && …`), and if it ever stops being
// true the exemption becomes an uncapped hole through which a trial can submit.
func TestDogfoodPackageOnlyStillContactsNothing(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"publishRequestId":"pubreq_seam","slug":"seam-block","version":"0.1.0","status":"pending"}`))
	}))
	defer srv.Close()
	hitCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return hits
	}

	dir := seamAppDir(t)
	zip := filepath.Join(dir, "bundle.zip")
	result, code := runSeamCLI(t, srv.URL, "app", "submit", dir, "--package-only", "--out", zip)
	if code != 0 {
		t.Fatalf("`app submit --package-only` failed, so its \"writes a zip, contacts nothing\" "+
			"meaning cannot be measured here:\n%s", result)
	}
	if n := hitCount(); n != 0 {
		t.Fatalf("`--package-only` reached the endpoint %d time(s). It is EXEMPT from the dogfood "+
			"submission cap on the grounds that it cannot — so this is an uncapped path to a real "+
			"submission.\n  fix: drop SUBMIT_NO_CONTACT_FLAG's exemption in scripts/dogfood/runner.py, "+
			"or restore the guarantee.\n%s", n, result)
	}
	if _, err := os.Stat(zip); err != nil {
		t.Errorf("`--package-only` wrote no bundle at %s (%v) — if it no longer packages, the "+
			"exemption is buying the trial nothing and should go.\n%s", zip, err, result)
	}

	// POSITIVE CONTROL for this test's own counter, so the zero above is a
	// measurement rather than a fact about the server never being reachable.
	yesResult, yesCode := runSeamCLI(t, srv.URL, "app", "submit", seamAppDir(t), "--yes")
	if yesCode != 0 || hitCount() < 1 {
		t.Fatalf("CONTROL failure, not a finding: `--yes` against the same recorder exited %d and "+
			"produced %d hit(s), so this test's hit counter is not wired to the submit path.\n%s",
			yesCode, hitCount(), yesResult)
	}

	// And the flag the exemption is keyed on must still be the flag the CLI
	// declares. A rename would otherwise turn the exemption into dead code —
	// silently, and in the over-refusing direction this file is here to notice.
	name := runnerConstant(t, "SUBMIT_NO_CONTACT_FLAG")
	src, err := os.ReadFile(filepath.Join("internal", "cmd", "app_submit.go"))
	if err != nil {
		t.Fatalf("reading app_submit.go: %v", err)
	}
	decl := regexp.MustCompile(`BoolVarP?\(\s*&packageOnly\s*,\s*"([^"]*)"`)
	m := decl.FindSubmatch(src)
	if m == nil {
		t.Fatalf("CONTROL failure, not a finding: no `BoolVar(&packageOnly, \"…\")` declaration found "+
			"in internal/cmd/app_submit.go, so the comparison below would be against nothing. The "+
			"binding was renamed — re-derive this guard.")
	}
	if got := string(m[1]); got != name {
		t.Errorf("runner.py exempts `--%s` from the submission cap and the CLI declares the "+
			"package-only flag as `--%s`. The exemption is dead code and every --%s invocation is "+
			"charged (and capped) again.", name, got, got)
	}
}

// runnerConstant dumps one module-level constant out of runner.py.
func runnerConstant(t *testing.T, name string) string {
	t.Helper()
	py := dogfoodPython(t)
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("r", sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
print(json.dumps(getattr(m, sys.argv[2])))
`
	cmd := exec.Command(py, "-c", script, filepath.Join(dogfoodDir, "runner.py"), name)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("reading %s out of runner.py: %v", name, err)
	}
	var got string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%s did not dump as a string: %v\n%s", name, err, out)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("CONTROL failure, not a finding: runner.py's %s is empty, so anything compared "+
			"against it passes vacuously", name)
	}
	return got
}

// ── seam guard 3: this repo's half — the trial's stdin is never a terminal ────

// 🔴 THE REFUND'S UNSTATED DEPENDENCY. `confirmSubmit` refuses only when stdin is
// NOT a TTY; on a terminal it prompts instead, and a prompt with no answer would
// produce a different result entirely. The harness satisfies that through
// `docker exec -i` — a pipe. If `-i` were dropped, or `-t` added, the refusal the
// refund matches would stop being what a trial sees.
//
// Measured off the argv the runner actually issued, not off a grep of its source:
// the offline fixture records every `subprocess.run` argv.
func TestDogfoodRunnerRunsEveryCommandOnANonTTYStdin(t *testing.T) {
	const cmd = "civitai app submit --package-only"
	tr := runFakeTrial(t, []string{cmd}, "")

	var cap struct {
		Subprocess [][]string `json:"subprocess"`
	}
	if err := json.Unmarshal([]byte(readFile(t, tr.capture)), &cap); err != nil {
		t.Fatalf("capture.json is not readable: %v", err)
	}
	found := 0
	for _, argv := range cap.Subprocess {
		if len(argv) == 0 || argv[len(argv)-1] != cmd {
			continue
		}
		found++
		has := func(flag string) bool {
			for _, a := range argv {
				if a == flag {
					return true
				}
			}
			return false
		}
		if !has("-i") {
			t.Errorf("the model's command ran WITHOUT `docker exec -i`, so its stdin may be a "+
				"terminal. `confirmSubmit` then PROMPTS instead of refusing, and the submission "+
				"cap's refund — which matches the refusal text — silently stops working: argv %v", argv)
		}
		if has("-t") || has("-it") {
			t.Errorf("the model's command ran with a TTY allocated (`-t`), which is the one "+
				"condition under which `civitai app submit` prompts rather than refusing: argv %v", argv)
		}
	}
	// POSITIVE CONTROL: a zero here would pass every assertion above without
	// having looked at anything.
	if found != 1 {
		t.Fatalf("CONTROL failure, not a finding: found %d docker argv/argvs whose last element is "+
			"the model's command, want 1 — the assertions above inspected nothing.\n%s",
			found, readFile(t, tr.capture))
	}
}
