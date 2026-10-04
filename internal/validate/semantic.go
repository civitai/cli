package validate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// blockGoodPayloadMaxBytes mirrors BLOCK_GOOD_PAYLOAD_MAX_BYTES in the server's
// block-goods.constants.ts. It is here rather than read out of the vendored
// schema because the schema does not contain it and cannot: this bounds the
// LENGTH OF THE JSON ENCODING of an opaque object, which JSON Schema has no
// vocabulary for.
//
// ⚠ The server measures `new TextEncoder().encode(JSON.stringify(payload))`,
// i.e. UTF-8 bytes of the compact encoding. Go's `json.Marshal` also emits
// compact UTF-8, so the two agree on byte count — but they agree by construction
// of both encoders, not by anything asserted here. A non-ASCII payload near the
// bound is where to look if they ever disagree.
const blockGoodPayloadMaxBytes = 2048

// The `app_unlock` bounds, mirrored from the same server module
// (BLOCK_APP_UNLOCK_MAX_PRICE_BUZZ, BLOCK_APP_UNLOCK_MAX_PER_MANIFEST).
//
// 🔴 WHY THESE ARE HERE, STATED CORRECTLY — the first draft said "JSON Schema
// cannot express them" and that is FALSE, so do not re-derive it. The dialect is
// draft/2020-12, where all three ARE expressible: `if`/`then` + `required` for
// the conditional `justification`, `contains` + `maxContains: 1` for the arity,
// `if`/`then` + `maximum` for the kind's ceiling. The upstream constants file
// says so itself — *"a conditional `if/then` on `kind` there is more surface than
// this one property is worth"* — i.e. CONSIDERED AND DECLINED, not impossible.
//
// The real reason is ownership: `schema/app-block.manifest.schema.json` is an
// auto-revendored BYTE-MIRROR of `civitai:public/schemas/app-block/v1.json`, and
// this repo may not unilaterally edit it — a local change would be reverted by
// the next re-vendor. So the Go port is the only layer the CLI controls.
//
// ⚠ THAT MAKES THIS A SECOND-BEST FIX, AND SAYING SO IS THE POINT: declaring the
// three rules upstream would cover the SDK, developer.civitai.com and every
// third-party validator, and would make most of this block redundant. The false
// "cannot express" version foreclosed that option by making it look unavailable.
// If you are reading this while touching these rules, the upstream change is the
// better one to argue for.
//
// Measured consequence of the current state: an `app_unlock` priced at 5001, and
// a second `app_unlock` in one manifest, BOTH validate against the vendored
// schema and are rejected at submit.
//
// ⚠ `blockGoodMinPriceBuzz` IS part of a check, and an earlier draft of this
// very block said it was not — "NOT a check; the schema's `minimum` owns the
// general floor". That was true while the price check was clamped and became
// false the moment the clamp came off: the condition below tests
// `price < blockGoodMinPriceBuzz`, so `{"kind":"app_unlock","priceBuzz":0}`
// emits the mirrored sentence AND the schema's `minimum: got 0, want 2`.
//
// 🔴 AND THE FLOOR ARM'S JUSTIFICATION IS NOT THE CEILING'S — do not collapse
// them. The wrong-bound argument below is CEILING-SPECIFIC: the floor is 2 in
// the schema, in the server and here, so on that arm the second message corrects
// no bound and is pure duplication. It is kept anyway, because the arm is how
// the mirror stays a faithful copy of the server's single condition rather than
// a hand-picked subset of it — and because a reader who deletes it on the
// "pure duplication" reasoning silently drops a server rule. The `app_unlock at
// 1` test row is what catches that deletion.
//
// 🔴 THE RULE THIS FILE ACTUALLY HOLDS IS "MIRROR THE SERVER'S SENTENCE", NOT
// "NEVER REPORT TWICE" — and an earlier draft of this block asserted the latter,
// which is false about the code it sits in. A goods entry with no `title` already
// emits BOTH `goods[0].title must be a non-empty string` (here) and
// `goods[0]: missing property 'title'` (schema), and has since before this
// block existed. So a non-report rule was never the invariant.
//
// That mattered, because the false invariant bought a real defect: an earlier
// draft scoped the `app_unlock` price check to 2..50000 "so as not to report
// twice", and an `app_unlock` at 60000 then got ONLY the schema's
// `maximum: got 60,000, want 50,000` — 10x the real ceiling. A developer is told
// to come down to 50000, does, and is rejected again with a different number.
// 🔴 NAMING THE WRONG BOUND IS WORSE THAN NAMING IT TWICE. The check below is
// therefore unclamped above: for an `app_unlock` it fires on the server's own
// condition, and the schema may fire alongside it.
const (
	blockGoodMinPriceBuzz        = 2
	blockAppUnlockMaxPriceBuzz   = 5_000
	blockAppUnlockMaxPerManifest = 1
)

