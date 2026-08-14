package cmd

// LOCAL-vs-PUBLISHED VERSION DRIFT IN `app status` — issue #412.
//
// A repo can fall BEHIND its own live deployment (five first-party apps were in
// that state when the issue was filed) and nothing said so. `app status` now
// compares the local block.manifest.json against the caller's highest APPROVED
// version and warns when the repo is behind.
//
// 🔴 THE WHOLE RISK OF THIS FEATURE IS A FALSE POSITIVE, so the negative
// controls outnumber the positive one on purpose. "Your repo is behind" tells an
// author to stop and go re-pull released code; saying it when it is not true is
// strictly worse than saying nothing. Every case below that expects SILENCE is
// therefore as load-bearing as the case that expects the warning.
//
// 🔴 AND THE FIXTURES DISAGREE ON THE APPROVED-vs-NEWEST AXIS BY CONSTRUCTION.
// A one-row listing cannot tell "highest approved" from "newest row by
// submittedAt" — it passes either way, which is exactly how #390 shipped green
// with four unpinned row picks. driftRows refuses to build a fixture whose rows
// share an identifying field, and the multi-row cases deliberately place the
// approved row BELOW newer pending/withdrawn rows so a newest-row read names a
// version this suite asserts must not appear.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// driftSlug is the app under test throughout. Named after the issue's own
// example so a reader can line the fixtures up against the report.
const driftSlug = "custom-generators"

// driftWarnCore is the distinguishing core of the warning. Written out BY HAND
// from the intended user-facing sentence, never derived from the implementation
// — a derived expectation agrees with any implementation, including a wrong one.
// A bare "behind" or "⚠" would be satisfiable by unrelated prose, so the phrase
// carries both the direction and the reference it was measured against.
const driftWarnCore = "BEHIND the highest APPROVED version"

// driftWarnLine is the full first line for the issue's own numbers.
const driftWarnLine = "⚠ local block.manifest.json is 0.4.0 — BEHIND the highest APPROVED version of custom-generators, which is 0.5.2."

// driftRow is one submissions row, spelled out field by field.
type driftRow struct {
	id, version, status, deploy, submittedAt string
}

// driftRows renders rows (newest first) as the route's list body, refusing any
// fixture whose rows share an identifying field.
//
// The guard is the anti-vacuity requirement made mechanical: two rows agreeing
// on version, id or submittedAt cannot separate "the highest approved version"
// from "the newest row", so a suite built on one would pass against a reading
// this suite exists to forbid.
func driftRows(t *testing.T, rows ...driftRow) map[string]any {
	t.Helper()
	seen := map[string]map[string]bool{"id": {}, "version": {}, "submittedAt": {}}
	for _, r := range rows {
		for field, v := range map[string]string{"id": r.id, "version": r.version, "submittedAt": r.submittedAt} {
			if seen[field][v] {
				t.Fatalf("two rows carry %s=%q — a shared field cannot tell the highest APPROVED row from the "+
					"NEWEST row, which is exactly how an approved-vs-newest read goes untested (#390/#412)", field, v)
			}
			seen[field][v] = true
		}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var deploy any
		if r.deploy != "" {
			deploy = r.deploy
		}
		var liveURL any
		if r.status == "approved" && r.deploy == "live" {
			liveURL = "https://" + driftSlug + ".civit.ai/"
		}
		out = append(out, map[string]any{
			"id": r.id, "blockId": driftSlug, "version": r.version, "status": r.status,
			"deployState": deploy, "submittedAt": r.submittedAt, "updatedAt": r.submittedAt,
			"createdAt": r.submittedAt, "liveUrl": liveURL,
		})
	}
	return map[string]any{"submissions": out}
}

