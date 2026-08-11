package cmd

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// Coverage for issue #350: `civitai app withdraw` destroyed the app's entire
// store listing — icon, cover and every captioned screenshot — with no warning,
// no prompt and rc=0.
//
// 🔴 WHO DELETES, TRACED IN `civitai/civitai` RATHER THAN GUESSED, because the
// answer decided the fix. The CLI does NOT issue the destructive call: the
// withdraw route's body schema has exactly one field, `publishRequestId`
// (src/pages/api/v1/blocks/withdraw.ts:78-83), and the tRPC spelling shares it.
// The delete is `withdrawRequest` → `deleteOnsiteDraftListingForSlug`
// (src/server/services/blocks/publish-request.service.ts:1543 → :1454-1459), a
// deliberate application-level `deleteMany`, NOT a Prisma cascade —
// `AppBlockPublishRequest` has no relation to `AppListing` at all. So the CLI
// cannot avoid the destruction, and the whole fix is: stop being surprising.
//
// 🔴 NOT ONE OF THESE TESTS PERFORMS A REAL WITHDRAWAL. Every case runs against
// httptest. A real withdraw destroys a listing and permanently consumes a
// blockId, so the seam that must never be exercised for real is exactly the one
// several of these tests assert was NEVER CALLED — which is why they count
// requests against a local server instead of asserting on output alone.

// countingWithdrawServer returns an httptest server for the withdraw route plus
// the counter of requests it received.
//
// The counter is the point. "Nothing was destroyed" is the claim every refusal
// path in this file makes, and an assertion on the error message cannot
// establish it: a command that printed a refusal AND fired the request would
// pass that assertion perfectly. Counting hits makes the claim structural.
func countingWithdrawServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// runWithdrawStdin drives the real root command with stdin wired to `stdin`,
// capturing stdout and stderr separately (the confirmation writes to stderr).
func runWithdrawStdin(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	// 🔴 Execute FIRST, then read the buffers. Go evaluates return operands left
	// to right, so `return out.String(), errb.String(), root.Execute()` captures
	// both buffers BEFORE the command has written anything — two empty strings
	// and a correct error, which reads exactly like a command that printed
	// nothing. Cost an assertion round here; keep it explicit.
	err := root.Execute()
	return out.String(), errb.String(), err
}

// withdrawEnv points the CLI at srv with a token configured.
func withdrawEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", baseURL)
}

// --- confirmWithdraw unit tests (the safety logic in isolation) ---

// --yes proceeds regardless of TTY, and prints NOTHING — a scripted run that
// emitted a prompt into its own stderr would be a new surprise of its own.
func TestConfirmWithdraw_YesBypassesPrompt(t *testing.T) {
	withStdinTTY(t, false)
	c, out := newConfirmCmd("")
	if err := confirmWithdraw(c, "pubreq_01H", true); err != nil {
		t.Fatalf("--yes should proceed: %v", err)
	}
	if out.String() != "" {
		t.Errorf("--yes must not prompt, got %q", out.String())
	}
}

// A non-interactive shell without --yes REFUSES. The message has to carry the
// thing the user did not know, so it is asserted against the constant the help
// and the prompt share rather than a hand-typed paraphrase.
func TestConfirmWithdraw_NonTTYRefuses(t *testing.T) {
	withStdinTTY(t, false)
	c, out := newConfirmCmd("")
	err := confirmWithdraw(c, "pubreq_01H", false)
	if err == nil {
		t.Fatal("a non-TTY withdraw without --yes must refuse")
	}
	if !strings.Contains(err.Error(), "refusing to withdraw without --yes") {
		t.Errorf("refusal should be recognisable: %v", err)
	}
	if !strings.Contains(err.Error(), withdrawDiscardsListing) {
		t.Errorf("refusal must say WHAT is destroyed (the shared constant), got: %v", err)
	}
	if !strings.Contains(err.Error(), "pubreq_01H") {
		t.Errorf("refusal should name the target: %v", err)
	}
	if out.String() != "" {
		t.Errorf("a refusal must not also print a prompt: %q", out.String())
	}
}

// 🔴 The prompt DEFAULTS TO NO. Every answer that is not an explicit yes aborts,
// including a bare Enter — the case a user hits by reflex. If this ever
// enumerated the REFUSING answers instead, `[y/N]` would silently become
// `[Y/n]` and a stray keystroke would delete a listing.
func TestConfirmWithdraw_TTYDeclineAborts(t *testing.T) {
	for _, answer := range []string{"", "\n", "n\n", "N\n", "no\n", "  \n", "nope\n", "yes please\n", "q\n"} {
		t.Run(strings.TrimSpace(answer)+"|", func(t *testing.T) {
			withStdinTTY(t, true)
			c, out := newConfirmCmd(answer)
			err := confirmWithdraw(c, "pubreq_01H", false)
			if err == nil {
				t.Fatalf("answer %q must abort", answer)
			}
			if !strings.Contains(err.Error(), "withdraw aborted") {
				t.Errorf("answer %q: want an abort error, got %v", answer, err)
			}
			// The prompt must have named the loss before asking.
			if !strings.Contains(out.String(), "Withdraw it? [y/N]:") {
				t.Errorf("prompt missing or re-defaulted: %q", out.String())
			}
		})
	}
}

