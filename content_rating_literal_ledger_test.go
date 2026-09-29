package cli_test

import (
	"go/ast"
	"go/parser"
	"go/token"
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
// ⚠ AND THEN TOO LITTLE — the narrowing above was itself incomplete, because the
// same commit that wrote it gave the anchor a SECOND job. It is the only thing
// pinning `frozenRatings`, the specimen the subset shrink guard in
// `pattern_test.go` iterates. Measured: delete the anchor and trim `frozenRatings`
// to one value and both this ledger and `internal/validate` stay green, with the
// shrink guard asserting almost nothing. Both jobs are now named in the SHRANK
// message. Recorded as a pair because the correction went too wide, then too
// narrow, and the next reader should not have to rediscover either direction.
//
// THREE things are asserted, and the third is not a spelling: the file SET, the
// per-file COUNT (so a spelling added inside an already-ledgered file is caught, for
// every entry and not just the anchor), and the anchor specimen's PROVENANCE against
// the syntax tree — see `assertFrozenRatingsIsHandWritten`.
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

// anchorRatingsVar names the anchor's specimen variable in
// `internal/validate/pattern_test.go`. Its PROVENANCE is asserted against that
// file's syntax tree by `assertFrozenRatingsIsHandWritten` below.
//
// 🔴 A REGRESSION THIS LADDER INTRODUCED, AND THEN GUARDED TWICE BADLY BEFORE
// GUARDING IT STRUCTURALLY — the history is kept because each step was measured.
// The anchor's hand-written list was hoisted into this variable so the subset shrink
// guard could reuse it rather than the ratings being typed a third time. That gave
// the literal a SECOND consumer, and the count pin above counts SPELLINGS: so
// re-deriving the anchor's operand while leaving the variable alive for the subset
// loop kept the count at 2 and the ledger GREEN, with the self-block returning the
// moment a canonical added a rating — on `ci.yml`'s path-filter-free `schema-drift`,
// a red check on every unrelated PR in the repo.
//
// The first repair pinned the CALL SITE TEXT with a regex. That had its own measured
// false green: a REASSIGNMENT after the declaration —
//
//	frozenRatings := []string{"g", "pg", "pg13", "r", "x"}
//	frozenRatings = schemaEnum(t, "properties", "contentRating", "enum")  // hazard
//
// left the pinned text matching, the spelling count at 2, every guard green, and the
// self-block fully restored on the next additive canonical.
//
// 🔴 THE LESSON, AFTER THREE SPELLING-BASED GUARDS IN THIS FILE WERE WALKED: the
// property is PROVENANCE — hand-written or schema-derived — and no comparison of
// VALUES can see it, because at rest the two are EQUAL. That equality is the whole
// reason the original defect shipped unnoticed. Provenance is a fact about the
// SOURCE, so it is asserted against the syntax tree, which also retires the regex's
// false reds: a rename or a wrapped argument no longer matters.
const anchorRatingsVar = "frozenRatings"

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
	// remedy names WHAT to restore when this entry's count drops, in this file's
	// own terms.
	//
	// 🔴 IT IS PER-ENTRY BECAUSE A SHARED REMEDY WAS MEASURED WRONG. The shrink arm
	// used to print the anchor's remedy — "the expected-message constant AND the
	// list handed to enumMessage" — for EVERY ledgered file. Mistyping
	// `pattern.go`'s want as 2 fired it against a file that contains no constant,
	// no `enumMessage` and no anchor, telling the reader to restore things that
	// were never there. That is the same defect the arm itself was created to fix,
	// reproduced one level up.
	remedy string
}

// anchorFile is the file whose enumMessage call site is pinned by
// anchorCallNeedle. Named once so the ledger entry and the check cannot drift.
const anchorFile = "internal/validate/pattern_test.go"

