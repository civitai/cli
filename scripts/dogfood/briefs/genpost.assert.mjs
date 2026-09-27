#!/usr/bin/env node
// The behavioural assertion for the `genpost` brief. See genpost.md.
//
//   node genpost.assert.mjs <dir>            # serves the dir, drives it, grades it
//   node genpost.assert.mjs http://host:port # grades something already served
//   node genpost.assert.mjs <dir|url> a,b    # …presenting the scopes `a` and `b`
//
// Prints one JSON line and exits 0 (pass) or 1 (fail). This is the PREDICATE,
// not the render oracle; the browser plumbing lives in `_cdp.mjs`.
//
// 🔴 WHAT THIS CAN AND CANNOT SEE — read this before reading a verdict.
// The oracle emulates a host by seeding `window.__CIVITAI_BLOCK_CONTEXT__`,
// which the SDK's transport detector answers with `InlineTransport`. That
// transport is a v1 stub: `sendRequest` REJECTS and host pushes never arrive.
// So ON THE DEFAULT AND UNCONSENTED ARMS no generation and no post can complete
// here, on any machine, with or without a credential, and an assertion that waited
// for a rendered image or a post id would time out against a perfect app. The
// exceptions are the entries of `INVARIANT_EXCEPTIONS` in `_cdp.mjs` — a LEDGER, so
// that neither this file nor any doc in this tree has to spell a count that goes
// stale the next time an arm lands. The two below say what each class costs.
//
// 🔴 THE PICK IS ANSWERED ON EVERY ARM, AND IT IS NOT A SPENDING ONE. Since
// 2026-09-25 the oracle answers a host RESOURCE PICK (`OPEN_RESOURCE_PICKER` /
// `OPEN_CHECKPOINT_PICKER`) with the resource the SDK's own mock host resolves
// with, because an app that gates Generate behind `openPicker` could otherwise
// never reach `generating` — measured on `ab-ship-mimo-02`, which graded
// `RENDER=no observed=ready generateDisabled=true` for that reason and no other.
// Off the post arm every OTHER request type still rejects with the SDK's own
// `InlineTransport.sendRequest is not implemented in v1`, and this file reports
// which ones did on the cell as `hostRefused` — so the paragraph above is checked
// per cell rather than promised in a comment. See `HOST_RESOURCE_PICKS` and
// `patchInlineTransport` in `_cdp.mjs`.
//
// 🔴 THAT STAYS TRUE WITH THE BLOCK'S SCOPES SEEDED. The bootstrap presents the
// scope list the block's own manifest declares (argv[3], handed down by
// oracle.sh) so that a consent-gated Generate handler takes the granted branch
// instead of the refused one — but `token.raw` is still `''` and `sendRequest`
// still rejects everything outside the running arm's answered set: the scope seed
// widens that set by nothing. Scopes buy a BRANCH, never a CAPABILITY. See
// `hostBootstrap` in `_cdp.mjs` for the four-arm measurement behind it.
//
// What IS deterministic without a host round-trip is the block's own state
// machine on the near side of the request: the prompt input, the `ready` resting
// state, the Post gate being CLOSED before any generation has succeeded, and the
// Generate click driving the machine into `generating`. That is what this
// grades. A green cell means "the model wired a generate-then-post flow whose
// gating and status machine behave as the brief specified" — NOT "a generation
// ran" and NOT "a post was created". Those need a credentialed live run
// (`dev:live` / a dev token), which is a different instrument.
//
// 🔴 WHY THE POST GATE IS THE LOAD-BEARING STEP. The `page-money` scaffold
// ALREADY ships a prompt field and a Generate button wired to a real
// `submitWorkflow` — so an assertion keyed on generation alone is satisfied by
// an untouched scaffold and measures nothing. Posting is the half no template
// ships: no `posts:write:self` scope, no Post control, no gate. Keying the
// verdict on the Post button's existence AND its disabled state is what makes a
// page-money-derived cell separable from a page-money scaffold. The scaffold
// control in genpost.md records all three templates failing.

// 🔴 AND THERE IS A SECOND ARM, BECAUSE EVERY PARAGRAPH ABOVE GRADES THE
// ALREADY-CONSENTED VIEWER AND NO REAL USER STARTS THERE.
// `CIVITAI_ASSERT_UNCONSENTED=1` seeds `token.scopes` EMPTY and replaces the
// predicate: instead of "did the Generate click drive the machine to
// `generating`", it asks "did the block ASK THE HOST FOR CONSENT". The two arms
// answer different questions about the same trial and both verdicts are real; the
// cell's `arm=` field is what says which one you are reading.
//
// It exists because this file graded `ab-ship-mimo-02` `RENDER=yes` and that app,
// once deployed, failed for a real user on the very first click — see
// `UNCONSENTED` in `_cdp.mjs` for the defect, the three reasons the default arm is
// structurally blind to it, and the measurement that the split is not a
// per-vendor story.
//
// 🔴 AND A THIRD ARM, BECAUSE THE SAME APP THEN SHIPPED A SECOND USER-FOUND
// DEFECT — ON THE HALF NO ARM ABOVE CAN REACH AT ALL.
// `CIVITAI_ASSERT_POST_PATH=1` answers the workflow and post requests in the page,
// so the block's POST branch becomes reachable, and replaces the predicate again:
// instead of "did the Generate click drive the machine to `generating`", it asks
// "does the Post gate hold for a generation that produced NO images, and does a
// generation that DID produce images actually post". `ab-img-poster@0.1.1` was
// graded green by both arms above and then showed a real user
// *"Posting failed. Please try again."* — because it opened Post on any terminal
// workflow and posted a workflow with nothing in it.
//
// 🔴 THIS ARM SUSPENDS THE INVARIANT IN THE PARAGRAPH ABOVE, AND ONLY THIS ARM.
// With the flag set, a canned generation DOES complete and a canned post IS
// created — in the page, with no credential and nothing leaving the browser. Every
// verdict this arc has recorded was taken with the flag unset, and with it unset
// `inlineHostSource()` returns byte-for-byte the source it returned before this
// arm existed (measured, and pinned by the unit driver). `arm=post` on the cell is
// what stops the two being confused. See `POST_PATH` in `_cdp.mjs` for the full
// statement, including what a green here does NOT prove.
import { launch, cdp, openPage, parseScopes, resolveTarget, SEND_HOST_INIT, HOST_VIEWER_LABEL, HOST_PICKS_LABEL, HOST_ARM, UNCONSENTED, POST_PATH, ARM_CONFLICT, CONSENT_MESSAGE, POST_ARM_PLAN, POST_ARM_NO_IMAGES_ERROR, POST_ARM_CREATE_POST_RESULT, POST_ARM_GLOBAL, POST_ARM_CEILING_TOKEN, seededScopes, CLICKABLES, labelExpr, sleep } from './_cdp.mjs';

