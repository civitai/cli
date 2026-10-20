package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	cli "github.com/civitai/cli"
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

// manifestOnlyResult is manifestOnlyFindings with the WARNINGS kept too: the
// feature gates below are advisories, so the errors alone cannot see them.
func manifestOnlyResult(t *testing.T, body string) Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	res, err := ManifestOnly(dir)
	if err != nil {
		t.Fatalf("ManifestOnly: %v", err)
	}
	return res
}

// gateWarnings returns the "not available yet" advisories among fs.
func gateWarnings(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if strings.Contains(f.Message, "is not available yet") {
			out = append(out, f)
		}
	}
	return out
}

// featureGateSuffix is what the CLI adds to the server's own text: that the
// server refuses the feature TODAY, and why this is a warning anyway.
const featureGateSuffix = " civitai.com refuses this at submit today; it is a warning here, not an " +
	"error, because the platform enables these features with a server-side switch and a CLI already " +
	"installed must not refuse a manifest the server has started to accept."

// TestStorageFeaturesNotYetAvailableAreWarnings — the five features the schema
// ADMITS (it carries the whole declaration shape) and the server refuses today
// through APP_STORAGE_ENABLED_FEATURES.
//
// 🔴 WARNINGS, NOT ERRORS. `app submit` blocks on validate ERRORS, and the server
// turns each of these on with a one-line switch. A hard error baked into a CLI
// that has already shipped would then refuse a manifest the server accepts,
// until every author upgrades. The warning still carries the server's text, so
// an author learns today that submit will refuse it.
//
// Every row also asserts NO error, so a gate that stayed an error fails here.
func TestStorageFeaturesNotYetAvailableAreWarnings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		collection string // the collection's name
		decl       string // gated declaration
		twin       string // the same collection without the gated feature
		scopes     []string
		field      string
		server     string // the server's own message, verbatim
	}{
		{
			name: "kind log", collection: "chat_log",
			decl:   `{"visibility":"private","kind":"log"}`,
			twin:   `{"visibility":"private","kind":"docs"}`,
			scopes: []string{"apps:storage"},
			field:  "storage.collections.chat_log.kind",
			server: `storage.collections.chat_log.kind "log" is not available yet — append-only log ` +
				`collections arrive in a later phase of App Storage; declare kind "docs" (or omit kind) for now.`,
		},
		{
			name: "write anyone", collection: "wiki_pages",
			decl:   `{"visibility":"shared","write":"anyone"}`,
			twin:   `{"visibility":"shared","write":"own"}`,
			scopes: []string{"apps:storage", "apps:storage:publish"},
			field:  "storage.collections.wiki_pages.write",
			server: `storage.collections.wiki_pages.write "anyone" is not available yet — documents any ` +
				`viewer may edit arrive in a later phase of App Storage; use "own" or "none" for now.`,
		},
		{
			name: "write owner", collection: "staff_picks",
			decl:   `{"visibility":"shared","write":"owner","marks":["star"]}`,
			twin:   `{"visibility":"shared","write":"none","marks":["star"]}`,
			scopes: []string{"apps:storage"},
			field:  "storage.collections.staff_picks.write",
			server: `storage.collections.staff_picks.write "owner" is not available yet — owner-written ` +
				`documents arrive in a later phase of App Storage; use "own" or "none" for now.`,
		},
		{
			name: "ttlSeconds", collection: "drafts",
			decl:   `{"visibility":"private","ttlSeconds":86400}`,
			twin:   `{"visibility":"private"}`,
			scopes: []string{"apps:storage"},
			field:  "storage.collections.drafts.ttlSeconds",
			server: `storage.collections.drafts.ttlSeconds is not available yet — expiring documents ` +
				`arrive in a later phase of App Storage; remove ttlSeconds for now.`,
		},
		{
			name: "dailyWritesPerAuthor", collection: "entries",
			decl:   `{"visibility":"shared","write":"own","dailyWritesPerAuthor":25}`,
			twin:   `{"visibility":"shared","write":"own"}`,
			scopes: []string{"apps:storage", "apps:storage:publish"},
			field:  "storage.collections.entries.dailyWritesPerAuthor",
			server: `storage.collections.entries.dailyWritesPerAuthor is not available yet — ` +
				`per-collection write budgets arrive in a later phase of App Storage; remove ` +
				`dailyWritesPerAuthor for now (the platform default applies).`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			twin := manifestOnlyResult(t, docManifest(oneCollection(tc.collection, tc.twin), tc.scopes...))
			if len(twin.Errors) != 0 || len(gateWarnings(twin.Warnings)) != 0 {
				t.Errorf("accept twin: want nothing, got errors:%s\n warnings:%s",
					describe(twin.Errors), describe(gateWarnings(twin.Warnings)))
			}
			res := manifestOnlyResult(t, docManifest(oneCollection(tc.collection, tc.decl), tc.scopes...))
			if len(res.Errors) != 0 {
				t.Errorf("a gated feature must not be an ERROR (submit blocks on errors), got:%s", describe(res.Errors))
			}
			got := gateWarnings(res.Warnings)
			if len(got) != 1 {
				t.Fatalf("want exactly 1 feature-gate warning, got %d:%s", len(got), describe(res.Warnings))
			}
			if want := tc.server + featureGateSuffix; got[0].Message != want {
				t.Errorf("message drifted\n  got:  %s\n  want: %s", got[0].Message, want)
			}
			if got[0].Field != tc.field {
				t.Errorf("field = %q, want %q", got[0].Field, tc.field)
			}
		})
	}
}