// ratingLiteralLedger is the EXACT set of Go files permitted to spell the rating
// list by hand. Anything else is a regression, in either direction.
var ratingLiteralLedger = map[string]ratingLedgerEntry{
	// The illustrative example in pattern.go's package doc: it explains what an
	// enum finding looks like, to justify why a `pattern` finding gains a gloss.
	// Prose about a specimen message, asserted by nothing.
	"internal/validate/pattern.go": {
		reason: "doc comment showing a specimen enum finding",
		want:   1,
		remedy: "the specimen enum finding in the package doc comment — prose, asserted by nothing",
	},

	// The SHAPE ANCHOR, twice: once in the expected-message constant, once in
	// `frozenRatings`. Both operands of that comparison are hand-written and neither
	// reads the schema, so it is a frozen specimen rather than a mirror and an
	// additive canonical cannot reach it.
	anchorFile: {
		reason: "the frozen shape anchor for enumMessage (const + frozenRatings)",
		want:   2,
		remedy: "the `wantContentRating` constant and the `frozenRatings` slice — both hand-written",
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
				// 🔴 THE ANCHOR-SPECIFIC PARAGRAPHS ARE GATED ON THE FILE, and an earlier
				// version printed them for EVERY ledgered file. Trimming the prose specimen
				// in `internal/validate/pattern.go` — an ordinary comment edit — fired a
				// fourteen-line lecture about an anchor, a constant and `frozenRatings`, none
				// of which that file contains, while the real anchor sat intact in another
				// file. The per-entry `remedy` existed and went unprinted. Same defect the
				// `n<want` arm was split to fix, in the arm that round did not touch.
				e := ratingLiteralLedger[p]
				msg := "SHRANK: %s no longer spells the contentRating list (%s).\n" +
					"  Restore the hand-written spelling this entry is for: " + e.remedy + "\n"
				if p == anchorFile {
					msg += "  🔴 The anchor has TWO jobs and losing it costs both.\n" +
						"  (1) WORDING: a reword of the validation library's lead-in can then be PAPERED\n" +
						"      OVER by editing enumMessage to match it, and nothing would object. (It does\n" +
						"      NOT mean nothing can fail — a reword still reds the want-map comparison\n" +
						"      against real output; that wider claim was measured false.)\n" +
						"  (2) SPECIMEN: the anchor is what keeps `" + anchorRatingsVar + "` a hand-written\n" +
						"      value at all — it is the subset shrink guard's specimen. Measured: with the\n" +
						"      anchor deleted, trimming it to one value leaves this package AND this ledger\n" +
						"      green, and the shrink guard then asserts almost nothing.\n" +
						"      ⚠ The anchor pins the value's EXISTENCE, not the shrink guard's USE of it —\n" +
						"      swapping that loop to range the schema's own values is a separate hazard, and\n" +
						"      `assertFrozenRatingsIsHandWritten` is what catches that one.\n" +
						"  So: restore it, or delete this ledger entry and say in its place what now pins\n" +
						"  the WORDING *and* what pins `" + anchorRatingsVar + "`. An earlier version asked\n" +
						"  only about the wording, so a reader who followed it exactly destroyed the shrink\n" +
						"  guard and saw nothing red.\n" +
						"  ⚠ If one of the FIRST THREE ratings was retired upstream this arm fires even\n" +
						"  though the anchor is INTACT — `ratingListNeedle` is frozen on `g, pg, pg13`, so\n" +
						"  a consistent retirement drops BOTH spellings and the file leaves the walk\n" +
						"  entirely. In that one case hand-edit the needle; deriving it never is correct."
				}
				t.Errorf(msg, p, e.reason)
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
				"  self-blocking, and it is the exact edit this whole change exists to undo. Restore the\n"+
				"  hand-written spelling this entry is for: %s\n"+
				"  OR, if the removal was deliberate and this entry's number is simply now wrong, lower\n"+
				"  its `want` and say why. A mis-typed want and a real regression print the same thing,\n"+
				"  so that branch has to be offered explicitly.\n"+
				"  (The upstream-retirement case does NOT reach this arm — a consistent retirement of\n"+
				"  one of the first three ratings drops every spelling in the file, so it routes to\n"+
				"  SHRANK above. That note lived here and was measured unreachable.)",
				p, n, e.want, e.reason, e.remedy)
		}
	}

	if _, ok := ratingLiteralLedger[anchorFile]; ok {
		assertFrozenRatingsIsHandWritten(t)
	}
}

