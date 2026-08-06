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
// src/components/AppBlocks/PageBlockHost.tsx and
// src/components/AppBlocks/usePostMessage.ts, read at civitai@35a9598dc9), in
// the same sense as `schema/`, the slot registry in
// `internal/validate/targets.go`, and the dev-embed mirror in
// `internal/devtunnel`. When the platform changes the handshake, this file
// changes with it. See AGENTS.md, "Intentional decisions that look wrong".
package blockproto

import (
	_ "embed"
	"sort"
)

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

// ackingPackages is the set of npm packages that perform the ready-ack FOR the
// app, so a project depending on one does not need the vendored emitter (and
// must not also ship it — two handshakes race, see the emitter's own header).
//
// 🔴 IT IS AN EXACT SET, NOT THE `@civitai/` SCOPE, AND THAT DISTINCTION IS
// MEASURED. Of the six published first-party packages, only ONE acks:
//
//	@civitai/blocks-react   0.39.0  ACKS — dist/internal/iframeTransport.js:311
//	                                `this.dispatch('BLOCK_READY', {height: 0})`
//	@civitai/app-sdk        0.31.0  does NOT — 17 runtime .js files, ZERO contain
//	                                BLOCK_READY; the only hits are a .d.ts type
//	                                literal and the README. It is the server-side
//	                                OAuth/session SDK and declares no dependencies,
//	                                so it cannot be acking transitively either.
//	@civitai/theme          0.2.0   does NOT — framework-agnostic CSS tokens
//	@civitai/components     0.3.0   does NOT — "Plain HTML or any framework" CSS
//	@civitai/components-react 0.3.0 does NOT — thin React bindings over the above
//	@civitai/cli            0.1.89  does NOT — installs this Go binary
//
// (Measured from fresh npm tarballs at the versions named, 2026-08.)
//
// Treating the whole scope as evidence is therefore wrong for FOUR of the six,
// and wrong in the expensive direction: `@civitai/theme` and `@civitai/components`
// are exactly what a hand-written, no-build page app would install, so a scope
// check goes SILENT on a genuinely broken app. Verified live — adding
// `@civitai/theme` to a static scaffold with `civitai-host.js` deleted (the
// literal #206 shape) suppressed the warning.
//
// Adding a package here is a claim that its RUNTIME code posts BLOCK_READY.
// Verify that against the published tarball — not the README, not the types —
// and record the version you measured.
var ackingPackages = map[string]bool{
	"@civitai/blocks-react": true,
}

// PackageAcksReady reports whether an npm dependency named `name` performs the
// block -> host ready-ack on the app's behalf.
//
// The comparison is EXACT. A prefix or substring test would accept a sibling
// (`@civitai/blocks-react-native`) or a third-party fork
// (`@myorg/@civitai/blocks-react`) that makes no such promise, which is the
// widening class every other pattern in this repo carries an explicit boundary
// against (see the terminating character classes in internal/antipattern).
func PackageAcksReady(name string) bool { return ackingPackages[name] }

// AckingPackages returns the acking package names, for messages and tests.
func AckingPackages() []string {
	out := make([]string, 0, len(ackingPackages))
	for name := range ackingPackages {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
