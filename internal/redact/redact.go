// Package redact removes what `civitai feedback send` must not carry off the
// machine, BEFORE anything is sent: the text a developer (or their coding agent)
// pastes into a friction report is an error message, a path, a log excerpt —
// exactly the material that holds a home directory, an e-mail address or a live
// credential.
//
// # It runs before the request is built, and it reports what it did
//
// The caller applies one Redactor to every free-text field and then prints
// Counts(), so the user can see that something was removed and of which kind.
// A count names a KIND and a number — never the text removed — and it counts
// what was removed from the text that is RETURNED: ApplyClipped does not count
// a redaction that fell beyond its cut.
//
// Input must already be text. DecodeText (text.go) is the read boundary: it
// turns a UTF-16 file into UTF-8 and refuses what is not text at all. Every
// rule here is a byte-oriented matcher, and NUL-interleaved UTF-16 walks
// straight past all of them.
//
// # The rules, in the order they run
//
//  1. URLs, on the ORIGINAL text: the userinfo of `scheme://user:pass@host`
//     (kind credential) and the `?query` (kind url-query; signed-URL
//     parameters live there).
//  2. token       — each configured credential value, matched EXACTLY.
//  3. credential  — a PEM private-key block; the value of an `Authorization:`
//     header (Bearer, Basic, Token, Digest); the value of an `x-api-key` /
//     `api-key` header or parameter; a bare `Bearer <value>`; and everything
//     internal/credscan's detectors locate (credscan.Spans): known token
//     formats and the value of a strict `KEY=value` assignment that passes its
//     entropy gate.
//  4. email       — an e-mail address.
//  5. home-path   — this machine's home directory, replaced by `~`.
//  6. user-path   — the user segment of `/home/<u>`, `/Users/<u>` and
//     `C:\Users\<u>`.
//
// 🔴 THE ORDER IS LOAD-BEARING, AND URLS GO FIRST FOR A MEASURED REASON. Every
// rule from 2 on leaves a placeholder containing `<` and `>`, and those are
// characters a URL matcher has to treat as "the URL ended here". When the query
// strip ran after them, `?token=<configured token>&sig=…` became
// `?token=<token>&sig=…`, the strip stopped at the `<`, and `&sig=…` was sent —
// while the count still said the query had been dropped. And a URL with
// `user:pass@` lost its scheme to the credential span, so the query rule never
// saw a URL at all. A URL is therefore taken apart while it is still a URL.
// A consequence, stated: a token that sits inside a query string is counted as
// part of the `url-query` that removed it, not as a `token`.
//
// The exact token still precedes the shape rules, so a credential the CLI can
// name with certainty is counted as itself rather than as whichever shape it
// resembles; and the credential rules precede `email`, because
// `user:pass@host.tld` is also e-mail-shaped.
//
// # 🔴 What it does NOT do
//
// It is not a guarantee that no secret leaves. Rule 2 is exact and certain; the
// rest are SHAPES, and rule 3 inherits every deliberate silence of
// internal/credscan (a placeholder-looking or low-entropy value, a Google API
// key shape, a headerless key body — see that package's doc). A credential in
// none of those shapes is sent as typed. That is why the command also shows the
// exact payload under --dry-run: the reader is the last check, and the output
// never claims the text is "clean".
//
// NOT COVERED, each pinned by TestStatedMissesAreStillMisses so this list is a
// measurement rather than a guess:
//
//   - a bare `Bearer <value>` (no `Authorization:` in front) whose value has no
//     digit — the digit is what keeps "Bearer authentication is required" from
//     being rewritten;
//   - an `x-api-key` / `api-key` value with no digit, for the same reason;
//   - the query of a URL written WITHOUT a scheme (`cdn.example/f?sig=…`) —
//     nothing marks where such a URL starts;
//   - a credential that appears transformed: URL-encoded, base64 of itself,
//     split across lines.
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
// were. `<` and `>` are terminators in several patterns below — that is the
// defect the package doc's ordering note records. The rules are ordered and
// written with that in mind; do not add one on the assumption that a
// placeholder is invisible to it.
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
// each replacement leaves its marker behind it, a replacement that swallows
// markers re-emits them, and the counts are read off whatever text is finally
// returned.
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

