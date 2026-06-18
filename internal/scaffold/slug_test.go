package scaffold

import "testing"

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
