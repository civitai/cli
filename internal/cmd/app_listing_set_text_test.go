package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// `civitai app listing set-text` — the WRITE half of the three text problems
// `civitai app doctor` reports.
//
// Everything here runs against an httptest fake. Nothing in this file has been
// run against a real listing.
//
// 🔴 THE FIXTURE VALUES ARE PAIRWISE DISTINCT AND DISTINCT FROM EVERY CONSTANT
// AN ASSERTION NAMES — including from the marketplace category vocabulary, so a
// mutant that hardcoded a category literal cannot be masked by a fixture that
// only ever produces that literal.
const (
	stSlug      = "set-text-app"
	stListingID = "apl_SETTEXT77"
	stBlockID   = "block_SETTEXT88"
	stTagline   = "Zephyr batch pipelines"
	stDesc      = "A much longer body of prose about the app."
	stCategory  = "utility"
)

// updateListingPath is spelled here, in the test, rather than read from the
// production route var: a test that reads the value it checks agrees with any
// change to it.
const updateListingPath = "/api/trpc/appListings.updateListing"

// setTextServer answers the two reads `set-text` makes plus the write, and
// RECORDS every request path and the update's raw body — the body is the whole
// point, because the tri-state this command exists to carry is only visible on
// the wire.
type setTextServer struct {
	*httptest.Server
	mu         sync.Mutex
	paths      []string
	updateBody []byte
	status     string
	pending    bool
	// reply overrides the update's result payload when non-nil.
	reply map[string]any
	// updateStatus, when non-zero, makes the update answer that HTTP status
	// with a tRPC error envelope instead of succeeding.
	updateStatus int
	updateMsg    string
}

func (s *setTextServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.paths))
	copy(out, s.paths)
	return out
}

// patchSent decodes the `patch` object the CLI actually put on the wire.
// 🔴 json.RawMessage per key, so an explicit `null` survives — unmarshalling
// into a typed struct would erase the very distinction under test.
func (s *setTextServer) patchSent(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.updateBody) == 0 {
		t.Fatal("no update request was ever sent — this assertion would be vacuous")
	}
	var env struct {
		JSON struct {
			ListingID string                     `json:"listingId"`
			Patch     map[string]json.RawMessage `json:"patch"`
		} `json:"json"`
	}
	if err := json.Unmarshal(s.updateBody, &env); err != nil {
		t.Fatalf("update body is not the expected envelope (%v): %s", err, s.updateBody)
	}
	if env.JSON.ListingID != stListingID {
		t.Errorf("update addressed listingId %q, want %q", env.JSON.ListingID, stListingID)
	}
	return env.JSON.Patch
}

func newSetTextServer(t *testing.T, opts ...func(*setTextServer)) *setTextServer {
	t.Helper()
	s := &setTextServer{status: "draft"}
	for _, o := range opts {
		o(s)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, stSlug, stBlockID)
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{
				"appListingId":       stListingID,
				"status":             s.status,
				"contentRating":      "g",
				"hasPendingRevision": s.pending,
			})
		case r.URL.Path == updateListingPath:
			body, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.updateBody = body
			s.mu.Unlock()
			if s.updateStatus != 0 {
				w.WriteHeader(s.updateStatus)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"` + s.updateMsg + `"}}}`))
				return
			}
			if s.reply != nil {
				trpcData(w, s.reply)
				return
			}
			trpcData(w, map[string]any{"requiresReview": false, "shadowId": nil})
		default:
			// 🔴 A 500 as well as a t.Errorf: a command that reached an
			// unexpected proc must FAIL, not merely be noted — `getAssets` and
			// `updateRevisionDraft` both 403 a CLI token in production, and a
			// test that only logged the call would stay green.
			t.Errorf("unexpected request to %s — set-text must not call it", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(s.Close)
	listingEnv(t, s.URL)
	return s
}

func withStatus(st string) func(*setTextServer) { return func(s *setTextServer) { s.status = st } }
func withPending(p bool) func(*setTextServer)   { return func(s *setTextServer) { s.pending = p } }
func withReply(m map[string]any) func(*setTextServer) {
	return func(s *setTextServer) { s.reply = m }
}
func withUpdateError(code int, msg string) func(*setTextServer) {
	return func(s *setTextServer) { s.updateStatus, s.updateMsg = code, msg }
}

// ---------------------------------------------------------------------------
// The tri-state on the wire. This is the contract the command exists for.
// ---------------------------------------------------------------------------

// TestSetTextSendsOnlyTheFieldsGiven: an omitted flag must send NO key, or the
// server would overwrite a column the user never mentioned.
func TestSetTextSendsOnlyTheFieldsGiven(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline); err != nil {
		t.Fatalf("set-text: %v", err)
	}
	patch := srv.patchSent(t)
	if got := len(patch); got != 1 {
		t.Fatalf("patch carries %d keys, want exactly 1 — an omitted flag must send NO key: %v", got, patch)
	}
	if string(patch["tagline"]) != `"`+stTagline+`"` {
		t.Errorf("tagline = %s, want %q", patch["tagline"], stTagline)
	}
	for _, absent := range []string{"description", "category"} {
		if _, ok := patch[absent]; ok {
			t.Errorf("patch carries %q, which was never passed — that would overwrite a column the user did not name", absent)
		}
	}
}

// TestSetTextEmptyStringIsSentAsAnEmptyString is the distinction an `omitempty`
// struct would silently destroy: `--tagline ""` is a real edit.
func TestSetTextEmptyStringIsSentAsAnEmptyString(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", ""); err != nil {
		t.Fatalf("set-text --tagline \"\": %v", err)
	}
	patch := srv.patchSent(t)
	raw, ok := patch["tagline"]
	if !ok {
		t.Fatalf("`--tagline \"\"` sent NO tagline key — an empty string is a legal value and must reach the server: %v", patch)
	}
	if string(raw) != `""` {
		t.Errorf("tagline = %s, want an empty JSON string (NOT null)", raw)
	}
}

