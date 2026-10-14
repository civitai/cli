// Package redact removes what `civitai feedback send` must not carry off the
// machine, BEFORE anything is sent: the text a developer (or their coding agent)
// pastes into a friction report is an error message, a path, a log excerpt —
// exactly the material that holds a home directory, an e-mail address or a live
// credential.
//
// # It runs before the request is built, and it reports what it did
//
// The caller applies one Redactor to the message and to the error excerpt and
// then prints Counts(), so the user can see that something was removed and of which kind.
// A count names a KIND and a number, not the text removed, and it counts what
// was removed from the text that is RETURNED: ApplyClipped does not count a
// redaction that fell beyond its cut.
//
// Input must already be UTF-8 text. UTF8Text (text.go) is the read boundary
// for file and pipe content: it accepts valid UTF-8 with no NUL and refuses
// everything else. The rules below are byte-oriented matchers over UTF-8.
//
// # The rules, in the order they run
//
//  1. URLs, on the ORIGINAL text. A URL is one token: from `scheme://` to the
//     next white space. Two cuts are made on it — everything between `://` and
//     its LAST `@` (kind credential), and everything from its first `?` or `#`
//     to its end (kind url-query, which therefore covers a fragment too). See
//     urls for the one case where the rest of the token goes as well.
//  2. credential  — the value of an `Authorization` header, whatever the
//     scheme: from the first non-blank character after the `:` or `=` — on
//     that line or a later one — to the end of the line that character is on.
//  3. token       — each configured credential value, matched EXACTLY.
//  4. credential  — a PEM private-key block; an `x-api-key` / `api-key` value
//     that has a digit; a bare `Bearer <value>` that has a digit; and what
//     internal/credscan's detectors locate (credscan.Spans): known token
//     formats and the value of a strict `KEY=value` assignment that passes its
//     entropy gate.
//  5. email       — an e-mail address.
//  6. home-path   — this machine's home directory, replaced by `~`.
//  7. user-path   — the user segment of `/home/<u>`, `/Users/<u>` and
//     `C:\Users\<u>`.
//
// 🔴 THE ORDER IS LOAD-BEARING, AND URLS GO FIRST FOR A MEASURED REASON. The
// later rules leave placeholders containing `<` and `>`. When the query strip
// ran after them, `?token=<configured token>&sig=…` became
// `?token=<token>&sig=…`, the strip stopped at the `<`, and `&sig=…` was sent
// while the count said the query had been dropped. A URL is therefore taken
// apart while it is still the text the user wrote.
//
// 🔴 THE URL RULE NAMES NO CHARACTER AT WHICH A QUERY OR A USERINFO "ENDS", AND
// THAT IS ITS SECOND REWRITE. The first fix above still carried a terminator
// class per part (a quote ends a query, the first `@` ends a userinfo, `/` and
// `?` cannot be in one). Each was defeated by that very character:
// `?name='x'&sig=…` kept `&sig=…`; `user:p@ss@host` kept `ss`; `user:a?b@host`
// kept `user:a`. White space is the only boundary now, and the price is in
// KNOWN OVER-REDACTIONS below.
//
// Two consequences for the counts, stated: a token that sits inside a URL's
// query or userinfo, or in an `Authorization` header, is counted under the rule
// that removed it (url-query / credential), not as a `token`; and a URL that
// loses both its userinfo and its tail is two redactions.
//
// # 🔴 What it does NOT do
//
// It is not a guarantee that no secret leaves. The exact-token rule matches a
// known string; the others match SHAPES, and rule 4 inherits the deliberate
// silences of internal/credscan (a placeholder-looking or low-entropy value, a
// Google API key shape, a headerless key body — see that package's doc). That
// is why the command also shows the exact payload under --dry-run, and why its
// output does not describe the text as clean.
//
// The two lists below are kept identical, entry for entry, in this comment, in
// claudedocs/decisions/41-platform-feedback.md and in redact_lists_test.go,
// where each entry is demonstrated by an input.
//
// KNOWN MISSES — sent as typed:
//
//   - a bare `Bearer <value>` with no `Authorization` header in front of it,
//     when the value has no digit
//   - an `x-api-key` / `api-key` value that has no digit
//   - the query or fragment of a URL written without a scheme
//     (`cdn.example/f?sig=…`)
//   - a credential that appears transformed: URL-encoded, base64 of itself, or
//     split across lines
//   - a configured credential shorter than 8 characters (stderr says when one
//     was skipped)
//   - a URL query that contains a literal space: the URL token ends at the
//     space, so `?q=hello world&sig=…` keeps `world&sig=…`
//   - a JSON-escaped URL (`https:\/\/…?sig=…`): it is not recognised as a URL,
//     so its query is kept
//
// KNOWN OVER-REDACTIONS — removed although not a secret. Each is in the safe
// direction and none gets a rule of its own:
//
//   - everything after `Authorization:` to the end of that line, whatever it is
//     — prose included, and anything else that shares the line
//   - the line after an `Authorization` header whose own line holds no value —
//     the value is looked for across line breaks
//   - an `api-key` / `apiKey` assignment whose value has a digit, even when the
//     value is a variable name
//   - any URL userinfo, even a bare user name (`ssh://git@host/…` is counted as
//     a credential)
//   - everything between `://` and the last `@` of a URL token — so an `@` in a
//     path or query takes the host with it — and, when a `?` or `#` came before
//     that `@`, the rest of the token too
//   - a URL's `#fragment`, and whatever is glued to a URL after its first `?`
//     or `#` with no white space: closing quotes, brackets, a trailing full
//     stop, the rest of a minified JSON line
//   - a Windows user segment runs to the next backslash on the line, so
//     `copy C:\Users\ingrid .\dist\out` loses `ingrid .`
//   - `/home/<name>` and `/Users/<name>` wherever they appear, including inside
//     a URL path
//   - the words of a private-key header (`BEGIN … PRIVATE KEY`) in prose
//
// It also does not look for instructions aimed at whoever reads the report.
// There is no "prompt-injection detector" here and one must not be added: it
// would be a phrase list, and AGENTS.md item 28 records how those lose.
package redact

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/civitai/cli/internal/credscan"
)

