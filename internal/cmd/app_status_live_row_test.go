package cmd

// THE "LIVE SUBMISSION" BLOCK IN `app status <slug>` — civitai/civitai-app-starters#573 item 5.
//
// The blockId detail view renders the NEWEST row. When that row is a `withdrawn`
// duplicate or a `pending` resubmission, the build actually serving was nowhere
// on screen, so an author read a live app as not-live. The detail view now names
// the highest APPROVED row beside the newest one whenever the two differ.
//
// 🔴 EVERY FIXTURE HERE KEEPS THE NEWEST ROW AND THE APPROVED ROW DISTINCT ON
// EVERY FIELD THE ASSERTIONS READ (id, version, status, deploy state, live URL),
// and the expected values are written out by hand. A mutant that renders the
// newest row a second time, or a hard-coded row, then prints a value this suite
// names as wrong. The one deliberate exception is the same-VERSION duplicate
// case, whose whole point is that only the publish-request id separates the rows.

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

const liveBlockSlug = "custom-generators"

// liveRow is one submissions row, spelled out field by field.
type liveRow struct {
	id, version, status, deploy, liveURL, submittedAt string
}

// liveRowsBody renders rows (newest first) as the route's list body. An empty
// deploy / liveURL emits JSON null.
func liveRowsBody(rows ...liveRow) map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var deploy, live any
		if r.deploy != "" {
			deploy = r.deploy
		}
		if r.liveURL != "" {
			live = r.liveURL
		}
		out = append(out, map[string]any{
			"id": r.id, "blockId": liveBlockSlug, "version": r.version, "status": r.status,
			"deployState": deploy, "submittedAt": r.submittedAt, "updatedAt": r.submittedAt,
			"createdAt": r.submittedAt, "liveUrl": live,
		})
	}
	return map[string]any{"submissions": out}
}

// The approved, serving row most fixtures sit BELOW a newer row.
var approvedLive = liveRow{"pubreq_A53", "0.5.3", "approved", "live", "https://approved-053.example.test/", "2026-08-01T10:00:00.000Z"}

// liveBlockOf returns the "Live submission" / "Approved submission" block's
// fields as a label→value map (whitespace-collapsed), plus its header line.
// header == "" means no block was printed.
func liveBlockOf(out string) (header string, fields map[string]string) {
	fields = map[string]string{}
	in := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Live submission (") || strings.HasPrefix(line, "Approved submission (") {
			header, in = line, true
			continue
		}
		if !in {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			break
		}
		label, value, ok := strings.Cut(strings.TrimSpace(line), ":\t")
		if !ok {
			label, value, ok = strings.Cut(strings.TrimSpace(line), ": ")
		}
		if ok {
			fields[label] = strings.Join(strings.Fields(value), " ")
		}
	}
	return header, fields
}

// headlineVersion returns the value of the TOP `Version:` line — the newest
// row's headline, which this change must leave alone.
func headlineVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		}
	}
	return ""
}

func runLiveStatus(t *testing.T, body map[string]any, args ...string) (out, errOut string, calls int32) {
	t.Helper()
	var n int32
	srv := driftServer(t, body, &n, 0)
	t.Chdir(t.TempDir()) // no manifest: keep the drift check out of the picture
	setupDriftEnv(t, srv.URL)
	out, errOut, err := run(t, append([]string{"app", "status"}, args...)...)
	if err != nil {
		t.Fatalf("app status %v: %v", args, err)
	}
	return out, errOut, atomic.LoadInt32(&n)
}

// TestAppStatusDetailNamesTheLiveSubmissionWhenTheNewestRowIsNot is the
// regression case: the newest row is a withdrawn / pending row above the
// approved, serving one.
func TestAppStatusDetailNamesTheLiveSubmissionWhenTheNewestRowIsNot(t *testing.T) {
	for _, tc := range []struct {
		name        string
		newest      liveRow
		wantHeadVer string
	}{
		{
			name:        "withdrawn duplicate newer than the approved row",
			newest:      liveRow{"pubreq_W61", "0.6.1", "withdrawn", "", "", "2026-08-03T10:00:00.000Z"},
			wantHeadVer: "0.6.1",
		},
		{
			name:        "pending resubmission newer than the approved row",
			newest:      liveRow{"pubreq_P70", "0.7.0", "pending", "building", "", "2026-08-02T10:00:00.000Z"},
			wantHeadVer: "0.7.0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := liveRowsBody(tc.newest, approvedLive,
				liveRow{"pubreq_R40", "0.4.0", "rejected", "failed", "", "2026-07-02T10:00:00.000Z"})
			out, _, calls := runLiveStatus(t, body, liveBlockSlug)

			if calls != 1 {
				t.Errorf("%d request(s), want 1 — the live row comes from the rows the slug lookup already holds", calls)
			}
			// The newest-row headline is unchanged: it still answers "state of my
			// latest submission".
			if got := headlineVersion(out); got != tc.wantHeadVer {
				t.Errorf("headline Version = %q, want %q (the NEWEST row) — the headline must not move:\n%s", got, tc.wantHeadVer, out)
			}
			header, f := liveBlockOf(out)
			if header != "Live submission (not the one shown above):" {
				t.Fatalf("no `Live submission` block was printed, so the serving build is invisible (#573 item 5); header=%q\n%s", header, out)
			}
			want := map[string]string{
				"Version":         "0.5.3",
				"Publish request": "pubreq_A53",
				"Status":          "approved",
				"Deploy state":    "live",
				"Live at":         "https://approved-053.example.test/",
			}
			for k, v := range want {
				if f[k] != v {
					t.Errorf("live block %s = %q, want %q (the APPROVED row's value)\n%s", k, f[k], v, out)
				}
			}
			if len(f) != len(want) {
				t.Errorf("live block has fields %v, want exactly %v", f, want)
			}
			// The old footer claimed the SLUG was not serving — false here.
			if strings.Contains(out, "Not live yet") {
				t.Errorf("printed `Not live yet` while the app IS serving the approved row:\n%s", out)
			}
			if !strings.Contains(out, "This submission is not live — "+liveBlockSlug+".civit.ai is serving the approved submission below.") {
				t.Errorf("missing the not-this-row sentence:\n%s", out)
			}
		})
	}
}

