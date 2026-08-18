package appapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// TESTS FOR THE PROVENANCE STAMP ON THE SUBMIT BODY (issue #411; server side
// civitai/civitai#4061).
//
// 🔴 THE ONE PROPERTY THAT MATTERS MOST IS NOT "THE FIELD ARRIVES". It is that a
// submit which used to work cannot be broken by the stamp. The server validates
// `sourceCommit` against `^[0-9a-f]{40}$` and answers a malformed one with a hard
// 400 that fails the WHOLE upload — deliberately, because silently dropping a bad
// value is the inert-feature shape. So the CLI carries the burden: anything that
// is not that shape must leave as ABSENT, and a diagnostic nicety must never cost
// the user their actual job. TestSubmitVersionCanNeverSendAValueTheServerRejects
// is that guard, driven from hostile inputs rather than from tidy ones.
//
// The second property is the TRI-STATE. `sourceDirty` absent/null means nobody
// said; `false` means a client looked and said clean. Those are different
// answers, and a value bool with omitempty would silently merge them — which is
// why the field is a *bool and why the table below carries both cases with
// DIFFERENT expected wire text.

// bodyOf runs one SubmitVersion against a recording server and returns the
// request body as raw JSON fields, so ABSENCE is observable. Decoding into a
// struct cannot see the difference between "key missing" and "zero value",
// which is the entire question here.
func bodyOf(t *testing.T, prov Provenance) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publishRequestId": "pubreq_1", "slug": "my-block", "version": "0.2.0", "status": "pending",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	if _, err := c.SubmitVersion(context.Background(), []byte("ZIPDATA"), "my-block", "0.2.0", prov); err != nil {
		t.Fatalf("SubmitVersion: %v", err)
	}
	if got == nil {
		t.Fatal("CONTROL failure: the server recorded no request body at all, so every assertion below " +
			"would be about an empty map")
	}
	if _, ok := got["bundleBase64"]; !ok {
		t.Fatal("CONTROL failure: the recorded body carries no bundleBase64, so it is not the submit body")
	}
	return got
}

func boolp(b bool) *bool { return &b }

// The fixture sha is a real-shaped 40-hex value whose characters are NOT all the
// same and which is distinct from every other constant in this file, so a mutant
// that hardcodes or truncates cannot land on it by accident.
const fixtureSHA = "9f2c41ab7de305619c8bd4a0e7f13c25db806e4f"

// TestSubmitVersionSendsTheProvenanceItWasGiven is the wire contract, stated as
// the exact JSON text of each field. Presence, absence and VALUE are asserted
// separately, because a mutant that sends `"sourceDirty":true` for a clean tree
// passes any presence-only check.
func TestSubmitVersionSendsTheProvenanceItWasGiven(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prov       Provenance
		wantCommit string // "" ⇒ the key must be ABSENT
		wantDirty  string // "" ⇒ the key must be ABSENT
	}{
		{
			name: "unknown — a scaffolded app with no repo sends NOTHING",
			prov: Provenance{},
		},
		{
			name:       "clean tree — the commit, and an EXPLICIT false",
			prov:       Provenance{Commit: fixtureSHA, Dirty: boolp(false)},
			wantCommit: `"` + fixtureSHA + `"`,
			wantDirty:  "false",
		},
		{
			name:       "--allow-dirty — the commit, and an explicit true",
			prov:       Provenance{Commit: fixtureSHA, Dirty: boolp(true)},
			wantCommit: `"` + fixtureSHA + `"`,
			wantDirty:  "true",
		},
		{
			name:       "commit known, dirtiness NOT established — the key stays out",
			prov:       Provenance{Commit: fixtureSHA},
			wantCommit: `"` + fixtureSHA + `"`,
		},
		{
			name: "dirtiness without a commit is not a fact — BOTH stay out",
			prov: Provenance{Dirty: boolp(true)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := bodyOf(t, tc.prov)
			assertField(t, body, "sourceCommit", tc.wantCommit)
			assertField(t, body, "sourceDirty", tc.wantDirty)
		})
	}
}

func assertField(t *testing.T, body map[string]json.RawMessage, key, want string) {
	t.Helper()
	raw, present := body[key]
	if want == "" {
		if present {
			t.Errorf("%s must be ABSENT from the body, but the server received %s.\n"+
				"Absent is the documented wire spelling of UNKNOWN; sending a value asserts something nobody established.",
				key, raw)
		}
		return
	}
	if !present {
		t.Fatalf("%s is missing from the body; want %s", key, want)
	}
	if string(raw) != want {
		t.Errorf("%s = %s, want %s", key, raw, want)
	}
}

