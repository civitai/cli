// The ready-ack emitter. It answers BLOCK_INIT with BLOCK_READY and never
// posts RESIZE_IFRAME.
window.addEventListener('message', function (event) {
  if (event.source !== window.parent) return;
  var data = event.data;
  if (!data || data.type !== 'BLOCK_INIT') return;
  window.parent.postMessage(
    { type: 'BLOCK_READY', payload: { height: 0 } },
    event.origin
  );
});
