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

// The HOST RESOURCE PICKER half of the render oracle, and the reason it has its
// own file.
//
// # What happened
//
// Measured on trial `ab-ship-mimo-02` (2026-09-25). The app opened the host's
// resource picker from a `Select Model` button —
//
//	const picked = await openPicker({ resourceType: 'Checkpoint' })   // src/App.jsx:36
//	disabled={isGenerating || !prompt.trim() || !model}               // src/App.jsx:154
//
// — and gated Generate on the pick. The oracle's host emulation is the SDK's
// `InlineTransport`, whose `sendRequest` rejects unconditionally and which
// delivers no host pushes, so `openPicker` never resolved, `model` stayed null,
// Generate stayed disabled, and the cell read
// `RENDER=no observed=ready generateDisabled=true`. Nothing in the app was wrong:
// asking the host for a pick is what the SDK's own docs prescribe and is a BETTER
// app than the hardcoded checkpoint the passing cells shipped.
//
// # THIS IS THE THIRD INSTANCE OF ONE CLASS
//
//	#686  the oracle seeded no signed-in viewer  -> auth-gated apps graded a
//	      branch production never exhibits
//	#690  the oracle seeded `token.scopes: []`   -> consent-gated apps could
//	      never reach `generating`
//	here  the oracle answered no resource pick   -> picker-gated apps could
//	      never reach `generating`
//
// Each time, an app that correctly gated on something the PLATFORM supplies was
// graded as broken because the harness supplied nothing. The fix is the same
// shape: present what a real host presents.
//
// # THE INVARIANT, AND WHERE IT IS ACTUALLY CHECKED
//
// `token.raw` stays empty, and every request that is not one of the two picker
// types still rejects with the SDK's own
// `InlineTransport.sendRequest is not implemented in v1`. A pick buys a block a
// BRANCH, never a capability. ⚠ Read that precisely: the SDK's stub no longer
// rejects *unconditionally* in a patched page — it rejects for everything except
// `OPEN_RESOURCE_PICKER` and `OPEN_CHECKPOINT_PICKER`, which are host DISCOVERY
// calls that hand back an id the author could have hardcoded. What survives
// untouched is the property that matters: nothing here can complete a generation,
// a post or a purchase. `TestOracleRefusesEveryRequestThatIsNotAPick` and
// `TestOracleInlineHostAnswersOnlyThePickerLedger` are what hold that line, and
// the second one fails if the allowlist either GROWS or SHRINKS.

// ── fixtures ─────────────────────────────────────────────────────────────────

// The SDK's v1 inline stub, VERBATIM from
// @civitai/blocks-react@0.57.2 `dist/transport/inlineTransport.js`, wrapped in the
// smallest class that can hold it.
//
// 🔴 VERBATIM MATTERS. The oracle finds this stub by its own error string, so a
// fixture that paraphrased it would be a textbook case the patch is allowed to
// match rather than the real one it must match — the "scanners allowlist their
// own canonical examples and scan clean" failure. Two of the tests below are red
// the moment this text and the published SDK's diverge, which is the point.
const fxSdkInlineStub = `class InlineTransport {
  sendRequest(_request, _responseType, _opts) {
      return Promise.reject(new Error('InlineTransport.sendRequest is not implemented in v1'));
  }
}
const transport = new InlineTransport();
`

// `useResourcePicker().open` and `useCheckpointPicker().open`, reduced to their
// wire behaviour: one `sendRequest` with the request type the SDK sends, reading
// `selected` off the resolved payload exactly as the SDK does
// (`const { selected } = await sendTypedRequest(...); return selected ?? null`).
const fxSdkPickers = `async function openPicker(opts) {
  const reply = await transport.sendRequest(
    { type: 'OPEN_RESOURCE_PICKER', payload: { resourceType: opts.resourceType } },
    'RESOURCE_PICKER_RESULT', { timeoutMs: 120000 });
  return reply.selected ?? null;
}
async function openCheckpointPicker() {
  const reply = await transport.sendRequest(
    { type: 'OPEN_CHECKPOINT_PICKER', payload: { baseModelGroup: 'Flux1' } },
    'CHECKPOINT_PICKER_RESULT', { timeoutMs: 120000 });
  return reply.selected ?? null;
}
`

// An index.html that loads its bundle as an EXTERNAL MODULE, which is what every
// real built block does and what makes the patch's Script path the one under test.
const fxExternalModuleHTML = `<!doctype html><meta charset="utf-8"><body>
<div id="root"></div>
<script type="module" src="app.js"></script>
</body>`

// 🔴 THE DEFECT FIXTURE: `ab-ship-mimo-02`, reduced. Generate is gated on a
// resource pick the HOST has to answer, and the generation itself goes through the
// transport — so the `spend-completed` branch below is reachable only by a harness
// that has started answering things it must not.
const fxPickerGatedApp = fxSdkInlineStub + fxSdkPickers + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="pick">Select Model</button>' +
  '<button id="gen" disabled>Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const promptEl = document.querySelector('[data-testid="prompt"]');
