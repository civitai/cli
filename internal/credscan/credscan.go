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
// matched text — a renderer cannot print what it was never given.
//
// 🔴 THAT TYPE-LEVEL HALF IS NOT SUFFICIENT ON ITS OWN, AND SAYING SO IS THE
// POINT. assignRe's key group is unanchored, so a "key" can be value bytes: a
// line reading `AbCdEfLEAKMARK9SecretKey123: <token>` printed all 27 of those
// characters as its label on the first shipped build, under a sentence promising
// it had not. labelFor closes it — a key is printed only when it is
// identifier-shaped AND fails this package's own secret test; anything else
// degrades to the credential WORD in upper case, a closed set of constants that
// carries nothing from the line. Pinned by TestLabelNeverCarriesSecretBytes,
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
//   - B  — nine known credential FORMATS, which need no assignment at all.
//
// The corpus rate is re-measured on every change to either half, and the
// measurement is the gate: see the PR for the current fired/scanned figure.
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
// # What this does NOT see — the coverage holes, enumerated
//
// A silent run is not a clean bill of health, and the reasons are specific:
//
//   - BINARY FILES are skipped — a NUL byte in the first 8 KiB (git's own
//     heuristic), 85 of the corpus's 3,917 packaged files. A credential embedded
//     in an image, a font or a compiled asset is not detected.
//   - A LINE OVER maxLineBytes is skipped. The REST OF THE FILE still is
//     scanned (it did not use to be — see that constant), but a credential
//     inside a minified bundle's single long line is not seen.
//   - A CONNECTION STRING whose key carries no credential word
//     (`DATABASE_URL=…`) is invisible to A2. The URL form in knownFormats
//     catches the `scheme://user:password@host` shape, and nothing catches a
//     password passed some other way.
//   - ONE FINDING PER LINE. A minified line holding several secrets reports only
//     the first that passes the gate (every credential word on the line gets its
//     turn — see matchAssignment — but the line stops at the first hit). An
//     UNQUOTED value still runs to end of line, so on dense one-line content the
//     value can be over-captured; a quoted one stops at its closing quote.
//   - GOOGLE API KEYS are exempt by shape — see publicByDesignRe — so a Google
//     *server* key is a deliberate false negative, and so is a headerless PKCS#8
//     body (see knownFormats).
//   - THE BYTE BUDGET (MaxScanBytes) can stop the scan early. That one is NOT
//     silent: Report.Truncated says so and the warning prints it.
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
	// Label says WHY the line matched: a key NAME for an assignment
	// (`API_SECRET`, `password`), or the format's name for a known credential
	// shape (`AWS access key id`).
	//
	// 🔴 A KEY IS ONLY PRINTED WHEN IT DOES NOT ITSELF LOOK LIKE A CREDENTIAL —
	// see labelFor. The unanchored key pattern can otherwise capture secret bytes
	// (measured: `AbCdEfLEAKMARK9SecretKey123` was printed as a "key"), so the
	// guarantee is enforced there rather than assumed here.
	Label string
}

const (
	// maxLineBytes bounds ONE line. A minified bundle is a single multi-megabyte
	// line, and neither detector can say anything useful about one, so a line
	// over this is skipped — but the REST OF THE FILE IS STILL SCANNED. It did
	// not use to be: bufio.Scanner stops at ErrTooLong and never resumes, so one
	// long line blinded every line after it, in a file (a `vendor.js`, a bundled
	// asset) whose long line is at the top. That silence was invisible.
	maxLineBytes = 1 << 20 // 1 MiB

	// binarySniffBytes is how much of a file is inspected for a NUL byte before
	// deciding it is binary. 8 KiB is git's own window.
	binarySniffBytes = 8 << 10

	// maxFindingsPerFile caps how many lines one file contributes. A file with
	// forty matching lines is not forty times more actionable than one with
	// five, and the header counts FILES, so the cap cannot make the count wrong.
	maxFindingsPerFile = 5

	// MaxScanBytes is the total budget across the whole bundle. Past it the scan
	// STOPS and says so (Report.Truncated) — a silently truncated scan is a
	// reassuring zero, which is the failure mode this whole feature exists to
	// avoid.
	//
	// 🔴 IT IS A LATENCY BOUND, AND THE NUMBER COMES FROM A MEASUREMENT. Scanning
	// costs ~230 ms/MiB on the machine this was measured on (19.4 MiB of text:
	// 233 ms → 4,759 ms end to end), and pkgzip's own caps legally permit a
	// 200 MiB / 2000-file bundle — i.e. ~45 s of silent stall between the
	// confirmation prompt and the upload. 16 MiB bounds that at ~3.7 s worst
	// case, while being ~57× the largest packaged TEXT payload measured across
	// 244 real project directories (287 KiB), so no real project is truncated.
	// Binary files cost only their 8 KiB sniff, so assets do not eat the budget.
	MaxScanBytes int64 = 16 << 20
)

