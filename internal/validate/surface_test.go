package validate

import (
	"bytes"
	"strings"
	"testing"

	cli "github.com/civitai/cli"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The literal the CLI must print for a manifest that declares no surface. It is
// spelled out here rather than read from surfaceMessage so that a change to the
// production string is a visible, reviewed test change. Its wording follows the
// platform's manifestSurfaceError (civitai block-manifest-validator.service.ts).
const wantSurfaceMessage = `manifest must declare at least one surface: add a "page" object for a full-page app ` +
	`(e.g. "page": { "path": "/", "title": "My App" }), or a non-empty "targets" array for a ` +
	`model-page slot app (e.g. "targets": [{ "slotId": "model.sidebar_top" }])`

// surfaceManifest is a manifest that is valid in every respect except its
// surface: iframe is declared so the renderMode=iframe semantic check stays
// quiet, and `extra` is spliced in as additional top-level keys.
func surfaceManifest(extra string) string {
	if extra != "" {
		extra = ",\n  " + extra
	}
	return `{
  "blockId": "ok-block",
  "version": "0.1.0",
  "name": "OK",
  "contentRating": "g",
  "scopes": [],
  "iframe": {"minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms"}` + extra + `
}`
}

// rawSchemaRejects reports the vendored schema's OWN verdict on doc, with no
// CLI message processing in between.
func rawSchemaRejects(t *testing.T, doc string) bool {
	t.Helper()
	s, err := schema()
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("fixture is not JSON: %v", err)
	}
	return s.Validate(inst) != nil
}

// REGRESSION (red before the surface rewording): a manifest with neither
// `page` nor `targets` used to print the schema's two anyOf leaves —
// "(root): missing property 'page'" and "(root): missing property 'targets'" —
// neither of which says that EITHER key will do. It must now print exactly one
// finding, at the document root, that names both fixes.
func TestValidateNoSurfaceNamesTheFix(t *testing.T) {
	cases := []struct{ name, extra string }{
		{"neither page nor targets", ""},
		{"empty targets and no page", `"targets": []`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := surfaceManifest(tc.extra)
			if !rawSchemaRejects(t, doc) {
				t.Fatalf("precondition: the vendored schema must reject this manifest itself")
			}
			res := validateManifest(t, doc)
			if len(res.Errors) != 1 {
				t.Fatalf("want exactly one error (the surface finding), got %d: %v", len(res.Errors), res.Errors)
			}
			got := res.Errors[0]
			if got.Message != wantSurfaceMessage {
				t.Errorf("message:\n got %q\nwant %q", got.Message, wantSurfaceMessage)
			}
			if got.Field != FieldDocument {
				t.Errorf("field: got %q, want %q", got.Field, FieldDocument)
			}
			for _, generic := range []string{"missing property 'page'", "missing property 'targets'", "minItems"} {
				if strings.Contains(got.Message, generic) {
					t.Errorf("the generic anyOf leaf %q leaked through: %q", generic, got.Message)
				}
			}
		})
	}
}

// A manifest with a surface passes, and the CLI's verdict agrees with the raw
// schema's in BOTH directions for every surface shape — the rewording changes
// the message only, never the outcome.
func TestValidateSurfaceVerdictMatchesSchema(t *testing.T) {
	cases := []struct {
		name, extra string
		wantOK      bool
	}{
		{"page only", `"page": {"path": "/", "title": "OK"}`, true},
		{"targets only", `"targets": [{"slotId": "model.sidebar_top"}]`, true},
		{"page and targets", `"page": {"path": "/", "title": "OK"}, "targets": [{"slotId": "model.sidebar_top"}]`, true},
		{"page with empty targets", `"page": {"path": "/", "title": "OK"}, "targets": []`, true},
		{"neither", "", false},
		{"empty targets, no page", `"targets": []`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := surfaceManifest(tc.extra)
			res := validateManifest(t, doc)
			if res.OK() != tc.wantOK {
				t.Fatalf("OK()=%v, want %v; errors: %v", res.OK(), tc.wantOK, res.Errors)
			}
			if schemaOK := !rawSchemaRejects(t, doc); schemaOK != tc.wantOK {
				t.Fatalf("raw schema verdict %v disagrees with want %v — the fixture no longer tests what it claims", schemaOK, tc.wantOK)
			}
			hasSurface := false
			for _, e := range res.Errors {
				if e.Message == wantSurfaceMessage {
					hasSurface = true
				}
			}
			if hasSurface == tc.wantOK {
				t.Errorf("surface finding present=%v on a manifest whose verdict is OK=%v: %v", hasSurface, tc.wantOK, res.Errors)
			}
		})
	}
}

