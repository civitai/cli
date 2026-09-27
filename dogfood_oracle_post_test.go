package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The POST-PATH ARM of the render oracle, and the reason it has its own file.
//
// # What happened — TWICE, on ONE live app
//
// `ab-img-poster` was graded green by this oracle, submitted, approved and
// deployed to https://ab-img-poster.civit.ai/. It then failed for real users
// twice:
//
//	v0.1.1  Generate -> "Generation failed. Please try again."   (the consent
//	        defect `dogfood_oracle_consent_test.go` exists for)
//	v0.1.2  Post     -> "Posting failed. Please try again."      (this one)
//
// The second one is measured in `dogfood-ab-imgposter-fixed` (the fix) against
// the image `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix` (the defect). The
// defect is two lines:
//
//	const postable = Boolean(workflowResult);                    // v0.1.1
//	const postable = Boolean(workflowResult &&                   // v0.1.2
//	  workflowResult.status === 'succeeded' &&
//	  workflowResult.imageUrls?.length > 0);
//
// `watch()` resolves on ANY terminal status (`TERMINAL_STATUSES` is
// succeeded/failed/canceled/expired) and `imageUrls` is OPTIONAL even on
// `succeeded` — host-side output moderation can empty it — so v0.1.1 opened Post
// for a generation that had produced nothing, and the only possible outcome was
// the host refusing.
//
// # WHY NEITHER EXISTING ARM COULD SEE IT
//
// Both arms above stop on the NEAR side of a request. `InlineTransport.sendRequest`
// rejects every workflow submit and every post, so no generation reaches a
// terminal snapshot at all, no Post gate is ever re-evaluated, and
// "Post works" and "Post fails for every user" leave the identical trace
// `ready>generating>ready`. Measured on the two real bundles (2026-09-27, flag
// unset): the DEFECT and the FIX are byte-for-byte indistinguishable to both arms
// — `pass=true` on the default arm and `pass=true` on the unconsented arm, for
// each of them.
//
// # THE ARM, AND ITS PRICE
//
// `CIVITAI_ASSERT_POST_PATH=1` answers four request types in the page —
// `ESTIMATE_WORKFLOW`, `SUBMIT_WORKFLOW`, `POLL_WORKFLOW`,
// `CREATE_POST_FROM_APP` — so a canned generation completes and a canned post is
// created. That SUSPENDS the invariant every other verdict in this arc rests on
// ("no generation and no post can complete here, ever, on any machine"), which is
// why it is opt-in, why the cell says `arm=post`, and why
// `TestThePostArmSeparatesTheGateFromTheWorkflowResult`'s two default-arm rows and
// `TestThePostArmShimAnswersOnlyItsOwnLedger`'s `default-arm-source-omits:*` checks
// exist. Nothing spends and nothing
// leaves the page: every reply is built by the shim in the browser, `token.raw` is
// still `''`, and the canned image is a `data:` URI rather than the mock host's
// `placehold.co` URL so that even RENDERING a result makes no request.
//
// # WHAT A GREEN HERE DOES NOT MEAN
//
// It proves the block's post BRANCH exists and that its payload satisfies the
// host's payload gate as the SDK's own mock host implements it. It does NOT prove
// the real host accepts the payload: the platform re-resolves every source
// server-side, re-checks the `posts:write:self` grant, opens a viewer confirm and
// moderates the outputs. The arm says so in its own `postArmCeiling` field on
// every cell, and `TestThePostArmStatesItsCeilingOnEveryCell` pins that.

// ── fixtures ─────────────────────────────────────────────────────────────────

// `useBuzzWorkflow` reduced to its wire behaviour: estimate, submit, then poll
// until a terminal status, using the SAME message types and reading the SAME
// response fields the SDK reads (`reply.snapshot`, `snapshot.cost.total`,
// `snapshot.imageUrls`).
//
// 🔴 THE ESTIMATE'S COST CHECK IS NOT DECORATION. The SDK's `estimate()` rejects a
// snapshot whose `cost.total` is not a number (`WorkflowEstimateError` code
// `'no-cost'`), so a shim that answered an estimate without a price would break
// every real app at the first await — and a fixture that did not check would not
// notice. The `TERMINAL` set is `TERMINAL_STATUSES` from
// `@civitai/blocks-react/dist/hooks/useBuzzWorkflow.js`.
const fxPostWorkflow = `
const TERMINAL = { succeeded: 1, failed: 1, canceled: 1, expired: 1 };
function req(type, payload, responseType) {
  return transport.sendRequest({ type: type, payload: payload }, responseType);
}
async function runWorkflow(body) {
  const est = await req('ESTIMATE_WORKFLOW', { body: body }, 'ESTIMATE_RESULT');
  if (typeof ((est.snapshot || {}).cost || {}).total !== 'number') throw new Error('no-cost');
  const sub = await req('SUBMIT_WORKFLOW',
    { body: body, idempotencyKey: 'dogfood-fixture-key' }, 'WORKFLOW_SUBMITTED');
  let snap = sub.snapshot;
  for (let i = 0; i < 20 && !TERMINAL[snap.status]; i++) {
    const p = await req('POLL_WORKFLOW',
      { workflowId: snap.workflowId, waitSeconds: 15 }, 'WORKFLOW_STATUS');
    snap = p.snapshot;
  }
  return snap;
}
`

