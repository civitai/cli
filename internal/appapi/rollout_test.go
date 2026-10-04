package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// The App-Blocks ROLLOUT probe (internal/appapi/rollout.go).
//
// Everything here runs against an httptest fake. 🔴 NOTHING IN THIS FILE HAS
// BEEN RUN AGAINST A REAL ACCOUNT ON EITHER SIDE OF THE ROLLOUT. Every expected
// status is read off the host's own source at `civitai/civitai@origin/main` —
// the ladder quoted with line numbers in rollout.go — so these are tests of the
// CLI's reading of that ladder, not evidence that the ladder is what production
// serves.
//
// 🔴 EVERY FIXTURE STRING BELOW IS DISTINCT FROM EVERY CONSTANT AN ASSERTION
// NAMES. The probe's whole thesis is that it keys on STATUS and not on text, and
// a fixture spelling the real server message at every arm could not tell a
// status-keyed implementation from a text-matching one. So the message bodies
// here are deliberately WRONG for their status wherever that is possible.

// rolloutProbeFake answers the submissions route with one status and body, and
// records the method and the raw query it was asked with.
type rolloutProbeFake struct {
	*httptest.Server
	method string
	query  string
	path   string
	hits   int
}

func newRolloutProbeFake(t *testing.T, status int, body string) *rolloutProbeFake {
	t.Helper()
	f := &rolloutProbeFake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits++
		f.method = r.Method
		f.query = r.URL.RawQuery
		f.path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func probeRollout(t *testing.T, status int, body string) (Rollout, *rolloutProbeFake) {
	t.Helper()
	f := newRolloutProbeFake(t, status, body)
	got := New(f.URL, "tok-rollout", "").CheckAppBlocksRollout(context.Background())
	// Positive control on the fake: a probe that never issued a request would
	// make every assertion below a fact about the zero value.
	if f.hits != 1 {
		t.Fatalf("the fake was hit %d time(s), want exactly 1 — the assertions would measure nothing", f.hits)
	}
	return got, f
}

// TestCheckAppBlocksRolloutMapsEveryStatus is the whole status→state table, with
// LITERAL expectations: every `want` here is written from the host's gate ladder,
// never read back out of the implementation.
func TestCheckAppBlocksRolloutMapsEveryStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   RolloutState
		why    string
	}{
		{
			name: "200 is enrolled", status: http.StatusOK,
			// 🔴 THE BODY SAYS THE OPPOSITE OF THE VERDICT, ON PURPOSE. A
			// text-matching implementation reports NOT enrolled here; a
			// status-keyed one reports enrolled. This single row is what
			// separates them.
			body: `{"submissions":[],"note":"Apps are not enabled"}`,
			want: RolloutEnrolled,
			why:  "200 is the route's success path, which sits past the app-blocks-enabled gate",
		},
		{
			name: "503 from the route is outside the rollout", status: http.StatusServiceUnavailable,
			// Also deliberately NOT the server's real wording.
			body: `{"message":"gate closed for this subject"}`,
			want: RolloutNotEnrolled,
			why:  "the only refusal on this route that answers 503 and carries the route's own envelope",
		},
		{
			name: "401 is a credential the host did not accept", status: http.StatusUnauthorized,
			body: `{"message":"nope"}`,
			want: RolloutNoCredential,
			why:  "the host never got as far as asking about the account — and a BANNED account looks identical",
		},
		{
			name: "403 refused before the rollout question", status: http.StatusForbidden,
			body: `{"message":"nope"}`,
			want: RolloutAuthorRefused,
			why:  "the Apps-AUTHOR gate runs first, so a 403 says nothing about app-blocks-enabled",
		},
		{
			name: "429 is an answer about the request", status: http.StatusTooManyRequests,
			body: `{"message":"slow down","retryAfterSeconds":30}`,
			want: RolloutRateLimited,
			why:  "no answer at all about the account, and the advice has to be `retry` rather than `ask for access`",
		},
		{
			name: "500 is unreadable, not a verdict", status: http.StatusInternalServerError,
			body: `{"message":"boom"}`,
			want: RolloutUnknown,
			why:  "an unexpected status must claim nothing in either direction",
		},
		{
			name: "422 is unreadable too", status: http.StatusUnprocessableEntity,
			body: `{"message":"Invalid query"}`,
			want: RolloutUnknown,
			why:  "unreachable while the probe sends no selector, and still not a rollout verdict if it ever is reached",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := probeRollout(t, tc.status, tc.body)
			if got.State != tc.want {
				t.Errorf("status %d gave state %q, want %q — %s", tc.status, got.State, tc.want, tc.why)
			}
			if got.HTTPStatus != tc.status {
				t.Errorf("HTTPStatus = %d, want %d — the published status must be the one the host sent, "+
					"so a reader can re-derive the branch", got.HTTPStatus, tc.status)
			}
		})
	}

	// 🔴 NEGATIVE CONTROL ON THE TABLE ITSELF. Seven rows read through one
	// helper; an implementation that always returned the same state would make
	// several of them agree with it. The table above already contains four
	// distinct expected states, so this control asserts the weaker property the
	// table cannot: that no two of its rows collapsed onto one answer by
	// accident.
	t.Run("control: the table distinguishes at least five states", func(t *testing.T) {
		seen := map[RolloutState]bool{}
		for _, tc := range cases {
			got, _ := probeRollout(t, tc.status, tc.body)
			seen[got.State] = true
		}
		if len(seen) < 5 {
			t.Fatalf("the whole table produced only %d distinct states (%v) — it cannot tell a status-keyed "+
				"implementation from one that answers the same thing every time", len(seen), seen)
		}
	})
}

