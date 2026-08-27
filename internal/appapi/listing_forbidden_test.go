package appapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// THE 403 ARM ANSWERS FOR MORE THAN ONE CAUSE, AND ONLY SOME OF THEM ARE ABOUT
// THE CALLER'S ACCESS.
//
// `listingError`'s forbidden arm used to be two branches: the token-SCOPE
// refusal, and a fallback that appended "managing store listings needs
// Apps-author access (invite-only beta)" to everything else. One common
// "everything else" is a moderator TAKEDOWN — the server's
// `OffsiteRequestError('FORBIDDEN', 'this listing has been removed by a
// moderator and can no longer be edited')` — where the caller's Apps-author
// access is fine, the remedy named is unreachable, and the CLI's sentence
// contradicts the server's own sentence printed two words earlier.
//
// That is the wrong-subject class this package already catalogs on the 400 arm
// (civitai/cli#374, #391), regenerated one arm over: an arm answering for N
// causes may only claim what is true of all N.
//
// 🔴 WHAT THIS TABLE PINS, and why each row is here rather than folded into
// `TestListingErrorMapping`:
//
//   - the takedown rows are the REGRESSION — measured red at `origin/main`
//     (d2a635c), where both fail on `wantAccessClaim` because the fallback
//     claimed the account's access was the problem;
//   - the cohort row keeps the fallback ALIVE. Deleting the false advice
//     everywhere would pass a test that only asserted its absence, and would
//     lose the one 403 the advice is actually true of;
//   - the scope row pins that the takedown branch was inserted BELOW it, not
//     in front of it;
//   - every row asserts the classification with `errors.Is`, never on text
//     (AGENTS.md item 7): `Error()` is byte-identical with the sentinel
//     stripped, so a message assertion says nothing about the exit code. 403 →
//     `ErrUnauthorized` → exit 3, which is what README publishes for "a
//     moderator removed the listing".
func TestListingForbiddenTakedownDoesNotBlameTheAccount(t *testing.T) {
	// The false diagnosis, spelled once. It is the whole subject of the
	// regression: the takedown rows must not carry it, the cohort row must.
	const accessClaim = "Apps-author access"

	rows := []struct {
		name string
		// msg is the server's own message, verbatim from the source that
		// throws it wherever the row cites one.
		msg string
		// wantSubstr is what the arm the CLI chose must say.
		wantSubstr string
		// wantAccessClaim is whether the Apps-author-access diagnosis belongs
		// on this 403 at all.
		wantAccessClaim bool
	}{
		{
			// civitai/civitai@origin/main,
			// src/server/services/blocks/offsite-listing.service.ts:1242 (the
			// updateListing write path) and :1888 (the prefill read path).
			name:            "moderator takedown, the edit refusal",
			msg:             "this listing has been removed by a moderator and can no longer be edited",
			wantSubstr:      "ask a Civitai moderator to relist it",
			wantAccessClaim: false,
		},
		{
			// The SECOND spelling, from a different service:
			// src/server/services/blocks/offsite-moderation.service.ts:1638,
			// the republish refusal. It shares only the core the predicate
			// matches — a predicate keyed on the first sentence would answer
			// this one with the false advice.
			name:            "moderator takedown, the republish refusal",
			msg:             "This listing was removed by a moderator and cannot be restored by its owner.",
			wantSubstr:      "ask a Civitai moderator to relist it",
			wantAccessClaim: false,
		},
		{
			// 🔴 INVARIANT GUARD, not regression coverage, and labelled as one:
			// no server message is MEASURED to capitalise the core today. It is
			// here because the predicate folds case deliberately — the two
			// measured spellings above already disagree on the sentence's first
			// letter — so a "simplification" that drops the fold is a real
			// hazard even though no shipped string exercises it yet.
			name:            "invariant: the core is matched case-insensitively",
			msg:             "Removed By A Moderator, pending appeal",
			wantSubstr:      "ask a Civitai moderator to relist it",
			wantAccessClaim: false,
		},
		{
			// The 403 the fallback is TRUE of — the invite-gated cohort. Same
			// string TestListingErrorMapping uses, kept here so a fix that
			// emptied the fallback cannot pass this file.
			name:            "a cohort 403 keeps the Apps-author-access advice",
			msg:             "Apps authoring is not enabled",
			wantSubstr:      accessClaim,
			wantAccessClaim: true,
		},
		{
			// The scope arm is tested above the takedown branch; a takedown
			// branch inserted in front of it would steal nothing today, but a
			// scope message that ever mentions a moderator would.
			name:            "the scope 403 still gets its own arm",
			msg:             "Your API key does not have the required scope for this action",
			wantSubstr:      "Apps submit scope",
			wantAccessClaim: false,
		},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"error": map[string]any{"json": map[string]any{"message": tc.msg}},
			})
			err := listingError(http.StatusForbidden, body, trpcSetIcon)
			if err == nil {
				t.Fatalf("expected an error for a 403 carrying %q", tc.msg)
			}
			got := err.Error()

			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("the 403 for %q must say %q.\ngot: %s", tc.msg, tc.wantSubstr, got)
			}

			// Every 403 arm surfaces the server's own sentence — the server is
			// the authority on WHICH 403 this is, and TestListingErrorMapping
			// pins the same property for the arms it covers.
			if !strings.Contains(got, tc.msg) {
				t.Errorf("the 403 for %q must echo the server's own message.\ngot: %s", tc.msg, got)
			}

			if has := strings.Contains(got, accessClaim); has != tc.wantAccessClaim {
				if tc.wantAccessClaim {
					t.Errorf("the 403 for %q dropped the %q advice, which is the one 403 it is TRUE of — "+
						"the fallback has been emptied, not narrowed.\ngot: %s", tc.msg, accessClaim, got)
				} else {
					t.Errorf("the 403 for %q still blames the caller's %q. Nothing about the account's "+
						"access is the problem here, and no grant, re-login or CLI command reverses this "+
						"state — the user is sent to fix something that is already fine.\ngot: %s",
						tc.msg, accessClaim, got)
				}
			}

			// AGENTS.md item 7: the exit code is the sentinel, never the text.
			if !errors.Is(err, civitai.ErrUnauthorized) {
				t.Errorf("a listing 403 must stay tagged ErrUnauthorized (→ exit 3); got %T: %v", err, err)
			}
		})
	}
}
