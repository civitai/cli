package appapi

import (
	"context"
	"sync/atomic"
	"testing"
)

// THE NEWEST-FIRST ROW PICK IN latestMatchingSubmission — issue #390.
//
// After a submit POST times out, recoverTimedOutSubmit polls the submissions
// list and reports ONE row's id as the PublishRequestID. That id is what
// `civitai app submit` prints and what the user then hands to
// `civitai app withdraw`, so naming the wrong row aims a withdrawal at the wrong
// submission.
//
// The pick is documented "newest-first, so the first match is latest" and was
// unpinned: every fixture in submit_recover_test.go holds a single row, where
// `subs[0]` and `subs[len(subs)-1]` are the same row. Measured on this branch's
// parent (d9f9c67), reversing the scan direction left the whole suite green —
// 3786 RUN, 0 FAIL.
//
// 🔴 THE FUNCTION IS NOT A PLAIN "NEWEST" PICK, AND THE TESTS BELOW MUST NOT
// PRETEND IT IS. It has two tiers: a non-terminal (pending/submitted) match wins
// outright, and only if none exists does the newest match of any status answer.
// So recency decides WITHIN a tier and the tier decides first — case (c) pins
// exactly that, and it is the one case where the answer is deliberately NOT the
// newest row. Writing "the newest row" as the expectation there would encode a
// contract this function does not have.
//
// What none of this can check is that the SERVER really orders the list
// newest-first. ListSubmissions does not sort and the route offers nothing to
// verify an order against, so these guards pin the CLI's half only; the server
// half stays an unverified dependency of the route's contract. See the ledger in
// internal/cmd/newest_row_pick_test.go, which covers this file too.

// recoverRow builds one submissions row for the recovery poll.
func recoverRow(id, version, status, submittedAt string) Submission {
	return Submission{
		ID:          id,
		BlockID:     "my-block",
		Version:     version,
		Status:      status,
		SubmittedAt: submittedAt,
	}
}

// TestRecoverTimedOutSubmitReadsTheNewestMatchingRow drives the real
// SubmitVersion timeout-recovery path over MULTI-ROW listings whose rows
// disagree, so which end the scan reads is observable.
func TestRecoverTimedOutSubmitReadsTheNewestMatchingRow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rows       []Submission
		wantID     string
		wantStatus string
		why        string
	}{
		{
			// (a) TIER 1, both ends eligible. Two pending rows for the same
			// slug+version — a resubmit after a timed-out submit is exactly how a
			// user ends up with two — so ONLY RECENCY separates them and the id
			// is the only assertion that can discriminate. Sharing the status is
			// the point of this case, not an oversight.
			name: "two pending rows, newest wins",
			rows: []Submission{
				recoverRow("pubreq_new", "0.2.0", "pending", "2026-07-29T10:00:00Z"),
				recoverRow("pubreq_old", "0.2.0", "pending", "2026-07-28T10:00:00Z"),
			},
			wantID:     "pubreq_new",
			wantStatus: "pending",
			why:        "the id reported after a timeout must be the submission that just landed, not its predecessor",
		},
		{
			// (b) TIER 2, the anyMatch fallback. Neither row is non-terminal, so
			// the fallback decides — and it must keep the same newest-first rule
			// rather than latching the last row it walked past.
			name: "no non-terminal row, newest of the fallback matches wins",
			rows: []Submission{
				recoverRow("pubreq_rej", "0.2.0", "rejected", "2026-07-29T10:00:00Z"),
				recoverRow("pubreq_wdn", "0.2.0", "withdrawn", "2026-07-28T10:00:00Z"),
			},
			wantID:     "pubreq_rej",
			wantStatus: "rejected",
			why:        "the fallback tier walks the same newest-first list and must not answer with the oldest row it saw",
		},
		{
			// (c) THE TIERS, NOT RECENCY. The newest row is terminal and an older
			// one is still pending, so the documented preference for a
			// non-terminal row answers with the OLDER id. Expected value written
			// from that contract, not from what the loop happens to return.
			name: "a non-terminal row beats a NEWER terminal one",
			rows: []Submission{
				recoverRow("pubreq_approved", "0.2.0", "approved", "2026-07-29T10:00:00Z"),
				recoverRow("pubreq_pending", "0.2.0", "pending", "2026-07-28T10:00:00Z"),
			},
			wantID:     "pubreq_pending",
			wantStatus: "pending",
			why:        "a non-terminal match wins outright; recency only orders rows WITHIN a tier",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// FIXTURE CONTROL, on the axis the assertion actually turns on: the
			// id. A fixture whose rows share an id collapses every candidate
			// implementation into the same output, which is how this pick went
			// untested in the first place. submittedAt is checked too, so the
			// rows are also distinguishable to a reader deciding which is newest.
			//
			// 🔴 STATUS IS DELIBERATELY NOT REQUIRED TO DIFFER, and case (a)
			// depends on that: two rows BOTH pending is the strongest form of
			// this test, because then nothing but recency separates them and the
			// id assertion is the only thing that can pass. Requiring a distinct
			// status here would have made that case unwritable — the guard would
			// be enforcing a rule stricter than the one that matters.
			seenID, seenAt := map[string]bool{}, map[string]bool{}
			for _, r := range tc.rows {
				if seenID[r.ID] {
					t.Fatalf("two rows carry id %q — the ends of this fixture are indistinguishable "+
						"on the field this test asserts (#390)", r.ID)
				}
				if seenAt[r.SubmittedAt] {
					t.Fatalf("two rows carry submittedAt %q — nothing marks which row is actually newer", r.SubmittedAt)
				}
				seenID[r.ID], seenAt[r.SubmittedAt] = true, true
			}

			rows := tc.rows
			srv, listCalls := newRecoverServer(t, func() []Submission { return rows })

			c := recoverClient(srv.URL)
			res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0")
			if err != nil {
				t.Fatalf("SubmitVersion should recover on timeout, got error: %v", err)
			}
			// REACHABILITY: the recovery poll must have actually run, or the
			// result below came from somewhere other than the row scan.
			if n := atomic.LoadInt32(listCalls); n < 1 {
				t.Fatalf("the recovery never polled /submissions (%d calls), so nothing here is about the row pick", n)
			}
			if res.PublishRequestID != tc.wantID {
				t.Errorf("PublishRequestID = %q, want %q — %s.\n"+
					"This id is what `app submit` prints and what the user hands to `civitai app withdraw` (#390).",
					res.PublishRequestID, tc.wantID, tc.why)
			}
			if res.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q — the reported status must belong to the same row as the id",
					res.Status, tc.wantStatus)
			}
		})
	}
}