// TestSetTextClearSendsExplicitNull: the OTHER half of the same distinction.
func TestSetTextClearSendsExplicitNull(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--clear", "tagline"); err != nil {
		t.Fatalf("set-text --clear tagline: %v", err)
	}
	patch := srv.patchSent(t)
	raw, ok := patch["tagline"]
	if !ok {
		t.Fatalf("`--clear tagline` sent no tagline key: %v", patch)
	}
	if string(raw) != "null" {
		t.Errorf("tagline = %s, want null — `--clear` must NULL the column, not empty it", raw)
	}
}

// TestSetTextEmptyStringAndNullAreDifferentOnTheWire states the property both
// tests above serve, in one place, so it cannot be satisfied by a build that
// collapses them to the same value.
func TestSetTextEmptyStringAndNullAreDifferentOnTheWire(t *testing.T) {
	srvEmpty := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--description", ""); err != nil {
		t.Fatalf("empty arm: %v", err)
	}
	empty := string(srvEmpty.patchSent(t)["description"])

	srvNull := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--clear", "description"); err != nil {
		t.Fatalf("null arm: %v", err)
	}
	null := string(srvNull.patchSent(t)["description"])

	if empty == null {
		t.Fatalf("`--description \"\"` and `--clear description` both sent %s — they are DIFFERENT server states "+
			"and collapsing them makes one of them unreachable through this CLI", empty)
	}
	if empty != `""` || null != "null" {
		t.Errorf("empty arm sent %s (want \"\"), null arm sent %s (want null)", empty, null)
	}
}

// TestSetTextSendsAllThreeInOnePatch: one proc, one request — not three.
func TestSetTextSendsAllThreeInOnePatch(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug,
		"--tagline", stTagline, "--description", stDesc, "--category", stCategory); err != nil {
		t.Fatalf("set-text: %v", err)
	}
	patch := srv.patchSent(t)
	if len(patch) != 3 {
		t.Fatalf("patch carries %d keys, want 3: %v", len(patch), patch)
	}
	// Count the writes: three fields must cost ONE update request, not three.
	writes := 0
	for _, p := range srv.seen() {
		if p == updateListingPath {
			writes++
		}
	}
	if writes != 1 {
		t.Errorf("three fields cost %d update requests, want 1 — the server takes one partial patch, "+
			"and N requests spend N of a 30/hour budget and can half-apply", writes)
	}
}

// ---------------------------------------------------------------------------
// It must never reach the procs that 403 a CLI token.
// ---------------------------------------------------------------------------

