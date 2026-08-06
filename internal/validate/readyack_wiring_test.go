package validate

// readyack_wiring_test.go covers the REACHABILITY tier: the half of the
// ready-ack advisory that asks whether the browser actually loads the file
// posting BLOCK_READY, rather than whether some file in the tree contains the
// literal.
//
// 🔴 EVERY FIXTURE HERE IS A RENDERED TEMPLATE, MUTATED. The defect being
// repaired was reproduced on a real `civitai app init` project, and a
// hand-written string fixture cannot fail the way a real scaffold does — it
// carries none of the template's comments, none of its import graph, and none of
// the near-miss shapes that made the old check look green.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/scaffold"
)

// renderTemplate renders tmpl into a fresh directory and returns it.
func renderTemplate(t *testing.T, tmpl scaffold.Template) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), string(tmpl))
	if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "ack-block", Name: "Ack Block"}); err != nil {
		t.Fatalf("render %s: %v", tmpl, err)
	}
	return dest
}

// editFile applies a replacement to a file in a rendered project and FAILS if
// the replacement did not change anything.
//
// 🔴 The failure check is the point, not defensiveness. A mutation regex that
// silently no-ops has produced false "surviving mutant" results three times in
// this workstream; a fixture mutation that silently no-ops produces the same lie
// in the other direction — a test that passes because the project was never
// broken.
func editFile(t *testing.T, dir, rel, old, new string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if !strings.Contains(string(raw), old) {
		t.Fatalf("fixture mutation is a no-op: %s does not contain %q — the template changed and this "+
			"test would have passed without ever breaking the project", rel, old)
	}
	out := strings.Replace(string(raw), old, new, 1)
	if err := os.WriteFile(p, []byte(out), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// ---------------------------------------------------------------------------
// THE REGRESSION UNDER REPAIR.
// ---------------------------------------------------------------------------

// TestEmitterPresentButNotLoadedWarns is the defect, reproduced.
//
// A blind dogfooder followed this check's own remedy on a pre-fix scaffold:
// copied `civitai-host.js` in, did not reference it from index.html, and
// `civitai app validate --strict` printed `✓ … is valid` and exited 0 for an app
// that was still entirely broken. Measured on the shipped CLI at 0ce0025.
//
// Both shapes below are that defect. They are separated because they fail
// differently: one keeps the emitter and drops the reference (the dogfooder's),
// the other keeps a reference to nothing and hides the ack in a file nobody
// loads (the orphan).
func TestEmitterPresentButNotLoadedWarns(t *testing.T) {
	t.Run("static: the emitter is there, index.html does not load it", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.Static)
		// Positive control: the untouched scaffold is silent, so a warning below
		// cannot come from the template being broken to begin with.
		wantAckKind(t, dir, "")

		editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
		if _, err := os.Stat(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
			t.Fatalf("the emitter must still be present — this case is about the REFERENCE: %v", err)
		}
		wantAckKind(t, dir, "unwired")
	})

	t.Run("page-vite: the emitter is there, the entry module does not import it", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.PageVite)
		wantAckKind(t, dir, "")

		editFile(t, dir, "src/main.jsx", `import './civitai-host.js';`, "")
		if _, err := os.Stat(filepath.Join(dir, "src", blockproto.ReadyAckFilename)); err != nil {
			t.Fatalf("the emitter must still be present: %v", err)
		}
		wantAckKind(t, dir, "unwired")
	})

	t.Run("a stylesheet in the graph is not ack evidence", func(t *testing.T) {
		// The entry graph contains files the browser loads that CANNOT implement
		// a handshake. `src/index.css` really is loaded by page-vite's entry
		// module, so without the readyAckSourceExts gate on the graph scan a
		// stylesheet naming the message would silence the check.
		dir := renderTemplate(t, scaffold.PageVite)
		editFile(t, dir, "src/main.jsx", `import './civitai-host.js';`, "")
		if err := os.Remove(filepath.Join(dir, "src", blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "src", "index.css"),
			[]byte("/* x */\n.a{content:'BLOCK_READY'}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// `missing` rather than `unwired` because the whole-tree scan does not
		// read `.css` either — the same extension policy on both tiers. What this
		// pins is that the graph scan does NOT accept it: drop the extension gate
		// in graphPostsReadyAck and this project goes silent.
		wantAckKind(t, dir, "missing")
	})

	t.Run("an orphan file containing the literal does not count", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.Static)
		// Remove the emitter AND its reference — the app is now the plain #206
		// shape — then hide a real, correct ack in a file nothing loads.
		editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
		if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "orphan.js"), blockproto.ReadyAckSource(), 0o600); err != nil {
			t.Fatal(err)
		}
		wantAckKind(t, dir, "unwired")
	})
}

