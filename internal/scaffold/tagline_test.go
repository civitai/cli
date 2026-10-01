package scaffold_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/validate"
)

// THE SCAFFOLDED TAGLINE (civitai/cli#762, decision D2).
//
// None of the three manifest templates carried `tagline`, so every scaffolded app
// started with an empty value on the one store-listing field the MANIFEST owns —
// and `civitai app doctor` reported `empty-tagline` on it forever, as an advisory
// that a measured agent trial read as optional.
//
// 🔴 THE BOUNDS ARE READ FROM THE CANONICAL SCHEMA, NOT RESTATED HERE. The
// template is validated through `validate.ManifestOnly`, which compiles the
// embedded mirror of the platform's schema — so `minLength: 1`,
// `pattern: "\\S"` and `maxLength: 140` are enforced by the same code the CLI
// enforces them with, rather than by a second copy of the numbers that can drift
// from it. The explicit assertions below are the ones the SCHEMA cannot make: that
// the value is not the empty string the templates used to imply, and that it never
// arrives with surrounding whitespace (the schema counts the RAW length while the
// server measures the TRIMMED one, so a padded value is refused locally and
// accepted server-side — the asymmetry the schema's own description warns about).

// taglineMaxRunes mirrors the canonical schema's `maxLength` for `tagline`. It is
// spelled here rather than imported from the package under test, because an
// expectation derived from the implementation it checks asserts nothing.
const taglineSchemaMaxRunes = 140

// TestScaffoldedManifestCarriesARealTagline is the regression test for the defect:
// at origin/main every one of these templates renders a manifest with NO `tagline`
// key at all.
func TestScaffoldedManifestCarriesARealTagline(t *testing.T) {
	examined := 0
	for _, tmpl := range scaffold.AllTemplates() {
		examined++
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), string(tmpl))
			// A display name that is NOT the slug and is NOT a lowercase
			// identifier, so a tagline echoing either can be told apart from one
			// derived from the name.
			if _, err := scaffold.Render(tmpl, dest, scaffold.Data{
				Slug: "tagline-block", Name: "Batch Upscaler",
			}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
			}

			raw, err := os.ReadFile(filepath.Join(dest, "block.manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("the rendered manifest is not valid JSON (%v):\n%s", err, raw)
			}
			v, ok := doc["tagline"]
			if !ok {
				t.Fatalf("the %q template renders no `tagline` key — every scaffolded app starts with an "+
					"`empty-tagline` problem on the one listing field its author can set from the repo:\n%s",
					tmpl, raw)
			}
			tagline, ok := v.(string)
			if !ok {
				t.Fatalf("`tagline` is %T, not a string: %#v", v, v)
			}

			if strings.TrimSpace(tagline) == "" {
				t.Errorf("`tagline` is blank (%q) — the schema's `pattern: \"\\\\S\"` refuses it, and a blank "+
					"value is the defect this ships to fix", tagline)
			}
			if tagline != strings.TrimSpace(tagline) {
				t.Errorf("`tagline` carries surrounding whitespace (%q). The schema counts the RAW length "+
					"and the server measures the TRIMMED one, so a padded value can be refused locally "+
					"while the server would accept it", tagline)
			}
			if n := utf8.RuneCountInString(tagline); n > taglineSchemaMaxRunes {
				t.Errorf("`tagline` is %d runes; the canonical schema caps it at %d", n, taglineSchemaMaxRunes)
			}
			// 🔴 IT IS NOT A PLACEHOLDER. A `TODO:`-shaped default would satisfy
			// every assertion above while putting unfinished text on a public
			// store card — i.e. it would stop `app doctor` reporting the problem
			// at the exact moment the problem becomes visible to users.
			for _, marker := range []string{"TODO", "FIXME", "XXX", "<", ">"} {
				if strings.Contains(tagline, marker) {
					t.Errorf("`tagline` = %q contains %q — the scaffolded default must be a real sentence, "+
						"not a marker or a template stub", tagline, marker)
				}
			}
			// Derived from the NAME the author gave, which is the whole point of
			// not shipping one fixed string.
			if !strings.Contains(tagline, "Batch Upscaler") {
				t.Errorf("`tagline` = %q does not carry the app's name — it is not derived from what the "+
					"author typed", tagline)
			}

			// The bounds the schema owns, enforced by the code that owns them.
			res, verr := validate.ManifestOnly(dest)
			if verr != nil {
				t.Fatalf("validate.ManifestOnly(%s): %v", tmpl, verr)
			}
			if !res.OK() {
				t.Fatalf("the scaffolded %q manifest FAILS the CLI's own validate:\n  %v", tmpl, validate.Messages(res.Errors))
			}
		})
	}
	// CONTROL: the loop asserts nothing if AllTemplates() is empty.
	if examined != 3 {
		t.Fatalf("examined %d template(s), want 3 — the loop is not walking what it claims to", examined)
	}
}