// TestSetTextNeverCallsTheShadowProc is the request LEDGER, and it pins the
// design decision as behaviour rather than as a comment.
//
// 🔴 `updateRevisionDraft` IS DELIBERATELY UNWIRED. For these three fields the
// server never routes to a shadow (`MATERIAL_PATCH_FIELDS` excludes all three
// and `patchHasMaterialChange` is a diff), so calling it would be dead code
// shaped like a safety mechanism. A ledger rather than a "did not call X" check:
// a SET assertion fails when the set grows as well as when it shrinks, so a
// later change that adds a proc has to come here and say so.
func TestSetTextNeverCallsTheShadowProc(t *testing.T) {
	srv := newSetTextServer(t, withStatus("approved"), withPending(true))
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--category", stCategory); err != nil {
		t.Fatalf("set-text on an approved listing: %v", err)
	}
	got := srv.seen()
	if len(got) == 0 {
		t.Fatal("the request ledger is EMPTY — the command made no request, so this test measures nothing")
	}
	for _, banned := range []string{
		"updateRevisionDraft", // would be dead code, and 403s a CLI token today
		"getAssets",           // deliberately NOT scope-annotated by #4341
		"beginListingRevision",
		"getMyListingForEdit", // 🔴 OPENS a shadow as a side effect — see below
	} {
		for _, p := range got {
			if strings.Contains(p, banned) {
				t.Errorf("set-text called %s (%s). Even on an APPROVED listing with a revision pending, "+
					"a text-only patch applies in place — and getMyListingForEdit would CREATE the shadow "+
					"whose existence the warning is about.", banned, p)
			}
		}
	}
	// Positive control: the write really did happen, so the negatives above are
	// not a fact about a command that did nothing.
	found := false
	for _, p := range got {
		if p == updateListingPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("no update request in %v — every 'did not call' assertion above is vacuous", got)
	}
}

// ---------------------------------------------------------------------------
// Category validation — refused locally, with the allowed values named.
// ---------------------------------------------------------------------------

// TestSetTextCategoryAcceptsEveryServerValue drives ALL seven, so a mirror that
// dropped or mistyped one is caught rather than only the value a single test
// happened to pick.
func TestSetTextCategoryAcceptsEveryServerValue(t *testing.T) {
	if len(appapi.MarketplaceCategories) != 7 {
		t.Fatalf("the category mirror holds %d values, want 7 — re-read "+
			"`<civitai>/src/server/services/blocks/marketplace-categories.constants.ts` and update this floor "+
			"deliberately: %v", len(appapi.MarketplaceCategories), appapi.MarketplaceCategories)
	}
	for _, cat := range appapi.MarketplaceCategories {
		t.Run(cat, func(t *testing.T) {
			srv := newSetTextServer(t)
			if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--category", cat); err != nil {
				t.Fatalf("category %q must be accepted: %v", cat, err)
			}
			if got := string(srv.patchSent(t)["category"]); got != `"`+cat+`"` {
				t.Errorf("category sent as %s, want %q", got, cat)
			}
		})
	}
}

// TestSetTextRejectsAnUnknownCategoryLocally: refused BEFORE any request, with
// the allowed values printed — which the server's own 400 does not do.
func TestSetTextRejectsAnUnknownCategoryLocally(t *testing.T) {
	srv := newSetTextServer(t)
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--category", "encabulation")
	if err == nil {
		t.Fatal("an unknown category must be refused")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("a bad flag VALUE is a usage error (exit 2), got %T: %v", err, err)
	}
	// 🔴 REFUSED BEFORE THE WIRE. The whole value of a local mirror is not
	// spending a round trip — and a rejected write must not have touched the
	// listing.
	if len(srv.seen()) != 0 {
		t.Errorf("a locally-refusable category still made requests %v — it must be refused before any network call", srv.seen())
	}
	// The message must NAME the allowed values, or it is no better than the 400.
	for _, cat := range appapi.MarketplaceCategories {
		if !strings.Contains(err.Error(), cat) {
			t.Errorf("the refusal does not name the allowed category %q: %v", cat, err)
		}
	}
}

// TestSetTextCategoryCaseMissIsNamed: the server compares exactly, so `Utility`
// is a real refusal. It gets its own sentence because it is the likeliest typo.
func TestSetTextCategoryCaseMissIsNamed(t *testing.T) {
	newSetTextServer(t)
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--category", "Utility")
	if err == nil {
		t.Fatal("`Utility` must be refused — categories are lower-case and the server compares exactly")
	}
	if !strings.Contains(err.Error(), "did you mean") || !strings.Contains(err.Error(), `"utility"`) {
		t.Errorf("a case-only miss should name the value the user meant, got: %v", err)
	}
}

