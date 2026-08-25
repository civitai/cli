package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#374. `listingOp` used to be chosen at the three CALL SITES, from
// the HTTP verb: trpcQuery → read, trpcMutation → change, MintImageUpload →
// upload. Two ingest routes are POSTs, so both landed on "change" and a 400
// from `appListings.ingestAssetFromDataUri` (an oversize icon — a
// first-attempt failure, the server caps the data URI at ~2 MiB) told the user
// the CLI's "store-listing change" was rejected and to go look at
// `civitai app listing status`, having changed no listing at all.
//
// The mirror direction is worse and was equally unpinned: mutating
// trpcMutation's constant to listingOpRead re-labels every setIcon / setCover /
// addScreenshot / submitListingRevision 400 as a LOOKUP that "changed nothing",
// about a mutation that may have partially applied. Measured before this file
// existed, that mutant survived `./internal/appapi` (235 tests) AND
// `./internal/cmd` — no test in the repo could see it, because the only
// per-route reachability test covered ONE route.
//
// The fix is structural: the op is a field of `listingRoute`, stated once where
// the route is declared. That makes the classification uniform, but it cannot
// make it CORRECT — a struct literal type-checks past a wrong op just as
// happily. The tests below are what pin correctness, and they are behavioural:
// each drives a real client method against an httptest server that answers 400
// on exactly one route, so a route whose op is wrong (or which is never
// reached) fails here.

// listingRouteCase drives ONE client method to a 400 on ONE route.
type listingRouteCase struct {
	name string
	// route is the route expected to answer 400.
	route listingRoute
	// wantOp is what the route DOES, asserted INDEPENDENTLY of route.op.
	//
	// 🔴 Reading route.op here instead would make this whole file vacuous: flip
	// an op in listing.go and the expectation flips with it. Measured — a first
	// draft did exactly that, and mutating getMyListingForEdit to
	// listingOpChange and submitListingRevision to listingOpRead both SURVIVED
	// the full 252-test package. wantOp is the second, hand-written opinion the
	// declaration is checked against.
	wantOp listingOp
	// serverMsg is unique per case, so a case observing another case's error —
	// or the wrong branch of a multi-step flow — cannot pass by coincidence.
	serverMsg string
	call      func(context.Context, *Client) error
}

func opName(op listingOp) string {
	switch op {
	case listingOpUnclassified:
		return "unclassified"
	case listingOpRead:
		return "read"
	case listingOpChange:
		return "change"
	case listingOpIngest:
		return "ingest"
	}
	return "op(" + strconv.Itoa(int(op)) + ")"
}

// listingRouteCases covers EVERY route the listing-media client can call. The
// input values are pairwise distinct (no two listing ids, image ids or captions
// repeat) so a wrong-argument bug cannot look right by collision.
func listingRouteCases() []listingRouteCase {
	return []listingRouteCase{
		{
			name: "getMyListingForApp", route: trpcGetMyListingForApp, wantOp: listingOpRead, serverMsg: "refused: entry lookup",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetMyListingForApp(ctx, "", "slug-alpha")
				return err
			},
		},
		{
			name: "getMyListingForEdit", route: trpcGetMyListingForEdit, wantOp: listingOpRead, serverMsg: "refused: edit view",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetMyListingForEdit(ctx, "apl_edit_101")
				return err
			},
		},
		{
			name: "getAssetScanStatuses", route: trpcGetAssetScanStatuses, wantOp: listingOpRead, serverMsg: "refused: scan poll",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.GetAssetScanStatuses(ctx, []int{202})
				return err
			},
		},
		{
			// Step 1 of the full-res upload. Not a listing write.
			name: "imageUploadMint", route: imageUploadRoute, wantOp: listingOpIngest, serverMsg: "refused: mint",
			call: func(ctx context.Context, c *Client) error {
				_, _, err := c.MintImageUpload(ctx)
				return err
			},
		},
		{
			// 🔴 A POST that creates an Image row and attaches it to nothing —
			// the route whose 400 `civitai app listing set-icon` shows first.
			name: "ingestAssetFromDataUri", route: trpcIngestAssetFromDataURI, wantOp: listingOpIngest, serverMsg: "refused: data uri too large",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.IngestAssetFromDataURI(ctx, []byte("ICONBYTES"), "image/png")
				return err
			},
		},
		{
			// 🔴 Step 3 of the SAME user action as imageUploadMint above.
			name: "persistAssetImage", route: trpcPersistAssetImage, wantOp: listingOpIngest, serverMsg: "refused: persist",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.IngestAssetFullRes(ctx, []byte("COVERBYTES"), ImageInfo{Width: 1600, Height: 900, MimeType: "image/jpeg"})
				return err
			},
		},
		{
			// The ownership-and-seats enumeration `set-text` reads for the
			// listing KIND. It takes no input and opens no shadow — a lookup in
			// the strictest sense available here.
			name: "listMine", route: trpcListMine, wantOp: listingOpRead, serverMsg: "refused: listing enumeration",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ListMyListings(ctx)
				return err
			},
		},
		{
			// The scalar TEXT write. A change like the attaches below it: it
			// writes the listing row, so a 400 may have partially applied.
			name: "updateListing", route: trpcUpdateListing, wantOp: listingOpChange, serverMsg: "refused: scalar text write",
			call: func(ctx context.Context, c *Client) error {
				// All three keys, so the authorControlledFields ledger is checked
				// against a request that really carries each one. The values are
				// pairwise distinct and distinct from every other fixture string.
				tag, desc, cat := "a new tagline", "a new description", "utility"
				_, err := c.UpdateListing(ctx, "apl_text_909", ListingTextPatch{
					Tagline: &tag, Description: &desc, Category: &cat,
				})
				return err
			},
		},
		{
			name: "setIcon", route: trpcSetIcon, wantOp: listingOpChange, serverMsg: "refused: icon attach",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.SetIcon(ctx, "apl_icon_303", 3031)
				return err
			},
		},
		{
			name: "setCover", route: trpcSetCover, wantOp: listingOpChange, serverMsg: "refused: cover attach",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.SetCover(ctx, "apl_cover_404", 4041)
				return err
			},
		},
		{
			name: "addScreenshot", route: trpcAddScreenshot, wantOp: listingOpChange, serverMsg: "refused: screenshot append",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.AddScreenshot(ctx, "apl_shot_505", 5051, "caption-echo")
				return err
			},
		},
		{
			name: "removeScreenshot", route: trpcRemoveScreenshot, wantOp: listingOpChange, serverMsg: "refused: screenshot remove",
			call: func(ctx context.Context, c *Client) error {
				return c.RemoveScreenshot(ctx, "shot_606")
			},
		},
		{
			name: "reorderScreenshots", route: trpcReorderScreenshots, wantOp: listingOpChange, serverMsg: "refused: reorder",
			call: func(ctx context.Context, c *Client) error {
				return c.ReorderScreenshots(ctx, "apl_order_707", []string{"shot_7071", "shot_7072"})
			},
		},
		{
			name: "beginListingRevision", route: trpcBeginListingRevision, wantOp: listingOpChange, serverMsg: "refused: begin revision",
			call: func(ctx context.Context, c *Client) error {
				_, _, err := c.BeginListingRevision(ctx, "apl_begin_808")
				return err
			},
		},
		{
			name: "submitListingRevision", route: trpcSubmitListingRevision, wantOp: listingOpChange, serverMsg: "refused: submit revision",
			call: func(ctx context.Context, c *Client) error {
				_, err := c.SubmitListingRevision(ctx, "shadow_909", "changelog-nine")
				return err
			},
		},
	}
}

