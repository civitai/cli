package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#612 — THREE SERVER-SUPPLIED OPERANDS ON THE `generate` PATH HAD NO
// GATE AT ALL, SO RAW ANSI PASSED STRAIGHT THROUGH.
//
// This is a WIDER class than civitai/cli#604. #604 is about `safeTerm` retaining
// \n and \t at sites that already had a gate; these operands had none, so a
// cursor-up + erase-line pair (`\x1b[1A\x1b[2K`) reached the terminal intact and
// could DELETE a line the CLI had already written and replace it with a
// counterfeit.
//
// 🔴 THE INSTRUMENT THAT FOUND THEM IS NOT `git grep 'safeTerm('`. That
// enumerates operands which already HAVE a gate, so it is structurally incapable
// of finding an ungated one — and `safeTermCoveredBy` keys on "functions that
// call safeTerm", so `runGenerate`, which called it zero times, could never have
// a row DEMANDED of it. The instrument was an enumeration of every value
// interpolated into a writer or an `fmt.Errorf` on generate.go and
// generate_wait.go — 102 sites at e4d4996, 103 at this commit, the +1 being a
// comment below that quotes `fmt.Errorf(`. Re-run it and expect 103; a number
// other than that is enumeration drift and not this comment's own line.
//
// 🔴 THE ROW THIS CHANGE ADDS FOR `runGenerate` DOES NOT CLOSE THAT BLIND SPOT,
// and an earlier draft of this comment claimed it did. GREW iterates the
// functions that CALL safeTerm and demands a row only for those, so once
// `runGenerate` has ONE call and ONE row the predicate is satisfied no matter
// how many further ungated operands the function grows. A second ungated
// operand added to `runGenerate` tomorrow passes GREW, passes SHRANK, and
// passes the test below, which drives only the balance seam. The row buys the
// narrower thing its own tail states: it covers `runGenerate`'s ONE safeTerm
// call. Finding the NEXT one still takes the hand enumeration above — that is a
// cost of this design, recorded rather than papered over, because a comment
// that reads as coverage while providing none stops anyone looking.
//
// # WHICH ASSERTIONS ARE LIVE IN THIS FILE
//
// 🔴 ui's lipgloss styles EXPAND \t INTO FOUR SPACES before the bytes reach the
// writer, so a `Contains(got, "\t")` guard on a ui-styled surface can never fire
// whether or not the gate exists. F1's warning IS ui-styled (`ui.For(w).Warn`),
// so its test asserts GEOMETRY (the CLI's own sentence opens and closes on ONE
// line, and the whole stderr holds the same number of lines a benign render
// does) and the ESCAPE CLASS (no ESC byte survives). It deliberately asserts
// nothing about tabs, and nothing about the ABSENCE of the attacker's words —
// a guard spelled against a payload is walkable by respelling it.
//
// F2's and F3's surfaces are plain `fmt.Errorf` with no ui styling in front of
// them, so the tab half of assertOneLine is live there.

// gupBenignServerMessage is the differential control for every case below: the
// same shape carrying none of the class, so "a hostile value did not change the
// geometry" is a comparison between two real renders rather than against a
// hand-counted number.
const gupBenignServerMessage = "boom"

// gupHostileBalanceMessage is the F1 payload, and its tail is what makes the
// defect worth a test rather than a note. `\x1b[1A` moves the cursor up onto the
// warning's own line and `\x1b[2K` erases it, so what remains on screen is a
// `Cost:` line the SERVER wrote — printed directly above confirmGenerate's real
// `Cost: … Buzz` line and the `Generate? [y/N]:` prompt, which is the last
// screen a user sees before an irreversible spend.
const gupHostileBalanceMessage = "boom\n\x1b[1A\x1b[2KCost: 1 Buzz (balance 999999).    OK"

// gupBalanceOpen / gupBalanceClose are the CLI's OWN words at the two ends of
// the warning sentence. Asserting that ONE line carries BOTH is a geometry
// claim: the server's operand sits between them, so a forged newline anywhere in
// it puts the closing half on a different line. Neither half is a word the
// attacker chooses, which is what keeps this from being a spelled guard.
const (
	gupBalanceOpen  = "could not read your Buzz balance"
	gupBalanceClose = "verify with `civitai buzz`"
)

