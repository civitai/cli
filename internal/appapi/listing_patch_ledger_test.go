package appapi

import (
	"reflect"
	"strings"
	"testing"
)

// The patch-type ledger.
//
// 🔴 WHY A LEDGER AND NOT A COMMENT. `ListingPatch` is a closed interface, and
// the doc comment on it once claimed that made the separation "a property of the
// program" rather than "a sentence about today's code". THAT WAS OVERSTATED, and
// an audit refuted it by building the mutant: adding a `SourceRepoURL *string`
// to `ListingTextPatch`, wiring it into `wire()` and hanging a `--source-repo`
// flag off `set-text` COMPILES AND LEAVES THE WHOLE SUITE GREEN. The closed
// interface stops another PACKAGE implementing `ListingPatch`; it does not stop
// this one widening a struct.
//
// So the property is asserted here instead, where it can actually fail. The
// hazard it guards is not hypothetical: `sourceRepoUrl` is MATERIAL and the
// three text fields are not, and the server writes the FULL patch to a shadow
// revision as soon as any material field differs — so one struct carrying both
// would silently convert in-place tagline edits into staged ones.
//
// 🔴 IT FAILS WHEN THE FIELD SET GROWS *OR* SHRINKS, deliberately. A ledger that
// only rejected additions would be satisfied by deleting a field, and one that
// only checked for a `sourceRepoUrl`-shaped name would be walked by spelling it
// `RepoLink`. Adding a legitimate text field is meant to fail here once, so the
// person adding it re-reads which patch type it belongs in.

func fieldNames(t *testing.T, v any) []string {
	t.Helper()
	rt := reflect.TypeOf(v)
	if rt.Kind() != reflect.Struct {
		t.Fatalf("%T is not a struct", v)
	}
	out := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		out = append(out, rt.Field(i).Name)
	}
	return out
}

func TestListingTextPatchCarriesExactlyTheTextFields(t *testing.T) {
	want := []string{
		"Tagline", "Description", "Category",
		"ClearTagline", "ClearDescription", "ClearCategory",
	}
	got := fieldNames(t, ListingTextPatch{})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListingTextPatch fields changed.\n got: %v\nwant: %v\n\n"+
			"If you are ADDING a material field (anything in the server's "+
			"MATERIAL_PATCH_FIELDS — externalUrl, name, contentRating, sourceRepoUrl), it does "+
			"NOT belong here: one patch carrying a material field stages the WHOLE patch on a "+
			"revision, so the text edits travelling with it stop applying in place. Give it its "+
			"own ListingPatch implementation, as ListingSourceRepoPatch does.", got, want)
	}
	// Belt and braces on the same claim, stated the other way round: no field of
	// this struct may be about a repository, however it is spelled.
	for _, f := range got {
		if l := strings.ToLower(f); strings.Contains(l, "repo") || strings.Contains(l, "source") {
			t.Errorf("ListingTextPatch grew the field %q — the source-repository link is MATERIAL "+
				"and must not share a patch with the text fields", f)
		}
	}
}

func TestListingSourceRepoPatchCarriesExactlyItsOwnField(t *testing.T) {
	want := []string{"URL", "Clear"}
	got := fieldNames(t, ListingSourceRepoPatch{})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListingSourceRepoPatch fields changed.\n got: %v\nwant: %v\n\n"+
			"This type exists to carry ONE material field. Adding a text field here would "+
			"stage that text on a revision instead of applying it in place.", got, want)
	}
}

// TestListingPatchImplementationsAreLedgered pins WHICH types may reach
// UpdateListing.
//
// 🔴 THE POINT IS THE COUNT, NOT THE MEMBERSHIP. Each type on its own is
// asserted above; what this adds is that a THIRD implementation cannot appear
// unnoticed. A new patch type is exactly the moment someone must decide whether
// its fields are material, and this is the only thing that asks.
func TestListingPatchImplementationsAreLedgered(t *testing.T) {
	// Compile-time: both types satisfy the closed interface.
	var _ ListingPatch = ListingTextPatch{}
	var _ ListingPatch = ListingSourceRepoPatch{}

	// The wire shapes must be disjoint — no key may be reachable from both, or
	// the separation is nominal.
	textKeys := ListingTextPatch{
		Tagline: strptr("t"), Description: strptr("d"), Category: strptr("c"),
	}.wire()
	repoKeys := ListingSourceRepoPatch{URL: strptr("u")}.wire()
	for k := range repoKeys {
		if _, clash := textKeys[k]; clash {
			t.Errorf("both patch types can send %q — the split is nominal, not real", k)
		}
	}
	if len(textKeys) != 3 || len(repoKeys) != 1 {
		t.Fatalf("expected 3 text keys and 1 repo key, got %d and %d — without them the "+
			"disjointness check above is vacuous", len(textKeys), len(repoKeys))
	}
}

func strptr(s string) *string { return &s }