// driftFixture is the canonical anti-vacuity listing: the approved row is
// NEITHER the newest row nor adjacent to it, and the two rows above it carry
// versions that a newest-row read would quote instead.
//
//	pubreq_07  0.7.0  withdrawn   2026-08-03   <- newest row
//	pubreq_06  0.6.0  pending     2026-08-02
//	pubreq_05  0.5.2  approved    2026-08-01   <- the answer
//	pubreq_02  0.2.0  rejected    2026-07-02
func driftFixture(t *testing.T) map[string]any {
	t.Helper()
	return driftRows(t,
		driftRow{"pubreq_07", "0.7.0", "withdrawn", "", "2026-08-03T10:00:00.000Z"},
		driftRow{"pubreq_06", "0.6.0", "pending", "building", "2026-08-02T10:00:00.000Z"},
		driftRow{"pubreq_05", "0.5.2", "approved", "live", "2026-08-01T10:00:00.000Z"},
		driftRow{"pubreq_02", "0.2.0", "rejected", "failed", "2026-07-02T10:00:00.000Z"},
	)
}

// driftServer serves the submissions route for both spellings the drift path
// uses: `?id=` answers with the single-row envelope (that is how the detail view
// reaches a row without a slug), anything else answers with the list.
//
// It counts requests, which is the reachability control: the drift check makes a
// SECOND call, so "1 request" and "2 requests" are how the tests below tell
// "the check declined" from "the check ran and found nothing".
func driftServer(t *testing.T, body map[string]any, calls *int32, listStatus int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(calls, 1)
		if id := r.URL.Query().Get("id"); id != "" {
			rows, _ := body["submissions"].([]map[string]any)
			for _, row := range rows {
				if row["id"] == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"submission": row})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Submission not found"})
			return
		}
		// listStatus (when set) applies to the SECOND call only — the first is
		// the detail lookup, which must still succeed so the failure under test
		// is the drift listing's, not the command's.
		if listStatus != 0 && n > 1 {
			w.WriteHeader(listStatus)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "boom"})
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeDriftManifest writes a minimal manifest into dir and chdirs there, so the
// command under test runs "inside an app directory" the way an author does.
// An empty version writes the key with an empty string; a blockID of "-" writes
// no blockId key at all.
func writeDriftManifest(t *testing.T, blockID, version string) {
	t.Helper()
	dir := t.TempDir()
	fields := []string{`"name": "Custom Generators"`}
	if blockID != "-" {
		fields = append(fields, fmt.Sprintf("%q: %q", "blockId", blockID))
	}
	fields = append(fields, fmt.Sprintf("%q: %q", "version", version))
	body := "{\n  " + strings.Join(fields, ",\n  ") + "\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	t.Chdir(dir)
}

// setupDriftEnv points the CLI at srv with a token, and pins color OFF so the
// literal assertions below are about the SENTENCE and not about an ANSI escape.
// The config dir is deliberately NOT the app directory: a config write landing
// next to the manifest would make the fixture non-hermetic.
func setupDriftEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", baseURL)
	t.Setenv("NO_COLOR", "1")
}

// assertNoDriftWarning fails if either stream carries the warning. Both streams
// are checked because "we moved it to stdout" must not read as "we suppressed
// it": a negative control that only watches stderr would pass a change that
// merely relocated the sentence.
func assertNoDriftWarning(t *testing.T, what, out, errOut string) {
	t.Helper()
	for _, s := range []struct{ name, body string }{{"stdout", out}, {"stderr", errOut}} {
		if strings.Contains(s.body, driftWarnCore) {
			t.Errorf("%s: a drift warning was printed on %s, but this case must stay SILENT — "+
				"a false 'your repo is behind' sends an author to re-pull released code for no reason.\n%s",
				what, s.name, s.body)
		}
	}
}

// ---------------------------------------------------------------------------
// Positive control
// ---------------------------------------------------------------------------

