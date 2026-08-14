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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	// 🔴 ONE request, not two. The `?blockId=` detail lookup already returns the
	// app's whole narrowed listing, so the drift check reads the rows it was
	// handed instead of re-issuing the byte-identical GET. The count is asserted
	// EXACTLY (not `>= 1`) because both directions are defects: 2 is the
	// duplicate request this PR removed, and 0 would mean the detail lookup
	// itself stopped happening.
	//
	// This case's reachability control is therefore no longer the request count
	// but the warning itself, asserted immediately below: the comparison cannot
	// have produced that sentence without running. The SILENT cases carry their
	// own note about what replaced their control.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("the drift check made %d request(s), want 1 — the detail lookup's own response IS the listing, "+
			"so comparing against it must cost no second round trip", n)
	}
	if !strings.Contains(errOut, driftWarnLine) {
		t.Errorf("the drift warning is missing or reworded.\nwant line: %s\ngot stderr:\n%s", driftWarnLine, errOut)
	}
	for _, want := range []string{
		"An approved version is what gets deployed, so submitting from this repo would replace newer code on approval.",
		// 🔴 `pull .` — see TestDriftRemedyCommandIsRunnableFromTheWarningsOwnDirectory
		// for why the dir argument is the difference between syncing THIS
		// checkout and cloning a second copy inside it.
		"Sync the released code (civitai app pull . --app custom-generators) or raise the local version above 0.5.2 before civitai app submit.",
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
	// 🔴 TWO here, and that is correct rather than a leftover. `?id=` answers
	// with a single-row envelope and no listing at all, so this is the one path
	// where the drift check has no rows to reuse and must ask. The slug path
	// asserts ONE for the same reason — the two counts are the contract.
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("the --id path made %d request(s), want 2 (the id lookup, then the listing it has no other way "+
			"to see) — a reuse optimisation that also silenced this path would break the warning here", n)
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
			if n := atomic.LoadInt32(&calls); n != 1 {
				t.Fatalf("%d request(s), want 1 — the detail lookup's rows are the listing; a second GET is the "+
					"duplicate this PR removed", n)
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
			// 🔴 REACHABILITY MOVED, AND THIS COMMENT IS THE RECORD OF IT.
			// This used to read `want 2`: the drift check's extra request was
			// what proved the comparison ran, so a deleted check failed here.
			// With the rows reused there is no second request to count, and
			// the count can no longer tell "compared, then chose silence" from
			// "never ran".
			//
			// What carries the reachability now is that the SAME fixture, with
			// a local version BELOW the approved one, is the positive control
			// in TestAppStatusWarnsWhenLocalManifestIsBehind — the path is
			// demonstrably live there and only the RELATION differs here, so
			// inverting the comparison (M1) still dies.
			if n := atomic.LoadInt32(&calls); n != 1 {
				t.Fatalf("the drift check made %d request(s), want 1 — the rows come from the detail lookup", n)
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
	// On the SLUG path the count can no longer distinguish "the manifest gate
	// declined" from "the check ran and found nothing" — both are 1 now that the
	// rows are reused. The manifest-before-network property is pinned where it
	// is still observable: TestAppStatusDriftManifestGateRunsBeforeTheNetwork
	// drives the `--id` path, where declining really does save a request.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d request(s) were made, want 1 — `app status <slug>` makes exactly one request, with or "+
			"without a local manifest", n)
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
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("%d request(s), want 1 — the rows come from the detail lookup", n)
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
				t.Errorf("%d request(s), want 1 — the detail lookup and nothing else", n)
			}
		})
	}
}

