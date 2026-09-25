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

// The UNCONSENTED ARM of the render oracle, and the reason it has its own file.
//
// # What happened
//
// `ab-ship-mimo-02` was graded `RENDER=yes` by this oracle, submitted, approved,
// and deployed to https://ab-img-poster.civit.ai/. It then FAILED FOR A REAL
// USER on the first click: Generate produced `Generation failed. Please try
// again.` and the operator had to find "review permissions" by hand. Measured in
// `dogfood-ab-ship-mimo-02` at `/work/ab-img-poster/src/App.jsx` —
//
//	await estimate(body);                 // ~line 70, no consent asked, ever
//	const snap = await submit(body);
//	…
//	} catch (err) {
//	  if (err?.signInRequired) requestSignIn();
//	  else if (err?.declined) { /* … */ }
//	  else setError('Generation failed. Please try again.');   // ← the user's screen
//	}
//
// — `requestConsent` appears once in the file, inside `handlePost`, for
// `posts:write:self`, which is unreachable until a generation has succeeded.
//
// # WHY THE ORACLE COULD NOT SEE IT — THREE THINGS COMPOUNDING
//
//  1. `InlineTransport.sendRequest` rejects EVERY request, so "generation fails
//     for everyone" and "generation works" leave the identical status trace
//     `ready>generating>ready`. The predicate grades the status word.
//  2. #690 seeds `token.scopes` from the manifest and #708 answers resource
//     picks. Both fixed real false negatives — and TOGETHER they mean an app that
//     never asks for consent is indistinguishable from one that asks correctly,
//     because the harness has already granted what the ask was for.
//  3. Every new user starts UNCONSENTED. That is the default state, and the
//     oracle graded only the already-consented path.
//
// # THIS IS THE FOURTH INSTANCE OF ONE CLASS, WITH THE SIGN FLIPPED
//
//	#686  no signed-in viewer seeded   -> auth-gated apps graded a branch
//	                                      production never exhibits
//	#690  `token.scopes: []` seeded    -> consent-gated apps could never reach
//	                                      `generating`
//	#708  no resource pick answered    -> picker-gated apps could never reach
//	                                      `generating`
//	here  ONLY the consented state     -> consent-BLIND apps were never asked to
//	      was ever presented             handle the state every user starts in
//
// The first three were false NEGATIVES: the harness presented less than a host
// does. This is a false POSITIVE, and its fix is the mirror image — present the
// state a host presents FIRST, before anything is granted.
//
// # THE INVARIANT, AND WHY THIS ARM IS STRICTLY SAFER THAN THE DEFAULT ONE
//
// The arm REMOVES capability and adds none. It seeds `token.scopes: []` (the
// value `_cdp.mjs` seeded unconditionally before #690), `token.raw` is still
// `''`, and it answers no new request type — it only WATCHES `sendMessage`,
// which `InlineTransport` already implements as a no-op. There is no reply
// channel on that path to grant anything with: the SDK documents
// `REQUEST_CONSENT` as fire-and-forget, and `InlineTransport.onMessage` returns a
// no-op unsubscribe, so a `TOKEN_REFRESH` could not be delivered even if this
// oracle invented one. `TestTheUnconsentedArmGrantsTheBlockNothing` and
// `TestTheMessageLedgerAnswersNothingAtAll` are what hold that line.

// ── fixtures ─────────────────────────────────────────────────────────────────

// The SDK's v1 inline stub with BOTH no-ops, VERBATIM from
// @civitai/blocks-react@0.57.1 `dist/transport/inlineTransport.js`.
//
// 🔴 IT IS NOT `fxSdkInlineStub` PLUS A LINE. The published `sendMessage` body is
// a COMMENT, not an empty `{}` — which is exactly the shape a needle written
// against the minified `sendMessage(r){}` would miss, and the reason the message
// needle accepts a comment-only body rather than an empty one. Paraphrasing this
// would be the "scanners allowlist their own canonical examples" failure: the
// fixture would be a case the patch is allowed to match instead of the real one
// it must match.
const fxSdkInlineStubWithMessage = `class InlineTransport {
    sendMessage(_message) {
        // v2 will invoke platform APIs directly; intentional no-op in v1.
    }
    sendRequest(_request, _responseType, _opts) {
        return Promise.reject(new Error('InlineTransport.sendRequest is not implemented in v1'));
    }
}
const transport = new InlineTransport();
`

// `useRequestConsent().requestConsent`, reduced to its wire behaviour, VERBATIM
// in shape from blocks-react `src/hooks/useRequestConsent.ts`:
//
//	transport.sendMessage({ type: 'REQUEST_CONSENT', ...(payload ? { payload } : {}) });
//
// 🔴 A `sendMessage`, NOT A `sendRequest`, AND THAT IS THE WHOLE FINDING. #708's
// shim instruments `sendRequest`; a consent ask has never gone anywhere near it.
const fxSdkConsent = `function requestConsent(payload) {
  transport.sendMessage({ type: 'REQUEST_CONSENT', ...(payload ? { payload } : {}) });
}
function grantedScopes() {
  const b = window.__CIVITAI_BLOCK_CONTEXT__ || {};
  return ((b.token || {}).scopes) || [];
}
`

// The markup every fixture below renders, so a difference between two of them is
// a difference in BEHAVIOUR rather than in test ids.
const fxConsentMarkup = `
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
`

