package redact

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The two lists a reader of this package needs in order not to over-trust it —
// what redaction does NOT remove, and what it removes that was not a secret —
// live in three places: the package doc, the decision doc, and here. They used
// to disagree (three shapes pinned, four in the package doc, five in the
// decision doc, under a sentence saying each was pinned).
//
// So HERE is the source. Every entry carries its documented sentence and an
// input that demonstrates it; TestKnownMissesAreStillMisses and
// TestKnownOverRedactionsStillHappen run the inputs, and
// TestKnownListsAreTheSameInEveryDoc requires both documents to carry exactly
// these sentences and no other bullet.

// knownCase is one demonstration of a list entry.
type knownCase struct {
	// tokens are the configured credentials for this case (nil = the usual two).
	tokens []string
	in     string
	// want is the exact output. For a miss it defaults to in.
	want string
	// survives, for a miss whose output is not the input, is the part that was
	// meant to go and is still there.
	survives string
}

type knownEntry struct {
	// doc is the sentence both documents must carry, verbatim.
	doc   string
	cases []knownCase
}

var b64Token = base64.StdEncoding.EncodeToString([]byte(testToken))

// knownMisses is what redaction does NOT remove.
var knownMisses = []knownEntry{
	{
		doc:   "a bare `Bearer <value>` with no `Authorization` header in front of it, when the value has no digit",
		cases: []knownCase{{in: "sent Bearer QwErTyUiOpAsDfGhJkLzXcVbNm to it"}},
	},
	{
		doc:   "an `x-api-key` / `api-key` value that has no digit",
		cases: []knownCase{{in: "x-api-key: QwErTyUiOpAsDfGhJk"}},
	},
	{
		doc:   "the query or fragment of a URL written without a scheme (`cdn.example/f?sig=…`)",
		cases: []knownCase{{in: "curl cdn.fieldstone.dev/f.bin?sig=Yt6Ru8Ei0Ow2Pq4La"}},
	},
	{
		doc: "a credential that appears transformed: URL-encoded, base64 of itself, or split across lines",
		cases: []knownCase{
			{tokens: []string{"Kd4+Rb6/Zq7=Lm2Xv9"}, in: "value Kd4%2BRb6%2FZq7%3DLm2Xv9 was rejected"},
			{in: "value " + b64Token + " was rejected"},
			{in: "value " + testToken[:16] + "\n" + testToken[16:] + " was rejected"},
		},
	},
	{
		doc:   "a configured credential shorter than 8 characters (stderr says when one was skipped)",
		cases: []knownCase{{tokens: []string{"ab12cd3"}, in: "the key ab12cd3 failed"}},
	},
	{
		doc: "a URL query that contains a literal space: the URL token ends at the space, so `?q=hello world&sig=…` keeps `world&sig=…`",
		cases: []knownCase{{
			in:       "GET https://h.fieldstone.dev/s?q=hello world&sig=Gh5Jk7Lm9Np1Qr3St failed",
			want:     "GET https://h.fieldstone.dev/s world&sig=Gh5Jk7Lm9Np1Qr3St failed",
			survives: "sig=Gh5Jk7Lm9Np1Qr3St",
		}},
	},
	{
		doc:   "a JSON-escaped URL (`https:\\/\\/…?sig=…`): it is not recognised as a URL, so its query is kept",
		cases: []knownCase{{in: `{"url":"https:\/\/cdn.fieldstone.dev\/f.bin?sig=Uv2Wx4Yz6Ab8Cd0Ef"}`}},
	},
}

