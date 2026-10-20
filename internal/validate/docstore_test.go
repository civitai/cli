package validate

import (
	"sort"
	"strings"
	"testing"
)

// docstore_test.go pins `civitai app validate`'s understanding of the DOCUMENT
// STORE: the manifest's `storage.collections` declaration, the scopes it needs,
// and the `app.storage.collection(<name>)` calls in an app's source.
//
// Two halves, graded differently on purpose (see storagemanifest.go and
// storagescope.go):
//
//   - The MANIFEST rules are HARD ERRORS. Each one mirrors a refusal the
//     server's `parseManifestStorage` / `appStorageScopeErrors` (civitai
//     `src/shared/constants/block-storage.constants.ts`) returns at submit, which
//     the vendored JSON Schema cannot express. Every message is pinned WHOLE,
//     because it is the server's text and a reword is a mirror drift.
//   - The SOURCE rules are ADVISORY WARNINGS, like every other check in this
//     package that reads src/: they infer runtime behaviour from static text.
//
// 🔴 EVERY REJECTION ROW CARRIES AN ACCEPT TWIN that differs only in the defect,
// so a check that fired unconditionally cannot pass the table.

// docManifest is a manifest that is clean apart from what `storage` and
// `scopes` make of it. Sensitive scopes get a justification automatically, so a
// sensitive-scope finding never stands in for the one under test. storage == ""
// omits the key.
func docManifest(storage string, scopes ...string) string {
	quoted := make([]string, 0, len(scopes))
	var just []string
	for _, s := range scopes {
		quoted = append(quoted, `"`+s+`"`)
		if isSensitiveBlockScope(s) {
			just = append(just, `"`+s+`":"viewers publish entries every viewer reads"`)
		}
	}
	m := `{"blockId":"docstore-fixture","name":"Doc Store Fixture","version":"1.0.0","contentRating":"g",` +
		`"scopes":[` + strings.Join(quoted, ",") + `],`
	if len(just) > 0 {
		m += `"scopeJustifications":{` + strings.Join(just, ",") + `},`
	}
	if storage != "" {
		m += `"storage":` + storage + `,`
	}
	return m + `"iframe":{"sandbox":"allow-scripts","minHeight":400,"resizable":false}}`
}

// oneCollection wraps a single collection declaration as a `storage` value.
func oneCollection(name, decl string) string {
	return `{"collections":{"` + name + `":` + decl + `}}`
}

func describe(fs []Finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString("\n  [" + f.Field + "] " + f.Message)
	}
	return b.String()
}

// assertOnlyError requires the manifest to produce EXACTLY one error, with this
// field and this whole message.
func assertOnlyError(t *testing.T, manifest, field, message string) {
	t.Helper()
	fs := manifestOnlyFindings(t, manifest)
	if len(fs) != 1 {
		t.Fatalf("want exactly 1 error, got %d:%s", len(fs), describe(fs))
	}
	if fs[0].Message != message {
		t.Errorf("message drifted from the server's text\n  got:  %s\n  want: %s", fs[0].Message, message)
	}
	if fs[0].Field != field {
		t.Errorf("field = %q, want %q", fs[0].Field, field)
	}
}

func assertNoErrors(t *testing.T, label, manifest string) {
	t.Helper()
	if fs := manifestOnlyFindings(t, manifest); len(fs) != 0 {
		t.Errorf("%s: want no errors, got %d:%s", label, len(fs), describe(fs))
	}
}

