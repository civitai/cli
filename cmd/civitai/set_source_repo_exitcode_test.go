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
	"os"
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
		//
		// 🔴 THE FIXTURES ARE THE SERVER'S EXACT STRINGS, read at civitai
		// origin/main. Two of them were paraphrases once, and the fragments
		// asserted here did not occur in the real messages at all ("material
		// change"; "this deployment yet" for "this environment yet"). Nothing
		// failed, because the CLI does not branch on message text on this path —
		// which is exactly what makes a paraphrase dangerous: it reads as a
		// record of what the server says while being fiction.
		//
		// ⚠ AND THIS CHECK IS A PASS-THROUGH CONTROL, NOT A SERVER CLAIM. The
		// fragment is a substring of the fixture this test itself sends, so it
		// proves the message SURVIVES to the user, not that the server sends it.
		// The claim about the server is the STATUS chain in the header.
		wantStderr string
		// readmeRow is the label used by the README table this pins.
		readmeRow string
	}{
		{
			name:   "owner-unpublished blocks a material change",
			status: http.StatusBadRequest,
			msg: "sourceRepoUrl cannot be changed while this listing is unpublished, because that " +
				"change needs moderator review and an unpublished listing has no way to reach it. " +
				"Republish the listing first, then edit that field — the edit will be staged for " +
				"review. Tagline, description and category can be edited now.",
			want:       2,
			wantStderr: "cannot be changed while this listing is unpublished",
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
			name:   "source-repo column migration not applied",
			status: http.StatusPreconditionFailed,
			msg: "The source repository link is not available on this environment yet. " +
				"Leave the field empty and try again later.",
			want:       1,
			wantStderr: "not available on this environment yet",
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

	// 🔴 AND THE README IS READ, NOT RESTATED. The ledger above closes one
	// direction — the code moves, the test goes red. It leaves the ORIGINAL
	// failure mode open: someone edits the table wrongly, or updates these rows
	// and not the prose. Demonstrated during review — editing the table to say
	// exit 9 left all 21 packages green, because nothing read it.
	//
	// The sibling `app_listing_lookup_exitcode_test.go` declares its cited set
	// rather than reading the doc, and says so; four other tests in this repo DO
	// read the README (readme_troubleshooting_test.go, whoami_test.go,
	// download_example_id_test.go, app_dev_tunnel_cmd_test.go). This follows the
	// closed form, because the whole reason this file exists is that a README
	// exit-code claim was wrong and nothing noticed.
	readme := readRepoREADME(t)
	for _, tc := range rows {
		// Each table row is `| <label…> | `<code>` |`. Finding the label and then
		// the code on the SAME line is what ties the two together; searching the
		// whole file for the code would pass on any other row that happens to
		// carry it.
		line := readmeTableLine(t, readme, tc.readmeRow)
		if want := fmt.Sprintf("`%d`", tc.want); !strings.Contains(line, want) {
			t.Errorf("README publishes %q with a different exit code than this file measures (%d).\n"+
				"README line: %s\n"+
				"Update whichever is wrong — the measurement is this file's, so if the code really "+
				"moved, the prose is the stale side.", tc.readmeRow, tc.want, strings.TrimSpace(line))
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

// readRepoREADME reads the published README from the repo root.
//
// 🔴 POSITIVE CONTROL ON THE READ ITSELF. An unreadable or truncated README
// would make every assertion above pass by finding nothing to disagree with —
// the reassuring-zero shape. The length floor is deliberately far below the real
// file (~180 KB) so it cannot go stale, while still failing on an empty or
// stub read.
func readRepoREADME(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read README.md: %v", err)
	}
	if len(raw) < 10_000 {
		t.Fatalf("CONTROL failure, not a finding: README.md is %d bytes, far below the published "+
			"file — the assertions below would pass by reading nothing", len(raw))
	}
	return string(raw)
}

// readmeTableLine returns the single README line carrying label, failing if it
// is absent or ambiguous.
//
// 🔴 EXACTLY ONE MATCH IS REQUIRED. Zero means the table was reworded and this
// file is now pinning prose that does not exist; more than one means the label
// is not specific enough to tie a code to a state, and a check that cannot tell
// two rows apart is not a check.
func readmeTableLine(t *testing.T, readme, label string) string {
	t.Helper()
	var hits []string
	for _, line := range strings.Split(readme, "\n") {
		if strings.Contains(line, label) && strings.HasPrefix(strings.TrimSpace(line), "|") {
			hits = append(hits, line)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0]
	case 0:
		t.Fatalf("no README table row contains %q — the table was reworded and this file still "+
			"pins the old text, so nothing is checking the exit codes any more", label)
	default:
		t.Fatalf("%d README table rows contain %q; the label must identify ONE state or a code "+
			"cannot be tied to it", len(hits), label)
	}
	return ""
}
