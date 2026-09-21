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
// 🔴 NO DEPENDENCIES, ON PURPOSE. Puppeteer/Playwright would be a node_modules
// tree inside a harness whose whole point is that a trial container holds an OS,
// node and nothing else. This speaks CDP over node's built-in WebSocket (node
// >= 22) to whatever Chromium is on the box.
//
// 🔴 WHY IT EMULATES A HOST BEFORE THE BUNDLE RUNS. An App Block is built to be
// iframed by civitai.com, which delivers its runtime context over a postMessage
// `BLOCK_INIT` handshake. Opened directly, no parent ever sends one, so a block
// that gates its UI on `ready` parks on "Connecting to host…" forever — measured
// on the `page-money` scaffold, which renders nothing but a spinner. Grading a
// working converter as a timeout because of that would be the capability
// confound arriving through the instrument, again.
//
// 🔴 AND WHY VIA `window.__CIVITAI_BLOCK_CONTEXT__` RATHER THAN A postMessage.
// A block's `IframeTransport` DROPS any inbound message whose `event.origin` is
// not in the allowlist baked in at build time — `.env.production` of a
// scaffolded app allowlists `https://civitai.com` and nothing else — so a
// `BLOCK_INIT` posted from whatever port this oracle serves on is discarded
// before it reaches the block, silently. The SDK's own transport detector
// branches FIRST on this global (`BlockTransportDetector.detect`:
// `win.__CIVITAI_BLOCK_CONTEXT__` present -> InlineTransport), which reads the
// bootstrap straight off the window with no origin gate. So this is the
// supported same-document path, not a bypass of the cross-origin one.
// MEASURED 2026-09-20 against a freshly built `page-money` scaffold: without the
// bootstrap the body reads "LOADING / Connecting to host…"; with it the app
// renders its full generation form.
// Set `CIVITAI_ASSERT_NO_HOST=1` to skip the injection — that is the control
// arm, and it is what proves the injection is doing something.
//
// ⚠ It is a v1 stub on the SDK side: `sendRequest` rejects and host pushes never
// arrive, so a block that AWAITS a host reply still hangs. This brief needs no
// host round-trip, so that gap is out of its scope — but do not read a pass here
// as evidence that the money path works.
//
// 🔴 WHY IT TYPES INSTEAD OF ASSIGNING `input.value`. Setting `.value` from
// script does not fire an `input` event, so a React app built with
// `useState` + `onChange` — the shape both page templates ship — would never
// see the digits and would report an empty result. That failure is
// indistinguishable from "the model did not build the converter", which is the
// exact confound this whole arc is trying not to introduce. `Input.insertText`
// against a focused element dispatches real input events, so a React app and a
// plain-DOM app are graded by the same instrument.

import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { readFile, mkdtemp, access } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname, normalize, sep } from 'node:path';

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
// The deadline each individual wait gets. A built bundle has to download, parse
// and mount before the input exists; a static file is there at first paint.
const WAIT_MS = Number(process.env.CIVITAI_ASSERT_WAIT_MS || 15000);

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

// The inline bootstrap a host injects on its own document. Field names and shape
// follow the SDK's `BLOCK_INIT` payload, because `InlineTransport` feeds this
// object to the same `snapshotFromInit()` the iframe path uses — a missing field
// surfaces as `undefined` inside the block, not as an error here.
//
// The values are deliberately inert: no real token, no real viewer, an empty
// scope list. A block that tries to spend with them gets a rejected request,
// which is the correct outcome for a grading run.
const HOST_BOOTSTRAP = {
  blockInstanceId: 'dogfood-oracle-instance',
  blockId: 'dogfood-oracle-block',
  appId: 'dogfood-oracle-app',
  token: { raw: '', scopes: [], expiresAt: new Date(0).toISOString() },
  context: {
    slotId: 'app.page',
    entityType: 'none',
    slug: 'dogfood-oracle-block',
    subPath: '',
    viewerUserId: null,
    viewerUsername: null,
    theme: 'light',
  },
  settings: { publisherSettings: {}, userSettings: {} },
  viewer: null,
  theme: 'light',
  renderMode: 'iframe',
};
const SEND_HOST_INIT = process.env.CIVITAI_ASSERT_NO_HOST !== '1';

const MIME = {
  '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript',
  '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml',
  '.png': 'image/png', '.jpg': 'image/jpeg', '.ico': 'image/x-icon',
  '.woff2': 'font/woff2', '.map': 'application/json',
};

