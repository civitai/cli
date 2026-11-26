package validate

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// storagemanifest.go mirrors the server's refusals of a manifest's `storage`
// declaration that the vendored JSON Schema CANNOT express.
//
// AUTHORITY: civitai `src/shared/constants/block-storage.constants.ts` —
// `parseManifestStorage` (the declaration) and `appStorageScopeErrors` (the
// declaration against `scopes`), both run by BlockManifestValidator at submit.
// The canonical schema carries the WHOLE declaration shape, deliberately
// including features the platform does not accept yet, so a schema-clean
// manifest can still be refused. This file is the difference, and nothing more:
// every shape rule the schema already states (names, enums, bounds, the
// shared-only keys, write:"none" needing a tally, ownedModelVersions) stays the
// schema's, so no rule lives in two places here. A collection's `schema` (its
// document schema) has its own server-only bounds: storageschema.go.
//
// HARD ERRORS, like the other ported semantic rules (AGENTS.md item 1): each
// one is a refusal the server returns, so a manifest that trips it cannot be
// submitted. Messages are the server's text, so the CLI and the 400 agree.
//
// 🔴 THE ONE EXCEPTION: THE FEATURE GATES ARE WARNINGS (storageFeatureGateWarnings).
// The server refuses six features today, but each is a one-line switch in
// APP_STORAGE_ENABLED_FEATURES, and `app submit` blocks on validate ERRORS. A
// hard error here would outlive the switch in every CLI already installed and
// refuse manifests the server has started to accept. The warning keeps the
// server's text and says the server refuses it at submit TODAY. The vendored
// schema's description is what tells this repository a feature was enabled:
// TestVendoredSchemaStillStatesTheFeatureGates goes red on the re-vendor PR.
// The seventh, `hook`, is switched ON (civitai#8126) but rolled out per author
// (the `app-blocks-storage-hooks` cohort), which the CLI cannot see — so it is a
// warning too, saying that (storageHookRolloutWarning).
//
// REMOVED PRIMITIVES (storageRemovedPrimitiveChecks): counters, marks and
// write "none" were removed (civitai #10391). The server refuses them at submit
// unless the previous approved version already declared them; the CLI cannot
// see that version, so it errors on every declaration and says so.
//
// THE ONE KNOWN DIVERGENCE: where the server lists collection names in
// DECLARATION order, this lists them SORTED. The manifest is decoded into a Go
// map, which keeps no key order; same names, same rule.

// The three document-store scopes (APP_STORAGE_BASE_SCOPE /
// APP_STORAGE_PUBLISH_SCOPE / APP_STORAGE_IMPORT_SCOPE).
const (
	appStorageBaseScope    = "apps:storage"
	appStoragePublishScope = "apps:storage:publish"
	appStorageImportScope  = "apps:storage:import"
)

// Mirrors of the server's import bounds and patterns (APP_STORAGE_MAX_IMPORTS,
// APP_STORAGE_MAX_IMPORT_COLLECTIONS, APP_STORAGE_IMPORT_FROM_RE). The schema
// states each of them; they are here only so storageImportChecks can tell which
// import entries the server's parseImports keeps (see there).
const (
	appStorageMaxImports           = 8
	appStorageMaxImportCollections = 16
)

var appStorageImportFromRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,38}[a-z0-9]$`)

// appStorageCollectionNameRe mirrors APP_STORAGE_COLLECTION_NAME_RE. The server
// skips a collection whose name fails it (the schema reports the name), so the
// imperative rules below skip it too.
var appStorageCollectionNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// appStorageTallyNameRe mirrors APP_STORAGE_TALLY_NAME_RE — a separate server
// constant that happens to hold the same pattern today.
var appStorageTallyNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Mirrors of the server bounds the gate ordering depends on: a value outside
// them gets the range error (the schema's, here), never the "not available yet"
// one.
const (
	appStorageMaxCollections       = 16
	appStorageTTLMinSeconds        = 60
	appStorageTTLMaxSeconds        = 31_536_000
	appStorageMaxDailyWritesAuthor = 1_000
	appStorageMaxCounters          = 8 // APP_STORAGE_MAX_COUNTERS
	appStorageMaxMarks             = 8 // APP_STORAGE_MAX_MARKS
)

// appStorageBuiltinMark mirrors APP_STORAGE_BUILTIN_MARKS: the one mark every
// shared collection has without declaring it, so no counter or mark may use it.
const appStorageBuiltinMark = "report"

// storageNotYet is the server's `notYet` (subject is the field, plus its value
// when the value is what is gated), followed by why the CLI only WARNS.
func storageNotYet(subject, feature, instead string) string {
	return subject + " is not available yet — " + feature +
		" arrive in a later phase of App Storage; " + instead + "." +
		" civitai.com refuses this at submit today; it is a warning here, not an error, because the " +
		"platform enables these features with a server-side switch and a CLI already installed must " +
		"not refuse a manifest the server has started to accept."
}

// declaredCollection is one entry of `storage.collections` that is an object
// under a valid name — the only entries the server goes on to evaluate.
type declaredCollection struct {
	name string
	decl map[string]any
}

// storageCollections returns the evaluable collections, sorted by name, and
// whether `storage.collections` is a usable map at all (an object holding 1 to
// 16 entries). An unusable one is the schema's to report and the server's
// parser stops there, so every imperative rule stays silent on it.
func storageCollections(generic any) ([]declaredCollection, bool) {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil, false
	}
	storage, ok := m["storage"].(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := storage["collections"].(map[string]any)
	if !ok || len(raw) == 0 || len(raw) > appStorageMaxCollections {
		return nil, false
	}
	names := make([]string, 0, len(raw))
	for name := range raw {
		if appStorageCollectionNameRe.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]declaredCollection, 0, len(names))
	for _, name := range names {
		if decl, ok := raw[name].(map[string]any); ok {
			out = append(out, declaredCollection{name: name, decl: decl})
		}
	}
	return out, true
}

// storageHookRolloutWarning says what civitai.com does with a well-formed hook
// now that the `hooks` feature is on (civitai#8126): it accepts a NEW or CHANGED
// hook only from an author in the `app-blocks-storage-hooks` cohort, and refuses
// it otherwise at submit (storage-hooks.ts, reason `not_in_cohort`). A hook
// pinned exactly as the approved version pins it skips the cohort.
//
// Neither input — the author's cohort, the approved manifest — is visible to the
// CLI, so this is a WARNING that states the condition rather than a verdict. The
// wording is the server's: the first sentence is the vendored schema's `hook`
// description, the remedy is the `not_in_cohort` refusal's.
func storageHookRolloutWarning(at string) string {
	return at + " is in a limited rollout: civitai.com accepts a new or changed hook only from " +
		"authors the rollout includes, and refuses it otherwise. If submit refuses it, keep the " +
		"approved hook or remove hook (the collection schema still applies). It is a warning here, " +
		"not an error, because whether this account is in the rollout, and whether the hook is new " +
		"or changed since the approved version, is known only to civitai.com."
}

// storageFeatureGateWarnings reports the six features switched off in
// APP_STORAGE_ENABLED_FEATURES, and the per-author rollout of `hook`, as
// WARNINGS (see the file comment for why).
//
// Each gate fires under exactly the condition the server reaches it: after the
// visibility parsed, and — for the two numeric keys — only once the value is
// in range, because an out-of-range value gets the range refusal instead.
// `registry` is the opposite, its gate coming before any check of its value.
// `hook` is NOT a mirror of the server's order — see the comment at its call.
func storageFeatureGateWarnings(generic any) []Finding {
	cols, ok := storageCollections(generic)
	if !ok {
		return nil
	}
	m, _ := generic.(map[string]any)
	outputDir := m["outputDir"]
	var out []Finding
	for _, c := range cols {
		at := childField("storage.collections", c.name)
		vis, _ := c.decl["visibility"].(string)
		if vis != "private" && vis != "shared" {
			continue // the server stops at an invalid visibility
		}
		shared := vis == "shared"
		write, _ := c.decl["write"].(string)

		if kind, _ := c.decl["kind"].(string); kind == "log" {
			out = append(out, newFinding(childField(at, "kind"), storageNotYet(
				at+`.kind "log"`, "append-only log collections", `declare kind "docs" (or omit kind) for now`)))
		}
		if shared && write == "anyone" {
			out = append(out, newFinding(childField(at, "write"), storageNotYet(
				at+`.write "anyone"`, "documents any viewer may edit", `use "own" for now`)))
		}
		if shared && write == "owner" {
			out = append(out, newFinding(childField(at, "write"), storageNotYet(
				at+`.write "owner"`, "owner-written documents", `use "own" for now`)))
		}
		if isIntegerIn(c.decl["ttlSeconds"], appStorageTTLMinSeconds, appStorageTTLMaxSeconds) {
			out = append(out, newFinding(childField(at, "ttlSeconds"), storageNotYet(
				at+".ttlSeconds", "expiring documents", "remove ttlSeconds for now")))
		}
		if shared && write != "owner" && write != "none" &&
			isIntegerIn(c.decl["dailyWritesPerAuthor"], 1, appStorageMaxDailyWritesAuthor) {
			out = append(out, newFinding(childField(at, "dailyWritesPerAuthor"), storageNotYet(
				at+".dailyWritesPerAuthor", "per-collection write budgets",
				"remove dailyWritesPerAuthor for now (the platform default applies)")))
		}
		// registryExport: the server checks the switch BEFORE the value and the
		// shared-only rule, so any declared `registry` draws the gate.
		if _, ok := c.decl["registry"]; ok {
			out = append(out, newFinding(childField(at, "registry"), storageNotYet(
				at+".registry", "collections published to registries",
				"remove it to keep the collection on Civitai only")))
		}
		// hooks: the server's cohort gate (storage-hooks.ts pinStorageHooks) runs
		// on the RAW declared hooks, BEFORE resolveDeclaredHooks checks their
		// shape — so at submit a malformed hook from an author outside the
		// cohort can be refused `not_in_cohort`, not only for its shape. This
		// warning is deliberately narrower: a malformed hook already draws an
		// ERROR here (the schema's, or the outputDir refusal in
		// storageDeclarationChecks) that blocks submit on its own, so the
		// rollout warning is kept for hooks that error-free validate would
		// otherwise let through.
		if hookShapeAccepted(c.decl["hook"], outputDir) {
			out = append(out, newFinding(childField(at, "hook"), storageHookRolloutWarning(at+".hook")))
		}
	}
	return out
}

// Mirrors of the server's hook shape (APP_STORAGE_HOOK_FILE_PATTERN,
// APP_STORAGE_HOOK_FILE_FORBIDDEN_SEGMENT_PATTERN, APP_STORAGE_HOOK_FILE_MAX_LENGTH,
// APP_STORAGE_HOOK_SHA256_PATTERN, APP_STORAGE_HOOK_TIMEOUT_MIN_MS/MAX_MS). The
// schema states each of them; they are here only so the hook gate fires when
// the server's would (see hookShapeAccepted).
var (
	appStorageHookFileRe      = regexp.MustCompile(`^([A-Za-z0-9_-][A-Za-z0-9_.-]*/)*[A-Za-z0-9_-][A-Za-z0-9_.-]*\.m?js$`)
	appStorageHookForbiddenRe = regexp.MustCompile(`(^|/)node_modules(/|$)`)
	appStorageHookSHA256Re    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const (
	appStorageHookFileMaxLength = 200
	appStorageHookTimeoutMinMs  = 1
	appStorageHookTimeoutMaxMs  = 500
)

// hookShapeAccepted mirrors the server's parseHook accepting a hook: an object
// holding only file, sha256 and timeoutMs, each well-formed, with `file` outside
// the manifest's `outputDir`. It reads no file and checks no digest — whether
// the file exists in the bundle with that sha256 is the server's, at submit.
func hookShapeAccepted(v any, outputDir any) bool {
	h, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for key := range h {
		if key != "file" && key != "sha256" && key != "timeoutMs" {
			return false
		}
	}
	file, ok := hookFileShapeAccepted(h["file"])
	if !ok || isStorageHookFileUnderOutputDir(file, outputDir) {
		return false
	}
	if sha, ok := h["sha256"].(string); !ok || !appStorageHookSHA256Re.MatchString(sha) {
		return false
	}
	if t, present := h["timeoutMs"]; present && !isIntegerIn(t, appStorageHookTimeoutMinMs, appStorageHookTimeoutMaxMs) {
		return false
	}
	return true
}

// hookFileShapeAccepted is parseHook's `file` check up to (not including) the
// outputDir one: a string of at most 200 characters matching the file pattern,
// with no node_modules segment. Every character the pattern admits is ASCII, so
// len() counts what JavaScript's `length` does for any file that can pass.
func hookFileShapeAccepted(v any) (string, bool) {
	file, ok := v.(string)
	if !ok || len(file) > appStorageHookFileMaxLength ||
		!appStorageHookFileRe.MatchString(file) || appStorageHookForbiddenRe.MatchString(file) {
		return "", false
	}
	return file, true
}

// outputDirPrefix mirrors the server's (block-storage.constants.ts): the
// manifest's `outputDir` as a bundle-path prefix — "" when it names the project
// root (every file is inside it), and ok=false when the manifest has none, or
// one that is not a string.
func outputDirPrefix(outputDir any) (string, bool) {
	s, ok := outputDir.(string)
	if !ok {
		return "", false
	}
	dir := jsTrim(s)
	for strings.HasPrefix(dir, "./") {
		dir = dir[2:]
	}
	for strings.HasSuffix(dir, "/") {
		dir = dir[:len(dir)-1]
	}
	if dir == "." {
		return "", true
	}
	return dir, true
}

// isStorageHookFileUnderOutputDir mirrors the server function of the same name:
// whether a hook `file` lies inside the manifest's `outputDir` (the build's
// output) — equal to it, or under it at a "/" boundary, so `dist2/hook.js` is
// NOT inside `dist`.
func isStorageHookFileUnderOutputDir(file string, outputDir any) bool {
	dir, ok := outputDirPrefix(outputDir)
	if !ok {
		return false
	}
	return dir == "" || file == dir || strings.HasPrefix(file, dir+"/")
}

// jsTrim is JavaScript's String.prototype.trim: it strips WhiteSpace (TAB, VT,
// FF, U+FEFF and every Zs space) and LineTerminator (LF, CR, U+2028, U+2029).
// Not strings.TrimSpace, which also strips U+0085 and keeps U+FEFF.
func jsTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		switch r {
		case '\t', '\v', '\f', '\n', '\r', '\uFEFF', '\u2028', '\u2029':
			return true
		}
		return unicode.Is(unicode.Zs, r)
	})
}

// storageHookOutputDirChecks reports parseHook's one refusal the schema cannot
// state: a hook `file` inside the manifest's `outputDir`. The server reaches it
// for any collection whose visibility parsed and whose `hook` is an object with
// a `file` that passed the shape and node_modules checks — independently of
// that hook's sha256, timeoutMs and keys, which it reports beside it. Message is
// the server's text (quoteName is JSON.stringify of boundKey; every character a
// passing file may hold is printable ASCII, so %q matches it).
func storageHookOutputDirChecks(generic any) []Finding {
	cols, ok := storageCollections(generic)
	if !ok {
		return nil
	}
	m, _ := generic.(map[string]any)
	var out []Finding
	for _, c := range cols {
		if vis, _ := c.decl["visibility"].(string); vis != "private" && vis != "shared" {
			continue // the server stops at an invalid visibility
		}
		h, ok := c.decl["hook"].(map[string]any)
		if !ok {
			continue
		}
		file, ok := hookFileShapeAccepted(h["file"])
		if !ok || !isStorageHookFileUnderOutputDir(file, m["outputDir"]) {
			continue
		}
		at := childField(childField(childField("storage.collections", c.name), "hook"), "file")
		out = append(out, newFinding(at, fmt.Sprintf(
			"%s %q is inside the build output (outputDir) — a hook must be a committed source file of "+
				"the bundle, shipped verbatim, so its sha256 can be checked at submit", at, boundKey(file))))
	}
	return out
}

// storageDeclarationChecks reports the declaration refusals the schema admits
// and the server keeps refusing: the two import rules (storageImportChecks), a
// hook file inside outputDir (storageHookOutputDirChecks — the schema sees no
// other key), and a counter and a mark sharing one name (two lists, each
// `uniqueItems` on its own, so no schema keyword relates them).
//
// The server compares the two lists only as ITS parser left them, and
// parseTallyNames discards a whole list at its first unusable entry — so a list
// the schema already rejects (see tallyNames) is compared as empty here too,
// and draws no second, collision finding.
func storageDeclarationChecks(generic any) []Finding {
	out := storageImportChecks(generic)
	out = append(out, storageHookOutputDirChecks(generic)...)
	cols, ok := storageCollections(generic)
	if !ok {
		return out
	}
	for _, c := range cols {
		at := childField("storage.collections", c.name)
		if vis, _ := c.decl["visibility"].(string); vis != "shared" {
			continue // tallies are shared-only; the server stops at an invalid visibility
		}
		marks := map[string]bool{}
		for _, mk := range tallyNames(c.decl["marks"], appStorageMaxMarks) {
			marks[mk] = true
		}
		for _, counter := range tallyNames(c.decl["counters"], appStorageMaxCounters) {
			if marks[counter] {
				out = append(out, newFinding(at, fmt.Sprintf(
					"%s declares %q as both a counter and a mark — use different names", at, counter)))
			}
		}
	}
	return out
}

// storageImportChecks reports the two refusals of `storage.imports` that the
// schema admits and the server's parseImports (block-storage.constants.ts)
// returns: an import FROM THIS APP ITSELF (`from === ownBlockId`), and a SECOND
// import from a source already imported (`imports.has(from)`). Every other
// parseImports refusal — not an array, 0 or more than 8 entries, an unknown
// key, a bad `from`, `access` or collection name, a repeated collection — is
// one the schema already states, so it stays the schema's.
//
// The duplicate rule is subtle on purpose: the server compares against the map
// of imports it KEPT, and it keeps an entry only when that entry drew no error
// at all. So `importEntryKept` mirrors the per-entry checks, and an entry the
// schema already refuses does not make a later one a "duplicate". Messages are
// the server's text.
func storageImportChecks(generic any) []Finding {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	storage, ok := m["storage"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := storage["imports"].([]any)
	// The server stops at an unusable array before reading any entry.
	if !ok || len(raw) == 0 || len(raw) > appStorageMaxImports {
		return nil
	}
	own := m["blockId"]
	kept := map[string]bool{}
	var out []Finding
	for i, e := range raw {
		at := indexField("storage.imports", i)
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		from, isString := entry["from"].(string)
		fromErr := false
		switch {
		case !isString || !appStorageImportFromRe.MatchString(from):
			fromErr = true // the schema's finding
		case from == own:
			// parseImports: `${at}.from names this app itself — …`
			out = append(out, newFinding(childField(at, "from"),
				at+".from names this app itself — declare its collections instead"))
			fromErr = true
		case kept[from]:
			// parseImports: `${at} imports from ${quoteName(from)} again — …`
			out = append(out, newFinding(at,
				at+` imports from "`+from+`" again — declare one import per app`))
			fromErr = true
		}
		if !fromErr && importEntryKept(entry) {
			kept[from] = true
		}
	}
	return out
}

// importEntryKept mirrors the parseImports checks other than `from`: an entry
// with any of these errors is not added to the server's map of imports.
func importEntryKept(entry map[string]any) bool {
	for key := range entry {
		if key != "from" && key != "collections" && key != "access" {
			return false
		}
	}
	if access, _ := entry["access"].(string); access != "read" && access != "readwrite" {
		return false
	}
	names, ok := entry["collections"].([]any)
	if !ok || len(names) == 0 || len(names) > appStorageMaxImportCollections {
		return false
	}
	seen := map[string]bool{}
	for _, n := range names {
		name, ok := n.(string)
		if !ok || !appStorageCollectionNameRe.MatchString(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

// storageImportSources returns the `from` of every import, in declaration
// order — the server's `[...parsed.imports.keys()]`. It is only read when the
// declaration drew no error, so every entry is then one the server kept.
func storageImportSources(generic any) []string {
	m, _ := generic.(map[string]any)
	storage, _ := m["storage"].(map[string]any)
	raw, _ := storage["imports"].([]any)
	var out []string
	for _, e := range raw {
		if entry, ok := e.(map[string]any); ok {
			if from, ok := entry["from"].(string); ok {
				out = append(out, from)
			}
		}
	}
	return out
}

// storageRemovedPrimitiveInstead is the server's VIEWER_KEYED_INSTEAD remedy.
const storageRemovedPrimitiveInstead = `declare a viewer-keyed collection instead (keyedBy "viewer" or ` +
	`"parent+viewer": each viewer's own document is their vote or entry), and report a document ` +
	`through the moderation report API — see https://developer.civitai.com/apps/guide/storage`

// storageRemovedPrimitiveGrandfather is the CLI's own addition to the server's
// first-version message: the one case the server accepts and the CLI cannot see.
const storageRemovedPrimitiveGrandfather = ` (civitai #10391). civitai.com accepts one only on a ` +
	`collection whose previous approved version already declares it, where it does nothing; the CLI ` +
	`cannot see the approved manifest, so it reports every declaration`

// storageRemovedPrimitiveChecks mirrors the server's deprecatedPrimitiveErrors
// (civitai src/server/services/apps/storage-schema-migrations.ts): counters,
// marks and `write: "none"` were REMOVED from App Storage (civitai #10391). At
// submit the server refuses one that the collection's PREVIOUS APPROVED version
// did not declare, and every one on a first version.
//
// 🔴 THE CLI CANNOT SEE THE APPROVED MANIFEST, so it cannot tell a kept
// (grandfathered, inert) declaration from a new one. These are ERRORS for every
// declaration — the first-version verdict — because a new declaration is the
// case that costs an author a refused submit, and a kept one does nothing and
// can simply be removed. The message says so, so an author of an approved app
// that keeps one knows civitai.com would accept it.
//
// Read as the server's parser leaves the declaration: only on a shared
// collection (the schema refuses the keys on a private one), and a tally list
// the parser would discard (tallyNames) declares nothing.
func storageRemovedPrimitiveChecks(generic any) []Finding {
	cols, ok := storageCollections(generic)
	if !ok {
		return nil
	}
	var out []Finding
	for _, c := range cols {
		at := childField("storage.collections", c.name)
		if vis, _ := c.decl["visibility"].(string); vis != "shared" {
			continue
		}
		for _, f := range []struct {
			field string
			max   int
		}{{"marks", appStorageMaxMarks}, {"counters", appStorageMaxCounters}} {
			names := tallyNames(c.decl[f.field], f.max)
			if len(names) == 0 {
				continue
			}
			quoted := make([]string, len(names))
			for i, n := range names {
				quoted[i] = fmt.Sprintf("%q", n)
			}
			out = append(out, newFinding(childField(at, f.field), fmt.Sprintf(
				"%s.%s: %s were removed and cannot be declared on a new app (declared: %s)%s; %s",
				at, f.field, f.field, strings.Join(quoted, ", "), storageRemovedPrimitiveGrandfather,
				storageRemovedPrimitiveInstead)))
		}
		if write, _ := c.decl["write"].(string); write == "none" {
			out = append(out, newFinding(childField(at, "write"), fmt.Sprintf(
				`%s.write: "none" was removed and cannot be declared on a new app%s; %s`,
				at, storageRemovedPrimitiveGrandfather, storageRemovedPrimitiveInstead)))
		}
	}
	return out
}

// tallyNames mirrors the server's parseTallyNames: the names of a `counters` or
// `marks` list, or NONE when the server would discard the list — not an array,
// more than max entries, or any entry that is not a snake_case string, is the
// built-in `report`, or repeats an earlier one. (Each of those is the schema's
// finding to report; this only decides what the collision rule sees.)
func tallyNames(v any, max int) []string {
	arr, ok := v.([]any)
	if !ok || len(arr) > max {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		name, ok := e.(string)
		if !ok || !appStorageTallyNameRe.MatchString(name) || name == appStorageBuiltinMark || seen[name] {
			return nil
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// storageScopeRuleChecks is the server's appStorageScopeErrors: the seven rules
// between `storage` and `scopes` (three for imports, four for collections).
//
// storageValid must be false whenever the declaration itself drew ANY ERROR
// (schema or imperative). The server evaluates these rules only against a
// declaration that parsed, because an invalid one declares nothing and a scope
// rule against it reports a second, misleading problem.
//
// 🔴 A FEATURE-GATE WARNING DOES NOT COUNT, deliberately. Today's server fails
// the parse on a gated feature and so reports the gate alone; here the gate is
// advisory, and the scope rules judge the declaration as the server will once
// the switch is on. An author who clears validate with a gated feature is then
// never refused, after the switch, for a missing scope validate never named. A
// manifest with no gated feature is unaffected: no gate finding either way.
//
// FIELD: each finding names the side that is MISSING (AGENTS.md item 23) — a
// declaration with no scope is a `scopes` finding, a scope with nothing to
// reach is a `storage` finding.
func storageScopeRuleChecks(generic any, storageValid bool) []Finding {
	if !storageValid {
		return nil
	}
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	declared := stringSet(m["scopes"])
	_, hasBase := declared[appStorageBaseScope]
	_, hasPublish := declared[appStoragePublishScope]
	_, hasImport := declared[appStorageImportScope]

	cols, _ := storageCollections(generic)
	var names, publishing []string
	for _, c := range cols {
		names = append(names, c.name)
		if collectionNeedsPublishScope(c.decl) {
			publishing = append(publishing, c.name)
		}
	}
	sources := storageImportSources(generic)

	// In the server's order (appStorageScopeErrors): the three import rules
	// first, then the four collection rules.
	var out []Finding
	if len(names) == 0 && len(sources) > 0 && !hasBase {
		out = append(out, newFinding("scopes", fmt.Sprintf(
			"storage imports from %s but scopes does not include %s — add it, or remove storage",
			strings.Join(sources, ", "), appStorageBaseScope)))
	}
	if len(sources) > 0 && !hasImport {
		out = append(out, newFinding("scopes", fmt.Sprintf(
			"storage imports from %s, which requires the %s scope — add it to scopes with a "+
				"scopeJustifications entry", strings.Join(sources, ", "), appStorageImportScope)))
	}
	if len(sources) == 0 && hasImport {
		out = append(out, newFinding("storage", fmt.Sprintf(
			"scopes includes %s but storage declares no imports — declare them under storage.imports, "+
				"or remove the scope", appStorageImportScope)))
	}
	if len(names) > 0 && !hasBase {
		out = append(out, newFinding("scopes", fmt.Sprintf(
			"storage declares %s but scopes does not include %s — add it, or remove storage",
			strings.Join(names, ", "), appStorageBaseScope)))
	}
	if len(publishing) > 0 && !hasPublish {
		out = append(out, newFinding("scopes", fmt.Sprintf(
			"storage collection %s lets viewers write documents other users read, which requires the %s "+
				"scope — add it to scopes with a scopeJustifications entry",
			strings.Join(publishing, ", "), appStoragePublishScope)))
	}
	// An app that only imports declares no collections of its own and still
	// needs apps:storage (the storage route requires it on every call), so the
	// "nothing to reach" rule waits for no imports either.
	if len(names) == 0 && len(sources) == 0 && hasBase {
		out = append(out, newFinding("storage", fmt.Sprintf(
			"scopes includes %s but storage declares no collections — declare them under "+
				"storage.collections, or remove the scope", appStorageBaseScope)))
	}
	if len(publishing) == 0 && hasPublish {
		// The server names only the viewer-authored policies an author can
		// declare TODAY — "anyone" is switched off, so "own" alone.
		out = append(out, newFinding("storage", fmt.Sprintf(
			`scopes includes %s but no storage collection is shared with write "own" — remove the `+
				"scope, or declare the collection it is for", appStoragePublishScope)))
	}
	return out
}

// collectionNeedsPublishScope mirrors the server's: a SHARED collection whose
// write policy lets a viewer author documents other viewers read. An allowlist,
// so `none` (counters and marks only) never needs the consent-gated scope;
// `anyone` stays listed although the feature gate refuses it today.
func collectionNeedsPublishScope(decl map[string]any) bool {
	vis, _ := decl["visibility"].(string)
	write, _ := decl["write"].(string)
	return vis == "shared" && (write == "own" || write == "anyone")
}

// hasFindingUnder reports whether any finding is located at `storage` or below
// it — the local stand-in for "the server's storage parse returned errors".
func hasFindingUnder(fs []Finding, root string) bool {
	for _, f := range fs {
		if f.Field == root || strings.HasPrefix(f.Field, root+".") || strings.HasPrefix(f.Field, root+"[") {
			return true
		}
	}
	return false
}

// isIntegerIn mirrors the server helper of the same name: a JSON number that is
// whole and within [min, max].
func isIntegerIn(v any, min, max float64) bool {
	n, ok := toNumber(v)
	return ok && n == math.Trunc(n) && n >= min && n <= max
}
