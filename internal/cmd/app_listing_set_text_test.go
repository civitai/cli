package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
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
	// shadow is the OPEN (unsubmitted) revision draft's id, or "" for none.
	// The distinction from `pending` is the whole point: a shadow can exist with
	// no publish request, which is the state the warning must catch.
	shadow string
	// kind is what listMine reports for this slug. Defaults to "offsite" — the
	// kind whose text the author really owns — so the pre-existing cases keep
	// testing the arm they were written for.
	kind string
	// omitFromListMine makes listMine NOT list this slug, exercising the
	// fail-closed arm.
	omitFromListMine bool
	// bulkRows, when set, REPLACES the listMine page entirely.
	bulkRows []map[string]any
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
	s := &setTextServer{status: "draft", kind: "offsite"}
	for _, o := range opts {
		o(s)
	}
	return newSetTextServerWith(t, s)
}

func newSetTextServerWith(t *testing.T, s *setTextServer) *setTextServer {
	t.Helper()
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, stSlug, stBlockID)
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			// 🔴 THE FAKE MIRRORS THE REAL PAYLOAD, INCLUDING THE KEYS THE CLI
			// DOES NOT READ. This reply used to omit `shadowId`/`editTargetId`,
			// which the real server ALWAYS sends — so the fake encoded the same
			// wrong assumption as the code (that no side-effect-free read
			// exposes an open shadow) and no test could ever have caught it. A
			// fake that agrees with the bug is worse than no fake.
			// Shape: offsite-listing.service.ts:2004-2015.
			shadow := any(nil)
			editTarget := stListingID
			if s.shadow != "" {
				shadow = s.shadow
				editTarget = s.shadow
			}
			trpcData(w, map[string]any{
				"appListingId":       stListingID,
				"status":             s.status,
				"contentRating":      "g",
				"hasPendingRevision": s.pending,
				"shadowId":           shadow,
				"editTargetId":       editTarget,
				"editBlockedReason":  nil,
			})
		case r.URL.Path == "/api/trpc/appListings.listMine":
			if s.bulkRows != nil {
				trpcData(w, s.bulkRows)
				return
			}
			rows := []map[string]any{}
			if !s.omitFromListMine {
				rows = append(rows, map[string]any{
					"appListingId": stListingID, "slug": stSlug, "kind": s.kind,
				})
			}
			// A second, unrelated row so a match that ignored the slug cannot pass.
			rows = append(rows, map[string]any{
				"appListingId": "apl_OTHER11", "slug": "some-other-app", "kind": "onsite",
			})
			trpcData(w, rows)
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
			t.Errorf("unexpected request to %s — neither set-text nor set-source-repo (which shares this fake) may call it", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(s.Close)
	listingEnv(t, s.URL)
	return s
}

func withStatus(st string) func(*setTextServer) { return func(s *setTextServer) { s.status = st } }
func withPending(p bool) func(*setTextServer)   { return func(s *setTextServer) { s.pending = p } }

// withOpenShadow gives the listing an OPEN, UNSUBMITTED revision draft —
// `hasPendingRevision` stays false, exactly as the server reports it. That
// combination is the state the overwrite warning exists for and could not see.
func withOpenShadow(id string) func(*setTextServer) {
	return func(s *setTextServer) { s.shadow = id }
}

// newSetTextServerRows serves an ARBITRARY listMine page, so a test can build a
// full-length (capped) one. The slug under test is deliberately absent from it.
func newSetTextServerRows(t *testing.T, rows []map[string]any) *setTextServer {
	t.Helper()
	s := &setTextServer{status: "draft", kind: "offsite", bulkRows: rows}
	return newSetTextServerWith(t, s)
}

