package scaffold

// The `bootSkeleton-not-empty` gate, implemented over a real HTML parse.
//
// WHY THIS EXISTS AS CODE RATHER THAN A CONVENTION. `bootSkeleton: true` in a
// block manifest tells the Civitai run host to stand down THREE things at once:
// the opaque branded veil, the iframe's `opacity: 0 → 1` reveal, and the
// `translateY(8px)` settle. The iframe is therefore opaque and unveiled from
// mount, and whatever the block's entry document paints IS the loading state.
// Declare the flag over an EMPTY mount container and the viewer gets a blank
// iframe for the entire load — strictly worse than never opting in.
//
// So the manifest key and the markup are ONE change with two halves, and the
// hazard is that a future template (or a future edit to an existing one) ships
// one half. That is what this function is for: `TestEveryScaffoldTemplate
// PassesTheBootSkeletonGate` runs it over every template's real rendered output,
// so a template declaring the key without painting anything fails the build
// rather than shipping to an author.
//
// The rules below are the platform's `bootSkeleton-not-empty` gate verbatim, so
// the CLI's verdict and the platform's cannot drift by construction. The gate
// keys on EMPTINESS — the actual hazard — and treats `data-boot-skeleton` as the
// affordance that makes the non-empty case unambiguous, plus one placement rule
// (a marker outside the container is never replaced by the app's own render, so
// it stays on screen after mount).

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// BootSkeletonMarkerAttr is the marker attribute a boot skeleton's outermost
// element carries.
//
// An ATTRIBUTE and not a class, deliberately: a class name is fair game for
// CSS-modules hashing or a purge pass, while an attribute survives the bundler
// and is deterministically greppable out of the BUILT html.
const BootSkeletonMarkerAttr = "data-boot-skeleton"

// bootSkeletonContainerIDs are the mount-container ids the gate recognises,
// alongside the `data-app-root` attribute. Matches the platform gate's
// `#root, #app, [data-app-root]`.
var bootSkeletonContainerIDs = map[string]bool{"root": true, "app": true}

// bootSkeletonInertTags are the tags that cannot satisfy the non-emptiness rule.
//
// 🔴 THEIR SUBTREES ARE SKIPPED WHOLESALE, not just the elements themselves, and
// that reading is forced by the contract's own worked case rather than chosen.
// The rule is "at least one descendant element whose tag is not one of these, OR
// at least one non-whitespace text node" — and a `#root` holding only a
// `<script>` MUST fail it. A `<script>` almost always contains a non-whitespace
// TEXT node (its source), so counting text found anywhere in the subtree would
// pass that case and make the gate vacuous for the one shape it most needs to
// catch. The same argument covers `<template>`, whose children parse as real
// element nodes here yet paint nothing.
var bootSkeletonInertTags = map[string]bool{
	"script":   true,
	"template": true,
	"style":    true,
	"link":     true,
	"noscript": true,
}

// ErrBootSkeletonEmptyContainer and ErrBootSkeletonMarkerOutsideContainer let a
// caller (and the table test) distinguish the two failure modes without matching
// on message text.
var (
	// ErrBootSkeletonEmptyContainer is returned when the manifest declares
	// bootSkeleton over a mount container that paints nothing.
	ErrBootSkeletonEmptyContainer = errors.New("bootSkeleton declared over an empty mount container")
	// ErrBootSkeletonMarkerOutsideContainer is returned when a
	// [data-boot-skeleton] element exists but sits outside every mount
	// container, where the app's own render can never replace it.
	ErrBootSkeletonMarkerOutsideContainer = errors.New("[data-boot-skeleton] is outside the mount container")
)