// TestStorageScopeRulesRunPastAFeatureGateWarning — the deliberate consequence
// of the gates being warnings. The server evaluates the scope rules only once
// the declaration parsed; a gated feature fails that parse TODAY, so today's
// server reports the gate alone. Here the gate is advisory, and the scope rules
// are judged as the server will judge them the moment the switch is on — so an
// author who passes validate with a gated feature is not then refused for a
// missing scope that validate never mentioned.
//
// A manifest with no gated feature is unaffected: it drew no gate finding before
// and draws none now, so the scope rules' gating (any ERROR under `storage`) is
// exactly what it was.
func TestStorageScopeRulesRunPastAFeatureGateWarning(t *testing.T) {
	for _, tc := range []struct {
		name, storage string
		scopes        []string
		field, msg    string
	}{
		{
			name:    "kind log without apps:storage",
			storage: oneCollection("chat_log", `{"visibility":"private","kind":"log"}`),
			scopes:  []string{"user:read:self"},
			field:   "scopes",
			msg:     `storage declares chat_log but scopes does not include apps:storage — add it, or remove storage`,
		},
		{
			name:    "write anyone without apps:storage:publish",
			storage: oneCollection("wiki_pages", `{"visibility":"shared","write":"anyone"}`),
			scopes:  []string{"apps:storage"},
			field:   "scopes",
			msg: `storage collection wiki_pages lets viewers write documents other users read, which requires ` +
				`the apps:storage:publish scope — add it to scopes with a scopeJustifications entry`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := manifestOnlyResult(t, docManifest(tc.storage, tc.scopes...))
			if len(res.Errors) != 1 || res.Errors[0].Message != tc.msg || res.Errors[0].Field != tc.field {
				t.Errorf("want exactly the scope rule [%s] %s, got %d:%s", tc.field, tc.msg, len(res.Errors), describe(res.Errors))
			}
			if len(gateWarnings(res.Warnings)) != 1 {
				t.Errorf("want the feature-gate warning beside it, got:%s", describe(res.Warnings))
			}
		})
	}
}