// reqLog records what the client actually sent, in order.
type reqLog struct {
	paths  []string
	method map[string]string // path -> the verb it was requested with
	body   map[string][]byte // path -> the request body it was sent with
}

func (l *reqLog) saw(path string) bool {
	for _, p := range l.paths {
		if p == path {
			return true
		}
	}
	return false
}

// listingStubServer answers 400 on badPath and something benign everywhere
// else, so a multi-step flow reaches the step under test. Every request is
// recorded in log.
func listingStubServer(badPath, badMsg string, log *reqLog) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.paths = append(log.paths, r.URL.Path)
		if log.method == nil {
			log.method = map[string]string{}
		}
		log.method[r.URL.Path] = r.Method
		if log.body == nil {
			log.body = map[string][]byte{}
		}
		if raw, err := io.ReadAll(r.Body); err == nil {
			log.body[r.URL.Path] = raw
		}
		switch {
		case r.URL.Path == badPath:
			trpcErr(w, http.StatusBadRequest, badMsg)
		case r.URL.Path == ImageUploadPath:
			// Plain REST, not a tRPC envelope.
			_, _ = w.Write([]byte(`{"id":"key-uuid","uploadURL":"http://` + r.Host + `/put/key-uuid"}`))
		case strings.HasPrefix(r.URL.Path, "/put/"):
			w.WriteHeader(http.StatusOK)
		default:
			// A non-empty result object decodes into every listing struct.
			trpcOK(w, map[string]any{})
		}
	}))
}

// valueBlame is the remedy the change arm carried until civitai/cli#391, kept
// as a PROHIBITION on the read and ingest arms rather than deleted with it.
// #374's ingest arm is exactly the one that used to tell two stories, one of
// which was this phrase; substituting the new remedy for it in these notWant
// sets — instead of adding alongside — would have retired that guard silently.
const valueBlame = "fix the value"

// exactForOp is the WHOLE message each 400 arm must produce, byte for byte,
// once the server's own reason is spliced in.
//
// 🔴 Substring want/notWant checks pin PRESENCE, and presence cannot exclude an
// ADDED clause. Measured: `… — the change may have partially applied — amend the
// parameter and retry; …` carries every required phrase, blames a value that was
// never sent, and passed the whole package green. A spelled list of blame
// phrasings is not a fix for that, because the next synonym is always outside
// the list. Equality is: there is exactly one sentence each arm may say, and
// anything appended, inserted or re-ordered fails here regardless of wording.
//
// This is the guard to update when an arm is deliberately re-worded — and
// updating it is the moment to re-read whether the new sentence is TRUE of every
// route that reaches the arm, which is the mistake #391 was filed for.
func exactForOp(op listingOp, serverMsg string) string {
	switch op {
	case listingOpRead:
		return "the server rejected this store-listing lookup (400): " + serverMsg +
			// 🔴 RE-READ AGAINST EVERY ROUTE THAT REACHES THIS ARM, which is
			// what this guard's own doc comment asks for. There are now FOUR,
			// and the fourth (listMine) takes NO input — so the old "check the
			// app you named" clause named a value one of its four callers never
			// sends. The sentence below is true of all four: it asserts only
			// that nothing changed, and points at a command that PRINTS the
			// valid app names rather than presuming the caller typed one.
			// (`civitai app doctor` ships in the sibling PR — see the merge-order
			// note on refuseOnsiteTextEdit.)
			" — nothing was changed; `civitai app doctor` lists every app you can work on"
	case listingOpIngest:
		return "the server rejected the image-upload request (400): " + serverMsg +
			" — no listing was changed; check the image and retry"
	case listingOpChange:
		return "the server rejected this store-listing change (400): " + serverMsg +
			" — the change may have partially applied; `civitai app listing status` shows the listing as it stands"
	}
	return ""
}

// wantForOp is the pinned WORDING of each 400 arm — literal strings, not
// derived from listing.go. want must all appear, notWant must not.
func wantForOp(op listingOp) (want, notWant []string) {
	switch op {
	case listingOpRead:
		return []string{"store-listing lookup", "nothing was changed"},
			[]string{"store-listing change", "image-upload", "partially applied", valueBlame}
	case listingOpIngest:
		return []string{"image-upload request", "no listing was changed"},
			[]string{"store-listing change", "store-listing lookup", "partially applied", valueBlame}
	case listingOpChange:
		// 🔴 "fix the value" is in notWant, not in want, and civitai/cli#391 is
		// why: this ONE arm answers for all seven change routes, and
		// `beginListingRevision` sends only a CLI-minted listing id, so that
		// remedy named a value its author never sent. What the arm may claim is
		// what is true of every change route — the write may have landed in
		// part. See listingError's change arm for the per-route derivation.
		return []string{"store-listing change", "may have partially applied", "civitai app listing status"},
			[]string{"nothing was changed", "image-upload", "store-listing lookup", valueBlame}
	}
	return nil, nil
}