// BootSkeletonGateOK applies the platform's blocking `bootSkeleton-not-empty`
// gate to one app: `manifestJSON` is the block manifest, `htmlDoc` is the entry
// document the platform would serve (the BUILT `<outputDir>/index.html`, or the
// shipped `index.html` for a no-build app).
//
// It returns nil when the gate passes and a wrapped
// ErrBootSkeletonEmptyContainer / ErrBootSkeletonMarkerOutsideContainer
// carrying the platform's own message when it does not.
//
// PASSES VACUOUSLY, on purpose, in two cases the platform also passes:
//   - the manifest does not set `bootSkeleton: true` (the gate does not apply);
//   - the document has no recognisable mount container (`#root`, `#app`,
//     `[data-app-root]`), so there is no empty-container hazard to detect and
//     the gate does not guess.
//
// Callers that need a non-vacuous answer should assert a container exists
// separately — see `TestReactTemplatesPaintABootSkeletonInsideTheMountContainer`,
// which pins the container AND the marker's placement rather than relying on
// this returning nil.
func BootSkeletonGateOK(manifestJSON, htmlDoc []byte) error {
	declared, err := bootSkeletonDeclared(manifestJSON)
	if err != nil {
		return err
	}
	if !declared {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(string(htmlDoc)))
	if err != nil {
		return fmt.Errorf("parse entry document: %w", err)
	}

	containers := findBootSkeletonContainers(doc)
	if len(containers) == 0 {
		// Rule 2 — nothing identifiable to mount into, so nothing to claim.
		return nil
	}

	// Rule 3 — every container must paint something.
	for _, c := range containers {
		if !bootSkeletonContainerPaints(c) {
			return fmt.Errorf(
				"%w: manifest declares bootSkeleton: true but %s is empty in the built index.html — "+
					"the run host stands down its loading veil for this app, so the viewer would see a blank "+
					"iframe for the whole load. Either paint a boot state inside the container, or remove "+
					"bootSkeleton from the manifest",
				ErrBootSkeletonEmptyContainer, bootSkeletonContainerLabel(c))
		}
	}

	// Rule 4 — a marker outside every container is never replaced on mount.
	for _, m := range findAllWithAttr(doc, BootSkeletonMarkerAttr) {
		inside := false
		for _, c := range containers {
			if isDescendantOf(m, c) {
				inside = true
				break
			}
		}
		if !inside {
			return fmt.Errorf(
				"%w: the [data-boot-skeleton] element is outside the mount container, so the app's own "+
					"render will not replace it and it will stay on screen after mount",
				ErrBootSkeletonMarkerOutsideContainer)
		}
	}

	return nil
}

// bootSkeletonDeclared reports whether the manifest sets `bootSkeleton` to the
// boolean true. Any other value — absent, false, or a non-boolean — is "not
// declared"; the gate is about the opt-in, not about type-checking the manifest
// (the JSON-Schema validator owns that).
func bootSkeletonDeclared(manifestJSON []byte) (bool, error) {
	var m map[string]any
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return false, fmt.Errorf("parse block manifest: %w", err)
	}
	b, ok := m["bootSkeleton"].(bool)
	return ok && b, nil
}

// findBootSkeletonContainers returns every element matching
// `#root, #app, [data-app-root]`, in document order.
func findBootSkeletonContainers(root *html.Node) []*html.Node {
	var out []*html.Node
	walkElements(root, func(n *html.Node) {
		for _, a := range n.Attr {
			if a.Key == "id" && bootSkeletonContainerIDs[a.Val] {
				out = append(out, n)
				return
			}
			if a.Key == "data-app-root" {
				out = append(out, n)
				return
			}
		}
	})
	return out
}

// bootSkeletonContainerPaints implements rule 3 for one container. See
// bootSkeletonInertTags for why inert subtrees are skipped wholesale.
// DEPTH IS NOT SCANNED, and that is a consequence rather than a shortcut: a
// non-inert element at ANY depth necessarily has a non-inert ANCESTOR chain up
// to the container (an inert tag's subtree is skipped wholesale, and the only
// way to reach depth 2 is through a depth-1 element), so the direct children
// decide the answer. Text is likewise only counted where it can paint —
// directly in the container, never inside an inert element.
func bootSkeletonContainerPaints(container *html.Node) bool {
	for c := container.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.ElementNode:
			if !bootSkeletonInertTags[strings.ToLower(c.Data)] {
				return true
			}
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return true
			}
		}
	}
	return false
}

// bootSkeletonContainerLabel renders a container the way the platform's message
// names it: `#root` for an id, `[data-app-root]` for the attribute form.
func bootSkeletonContainerLabel(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "id" && bootSkeletonContainerIDs[a.Val] {
			return "#" + a.Val
		}
	}
	return "[data-app-root]"
}

// findAllWithAttr returns every element carrying the named attribute.
func findAllWithAttr(root *html.Node, attr string) []*html.Node {
	var out []*html.Node
	walkElements(root, func(n *html.Node) {
		for _, a := range n.Attr {
			if a.Key == attr {
				out = append(out, n)
				return
			}
		}
	})
	return out
}

// isDescendantOf reports whether n is a strict descendant of anc.
func isDescendantOf(n, anc *html.Node) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p == anc {
			return true
		}
	}
	return false
}

// walkElements calls fn for every element node under root, in document order.
func walkElements(root *html.Node, fn func(*html.Node)) {
	if root.Type == html.ElementNode {
		fn(root)
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		walkElements(c, fn)
	}
}
