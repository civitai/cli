#!/usr/bin/env node
// The behavioural assertion for the `celsius` brief. See celsius.md.
//
//   node celsius.assert.mjs <dir>            # serves the dir, drives it, grades it
//   node celsius.assert.mjs http://host:port # grades something already served
//
// Prints one JSON line and exits 0 (pass) or 1 (fail). This is the PREDICATE,
// not the render oracle: it drives a block that is already built and reachable.
// Serving it from inside a trial container, running `civitai app validate` as
// the fail-fast gate ahead of it, and folding the result into a per-cell grade
// are the oracle's job and are not done here.
//
// The browser plumbing — a dependency-free CDP client, the host bootstrap the
// SDK's transport detector reads, and why typing beats assigning `.value` —
// lives in `_cdp.mjs`, shared with every other brief in this directory. Read
// that file's header for the reasoning; it is load-bearing and is not repeated
// here.

import { launch, cdp, openPage, resolveTarget, SEND_HOST_INIT, labelExpr } from './_cdp.mjs';

const TARGET = process.argv[2];
if (!TARGET) {
  console.error('usage: celsius.assert.mjs <dir|url>');
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
// 100 C is 212 F. Chosen because the two common wrong implementations miss it
// by a wide, unmistakable margin — `c * 9/5` alone gives 180, `(c + 32) * 9/5`
// gives 237.6 — so a near-miss is visibly a near-miss in the evidence rather
// than a rounding argument.
const INPUT_C = '100';
const EXPECT_F = '212';
const SEL_IN = '[data-testid="celsius"]';
const SEL_OUT = '[data-testid="fahrenheit"]';
const BUTTON_LABEL = 'convert';

async function main() {
  const { url, server } = await resolveTarget(TARGET);
  const { proc, ws } = await launch();
  const c = cdp(ws);
  await c.open;

  const evidence = {
    target: TARGET, url, input: INPUT_C, expected: EXPECT_F, hostInit: SEND_HOST_INIT,
  };
  let pass = false;
  let reason = null;
  let page = null;

  try {
    page = await openPage(c, url);

    // ── step 1: the input exists ─────────────────────────────────────────────
    await page.waitFor(`!!document.querySelector('${SEL_IN}')`, `${SEL_IN} to appear`);

    // ── step 2: type the value (see _cdp.mjs on why typing, not assigning) ──
    evidence.inputValue = await page.typeInto(SEL_IN, INPUT_C);
    if (evidence.inputValue !== INPUT_C) {
      throw new Error(`the input did not accept "${INPUT_C}" (holds ${JSON.stringify(evidence.inputValue)})`);
    }

    // ── step 3: click the button labelled Convert ────────────────────────────
    // Matched on its visible label, case-insensitively, because the brief names
    // a LABEL and not a selector — the block is free to mark the button up
    // however it likes. `role="button"` counts.
    const clicked = await page.evalJs(`(() => {
      const hit = ${labelExpr(BUTTON_LABEL)};
      if (!hit) return null;
      hit.click();
      return true;
    })()`);
    if (!clicked) {
      evidence.buttonLabels = await page.evalJs(`JSON.stringify([...document.querySelectorAll('button, input[type=button], input[type=submit], [role=button]')].map((e) => ((e.textContent || e.value || '') + '').trim()))`);
      throw new Error(`no clickable element labelled "${BUTTON_LABEL}"`);
    }

    // ── step 4: read the output ──────────────────────────────────────────────
    await page.waitFor(`(() => { const e = document.querySelector('${SEL_OUT}');
      return !!e && (e.textContent || '').trim().length > 0; })()`,
      `${SEL_OUT} to hold text`);
    evidence.observed = await page.evalJs(`(document.querySelector('${SEL_OUT}').textContent || '').trim()`);

    pass = evidence.observed === EXPECT_F;
    if (!pass) reason = `observed ${JSON.stringify(evidence.observed)}, expected ${JSON.stringify(EXPECT_F)}`;
  } catch (e) {
    reason = e.message;
    // 🔴 REPORT WHAT WAS ON THE PAGE WHEN IT FAILED. A bare `no` is the
    // capability confound in miniature — "the model built nothing", "the model
    // built it with different testids" and "the page never loaded" all look the
    // same without this.
    if (page) evidence.bodyHtml = await page.bodyHtml();
  } finally {
    try { c.close(); } catch { /* already gone */ }
    proc.kill('SIGKILL');
    server?.close();
  }

  console.log(JSON.stringify({ assertion: 'celsius', pass, reason, ...evidence }));
  process.exit(pass ? 0 : 1);
}

main().catch((e) => {
  console.log(JSON.stringify({ assertion: 'celsius', pass: false, reason: `harness error: ${e.message}` }));
  process.exit(2);
});