// TestRolloutVerdictIsStatusKeyedNotTextKeyed is the anti-string-match guard,
// stated as its own test because it is the design decision most likely to be
// "simplified" away.
//
// 🔴 THE HOST IS BEING REWORDED, WHICH IS WHY THIS IS A TEST AND NOT A COMMENT.
// Both arms carry the literal production message `Apps are not enabled` on the
// WRONG side of the verdict: a 200 that quotes it must still read ENROLLED, and
// a 503 that does NOT quote it must still read NOT ENROLLED. An implementation
// matching the string fails both.
func TestRolloutVerdictIsStatusKeyedNotTextKeyed(t *testing.T) {
	enrolled, _ := probeRollout(t, http.StatusOK, `{"message":"Apps are not enabled"}`)
	if enrolled.State != RolloutEnrolled {
		t.Errorf("a 200 quoting %q read as %q — the verdict must come from the STATUS, not the text",
			"Apps are not enabled", enrolled.State)
	}
	outside, _ := probeRollout(t, http.StatusServiceUnavailable,
		`{"message":"this account is outside the current audience"}`)
	if outside.State != RolloutNotEnrolled {
		t.Errorf("a 503 NOT quoting the production wording read as %q — a reworded refusal must not "+
			"change the verdict", outside.State)
	}
}

// TestRollout503FromAnEdgeIsNotAVerdict: a 503 that did not come from the route
// must claim nothing.
//
// 🔴 THIS IS THE DEFECT THE WHOLE CHECK EXISTS TO AVOID, ONE LEVEL OUT. An
// edge/CDN outage page is the likeliest way "the host is having an incident"
// would be reported to a developer as "you are not in the rollout" — and a
// status code alone cannot tell them apart, because any intermediary can emit a
// 503.
func TestRollout503FromAnEdgeIsNotAVerdict(t *testing.T) {
	for _, body := range []string{
		`<html><head><title>503 Service Temporarily Unavailable</title></head></html>`,
		``,
		`{}`,
		`{"message":""}`,
		`{"error":"Apps are not enabled"}`, // the route uses `message`, not `error`
	} {
		got, _ := probeRollout(t, http.StatusServiceUnavailable, body)
		if got.State != RolloutUnknown {
			t.Errorf("a 503 with body %q read as %q, want %q — only a body carrying the ROUTE's own "+
				"non-empty `message` may be read as a rollout refusal", body, got.State, RolloutUnknown)
		}
		if got.Enrolled() != nil {
			t.Errorf("a 503 with body %q reported an established verdict", body)
		}
	}
	// Positive control: the same status WITH the route's envelope is a verdict,
	// so the arm above is distinguishing the body and not refusing every 503.
	got, _ := probeRollout(t, http.StatusServiceUnavailable, `{"message":"anything at all"}`)
	if got.State != RolloutNotEnrolled {
		t.Fatalf("control: a 503 carrying the route's envelope read as %q — this test is refusing every "+
			"503 rather than reading the body", got.State)
	}
}

