package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#436 — A REMOVAL THAT NOTHING CAN PUBLISH, BEHIND A ✓.
//
// `removeScreenshot` sends only a `screenshotId`, so the server resolves the
// owning listing from the row itself — and on an APPROVED listing the `apls_…`
// ids `app listing status` prints are the SHADOW revision's rows (`status`
// renders getMyListingForEdit, which idempotently opens that shadow). The
// removal therefore lands in the revision, which is correct; what was wrong is
// that `rm-screenshot` printed `✓ Screenshot removed` and exited 0, while the
// public gallery still served the screenshot and NO command in this CLI could
// submit the revision that held the removal. Measured live in #436 against
// `model-benchmarking`: after the removal `app listing status` reported 3
// screenshots and `GET /api/v1/apps/model-benchmarking` still returned 4.
//
// 🔴 SO EXIT 0 IS NOT THE ASSERTION — THE BUG EXITS 0. What these tests pin is
// WHAT WAS PRINTED (the whole outcome line, normalised, not a word another
// feature can spell) and WHETHER A SUBMIT WAS ISSUED (zero, from this command).
//
// The fixture ids are pairwise distinct AND distinct from every constant these
// assertions name, so a mutant that hardcodes one cannot survive by landing on a
// value the fixture could only ever produce.
const (
	rmParentListingID = "apl_parent_436"
	rmShadowID        = "apl_shadow_852"
	rmPubReqID        = "alpr_pub_913"
	rmScreenshotID    = "apls_target_314"
)

// The two outcome lines, spelled in full. A `Contains("removed")` guard is
// walkable by a reword and satisfied by prose that says nothing about
// publication, which is the defect: the shipped line was TRUE of the revision
// and FALSE of the listing. Pinning the whole normalised line costs a test edit
// on a cosmetic reword; that is the price of a machine-readable claim.
const (
	rmDraftOutcomeLine = "✓ Screenshot removed"
	rmLiveOutcomeLine  = "✓ Screenshot removal staged on an open revision — not submitted for review yet."
)

// rmStub is a listing server that records every listing call it was asked to
// make, and what it was asked to make it against.
type rmStub struct {
	t *testing.T
	// status is the parent listing's lifecycle state (draft|pending|approved).
	status string
	// beginCreated is what beginListingRevision reports: true means it MINTED a
	// shadow, i.e. there was no open revision.
	beginCreated bool
	// submitStatus is the HTTP status submitListingRevision answers with (0 = 200).
	submitStatus int
	// editFails makes the getMyListingForEdit re-read fail.
	editFails bool
	// viewIcon / viewCover are the image ids the re-read reports in each floor
	// slot; 0 leaves the slot empty.
	viewIcon, viewCover int

	mu              sync.Mutex
	calls           []string
	removedID       string
	beginListingID  string
	submitShadowID  string
	submitChangelog string
}

func (s *rmStub) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	listingEnv(t, srv.URL)
}

// input decodes a tRPC mutation body's `json` payload.
func (s *rmStub) input(r *http.Request) map[string]any {
	raw, err := readBodyJSON(r)
	if err != nil {
		s.t.Errorf("decoding the body of %s: %v", r.URL.Path, err)
		return map[string]any{}
	}
	in, _ := raw["json"].(map[string]any)
	if in == nil {
		s.t.Errorf("%s carried no `json` payload: %v", r.URL.Path, raw)
		return map[string]any{}
	}
	return in
}

func (s *rmStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/blocks/submissions"):
		s.calls = append(s.calls, "submissions")
		submissionRow(w, "my-app", "block_1")
	case strings.Contains(p, "getMyListingForApp"):
		s.calls = append(s.calls, "getMyListingForApp")
		trpcData(w, map[string]any{"appListingId": rmParentListingID, "status": s.status, "contentRating": "g"})
	case strings.Contains(p, "removeScreenshot"):
		s.calls = append(s.calls, "removeScreenshot")
		s.removedID, _ = s.input(r)["screenshotId"].(string)
		trpcData(w, map[string]any{})
	case strings.Contains(p, "beginListingRevision"):
		s.calls = append(s.calls, "beginListingRevision")
		s.beginListingID, _ = s.input(r)["listingId"].(string)
		trpcData(w, map[string]any{"shadowId": rmShadowID, "created": s.beginCreated})
	case strings.Contains(p, "submitListingRevision"):
		s.calls = append(s.calls, "submitListingRevision")
		in := s.input(r)
		s.submitShadowID, _ = in["shadowId"].(string)
		s.submitChangelog, _ = in["changelog"].(string)
		if s.submitStatus != 0 {
			w.WriteHeader(s.submitStatus)
			// The server's own wording for the below-floor refusal, verbatim
			// from civitai/cli#400.
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"Listing needs at least an icon and cover before it can be published (missing: cover)."}}}`))
			return
		}
		trpcData(w, map[string]any{"publishRequestId": rmPubReqID, "shadowId": rmShadowID, "slug": "my-app"})
	case strings.Contains(p, "getMyListingForEdit"):
		s.calls = append(s.calls, "getMyListingForEdit")
		if s.editFails {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"boom"}}}`))
			return
		}
		trpcData(w, map[string]any{
			"parentId": rmParentListingID, "slug": "my-app", "status": s.status,
			"assets": map[string]any{
				"icon":        rmSlot(s.viewIcon),
				"cover":       rmSlot(s.viewCover),
				"screenshots": []any{},
			},
		})
	default:
		s.t.Errorf("unexpected path %s", p)
	}
}

