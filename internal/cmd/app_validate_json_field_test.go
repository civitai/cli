package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// app_validate_json_field_test.go pins the `--json` contract README states:
// "`--json` emits the structured result (`ok`, plus `errors`/`warnings` each
// with `field`/`message`)".
//
// 🔴 EVERY ASSERTION HERE IS ON A DECODED map[string]any, NEVER ON THE RAW TEXT.
// A `strings.Contains(out, "field")` would be satisfied by the literal appearing
// anywhere — including inside a MESSAGE, and several of these messages contain
// the word — so a text search cannot tell a present key from a mentioned word,
// and cannot see `"field": null` at all. Same reasoning as
// `TestGraph_UnsetFieldsAreAbsent` (AGENTS.md item 14): `"cfg"` substring-matches
// `"cfgScale"`, so key identity is a question only the decoder can answer.
//
// 🔴 WHAT THIS FILE CAN AND CANNOT SEE, stated so a green here is not over-read.
// `issues()` maps an empty field to `(root)` as a last line of defence for the
// README's "never null" promise, so a check that computes its field to "" at
// RUNTIME reaches the wire looking well-formed. Measured against exactly that
// mutant (the sandbox-escape rule rewired to an empty field):
// TestValidateJSONEveryFindingCarriesAField PASSED — it only asks that the value
// is non-null, and the fallback supplied one — while
// TestValidateJSONSemanticChecksAreFieldTagged FAILED, because it asks for the
// SPECIFIC field and got `(root)`. That is the whole argument for asserting
// per-check rather than per-shape. The general claim over ALL checks still
// belongs to `TestEveryCheckEmitsAField` in internal/validate, which sees the
// finding before the fallback.
//
// RED/GREEN at base (`origin/main` @ e9a44c4, this file copied in verbatim):
//   - TestValidateJSONEveryFindingCarriesAField    RED  (7 of 11 findings had no field key)
//   - TestValidateJSONSemanticChecksAreFieldTagged RED  (all 6 semantic fields "", both schema fields JSON Pointer)
//   - TestValidateJSONFieldsUseDottedNotation      RED  (/blockId, /scopes/2)
//   - TestValidateJSONStreamSeparation             GREEN — an INVARIANT GUARD, not
//     regression coverage: stream separation was already correct and the issue
//     asked that it stay that way. Labelled rather than counted.
//   - TestValidateJSONCleanManifestHasEmptyBuckets GREEN — the NEGATIVE CONTROL,
//     green on purpose in both directions.

// brokenSixWays is the manifest from issue #225 — the one whose 7 findings came
// back with 4 `field: null`. The four nulls were the SEMANTIC checks:
// sandbox escape, disallowed sandbox token, unjustified sensitive scope, and the
// budgeted-page warning.
const brokenSixWays = `{
  "blockId": "Bad_Id",
  "name": "Six Ways Broken",
  "version": "1.0.0",
  "scopes": ["user:read:self", "ai:write:budgeted", "not-a-real-scope"],
  "page": {"path": "/", "title": "Broken"},
  "iframe": {"sandbox": "allow-same-origin allow-scripts allow-popups"},
  "contentRating": "g"
}`

// decodeValidateJSON runs `app validate --json` and decodes stdout GENERICALLY,
// so key presence and JSON null are both observable. It returns the decoded
// object plus stderr and the command error.
func decodeValidateJSON(t *testing.T, dir string, extraArgs ...string) (map[string]any, string, error) {
	t.Helper()
	args := append([]string{"app", "validate", dir, "--json"}, extraArgs...)
	out, errOut, err := run(t, args...)
	var got map[string]any
	if uerr := json.Unmarshal([]byte(out), &got); uerr != nil {
		t.Fatalf("--json stdout is not valid JSON: %v\nstdout:\n%s", uerr, out)
	}
	return got, errOut, err
}

// findingObjects pulls the errors+warnings arrays out of the decoded payload
// WITHOUT typing them, so a missing key and a null value stay distinguishable.
func findingObjects(t *testing.T, payload map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := payload[key]
	if !ok {
		t.Fatalf("payload has no %q key: %v", key, payload)
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("%q is not an array: %T", key, raw)
	}
	out := make([]map[string]any, 0, len(arr))
	for i, e := range arr {
		obj, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("%s[%d] is not an object: %T", key, i, e)
		}
		out = append(out, obj)
	}
	return out
}

