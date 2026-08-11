package cmd

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// Issue #223: `civitai app view <slug>` 404s for an app that `civitai app status`
// reports as approved + live and that really is serving traffic. The two commands
// read DIFFERENT resources — the public store catalog (GET /api/v1/apps/{slug})
// vs the author submission pipeline (GET /api/v1/blocks/submissions) — so the 404
// is truthful and the defect is the MESSAGE. These tests pin the message, the
// three ways it must NOT fire, and the exit-code contract on all of them.
//
// 🔴 The exit code is asserted with errors.Is (AGENTS.md item 7), never by
// message text: the classification sentinels carry no visible text, so a text
// assertion says nothing about rc=4.

// appViewAdviceMarkers are the load-bearing pieces of the owned-slug advice. A
// "plain 404" case must contain NONE of them.
var appViewAdviceMarkers = []string{
	"one of your own apps",
	"civitai app listing status",
	"civitai app status",
}

// appViewServer wires the store 404 and a submissions responder onto one
// httptest server, returning a counter of submissions-route hits so a test can
// prove the ownership probe ran (or did not).
func appViewServer(t *testing.T, subs func(w http.ResponseWriter, r *http.Request)) *int32 {
	t.Helper()
	var probes int32
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"App not found"}`))
		case r.URL.Path == "/api/v1/blocks/submissions":
			atomic.AddInt32(&probes, 1)
			// The probe must narrow server-side by slug; a probe that asked for
			// the whole listing would be reading someone else's newest row.
			if got := r.URL.Query().Get("blockId"); got != "buzz-generator" {
				t.Errorf("submissions probe should narrow by blockId, got %q", got)
			}
			subs(w, r)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return &probes
}

// ownedSubmissionBody is the owned-slug listing the probe reads, newest first.
//
// 🔴 IT HAS TWO ROWS THAT DISAGREE ON EVERY FIELD THE ADVICE PRINTS, and that is
// the point (#390). It used to be ONE row, where `subs[0]` and
// `subs[len(subs)-1]` are the same row — so ownedSubmission's choice of row was
// unobservable here, and reversing that choice left the whole suite green
// (measured on this branch's parent: 3786 RUN, 0 FAIL, 4 SKIP-of-others).
//
// The newest row is an un-deployed resubmission over an approved/live
// predecessor, because that is the state a user is actually in when `app view`
// 404s on an app they own — and it is the state whose two ends give OPPOSITE
// answers to "is your deploy broken?".
//
// id, version, status, deployState, submittedAt and liveUrl all differ between
// the rows. A field shared across rows cannot tell the ends apart, so sharing
// one would silently re-open this hole on whichever axis an assertion uses.
const ownedSubmissionBody = `{"submissions":[
  {"id":"pubreq_02H","blockId":"buzz-generator","version":"0.2.0","status":"pending",
   "deployState":"building","submittedAt":"2026-07-29T10:00:00Z","updatedAt":"2026-07-29T10:00:00Z",
   "createdAt":"2026-07-29T10:00:00Z","liveUrl":null},
  {"id":"pubreq_01H","blockId":"buzz-generator","version":"0.1.1","status":"approved",
   "deployState":"live","submittedAt":"2026-07-28T10:00:00Z","updatedAt":"2026-07-28T10:00:00Z",
   "createdAt":"2026-07-28T10:00:00Z","liveUrl":"https://buzz-generator.civit.ai/"}]}`

// (a) 404 on a slug the caller OWNS → the actionable message, and rc is STILL 4.
func TestAppViewNotFoundOnOwnedSlugExplains(t *testing.T) {
	probes := appViewServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ownedSubmissionBody))
	})

	_, _, err := run(t, "app", "view", "buzz-generator")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	// The exit-code contract is unchanged by the richer message.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("404 must still classify as ErrNotFound (rc=4), got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(probes); n != 1 {
		t.Errorf("ownership probe should run exactly once on the 404 path, ran %d times", n)
	}
	msg := err.Error()
	// The original server message survives — the advice is appended, not swapped in.
	if !strings.Contains(msg, "App not found") {
		t.Errorf("the server's own 404 message must survive, got: %v", err)
	}
	for _, want := range appViewAdviceMarkers {
		if !strings.Contains(msg, want) {
			t.Errorf("owned-slug advice missing %q, got: %v", want, err)
		}
	}
	// It must name the listing command with the slug so the next step is
	// copy-pasteable.
	for _, want := range []string{"civitai app listing status --slug buzz-generator", "civitai app status buzz-generator"} {
		if !strings.Contains(msg, want) {
			t.Errorf("owned-slug advice missing %q, got: %v", want, err)
		}
	}
	// And it must report what the submissions route actually said — asserted on
	// the `(submission status: X, deploy: Y)` PAIR rather than on the bare words,
	// because the prose around it already contains "the live URL": a
	// `Contains(msg, "live")` check passes on a message that never read the
	// deployState at all, which is the spelled-guard shape. These two values are
	// written out by hand from ownedSubmissionBody's NEWEST row, not read back
	// from the row the implementation picked.
	assertOwnedStatePair(t, msg, "pending", "building")
}

// (b) 404 on a slug the caller does NOT own → the plain not-found, no claim about
// anybody's store listing.
func TestAppViewNotFoundNotOwnedStaysPlain(t *testing.T) {
	probes := appViewServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"submissions":[]}`))
	})

	_, _, err := run(t, "app", "view", "buzz-generator")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("404 should classify as ErrNotFound, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(probes); n != 1 {
		t.Errorf("the probe must have RUN (positive control), ran %d times", n)
	}
	assertNoOwnedAdvice(t, err)
}