// TestListingBadRequestSubjectIsReachablePerRoute generalises
// TestListingReadBadRequestIsReachableAsARead to EVERY route: each client
// method is driven to a real 400 and the subject it produces is checked against
// its route's op. A structural check (op is a struct field) type-checks past a
// wrong op; this is what does not.
func TestListingBadRequestSubjectIsReachablePerRoute(t *testing.T) {
	cases := listingRouteCases()
	if len(cases) < 15 {
		// civitai/cli#391 N3: the floor is a positive control against a table
		// that quietly emptied, NOT a claim that fifteen is forever. Retiring a
		// route is a legitimate reason to see fourteen, so the message says which
		// edit is expected instead of only reporting the count.
		t.Fatalf("positive control: this table drives %d routes, and %d were expected.\n"+
			"If a route was RETIRED, lower this floor in the same commit that deletes its case, "+
			"its entry in TestListingRouteOpsAreExactlyThisSpelledSet and its row in procname_leak_test.go. "+
			"If nothing was retired, the table lost a case.", len(cases), 15)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The stub 400s whatever tc.route.path says, so without this the
			// case would still "pass" if the route it names had the wrong
			// PATH — every assertion below would then be true of the wrong
			// route. The case name IS the proc name, so pin them equal.
			if strings.HasPrefix(tc.route.path, "/api/trpc/") {
				if want := "appListings." + tc.name; tc.route.name() != want {
					t.Fatalf("case %q is wired to route %q (proc %q), want proc %q", tc.name, tc.route.path, tc.route.name(), want)
				}
			} else if tc.route.path != ImageUploadPath {
				t.Fatalf("case %q is wired to non-tRPC route %q, want %q", tc.name, tc.route.path, ImageUploadPath)
			}

			var log reqLog
			srv := listingStubServer(tc.route.path, tc.serverMsg, &log)
			defer srv.Close()

			err := tc.call(context.Background(), New(srv.URL, "tok", ""))
			if err == nil {
				t.Fatalf("expected a 400 error from %s", tc.route.path)
			}
			// Reachability: the 400 must have come from the route under test,
			// not from an earlier local guard or an earlier step.
			if !log.saw(tc.route.path) {
				t.Fatalf("%s was never requested (paths hit: %v) — the error came from somewhere else: %v", tc.route.path, log.paths, err)
			}
			// 🔴 The one cross-check that owes NOTHING to wantOp: the verb the
			// client actually sent. The verb does not decide the op (both ingest
			// routes are POSTs and are not changes) but it does rule one out —
			// a route the CLI sends as a POST is never a lookup. This is what
			// stops a "read" being talked into the wrong column in BOTH the
			// declaration and this table at once, which is the direction that
			// tells a user nothing changed when something may have.
			if got := log.method[tc.route.path]; tc.route.op == listingOpRead && got != http.MethodGet {
				t.Errorf("route %s is classified a lookup but the CLI sends it as %s — a %s can change data", tc.route.path, got, got)
			} else if tc.route.op != listingOpRead && got == http.MethodGet {
				t.Errorf("route %s is sent as GET but classified %s", tc.route.path, opName(tc.route.op))
			}
			msg := err.Error()
			// The unique server reason proves this is THIS route's 400.
			if !strings.Contains(msg, tc.serverMsg) {
				t.Fatalf("error does not carry this route's own reason %q: %s", tc.serverMsg, msg)
			}
			// The declaration is checked against this file's independent
			// opinion of what the route does, not the other way round.
			if tc.route.op != tc.wantOp {
				t.Errorf("route %s is declared %s, but it does a %s", tc.route.path, opName(tc.route.op), opName(tc.wantOp))
			}
			want, notWant := wantForOp(tc.wantOp)
			if len(want) == 0 {
				t.Fatalf("case %s wants op %s, which has no pinned wording", tc.name, opName(tc.wantOp))
			}
			for _, w := range want {
				if !strings.Contains(msg, w) {
					t.Errorf("missing %q; got: %s", w, msg)
				}
			}
			for _, w := range notWant {
				if strings.Contains(msg, w) {
					t.Errorf("must not claim %q; got: %s", w, msg)
				}
			}
			// 🔴 The pin the two loops above CANNOT be: they check presence and
			// absence of phrases, so a clause appended to an otherwise-correct
			// sentence satisfies both. Equality admits exactly one sentence per
			// arm. See exactForOp.
			if exact := exactForOp(tc.wantOp, tc.serverMsg); msg != exact {
				t.Errorf("route %s does not produce its arm's exact message.\n  got:  %s\n  want: %s\n"+
					"If you re-worded this arm on purpose, update exactForOp — and while you are there, "+
					"confirm the new sentence is true of EVERY route that reaches the arm (civitai/cli#391).",
					tc.route.path, msg, exact)
			}
			// Item 7: the published exit code is pinned by errors.Is, never by
			// message text — re-wording a 400 must not move it off exit 2.
			if !errors.Is(err, civitai.ErrBadRequest) {
				t.Errorf("a 400 must stay tagged ErrBadRequest, got %T: %v", err, err)
			}
		})
	}
}