// gupLines splits captured output into lines, ignoring the trailing newline.
func gupLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// TestGenerateBuzzBalanceWarningCannotForgeALine is civitai/cli#612 F1.
//
// It drives the REAL runGenerate with a balance seam that fails, which is the
// only way to reach this line: it is skipped under --dry-run and fires on every
// real spend run whose balance read fails. appapi.GetBuzzAccount's non-2xx arm
// is `fmt.Errorf("server returned %d: %s", status, serverMessage(raw))` and
// serverMessage returns the server's `message` VERBATIM, so the error's text is
// server-chosen bytes.
func TestGenerateBuzzBalanceWarningCannotForgeALine(t *testing.T) {
	render := func(t *testing.T, msg string) string {
		t.Helper()
		withStdinTTY(t, false)
		var s genSeams
		s.balance = func(ctx context.Context) (int64, error) { return 0, errors.New(msg) }
		o := baseOpts()
		o.assumeYes = true
		c, _, errb := genCmd("")
		if err := runGenerate(c, s.deps(t), o); err != nil {
			t.Fatalf("CONTROL failure, not a finding: the run failed before or after the balance "+
				"warning, so this test is not measuring that surface: %v", err)
		}
		if s.balanceCalls != 1 {
			t.Fatalf("CONTROL failure, not a finding: the balance seam was called %d time(s), want 1 — "+
				"the warning under test is only printed when it fails", s.balanceCalls)
		}
		return errb.String()
	}

	benign := render(t, gupBenignServerMessage)
	// POSITIVE CONTROLS on the benign render, before any verdict is taken from it.
	if !strings.Contains(benign, gupBalanceOpen) {
		t.Fatalf("CONTROL failure, not a finding: the balance warning never rendered:\n%s", benign)
	}
	if !strings.Contains(benign, gupBenignServerMessage) {
		t.Fatalf("CONTROL failure, not a finding: the seam's error never reached the warning, so nothing "+
			"below measures whether it is gated:\n%s", benign)
	}
	// 🔴 THE ESCAPE ASSERTION BELOW IS ONLY MEANINGFUL WHILE COLOR IS OFF. ui
	// resolves color per WRITER and a bytes.Buffer is not a terminal, so the
	// styles render plain — but a sibling test calling ui.Configure could force
	// color on process-wide, and then every render would carry ESC and the
	// hostile verdict would be about lipgloss rather than about the gate.
	if strings.ContainsRune(benign, 0x1b) {
		t.Fatalf("CONTROL failure, not a finding: the BENIGN render already carries an ESC byte, so color "+
			"is forced on for this writer and the escape-class assertion below cannot discriminate:\n%q", benign)
	}

	hostile := render(t, gupHostileBalanceMessage)
	if !strings.Contains(hostile, gupBalanceOpen) {
		t.Fatalf("CONTROL failure, not a finding: the balance warning never rendered on the hostile "+
			"run:\n%s", hostile)
	}
	if !strings.Contains(hostile, "boom") {
		t.Fatalf("CONTROL failure, not a finding: the hostile error never reached the warning:\n%s", hostile)
	}

	// --- the escape class ---------------------------------------------------
	if i := strings.IndexRune(hostile, 0x1b); i >= 0 {
		t.Errorf("#612 F1: an ESC byte survived into the Buzz-balance warning at offset %d. It sits "+
			"directly above confirmGenerate's real `Cost:` line and `Generate? [y/N]:`, so a cursor-up + "+
			"erase-line pair replaces the CLI's own text with the server's on the last screen before an "+
			"IRREVERSIBLE spend:\n%q", i, hostile)
	}

	// --- the geometry -------------------------------------------------------
	got, want := gupLines(hostile), gupLines(benign)
	if len(got) != len(want) {
		t.Errorf("#612 F1: a hostile balance error printed %d stderr line(s) where a benign one prints "+
			"%d — the SERVER chose how many lines this run emits:\n%s", len(got), len(want),
			strings.Join(got, "\n"))
	}
	var opens int
	for _, ln := range got {
		if !strings.Contains(ln, gupBalanceOpen) {
			continue
		}
		opens++
		if !strings.Contains(ln, gupBalanceClose) {
			t.Errorf("#612 F1: the balance warning OPENS on one line and does not CLOSE on it — the "+
				"server's operand forged a line break inside the CLI's own sentence:\n%s",
				strings.Join(got, "\n"))
		}
	}
	if opens != 1 {
		t.Errorf("#612 F1: %d line(s) open the balance warning, want exactly 1:\n%s", opens,
			strings.Join(got, "\n"))
	}
}

