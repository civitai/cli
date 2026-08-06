// FIXTURE — deliberately WRONG. Never shipped to an author.
//
// This is the runtime guard's NEGATIVE CONTROL: it is what a plausible-looking
// hand-rolled ack gets wrong (top-level fields instead of the `{type,payload}`
// envelope, a `'*'` target, no BLOCK_INIT gate, no dedupe). The Go assertions
// are run against it and MUST report every one of those problems — otherwise
// their silence on the real emitter says nothing.
window.addEventListener('message', function () {
  window.parent.postMessage({ type: 'BLOCK_READY', height: 0 }, '*');
});
