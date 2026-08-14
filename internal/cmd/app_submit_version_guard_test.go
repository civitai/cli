package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		"Submitting an older version replaces the newer deployment on approval.",
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
