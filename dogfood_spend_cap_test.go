package cli_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The SPEND-BOUND and LISTING-GATING paths of the blind dogfood harness
// (scripts/dogfood).
//
// 🔴 DEFECT 1 — `--max-cost` COULD SILENTLY NEVER FIRE. runner.py's loop
// accumulated spend with `usage_total["cost"] += u.get("cost", 0.0) or 0.0`,
// the only cost-accumulation site in the file, with no price table and no
// `/api/v1/models` fallback behind it. A model or a route whose usage payload
// omits `cost` — or returns it null — therefore accumulated $0 forever: the cap
// never tripped, and `--max-steps` was the only remaining bound, which the
// module's own docstring says in so many words is NOT a spend cap and that
// `--max-cost` is "the only bound on money". An unbounded-spend path, and the
// `end` record's `cost: 0.0` was the money-shaped twin of the `stop:
// "finished"` defect one file over: an absent value rendered as a successful
// measurement.
//
// 🔴 DEFECT 2 — THE APP-PREFIX CAP WAS BYPASSABLE BY WRITING `--slug=NAME`.
// `positional()` dropped every token starting with `-`, so the ATTACHED form of
// a flag contributed no candidate at all. `civitai app listing status
// --slug=some-other-app` then fell through to the "read every
// `block.manifest.json` under /work" branch and was ACCEPTED, because the
// workspace manifests all carry the prefix while `--slug` pointed at a real
// listing on the operator's account. The separated form (`--slug NAME`) WAS
// caught; cobra treats the two spellings as identical and this classifier did
// not. It defeated the cap for every gated verb, not just the one it was found
// on.
//
// Everything here runs offline through testdata/fake_trial.py: no Docker, no
// OpenRouter, no money, no account.

// The stop value a run that cannot be priced must carry. Pinned as a literal
// rather than read out of runner.py, because deriving it would make the test
// agree with whatever runner.py says — which is not a contract.
const unpricedStopPrefix = "unpriced-turn"

// How many requests the runner actually issued to the provider. This is the
// money question: one unpriced turn must not be followed by a second billed
// call.
func requestCount(t *testing.T, capture string) int {
	t.Helper()
	var c struct {
		Payloads []json.RawMessage `json:"payloads"`
	}
	if err := json.Unmarshal([]byte(readFile(t, capture)), &c); err != nil {
		t.Fatalf("reading the capture: %v", err)
	}
	return len(c.Payloads)
}

// ── fix 1: an unpriced turn is a hard stop ───────────────────────────────────

// 🔴 THE REGRESSION TEST. Red on origin/main, where every one of these shapes
// is scored `stop: "finished"` with `usage.cost: 0` — a trial that really was
// billed, recorded as free, with the only money bound in the harness inert.
//
// The four shapes are the four ways a provider can fail to price a turn, and
// they are separate cases rather than one because they arrive by different
// routes: an older API version that never sent the field, a route that sends
// the key with a null, a provider that omits the `usage` object entirely, and
// one that sends the figure as a STRING.
//
// ⚠ THE FOUR ARMS ARE NOT RED AT BASE FOR THE SAME REASON, AND THE DIFFERENCE
// IS WORTH THE SENTENCE. Measured at 4d4a45e: the first three record
// `stop: "finished"` with `usage.cost: 0` — the silent defect this test is
// named for. The STRING arm instead dies with
// `TypeError: unsupported operand type(s) for +=: 'float' and 'str'`, because
// the old `usage_total["cost"] += u.get("cost", 0.0) or 0.0` cannot add it.
// That is still a real defect — the trial crashes after it has been billed,
// with no `end` record, and driver.sh re-runs it — but it is a CRASH, not a
// false success, so do not count that one arm as evidence for the silent case.
// The other three are.
func TestDogfoodUnpricedTurnIsAHardStop(t *testing.T) {
	for _, tc := range []struct{ name, cost string }{
		{"no cost key", "__absent__"},
		{"cost is null", "__null__"},
		{"no usage object at all", "__nousage__"},
		{"cost is not a number", "0.004-usd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrialEnv(t, []string{"FAKE_USAGE_COST=" + tc.cost}, nil, "")

			end := endRecord(t, readFile(t, tr.transcript))
			got, _ := end["stop"].(string)
			if !strings.HasPrefix(got, unpricedStopPrefix) {
				t.Fatalf("a turn the provider did not price was recorded stop=%q, want a %q value.\n"+
					"The harness cannot bound this run's spend and must say so instead of "+
					"counting the turn as $0.\n%s",
					got, unpricedStopPrefix, readFile(t, tr.transcript))
			}
			// The terminal state is NOT lost to the new value: `finish_reason`
			// still rides out on the end record, so a reader can still tell what
			// the provider said the model did.
			if end["finish_reason"] != "stop" {
				t.Fatalf("the end record dropped finish_reason: %v", end["finish_reason"])
			}
			// And the summary an operator actually reads.
			s := runSummary(t, tr.stdout)
			if sv, _ := s["stop"].(string); !strings.HasPrefix(sv, unpricedStopPrefix) {
				t.Fatalf("the run summary hides the unpriced turn: stop=%v\n%s", s["stop"], tr.stdout)
			}
		})
	}
}