// TestValidateJSONEveryFindingCarriesAField is the direct regression test for
// issue #225.
//
// It asserts, on the DECODED object:
//   - the "field" KEY IS PRESENT on every finding (the old code omitted it),
//   - its value is NOT JSON null and NOT the empty string,
//   - "message" likewise.
//
// The positive control is the count: the fixture must produce a minimum number
// of findings in BOTH buckets, so "zero findings had a null field" cannot mean
// "there were no findings".
func TestValidateJSONEveryFindingCarriesAField(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, brokenSixWays)

	payload, _, err := decodeValidateJSON(t, dir)
	if err == nil {
		t.Fatal("a manifest this broken must still exit non-zero in --json mode")
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Errorf("broken manifest reported ok:true: %v", payload)
	}

	errs := findingObjects(t, payload, "errors")
	warns := findingObjects(t, payload, "warnings")

	// Positive control. The issue's manifest produced 7 findings; requiring
	// several in each bucket means a harness that observed nothing cannot pass.
	if len(errs) < 5 {
		t.Fatalf("expected at least 5 errors from the six-ways-broken manifest, got %d: %v", len(errs), errs)
	}
	if len(warns) < 1 {
		t.Fatalf("expected at least 1 warning from the six-ways-broken manifest, got %d: %v", len(warns), warns)
	}

	for _, bucket := range []struct {
		name  string
		items []map[string]any
	}{{"errors", errs}, {"warnings", warns}} {
		for i, item := range bucket.items {
			raw, present := item["field"]
			if !present {
				t.Errorf("%s[%d] has NO \"field\" key — issue #225 is back: %v", bucket.name, i, item)
				continue
			}
			if raw == nil {
				t.Errorf("%s[%d] has \"field\": null — issue #225 is back: %v", bucket.name, i, item)
				continue
			}
			s, ok := raw.(string)
			if !ok {
				t.Errorf("%s[%d] \"field\" is %T, want string: %v", bucket.name, i, raw, item)
				continue
			}
			if s == "" {
				t.Errorf("%s[%d] has an EMPTY \"field\": %v", bucket.name, i, item)
			}
			msg, ok := item["message"].(string)
			if !ok || msg == "" {
				t.Errorf("%s[%d] has no usable \"message\": %v", bucket.name, i, item)
			}
		}
	}
}

// TestValidateJSONSemanticChecksAreFieldTagged is the SHARP version of the test
// above: it names the four findings that came back null and requires each to
// carry a SPECIFIC field.
//
// "at least one finding has a field" — which is what this file's predecessor
// asserted — was green throughout the entire life of the bug, because the schema
// errors always had one. The claim that matters is per-check, so it is asserted
// per-check.
func TestValidateJSONSemanticChecksAreFieldTagged(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, brokenSixWays)

	payload, _, err := decodeValidateJSON(t, dir)
	if err == nil {
		t.Fatal("broken manifest must exit non-zero")
	}

	byMessageFragment := map[string]string{}
	for _, item := range append(findingObjects(t, payload, "errors"), findingObjects(t, payload, "warnings")...) {
		msg, _ := item["message"].(string)
		field, _ := item["field"].(string)
		byMessageFragment[msg] = field
	}

	// Each entry: a fragment identifying ONE finding, and the field it must
	// carry. These are the exact four the issue reported as null, plus the two
	// schema findings whose notation changed.
	want := []struct {
		fragment string
		field    string
	}{
		{"MUST NOT combine allow-same-origin with allow-scripts", "iframe.sandbox"},
		{`sandbox token "allow-popups" is not allowed`, "iframe.sandbox"},
		{"sensitive scopes require a justification", "scopeJustifications"},
		{"page.buzzBudgetPerGen is not set", "page.buzzBudgetPerGen"},
		{"iframe.minHeight is required", "iframe.minHeight"},
		{"iframe.resizable is required", "iframe.resizable"},
		// Schema findings: dotted, not JSON Pointer.
		{"does not match pattern", "blockId"},
		{"value must be one of", "scopes[2]"},
	}
	for _, w := range want {
		var found bool
		for msg, field := range byMessageFragment {
			if !strings.Contains(msg, w.fragment) {
				continue
			}
			found = true
			if field != w.field {
				t.Errorf("finding %q carries field %q, want %q", w.fragment, field, w.field)
			}
		}
		if !found {
			t.Errorf("the fixture did not produce the finding %q at all — this test is not "+
				"checking what it claims to. Findings seen: %v", w.fragment, byMessageFragment)
		}
	}
}