// TestRolloutServerMessageIsNeverTheRawBody pins the split between
// rolloutEnvelope and serverMessage.
//
// 🔴 MEASURED AS A REAL DEFECT, NOT ANTICIPATED. The first version used
// serverMessage, whose documented fallback is the trimmed RAW BODY, and the 200
// arm reported `The host said: {"submissions":[]}` — the route's own payload
// rendered as if it were a sentence from the host. The same mechanism on an edge
// 503 would have put a whole HTML outage page on the user's terminal.
func TestRolloutServerMessageIsNeverTheRawBody(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{http.StatusOK, `{"submissions":[{"id":"pr_1"}]}`},
		{http.StatusServiceUnavailable, `<html>503 from the edge</html>`},
		{http.StatusInternalServerError, `not json at all`},
	} {
		got, _ := probeRollout(t, tc.status, tc.body)
		if got.ServerMessage != "" {
			t.Errorf("status %d with body %q published ServerMessage %q — a body with no `message` field "+
				"must publish NOTHING, or the report quotes a payload as if the host had said it",
				tc.status, tc.body, got.ServerMessage)
		}
	}
	// Positive control: a real `message` IS carried, so the assertion above is
	// not simply reporting that the field is never populated.
	got, _ := probeRollout(t, http.StatusServiceUnavailable, `{"message":"a sentence from the host"}`)
	if got.ServerMessage != "a sentence from the host" {
		t.Fatalf("control: the route's own message was not carried (got %q) — every assertion above would "+
			"pass against a field that is always empty", got.ServerMessage)
	}
}

// TestRolloutProbeSendsAnUnnarrowedGET pins the wire shape.
//
// 🔴 THE EMPTY QUERY IS LOAD-BEARING. A selector would add a 404/422 arm that
// answers the same question less clearly, and `?blockId=` specifically answers
// 200-with-an-empty-list for a miss — which would still be read as enrolled, but
// through a second mechanism nobody asked for. The METHOD matters too: the route
// 405s anything but GET, and a 405 is in the unknown arm.
func TestRolloutProbeSendsAnUnnarrowedGET(t *testing.T) {
	_, f := probeRollout(t, http.StatusOK, `{"submissions":[]}`)
	if f.method != http.MethodGet {
		t.Errorf("the probe used %s, want GET — the route 405s every other method", f.method)
	}
	if f.query != "" {
		t.Errorf("the probe sent a query string %q — it must send no selector at all", f.query)
	}
	if f.path != SubmissionsPath {
		t.Errorf("the probe asked for %q, want %q", f.path, SubmissionsPath)
	}
}

// TestRolloutTransportFailureIsUnreachableNotNotEnrolled: no response is not a
// negative answer.
func TestRolloutTransportFailureIsUnreachableNotNotEnrolled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now
	got := New(url, "tok-rollout", "").CheckAppBlocksRollout(context.Background())
	if got.State != RolloutUnreachable {
		t.Errorf("a dial failure read as %q, want %q — an unreachable host must never be reported as a "+
			"rollout refusal", got.State, RolloutUnreachable)
	}
	if got.Enrolled() != nil {
		t.Error("a dial failure reported an established verdict")
	}
	if got.HTTPStatus != 0 {
		t.Errorf("HTTPStatus = %d, want 0 — no response arrived", got.HTTPStatus)
	}
	if got.TransportDetail == "" {
		t.Error("an unreachable result must carry the transport detail, or the report cannot say WHY")
	}
}

// failingTokenSource is a TokenSource that cannot produce a credential — the
// shape internal/auth has when no refresh token is stored. It tags
// ErrUnauthorized exactly as internal/auth does.
type failingTokenSource struct{}

func (failingTokenSource) Token(context.Context) (string, error) {
	return "", civitai.Tag(civitai.ErrUnauthorized, errorString("no refresh token stored"))
}
func (failingTokenSource) Refresh(context.Context) (string, error) {
	return "", civitai.ErrNoRefresh
}

type errorString string

func (e errorString) Error() string { return string(e) }

