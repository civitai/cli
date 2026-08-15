package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// TESTS FOR THE MONOTONIC-VERSION GUARD (issue #412).
//
// 🔴 ANTI-VACUITY IS THE POINT OF THIS FILE'S FIXTURES, NOT A NICETY. A list
// holding ONE submission cannot tell "highest APPROVED" from "newest row by
// submittedAt": both answers are that row, so a guard implementing either
// passes. That is precisely how #390 shipped green against a single-row fixture.
//
// So `regressionFixture` DISAGREES on that axis by construction: its newest row
// is a WITHDRAWN 0.6.0, the row after it a PENDING 0.7.1, and the highest
// version anywhere is a REJECTED 0.9.9 — while the answer the guard must reach
// is the APPROVED 0.5.2, which is neither the newest nor the highest. Another
// app's approved 9.9.9 sits in the list too, so an unfiltered scan is a fourth
// distinct wrong answer.
//
// 🔴 AND THE NEWEST *APPROVED* ROW IS ALSO NOT THE ANSWER — 0.4.9, a rollback
// approved after 0.5.2 whose deploy failed. That fifth row is not decoration: it
// is the one the first version of this fixture LACKED, and a hand-mutation that
// took the first approved row of a newest-first list instead of the maximum
// SURVIVED the whole file until it was added. "Newest approved" and "highest
// approved" are two different predicates, and only a fixture where they diverge
// can tell them apart.
//
// Every id, version, status, timestamp and deploy state is pairwise distinct, so
// a read of the wrong field cannot coincide with the right answer.

const guardSlug = "custom-generators"

func strptr(s string) *string { return &s }

// regressionFixture is the deliberately-disagreeing row set described above.
// The server returns submissions newest-first, so this is ordered that way.
func regressionFixture() []appapi.Submission {
	return []appapi.Submission{
		// Newest row overall — HIGHER version than the approved one, withdrawn.
		{ID: "pubreq_withdrawn", BlockID: guardSlug, Version: "0.6.0", Status: "withdrawn", SubmittedAt: "2026-08-14T10:00:00Z"},
		// A different app entirely, approved+live at a much higher version.
		{ID: "pubreq_otherapp", BlockID: "gen-matrix", Version: "9.9.9", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-13T11:00:00Z"},
		// Higher again, pending — in review, not serving.
		{ID: "pubreq_pending", BlockID: guardSlug, Version: "0.7.1", Status: "pending", SubmittedAt: "2026-08-12T09:00:00Z"},
		// The NEWEST APPROVED row — and a LOWER version: a rollback approved
		// after 0.5.2 whose deploy failed. "Newest approved" is a wrong answer
		// only this row can expose.
		{ID: "pubreq_rollback", BlockID: guardSlug, Version: "0.4.9", Status: "approved", DeployState: strptr("failed"), SubmittedAt: "2026-08-11T12:00:00Z"},
		// THE ANSWER: highest approved for this slug, and deployed.
		{ID: "pubreq_approved", BlockID: guardSlug, Version: "0.5.2", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
		// Highest version anywhere for this slug — rejected, so never served.
		{ID: "pubreq_rejected", BlockID: guardSlug, Version: "0.9.9", Status: "rejected", SubmittedAt: "2026-07-20T07:00:00Z"},
	}
}

// listerFor returns a submissionLister over rows, plus a pointer to its call
// count so a test can assert the network read was (or was not) made.
func listerFor(rows []appapi.Submission) (submissionLister, *int) {
	calls := 0
	return func(_ context.Context, _ string) ([]appapi.Submission, error) {
		calls++
		return rows, nil
	}, &calls
}

func failingLister(err error) submissionLister {
	return func(_ context.Context, _ string) ([]appapi.Submission, error) { return nil, err }
}

// listerCapturing returns a submissionLister plus a pointer to the blockId
// ARGUMENTS it was called with. listerFor discards them, which is why a mutation
// of the narrowing argument to "" survived the original battery: every fixture
// is filtered slug-side afterwards, so the RESULT is identical and only the
// argument itself can tell the two apart.
func listerCapturing(rows []appapi.Submission) (submissionLister, *[]string) {
	var got []string
	return func(_ context.Context, blockID string) ([]appapi.Submission, error) {
		got = append(got, blockID)
		return rows, nil
	}, &got
}

// guardRun runs the guard over rows and returns (warnings-written, err).
func guardRun(t *testing.T, rows []appapi.Submission, version string, allowDowngrade bool) (string, error) {
	t.Helper()
	lister, _ := listerFor(rows)
	var warn bytes.Buffer
	err := checkVersionNotRegression(context.Background(), lister, &warn, guardSlug, version, allowDowngrade)
	return warn.String(), err
}

// --- the unit under the guard: which row does it pick? ---

// TestHighestApprovedVersionIsNotTheNewestRow is the anti-vacuity assertion
// stated directly. Against a fixture whose newest row, whose highest version and
// whose approved peak are THREE DIFFERENT rows, only the approved peak is
// acceptable.
func TestHighestApprovedVersionIsNotTheNewestRow(t *testing.T) {
	peak := highestApprovedVersion(regressionFixture(), guardSlug)
	if !peak.found {
		t.Fatal("no approved version found — the fixture holds two approved rows for this slug")
	}
	if peak.version != "0.5.2" {
		t.Errorf("highest approved = %q, want 0.5.2.\n"+
			"0.6.0 is the NEWEST row (withdrawn), 0.7.1 is pending, 0.9.9 is the highest version (rejected), "+
			"9.9.9 belongs to another app, and 0.4.9 is the NEWEST APPROVED row (a failed rollback) — "+
			"each is a distinct wrong answer, and 0.4.9 in particular is the one a first-approved-row pick returns.",
			peak.version)
	}
	if !peak.live {
		t.Error("the winning approved row carries deployState=live; peak.live should say so")
	}
	// State the divergence the fixture is built on, so a later edit that
	// flattens it fails HERE rather than silently making every other case in
	// this file vacuous.
	newestApproved := ""
	for _, s := range regressionFixture() {
		if s.BlockID == guardSlug && s.Status == "approved" {
			newestApproved = s.Version
			break // the list is newest-first
		}
	}
	if newestApproved == peak.version {
		t.Fatalf("the fixture's newest approved row (%s) equals its highest approved row — the fixture "+
			"cannot distinguish the two predicates, so every case in this file would pass under either", newestApproved)
	}
}

// TestHighestApprovedVersionIgnoresOtherApps pins the slug filter on its own:
// given ONLY another app's approved row, there is no answer for this slug.
func TestHighestApprovedVersionIgnoresOtherApps(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_otherapp", BlockID: "gen-matrix", Version: "9.9.9", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-13T11:00:00Z"},
	}
	if peak := highestApprovedVersion(rows, guardSlug); peak.found {
		t.Errorf("another app's approved row must not answer for %s, got %q", guardSlug, peak.version)
	}
}

// --- POSITIVE CONTROL: the case that MUST be refused ---

// TestVersionGuardRefusesBelowHighestApproved is the issue's own scenario:
// custom-generators@0.4.1 against a live 0.5.2.
func TestVersionGuardRefusesBelowHighestApproved(t *testing.T) {
	_, err := guardRun(t, regressionFixture(), "0.4.1", false)
	if err == nil {
		t.Fatal("submitting 0.4.1 while 0.5.2 is approved+live must be refused")
	}
	// The SENTINEL, not a substring — the exit-code contract is pinned against
	// this, and a string match would survive a reworded message.
	if !errors.Is(err, ErrVersionRegression) {
		t.Errorf("refusal must carry ErrVersionRegression, got %#v", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"refusing to submit custom-generators@0.4.1",
		"0.5.2 is already approved and live",
		"Submitting an older version replaces the newer live deployment on approval.",
		"--allow-downgrade",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal message missing %q, got:\n%s", want, msg)
		}
	}
	// A wrong-row read would name one of these instead of 0.5.2.
	for _, never := range []string{"0.6.0", "0.7.1", "0.9.9", "9.9.9", "0.4.9"} {
		if strings.Contains(msg, never) {
			t.Errorf("refusal names %q — it compared against the wrong row:\n%s", never, msg)
		}
	}
}