// 🔴 THE DEFECT FIXTURE: `ab-ship-mimo-02`, reduced to the three lines that
// matter. It never asks for consent, it drives the status machine optimistically,
// and its catch produces the user's actual screen. On the CONSENTED arm it is
// indistinguishable from a correct app — `ready>generating>ready`, `RENDER=yes` —
// which is the verdict the real trial earned before it shipped and broke.
const fxConsentBlindApp = fxSdkInlineStubWithMessage + fxSdkConsent + fxConsentMarkup + `
genEl.onclick = async function () {
  statusEl.textContent = 'generating';
  try {
    await transport.sendRequest({ type: 'ESTIMATE_WORKFLOW', payload: {} }, 'WORKFLOW_ESTIMATE');
    statusEl.textContent = 'spend-completed';
  } catch (e) {
    statusEl.textContent = 'ready';
    errEl.textContent = 'Generation failed. Please try again.';
  }
};
`

// 🔴 THE CORRECT APP: `ab-genpost-dsv4-01`'s shape. It reads the token's scopes,
// and when the one it needs is missing it asks the host INSTEAD of spending.
// Measured live on that trial (2026-09-25): on the unconsented arm it sends
// `REQUEST_CONSENT:ai:write:budgeted` and refuses no workflow request at all,
// because it never attempts one.
const fxConsentFirstApp = fxSdkInlineStubWithMessage + fxSdkConsent + fxConsentMarkup + `
genEl.onclick = async function () {
  if (grantedScopes().indexOf('ai:write:budgeted') === -1) {
    requestConsent({ scopes: ['ai:write:budgeted'] });
    return;
  }
  statusEl.textContent = 'generating';
  try {
    await transport.sendRequest({ type: 'ESTIMATE_WORKFLOW', payload: {} }, 'WORKFLOW_ESTIMATE');
    statusEl.textContent = 'spend-completed';
  } catch (e) { statusEl.textContent = 'ready'; }
};
`

// 🔴 THE OTHER CORRECT SHAPE, AND THE ONE THE DEFAULT ARM'S STILL-DISABLED THROW
// WOULD FAIL. Generate is SHUT until the scope arrives — which on this arm it
// never can, because a grant needs a `TOKEN_REFRESH` push and inline mode
// receives none — and a separate control does the asking. An arm that reached the
// default arm's `is still disabled with the prompt typed` throw would fail
// exactly the apps that handle the unconsented state best.
const fxConsentGatedDisabledApp = fxSdkInlineStubWithMessage + fxSdkConsent + fxConsentMarkup + `
const granted = grantedScopes().indexOf('ai:write:budgeted') !== -1;
const grantEl = document.createElement('button');
grantEl.textContent = 'Allow image generation';
root.appendChild(grantEl);
function sync() { genEl.disabled = !(granted && promptEl.value.trim()); }
promptEl.addEventListener('input', sync);
sync();
grantEl.onclick = function () { requestConsent({ scopes: ['ai:write:budgeted'] }); };
genEl.onclick = function () { statusEl.textContent = 'generating'; };
`

// 🔴 THE INVARIANT FIXTURE FOR THIS ARM. It asks for consent — so it reaches the
// arm's predicate — and then attempts a SUBMIT anyway. Its error element carries
// `spend-completed` if the submit was ANSWERED and `refused-with:<message>` if it
// was refused with anything other than the SDK's own error, so a harness that had
// started answering a money request on the unconsented arm is named rather than
// merely failing. A consent GRANT is what stands between a block and a spend, so
// this is the arm where that must be checked, not assumed.
const fxConsentThenSpendApp = fxSdkInlineStubWithMessage + fxSdkConsent + fxConsentMarkup + `
genEl.onclick = async function () {
  requestConsent({ scopes: ['ai:write:budgeted'] });
  try {
    await transport.sendRequest({ type: 'SUBMIT_WORKFLOW', payload: {} }, 'WORKFLOW_SUBMITTED');
    statusEl.textContent = 'spend-completed';
  } catch (e) {
    statusEl.textContent = e.message === 'InlineTransport.sendRequest is not implemented in v1'
      ? 'refused-by-the-sdk' : 'refused-with:' + e.message;
  }
};
`

// 🔴 THE BOOTSTRAP PROBE FOR THIS ARM, and it asserts LITERALS rather than
// anything recomputed from the source the oracle reads. Run with
// `fxManifestScoped`, which declares two scopes: on the unconsented arm the block
// must be shown NEITHER, and `raw` must still be empty. A probe that recomputed
// the expected list would agree with an arm that silently did nothing.
const fxUnconsentedBootstrapProbeApp = fxSdkInlineStubWithMessage + fxSdkConsent + fxConsentMarkup + `
genEl.onclick = function () {
  const b = window.__CIVITAI_BLOCK_CONTEXT__ || {};
  const t = b.token || {}, v = b.viewer;
  const bad = [];
  if (!t.scopes || t.scopes.length !== 0) bad.push('scopes:' + JSON.stringify(t.scopes));
  if (t.raw !== '') bad.push('token-raw-nonempty');
  // 🔴 THE VIEWER IS STILL SIGNED IN. "Unconsented" is not "anonymous": the SDK's
  // consent path exists for a LOGGED-IN viewer whose token lacks a scope, and an
  // anonymous arm would grade the sign-in CTA branch instead (#686's defect).
  if (!v || v.signedIn !== true) bad.push('viewer:' + JSON.stringify(v));
  // 🔴 THE VERDICT GOES IN THE STATUS ELEMENT AND THE ASK IS WITHHELD ON A BAD
  // BOOTSTRAP, so a broken arm fails the run AND names the field that is wrong in
  // "observed" — the same mechanism fxGenpostBootstrapProbe and
  // fxPickShapeProbeApp use. Asserting a Go-side expectation instead would let a
  // recomputed expectation agree with a broken oracle.
  if (bad.length) { statusEl.textContent = bad.join(','); return; }
  requestConsent({ scopes: ['ai:write:budgeted'] });
};
`