// authorControlledFields is civitai/cli#391's ledger: for each CHANGE route,
// the fields of the request whose value the author can change by re-running the
// command differently. Everything else in the payload is minted by the CLI from
// a lookup that already succeeded, and is therefore not something a rejection
// can sensibly ask them to "fix".
//
// Derived per route from the call sites in internal/cmd/app_listing.go, NOT
// inherited from the #386 review that assumed all seven carried one:
//
//   - setIcon / setCover: `imageId` is minted by the CLI, but it is the ingest
//     of the FILE the author named positionally, and the server validates that
//     image's geometry/aspect/MIME/bytes at attach — a different file gives a
//     different verdict, so it is author-controlled.
//   - addScreenshot: the same, plus `caption` from --caption.
//   - removeScreenshot: `screenshotId` is argv (`rm-screenshot alsc_…`).
//   - reorderScreenshots: `orderedIds` is argv.
//   - beginListingRevision: NOTHING. `listingId` comes from resolveListing, a
//     getMyListingForApp that returned 200 before this call was made.
//   - submitListingRevision: `changelog` only, and only when the author passed
//     --changelog; under --yes (or a TTY "y") confirmLiveRevision mints the
//     default "Update listing <kind> via civitai CLI" and the author supplied
//     nothing either.
var authorControlledFields = map[string][]string{
	// 🔴 `patch`, NOT `listingId` — and NOT the patch's inner keys. This ledger
	// walks the request's TOP-LEVEL fields, which is the right granularity for
	// the question it asks (does the change arm's remedy presume a value the
	// author supplied?) and the reason the entry is one name rather than three.
	// The whole of `patch` is author-typed: every key inside it comes from a
	// `--tagline` / `--description` / `--category` / `--clear` flag, so the answer
	// is the same for all of them. The listing id is CLI-minted from a lookup that
	// already returned 200, exactly like `beginListingRevision`'s.
	"updateListing":         {"patch"},
	"setIcon":               {"imageId"},
	"setCover":              {"imageId"},
	"addScreenshot":         {"imageId", "caption"},
	"removeScreenshot":      {"screenshotId"},
	"reorderScreenshots":    {"orderedIds"},
	"beginListingRevision":  {},
	"submitListingRevision": {"changelog"},
}

// cliMintedFields is the other half of the same ledger — payload fields the
// author never types. Together the two must cover every key the wire carries,
// so a field added to a change route cannot slip in unclassified and quietly
// move the premise this test's conclusion rests on.
//
// 🔴 Keyed PER ROUTE, deliberately. A flat set of field names would be a
// route-agnostic silencer: adding one entry to shut up the route you are working
// on would also pre-authorise that field on the other six, so the next route to
// grow it would pass unnoticed. The whole point of this ledger is that a field
// appearing where it did not before is loud.
var cliMintedFields = map[string][]string{
	"updateListing":         {"listingId"},
	"setIcon":               {"listingId"},
	"setCover":              {"listingId"},
	"addScreenshot":         {"listingId"},
	"removeScreenshot":      {},
	"reorderScreenshots":    {"listingId"},
	"beginListingRevision":  {"listingId"},
	"submitListingRevision": {"shadowId"},
}

// TestChangeArmPresumesNoUserSuppliedValue is civitai/cli#391's regression
// guard, and it pins a RELATIONSHIP rather than a component: what the CLI SENDS
// on a change route against what the CLI is allowed to SAY when that route
// 400s.
//
// The defect: the change arm advised "fix the value and retry", which presumes
// the request carried a value the author supplied. Measured from the wire,
// `beginListingRevision` carries none — so a 400 opening a shadow revision told
// the author to go fix something they never sent. That is #374's wrong-subject
// class, in the one arm #374 left alone.
//
// 🔴 It reads the PAYLOAD, not the wording, to establish the premise. A test
// that only grepped the message for "fix the value" would pass the day someone
// re-words it to "correct what you passed"; and it would say nothing about WHY
// the phrase is wrong. This one fails if the premise moves in either direction:
// a change route growing an author-controlled field, or a field appearing in
// neither half of the ledger.
func TestChangeArmPresumesNoUserSuppliedValue(t *testing.T) {
	// Selected by the table's INDEPENDENT opinion of what each route does, not
	// by route.op — reading route.op here would make the selection follow the
	// declaration this file exists to check.
	var changeRoutes int
	valueless := map[string]bool{}

	for _, tc := range listingRouteCases() {
		if tc.wantOp != listingOpChange {
			continue
		}
		changeRoutes++
		t.Run(tc.name, func(t *testing.T) {
			ledger, ok := authorControlledFields[tc.name]
			if !ok {
				t.Fatalf("change route %q has no entry in authorControlledFields — "+
					"record which of its request fields the author can change by re-running the command, "+
					"then re-read listingError's change arm: its wording may no longer be true", tc.name)
			}

			var log reqLog
			srv := listingStubServer(tc.route.path, tc.serverMsg, &log)
			defer srv.Close()
			err := tc.call(context.Background(), New(srv.URL, "tok", ""))
			if err == nil {
				t.Fatalf("expected a 400 from %s", tc.route.path)
			}
			raw, ok := log.body[tc.route.path]
			if !ok {
				t.Fatalf("no request body was recorded for %s (paths hit: %v) — the instrument saw nothing", tc.route.path, log.paths)
			}
			// tRPC mutations send `{"json": <input>}`.
			var env struct {
				JSON map[string]json.RawMessage `json:"json"`
			}
			if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil || env.JSON == nil {
				t.Fatalf("could not read %s's request payload (%v): %s", tc.name, jsonErr, string(raw))
			}

			// Bidirectional: every key on the wire is classified, and every
			// classified author-controlled key is really on the wire.
			minted := cliMintedFields[tc.name]
			for key := range env.JSON {
				if containsStr(minted, key) {
					continue
				}
				if !containsStr(ledger, key) {
					t.Errorf("%s sends field %q, which is in neither half of civitai/cli#391's ledger.\n"+
						"Classify it: author-controlled (authorControlledFields) or CLI-minted (cliMintedFields). "+
						"If it is author-controlled, the change arm may be able to say more than it does today; "+
						"if it is CLI-minted, it must not.", tc.name, key)
				}
			}
			for _, key := range ledger {
				if _, sent := env.JSON[key]; !sent {
					// addScreenshot's caption and submitListingRevision's
					// changelog are omitted when empty; the cases pass both, so
					// this is a real absence.
					t.Errorf("%s is recorded as carrying the author-controlled field %q, but the request did not send it: %s",
						tc.name, key, string(raw))
				}
			}
			if len(ledger) == 0 {
				valueless[tc.name] = true
				// The whole point, asserted on the route that proves it: this
				// 400 may not blame a value, because none was sent.
				msg := err.Error()
				// 🔴 A SPELLED list, and it is defeatable by the next synonym —
				// this comment used to claim "a sentence cannot both satisfy
				// [the required phrases] and be a bare 'go fix your value'",
				// which is simply false: `… may have partially applied — amend
				// the parameter and retry; …` did both and the package stayed
				// green. What actually forecloses an added clause is the
				// EQUALITY pin in TestListingBadRequestSubjectIsReachablePerRoute
				// (see exactForOp); this list survives only because it names the
				// specific hazard when it fires, which an equality diff does
				// not. Treat it as a message, not as the guard.
				for _, blame := range []string{
					"fix the value", "the value you", "check the value",
					"correct the value", "what you passed", "you supplied",
					"what you sent", "your input", "the value and retry",
				} {
					if strings.Contains(strings.ToLower(msg), blame) {
						t.Errorf("%s sends only CLI-minted fields (%v), but its 400 blames a value the author never supplied (%q):\n  %s",
							tc.name, keysOfRaw(env.JSON), blame, msg)
					}
				}
				// What it must say instead stays pinned, so "say nothing" is
				// not an accepted way to satisfy the check above.
				for _, keep := range []string{"store-listing change", "may have partially applied", "civitai app listing status"} {
					if !strings.Contains(msg, keep) {
						t.Errorf("%s's 400 lost %q — the arm still has to name the operation, the partial-write risk and the next command:\n  %s",
							tc.name, keep, msg)
					}
				}
			}
		})
	}

	// Positive control on the selection: a filter that matched nothing would
	// make every subtest above vacuous.
	if changeRoutes != 8 {
		t.Errorf("expected 8 change routes to check, got %d — if a change route was added or retired, "+
			"give it an authorControlledFields entry and update this count in the same commit", changeRoutes)
	}
	// The conclusion the change arm's wording rests on. If this ever goes empty
	// — because every change route grew an author-supplied field — the arm MAY
	// legitimately advise fixing a value again, and this is where to re-derive
	// it rather than assume.
	if len(valueless) == 0 {
		t.Errorf("no change route sends a payload of only CLI-minted fields any more.\n" +
			"civitai/cli#391 removed \"fix the value and retry\" from the change arm BECAUSE " +
			"`beginListingRevision` was such a route. Re-derive the wording against the call sites " +
			"before restoring that advice — do not restore it just because this check went quiet.")
	}
}

