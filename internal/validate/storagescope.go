// storagescope.go implements the STORAGE-SCOPE advisory.
//
// THE MOTIVATING FAILURE (2026-10-01). An app built from the documented
// onboarding prompt saved every character, stack and sheet through
// `useAppStorage()` and declared only `ai:write:budgeted` in its manifest. It
// passed 198 unit tests, the dev harness, `civitai app validate` and a full
// submit — then every save failed in production:
//
//	"message": "storage set requires the apps:storage:write scope",
//	"data": { "code": "FORBIDDEN", "httpStatus": 403, "path": "apps.storage.set" }
//
// which the viewer saw as "Saving failed for an unknown reason… try again".
//
// FOUR SURFACES SAID GREEN AND NONE COULD HAVE SAID OTHERWISE. The onboarding
// prompt never mentions scopes (it is a setup document). The `AGENTS.md` written
// into every project names `ai:write:budgeted` explicitly and `apps:storage:*`
// not once. The scaffold's own manifest declares exactly one scope — the money
// one — so the nearest example demonstrates the mechanism for generation and for
// nothing else. And `validate` read the manifest alone, so it could not relate
// "this app calls storage" to "this manifest declares no storage scope".
//
// This check closes the fourth. It is the only one of the four that can catch an
// app with no tests at all.
//
// ---
//
// WHAT IT ASKS, AND WHY THAT PRECISE QUESTION.
//
// Not "does every storage call have a matching scope" — that would need real
// call-graph analysis and would false-warn at a read-only app that correctly
// declared only `apps:storage:read`. It asks the narrower, unambiguous question:
//
//	does this source use a storage store while the manifest declares NO scope
//	for that store at all?
//
// A manifest with ONE storage scope is the work of someone who knows scopes
// exist and chose; this check does not second-guess which they picked. A manifest
// with NONE is the defect above. The two stores (per-viewer and shared) are asked
// independently, because declaring one says nothing about the other.
//
// ADVISORY, NOT FATAL, for the same reason readyack is: it infers runtime
// behaviour from static text, and a file that merely imports a hook it never
// calls is a false positive this must not fail a build over. `--strict` promotes
// it for anyone who wants the stronger contract. The hard stop for this defect
// now lives in the dev host, which refuses an undeclared storage op outright, so
// an app with tests fails them before reaching here.
//
// FIELD: `scopes`, NOT FieldProject — and this is the deliberate opposite of
// readyack's choice. readyack reports FieldProject because its remedy is two
// edits in source files and the manifest is not one of them. Here the source is
// CORRECT (using storage is the point) and the manifest is what is wrong, so
// `scopes` is the one field a consumer should be sent to.
package validate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/blockproto"
)

// The hook names that prove a store is used.
//
// 🔴 A CURATED SET, NOT A PREFIX MATCH. `useAppStorage` / `useSharedStorage` are
// the hook names this check looks for. ⚠️ They are NOT the only way a block
// reaches either store, and this comment used to claim they were: the REST routes
// under `/api/v1/blocks/shared-storage/` and `/api/v1/blocks/app-storage/` reach
// the same stores, and for SHARED storage that REST path is the one the platform
// is consolidating on (so `internal/antipattern` no longer refuses it — the
// `shared-storage-rest` rule was removed).
//
// That is a known LIMITATION of this check, not a reason to widen it: an app that
// uses only the REST path declares the same scopes and needs the same advisory,
// but gets none, because nothing in its source names a hook. The scope
// requirement itself is unaffected and correct either way — the server gates
// these stores by scope PRESENCE regardless of transport, so
// `apps:storage:shared:read`/`write` are needed for REST exactly as for the
// bridge. Matching something broader (`storage`, `apps.storage`) would fire on
// prose, on an unrelated local named `storage`, and on any app that imports the
// SDK at all, since the transport internals mention the message names.
//
// That last one is not hypothetical: scanning BUNDLES rather than source once
// produced three false "users" of `useWildcardPack()` because `GET_WILDCARD_PACK`
// sits in blocks-react's always-included transport internals. This check reads
// SOURCE only, for exactly that reason.
const (
	appStorageHook    = "useAppStorage"
	sharedStorageHook = "useSharedStorage"
)

