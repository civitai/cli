package scaffold

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// runesThatLowerIntoASCII walks ALL of Unicode for runes above ASCII whose
// unicode.ToLower lands back inside ASCII. It is a walk rather than a literal
// pair so the "exactly two" claim in Slugify's header tracks the Go unicode
// tables instead of somebody's memory of them.
func runesThatLowerIntoASCII() []rune {
	var out []rune
	for r := rune(utf8.RuneSelf); r <= unicode.MaxRune; r++ {
		if !utf8.ValidRune(r) {
			continue
		}
		if unicode.ToLower(r) < utf8.RuneSelf {
			out = append(out, r)
		}
	}
	return out
}

// legacySlugify is the derivation EXACTLY as it stood before the refusal landed
// (`e800129`-era `Slugify`, minus the two new guards). It exists so a
// `mustNotProduce` row is a MEASUREMENT of the old behaviour rather than a
// remembered string: a row naming an output the old algorithm never produced
// would be evidence about nothing, and nothing else in the suite could tell.
func legacySlugify(name string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(name))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) < 3 {
		return "", fmt.Errorf("cannot derive a valid slug from %q (need ≥3 chars)", name)
	}
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if !slugPattern.MatchString(s) {
		return "", fmt.Errorf("derived slug %q is invalid", s)
	}
	return s, nil
}

// TestSlugifyRefusesRatherThanDroppingCharacters is the headline of issue #259.
//
// 🔴 EACH ROW PINS THE OLD OUTPUT IT MUST NOT PRODUCE, not merely "an error".
// "an error came back" is satisfied by a refusal for the wrong reason and by a
// refusal that happens to fire everywhere; naming `berapp` / `caf-del-mar` is
// what makes the row evidence about THIS defect.
//
// 🔴 AND THE ASSERTION HAS TO EXECUTE. The first version put that check behind a
// `t.Fatalf("want a refusal")` that had already aborted the subtest whenever
// `err == nil`, so the `mustNotProduce` block was UNREACHABLE — measured:
// deleting it left the package green while a positive control reddened 1, i.e.
// the harness could go red and this assertion simply never ran. The PR body's
// headline claim rested on it. It now lives INSIDE the `err == nil` branch,
// where it is the only thing that can distinguish "derivation came back" from
// "derivation came back with the exact #259 mangling".
func TestSlugifyRefusesRatherThanDroppingCharacters(t *testing.T) {
	cases := []struct {
		name string
		// mustNotProduce is what Slugify returned BEFORE the fix — a silently
		// different permanent public identity. "" means the pre-fix derivation
		// bottomed out in one of the two ASCII dead ends instead.
		mustNotProduce string
		// chars are the characters the refusal has to point at, AS TYPED.
		chars []string
	}{
		{
			// The leading Ü becomes a hyphen and is then trimmed, so the app's
			// public id loses its first letter outright.
			name:           "ÜberApp Ω",
			mustNotProduce: "berapp",
			chars:          []string{"Ü", "Ω"},
		},
		{
			// Worse than dropping: mid-string it INSERTS a word boundary that
			// the author never typed.
			name:           "Café Del Mar",
			mustNotProduce: "caf-del-mar",
			chars:          []string{"é"},
		},
		{
			// 🔴 THE ROW THAT PINS "REPORT THE ORIGINAL, NOT THE LOWERED RUNE".
			// U+1E9E LATIN CAPITAL LETTER SHARP S lowers to U+00DF ß — a
			// DIFFERENT character, and one absent from this input. The first
			// version classified and reported off strings.ToLower(name) and so
			// quoted "ß" at a user who never typed it.
			name:           "\u1e9eE App",
			mustNotProduce: "e-app",
			chars:          []string{"\u1e9e"},
		},
		{
			// Same class, fullwidth: Ａ lowers to ａ, so the pre-fix message
			// named "ａ", "ｂ", "ｃ" instead of what was typed.
			name:           "\uff21\uff22\uff23 Widget",
			mustNotProduce: "widget",
			chars:          []string{"\uff21", "\uff22", "\uff23"},
		},
		{
			name:           "日本語アプリ",
			mustNotProduce: "",
			chars:          []string{"日", "本", "語"},
		},
		{
			name:           "Приложение",
			mustNotProduce: "",
			chars:          []string{"П", "р"},
		},
		{
			name:           "تطبيق",
			mustNotProduce: "",
			chars:          []string{"ت", "ط"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Positive control on the FIXTURE: mustNotProduce must be what the
			// pre-fix derivation really produced, or the row below is asserting
			// against a string nothing ever emitted.
			legacy, legacyErr := legacySlugify(tc.name)
			if tc.mustNotProduce == "" {
				if legacyErr == nil {
					t.Fatalf("fixture is broken: the pre-fix derivation produced %q for %q, so mustNotProduce must name it", legacy, tc.name)
				}
			} else if legacyErr != nil || legacy != tc.mustNotProduce {
				t.Fatalf("fixture is broken: the pre-fix derivation gave (%q, %v) for %q, want %q", legacy, legacyErr, tc.name, tc.mustNotProduce)
			}

			got, err := Slugify(tc.name)
			if err == nil {
				// 🔴 This is the reachable form of "the pre-fix output must be
				// unreachable". A build that reintroduces the mangling fails
				// HERE, naming it, rather than in the generic branch below.
				if tc.mustNotProduce != "" && got == tc.mustNotProduce {
					t.Fatalf("Slugify(%q) = %q — that is the exact pre-fix mangling issue #259 is about, a different permanent public id than the author typed", tc.name, got)
				}
				t.Fatalf("Slugify(%q) = %q, want a refusal", tc.name, got)
			}
			if got != "" {
				t.Errorf("a refused Slugify must return no slug, got %q", got)
			}
			msg := err.Error()
			for _, c := range tc.chars {
				if !strings.Contains(msg, c) {
					t.Errorf("refusal must NAME the offending character %q AS TYPED: %s", c, msg)
				}
			}
			// 🔴 The refusal quotes the INPUT. Nothing asserted this, so a
			// mutant replacing the interpolated name with a fixed literal
			// survived — an error naming somebody else's name is worse than one
			// naming none.
			if want := fmt.Sprintf("%q", tc.name); !strings.Contains(msg, want) {
				t.Errorf("refusal must quote the name the author typed (%s): %s", want, msg)
			}
			// Actionable: it points at the escape hatch, and states the rule.
			if !strings.Contains(msg, "--slug") {
				t.Errorf("refusal must point at --slug: %s", msg)
			}
			if !strings.Contains(msg, "a-z") || !strings.Contains(msg, "hyphens") {
				t.Errorf("refusal must state the blockId constraint: %s", msg)
			}
		})
	}
}

