package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#424 — the narrowed scope claim on `resolveListing`.
//
// The defect #424 reported was in PROSE: the doc comment said a pre-approval draft is
// "resolvable ONLY BY SLUG" with the slug argument "load-bearing when it is absent",
// which reads as a claim about how wide the SERVER's slug arm is. The CLI only ever
// had evidence for a claim about which SELECTOR it can send. This file holds the two
// things that outlive the wording: the wire-shape relationship the narrowed comment
// still rests on, and the corrected paragraphs themselves.

// TestListingLookupCarriesTheSlugOnBothArms pins the RELATIONSHIP the narrowed
// comment rests on: the slug goes on BOTH `getMyListingForApp` calls — the one that
// also carries the submission's appBlockId, and the one that carries nothing else.
//
// 🟡 THIS IS AN INVARIANT GUARD, NOT REGRESSION COVERAGE. The bug was in the prose;
// the code already sent the slug on both arms, so this is green at the base commit by
// construction and must not be counted as evidence that anything was broken. It is
// here because the old comment was the only thing calling the slug "load-bearing", and
// narrowing that comment removes the stated reason a future reader would keep the
// argument on the appBlockId arm.
//
// 🔴 NOTHING PINNED THE PAIR BEFORE — each arm was covered ALONE.
// TestOnsiteHappyPathMakesNoExtraCall asserts only that the appBlockId is present on
// the onsite arm and says nothing about the slug; TestAppListingPendingReachesDraftBySlug
// only ever exercises the arm where no appBlockId exists. Dropping `slug` from the
// appBlockId call left both of them green. So this is driven as ONE table over both
// arms, and the arm COUNT is asserted: the claim is about the set, and a third arm
// that quietly dropped the slug would otherwise not be seen by it.
func TestListingLookupCarriesTheSlugOnBothArms(t *testing.T) {
	cases := []struct {
		name       string
		appBlockID string // what the submission row carries
		wantBlock  bool   // must the lookup input carry an appBlockId key?
	}{
		{"submission HAS an appBlockId", "block_1", true},
		{"submission has NO appBlockId (pending first version)", "", false},
	}
	if len(cases) != 2 {
		t.Fatalf("resolveListing has exactly two lookup arms; the table covers %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var lookups int
			var input string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
					submissionRow(w, "my-app", tc.appBlockID)
				case strings.Contains(r.URL.Path, "getMyListingForApp"):
					lookups++
					input = r.URL.Query().Get("input")
					trpcData(w, listingRefBody("apl_1", "draft"))
				case strings.Contains(r.URL.Path, "getMyListingForEdit"):
					trpcData(w, map[string]any{
						"parentId": "apl_1", "slug": "my-app", "status": "draft",
						"hasPendingRevision": false, "shadowId": nil,
						"assets": map[string]any{
							"icon":        map[string]any{"imageId": 11, "url": "http://x/i.png"},
							"cover":       map[string]any{"imageId": 12, "url": "http://x/c.png"},
							"screenshots": []any{},
						},
					})
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			listingEnv(t, srv.URL)

			if _, _, err := run(t, "app", "listing", "status", "--slug", "my-app"); err != nil {
				t.Fatalf("PREMISE BROKEN — this arm must resolve: %v", err)
			}
			// PREMISE: the lookup really ran, exactly once. Without this the slug
			// assertion below is vacuous for a CLI that never asks.
			if lookups != 1 {
				t.Fatalf("getMyListingForApp must run exactly once on this arm, ran %d times", lookups)
			}
			// THE CLAIM: the slug is on this arm, whichever arm it is.
			if !strings.Contains(input, `"slug":"my-app"`) {
				t.Errorf("the slug must be sent on BOTH arms — a pre-approval draft has "+
					"appBlockId = NULL, so the slug is the only selector that can name it; input was %q", input)
			}
			// And the appBlockId is present exactly when the submission had one: an
			// empty id must be OMITTED, not sent as "", or the server's "either
			// appBlockId or slug" schema sees a supplied-but-empty id.
			if got := strings.Contains(input, "appBlockId"); got != tc.wantBlock {
				t.Errorf("appBlockId present = %v, want %v; input was %q", got, tc.wantBlock, input)
			}
		})
	}
}