// TestAppStatusWarnsWhenLocalManifestIsBehind is the issue's own case, with the
// issue's own numbers: repo 0.4.0, highest approved 0.5.2.
//
// It asserts the LITERAL sentence, not merely that something was printed, and it
// asserts that neither of the two newer-but-unapproved versions is named — that
// pair is what separates "highest approved" from "newest row".
func TestAppStatusWarnsWhenLocalManifestIsBehind(t *testing.T) {
	var calls int32
	srv := driftServer(t, driftFixture(t), &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	out, errOut, err := run(t, "app", "status", driftSlug)
	// 🔴 EXIT CODE UNCHANGED. The drift line is a warning, not a refusal; a
	// non-nil error here would mean `app status` started failing on a repo that
	// is merely out of date, which would break every script that runs it.
	if err != nil {
		t.Fatalf("the drift warning must not change the exit status of `app status`: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("the drift check made %d request(s), want 2 (the detail lookup + the listing it compares against) — "+
			"nothing below is about the comparison if the listing was never fetched", n)
	}
	if !strings.Contains(errOut, driftWarnLine) {
		t.Errorf("the drift warning is missing or reworded.\nwant line: %s\ngot stderr:\n%s", driftWarnLine, errOut)
	}
	for _, want := range []string{
		"An approved version is what gets deployed, so submitting from this repo would replace newer code on approval.",
		"Sync the released code (civitai app pull --app custom-generators) or raise the local version above 0.5.2 before civitai app submit.",
	} {
		if !strings.Contains(errOut, want) {
			t.Errorf("the warning must name the remedy — missing %q; stderr:\n%s", want, errOut)
		}
	}
	// The approved-vs-newest discriminator. 0.7.0 is the newest row and 0.6.0
	// the newest non-terminal one; naming either would mean the check compared
	// against something nobody is running.
	for _, bad := range []string{"0.7.0", "0.6.0"} {
		if strings.Contains(errOut, "which is "+bad) {
			t.Errorf("the warning compared against %s — a %s row, not the highest APPROVED version. "+
				"The newest row and the highest approved version are different questions (#390).\nstderr:\n%s",
				bad, map[string]string{"0.7.0": "withdrawn", "0.6.0": "pending"}[bad], errOut)
		}
	}
	// The detail table itself is untouched: it still reports the NEWEST row,
	// which is what `app status <slug>` has always shown. The warning names its
	// own reference precisely because the two numbers legitimately differ.
	if v := detailField(t, out, "Version"); v != "0.7.0" {
		t.Errorf("the detail view reports Version %q, want 0.7.0 (the newest row) — the drift check must not "+
			"change which row the detail view prints", v)
	}
	if strings.Contains(out, driftWarnCore) {
		t.Errorf("the warning belongs on stderr, so `--json` and piped stdout stay clean; stdout:\n%s", out)
	}
}

// TestAppStatusDriftWarnsOnTheIDPathAndKeepsJSONPure — the `--id` spelling
// reaches a row without naming a slug, so the drift check has to take the slug
// off the ROW. And `--json` is the scripted rendering: the payload must stay
// parseable while the human still sees the warning.
func TestAppStatusDriftWarnsOnTheIDPathAndKeepsJSONPure(t *testing.T) {
	var calls int32
	srv := driftServer(t, driftFixture(t), &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	out, errOut, err := run(t, "app", "status", "--id", "pubreq_07", "--json")
	if err != nil {
		t.Fatalf("app status --id --json: %v", err)
	}
	if !strings.Contains(errOut, driftWarnLine) {
		t.Errorf("the --id path must warn too — the trap is the repo, not the spelling of the lookup.\nstderr:\n%s", errOut)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("--json stdout must stay parseable JSON (%v):\n%s", err, out)
	}
	if parsed["blockId"] != driftSlug {
		t.Errorf("--json payload lost its shape: %s", out)
	}
	if strings.Contains(out, "⚠") {
		t.Errorf("--json stdout must carry no prose:\n%s", out)
	}
}

// TestAppStatusDriftUsesHighestApprovedNotNewestApproved pins the remaining
// approved-vs-newest degree of freedom: filtering to approved rows is not enough
// if the pick then takes the NEWEST of them. A rollback approval (a lower
// version approved later) is the state where those two answers differ.
//
// The chosen answer is the HIGHEST, matching the reference `app submit`'s
// monotonic guard refuses against — the two commands must quote the same number
// or they contradict each other on the same repo.
func TestAppStatusDriftUsesHighestApprovedNotNewestApproved(t *testing.T) {
	var calls int32
	body := driftRows(t,
		driftRow{"pubreq_09", "0.3.1", "approved", "live", "2026-08-09T10:00:00.000Z"}, // newest approved, LOWER
		driftRow{"pubreq_05", "0.5.2", "approved", "live", "2026-08-01T10:00:00.000Z"}, // highest approved
	)
	srv := driftServer(t, body, &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	_, errOut, err := run(t, "app", "status", driftSlug)
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(errOut, "which is 0.5.2.") {
		t.Errorf("want the HIGHEST approved version (0.5.2); stderr:\n%s", errOut)
	}
	if strings.Contains(errOut, "which is 0.3.1.") {
		t.Errorf("the check took the NEWEST approved row (0.3.1) rather than the highest. "+
			"`app submit` refuses against the highest approved version, so quoting a different one here "+
			"makes the two commands disagree about the same repo.\nstderr:\n%s", errOut)
	}
}

// ---------------------------------------------------------------------------
// Semver ordering — the mutation a string compare would survive
// ---------------------------------------------------------------------------

// TestAppStatusDriftOrdersBySemverNotByString runs BOTH directions across the
// 9/10 boundary. A `local < published` string compare answers wrongly on each,
// in opposite directions, so no constant verdict can pass both.
func TestAppStatusDriftOrdersBySemverNotByString(t *testing.T) {
	for _, tc := range []struct {
		name             string
		local, published string
		wantWarn         bool
		why              string
	}{
		{
			name: "0.10.0 local over 0.9.0 published", local: "0.10.0", published: "0.9.0", wantWarn: false,
			why: `"0.10.0" < "0.9.0" as STRINGS, so a string compare calls a repo that is a minor version AHEAD "behind"`,
		},
		{
			name: "0.9.0 local under 0.10.0 published", local: "0.9.0", published: "0.10.0", wantWarn: true,
			why: `"0.9.0" > "0.10.0" as STRINGS, so a string compare stays SILENT on a repo that really is behind`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			body := driftRows(t,
				driftRow{"pubreq_b", tc.published, "approved", "live", "2026-08-05T10:00:00.000Z"},
				driftRow{"pubreq_a", "0.1.0", "approved", "building", "2026-07-05T10:00:00.000Z"},
			)
			srv := driftServer(t, body, &calls, 0)
			writeDriftManifest(t, driftSlug, tc.local)
			setupDriftEnv(t, srv.URL)

			_, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			if n := atomic.LoadInt32(&calls); n != 2 {
				t.Fatalf("the listing was not fetched (%d request(s)) — the comparison never happened", n)
			}
			got := strings.Contains(errOut, driftWarnCore)
			if got != tc.wantWarn {
				t.Errorf("warn=%v, want %v. %s.\nstderr:\n%s", got, tc.wantWarn, tc.why, errOut)
			}
			if tc.wantWarn && !strings.Contains(errOut, "which is "+tc.published+".") {
				t.Errorf("the warning must quote the published version %s; stderr:\n%s", tc.published, errOut)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Negative controls — every one of these must print NOTHING
// ---------------------------------------------------------------------------

// TestAppStatusDriftSilentWhenAheadOrEqual: being ahead is the normal state of a
// repo about to release, and being equal is the healthy state right after one.
// Warning on either would make the line noise, and noise is how the BEHIND case
// gets ignored.
func TestAppStatusDriftSilentWhenAheadOrEqual(t *testing.T) {
	for _, local := range []string{"0.5.3", "1.0.0", "0.5.2"} {
		t.Run(local, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			writeDriftManifest(t, driftSlug, local)
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			// REACHABILITY: the silence has to be a DECISION, not an early
			// return. Without this the case would also pass against a drift
			// check that was deleted outright.
			if n := atomic.LoadInt32(&calls); n != 2 {
				t.Fatalf("the drift check made %d request(s), want 2 — this case is only meaningful if the "+
					"comparison actually ran and chose to stay quiet", n)
			}
			assertNoDriftWarning(t, "local "+local+" vs approved 0.5.2", out, errOut)
		})
	}
}

// TestAppStatusDriftSilentWithoutALocalManifest — `app status <slug>` from
// anywhere that is not an app directory is the ordinary way this command is run.
// It must say nothing AND cost nothing: the manifest gate is checked before the
// network, so the request count must not move.
func TestAppStatusDriftSilentWithoutALocalManifest(t *testing.T) {
	var calls int32
	srv := driftServer(t, driftFixture(t), &calls, 0)
	t.Chdir(t.TempDir()) // a real directory with no manifest in it
	setupDriftEnv(t, srv.URL)

	out, errOut, err := run(t, "app", "status", driftSlug)
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	assertNoDriftWarning(t, "no local manifest", out, errOut)
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d request(s) were made, want 1. With no local manifest there is nothing to compare, so the "+
			"drift listing must not be fetched at all — otherwise every `app status <slug>` pays for a "+
			"comparison it can never make", n)
	}
}

// TestAppStatusDriftSilentOnAnUnreadableManifest — a directory that HAS a
// manifest the CLI cannot parse is the case most likely to produce a confident
// wrong answer, because a partial read yields a plausible-looking version.
func TestAppStatusDriftSilentOnAnUnreadableManifest(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"malformed JSON", `{"blockId": "custom-generators", "version": `},
		{"not an object", `["custom-generators", "0.4.0"]`},
		{"empty file", ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(tc.body), 0o600); err != nil {
				t.Fatalf("writing manifest: %v", err)
			}
			t.Chdir(dir)
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("an unreadable manifest must not fail `app status`: %v", err)
			}
			assertNoDriftWarning(t, tc.name, out, errOut)
		})
	}
}