const statusEl = document.querySelector('[data-testid="status"]');
const genEl = document.getElementById('gen');
const pickEl = document.getElementById('pick');
let model = null;
function sync() { genEl.disabled = !(promptEl.value.trim() && model); }
promptEl.addEventListener('input', sync);
pickEl.onclick = async function () {
  try {
    const picked = await openPicker({ resourceType: 'Checkpoint' });
    if (!picked) return;
    model = picked;
    pickEl.textContent = 'Model: ' + picked.modelName;
    sync();
  } catch (e) { console.error('picker:', e.message); }
};
genEl.onclick = async function () {
  statusEl.textContent = 'generating';
  try {
    await transport.sendRequest(
      { type: 'ESTIMATE_WORKFLOW', payload: { modelVersionId: model.versionId } },
      'WORKFLOW_ESTIMATE');
    statusEl.textContent = 'spend-completed';
  } catch (e) { statusEl.textContent = 'ready'; }
};
`

// 🔴 THE INVARIANT FIXTURE. It needs NO pick: Generate is live on a typed prompt.
// Its status reaches `generating` if and only if a SUBMIT_WORKFLOW was refused
// with the SDK's own error, and it names any other outcome instead of quietly
// passing. So a harness that answered a submit — or refused it with a message of
// its own invention — grades `no` here, with `observed` saying which.
const fxSpendRefusedApp = fxSdkInlineStub + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="gen">Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const statusEl = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = async function () {
  try {
    await transport.sendRequest({ type: 'SUBMIT_WORKFLOW', payload: {} }, 'WORKFLOW_SUBMITTED');
    statusEl.textContent = 'spend-completed';
  } catch (e) {
    statusEl.textContent =
      e.message === 'InlineTransport.sendRequest is not implemented in v1'
        ? 'generating' : 'refused-with:' + e.message;
  }
};
`

// 🔴 THE SEAM GUARD, and it is the picker's counterpart to
// `fxGenpostBootstrapProbe`: what the oracle actually RESOLVES a pick with,
// asserted from inside the page against LITERAL values rather than against
// anything computed from the same source the oracle reads. A probe that recomputed
// the expected object would agree with a broken oracle.
//
// The four things it pins:
//
//   - a Checkpoint pick's key set is EXACTLY `BlockResourceInfo`'s. That interface
//     is the host's own `RESOURCE_PICKER_RESULT` projection in civitai/civitai's
//     `PageBlockHost.tsx`; a wider fake would let a block read a field production
//     never sends and still grade green here.
//   - its VALUES are the SDK's `DEFAULT_CHECKPOINT_PICK`, so the oracle and every
//     other host emulation in the ecosystem hand a block the same resource.
//   - a CHECKPOINT picker pick is the NARROWER five-field `BlockCheckpointInfo`,
//     not the same object twice. The host sends two different projections and a
//     block is entitled to that difference.
//   - an unsupported `resourceType` resolves to a DISMISSAL (`null`), which is what
//     the platform does — its native modal refuses to open outside
//     `BlockResourcePickerType` — rather than to some other resource.
const fxPickShapeProbeApp = fxSdkInlineStub + fxSdkPickers + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="pick">Select Model</button>' +
  '<button id="gen" disabled>Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const promptEl = document.querySelector('[data-testid="prompt"]');
const statusEl = document.querySelector('[data-testid="status"]');
const genEl = document.getElementById('gen');
let ok = false;
function sync() { genEl.disabled = !(promptEl.value.trim() && ok); }
promptEl.addEventListener('input', sync);
document.getElementById('pick').onclick = async function () {
  const bad = [];
  try {
    const r = await openPicker({ resourceType: 'Checkpoint' });
    if (!r) { bad.push('resource-pick-null'); }
    else {
      const keys = Object.keys(r).sort().join('+');
      if (keys !== 'baseModel+clipSkip+maxStrength+minStrength+modelId+modelName+modelType+strength+trainedWords+versionId+versionName') {
        bad.push('resource-keys:' + keys);
      }
      if (r.versionId !== 691639) bad.push('versionId:' + r.versionId);
      if (r.modelId !== 618692) bad.push('modelId:' + r.modelId);
      if (r.modelName !== 'FLUX.1 [dev]') bad.push('modelName:' + r.modelName);
      if (r.versionName !== 'fp8') bad.push('versionName:' + r.versionName);
      if (r.baseModel !== 'Flux.1 D') bad.push('baseModel:' + r.baseModel);
      if (r.modelType !== 'Checkpoint') bad.push('modelType:' + r.modelType);
    }
    const c = await openCheckpointPicker();
    if (!c) { bad.push('checkpoint-pick-null'); }
    else {
      const ckeys = Object.keys(c).sort().join('+');
      if (ckeys !== 'baseModel+modelId+modelName+versionId+versionName') {
        bad.push('checkpoint-keys:' + ckeys);
      }
      if (c.versionId !== 691639) bad.push('checkpoint-versionId:' + c.versionId);
    }
    const lora = await openPicker({ resourceType: 'LORA' });
    if (!lora || lora.modelType !== 'LORA') bad.push('lora-pick:' + JSON.stringify(lora));
    const nope = await openPicker({ resourceType: 'Embedding' });
    if (nope !== null) bad.push('unsupported-type-resolved:' + JSON.stringify(nope));
  } catch (e) {
    bad.push('threw:' + e.message);
  }
  if (bad.length) { statusEl.textContent = bad.join(','); return; }
  ok = true;
  sync();
};
genEl.onclick = function () { statusEl.textContent = 'generating'; };
`

// A picker-gated app with NO Post control — the scaffold shape. It exists to pin
// that answering a pick buys a block nothing it has not built: the Post gate is
// still what separates this brief from a template.
const fxPickerGatedNoPostApp = fxSdkInlineStub + fxSdkPickers + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="pick">Select Model</button>' +
  '<button id="gen" disabled>Generate</button>' +
  '<div data-testid="status">ready</div>';
const promptEl = document.querySelector('[data-testid="prompt"]');
const statusEl = document.querySelector('[data-testid="status"]');
const genEl = document.getElementById('gen');
let model = null;
function sync() { genEl.disabled = !(promptEl.value.trim() && model); }
promptEl.addEventListener('input', sync);
document.getElementById('pick').onclick = async function () {
  model = await openPicker({ resourceType: 'Checkpoint' });
  sync();
};
genEl.onclick = function () { statusEl.textContent = 'generating'; };
`