// TestAppBlockIDArmNotFoundIsNotRetriedBySlug is the error path the narrowed comment
// implies and nothing covered: when the submission RESOLVED, its appBlockId is the
// answer, and a not-found from that lookup is final.
//
// 🟡 INVARIANT GUARD (green at base). It exists because the obvious "fix" suggested by
// #424's own framing — "the slug arm is wide now, so retry by slug whenever anything
// misses" — would be wrong here in two separate ways, and neither is currently pinned:
//
//	(a) it would spend a third request on a case the server already answered, and
//	(b) the listing the submission NAMES is the one that just missed, so whatever a
//	    slug retry resolved would by construction be a different row — and the CLI
//	    has no basis for preferring it. (`appBlockId` WINS over a conflicting slug
//	    inside one call; that precedence says nothing about a second call.)
//
// It also pins that the offsite explanation does NOT run on this arm: explainOffsiteMiss
// is reached only when the SUBMISSION lookup failed, so an app that has a submission
// must never be described as offsite.
func TestAppBlockIDArmNotFoundIsNotRetriedBySlug(t *testing.T) {
	var lookups, storeProbes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			lookups++
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"no listing found for app my-app"}}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			storeProbes++
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	_, _, err := run(t, "app", "listing", "status", "--slug", "my-app")
	if err == nil {
		t.Fatal("expected an error when the appBlockId lookup answers not-found")
	}
	// AGENTS.md item 7 — the classification, not the wording. The listing is
	// genuinely absent, so this stays not-found (exit 4).
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("a not-found listing must stay not-found (exit 4), got %T: %v", err, err)
	}
	// THE CLAIM: exactly one lookup. A by-slug retry would make it two.
	if lookups != 1 {
		t.Errorf("a resolved submission's appBlockId is the answer — a not-found from it "+
			"must not be retried by slug; getMyListingForApp ran %d times", lookups)
	}
	// The submission lookup SUCCEEDED, so the offsite diagnosis has nothing to
	// explain and its probe must not run.
	if storeProbes != 0 {
		t.Errorf("the store-catalog probe belongs to the submission-miss path only, ran %d times", storeProbes)
	}
	if strings.Contains(err.Error(), "OFFSITE app") {
		t.Errorf("an app that HAS a block submission must never be described as offsite:\n%s", err)
	}
}

