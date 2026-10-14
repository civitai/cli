package credscan

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Span is a half-open byte range [Start, End) of one line that this package's
// detectors consider credential material.
//
// 🔴 IT IS A LOCATION, NOT THE VALUE — the same rule Finding lives under. A
// caller that wants to REMOVE the material (internal/redact) needs to know where
// it is; nothing here hands the bytes to a renderer.
type Span struct {
	Start, End int
}

// Spans returns where, in one line, the two detectors Scan runs would find
// credential material: the VALUE of every strict assignment that passes
// valueLooksLikeSecret (A2), and every known-format match that passes its
// validator (B). Ranges are sorted and non-overlapping.
//
// It exists so that a second consumer — the redaction `civitai feedback send`
// applies before anything leaves the machine — uses THESE matchers rather than
// a private copy of them. Two tables that are meant to agree is the shape
// `internal/saferune` records going wrong (#393).
//
// 🔴 IT INHERITS EVERY DELIBERATE SILENCE OF THOSE DETECTORS, and a caller must
// not read an empty result as "this line holds no secret": a placeholder-shaped
// or low-entropy value, a Google-API-key shape and a headerless key body are all
// unreported on purpose (see the package doc). For a warning that is the right
// trade; a redactor has to say so wherever it describes itself.
//
// Unlike match, it does not stop at the first hit and it does not prefer one
// detector over the other — a line can carry several credentials, and a
// redactor that removed only the first would leave the rest.
func Spans(line string) []Span {
	var out []Span

	for pos := 0; pos < len(line); {
		loc := assignRe.FindStringSubmatchIndex(line[pos:])
		if loc == nil {
			break
		}
		rawStart, rawEnd := pos+loc[8], pos+loc[9]
		if sp, ok := assignment(line, rawStart, rawEnd); ok {
			// 🔴 A CUT UNQUOTED CAPTURE IS EXTENDED WHEREVER cleanValue LEFT ITS END,
			// not only when that end sat exactly at the cut: cleanValue trims a
			// trailing `;`, so `SECRET=<511 chars>;<200 chars>` used to stop before
			// the `;` and send the 200. extendUnquoted re-reads the LINE with the
			// same terminators cleanValue cuts at, so a delimiter that really ends
			// the value still ends it. (A cut QUOTED capture is already whole —
			// assignment read it to its closing quote.)
			// TestSpansCoverACutValueWhoseEndTheCleanerMoved and
			// TestSpansStopAtADelimiterThatEndsTheValue pin both sides.
			if cutUnquoted(line, rawStart, rawEnd) {
				sp.End = extendUnquoted(line, sp.End)
			}
			out = append(out, sp)
		}
		// The same advance matchAssignment makes, with the same floor: resume just
		// past this match's operator so the next credential word gets its turn.
		next := pos + loc[7]
		if next <= pos {
			next = pos + 1
		}
		pos = next
	}

	for _, f := range knownFormats {
		for _, m := range f.re.FindAllStringIndex(line, -1) {
			if f.validate != nil && !f.validate(line[m[0]:m[1]]) {
				continue
			}
			out = append(out, Span{m[0], m[1]})
		}
	}

	return mergeSpans(out)
}

// assignValueBound is the length assignRe's value group stops at. It is restated
// here, and TestSpansCoverAValueLongerThanTheCaptureBound fails if the two
// disagree (a 700-byte value would come back short).
//
// 🔴 IT IS A COUNT OF CHARACTERS (runes), NOT BYTES. Go's regexp matches runes,
// so `\S{1,512}` stops after 512 characters — 512 bytes only when they are all
// ASCII. Spans once compared the capture's BYTE length to it, so a value
// carrying non-ASCII text was never seen as cut and everything past its 512th
// character was left outside the span — and sent by the redactor.
// TestSpansCoverANonASCIIValueLongerThanTheCaptureBound pins the unit.
const assignValueBound = 512

// isQuote reports whether c opens a quoted value, as cleanValue reads one.
func isQuote(c byte) bool { return c == '"' || c == '\'' || c == '`' }

// capturedToTheBound reports whether assignRe's capture of [rawStart, rawEnd)
// stopped at its bound rather than at the value's end.
func capturedToTheBound(line string, rawStart, rawEnd int) bool {
	return utf8.RuneCountInString(line[rawStart:rawEnd]) == assignValueBound
}

func cutUnquoted(line string, rawStart, rawEnd int) bool {
	return !isQuote(line[rawStart]) && capturedToTheBound(line, rawStart, rawEnd)
}