// 🔴 THE SCOPING FIXTURE. Its PICKER button drives the status machine to
// `generating` and its Generate button does nothing — the shape that a verdict
// keyed on `seq.includes('generating')` would grade GREEN now that the assertion
// clicks a picker affordance with the recorder already live. It must grade `no`,
// and the reason must be about Generate.
//
// ⚠ It cannot be made red on pre-change code: at `origin/main` nothing ever clicks
// the picker, so the status never moves and the verdict is `no` for a different
// reason. It is an INVARIANT GUARD on the new predicate's scoping, and it is here
// because that scoping is otherwise unguarded — mutating `slice(beforeClick)` back
// to `includes` survives every other test in this file.
const fxPickerDrivesStatusApp = fxSdkInlineStub + fxSdkPickers + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="pick">Select Model</button>' +
  '<button id="gen" disabled>Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const promptEl = document.querySelector('[data-testid="prompt"]');
const statusEl = document.querySelector('[data-testid="status"]');
const genEl = document.getElementById('gen');
document.getElementById('pick').onclick = async function () {
  const picked = await openPicker({ resourceType: 'Checkpoint' });
  if (!picked) return;
  statusEl.textContent = 'generating';   // WRONG: the pick is not a generation
  genEl.disabled = false;
};
// 🔴 IT MOVES THE MACHINE, TO A WORD THAT IS NOT 'generating'. An INERT Generate
// makes the post-click wait time out, and a throw from that wait means the
// predicate is never evaluated — so the mutation this fixture exists to catch
// (scoping the predicate back to the whole sequence) SURVIVED a fully green sweep
// until this line existed. Measured: M5, 2026-09-25.
genEl.onclick = function () { statusEl.textContent = 'failed'; };
`

// 🔴 A STUB SPELLING THE NEEDLE CANNOT MATCH, AND IT IS A REALISTIC ONE. The
// message is hoisted into a binding, so the literal is still in the file — which
// is what a minifier must preserve and what `unmatched` keys on — while the
// `Promise.reject(...)` expression no longer carries it. Every needle in
// `patchInlineTransport` matches on the literal SITTING INSIDE the reject call,
// so this patches 0 sites.
//
// ⚠ BUILT FROM A REAL TRANSFORM, NOT A TEXTBOOK ONE. Two real bundles already
// disagreed about this expression (`new Error("…")` in blocks-react 0.57.2,
// “Error(`…`)“ in 0.53.1); constant-hoisting is the ordinary next variant, and
// the point of the fixture is that the harness must not be able to tell which
// unknown spelling it is looking at — only that it could not install.
const fxUnmatchableStub = `const STUB_MESSAGE = 'InlineTransport.sendRequest is not implemented in v1';
class InlineTransport {
  sendRequest(_request, _responseType, _opts) {
      return Promise.reject(new Error(STUB_MESSAGE));
  }
}
const transport = new InlineTransport();
`

// 🔴 THE UNMEASURED FIXTURE: an app that DOES ask for a pick, on a bundle this
// oracle cannot instrument. Without the `pickerBlind` branch it grades
// `RENDER=no` — byte-identical to the verdict `ab-ship-mimo-02` earned and
// attributed to the model — which is this whole PR's defect, silently
// reintroduced by a minifier.
const fxUnmatchablePickerGatedApp = fxUnmatchableStub + fxSdkPickers + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="pick">Select Model</button>' +
  '<button id="gen" disabled>Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const promptEl = document.querySelector('[data-testid="prompt"]');
const statusEl = document.querySelector('[data-testid="status"]');
const genEl = document.getElementById('gen');
let model = null;
function sync() { genEl.disabled = !(promptEl.value.trim() && model); }
promptEl.addEventListener('input', sync);
document.getElementById('pick').onclick = async function () {
  try {
    model = await openPicker({ resourceType: 'Checkpoint' });
    sync();
  } catch (e) { console.error('picker:', e.message); }
};
genEl.onclick = function () { statusEl.textContent = 'generating'; };
`