func TestConfirmWithdraw_TTYAcceptProceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  y  \n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			withStdinTTY(t, true)
			c, out := newConfirmCmd(answer)
			if err := confirmWithdraw(c, "pubreq_01H", false); err != nil {
				t.Fatalf("answer %q should proceed: %v", answer, err)
			}
			if !strings.Contains(out.String(), withdrawDiscardsListing) {
				t.Errorf("the prompt must state what is destroyed BEFORE asking: %q", out.String())
			}
		})
	}
}

// The confirmation goes to STDERR, so stdout stays machine-clean.
func TestConfirmWithdraw_PromptGoesToStderr(t *testing.T) {
	withStdinTTY(t, true)
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader("n\n"))
	if err := confirmWithdraw(root, "pubreq_01H", false); err == nil {
		t.Fatal("expected the decline to abort")
	}
	if out.String() != "" {
		t.Errorf("the prompt must not touch stdout, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "Withdraw it? [y/N]:") {
		t.Errorf("the prompt belongs on stderr, got %q", errb.String())
	}
}

// --- end-to-end: what the refusal did to the network ---

// 🔴 THE CENTRAL GUARANTEE. A non-interactive `app withdraw <id>` without --yes
// refuses AND issues ZERO requests, so "nothing was destroyed" is a fact about
// the wire rather than a claim about the copy.
func TestAppWithdrawNonTTYRefusalIssuesNoRequest(t *testing.T) {
	withStdinTTY(t, false)
	srv, hits := countingWithdrawServer(t, http.StatusOK)
	withdrawEnv(t, srv.URL)

	out, _, err := run(t, "app", "withdraw", "pubreq_01H")
	if err == nil {
		t.Fatal("a scripted withdraw without --yes must refuse")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the refusal issued %d request(s) — it must issue NONE, or the refusal "+
			"is printed over a listing that has already been deleted", n)
	}
	if !strings.Contains(err.Error(), "refusing to withdraw without --yes") {
		t.Errorf("refusal message: %v", err)
	}
	if strings.Contains(out, "Withdrew") {
		t.Errorf("a refusal must never print the success line: %q", out)
	}
}

// The refusal exits 1 (generic), NOT 2/3/4/5/6.
//
// Per AGENTS.md item 7 the exit code is decided by errors.Is against the
// classification sentinels and by NOTHING in the message, so this asserts the
// error is untagged — which is precisely what routes it to exitGeneric in
// cmd/civitai/main.go's default branch. Asserting the wording instead would say
// nothing at all about the exit code.
func TestAppWithdrawRefusalCarriesNoClassificationSentinel(t *testing.T) {
	withStdinTTY(t, false)
	srv, _ := countingWithdrawServer(t, http.StatusOK)
	withdrawEnv(t, srv.URL)

	_, _, err := run(t, "app", "withdraw", "pubreq_01H")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, s := range []struct {
		name string
		err  error
	}{
		{"ErrUsage", ErrUsage},
		{"civitai.ErrBadRequest", civitai.ErrBadRequest},
		{"civitai.ErrUnauthorized", civitai.ErrUnauthorized},
		{"civitai.ErrNotFound", civitai.ErrNotFound},
		{"civitai.ErrRateLimited", civitai.ErrRateLimited},
		{"civitai.ErrNetwork", civitai.ErrNetwork},
	} {
		if errors.Is(err, s.err) {
			t.Errorf("the withdraw refusal is tagged %s, so it exits with that kind's code "+
				"instead of the generic 1 the README publishes for it", s.name)
		}
	}
}

// An interactive DECLINE issues no request either.
func TestAppWithdrawInteractiveDeclineIssuesNoRequest(t *testing.T) {
	withStdinTTY(t, true)
	srv, hits := countingWithdrawServer(t, http.StatusOK)
	withdrawEnv(t, srv.URL)

	out, errb, err := runWithdrawStdin(t, "n\n", "app", "withdraw", "pubreq_01H")
	if err == nil {
		t.Fatal("answering n must abort")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("declining issued %d request(s) — it must issue NONE", n)
	}
	if !strings.Contains(errb, "Withdraw it? [y/N]:") {
		t.Errorf("expected the prompt on stderr, got %q", errb)
	}
	if strings.Contains(out, "Withdrew") {
		t.Errorf("a declined withdraw must not print the success line: %q", out)
	}
}

