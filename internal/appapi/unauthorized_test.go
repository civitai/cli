package appapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// 🔴 FIVE GUARDS HAVE STOOD HERE. THE FOUR THAT ANALYSED SOURCE ARE ALL GONE.
//
// A per-file ledger, then a regex over `case http.StatusUnauthorized:`, then a
// go/parser walk over case clauses, then a go/parser walk keyed on message
// construction. An audit round refuted each one by finding a spelling it could
// not see — and each time the guard's own comment had denied having a blind spot,
// including in this block, which asserted "cannot have a pattern blind spot at
// all" and outlived by two rounds the draft it was describing.
//
// The replacement is unauthorized_mappers_test.go, which reads OUTPUT instead of
// source: it drives every status mapper with a 401 and compares what comes back.
// Its coverage limit is stated there rather than denied here.
//
// ⚠ This block itself carried FOUR stale claims for a whole round — a deleted
// test's name, a ledger that no longer existed, the omniscience sentence, and a
// "reports file:line for every arm" that was never true — while the file it sits
// in was edited thirty lines below. It is kept short now for that reason: a long
// explanation of retired machinery is a surface nobody re-reads and everybody
// trusts.

// TestUnauthorizedErrorNamesBothRemedies pins what the message SAYS, as a whole
// normalised string rather than by keyword.
//
// 🔴 A KEYWORD ASSERTION HERE WOULD BE WALKABLE BY REWORDING, which is the
// failure this repo's rules name for prose guards: `Contains("civitai login")`
// is satisfied by a message that says to run it and then contradicts itself in
// the next clause, and `Contains("civitai.com/user/account")` is satisfied by a
// message that names the URL without saying what to do with it. The expected
// text is CONSTRUCTED from the constant the production path uses, so a
// code-side change moves both sides together — and the literal clauses below
// are what stops that from being vacuous.
func TestUnauthorizedErrorNamesBothRemedies(t *testing.T) {
	withMsg := unauthorizedError("token expired").Error()
	bare := unauthorizedError("").Error()

	if want := "not logged in (401): token expired — " + unauthorizedRemedy; withMsg != want {
		t.Errorf("unauthorizedError(%q) =\n  %q\nwant\n  %q", "token expired", withMsg, want)
	}
	if want := "not logged in (401) — " + unauthorizedRemedy; bare != want {
		t.Errorf("unauthorizedError(\"\") =\n  %q\nwant\n  %q", bare, want)
	}

	// 🔴 The literals. Without these the two assertions above compare the
	// constant against itself and hold however it is reworded — including
	// reworded back to the version that named no key route at all, which is the
	// regression this whole change exists to fix.
	for _, clause := range []string{
		"run `civitai login` (or set CIVITAI_TOKEN)",
		"already tried to refresh and retry",
		"https://civitai.com/user/account",
		"`civitai login --token <key>`",
	} {
		if !strings.Contains(unauthorizedRemedy, clause) {
			t.Errorf("the 401 remedy no longer contains %q.\n\nfull text: %s\n\n"+
				"Each clause is a separate action a user can take, and the key route is the one that "+
				"works when `civitai login` does not. 🔴 The middle clause is CONDITIONED on OAuth "+
				"and must NOT name which half of refresh-and-retry failed. Two earlier wordings did "+
				"and each was false about the other case: \"the refresh failed too\" is false when "+
				"the refresh SUCCEEDED and the retry 401d, and \"already failed twice\" is false when "+
				"the refresh ERRORED so no retry ever ran. A personal key never refreshes at all "+
				"(auth/source.go returns ErrNoRefresh), which is what the OAuth condition covers.", clause, unauthorizedRemedy)
		}
	}
}

// TestUnauthorizedErrorIsStatusTagged guards the property the PUBLISHED EXIT
// CODE depends on: every appapi route defers `civitai.TagStatus(status, err)`,
// so a 401 built by the helper must still classify as civitai.ErrUnauthorized.
// It drives the REAL arm rather than the helper, because the helper cannot tag
// itself — a test that only called unauthorizedError would pass while the arm it
// replaced had lost its `defer`.
//
// 🔴 AN EARLIER DRAFT OF THIS TEST ASSERTED ONLY THE MESSAGE TEXT while its
// docstring claimed it guarded the tag. That is the defect class this repo's
// rules name — "a docstring names a RELATIONSHIP, the body inspects one SIDE" —
// found in this file before it shipped, and kept in the comment rather than
// quietly corrected because the tell is worth more than the fix: the sentence
// read as coverage while providing none, which stops anyone looking.
func TestUnauthorizedErrorIsStatusTagged(t *testing.T) {
	err := analyticsError(http.StatusUnauthorized, []byte(`{"error":{"json":{"message":"nope"}}}`))
	if err == nil {
		t.Fatal("analyticsError returned nil for a 401")
	}
	// The property, asserted rather than described. Without this the exit code
	// silently moves off 3 the next time an arm's `defer` is dropped.
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("a 401 from analyticsError is not tagged civitai.ErrUnauthorized, so the CLI-wide "+
			"exit-code classifier will not see it as an auth failure:\n  %v", err)
	}
	// And the text, because the tag alone says nothing about what a user reads.
	if !strings.Contains(err.Error(), "https://civitai.com/user/account") {
		t.Errorf("the 401 that reaches a user does not name the key route:\n  %s", err.Error())
	}
	// Control: the tag must not be something every error here carries. A 404
	// through the same route must NOT be ErrUnauthorized, or the assertion above
	// holds for reasons unrelated to the 401 arm.
	other := analyticsError(http.StatusNotFound, []byte(`{"error":{"json":{"message":"nope"}}}`))
	if errors.Is(other, civitai.ErrUnauthorized) {
		t.Errorf("CONTROL failure: a 404 is also tagged ErrUnauthorized, so the check above cannot "+
			"distinguish the 401 arm from any other:\n  %v", other)
	}
}
