package validate

import "strings"

// jsregexp.go is a CONSERVATIVE syntax check for the language the server
// compiles a collection schema's `pattern` in: an ECMAScript regular expression
// with the `u` flag (`new RegExp(p, "u")`, no `v`).
//
// It reports only constructs that are a SyntaxError in that language — never
// anything valid there — so a report is a WARNING with a reason, and silence
// is NOT a claim the server accepts the pattern. What it knows:
//
//   - outside [...], an escape of a character as itself is allowed only for
//     the syntax characters ^ $ \ . * + ? ( ) [ ] { } | and /; inside [...],
//     also - (so \- \_ \: \# \@ \z \Q are refused, [\-] is not);
//   - the letter escapes "u" mode defines: \d \D \w \W \s \S, \b \B (\B not
//     inside [...]), \f \n \r \t \v, \cX (X a letter), \0 (not before a
//     digit), \xHH, \uHHHH and \u{...} (at most 10FFFF), \p{...}/\P{...}, and
//     \k<name> / \1… backreferences outside [...] (a digit escape inside [...]
//     is refused);
//   - a { that does not open a {n}, {n,} or {n,m} quantifier, and a lone } or
//     ] (the "u" flag drops the web-compat reading of them as characters);
//   - an inline flag group (?i) / (?s) and Python's (?P<…>);
//   - unbalanced ( ) and an unterminated [ or trailing \.
//
// What it does NOT check, each left to the server: a backreference to a group
// that does not exist, a named group's name, a \p property name, a quantifier
// with nothing to repeat or its bounds out of order, a class range out of
// order or with a class escape at either end, and other (? forms. Nor ReDoS.

const (
	jsIdentityTail = ` (with "u", only ^ $ \ . * + ? ( ) [ ] { } | / may be escaped as themselves, and - only inside [...])`
	jsClassTail    = ` inside [...] (with "u", only ^ $ \ . * + ? ( ) [ ] { } | / - may be escaped as themselves there)`
)

// jsUModePatternError returns why `new RegExp(src, "u")` throws, or "" when
// this check finds nothing (see the file comment: not proof it compiles).
func jsUModePatternError(src string) string {
	p := []rune(src)
	depth := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			n, reason := jsEscape(p, i, false)
			if reason != "" {
				return reason
			}
			i += n - 1
		case '[':
			n, reason := jsClass(p, i)
			if reason != "" {
				return reason
			}
			i += n - 1
		case '(':
			if i+1 < len(p) && p[i+1] == '?' {
				rest := string(p[i+2:])
				if strings.HasPrefix(rest, "P") {
					return `the Python-style named group (?P<…>) (JavaScript writes (?<name>…))`
				}
				j := i + 2
				for j < len(p) && (p[j] == '-' || isASCIILetter(p[j])) {
					j++
				}
				if j < len(p) && p[j] == ')' {
					return `the inline flag group ` + string(p[i:j+1])
				}
			}
			depth++
		case ')':
			if depth == 0 {
				return `a ) with no matching (`
			}
			depth--
		case '{':
			n := jsQuantifierLen(p, i)
			if n == 0 {
				return `a lone { (write \{ for the character itself)`
			}
			i += n - 1
		case '}':
			return `a lone } (write \} for the character itself)`
		case ']':
			return `a lone ] (write \] for the character itself)`
		}
	}
	if depth > 0 {
		return `a ( with no matching )`
	}
	return ""
}

// jsClass checks the [...] opening at p[i]; it returns the class's length in runes.
func jsClass(p []rune, i int) (int, string) {
	for j := i + 1; j < len(p); j++ {
		switch p[j] {
		case ']':
			return j - i + 1, ""
		case '\\':
			n, reason := jsEscape(p, j, true)
			if reason != "" {
				return 0, reason
			}
			j += n - 1
		}
	}
	return 0, `a [ with no matching ]`
}

// jsEscape checks the escape starting at the backslash p[i]; it returns the
// escape's length in runes.
func jsEscape(p []rune, i int, inClass bool) (int, string) {
	if i+1 >= len(p) {
		return 0, `a trailing \`
	}
	c := p[i+1]
	switch {
	case strings.ContainsRune(`^$\.*+?()[]{}|/`, c), inClass && c == '-':
		return 2, ""
	case strings.ContainsRune("dDwWsSfnrtv", c), c == 'b', c == 'B' && !inClass:
		return 2, ""
	case c == 'c':
		if i+2 < len(p) && isASCIILetter(p[i+2]) {
			return 3, ""
		}
		return 0, `\c not followed by a letter`
	case c == '0':
		if i+2 < len(p) && isASCIIDigit(p[i+2]) {
			return 0, `\0 followed by a digit`
		}
		return 2, ""
	case c >= '1' && c <= '9' && !inClass:
		n := 2
		for i+n < len(p) && isASCIIDigit(p[i+n]) {
			n++
		}
		return n, ""
	case c == 'x':
		if i+3 < len(p) && isHex(p[i+2]) && isHex(p[i+3]) {
			return 4, ""
		}
		return 0, `\x not followed by two hex digits`
	case c == 'u':
		if n := jsUnicodeEscapeLen(p, i); n > 0 {
			return n, ""
		}
		return 0, `\u not followed by four hex digits or a {…} code point`
	case c == 'p' || c == 'P':
		if i+2 < len(p) && p[i+2] == '{' {
			// The property name is the server's to judge; only the braces are checked here.
			for j := i + 3; j < len(p); j++ {
				if p[j] == '}' {
					return j - i + 1, ""
				}
			}
			return 0, `\` + string(c) + `{ with no closing }`
		}
		return 0, `\` + string(c) + ` not followed by {…} (a Unicode property is written \` + string(c) + `{L})`
	case c == 'k' && !inClass:
		if i+2 < len(p) && p[i+2] == '<' {
			return 3, "" // the group name is the server's to judge
		}
		return 0, `\k not followed by <name>`
	}
	if inClass {
		return 0, `the escape \` + string(c) + jsClassTail
	}
	return 0, `the escape \` + string(c) + jsIdentityTail
}

// jsUnicodeEscapeLen is the length of \uHHHH or \u{H…} (≤ 10FFFF) at p[i], or 0.
func jsUnicodeEscapeLen(p []rune, i int) int {
	if i+2 < len(p) && p[i+2] == '{' {
		v := 0
		j := i + 3
		for j < len(p) && isHex(p[j]) {
			v = v*16 + hexVal(p[j])
			if v > 0x10FFFF {
				return 0
			}
			j++
		}
		if j == i+3 || j >= len(p) || p[j] != '}' {
			return 0
		}
		return j - i + 1
	}
	for k := 2; k < 6; k++ {
		if i+k >= len(p) || !isHex(p[i+k]) {
			return 0
		}
	}
	return 6
}

// jsQuantifierLen is the length of a {n}, {n,} or {n,m} quantifier at p[i], or 0.
func jsQuantifierLen(p []rune, i int) int {
	j := i + 1
	start := j
	for j < len(p) && isASCIIDigit(p[j]) {
		j++
	}
	if j == start {
		return 0
	}
	if j < len(p) && p[j] == ',' {
		j++
		for j < len(p) && isASCIIDigit(p[j]) {
			j++
		}
	}
	if j >= len(p) || p[j] != '}' {
		return 0
	}
	return j - i + 1
}

func isASCIILetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
func isASCIIDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isHex(r rune) bool         { return isASCIIDigit(r) || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' }

func hexVal(r rune) int {
	switch {
	case isASCIIDigit(r):
		return int(r - '0')
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10
	default:
		return int(r-'A') + 10
	}
}