// TestAppStatusDetailLiveBlockKeysOnTheRowNotTheVersion — a withdrawn duplicate
// of the SAME version as the approved row. Only the publish-request id tells
// them apart, so a version-equality shortcut would hide the live row here.
func TestAppStatusDetailLiveBlockKeysOnTheRowNotTheVersion(t *testing.T) {
	body := liveRowsBody(
		liveRow{"pubreq_W53", "0.5.3", "withdrawn", "", "", "2026-08-03T10:00:00.000Z"},
		approvedLive,
	)
	out, _, _ := runLiveStatus(t, body, liveBlockSlug)
	header, f := liveBlockOf(out)
	if header == "" {
		t.Fatalf("no live block for a same-version withdrawn duplicate:\n%s", out)
	}
	if f["Publish request"] != "pubreq_A53" {
		t.Errorf("live block Publish request = %q, want pubreq_A53\n%s", f["Publish request"], out)
	}
}

// TestAppStatusDetailNoLiveBlockWhenNewestIsTheApprovedRow — the healthy steady
// state. A block repeating the row above would be noise on every run.
func TestAppStatusDetailNoLiveBlockWhenNewestIsTheApprovedRow(t *testing.T) {
	body := liveRowsBody(approvedLive,
		liveRow{"pubreq_W49", "0.4.9", "withdrawn", "", "", "2026-07-20T10:00:00.000Z"})
	out, _, _ := runLiveStatus(t, body, liveBlockSlug)
	if header, _ := liveBlockOf(out); header != "" {
		t.Errorf("printed %q although the newest row IS the approved row:\n%s", header, out)
	}
	if !strings.Contains(out, "Live at: https://approved-053.example.test/") {
		t.Errorf("CONTROL: the approved newest row's own Live at line is missing, so the render did not run:\n%s", out)
	}
}

// TestAppStatusDetailNoLiveBlockWithoutAnApprovedRow — nothing approved: the
// old `Not live yet` footer stands and nothing extra is printed.
func TestAppStatusDetailNoLiveBlockWithoutAnApprovedRow(t *testing.T) {
	body := liveRowsBody(
		liveRow{"pubreq_P70", "0.7.0", "pending", "building", "", "2026-08-02T10:00:00.000Z"},
		liveRow{"pubreq_R40", "0.4.0", "rejected", "failed", "", "2026-07-02T10:00:00.000Z"},
	)
	out, _, _ := runLiveStatus(t, body, liveBlockSlug)
	if header, _ := liveBlockOf(out); header != "" {
		t.Errorf("printed %q with no approved row at all:\n%s", header, out)
	}
	if !strings.Contains(out, "Not live yet — "+liveBlockSlug+".civit.ai only serves after") {
		t.Errorf("the `Not live yet` footer must stand when nothing is approved:\n%s", out)
	}
}

// TestAppStatusDetailApprovedButNotServingIsNotCalledLive — an approved row that
// has not finished deploying is named, but under a header that does not claim
// it serves, and the `Not live yet` footer (true here) stays.
func TestAppStatusDetailApprovedButNotServingIsNotCalledLive(t *testing.T) {
	body := liveRowsBody(
		liveRow{"pubreq_P70", "0.7.0", "pending", "", "", "2026-08-02T10:00:00.000Z"},
		liveRow{"pubreq_A60", "0.6.0", "approved", "building", "", "2026-08-01T10:00:00.000Z"},
	)
	out, _, _ := runLiveStatus(t, body, liveBlockSlug)
	header, f := liveBlockOf(out)
	if header != "Approved submission (not the one shown above; not reported as serving):" {
		t.Fatalf("header = %q, want the not-serving header\n%s", header, out)
	}
	if f["Publish request"] != "pubreq_A60" || f["Deploy state"] != "building" {
		t.Errorf("block = %v, want pubreq_A60 / building\n%s", f, out)
	}
	if _, ok := f["Live at"]; ok {
		t.Errorf("a not-serving row must print no Live at:\n%s", out)
	}
	if !strings.Contains(out, "Not live yet") {
		t.Errorf("nothing is serving, so `Not live yet` must stay:\n%s", out)
	}
}