// The scope prefixes that satisfy each store.
//
// Mirrors `BLOCK_SCOPES` in `@civitai/app-sdk`, which states the server's rule:
// the storage scopes "have no OAuth bit … the server gates them by presence in
// the block's approved scope set, not a bitmask". Presence is the whole test.
//
// 🔴 ORDER MATTERS IN THE CHECK, NOT HERE: `apps:storage:shared:read` has
// `apps:storage:` as a prefix, so a naive prefix test would read a shared-only
// manifest as having declared per-viewer storage. See declaresStore.
const (
	appStorageScopePrefix    = "apps:storage:"
	sharedStorageScopePrefix = "apps:storage:shared:"
)

// storageScopeChecks returns the storage-scope advisories for the project in dir.
//
// It runs in the projectState branch of validateDir (beside readyAckChecks) and
// NOT in warningChecks: warningChecks is reached under ManifestOnly, which
// `civitai app init` uses to self-check the template it just wrote, and a check
// that reads `src/` has no business running there.
func storageScopeChecks(dir string, generic any) []Finding {
	declared := declaredScopeStrings(generic)

	// Ask the cheap question first: a manifest that declares BOTH stores can
	// never produce a finding, so the tree walk is pure cost.
	needApp := !declaresStore(declared, appStorageScopePrefix)
	needShared := !declaresStore(declared, sharedStorageScopePrefix)
	if !needApp && !needShared {
		return nil
	}

	scan := scanForStorageUse(dir)
	if scan.partial {
		// Reading only PART of a tree is not finding nothing. Same posture as
		// readyack's caps: an unobservable scan stays silent rather than
		// reporting against a project we did not finish reading.
		return nil
	}

	var out []Finding
	if needApp && scan.usesAppStorage {
		out = append(out, newFinding("scopes", storageScopeAdvice(
			appStorageHook, "apps:storage:read", "apps:storage:write",
		)))
	}
	if needShared && scan.usesSharedStorage {
		out = append(out, newFinding("scopes", storageScopeAdvice(
			sharedStorageHook, "apps:storage:shared:read", "apps:storage:shared:write",
		)))
	}
	return out
}

// storageScopeAdvice is the whole message, naming the hook found, both scopes,
// and what the failure looks like if it ships. It names the FAILURE because the
// defect is invisible until production: a developer who has only ever run the
// mock host has no reason to connect "saving failed" to a manifest field.
func storageScopeAdvice(hook, readScope, writeScope string) string {
	return "this project's source calls `" + hook + "()` but the manifest declares no " +
		"storage scope — add the ones it uses to `scopes` (`" + readScope + "` to read, `" +
		writeScope + "` to write) each with a `scopeJustifications` entry. The server gates " +
		"storage by PRESENCE in the approved scope set, so without them every call fails in " +
		"production with a 403 (`storage set requires the " + writeScope + " scope`) while " +
		"working locally — and an authorization failure is deliberately unclassified by the " +
		"SDK, so a block usually surfaces it as an unexplained \"try again\". ADVISORY: this " +
		"reads source statically, so a hook imported but never called is a false positive."
}

// declaresStore reports whether declared contains a scope for the store named by
// prefix.
//
// 🔴 THE SHARED PREFIX IS A PREFIX OF THE PER-VIEWER ONE, so a plain
// strings.HasPrefix for `apps:storage:` matches `apps:storage:shared:read` too.
// Left alone, a shared-only manifest would read as having declared per-viewer
// storage and the per-viewer finding would be silently suppressed — a false
// NEGATIVE, in the one direction this check exists to remove. The per-viewer
// question therefore excludes the shared namespace explicitly.
func declaresStore(declared []string, prefix string) bool {
	for _, s := range declared {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		if prefix == appStorageScopePrefix && strings.HasPrefix(s, sharedStorageScopePrefix) {
			continue // a shared scope does not satisfy the per-viewer store
		}
		return true
	}
	return false
}