// The two `kind` values, mirroring BLOCK_GOOD_KINDS / BLOCK_GOOD_DEFAULT_KIND.
// `app_unlock` is the one carrying extra rules; `good` is the default an absent
// `kind` resolves to.
const (
	blockGoodDefaultKind   = "good"
	blockGoodKindAppUnlock = "app_unlock"
)

// semantic.go ports the *semantic* manifest rules the server runs at approve
// time in BlockManifestValidator
// (civitai/civitai → src/server/services/block-manifest-validator.service.ts).
//
// The vendored JSON Schema only covers syntactic shape (types, enums, ranges
// when a field is present). These checks cover the cross-field / tier-gated
// rules the schema cannot express, so `validate` stops green-lighting manifests
// the server rejects. They operate on the decoded generic map (like the
// server's RawManifest), not the typed CLI struct.
//
// IMPORTANT: trustTier is server-owned and forced to "unverified" at submit, so
// every tier-gated rule below is evaluated against the UNVERIFIED tier — the
// only tier a fresh submission can have. (A dev-set trustTier is already a hard
// error via serverOwnedFieldChecks.)

// sandboxUnverifiedAllowlist is the sandbox-token allowlist for the unverified
// trust tier — the only tier a submitted block can hold. Mirrors
// SANDBOX_ALLOWLIST.unverified in the server validator (validator ~L149-173):
// only allow-scripts + allow-forms. Notably it excludes allow-same-origin,
// allow-popups, allow-top-navigation, etc.
var sandboxUnverifiedAllowlist = map[string]struct{}{
	"allow-scripts": {},
	"allow-forms":   {},
}

// iframe height envelope — mirrors HEIGHT_MIN_FLOOR / HEIGHT_MAX_CEILING
// (validator ~L58-59).
const (
	heightMinFloor   = 40
	heightMaxCeiling = 4000
)

// semanticChecks runs the ported BlockManifestValidator semantic rules over the
// decoded manifest map.
//
// 🔴 EVERY finding here carries a Field, and that is the whole point of issue
// #225: these are the checks the JSON Schema cannot express, so they are the
// ones `--json` exists to surface — and they were also the four that came back
// `field: null`, because their messages are prose with no parseable path
// prefix. A field derived at the printer could never have worked for them.
//
// FIELD ASSIGNMENT: a finding names the manifest location it is ABOUT — the
// field whose presence, absence, or value the rule reports on. For an
// "X requires Y" rule that is Y, the thing that must appear.
func semanticChecks(generic any) []Finding {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	var errs []Finding

	// renderMode defaults to "iframe" (validator ~L280).
	renderMode := "iframe"
	if rm, ok := m["renderMode"].(string); ok && rm != "" {
		renderMode = rm
	}

	// renderMode tier gate: inline/hybrid require a verified/internal tier
	// (validator ~L289). A submitted block is unverified, so inline/hybrid is
	// always rejected. The schema can't express this (it's a cross-field gate
	// on the server-owned trustTier).
	if renderMode == "inline" || renderMode == "hybrid" {
		errs = append(errs, newFinding("renderMode", fmt.Sprintf(
			"renderMode %q requires a verified/internal trust tier (INLINE_REQUIRES_VERIFIED_TIER); "+
				"a submitted block is always unverified, so use renderMode \"iframe\"", renderMode)))
	}

	iframe, hasIframe := m["iframe"].(map[string]any)
	_, hasPage := m["page"].(map[string]any)

	// page ⇒ iframe required (validator ~L504): a manifest declaring a page
	// must also ship an iframe block (the bundle the page mounts).
	if hasPage && !hasIframe {
		errs = append(errs, newFinding("iframe",
			"a manifest declaring \"page\" must also declare an iframe block (the bundle the page mounts)"))
	}

	// renderMode=iframe ⇒ iframe block required (validator ~L375-377).
	if renderMode == "iframe" && !hasIframe {
		errs = append(errs, newFinding("iframe", "iframe block is required for renderMode=iframe"))
	}

	// iframe required sub-fields when an iframe block is present (validator
	// ~L387-415). The schema validates ranges/types WHEN these fields appear,
	// but does not make them required — so a block lacking minHeight/resizable
	// passes the schema yet the server rejects it.
	if hasIframe {
		errs = append(errs, iframeRequiredFields(iframe)...)
		errs = append(errs, sandboxChecks(iframe)...)
	}

	// scopeJustifications keys must be a subset of the declared scopes
	// (validator: justifications for scopes the block doesn't request are
	// rejected). The JSON Schema type-gates the map + its values but cannot
	// express the keys⊆scopes cross-field rule.
	errs = append(errs, scopeJustificationChecks(m)...)

	// Every declared SENSITIVE scope MUST carry a non-empty justification — the
	// server now REJECTS a manifest that declares one without it at submit
	// (block-manifest-validator: unjustifiedSensitiveScopes). Mirror it as a
	// hard error so `civitai app validate` fails locally with the SAME message
	// instead of eating a server 400 after a full package+submit round-trip.
	errs = append(errs, sensitiveScopeJustificationChecks(m)...)

	// The three `goods` rules the vendored JSON Schema cannot express. Same
	// contract as the sensitive-scope mirror above: fail locally with the
	// server's own words instead of eating a 400 after a full package+submit.
	errs = append(errs, goodsChecks(m)...)

	return errs
}

