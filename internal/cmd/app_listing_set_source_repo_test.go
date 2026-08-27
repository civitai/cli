package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// `civitai app listing set-source-repo` — the OFF-SITE public source-repository
// link.
//
// Everything here runs against the httptest fake `set-text` already defines
// (`setTextServer`): the two commands call the SAME two reads and the SAME write
// proc, so a second fake would be a second thing to keep in step with the
// server's payload shape — and this repo has already been bitten once by a fake
// that agreed with the bug.
//
// Nothing in this file has been run against a real listing.

const srSourceRepoURL = "https://github.com/zephyr-labs/quartz-tool"

// ---------------------------------------------------------------------------
// What goes on the wire. Two states, and `null` is one of them.
// ---------------------------------------------------------------------------

// TestSetSourceRepoSendsTheURL: exactly one key, carrying the URL verbatim.
//
// 🔴 VERBATIM IS THE ASSERTION. The command deliberately does not normalise or
// pre-validate the value (the server's `validateRepositoryUrl` is the authority
// and returns the canonical form), so a CLI that "helpfully" trimmed a `.git` or
// lower-cased the host would be making up an answer the server did not give.
func TestSetSourceRepoSendsTheURL(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug); err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	patch := srv.patchSent(t)
	if got := len(patch); got != 1 {
		t.Fatalf("patch carries %d keys, want exactly 1 — nothing else may ride along: %v", got, patch)
	}
	raw, ok := patch["sourceRepoUrl"]
	if !ok {
		t.Fatalf("patch has no sourceRepoUrl key: %v", patch)
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("sourceRepoUrl is not a JSON string (%v): %s", err, raw)
	}
	if got != srSourceRepoURL {
		t.Errorf("sent sourceRepoUrl %q, want %q — the URL must go out verbatim", got, srSourceRepoURL)
	}
}

// TestSetSourceRepoClearSendsExplicitNull: `--clear` must send the KEY with a
// JSON null, not omit it.
//
// 🔴 OMITTED AND NULL ARE DIFFERENT SERVER STATES: `updateListingPatchSchema`
// leaves an omitted field untouched and CLEARS an explicit null. A `--clear`
// that dropped the key would report success while changing nothing.
func TestSetSourceRepoClearSendsExplicitNull(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-source-repo", "--clear", "--slug", stSlug); err != nil {
		t.Fatalf("set-source-repo --clear: %v", err)
	}
	patch := srv.patchSent(t)
	raw, ok := patch["sourceRepoUrl"]
	if !ok {
		t.Fatalf("--clear sent no sourceRepoUrl key at all — an omitted key leaves the column "+
			"UNTOUCHED, so this would silently do nothing: %v", patch)
	}
	if strings.TrimSpace(string(raw)) != "null" {
		t.Errorf("--clear sent sourceRepoUrl %s, want the literal null", raw)
	}
}

// ---------------------------------------------------------------------------
// Refusals, all of which must happen BEFORE any request.
// ---------------------------------------------------------------------------

func TestSetSourceRepoUsageRefusals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"url and --clear together", []string{srSourceRepoURL, "--clear"}, "not both"},
		{"neither a url nor --clear", []string{}, "nothing to do"},
		{"a blank url", []string{"   "}, "blank"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSetTextServer(t)
			args := append([]string{"app", "listing", "set-source-repo"}, tc.args...)
			args = append(args, "--slug", stSlug)
			_, _, err := run(t, args...)
			if err == nil {
				t.Fatal("expected a refusal, got success")
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("refusal must carry ErrUsage so its exit code is 2 (a malformed "+
					"invocation), got %T: %v", err, err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("refusal should say %q, got: %v", tc.want, err)
			}
			// 🔴 AND IT MUST COST NO REQUEST. These are decidable locally, and a
			// round trip would spend one of the ~30/hour rate-limit budget to be
			// told what the CLI already knew.
			for _, p := range srv.seen() {
				if p == updateListingPath {
					t.Error("a locally-decidable refusal still sent the update")
				}
			}
		})
	}
}