// TestRolloutLocalCredentialFailureIsNotUnreachable: the two halves of
// authedDo's error are different answers.
//
// 🔴 THIS BRANCH WAS UNREACHED BY EVERY OTHER TEST IN THIS FILE, AND THE ONLY
// WAY TO REACH IT IS A TOKEN SOURCE THAT FAILS. `New(url, "tok", "")` builds a
// StaticToken, which never errors, so the arm distinguishing a local credential
// failure from a dead host was a guard no case executed — a guard proven
// breakable is not a guard proven reachable. The fake here is the real shape:
// internal/auth returns an ErrUnauthorized-tagged error when no refresh token is
// stored, and nothing left the machine.
//
// 🔴 THE SERVER IS LIVE, WHICH IS THE POINT. If the listener were closed, both
// branches would produce `unreachable` and the test would pass against the
// mutant that deletes the split.
func TestRolloutLocalCredentialFailureIsNotUnreachable(t *testing.T) {
	f := newRolloutProbeFake(t, http.StatusOK, `{"submissions":[]}`)
	c := NewWithSource(f.URL, failingTokenSource{}, "")
	got := c.CheckAppBlocksRollout(context.Background())
	if got.State != RolloutNoCredential {
		t.Errorf("a local token-source failure read as %q, want %q — the host is UP here, so reporting it "+
			"as unreachable is both wrong and unactionable", got.State, RolloutNoCredential)
	}
	if f.hits != 0 {
		t.Errorf("the probe reached the server %d time(s) — nothing should have left the machine", f.hits)
	}
	if got.TransportDetail == "" {
		t.Error("the local failure must be carried, or the report cannot say why no request was made")
	}

	// 🔴 POSITIVE CONTROL ON THE SPLIT. The same live server with a WORKING
	// token source must produce a different state, or this test is passing
	// because the probe always answers no-credential.
	ok := New(f.URL, "tok-rollout", "").CheckAppBlocksRollout(context.Background())
	if ok.State != RolloutEnrolled {
		t.Fatalf("control: the same server with a working token source read as %q — the assertion above "+
			"cannot distinguish the branches", ok.State)
	}
}

// TestRolloutEnrolledIsATristate: `nil` is not `false`.
//
// 🔴 THE FIVE nil STATES ARE ASSERTED INDIVIDUALLY, NOT AS A GROUP. A `default:
// return nil` is correct today and would silently start answering nil for a
// future state that genuinely means `false`; naming each one is what makes a
// later addition a decision rather than an inheritance.
func TestRolloutEnrolledIsATristate(t *testing.T) {
	if got := (Rollout{State: RolloutEnrolled}).Enrolled(); got == nil || !*got {
		t.Errorf("enrolled -> %v, want a pointer to true", got)
	}
	if got := (Rollout{State: RolloutNotEnrolled}).Enrolled(); got == nil || *got {
		t.Errorf("not-enrolled -> %v, want a pointer to false", got)
	}
	for _, st := range []RolloutState{
		RolloutAuthorRefused, RolloutNoCredential, RolloutRateLimited,
		RolloutUnreachable, RolloutUnknown, RolloutState("a-state-from-the-future"),
	} {
		r := Rollout{State: st}
		if got := r.Enrolled(); got != nil {
			t.Errorf("state %q -> %v, want nil — a script asking `is this developer outside the rollout` "+
				"must not be handed false for a question that was never answered", st, *got)
		}
		if r.Enrolled() != nil {
			t.Errorf("state %q reported an established verdict", st)
		}
	}
}

// TestRolloutStatesAreDistinctStrings: the states are published in `--json`, so
// two of them collapsing onto one spelling would make the payload lie about
// which cause occurred.
func TestRolloutStatesAreDistinctStrings(t *testing.T) {
	all := []RolloutState{
		RolloutEnrolled, RolloutNotEnrolled, RolloutAuthorRefused,
		RolloutNoCredential, RolloutRateLimited, RolloutUnreachable, RolloutUnknown,
	}
	seen := map[RolloutState]bool{}
	for _, s := range all {
		if strings.TrimSpace(string(s)) == "" {
			t.Errorf("a rollout state has an empty spelling — `--json` would publish it as \"\"")
		}
		if seen[s] {
			t.Errorf("two rollout states share the spelling %q", s)
		}
		seen[s] = true
	}
	if len(seen) != 7 {
		t.Fatalf("expected 7 distinct states, got %d — if a state was added or removed, the cmd-layer "+
			"copy table (doctorRolloutPayload) and its ledger test must move with it", len(seen))
	}
}