// knownOverRedactions is what redaction removes that was not a secret. Every
// one is in the safe direction, and none gets a rule of its own.
var knownOverRedactions = []knownEntry{
	{
		doc: "everything after `Authorization:` to the end of that line, whatever it is — prose included, and anything else that shares the line",
		cases: []knownCase{
			{in: "Authorization: Bearer credentials missing", want: "Authorization: <credential>"},
			{in: `curl -H "Authorization: Basic xyz" https://api.fieldstone.dev/v1`, want: `curl -H "Authorization: <credential>`},
		},
	},
	{
		doc: "the line after an `Authorization` header whose own line holds no value — the value is looked for across line breaks",
		cases: []knownCase{
			{in: "Authorization:\nHost: api.fieldstone.dev\nAccept: */*", want: "Authorization:\n<credential>\nAccept: */*"},
		},
	},
	{
		doc:   "an `api-key` / `apiKey` assignment whose value has a digit, even when the value is a variable name",
		cases: []knownCase{{in: "const apiKey = options.apiKey2", want: "const apiKey = <credential>"}},
	},
	{
		doc:   "any URL userinfo, even a bare user name (`ssh://git@host/…` is counted as a credential)",
		cases: []knownCase{{in: "clone ssh://git@github.com/civitai/cli.git", want: "clone ssh://<credential>@github.com/civitai/cli.git"}},
	},
	{
		doc: "everything between `://` and the last `@` of a URL token — so an `@` in a path or query takes the host with it — and, when a `?` or `#` came before that `@`, the rest of the token too",
		cases: []knownCase{
			{in: "see https://h.fieldstone.dev/users/@ingrid/posts", want: "see https://<credential>@ingrid/posts"},
			{in: "GET https://h.fieldstone.dev/p?notify=ops@fieldstone.dev&x=1", want: "GET https://<credential>@"},
		},
	},
	{
		doc: "a URL's `#fragment`, and whatever is glued to a URL after its first `?` or `#` with no white space: closing quotes, brackets, a trailing full stop, the rest of a minified JSON line",
		cases: []knownCase{
			{in: "(see https://developer.civitai.com/apps/guide#scopes).", want: "(see https://developer.civitai.com/apps/guide"},
			{in: `{"url":"https://h.fieldstone.dev/p?a=1","retries":3}`, want: `{"url":"https://h.fieldstone.dev/p`},
		},
	},
	{
		doc:   "a Windows user segment runs to the next backslash on the line, so `copy C:\\Users\\ingrid .\\dist\\out` loses `ingrid .`",
		cases: []knownCase{{in: `copy C:\Users\ingrid .\dist\out`, want: `copy C:\Users\<user>\dist\out`}},
	},
	{
		doc:   "`/home/<name>` and `/Users/<name>` wherever they appear, including inside a URL path",
		cases: []knownCase{{in: "docs at https://docs.fieldstone.dev/home/welcome/intro", want: "docs at https://docs.fieldstone.dev/home/<user>/intro"}},
	},
	{
		doc:   "the words of a private-key header (`BEGIN … PRIVATE KEY`) in prose",
		cases: []knownCase{{in: "the BEGIN RSA PRIVATE KEY line is missing", want: "the <credential> line is missing"}},
	},
}

func (c knownCase) redactor() *Redactor {
	if c.tokens != nil {
		return New(Options{Home: testHome, Tokens: c.tokens})
	}
	return newTestRedactor()
}

// TestKnownMissesAreStillMisses pins every entry of the KNOWN MISSES list. If
// one of these starts being redacted the list is stale in the safe direction:
// remove the entry here and the doc test below will make both documents follow.
func TestKnownMissesAreStillMisses(t *testing.T) {
	for _, e := range knownMisses {
		if len(e.cases) == 0 {
			t.Errorf("%q has no demonstration, so it is a claim and not a measurement", e.doc)
		}
		for _, c := range e.cases {
			r := c.redactor()
			got := r.Apply(c.in)
			want := c.want
			if want == "" {
				want = c.in
			}
			if got != want {
				t.Errorf("the documented miss no longer behaves as documented.\n entry: %s\n    in: %q\n   got: %q\n  want: %q", e.doc, c.in, got, want)
			}
			if c.survives != "" && !strings.Contains(got, c.survives) {
				t.Errorf("documented as NOT covered, but %q is gone — remove the entry.\n entry: %s\n   out: %q", c.survives, e.doc, got)
			}
			if n := len(r.Counts()); want == c.in && n != 0 {
				t.Errorf("%q: nothing was removed, yet %+v was counted", c.in, r.Counts())
			}
		}
	}
	// The short-token entry's parenthesis is a claim too.
	if got := New(Options{Tokens: []string{"ab12cd3"}}).SkippedShortTokens(); got != 1 {
		t.Errorf("SkippedShortTokens = %d for one 7-character token, want 1 — that is what the stderr note is driven by", got)
	}
}

