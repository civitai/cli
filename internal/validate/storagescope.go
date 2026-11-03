// storagescope.go is the SOURCE half of `validate`'s storage checks: what an
// app's code does with storage, read against what its manifest declares. The
// manifest half — the server's refusals of the declaration itself — is
// storagemanifest.go.
//
// THE MOTIVATING FAILURE (2026-10-01). An app saved every character, stack and
// sheet through storage and declared only `ai:write:budgeted`. It passed 198
// unit tests, the dev harness, `civitai app validate` and a full submit — then
// every save failed in production with a 403 the viewer saw as "Saving failed
// for an unknown reason… try again". `validate` read the manifest alone, so it
// could not relate "this app calls storage" to "this manifest declares no
// storage scope". This check is the one surface that can catch an app with no
// tests at all.
//
// ---
//
// WHAT IT REPORTS (three findings, all ADVISORY warnings):
//
//  1. A STRING-LITERAL collection name the manifest does not declare —
//     `app.storage.collection('scores')` with no `scores` under
//     `storage.collections`. A collection that is not declared does not exist at
//     runtime: every call on it is refused `collection_not_declared`. Only a
//     literal is a claim about a name; a computed one (`collection(name)`, a
//     template with `${}`, a concatenation) is unknowable here and not flagged.
//     A call with a string-literal `from` option — `collection('kv', { from:
//     'parent' })`, another app's collection through `storage.imports` — is
//     checked against THAT import instead: warned when no import names the
//     source, or when the import does not list the collection. A computed
//     `from` is unknowable and not flagged; an options VARIABLE is read as an
//     own collection, as it always was.
//  2. The document store used while the manifest has neither `apps:storage`
//     nor a `storage` declaration — every call is refused `insufficient_scope`.
//     Any call counts, literal or not. When `storage` IS declared, the missing
//     scope is the manifest error storageScopeRuleChecks already reports, so
//     this stays quiet rather than tell the same story twice.
//  3. A CALL to a legacy hook while the manifest declares NO scope for that
//     store: `useAppStorage()` without `apps:storage:read` or
//     `apps:storage:write`, `useSharedStorage()` without
//     `apps:storage:shared:read` or `apps:storage:shared:write`. Every save then
//     403s in production — the 2026-10-01 failure, which is what this advisory
//     was written for (cli #767). One of a store's scopes is someone who knows
//     scopes exist and chose, so it is silent; the two stores are asked
//     independently. The scopes are an explicit SET, not a prefix:
//     `apps:storage:publish` starts with `apps:storage:` and grants the legacy
//     store nothing.
//
// WHAT IT MATCHES, AND WHY THAT PRECISE SHAPE. This replaced HOOK-NAME matching
// (cli #767), which fired on the bare names `useAppStorage` / `useSharedStorage`
// anywhere in source. Its measured false positive: the playable-collections app's
// `src/scope-contract.test.ts`, an object whose string VALUES name hooks — code,
// not a call. Two changes close that class:
//
//   - a finding needs a CALL SHAPE: `useAppStorage(` (an optional type argument
//     allowed), and for the document store a `.collection(` on one of two
//     receivers: a name ending in `storage`/`Storage` (`app.storage`,
//     `fake.storage`, `(await app()).storage`, a renamed `docStorage`), or an
//     AWAITED, ARGUMENT-LESS CALL — `(await docs()).collection(` — the shape
//     every real port uses, where the client comes from a getter. Any other
//     `.collection(` — the same app's `PATHS.collection(id)`, an un-awaited
//     `getApi().collection(` — is not the store. Type arguments may nest two
//     levels (`collection<Record<string, Array<Row>>>(`);
//   - TEST CODE IS NOT READ. A test drives fakes, ledgers and deliberately wrong
//     names; none of it is the app's runtime. THE CONVENTION (isTestFile): a
//     basename containing `.test.` / `.spec.`, a basename STARTING `fake-`,
//     `test-` or `mock-` (the scaffolds' `src/fake-doc-storage.ts`,
//     `src/test-setup.ts`, `src/mock-buzz.ts`), or anything under a
//     `__tests__/`, `__mocks__/` or `testing/` directory. Name a test helper
//     that way and it is skipped; name it anything else and it is read as app
//     code.
//
// The known residuals: a client held in a variable whose name does not end in
// `storage` (`const s = app.storage; s.collection('x')`), or reached through a
// call that takes arguments or is not awaited, is not seen — the silent
// direction, accepted because widening the receiver is what produced false
// positives; a call spelled inside a string literal is matched; and a
// production file whose name starts `fake-`/`test-`/`mock-` is skipped.
//
// ADVISORY, NOT FATAL, for the reason readyack is: it infers runtime behaviour
// from static text. `--strict` promotes it. The manifest-only refusals in
// storagemanifest.go ARE errors, because those are certain.
//
// FIELDS: (1) names the undeclared key, `storage.collections.<name>` — the
// declaration is the usual edit; (2) and (3) are `scopes` — the source is right
// (using storage is the point) and the manifest is what lacks the scope.
package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/blockproto"
)