// 🔴 THE MONEY HALF, AND IT IS A DIFFERENT CLAIM FROM THE STOP VALUE. A "hard
// stop" that recorded the right word and then issued another billed request
// would satisfy the test above completely. What has to be true is that the
// SECOND call never happens: the run ends after the one request that was
// unavoidable.
//
// 🔴 ONE CALL IS THE FLOOR, DELIBERATELY. You cannot know a provider omits
// `cost` until a response arrives, so the first request is spent no matter
// what. There is no grace period beyond it — N turns of unknown cost is still
// unbounded spend, bounded only by a number nobody can convert into money.
//
// Red on origin/main: there the loop runs the tool call, takes a second turn,
// and keeps going — 2 requests and a `tool` record.
func TestDogfoodUnpricedTurnStopsBeforeTheNextBilledCall(t *testing.T) {
	tr := runFakeTrialEnv(t, []string{"FAKE_USAGE_COST=__absent__"},
		[]string{"echo one", "echo two"}, "")

	if n := requestCount(t, tr.capture); n != 1 {
		t.Fatalf("the runner issued %d provider request(s) after an unpriced turn, want 1.\n"+
			"Every request past the first is spend the harness cannot measure and cannot "+
			"cap.", n)
	}
	if v := stepVerdicts(t, readFile(t, tr.transcript)); len(v) != 0 {
		t.Fatalf("the unpriced turn's tool calls were executed anyway (%v) — the run was "+
			"already over", v)
	}
	end := endRecord(t, readFile(t, tr.transcript))
	if got, _ := end["stop"].(string); !strings.HasPrefix(got, unpricedStopPrefix) {
		t.Fatalf("stop=%q, want %q", got, unpricedStopPrefix)
	}
}

// 🔴 THE DISCRIMINATING CONTROL, WITHOUT WHICH EVERY ASSERTION ABOVE IS
// SATISFIED BY "ALWAYS STOP". A provider that SAYS a turn cost $0 — a free
// route — is making a claim, and the run must proceed on it. Absence is not a
// price; zero is. An implementation that treated a falsy cost as unknown passes
// all four arms above and fails here.
//
// ⚠ THIS IS AN INVARIANT GUARD, NOT REGRESSION COVERAGE. It PASSES on
// origin/main — it has to, because what it pins is that nothing changed for a
// well-behaved provider. It was watched to go red the only way it can: by
// mutating turn_cost to `return None if not value`, i.e. the defect it exists
// to catch. Do not count it in a red-at-base matrix.
func TestDogfoodAPricedZeroCostTurnStillRuns(t *testing.T) {
	tr := runFakeTrialEnv(t, []string{"FAKE_USAGE_COST=0"}, []string{"echo one"}, "")

	end := endRecord(t, readFile(t, tr.transcript))
	if got := end["stop"]; got != "finished" {
		t.Fatalf("a provider reporting a $0 turn was stopped with stop=%q, want \"finished\". "+
			"A stated zero is a price; only an ABSENT figure is unknown.\n%s",
			got, readFile(t, tr.transcript))
	}
	if n := requestCount(t, tr.capture); n != 2 {
		t.Fatalf("a $0-priced run issued %d request(s), want 2 (the tool turn and the terminal "+
			"one) — the run was cut short", n)
	}
}

// The other half of the same discrimination: an ORDINARY priced run is
// untouched. Same invariant-guard caveat as above — it passes at base.
func TestDogfoodAnOrdinaryPricedRunIsUnchanged(t *testing.T) {
	tr := runFakeTrial(t, []string{"echo hello"}, "")
	if got := endRecord(t, readFile(t, tr.transcript))["stop"]; got != "finished" {
		t.Fatalf("an ordinarily priced run was recorded stop=%q, want \"finished\"\n%s",
			got, readFile(t, tr.transcript))
	}
}

