package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestRenderDeviceInstructionsComplete: renderDeviceInstructions is the MANUAL
// fallback (browser not opened). When the server supplies
// verification_uri_complete it prints a SINGLE actionable form — the
// code-prefilled complete URL — and does NOT also print the bare URL:/Code:
// form. The device_code (secret) is never printed.
func TestRenderDeviceInstructionsComplete(t *testing.T) {
	da := &civitai.DeviceAuth{
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
	// The old "both URLs" form is gone: no separate bare URL:/Code: manual block.
	if strings.Contains(out, "URL:  ") || strings.Contains(out, "Code: ") {
		t.Errorf("did not expect the bare URL:/Code: block when a complete URL is present:\n%s", out)
	}
	if strings.Contains(out, da.DeviceCode) {
		t.Errorf("device_code (secret) must NEVER be printed:\n%s", out)
	}
}

// TestRenderDeviceInstructionsNoComplete: when verification_uri_complete is
// empty, the output falls back to the bare URL:/Code: form and contains no
// ?code= prefilled link. The device_code is never printed.
func TestRenderDeviceInstructionsNoComplete(t *testing.T) {
	da := &civitai.DeviceAuth{
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
	da := &civitai.DeviceAuth{UserCode: "ABCD-1234"}
	var buf bytes.Buffer
	renderDeviceInstructions(&buf, da, "https://civitai.com")
	out := buf.String()

	if !strings.Contains(out, "https://civitai.com/login/oauth/device") {
		t.Errorf("expected baseURL-derived device path fallback:\n%s", out)
	}
}

// TestDeviceVerificationURIFallback covers the bare-URI helper's baseURL fallback.
func TestDeviceVerificationURIFallback(t *testing.T) {
	if got := deviceVerificationURI(&civitai.DeviceAuth{VerificationURI: "https://x/y"}, "https://b"); got != "https://x/y" {
		t.Errorf("expected the server-supplied URI, got %q", got)
	}
	if got := deviceVerificationURI(&civitai.DeviceAuth{}, "https://b"); got != "https://b/login/oauth/device" {
		t.Errorf("expected baseURL fallback, got %q", got)
	}
}
