package validate

// readyack.go implements the PAGE-APP READY-ACK advisory.
//
// WHY THIS EXISTS. A `page` app is not revealed by the host until it posts
// `BLOCK_READY` — that handler in `PageBlockHost.tsx` is the only transition
// into the host's `ready` state. An app that never posts it renders perfectly
// everywhere you can look locally and is replaced by a visible failure card in
// the real host once the bounded init retries run out (~37s). That is issue
// #206, and `4018e2c` fixed the `static` and `page-vite` TEMPLATES — but every
// app scaffolded before that commit is still broken, and nothing tells its
// author. This check is the thing that tells them.
//
// 🔴 PRESENCE IS NOT REACHABILITY, AND THE FIRST VERSION OF THIS CHECK ONLY
// MEASURED PRESENCE. Its own remedy told the author to do TWO things — copy
// `civitai-host.js` in, and LOAD it from index.html or the entry module — and it
// verified only the first. Measured on a genuinely pre-fix scaffold: with the
// emitter copied in and never referenced, `civitai app validate --strict`
// printed `✓ … is valid` and exited 0, for an app that was still exactly as
// broken as before. An orphan file containing the literal ANYWHERE in the tree
// passed identically. A green check earned by obeying our own advice is worse
// than the silence it replaced, so the check now RESOLVES THE ENTRY GRAPH
// (`blockproto.ResolveEntryGraph`) wherever it can, and says which of the two
// questions it answered.
//
// 🔴 IT IS A WARNING, NOT AN ERROR, AND THAT IS DELIBERATE.
// The lockfile check next door (lockfile.go) earns hard-error status because the
// platform PROVABLY fails: the build recipe runs `npm ci` and dies. Nothing here
// is provable. This check infers RUNTIME behaviour from STATIC TEXT, and there
// are real, correct projects it cannot read: an ack that arrives through a
// bundled dependency, a framework wrapper, a code-split chunk, a generated file,
// or simply a file extension this scan does not open. Hard-failing on a
// heuristic is the exact false-warning-at-a-correct-project failure the
// dev-tunnel preflight (AGENTS.md item 10) spent four measured corrections
// avoiding — and unlike that one, `--strict` here would turn it into a build
// break in somebody's CI. `Result.Warnings` costs an author nothing to ignore
// and still puts the sentence in front of them.
//
// THE TWO TIERS, AND WHY THE MESSAGE NAMES WHICH ONE RAN.
//
//	REACHABILITY (strong). The entry graph resolved COMPLETELY: index.html is at
//	the project root and every `<script src>` and import below it was accounted
//	for. Then "nothing the browser loads posts BLOCK_READY" is a real finding,
//	and an emitter sitting unreferenced in the tree is reported as the orphan it
//	is.
//
//	PRESENCE ONLY (weak). The graph is INCOMPLETE — no root index.html, a
//	bundler alias, a specifier pointing at a file that is not there, a budget
//	exhausted. Then wiring is undecidable and the check falls back to the
//	whole-tree scan it always did. 🔴 Its message SAYS SO, in the same breath as
//	the remedy that tells the author to wire the emitter up. A check that
//	silently changes strength between project shapes is how the false pass above
//	happened in the first place.
//
//	🔴 AND IT NAMES THE REASON RATHER THAN GUESSING AT IT (issue #258). The
//	resolver records why it stopped, per reference, in `EntryGraph.Gaps`; this
//	check used to discard that and offer a fixed list of plausible causes
//	instead, so the canonical #206 project — a `static` scaffold whose
//	`civitai-host.js` was deleted, where index.html plainly references a file
//	that is not there — was told to go looking for a bundler alias. The reasons
//	are now appended verbatim (capped, with the overflow counted). See
//	presenceOnlyAdvice.
//
// 🔴 "CANNOT OBSERVE" MUST NOT SILENTLY BECOME "EVERYTHING PASSES" — but it also
// must not manufacture advice. The split: an INCOMPLETE graph never produces a
// wiring finding (that is the AGENTS.md item 10 doctrine), yet the presence
// finding still fires when nothing in the tree mentions the message at all, and
// discloses its own weakness when it does. The residual, stated rather than
// hidden: a project whose entry graph we cannot resolve AND which contains the
// literal somewhere gets silence, exactly as before. That is a false negative,
// and false negatives are the cheap direction here.
//
// EVIDENCE ORDER — THE DEPENDENCY IS CHECKED FIRST, AND THAT IS LOAD-BEARING.
// For a project depending on `@civitai/blocks-react`, the ack comes from the
// SDK's `IframeTransport` and the literal `BLOCK_READY` NEVER APPEARS in `src/`.
// A source scan alone therefore warns at every correct page-money app on the
// platform, which is the worst possible outcome for advisory output: it teaches
// authors that this warning is noise. So package.json is read first and a
// dependency that ACKS ends the check.
//
// 🔴 "A dependency that acks" is an EXACT SET (blockproto.PackageAcksReady), not
// the `@civitai/` scope. Four of the six published first-party packages do not
// ack at all — `@civitai/theme` and `@civitai/components` are plain CSS, and are
// exactly what a hand-written no-build page app installs — so a scope test goes
// SILENT on a genuinely broken app. Measured per package in blockproto, and
// reproduced live on a `static` scaffold with the emitter deleted. Do not
// re-widen this to a prefix.
//
// WHAT IT STILL DOES NOT PROVE. That the ack FIRES. No static check can — an
// emitter with an inverted `event.source` guard satisfies every assertion here
// (that is what the scaffold's Guard B exists for, see AGENTS.md item 11). At
// its strongest this check answers: does a file the browser actually loads so
// much as MENTION the message, outside a comment.
//
// SCOPE OF THE PRESENCE SCAN — IT ONLY FIRES WHERE IT CAN OBSERVE, AND
// "OBSERVE" MEANS THE WHOLE TREE. Mirroring lockfile.go's early-out shape: no
// `page` surface, no check; unreadable package.json, no check. Beyond that, the
// scan concludes "this app never acks" ONLY if it read every source file it
// could reach and found nothing. Anything that leaves a gap — an unresolvable
// symlink, a read error, a file over the size cap, the file-count budget —
// reports UNOBSERVABLE and stays silent, and that verdict gates BOTH tiers: a
// tree we could not read is not a tree we can draw a wiring conclusion about
// either.
//
// 🔴 SKIPPING IS A COST DECISION, NEVER A CORRECTNESS ONE, because the tree scan
// is a PRESENCE check: scanning an extra directory can only ever ADD ack
// evidence, while skipping one can only ever CREATE a false warning. That
// asymmetry is why the manifest's `outputDir` is NOT skipped (it was, and it was
// wrong — a perfectly valid `"outputDir": "src"` on a page-vite app skipped the
// directory holding the emitter and warned at a correct project;
// `"outputDir": "."` skipped the entire tree). What remains is a fixed list of
// names that are never source — dependencies, VCS metadata, and the conventional
// build directories — chosen for cost. The residual trade is accepted and
// stated: a stale committed build under a NON-conventional output directory can
// retain an ack the source has lost, silencing a genuinely broken app.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/blockproto"
)

