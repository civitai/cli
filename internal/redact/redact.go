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
// A count names a KIND and a number — never the text removed.
//
// # The rules, in the order they run
//
//  1. token       — each configured credential value, matched EXACTLY.
//  2. credential  — a PEM private-key block, a `Bearer <value>` header value,
//     and everything internal/credscan's detectors locate (credscan.Spans):
//     known token formats, a URL with an embedded password, and the value of a
//     strict `KEY=value` assignment that passes its entropy gate.
//  3. email       — an e-mail address.
//  4. url-query   — the `?query` of a URL (signed-URL parameters live there).
//  5. home-path   — this machine's home directory, replaced by `~`.
//  6. user-path   — the user segment of `/home/<u>`, `/Users/<u>` and
//     `C:\Users\<u>`.
//
// The order is load-bearing in two places. The exact token goes FIRST, so a
// credential the CLI can name with certainty is counted as itself rather than as
// whichever shape it happens to resemble. And the URL-password rule (inside 2)
// runs before `email`, because `user:pass@host.tld` is also e-mail-shaped and
// the e-mail rule would remove the host while leaving the password.
//
// # 🔴 What it does NOT do
//
// It is not a guarantee that no secret leaves. Rule 1 is exact and certain; the
// rest are SHAPES, and rule 2 inherits every deliberate silence of
// internal/credscan (a placeholder-looking or low-entropy value, a Google API
// key shape, a headerless key body — see that package's doc). A credential in
// none of those shapes is sent as typed. That is why the command also shows the
// exact payload under --dry-run: the reader is the last check, and the output
// never claims the text is "clean".
//
// It also does not look for instructions aimed at whoever reads the report.
// There is no "prompt-injection detector" here and one must not be added: it
// would be a phrase list, and AGENTS.md item 28 records how those lose.
package redact

import (
	"regexp"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/credscan"
)

// The kinds a Count can carry. They are part of `civitai feedback send`'s
// published --json contract (README, "Platform feedback"), so renaming one is a
// breaking change there.
const (
	KindToken      = "token"
	KindCredential = "credential"
	KindEmail      = "email"
	KindURLQuery   = "url-query"
	KindHomePath   = "home-path"
	KindUserPath   = "user-path"
)

// kindOrder is the order Counts reports in — the order the rules run.
var kindOrder = []string{KindToken, KindCredential, KindEmail, KindURLQuery, KindHomePath, KindUserPath}

// The placeholders left where something was removed. They are deliberately not
// valid as the thing they replace, and none contains a character a later rule
// would match again.
const (
	tokenPlaceholder      = "<token>"
	credentialPlaceholder = "<credential>"
	emailPlaceholder      = "<email>"
	userPlaceholder       = "<user>"
	homePlaceholder       = "~"
)

// MinTokenLen is the shortest configured credential value rule 1 will match.
//
// A real Civitai credential is tens of characters. The floor exists because the
// match is a plain substring replace: a one- or two-character "token" (a typo in
// CIVITAI_TOKEN, a test value) would otherwise be replaced everywhere it occurs
// inside ordinary words and turn the report into noise. Below the floor the
// value is NOT redacted by rule 1, and the caller is told so through
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
	// which case rule 5 is skipped and rule 6 still covers the common layouts).
	Home string
	// Tokens are the credential values to remove exactly. Empty and duplicate
	// entries are ignored.
	Tokens []string
}

// Redactor applies the rules and accumulates counts across every Apply call, so
// one instance covers every field of one report.
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

// Counts reports what fired, in rule order, omitting kinds that never did.
func (r *Redactor) Counts() []Count {
	out := []Count{}
	for _, k := range kindOrder {
		if n := r.counts[k]; n > 0 {
			out = append(out, Count{Kind: k, Count: n})
		}
	}
	return out
}

// Apply returns s with every rule applied, and adds to the counts.
func (r *Redactor) Apply(s string) string {
	if s == "" {
		return s
	}
	s = r.exactTokens(s)
	s = r.credentials(s)
	s = r.replaceRe(emailRe, s, emailPlaceholder, KindEmail)
	s = r.urlQueries(s)
	s = r.homeDir(s)
	s = r.userPaths(s)
	return s
}