// Report is the result of a scan: what was found, and what was NOT looked at.
//
// 🔴 THE SECOND HALF IS THE POINT. A scan that stopped early and said nothing
// would be indistinguishable from a clean bundle, and this feature's whole
// premise is that silence gets believed.
type Report struct {
	// Findings are in the order the files were given, ascending by line.
	Findings []Finding
	// Truncated reports that the byte budget ran out before every file was read.
	Truncated bool
	// FilesScanned counts the files this scan actually opened and read (binary
	// files included — they were read far enough to be classified), and
	// FilesTotal is how many it was given. They differ only when Truncated.
	FilesScanned int
	FilesTotal   int
}

// Scan reads each of files (archive-relative, as returned by pkgzip.Result.Files)
// under dir and reports the lines that look like credentials.
//
// It never returns an error: this is advisory. A file that cannot be opened or
// read is skipped — the packager has already read it successfully, so a failure
// here is a race or a permissions oddity, and turning that into a submit failure
// would trade a warning for an outage.
func Scan(dir string, files []string) Report {
	rep := Report{FilesTotal: len(files)}
	budget := MaxScanBytes
	for _, rel := range files {
		if budget <= 0 {
			rep.Truncated = true
			break
		}
		found, spent := scanFile(dir, rel, budget)
		budget -= spent
		rep.FilesScanned++
		rep.Findings = append(rep.Findings, found...)
	}
	return rep
}

// scanFile scans one packaged file within budget bytes, and reports the bytes it
// spent. rel is archive-relative and slash-separated.
func scanFile(dir, rel string, budget int64) ([]Finding, int64) {
	f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	// Sized to the sniff window so Peek can fill it in one go — the default
	// 4 KiB reader would answer Peek(8 KiB) with ErrBufferFull and half the
	// window, which is a quieter detector than the one that was measured.
	br := bufio.NewReaderSize(f, binarySniffBytes)
	if isBinary(br) {
		// A binary file costs its sniff and nothing more, which is why a bundle
		// full of images does not eat the byte budget.
		return nil, binarySniffBytes
	}

	var (
		found []Finding
		spent int64
	)
	for n := 1; ; n++ {
		line, skipped, consumed, err := readLine(br)
		spent += int64(consumed)
		if consumed == 0 && err != nil {
			break
		}
		// A line past maxLineBytes is skipped and the file CONTINUES — see the
		// constant's comment for the silence that cost.
		if !skipped {
			if label, ok := match(line); ok {
				found = append(found, Finding{Path: rel, Line: n, Label: label})
				if len(found) == maxFindingsPerFile {
					break
				}
			}
		}
		if err != nil || spent >= budget {
			break
		}
	}
	return found, spent
}

