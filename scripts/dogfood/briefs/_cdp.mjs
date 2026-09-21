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
// its own doc. A green cell is never evidence that the money path works. That
// holds with the block's declared scopes seeded (see `hostBootstrap`): the
// scope list picks a BRANCH, `token.raw` would be the CAPABILITY, and it stays
// empty.
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

/** The deadline each individual IN-PAGE wait gets. A built bundle has to
 * download, parse and mount before its first element exists; a static file is
 * there at first paint. */
export const WAIT_MS = Number(process.env.CIVITAI_ASSERT_WAIT_MS || 15000);

/**
 * The deadline the BROWSER LAUNCH gets — deliberately NOT `WAIT_MS`.
 *
 * 🔴 THESE ARE TWO UNRELATED QUANTITIES AND SHARING ONE NUMBER IS WHAT MADE
 * `build-test` FLAKY ON EVERY PR, INCLUDING DOCS-ONLY ONES. `WAIT_MS` is "how
 * long may a React tree take to mount" — a property of the block under test.
 * This is "how long may a cold browser process take to bind a debugging port on
 * whatever machine CI handed us" — a property of the machine, which nobody
 * here chose and which is far more variable than a mount.
 *
 * Measured 2026-09-21, four `build-test` runs on `main` and on three PRs (one
 * of them a single markdown file): the browser printed its dbus startup noise
 * and then produced NO DevTools endpoint inside 15 s, the assertion exited 2
 * ("nothing was measured"), and the job went red with zero `--- FAIL:` lines.
 * Two of those runs went green on re-run with no change.
 *
 * The number 15000 was never chosen for this. How slow is a GitHub runner?
 * `chromium --version` — a bare exec that does essentially no work — was
 * measured IN THE FAILING JOBS THEMSELVES at 1.9 s, 2.2 s, 2.8 s, 4.7 s, 5.0 s
 * and 6.5 s. The same command on a warm developer box takes 0.02 s. That is
 * 100–300× slower with a 3.4× spread between runs, on the cheapest possible
 * browser invocation; a full launch with a fresh profile is a great deal more
 * work than that. ⚠ Note what this does NOT say: the `--version` time does not
 * separate the passing runs from the failing ones (2.2 s passed, 2.8 s failed),
 * so it is evidence about the ENVIRONMENT's speed class, not a per-run
 * predictor.
 */
export const LAUNCH_MS = Number(process.env.CIVITAI_ASSERT_LAUNCH_MS || 30000);

/**
 * How many times a launch may be attempted before the harness gives up.
 *
 * 🔴 A RETRY IS AN EXCELLENT WAY TO STOP NOTICING A BROKEN BROWSER, so this one
 * is bounded, LOUD ON SUCCESS, and not the primary fix. `launchOnce` below
 * removes the startup work that can actually stall (see `LAUNCH_FLAGS`) and
 * `LAUNCH_MS` gives the launch a budget sized for the machine; this covers only
 * the residual tail of an environment neither this repo nor this browser
 * controls.
 *
 * What keeps it honest is that a retry is never silent. Every attempt prints a
 * line naming the attempt number, the elapsed ms and the browser's own output,
 * and a launch that needed a retry prints a `BROWSER LAUNCH RETRY` warning even
 * though it SUCCEEDED — which is the half a retry normally hides. `ci.yml`'s
 * browser-smoke step greps for exactly that string and raises a `::warning` on
 * it, so "the browser has started failing half the time" is visible in the run
 * rather than absorbed into a green.
 *
 * Set `CIVITAI_ASSERT_LAUNCH_ATTEMPTS=1` to take the retry away — which is what
 * the negative control in `ci.yml` does, so the failing path stays measured.
 */
export const LAUNCH_ATTEMPTS = Math.max(
  1, Number(process.env.CIVITAI_ASSERT_LAUNCH_ATTEMPTS || 2) || 1);

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
 * token below stays `raw: ''`, and on the platform every privileged path
 * re-derives identity from the JWT `sub` rather than from anything the block was
 * handed — so a populated `viewer` cannot make a generation or a post appear to
 * succeed. `InlineTransport.sendRequest` rejects unconditionally anyway.
 * `fxGenpostBootstrapProbe` in dogfood_oracle_test.go asserts the seeded object
 * from inside the page. The scope LIST is not empty any more — see
 * `hostBootstrap` — and that changes which branch a block takes, never what it
 * can complete.
 */
export const HOST_VIEWER = ANON_VIEWER
  ? null
  : { id: 2, username: 'dogfood-oracle-viewer', signedIn: true };

