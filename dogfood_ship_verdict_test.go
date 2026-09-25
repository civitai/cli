package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The SHIP VERDICT of the blind dogfood harness (scripts/dogfood/ship.verdict.sh).
//
// `oracle.sh` grades the DOM. A submission is SERVER state — it leaves nothing
// in the DOM at all — so the ship half is a different instrument reading a
// different surface: `civitai app status --json`, run inside the trial's own
// container with the trial's own credential.
//
// 🔴 THE ONE PROPERTY THESE TESTS EXIST FOR IS THE IDENTITY CHECK. The account a
// credentialed trial authenticates as owns roughly a dozen REAL published apps,
// every one of which has a submission row. A verdict that asked only "is there a
// pending submission?" would grade the operator's back catalogue as this trial's
// success — a negative control passing for the wrong reason, which is the exact
// shape that has already cost this arc two wrong headlines (the brief defaulting
// to `celsius`; the control that passed off a shell metacharacter). So the
// negative controls below are not ceremony: each one holds every conjunct but
// ONE, and a verdict that still says `yes` has stopped measuring the thing.
//
// Docker is STUBBED (stubDockerScript, shared with dogfood_oracle_test.go) and
// so is the CLI, so these need no daemon, no network, no credential and no
// account. Everything else — the real script, the real jq, the real date
// arithmetic — executes.

const shipDir = "scripts/dogfood"

// stubCivitaiScript answers exactly the one read ship.verdict.sh makes.
//
// 🔴 IT MUST BE ABLE TO FAIL. `STUB_STATUS_RC` is the negative control for the
// instrument itself: a stub that always succeeds could not tell a script that
// distinguishes "the call failed" from "no such submission" from one that folds
// them together, which is the distinction the whole exit-2 split rests on.
const stubCivitaiScript = `#!/bin/sh
if [ "${1:-}" = "app" ] && [ "${2:-}" = "status" ]; then
  rc="${STUB_STATUS_RC:-0}"
  if [ "$rc" != "0" ]; then
    echo "stub civitai: the API refused" >&2
    exit "$rc"
  fi
  # The real command writes its API-cap note to stderr so --json stdout stays a
  # pure payload; the stub does the same, so a script merging the streams breaks
  # here rather than in production.
  echo "note: the server returned the newest N submissions" >&2
  cat "$STUB_STATUS_FILE"
  exit 0
fi
exit 0
`

type shipSub struct {
	ID          string `json:"id"`
	BlockID     string `json:"blockId"`
	Status      string `json:"status"`
	SubmittedAt string `json:"submittedAt"`
}

// The operator's real back catalogue, in miniature. Present in EVERY case below
// (including the passing one), because a listing that held only the trial's own
// row would let a verdict with no identity check pass every test here.
func shipPreExisting(now time.Time) []shipSub {
	return []shipSub{
		{ID: "pubreq_old_a", BlockID: "panorama-360", Status: "approved",
			SubmittedAt: now.Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)},
		{ID: "pubreq_old_b", BlockID: "custom-generators", Status: "pending",
			SubmittedAt: now.Add(-9 * 24 * time.Hour).UTC().Format(time.RFC3339)},
	}
}

type shipEnv struct {
	state      string // "running", "exited", "" (absent)
	manifests  map[string]string
	noCivitai  bool
	statusRC   string // non-"0" => the status call fails
	statusBody string // raw stdout of `civitai app status --json`
	transcript string // "" => no transcript at all
}

