// Shared plumbing for the brief assertions in this directory. NOT a brief and
// NOT an assertion: the `_` prefix keeps it out of `briefs/*.brief.txt` and out
// of the oracle's `briefs/<name>.assert.mjs` lookup.
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
// working app as a timeout because of that would be the capability confound
// arriving through the instrument.
//
// 🔴 AND WHY VIA `window.__CIVITAI_BLOCK_CONTEXT__` RATHER THAN A postMessage.
// A block's `IframeTransport` DROPS any inbound message whose `event.origin` is
// not in the allowlist baked in at build time — `.env.production` of a
// scaffolded app allowlists `https://civitai.com` and nothing else — so a
// `BLOCK_INIT` posted from whatever port an oracle serves on is discarded before
// it reaches the block, silently. The SDK's own transport detector branches
// FIRST on this global (`BlockTransportDetector.detect`:
// `win.__CIVITAI_BLOCK_CONTEXT__` present -> InlineTransport), which reads the
// bootstrap straight off the window with no origin gate. So this is the
// supported same-document path, not a bypass of the cross-origin one.
// Set `CIVITAI_ASSERT_NO_HOST=1` to skip the injection — that is the control
// arm, and it is what proves the injection is doing something.
//
// ⚠ It is a v1 stub on the SDK side: `InlineTransport.sendRequest` REJECTS and
// host pushes never arrive. So a block that AWAITS a host reply — a real
// generation, a real post — never gets one here. Every assertion in this
// directory must be satisfiable WITHOUT a host round-trip, and must say so in
// its own doc. A green cell is never evidence that the money path works.
//
// 🔴 AND IT PRESENTS A SIGNED-IN VIEWER, BECAUSE AN ANONYMOUS-ONLY HOST GRADES
// A BRANCH THE BRIEF CANNOT BE SATISFIED IN. This used to seed `viewer: null`
// with an "inert on purpose" comment, and the cost was measured on
// `ab-genpost-mimo-01` (2026-09-21): a reasonable generate-then-post app that
// gates its UI on `ready && !viewer` — the shape the CLI's own `page-money`
// scaffold ships, sign-in CTA and all — rendered `<div data-testid="status">
// ready</div><p>Please sign in to generate images.</p>` and graded `RENDER=no`
// with `timed out ... waiting for [data-testid="prompt"]`. That verdict was
// about the harness's viewer, not about the model. See `HOST_VIEWER`.

import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { readFile, mkdtemp, access } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, extname, normalize, sep } from 'node:path';

/** The deadline each individual wait gets. A built bundle has to download,
 * parse and mount before its first element exists; a static file is there at
 * first paint. */
export const WAIT_MS = Number(process.env.CIVITAI_ASSERT_WAIT_MS || 15000);

/** The control arm for the viewer, exactly as `CIVITAI_ASSERT_NO_HOST` is the
 * control arm for the handshake: set `CIVITAI_ASSERT_ANON_VIEWER=1` and the
 * oracle presents an anonymous viewer instead. It is what proves the seeded
 * viewer is doing something, and it is the only way to grade a block's
 * signed-out branch deliberately. */
export const ANON_VIEWER = process.env.CIVITAI_ASSERT_ANON_VIEWER === '1';