// 🔴 A `sendMessage` THE NEEDLE CANNOT INSTALL ON, AND A REALISTIC ONE: a body
// with a real statement in it, which is what a v2 implementation of the method
// looks like. The needle deliberately refuses a non-empty body — it INSERTS into
// the no-op rather than REPLACING the body, so matching a real implementation
// would mean instrumenting code that does something. The stub message is still in
// the file (`sendRequest` is untouched), so `messageUnmatched` fires: the
// instrument saying "an inline transport went past me and I could not watch it".
const fxUninstrumentableMessageStub = `class InlineTransport {
    sendMessage(_message) {
        this._outbox.push(_message);
    }
    sendRequest(_request, _responseType, _opts) {
        return Promise.reject(new Error('InlineTransport.sendRequest is not implemented in v1'));
    }
}
const transport = new InlineTransport();
transport._outbox = [];
`

// 🔴 THE UNMEASURED FIXTURE: an app that DOES ask, on a bundle this oracle cannot
// watch. Without the `consentBlind` branch it grades `RENDER=no` — byte-identical
// to the verdict the DEFECT earns, and attributed to the model. That is this
// change's own defect, silently reintroduced by somebody else's build tool.
const fxUninstrumentableConsentFirstApp = fxUninstrumentableMessageStub + fxSdkConsent + fxConsentMarkup + `
genEl.onclick = function () {
  if (grantedScopes().indexOf('ai:write:budgeted') === -1) {
    requestConsent({ scopes: ['ai:write:budgeted'] });
    return;
  }
  statusEl.textContent = 'generating';
};
`

// 🔴 THE OVER-REFUSAL CONTROL, and it is what keeps `consentBlind` from being a
// blanket refusal. The SAME unwatchable bundle, and an app that fails for a
// reason an unobserved message cannot produce: it has no Post control at all. The
// blindness harmed nothing here, so it must still earn an ordinary verdict, with
// the blindness merely REPORTED on the cell.
const fxUninstrumentableNoPostApp = fxUninstrumentableMessageStub + fxSdkConsent + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="gen">Generate</button>' +
  '<div data-testid="status">ready</div>';
