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
// So NO generation and NO post can complete here, ever, on any machine, with or
// without a credential. An assertion that waited for a rendered image or a post
// id would time out against a perfect app.
//
// 🔴 ONE REQUEST CLASS IS ANSWERED, AND IT IS NOT A SPENDING ONE. Since
// 2026-09-25 the oracle answers a host RESOURCE PICK (`OPEN_RESOURCE_PICKER` /
// `OPEN_CHECKPOINT_PICKER`) with the resource the SDK's own mock host resolves
// with, because an app that gates Generate behind `openPicker` could otherwise
// never reach `generating` — measured on `ab-ship-mimo-02`, which graded
// `RENDER=no observed=ready generateDisabled=true` for that reason and no other.
// Every OTHER request type still rejects with the SDK's own
// `InlineTransport.sendRequest is not implemented in v1`, and this file reports
// which ones did on the cell as `hostRefused` — so the paragraph above is checked
// per cell rather than promised in a comment. See `HOST_RESOURCE_PICKS` and
// `patchInlineTransport` in `_cdp.mjs`.
//
// 🔴 THAT STAYS TRUE WITH THE BLOCK'S SCOPES SEEDED. The bootstrap presents the
// scope list the block's own manifest declares (argv[3], handed down by
// oracle.sh) so that a consent-gated Generate handler takes the granted branch
// instead of the refused one — but `token.raw` is still `''` and `sendRequest`
// still rejects unconditionally. Scopes buy a BRANCH, never a CAPABILITY. See
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

import { launch, cdp, openPage, parseScopes, resolveTarget, SEND_HOST_INIT, HOST_VIEWER_LABEL, HOST_PICKS_LABEL, CLICKABLES, labelExpr, sleep } from './_cdp.mjs';

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
    hostScopes: SCOPES.join(',') || 'none',
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
    evidence.pickerShim = `${HOST_PICKS_LABEL}:docs=${page.shim.documents},sites=${page.shim.sites}` +
      (page.shim.unmatched ? `,unmatched=${page.shim.unmatched}` : '') +
      (page.shim.skipped ? `,skipped=${page.shim.skipped}` : '');
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
  console.log(JSON.stringify({ assertion: 'genpost', pass, unmeasured, reason, ...evidence }));
  process.exit(unmeasured ? 2 : (pass ? 0 : 1));
}

main().catch((e) => {
  console.log(JSON.stringify({ assertion: 'genpost', pass: false, reason: `harness error: ${e.message}` }));
  process.exit(2);
});