// TestStorageRefusesFeaturesNotYetAvailable — the five features the schema
// ADMITS (it carries the whole declaration shape) and the server refuses today
// through APP_STORAGE_ENABLED_FEATURES. Before this check every one of them
// validated clean offline and was refused at submit.
func TestStorageRefusesFeaturesNotYetAvailable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		collection string // the collection's name
		decl       string // refused declaration
		twin       string // the same collection without the refused feature
		scopes     []string
		field      string
		message    string
	}{
		{
			name: "kind log", collection: "chat_log",
			decl:   `{"visibility":"private","kind":"log"}`,
			twin:   `{"visibility":"private","kind":"docs"}`,
			scopes: []string{"apps:storage"},
			field:  "storage.collections.chat_log.kind",
			message: `storage.collections.chat_log.kind "log" is not available yet — append-only log ` +
				`collections arrive in a later phase of App Storage; declare kind "docs" (or omit kind) for now`,
		},
		{
			name: "write anyone", collection: "wiki_pages",
			decl:   `{"visibility":"shared","write":"anyone"}`,
			twin:   `{"visibility":"shared","write":"own"}`,
			scopes: []string{"apps:storage", "apps:storage:publish"},
			field:  "storage.collections.wiki_pages.write",
			message: `storage.collections.wiki_pages.write "anyone" is not available yet — documents any ` +
				`viewer may edit arrive in a later phase of App Storage; use "own" or "none" for now`,
		},
		{
			name: "write owner", collection: "staff_picks",
			decl:   `{"visibility":"shared","write":"owner","marks":["star"]}`,
			twin:   `{"visibility":"shared","write":"none","marks":["star"]}`,
			scopes: []string{"apps:storage"},
			field:  "storage.collections.staff_picks.write",
			message: `storage.collections.staff_picks.write "owner" is not available yet — owner-written ` +
				`documents arrive in a later phase of App Storage; use "own" or "none" for now`,
		},
		{
			name: "ttlSeconds", collection: "drafts",
			decl:   `{"visibility":"private","ttlSeconds":86400}`,
			twin:   `{"visibility":"private"}`,
			scopes: []string{"apps:storage"},
			field:  "storage.collections.drafts.ttlSeconds",
			message: `storage.collections.drafts.ttlSeconds is not available yet — expiring documents ` +
				`arrive in a later phase of App Storage; remove ttlSeconds for now`,
		},
		{
			name: "dailyWritesPerAuthor", collection: "entries",
			decl:   `{"visibility":"shared","write":"own","dailyWritesPerAuthor":25}`,
			twin:   `{"visibility":"shared","write":"own"}`,
			scopes: []string{"apps:storage", "apps:storage:publish"},
			field:  "storage.collections.entries.dailyWritesPerAuthor",
			message: `storage.collections.entries.dailyWritesPerAuthor is not available yet — ` +
				`per-collection write budgets arrive in a later phase of App Storage; remove ` +
				`dailyWritesPerAuthor for now (the platform default applies)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertNoErrors(t, "accept twin", docManifest(oneCollection(tc.collection, tc.twin), tc.scopes...))
			assertOnlyError(t, docManifest(oneCollection(tc.collection, tc.decl), tc.scopes...), tc.field, tc.message)
		})
	}
}

// TestStorageFeatureGateOrderMirrorsTheServer — the server checks a value's
// RANGE (or applicability) before the feature switch, so an out-of-range value
// gets the range error and never the "not available yet" one. Here the range
// error is the schema's; the gate must stay quiet so the author is not told two
// stories about one key.
func TestStorageFeatureGateOrderMirrorsTheServer(t *testing.T) {
	for _, tc := range []struct{ name, decl string }{
		{"ttlSeconds below the minimum", `{"visibility":"private","ttlSeconds":59}`},
		{"dailyWritesPerAuthor on write none", `{"visibility":"shared","write":"none","counters":["plays"],"dailyWritesPerAuthor":5}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := manifestOnlyFindings(t, docManifest(oneCollection("tallies", tc.decl), "apps:storage"))
			if len(fs) == 0 {
				t.Fatal("the schema accepted it — this row no longer exercises the ordering")
			}
			for _, f := range fs {
				if strings.Contains(f.Message, "is not available yet") {
					t.Errorf("feature gate fired on a value the server rejects for another reason first: %s", f.Message)
				}
			}
		})
	}
}

