package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
)

// `app listing status --json` (civitai/cli#447).
//
// 🔴 THE POINT OF THE FLAG IS THAT THE TWO ID SPACES BECOME READABLE.
// civitai/cli#430 was a two-id-space bug — `reorder` addressed the PARENT while
// `status` printed the SHADOW revision's screenshot rows — and the human output
// shows NEITHER id, so diagnosing it took hand-written tRPC calls. So the
// assertions below are on the PARSED payload and pin `parentId` and `shadowId`
// as distinct values; a `strings.Contains` on the blob would pass on output that
// prints one id twice.
//
// Everything here runs against an httptest fake. Nothing in this file has been
// run against a real listing.

// The fixture ids. Pairwise distinct, and distinct from every other string in
// the payload, so an assertion cannot pass on a value the code copied from
// somewhere else.
const (
	jsonFixtureSlug     = "listing-json-app"
	jsonFixtureParentID = "apl_PARENT7Q1"
	jsonFixtureShadowID = "apl_SHADOW9Z2"
	jsonFixtureShotA    = "alsc_SHOT4A1"
	jsonFixtureShotB    = "alsc_SHOT4B2"
)

// listingStatusPayloadShape mirrors what `--json` emits. It is hand-written
// rather than reusing the production type on purpose: a test that unmarshals
// into the very struct it is checking agrees with any renaming of a field, which
// is the published contract this test exists to pin.
type listingStatusPayloadShape struct {
	Slug               string  `json:"slug"`
	ParentID           string  `json:"parentId"`
	ShadowID           *string `json:"shadowId"`
	Status             string  `json:"status"`
	HasPendingRevision bool    `json:"hasPendingRevision"`
	Assets             struct {
		Icon struct {
			Present bool `json:"present"`
			ImageID *int `json:"imageId"`
		} `json:"icon"`
		Cover struct {
			Present bool `json:"present"`
			ImageID *int `json:"imageId"`
		} `json:"cover"`
		Screenshots []struct {
			ID      string  `json:"id"`
			Order   int     `json:"order"`
			Caption *string `json:"caption"`
			ImageID *int    `json:"imageId"`
		} `json:"screenshots"`
	} `json:"assets"`
	Floor struct {
		Met     bool     `json:"met"`
		Missing []string `json:"missing"`
	} `json:"floor"`
}

// approvedListingServer answers the two reads `app listing status` makes for an
// APPROVED listing whose media is resolved through an open shadow revision.
//
// 🔴 Two fields are deliberately given DIFFERENT values on the two replies —
// `status` and `hasPendingRevision`. In production the parent lookup
// (getMyListingForApp) and the edit read (getMyListingForEdit) are expected to
// agree on both; the fixture disagrees only so the assertions can pin WHICH read
// each JSON field comes from. That is a claim about this CLI's rendering, not a
// claim about the server.
func approvedListingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, jsonFixtureSlug, "block_json_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{
				"appListingId":       jsonFixtureParentID,
				"status":             "approved",
				"contentRating":      "g",
				"hasPendingRevision": false,
			})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId":           jsonFixtureParentID,
				"slug":               jsonFixtureSlug,
				"status":             "draft",
				"hasPendingRevision": true,
				"shadowId":           jsonFixtureShadowID,
				"assets": map[string]any{
					"icon":  map[string]any{"imageId": 4711, "url": "http://x/icon.png"},
					"cover": map[string]any{"imageId": nil, "url": nil},
					"screenshots": []any{
						map[string]any{"id": jsonFixtureShotA, "imageId": 8123, "url": "http://x/a.png", "caption": "Side by side", "order": 0},
						map[string]any{"id": jsonFixtureShotB, "imageId": 8124, "url": "http://x/b.png", "caption": nil, "order": 3},
					},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

// draftListingServer answers for a DRAFT listing: no shadow revision exists, and
// the server omits `shadowId` from the reply entirely. Both required assets are
// attached, so the floor is met.
func draftListingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, jsonFixtureSlug, "block_json_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{
				"appListingId":       jsonFixtureParentID,
				"status":             "draft",
				"contentRating":      "g",
				"hasPendingRevision": false,
			})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId":           jsonFixtureParentID,
				"slug":               jsonFixtureSlug,
				"status":             "draft",
				"hasPendingRevision": false,
				"assets": map[string]any{
					"icon":        map[string]any{"imageId": 4711, "url": "http://x/icon.png"},
					"cover":       map[string]any{"imageId": 5322, "url": "http://x/cover.png"},
					"screenshots": []any{},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

// decodeListingStatusJSON parses stdout as ONE JSON value and fails when
// anything else shares the stream. `--json` stdout is a published contract:
// `… --json | jq -e .` must always parse (README, "Scripting with --json").
func decodeListingStatusJSON(t *testing.T, stdout string) listingStatusPayloadShape {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var got listingStatusPayloadShape
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("stdout is not valid JSON (%v); stdout was:\n%s", err, stdout)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("stdout carries more than the JSON payload (next token err = %v); stdout was:\n%s", err, stdout)
	}
	return got
}

