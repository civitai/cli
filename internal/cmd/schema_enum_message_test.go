package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	cli "github.com/civitai/cli"
)

// schema_enum_message_test.go lets internal/cmd assert an enum finding's WORDING
// without hand-typing the enum's CONTENT.
//
// 🔴 WHY IT EXISTS. `TestValidateEnumFindingWordingIsUnchanged` used to compare
// the real `app validate` stderr against a hand-typed copy of the entire
// `contentRating` message, rating list and all. That is a
// guard over a MIRROR that pins CONTENT, so it could never accept an additive
// canonical — and `ci.yml` runs `schema-drift` on every PR in the repo with no
// path filter, so one such canonical reds a check on every unrelated human PR
// until somebody edits this literal by hand. The same defect was fixed in
// `internal/validate/pattern_test.go` and this was its second, independent
// spelling.
//
// 🔴 THE SPLIT, WHICH IS THE POINT: the VALUES are derived from the vendored
// schema; the WORDING — the lead-in, the single quotes, the ", " separator — is
// spelled out below by hand. So an enum that GROWS moves both sides together and
// stays green, while a reword of the library's rendering moves only the real
// output and reds. Deriving both halves would be the trap on the other side:
// `want` and `got` would move together on a reword and nothing could fail.
//
// 🔴 IT IS DELIBERATELY NOT SHARED WITH `internal/validate`'s `schemaEnum`, and
// that is not an oversight. What is duplicated is the WORDING CLAIM, and each
// package must be able to fail on it alone: if internal/cmd imported the
// template, a reword would be papered over by one edit and this surface — the
// one an author actually sees — would have no independent assertion left. The
// enum CONTENT, which is the volatile half, is not duplicated anywhere.

// schemaEnumFindingMessage renders the enum finding the validator must produce
// for `field`, reading the allowed values out of the vendored schema at an
// explicit key path.
//
// The path is spelled by the caller rather than searched for: a search would find
// "some enum" and could silently move to a different one.
func schemaEnumFindingMessage(t *testing.T, field string, path ...string) string {
	t.Helper()
	return enumFindingMessage(field, schemaEnumValues(t, path...))
}

// enumFindingMessage is the wording, written out once and literally. It takes its
// values as an argument and reads nothing, so it is testable against a set the
// schema could never contain — which is what proves it is a formatter rather than
// a disguised copy of the ratings.
func enumFindingMessage(field string, allowed []string) string {
	quoted := make([]string, len(allowed))
	for i, v := range allowed {
		quoted[i] = "'" + v + "'"
	}
	return field + ": value must be one of " + strings.Join(quoted, ", ")
}

// schemaEnumValues returns the `enum` the VENDORED schema declares at an explicit
// key path, in the schema's own order — which is the order the validation library
// renders them in, so a derived message matches byte for byte.
//
// Every way of not-finding it is FATAL, because an expectation derived from a
// missing or empty enum is vacuous rather than wrong: `enum: []` would render
// "value must be one of " on BOTH sides and agree.
func schemaEnumValues(t *testing.T, path ...string) []string {
	t.Helper()
	var doc any
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		t.Fatalf("vendored schema does not decode: %v", err)
	}
	node := doc
	for i, key := range path {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v: %v is not an object, so %q cannot be read",
				path, path[:i], key)
		}
		if node, ok = m[key]; !ok {
			t.Fatalf("schema path %v: no %q — this test is reading a shape the schema "+
				"no longer has, so its expectation is vacuous", path, key)
		}
	}
	raw, ok := node.([]any)
	if !ok {
		t.Fatalf("schema path %v does not resolve to an array", path)
	}
	if len(raw) == 0 {
		t.Fatalf("schema path %v resolves to an EMPTY enum — a message derived from it "+
			"would agree with anything", path)
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("schema path %v index %d is %T, not a string", path, i, v)
		}
		out[i] = s
	}
	return out
}

