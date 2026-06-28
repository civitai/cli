package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// TestRenderDeviceInstructionsComplete: when the server supplies
// verification_uri_complete, the printed output uses it as the PRIMARY copyable
// (code pre-filled) link, and still surfaces the bare URL + user_code as a
// manual fallback. The device_code (secret) is never printed.
func TestRenderDeviceInstructionsComplete(t *testing.T) {
	da := &api.DeviceAuth{
		DeviceCode:              "dev-secret-code",
		UserCode:                "ABCD-1234",
		VerificationURI:         "https://civitai.com/login/oauth/device",
		VerificationURIComplete: "https://civitai.com/login/oauth/device?code=ABCD-1234",
	}
	var buf bytes.Buffer
	renderDeviceInstructions(&buf, da, "https://civitai.com")
	out := buf.String()

	if !strings.Contains(out, da.VerificationURIComplete) {
		t.Errorf("expected the code-prefilled complete URL %q in output:\n%s", da.VerificationURIComplete, out)
	}
	if !strings.Contains(out, "?code=") {
		t.Errorf("expected a ?code= prefilled link in output:\n%s", out)
	}
	if !strings.Contains(out, da.VerificationURI) {
		t.Errorf("expected the bare verification_uri %q as manual fallback:\n%s", da.VerificationURI, out)
	}
	if !strings.Contains(out, da.UserCode) {
		t.Errorf("expected the user_code %q for manual entry:\n%s", da.UserCode, out)
	}
	if strings.Contains(out, da.DeviceCode) {
		t.Errorf("device_code (secret) must NEVER be printed:\n%s", out)
	}
}

// TestRenderDeviceInstructionsNoComplete: when verification_uri_complete is
// empty, the output falls back to the bare URL:/Code: form and contains no
// ?code= prefilled link. The device_code is never printed.
func TestRenderDeviceInstructionsNoComplete(t *testing.T) {
	da := &api.DeviceAuth{
		DeviceCode:      "dev-secret-code",
		UserCode:        "ABCD-1234",
		VerificationURI: "https://civitai.com/login/oauth/device",
	}
	var buf bytes.Buffer
	renderDeviceInstructions(&buf, da, "https://civitai.com")
	out := buf.String()

	if strings.Contains(out, "?code=") {
		t.Errorf("no prefilled ?code= link expected when complete URI is empty:\n%s", out)
	}
	if !strings.Contains(out, "URL:  "+da.VerificationURI) {
		t.Errorf("expected bare URL: form:\n%s", out)
	}
	if !strings.Contains(out, "Code: "+da.UserCode) {
		t.Errorf("expected bare Code: form:\n%s", out)
	}
	if strings.Contains(out, da.DeviceCode) {
		t.Errorf("device_code (secret) must NEVER be printed:\n%s", out)
	}
}

// TestRenderDeviceInstructionsEmptyURIFallback: when both verification_uri and
// the complete URI are empty, the bare URL falls back to baseURL + the device
// path (preserving the existing behavior).
func TestRenderDeviceInstructionsEmptyURIFallback(t *testing.T) {
	da := &api.DeviceAuth{UserCode: "ABCD-1234"}
	var buf bytes.Buffer
	renderDeviceInstructions(&buf, da, "https://civitai.com")
	out := buf.String()

	if !strings.Contains(out, "https://civitai.com/login/oauth/device") {
		t.Errorf("expected baseURL-derived device path fallback:\n%s", out)
	}
}
