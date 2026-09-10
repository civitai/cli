package civitai

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// FlexString exists because the Civitai API sends an all-digit username as a
// BARE JSON NUMBER (civitai/cli#513, reported by Rochet2). These tests pin both
// halves of that: WHICH shapes are accepted, and — the half that matters — that
// an accepted number keeps its LITERAL digits rather than being laundered
// through float64.
//
// 🔴 RED/GREEN MATRIX, MEASURED — and it splits this file in two, because only
// half of it CAN be run against the pre-change code.
//
// The three SEAM tests at the bottom read the field back through
// `string(item.Username)`, which compiles against a `string` field and against a
// FlexString one, so they were run at origin/main (46c928a) in a detached
// worktree: all three FAIL there, each with the reported symptom verbatim —
// "unexpected response from /api/v1/images (status 200)". They pass at HEAD.
// That is the regression coverage for #513.
//
// The FlexString unit tests above them CANNOT be red at base: the type does not
// exist there, so they are a compile error rather than a failing assertion.
// Their standing is the MUTATION matrix instead — six mutants of
// UnmarshalJSON, each killed by a named assertion; see the PR body.
//
// Three are INVARIANT GUARDS either way, labelled as such at their definitions,
// because a plain `string` field already behaved this way and none of them
// pins new behaviour: TestFlexStringNullLeavesTheZeroValue,
// TestFlexStringRejectsWrongShapes, and TestFlexStringAcceptsAJSONString's
// "plain" and "escaped unicode" rows.
//
// 🔴 FIXTURE RULE. The digit fixtures below are pairwise distinct AND distinct
// from every constant an assertion names, so a mutant that hardcodes a literal
// cannot survive: each row's want is reachable only by actually decoding that
// row's input.

// beyondFloat64 is 2^53 + 1 — the smallest positive integer a float64 CANNOT
// represent. It is here so the fidelity claim does not rest on formatting alone:
// any implementation that routes the number through float64 returns
// 9007199254740992 (…992, not …993) no matter how it is then rendered, which the
// reported username (2802169344506, comfortably under 2^53) would not catch on
// its own — that one only catches the `%v`/'g' exponent rendering.
const beyondFloat64 = "9007199254740993"

// reportedUsername is the value from the issue: https://civitai.com/user/2802169344506.
const reportedUsername = "2802169344506"

// decodeFlex decodes one JSON value into a FlexString through the same
// encoding/json path a struct field takes.
func decodeFlex(t *testing.T, body string) (FlexString, error) {
	t.Helper()
	var v struct {
		U FlexString `json:"u"`
	}
	err := json.Unmarshal([]byte(`{"u":`+body+`}`), &v)
	return v.U, err
}

// TestFlexStringPreservesBigIntegerDigits is the regression test for #513's
// CORRECTNESS half: the digits must survive byte-for-byte.
func TestFlexStringPreservesBigIntegerDigits(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"the reported username", reportedUsername, reportedUsername},
		{"beyond float64's mantissa", beyondFloat64, beyondFloat64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFlex(t, tc.body)
			if err != nil {
				t.Fatalf("decoding the bare number %s failed: %v — this is #513 itself", tc.body, err)
			}
			if got.String() != tc.want {
				t.Errorf("a numeric username decoded to %q, want the literal digits %q.\n"+
					"A value routed through float64 comes back as an exponent (2.802169344506e+12) or, "+
					"past 2^53, with the last digit changed — either way the username matches no account "+
					"and cannot be pasted back into --username.", got.String(), tc.want)
			}
		})
	}
}

// TestFlexStringKeepsANumberLiteralVerbatim pins the decided behaviour for the
// number shapes that are NOT plain integers. Each stays exactly as the server
// wrote it: FlexString carries a literal, it does not normalise one.
func TestFlexStringKeepsANumberLiteralVerbatim(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"a float that looks like an int", "1.0", "1.0"},
		{"a fractional float", "76.25", "76.25"},
		{"a negative number", "-31", "-31"},
		{"exponent notation", "4e6", "4e6"},
		{"zero", "0", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFlex(t, tc.body)
			if err != nil {
				t.Fatalf("decode %s: %v", tc.body, err)
			}
			if got.String() != tc.want {
				t.Errorf("decoded %s to %q, want the literal %q", tc.body, got.String(), tc.want)
			}
		})
	}
}