// The kinds a Count can carry. They are part of `civitai feedback send`'s
// --json contract (pinned by its golden files), so renaming one is a breaking
// change there.
const (
	KindToken      = "token"
	KindCredential = "credential"
	KindEmail      = "email"
	KindURLQuery   = "url-query"
	KindHomePath   = "home-path"
	KindUserPath   = "user-path"
)

// kindOrder is the order Counts reports in.
var kindOrder = []string{KindToken, KindCredential, KindEmail, KindURLQuery, KindHomePath, KindUserPath}

// The placeholders left where something was removed. They are deliberately not
// valid as the thing they replace.
//
// 🔴 THEY ARE NOT INERT TO LATER RULES, and an earlier comment here said they
// were. Some patterns below exclude `<` from what they match (so a segment is
// not rewritten twice), which means a placeholder can end a match — that is the
// defect the package doc's ordering note records. Do not add a rule on the
// assumption that a placeholder is invisible to it.
var placeholderOf = map[string]string{
	KindToken:      "<token>",
	KindCredential: "<credential>",
	KindEmail:      "<email>",
	KindURLQuery:   "",
	KindHomePath:   "~",
	KindUserPath:   "<user>",
}

// markerOf maps each kind to the code point that records, INSIDE the text being
// processed, that a redaction of that kind happened just before it.
//
// Counting has to follow the text: a later rule can swallow an earlier
// placeholder, and ApplyClipped throws most of the text away. A side counter
// incremented at replacement time cannot see either, which is how the counts
// came to describe a megabyte of input rather than the 500 characters sent. So
// each replacement leaves its marker behind it, a replacement made through
// `replaced` re-emits the markers inside what it removed, and the counts are
// read off whatever text is finally returned.
//
// U+FDD0–U+FDEF are Unicode NONCHARACTERS: permanently reserved for a process's
// internal use and never valid in interchanged text. Any already present in the
// input are removed first, so one cannot be mistaken for a redaction, and none
// survives into the output (TestInternalMarkersNeverReachTheOutput).
var markerOf = map[string]rune{
	KindToken:      0xFDD0,
	KindCredential: 0xFDD1,
	KindEmail:      0xFDD2,
	KindURLQuery:   0xFDD3,
	KindHomePath:   0xFDD4,
	KindUserPath:   0xFDD5,
}