// goodsChecks mirrors the rules in the server's `parseManifestGoods`
// (civitai → src/shared/constants/block-goods.constants.ts) that JSON Schema has
// no way to state, and ONLY those.
//
// The vendored schema already covers every shape rule — `id`'s pattern and
// maxLength, `title`/`description` maxLength, `priceBuzz`'s 2..50000, `kind`'s
// enum, `maxItems: 32`, and that `goods` is an array of objects. Adding a second
// copy of those here would be the duplicated-predicate failure, wrong at one of
// the two sites the first time either moves. What is left is exactly three
// properties a schema cannot say:
//
//  1. `id` UNIQUENESS across the array. 🔴 This is the one with teeth. The
//     server's own comment: duplicates are fatal "rather than 'last wins': the
//     purchase path looks a good up by id, and two rows answering to one id
//     means the price charged depends on iteration order." So a manifest the
//     CLI passes can make the price CHARGED nondeterministic.
//  2. `title` non-empty AFTER TRIMMING. The schema's `minLength: 1` is a rune
//     count and accepts "   ".
//  3. `payload`'s SERIALIZED byte size. The schema can bound an object's shape,
//     never the length of its JSON encoding.
//
// 🔴 ONE FINDING PER OFFENDING ENTRY, IN DECLARATION ORDER, AND THE FIRST HIT
// ENDS THAT ENTRY — because the server is a `forEach` whose every rejection
// `return`s. Emitting two findings for one entry, or reordering them, would make
// the local output a different SET from the 400 it predicts.
//
// ⚠ AND THE LIMIT OF THAT CLAIM, STATED RATHER THAN IMPLIED: per-message
// equality is what this buys, never set-equality for every manifest. Where the
// SCHEMA rejects something the server also rejects, the two outputs already
// differ in shape — the server emits one sentence per entry and stops, the
// schema layer emits one per violated keyword. A manifest with a malformed `id`
// is the worked example: it fails either way, and the message sets are not equal.
// Do not "fix" that by re-implementing the schema's rules here.
//
// 🔴 DELIBERATELY NO COPY OF THE ID PATTERN OR MAXLENGTH. An earlier draft of
// this function pre-checked both so it could skip an entry exactly where the
// server's `forEach` returns. That is schema content living in a second place,
// wrong at one of the two sites the moment either moves — the duplicated-
// predicate failure. The cost of dropping it is bounded and cosmetic: two
// entries sharing ONE malformed id get a duplicate finding here that the server
// would not have emitted, on a manifest that is already failing on the pattern.
// Messages are the server's verbatim, including the quoting of the duplicated id.
func goodsChecks(generic map[string]any) []Finding {
	raw, ok := generic["goods"].([]any)
	if !ok {
		// Absent, null, or a non-array: absent sells nothing and is valid, and a
		// non-array is the schema's to reject. Either way, nothing here.
		return nil
	}

	var out []Finding
	seen := make(map[string]struct{}, len(raw))
	appUnlockCount := 0
	for i, entry := range raw {
		e, ok := entry.(map[string]any)
		if !ok {
			continue // `goods[i] must be an object` — schema-covered.
		}
		at := fmt.Sprintf("goods[%d]", i)

		// A non-string id is the schema's to reject, and it cannot key a
		// duplicate test either.
		id, idOK := e["id"].(string)
		if !idOK {
			continue
		}
		if _, dup := seen[id]; dup {
			out = append(out, newFinding(at+".id", fmt.Sprintf(
				"%s.id duplicates an earlier good id (%q)", at, id)))
			continue
		}
		seen[id] = struct{}{}

		if title, ok := e["title"].(string); !ok || strings.TrimSpace(title) == "" {
			out = append(out, newFinding(at+".title", fmt.Sprintf(
				"%s.title must be a non-empty string", at)))
			continue
		}

		// Resolve the kind exactly as the server does: absent or unset means the
		// default. A kind that is PRESENT but not a member is the schema's enum to
		// reject, and the server `return`s on it — so the entry's kind-specific
		// rules below are unreachable for it here too.
		//
		// ⚠ AUDIT FINDING REJECTED, WITH THE EVIDENCE, SO IT IS NOT RE-RAISED: a
		// round-0 pass read this `continue` as suppressing the `payload` byte-size
		// finding — the one goods rule JSON Schema provably cannot express — for an
		// entry whose `kind` is invalid. It does, and that is CORRECT: the server's
		// kind check `return`s at block-goods.constants.ts:385-387, well before its
		// own payload check at :464, so it suppresses the identical finding. Making
		// the CLI continue to the payload check here would be a DIVERGENCE from the
		// server, which is the one thing this file exists not to do. The cost is
		// real but is the server's: an author with both defects learns about the
		// kind first and the payload on the next run.
		kind := blockGoodDefaultKind
		if raw, present := e["kind"]; present {
			k, isString := raw.(string)
			if !isString || (k != blockGoodDefaultKind && k != blockGoodKindAppUnlock) {
				continue // `kind must be one of good, app_unlock` — schema-covered.
			}
			kind = k
		}

		// The `app_unlock` price ceiling, mirroring the server's own condition for
		// this kind: a whole number in 2..5000. Deliberately NOT clamped to the
		// window where the vendored schema passes — see the constant block for why
		// that clamp was a defect rather than a courtesy. Above 50000 the schema
		// fires too; two findings naming the right bound beat one naming a bound
		// 10x too high.
		//
		// A non-numeric `priceBuzz` is left to the schema's `type`: `toNumber`
		// cannot evaluate the server's condition on it, so there is nothing to
		// mirror.
		//
		// 🔴 The message names the KIND'S ceiling, not the general one, verbatim
		// including the ` for an app_unlock good` suffix. The server's own comment
		// is explicit that for an `app_unlock` this string is the only
		// machine-delivered statement of the real limit.
		if kind == blockGoodKindAppUnlock {
			if price, isNumber := toNumber(e["priceBuzz"]); isNumber &&
				(price != float64(int64(price)) ||
					price < blockGoodMinPriceBuzz ||
					price > blockAppUnlockMaxPriceBuzz) {
				out = append(out, newFinding(at+".priceBuzz", fmt.Sprintf(
					"%s.priceBuzz must be a whole number between %d and %d Buzz for an %s good",
					at, blockGoodMinPriceBuzz, blockAppUnlockMaxPriceBuzz, blockGoodKindAppUnlock)))
				continue
			}
		}

		// `justification`. Shape-checked for ANY good so a developer who explains an
		// ordinary item is not rejected for it; REQUIRED only for `app_unlock`.
		//
		// Only the two rules the schema cannot express are here. The schema already
		// carries `minLength: 1` and `maxLength: 500`, and it measures the RAW
		// string while the server measures the TRIMMED one — which makes the schema
		// never MORE permissive, so its length bound cannot pass something the
		// server's rejects. What it CANNOT see is whitespace-only (`" "` satisfies
		// `minLength: 1`), and absence (`justification` is not in the schema's
		// `required`, and could not be: the requirement is conditional on `kind`).
		if raw, present := e["justification"]; present {
			if s, isString := raw.(string); !isString || strings.TrimSpace(s) == "" {
				out = append(out, newFinding(at+".justification", fmt.Sprintf(
					"%s.justification must be a non-empty string", at)))
				continue
			}
		} else if kind == blockGoodKindAppUnlock {
			out = append(out, newFinding(at+".justification", fmt.Sprintf(
				"%s.justification is required for an %s good — it makes the app paid, so a moderator must be told why",
				at, blockGoodKindAppUnlock)))
			continue
		}

		if payload, present := e["payload"]; present {
			p, ok := payload.(map[string]any)
			if !ok {
				continue // `payload must be an object` — schema-covered.
			}
			// json.Marshal on a map decoded from JSON cannot fail, but the error
			// is checked rather than discarded: a future caller passing a
			// hand-built map with an unsupported value would otherwise measure
			// the size of an empty slice and pass.
			b, err := json.Marshal(p)
			if err != nil {
				continue
			}
			if len(b) > blockGoodPayloadMaxBytes {
				out = append(out, newFinding(at+".payload", fmt.Sprintf(
					"%s.payload must serialize to at most %d bytes", at, blockGoodPayloadMaxBytes)))
				continue
			}
		}

		// Reached only by an entry that survived every check above — which is what
		// the server's parsed `goods` array holds, and what its arity check counts.
		if kind == blockGoodKindAppUnlock {
			appUnlockCount++
		}
	}

	// ARITY, a whole-array property: at most one `app_unlock` per manifest, so
	// "is this viewer admitted?" has exactly one answer. Unlike every check in the
	// loop this one is ADDITIVE rather than entry-terminating — the server pushes
	// it and falls through to return, so it can accompany a per-entry finding.
	// Its path is `goods`, not `goods[i]`.
	//
	// ⚠ A FAILING ENTRY IS NOT COUNTED, WHICH CAN TAKE THE SURVIVING COUNT BELOW
	// THE LIMIT — it does not suppress this check in general. Every per-entry
	// check above `continue`s, so a failing entry never reaches the counter; the
	// arity finding then disappears only when what SURVIVES is one unlock or
	// fewer. With three unlocks and one over-cap, two survive and BOTH findings
	// appear — which is the ADDITIVE property stated directly above, and is
	// pinned by the `arity is ADDITIVE` row below.
	//
	// ⚠ Do not restate this as "an earlier finding suppresses the arity one". An
	// earlier draft of this paragraph did, and it was false while reading
	// plausibly, because BOTH server consequences quoted next are 2-unlock cases
	// where the surviving count happens to fall to 1. It also contradicted the
	// paragraph three lines above it and a test row named after that paragraph.
	//
	// The two consequences, in the server's own words at
	// block-goods.constants.ts:501-509 — "two unlocks where one is over-cap
	// reports ONLY the price error, and two unlocks sharing an id report ONLY the
	// duplicate error" — both hold here, each pinned by its own row below.
	//
	// Note the price arm of that only became true when the clamp came off: while
	// the check was scoped to 2..50000, an unlock at 60000 did NOT `continue`, so
	// it was counted and a second over-cap unlock reported the arity error the
	// server would not have reported. Removing the clamp fixed a mirror defect
	// nobody had looked for.
	//
	// ⚠ THE LIMIT, STATED: the server counts only goods that PARSED, and it
	// enforces rules this package deliberately leaves to the schema (the `id`
	// pattern and length, `title` length, `description`, the `kind` enum, the
	// general price range). An entry failing one of those is dropped from the
	// server's count and cannot be dropped from this one, so on a manifest that is
	// ALREADY failing the schema the CLI can report an arity violation the server
	// would not. Same bounded-and-cosmetic trade as the duplicate-id note above,
	// and the same reason: re-implementing the schema's rules here to close it
	// would put schema content in a second place.
	if appUnlockCount > blockAppUnlockMaxPerManifest {
		out = append(out, newFinding("goods", fmt.Sprintf(
			"goods may declare at most %d %s good (found %d) — app access is one question with one answer",
			blockAppUnlockMaxPerManifest, blockGoodKindAppUnlock, appUnlockCount)))
	}

	return out
}