// rawKeys re-parses stdout as a generic object so a test can assert on KEY
// PRESENCE and on an explicit `null` — neither of which survives unmarshalling
// into a typed struct.
func rawKeys(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout is not a JSON object (%v); stdout was:\n%s", err, stdout)
	}
	return raw
}

// TestAppListingStatusJSONApprovedNamesBothIdSpaces is the #447 assertion:
// an approved listing's payload names the PARENT and the SHADOW separately, so
// the id a mutation must be addressed to is readable by machine.
func TestAppListingStatusJSONApprovedNamesBothIdSpaces(t *testing.T) {
	srv := approvedListingServer(t)
	defer srv.Close()
	listingEnv(t, srv.URL)

	stdout, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	got := decodeListingStatusJSON(t, stdout)

	if got.ParentID != jsonFixtureParentID {
		t.Errorf("parentId = %q, want %q", got.ParentID, jsonFixtureParentID)
	}
	if got.ShadowID == nil {
		t.Fatalf("shadowId is null on an approved listing whose read opened a shadow — "+
			"the two id spaces are what #447 exists to expose; stdout was:\n%s", stdout)
	}
	if *got.ShadowID != jsonFixtureShadowID {
		t.Errorf("shadowId = %q, want %q", *got.ShadowID, jsonFixtureShadowID)
	}
	// 🔴 The whole bug class in one assertion: the two ids must not be the same
	// value. #430 shipped because the id the user could SEE and the id the CLI
	// ADDRESSED were from different spaces and nothing showed both.
	if *got.ShadowID == got.ParentID {
		t.Errorf("shadowId and parentId are the same value (%q) — the payload must distinguish them", got.ParentID)
	}
	if got.Slug != jsonFixtureSlug {
		t.Errorf("slug = %q, want %q", got.Slug, jsonFixtureSlug)
	}
	// `status` is the PARENT's lifecycle status (what the human line prints);
	// the edit read says "draft" in this fixture, and must not win.
	if got.Status != "approved" {
		t.Errorf("status = %q, want %q (the parent lookup's value)", got.Status, "approved")
	}
	// `hasPendingRevision` comes from the EDIT read (again, matching the human
	// line); the parent lookup says false in this fixture.
	if !got.HasPendingRevision {
		t.Errorf("hasPendingRevision = false, want true (the edit read's value)")
	}

	if !got.Assets.Icon.Present {
		t.Errorf("assets.icon.present = false, want true")
	}
	if got.Assets.Icon.ImageID == nil || *got.Assets.Icon.ImageID != 4711 {
		t.Errorf("assets.icon.imageId = %v, want 4711", got.Assets.Icon.ImageID)
	}
	if got.Assets.Cover.Present {
		t.Errorf("assets.cover.present = true, want false (this fixture has no cover)")
	}
	if got.Assets.Cover.ImageID != nil {
		t.Errorf("assets.cover.imageId = %v, want null", *got.Assets.Cover.ImageID)
	}

	if len(got.Assets.Screenshots) != 2 {
		t.Fatalf("assets.screenshots has %d rows, want 2; stdout was:\n%s", len(got.Assets.Screenshots), stdout)
	}
	first, second := got.Assets.Screenshots[0], got.Assets.Screenshots[1]
	if first.ID != jsonFixtureShotA || second.ID != jsonFixtureShotB {
		t.Errorf("screenshot ids = %q, %q; want %q, %q", first.ID, second.ID, jsonFixtureShotA, jsonFixtureShotB)
	}
	// The orders are 0 and 3 — NOT 0 and 1 — so a payload that emitted the slice
	// index instead of the server's `order` cannot pass.
	if first.Order != 0 || second.Order != 3 {
		t.Errorf("screenshot orders = %d, %d; want 0, 3 (the server's own order values)", first.Order, second.Order)
	}
	if first.Caption == nil || *first.Caption != "Side by side" {
		t.Errorf("first screenshot caption = %v, want %q", first.Caption, "Side by side")
	}
	if second.Caption != nil {
		t.Errorf("second screenshot caption = %q, want null (the fixture has none)", *second.Caption)
	}
	if first.ImageID == nil || *first.ImageID != 8123 || second.ImageID == nil || *second.ImageID != 8124 {
		t.Errorf("screenshot imageIds = %v, %v; want 8123, 8124", first.ImageID, second.ImageID)
	}

	if got.Floor.Met {
		t.Errorf("floor.met = true, want false (no cover is attached)")
	}
	if len(got.Floor.Missing) != 1 || got.Floor.Missing[0] != "cover" {
		t.Errorf("floor.missing = %v, want [cover]", got.Floor.Missing)
	}
}

