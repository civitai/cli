package scaffold

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
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

// LossyRunes returns, in first-appearance order and deduplicated, the runes in
// name that slug derivation would DROP rather than fold into a hyphen.
//
// 🔴 The point is the ASYMMETRY between the two things a non-`[a-z0-9]` rune can
// be. A SEPARATOR (space, `_`, `.`, `/`, `-`, `!`, `&`, an em dash …) carries no
// information of its own: turning it into a hyphen is what the author meant, and
// that is why `"My Cool Block"` → `my-cool-block` is correct. A LETTER, DIGIT or
// MARK the ASCII slug alphabet cannot carry is the opposite: it is content the
// author typed, and there is no lossless place to put it. Dropping it silently
// mints a DIFFERENT permanent public identity — `"ÜberApp Ω"` → `berapp` (the
// leading `Ü` becomes a hyphen and is then trimmed), `"Café Del Mar"` →
// `caf-del-mar` (a spurious word boundary mid-name). Neither is a truncation the
// author can recognise, and a blockId is not renameable, so the caller refuses
// and asks for an explicit slug instead.
//
// Transliteration was considered and rejected: `Ω → o` vs `omega` is a
// locale-dependent judgement, a general transliterator means a new
// `golang.org/x/text` dependency, and it produces nothing usable for CJK,
// Cyrillic or Arabic — which is most of the population this refusal serves.
//
// 🔴 EVERY ASCII RUNE IS EXEMPT BY CONSTRUCTION, and that is load-bearing rather
// than lazy: it is what makes the ASCII derivations that work today
// byte-identical, including the two dead ends whose existing messages are good
// (`"123 Numbers"` → starts with a digit; `"!!!"` → nothing left). Above ASCII
// the classification is by Unicode category, so an em dash or a `»` is still an
// ordinary separator.
func LossyRunes(name string) []rune {
	var lossy []rune
	seen := map[rune]bool{}
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r < utf8.RuneSelf {
			// ASCII: either a slug character or a separator. Byte-identical to
			// the derivation this CLI has always done.
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			// A separator with no content of its own — becomes a hyphen.
			continue
		}
		if seen[r] {
			continue
		}
		seen[r] = true
		lossy = append(lossy, r)
	}
	return lossy
}

// maxNamedRunes bounds how many offending characters the refusal spells out. A
// fully non-Latin name can carry dozens of distinct runes and an error that
// re-prints the whole name back as a quoted list stops being readable; the first
// few are enough to make the refusal recognisable, and the name itself is quoted
// in the same sentence.
const maxNamedRunes = 6

// quoteRunes renders runes as a comma-separated list of quoted characters, for
// an error that has to NAME what it is refusing.
func quoteRunes(rs []rune) string {
	more := ""
	if len(rs) > maxNamedRunes {
		more = fmt.Sprintf(" (and %d more)", len(rs)-maxNamedRunes)
		rs = rs[:maxNamedRunes]
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("%q", string(r))
	}
	return strings.Join(parts, ", ") + more
}

// Slugify converts an arbitrary name into a valid block slug, or returns an
// error if it cannot produce one within the length bounds — or if deriving one
// would silently DISCARD characters the author typed (see LossyRunes).
func Slugify(name string) (string, error) {
	if lossy := LossyRunes(name); len(lossy) > 0 {
		return "", fmt.Errorf("cannot derive a slug from %q: %s cannot appear in a blockId (lowercase a-z, 0-9 and hyphens only), and dropping them would give your app a different permanent public id than you typed — choose one yourself with --slug <slug>", name, quoteRunes(lossy))
	}
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
