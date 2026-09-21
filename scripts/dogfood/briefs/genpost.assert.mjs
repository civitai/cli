#!/usr/bin/env node
// The behavioural assertion for the `genpost` brief. See genpost.md.
//
//   node genpost.assert.mjs <dir>            # serves the dir, drives it, grades it
//   node genpost.assert.mjs http://host:port # grades something already served
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

import { launch, cdp, openPage, resolveTarget, SEND_HOST_INIT, CLICKABLES, labelExpr } from './_cdp.mjs';

const TARGET = process.argv[2];
if (!TARGET) {
  console.error('usage: genpost.assert.mjs <dir|url>');
  process.exit(2);
}
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

  const evidence = { target: TARGET, url, prompt: PROMPT_TEXT, hostInit: SEND_HOST_INIT };
  let pass = false;
  let reason = null;
  let page = null;

  try {
    page = await openPage(c, url);

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

    // ── step 4: the Generate control exists and is usable ────────────────────
    evidence.generateDisabled = await page.evalJs(disabledExpr(labelExpr(GENERATE_LABEL)));
    if (evidence.generateDisabled === null) {
      evidence.buttonLabels = await page.evalJs(
        `JSON.stringify(${CLICKABLES}.map((e) => ((e.textContent || e.value || '') + '').trim()))`);
      throw new Error(`no clickable element labelled "${GENERATE_LABEL}"`);
    }

    // ── step 5: record the status machine, then type and click ───────────────
    await page.evalJs(INSTALL_RECORDER);
    evidence.inputValue = await page.typeInto(SEL_PROMPT, PROMPT_TEXT);
    if (evidence.inputValue !== PROMPT_TEXT) {
      throw new Error(`the prompt input did not accept the text (holds ${JSON.stringify(evidence.inputValue)})`);
    }
    const clicked = await page.evalJs(`(() => {
      const hit = ${labelExpr(GENERATE_LABEL)};
      if (!hit) return null;
      hit.click();
      return true;
    })()`);
    if (!clicked) throw new Error(`the "${GENERATE_LABEL}" control vanished before it could be clicked`);

    // ── step 6: the click drove the machine ──────────────────────────────────
    await page.waitFor(
      `(window.__dogfoodStatusSeen || []).some((v) => v !== ${JSON.stringify(STATUS_IDLE)})`,
      `the status to leave ${JSON.stringify(STATUS_IDLE)} after clicking ${GENERATE_LABEL}`);
    const seq = JSON.parse(await page.evalJs(READ_RECORDER));
    // 🔴 `observed` IS THE WHOLE SEQUENCE, NOT THE FINAL VALUE. The stub host
    // rejects the request, so a correct app lands on its own failure state a
    // moment later; reporting only where it ended would make every correct app
    // look broken. The sequence shows the transition that actually matters.
    evidence.observed = seq.join('>');
    pass = seq.includes(STATUS_BUSY);
    if (!pass) {
      reason = `the status never read ${JSON.stringify(STATUS_BUSY)}; it went ${JSON.stringify(evidence.observed)}`;
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
    }
  } finally {
    try { c.close(); } catch { /* already gone */ }
    proc.kill('SIGKILL');
    server?.close();
  }

  console.log(JSON.stringify({ assertion: 'genpost', pass, reason, ...evidence }));
  process.exit(pass ? 0 : 1);
}

main().catch((e) => {
  console.log(JSON.stringify({ assertion: 'genpost', pass: false, reason: `harness error: ${e.message}` }));
  process.exit(2);
});