// readyAckType is the message the page host waits for. It is the same literal
// blockproto's emitter posts; the emitter is the authority for the SHAPE, this
// is only the token we look for.
const readyAckType = "BLOCK_READY"

// readyAckSourceExts are the extensions a hand-written emitter can live in.
// Deliberately CODE ONLY — `.md` is excluded because a README that explains the
// handshake is not an implementation of it, and both shipped scaffold READMEs
// describe `BLOCK_READY` at length. Including docs would silence the check for
// exactly the apps it exists to find.
//
// It gates BOTH tiers: a `.css` a module imports is in the entry graph (the
// browser really loads it) but cannot implement a handshake, so it is not read
// as ack evidence either.
var readyAckSourceExts = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	".html": true, ".htm": true,
	".vue": true, ".svelte": true, ".astro": true,
}

// readyAckSkipDirs are never descended into by the TREE scan. These names are
// never source: dependencies, VCS metadata, and the conventional build
// directories. The list is a COST decision (see the header) — every entry can
// only cost a false warning, so it is kept short and conventional rather than
// expanded to every directory that might be large. `vendor` and `public` are
// deliberately ABSENT: both routinely hold hand-written source (a vendored
// emitter, Vite's static `public/`), and cost is bounded by the caps below
// instead.
//
// It does NOT apply to the entry graph: a file index.html demonstrably loads is
// loaded whatever directory it sits in.
var readyAckSkipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	"out": true, "coverage": true, ".vite": true, ".next": true,
	".svelte-kit": true, ".output": true,
}

