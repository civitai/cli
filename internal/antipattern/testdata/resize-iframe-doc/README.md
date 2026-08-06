# Sample Block

`RESIZE_IFRAME` is **not** part of a page app's protocol: the host renders a
page block full-viewport, so it does not size to content and ignores the
message. Do not post RESIZE_IFRAME from a page app.

The one message a page app must send is `BLOCK_READY`.