// shipStubEnv builds the stub PATH, the fixture /work tree and the runs dir.
//
// 🔴 NOT t.TempDir(), for the reason recorded in stubOracleEnv: t.TempDir()
// builds its path out of the TEST NAME, the stub docker substitutes that path
// into the script's `find /work …` command text, and a subtest name carrying a
// shell metacharacter turns every in-"container" command into a syntax error —
// which yields exactly the empty output a real negative case yields, so the
// control passes for entirely the wrong reason.
func shipStubEnv(t *testing.T, s shipEnv) []string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dogfood-ship-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	stub := filepath.Join(dir, "bin")
	work := filepath.Join(dir, "work")
	tmp := filepath.Join(dir, "tmp")
	runs := filepath.Join(dir, "runs")
	for _, d := range []string{stub, work, tmp, runs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, body string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(stub, "docker"), stubDockerScript, 0o755)
	if !s.noCivitai {
		write(filepath.Join(stub, "civitai"), stubCivitaiScript, 0o755)
	}
	for rel, body := range s.manifests {
		write(filepath.Join(work, rel, "block.manifest.json"), body, 0o644)
	}
	statusFile := filepath.Join(dir, "status.json")
	write(statusFile, s.statusBody, 0o644)
	if s.transcript != "" {
		write(filepath.Join(runs, "ctl", "transcript.jsonl"), s.transcript, 0o644)
	}
	rc := s.statusRC
	if rc == "" {
		rc = "0"
	}
	// 🔴 `STUB_CIVITAI=0` IS WHAT MAKES "the container has no CLI" A REAL ARM.
	// Omitting the stub script is NOT enough: the stub docker keeps the host PATH
	// behind it (it needs the real find/cat/jq), so `command -v civitai` would
	// resolve the OPERATOR'S INSTALLED CLI and the arm would run a live,
	// credentialed read against their account. Measured: it did exactly that, and
	// came back with 100 real submissions.
	civitaiPresent := "1"
	if s.noCivitai {
		civitaiPresent = "0"
	}
	return []string{
		// The stub dir goes FIRST but the rest of PATH is kept: the stub docker
		// rewrites container paths and then needs the real find/cat/jq/date.
		"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STUB_STATE=" + s.state,
		"STUB_ROOT=" + work,
		"STUB_TMP=" + tmp,
		"STUB_NODE=1",
		"STUB_CIVITAI=" + civitaiPresent,
		"STUB_STATUS_RC=" + rc,
		"STUB_STATUS_FILE=" + statusFile,
		"DOGFOOD_RUNS=" + runs,
		// Never inherit an operator's shell default — it would silently widen
		// the window in every case below.
		"DOGFOOD_SHIP_GRACE_S=",
	}
}

// A transcript shaped like runner.py's: a `start` record carrying the run's
// wall-clock start and an `end` record carrying its finish. Those two floats ARE
// the run window.
func shipTranscript(start, end time.Time, briefName string) string {
	s, _ := json.Marshal(map[string]any{
		"t": float64(start.UnixNano()) / 1e9, "kind": "start", "trial": "ctl",
		"model": "fake/model", "image": "fake-image", "user": "root", "agent_env": "",
		"brief": "…", "brief_name": briefName, "container": "dogfood-ctl",
		"credentialed": true, "credential_sha256": "deadbeefcafe",
	})
	e, _ := json.Marshal(map[string]any{
		"t": float64(end.UnixNano()) / 1e9, "kind": "end", "stop": "finished",
	})
	return string(s) + "\n" + string(e) + "\n"
}

func shipStatusBody(t *testing.T, subs []shipSub) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"submissions": subs})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const fxShipManifest = `{"blockId":"ab-ship-01","version":"0.1.0","name":"Ship","type":"block",
"scopes":["ai:write:budgeted","posts:write:self"],"buildCommand":"npm run build","outputDir":"dist"}`

// Reads a `key=value` off the verdict's single-line summary. The line is found
// by its `ship_trial=` prefix, which nothing else in the output starts with.
func shipField(t *testing.T, out, key string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "ship_trial=") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, key+"=") {
				return strings.TrimPrefix(f, key+"=")
			}
		}
	}
	t.Fatalf("no %q field on any summary line of:\n%s", key, out)
	return ""
}

// The standard fixture: a finished trial whose window is [now-1h, now-30m].
type shipWindow struct{ start, end, mid time.Time }

func shipStdWindow() shipWindow {
	now := time.Now()
	start := now.Add(-1 * time.Hour)
	end := now.Add(-30 * time.Minute)
	return shipWindow{start: start, end: end, mid: now.Add(-45 * time.Minute)}
}

func runShipVerdict(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	return runScript(t, "ship.verdict.sh", env, args...)
}

// ── nothing-was-measured: exit 2, and never a verdict ────────────────────────

