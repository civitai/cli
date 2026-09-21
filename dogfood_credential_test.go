package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The CREDENTIALED-TRIAL path of the blind dogfood harness (scripts/dogfood).
//
// 🔴 THE DEFECT THESE EXIST FOR. `--agent-env VAR=VALUE` was the only way to get
// a variable into a trial container, and runner.py wrote its value verbatim into
// the `start` record of runs/<trial>/transcript.jsonl. Using it to carry a token
// therefore wrote a live account credential, in plaintext, into a file that
// persists on disk and is read and quoted by humans and agents afterwards.
// `--credential-file` replaces that path, and these tests are what make
// "nothing secret leaks" a MEASUREMENT rather than a claim.
//
// Everything runs offline through testdata/fake_trial.py: no Docker, no
// OpenRouter, no money, no account.

// A high-entropy string that cannot occur by accident in any output and is not
// a word anything else prints. If it appears anywhere in the artifacts a
// credentialed run leaves behind, the redaction is broken.
const plantedSecret = "civitai-dogfood-LEAK-CANARY-7f3a91c04be6d258"

func credentialFile(t *testing.T) (path, body string) {
	t.Helper()
	body = "access_token: " + plantedSecret + "\n" +
		"refresh_token: rt-" + plantedSecret + "\n" +
		"scope: read write\nauth_kind: oauth\nbase_url: https://civitai.com\n"
	path = filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, body
}

// Runs one offline trial. `extra` are runner.py flags; `commands` are served to
// the runner one per assistant turn.
type fakeTrial struct {
	dir        string
	transcript string
	commandLog string
	capture    string
	stdout     string
}

func runFakeTrial(t *testing.T, commands []string, toolOutput string, extra ...string) fakeTrial {
	t.Helper()
	return runFakeTrialEnv(t, nil, commands, toolOutput, extra...)
}

// The same offline trial, with extra environment handed to fake_trial.py — the
// knobs that shape what the STUB PROVIDER returns (finish_reason, an empty or
// null content, reasoning blocks, a whole recorded response body). Kept
// separate so the common case stays a three-argument call.
func runFakeTrialEnv(t *testing.T, env []string, commands []string, toolOutput string, extra ...string) fakeTrial {
	t.Helper()
	py := dogfoodPython(t)
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.json")
	args := []string{
		filepath.Join(dogfoodDir, "testdata", "fake_trial.py"),
		filepath.Join(dogfoodDir, "runner.py"),
		filepath.Join(dir, "runs"), capture, "",
	}
	if len(extra) > 0 {
		args = append(append(args, "--"), extra...)
	}
	cmd := exec.Command(py, args...)
	// 🔴 fake_trial.py IMPORTS runner.py, and an import consults __pycache__.
	// CPython validates that cache on the source's mtime in whole SECONDS plus
	// its size, so a same-length edit landing in the same second as the last
	// import runs the ORIGINAL bytecode. Nothing here may be cached.
	cmd.Env = append(os.Environ(),
		"PYTHONDONTWRITEBYTECODE=1",
		"FAKE_TOOL_COMMAND="+strings.Join(commands, "\n"),
		"FAKE_TOOL_OUTPUT="+toolOutput)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake trial failed: %v\n%s", err, out)
	}
	return fakeTrial{
		dir:        dir,
		transcript: filepath.Join(dir, "runs", "faketrial", "transcript.jsonl"),
		commandLog: filepath.Join(dir, "runs", "faketrial", "commands.log"),
		capture:    capture,
		stdout:     string(out),
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading %s: %v", p, err)
	}
	return string(b)
}

// The one grep every arm of the leak test uses — the SAME function for the
// artifacts and for the positive control, so "it found nothing" and "it can
// find something" are claims about one instrument.
func countHits(haystack, needle string) int { return strings.Count(haystack, needle) }

// ── the leak test ────────────────────────────────────────────────────────────