// assignment decides whether one assignRe match assigns a secret, and where
// that value is in line. It is the ONE verdict: matchAssignment (the
// `app submit` warning) and Spans (the redactor) both call it, so the detector
// and the redactor cannot disagree.
//
// 🔴 THE BOUND IS A COMPLEXITY FIX FOR THE DETECTOR, NOT A STATEMENT ABOUT HOW
// LONG A SECRET IS. assignRe stops its capture at 512 characters so a line
// costs linear time. A consumer that REMOVES the secret needs all of it, and a
// cut capture can be the wrong thing to judge as well:
//
//   - 🔴 A CUT QUOTED VALUE IS A SECRET IF EITHER THE WHOLE VALUE OR main's
//     FRAGMENT PASSES THE GATE, and the span is the WHOLE value, read to its
//     real closing quote. Each reading misses what the other catches:
//   - the FRAGMENT is cleanValue's reading of the capture alone, which sends
//     a quoted value whose close lies past the cut through the unquoted rules
//     and ends it at its first `,`. In JSON escaped into a log line —
//     `{"credentials":"{\"client_id\":\"abc\",\"client_secret\":\"<600 chars>\"}"}`
//     — the fragment is `{\"client_id\":\"abc\"`, which has no digit, so main
//     located nothing and sent the client_secret.
//   - the WHOLE value can fail where the fragment passed: a reject word
//     (`test`), an interpolation marker (`${`), or a low-entropy tail past
//     character 512. main redacted those; a whole-only verdict sent them.
//     TestACutQuotedValueIsJudgedWhole and TestParityWithMain
//     (internal/redact) pin both directions.
//   - everything else is cleanValue's reading of the capture; Spans extends a
//     cut unquoted one afterwards (extendUnquoted).
//
// The quote scan stays linear across a line: every later match whose value
// opens with the same quote character puts an unescaped quote in its path (the
// character before it is the operator or white space, never a backslash), so
// a scan never runs past the next same-quote match.
func assignment(line string, rawStart, rawEnd int) (Span, bool) {
	raw := line[rawStart:rawEnd]
	value, quoted := cleanValue(raw)
	// cleanValue only ever trims and unquotes, so the value it returns is a
	// substring of the raw capture; locating it there recovers the offset
	// without a second copy of the cleaning rules.
	start := rawStart + strings.Index(raw, value)
	sp, ok := Span{start, start + len(value)}, valueLooksLikeSecret(value, quoted)

	if q := line[rawStart]; isQuote(q) && capturedToTheBound(line, rawStart, rawEnd) {
		whole := Span{rawStart + 1, closingQuote(line, rawStart+1, q)}
		// Either reading passing is a secret; the span is the whole value.
		return whole, ok || valueLooksLikeSecret(line[whole.Start:whole.End], true)
	}
	return sp, ok
}

// closingQuote returns the index of the quote q that closes a value starting at
// start, or len(line) when the line ends first. A backslash escapes the next
// byte (not inside backticks).
//
// 🔴 IT WALKS FROM THE VALUE'S OWN START. A walk from wherever cleanValue's
// fragment ended cannot see a backslash behind it, and once took the `"` of an
// escaped `\"` as the close — after a `,` just before it, or when the cut fell
// between `\` and `"` — and sent the rest of the value.
// TestSpansFindTheRealClosingQuoteOfAnEscapedValue pins both shapes.
func closingQuote(line string, start int, q byte) int {
	for i := start; i < len(line); i++ {
		if line[i] == '\\' && q != '`' {
			i++
			continue
		}
		if line[i] == q {
			return i
		}
	}
	return len(line)
}

// extendUnquoted moves the end of a cut unquoted value forward to white space
// or to the first character that ends an unquoted value (endsUnquotedValue —
// the same terminators cleanValue cuts at).
func extendUnquoted(line string, end int) int {
	for end < len(line) {
		r, w := utf8.DecodeRuneInString(line[end:])
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' || endsUnquotedValue(r) {
			break
		}
		end += w
	}
	return end
}

// mergeSpans sorts spans and joins any that overlap or touch, so a caller can
// replace them left to right without one replacement invalidating the next.
func mergeSpans(in []Span) []Span {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(a, b int) bool {
		if in[a].Start != in[b].Start {
			return in[a].Start < in[b].Start
		}
		return in[a].End < in[b].End
	})
	out := in[:1]
	for _, s := range in[1:] {
		last := &out[len(out)-1]
		if s.Start <= last.End {
			if s.End > last.End {
				last.End = s.End
			}
			continue
		}
		out = append(out, s)
	}
	return out
}