// 🔴 THE SAME SINGLE MOST IMPORTANT PROPERTY oracle.sh HAS. Every case here is
// reachable on an ordinary credentialed run: a typo'd trial id, a daemon
// restart, a trial whose npm install never finished, an expired token, a
// Cloudflare 5xx. Folding any of them into `SHIP=no` would report "the model did
// not submit" about a run in which the account was never read — the capability
// confound, arriving through the grader.
func TestShipVerdictRefusesWhenNothingCanBeMeasured(t *testing.T) {
	w := shipStdWindow()
	ok := shipStatusBody(t, shipPreExisting(time.Now()))
	tr := shipTranscript(w.start, w.end, "ship")
	for _, tc := range []struct {
		name    string
		env     shipEnv
		wantMsg string
	}{
		{
			name:    "no transcript, so no run window",
			env:     shipEnv{state: "running", statusBody: ok, manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "no transcript",
		},
		{
			name: "a transcript with no start record",
			env: shipEnv{state: "running", statusBody: ok, transcript: "{\"t\":2.0,\"kind\":\"end\",\"stop\":\"finished\"}\n",
				manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "no `start` record",
		},
		{
			name:    "no such container",
			env:     shipEnv{state: "", statusBody: ok, transcript: tr, manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "no such container",
		},
		{
			name:    "container not running",
			env:     shipEnv{state: "exited", statusBody: ok, transcript: tr, manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "not running",
		},
		{
			// No manifest => no blockId => the identity conjunct has no
			// left-hand side. `no` here would be a verdict computed without the
			// only field that makes the verdict mean anything.
			name:    "no manifest, so no blockId to attribute a submission to",
			env:     shipEnv{state: "running", statusBody: ok, transcript: tr},
			wantMsg: "no block.manifest.json",
		},
		{
			name: "no civitai in the container",
			env: shipEnv{state: "running", statusBody: ok, transcript: tr, noCivitai: true,
				manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "no civitai on PATH",
		},
		{
			// 🔴 THE ONE THAT MATTERS MOST. A failed read is what an expired
			// token, a network partition and an API outage all look like, and it
			// is byte-indistinguishable from an honest empty account unless the
			// exit code is honoured.
			name: "the status call failed",
			env: shipEnv{state: "running", statusBody: ok, transcript: tr, statusRC: "1",
				manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "exited 1",
		},
		{
			name: "the status call returned something with no submissions array",
			env: shipEnv{state: "running", statusBody: `{"error":"nope"}`, transcript: tr,
				manifests: map[string]string{"app": fxShipManifest}},
			wantMsg: "no `submissions` array",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := shipStubEnv(t, tc.env)
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 2 {
				t.Fatalf("exit %d, want 2 (nothing measured)\n%s", code, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("the refusal does not say %q:\n%s", tc.wantMsg, out)
			}
			// It must not ALSO print a verdict — a reader grepping SHIP= would
			// otherwise find one attached to a run that measured nothing.
			if strings.Contains(out, "SHIP=") {
				t.Fatalf("a run that measured nothing still printed a SHIP verdict:\n%s", out)
			}
		})
	}
}

// ── the verdict ──────────────────────────────────────────────────────────────

// T0 = SUBMITTED. All three conjuncts hold, and the account's back catalogue is
// present alongside so that the pass is not an artifact of a listing containing
// only one row.
func TestShipVerdictGradesARealSubmission(t *testing.T) {
	w := shipStdWindow()
	subs := append([]shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}, shipPreExisting(time.Now())...)
	env := shipStubEnv(t, shipEnv{
		state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
		manifests:  map[string]string{"app": fxShipManifest},
		statusBody: shipStatusBody(t, subs),
	})
	out, code := runShipVerdict(t, env, "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0 (something was measured)\n%s", code, out)
	}
	if got := shipField(t, out, "SHIP"); got != "yes" {
		t.Fatalf("SHIP=%s, want yes\n%s", got, out)
	}
	// 🔴 THE OBSERVED VALUES ARE ON THE LINE, NOT JUST A BOOLEAN. A bare `yes`
	// cannot be checked against the account afterwards, and a bare `no` cannot
	// tell a near-miss (the right app, the wrong status) from nothing at all.
	for k, want := range map[string]string{
		"sub_id":     "pubreq_trial",
		"sub_block":  "ab-ship-01",
		"sub_status": "pending",
	} {
		if got := shipField(t, out, k); got != want {
			t.Fatalf("%s=%s, want %s\n%s", k, got, want, out)
		}
	}
	if got := shipField(t, out, "submitted_at"); got == "" || got == "none" {
		t.Fatalf("submitted_at is %q — the verdict carries no evidence of WHEN\n%s", got, out)
	}
	// The positive control for the read itself. A zero here is what a probe
	// wired to nothing returns, and it must be visible on the cell rather than
	// inferred from a `no`.
	if got := shipField(t, out, "account_submissions"); got != "3" {
		t.Fatalf("account_submissions=%s, want 3 — without this a `no` cannot be told from a probe that read an empty account\n%s", got, out)
	}
}

// 🔴 THE FOUR NEGATIVE CONTROLS. Each holds every conjunct but ONE. Read them
// as a matrix: if any of them goes green, the verdict has stopped measuring the
// conjunct that case removes, and a ship cell will report success for the
// operator's own back catalogue.
func TestShipVerdictFailsEveryNegativeControl(t *testing.T) {
	w := shipStdWindow()
	inWindow := w.mid.UTC().Format(time.RFC3339)
	for _, tc := range []struct {
		name     string
		subs     []shipSub
		wantMsg  string
		wantMine string // how many rows named the trial's own app
	}{
		{
			// (1) An untouched scaffold that was never submitted. The manifest
			// exists — a scaffold has one — so this is a VERDICT, not an
			// unmeasured. The account still holds its real apps.
			name:     "an untouched scaffold that was never submitted",
			subs:     nil,
			wantMsg:  "names any of the trial's own apps",
			wantMine: "0",
		},
		{
			// (2) THE IDENTITY CONTROL, and the one the whole script exists for.
			// A pre-existing app, PENDING, submitted INSIDE the run window: every
			// conjunct except the blockId holds. A verdict without the identity
			// check grades the operator's back catalogue as this trial's success.
			name: "a pending submission for a PRE-EXISTING app, inside the window",
			subs: []shipSub{
				{ID: "pubreq_other", BlockID: "custom-generators", Status: "pending", SubmittedAt: inWindow},
			},
			wantMsg:  "names any of the trial's own apps",
			wantMine: "0",
		},
		{
			// (3) THE TIME CONTROL. The trial's OWN app, PENDING — but submitted
			// a week before this trial started. This is the shape a RE-GRADE of an
			// older trial takes, and the shape an operator hand-submitting the
			// same slug takes.
			name: "the trial's own app, pending, but submitted before the run window",
			subs: []shipSub{
				{ID: "pubreq_stale", BlockID: "ab-ship-01", Status: "pending",
					SubmittedAt: w.start.Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)},
			},
			wantMsg:  "outside this trial's run window",
			wantMine: "1",
		},
		{
			// (4a-c) THE STATUS CONTROL, three ways. Right app, right window,
			// wrong state: none of these is "in the moderation queue".
			name: "the trial's own app, in the window, but WITHDRAWN",
			subs: []shipSub{
				{ID: "pubreq_wd", BlockID: "ab-ship-01", Status: "withdrawn", SubmittedAt: inWindow},
			},
			wantMsg:  "is 'withdrawn', not 'pending'",
			wantMine: "1",
		},
		{
			name: "the trial's own app, in the window, but REJECTED",
			subs: []shipSub{
				{ID: "pubreq_rj", BlockID: "ab-ship-01", Status: "rejected", SubmittedAt: inWindow},
			},
			wantMsg:  "is 'rejected', not 'pending'",
			wantMine: "1",
		},
		{
			// ⚠ `approved` is the arguable one and the call is deliberate: T0 is
			// SUBMITTED, graded from a trial's own run window, and nothing a
			// moderator does inside a 20-minute trial produces an approval. An
			// `approved` row for the trial's slug inside the window therefore
			// means the state machine is not what this verdict models, and
			// reporting `no` with the status on the line is the honest answer.
			name: "the trial's own app, in the window, but already APPROVED",
			subs: []shipSub{
				{ID: "pubreq_ap", BlockID: "ab-ship-01", Status: "approved", SubmittedAt: inWindow},
			},
			wantMsg:  "is 'approved', not 'pending'",
			wantMine: "1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subs := append(append([]shipSub{}, tc.subs...), shipPreExisting(time.Now())...)
			env := shipStubEnv(t, shipEnv{
				state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
				manifests:  map[string]string{"app": fxShipManifest},
				statusBody: shipStatusBody(t, subs),
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 (a verdict, not unmeasured)\n%s", code, out)
			}
			if got := shipField(t, out, "SHIP"); got != "no" {
				t.Fatalf("SHIP=%s, want no — this control removes exactly one conjunct, so a "+
					"`yes` means that conjunct is no longer checked\n%s", got, out)
			}
			if got := shipField(t, out, "matched"); got != tc.wantMine {
				t.Fatalf("matched=%s, want %s\n%s", got, tc.wantMine, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("the reason does not say %q — a bare `no` cannot tell a near-miss "+
					"from an account that was never touched:\n%s", tc.wantMsg, out)
			}
		})
	}
}

// 🔴 THE DISCRIMINATING CONTROL FOR THE CONTROLS. Without it, a script that
// always answered `no` would satisfy every negative case above — and six green
// negative controls would read as thorough coverage of a verdict that can never
// say yes. This is the same role `TestDogfoodAPricedZeroCostTurnStillRuns` plays
// for the unpriced-turn stop.
//
// It pairs the passing arm with the identity arm on ONE fixture set that differs
// only in the manifest's blockId, so the two verdicts cannot both be explained
// by anything else.
func TestShipVerdictSeparatesTheTrialsAppFromTheAccountsOwn(t *testing.T) {
	w := shipStdWindow()
	subs := append([]shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}, shipPreExisting(time.Now())...)
	body := shipStatusBody(t, subs)
	for _, tc := range []struct {
		name     string
		blockID  string
		wantShip string
	}{
		{"the manifest names the submitted app", "ab-ship-01", "yes"},
		{"the manifest names a DIFFERENT app the trial built", "ab-ship-02", "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := fmt.Sprintf(`{"blockId":%q,"version":"0.1.0","name":"Ship","type":"block"}`, tc.blockID)
			env := shipStubEnv(t, shipEnv{
				state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
				manifests: map[string]string{"app": m}, statusBody: body,
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := shipField(t, out, "SHIP"); got != tc.wantShip {
				t.Fatalf("SHIP=%s, want %s (blockId %q against the same listing)\n%s",
					got, tc.wantShip, tc.blockID, out)
			}
		})
	}
}

// The trial's slug is read out of the CONTAINER'S manifest, normalised the way
// appapi.SameSlug normalises — trimmed and case-folded. `manifest.Load` is a
// bare json.Unmarshal with no schema validation, so a hand-edited
// `"blockId": " Ab-Ship-01 "` reaches this comparison today, and an exact byte
// compare against the server's spelling would report a confident `no` for a
// submission that plainly is the trial's.
//
// ⚠ An INVARIANT guard, not regression coverage: there is no pre-change script
// for it to have been red against.
func TestShipVerdictNormalisesTheSlugTheWayTheAPIDoes(t *testing.T) {
	w := shipStdWindow()
	subs := []shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}
	env := shipStubEnv(t, shipEnv{
		state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
		manifests:  map[string]string{"app": `{"blockId":"  Ab-Ship-01  ","version":"0.1.0"}`},
		statusBody: shipStatusBody(t, subs),
	})
	out, code := runShipVerdict(t, env, "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := shipField(t, out, "SHIP"); got != "yes" {
		t.Fatalf("SHIP=%s, want yes — a padded, mixed-case blockId is the same app\n%s", got, out)
	}
}

// A trial with no `end` record — the shape the driver's 1500s timeout leaves —
// is still gradeable: the window's upper bound becomes `now`, and the cell SAYS
// which bound it used rather than presenting an open window as a measured one.
func TestShipVerdictGradesATrialThatNeverFinished(t *testing.T) {
	start := time.Now().Add(-20 * time.Minute)
	s, _ := json.Marshal(map[string]any{
		"t": float64(start.UnixNano()) / 1e9, "kind": "start", "trial": "ctl",
		"brief_name": "ship", "container": "dogfood-ctl",
	})
	subs := []shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339),
	}}
	env := shipStubEnv(t, shipEnv{
		state: "running", transcript: string(s) + "\n",
		manifests:  map[string]string{"app": fxShipManifest},
		statusBody: shipStatusBody(t, subs),
	})
	out, code := runShipVerdict(t, env, "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := shipField(t, out, "SHIP"); got != "yes" {
		t.Fatalf("SHIP=%s, want yes\n%s", got, out)
	}
	if !strings.Contains(out, "window_end_source=now-no-end-record") {
		t.Fatalf("the cell does not say the upper bound was inferred:\n%s", out)
	}
}