// TestSetTextEmptyCategoryIsRefusedAndNamesClear: `""` is legal for the other
// two and NOT for category, and the remedy names the flag that does what the
// user was reaching for.
func TestSetTextEmptyCategoryIsRefusedAndNamesClear(t *testing.T) {
	srv := newSetTextServer(t)
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--category", "")
	if err == nil {
		t.Fatal("an empty category must be refused — the server's enum has no empty member")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("want a usage error (exit 2), got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "--clear category") {
		t.Errorf("the refusal must name `--clear category`, which is what the user probably wanted: %v", err)
	}
	if len(srv.seen()) != 0 {
		t.Errorf("refused locally, but requests were made: %v", srv.seen())
	}
}

// ---------------------------------------------------------------------------
// Local refusals: bounds, contradictions, and the empty patch.
// ---------------------------------------------------------------------------

func TestSetTextRefusalsHappenBeforeAnyRequest(t *testing.T) {
	long := strings.Repeat("x", appapi.MaxTaglineRunes+1)
	longDesc := strings.Repeat("y", appapi.MaxDescriptionRunes+1)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no field at all", []string{"--slug", stSlug}, "nothing to set"},
		{"tagline over the cap", []string{"--slug", stSlug, "--tagline", long}, "at most 140"},
		{"description over the cap", []string{"--slug", stSlug, "--description", longDesc}, "at most 2000"},
		{"set and clear contradict", []string{"--slug", stSlug, "--tagline", stTagline, "--clear", "tagline"}, "contradict"},
		{"unknown clear field", []string{"--slug", stSlug, "--clear", "taglines"}, "does not know the field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSetTextServer(t)
			_, _, err := run(t, append([]string{"app", "listing", "set-text"}, tc.args...)...)
			if err == nil {
				t.Fatalf("expected a refusal for %s", tc.name)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("want ErrUsage (exit 2), got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal should say %q, got: %v", tc.want, err)
			}
			// Every one of these is knowable without the server.
			if len(srv.seen()) != 0 {
				t.Errorf("%s reached the network before refusing: %v", tc.name, srv.seen())
			}
		})
	}
}

// TestSetTextBoundsAtTheCapAreAccepted is the boundary's other side. Without it,
// the cap tests above are equally true of a build that refuses everything.
func TestSetTextBoundsAtTheCapAreAccepted(t *testing.T) {
	srv := newSetTextServer(t)
	exact := strings.Repeat("z", appapi.MaxTaglineRunes)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", exact); err != nil {
		t.Fatalf("a tagline of exactly %d characters must be accepted: %v", appapi.MaxTaglineRunes, err)
	}
	if got := srv.patchSent(t)["tagline"]; len(got) == 0 {
		t.Error("the at-cap tagline never reached the wire")
	}
}

// TestSetTextCapsCountRunesNotBytes: the cap is a character count server-side,
// so a multi-byte tagline within the limit must not be refused.
func TestSetTextCapsCountRunesNotBytes(t *testing.T) {
	srv := newSetTextServer(t)
	// 140 runes, ~420 bytes — over the cap if counted as bytes, at it as runes.
	multibyte := strings.Repeat("é", appapi.MaxTaglineRunes)
	if len(multibyte) <= appapi.MaxTaglineRunes {
		t.Fatalf("fixture is not multi-byte enough to discriminate (%d bytes)", len(multibyte))
	}
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", multibyte); err != nil {
		t.Fatalf("a %d-rune tagline must be accepted however many BYTES it occupies: %v", appapi.MaxTaglineRunes, err)
	}
	if got := srv.patchSent(t)["tagline"]; len(got) == 0 {
		t.Error("the multi-byte tagline never reached the wire")
	}
}

// ---------------------------------------------------------------------------
// What it reports.
// ---------------------------------------------------------------------------

// TestSetTextReportsTheServersBranchNotItsOwnBelief: `requiresReview` is decoded
// and printed. It is unreachable for a text-only patch TODAY — that is a claim
// about a constant in another repo, which can move.
func TestSetTextReportsTheServersBranchNotItsOwnBelief(t *testing.T) {
	shadow := "apl_STAGED42"
	newSetTextServer(t, withStatus("approved"), withReply(map[string]any{
		"requiresReview": true, "shadowId": shadow,
	}))
	stdout, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err != nil {
		t.Fatalf("set-text: %v", err)
	}
	for _, want := range []string{"REVISION", "moderator review", shadow, "civitai app listing submit-revision"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("when the SERVER says it staged a revision the CLI must say so (missing %q):\n%s", want, stdout)
		}
	}
}

