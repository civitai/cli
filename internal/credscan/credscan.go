// Package credscan reads the files `civitai app submit` is ABOUT TO UPLOAD and
// reports the lines that look like they hold a credential.
//
// # Why this exists (issue #464, split out of #435)
//
// `internal/pkgzip`'s exclusions are all NAME rules — prefix, suffix, exact,
// case-folded. A credential in a file whose name looks like ordinary app content
// (`secrets.json`, `src/credentials.yaml`, `config.toml`, or an allow-listed
// `.env.production`) is invisible to every one of them, at every depth, and
// always will be. That bundle goes to Forgejo `civitai-apps/<slug>`, is read by a
// human moderator reviewer, and cannot be recalled.
//
// # 🔴 IT WARNS. IT NEVER DROPS, REFUSES, OR CHANGES AN EXIT CODE.
//
// The asymmetry that governs every rule in pkgzip points the other way here. For
// a NAME rule, matching too much costs one file the author renames. For a
// CONTENT heuristic, matching too much would silently delete app source — a
// `.ts` file containing the literal string `password` is ordinary code. So this
// package returns findings and the caller prints them; nothing here may become
// an exclusion, a gate, or a non-zero exit. `--yes`/CI paths must keep working
// exactly as they did.
//
// # 🔴 A FINDING CARRIES A LOCATION AND A KEY NAME, NEVER THE VALUE.
//
// This CLI's output lands in CI logs and terminal scrollback, so printing the
// matched secret would move the credential into a SECOND durable place while
// telling the author off for the first. Finding has no field that can hold the
// matched text, which is the structural version of that rule: a renderer cannot
// print what it was never given. Pinned by
// TestScanNeverReturnsTheValue and, end-to-end, by
// TestSubmitCredentialWarningNeverPrintsTheValue.
//
// # 🔴 THE SCOPE IS EXACTLY WHAT SHIPPED.
//
// Scan takes the file list from pkgzip.Result.Files and reads nothing else. It
// does not re-derive the exclusion rules — a second copy of those drifts from
// the first, which is the whole #435 class — so it can never name a file the
// packager dropped, and it automatically covers whatever pkgzip decides to keep
// (including `.env.production`, kept by name because the build reads it, which
// keptEnvFiles' own comment is explicit says nothing about whether it is safe).
//
// # The detector: A2 ∪ B
//
// Chosen by measuring five candidates over 244 real project directories and
// 3,917 packaged files. A2 ∪ B fired on ONE project (0.4%) — two lines in one
// `.test.ts`. Plain keyword-assignment matching (no entropy gate) fired on 86.1%
// of projects, and an entropy-only detector on 100% of authored projects
// (lockfile `integrity` hashes), so both were rejected: a warning that fires on
// every run trains authors to ignore it and is worse than nothing.
//
//   - A2 — a strict assignment whose VALUE survives a placeholder/entropy gate.
//   - B  — ten known credential FORMATS, which need no assignment at all.
//
// # 🔴 THERE IS NO `*.test.*` CARVE-OUT, ON PURPOSE.
//
// The two known false positives in the corpus are in a test file, so excluding
// test files would take the measured rate to zero — and that is exactly why it
// is not done. A credential in a test file is uploaded, reviewed and deployed
// identically to one anywhere else, and a carve-out shaped like "the files where
// our false positives happen to live" is how a guard gets hollowed out later.
// Two false positives are the accepted price. Pinned by
// TestNoTestFileCarveOut.
//
// # Binary files are SKIPPED, deliberately and not silently
//
// A file whose first 8 KiB contain a NUL byte is treated as binary and not
// scanned (git's own heuristic). 85 of the corpus's 3,917 packaged files are
// such files — images, fonts, `.ico`s. Line-scanning them produces nothing
// useful and would spend the budget on megabytes of noise. The cost is stated
// rather than hidden: **a credential embedded in a binary asset is not
// detected**, and neither is one on a line longer than maxLineBytes (a minified
// bundle). Both are coverage holes this warning does not close.
package credscan