// 🔴 THE OVER-REFUSAL CONTROL, and it is the half that keeps `pickerBlind` from
// being a blanket refusal. SAME uninstrumentable bundle, but the app needs no
// pick: Generate opens on a typed prompt. The instrument being blind harmed
// nothing here, so this must still earn an ordinary verdict — `RENDER=yes` — with
// the blindness merely REPORTED on the cell.
const fxUnmatchableNoPickApp = fxUnmatchableStub + `
const root = document.getElementById('root');
root.innerHTML = '<input data-testid="prompt" type="text">' +
  '<button id="gen">Generate</button>' +
  '<button id="post" disabled>Post</button>' +
  '<div data-testid="status">ready</div>';
const statusEl = document.querySelector('[data-testid="status"]');
document.getElementById('gen').onclick = async function () {
  statusEl.textContent = 'generating';
  try {
    await transport.sendRequest({ type: 'ESTIMATE_WORKFLOW', payload: {} }, 'WORKFLOW_ESTIMATE');
    statusEl.textContent = 'spend-completed';
  } catch (e) { statusEl.textContent = 'ready'; }
};
`

// ── helpers ──────────────────────────────────────────────────────────────────

// The assertion's one JSON line, parsed. The oracle's summary line carries the
// verdict and `observed`; everything else the assertion measured — which requests
// the host answered, which it refused, what the source patch did — is here.
func assertionLine(t *testing.T, out string) map[string]any {
	t.Helper()
	var last map[string]any
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, `{"assertion"`) {
			continue
		}
		// 🔴 DECODE, DO NOT Unmarshal. On the exit-2 path `oracle.sh`'s `fatal`
		// re-prints the assertion's whole output inside its own message, so the last
		// matching line is the JSON followed by ` — nothing was measured (…)`.
		// `Unmarshal` rejects trailing content; a decoder stops at the end of the
		// first value, which is exactly the object we want.
		m := map[string]any{}
		if err := json.NewDecoder(strings.NewReader(line)).Decode(&m); err != nil {
			t.Fatalf("assertion line is not JSON: %q", line)
		}
		last = m
	}
	if last == nil {
		t.Fatalf("no assertion JSON line in:\n%s", out)
	}
	return last
}

// summaryField without the Fatal: `""` when the oracle emitted no summary line at
// all, which is itself the thing an unmeasured run has to be checked for.
func summaryFieldOrEmpty(out, key string) string {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "brief=") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, key+"=") {
				return strings.TrimPrefix(f, key+"=")
			}
		}
	}
	return ""
}

func assertionField(t *testing.T, out, key string) string {
	t.Helper()
	v, ok := assertionLine(t, out)[key]
	if !ok {
		t.Fatalf("the assertion reported no %q field; it printed:\n%s", key, out)
	}
	if s, isStr := v.(string); isStr {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// Runs the oracle over a picker fixture served as a built app with an external
// module bundle. `extraEnv` carries the control-arm knobs.
func runPickerOracle(t *testing.T, browser, manifest, appJS string, extraEnv ...string) (string, int) {
	t.Helper()
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0", transcript: startRecord("", "genpost"),
		manifest: manifest, outputDir: "dist", appHTML: fxExternalModuleHTML,
		appFiles: map[string]string{"app.js": appJS},
	}), "CIVITAI_CHROME="+browser)
	env = append(env, extraEnv...)
	return runScript(t, "oracle.sh", env, "ctl", "root")
}

// ── the regression: a picker-gated app can reach `generating` ────────────────

