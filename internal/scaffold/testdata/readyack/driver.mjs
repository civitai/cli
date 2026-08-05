// Runtime driver for the scaffold ready-ack guard (Guard B).
//
// Loads a ready-ack emitter under a minimal `window` stub, plays the host's
// side of the handshake at it, and prints what the emitter ACTUALLY did as
// JSON on stdout. It asserts nothing — every verdict is made in Go
// (internal/scaffold/ready_ack_runtime_test.go), so this file cannot decide it
// is happy with itself.
//
// Usage: node driver.mjs <path-to-emitter.js> [--throw-first-post]
//
// `--throw-first-post` makes the FIRST outbound postMessage throw, the way a
// browser does when `targetOrigin` cannot be parsed as a URL (MEASURED in
// Chromium 1228: `postMessage(msg, 'null')` throws
// `SyntaxError: Invalid target origin 'null'`, and `event.origin` is the string
// "null" whenever the sender is itself at an opaque origin). An emitter that
// latches its "already acked" flag BEFORE posting goes permanently silent in
// that case; one that latches after can still answer the host's next retry.
import { readFileSync } from 'node:fs';

const emitterPath = process.argv[2];
if (!emitterPath) {
  console.error('usage: driver.mjs <path-to-emitter.js> [--throw-first-post]');
  process.exit(2);
}
const throwFirstPost = process.argv.includes('--throw-first-post');

export const HOST_ORIGIN = 'https://host.civitai.example';
const FOREIGN_ORIGIN = 'https://not-the-host.example';

/** Every outbound postMessage, in order. */
const posted = [];
/** Posts that threw, in order — an attempt the emitter made and lost. */
const threw = [];
/** Listener registrations, by event type, as the emitter made them. */
const listeners = Object.create(null);

const parentWindow = {
  postMessage(message, targetOrigin) {
    if (throwFirstPost && threw.length === 0) {
      threw.push({ message, targetOrigin });
      // Same constructor and shape a browser raises for an unparseable target.
      throw new SyntaxError(
        `Failed to execute 'postMessage' on 'Window': Invalid target origin '${targetOrigin}' in a call to 'postMessage'.`
      );
    }
    posted.push({ window: 'parent', message, targetOrigin });
  },
};

// A window that is NOT the parent. If the emitter ever posts here, the source
// check is not doing its job.
const foreignWindow = {
  postMessage(message, targetOrigin) {
    posted.push({ window: 'foreign', message, targetOrigin });
  },
};

const win = {
  parent: parentWindow,
  addEventListener(type, fn) {
    (listeners[type] ??= []).push(fn);
  },
  removeEventListener(type, fn) {
    const set = listeners[type];
    if (!set) return;
    const i = set.indexOf(fn);
    if (i >= 0) set.splice(i, 1);
  },
  postMessage(message, targetOrigin) {
    posted.push({ window: 'self', message, targetOrigin });
  },
};

globalThis.window = win;
// Enough DOM for an emitter that (wrongly) waits on the document to at least
// load without throwing — a crash and an inert emitter must not look alike.
globalThis.document = {
  addEventListener(type, fn) {
    (listeners[`document:${type}`] ??= []).push(fn);
  },
  removeEventListener() {},
  getElementById: () => null,
  documentElement: { scrollHeight: 1234 },
  readyState: 'complete',
};

let loadError = null;
try {
  // Run the emitter the way a <script> tag does: bare global scope, with
  // `window` resolving to the stub above.
  new Function(readFileSync(emitterPath, 'utf8'))();
} catch (err) {
  loadError = String(err && err.stack ? err.stack : err);
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 25));

/** Exceptions thrown out of a listener, in order. */
const listenerErrors = [];

function deliver(event) {
  for (const fn of [...(listeners.message ?? [])]) {
    // A browser isolates each listener invocation: an exception becomes an
    // uncaught error and does NOT abort the other listeners or the page. Model
    // that, or a throwing emitter would kill this driver instead of being
    // observed.
    try {
      fn(event);
    } catch (err) {
      listenerErrors.push(String(err && err.message ? err.message : err));
    }
  }
}

const steps = [];
async function step(label, event) {
  if (event) deliver(event);
  // The SDK's own transport queues the ack onto a microtask, so give an
  // async emitter a chance to post before the count is read.
  await flush();
  steps.push({ label, posts: posted.length });
}

const init = (payload = {}) => ({ type: 'BLOCK_INIT', payload });

await step('after-load', null);

// NEGATIVE — an init-shaped message from a window that is not the parent.
await step('after-foreign-source', {
  source: foreignWindow,
  origin: FOREIGN_ORIGIN,
  data: init({ token: 'attacker' }),
});

// NEGATIVE — a real host message that is not BLOCK_INIT.
await step('after-unrelated-type', {
  source: parentWindow,
  origin: HOST_ORIGIN,
  data: { type: 'TOKEN_REFRESH', payload: { token: 'tok_rotated' } },
});

// THE HANDSHAKE.
await step('after-first-init', {
  source: parentWindow,
  origin: HOST_ORIGIN,
  data: init({ token: 'tok_abc', blockId: 'ready-block', viewer: { id: 42 } }),
});

// The host re-posts BLOCK_INIT on an interval until it observes BLOCK_READY,
// and its inbound channel is rate-limited across all types — so a repeat must
// be a complete no-op.
await step('after-second-init', {
  source: parentWindow,
  origin: HOST_ORIGIN,
  data: init({ token: 'tok_abc', blockId: 'ready-block', viewer: { id: 42 } }),
});

process.stdout.write(
  JSON.stringify(
    {
      emitterPath,
      throwFirstPost,
      loadError,
      hostOrigin: HOST_ORIGIN,
      windowMessageListeners: (listeners.message ?? []).length,
      registeredTypes: Object.keys(listeners).sort(),
      posted,
      threw,
      listenerErrors,
      steps,
    },
    null,
    2
  ) + '\n'
);