// ── the RENDER half of a ship cell ───────────────────────────────────────────

// 🔴 THE DELEGATION IS A CLAIM AND THIS IS WHAT CHECKS IT. `ship.assert.mjs`
// spawns `genpost.assert.mjs`, and the whole argument for doing that — one
// predicate, one place, `genpost.md`'s controls still being controls for it — is
// only true while the two produce the SAME verdict on the SAME block. A structural
// grep for the delegate's filename (TestShipBriefAndAssertionAgree, clause d)
// cannot see a wrapper that swallows stdout, drops argv[3], or turns exit 2 into
// exit 1: it type-checks past all three. So this drives the real oracle, the real
// browser and both assertions over one fixture set and compares the results.
//
// Requires a browser, and FAILS rather than skips under $CI for the same reason
// every other oracle test does: a skip and a pass read identically in the log a
// merge gate is read from.
func TestOracleGradesTheShipBriefThroughTheGenpostAssertion(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		html       string
		wantRender string
	}{
		{"a generate-then-post app", fxGenpost, "yes"},
		{"generate only, no Post control", fxGenpostNoPost, "no"},
		{"Post open before anything exists to post", fxGenpostPostEnabled, "no"},
		// ⚠ A SHAPED fixture, not a real scaffold: a block with none of the
		// brief's test ids, standing in for "the model changed nothing that
		// matters". The genuine `civitai app init` scaffold control for these
		// exact constants is recorded in briefs/genpost.md (all three templates,
		// measured) and is INHERITED here through the delegation — it was not
		// re-run for this brief. briefs/ship.md says so.
		{"a block with none of the brief's hooks", fxGood, "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := func(brief string) (render, reason, observed string) {
				t.Helper()
				env := append(stubOracleEnv(t, stubEnv{
					state: "running", civitaiRC: "0",
					manifest: fxManifestScoped, outputDir: "dist", appHTML: tc.html,
				}), "CIVITAI_CHROME="+browser)
				out, code := runScript(t, "oracle.sh", env, "ctl", "root", brief)
				if code != 0 {
					t.Fatalf("oracle.sh %s exited %d, want 0\n%s", brief, code, out)
				}
				for _, line := range strings.Split(out, "\n") {
					if strings.HasPrefix(line, "render_reason=") {
						reason = strings.TrimPrefix(line, "render_reason=")
					}
				}
				return summaryField(t, out, "RENDER"), reason, summaryField(t, out, "observed")
			}
			shipR, shipReason, shipObs := read("ship")
			gpR, gpReason, gpObs := read("genpost")
			if shipR != tc.wantRender {
				t.Fatalf("ship RENDER=%s, want %s", shipR, tc.wantRender)
			}
			// 🔴 THE EQUIVALENCE, not just the verdict. `observed` and the reason
			// are what a reader uses to tell a near-miss from nothing at all, and
			// a wrapper that lost either would still agree on the boolean.
			if shipR != gpR || shipReason != gpReason || shipObs != gpObs {
				t.Fatalf("ship.assert.mjs and genpost.assert.mjs disagree on the same block — "+
					"the delegation is not transparent:\n  ship:    RENDER=%s observed=%s reason=%q\n"+
					"  genpost: RENDER=%s observed=%s reason=%q",
					shipR, shipObs, shipReason, gpR, gpObs, gpReason)
			}
		})
	}
}