// An interactive ACCEPT proceeds — exactly one request, and the success line.
func TestAppWithdrawInteractiveAcceptIssuesTheRequest(t *testing.T) {
	withStdinTTY(t, true)
	srv, hits := countingWithdrawServer(t, http.StatusOK)
	withdrawEnv(t, srv.URL)

	out, errb, err := runWithdrawStdin(t, "y\n", "app", "withdraw", "pubreq_01H")
	if err != nil {
		t.Fatalf("answering y should withdraw: %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("a confirmed withdraw issued %d request(s), want exactly 1", n)
	}
	// 🔴 The prompt must actually have been SHOWN. Without this the test passes
	// against pre-#350 code, which ignored stdin entirely and withdrew — a green
	// that would have certified the bug.
	if !strings.Contains(errb, "Withdraw it? [y/N]:") {
		t.Errorf("the withdraw proceeded without ever asking: %q", errb)
	}
	if !strings.Contains(out, "Withdrew") {
		t.Errorf("expected the success line, got %q", out)
	}
}

// CONTROL — a legitimate confirmed withdraw still succeeds, over --yes rather
// than the prompt. If this ever goes red the gate has broken the command it was
// added to protect.
func TestAppWithdrawWithYesStillSucceeds(t *testing.T) {
	withStdinTTY(t, false)
	srv, hits := countingWithdrawServer(t, http.StatusOK)
	withdrawEnv(t, srv.URL)

	out, _, err := run(t, "app", "withdraw", "pubreq_01H", "--yes")
	if err != nil {
		t.Fatalf("app withdraw --yes: %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("--yes issued %d request(s), want exactly 1", n)
	}
	if !strings.Contains(out, "Withdrew") {
		t.Errorf("expected the success line, got %q", out)
	}
}

// CONTROL — a withdraw of an id the server does not know still 404s, and still
// classifies as ErrNotFound (exit 4). #341 fixed exactly this class for
// `workflows cancel`; the gate added here must not regress it, and must not
// swallow the 404 behind a confirmation.
func TestAppWithdrawUnknownIDStill404s(t *testing.T) {
	withStdinTTY(t, false)
	srv, hits := countingWithdrawServer(t, http.StatusNotFound)
	withdrawEnv(t, srv.URL)

	_, _, err := run(t, "app", "withdraw", "pubreq_nope", "--yes")
	if err == nil {
		t.Fatal("an unknown publish-request id must still fail")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("the 404 control issued %d request(s), want 1 — the confirmation must not "+
			"have replaced the server's own answer", n)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("an unknown id must stay classified ErrNotFound (exit 4), got %v", err)
	}
}

// The line printed AFTER a successful withdraw names the loss, because a --yes
// run never saw the prompt. Issue #350's reporter's only signal was a bare
// "Withdrew …" and rc=0.
func TestAppWithdrawSuccessWarnsAboutTheListing(t *testing.T) {
	withStdinTTY(t, false)
	srv, _ := countingWithdrawServer(t, http.StatusOK)
	withdrawEnv(t, srv.URL)

	out, errb, err := run(t, "app", "withdraw", "pubreq_01H", "--yes")
	if err != nil {
		t.Fatalf("app withdraw --yes: %v", err)
	}
	for _, want := range []string{"store listing", "icon", "cover", "screenshot"} {
		if !strings.Contains(strings.ToLower(errb), want) {
			t.Errorf("the post-withdraw note must name %q, got %q", want, errb)
		}
	}
	if !strings.Contains(errb, "civitai app listing status") {
		t.Errorf("the note must name the next command to run, got %q", errb)
	}
	// The note is advisory: it belongs on stderr so `civitai app withdraw … >out`
	// keeps the same stdout it always had.
	if strings.Contains(out, "store listing") {
		t.Errorf("the advisory note must not be on stdout: %q", out)
	}
}

// --- the docs half of #350 ---

// TestWithdrawHelpRetractsTheFreeRepairFraming pins the help-text corrections
// #350 asked for, using only LITERAL strings.
//
// The literals are deliberate: this test compiles against pre-#350 code, so it
// can be watched to FAIL there. Its sibling below asserts the same body against
// the shared constant, which is stronger coupling but cannot exist at base — see
// that test's note.
func TestWithdrawHelpRetractsTheFreeRepairFraming(t *testing.T) {
	out, _, err := run(t, "app", "withdraw", "--help")
	if err != nil {
		t.Fatalf("app withdraw --help: %v", err)
	}
	for _, want := range []string{
		// The idempotency sentence is true of the submission and false of the
		// listing; `--help` used to say it flatly.
		"idempotent WITH RESPECT TO THE SUBMISSION ONLY",
		// A rejection discards the draft too (publish-request.service.ts:4969),
		// which the issue did not know and an author needs to.
		"MODERATOR REJECTS",
		// The gate itself has to be discoverable from --help, or a script author
		// meets it for the first time as a CI failure.
		"CONFIRMATION:",
		"REFUSES rather than deleting a listing silently",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("`app withdraw --help` no longer says %q:\n%s", want, out)
		}
	}
	// The retracted claims: withdraw is NOT non-interactive any more, and --yes
	// is NOT a no-op. Leaving either sentence in place would document the exact
	// behaviour the fix removed — and both were in the help #350 quoted.
	for _, gone := range []string{"never prompts", "no-op", "non-interactive (it"} {
		if strings.Contains(out, gone) {
			t.Errorf("`app withdraw --help` still describes the pre-#350 behaviour %q:\n%s", gone, out)
		}
	}
}

// TestWithdrawHelpQuotesTheDestructionConstant couples `--help` to the SAME
// sentence the prompt and the refusal use, so a reword moves all three or fails.
//
// It cannot be run against pre-#350 code: the constant does not exist there, so
// the package does not compile. That is a statement about the test, not evidence
// about the fix — the behavioural red-at-base claims live in the tests above.
//
// Whitespace is normalised on both sides. The help hard-wraps the constant at 78
// columns (cobra does not wrap `Long`, so an unwrapped 190-column paragraph would
// break mid-clause in a standard terminal) while the refusal emits it as one
// line. Pinning the bytes would therefore pin the LINE BREAKS, and this guard
// exists to pin the CLAIM.
func TestWithdrawHelpQuotesTheDestructionConstant(t *testing.T) {
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	out, _, err := run(t, "app", "withdraw", "--help")
	if err != nil {
		t.Fatalf("app withdraw --help: %v", err)
	}
	if !strings.Contains(norm(out), norm(withdrawDiscardsListing)) {
		t.Errorf("`app withdraw --help` no longer states withdrawDiscardsListing, so the "+
			"help, the prompt and the refusal can now disagree about what is destroyed:\n%s", out)
	}
	// NEGATIVE CONTROL: whitespace normalisation must not have turned the check
	// into one that matches anything.
	if strings.Contains(norm(out), norm(withdrawDiscardsListing)+" and pigs fly") {
		t.Error("the normalised matcher cannot report a miss, so its hit proves nothing")
	}
}

// TestCarriesForwardClaimsAreQualifiedEverywhere is the SEAM guard for the docs
// half of #350.
//
// The defect was not in any one surface: three separately-correct sentences —
// the README quickstart, `app listing --help`, and `app submit`'s success
// output — each said the listing media "carries forward", each was true across
// APPROVAL and false across withdraw, and nothing distinguished the two for the
// reader. Fixing one and leaving two is the failure mode this forbids, so the
// assertion is over the SET of surfaces, not over any member of it.
//
// The two Go surfaces are checked against the shared constant, so a reword moves
// them together. The README cannot import a Go constant, so it is checked
// against the constant's TEXT with whitespace normalised — that lets prose wrap
// however it likes while still failing if the claim itself drifts.
func TestCarriesForwardClaimsAreQualifiedEverywhere(t *testing.T) {
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	caveat := norm(withdrawListingCaveat)

	var submitHeadsUp bytes.Buffer
	printListingFloorHeadsUp(&submitHeadsUp)

	listingHelp, _, err := run(t, "app", "listing", "--help")
	if err != nil {
		t.Fatalf("app listing --help: %v", err)
	}

	surfaces := []struct{ name, body string }{
		{"`app submit` success output (printListingFloorHeadsUp)", submitHeadsUp.String()},
		{"`app listing --help`", listingHelp},
		{"README.md", readREADME(t)},
	}
	for _, s := range surfaces {
		if !strings.Contains(norm(s.body), caveat) {
			t.Errorf("%s promises the media carries forward without the #350 qualification.\n"+
				"It must contain (whitespace-insensitively):\n  %s\n"+
				"Fixing one surface and leaving the others is what made this a data-loss bug: "+
				"the CLI reprinted the unqualified promise over a listing it had just deleted.", s.name, caveat)
		}
	}

	// POSITIVE CONTROL. Every assertion above is a `Contains`, and a Contains
	// suite passes identically whether the extractor works or reads an empty
	// string. Prove each surface is really loaded and non-trivial first.
	for _, s := range surfaces {
		if len(s.body) < 200 {
			t.Fatalf("%s produced only %d bytes — the guard above is asserting over nothing",
				s.name, len(s.body))
		}
	}

	// NEGATIVE CONTROL on the matcher itself: it must be able to MISS. A
	// substring check that somehow matched everything would make the whole test
	// vacuous.
	if strings.Contains(norm(submitHeadsUp.String()), caveat+" and pigs fly") {
		t.Error("the caveat matcher cannot report a miss, so its hits prove nothing")
	}
}
