package validate

// readyack_sdkmount_test.go covers the SDK-MOUNT finding: a page app depending
// on `@civitai/blocks-react` whose entry graph never constructs the SDK
// transport at mount, so BLOCK_READY never goes out. Reproduced in production
// on an app using only `useBuzzWorkflow`, which passed `validate` and `submit`
// because the dependency alone used to end the ready-ack check.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/scaffold"
)

const sdkPkgJSON = `{"dependencies": {"react": "^19.0.0", "react-dom": "^19.0.0", "@civitai/blocks-react": "^0.71.0"}}`

// sdkIndexHTML loads the entry module the way a Vite page app does.
const sdkIndexHTML = `<!doctype html><div id="root"></div><script type="module" src="/src/main.tsx"></script>`

// lazyOnlyApp is the production shape: the ONLY SDK hook is useBuzzWorkflow,
// which calls getTransport() inside the functions it returns, never at mount.
const lazyOnlyApp = "import { useBuzzWorkflow } from '@civitai/blocks-react';\n" +
	"export function App() {\n" +
	"  const { estimate, submit } = useBuzzWorkflow();\n" +
	"  return <button onClick={() => submit(estimate)}>go</button>;\n" +
	"}\n"

// sdkMainImportingApp is an entry module that renders ./App.
const sdkMainImportingApp = "import { createRoot } from 'react-dom/client';\n" +
	"import { App } from './App';\n" +
	"createRoot(document.getElementById('root')).render(<App />);\n"

// sdkProject builds a blocks-react page app whose src/App.tsx is app. Extra
// files override or extend the defaults.
func sdkProject(t *testing.T, app string, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"package.json": sdkPkgJSON,
		"index.html":   sdkIndexHTML,
		"src/main.tsx": sdkMainImportingApp,
		"src/App.tsx":  app,
	}
	for k, v := range extra {
		files[k] = v
	}
	return ackProject(t, ackManifest(true), files)
}