// TestStorageCounterAndMarkMayNotShareAName — a cross-array rule JSON Schema
// cannot express (each list is `uniqueItems` on its own).
func TestStorageCounterAndMarkMayNotShareAName(t *testing.T) {
	const name = "popular_plays"
	assertNoErrors(t, "accept twin", docManifest(oneCollection(name,
		`{"visibility":"shared","write":"none","counters":["plays","likes"],"marks":["liked"]}`), "apps:storage"))
	assertOnlyError(t,
		docManifest(oneCollection(name,
			`{"visibility":"shared","write":"none","counters":["plays","likes"],"marks":["likes"]}`), "apps:storage"),
		"storage.collections.popular_plays",
		`storage.collections.popular_plays declares "likes" as both a counter and a mark — use different names`)
}

// TestStorageScopeRules — the four cross-field rules between `storage` and
// `scopes` (appStorageScopeErrors). Each finding names the side that is
// MISSING (AGENTS.md item 23): a declaration with no scope is a `scopes`
// finding, a scope with nothing to reach is a `storage` finding.
func TestStorageScopeRules(t *testing.T) {
	t.Run("collections declared without apps:storage", func(t *testing.T) {
		const st = `{"collections":{"kv":{"visibility":"private"},"drafts":{"visibility":"private"}}}`
		assertNoErrors(t, "accept twin", docManifest(st, "user:read:self", "apps:storage"))
		// Names are listed SORTED: Go's decoded map has no key order, so the
		// server's declaration order cannot be reproduced. Same names, same rule.
		assertOnlyError(t, docManifest(st, "user:read:self"), "scopes",
			`storage declares drafts, kv but scopes does not include apps:storage — add it, or remove storage`)
	})
	t.Run("viewer-written shared collection without apps:storage:publish", func(t *testing.T) {
		st := oneCollection("guestbook", `{"visibility":"shared","write":"own"}`)
		assertNoErrors(t, "accept twin", docManifest(st, "apps:storage", "apps:storage:publish"))
		assertOnlyError(t, docManifest(st, "apps:storage"), "scopes",
			`storage collection guestbook lets viewers write documents other users read, which requires `+
				`the apps:storage:publish scope — add it to scopes with a scopeJustifications entry`)
	})
	t.Run("apps:storage with no collections", func(t *testing.T) {
		assertNoErrors(t, "accept twin", docManifest("", "user:read:self"))
		assertOnlyError(t, docManifest("", "user:read:self", "apps:storage"), "storage",
			`scopes includes apps:storage but storage declares no collections — declare them under `+
				`storage.collections, or remove the scope`)
	})
	t.Run("apps:storage:publish with no viewer-written collection", func(t *testing.T) {
		st := oneCollection("vote_tally", `{"visibility":"shared","write":"none","counters":["votes"]}`)
		assertNoErrors(t, "accept twin", docManifest(st, "apps:storage"))
		assertOnlyError(t, docManifest(st, "apps:storage", "apps:storage:publish"), "storage",
			`scopes includes apps:storage:publish but no storage collection is shared with write "own" — `+
				`remove the scope, or declare the collection it is for`)
	})
}