// TestRecoverTimedOutSubmitIgnoresOtherVersionsAmongMatches is the negative half:
// with several rows present, the version filter still decides membership, so a
// newer row for a DIFFERENT version must not be reported as this submit's
// success just because it is at the head of the list.
func TestRecoverTimedOutSubmitIgnoresOtherVersionsAmongMatches(t *testing.T) {
	rows := []Submission{
		recoverRow("pubreq_other_version", "0.3.0", "pending", "2026-07-30T10:00:00Z"),
		recoverRow("pubreq_ours", "0.2.0", "pending", "2026-07-29T10:00:00Z"),
		recoverRow("pubreq_ours_old", "0.2.0", "withdrawn", "2026-07-28T10:00:00Z"),
	}
	srv, _ := newRecoverServer(t, func() []Submission { return rows })

	c := recoverClient(srv.URL)
	res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0")
	if err != nil {
		t.Fatalf("SubmitVersion should recover on timeout, got error: %v", err)
	}
	if res.PublishRequestID != "pubreq_ours" {
		t.Errorf("PublishRequestID = %q, want %q — the newest row MATCHING the submitted version, "+
			"not the newest row overall", res.PublishRequestID, "pubreq_ours")
	}
	if res.Version != "0.2.0" {
		t.Errorf("Version = %q, want the submitted version 0.2.0", res.Version)
	}
}

// TestLatestMatchingSubmissionPrefersNonTerminalOverNewer asserts the tier rule
// directly on the helper, so the preference is a statement about the function
// rather than about one transport path through it.
func TestLatestMatchingSubmissionPrefersNonTerminalOverNewer(t *testing.T) {
	// The tiering is behaviour, asserted directly on the helper rather than
	// through the transport, so this stays a statement about the function.
	subs := []Submission{
		recoverRow("pubreq_newest_terminal", "0.2.0", "approved", "2026-07-29T10:00:00Z"),
		recoverRow("pubreq_older_pending", "0.2.0", "pending", "2026-07-28T10:00:00Z"),
	}
	got := latestMatchingSubmission(subs, "my-block", "0.2.0")
	if got == nil {
		t.Fatal("no match found for a listing that contains two matching rows")
	}
	if got.ID != "pubreq_older_pending" {
		t.Errorf("latestMatchingSubmission = %q, want the non-terminal row %q", got.ID, "pubreq_older_pending")
	}
	// A slug that matches nothing must answer nil rather than the head of the
	// list — the positive control for the filter itself.
	if other := latestMatchingSubmission(subs, "someone-else", "0.2.0"); other != nil {
		t.Errorf("a non-matching slug returned %q; an unmatched lookup must be nil", other.ID)
	}
}