// TestFlexStringAcceptsAJSONString covers the ordinary path. The two rows named
// in the matrix at the top of this file are INVARIANT GUARDS (a plain `string`
// field already did this); "digits inside quotes" is not — it is the
// discriminator that a quoted value is NOT re-parsed as a number, which would
// eat the leading zero.
func TestFlexStringAcceptsAJSONString(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"plain", `"pineapple-lamp"`, "pineapple-lamp"},
		// The BODY carries the six-character \u escape sequences literally (note
		// the doubled backslashes); only a real json.Unmarshal of the quoted
		// value turns them into runes. A branch that handed the raw bytes back
		// without unquoting would fail this row and no other.
		{"escaped unicode", "\"\\u00e9t\\u00e9-2019\"", "été-2019"},
		{"digits inside quotes keep their leading zero", `"007734"`, "007734"},
		{"empty string", `""`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFlex(t, tc.body)
			if err != nil {
				t.Fatalf("decode %s: %v", tc.body, err)
			}
			if got.String() != tc.want {
				t.Errorf("decoded %s to %q, want %q", tc.body, got.String(), tc.want)
			}
		})
	}
}

// TestFlexStringNullLeavesTheZeroValue is an INVARIANT GUARD, not regression
// coverage: `"username": null` already left "" when the field was a plain
// string, and CollectionUser's doc comment documents that nullability. It is
// here so a future tightening of UnmarshalJSON cannot turn a nullable field into
// a hard decode failure for a whole page.
func TestFlexStringNullLeavesTheZeroValue(t *testing.T) {
	got, err := decodeFlex(t, "null")
	if err != nil {
		t.Fatalf("null must not be an error — CollectionUser.Username is documented nullable: %v", err)
	}
	if got.String() != "" {
		t.Errorf("null decoded to %q, want the zero value", got.String())
	}
}

// TestFlexStringRejectsWrongShapes is the error-path half, and the reason this
// type is NOT a lenient catch-all: a boolean, an array or an object in this
// field is real schema drift and must still fail loudly rather than be coerced
// into some plausible string. These rows are INVARIANT GUARDS — a plain `string`
// field rejected all three too — kept so a later "make it accept anything"
// simplification cannot pass unnoticed.
func TestFlexStringRejectsWrongShapes(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"object", `{"name":"kiwi"}`},
		{"array", `["kiwi"]`},
		{"boolean true", `true`},
		{"boolean false", `false`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFlex(t, tc.body)
			if err == nil {
				t.Fatalf("decoding %s succeeded with %q — a wrong shape here is schema drift and "+
					"must not be silently coerced", tc.body, got.String())
			}
		})
	}
}

// TestFlexStringRejectsMalformedInputDirectly covers the branches encoding/json
// can never reach, because its own scanner rejects the document first: an EMPTY
// slice, a bare word that only LOOKS like null, and a number literal JSON does
// not allow. They are reachable by any hand-rolled caller of the method, and
// leaving them untested is how a `case 'n':` that matched a prefix rather than
// the whole word would ship.
func TestFlexStringRejectsMalformedInputDirectly(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", nil, "empty JSON value"},
		{"a bare word that is not null", []byte("nul"), "want a JSON string or number"},
		{"null with a tail", []byte("nullish"), "want a JSON string or number"},
		{"a number literal JSON forbids", []byte("01"), "invalid character"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var f FlexString
			err := f.UnmarshalJSON(tc.in)
			if err == nil {
				t.Fatalf("UnmarshalJSON(%q) must return an error, got %q", tc.in, f.String())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("UnmarshalJSON(%q) said %q, which does not contain %q", tc.in, err, tc.want)
			}
			if f != "" {
				t.Errorf("a rejected value must leave the field untouched, got %q", f.String())
			}
		})
	}
}

// TestFlexStringErrorNamesTheOffendingKind asserts the message is diagnosable —
// the whole point of NOT being lenient is that the reader learns what arrived.
func TestFlexStringErrorNamesTheOffendingKind(t *testing.T) {
	for _, tc := range []struct{ body, wantKind string }{
		{`{"a":1}`, "object"},
		{`[1]`, "array"},
		{`true`, "boolean"},
	} {
		_, err := decodeFlex(t, tc.body)
		if err == nil {
			t.Fatalf("decoding %s must fail", tc.body)
		}
		if !strings.Contains(err.Error(), tc.wantKind) {
			t.Errorf("decoding %s produced %q, which does not name the kind %q", tc.body, err, tc.wantKind)
		}
	}
}

// TestFlexStringMarshalsAsAJSONString records the round-trip shape rather than
// leaving it to be discovered: a numeric username re-encodes QUOTED. Nothing in
// this repo marshals these structs (--json is served from the RAW server bytes
// by internal/cmd/read.go's emitJSON), so this is a claim about the type, not a
// live output change — and it is pinned so that adding a marshal path later is a
// decision somebody makes on purpose.
func TestFlexStringMarshalsAsAJSONString(t *testing.T) {
	b, err := json.Marshal(FlexString(reportedUsername))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `"`+reportedUsername+`"` {
		t.Errorf("FlexString marshalled to %s, want the quoted form", b)
	}
}

// ----------------------------------------------------------------------------
// The seam: a real /api/v1/images body through the real client path
// ----------------------------------------------------------------------------