func rmSlot(imageID int) map[string]any {
	if imageID == 0 {
		return map[string]any{"imageId": nil}
	}
	return map[string]any{"imageId": imageID}
}

func (s *rmStub) called(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c == name {
			n++
		}
	}
	return n
}

func (s *rmStub) removed() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removedID
}

// outcomeLines splits captured output into trimmed, non-empty lines so an
// assertion can compare a WHOLE line rather than hunt for a substring.
func outcomeLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func hasLine(out, want string) bool {
	for _, l := range outcomeLines(out) {
		if l == want {
			return true
		}
	}
	return false
}

// TestAppListingRmScreenshotOnLiveListingReportsItIsNotPublished is the #436
// regression guard. RED at b1213a3 on the first-line assertion below, which
// finds the bare `✓ Screenshot removed` there instead.
func TestAppListingRmScreenshotOnLiveListingReportsItIsNotPublished(t *testing.T) {
	st := &rmStub{t: t, status: "approved"}
	st.start(t)

	out, _, err := run(t, "app", "listing", "rm-screenshot", rmScreenshotID, "--slug", "my-app")
	if err != nil {
		t.Fatalf("rm-screenshot on a live listing: %v", err)
	}

	// 🔴 THE LOAD-BEARING ASSERTION, and it is not "it exited 0" — the bug does
	// that too. The whole outcome line is pinned, because the shipped line was
	// true of the revision and false of the listing while containing the right
	// words.
	lines := outcomeLines(out)
	if len(lines) == 0 {
		t.Fatalf("rm-screenshot printed nothing at all")
	}
	if lines[0] != rmLiveOutcomeLine {
		t.Errorf("live rm-screenshot's outcome line is:\n  %q\nwant:\n  %q\n"+
			"civitai/cli#436: the removal lands in the SHADOW revision, which no moderator has seen "+
			"and which this command does not submit — a line that does not say so claims a public "+
			"gallery change that has not happened. Full output:\n%s", lines[0], rmLiveOutcomeLine, out)
	}
	// The draft arm's line must be ABSENT, as a whole line: that is the bare
	// success claim #436 reports.
	if hasLine(out, rmDraftOutcomeLine) {
		t.Errorf("live rm-screenshot printed the draft arm's bare success line %q — it is true of the "+
			"revision and false of the live listing:\n%s", rmDraftOutcomeLine, out)
	}
	// And it must NAME the command that publishes the removal. Before #436 there
	// was none, which is why the staged change was unpublishable.
	if !strings.Contains(out, "civitai app listing submit-revision") {
		t.Errorf("live rm-screenshot does not name the command that publishes the removal "+
			"(`civitai app listing submit-revision`):\n%s", out)
	}

	// The removal itself still happens, and carries the id the user typed.
	if got := st.called("removeScreenshot"); got != 1 {
		t.Errorf("removeScreenshot ran %d time(s), want 1", got)
	}
	if got := st.removed(); got != rmScreenshotID {
		t.Errorf("removeScreenshot carried screenshotId %q, want %q", got, rmScreenshotID)
	}

	// 🔴 AND NO SUBMIT. Option (a) of #436 — auto-submitting — is deliberately
	// NOT what shipped: curation is N removals, so the first one must not open a
	// moderator review cycle mid-edit. If that decision is ever reversed, this
	// is the assertion that has to be changed on purpose.
	if got := st.called("submitListingRevision"); got != 0 {
		t.Errorf("submitListingRevision ran %d time(s), want 0 — `rm-screenshot` deliberately does not "+
			"open a review cycle; `app listing submit-revision` is the explicit act", got)
	}
	if got := st.called("beginListingRevision"); got != 0 {
		t.Errorf("beginListingRevision ran %d time(s), want 0 — removeScreenshot resolves the owning "+
			"listing from the row, so nothing here needs a shadow id", got)
	}
}