// 🔴 RED AT `origin/main`, GREEN HERE. On pre-change code the first row grades
// `RENDER=no` with `the status never read "generating"` and `generateDisabled=true`
// — byte-identical to the verdict `ab-ship-mimo-02` earned, which is the whole
// point of the fixture.
//
// The second row is the CONTROL ARM, and it is what proves the answered pick is
// doing the work rather than something else that changed in the same commit: with
// `CIVITAI_ASSERT_NO_HOST_PICKS=1` the oracle installs no shim and patches no
// bundle, so the identical fixture grades `no`. ⚠ That arm is red on pre-change
// code too, for a DIFFERENT reason (pre-change nothing clicked the picker at all),
// so it is a control rather than a regression test.
func TestOracleAnswersAHostResourcePick(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		env        []string
		wantRender string
		wantObs    string
		wantAnswer string
		wantReason string
	}{
		{
			name: "a picker-gated app is answered a pick and can generate",
			// 🔴 `ready>generating>ready`, NOT `ready>generating>spend-completed`.
			// The fixture's generation goes through the same transport, so this
			// string is also the assertion that the ESTIMATE was refused — a
			// harness that answered everything would pass the verdict and fail
			// here.
			wantRender: "yes", wantObs: "ready>generating>ready",
			wantAnswer: "OPEN_RESOURCE_PICKER:Checkpoint",
		},
		{
			name:       "the same app with the pick unanswered (control)",
			env:        []string{"CIVITAI_ASSERT_NO_HOST_PICKS=1"},
			wantRender: "no", wantAnswer: "none",
			// 🔴 THE CONTROL ARM MUST FAIL FOR ITS OWN REASON. A `no` here is the
			// expected value of a run that never found the app, never started a
			// browser, or crashed — so the arm is only a control if the reason names
			// the gate the pick was supposed to open.
			wantReason: `is still disabled with the prompt typed`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runPickerOracle(t, browser, fxManifestScoped, fxPickerGatedApp, tc.env...)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			// 🔴 POSITIVE CONTROL ON THE FIXTURE TREE. One row expects `no`, and so
			// does a run that never found the app — without this the control arm
			// could pass for a reason that has nothing to do with the picker.
			if got := summaryField(t, out, "app_dirs"); got != "1" {
				t.Fatalf("app_dirs=%s, want 1 — the fixture app was never found, so this arm's "+
					"verdict is about the harness, not the picker\n%s", got, out)
			}
			if got := summaryField(t, out, "RENDER"); got != tc.wantRender {
				t.Fatalf("RENDER=%s, want %s\n%s", got, tc.wantRender, out)
			}
			if got := assertionField(t, out, "hostAnswered"); got != tc.wantAnswer {
				t.Fatalf("hostAnswered=%q, want %q\n%s", got, tc.wantAnswer, out)
			}
			if tc.wantObs != "" {
				if got := assertionField(t, out, "observed"); got != tc.wantObs {
					t.Fatalf("observed=%q, want %q\n%s", got, tc.wantObs, out)
				}
			}
			if tc.wantReason != "" {
				r, _ := assertionLine(t, out)["reason"].(string)
				if !strings.Contains(r, tc.wantReason) {
					t.Fatalf("reason = %q, want it to contain %q\n%s", r, tc.wantReason, out)
				}
			}
		})
	}
}

// 🔴 AND A PICK BUYS A SCAFFOLD NOTHING. The Post gate is what separates this
// brief from a template (see briefs/genpost.md); a harness that answers picks must
// not have widened what counts as a pass. Red at `origin/main` only in the sense
// that it is `no` there too — it is an INVARIANT GUARD on the gate, not a
// regression test, and it is here because the change it guards against is exactly
// the kind a picker fix could make by accident.
func TestAPickDoesNotBuyAScaffoldAPass(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPickerOracle(t, browser, fxManifestScoped, fxPickerGatedNoPostApp)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "app_dirs"); got != "1" {
		t.Fatalf("app_dirs=%s, want 1\n%s", got, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no — answering a pick must not excuse a missing Post gate\n%s", got, out)
	}
	if r, _ := assertionLine(t, out)["reason"].(string); !strings.Contains(r, `labelled "post"`) {
		t.Fatalf("reason = %q, want the missing Post control — a `no` for any other reason means "+
			"this arm stopped measuring the gate\n%s", r, out)
	}
}

// 🔴 AND A PICKER CLICK IS NOT A GENERATION. The assertion now clicks a host
// affordance with the status recorder already running, so the verdict has to be
// scoped to what follows the GENERATE click; otherwise an app whose picker sets
// `generating` and whose Generate does nothing grades green. See
// fxPickerDrivesStatusApp for why this is an invariant guard rather than a
// regression test, and the PR's mutation matrix for it being watched to fail.
func TestOnlyTheGenerateClickCanEarnTheVerdict(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPickerOracle(t, browser, fxManifestScoped, fxPickerDrivesStatusApp)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no — this app's PICKER set `generating` and its Generate does "+
			"nothing, so a green here means the verdict is no longer a claim about Generate\n%s",
			got, out)
	}
	// 🔴 THE STATE, NOT A WORD IN THE REASON. What makes this arm a measurement is
	// that the machine was ALREADY in `generating` when Generate was clicked — the
	// exact condition an unscoped `seq.includes('generating')` would have accepted —
	// and the verdict is still `no`. Asserting the reason's wording instead would be
	// a guard a reword walks past, and there are two legitimate reasons here (the
	// post-click wait times out before the predicate is ever evaluated).
	if got := assertionField(t, out, "statusBeforeClick"); got != "generating" {
		t.Fatalf("statusBeforeClick=%q, want \"generating\" — the fixture's picker is supposed to "+
			"have moved the machine BEFORE the Generate click, and if it did not then this arm "+
			"cannot see the scoping at all\n%s", got, out)
	}
	if got := assertionField(t, out, "observed"); !strings.Contains(got, "generating") {
		t.Fatalf("observed=%q, want it to contain `generating`\n%s", got, out)
	}
	if got := assertionField(t, out, "hostAnswered"); got != "OPEN_RESOURCE_PICKER:Checkpoint" {
		t.Fatalf("hostAnswered=%q — the picker must have been answered for this arm to reach the "+
			"state it is about\n%s", got, out)
	}
}

// ── a blind instrument reports UNMEASURED, never `no` ───────────────────────

