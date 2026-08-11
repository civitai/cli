package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// pullDisambiguationServer answers BOTH routes `app pull` now uses: the
// clone-info query (always 404 — the server cannot tell "no such app" from "not
// approved yet", which is the whole bug) and the submissions list the CLI
// consults to tell them apart.
//
// submissions is written straight to the wire so a row can carry a NULL
// appBlockId, which is the actual discriminator: the server nulls it until a
// version is APPROVED.
//
// 🔴 IT FILTERS ON `?blockId=`, LIKE THE REAL ROUTE, AND THAT IS LOAD-BEARING.
// It used to ignore the query and hand its fixture back for any request, which
// made TestAppPullAppBlockIDSelectorKeepsTheServerAnswer vacuous: that test's
// fixture is empty, so the run took the len(subs)==0 branch and was
// byte-identical to TestAppPullUnknownSlugKeepsTheServerAnswer below — the
// appBlockId in `--app` decided nothing. The real route's zod query schema
// accepts only `id` and `blockId` (see appapi.ListSubmissionsCap), and `blockId`
// IS the slug, so only a filtering fake can tell a slug selector from an
// appBlockId one.
func pullDisambiguationServer(t *testing.T, submissions string, submissionsStatus int) *httptest.Server {
	t.Helper()
	return pullDisambiguationServerRec(t, submissions, submissionsStatus, nil)
}

// pullDisambiguationServerRec is the same server, additionally recording every
// `?blockId=` filter it was asked for (in order) into seen. The REQUEST is the
// only place the selector's spelling is observable — the reply for an appBlockId
// is empty either way — so a test that wants to pin "the appBlockId went out
// verbatim as the slug filter" has to read it here.
func pullDisambiguationServerRec(t *testing.T, submissions string, submissionsStatus int, seen *[]string) *httptest.Server {
	t.Helper()
	// Parsed HERE, on the test goroutine: a t.Fatalf inside the handler would
	// run on the server's goroutine, where FailNow is not allowed.
	rows := parseSubmissionRows(t, submissions)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blocks/submissions") {
			filter := r.URL.Query().Get("blockId")
			if seen != nil {
				*seen = append(*seen, filter)
			}
			if submissionsStatus != 0 && submissionsStatus != http.StatusOK {
				w.WriteHeader(submissionsStatus)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "nope"})
				return
			}
			_, _ = w.Write([]byte(filterSubmissionRows(rows, filter)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "App block not found", "code": 404}},
		})
	}))
}

// parseSubmissionRows splits a fixture envelope into its rows, kept as raw JSON.
// Rows are NEVER round-tripped through appapi.Submission: that would render a
// null appBlockId as Go's zero value, and the null is the discriminator every
// test in this file turns on.
func parseSubmissionRows(t *testing.T, body string) []json.RawMessage {
	t.Helper()
	if strings.TrimSpace(body) == "" {
		return nil
	}
	var wrap struct {
		Submissions []json.RawMessage `json:"submissions"`
	}
	if err := json.Unmarshal([]byte(body), &wrap); err != nil {
		t.Fatalf("submissions fixture is not a `{\"submissions\":[…]}` envelope: %v\n%s", err, body)
	}
	for _, row := range wrap.Submissions {
		var only struct {
			BlockID string `json:"blockId"`
		}
		if err := json.Unmarshal(row, &only); err != nil {
			t.Fatalf("submissions fixture row is not an object: %v\n%s", err, row)
		}
	}
	return wrap.Submissions
}

// filterSubmissionRows applies the route's own `where.slug` filter. An empty
// blockID is the UNFILTERED listing the route also allows.
func filterSubmissionRows(rows []json.RawMessage, blockID string) string {
	kept := make([]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		if blockID == "" {
			kept = append(kept, row)
			continue
		}
		var only struct {
			BlockID string `json:"blockId"`
		}
		// Parsed at construction time, so this cannot fail here.
		_ = json.Unmarshal(row, &only)
		if only.BlockID == blockID {
			kept = append(kept, row)
		}
	}
	out, _ := json.Marshal(map[string]any{"submissions": kept})
	return string(out)
}

func pullEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	stubGit(t, func(dir string, args ...string) error {
		t.Error("git must not run when the clone info could not be resolved")
		return nil
	})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
}

