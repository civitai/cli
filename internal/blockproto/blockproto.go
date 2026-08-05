// Package blockproto is the CLI's single authority for the block -> host
// postMessage contract that scaffolded, SDK-free apps must implement
// themselves.
//
// It exists so the handshake code has exactly ONE copy in this repo. The
// templates do not each carry their own `.tmpl` of it — `scaffold.Render`
// writes these bytes verbatim into every template that needs it, and
// `internal/scaffold/ready_ack_contract_test.go` asserts the rendered copies
// are byte-identical to what `ReadyAckSource` returns. Two hand-maintained
// copies would drift, and the failure mode of drift is invisible: the app
// renders locally and dies only inside the real host.
//
// This is a VENDORED MIRROR of a server-side contract (civitai/civitai ->
// src/components/AppBlocks/PageBlockHost.tsx and src/hooks/usePostMessage.ts),
// in the same sense as `schema/`, the slot registry in
// `internal/validate/targets.go`, and the dev-embed mirror in
// `internal/devtunnel`. When the platform changes the handshake, this file
// changes with it. See AGENTS.md, "Intentional decisions that look wrong".
package blockproto

import _ "embed"

// readyAck is the canonical ready-ack emitter. It is plain, dependency-free,
// ES5-safe JavaScript so the no-build `static` template can load it with a
// bare <script> tag and the bundled templates can `import` it unchanged.
//
//go:embed ready-ack.js
var readyAck []byte

// ReadyAckFilename is the on-disk name the emitter is scaffolded under. It is
// deliberately host-flavoured rather than generic (`host.js`) so it reads as
// platform plumbing an author should leave alone.
const ReadyAckFilename = "civitai-host.js"

// ReadyAckSource returns the canonical ready-ack emitter source.
//
// It returns a fresh copy each call: the embedded bytes back every scaffold
// this process writes, so handing out the underlying slice would let one
// caller's mutation corrupt every later render.
func ReadyAckSource() []byte {
	out := make([]byte, len(readyAck))
	copy(out, readyAck)
	return out
}
