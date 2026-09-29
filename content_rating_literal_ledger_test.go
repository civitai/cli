package cli_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// content_rating_literal_ledger_test.go pins WHERE a hand-typed `contentRating`
// rating list may appear in Go source, because that literal has now been a
// self-blocking guard TWICE, in two different files, and the second copy is what
// made the obvious one-site fix read as complete when it was not.
//
// 🔴 THE DEFECT THIS EXISTS TO PREVENT. A guard that spells the rating set out
// and compares it against something derived from the schema — the schema's own
// enum, or the validator's rendered message — cannot ever accept an additive
// canonical. `ci.yml` runs `schema-drift` on `pull_request:` with NO path filter,
// so one additive canonical then reds a check on EVERY unrelated human PR in the
// repo until somebody edits the literal by hand. That is the failure
// `internal/validate/pattern_test.go` was rewritten to remove; the rewrite moved
// it one field over, from `scopes` to `contentRating`, and left a second
// independent spelling in `internal/cmd/message_quality_test.go`.
//
// 🔴 IT FAILS IN BOTH DIRECTIONS, which is the whole point of a ledger. A GROW
// means a third site now spells the ratings by hand — the most likely shape of
// that is a new assertion comparing the literal against real validator output,
// i.e. the self-block re-created somewhere new. A SHRINK means the shape anchor
// in `pattern_test.go` was deleted, and with it the only thing standing between a
// fully-derived expectation and a test that cannot fail: if both the template and
// the message are read out of the schema, a reword of the library's lead-in moves
// `want` and `got` together and nothing reds.
//
// 🔴 WHAT IT DOES NOT CATCH, stated so the comment is not wider than the code.
// The needle is a SPELLING — the first three ratings in order, in either Go-string
// or single-quoted-message form, whitespace-tolerant. A site that builds the same
// list a different way (`strings.Split("g,pg,pg13,r,x", ",")`, a subset, a
// different order) is invisible to it. It is a regression guard on the shape that
// has actually shipped twice, not a proof that no hand-typed rating set exists.
// The claims that the surviving derivations are real derivations live where they
// can be exercised: `internal/cmd`'s `TestEnumFindingMessageIsAFormatterNotACopyOfTheRatings`
// and `TestSchemaEnumValuesReadsTheLiveVendoredRatings`.
//
// 🔴 THE NEEDLE IS A FROZEN LITERAL ON PURPOSE AND MUST NOT BE DERIVED FROM THE
// SCHEMA. Deriving it would re-create the very bug: a canonical that grows the
// ratings would move the needle, stop it matching `pattern_test.go`'s frozen
// five-value specimen, and this ledger would report a SHRINK — a guard over a
// self-block, self-blocking. Because the needle is frozen and hand-typed, this
// file matches it too, so this file is excluded from its own walk by path.

// ratingListNeedle matches a hand-typed enumeration of the first three ratings in
// schema order, in either quoting style:
//
//	'g', 'pg', 'pg13'     — as rendered inside a finding message
//	"g", "pg", "pg13"     — as a Go string slice
var ratingListNeedle = regexp.MustCompile(`['"]g['"]\s*,\s*['"]pg['"]\s*,\s*['"]pg13['"]`)

// ratingLiteralLedger is the EXACT set of Go files permitted to spell the rating
// list by hand, with the reason each is allowed. Anything else is a regression.
var ratingLiteralLedger = map[string]string{
	// The illustrative example in pattern.go's package doc: it explains what an
	// enum finding looks like, to justify why a `pattern` finding gains a gloss.
	// Prose about a specimen message, asserted by nothing.
	"internal/validate/pattern.go": "doc comment showing a specimen enum finding",

	// The SHAPE ANCHOR. Both operands of that comparison are hand-written and
	// neither reads the schema, so it is a frozen specimen rather than a mirror
	// and an additive canonical cannot reach it. It is what makes a reword of the
	// library's lead-in impossible to paper over by editing the template.
	"internal/validate/pattern_test.go": "the frozen shape anchor for enumMessage",
}

// wantAnchorOccurrences is how many times the needle may appear in the anchor
// file: once in the expected-message constant, once in the hand-written list fed
// to the template. Pinning the COUNT is what catches a literal re-added inside a
// file that is already on the ledger — a file-set check alone cannot see that.
const wantAnchorOccurrences = 2

func TestContentRatingLiteralHasExactlyTheLedgeredWriters(t *testing.T) {
	self := "content_rating_literal_ledger_test.go"

	found := map[string]int{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || path == self {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if n := len(ratingListNeedle.FindAll(b, -1)); n > 0 {
			found[filepath.ToSlash(path)] = n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	// POSITIVE CONTROL. A walk that silently matched nothing — a broken needle, a
	// wrong cwd, an over-eager SkipDir — would report an empty set, and an empty
	// set read against a ledger of two would look like two SHRINKs rather than
	// like an instrument that never ran. Zero is the reading this guard must never
	// return quietly.
	if len(found) == 0 {
		t.Fatal("the needle matched NOTHING anywhere in the tree — this guard observed nothing. " +
			"Either the walk is rooted somewhere unexpected or ratingListNeedle no longer matches " +
			"the spelling it is written for; fix the instrument before trusting any verdict from it")
	}

	var got, want []string
	for p := range found {
		got = append(got, p)
	}
	for p := range ratingLiteralLedger {
		want = append(want, p)
	}
	sort.Strings(got)
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		for _, p := range got {
			if _, ok := ratingLiteralLedger[p]; !ok {
				t.Errorf("GREW: %s spells the contentRating list by hand and is not on the ledger.\n"+
					"  If it compares that literal against the schema's enum or against real validator\n"+
					"  output, it is the self-block again: an additive canonical will red it, and with it\n"+
					"  `schema-drift` on every unrelated PR in this repo. DERIVE the values from\n"+
					"  cli.SchemaJSON and spell only the wording, as internal/cmd's\n"+
					"  schemaEnumFindingMessage does. If it is genuinely a frozen specimen that reads\n"+
					"  nothing, add it here with the reason.", p)
			}
		}
		for _, p := range want {
			if _, ok := found[p]; !ok {
				t.Errorf("SHRANK: %s no longer spells the contentRating list (%s).\n"+
					"  If the shape anchor is gone, the enum expectations are derived on BOTH sides and a\n"+
					"  reword of the validation library's lead-in moves want and got together — nothing\n"+
					"  can fail. Restore a hand-written specimen, or delete this ledger entry and say in\n"+
					"  its place what now pins the wording.", p, ratingLiteralLedger[p])
			}
		}
	}

	if n := found["internal/validate/pattern_test.go"]; n != 0 && n != wantAnchorOccurrences {
		t.Errorf("the shape anchor file spells the rating list %d times, want %d — a third spelling in "+
			"this file is how the self-block came back last time. The anchor is the constant and the "+
			"hand-written list fed to enumMessage; an expectation compared against real output must "+
			"DERIVE its values instead.", n, wantAnchorOccurrences)
	}
}