// SENSITIVE_BLOCK_SCOPES mirrors the server's single-sourced sensitive set
// (civitai → src/shared/constants/block-scope.constants.ts SENSITIVE_BLOCK_SCOPES):
// the scopes that can spend/read the viewer's Buzz, read their PRIVATE data, or
// write data other users see. A declared scope in this set MUST carry a
// non-empty scopeJustifications entry or the server rejects the manifest at
// submit — so this Go mirror fails the same manifests LOCALLY. Keep it byte-in-
// step with the server set; the sensitiveScopeJustificationError message is
// mirrored verbatim so CLI and server agree.
var SENSITIVE_BLOCK_SCOPES = map[string]struct{}{
	"ai:write:budgeted":         {},
	"social:tip:self":           {},
	"buzz:read:self":            {},
	"collections:read:private":  {},
	"apps:storage:shared:write": {},
	// posts:write:self publishes a PUBLIC post under the viewer's own byline —
	// the "write data other users see" arm of the server's criterion, and the
	// most consequential member of it.
	"posts:write:self": {},
	// goods:purchase:self SPENDS the viewer's Buzz on a manifest-declared good
	// — the "can spend the viewer's Buzz" arm. Its READ half, `goods:read:self`,
	// is deliberately NOT here and must not be added: the server scopes that
	// reply to `claims.appBlockId`, so an app reading it sees only what it
	// itself sold to that viewer. Verified against the DEPLOYED constant
	// (civitai `origin/release`, not `main` — dp-prod serves `release`), where
	// the set is these seven.
	"goods:purchase:self": {},
}

