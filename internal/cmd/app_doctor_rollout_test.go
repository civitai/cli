package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// `civitai app doctor`'s ACCOUNT-level App-Blocks rollout section.
//
// The defect it closes (civitai/civitai-app-starters#537): a developer's block
// storage calls answered `401 "Apps are not enabled"`; they verified the app was
// approved and that the token carried both storage scopes, and could not explain
// it, because the gate is an ACCOUNT-level flag orthogonal to both. The listing
// half of this command could never have reported it — `appListings.listMine` is
// gated on a DIFFERENT flag (`app-blocks-author`) and nothing on its wire path
// consults `app-blocks-enabled`.
//
// Everything here runs against an httptest fake; see internal/appapi/
// rollout_test.go's header for what has and has not been measured live.

// rolloutProbeResponse is one fake answer to the probe.
type rolloutProbeResponse struct {
	status int
	body   string
}

// The seven outcomes, each with the fake response that produces it. Spelled as
// a table because four separate tests below have to walk ALL of them, and a
// per-test list is how one of them silently stops covering a state.
//
// 🔴 NOT ONE OF THESE BODIES IS THE PRODUCTION WORDING FOR ITS OWN STATUS. The
// implementation keys on the status; a fixture quoting the real message at each
// arm could not tell that from a text match.
var rolloutOutcomeFixtures = []struct {
	state string
	resp  rolloutProbeResponse
	// wantEnrolled is the `--json` spelling of the tristate: "true", "false" or
	// "null". A LITERAL, never derived from the implementation.
	wantEnrolled string
}{
	{string(appapi.RolloutEnrolled), rolloutProbeResponse{http.StatusOK, `{"submissions":[]}`}, "true"},
	{string(appapi.RolloutNotEnrolled), rolloutProbeResponse{http.StatusServiceUnavailable, `{"message":"the gate refused this subject"}`}, "false"},
	{string(appapi.RolloutAuthorRefused), rolloutProbeResponse{http.StatusForbidden, `{"message":"refused"}`}, "null"},
	{string(appapi.RolloutNoCredential), rolloutProbeResponse{http.StatusUnauthorized, `{"message":"refused"}`}, "null"},
	{string(appapi.RolloutRateLimited), rolloutProbeResponse{http.StatusTooManyRequests, `{"message":"slow down"}`}, "null"},
	{string(appapi.RolloutUnknown), rolloutProbeResponse{http.StatusServiceUnavailable, `<html>503 from the edge</html>`}, "null"},
	// 🔴 UNREACHABLE HAS NO (status, body) — IT IS THE ABSENCE OF A RESPONSE,
	// which is exactly why it must not share an arm with any of the above. Its
	// fake is a closed listener, so it is driven by its own test below rather
	// than from this table.
}

// ---------------------------------------------------------------------------
// The exit code. This is the half that could break someone else's release.
// ---------------------------------------------------------------------------