// TestAppListingStatusJSONDraftReportsANullShadow pins the OTHER arm: a draft
// listing has no shadow revision, and the payload says so with an explicit
// `null` rather than by dropping the key. A consumer must be able to ask
// `.shadowId` on every listing and get an answer.
func TestAppListingStatusJSONDraftReportsANullShadow(t *testing.T) {
	srv := draftListingServer(t)
	defer srv.Close()
	listingEnv(t, srv.URL)

	stdout, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	got := decodeListingStatusJSON(t, stdout)
	if got.ShadowID != nil {
		t.Errorf("shadowId = %q on a draft listing, want null", *got.ShadowID)
	}
	raw := rawKeys(t, stdout)
	shadow, ok := raw["shadowId"]
	if !ok {
		t.Fatalf("the `shadowId` key is ABSENT on a draft listing — it must be present and null "+
			"so a script can read it unconditionally; stdout was:\n%s", stdout)
	}
	if string(shadow) != "null" {
		t.Errorf("shadowId = %s, want null", shadow)
	}
	if got.Status != "draft" {
		t.Errorf("status = %q, want draft", got.Status)
	}
	if got.HasPendingRevision {
		t.Errorf("hasPendingRevision = true, want false")
	}
	if !got.Floor.Met {
		t.Errorf("floor.met = false, want true (icon and cover are both attached)")
	}
	// Empty COLLECTIONS, never null: `jq '.floor.missing | length'` and
	// `.assets.screenshots[]` must work without a null check.
	if got.Floor.Missing == nil {
		t.Errorf("floor.missing is null, want an empty array")
	}
	if len(got.Floor.Missing) != 0 {
		t.Errorf("floor.missing = %v, want empty", got.Floor.Missing)
	}
	if got.Assets.Screenshots == nil {
		t.Errorf("assets.screenshots is null, want an empty array")
	}
	if len(got.Assets.Screenshots) != 0 {
		t.Errorf("assets.screenshots has %d rows, want 0", len(got.Assets.Screenshots))
	}
	if got.Assets.Cover.ImageID == nil || *got.Assets.Cover.ImageID != 5322 {
		t.Errorf("assets.cover.imageId = %v, want 5322", got.Assets.Cover.ImageID)
	}
}