// isSensitiveBlockScope reports whether scope is in SENSITIVE_BLOCK_SCOPES.
// Scope names are canonical lowercase, so the match is exact (no case-folding),
// mirroring the server's isSensitiveBlockScope.
func isSensitiveBlockScope(scope string) bool {
	_, ok := SENSITIVE_BLOCK_SCOPES[scope]
	return ok
}

// unjustifiedSensitiveScopes returns the DECLARED sensitive scopes that lack a
// non-empty (trimmed) scopeJustifications entry, deduped and in DECLARATION
// order — mirroring the server helper of the same name so the CLI names the
// same offenders in the same order as the 400 the server would return. A
// justification "counts" only when its value is a string with non-whitespace
// content; an empty/whitespace value, a non-string value, or a missing key all
// leave the sensitive scope unjustified. A non-array `scopes` yields nil.
func unjustifiedSensitiveScopes(generic map[string]any) []string {
	arr, ok := generic["scopes"].([]any)
	if !ok {
		return nil
	}
	just, _ := generic["scopeJustifications"].(map[string]any)

	var out []string
	seen := make(map[string]struct{}, len(arr))
	for _, e := range arr {
		scope, ok := e.(string)
		if !ok || !isSensitiveBlockScope(scope) {
			continue
		}
		if _, dup := seen[scope]; dup {
			continue
		}
		seen[scope] = struct{}{}
		if s, ok := just[scope].(string); ok && strings.TrimSpace(s) != "" {
			continue // justified
		}
		out = append(out, scope)
	}
	return out
}