func keysOfRaw(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestUnclassifiedRouteClaimsNothing pins the zero value. `listingRoute{path:
// "…"}` is a keyed literal Go accepts with no op, and if the zero op meant
// `read` that route would ship telling every user "nothing was changed" —
// #374's dangerous direction, re-created by an omission rather than a mistake.
// The arm it lands on instead must assert nothing about the listing.
func TestUnclassifiedRouteClaimsNothing(t *testing.T) {
	if (listingRoute{}).op == listingOpRead {
		t.Fatal("the zero listingOp must not be `read` — a route declared without an op would claim nothing was changed")
	}
	forgotten := listingRoute{path: "/api/trpc/appListings.hypotheticalNewProc"}
	body := []byte(`{"error":{"json":{"message":"Invalid input"}}}`)
	err := listingError(http.StatusBadRequest, body, forgotten)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, forbidden := range []string{
		"nothing was changed", "no listing was changed",
		"store-listing change", "store-listing lookup",
		// The change arm's remedy, whichever wording it carries. "fix the
		// value" was it until civitai/cli#391; "partially applied" is it now.
		// Both stay listed: an unclassified route may claim NEITHER, and a
		// forbidden phrase that no arm can produce is a dead assertion, so the
		// live one has to be the one in the code today.
		valueBlame, changeRemedy,
	} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("an unclassified route must claim nothing about the listing, but says %q: %s", forbidden, msg)
		}
	}
	if !strings.Contains(msg, "Invalid input") {
		t.Errorf("the server's own reason must survive: %s", msg)
	}
	if !strings.Contains(msg, forgotten.name()) {
		t.Errorf("an unclassified route is a CLI bug — name the call so it can be reported: %s", msg)
	}
	if !errors.Is(err, civitai.ErrBadRequest) {
		t.Errorf("still a 400, so still ErrBadRequest: %T %v", err, err)
	}
}

// TestIngestFullResTellsOneStoryAtEitherStep is civitai/cli#374's sharpest
// case. ONE user action (`app listing set-cover`) runs mint → PUT → persist. A
// 400 at step 1 and a 400 at step 3 used to produce contradictory errors: "no
// listing was changed" versus "fix the value and retry; `civitai app listing
// status` shows the listing as it stands" (the change arm's wording of that
// day — civitai/cli#391 has since replaced "fix the value and retry"; the quote
// is kept because it is what the bug printed). Neither step touches a listing,
// so both must say the same thing.
func TestIngestFullResTellsOneStoryAtEitherStep(t *testing.T) {
	tell := func(badPath, badMsg string) string {
		t.Helper()
		var log reqLog
		srv := listingStubServer(badPath, badMsg, &log)
		defer srv.Close()
		_, err := New(srv.URL, "tok", "").IngestAssetFullRes(
			context.Background(), []byte("COVERBYTES"), ImageInfo{Width: 1600, Height: 900, MimeType: "image/jpeg"})
		if err == nil {
			t.Fatalf("expected a 400 when %s fails", badPath)
		}
		if !log.saw(badPath) {
			t.Fatalf("%s was never requested (hit: %v)", badPath, log.paths)
		}
		return err.Error()
	}

	atMint := tell(ImageUploadPath, "refused: mint step")
	atPersist := tell(trpcPersistAssetImage.path, "refused: persist step")

	for _, phrase := range []string{"image-upload request", "no listing was changed"} {
		if !strings.Contains(atMint, phrase) {
			t.Errorf("mint-step 400 missing %q: %s", phrase, atMint)
		}
		if !strings.Contains(atPersist, phrase) {
			t.Errorf("persist-step 400 missing %q: %s", phrase, atPersist)
		}
	}
	if strings.Contains(atPersist, "store-listing change") {
		t.Errorf("the persist step changed no listing: %s", atPersist)
	}
	// Strip each step's own server reason; what is left is the CLI's story and
	// it must be identical at both steps.
	if a, b := strings.Replace(atMint, "refused: mint step", "", 1), strings.Replace(atPersist, "refused: persist step", "", 1); a != b {
		t.Errorf("one action, two stories:\n  mint:    %s\n  persist: %s", a, b)
	}
}