// The markup and the generate handler every fixture below shares, so a difference
// between two of them is a difference in the POST GATE or the POST PAYLOAD and
// nothing else. `postable` and `postSources` are supplied per fixture.
const fxPostMarkup = `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="gen">Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>' +
  '<div data-testid="error"></div>';
const promptEl = document.querySelector('[data-testid="prompt"]');
const statusEl = document.querySelector('[data-testid="status"]');
const errEl = document.querySelector('[data-testid="error"]');
const genEl = document.getElementById('gen');
const postEl = document.getElementById('post');
let last = null;
genEl.onclick = async function () {
  errEl.textContent = '';
  statusEl.textContent = 'generating';
  postEl.disabled = true;
  try {
    last = await runWorkflow({ kind: 'textToImage', params: { prompt: promptEl.value } });
  } catch (e) {
    last = null;
    errEl.textContent = 'Generation failed. Please try again.';
  }
  statusEl.textContent = 'ready';
  postEl.disabled = !postable(last);
};
`

// The post handler the two payload fixtures share. It reads the reply exactly as
// `useCreatePostFromApp` does — `if (reply.error || !reply.result) throw` — so a
// host refusal lands in the same branch a real block's does.
const fxPostHandler = `
postEl.onclick = async function () {
  if (!postable(last)) return;
  try {
    const r = await req('CREATE_POST_FROM_APP',
      { sources: postSources(), title: 'Generated' }, 'CREATE_POST_RESULT');
    if (r.error || !r.result) throw new Error(r.error || 'no images to post');
    errEl.textContent = 'posted:' + r.result.postId;
  } catch (e) {
    errEl.textContent = 'Posting failed. Please try again.';
  }
};
`

// 🔴 THE FIX, i.e. `ab-img-poster@0.1.2`: Post opens only for a workflow that
// SUCCEEDED and carries at least one image, and it posts that workflow.
const fxPostGateCorrectApp = fxSdkInlineStubWithMessage + fxPostWorkflow + fxPostMarkup + `
function postable(s) {
  return !!s && s.status === 'succeeded' && (s.imageUrls || []).length > 0;
}
function postSources() { return [{ kind: 'workflow', workflowId: last.workflowId }]; }
` + fxPostHandler

// 🔴 THE DEFECT, i.e. `ab-img-poster@0.1.1`, reduced to the ONE expression that
// differs from the fixture above: the gate asks whether the workflow ENDED, not
// whether it produced anything. On the default and unconsented arms this is
// indistinguishable from the fix; on this arm it opens Post for an image-less
// success and the host refuses the post it then sends.
const fxPostGateBlindApp = fxSdkInlineStubWithMessage + fxPostWorkflow + fxPostMarkup + `
function postable(s) { return !!s; }
function postSources() { return [{ kind: 'workflow', workflowId: last.workflowId }]; }
` + fxPostHandler

// 🔴 THE `createMockHost` GATE'S OWN FIXTURE: a correct Post gate and an EMPTY
// `sources` array. `createMockHost` refuses that with `no images to post` — "Mirror
// the real host's payload gate (`resolveCreatePostRequest`) … Without this a block
// bug (an empty selection, say) would WORK in the mock and be refused in
// production" — and so does this arm. It is the reachability case for gate 1, which
// the two fixtures above never touch (both send a one-element array).
const fxPostEmptySourcesApp = fxSdkInlineStubWithMessage + fxPostWorkflow + fxPostMarkup + `
function postable(s) {
  return !!s && s.status === 'succeeded' && (s.imageUrls || []).length > 0;
}
function postSources() { return []; }
` + fxPostHandler

// 🔴 A POST BUTTON WIRED TO NOTHING, and it is not a straw man: the gate logic is
// the CORRECT one, so phase 1 passes and every other field on the cell looks
// healthy. Without phase 2 actually clicking Post and reading the host's ledger,
// this app is indistinguishable from the fix.
const fxPostWiredToNothingApp = fxSdkInlineStubWithMessage + fxPostWorkflow + fxPostMarkup + `
function postable(s) {
  return !!s && s.status === 'succeeded' && (s.imageUrls || []).length > 0;
}
postEl.onclick = function () { errEl.textContent = 'thanks!'; };
`

// 🔴 THE OVER-STRICT APP, and the reason phase 1 alone is not the arm. Its gate
// never opens, so it passes the image-less phase for a reason that has nothing to
// do with handling the hazard: it cannot post at all, which is exactly what the
// brief's step 3 (`no clickable element labelled "post" — nothing posts`) exists
// to reject one step earlier.
const fxPostNeverOpensApp = fxSdkInlineStubWithMessage + fxPostWorkflow + fxPostMarkup + `
function postable(s) { return false; }
function postSources() { return []; }
` + fxPostHandler