// TestStorageScopeRulesWaitForAValidDeclaration — the server evaluates the scope
// rules only when `storage` itself parsed, because an invalid declaration
// declares nothing and a scope rule against it tells a second, misleading
// story. Both arms: a refusal from the feature gate, and one from the schema.
func TestStorageScopeRulesWaitForAValidDeclaration(t *testing.T) {
	for _, tc := range []struct{ name, storage string }{
		{"declaration refused by the feature gate", oneCollection("chat_log", `{"visibility":"private","kind":"log"}`)},
		{"declaration refused by the schema", oneCollection("kv", `{"write":"own"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := manifestOnlyFindings(t, docManifest(tc.storage, "user:read:self"))
			if len(fs) == 0 {
				t.Fatal("no error at all — the declaration is not invalid, so this row measures nothing")
			}
			for _, f := range fs {
				if strings.Contains(f.Message, "scopes does not include apps:storage") {
					t.Errorf("scope rule evaluated against an invalid declaration: %s", f.Message)
				}
			}
		})
	}
	// CONTROL: the same scopes with a VALID declaration do produce the scope
	// rule, so the silence above is the gate's and not a rule that never fires.
	fs := manifestOnlyFindings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "user:read:self"))
	var hit bool
	for _, f := range fs {
		hit = hit || strings.Contains(f.Message, "scopes does not include apps:storage")
	}
	if !hit {
		t.Fatalf("control: a valid declaration without apps:storage produced no scope finding:%s", describe(fs))
	}
}

// playableManifest is the playable-collections app's real declaration: a
// per-viewer `kv` and a counters-and-marks `popular_plays`, under `apps:storage`
// alone (no viewer authors a document, so no publish scope).
func playableManifest() string {
	return docManifest(`{"collections":{`+
		`"kv":{"visibility":"private"},`+
		`"popular_plays":{"visibility":"shared","write":"none","counters":["plays"],"marks":["played"]}}}`,
		"user:read:self", "apps:storage")
}

// storageSourceWarnings returns the warnings this package's source-reading
// storage checks produced, i.e. everything except other checks' advisories
// (none are expected on these non-page fixtures, but filtering keeps a stray one
// from standing in for the finding under test). It fails on any hard error: a
// fixture whose manifest is not valid measures nothing.
func storageSourceWarnings(t *testing.T, manifest string, files map[string]string) []Finding {
	t.Helper()
	res, err := Dir(storageProject(t, manifest, files))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("fixture manifest must be VALID or the test measures nothing:%s", describe(res.Errors))
	}
	var out []Finding
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "collection") || strings.Contains(w.Message, "Storage") ||
			strings.Contains(w.Message, "storage") {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Message < out[j].Message })
	return out
}

func undeclaredMessage(file, name string) string {
	return file + " calls `storage.collection(\"" + name + "\")` but storage.collections does not declare \"" +
		name + "\" — a collection the manifest does not declare does not exist at runtime, so every call on " +
		"it is refused with `collection_not_declared`. Declare it under storage.collections, or correct the " +
		"name. ADVISORY: this reads source statically, so only string-literal names are checked."
}

// TestDocStoreUndeclaredCollectionName — a string-literal collection name that
// the manifest does not declare.
//
// 🔴 The undeclared name (`quest_ledger_q7`) appears NOWHERE but in this
// fixture's source — not in the manifest, not in the package — so a check that
// hardcoded a name, or echoed a manifest key, cannot produce this message. The
// declared `kv` call beside it is the accept twin in the same file.
func TestDocStoreUndeclaredCollectionName(t *testing.T) {
	got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
		map[string]string{
			"src/quests.ts": "import { app } from './sdk';\n" +
				"export const prefs = () => app.storage.collection('kv').get('prefs');\n" +
				"export const ledger = () => app.storage.collection('quest_ledger_q7').list();\n",
		})
	if len(got) != 1 {
		t.Fatalf("want exactly the undeclared-name warning, got %d:%s", len(got), describe(got))
	}
	if want := undeclaredMessage("src/quests.ts", "quest_ledger_q7"); got[0].Message != want {
		t.Errorf("message\n  got:  %s\n  want: %s", got[0].Message, want)
	}
	if got[0].Field != "storage.collections.quest_ledger_q7" {
		t.Errorf("field = %q, want the undeclared key storage.collections.quest_ledger_q7", got[0].Field)
	}
}

// TestDocStoreCallShapes — the spellings a real call takes. Every row's name is
// undeclared, so each must be FOUND; a row the scanner misses is a silent miss.
func TestDocStoreCallShapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"double quotes", `app.storage.collection("hi_scores").list();`, "hi_scores"},
		{"TS type argument", `app.storage.collection<Score>('hi_scores_typed').list();`, "hi_scores_typed"},
		{"plain template literal", "app.storage.collection(`hi_scores_tpl`).list();", "hi_scores_tpl"},
		{"chained across lines", "fake.storage\n  .collection('hi_scores_chain')\n  .list();", "hi_scores_chain"},
		{"awaited client", `(await app()).storage.collection('hi_scores_await').list();`, "hi_scores_await"},
		{"optional chaining", `app?.storage?.collection('hi_scores_opt').list();`, "hi_scores_opt"},
		{"a renamed storage client", `const docStorage = app.storage; docStorage.collection('hi_scores_alias');`, "hi_scores_alias"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
				map[string]string{"src/scores.ts": tc.src})
			if len(got) != 1 || got[0].Message != undeclaredMessage("src/scores.ts", tc.want) {
				t.Errorf("want one undeclared-name warning for %q, got %d:%s", tc.want, len(got), describe(got))
			}
		})
	}
}

