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
	if want := onsiteSubjectSourceRepo.clause; !strings.Contains(err.Error(), want) {
		t.Errorf("the refusal must carry the SOURCE-REPO remedy verbatim.\n got: %v\nwant it to contain: %s", err, want)
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
