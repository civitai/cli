package cmd

import (
	"strings"
	"testing"
)

// TestDownloadHelpNoAnonymousClaim proves the download help no longer implies
// public files download anonymously (Lever #2 — the false claim a dogfood run
// hit when a 336 KB public embedding still 401'd). It must instead state that
// downloads require authentication.
func TestDownloadHelpNoAnonymousClaim(t *testing.T) {
	cmd := newDownloadCmd()
	blob := cmd.Short + "\n" + cmd.Long

	// The specific false phrasing ("Public files download anonymously") must be gone.
	if strings.Contains(strings.ToLower(blob), "download anonymously") {
		t.Errorf("download help still claims anonymous download:\n%s", cmd.Long)
	}
	// It must affirmatively state downloads require auth.
	low := strings.ToLower(blob)
	if !strings.Contains(low, "requires authentication") {
		t.Errorf("download help should state downloads require authentication:\n%s", cmd.Long)
	}
	if !strings.Contains(blob, "civitai login") {
		t.Errorf("download help should point at 'civitai login':\n%s", cmd.Long)
	}

	// The --anon flag usage must note downloads still 401 (it's for read commands).
	anon := cmd.Flags().Lookup("anon")
	if anon == nil {
		t.Fatal("--anon flag missing")
	}
	if !strings.Contains(strings.ToLower(anon.Usage), "401") {
		t.Errorf("--anon usage should note downloads 401 without a token: %q", anon.Usage)
	}
}

// TestDownloadBaseModelShownInPlan proves the version's base model is always
// surfaced in the download plan/output.
func TestDownloadBaseModelShownInPlan(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	// newDLServer reports baseModel "SD 1.5".
	out, _, err := run(t, "download", "128713", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(out, "base model: SD 1.5") {
		t.Errorf("plan should show the base model:\n%s", out)
	}
}