// sensitiveScopeJustificationChecks emits ONE hard error naming every declared
// sensitive scope that lacks a non-empty justification, using the server's exact
// message so the local failure is byte-identical to the submit-time 400.
//
// FIELD: `scopeJustifications` — the map the author must add entries to. Not
// `scopes` (the declared scopes are correct; it is the justifications that are
// missing) and not one entry per scope (the server emits ONE message naming
// them all, and the mirror is byte-identical on purpose).
func sensitiveScopeJustificationChecks(generic map[string]any) []Finding {
	unjustified := unjustifiedSensitiveScopes(generic)
	if len(unjustified) == 0 {
		return nil
	}
	return []Finding{newFinding("scopeJustifications",
		"sensitive scopes require a justification — add a non-empty scopeJustifications entry for: "+
			strings.Join(unjustified, ", "),
	)}
}

// scopeJustificationChecks enforces that every key in scopeJustifications is a
// scope the manifest actually declares in scopes[] — mirroring the server, which
// rejects justifications for scopes the block doesn't request. The vendored JSON
// Schema already covers the value shape (non-empty string ≤500 chars); this adds
// ONLY the keys⊆scopes rule the schema cannot express. Absent map or empty map
// yields no error (backward-compatible). Scope names are canonical lowercase, so
// the comparison is an exact match with no case-folding (mirroring the server).
//
// FIELD: `scopeJustifications.<key>` — ONE finding per offending key, each
// naming that key. This is the check whose findings would collapse into each
// other if the message ever stopped interpolating the key, which is why
// dedupeFindings keys on the (field, message) PAIR rather than the message.
func scopeJustificationChecks(generic map[string]any) []Finding {
	just, ok := generic["scopeJustifications"].(map[string]any)
	if !ok || len(just) == 0 {
		// Absent, wrong-typed (schema-handled), or empty → nothing to check.
		return nil
	}
	scopes := stringSet(generic["scopes"])

	// Iterate sorted keys so error order is deterministic.
	keys := make([]string, 0, len(just))
	for k := range just {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var errs []Finding
	for _, key := range keys {
		if _, declared := scopes[key]; !declared {
			errs = append(errs, newFinding(childField("scopeJustifications", key), fmt.Sprintf(
				"scopeJustifications key %q is not one of the manifest's declared scopes", key)))
		}
	}
	return errs
}

// iframeRequiredFields enforces the server's required-ness of minHeight and
// resizable on a present iframe block (validator ~L387-415). Range bounds when
// present are already covered by the JSON Schema; here we add the required-ness
// the schema omits, with the same bounds for a clear single message.
func iframeRequiredFields(iframe map[string]any) []Finding {
	var errs []Finding

	minHeightField := childField("iframe", "minHeight")
	mh, ok := iframe["minHeight"]
	if !ok {
		errs = append(errs, newFinding(minHeightField, fmt.Sprintf(
			"iframe.minHeight is required and must be a number in [%d, %d]", heightMinFloor, heightMaxCeiling)))
	} else if n, isNum := toNumber(mh); !isNum || n < heightMinFloor || n > heightMaxCeiling {
		errs = append(errs, newFinding(minHeightField, fmt.Sprintf(
			"iframe.minHeight must be a number in [%d, %d]", heightMinFloor, heightMaxCeiling)))
	}

	resizableField := childField("iframe", "resizable")
	if rz, ok := iframe["resizable"]; !ok {
		errs = append(errs, newFinding(resizableField, "iframe.resizable is required and must be a boolean"))
	} else if _, isBool := rz.(bool); !isBool {
		errs = append(errs, newFinding(resizableField, "iframe.resizable must be a boolean"))
	}

	return errs
}

// sandboxChecks ports validateSandbox (validator ~L175-206) for the UNVERIFIED
// tier — the only tier a submitted block can hold. Any token outside the
// unverified allowlist is rejected, and the allow-same-origin + allow-scripts
// sandbox-escape combo is rejected explicitly (defense in depth).
//
// FIELD: `iframe.sandbox` on every finding — including the sandbox-escape combo
// rule, which is genuinely about the interaction of two TOKENS inside one
// string value rather than about two fields, so the string is the location.
func sandboxChecks(iframe map[string]any) []Finding {
	sandboxField := childField("iframe", "sandbox")
	raw, ok := iframe["sandbox"]
	if !ok {
		// sandbox required-ness / type is handled by the schema (minLength 1).
		return nil
	}
	sandbox, isStr := raw.(string)
	if !isStr {
		// type handled by the schema.
		return nil
	}

	tokens := strings.Fields(sandbox)
	if len(tokens) == 0 {
		return []Finding{newFinding(sandboxField, "iframe.sandbox must contain at least one token")}
	}

	var errs []Finding
	seen := make(map[string]struct{}, len(tokens))
	// Iterate deterministically over deduped tokens so error order is stable.
	var order []string
	for _, tok := range tokens {
		if _, dup := seen[tok]; dup {
			continue
		}
		seen[tok] = struct{}{}
		order = append(order, tok)
	}
	for _, tok := range order {
		if _, allowed := sandboxUnverifiedAllowlist[tok]; !allowed {
			errs = append(errs, newFinding(sandboxField, fmt.Sprintf(
				"sandbox token %q is not allowed for unverified blocks "+
					"(trustTier is server-forced to unverified at submit; "+
					"only allow-scripts and allow-forms are permitted)", tok)))
		}
	}

	// Defense-in-depth: the sandbox-escape combo is rejected even if a future
	// allowlist change added the individual tokens (validator ~L201).
	_, hasSameOrigin := seen["allow-same-origin"]
	_, hasScripts := seen["allow-scripts"]
	if hasSameOrigin && hasScripts {
		errs = append(errs, newFinding(sandboxField,
			"iframe.sandbox MUST NOT combine allow-same-origin with allow-scripts (sandbox escape) — "+
				"forbidden outside the internal trust tier"))
	}

	return errs
}

// toNumber coerces a JSON-decoded numeric value (float64 from encoding/json) to
// a float for range checks. Returns ok=false for non-numbers.
func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