// 🔴 THE INVARIANT FIXTURE FOR THIS ARM. Correct in every graded respect, and it
// additionally attempts three spend-adjacent requests that are NOT on the arm's
// allowlist — a top-up purchase, a workflow cancel and a bare image publish, all
// reachable from a real generate-then-post app and none of them needed to reach
// Post.
//
// 🔴 IT OBSERVES THE OUTCOME FROM INSIDE THE PAGE, AND THE FIRST VERSION DID NOT
// — WHICH IS WHY THIS COMMENT EXISTS. That version ignored the results and the
// guard read the oracle's own `hostRefused` ledger instead. A mutation that
// replaced the shim's `Promise.reject(new Error(STUB))` with a `Promise.resolve`
// SURVIVED a fully green run: the shim still pushed the type onto `refused` on its
// way past, so the ledger named all three while all three had in fact been
// ANSWERED. The guard was SPELLED, not structural. Now the block itself checks that
// each one rejected with the SDK's own message, and SHUTS ITS OWN POST GATE if any
// did not — so the arm's verdict moves, which is the only signal a block cannot be
// fooled about. The ledger assertions are kept as the cheap structural half.
const fxPostArmSpendProbeApp = fxSdkInlineStubWithMessage + fxPostWorkflow + fxPostMarkup + `
const STUB = 'InlineTransport.sendRequest is not implemented in v1';
const spendAnswered = [];
function probe(type, payload) {
  return req(type, payload, 'IGNORED').then(
    function () { spendAnswered.push('resolved:' + type); },
    function (e) { if (e.message !== STUB) spendAnswered.push('wrong-error:' + type); });
}
probe('OPEN_BUZZ_PURCHASE', {});
probe('CANCEL_WORKFLOW', { workflowId: 'wf_dogfood_1' });
probe('PUBLISH_GENERATION_OUTPUTS', { workflowId: 'wf_dogfood_1', imageIndexes: [0] });
function postable(s) {
  if (spendAnswered.length) {
    errEl.textContent = 'ANSWERED:' + spendAnswered.join('+');
    return false;
  }
  return !!s && s.status === 'succeeded' && (s.imageUrls || []).length > 0;
}
function postSources() { return [{ kind: 'workflow', workflowId: last.workflowId }]; }
` + fxPostHandler

// ── helpers ──────────────────────────────────────────────────────────────────

// The one env assignment that selects the arm. Named rather than spelled at a
// dozen call sites: it is also the string `stubOracleEnv` clears, and the two must
// not drift.
const postPathArm = "CIVITAI_ASSERT_POST_PATH=1"

// Runs the oracle over a post fixture served as a built app with an external
// module bundle — the shape every real built block has, and the one that makes the
// patch's `Script` interception path the one under test.
func runPostOracle(t *testing.T, browser, appJS string, extraEnv ...string) (string, int) {
	t.Helper()
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
		manifest: fxManifestScoped, outputDir: "dist", appHTML: fxExternalModuleHTML,
		appFiles: map[string]string{"app.js": appJS},
	}), "CIVITAI_CHROME="+browser)
	env = append(env, extraEnv...)
	return runScript(t, "oracle.sh", env, "ctl", "root")
}

// ── the regression: one expression, two arms, two verdicts ───────────────────

// 🔴 THE HEADLINE TEST, AND IT IS A 2x2. The blind gate and the correct gate
// differ in ONE expression. Both must grade `yes` on the DEFAULT arm — that arm's
// question ("did the Generate click drive the machine to `generating`") is answered
// correctly by both, and no existing verdict may move — and they must SEPARATE on
// the post arm.
//
// ⚠ THE PAIR IS THE TEST, IN BOTH DIRECTIONS. The two post rows alone are
// satisfiable by an arm that fails everything except the one shape it was written
// against; the two default rows alone are satisfiable by an arm that does nothing.
// Measured on the REAL bundles as well — see the matrix in the PR body: the
// pre-fix bundle (md5 `b8a1936e5939e39aa3e26f6e9e777113`) grades `no` on this arm
// and `yes` on both others, the fixed one (md5
// `c603fa854eba4d5206b73e04c14a1f3c`) grades `yes` on all three.
//
// RED AT THE PR'S BASE: `CIVITAI_ASSERT_POST_PATH` does not exist there, so
// `arm=post` is never reported (the `arm` assertion fails first) and both post rows
// grade `yes`.
func TestThePostArmSeparatesTheGateFromTheWorkflowResult(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		app        string
		env        []string
		wantArm    string
		wantRender string
		// A substring the run's own reason must carry, so a `no` earned for some
		// unrelated reason cannot pass as this one.
		wantReason string
	}{
		{"the blind gate on the default arm (must not move)", fxPostGateBlindApp, nil, "consented", "yes", ""},
		{"the correct gate on the default arm (must not move)", fxPostGateCorrectApp, nil, "consented", "yes", ""},
		{"the blind gate on the post arm", fxPostGateBlindApp, []string{postPathArm}, "post", "no",
			`is OPEN after a workflow that reached "succeeded" with an EMPTY imageUrls list`},
		{"the correct gate on the post arm", fxPostGateCorrectApp, []string{postPathArm}, "post", "yes", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runPostOracle(t, browser, tc.app, tc.env...)
			if code != 0 {
				t.Fatalf("exit %d, want 0 — a verdict was measured either way\n%s", code, out)
			}
			if got := summaryField(t, out, "arm"); got != tc.wantArm {
				t.Fatalf("arm=%s, want %s — the cell does not say which question it answers\n%s",
					got, tc.wantArm, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
			if tc.wantReason != "" && !strings.Contains(out, tc.wantReason) {
				t.Fatalf("the reason does not carry %q — this `no` may have been earned for an "+
					"unrelated reason\n%s", tc.wantReason, out)
			}
			if tc.wantRender == "no" {
				// The causal chain, from the oracle's own ledger: the app sent a post
				// for the image-less workflow and the host refused it with the
				// platform's own code. A reason without this is a claim; with it, it is
				// a measurement.
				if got := assertionField(t, out, "postArmPosts"); !strings.HasPrefix(got, "refused:image-less-workflow:") {
					t.Fatalf("postArmPosts=%q, want a refused:image-less-workflow: entry — the arm did "+
						"not actually show the consequence\n%s", got, out)
				}
			}
			if tc.wantArm == "consented" {
				// 🔴 THE FLAG-UNSET HALF, ASSERTED RATHER THAN ASSUMED. With the arm
				// off, the workflow requests must still be REFUSED with the SDK's own
				// error — which is what makes "no recorded verdict moves" a measurement.
				// `ESTIMATE_WORKFLOW` is the first one every fixture here attempts.
				if got := assertionField(t, out, "hostRefused"); !strings.Contains(got, "ESTIMATE_WORKFLOW") {
					t.Fatalf("hostRefused=%q on the DEFAULT arm, want it to name ESTIMATE_WORKFLOW — "+
						"the post arm is answering requests with its flag unset\n%s", got, out)
				}
			}
		})
	}
}