document.getElementById('gen').onclick = function () {
  document.querySelector('[data-testid="status"]').textContent = 'generating';
};
`

// 🔴 THE FIXTURE THAT MAKES `consentBlind`'s NARROWNESS REACHABLE, and it exists
// because a mutation SURVIVED a fully green sweep without it (M5, 2026-09-25).
//
// It bundles NO SDK AT ALL — no inline transport, so `messageSites` is 0 and
// `messageUnmatched` is 0 — and it never asks for consent. That is the COMMON,
// HARMLESS shape: a hand-written block, a `static` template, anything not built on
// `@civitai/blocks-react`. It must earn an ordinary `no`, because the instrument
// was not blind; there was simply nothing to instrument.
//
// ⚠ WHY NO OTHER ROW REACHES IT. Widening `consentBlind` from
// `messageUnmatched > 0` to `messageSites === 0` — the exact confusion
// `pickerBlind`'s docblock warns against ("refusing those would trade one wrong
// verdict for a hundred") — changes NOTHING for any other fixture here: the ones
// that ask have `sites=1`, the consent-blind one has `sites=1`, and the
// over-refusal control throws at the Post gate before the consent branch is ever
// reached. Only a bundle with no transport at all separates the two predicates,
// and until this fixture existed the suite had none. The live fixture
// `ab-genpost-glm-01` is in exactly this state (`messageShim: sites=0`) and does
// not reach it either, because it fails earlier for a render reason.
const fxNoSdkNeverAsksApp = `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="gen">Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const statusEl = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = function () {
  statusEl.textContent = 'generating';
  setTimeout(function () { statusEl.textContent = 'ready'; }, 10);
};
`

// ── helpers ──────────────────────────────────────────────────────────────────

// Runs the oracle over a consent fixture served as a built app with an external
// module bundle — the shape every real built block has, and the one that makes
// the patch's `Script` interception path the one under test.
func runConsentOracle(t *testing.T, browser, manifest, appJS string, extraEnv ...string) (string, int) {
	t.Helper()
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
		manifest: manifest, outputDir: "dist", appHTML: fxExternalModuleHTML,
		appFiles: map[string]string{"app.js": appJS},
	}), "CIVITAI_CHROME="+browser)
	env = append(env, extraEnv...)
	return runScript(t, "oracle.sh", env, "ctl", "root")
}

// The one env assignment that selects the arm. Named rather than spelled at eight
// call sites: it is also the string `stubOracleEnv` clears, and the two must not
// drift.
const unconsentedArm = "CIVITAI_ASSERT_UNCONSENTED=1"

// ── the regression: the same app, two arms, two verdicts ────────────────────

// 🔴 THE HEADLINE TEST, AND IT IS ONE FIXTURE IN TWO ARMS. The consent-blind app
// — `ab-ship-mimo-02` reduced — must still grade `yes` on the DEFAULT arm,
// because that arm's question ("did the Generate click drive the machine to
// `generating`") is answered correctly by it and no existing verdict may move.
// The same fixture must grade `no` on the UNCONSENTED arm, because the question
// there is "did the block ask", and it never does.
//
// RED AT `origin/main`: the env var does not exist there, so both rows grade
// `yes` and the second fails with `RENDER=yes, want no`. Watched, 2026-09-25.
//
// ⚠ The pair is the test. Either row alone is satisfiable by an oracle that
// always says the same thing.
func TestTheSameAppPassesConsentedAndFailsUnconsented(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		env        []string
		wantRender string
		wantArm    string
		wantAsked  string
	}{
		{
			name:       "the consented arm still grades the status machine (must not move)",
			wantRender: "yes", wantArm: "consented", wantAsked: "false",
		},
		{
			name:       "the unconsented arm grades the ask, and it never happened",
			env:        []string{unconsentedArm},
			wantRender: "no", wantArm: "unconsented", wantAsked: "false",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runConsentOracle(t, browser, fxManifestScoped, fxConsentBlindApp, tc.env...)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			// 🔴 POSITIVE CONTROL ON THE FIXTURE TREE. One row expects `no`, and so
			// does a run that never found the app — without this the arm could pass
			// for a reason that has nothing to do with consent.
			if got := summaryField(t, out, "app_dirs"); got != "1" {
				t.Fatalf("app_dirs=%s, want 1 — the fixture app was never found\n%s", got, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
			if got := summaryField(t, out, "arm"); got != tc.wantArm {
				t.Fatalf("arm=%s, want %s — a cell that does not name its arm cannot be read\n%s",
					got, tc.wantArm, out)
			}
			if got := assertionField(t, out, "consentRequested"); got != tc.wantAsked {
				t.Fatalf("consentRequested=%s, want %s\n%s", got, tc.wantAsked, out)
			}
			// 🔴 THE STATE THE ARM IS ABOUT, NOT A WORD IN THE REASON. Both rows must
			// have driven the app all the way through its generate path — the status
			// machine reaching `generating` and coming back is what proves the click
			// landed and the spend was refused. Without this, an unconsented `no`
			// earned by never clicking anything would look identical.
			if got := assertionField(t, out, "observed"); got != "ready>generating>ready" {
				t.Fatalf("observed=%q, want \"ready>generating>ready\" — this fixture's Generate "+
					"click must have run on BOTH arms, or the `no` is about the driving\n%s", got, out)
			}
			if got := assertionField(t, out, "hostRefused"); !strings.Contains(got, "ESTIMATE_WORKFLOW") {
				t.Fatalf("hostRefused=%q, want it to name ESTIMATE_WORKFLOW — this app spends "+
					"without asking, and that refusal is the evidence it tried\n%s", got, out)
			}
		})
	}
}

// 🔴 AND THE ARM MUST PASS THE APP THAT DOES IT RIGHT, OR IT IS JUST A HARNESS
// THAT SAYS `no`. Two correct shapes, because they exercise DIFFERENT branches of
// the arm: one keeps Generate live and asks from the click handler
// (`ab-genpost-dsv4-01`), the other SHUTS Generate until the scope arrives and
// asks from a separate control.
//
// RED AT `origin/main` for both rows: the arm does not exist there, so `scopes`
// are seeded from the manifest, the first row takes its granted branch and
// reports `consentRequested` — a field that does not exist — while the second
// row's Generate opens and no ask is ever made. Watched, 2026-09-25.
func TestTheUnconsentedArmPassesAnAppThatAsks(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name     string
		app      string
		wantMsgs string
		// Whether Generate was clickable. Reported by the assertion and asserted
		// here because it is what separates the two shapes: a row where both
		// reported the same value would be one shape tested twice.
		wantClicked string
	}{
		{
			name: "asks from the Generate click, Generate stays live",
			app:  fxConsentFirstApp, wantMsgs: "REQUEST_CONSENT:ai:write:budgeted",
			wantClicked: "true",
		},
		{
			name: "shuts Generate until granted and asks from another control",
			app:  fxConsentGatedDisabledApp, wantMsgs: "REQUEST_CONSENT:ai:write:budgeted",
			wantClicked: "false",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runConsentOracle(t, browser, fxManifestScoped, tc.app, unconsentedArm)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := summaryField(t, out, "RENDER"); got != "yes" {
				t.Fatalf("RENDER=%s, want yes — this app asks for consent on an unconsented "+
					"token, which is the whole thing this arm grades\n%s", got, out)
			}
			if got := assertionField(t, out, "hostMessages"); got != tc.wantMsgs {
				t.Fatalf("hostMessages=%q, want %q — the ledger is the evidence, and the scopes "+
					"HINT is part of it (a bare requestConsent() can never receive a "+
					"CONSENT_UNAVAILABLE)\n%s", got, tc.wantMsgs, out)
			}
			if got := assertionField(t, out, "generateClicked"); got != tc.wantClicked {
				t.Fatalf("generateClicked=%s, want %s — the two rows are supposed to exercise "+
					"different branches of the arm, and if they agree here they do not\n%s",
					got, tc.wantClicked, out)
			}
			// 🔴 NEITHER SHAPE MAY HAVE SPENT. An app that asks first must not also
			// have fired a workflow request; if it did, the arm would be passing an app
			// for asking while it charged ahead anyway.
			if got := assertionField(t, out, "hostRefused"); strings.Contains(got, "WORKFLOW") {
				t.Fatalf("hostRefused=%q — a consent-first app must not have attempted a "+
					"workflow request at all\n%s", got, out)
			}
		})
	}
}

// ── the invariant: the arm grants the block nothing ─────────────────────────

// 🔴 WHAT THE BLOCK IS ACTUALLY SHOWN ON THIS ARM, ASSERTED FROM INSIDE THE PAGE.
// The manifest declares two scopes; the block must be shown NEITHER, `token.raw`
// must still be empty, and the viewer must still be SIGNED IN — "unconsented" is
// not "anonymous", and an arm that quietly went anonymous would be grading #686's
// sign-in CTA branch instead.
//
// RED AT `origin/main`: the arm does not exist, so `t.scopes` holds the
// manifest's two entries and the probe writes `scopes:["ai:write:budgeted",…]`.
// Watched, 2026-09-25.
func TestTheUnconsentedArmGrantsTheBlockNothing(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runConsentOracle(t, browser, fxManifestScoped, fxUnconsentedBootstrapProbeApp, unconsentedArm)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "yes" {
		t.Fatalf("RENDER=%s, want yes\n%s", got, out)
	}
	// The probe's own verdict, out of the DOM the assertion saw. `ready` means it
	// withheld nothing; anything else is the list of bootstrap fields that
	// disagree, so a failure names which half is wrong.
	if got := assertionField(t, out, "observed"); got != "ready" {
		t.Fatalf("observed=%q, want \"ready\" — anything else is the in-page probe naming a "+
			"bootstrap field that is not what this arm claims to present\n%s", got, out)
	}
	// The same fact from the other side of the process boundary. `oracle.sh` has
	// its own guard on this pair; asserting it here too is what makes the guard's
	// own failure visible as a test rather than as an exit 2 nobody reads.
	if got := assertionField(t, out, "hostScopes"); got != "none" {
		t.Fatalf("hostScopes=%q, want \"none\"\n%s", got, out)
	}
	if got := assertionField(t, out, "hostScopesDeclared"); got != "ai:write:budgeted,posts:write:self" {
		t.Fatalf("hostScopesDeclared=%q — the manifest's declaration must still cross the "+
			"process boundary intact, or the arm's emptying cannot be distinguished from an "+
			"assertion that ignored its argument\n%s", got, out)
	}
}

// 🔴 AND A CONSENT ASK BUYS NO SPEND. The fixture asks, and then tries a submit
// anyway; the arm must still refuse it with the SDK's own error. This is the arm
// where the check belongs — a granted scope is precisely what stands between a
// block and a spend, so an arm that is ABOUT consent is the one where "we did not
// accidentally grant anything" has to be measured rather than promised.
//
// ⚠ AN INVARIANT GUARD, NOT A REGRESSION TEST: refusing a submit is what
// pre-change code did for every request. Watched to fail by mutation (M3), not by
// `origin/main`.
func TestAConsentAskBuysNoSpendOnTheUnconsentedArm(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runConsentOracle(t, browser, fxManifestScoped, fxConsentThenSpendApp, unconsentedArm)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "yes" {
		t.Fatalf("RENDER=%s, want yes — the fixture asks for consent\n%s", got, out)
	}
	if got := assertionField(t, out, "hostRefused"); got != "SUBMIT_WORKFLOW" {
		t.Fatalf("hostRefused=%q, want \"SUBMIT_WORKFLOW\" — the spend must still have been "+
			"refused on this arm\n%s", got, out)
	}
	if got := assertionField(t, out, "observed"); got != "ready>refused-by-the-sdk" {
		t.Fatalf("observed=%q, want \"ready>refused-by-the-sdk\" — `spend-completed` means the "+
			"submit was ANSWERED, and `refused-with:…` means it was refused with a message this "+
			"harness invented rather than the SDK's own\n%s", got, out)
	}
}

// ── a blind instrument reports UNMEASURED, never `no` ───────────────────────

// 🔴 THE GUARD FOR THE SILENT INSTRUMENT FAILURE, one axis over from #708's. If
// the `sendMessage` needle stops matching — a bundler reshapes the method, the
// SDK implements it for real — a consent ask is never RECORDED, and the
// unconsented arm reads `no`: byte-identical to the verdict the DEFECT earns, and
// attributed to the model.
//
// ⚠ Both rows are required, and the second is what keeps it from being a blanket
// refusal: an app that fails for a reason an unobserved message cannot produce
// must still earn an ordinary verdict on the very same unwatchable bundle.
func TestABlindMessageInstrumentReportsUnmeasuredNotNo(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name     string
		app      string
		wantCode int
		wantLine string
		// The `messageShim` substring this row's bundle must produce, and whether it
		// must be free of `unmatched=`. Defaults (empty / false) keep the two blind
		// rows asserting exactly what they asserted before the no-SDK row existed.
		wantShim    string
		noUnmatched bool
		wantReason  string
	}{
		{
			name: "an ask that could not be observed is UNMEASURED",
			app:  fxUninstrumentableConsentFirstApp, wantCode: 2,
			wantLine: "nothing was measured",
		},
		{
			name: "the same blind bundle, an app failing for another reason (over-refusal control)",
			app:  fxUninstrumentableNoPostApp, wantCode: 0,
			wantLine: "RENDER=no",
		},
		{
			// 🔴 THE SECOND OVER-REFUSAL CONTROL, AND THE ONE A MUTATION FOUND
			// MISSING. No SDK at all, so nothing was instrumentable and nothing was
			// blind. It must earn an ordinary `no` — see fxNoSdkNeverAsksApp for why
			// no other row can see the difference.
			name: "no SDK at all: nothing to instrument is not blindness",
			app:  fxNoSdkNeverAsksApp, wantCode: 0,
			wantLine: "RENDER=no",
			wantShim: "sites=0", noUnmatched: true,
			wantReason: "never asked the host for consent",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runConsentOracle(t, browser, fxManifestScoped, tc.app, unconsentedArm)
			if code != tc.wantCode {
				t.Fatalf("exit %d, want %d\n%s", code, tc.wantCode, out)
			}
			if !strings.Contains(out, tc.wantLine) {
				t.Fatalf("the run does not say %q\n%s", tc.wantLine, out)
			}
			// 🔴 POSITIVE CONTROL ON THE FIXTURE ITSELF. The first two rows are about a
			// bundle the message needle could NOT match; if it matched after all, the
			// first would pass for the wrong reason and the second would test nothing.
			// The third row's control is the MIRROR: it must show no `unmatched=` at
			// all, because "nothing to instrument" and "could not instrument" are the
			// two states the predicate has to keep apart.
			shim := assertionField(t, out, "messageShim")
			wantShim := tc.wantShim
			if wantShim == "" {
				wantShim = "sites=0"
			}
			if !strings.Contains(shim, wantShim) {
				t.Fatalf("messageShim=%q, want it to contain %q\n%s", shim, wantShim, out)
			}
			if tc.noUnmatched {
				if strings.Contains(shim, "unmatched") {
					t.Fatalf("messageShim=%q — this row's bundle carries NO inline transport, so "+
						"an `unmatched` count means the fixture is not in the state the row is "+
						"about\n%s", shim, out)
				}
			} else if !strings.Contains(shim, "unmatched=1") {
				t.Fatalf("messageShim=%q, want unmatched=1 — the fixture's sendMessage is supposed "+
					"to defeat the needle, and if it no longer does then this row is not about a "+
					"blind instrument\n%s", shim, out)
			}
			if tc.wantReason != "" {
				r, _ := assertionLine(t, out)["reason"].(string)
				if !strings.Contains(r, tc.wantReason) {
					t.Fatalf("reason = %q, want it to contain %q — an ordinary `no` here must be "+
						"about the consent predicate, not about the instrument\n%s", r, tc.wantReason, out)
				}
			}
			if tc.wantCode == 2 {
				if got := assertionField(t, out, "unmeasured"); got != "true" {
					t.Fatalf("unmeasured=%s, want true\n%s", got, out)
				}
				if got := summaryFieldOrEmpty(out, "RENDER"); got != "" {
					t.Fatalf("the run emitted RENDER=%s — an unmeasured run must emit NO verdict "+
						"line at all, or a reader folds it back into `no`\n%s", got, out)
				}
			} else if tc.wantReason == "" {
				// The first over-refusal row's `no` must be about the missing Post
				// control, not about consent — otherwise `consentBlind` has swallowed it
				// after all and the row is measuring the opposite of what it claims.
				r, _ := assertionLine(t, out)["reason"].(string)
				if !strings.Contains(r, `labelled "post"`) {
					t.Fatalf("reason = %q, want the missing Post control\n%s", r, out)
				}
			}
		})
	}
}

// ── the source patch and the ledger, without a browser ──────────────────────

// The node driver for the unit-level guards on the message half. No browser, no
// network, no CLI: it imports `_cdp.mjs` and exercises pure functions, printing
// one JSON line per check so a failure names itself.
func writeConsentUnitDriver(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(cdpModule)
	if err != nil {
		t.Fatal(err)
	}
	// 🔴 SEVEN REAL SPELLINGS OF ONE METHOD, AND EVERY ONE WAS MEASURED OUT OF A
	// FILE RATHER THAN IMAGINED. Five trial bundles (blocks-react 0.53.1 and
	// 0.57.x, two different minifiers), the published `dist`, and the TypeScript
	// source. The published `dist` is the one that decides the needle's shape: its
	// no-op body is a COMMENT, so a needle written against the minified `{}` would
	// patch nothing there while reporting an ordinary green.
	spellings := map[string]string{
		"minified-0.57.2":   `sendMessage(r){}sendRequest(r,d,s){return Promise.reject(new Error("InlineTransport.sendRequest is not implemented in v1"))}`,
		"minified-0.53.1":   "sendMessage(e){}sendRequest(e,t,n){return Promise.reject(Error(`InlineTransport.sendRequest is not implemented in v1`))}",
		"published-dist":    "    sendMessage(_message) {\n        // v2 will invoke platform APIs directly; intentional no-op in v1.\n    }\n    sendRequest(_request, _responseType, _opts) {\n        return Promise.reject(new Error('InlineTransport.sendRequest is not implemented in v1'));\n    }",
		"typescript-source": "  sendMessage(_message: BlockToParentMessage): void {\n    // v2 will invoke platform APIs directly; intentional no-op in v1.\n  }\n\n  sendRequest(\n    _request: OutboundRequest,\n    _responseType: ParentToBlockMessageType,\n    _opts?: { timeoutMs?: number },\n  ): Promise<unknown> {\n    return Promise.reject(new Error('InlineTransport.sendRequest is not implemented in v1'));\n  }",
	}
	spellingsJSON, err := json.Marshal(spellings)
	if err != nil {
		t.Fatal(err)
	}
	// 🔴 SHAPES THE NEEDLE MUST NOT TOUCH, AND THE FIRST TWO ARE THE SAFETY
	// PROPERTY. A `sendMessage` with a real body is either another class's (the
	// iframe transport posts a message) or a v2 implementation of this one;
	// instrumenting either would be inserting a call into code that does
	// something, on a patch whose whole licence is that it is inserting into a
	// no-op.
	nonTargets := map[string]string{
		"a real body (iframe transport)": `sendMessage(m){this.post(m)}sendRequest(a,b,c){return Promise.reject(new Error("InlineTransport.sendRequest is not implemented in v1"))}`,
		"a v2 implementation":            "sendMessage(r){ this.x(r); }\nsendRequest(a,b,c){return Promise.reject(new Error(\"InlineTransport.sendRequest is not implemented in v1\"))}",
		"no stub anywhere near":          `sendMessage(r){}sendRequest(r,d,s){return Promise.reject(new Error("something else"))}`,
	}
	nonTargetsJSON, err := json.Marshal(nonTargets)
	if err != nil {
		t.Fatal(err)
	}
	// The SDK's full outbound vocabulary, copied whole from
	// @civitai/app-sdk `dist/blocks/messages.d.ts` — the same ledger
	// dogfood_oracle_picker_test.go pins the ANSWERED set against. Here it is used
	// for the opposite claim: the message path answers NONE of them.
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
	body := `import { patchInlineTransport, blockMessageSource, INLINE_STUB_MESSAGE,
  INLINE_MESSAGE_GLOBAL, CONSENT_MESSAGE, UNCONSENTED, HOST_ARM, hostBootstrap, seededScopes
} from ` + jsQuote(abs) + `;

