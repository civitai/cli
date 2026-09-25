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
// ⚠ ONE EXCEPTION, ADDED 2026-09-25, AND IT COMPLETES NOTHING: a host RESOURCE
// PICK is answered. `patchInlineTransport` rewrites that one stub expression so it
// consults `inlineHostSource`'s shim, which resolves `OPEN_RESOURCE_PICKER` and
// `OPEN_CHECKPOINT_PICKER` with a stubbed `BlockResourceInfo` and rejects every
// other request type with the SDK's own error, byte for byte. So "a block that
// awaits a real generation or a real post never gets one" is unchanged; what
// changed is that an app gating Generate on a PICK is no longer graded on a branch
// production never puts it in. See `HOST_RESOURCE_PICKS` for the measurement.
//
// 🔴 AND IT ANSWERS A HOST RESOURCE-PICK, BECAUSE A PICKER-GATED APP OTHERWISE
// GRADES A BRANCH PRODUCTION NEVER EXHIBITS EITHER. Third instance of one class
// (after the viewer and the scope list): an app that correctly gates Generate on
// something the PLATFORM supplies was graded as broken because the harness
// supplied nothing. Measured on `ab-ship-mimo-02` (2026-09-25) — see
// `HOST_RESOURCE_PICKS` and `inlineHostSource` for the measurement, the shape,
// and the exact boundary of what the answer buys the block (a BRANCH; never a
// capability — every non-picker request still rejects with the SDK's own error).
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

// ── the host resource picker ─────────────────────────────────────────────────

/**
 * The control arm for the picker, exactly as `CIVITAI_ASSERT_NO_HOST` is the
 * control arm for the handshake and `CIVITAI_ASSERT_ANON_VIEWER` for the viewer:
 * set `CIVITAI_ASSERT_NO_HOST_PICKS=1` and the oracle answers no pick at all,
 * leaving `InlineTransport.sendRequest` exactly as the SDK ships it. It is what
 * proves the answered pick is doing something, and it is the only way to grade a
 * block's dismissed-picker branch deliberately.
 */
export const SEND_HOST_PICKS =
  process.env.CIVITAI_ASSERT_NO_HOST_PICKS !== '1' && process.env.CIVITAI_ASSERT_NO_HOST !== '1';

/** What the cell reports about the picker the block was shown. Same reason as
 * `HOST_VIEWER_LABEL`: a bare `no` cannot tell "the model built nothing" from
 * "the model asked the host to open a picker and the harness never answered". */
export const HOST_PICKS_LABEL = SEND_HOST_PICKS ? 'answered' : 'unanswered';

/**
 * The SDK's OWN error, byte for byte, from
 * `@civitai/blocks-react/dist/transport/inlineTransport.js` (0.57.2):
 *
 *     sendRequest(_request, _responseType, _opts) {
 *         return Promise.reject(new Error('InlineTransport.sendRequest is not implemented in v1'));
 *     }
 *
 * It is quoted here for two jobs. It is the NEEDLE `patchInlineTransport` finds —
 * a string unique to that one stub, which is why the patch cannot land anywhere
 * else in a bundle — and it is the rejection `inlineHostSource` re-throws for
 * every request that is not a picker, so anything already matching on the wording
 * keeps working and the refusal a block sees is the SDK's, not the oracle's.
 */
export const INLINE_STUB_MESSAGE = 'InlineTransport.sendRequest is not implemented in v1';

/** The page global the patched stub delegates to. Named, exported and asserted
 * rather than spelled twice: the source rewrite and the shim have to agree or the
 * patch silently falls back to the original rejection. */
export const INLINE_HOST_GLOBAL = '__CIVITAI_DOGFOOD_INLINE_HOST__';

/**
 * The request types this oracle will ANSWER. Everything else — every workflow
 * estimate, submit and poll, every post, every purchase, every token refresh —
 * keeps rejecting with {@link INLINE_STUB_MESSAGE}.
 *
 * 🔴 THIS LIST IS THE INVARIANT, AND IT IS AN ALLOWLIST FOR THAT REASON. The two
 * entries are host *discovery* calls: they hand the block an id it could have
 * hardcoded, and the SDK's own docblock says so — "DISCOVERY ONLY: the returned
 * `versionId` is a hint, never an entitlement … the spend path is the enforcement
 * boundary, not the picker". Nothing here can complete a generation or a post,
 * because nothing here answers a request that does one.
 */
