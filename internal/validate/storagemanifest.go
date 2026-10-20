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

// Mirrors of the server bounds the gate ordering depends on: a value outside
// them gets the range error (the schema's, here), never the "not available yet"
// one.
const (
	appStorageMaxCollections       = 16
	appStorageTTLMinSeconds        = 60
	appStorageTTLMaxSeconds        = 31_536_000
	appStorageMaxDailyWritesAuthor = 1_000
)

// storageNotYet is the server's `notYet`: subject is the field, plus its value
// when the value is what is gated.
func storageNotYet(subject, feature, instead string) string {
	return subject + " is not available yet — " + feature +
		" arrive in a later phase of App Storage; " + instead
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

// storageDeclarationChecks reports the declaration refusals the schema admits:
// the five features switched off in APP_STORAGE_ENABLED_FEATURES, and a counter
// and a mark sharing one name (two lists, each `uniqueItems` on its own, so no
// schema keyword relates them).
//
// Each gate fires under exactly the condition the server reaches it: after the
// visibility parsed, and — for the two numeric keys — only once the value is
// in range, because an out-of-range value gets the range refusal instead.
func storageDeclarationChecks(generic any) []Finding {
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
		if shared {
			marks := map[string]bool{}
			for _, mk := range stringsOf(c.decl["marks"]) {
				marks[mk] = true
			}
			for _, counter := range stringsOf(c.decl["counters"]) {
				if marks[counter] {
					out = append(out, newFinding(at, fmt.Sprintf(
						"%s declares %q as both a counter and a mark — use different names", at, counter)))
				}
			}
		}
	}
	return out
}

// storageScopeRuleChecks is the server's appStorageScopeErrors: the four rules
// between `storage` and `scopes`.
//
// storageValid must be false whenever the declaration itself drew ANY finding
// (schema or imperative). The server evaluates these rules only against a
// declaration that parsed, because an invalid one declares nothing and a scope
// rule against it reports a second, misleading problem.
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

// stringsOf returns the string elements of a JSON array, ignoring the rest (a
// malformed list is the schema's to report).
func stringsOf(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