// TestAppListingStatusJSONIsTheOnlyThingOnStdout — no human line may share the
// JSON stream. decodeListingStatusJSON already fails on trailing tokens; this
// adds the direction it cannot see, a human line PREPENDED as valid JSON is not,
// plus the specific strings the human renderer emits.
func TestAppListingStatusJSONIsTheOnlyThingOnStdout(t *testing.T) {
	srv := approvedListingServer(t)
	defer srv.Close()
	listingEnv(t, srv.URL)

	stdout, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if !strings.HasPrefix(strings.TrimLeft(stdout, " \t\r\n"), "{") {
		t.Fatalf("stdout does not start with the JSON object; stdout was:\n%s", stdout)
	}
	decodeListingStatusJSON(t, stdout)
	for _, human := range []string{
		"App:", "Listing status:", "Screenshots:", "Publish floor met", "Not publishable yet",
		"This listing is live", "under moderator review",
	} {
		if strings.Contains(stdout, human) {
			t.Errorf("human output %q leaked onto the --json stdout:\n%s", human, stdout)
		}
	}
}

// TestAppListingStatusJSONEmitsNoANSIEvenWithColorForced is the
// `internal/ui/CONVENTION.md` rule ("machine-readable output stays raw"), tested
// in the ONLY configuration that can see a violation.
//
// 🔴 A run with color OFF proves nothing here: the test harness captures stdout
// into a bytes.Buffer, which is not a TTY, so `ui.*` returns plain text and a
// JSON path routed through `ui.Success` would still be escape-free. So color is
// FORCED — once by flag, once by env — and each arm carries a POSITIVE CONTROL
// on the human path in the same configuration: if the control does not show
// escapes, the forcing did not work and the arm proves nothing.
func TestAppListingStatusJSONEmitsNoANSIEvenWithColorForced(t *testing.T) {
	// The ui package's color mode is process-global. Every run() reconfigures it
	// from that command's own flags, but restore a plain default so nothing after
	// this test inherits forced color.
	t.Cleanup(func() { ui.Configure(ui.Options{Writer: io.Discard}) })

	const esc = "\x1b["

	t.Run("--color flag", func(t *testing.T) {
		srv := approvedListingServer(t)
		defer srv.Close()
		listingEnv(t, srv.URL)

		human, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--color")
		if err != nil {
			t.Fatalf("status --color: %v", err)
		}
		if !strings.Contains(human, esc) {
			t.Fatalf("POSITIVE CONTROL FAILED: --color produced no ANSI on the HUMAN path, so this "+
				"test cannot see a violation on the JSON path either; human stdout was:\n%q", human)
		}
		stdout, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--json", "--color")
		if err != nil {
			t.Fatalf("status --json --color: %v", err)
		}
		if strings.Contains(stdout, esc) {
			t.Errorf("--json emitted an ANSI escape with --color (internal/ui/CONVENTION.md rule 1):\n%q", stdout)
		}
		decodeListingStatusJSON(t, stdout)
	})

	t.Run("CLICOLOR_FORCE", func(t *testing.T) {
		srv := approvedListingServer(t)
		defer srv.Close()
		listingEnv(t, srv.URL)
		t.Setenv("CLICOLOR_FORCE", "1")

		human, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if !strings.Contains(human, esc) {
			t.Fatalf("POSITIVE CONTROL FAILED: CLICOLOR_FORCE produced no ANSI on the HUMAN path, so "+
				"this test cannot see a violation on the JSON path either; human stdout was:\n%q", human)
		}
		stdout, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--json")
		if err != nil {
			t.Fatalf("status --json: %v", err)
		}
		if strings.Contains(stdout, esc) {
			t.Errorf("--json emitted an ANSI escape under CLICOLOR_FORCE (internal/ui/CONVENTION.md rule 1):\n%q", stdout)
		}
		decodeListingStatusJSON(t, stdout)
	})
}