export const HOST_PICKER_REQUESTS = ['OPEN_RESOURCE_PICKER', 'OPEN_CHECKPOINT_PICKER'];

/**
 * The resources a pick resolves with, keyed by `BlockResourcePickerType`.
 *
 * 🔴 NOT INVENTED — these are the SDK's own canned picks, `DEFAULT_CHECKPOINT_PICK`
 * and `DEFAULT_LORA_PICK` in `@civitai/blocks-react/dist/internal/mockHost.js`
 * (0.57.2), field for field and value for value. Two reasons. A block is entitled
 * to treat this as a `BlockResourceInfo`, and that interface is the host's
 * projection in civitai/civitai's `PageBlockHost.tsx` — inventing a shape here
 * would let a block read a field production never sends and still grade green,
 * which is the both-wrong-blind failure arriving through the instrument. And
 * every OTHER host emulation in this ecosystem already resolves with exactly
 * these objects (`createMockHost`, and `createLiveHost`'s in-harness overlay
 * resolves a real one of the same shape), so an app that works in `dev:harness`
 * behaves identically here.
 *
 * ⚠ THE IDS ARE REAL AND THAT BUYS NOTHING. They are the SDK's, so they name a
 * real model version — and a real id is worth exactly as much as a made-up one
 * here, because the only thing a block can do with it is put it in a request that
 * `sendRequest` refuses. The platform re-validates every id server-side at
 * estimate/submit regardless of what any picker showed.
 */
export const HOST_RESOURCE_PICKS = {
  Checkpoint: {
    versionId: 691639,
    modelId: 618692,
    modelName: 'FLUX.1 [dev]',
    versionName: 'fp8',
    baseModel: 'Flux.1 D',
    modelType: 'Checkpoint',
    strength: 1,
    minStrength: -1,
    maxStrength: 2,
    trainedWords: [],
    clipSkip: null,
  },
  LORA: {
    versionId: 666002,
    modelId: 555002,
    modelName: 'Sinfully Stylish',
    versionName: 'v2.0',
    baseModel: 'SDXL 1.0',
    modelType: 'LORA',
    strength: 1,
    minStrength: -1,
    maxStrength: 2,
    trainedWords: ['sinfully stylish'],
    clipSkip: null,
  },
};

/**
 * Rewrite the SDK's v1 inline stub so it consults this oracle's host shim.
 *
 * 🔴 WHY A SOURCE REWRITE AND NOT A SEEDED FIELD, WHICH IS WHAT THE PREVIOUS TWO
 * FIXES IN THIS CLASS WERE. A viewer and a scope list are BOOTSTRAP DATA: the
 * oracle puts them on `window.__CIVITAI_BLOCK_CONTEXT__` and the SDK reads them
 * off the window. A pick is not data, it is a REQUEST/RESPONSE — `useResourcePicker`
 * calls `sendTypedRequest(getTransport(), { type: 'OPEN_RESOURCE_PICKER', … })` —
 * and in inline mode that lands on `InlineTransport.sendRequest`, a method on a
 * class that is bundled INTO the block. There is no window surface for it: the
 * SDK's whole inline host-cooperation contract is the one global, and grepping
 * 0.57.2 for `__CIVITAI` finds `__CIVITAI_BLOCK_CONTEXT__` and nothing else. So
 * the choices were (a) patch that one stub, (b) abandon `InlineTransport` for the
 * iframe protocol — which needs the block's build-time origin allowlist to accept
 * the oracle, is what the module docblock above explains it cannot, and would
 * hand the harness the power to answer a SUBMIT — or (c) leave a correct app
 * graded as broken. (a) is the only one that keeps "nothing can complete here"
 * structurally true.
 *
 * 🔴 IT IS DELIBERATELY THE NARROWEST EDIT THAT WORKS, AND IT IS REVERSIBLE AT
 * RUNTIME. It replaces one expression — the stub's `Promise.reject(new Error(<the
 * SDK's message>))` — with a conditional that calls the shim IF the page has one
 * and otherwise evaluates the ORIGINAL expression, unchanged. So a bundle patched
 * with no shim installed behaves exactly as the SDK ships it, and the control arm
 * (`CIVITAI_ASSERT_NO_HOST_PICKS=1`) does not even patch.
 *
 * ⚠ `arguments` rather than a captured parameter name, because the parameter name
 * is whatever the block's minifier chose (measured on a real trial bundle:
 * `sendRequest(r,d,s)`), while `arguments[0]` is name-independent. It is guarded
 * by `typeof arguments === "undefined"` — an unresolvable identifier is safe to
 * `typeof` — so if some future bundler turns the method into an arrow function,
 * where `arguments` does not exist, the shim is handed `undefined` and refuses,
 * rather than throwing a ReferenceError synchronously where a rejection was due.
 *
 * Returns the code plus the number of sites patched. `hits === 0` means the
 * needle was not there: either the block does not bundle the SDK's inline
 * transport at all, or the SDK reworded its stub. Both are reported on the cell
 * (`pickerShim`) rather than assumed, because a silent 0 here and a block that
 * never opens a picker produce the same green.
 */
