package validate

import (
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// finding.go defines the ONE shape every validation finding has, and the ONE
// notation every finding names its location in.
//
// WHY THIS IS A STRUCT AND NOT A STRING (issue #225). `--json` promised
// `errors`/`warnings` each with `field`/`message`, and the field was
// reverse-engineered at the PRINTER by string-parsing a schema-style
// "<path>: <reason>" prefix. The schema errors parsed; the SEMANTIC checks —
// the ported BlockManifestValidator rules of AGENTS.md item 1, i.e. exactly the
// findings a local pre-check exists for — emit prose, so 4 of 7 findings on a
// six-ways-broken manifest came back `field: null`. Grouping findings by field
// in CI, the one thing `--json` is for, did not work for the findings that
// matter most.
//
// 🔴 The fix has to be that a finding CARRIES its field from where it is
// PRODUCED. Adding more prefix heuristics to the printer regenerates the same
// bug at every check added afterwards: a new check emits prose, the parser
// finds no path, and the null is back with nothing failing. That is why every
// check function in this package returns []Finding rather than []string.

// Finding is a single validation error or warning.
type Finding struct {
	// Field is the manifest location this finding is about, in DOTTED notation
	// (see the notation contract below). It is NEVER empty: a finding with no
	// single natural field carries one of the two documented sentinels.
	Field string

	// Message is the COMPLETE human-readable line — byte-for-byte what the text
	// renderer prints. It is deliberately not a reason fragment to be composed
	// with Field: the semantic messages name their field inside prose
	// ("iframe.sandbox MUST NOT combine …"), so composing would stutter, and
	// the schema messages already carry a "<field>: <reason>" shape.
	Message string
}

// ---------------------------------------------------------------------------
// The notation contract.
// ---------------------------------------------------------------------------
//
// Findings used to mix THREE notations: `(root)` and JSON Pointer `/blockId`
// in `--json`, and dotted `iframe.sandbox` in the human-readable text. They are
// unified on DOTTED — `iframe.sandbox`, `scopes[1]`, `targets[0].slotId`.
//
// Dotted rather than JSON Pointer because dotted is what every other surface
// already speaks: the ported semantic messages ("iframe.minHeight is required",
// "page.buzzBudgetPerGen is not set"), the README, AGENTS.md, and the schema's
// own descriptions. Unifying on JSON Pointer would have meant rewriting every
// prose message to `/iframe/sandbox` — a far larger change to the output
// authors read, to buy an escaping rigour a manifest whose keys are all
// identifiers does not need. Dotted is also what an author types to find the
// key in their file.
//
// Residuals, stated rather than hidden. A member name containing `.` or `[`
// renders ambiguously (`scopeJustifications` keys are author-supplied; a key
// "a.b" renders as `scopeJustifications.a.b`), and a numeric OBJECT key renders
// like an array index. Neither is reachable from this schema's own fields — the
// manifest's keys are fixed identifiers and scope names use `:`, not `.` — and
// a consumer grouping by field is unharmed by either.

const (
	// FieldDocument is the Field of a finding about the manifest AS A WHOLE:
	// the manifest is absent or unparseable, or the schema reports a violation
	// at the document root (a missing top-level required property is located at
	// the object that lacks it, not at the key that is not there).
	//
	// It is the literal the previous JSON output already used for schema root
	// errors, kept so existing consumers keep grouping the same way.
	FieldDocument = "(root)"

	// FieldProject is the Field of a finding about PROJECT STATE OUTSIDE the
	// manifest — the committed lockfile, the source tree. These are real
	// findings with no manifest field to name: the lockfile check reports on a
	// file that is or is not committed, and the ready-ack advisory reports on
	// what the browser loads.
	//
	// 🔴 It is a SECOND sentinel rather than reusing FieldDocument on purpose.
	// Both would be "not a manifest field", but they are not the same kind of
	// finding and a consumer routes them differently: `(root)` says "your
	// manifest document is wrong", `(project)` says "your repository state is
	// wrong, and no manifest edit alone fixes it". Collapsing them would tell a
	// CI job to go looking in block.manifest.json for a missing lockfile.
	//
	// Some of these findings DO offer a manifest edit as one remedy — the
	// lockfile error suggests setting `buildCommand` to match the lockfile you
	// already have — but the finding is not ABOUT that field, and pinning it
	// there would mis-group every project that takes the other remedy.
	FieldProject = "(project)"
)

// newFinding builds a Finding. 🔴 It is the ONLY constructor: every finding in
// this package goes through it, so "does this check carry a field?" is a
// question about ONE call rather than about a struct literal somewhere.
//
// TestFindingsAreConstructedWithAField enforces that structurally — it parses
// every non-test file in the package, rejects a bare `Finding{…}` composite
// literal, and rejects a `newFinding("" , …)`. That is the guard that a NEWLY
// ADDED check cannot ship without a field: it fires on the construction site
// itself, so it does not depend on the new check being exercised by any test.
func newFinding(field, message string) Finding {
	if h := findingSiteHook; h != nil {
		recordFindingSite(h)
	}
	return Finding{Field: field, Message: message}
}

// findingSiteHook is a TEST SEAM, nil in every production run: one nil compare
// per finding, and `recordFindingSite` (the only thing that pays for a
// `runtime.Caller`) is not entered unless a test installed it.
//
// 🔴 It exists because a corpus test that only asserts "every finding I saw
// carried a field" cannot distinguish a thorough corpus from one that never
// reached the check in question — the reassuring-zero shape. With the hook,
// TestEveryCheckEmitsAField compares the finding-producing functions the corpus
// ACTUALLY REACHED against the set the AST finds in the source, so a newly
// added check with no fixture fails loudly instead of passing unobserved.
//
// A test that installs it must not run in parallel with another that does.
var findingSiteHook func(fn string)

func recordFindingSite(h func(fn string)) {
	pc, _, _, ok := runtime.Caller(2) // 0=recordFindingSite, 1=newFinding, 2=the check
	if !ok {
		return
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		return
	}
	h(f.Name())
}

// Messages returns just the human-readable lines, in order. Callers that render
// text (the validate/submit printers, `app init`'s self-check) want this;
// callers that render JSON want the Findings themselves.
func Messages(fs []Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Message
	}
	return out
}