func withKind(k string) func(*setTextServer) { return func(s *setTextServer) { s.kind = k } }
func withNotListed() func(*setTextServer) {
	return func(s *setTextServer) { s.omitFromListMine = true }
}
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
	// --yes because blanking a public field is now a guarded act; the WIRE
	// contract under test is unchanged by that gate.
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", "", "--yes"); err != nil {
		t.Fatalf("set-text --tagline \"\" --yes: %v", err)
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
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--description", "", "--yes"); err != nil {
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
// shaped like a safety mechanism.
//
// 🔴 THIS IS A BLOCKLIST, NOT A SET LEDGER, AND THE COMMENT USED TO CLAIM
// OTHERWISE. It asserts that specific procs were NOT called; it does not pin the
// request set, so it does not fail when the set GROWS. Proof, measured: adding
// `listMine` to this command's requests needed no edit here and stayed green.
// The growth property is real but comes from elsewhere — `newSetTextServer`'s
// `default:` arm t.Errorf's and 500s on any unexpected path, so a NEW proc fails
// every test using the fake. Naming the wrong mechanism is not harmless: it tells
// the next reader this assertion covers growth, so they will not add the case
// that actually would.
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
	// 🔴 THE KIND CAVEAT IS GONE FROM THIS MESSAGE ON PURPOSE. It used to say
	// "if this app is OFF-SITE …; an ON-SITE app is unaffected", which was a
	// hedge the command now makes unnecessary: `refuseOnsiteEdit` means an
	// onsite listing never reaches this line at all, so the warning states the
	// risk plainly instead of asking the reader to work out which case they are.
	for _, want := range []string{"already under moderator review", "undo this edit", "civitai app doctor"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the warning must state the overwrite risk (missing %q):\n%s", want, stderr)
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
	emptyOut, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", "", "--yes")
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

// TestSetTextWarnsOnAnOpenUnsubmittedShadow is the regression for the defect the
// audit found: the warning could not fire in the state it exists for.
//
// 🔴 THE REPRODUCTION IS THE NORMAL PATH, NOT AN EDGE CASE. `rm-screenshot`,
// `set-icon`, `set-cover` and `add-screenshot` all mint the shadow LAZILY and —
// for `rm-screenshot`, by documented design — leave it UNSUBMITTED. So:
//
//  1. stage a media change on an approved off-site listing -> a shadow exists,
//     its `tagline` copied from the parent at open time, and NO publish request;
//  2. `set-text --tagline "..."` writes the LIVE parent;
//  3. `submit-revision`, then approval -> `applyApprovedRevision`'s offsite
//     branch copies the shadow's OLD tagline back over the parent, silently
//     reverting step 2.
//
// The old gate was `hasPendingRevision`, which is a
// `appListingPublishRequest.findFirst({status:'pending'})` — SUBMITTED, not
// "a shadow exists" — so in step 2 it was false and nothing was said.
func TestSetTextWarnsOnAnOpenUnsubmittedShadow(t *testing.T) {
	const openShadow = "apl_OPENSHADOW9"
	newSetTextServer(t, withStatus("approved"), withPending(false), withOpenShadow(openShadow))
	stdout, stderr, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err != nil {
		t.Fatalf("the warning is advisory — the command must still succeed: %v", err)
	}
	if !strings.Contains(stderr, "revision") {
		t.Fatalf("an OPEN but UNSUBMITTED shadow must still warn — this is the exact window the warning "+
			"exists for, and `hasPendingRevision` is false here because nothing has been submitted.\n"+
			"stderr was:\n%s\nstdout was:\n%s", stderr, stdout)
	}
	// It must name the revision it is talking about, or the reader cannot act.
	if !strings.Contains(stderr, openShadow) {
		t.Errorf("the warning should name the open revision %q so it can be inspected:\n%s", openShadow, stderr)
	}
	if !strings.Contains(stderr, "undo this edit") {
		t.Errorf("the warning must state the overwrite risk:\n%s", stderr)
	}
}

// TestSetTextShadowDetectionCostsNoExtraRequest: the field comes off a read the
// command ALREADY makes. If detecting the shadow needed another call, the
// obvious candidate would be `getMyListingForEdit` — which OPENS a shadow as a
// side effect and would therefore CREATE the hazard on every run.
func TestSetTextShadowDetectionCostsNoExtraRequest(t *testing.T) {
	srv := newSetTextServer(t, withStatus("approved"), withOpenShadow("apl_OPEN2"))
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline); err != nil {
		t.Fatalf("set-text: %v", err)
	}
	got := srv.seen()
	if len(got) == 0 {
		t.Fatal("empty request ledger — this test measures nothing")
	}
	for _, p := range got {
		if strings.Contains(p, "getMyListingForEdit") || strings.Contains(p, "beginListingRevision") {
			t.Errorf("shadow detection reached %s, which MINTS a shadow — that creates the very "+
				"state the warning is about. It must come from getMyListingForApp, a pure read. Ledger: %v", p, got)
		}
	}
}