// TestAppStatusDetailByIDPrintsNoLiveBlock pins the documented `--id` decision:
// that lookup reads a single-row envelope and no listing, and the live block
// does not buy itself a second request.
func TestAppStatusDetailByIDPrintsNoLiveBlock(t *testing.T) {
	body := liveRowsBody(
		liveRow{"pubreq_W61", "0.6.1", "withdrawn", "", "", "2026-08-03T10:00:00.000Z"},
		approvedLive,
	)
	out, _, calls := runLiveStatus(t, body, "--id", "pubreq_W61")
	if calls != 1 {
		t.Errorf("%d request(s), want 1 — `--id` must not fetch a listing for the live block", calls)
	}
	if header, _ := liveBlockOf(out); header != "" {
		t.Errorf("`--id` printed %q; it shows exactly the requested row:\n%s", header, out)
	}
	if got := headlineVersion(out); got != "0.6.1" {
		t.Errorf("CONTROL: headline Version = %q, want 0.6.1", got)
	}
}

// TestLiveSubmissionBlockCellsCannotBeForged — the live block's values are
// server text in a tabwriter, exactly like the detail table's cells (#552), so
// each must render as one inert string and none may reach column zero.
func TestLiveSubmissionBlockCellsCannotBeForged(t *testing.T) {
	deploy, url := forgeCell("lsdeploy"), forgeCell("lsurl")
	live := &appapi.Submission{
		BlockID: "my-app", ID: forgeCell("lsid"), Version: forgeCell("lsver"),
		Status: forgeCell("lsstatus"), DeployState: &deploy, LiveURL: &url,
	}
	var buf bytes.Buffer
	printSubmissionDetail(&buf, &appapi.Submission{
		BlockID: "my-app", ID: "pubreq_new", Version: "9.9.9", Status: "pending",
		SubmittedAt: "2026-09-01T10:00:00Z",
	}, live)
	out := buf.String()
	if header, _ := liveBlockOf(out); header == "" {
		t.Fatalf("CONTROL: the live block did not render:\n%s", out)
	}
	for _, label := range []string{"lsid", "lsver", "lsstatus", "lsdeploy", "lsurl"} {
		if !strings.Contains(out, forgeWant(label)) {
			t.Errorf("%s did not render as one inert string; want %q:\n%q", label, forgeWant(label), out)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, forgedRow) {
			t.Errorf("line %d begins with forged text — server text reached column zero:\n%s", i+1, out)
		}
	}
}

// TestAppStatusJSONCarriesTheLiveSubmission — the --json half: additive key,
// row fields still flattened at the top level, key absent when there is
// nothing to say.
func TestAppStatusJSONCarriesTheLiveSubmission(t *testing.T) {
	t.Run("newest withdrawn: liveSubmission is the approved row", func(t *testing.T) {
		body := liveRowsBody(
			liveRow{"pubreq_W61", "0.6.1", "withdrawn", "", "", "2026-08-03T10:00:00.000Z"},
			approvedLive,
		)
		out, _, _ := runLiveStatus(t, body, liveBlockSlug, "--json")
		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("--json is not valid JSON: %v\n%s", err, out)
		}
		// Existing consumers: the shown row's fields stay at the top level.
		if parsed["id"] != "pubreq_W61" || parsed["version"] != "0.6.1" || parsed["status"] != "withdrawn" || parsed["blockId"] != liveBlockSlug {
			t.Errorf("top-level row fields moved or changed: %s", out)
		}
		live, ok := parsed["liveSubmission"].(map[string]any)
		if !ok {
			t.Fatalf("no liveSubmission object in --json:\n%s", out)
		}
		for k, v := range map[string]string{
			"id": "pubreq_A53", "version": "0.5.3", "status": "approved",
			"deployState": "live", "liveUrl": "https://approved-053.example.test/",
		} {
			if live[k] != v {
				t.Errorf("liveSubmission.%s = %v, want %q", k, live[k], v)
			}
		}
	})
	t.Run("newest is the approved row: no liveSubmission key", func(t *testing.T) {
		body := liveRowsBody(approvedLive,
			liveRow{"pubreq_W49", "0.4.9", "withdrawn", "", "", "2026-07-20T10:00:00.000Z"})
		out, _, _ := runLiveStatus(t, body, liveBlockSlug, "--json")
		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("--json is not valid JSON: %v\n%s", err, out)
		}
		if parsed["id"] != "pubreq_A53" {
			t.Fatalf("CONTROL: top-level id = %v, want pubreq_A53", parsed["id"])
		}
		if _, ok := parsed["liveSubmission"]; ok {
			t.Errorf("liveSubmission must be omitted when the newest row IS the approved row:\n%s", out)
		}
	})
}