// TestAppListingRmScreenshotOnUnapprovedListingIsUnchanged is an INVARIANT
// GUARD, not regression coverage: on a draft or pending listing there is no
// shadow, the removal IS the listing, and `✓ Screenshot removed` was already
// true. It pins that #436's fix did not move that arm. (#436 marked the
// draft/pending behaviour INFERRED from the code path, never measured — the
// reporter had no draft listing to test against.)
func TestAppListingRmScreenshotOnUnapprovedListingIsUnchanged(t *testing.T) {
	for _, status := range []string{"draft", "pending"} {
		t.Run(status, func(t *testing.T) {
			st := &rmStub{t: t, status: status}
			st.start(t)

			out, _, err := run(t, "app", "listing", "rm-screenshot", rmScreenshotID, "--slug", "my-app")
			if err != nil {
				t.Fatalf("rm-screenshot on a %s listing: %v", status, err)
			}
			lines := outcomeLines(out)
			if len(lines) != 1 || lines[0] != rmDraftOutcomeLine {
				t.Errorf("a %s listing is edited directly, so the plain line is the honest one.\ngot:\n%s\nwant exactly one line: %q",
					status, out, rmDraftOutcomeLine)
			}
			if strings.Contains(out, "revision") || strings.Contains(out, "moderator") {
				t.Errorf("a %s listing has no revision and reaches no moderator:\n%s", status, out)
			}
			if got := st.removed(); got != rmScreenshotID {
				t.Errorf("removeScreenshot carried screenshotId %q, want %q", got, rmScreenshotID)
			}
			if got := st.called("submitListingRevision"); got != 0 {
				t.Errorf("submitListingRevision ran %d time(s) on a %s listing, want 0", got, status)
			}
		})
	}
}

// TestAppListingSubmitRevisionPublishesTheOpenRevision covers the standalone
// submit #436 identifies as the root cause: before it, `submitListingRevision`
// had exactly one call site (step 7 of the attach flow), so a staged removal was
// unpublishable from this CLI.
func TestAppListingSubmitRevisionPublishesTheOpenRevision(t *testing.T) {
	for _, tc := range []struct {
		name          string
		extra         []string
		wantChangelog string
	}{
		{name: "default changelog", wantChangelog: "Update listing via civitai CLI"},
		{name: "authored changelog", extra: []string{"--changelog", "Dropped the outdated grid shot"},
			wantChangelog: "Dropped the outdated grid shot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &rmStub{t: t, status: "approved", beginCreated: false}
			st.start(t)

			args := append([]string{"app", "listing", "submit-revision", "--slug", "my-app"}, tc.extra...)
			out, _, err := run(t, args...)
			if err != nil {
				t.Fatalf("submit-revision: %v", err)
			}
			if got := st.called("submitListingRevision"); got != 1 {
				t.Fatalf("submitListingRevision ran %d time(s), want 1 — this command exists to issue it", got)
			}
			if got := st.submitShadowID; got != rmShadowID {
				t.Errorf("submitListingRevision was addressed to %q, want the shadow %q", got, rmShadowID)
			}
			if got := st.beginListingID; got != rmParentListingID {
				t.Errorf("beginListingRevision was addressed to %q, want the PARENT %q", got, rmParentListingID)
			}
			if got := st.submitChangelog; got != tc.wantChangelog {
				t.Errorf("submitListingRevision carried changelog %q, want %q", got, tc.wantChangelog)
			}
			for _, want := range []string{rmPubReqID, "pending moderator review", "Your live listing is unchanged"} {
				if !strings.Contains(out, want) {
					t.Errorf("submit-revision output does not say %q:\n%s", want, out)
				}
			}
		})
	}
}

