package validate

import (
	"fmt"
	"sort"
	"strings"
)

// targets.go ports the server's targets[].slotId registry check (validator
// ~L426-460). Each target's slotId must be a KNOWN registered slot id, and a
// model target must NOT be the page slot (a full page is declared via the
// `page` field, not a `targets` entry).
//
// The CLI cannot import the server slot-registry
// (civitai/civitai → src/shared/constants/slot-registry.ts), so the known slot
// ids are VENDORED below. There are only four (3 model + 1 page) and they are
// the durable historical contract (the model tuple is regression-locked
// server-side), so vendoring is cheap. KEEP IN SYNC with the server registry on
// any slot change. The durable fix for this drift is a server-side
// `civitai app validate` endpoint that calls the real BlockManifestValidator —
// see the README's "best-effort local pre-check" note.
//
// We are launching PAGE-ONLY; page apps have no targets[] (they declare a
// `page` field), so this path is rarely exercised today — but porting it keeps
// the model-slot path honest for when targets ship.

// vendoredSlotIDs mirrors SLOT_REGISTRY keys in the server slot-registry.
// pageSlot reports whether a slot is the full-page slot (SlotDef.kind === "page").
var vendoredSlotIDs = map[string]bool{
	// id -> isPageSlot
	"model.sidebar_top":   false,
	"model.below_images":  false,
	"model.actions_extra": false,
	"app.page":            true, // the W10 full-page slot
}

func isKnownSlotID(id string) bool { _, ok := vendoredSlotIDs[id]; return ok }
func isPageSlotID(id string) bool  { return vendoredSlotIDs[id] }

// knownSlotIDList renders the known slotIds as a stable, single-quoted,
// comma-separated list — matching the JSON Schema enum error's phrasing
// ("value must be one of 'a', 'b', …", single quotes via the jsonschema
// library) so the slot error is self-documenting like the scope error.
func knownSlotIDList() string {
	ids := make([]string, 0, len(vendoredSlotIDs))
	for id := range vendoredSlotIDs {
		ids = append(ids, "'"+id+"'")
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

// targetChecks validates targets[].slotId against the vendored registry. Shape
// (array of objects with a non-empty string slotId, ≤16 entries) is already
// enforced by the JSON Schema; here we add the registry-membership + page-slot
// semantic the schema cannot express.
//
// FIELD: `targets[i].slotId`, carrying the ELEMENT INDEX. `targets` alone would
// be useless on a manifest with several targets — which is the only shape where
// this check fires more than once — and the index is exactly what a consumer
// grouping findings needs in order to point at a line.
func targetChecks(generic any) []Finding {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := m["targets"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		// type handled by the schema.
		return nil
	}

	var errs []Finding
	for i, t := range arr {
		obj, ok := t.(map[string]any)
		if !ok {
			continue // shape handled by the schema.
		}
		slotID, ok := obj["slotId"].(string)
		if !ok || slotID == "" {
			continue // shape handled by the schema.
		}
		field := childField(indexField("targets", i), "slotId")
		if !isKnownSlotID(slotID) {
			errs = append(errs, newFinding(field, fmt.Sprintf(
				"target slotId %q is not a known slot — value must be one of %s", slotID, knownSlotIDList())))
			continue
		}
		if isPageSlotID(slotID) {
			errs = append(errs, newFinding(field, fmt.Sprintf(
				"target slotId %q is the page slot — declare a full page via the \"page\" field, not targets", slotID)))
		}
	}
	return errs
}