// ── the payload gate, and the two halves of it ──────────────────────────────

// 🔴 THE MOCK HOST'S OWN GATE, REACHED. Both fixtures above send a one-element
// `sources` array, so neither exercises `Array.isArray(sources) && sources.length`
// — the rule `createMockHost` implements and warns about dropping. This app's gate
// logic is CORRECT, so it passes phase 1 and fails only on the payload.
//
// ⚠ IT IS ALSO THE CASE THAT SEPARATES THIS ARM FROM A GATE TEST. An arm that only
// read the Post button's disabled state would grade this app `yes`.
func TestThePostArmRefusesAPayloadTheHostWouldRefuse(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPostOracle(t, browser, fxPostEmptySourcesApp, postPathArm)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no — this app posts an EMPTY sources list, which the host "+
			"refuses with `no images to post`\n%s", got, out)
	}
	if got := assertionField(t, out, "postGateAfterImagelessRun"); got != "true" {
		t.Fatalf("postGateAfterImagelessRun=%q, want true — this fixture's GATE is correct and the "+
			"`no` must come from the payload, not from phase 1\n%s", got, out)
	}
	if got := assertionField(t, out, "postArmPosts"); got != "refused:no-sources" {
		t.Fatalf("postArmPosts=%q, want \"refused:no-sources\" — a different entry means some other "+
			"rule fired, and `createMockHost`'s own array gate was never reached\n%s", got, out)
	}
	if !strings.Contains(out, "the host REFUSED the post this block sent") {
		t.Fatalf("the reason does not name the refusal:\n%s", out)
	}
}

// 🔴 A POST CONTROL THAT DOES NOTHING, WHICH IS THE OTHER WAY TO SHIP A BROKEN
// POST PATH. Every gate reads correctly; the click sends no request at all. This is
// the reachability case for the "sent NO CREATE_POST_FROM_APP" branch, and the
// reason phase 2 clicks Post instead of stopping at the gate's disabled state.
func TestThePostArmSeesAPostControlWiredToNothing(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPostOracle(t, browser, fxPostWiredToNothingApp, postPathArm)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no\n%s", got, out)
	}
	if got := assertionField(t, out, "postArmPosts"); got != "none" {
		t.Fatalf("postArmPosts=%q, want \"none\" — the click must have sent nothing\n%s", got, out)
	}
	if !strings.Contains(out, "sent NO CREATE_POST_FROM_APP") {
		t.Fatalf("the reason does not name the missing request:\n%s", out)
	}
}

// 🔴 AND THE OVER-STRICT APP, WHICH IS WHY PHASE 1 ALONE IS NOT THE ARM. Its gate
// never opens, so it passes the image-less phase without handling the hazard at
// all.
func TestThePostArmFailsAnAppThatCanNeverPost(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPostOracle(t, browser, fxPostNeverOpensApp, postPathArm)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no\n%s", got, out)
	}
	if got := assertionField(t, out, "postGateAfterImagelessRun"); got != "true" {
		t.Fatalf("postGateAfterImagelessRun=%q, want true — phase 1 passes vacuously here, which is "+
			"the point of the case\n%s", got, out)
	}
	if !strings.Contains(out, "never opened after a workflow that reached") {
		t.Fatalf("the reason does not name the closed gate:\n%s", out)
	}
}

// ── the invariant: the arm's allowlist is still an allowlist ────────────────

// 🔴 THE ARM WIDENS THE ANSWERED SET BY FOUR TYPES AND BY NOTHING ELSE, CHECKED
// BEHAVIOURALLY FROM INSIDE THE PAGE. A top-up purchase, a workflow cancel and a
// bare image publish are all reachable from a generate-then-post app and none of
// them is needed to reach Post, so all three must still reject with the SDK's own
// error.
//
// 🔴 WATCHED TO FAIL, AND THE FIRST SHAPE OF THIS GUARD SURVIVED THE SAME MUTATION.
// Replacing the shim's `Promise.reject(new Error(STUB))` with a `Promise.resolve`
// answers every unlisted request — and the ledger-only version of this test stayed
// GREEN, because the shim pushes onto `refused` before returning either way. The
// verdict assertion below is what kills it: the fixture shuts its own Post gate when
// any probe resolves, so `RENDER` moves to `no`. See `fxPostArmSpendProbeApp`.
func TestThePostArmStillRefusesEveryRequestOutsideItsLedger(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPostOracle(t, browser, fxPostArmSpendProbeApp, postPathArm)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "yes" {
		t.Fatalf("RENDER=%s, want yes — this fixture is the correct app plus three spend-adjacent "+
			"requests it EXPECTS to be refused with the SDK's own error, and it SHUTS ITS OWN POST "+
			"GATE when any of them resolves (or rejects with a different message). A `no` here means "+
			"the arm's allowlist has stopped being an allowlist; the page's `ANSWERED:` marker in the "+
			"reported body names which type\n%s", got, out)
	}
	refused := assertionField(t, out, "hostRefused")
	for _, want := range []string{"OPEN_BUZZ_PURCHASE", "CANCEL_WORKFLOW", "PUBLISH_GENERATION_OUTPUTS"} {
		if !strings.Contains(refused, want) {
			t.Fatalf("hostRefused=%q does not name %s — either the arm has started ANSWERING it, or "+
				"the fixture never attempted it (both make this guard vacuous)\n%s", refused, want, out)
		}
	}
	// The positive control for the list above: the four types the arm DOES answer
	// must be on the answered side, or `hostRefused` naming three types would be
	// satisfied by an arm that answers nothing at all.
	answered := assertionField(t, out, "hostAnswered")
	for _, want := range []string{"ESTIMATE_WORKFLOW", "SUBMIT_WORKFLOW", "POLL_WORKFLOW", "CREATE_POST_FROM_APP"} {
		if !strings.Contains(answered, want) {
			t.Fatalf("hostAnswered=%q does not name %s — the arm is not answering its own "+
				"allowlist, so the refusals above prove nothing\n%s", answered, want, out)
		}
	}
}