/**
 * Parse the comma-separated scope list an assertion was handed into the array
 * `token.scopes` is. Empty, absent or all-whitespace means "this block declared
 * none", which is `[]` — the same value the oracle renders as `scopes=none`.
 *
 * 🔴 ONE PARSER, AND IT IS NOT THIS ONE. The authoritative read of a block's
 * declared scopes is `oracle.sh`'s single `jq` over the trial's own
 * `block.manifest.json`; this only turns the string that read produced back into
 * a list. Adding a second manifest parser here would give the cell's `scopes=`
 * field and the block's `token.scopes` two independent sources that can disagree
 * without anything noticing.
 */
export function parseScopes(csv) {
  return String(csv ?? '').split(',').map((s) => s.trim()).filter(Boolean);
}

/**
 * The inline bootstrap a host injects on its own document. Field names and
 * shape follow the SDK's `BLOCK_INIT` payload, because `InlineTransport` feeds
 * this object to the same `snapshotFromInit()` the iframe path uses — a missing
 * field surfaces as `undefined` inside the block, not as an error here.
 *
 * 🔴 THE SCOPE LIST IS THE BLOCK'S OWN MANIFEST'S, NOT AN EMPTY ONE, AND THAT
 * CHANGE HAS A MEASUREMENT BEHIND IT. This used to seed `scopes: []`
 * unconditionally, with a comment calling the whole credential half "deliberately
 * inert". The cost was measured on `ab-genpost-dsv4-01` (2026-09-21): a correct
 * generate-then-post app whose Generate handler reads
 * `hasBudgetedScope(token.scopes)` and, when it is false, asks the host for
 * consent *instead of* generating — the consent-first shape the SDK's own
 * `useRequestConsent` exists for — never reached `setStatus('generating')`, so
 * the assertion timed out and the cell read `RENDER=no observed=ready`. That
 * verdict was about the harness's scope list. One knob, four arms, same
 * containers:
 *
 *     deepseek, scopes []                          RENDER=no   observed=ready
 *     deepseek, scopes from its manifest           RENDER=yes  observed=ready>generating>ready
 *     unmodified page-money scaffold, seeded       RENDER=no   (the arm that decides
 *                                                   shippability: seeding is not
 *                                                   permissiveness)
 *     mimo (positive control), seeded              RENDER=yes  observed=ready>generating>ready
 *
 * An empty list does not grade a neutral branch — it grades the *refused* one,
 * rewarding an app that flips a status optimistically and penalising one that
 * checks consent first. Seeding what the manifest declares grades the branch the
 * platform would put the block on, because the platform's own grant is derived
 * from that same declaration at review time.
 *
 * 🔴 `raw` STAYS EMPTY, AND THAT IS THE LOAD-BEARING HALF. Scopes buy the block
 * a BRANCH, never a CAPABILITY: `InlineTransport.sendRequest` rejects
 * unconditionally whatever the scope list says, so briefs/genpost.md's premise —
 * no generation and no post can complete here, on any machine — is untouched. A
 * bootstrap carrying a real `raw` would make a green cell earnable with a
 * credential the oracle handed over, which is a different and much worse
 * instrument. `fxGenpostBootstrapProbe` in dogfood_oracle_test.go asserts BOTH
 * halves from inside the page: the scopes match the manifest, and `raw` is `''`.
 *
 * `context.viewerUserId` / `viewerUsername` track `HOST_VIEWER` rather than
 * being hardcoded: on the page slot the host sends the viewer's identity
 * through BOTH channels (`PageBlockHost.buildContext()` is not run through the
 * model-slot allowlist that drops them), so a block is free to gate on either,
 * and a bootstrap where the two disagree is a state no host produces.
 */