// TestSlugifyReportsTheCharacterTheAuthorTyped is the same claim as the "ẞE App"
// row above, stated directly against LossyChars so it cannot be satisfied by any
// other part of the refusal sentence. Each row's lowered form is a character
// that is NOT in the input — that is what makes the row discriminating.
func TestSlugifyReportsTheCharacterTheAuthorTyped(t *testing.T) {
	cases := []struct {
		in            string
		want          []string
		mustNotReport []string
	}{
		{in: "\u1e9eE App", want: []string{"\u1e9e"}, mustNotReport: []string{"\u00df"}},
		{in: "\uff21\uff22\uff23", want: []string{"\uff21", "\uff22", "\uff23"}, mustNotReport: []string{"\uff41", "\uff42", "\uff43"}},
		{in: "ÜberApp Ω", want: []string{"Ü", "Ω"}, mustNotReport: []string{"ü", "ω"}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := strings.Join(LossyChars(tc.in), "")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("LossyChars(%q) = %q, must report the typed %q", tc.in, got, w)
				}
			}
			for _, bad := range tc.mustNotReport {
				if strings.Contains(got, bad) {
					t.Errorf("LossyChars(%q) = %q — %q is the LOWERED form and does not appear in the input", tc.in, got, bad)
				}
			}
			// And the same through the real message, so the fix cannot be
			// correct in the predicate and lost at the printer.
			_, err := Slugify(tc.in)
			if err == nil {
				t.Fatalf("Slugify(%q) should refuse", tc.in)
			}
			for _, bad := range tc.mustNotReport {
				if strings.Contains(err.Error(), bad) {
					t.Errorf("refusal names the lowered %q, absent from the input: %v", bad, err)
				}
			}
		})
	}
}

