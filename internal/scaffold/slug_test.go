package scaffold

import (
	"strings"
	"testing"
)

func TestTitleFromSlug(t *testing.T) {
	cases := map[string]string{
		"my-cool-block": "My Cool Block",
		"notepad":       "Notepad",
		"a-b-c":         "A B C",
	}
	for in, want := range cases {
		if got := TitleFromSlug(in); got != want {
			t.Errorf("TitleFromSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyTooLongTrims(t *testing.T) {
	long := "this-is-a-really-long-name-that-exceeds-the-forty-char-limit"
	got, err := Slugify(long)
	if err != nil {
		t.Fatalf("Slugify: %v", err)
	}
	if len(got) > 40 {
		t.Errorf("slug %q exceeds 40 chars (%d)", got, len(got))
	}
	if err := ValidateSlug(got); err != nil {
		t.Errorf("trimmed slug %q should be valid: %v", got, err)
	}
}

func TestSlugifyAllPunctuationFails(t *testing.T) {
	if _, err := Slugify("!!!"); err == nil {
		t.Error("expected error slugifying pure punctuation")
	}
}

func TestSlugifyAllTemplates(t *testing.T) {
	ts := AllTemplates()
	if len(ts) != 3 {
		t.Errorf("AllTemplates = %v, want 3", ts)
	}
}

func TestRandomSuffix(t *testing.T) {
	for _, n := range []int{1, 5, 12} {
		s := RandomSuffix(n)
		if len(s) != n {
			t.Errorf("RandomSuffix(%d) len = %d", n, len(s))
		}
		for _, r := range s {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
				t.Errorf("RandomSuffix(%d)=%q has non-alphanumeric %q", n, s, r)
			}
		}
	}
	// n<=0 falls back to a non-empty default.
	if got := RandomSuffix(0); len(got) == 0 {
		t.Error("RandomSuffix(0) should not be empty")
	}
}

func TestSuffixSlug(t *testing.T) {
	got, err := SuffixSlug("my-block", "abc12")
	if err != nil {
		t.Fatalf("SuffixSlug: %v", err)
	}
	if got != "my-block-abc12" {
		t.Errorf("SuffixSlug = %q, want my-block-abc12", got)
	}
	if err := ValidateSlug(got); err != nil {
		t.Errorf("result should be a valid slug: %v", err)
	}

	// A long original is truncated so the result stays within 40 chars.
	long, err := SuffixSlug(strings.Repeat("a", 60), "abc12")
	if err != nil {
		t.Fatalf("SuffixSlug(long): %v", err)
	}
	if len(long) > 40 {
		t.Errorf("SuffixSlug(long) = %q exceeds 40 chars (%d)", long, len(long))
	}
	if err := ValidateSlug(long); err != nil {
		t.Errorf("truncated result should be valid: %v", err)
	}

	// Empty / hyphenated / non-alphanumeric suffixes are rejected (uppercase is
	// normalized to lowercase, not rejected).
	for _, bad := range []string{"", "a-b", "a!b"} {
		if _, err := SuffixSlug("my-block", bad); err == nil {
			t.Errorf("SuffixSlug with suffix %q should error", bad)
		}
	}
	// Uppercase is normalized.
	if got, err := SuffixSlug("my-block", "ABCDE"); err != nil || got != "my-block-abcde" {
		t.Errorf("SuffixSlug normalize uppercase = %q, %v", got, err)
	}
}
