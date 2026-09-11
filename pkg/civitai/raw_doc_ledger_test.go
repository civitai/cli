package civitai

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// rawBearingTypes is the LEDGER of exported result types carrying a `Raw
// []byte` field. It fails in BOTH directions: a new result type that exposes
// Raw and is not listed (the set grew — it needs the disclaimer too), and a
// listed type that no longer has the field (the set shrank — this ledger and
// getInto's cross-reference have gone stale).
//
// It is a ledger and not a count because a count is tradeable: delete one
// type's row and add another's and the number does not move.
var rawBearingTypes = []string{
	"AppSearchResult",
	"ArticleSearchResult",
	"CollectionSearchResult",
	"CreatorSearchResult",
	"ImageSearchResult",
	"ModelSearchResult",
	"TagSearchResult",
	"UserSearchResult",
}

// rawDisclaimer is the sentence every one of those types must carry. It is the
// wording civitai/cli#526's follow-up put on CollectionSearchResult and
// ArticleSearchResult; this test is what makes the other six say it too.
const rawDisclaimer = "NOT promised to be the server's own bytes"

// TestRawDocCommentsDisclaimByteIdentity is round 3's finding 1.
//
// 🔴 A CROSS-REFERENCE IS ONLY AS TRUE AS ITS REFERENTS, AND THIS ONE WAS TRUE
// OF TWO OUT OF EIGHT. getInto's doc comment says the bytes it returns are the
// REPAIRED body and points the reader at "the note on Raw in the result types".
// That note was written on two of the eight types that have a Raw field; the
// other six still said Raw was "the raw response body" / "the raw body", which
// is the byte-identity claim the same branch was correcting everywhere else.
// The evidence file went further and asserted the sweep was CLOSED at three
// sites; at least seven more were live.
//
// The fix for a claim nothing asserts on is not a better sentence. This is the
// assertion: every type in the ledger must carry the disclaimer, and the ledger
// must be exactly the set of types that have the field.
//
// 🔴 WHAT THIS DOES **NOT** COVER, STATED SO IT IS NOT READ AS WIDER THAN IT IS:
//
//   - The DETAIL getters (GetModel, GetArticle, GetCollection, GetApp,
//     GetModelVersion, GetModelVersionByHash) return raw bytes as a bare second
//     `[]byte` return value, not as a struct field. They have no doc-comment
//     slot this test can find, and they are not checked.
//   - Narrative prose elsewhere in the package — "preserved for --json via the
//     raw body" on ModelDetail, ArticleDetail, ImageItem and friends — is a claim
//     about FIELD preservation (a key the struct does not model survives into
//     --json), which is true, and is deliberately left alone. A future comment
//     that phrases a BYTE claim in a place this test does not read is not
//     caught. That residual is why the enumeration in
//     claudedocs/decisions/38-read-body-repair-and-snippet.md is written as open
//     rather than as a closed count.
func TestRawDocCommentsDisclaimByteIdentity(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := 0
	docs := map[string]string{}
	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, perr)
		}
		files++
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					continue
				}
				if !hasRawBytesField(st) {
					continue
				}
				found = append(found, ts.Name.Name)
				// A `type ( … )` block shares one doc comment; a lone `type X
				// struct` carries it on the GenDecl. Accept either.
				doc := ""
				if ts.Doc != nil {
					doc = ts.Doc.Text()
				} else if gd.Doc != nil {
					doc = gd.Doc.Text()
				}
				docs[ts.Name.Name] = doc
			}
		}
	}

	// POSITIVE CONTROLS, before any verdict. A parser that walked nothing, or
	// matched no type, reports a reassuring zero that is indistinguishable from
	// "everything is fine".
	if files < 5 {
		t.Fatalf("CONTROL failure, not a finding: parsed only %d non-test source files "+
			"in pkg/civitai — the scan is not reading the package", files)
	}
	// 🔴 THIS IS A HARD FLOOR OF ONE, NOT A FLOOR OF len(rawBearingTypes), AND
	// THE DIFFERENCE IS THE WHOLE POINT. It used to read
	// `len(found) < len(rawBearingTypes)`, which fires on EVERY shrink — so a
	// type that genuinely dropped its Raw field was reported as "CONTROL
	// failure, not a finding" and the SHRANK guidance below was unreachable.
	// The label was wrong in the one direction the ledger exists for. A floor of
	// one still separates a broken detector (matches nothing at all) from a
	// finding (matches fewer than ledgered), and the comparison below now owns
	// every shrink.
	if len(found) == 0 {
		t.Fatalf("CONTROL failure, not a finding: the `Raw []byte` field detector "+
			"matched no type at all across %d parsed files — it is broken, or every "+
			"result type dropped the field at once.", files)
	}

	sort.Strings(found)
	want := append([]string(nil), rawBearingTypes...)
	sort.Strings(want)
	if strings.Join(found, ",") != strings.Join(want, ",") {
		t.Errorf("types with a `Raw []byte` field are %v, ledgered as %v.\n"+
			"GREW: a new result type exposes Raw — give it the disclaimer and add it here.\n"+
			"SHRANK: a type dropped the field — getInto's \"see the note on Raw in the\n"+
			"        result types\" now points at one fewer referent.",
			found, want)
	}

	for _, name := range want {
		doc, ok := docs[name]
		if !ok {
			continue // already reported by the ledger comparison above
		}
		// Match on WHITESPACE-NORMALISED text. A doc comment is hard-wrapped, so
		// the sentence lands with a newline at a different word in every file —
		// a raw Contains reports "missing" for six comments that carry it, which
		// is the instrument lying, not a finding. (Measured: it did exactly that
		// on the first run of this test.)
		if !strings.Contains(strings.Join(strings.Fields(doc), " "), rawDisclaimer) {
			t.Errorf("%s's doc comment does not carry %q.\n"+
				"Raw is the body that DECODED, which is the REPAIRED body when the wire\n"+
				"body would not decode (EscapeJSONStringControlChars). Six of these eight\n"+
				"comments claimed byte identity for a full branch while the same branch\n"+
				"was correcting the claim elsewhere.\ngot:\n%s",
				name, rawDisclaimer, doc)
		}
	}
}

// hasRawBytesField reports whether st declares a field named Raw of type []byte.
func hasRawBytesField(st *ast.StructType) bool {
	for _, fld := range st.Fields.List {
		at, ok := fld.Type.(*ast.ArrayType)
		if !ok || at.Len != nil {
			continue
		}
		id, ok := at.Elt.(*ast.Ident)
		if !ok || id.Name != "byte" {
			continue
		}
		for _, n := range fld.Names {
			if n.Name == "Raw" {
				return true
			}
		}
	}
	return false
}