// TestAppStatusDriftSilentWhenTheListingFails — the drift check's own request
// can 403 (the route is invite-gated), 500, or time out. None of that is
// evidence about the repo, and none of it may break a command that has already
// produced its answer.
//
// 🔴 IT DRIVES THE `--id` PATH ON PURPOSE, and that is a consequence of the
// row-reuse change: on the slug path the drift check no longer issues a request
// of its own, so there is no longer a second call for the server to fail. Left
// on the slug path this case would have kept passing while testing NOTHING —
// the listing it "failed" would never have been requested. The `--id` path is
// where a failing drift listing is still reachable, so that is where the
// tolerance is pinned. The request count below is what makes that non-vacuous.
func TestAppStatusDriftSilentWhenTheListingFails(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, code)
			writeDriftManifest(t, driftSlug, "0.4.0")
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", "--id", "pubreq_07")
			if err != nil {
				t.Fatalf("a failed drift listing must not fail `app status` (the detail view already "+
					"answered successfully): %v", err)
			}
			if n := atomic.LoadInt32(&calls); n != 2 {
				t.Fatalf("%d request(s), want 2 — the failing listing must actually have been REQUESTED, or "+
					"this case asserts silence about a call that never happened", n)
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

// ---------------------------------------------------------------------------
// The remedy the warning prints must be the command a reader can RUN — #413
// audit finding 1
// ---------------------------------------------------------------------------

// remedyFromWarning lifts the parenthesised command out of the rendered warning.
//
// It reads the command back out of the USER-FACING sentence rather than calling
// driftRemedyCommand, so the test covers the whole path a reader takes: a
// remedy that is correct in a helper but interpolated into the wrong sentence
// still fails here.
func remedyFromWarning(t *testing.T, msg string) string {
	t.Helper()
	const lead = "Sync the released code ("
	i := strings.Index(msg, lead)
	if i < 0 {
		t.Fatalf("the warning no longer offers a remedy in the expected shape %q:\n%s", lead+"…)", msg)
	}
	rest := msg[i+len(lead):]
	j := strings.Index(rest, ")")
	if j < 0 {
		t.Fatalf("the remedy is not closed by a %q:\n%s", ")", msg)
	}
	return strings.TrimSpace(rest[:j])
}

// parsedRemedy is what the real command tree made of a remedy string.
type parsedRemedy struct {
	path       string   // e.g. "civitai app pull"
	positional []string // args left after flag parsing
	app        string   // --app
	argsErr    error    // the command's own Args validator on the positionals
}

// parseAsCommand runs a candidate command line through the REAL cobra tree.
//
// 🔴 THIS IS THE POINT OF THE TEST. A `strings.Contains(errOut, "civitai app
// pull …")` assertion is satisfied by any string at all, including one this CLI
// would reject or — worse — accept while doing something else. Resolving the
// line against the actual command tree is what makes "runnable" a measured
// property instead of a spelling.
func parseAsCommand(t *testing.T, line string) parsedRemedy {
	t.Helper()
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "civitai" {
		t.Fatalf("a remedy printed to a user must be a complete command line starting with the binary name; got %q", line)
	}
	root := NewRootCmd()
	c, rest, err := root.Find(fields[1:])
	if err != nil {
		t.Fatalf("the remedy %q does not resolve to a command in this CLI: %v", line, err)
	}
	if err := c.ParseFlags(rest); err != nil {
		t.Fatalf("the remedy %q does not parse against %s's flags: %v", line, c.CommandPath(), err)
	}
	app := ""
	if f := c.Flags().Lookup("app"); f != nil {
		app = f.Value.String()
	}
	pos := c.Flags().Args()
	return parsedRemedy{path: c.CommandPath(), positional: pos, app: app, argsErr: c.ValidateArgs(pos)}
}

// TestDriftRemedyCommandIsRunnableFromTheWarningsOwnDirectory is #413 audit
// finding 1.
//
// 🔴 THE WARNING CAN ONLY EVER PRINT FROM INSIDE THE APP CHECKOUT — the drift
// check reads the manifest at driftManifestDir ("."), so cwd IS the repo that is
// behind. That makes the remedy's directory argument load-bearing: `civitai app
// pull --app <slug>` with no [dir] defaults its target to ./<slug>, so run
// verbatim from where this line printed it clones a SECOND copy of the app
// nested inside the user's repo and leaves the actual checkout exactly as behind
// as it was. The user follows the advice, sees a success line, and submits the
// downgrade anyway — the outcome #412 exists to prevent.
//
// The old remedy shipped that mistake. Nothing caught it because the tests only
// asserted the literal string, which agrees with a wrong command as readily as a
// right one. So this asserts the STRUCTURE, resolved against the real tree, and
// carries its own control below.
func TestDriftRemedyCommandIsRunnableFromTheWarningsOwnDirectory(t *testing.T) {
	var sink strings.Builder
	msg := versionDriftWarning(&sink, driftSlug, "0.4.0", "0.5.2")
	if msg == "" {
		t.Fatalf("the BEHIND case must render a warning — nothing below is meaningful without one")
	}
	got := parseAsCommand(t, remedyFromWarning(t, msg))

	if got.path != "civitai app pull" {
		t.Errorf("the remedy resolves to %q, want `civitai app pull` — syncing the released code is what an "+
			"author who is BEHIND has to do", got.path)
	}
	if got.app != driftSlug {
		t.Errorf("the remedy names --app %q, want %q", got.app, driftSlug)
	}
	if got.argsErr != nil {
		t.Errorf("the remedy's positional arguments are rejected by the command it names (%v) — a remedy that "+
			"exits 2 is worse than no remedy", got.argsErr)
	}
	// The whole finding, in one assertion: the target directory must be THIS
	// one. app_pull.go's default target is ./<slug>, so an absent [dir] is not a
	// harmless omission — it is a different, wrong command.
	if len(got.positional) != 1 || got.positional[0] != "." {
		t.Errorf("the remedy passes %v as [dir], want exactly [\".\"]. Without it `app pull` targets ./%s, so the "+
			"command CLONES A SECOND COPY of the app inside the repo this warning was printed in and leaves the "+
			"checkout still behind — the author then submits the downgrade believing they synced.",
			got.positional, driftSlug)
	}

	// 🔴 CONTROL ON THE INSTRUMENT. The assertion above must be able to tell the
	// two forms apart; if the parser reported [""] or ["."] for both, it would
	// pass over the very bug it exists to catch. The pre-fix string is fed
	// through the SAME parser and must come back with no [dir] at all.
	old := parseAsCommand(t, "civitai app pull --app "+driftSlug)
	if len(old.positional) != 0 {
		t.Fatalf("the control parsed %v as positionals for the no-[dir] form, want none — this test cannot "+
			"distinguish the fixed remedy from the broken one, so its verdict above means nothing", old.positional)
	}
	if old.path != "civitai app pull" || old.argsErr != nil {
		t.Fatalf("the control must differ from the fixed form ONLY in [dir] (path=%q err=%v)", old.path, old.argsErr)
	}
}

// TestAppStatusDriftRemedyReachesTheUser is the end-to-end half: the structural
// test above proves the string is a runnable command, this proves that string is
// what an author actually sees on stderr.
func TestAppStatusDriftRemedyReachesTheUser(t *testing.T) {
	var calls int32
	srv := driftServer(t, driftFixture(t), &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	_, errOut, err := run(t, "app", "status", driftSlug)
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	want := "civitai app pull . --app " + driftSlug
	if !strings.Contains(errOut, want) {
		t.Errorf("the printed remedy is not %q:\n%s", want, errOut)
	}
	if strings.Contains(errOut, "app pull --app "+driftSlug) {
		t.Errorf("the printed remedy still carries the no-[dir] form, which clones a second copy of the app "+
			"inside the checkout instead of syncing it:\n%s", errOut)
	}
}

// ---------------------------------------------------------------------------
// The advisory check runs AFTER the answer is rendered — #413 audit finding 2
// ---------------------------------------------------------------------------

// orderedSink is one buffer standing in for BOTH streams, so the relative order
// of stdout and stderr writes becomes observable. The mutex is not decoration:
// the assertion in the `--id` arm reads it from the HTTP handler's goroutine
// while the command is writing from its own.
type orderedSink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *orderedSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *orderedSink) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// runInto executes the root command with BOTH streams pointed at w.
func runInto(t *testing.T, w io.Writer, args ...string) error {
	t.Helper()
	root := NewRootCmd()
	root.SetOut(w)
	root.SetErr(w)
	root.SetArgs(args)
	return root.Execute()
}

// TestAppStatusRendersTheAnswerBeforeTheAdvisoryDriftCheck is #413 audit
// finding 2.
//
// The drift line is advisory and MAY NOT PRINT AT ALL, while the detail view is
// the answer the user asked for and is already in hand. Running the check first
// made every `app status` hold that answer back behind an optional second round
// trip on a 30s client budget — a slow or hanging API delayed output the command
// had already fetched.
//
// 🔴 IT IS AN ORDERING TEST, NOT A STREAM TEST, AND THE TWO ARE INDEPENDENT.
// stdout/stderr purity is asserted elsewhere and is unaffected: pointing both
// streams at one buffer here is exactly what makes ORDER — which the separate
// buffers of the normal harness cannot see — observable at all.
func TestAppStatusRendersTheAnswerBeforeTheAdvisoryDriftCheck(t *testing.T) {
	t.Run("the rendered answer precedes the warning in the output stream", func(t *testing.T) {
		var calls int32
		srv := driftServer(t, driftFixture(t), &calls, 0)
		writeDriftManifest(t, driftSlug, "0.4.0")
		setupDriftEnv(t, srv.URL)

		sink := &orderedSink{}
		if err := runInto(t, sink, "app", "status", driftSlug); err != nil {
			t.Fatalf("app status: %v", err)
		}
		all := sink.snapshot()
		answer := strings.Index(all, "Block ID:")
		warn := strings.Index(all, driftWarnCore)
		if answer < 0 || warn < 0 {
			t.Fatalf("this case needs BOTH the detail view and the warning to appear (answer=%d warn=%d) — "+
				"otherwise the ordering below is read off a stream missing one of them:\n%s", answer, warn, all)
		}
		if answer > warn {
			t.Errorf("the advisory drift warning was emitted BEFORE the answer the command was asked for. "+
				"The warning is optional and costs a second round trip; the detail view is already in hand.\n%s", all)
		}
	})

	// The stronger arm, and the one that actually pins the finding: the ordering
	// above could be produced by buffering. This asserts the REQUEST goes out
	// after the render, by looking at what had been written when it arrived.
	// It uses `--id` because that is the path where the drift check still makes
	// a request of its own (the slug path reuses rows — see the reuse test).
	t.Run("the advisory request is issued after the answer is written", func(t *testing.T) {
		sink := &orderedSink{}
		var calls int32
		var seenAtDrift string
		var mu sync.Mutex
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&calls, 1)
			if id := r.URL.Query().Get("id"); id != "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"submission": map[string]any{
					"id": id, "blockId": driftSlug, "version": "0.7.0", "status": "withdrawn",
					"deployState": nil, "submittedAt": "2026-08-03T10:00:00.000Z", "liveUrl": nil,
				}})
				return
			}
			if n > 1 {
				mu.Lock()
				seenAtDrift = sink.snapshot()
				mu.Unlock()
			}
			_ = json.NewEncoder(w).Encode(driftFixture(t))
		}))
		defer srv.Close()
		writeDriftManifest(t, driftSlug, "0.4.0")
		setupDriftEnv(t, srv.URL)

		if err := runInto(t, sink, "app", "status", "--id", "pubreq_07"); err != nil {
			t.Fatalf("app status --id: %v", err)
		}
		if n := atomic.LoadInt32(&calls); n != 2 {
			t.Fatalf("%d request(s), want 2 — without the advisory request there is no ordering to observe", n)
		}
		mu.Lock()
		at := seenAtDrift
		mu.Unlock()
		if !strings.Contains(at, "Block ID:") {
			t.Errorf("the advisory drift listing was requested while the answer was still unwritten. What had been "+
				"emitted at that moment was:\n%q\nThe render must not wait on an optional lookup.", at)
		}
	})
}