// Scan caps. `validate` used to be a manifest-only read; walking a project tree
// is new cost, and an app that commits a large bundle or a huge vendored tree
// must not turn it into a memory event (measured before these existed: 20,008
// files with one 88 MB .js peaked at 316 MB RSS).
//
// Hitting either cap makes the scan UNOBSERVABLE, not "absent" — we stopped
// reading, so we do not know. That is deliberately the direction that stays
// silent.
const (
	maxAckScanFiles = 5000
	maxAckFileBytes = 2 << 20 // 2 MiB
)

// maxAckGraphFiles bounds the entry-graph walk. It is far smaller than
// maxAckScanFiles because an entry graph is not a tree walk: a project whose
// index.html reaches 200 modules through relative imports alone is not a shape
// this resolver models honestly, and exhausting the budget makes the graph
// incomplete (i.e. drops to the presence tier) rather than producing a finding.
const maxAckGraphFiles = 200

// readyAckChecks returns the ready-ack advisory for the project in dir, or nil.
//
// It runs in the projectState branch of validateDir (beside lockfileChecks) and
// NOT in warningChecks: warningChecks is reached under ManifestOnly, which
// `civitai app init` uses to self-check the template it just wrote, and a check
// that reads `src/` has no business running there.
//
// FIELD: FieldProject on every tier. The manifest's `page` is what makes the
// check APPLY, but the finding is about what the browser loads — the remedy is
// two edits in source files and the manifest is not one of them. Reporting it
// against `page` would send a consumer to the one field that is correct.
func readyAckChecks(dir string, generic any) []Finding {
	if !declaresPage(generic) {
		return nil
	}
	// The dependency is checked FIRST — see the file header. `unknown` means the
	// package.json is there but unreadable, i.e. we cannot answer the question
	// that decides whether any source reading is meaningful.
	deps, state := packageDeps(dir)
	switch state {
	case sdkPresent, sdkUnknown:
		return nil
	}

	// The whole-tree presence scan runs FIRST and gates everything: if we could
	// not read the tree, we have no business drawing a conclusion from a subset
	// of it either.
	tree := scanForReadyAck(dir)
	if tree == ackUnobservable {
		return nil
	}

	graph, loadedPostsAck := resolveLoadedFiles(dir, deps)
	if graph.Complete {
		if loadedPostsAck {
			return nil
		}
		// Reachability tier: nothing the browser loads posts the message.
		if tree == ackFound {
			return []Finding{newFinding(FieldProject, readyAckAdviceUnwired)}
		}
		return []Finding{newFinding(FieldProject, readyAckAdviceMissing)}
	}

	// Presence tier: wiring is undecidable for this project shape. The
	// resolver's OWN reasons ride along — see presenceOnlyAdvice.
	if tree == ackFound {
		return nil
	}
	return []Finding{newFinding(FieldProject, presenceOnlyAdvice(graph.Gaps))}
}

