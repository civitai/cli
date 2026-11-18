package scaffold

import (
	"strings"
	"unicode"
)

// THE SCAFFOLDED TAGLINE.
//
// 🔴 WHY THE TEMPLATES CARRY ONE AT ALL. `tagline` is the one store-listing field
// the MANIFEST owns: it flows to the listing when `civitai app submit` mints the
// draft and is re-derived from the manifest on every subsequent approved version
// (`buildListingScalarSync`, scoped `kind: 'onsite'`). None of the three manifest
// templates carried the key, so EVERY scaffolded app started life with an empty
// tagline on the one listing field its author could actually set from the repo —
// and `civitai app doctor` reported `empty-tagline` on it forever, as an advisory
// that the measured agent trial read as optional and stopped on.
//
// 🔴 IT IS NOT A PLACEHOLDER, AND THAT IS THE DECISION RATHER THAN A DETAIL. A
// `"TODO: one-line pitch"` would ship a string that is worse than the empty one
// it replaces: it satisfies `doctor` while putting literal TODO text on a public
// store card, so the problem stops being reported at the exact moment it becomes
// visible to users. What the scaffold KNOWS is the name the author just gave
// `civitai app create`, and "<Name> - a Civitai app" is a true sentence about
// every app this scaffold produces. It is a real default, not a marker, and it is
// obviously worth replacing without pretending to be unfinished.

// taglineSuffix is what a derived tagline appends to the app's name. ASCII on
// purpose: no template in this scaffold ships a non-ASCII byte today, and an
// em-dash here would make this the first one for a purely cosmetic gain.
const taglineSuffix = " - a Civitai app"

// taglineMaxRunes is the bound the canonical schema publishes for `tagline`
// (`maxLength: 140` in schema/app-block.manifest.schema.json, matching the
// server's MANIFEST_TAGLINE_MAX_LENGTH).
//
// 🔴 IT IS COUNTED ON THE RAW STRING, AND THE DERIVATION NEVER PADS. The schema
// counts the raw value while the server measures the TRIMMED length, so the two
// disagree for a string with surrounding whitespace — a value padded past 140 is
// refused locally and accepted server-side. Nothing derived here can be in that
// gap: the name is trimmed and its interior whitespace collapsed BEFORE the
// suffix is appended, so the result has no leading or trailing space and its raw
// and trimmed lengths are equal.
const taglineMaxRunes = 140

// taglineFallback is the tagline for a name that survives normalisation as
// nothing at all. `minLength: 1` plus `pattern: "\\S"` means the empty string and
// a whitespace-only string are both schema-INVALID, and `app init` validates the
// manifest it just wrote — so this arm is what keeps a pathological name from
// turning into an "internal error: scaffolded manifest failed validation".
const taglineFallback = "A Civitai app"

// TaglineFromName derives a scaffolded app's store tagline from its display name.
//
// The result is always schema-valid for the canonical `tagline`: non-empty, with
// at least one non-whitespace rune, no leading or trailing whitespace, and at
// most taglineMaxRunes runes counted the way the schema counts them.
//
// 🔴 IT STRIPS WHAT WOULD BREAK THE FILE IT IS WRITTEN INTO. The manifest
// templates interpolate this value into a JSON string literal by plain
// substitution, so a `"` or a `\` in the name would render syntactically invalid
// JSON, and a control byte would render a JSON document no parser accepts. Those
// are removed rather than escaped: the name is the author's own and they can
// retype it, whereas an escaping scheme in a template is a second encoder to keep
// in step with `encoding/json`. (The pre-existing `{{ .Name }}` substitution in
// those same templates has the same exposure and is out of scope here — this
// function is only responsible for what IT emits.)
//
// 🔴 THE LENGTH ARM TRUNCATES THE NAME, NOT THE RESULT. Cutting the composed
// string would leave a half-written " - a Civitai a", which reads as a bug rather
// than as a long name; reserving room for the suffix keeps every output a
// complete sentence. `name` has no length cap in the schema, so this arm is
// reachable rather than defensive.
func TaglineFromName(name string) string {
	n := normaliseTaglineName(name)
	if n == "" {
		return taglineFallback
	}
	room := taglineMaxRunes - len([]rune(taglineSuffix))
	if r := []rune(n); len(r) > room {
		// TrimRight after the cut: the cut can land immediately after a space,
		// and a trailing space before the suffix would double it.
		n = strings.TrimRight(string(r[:room]), " ")
		if n == "" {
			return taglineFallback
		}
	}
	return n + taglineSuffix
}

// normaliseTaglineName reduces a display name to the characters that may appear
// in a JSON string literal, with interior whitespace collapsed to single spaces
// and the ends trimmed.
func normaliseTaglineName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	space := false
	for _, r := range name {
		switch {
		case r == '"' || r == '\\':
			// Dropped, not escaped — see TaglineFromName.
			continue
		case unicode.IsSpace(r):
			// Every run of whitespace (including a tab or a newline, which are
			// also illegal raw in a JSON string) becomes one plain space.
			space = true
			continue
		case unicode.IsControl(r):
			continue
		}
		if space && b.Len() > 0 {
			b.WriteRune(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}