/**
 * The `BLOCK_INIT.viewer` this oracle presents.
 *
 * 🔴 SIGNED IN BY DEFAULT, AND THE KEY SET IS PRODUCTION'S, NOT A CONVENIENT
 * ONE. civitai.com funnels both block hosts through one choke point —
 * `withSignedInFlag()` in civitai/civitai `src/components/AppBlocks/
 * projectBlockInit.ts`, whose fields are PICKED, never spread — and it returns
 * exactly `{ id, username, signedIn: true }` for a present viewer and `null`
 * for an anonymous one. No `status`: the platform withholds the viewer's
 * ban/mute state from third-party iframes (civitai #2521). This object is that
 * object. Widening it (a `status`, an `email`, a `roles`) would let a block
 * read a field production never sends and still grade green here — the
 * both-wrong-blind shape, arriving through the instrument.
 *
 * 🔴 WHY SIGNED IN IS THE RIGHT DEFAULT, MEASURED RATHER THAN ASSUMED:
 *   - Every other host emulation in this ecosystem defaults to a signed-in
 *     viewer and makes anonymous the OPT-IN: the SDK's `createMockHost`
 *     (`DEFAULT_VIEWER = { id: 2, username: 'dev-viewer', signedIn: true }`),
 *     `createLiveHost` (which has an `anonFallbackViewer`, i.e. a fallback),
 *     and this repo's own `page-money` harness
 *     (`internal/scaffold/templates/page-money/src/Harness.tsx.tmpl`, where
 *     `?viewer=anon` is the knob you add to get an anonymous one).
 *   - The platform REFUSES the `genpost` brief's behaviour to an anonymous
 *     subject. A block token minted for an anonymous viewer carries
 *     `sub: "anon"`, and civitai's block-scope middleware hard-rejects
 *     `posts:write:self` for that subject and requires a positive `buzzBudget`
 *     claim for `ai:write:budgeted`. So an anonymous-only oracle grades a
 *     branch in which the thing the brief asks for cannot exist.
 *   - The `page-money` scaffold — which every genpost cell derives from —
 *     ships an explicit `const anon = ready && !viewer` branch rendering a
 *     "Sign in to generate" CTA. Under an anonymous-only oracle EVERY
 *     scaffold-derived app renders that CTA and no cell can ever pass for a
 *     reason that is about the model.
 *
 * ⚠ IT BUYS THE BLOCK NO CAPABILITY, AND THAT IS CHECKED, NOT ASSUMED. The
 * token below stays `{ raw: '', scopes: [] }`, and on the platform every
 * privileged path re-derives identity from the JWT `sub` rather than from
 * anything the block was handed — so a populated `viewer` cannot make a
 * generation or a post appear to succeed. `InlineTransport.sendRequest`
 * rejects unconditionally anyway. `fxGenpostBootstrapProbe` in
 * dogfood_oracle_test.go asserts the seeded object from inside the page.
 */
export const HOST_VIEWER = ANON_VIEWER
  ? null
  : { id: 2, username: 'dogfood-oracle-viewer', signedIn: true };

/**
 * The inline bootstrap a host injects on its own document. Field names and
 * shape follow the SDK's `BLOCK_INIT` payload, because `InlineTransport` feeds
 * this object to the same `snapshotFromInit()` the iframe path uses — a missing
 * field surfaces as `undefined` inside the block, not as an error here.
 *
 * The CREDENTIAL half is deliberately inert: no real token, an empty scope
 * list. A block that tries to spend with it gets a rejected request, which is
 * the correct outcome for a grading run. The VIEWER half is not inert and must
 * not be — see `HOST_VIEWER`.
 *
 * `context.viewerUserId` / `viewerUsername` track `HOST_VIEWER` rather than
 * being hardcoded: on the page slot the host sends the viewer's identity
 * through BOTH channels (`PageBlockHost.buildContext()` is not run through the
 * model-slot allowlist that drops them), so a block is free to gate on either,
 * and a bootstrap where the two disagree is a state no host produces.
 */
export const HOST_BOOTSTRAP = {
  blockInstanceId: 'dogfood-oracle-instance',
  blockId: 'dogfood-oracle-block',
  appId: 'dogfood-oracle-app',
  token: { raw: '', scopes: [], expiresAt: new Date(0).toISOString() },
  context: {
    slotId: 'app.page',
    entityType: 'none',
    slug: 'dogfood-oracle-block',
    subPath: '',
    viewerUserId: HOST_VIEWER ? HOST_VIEWER.id : null,
    viewerUsername: HOST_VIEWER ? HOST_VIEWER.username : null,
    theme: 'light',
  },
  settings: { publisherSettings: {}, userSettings: {} },
  viewer: HOST_VIEWER,
  theme: 'light',
  renderMode: 'iframe',
};