// ---------------------------------------------------------------------------
// The drift check reuses the rows the detail lookup already read — #413 audit
// finding 3
// ---------------------------------------------------------------------------

// urlRecordingServer answers both spellings and records every request URL, so a
// test can assert not only HOW MANY requests were made but WHICH.
func urlRecordingServer(t *testing.T, body map[string]any, urls *[]string, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*urls = append(*urls, r.URL.String())
		mu.Unlock()
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
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestAppStatusDriftReusesTheRowsTheDetailLookupAlreadyRead is #413 audit
// finding 3.
//
// 🔴 THE TWO REQUESTS WERE BYTE-IDENTICAL. `GetSubmission(ctx, "", slug)` and
// `ListSubmissions(ctx, slug)` both build submissionsURL("", slug); the first
// one already had the whole narrowed listing and threw everything but
// Submissions[0] away, and the drift check then asked for it again. Same URL,
// same auth, same page — pure latency and rate-limit budget (429 is a case this
// command handles) for data already in memory.
//
// Both paths are asserted, because the fix must not be "make the second call
// go away everywhere": the `--id` spelling answers with a single-row envelope
// and no listing at all, so there it genuinely has to ask.
func TestAppStatusDriftReusesTheRowsTheDetailLookupAlreadyRead(t *testing.T) {
	t.Run("slug path: one request, and it is the detail lookup", func(t *testing.T) {
		var mu sync.Mutex
		var urls []string
		srv := urlRecordingServer(t, driftFixture(t), &urls, &mu)
		writeDriftManifest(t, driftSlug, "0.4.0")
		setupDriftEnv(t, srv.URL)

		_, errOut, err := run(t, "app", "status", driftSlug)
		if err != nil {
			t.Fatalf("app status: %v", err)
		}
		// The warning still has to be RIGHT — a reuse that silenced the check
		// would also make the count 1.
		if !strings.Contains(errOut, driftWarnLine) {
			t.Fatalf("reusing the rows must not change the answer; want %q:\n%s", driftWarnLine, errOut)
		}
		mu.Lock()
		got := append([]string(nil), urls...)
		mu.Unlock()
		if len(got) != 1 {
			t.Fatalf("`app status <slug>` made %d requests (%v), want 1 — the drift check must read the rows the "+
				"detail lookup already returned, not re-issue the identical GET", len(got), got)
		}
		if !strings.Contains(got[0], "blockId="+driftSlug) {
			t.Errorf("the single request was %q, want the ?blockId= narrowing", got[0])
		}
	})

	t.Run("--id path: two requests, because there is no listing to reuse", func(t *testing.T) {
		var mu sync.Mutex
		var urls []string
		srv := urlRecordingServer(t, driftFixture(t), &urls, &mu)
		writeDriftManifest(t, driftSlug, "0.4.0")
		setupDriftEnv(t, srv.URL)

		_, errOut, err := run(t, "app", "status", "--id", "pubreq_07")
		if err != nil {
			t.Fatalf("app status --id: %v", err)
		}
		if !strings.Contains(errOut, driftWarnLine) {
			t.Fatalf("the --id path must still warn:\n%s", errOut)
		}
		mu.Lock()
		got := append([]string(nil), urls...)
		mu.Unlock()
		if len(got) != 2 {
			t.Fatalf("`app status --id` made %d requests (%v), want 2 — a `?id=` lookup returns ONE row and no "+
				"listing, so the drift check has no rows to reuse and must fetch them", len(got), got)
		}
		if !strings.Contains(got[0], "id=pubreq_07") {
			t.Errorf("first request %q should be the id lookup", got[0])
		}
		if !strings.Contains(got[1], "blockId="+driftSlug) || strings.Contains(got[1], "id=pubreq_07") {
			t.Errorf("second request %q should be the ?blockId= listing the id lookup could not supply", got[1])
		}
	})
}

// TestAppStatusDriftManifestGateRunsBeforeTheNetwork keeps the "manifest first,
// network second" property measurable after the row reuse.
//
// It used to be pinned on the slug path by a 1-vs-2 request count. Reusing the
// rows makes that path 1 either way, so the property moved here, to the `--id`
// path — the only path left where declining actually SAVES a request. Both arms
// run against the same server so the difference is the manifest and nothing
// else.
func TestAppStatusDriftManifestGateRunsBeforeTheNetwork(t *testing.T) {
	for _, tc := range []struct {
		name      string
		manifest  string // "" = write no manifest at all
		wantCalls int32
		wantWarn  bool
	}{
		{name: "no manifest in cwd: the listing is never fetched", manifest: "", wantCalls: 1, wantWarn: false},
		// 🔴 THE OFFLINE GATES ARE PLURAL, AND THIS ONE IS THE UNPARSEABLE
		// VERSION. It is decided from the manifest alone, so it must also land
		// before the network — and asserting it here is what gives that gate a
		// killing mutation of its OWN. versionDriftWarning refuses an
		// unparseable version too (audit finding 4), which means deleting this
		// gate changes no OUTPUT anywhere: the mutant survives every text
		// assertion in this file and dies only on the request count. Redundant
		// guards need a reason each or one of them is untested.
		{name: "an unparseable local version: still decided offline", manifest: "dev", wantCalls: 1, wantWarn: false},
		{name: "a matching manifest: the listing is fetched", manifest: "0.4.0", wantCalls: 2, wantWarn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			if tc.manifest != "" {
				writeDriftManifest(t, driftSlug, tc.manifest)
			} else {
				t.Chdir(t.TempDir())
			}
			setupDriftEnv(t, srv.URL)

			_, errOut, err := run(t, "app", "status", "--id", "pubreq_07")
			if err != nil {
				t.Fatalf("app status --id: %v", err)
			}
			if n := atomic.LoadInt32(&calls); n != tc.wantCalls {
				t.Errorf("%d request(s), want %d — everything decidable from the local manifest is decided BEFORE "+
					"the network, so a caller who is not standing in a comparable app directory pays nothing for "+
					"a comparison that cannot be made", n, tc.wantCalls)
			}
			if got := strings.Contains(errOut, driftWarnCore); got != tc.wantWarn {
				t.Errorf("warn=%v, want %v — the arms must differ in the WARNING as well as the count, or the "+
					"count difference is not attributable to the gate.\nstderr:\n%s", got, tc.wantWarn, errOut)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The no-false-BEHIND property belongs to the FUNCTION — #413 audit finding 4
// ---------------------------------------------------------------------------

// TestVersionDriftWarningRefusesUnparseableVersionsItself is #413 audit
// finding 4.
//
// 🔴 THE PROPERTY USED TO LIVE ONLY IN THE CALLER, AND THE FUNCTION IS THE
// REUSABLE HALF. warnLocalVersionDrift gates on isParseableVersion, so no live
// path could reach this — but called directly, the pre-fix function produced
// exactly the two claims the whole feature is built to avoid:
//
//	versionDriftWarning(w, "custom-generators", "dev", "0.5.2")
//	  -> "⚠ local block.manifest.json is dev — BEHIND the highest APPROVED version …"
//	versionDriftWarning(w, "custom-generators", "", "0.5.2")
//	  -> "⚠ local block.manifest.json is  — BEHIND …"   (a hole where the version goes)
//
// compareVersions treats an unparseable FIRST argument as OLDER — a deliberate
// default for the self-update check, where any real release should surface —
// and through this function it reads as a confident "your repo is BEHIND".
// TestVersionDriftWarningIsThreeWay only ever fed it parseable pairs, so the
// trap was unpinned and the next caller (the `app submit` guard is the known
// one) would have inherited none of the caller's protection.
//
// Zero behaviour change today by construction; this is the case that keeps it
// that way.
func TestVersionDriftWarningRefusesUnparseableVersionsItself(t *testing.T) {
	var sink strings.Builder
	for _, tc := range []struct{ name, local, published string }{
		{"unparseable local", "dev", "0.5.2"},
		{"EMPTY local — the double-space hole", "", "0.5.2"},
		{"unparseable published", "0.4.0", "nightly"},
		{"both unparseable", "dev", "nightly"},
		{"whitespace local", "   ", "0.5.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionDriftWarning(&sink, driftSlug, tc.local, tc.published); got != "" {
				t.Errorf("versionDriftWarning(%q, %q) stated a comparison it cannot make:\n%s\n"+
					"A version this CLI cannot order is not evidence of drift, and the caller's gate is not "+
					"this function's guarantee — the next caller does not inherit it.", tc.local, tc.published, got)
			}
		})
	}
	// Positive control: the guard must not have swallowed the real case.
	if got := versionDriftWarning(&sink, driftSlug, "0.4.0", "0.5.2"); got == "" {
		t.Fatalf("the parseable BEHIND case went silent — the guard above rejects input it must accept, so the " +
			"silences it reports prove nothing")
	}
}

// ---------------------------------------------------------------------------
// "approved" is matched the way every other reader in this binary matches it —
// #413 audit nit 6
// ---------------------------------------------------------------------------

// TestHighestApprovedVersionNormalisesTheStatus is #413 audit nit 6.
//
// 🔴 A RAW `s.Status == "approved"` CANNOT FAIL LOUDLY. If the server ever
// answers "Approved", the filter matches nothing, highestApprovedVersion reports
// "nothing approved", and the drift warning goes permanently, silently inert —
// the feature is off and every test still passes because every fixture spells it
// lower-case. appblocks.go and app_pull.go's pullReviewAdvice both already fold
// this field; this read is now aligned with them, which also shrinks the
// consolidation delta against the `app submit` guard that will share the
// predicate.
func TestHighestApprovedVersionNormalisesTheStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		want   string
		wantOK bool
	}{
		{name: "padded and capitalised", status: " Approved ", want: "0.5.2", wantOK: true},
		{name: "shouted", status: "APPROVED", want: "0.5.2", wantOK: true},
		{name: "tab-padded", status: "\tapproved\n", want: "0.5.2", wantOK: true},
		// 🔴 The control on the normalisation itself: folding case and trimming
		// space must not turn into a substring match. A status that merely
		// CONTAINS the word is a different state and must not count.
		{name: "not-approved must still not count", status: "not approved", want: "", wantOK: false},
		{name: "preapproved must still not count", status: "preapproved", want: "", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := driftSubs([3]string{driftSlug, "0.5.2", tc.status})
			got, ok := highestApprovedVersion(in, driftSlug)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("highestApprovedVersion(status=%q) = (%q, %v), want (%q, %v)",
					tc.status, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestAppStatusDriftWarnsOnACapitalisedApprovedStatus is the end-to-end half of
// nit 6: the row goes through the real JSON decode and the real command.
func TestAppStatusDriftWarnsOnACapitalisedApprovedStatus(t *testing.T) {
	var calls int32
	body := driftRows(t,
		driftRow{"pubreq_06", "0.6.0", "pending", "building", "2026-08-02T10:00:00.000Z"},
		driftRow{"pubreq_05", "0.5.2", " Approved ", "live", "2026-08-01T10:00:00.000Z"},
	)
	srv := driftServer(t, body, &calls, 0)
	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	_, errOut, err := run(t, "app", "status", driftSlug)
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(errOut, "which is 0.5.2.") {
		t.Errorf("a status of \" Approved \" was not recognised as approved, so the drift warning went silent. "+
			"A case-sensitive filter fails this way and only this way — inert, never loud.\nstderr:\n%s", errOut)
	}
	if strings.Contains(errOut, "which is 0.6.0.") {
		t.Errorf("the pending row was counted as approved:\n%s", errOut)
	}
}

// ---------------------------------------------------------------------------
// The advisory request's OWN deadline — #413 delta-audit finding 2
// ---------------------------------------------------------------------------

// TestAppStatusDriftLookupGetsADeadlineOfItsOwn pins the REMOVAL direction of
// driftLookupTimeout, which the rest of this suite could not see.
//
// 🔴 SETTING THE TIMEOUT TO 1ns KILLS A DOZEN TESTS AND STILL PROVES NOTHING
// ABOUT THIS. That mutation only shows the value is wired to something; deleting
// the `context.WithTimeout` outright and handing warnLocalVersionDrift the
// command's own ctx survived the entire suite (measured on d1df4f5), because no
// case observed how LONG a hung advisory listing may hold the command.
//
// 🔴 AND IT CANNOT BE PINNED THE OBVIOUS WAY — the advisory request's deadline
// is not comparable to "the parent's", because `app status` builds its own
// `ctx := context.Background()` and never reads cmd.Context(). A parent deadline
// handed to root.ExecuteContext is therefore inherited by nothing, and a test
// asserting the advisory budget is shorter than it would be comparing against a
// context the command does not use. (Measured while writing this test: with the
// WithTimeout deleted, the command ran 30s against a 4s "parent" — the wall
// clocks are simply independent.) What actually bounds the request without the
// dedicated deadline is the appapi client's own 30s ANSWER budget, so that is
// the reference this case measures against.
//
// The construction: an advisory budget shrunk to 50ms and a drift listing that
// NEVER answers. The only thing that can end that request is the client giving
// up, so how long the command takes IS the deadline the advisory request
// carried — bounded on BOTH sides, because a fast return alone would also be
// produced by a check that declined to run. With the dedicated budget the
// command returns just after 50ms; with it removed it sits for the client's
// full 30s.
//
// It drives the `--id` path because that is the only path that still makes an
// advisory request at all — on the slug path the rows are reused and the lister
// never touches the context (#413's other half).
func TestAppStatusDriftLookupGetsADeadlineOfItsOwn(t *testing.T) {
	advisoryBudget := 50 * time.Millisecond
	// The budget that bounds this request when the dedicated one is removed:
	// appapi's unexported defaultTimeout, the client's budget for the ANSWER.
	// Spelled out here because it is the thing being distinguished FROM; if it
	// is ever lowered towards `ceiling` this case weakens and must be retuned.
	clientAnswerBudget := 30 * time.Second
	ceiling := 2 * time.Second
	// Fixture control: the two budgets must be far enough apart that neither
	// scheduling noise nor a slow machine can make one read as the other.
	if ceiling < 20*advisoryBudget || clientAnswerBudget < 10*ceiling {
		t.Fatalf("budgets %v / %v / %v are not separated enough to tell a dedicated deadline from the client's "+
			"answer budget", advisoryBudget, ceiling, clientAnswerBudget)
	}

	orig := driftLookupTimeout
	t.Cleanup(func() { driftLookupTimeout = orig })
	driftLookupTimeout = advisoryBudget

	var calls int32
	var reachedTheHang, clientGaveUp atomic.Bool
	rows, _ := driftFixture(t)["submissions"].([]map[string]any)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if id := r.URL.Query().Get("id"); id != "" {
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
		// The drift listing: answer never. Returning only once the request
		// context dies means the server contributes no timing of its own.
		reachedTheHang.Store(true)
		<-r.Context().Done()
		clientGaveUp.Store(true)
	}))
	t.Cleanup(srv.Close)

	writeDriftManifest(t, driftSlug, "0.4.0")
	setupDriftEnv(t, srv.URL)

	start := time.Now()
	out, errOut, err := run(t, "app", "status", "--id", "pubreq_07")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("a drift listing that never answers must not fail `app status` — the detail view already "+
			"rendered its answer: %v", err)
	}
	// REACHABILITY, both halves: the advisory request must have been ISSUED and
	// must have reached the hang, or "it returned quickly" is trivially true of
	// a check that declined to run.
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("%d request(s), want 2 (the id lookup, then the advisory listing) — without the second call "+
			"this case measures the deadline of a request that was never made", n)
	}
	if !reachedTheHang.Load() {
		t.Fatalf("the advisory listing handler never reached its hang, so nothing here was bounded by a deadline")
	}

	// 🔴 THE DISCRIMINATOR, two-sided. The upper bound goes red the moment the
	// dedicated deadline is deleted and the request falls back to the client's
	// answer budget; the lower bound is what stops a command that returned
	// instantly — for any reason other than this deadline — from reading as a
	// pass.
	if elapsed < advisoryBudget {
		t.Errorf("`app status --id` returned in %v, sooner than the %v the advisory request was given against a "+
			"listing that never answers — so whatever ended it, it was not this deadline, and this case is "+
			"measuring something else.", elapsed, advisoryBudget)
	}
	if elapsed >= ceiling {
		t.Errorf("`app status --id` took %v against a %v advisory budget. The drift check's request is optional "+
			"and runs AFTER the answer is printed, so it gets its own, shorter deadline; without it the request "+
			"falls back to the appapi client's %v ANSWER budget and a hung listing becomes an arbitrary wait for "+
			"a line the user may never see (#412/#413).", elapsed, advisoryBudget, clientAnswerBudget)
	}
	// A listing that never answered cannot have produced a published version, so
	// silence here is also a check that the timeout was reached rather than the
	// request somehow succeeding.
	assertNoDriftWarning(t, "the advisory listing timed out", out, errOut)

	// Last, and POLLED rather than read once: the handler observes the client
	// disconnect on its own goroutine, so an instantaneous read here races it —
	// measured, that race made the removal mutant fail on THIS line instead of
	// on the timing bounds above, which is a red for the wrong reason. It stays
	// because it is the one check that says the CLIENT ended the request, but it
	// runs after the discriminator and gives the server side time to notice.
	deadline := time.Now().Add(ceiling)
	for !clientGaveUp.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !clientGaveUp.Load() {
		t.Errorf("the hanging request never ended with the client disconnecting — the server, not a deadline, "+
			"decided when this finished, so %v is not evidence about the advisory budget", elapsed)
	}
}
