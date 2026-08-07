package scaffold

import (
	"fmt"
	"strings"
	"testing"
)

// TestSlugifyRefusesRatherThanDroppingCharacters is the headline of issue #259.
//
// 🔴 EACH ROW PINS THE OLD OUTPUT IT MUST NOT PRODUCE, not merely "an error".
// "an error came back" is satisfied by a refusal for the wrong reason and by a
// refusal that happens to fire everywhere; naming `berapp` / `caf-del-mar`
// is what makes the row evidence about THIS defect.
func TestSlugifyRefusesRatherThanDroppingCharacters(t *testing.T) {
	cases := []struct {
		name string
		// mustNotProduce is what Slugify returned BEFORE the fix — a silently
		// different permanent public identity.
		mustNotProduce string
		// names are the characters the refusal has to point at.
		names []string
	}{
		{
			// The leading Ü becomes a hyphen and is then trimmed, so the app's
			// public id loses its first letter outright.
			name:           "ÜberApp Ω",
			mustNotProduce: "berapp",
			names:          []string{"ü", "ω"},
		},
		{
			// Worse than dropping: mid-string it INSERTS a word boundary that
			// the author never typed.
			name:           "Café Del Mar",
			mustNotProduce: "caf-del-mar",
			names:          []string{"é"},
		},
		{
			name:           "日本語アプリ",
			mustNotProduce: "",
			names:          []string{"日", "本", "語"},
		},
		{
			name:           "Приложение",
			mustNotProduce: "",
			names:          []string{"п", "р"},
		},
		{
			name:           "تطبيق",
			mustNotProduce: "",
			names:          []string{"ت", "ط"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Slugify(tc.name)
			if err == nil {
				t.Fatalf("Slugify(%q) = %q, want a refusal", tc.name, got)
			}
			if got != "" {
				t.Errorf("a refused Slugify must return no slug, got %q", got)
			}
			// The pre-fix output must be unreachable, not merely unlikely.
			if tc.mustNotProduce != "" {
				if s, e := Slugify(tc.name); e == nil && s == tc.mustNotProduce {
					t.Errorf("Slugify(%q) still produces the mangled %q", tc.name, tc.mustNotProduce)
				}
			}
			msg := err.Error()
			for _, r := range tc.names {
				if !strings.Contains(msg, r) {
					t.Errorf("refusal must NAME the offending character %q: %s", r, msg)
				}
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

// TestSlugifyAsciiDerivationIsByteIdentical is the CONTROL. These rows were
// green before the refusal existed and must stay green: the predicate exempts
// every ASCII rune by construction, so ordinary punctuation and multiple spaces
// still fold to a single hyphen. An invariant guard, not regression coverage —
// its job is to fail loudly if the refusal ever widens.
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

// TestSlugifyNonAsciiSeparatorsStaySeparators: above ASCII the classification is
// by Unicode category, so a punctuation or symbol rune is still a separator with
// no content of its own. Getting this wrong would refuse names that derive
// perfectly well — the false-refusal direction.
func TestSlugifyNonAsciiSeparatorsStaySeparators(t *testing.T) {
	cases := map[string]string{
		"Widget — Pro":   "widget-pro", // em dash (Pd)
		"Widget « Pro »": "widget-pro", // guillemets (Pi/Pf)
		"Widget © Pro":   "widget-pro", // copyright sign (So)
		"Widget Pro":     "widget-pro", // no-break space (Zs)
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

// TestLossyRunesIsDeduplicatedAndOrdered — the message lists characters, so a
// name repeating one must not repeat it in the error.
func TestLossyRunesIsDeduplicatedAndOrdered(t *testing.T) {
	got := LossyRunes("ÜÜber Ω Über")
	want := []rune{'ü', 'ω'}
	if len(got) != len(want) {
		t.Fatalf("LossyRunes = %q, want %q", string(got), string(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LossyRunes = %q, want %q (first-appearance order)", string(got), string(want))
		}
	}
	if len(LossyRunes("My Cool Block")) != 0 {
		t.Error("an ASCII name has no lossy runes")
	}
}

// TestSlugifyRefusalBoundsTheCharacterList — a fully non-Latin name can carry
// dozens of distinct runes; the refusal names the first few and counts the rest
// rather than echoing the whole name back as a quoted list.
func TestSlugifyRefusalBoundsTheCharacterList(t *testing.T) {
	long := "日本語アプリケーションのテスト"
	lossy := LossyRunes(long)
	if len(lossy) <= maxNamedRunes {
		t.Fatalf("fixture is broken: %q has %d distinct lossy runes, need > %d", long, len(lossy), maxNamedRunes)
	}
	_, err := Slugify(long)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	msg := err.Error()
	// The bounded prefix is named …
	for _, r := range lossy[:maxNamedRunes] {
		if !strings.Contains(msg, string(r)) {
			t.Errorf("refusal should name %q: %s", string(r), msg)
		}
	}
	// … and the remainder is COUNTED, not silently dropped.
	if !strings.Contains(msg, fmt.Sprintf("(and %d more)", len(lossy)-maxNamedRunes)) {
		t.Errorf("refusal should count the runes it did not spell out: %s", msg)
	}
	// The quoted-list is bounded: exactly maxNamedRunes quoted single characters.
	if got := strings.Count(msg, `", "`); got != maxNamedRunes-1 {
		t.Errorf("expected %d separators in the quoted list, got %d: %s", maxNamedRunes-1, got, msg)
	}
}