// resolveLoadedFiles walks the entry graph and reports whether any file the
// BROWSER LOADS mentions the message outside a comment.
//
// 🔴 The answer is accumulated through `Inspect`, not read off a retained field,
// so this costs O(one file) rather than O(graph) — the same discipline the tree
// scan above follows. Getting there took TWO fixes, and the intermediate state
// is why this comment now carries numbers instead of an adjective: storing every
// file's stripped contents on EntryFile peaked 421-439 MB on a 200-module graph
// entirely INSIDE these budgets, and dropping that field still left 558-628 MB
// because an un-cloned import specifier pins its whole file's backing array.
// Both fixed: 31.4-33.3 MB, against 26.4-27.8 MB at e800129 before the graph
// existed. See blockproto's Inspect doc for the fixture — a tree padded with
// COMMENTS, or whose leaves carry no imports, does not exercise either hazard.
// `app submit` pays this too.
func resolveLoadedFiles(dir string, deps map[string]bool) (*blockproto.EntryGraph, bool) {
	found := false
	g := blockproto.ResolveEntryGraph(dir, blockproto.EntryGraphOptions{
		MaxFiles:     maxAckGraphFiles,
		MaxFileBytes: maxAckFileBytes,
		Dependencies: deps,
		Inspect: func(f blockproto.EntryFile, code string) {
			// readyAckSourceExts gates BOTH tiers: a `.css` an entry module
			// imports really is loaded by the browser and really cannot
			// implement a handshake, so it is in the graph but is not evidence.
			if !readyAckSourceExts[strings.ToLower(filepath.Ext(f.Rel))] {
				return
			}
			if strings.Contains(code, readyAckType) {
				found = true
			}
		},
	})
	return g, found
}

// declaresPage reports whether the manifest declares a non-null `page` surface.
// `"page": null` is not a page app; an absent key is not either.
func declaresPage(generic any) bool {
	m, ok := generic.(map[string]any)
	if !ok {
		return false
	}
	v, present := m["page"]
	return present && v != nil
}

// sdkState is the three-valued answer to "does this project depend on a
// first-party package". The third value is the point: "we could not tell" must
// not be collapsed into "no", which is what would warn at a project whose
// package.json this CLI simply failed to parse.
type sdkState int

const (
	sdkAbsent sdkState = iota
	sdkPresent
	sdkUnknown
)

