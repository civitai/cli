package blockproto

import (
	"testing"
)

// TestMountConnectingSetMembership pins the classification that matters, in
// BOTH directions, with names taken from the SDK source the set was derived
// from (see blocksReactMountConnecting). Spot rows, not a copy of the map: a
// copy would only assert the map equals itself.
func TestMountConnectingSetMembership(t *testing.T) {
	in := []string{
		"useBlockContext", "getTransport", // the remedy the advisory names
		"useBlockToken", "useHostOrigin", // render, via useTransportSnapshot / directly
		"useViewer", "useBuzzBalance", // mount effect
		"useEntitlements", // render, via useBlockContext
	}
	// 🔴 useBuzzWorkflow is THE production defect: it is the hook an app used
	// alone, and it connects only inside its returned callbacks. If it ever
	// lands in the set, the finding is inert on exactly the app that broke.
	out := []string{"useBuzzWorkflow", "useBatchEstimate", "useBuzzPurchase", "useBlockAnalytics",
		"useRequestSignIn", "useBlockBreakpoint",
		// CONDITIONAL mount effects: each connects at mount only on some inputs
		// and not in its no-op case (non-positive id / ref not attached / no
		// ids). Listing one silences the finding on that no-op case — the
		// incident's own shape — so they are out.
		"useWildcardPack", "useBlockResize", "useWorkflowBatch"}
	set := map[string]bool{}
	for _, n := range BlocksReactMountConnecting() {
		set[n] = true
	}
	for _, n := range in {
		if !set[n] {
			t.Errorf("%s constructs the transport at mount (SDK 0.71.0) but is not in the set — an app "+
				"using it would be warned at although it acks", n)
		}
	}
	for _, n := range out {
		if set[n] {
			t.Errorf("%s connects only inside a caller-invoked callback, but is in the set — an app using "+
				"only it would pass while never sending BLOCK_READY", n)
		}
	}
}

// TestReferencesMountConnectingAPI pins the identifier boundary. Every reject
// row CONTAINS a set member as a substring, so a `strings.Contains`
// implementation fails them.
func TestReferencesMountConnectingAPI(t *testing.T) {
	cases := []struct {
		code string
		want bool
	}{
		{"const { ready } = useBlockContext();", true},
		{"useBlockContext()", true}, // at start of input
		{"x = getTransport", true},  // at end of input
		{"Blocks.useBlockContext();", true},
		{"import { useBlockContext as useCtx } from '@civitai/blocks-react';", true},
		{"<TipButton amount={5} />", true},

		{"useBlockContextual();", false},
		{"myuseBlockContext();", false},
		{"$getTransport();", false},
		{"getTransport_();", false},
		{"getTransport$();", false},
		{"const useBlockContext2 = 1;", false},
		{"const { submit } = useBuzzWorkflow();", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := ReferencesMountConnectingAPI(tc.code); got != tc.want {
			t.Errorf("ReferencesMountConnectingAPI(%q) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

// TestTSImporterJSSpecifierResolvesToTS pins the `./App.js` -> `./App.tsx`
// rewrite, its TS-importer condition, and that a real `.js` file still wins.
func TestTSImporterJSSpecifierResolvesToTS(t *testing.T) {
	t.Run("a TS importer's ./App.js names ./App.tsx", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script type="module" src="/src/main.tsx"></script>`},
			{"src/main.tsx", `import { App } from './App.js';`},
			{"src/App.tsx", `export const App = 1;`},
		})
		g := ResolveEntryGraph(dir, EntryGraphOptions{})
		if !g.Complete || !graphHasRel(g, "src/App.tsx") {
			t.Fatalf("Complete=%v files=%v gaps=%v, want src/App.tsx in a complete graph", g.Complete, graphRels(g), g.Gaps)
		}
	})
	t.Run(".mjs names .mts and .jsx names .tsx", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script type="module" src="/src/main.ts"></script>`},
			{"src/main.ts", "import './a.mjs';\nimport './b.jsx';"},
			{"src/a.mts", `export {};`},
			{"src/b.tsx", `export {};`},
		})
		g := ResolveEntryGraph(dir, EntryGraphOptions{})
		if !g.Complete || !graphHasRel(g, "src/a.mts") || !graphHasRel(g, "src/b.tsx") {
			t.Fatalf("Complete=%v files=%v gaps=%v", g.Complete, graphRels(g), g.Gaps)
		}
	})
	t.Run("a JS importer gets no rewrite", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script type="module" src="/src/main.js"></script>`},
			{"src/main.js", `import { App } from './App.js';`},
			{"src/App.tsx", `export const App = 1;`},
		})
		if g := ResolveEntryGraph(dir, EntryGraphOptions{}); g.Complete {
			t.Fatalf("a .js importer's ./App.js resolved to App.tsx — Vite applies the rewrite to TS importers only; files=%v", graphRels(g))
		}
	})
	t.Run("an HTML src gets no rewrite", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script type="module" src="./main.js"></script>`},
			{"main.ts", `export {};`},
		})
		if g := ResolveEntryGraph(dir, EntryGraphOptions{}); g.Complete {
			t.Fatalf("a <script src> of main.js resolved to main.ts — a browser fetches the literal URL")
		}
	})
	t.Run("an existing .js file wins over the .ts sibling", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script type="module" src="/src/main.ts"></script>`},
			{"src/main.ts", `import './x.js';`},
			{"src/x.js", `export {};`},
			{"src/x.ts", `export {};`},
		})
		g := ResolveEntryGraph(dir, EntryGraphOptions{})
		if !graphHasRel(g, "src/x.js") || graphHasRel(g, "src/x.ts") {
			t.Fatalf("files=%v, want src/x.js and not src/x.ts", graphRels(g))
		}
	})
}
