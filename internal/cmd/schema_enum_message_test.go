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
// A helper that ignored its argument and returned the real ratings string would
// satisfy `TestValidateEnumFindingWordingIsUnchanged` perfectly — it would just be
// the hand-typed literal again, one call deep. So the formatter is fed a set the
// `contentRating` enum CANNOT contain, and the whole message is pinned: any
// hardcoding, and any drift in the lead-in or the separator, fails here.
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
}

// TestSchemaEnumValuesReadsTheLiveVendoredRatings proves the READ half reaches the
// schema, which the formatter test above cannot see.
//
// Without it, `schemaEnumValues` could return a frozen slice and every derived
// expectation would silently be a literal again. It asserts against the schema's
// own parsed content rather than against a copied list, so it grows with the
// canonical instead of blocking it.
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
