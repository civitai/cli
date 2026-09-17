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

// unauthorizedLedger is the ASSERTED set of non-test files in this package that
// contain a `case http.StatusUnauthorized:` arm. It is checked in BOTH
// directions: the guard fails when the set GROWS (a new 401 arm was written)
// and when it SHRINKS (a ledgered one was removed or renamed).
//
// 🔴 A ONE-DIRECTIONAL VERSION OF THIS WOULD BE THE DEFECT IT EXISTS TO CATCH.
// The state before this change was five open-coded 401 literals, four spelling
// the remedy one way and the fifth another. A guard that only checked the files
// it already knew about would have gone on passing while a sixth site was added
// with a sixth wording — which is how the fifth got there.
var unauthorizedLedger = map[string]string{
	"listing.go":   "store-listing routes (`app listing …`)",
	"appblocks.go": "submit, dev-token, dev-tunnel and submissions",
	"analytics.go": "`app metrics`",
}

var statusUnauthorizedArm = regexp.MustCompile(`(?m)^\s*case http\.StatusUnauthorized:`)

// TestUnauthorizedCallersAreLedgered is the seam guard. It pins a RELATIONSHIP —
// "every 401 arm in this package returns unauthorizedError" — rather than
// inspecting one side of it.
//
// Mutation-verified when it landed, and the controls are recorded because a
// structural check like this type-checks past a wrong argument:
//   - restoring any one of the five original `fmt.Errorf("not logged in (401)…`
//     literals reddens it by file name;
//   - adding a `case http.StatusUnauthorized:` arm in a new file reddens the
//     GROWS direction;
//   - deleting `analytics.go`'s arm reddens the SHRINKS direction.
func TestUnauthorizedCallersAreLedgered(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	found := map[string]int{}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if n := len(statusUnauthorizedArm.FindAllIndex(src, -1)); n > 0 {
			found[name] = n
		}
	}
	// Positive control: a walk that read no files would report every ledgered
	// entry as missing (loud) AND every unledgered one as absent (silent). The
	// floor pins the walk to the real package.
	if scanned < 5 {
		t.Fatalf("CONTROL failure: scanned only %d non-test .go files in this package — "+
			"the directory walk is reading the wrong tree, and neither direction below is meaningful", scanned)
	}

	for name, why := range unauthorizedLedger {
		if _, ok := found[name]; !ok {
			t.Errorf("unauthorizedLedger names %s (%s), which no longer has a `case http.StatusUnauthorized:` arm. "+
				"Drop the entry, or restore the arm — a ledger entry for code that is gone is a rule nobody can check.",
				name, why)
		}
	}
	var unledgered []string
	for name := range found {
		if _, ok := unauthorizedLedger[name]; !ok {
			unledgered = append(unledgered, name)
		}
	}
	if len(unledgered) > 0 {
		sort.Strings(unledgered)
		t.Errorf("these files answer a 401 but are not in unauthorizedLedger: %v\n\n"+
			"Every 401 arm in this package must return unauthorizedError(serverMsg) so the remedy is "+
			"spelled once. Five open-coded literals is the state this replaced, and they had already "+
			"drifted apart. Add the file to the ledger once its arm calls the helper.", unledgered)
	}

	// The relationship itself: a ledgered file must CALL the helper, not merely
	// have an arm. A structural check that stopped at the previous assertion
	// would pass over a file that answers 401 with its own literal.
	for name := range unauthorizedLedger {
		src, err := os.ReadFile(name)
		if err != nil {
			continue // already reported above
		}
		if !strings.Contains(string(src), "unauthorizedError(") {
			t.Errorf("%s has a `case http.StatusUnauthorized:` arm but never calls unauthorizedError — "+
				"it is spelling the remedy itself, which is the drift this helper exists to end", name)
		}
		if strings.Contains(string(src), `"not logged in (401)`) {
			t.Errorf("%s still carries a literal `not logged in (401)` message. The message lives in "+
				"unauthorized.go and nowhere else; a second copy is free to disagree with it, and the "+
				"five copies this replaced already did.", name)
		}
	}
}

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
		"the refresh failed too",
		"https://civitai.com/user/account",
		"`civitai login --token <key>`",
	} {
		if !strings.Contains(unauthorizedRemedy, clause) {
			t.Errorf("the 401 remedy no longer contains %q.\n\nfull text: %s\n\n"+
				"Each clause is a separate action a user can take, and the key route is the one that "+
				"works when `civitai login` does not — an expired OAuth REFRESH token cannot be "+
				"refreshed by logging in again through the same path.", clause, unauthorizedRemedy)
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