const out = [];
const check = (name, ok, detail) => out.push({ name, ok: !!ok, detail: String(detail ?? '') });

// ── the needle finds every real spelling, and only no-ops ────────────────────
for (const [name, src] of Object.entries(` + string(spellingsJSON) + `)) {
  const r = patchInlineTransport(src);
  check('spelling-patched:' + name, r.messageHits === 1, r.messageHits + ' hits');
}
for (const [name, src] of Object.entries(` + string(nonTargetsJSON) + `)) {
  const r = patchInlineTransport(src);
  check('non-target-untouched:' + name, r.messageHits === 0, r.messageHits + ' hits');
}
// A stub 500 characters away is out of the anchor's reach, which is what keeps
// the needle from wandering into an unrelated class in a big bundle.
{
  const far = 'sendMessage(r){}' + 'x'.repeat(500) + 'Promise.reject(new Error("' + INLINE_STUB_MESSAGE + '"))';
  check('non-target-untouched:too-far', patchInlineTransport(far).messageHits === 0, '');
}

// ── the patched method still returns undefined, and delegates ────────────────
const MIN = 'export const t = { sendMessage(_m) {}, ' +
  'sendRequest(a, b, c) { return Promise.reject(new Error("' + INLINE_STUB_MESSAGE + '")); } };';