// TestVersionGuardRefusesEqualVersion — resubmitting the live version is what a
// behind-live repo produces naturally, so equality refuses too, with its own
// middle line (nothing is being replaced by something OLDER there).
func TestVersionGuardRefusesEqualVersion(t *testing.T) {
	_, err := guardRun(t, regressionFixture(), "0.5.2", false)
	if err == nil {
		t.Fatal("resubmitting the approved version 0.5.2 must be refused")
	}
	if !errors.Is(err, ErrVersionRegression) {
		t.Errorf("equal-version refusal must carry ErrVersionRegression, got %#v", err)
	}
	if !strings.Contains(err.Error(), "Resubmitting the version that is already live is almost always an accident.") {
		t.Errorf("equal-version refusal should say resubmitting the live version is almost always an accident, got:\n%s", err.Error())
	}
}

// TestVersionGuardDropsTheLiveClaimWhenNotDeployed — "approved and live" is a
// claim about deployState, and the guard must not make it for an approved row
// that has not finished deploying.
func TestVersionGuardDropsTheLiveClaimWhenNotDeployed(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_building", BlockID: guardSlug, Version: "0.5.2", Status: "approved", DeployState: strptr("building"), SubmittedAt: "2026-08-01T08:00:00Z"},
	}
	_, err := guardRun(t, rows, "0.4.1", false)
	if err == nil {
		t.Fatal("0.4.1 below an approved 0.5.2 must be refused whatever its deploy state")
	}
	if strings.Contains(err.Error(), "approved and live") {
		t.Errorf("deployState is \"building\", so the message must not claim live:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "0.5.2 is already approved.") {
		t.Errorf("message should still name the approved version:\n%s", err.Error())
	}
}

// --- NEGATIVE CONTROLS: the adjacent cases that must still pass ---

// TestVersionGuardAllowsStrictlyHigher — the ordinary forward bump, and a bump
// that also clears the higher NON-approved rows.
func TestVersionGuardAllowsStrictlyHigher(t *testing.T) {
	for _, v := range []string{"0.5.3", "0.6.0", "0.8.0", "1.0.0"} {
		warn, err := guardRun(t, regressionFixture(), v, false)
		if err != nil {
			t.Errorf("%s is above the approved 0.5.2 and must be allowed, got: %v", v, err)
		}
		if warn != "" {
			t.Errorf("%s: the happy path must be silent, got warning:\n%s", v, warn)
		}
	}
}

// TestVersionGuardAllowDowngradeSkipsTheCheckEntirely — the escape hatch, and it
// must not even make the network read: a deliberate rollback should not pay for
// an answer it would discard, and a release script must not fail on it during an
// API outage.
func TestVersionGuardAllowDowngradeSkipsTheCheckEntirely(t *testing.T) {
	lister, calls := listerFor(regressionFixture())
	var warn bytes.Buffer
	if err := checkVersionNotRegression(context.Background(), lister, &warn, guardSlug, "0.4.1", true); err != nil {
		t.Fatalf("--allow-downgrade must permit 0.4.1 under an approved 0.5.2, got: %v", err)
	}
	if *calls != 0 {
		t.Errorf("--allow-downgrade made %d submissions call(s); it must short-circuit before the network", *calls)
	}
	if warn.String() != "" {
		t.Errorf("--allow-downgrade must be silent, got:\n%s", warn.String())
	}
}

// TestVersionGuardFirstSubmitHasNothingToCompare — an app with no prior
// submissions at all. Allowed, and SILENT: a first submit is the normal case.
func TestVersionGuardFirstSubmitHasNothingToCompare(t *testing.T) {
	warn, err := guardRun(t, nil, "0.1.0", false)
	if err != nil {
		t.Fatalf("a first submit must be allowed, got: %v", err)
	}
	if warn != "" {
		t.Errorf("a first submit must be silent, got:\n%s", warn)
	}
}

// TestVersionGuardNoApprovedRowAtAll — rows exist, but none is approved. Nothing
// is serving, so nothing can be regressed, and a LOWER version than every
// pending/rejected/withdrawn row is still fine.
func TestVersionGuardNoApprovedRowAtAll(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_p", BlockID: guardSlug, Version: "0.7.1", Status: "pending", SubmittedAt: "2026-08-12T09:00:00Z"},
		{ID: "pubreq_r", BlockID: guardSlug, Version: "0.9.9", Status: "rejected", SubmittedAt: "2026-07-20T07:00:00Z"},
		{ID: "pubreq_w", BlockID: guardSlug, Version: "0.6.0", Status: "withdrawn", SubmittedAt: "2026-08-14T10:00:00Z"},
	}
	warn, err := guardRun(t, rows, "0.2.0", false)
	if err != nil {
		t.Fatalf("no approved row means nothing to regress; 0.2.0 must be allowed, got: %v", err)
	}
	if warn != "" {
		t.Errorf("no approved row must be silent, got:\n%s", warn)
	}
}

// --- SEMVER ORDERING (a string compare must fail BOTH halves) ---

// TestVersionGuardOrdersBySemverNotByString. 0.10.0 sorts BELOW 0.9.0 as a
// string, so a string-compare implementation refuses the legitimate bump and
// permits the real regression — this test asserts both directions so either
// mutation is caught.
func TestVersionGuardOrdersBySemverNotByString(t *testing.T) {
	approved09 := []appapi.Submission{
		{ID: "pubreq_a09", BlockID: guardSlug, Version: "0.9.0", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
	}
	if _, err := guardRun(t, approved09, "0.10.0", false); err != nil {
		t.Errorf("0.10.0 > 0.9.0 by semver and must be allowed (a string compare refuses it), got: %v", err)
	}

	approved010 := []appapi.Submission{
		{ID: "pubreq_a010", BlockID: guardSlug, Version: "0.10.0", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-02T08:00:00Z"},
	}
	_, err := guardRun(t, approved010, "0.9.0", false)
	if err == nil {
		t.Error("0.9.0 < 0.10.0 by semver and must be refused (a string compare permits it)")
	} else if !strings.Contains(err.Error(), "0.10.0 is already approved") {
		t.Errorf("refusal should name the approved 0.10.0, got:\n%s", err.Error())
	}

	// The same inversion one field up, so a guard that only orders the patch
	// component is caught too.
	approved2 := []appapi.Submission{
		{ID: "pubreq_a2", BlockID: guardSlug, Version: "2.0.0", Status: "approved", SubmittedAt: "2026-08-03T08:00:00Z"},
	}
	if _, err := guardRun(t, approved2, "10.0.0", false); err != nil {
		t.Errorf("10.0.0 > 2.0.0 by semver and must be allowed, got: %v", err)
	}
}

// --- UNORDERABLE VERSIONS: warn, never silently proceed, never hard-fail ---

// TestVersionGuardWarnsOnUnorderableLocalVersion — a pre-release local version
// cannot be ordered against a plain approved one without a full semver
// implementation, so the guard says so and proceeds. Refusing here would block
// every app on a pre-release tag.
func TestVersionGuardWarnsOnUnorderableLocalVersion(t *testing.T) {
	for _, v := range []string{"0.5.0-beta.1", "0.4.1+build.7", "0.5.2-rc.1"} {
		warn, err := guardRun(t, regressionFixture(), v, false)
		if err != nil {
			t.Errorf("%s: an unorderable local version must not hard-fail, got: %v", v, err)
		}
		if !strings.Contains(warn, "cannot order") || !strings.Contains(warn, v) {
			t.Errorf("%s: the guard must WARN that it could not order the version, got:\n%s", v, warn)
		}
		if !strings.Contains(warn, "0.5.2") {
			t.Errorf("%s: the warning should name the approved version it could not compare against, got:\n%s", v, warn)
		}
	}
}

// TestVersionGuardWarnsAndStillComparesWhenSomeApprovedRowsAreUnorderable — a
// mixed set. The unorderable approved row is REPORTED, and the guard still does
// its job against the rows it can order.
func TestVersionGuardWarnsAndStillComparesWhenSomeApprovedRowsAreUnorderable(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_rc", BlockID: guardSlug, Version: "2.0.0-rc.1", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-10T10:00:00Z"},
		{ID: "pubreq_ga", BlockID: guardSlug, Version: "1.5.0", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-05T09:00:00Z"},
	}
	warn, err := guardRun(t, rows, "1.4.0", false)
	if err == nil {
		t.Fatal("1.4.0 is below the orderable approved 1.5.0 and must be refused")
	}
	if !strings.Contains(err.Error(), "1.5.0 is already approved") {
		t.Errorf("refusal should compare against the orderable 1.5.0, got:\n%s", err.Error())
	}
	if !strings.Contains(warn, "2.0.0-rc.1") || !strings.Contains(warn, "ignoring 1 approved version(s)") {
		t.Errorf("the skipped approved version must be reported, not silently dropped, got:\n%s", warn)
	}
}

// TestVersionGuardWarnsWhenEveryApprovedRowIsUnorderable — nothing orderable to
// compare against: warn and proceed, never refuse.
func TestVersionGuardWarnsWhenEveryApprovedRowIsUnorderable(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_only", BlockID: guardSlug, Version: "1.0.0-rc.1", Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-10T10:00:00Z"},
	}
	warn, err := guardRun(t, rows, "0.1.0", false)
	if err != nil {
		t.Fatalf("no orderable approved version means no verdict; must not refuse, got: %v", err)
	}
	if !strings.Contains(warn, "1.0.0-rc.1") {
		t.Errorf("the guard must say which approved version it could not order, got:\n%s", warn)
	}
}