// TestAppStatusDriftSilentOnUnparseableVersions — a version this CLI cannot
// order is not evidence of anything. Note the LOCAL half in particular:
// compareVersions treats an unparseable "current" as OLDER (so `civitai version`
// still surfaces an update), and inheriting that default here would turn a typo
// in a manifest into a confident "your repo is BEHIND".
func TestAppStatusDriftSilentOnUnparseableVersions(t *testing.T) {
	for _, tc := range []struct{ name, local, published string }{
		{"local is not semver", "dev", "0.5.2"},
		{"local is empty", "", "0.5.2"},
		{"published is not semver", "0.4.0", "nightly"},
		{"both unparseable", "dev", "nightly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			body := driftRows(t,
				driftRow{"pubreq_p", tc.published, "approved", "live", "2026-08-05T10:00:00.000Z"},
			)
			srv := driftServer(t, body, &calls, 0)
			writeDriftManifest(t, driftSlug, tc.local)
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			assertNoDriftWarning(t, tc.name, out, errOut)
		})
	}
}

// TestAppStatusDriftPreReleaseComparesByTheNumericTriple documents an INHERITED
// limitation rather than asserting a design of its own. parseSemver (shared with
// the self-update check, and deliberately not modified here) drops any
// `-prerelease`/`+build` suffix, so comparison is over the numeric triple only.
//
// The consequence is stated in both directions because only one of them is safe:
//
//   - a suffixed local version BELOW the published one still warns, and the
//     message quotes the RAW string the manifest holds — nothing is invented;
//   - a `-rc` of the published version reads as EQUAL and stays silent. That is
//     the conservative direction (it can under-report, never fabricate a
//     BEHIND), and it is the reason this is recorded as a limitation instead of
//     being papered over with a stricter local parser.
func TestAppStatusDriftPreReleaseComparesByTheNumericTriple(t *testing.T) {
	for _, tc := range []struct {
		name, local string
		wantWarn    bool
	}{
		{"a git-describe below the published triple still warns", "0.4.0-3-gabc123", true},
		{"an rc of the published version reads as equal and is silent", "0.5.2-rc1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			writeDriftManifest(t, driftSlug, tc.local)
			setupDriftEnv(t, srv.URL)

			_, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			if got := strings.Contains(errOut, driftWarnCore); got != tc.wantWarn {
				t.Errorf("warn=%v, want %v for local %q vs approved 0.5.2.\nstderr:\n%s",
					got, tc.wantWarn, tc.local, errOut)
			}
			if tc.wantWarn && !strings.Contains(errOut, "local block.manifest.json is "+tc.local+" —") {
				t.Errorf("the warning must quote the RAW local version %q, not the triple it was reduced to "+
					"for comparison — quoting a value the manifest does not contain sends the author looking "+
					"for a string that is not there.\nstderr:\n%s", tc.local, errOut)
			}
		})
	}
}

