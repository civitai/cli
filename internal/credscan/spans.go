package credscan

import (
	"sort"
	"strings"
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
		raw := line[rawStart:rawEnd]
		if value, quoted := cleanValue(raw); valueLooksLikeSecret(value, quoted) {
			// cleanValue only ever trims and unquotes, so the value it returns is
			// a substring of the raw capture; locating it there recovers the
			// offset without a second copy of the cleaning rules.
			if i := strings.Index(raw, value); i >= 0 {
				out = append(out, Span{rawStart + i, rawStart + i + len(value)})
			}
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