const TARGET = process.argv[2];
if (!TARGET) {
  console.error('usage: genpost.assert.mjs <dir|url> [scope,scope,…]');
  process.exit(2);
}
// The block's OWN declared scopes, read out of its `block.manifest.json` by
// `oracle.sh` and handed down here rather than re-parsed. Absent = `[]`, which
// is what a hand-run assertion grades and what this file graded before the
// argument existed. See `hostBootstrap` in `_cdp.mjs` for why an empty list is
// not a neutral default.
const SCOPES = parseScopes(process.argv[3]);
// Named rather than left to fail as `WebSocket is not defined` three frames
// deep: the global landed in node 22, and a trial image is free to ship an
// older one.
if (typeof WebSocket === 'undefined') {
  console.error(`node ${process.versions.node} has no global WebSocket — this needs node >= 22`);
  process.exit(2);
}
// 🔴 AN INCOHERENT ARM COMBINATION IS REFUSED BEFORE A BROWSER IS EVEN STARTED,
// and it exits 2 (`oracle.sh` renders that as `RENDER=unmeasured`) rather than
// emitting the verdict it would have produced. Every arm here is selected by an
// AMBIENT environment variable, so the combinations `ARM_CONFLICT` names are one
// stale export away on any operator's shell — and each of them would grade a
// confident `no` about nothing. See `ARM_CONFLICT` in `_cdp.mjs`.
if (ARM_CONFLICT) {
  console.log(JSON.stringify({
    assertion: 'genpost', pass: false, unmeasured: true,
    reason: `harness error: ${ARM_CONFLICT}`, hostArm: HOST_ARM,
  }));
  process.exit(2);
}

// ── the assertion's constants ────────────────────────────────────────────────
const SEL_PROMPT = '[data-testid="prompt"]';
const SEL_STATUS = '[data-testid="status"]';
const GENERATE_LABEL = 'generate';
const POST_LABEL = 'post';
// The two status words the brief names. `ready` is the resting state; the click
// must drive the machine to `generating`.
const STATUS_IDLE = 'ready';
const STATUS_BUSY = 'generating';
// Ordinary, unambiguously safe prompt text. It is typed, never submitted to any
// real backend — see the header.
const PROMPT_TEXT = 'a red cube on a white table';
/**
 * How many HOST-INTERACTION affordances the assertion will work through before
 * giving up on a disabled Generate, and how long each gets to settle.
 *
 * 🔴 WHY IT CLICKS ANYTHING AT ALL BESIDES Generate. Measured on `ab-ship-mimo-02`
 * (2026-09-25): the app opens the host's resource picker from a `Select Model`
 * button and gates Generate on `!model`, so Generate is disabled until a pick
 * comes back. Nothing in this assertion had ever clicked anything but Generate, so
 * the click landed on a disabled control, the status never moved, and the cell
 * read `RENDER=no observed=ready generateDisabled=true` — a verdict about the
 * harness's driving, not about the app. A viewer sitting in front of that app
 * clicks the picker; the brief never forbade one, and hardcoding a checkpoint
 * (what the passing cells did) is the WORSE app.
 *
 * 🔴 BOUNDED, AND Generate/Post ARE EXCLUDED BY NAME. Unbounded clicking would
 * turn this into a fuzzer whose verdict depends on button order, and clicking
 * Generate here would make step 7's "the click drove the machine" unattributable.
 * Post is excluded because clicking it is the one interaction the brief says must
 * not be reachable yet — step 3 has just asserted it is closed, and prodding it
 * would be the assertion testing its own earlier claim rather than the app.
 */
const PREREQ_CLICK_LIMIT = 4;
const PREREQ_SETTLE_MS = 400;
/**
 * How long the UNCONSENTED arm waits for a consent ask after the Generate click
 * before reading the ledger.
 *
 * 🔴 IT IS A SETTLE, NOT A `waitFor`, AND THAT IS DELIBERATE. A `waitFor` would
 * burn the full `WAIT_MS` on every app that never asks — which in this arm is the
 * expected FAILING case, so the common path would be the slow one. More
 * importantly, an ask is SYNCHRONOUS from the click on every shape measured (the
 * SDK's `requestConsent` is a plain `sendMessage`, and the two consent-first
 * fixtures call it in the click handler before any await), so a long wait buys
 * nothing a short one does not already have. The four `PREREQ_SETTLE_MS` windows
 * step 6 may already have spent are on top of this.
 */
const CONSENT_SETTLE_MS = 1200;

// ── the POST arm's own timings ───────────────────────────────────────────────