// The legacy hooks, by name. A finding needs a CALL to one (legacyHookCallRe).
const (
	appStorageHook    = "useAppStorage"
	sharedStorageHook = "useSharedStorage"
)

// legacyHookScopes is, per hook, the scopes that reach its store. Declaring
// ANY one of them silences that hook's advisory. An explicit set, never a
// prefix: see the file comment.
var legacyHookScopes = map[string][2]string{
	appStorageHook:    {"apps:storage:read", "apps:storage:write"},
	sharedStorageHook: {"apps:storage:shared:read", "apps:storage:shared:write"},
}

// typeArgRe is an optional TS type argument list, with up to two levels of
// nesting (`<Row>`, `<Record<string, Row>>`, `<Map<string, Array<Row>>>`).
// Parentheses end it, so a `<` that is really a comparison cannot swallow a
// call.
const typeArgRe = `(?:<(?:[^<>()]|<(?:[^<>()]|<[^<>()]*>)*>)*>\s*)?`

// legacyHookCallRe matches a call to a legacy hook, with an optional TS type
// argument (`useSharedStorage<Entry<number>>()`). The leading boundary keeps
// `myuseAppStorage(` out.
var legacyHookCallRe = regexp.MustCompile(`\b(useAppStorage|useSharedStorage)\s*` + typeArgRe + `\(`)

// docStoreCallRe matches the opening of a document-store call up to and
// including its `(`: a receiver — a name ending in `storage`/`Storage`, or an
// awaited argument-less call in parentheses, `(await docs())` — then `.` or
// `?.` (whitespace and newlines allowed, for a chain split across lines), then
// `collection`, an optional type argument, and the paren.
var docStoreCallRe = regexp.MustCompile(
	`(?:[sS]torage|\(\s*await\s+[A-Za-z_$][\w$]*\s*\(\s*\)\s*\))\s*\??\.\s*collection\s*` + typeArgRe + `\(`)