// TestDoctorRolloutNeverChangesTheExitCode is the load-bearing test in this
// file.
//
// 🔴 THE DECISION IS WARN, NOT FAIL, AND THIS IS WHERE IT IS PINNED. A developer
// outside the rollout is in a SUPPORTED state — the flag is deliberately dark
// pre-GA and the remedy is a cohort invitation they cannot issue themselves —
// while `doctor`'s exit 1 is published as "a blocking problem on a listing that
// can still publish" and is wired into release scripts on that promise. Gating
// on an account-level cohort would make every non-cohort developer's release red
// forever for a cause they cannot act on: exactly the failure this command's own
// delisted rule exists to prevent.
//
// 🔴 BOTH ARMS, BECAUSE ONE CANNOT TELL "DOES NOT CHANGE THE CODE" FROM "ALWAYS
// EXITS 0". The clean arm proves no rollout outcome introduces a failure; the
// blocked arm proves none of them MASKS one either — a check that swallowed the
// listing verdict would pass a clean-only table.
//
// 🔴 AND THE 503 ARM IS THE ONE TO WATCH. `civitai.TagStatus` classifies 503 as
// ErrNetwork, which cmd/civitai's mapper publishes as exit 5, so a probe whose
// result was ever propagated as an error turns a healthy run into a transport
// failure. appapi.CheckAppBlocksRollout returns no error at all, which is what
// makes that structural rather than a promise — and this test is what notices if
// that signature changes.
func TestDoctorRolloutNeverChangesTheExitCode(t *testing.T) {
	for _, f := range rolloutOutcomeFixtures {
		t.Run(f.state+"/clean listing exits 0", func(t *testing.T) {
			newDoctorServerRollout(t, f.resp.status, f.resp.body,
				doctorRow(docSlugA, docListingA, "draft", "owner", nil))
			stdout, _, err := run(t, "app", "doctor")
			if err != nil {
				t.Fatalf("rollout state %q turned a clean run into a failure (%v) — the rollout check must "+
					"never set the exit code.\nstdout:\n%s", f.state, err, stdout)
			}
		})
		t.Run(f.state+"/blocked listing still exits 1", func(t *testing.T) {
			newDoctorServerRollout(t, f.resp.status, f.resp.body,
				doctorRow(docSlugA, docListingA, "draft", "owner", nil,
					doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
			stdout, _, err := run(t, "app", "doctor")
			if err == nil {
				t.Fatalf("rollout state %q MASKED a blocking listing problem — the verdict must still be "+
					"the listing's.\nstdout:\n%s", f.state, stdout)
			}
			if !errors.Is(err, ErrListingBlocked) {
				t.Errorf("rollout state %q changed WHICH error the command returns: %v. The sentinel must "+
					"stay ErrListingBlocked, because that is what pins exit 1 (AGENTS.md item 7).", f.state, err)
			}
			// 🔴 THE CLASSIFICATION, NOT JUST THE SENTINEL. A 503 reaching the
			// exit mapper tagged ErrNetwork would publish exit 5 while the
			// sentinel assertion above still passed — the exact "a field exists
			// but nothing branches on it" shape, in the error chain.
			for _, kind := range []struct {
				sentinel error
				name     string
				code     string
			}{
				{civitai.ErrNetwork, "civitai.ErrNetwork", "5"},
				{civitai.ErrUnauthorized, "civitai.ErrUnauthorized", "3"},
				{civitai.ErrRateLimited, "civitai.ErrRateLimited", "6"},
				{civitai.ErrBadRequest, "civitai.ErrBadRequest", "2"},
				{civitai.ErrNotFound, "civitai.ErrNotFound", "4"},
			} {
				if errors.Is(err, kind.sentinel) {
					t.Errorf("rollout state %q tagged the listing verdict %s, which cmd/civitai publishes "+
						"as exit %s instead of 1", f.state, kind.name, kind.code)
				}
			}
		})
	}

	// 🔴 NEGATIVE CONTROL ON THE WHOLE TABLE. Every arm above reads its verdict
	// through `run`, so a harness that never surfaced an error would make the
	// clean arms vacuous. This drives the SAME harness to a different code.
	t.Run("control: the harness can still observe a failure", func(t *testing.T) {
		newDoctorServerRollout(t, http.StatusOK, rolloutEnrolledBody,
			doctorRow(docSlugA, docListingA, "draft", "owner", nil))
		if _, _, err := run(t, "app", "doctor", "not-an-app-of-mine"); !errors.Is(err, civitai.ErrNotFound) {
			t.Fatalf("control: an unknown slug did not come back as ErrNotFound (%v) — the clean arms above "+
				"prove nothing if this harness cannot report a failure", err)
		}
	})
}

// ---------------------------------------------------------------------------
// The payload.
// ---------------------------------------------------------------------------

// TestDoctorRolloutJSONCoversEveryState walks every outcome through the REAL
// command and asserts the published shape.
//
// 🔴 `enrolled` IS ASSERTED AS THE RAW JSON SPELLING, NOT AS A Go bool. The
// whole point of the tristate is that `null` reaches a consumer as `null` rather
// than as `false`, and a decode into `*bool` would make the two
// indistinguishable in the assertion the same way it is in a naive consumer.
func TestDoctorRolloutJSONCoversEveryState(t *testing.T) {
	for _, f := range rolloutOutcomeFixtures {
		t.Run(f.state, func(t *testing.T) {
			newDoctorServerRollout(t, f.resp.status, f.resp.body,
				doctorRow(docSlugA, docListingA, "draft", "owner", nil))
			stdout, _, err := run(t, "app", "doctor", "--json")
			if err != nil {
				t.Fatalf("app doctor --json: %v", err)
			}
			var raw struct {
				Rollout struct {
					State      string          `json:"state"`
					Enrolled   json.RawMessage `json:"enrolled"`
					Detail     string          `json:"detail"`
					Fix        string          `json:"fix"`
					HTTPStatus int             `json:"httpStatus"`
				} `json:"rollout"`
			}
			if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
				t.Fatalf("payload did not parse (%v):\n%s", err, stdout)
			}
			if raw.Rollout.State != f.state {
				t.Errorf("state = %q, want %q", raw.Rollout.State, f.state)
			}
			if got := string(raw.Rollout.Enrolled); got != f.wantEnrolled {
				t.Errorf("enrolled = %s, want %s — `null` and `false` are different answers and a consumer "+
					"must be able to tell them apart", got, f.wantEnrolled)
			}
			// Every state owes the reader a sentence. An empty detail is a state
			// nobody wrote copy for, which reads as a blank section.
			if strings.TrimSpace(raw.Rollout.Detail) == "" {
				t.Errorf("state %q published an EMPTY detail — a blank is indistinguishable from a read "+
					"that failed and was swallowed", f.state)
			}
			if raw.Rollout.HTTPStatus != f.resp.status {
				t.Errorf("httpStatus = %d, want %d", raw.Rollout.HTTPStatus, f.resp.status)
			}
		})
	}
}

// TestDoctorRolloutUnreachableIsNotNotEnrolled is the arm the outcome table
// cannot express: an absence has many causes, and conflating them is the defect
// being fixed.
//
// ⚠ IT IS A UNIT TEST OF THE COPY, NOT AN END-TO-END RUN, AND SAYS SO RATHER
// THAN IMPLYING OTHERWISE. Reaching the unreachable arm through the real command
// needs the ROLLOUT route dead while `listMine` still answers, and both are
// served by one base URL — so the honest options were a second listener or this.
// The appapi half IS measured against a closed listener
// (TestRolloutTransportFailureIsUnreachableNotNotEnrolled); what is left for
// this level is the sentence the developer reads, which is what is asserted.
func TestDoctorRolloutUnreachableIsNotNotEnrolled(t *testing.T) {
	payload := doctorRolloutPayload(appapi.Rollout{
		State:           appapi.RolloutUnreachable,
		TransportDetail: "dial tcp 127.0.0.1:1: connect: connection refused",
	})
	if payload.Enrolled != nil {
		t.Errorf("an unreachable host published enrolled=%v, want null", *payload.Enrolled)
	}
	low := strings.ToLower(payload.Detail + " " + payload.Fix)
	if strings.Contains(low, "not in the app blocks rollout") {
		t.Errorf("an unreachable host is described as being outside the rollout:\n%s", payload.Detail)
	}
	if !strings.Contains(payload.Detail, "NOT a report that you are outside the rollout") {
		t.Errorf("the unreachable copy must say explicitly that it is NOT a negative verdict, because that "+
			"is the conflation this check exists to prevent:\n%s", payload.Detail)
	}

	// 🔴 POSITIVE CONTROL ON THE SAME ASSERTION. The banned phrase above would
	// also be absent from copy that said nothing at all, so the arm that SHOULD
	// contain it must be shown to.
	outside := doctorRolloutPayload(appapi.Rollout{State: appapi.RolloutNotEnrolled})
	if !strings.Contains(strings.ToLower(outside.Detail), "not in the app blocks rollout") {
		t.Fatalf("control: the NOT-ENROLLED copy does not contain the phrase this test bans elsewhere, so "+
			"the ban is vacuous:\n%s", outside.Detail)
	}
}

// TestDoctorRolloutHumanOutputSeparatesTheThreeAnswers pins the three headlines
// a reader scans for, and that no two outcomes share one.
func TestDoctorRolloutHumanOutputSeparatesTheThreeAnswers(t *testing.T) {
	cases := []struct {
		resp     rolloutProbeResponse
		want     string
		mustNot  []string
		whyNotIt string
	}{
		{
			resp:     rolloutProbeResponse{http.StatusOK, `{"submissions":[]}`},
			want:     rolloutHeadlineEnrolled,
			mustNot:  []string{rolloutHeadlineOutside, rolloutHeadlineUnknown},
			whyNotIt: "an enrolled account must not be told anything is wrong",
		},
		{
			resp:     rolloutProbeResponse{http.StatusServiceUnavailable, `{"message":"refused"}`},
			want:     rolloutHeadlineOutside,
			mustNot:  []string{rolloutHeadlineUnknown},
			whyNotIt: "a definite negative answer must not be softened into `could not check`",
		},
		{
			resp:     rolloutProbeResponse{http.StatusUnauthorized, `{"message":"refused"}`},
			want:     rolloutHeadlineUnknown,
			mustNot:  []string{rolloutHeadlineOutside},
			whyNotIt: "a credential the host would not accept is not a rollout refusal",
		},
		{
			resp:     rolloutProbeResponse{http.StatusForbidden, `{"message":"refused"}`},
			want:     rolloutHeadlineUnknown,
			mustNot:  []string{rolloutHeadlineOutside},
			whyNotIt: "the Apps-AUTHOR gate fires first, so a 403 never reached the rollout question",
		},
		{
			resp:     rolloutProbeResponse{http.StatusServiceUnavailable, `<html>edge</html>`},
			want:     rolloutHeadlineUnknown,
			mustNot:  []string{rolloutHeadlineOutside},
			whyNotIt: "a 503 that did not come from the route is a host incident, not an account state",
		},
	}
	for _, tc := range cases {
		t.Run(tc.want+"/"+tc.resp.body, func(t *testing.T) {
			newDoctorServerRollout(t, tc.resp.status, tc.resp.body,
				doctorRow(docSlugA, docListingA, "draft", "owner", nil))
			stdout, _, err := run(t, "app", "doctor")
			if err != nil {
				t.Fatalf("app doctor: %v", err)
			}
			if !strings.Contains(stdout, tc.want) {
				t.Errorf("stdout does not carry %q:\n%s", tc.want, stdout)
			}
			for _, bad := range tc.mustNot {
				if strings.Contains(stdout, bad) {
					t.Errorf("stdout ALSO carries %q — %s:\n%s", bad, tc.whyNotIt, stdout)
				}
			}
		})
	}
	// 🔴 THE THREE HEADLINES MUST NOT BE SUBSTRINGS OF ONE ANOTHER, or every
	// mustNot above is vacuous.
	for _, a := range []string{rolloutHeadlineEnrolled, rolloutHeadlineOutside, rolloutHeadlineUnknown} {
		for _, b := range []string{rolloutHeadlineEnrolled, rolloutHeadlineOutside, rolloutHeadlineUnknown} {
			if a != b && strings.Contains(a, b) {
				t.Fatalf("headline %q contains %q — the Contains assertions above cannot distinguish them", a, b)
			}
		}
	}
}

// TestDoctorRolloutSaysItDoesNotGate: a reader who sees a shouting NOT ENROLLED
// beside a process that exited 0 has no way to tell that from a broken gate —
// the same reason the delisted section announces itself.
func TestDoctorRolloutSaysItDoesNotGate(t *testing.T) {
	for _, f := range rolloutOutcomeFixtures {
		t.Run(f.state, func(t *testing.T) {
			newDoctorServerRollout(t, f.resp.status, f.resp.body,
				doctorRow(docSlugA, docListingA, "draft", "owner", nil))
			stdout, _, err := run(t, "app", "doctor")
			if err != nil {
				t.Fatalf("app doctor: %v", err)
			}
			has := strings.Contains(stdout, rolloutNoGateNote)
			wantNote := f.state != string(appapi.RolloutEnrolled)
			if has != wantNote {
				t.Errorf("state %q: exit-code note present = %v, want %v. Every non-enrolled outcome must "+
					"say it does not gate; the enrolled one must not clutter a clean run with it.\n%s",
					f.state, has, wantNote, stdout)
			}
		})
	}
}

// TestDoctorRolloutIsReportedWithNoListingsAtAll.
//
// 🔴 THE no-listings PATH RETURNED BEFORE PRINTING ANYTHING ELSE, which is why
// the section is emitted ABOVE it. A developer who has submitted nothing yet is
// precisely the one most likely to be outside the rollout and least likely to
// know the flag exists.
func TestDoctorRolloutIsReportedWithNoListingsAtAll(t *testing.T) {
	newDoctorServerRollout(t, http.StatusServiceUnavailable, rolloutNotEnrolledBody)
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("no listings must still exit 0: %v", err)
	}
	if !strings.Contains(stdout, rolloutHeadlineOutside) {
		t.Errorf("the rollout section is missing from a run with no listings:\n%s", stdout)
	}
	// The pre-existing sentence must still be there — the section is added
	// above it, not instead of it.
	if !strings.Contains(stdout, "No App listings to check") {
		t.Errorf("the no-listings sentence was lost:\n%s", stdout)
	}
}