export function patchInlineTransport(code) {
  const src = String(code);
  // Built fresh per call: a module-level /g regex carries `lastIndex` between
  // callers, which is how a second file silently starts matching from an offset.
  // 🔴 `new` IS OPTIONAL AND THE QUOTE CAN BE A BACKTICK, BOTH MEASURED RATHER
  // THAN IMAGINED. The first version of this needle required `new Error('…')`,
  // which is what the SDK's SOURCE says and what `ab-ship-mimo-02`'s bundle
  // happened to preserve. Then a SECOND real bundle was read —
  // `ab-genpost-mimo-01`, blocks-react 0.53.1 — and its minifier had emitted
  // `Promise.reject(Error(\`InlineTransport.sendRequest is not implemented in v1\`))`:
  // no `new`, and a template literal for the message. `Error(x)` and `new Error(x)`
  // are equivalent in JS, so dropping the keyword is an ordinary minifier
  // transform, and a needle pinned to one spelling patches NOTHING on the other
  // while reporting a perfectly ordinary green. One bundle was not a general
  // claim; two are not either, which is why `pickerShim` puts the site count on
  // every cell.
  const needle = new RegExp(
    String.raw`Promise\.reject\(\s*(?:new\s+)?Error\(\s*(['"\x60])`
    + INLINE_STUB_MESSAGE.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    + String.raw`\1\s*\)\s*\)`,
    'g');
  let hits = 0;
  const out = src.replace(needle, (original) => {
    hits += 1;
    const g = `globalThis.${INLINE_HOST_GLOBAL}`;
    return `(${g}?${g}(typeof arguments==="undefined"?undefined:arguments[0]):${original})`;
  });
  return { code: hits ? out : src, hits };
}

/**
 * The page-side host shim the patched stub calls. Runs before the block's first
 * script, like the bootstrap.
 *
 * 🔴 IT IS AN ALLOWLIST AND THE REFUSAL IS THE SDK'S OWN ERROR. A picker request
 * resolves with the wire shape the iframe transport resolves with — the response
 * message's `payload`, i.e. `{ requestId, selected }` (`IframeTransport`'s
 * `pending.resolve(payload)`) — and every other request type rejects with
 * {@link INLINE_STUB_MESSAGE}, the string the SDK's own stub throws. So a block
 * cannot tell this harness from an unpatched one on any path that spends or
 * publishes, and `briefs/genpost.md`'s premise — no generation and no post can
 * complete here, on any machine — is untouched.
 *
 * 🔴 AND IT COUNTS BOTH SIDES, ONTO THE CELL. `window.__dogfoodHostPicks` records
 * what was answered and `window.__dogfoodHostRefused` what was refused, and the
 * assertion carries both out as evidence. That is the difference between claiming
 * the money path still refuses and SHOWING it refused, per cell, in the same line
 * as the verdict.
 *
 * An unsupported `resourceType` resolves with NO `selected`, which the SDK
 * normalises to `null` — "the user dismissed without picking". That mirrors
 * `createMockHost` (`cannedPicks[rtype]` is `undefined` for anything else) and the
 * platform, whose native modal refuses to open for a type outside
 * `BlockResourcePickerType`.
 */