func (r *Redactor) exactTokens(s string) string {
	for _, t := range r.tokens {
		if n := strings.Count(s, t); n > 0 {
			r.counts[KindToken] += n
			s = strings.ReplaceAll(s, t, tokenPlaceholder)
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

// bearerRe matches an HTTP bearer credential as it appears in a pasted request
// or `curl -v` log. The scheme word is kept so the line still reads as a header;
// only the value goes. credscan has no rule for this shape — an `Authorization`
// header carries no credential WORD in its key.
var bearerRe = regexp.MustCompile(`(?i)\b(Bearer\s+)([A-Za-z0-9._~+/=-]{16,})`)

// bearerValueLooksReal keeps the bearer rule off prose: "Bearer authentication
// is required" is a sentence, and a word is letters only. So the value must
// carry at least one digit. A bearer credential with no digit anywhere in it is
// a stated miss.
func bearerValueLooksReal(v string) bool {
	return strings.ContainsAny(v, "0123456789")
}

func (r *Redactor) credentials(s string) string {
	s = r.replaceRe(pemBlockRe, s, credentialPlaceholder, KindCredential)

	s = bearerRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := bearerRe.FindStringSubmatch(m)
		if !bearerValueLooksReal(sub[2]) {
			return m
		}
		r.counts[KindCredential]++
		return sub[1] + credentialPlaceholder
	})

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
			b.WriteString(credentialPlaceholder)
			prev = sp.End
		}
		b.WriteString(line[prev:])
		lines[i] = b.String()
		r.counts[KindCredential] += len(spans)
	}
	return strings.Join(lines, "\n")
}

// emailRe matches an e-mail address. It is deliberately permissive about the
// local part and strict about the domain having a dot and an alphabetic TLD, so
// `user@host` in an SSH error is left alone and `name@example.org` is not.
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)*\.[A-Za-z]{2,}`)

// urlQueryRe matches a URL up to its `?`, then the query. Group 1 is everything
// kept. The query runs to the first character that cannot be part of one in
// running text (whitespace, a quote, an angle bracket) or to a `#` — the
// fragment is kept, since it names a place in a page, not a parameter.
var urlQueryRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://[^\s?#"'<>]+)\?[^\s#"'<>]*`)

func (r *Redactor) urlQueries(s string) string {
	return urlQueryRe.ReplaceAllStringFunc(s, func(m string) string {
		r.counts[KindURLQuery]++
		return urlQueryRe.ReplaceAllString(m, "${1}")
	})
}

// homeDir replaces this machine's home directory with `~` wherever it appears
// as a whole path prefix — followed by a separator, or by something that cannot
// continue a path segment. `/home/kestrel` therefore does not match inside
// `/home/kestrel2`; rule 6 handles that one.
func (r *Redactor) homeDir(s string) string {
	if r.home == "" {
		return s
	}
	var b strings.Builder
	n := 0
	for {
		i := strings.Index(s, r.home)
		if i < 0 {
			break
		}
		end := i + len(r.home)
		if end < len(s) && isPathSegmentByte(s[end]) {
			// Part of a longer name: keep it and move on.
			b.WriteString(s[:end])
			s = s[end:]
			continue
		}
		b.WriteString(s[:i])
		b.WriteString(homePlaceholder)
		s = s[end:]
		n++
	}
	if n == 0 && b.Len() == 0 {
		return s
	}
	b.WriteString(s)
	r.counts[KindHomePath] += n
	return b.String()
}

// isPathSegmentByte reports whether c could continue a file-name segment, i.e.
// whether a match ending just before it stopped in the middle of a name.
func isPathSegmentByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '_', c == '-', c == '.':
		return true
	}
	return c >= 0x80 // any byte of a multi-byte rune
}

// unixUserRe and windowsUserRe match the user segment of the three home layouts
// this CLI's platforms use. Group 1 is the prefix that is kept.
//
// The Windows pattern accepts one OR two backslashes, because a path inside a
// JSON string or a Go-quoted error arrives doubled. `C:/Users/…` needs no rule
// of its own: its tail is the `/Users/` case.
//
// The user segment excludes `<` so that a segment this rule already rewrote is
// never rewritten (and counted) twice.
var (
	unixUserRe    = regexp.MustCompile(`(/home/|/Users/)[^/\\\s"'<>:]+`)
	windowsUserRe = regexp.MustCompile(`(?i)([A-Za-z]:\\{1,2}Users\\{1,2})[^/\\\s"'<>:]+`)
)

func (r *Redactor) userPaths(s string) string {
	for _, re := range []*regexp.Regexp{unixUserRe, windowsUserRe} {
		re := re
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			r.counts[KindUserPath]++
			return re.ReplaceAllString(m, "${1}"+userPlaceholder)
		})
	}
	return s
}

func (r *Redactor) replaceRe(re *regexp.Regexp, s, with, kind string) string {
	return re.ReplaceAllStringFunc(s, func(string) string {
		r.counts[kind]++
		return with
	})
}