// TestValidateJSONFieldsUseDottedNotation pins the SECONDARY defect: three
// notations used to coexist (`(root)`, `/blockId`, `iframe.sandbox`). Exactly
// one survives on the wire.
//
// It asserts the NEGATIVE — no field is a JSON Pointer — because that is the
// notation that was there. A positive-only assertion ("some field is dotted")
// was true before the fix too.
func TestValidateJSONFieldsUseDottedNotation(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, brokenSixWays)

	payload, _, _ := decodeValidateJSON(t, dir)
	var sawNested, sawIndexed bool
	items := append(findingObjects(t, payload, "errors"), findingObjects(t, payload, "warnings")...)
	for _, item := range items {
		field, _ := item["field"].(string)
		if strings.HasPrefix(field, "/") || strings.Contains(field, "/") {
			t.Errorf("field %q is a JSON Pointer; --json is unified on dotted paths: %v", field, item)
		}
		switch {
		case field == "(root)" || field == "(project)":
			// A documented sentinel, not a path — see the README --json table.
		case strings.Contains(field, "["):
			sawIndexed = true
		case strings.Contains(field, "."):
			sawNested = true
		}
	}
	// Positive controls: the fixture really does produce the shapes whose
	// notation is being asserted, so the negative above had something to catch.
	if !sawNested {
		t.Error("positive control: no nested field (e.g. iframe.sandbox) in the payload")
	}
	if !sawIndexed {
		t.Error("positive control: no array-indexed field (e.g. scopes[2]) in the payload")
	}
}

// TestValidateJSONStreamSeparation pins what the issue explicitly asked be kept:
// the JSON goes to STDOUT and nothing else does, while the failure is signalled
// out of band. `main` prints "Error: …" to stderr; at this level that is the
// non-nil error, and stdout must stay machine-parseable in full.
func TestValidateJSONStreamSeparation(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, brokenSixWays)

	out, errOut, err := run(t, "app", "validate", dir, "--json")
	if err == nil {
		t.Fatal("broken manifest must exit non-zero")
	}
	// The WHOLE of stdout must decode. A single stray human line would break a
	// consumer piping stdout into jq, and would not be caught by decoding a
	// prefix.
	var payload map[string]any
	if uerr := json.Unmarshal([]byte(out), &payload); uerr != nil {
		t.Fatalf("stdout is not pure JSON — stream separation broken: %v\nstdout:\n%s", uerr, out)
	}
	if strings.TrimSpace(errOut) != "" {
		t.Errorf("--json mode wrote to stderr; the human summary must be suppressed:\n%s", errOut)
	}
}

// TestValidateJSONCleanManifestHasEmptyBuckets is the negative control for the
// tests above: on a manifest with nothing wrong, the same code path must produce
// ZERO findings and ok:true. Without it, a harness that fabricated a field on
// everything would look identical.
func TestValidateJSONCleanManifestHasEmptyBuckets(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)
	if _, _, err := run(t, "app", "init", "clean-field-block"); err != nil {
		t.Fatalf("app init: %v", err)
	}
	dir := filepath.Join(tmp, "clean-field-block")

	payload, _, err := decodeValidateJSON(t, dir)
	if err != nil {
		t.Fatalf("clean project should exit 0: %v", err)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Errorf("clean project should report ok:true: %v", payload)
	}
	if n := len(findingObjects(t, payload, "errors")); n != 0 {
		t.Errorf("clean project should have no errors, got %d", n)
	}
}

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