// readLine reads one line, never buffering more than maxLineBytes of it.
//
// It returns the line (without its terminator), whether the line was SKIPPED for
// exceeding the cap, how many bytes were consumed from the reader (the budget is
// charged on what was read, not on what was kept), and the reader's error — with
// io.EOF returned alongside a final unterminated line rather than instead of it.
//
// 🔴 A SKIPPED LINE DOES NOT END THE FILE. That is the whole reason this exists
// instead of bufio.Scanner, whose ErrTooLong is terminal.
func readLine(br *bufio.Reader) (line string, skipped bool, consumed int, err error) {
	var buf []byte
	for {
		chunk, e := br.ReadSlice('\n')
		consumed += len(chunk)
		switch {
		case skipped:
			// Already over the cap: keep draining to the newline, keep nothing.
		case len(buf)+len(chunk) > maxLineBytes:
			skipped = true
			buf = nil
		default:
			buf = append(buf, chunk...)
		}
		if e == bufio.ErrBufferFull {
			continue
		}
		return string(bytes.TrimRight(buf, "\r\n")), skipped, consumed, e
	}
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
// credential into. Matched case-insensitively against the START of the value.
//
// 🔴 PREFIX, NOT SUBSTRING, AND THE REASON THIS COMMENT USED TO GIVE WAS FALSE.
// It claimed `theRealTokenGoesHere1` shows why a substring test is wrong — but
// `the` is in the list, so the PREFIX test rejects that string too, and an
// auditor confirmed it does not fire. The real reason is the opposite case: a
// genuine random token is long, and every short word here (`the`, `my`, `xxx`)
// turns up INSIDE random strings, so a substring test would silence real
// credentials. `Q8vN2mR7kX4zL9pWtheB6` is the fixture that proves it — it is
// reported, and would not be under a substring test.
// 🔴 `$` IS IN THE LIST BECAUSE A CORPUS RUN PUT IT THERE. A value beginning
// with `$` is a shell/env REFERENCE (`$FORGEJO_TOKEN`), the same class as
// `process.env.X` and `${…}` — and a real README line,
// `https://civitai-admin:$FORGEJO_TOKEN@forgejo.civitai.com/…`, was the single
// biggest source of false positives in the whole 244-project corpus.
var placeholderPrefixes = []string{
	"your", "my", "the", "<", "$", "null", "true", "false",
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

// matchAssignment implements A2. It returns a LABEL — never the value, and never
// the key when the key itself looks like a credential (see labelFor).
//
// 🔴 IT RETRIES PAST A REJECTED MATCH. The value capture runs to end of line, so
// one line yields one regexp match however many credential words it holds — and
// a minified single-line JSON blob (`{"token":"","API_SECRET":"…"}`) put the
// FIRST word in front of the gate and, when that failed, gave up on the rest of
// the line. The retry resumes just past the operator that failed.
func matchAssignment(line string) (string, bool) {
	for pos := 0; pos < len(line); {
		loc := assignRe.FindStringSubmatchIndex(line[pos:])
		if loc == nil {
			return "", false
		}
		key := line[pos+loc[2] : pos+loc[3]]
		word := line[pos+loc[4] : pos+loc[5]]
		value, quoted := cleanValue(line[pos+loc[8] : pos+loc[9]])
		if valueLooksLikeSecret(value, quoted) {
			return labelFor(key, word), true
		}
		// Resume just past this match's OPERATOR, so the next credential word on
		// the line gets its own turn. Advancing by at least one byte is what
		// makes the loop terminate.
		next := pos + loc[7]
		if next <= pos {
			next = pos + 1
		}
		pos = next
	}
	return "", false
}

// labelFor decides what may be printed for an assignment match, and it is the
// ONE place the never-print-the-value guarantee is ENFORCED rather than assumed.
//
// 🔴 THE KEY PATTERN IS UNANCHORED, SO A "KEY" CAN BE VALUE BYTES. Measured on
// the shipped build: a line reading `AbCdEfLEAKMARK9SecretKey123: <token>`
// printed `AbCdEfLEAKMARK9SecretKey123` as its label — 27 bytes of
// credential-shaped material, directly above a sentence promising the value was
// not printed. Two of those bytes runs would be enough to make a leaked token
// searchable.
//
// So a key is printed only when it BOTH looks like a key (identifier-shaped) and
// fails this package's own secret test — the same gate a value goes through,
// plus the known-format table. Anything else degrades to the credential WORD in
// upper case, which is one of a closed set of constants from assignRe's own
// alternation and therefore carries no material from the line.
func labelFor(key, word string) string {
	k := keyLabel(key)
	if keyShapeRe.MatchString(k) && !looksLikeCredential(k) {
		return k
	}
	return strings.ToUpper(word)
}

// looksLikeCredential is the shared test "would this string, standing alone, be
// reported as a secret". Used on a KEY before printing it.
func looksLikeCredential(s string) bool {
	if _, ok := matchKnownFormat(s); ok {
		return true
	}
	return valueLooksLikeSecret(s, false)
}

// keyShapeRe is what a key NAME looks like: an identifier, optionally dotted or
// hyphenated. It rejects a capture that is mostly punctuation or base64 padding,
// which is what a swallowed value fragment looks like.
var keyShapeRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$.\-]*$`)

// cleanValue reduces the raw capture to the value itself: trailing comment off,
// trailing `,`/`;` off, matching quotes off. It reports whether the value WAS
// quoted, which is what makes the dotted-reference rule safe to apply.
func cleanValue(raw string) (value string, quoted bool) {
	// 🔴 A QUOTED VALUE ENDS AT ITS CLOSING QUOTE, NOT AT END OF LINE. The capture
	// group runs to EOL, which on minified single-line JSON made the value of
	// `{"token":""` the whole REST OF THE LINE — so an empty value looked like a
	// high-entropy secret and the finding was attributed to the wrong key.
	if closed, ok := untilClosingQuote(raw); ok {
		return closed, true
	}
	v := strings.TrimSpace(stripTrailingComment(raw))
	v = strings.TrimSpace(strings.TrimRight(v, ",;"))
	if len(v) >= 2 {
		if q := v[0]; (q == '"' || q == '\'' || q == '`') && v[len(v)-1] == q {
			return v[1 : len(v)-1], true
		}
	}
	return v, false
}

