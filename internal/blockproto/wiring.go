package blockproto

// wiring.go answers "is the vendored emitter at ackRel actually LOADED by this
// project?" on top of the entry graph.
//
// 🔴 Every reference is RESOLVED to a path and compared with where the emitter
// actually IS. It is deliberately NOT a basename match. A basename match is a
// SPELLED check rather than a structural one, and it passes while the hazard
// exists in a different shape — both of these were MEASURED surviving the
// entire suite before the original (test-only) version of this was rewritten:
//
//	static/index.html    src="./vendor/civitai-host.js"          -> ships a 404
//	page-vite/main.jsx   import '../nonexistent/civitai-host.js' -> build fails
//
// Both reproduce #206 with every check green.

import (
	"os"
	"path/filepath"
	"strings"
)

// readyAckWiringDepth keeps Guard A's original reach: index.html's `<script
// src>` entries (depth 1) and the direct imports of those entries (depth 2).
// One level of imports on purpose — a TEMPLATE's emitter is meant to be loaded
// by the entry itself, and burying it behind a chain is a template smell.
// `validate` walks an author's project and uses the deeper default instead,
// because an author's module layout is not ours to police.
const readyAckWiringDepth = 2

// ReadyAckWiring returns a description of where the emitter at ackRel is loaded
// from, or an error if nothing in the entry graph reaches it.
//
// It follows the same path a browser does — index.html's `<script>` tags, then
// (for a module entry) that module's imports — rather than grepping the tree, so
// an emitter referenced only from a README, a comment, or a file nothing loads
// does not satisfy it.
func ReadyAckWiring(dir, ackRel string) (string, error) {
	want := filepath.Clean(filepath.Join(dir, filepath.FromSlash(ackRel)))
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return "", err
	}
	g := ResolveEntryGraph(dir, EntryGraphOptions{MaxDepth: readyAckWiringDepth})
	if i := g.FileAt(want); i >= 0 {
		return g.Chain(i), nil
	}
	if g.ScriptRefs == 0 {
		return "", ErrNoScriptTags
	}
	return "", readyAckWiringError(ErrNotReached.Error() + "\n  references resolved:\n    " +
		strings.Join(g.Trace, "\n    "))
}

type readyAckWiringError string

func (e readyAckWiringError) Error() string { return string(e) }

// The two rejection reasons callers pin by name. A test that only asserts "an
// error came back" is green whichever branch produced it, which is how an early
// return gets deleted unnoticed.
const (
	ErrNoScriptTags = readyAckWiringError("the rendered index.html has no <script src> at all")
	ErrNotReached   = readyAckWiringError("no <script src> in index.html resolves to it, and no import in an entry module resolves to it")
)