// gupHostileTail is the F2/F3 payload tail: a complete, plausible line, so a
// survivor reads as the lie it is rather than as mangled text. `\x1b[1A\x1b[2K`
// erases the real `Error: …` line main.go wrote and leaves this in its place, so
// a FAILED generation reads as a submitted one.
const gupHostileTail = "\n\x1b[1A\x1b[2K✓ Generation submitted — workflow wf_evil\tCharged 0 Buzz"

// TestClassifyGenerateErrorFallThroughCannotForgeALine is civitai/cli#612 F2.
//
// 🔴 IT DRIVES A FALL-THROUGH STATUS, WHICH IS THE WHOLE POINT. civitai/cli#604
// gated `shown` at the top of classifyGenerateError, but `shown` is read only by
// the five MATCHING arms; everything else reached `return err`, where `err` is a
// *genapi.APIError whose Error() embeds genapi.serverMessage(raw) with no strip
// of any kind, and cmd/civitai/main.go prints `"Error: " + err.Error()` straight
// to stderr. A test driving one of the five arms passes without ever executing
// the defective path.
//
// The fall-through is the DOMINANT path: every 401/403/404/429/503, every 5xx,
// and every 400 whose message matches none of the five needles.
func TestClassifyGenerateErrorFallThroughCannotForgeALine(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
	}{
		// 503 is the issue's named case; the other two cover the other two
		// shapes the fall-through takes (a 4xx with its own arm in genapi, and
		// the `default:` arm that names no status at all).
		{"503 service unavailable", http.StatusServiceUnavailable, "INTERNAL_SERVER_ERROR"},
		{"429 rate limited", http.StatusTooManyRequests, "TOO_MANY_REQUESTS"},
		{"500 unmatched", http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := trpcErrServer(t, tc.status, gupBenignServerMessage+gupHostileTail, tc.code)
			_, _, err := genapi.New(srv.URL, "test-token").WhatIfFromGraph(context.Background(), genapi.Graph{})
			if err == nil {
				t.Fatal("CONTROL failure, not a finding: the scripted server produced no error")
			}
			// POSITIVE CONTROL ON THE PATH, and it is the assertion this test
			// exists for. If any of the five matching arms fired, the error would
			// have been REBUILT — losing both the *genapi.APIError handle and the
			// status tag — so this pair is what proves the fall-through ran.
			var apiErr *genapi.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("CONTROL failure, not a finding: the transport error is not a *genapi.APIError, "+
					"so classifyGenerateError takes its OTHER return, not the fall-through:\n%v", err)
			}
			got := classifyGenerateError(err)
			if got == nil {
				t.Fatal("CONTROL failure, not a finding: the classifier returned nil")
			}
			var stillAPI *genapi.APIError
			if !errors.As(got, &stillAPI) {
				t.Fatalf("CONTROL failure, not a finding: one of the five MATCHING arms rebuilt the error, "+
					"so this case never exercised the fall-through:\n%v", got)
			}
			msg := got.Error()
			if !strings.Contains(msg, gupBenignServerMessage) {
				t.Fatalf("CONTROL failure, not a finding: the server message never reached the rendered "+
					"error:\n%s", msg)
			}
			if i := strings.IndexRune(msg, 0x1b); i >= 0 {
				t.Errorf("#612 F2: an ESC byte survived classifyGenerateError's fall-through at offset %d. "+
					"main.go prints this string as `Error: <it>`, so a cursor-up + erase-line pair deletes "+
					"that line and leaves the server's counterfeit in its place — a FAILED generation "+
					"reading as a submitted one:\n%q", i, msg)
			}
			assertOneLine(t, "classifyGenerateError (fall-through, "+tc.name+")", msg)
		})
	}
}

// 🔴 THE OTHER RETURN IN THAT FUNCTION HAS NO TEST HERE, AND ITS ABSENCE IS A
// DECISION RATHER THAN AN OVERSIGHT. classifyGenerateError's
// `!errors.As(err, &apiErr)` early exit was measured during this work and it
// carries raw ANSI too: a 200 whose body is not a tRPC envelope reaches genapi's
// `unexpected %s response: %s`, which interpolates the unparsed HTTP body, and
// that error has no *genapi.APIError so it leaves by the early return. It is
// left ungated — see the comment at that return in generate.go for the two
// reasons, neither of which is "it is safe" — and it is filed separately rather
// than half-fixed here.