// storageScopeChecks returns the source-reading storage advisories for the
// project in dir.
//
// It runs in the projectState branch of validateDir (beside readyAckChecks) and
// NOT in warningChecks: warningChecks is reached under ManifestOnly, which
// `civitai app init` uses to self-check the template it just wrote, and a check
// that reads `src/` has no business running there.
func storageScopeChecks(dir string, generic any) []Finding {
	scan := scanForStorageUse(dir)
	if scan.partial {
		// Reading only PART of a tree is not finding nothing. Same posture as
		// readyack's caps: an unobservable scan stays silent rather than
		// reporting against a project we did not finish reading.
		return nil
	}
	m, _ := generic.(map[string]any)

	var out []Finding

	// (1) Undeclared literal names. The declared set is the raw keys of
	// `storage.collections`, valid or not: a name that IS there but malformed
	// is the schema's finding, and reporting it a second time as "undeclared"
	// would be wrong.
	declared := map[string]bool{}
	if st, ok := m["storage"].(map[string]any); ok {
		if cols, ok := st["collections"].(map[string]any); ok {
			for name := range cols {
				declared[name] = true
			}
		}
	}
	for _, name := range sortedKeys(scan.literalNames) {
		if declared[name] {
			continue
		}
		field := "storage.collections"
		if appStorageCollectionNameRe.MatchString(name) {
			field = childField(field, name)
		}
		out = append(out, newFinding(field, fmt.Sprintf(
			"%s calls `storage.collection(%q)` but storage.collections does not declare %q — a collection "+
				"the manifest does not declare does not exist at runtime, so every call on it is refused "+
				"with `collection_not_declared`. Declare it under storage.collections, or correct the name. "+
				"ADVISORY: this reads source statically, so only string-literal names are checked.",
			scan.literalNames[name], name, name)))
	}

	// (1b) A literal name read through an import — `collection('kv', { from:
	// 'parent' })` — is another app's collection, so it is checked against the
	// import from that app, never against storage.collections. The runtime
	// refuses both misses below with `import_not_declared` (civitai
	// doc-store-access.ts). A malformed `storage.imports` is the schema's
	// finding; stay quiet rather than call every import missing.
	if len(scan.importNames) > 0 {
		var rawImports []any
		importsOK := true
		if st, ok := m["storage"].(map[string]any); ok {
			if v, present := st["imports"]; present {
				rawImports, importsOK = v.([]any)
			}
		}
		if importsOK {
			out = append(out, importedCollectionAdvisories(scan.importNames, rawImports)...)
		}
	}

	// (2) The document store with neither the scope nor a declaration. A
	// malformed `scopes` (not an array) is the schema's finding; stay quiet.
	if scan.docStoreFile != "" && m != nil {
		_, hasStorage := m["storage"]
		scopes, scopesOK := m["scopes"].([]any)
		if scopesOK && !hasStorage && !containsString(scopes, appStorageBaseScope) {
			out = append(out, newFinding("scopes", scan.docStoreFile+
				" calls the document store (`storage.collection()`) but scopes does not include "+
				"apps:storage and the manifest declares no storage — every document-store call is "+
				"refused in production with `insufficient_scope`. Add apps:storage to scopes and "+
				"declare each collection under storage.collections. ADVISORY: this reads source statically."))
		}
	}

	// (3) Legacy hooks called while the manifest declares no scope for that
	// store.
	declaredScopes := stringSet(m["scopes"])
	for _, hook := range sortedKeys(scan.legacyHooks) {
		scopes := legacyHookScopes[hook]
		_, hasRead := declaredScopes[scopes[0]]
		_, hasWrite := declaredScopes[scopes[1]]
		if hasRead || hasWrite {
			continue
		}
		out = append(out, newFinding("scopes", legacyHookScopeAdvice(hook, scopes[0], scopes[1])))
	}
	return out
}

// importedCollectionAdvisories reports each `from` call whose source app the
// manifest does not import from, or whose collection that import does not list.
// FIELDS name the declaration to edit: `storage.imports` when no import names
// the source, that import's own `collections` when it does.
func importedCollectionAdvisories(calls map[string]map[string]string, rawImports []any) []Finding {
	type decl struct {
		index int
		names map[string]bool
	}
	imports := map[string]decl{}
	for i, e := range rawImports {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		from, ok := entry["from"].(string)
		if !ok {
			continue
		}
		if _, dup := imports[from]; dup {
			continue // the first declaration is the one to edit; a second is refused anyway
		}
		names := map[string]bool{}
		if cols, ok := entry["collections"].([]any); ok {
			for _, c := range cols {
				if s, ok := c.(string); ok {
					names[s] = true
				}
			}
		}
		imports[from] = decl{index: i, names: names}
	}

	sources := make([]string, 0, len(calls))
	for from := range calls {
		sources = append(sources, from)
	}
	sort.Strings(sources)
	var out []Finding
	for _, from := range sources {
		d, imported := imports[from]
		for _, name := range sortedKeys(calls[from]) {
			call := fmt.Sprintf("%s calls `storage.collection(%q, { from: %q })`", calls[from][name], name, from)
			switch {
			case !imported:
				out = append(out, newFinding("storage.imports", fmt.Sprintf(
					"%s but storage.imports declares no import from %q — an app reaches another app's "+
						"collection only through an import it declares, so every call on it is refused with "+
						"`import_not_declared`. Declare the import under storage.imports, or correct the "+
						"source. ADVISORY: this reads source statically, so only string-literal names are checked.",
					call, from)))
			case !d.names[name]:
				out = append(out, newFinding(childField(indexField("storage.imports", d.index), "collections"),
					fmt.Sprintf(
						"%s but the import from %q does not list %q — a collection an import does not list "+
							"is not reachable, so every call on it is refused with `import_not_declared`. Add it "+
							"to that import's collections, or correct the name. ADVISORY: this reads source "+
							"statically, so only string-literal names are checked.",
						call, from, name)))
			}
		}
	}
	return out
}