// TestVendoredSchemaStillStatesTheFeatureGates is a DRIFT guard on the vendored
// canonical schema, the one place this repository learns which storage features
// the server refuses. The weekly re-vendor copies the server's schema in; when
// the server enables a feature its description changes, this goes red on that
// PR, and whoever lands it removes the matching warning in storagemanifest.go
// in the same change.
//
// Every sentence is pinned WHOLE (whitespace-normalised): the claim is
// machine-read, so a reword is a change to re-read, not noise to tolerate.
func TestVendoredSchemaStillStatesTheFeatureGates(t *testing.T) {
	var doc struct {
		Properties struct {
			Storage struct {
				Description string `json:"description"`
				Properties  struct {
					Collections struct {
						PatternProperties map[string]struct {
							Properties map[string]struct {
								Description string `json:"description"`
							} `json:"properties"`
						} `json:"patternProperties"`
					} `json:"collections"`
				} `json:"properties"`
			} `json:"storage"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		t.Fatalf("vendored schema: %v", err)
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	coll, ok := doc.Properties.Storage.Properties.Collections.PatternProperties[`^[a-z][a-z0-9_]{0,31}$`]
	if !ok {
		t.Fatal("storage.collections no longer has the collection-name patternProperties this guard reads")
	}
	prop := func(name string) string { return norm(coll.Properties[name].Description) }

	for _, tc := range []struct{ where, text, sentence string }{
		{"storage", norm(doc.Properties.Storage.Description),
			"This schema describes the WHOLE declaration, including features the platform does not " +
				"accept yet: today the server's manifest validation refuses `kind` \"log\", `ttlSeconds`, " +
				"`dailyWritesPerAuthor`, `write` \"anyone\" and `write` \"owner\" with a message saying they " +
				"are not available yet, so those validate offline and are rejected at submit."},
		{"kind", prop("kind"),
			"RESERVED: civitai.com refuses \"log\" until log collections are enabled, so declare \"docs\" (or omit `kind`)."},
		{"write (anyone)", prop("write"),
			"\"anyone\": eligible signed-in viewers may update or delete any document — RESERVED: " +
				"civitai.com refuses \"anyone\" until it is enabled."},
		{"write (owner)", prop("write"),
			"RESERVED: civitai.com refuses \"owner\" until owner-written documents are enabled."},
		{"write (available)", prop("write"), "Available today: \"own\" and \"none\"."},
		{"ttlSeconds", prop("ttlSeconds"),
			"RESERVED — civitai.com refuses this field until expiring documents are enabled, so do not declare it yet."},
		{"dailyWritesPerAuthor", prop("dailyWritesPerAuthor"),
			"RESERVED — civitai.com refuses this field until per-collection write budgets are enabled, so do " +
				"not declare it yet."},
	} {
		if !strings.Contains(tc.text, tc.sentence) {
			t.Errorf("the vendored schema's %s description no longer says:\n  %s\n"+
				"If the server now ACCEPTS that feature, remove its warning from storageFeatureGateWarnings "+
				"(storagemanifest.go) and update this guard in the same change.\n  description now: %s",
				tc.where, tc.sentence, tc.text)
		}
	}
}

// TestStorageFeatureGateOrderMirrorsTheServer — the server checks a value's
// RANGE (or applicability) before the feature switch, so an out-of-range value
// gets the range error and never the "not available yet" one. Here the range
// error is the schema's; the gate must stay quiet so the author is not told two
// stories about one key. The gate is a WARNING, so both lists are read.
func TestStorageFeatureGateOrderMirrorsTheServer(t *testing.T) {
	for _, tc := range []struct{ name, decl string }{
		{"ttlSeconds below the minimum", `{"visibility":"private","ttlSeconds":59}`},
		{"dailyWritesPerAuthor on write none", `{"visibility":"shared","write":"none","counters":["plays"],"dailyWritesPerAuthor":5}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := manifestOnlyResult(t, docManifest(oneCollection("tallies", tc.decl), "apps:storage"))
			if len(res.Errors) == 0 {
				t.Fatal("the schema accepted it — this row no longer exercises the ordering")
			}
			for _, f := range append(append([]Finding{}, res.Errors...), res.Warnings...) {
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

// TestStorageCounterMarkCollisionSkipsListsTheServerRejects — the server's
// parseTallyNames discards a WHOLE counters/marks list as soon as one entry is
// unusable (not a string, not snake_case, the built-in `report`, a duplicate,
// or more than 8), so its collision check compares against an empty list and
// stays silent. The schema reports each of those defects; a collision finding
// on top of it is a second story the server never tells. Every row carries the
// collision (`likes` in both lists) AND one defect that voids a list.
func TestStorageCounterMarkCollisionSkipsListsTheServerRejects(t *testing.T) {
	nine := `"likes","a1","a2","a3","a4","a5","a6","a7","a8"`
	for _, tc := range []struct{ name, counters, marks string }{
		{"counters name the built-in report", `"likes","report"`, `"likes"`},
		{"marks declare the built-in report", `"likes"`, `"likes","report"`},
		{"a counter that is not snake_case", `"likes","Bad-Name"`, `"likes"`},
		{"a mark that is not a string", `"likes"`, `"likes",5`},
		{"a duplicated counter", `"likes","likes"`, `"likes"`},
		{"more than 8 counters", nine, `"likes"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decl := `{"visibility":"shared","write":"none","counters":[` + tc.counters + `],"marks":[` + tc.marks + `]}`
			fs := manifestOnlyFindings(t, docManifest(oneCollection("tallies", decl), "apps:storage"))
			if len(fs) == 0 {
				t.Fatal("no error at all — the list is not invalid, so this row measures nothing")
			}
			for _, f := range fs {
				if strings.Contains(f.Message, "as both a counter and a mark") {
					t.Errorf("collision reported on a list the server discards: %s", f.Message)
				}
			}
		})
	}
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
// story. Both arms: a refusal from this package's own imperative rules (the
// counter/mark collision), and one from the schema. A feature-gate WARNING is
// not a refusal here — see TestStorageScopeRulesRunPastAFeatureGateWarning.
func TestStorageScopeRulesWaitForAValidDeclaration(t *testing.T) {
	for _, tc := range []struct{ name, storage string }{
		{"declaration refused by the counter/mark collision",
			oneCollection("tallies", `{"visibility":"shared","write":"none","counters":["n"],"marks":["n"]}`)},
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
		// The DOMINANT real shape (every forgejo app's docstore port, and
		// playable-collections' src/lib/kv-store.ts): the client is the result
		// of an awaited call, so no receiver name ends in "storage".
		{"awaited client call as the receiver", `(await docs()).collection('hi_scores_docs').list();`, "hi_scores_docs"},
		{"awaited client call, typed", `const kv = async () => (await docs()).collection<KvDocument>('hi_scores_docs_t');`, "hi_scores_docs_t"},
		{"awaited client call, optional chaining", "(await getStore())\n  ?.collection('hi_scores_docs_opt');", "hi_scores_docs_opt"},
		// NESTED type arguments, one and two levels deep.
		{"nested type argument", `app.storage.collection<Record<string, Row>>('hi_scores_nested').list();`, "hi_scores_nested"},
		{"two-level nested type argument", `app.storage.collection<Map<string, Array<Row>>>('hi_scores_nested2');`, "hi_scores_nested2"},
		{"nested type argument on an awaited client", `(await docs()).collection<Record<string, unknown>>('hi_scores_nested_docs');`, "hi_scores_nested_docs"},
		{"object type argument", `(await docs()).collection<{ value: unknown }>('hi_scores_obj');`, "hi_scores_obj"},
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
		"export const f = () => api.collection('not_the_store');\n" +
		"export const h = () => (await fetchPage(id)).collection('not_the_store_args');\n" +
		"export const i = () => getApi().collection('not_the_store_unawaited');\n"
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

// TestDocStoreUseWithoutTheScopeRealShape — the no-scope advisory on the shape
// the real apps use: a constant name on an awaited client call, with a nested
// type argument. Before the call-result receiver was matched, an app written
// this way that dropped apps:storage got no advisory at all.
func TestDocStoreUseWithoutTheScopeRealShape(t *testing.T) {
	files := map[string]string{"src/kv-store.ts": "export const KV_COLLECTION = 'kv';\n" +
		"const kv = async () =>\n  (await docs()).collection<Record<string, unknown>>(KV_COLLECTION);\n"}
	got := storageSourceWarnings(t, docManifest("", "user:read:self"), files)
	if len(got) != 1 || got[0].Message != noScopeMessage("src/kv-store.ts") {
		t.Fatalf("want exactly the no-scope warning naming src/kv-store.ts, got %d:%s", len(got), describe(got))
	}
}

// TestDocStoreIgnoresTestHelpers — test HELPERS that are not named *.test.* are
// not the app's runtime either. The documented convention: a basename starting
// `fake-`, `test-` or `mock-`, or anything under a `testing/`, `__mocks__/` or
// `__tests__/` directory. The incident: every forgejo app's docstore port keeps
// an in-memory store in `src/fake-doc-storage.ts`, which sorts before the real
// client, so the no-scope advisory named the FAKE as the file to fix.
func TestDocStoreIgnoresTestHelpers(t *testing.T) {
	fake := "export const seed = (fake: FakeStorage) => fake.storage.collection('ghost_helper_tbl').put('k', {});\n" +
		"export const y = () => useAppStorage();\n"
	for _, path := range []string{
		"src/fake-doc-storage.ts", "src/sheet/fake-doc-storage.ts", "src/test-doc-store.ts",
		"src/test-setup.ts", "src/mock-host.ts", "src/testing/store.ts", "src/__mocks__/sdk.ts",
	} {
		t.Run(path, func(t *testing.T) {
			got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
				map[string]string{path: fake, "src/main.ts": "export const x = 1;\n"})
			if len(got) != 0 {
				t.Errorf("a test helper produced warnings:%s", describe(got))
			}
		})
	}
	// TWIN: the convention is a PREFIX of the basename (or a directory), so a
	// production file that merely contains the word is still read.
	got := storageSourceWarnings(t, docManifest(oneCollection("kv", `{"visibility":"private"}`), "apps:storage"),
		map[string]string{"src/board-fake-free.ts": fake})
	if len(got) != 2 {
		t.Errorf("twin: want 2 warnings from the same body in a production file, got %d:%s", len(got), describe(got))
	}
	// THE INCIDENT: a fake beside the real client, under a manifest with no
	// storage at all. The no-scope advisory must name the PRODUCTION file.
	got = storageSourceWarnings(t, docManifest("", "user:read:self"), map[string]string{
		"src/fake-doc-storage.ts": "export const f = (fake: FakeStorage) => fake.storage.collection(KV).list();\n",
		"src/sync.ts":             "export const s = (n: string) => app.storage.collection(n).list();\n",
	})
	if len(got) != 1 || got[0].Message != noScopeMessage("src/sync.ts") {
		t.Errorf("want the no-scope warning naming src/sync.ts, got %d:%s", len(got), describe(got))
	}
}

// legacyMessage is the legacy-hook advisory, pinned WHOLE. It is the pre-#856
// text (cli #767) restored, with one edit: its last sentence used to warn that
// "a hook imported but never called is a false positive", and the match now
// needs a CALL, so that caveat is gone.
func legacyMessage(hook, readScope, writeScope string) string {
	return "this project's source calls `" + hook + "()` but the manifest declares no " +
		"storage scope — add the ones it uses to `scopes` (`" + readScope + "` to read, `" +
		writeScope + "` to write) each with a `scopeJustifications` entry. The server gates " +
		"storage by PRESENCE in the approved scope set, so without them every call fails in " +
		"production with a 403 (`storage set requires the " + writeScope + " scope`) while " +
		"working locally — and an authorization failure is deliberately unclassified by the " +
		"SDK, so a block usually surfaces it as an unexplained \"try again\". ADVISORY: this " +
		"reads source statically."
}
