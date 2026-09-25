package appapi

import (
	"errors"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// 🔴 THIS FILE REPLACES FOUR DRAFTS OF A STATIC GUARD, AND THE REASON IS THE
// FOUR DRAFTS.
//
// The invariant is: every 401 this package returns is a string unauthorizedError
// produced. Four attempts tried to establish that by ANALYSING THE SOURCE, and an
// audit round refuted each one — every time by finding a spelling the analyser
// could not see, and every time the analyser's own comment had denied having a
// blind spot:
//
//	draft 1  strings.Contains(src, "unauthorizedError(")   missed 4 arms in a converted file
//	draft 2  regex on `case http.StatusUnauthorized:`      missed `case A, B:` and branching bodies
//	draft 3  go/parser over *ast.CaseClause                missed `if`, `switch{case expr}`, bare `401`
//	draft 4  go/parser keyed on message CONSTRUCTION       missed `st == 401 || st == 403`, and
//	                                                       false-positived on `if st != 401 { … }`
//
// Draft 4 was the one that was supposed to end the class by keying on what the
// invariant is about rather than on syntax. It moved the syntax dependency down
// one level instead — from which STATEMENT encloses the 401 to which EXPRESSION
// tests it — because `testsUnauthorized` recursed on `==`/`!=` and stopped at
// `&&`/`||`. Round 3 proved it with three surviving mutants, one of which is
// `withdrawError`'s real historical shape re-spelled as an `if`.
//
// # So this draft does not analyse the source at all
//
// It DRIVES every status mapper in the package with a 401 and compares what they
// return. That is the user-visible invariant, stated directly, with no pattern to
// be blind in: a mapper that answers 401 with its own wording fails here whatever
// syntax it used, because the test reads the OUTPUT.
//
// # What it does NOT cover, stated rather than denied
//
// A mapper missing from every401Mapper below is not driven, and TestMapperSetIsComplete
// is what makes that loud — via ONE regex with ONE job (a `func …Error(status int`
// signature at line start). A mapper that does not match that naming, or that takes
// its status second, is invisible to it. That is a real gap. It is narrow, it is
// nameable, and unlike the four drafts above it is written down here instead of
// being denied two lines from code that contains it.

// every401Mapper is every status→error mapper in this package, adapted to one
// signature so it can be driven uniformly. The two adapters supply arguments
// that do not affect the 401 arm.
var every401Mapper = map[string]func(int, []byte) error{
	"analyticsError":     analyticsError,
	"cloneInfoError":     cloneInfoError,
	"devTokenError":      devTokenError,
	"devTunnelError":     devTunnelError,
	"myAppManifestError": myAppManifestError,
	"serverError":        serverError,
	"withdrawError":      withdrawError,
	"listingError":       func(s int, raw []byte) error { return listingError(s, raw, listingRoute{}) },
	"submissionsError":   func(s int, raw []byte) error { return submissionsError(s, raw, "", "") },
}

// TestEveryMapperAnswers401WithTheHelperText is the seam guard, behavioural.
//
// It accepts EITHER shape the helper produces — with the server's words or
// without — because dropping them is a per-route judgement (`devTunnelError`
// drops a misleading origin-gate string and says why). What it does not accept is
// a third string.
func TestEveryMapperAnswers401WithTheHelperText(t *testing.T) {
	// 🔴 THE SERVER'S WORDS ARE THE ONE FREE PART, AND THAT IS NOT A WEAKENING.
	// A first draft of this test compared the WHOLE string against
	// unauthorizedError(serverMsg) and four mappers failed — not because their
	// wording diverged but because they extract the server message differently
	// (`analyticsError` and `listingError` unwrap the tRPC `error.json.message`
	// envelope; `devTokenError`, `serverError`, `submissionsError` and
	// `withdrawError` take `serverMessage(raw)`, which does not). That is a real
	// and deliberate difference about the SERVER's text, and reading it as a
	// wording divergence would have been an instrument fault reported as a defect.
	//
	// So the assertion pins everything the CLI writes — the prefix, the join, and
	// the remedy verbatim — and leaves the middle free. The control below proves
	// the middle really is the server's words and not a constant the pattern would
	// also accept.
	const prefix = "not logged in (401)"
	bare := unauthorizedError("").Error()

	bodyA := []byte(`{"error":{"json":{"message":"token no longer valid"}}}`)
	bodyB := []byte(`{"error":{"json":{"message":"a completely different refusal"}}}`)

	names := make([]string, 0, len(every401Mapper))
	for name := range every401Mapper {
		names = append(names, name)
	}
	sort.Strings(names)

	shaped := func(s string) bool {
		if s == bare {
			return true
		}
		return strings.HasPrefix(s, prefix+": ") && strings.HasSuffix(s, " — "+unauthorizedRemedy)
	}

	varies := 0
	for _, name := range names {
		got := every401Mapper[name](http.StatusUnauthorized, bodyA)
		if got == nil {
			t.Errorf("%s returned nil for a 401", name)
			continue
		}
		if s := got.Error(); !shaped(s) {
			t.Errorf("%s answers a 401 with its own wording:\n  got  %q\n\nEvery 401 in this package "+
				"returns unauthorizedError, so a user reads the same two remedies whichever route they "+
				"were on. Before that helper existed FOUR spellings had drifted apart — one dropped the "+
				"CIVITAI_TOKEN route for the command group most likely to run in CI, one said \"check "+
				"your token\", one said \"check your API key / Apps invite\". None names a command "+
				"anyone can run.\n\nExpected either %q, or %q + \": <the server's words> — \" + the "+
				"same remedy.", name, s, bare, prefix)
			continue
		}
		// 🔴 The tag the published exit code depends on, PER MAPPER: each owns its
		// own `defer civitai.TagStatus`, so one can lose it while the rest keep it.
		if !errors.Is(got, civitai.ErrUnauthorized) {
			t.Errorf("%s's 401 is not tagged civitai.ErrUnauthorized, so the CLI-wide exit-code "+
				"classifier will not see it as an auth failure: %v", name, got)
		}
		// 🔴 CONTROL, per mapper: feed different server words and require the
		// output to MOVE. Without this, `shaped` is satisfied by a mapper that
		// hardcodes a constant in the middle — which is precisely the divergence
		// this test exists to catch, wearing the right prefix and suffix.
		other := every401Mapper[name](http.StatusUnauthorized, bodyB)
		if other != nil && other.Error() != got.Error() {
			varies++
		} else if got.Error() != bare {
			t.Errorf("%s produced the SAME 401 text for two different server messages, but its output "+
				"is not the no-server-message shape. The middle of its message is a constant, so the "+
				"shape check above cannot tell it from the helper's:\n  %q", name, got)
		}
	}

	// A floor on the control itself: at least the mappers that DO echo the server
	// must have moved, or every per-mapper control above passed vacuously.
	if varies < 4 {
		t.Fatalf("CONTROL failure: only %d mapper(s) changed their 401 text when the server's words "+
			"changed. Most of these echo the server; if almost none moved, the fixtures are not "+
			"reaching the message and every verdict above is about text this test did not vary.", varies)
	}
}

// TestMapperSetIsComplete is the anti-vacuity half: every401Mapper must name
// every mapper the package defines.
//
// 🔴 WITHOUT THIS THE TEST ABOVE IS A LEDGER THAT SILENTLY SHRINKS. A new route
// whose mapper nobody added to the map is exactly the case the whole exercise is
// about — it is how the fifth, sixth and seventh spellings of a 401 got written
// in the first place.
var mapperSignature = regexp.MustCompile(`(?m)^func (\w+Error)\(status int`)

func TestMapperSetIsComplete(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	found := map[string]string{}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		for _, m := range mapperSignature.FindAllStringSubmatch(string(src), -1) {
			found[m[1]] = name
		}
	}
	if scanned < 5 {
		t.Fatalf("CONTROL failure: scanned only %d non-test .go files — wrong tree", scanned)
	}
	// Positive control on the REGEX: it has one job and this proves it does it.
	// A pattern that matched nothing would report every mapper as absent from the
	// source (loud) but would also report the map as complete (silent).
	if len(found) < len(every401Mapper) {
		t.Fatalf("CONTROL failure: the signature regex found only %d mapper(s) in %d file(s), "+
			"fewer than the %d already in every401Mapper. It is not matching `func …Error(status int` "+
			"— so the missing/extra verdicts below are about the regex, not the package.",
			len(found), scanned, len(every401Mapper))
	}

	var missing, extra []string
	for fn, file := range found {
		if _, ok := every401Mapper[fn]; !ok {
			missing = append(missing, fn+"  ("+file+")")
		}
	}
	for fn := range every401Mapper {
		if _, ok := found[fn]; !ok {
			extra = append(extra, fn)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("%d status mapper(s) are not in every401Mapper, so nothing checks what they answer "+
			"a 401 with:\n    %s\n\nAdd them to the map. If a mapper genuinely cannot 401, add it "+
			"anyway — driving it costs one line and proves the claim.", len(missing), strings.Join(missing, "\n    "))
	}
	if len(extra) > 0 {
		t.Errorf("every401Mapper names %d function(s) the package no longer defines: %v\n\n"+
			"A ledger entry for code that is gone is a rule nobody can check.", len(extra), extra)
	}
}