// serverSourceCommitRe is the SERVER's rule, written out here independently of
// the implementation's own copy. Asserting against `sourceCommitRe` would be a
// restatement of the code under test.
var serverSourceCommitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// TestSubmitVersionCanNeverSendAValueTheServerRejects is the highest-value test
// in this file: it feeds the seam the shapes a real `git rev-parse` can produce
// when something has gone wrong, and asserts the request body either omits
// `sourceCommit` or carries a value the server's own regex accepts. Never a
// third thing.
//
// 🔴 THE FAILURE IT PREVENTS IS NOT A MISSING STAMP, IT IS A FAILED SUBMIT. A
// value outside `^[0-9a-f]{40}$` is a 400 on the whole request, so every row here
// is a way the provenance feature could take away an upload that worked before
// it existed.
func TestSubmitVersionCanNeverSendAValueTheServerRejects(t *testing.T) {
	hostile := []struct{ name, commit string }{
		{"empty", ""},
		{"whitespace only", "   "},
		{"a tab", "\t"},
		{"uppercase — git never emits it, and the server rejects it", strings.ToUpper(fixtureSHA)},
		{"mixed case", "9F2c41ab7de305619c8bd4a0e7f13c25db806e4F"},
		{"an abbreviated sha", fixtureSHA[:7]},
		{"39 characters — one short", fixtureSHA[:39]},
		{"41 characters — one long", fixtureSHA + "0"},
		{"a trailing newline", fixtureSHA + "\n"},
		{"a trailing CR", fixtureSHA + "\r"},
		{"a leading space", " " + fixtureSHA},
		{"the literal ref name", "HEAD"},
		{"a detached-HEAD message rather than a sha", "HEAD detached at 9f2c41a"},
		{"a git error on stdout", "fatal: ambiguous argument 'HEAD': unknown revision"},
		{"non-hex characters of the right length", strings.Repeat("z", 40)},
		{"a sha with an embedded NUL", fixtureSHA[:20] + "\x00" + fixtureSHA[21:]},
		{"two shas on one line", fixtureSHA + " " + fixtureSHA},
		{"a JSON-breaking value", `","sourceDirty":true,"x":"`},
	}
	for _, h := range hostile {
		for _, dirty := range []*bool{nil, boolp(false), boolp(true)} {
			t.Run(h.name, func(t *testing.T) {
				body := bodyOf(t, Provenance{Commit: h.commit, Dirty: dirty})
				raw, present := body["sourceCommit"]
				if !present {
					// The required outcome for every row here — and the dirty
					// flag must go with it, or the row claims a dirtiness for a
					// commit it could not name.
					if _, d := body["sourceDirty"]; d {
						t.Errorf("sourceCommit was dropped but sourceDirty was still sent — "+
							"that asserts a work-tree state for a commit the CLI could not name (input %q)", h.commit)
					}
					return
				}
				var s string
				if err := json.Unmarshal(raw, &s); err != nil {
					t.Fatalf("sourceCommit is not even a JSON string: %s", raw)
				}
				if !serverSourceCommitRe.MatchString(s) {
					t.Errorf("the CLI put %q on the wire for input %q.\n"+
						"The server validates ^[0-9a-f]{40}$ and answers a malformed value with a 400 that FAILS THE SUBMIT. "+
						"A provenance stamp must be dropped, never guessed at.", s, h.commit)
				}
			})
		}
	}

	// POSITIVE CONTROL: the same harness must be able to see a GOOD value
	// arrive, or "nothing malformed was sent" is indistinguishable from a
	// recorder wired to nothing.
	body := bodyOf(t, Provenance{Commit: fixtureSHA, Dirty: boolp(true)})
	if _, ok := body["sourceCommit"]; !ok {
		t.Fatal("CONTROL failure: a well-formed sha did NOT reach the body, so the loop above proves nothing — " +
			"it would report the same result if the field had been deleted from submitBody entirely")
	}
}

// TestSubmitBodyEnvelopeIsTheEmptyMarshal pins the constant the size arithmetic
// starts from against the marshaller itself. Adding a field to submitBody
// WITHOUT omitempty would change this document, and the number printed to users
// would silently stop being exact.
func TestSubmitBodyEnvelopeIsTheEmptyMarshal(t *testing.T) {
	got, err := json.Marshal(submitBody{})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != submitBodyEnvelope {
		t.Errorf("json.Marshal(submitBody{}) = %s, but submitBodyEnvelope is %s.\n"+
			"The two must move together: SubmitBodySize adds len(submitBodyEnvelope) to the base64 length and "+
			"prints the result to users as the EXACT number a request-body limit applies to.", got, submitBodyEnvelope)
	}
}

// TestSubmitBodySizeCountsTheProvenanceItSends is the #423 contract re-derived
// with #411's fields present.
//
// 🔴 THE EXPECTATION IS AN INDEPENDENT MARSHAL, NOT THE FUNCTION'S OWN
// ARITHMETIC. It builds the request document with a map — the same JSON
// encoder, no shared code with submitEnvelopeLen — so a dropped term, a wrong
// envelope or a field that is sent but not counted all show up here.
func TestSubmitBodySizeCountsTheProvenanceItSends(t *testing.T) {
	for _, tc := range []struct {
		name string
		prov Provenance
	}{
		{"no provenance", Provenance{}},
		{"commit only", Provenance{Commit: fixtureSHA}},
		{"commit + clean", Provenance{Commit: fixtureSHA, Dirty: boolp(false)}},
		{"commit + dirty", Provenance{Commit: fixtureSHA, Dirty: boolp(true)}},
		{"a rejected commit is not on the wire and must not be counted",
			Provenance{Commit: strings.ToUpper(fixtureSHA), Dirty: boolp(true)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Lengths cover every base64 padding residue.
			for _, zipLen := range []int{0, 1, 2, 3, 97, 811, 65_537} {
				payload := make([]byte, zipLen)
				for i := range payload {
					payload[i] = byte(i*17 + 3)
				}
				doc := map[string]any{"bundleBase64": base64.StdEncoding.EncodeToString(payload)}
				commit, dirty := tc.prov.sanitised()
				if commit != "" {
					doc["sourceCommit"] = commit
				}
				if dirty != nil {
					doc["sourceDirty"] = *dirty
				}
				want, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				if got := SubmitBodySize(zipLen, tc.prov); got != len(want) {
					t.Errorf("SubmitBodySize(%d, %+v) = %d, but the real marshalled body is %d bytes",
						zipLen, tc.prov, got, len(want))
				}
			}
		})
	}
}