// TestDocStoreDynamicNamesAreNotFlagged — only a STRING LITERAL is a claim about
// a name. Anything computed is unknowable statically and must stay silent.
// Also: a `.collection(` on something that is not the storage client (the
// playable-collections app's own `PATHS.collection(id)` and
// `VIEW_PREF_KEYS.collection(101)`) is not a document-store call at all.
func TestDocStoreDynamicNamesAreNotFlagged(t *testing.T) {
	src := "export const a = (name: string) => app.storage.collection(name).list();\n" +
		"export const b = (id: number) => app.storage.collection(`run_${id}`).list();\n" +
		"export const c = (x: string) => app.storage.collection('pre_' + x).list();\n" +
		"export const d = (id: number) => request(PATHS.collection(id));\n" +
		"export const e = () => VIEW_PREF_KEYS.collection(101);\n" +
		"export const f = () => api.collection('not_the_store');\n"
	got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
		map[string]string{"src/dyn.ts": src})
	if len(got) != 0 {
		t.Errorf("want no warning for computed names or non-storage receivers, got %d:%s", len(got), describe(got))
	}
	// CONTROL: the same file plus ONE literal undeclared call is found, so the
	// silence above is the dynamic-name rule's and not a scanner that read nothing.
	got = storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
		map[string]string{"src/dyn.ts": src + "export const g = () => app.storage.collection('dyn_control_z3');\n"})
	if len(got) != 1 || !strings.Contains(got[0].Message, "dyn_control_z3") {
		t.Errorf("control: want one warning naming dyn_control_z3, got %d:%s", len(got), describe(got))
	}
}

// TestDocStoreIgnoresTestFiles — a test file exercises fakes, ledgers and
// deliberately-wrong names; none of it is the app's runtime. The incident: the
// hook-name matcher this replaced flagged `src/scope-contract.test.ts` in the
// playable-collections app, an object whose string VALUES name hooks.
func TestDocStoreIgnoresTestFiles(t *testing.T) {
	body := "const fake = createFakeStorage();\n" +
		"export const x = () => fake.storage.collection('ghost_tbl').list();\n" +
		"export const y = () => useAppStorage();\n"
	for _, path := range []string{"src/ghost.test.ts", "src/ghost.spec.tsx", "src/__tests__/ghost.ts"} {
		t.Run(path, func(t *testing.T) {
			// An ordinary source file beside it, so the tree scan reads SOMETHING:
			// with the test file alone it would read nothing, go unobservable and
			// stay silent for that reason instead — green with the rule deleted.
			got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
				map[string]string{path: body, "src/main.ts": "export const x = 1;\n"})
			if len(got) != 0 {
				t.Errorf("a test file produced warnings:%s", describe(got))
			}
		})
	}
	// TWIN: the same body in an ordinary source file IS reported (both the
	// undeclared name and the legacy hook), so the silence above is the
	// test-file rule's.
	got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
		map[string]string{"src/ghost.ts": body})
	if len(got) != 2 {
		t.Errorf("twin: want 2 warnings from the same body outside a test file, got %d:%s", len(got), describe(got))
	}
}