// TestKnownOverRedactionsStillHappen pins every entry of the KNOWN
// OVER-REDACTIONS list with the exact text that is left.
func TestKnownOverRedactionsStillHappen(t *testing.T) {
	for _, e := range knownOverRedactions {
		if len(e.cases) == 0 {
			t.Errorf("%q has no demonstration", e.doc)
		}
		for _, c := range e.cases {
			r := c.redactor()
			got := r.Apply(c.in)
			if got == c.in {
				t.Errorf("documented as an over-redaction, but the text was left alone — remove it from the list.\n entry: %s\n    in: %q", e.doc, c.in)
				continue
			}
			if got != c.want {
				t.Errorf("entry: %s\n    in: %q\n   got: %q\n  want: %q", e.doc, c.in, got, c.want)
			}
		}
	}
}

var (
	listSpaceRe = regexp.MustCompile(`\s+`)
	// A bullet is a line starting `- ` (markdown) or `//   - ` (the Go doc).
	listBulletRe = regexp.MustCompile(`(?m)^(?://\s*)?\s*- `)
)

// listSection returns the text between two anchors of a document, with Go
// comment markers removed and white space collapsed.
func listSection(t *testing.T, path, from, to string) (normalised string, bullets int) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc := string(raw)
	i := strings.Index(doc, from)
	if i < 0 {
		t.Fatalf("%s has no %q heading — the list this test polices is gone or renamed", path, from)
	}
	rest := doc[i+len(from):]
	j := strings.Index(rest, to)
	if j < 0 {
		t.Fatalf("%s: nothing matching %q follows %q, so the list has no end", path, to, from)
	}
	section := rest[:j]
	bullets = len(listBulletRe.FindAllString(section, -1))
	section = regexp.MustCompile(`(?m)^//`).ReplaceAllString(section, "")
	return strings.TrimSpace(listSpaceRe.ReplaceAllString(section, " ")), bullets
}

// TestKnownListsAreTheSameInEveryDoc: the package doc and the decision doc each
// carry both lists, with exactly the entries above — every sentence present
// verbatim, and no bullet that is not one of them.
func TestKnownListsAreTheSameInEveryDoc(t *testing.T) {
	decision := filepath.Join("..", "..", "claudedocs", "decisions", "41-platform-feedback.md")
	docs := []struct {
		name, path string
		// the anchors that open each list and close the second one
		misses, over, end string
	}{
		{"package doc", "redact.go", "KNOWN MISSES —", "KNOWN OVER-REDACTIONS —", "It also does not look for instructions"},
		{"decision doc", decision, "KNOWN MISSES —", "KNOWN OVER-REDACTIONS —", "## 3."},
	}
	for _, d := range docs {
		for _, list := range []struct {
			name     string
			from, to string
			entries  []knownEntry
		}{
			{"KNOWN MISSES", d.misses, d.over, knownMisses},
			{"KNOWN OVER-REDACTIONS", d.over, d.end, knownOverRedactions},
		} {
			section, bullets := listSection(t, d.path, list.from, list.to)
			for _, e := range list.entries {
				want := listSpaceRe.ReplaceAllString(e.doc, " ")
				if !strings.Contains(section, want) {
					t.Errorf("%s, %s: missing the entry\n    %s", d.name, list.name, e.doc)
				}
			}
			if bullets != len(list.entries) {
				t.Errorf("%s, %s: %d bullet(s) for %d pinned entr(ies) — a bullet nothing pins, or an entry not listed",
					d.name, list.name, bullets, len(list.entries))
			}
		}
	}
}