// TestEveryListingRouteHasAReachabilityCase is the bidirectional ledger between
// the routes DECLARED in listing.go and the routes the behavioural table above
// actually drives. Adding a route without a case (the shape that hid #374 —
// classification stated in one place, checked nowhere) fails here; so does a
// case naming a route that no longer exists.
func TestEveryListingRouteHasAReachabilityCase(t *testing.T) {
	declared := declaredListingRoutes(t)

	// Positive control on the PARSER: a walker that silently matches nothing
	// would make this test pass with an empty ledger. civitai/cli#391 N3: a
	// count short of 15 also happens when a route is legitimately retired, so
	// the message names that reading too rather than only blaming the sweep.
	if len(declared) < 15 {
		t.Fatalf("the sweep found %d listingRoute declarations in %s, and %d were expected.\n"+
			"If a route was RETIRED, lower this floor in the commit that removes it. "+
			"Otherwise the sweep is not reading what it thinks it is.", len(declared), mustGetwd(t), 15)
	}
	for _, must := range []string{"/api/trpc/appListings.setIcon", ImageUploadPath} {
		if _, ok := declared[must]; !ok {
			t.Fatalf("the sweep did not find %q among %v — instrument is wrong", must, keysOf(declared))
		}
	}

	exercised := map[string]bool{}
	for _, tc := range listingRouteCases() {
		var log reqLog
		srv := listingStubServer(tc.route.path, tc.serverMsg, &log)
		if err := tc.call(context.Background(), New(srv.URL, "tok", "")); err == nil {
			t.Errorf("%s: expected a 400", tc.name)
		}
		srv.Close()
		for _, p := range log.paths {
			exercised[p] = true
		}
	}

	for p, d := range declared {
		if !exercised[p] {
			t.Errorf("route %q (declared at %s) has no case in listingRouteCases() driving a 400 through it — its op is unpinned", p, d.file)
		}
	}
	for p := range exercised {
		if _, ok := declared[p]; strings.HasPrefix(p, "/api/") && !ok {
			t.Errorf("a case reached %q, which is not a declared listingRoute", p)
		}
	}
}

// TestListingRouteOpsAreExactlyThisSpelledSet is the THIRD, independent opinion
// on the classification — and the one a coordinated edit cannot quietly satisfy.
//
// 🔴 Why it exists: `listingRouteCases()`'s `wantOp` is checked against
// `route.op`, so editing BOTH re-labels a route with a green suite. Six of the
// seven `change` routes were vulnerable to exactly that. This ledger is read
// from the SOURCE TEXT by the AST parser and compared to a set spelled out here,
// in a different file from `wantOp`, so a re-label has to be contradicted in
// three places — and each of those places states why it matters.
//
// 🔴 What that is NOT, corrected by civitai/cli#391 N1. This used to claim a
// re-label "has to be written down three times before anything goes green",
// which asserted that each of the three guards is individually
// tamper-resistant. It is not. This one reads source TEXT, so it is defeatable
// on its own terms: a decoy literal for a path that is already declared used to
// SHADOW the real declaration (last-write-wins), and a route declared through a
// type alias used to be invisible entirely. Both are closed now — duplicates
// are reported, and the type set is resolved rather than spelled — but the
// honest statement of the guarantee is that the three guards are independent
// IN PRACTICE, defeating them together takes three coordinated edits in two
// files, and no one of them is claimed to be un-defeatable alone. If you find a
// fourth way past this one, that is a finding, not a paradox.
//
// If you are here because you deliberately re-classified a route: this ledger is
// not the thing to fix first. Fix the wording tests, confirm the new subject is
// TRUE of that route (a `change` that 400s may have partially applied; an
// `ingest` and a `read` may not claim it did), and only then record it here.
func TestListingRouteOpsAreExactlyThisSpelledSet(t *testing.T) {
	// Spelled out, deliberately not derived from the route vars.
	want := map[string]string{
		"/api/trpc/appListings.getMyListingForApp":   "listingOpRead",
		"/api/trpc/appListings.getMyListingForEdit":  "listingOpRead",
		"/api/trpc/appListings.getAssetScanStatuses": "listingOpRead",

		"/api/v1/image-upload":                         "listingOpIngest",
		"/api/trpc/appListings.ingestAssetFromDataUri": "listingOpIngest",
		"/api/trpc/appListings.persistAssetImage":      "listingOpIngest",

		"/api/trpc/appListings.listMine":              "listingOpRead",
		"/api/trpc/appListings.updateListing":         "listingOpChange",
		"/api/trpc/appListings.setIcon":               "listingOpChange",
		"/api/trpc/appListings.setCover":              "listingOpChange",
		"/api/trpc/appListings.addScreenshot":         "listingOpChange",
		"/api/trpc/appListings.removeScreenshot":      "listingOpChange",
		"/api/trpc/appListings.reorderScreenshots":    "listingOpChange",
		"/api/trpc/appListings.beginListingRevision":  "listingOpChange",
		"/api/trpc/appListings.submitListingRevision": "listingOpChange",
	}
	declared := declaredListingRoutes(t)
	// 🔴 civitai/cli#391 N3: this used to be `len(declared) < len(want)` and
	// FATAL, which made the actionable branch below — the one that names the
	// route and says what to do about it — unreachable in the exact situation it
	// was written for. Retiring a route legitimately shrinks `declared` below
	// `len(want)`, so a maintainer deleting a retired route got "the instrument
	// is under-reading", which blames the parser, and the test aborted before it
	// could say WHICH route.
	//
	// The floor it replaces still has a job — a walker wired to nothing must not
	// look like a clean run — but the honest threshold for "wired to nothing" is
	// zero, and zero does not move when a route is retired. Under-reading short
	// of zero is diagnosed by the loop below, which now names both mechanisms
	// instead of assuming one.
	if len(declared) == 0 {
		t.Fatalf("the sweep found NO listingRoute declarations at all in %s — the instrument is wired to nothing, "+
			"so every comparison below would be vacuous", mustGetwd(t))
	}
	for path, wantOp := range want {
		d, ok := declared[path]
		if !ok {
			// 🔴 An absence cannot distinguish its two mechanisms, so this names
			// both rather than guessing: the route was retired, or the parser
			// stopped seeing a declaration that is still there. The found set is
			// printed because that is the signal the two disagree about — a
			// plausible list one short reads "retired"; a list missing unrelated
			// routes reads "instrument".
			t.Errorf("this ledger names route %q, but the sweep found no declaration of it.\n"+
				"  If the platform RETIRED the route: delete this entry, its case in listingRouteCases() "+
				"and its row in procname_leak_test.go, in one commit.\n"+
				"  If the route IS still declared: the sweep stopped seeing it — fix declaredListingRoutes, do not delete the entry.\n"+
				"  The sweep found %d routes: %v", path, len(declared), keysOf(declared))
			continue
		}
		if d.opName != wantOp {
			got := d.opName
			if got == "" {
				got = "(no op given)"
			}
			t.Errorf("route %q is declared %s at %s, but this ledger says %s.\n"+
				"A wrong op makes the CLI state something false about the user's listing after a 400: "+
				"a `change` may have partially applied, an `ingest` and a `read` may not claim it did.",
				path, got, d.file, wantOp)
		}
	}
	for path, d := range declared {
		if _, ok := want[path]; !ok {
			got := d.opName
			if got == "" {
				got = "(no op given)"
			}
			t.Errorf("route %q (declared %s at %s) is not in this ledger — classify it here, and give it a case in listingRouteCases()", path, got, d.file)
		}
	}
}