// TestAppStatusDriftSilentWhenNothingIsApproved — an app whose every submission
// is pending/rejected/withdrawn has no published version to be behind. The
// versions on those rows are HIGHER than the local one, so a check that skipped
// the status filter would warn here.
func TestAppStatusDriftSilentWhenNothingIsApproved(t *testing.T) {
	var calls int32
	body := driftRows(t,
		driftRow{"pubreq_07", "0.7.0", "withdrawn", "", "2026-08-03T10:00:00.000Z"},
		driftRow{"pubreq_06", "0.6.0", "pending", "building", "2026-08-02T10:00:00.000Z"},
		driftRow{"pubreq_03", "0.3.0", "rejected", "failed", "2026-07-03T10:00:00.000Z"},
	)
	srv := driftServer(t, body, &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	out, errOut, err := run(t, "app", "status", driftSlug)
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("the listing was not fetched (%d request(s)) — the status filter was never exercised", n)
	}
	assertNoDriftWarning(t, "nothing approved", out, errOut)
}

// TestAppStatusDriftSilentWhenTheListingIsEmpty — the `--id` path can reach a
// row while a `?blockId=` listing answers with nothing (a narrowing the server
// applied differently, or a row that has since moved). No rows means no
// reference version; it must not become "behind".
func TestAppStatusDriftSilentWhenTheListingIsEmpty(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Query().Get("id") != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"submission": map[string]any{
				"id": "pubreq_07", "blockId": driftSlug, "version": "0.7.0", "status": "approved",
				"deployState": "live", "submittedAt": "2026-08-03T10:00:00.000Z", "liveUrl": nil,
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
	}))
	defer srv.Close()
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	out, errOut, err := run(t, "app", "status", "--id", "pubreq_07")
	if err != nil {
		t.Fatalf("app status --id: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("the listing was not fetched (%d request(s)) — the empty-list branch was never reached", n)
	}
	assertNoDriftWarning(t, "empty listing", out, errOut)
}