/**
 * How long the POST arm gives the block to finish RENDERING a terminal workflow
 * snapshot, after the shim has already answered the poll that carried it.
 *
 * 🔴 TWO WAITS, AND ONLY ONE OF THEM CAN BE A `waitFor`. The arm's first phase
 * asserts an ABSENCE — the Post control must still be shut after a workflow that
 * produced no images — and you cannot wait for an absence: a `waitFor` would
 * either return immediately (proving nothing about a block that opens Post 50 ms
 * later) or burn the full budget on every correct app. So the sequence is: wait for
 * the terminal snapshot to have been DELIVERED (a positive fact the shim's own
 * ledger carries), then wait for the block to have visibly finished with it — the
 * Generate control coming back is the signal where an app offers one, bounded by
 * {@link GENERATE_RECOVERY_MS} because an app is free not to — and only then read
 * the gate, after a fixed settle.
 *
 * ⚠ SAY WHAT THAT DOES NOT COVER: an app that opens Post LATER than the settle
 * would pass phase 1 here. The settle is sized against the measured shape (React
 * commits the terminal snapshot in one tick after the awaited poll resolves), not
 * against an adversary.
 */
const POST_SETTLE_MS = 700;

/** How long the arm waits for the Generate control to become clickable again
 * after a run — the positive signal that the block has finished handling a
 * terminal snapshot. Bounded and NOT fatal: an app that deliberately keeps
 * Generate shut after a run is not wrong, it just offers no such signal, and the
 * fixed settle above covers it. */
const GENERATE_RECOVERY_MS = 3000;

/** The two canned runs the arm drives, named here so a reason can say which phase
 * it is talking about. Read off the plan rather than retyped, so the labels on a
 * cell and the outcomes the shim serves cannot drift. */
const PHASE_LABELS = POST_ARM_PLAN.map((p) => p.label);

// A recorder for every value `[data-testid="status"]` ever holds, installed
// BEFORE the click. 🔴 Polling cannot do this job: the machine may pass through
// `generating` and out the other side (the stub host rejects the request) faster
// than any poll interval, and a missed transition would grade a correct app as
// one whose button does nothing. A MutationObserver over the whole body catches
// both an in-place text mutation and a wholesale re-render that REPLACES the
// element, which React does depending on how the tree is keyed.
const INSTALL_RECORDER = `(() => {
  const read = () => {
    const e = document.querySelector('${SEL_STATUS}');
    return e ? (e.textContent || '').trim() : null;
  };
  const seen = [];
  const push = () => {
    const v = read();
    if (v !== null && v !== seen[seen.length - 1]) seen.push(v);
  };
  push();
  window.__dogfoodStatusSeen = seen;
  new MutationObserver(push).observe(document.body, {
    subtree: true, childList: true, characterData: true, attributes: true,
  });
  return true;
})()`;

// Read the recorded sequence, then top it up with a live read — a machine that
// settled before the observer was wired, or one whose final value arrived with
// no mutation the observer could see, must still be represented.
const READ_RECORDER = `(() => {
  const seen = window.__dogfoodStatusSeen || [];
  const e = document.querySelector('${SEL_STATUS}');
  const now = e ? (e.textContent || '').trim() : null;
  if (now !== null && now !== seen[seen.length - 1]) seen.push(now);
  return JSON.stringify(seen);
})()`;

/** `true` when the element is disabled by either the property or ARIA. */
function disabledExpr(elExpr) {
  return `(() => { const e = ${elExpr};
    if (!e) return null;
    return e.disabled === true || e.getAttribute('aria-disabled') === 'true';
  })()`;
}

