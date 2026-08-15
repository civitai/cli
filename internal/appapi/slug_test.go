package appapi

import (
	"context"
	"sync/atomic"
	"testing"
)

// THE SHARED SLUG PREDICATE, AND THE ONE appapi-SIDE SITE THAT NOW CALLS IT.
//
// Before the consolidation, three of the four blockId comparisons in this binary
// were `==`/`!=` and the whole suite was green over a cased or padded blockId at
// every one of them — 4 sites, 0 fixtures. These cases are the fixtures that
// were missing: each drives ONE site with a spelling only the shared predicate
// accepts, so each is red on the exact compare it replaced.
//
// 🔴 THE NEGATIVE CONTROLS ARE NOT DECORATION. A predicate that widened too far
// would pass every positive case here and be catastrophically wrong: it decides
// whether a submission row is YOUR app. So every positive case below is paired
// with the same four rejects — an unrelated slug, a slug this one is a PREFIX
// of, a slug this one is a SUFFIX-EXTENSION of, and the empty string.

// slugRejects are the spellings that must NEVER match "my-block". They are
// chosen so a plausible over-wide implementation fails at least one:
// substring/prefix matching fails on "my-block-2" and "my", a Contains fails on
// both, and an implementation that treats empty as a wildcard fails on "".
var slugRejects = []struct{ name, spelling, breaks string }{
	{"unrelated app", "gen-matrix", "a different app entirely"},
	{"extension of the slug", "my-block-2", "prefix matching would accept a DIFFERENT registered app"},
	{"prefix of the slug", "my", "suffix/Contains matching would accept a shorter unrelated slug"},
	{"empty", "", "no slug names an app; empty must never be a wildcard"},
}

// TestSameSlugAcceptsOnlyRespellingsOfTheSameSlug pins the predicate itself:
// what it admits over `==`, and what it still refuses.
func TestSameSlugAcceptsOnlyRespellingsOfTheSameSlug(t *testing.T) {
	const canonical = "my-block"

	// (a) What normalising ADMITS that `==` rejected — and only this. Every
	// entry is an invalid spelling per schema/app-block.manifest.schema.json's
	// `^[a-z][a-z0-9-]*[a-z0-9]$`, which is why joining it to the canonical slug
	// cannot collide with a second real app.
	for _, respelling := range []string{
		"My-Block", "MY-BLOCK", "my-Block",
		" my-block", "my-block ", "  my-block  ",
		"\tmy-block\n", " My-Block ",
	} {
		if !SameSlug(respelling, canonical) {
			t.Errorf("SameSlug(%q, %q) = false, want true — a cased or padded blockId names the SAME app, "+
				"and rejecting it is the silent failure this predicate exists to remove", respelling, canonical)
		}
		// Symmetric: the callers pass the row on either side depending on the
		// site (row-vs-request at three of them, manifest-vs-row at the fourth).
		if !SameSlug(canonical, respelling) {
			t.Errorf("SameSlug(%q, %q) = false, want true — the predicate must not depend on argument order, "+
				"because its four callers do not agree on which side is which", canonical, respelling)
		}
	}

	// (b) The negative controls, in both argument orders.
	for _, r := range slugRejects {
		if SameSlug(r.spelling, canonical) {
			t.Errorf("SameSlug(%q, %q) = true, want false — %s", r.spelling, canonical, r.breaks)
		}
		if SameSlug(canonical, r.spelling) {
			t.Errorf("SameSlug(%q, %q) = true, want false — %s", canonical, r.spelling, r.breaks)
		}
	}

	// (c) EMPTY AGAINST EMPTY, the one case where this is stricter than the `==`
	// it replaces. `"" == ""` was TRUE at three sites, so a row carrying no
	// blockId matched a request carrying no slug and read as "yes, that is your
	// app". All-whitespace is the same case after TrimSpace, and the
	// hand-written `m.BlockID == ""` guard in app_status.go did not catch it.
	for _, pair := range [][2]string{{"", ""}, {" ", ""}, {"", "\t"}, {"   ", "  "}} {
		if SameSlug(pair[0], pair[1]) {
			t.Errorf("SameSlug(%q, %q) = true, want false — an empty slug names no app, so answering "+
				"'these are the same app' is a false claim the callers act on", pair[0], pair[1])
		}
	}
}

// ---------------------------------------------------------------------------
// Site 3: internal/appapi/appblocks.go — latestMatchingSubmission
// ---------------------------------------------------------------------------

// recoverRowSlug is recoverRow (submit_recover_newest_test.go) with the blockId
// under the caller's control, which that helper hard-codes.
func recoverRowSlug(id, blockID, version, status, submittedAt string) Submission {
	return Submission{
		ID:          id,
		BlockID:     blockID,
		Version:     version,
		Status:      status,
		SubmittedAt: submittedAt,
	}
}