// 🔴 THE GUARD FOR THE SILENT INSTRUMENT FAILURE. If `patchInlineTransport`'s
// needle stops matching a bundle — a minifier reshapes the reject expression, the
// SDK rewords its stub — then a pick can never be answered, Generate stays shut,
// the status never leaves `ready`, and the cell reads `RENDER=no`. That is
// byte-identical to the verdict `ab-ship-mimo-02` earned before this PR, and it
// would be attributed to the model: this PR's own defect, reintroduced silently by
// somebody else's build tool.
//
// It is not hypothetical. The needle WAS wrong on the second real bundle anyone
// looked at (blocks-react 0.53.1 emits “Error(`…`)“ with no `new`) and patched 0
// sites while its cell stayed green — only because that app never opens a picker.
//
// `oracle.sh` already owns the right state for this and says so in its own words:
// exit 2 / `RENDER=unmeasured`, because folding an unmeasurable run into `no`
// reports "the model did not build the app" about a run where the INSTRUMENT
// failed. So a pick that could not have been answered must land there.
//
// ⚠ Both arms are required, and the second is the one that keeps this from being a
// blanket refusal: an app that never needed a pick must still earn an ordinary
// verdict on the very same uninstrumentable bundle.
func TestABlindPickerInstrumentReportsUnmeasuredNotNo(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name     string
		app      string
		wantCode int
		wantLine string
	}{
		{
			name: "a pick that could not be answered is UNMEASURED",
			app:  fxUnmatchablePickerGatedApp, wantCode: 2,
			wantLine: "nothing was measured",
		},
		{
			name: "the same blind bundle, an app that needs no pick (over-refusal control)",
			app:  fxUnmatchableNoPickApp, wantCode: 0,
			wantLine: "RENDER=yes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runPickerOracle(t, browser, fxManifestScoped, tc.app)
			if code != tc.wantCode {
				t.Fatalf("exit %d, want %d\n%s", code, tc.wantCode, out)
			}
			if !strings.Contains(out, tc.wantLine) {
				t.Fatalf("the run does not say %q\n%s", tc.wantLine, out)
			}
			// 🔴 POSITIVE CONTROL ON THE FIXTURE ITSELF. Both arms are about a
			// bundle the needle could NOT match; if it matched after all, the first
			// arm would pass for the wrong reason (an ordinary `no` is not exit 2,
			// but a future edit could make it one) and the second would be testing
			// nothing at all. `unmatched=` proves the fixture is still unmatchable.
			shim := assertionField(t, out, "pickerShim")
			if !strings.Contains(shim, "sites=0") || !strings.Contains(shim, "unmatched=1") {
				t.Fatalf("pickerShim=%q, want sites=0 and unmatched=1 — the fixture's stub is "+
					"supposed to defeat the needle, and if it no longer does then neither arm of "+
					"this test is about a blind instrument\n%s", shim, out)
			}
			if tc.wantCode == 2 {
				// The assertion's own third state, distinct from `pass`.
				if got := assertionField(t, out, "unmeasured"); got != "true" {
					t.Fatalf("unmeasured=%s, want true\n%s", got, out)
				}
				if got := summaryFieldOrEmpty(out, "RENDER"); got != "" {
					t.Fatalf("the run emitted RENDER=%s — an unmeasured run must emit NO verdict "+
						"line at all, or a reader folds it back into `no`\n%s", got, out)
				}
			}
		})
	}
}

// ── the invariant: everything that is not a pick is still refused ────────────

// 🔴 AN INVARIANT GUARD, NOT A REGRESSION TEST — it is GREEN at `origin/main`,
// because refusing a submit is what pre-change code did for every request. It is
// the thing that must not change, so it is written to fail LOUDLY if it does: the
// fixture reaches `generating` only when a `SUBMIT_WORKFLOW` is refused with the
// SDK's own message, and it puts the actual outcome in the status otherwise, so
// `observed` names the breach.
//
// Watched to fail: with the shim's allowlist removed (answering every request
// type), this grades `RENDER=no observed=ready>spend-completed`. See the mutation
// matrix in the PR body.
func TestOracleRefusesEveryRequestThatIsNotAPick(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPickerOracle(t, browser, fxManifestScoped, fxSpendRefusedApp)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "yes" {
		t.Fatalf("RENDER=%s, want yes — the fixture only reaches `generating` when the submit was "+
			"REFUSED with the SDK's own error; anything else means the oracle has started "+
			"answering a money request\n%s", got, out)
	}
	if got := assertionField(t, out, "observed"); got != "ready>generating" {
		t.Fatalf("observed=%q, want \"ready>generating\" — `spend-completed` means the submit was "+
			"answered, `refused-with:…` means it was refused with a message this harness invented\n%s",
			got, out)
	}
	// The refusal, counted from inside the page and carried onto the cell. This is
	// the field that lets a READER of a matrix see that the money path refused,
	// rather than taking a docblock's word for it.
	if got := assertionField(t, out, "hostRefused"); got != "SUBMIT_WORKFLOW" {
		t.Fatalf("hostRefused=%q, want \"SUBMIT_WORKFLOW\"\n%s", got, out)
	}
}

// ── the seam: the shape a pick resolves with ─────────────────────────────────

