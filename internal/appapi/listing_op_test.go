package appapi

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
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

// wantForOp is the pinned WORDING of each 400 arm — literal strings, not
// derived from listing.go. want must all appear, notWant must not.
func wantForOp(op listingOp) (want, notWant []string) {
	switch op {
	case listingOpRead:
		return []string{"store-listing lookup", "nothing was changed"},
			[]string{"store-listing change", "image-upload"}
	case listingOpIngest:
		return []string{"image-upload request", "no listing was changed"},
			[]string{"store-listing change", "store-listing lookup", "fix the value"}
	case listingOpChange:
		return []string{"store-listing change", "fix the value", "civitai app listing status"},
			[]string{"nothing was changed", "image-upload", "store-listing lookup"}
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
	if len(cases) < 13 {
		t.Fatalf("positive control: expected the case table to cover every route, got %d", len(cases))
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
			// Item 7: the published exit code is pinned by errors.Is, never by
			// message text — re-wording a 400 must not move it off exit 2.
			if !errors.Is(err, civitai.ErrBadRequest) {
				t.Errorf("a 400 must stay tagged ErrBadRequest, got %T: %v", err, err)
			}
		})
	}
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
		"store-listing change", "store-listing lookup", "fix the value",
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
// status` shows the listing as it stands". Neither step touches a listing, so
// both must say the same thing.
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
	// would make this test pass with an empty ledger.
	if len(declared) < 13 {
		t.Fatalf("parser found only %d listingRoute declarations in %s — it is not reading what it thinks", len(declared), mustGetwd(t))
	}
	for _, must := range []string{"/api/trpc/appListings.setIcon", ImageUploadPath} {
		if _, ok := declared[must]; !ok {
			t.Fatalf("parser did not find %q among %v — instrument is wrong", must, keysOf(declared))
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
// in a different file from `wantOp`, so a re-label has to be written down three
// times before anything goes green — and each of those places states why it
// matters.
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

		"/api/trpc/appListings.setIcon":               "listingOpChange",
		"/api/trpc/appListings.setCover":              "listingOpChange",
		"/api/trpc/appListings.addScreenshot":         "listingOpChange",
		"/api/trpc/appListings.removeScreenshot":      "listingOpChange",
		"/api/trpc/appListings.reorderScreenshots":    "listingOpChange",
		"/api/trpc/appListings.beginListingRevision":  "listingOpChange",
		"/api/trpc/appListings.submitListingRevision": "listingOpChange",
	}
	declared := declaredListingRoutes(t)
	if len(declared) < len(want) {
		t.Fatalf("the parser found %d routes but this ledger names %d — the instrument is under-reading", len(declared), len(want))
	}
	for path, wantOp := range want {
		d, ok := declared[path]
		if !ok {
			t.Errorf("this ledger names route %q, which is no longer declared — delete the entry if the route is gone", path)
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

	out := map[string]declaredRoute{}
	for i, f := range files {
		fileName := names[i]
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			id, ok := cl.Type.(*ast.Ident)
			if !ok || id.Name != "listingRoute" || len(cl.Elts) == 0 {
				return true
			}
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
			if gotPath {
				out[path] = declaredRoute{path: path, opName: opName, file: where}
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