// (b') The server ignoring ?blockId= must not be readable as ownership: a row for
// a DIFFERENT app is not evidence about this slug.
func TestAppViewNotFoundForeignRowIsNotOwnership(t *testing.T) {
	probes := appViewServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"submissions":[{
		  "id":"pubreq_02H","blockId":"someone-elses-app","version":"1.0.0","status":"approved",
		  "deployState":"live","submittedAt":"2026-07-28T10:00:00Z","updatedAt":"2026-07-28T10:00:00Z",
		  "createdAt":"2026-07-28T10:00:00Z"}]}`))
	})

	_, _, err := run(t, "app", "view", "buzz-generator")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("404 should classify as ErrNotFound, got %T: %v", err, err)
	}
	if n := atomic.LoadInt32(probes); n != 1 {
		t.Errorf("the probe must have RUN (positive control), ran %d times", n)
	}
	assertNoOwnedAdvice(t, err)
}

// (c) The ownership lookup ITSELF fails → fall back to the plain 404. Reading
// nothing is not finding nothing: a "your listing is unpublished" claim we could
// not check is a false diagnosis, worse than the terse 404.
func TestAppViewNotFoundProbeFailureFallsBack(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusInternalServerError, `{"message":"boom"}`},
		{"not an apps author", http.StatusForbidden, `{"message":"invite-only"}`},
		{"submissions route missing", http.StatusNotFound, `{"message":"no such submission"}`},
		{"unparseable body", http.StatusOK, `<html>gateway</html>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probes := appViewServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, _, err := run(t, "app", "view", "buzz-generator")
			if err == nil {
				t.Fatal("expected a not-found error")
			}
			// The 404 the USER asked about must survive — a failed probe must not
			// re-classify the error as the probe's own failure (that would move
			// the exit code off 4).
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("a failed probe must leave the store 404 intact, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "App not found") {
				t.Errorf("the store's 404 message must survive a failed probe, got: %v", err)
			}
			if n := atomic.LoadInt32(probes); n != 1 {
				t.Errorf("the probe must have RUN (positive control), ran %d times", n)
			}
			assertNoOwnedAdvice(t, err)
		})
	}
}

// (d) The happy path is unchanged — and pays for NO ownership probe.
func TestAppViewHappyPathDoesNotProbeOwnership(t *testing.T) {
	const body = `{
	  "id":"d1","serialId":42,"slug":"buzz-generator","kind":"onsite","name":"Buzz Generator",
	  "tagline":"neat","description":"A longer description.","category":"generation","contentRating":"pg",
	  "creator":{"id":7,"username":"alice","image":""},
	  "recommend":{"recommendedCount":9,"notRecommendedCount":1,"recommendPct":0.9},"reviewCount":10,
	  "screenshots":[],
	  "kindData":{"kind":"onsite","appBlockId":"blk_9","hasPage":true,"liveUrl":"https://buzz-generator.civit.ai"}
	}`
	var probes int32
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/blocks/submissions" {
			atomic.AddInt32(&probes, 1)
			_, _ = w.Write([]byte(ownedSubmissionBody))
			return
		}
		_, _ = w.Write([]byte(body))
	})

	out, _, err := run(t, "app", "view", "buzz-generator")
	if err != nil {
		t.Fatalf("app view: %v", err)
	}
	if n := atomic.LoadInt32(&probes); n != 0 {
		t.Errorf("a successful view must not pay for an ownership probe, made %d", n)
	}
	for _, want := range []string{"Buzz Generator", "buzz-generator", "generation", "alice", "https://buzz-generator.civit.ai"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q:\n%s", want, out)
		}
	}
}

// A non-404 failure must never grow the advice (nor probe): the ownership story
// is a claim about a MISSING store listing, and nothing else.
func TestAppViewNon404DoesNotExplainOwnership(t *testing.T) {
	var probes int32
	setupAuthedAppsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/blocks/submissions" {
			atomic.AddInt32(&probes, 1)
			_, _ = w.Write([]byte(ownedSubmissionBody))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"kaboom"}`))
	})

	_, _, err := run(t, "app", "view", "buzz-generator")
	if err == nil {
		t.Fatal("expected a server error")
	}
	if errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("a 500 must not classify as not-found, got: %v", err)
	}
	if n := atomic.LoadInt32(&probes); n != 0 {
		t.Errorf("only a 404 may trigger the ownership probe, made %d", n)
	}
	assertNoOwnedAdvice(t, err)
}

func assertNoOwnedAdvice(t *testing.T, err error) {
	t.Helper()
	for _, marker := range appViewAdviceMarkers {
		if strings.Contains(err.Error(), marker) {
			t.Errorf("unowned/unverifiable 404 must not claim ownership (%q), got: %v", marker, err)
		}
	}
}