// TestAppStatusDriftSilentForADifferentApp — standing in app A's directory while
// asking about app B. Comparing the two would be a fabricated regression, and it
// is the shape a slug-blind check produces on a perfectly ordinary invocation.
func TestAppStatusDriftSilentForADifferentApp(t *testing.T) {
	for _, blockID := range []string{"some-other-app", "-"} { // "-" writes NO blockId key
		t.Run(blockID, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			writeDriftManifest(t, blockID, "0.4.0")
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			assertNoDriftWarning(t, "manifest blockId="+blockID, out, errOut)
			if n := atomic.LoadInt32(&calls); n != 1 {
				t.Errorf("%d request(s), want 1 — a manifest for a DIFFERENT app is decided offline, "+
					"before any listing is fetched", n)
			}
		})
	}
}

// TestAppStatusDriftSilentWhenTheListingFails — the drift check's own request
// can 403 (the route is invite-gated), 500, or time out. None of that is
// evidence about the repo, and none of it may break a command that has already
// produced its answer.
func TestAppStatusDriftSilentWhenTheListingFails(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, code)
			writeDriftManifest(t, driftSlug, "0.4.0")
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("a failed drift listing must not fail `app status` (the detail view already "+
					"answered successfully): %v", err)
			}
			assertNoDriftWarning(t, fmt.Sprintf("listing returned %d", code), out, errOut)
			if !strings.Contains(out, driftSlug) {
				t.Errorf("the detail view must still render in full:\n%s", out)
			}
		})
	}
}

// TestAppStatusDriftSilentOnTheListView — the bare listing is many apps at once
// and has no single "the version" line, so the drift line is deliberately scoped
// to the detail view. Pinned so the scope is a recorded decision rather than an
// accident someone "fixes" without noticing the list view has no anchor for it.
func TestAppStatusDriftSilentOnTheListView(t *testing.T) {
	var calls int32
	srv := driftServer(t, driftFixture(t), &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	out, errOut, err := run(t, "app", "status")
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	assertNoDriftWarning(t, "bare list view", out, errOut)
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d request(s), want 1 — the list view must not acquire a second call", n)
	}
}

// ---------------------------------------------------------------------------
// Unit cases on the two pure halves
// ---------------------------------------------------------------------------