async function serve(dir) {
  const srv = createServer(async (req, res) => {
    try {
      let p = decodeURIComponent(new URL(req.url, 'http://x').pathname);
      if (p.endsWith('/')) p += 'index.html';
      // Containment check: a block under test is model-authored, and so is any
      // path it asks for. `..` must not escape the directory being graded.
      const full = normalize(join(dir, p));
      if (full !== dir && !full.startsWith(dir + sep)) { res.writeHead(403).end(); return; }
      const body = await readFile(full);
      res.writeHead(200, { 'content-type': MIME[extname(full)] || 'application/octet-stream' });
      res.end(body);
    } catch {
      res.writeHead(404).end('not found');
    }
  });
  await new Promise((r) => srv.listen(0, '127.0.0.1', r));
  return { url: `http://127.0.0.1:${srv.address().port}/`, close: () => srv.close() };
}

async function findChrome() {
  if (process.env.CIVITAI_CHROME) return process.env.CIVITAI_CHROME;
  const names = ['chromium', 'chromium-browser', 'google-chrome', 'google-chrome-stable', 'chrome'];
  for (const d of (process.env.PATH || '').split(':')) {
    for (const n of names) {
      const p = join(d, n);
      try { await access(p); return p; } catch { /* next */ }
    }
  }
  throw new Error('no Chromium on PATH; set CIVITAI_CHROME to a browser binary');
}

async function launch() {
  const bin = await findChrome();
  const profile = await mkdtemp(join(tmpdir(), 'civitai-assert-'));
  const proc = spawn(bin, [
    '--headless=new', '--remote-debugging-port=0', `--user-data-dir=${profile}`,
    '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage',
    '--no-first-run', '--no-default-browser-check', 'about:blank',
  ], { stdio: ['ignore', 'pipe', 'pipe'] });

  const ws = await new Promise((resolve, reject) => {
    let buf = '';
    const to = setTimeout(() => reject(new Error(`browser never printed a DevTools endpoint:\n${buf}`)), WAIT_MS);
    proc.stderr.on('data', (d) => {
      buf += d;
      const m = buf.match(/ws:\/\/\S+/);
      if (m) { clearTimeout(to); resolve(m[0]); }
    });
    proc.on('exit', (c) => { clearTimeout(to); reject(new Error(`browser exited ${c}:\n${buf}`)); });
  });
  return { proc, ws };
}