// assertFrozenRatingsIsHandWritten pins the PROVENANCE of the anchor's specimen
// against the anchor file's syntax tree: it must be declared once from a literal,
// never reassigned, and it must be the thing the subset shrink guard ranges over.
//
// 🔴 EACH OF THE THREE CLAUSES CLOSES A MEASURED FALSE GREEN, and all three were
// invisible to the spelling counts above — which is the reason this is an AST check
// and not a fourth regex:
//
//	declared-from-a-literal   `frozenRatings := schemaEnum(…)` makes the anchor a
//	                          mirror again. Caught before the specimen was hoisted
//	                          into a variable; silent afterwards.
//	never-reassigned          `frozenRatings = schemaEnum(…)` on a later line keeps
//	                          the declaration, the spelling count AND the call-site
//	                          text intact while the value becomes schema-derived.
//	ranged-by-the-subset-loop `range frozenRatings` -> `range ratings` compiles, is
//	                          green everywhere, and makes the subset assertion
//	                          tautological — so the retirement coverage this PR
//	                          added to replace what it removed silently disappears.
//
// All three share one shape: the guarded property is whether a value is HAND-WRITTEN
// or SCHEMA-DERIVED, and at rest those two are equal, so no comparison of values can
// tell them apart. That equality is exactly why the original defect shipped unseen.
//
// ⚠ Scope, so this comment is not wider than its code: it reads ONE function in ONE
// file and asserts three syntactic facts about one identifier. It does not prove the
// anchor is correct, and a rename of the identifier reds it — update
// `anchorRatingsVar`. It cannot see a hazard routed through a helper that returns a
// schema read while looking literal at this call depth; that residue is unguarded and
// is recorded here rather than implied away.
func assertFrozenRatingsIsHandWritten(t *testing.T) {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, anchorFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing the anchor file %s: %v", anchorFile, err)
	}

	var decls, reassigns, rangedBy int
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != anchorRatingsVar {
					continue
				}
				if node.Tok == token.DEFINE {
					decls++
					// The RHS must be a literal, not a call. A call here is the
					// mirror the whole change exists to remove.
					if i < len(node.Rhs) {
						if _, isLit := node.Rhs[i].(*ast.CompositeLit); !isLit {
							t.Errorf("%s declares %s from something other than a composite literal.\n"+
								"  🔴 If that is a schema read, THE SELF-BLOCK IS BACK: the anchor becomes a\n"+
								"  mirror of the live enum, and an additive canonical reds it — and with it\n"+
								"  `schema-drift` on every unrelated PR in this repo. The specimen must be\n"+
								"  hand-written.", anchorFile, anchorRatingsVar)
						}
					}
				} else {
					reassigns++
				}
			}
		case *ast.RangeStmt:
			if id, ok := node.X.(*ast.Ident); ok && id.Name == anchorRatingsVar {
				rangedBy++
			}
		}
		return true
	})

	if decls != 1 {
		t.Errorf("%s declares %s %d times, want exactly 1 — this check reads the wrong thing "+
			"if the identifier moved or was renamed; update anchorRatingsVar.",
			anchorFile, anchorRatingsVar, decls)
	}
	if reassigns != 0 {
		t.Errorf("%s REASSIGNS %s %d time(s) after declaring it.\n"+
			"  🔴 A reassignment is how the specimen becomes schema-derived while every textual\n"+
			"  guard stays green: the declaration, the spelling count and the call site are all\n"+
			"  unchanged, and the self-block returns the moment a canonical adds a rating.\n"+
			"  If the reassignment is genuinely hand-written data, inline it into the declaration.",
			anchorFile, anchorRatingsVar, reassigns)
	}
	if rangedBy != 1 {
		t.Errorf("%s ranges over %s %d times, want exactly 1 — the subset shrink guard.\n"+
			"  🔴 If that loop now ranges the SCHEMA's values instead, the assertion is tautological\n"+
			"  and green forever: the retirement coverage this PR added to replace the coverage it\n"+
			"  removed is gone, and an upstream retirement becomes invisible repo-wide. The loop must\n"+
			"  range the hand-written specimen and compare it AGAINST the schema.",
			anchorFile, anchorRatingsVar, rangedBy)
	}
}
