package api

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
		{"non-json falls back to trimmed body", "  boom  ", "boom"},
		{"json without error field falls back", `{"foo":1}`, `{"foo":1}`},
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