// TestSDKMountConnection is the fixture matrix for the SDK-mount finding.
//
// 🔴 EVERY "silent" ROW HAS A FIRING TWIN. A silent row alone is satisfied by a
// check that never fires; each one below differs from lazyOnlyApp (which fires,
// row 1) only in the property under test.
func TestSDKMountConnection(t *testing.T) {
	cases := []struct {
		name  string
		app   string
		extra map[string]string
		want  string
	}{
		// (a) THE PRODUCTION DEFECT.
		{"only useBuzzWorkflow: nothing constructs the transport at mount", lazyOnlyApp, nil, "sdk-unconnected"},

		// (b) The fix, in the root component.
		{"useBlockContext() in the root component", "import { useBlockContext, useBuzzWorkflow } from '@civitai/blocks-react';\n" +
			"export function App() { const { ready } = useBlockContext(); const { submit } = useBuzzWorkflow(); return ready && submit; }\n",
			nil, ""},
		{"getTransport() at module scope", "import { getTransport, useBuzzWorkflow } from '@civitai/blocks-react';\n" +
			"getTransport();\nexport function App() { const { submit } = useBuzzWorkflow(); return submit; }\n", nil, ""},
		{"a hook that connects in a MOUNT EFFECT (useBuzzBalance)", "import { useBuzzBalance } from '@civitai/blocks-react';\n" +
			"export function App() { const { balance } = useBuzzBalance(); return balance; }\n", nil, ""},
		{"a namespace import calling the hook", "import * as Blocks from '@civitai/blocks-react';\n" +
			"export function App() { Blocks.useBlockContext(); return Blocks.useBuzzWorkflow(); }\n", nil, ""},
		{"the connecting hook lives in a CHILD module the root imports", "import { Header } from './Header';\n" +
			"import { useBuzzWorkflow } from '@civitai/blocks-react';\n" +
			"export function App() { useBuzzWorkflow(); return <Header />; }\n",
			map[string]string{"src/Header.tsx": "import { useBlockTheme } from '@civitai/blocks-react';\n" +
				"export function Header() { return useBlockTheme(); }\n"}, ""},
		{"a hand-written emitter loaded beside the SDK dependency", lazyOnlyApp,
			map[string]string{"index.html": `<!doctype html><script src="./civitai-host.js"></script>` + sdkIndexHTML,
				"civitai-host.js": realEmitter()}, ""},

		// CONDITIONAL mount-effect hooks are NOT evidence: in their no-op case
		// (as written here) they never reach getTransport() at mount on a
		// pre-fix SDK, so an app using only one of them is the incident's shape.
		{"only useWildcardPack, with a non-positive id", "import { useWildcardPack } from '@civitai/blocks-react';\n" +
			"export function App() { const { pack } = useWildcardPack(0); return pack; }\n", nil, "sdk-unconnected"},
		{"only useBlockResize, ref not attached at mount", "import { useRef } from 'react';\n" +
			"import { useBlockResize } from '@civitai/blocks-react';\n" +
			"export function App() { const ref = useRef(null); useBlockResize(ref); return null; }\n", nil, "sdk-unconnected"},
		{"only useWorkflowBatch, with no ids", "import { useWorkflowBatch } from '@civitai/blocks-react';\n" +
			"export function App() { const { cells } = useWorkflowBatch([]); return cells; }\n", nil, "sdk-unconnected"},

		// Near-misses that must NOT satisfy it — each fires.
		{"a COMMENT naming useBlockContext does not count", "// TODO: call useBlockContext() here\n" +
			"/* getTransport() would also work */\n" + lazyOnlyApp, nil, "sdk-unconnected"},
		{"an identifier that merely CONTAINS the name does not count", "import { useBuzzWorkflow } from '@civitai/blocks-react';\n" +
			"function useBlockContextual() { return 1; }\nconst $getTransport = 2; const getTransport_ = 3;\n" +
			"export function App() { useBlockContextual(); useBuzzWorkflow(); return $getTransport + getTransport_; }\n",
			nil, "sdk-unconnected"},
		{"an ORPHAN module calling useBlockContext that nothing loads", lazyOnlyApp,
			map[string]string{"src/Unused.tsx": "import { useBlockContext } from '@civitai/blocks-react';\n" +
				"export function Unused() { return useBlockContext(); }\n"}, "sdk-unconnected"},
		{"a stylesheet the entry loads is not evidence", lazyOnlyApp,
			map[string]string{"src/main.tsx": "import './index.css';\n" + sdkMainImportingApp,
				"src/index.css": ".useBlockContext{color:red}\n"}, "sdk-unconnected"},

		// (c) THE DOCTRINE: an incomplete graph never yields this finding.
		{"an unresolvable bundler alias makes the graph incomplete", lazyOnlyApp,
			map[string]string{"src/main.tsx": "import { App } from '@/App';\n" + "export const x = App;\n"}, ""},
		{"no root index.html: nothing to resolve", lazyOnlyApp,
			map[string]string{"index.html": "", "src/index.html": sdkIndexHTML}, ""},

		// Scope of the gate.
		{"another acking package beside blocks-react keeps the old silence", lazyOnlyApp,
			// react + react-dom stay declared: without them main.tsx's
			// `react-dom/client` import is a gap, the graph goes incomplete,
			// and this row is silent by DOCTRINE whether or not the
			// other-acker gate exists (a measured survivor before this note).
			map[string]string{"package.json": `{"dependencies": {"react": "^19.0.0", "react-dom": "^19.0.0", ` +
				`"@civitai/blocks-react": "^0.71.0", "@civitai/sdk": "^0.10.1"}}`}, ""},
		{"an @civitai/sdk app (no blocks-react) is not this check's to judge", lazyOnlyApp,
			map[string]string{"package.json": `{"dependencies": {"react": "^19.0.0", "react-dom": "^19.0.0", ` +
				`"@civitai/sdk": "^0.10.1"}}`}, ""},
	}
	fired := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := sdkProject(t, tc.app, tc.extra)
			if tc.name == "no root index.html: nothing to resolve" {
				if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
					t.Fatal(err)
				}
			}
			res := wantAckKind(t, dir, tc.want)
			if tc.want != "" {
				fired++
				// Advisory, never an Error (item 18). The fixture's other
				// Errors (no lockfile beside a buildCommand) are not this
				// test's subject, so match on the message, not res.OK().
				for _, e := range res.Errors {
					if e.Message == readyAckAdviceSDKNotConnected {
						t.Fatalf("the SDK-mount finding reached Errors — it must stay a warning")
					}
				}
			}
		})
	}
	// COUNT POSITIVE CONTROL: a matrix whose firing rows all silently stopped
	// running would otherwise read as green.
	if fired < 5 {
		t.Fatalf("only %d firing rows ran, want at least 5", fired)
	}
}