// Redactor applies the rules and accumulates counts across every Apply /
// ApplyClipped call, so one instance covers every field of one report.
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
	s = r.exactTokens(s)
	s = r.credentials(s)
	s = emailRe.ReplaceAllStringFunc(s, func(m string) string { return replaced(KindEmail, m) })
	s = r.homeDir(s)
	s = r.userPaths(s)
	return s
}

// urlRe matches one URL and splits it into the parts that are kept and the
// parts that go: 1 = `scheme://`, 2 = `userinfo@` (optional), 3 = host and path,
// 4 = `?query` (optional). A fragment is not part of the match and so is kept —
// it names a place in a page.
//
// 🔴 THE CLASSES STOP AT WHITE SPACE AND QUOTES, AND DELIBERATELY NOT AT `<` OR
// `>`. This runs on the original text, so there are no placeholders to stop at,
// and the user's own `?id=<id>&key=…` must not end the strip early. The cost is
// that a `>` closing a markdown autolink is taken with the query.
var urlRe = regexp.MustCompile("([A-Za-z][A-Za-z0-9+.\\-]*://)([^\\s/?#@\"'`]*@)?([^\\s?#\"'`]*)(\\?[^\\s#\"'`]*)?")

func (r *Redactor) urls(s string) string {
	return urlRe.ReplaceAllStringFunc(s, func(m string) string {
		g := urlRe.FindStringSubmatch(m)
		out := g[1]
		if g[2] != "" {
			// The whole userinfo goes, user name included: `deploy:` beside a
			// removed password is half an account.
			out += replaced(KindCredential, g[2]) + "@"
		}
		out += g[3]
		if g[4] != "" {
			out += replaced(KindURLQuery, g[4])
		}
		return out
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

// authHeaderRe matches the value of an HTTP Authorization header as it appears
// in a pasted request, a `curl -v` log or a JSON headers object. Group 1 is
// kept (the header name and scheme word, so the line still reads as a header);
// group 2 is the credential.
//
// The header name in front is the signal, so the value needs no digit: Basic is
// base64 and a bearer token can be letters only. The value may not START with
// `<` — that is a placeholder the exact-token rule already left.
var authHeaderRe = regexp.MustCompile(`(?i)\b(authorization["']?\s*[:=]\s*["']?(?:bearer|basic|token|digest)\s+)([^\s"',;<][^\s"',;]{7,})`)

// apiKeyRe matches the value of an `x-api-key` / `api-key` / `api_key` header
// or parameter. Group 1 is kept, group 2 is the credential — when it carries a
// digit (see valueLooksReal). credscan's assignment rule knows `API_KEY` and
// `APIKEY` behind an entropy gate; it does not know the hyphenated header form.
var apiKeyRe = regexp.MustCompile(`(?i)\b((?:x[-_])?api[-_]?key["']?\s*[:=]\s*["']?)([^\s"'&,;<][^\s"'&,;]{7,})`)

// bearerRe matches a bearer credential with NO header name in front of it.
var bearerRe = regexp.MustCompile(`(?i)\b(Bearer\s+)([A-Za-z0-9._~+/=-]{16,})`)

// valueLooksReal keeps the two rules that have no header name to lean on off
// prose: "Bearer authentication is required" and "api-key: required" are
// sentences, and a word is letters only. So the value must carry at least one
// digit. A credential with no digit anywhere in it is a stated miss (package
// doc).
func valueLooksReal(v string) bool {
	return strings.ContainsAny(v, "0123456789")
}

// replaceGroup2 rewrites every match of re as group 1 followed by the
// credential placeholder, when keep(group 2) allows it.
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
	s = replaceGroup2(authHeaderRe, s, nil)
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
// The user segment excludes `<`, so a segment this rule already rewrote is
// never rewritten twice, and excludes the marker range, so it cannot swallow
// the record of an earlier redaction.
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