// packageDeps returns the project's declared dependency names and whether one of
// them ACKS.
//
// The names are needed by the entry-graph resolver: a bare specifier naming a
// declared dependency is accounted for (it is a package, not a file in this
// project), while one that names nothing declared is a bundler alias we cannot
// follow — which must make the graph incomplete rather than look like a dead
// end. Both answers come from ONE read of package.json.
func packageDeps(dir string) (map[string]bool, sdkState) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		if os.IsNotExist(err) {
			// No package.json at all: a static, no-build app. It installs
			// nothing, so it cannot be acking through a dependency — reading the
			// source is the whole answer for it.
			return nil, sdkAbsent
		}
		return nil, sdkUnknown
	}
	// json.RawMessage values, not strings: a package.json with an unusual
	// (non-string) version entry must not make the whole decode fail and warn.
	var pkg struct {
		Dependencies     map[string]json.RawMessage `json:"dependencies"`
		DevDependencies  map[string]json.RawMessage `json:"devDependencies"`
		PeerDependencies map[string]json.RawMessage `json:"peerDependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, sdkUnknown
	}
	names := map[string]bool{}
	state := sdkAbsent
	for _, set := range []map[string]json.RawMessage{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies} {
		for name := range set {
			names[name] = true
			if blockproto.PackageAcksReady(name) {
				state = sdkPresent
			}
		}
	}
	return names, state
}

// sdkDependency is packageDeps' verdict alone, for callers (and tests) that only
// need to know whether a dependency acks.
func sdkDependency(dir string) sdkState {
	_, state := packageDeps(dir)
	return state
}

// ackScanResult is the THREE-valued outcome of the source scan. Two values
// would collapse "we did not look" into "it is not there", which is the whole
// class of bug this check has to avoid.
type ackScanResult int

const (
	// ackFound: a source file mentions the message outside a comment.
	ackFound ackScanResult = iota
	// ackAbsent: every reachable source file was read, and none mentions it.
	ackAbsent
	// ackUnobservable: the scan left a gap — an unresolvable symlink, a read
	// error, a file over the size cap, the file budget, or simply no source
	// files at all.
	ackUnobservable
)

// scanForReadyAck walks dir looking for the ready-ack message in source.
//
// 🔴 READING NOTHING IS NOT THE SAME AS FINDING NOTHING. A zero-hit scan over a
// zero-file tree is indistinguishable from a scanner wired to nothing — a broken
// extension table, an over-eager skip list, or `validate` pointed at a directory
// that holds a manifest and no checkout (the shape of every manifest-only
// fixture in this package). So "no files read" is ackUnobservable, not ackAbsent.
//
// 🔴 The comment strip is load-bearing, not tidiness. Both shipped SDK-free
// templates carry a source COMMENT naming `BLOCK_READY`
// (`static/app.js`, `page-vite/src/App.jsx`: "The ONE message a page app must
// send is `BLOCK_READY`"), and those comments SURVIVE deleting `civitai-host.js`.
// Without stripping, the exact repair this check exists to demand — restore the
// emitter — would already look satisfied, and the check would be inert on the
// one population it was written for. Measured both ways. The stripper itself
// lives in blockproto, because the entry-graph resolver and the scaffold's
// Guard A ask the identical question.
//
// 🔴 SYMLINKED DIRECTORIES ARE FOLLOWED. filepath.WalkDir does not follow them,
// and it does not report them as directories either, so a monorepo whose `src`
// is a symlink into a shared package had its ENTIRE source tree skipped and
// warned at a correct project — reproduced live. Following is also the safe
// direction for a presence check (more files can only add evidence). Cycles are
// bounded by a visited set keyed on the RESOLVED path, and total work by the
// caps above.
func scanForReadyAck(dir string) ackScanResult {
	s := &ackScanner{visited: map[string]bool{}}
	s.walk(dir)
	switch {
	case s.found:
		return ackFound
	case s.partial || s.files == 0:
		return ackUnobservable
	default:
		return ackAbsent
	}
}

type ackScanner struct {
	visited map[string]bool
	files   int
	found   bool
	// partial records that something in the tree could not be read. It is
	// sticky: once set, the answer can never be ackAbsent.
	partial bool
}

// walk descends dir, stopping at the first ack and at the first thing it cannot
// read.
//
// 🔴 THE TWO `s.found || s.partial` SHORT-CIRCUITS BELOW ARE A MASKING PAIR, NOT
// A DUPLICATED GUARD — ISSUE #302, AND THE ASYMMETRY IS THE POINT. Measured, not
// reasoned:
//   - THIS one (on entry) has a condition that is never true as the code stands.
//     walk is entered only from scanForReadyAck (both flags false) and from the
//     recursion below, which sits after the per-entry guard and after an os.Stat
//     that returns on error. A probe panicking in its body fired 0 times across
//     this package's 332 test leaves — and 1 time the moment the per-entry guard
//     was removed. So deleting it ALONE is an equivalent mutation, and it is not
//     dead code either: it is the line that becomes load-bearing when the other
//     one breaks.
//   - The PER-ENTRY one is load-bearing today: delete it alone and a scan that
//     already set `partial` walks on to a sibling FILE and reports ackFound.
//   - Delete BOTH and it also walks into a sibling DIRECTORY.
//
// Either flip reports an ack in a tree we did not finish reading, which is the
// direction AGENTS.md item 18 refuses: reading only PART is not finding nothing.
// All three deletions survived the whole repo (0 `--- FAIL` over 19 packages)
// until TestPartialScanNeverBecomesFound, which carries one fixture row per
// guard because a single-mutation sweep cannot see the pair by construction.
func (s *ackScanner) walk(dir string) {
	if s.found || s.partial {
		return
	}
	// Resolve before recording: two paths that reach the same directory (a
	// symlink and its target) must count once, or a self-referential link
	// recurses forever.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		s.partial = true
		return
	}
	if s.visited[real] {
		return
	}
	s.visited[real] = true

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.partial = true
		return
	}
	for _, e := range entries {
		if s.found || s.partial {
			return
		}
		path := filepath.Join(dir, e.Name())
		// os.Stat FOLLOWS symlinks — that is the point: a symlinked source
		// directory must be walked as a directory. A dangling link fails here
		// and correctly makes the scan partial.
		info, err := os.Stat(path)
		if err != nil {
			s.partial = true
			return
		}
		if info.IsDir() {
			// Note the skip list is applied to ENTRIES only, never to the root
			// we were handed — no skip rule can remove the whole tree.
			if readyAckSkipDirs[e.Name()] {
				continue
			}
			s.walk(path)
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !readyAckSourceExts[ext] {
			continue
		}
		if info.Size() > maxAckFileBytes || s.files >= maxAckScanFiles {
			s.partial = true
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			s.partial = true
			return
		}
		s.files++
		if strings.Contains(blockproto.StripCommentsForExt(string(data), ext), readyAckType) {
			s.found = true
			return
		}
	}
}

// ---------------------------------------------------------------------------
// The advisories.
//
// Three messages, built from shared fragments so the remedy cannot drift apart
// between them, and so the ONE thing each has to add — what this run actually
// verified — is the only thing that differs.
// ---------------------------------------------------------------------------

// readyAckWhy is the consequence. Every advisory carries it: an author who does
// not know what breaks has no reason to act.
const readyAckWhy = "the host will not reveal a page app until it acks the host's BLOCK_INIT, so an app that " +
	"never sends it renders fine locally and is replaced by a failure card in the real host once the bounded " +
	"init retries run out (issue #206)"

// readyAckRemedy names the concrete next command, per the house rule that a
// message must tell the author what to run.
//
// 🔴 IT SPELLS OUT BOTH STEPS AND SAYS THE FIRST ONE ALONE DOES NOTHING. The
// earlier wording ("copy its civitai-host.js into this project, loading it from
// index.html or from the entry module") read as one action with a parenthetical,
// and a blind dogfooder did the copy, skipped the load, and got a green check.
const readyAckRemedy = "run `civitai app init` into a scratch directory and copy its `" +
	blockproto.ReadyAckFilename + "` into this project — then LOAD it, which is a SECOND edit in a " +
	"DIFFERENT file: add `<script src=\"./" + blockproto.ReadyAckFilename + "\"></script>` to index.html " +
	"above your own script tags, or `import './" + blockproto.ReadyAckFilename + "';` as the first line of " +
	"the entry module index.html loads. Copying the file in is not enough on its own — a browser never " +
	"fetches a file nothing references. The alternative is to adopt `@civitai/blocks-react`, whose iframe " +
	"transport acks for you (never both: whichever answers the first BLOCK_INIT cancels the host's retry). " +
	"Note `@civitai/app-sdk` alone does NOT ack — it is the server-side SDK and no runtime code in it posts " +
	readyAckType

// readyAckTail states the tier and the limit. Every advisory ends with it.
const readyAckTail = "no static check can prove the ack FIRES — only the real host can; this is advisory only " +
	"and never fails `civitai app validate` unless you pass --strict"

// readyAckAdviceUnwired is the REACHABILITY finding for a project that has an
// emitter nothing loads. This is the exact false pass this check was rewritten
// to close.
var readyAckAdviceUnwired = "the manifest declares a \"page\" surface, and this project's source DOES contain " +
	readyAckType + " — but nothing index.html loads reaches it: resolving index.html and every script and " +
	"module it loads found no " + readyAckType + " among them, so whatever posts it is an orphan the browser " +
	"never fetches — " + readyAckWhy + ". Wire it up: " + readyAckRemedy + ". Caveat: " + readyAckTail

// readyAckAdviceMissing is the REACHABILITY finding for a project with no
// emitter at all — the original #206 shape, now stated with the stronger
// evidence.
var readyAckAdviceMissing = "the manifest declares a \"page\" surface but nothing in this project's source " +
	"posts " + readyAckType + ", and nothing index.html loads posts it either — " + readyAckWhy + ". Apps " +
	"scaffolded before that fix ship no emitter: " + readyAckRemedy + ". Caveat: " + readyAckTail

// readyAckAdvicePresenceOnlyHead is the PRESENCE finding up to the point where
// the resolver's own reasons are spliced in. It discloses its own weakness: it
// fires where the entry graph could not be resolved, so it is the one message
// that must not let an author read "valid" as "wired".
//
// 🔴 IT USED TO GUESS AT WHY IT HAD FALLEN BACK, AND THE GUESS WAS WRONG IN THE
// COMMONEST CASE. This clause read "(there is no index.html at the project root,
// or it holds a reference this CLI cannot follow — a bundler alias, a generated
// file, an off-project URL)". In the canonical #206 shape — a `static` scaffold
// whose `civitai-host.js` has been deleted — NONE of those is the reason: the
// reason is that `<script src="./civitai-host.js">` points at a file that is not
// there, and the resolver had already recorded exactly that in `EntryGraph.Gaps`
// before this constant threw it away. So a five-file no-build app sent its
// author hunting for a bundler alias that cannot exist in it. Issue #258. The
// guess is gone; the real reasons are appended by presenceOnlyAdvice, and this
// clause now only states what was NOT established.
//
// The DIAGNOSIS and the disclosure come first, then the gap report, then the
// remedy — deliberately, and not the order this message grew in. The remedy is
// the longest fragment by far (it is shared with both reachability tiers), so
// splicing the reasons after it put the one project-specific sentence in the
// message roughly two thirds of the way down a wall of generic advice. The
// concrete cause now sits immediately after the sentence that admits the check
// could not resolve the entry graph, which is the sentence it explains.
var readyAckAdvicePresenceOnlyHead = "the manifest declares a \"page\" surface but nothing in this project's " +
	"source posts " + readyAckType + " — " + readyAckWhy + ". 🔴 What this run did NOT check: it could not " +
	"resolve the files your index.html loads, so it checked only whether SOME file in the project mentions " +
	readyAckType + "; it did NOT check that the file is loaded."

// readyAckAdvicePresenceOnlyTail is the rest of the presence finding, after the
// gap report.
var readyAckAdvicePresenceOnlyTail = " Apps scaffolded before that fix ship no emitter: " + readyAckRemedy +
	". Adding the emitter without referencing it will silence this warning " +
	"and leave the app just as broken — verify the reference yourself. It is also wrong about a project " +
	"whose ack arrives from a bundled dependency or a file type it does not read. Caveat: " + readyAckTail

// readyAckAdvicePresenceOnly is the presence finding with NO gap report — the
// shape emitted only if an incomplete graph somehow recorded no reason at all
// (today it cannot: `EntryGraph.Complete` is cleared only by `gap()`, which
// appends). It is also the LEDGER value below and the prose the message tests
// assert against, so the wording every tier shares stays pinned in one place.
var readyAckAdvicePresenceOnly = readyAckAdvicePresenceOnlyHead + readyAckAdvicePresenceOnlyTail

// readyAckGapCap bounds how many resolver reasons the presence advisory renders.
// A large project can produce many, and a wall of them buries the remedy that
// follows.
//
// 🔴 THE VALUE IS PART OF THE CONTRACT, AND IT WAS UNPINNED. Every assertion in
// TestGapReportUnitCap is written RELATIVE to this constant, so `= 99` reddened
// **0** subtests — the wall of gaps the cap exists to prevent came straight back
// under a green suite. `TestGapReportCapValue` now pins the literal.
//
// 🔴 THE OVERFLOW IS COUNTED OUT LOUD. A silently truncated list reads as "that
// was all of them", which is the same class of lie as the guess this report
// replaced: the author fixes three references, re-runs, and is told about three
// more they were never shown. `readyAckGapReport` says how many it withheld.
const readyAckGapCap = 3

// readyAckGapLead introduces the resolver's own reasons when ALL of them are
// shown. Only then can the message claim the cause is among them.
const readyAckGapLead = " Here is what it could not follow, in this project's own terms — one of these is " +
	"usually the actual bug: "

// readyAckGapLeadTruncated is the lead when the cap withheld some.
//
// 🔴 THE UNCONDITIONAL LEAD WAS THIS PR'S OWN THESIS FAILING IN A NEW SHAPE.
// Measured on an index.html carrying three CDN `<script src>` tags above a
// dangling `./civitai-host.js`: at three shown of four, the report listed the
// three off-project URLs and withheld the dangling reference — the actual bug —
// under a lead-in asserting "one of these is usually the actual bug". Order is
// deterministic, so that is a stable wrong emphasis, not a flake. This PR exists
// because the message pointed away from the cause; a cap that recreates it is
// the same defect with a different mechanism.
//
// TWO fixes, because either alone is insufficient. `blockproto.rankGaps` puts
// the likely causes first, so the withheld ones are now the LEAST likely — that
// is what makes any claim about this list defensible at all. And this lead stops
// claiming the cause is present, because ranking is a heuristic and the cap can
// still withhold the one that mattered.
const readyAckGapLeadTruncated = " Here is what it could not follow, in this project's own terms, most-likely " +
	"first — this list is TRUNCATED, so if none of these is your bug, fix them and re-run to see the rest: "

// presenceOnlyAdvice is the presence-tier message for a graph that failed to
// resolve for the reasons in gaps.
//
// 🔴 THE REASONS COME FROM THE RESOLVER, NEVER FROM A GUESS AT THE PRINTER —
// which is the same discipline AGENTS.md item 23 imposes on a finding's `Field`,
// applied to its explanation. `EntryGraph.Gaps` already names the referencing
// file, the specifier and the missing target for EVERY gap kind (a dangling
// reference, a bare specifier, an off-project URL, an unreadable file, an
// exhausted file budget, a truncated depth), so surfacing them generally fixes
// all six at once rather than special-casing the one issue #258 reported.
func presenceOnlyAdvice(gaps []string) string {
	return readyAckAdvicePresenceOnlyHead + readyAckGapReport(gaps) + readyAckAdvicePresenceOnlyTail
}

// readyAckGapReport renders gaps as one sentence, or "" when there are none.
//
// 🔴 THE RESULT IS ONE LINE. A `Finding.Message` is a `--json` string field and
// the human output wraps at the PRINTER (internal/cmd), per the inverse of
// AGENTS.md item 23: the field comes from the producer, the layout does not. A
// gap interpolates `%v` of an OS error, which is not guaranteed newline-free, so
// each one is collapsed rather than trusted.
func readyAckGapReport(gaps []string) string {
	if len(gaps) == 0 {
		return ""
	}
	shown, extra := gaps, 0
	if len(shown) > readyAckGapCap {
		extra = len(shown) - readyAckGapCap
		shown = shown[:readyAckGapCap]
	}
	var b strings.Builder
	if extra > 0 {
		b.WriteString(readyAckGapLeadTruncated)
	} else {
		b.WriteString(readyAckGapLead)
	}
	for i, g := range shown {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "(%d) %s", i+1, strings.Join(strings.Fields(g), " "))
	}
	if extra > 0 {
		fmt.Fprintf(&b, "; and %d more this message does not list", extra)
	}
	b.WriteString(".")
	return b.String()
}

// readyAckAdvisories is every FIXED advisory string this check can emit.
// Callers and tests match against the SET rather than one variable, so a new
// tier cannot be introduced without the tests that classify them noticing.
//
// 🔴 IT IS A LEDGER OF BASES, NOT OF LITERAL OUTPUTS, AND SAYING SO IS THE POINT.
// The presence tier's real message is `presenceOnlyAdvice(gaps)` — this base with
// the resolver's reasons spliced between `…PresenceOnlyHead` and
// `…PresenceOnlyTail` — so an equality test against this slice classifies the
// two reachability tiers and MISSES every real presence-tier finding. Anything
// that must recognise the emitted message has to bracket it with those two
// halves instead (see readyAckKind in the tests). Describing this slice as
// "every string this check can emit" would be false, and the falsity is silent:
// a matcher built on it simply stops seeing one tier.
var readyAckAdvisories = []string{
	readyAckAdviceUnwired,
	readyAckAdviceMissing,
	readyAckAdvicePresenceOnly,
}