// TestWiredEmitterIsSilent is the other half of the pair. Without it, "an
// unreferenced emitter warns" is satisfied by a check that warns at every
// project — which would be a false warning at every correct app on the platform,
// the worst outcome for advisory output (AGENTS.md item 10).
func TestWiredEmitterIsSilent(t *testing.T) {
	t.Run("static: loaded by a <script src>", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.Static)
		wantAckKind(t, dir, "")
		// And the reference really is what carries it: move the emitter to a
		// subdirectory, point the tag at the new location, and it must stay
		// silent — so the accept above is not a basename or a fixed-path match.
		if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(dir, blockproto.ReadyAckFilename),
			filepath.Join(dir, "vendor", blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		editFile(t, dir, "index.html", `src="./civitai-host.js"`, `src="./vendor/civitai-host.js"`)
		wantAckKind(t, dir, "")
	})

	t.Run("page-vite: imported by the entry module", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.PageVite)
		wantAckKind(t, dir, "")
	})

	t.Run("page-money: the SDK dependency acks, with no literal anywhere", func(t *testing.T) {
		dir := renderTemplate(t, scaffold.PageMoney)
		// Stated as evidence rather than assumed: the tree really contains no
		// BLOCK_READY, so this template is accepted by the DEPENDENCY branch and
		// nothing else. If that ever stopped being true, the silence below would
		// be green for a reason that has nothing to do with the branch it covers.
		if got := scanForReadyAck(dir); got != ackAbsent {
			t.Fatalf("scanForReadyAck(page-money) = %v, want ackAbsent", got)
		}
		wantAckKind(t, dir, "")
	})
}

// TestHTMLSrcIsAURLNotAModuleSpecifier is the end-to-end guard for the audit's
// root cause: `<script src>` was resolved with JS module-specifier rules.
//
// Every case below is a real `static` scaffold with ONE character changed in
// `index.html`, and every one is ordinary HTML that a browser resolves to
// exactly the same file as the shipped form. Two were measured failing before
// the fix:
//
//	src="app.js"           -> the emitter-not-wired defect went SILENT again
//	src=./civitai-host.js  -> a CORRECT project got --strict rc=1
func TestHTMLSrcIsAURLNotAModuleSpecifier(t *testing.T) {
	// Correct projects: the reference is spelled differently, the file is the
	// same, and the check must stay silent. A warning here is the expensive
	// failure — advisory output that fires at a correct project.
	for _, c := range []struct{ name, old, new string }{
		{"unquoted src", `src="./civitai-host.js"`, `src=./civitai-host.js`},
		{"single-quoted src", `src="./civitai-host.js"`, `src='./civitai-host.js'`},
		{"no leading ./", `src="./civitai-host.js"`, `src="civitai-host.js"`},
		{"root-relative", `src="./civitai-host.js"`, `src="/civitai-host.js"`},
		{"extra attribute after src", `src="./civitai-host.js"`, `src="./civitai-host.js" defer`},
	} {
		t.Run("silent: "+c.name, func(t *testing.T) {
			dir := renderTemplate(t, scaffold.Static)
			wantAckKind(t, dir, "") // positive control on the untouched scaffold
			editFile(t, dir, "index.html", c.old, c.new)
			wantAckKind(t, dir, "")
		})
	}

	// And the defect must still be caught when the SAME spellings are used for
	// the project's own script while the emitter is unreferenced. Without this
	// half, "stays silent" is satisfied by a check that never fires at all.
	for _, c := range []struct{ name, old, new string }{
		{"unquoted src", `src="./app.js"`, `src=./app.js`},
		{"no leading ./", `src="./app.js"`, `src="app.js"`},
	} {
		t.Run("warns: emitter unwired, "+c.name, func(t *testing.T) {
			dir := renderTemplate(t, scaffold.Static)
			editFile(t, dir, "index.html", c.old, c.new)
			editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
			wantAckKind(t, dir, "unwired")
		})
	}

	t.Run("an extensionless src is a 404, not an extension to guess", func(t *testing.T) {
		// 🔴 A browser fetches the literal path. On the no-build `static`
		// template `<script src="./civitai-host">` is a 404, so the emitter is
		// NOT loaded — guessing `.js` accepted the exact "ships a 404" shape the
		// resolver exists to reject, and reported the strong tier's silence.
		// Now the reference resolves to nothing, which is a GAP, so the check
		// drops to the presence tier and (the tree does hold the literal) stays
		// quiet WITHOUT claiming the emitter is wired.
		dir := renderTemplate(t, scaffold.Static)
		editFile(t, dir, "index.html", `src="./civitai-host.js"`, `src="./civitai-host"`)
		wantAckKind(t, dir, "")
		g, loaded := resolveLoadedFiles(dir, nil)
		if g.Complete {
			t.Fatalf("a src pointing at a file that does not exist must make the graph INCOMPLETE, or the "+
				"check reports a confident finding on a model that disagrees with the project\ntrace: %v", g.Trace)
		}
		if loaded {
			t.Fatal("the extensionless src resolved to the emitter — a browser would 404 that URL")
		}
	})
}