const patched = patchInlineTransport(MIN);
check('module-stub-patched', patched.messageHits === 1 && patched.hits === 1,
  'message=' + patched.messageHits + ' request=' + patched.hits);
const { writeFile } = await import('node:fs/promises');
const { join } = await import('node:path');
const p = join(process.argv[2], 'patched-messages.mjs');
await writeFile(p, patched.code);
const { t } = await import('file://' + p);
// No recorder installed: byte-identical behaviour to the SDK as published.
check('no-op-without-a-recorder', t.sendMessage({ type: 'REQUEST_CONSENT' }) === undefined, '');

// ── the recorder watches, and answers NOTHING ────────────────────────────────
const win = {};
new Function('window', blockMessageSource())(win);
globalThis[INLINE_MESSAGE_GLOBAL] = win[INLINE_MESSAGE_GLOBAL];
check('recorder-installed', typeof win[INLINE_MESSAGE_GLOBAL] === 'function',
  typeof win[INLINE_MESSAGE_GLOBAL]);
// 🔴 THE INVARIANT, OVER THE SDK'S WHOLE OUTBOUND VOCABULARY. The picker shim's
// ledger test asserts which requests it ANSWERS; this asserts that the message
// path answers NONE — every type returns undefined, nothing is a thenable, and
// nothing throws. A message path that started returning a value would be a reply
// channel, which is the one thing this arm must not have.
const answered = [], threw = [];
for (const type of ` + string(ledgerJSON) + `) {
  let r;
  try { r = t.sendMessage({ type, payload: { scopes: ['ai:write:budgeted'] } }); }
  catch (e) { threw.push(type + ':' + e.message); continue; }
  if (r !== undefined) answered.push(type + ':' + JSON.stringify(r));
}
check('answers-nothing-at-all', answered.length === 0, answered.join(' '));
check('throws-at-nothing', threw.length === 0, threw.join(' '));
check('recorded-every-type', (win.__dogfoodBlockMessages || []).length === ` + jsonLen(ledger) + `,
  (win.__dogfoodBlockMessages || []).length + ' of ' + ` + jsonLen(ledger) + `);