// Minimal CDP client. `flatten: true` puts page-session messages on the browser
// socket, so one connection is enough.
function cdp(endpoint) {
  const sock = new WebSocket(endpoint);
  let id = 0;
  const pending = new Map();
  const events = [];
  sock.addEventListener('message', (e) => {
    const m = JSON.parse(e.data);
    if (m.id !== undefined && pending.has(m.id)) {
      const { resolve, reject } = pending.get(m.id);
      pending.delete(m.id);
      m.error ? reject(new Error(`${m.error.message} (${JSON.stringify(m.error.data ?? null)})`)) : resolve(m.result);
    } else if (m.method) {
      events.push(m);
    }
  });
  const open = new Promise((res, rej) => {
    sock.addEventListener('open', res);
    sock.addEventListener('error', () => rej(new Error('devtools socket error')));
  });
  return {
    open,
    close: () => sock.close(),
    send(method, params = {}, sessionId) {
      const msg = { id: ++id, method, params };
      if (sessionId) msg.sessionId = sessionId;
      return new Promise((resolve, reject) => {
        pending.set(msg.id, { resolve, reject });
        sock.send(JSON.stringify(msg));
      });
    },
    sawEvent: (name) => events.some((e) => e.method === name),
  };
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  let server = null;
  let url = TARGET;
  if (!/^https?:\/\//.test(TARGET)) {
    server = await serve(normalize(TARGET).replace(/\/+$/, ''));
    url = server.url;
  }

  const { proc, ws } = await launch();
  const c = cdp(ws);
  await c.open;

  const evidence = {
    target: TARGET, url, input: INPUT_C, expected: EXPECT_F, hostInit: SEND_HOST_INIT,
  };
  let pass = false;
  let reason = null;
  let sessionId = null;

  try {
    const { targetId } = await c.send('Target.createTarget', { url: 'about:blank' });
    ({ sessionId } = await c.send('Target.attachToTarget', { targetId, flatten: true }));
    await c.send('Page.enable', {}, sessionId);
    await c.send('Runtime.enable', {}, sessionId);
    // 🔴 BEFORE `Page.navigate`, AND VIA addScriptToEvaluateOnNewDocument — not
    // a `Runtime.evaluate` after the load. The block's bundle calls
    // `getTransport()` on its first module evaluation and the transport is a
    // process-wide singleton whose FIRST construction wins, so a global set even
    // one tick late is read by nothing. This CDP method runs its source before
    // any script in the document.
    if (SEND_HOST_INIT) {
      await c.send('Page.addScriptToEvaluateOnNewDocument', {
        source: `window.__CIVITAI_BLOCK_CONTEXT__ = ${JSON.stringify(HOST_BOOTSTRAP)};`,
      }, sessionId);
    }
    await c.send('Page.navigate', { url }, sessionId);

    const evalJs = async (expr) => {
      const r = await c.send('Runtime.evaluate', {
        expression: expr, returnByValue: true, awaitPromise: true,
      }, sessionId);
      if (r.exceptionDetails) throw new Error(`page threw: ${r.exceptionDetails.text}`);
      return r.result.value;
    };
    const waitFor = async (expr, what) => {
      const deadline = Date.now() + WAIT_MS;
      for (;;) {
        try { if (await evalJs(expr)) return true; } catch { /* pre-navigation */ }
        if (Date.now() > deadline) throw new Error(`timed out after ${WAIT_MS}ms waiting for ${what}`);
        await sleep(100);
      }
    };

    // ── step 1: the input exists ─────────────────────────────────────────────
    await waitFor(`!!document.querySelector('${SEL_IN}')`, `${SEL_IN} to appear`);

    // ── step 2: type the value (see the header on why typing, not assigning) ─
    await evalJs(`(() => { const e = document.querySelector('${SEL_IN}');
      e.focus(); if ('value' in e) e.value = ''; return true; })()`);
    await c.send('Input.insertText', { text: INPUT_C }, sessionId);
    evidence.inputValue = await evalJs(`document.querySelector('${SEL_IN}').value ?? null`);
    if (evidence.inputValue !== INPUT_C) {
      throw new Error(`the input did not accept "${INPUT_C}" (holds ${JSON.stringify(evidence.inputValue)})`);
    }

    // ── step 3: click the button labelled Convert ────────────────────────────
    // Matched on its visible label, case-insensitively, because the brief names
    // a LABEL and not a selector — the block is free to mark the button up
    // however it likes. `role="button"` counts.
    const clicked = await evalJs(`(() => {
      const cand = [...document.querySelectorAll('button, input[type=button], input[type=submit], [role=button]')];
      const hit = cand.find((e) => ((e.textContent || e.value || '') + '').trim().toLowerCase() === '${BUTTON_LABEL}');
      if (!hit) return null;
      hit.click();
      return true;
    })()`);
    if (!clicked) {
      evidence.buttonLabels = await evalJs(`JSON.stringify([...document.querySelectorAll('button, input[type=button], input[type=submit], [role=button]')].map((e) => ((e.textContent || e.value || '') + '').trim()))`);
      throw new Error(`no clickable element labelled "${BUTTON_LABEL}"`);
    }

    // ── step 4: read the output ──────────────────────────────────────────────
    await waitFor(`(() => { const e = document.querySelector('${SEL_OUT}');
      return !!e && (e.textContent || '').trim().length > 0; })()`,
      `${SEL_OUT} to hold text`);
    evidence.observed = await evalJs(`(document.querySelector('${SEL_OUT}').textContent || '').trim()`);

    pass = evidence.observed === EXPECT_F;
    if (!pass) reason = `observed ${JSON.stringify(evidence.observed)}, expected ${JSON.stringify(EXPECT_F)}`;
  } catch (e) {
    reason = e.message;
    // 🔴 REPORT WHAT WAS ON THE PAGE WHEN IT FAILED. A bare `no` is the
    // capability confound in miniature — "the model built nothing", "the model
    // built it with different testids" and "the page never loaded" all look the
    // same without this.
    try {
      if (sessionId) {
        const r = await c.send('Runtime.evaluate', {
          expression: `document.body ? document.body.innerHTML.slice(0, 1200) : '(no body)'`,
          returnByValue: true,
        }, sessionId);
        evidence.bodyHtml = r?.result?.value ?? null;
      }
    } catch { /* the page may be unreachable; the reason above still stands */ }
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