// TestDoctorRolloutVerdictComesFromTheStatusNotTheRows is the behavioural pin
// named by `cmd/app_doctor.go`'s entry in submissionsListReaders — the one entry
// in that ledger whose file consumes NO row.
//
// 🔴 BOTH DIRECTIONS, BECAUSE EITHER ALONE IS SATISFIED BY A BUG. A 200 carrying
// a full, realistic submissions list must read ENROLLED (so the verdict is not
// "did the body have rows"), and a 503 carrying that SAME list must read NOT
// ENROLLED (so a row reader could not have produced it).
func TestDoctorRolloutVerdictComesFromTheStatusNotTheRows(t *testing.T) {
	// A realistic list whose every field disagrees with the enrolled fixture, so
	// no assertion can pass on a value copied from elsewhere.
	rows := `{"submissions":[` +
		`{"id":"pr_ROLLOUT_A","blockId":"block_ROLLOUT_A","version":"9.9.9","status":"approved"},` +
		`{"id":"pr_ROLLOUT_B","blockId":"block_ROLLOUT_B","version":"8.8.8","status":"rejected"}]}`

	newDoctorServerRollout(t, http.StatusOK, rows, doctorRow(docSlugA, docListingA, "draft", "owner", nil))
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("app doctor: %v", err)
	}
	if !strings.Contains(stdout, rolloutHeadlineEnrolled) {
		t.Errorf("a 200 carrying two submission rows did not read as enrolled:\n%s", stdout)
	}
	// 🔴 AND NO ROW REACHED THE REPORT. The probe decodes nothing but the
	// route's `message`, so a submission id appearing in `doctor`'s output would
	// mean the body is being consumed after all.
	for _, leaked := range []string{"pr_ROLLOUT_A", "block_ROLLOUT_B", "9.9.9"} {
		if strings.Contains(stdout, leaked) {
			t.Errorf("the probe's response body leaked %q into the report — it must read only the STATUS "+
				"(and the route's own `message`):\n%s", leaked, stdout)
		}
	}

	newDoctorServerRollout(t, http.StatusServiceUnavailable, rows,
		doctorRow(docSlugA, docListingA, "draft", "owner", nil))
	stdout2, _, err2 := run(t, "app", "doctor")
	if err2 != nil {
		t.Fatalf("app doctor: %v", err2)
	}
	// 🔴 THAT BODY HAS NO `message`, SO IT IS NOT THE ROUTE'S ENVELOPE — the
	// honest verdict is COULD NOT CHECK, and asserting NOT ENROLLED here would
	// have been asserting the conflation this check forbids.
	if !strings.Contains(stdout2, rolloutHeadlineUnknown) {
		t.Errorf("a 503 whose body is NOT the route's refusal envelope must read as COULD NOT CHECK, "+
			"whatever rows it happens to carry:\n%s", stdout2)
	}
}

