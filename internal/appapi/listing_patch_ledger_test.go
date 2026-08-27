package appapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
// 🔴 THIS TEST ONCE CLAIMED A COUNT IT NEVER COUNTED. It asserted two
// `var _ ListingPatch = …` lines and a disjointness check over the two types it
// already knew about, while its comment promised "a THIRD implementation cannot
// appear unnoticed". An audit added `ListingComboPatch` — one type carrying a
// text field AND the material `sourceRepoUrl`, exactly the hazard this file
// exists for — and the whole suite stayed green.
//
// Go cannot enumerate a closed interface's implementations at runtime, so the
// enumeration is done over the package SOURCE: every method set that declares
// `wire() map[string]any` is an implementation, because that is what makes a type
// satisfy `ListingPatch`. A new one must be added here deliberately, which is the
// moment someone has to decide whether its fields are material.
func TestListingPatchImplementationsAreLedgered(t *testing.T) {
	// Compile-time: both known types satisfy the closed interface.
	var _ ListingPatch = ListingTextPatch{}
	var _ ListingPatch = ListingSourceRepoPatch{}

	want := map[string]bool{"ListingTextPatch": true, "ListingSourceRepoPatch": true}

	// 🔴 ParseFile PER FILE, NOT parser.ParseDir — the latter is deprecated as of
	// Go 1.25 (SA1019) because it ignores build tags. Walking the directory here
	// keeps the enumeration honest for THIS package's own files, which is all it
	// claims to cover.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	found := map[string]bool{}
	files := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, err)
		}
		files++
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "wire" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, isPtr := recv.(*ast.StarExpr); isPtr {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				found[id.Name] = true
			}
		}
	}
	// 🔴 POSITIVE CONTROL. A parse that read no files, or a `wire` spelling that
	// matched nothing, would report an empty set and pass every check below —
	// the reassuring zero this repo keeps getting caught by.
	if files == 0 {
		t.Fatal("CONTROL failure: parsed 0 non-test files; the enumeration below is vacuous")
	}
	if len(found) == 0 {
		t.Fatal("CONTROL failure: found 0 `wire()` methods; the enumeration below is vacuous")
	}

	for name := range found {
		if !want[name] {
			t.Errorf("%s implements ListingPatch but is not ledgered here.\n"+
				"A new patch type is the moment to decide whether its fields are MATERIAL: a type "+
				"carrying both a text field and sourceRepoUrl would stage the text on a revision "+
				"instead of applying it in place. Add it above only after checking that.", name)
		}
	}
	for name := range want {
		if !found[name] {
			t.Errorf("%s is ledgered as a ListingPatch implementation but declares no wire() method "+
				"— a stale entry looks like coverage and is not", name)
		}
	}

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
