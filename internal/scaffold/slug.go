package scaffold

import (
	"fmt"
	"regexp"
	"strings"
)

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