// --- NETWORK FAILURE: warn and proceed (an accident preventer, not a gate) ---

// TestVersionGuardWarnsAndProceedsWhenTheLookupFails — a transient API failure
// must not block every submit in the fleet. It must also not be silent.
func TestVersionGuardWarnsAndProceedsWhenTheLookupFails(t *testing.T) {
	var warn bytes.Buffer
	boom := errors.New("dial tcp 1.2.3.4:443: connect: connection refused")
	err := checkVersionNotRegression(context.Background(), failingLister(boom), &warn, guardSlug, "0.4.1", false)
	if err != nil {
		t.Fatalf("a failed submissions lookup must not block the submit, got: %v", err)
	}
	s := warn.String()
	if !strings.Contains(s, "could not check") || !strings.Contains(s, guardSlug) {
		t.Errorf("the fail-open must be announced, got:\n%s", s)
	}
	if !strings.Contains(s, "connection refused") {
		t.Errorf("the warning should carry the underlying error so the cause is visible, got:\n%s", s)
	}
}

// --- END TO END: the guard is actually WIRED into `civitai app submit` ---
//
// 🔴 A unit test on checkVersionNotRegression proves the predicate, never that
// the command REACHES it. This drives the real command and asserts the submit
// endpoint is never touched — the whole point of the issue is that the damage
// happens server-side on approval.

// writeManifestVersion writes a minimal valid static-page manifest with an
// explicit blockId + version. (writeStaticManifest in app_submit_cmd_test.go is
// fixed at demo-block@0.1.0.)
func writeManifestVersion(t *testing.T, dir, blockID, version string) {
	t.Helper()
	m := fmt.Sprintf(`{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": %q,
  "version": %q,
  "name": "Custom Generators",
  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "Custom Generators", "icon": "bolt" },
  "iframe": { "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "minApiVersion": "1.0"
}`, blockID, version)
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// submitGuardServer serves the submissions route from rows and records whether
// the submit route was ever hit.
func submitGuardServer(t *testing.T, rows []appapi.Submission) (*httptest.Server, *bool) {
	t.Helper()
	submitted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": rows})
			return
		}
		submitted = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"publishRequestId": "pubreq_new", "slug": guardSlug, "version": "x", "status": "pending",
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &submitted
}

func withGuardEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-test")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/v1/blocks/submit-version")
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
}

// TestAppSubmitRefusesARegressionAndNeverUploads is the reachability half of the
// positive control.
func TestAppSubmitRefusesARegressionAndNeverUploads(t *testing.T) {
	withStdinTTY(t, false) // --yes is passed, so the TTY gate is not what refuses
	tmp := t.TempDir()
	writeManifestVersion(t, tmp, guardSlug, "0.4.1")
	srv, submitted := submitGuardServer(t, regressionFixture())
	withGuardEnv(t, srv)

	stdout, _, err := run(t, "app", "submit", tmp, "--yes")
	if err == nil {
		t.Fatalf("`app submit` must refuse 0.4.1 under an approved 0.5.2; stdout:\n%s", stdout)
	}
	if !errors.Is(err, ErrVersionRegression) {
		t.Errorf("the command error must carry ErrVersionRegression (that is what pins exit code 1), got %#v", err)
	}
	if !strings.Contains(err.Error(), "0.5.2 is already approved and live") {
		t.Errorf("refusal should name the approved version, got:\n%s", err.Error())
	}
	if *submitted {
		t.Error("the submit route was hit — the guard must refuse BEFORE uploading anything")
	}
	if strings.Contains(stdout, "Packaged ") {
		t.Errorf("the guard must run before packaging, got stdout:\n%s", stdout)
	}
}

