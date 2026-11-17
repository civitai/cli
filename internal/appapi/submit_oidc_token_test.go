package appapi

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// TESTS FOR THE GITHUB ACTIONS OIDC TOKEN ON THE SUBMIT BODY.
//
// The token travels as `githubOidcToken`. It is a separate answer from the
// sourceCommit/sourceDirty CLAIM: GitHub signs it and the server checks it. So
// it must arrive when set, be absent when not (a submit outside Actions is
// byte-identical to before), survive a commit the CLI refuses to send, and be
// counted by SubmitBodySize, whose number is printed as the exact body size.

// fixtureToken is JWT-shaped and distinct from every other constant here.
const fixtureToken = "eyJhbGciOiJSUzI1NiIsImtpZCI6Ik9JREMifQ.eyJyZXBvc2l0b3J5IjoiYWNtZS9hcHAifQ.c2lnLXRlc3QtNzcx"

func TestSubmitVersionSendsTheOIDCTokenOnlyWhenSet(t *testing.T) {
	t.Run("set: sent verbatim as githubOidcToken", func(t *testing.T) {
		body := bodyOf(t, Provenance{Commit: fixtureSHA, Dirty: boolp(false), GitHubOIDCToken: fixtureToken})
		if got := string(body["githubOidcToken"]); got != `"`+fixtureToken+`"` {
			t.Errorf("githubOidcToken = %s, want %q", got, fixtureToken)
		}
		// The claim is still the claim: the token neither replaces nor drops it.
		if got := string(body["sourceCommit"]); got != `"`+fixtureSHA+`"` {
			t.Errorf("sourceCommit = %s, want %q alongside the token", got, fixtureSHA)
		}
	})

	t.Run("empty: the key is absent", func(t *testing.T) {
		body := bodyOf(t, Provenance{Commit: fixtureSHA})
		if raw, ok := body["githubOidcToken"]; ok {
			t.Errorf("a submit with no token sent githubOidcToken=%s; it must send no key", raw)
		}
	})

	t.Run("a commit the CLI refuses to send does not take the token with it", func(t *testing.T) {
		body := bodyOf(t, Provenance{Commit: strings.ToUpper(fixtureSHA), GitHubOIDCToken: fixtureToken})
		if _, ok := body["sourceCommit"]; ok {
			t.Fatal("CONTROL failure: the malformed commit was sent, so this row tests nothing")
		}
		if got := string(body["githubOidcToken"]); got != `"`+fixtureToken+`"` {
			t.Errorf("githubOidcToken = %s, want %q: the token is not part of the claim", got, fixtureToken)
		}
	})
}

// TestSubmitBodySizeCountsTheOIDCToken: the expectation is an independent marshal
// of a map, sharing no code with submitEnvelopeLen.
func TestSubmitBodySizeCountsTheOIDCToken(t *testing.T) {
	for _, prov := range []Provenance{
		{GitHubOIDCToken: fixtureToken},
		{Commit: fixtureSHA, Dirty: boolp(true), GitHubOIDCToken: fixtureToken},
	} {
		for _, zipLen := range []int{0, 1, 2, 811} {
			doc := map[string]any{"bundleBase64": base64.StdEncoding.EncodeToString(make([]byte, zipLen))}
			if commit, dirty := prov.sanitised(); commit != "" {
				doc["sourceCommit"] = commit
				if dirty != nil {
					doc["sourceDirty"] = *dirty
				}
			}
			doc["githubOidcToken"] = prov.GitHubOIDCToken
			want, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if got := SubmitBodySize(zipLen, prov); got != len(want) {
				t.Errorf("SubmitBodySize(%d, token, commit=%q) = %d, but the real body is %d bytes",
					zipLen, prov.Commit, got, len(want))
			}
		}
	}
}

func TestSubmitResultDecodesActionsRunSourceRecorded(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want *bool
	}{
		{"absent (no token, flag off, older server)", `{"publishRequestId":"pubreq_1"}`, nil},
		{"recorded", `{"publishRequestId":"pubreq_1","actionsRunSourceRecorded":true}`, boolp(true)},
		{"verified but not recorded", `{"publishRequestId":"pubreq_1","actionsRunSourceRecorded":false}`, boolp(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r SubmitResult
			if err := json.Unmarshal([]byte(tc.body), &r); err != nil {
				t.Fatal(err)
			}
			switch {
			case tc.want == nil && r.ActionsRunSourceRecorded != nil:
				t.Errorf("got %v, want nil", *r.ActionsRunSourceRecorded)
			case tc.want != nil && (r.ActionsRunSourceRecorded == nil || *r.ActionsRunSourceRecorded != *tc.want):
				t.Errorf("got %v, want %v", r.ActionsRunSourceRecorded, *tc.want)
			}
		})
	}
}
