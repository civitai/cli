package cmd

import (
	"errors"
	"fmt"
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
// 🔴 THE DISCRIMINATOR IS THE LISTING'S STATE, NEVER THE SERVER'S PROSE. Matching
// the 400's message text would be a spelled guard: it goes quietly false when the
// server rewords, and it fails in the REASSURING direction — a genuine rejection
// would start exiting 0. So the CLI asks three structural questions instead, and
// the table below is one case per way of answering them wrongly:
//
//  1. Was this even a REFUSAL? Only a 400 can be the floor talking; a 5xx says
//     nothing about the listing, and swallowing one hides an outage on the
//     busiest path this command has.
//  2. Did the asset I attached land — BY IDENTITY, not by the slot being
//     non-empty? A listing being re-branded already HAS an icon.
//  3. Is the floor still unmet, so that it can even BE the reason?
//
// Every "is a failure" row below is a mutation that would otherwise survive:
// each one is a state in which a plausible-looking implementation reports
// success while something actually went wrong.
func TestSetMediaBelowFloorOnLiveListing(t *testing.T) {
	const ingested = 501 // the image id every ingest in this test mints

	for _, tc := range []struct {
		name string
		// cmd is the media subcommand to drive.
		cmd string
		// submitStatus is the status submitListingRevision answers with.
		submitStatus int
		// viewIcon / viewCover are the image ids the post-submit re-read
		// reports in each floor slot; 0 means the slot is empty.
		viewIcon, viewCover int
		// viewShots are the screenshot ids the re-read reports.
		viewShots []string
		// attachedShotID is the id addScreenshot echoes back.
		attachedShotID string
		// echoID is the image id setIcon/setCover echo back; 0 means "echo the
		// ingested id", which is what a server that stores what it was given does.
		echoID int
		// readFails makes the post-submit re-read itself fail.
		readFails bool
		// wantErr is whether the user gets an error at all.
		wantErr bool
		// wantOut are fragments the success output must contain.
		wantOut []string
		// why records what this case is defending.
		why string
	}{
		{
			name: "icon staged below floor is progress",
			cmd:  "set-icon", submitStatus: 400, viewIcon: ingested,
			wantOut: []string{"staged", "not submitted", "Still required before publishing: cover.",
				"Next: civitai app listing set-cover <file>"},
			why: "the reported bug: the icon landed and the floor is genuinely unmet, so nothing failed",
		},
		{
			name: "cover staged below floor is progress",
			cmd:  "set-cover", submitStatus: 400, viewCover: ingested,
			wantOut: []string{"staged", "not submitted", "Still required before publishing: icon.",
				"Next: civitai app listing set-icon <file>"},
			why: "the same state reached from the other floor slot — and the next command must be the one that CLEARS the floor, not the one just run",
		},
		{
			name: "screenshot staged below floor is progress",
			cmd:  "add-screenshot", submitStatus: 400, attachedShotID: "alsc_9", viewShots: []string{"alsc_9"},
			wantOut: []string{"staged", "not submitted", "Still required before publishing: icon and cover.",
				"Next: civitai app listing set-icon <file>"},
			why: "screenshots are not part of the floor, so this kind can only be proven landed by id",
		},
		{
			name: "icon attach did not land is a failure",
			cmd:  "set-icon", submitStatus: 400, viewIcon: 0, wantErr: true,
			why: "the floor is unmet but the icon is NOT there — the submit 400 is the only thing the user has, and swallowing it would report success for an attach that did not happen",
		},
		{
			name: "icon slot holds a DIFFERENT image is a failure",
			cmd:  "set-icon", submitStatus: 400, viewIcon: 999, wantErr: true,
			why: "a live listing being re-branded already HAS an icon, so presence alone would prove the OLD image and report the new one staged while looking at it",
		},
		{
			name: "cover slot holds a DIFFERENT image is a failure",
			cmd:  "set-cover", submitStatus: 400, viewCover: 999, wantErr: true,
			why: "the same identity hole in the other slot — a per-kind check is wrong at one kind at a time",
		},
		{
			name: "screenshot id not in the listing is a failure",
			cmd:  "add-screenshot", submitStatus: 400, attachedShotID: "alsc_9", viewShots: []string{"alsc_OTHER"},
			wantErr: true,
			why:     "a screenshot that was already there is not the one this command added",
		},
		{
			name: "screenshot attach echoed no id is a failure",
			cmd:  "add-screenshot", submitStatus: 400, attachedShotID: "", viewShots: []string{"alsc_9"},
			wantErr: true,
			why:     "with no id from the attach there is nothing to match, so nothing can be proven landed",
		},
		{
			// The empty-id guard is NOT redundant with the loop below it, and
			// this is the case that proves it: drop the guard and an empty id
			// MATCHES an empty id, so a degenerate row would certify itself.
			// Without this fixture the mutant is equivalent and survives.
			name: "an empty id matching an empty row is still a failure",
			cmd:  "add-screenshot", submitStatus: 400, attachedShotID: "", viewShots: []string{""},
			wantErr: true,
			why:     "two absent ids are not a match — nothing is proven by comparing nothing to nothing",
		},
		{
			// 🔴 Pins the reason the ECHO is preferred over the ingested id. The
			// two are equal in every other fixture here, which makes "use the
			// echo" and "use what I ingested" indistinguishable — and an
			// indistinguishable pair is an untested claim, since the doc comment
			// asserts this survives the server storing a re-encoded derivative
			// under a NEW id. Here the attach echoes 888 for an ingest of 501.
			name: "the id the attach echoed wins over the ingested one",
			cmd:  "set-icon", submitStatus: 400, echoID: 888, viewIcon: 888,
			wantOut: []string{"staged", "not submitted", "Next: civitai app listing set-cover <file>"},
			why:     "the server's own statement of what it wrote is the authority, not the id the CLI sent",
		},
		{
			name: "floor met is a failure",
			cmd:  "set-icon", submitStatus: 400, viewIcon: ingested, viewCover: 777, wantErr: true,
			why: "the floor cannot be why this was refused, so the 400 is a genuine rejection and must survive",
		},
		{
			// 🔴 The widest "reports success while something went wrong" window,
			// and it is on the BUSIEST path: below-floor is the normal state of
			// every CLI-authored app's first media command, so gating on "the
			// submit errored at all" would make a platform outage invisible
			// exactly where it is most likely to be met.
			name: "a 500 on submit is a failure",
			cmd:  "set-icon", submitStatus: 500, viewIcon: ingested, wantErr: true,
			why: "a 5xx says nothing about the listing — only a refusal can be the floor talking",
		},
		{
			// Both 5xx shapes, because they are classified DIFFERENTLY and a
			// gate could plausibly pass one and swallow the other: 500 carries
			// no sentinel at all (exit 1), while 503 is tagged ErrNetwork
			// (exit 5, the code the README tells scripts to RETRY on). Getting
			// this wrong would tell a retry loop that an outage was a success.
			name: "a 503 on submit is a failure",
			cmd:  "set-icon", submitStatus: 503, viewIcon: ingested, wantErr: true,
			why: "the retryable outage shape must stay retryable",
		},
		{
			// 🔴 This case exists because its absence was MEASURED, not imagined.
			// Flipping the read-failure guard to fail-OPEN (`return true`, i.e.
			// claim progress when the CLI cannot see the listing at all) left the
			// ENTIRE internal/cmd package green — the guard was unreachable from
			// any test, so the cases above certified a branch nothing exercised.
			name: "unreadable listing is a failure",
			cmd:  "set-icon", submitStatus: 400, viewIcon: ingested, readFails: true, wantErr: true,
			why: "the re-read failed, so the CLI cannot show the attach landed and must not claim it did",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := belowFloorStub(t, belowFloorState{
				ingested: ingested, submitStatus: tc.submitStatus,
				icon: tc.viewIcon, cover: tc.viewCover, shots: tc.viewShots,
				attachedShotID: tc.attachedShotID, echoID: tc.echoID, readFails: tc.readFails,
			})
			defer srv.Close()
			listingEnv(t, srv.URL)

			dir := t.TempDir()
			root := NewRootCmd()
			var sink strings.Builder
			root.SetOut(&sink)
			root.SetErr(&sink)
			root.SetArgs([]string{"app", "listing", tc.cmd, img(t, dir, "img.png"),
				"--slug", "my-app", "--changelog", "cl"})

			err := root.Execute()
			out := sink.String()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error (%s)\noutput:\n%s", tc.why, out)
				}
				// Item 7: the exit code is pinned by errors.Is, never by the
				// message — TagStatus attaches the sentinel while Error() stays
				// byte-for-byte unchanged, so asserting the text would say
				// nothing about what a script sees. Every surviving error must
				// keep the exact classification it had before this path
				// existed: 400 → ErrBadRequest (exit 2), 503 → ErrNetwork
				// (exit 5, the retryable one), 500 → deliberately UNCLASSIFIED
				// (exit 1), which statusKind pins by mapping only 502/503/504.
				switch tc.submitStatus {
				case 400:
					if !errors.Is(err, civitai.ErrBadRequest) {
						t.Errorf("the surviving refusal lost ErrBadRequest (exit 2): %v", err)
					}
				case 503:
					if !errors.Is(err, civitai.ErrNetwork) {
						t.Errorf("the surviving outage lost ErrNetwork (exit 5, the retryable code): %v", err)
					}
				default:
					if errors.Is(err, civitai.ErrBadRequest) {
						t.Errorf("a %d was classified as a refusal: %v", tc.submitStatus, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("expected success (%s), got: %v\noutput:\n%s", tc.why, err, out)
			}
			// The message may not say the asset is "set": it is staged on a
			// revision that is deliberately NOT submitted, and the LIVE listing
			// is still unchanged. Claiming otherwise is the same class of false
			// report as the exit code this test exists to fix.
			for _, want := range tc.wantOut {
				if !strings.Contains(out, want) {
					t.Errorf("output is missing %q — the user has to learn the revision is open and what it still needs:\n%s", want, out)
				}
			}
			if strings.Contains(out, "pending moderator review") {
				t.Errorf("output claims a moderator review that was never opened:\n%s", out)
			}
		})
	}
}

// belowFloorState is what the stub should report for one case.
type belowFloorState struct {
	ingested       int
	submitStatus   int
	icon, cover    int
	shots          []string
	attachedShotID string
	echoID         int
	readFails      bool
}

// echo is the image id the attach reports writing: the explicit one when the
// case sets it, otherwise the id that was ingested.
func (st belowFloorState) echo() int {
	if st.echoID != 0 {
		return st.echoID
	}
	return st.ingested
}

// belowFloorStub answers the whole live-listing attach flow — both ingest paths,
// all three attaches — refusing ONLY submitListingRevision, and reporting the
// post-attach listing state the case under test asks for.
func belowFloorStub(t *testing.T, st belowFloorState) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(p, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "approved",
				"contentRating": "g", "hasPendingRevision": false})

		// Ingest: the inline data-URI path (icon) and the mint/put/persist path
		// (cover, screenshot) both mint the same image id.
		case strings.Contains(p, "ingestAssetFromDataUri"), strings.Contains(p, "persistAssetImage"):
			trpcData(w, map[string]any{"imageId": st.ingested})
		case strings.Contains(p, "image-upload"):
			// REST, not tRPC — this one is NOT wrapped in result.data.json.
			fmt.Fprintf(w, `{"id":"key","uploadURL":"http://%s/put/key"}`, r.Host)
		case strings.HasPrefix(p, "/put/"):
			w.WriteHeader(http.StatusOK)

		case strings.Contains(p, "beginListingRevision"):
			trpcData(w, map[string]any{"shadowId": "shadow_1", "created": true})
		case strings.Contains(p, "setIcon"):
			trpcData(w, map[string]any{"status": "attached", "iconId": st.echo()})
		case strings.Contains(p, "setCover"):
			trpcData(w, map[string]any{"status": "attached", "coverId": st.echo()})
		case strings.Contains(p, "addScreenshot"):
			trpcData(w, map[string]any{"status": "attached", "id": st.attachedShotID, "order": 0})

		case strings.Contains(p, "submitListingRevision"):
			w.WriteHeader(st.submitStatus)
			// The 400 body is the server's own wording, verbatim from #400.
			msg := "Listing needs at least an icon and cover before it can be published (missing: cover)."
			if st.submitStatus >= 500 {
				msg = "upstream unavailable"
			}
			fmt.Fprintf(w, `{"error":{"json":{"message":%q}}}`, msg)

		case strings.Contains(p, "getMyListingForEdit"):
			if st.readFails {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"boom"}}}`))
				return
			}
			shots := []any{}
			for i, id := range st.shots {
				shots = append(shots, map[string]any{"id": id, "imageId": 900 + i, "order": i})
			}
			trpcData(w, map[string]any{"parentId": "listing_1", "status": "approved",
				"assets": map[string]any{
					"icon":        slotJSON(st.icon),
					"cover":       slotJSON(st.cover),
					"screenshots": shots,
				}})
		default:
			t.Errorf("unexpected path %s", p)
		}
	}))
}

// slotJSON renders a floor slot: id 0 means empty.
func slotJSON(imageID int) map[string]any {
	if imageID == 0 {
		return map[string]any{"imageId": nil}
	}
	return map[string]any{"imageId": imageID}
}