// untilClosingQuote returns the contents of a quoted value when raw begins with
// a quote and that quote is closed on the same line. A backslash escapes the
// next byte, so `"a\"b"` is one value.
func untilClosingQuote(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	q := raw[0]
	if q != '"' && q != '\'' && q != '`' {
		return "", false
	}
	for i := 1; i < len(raw); i++ {
		if raw[i] == '\\' && q != '`' {
			i++
			continue
		}
		if raw[i] == q {
			return raw[1:i], true
		}
	}
	return "", false
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
	if publicByDesignRe.MatchString(value) {
		return false
	}
	return shannonBitsPerChar(value) >= minEntropyBitsPerChar
}

// publicByDesignRe is the one value shape this detector deliberately stays quiet
// about: a Google API key, because a FIREBASE WEB key is that shape and Google
// documents it as public — it ships in the client bundle and lives in the
// `.env.production` this feature puts in scope. Dropping the B pattern alone
// would not have helped: A2 reports the same key through
// `VITE_FIREBASE_API_KEY=AIza…`, so a Firebase block would warn on EVERY submit,
// which is the noise failure the whole design optimises against.
//
// 🔴 THE COST IS REAL AND IS STATED: a Google *server* key is secret and has the
// same shape, so it is a deliberate false negative. Anchored to the exact key
// shape, so it exempts nothing else.
var publicByDesignRe = regexp.MustCompile(`^AIza[0-9A-Za-z_-]{35}$`)

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
//
// validate, when set, is a second test the matched text must pass. Only the URL
// form needs one: `postgres://admin:hunter2@db` is a leak and
// `postgres://user:password@localhost:5432/db` is a README, and no regexp that
// finds the first without finding the second is worth reading.
type knownFormat struct {
	name     string
	re       *regexp.Regexp
	validate func(match string) bool
}

