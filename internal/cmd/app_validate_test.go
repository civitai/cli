package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// validateJSON is the parsed shape of `app validate --json`.
type validateJSON struct {
	OK       bool        `json:"ok"`
	Dir      string      `json:"dir"`
	Errors   []issueJSON `json:"errors"`
	Warnings []issueJSON `json:"warnings"`
}

type issueJSON struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// TestAppValidateJSONOK asserts a clean manifest yields ok:true, empty
// errors/warnings, exit 0, and parseable JSON on stdout.
func TestAppValidateJSONOK(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "clean-block"); err != nil {
		t.Fatalf("app init: %v", err)
	}
	dir := filepath.Join(tmp, "clean-block")

	out, _, err := run(t, "app", "validate", dir, "--json")
	if err != nil {
		t.Fatalf("clean validate --json should exit 0: %v\n%s", err, out)
	}
	var got validateJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if !got.OK {
		t.Errorf("clean manifest should report ok:true: %+v", got)
	}
	if len(got.Errors) != 0 {
		t.Errorf("clean manifest should have no errors: %+v", got.Errors)
	}
}

// TestAppValidateJSONErrors asserts a bad manifest yields ok:false, a non-empty
// errors list (each carrying a message, with a field on schema-path errors), AND
// a non-zero exit so scripts still detect failure.
func TestAppValidateJSONErrors(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "block.manifest.json"), []byte(`{"blockId":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	out, _, err := run(t, "app", "validate", tmp, "--json")
	if err == nil {
		t.Fatal("a bad manifest must still exit non-zero in --json mode")
	}
	var got validateJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if got.OK {
		t.Errorf("bad manifest should report ok:false: %+v", got)
	}
	if len(got.Errors) == 0 {
		t.Fatal("bad manifest should list at least one error")
	}
	for _, e := range got.Errors {
		if e.Message == "" {
			t.Errorf("every error must carry a message: %+v", e)
		}
	}
	// Schema-path errors should surface a field (e.g. a missing required prop at
	// the root, or a bad /version). At least one error should be field-tagged.
	var anyField bool
	for _, e := range got.Errors {
		if e.Field != "" {
			anyField = true
			break
		}
	}
	if !anyField {
		t.Errorf("expected at least one schema-path error with a field: %+v", got.Errors)
	}
}

// TestAppValidateJSONStrictWarnings asserts --json --strict reports ok:false and
// exits non-zero when only warnings are present, while still listing the
// warnings as structured items.
func TestAppValidateJSONStrictWarnings(t *testing.T) {
	tmp := t.TempDir()
	writeBudgetedNoBudgetManifest(t, tmp)

	out, _, err := run(t, "app", "validate", tmp, "--json", "--strict")
	if err == nil {
		t.Fatal("--strict with warnings should exit non-zero")
	}
	var got validateJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	if got.OK {
		t.Errorf("--strict with warnings should report ok:false: %+v", got)
	}
	if len(got.Warnings) == 0 {
		t.Errorf("expected the warnings to be listed: %+v", got)
	}

	// Without --strict the same manifest should be ok:true (warnings don't fail).
	out2, _, err2 := run(t, "app", "validate", tmp, "--json")
	if err2 != nil {
		t.Fatalf("warning-only validate --json should exit 0: %v", err2)
	}
	var got2 validateJSON
	if err := json.Unmarshal([]byte(out2), &got2); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out2)
	}
	if !got2.OK {
		t.Errorf("warning-only (no --strict) should report ok:true: %+v", got2)
	}
	if len(got2.Warnings) == 0 {
		t.Errorf("warnings should still be listed even when ok:true: %+v", got2)
	}
}