// 🔴 THE CEILING RIDES ON EVERY CELL, BECAUSE THIS IS THE ARM WHOSE GREEN IS
// EASIEST TO OVER-READ. It answers a submit and creates a post, so "the post path
// works" is one careless sentence away from "posting works on civitai.com". A
// reader must not be able to get the verdict without the sentence.
func TestThePostArmStatesItsCeilingOnEveryCell(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct{ name, app string }{
		{"on a pass", fxPostGateCorrectApp},
		{"on a fail", fxPostGateBlindApp},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runPostOracle(t, browser, tc.app, postPathArm)
			ceiling := assertionField(t, out, "postArmCeiling")
			for _, want := range []string{
				"Does NOT prove the real host accepts it",
				"No credential was used and nothing left the page",
			} {
				if !strings.Contains(ceiling, want) {
					t.Fatalf("the cell's postArmCeiling does not say %q:\n%s", want, ceiling)
				}
			}
			// And the human-readable half, on the run itself rather than only inside
			// the assertion's JSON.
			if !strings.Contains(out, "⚠ POST-PATH ARM:") {
				t.Fatalf("the run does not carry the arm's banner:\n%s", out)
			}
		})
	}
	// The mirror half: a DEFAULT-arm cell must not carry the field at all. A
	// `postArmCeiling` on every cell is exactly how a reader stops noticing which
	// arm they are looking at.
	t.Run("and not on a default-arm cell", func(t *testing.T) {
		out, _ := runPostOracle(t, browser, fxPostGateCorrectApp)
		if strings.Contains(out, "postArmCeiling") || strings.Contains(out, "POST-PATH ARM") {
			t.Fatalf("a default-arm run carries the post arm's own fields:\n%s", out)
		}
	})
}

// ── an incoherent arm combination, without a browser ────────────────────────

// 🔴 REFUSED BEFORE A BROWSER IS STARTED, AND IT EXITS 2 RATHER THAN EMITTING A
// VERDICT. Every arm is selected by an AMBIENT environment variable, so both
// combinations below are one stale export away on any operator's shell — and each
// would produce a confident `no` about nothing: the post arm drives a generation on
// a token an unconsented arm has emptied, and `NO_HOST_PICKS` removes the very shim
// the post arm answers with.
//
// 🔴 THIS CASE NEVER LAUNCHES A BROWSER, DELIBERATELY. The conflict is checked
// before `launch()`, so `CIVITAI_CHROME` only has to name a file that exists — and
// that makes this guard one that RUNS on a machine with no Chromium, where the rest
// of this file skips. A skipped guard on an ambient-flag hazard is a green that
// checked nothing.
func TestThePostArmRefusesAnIncoherentArmCombination(t *testing.T) {
	browserStandIn := dogfoodTool(t, "bash")
	for _, tc := range []struct {
		name   string
		env    []string
		clause string
	}{
		{"with the unconsented arm", []string{postPathArm, unconsentedArm},
			"are both set"},
		{"with the picker control arm", []string{postPathArm, "CIVITAI_ASSERT_NO_HOST_PICKS=1"},
			"only thing this arm answers with"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runPostOracle(t, browserStandIn, fxPostGateCorrectApp, tc.env...)
			if code != 2 {
				t.Fatalf("exit %d, want 2 — an incoherent arm must report that NOTHING was measured, "+
					"never a verdict about the block\n%s", code, out)
			}
			if !strings.Contains(out, "the assertion could not run (exit 2)") {
				t.Fatalf("the run does not say the assertion could not run:\n%s", out)
			}
			// 🔴 THE EXIT CODE ALONE IS NOT DISCRIMINATING HERE, AND THAT WAS MEASURED
			// RATHER THAN REASONED. At the PR's base this same case ALSO exits 2 —
			// because the stand-in browser is `bash`, which the assertion then tries to
			// launch and fails. So the exit code is satisfied for a completely unrelated
			// reason, and only these two assertions separate them: the refusal must name
			// the conflict, and it must have happened BEFORE any launch was attempted.
			if !strings.Contains(out, tc.clause) {
				t.Fatalf("the refusal does not name the conflict (%q):\n%s", tc.clause, out)
			}
			if strings.Contains(out, "browser never printed a DevTools endpoint") {
				t.Fatalf("the conflict was detected only AFTER a browser launch was attempted — the "+
					"whole point of checking it at import time is that an incoherent arm costs nothing "+
					"and cannot be mistaken for a browser problem:\n%s", out)
			}
			// 🔴 AND NOT A VERDICT ANYWHERE ON THE STREAM. `fatal` must have run before
			// any summary line was printed, or a downstream reader takes the `RENDER=`
			// off it regardless of the exit code.
			if len(summaryLines(out)) != 0 {
				t.Fatalf("a summary line was printed for a run that measured nothing:\n%s", out)
			}
		})
	}
}