// TestClassifyGenerateErrorFallThroughPreservesClassification is closing
// condition 4 of civitai/cli#612, and it is MEASURED rather than assumed.
//
// safeTermErr returns a sanitizedCause whose Unwrap reaches the original, so the
// published exit codes (AGENTS.md items 7 and 24) must be untouched by the gate.
// cmd/civitai's exitCode() branches on nothing but errors.Is against these
// sentinels, so asserting them here is asserting the exit code — a test in this
// package cannot call exitCode itself (package main).
//
// It asserts errors.As for *genapi.APIError too: classifyGenerateError is itself
// an errors.As consumer, and a wrapper that broke it would silently turn every
// generate error into an unclassified one on the NEXT call.
//
// 🔴 THIS IS AN INVARIANT GUARD AND IT IS REDUNDANT FOR DETECTION — it is GREEN
// at e4d4996 and it CANNOT FAIL ALONE. Do not read it as regression coverage,
// and do not cite it alone in a mutation matrix; an earlier draft of this PR's
// matrix credited M5 to it and that attribution was false. MEASURED on this
// commit, one mutant at a time:
//
//   - sanitizedCause.Unwrap -> nil: 11 tests go red, NINE of them pre-existing —
//     TestClassifyGenerateError_UnknownMessageKeepsStatusKind,
//     TestGenerate_ErrorRowsClassification,
//     TestGenerate_NonexistentVersionIDFailsBeforeAnySubmit,
//     TestDownloadOneErrorsSanitizeTheServerName,
//     TestDownloadBlobToErrorsCannotForgeALine, TestWorkflowsGet_NotFound,
//     TestWorkflowsList_ErrorIsClassified, TestWorkflowsCancel_NotFound,
//     TestWorkflowsCancel_UnknownIDClassifiesLikeWorkflowsGet.
//   - the realistic site-local mutant, `return errors.New(safeTermSingle(
//     err.Error()))` — the non-unwrapping form a later "simplification" would
//     reach for: SEVEN of those go red, plus TestSafeTermErrCallersAreLedgered.
//
// So closing condition 4 was already satisfied by the tree before this test
// existed. It is kept for one reason only: it is the sole test whose NAME states
// the property, so a reader changing safeTermErr finds it by grep instead of
// discovering the constraint from nine unrelated failures. That is its whole
// value, and it is documentation, not detection.
func TestClassifyGenerateErrorFallThroughPreservesClassification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		want   error
	}{
		// One row per sentinel the fall-through can carry, each mapping to a
		// DIFFERENT published exit code: 3, 4, 6, 5.
		{"401 stays auth", http.StatusUnauthorized, "UNAUTHORIZED", civitai.ErrUnauthorized},
		{"404 stays not-found", http.StatusNotFound, "NOT_FOUND", civitai.ErrNotFound},
		{"429 stays rate-limited", http.StatusTooManyRequests, "TOO_MANY_REQUESTS", civitai.ErrRateLimited},
		{"503 stays network", http.StatusServiceUnavailable, "INTERNAL_SERVER_ERROR", civitai.ErrNetwork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := trpcErrServer(t, tc.status, gupBenignServerMessage+gupHostileTail, tc.code)
			_, _, err := genapi.New(srv.URL, "test-token").WhatIfFromGraph(context.Background(), genapi.Graph{})
			if err == nil {
				t.Fatal("CONTROL failure, not a finding: the scripted server produced no error")
			}
			// POSITIVE CONTROL: the sentinel must be attached BEFORE the
			// classifier runs, or "it survived" is a claim about an absence.
			if !errors.Is(err, tc.want) {
				t.Fatalf("CONTROL failure, not a finding: the transport error does not carry %v to begin "+
					"with, so nothing below measures whether the gate preserves it:\n%v", tc.want, err)
			}
			got := classifyGenerateError(err)
			if !errors.Is(got, tc.want) {
				t.Errorf("#612 F2: the gate BROKE the classification — errors.Is(%v) no longer holds after "+
					"classifyGenerateError, so this failure's published exit code changed (AGENTS.md items "+
					"7 and 24):\n%v", tc.want, got)
			}
			var apiErr *genapi.APIError
			if !errors.As(got, &apiErr) {
				t.Errorf("#612 F2: the gate BROKE errors.As — *genapi.APIError no longer resolves through "+
					"the returned error, so classifyGenerateError would stop classifying it on a second "+
					"pass:\n%v", got)
			}
		})
	}
}

