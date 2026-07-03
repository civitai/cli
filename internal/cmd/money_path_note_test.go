package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/manifest"
)

// TestManifestNeedsSpend pins the money-path predicate: only a manifest that
// declares an `ai:write*` (Buzz-spend) scope is a money app. Matching is
// case-insensitive and tolerant of surrounding whitespace; a non-spend scope
// (even one containing "write") does NOT count.
func TestManifestNeedsSpend(t *testing.T) {
	cases := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{"budgeted spend", []string{"user:read:self", "ai:write:budgeted"}, true},
		{"bare ai:write", []string{"ai:write"}, true},
		{"upper-case + padding", []string{"  AI:WRITE:BUDGETED  "}, true},
		{"no scopes", nil, false},
		{"read-only", []string{"user:read:self", "identity:read"}, false},
		{"other write scope is not spend", []string{"models:write"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &manifest.Manifest{Scopes: tc.scopes}
			if got := manifestNeedsSpend(m); got != tc.want {
				t.Errorf("manifestNeedsSpend(%v) = %v, want %v", tc.scopes, got, tc.want)
			}
		})
	}
}

// TestPrintMoneyPathNotePrintsForSpendApps: a money app gets the OAuth-can't-spend
// reminder (full-scope personal key needed for real dev:live Buzz spend); a
// non-money app gets nothing (the note stays scoped to money apps).
func TestPrintMoneyPathNotePrintsForSpendApps(t *testing.T) {
	var spend bytes.Buffer
	printMoneyPathNote(&spend, &manifest.Manifest{Scopes: []string{"ai:write:budgeted"}})
	got := spend.String()
	for _, want := range []string{
		"full-scope personal API key",
		"civitai login --token",
		"cannot spend Buzz",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("money-path note should mention %q:\n%s", want, got)
		}
	}

	var noSpend bytes.Buffer
	printMoneyPathNote(&noSpend, &manifest.Manifest{Scopes: []string{"user:read:self"}})
	if noSpend.Len() != 0 {
		t.Errorf("non-money app should print no money-path note, got:\n%s", noSpend.String())
	}
}