// TestDoctorRolloutProbeSendsNoQueryFromTheCommand: the wire shape, measured
// through the real command rather than only at the appapi seam.
func TestDoctorRolloutProbeSendsNoQueryFromTheCommand(t *testing.T) {
	srv := newDoctorServerRollout(t, http.StatusOK, rolloutEnrolledBody,
		doctorRow(docSlugA, docListingA, "draft", "owner", nil))
	if _, _, err := run(t, "app", "doctor"); err != nil {
		t.Fatalf("app doctor: %v", err)
	}
	seen := srv.rolloutQueriesSeen()
	if len(seen) == 0 {
		t.Fatal("no query strings were recorded — the probe was never issued, so this measures nothing")
	}
	for _, q := range seen {
		if q != "" {
			t.Errorf("the rollout probe was sent with a query string %q — it must narrow by nothing, so "+
				"its verdict cannot depend on a selector the caller never supplied", q)
		}
	}
}

// ---------------------------------------------------------------------------
// Server text on a human surface.
// ---------------------------------------------------------------------------

// TestDoctorRolloutServerMessageCannotForgeTheVerdict is the test named by
// printDoctorRollout's row in safeTermCoveredBy.
//
// 🔴 IT DRIVES BOTH HALVES OF safeTermBounded, because each alone is satisfied
// by the wrong input: the length cap alone passes a short string full of ESC,
// and the rune strip alone passes a long clean one. The hostile fixture is both
// — a cursor-move forgery of this very section's own headline, repeated past the
// cap.
//
// 🔴 THE TWO HALVES WERE MEASURED SEPARATELY, AND THE ISOLATION MATTERED. Two
// mutants, not one, because the obvious single mutant does NOT exercise both:
//
//   - Deleting the sanitiser entirely (`safeTermBounded(x)` -> `x`) fails the
//     ESC and invisible-rune assertions — and NOT the length one, because the
//     raw payload still carries its `\n`, so the line holding `The host said:`
//     ends before the 4,000 padding runes and the rune count never sees them.
//   - Weakening it to `safeTermSingle` — the strip WITHOUT the cap — is what
//     fails the length assertion, at 4054 runes, because collapsing that `\n`
//     to a space is precisely what puts the whole payload on one line.
//
// So each half is killed by its own mutant and neither is vacuous — but a single
// "delete the call" sweep would have scored the length assertion as untested
// while reporting the function as covered.
func TestDoctorRolloutServerMessageCannotForgeTheVerdict(t *testing.T) {
	// A counterfeit of the section's OWN enrolled headline, with a line-clear in
	// front of it, an invisible rune, a tab, a newline, and enough length to
	// soft-wrap several display rows at any terminal width.
	forgery := "\x1b[1A\x1b[2K" + rolloutHeadlineEnrolled + "\u200b\tx\n" + strings.Repeat("A", 4000)
	body, err := json.Marshal(map[string]string{"message": forgery})
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}
	newDoctorServerRollout(t, http.StatusServiceUnavailable, string(body),
		doctorRow(docSlugA, docListingA, "draft", "owner", nil))
	stdout, _, runErr := run(t, "app", "doctor")
	if runErr != nil {
		t.Fatalf("app doctor: %v", runErr)
	}

	// Positive control FIRST: the message has to have reached the surface at
	// all, or every assertion below passes over output that never carried it.
	if !strings.Contains(stdout, "The host said:") {
		t.Fatalf("the host's message was not rendered at all, so the gate assertions measure nothing:\n%s", stdout)
	}
	if !strings.Contains(stdout, "AAAA") {
		t.Fatalf("none of the hostile payload reached the surface — the assertions below are vacuous:\n%s", stdout)
	}

	// Half one: the rune class is gone.
	if strings.Contains(stdout, "\x1b") {
		t.Error("an ESC byte from the host's message reached the terminal — it could move the cursor over " +
			"this section's own verdict lines")
	}
	if strings.Contains(stdout, "\u200b") {
		t.Error("an invisible rune from the host's message reached the terminal")
	}

	// Half two: the length is bounded. Measured on the rendered LINE, which is
	// the unit a soft wrap turns into display rows.
	var quoted string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "The host said:") {
			quoted = line
			break
		}
	}
	if quoted == "" {
		t.Fatal("could not isolate the quoted line")
	}
	if n := utf8.RuneCountInString(quoted); n > 200 {
		t.Errorf("the line quoting the host's message is %d runes — unbounded server text can author an "+
			"arbitrary number of display rows by SOFT WRAP, which is how a counterfeit headline gets above "+
			"the real one with no control byte in it at all", n)
	}
	// 🔴 AND THE FORGED HEADLINE MUST NOT APPEAR AS ITS OWN LINE. The quoted
	// line legitimately contains the words (it is quoting them, prefixed), so
	// the assertion is about a line that IS the headline.
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == rolloutHeadlineEnrolled {
			t.Errorf("the host's message produced a bare %q line — a reader cannot tell it from the "+
				"CLI's own verdict:\n%s", rolloutHeadlineEnrolled, stdout)
		}
	}
}