// TestSDKMountSilenceOnIncompleteGraphIsTheGraphs is the control for row (c):
// the alias project goes silent BECAUSE the graph is incomplete — not because
// the check never runs on it. Resolving the alias (a relative import of the
// same file) makes the very same lazy-only app fire.
func TestSDKMountSilenceOnIncompleteGraphIsTheGraphs(t *testing.T) {
	alias := sdkProject(t, lazyOnlyApp, map[string]string{"src/main.tsx": "import { App } from '@/App';\nexport const x = App;\n"})
	deps, _ := packageDeps(alias)
	if g := blockproto.ResolveEntryGraph(alias, blockproto.EntryGraphOptions{Dependencies: deps}); g.Complete {
		t.Fatalf("the alias fixture resolved COMPLETELY — it no longer exercises the incomplete-graph branch")
	}
	wantAckKind(t, alias, "")

	resolved := sdkProject(t, lazyOnlyApp, map[string]string{"src/main.tsx": "import { App } from './App';\nexport const x = App;\n"})
	wantAckKind(t, resolved, "sdk-unconnected")
}

// TestPageMoneyIsSilentBecauseItConnects pins WHY the shipped blocks-react
// template is accepted. Its entry graph must resolve COMPLETELY — otherwise its
// silence would come from the doctrine branch and say nothing about the hook
// detection — and stripping its mount-connecting references must make it fire.
func TestPageMoneyIsSilentBecauseItConnects(t *testing.T) {
	dir := renderTemplate(t, scaffold.PageMoney)
	deps, state := packageDeps(dir)
	if state != sdkPresent || !deps[blockproto.BlocksReactPackage] {
		t.Fatalf("page-money no longer depends on %s — this test has nothing to pin", blockproto.BlocksReactPackage)
	}
	g := blockproto.ResolveEntryGraph(dir, blockproto.EntryGraphOptions{
		MaxFiles: maxAckGraphFiles, MaxFileBytes: maxAckFileBytes, Dependencies: deps,
	})
	if !g.Complete {
		t.Fatalf("page-money's entry graph is INCOMPLETE (%v) — its silence would be the doctrine's, not "+
			"the hook detection's", g.Gaps)
	}
	wantAckKind(t, dir, "")

	// Negative control: neuter every mount-connecting name across the loaded
	// source (renaming, so the files still parse as the same graph) and the
	// finding must fire. Proves the template is silent because of what it calls.
	for _, f := range g.Files {
		if !readyAckSourceExts[strings.ToLower(filepath.Ext(f.Rel))] {
			continue
		}
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(raw)
		for _, name := range blockproto.BlocksReactMountConnecting() {
			s = strings.ReplaceAll(s, name, "neutered"+name)
		}
		if err := os.WriteFile(f.Path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wantAckKind(t, dir, "sdk-unconnected")
}

// TestSDKMountAdviceIsActionable pins the remedy the author acts on.
func TestSDKMountAdviceIsActionable(t *testing.T) {
	msg := readyAckAdviceSDKNotConnected
	for _, want := range []string{
		"call useBlockContext() (or getTransport()) in your root component",
		"didn't load in time",
		"10 s",
		"useBuzzWorkflow",
		blockproto.BlocksReactPackage,
		"--strict",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the SDK-mount advisory does not say %q:\n%s", want, msg)
		}
	}
	// It must NOT tell an SDK app to scaffold and load the emitter: two
	// handshakes race. That is the emitter family's remedy, not this one's.
	if strings.Contains(msg, "civitai app init") {
		t.Errorf("the SDK-mount advisory tells the author to scaffold the emitter:\n%s", msg)
	}
	if strings.ContainsAny(msg, "\n\r") {
		t.Errorf("the SDK-mount advisory is not one line — it rides in a --json message field")
	}
	for _, other := range readyAckAdvisories {
		if other == msg {
			t.Errorf("the SDK-mount advisory is a member of readyAckAdvisories; see its doc comment")
		}
	}
}
