package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// TESTS FOR THE LISTING-COMPLETENESS GATE ON `civitai app submit` (civitai/cli#762).
//
// 🔴 EVERY TEST IN THE FIRST HALF OF THIS FILE DRIVES THE REAL COMMAND, NOT THE
// PREDICATE. The defect is a SEQUENCING one — nothing was missing from the CLI's
// detection or from its repair commands, so a unit test of either would have been
// green before and after. What has to be observable is that `app submit` REFUSES,
// that the refusal arrives BEFORE the upload, and that the flag an agent reaches
// for (`--yes`) is not the one that waives it. Each of those is a property of the
// command, and all three are pinned end to end against a fake server.
//
// 🔴 THE FIXTURE CARRIES MORE THAN ONE LISTING, AND THE OTHER ONE IS BROKEN. A
// one-row fixture cannot tell "this app's problems" from "any problem anywhere":
// `listMine` is an ACCOUNT-WIDE read that takes no input and is filtered in Go, so
// a gate that forgot to narrow by slug would pass every single-row test and refuse
// every real submit on an account with one old incomplete app. So every fixture
// below also holds `gate-other-app`, missing BOTH its icon and its cover, and the
// permitted arms assert that a submit goes through anyway.

// listMineTestPath is the tRPC route `appListings.listMine` answers on. Spelled
// as a literal because internal/appapi's route vars are unexported, and a literal
// is what makes the fake answer the route the client really asks for.
const listMineTestPath = "/api/trpc/appListings.listMine"

const gateSlug = "gate-subject-app"

// gateOtherAppRows is the never-the-subject noise described in the header: a
// DIFFERENT app of the caller's, below the publish floor on both assets.
func gateOtherAppRows() []map[string]any {
	return []map[string]any{gateRow("gate-other-app", "apl_OTHER99", "draft", "onsite",
		gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
		gateProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"),
	)}
}

func gateProblem(code, label, severity string) map[string]any {
	return map[string]any{"code": code, "label": label, "severity": severity}
}

func gateRow(slug, listingID, status, kind string, problems ...map[string]any) map[string]any {
	if problems == nil {
		problems = []map[string]any{}
	}
	return map[string]any{
		"appListingId": listingID,
		"slug":         slug,
		"name":         "Gate " + slug,
		"status":       status,
		"role":         "owner",
		"appBlockId":   nil,
		// The real server always sends a kind, so the fake must — omitting it
		// would run this whole file on the unknown-kind arm, i.e. structurally
		// blind to every kind-dependent branch the gate has.
		"kind":     kind,
		"problems": problems,
	}
}

// gateServer answers the submissions route (so the version guard is satisfied),
// the `listMine` route from rows, and records whether the SUBMIT route was hit.
//
// `listMineStatus` non-zero makes the listing read fail with that status, which is
// how the fail-open arm is reached.
func gateServer(t *testing.T, rows []map[string]any, listMineStatus int) (*httptest.Server, *bool, *int) {
	t.Helper()
	submitted := false
	listMineCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath):
			w.Header().Set("Content-Type", "application/json")
			// No approved rows for this slug, so the version guard is silent and
			// whatever refuses below is this gate rather than that one.
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
		case strings.HasPrefix(r.URL.Path, listMineTestPath):
			listMineCalls++
			if listMineStatus != 0 {
				w.WriteHeader(listMineStatus)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"boom"}}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"data": map[string]any{"json": rows}},
			})
		default:
			submitted = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishRequestId": "pubreq_gate", "slug": gateSlug, "version": "0.1.0", "status": "pending",
			})
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &submitted, &listMineCalls
}

// writeGateManifest writes a minimal valid static-page manifest. `tagline` is
// emitted only when non-empty, so the "no tagline in the manifest" case is a
// manifest with no such key — which is what every app scaffolded before this
// change has.
func writeGateManifest(t *testing.T, dir, blockID, tagline string) {
	t.Helper()
	tagField := ""
	if tagline != "" {
		b, err := json.Marshal(tagline)
		if err != nil {
			t.Fatal(err)
		}
		tagField = "  \"tagline\": " + string(b) + ",\n"
	}
	m := `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "` + blockID + `",
  "version": "0.3.0",
  "name": "Gate Subject App",
` + tagField + `  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "Gate Subject App", "icon": "bolt" },
  "iframe": { "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "minApiVersion": "1.0"
}`
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func withGateEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-gate")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/v1/blocks/submit-version")
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
}