// TestLatestMatchingSubmissionMatchesARespelledBlockID is site 3's red-at-base
// case. `s.BlockID != slug` dropped the only matching row, so recoverTimedOutSubmit
// found nothing and SubmitVersion reported the submit as timed out — telling an
// author their upload may not have landed while the row proving it landed was in
// the response being scanned.
func TestLatestMatchingSubmissionMatchesARespelledBlockID(t *testing.T) {
	for _, spelling := range []string{"My-Block", " my-block ", "MY-BLOCK", "\tmy-block"} {
		t.Run(spelling, func(t *testing.T) {
			subs := []Submission{
				recoverRowSlug("pubreq_ours", spelling, "0.2.0", "pending", "2026-07-29T10:00:00Z"),
			}
			got := latestMatchingSubmission(subs, "my-block", "0.2.0")
			if got == nil {
				t.Fatalf("latestMatchingSubmission found no match for a row whose blockId is %q — "+
					"that is the same app, and dropping it makes a landed submit report as timed out", spelling)
			}
			if got.ID != "pubreq_ours" {
				t.Errorf("matched %q, want pubreq_ours", got.ID)
			}
		})
	}
}

// TestLatestMatchingSubmissionStillRejectsOtherSlugs is the negative half, and
// the guard against a fix that widened too far. It also pins that the VERSION
// compare stayed EXACT — a deliberate decision recorded at the call site, and
// the thing a blanket "normalise everything" change would have broken silently.
func TestLatestMatchingSubmissionStillRejectsOtherSlugs(t *testing.T) {
	for _, r := range slugRejects {
		t.Run("slug/"+r.name, func(t *testing.T) {
			subs := []Submission{
				recoverRowSlug("pubreq_theirs", r.spelling, "0.2.0", "pending", "2026-07-29T10:00:00Z"),
			}
			if got := latestMatchingSubmission(subs, "my-block", "0.2.0"); got != nil {
				t.Errorf("a row for blockId %q matched a request for %q (%s) — reporting it would name a "+
					"DIFFERENT app's publishRequestId as this submit's result", r.spelling, "my-block", r.breaks)
			}
		})
	}

	// 🔴 THE VERSION COMPARE IS STILL EXACT. If a future change reached for
	// SameSlug on the version too — the obvious "make it consistent" move — this
	// fails. The version's ordering policy lives in comparableVersion
	// (internal/cmd/approved_version.go) and declares a pre-release NOT
	// ORDERABLE; a fold-and-trim compare here would contradict that quietly.
	for _, v := range []string{" 0.2.0", "0.2.0 ", "V0.2.0", "v0.2.0", "0.2.0-rc.1"} {
		t.Run("version/"+v, func(t *testing.T) {
			subs := []Submission{
				recoverRowSlug("pubreq_other_version", "my-block", v, "pending", "2026-07-29T10:00:00Z"),
			}
			if got := latestMatchingSubmission(subs, "my-block", "0.2.0"); got != nil {
				t.Errorf("a row at version %q matched a request for %q — the version compare is deliberately "+
					"EXACT (see the call site); normalising it is a separate decision with its own ordering policy",
					v, "0.2.0")
			}
		})
	}
}

// TestRecoverTimedOutSubmitMatchesARespelledBlockID drives the same site through
// the REAL transport, so the fix is pinned as a property of `civitai app submit`
// and not only of the helper. Without it the only coverage of this path would be
// a direct call, which cannot show that the recovery actually reports success.
func TestRecoverTimedOutSubmitMatchesARespelledBlockID(t *testing.T) {
	rows := []Submission{
		recoverRowSlug("pubreq_ours", " My-Block ", "0.2.0", "pending", "2026-07-29T10:00:00Z"),
	}
	srv, listCalls := newRecoverServer(t, func() []Submission { return rows })

	c := recoverClient(srv.URL)
	res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0")
	if err != nil {
		t.Fatalf("the submit landed — a row for it is in the listing, spelled %q — but the recovery "+
			"reported a timeout: %v", rows[0].BlockID, err)
	}
	// REACHABILITY: the recovery poll must have run, or nothing here is about
	// the row scan.
	if n := atomic.LoadInt32(listCalls); n < 1 {
		t.Fatalf("the recovery never polled /submissions (%d calls)", n)
	}
	if res.PublishRequestID != "pubreq_ours" {
		t.Errorf("PublishRequestID = %q, want pubreq_ours — this is the id the user hands to "+
			"`civitai app withdraw`", res.PublishRequestID)
	}
	// The SERVER's spelling is what gets reported back, not the caller's — the
	// row is the authority on its own blockId.
	if res.Slug != " My-Block " {
		t.Errorf("Slug = %q, want the row's own spelling %q", res.Slug, " My-Block ")
	}
}