// TestTaglineFromNameStaysInsideTheSchemaBounds drives the derivation directly,
// over the inputs a rendered template cannot reach: a name with JSON-hostile
// characters, a name that is nothing but whitespace, and a name long enough that
// the composed tagline would overrun the schema's cap.
//
// 🔴 THE LONG-NAME ARM IS REACHABLE, NOT DEFENSIVE. The canonical schema puts
// `minLength: 1` and no maximum on `name`, so an author may legitimately type one
// longer than the tagline cap and `civitai app init` validates the manifest it
// just wrote — an overrun would surface as "internal error: scaffolded manifest
// failed validation" on a perfectly legal name.
func TestTaglineFromNameStaysInsideTheSchemaBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string // "" => only the invariants are asserted
		why  string
	}{
		{
			name: "an ordinary name",
			in:   "Batch Upscaler",
			want: "Batch Upscaler - a Civitai app",
			why:  "the whole normalised string is pinned, so a reword has to be re-approved deliberately",
		},
		{
			name: "surrounding and interior whitespace is normalised",
			in:   "  Batch\t\nUpscaler  ",
			want: "Batch Upscaler - a Civitai app",
			why:  "a tab or a newline is illegal raw inside a JSON string, and padding moves the raw length",
		},
		{
			name: "a quote and a backslash are dropped, not escaped",
			in:   `The "Best" C:\Upscaler`,
			want: "The Best C:Upscaler - a Civitai app",
			why: "the template substitutes this into a JSON string literal by hand; an unescaped quote " +
				"renders a manifest no parser accepts",
		},
		{
			name: "a name that normalises to nothing gets the fallback",
			in:   "   \t  ",
			want: "A Civitai app",
			why:  "`minLength: 1` plus `pattern: \"\\\\S\"` refuse the empty and the whitespace-only string",
		},
		{
			name: "control characters are dropped",
			in:   "Batch\x00\x07Upscaler",
			want: "BatchUpscaler - a Civitai app",
			why:  "a control byte renders a JSON document no parser accepts",
		},
		{
			name: "a very long name is truncated to fit the suffix",
			in:   strings.Repeat("Ab", 200),
			why:  "the suffix must survive; a cut composed string would read as a bug rather than a long name",
		},
		{
			name: "a long name whose cut lands on a space does not double it",
			in:   strings.Repeat("Ab ", 200),
			why: "the trailing space before the suffix would otherwise be doubled, and a trailing space " +
				"in the value moves the raw length the schema counts",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scaffold.TaglineFromName(tc.in)
			if tc.want != "" && got != tc.want {
				t.Errorf("TaglineFromName(%q) = %q, want %q — %s", tc.in, got, tc.want, tc.why)
			}
			// The invariants hold for EVERY input, including the two long arms
			// that pin no exact string.
			if strings.TrimSpace(got) == "" {
				t.Errorf("TaglineFromName(%q) is blank — the schema refuses it", tc.in)
			}
			if got != strings.TrimSpace(got) {
				t.Errorf("TaglineFromName(%q) = %q carries surrounding whitespace", tc.in, got)
			}
			if n := utf8.RuneCountInString(got); n > taglineSchemaMaxRunes {
				t.Errorf("TaglineFromName(%q) is %d runes, over the schema's %d cap", tc.in, n, taglineSchemaMaxRunes)
			}
			if strings.ContainsAny(got, "\"\\") {
				t.Errorf("TaglineFromName(%q) = %q carries a character that breaks the JSON string it is "+
					"substituted into", tc.in, got)
			}
			for _, r := range got {
				if r < 0x20 || r == 0x7f {
					t.Errorf("TaglineFromName(%q) = %q carries the control rune %U", tc.in, got, r)
				}
			}
		})
	}
}

// TestRenderHonoursAnExplicitTagline pins the override arm, so the derivation is
// a DEFAULT rather than the only reachable value. Without this the field could be
// ignored entirely and every other test here would still pass.
func TestRenderHonoursAnExplicitTagline(t *testing.T) {
	const want = "Turns a folder of PNGs into a contact sheet"
	dest := filepath.Join(t.TempDir(), "explicit")
	if _, err := scaffold.Render(scaffold.Static, dest, scaffold.Data{
		Slug: "explicit-block", Name: "Explicit Block", Tagline: want,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dest, "block.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Tagline string `json:"tagline"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the rendered manifest is not valid JSON (%v):\n%s", err, raw)
	}
	if doc.Tagline != want {
		t.Errorf("tagline = %q, want the explicit value %q — Render overrode a caller-supplied tagline "+
			"with its own derivation", doc.Tagline, want)
	}
}
