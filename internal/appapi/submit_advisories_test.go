package appapi

import "testing"

// The submit-version response may carry an OPTIONAL `advisories` array of
// non-blocking guidance ({code, message, docsUrl?}). A server that predates it
// sends no such key; a shape this client does not understand must still leave
// the ACCEPTED submit decoding successfully, because a decode error here is
// reported as a failed submit whose retry then collides with the pending one.

func TestSubmitVersionDecodesAdvisories(t *testing.T) {
	// Field values are pairwise distinct so a decode that crossed two fields
	// shows up.
	res := submitWithBody(t, `{
		"publishRequestId": "pr_adv", "slug": "demo", "version": "0.2.0", "status": "pending",
		"advisories": [
			{"code": "stock-theme", "message": "This app uses the stock look.", "docsUrl": "https://example.test/theming"},
			{"code": "no-docs", "message": "A note with no docs link."}
		]
	}`)
	if len(res.Advisories) != 2 {
		t.Fatalf("Advisories = %+v, want 2", res.Advisories)
	}
	a0, a1 := res.Advisories[0], res.Advisories[1]
	if a0.Code != "stock-theme" || a0.Message != "This app uses the stock look." || a0.DocsURL != "https://example.test/theming" {
		t.Errorf("Advisories[0] = %+v", a0)
	}
	if a1.Code != "no-docs" || a1.Message != "A note with no docs link." || a1.DocsURL != "" {
		t.Errorf("Advisories[1] = %+v", a1)
	}
	if res.PublishRequestID != "pr_adv" || res.Status != "pending" {
		t.Errorf("the rest of the result did not decode beside advisories: %+v", res)
	}
}

func TestSubmitVersionToleratesAbsentNullAndMalformedAdvisories(t *testing.T) {
	const head = `{"publishRequestId":"pr_m","slug":"demo","version":"0.2.0","status":"pending"`
	for _, tc := range []struct {
		name, body string
		wantCodes  []string
	}{
		{"absent (older server)", head + `}`, nil},
		{"null", head + `,"advisories":null}`, nil},
		{"a string, not an array", head + `,"advisories":"use your own colours"}`, nil},
		{"an object, not an array", head + `,"advisories":{"code":"x","message":"y"}}`, nil},
		{"a number", head + `,"advisories":7}`, nil},
		{"bad entries dropped, good one kept", head + `,"advisories":[` +
			`42,"str",null,{"code":5,"message":"typed wrong"},{"code":"blank","message":"  "},` +
			`{"code":"kept","message":"the one valid entry"}]}`, []string{"kept"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := submitWithBody(t, tc.body)
			if res.PublishRequestID != "pr_m" {
				t.Errorf("PublishRequestID = %q, want pr_m — the submit result must still decode", res.PublishRequestID)
			}
			var got []string
			for _, a := range res.Advisories {
				got = append(got, a.Code)
			}
			if len(got) != len(tc.wantCodes) {
				t.Fatalf("advisory codes = %q, want %q", got, tc.wantCodes)
			}
			for i := range got {
				if got[i] != tc.wantCodes[i] {
					t.Errorf("advisory codes = %q, want %q", got, tc.wantCodes)
				}
			}
		})
	}
}