async function main() {
  const { url, server } = await resolveTarget(TARGET);
  const { proc, ws } = await launch();
  const c = cdp(ws);
  await c.open;

  const evidence = {
    target: TARGET, url, prompt: PROMPT_TEXT,
    hostInit: SEND_HOST_INIT, hostViewer: HOST_VIEWER_LABEL,
    // 🔴 TWO SCOPE FIELDS, NOT ONE, AND `oracle.sh` CHECKS A DIFFERENT SEAM WITH
    // EACH. `hostScopesDeclared` is the argument that crossed the process
    // boundary — it must equal the manifest's declaration, whatever arm is
    // running, which is what catches an assertion that ignored its argument.
    // `hostScopes` is what the BLOCK WAS SHOWN, so on the unconsented arm it is
    // `none`; comparing THAT against the manifest would make the arm look like a
    // harness defect. Both come from `_cdp.mjs` rather than being recomputed here.
    hostScopesDeclared: SCOPES.join(',') || 'none',
    hostScopes: seededScopes(SCOPES).join(',') || 'none',
    hostArm: HOST_ARM,
  };
  let pass = false;
  let reason = null;
  let page = null;
  // 🔴 THE THIRD STATE. `pass` is a claim about the BLOCK; this is a claim about
  // the HARNESS, and `oracle.sh` turns it into exit 2 / `RENDER=unmeasured` rather
  // than a verdict. See `pickerBlind` below for the one condition that sets it.
  let unmeasured = false;

  /**
   * What the oracle's host shim was asked for, and what the source patch did.
   *
   * 🔴 CALLED ON THE PASS PATH *AND* FROM THE catch, AS LATE AS POSSIBLE IN BOTH.
   * These are the fields that say whether the picker half of the harness was
   * working at all, so they matter MOST on the arm that failed — a cell reading
   * `pickerShim: answered:docs=1,sites=0` is "the SDK reworded its stub", which is
   * a completely different finding from "the model built no picker", and without
   * this read they are the same bare `no`.
   */
  const captureHostEvidence = async () => {
    if (!page) return;
    const seen = await page.hostRequests();
    evidence.hostAnswered = seen.answered.join(',') || 'none';
    // 🔴 THE INVARIANT, MEASURED PER CELL RATHER THAN ASSERTED IN PROSE. Every
    // request that is not a resource pick is refused with the SDK's own
    // `InlineTransport.sendRequest is not implemented in v1`, so this field is the
    // per-cell evidence that the generation the app just attempted could not have
    // completed — rather than a promise in a docblock that nothing checks.
    evidence.hostRefused = seen.refused.join(',') || 'none';
    // 🔴 EVERY FIRE-AND-FORGET MESSAGE THE BLOCK SENT, ON BOTH ARMS. Nothing
    // answered any of them (see `blockMessageSource`), so this adds no capability
    // — it is pure evidence, and it is the field that makes "this green app never
    // asked for consent" visible on a DEFAULT-arm cell instead of only to the arm
    // that goes looking.
    evidence.hostMessages = seen.messages.join(',') || 'none';
    // The decided fact of the unconsented arm, reported on both so a reader can
    // compare the same trial across arms without re-parsing `hostMessages`.
    evidence.consentRequested = seen.messages.some(
      (m) => m === CONSENT_MESSAGE || m.startsWith(`${CONSENT_MESSAGE}:`));
    evidence.pickerShim = `${HOST_PICKS_LABEL}:docs=${page.shim.documents},sites=${page.shim.sites}` +
      (page.shim.unmatched ? `,unmatched=${page.shim.unmatched}` : '') +
      (page.shim.skipped ? `,skipped=${page.shim.skipped}` : '');
    // 🔴 ITS OWN FIELD, NOT A SUFFIX ON `pickerShim`. The two patches fail
    // independently and regrade different apps; a reader who sees
    // `messageShim: sites=0,unmatched=1` knows a consent ask could not have been
    // OBSERVED, which is a completely different finding from a picker that could
    // not be ANSWERED.
    evidence.messageShim = `sites=${page.shim.messageSites}` +
      (page.shim.messageUnmatched ? `,unmatched=${page.shim.messageUnmatched}` : '');
    // 🔴 THE POST ARM'S LEDGER, AND ONLY ON THAT ARM. On every other arm the shim
    // installs no ledger, so this field would be a row of zeros on every cell —
    // and a `postArm=` field that is always present is exactly how a reader stops
    // noticing which arm they are looking at. `posts` is the decided half: an
    // `answered:` entry means the host CREATED the canned post, a `refused:` entry
    // names which gate turned the payload down.
    if (POST_PATH) {
      const arm = await page.postArm();
      evidence.postArm = `estimates=${arm.estimates},submits=${arm.submits},` +
        `polls=${arm.polls},terminal=${arm.terminal}`;
      evidence.postArmQueued = arm.queued.join(',') || 'none';
      evidence.postArmPosts = arm.posts.join(',') || 'none';
      if (arm.unknownPolls.length) evidence.postArmUnknownPolls = arm.unknownPolls.join(',');
    }
  };

  /**
   * TRUE when this oracle served a bundle carrying the SDK's inline transport and
   * could not instrument a single one of its stubs.
   *
   * 🔴 WHY THIS EXISTS: `sites=0` IS A SILENT INSTRUMENT FAILURE THAT REGRADES THE
   * DEFECT THIS WHOLE FILE WAS CHANGED FOR. If the needle stops matching — a
   * minifier reshapes the reject expression, the SDK rewords the stub — then a pick
   * is never answered, `model` never arrives, Generate stays shut, the status never
   * leaves `ready`, and the cell reads `RENDER=no`: BYTE-IDENTICAL to the verdict
   * `ab-ship-mimo-02` earned, and attributed to the model. Not hypothetical — the
   * needle WAS wrong on the second real bundle anyone tried (0.53.1 emits
   * `Error(\`…\`)` with no `new`), and it patched 0 sites while the cell stayed
   * green, *only* because that app happens never to open a picker.
   *
   * 🔴 IT IS DELIBERATELY NOT "sites === 0". That alone is the COMMON, HARMLESS
   * case — a block that does not bundle the SDK's inline transport at all — and
   * refusing those would trade one wrong verdict for a hundred. The trigger is
   * `unmatched > 0`: a response carrying the stub's MESSAGE (a string literal a
   * minifier must preserve) that matched no needle. That is the instrument saying
   * "an inline transport went past me and I could not install on it".
   *
   * ⚠ AND IT ONLY DEGRADES ONE OUTCOME — the gate that could not open. A blind run
   * that still reaches `generating` is reported, not refused: nothing was harmed,
   * and `pickerShim` carries `unmatched=` either way. A blind run that fails for
   * some other reason — no Post control, the wrong resting word, no prompt element
   * — is likewise a verdict, because none of those are what an unanswered pick does.
   */
  const pickerBlind = () => !!page && page.shim.unmatched > 0;

  /**
   * TRUE when this oracle served a bundle carrying the SDK's inline transport and
   * could not instrument a single one of its `sendMessage` no-ops.
   *
   * 🔴 THE SAME HAZARD AS `pickerBlind`, ONE AXIS OVER, AND IT REGRADES THE
   * DEFECT THIS ARM WAS BUILT FOR. If the `sendMessage` needle stops matching — a
   * bundler reshapes the method, the SDK implements it for real — then a consent
   * ask is never RECORDED, `consentRequested` is false, and the unconsented arm
   * reads `no`: byte-identical to the verdict a consent-blind app earns, and
   * attributed to the model. So an unconsented run that saw no ask on a bundle it
   * could not instrument is `unmeasured`, not `no`.
   *
   * ⚠ IT DEGRADES ONE OUTCOME ONLY. A blind run that DID somehow see an ask is
   * reported (nothing was harmed), and a blind run that failed earlier — no
   * prompt, the wrong resting word, no Post control — is still a verdict, because
   * none of those are what an unobserved message does. `messageShim` carries
   * `unmatched=` on the cell either way.
   */
  const consentBlind = () => !!page && page.shim.messageUnmatched > 0;

  /**
   * The DEFAULT arm's steps 6-tail and 7: the gate must open and the Generate
   * click must drive the status machine to `generating`.
   *
   * 🔴 IT IS A FUNCTION SO THE TWO ARMS ARE SIBLINGS RATHER THAN ONE BEING THE
   * EARLY-RETURN OF THE OTHER. Its body is UNCHANGED from before the unconsented
   * arm existed — including the `pickerBlind` unmeasured branch and the
   * `slice(beforeClick)` scoping — because the standing requirement on this change
   * is that no fixture's DEFAULT verdict moves. A reader comparing it against
   * `origin/main` should find only the wrapping.
   */
  const gradeTheGenerateClick = async () => {
    if (evidence.generateDisabled === true) {
      // 🔴 THE ONE PLACE A `no` IS NOT SAFE TO EMIT. A gate that did not open,
      // on a page whose inline transport this oracle could not instrument, is
      // EXACTLY the signature an unanswerable pick produces — so the honest
      // report is "nothing was measured", not "the model did not build the app".
      // `oracle.sh` renders exit 2 as `RENDER=unmeasured`, which is the state it
      // already keeps for precisely this confound.
      if (pickerBlind()) {
        unmeasured = true;
        throw new Error(`harness error: the "${GENERATE_LABEL}" control is still disabled with ` +
          `the prompt typed, and this oracle could not instrument ${page.shim.unmatched} ` +
          `response(s) carrying the SDK's inline transport (${evidence.pickerShim}) — so a host ` +
          `resource pick could not have been answered here. That is an instrument failure, not a ` +
          `verdict about the block: patchInlineTransport's needle no longer matches this bundle's ` +
          `spelling of the stub. Clicked ${JSON.stringify(evidence.prereqClicks)}.`);
      }
      throw new Error(`the "${GENERATE_LABEL}" control is still disabled with the prompt typed` +
        ` (clicked ${evidence.prereqClicks.length} host affordance(s): ` +
        `${JSON.stringify(evidence.prereqClicks)}; host answered ${evidence.hostAnswered})`);
    }
    // Where the recorder's sequence stood BEFORE the Generate click. Everything
    // after this index is attributable to that click and nothing else — see the
    // predicate at the end of step 7.
    const beforeClick = JSON.parse(await page.evalJs(READ_RECORDER)).length;
    evidence.statusBeforeClick = await page.evalJs(
      `(document.querySelector('${SEL_STATUS}').textContent || '').trim()`);

    // ── step 7: click Generate, and grade only what follows the click ────────
    const clicked = await page.evalJs(`(() => {
      const hit = ${labelExpr(GENERATE_LABEL)};
      if (!hit) return null;
      hit.click();
      return true;
    })()`);
    if (!clicked) throw new Error(`the "${GENERATE_LABEL}" control vanished before it could be clicked`);

    await page.waitFor(
      `(window.__dogfoodStatusSeen || []).slice(${beforeClick}).length > 0`,
      `the status to move after clicking ${GENERATE_LABEL}`);
    const seq = JSON.parse(await page.evalJs(READ_RECORDER));
    // 🔴 `observed` IS THE WHOLE SEQUENCE, NOT THE FINAL VALUE. The stub host
    // rejects the request, so a correct app lands on its own failure state a
    // moment later; reporting only where it ended would make every correct app
    // look broken. The sequence shows the transition that actually matters.
    evidence.observed = seq.join('>');
    // 🔴 AFTER THE CLICK, NOT ANYWHERE IN THE SEQUENCE. Step 6 may now click other
    // controls while the recorder is live, so `seq.includes(STATUS_BUSY)` would
    // accept an app whose PICKER button, not its Generate button, drove the
    // machine. Slicing at `beforeClick` keeps the verdict a claim about Generate.
    // For a cell that clicked nothing in step 6 — every arm that passed before
    // this existed — the slice is the whole post-click tail and the predicate is
    // unchanged.
    pass = seq.slice(beforeClick).includes(STATUS_BUSY);
    if (!pass) {
      reason = `the status never read ${JSON.stringify(STATUS_BUSY)} after the ` +
        `${GENERATE_LABEL} click; it went ${JSON.stringify(evidence.observed)}`;
    }
    // Re-read: the Generate click is what fires the workflow requests, so the
    // refusal list is only complete AFTER it. The earlier call is for the
    // still-disabled throw above, which happens before any of that.
    await captureHostEvidence();
  };

  // ── the POST arm ───────────────────────────────────────────────────────────

  /** Wait for the block to have finished with canned run number `n`, then settle.
   * See POST_SETTLE_MS for why the two halves are different kinds of wait. */
  const settleAfterRun = async (n) => {
    await page.waitFor(
      `(window.${POST_ARM_GLOBAL} || { terminal: 0 }).terminal >= ${n}`,
      `the canned workflow #${n} (${PHASE_LABELS[Math.min(n, PHASE_LABELS.length) - 1]}) ` +
      `to reach a terminal snapshot`);
    const deadline = Date.now() + GENERATE_RECOVERY_MS;
    for (;;) {
      if (await page.evalJs(disabledExpr(labelExpr(GENERATE_LABEL))) !== true) break;
      if (Date.now() > deadline) break;
      await sleep(100);
    }
    await sleep(POST_SETTLE_MS);
  };

  /** Click a label that must be live, returning `true` if it was clicked. */
  const clickLive = async (label) => !!(await page.evalJs(`(() => {
    const hit = ${labelExpr(label)};
    if (!hit || hit.disabled === true || hit.getAttribute('aria-disabled') === 'true') return false;
    hit.click();
    return true;
  })()`));

  /**
   * The POST arm's steps 7-onward. TWO canned runs, in this order:
   *
   *   1. a workflow that reaches `succeeded` carrying NO `imageUrls`. The Post gate
   *      must STAY SHUT. This is the users' case and it is first precisely so that
   *      the discriminating claim needs nothing else to be earned.
   *   2. a workflow that succeeds WITH an image. The gate must OPEN, the Post click
   *      must actually send a `CREATE_POST_FROM_APP`, and the payload must satisfy
   *      the host's gate.
   *
   * 🔴 BOTH PHASES ARE REQUIRED AND NEITHER IS SUFFICIENT. Phase 1 alone is passed
   * by an app with no Post control at all — the gate is trivially shut — which is
   * the very thing the brief's step 3 exists to reject. Phase 2 alone is passed by
   * `ab-img-poster@0.1.1`, which posts perfectly well when there IS an image and
   * shipped broken anyway.
   *
   * 🔴 AND PHASE 1 DOES NOT STOP AT THE GATE. If the gate is open, the arm CLICKS
   * Post and records what the host did with it, because "this app would have
   * published a workflow with nothing in it" and "the host refused it with
   * `no images to post`" is the whole causal chain of the reported defect — and a
   * reason that carries the refusal is a reason a reader can act on, where a bare
   * "the gate was open" is a claim they have to take on trust.
   */
  const gradeThePostPath = async () => {
    // Phase 1's front half is the DEFAULT arm's step 7, unchanged and reused: the
    // Generate click must drive the machine to `generating`. If it did not, the post
    // path was never reachable and that arm's reason is the right one to report.
    await gradeTheGenerateClick();
    if (!pass) {
      reason = `${reason} (the post arm never reached its own phases: the generate ` +
        `click has to work first)`;
      return;
    }
    pass = false;

    // ── phase 1: a succeeded workflow that produced NO images ────────────────
    await settleAfterRun(1);
    evidence.postGateAfterImagelessRun = await page.evalJs(disabledExpr(labelExpr(POST_LABEL)));
    await captureHostEvidence();
    if (evidence.postGateAfterImagelessRun === null) {
      // Step 3 found a Post control at rest and it has since vanished. Reported as
      // its own reason rather than collapsing into the gate verdict.
      reason = `the "${POST_LABEL}" control disappeared after the first generation ` +
        `(${PHASE_LABELS[0]}), so there is nothing to post with`;
      return;
    }
    if (evidence.postGateAfterImagelessRun !== true) {
      // 🔴 THE DEFECT. Show the consequence rather than asserting it: the click
      // sends whatever payload the app would have sent in production, and the
      // host's own refusal code comes back on the arm's ledger.
      evidence.postClickedOnImagelessRun = await clickLive(POST_LABEL);
      if (evidence.postClickedOnImagelessRun) {
        try {
          await page.waitFor(`(window.${POST_ARM_GLOBAL} || { posts: [] }).posts.length > 0`,
            'the host to answer the post this app sent for an image-less workflow');
        } catch { /* it sent nothing at all; the ledger says so */ }
        await sleep(POST_SETTLE_MS);
        await captureHostEvidence();
      }
      reason = `the "${POST_LABEL}" control is OPEN after a workflow that reached ` +
        `"succeeded" with an EMPTY imageUrls list — there is nothing to post, and a viewer ` +
        `who clicks it gets the host's refusal. Clicking it here ` +
        `${evidence.postClickedOnImagelessRun
          ? `sent ${evidence.postArmPosts === 'none'
            ? 'no CREATE_POST_FROM_APP at all' : `a post the host answered ${evidence.postArmPosts}`}`
          : 'was not possible (the control went away)'}. ` +
        `That is the live defect ab-img-poster@0.1.1 shipped: "Posting failed. Please try again.". ` +
        `A correct app gates Post on status === "succeeded" AND a non-empty imageUrls.`;
      return;
    }

    // ── phase 2: a succeeded workflow that DID produce an image ──────────────
    evidence.secondGenerateClicked = await clickLive(GENERATE_LABEL);
    if (!evidence.secondGenerateClicked) {
      // 🔴 UNMEASURED, NOT `no`. The arm needs a second generation to reach the
      // post path at all, and an app that will not start one — because it demands a
      // fresh prompt, say — has not been shown to be wrong about anything. Emitting
      // `no` here would be this file's own blindness, one level up.
      unmeasured = true;
      throw new Error(`harness error: the post arm held its Post gate correctly on the ` +
        `image-less run, and then could not start the SECOND canned run ` +
        `(${PHASE_LABELS[1]}) — the "${GENERATE_LABEL}" control was not clickable ` +
        `${GENERATE_RECOVERY_MS}ms after the first run finished. The post half of this arm ` +
        `needs that run, so nothing was measured about it. Queued so far: ${evidence.postArmQueued}.`);
    }
    await settleAfterRun(2);
    evidence.postGateAfterSuccessfulRun = await page.evalJs(disabledExpr(labelExpr(POST_LABEL)));
    await captureHostEvidence();
    try { evidence.observed = JSON.parse(await page.evalJs(READ_RECORDER)).join('>'); } catch { /* none */ }
    if (evidence.postGateAfterSuccessfulRun !== false) {
      reason = `the "${POST_LABEL}" control never opened after a workflow that reached ` +
        `"succeeded" with an image (${PHASE_LABELS[1]}) — it reads ` +
        `${JSON.stringify(evidence.postGateAfterSuccessfulRun)}. The gate held on the ` +
        `image-less run, so this app is not wrong about the hazard; it cannot post at all. ` +
        `Workflows queued: ${evidence.postArmQueued}; host refused: ${evidence.hostRefused}.`;
      return;
    }
    if (!(await clickLive(POST_LABEL))) {
      reason = `the "${POST_LABEL}" control was open and then could not be clicked`;
      return;
    }
    try {
      await page.waitFor(`(window.${POST_ARM_GLOBAL} || { posts: [] }).posts.length > 0`,
        `the block to send a ${'CREATE_POST_FROM_APP'} after the ${POST_LABEL} click`);
    } catch { /* nothing was sent; the reason below says so */ }
    await sleep(POST_SETTLE_MS);
    await captureHostEvidence();
    const posts = (evidence.postArmPosts || 'none').split(',');
    const last = posts[posts.length - 1];
    pass = last.startsWith('answered:');
    if (!pass) {
      reason = last === 'none'
        ? `clicking "${POST_LABEL}" on a postable generation sent NO CREATE_POST_FROM_APP — ` +
          `the control is live and wired to nothing the host can act on. Host refused: ` +
          `${evidence.hostRefused}; messages sent: ${evidence.hostMessages}.`
        : `the host REFUSED the post this block sent for a workflow that succeeded WITH an ` +
          `image (${last}). The payload does not satisfy the host's gate: ` +
          `\`sources\` must be a non-empty array, and a \`workflow\` source must name a ` +
          `workflow that actually produced images. The canned refusal is ` +
          `${JSON.stringify(POST_ARM_NO_IMAGES_ERROR)}, exactly as the platform sends it.`;
    } else {
      // Reported on a PASS too, because "the post was created" is the one fact this
      // arm exists to establish and a reader must be able to see it without
      // re-deriving it from a boolean.
      evidence.postCreated = POST_ARM_CREATE_POST_RESULT.postId;
    }
  };

  try {
    page = await openPage(c, url, { scopes: SCOPES });

    // ── step 1: the prompt input exists ──────────────────────────────────────
    await page.waitFor(`!!document.querySelector('${SEL_PROMPT}')`, `${SEL_PROMPT} to appear`);

    // ── step 2: the status element exists and rests at `ready` ───────────────
    await page.waitFor(`(() => { const e = document.querySelector('${SEL_STATUS}');
      return !!e && (e.textContent || '').trim().length > 0; })()`,
      `${SEL_STATUS} to hold text`);
    evidence.initialStatus = await page.evalJs(
      `(document.querySelector('${SEL_STATUS}').textContent || '').trim()`);
    if (evidence.initialStatus !== STATUS_IDLE) {
      throw new Error(`${SEL_STATUS} reads ${JSON.stringify(evidence.initialStatus)} at rest, expected ${JSON.stringify(STATUS_IDLE)}`);
    }

    // ── step 3: the Post gate exists and is CLOSED ───────────────────────────
    // 🔴 The step that separates this brief from the scaffolds. Both halves
    // matter: a missing Post button is "the model built a generator, not a
    // generate-then-post app", and an ENABLED one is "the model wired a control
    // that would publish under the viewer's byline before anything exists to
    // publish". They are different findings and the reason says which.
    evidence.postDisabled = await page.evalJs(disabledExpr(labelExpr(POST_LABEL)));
    if (evidence.postDisabled === null) {
      evidence.buttonLabels = await page.evalJs(
        `JSON.stringify(${CLICKABLES}.map((e) => ((e.textContent || e.value || '') + '').trim()))`);
      throw new Error(`no clickable element labelled "${POST_LABEL}" — nothing posts`);
    }
    if (evidence.postDisabled !== true) {
      throw new Error(`the "${POST_LABEL}" control is enabled before any generation has succeeded`);
    }

    // ── step 4: the Generate control exists ──────────────────────────────────
    // `generateDisabledAtRest` is REPORTED and decides nothing: a Generate button
    // disabled on an empty prompt is correct, and both page templates ship exactly
    // that. What must be true is that it is clickable by the time step 7 clicks
    // it, which is what steps 5–6 establish.
    evidence.generateDisabledAtRest = await page.evalJs(disabledExpr(labelExpr(GENERATE_LABEL)));
    if (evidence.generateDisabledAtRest === null) {
      evidence.buttonLabels = await page.evalJs(
        `JSON.stringify(${CLICKABLES}.map((e) => ((e.textContent || e.value || '') + '').trim()))`);
      throw new Error(`no clickable element labelled "${GENERATE_LABEL}"`);
    }

    // ── step 5: record the status machine, then type the prompt ──────────────
    await page.evalJs(INSTALL_RECORDER);
    evidence.inputValue = await page.typeInto(SEL_PROMPT, PROMPT_TEXT);
    if (evidence.inputValue !== PROMPT_TEXT) {
      throw new Error(`the prompt input did not accept the text (holds ${JSON.stringify(evidence.inputValue)})`);
    }

    // ── step 6: satisfy whatever host interaction still gates Generate ───────
    // See PREREQ_CLICK_LIMIT for the measurement. The prompt is already typed, so
    // a Generate still disabled here is waiting on something ELSE — on
    // `ab-ship-mimo-02` a resource pick the host has to answer.
    evidence.generateDisabled = await page.evalJs(disabledExpr(labelExpr(GENERATE_LABEL)));
    evidence.prereqClicks = [];
    if (evidence.generateDisabled === true) {
      const offered = JSON.parse(await page.evalJs(`JSON.stringify(${CLICKABLES}
        .filter((e) => !(e.disabled === true || e.getAttribute('aria-disabled') === 'true'))
        .map((e) => ((e.textContent || e.value || '') + '').trim())
        .filter((l) => {
          const k = l.toLowerCase();
          return k !== ${JSON.stringify(GENERATE_LABEL)} && k !== ${JSON.stringify(POST_LABEL)};
        }))`));
      evidence.prereqOffered = offered;
      for (const label of offered.slice(0, PREREQ_CLICK_LIMIT)) {
        // Clicked by INDEX-FREE label match, re-resolved each time: a pick
        // typically RE-LABELS the control it came from (`Select Model` becomes
        // `Model: FLUX.1 [dev]`), so an element handle or a position captured
        // before the click names something that is no longer there.
        const hit = await page.evalJs(`(() => {
          const el = ${CLICKABLES}.find((e) =>
            ((e.textContent || e.value || '') + '').trim() === ${JSON.stringify(label)}
            && !(e.disabled === true || e.getAttribute('aria-disabled') === 'true'));
          if (!el) return false;
          el.click();
          return true;
        })()`);
        evidence.prereqClicks.push(`${label}${hit ? '' : ' (vanished)'}`);
        await sleep(PREREQ_SETTLE_MS);
        evidence.generateDisabled = await page.evalJs(disabledExpr(labelExpr(GENERATE_LABEL)));
        if (evidence.generateDisabled !== true) break;
      }
    }
    await captureHostEvidence();

    // ── the UNCONSENTED arm's own step 7, and it grades a different thing ─────
    // 🔴 IT BRANCHES *BEFORE* THE STILL-DISABLED THROW BELOW, AND THAT IS THE
    // WHOLE POINT. On an unconsented token, an app that DISABLES Generate until
    // the scope arrives is CORRECT — and in this arm the scope can never arrive,
    // because a grant would need a `TOKEN_REFRESH` push and inline mode receives
    // none (see `CONSENT_MESSAGE`). Reaching the default arm's throw would fail
    // exactly the apps that handle the state best.
    if (UNCONSENTED) {
      // Click Generate when it is live; a disabled one is a legitimate design
      // here, so its state is REPORTED and decides nothing.
      evidence.generateClicked = evidence.generateDisabled !== true
        && !!(await page.evalJs(`(() => {
          const hit = ${labelExpr(GENERATE_LABEL)};
          if (!hit || hit.disabled === true || hit.getAttribute('aria-disabled') === 'true') return false;
          hit.click();
          return true;
        })()`));
      await sleep(CONSENT_SETTLE_MS);
      await captureHostEvidence();
      try { evidence.observed = JSON.parse(await page.evalJs(READ_RECORDER)).join('>'); } catch { /* none */ }
      pass = evidence.consentRequested === true;
      if (!pass) {
        // 🔴 UNMEASURED BEFORE `no`, for the same reason `pickerBlind` exists: an
        // ask this oracle could not have OBSERVED and an app that never asked are
        // the same bare `no`, and only one of them is about the block.
        if (consentBlind()) {
          unmeasured = true;
          throw new Error(`harness error: no ${CONSENT_MESSAGE} was observed on the UNCONSENTED arm, ` +
            `and this oracle could not instrument ${page.shim.messageUnmatched} response(s) carrying ` +
            `the SDK's inline transport (${evidence.messageShim}) — so an ask could not have been ` +
            `SEEN here. That is an instrument failure, not a verdict about the block: ` +
            `patchInlineTransport's sendMessage needle no longer matches this bundle.`);
        }
        reason = `the block never asked the host for consent on an UNCONSENTED token ` +
          `(scopes=none, viewer signed in): Generate was ` +
          `${evidence.generateDisabled === true ? 'disabled and nothing else asked either'
            : `clicked and sent ${evidence.hostMessages}`}. ` +
          `A real first-time viewer of this block gets whatever its no-consent path does — ` +
          `for ab-ship-mimo-02 that was "Generation failed. Please try again.".`;
      }
    } else if (POST_PATH) {
      await gradeThePostPath();
    } else {
      await gradeTheGenerateClick();
    }
  } catch (e) {
    reason = e.message;
    // 🔴 REPORT WHAT WAS ON THE PAGE WHEN IT FAILED. A bare `no` is the
    // capability confound in miniature — "the model built nothing", "the model
    // built it with different testids" and "the page never loaded" all look the
    // same without this.
    if (page) {
      evidence.bodyHtml = await page.bodyHtml();
      try { evidence.observed = JSON.parse(await page.evalJs(READ_RECORDER)).join('>'); } catch { /* none recorded */ }
      try { await captureHostEvidence(); } catch { /* the page is gone */ }
    }
  } finally {
    try { c.close(); } catch { /* already gone */ }
    proc.kill('SIGKILL');
    server?.close();
  }

  // 🔴 THREE STATES, NOT TWO. 0 = the block passed, 1 = the block failed, 2 = THIS
  // HARNESS could not measure — the code `oracle.sh` already turns into
  // `RENDER=unmeasured` rather than a verdict. `pass` is forced false on the
  // unmeasured path so no reader can take the pair for a verdict either way.
  if (unmeasured) pass = false;
  // 🔴 THE POST ARM STATES ITS OWN CEILING IN ITS OWN OUTPUT, AS A FIELD, NOT ONLY
  // IN A DOC. This arm is the one whose green is easiest to over-read — it answers
  // a submit and creates a post, so "the post path works" is one careless sentence
  // away from "posting works on civitai.com".
  //
  // 🔴 TWO LAYERS, BECAUSE THE PROSE CANNOT RIDE ON THE ROW. `postArmCeiling` is the
  // full sentence and it lives here, in the assertion's JSON — which `grade.sh` does
  // not parse. `postCeiling` is a short TOKEN, which `oracle.sh` puts on the summary
  // line and `grade.sh` carries onto the grade row, so a reader of a matrix row cannot
  // get this arm's verdict without it either. See `POST_ARM_CEILING_TOKEN` in
  // `_cdp.mjs` for why it is a token and why it is appended only on this arm.
  if (POST_PATH) {
    evidence.postCeiling = POST_ARM_CEILING_TOKEN;
    evidence.postArmCeiling = 'proves the block\'s post BRANCH exists and its payload satisfies '
      + 'the host payload gate as the SDK\'s own mock host implements it (non-empty `sources`, and '
      + 'a `workflow` source that actually produced images). Does NOT prove the real host accepts '
      + 'it: the platform re-resolves every source server-side, re-checks the posts:write:self '
      + 'grant, opens a viewer confirm and moderates the outputs. No credential was used and '
      + 'nothing left the page.';
  }
  console.log(JSON.stringify({ assertion: 'genpost', pass, unmeasured, reason, ...evidence }));
  process.exit(unmeasured ? 2 : (pass ? 0 : 1));
}

main().catch((e) => {
  console.log(JSON.stringify({ assertion: 'genpost', pass: false, reason: `harness error: ${e.message}` }));
  process.exit(2);
});