// TestEnumFindingMessageIsAFormatterNotACopyOfTheRatings is the POSITIVE CONTROL
// on the derivation, and it is what makes the derived expectation worth trusting.
//
// It feeds the formatter a set the `contentRating` enum cannot contain and pins the
// whole message, so a formatter that reorders its values, changes the separator or
// drifts the lead-in fails here.
//
// ⚠ WHAT IT DOES NOT CATCH — narrowed after measurement, because the earlier
// wording ("any hardcoding … fails here") was FALSE. This test only ever passes the
// field names it spells below, so a hardcode CONDITIONAL on the real field —
// `if field == "contentRating" { return <the literal> }` — leaves this test, and all
// of `internal/cmd`, GREEN. Read it as proving the FORMATTER is a formatter, not as
// proving the derivation is real. The no-re-spelling claim is carried by
// `content_rating_literal_ledger_test.go`, and that keys on a SPELLING, so a list
// built another way (`strings.Split("g,pg,pg13,r,x", ",")`) evades both. That
// residue is known and unguarded; it is recorded rather than implied away.
func TestEnumFindingMessageIsAFormatterNotACopyOfTheRatings(t *testing.T) {
	// 'aa'/'bb' are not ratings and never will be, so this expectation cannot be
	// satisfied by a function that returns the real set.
	if got, want := enumFindingMessage("someField", []string{"aa", "bb"}),
		"someField: value must be one of 'aa', 'bb'"; got != want {
		t.Errorf("the enum finding wording moved\n  want: %s\n  got:  %s", want, got)
	}
	// Single value: no separator. Pinned because a `strings.Join` replaced by a
	// hand-rolled loop is exactly where a trailing ", " appears.
	if got, want := enumFindingMessage("f", []string{"only"}), "f: value must be one of 'only'"; got != want {
		t.Errorf("single-value wording moved\n  want: %s\n  got:  %s", want, got)
	}
	// FIVE values, because the rows above are 1- and 2-element and a formatter that
	// truncates only a LONGER list escapes both — measured with
	// `if len(allowed) > 3 { allowed = allowed[:len(allowed)-1] }`, which left this
	// test green. Five is the live rating count, so this also exercises the length
	// the real message is rendered at.
	//
	// ⚠ IT BUYS ATTRIBUTION, NOT A KILL, and the commit that added it claimed
	// otherwise. That same truncation mutant was ALREADY dead at the pre-fix tip —
	// `TestSchemaEnumValuesReadsTheLiveVendoredRatings` catches it, because the
	// derived message then omits a rating the schema declares. So the suite did not
	// gain coverage here; what it gained is a failure that names the FORMATTER
	// instead of the schema read, which is the difference between a five-minute
	// diagnosis and a wrong one. Worth keeping, worth not overstating.
	if got, want := enumFindingMessage("five", []string{"a", "b", "c", "d", "e"}),
		"five: value must be one of 'a', 'b', 'c', 'd', 'e'"; got != want {
		t.Errorf("multi-value wording moved\n  want: %s\n  got:  %s", want, got)
	}
}

// TestSchemaEnumValuesReadsTheLiveVendoredRatings proves the READ half reaches the
// schema, which the formatter test above cannot see.
//
// It asserts against the schema's own parsed content rather than against a copied
// list, so it grows with the canonical instead of blocking it.
//
// ⚠ NARROWED AFTER MEASUREMENT. This used to claim that without it
// `schemaEnumValues` "could return a frozen slice and every derived expectation
// would silently be a literal again", implying this test is what stops that. It is
// not: a `schemaEnumValues` returning a frozen 5-value slice leaves `internal/cmd`
// GREEN while the schema is unchanged, because the frozen values and the live ones
// coincide. What reds is the CANONICAL MOVING — the helper then disagrees with the
// independent re-decode below. So this guards a frozen read that SURVIVES an
// additive canonical, which is the case that matters, not freezing as such.
func TestSchemaEnumValuesReadsTheLiveVendoredRatings(t *testing.T) {
	ratings := schemaEnumValues(t, "properties", "contentRating", "enum")

	// Derived, not typed: re-decode the schema independently of the helper and
	// require agreement. A helper returning anything but the schema's own array
	// fails here.
	var doc struct {
		Properties struct {
			ContentRating struct {
				Enum []string `json:"enum"`
			} `json:"contentRating"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		t.Fatalf("vendored schema does not decode: %v", err)
	}
	if len(doc.Properties.ContentRating.Enum) == 0 {
		t.Fatal("PREMISE BROKEN: the vendored schema declares no contentRating enum")
	}
	if strings.Join(ratings, ",") != strings.Join(doc.Properties.ContentRating.Enum, ",") {
		t.Errorf("schemaEnumValues does not return the schema's own contentRating enum\n"+
			"  schema: %v\n  helper: %v", doc.Properties.ContentRating.Enum, ratings)
	}

	// And the message must NAME every value the schema declares — the check that
	// fails if a future formatter truncates or de-duplicates the set.
	msg := schemaEnumFindingMessage(t, "contentRating", "properties", "contentRating", "enum")
	for _, r := range ratings {
		if !strings.Contains(msg, "'"+r+"'") {
			t.Errorf("the derived message omits the schema's rating %q:\n  %s", r, msg)
		}
	}
}