// 🔴 TWO PATTERNS WERE REMOVED FOR FIRING ON PUBLIC, NON-SECRET MATERIAL, and
// removing them is the design rather than a retreat from it: a warning that
// fires on every submit is the "trains authors to ignore it" failure this
// detector is built to avoid, and it would land on exactly the projects the
// feature is meant to protect.
//
//   - `AIza[0-9A-Za-z_-]{35}` — a Google API key, and a FIREBASE WEB API KEY has
//     that same shape. Google documents the web key as public: it ships in
//     client bundles, and a Firebase-based block keeps it in the
//     `.env.production` this feature deliberately puts in scope. The two are
//     indistinguishable by shape, so the pattern cannot be made precise. See
//     publicByDesignRe — the value gate exempts it too, because A2 would
//     otherwise report the same key through `VITE_FIREBASE_API_KEY=…`.
//   - `MII[A-Za-z0-9+/]{40,}` — the base64 body of a DER structure, which is an
//     X.509 CERTIFICATE and a `BEGIN PUBLIC KEY` body just as often as a private
//     key. A private key still carries its PEM header, and that pattern is
//     precise, so nothing that matters is lost.
//
// Both are stated as deliberate false negatives in the package doc and README: a
// Google *server* key (secret, same shape) and a headerless PKCS#8 body are not
// reported.
var knownFormats = []knownFormat{
	{name: "PEM private key block", re: regexp.MustCompile(`BEGIN [A-Z0-9 ]*PRIVATE KEY`)},
	{name: "JWT", re: regexp.MustCompile(`eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}`)},
	{name: "AWS access key id", re: regexp.MustCompile(`(AKIA|ASIA|AGPA|AIDA|AROA|ANPA)[0-9A-Z]{16}`)},
	{name: "GitHub token", re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	{name: "Slack token", re: regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`)},
	{name: "OpenAI API key", re: regexp.MustCompile(`sk-[A-Za-z0-9]{32,}`)},
	{name: "Stripe API key", re: regexp.MustCompile(`[sr]k_(live|test)_[A-Za-z0-9]{16,}`)},
	{name: "npm token", re: regexp.MustCompile(`npm_[A-Za-z0-9]{36}`)},
	{
		// The shape a connection string leaks in — `DATABASE_URL=postgres://…`
		// carries no credential word in its KEY, so A2 cannot see it, and an
		// auditor named it the most common real leak shape in a web project.
		name:     "URL with an embedded password",
		re:       regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://[^\s:/?#@]+:[^\s:/?#@]+@`),
		validate: urlPasswordLooksReal,
	},
}

// urlPasswordLooksReal keeps the URL form off documentation. It takes the
// userinfo password out of the match and asks the same placeholder/entropy
// questions a value gets, minus the length floor — a real database password is
// routinely shorter than 12 characters, which is exactly why it is worth saying.
func urlPasswordLooksReal(match string) bool {
	at := strings.LastIndex(match, "@")
	colon := strings.LastIndex(match[:at], ":")
	if at < 0 || colon < 0 {
		return false
	}
	pw := match[colon+1 : at]
	if len(pw) < 4 || rejectRe.MatchString(pw) || rejectRe.MatchString(match) {
		return false
	}
	lower := strings.ToLower(pw)
	for _, p := range placeholderPrefixes {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}
	// `user:password@`, `admin:pass@` — the words documentation uses.
	switch lower {
	case "password", "pass", "pwd", "secret", "hunter2":
		return false
	}
	// A reference is not a secret: `$FORGEJO_TOKEN`, `${DB_PASSWORD}`, `%s`.
	// Measured — the `$VAR` form was 12 of the 14 findings in a corpus run.
	return !strings.ContainsAny(pw, "$%")
}

// matchKnownFormat implements B. Order is the table's order, so the label for a
// line matching two formats is stable.
func matchKnownFormat(line string) (string, bool) {
	for _, f := range knownFormats {
		m := f.re.FindString(line)
		if m == "" {
			continue
		}
		if f.validate != nil && !f.validate(m) {
			continue
		}
		return f.name, true
	}
	return "", false
}