// subs is a terse builder for highestApprovedVersion's input.
func driftSubs(rows ...[3]string) []appapi.Submission {
	out := make([]appapi.Submission, 0, len(rows))
	for _, r := range rows {
		out = append(out, appapi.Submission{BlockID: r[0], Version: r[1], Status: r[2]})
	}
	return out
}

func TestHighestApprovedVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []appapi.Submission
		want    string
		wantOK  bool
		because string
	}{
		{
			name: "picks the highest approved, not the first or last row",
			in: driftSubs([3]string{driftSlug, "0.7.0", "withdrawn"}, [3]string{driftSlug, "0.6.0", "pending"},
				[3]string{driftSlug, "0.5.2", "approved"}, [3]string{driftSlug, "0.2.0", "rejected"}),
			want: "0.5.2", wantOK: true,
		},
		{
			name: "semver, not string, ordering across the 9/10 boundary",
			in:   driftSubs([3]string{driftSlug, "0.9.0", "approved"}, [3]string{driftSlug, "0.10.0", "approved"}),
			want: "0.10.0", wantOK: true,
			because: `"0.10.0" < "0.9.0" as strings`,
		},
		{
			name:   "another app's rows never contribute",
			in:     driftSubs([3]string{"other-app", "9.9.9", "approved"}, [3]string{driftSlug, "0.5.2", "approved"}),
			want:   "0.5.2",
			wantOK: true,
			because: "a server that ignored the ?blockId= narrowing would otherwise make someone else's " +
				"version the number this CLI quotes",
		},
		{
			name:    "an unorderable approved version is skipped, not quoted",
			in:      driftSubs([3]string{driftSlug, "nightly", "approved"}, [3]string{driftSlug, "0.5.2", "approved"}),
			want:    "0.5.2",
			wantOK:  true,
			because: "a value we cannot order must never become the reference a warning states",
		},
		{
			name:   "no approved rows at all",
			in:     driftSubs([3]string{driftSlug, "0.7.0", "pending"}, [3]string{driftSlug, "0.6.0", "withdrawn"}),
			want:   "",
			wantOK: false,
		},
		{
			name:   "only an unorderable approved row",
			in:     driftSubs([3]string{driftSlug, "nightly", "approved"}),
			want:   "",
			wantOK: false,
		},
		{name: "empty listing", in: nil, want: "", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := highestApprovedVersion(tc.in, driftSlug)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("highestApprovedVersion = (%q, %v), want (%q, %v). %s",
					got, ok, tc.want, tc.wantOK, tc.because)
			}
		})
	}
}

// TestHighestApprovedVersionIsOrderIndependent is the property that lets this
// read sit in a file whose every OTHER read of this route depends on the
// server's newest-first ordering (see newest_row_pick_test.go). A maximum over a
// filtered set has no ends to get wrong — asserted here over every permutation
// of a four-row fixture rather than left as a claim in a comment.
func TestHighestApprovedVersionIsOrderIndependent(t *testing.T) {
	rows := driftSubs(
		[3]string{driftSlug, "0.7.0", "withdrawn"},
		[3]string{driftSlug, "0.6.0", "pending"},
		[3]string{driftSlug, "0.5.2", "approved"},
		[3]string{driftSlug, "0.3.1", "approved"},
	)
	perms := 0
	var walk func(cur []appapi.Submission, rest []appapi.Submission)
	walk = func(cur, rest []appapi.Submission) {
		if len(rest) == 0 {
			perms++
			got, ok := highestApprovedVersion(cur, driftSlug)
			if got != "0.5.2" || !ok {
				t.Fatalf("permutation %v gave (%q, %v), want (0.5.2, true) — the pick depends on row order",
					versionsOf(cur), got, ok)
			}
			return
		}
		for i := range rest {
			next := make([]appapi.Submission, 0, len(rest)-1)
			next = append(next, rest[:i]...)
			next = append(next, rest[i+1:]...)
			walk(append(append([]appapi.Submission{}, cur...), rest[i]), next)
		}
	}
	walk(nil, rows)
	if perms != 24 {
		t.Fatalf("walked %d permutations, want 24 — the positive control on this test's own enumeration", perms)
	}
}