// TestAppPullNotApprovedSaysSoInsteadOfDenyingTheApp is the civitai/cli#359
// regression: `app pull` answered a never-approved app with "no such app for
// your account — check the slug with `civitai app status`", and `app status`
// then listed the app the error had just denied.
func TestAppPullNotApprovedSaysSoInsteadOfDenyingTheApp(t *testing.T) {
	srv := pullDisambiguationServer(t, `{"submissions":[
		{"id":"p2","blockId":"my-block","appBlockId":null,"version":"0.1.1","status":"pending"},
		{"id":"p1","blockId":"my-block","appBlockId":null,"version":"0.1.0","status":"withdrawn"}
	]}`, http.StatusOK)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected an error when nothing is approved yet")
	}
	msg := err.Error()

	// 🔴 The remedy that disproved the error. The app IS listed by `app status`.
	if strings.Contains(msg, "no such app for your account") {
		t.Errorf("the app exists — the error must not deny it; got: %s", msg)
	}
	for _, want := range []string{
		"no approved version yet",     // the real precondition
		"civitai app status my-block", // where to see the review state
		"0.1.1 pending",               // the newest submission's actual state
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got: %s", want, msg)
		}
	}
	// AGENTS.md item 7: the exit code is pinned by the sentinel, not the text.
	// This message replaces a not-found error and must stay exit 4.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("the replacement message must keep the not-found classification (exit 4), got %T: %v", err, err)
	}
}

// TestAppPullTerminalSubmissionIsNotDescribedAsInReview: the CLI has
// `subs[0].Status` in hand, so a submission that is REJECTED or WITHDRAWN — no
// review pending, nothing arriving — must not be answered with "check where it
// is in review". The next step there is a new submission.
func TestAppPullTerminalSubmissionIsNotDescribedAsInReview(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   string
	}{
		{"rejected", "REJECTED"},
		{"withdrawn", "WITHDRAWN"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			srv := pullDisambiguationServer(t, `{"submissions":[
				{"id":"p1","blockId":"my-block","appBlockId":null,"version":"0.2.0","status":"`+tc.status+`"}
			]}`, http.StatusOK)
			defer srv.Close()
			pullEnv(t, srv)

			_, _, err := run(t, "app", "pull", "--app", "my-block")
			if err == nil {
				t.Fatal("expected an error when nothing is approved yet")
			}
			msg := err.Error()
			if strings.Contains(msg, "check where it is in review") {
				t.Errorf("a %s submission is not in review; got: %s", tc.status, msg)
			}
			for _, want := range []string{tc.want, "civitai app submit"} {
				if !strings.Contains(msg, want) {
					t.Errorf("missing %q; got: %s", want, msg)
				}
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("want not-found classification (exit 4), got %T: %v", err, err)
			}
		})
	}
}

// TestAppPullPendingSubmissionStillPointsAtReview is the negative control for
// the guard above: a PENDING submission genuinely is in review, and the review
// pointer must not have been stripped for everyone.
func TestAppPullPendingSubmissionStillPointsAtReview(t *testing.T) {
	srv := pullDisambiguationServer(t, `{"submissions":[
		{"id":"p1","blockId":"my-block","appBlockId":null,"version":"0.1.1","status":"pending"}
	]}`, http.StatusOK)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "check where it is in review with `civitai app status my-block`") {
		t.Errorf("a pending submission IS in review; got: %v", err)
	}
}

// TestAppPullBlankSubmissionStateDropsTheParenthetical: the row's version and
// status are server-supplied and can both be blank, which rendered the literal
// `(latest submission:  )` — a field the CLI is telling the user to read, with
// nothing in it.
func TestAppPullBlankSubmissionStateDropsTheParenthetical(t *testing.T) {
	srv := pullDisambiguationServer(t, `{"submissions":[
		{"id":"p1","blockId":"my-block","appBlockId":null,"version":"","status":""}
	]}`, http.StatusOK)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "latest submission") {
		t.Errorf("with no version and no status there is nothing to report; got: %s", msg)
	}
	if !strings.Contains(msg, "no approved version yet") {
		t.Errorf("the explanation itself must survive a blank row; got: %s", msg)
	}
}