// TestSubmitBodySizeGrowsWithProvenance is the reason the signature changed,
// stated as a measurement: a submit that carries the stamp really is a bigger
// request than one that does not, so a size function that could not see the
// provenance would print a number that is no longer the one on the wire.
func TestSubmitBodySizeGrowsWithProvenance(t *testing.T) {
	const zipLen = 8_201_270 // #423's real bundle
	bare := SubmitBodySize(zipLen, Provenance{})
	stamped := SubmitBodySize(zipLen, Provenance{Commit: fixtureSHA, Dirty: boolp(true)})
	if stamped <= bare {
		t.Fatalf("a stamped body (%d) is not larger than a bare one (%d) — either the fields are not being "+
			"marshalled or the size function is ignoring them", stamped, bare)
	}
	// And the no-provenance answer is unchanged from the pre-#411 arithmetic, so
	// every path that sends nothing prints exactly the number it used to.
	if want := base64.StdEncoding.EncodedLen(zipLen) + len(submitBodyEnvelope); bare != want {
		t.Errorf("SubmitBodySize(%d, none) = %d, want %d — the no-provenance path must be byte-identical "+
			"to the pre-#411 number", zipLen, bare, want)
	}
}

// TestSubmissionDecodesTheSourceTriState is the READ half. Three server answers
// that a `?? false` reader would flatten into one.
func TestSubmissionDecodesTheSourceTriState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		row        string
		wantCommit *string
		wantDirty  *bool
	}{
		{
			name: "a pre-feature row carries neither key",
			row:  `{"id":"pubreq_1","blockId":"a","version":"0.1.0","status":"approved","submittedAt":"x","updatedAt":"x","createdAt":"x"}`,
		},
		{
			name: "explicit nulls are UNKNOWN, exactly like absence",
			row:  `{"id":"pubreq_1","blockId":"a","version":"0.1.0","status":"approved","submittedAt":"x","updatedAt":"x","createdAt":"x","sourceCommit":null,"sourceDirty":null}`,
		},
		{
			name:       "false is an ASSERTION of clean, and is not null",
			row:        `{"id":"pubreq_1","blockId":"a","version":"0.1.0","status":"approved","submittedAt":"x","updatedAt":"x","createdAt":"x","sourceCommit":"` + fixtureSHA + `","sourceDirty":false}`,
			wantCommit: strp(fixtureSHA),
			wantDirty:  boolp(false),
		},
		{
			name:       "true is an assertion of dirty",
			row:        `{"id":"pubreq_1","blockId":"a","version":"0.1.0","status":"approved","submittedAt":"x","updatedAt":"x","createdAt":"x","sourceCommit":"` + fixtureSHA + `","sourceDirty":true}`,
			wantCommit: strp(fixtureSHA),
			wantDirty:  boolp(true),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s Submission
			if err := json.Unmarshal([]byte(tc.row), &s); err != nil {
				t.Fatalf("decode: %v", err)
			}
			switch {
			case tc.wantCommit == nil && s.SourceCommit != nil:
				t.Errorf("sourceCommit decoded as %q, want nil (UNKNOWN)", *s.SourceCommit)
			case tc.wantCommit != nil && s.SourceCommit == nil:
				t.Errorf("sourceCommit decoded as nil, want %q", *tc.wantCommit)
			case tc.wantCommit != nil && *s.SourceCommit != *tc.wantCommit:
				t.Errorf("sourceCommit = %q, want %q", *s.SourceCommit, *tc.wantCommit)
			}
			switch {
			case tc.wantDirty == nil && s.SourceDirty != nil:
				t.Errorf("sourceDirty decoded as %v, want nil.\n"+
					"null and false are DIFFERENT answers: null is 'nobody said', false is 'a client looked and said clean'.",
					*s.SourceDirty)
			case tc.wantDirty != nil && s.SourceDirty == nil:
				t.Errorf("sourceDirty decoded as nil, want %v", *tc.wantDirty)
			case tc.wantDirty != nil && *s.SourceDirty != *tc.wantDirty:
				t.Errorf("sourceDirty = %v, want %v", *s.SourceDirty, *tc.wantDirty)
			}
		})
	}
}

func strp(s string) *string { return &s }
