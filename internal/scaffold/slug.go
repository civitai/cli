package scaffold

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
)

// slugSuffixAlphabet is lowercase-alphanumeric only (no hyphens) so a generated
// suffix is always safe to append and to end a slug on.
const slugSuffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// RandomSuffix returns a lowercase-alphanumeric string of length n, suitable as
// a slug suffix (contains no hyphens, so it can safely start/end a slug). It uses
// crypto/rand; on the (near-impossible) read error it falls back to a fixed pad
// so callers never get an empty suffix.
func RandomSuffix(n int) string {
	if n <= 0 {
		n = 5
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("x", n)
	}
	out := make([]byte, n)
	for i, c := range b {
		out[i] = slugSuffixAlphabet[int(c)%len(slugSuffixAlphabet)]
	}
	return string(out)
}

// SuffixSlug returns "<original>-<suffix>", truncating original if needed so the
// result stays within the 3-40 char slug bounds, and validates it against the
// server slug contract. suffix must be lowercase-alphanumeric (as RandomSuffix
// produces). It is used to auto-rename a slug that collides with an app owned by
// another account.
func SuffixSlug(original, suffix string) (string, error) {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	if suffix == "" || nonSlugChars.MatchString(suffix) || strings.Contains(suffix, "-") {
		return "", fmt.Errorf("slug suffix %q must be lowercase-alphanumeric", suffix)
	}
	// Reserve room for the hyphen + suffix within the 40-char cap.
	maxBase := 40 - 1 - len(suffix)
	if maxBase < 1 {
		return "", fmt.Errorf("slug suffix %q is too long", suffix)
	}
	base := strings.ToLower(strings.TrimSpace(original))
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	base = strings.Trim(base, "-")
	if base == "" {
		return "", fmt.Errorf("cannot derive a base slug from %q", original)
	}
	candidate := base + "-" + suffix
	if err := ValidateSlug(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

// slugPattern mirrors the server SLUG_REGEX: starts with a letter, lowercase,
// hyphen-separated, ends alphanumeric, 3-40 chars.
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts an arbitrary name into a valid block slug, or returns an
// error if it cannot produce one within the length bounds.
func Slugify(name string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlugChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	// Collapse repeated hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) < 3 {
		return "", fmt.Errorf("cannot derive a valid slug from %q (need ≥3 chars; use lowercase letters/numbers/hyphens)", name)
	}
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if !slugPattern.MatchString(s) {
		return "", fmt.Errorf("derived slug %q is invalid (must start with a letter, be lowercase, hyphen-separated)", s)
	}
	return s, nil
}

// ValidateSlug checks an explicit slug against the server contract.
func ValidateSlug(slug string) error {
	if len(slug) < 3 || len(slug) > 40 {
		return fmt.Errorf("slug %q must be 3-40 chars", slug)
	}
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug %q must start with a letter, be lowercase, and contain only letters, numbers, and hyphens", slug)
	}
	return nil
}

// TitleFromSlug produces a human-readable display name from a slug.
func TitleFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