// TestAppListingStatusJSONWritesNothingOnAFailedRead pins the error half of the
// published `--json` contract: the error goes to the returned error (stderr in
// the real binary) and stdout stays EMPTY, so `jq` never sees prose.
//
// The exit code is asserted with errors.Is, never on message text — AGENTS.md
// item 7 (claudedocs/decisions/07-exit-codes-pinned-by-errors-is.md): the
// classification sentinels carry no visible text, so a message assertion says
// nothing at all about the exit code.
func TestAppListingStatusJSONWritesNothingOnAFailedRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, jsonFixtureSlug, "block_json_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"No listing found","code":-32004}}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	stdout, _, err := run(t, "app", "listing", "status", "--slug", jsonFixtureSlug, "--json")
	if err == nil {
		t.Fatalf("expected an error; stdout was:\n%s", stdout)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("error is not classified ErrNotFound (exit 4): %v", err)
	}
	if stdout != "" {
		t.Errorf("--json wrote to stdout on a failed read, so a pipe sees a partial object:\n%q", stdout)
	}
}

// TestAppListingStatusJSONUsageErrorPrintsNothingAtAll pins the OTHER published
// rule (README, exit code 2): a usage error emits no JSON object in any mode.
//
// 🔴 LABELLED HONESTLY: this one is an INVARIANT GUARD, not regression coverage.
// It is the only test in this file that PASSES on the pre-flag code, because
// there `--json` is an unknown flag — itself a usage error with an empty stdout.
// It earns its keep from the mutation direction instead: writing an empty object
// before the resolve (mutant M9) kills it.
func TestAppListingStatusJSONUsageErrorPrintsNothingAtAll(t *testing.T) {
	listingEnv(t, "http://127.0.0.1:1") // never reached — the flag check fails first
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(`{"version":"0.1.0"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	stdout, _, err := run(t, "app", "listing", "status", "--dir", dir, "--json")
	if err == nil {
		t.Fatalf("expected a usage error; stdout was:\n%s", stdout)
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("error is not classified ErrUsage (exit 2): %v", err)
	}
	if stdout != "" {
		t.Errorf("--json wrote to stdout on a usage error, which the README says prints nothing at all:\n%q", stdout)
	}
}

// TestAppListingStatusJSONHelpWarnsAboutTheShadowSideEffect — the flag's own
// documentation must say that this "read" is not a pure one.
//
// 🔴 getMyListingForEdit OPENS A SHADOW REVISION on an APPROVED listing as a
// server-side side effect (idempotently). A script polling `--json` in a loop is
// therefore a WRITER, and the whole reason to give this command a machine
// interface is that somebody will poll it. Its classification as a "read" is the
// open question in civitai/cli#389; this test does not settle that, it only
// pins that the help text tells the truth about it.
func TestAppListingStatusJSONHelpWarnsAboutTheShadowSideEffect(t *testing.T) {
	cmd := newAppListingStatusCmd()
	if cmd.Flags().Lookup("json") == nil {
		t.Fatal("`app listing status` has no --json flag")
	}
	long := cmd.Long
	for _, want := range []string{"--json", "poll", "shadow revision", "389"} {
		if !strings.Contains(long, want) {
			t.Errorf("`app listing status` Long does not mention %q — a machine interface that "+
				"invites polling must say that polling it opens a revision:\n%s", want, long)
		}
	}
	// The `editTargetId` absence is a decision, not an oversight, so the help
	// says so rather than leaving a script author to wonder where it went.
	if !strings.Contains(long, "editTargetId") {
		t.Errorf("`app listing status` Long does not say why `editTargetId` is absent from the payload:\n%s", long)
	}
}