// 🔴 RED AT `origin/main` (the pick never resolves, so the probe never runs and
// Generate stays disabled). Green here only if the resolved objects are exactly
// what the published SDK's own mock host resolves with — see fxPickShapeProbeApp.
func TestOracleResolvesAPickWithThePublishedShape(t *testing.T) {
	browser := oracleBrowser(t)
	out, code := runPickerOracle(t, browser, fxManifestScoped, fxPickShapeProbeApp)
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := summaryField(t, out, "RENDER"); got != "yes" {
		t.Fatalf("RENDER=%s, want yes — `observed` names the field that disagrees\n%s", got, out)
	}
	if got := assertionField(t, out, "observed"); got != "ready>generating" {
		t.Fatalf("observed=%q, want \"ready>generating\"; anything else is the probe naming a "+
			"field of the resolved pick that is not what the SDK's host sends\n%s", got, out)
	}
	if got := assertionField(t, out, "hostAnswered"); got !=
		"OPEN_RESOURCE_PICKER:Checkpoint,OPEN_CHECKPOINT_PICKER,OPEN_RESOURCE_PICKER:LORA,OPEN_RESOURCE_PICKER:Embedding" {
		t.Fatalf("hostAnswered=%q — the probe asks for four picks in that order; a different list "+
			"means it stopped early and its later assertions never ran\n%s", got, out)
	}
}

// ── the source patch, without a browser ─────────────────────────────────────