// TestSlugifyRefusesInvalidUTF8 — residual (a) of the Slugify header, now
// closed. `for _, r := range` yields U+FFFD per invalid byte and U+FFFD is a
// Symbol, i.e. the SEPARATOR branch, so mojibake derived a slug with rc 0 AND
// the scaffold wrote the raw bytes into block.manifest.json.
func TestSlugifyRefusesInvalidUTF8(t *testing.T) {
	// Measured pre-fix outputs, i.e. exactly the #259 shape.
	cases := map[string]string{
		"caf\xe9 app":     "caf-app",
		"\xff\xfeWidget":  "widget",
		"Sm\xc3rt Widget": "sm-rt-widget",
	}
	for in, legacy := range cases {
		t.Run(fmt.Sprintf("%q", in), func(t *testing.T) {
			// Positive control on the fixture: the bytes really are invalid and
			// the pre-fix derivation really produced the named slug.
			if got, err := legacySlugify(in); err != nil || got != legacy {
				t.Fatalf("fixture is broken: pre-fix derivation gave (%q, %v), want %q", got, err, legacy)
			}
			got, err := Slugify(in)
			if err == nil {
				t.Fatalf("Slugify(%q) = %q, want a refusal — invalid UTF-8 silently loses bytes", in, got)
			}
			if got != "" {
				t.Errorf("a refused Slugify must return no slug, got %q", got)
			}
			if !strings.Contains(err.Error(), "UTF-8") {
				t.Errorf("the refusal should say what is wrong with the name: %v", err)
			}
			if !strings.Contains(err.Error(), "--slug") {
				t.Errorf("refusal must point at the escape hatch: %v", err)
			}
		})
	}
}

// TestLossyCharsReportsACombiningMarkWithItsBase — F5. macOS paths and some
// paste routes deliver NFD, so the SAME VISIBLE NAME arrives as two different
// byte sequences. Before this, NFC reported "é" and NFD reported a bare
// combining acute rendered over nothing. The refusal SET is identical either
// way (a mark was always lossy); only the rendering differed.
func TestLossyCharsReportsACombiningMarkWithItsBase(t *testing.T) {
	const (
		nfc  = "Caf\u00e9 App"     // e-acute as ONE rune (U+00E9)
		nfd  = "Cafe\u0301 App"    // 'e' + U+0301 COMBINING ACUTE ACCENT
		bare = "\u0301Widget name" // a combining mark with no base at all
	)
	// Fixture control. A character that RENDERS identically can be a different
	// code point, so assert the bytes rather than trusting the source text.
	if nfc == nfd {
		t.Fatal("fixture is broken: the NFC and NFD forms must be different byte sequences")
	}
	if !strings.ContainsRune(nfd, '\u0301') || strings.ContainsRune(nfc, '\u0301') {
		t.Fatalf("fixture is broken: only the NFD form may carry U+0301 (nfc=% x nfd=% x)", nfc, nfd)
	}

	for _, tc := range []struct{ in, want string }{
		{nfc, "\u00e9"},
		{nfd, "e\u0301"},
	} {
		got := LossyChars(tc.in)
		if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("LossyChars(% x) = %q, want the single cluster [%q] the author actually sees", tc.in, got, tc.want)
		}
		// READABILITY is the whole point: the reported token must lead with a
		// base character, never a lone combining mark floating over nothing.
		if []rune(got[0])[0] == '\u0301' {
			t.Errorf("LossyChars(% x) reported a bare combining mark %q", tc.in, got[0])
		}
		if _, err := Slugify(tc.in); err == nil {
			t.Fatalf("Slugify(% x) should refuse", tc.in)
		}
	}

	// A leading mark has no base to attach to, so it is shown on a dotted
	// circle (the Unicode convention) rather than bare.
	got := LossyChars(bare)
	if len(got) == 0 {
		t.Fatal("a leading combining mark is still content derivation would drop")
	}
	if !strings.HasPrefix(got[0], dottedCircle) {
		t.Errorf("a base-less combining mark must be shown on a dotted circle, got %q", got[0])
	}
}