// The hint is part of the ledger entry, because a bare requestConsent() can never
// receive a CONSENT_UNAVAILABLE and a cell that could not tell the two apart
// would hide that.
check('records-the-scopes-hint',
  (win.__dogfoodBlockMessages || []).includes(CONSENT_MESSAGE + ':ai:write:budgeted'),
  JSON.stringify((win.__dogfoodBlockMessages || []).slice(0, 3)));
// A malformed message must not throw into the block: sendMessage was a guaranteed
// no-op before the patch, and a recorder that threw would make it a throw.
let hostile = 'ok';
try {
  t.sendMessage({ get type() { throw new Error('boom'); } });
} catch (e) { hostile = 'threw:' + e.message; }
check('a-hostile-message-cannot-throw-into-the-block', hostile === 'ok', hostile);
delete globalThis[INLINE_MESSAGE_GLOBAL];

// ── the arm's own seed ───────────────────────────────────────────────────────
const declared = ['ai:write:budgeted', 'posts:write:self'];
const seeded = seededScopes(declared);
const bootstrap = hostBootstrap(declared);
check('arm-label-matches-the-env', HOST_ARM === (UNCONSENTED ? 'unconsented' : 'consented'),
  HOST_ARM + ' / ' + UNCONSENTED);
check('seed-matches-the-arm',
  UNCONSENTED ? seeded.length === 0 : seeded.join(',') === declared.join(','),
  HOST_ARM + ' -> ' + JSON.stringify(seeded));