// The node driver for the two unit-level guards. No browser, no network, no CLI:
// it imports `_cdp.mjs` and exercises two pure functions, printing one JSON line
// per check so a failure names itself.
//
// 🔴 IT CAN REACH NOTHING. `#704` added `STUB_CIVITAI` after a test arm reached
// the operator's real account through a leaked PATH; these two need no PATH at all
// — `patchInlineTransport` is string surgery and `inlineHostSource` is evaluated
// with `new Function`, in-process.
func writePickerUnitDriver(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(cdpModule)
	if err != nil {
		t.Fatal(err)
	}
	// 🔴 THE LEDGER IS THE SDK'S OWN `BLOCK_TO_PARENT_MESSAGE_TYPES`
	// (@civitai/app-sdk@0.51.0 `dist/blocks/messages.d.ts`), copied whole. A guard
	// that listed only the types someone thought of would pass while a new
	// spend-adjacent request quietly fell through to the answered branch; this one
	// fails if the ANSWERED SET grows or shrinks against the full wire vocabulary.
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
	// The SDK stub as a real bundle carries it: minified, parameter names chosen by
	// the minifier. Measured verbatim out of `ab-ship-mimo-02`'s
	// dist/assets/index-CkrWgKXb.js on 2026-09-25.
	minified := `sendMessage(r){}sendRequest(r,d,s){return Promise.reject(new Error("InlineTransport.sendRequest is not implemented in v1"))}onMessage(r,d){return()=>{}}`
	// 🔴 A SECOND REAL BUNDLE, AND IT SPELLS THE SAME STUB DIFFERENTLY: no `new`,
	// and a TEMPLATE LITERAL for the message. Measured verbatim out of
	// `ab-genpost-mimo-01`'s dist/assets/index-BoKmFcvM.js (blocks-react 0.53.1) on
	// 2026-09-25 — the needle's first version required `new Error('…')` and patched
	// 0 sites here while reporting a perfectly ordinary green, because that app
	// never opens a picker. One bundle is not a general claim.
	minifiedNoNew := "sendMessage(e){}sendRequest(e,t,n){return Promise.reject(Error(`InlineTransport.sendRequest is not implemented in v1`))}onMessage(e,t){return()=>{}}"
	body := `import { patchInlineTransport, inlineHostSource, INLINE_STUB_MESSAGE,
  INLINE_HOST_GLOBAL, HOST_PICKER_REQUESTS } from ` + jsQuote(abs) + `;
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const out = [];
const check = (name, ok, detail) => out.push({ name, ok: !!ok, detail: String(detail ?? '') });

// ── patchInlineTransport finds the SDK's stub, and only it ───────────────────
const MIN = ` + jsQuote(minified) + `;
const MIN_NO_NEW = ` + jsQuote(minifiedNoNew) + `;
const SRC = "    sendRequest(_request, _responseType, _opts) {\n" +
  "        return Promise.reject(new Error('" + INLINE_STUB_MESSAGE + "'));\n    }";
check('minified-stub-patched', patchInlineTransport(MIN).hits === 1, patchInlineTransport(MIN).hits);
check('minified-no-new-stub-patched', patchInlineTransport(MIN_NO_NEW).hits === 1,
  patchInlineTransport(MIN_NO_NEW).hits);
check('source-stub-patched', patchInlineTransport(SRC).hits === 1, patchInlineTransport(SRC).hits);
for (const near of [
  MIN.replace('in v1', 'in v2'),
  MIN_NO_NEW.replace('in v1', 'in v2'),
  MIN.replace(INLINE_STUB_MESSAGE, 'something else entirely'),
  'Promise.resolve(new Error("' + INLINE_STUB_MESSAGE + '"))',
]) {
  const r = patchInlineTransport(near);
  check('near-miss-untouched', r.hits === 0 && r.code === near, r.hits + ' hits');
}

// ── the patched stub delegates, and falls back when nothing is installed ─────
const mod = "export const t = { sendRequest(_request, _responseType, _opts) {\n" +
  "  return Promise.reject(new Error('" + INLINE_STUB_MESSAGE + "'));\n} };";
const patched = patchInlineTransport(mod);
check('module-stub-patched', patched.hits === 1, patched.hits);
const p = join(process.argv[2], 'patched.mjs');
await writeFile(p, patched.code);
const { t } = await import('file://' + p);
// No shim installed: byte-identical behaviour to the SDK as published.
let fellBack = null;
await t.sendRequest({ type: 'OPEN_RESOURCE_PICKER', payload: {} }).catch((e) => { fellBack = e.message; });
check('falls-back-without-a-shim', fellBack === INLINE_STUB_MESSAGE, fellBack);
// With one installed: it is handed the REQUEST, name-independently.
globalThis[INLINE_HOST_GLOBAL] = (req) => Promise.resolve({ sawType: req && req.type });
const got = await t.sendRequest({ type: 'OPEN_RESOURCE_PICKER', payload: { resourceType: 'LORA' } });
check('delegates-the-request', got && got.sawType === 'OPEN_RESOURCE_PICKER', JSON.stringify(got));
delete globalThis[INLINE_HOST_GLOBAL];

// ── the shim answers the two picker types and refuses the rest ───────────────
const win = {};
new Function('window', inlineHostSource())(win);
const host = win[INLINE_HOST_GLOBAL];
check('shim-installed', typeof host === 'function', typeof host);
const answered = [], refused = [], wrongError = [];
for (const type of ` + string(ledgerJSON) + `) {
  try {
    await host({ type, payload: { resourceType: 'Checkpoint', requestId: 'r-' + type } });
    answered.push(type);
  } catch (e) {
    refused.push(type);
    if (e.message !== INLINE_STUB_MESSAGE) wrongError.push(type + ':' + e.message);
  }
}
const want = [...HOST_PICKER_REQUESTS].sort().join(',');
check('answers-exactly-the-allowlist', answered.slice().sort().join(',') === want,
  'answered=' + answered.sort().join(',') + ' want=' + want);
check('refuses-with-the-sdk-error', wrongError.length === 0, wrongError.join(' '));
check('refused-the-rest', refused.length === ` + jsonLen(ledger) + ` - HOST_PICKER_REQUESTS.length,
  refused.length + ' of ' + ` + jsonLen(ledger) + `);
check('counters-recorded',
  (win.__dogfoodHostPicks || []).length === HOST_PICKER_REQUESTS.length &&
  (win.__dogfoodHostRefused || []).length === refused.length,
  JSON.stringify({ picks: win.__dogfoodHostPicks, refused: (win.__dogfoodHostRefused || []).length }));
// An unsupported resourceType is a DISMISSAL, never some other resource.
const dismissed = await host({ type: 'OPEN_RESOURCE_PICKER', payload: { resourceType: 'Embedding' } });
check('unsupported-type-dismisses', dismissed && dismissed.selected === undefined,
  JSON.stringify(dismissed));

for (const r of out) console.log(JSON.stringify(r));
`
	path := filepath.Join(dir, "picker-unit.mjs")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// jsQuote renders a Go string as a JS string literal.
func jsQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// jsonLen renders a slice's length as JS source, so the driver's arithmetic is
// pinned to the ledger above rather than to a number retyped beside it.
func jsonLen(v []string) string {
	b, err := json.Marshal(len(v))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// 🔴 RED AT `origin/main`: the driver imports `patchInlineTransport`,
// `inlineHostSource`, `INLINE_STUB_MESSAGE`, `INLINE_HOST_GLOBAL` and
// `HOST_PICKER_REQUESTS`, none of which exist there, so the import fails and every
// check is missing rather than merely wrong.
func TestOracleInlineHostAnswersOnlyThePickerLedger(t *testing.T) {
	node := dogfoodTool(t, "node")
	dir, err := os.MkdirTemp("", "dogfood-picker-unit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	driver := writePickerUnitDriver(t, dir)

	cmd := exec.Command(node, driver, dir)
	// 🔴 A BARE ENVIRONMENT, AND NOT FOR TIDINESS. `#704` added `STUB_CIVITAI`
	// after a test arm reached the operator's REAL account through a leaked PATH.
	// This driver needs no PATH, no HOME and no credential — it calls two pure
	// functions — so it is given none, which makes "it cannot reach the network or
	// the real CLI" a property of the invocation rather than a claim about the code.
	cmd.Env = []string{"PATH=/nonexistent"}
	raw, err := cmd.CombinedOutput()
	out := string(raw)
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running the picker unit driver: %v\n%s", err, out)
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
	// 🔴 A MINIMUM COUNT, because a driver that died halfway prints only the checks
	// it reached and every one of them can be green. 14 checks: 3 stub spellings, 4
	// near misses, 1 module patch, 2 delegation arms, and 4 on the shim.
	if seen < 14 {
		t.Fatalf("only %d checks reported, want at least 14 — the driver stopped early\n%s", seen, out)
	}
}
