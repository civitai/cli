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
// means a site now spells the ratings by hand that did not before — the most
// likely shape of that is a new assertion comparing the literal against real
// validator output, i.e. the self-block re-created somewhere new. A SHRINK means
// the shape anchor in `pattern_test.go` was deleted or re-derived.
//
// ⚠ WHAT A SHRINK COSTS — stated NARROWLY, because an earlier version of this
// comment overstated it and the overstatement was measured false. It said that
// with the anchor gone "a reword of the library's lead-in moves `want` and `got`
// together and nothing reds". It does NOT: derive BOTH of the anchor's operands
// from the schema, then reword the lead-in where the code consumes the library
// message, and `TestEnumFindingsKeepTheirExactWording` still fails — because the
// wording lives in `enumMessage`'s own hand-written source text, not in the
// schema, so "read out of the schema" is unsatisfiable for the wording half.
// What the anchor actually buys is narrower and real: it stops a reword from
// being PAPERED OVER by editing `enumMessage` to match the new wording. The
// anchor's own comment in `pattern_test.go` states that correctly; this one used
// to promise more than the code delivers.
//
// The file SET and the per-file COUNT are both asserted, so a spelling added
// inside a file that is already ledgered is caught too — for every ledgered file,
// not just the anchor.
//
// 🔴 WHAT IT DOES NOT CATCH, stated so the comment is not wider than the code.
// The needle is a SPELLING — the first three ratings in order, in either Go-string
// or single-quoted-message form, whitespace-tolerant. A site that builds the same
// list a different way (`strings.Split("g,pg,pg13,r,x", ",")`, a subset, a
// different order) is invisible to it. It is a regression guard on the shape that
// has actually shipped twice, not a proof that no hand-typed rating set exists.
//
// ⚠ AND THE ATTRIBUTION HERE USED TO BE CIRCULAR. This comment pointed at
// `internal/cmd`'s two controls as where "the surviving derivations are real
// derivations" is exercised, while those controls' own comments pointed back here.
// Measured, neither delivers it: a hardcode conditional on the real field name,
// and a frozen read half, both leave `internal/cmd` green — this ledger is what
// reds, and only when the literal is SPELLED. So the honest statement is that
// between them they cover a spelled re-mirror and nothing else; a list built by
// `strings.Split` evades all three. Each file now states its own scope.
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

// ratingLedgerEntry is one permitted writer: why it may spell the list, and HOW
// MANY times. The count is per entry rather than a single constant for one file —
// an earlier version pinned only the anchor, so a second spelling added to
// `pattern.go` (the other ledgered file) left this test PASSING while its own
// comment claimed a file-set check "cannot see that" and implied the count closed
// it. Measured: needle count there went 1 → 2 and the ledger stayed green.
type ratingLedgerEntry struct {
	reason string
	// want is the exact number of needle matches permitted in this file. Every
	// ledgered file carries one, so the pin is as wide as the sentence above it.
	want int
}

// ratingLiteralLedger is the EXACT set of Go files permitted to spell the rating
// list by hand. Anything else is a regression, in either direction.
var ratingLiteralLedger = map[string]ratingLedgerEntry{
	// The illustrative example in pattern.go's package doc: it explains what an
	// enum finding looks like, to justify why a `pattern` finding gains a gloss.
	// Prose about a specimen message, asserted by nothing.
	"internal/validate/pattern.go": {
		reason: "doc comment showing a specimen enum finding",
		want:   1,
	},

	// The SHAPE ANCHOR, twice: once in the expected-message constant, once in the
	// hand-written list fed to the template. Both operands of that comparison are
	// hand-written and neither reads the schema, so it is a frozen specimen rather
	// than a mirror and an additive canonical cannot reach it.
	"internal/validate/pattern_test.go": {
		reason: "the frozen shape anchor for enumMessage (const + hand-written list)",
		want:   2,
	},
}

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
					"  If the shape anchor is gone, a reword of the validation library's lead-in can be\n"+
					"  PAPERED OVER by editing enumMessage to match it, and nothing would object. (It does\n"+
					"  NOT mean nothing can fail — a reword still reds the want-map comparison against real\n"+
					"  output; that wider claim was measured false.) Restore a hand-written specimen, or\n"+
					"  delete this ledger entry and say in its place what now pins the wording.",
					p, ratingLiteralLedger[p].reason)
			}
		}
	}

	// PER-ENTRY COUNTS, and the two directions get DIFFERENT advice — which is the
	// whole point, because they have opposite remedies and an earlier version gave
	// the GROW remedy to both.
	//
	// 🔴 THAT WAS ACTIVELY HARMFUL, MEASURED. The old message said "a third
	// spelling in this file is how the self-block came back last time … an
	// expectation compared against real output must DERIVE its values instead."
	// At the base commit the anchor file held ONE spelling, not three: the block
	// existed because the anchor's OTHER operand read the schema. So the fired-on
	// case is n BELOW want, and the reader was told it was a third spelling
	// (false) and instructed to derive (the edit that re-creates the block).
	for _, p := range want {
		n, present := found[p]
		if !present {
			continue // already reported as SHRANK above
		}
		e := ratingLiteralLedger[p]
		switch {
		case n > e.want:
			t.Errorf("GREW WITHIN A LEDGERED FILE: %s spells the rating list %d times, want %d (%s).\n"+
				"  An extra hand-typed spelling here is a new mirror. If it is compared against the\n"+
				"  schema's enum or against real validator output it is the self-block again — DERIVE\n"+
				"  those values from cli.SchemaJSON and spell only the wording. If it is genuinely a\n"+
				"  second frozen specimen, raise this entry's want and say why.", p, n, e.want, e.reason)
		case n < e.want:
			t.Errorf("A FROZEN SPECIMEN WENT MISSING: %s spells the rating list %d times, want %d (%s).\n"+
				"  🔴 DO NOT 'fix' this by deriving the values — deriving is what makes the guard\n"+
				"  self-blocking, and it is the exact edit this whole change exists to undo. The anchor\n"+
				"  needs BOTH operands hand-written: the expected-message constant AND the list handed\n"+
				"  to enumMessage. Restore the hand-written one.", p, n, e.want, e.reason)
		}
	}
}