// TestResolveListingScopeClaimIsPinnedWhole pins the corrected #424 prose as a WHOLE
// NORMALISED STRING rather than by checking for keywords.
//
// 🔴 WHICH GUARD THIS IS, AND WHY THAT ONE. A guard on WORDS is walkable by
// REWORDING — the class this package already met in TestAppListingOffsiteRefusesPrecisely,
// where a banned absolute was re-inserted as a SYNONYM and survived the whole suite.
// The defect #424 reported was a comment claiming more than the server does, so
// `strings.Contains(comment, "onsite")` would be satisfied by any number of
// restatements of the over-general claim, including the original one. So the two
// paragraphs carrying the claim are compared IN FULL, and the paragraph COUNT is
// asserted as well: deleting one, or adding a third that re-broadens the claim, fails
// too. That is the "pin the whole normalised string" option, not the enumerated-set
// one — the claim is prose about a moving server contract, and an enumeration of
// conditions would go stale the next time the server moves (it already has once).
//
// 🔴 REGRESSION GUARD, NOT AN INVARIANT ONE — but on the prose, not on behaviour. It
// is red at the base commit because the base carries the over-general text. It cannot
// go red for a behavioural reason; there is no behaviour here.
//
// A cosmetic reword now fails this test. That is the price of a machine-readable
// claim: re-read the paragraphs against the server before updating the literal, never
// weaken the comparison back to a keyword.
func TestResolveListingScopeClaimIsPinnedWhole(t *testing.T) {
	const marker = "civitai/cli#424"
	paras := docCommentParagraphs(t, "app_listing.go", "func resolveListing(")

	// INSTRUMENT CONTROL: the extractor must have found a real doc comment. A silent
	// zero here would make every assertion below vacuously true.
	if len(paras) < 2 {
		t.Fatalf("extractor found %d doc-comment paragraphs on resolveListing — it is wired to nothing", len(paras))
	}
	var claim []string
	for _, p := range paras {
		if strings.Contains(p, marker) {
			claim = append(claim, p)
		}
	}
	if len(claim) != 2 {
		t.Fatalf("the #424 scope claim is exactly TWO paragraphs (the selector-vs-arm correction "+
			"and the unmeasured-pending caveat); found %d carrying %q. Deleting one, or adding a "+
			"third, is a change to the claim — update this test deliberately.", len(claim), marker)
	}

	const wantSelectorClaim = "🔴 \"ONLY BY SLUG\" IS A CLAIM ABOUT WHICH SELECTOR THE CLI CAN SEND, NEVER " +
		"ABOUT HOW WIDE THE SERVER'S SLUG ARM IS — civitai/cli#424, where this comment asserted the " +
		"second while only ever having evidence for the first. Against the server it was written for, " +
		"the arm was `{slug, kind:'onsite', appBlockId:null, status:'draft'}`: four clauses that " +
		"admitted the pre-approval draft and nothing else, so every OTHER shape 404ed by slug — " +
		"`gen-matrix` and `custom-generators` (onsite, approved, so carrying an appBlockId) failed " +
		"clauses 2-3, `radio` and `comfy` failed clause 1. civitai/civitai#3989 then dropped it to " +
		"`{slug, revisionOfId: null}`, which is what the paragraph below rests on; re-verified against " +
		"civitai/civitai origin/main at `src/server/services/blocks/offsite-listing.service.ts:1746`. " +
		"The pre-approval draft is now ONE MEMBER of a wide set rather than the only thing the arm " +
		"admits, so do not re-derive a narrow server scope from this paragraph — read the server, it moves."

	const wantPendingCaveat = "🟡 AND THE PENDING CASE IS ASSERTED FROM SERVER SOURCE, NOT MEASURED. No " +
		"genuinely `pending` first-version onsite app was ever probed: the only two `appBlockId = nil` " +
		"rows reachable on 2026-08-17 were WITHDRAWN, and the old clause keyed on the LISTING's status " +
		"rather than the submission's, so they could not answer it either way. \"The server supports " +
		"it\" is therefore unverified, not proven — do not upgrade it to a measurement, and do not " +
		"downgrade it to a doubt either. Nothing turns on it today: the widened clause admits the row " +
		"on the slug whatever its status, so this is archaeology. civitai/cli#424 carries the recipe " +
		"if anyone wants to settle it."

	for i, want := range []string{wantSelectorClaim, wantPendingCaveat} {
		if claim[i] != want {
			t.Errorf("#424 paragraph %d is pinned WHOLE, not by banned spellings.\nIf you changed it "+
				"on purpose, re-read it against the server and update the literal.\n want: %s\n got:  %s",
				i+1, want, claim[i])
		}
	}
}

// docCommentParagraphs returns the doc comment immediately above the first line of
// file that starts with decl, split into paragraphs on bare `//` lines, with each
// paragraph's `// ` prefixes stripped and its whitespace collapsed.
//
// Kept deliberately dumb — no go/ast — because the thing being pinned is the literal
// text a human reads, and a parser that normalised it would pin something else.
func docCommentParagraphs(t *testing.T, file, decl string) []string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	lines := strings.Split(string(src), "\n")
	declAt := -1
	for i, l := range lines {
		if strings.HasPrefix(l, decl) {
			declAt = i
			break
		}
	}
	if declAt < 0 {
		t.Fatalf("%s: no line starting with %q — the extractor is pointed at nothing", file, decl)
	}
	start := declAt
	for start > 0 && strings.HasPrefix(lines[start-1], "//") {
		start--
	}
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, normaliseMessage(strings.Join(cur, " ")))
			cur = nil
		}
	}
	for _, l := range lines[start:declAt] {
		body := strings.TrimPrefix(strings.TrimPrefix(l, "//"), " ")
		if strings.TrimSpace(body) == "" {
			flush()
			continue
		}
		cur = append(cur, body)
	}
	flush()
	return paras
}
