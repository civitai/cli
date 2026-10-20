package validate

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
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
// schema's, so no rule lives in two places here.
//
// HARD ERRORS, like the other ported semantic rules (AGENTS.md item 1): each
// one is a refusal the server returns, so a manifest that trips it cannot be
// submitted. Messages are the server's text, so the CLI and the 400 agree.
//
// 🔴 THE ONE EXCEPTION: THE FEATURE GATES ARE WARNINGS (storageFeatureGateWarnings).
// The server refuses five features today, but each is a one-line switch in
// APP_STORAGE_ENABLED_FEATURES, and `app submit` blocks on validate ERRORS. A
// hard error here would outlive the switch in every CLI already installed and
// refuse manifests the server has started to accept. The warning keeps the
// server's text and says the server refuses it at submit TODAY. The vendored
// schema's description is what tells this repository a feature was enabled:
// TestVendoredSchemaStillStatesTheFeatureGates goes red on the re-vendor PR.
//
// THE ONE KNOWN DIVERGENCE: where the server lists collection names in
// DECLARATION order, this lists them SORTED. The manifest is decoded into a Go
// map, which keeps no key order; same names, same rule.

// The two document-store scopes (APP_STORAGE_BASE_SCOPE / APP_STORAGE_PUBLISH_SCOPE).
const (
	appStorageBaseScope    = "apps:storage"
	appStoragePublishScope = "apps:storage:publish"
)

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

// storageFeatureGateWarnings reports the five features switched off in
// APP_STORAGE_ENABLED_FEATURES, as WARNINGS (see the file comment for why).
//
// Each gate fires under exactly the condition the server reaches it: after the
// visibility parsed, and — for the two numeric keys — only once the value is
// in range, because an out-of-range value gets the range refusal instead.
func storageFeatureGateWarnings(generic any) []Finding {
	cols, ok := storageCollections(generic)
	if !ok {
		return nil
	}
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
				at+`.write "anyone"`, "documents any viewer may edit", `use "own" or "none" for now`)))
		}
		if shared && write == "owner" {
			out = append(out, newFinding(childField(at, "write"), storageNotYet(
				at+`.write "owner"`, "owner-written documents", `use "own" or "none" for now`)))
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
	}
	return out
}

// storageDeclarationChecks reports the declaration refusal the schema admits
// and the server keeps refusing: a counter and a mark sharing one name (two
// lists, each `uniqueItems` on its own, so no schema keyword relates them).
//
// The server compares the two lists only as ITS parser left them, and
// parseTallyNames discards a whole list at its first unusable entry — so a list
// the schema already rejects (see tallyNames) is compared as empty here too,
// and draws no second, collision finding.
func storageDeclarationChecks(generic any) []Finding {
	cols, ok := storageCollections(generic)
	if !ok {
		return nil
	}
	var out []Finding
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

// storageScopeRuleChecks is the server's appStorageScopeErrors: the four rules
// between `storage` and `scopes`.
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

	cols, _ := storageCollections(generic)
	var names, publishing []string
	for _, c := range cols {
		names = append(names, c.name)
		if collectionNeedsPublishScope(c.decl) {
			publishing = append(publishing, c.name)
		}
	}

	var out []Finding
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
	if len(names) == 0 && hasBase {
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
