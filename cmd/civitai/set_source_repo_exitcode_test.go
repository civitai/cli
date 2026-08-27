package main

// THE PUBLISHED EXIT CODES FOR `civitai app listing set-source-repo`'s REFUSED
// STATES.
//
// 🔴 THIS EXISTS BECAUSE THE README GOT THEM WRONG AND NOTHING NOTICED. The
// "Link your source code" section shipped a sentence saying two refused states
// "both exit `1`". Measured, the three are 2 / 3 / 1 — wrong on two of three, in
// a repo where AGENTS item 7 makes exit codes an `errors.Is` contract. It was
// caught by an audit reading the classifier, not by a test, because a README
// sentence is prose and prose does not run.
//
// That is the same failure `app_listing_lookup_exitcode_test.go` was written for
// one command over, and this file follows it deliberately: build the REAL binary
// and read the PROCESS EXIT STATUS, rather than asserting a classification that
// `exitCode` might route somewhere else.
//
// The chain each row exercises, read at civitai `origin/main`:
//
//	owner-unpublished + a material field
//	    offsite-listing.service.ts   -> MATERIAL_CHANGE_BLOCKED
//	    app-listings.router.ts       -> mapOffsiteError -> BAD_REQUEST -> 400
//	    pkg/civitai/errkind.go       -> ErrBadRequest
//	    cmd/civitai/main.go          -> exitUsage        = 2
//
//	moderator-removed
//	                                 -> FORBIDDEN        -> 403
//	    pkg/civitai/errkind.go       -> ErrUnauthorized
//	    cmd/civitai/main.go          -> exitAuth          = 3
//
//	source-repo column migration not applied
//	    app-listing-source-repo.service.ts -> PRECONDITION_FAILED -> 412
//	    pkg/civitai/errkind.go       -> (unclassified)
//	    cmd/civitai/main.go          -> exitGeneric       = 1
//
// `sourceRepoUrl` is in MATERIAL_PATCH_FIELDS, so the first row's refusal really
// does apply to this command's patch and is not borrowed from another field.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetSourceRepoRefusalExitCodes(t *testing.T) {
	bin := buildCLI(t)

	const slug = "zzz-srcrepo-fixture"

	rows := []struct {
		name string
		// status is what appListings.updateListing answers with.
		status int
		msg    string
		// want is the MEASURED process exit status.
		want int
		// wantStderr is a fragment of the message the user actually sees.
		wantStderr string
		// readmeRow is the label used by the README table this pins.
		readmeRow string
	}{
		{
			name:       "owner-unpublished blocks a material change",
			status:     http.StatusBadRequest,
			msg:        "cannot make a material change while the listing is unpublished",
			want:       2,
			wantStderr: "material change",
			readmeRow:  "you unpublished the listing yourself",
		},
		{
			name:       "moderator-removed listing",
			status:     http.StatusForbidden,
			msg:        "this listing has been removed by a moderator and can no longer be edited",
			want:       3,
			wantStderr: "removed by a moderator",
			readmeRow:  "a moderator removed the listing",
		},
		{
			name:       "source-repo column migration not applied",
			status:     http.StatusPreconditionFailed,
			msg:        "The source repository link is not available on this deployment yet",
			want:       1,
			wantStderr: "not available on this deployment yet",
			readmeRow:  "the platform has not yet applied the migration",
		},
	}

	// 🔴 THE PROVENANCE LEDGER, in the shape the sibling file established. The
	// README publishes exactly these three rows; declaring them here and checking
	// against the ROWS (not against what a run happened to execute) means a `-run`
	// filter cannot make this pass by excluding the case it is counting.
	if len(rows) != 3 {
		t.Fatalf("the README table publishes 3 refused states; this file has %d rows", len(rows))
	}
	seen := map[int]bool{}
	for _, r := range rows {
		if seen[r.want] {
			t.Fatalf("two rows claim exit %d — the table's whole point is that the three "+
				"states do NOT share a code, so a duplicate means one row is mislabelled", r.want)
		}
		seen[r.want] = true
	}
	for _, want := range []int{1, 2, 3} {
		if !seen[want] {
			t.Errorf("no row measures exit %d, which the README table publishes", want)
		}
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "listMine"):
					// OFFSITE, so the kind gate lets the write through — the
					// point of these rows is the WRITE's refusal, not the gate's.
					_, _ = fmt.Fprintf(w,
						`{"result":{"data":{"json":[{"appListingId":"apl_SR1","slug":%q,"kind":"offsite"}]}}}`, slug)
				case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
					// Empty, so resolveListing takes the by-slug fallback.
					_, _ = w.Write([]byte(`{"submissions":[]}`))
				case strings.Contains(r.URL.Path, "getMyListingForApp"):
					_, _ = fmt.Fprintf(w,
						`{"result":{"data":{"json":{"appListingId":"apl_SR1","status":"approved",`+
							`"contentRating":"g","hasPendingRevision":false,"shadowId":null,`+
							`"editTargetId":"apl_SR1","editBlockedReason":null}}}}`)
				case strings.Contains(r.URL.Path, "updateListing"):
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprintf(w, `{"error":{"json":{"message":%q}}}`, tc.msg)
				default:
					// Nothing else may be reached. A plausible body would let a
					// stray call pass unnoticed; 418 cannot.
					w.WriteHeader(http.StatusTeapot)
				}
			}))
			defer srv.Close()

			rc, stderr := runCLI(t, bin, []string{
				"XDG_CONFIG_HOME=" + filepath.Join(t.TempDir(), "config"),
				"CIVITAI_TOKEN=tok-1",
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_NO_UPDATE_CHECK=1",
			}, "app", "listing", "set-source-repo", "https://github.com/o/r", "--slug", slug)

			if rc != tc.want {
				t.Errorf("exit status %d, want %d — the README publishes %q as exit %d\nstderr: %s",
					rc, tc.want, tc.readmeRow, tc.want, stderr)
			}
			// The server's own words must survive to the user; a code with a
			// message about something else is a worse answer than either alone.
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr must carry the server's own refusal %q; got:\n%s", tc.wantStderr, stderr)
			}
			// 🔴 NONE of these is the ON-SITE verdict. That refusal is also exit 1,
			// so without this the 412 row would pass against a CLI that had
			// mistaken an offsite listing for an onsite one and never sent the
			// write at all.
			if strings.Contains(stderr, "ON-SITE app") {
				t.Errorf("a %d from the write must not surface as the on-site kind verdict:\n%s",
					tc.status, stderr)
			}
		})
	}
}