// ---------------------------------------------------------------------------
// The advice.
// ---------------------------------------------------------------------------

// TestDoctorRolloutAdviceNamesOnlyCommandsThatExist resolves every `civitai …`
// reference in every rollout sentence against the REAL cobra tree — the same
// rule, and the same reason, as TestDoctorFixAdviceNamesOnlyCommandsThatExist:
// advice naming a command that does not exist is worse than no advice, because
// the author follows it, is refused, and concludes the tool is broken.
func TestDoctorRolloutAdviceNamesOnlyCommandsThatExist(t *testing.T) {
	root := NewRootCmd()

	// 🔴 THE NEGATIVE CONTROL COMES FIRST. "Every reference resolved" and "the
	// resolver says yes to everything" are the same output.
	if err := resolveCommandRef(root, "civitai app rollout-join"); err == nil {
		t.Fatal("negative control: the resolver accepted a command that does not exist — every positive " +
			"below would be a fact about the resolver, not about the advice")
	}

	seen := 0
	for _, st := range []appapi.RolloutState{
		appapi.RolloutEnrolled, appapi.RolloutNotEnrolled, appapi.RolloutAuthorRefused,
		appapi.RolloutNoCredential, appapi.RolloutRateLimited, appapi.RolloutUnreachable,
		appapi.RolloutUnknown,
	} {
		p := doctorRolloutPayload(appapi.Rollout{State: st})
		for _, prose := range []string{p.Detail, p.Fix, rolloutNoGateNote} {
			for _, ref := range civitaiCommandRef.FindAllString(prose, -1) {
				seen++
				if err := resolveCommandRef(root, ref); err != nil {
					t.Errorf("state %q advises %q, which does not resolve in the command tree: %v", st, ref, err)
				}
			}
		}
	}
	// 🔴 THE FLOOR IS THE MEASURED COUNT, NOT A SAFE-LOOKING SMALLER ONE.
	// Measured by arming it at 999 and reading this test's own message: 4
	// (`civitai whoami` on the not-enrolled arm, `civitai login` on the
	// author-refused and no-credential arms, `civitai app doctor` on the
	// rate-limited arm). A floor with slack is a control for a scan that has
	// half stopped working.
	if seen < 4 {
		t.Fatalf("the rollout prose scan found only %d command reference(s) — the regex is not reading "+
			"what it thinks it is, so the resolutions above prove nothing", seen)
	}
	t.Logf("resolved %d command references across 7 rollout states", seen)
}