func keysOf(m map[string]declaredRoute) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// declaredRoute is one `listingRoute{…}` composite literal as WRITTEN in the
// source: its path, and the identifier its op was spelled with ("" when the op
// was left off entirely).
type declaredRoute struct {
	path   string
	opName string
	file   string
}

// declaredListingRoutes parses EVERY non-test .go file in this package and
// returns one entry per `listingRoute{…}` composite literal. Reading the SOURCE,
// not a hand-kept list, is what makes the ledger notice a route added tomorrow —
// and it reads the whole package, not just listing.go, because a route declared
// in a sibling file of the same package is exactly as reachable and would
// otherwise be invisible to the one guard whose job is to see it.
func declaredListingRoutes(t *testing.T) map[string]declaredRoute {
	t.Helper()
	fset := token.NewFileSet()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	var files []*ast.File
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		// civitai/cli#391 N4: ask the BUILD what is in this package, not the
		// filename. `parser.ParseFile` happily reads a file behind a
		// `//go:build never` constraint or a `_windows.go` suffix, so a route
		// declared in a file that is not compiled would be counted as declared
		// and would then demand a reachability case for a route no caller can
		// reach. MatchFile applies the same constraints the toolchain does.
		// (Over-demanding is the safe direction, which is why this was 🟢 — but
		// the honest sweep is the one that matches the build.)
		switch ok, err := build.Default.MatchFile(".", n); {
		case err != nil:
			t.Fatalf("deciding whether %s is in this build: %v", n, err)
		case !ok:
			// 🔴 Excluding the file trades an over-demand for a possible SILENT
			// UNDER-READ, and under-reading is the direction that hid #374. On
			// linux a route in `foo_windows.go` would simply vanish from every
			// ledger with no diagnostic. So: skipping is fine only for a file
			// that cannot matter. If an excluded file so much as mentions the
			// type, say so and fail rather than quietly seeing less.
			if src, readErr := os.ReadFile(n); readErr == nil && strings.Contains(string(src), "listingRoute") {
				t.Errorf("%s is excluded from this build (GOOS=%s GOARCH=%s) but mentions listingRoute.\n"+
					"This sweep can only see the files the current build selects, so any route declared there is "+
					"invisible on this platform — the ledgers would under-read with no diagnostic at all.\n"+
					"Remedy: declare listing routes only in unconstrained files, or teach this sweep to parse "+
					"each build configuration you ship.", n, build.Default.GOOS, build.Default.GOARCH)
			}
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", n, err)
		}
		files = append(files, f)
		names = append(names, n)
	}
	// Positive control on the file sweep itself: a wrong cwd or a filter that
	// matches nothing would make every ledger below vacuous.
	if len(files) < 2 {
		t.Fatalf("the package sweep found %d non-test .go files in %q — it is not reading what it thinks", len(files), mustGetwd(t))
	}

	// String consts anywhere in the package, so a route declared as
	// `listingRoute{ImageUploadPath, …}` resolves.
	consts := map[string]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, n := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
						if v, err := strconv.Unquote(bl.Value); err == nil {
							consts[n.Name] = v
						}
					}
				}
			}
		}
	}

	// resolvePath reads a path expression: a string literal, or an identifier
	// naming a string const in this package.
	resolvePath := func(e ast.Expr, where string) (string, bool) {
		switch v := e.(type) {
		case *ast.BasicLit:
			if v.Kind != token.STRING {
				break
			}
			s, err := strconv.Unquote(v.Value)
			if err != nil {
				break
			}
			return s, true
		case *ast.Ident:
			if s, ok := consts[v.Name]; ok {
				return s, true
			}
			t.Errorf("%s: listingRoute path %s is not a string const in this package.\n"+
				"Remedy: give the route a literal path, or declare the const in %s.",
				where, v.Name, "internal/appapi")
			return "", false
		}
		t.Errorf("%s: this ledger cannot read a listingRoute path written as %T.\n"+
			"Remedy: write the path as a string literal or as an identifier naming a string const in this package — "+
			"or teach resolvePath in listing_op_test.go to read this form. Do NOT delete the ledger: "+
			"it is the only guard that notices a route added with no reachability case.", where, e)
		return "", false
	}

	// civitai/cli#391 N2: the walk used to match the identifier SPELLING
	// "listingRoute", so `type lroute = listingRoute` made every route declared
	// through the alias invisible — no ledger entry, no reachability case
	// demanded, no wording row demanded, and a fully GREEN silent suite. That is
	// the only defeat found that produced no diagnostic at all, and "silent" is
	// the property that made #374 expensive.
	//
	// Resolving through go/types would be the textbook answer; it would mean
	// type-checking the package, and x/tools is a dependency this repo has not
	// taken (AGENTS.md: ask first). The cheap equivalent is to collect the names
	// that ARE listingRoute — the alias form `type X = listingRoute` and the
	// defined form `type X listingRoute` — and match against that set rather
	// than against one spelling.
	//
	// 🔴 To a FIXPOINT, not one hop: `type lr2 = lr1` where `lr1 = listingRoute`
	// is a chain, and a single pass would resolve lr1 and stay blind to lr2 —
	// silently, which is the property that made this worth closing at all.
	// Iterating until the set stops growing is bounded by the number of type
	// declarations in the package.
	routeTypeNames := map[string]bool{"listingRoute": true}
	for grew := true; grew; {
		grew = false
		for _, f := range files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					id, ok := ts.Type.(*ast.Ident)
					if !ok || !routeTypeNames[id.Name] || routeTypeNames[ts.Name.Name] {
						continue
					}
					routeTypeNames[ts.Name.Name] = true
					grew = true
				}
			}
		}
	}
	// isRouteType reports whether an expression names listingRoute (or anything
	// declared as it), through a pointer if need be.
	var isRouteType func(ast.Expr) bool
	isRouteType = func(e ast.Expr) bool {
		switch v := e.(type) {
		case *ast.Ident:
			return routeTypeNames[v.Name]
		case *ast.StarExpr:
			return isRouteType(v.X)
		}
		return false
	}
	// isRouteContainer reports whether an expression is a slice/array/map whose
	// elements bottom out in routes. Element literals may omit the type entirely
	// (`[]listingRoute{{"…", op}}`), so they are never seen by the type match
	// above — the same silent shape as the alias, reached a different way.
	//
	// Recursive, so `map[string][]listingRoute{…}` and deeper nestings are seen
	// too; a one-level check would have left exactly that shape silent.
	var isRouteContainer func(ast.Expr) bool
	isRouteContainer = func(e ast.Expr) bool {
		switch v := e.(type) {
		case *ast.ArrayType:
			return isRouteType(v.Elt) || isRouteContainer(v.Elt)
		case *ast.MapType:
			return isRouteType(v.Value) || isRouteContainer(v.Value)
		}
		return false
	}

	out := map[string]declaredRoute{}
	recorded := map[token.Pos]bool{}
	record := func(cl *ast.CompositeLit, fileName string) {
		if len(cl.Elts) == 0 || recorded[cl.Pos()] {
			return
		}
		recorded[cl.Pos()] = true
		where := fileName + ":" + strconv.Itoa(fset.Position(cl.Pos()).Line)

		var path, opName string
		var gotPath bool
		if _, keyed := cl.Elts[0].(*ast.KeyValueExpr); keyed {
			// `listingRoute{path: "…", op: listingOpChange}` — correct,
			// idiomatic Go, and the exact form listingOpUnclassified exists
			// for. Read it by key; a missing `op` key is what leaves opName
			// empty, which the op ledger reports.
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch key.Name {
				case "path":
					path, gotPath = resolvePath(kv.Value, where)
				case "op":
					if v, ok := kv.Value.(*ast.Ident); ok {
						opName = v.Name
					}
				}
			}
		} else {
			path, gotPath = resolvePath(cl.Elts[0], where)
			if len(cl.Elts) > 1 {
				if v, ok := cl.Elts[1].(*ast.Ident); ok {
					opName = v.Name
				}
			}
		}
		if !gotPath {
			return
		}
		// civitai/cli#391 N1: this ledger keys on the PATH, and the assignment
		// used to be last-write-wins. A second literal for the same route —
		// `var _ = listingRoute{"…setIcon", listingOpChange}` left further down
		// a file — therefore SHADOWED the real declaration, and the op ledger
		// read the decoy's classification instead of the one that ships. (The
		// mutant still died, via wantOp and the wording table; this is the third
		// guard being individually honest rather than leaning on the other two.)
		// A duplicate path is a defect in its own right regardless: two literals
		// for one route means two places to keep the op right.
		if prev, dup := out[path]; dup {
			t.Errorf("route %q is declared twice — at %s and at %s.\n"+
				"This ledger keys on the path, so the second literal SHADOWS the first and the op "+
				"reported for the route would be whichever the sweep happened to read last. "+
				"Delete the duplicate; a route's op must be stated exactly once.",
				path, prev.file, where)
			return
		}
		out[path] = declaredRoute{path: path, opName: opName, file: where}
	}

	// elemTypeOf gives a container's element type, so an element literal that
	// elides its own type still has one to be judged against.
	elemTypeOf := func(e ast.Expr) ast.Expr {
		switch v := e.(type) {
		case *ast.ArrayType:
			return v.Elt
		case *ast.MapType:
			return v.Value
		}
		return nil
	}
	// walkLit descends a literal against the type it is KNOWN to have, which is
	// the only way to read an element that spells no type of its own. Recursing
	// (rather than looking one level down) is what makes
	// `map[string][]listingRoute{…}` visible.
	var walkLit func(cl *ast.CompositeLit, typ ast.Expr, fileName string)
	walkLit = func(cl *ast.CompositeLit, typ ast.Expr, fileName string) {
		switch {
		case isRouteType(typ):
			record(cl, fileName)
		case isRouteContainer(typ):
			elem := elemTypeOf(typ)
			for _, elt := range cl.Elts {
				v := elt
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					v = kv.Value
				}
				inner, ok := v.(*ast.CompositeLit)
				if !ok {
					continue
				}
				it := inner.Type
				if it == nil {
					it = elem
				}
				walkLit(inner, it, fileName)
			}
		}
	}

	for i, f := range files {
		fileName := names[i]
		ast.Inspect(f, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok && cl.Type != nil {
				walkLit(cl, cl.Type, fileName)
			}
			return true
		})
	}
	return out
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