// 🔴 ONE SOURCE, TWO READINGS. If these could disagree, the cell's hostScopes
// field would describe a list the block never received — the seam oracle.sh's
// scope guard exists to catch, reintroduced one level down.
check('seed-is-the-bootstrap-token',
  seeded.join(',') === bootstrap.token.scopes.join(','),
  JSON.stringify(seeded) + ' vs ' + JSON.stringify(bootstrap.token.scopes));
// 🔴 AND IT NEVER BUYS A CREDENTIAL, ON EITHER ARM.
check('token-raw-stays-empty', bootstrap.token.raw === '', JSON.stringify(bootstrap.token.raw));
// Unconsented is not anonymous: the consent path is for a LOGGED-IN viewer.
check('viewer-is-still-signed-in',
  !!bootstrap.viewer && bootstrap.viewer.signedIn === true, JSON.stringify(bootstrap.viewer));

for (const r of out) console.log(JSON.stringify(r));
`
	path := filepath.Join(dir, "consent-unit.mjs")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 🔴 RED AT `origin/main`: the driver imports `blockMessageSource`,
// `INLINE_MESSAGE_GLOBAL`, `CONSENT_MESSAGE`, `UNCONSENTED`, `HOST_ARM` and
// `seededScopes`, none of which exist there, so the import fails and every check
// is MISSING rather than merely wrong.
//
// ⚠ BOTH ARMS ARE RUN, because `UNCONSENTED` is read from the environment at
// module load: a driver run only in the default arm could not tell an arm that
// empties the seed from one that does nothing at all.
func TestTheMessageLedgerAnswersNothingAtAll(t *testing.T) {
	node := dogfoodTool(t, "node")
	for _, arm := range []struct {
		name string
		env  []string
	}{
		{"consented (default)", nil},
		{"unconsented", []string{unconsentedArm}},
	} {
		t.Run(arm.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "dogfood-consent-unit-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			driver := writeConsentUnitDriver(t, dir)

			cmd := exec.Command(node, driver, dir)
			// 🔴 A BARE ENVIRONMENT, for the reason #704 added `STUB_CIVITAI`: a test
			// arm once reached the operator's real account through a leaked PATH. This
			// driver calls pure functions and needs no PATH, no HOME and no credential,
			// so it is given none — which makes "it cannot reach the network or the real
			// CLI" a property of the invocation rather than a claim about the code.
			cmd.Env = append([]string{"PATH=/nonexistent"}, arm.env...)
			raw, err := cmd.CombinedOutput()
			out := string(raw)
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatalf("running the consent unit driver: %v\n%s", err, out)
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
			// checks it reached and every one of them can be green. 18 checks: 4
			// spellings, 4 non-targets, 1 module patch, 1 no-op, 6 on the recorder, and
			// 6 on the seed — 22, so 18 leaves room for nothing being quietly dropped
			// while still failing loudly on an early exit.
			if seen < 18 {
				t.Fatalf("only %d checks reported, want at least 18 — the driver stopped early\n%s",
					seen, out)
			}
		})
	}
}