// TestSetTextIsSilentWithNoShadowAndNoPendingRevision is the negative control
// for BOTH gates: without it, the two tests above are equally true of a build
// that warns unconditionally.
func TestSetTextIsSilentWithNoShadowAndNoPendingRevision(t *testing.T) {
	newSetTextServer(t, withStatus("approved"), withPending(false)) // no shadow
	_, stderr, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err != nil {
		t.Fatalf("set-text: %v", err)
	}
	if strings.Contains(stderr, "revision") {
		t.Errorf("no shadow and no pending revision — the warning must NOT fire; a warning that always "+
			"fires is one nobody reads:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// The KIND gate. An onsite listing's copy is manifest-governed.
// ---------------------------------------------------------------------------

// want0Text is the ON-SITE text remedy, written out here rather than read from
// the constant the command uses. See the assertion below.
const want0Text = "tagline, description and category come from block.manifest.json — editing them here would be overwritten by the manifest at your next approved version. Edit `name` / `tagline` / `description` in block.manifest.json and run `civitai app submit`. (Category is set by a moderator on an on-site app.)"

// TestSetTextRefusesAnOnsiteListing is the correctness gate.
//
// 🔴 THE WRITE WOULD APPEAR TO SUCCEED AND BE REVERTED LATER. `(3b-sync)` in
// `<civitai>/src/server/services/blocks/publish-request.service.ts:2742-2800`
// overwrites name/tagline/description/category from the manifest on EVERY
// subsequent-version moderator approve, scoped `kind: 'onsite'`. Nothing
// server-side refuses the write — `updateListing` selects `kind` and never
// branches on it — so the gate has to be here, and it must refuse BEFORE the
// write rather than warn after it.
func TestSetTextRefusesAnOnsiteListing(t *testing.T) {
	srv := newSetTextServer(t, withKind("onsite"))
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err == nil {
		t.Fatal("an ON-SITE listing's text is manifest-governed — the write must be refused, " +
			"not silently reverted at the next approve")
	}
	// 🔴 THE WHOLE NORMALISED CLAUSE, NOT KEYWORDS. This asserted
	// {"ON-SITE", "block.manifest.json", "civitai app submit"} — every one of
	// which also appears in `onsiteSubjectSourceRepo.clause`, because the two
	// remedies are the same SHAPE. So once the gate's prose became a parameter,
	// passing the source-repo subject here printed a completely wrong remedy for
	// a tagline edit and the whole suite stayed green (measured). A keyword pin
	// on text a sibling can spell is not a guard.
	// 🔴 A LITERAL, NOT `onsiteSubjectText.clause`. Asserting against the constant
	// under test only pins WHICH subject the gate was handed — an audit reworded
	// the clause to "…come from the moon…" and the suite stayed green. Deriving an
	// expectation from the implementation it tests is how a guard ends up agreeing
	// with any change to it.
	//
	// The price is that a deliberate reword fails here once. Pay it: this string is
	// shipped user-facing advice about a public listing, and a machine-checkable
	// claim is worth one test edit.
	if want := want0Text; !strings.Contains(err.Error(), want) {
		t.Errorf("the refusal must carry the TEXT remedy verbatim.\n got: %v\nwant it to contain: %s", err, want)
	}
	// The constant must still BE that literal — otherwise the two could drift apart
	// with only this test's copy staying right.
	if onsiteSubjectText.clause != want0Text {
		t.Errorf("onsiteSubjectText.clause no longer matches the pinned text.\n got: %s\nwant: %s",
			onsiteSubjectText.clause, want0Text)
	}
	if onsiteSubjectText.clause == onsiteSubjectSourceRepo.clause {
		t.Fatal("the two onsite remedies are identical — the assertion above cannot tell them apart")
	}
	// 🔴 REFUSED BEFORE THE WRITE. A refusal that still wrote would be no fix.
	for _, p := range srv.seen() {
		if p == updateListingPath {
			t.Errorf("the onsite refusal still issued the write: %v", srv.seen())
		}
	}
}

// TestSetTextAllowsAnOffsiteListing is the gate's other arm — without it, the
// test above is equally true of a command that refuses everything.
func TestSetTextAllowsAnOffsiteListing(t *testing.T) {
	srv := newSetTextServer(t, withKind("offsite"))
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline); err != nil {
		t.Fatalf("an OFF-SITE listing's copy is author-supplied and must be writable: %v", err)
	}
	if got := string(srv.patchSent(t)["tagline"]); got != `"`+stTagline+`"` {
		t.Errorf("tagline = %s, want %q", got, stTagline)
	}
}

// TestSetTextKindGateFailsClosed: an unknown kind, or a slug listMine does not
// list, REFUSES. The directions are not symmetric — a wrongly-refused offsite
// edit is recoverable in the browser and says so; a wrongly-permitted onsite
// edit corrupts a public listing in a way nobody observes being reverted.
func TestSetTextKindGateFailsClosed(t *testing.T) {
	t.Run("unknown kind", func(t *testing.T) {
		srv := newSetTextServer(t, withKind(""))
		_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
		if err == nil {
			t.Fatal("an unestablished kind must refuse, not proceed")
		}
		if !strings.Contains(err.Error(), "could not establish") {
			t.Errorf("the refusal should say the kind could not be established: %v", err)
		}
		for _, p := range srv.seen() {
			if p == updateListingPath {
				t.Error("fail-closed must mean no write happened")
			}
		}
	})
	t.Run("slug not listed", func(t *testing.T) {
		srv := newSetTextServer(t, withNotListed())
		_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
		if err == nil {
			t.Fatal("a slug listMine does not list must refuse")
		}
		if !errors.Is(err, civitai.ErrNotFound) {
			t.Errorf("want ErrNotFound (exit 4), got %T: %v", err, err)
		}
		for _, p := range srv.seen() {
			if p == updateListingPath {
				t.Error("fail-closed must mean no write happened")
			}
		}
	})
}

// TestSetTextKindMatchIsNormalised: `kind` is a server string the CLI compares.
func TestSetTextKindMatchIsNormalised(t *testing.T) {
	for _, spelling := range []string{"onsite", "ONSITE", "Onsite", " onsite "} {
		t.Run(strings.TrimSpace(spelling), func(t *testing.T) {
			newSetTextServer(t, withKind(spelling))
			if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline); err == nil {
				t.Errorf("kind %q must read as onsite and refuse", spelling)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The category mirror's CONTENTS, not just its cardinality.
// ---------------------------------------------------------------------------

// TestCategoryMirrorMatchesTheServerVocabularyExactly pins the WHOLE normalised
// string.
//
// 🔴 A CARDINALITY FLOOR IS NOT A CONTENT GUARD, and the gap was demonstrated
// rather than theorised: misspelling `"discovery"` as `"discovry"` leaves a
// count of 7 and the entire suite green — while
// `TestSetTextCategoryAcceptsEveryServerValue` drives the typo through a FAKE
// that accepts any string, so it confirms the typo instead of catching it.
//
// The failure inverts: `--category discovery` is then refused LOCALLY, and the
// refusal NAMES THE TYPO as an allowed value, so the user copies it and gets a
// server 400 plus a burnt rate-limit slot. Misspelling is the likelier drift for
// a hand-copied list than deletion, which is all the S8 mutant covered.
func TestCategoryMirrorMatchesTheServerVocabularyExactly(t *testing.T) {
	const want = "generation,games,utility,discovery,moderation,analytics,other"
	got := strings.Join(appapi.MarketplaceCategories, ",")
	if got != want {
		t.Errorf("the category mirror has drifted from the server vocabulary.\n got: %s\nwant: %s\n"+
			"Authority: `MARKETPLACE_CATEGORIES` in "+
			"<civitai>/src/server/services/blocks/marketplace-categories.constants.ts (read at origin/main, "+
			"2026-08-24). Order is part of the contract — it is what every refusal prints.", got, want)
	}
}

// ---------------------------------------------------------------------------
// The blank-set guard, --json, and whitespace.
// ---------------------------------------------------------------------------

// TestSetTextRefusesABlankSetWithoutYes is the safety gate on a PUBLIC write.
//
// 🔴 THE SHELL MAKES THIS AN ACCIDENT, NOT A CHOICE. `--tagline "$T"` with `T`
// unset expands to `--tagline ""`, and this command's own documented example
// `--description "$(cat DESCRIPTION.md)"` does the same whenever `cat` fails —
// silently blanking a public field at exit 0. Every destructive sibling gates
// (`app submit` needs `--yes`, `app withdraw` confirms); this had nothing.
func TestSetTextRefusesABlankSetWithoutYes(t *testing.T) {
	for _, tc := range []struct{ flag, val string }{
		{"tagline", ""},
		{"description", ""},
		// 🔴 Whitespace-only is blank too: the server's `isEmpty` TRIMS, so
		// " " leaves `civitai app doctor` still reporting empty-tagline. A
		// command that reported success for that would be lying about the fix.
		{"tagline", "   "},
		{"description", "\t\n "},
	} {
		t.Run(tc.flag+"="+strings.TrimSpace(tc.val)+"/blank", func(t *testing.T) {
			srv := newSetTextServer(t)
			_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--"+tc.flag, tc.val)
			if err == nil {
				t.Fatalf("--%s %q must be refused without --yes — it would empty a public field", tc.flag, tc.val)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("want ErrUsage (exit 2), got %T: %v", err, err)
			}
			for _, want := range []string{"--yes", "--clear " + tc.flag} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal must name %q so both intents are reachable: %v", want, err)
				}
			}
			// Refused BEFORE the wire — a guard that still wrote would be none.
			if len(srv.seen()) != 0 {
				t.Errorf("the blank-set refusal still made requests: %v", srv.seen())
			}
		})
	}
}

// TestSetTextAllowsABlankSetWithYes is the guard's other arm. Without it, the
// test above is equally true of a build that refuses every blank forever, which
// would make "set to an empty string" unreachable — the exact state the tri-state
// exists to carry.
func TestSetTextAllowsABlankSetWithYes(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", "", "--yes"); err != nil {
		t.Fatalf("--yes must allow a deliberate blank: %v", err)
	}
	if got := string(srv.patchSent(t)["tagline"]); got != `""` {
		t.Errorf("tagline = %s, want an empty JSON string", got)
	}
}

// TestSetTextClearNeedsNoYes: `--clear` is already an explicit request to empty
// the field, so a second confirmation would be noise.
func TestSetTextClearNeedsNoYes(t *testing.T) {
	srv := newSetTextServer(t)
	if _, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--clear", "tagline"); err != nil {
		t.Fatalf("--clear is already explicit and must not need --yes: %v", err)
	}
	if got := string(srv.patchSent(t)["tagline"]); got != "null" {
		t.Errorf("tagline = %s, want null", got)
	}
}

// TestSetTextJSONShapeIsPinnedWhole: `--json` is a published contract, pinned
// WHOLE so a renamed or dropped field is visible.
//
// 🔴 IT EXISTS BECAUSE THE LOOP WAS MACHINE-READABLE AT BOTH OTHER ENDS.
// `civitai app doctor --json` gives a machine-readable diagnosis; without this a
// script could read the problem and apply the fix but had no machine-readable
// confirmation of what changed — and `app listing status --json`, the obvious
// substitute, cannot serve because it OPENS a shadow revision as a side effect.
func TestSetTextJSONShapeIsPinnedWhole(t *testing.T) {
	const openShadow = "apl_JSONSHADOW5"
	newSetTextServer(t, withStatus("approved"), withOpenShadow(openShadow))
	stdout, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug,
		"--tagline", stTagline, "--clear", "description", "--json")
	if err != nil {
		t.Fatalf("set-text --json: %v", err)
	}
	want := `{
  "slug": "` + stSlug + `",
  "appListingId": "` + stListingID + `",
  "fields": {
    "description": "cleared",
    "tagline": "set"
  },
  "requiresReview": false,
  "shadowId": "` + openShadow + `",
  "openRevision": true
}
`
	if stdout != want {
		t.Errorf("--json payload changed.\n--- got ---\n%s\n--- want ---\n%s", stdout, want)
	}
}

// TestSetTextJSONIsTheWholeOfStdout: `… --json | jq -e .` must always parse, and
// the advisory must not contaminate it.
func TestSetTextJSONIsTheWholeOfStdout(t *testing.T) {
	newSetTextServer(t, withStatus("approved"), withOpenShadow("apl_X9"))
	stdout, stderr, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline, "--json")
	if err != nil {
		t.Fatalf("set-text --json: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("stdout is not valid JSON (%v):\n%s", err, stdout)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("stdout carries more than the payload:\n%s", stdout)
	}
	for _, glyph := range []string{"✓", "⚠", "\x1b["} {
		if strings.Contains(stdout, glyph) {
			t.Errorf("--json stdout carries the styling marker %q:\n%s", glyph, stdout)
		}
	}
	// The advisory still has to reach the operator — on stderr.
	if !strings.Contains(stderr, "revision") {
		t.Errorf("the overwrite advisory must still print on stderr under --json:\n%s", stderr)
	}
}

// TestSetTextJSONReportsBlankAsEmptyNotSet: a script must be able to tell an
// empty write from a real one, because the server's `isEmpty` trims and `doctor`
// will still report the field as empty afterwards.
func TestSetTextJSONReportsBlankAsEmptyNotSet(t *testing.T) {
	newSetTextServer(t)
	stdout, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", "  ", "--yes", "--json")
	if err != nil {
		t.Fatalf("set-text: %v", err)
	}
	var got struct {
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("bad payload (%v): %s", err, stdout)
	}
	if got.Fields["tagline"] != "empty" {
		t.Errorf(`fields.tagline = %q, want "empty" — a whitespace-only value leaves `+
			"`doctor` still reporting empty-tagline, so reporting it as \"set\" would be false", got.Fields["tagline"])
	}
}

// TestSetTextOnsiteRefusalIsAVerdictNotAUsageError pins the exit CLASSIFICATION
// of the onsite refusal, which the message assertions above do not touch.
//
// 🔴 IT IS 1, NOT 2, AND NOTHING ELSE IN THE SUITE WOULD NOTICE THE MOVE.
// Stripping the sentinel, or tagging it civitai.ErrBadRequest, leaves every
// character of the refusal intact and silently reclassifies it — the exact gap
// AGENTS item 7 exists for. `2` means the INVOCATION was wrong and would send a
// script's owner re-reading their command line; every flag and the slug are
// well-formed here. What is wrong is the SUBJECT, which the contract publishes
// under `1` alongside an invalid manifest and a version regression.
//
// Measured live against production 2026-08-25: `set-text --slug
// model-benchmarking --tagline "…"` (a real ON-SITE approved listing) exited 1
// with this message, and the listing's tagline md5 was byte-identical before and
// after — the refusal wrote nothing.
func TestSetTextOnsiteRefusalIsAVerdictNotAUsageError(t *testing.T) {
	newSetTextServer(t, withKind("onsite"))
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err == nil {
		t.Fatal("expected the onsite refusal")
	}
	if !errors.Is(err, ErrOnsiteNotEditable) {
		t.Errorf("the refusal must carry ErrOnsiteNotEditable so its exit code is assertable, got %T: %v", err, err)
	}
	// 🔴 NOT a usage error, and not any API kind — either would move the code.
	if errors.Is(err, ErrUsage) {
		t.Error("the onsite refusal must NOT be a usage error: the invocation is well-formed, the SUBJECT is not editable")
	}
	for name, kind := range map[string]error{
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	} {
		if errors.Is(err, kind) {
			t.Errorf("the onsite refusal must not match civitai.%s — that would move its exit code", name)
		}
	}
	// POSITIVE CONTROL on the walk: it must be able to SEE a kind, or the five
	// negatives above are a fact about a comparison that matches nothing.
	if !errors.Is(civitai.Tag(civitai.ErrNotFound, errors.New("x")), civitai.ErrNotFound) {
		t.Fatal("the errors.Is walk cannot see a kind it should — the negatives prove nothing")
	}
}

// TestSetTextUnknownKindRefusalIsClassified pins the OTHER fail-closed arm's
// exit code. The ONSITE arm got a sentinel and a classification test; this one
// was left as a bare error pinned only by prose, so stripping the tag — or
// swapping it for an API kind — would move the exit code with every character of
// the message intact.
func TestSetTextUnknownKindRefusalIsClassified(t *testing.T) {
	newSetTextServer(t, withKind(""))
	_, _, err := run(t, "app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline)
	if err == nil {
		t.Fatal("an unestablished kind must refuse")
	}
	if !errors.Is(err, ErrOnsiteNotEditable) {
		t.Errorf("the unknown-kind refusal must carry the sentinel so its exit code is assertable, got %T: %v", err, err)
	}
	if errors.Is(err, ErrUsage) {
		t.Error("it is a verdict about the SUBJECT, not a malformed invocation")
	}
	for name, kind := range map[string]error{
		"ErrBadRequest": civitai.ErrBadRequest, "ErrNotFound": civitai.ErrNotFound,
		"ErrUnauthorized": civitai.ErrUnauthorized, "ErrNetwork": civitai.ErrNetwork,
	} {
		if errors.Is(err, kind) {
			t.Errorf("must not match civitai.%s — that would move its exit code", name)
		}
	}
}

// TestSetTextNotFoundOnACappedPageIsNotConclusive.
//
// 🔴 THE KIND GATE INHERITS `listMine`'s 200-ROW CAP AND USED TO STATE THE
// CONSEQUENCE AS FACT. It treats "absent from listMine" as proof the listing
// does not exist. For a caller whose accessible set exceeds appapi.ListMineCap, a
// target OUTSIDE the newest page is indistinguishable from a nonexistent one —
// so the command hard-blocked a write `updateListing` would have accepted while
// telling the author their own app does not exist. Refusing stays right (the
// kind is what makes the write safe); the CLAIM had to stop being certain.
func TestSetTextNotFoundOnACappedPageIsNotConclusive(t *testing.T) {
	rows := make([]map[string]any, 0, appapi.ListMineCap)
	for i := 0; i < appapi.ListMineCap; i++ {
		rows = append(rows, map[string]any{
			"appListingId": fmt.Sprintf("apl_BULK%03d", i),
			"slug":         fmt.Sprintf("bulk-%03d", i),
			"kind":         "offsite",
		})
	}
	srv := newSetTextServerRows(t, rows)
	_, _, err := run(t, "app", "listing", "set-text", "--slug", "an-app-past-the-cap", "--tagline", stTagline)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("still exit 4, got %T: %v", err, err)
	}
	for _, want := range []string{"CAPS this read", "not conclusive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("a miss off a capped page must say it is uncertain (missing %q): %v", want, err)
		}
	}
	// Fail-closed still means no write.
	for _, p := range srv.seen() {
		if p == updateListingPath {
			t.Errorf("the refusal still issued the write: %v", srv.seen())
		}
	}
	// NEGATIVE CONTROL: below the cap the answer IS conclusive and must not be
	// hedged, or the hedge becomes noise on every ordinary typo.
	newSetTextServer(t)
	_, _, err2 := run(t, "app", "listing", "set-text", "--slug", "definitely-not-mine", "--tagline", stTagline)
	if err2 == nil {
		t.Fatal("control: expected a refusal")
	}
	if strings.Contains(err2.Error(), "not conclusive") {
		t.Errorf("below the cap the answer is conclusive and must not be hedged: %v", err2)
	}
}