// 🔴 THE TEST THIS WHOLE FLAG EXISTS TO PASS. A credentialed trial runs end to
// end, the model `cat`s the credential file (the most obvious way a token
// reaches an artifact), and every surface a secret could land on is grepped for
// the planted value: the transcript, the flat command log, the process argv of
// every subprocess the runner spawned, and the runner's own stdout.
//
// 🔴 WITH A POSITIVE CONTROL, because a grep that finds nothing because the
// PATTERN is wrong is indistinguishable from one that finds nothing because
// there is nothing there — and that is the exact failure mode of a leak test.
// The control feeds the same countHits the un-redacted credential file and
// requires a non-zero count. Report the pair, never the zero alone.
func TestDogfoodCredentialNeverLeaks(t *testing.T) {
	credPath, credBody := credentialFile(t)

	// POSITIVE CONTROL FIRST. If this is zero, every assertion below passes
	// vacuously and the test is measuring its own pattern.
	if n := countHits(credBody, plantedSecret); n < 2 {
		t.Fatalf("positive control: countHits found %d occurrences of the planted secret in the "+
			"un-redacted credential file, want >= 2. The pattern is broken, so every zero "+
			"below would be meaningless.", n)
	}

	tr := runFakeTrial(t, []string{
		"cat ~/.config/civitai/config.yaml",
		"echo \"token=$CIVITAI_TOKEN\"",
	}, credBody, "--credential-file", credPath)

	// The argv of every subprocess the runner spawned — this is what `ps` would
	// have shown on a real run.
	var cap struct {
		Subprocess [][]string `json:"subprocess"`
	}
	if err := json.Unmarshal([]byte(readFile(t, tr.capture)), &cap); err != nil {
		t.Fatal(err)
	}
	if len(cap.Subprocess) == 0 {
		t.Fatal("the capture recorded no subprocess argv — this test could not have caught an " +
			"argv leak either")
	}
	argvBlob, err := json.Marshal(cap.Subprocess)
	if err != nil {
		t.Fatal(err)
	}

	for _, s := range []struct{ what, body string }{
		{"transcript.jsonl", readFile(t, tr.transcript)},
		{"commands.log", readFile(t, tr.commandLog)},
		{"process argv", string(argvBlob)},
		{"runner stdout", tr.stdout},
	} {
		if n := countHits(s.body, plantedSecret); n != 0 {
			t.Errorf("the credential leaked into %s: %d occurrence(s) of the planted secret.\n%s",
				s.what, n, s.body)
		}
	}

	// NEGATIVE CONTROL ON THE FIXTURE: the trial must actually have produced
	// output containing the secret, or the redaction was never exercised. The
	// transcript has to show the redaction marker where the `cat` output went.
	transcript := readFile(t, tr.transcript)
	if !strings.Contains(transcript, "[REDACTED:") {
		t.Fatalf("no redaction marker in the transcript — the `cat` output never carried the "+
			"credential, so the zero above is about a path that was never walked:\n%s", transcript)
	}
}

// The marker that replaces the value must identify the run without revealing
// any of it. A sha256 prefix is not reversible and is not a substring.
func TestDogfoodCredentialMarkerIsNotDerivedFromTheValue(t *testing.T) {
	credPath, credBody := credentialFile(t)
	tr := runFakeTrial(t, nil, "", "--credential-file", credPath)

	var start struct {
		Kind         string `json:"kind"`
		Credentialed bool   `json:"credentialed"`
		Sha          string `json:"credential_sha256"`
	}
	for _, line := range strings.Split(strings.TrimSpace(readFile(t, tr.transcript)), "\n") {
		var r struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("transcript line is not JSON: %q", line)
		}
		if r.Kind == "start" {
			if err := json.Unmarshal([]byte(line), &start); err != nil {
				t.Fatal(err)
			}
		}
	}
	if start.Kind != "start" {
		t.Fatalf("no start record:\n%s", readFile(t, tr.transcript))
	}
	if !start.Credentialed {
		t.Fatal("a credentialed run did not record `credentialed: true` — a reader cannot tell " +
			"which trials reached an account")
	}
	if len(start.Sha) < 8 {
		t.Fatalf("credential_sha256 = %q, want a usable prefix so two runs can be told apart", start.Sha)
	}
	// 🔴 The marker must not be a slice of the secret. Both directions, because
	// a naive "first 12 chars of the token" satisfies one of them.
	if strings.Contains(credBody, start.Sha) {
		t.Fatalf("credential_sha256 %q is a SUBSTRING of the credential file — it is a piece of "+
			"the secret, not a digest of it", start.Sha)
	}
	if strings.Contains(start.Sha, plantedSecret[:8]) {
		t.Fatalf("credential_sha256 %q carries the secret's own leading bytes", start.Sha)
	}
}