// TestSearchImagesDecodesANumericUsername is the #513 REGRESSION TEST at the
// level the bug was reported at. It drives Client.SearchImages against an
// httptest server returning the exact shape the API sends for an all-digit
// account — `"username": 2802169344506`, unquoted — and asserts the page decodes
// and the digits survive.
//
// Before this change getInto's json.Unmarshal rejected that body and the command
// exited 1 with "unexpected response from /api/v1/images (status 200)".
func TestSearchImagesDecodesANumericUsername(t *testing.T) {
	body := `{"items":[
	  {"id":136456589,"url":"https://img/1","width":832,"height":1216,"nsfwLevel":"None",
	   "username":` + reportedUsername + `,"baseModel":"Illustrious","stats":{"heartCount":9}},
	  {"id":136456590,"url":"https://img/2","width":512,"height":512,"nsfwLevel":"None",
	   "username":"quilted-heron","baseModel":"Pony","stats":{"heartCount":3}}
	],"metadata":{}}`
	srv, gotPath, _, _ := newTestServer(t, body)

	res, err := New(srv.URL, "").SearchImages(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchImages on a body with a numeric username failed: %v\n"+
			"This is civitai/cli#513: the API sends an all-digit username unquoted and the "+
			"whole page decode used to fail with \"unexpected response … (status 200)\".", err)
	}
	if *gotPath != "/api/v1/images" {
		t.Fatalf("path = %q, want /api/v1/images", *gotPath)
	}
	if len(res.Items) != 2 {
		t.Fatalf("decoded %d items, want 2: %+v", len(res.Items), res.Items)
	}
	if got := string(res.Items[0].Username); got != reportedUsername {
		t.Errorf("numeric username decoded to %q, want %q", got, reportedUsername)
	}
	// The ordinary quoted username on the same page must be untouched — a fix
	// that only ever produced digits would pass the assertion above alone.
	if got := string(res.Items[1].Username); got != "quilted-heron" {
		t.Errorf("string username decoded to %q, want %q", got, "quilted-heron")
	}
	// Raw is what --json emits, and it must still carry the number UNQUOTED.
	//
	// This is a claim about the DOCUMENT, not about the bytes: an earlier
	// revision said "it must still be the server's own bytes" and "--json is a
	// raw passthrough", which is the same false byte-identity claim this branch
	// corrected in internal/cmd/numeric_username_test.go and on every Raw field's
	// doc comment. Raw is the body that DECODED — a body the API sent with a raw
	// control byte inside a string is repaired first (EscapeJSONStringControlChars),
	// and emitJSON re-indents on the way to stdout. Neither touches the digits
	// below; both make byte-identity false.
	if !strings.Contains(string(res.Raw), `"username":`+reportedUsername) {
		t.Errorf("Raw no longer carries the server's unquoted username — FlexString "+
			"re-quoted it, or something marshalled the struct:\n%s", res.Raw)
	}
}

// TestSearchUsersDecodesANumericUsername covers the OTHER endpoint a person hits
// with such an account: `civitai users get <all-digit-name>` resolves through
// GET /api/v1/users, whose items carry the same field.
func TestSearchUsersDecodesANumericUsername(t *testing.T) {
	body := `{"items":[{"id":4471, "username":` + beyondFloat64 + `, "image":"https://img/u"}]}`
	srv, gotPath, _, _ := newTestServer(t, body)

	res, err := New(srv.URL, "").SearchUsers(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchUsers on a body with a numeric username failed: %v", err)
	}
	if *gotPath != "/api/v1/users" {
		t.Fatalf("path = %q, want /api/v1/users", *gotPath)
	}
	if len(res.Items) != 1 {
		t.Fatalf("decoded %d items, want 1", len(res.Items))
	}
	if got := string(res.Items[0].Username); got != beyondFloat64 {
		t.Errorf("numeric username decoded to %q, want the literal digits %q", got, beyondFloat64)
	}
}

// TestSearchModelsDecodesANumericCreatorUsername covers the nested case — the
// field is on an embedded `creator` object rather than at item level, which is a
// different decode path through encoding/json.
func TestSearchModelsDecodesANumericCreatorUsername(t *testing.T) {
	const creator = "5150042331"
	body := `{"items":[{"id":81,"name":"Lamp","type":"LORA","nsfw":false,
	  "creator":{"username":` + creator + `},
	  "stats":{"downloadCount":11,"thumbsUpCount":2,"commentCount":1}}],"metadata":{}}`
	srv, _, _, _ := newTestServer(t, body)

	res, err := New(srv.URL, "").SearchModels(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchModels on a body with a numeric creator username failed: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Creator == nil {
		t.Fatalf("decoded items wrong: %+v", res.Items)
	}
	if got := string(res.Items[0].Creator.Username); got != creator {
		t.Errorf("numeric creator username decoded to %q, want %q", got, creator)
	}
}
