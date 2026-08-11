package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// civitai/cli#391. `listingError`'s change arm is ALL the error some routes
// print, and follow-on advice is what the others add — so both README's
// Troubleshooting row and listingError's own comment enumerate which routes
// print advice and which do not.
//
// 🔴 That enumeration has been WRONG TWICE in this change's own history: first
// as a vague overstatement ("the routes where you did supply one … the advice
// that names it follows"), then as the precise-but-unpinned replacement. Each
// time it was prose asserting a property of the code with nothing checking it,
// which is the exact defect class #374 and #391 exist to remove. Measured: a
// fourth `attachRejectionAdvice` call site on rm-screenshot's error path left
// the whole suite green while both the row and the comment silently became
// false.
//
// So the property is pinned here, from the OUTSIDE: every change route is driven
// through the real command tree to a real 400, and the number of lines the user
// actually gets is asserted. That is the thing a reader cares about — does
// advice follow this error? — rather than an implementation detail like how many
// call sites `attachRejectionAdvice` has, which can be true while the rendered
// output is not.
func TestListingChangeRouteAdviceLedger(t *testing.T) {
	const caption = "MY-CAPTION-ZZZ"

	for _, tc := range []struct {
		name string
		// bad is the proc whose 400 the user is meant to see.
		bad string
		// live drives the approved-listing branch (the revision routes).
		live bool
		// wantLines is the WHOLE error, in lines, as rendered to the user.
		wantLines int
		// why records what those lines are, so a diff is readable.
		why  string
		args func(t *testing.T, dir string) []string
	}{
		{
			name: "set-icon", bad: "setIcon", wantLines: 7,
			why: "the 400 + `what was sent:` + the 5-line icon re-encode paragraph",
			args: func(t *testing.T, dir string) []string {
				return []string{"app", "listing", "set-icon", img(t, dir, "icon.png")}
			},
		},
		{
			name: "set-cover", bad: "setCover", wantLines: 3,
			why: "the 400 + `what was sent:` + the README pointer (no re-encode paragraph off the icon path)",
			args: func(t *testing.T, dir string) []string {
				return []string{"app", "listing", "set-cover", img(t, dir, "cover.png")}
			},
		},
		{
			name: "add-screenshot", bad: "addScreenshot", wantLines: 3,
			why: "same as set-cover — and note the advice names the FILE, never the --caption",
			args: func(t *testing.T, dir string) []string {
				return []string{"app", "listing", "add-screenshot", img(t, dir, "shot.png"), "--caption", caption}
			},
		},
		{
			name: "rm-screenshot", bad: "removeScreenshot", wantLines: 1,
			why:  "no advice: app_listing.go returns this error bare",
			args: func(*testing.T, string) []string { return []string{"app", "listing", "rm-screenshot", "alsc_1"} },
		},
		{
			name: "reorder", bad: "reorderScreenshots", wantLines: 1,
			why:  "no advice: app_listing.go returns this error bare",
			args: func(*testing.T, string) []string { return []string{"app", "listing", "reorder", "alsc_1", "alsc_2"} },
		},
		{
			name: "beginListingRevision", bad: "beginListingRevision", live: true, wantLines: 1,
			why: "no advice, and nothing the author sent to advise about — civitai/cli#391's whole point",
			args: func(t *testing.T, dir string) []string {
				return []string{"app", "listing", "set-icon", img(t, dir, "icon.png"), "--changelog", "cl"}
			},
		},
		{
			name: "submitListingRevision", bad: "submitListingRevision", live: true, wantLines: 1,
			why: "no advice: the revision submit returns bare",
			args: func(t *testing.T, dir string) []string {
				return []string{"app", "listing", "set-icon", img(t, dir, "icon.png"), "--changelog", "cl"}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := "draft"
			if tc.live {
				status = "approved"
			}
			srv := listingAdviceStub(t, tc.bad, status)
			defer srv.Close()
			t.Setenv("CIVITAI_TOKEN", "tok")
			t.Setenv("CIVITAI_BASE_URL", srv.URL)

			dir := t.TempDir()
			root := NewRootCmd()
			var sink strings.Builder
			root.SetOut(&sink)
			root.SetErr(&sink)
			root.SetArgs(append(tc.args(t, dir), "--slug", "my-app"))

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected a 400 from %s", tc.bad)
			}
			msg := err.Error()

			// Reachability: this must be THIS route's 400, not an earlier
			// refusal that happens to also be one line long.
			if !strings.Contains(msg, "REFUSED-BY-"+tc.bad) {
				t.Fatalf("the error did not come from %s (no unique marker): %s", tc.bad, msg)
			}
			if got := len(strings.Split(strings.TrimRight(msg, "\n"), "\n")); got != tc.wantLines {
				t.Errorf("`civitai app listing %s` renders %d line(s), want %d (%s).\n"+
					"This number is PUBLISHED: README's `store-listing change (400)` Troubleshooting row and "+
					"listingError's change-arm comment both enumerate which routes print follow-on advice.\n"+
					"If you added or removed an attachRejectionAdvice call site, update BOTH of those and this "+
					"ledger together — prose asserting this property has been wrong twice already.\n--- got ---\n%s",
					tc.name, got, tc.wantLines, tc.why, msg)
			}
			// The advice names the FILE, never the caption. Both the README row
			// and the code comment say so in as many words.
			if strings.Contains(msg, caption) {
				t.Errorf("the --caption reached the error text; the advice is measured from the FILE only:\n%s", msg)
			}
		})
	}
}

// img writes a tiny valid PNG and returns its path.
func img(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, pngBytes(t, 64, 64), 0o644); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return p
}

// listingAdviceStub answers every step of the media flow benignly except the one
// proc under test, which 400s with a marker unique to it.
func listingAdviceStub(t *testing.T, badProc, listingStatus string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		write := func(s string) { _, _ = w.Write([]byte(s)) }
		switch {
		case strings.HasSuffix(p, "."+badProc), strings.Contains(p, "/"+badProc):
			w.WriteHeader(http.StatusBadRequest)
			write(`{"error":{"json":{"message":"REFUSED-BY-` + badProc + `"}}}`)
		case strings.Contains(p, "blocks/submissions"):
			write(`{"submissions":[{"id":"sub_1","blockId":"my-app","status":"approved","appBlockId":"ab_1"}]}`)
		case strings.Contains(p, "getMyListingForApp"):
			write(`{"result":{"data":{"json":{"appListingId":"apl_1","status":"` + listingStatus +
				`","contentRating":"pg","hasPendingRevision":false}}}}`)
		case strings.Contains(p, "image-upload"):
			write(`{"id":"key","uploadURL":"http://` + r.Host + `/put/key"}`)
		case strings.HasPrefix(p, "/put/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(p, "ingestAssetFromDataUri"), strings.Contains(p, "persistAssetImage"):
			write(`{"result":{"data":{"json":{"imageId":501}}}}`)
		case strings.Contains(p, "beginListingRevision"):
			write(`{"result":{"data":{"json":{"shadowId":"shadow_1","created":true}}}}`)
		case strings.Contains(p, "getAssetScanStatuses"):
			write(`{"result":{"data":{"json":{"statuses":[{"imageId":501,"status":"scanned"}]}}}}`)
		case strings.Contains(p, "submitListingRevision"):
			write(`{"result":{"data":{"json":{"publishRequestId":"pr_1","shadowId":"shadow_1","slug":"my-app"}}}}`)
		default:
			write(`{"result":{"data":{"json":{}}}}`)
		}
	}))
}