// ── the shim and the canned shapes, without a browser ───────────────────────

// The node driver for the unit-level guards on this arm. No browser, no network,
// no CLI: it imports `_cdp.mjs` and exercises pure functions, printing one JSON
// line per check so a failure names itself.
func writePostUnitDriver(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(cdpModule)
	if err != nil {
		t.Fatal(err)
	}
	// The SDK's full outbound vocabulary, copied whole from @civitai/app-sdk
	// `dist/blocks/messages.d.ts` — the same ledger the picker and consent drivers
	// use. A guard that listed only the types someone thought of would pass while a
	// new spend-adjacent request quietly fell through to an answered branch; this one
	// fails if the answered set grows OR shrinks against the whole vocabulary.
	ledger := []string{
		"BLOCK_HELLO", "BLOCK_READY", "BLOCK_ERROR", "BLOCK_MESSAGE_REJECTED", "REQUEST_TOKEN",
		"RESIZE_IFRAME", "SUBMIT_WORKFLOW", "ESTIMATE_WORKFLOW", "POLL_WORKFLOW", "CANCEL_WORKFLOW",
		"OPEN_BUZZ_PURCHASE", "GET_BUZZ_BALANCE", "GET_VIEWER", "GET_BUZZ_TRANSACTIONS",
		"GET_BUZZ_ACCOUNTS", "GET_DAILY_COMPENSATION", "GET_WILDCARD_PACK", "QUERY_APP_WORKFLOWS",
		"PUBLISH_GENERATION_OUTPUTS", "CREATE_POST_FROM_APP", "GET_IMAGES_BY_IDS",
		"CANCEL_APP_WORKFLOW", "OPEN_CHECKPOINT_PICKER", "OPEN_RESOURCE_PICKER", "OPEN_IMAGE_UPLOAD",
		"SET_USER_CHECKPOINT", "NAVIGATE", "REQUEST_SIGN_IN", "REQUEST_CONSENT", "TRACK_EVENT",
		"APP_STORAGE_GET", "APP_STORAGE_SET", "APP_STORAGE_DELETE", "APP_STORAGE_LIST",
		"APP_STORAGE_QUOTA", "SHARED_LIST", "SHARED_GET_COUNT", "SHARED_GET_COUNTS", "SHARED_APPEND",
		"SHARED_VOTE", "SHARED_UNVOTE", "SHARED_WITHDRAW", "SHARED_UPDATE", "SHARED_GET",
		"SHARED_REPORT", "SAVE_IMAGE", "SET_COLLECTION_FOLLOW",
	}
	ledgerJSON, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	body := `import { inlineHostSource, INLINE_STUB_MESSAGE, INLINE_HOST_GLOBAL,
  HOST_PICKER_REQUESTS, HOST_POST_REQUESTS, HOST_ANSWERED_REQUESTS, POST_ARM_GLOBAL,
  POST_ARM_PLAN, POST_ARM_COST, POST_ARM_IMAGE, POST_ARM_CREATE_POST_RESULT,
  POST_ARM_NO_IMAGES_ERROR, POST_ARM_SPENT_ACCOUNT, POST_PATH, HOST_ARM, ARM_CONFLICT
} from ` + jsQuote(abs) + `;

const out = [];
const check = (name, ok, detail) => out.push({ name, ok: !!ok, detail: String(detail ?? '') });
const src = inlineHostSource();

check('arm-label-matches-the-env', HOST_ARM === (POST_PATH ? 'post' : 'consented'),
  HOST_ARM + ' / ' + POST_PATH);
check('no-conflict-in-this-invocation', ARM_CONFLICT === null, String(ARM_CONFLICT));

if (!POST_PATH) {
  // 🔴 THE FLAG-UNSET CLAIM, MECHANICALLY. The shim source must not MENTION any of
  // the four request types, must install no ledger, and must carry neither the
  // canned post nor the canned image — which together are what make "the default
  // arm is the source it always was" checkable rather than a reading of branches.
  for (const needle of ['ESTIMATE_WORKFLOW', 'SUBMIT_WORKFLOW', 'POLL_WORKFLOW',
                        'CREATE_POST_FROM_APP', POST_ARM_GLOBAL, 'data:image',
                        String(POST_ARM_CREATE_POST_RESULT.postId)]) {
    check('default-arm-source-omits:' + needle, !src.includes(needle), 'present');
  }
  check('default-arm-answers-only-the-pickers',
    HOST_ANSWERED_REQUESTS.join(',') === HOST_PICKER_REQUESTS.join(','),
    HOST_ANSWERED_REQUESTS.join(','));
  check('default-arm-adds-no-post-types', HOST_POST_REQUESTS.length === 0,
    HOST_POST_REQUESTS.join(','));
} else {
  check('post-arm-answers-pickers-plus-four',
    HOST_ANSWERED_REQUESTS.join(',') ===
      HOST_PICKER_REQUESTS.concat(['ESTIMATE_WORKFLOW', 'SUBMIT_WORKFLOW', 'POLL_WORKFLOW',
                                   'CREATE_POST_FROM_APP']).join(','),
    HOST_ANSWERED_REQUESTS.join(','));
}

// 🔴 NOTHING CAN REACH THE NETWORK, ON EITHER ARM, AND THE CANNED IMAGE IS WHERE
// THAT WOULD HAVE LEAKED. createMockHost's default output is a placehold.co URL;
// a block rendering it makes a real request from the page.
check('canned-image-is-a-data-uri', POST_ARM_IMAGE.startsWith('data:image/'),
  POST_ARM_IMAGE.slice(0, 24));
check('canned-image-reaches-no-host', !/^https?:/i.test(POST_ARM_IMAGE) &&
  !POST_ARM_IMAGE.includes('placehold'), POST_ARM_IMAGE.slice(0, 40));

// ── the canned shapes are the SDK's mock host's, asserted as LITERALS ────────
// 🔴 LITERALS, NOT A RE-DERIVATION. These values exist to make an app that works
// in dev:harness behave identically here; an expectation computed from the same
// constants would agree with any value at all.
check('canned-post-is-the-mock-hosts',
  POST_ARM_CREATE_POST_RESULT.postId === 4242 &&
  POST_ARM_CREATE_POST_RESULT.url === 'https://civitai.com/posts/4242' &&
  POST_ARM_CREATE_POST_RESULT.imageIds.join(',') === '9101,9102',
  JSON.stringify(POST_ARM_CREATE_POST_RESULT));
check('canned-cost-is-the-mock-hosts', POST_ARM_COST === 8, POST_ARM_COST);
check('canned-pool-is-the-mock-hosts', POST_ARM_SPENT_ACCOUNT === 'yellow', POST_ARM_SPENT_ACCOUNT);
check('refusal-is-the-sdks-closed-code', POST_ARM_NO_IMAGES_ERROR === 'no images to post',
  POST_ARM_NO_IMAGES_ERROR);
// The plan's two phases, in order: the image-less success FIRST (the users' case),
// then the postable one.
check('plan-is-imageless-then-postable',
  POST_ARM_PLAN.length === 2 &&
  POST_ARM_PLAN[0].status === 'succeeded' && POST_ARM_PLAN[0].imageUrls.length === 0 &&
  POST_ARM_PLAN[1].status === 'succeeded' && POST_ARM_PLAN[1].imageUrls.length === 1,
  JSON.stringify(POST_ARM_PLAN.map((p) => p.label + ':' + p.imageUrls.length)));

// ── the shim answers exactly its allowlist, over the whole vocabulary ────────
const win = {};
new Function('window', src)(win);
const host = win[INLINE_HOST_GLOBAL];
check('shim-installed', typeof host === 'function', typeof host);
const answered = [], refused = [], wrongError = [];
for (const type of ` + string(ledgerJSON) + `) {
  try {
    await host({ type, payload: { resourceType: 'Checkpoint', requestId: 'r-' + type,
                                  sources: [{ kind: 'image', imageId: 1 }],
                                  body: { kind: 'textToImage' } } });
    answered.push(type);
  } catch (e) {
    refused.push(type);
    if (e.message !== INLINE_STUB_MESSAGE) wrongError.push(type + ':' + e.message);
  }
}
const want = [...HOST_ANSWERED_REQUESTS].sort().join(',');
check('answers-exactly-the-allowlist', answered.slice().sort().join(',') === want,
  'answered=' + answered.slice().sort().join(',') + ' want=' + want);
check('refuses-with-the-sdk-error', wrongError.length === 0, wrongError.join(' '));
check('refused-the-rest', refused.length === ` + jsonLen(ledger) + ` - HOST_ANSWERED_REQUESTS.length,
  refused.length + ' of ' + ` + jsonLen(ledger) + `);

// ── the workflow lifecycle and the post gate, end to end in-process ─────────
if (POST_PATH) {
  const fresh = {};
  new Function('window', inlineHostSource())(fresh);
  const h = fresh[INLINE_HOST_GLOBAL];
  const est = await h({ type: 'ESTIMATE_WORKFLOW', payload: { body: { kind: 'textToImage' } } });
  check('estimate-carries-a-numeric-price',
    est.snapshot.workflowId === 'wf_estimate' && est.snapshot.status === 'pending' &&
    est.snapshot.cost.total === POST_ARM_COST, JSON.stringify(est));
  const run = async (body) => {
    const s = await h({ type: 'SUBMIT_WORKFLOW', payload: { body, idempotencyKey: 'k' } });
    const p = await h({ type: 'POLL_WORKFLOW',
      payload: { workflowId: s.snapshot.workflowId, waitSeconds: 15 } });
    return { submitted: s.snapshot, terminal: p.snapshot };
  };
  const first = await run({ kind: 'textToImage' });
  check('submit-is-pending-then-poll-is-terminal',
    first.submitted.status === 'pending' && first.terminal.status === 'succeeded',
    JSON.stringify([first.submitted.status, first.terminal.status]));
  check('first-run-succeeds-with-no-images', first.terminal.imageUrls.length === 0,
    JSON.stringify(first.terminal.imageUrls.length));
  check('terminal-snapshot-carries-the-mock-hosts-fields',
    first.terminal.cost.total === POST_ARM_COST &&
    first.terminal.spentAccountType === POST_ARM_SPENT_ACCOUNT,
    JSON.stringify(first.terminal));
  // 🔴 GATE 2, THE REAL HOST'S HALF: a non-empty sources array naming a workflow
  // that produced nothing. This is the defect ab-img-poster@0.1.1 shipped and the
  // one createMockHost's array gate CANNOT see.
  const refusedPost = await h({ type: 'CREATE_POST_FROM_APP',
    payload: { sources: [{ kind: 'workflow', workflowId: first.terminal.workflowId }] } });
  check('post-on-an-image-less-workflow-is-refused',
    refusedPost.error === POST_ARM_NO_IMAGES_ERROR && refusedPost.result === undefined,
    JSON.stringify(refusedPost));
  // 🔴 GATE 1, THE MOCK HOST'S OWN: not an array, or an empty one.
  for (const [label, sources] of [['empty', []], ['absent', undefined], ['not-an-array', 'wf_1']]) {
    const r = await h({ type: 'CREATE_POST_FROM_APP', payload: { sources } });
    check('post-with-' + label + '-sources-is-refused',
      r.error === POST_ARM_NO_IMAGES_ERROR, JSON.stringify(r));
  }
  const second = await run({ kind: 'textToImage', accountType: 'blue' });
  check('second-run-succeeds-with-an-image', second.terminal.imageUrls.length === 1,
    JSON.stringify(second.terminal.imageUrls.length));
  check('the-picked-pool-is-echoed', second.terminal.spentAccountType === 'blue',
    second.terminal.spentAccountType);
  const okPost = await h({ type: 'CREATE_POST_FROM_APP',
    payload: { sources: [{ kind: 'workflow', workflowId: second.terminal.workflowId }],
               title: 'x' } });
  check('post-on-a-postable-workflow-is-answered',
    okPost.error === undefined && okPost.result.postId === POST_ARM_CREATE_POST_RESULT.postId,
    JSON.stringify(okPost));
  // 🔴 AND A REPLY IS A COPY, NOT THE CONSTANT. A block that mutated the result it
  // was handed would otherwise silently rewrite what every later cell is graded
  // against.
  okPost.result.postId = -1;
  const again = await h({ type: 'CREATE_POST_FROM_APP',
    payload: { sources: [{ kind: 'workflow', workflowId: second.terminal.workflowId }] } });
  check('the-canned-post-cannot-be-mutated-by-the-block',
    again.result.postId === POST_ARM_CREATE_POST_RESULT.postId, JSON.stringify(again.result));
  // An id this shim never minted is answered TERMINALLY rather than left polling to
  // watch()'s ten-minute deadline, and it is recorded.
  const unknown = await h({ type: 'POLL_WORKFLOW', payload: { workflowId: 'wf_invented' } });
  check('an-unknown-workflow-is-answered-terminally', unknown.snapshot.status === 'expired',
    JSON.stringify(unknown));
  const arm = fresh[POST_ARM_GLOBAL];
  check('the-ledger-records-both-runs-and-every-post',
    arm.submits === 2 && arm.terminal === 2 && arm.posts.length === 6 &&
    arm.unknownPolls.join(',') === 'wf_invented',
    JSON.stringify(arm));
}

for (const r of out) console.log(JSON.stringify(r));
`
	path := filepath.Join(dir, "post-unit.mjs")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 🔴 RED AT THE PR'S BASE: the driver imports `HOST_POST_REQUESTS`,
// `HOST_ANSWERED_REQUESTS`, `POST_ARM_GLOBAL`, `POST_ARM_PLAN`, `POST_ARM_COST`,
// `POST_ARM_IMAGE`, `POST_ARM_CREATE_POST_RESULT`, `POST_ARM_NO_IMAGES_ERROR`,
// `POST_ARM_SPENT_ACCOUNT`, `POST_PATH` and `ARM_CONFLICT`, none of which exist
// there, so the import fails and every check is MISSING rather than merely wrong.
//
// ⚠ BOTH ARMS ARE RUN, because `POST_PATH` is read from the environment at module
// load: a driver run only with the flag set could not tell an arm that is off from
// one that does nothing, and a driver run only with it unset could not see the arm
// at all.
func TestThePostArmShimAnswersOnlyItsOwnLedger(t *testing.T) {
	node := dogfoodTool(t, "node")
	for _, arm := range []struct {
		name    string
		env     []string
		minimum int
	}{
		// 6 source-omission checks + 2 ledger checks + 4 network/image + 5 shape
		// + 3 vocabulary + 2 arm = 22, so 18 leaves room for nothing being quietly
		// dropped while still failing loudly on an early exit.
		{"the flag unset (default arm)", nil, 18},
		// …and with it set, the lifecycle block adds 15 more.
		{"the flag set (post arm)", []string{postPathArm}, 26},
	} {
		t.Run(arm.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "dogfood-post-unit-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			driver := writePostUnitDriver(t, dir)

			cmd := exec.Command(node, driver, dir)
			// 🔴 A BARE ENVIRONMENT, for the reason #704 added `STUB_CIVITAI`: a test arm
			// once reached the operator's REAL account through a leaked PATH. This driver
			// calls pure functions and evaluates the shim with `new Function`, in-process,
			// so it is given no PATH, no HOME and no credential — which makes "it cannot
			// reach the network or the real CLI" a property of the invocation rather than
			// a claim about the code.
			cmd.Env = append([]string{"PATH=/nonexistent"}, arm.env...)
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("running the post unit driver: %v\n%s", err, out)
			}
			if code != 0 {
				t.Fatalf("the driver exited %d — the checks below were never run\n%s", code, out)
			}
			type result struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
			}
			seen := 0
			for _, line := range strings.Split(out, "\n") {
				if !strings.HasPrefix(line, "{") {
					continue
				}
				var r result
				if err := json.Unmarshal([]byte(line), &r); err != nil {
					t.Fatalf("driver line is not JSON: %q", line)
				}
				seen++
				if !r.OK {
					t.Errorf("%s: %s", r.Name, r.Detail)
				}
			}
			// 🔴 A MINIMUM COUNT, because a driver that died halfway prints only the
			// checks it reached and every one of them can be green.
			if seen < arm.minimum {
				t.Fatalf("only %d checks reported, want at least %d — the driver stopped early\n%s",
					seen, arm.minimum, out)
			}
		})
	}
}