// The credential reaches the container by PATH, never by value: a `docker cp`
// of the file, then a fixed installer script run as root. This pins the
// mechanism, so a later "simplification" to `docker run -e CIVITAI_TOKEN=…` —
// which would put the value in argv for every `ps` on the host — fails here
// rather than in a leak nobody re-greps.
func TestDogfoodCredentialIsInjectedByPathNotByValue(t *testing.T) {
	credPath, _ := credentialFile(t)
	tr := runFakeTrial(t, nil, "", "--credential-file", credPath)

	var cap struct {
		Subprocess [][]string `json:"subprocess"`
	}
	if err := json.Unmarshal([]byte(readFile(t, tr.capture)), &cap); err != nil {
		t.Fatal(err)
	}
	var sawCp, sawInstall bool
	for _, argv := range cap.Subprocess {
		joined := strings.Join(argv, " ")
		if len(argv) > 1 && argv[1] == "cp" && strings.Contains(joined, credPath) {
			sawCp = true
		}
		if strings.Contains(joined, "exec") && strings.Contains(joined, "config/civitai") {
			sawInstall = true
		}
		// The `docker run` that creates the container must not have gained an
		// `-e` carrying the credential path or anything else new.
		if len(argv) > 1 && argv[1] == "run" && strings.Contains(joined, credPath) {
			t.Fatalf("the credential path reached `docker run`'s argv: %v", argv)
		}
	}
	if !sawCp {
		t.Fatalf("no `docker cp` of the credential file — how did it get in?\n%v", cap.Subprocess)
	}
	if !sawInstall {
		t.Fatalf("no installer exec placing it under ~/.config/civitai\n%v", cap.Subprocess)
	}
}

// ── the in-container installer ───────────────────────────────────────────────

// The installer script, read out of runner.py by importing it — never by
// regexing the source, which would pass on a file whose Python no longer parses.
func installScript(t *testing.T) string {
	t.Helper()
	py := dogfoodPython(t)
	out, err := exec.Command(py, "-c",
		"import importlib.util,sys;"+
			"spec=importlib.util.spec_from_file_location('r', sys.argv[1]);"+
			"m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m);"+
			"sys.stdout.write(m.INSTALL_SH)",
		filepath.Join(dogfoodDir, "runner.py")).Output()
	if err != nil {
		t.Fatalf("reading INSTALL_SH out of runner.py: %v", err)
	}
	if !strings.Contains(string(out), "config/civitai") {
		t.Fatalf("that is not the installer script: %q", out)
	}
	return string(out)
}

