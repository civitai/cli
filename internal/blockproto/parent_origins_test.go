package blockproto

import "testing"

// TestCivitaiParentOrigins pins the list and each of its spellings literally.
// It is an invariant guard: the scaffold and devtunnel tests are what show the
// list reaches the CSP, the allowlist and the preflight.
func TestCivitaiParentOrigins(t *testing.T) {
	got := CivitaiParentOrigins()
	want := []string{"https://civitai.com", "https://civitai.red", "https://civitai.green"}
	if len(got) != len(want) {
		t.Fatalf("CivitaiParentOrigins() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CivitaiParentOrigins() = %q, want %q", got, want)
		}
	}
	got[0] = "https://evil.example"
	if CivitaiParentOrigins()[0] != "https://civitai.com" {
		t.Fatal("CivitaiParentOrigins() handed out its own backing array")
	}
	if s := CivitaiParentOriginsCSP(); s != "https://civitai.com https://civitai.red https://civitai.green" {
		t.Errorf("CivitaiParentOriginsCSP() = %q", s)
	}
	if s := CivitaiParentOriginsCSV(); s != "https://civitai.com,https://civitai.red,https://civitai.green" {
		t.Errorf("CivitaiParentOriginsCSV() = %q", s)
	}
	if s := CivitaiParentOriginsJS(); s != "  'https://civitai.com',\n  'https://civitai.red',\n  'https://civitai.green',\n" {
		t.Errorf("CivitaiParentOriginsJS() = %q", s)
	}
}