// ---------------------------------------------------------------------------
// Which problems refuse a submit, and which do not.
// ---------------------------------------------------------------------------

// TestSubmitListingGateBlocksAndPermitsEndToEnd is the whole table, driven
// through the real command.
//
// 🔴 THE PERMITTED ARMS ARE NOT PADDING. With only the refused arms, an
// implementation that refuses on ANY problem would pass — and that implementation
// is wrong in a way that matters: it would wedge every submit behind a blocked
// screenshot whose removal waits on a moderator, and behind a category only a
// moderator can set on an on-site app.
func TestSubmitListingGateBlocksAndPermitsEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		// manifestTagline is written into block.manifest.json when non-empty.
		manifestTagline string
		problems        []map[string]any
		wantRefused     bool
		// mustSay is asserted on the refusal, or on stderr when permitted.
		mustSay []string
		why     string
	}{
		{
			name: "missing icon refuses",
			kind: "onsite",
			problems: []map[string]any{
				gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking")},
			wantRefused: true,
			mustSay:     []string{"missing-icon", "civitai app listing set-icon"},
			why:         "the publish floor, and `set-icon` works on every listing status and both kinds",
		},
		{
			name: "missing cover refuses",
			kind: "onsite",
			problems: []map[string]any{
				gateProblem("missing-cover", "Missing cover image (required before publishing)", "blocking")},
			wantRefused: true,
			mustSay:     []string{"missing-cover", "civitai app listing set-cover"},
			why:         "the other half of the publish floor",
		},
		{
			name: "an OFFSITE empty description refuses",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("empty-description", "Missing description", "advisory")},
			wantRefused: true,
			mustSay:     []string{"empty-description"},
			why: "server-ADVISORY and exactly what the measured trial shipped without; `set-text " +
				"--description` writes it in place on an offsite listing, so the author can fix it now",
		},
		{
			name: "an OFFSITE empty tagline refuses",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("empty-tagline", "Missing tagline", "advisory")},
			wantRefused: true,
			mustSay:     []string{"empty-tagline"},
			why:         "`set-text --tagline` writes it in place",
		},
		{
			name: "an ONSITE empty tagline refuses when the manifest has none either",
			kind: "onsite",
			problems: []map[string]any{
				gateProblem("empty-tagline", "Missing tagline", "advisory")},
			wantRefused: true,
			mustSay:     []string{"empty-tagline", "block.manifest.json"},
			why: "the manifest is the only author surface for an onsite tagline, and the remedy has to " +
				"name it rather than the browser editor",
		},
		{
			name:            "an ONSITE empty tagline is PERMITTED once the manifest carries one",
			kind:            "onsite",
			manifestTagline: "Batch upscaling, in your browser",
			problems: []map[string]any{
				gateProblem("empty-tagline", "Missing tagline", "advisory")},
			wantRefused: false,
			mustSay:     []string{"empty-tagline"},
			why: "THE DEADLOCK ARM. An onsite tagline is re-derived from the manifest only when a version " +
				"is APPROVED, so refusing here would refuse the one act that fixes the problem — forever",
		},
		{
			name:            "the tagline suppression does NOT swallow a missing icon on the same listing",
			kind:            "onsite",
			manifestTagline: "Batch upscaling, in your browser",
			problems: []map[string]any{
				gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
				gateProblem("empty-tagline", "Missing tagline", "advisory")},
			wantRefused: true,
			mustSay:     []string{"missing-icon", "civitai app listing set-icon"},
			why: "the suppression is scoped to the ONE code it is about; widened to every blocking " +
				"finding it would let a manifest tagline waive the publish floor, and a mutant that " +
				"widens it SURVIVED until this arm existed",
		},
		{
			name: "an ONSITE empty description is PERMITTED",
			kind: "onsite",
			problems: []map[string]any{
				gateProblem("empty-description", "Missing description", "advisory")},
			wantRefused: false,
			mustSay:     []string{"empty-description"},
			why: "`set-text` REFUSES an onsite listing and the vendored schema declares no `description` " +
				"key, so this CLI has no route to name — a refusal whose remedy the credential cannot " +
				"take is worse than no refusal",
		},
		{
			name: "no screenshots is PERMITTED",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory")},
			wantRefused: false,
			mustSay:     []string{"no-screenshots"},
			why: "documented recommended-and-optional, and `add-screenshot` APPENDS with no removal in " +
				"the same command — forcing it buys junk media nobody can take back",
		},
		{
			name: "an empty category is PERMITTED",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("empty-category", "Missing category", "advisory")},
			wantRefused: false,
			mustSay:     []string{"empty-category"},
			why: "on an ONSITE app only a moderator's curation writes the column the store reads, so a " +
				"gate on it would block a submit on somebody else's queue; one rule for both kinds",
		},
		{
			name: "blocked media is PERMITTED even though the server calls it BLOCKING",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("blocked-media", "Replace the blocked screenshot before it can publish", "blocking")},
			wantRefused: false,
			mustSay:     []string{"blocked-media"},
			why: "a blocked SCREENSHOT's remedy ends at a moderator approving a revision, and `listMine` " +
				"keeps reporting it until then — blocking would wedge every submit, and buys nothing " +
				"because the platform refuses go-live on a blocked asset regardless",
		},
		{
			name: "still-scanning media is PERMITTED",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("scanning-media", "Media still scanning", "advisory")},
			wantRefused: false,
			mustSay:     []string{"scanning-media"},
			why:         "it resolves itself; a gate on it makes a submit a coin flip on scan timing",
		},
		{
			name: "an UNKNOWN code is reported and PERMITTED",
			kind: "offsite",
			problems: []map[string]any{
				gateProblem("some-ninth-problem", "Something this CLI has never heard of", "blocking")},
			wantRefused: false,
			mustSay:     []string{"some-ninth-problem"},
			why: "a gate that starts refusing every submit the day the server adds a problem code is a " +
				"gate people waive and never re-enable",
		},
		{
			name: "an UNRECOGNISED kind does not refuse on TEXT",
			kind: "sideways",
			problems: []map[string]any{
				gateProblem("empty-tagline", "Missing tagline", "advisory"),
				gateProblem("empty-description", "Missing description", "advisory")},
			wantRefused: false,
			mustSay:     []string{"empty-tagline"},
			why: "the CLI cannot establish which remedy is even true, and a wrongly-refused submit's only " +
				"way out is the escape hatch",
		},
		{
			name: "an UNRECOGNISED kind still refuses on MEDIA",
			kind: "sideways",
			problems: []map[string]any{
				gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking")},
			wantRefused: true,
			mustSay:     []string{"missing-icon"},
			why:         "the media remedy does not depend on the kind, so nothing is unestablished",
		},
		{
			name:        "a complete listing submits",
			kind:        "onsite",
			problems:    nil,
			wantRefused: false,
			why:         "the negative control on the whole gate",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStdinTTY(t, false) // --yes is passed, so the TTY gate is not what refuses
			tmp := t.TempDir()
			writeGateManifest(t, tmp, gateSlug, tc.manifestTagline)
			rows := append(gateOtherAppRows(),
				gateRow(gateSlug, "apl_GATE11", "draft", tc.kind, tc.problems...))
			srv, submitted, listMineCalls := gateServer(t, rows, 0)
			withGateEnv(t, srv)

			stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")

			if *listMineCalls == 0 {
				t.Fatal("CONTROL failure: the listing read was never made, so this arm's verdict is not " +
					"about the gate at all")
			}
			if tc.wantRefused {
				if err == nil {
					t.Fatalf("`app submit` must refuse — %s.\nstdout:\n%s\nstderr:\n%s", tc.why, stdout, stderr)
				}
				if *submitted {
					t.Error("the submit route was hit — the gate must refuse BEFORE uploading anything")
				}
				if strings.Contains(stdout, "Packaged ") {
					t.Errorf("the gate must run before packaging, got stdout:\n%s", stdout)
				}
				for _, want := range tc.mustSay {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("the refusal does not name %q — %s.\ngot:\n%s", want, tc.why, err.Error())
					}
				}
				// 🔴 THE REFUSAL NAMES THE ESCAPE HATCH *AND* SAYS --yes IS NOT
				// ONE. --yes is the flag a script already passes, so leaving that
				// unsaid invites exactly one retry that cannot work.
				if !strings.Contains(err.Error(), "--allow-incomplete-listing") {
					t.Errorf("the refusal does not name the escape hatch:\n%s", err.Error())
				}
				if !strings.Contains(err.Error(), "--yes does not waive this") {
					t.Errorf("the refusal does not say --yes is not the waiver:\n%s", err.Error())
				}
				// 🔴 AND IT PRINTS NEITHER BUNDLE-ACCOUNT BLOCK, which is the
				// claim README's `## Submit & auth` paragraph and the published
				// Troubleshooting row now make about FIVE pre-upload refusals
				// rather than four. A structural ledger of those lists cannot
				// see this; only driving the command can. Both blocks open with
				// "What this CLI", and either would be a past-tense account of
				// an upload that never happened.
				if strings.Contains(stderr, "What this CLI") {
					t.Errorf("a pre-upload refusal printed a bundle-account block, which claims bytes "+
						"were (or would have been) sent:\n%s", stderr)
				}
				return
			}
			if err != nil {
				t.Fatalf("`app submit` must proceed — %s; got %v\nstdout:\n%s\nstderr:\n%s",
					tc.why, err, stdout, stderr)
			}
			if !*submitted {
				t.Errorf("the submit route was not reached — %s", tc.why)
			}
			// A permitted problem is still REPORTED. Hiding it is half of what
			// made the advisories look optional in the first place.
			for _, want := range tc.mustSay {
				if !strings.Contains(stderr, want) {
					t.Errorf("a permitted problem must still be reported on stderr; %q is missing — %s.\nstderr:\n%s",
						want, tc.why, stderr)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The waiver, and the flag that is NOT the waiver.
// ---------------------------------------------------------------------------

// TestSubmitYesDoesNotWaiveTheListingGate is the point of putting the gate on
// `submit` at all: the flag an agent and a CI script already pass must not open
// it. Both spellings, because `-y` is a real shorthand of the same flag and a
// guard keyed on the long name only would be walkable.
func TestSubmitYesDoesNotWaiveTheListingGate(t *testing.T) {
	for _, yes := range []string{"--yes", "-y"} {
		t.Run(yes, func(t *testing.T) {
			withStdinTTY(t, false)
			tmp := t.TempDir()
			writeGateManifest(t, tmp, gateSlug, "")
			rows := append(gateOtherAppRows(), gateRow(gateSlug, "apl_GATE11", "draft", "onsite",
				gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
			srv, submitted, _ := gateServer(t, rows, 0)
			withGateEnv(t, srv)

			stdout, stderr, err := run(t, "app", "submit", tmp, yes)
			if err == nil {
				t.Fatalf("%s waived the listing gate.\nstdout:\n%s\nstderr:\n%s", yes, stdout, stderr)
			}
			if *submitted {
				t.Errorf("%s reached the submit route past the gate", yes)
			}
		})
	}
}

// TestSubmitAllowIncompleteListingUploadsAnyway is the reachability half of the
// escape hatch: the same refused invocation plus the flag reaches the submit
// route. Without this the gate could be unwaivable and every other test here
// would still pass.
func TestSubmitAllowIncompleteListingUploadsAnyway(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	rows := append(gateOtherAppRows(), gateRow(gateSlug, "apl_GATE11", "draft", "onsite",
		gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
		gateProblem("missing-cover", "Missing cover image (required before publishing)", "blocking")))
	srv, submitted, listMineCalls := gateServer(t, rows, 0)
	withGateEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes", "--allow-incomplete-listing")
	if err != nil {
		t.Fatalf("--allow-incomplete-listing must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("--allow-incomplete-listing must reach the submit route")
	}
	// 🔴 IT RETURNS BEFORE THE NETWORK CALL, like --allow-downgrade. A waiver that
	// still paid a round trip to compute an answer it discards is latency on the
	// path a release script takes, and the assertion is what keeps the early
	// return from being quietly moved below the read.
	if *listMineCalls != 0 {
		t.Errorf("the listing read was made %d time(s) despite the waiver — the waiver must return "+
			"before the network call", *listMineCalls)
	}
}

// ---------------------------------------------------------------------------
// The paths that must be UNCHANGED, and must stay distinguishable.
// ---------------------------------------------------------------------------

// TestSubmitNoTokenPathStillExitsZeroAndSaysNotSubmitted is the trap this gate's
// exit code was chosen against.
//
// 🔴 `civitai app submit` ALREADY EXITS 0 WITHOUT SUBMITTING. With no token it
// writes the canonical .zip, prints `⚠ NOT SUBMITTED`, and returns nil. So a
// refusal that also exited 0 would be indistinguishable from it — and from a real
// submit — in the one command an agent grades itself on. This pins the old path
// unchanged (exit 0, the banner, and NO listing read, because a run that cannot
// upload cannot carry an incomplete listing forward) so that the gate's non-zero
// refusal means something.
//
// ⚠ INVARIANT GUARD, NOT REGRESSION COVERAGE: it passes on the pre-change code
// too, which is the point — it is what says the pre-change behaviour survived.
func TestSubmitNoTokenPathStillExitsZeroAndSaysNotSubmitted(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	rows := append(gateOtherAppRows(), gateRow(gateSlug, "apl_GATE11", "draft", "onsite",
		gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
	srv, submitted, listMineCalls := gateServer(t, rows, 0)
	withGateEnv(t, srv)
	// The one difference from every other case here.
	t.Setenv("CIVITAI_TOKEN", "")

	// The .zip lands in the CWD, so write it somewhere disposable.
	out := filepath.Join(t.TempDir(), "bundle.zip")
	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes", "-o", out)
	if err != nil {
		t.Fatalf("the no-token path must still exit 0; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "NOT SUBMITTED") {
		t.Errorf("the no-token path must still print its NOT SUBMITTED banner:\n%s", stdout)
	}
	if *submitted {
		t.Error("the no-token path reached the submit route")
	}
	if *listMineCalls != 0 {
		t.Errorf("the no-token path made %d listing read(s) — it cannot upload, so there is nothing to "+
			"gate and nothing to spend a round trip on", *listMineCalls)
	}
	if _, serr := os.Stat(out); serr != nil {
		t.Errorf("the no-token path must still write the bundle: %v", serr)
	}
}

// TestSubmitPackageOnlySkipsTheListingGate: --package-only never contacts the
// server, so there is nothing for the gate to read and nothing it could carry
// forward. Same class of invariant guard as the test above.
func TestSubmitPackageOnlySkipsTheListingGate(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	rows := append(gateOtherAppRows(), gateRow(gateSlug, "apl_GATE11", "draft", "onsite",
		gateProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
	srv, submitted, listMineCalls := gateServer(t, rows, 0)
	withGateEnv(t, srv)

	out := filepath.Join(t.TempDir(), "bundle.zip")
	stdout, stderr, err := run(t, "app", "submit", tmp, "--package-only", "-o", out)
	if err != nil {
		t.Fatalf("--package-only must not be gated; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if *submitted || *listMineCalls != 0 {
		t.Errorf("--package-only contacted the server (submitted=%v, listMine=%d)", *submitted, *listMineCalls)
	}
}

// TestSubmitListingGateFailsOpenOnAReadError: this is an accident preventer, not
// an authorization check. Hard-failing here would let one API blip block every
// submit in the fleet, which is strictly worse than the defect being prevented —
// so it WARNS and proceeds. The warning is what keeps the fail-open from being
// silent.
func TestSubmitListingGateFailsOpenOnAReadError(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	srv, submitted, listMineCalls := gateServer(t, nil, http.StatusInternalServerError)
	withGateEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("a failed listing read must not block a submit; got %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout, stderr)
	}
	if *listMineCalls == 0 {
		t.Fatal("CONTROL failure: the read was never attempted, so nothing failed open")
	}
	if !*submitted {
		t.Error("the submit route was not reached")
	}
	if !strings.Contains(stderr, "without the listing-completeness gate") {
		t.Errorf("a fail-open must SAY so; stderr:\n%s", stderr)
	}
}

// TestSubmitFirstSubmitIsNotGated: a first submit is what MINTS the store
// listing, so before it there is no listing to read and nothing to refuse. The
// account still holds a broken OTHER app, which is what makes this a real
// assertion rather than a fixture with nothing in it.
func TestSubmitFirstSubmitIsNotGated(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	srv, submitted, listMineCalls := gateServer(t, gateOtherAppRows(), 0)
	withGateEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("a first submit has no listing yet and must not be refused; got %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout, stderr)
	}
	if *listMineCalls == 0 {
		t.Fatal("CONTROL failure: the read was never made, so this arm proves nothing about the gate")
	}
	if !*submitted {
		t.Error("the submit route was not reached")
	}
	// 🔴 AND ANOTHER APP'S PROBLEMS MUST NOT BE REPORTED AGAINST THIS ONE.
	// `listMine` is account-wide and filtered in Go; a gate that forgot to narrow
	// would print `gate-other-app`'s findings here.
	if strings.Contains(stderr, "gate-other-app") {
		t.Errorf("another app's listing problems leaked into this submit's report:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// The sentinel and the exit-code class.
// ---------------------------------------------------------------------------

// TestSubmitListingIncompleteCarriesItsOwnSentinel pins the two facts that decide
// how the refusal is PRESENTED, neither of which any assertion above can see.
//
// 🔴 IT MUST BE ErrListingIncomplete AND MUST NOT BE ErrListingBlocked.
// `cmd/civitai`'s `errorLine` is a deliberate whitelist of ONE — ErrListingBlocked
// renders WITHOUT the "Error: " prefix, because `app doctor`'s job is to return a
// verdict. `app submit`'s job is to submit, so a refusal there is a failure of
// what the caller asked for and keeps the prefix like every other submit refusal.
// Re-using doctor's sentinel would have silently un-prefixed it, and nothing else
// in the suite would have noticed.
func TestSubmitListingIncompleteCarriesItsOwnSentinel(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeGateManifest(t, tmp, gateSlug, "")
	rows := append(gateOtherAppRows(), gateRow(gateSlug, "apl_GATE11", "draft", "onsite",
		gateProblem("missing-cover", "Missing cover image (required before publishing)", "blocking")))
	srv, _, _ := gateServer(t, rows, 0)
	withGateEnv(t, srv)

	_, _, err := run(t, "app", "submit", tmp, "--yes")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !errors.Is(err, ErrListingIncomplete) {
		t.Errorf("the refusal must carry ErrListingIncomplete (that is what pins exit code 1), got %#v", err)
	}
	if errors.Is(err, ErrListingBlocked) {
		t.Error("the refusal must NOT carry ErrListingBlocked — that sentinel is cmd/civitai's whitelist " +
			"for printing an error with no `Error: ` prefix, and a submit refusal is a failure of the " +
			"thing the caller asked for")
	}
	// 🔴 NOT A USAGE ERROR. Every flag, argument and path is well-formed; what is
	// incomplete is the listing. Exit 2 would send someone re-reading their
	// command line.
	if errors.Is(err, ErrUsage) {
		t.Error("the refusal is tagged as a USAGE error, which would publish exit 2 for a verdict about " +
			"the project")
	}
}

// TestSubmitGateBlockTableIsWhatItClaims drives the classifier directly, over the
// (code, kind) pairs the end-to-end table cannot enumerate cheaply.
//
// 🔴 IT IS NOT REDUNDANT WITH THE TABLE ABOVE, AND NEITHER SUBSUMES THE OTHER.
// That one proves the COMMAND refuses; this one pins which pairs the RULE covers,
// including the two `!listingIsOnsite`-vs-`listingIsOffsite` cases where the
// obvious implementation and the correct one differ only on an unrecognised kind.
func TestSubmitGateBlockTableIsWhatItClaims(t *testing.T) {
	for _, tc := range []struct {
		code, kind string
		want       bool
	}{
		{problemMissingIcon, "onsite", true},
		{problemMissingIcon, "offsite", true},
		{problemMissingIcon, "", true},
		{problemMissingCover, "onsite", true},
		{problemMissingCover, "offsite", true},
		{problemEmptyTagline, "offsite", true},
		{problemEmptyTagline, "OffSite", true}, // normalised, like every sibling predicate
		{problemEmptyTagline, "onsite", true},
		{problemEmptyTagline, "OnSite", true},
		{problemEmptyTagline, "", false},
		{problemEmptyTagline, "sideways", false},
		{problemEmptyDescription, "offsite", true},
		{problemEmptyDescription, "onsite", false},
		{problemEmptyDescription, "", false},
		{problemEmptyDescription, "sideways", false},
		{problemNoScreenshots, "offsite", false},
		{problemEmptyCategory, "offsite", false},
		{problemBlockedMedia, "offsite", false},
		{problemScanningMedia, "offsite", false},
		{"some-ninth-problem", "offsite", false},
	} {
		if got := submitGateBlocks(tc.code, tc.kind); got != tc.want {
			t.Errorf("submitGateBlocks(%q, %q) = %v, want %v", tc.code, tc.kind, got, tc.want)
		}
	}
	// CONTROL: a table that agreed with a constant function would be vacuous.
	blocks, permits := 0, 0
	for _, kind := range []string{"onsite", "offsite", "", "sideways"} {
		for _, code := range allEightCodes() {
			if submitGateBlocks(code, kind) {
				blocks++
			} else {
				permits++
			}
		}
	}
	if blocks == 0 || permits == 0 {
		t.Fatalf("the classifier is constant over the eight codes x four kinds (%d block, %d permit) — "+
			"the table above would agree with a stub", blocks, permits)
	}
}