// TestAppPullAppBlockIDSelectorKeepsTheServerAnswer is an INVARIANT GUARD, not
// regression coverage: it pins a known, deliberate limit rather than a fixed
// defect. `--app` accepts a slug OR an appBlockId, but ListSubmissions filters on
// `blockId` (which IS the slug), so an appBlockId matches no row and the
// disambiguation declines. That costs nothing — the server nulls `appBlockId`
// until a version is APPROVED, so a never-approved app has no appBlockId to
// type — and the README says so. If someone later resolves the selector, this
// test is what tells them the README paragraph moves with it.
//
// 🔴 IT PINS THE REQUEST, BECAUSE THE REPLY CANNOT DISCRIMINATE. The first
// version asserted only the message, against a `{"submissions":[]}` fixture and
// a fake that ignored the query — so it took the len(subs)==0 branch and was
// byte-identical to TestAppPullUnknownSlugKeepsTheServerAnswer below. Measured:
// a mutant implementing the very future fix this comment says it would catch,
// PLUS swapping `ab_0192abc` for a plain unknown slug, left all 33 TestAppPull*
// tests passing. Nothing about the appBlockId was load-bearing.
//
// Note what CANNOT be pinned, so nobody re-derives it: the OUTCOME can never
// differ. Any world where a user can type an appBlockId is a world where some
// row carries one, and explainMissingApp scans every row and defers to `orig`
// the moment it finds one — so "resolve the selector" reaches the same fallback
// by a different route. The observable that does move is the REQUEST: today the
// `--app` value goes out verbatim as `?blockId=`, untranslated. That is the
// mechanism the README paragraph describes, and reading it is what makes both
// the selector's spelling and the future fix visible to this test.
func TestAppPullAppBlockIDSelectorKeepsTheServerAnswer(t *testing.T) {
	// A REAL app with nothing approved — the state the good message exists for.
	// The empty fixture this replaces gave the server nothing to give ANYONE,
	// which is what made a passing run meaningless.
	const fixture = `{"submissions":[
		{"id":"p1","blockId":"my-block","appBlockId":null,"version":"0.3.0","status":"pending"}
	]}`

	var seen []string
	srv := pullDisambiguationServerRec(t, fixture, http.StatusOK, &seen)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "ab_0192abc")
	if err == nil {
		t.Fatal("expected an error")
	}
	// THE MECHANISM: the appBlockId left as the slug filter, verbatim.
	if len(seen) != 1 || seen[0] != "ab_0192abc" {
		t.Errorf("the disambiguation is keyed on the SLUG and must send `--app` verbatim as `?blockId=`; "+
			"the lookup asked for %q. If the selector is now resolved, the README paragraph for "+
			"`app pull` moves with it — it tells readers an appBlockId is never the not-approved case.", seen)
	}
	// THE CONSEQUENCE: a slug-keyed lookup matches no row, so the server's answer stands.
	if !strings.Contains(err.Error(), "no such app for your account") {
		t.Errorf("an appBlockId selector cannot be disambiguated by a slug-keyed lookup; got: %v", err)
	}
	if strings.Contains(err.Error(), "no approved version yet") {
		t.Errorf("the disambiguation must decline for an appBlockId selector, not guess; got: %v", err)
	}

	// POSITIVE CONTROL, on the SAME server: the fixture really can produce the
	// other answer, reached by the slug. Without it, both assertions above are
	// equally satisfied by a server with nothing to give anyone.
	seen = nil
	_, _, slugErr := run(t, "app", "pull", "--app", "my-block")
	if slugErr == nil || !strings.Contains(slugErr.Error(), "no approved version yet") {
		t.Fatalf("the fixture cannot produce the good message for ANY selector — the assertions above are vacuous; got: %v", slugErr)
	}
	if len(seen) != 1 || seen[0] != "my-block" {
		t.Fatalf("the positive control did not reach the lookup with the slug; asked for %q", seen)
	}
}

// TestAppPullUnknownSlugKeepsTheServerAnswer is the negative control: with no
// submissions at all the original message is the CORRECT one, and must survive.
// Without this, "always say not-approved" would pass the test above.
func TestAppPullUnknownSlugKeepsTheServerAnswer(t *testing.T) {
	srv := pullDisambiguationServer(t, `{"submissions":[]}`, http.StatusOK)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "ghost")
	if err == nil {
		t.Fatal("expected an error for a slug that matches nothing")
	}
	if !strings.Contains(err.Error(), "no such app for your account") {
		t.Errorf("a genuine unknown slug must keep the server's answer; got: %v", err)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want not-found classification, got %T: %v", err, err)
	}
}

// TestAppPullApprovedElsewhereKeepsTheServerAnswer: an approved version exists
// (appBlockId is set) yet clone info still 404s. "Not approved yet" would be a
// NEW false explanation replacing the old one, so the CLI must not guess.
func TestAppPullApprovedElsewhereKeepsTheServerAnswer(t *testing.T) {
	srv := pullDisambiguationServer(t, `{"submissions":[
		{"id":"p1","blockId":"my-block","appBlockId":"ab_1","version":"1.0.0","status":"approved"}
	]}`, http.StatusOK)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "no approved version yet") {
		t.Errorf("an approved version exists — the CLI must not claim otherwise; got: %v", err)
	}
}

// TestAppPullDisambiguationFailureKeepsTheServerAnswer: the lookup that would
// disambiguate can itself fail. A disambiguation that cannot be made must never
// REPLACE (or swallow) the server's answer.
func TestAppPullDisambiguationFailureKeepsTheServerAnswer(t *testing.T) {
	srv := pullDisambiguationServer(t, "", http.StatusInternalServerError)
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "no such app for your account") {
		t.Errorf("a failed lookup must fall back to the server's own answer; got: %v", err)
	}
}

// TestAppPullNotApprovedDoesNotRegressTheHappyPath: the disambiguation must only
// run on a not-found. A 403 is a different question and keeps its own message.
func TestAppPullForbiddenIsNotDisambiguated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/blocks/submissions") {
			t.Error("a 403 must not trigger the submissions lookup")
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": "nope", "code": 403}},
		})
	}))
	defer srv.Close()
	pullEnv(t, srv)

	_, _, err := run(t, "app", "pull", "--app", "my-block")
	if err == nil || !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("403 must keep its own message, got: %v", err)
	}
}