// declaredScopeStrings pulls `scopes` out of the decoded manifest, tolerating
// every shape a malformed manifest can present. A missing, wrong-typed or
// partially-wrong `scopes` yields the strings it can and ignores the rest: the
// schema layer is what reports a malformed `scopes`, and this check must not
// report a second, confusing finding about the same defect.
func declaredScopeStrings(generic any) []string {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := m["scopes"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// storageScanResult is what the tree walk learned. `partial` is sticky and
// dominates: it means we stopped reading, so neither `uses*` field is a claim.
type storageScanResult struct {
	usesAppStorage    bool
	usesSharedStorage bool
	partial           bool
}

// Scan caps, deliberately the same values readyack uses — the hazard is
// identical (a project that commits a large bundle or a vendored tree must not
// turn `validate` into a memory event) and two different budgets for one tree
// walk would be a coincidence waiting to drift.
const (
	maxStorageScanFiles = maxAckScanFiles
	maxStorageFileBytes = maxAckFileBytes
)

// scanForStorageUse walks dir for the storage hooks.
//
// 🔴 IT DOES NOT SHORT-CIRCUIT ON THE FIRST HIT, and that is the reason it is a
// separate walk rather than a predicate passed to `ackScanner`: the ack scan
// stops at the first ack by design, while this one must learn about BOTH stores
// independently and so has to keep going until the caps.
//
// Not a refactor of ackScanner on purpose. That walk carries a documented
// MASKING PAIR of `found || partial` short-circuits whose asymmetry is load
// bearing — three separate deletions survived this entire repository until
// TestPartialScanNeverBecomesFound was written for them specifically. Threading a
// predicate through it is a change to that control flow, and worth doing only
// with those fixtures in hand. Unifying the two walks is a reasonable follow-up;
// doing it in passing is not.
//
// 🔴 SYMLINKED DIRECTORIES ARE FOLLOWED, for the reason readyack records:
// filepath.WalkDir does not follow them and does not report them as directories
// either, so a monorepo whose `src` is a symlink into a shared package had its
// ENTIRE source tree skipped — reproduced live, and it warned at a correct
// project. Following is also the safe direction here (more files can only add
// evidence). Cycles are bounded by a visited set keyed on the RESOLVED path.
func scanForStorageUse(dir string) storageScanResult {
	s := &storageScanner{visited: map[string]bool{}}
	s.walk(dir)
	if s.files == 0 {
		// Nothing was read at all — not "no storage use".
		s.res.partial = true
	}
	return s.res
}

type storageScanner struct {
	visited map[string]bool
	files   int
	res     storageScanResult
}

func (s *storageScanner) walk(dir string) {
	if s.res.partial {
		return
	}
	// Resolve before recording: two paths reaching the same directory (a symlink
	// and its target) must count once, or a self-referential link recurses
	// forever.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		s.res.partial = true
		return
	}
	if s.visited[real] {
		return
	}
	s.visited[real] = true

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.res.partial = true
		return
	}
	for _, e := range entries {
		if s.res.partial {
			return
		}
		path := filepath.Join(dir, e.Name())
		// os.Stat FOLLOWS symlinks — that is the point. A dangling link fails
		// here and correctly makes the scan partial.
		info, err := os.Stat(path)
		if err != nil {
			s.res.partial = true
			return
		}
		if info.IsDir() {
			// The skip list applies to ENTRIES only, never to the root we were
			// handed: no skip rule can remove the whole tree.
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
		if info.Size() > maxStorageFileBytes {
			s.res.partial = true
			return
		}
		s.files++
		if s.files > maxStorageScanFiles {
			s.res.partial = true
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			s.res.partial = true
			return
		}
		// Comments are stripped for the reason readyack strips them: the
		// scaffolds DOCUMENT these hooks in prose, and a comment naming one is
		// not a call. Without stripping, the check would fire at a project that
		// only mentions storage in a code comment.
		code := blockproto.StripCommentsForExt(string(data), ext)
		if strings.Contains(code, appStorageHook) {
			s.res.usesAppStorage = true
		}
		if strings.Contains(code, sharedStorageHook) {
			s.res.usesSharedStorage = true
		}
	}
}
