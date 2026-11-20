package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// TESTS FOR THE ACTIONS-RUN LINE in `civitai app status <blockId>`.
//
// The server returns `actionsRunSource { repository, tag, sha }` on a
// submission that a GitHub Actions run sent (public repositories only). The
// detail view then says which run submitted it, in the one sentence the server
// allows, in place of the "reported by the client … unverified" note. Without
// the field the output is exactly what it was.
//
// Fixture values are pairwise distinct and distinct from every constant the
// assertions name: the run's sha is not the claimed commit, the tag is not the
// version, so a renderer that read the wrong field cannot pass by luck.

const (
	runSHA     = "177445b0c9d2e8f1a3b5c7d9e1f2a4b6c8d0e2f4"
	runClaimed = "5be20c71f4a39d8e6b0c2a4f81d7395e0ac6b2d1"
)

func runRow(withRun bool, withCommit bool) map[string]any {
	row := map[string]any{
		"id": "pubreq_9", "blockId": "knot-tier", "version": "0.9.1", "status": "approved",
		"deployState": "live", "submittedAt": "2026-11-18T09:00:00.000Z",
		"liveUrl": "https://knot-tier.civit.ai/",
	}
	if withCommit {
		row["sourceCommit"] = runClaimed
		row["sourceDirty"] = false
	}
	if withRun {
		row["actionsRunSource"] = map[string]any{
			"repository": "harbor-tools/knot-tier", "tag": "v2.4.0-rc.1", "sha": runSHA,
		}
	}
	return row
}

func statusDetail(t *testing.T, rows []map[string]any, args ...string) string {
	t.Helper()
	srv := statusServer(t, map[string]any{"submissions": rows}, http.StatusOK, nil)
	defer srv.Close()
	provStatusEnv(t, srv.URL)
	out, _, err := run(t, append([]string{"app", "status", "knot-tier"}, args...)...)
	if err != nil {
		t.Fatalf("app status knot-tier: %v", err)
	}
	return out
}