var kindOfMarker = func() map[rune]string {
	m := map[rune]string{}
	for k, r := range markerOf {
		m[r] = k
	}
	return m
}()

func isMarker(r rune) bool { return r >= 0xFDD0 && r <= 0xFDEF }

// markerClass is the marker range as a regexp character-class fragment, for the
// patterns whose "any character but…" classes must not run across one.
const markerClass = `\x{FDD0}-\x{FDEF}`

// markersIn returns the markers contained in s, in order.
func markersIn(s string) string {
	if !strings.ContainsFunc(s, isMarker) {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if isMarker(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripMarkers(s string) string {
	if !strings.ContainsFunc(s, isMarker) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isMarker(r) {
			return -1
		}
		return r
	}, s)
}

// replaced is what stands in for `removed`: the kind's placeholder, its marker,
// and any markers `removed` itself contained.
func replaced(kind, removed string) string {
	return placeholderOf[kind] + string(markerOf[kind]) + markersIn(removed)
}

// MinTokenLen is the shortest configured credential value the exact rule will
// match.
//
// A real Civitai credential is tens of characters. The floor exists because the
// match is a plain substring replace: a one- or two-character "token" (a typo in
// CIVITAI_TOKEN, a test value) would otherwise be replaced everywhere it occurs
// inside ordinary words and turn the report into noise. Below the floor the
// value is NOT redacted by that rule, and the caller is told so through
// SkippedShortTokens rather than left to assume it was.
const MinTokenLen = 8

// Count is how many times one kind of redaction fired.
type Count struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// Options is what a Redactor needs to know about this machine.
type Options struct {
	// Home is the user's home directory, or "" when it could not be resolved (in
	// which case the home-path rule is skipped and user-path still covers the
	// common layouts).
	Home string
	// Tokens are the credential values to remove exactly. Empty and duplicate
	// entries are ignored.
	Tokens []string
}

// Redactor applies the rules and accumulates counts across its Apply /
// ApplyClipped calls, so one instance can cover several fields of one report.
type Redactor struct {
	home   string
	tokens []string
	// skippedShort counts configured tokens below MinTokenLen.
	skippedShort int
	counts       map[string]int
}

// New builds a Redactor.
func New(opts Options) *Redactor {
	r := &Redactor{counts: map[string]int{}}

	home := strings.TrimRight(opts.Home, `/\`)
	// A home of "" or a bare root would turn every absolute path into "~".
	if len(home) > 1 {
		r.home = home
	}

	seen := map[string]bool{}
	for _, t := range opts.Tokens {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		if len(t) < MinTokenLen {
			r.skippedShort++
			continue
		}
		r.tokens = append(r.tokens, t)
	}
	// Longest first, so a token that contains another is removed whole.
	sort.SliceStable(r.tokens, func(a, b int) bool { return len(r.tokens[a]) > len(r.tokens[b]) })
	return r
}

// SkippedShortTokens is how many configured credential values were too short
// for the exact-match rule (see MinTokenLen).
func (r *Redactor) SkippedShortTokens() int { return r.skippedShort }

// Tokens returns the credential values the exact-match rule covers, so a caller
// can assert that none of them survived into what it is about to send.
func (r *Redactor) Tokens() []string { return append([]string(nil), r.tokens...) }

// Counts reports what was removed from the text returned so far, in a fixed
// order, omitting kinds that never fired. It is never nil.
func (r *Redactor) Counts() []Count {
	out := []Count{}
	for _, k := range kindOrder {
		if n := r.counts[k]; n > 0 {
			out = append(out, Count{Kind: k, Count: n})
		}
	}
	return out
}

// Apply returns s with every rule applied, and adds what it removed to the
// counts.
func (r *Redactor) Apply(s string) string {
	if s == "" {
		return s
	}
	return r.settle(r.mark(s))
}

// ApplyClipped redacts s, trims surrounding white space, and returns at most
// max UTF-16 code units of the result (the unit the server's length limits are
// counted in), reporting whether anything was cut off.
//
// 🔴 ONLY WHAT IS RETURNED IS COUNTED. The cut is made on the REDACTED text —
// so a long path costs its placeholder's length, not its own — and a redaction
// beyond the cut is not reported, because it describes text nobody is sending.
//
// A cut that would land inside a placeholder backs up to the placeholder's
// start: half of `<token>` is neither informative nor countable.
func (r *Redactor) ApplyClipped(s string, max int) (string, bool) {
	m := strings.TrimSpace(r.mark(s))

	cut, units := len(m), 0
	for i, c := range m {
		if isMarker(c) {
			continue
		}
		n := utf16.RuneLen(c)
		if n < 1 {
			n = 1
		}
		if units+n > max {
			cut = i
			break
		}
		units += n
	}
	if cut < len(m) {
		// The first marker at or after the cut closes a placeholder that ends
		// right before it. If that placeholder began before the cut, the cut is
		// inside it.
		for i, c := range m[cut:] {
			if !isMarker(c) {
				continue
			}
			if start := cut + i - len(placeholderOf[kindOfMarker[c]]); start < cut {
				cut = start
			}
			break
		}
	}
	clipped := stripMarkers(m[cut:]) != ""
	return strings.TrimSpace(r.settle(m[:cut])), clipped
}

// settle reads the counts off marked text and returns it without its markers.
func (r *Redactor) settle(marked string) string {
	if !strings.ContainsFunc(marked, isMarker) {
		return marked
	}
	for _, c := range marked {
		if k, ok := kindOfMarker[c]; ok {
			r.counts[k]++
		}
	}
	return stripMarkers(marked)
}

// mark applies every rule, leaving a marker behind each replacement. It counts
// nothing; settle does.
func (r *Redactor) mark(s string) string {
	s = stripMarkers(s)
	s = r.urls(s)
	s = authHeaderRe.ReplaceAllStringFunc(s, func(m string) string {
		g := authHeaderRe.FindStringSubmatch(m)
		return g[1] + replaced(KindCredential, g[2])
	})
	s = r.exactTokens(s)
	s = r.credentials(s)
	s = emailRe.ReplaceAllStringFunc(s, func(m string) string { return replaced(KindEmail, m) })
	s = r.homeDir(s)
	s = r.userPaths(s)
	return s
}

// urlTokenRe matches one URL TOKEN: a scheme, `://`, and every following
// character that is not white space.
//
// 🔴 WHITE SPACE IS THE ONLY BOUNDARY, ON PURPOSE. This pattern has been two
// others: one whose query ran to a quote or angle bracket, and one whose
// userinfo ran to the first `@` and could not contain `/`, `?` or `#`. Both
// were lists of characters at which a part of a URL "ends", and each was
// defeated by a secret (or its neighbour) containing a listed character — the
// part ended early and the remainder was sent, under a count that said it was
// removed. A pasted URL does not follow a grammar; where it stops in running
// text is the one thing that can be read off the text.
// TestPropertyASecretInsideAURLTokenNeverSurvives generates the surroundings
// instead of enumerating them.
var urlTokenRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://\S*`)

// urls takes each URL token apart. With `rest` being what follows `://`:
//
//  1. USERINFO. If rest contains an `@`, everything before its LAST `@` is
//     replaced by the credential placeholder. The last, because a password may
//     contain `@`; and this cut is made FIRST, so a `?` or `#` inside a
//     password cannot end the token before the password is reached.
//  2. If a `?` or `#` occurred BEFORE that last `@`, everything after the `@`
//     is removed as well, and a url-query redaction is recorded beside the
//     credential one. Neither character is legal in a userinfo, so the
//     token is either a password that contains one (`u:a?b@host`) or a URL
//     whose query or fragment contains an `@` (`host/p?to=x@SECRET`) — and the
//     text cannot say which. In the second reading what follows the `@` is the
//     tail of a query, so it is not kept. The cost is that the first reading
//     loses its host.
//  3. QUERY AND FRAGMENT. Otherwise, everything from the first `?` or `#` of
//     what is left to the end of the token is removed.
//
// What this takes that it should not is listed under KNOWN OVER-REDACTIONS in
// the package doc.
func (r *Redactor) urls(s string) string {
	return urlTokenRe.ReplaceAllStringFunc(s, func(tok string) string {
		sep := strings.Index(tok, "://") + len("://")
		scheme, rest := tok[:sep], tok[sep:]

		out := scheme
		if at := strings.LastIndexByte(rest, '@'); at >= 0 {
			userinfo := rest[:at]
			out += replaced(KindCredential, userinfo) + "@"
			rest = rest[at+1:]
			if strings.ContainsAny(userinfo, "?#") {
				// Recorded as a url-query redaction as well, whether or not
				// anything followed the `@`: under the second reading the query
				// is what was just removed.
				return out + replaced(KindURLQuery, rest)
			}
		}
		if cut := strings.IndexAny(rest, "?#"); cut >= 0 {
			return out + rest[:cut] + replaced(KindURLQuery, rest[cut:])
		}
		return out + rest
	})
}

func (r *Redactor) exactTokens(s string) string {
	for _, t := range r.tokens {
		if strings.Contains(s, t) {
			s = strings.ReplaceAll(s, t, replaced(KindToken, ""))
		}
	}
	return s
}

// pemBlockRe matches a whole PEM private-key block, or — when the END line is
// missing because the excerpt was cut — from the BEGIN line to the end of the
// text. credscan's own PEM pattern locates only the header WORDS, which is right
// for a warning that prints a location and wrong for a redactor: removing the
// header would leave the key body behind.
//
// The five dashes are required. Without them the words alone ("the BEGIN
// PRIVATE KEY line is missing") would open a block that runs to the end of the
// text and delete the rest of somebody's sentence; that bare mention is left to
// credscan's pattern, which removes just the words.
var pemBlockRe = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z0-9 ]*PRIVATE KEY-----|\z)`)

// authHeaderRe matches an `Authorization` header and its VALUE. Group 1 is the
// name, its `:` or `=` (on the SAME line as the name), and any white space
// after it — line breaks included — and is kept. Group 2 starts at the first non-blank character after that and
// runs to the end of the line it is on, and is removed.
//
// Two properties, each of which was once missing:
//
//   - THE VALUE RUNS TO THE END OF ITS LINE, FOR ANY SCHEME. The first version
//     named four schemes and took one space-free word after the scheme, which
//     fits Bearer and Basic and not Digest: `Digest username="Mufasa", …,
//     response="6629…"` lost `username=` and kept the response hash.
//   - THE VALUE MAY BE ON A LATER LINE. The second version allowed only spaces
//     and tabs before the value, so a pretty-printed JSON headers object
//     (`"Authorization":` then the value on the next line) and a folded header
//     went out untouched — which the first version had caught.
//
// The cost of both — prose after the word, anything else on the value's line,
// and the NEXT line when the header's own line is empty — is in KNOWN
// OVER-REDACTIONS.
//
// It runs before the exact-token rule so that a header holding the configured
// token is one redaction, not a placeholder taken in by another.
var authHeaderRe = regexp.MustCompile("(?i)\\b(authorization[\"']?[ \\t]*[:=]\\s*)(\\S[^\\r\\n]*)")

// apiKeyRe matches the value of an `x-api-key` / `api-key` / `apiKey` /
// `api_key` header, parameter or assignment. Group 1 is kept; group 2 is
// removed when it carries a digit (see valueLooksReal). credscan's assignment
// rule knows `API_KEY` and `APIKEY` behind an entropy gate; it does not know the
// hyphenated header form.
var apiKeyRe = regexp.MustCompile(`(?i)\b((?:x[-_])?api[-_]?key["']?\s*[:=]\s*["']?)([^\s"'&,;<][^\s"'&,;]{7,})`)

// bearerRe matches a bearer credential with NO header name in front of it.
var bearerRe = regexp.MustCompile(`(?i)\b(Bearer\s+)([A-Za-z0-9._~+/=-]{16,})`)

// valueLooksReal is the digit test the api-key and bare-Bearer rules apply to a
// value: "Bearer authentication is required" and "api-key: required" are
// sentences, and their "values" have no digit. It is a heuristic in both
// directions — a credential with no digit is a KNOWN MISS, and a non-secret
// value with one (`apiKey = options.apiKey2`) is a KNOWN OVER-REDACTION.
func valueLooksReal(v string) bool {
	return strings.ContainsAny(v, "0123456789")
}

// replaceGroup2 rewrites each match of re as group 1 followed by the
// credential placeholder, when redactIf(group 2) allows it.
func replaceGroup2(re *regexp.Regexp, s string, redactIf func(string) bool) string {
	return re.ReplaceAllStringFunc(s, func(m string) string {
		g := re.FindStringSubmatch(m)
		if redactIf != nil && !redactIf(g[2]) {
			return m
		}
		return g[1] + replaced(KindCredential, g[2])
	})
}

func (r *Redactor) credentials(s string) string {
	s = pemBlockRe.ReplaceAllStringFunc(s, func(m string) string { return replaced(KindCredential, m) })
	s = replaceGroup2(apiKeyRe, s, valueLooksReal)
	s = replaceGroup2(bearerRe, s, valueLooksReal)

	// credscan's detectors are line-oriented; keep the line breaks exactly.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		spans := credscan.Spans(line)
		if len(spans) == 0 {
			continue
		}
		var b strings.Builder
		prev := 0
		for _, sp := range spans {
			b.WriteString(line[prev:sp.Start])
			b.WriteString(replaced(KindCredential, line[sp.Start:sp.End]))
			prev = sp.End
		}
		b.WriteString(line[prev:])
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}

// emailRe matches an e-mail address. It is deliberately permissive about the
// local part and strict about the domain having a dot and an alphabetic TLD, so
// `user@host` in an SSH error is left alone and `name@example.org` is not.
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)*\.[A-Za-z]{2,}`)

// homeDir replaces this machine's home directory with `~` wherever it appears
// as a whole path prefix — followed by a separator, or by something that cannot
// continue a path segment. `/home/kestrel` therefore does not match inside
// `/home/kestrel2`; the user-path rule handles that one.
func (r *Redactor) homeDir(s string) string {
	if r.home == "" || !strings.Contains(s, r.home) {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, r.home)
		if i < 0 {
			break
		}
		end := i + len(r.home)
		if continuesPathSegment(s[end:]) {
			// Part of a longer name: keep it and move on.
			b.WriteString(s[:end])
			s = s[end:]
			continue
		}
		b.WriteString(s[:i])
		b.WriteString(replaced(KindHomePath, ""))
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}

// continuesPathSegment reports whether rest begins with something that could
// continue a file-name segment, i.e. whether a match ending just before it
// stopped in the middle of a name. A marker is not part of any name.
func continuesPathSegment(rest string) bool {
	if rest == "" {
		return false
	}
	c, _ := utf8.DecodeRuneInString(rest)
	switch {
	case isMarker(c):
		return false
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '_', c == '-', c == '.':
		return true
	}
	return c >= 0x80
}

// unixUserRe and windowsUserRe match the user segment of the three home layouts
// this CLI's platforms use. Group 1 is the prefix that is kept.
//
// The Windows pattern accepts one OR two backslashes, because a path inside a
// JSON string or a Go-quoted error arrives doubled. `C:/Users/…` needs no rule
// of its own: its tail is the `/Users/` case.
//
// 🔴 A WINDOWS USER NAME CAN CONTAIN A SPACE ("Ingrid Solberg"), so the Windows
// pattern has two arms and their order is the rule: first a segment that may
// hold spaces and is CLOSED by a backslash (group 2, re-emitted), and only when
// no backslash follows, a segment that ends at white space. With one arm that
// stopped at white space, `C:\\Users\\Ingrid Solberg\\src` came out as
// `<user> Solberg\\src`. With no closing separator the name's extent cannot be
// known, and the second arm removes its first word only.
//
// The first arm runs to the NEXT backslash on the line, wherever that is — see
// KNOWN OVER-REDACTIONS.
//
// The user segment excludes `<` (so a segment this rule already rewrote is not
// matched again) and the marker range (so it does not take in the record of an
// earlier redaction).
var (
	unixUserRe    = regexp.MustCompile(`(/home/|/Users/)[^/\\\s"'<>:` + markerClass + `]+`)
	windowsUserRe = regexp.MustCompile(`(?i)([A-Za-z]:\\{1,2}Users\\{1,2})(?:[^/\\"'<>:\r\n` + markerClass + `]+(\\)|[^/\\\s"'<>:` + markerClass + `]+)`)
)

func (r *Redactor) userPaths(s string) string {
	s = unixUserRe.ReplaceAllStringFunc(s, func(m string) string {
		return unixUserRe.FindStringSubmatch(m)[1] + replaced(KindUserPath, "")
	})
	s = windowsUserRe.ReplaceAllStringFunc(s, func(m string) string {
		g := windowsUserRe.FindStringSubmatch(m)
		return g[1] + replaced(KindUserPath, "") + g[2]
	})
	return s
}