// TestAppListingSubmitRevisionRefusesWhenThereIsNothingToSubmit pins the two
// refusals that keep this command from sending a moderator an empty review.
func TestAppListingSubmitRevisionRefusesWhenThereIsNothingToSubmit(t *testing.T) {
	t.Run("no open revision", func(t *testing.T) {
		// beginListingRevision is idempotent: created=true means it MINTED the
		// shadow, so nothing had been staged in one.
		st := &rmStub{t: t, status: "approved", beginCreated: true}
		st.start(t)

		_, _, err := run(t, "app", "listing", "submit-revision", "--slug", "my-app")
		if err == nil {
			t.Fatal("expected a refusal when no revision was open")
		}
		if !strings.Contains(err.Error(), "no open revision to submit") {
			t.Errorf("the refusal must say what is missing, got: %v", err)
		}
		if !strings.Contains(err.Error(), "rm-screenshot") {
			t.Errorf("the refusal must name a next command to run, got: %v", err)
		}
		if got := st.called("submitListingRevision"); got != 0 {
			t.Errorf("submitListingRevision ran %d time(s), want 0 — an empty revision must not reach "+
				"a moderator", got)
		}
	})

	for _, status := range []string{"draft", "pending"} {
		t.Run("not live: "+status, func(t *testing.T) {
			st := &rmStub{t: t, status: status}
			st.start(t)

			_, _, err := run(t, "app", "listing", "submit-revision", "--slug", "my-app")
			if err == nil {
				t.Fatalf("expected a refusal on a %s listing", status)
			}
			if !strings.Contains(err.Error(), "not live") {
				t.Errorf("the refusal must say the listing is not live, got: %v", err)
			}
			if got := st.called("beginListingRevision"); got != 0 {
				t.Errorf("beginListingRevision ran %d time(s) on a %s listing, want 0 — a listing that "+
					"is edited directly has no revision to open", got, status)
			}
			if got := st.called("submitListingRevision"); got != 0 {
				t.Errorf("submitListingRevision ran %d time(s) on a %s listing, want 0", got, status)
			}
		})
	}
}

// TestAppListingSubmitRevisionBelowFloorStillFails is the deliberate ASYMMETRY
// with civitai/cli#400.
//
// #400's rule is that a below-floor submit refusal is not a failure of a command
// whose job was to ATTACH — the image landed, so `set-icon` exits 0 and reports
// progress. Here the submit IS the job: exiting 0 having submitted nothing would
// be #436's own false-success class, rebuilt inside the command that fixes it.
// So the error stands, the exit code stays 2, and what the command adds is the
// diagnosis — printed as context, never folded into the error text.
func TestAppListingSubmitRevisionBelowFloorStillFails(t *testing.T) {
	for _, tc := range []struct {
		name      string
		icon      int
		cover     int
		editFails bool
		// submitStatus is the HTTP status the submit is refused with; 0 means the
		// below-floor 400.
		submitStatus int
		wantLines    []string
		wantAbsent   []string
	}{
		{
			name: "floor unmet — the refusal is explained", icon: 77,
			wantLines: []string{
				"Nothing was submitted — this listing is below the publish floor.",
				"Still required before publishing: cover.",
			},
			wantAbsent: []string{"staged on an open revision", "Revision submitted"},
		},
		{
			// 🔴 THE CONTROL FOR THE 400 GATE. A 503 says nothing about the
			// listing, so an outage must not be reported as the publish floor —
			// and with only 400 rows in this table, deleting that gate survives a
			// fully green suite. The floor is left unmet so the ONLY thing
			// stopping the diagnosis is the status.
			name: "an outage is not the floor", icon: 77, submitStatus: http.StatusServiceUnavailable,
			wantAbsent: []string{"below the publish floor", "Still required before publishing"},
		},
		{
			// The floor is MET, so the 400 was about something else and this
			// command has nothing to add — the server's sentence is the story.
			name: "floor met — no diagnosis is invented", icon: 77, cover: 88,
			wantAbsent: []string{"below the publish floor", "Still required before publishing"},
		},
		{
			// It cannot read the listing back, so it claims nothing.
			name: "the re-read fails — nothing is claimed", icon: 77, editFails: true,
			wantAbsent: []string{"below the publish floor", "Still required before publishing"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := tc.submitStatus
			if status == 0 {
				status = http.StatusBadRequest
			}
			st := &rmStub{
				t: t, status: "approved",
				submitStatus: status,
				viewIcon:     tc.icon, viewCover: tc.cover, editFails: tc.editFails,
			}
			st.start(t)

			out, _, err := run(t, "app", "listing", "submit-revision", "--slug", "my-app")
			if err == nil {
				t.Fatalf("a refused submit must FAIL — the user asked for a submit and got none:\n%s", out)
			}
			// The server's refusal reaches the exit-code classifier untouched
			// (AGENTS item 7): a 400 exits 2, a 503 keeps its own code.
			wantTag := civitai.ErrBadRequest
			if status == http.StatusServiceUnavailable {
				wantTag = civitai.ErrNetwork
			}
			if !errors.Is(err, wantTag) {
				t.Errorf("the submit's %d must reach the exit-code classifier intact, got %T: %v", status, err, err)
			}
			for _, want := range tc.wantLines {
				if !hasLine(out, want) {
					t.Errorf("output has no line %q:\n%s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("output must not say %q here:\n%s", absent, out)
				}
			}
		})
	}
}