// TestAppSubmitAllowDowngradeUploadsAnyway is the reachability half of the
// escape hatch: the same refused invocation, plus --allow-downgrade, reaches the
// submit route.
func TestAppSubmitAllowDowngradeUploadsAnyway(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeManifestVersion(t, tmp, guardSlug, "0.4.1")
	srv, submitted := submitGuardServer(t, regressionFixture())
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes", "--allow-downgrade")
	if err != nil {
		t.Fatalf("--allow-downgrade must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("--allow-downgrade must reach the submit route")
	}
}

// TestAppSubmitAllowsAForwardBumpEndToEnd is the negative control on the same
// wiring: an ordinary forward bump is untouched by the new gate.
func TestAppSubmitAllowsAForwardBumpEndToEnd(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeManifestVersion(t, tmp, guardSlug, "0.6.1")
	srv, submitted := submitGuardServer(t, regressionFixture())
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("0.6.1 is above the approved 0.5.2 and must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !*submitted {
		t.Error("a forward bump must reach the submit route")
	}
}

// --- MESSAGE ACCURACY: every line conditional on what is KNOWN ---
//
// 🔴 THE ORIGINAL BATTERY TESTED DECISIONS, NOT WORDING, and the wording had a
// self-contradiction no decision test can see: the equal-version branch
// hard-coded "the version that is already live" while line 1 had already
// dropped "and live" for a non-live row. There was no equal + NOT-live fixture
// anywhere in this file, which is exactly why it survived.

// TestVersionGuardEqualVersionThatIsNotLiveNeverClaimsItIsLive is that missing
// combination. An approved row whose deploy FAILED, resubmitted at the same
// version, is the retry path — the one case in this guard where sending the
// same version again is a plausible deliberate act. Every line must agree that
// it is not live, and none may tell the author their repo is behind: the repo
// is at exactly the version the server holds.
func TestVersionGuardEqualVersionThatIsNotLiveNeverClaimsItIsLive(t *testing.T) {
	for _, deployState := range []string{"failed", "building", "deploying"} {
		rows := []appapi.Submission{
			{ID: "pubreq_notlive", BlockID: guardSlug, Version: "0.5.2", Status: "approved",
				DeployState: strptr(deployState), SubmittedAt: "2026-08-01T08:00:00Z"},
		}
		_, err := guardRun(t, rows, "0.5.2", false)
		if err == nil {
			t.Fatalf("deployState=%q: resubmitting the approved 0.5.2 must still be refused", deployState)
		}
		msg := err.Error()
		// Line 1 already says "approved.", so nothing below it may say live.
		for _, never := range []string{
			"already live",
			"approved and live",
			"almost always an accident",
			"Your repo may be behind what was last released",
		} {
			if strings.Contains(msg, never) {
				t.Errorf("deployState=%q: the approved row is NOT live and the repo is not behind, "+
					"so the refusal must not say %q:\n%s", deployState, never, msg)
			}
		}
		if !strings.Contains(msg, "approved but not live") {
			t.Errorf("deployState=%q: the refusal should name the state it actually found, got:\n%s", deployState, msg)
		}
		if !strings.Contains(msg, "resubmit of a deploy that has not landed") {
			t.Errorf("deployState=%q: this is the not-yet-landed RETRY path and the message should say so "+
				"rather than calling it an accident, got:\n%s", deployState, msg)
		}
		// 🔴 "never landed" is FALSE for building/deploying — those are deploys
		// IN PROGRESS, and this arm covers all three states with one sentence.
		if strings.Contains(msg, "never landed") {
			t.Errorf("deployState=%q: a deploy that is still building or deploying has not landed YET; "+
				"claiming it never landed is a statement about the future:\n%s", deployState, msg)
		}
		// 🔴 The CLI packages whatever is on disk (#411: the tree may be dirty),
		// so it cannot know the bundle is the same one that was submitted before.
		if strings.Contains(msg, "unchanged") {
			t.Errorf("deployState=%q: the CLI packages the working tree and cannot know the bundle is "+
				"unchanged, so the advice must not claim it:\n%s", deployState, msg)
		}
		// The escape hatch still has to be reachable from the text.
		if !strings.Contains(msg, "--allow-downgrade") {
			t.Errorf("deployState=%q: the refusal must still name the escape hatch, got:\n%s", deployState, msg)
		}
	}
}

// TestVersionGuardEqualVersionThatIsLiveStillCallsItAnAccident is the other half
// of the same switch — the negative control that keeps the fix from collapsing
// both equal cases into one wording. Here "already live" is TRUE.
func TestVersionGuardEqualVersionThatIsLiveStillCallsItAnAccident(t *testing.T) {
	_, err := guardRun(t, regressionFixture(), "0.5.2", false)
	if err == nil {
		t.Fatal("resubmitting the live 0.5.2 must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "0.5.2 is already approved and live") {
		t.Errorf("the winning row IS live and line 1 should say so, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Resubmitting the version that is already live is almost always an accident.") {
		t.Errorf("a live equal-version resubmit is the accident case, got:\n%s", msg)
	}
	if strings.Contains(msg, "approved but not live") {
		t.Errorf("the not-live wording must not leak into the live case:\n%s", msg)
	}
}

// TestVersionGuardLowerVersionKeepsTheBehindWording is the third arm: only the
// LOWER case may say the repo is behind, because only there is it true.
func TestVersionGuardLowerVersionKeepsTheBehindWording(t *testing.T) {
	_, err := guardRun(t, regressionFixture(), "0.4.1", false)
	if err == nil {
		t.Fatal("0.4.1 under an approved 0.5.2 must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Your repo may be behind what was last released") {
		t.Errorf("a LOWER version is the case where the repo really may be behind, got:\n%s", msg)
	}
	if strings.Contains(msg, "approved but not live") || strings.Contains(msg, "almost always an accident") {
		t.Errorf("neither equal-case wording belongs on the lower-version path:\n%s", msg)
	}
}

// --- NORMALISATION: the layer the original battery left entirely unpinned ---
//
// 🔴 EACH OF THESE KILLS A MUTANT THAT SURVIVED THE WHOLE FILE. The guard
// normalises status, deployState and the "v" prefix, and until these tests
// existed every one of those normalisations was dead code as far as the suite
// was concerned: strip it and 39/39 stayed green.

// TestHighestApprovedVersionNormalisesTheStatus — mutant (a): replacing
// EqualFold+TrimSpace on Status with an exact `!= "approved"` compare. A server
// that cases or pads the field turns BOTH halves of #412 OFF (this guard stops
// refusing, `app status`'s drift line stops warning), and off is silent.
//
// 🔴 THIS IS THE MERGED SURVIVOR OF A DUPLICATE PAIR. #413 added a test of this
// exact name against its own copy of the predicate; with one copy left
// (approved_version.go) the two are the same assertion, so #413's distinct
// fixtures were folded in here rather than kept as a second test — "\tapproved\n"
// below, and "not approved"/"preapproved" in the negative control. Those last
// two are the control on the NORMALISATION itself: folding case and trimming
// space must not decay into a substring match. See the note where the duplicate
// stood in app_status_drift_test.go.
func TestHighestApprovedVersionNormalisesTheStatus(t *testing.T) {
	for _, status := range []string{" Approved ", "APPROVED", "Approved", "approved\t", "\tapproved\n"} {
		rows := []appapi.Submission{
			{ID: "pubreq_cased", BlockID: guardSlug, Version: "0.5.2", Status: status,
				DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
		}
		peak := highestApprovedVersion(rows, guardSlug)
		if !peak.found {
			t.Errorf("Status=%q must count as approved — an exact compare here silently disables the guard", status)
			continue
		}
		if peak.version != "0.5.2" {
			t.Errorf("Status=%q: peak = %q, want 0.5.2", status, peak.version)
		}
		// And the whole way through the guard, not just the picker.
		_, err := guardRun(t, rows, "0.4.1", false)
		if err == nil {
			t.Errorf("Status=%q: 0.4.1 under that approved 0.5.2 must be refused", status)
		} else if !strings.Contains(err.Error(), "0.5.2 is already approved") {
			t.Errorf("Status=%q: refusal should name 0.5.2, got:\n%s", status, err.Error())
		}
	}
	// Negative control: normalising the case must not make an UNRELATED status
	// approved, and must not decay into a SUBSTRING match. Without this,
	// `EqualFold(s.Status, "")`-style breakage passes — and so does a
	// `strings.Contains(…, "approved")`, which "not approved" and "preapproved"
	// (folded in from #413's duplicate) are the two shapes that catch.
	for _, status := range []string{"pending", "Rejected", " withdrawn ", "approved-pending", "not approved", "preapproved"} {
		rows := []appapi.Submission{
			{ID: "pubreq_other", BlockID: guardSlug, Version: "0.5.2", Status: status, SubmittedAt: "2026-08-01T08:00:00Z"},
		}
		if peak := highestApprovedVersion(rows, guardSlug); peak.found {
			t.Errorf("Status=%q must NOT count as approved, got peak %q", status, peak.version)
		}
	}
}

// TestHighestApprovedVersionNormalisesTheDeployState — mutant (e): replacing
// EqualFold+TrimSpace on DeployState with an exact `== "live"`. The cost of that
// mutant is not a wrong verdict, it is a wrong CLAIM: the refusal drops "and
// live" for a row that really is serving, which is the exact understatement the
// live flag was added to avoid.
func TestHighestApprovedVersionNormalisesTheDeployState(t *testing.T) {
	for _, ds := range []string{"live", "Live", "LIVE", " live "} {
		rows := []appapi.Submission{
			{ID: "pubreq_ds", BlockID: guardSlug, Version: "0.5.2", Status: "approved",
				DeployState: strptr(ds), SubmittedAt: "2026-08-01T08:00:00Z"},
		}
		peak := highestApprovedVersion(rows, guardSlug)
		if !peak.live {
			t.Errorf("deployState=%q says the row is serving; peak.live must agree", ds)
		}
		_, err := guardRun(t, rows, "0.4.1", false)
		if err == nil {
			t.Fatalf("deployState=%q: 0.4.1 must be refused", ds)
		}
		if !strings.Contains(err.Error(), "0.5.2 is already approved and live") {
			t.Errorf("deployState=%q: the refusal should claim live, got:\n%s", ds, err.Error())
		}
	}
	// Negative control: a NON-live deploy state must still not be claimed live,
	// so the normalisation cannot be widened into "any deployState is live".
	for _, ds := range []string{"failed", "building", "deploying", "livewire"} {
		rows := []appapi.Submission{
			{ID: "pubreq_ds2", BlockID: guardSlug, Version: "0.5.2", Status: "approved",
				DeployState: strptr(ds), SubmittedAt: "2026-08-01T08:00:00Z"},
		}
		if peak := highestApprovedVersion(rows, guardSlug); peak.live {
			t.Errorf("deployState=%q is not live; peak.live must be false", ds)
		}
	}
	// A nil deployState is the third shape and must not panic or claim live.
	rows := []appapi.Submission{
		{ID: "pubreq_nil", BlockID: guardSlug, Version: "0.5.2", Status: "approved", SubmittedAt: "2026-08-01T08:00:00Z"},
	}
	if peak := highestApprovedVersion(rows, guardSlug); peak.live {
		t.Error("a nil deployState must not read as live")
	}
}

// TestVersionGuardOrdersAVPrefixedApprovedVersion pins that a `v`-prefixed
// version is ORDERABLE rather than routed to the unorderable warn-and-proceed
// branch — the behaviour the TrimPrefix in comparableVersion exists for.
//
// 🔴 HONEST SCOPE: this does NOT kill mutant (d) ("drop TrimPrefix from
// comparableVersion") on its own, because parseSemver (update_check.go) trims a
// leading "v" as well — so for every realistic input the two spellings agree and
// (d) is an EQUIVALENT mutant, not a surviving one. What this test does pin is
// the observable contract both sites implement together: remove the prefix
// handling from BOTH and this reddens. That is the claim worth making, and it is
// stated here rather than left as a green line implying more than it proves.
func TestVersionGuardOrdersAVPrefixedApprovedVersion(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_vpfx", BlockID: guardSlug, Version: "v0.5.2", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
	}
	if v, ok := comparableVersion("v0.5.2"); !ok {
		t.Error("v0.5.2 must be comparable")
	} else if plain, _ := comparableVersion("0.5.2"); v.compare(plain) != 0 {
		t.Errorf("v0.5.2 and 0.5.2 must order equal, got compare = %d", v.compare(plain))
	}

	warn, err := guardRun(t, rows, "0.4.1", false)
	if err == nil {
		t.Fatal("0.4.1 under an approved v0.5.2 must be refused — a v-prefixed version is orderable")
	}
	if strings.Contains(warn, "ignoring") {
		t.Errorf("v0.5.2 must not be reported as unorderable, got warning:\n%s", warn)
	}
	// The refusal reproduces the server's raw string verbatim, prefix included.
	if !strings.Contains(err.Error(), "v0.5.2 is already approved and live") {
		t.Errorf("the refusal should quote the server's own spelling, got:\n%s", err.Error())
	}
	// And the local side too: a v-prefixed forward bump is an ordinary submit.
	if _, err := guardRun(t, rows, "v0.6.0", false); err != nil {
		t.Errorf("v0.6.0 is above v0.5.2 and must be allowed, got: %v", err)
	}
}

// --- THE TIE-BREAK AND THE LIVE FLAG: which ROW wins, and what it carries ---

// TestHighestApprovedVersionKeepsTheFirstOfTwoEqualApprovedRows — mutant (b):
// `v.compare(peak.parsed) <= 0` → `< 0`, i.e. a later EQUAL row replaces the
// incumbent instead of losing the tie.
//
// It survived because no fixture had two approved rows at the same version. It
// matters because the two rows can disagree about what is SERVING: the guard's
// message claims "and live" from the winning row, so the tie-break decides
// whether a true statement or a false one is printed. Both list orders are
// asserted, with opposite expected answers, so "first wins" is pinned as a
// direction rather than coincidentally satisfied.
func TestHighestApprovedVersionKeepsTheFirstOfTwoEqualApprovedRows(t *testing.T) {
	live := appapi.Submission{ID: "pubreq_eq_live", BlockID: guardSlug, Version: "0.5.2",
		Status: "approved", DeployState: strptr("live"), SubmittedAt: "2026-08-05T08:00:00Z"}
	// Same ordered version, DIFFERENT raw spelling and deploy state, so the
	// winner is identifiable from either field.
	failed := appapi.Submission{ID: "pubreq_eq_failed", BlockID: guardSlug, Version: "v0.5.2",
		Status: "approved", DeployState: strptr("failed"), SubmittedAt: "2026-08-04T08:00:00Z"}

	liveFirst := highestApprovedVersion([]appapi.Submission{live, failed}, guardSlug)
	if liveFirst.version != "0.5.2" || !liveFirst.live {
		t.Errorf("live-first: peak = %q live=%v, want \"0.5.2\" live=true — a later EQUAL row must lose the "+
			"tie; taking it would swap the winner and print \"approved\" for a row that IS serving",
			liveFirst.version, liveFirst.live)
	}

	failedFirst := highestApprovedVersion([]appapi.Submission{failed, live}, guardSlug)
	if failedFirst.version != "v0.5.2" || failedFirst.live {
		t.Errorf("failed-first: peak = %q live=%v, want \"v0.5.2\" live=false — the SAME tie-break in the "+
			"other order; a later equal row winning here would print \"and live\" for a failed deploy",
			failedFirst.version, failedFirst.live)
	}

	// The answers above are only meaningful if the two rows really tie.
	a, okA := comparableVersion(live.Version)
	b, okB := comparableVersion(failed.Version)
	if !okA || !okB || a.compare(b) != 0 {
		t.Fatalf("fixture broken: %q and %q must order EQUAL for this to test the tie-break",
			live.Version, failed.Version)
	}
}

// TestHighestApprovedVersionLiveFlagBelongsToTheWinningRow — mutant (c):
// `peak.live = …` → `peak.live || …`, which makes the flag STICKY. Once any
// approved row is live the message claims live forever, including for a higher
// approved row whose deploy failed — the precise false claim the field's own
// comment says it exists to prevent.
//
// The kill needs the live row to be seen FIRST, so the sticky value has
// something to survive on; the fixture is ordered for that and says so.
func TestHighestApprovedVersionLiveFlagBelongsToTheWinningRow(t *testing.T) {
	rows := []appapi.Submission{
		// Seen first, live, but LOWER — the value a sticky flag would keep.
		{ID: "pubreq_low_live", BlockID: guardSlug, Version: "0.5.0", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
		// Seen second, HIGHER, and its deploy failed — the actual winner.
		{ID: "pubreq_high_failed", BlockID: guardSlug, Version: "0.6.0", Status: "approved",
			DeployState: strptr("failed"), SubmittedAt: "2026-08-02T08:00:00Z"},
	}
	peak := highestApprovedVersion(rows, guardSlug)
	if peak.version != "0.6.0" {
		t.Fatalf("peak = %q, want 0.6.0", peak.version)
	}
	if peak.live {
		t.Error("the winning row (0.6.0) has deployState=failed — peak.live must describe THAT row, " +
			"not carry over from the lower 0.5.0 that happened to be live")
	}
	_, err := guardRun(t, rows, "0.4.1", false)
	if err == nil {
		t.Fatal("0.4.1 under an approved 0.6.0 must be refused")
	}
	if strings.Contains(err.Error(), "approved and live") {
		t.Errorf("0.6.0's deploy failed, so the refusal must not claim it is live:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "0.6.0 is already approved.") {
		t.Errorf("the refusal should name 0.6.0 as approved-not-live, got:\n%s", err.Error())
	}
}

// --- THE NARROWING ARGUMENT AND THE WARNING'S DESTINATION ---

// TestVersionGuardNarrowsTheListingToThisApp — mutant (f): `lister(ctx, slug)` →
// `lister(ctx, "")`. Every fixture is filtered slug-side afterwards, so the
// VERDICT is unchanged and only the argument can catch it. Dropping the
// narrowing is not cosmetic: the route caps at ListSubmissionsCap rows, so an
// unnarrowed read of a busy account can page this app's approved row off the
// end and silently disable the guard.
func TestVersionGuardNarrowsTheListingToThisApp(t *testing.T) {
	lister, got := listerCapturing(regressionFixture())
	var warn bytes.Buffer
	if err := checkVersionNotRegression(context.Background(), lister, &warn, guardSlug, "0.6.1", false); err != nil {
		t.Fatalf("0.6.1 is above the approved 0.5.2 and must be allowed, got: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("the guard made %d submissions call(s), want exactly 1: %q", len(*got), *got)
	}
	if (*got)[0] != guardSlug {
		t.Errorf("the guard read the submissions route with blockId=%q, want %q — an empty blockId asks "+
			"for EVERY app's rows and can push this app's approved row past the server's row cap",
			(*got)[0], guardSlug)
	}
}

// TestAppSubmitGuardWarningsGoToStderrNotStdout — mutant (g): the warn writer at
// the call site, `cmd.ErrOrStderr()` → `cmd.OutOrStdout()`.
//
// 🔴 THIS IS A MACHINE-CONSUMER CONTRACT, not tidiness. `app submit`'s stdout is
// the parseable channel; a guard advisory landing there corrupts it for anything
// reading the run. The unit tests all pass their own buffer, so the call site's
// choice of writer was never observed by any of them — only an end-to-end run
// with SEPARATE streams can see it.
func TestAppSubmitGuardWarningsGoToStderrNotStdout(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeManifestVersion(t, tmp, guardSlug, "1.6.0")
	// An approved row carrying pre-release metadata is unorderable, so the guard
	// WARNS about it and still compares against the orderable 1.5.0 — a run that
	// both emits a guard warning and succeeds.
	rows := []appapi.Submission{
		{ID: "pubreq_rc", BlockID: guardSlug, Version: "2.0.0-rc.1", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-10T10:00:00Z"},
		{ID: "pubreq_ga", BlockID: guardSlug, Version: "1.5.0", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-05T09:00:00Z"},
	}
	srv, submitted := submitGuardServer(t, rows)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("1.6.0 is above the approved 1.5.0 and must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	// POSITIVE CONTROL: without these the assertions below pass vacuously on a
	// run that never warned at all.
	if !*submitted {
		t.Fatal("the run must reach the submit route, or the streams below prove nothing")
	}
	if !strings.Contains(stdout, "Packaged ") {
		t.Fatalf("the run must have produced its ordinary stdout, got:\n%s", stdout)
	}
	for _, want := range []string{"ignoring 1 approved version(s)", "2.0.0-rc.1"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the guard warning must reach STDERR (missing %q), got stderr:\n%s", want, stderr)
		}
		if strings.Contains(stdout, want) {
			t.Errorf("the guard warning leaked onto STDOUT (%q) — stdout is the machine-readable "+
				"channel for this command, got stdout:\n%s", want, stdout)
		}
	}
}

// --- THE SLUG: normalised, and the mismatch no longer silent (finding 4) ---

// TestHighestApprovedVersionNormalisesTheSlug — the slug used to be the ONE
// field compared byte-for-byte while status and deployState were normalised.
// That asymmetry is worse than it looks: a slug that fails to match lands in the
// "no approved rows" branch, which is the single branch that proceeds SILENTLY
// by design, so a casing difference would switch the whole guard off on the very
// app it exists to protect and print nothing at all.
func TestHighestApprovedVersionNormalisesTheSlug(t *testing.T) {
	for _, blockID := range []string{"custom-generators", "Custom-Generators", "CUSTOM-GENERATORS", " custom-generators", "custom-generators "} {
		rows := []appapi.Submission{
			{ID: "pubreq_slug", BlockID: blockID, Version: "0.5.2", Status: "approved",
				DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
		}
		peak := highestApprovedVersion(rows, guardSlug)
		if !peak.found {
			t.Errorf("blockId=%q must match slug %q — an exact compare here disables the guard silently",
				blockID, guardSlug)
			continue
		}
		if peak.version != "0.5.2" {
			t.Errorf("blockId=%q: peak = %q, want 0.5.2", blockID, peak.version)
		}
	}
	// Negative control, and the reason normalising is safe: a DIFFERENT app is
	// still a different app. Without this the mutation `appapi.SameSlug -> true` passes.
	for _, blockID := range []string{"gen-matrix", "custom-generators-2", "custom", ""} {
		rows := []appapi.Submission{
			{ID: "pubreq_other", BlockID: blockID, Version: "9.9.9", Status: "approved",
				DeployState: strptr("live"), SubmittedAt: "2026-08-13T11:00:00Z"},
		}
		if peak := highestApprovedVersion(rows, guardSlug); peak.found {
			t.Errorf("blockId=%q is not %q and must not answer for it, got peak %q", blockID, guardSlug, peak.version)
		}
	}
}

// TestVersionGuardAnnouncesWhenTheListingHeldNoRowForThisApp is the other half
// of finding 4. Normalising removes the LIKELY slug mismatch; it cannot remove
// the residue (a server that returns a genuinely different identifier on this
// route — which nothing in this CLI pins). So the residue now announces itself
// instead of masquerading as a first submit.
func TestVersionGuardAnnouncesWhenTheListingHeldNoRowForThisApp(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_otherapp", BlockID: "gen-matrix", Version: "9.9.9", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-13T11:00:00Z"},
		{ID: "pubreq_otherapp2", BlockID: "gen-matrix", Version: "9.9.8", Status: "pending",
			SubmittedAt: "2026-08-12T11:00:00Z"},
	}
	warn, err := guardRun(t, rows, "0.1.0", false)
	if err != nil {
		t.Fatalf("an unmatched listing must not hard-fail the submit, got: %v", err)
	}
	if warn == "" {
		t.Fatal("rows came back and NONE was for this app — that is a blockId mismatch, not a first " +
			"submit, and it must not proceed silently: the #412 trap would return with zero signal")
	}
	for _, want := range []string{"2 row(s)", guardSlug, "blockId mismatch"} {
		if !strings.Contains(warn, want) {
			t.Errorf("the announcement should carry %q so the cause is diagnosable, got:\n%s", want, warn)
		}
	}
}

// TestVersionGuardStaysSilentOnAGenuineFirstSubmitWithRows is the negative
// control that keeps the announcement from becoming noise on the happy path: an
// app whose rows ARE all its own, none approved yet, is an ordinary first
// submit and must stay silent. Without this the announcement could be widened to
// "no approved rows" and still look correct.
func TestVersionGuardStaysSilentOnAGenuineFirstSubmitWithRows(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_p", BlockID: guardSlug, Version: "0.1.0", Status: "pending", SubmittedAt: "2026-08-12T09:00:00Z"},
		{ID: "pubreq_r", BlockID: "Custom-Generators", Version: "0.0.9", Status: "rejected", SubmittedAt: "2026-07-20T07:00:00Z"},
	}
	warn, err := guardRun(t, rows, "0.2.0", false)
	if err != nil {
		t.Fatalf("no approved row means nothing to regress, got: %v", err)
	}
	if warn != "" {
		t.Errorf("these rows ARE for this app (one of them only differs in case) — an ordinary "+
			"not-yet-approved app must stay silent, got:\n%s", warn)
	}
}

// --- LIVENESS IS THE SERVER'S ANSWER, NOT deployState's (delta finding 1) ---
//
// 🔴 THE DEFECT THIS PINS WAS INTRODUCED BY THE PREVIOUS FIX ROUND, and it is
// the worst shape available: a POSITIVE false claim that then recommends the
// exact accident the guard exists to prevent.
//
// The server's shared predicate isApprovedAndServing
// (civitai:src/shared/constants/app-block-deploy.constants.ts) returns TRUE for
// an approved row whose deployState is NULL whenever it carries a
// deployUpdatedAt, has no reviewedAt, or was reviewed before the tracking epoch
// — every LEGACY approval that predates deploy-state tracking. shapeRow
// (civitai:src/pages/api/v1/blocks/submissions.ts) emits `liveUrl` from exactly
// that predicate, so `liveUrl != null` IS the server saying "serving", and it is
// the ONLY field on this route that says it for those rows.
//
// A guard reading deployState alone therefore printed, for a row serving at a
// real URL: "That version is approved but not live … pass --allow-downgrade to
// resubmit it" — while `civitai app status` printed `Live at: <that url>` for
// the same row, off the same field. Three failures in one message: it is false,
// it contradicts the sibling command, and its advice inverts to the flag that
// submits under an already-serving version.

// liveByURLOnlyRow is the row shape the whole class turns on: approved, NO
// deployState, and a liveUrl the server only emits when it considers the row to
// be serving.
func liveByURLOnlyRow(version string) appapi.Submission {
	return appapi.Submission{
		ID: "pubreq_legacy_live", BlockID: guardSlug, Version: version, Status: "approved",
		DeployState: nil, // the legacy shape: tracking did not exist when it was approved
		LiveURL:     strptr("https://custom-generators.civit.ai/"),
		SubmittedAt: "2026-08-01T08:00:00Z",
	}
}

// TestHighestApprovedVersionReadsLivenessFromLiveURLNotOnlyDeployState is the
// unit half: peak.live must agree with the server.
func TestHighestApprovedVersionReadsLivenessFromLiveURLNotOnlyDeployState(t *testing.T) {
	peak := highestApprovedVersion([]appapi.Submission{liveByURLOnlyRow("0.5.2")}, guardSlug)
	if !peak.found {
		t.Fatal("the row is approved and must be found")
	}
	if !peak.live {
		t.Error("the server emits liveUrl ONLY for a row its own isApprovedAndServing predicate calls " +
			"serving, so a non-nil liveUrl means live even with deployState=null (the legacy approval " +
			"shape). Reading deployState alone reports a serving app as not live.")
	}
	// Negative control: liveUrl is the SERVER's answer, so its ABSENCE must not
	// be read as live — otherwise the fix widens into "every approved row is
	// live" and the not-live arm becomes unreachable.
	for _, name := range []string{"", "failed", "building", "deploying"} {
		var ds *string
		if name != "" {
			ds = strptr(name)
		}
		rows := []appapi.Submission{{ID: "pubreq_nolive", BlockID: guardSlug, Version: "0.5.2",
			Status: "approved", DeployState: ds, LiveURL: nil, SubmittedAt: "2026-08-01T08:00:00Z"}}
		if peak := highestApprovedVersion(rows, guardSlug); peak.live {
			t.Errorf("deployState=%q (empty = null) with no liveUrl must NOT read as live", name)
		}
	}
	// An EMPTY liveUrl is not a URL. The server never emits one, but reading a
	// blank string as "serving" would make the claim rest on the field being
	// present rather than on what it holds.
	blank := []appapi.Submission{{ID: "pubreq_blank", BlockID: guardSlug, Version: "0.5.2",
		Status: "approved", LiveURL: strptr("  "), SubmittedAt: "2026-08-01T08:00:00Z"}}
	if peak := highestApprovedVersion(blank, guardSlug); peak.live {
		t.Error("an empty liveUrl is not the server saying the row is serving")
	}
}

// TestVersionGuardNeverCallsAServingRowNotLive is the message half — the claim
// the user actually reads, in both refusal directions.
func TestVersionGuardNeverCallsAServingRowNotLive(t *testing.T) {
	rows := []appapi.Submission{liveByURLOnlyRow("0.5.2")}

	for _, tc := range []struct{ name, version, wantMiddle string }{
		{"equal", "0.5.2", "Resubmitting the version that is already live is almost always an accident."},
		{"lower", "0.4.1", "Submitting an older version replaces the newer live deployment on approval."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := guardRun(t, rows, tc.version, false)
			if err == nil {
				t.Fatalf("%s must still be refused against the approved 0.5.2", tc.version)
			}
			msg := err.Error()
			// The false claim itself.
			if strings.Contains(msg, "not live") {
				t.Errorf("the server reports this row as serving (liveUrl is set), so the refusal must "+
					"not tell the author it is not live — `civitai app status` prints `Live at:` for the "+
					"SAME row off the SAME field:\n%s", msg)
			}
			if !strings.Contains(msg, "0.5.2 is already approved and live") {
				t.Errorf("line 1 must claim live for a serving row, got:\n%s", msg)
			}
			if !strings.Contains(msg, tc.wantMiddle) {
				t.Errorf("want middle line %q, got:\n%s", tc.wantMiddle, msg)
			}
			// 🔴 The consequence that makes this deploy-blocking: the not-live arm
			// recommends --allow-downgrade as a RETRY, which submits under a
			// version that is already serving — the #412 accident itself.
			if strings.Contains(msg, "--allow-downgrade to submit this version again") {
				t.Errorf("recommending --allow-downgrade as a retry of a deploy that has not landed is "+
					"wrong here: the deploy DID land, and taking that advice resubmits under a serving "+
					"version — the exact #412 accident:\n%s", msg)
			}
		})
	}
}

// rawSubmitGuardServer is submitGuardServer with the submissions body written as
// LITERAL JSON rather than encoded from appapi.Submission.
//
// 🔴 THAT DIFFERENCE IS THE WHOLE TEST, AND THE FIRST CUT OF IT WAS VACUOUS.
// submitGuardServer marshals the very struct the CLI unmarshals, so a wrong or
// missing `json:` tag is applied symmetrically on both sides and the round trip
// still works — measured: renaming the tag to `live_url` left this test GREEN.
// A fake that encodes with the same code under test can only ever prove the code
// agrees with itself. The body below is spelled the way
// civitai:src/pages/api/v1/blocks/submissions.ts shapeRow spells it.
func rawSubmitGuardServer(t *testing.T, body string) (*httptest.Server, *bool) {
	t.Helper()
	submitted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath) {
			_, _ = io.WriteString(w, body)
			return
		}
		submitted = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"publishRequestId": "pubreq_new", "slug": guardSlug, "version": "x", "status": "pending",
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &submitted
}

// TestAppSubmitNeverCallsAServingRowNotLiveEndToEnd is the seam the two tests
// above structurally CANNOT see: they build appapi.Submission in Go, so the
// `liveUrl` field NAME is never exercised. A wrong or missing tag decodes to
// nil, peak.live goes false, and every unit assertion above still passes while
// the real binary prints the false claim. This drives the actual command over
// HTTP against a body written in the SERVER's spelling.
func TestAppSubmitNeverCallsAServingRowNotLiveEndToEnd(t *testing.T) {
	withStdinTTY(t, false)
	tmp := t.TempDir()
	writeManifestVersion(t, tmp, guardSlug, "0.5.2")
	// The legacy shape, as the route really emits it: approved, deployState
	// null, and a liveUrl the server only writes when isApprovedAndServing.
	srv, submitted := rawSubmitGuardServer(t, `{"submissions":[{
		"id": "pubreq_legacy_live",
		"blockId": "custom-generators",
		"version": "0.5.2",
		"status": "approved",
		"deployState": null,
		"deployUpdatedAt": "2026-07-01T08:00:00Z",
		"submittedAt": "2026-08-01T08:00:00Z",
		"reviewedAt": null,
		"updatedAt": "2026-08-01T08:00:00Z",
		"createdAt": "2026-08-01T08:00:00Z",
		"liveUrl": "https://custom-generators.civit.ai/"
	}]}`)
	withGuardEnv(t, srv)

	stdout, _, err := run(t, "app", "submit", tmp, "--yes")
	if err == nil {
		t.Fatalf("resubmitting the serving 0.5.2 must be refused; stdout:\n%s", stdout)
	}
	if !errors.Is(err, ErrVersionRegression) {
		t.Errorf("the refusal must carry ErrVersionRegression, got %#v", err)
	}
	if *submitted {
		t.Error("the submit route was hit — the guard must refuse before uploading")
	}
	msg := err.Error()
	if !strings.Contains(msg, "0.5.2 is already approved and live") {
		t.Errorf("the row decoded from the wire carries liveUrl, so the refusal must claim live. "+
			"A nil here means the `liveUrl` JSON tag did not decode — invisible to every unit test in "+
			"this file, all of which build the struct directly:\n%s", msg)
	}
	if strings.Contains(msg, "not live") || strings.Contains(msg, "--allow-downgrade to submit this version again") {
		t.Errorf("the server says this row is serving; calling it not live and offering "+
			"--allow-downgrade as a retry is the #412 accident being recommended:\n%s", msg)
	}
}

// --- NO blockId AT ALL: not a mismatch (delta finding 2) ---

// TestVersionGuardWithNoBlockIdSaysSoInsteadOfCryingMismatch. manifest.Load does
// not require blockId and `--skip-validate` waives the schema, so the guard can
// be handed an EMPTY slug. Before this branch existed the empty slug produced an
// UNNARROWED listing read (submissionsURL omits an empty blockId), every row
// then failed appapi.SameSlug, and the mismatch announcement fired with the name
// spliced in blank: "none of them for  — … If  is already published, this is a
// blockId mismatch rather than a first submit." That misdiagnoses "your manifest
// has no blockId" as "your blockId does not match the server's".
func TestVersionGuardWithNoBlockIdSaysSoInsteadOfCryingMismatch(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_otherapp", BlockID: "gen-matrix", Version: "9.9.9", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-13T11:00:00Z"},
	}
	lister, calls := listerFor(rows)
	var warn bytes.Buffer
	if err := checkVersionNotRegression(context.Background(), lister, &warn, "", "0.1.0", false); err != nil {
		t.Fatalf("a manifest with no blockId must not hard-fail the submit, got: %v", err)
	}
	s := warn.String()
	if !strings.Contains(s, "no blockId") {
		t.Errorf("the guard must name the real cause — the manifest declares no blockId — got:\n%s", s)
	}
	if strings.Contains(s, "blockId mismatch") {
		t.Errorf("there is nothing to mismatch: the manifest named no app at all. Calling this a "+
			"mismatch sends the author looking for a slug difference that does not exist:\n%s", s)
	}
	// The empty-splice artifact, asserted directly: the announcement interpolates
	// the slug twice, so a blank one leaves "for  —" and "If  is".
	for _, artifact := range []string{"none of them for  ", "If  is already published"} {
		if strings.Contains(s, artifact) {
			t.Errorf("the warning spliced an empty app name into its own sentence (%q):\n%s", artifact, s)
		}
	}
	// And it must not pay for — or provoke — an unnarrowed listing read. An empty
	// blockId asks the route for EVERY app's rows, which is the very read
	// TestVersionGuardNarrowsTheListingToThisApp exists to prevent.
	if *calls != 0 {
		t.Errorf("the guard made %d submissions call(s) with an empty blockId; there is no app to "+
			"compare against, and an empty blockId is not narrowed server-side", *calls)
	}
}

// --- EVERY ARM STATES ONLY WHAT IT KNOWS (delta finding 7) ---

// TestVersionRegressionArmsClaimOnlyWhatIsKnown is the four-way sweep the
// three-arm switch could not express. The lower-version case used to be ONE arm
// asserting "replaces the newer deployment on approval" whatever peak.live held
// — the same false-claim class the equal arm had just been fixed for, left
// standing one branch over. Against an approved row that is not serving there is
// no deployment of that version to replace.
func TestVersionRegressionArmsClaimOnlyWhatIsKnown(t *testing.T) {
	live := []appapi.Submission{
		{ID: "pubreq_live", BlockID: guardSlug, Version: "0.5.2", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-01T08:00:00Z"},
	}
	notLive := []appapi.Submission{
		{ID: "pubreq_failed", BlockID: guardSlug, Version: "0.5.2", Status: "approved",
			DeployState: strptr("failed"), SubmittedAt: "2026-08-01T08:00:00Z"},
	}
	for _, tc := range []struct {
		name    string
		rows    []appapi.Submission
		version string
		want    []string
		never   []string
	}{
		{
			name: "equal+live", rows: live, version: "0.5.2",
			want:  []string{"0.5.2 is already approved and live", "almost always an accident"},
			never: []string{"not live", "unchanged", "may be behind"},
		},
		{
			name: "equal+not-live", rows: notLive, version: "0.5.2",
			want: []string{"0.5.2 is already approved.", "approved but not live", "has not landed"},
			// "unchanged" claims the bundle; "behind" is false (the repo is AT
			// that version); "never landed" is false while a deploy is running.
			never: []string{"unchanged", "may be behind", "never landed", "accident"},
		},
		{
			name: "lower+live", rows: live, version: "0.4.1",
			want:  []string{"0.5.2 is already approved and live", "replaces the newer live deployment on approval"},
			never: []string{"not live", "accident", "unchanged"},
		},
		{
			name: "lower+not-live", rows: notLive, version: "0.4.1",
			want: []string{"0.5.2 is already approved.", "approved but not live", "deploys code older than the highest approved version"},
			// 🔴 THE FINDING: nothing of 0.5.2 is deployed, so approval cannot
			// replace a deployment of it.
			never: []string{"replaces the newer", "deployment on approval", "accident", "unchanged"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := guardRun(t, tc.rows, tc.version, false)
			if err == nil {
				t.Fatalf("%s must be refused", tc.version)
			}
			msg := err.Error()
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("missing %q in:\n%s", w, msg)
				}
			}
			for _, n := range tc.never {
				if strings.Contains(msg, n) {
					t.Errorf("this arm cannot know %q is true — it is printed for a %s row:\n%s", n, tc.name, msg)
				}
			}
		})
	}
}

// TestVersionRegressionMessageLineOrderIsPinned. README:Troubleshooting tells the
// reader "**The second line** tells you which case you are in", which is a claim
// about ORDER that nothing pinned: swapping `middle` and `tail` in the
// fmt.Errorf argument list survived the entire suite (mutant M17), because every
// other assertion here is a whole-message Contains.
func TestVersionRegressionMessageLineOrderIsPinned(t *testing.T) {
	rowsFor := func(deployState *string) []appapi.Submission {
		return []appapi.Submission{{ID: "pubreq_order", BlockID: guardSlug, Version: "0.5.2",
			Status: "approved", DeployState: deployState, SubmittedAt: "2026-08-01T08:00:00Z"}}
	}
	for _, tc := range []struct {
		name          string
		rows          []appapi.Submission
		version       string
		second, third string
	}{
		{"equal+live", rowsFor(strptr("live")), "0.5.2",
			"Resubmitting the version that is already live is almost always an accident.",
			"Bump the version in your manifest, or pass --allow-downgrade if this is deliberate"},
		{"equal+not-live", rowsFor(strptr("failed")), "0.5.2",
			"That version is approved but not live, so this may be a deliberate resubmit of a deploy that has not landed.",
			"Bump the version in your manifest, or pass --allow-downgrade to submit this version again"},
		{"lower+live", rowsFor(strptr("live")), "0.4.1",
			"Submitting an older version replaces the newer live deployment on approval.",
			"Your repo may be behind what was last released. Pass --allow-downgrade if this is deliberate"},
		{"lower+not-live", rowsFor(strptr("failed")), "0.4.1",
			"That version is approved but not live, so approving an older one deploys code older than the highest approved version.",
			"Your repo may be behind the highest approved version. Pass --allow-downgrade if this is deliberate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := guardRun(t, tc.rows, tc.version, false)
			if err == nil {
				t.Fatalf("%s must be refused", tc.version)
			}
			lines := strings.Split(err.Error(), "\n")
			if len(lines) != 3 {
				t.Fatalf("the refusal must be exactly 3 lines (the README indexes them by position), got %d:\n%s",
					len(lines), err.Error())
			}
			if !strings.HasPrefix(lines[0], "refusing to submit ") {
				t.Errorf("line 1 must be the refusal itself, got %q", lines[0])
			}
			if lines[1] != tc.second {
				t.Errorf("line 2 (the README's \"which case you are in\" line) = %q, want %q", lines[1], tc.second)
			}
			if lines[2] != tc.third {
				t.Errorf("line 3 (the remedy) = %q, want %q", lines[2], tc.third)
			}
		})
	}
}

// TestVersionGuardAnnouncesWithExactlyOneForeignRow is the boundary the two-row
// fixture in TestVersionGuardAnnouncesWhenTheListingHeldNoRowForThisApp cannot
// see: with two rows, mutating `len(subs) > 0` to `len(subs) > 1` survives. ONE
// foreign row is the smallest listing that is a mismatch rather than a first
// submit, and it must announce.
func TestVersionGuardAnnouncesWithExactlyOneForeignRow(t *testing.T) {
	rows := []appapi.Submission{
		{ID: "pubreq_otherapp", BlockID: "gen-matrix", Version: "9.9.9", Status: "approved",
			DeployState: strptr("live"), SubmittedAt: "2026-08-13T11:00:00Z"},
	}
	warn, err := guardRun(t, rows, "0.1.0", false)
	if err != nil {
		t.Fatalf("an unmatched listing must not hard-fail the submit, got: %v", err)
	}
	if warn == "" {
		t.Fatal("ONE row came back and it was not for this app — a `len(subs) > 1` off-by-one is silent " +
			"here and only a single-row fixture can see it")
	}
	for _, want := range []string{"1 row(s)", guardSlug, "blockId mismatch"} {
		if !strings.Contains(warn, want) {
			t.Errorf("the announcement should carry %q, got:\n%s", want, warn)
		}
	}
}

// TestAppSubmitHasTheAllowDowngradeFlag pins the flag's existence and its
// default, so the escape hatch cannot be renamed out from under the docs.
func TestAppSubmitHasTheAllowDowngradeFlag(t *testing.T) {
	f := newAppSubmitCmd().Flags().Lookup("allow-downgrade")
	if f == nil {
		t.Fatal("`app submit` must carry --allow-downgrade")
	}
	if f.DefValue != "false" {
		t.Errorf("--allow-downgrade default = %q, want false (the guard is on by default)", f.DefValue)
	}
}