// 🔴 THE SCRIPT RUNS AS ROOT, SO `$HOME` IS ROOT'S HOME AND NOT THE TRIAL
// USER'S. Falling back to it would install the credential where the trial cannot
// read it, and the trial would then grade as an ordinary "not authenticated"
// failure — the capability confound, arriving through the setup step. It must
// resolve the named user's home or REFUSE.
//
// Run on the host with a stubbed `getent`, against a staging path substituted
// into the script, so nothing touches the operator's real ~/.config/civitai.
func TestDogfoodCredentialInstallerResolvesTheUsersHome(t *testing.T) {
	sh := dogfoodTool(t, "sh")
	raw := installScript(t)
	dir := t.TempDir()
	stage := filepath.Join(dir, "staged-credential")
	if err := os.WriteFile(stage, []byte("token: planted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The only edit is the staging PATH constant — the logic under test is
	// byte-for-byte what runner.py ships.
	script := strings.ReplaceAll(raw, "/tmp/.dogfood-credential", stage)
	if script == raw {
		t.Fatal("the staging path constant moved; this test would have run against the real /tmp path")
	}

	home := filepath.Join(dir, "home")
	stub := filepath.Join(dir, "bin")
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Answers for `dev`, and for nobody else — so the refusal arm is a real
	// lookup failure rather than a missing binary.
	getent := "#!/bin/sh\n[ \"$2\" = dev ] || exit 2\necho \"dev:x:1000:1000::" + home + ":/bin/sh\"\n"
	if err := os.WriteFile(filepath.Join(stub, "getent"), []byte(getent), 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(user string) (string, int) {
		cmd := exec.Command(sh, "-c", script, "sh", user)
		cmd.Env = append(os.Environ(), "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("running the installer: %v\n%s", err, out)
		}
		return string(out), code
	}

	// Arm 1: the user resolves. The credential lands under THAT home, the
	// staging copy is gone, and the mode is tight.
	out, code := run("dev")
	if code != 0 {
		t.Fatalf("installer exited %d for a resolvable user, want 0\n%s", code, out)
	}
	dest := filepath.Join(home, ".config", "civitai", "config.yaml")
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("the credential was not installed at %s: %v\n%s", dest, err, out)
	}
	if string(body) != "token: planted\n" {
		t.Fatalf("installed content is %q, want the staged bytes", body)
	}
	if fi, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	} else if fi.Mode().Perm() != 0o600 {
		t.Fatalf("installed mode is %v, want 0600 — the trial's own shell is not the only reader "+
			"of a container filesystem", fi.Mode().Perm())
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("the staging copy at %s survived the install (err=%v) — a second copy of the "+
			"credential is left where anything in the container can read it", stage, err)
	}

	// Arm 2: the user does not resolve. It must REFUSE and say so — under
	// `set -e` the obvious `[ … ] || { … && … ; }` fallback chain exits silently
	// at the first false branch, which is a failure with no diagnosis.
	if err := os.WriteFile(stage, []byte("token: planted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = run("nobody-who-has-no-passwd-entry")
	if code == 0 {
		t.Fatalf("installer accepted an unresolvable user:\n%s", out)
	}
	if !strings.Contains(out, "cannot resolve a home directory") {
		t.Fatalf("the refusal does not say what went wrong (exit %d):\n%s", code, out)
	}
}

// ── the uncredentialed default ───────────────────────────────────────────────

// 🔴 BYTE-IDENTICAL BY DEFAULT, PROVED RATHER THAN ASSERTED. Every setup grid
// already measured was produced by a runner whose `start` record carried exactly
// these keys and whose `end` record carried exactly those. A credential path
// that quietly added a field to every run would re-base the grid the same way a
// changed task would — which is the hazard TestDogfoodTaskDefaultIsByteIdentical
// exists for, one layer down.
//
// ⚠ THIS IS AN INVARIANT GUARD, NOT REGRESSION COVERAGE. It PASSES on
// origin/main — it has to, because what it pins is that nothing changed. It was
// watched to go red the only way it can: by mutating runner.py to emit the caps
// record unconditionally, which is the defect it exists to catch. Do not count
// it in a red-at-base matrix.
func TestDogfoodUncredentialedRunIsUnchanged(t *testing.T) {
	tr := runFakeTrial(t, []string{"echo hello"}, "")

	want := map[string][]string{
		// `brief_name` joined the `start` record with the oracle's brief-derivation
		// fix, on exactly the contract `brief` already had: emitted
		// UNCONDITIONALLY, empty string included, so "this run named no brief" is
		// a POSITIVE assertion rather than an absence indistinguishable from an
		// older runner's transcript. Additive and never read by the model, so no
		// already-measured grid's TASK moved — that claim lives in
		// TestDogfoodTaskDefaultIsByteIdentical, which is untouched.
		"start": {"agent_env", "brief", "brief_name", "container", "image", "kind", "model", "t", "trial", "user"},
		// `finish_reason` joined the `end` record deliberately, and it is the
		// ONE key added since the setup grid was measured. It is additive and
		// present on every `end` record, so a `jq` over a directory of old and
		// new transcripts needs no branch; the grid's own comparability lives
		// in the TASK (TestDogfoodTaskDefaultIsByteIdentical) and in
		// `end.usage`, neither of which moved. The `stop` VOCABULARY did widen
		// — see TestDogfoodTerminalStatesAreDistinguishable — which is the
		// point of the change, not a side effect of it.
		"end": {"final", "finish_reason", "kind", "steps", "stop", "t", "usage"},
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(readFile(t, tr.transcript)), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("transcript line is not JSON: %q", line)
		}
		kind, _ := r["kind"].(string)
		if kind == "caps" || kind == "refused" {
			t.Fatalf("an uncredentialed run emitted a %q record — the caps armed themselves:\n%s",
				kind, readFile(t, tr.transcript))
		}
		w, ok := want[kind]
		if !ok {
			continue
		}
		seen[kind] = true
		var got []string
		for k := range r {
			got = append(got, k)
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(w, ",") {
			t.Fatalf("the %q record's key set changed.\n got: %v\nwant: %v\n\nEvery already-measured "+
				"grid was produced by a runner emitting the `want` keys.", kind, got, w)
		}
	}
	for k := range want {
		if !seen[k] {
			t.Fatalf("no %q record in:\n%s", k, readFile(t, tr.transcript))
		}
	}
}

// ── the run caps ─────────────────────────────────────────────────────────────

// Reads the transcript's per-step verdicts in order: "run" for a command that
// reached the container, "refused" for one that did not.
func stepVerdicts(t *testing.T, transcript string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(transcript), "\n") {
		var r struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("transcript line is not JSON: %q", line)
		}
		switch r.Kind {
		case "tool":
			out = append(out, "run")
		case "refused":
			out = append(out, "refused")
		}
	}
	return out
}

// 🔴 A CAPPED COMMAND IS NEVER EXECUTED. The counter lives in the runner
// process, the judgement happens before the `docker exec`, and the model is
// handed the refusal INSTEAD of a result — so the cap is a property of this
// process and not of anything the trial can reach. A "cap" that ran the command
// and then recorded a complaint would satisfy a test keyed only on the refusal
// string; this one is keyed on the absence of a `tool` record.
func TestDogfoodCapsRefuseBeforeTheCommandRuns(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, tc := range []struct {
		name    string
		command string
		want    string
		says    string
	}{
		{"generate past the cap", `civitai generate "a cat"`, "refused", "generation cap"},
		{"submit past the cap", "civitai app submit --yes", "refused", "submission cap"},
		{"withdraw at all", "civitai app withdraw pubreq_01ABC", "refused", "withdraw"},
		{"a foreign slug", "civitai app listing set-icon ./i.png --slug sensei", "refused", "dogfood4-"},
		{"an unreadable invocation", `eval "civitai app submit"`, "refused", "not a form the harness can read"},
		{"an ordinary command", "npm install", "run", ""},
		{"looking the CLI up", "command -v civitai", "run", ""},
		{"a prefixed app", "civitai app init dogfood4-thing", "run", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrial(t, []string{tc.command}, "",
				"--credential-file", credPath, "--app-prefix", "dogfood4-",
				"--max-generations", "0", "--max-submissions", "0")
			got := stepVerdicts(t, readFile(t, tr.transcript))
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("%q: verdicts %v, want [%s]\n%s",
					tc.command, got, tc.want, readFile(t, tr.transcript))
			}
			if tc.says != "" && !strings.Contains(readFile(t, tr.transcript), tc.says) {
				t.Fatalf("the refusal does not say why (%q missing):\n%s",
					tc.says, readFile(t, tr.transcript))
			}
		})
	}
}