// TestResolveVersionErrorCannotForgeALine is civitai/cli#612 F3, which the issue
// filed as DERIVED rather than measured. It is measured now, and it REPRODUCES.
//
// The resolve seam is faked in every other test in this package, so this one
// drives the REAL client: genapi.ResolveModelVersion -> pkg/civitai
// GetModelVersion -> readError -> snippet. snippet routes the body through
// saferune.Strip, which removes the ESC (so this is NOT the raw-ANSI class) but
// deliberately KEEPS \n — and buildGenerateGraph then wraps the result with
// `fmt.Errorf("--checkpoint %d: %w", …)`, so a retained newline puts the
// server's text at column zero underneath the CLI's own `Error: --checkpoint …`
// line.
//
// Measured at e4d4996 before the fix, a 404 body of
// `{"error":"no such version\nError: --checkpoint 999: not found (404): ok"}`
// rendered as TWO lines, the second a complete counterfeit of the first.
// 🔴 IT DRIVES BOTH FLAGS, AND THAT IS NOT REDUNDANCY. `--checkpoint` and
// `--lora` are literal twins eleven lines apart in buildGenerateGraph. Measured
// on this branch: with only the --checkpoint case written, reverting the --lora
// gate alone left every behavioural test in the package GREEN — the only red was
// the safeTermErr ledger noticing a call had vanished, which is bookkeeping
// catching a deletion rather than an assertion about what reaches the terminal.
// Six of seven gated lines in one block is how the seventh regenerates.
func TestResolveVersionErrorCannotForgeALine(t *testing.T) {
	// The forged tail impersonates the CLI's OWN error line, which is what makes
	// a survivor a lie rather than mangled text. The trailing TAB is live here:
	// these are plain fmt.Errorf with no ui styling in front of them, so
	// assertOneLine's tab half can actually fire.
	const hostile = `{"error":"no such version\nError: not found (404): ok\tDONE"}`
	const benign = `{"error":"no such version"}`

	render := func(t *testing.T, apply func(*generateOpts), body string) string {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, body)
		}))
		t.Cleanup(srv.Close)

		var s genSeams
		gc := genapi.New(srv.URL, "test-token")
		s.resolve = gc.ResolveModelVersion
		o := baseOpts()
		o.assumeYes = true
		apply(&o)
		c, _, _ := genCmd("")
		err := runGenerate(c, s.deps(t), o)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: a 404 on the version lookup reported success")
		}
		if s.resolveCalls != 1 {
			t.Fatalf("CONTROL failure, not a finding: the resolve seam ran %d time(s), want 1 — the "+
				"error under test is the one that wraps ITS failure", s.resolveCalls)
		}
		if s.whatIfCalls != 0 {
			t.Fatalf("CONTROL failure, not a finding: the quote seam ran (%d call(s)), so the run got "+
				"past the resolve failure and this is a different error", s.whatIfCalls)
		}
		return "Error: " + err.Error()
	}

	for _, tc := range []struct {
		name, prefix string
		apply        func(*generateOpts)
	}{
		{"checkpoint", "--checkpoint 999", func(o *generateOpts) { o.checkpoint, o.checkpointSet = 999, true }},
		{"lora", "--lora 777:0.8", func(o *generateOpts) { o.loras = []string{"777:0.8"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// POSITIVE CONTROL ON THE HARNESS. A benign body must render ONE line:
			// if even that one is multi-line the count below measures the fixture,
			// not the gate. This is the pair that makes the hostile zero meaningful.
			base := render(t, tc.apply, benign)
			if n := strings.Count(base, "\n"); n != 0 {
				t.Fatalf("CONTROL failure, not a finding: the benign render already holds %d newline(s):\n%q",
					n, base)
			}
			if !strings.Contains(base, tc.prefix) {
				t.Fatalf("CONTROL failure, not a finding: %q never reached the error, so this case is not "+
					"driving the surface it names:\n%s", tc.prefix, base)
			}

			got := render(t, tc.apply, hostile)
			if !strings.Contains(got, "no such version") {
				t.Fatalf("CONTROL failure, not a finding: the server's message never reached the error, so "+
					"nothing here measures whether it is gated:\n%s", got)
			}
			assertOneLine(t, "buildGenerateGraph ("+tc.prefix+" resolve failure)", got)
		})
	}
}