// TestSetTextWarnsWhenARevisionIsUnderReview — the real, kind-scoped hazard.
func TestSetTextWarnsWhenARevisionIsUnderReview(t *testing.T) {
	newSetTextServer(t, withStatus("approved"), withPending(true))
	stdout, stderr, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err != nil {
		t.Fatalf("the warning is advisory — the command must still succeed: %v", err)
	}
	for _, want := range []string{"already under moderator review", "OFF-SITE", "ON-SITE", "undo this edit"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the warning must state the OFFSITE-only overwrite risk (missing %q):\n%s", want, stderr)
		}
	}
	// 🔴 STDERR, so stdout stays a clean report — the same stream split every
	// other advisory in this CLI uses.
	if strings.Contains(stdout, "under moderator review") {
		t.Errorf("the advisory belongs on stderr, not stdout:\n%s", stdout)
	}
}

// TestSetTextIsSilentWhenNoRevisionIsPending is the warning's negative control:
// without it, the test above is equally true of a build that warns always.
func TestSetTextIsSilentWhenNoRevisionIsPending(t *testing.T) {
	newSetTextServer(t, withStatus("approved"), withPending(false))
	_, stderr, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err != nil {
		t.Fatalf("set-text: %v", err)
	}
	if strings.Contains(stderr, "under moderator review") {
		t.Errorf("no revision is pending, so the overwrite warning must NOT print — a warning that always "+
			"fires is one nobody reads:\n%s", stderr)
	}
}

// TestSetTextSuccessLineDistinguishesEmptyFromCleared: the two states the
// command exists to keep separate must not read the same afterwards either.
func TestSetTextSuccessLineDistinguishesEmptyFromCleared(t *testing.T) {
	newSetTextServer(t)
	emptyOut, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", "")
	if err != nil {
		t.Fatalf("empty arm: %v", err)
	}
	newSetTextServer(t)
	clearOut, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--clear", "tagline")
	if err != nil {
		t.Fatalf("clear arm: %v", err)
	}
	if emptyOut == clearOut {
		t.Fatalf("setting an empty string and clearing print the SAME line:\n%s", emptyOut)
	}
	if !strings.Contains(emptyOut, "empty string") {
		t.Errorf("the empty-string arm should say so:\n%s", emptyOut)
	}
	if !strings.Contains(clearOut, "cleared") {
		t.Errorf("the clear arm should say cleared:\n%s", clearOut)
	}
}

// ---------------------------------------------------------------------------
// Server refusals keep their own classification.
// ---------------------------------------------------------------------------

func TestSetTextServerRefusalsKeepTheirExitCode(t *testing.T) {
	cases := []struct {
		name   string
		status int
		msg    string
		kind   error
	}{
		{"403 not owned or removed", http.StatusForbidden, "you do not own this listing", civitai.ErrUnauthorized},
		{"400 invalid revision or must resubmit", http.StatusBadRequest, "this listing must be resubmitted", civitai.ErrBadRequest},
		{"404 no listing", http.StatusNotFound, "listing not found", civitai.ErrNotFound},
		{"429 rate limited", http.StatusTooManyRequests, "Too many edits — slow down.", civitai.ErrRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newSetTextServer(t, withUpdateError(tc.status, tc.msg))
			_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
			if err == nil {
				t.Fatalf("expected a %d to fail", tc.status)
			}
			if !errors.Is(err, tc.kind) {
				t.Errorf("a %d must classify as %v, got %T: %v", tc.status, tc.kind, err, err)
			}
			// The server's own reason must survive — the CLI cannot tell
			// NOT_OWNED from a removed listing (both 403), nor MUST_RESUBMIT
			// from INVALID_REVISION (both 400), so the message is the only thing
			// carrying which one it was.
			if !strings.Contains(err.Error(), tc.msg) {
				t.Errorf("the server's own message must survive; got: %v", err)
			}
		})
	}
}

// TestSetTextChangeRefusalDoesNotClaimNothingChanged: a 400 on a write may have
// PARTIALLY applied, and the shared change arm must not say otherwise.
func TestSetTextChangeRefusalDoesNotClaimNothingChanged(t *testing.T) {
	newSetTextServer(t, withUpdateError(http.StatusBadRequest, "bad patch"))
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err == nil {
		t.Fatal("expected a 400")
	}
	if strings.Contains(err.Error(), "nothing was changed") {
		t.Errorf("a WRITE that 400s may have partially applied — it must not claim nothing changed: %v", err)
	}
	if !strings.Contains(err.Error(), "may have partially applied") {
		t.Errorf("the change arm's remedy should survive: %v", err)
	}
}