// The cap is a COUNT, not a switch: N through, the N+1th refused. A cap wired as
// "refuse whenever the flag is set" passes every case in the table above and
// fails here.
func TestDogfoodCapsAllowUpToTheLimitThenRefuse(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, tc := range []struct {
		name    string
		flag    string
		command string
	}{
		{"generations", "--max-generations", `civitai generate "a cat"`},
		{"submissions", "--max-submissions", "civitai app submit --yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrial(t, []string{tc.command, tc.command, tc.command}, "",
				"--credential-file", credPath, "--app-prefix", "dogfood4-", tc.flag, "2")
			got := stepVerdicts(t, readFile(t, tr.transcript))
			want := []string{"run", "run", "refused"}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("with %s 2 the verdicts were %v, want %v\n%s",
					tc.flag, got, want, readFile(t, tr.transcript))
			}
		})
	}
}

// 🔴 THE COMMAND LOG IS THE THIRD CAP, AND THE ONLY ONE THAT IS PURELY
// MECHANICAL. It is a flat, greppable list of what actually ran with the
// counters beside each line — what an operator reads after a credentialed run to
// answer "what did it do", without paging a transcript that grows with token
// usage. Every command appears, refused ones included.
func TestDogfoodCommandLogRecordsEveryCommandAndItsVerdict(t *testing.T) {
	credPath, _ := credentialFile(t)
	tr := runFakeTrial(t, []string{"npm install", `civitai generate "a cat"`}, "",
		"--credential-file", credPath, "--app-prefix", "dogfood4-", "--max-generations", "0")
	log := readFile(t, tr.commandLog)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 {
		t.Fatalf("commands.log has %d line(s), want 2 (one per command, refused included):\n%s",
			len(lines), log)
	}
	if !strings.Contains(lines[0], "verdict=run") || !strings.Contains(lines[0], "npm install") {
		t.Fatalf("line 1 is not the executed command: %q", lines[0])
	}
	if !strings.Contains(lines[1], "verdict=refused") || !strings.Contains(lines[1], "civitai generate") {
		t.Fatalf("line 2 is not the refused command: %q", lines[1])
	}
	// The counters travel with the line, so a reader does not have to replay the
	// log to know where the run stood.
	if !strings.Contains(lines[0], "gen=") || !strings.Contains(lines[0], "sub=") {
		t.Fatalf("the log line carries no cap counters: %q", lines[0])
	}
}