export function inlineHostSource() {
  return `(() => {
  var PICKS = ${JSON.stringify(HOST_RESOURCE_PICKS)};
  var STUB = ${JSON.stringify(INLINE_STUB_MESSAGE)};
  var answered = [], refused = [];
  window.__dogfoodHostPicks = answered;
  window.__dogfoodHostRefused = refused;
  window.${INLINE_HOST_GLOBAL} = function (request) {
    var type = request && request.type;
    var payload = (request && request.payload) || {};
    // The inline path never assigns one (IframeTransport adds it on dispatch),
    // so this is the wire field, filled in rather than left undefined.
    var requestId = payload.requestId || 'dogfood-oracle-inline';
    if (type === 'OPEN_RESOURCE_PICKER') {
      var picked = PICKS[payload.resourceType];
      answered.push(type + ':' + payload.resourceType);
      return Promise.resolve(picked ? { requestId: requestId, selected: picked }
                                    : { requestId: requestId });
    }
    if (type === 'OPEN_CHECKPOINT_PICKER') {
      // The five-field BlockCheckpointInfo projection, exactly as the host's
      // CHECKPOINT_PICKER_RESULT sends it — NOT the wider BlockResourceInfo.
      var c = PICKS.Checkpoint;
      answered.push(type);
      return Promise.resolve({ requestId: requestId, selected: {
        versionId: c.versionId, modelId: c.modelId, modelName: c.modelName,
        versionName: c.versionName, baseModel: c.baseModel } });
    }
    refused.push(type || '(untyped)');
    return Promise.reject(new Error(STUB));
  };
})();`;
}

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
  // Live subscribers, for the events that must be ANSWERED rather than counted
  // afterwards. `Fetch.requestPaused` is the case: the browser has stopped a
  // response and is waiting for this process to say what to do with it, so
  // reading it out of `events` after the fact would be reading it after the page
  // had already given up on it.
  const handlers = new Map();
  sock.addEventListener('message', (e) => {
    const m = JSON.parse(e.data);
    if (m.id !== undefined && pending.has(m.id)) {
      const { resolve, reject } = pending.get(m.id);
      pending.delete(m.id);
      m.error ? reject(new Error(`${m.error.message} (${JSON.stringify(m.error.data ?? null)})`)) : resolve(m.result);
    } else if (m.method) {
      events.push(m);
      for (const h of handlers.get(m.method) ?? []) {
        // 🔴 NEVER LET A HANDLER'S THROW REACH THE SOCKET LISTENER. An
        // unhandled rejection here would take the whole assertion down with a
        // message about this plumbing, which is the "harness error reported as a
        // verdict" shape this file exists to avoid.
        try {
          Promise.resolve(h(m.params ?? {}, m.sessionId)).catch((err) => {
            console.error(`[cdp] handler for ${m.method} failed: ${err.message}`);
          });
        } catch (err) {
          console.error(`[cdp] handler for ${m.method} threw: ${err.message}`);
        }
      }
    }
  });
  const open = new Promise((res, rej) => {
    sock.addEventListener('open', res);
    sock.addEventListener('error', () => rej(new Error('devtools socket error')));
  });
  return {
    open,
    close: () => sock.close(),
    /** Subscribe to a CDP event. Returns an unsubscribe. */
    on(method, handler) {
      const set = handlers.get(method) ?? new Set();
      set.add(handler);
      handlers.set(method, set);
      return () => set.delete(handler);
    },
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
 * Answer one `Fetch.requestPaused` at the Response stage: read the body, and if
 * it carries the SDK's inline stub, hand the browser a patched copy instead.
 *
 * 🔴 EVERY PATH ENDS WITH THE RESPONSE CONTINUING. A paused response that is
 * never answered is a page that never finishes loading, which the assertion would
 * report as the block not rendering — a harness failure wearing a verdict's
 * clothes. So the read, the decode and the patch are all inside one try, any
 * failure is COUNTED and NAMED on stderr, and the original response goes through.
 */
async function patchPausedResponse(c, sessionId, params, shim) {
  const { requestId } = params;
  const cont = async () => {
    try { await c.send('Fetch.continueRequest', { requestId }, sessionId); } catch { /* target gone */ }
  };
  try {
    // A failed or redirected response has no body to read.
    if (params.responseErrorReason || params.responseStatusCode === undefined) return cont();
    const { body, base64Encoded } = await c.send('Fetch.getResponseBody', { requestId }, sessionId);
    const text = base64Encoded ? Buffer.from(body, 'base64').toString('utf8') : String(body);
    const { code, hits } = patchInlineTransport(text);
    if (!hits) {
      // 🔴 THE INSTRUMENT'S OWN FAILURE SIGNAL, AND IT IS WHY `sites=0` IS NOT
      // ENOUGH ON ITS OWN. A response that carries the SDK's stub MESSAGE but
      // matched no needle is an inline transport this oracle could not instrument
      // — the message is a string literal a minifier must preserve, while the
      // expression around it is not (measured: 0.53.1 emits `Error(\`…\`)`, 0.57.2
      // `new Error("…")`). Without this counter that state is indistinguishable
      // from "this block does not bundle the SDK at all", and the two have
      // opposite consequences for a `no`. See `pickerBlind` in genpost.assert.mjs.
      if (text.includes(INLINE_STUB_MESSAGE)) shim.unmatched += 1;
      return cont();
    }
    shim.documents += 1;
    shim.sites += hits;
    await c.send('Fetch.fulfillRequest', {
      requestId,
      responseCode: params.responseStatusCode,
      // 🔴 DROP `content-length` AND `content-encoding`. `getResponseBody` hands
      // back the DECODED body and the patch changes its length, so replaying
      // either header describes a body that is not the one being delivered —
      // which a browser enforces by truncating or by failing the load outright.
      responseHeaders: (params.responseHeaders ?? []).filter(
        (h) => !/^(content-length|content-encoding)$/i.test(h.name)),
      body: Buffer.from(code, 'utf8').toString('base64'),
    }, sessionId);
  } catch (e) {
    shim.skipped += 1;
    console.error(`[cdp] inline-host patch skipped for ${params.request?.url}: ${e.message}`);
    return cont();
  }
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
 *
 * 🔴 AND THE PICKER PATCH GOES IN OVER `Fetch`, IN THE BROWSER, RATHER THAN IN
 * EITHER SERVER. There are two servers — `serve-block.mjs` runs INSIDE the trial
 * container for a URL target, `serve()` above runs in this process for a
 * directory target — so a rewrite done server-side would be one rule open-coded
 * in two places, only one of which a hand-run `node briefs/<brief>.assert.mjs
 * <dir>` exercises. Intercepting the response the browser is about to execute is
 * ONE place that covers both, and it is the same side of the boundary the
 * bootstrap injection is already on.
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
  // What the picker patch actually did, so the cell can say so instead of a
  // reader assuming it. `sites` is how many stubs were rewritten across the whole
  // document; `unmatched` counts responses that carry the SDK's stub message in a
  // spelling no needle matched — the instrument-blind state; `skipped` counts
  // responses the interception could not read at all.
  const shim = { documents: 0, sites: 0, unmatched: 0, skipped: 0 };
  if (SEND_HOST_PICKS) {
    await c.send('Page.addScriptToEvaluateOnNewDocument', { source: inlineHostSource() }, sessionId);
    c.on('Fetch.requestPaused', (params, evSessionId) => {
      if (evSessionId && evSessionId !== sessionId) return;
      return patchPausedResponse(c, sessionId, params, shim);
    });
    // Documents AND scripts: a block's bundle is normally an external module, but
    // a small one can be inlined into index.html, and a fixture written by hand
    // usually is. Nothing else is intercepted — an image or a font cannot carry
    // the needle and paying to base64 it would be pure latency.
    await c.send('Fetch.enable', {
      patterns: [
        { urlPattern: '*', resourceType: 'Document', requestStage: 'Response' },
        { urlPattern: '*', resourceType: 'Script', requestStage: 'Response' },
      ],
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
  /**
   * What the host shim was asked for and what it did about it, read out of the
   * page. `answered` is the picker requests it satisfied; `refused` is every other
   * request type it turned down with the SDK's own error — which is the half that
   * SHOWS, per cell, that the money path still cannot complete.
   */
  const hostRequests = async () => {
    try {
      return JSON.parse(await evalJs(`JSON.stringify({
        answered: window.__dogfoodHostPicks || [],
        refused: window.__dogfoodHostRefused || [],
      })`));
    } catch { return { answered: [], refused: [] }; }
  };
  return { sessionId, evalJs, waitFor, typeInto, bodyHtml, hostRequests, shim };
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