// TestDocStorePlayableCollectionsShape — the real app this check exists for,
// reduced to the shapes that matter: both declared collections called by
// literal, a dynamic call through a helper, the app's own non-storage
// `.collection(` helpers, and the scope-contract ledger test whose string
// values name the legacy hooks. Nothing here is a defect; any finding is a false
// positive.
func TestDocStorePlayableCollectionsShape(t *testing.T) {
	got := storageSourceWarnings(t, playableManifest(), map[string]string{
		"src/lib/kv-store.ts": "export const prefs = () => app.storage.collection('kv').get('playable-collections:browse-prefs');\n",
		"src/lib/popular.ts": "export const POPULAR_PLAYS_COLLECTION = 'popular_plays';\n" +
			"export const bump = (key: string) => app.storage.collection('popular_plays').increment(key, 'plays');\n",
		"src/lib/sdk-runtime.ts": "export const store = (name: string) => ({\n" +
			"  list: async (q?: unknown) => (await app()).storage.collection(name).list(q),\n" +
			"});\n",
		"src/lib/api.ts":    "export const page = (id: number) => request(PATHS.collection(id));\n",
		"src/view-modes.ts": "export const raw = (id: number) => safeGet(storage, VIEW_PREF_KEYS.collection(id));\n",
		"src/scope-contract.test.ts": "const LEDGER = { appStorage: 'useAppStorage', shared: 'useSharedStorage' };\n" +
			"test('ledger', () => { expect(fake.storage.collection('kv')).toBeDefined(); useAppStorage(); });\n",
	})
	if len(got) != 0 {
		t.Errorf("the playable-collections shape is correct, got %d warnings:%s", len(got), describe(got))
	}
}

func noScopeMessage(file string) string {
	return file + " calls the document store (`storage.collection()`) but scopes does not include " +
		"apps:storage and the manifest declares no storage — every document-store call is refused in " +
		"production with `insufficient_scope`. Add apps:storage to scopes and declare each collection " +
		"under storage.collections. ADVISORY: this reads source statically."
}

// TestDocStoreUseWithoutTheScope — source on the document store, manifest with
// neither `apps:storage` nor a `storage` declaration. A DYNAMIC call counts here
// (the scope is needed whatever the name), which is what separates this check
// from the undeclared-name one: the row below has no literal at all.
func TestDocStoreUseWithoutTheScope(t *testing.T) {
	files := map[string]string{"src/board.ts": "export const read = (n: string) => app.storage.collection(n).list();\n"}
	got := storageSourceWarnings(t, docManifest("", "user:read:self"), files)
	if len(got) != 1 || got[0].Message != noScopeMessage("src/board.ts") {
		t.Fatalf("want exactly the no-scope warning, got %d:%s", len(got), describe(got))
	}
	if got[0].Field != "scopes" {
		t.Errorf("field = %q, want \"scopes\" — the source is right, the manifest is missing the scope", got[0].Field)
	}
	// TWIN: the same source under a manifest that declares a collection and the
	// scope is silent.
	if got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"), files); len(got) != 0 {
		t.Errorf("twin: want no warning with apps:storage declared, got %d:%s", len(got), describe(got))
	}
}

func legacyMessage(file, hook string) string {
	return file + " calls `" + hook + "()` — the legacy storage hooks (useAppStorage, useSharedStorage) " +
		"are deprecated and their routes are being retired; move this data to the document store: " +
		"declare a collection under storage.collections, add apps:storage to scopes, and use " +
		"`app.storage.collection()` from @civitai/sdk. ADVISORY: this reads source statically."
}
