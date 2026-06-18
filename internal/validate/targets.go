package validate

import "fmt"

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

// targetChecks validates targets[].slotId against the vendored registry. Shape
// (array of objects with a non-empty string slotId, ≤16 entries) is already
// enforced by the JSON Schema; here we add the registry-membership + page-slot
// semantic the schema cannot express.
func targetChecks(generic any) []string {
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

	var errs []string
	for _, t := range arr {
		obj, ok := t.(map[string]any)
		if !ok {
			continue // shape handled by the schema.
		}
		slotID, ok := obj["slotId"].(string)
		if !ok || slotID == "" {
			continue // shape handled by the schema.
		}
		if !isKnownSlotID(slotID) {
			errs = append(errs, fmt.Sprintf("target slotId %q is not a known slot", slotID))
			continue
		}
		if isPageSlotID(slotID) {
			errs = append(errs, fmt.Sprintf(
				"target slotId %q is the page slot — declare a full page via the \"page\" field, not targets", slotID))
		}
	}
	return errs
}