export const SEND_HOST_INIT = process.env.CIVITAI_ASSERT_NO_HOST !== '1';

/** What the cell reports about the viewer the block was shown. A verdict read
 * without it cannot tell "the model built nothing" from "the model built a
 * sign-in CTA and the harness was anonymous". */
export const HOST_VIEWER_LABEL = HOST_VIEWER ? 'signed-in' : 'anonymous';

const MIME = {
  '.html': 'text/html', '.js': 'text/javascript', '.mjs': 'text/javascript',
  '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml',
  '.png': 'image/png', '.jpg': 'image/jpeg', '.ico': 'image/x-icon',
  '.woff2': 'font/woff2', '.map': 'application/json',
};

/** Serve `dir` on an ephemeral loopback port, refusing to walk out of it. */
export async function serve(dir) {
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

export async function findChrome() {
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

export async function launch() {
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

/** Minimal CDP client. `flatten: true` puts page-session messages on the browser
 * socket, so one connection is enough. */
export function cdp(endpoint) {
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

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/**
 * Resolve the assertion's target to a URL, serving a directory if that is what
 * was handed in. Returns `{ url, server }`; `server` is null for a URL target.
 */
export async function resolveTarget(target) {
  if (/^https?:\/\//.test(target)) return { url: target, server: null };
  const server = await serve(normalize(target).replace(/\/+$/, ''));
  return { url: server.url, server };
}

/**
 * Open a page, seeding the host bootstrap ahead of the document's first script,
 * and return the helpers every assertion in this directory needs.
 *
 * 🔴 `Page.addScriptToEvaluateOnNewDocument` BEFORE `Page.navigate`, never a
 * `Runtime.evaluate` after the load. The block's bundle calls `getTransport()`
 * on its first module evaluation and the transport is a process-wide singleton
 * whose FIRST construction wins, so a global set even one tick late is read by
 * nothing. This CDP method runs its source before any script in the document.
 */
export async function openPage(c, url) {
  const { targetId } = await c.send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await c.send('Target.attachToTarget', { targetId, flatten: true });
  await c.send('Page.enable', {}, sessionId);
  await c.send('Runtime.enable', {}, sessionId);
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
  /** Type `text` into the focused element with real key/input events.
   *
   * 🔴 TYPE, NEVER ASSIGN `.value`. A scripted assignment fires no `input`
   * event, so a React app built with `useState` + `onChange` — the shape both
   * page templates ship — would never see the characters and would report an
   * empty result. That failure is indistinguishable from "the model did not
   * build the thing", which is the exact confound this harness exists not to
   * introduce. `Input.insertText` against a focused element dispatches real
   * input events, so a React app and a plain-DOM app are graded identically. */
  const typeInto = async (selector, text) => {
    await evalJs(`(() => { const e = document.querySelector('${selector}');
      e.focus(); if ('value' in e) e.value = ''; return true; })()`);
    await c.send('Input.insertText', { text }, sessionId);
    return evalJs(`document.querySelector('${selector}').value ?? null`);
  };
  const bodyHtml = async () => {
    try {
      const r = await c.send('Runtime.evaluate', {
        expression: `document.body ? document.body.innerHTML.slice(0, 1200) : '(no body)'`,
        returnByValue: true,
      }, sessionId);
      return r?.result?.value ?? null;
    } catch { return null; }
  };
  return { sessionId, evalJs, waitFor, typeInto, bodyHtml };
}

/**
 * The clickable-by-visible-label expression every brief here uses. Briefs name a
 * LABEL, not a selector, so the block is free to mark its buttons up however it
 * likes; `role="button"` counts. `label` must already be lowercase.
 */
export const CLICKABLES =
  `[...document.querySelectorAll('button, input[type=button], input[type=submit], [role=button]')]`;

export function labelExpr(label) {
  return `${CLICKABLES}.find((e) => ((e.textContent || e.value || '') + '').trim().toLowerCase() === '${label}')`;
}