// 🔴 THE TABLE THAT PINS WHAT COUNTS AS A PRICE, exercised against the real
// function rather than through a trial, because two of these shapes cannot be
// produced by the stub provider's env knobs (`cost: true`, which Python's
// `isinstance(True, int)` would otherwise accept as a number, and a bare
// non-dict `usage`).
//
// ⚠ NOT REGRESSION COVERAGE, AND DELIBERATELY NOT COUNTED AS SUCH. `turn_cost`
// does not exist on origin/main, so at base this dies with an AttributeError —
// a DIFFERENT failure from the one it claims to test. The red-at-base evidence
// for this defect is the four end-to-end arms above; this is a unit table that
// keeps the predicate's edges from drifting.
func TestDogfoodTurnCostTreatsOnlyANumberAsAPrice(t *testing.T) {
	py := dogfoodPython(t)
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("r", sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
cases = [
    ({"cost": 0.25}, 0.25),
    ({"cost": 0}, 0.0),
    ({"cost": 0.0}, 0.0),
    ({"cost": -1.5}, -1.5),
    ({}, None),
    ({"cost": None}, None),
    ({"cost": "0.25"}, None),
    ({"cost": True}, None),
    ({"cost": []}, None),
    (None, None),
    ("usage", None),
]
bad = []
for usage, want in cases:
    got = m.turn_cost(usage)
    if got != want:
        bad.append("turn_cost(%r) = %r, want %r" % (usage, got, want))
print(json.dumps(bad))
`
	out, err := exec.Command(py, "-c", script, filepath.Join(dogfoodDir, "runner.py")).CombinedOutput()
	if err != nil {
		t.Fatalf("exercising turn_cost: %v\n%s", err, out)
	}
	var bad []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &bad); err != nil {
		t.Fatalf("turn_cost table did not report: %v\n%s", err, out)
	}
	if len(bad) != 0 {
		t.Fatalf("turn_cost disagrees with the contract:\n  %s", strings.Join(bad, "\n  "))
	}
}

// ── fix 2a: `--slug=NAME` must not bypass the app-prefix cap ─────────────────

// 🔴 THE REGRESSION TEST FOR THE BYPASS. Red on origin/main, where every one of
// these is ALLOWED and reaches the container: `positional()` dropped the
// attached-form token, no slug candidate was produced, and _prefix_ok fell
// through to its manifest branch — which passes, because the workspace
// manifests were created by the trial and all carry the prefix while `--slug`
// names something else.
//
// 🔴 ASSERTED ON THE SPECIFIC REFUSAL, NOT MERELY ON "REFUSED". Every command
// here would also be refused by a classifier that refused everything, and by
// the fail-closed rule, and by a cap counter — three ways to be green for the
// wrong reason. The refusal has to name the FOREIGN SLUG it caught.
func TestDogfoodAttachedSlugFlagDoesNotBypassThePrefixCap(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, tc := range []struct{ name, command, slug string }{
		// The one that was found in the wild: a "status" read pointed at a real
		// listing on the operator's account.
		{"listing status", "civitai app listing status --slug=panorama-360", "panorama-360"},
		{"listing status json", "civitai app listing status --json --slug=panorama-360", "panorama-360"},
		{"listing set-icon", "civitai app listing set-icon ./i.png --slug=sensei", "sensei"},
		{"listing rm-screenshot", "civitai app listing rm-screenshot alsc_01 --slug=sensei", "sensei"},
		{"listing submit-revision", "civitai app listing submit-revision --slug=sensei", "sensei"},
		// The bypass was never listing-specific — it defeated the cap for every
		// gated verb, and a fix that only taught `listing` about `=` would leave
		// these open.
		{"app submit", "civitai app submit --yes --slug=sensei", "sensei"},
		{"app create", "civitai app create --slug=sensei", "sensei"},
		// 🔴 THE STRUCTURAL ARM. Everything above spells `--slug=`, so a fix
		// that special-cased that one flag name would satisfy all of it — and
		// the hazard is the `=` SHAPE, not the word. `--dir` is the other flag
		// `listingCommon` binds, and the SEPARATED form `--dir sensei` is
		// already refused at base, so this is the attached form being made to
		// agree with it rather than a new strictness.
		{"a flag that is not --slug", "civitai app submit --dir=sensei", "sensei"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrial(t, []string{tc.command}, "",
				"--credential-file", credPath, "--app-prefix", "dogfood4-")
			got := stepVerdicts(t, readFile(t, tr.transcript))
			if len(got) != 1 || got[0] != "refused" {
				t.Fatalf("%q reached the container (verdicts %v).\nThe attached form of --slug "+
					"bypassed the app-prefix cap: the harness saw no slug and fell back to the "+
					"workspace manifests, which all carry the prefix.\n%s",
					tc.command, got, readFile(t, tr.transcript))
			}
			// The SPECIFIC refusal: the prefix check, naming the slug it caught.
			body := readFile(t, tr.transcript)
			if !strings.Contains(body, "dogfood4-") || !strings.Contains(body, tc.slug) {
				t.Fatalf("%q was refused for the wrong reason — the refusal names neither the "+
					"prefix nor %q, so this is not the prefix cap firing:\n%s",
					tc.command, tc.slug, body)
			}
		})
	}
}

// The mirror image, and the reason the fix is a widening rather than a blanket
// refusal: the trial's OWN app, addressed in the attached form, must still run.
// A "fix" that refused every `--opt=value` token would pass every arm above.
//
// ⚠ Red at base for a reason worth stating: at base these were allowed too, so
// this arm alone proves nothing about the bypass. It is here to stop the fix
// over-refusing, which is the failure mode that would make the cap useless by
// making the trial unrunnable.
func TestDogfoodAttachedSlugOnTheTrialsOwnAppStillRuns(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, cmd := range []string{
		"civitai app listing status --slug=dogfood4-thing",
		"civitai app init --slug=dogfood4-thing",
		"civitai app listing set-icon ./icon.png --slug=dogfood4-thing",
		// A flag whose value is a path, not a slug — it must not be read as one.
		"civitai app listing set-icon ./icon.png --dir=./dogfood4-thing",
	} {
		t.Run(cmd, func(t *testing.T) {
			tr := runFakeTrial(t, []string{cmd}, "",
				"--credential-file", credPath, "--app-prefix", "dogfood4-")
			got := stepVerdicts(t, readFile(t, tr.transcript))
			if len(got) != 1 || got[0] != "run" {
				t.Fatalf("%q was refused (verdicts %v) — the prefix cap now blocks the trial's "+
					"own app, which makes a credentialed trial unrunnable.\n%s",
					cmd, got, readFile(t, tr.transcript))
			}
		})
	}
}

// ── fix 2b: `app listing set-text` is refused by default ─────────────────────

// 🔴 THE REGRESSION TEST. Red on origin/main, where `set-text` is only
// prefix-gated — and the prefix gate is OPTIONAL: `armed` is true on a
// credential alone, and `_prefix_ok` returns immediately when `--app-prefix` is
// empty. So a credentialed `runner.py` invocation without a prefix could
// rewrite the public tagline / description / category of any listing on the
// account, in place, with no revision and no undo.
//
// Both arms matter and they fail differently: WITH a prefix the old code
// allowed it because the command named no foreign slug; WITHOUT a prefix the
// old code allowed it because nothing was checked at all.
func TestDogfoodListingSetTextIsRefusedByDefault(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, tc := range []struct {
		name  string
		extra []string
	}{
		{"with an app prefix", []string{"--app-prefix", "dogfood4-"}},
		{"with no app prefix at all", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--credential-file", credPath}, tc.extra...)
			tr := runFakeTrial(t,
				[]string{`civitai app listing set-text --tagline "Batch upscaling, in your browser"`},
				"", args...)
			got := stepVerdicts(t, readFile(t, tr.transcript))
			if len(got) != 1 || got[0] != "refused" {
				t.Fatalf("`app listing set-text` reached the container (verdicts %v). It applies "+
					"IN PLACE on a listing of any status — public on return, no revision, no "+
					"command in this CLI that restores the previous value.\n%s",
					got, readFile(t, tr.transcript))
			}
			// The SPECIFIC refusal. Without this the arm is satisfied by the
			// prefix cap firing on the tagline text, which is a different
			// mechanism that the no-prefix arm does not even have.
			body := readFile(t, tr.transcript)
			if !strings.Contains(body, "set-text") || !strings.Contains(body, "restores the previous value") {
				t.Fatalf("set-text was refused for some OTHER reason — the refusal does not name "+
					"the command or why it is disabled:\n%s", body)
			}
		})
	}
}

// The escape hatch, on the `--allow-withdraw` precedent: one flag turns it on
// for a brief that genuinely needs to write listing copy. Deliberately NOT
// threaded through driver.sh, so a matrix run can never reach it.
//
// ⚠ NOT COUNTED AS REGRESSION COVERAGE. At base `--allow-listing-text` is an
// unrecognised argument, so runner.py exits 2 and this dies on "fake trial
// failed" — a DIFFERENT failure from the one it claims to test. It is the
// reachability control for the flag: without it, "refused by default" would be
// indistinguishable from "refused always".
func TestDogfoodListingSetTextIsAllowedWithTheFlag(t *testing.T) {
	credPath, _ := credentialFile(t)
	tr := runFakeTrial(t,
		[]string{`civitai app listing set-text --tagline "Batch upscaling, in your browser"`},
		"", "--credential-file", credPath, "--app-prefix", "dogfood4-", "--allow-listing-text")
	got := stepVerdicts(t, readFile(t, tr.transcript))
	if len(got) != 1 || got[0] != "run" {
		t.Fatalf("--allow-listing-text did not permit set-text (verdicts %v)\n%s",
			got, readFile(t, tr.transcript))
	}
}

// The flag must not widen anything else. `withdraw` has its own flag and keeps
// it; a shared "allow the dangerous things" switch would be a worse gate than
// two narrow ones.
func TestDogfoodAllowListingTextDoesNotPermitWithdraw(t *testing.T) {
	credPath, _ := credentialFile(t)
	tr := runFakeTrial(t, []string{"civitai app withdraw pubreq_01ABC"}, "",
		"--credential-file", credPath, "--app-prefix", "dogfood4-", "--allow-listing-text")
	got := stepVerdicts(t, readFile(t, tr.transcript))
	if len(got) != 1 || got[0] != "refused" {
		t.Fatalf("--allow-listing-text also permitted `app withdraw` (verdicts %v)\n%s",
			got, readFile(t, tr.transcript))
	}
	if !strings.Contains(readFile(t, tr.transcript), "withdraw") {
		t.Fatalf("withdraw was refused by something other than its own gate:\n%s",
			readFile(t, tr.transcript))
	}
}

// ── fix 2c: `app listing status` is not a read, and is not treated as one ────
//
// 🔴 THE CLAIM THE HARNESS COMMENT USED TO MAKE, PINNED AS BEHAVIOUR RATHER
// THAN AS PROSE. runner.py's comment listed `status` among the "read-only and
// ungated" `app` subcommands. That is true of `civitai app status` and FALSE of
// `civitai app listing status`, which (a) goes through the prefix gate because
// `listing` is in APP_MUTATING, and (b) is not a read at all — on a LIVE
// listing it idempotently OPENS a shadow revision draft server-side, and this
// CLI has no `discard-revision` to close it. A report written off that comment
// concluded the opposite of what the code does.
//
// A guard on the comment's WORDS would be walkable by rewording it, so this
// pins the two behaviours the comment is about instead.
func TestDogfoodAppStatusAndAppListingStatusAreGatedDifferently(t *testing.T) {
	credPath, _ := credentialFile(t)

	// `civitai app status` IS a read, and stays ungated — the half of the old
	// comment that was correct.
	//
	// ⚠ Invariant guard: passes at base.
	tr := runFakeTrial(t, []string{"civitai app status --slug=sensei"}, "",
		"--credential-file", credPath, "--app-prefix", "dogfood4-")
	if got := stepVerdicts(t, readFile(t, tr.transcript)); len(got) != 1 || got[0] != "run" {
		t.Fatalf("`civitai app status` was gated (verdicts %v) — it is a read and gating it "+
			"would change what the harness measures\n%s", got, readFile(t, tr.transcript))
	}

	// `civitai app listing status` is NOT, and must be caught when it names a
	// foreign listing. Red at base via the `--slug=` form.
	tr = runFakeTrial(t, []string{"civitai app listing status --slug=panorama-360"}, "",
		"--credential-file", credPath, "--app-prefix", "dogfood4-")
	if got := stepVerdicts(t, readFile(t, tr.transcript)); len(got) != 1 || got[0] != "refused" {
		t.Fatalf("`civitai app listing status --slug=<foreign>` reached the container "+
			"(verdicts %v). It is not a read: on a LIVE listing it opens a shadow revision "+
			"draft that nothing in this CLI can discard.\n%s",
			got, readFile(t, tr.transcript))
	}
}