import (
	"bufio"
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding is one line that looks like it holds a credential.
//
// 🔴 IT HAS NO FIELD FOR THE MATCHED VALUE, AND MUST NOT GROW ONE. See the
// package doc: the leak-prevention property of this feature is enforced by the
// type, not by every renderer remembering.
type Finding struct {
	// Path is the archive-relative path, spelled exactly as pkgzip.Result.Files
	// spells it (forward slashes, project-relative).
	Path string
	// Line is 1-based, so it pastes into an editor as `path:line`.
	Line int
	// Label says WHY the line matched: the key name for an assignment
	// (`API_SECRET`, `password`), or the format's name for a known credential
	// shape (`AWS access key id`). A key name is not the secret; the value is.
	Label string
}

const (
	// maxLineBytes bounds one line. A minified bundle is a single multi-megabyte
	// line, and neither detector can say anything useful about one; a file whose
	// lines exceed this is abandoned rather than partially scanned, because a
	// truncated line splits tokens and would report matches at invented offsets.
	maxLineBytes = 1 << 20 // 1 MiB

	// binarySniffBytes is how much of a file is inspected for a NUL byte before
	// deciding it is binary. 8 KiB is git's own window.
	binarySniffBytes = 8 << 10

	// maxFindingsPerFile caps how many lines one file contributes. A file with
	// forty matching lines is not forty times more actionable than one with
	// five, and the header counts FILES, so the cap cannot make the count wrong.
	maxFindingsPerFile = 5
)

// Scan reads each of files (archive-relative, as returned by pkgzip.Result.Files)
// under dir and returns the lines that look like credentials, in the order the
// files were given and by ascending line within a file.
//
// It never returns an error: this is advisory. A file that cannot be opened or
// read is skipped — the packager has already read it successfully, so a failure
// here is a race or a permissions oddity, and turning that into a submit failure
// would trade a warning for an outage.
func Scan(dir string, files []string) []Finding {
	var out []Finding
	for _, rel := range files {
		out = append(out, scanFile(dir, rel)...)
	}
	return out
}

// scanFile scans one packaged file. rel is archive-relative and slash-separated.
func scanFile(dir, rel string) []Finding {
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	defer f.Close()

	// Sized to the sniff window so Peek can fill it in one go — the default
	// 4 KiB reader would answer Peek(8 KiB) with ErrBufferFull and half the
	// window, which is a quieter detector than the one that was measured.
	br := bufio.NewReaderSize(f, binarySniffBytes)
	if isBinary(br) {
		return nil
	}

	var found []Finding
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for n := 1; sc.Scan(); n++ {
		label, ok := match(sc.Text())
		if !ok {
			continue
		}
		found = append(found, Finding{Path: rel, Line: n, Label: label})
		if len(found) == maxFindingsPerFile {
			break
		}
	}
	// A scan error (a line past maxLineBytes, an I/O failure) abandons the REST
	// of the file and keeps what was already found. See maxLineBytes.
	return found
}

// isBinary reports whether the buffered file looks binary — a NUL byte in the
// first binarySniffBytes. It peeks, so the reader is left positioned at the
// start either way.
func isBinary(br *bufio.Reader) bool {
	head, err := br.Peek(binarySniffBytes)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return true // unreadable — say nothing about it
	}
	return bytes.IndexByte(head, 0) >= 0
}

// match runs the two detectors over one line. A2 first, because when both fire
// the key name is the more useful label.
func match(line string) (string, bool) {
	if label, ok := matchAssignment(line); ok {
		return label, true
	}
	return matchKnownFormat(line)
}

// ---------------------------------------------------------------------------
// A2 — strict assignment, placeholder- and entropy-gated
// ---------------------------------------------------------------------------

// assignRe matches `<key containing a credential word> <op> <value to EOL>`.
//
// 🔴 THE TRAILING CHARACTER CLASS MUST INCLUDE `"`. A JSON key is QUOTED, so
// without it the pattern cannot get past the closing quote of `"API_SECRET":`
// and the detector misses `secrets.json` — issue #464's own headline file. An
// earlier revision omitted it and did exactly that; only a positive control
// caught it. TestAssignmentMatchesAQuotedJSONKey pins it.
//
// Submatches: 1 = the key (with whatever quoting/indexing surrounds it),
// 2 = the credential word, 3 = the operator, 4 = the value to end of line.
var assignRe = regexp.MustCompile(
	`(?i)([A-Za-z0-9_.\-\[\]"']{0,40}(SECRET|TOKEN|PASSWORD|PASSWD|API_?KEY|PRIVATE_?KEY|ACCESS_?KEY|CREDENTIALS?)[A-Za-z0-9_.\-"']{0,24})\s*(:=|=>|=|:)\s*(\S.*)$`)

// placeholderPrefixes are the openings of a value nobody ever pasted a live
// credential into. Matched case-insensitively against the START of the value,
// because `your-api-key-here` is a placeholder and `theRealTokenGoesHere1` is
// not — a substring test would reject the second for spelling "the".
var placeholderPrefixes = []string{
	"your", "my", "the", "<", "null", "true", "false",
	"example", "sample", "dummy", "fake", "placeholder", "todo",
	"changeme", "redacted", "hidden", "xxx", "...",
}

// interpolationMarkers are the spellings of "this is not the value, it is a
// reference to it". A value carrying one is a variable read, not a secret.
var interpolationMarkers = []string{"${", "process.env", "import.meta"}

// rejectRe drops a value that names itself as non-real. Applied to the WHOLE
// value (not just its start), because `sk-test-...` and `abc-localhost-9090`
// carry the tell in the middle.
var rejectRe = regexp.MustCompile(`(?i)(mock|dummy|fake|sample|example|placeholder|localhost|changeme|redacted|not[._-]?a[._-]?real|xxxx|test|demo|dev\.)`)