// TestAckBelowTheDepthBoundIsAGapNotAFinding pins the third root-cause bug: the
// depth bound truncated the walk WITHOUT clearing Complete, so a correct project
// whose emitter sits below it got the strong tier's warning and `--strict` rc=1.
// Three separate comments claimed an exhausted budget made the graph incomplete;
// none of them was true of this budget.
func TestAckBelowTheDepthBoundIsAGapNotAFinding(t *testing.T) {
	dir := renderTemplate(t, scaffold.Static)
	// Rebuild the entry as a chain deeper than blockproto's default bound, with
	// the emitter genuinely loaded at the bottom. This project WORKS.
	editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
	editFile(t, dir, "index.html", `<script src="./app.js"></script>`, `<script type="module" src="./m0.js"></script>`)
	const depth = blockproto.DefaultEntryMaxDepth + 3
	for i := 0; i < depth; i++ {
		next := fmt.Sprintf("./m%d.js", i+1)
		if i == depth-1 {
			next = "./" + blockproto.ReadyAckFilename
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("m%d.js", i)),
			[]byte("import '"+next+"';\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	g, _ := resolveLoadedFiles(dir, nil)
	if g.Complete {
		t.Fatalf("the walk stopped at the depth bound and still reported a COMPLETE graph — every finding "+
			"built on it is a claim about files we chose not to read\ngaps: %v", g.Gaps)
	}
	// The observable consequence: no strong-tier warning at a project that works.
	wantAckKind(t, dir, "")
}

// ---------------------------------------------------------------------------
// THE DECIDABILITY BOUNDARY, asserted explicitly.
//
// The check verifies reachability only when the entry graph resolves COMPLETELY.
// Anything it cannot follow drops it to the presence tier, whose message says so.
// These cases pin both directions of that switch — a check that changed strength
// SILENTLY between project shapes is exactly how the defect above shipped.
// ---------------------------------------------------------------------------

func TestReadyAckDecidabilityBoundary(t *testing.T) {
	t.Run("a bundler alias makes wiring undecidable, and the emitter is accepted on presence", func(t *testing.T) {
		// 🔴 THE ACCEPTED RESIDUAL, stated as a test rather than left implicit.
		// `import '@/civitai-host.js'` is a real file behind a `resolve.alias`
		// this CLI does not read. The graph is incomplete, so no wiring finding
		// may be made — and because the tree DOES contain the literal, the check
		// goes quiet. A project that wired its emitter through an alias and one
		// that left it orphaned are indistinguishable here, and quiet is the
		// direction that never warns at a correct project.
		dir := renderTemplate(t, scaffold.PageVite)
		editFile(t, dir, "src/main.jsx", `import './civitai-host.js';`, `import '@/civitai-host.js';`)
		wantAckKind(t, dir, "")

		// The negative control: the SAME undecidable shape with no emitter
		// anywhere still warns — "cannot observe" must not become "everything
		// passes", which is the bug this whole change exists to close.
		if err := os.Remove(filepath.Join(dir, "src", blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		wantAckKind(t, dir, "presence-only")
	})

	t.Run("no index.html at the project root is presence-only", func(t *testing.T) {
		// A framework-owned entry point (Next.js, SvelteKit) has no root
		// index.html to resolve. Nothing mentions the ack, so it warns — with the
		// message that discloses it could not check wiring.
		dir := ackProject(t, ackManifest(true), map[string]string{
			"package.json": `{"dependencies": {"next": "^15.0.0"}}`,
			"app/page.tsx": `export default function Page() { return null; }`,
		})
		wantAckKind(t, dir, "presence-only")
	})

	t.Run("a dangling reference is a gap, not a decided absence", func(t *testing.T) {
		// Deleting the emitter from a CURRENT scaffold leaves index.html pointing
		// at a file that is not there. That means this resolver's model of the
		// project disagrees with a project that presumably builds, so it is a gap:
		// the finding drops to the presence tier rather than claiming to have
		// resolved a graph it did not.
		dir := renderTemplate(t, scaffold.Static)
		if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		wantAckKind(t, dir, "presence-only")
	})

	t.Run("a genuinely pre-fix project is decided on reachability", func(t *testing.T) {
		// The shape of an app scaffolded before 4018e2c: no emitter and no
		// reference to one. Every reference resolves, so the graph is complete and
		// the check gets to say the strong thing.
		dir := renderTemplate(t, scaffold.Static)
		editFile(t, dir, "index.html", `<script src="./civitai-host.js"></script>`, "")
		if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
			t.Fatal(err)
		}
		wantAckKind(t, dir, "missing")
	})
}

// TestShippedSDKFreeTemplatesResolveCompletely is the SEAM GUARD for the
// boundary above, and the reason the reachability tier is not quietly dead.
//
// 🔴 If a template edit made the shipped scaffolds' entry graphs unresolvable,
// every real project would silently fall to the presence tier and this whole
// change would be inert — while every test that only asserts "no warning" stayed
// green, because the presence tier is silent at a correct project too. Assert
// the strength, not just the verdict.
func TestShippedSDKFreeTemplatesResolveCompletely(t *testing.T) {
	examined := 0
	for _, tmpl := range []scaffold.Template{scaffold.Static, scaffold.PageVite} {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := renderTemplate(t, tmpl)
			deps, _ := packageDeps(dir)
			g := blockproto.ResolveEntryGraph(dir, blockproto.EntryGraphOptions{
				MaxFiles: maxAckGraphFiles, MaxFileBytes: maxAckFileBytes, Dependencies: deps,
			})
			if !g.Complete {
				t.Fatalf("the rendered %s template's entry graph does not resolve: %v\ntrace:\n  %s\n"+
					"every %s app would therefore be checked on PRESENCE only, and the wiring tier would be "+
					"dead in production while this suite stayed green",
					tmpl, g.Gaps, strings.Join(g.Trace, "\n  "), tmpl)
			}
			rel := tmpl.ReadyAckPath()
			if rel == "" {
				t.Fatalf("%s declares no ReadyAckPath", tmpl)
			}
			if g.FileAt(filepath.Join(dir, filepath.FromSlash(rel))) < 0 {
				t.Fatalf("%s's entry graph does not reach %s; it reached %v", tmpl, rel, g.Trace)
			}
		})
		examined++
	}
	// COUNT POSITIVE CONTROL: a zero is otherwise indistinguishable from an
	// enumeration that returned nothing.
	if examined != 2 {
		t.Fatalf("examined %d SDK-free templates, want 2", examined)
	}
}
