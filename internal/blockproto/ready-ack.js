// civitai-host.js — the block -> host readiness handshake. Do not delete.
//
// WHAT THIS IS
// A VENDORED MIRROR of the Civitai App host contract. The authority lives in
// the platform, not here (read at civitai@35a9598dc9):
//   civitai/civitai -> src/components/AppBlocks/PageBlockHost.tsx
//   civitai/civitai -> src/components/AppBlocks/usePostMessage.ts
// Apps built on `@civitai/blocks-react` never need this file — its
// `IframeTransport` performs the same handshake internally. A dependency-free
// app has to send it itself, which is what this file does.
//
// 🔴 IF YOU ADOPT `@civitai/blocks-react`, DELETE THIS FILE IN THE SAME CHANGE.
// Keeping both is worse than having neither. Whichever handshake answers the
// host's FIRST `BLOCK_INIT` cancels the host's retry loop *and* its readiness
// timeout. If this file wins that race, the SDK's transport can be left never
// having seen an init: its own `waitForInit` rejects after 10s, and the host
// sits in a "ready" state showing an app that never started — with no retry and
// no error card. That is a worse failure than the one this file exists to
// prevent, and it is silent.
//
// WHY IT EXISTS
// The host mounts your app in a sandboxed iframe and will not reveal it until
// the app announces it is alive with a `BLOCK_READY` message. That message is
// the ONLY transition into the host's `ready` state. Without it the host runs
// out its bounded init retries and replaces the app with a failure card — the
// app itself is fine, it just never said hello.
//
// THE ENVELOPE IS PART OF THE CONTRACT
// The host dispatches `event.data.payload` to its subscribers, so EVERY
// block -> host message is `{ type, payload }`. Fields placed at the top level
// (`{ type: 'BLOCK_READY', height: 0 }`) arrive as `payload: undefined`.
//
// 🔴 WHAT THIS FILE DOES *NOT* DO: VERIFY THE SENDER'S IDENTITY.
// It answers `window.parent` — the window that framed us — and addresses the
// reply at whatever origin that window sent from. That establishes "our
// embedder", NOT "Civitai". It is sound for THIS message and only this message,
// because the ack carries no data (`{height: 0}`) and discloses nothing a page
// that chose to frame us does not already know.
// It is NOT sufficient for anything you add later. The moment you handle an
// inbound message that carries data — a token, a viewer, storage, a generation
// result — you must check `event.origin` against an allowlist of origins you
// trust, or any page on the internet can frame your app and feed it whatever it
// likes. This file deliberately does not vendor such an allowlist: the correct
// list (production, preview subdomains, dev tunnels) is platform state that
// changes without notice, and `@civitai/blocks-react` already maintains it from
// `VITE_BLOCK_ALLOWED_PARENT_ORIGINS`. Adopt the SDK before you handle data.
//
// WHY ACK ON `BLOCK_INIT` RATHER THAN ON LOAD
// Posting on load also works today — this is not a fix for a broken
// alternative. It is chosen because it is strictly more robust:
//   1. No race. The host registers its `BLOCK_READY` subscriber inside an
//      effect and SILENTLY DROPS a message whose type has no subscriber yet,
//      with no retry. `BLOCK_INIT` cannot arrive before that point, and the
//      host re-posts it on an interval until it is acked.
//   2. No `'*'`. The init event hands us the sender's origin, so the ack is
//      addressed to the window that framed us instead of broadcast to anyone.
//   3. Correct timing. The host fades its launch overlay and enables pointer
//      events on ready, so acking before the app can paint reveals an empty
//      interactive frame.
//   4. It is what the SDK's own transport does, so both paths behave alike.

(function () {
  'use strict';

  var acked = false;

  // Attached at top level, synchronously — NOT inside a load/DOMContentLoaded
  // handler or any async chain. The host's init retry is bounded by its
  // readiness timeout, so a listener attached late can miss every `BLOCK_INIT`.
  window.addEventListener('message', function (event) {
    // Only our embedder, and only its init message. Repeats are a no-op: the
    // host re-posts `BLOCK_INIT` until it sees the ack, and its inbound
    // messages are rate-limited, so answering every retry would burn budget
    // other messages need.
    if (acked || event.source !== window.parent) return;
    var data = event.data;
    if (!data || typeof data !== 'object' || data.type !== 'BLOCK_INIT') return;

    // height: 0 is a placeholder. A page app fills the surface and does not
    // size to content, so the host ignores it.
    window.parent.postMessage(
      { type: 'BLOCK_READY', payload: { height: 0 } },
      event.origin
    );

    // Latched AFTER the post, not before, so a throw leaves us able to answer
    // the next retry instead of going permanently silent. `postMessage` rejects
    // a targetOrigin it cannot parse as a URL — measured in Chromium 1228,
    // `postMessage(msg, 'null')` throws
    // `SyntaxError: Invalid target origin 'null'` — and `event.origin` is the
    // string "null" whenever the sender is itself at an opaque origin. Today's
    // page host is not at an opaque origin, so this is defence against a host
    // arrangement that does not exist yet, not a bug being fixed.
    acked = true;
  });
})();