// want0SourceRepo is the ON-SITE source-repo remedy, written out here rather
// than read from the constant the command uses.
const want0SourceRepo = "source-repository link comes from the `repository` key in block.manifest.json — editing it here would be overwritten by the manifest at your next approved version. Set `repository` in block.manifest.json and run `civitai app submit`."

// TestSetSourceRepoRefusesOnsite pins the kind gate for THIS command.
//
// 🔴 THE GATE IS SHARED WITH `set-text` BUT THE REMEDY IS NOT, and the remedy is
// the half that is worth asserting: an on-site app's link comes from the
// manifest's `repository` key, NOT from `tagline`/`description`/`category`. A
// refusal that named the text remedy would send the author to edit three fields
// that have nothing to do with what they asked for.
func TestSetSourceRepoRefusesOnsite(t *testing.T) {
	srv := newSetTextServer(t, withKind(appapi.ListingKindOnsite))
	_, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err == nil {
		t.Fatal("an ON-SITE listing must be refused")
	}
	if !errors.Is(err, ErrOnsiteNotEditable) {
		t.Errorf("the refusal must carry ErrOnsiteNotEditable so its exit code is assertable, got %T: %v", err, err)
	}
	// 🔴 THE WHOLE NORMALISED CLAUSE, for the same reason as its `set-text`
	// counterpart: "repository" alone is a keyword, and a keyword pin is
	// satisfiable by a sibling subject that merely happens to contain it.
	// 🔴 A LITERAL, for the same reason as its `set-text` counterpart: an
	// expectation read from the constant under test cannot see a reword of that
	// constant, and an audit demonstrated exactly that.
	if want := want0SourceRepo; !strings.Contains(err.Error(), want) {
		t.Errorf("the refusal must carry the SOURCE-REPO remedy verbatim.\n got: %v\nwant it to contain: %s", err, want)
	}
	if onsiteSubjectSourceRepo.clause != want0SourceRepo {
		t.Errorf("onsiteSubjectSourceRepo.clause no longer matches the pinned text.\n got: %s\nwant: %s",
			onsiteSubjectSourceRepo.clause, want0SourceRepo)
	}
	if onsiteSubjectText.clause == onsiteSubjectSourceRepo.clause {
		t.Fatal("the two onsite remedies are identical — the assertion above cannot tell them apart")
	}
	for _, p := range srv.seen() {
		if p == updateListingPath {
			t.Error("the onsite refusal still wrote to the listing")
		}
	}
}

// ---------------------------------------------------------------------------
// The material branch. This is what makes the command different from set-text.
// ---------------------------------------------------------------------------

// TestSetSourceRepoReportsAStagedRevision: when the server stages the edit, the
// success line must NOT read as "published".
//
// 🔴 THIS BRANCH IS REACHABLE HERE AND UNREACHABLE IN `set-text`, which is the
// entire reason this is a separate command. `sourceRepoUrl` is in
// MATERIAL_PATCH_FIELDS, so on an APPROVED listing the server opens a shadow and
// the live page is unchanged until a moderator approves it. Printing a bare
// "Updated" would be false, and a script reading exit 0 as "the link is live"
// would be wrong.
func TestSetSourceRepoReportsAStagedRevision(t *testing.T) {
	const shadow = "apl_SHADOW_QZ42"
	newSetTextServer(t, withStatus("approved"),
		withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		// Staging is what was asked for and it succeeded, so this is exit 0 with
		// an instruction — not a failure.
		t.Fatalf("a staged edit is a success, not an error: %v", err)
	}
	for _, want := range []string{"STAGED", shadow, "civitai app listing submit-revision"} {
		if !strings.Contains(out, want) {
			t.Errorf("staged output must contain %q so the author knows the link is not public "+
				"yet and what publishes it; got:\n%s", want, out)
		}
	}
	if !strings.Contains(strings.ToLower(out), "unchanged") {
		t.Errorf("staged output must say the LIVE listing is unchanged — that is the fact a "+
			"reader would otherwise get wrong; got:\n%s", out)
	}
}

