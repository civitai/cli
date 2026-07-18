package cmd

import (
	"strings"
	"testing"
)

// TestBaseModelFamilyGroups pins the classifier's family table — the groups the
// --for-base mismatch check reasons over.
func TestBaseModelFamilyGroups(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"SD 1.4", "sd1"},
		{"SD 1.5", "sd1"},
		{"SD 1.5 LCM", "sd1"},
		{"SD 2.0", "sd2"},
		{"SD 2.1", "sd2"},
		{"SDXL 1.0", "sdxl"},
		{"SDXL Turbo", "sdxl"},
		{"SDXL Lightning", "sdxl"},
		{"Pony", "sdxl"},
		{"Illustrious", "sdxl"},
		{"NoobAI", "sdxl"},
		{"SD 3.5", "sd3"},
		{"SD 3", "sd3"},
		{"Flux.1 D", "flux"},
		{"Flux.1 S", "flux"},
		{"Flux.1 Kontext", "flux"},
		{"Stable Cascade", "cascade"},
		{"PixArt E", "pixart"},
		{"Kolors", "kolors"},
		{"Hunyuan DiT", "hunyuan"},
		{"Lumina", "lumina"},
		{"AuraFlow", "aura"},
		{"Wan Video 2.2 T2V-A14B", "video"},
		{"Hunyuan Video", "video"},
		{"LTXV", "video"},
		{"Mochi", "video"},
		{"CogVideoX", "video"},
		{"SVD", "video"},
		{"", ""},
		{"Some Future Model", ""},
	}
	for _, tc := range cases {
		if got := baseModelFamily(tc.base); got != tc.want {
			t.Errorf("baseModelFamily(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

// TestBaseModelMismatchConfidentOnly proves the mismatch check warns on confident
// cross-family pairs and stays silent on same-family, near-neighbours, and
// unknowns.
func TestBaseModelMismatchConfidentOnly(t *testing.T) {
	warns := [][2]string{
		{"SD 1.5", "SDXL 1.0"}, // the canonical EasyNegative footgun
		{"Flux.1 D", "SD 1.5"},
		{"SD 1.5", "SD 3.5"},
		{"Wan Video 2.2 T2V-A14B", "SDXL 1.0"},
	}
	for _, p := range warns {
		if !baseModelMismatch(p[0], p[1]) {
			t.Errorf("expected a confident mismatch for %q vs %q", p[0], p[1])
		}
		if baseModelWarning(p[0], p[1]) == "" {
			t.Errorf("expected a warning string for %q vs %q", p[0], p[1])
		}
	}
	quiet := [][2]string{
		{"SD 1.5", "SD 1.5"},           // identical
		{"Pony", "Illustrious"},        // near-neighbours, same SDXL arch → no false alarm
		{"SDXL 1.0", "Pony"},           // same family
		{"SD 1.5", "Some Future Base"}, // one side unclassifiable
		{"Mystery A", "Mystery B"},     // both unclassifiable
	}
	for _, p := range quiet {
		if baseModelMismatch(p[0], p[1]) {
			t.Errorf("did NOT expect a mismatch for %q vs %q", p[0], p[1])
		}
		if baseModelWarning(p[0], p[1]) != "" {
			t.Errorf("did NOT expect a warning for %q vs %q", p[0], p[1])
		}
	}
}

// TestBaseModelWarningMentionsBothModels proves the warning text names both the
// version's base and the target base.
func TestBaseModelWarningMentionsBothModels(t *testing.T) {
	w := baseModelWarning("SD 1.5", "SDXL 1.0")
	if !strings.Contains(w, "SD 1.5") || !strings.Contains(w, "SDXL 1.0") {
		t.Errorf("warning should name both bases: %q", w)
	}
}
