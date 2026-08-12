package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#400. A LIVE listing that is still below the publish floor cannot
// have a revision SUBMITTED — the server refuses `submitListingRevision` with a
// 400 naming what is missing. Until this change the CLI returned that 400 as the
// command's error, so `set-icon` reported a failure having just succeeded: the
// icon was attached to the shadow revision, the scan was clean, and the only
// thing that did not happen was the submit, which could not have happened yet
// BY DESIGN. #186's own framing is that CLI-authored apps are born below the
// floor, so this is the ordinary first media command on such an app once it is
// live, and the natural scripted form breaks on it:
//
//	civitai app listing set-icon icon.png && civitai app listing set-cover cover.png
//
// The floor takes TWO commands to clear, so the first one must not be reported
// as a failure for the floor not yet being cleared.
//
// 🔴 THE DISCRIMINATOR IS THE LISTING'S STATE, NEVER THE SERVER'S PROSE, and the
// three cases below are what pin that. Matching the 400's message text would be
// a spelled guard: it goes quietly false when the server rewords, and it fails
// in the REASSURING direction — a genuine rejection would start exiting 0. So
// the CLI re-reads the listing and asks two structural questions instead: did
// the asset I just attached land, and is the floor still unmet? Progress is
// reported only when BOTH hold, which is why "attach did not land" and "floor is
// met" below must both still be errors.
func TestSetIconBelowFloorOnLiveListing(t *testing.T) {
	for _, tc := range []struct {
		name string
		// iconLanded / coverPresent drive what the post-submit re-read reports.
		iconLanded, coverPresent bool
		// readFails makes the post-submit re-read itself fail.
		readFails bool
		// wantErr is whether the user gets an error at all.
		wantErr bool
		// why records what this case is defending.
		why string
	}{
		{
			name: "staged below floor is progress", iconLanded: true, coverPresent: false, wantErr: false,
			why: "the reported bug: the icon landed and the floor is genuinely unmet, so nothing failed",
		},
		{
			name: "attach did not land is a failure", iconLanded: false, coverPresent: false, wantErr: true,
			why: "the floor is unmet but the icon is NOT there — the submit 400 is the only thing the user has, and swallowing it would report success for an attach that did not happen",
		},
		{
			name: "floor met is a failure", iconLanded: true, coverPresent: true, wantErr: true,
			why: "the floor cannot be why this was refused, so the 400 is a genuine rejection and must survive",
		},
		{
			// 🔴 This case exists because its absence was MEASURED, not imagined.
			// Flipping the read-failure guard to fail-OPEN (`return true`, i.e.
			// claim progress when the CLI cannot see the listing at all) left the
			// ENTIRE internal/cmd package green — the guard was unreachable from
			// any test, so the three cases above certified a branch nothing
			// exercised. The discriminator's whole authority is the re-read; with
			// no re-read there is no evidence the attach landed, and reporting
			// success on no evidence is the bug this change exists to remove.
			name: "unreadable listing is a failure", iconLanded: true, coverPresent: false, readFails: true, wantErr: true,
			why: "the re-read failed, so the CLI cannot show the attach landed and must not claim it did",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := belowFloorStub(t, tc.iconLanded, tc.coverPresent, tc.readFails)
			defer srv.Close()
			listingEnv(t, srv.URL)

			dir := t.TempDir()
			root := NewRootCmd()
			var sink strings.Builder
			root.SetOut(&sink)
			root.SetErr(&sink)
			root.SetArgs([]string{"app", "listing", "set-icon", img(t, dir, "icon.png"),
				"--slug", "my-app", "--changelog", "cl"})

			err := root.Execute()
			out := sink.String()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error (%s)\noutput:\n%s", tc.why, out)
				}
				// Item 7: the exit code is pinned by errors.Is, never by the
				// message — TagStatus attaches ErrBadRequest while Error() stays
				// byte-for-byte unchanged, so asserting the text would say
				// nothing about what a script sees. A 400 the CLI passes through
				// is exit 2 (usage), and it must STAY exit 2 here.
				if !errors.Is(err, civitai.ErrBadRequest) {
					t.Errorf("the surviving 400 lost its classification: want civitai.ErrBadRequest, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected success (%s), got: %v\noutput:\n%s", tc.why, err, out)
			}
			// The message may not say the icon is "set": it is staged on a
			// revision that is deliberately NOT submitted, and the LIVE listing
			// is still unchanged. Claiming otherwise is the same class of false
			// report as the exit code this test exists to fix.
			for _, want := range []string{"staged", "not submitted", "cover"} {
				if !strings.Contains(out, want) {
					t.Errorf("output does not say %q — the user has to learn the revision is open and what it still needs:\n%s", want, out)
				}
			}
			if strings.Contains(out, "pending moderator review") {
				t.Errorf("output claims a moderator review that was never opened:\n%s", out)
			}
		})
	}
}

// belowFloorStub answers the whole live-listing set-icon flow, refusing ONLY
// submitListingRevision — with the server's real below-floor message — and
// reporting the post-attach listing state the case under test asks for.
func belowFloorStub(t *testing.T, iconLanded, coverPresent, readFails bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(p, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "approved",
				"contentRating": "g", "hasPendingRevision": false})
		case strings.Contains(p, "ingestAssetFromDataUri"):
			trpcData(w, map[string]any{"imageId": 501})
		case strings.Contains(p, "beginListingRevision"):
			trpcData(w, map[string]any{"shadowId": "shadow_1", "created": true})
		case strings.Contains(p, "setIcon"):
			trpcData(w, map[string]any{"status": "attached", "iconId": 501})
		case strings.Contains(p, "submitListingRevision"):
			// The server's own wording, verbatim from the #400 report.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"Listing needs at least an icon and cover before it can be published (missing: cover)."}}}`))
		case strings.Contains(p, "getMyListingForEdit"):
			if readFails {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"boom"}}}`))
				return
			}
			icon := map[string]any{"imageId": nil}
			if iconLanded {
				icon = map[string]any{"imageId": 501}
			}
			cover := map[string]any{"imageId": nil}
			if coverPresent {
				cover = map[string]any{"imageId": 777}
			}
			trpcData(w, map[string]any{"parentId": "listing_1", "status": "approved",
				"assets": map[string]any{"icon": icon, "cover": cover, "screenshots": []any{}}})
		default:
			t.Errorf("unexpected path %s", p)
		}
	}))
}