// TestSetSourceRepoInPlaceMentionsNoRevision is the NEGATIVE half of the test
// above, and without it that one proves much less: a renderer that printed the
// revision advice unconditionally would satisfy it.
func TestSetSourceRepoInPlaceMentionsNoRevision(t *testing.T) {
	newSetTextServer(t, withStatus("draft"),
		withReply(map[string]any{"requiresReview": false, "shadowId": nil}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if !strings.Contains(out, srSourceRepoURL) {
		t.Errorf("the success line should echo the link that was set; got:\n%s", out)
	}
	for _, unwanted := range []string{"STAGED", "submit-revision", "moderator"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("an in-place edit must NOT mention %q — the server applied it directly, and "+
				"telling the author to submit a revision would send them after a shadow that does "+
				"not exist; got:\n%s", unwanted, out)
		}
	}
}

// ---------------------------------------------------------------------------
// --json. PUBLISHED CONTRACT.
// ---------------------------------------------------------------------------

func TestSetSourceRepoJSONPayload(t *testing.T) {
	t.Run("set carries the url, the action and the server's branch", func(t *testing.T) {
		const shadow = "apl_SHADOW_JS7"
		newSetTextServer(t, withStatus("approved"),
			withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
		out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug, "--json")
		if err != nil {
			t.Fatalf("set-source-repo --json: %v", err)
		}
		var got struct {
			Slug           string  `json:"slug"`
			AppListingID   string  `json:"appListingId"`
			SourceRepoURL  *string `json:"sourceRepoUrl"`
			Action         string  `json:"action"`
			RequiresReview bool    `json:"requiresReview"`
			ShadowID       *string `json:"shadowId"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("--json did not emit one JSON object (%v):\n%s", err, out)
		}
		if got.Slug != stSlug || got.AppListingID != stListingID {
			t.Errorf("payload identifies %q/%q, want %q/%q", got.Slug, got.AppListingID, stSlug, stListingID)
		}
		if got.SourceRepoURL == nil || *got.SourceRepoURL != srSourceRepoURL {
			t.Errorf("payload sourceRepoUrl = %v, want %q", got.SourceRepoURL, srSourceRepoURL)
		}
		if got.Action != "set" {
			t.Errorf("payload action = %q, want \"set\"", got.Action)
		}
		// 🔴 THE WHOLE POINT OF THE PAYLOAD. A consumer that cannot see this is
		// one that reports a staged link as published.
		if !got.RequiresReview {
			t.Error("payload requiresReview = false, want true — it must pass the SERVER's branch through")
		}
		if got.ShadowID == nil || *got.ShadowID != shadow {
			t.Errorf("payload shadowId = %v, want %q", got.ShadowID, shadow)
		}
	})

	t.Run("cleared reports a null url and its own action", func(t *testing.T) {
		newSetTextServer(t)
		out, _, err := run(t, "app", "listing", "set-source-repo", "--clear", "--slug", stSlug, "--json")
		if err != nil {
			t.Fatalf("set-source-repo --clear --json: %v", err)
		}
		var got struct {
			SourceRepoURL *string `json:"sourceRepoUrl"`
			Action        string  `json:"action"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("--json did not emit one JSON object (%v):\n%s", err, out)
		}
		if got.SourceRepoURL != nil {
			t.Errorf("cleared payload sourceRepoUrl = %q, want null", *got.SourceRepoURL)
		}
		// 🔴 `action` EXISTS SO null IS NOT THE ONLY SIGNAL. A consumer must be
		// able to tell "cleared" from "the key was absent" without inferring it.
		if got.Action != "cleared" {
			t.Errorf("cleared payload action = %q, want \"cleared\"", got.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// The seam between the two commands.
// ---------------------------------------------------------------------------

// TestSetTextNeverSendsSourceRepoURL is the BEHAVIOURAL half of a guarantee the
// type system already makes structurally (`appapi.ListingPatch` is a closed
// interface and `ListingTextPatch` has no source-repo field).
//
// 🔴 IT IS KEPT ANYWAY BECAUSE THE STRUCTURAL GUARD IS INVISIBLE AT THE SEAM.
// Both commands drive ONE proc with ONE `patch` object, so the property that
// matters is about the REQUEST, not about a Go type: if someone later merges the
// two patch types back together "to simplify", the compiler is satisfied and
// only this test notices. A `set-text` that sent `sourceRepoUrl` would convert
// every tagline edit on an approved listing into a staged revision.
func TestSetTextNeverSendsSourceRepoURL(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text",
		"--slug", stSlug, "--tagline", stTagline, "--description", stDesc, "--category", stCategory); err != nil {
		t.Fatalf("set-text: %v", err)
	}
	patch := srv.patchSent(t)
	if _, present := patch["sourceRepoUrl"]; present {
		t.Errorf("set-text put sourceRepoUrl on the wire — that is a MATERIAL field, so it would "+
			"stage the whole patch (tagline included) on a revision instead of applying it in "+
			"place: %v", patch)
	}
	// A positive control: the assertion above is only meaningful if this patch
	// actually carried the fields it was supposed to.
	if len(patch) != 3 {
		t.Fatalf("expected the 3 text keys on the wire, got %d — without them the assertion "+
			"above is vacuous: %v", len(patch), patch)
	}
}

// ---------------------------------------------------------------------------
// The pre-existing-revision hazard. Added after an audit found the command
// giving advice that publishes work the author never meant to publish.
// ---------------------------------------------------------------------------

// TestSetSourceRepoWarnsOnAPreExistingRevision.
//
// 🔴 `beginListingRevision` IS IDEMPOTENT, WHICH IS WHAT MAKES THIS A HAZARD
// RATHER THAN A COSMETIC ISSUE. It reuses an open shadow instead of minting a
// second one, so this edit lands beside whatever was already staged there — an
// `rm-screenshot` deliberately left unsubmitted (AGENTS item 30) is the normal
// case, not an exotic one. `applyApprovedRevision` then copies the shadow's
// WHOLE scalar set and its screenshots onto the parent, so a bare "send it for
// review" publishes all of it and can revert a live tagline.
func TestSetSourceRepoWarnsOnAPreExistingRevision(t *testing.T) {
	const shadow = "apl_PREEXISTING_9"
	newSetTextServer(t, withStatus("approved"), withOpenShadow(shadow),
		withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	for _, want := range []string{"ALREADY existed", "publishes EVERYTHING staged on it", "civitai app listing status"} {
		if !strings.Contains(out, want) {
			t.Errorf("a pre-existing revision must be called out (missing %q); got:\n%s", want, out)
		}
	}
	// 🔴 AND THE BARE INSTRUCTION MUST BE GONE. Printing the warning while ALSO
	// saying "Send it for review:" on its own line would leave the dangerous
	// advice on screen — the reader follows the imperative, not the caveat.
	if strings.Contains(out, "Send it for review:") {
		t.Errorf("the unconditional 'Send it for review' line must not appear when the revision "+
			"already carried other staged work; got:\n%s", out)
	}
}

// TestSetSourceRepoFreshRevisionKeepsTheSimpleInstruction is the other arm.
// Without it the test above is equally satisfied by a command that never prints
// the simple instruction at all.
func TestSetSourceRepoFreshRevisionKeepsTheSimpleInstruction(t *testing.T) {
	const shadow = "apl_FRESH_4"
	newSetTextServer(t, withStatus("approved"), // no withOpenShadow: the server minted this one
		withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if !strings.Contains(out, "Send it for review:") {
		t.Errorf("a revision the server opened FOR this edit carries nothing else, so the simple "+
			"instruction is correct and must still appear; got:\n%s", out)
	}
	if strings.Contains(out, "ALREADY existed") {
		t.Errorf("a freshly-minted revision must NOT be described as pre-existing — that would send "+
			"the author hunting for staged work that does not exist; got:\n%s", out)
	}
}

// TestSetSourceRepoJSONOpenRevision pins the machine-readable half of the same
// distinction.
//
// 🔴 `shadowId` ALONE CANNOT ANSWER IT: it is populated in BOTH cases, because
// the server reuses an open shadow. Without `openRevision` a script cannot tell
// "the server opened a revision for my change" from "my change joined one that
// already carries somebody else's staged work".
func TestSetSourceRepoJSONOpenRevision(t *testing.T) {
	decode := func(t *testing.T, out string) (bool, *string) {
		t.Helper()
		var got struct {
			OpenRevision bool    `json:"openRevision"`
			ShadowID     *string `json:"shadowId"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("--json did not emit one JSON object (%v):\n%s", err, out)
		}
		return got.OpenRevision, got.ShadowID
	}

	t.Run("true when a revision already existed", func(t *testing.T) {
		const shadow = "apl_PREEXISTING_J"
		newSetTextServer(t, withStatus("approved"), withOpenShadow(shadow),
			withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
		out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug, "--json")
		if err != nil {
			t.Fatalf("set-source-repo --json: %v", err)
		}
		open, sid := decode(t, out)
		if !open {
			t.Error("openRevision = false, want true — a revision existed before this edit")
		}
		if sid == nil || *sid != shadow {
			t.Errorf("shadowId = %v, want %q", sid, shadow)
		}
	})

	t.Run("false when the server opened one for this edit", func(t *testing.T) {
		const shadow = "apl_FRESH_J"
		newSetTextServer(t, withStatus("approved"),
			withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
		out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug, "--json")
		if err != nil {
			t.Fatalf("set-source-repo --json: %v", err)
		}
		open, sid := decode(t, out)
		if open {
			t.Error("openRevision = true, want false — nothing was staged before this edit")
		}
		// 🔴 THE POSITIVE CONTROL ON THE PAIR. shadowId is populated in BOTH
		// subtests, which is exactly why openRevision has to exist; if this were
		// nil the two cases would be trivially distinguishable and the field
		// would be redundant.
		if sid == nil || *sid != shadow {
			t.Fatalf("shadowId = %v, want %q — without it this pair proves nothing", sid, shadow)
		}
	})
}

// ---------------------------------------------------------------------------
// Round-2 arms. Added with the fixes, not after them — round 1 shipped two
// behaviours unguarded and only the mutation battery noticed.
// ---------------------------------------------------------------------------

// TestSetSourceRepoPendingRevisionDoesNotAdviseSubmitting.
//
// 🔴 `submitListingRevision` ON AN ALREADY-PENDING REVISION IS A NO-OP: the
// server returns the existing publish request unchanged. Offering it there is
// not dangerous, it is MISLEADING — it implies an action is outstanding when the
// edit is already in the moderator queue, and the author goes looking for a
// button that does nothing.
func TestSetSourceRepoPendingRevisionDoesNotAdviseSubmitting(t *testing.T) {
	const shadow = "apl_PENDING_7"
	newSetTextServer(t, withStatus("approved"), withOpenShadow(shadow), withPending(true),
		withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if !strings.Contains(out, "ALREADY under moderator review") {
		t.Errorf("a submitted revision must be described as such; got:\n%s", out)
	}
	if strings.Contains(out, "submit-revision") {
		t.Errorf("must NOT advise submit-revision on a revision already under review — the server "+
			"returns the existing request unchanged, so the advice implies work that does not "+
			"exist; got:\n%s", out)
	}
}

// TestSetSourceRepoStagedAdviceCarriesTheSlug.
//
// 🔴 BOTH COMMANDS IN THE BLOCK BIND `--slug`. Printing it on the first line and
// omitting it on the second sends a user who reached this command with `--slug`,
// from a directory with no matching block.manifest.json, into a slug-resolve
// failure on the follow-up.
func TestSetSourceRepoStagedAdviceCarriesTheSlug(t *testing.T) {
	const shadow = "apl_SLUGADVICE_2"
	newSetTextServer(t, withStatus("approved"), withOpenShadow(shadow),
		withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	for _, want := range []string{
		"civitai app listing status --slug " + stSlug,
		"civitai app listing submit-revision --slug " + stSlug,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("every command offered in the advice block must carry the slug the user passed "+
				"(missing %q); got:\n%s", want, out)
		}
	}

	// 🔴 THE OTHER STAGED ARM, WHICH THE FIRST FIX MISSED. A material edit on an
	// approved listing with NO pre-existing revision is the ORDINARY case, and it
	// prints its own `submit-revision` line. Fixing the slug on the rarer arm and
	// not this one left the common path broken — and the guard, scoped to one arm,
	// could not see it.
	newSetTextServer(t, withStatus("approved"),
		withReply(map[string]any{"requiresReview": true, "shadowId": "apl_FRESH_SLUG"}))
	fresh, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if want := "civitai app listing submit-revision --slug " + stSlug; !strings.Contains(fresh, want) {
		t.Errorf("the fresh-revision arm must carry the slug too (missing %q); got:\n%s", want, fresh)
	}
}

// TestSetSourceRepoInPlaceWarningKeepsTheTwoStatesApart.
//
// 🔴 "UNDER REVIEW" AND "OPEN BUT NOT SUBMITTED" ARE DIFFERENT, AND THE READER
// ACTS ON THEM DIFFERENTLY — one is with a moderator, the other is still theirs
// to edit. `warnOpenRevision`'s doc comment states that rule for `set-text`, and
// the staged branch of this command already honours it; the in-place branch
// collapsed both into one sentence when it was added.
func TestSetSourceRepoInPlaceWarningKeepsTheTwoStatesApart(t *testing.T) {
	inPlace := map[string]any{"requiresReview": false, "shadowId": nil}

	newSetTextServer(t, withStatus("approved"), withOpenShadow("apl_OPEN_A"), withReply(inPlace))
	open, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if !strings.Contains(open, "not been submitted yet") {
		t.Errorf("an OPEN, unsubmitted revision must be described as such; got:\n%s", open)
	}

	newSetTextServer(t, withStatus("approved"), withOpenShadow("apl_OPEN_B"), withPending(true), withReply(inPlace))
	pending, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if !strings.Contains(pending, "already under moderator review") {
		t.Errorf("a SUBMITTED revision must be described as such; got:\n%s", pending)
	}
	// 🔴 AND THE TWO MUST NOT BE THE SAME SENTENCE. Without this, a renderer that
	// printed one wording for both would satisfy whichever assertion happened to
	// match it.
	if strings.Contains(pending, "not been submitted yet") {
		t.Errorf("a revision under review must NOT be called unsubmitted; got:\n%s", pending)
	}
	if strings.Contains(open, "already under moderator review") {
		t.Errorf("an unsubmitted revision must NOT be called under review; got:\n%s", open)
	}
}

// TestSetSourceRepoInPlaceStillWarnsAboutAnOpenRevision.
//
// 🔴 THE TWO RENDERINGS MUST NOT DISAGREE ABOUT WHETHER THE HAZARD APPLIES.
// `--json` reports `openRevision` from the listing regardless of which branch the
// server took, so a human path that only warned inside the staged branch left an
// approved listing with an open shadow printing a bare success line while the
// machine payload said `openRevision:true` — the exact defect recorded in
// `warnOpenRevision`'s own doc comment, reintroduced one file over.
//
// Reachable when the server judges the patch non-material (a canonical no-op),
// and it matters because that shadow can carry a DIFFERENT staged sourceRepoUrl
// that replaces the live one on approval.
func TestSetSourceRepoInPlaceStillWarnsAboutAnOpenRevision(t *testing.T) {
	const shadow = "apl_INPLACE_SHADOW"
	newSetTextServer(t, withStatus("approved"), withOpenShadow(shadow),
		withReply(map[string]any{"requiresReview": false, "shadowId": nil}))
	out, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	// Asserts the LOAD-BEARING claim ("it will overwrite this link"), not the
	// sentence around it: the two revision states are worded differently on
	// purpose and TestSetSourceRepoInPlaceWarningKeepsTheTwoStatesApart owns that
	// distinction. Pinning the whole sentence here would make this test a second,
	// weaker copy of that one.
	if !strings.Contains(out, "overwrite this link") {
		t.Errorf("an in-place edit on a listing with an open revision must still warn — the "+
			"revision can overwrite this link on approval; got:\n%s", out)
	}
	// The other arm: no open revision, no warning. Without this the assertion
	// above is satisfied by a command that warns unconditionally.
	newSetTextServer(t, withStatus("draft"),
		withReply(map[string]any{"requiresReview": false, "shadowId": nil}))
	out2, _, err := run(t, "app", "listing", "set-source-repo", srSourceRepoURL, "--slug", stSlug)
	if err != nil {
		t.Fatalf("set-source-repo: %v", err)
	}
	if strings.Contains(out2, "overwrite this link") {
		t.Errorf("a listing with no open revision must not be warned about one; got:\n%s", out2)
	}
}