// dottedRefRe matches an UNQUOTED dotted reference expression — `opts.token`,
// `options.credentials.token`. Quoted values are exempt: `"a.b.c.d.e.f.g"` in a
// config file is data, not an expression.
var dottedRefRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)+$`)

// minValueLen is the shortest value this detector will call a credential. Below
// it the string is a flag, a port, a length, or a short word.
const minValueLen = 12

// minEntropyBitsPerChar is the Shannon-entropy floor, in bits per character.
// Below it a value is a repeated or tiny alphabet — `aaaaaaaaaaaa1`,
// `000000000000` — which no credential generator produces.
const minEntropyBitsPerChar = 3.0

// matchAssignment implements A2. It returns the KEY as the label, never the
// value.
func matchAssignment(line string) (string, bool) {
	m := assignRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	value, quoted := cleanValue(m[4])
	if !valueLooksLikeSecret(value, quoted) {
		return "", false
	}
	return keyLabel(m[1]), true
}

// cleanValue reduces the raw capture to the value itself: trailing comment off,
// trailing `,`/`;` off, matching quotes off. It reports whether the value WAS
// quoted, which is what makes the dotted-reference rule safe to apply.
func cleanValue(raw string) (value string, quoted bool) {
	v := strings.TrimSpace(stripTrailingComment(raw))
	v = strings.TrimSpace(strings.TrimRight(v, ",;"))
	if len(v) >= 2 {
		if q := v[0]; (q == '"' || q == '\'' || q == '`') && v[len(v)-1] == q {
			return v[1 : len(v)-1], true
		}
	}
	return v, false
}

// stripTrailingComment removes a `//` or `#` comment.
//
// The marker only counts at the start of the value or after whitespace, so a
// URL value (`https://…`) keeps its slashes instead of being cut to `https:`.
func stripTrailingComment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != '#' && !(s[i] == '/' && i+1 < len(s) && s[i+1] == '/') {
			continue
		}
		if i == 0 || s[i-1] == ' ' || s[i-1] == '\t' {
			return s[:i]
		}
	}
	return s
}

// valueLooksLikeSecret is A2's gate: every condition must hold, then the two
// rejections must not.
func valueLooksLikeSecret(value string, quoted bool) bool {
	if len(value) < minValueLen {
		return false
	}
	if strings.ContainsAny(value, " \t") {
		return false
	}
	for _, marker := range interpolationMarkers {
		if strings.Contains(value, marker) {
			return false
		}
	}
	lower := strings.ToLower(value)
	for _, p := range placeholderPrefixes {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}
	if !quoted && dottedRefRe.MatchString(value) {
		return false
	}
	if !hasDigit(value) || !hasLetter(value) {
		return false
	}
	if rejectRe.MatchString(value) {
		return false
	}
	return shannonBitsPerChar(value) >= minEntropyBitsPerChar
}

func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

func hasLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i] | 0x20; c >= 'a' && c <= 'z' {
			return true
		}
	}
	return false
}

// shannonBitsPerChar is the Shannon entropy of s over its byte frequencies, in
// bits per character. Empty is 0.
func shannonBitsPerChar(s string) float64 {
	if s == "" {
		return 0
	}
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

// keyLabel tidies the captured key for printing: quoting and indexing syntax
// off, and a length cap so a pathological key cannot own the terminal. It can
// only ever shorten the key — it never touches the value, which the caller does
// not have.
func keyLabel(key string) string {
	k := strings.Trim(strings.TrimSpace(key), `"'[].`)
	const keyLabelCap = 40
	if len(k) > keyLabelCap {
		k = k[:keyLabelCap] + "…"
	}
	return k
}

// ---------------------------------------------------------------------------
// B — known credential formats
// ---------------------------------------------------------------------------

// knownFormat is one credential shape that identifies itself, no assignment
// required. The name is printed; the match never is.
type knownFormat struct {
	name string
	re   *regexp.Regexp
}

var knownFormats = []knownFormat{
	{"PEM private key block", regexp.MustCompile(`BEGIN [A-Z0-9 ]*PRIVATE KEY`)},
	{"JWT", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}`)},
	{"AWS access key id", regexp.MustCompile(`(AKIA|ASIA|AGPA|AIDA|AROA|ANPA)[0-9A-Z]{16}`)},
	{"GitHub token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	{"Slack token", regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`)},
	{"OpenAI API key", regexp.MustCompile(`sk-[A-Za-z0-9]{32,}`)},
	{"Stripe API key", regexp.MustCompile(`[sr]k_(live|test)_[A-Za-z0-9]{16,}`)},
	{"Google API key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	{"npm token", regexp.MustCompile(`npm_[A-Za-z0-9]{36}`)},
	{"PKCS#8 key body", regexp.MustCompile(`MII[A-Za-z0-9+/]{40,}`)},
}

// matchKnownFormat implements B. Order is the table's order, so the label for a
// line matching two formats is stable.
func matchKnownFormat(line string) (string, bool) {
	for _, f := range knownFormats {
		if f.re.MatchString(line) {
			return f.name, true
		}
	}
	return "", false
}