// TestSlugifyAsciiDerivationIsByteIdentical is the CONTROL. These rows were
// green before the refusal existed and must stay green: the predicate exempts
// every rune that LOWERS INTO ASCII by construction, so ordinary punctuation and
// multiple spaces still fold to a single hyphen. An invariant guard, not
// regression coverage — its job is to fail loudly if the refusal ever widens.
func TestSlugifyAsciiDerivationIsByteIdentical(t *testing.T) {
	cases := map[string]string{
		"My Cool Block":         "my-cool-block",
		"my-block":              "my-block",
		"  Spaced   Out  ":      "spaced-out",
		"Foo/Bar_Baz.Qux!":      "foo-bar-baz-qux",
		"a & b + c = d":         "a-b-c-d",
		"Trailing punctuation.": "trailing-punctuation",
		"UPPER case NAME":       "upper-case-name",
		"app2 v3":               "app2-v3",
		"a---b":                 "a-b",
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if err != nil {
			t.Errorf("Slugify(%q) errored, want %q: %v", in, want, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSlugifyAsciiExemptionBoundary pins the `< utf8.RuneSelf` comparison from
// BOTH sides. Widening it to `<=` was a surviving mutant: U+0080 is a C1 control
// nobody types, so no behavioural row saw the change — but the doc comment calls
// that boundary load-bearing, and an unpinned boundary is a claim nothing holds.
func TestSlugifyAsciiExemptionBoundary(t *testing.T) {
	// U+007F DEL is the last ASCII rune: exempt, folded to a hyphen like any
	// other non-slug ASCII character.
	if got, err := Slugify("del\x7fimiter app"); err != nil || got != "del-imiter-app" {
		t.Errorf("U+007F is ASCII and must stay exempt: Slugify = (%q, %v), want del-imiter-app", got, err)
	}
	// U+0080 is the first rune ABOVE ASCII. It is a control (Cc) — not space,
	// punct or symbol — so it is content with nowhere to go, and refusing it is
	// what the boundary says.
	got, err := Slugify("ctrl\u0080name app")
	if err == nil {
		t.Errorf("U+0080 is above ASCII and must be refused, got %q", got)
	}
	if len(LossyChars("ctrl\u0080name")) != 1 {
		t.Errorf("LossyChars must report U+0080: %q", LossyChars("ctrl\u0080name"))
	}
}

// TestSlugifyLowersIntoAsciiIsADocumentedException — residual (b). Exactly two
// runes above ASCII lower INTO ASCII, and both derive rather than refuse. This
// is deliberate (it is the nicest transliteration available and costs nothing),
// so it is pinned as BEHAVIOUR rather than left as an accident of the boundary:
// a future "tighten the exemption to `r < utf8.RuneSelf` on the ORIGINAL rune"
// would silently start refusing İstanbul.
func TestSlugifyLowersIntoAsciiIsADocumentedException(t *testing.T) {
	cases := map[string]string{
		"\u0130stanbul App": "istanbul-app", // U+0130 LATIN CAPITAL LETTER I WITH DOT ABOVE -> i
		"Temp Kelvin":       "temp-kelvin",  // U+212A KELVIN SIGN -> k
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if err != nil {
			t.Errorf("Slugify(%q) errored, want the documented %q: %v", in, want, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	// The claim is that these are the ONLY two. If a future Go unicode table
	// grows a third, the header's "exactly two" stops being true and this fails.
	if n := len(runesThatLowerIntoASCII()); n != 2 {
		t.Errorf("the header claims exactly 2 runes above ASCII lower into ASCII; found %d", n)
	}
}

// TestSlugifySymbolsAndEmojiStillFoldToAHyphen — residual (c), pinned as the
// deliberate behaviour it is rather than left undescribed. If someone decides to
// close it, this test is what says the change was intentional.
func TestSlugifySymbolsAndEmojiStillFoldToAHyphen(t *testing.T) {
	cases := map[string]string{
		"Rocket \U0001F680 App": "rocket-app",
		"Widget \u2764 Pro":     "widget-pro",
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if err != nil {
			t.Errorf("Slugify(%q) errored, want the documented %q: %v", in, want, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q — symbols are a documented exception", in, got, want)
		}
	}
}

// TestSlugifyNonAsciiSeparatorsStaySeparators: above ASCII the classification is
// by Unicode category, so a punctuation or symbol rune is still a separator with
// no content of its own. Getting this wrong would refuse names that derive
// perfectly well — the false-refusal direction.
func TestSlugifyNonAsciiSeparatorsStaySeparators(t *testing.T) {
	cases := map[string]string{
		"Widget — Pro":   "widget-pro", // em dash (Pd)
		"Widget « Pro »": "widget-pro", // guillemets (Pi/Pf)
		"Widget © Pro":   "widget-pro", // copyright sign (So)
		"Widget Pro":     "widget-pro", // no-break space (Zs)
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if err != nil {
			t.Errorf("Slugify(%q) errored, want %q: %v", in, want, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSlugifyExistingDeadEndsKeepTheirMessages. Both of these are ASCII, so the
// new refusal must not intercept them — the issue itself praises their messages,
// and stealing them would ALSO make the "names the offending characters" claim
// false (there are none to name).
func TestSlugifyExistingDeadEndsKeepTheirMessages(t *testing.T) {
	cases := map[string]string{
		"123 Numbers": "must start with a letter",
		"!!!":         "need ≥3 chars",
	}
	for in, want := range cases {
		_, err := Slugify(in)
		if err == nil {
			t.Errorf("Slugify(%q) should still fail", in)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Slugify(%q) message changed: got %q, want it to contain %q", in, err, want)
		}
		if strings.Contains(err.Error(), "--slug <slug>") {
			t.Errorf("Slugify(%q) was intercepted by the lossy-rune refusal: %v", in, err)
		}
	}
}

// TestLossyCharsIsDeduplicatedAndOrdered — the message lists characters, so a
// name repeating one must not repeat it in the error.
func TestLossyCharsIsDeduplicatedAndOrdered(t *testing.T) {
	got := LossyChars("ÜÜber Ω Über")
	want := []string{"Ü", "Ω"}
	if len(got) != len(want) {
		t.Fatalf("LossyChars = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LossyChars = %q, want %q (first-appearance order)", got, want)
		}
	}
	if len(LossyChars("My Cool Block")) != 0 {
		t.Error("an ASCII name has no lossy characters")
	}
}

// TestSlugifyRefusalBoundsTheCharacterList — a fully non-Latin name can carry
// dozens of distinct characters; the refusal names the first few and counts the
// rest rather than echoing the whole name back as a quoted list.
func TestSlugifyRefusalBoundsTheCharacterList(t *testing.T) {
	long := "日本語アプリケーションのテスト"
	lossy := LossyChars(long)
	if len(lossy) <= maxNamedChars {
		t.Fatalf("fixture is broken: %q has %d distinct lossy characters, need > %d", long, len(lossy), maxNamedChars)
	}
	_, err := Slugify(long)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	msg := err.Error()
	// The bounded prefix is named …
	for _, c := range lossy[:maxNamedChars] {
		if !strings.Contains(msg, c) {
			t.Errorf("refusal should name %q: %s", c, msg)
		}
	}
	// … and the remainder is COUNTED, not silently dropped.
	if !strings.Contains(msg, fmt.Sprintf("(and %d more)", len(lossy)-maxNamedChars)) {
		t.Errorf("refusal should count the characters it did not spell out: %s", msg)
	}
	// The quoted-list is bounded: exactly maxNamedChars quoted characters.
	if got := strings.Count(msg, `", "`); got != maxNamedChars-1 {
		t.Errorf("expected %d separators in the quoted list, got %d: %s", maxNamedChars-1, got, msg)
	}
}