func versionsOf(ss []appapi.Submission) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Version)
	}
	return out
}

// TestVersionDriftWarningIsThreeWay pins the renderer directly: only BEHIND
// speaks. Equality is called out because it is the one people reach for — the
// companion `app submit` guard DOES refuse an equal resubmit, and copying that
// rule into a status line would print a warning on every healthy repo the day
// after a release.
func TestVersionDriftWarningIsThreeWay(t *testing.T) {
	var sink strings.Builder
	for _, tc := range []struct {
		local, published string
		wantWarn         bool
	}{
		{"0.4.0", "0.5.2", true},
		{"0.9.0", "0.10.0", true},
		{"0.5.2", "0.5.2", false},
		{"0.5.3", "0.5.2", false},
		{"0.10.0", "0.9.0", false},
		{"1.0.0", "0.5.2", false},
	} {
		got := versionDriftWarning(&sink, driftSlug, tc.local, tc.published)
		if (got != "") != tc.wantWarn {
			t.Errorf("versionDriftWarning(%s, %s) = %q, want warn=%v", tc.local, tc.published, got, tc.wantWarn)
		}
		if tc.wantWarn {
			want := fmt.Sprintf("local block.manifest.json is %s — BEHIND the highest APPROVED version of %s, which is %s.",
				tc.local, driftSlug, tc.published)
			if !strings.Contains(got, want) {
				t.Errorf("want the sentence %q, got:\n%s", want, got)
			}
		}
	}
}

// TestWarnLocalVersionDriftIgnoresAnErroredListing drives the orchestrator
// directly with a lister that returns rows AND an error.
//
// 🔴 IT EXISTS BECAUSE A MUTATION SURVIVED. Deleting the `if err != nil` gate
// left the whole behavioural suite green: today ListSubmissions returns
// `nil, err` on every failure path, so the downstream "no approved rows" branch
// happens to absorb the error and the guard never gets to matter. A guard whose
// only killing input is one the current API cannot produce is untested by the
// end-to-end cases, so it is pinned at the seam instead — a listing that errored
// is not evidence about the repo, however many rows came back with it.
func TestWarnLocalVersionDriftIgnoresAnErroredListing(t *testing.T) {
	writeDriftManifest(t, driftSlug, "0.4.0")
	rows := []appapi.Submission{{BlockID: driftSlug, Version: "0.5.2", Status: "approved"}}
	sub := &appapi.Submission{BlockID: driftSlug, Version: "0.7.0", Status: "withdrawn"}

	// Positive control FIRST: the identical call with a nil error must warn, or
	// the silence below would prove nothing about the error gate.
	var ok strings.Builder
	warnLocalVersionDrift(context.Background(),
		func(context.Context, string) ([]appapi.Submission, error) { return rows, nil },
		&ok, sub)
	if !strings.Contains(ok.String(), driftWarnCore) {
		t.Fatalf("the control arm did not warn, so this test cannot attribute anything to the error gate:\n%s", ok.String())
	}

	var got strings.Builder
	warnLocalVersionDrift(context.Background(),
		func(context.Context, string) ([]appapi.Submission, error) {
			return rows, errors.New("rate limited (429)")
		},
		&got, sub)
	if got.Len() != 0 {
		t.Errorf("a listing that ERRORED was read anyway. A partial answer is not evidence about the repo — "+
			"the rows that came back with an error may be a truncated or cached view, and quoting one as "+
			"'your highest approved version' states a fact the call did not establish.\ngot:\n%s", got.String())
	}
}

// TestAppStatusHelpDocumentsTheDriftWarning — a warning nobody expects reads as
// a bug, so the behaviour is documented where the flags are.
func TestAppStatusHelpDocumentsTheDriftWarning(t *testing.T) {
	out, _, err := run(t, "app", "status", "--help")
	if err != nil {
		t.Fatalf("app status --help: %v", err)
	}
	for _, want := range []string{"block.manifest.json", "highest APPROVED version"} {
		if !strings.Contains(out, want) {
			t.Errorf("app status --help does not mention %q:\n%s", want, out)
		}
	}
}