export function hostBootstrap(scopes = []) {
  return {
    blockInstanceId: 'dogfood-oracle-instance',
    blockId: 'dogfood-oracle-block',
    appId: 'dogfood-oracle-app',
    token: { raw: '', scopes: [...scopes], expiresAt: new Date(0).toISOString() },
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
}

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

/**
 * The browser binaries this harness will accept, in preference order.
 *
 * 🔴 RELEASE BUILDS FIRST, CONTINUOUS-BUILD SNAPSHOTS LAST, AND THAT ORDER IS
 * NOT COSMETIC ON THE RUNNER THIS REPO'S CI USES. `ubuntu-latest` ships BOTH:
 * `/usr/bin/google-chrome` is a `google-chrome-stable` .deb — a release —
 * while `/usr/bin/chromium` is a symlink into `/usr/local/share/chromium/`,
 * unzipped by `install-google-chrome.sh` straight out of the
 * `chromium-browser-snapshots` bucket at whatever per-commit revision sits
 * nearest Chrome's. The version strings say it: Chrome `152.0.7977.82` against
 * Chromium `152.0.7977.0`. The old order named `chromium` first, so every CI
 * run drove an un-release-qualified trunk snapshot while a shipped release sat
 * on the same disk.
 *
 * ⚠ SAY WHAT THIS IS AND IS NOT. It is a determinism argument — prefer the
 * build someone qualified — not a measurement: no run here has shown the
 * snapshot launching less reliably than the release. It is listed as part of
 * the launch-flake fix because it is free and it narrows the population, not
 * because it is the mechanism.
 *
 * 🔴 THIS LIST EXISTS FOUR TIMES AND THEY MUST AGREE: here, in
 * `scripts/dogfood/oracle.sh`, in `.github/workflows/ci.yml`'s resolve step,
 * and in `dogfood_oracle_test.go`'s `oracleBrowser`. One rule, four places, so
 * it is pinned by `TestEveryBrowserResolverAgreesOnTheSameOrder` rather than by
 * a comment asking nicely.
 */
export const BROWSER_NAMES = [
  'google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser', 'chrome',
];

export async function findChrome() {
  if (process.env.CIVITAI_CHROME) return process.env.CIVITAI_CHROME;
  for (const d of (process.env.PATH || '').split(':')) {
    for (const n of BROWSER_NAMES) {
      const p = join(d, n);
      try { await access(p); return p; } catch { /* next */ }
    }
  }
  throw new Error('no Chromium on PATH; set CIVITAI_CHROME to a browser binary');
}

/**
 * The launch flags, grouped by what each group buys.
 *
 * 🔴 THE `--password-store` LINE IS THE MEASURED ONE. The CI failures all
 * carried the same stderr — `dbus/bus.cc:405 Failed to connect to the bus:
 * Could not parse server address: Unknown address type` — and that string is
 * REPRODUCIBLE: set `DBUS_SESSION_BUS_ADDRESS` to anything with a transport
 * type libdbus does not know (`bogus:path=/nope`) and chromium emits it
 * verbatim. So the runner's session-bus address is unparseable, and every
 * session-bus lookup chromium makes during startup fails.
 *
 * Measured on chromium 153 with that bogus address, `--user-data-dir` fresh:
 *
 *     no extra flags          4 `dbus/bus.cc` errors, then DevTools listening
 *     --password-store=basic  3 `dbus/bus.cc` errors, then DevTools listening
 *
 * i.e. the flag removes exactly one session-bus round trip — the password-store
 * / keyring probe, which is the LAST one before the DevTools line and the only
 * one chromium makes synchronously on the startup path. Three of the four CI
 * failures stalled after exactly THREE dbus errors, i.e. precisely where that
 * probe is; the fourth stalled after one, so this is not the whole story and is
 * not claimed to be. ⚠ And the null result, because it looked promising and is
 * worth not re-deriving: CLEARING `DBUS_SESSION_BUS_ADDRESS` in the child does
 * nothing — 4 errors either way — so the fix is not an env fix.
 *
 * The remaining groups remove startup work whose cost is disk and network on a
 * machine measured at 100–300× slower than a dev box (see `LAUNCH_MS`): the
 * variations seed, the component updater, safe-browsing list fetches, sync, the
 * default-apps scan, the search-engine choice screen. None of them touch page
 * loading — the block under test fetches over the loopback server as before.
 */
export const LAUNCH_FLAGS = [
  '--headless=new', '--remote-debugging-port=0',
  '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage',
  '--no-first-run', '--no-default-browser-check',
  // Do not go looking for a keyring over a session bus that does not work.
  '--password-store=basic', '--use-mock-keychain',
  // Do not go to the network before binding the debugging port.
  '--disable-background-networking', '--disable-component-update',
  '--disable-client-side-phishing-detection', '--disable-sync',
  '--disable-default-apps', '--no-service-autorun', '--metrics-recording-only',
  '--disable-search-engine-choice-screen',
];

/** One launch attempt. Resolves with a live browser, or throws having KILLED
 * the process it started and attached the browser's own output as
 * `.diagnostics`. */
async function launchOnce(bin) {
  // A fresh profile PER ATTEMPT. A stalled attempt leaves a half-written
  // profile behind and handing that to the retry would make attempt 2 a
  // strictly worse experiment than attempt 1.
  const profile = await mkdtemp(join(tmpdir(), 'civitai-assert-'));
  const started = Date.now();
  const proc = spawn(bin, [...LAUNCH_FLAGS, `--user-data-dir=${profile}`, 'about:blank'],
    { stdio: ['ignore', 'pipe', 'pipe'] });

  // 🔴 BOTH STREAMS. The endpoint has always arrived on stderr, but a browser
  // that fails in a new way may say so on stdout, and a diagnostic that drops
  // half of what the process said is how "nothing was printed" gets reported
  // about a process that printed the answer.
  let buf = '';
  proc.stdout.on('data', (d) => { buf += d; });
  proc.stderr.on('data', (d) => { buf += d; });

  try {
    const ws = await new Promise((resolve, reject) => {
      let settled = false;
      const finish = (fn, v) => { if (!settled) { settled = true; clearTimeout(to); fn(v); } };
      const to = setTimeout(() => finish(reject,
        new Error(`no DevTools endpoint within ${LAUNCH_MS}ms`)), LAUNCH_MS);
      const scan = () => { const m = buf.match(/ws:\/\/\S+/); if (m) finish(resolve, m[0]); };
      proc.stdout.on('data', scan);
      proc.stderr.on('data', scan);
      proc.on('exit', (c, sig) => finish(reject,
        new Error(`browser exited ${c}${sig ? ` on ${sig}` : ''}`)));
      proc.on('error', (e) => finish(reject,
        new Error(`browser could not be started: ${e.message}`)));
      scan(); // anything that arrived before these listeners were attached
    });
    return { proc, ws, ms: Date.now() - started };
  } catch (e) {
    // 🔴 KILL IT. The old code left the process running on every timeout —
    // visible in the CI logs of all four failures as the runner's own
    // `Terminate orphan process: pid (…) (chrome)` cleanup. A leak was merely
    // untidy while nothing retried; with a retry it is a second browser
    // competing for the same machine, which would make attempt 2 fail for a
    // reason attempt 1 created.
    try { proc.kill('SIGKILL'); } catch { /* already gone */ }
    e.diagnostics = buf;
    e.ms = Date.now() - started;
    throw e;
  }
}

const indent = (s) => String(s || '(the browser printed nothing at all)')
  .replace(/\n+$/, '').split('\n').map((l) => `      ${l}`).join('\n');

export async function launch() {
  const bin = await findChrome();
  const failures = [];

  for (let attempt = 1; attempt <= LAUNCH_ATTEMPTS; attempt++) {
    try {
      const { proc, ws, ms } = await launchOnce(bin);
      if (attempt > 1) {
        // 🔴 LOUD ON SUCCESS. This is the line that stops the retry from being
        // a way to stop noticing a browser that has started failing: a green
        // run whose browser needed two goes SAYS SO. ci.yml greps for
        // `BROWSER LAUNCH RETRY`.
        console.error(
          `[cdp] ⚠ BROWSER LAUNCH RETRY: ${bin} launched on attempt ${attempt} of ` +
          `${LAUNCH_ATTEMPTS} (${ms}ms). This run is green, but the browser FAILED ` +
          `${attempt - 1} time(s) first:\n${failures.join('\n')}`);
      } else {
        console.error(`[cdp] browser launched in ${ms}ms (${bin}, attempt 1 of ${LAUNCH_ATTEMPTS})`);
      }
      return { proc, ws };
    } catch (e) {
      failures.push(
        `    attempt ${attempt}/${LAUNCH_ATTEMPTS} failed after ${e.ms ?? 0}ms: ${e.message}\n` +
        `${indent(e.diagnostics)}`);
      console.error(
        `[cdp] browser launch attempt ${attempt} of ${LAUNCH_ATTEMPTS} FAILED after ` +
        `${e.ms ?? 0}ms: ${e.message}`);
    }
  }

  // 🔴 STILL FAILS LOUDLY, AND KEEPS THE PHRASE. The assertion turns this into
  // its `harness error:` reason and oracle.sh turns THAT into exit 2 / nothing
  // was measured — the property that stops a browser this harness cannot start
  // being reported as an app that does not work. Every attempt's diagnostics
  // ride along, so a genuinely unavailable browser is more legible than before,
  // not less.
  throw new Error(
    `browser never printed a DevTools endpoint (${bin}, ${LAUNCH_ATTEMPTS} attempt(s), ` +
    `all failed, ${LAUNCH_MS}ms each):\n${failures.join('\n')}`);
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
 * `scopes` is the block's own declared scope list, handed down from `oracle.sh`
 * (which reads it out of the trial's `block.manifest.json`). It defaults to `[]`
 * — a hand-run `node briefs/<brief>.assert.mjs <dir>` with no scope argument
 * grades the no-scope branch, which is what it graded before this parameter
 * existed.
 *
 * 🔴 `Page.addScriptToEvaluateOnNewDocument` BEFORE `Page.navigate`, never a
 * `Runtime.evaluate` after the load. The block's bundle calls `getTransport()`
 * on its first module evaluation and the transport is a process-wide singleton
 * whose FIRST construction wins, so a global set even one tick late is read by
 * nothing. This CDP method runs its source before any script in the document.
 */
export async function openPage(c, url, { scopes = [] } = {}) {
  const { targetId } = await c.send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await c.send('Target.attachToTarget', { targetId, flatten: true });
  await c.send('Page.enable', {}, sessionId);
  await c.send('Runtime.enable', {}, sessionId);
  if (SEND_HOST_INIT) {
    await c.send('Page.addScriptToEvaluateOnNewDocument', {
      source: `window.__CIVITAI_BLOCK_CONTEXT__ = ${JSON.stringify(hostBootstrap(scopes))};`,
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