// sortFindings orders findings by MESSAGE first, Field second.
//
// Message-first is deliberate: the text output prints messages and nothing
// else, so ordering on the message keeps that output's order exactly what it
// was when findings were bare strings. Field is the tiebreak so the order is
// total (two checks can legitimately produce the same sentence about two
// different fields — see dedupeFindings).
func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Message != fs[j].Message {
			return fs[i].Message < fs[j].Message
		}
		return fs[i].Field < fs[j].Field
	})
}

// dedupeFindings collapses findings that are identical in BOTH field and
// message, and only those.
//
// The semantics are the point, so they are stated: two findings with the same
// MESSAGE and different FIELDS are two findings and both survive (the
// scopeJustifications key rule emits one sentence per offending key, and a
// per-key field is the only thing that distinguishes them once the key is
// interpolated out of the sentence — which it is not today, but a message that
// stopped naming the key would silently collapse into one). Two findings with
// the same FIELD and different messages are likewise both kept: an iframe can
// be wrong in several ways at once. Only the exact pair collapses, which is
// what the string dedupe did before, restricted to the axis that used to be the
// whole value.
func dedupeFindings(in []Finding) []Finding {
	seen := make(map[Finding]struct{}, len(in))
	var out []Finding
	for _, f := range in {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

// fieldPath renders a schema InstanceLocation (raw, UNESCAPED JSON Pointer
// tokens — the jsonschema library escapes only when it prints) as a dotted
// path. An empty location is the document itself.
//
// A token that is entirely ASCII digits is rendered as an array subscript,
// which is what it is everywhere in this schema (`scopes`, `targets`,
// `permissions` are the only arrays and none holds objects with numeric keys).
func fieldPath(tokens []string) string {
	if len(tokens) == 0 {
		return FieldDocument
	}
	var b strings.Builder
	for i, tok := range tokens {
		if isIndexToken(tok) {
			b.WriteByte('[')
			b.WriteString(tok)
			b.WriteByte(']')
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(tok)
	}
	// A location that starts with an index (impossible for an object-rooted
	// manifest, but not for a hypothetical array document) would render
	// "[0]…"; that is still a usable path, so no special case.
	return b.String()
}

func isIndexToken(tok string) bool {
	if tok == "" {
		return false
	}
	for i := 0; i < len(tok); i++ {
		if tok[i] < '0' || tok[i] > '9' {
			return false
		}
	}
	return true
}

// childField joins a parent path with a member name: childField("iframe",
// "sandbox") is "iframe.sandbox". Kept as a function so the notation lives in
// one place rather than in a dozen string concatenations.
func childField(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// indexField renders an element of an array field: indexField("targets", 0) is
// "targets[0]".
func indexField(parent string, i int) string {
	return parent + "[" + strconv.Itoa(i) + "]"
}