// The rewording targets the surface clause ONLY. A wrong-typed surface key
// satisfies the anyOf (minItems ignores non-arrays; `required` only asks that
// the key exist) and fails on its own `type` — that message must survive
// untouched, and the surface sentence must not appear.
func TestValidateWrongTypedSurfaceKeepsTheSchemaMessage(t *testing.T) {
	cases := []struct{ name, extra, want string }{
		{"targets is an object", `"targets": {}`, "targets: got object, want array"},
		{"page is null", `"page": null`, "page: got null, want object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := validateManifest(t, surfaceManifest(tc.extra))
			var msgs []string
			for _, e := range res.Errors {
				msgs = append(msgs, e.Message)
				if e.Message == wantSurfaceMessage {
					t.Errorf("surface message emitted for a type error: %v", res.Errors)
				}
			}
			if !strings.Contains(strings.Join(msgs, "\n"), tc.want) {
				t.Errorf("errors %q do not contain the schema's own %q", msgs, tc.want)
			}
		})
	}
}

// The schema has other `anyOf`s (storage needs `collections` or `imports`).
// Their failures must keep the schema's own leaves — the rewrite is keyed on
// the surface clause's location, not on "any anyOf".
func TestValidateOtherAnyOfIsNotReworded(t *testing.T) {
	doc := surfaceManifest(`"page": {"path": "/", "title": "OK"}, "storage": {}`)
	if !rawSchemaRejects(t, doc) {
		t.Fatalf("precondition: the schema must reject an empty storage block")
	}
	res := validateManifest(t, doc)
	var msgs []string
	for _, e := range res.Errors {
		msgs = append(msgs, e.Message)
		if e.Message == wantSurfaceMessage {
			t.Errorf("storage anyOf failure was reworded as a surface error: %v", res.Errors)
		}
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{"storage: missing property 'collections'", "storage: missing property 'imports'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("errors %q lack the schema's own %q", msgs, want)
		}
	}
}

// INVARIANT GUARD (not regression coverage): the embedded schema carries the
// surface clause and surfaceClauseIndex finds it. If a re-vendor reshapes the
// clause, this goes red instead of the CLI silently falling back to the
// generic anyOf leaves. The negative rows show the matcher declines anything
// that is not exactly that clause.
func TestSurfaceClauseIsFoundInTheEmbeddedSchema(t *testing.T) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(cli.SchemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	if i := surfaceClauseIndex(doc); i < 0 {
		t.Fatalf("the embedded schema has no surface clause the CLI recognises — the no-surface message would fall back to the schema's generic anyOf leaves")
	}
	if _, err := schema(); err != nil {
		t.Fatal(err)
	}
	if surfaceClauseURL == "" {
		t.Fatalf("schema() did not record surfaceClauseURL")
	}

	decline := map[string]string{
		"no allOf":                  `{}`,
		"no anyOf":                  `{"allOf":[{"if":{},"then":{}}]}`,
		"targets lacks minItems":    `{"allOf":[{"anyOf":[{"required":["targets"]},{"required":["page"]}]}]}`,
		"minItems is 2":             `{"allOf":[{"anyOf":[{"required":["targets"],"properties":{"targets":{"minItems":2}}},{"required":["page"]}]}]}`,
		"three branches":            `{"allOf":[{"anyOf":[{"required":["targets"],"properties":{"targets":{"minItems":1}}},{"required":["page"]},{"required":["x"]}]}]}`,
		"a branch needs two":        `{"allOf":[{"anyOf":[{"required":["targets","x"],"properties":{"targets":{"minItems":1}}},{"required":["page"]}]}]}`,
		"page twice":                `{"allOf":[{"anyOf":[{"required":["page"]},{"required":["page"]}]}]}`,
		"targets or something else": `{"allOf":[{"anyOf":[{"required":["targets"],"properties":{"targets":{"minItems":1}}},{"required":["x"]}]}]}`,
	}
	for name, src := range decline {
		d, err := jsonschema.UnmarshalJSON(strings.NewReader(src))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if i := surfaceClauseIndex(d); i != -1 {
			t.Errorf("%s: matched index %d, want -1", name, i)
		}
	}
	// Positive control at a non-zero index, so a matcher that always answers 0
	// cannot pass.
	d, _ := jsonschema.UnmarshalJSON(strings.NewReader(
		`{"allOf":[{"if":{},"then":{}},{"anyOf":[{"required":["page"]},{"required":["targets"],"properties":{"targets":{"minItems":1}}}]}]}`))
	if i := surfaceClauseIndex(d); i != 1 {
		t.Errorf("positive control: got index %d, want 1", i)
	}
}

// The slot id in the message's example must be a real model-page slot, not the
// page slot, or the example would itself fail targetChecks.
func TestSurfaceMessageExampleSlotIsAKnownModelSlot(t *testing.T) {
	const slot = "model.sidebar_top"
	if !strings.Contains(surfaceMessage, `"slotId": "`+slot+`"`) {
		t.Fatalf("surfaceMessage no longer uses %q as its example slot; update this test with it", slot)
	}
	if !isKnownSlotID(slot) || isPageSlotID(slot) {
		t.Errorf("%q must be a known, non-page slot id", slot)
	}
}