// An uncredentialed, uncapped run still gets a command log — it is evidence, not
// enforcement — but it must not gain cap counters that were never armed.
func TestDogfoodCommandLogExistsWithoutACredential(t *testing.T) {
	tr := runFakeTrial(t, []string{"echo hello"}, "")
	if !strings.Contains(readFile(t, tr.commandLog), "echo hello") {
		t.Fatalf("no command log for an uncredentialed run:\n%s", readFile(t, tr.commandLog))
	}
}

// A path that does not exist, or one that exists and is empty, must fail BEFORE
// a container is created — a trial that reaches the model without the credential
// it was supposed to carry grades as an ordinary failure and burns a cell.
func TestDogfoodRefusesAnUnusableCredentialFile(t *testing.T) {
	py := dogfoodPython(t)
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 🔴 The `says` strings name the SPECIFIC problem, never just the flag. An
	// earlier draft asserted only that the output mentioned `credential-file`,
	// which argparse's own `unrecognized arguments: --credential-file …` also
	// satisfies — so the test passed on a build that had never heard of the flag.
	for _, tc := range []struct{ name, path, says string }{
		{"missing", filepath.Join(dir, "no-such-file.yaml"), "No such file"},
		{"empty", empty, "empty file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(py,
				filepath.Join(dogfoodDir, "runner.py"),
				"--model", "x", "--image", "x", "--trial", "x",
				"--out", t.TempDir(), "--credential-file", tc.path).CombinedOutput()
			if err == nil {
				t.Fatalf("a %s credential file was accepted:\n%s", tc.name, out)
			}
			if !strings.Contains(string(out), tc.says) {
				t.Fatalf("the refusal does not name the problem (%q):\n%s", tc.says, out)
			}
		})
	}
}
