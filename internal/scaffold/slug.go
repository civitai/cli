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
//
// 🔴 THE TRUNCATION HERE IS NOT THE ONE #291 REFUSED, AND THE DIFFERENCE IS THE
// INPUT. Slugify turns a NAME into an identity nobody has chosen yet, so cutting
// it silently substitutes a different permanent public id — it refuses instead.
// SuffixSlug is handed a slug the author ALREADY has (its only caller,
// mintDevTokenWithRename in internal/cmd/app_dev_token.go, reads it out of
// block.manifest.json), which the server has just told us is taken; shortening
// the base to make room for `-<suffix>` is the whole point of the operation, the
// author is told which slug replaced which, and the manifest is rewritten so
// nothing is left pointing at the old one. It does NOT call Slugify, so the two
// paths are disjoint: the refusal cannot break this, and this cannot bypass the
// refusal.
func SuffixSlug(original, suffix string) (string, error) {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	if suffix == "" || nonSlugChars.MatchString(suffix) || strings.Contains(suffix, "-") {
		return "", fmt.Errorf("slug suffix %q must be lowercase-alphanumeric", suffix)
	}
	// Reserve room for the hyphen + suffix within the 40-char cap.
	maxBase := maxSlugLen - 1 - len(suffix)
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

// minSlugLen / maxSlugLen are the server's slug length bounds, in ONE place.
// They used to be four literal `40`s across three sites — ValidateSlug (the
// explicit --slug path), SuffixSlug's reservation, and Slugify's truncation
// (twice) — and #291 is what a disagreement between two of them costs: the
// explicit path REFUSED at 41 while the derived path silently cut to 40, so two
// names could mint one un-renameable blockId. A shared constant is what makes
// "both paths cap at the same number" a fact rather than a claim.
const (
	minSlugLen = 3
	maxSlugLen = 40
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// isCombiningMark reports whether r is a Unicode Mark (Mn/Mc/Me) — a character
// that has no standalone appearance and renders on top of the one before it.
func isCombiningMark(r rune) bool { return unicode.Is(unicode.M, r) }

// isLossyBase classifies ONE base rune: true when derivation would DROP it
// rather than fold it into a hyphen.
//
// 🔴 THE CLASSIFICATION IS ON THE LOWERED RUNE AND THE REPORT IS ON THE ORIGINAL
// — the two must not be collapsed. Derivation lowercases first, so lowering is
// what decides whether a rune survives; but a message that quotes the LOWERED
// rune names a character the user never typed. Measured on the first version of
// this code, which classified and reported off `strings.ToLower(name)`:
// `"ẞE App"` reported `"ß"`, a rune ABSENT from the input, and `"ＡＢＣ"`
// reported `"ａ", "ｂ", "ｃ"`. Someone searching their own name for the quoted
// character finds nothing.
//
// 🔴 The rule itself is the ASYMMETRY between the two things a non-`[a-z0-9]`
// rune can be. A SEPARATOR (space, `_`, `.`, `/`, `-`, `!`, `&`, an em dash …)
// carries no information of its own: turning it into a hyphen is what the author
// meant, and that is why `"My Cool Block"` → `my-cool-block` is correct. A
// LETTER, DIGIT or MARK the ASCII slug alphabet cannot carry is the opposite: it
// is content the author typed, and there is no lossless place to put it.
// Dropping it silently mints a DIFFERENT permanent public identity —
// `"ÜberApp Ω"` → `berapp` (the leading `Ü` becomes a hyphen and is then
// trimmed), `"Café Del Mar"` → `caf-del-mar` (a spurious word boundary
// mid-name). Neither is a truncation the author can recognise, and a blockId is
// not renameable, so the caller refuses and asks for an explicit slug instead.
//
// 🔴 EVERY RUNE THAT LOWERS INTO ASCII IS EXEMPT BY CONSTRUCTION, and that is
// load-bearing rather than lazy: it is what makes the ASCII derivations that
// work today byte-identical, including the two dead ends whose existing messages
// are good (`"123 Numbers"` → starts with a digit; `"!!!"` → nothing left). It
// is also the source of the (b) exception in the Slugify header: exactly two
// runes above ASCII lower INTO ASCII. Do not "improve" the exemption into a
// character allowlist — an allowlist re-decides every ASCII derivation this CLI
// has ever produced, and the whole point of the boundary is that it decides
// none of them.
func isLossyBase(r rune) bool {
	lower := unicode.ToLower(r)
	if lower < utf8.RuneSelf {
		// Lowers into ASCII: either a slug character or a separator.
		// Byte-identical to the derivation this CLI has always done.
		return false
	}
	// A separator with no content of its own — becomes a hyphen.
	return !unicode.IsSpace(lower) && !unicode.IsPunct(lower) && !unicode.IsSymbol(lower)
}

// dottedCircle is the Unicode convention for displaying a combining mark that
// has no base character to sit on (U+25CC DOTTED CIRCLE).
const dottedCircle = "◌"

// LossyChars returns, in first-appearance order and deduplicated, the characters
// in name that slug derivation would DROP rather than fold into a hyphen — each
// as the printable string the AUTHOR typed, never a lowered or decomposed
// substitute.
//
// 🔴 A COMBINING MARK IS REPORTED WITH ITS BASE, because on its own it renders
// as an accent floating over nothing. macOS paths and some paste routes deliver
// NFD, so `"Café App"` arrives as `e` + U+0301 and the first version of this
// message read `"́" cannot appear in a blockId` — the same VISIBLE name yielding
// two different messages, one of them pointing at nothing the user can see. The
// cluster is what gets quoted, so NFC and NFD both report `"é"`. This changes
// only the RENDERING: a mark is non-ASCII and is neither space, punct nor
// symbol, so it was already refused, and the set of refused names is unchanged.
// A mark with no base at all (a name STARTING with one) is shown on a dotted
// circle rather than bare.
//
// Transliteration was considered and rejected: `Ω → o` vs `omega` is a
// locale-dependent judgement, a general transliterator means a new
// `golang.org/x/text` dependency, and it produces nothing usable for CJK,
// Cyrillic or Arabic — which is most of the population this refusal serves.
//
// name MUST be valid UTF-8; Slugify gates that before calling here. Ranging over
// invalid bytes yields U+FFFD, which is a Symbol and would be waved through as a
// separator.
func LossyChars(name string) []string {
	runes := []rune(strings.TrimSpace(name))
	var lossy []string
	seen := map[string]bool{}
	add := func(s string) {
		if seen[s] {
			return
		}
		seen[s] = true
		lossy = append(lossy, s)
	}
	for i := 0; i < len(runes); {
		base := runes[i]
		j := i + 1
		for j < len(runes) && isCombiningMark(runes[j]) {
			j++
		}
		switch {
		case isCombiningMark(base):
			// Only reachable at the very start of the name: every other mark is
			// consumed by the base before it.
			add(dottedCircle + string(runes[i:j]))
		case j > i+1 || isLossyBase(base):
			// Either the cluster carries a mark (content, dropped by
			// derivation) or the base itself is lossy. Report the cluster.
			add(string(runes[i:j]))
		}
		i = j
	}
	return lossy
}

// maxNamedChars bounds how many offending characters the refusal spells out. A
// fully non-Latin name can carry dozens of distinct characters and an error that
// re-prints the whole name back as a quoted list stops being readable; the first
// few are enough to make the refusal recognisable, and the name itself is quoted
// in the same sentence.
const maxNamedChars = 6

// quoteChars renders characters as a comma-separated list of quoted characters,
// for an error that has to NAME what it is refusing.
func quoteChars(cs []string) string {
	more := ""
	if len(cs) > maxNamedChars {
		more = fmt.Sprintf(" (and %d more)", len(cs)-maxNamedChars)
		cs = cs[:maxNamedChars]
	}
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = fmt.Sprintf("%q", c)
	}
	return strings.Join(parts, ", ") + more
}

// Slugify converts an arbitrary name into a valid block slug, or returns an
// error if it cannot produce one within the length bounds — or if deriving one
// would silently DISCARD characters the author typed (see LossyChars).
//
// 🔴 THIS NARROWS ISSUE #259, IT DOES NOT CLOSE IT. FOUR classes of input have
// derived a slug that quietly differs from the name. Two are now CLOSED by a
// refusal — (a) and (d) — and two still derive; all four are stated here rather
// than implied away, because a residual nobody writes down is indistinguishable
// from a bug nobody noticed.
//
// 🔴 THIS LIST SAID "THREE" AND OMITTED (d) FROM THE DAY IT WAS WRITTEN, AND THE
// OMISSION WAS FILED AS A DEFECT IN ITS OWN RIGHT (#291) — separately from the
// truncation it hid, precisely because an enumeration that presents itself as
// complete and is not costs a reader more than no enumeration at all: it is the
// artefact they use to decide they have finished looking. When you close a class
// here, or find a new one, re-count the sentence above; "all N are stated here"
// is a claim, and it is the first thing that goes stale.
//
//   - (a) INVALID UTF-8 — CLOSED. `caf\xe9 app` used to derive `caf-app` with
//     rc 0, because `for _, r := range` yields U+FFFD per bad byte and U+FFFD is
//     a Symbol, i.e. the separator branch. Worse, the manifest the scaffold then
//     wrote held the raw `0xE9` and was not valid UTF-8 — and `validate` accepted
//     it. The `utf8.ValidString` guard below refuses it. (The manifest half is
//     closed at the call site, which also refuses an invalid-UTF-8 DISPLAY name;
//     `internal/validate` still has no UTF-8 check of its own.)
//   - (b) THE TWO RUNES ABOVE ASCII THAT LOWER INTO ASCII — a deliberate,
//     ENUMERATED exception, not an oversight. All of Unicode was walked: exactly
//     two exist, `İ` U+0130 → `i` and `K` U+212A KELVIN SIGN → `k`. So
//     `"İstanbul App"` derives `istanbul-app` with rc 0. That is arguably the
//     nicest transliteration available and it costs nothing, so it stays — but
//     it means the exemption below is "lowers into ASCII", never "is ASCII",
//     and any absolute claim that every non-ASCII letter is refused is false.
//   - (c) SYMBOLS, EMOJI AND NON-ASCII PUNCTUATION fold to a hyphen, so
//     `"Rocket 🚀 App"` derives `rocket-app` with rc 0. Census over printable
//     non-ASCII runes: 8,580 take the separator branch, 140,321 the refuse
//     branch. This is the asymmetry working as designed for `—` and `»`, and
//     arguably NOT what an author means by an emoji — but an emoji has no
//     lossless ASCII form either, so refusing would only trade a silent drop for
//     a dead end. Left as-is deliberately, tracked separately.
//   - (d) A NAME WHOSE SLUG RUNS PAST 40 CHARACTERS — CLOSED (#291). Derivation
//     used to CUT it (`s = strings.Trim(s[:40], "-")`) at rc 0. Measured:
//     `"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee"` and the same
//     name ending `ZZZZZZZZZZ` — 54 characters each, differing from character 44
//     — both minted `aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd`. Truncation alone
//     is recoverable; two apps competing for ONE un-renameable public id is not,
//     and nothing local tells the author it happened. An EXPLICIT `--slug` of the
//     same length was already refused ("must be 3-40 chars"), so the derived path
//     — the one a first-time author walks — was the inconsistent one. It refuses
//     now too, with the same `--slug` escape hatch as (a) and the lossy-character
//     case. Exactly 40 is AT the cap, not over it, and still derives
//     byte-identically.
func Slugify(name string) (string, error) {
	// 🔴 BEFORE anything ranges over the string. `for _, r := range` silently
	// substitutes U+FFFD for each invalid byte, and U+FFFD classifies as a
	// Symbol — the separator branch — so a mojibake name derived a slug that
	// dropped the bad bytes AND wrote them raw into block.manifest.json.
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("cannot derive a slug from %q: the name is not valid UTF-8, so the characters it holds cannot be read (a blockId is lowercase a-z, 0-9 and hyphens only), and deriving one anyway would give your app a different permanent public id than you typed — choose one yourself with --slug <slug>", name)
	}
	if lossy := LossyChars(name); len(lossy) > 0 {
		return "", fmt.Errorf("cannot derive a slug from %q: %s cannot appear in a blockId (lowercase a-z, 0-9 and hyphens only), and dropping them would give your app a different permanent public id than you typed — choose one yourself with --slug <slug>", name, quoteChars(lossy))
	}
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlugChars.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	// Collapse repeated hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) < minSlugLen {
		return "", fmt.Errorf("cannot derive a valid slug from %q (need ≥%d chars; use lowercase letters/numbers/hyphens)", name, minSlugLen)
	}
	// 🔴 REFUSE, DO NOT TRUNCATE — residual (d) above. This is the SAME cap
	// ValidateSlug applies to an explicit --slug, deliberately reached through
	// the same constant, so the two paths cannot drift apart again.
	if len(s) > maxSlugLen {
		return "", fmt.Errorf("cannot derive a slug from %q: it derives the %d-character blockId %q and the limit is %d, and cutting it to fit would give your app a different permanent public id than you typed — one that every other name sharing those first %d characters would be given too — so choose one yourself with --slug <slug>", name, len(s), s, maxSlugLen, maxSlugLen)
	}
	if !slugPattern.MatchString(s) {
		return "", fmt.Errorf("derived slug %q is invalid (must start with a letter, be lowercase, hyphen-separated)", s)
	}
	return s, nil
}

// ValidateSlug checks an explicit slug against the server contract.
func ValidateSlug(slug string) error {
	if len(slug) < minSlugLen || len(slug) > maxSlugLen {
		return fmt.Errorf("slug %q must be %d-%d chars", slug, minSlugLen, maxSlugLen)
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