// 🔴 THE VERDICT READS ONLY `civitai app status`, AND NEVER A MUTATING VERB.
// `civitai app listing status` is NOT a read — on a live listing it opens a
// shadow revision server-side that this CLI has no command to close (it happened
// to the operator's `panorama-360` listing on 2026-09-25). A grader that
// mutated the thing it grades would be the "verification step caused the
// failure" shape, and nothing else here would notice.
//
// This is a STRUCTURAL guard over the script's own text, because the behavioural
// route cannot see it: the stub CLI answers any argv, so a script that called
// `app listing status` would still produce a green verdict.
func TestShipVerdictNeverMutatesTheAccount(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(shipDir, "ship.verdict.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	// The claim: no line that ACTUALLY runs something (i.e. is not a comment)
	// may name anything from the mutating set. The comments DO name several of
	// them, because they explain why each is absent — so a whole-file grep would
	// be a guard on prose, and would have to be weakened until it measured
	// nothing. Skipping comment lines is what keeps it a guard on code.
	for i, line := range strings.Split(body, "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "#") {
			continue
		}
		for _, forbidden := range []string{
			"civitai app listing", "civitai app submit", "civitai app withdraw",
			"civitai app init", "civitai app create", "civitai generate",
			"docker rm", "docker stop", "docker commit",
		} {
			if strings.Contains(code, forbidden) {
				t.Errorf("line %d runs %q — the ship verdict must READ the account, never change it "+
					"and never destroy the evidence container:\n  %s", i+1, forbidden, code)
			}
		}
	}
	// POSITIVE CONTROL: the scan must be able to SEE a command at all. Without
	// it, a script whose lines were all comments would pass the loop above
	// having checked nothing.
	if !strings.Contains(body, "civitai app status --json") {
		t.Fatal("the script does not run `civitai app status --json` — this scan just proved nothing")
	}
}