// The `ListMineCap` drift guard lives in app_doctor_test.go
// (TestListMineCapMatchesTheServer). It pins ONE constant, so there is ONE
// guard: `app doctor` and `app listing set-text` are both consumers of the same
// cap, and each branch added its own copy independently. Two copies of one pin
// is the duplicated-predicate shape this repo keeps finding wrong at N-1 sites —
// here the compiler caught it, because they landed in the same package.

// TestSetTextAdvisoryReachesBothRenderings is finding 5: the human path RETURNED
// before the shared advisory while `--json` called it unconditionally, so with
// `requiresReview:true` AND an open shadow the two renderings disagreed about
// whether the overwrite hazard applied — the exact property warnOpenRevision's
// doc comment claimed they could not.
func TestSetTextAdvisoryReachesBothRenderings(t *testing.T) {
	const shadow = "apl_BOTHPATHS7"
	for _, jsonOut := range []bool{false, true} {
		name := "human"
		args := []string{"app", "listing", "set-text", "--slug", stSlug, "--tagline", stTagline}
		if jsonOut {
			name, args = "json", append(args, "--json")
		}
		t.Run(name, func(t *testing.T) {
			newSetTextServer(t, withStatus("approved"), withOpenShadow(shadow),
				withReply(map[string]any{"requiresReview": true, "shadowId": shadow}))
			_, stderr, err := run(t, args...)
			if err != nil {
				t.Fatalf("set-text: %v", err)
			}
			if !strings.Contains(stderr, "revision") {
				t.Errorf("the overwrite advisory must reach the %s rendering too — a branch that returns "+
					"early makes the two disagree about whether the hazard applies:\n%s", name, stderr)
			}
		})
	}
}
