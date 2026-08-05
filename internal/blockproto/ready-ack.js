// civitai-host.js — the block -> host readiness handshake. Do not delete.
//
// WHAT THIS IS
// A VENDORED MIRROR of the Civitai App host contract. The authority lives in
// the platform, not here:
//   civitai/civitai -> src/components/AppBlocks/PageBlockHost.tsx
//   civitai/civitai -> src/hooks/usePostMessage.ts
// Apps built on `@civitai/blocks-react` never need this file — its
// `IframeTransport` performs the same handshake internally. A dependency-free
// app has to send it itself, which is what this file does.
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
// WHY ACK ON `BLOCK_INIT` RATHER THAN ON LOAD
// Posting on load also works today — this is not a fix for a broken
// alternative. It is chosen because it is strictly more robust:
//   1. No race. The host registers its `BLOCK_READY` subscriber inside an
//      effect and SILENTLY DROPS a message whose type has no subscriber yet,
//      with no retry. `BLOCK_INIT` cannot arrive before that point, and the
//      host re-posts it on an interval until it is acked.
//   2. No `'*'`. The init event hands us `event.origin`, so the ack is
//      addressed to the framing host instead of broadcast to anyone.
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
    // Only the framing host, and only its init message. Repeats are a no-op:
    // the host re-posts `BLOCK_INIT` until it sees the ack, and its inbound
    // messages are rate-limited, so answering every retry would burn budget
    // other messages need.
    if (acked || event.source !== window.parent) return;
    var data = event.data;
    if (!data || typeof data !== 'object' || data.type !== 'BLOCK_INIT') return;

    acked = true;
    // height: 0 is a placeholder. A page app fills the surface and does not
    // size to content, so the host ignores it.
    window.parent.postMessage(
      { type: 'BLOCK_READY', payload: { height: 0 } },
      event.origin
    );
  });
})();