// TestAppStatusDetailNamesTheActionsRun pins the exact sentence, and that it
// takes the claim note's place while the Source commit line stays.
func TestAppStatusDetailNamesTheActionsRun(t *testing.T) {
	out := statusDetail(t, []map[string]any{runRow(true, true)})

	const want = "\n  Submitted by a GitHub Actions run of harbor-tools/knot-tier @ v2.4.0-rc.1 (177445b)\n"
	if !strings.Contains(out, want) {
		t.Errorf("the detail view does not carry the Actions-run line %q:\n%s", want, out)
	}
	if strings.Contains(out, sourceClaimNote) {
		t.Errorf("the unverified-claim note still printed beside a recorded Actions run:\n%s", out)
	}
	if !strings.Contains(out, "Source commit:") || !strings.Contains(out, runClaimed) {
		t.Errorf("the Source commit line was dropped; only the note below it is replaced:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "verified") {
		t.Errorf("the output says \"verified\"; a recorded run names who SUBMITTED, not what was built:\n%s", out)
	}
}

// TestAppStatusDetailRunWithoutAClaimedCommit — the Action may submit with no
// sourceCommit; the run is still the fact worth showing, and no claim note
// prints because nothing was claimed.
func TestAppStatusDetailRunWithoutAClaimedCommit(t *testing.T) {
	out := statusDetail(t, []map[string]any{runRow(true, false)})
	if !strings.Contains(out, "\n  Submitted by a GitHub Actions run of harbor-tools/knot-tier @ v2.4.0-rc.1 (177445b)\n") {
		t.Errorf("no Actions-run line for a row with a run and no claimed commit:\n%s", out)
	}
	if strings.Contains(out, "Source commit") || strings.Contains(out, sourceClaimNote) {
		t.Errorf("a row that claimed no commit printed commit text:\n%s", out)
	}
}

// TestAppStatusDetailWithoutARunIsUnchanged — an older server (no key), a row
// with no run (null) and a malformed run (an empty field) all render the
// pre-feature output: the claim note, and no Actions-run sentence.
func TestAppStatusDetailWithoutARunIsUnchanged(t *testing.T) {
	absent := statusDetail(t, []map[string]any{runRow(false, true)})

	nullRow := runRow(false, true)
	nullRow["actionsRunSource"] = nil
	null := statusDetail(t, []map[string]any{nullRow})

	emptyRow := runRow(true, true)
	emptyRow["actionsRunSource"] = map[string]any{"repository": "harbor-tools/knot-tier", "tag": "", "sha": runSHA}
	empty := statusDetail(t, []map[string]any{emptyRow})

	if !strings.Contains(absent, "\n  "+sourceClaimNote+"\n") {
		t.Fatalf("CONTROL: the absent-field row does not print the claim note:\n%s", absent)
	}
	if strings.Contains(absent, "GitHub Actions") {
		t.Errorf("a row with no run mentions GitHub Actions:\n%s", absent)
	}
	if null != absent {
		t.Errorf("actionsRunSource:null renders differently from an absent key.\nabsent:\n%s\nnull:\n%s", absent, null)
	}
	if empty != absent {
		t.Errorf("a run with an empty field renders differently from no run.\nabsent:\n%s\nempty:\n%s", absent, empty)
	}
	for _, field := range []string{"repository", "sha"} {
		row := runRow(true, true)
		run := map[string]any{"repository": "harbor-tools/knot-tier", "tag": "v2.4.0-rc.1", "sha": runSHA}
		run[field] = ""
		row["actionsRunSource"] = run
		if got := statusDetail(t, []map[string]any{row}); got != absent {
			t.Errorf("a run with an empty %s renders differently from no run.\nabsent:\n%s\ngot:\n%s", field, absent, got)
		}
	}
}

// TestAppStatusJSONActionsRunSource — `--json` carries the object when present
// and no key at all when absent (byte-identical to the pre-field payload).
func TestAppStatusJSONActionsRunSource(t *testing.T) {
	with := statusDetail(t, []map[string]any{runRow(true, true)}, "--json")
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(with), &doc); err != nil {
		t.Fatalf("--json is not JSON: %v\n%s", err, with)
	}
	var got appapi.ActionsRunSource
	if err := json.Unmarshal(doc["actionsRunSource"], &got); err != nil {
		t.Fatalf("--json has no decodable actionsRunSource: %v\n%s", err, with)
	}
	if got != (appapi.ActionsRunSource{Repository: "harbor-tools/knot-tier", Tag: "v2.4.0-rc.1", SHA: runSHA}) {
		t.Errorf("--json actionsRunSource = %+v", got)
	}

	without := statusDetail(t, []map[string]any{runRow(false, true)}, "--json")
	if strings.Contains(without, "actionsRunSource") {
		t.Errorf("--json for a row with no run carries an actionsRunSource key:\n%s", without)
	}
}

// TestActionsRunLineCannotBeForged — every field of the run is SERVER text on
// the human's terminal, so each goes through safeTermSingle. One hostile byte
// per field; delete any one of the three gates and this goes red.
func TestActionsRunLineCannotBeForged(t *testing.T) {
	for _, tc := range []struct{ name, repo, tag, sha string }{
		{"repository", "harbor-tools/\x1b[2Jknot-tier", "v2.4.0-rc.1", runSHA},
		{"tag", "harbor-tools/knot-tier", "v2.4.0\x1b[1A", runSHA},
		{"sha", "harbor-tools/knot-tier", "v2.4.0-rc.1", "\x1b[2J" + runSHA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := runRow(true, false)
			row["actionsRunSource"] = map[string]any{"repository": tc.repo, "tag": tc.tag, "sha": tc.sha}
			out := statusDetail(t, []map[string]any{row})
			if !strings.Contains(out, "Submitted by a GitHub Actions run of ") {
				t.Fatalf("CONTROL: no Actions-run line, so this test cannot see a raw escape in it:\n%q", out)
			}
			if strings.Contains(out, "\x1b") {
				t.Errorf("a hostile %s put a raw ESC on stdout:\n%q", tc.name, out)
			}
		})
	}
}
