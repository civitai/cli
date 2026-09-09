package cmd

import (
	"net/http"
	"strings"
	"testing"
)

// civitai/cli#513, reported by Rochet2: a Civitai username can be entirely
// digits (https://civitai.com/user/2802169344506), and the API serialises such a
// value as a BARE JSON NUMBER — `"username": 2802169344506`, unquoted. The SDK
// typed that field as `string`, so json.Unmarshal rejected the whole page and
// `civitai images search` exited 1 with:
//
//	unexpected response from /api/v1/images (status 200): {"items":[…
//
// pkg/civitai's FlexString is the fix. These are the COMMAND-level regression
// tests — the level the bug was actually reported at — driving the real Cobra
// tree against an httptest server. Every test in this file is RED at
// origin/main (46c928a) and green at HEAD.
//
// 🔴 The digit fixtures are pairwise distinct and distinct from every constant
// asserted on, so a mutant that hardcodes one literal cannot satisfy another.

// numericUploader is the username from the issue.
const numericUploader = "2802169344506"

// numericCreator and numericCandidate are DIFFERENT all-digit usernames, used on
// the model and user paths so no assertion here can be satisfied by a value
// another test's fixture supplied.
const (
	numericCreator   = "9007199254740993" // also past float64's mantissa
	numericCandidate = "48815072290"
)

// TestImagesSearchDecodesANumericUsername is the reported repro: `civitai images
// search` over a page whose uploader has an all-digit username must exit 0 and
// print the digits.
func TestImagesSearchDecodesANumericUsername(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
		  {"id":136456589,"url":"https://img/1","width":832,"height":1216,"nsfwLevel":"None",
		   "username":` + numericUploader + `,"baseModel":"Illustrious","stats":{"heartCount":9}},
		  {"id":136456590,"url":"https://img/2","width":512,"height":512,"nsfwLevel":"None",
		   "username":"quilted-heron","baseModel":"Pony","stats":{"heartCount":3}}
		],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "search")
	if err != nil {
		t.Fatalf("images search over a numeric username failed: %v\n"+
			"This is #513: the page decode used to fail with \"unexpected response from "+
			"/api/v1/images (status 200)\" and exit 1.", err)
	}
	if !strings.Contains(out, numericUploader) {
		t.Errorf("the UPLOADER column should carry the literal digits %q:\n%s", numericUploader, out)
	}
	// A float64 round trip renders as an exponent; assert the corrupted form is
	// absent as well, so a fix that decodes-but-mangles cannot pass on the
	// substring check alone.
	if strings.Contains(out, "e+12") {
		t.Errorf("the username was routed through float64 and rendered as an exponent:\n%s", out)
	}
	// The ordinary quoted username on the same page must still render.
	if !strings.Contains(out, "quilted-heron") {
		t.Errorf("the string username on the same page should still render:\n%s", out)
	}
}

// TestImagesSearchJSONKeepsTheUnquotedUsername pins the --json contract across
// the fix: it is a raw passthrough of the server's own bytes, so the number stays
// UNQUOTED on stdout. FlexString would re-emit it quoted if anything marshalled
// the struct — nothing does, and this is the assertion that says so at the
// published surface rather than in a comment.
func TestImagesSearchJSONKeepsTheUnquotedUsername(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":1,"url":"u","username":` + numericUploader + `}],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "search", "--json")
	if err != nil {
		t.Fatalf("images search --json: %v", err)
	}
	if !strings.Contains(out, `"username": `+numericUploader) {
		t.Errorf("--json must pass the server's UNQUOTED number through untouched:\n%s", out)
	}
	if strings.Contains(out, `"`+numericUploader+`"`) {
		t.Errorf("--json re-emitted the username QUOTED — it is no longer the API's document:\n%s", out)
	}
}

// TestUsersGetDecodesANumericUsername covers `civitai users get`, the command a
// person reaches for when they have such an account id in hand.
func TestUsersGetDecodesANumericUsername(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":4471,"username":` + numericCreator + `,"image":"https://img/u"}]}`))
	})
	out, _, err := run(t, "users", "get", "4471")
	if err != nil {
		t.Fatalf("users get over a numeric username failed: %v", err)
	}
	if !strings.Contains(out, numericCreator) {
		t.Errorf("the username line should carry the literal digits %q:\n%s", numericCreator, out)
	}
	// 9007199254740993 is 2^53+1: any float64 round trip returns …992.
	if strings.Contains(out, "9007199254740992") {
		t.Errorf("the username lost its last digit to a float64 round trip:\n%s", out)
	}
}

// TestUsersGetListsANumericCandidate exercises the OTHER consumer of the field
// on this path — the no-exact-match branch, which builds the "closest matches"
// list and does a case-insensitive comparison against the argument. Both had to
// be converted when the field stopped being a `string`.
func TestUsersGetListsANumericCandidate(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":91,"username":` + numericCandidate + `,"image":""}]}`))
	})
	out, _, err := run(t, "users", "get", "marmalade-otter")
	if err == nil {
		t.Fatalf("a fuzzy-only match must not be printed as the answer; got:\n%s", out)
	}
	if !strings.Contains(err.Error(), "no user found with exact username") {
		t.Fatalf("expected the no-exact-match refusal, got: %v", err)
	}
	if !strings.Contains(err.Error(), numericCandidate) {
		t.Errorf("the candidate list should name the numeric username %q, got: %v", numericCandidate, err)
	}
}

// TestModelsSearchDecodesANumericCreatorUsername covers the NESTED shape — the
// field hangs off an embedded `creator` object rather than sitting at item level.
func TestModelsSearchDecodesANumericCreatorUsername(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":81,"name":"Brass Lamp","type":"LORA","nsfw":false,
		  "creator":{"username":` + numericCandidate + `},
		  "stats":{"downloadCount":11,"thumbsUpCount":2,"commentCount":1}}],"metadata":{}}`))
	})
	out, _, err := run(t, "models", "search")
	if err != nil {
		t.Fatalf("models search over a numeric creator username failed: %v", err)
	}
	if !strings.Contains(out, numericCandidate) {
		t.Errorf("the CREATOR column should carry the literal digits %q:\n%s", numericCandidate, out)
	}
}
