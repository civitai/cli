package appapi

import (
	"strings"
	"testing"
)

func TestOAuthMsg(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"error+description", `{"error":"invalid_grant","error_description":"bad code"}`, "invalid_grant: bad code"},
		{"error only", `{"error":"slow_down"}`, "slow_down"},
		// 🔴 WAS the trimmed raw body. Every caller is a non-200 on a route whose
		// success payload is a credential, so an unrecognised body is reported
		// by its size alone — never echoed.
		{"non-json body is withheld", "  boom  ", "8-byte response body not shown because it may carry credentials"},
		{"json without error field is withheld", `{"foo":1}`, "9-byte response body not shown because it may carry credentials"},
		{"empty body", "  ", "empty response body"},
		{"description holding a URL is dropped", `{"error":"server_error","error_description":"see https://x.example/?t=abc"}`, "server_error"},
		{"description with a control byte is dropped", "{\"error\":\"server_error\",\"error_description\":\"a\\u001b[2Jb\"}", "server_error"},
		{"error code that is not plain text is withheld", `{"error":"{\"access_token\":\"x\"}"}`, "36-byte response body not shown because it may carry credentials"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oauthMsg([]byte(tc.raw)); got != tc.want {
				t.Errorf("oauthMsg(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDeviceFlowErrorMessage(t *testing.T) {
	cases := []struct {
		err  *DeviceFlowError
		want string
	}{
		{&DeviceFlowError{Code: "access_denied", Description: "user said no"}, "user said no"},
		{&DeviceFlowError{Code: "expired_token"}, "expired"},
		{&DeviceFlowError{Code: "access_denied"}, "denied"},
		{&DeviceFlowError{Code: "weird_thing"}, "weird_thing"},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); !strings.Contains(got, tc.want) {
			t.Errorf("DeviceFlowError{%q,%q}.Error() = %q, want contains %q",
				tc.err.Code, tc.err.Description, got, tc.want)
		}
	}
}

// TestOAuthErrorFieldsAreShownOnlyWhenPlainText pins that the typed OAuth errors
// print the server's `error` / `error_description` only when they are short
// plain text: both come from a response body on a credential route.
func TestOAuthErrorFieldsAreShownOnlyWhenPlainText(t *testing.T) {
	const marker = "SENTINEL-OAUTH-FIELD-2e7c"
	signed := "https://x.example/cb?code=" + marker
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"device flow: URL in description", &DeviceFlowError{Code: "server_error", Description: signed}, "device login failed: server_error"},
		{"device flow: JSON in code", &DeviceFlowError{Code: `{"access_token":"` + marker + `"}`}, "device login failed: unrecognised error code"},
		{"device flow: plain description kept", &DeviceFlowError{Code: "server_error", Description: "try later"}, "device login failed: try later (server_error)"},
		{"invalid scope: URL in description", &InvalidScopeError{Requested: DeviceScope, Description: signed}, "the Civitai auth server rejected this login's scopes (invalid_scope)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if strings.Contains(got, marker) {
				t.Errorf("the error carries the server field verbatim: %q", got)
			}
			if !strings.HasPrefix(got, tc.want) {
				t.Errorf("got %q, want prefix %q", got, tc.want)
			}
		})
	}
}