// TestDoctorRolloutCopyNamesWhatIsNotTheCause.
//
// 🔴 THIS IS THE DEFECT, NOT A NICETY. The report that produced this check is of
// a developer who checked the app's approval and the token's scopes and was
// still refused. A finding that said only "you are not enrolled" would leave
// them checking those two things again, which is the cost the whole arc was
// about.
func TestDoctorRolloutCopyNamesWhatIsNotTheCause(t *testing.T) {
	p := doctorRolloutPayload(appapi.Rollout{State: appapi.RolloutNotEnrolled})
	for _, want := range []string{
		"approval",     // not your app's approval
		"scopes",       // not your token's scopes
		"your ACCOUNT", // what it IS
	} {
		if !strings.Contains(p.Detail, want) {
			t.Errorf("the NOT-ENROLLED copy does not mention %q. The developer this check exists for had "+
				"already verified the app's approval and the token's scopes; copy that does not rule those "+
				"out sends them back round the same loop.\n%s", want, p.Detail)
		}
	}
	if p.Fix == "" {
		t.Error("the NOT-ENROLLED arm has no fix line — the remedy is a cohort invitation the developer " +
			"cannot issue themselves, so it has to be named")
	}
}

// TestDoctorRolloutHelpDocumentsTheNoGateRule: `--help` is the contract a reader
// has offline, and the surprising half of this check is that a shouting NOT
// ENROLLED exits 0.
func TestDoctorRolloutHelpDocumentsTheNoGateRule(t *testing.T) {
	out, _, err := run(t, "app", "doctor", "--help")
	if err != nil {
		t.Fatalf("app doctor --help: %v", err)
	}
	for _, want := range []string{
		"APP BLOCKS ROLLOUT",
		"NEVER sets the exit code",
		"COULD NOT CHECK",
		"null, not false",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("`app doctor --help` must document the rollout rule (missing %q):\n%s", want, out)
		}
	}
}