// legacyHookScopeAdvice is the whole message, naming the hook found, both
// scopes, and what the failure looks like if it ships. It names the FAILURE
// because the defect is invisible until production: a developer who has only
// ever run the mock host has no reason to connect "saving failed" to a manifest
// field. The pre-#856 text, minus its old last clause ("a hook imported but
// never called is a false positive"), which a call-shape match made untrue.
func legacyHookScopeAdvice(hook, readScope, writeScope string) string {
	return "this project's source calls `" + hook + "()` but the manifest declares no " +
		"storage scope — add the ones it uses to `scopes` (`" + readScope + "` to read, `" +
		writeScope + "` to write) each with a `scopeJustifications` entry. The server gates " +
		"storage by PRESENCE in the approved scope set, so without them every call fails in " +
		"production with a 403 (`storage set requires the " + writeScope + " scope`) while " +
		"working locally — and an authorization failure is deliberately unclassified by the " +
		"SDK, so a block usually surfaces it as an unexplained \"try again\". ADVISORY: this " +
		"reads source statically."
}

func containsString(arr []any, want string) bool {
	for _, v := range arr {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// storageScanResult is what the tree walk learned. Each map value, and
// docStoreFile, is the FIRST file (in walk order, which is sorted) the thing
// was seen in, relative to the project root with `/` separators. `partial` is
// sticky and dominates: it means we stopped reading, so nothing else is a claim.
type storageScanResult struct {
	literalNames map[string]string // collection name -> first file (an own collection: no `from`)
	// importNames is source blockId -> collection name -> first file, for calls
	// with a string-literal `from` option: `collection('kv', { from: 'parent' })`.
	importNames  map[string]map[string]string
	docStoreFile string            // first file with any document-store call
	legacyHooks  map[string]string // hook name -> first file
	partial      bool
}

// isTestFile reports whether a source file is test code, by the documented
// convention (see the file comment): `*.test.*` / `*.spec.*`; a basename
// starting `fake-`, `test-` or `mock-`; or anything under `__tests__/`,
// `__mocks__/` or `testing/`.
func isTestFile(rel string) bool {
	segs := strings.Split(rel, "/")
	for _, seg := range segs[:len(segs)-1] {
		switch seg {
		case "__tests__", "__mocks__", "testing":
			return true
		}
	}
	base := strings.ToLower(filepath.Base(rel))
	for _, prefix := range []string{"fake-", "test-", "mock-"} {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
}

// literalArgEnd parses the argument that starts at code[i] (just past the
// call's `(`). It returns the name and true only for a single string literal —
// '…', "…" or a backtick template with no `${` — followed by `)` or `,`, plus
// the index of that `)` or `,` so a caller can read what follows. Anything else
// (an identifier, a concatenation, an interpolation, an escape) is a computed
// name and returns false.
func literalArgEnd(code string, i int) (string, int, bool) {
	for i < len(code) && strings.ContainsRune(" \t\r\n", rune(code[i])) {
		i++
	}
	if i >= len(code) {
		return "", 0, false
	}
	q := code[i]
	if q != '\'' && q != '"' && q != '`' {
		return "", 0, false
	}
	end := strings.IndexByte(code[i+1:], q)
	if end < 0 {
		return "", 0, false
	}
	name := code[i+1 : i+1+end]
	if strings.ContainsAny(name, "\\\n") || (q == '`' && strings.Contains(name, "${")) {
		return "", 0, false
	}
	j := i + 1 + end + 1
	for j < len(code) && strings.ContainsRune(" \t\r\n", rune(code[j])) {
		j++
	}
	if j >= len(code) || (code[j] != ')' && code[j] != ',') {
		return "", 0, false
	}
	return name, j, true
}

// fromKind classifies a collection call's options argument.
type fromKind int

const (
	noFrom       fromKind = iota // no options, or an options object with no `from` key: an own collection
	literalFrom                  // `{ from: 'parent' }`: the import from that app
	computedFrom                 // `{ from: PARENT }` / `{ from }`: another app's, but which is unknowable
)

// fromKeyRe finds a `from` key in an options object literal, quoted or not.
// The leading boundary keeps `fromCache:` / `datefrom:` out; the trailing one
// accepts `:` (a value follows) and `,` / end (the `{ from }` shorthand).
var fromKeyRe = regexp.MustCompile(`(?:^|[\s,{])(?:from|'from'|"from")\s*(:|,|$)`)

// maxOptionsScan bounds how far importSourceArg reads for the options
// object's closing brace.
const maxOptionsScan = 2000

// importSourceArg reads the options argument of a collection call whose name
// argument ended at code[end] (`)` or `,`). Only an object LITERAL is read: an
// options variable (`collection('kv', opts)`) is not, and keeps the own-
// collection reading it always had — the documented residual.
func importSourceArg(code string, end int) (string, fromKind) {
	if end >= len(code) || code[end] != ',' {
		return "", noFrom
	}
	k := end + 1
	for k < len(code) && strings.ContainsRune(" \t\r\n", rune(code[k])) {
		k++
	}
	if k >= len(code) || code[k] != '{' {
		return "", noFrom
	}
	depth, close := 0, -1
	for p := k; p < len(code) && p-k < maxOptionsScan; p++ {
		switch code[p] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 {
			close = p
			break
		}
	}
	if close < 0 {
		return "", noFrom
	}
	obj := code[k+1 : close]
	loc := fromKeyRe.FindStringSubmatchIndex(obj)
	if loc == nil {
		return "", noFrom
	}
	if obj[loc[2]:loc[3]] != ":" {
		return "", computedFrom // `{ from }` — a variable named from
	}
	// The value, read as literalArgEnd reads a name; the appended `,` stands in for
	// the end of the object when `from` is its last member.
	if from, _, ok := literalArgEnd(obj+",", loc[3]); ok {
		return from, literalFrom
	}
	return "", computedFrom
}

// Scan caps, deliberately the same values readyack uses — the hazard is
// identical (a project that commits a large bundle or a vendored tree must not
// turn `validate` into a memory event) and two different budgets for one tree
// walk would be a coincidence waiting to drift.
const (
	maxStorageScanFiles = maxAckScanFiles
	maxStorageFileBytes = maxAckFileBytes
)

// scanForStorageUse walks dir for storage calls.
//
// 🔴 IT DOES NOT SHORT-CIRCUIT ON THE FIRST HIT, and that is the reason it is a
// separate walk rather than a predicate passed to `ackScanner`: the ack scan
// stops at the first ack by design, while this one must collect every literal
// collection name and both hooks, and so has to keep going until the caps.
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
// project. Cycles are bounded by a visited set keyed on the RESOLVED path.
func scanForStorageUse(dir string) storageScanResult {
	s := &storageScanner{
		visited: map[string]bool{},
		res: storageScanResult{
			literalNames: map[string]string{},
			importNames:  map[string]map[string]string{},
			legacyHooks:  map[string]string{},
		},
	}
	s.walk(dir, "")
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

// walk reads dir, whose path relative to the project root is rel ("" for the
// root itself). rel is what a finding names, so it follows the path as the
// author sees it — through a symlink, not to its target.
func (s *storageScanner) walk(dir, rel string) {
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
		relPath := e.Name()
		if rel != "" {
			relPath = rel + "/" + e.Name()
		}
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
			s.walk(path, relPath)
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !readyAckSourceExts[ext] || isTestFile(relPath) {
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
		// scaffolds DOCUMENT these calls in prose, and a comment naming one is
		// not a call.
		s.record(relPath, blockproto.StripCommentsForExt(string(data), ext))
	}
}

// record notes every storage call in one file's comment-stripped code.
func (s *storageScanner) record(rel, code string) {
	for _, m := range docStoreCallRe.FindAllStringIndex(code, -1) {
		if s.res.docStoreFile == "" {
			s.res.docStoreFile = rel
		}
		name, end, ok := literalArgEnd(code, m[1])
		if !ok {
			continue
		}
		switch from, kind := importSourceArg(code, end); kind {
		case noFrom:
			if _, seen := s.res.literalNames[name]; !seen {
				s.res.literalNames[name] = rel
			}
		case literalFrom:
			if s.res.importNames[from] == nil {
				s.res.importNames[from] = map[string]string{}
			}
			if _, seen := s.res.importNames[from][name]; !seen {
				s.res.importNames[from][name] = rel
			}
		case computedFrom:
			// Another app's collection, but which app is unknowable here. NOT
			// recorded as an own collection: it is not one.
		}
	}
	for _, m := range legacyHookCallRe.FindAllStringSubmatch(code, -1) {
		if _, seen := s.res.legacyHooks[m[1]]; !seen {
			s.res.legacyHooks[m[1]] = rel
		}
	}
}
