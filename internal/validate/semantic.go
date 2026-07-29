package validate

import (
	"fmt"
	"sort"
	"strings"
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
func semanticChecks(generic any) []string {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	var errs []string

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
		errs = append(errs, fmt.Sprintf(
			"renderMode %q requires a verified/internal trust tier (INLINE_REQUIRES_VERIFIED_TIER); "+
				"a submitted block is always unverified, so use renderMode \"iframe\"", renderMode))
	}

	iframe, hasIframe := m["iframe"].(map[string]any)
	_, hasPage := m["page"].(map[string]any)

	// page ⇒ iframe required (validator ~L504): a manifest declaring a page
	// must also ship an iframe block (the bundle the page mounts).
	if hasPage && !hasIframe {
		errs = append(errs, "a manifest declaring \"page\" must also declare an iframe block (the bundle the page mounts)")
	}

	// renderMode=iframe ⇒ iframe block required (validator ~L375-377).
	if renderMode == "iframe" && !hasIframe {
		errs = append(errs, "iframe block is required for renderMode=iframe")
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

	return errs
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
func sensitiveScopeJustificationChecks(generic map[string]any) []string {
	unjustified := unjustifiedSensitiveScopes(generic)
	if len(unjustified) == 0 {
		return nil
	}
	return []string{
		"sensitive scopes require a justification — add a non-empty scopeJustifications entry for: " +
			strings.Join(unjustified, ", "),
	}
}

// scopeJustificationChecks enforces that every key in scopeJustifications is a
// scope the manifest actually declares in scopes[] — mirroring the server, which
// rejects justifications for scopes the block doesn't request. The vendored JSON
// Schema already covers the value shape (non-empty string ≤500 chars); this adds
// ONLY the keys⊆scopes rule the schema cannot express. Absent map or empty map
// yields no error (backward-compatible). Scope names are canonical lowercase, so
// the comparison is an exact match with no case-folding (mirroring the server).
func scopeJustificationChecks(generic map[string]any) []string {
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

	var errs []string
	for _, key := range keys {
		if _, declared := scopes[key]; !declared {
			errs = append(errs, fmt.Sprintf(
				"scopeJustifications key %q is not one of the manifest's declared scopes", key))
		}
	}
	return errs
}

// iframeRequiredFields enforces the server's required-ness of minHeight and
// resizable on a present iframe block (validator ~L387-415). Range bounds when
// present are already covered by the JSON Schema; here we add the required-ness
// the schema omits, with the same bounds for a clear single message.
func iframeRequiredFields(iframe map[string]any) []string {
	var errs []string

	mh, ok := iframe["minHeight"]
	if !ok {
		errs = append(errs, fmt.Sprintf(
			"iframe.minHeight is required and must be a number in [%d, %d]", heightMinFloor, heightMaxCeiling))
	} else if n, isNum := toNumber(mh); !isNum || n < heightMinFloor || n > heightMaxCeiling {
		errs = append(errs, fmt.Sprintf(
			"iframe.minHeight must be a number in [%d, %d]", heightMinFloor, heightMaxCeiling))
	}

	if rz, ok := iframe["resizable"]; !ok {
		errs = append(errs, "iframe.resizable is required and must be a boolean")
	} else if _, isBool := rz.(bool); !isBool {
		errs = append(errs, "iframe.resizable must be a boolean")
	}

	return errs
}

// sandboxChecks ports validateSandbox (validator ~L175-206) for the UNVERIFIED
// tier — the only tier a submitted block can hold. Any token outside the
// unverified allowlist is rejected, and the allow-same-origin + allow-scripts
// sandbox-escape combo is rejected explicitly (defense in depth).
func sandboxChecks(iframe map[string]any) []string {
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
		return []string{"iframe.sandbox must contain at least one token"}
	}

	var errs []string
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
			errs = append(errs, fmt.Sprintf(
				"sandbox token %q is not allowed for unverified blocks "+
					"(trustTier is server-forced to unverified at submit; "+
					"only allow-scripts and allow-forms are permitted)", tok))
		}
	}

	// Defense-in-depth: the sandbox-escape combo is rejected even if a future
	// allowlist change added the individual tokens (validator ~L201).
	_, hasSameOrigin := seen["allow-same-origin"]
	_, hasScripts := seen["allow-scripts"]
	if hasSameOrigin && hasScripts {
		errs = append(errs,
			"iframe.sandbox MUST NOT combine allow-same-origin with allow-scripts (sandbox escape) — "+
				"forbidden outside the internal trust tier")
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
