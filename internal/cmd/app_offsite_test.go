package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#422 outcome 2 — the OFFSITE refusal.
//
// The regression risk this file exists to hold down is NOT the new message. It
// is the old one: `app listing` / `app status <slug>` for an ONSITE app that has
// simply never been submitted must keep saying exactly what it says today, and
// so must every case where the diagnostic probe cannot answer. A probe whose own
// failure replaces a real error with a confident wrong one is worse than the
// error it replaced, so most of the cases below assert the UNCHANGED string.
//
// 🔴 THE FIXTURES ARE PAIRWISE DISTINCT ON EVERY FIELD THE ASSERTIONS READ, and
// that is deliberate (#422's own warning about fixtures that can only produce
// the expected value). The offsite rows differ from the onsite row in `kind`
// (the branch) AND in `kindData` (the rendered external URL), and the two
// offsite rows differ from EACH OTHER in that URL — so a mutant that hardcodes
// either the branch or the URL literal is killed rather than surviving green.
//
// 🔴 AND DISTINCT ON THE SLUG TOO, which the first cut missed. The store detail
// body echoes a `slug`, and the fixtures set it equal to the slug the caller
// typed — so swapping the message's `%s` from the caller's slug to the server's
// `d.Slug` SURVIVED a fully green suite. It is exactly the case a fixture whose
// two fields can only hold one value cannot see. serverEchoSlug now makes the
// server's echo a value the caller's slug is not even a SUBSTRING of, and
// TestOffsiteFixturesAreDistinctOnSlug is the control that keeps it that way.

const (
	// offsiteSlugA/B are two DIFFERENT offsite apps with two DIFFERENT external
	// targets. Neither URL appears as a literal anywhere in non-test source —
	// checked by TestOffsiteRefusalRendersTheAppsOwnExternalURL, which is the
	// control that the message really reads the probe's payload.
	offsiteSlugA = "radio-zzz"
	offsiteURLA  = "https://radio.zzz-fixture-a.test/launch"
	offsiteSlugB = "comfy-zzz"
	offsiteURLB  = "https://comfy.zzz-fixture-b.test/open"
	// onsiteSlug is an app that EXISTS and is onsite — the app-was-never-
	// submitted case, which must keep today's message.
	onsiteSlug = "gen-matrix-zzz"
)

// appProbeServer is a fake civitai.com that answers the submissions route with
// an EMPTY list (the #422 wall) and the store detail route from a per-slug
// table, counting how many times each is hit.
type appProbeServer struct {
	*httptest.Server
	// appCalls counts GET /api/v1/apps/{slug} — the diagnostic probe. On every
	// SUCCESSFUL command it must stay 0.
	appCalls atomic.Int32
	subCalls atomic.Int32
	// listingCalls counts appListings.getMyListingForApp — resolveListing's
	// BY-SLUG fallback (civitai/cli#422 outcome 1). It is what separates "this
	// server predates civitai/civitai#3989" from "this CLI never asked": every
	// refusal case below now has to prove the fallback was ATTEMPTED and missed,
	// or it pins the ABSENCE of a feature rather than its failure mode.
	listingCalls atomic.Int32
	// lastListingInput holds the raw `?input=` of the last getMyListingForApp, so
	// a test can assert WHICH selector was used: the fallback must send the slug
	// and no appBlockId.
	lastListingInput atomic.Value
}

// serverEchoSlug is the `slug` the fake store route puts in the RESPONSE, and it
// is deliberately NOT the slug the caller asked about — nor a string that
// contains it. Every refusal must render the slug the USER typed (the one their
// next command will work with); a body echoing the same value made that
// unobservable, and the `d.Slug` mutant survived. The `zzz-srv-` prefix and the
// dropped `-zzz` suffix are what make containment impossible in both directions.
func serverEchoSlug(slug string) string {
	return "zzz-srv-" + strings.TrimSuffix(slug, "-zzz")
}

// appDetailJSON renders a store-detail body. The onsite and offsite shapes carry
// DIFFERENT kindData, mirroring the backend's discriminated union.
func appDetailJSON(slug, kind, externalURL string) string {
	echo := serverEchoSlug(slug)
	if kind == "onsite" {
		return fmt.Sprintf(`{"id":"apl_%s","slug":%q,"kind":"onsite","name":"Onsite %s",
			"iconUrl":"https://img.zzz/icon-onsite.png","coverUrl":"https://img.zzz/cover-onsite.png",
			"kindData":{"kind":"onsite","appBlockId":"apb_zzz_onsite","hasPage":true,"liveUrl":"https://%s.civit.ai/"}}`,
			slug, echo, slug, slug)
	}
	// externalUrl goes through jsonString (json.Marshal), NOT %q: Go's %q emits
	// `\x1b` and `\a` for ESC and BEL, which are not JSON escapes at all, so a
	// fixture carrying a control character would arrive as an unparseable body
	// and the probe would collapse to "could not answer" — a green that means
	// nothing. json.Marshal emits the spec-compliant \uXXXX form.
	return fmt.Sprintf(`{"id":"apl_%s","slug":%q,"kind":%q,"name":"Offsite %s",
		"iconUrl":"https://img.zzz/icon-%s.png","coverUrl":"https://img.zzz/cover-%s.png",
		"kindData":{"kind":%q,"subKind":"connect","externalUrl":%s,"connectClientId":"cc_zzz"}}`,
		slug, echo, kind, slug, slug, slug, kind, jsonString(externalURL))
}

// TestOffsiteFixturesAreDistinctOnSlug is the control ON the fixtures. Every
// assertion that a refusal names the caller's slug is vacuous unless the server
// echoes a DIFFERENT one, and "different" is not enough — a `Contains` assertion
// passes for any echo the caller's slug is a substring of, which is what a naive
// `slug + "-server"` echo would have been. This fails before any of those
// assertions can pass for the wrong reason.
func TestOffsiteFixturesAreDistinctOnSlug(t *testing.T) {
	for _, slug := range []string{offsiteSlugA, offsiteSlugB, onsiteSlug} {
		echo := serverEchoSlug(slug)
		if echo == slug || strings.Contains(echo, slug) || strings.Contains(slug, echo) {
			t.Errorf("the server echo %q must not equal or contain the caller's slug %q (nor be contained by it) — "+
				"otherwise every `the message names the slug` assertion passes for a message rendering the server's value",
				echo, slug)
		}
	}
}

// newAppProbeServer wires the fake and the environment for the cases where the
// BY-SLUG fallback misses — a server that predates civitai/civitai#3989, or an
// app with no listing row. apps maps a slug to the raw detail body the store
// route answers with; a slug that is absent 404s, which is the "no such app"
// case.
//
// 🔴 THE tRPC ARM 404s BY DEFAULT, AND THAT IS A FIXTURE DECISION, NOT A GAP.
// Before #422 outcome 1 this fake had no getMyListingForApp arm at all, because
// nothing on the error path called it; leaving it out now would make every
// refusal case here fail on `unexpected path` instead of measuring anything. A
// 404 is the honest model of the two servers the narrowed refusal is FOR.
func newAppProbeServer(t *testing.T, apps map[string]string) *appProbeServer {
	t.Helper()
	ps := &appProbeServer{}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			ps.subCalls.Add(1)
			if r.URL.Query().Get("id") != "" {
				// The `--id` spelling 404s natively (there is no such publish
				// request), which is a different arm of submissionsError.
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"submissions":[]}`))
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			ps.listingCalls.Add(1)
			ps.lastListingInput.Store(r.URL.Query().Get("input"))
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"No listing found for this app","code":-32004}}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			ps.appCalls.Add(1)
			slug := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
			body, ok := apps[slug]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(ps.Close)
	listingEnv(t, ps.URL)
	return ps
}

// trpcInputSlug reads the `slug` out of a tRPC query's `?input={"json":{…}}`.
// t.Errorf rather than t.Fatalf: this runs on the server's goroutine, where
// FailNow is not allowed.
func trpcInputSlug(t *testing.T, raw string) string {
	t.Helper()
	var env struct {
		JSON struct {
			Slug string `json:"slug"`
		} `json:"json"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Errorf("undecodable tRPC input %q: %v", raw, err)
		return ""
	}
	return env.JSON.Slug
}

// listingRefBody is the getMyListingForApp payload for a slug that RESOLVES.
func listingRefBody(appListingID, status string) map[string]any {
	return map[string]any{
		"appListingId": appListingID, "status": status,
		"contentRating": "g", "hasPendingRevision": false,
	}
}

// normaliseMessage collapses every whitespace run to one space so the pinned
// literal below can be written across source lines. It normalises LAYOUT only —
// every word, every punctuation mark and every backtick survives, so a reword is
// still a failure. That is the point: line wrapping is not a claim, wording is.
func normaliseMessage(s string) string { return strings.Join(strings.Fields(s), " ") }

// wantOffsiteListingRefusal is the `app listing` refusal, WHOLE, for offsiteSlugA
// with offsiteURLA.
//
// 🔴 WRITTEN FROM THE MESSAGE, NOT DERIVED FROM IT. It is a literal, never
// `offsiteListingRefusal(offsiteSlugA, d)` — an expectation computed by calling
// the function under test asserts only that the function is deterministic, which
// is true of every wrong message too.
//
// 🔴 WHAT IT IS ACTUALLY GUARDING, so a future reader knows which words are
// load-bearing rather than incidental. (a) NO ABSOLUTE: nothing here may say the
// listing cannot be addressed / reached / read from this CLI, in any phrasing —
// it can, on a server carrying civitai/civitai#3989, and the ban on one spelling
// was walked by a synonym. (b) The causes named are the two that really land
// here; "a listing this account does not own" appears ONLY as the case that is
// refused 403 elsewhere, because after the fallback's error gate it no longer
// reaches this message on a current server. (c) `civitai app submit` is named as
// NOT the step, never as the step. (d) The slug rendered is the CALLER's
// (`radio-zzz`), never the server's echo (`zzz-srv-radio`). (e) NO PROMISE TO THE
// READER: offsiteApp does no ownership check and the store catalog is public, so
// the App-store listing UI is named as where the media is MANAGED — never as a
// surface that will work for whoever ran the command (civitai/cli#427 residual 1,
// closed 2026-08-17; it said "Changing that media always works … where this app
// was registered" until then).
const wantOffsiteListingRefusal = "`" + offsiteSlugA + "` is an OFFSITE app (registered at " + offsiteURLA + "), " +
	"and this CLI could not reach its store listing here — `civitai app listing` resolves a listing through the " +
	"app's block submission, which an offsite app has none of, and then by the slug alone; neither answered. " +
	"`civitai app submit` is NOT the missing step: it packages a block bundle, and an offsite app is a registered " +
	"URL, so no submission it could create exists to be made. " +
	"The by-slug lookup reaches an offsite listing only on a Civitai carrying civitai/civitai#3989, so an older " +
	"or self-hosted server, or an app with no listing row at all, each land here; a listing this account does not " +
	"own is refused as a 403 by any server carrying #3989, and reaches this message only on one that does not. " +
	"Its store listing may be live all the same — run `civitai app view " + offsiteSlugA + "` to see the icon and " +
	"cover it is serving. That media is managed in the App-store listing UI on civitai.com by the account that " +
	"registered the app; that surface is authoritative whatever this CLI can reach"

// bothOffsiteAndOnsite is the fixture table every case below shares.
func bothOffsiteAndOnsite() map[string]string {
	return map[string]string{
		offsiteSlugA: appDetailJSON(offsiteSlugA, "offsite", offsiteURLA),
		offsiteSlugB: appDetailJSON(offsiteSlugB, "offsite", offsiteURLB),
		onsiteSlug:   appDetailJSON(onsiteSlug, "onsite", ""),
	}
}

// ---------------------------------------------------------------------------
// civitai/cli#422 OUTCOME 1 — `app listing` REACHES an offsite app
// ---------------------------------------------------------------------------
//
// The repair, and the thing the rest of this file used to assert was impossible.
// `civitai/civitai#3989` rescoped `appListings.getMyListingForApp`'s slug arm
// from `{slug, kind:'onsite', appBlockId:null, status:'draft'}` to
// `{slug, revisionOfId:null}`, so resolveListing can fall back to it when the
// submission lookup 404s. Measured live 2026-08-17: 4/4 offsite apps resolved by
// slug, `gen-matrix` (onsite) still resolved, an unknown slug still 404ed.

// offsiteListingID is the `apl_` row the by-slug selector resolves to, and
// offsiteShadowID is the revision the APPROVED path opens over it. Both are
// deliberately NOT derivable from the slug, so an assertion that the command
// addressed THIS listing cannot pass for one that echoed the caller's input.
//
// 🔴 THAT SENTENCE WAS A CLAIM ABOUT A TEST THAT DID NOT EXIST. Until the fake
// below started CHECKING the `listingId` it is sent, no assertion anywhere
// observed which listing was addressed: hardcoding
// `GetMyListingForEdit(ctx, "apl_WRONG_TARGET")` survived the FULL suite,
// because the fake answered every id identically and the human output prints the
// caller's slug rather than the id. A constant chosen so a wrong value cannot
// coincide is necessary and does nothing on its own — something has to READ it.
// See assertListingTarget.
const (
	offsiteListingID = "apl_zzz_offsite_1"
	offsiteShadowID  = "apl_zzz_offsite_shadow_9"
)

// trpcListingID pulls `listingId` out of a tRPC call however it was sent — a
// query's `?input={"json":{…}}` or a mutation's POST body — and returns "" when
// the proc carries none (removeScreenshot sends only a screenshotId,
// submitListingRevision only a shadowId, the ingests neither).
//
// Runs on the server goroutine, so t.Errorf and never t.Fatalf.
func trpcListingID(t *testing.T, r *http.Request) string {
	t.Helper()
	raw := r.URL.Query().Get("input")
	if raw == "" && r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("unreadable tRPC body for %s: %v", r.URL.Path, err)
			return ""
		}
		raw = string(b)
	}
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var env struct {
		JSON struct {
			ListingID string `json:"listingId"`
		} `json:"json"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Errorf("undecodable tRPC input %q for %s: %v", raw, r.URL.Path, err)
		return ""
	}
	return env.JSON.ListingID
}

// trpcProc returns the bare proc name from a tRPC path —
// `/api/trpc/appListings.setIcon` → `setIcon`.
func trpcProc(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndex(path, "."); i >= 0 {
		path = path[i+1:]
	}
	return path
}

// listingKeyedProcs is the LEDGER the empty-id exemption rests on: every tRPC
// proc this fixture routes through assertListingTarget, mapped to whether its
// wire input MUST carry a `listingId`.
//
// 🔴 IT EXISTS BECAUSE THE EXEMPTION USED TO BE AN ASSUMPTION, AND THE ASSUMPTION
// WAS THE HOLE. assertListingTarget returned early on an empty id, justified by a
// call site that did not exist (removeScreenshot was never routed through it), and
// the exemption applied to the five that did. MEASURED: renaming the wire key in
// `appapi.GetMyListingForEdit` (`"listingId"` → `"listing_id"`) empties the id the
// fake reads, every assertion returns early, and the mutant SURVIVES the FULL
// suite — 20/20 packages ok. The positive control on the same line (the id
// replaced by a wrong literal) went red with this function's own message, so the
// guard was reachable and the `""` escape was the only hole. The ledger closes it:
// an empty id from a proc declared `true` is a FAILURE, and a listing id from one
// declared `false` is one too.
//
// 🔴 AN UNLISTED PROC IS ALSO A FAILURE, which is what keeps this a ledger rather
// than a list someone remembered to update: a new listing proc cannot be routed
// through the fake without declaring which side of the exemption it is on. The
// ledger's reach is exactly the procs the fake asserts on — it says nothing about
// a proc no handler routes here, which is why the two un-keyed ones below are
// asserted rather than merely described.
var listingKeyedProcs = map[string]bool{
	"getMyListingForEdit":  true,
	"setIcon":              true,
	"setCover":             true,
	"addScreenshot":        true,
	"reorderScreenshots":   true,
	"beginListingRevision": true,
	// Keyed by screenshotId alone — the server resolves the owning listing from
	// the row, which is civitai/cli#436's whole mechanism.
	"removeScreenshot": false,
	// Keyed by the shadowId being submitted (plus an optional changelog); the
	// parent listing is the server's to resolve.
	"submitListingRevision": false,
}

// assertListingTarget fails the test when a listing-keyed proc was addressed at
// anything other than want. This is the assertion civitai/cli#422's fixture
// comment PROMISED and did not make.
//
// 🔴 IT IS A STATE ASSERTION, NOT A SPELLING ONE, over the procs listingKeyedProcs
// declares. It compares the id the wire carried against the id the fixture
// published, so for a listing-keyed proc it catches every way the wrong listing
// can be addressed — an echoed slug, a hardcoded literal, the parent where the
// shadow was meant, a stale variable — rather than the one substitution someone
// thought of. What it CANNOT see, stated so it is not re-promised: a rename that
// moves the key in the fake's decoder (trpcListingID) at the same time, and any
// proc no handler routes through here.
//
// The empty-id case is no longer an exemption but a verdict the ledger returns:
// `false` procs must carry NO listing id, `true` procs must carry `want`.
func assertListingTarget(t *testing.T, r *http.Request, want string) {
	t.Helper()
	proc := trpcProc(r.URL.Path)
	mustCarry, declared := listingKeyedProcs[proc]
	if !declared {
		t.Errorf("tRPC proc %q is asserted on but not declared in listingKeyedProcs — say whether its "+
			"input carries a `listingId`; an undeclared proc is how the empty-id exemption silently widens", proc)
		return
	}
	got := trpcListingID(t, r)
	if !mustCarry {
		if got != "" {
			t.Errorf("%s carried listingId %q, but listingKeyedProcs declares it un-keyed — either the "+
				"wire input grew a listing id or the ledger is stale", r.URL.Path, got)
		}
		return
	}
	if got == "" {
		t.Errorf("%s carried NO listingId though it is declared listing-keyed (want %q) — the wire key was "+
			"renamed, dropped, or nested where trpcListingID does not look, so NOTHING here observed which "+
			"listing was addressed", r.URL.Path, want)
		return
	}
	if got != want {
		t.Errorf("%s was addressed at listing %q, want %q — the command resolved the wrong target",
			r.URL.Path, got, want)
	}
}

// TestOffsiteAppListingResolvesBySlug is the failing path from civitai/cli#422,
// reproduced and now green: an app with NO block submission whose listing the
// slug selector resolves.
//
// 🔴 THE TWO PREMISE ASSERTIONS ARE WHAT MAKE IT MORE THAN "A COMMAND EXITED 0".
// (a) The store probe must NOT have run: reaching the diagnostic means the
// command failed, so `appCalls == 0` says the success is a success and not a
// refusal that happened to print. (b) The lookup must have carried the SLUG and
// no appBlockId — an offsite app has no block id to send, so a fallback that
// quietly sent an empty one alongside would be resolving by something else.
func TestOffsiteAppListingResolvesBySlug(t *testing.T) {
	fake := newOffsiteListingServer(t)
	listingEnv(t, fake.URL)

	out, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	if err != nil {
		t.Fatalf("civitai/cli#422: `app listing status` must now SUCCEED for an offsite app, got: %v", err)
	}
	for _, want := range []string{"App:", offsiteSlugA, "Listing status:", "draft"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in the rendered status:\n%s", want, out)
		}
	}
	if n := fake.listingCalls.Load(); n != 1 {
		t.Errorf("the by-slug fallback must have resolved in exactly one call, ran %d times", n)
	}
	if n := fake.appCalls.Load(); n != 0 {
		t.Errorf("PREMISE: a command that SUCCEEDED never reaches the diagnostic probe; it ran %d times", n)
	}
	in, _ := fake.lastListingInput.Load().(string)
	if !strings.Contains(in, `"slug":"`+offsiteSlugA+`"`) {
		t.Errorf("the fallback must select by SLUG; input was %q", in)
	}
	if strings.Contains(in, "appBlockId") {
		t.Errorf("an offsite app has no block id — the fallback must send none; input was %q", in)
	}
}

// TestOffsiteRepairReachesEverySubcommand is the reachability half of the
// repair, and the mirror of TestOffsiteRefusalReachesEverySubcommand below.
// resolveListing is the ONE funnel for `app listing`, so a fix applied there
// must be observable from every subcommand — not just the one `status` drives.
//
// 🔴 EACH ROW ASSERTS AN OUTCOME, NOT MERELY "NOT THE REFUSAL". A row that only
// checked for the absence of the word OFFSITE would pass for a command that
// resolved the listing and then failed for any other reason, which is the same
// green for a different world. Six drive to real success against a DRAFT offsite
// listing; `submit-revision` refuses BECAUSE the listing is a draft, which is
// itself proof that it read the resolved ref's status.
func TestOffsiteRepairReachesEverySubcommand(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    func(t *testing.T) []string
		wantErr string // "" means the command must succeed
		wantOut string
	}{
		{"status", func(*testing.T) []string {
			return []string{"app", "listing", "status", "--slug", offsiteSlugA}
		}, "", "Listing status:"},
		{"set-icon", func(t *testing.T) []string {
			return []string{"app", "listing", "set-icon", writePNG(t, 512, 512), "--slug", offsiteSlugA, "-y"}
		}, "", "Icon set"},
		{"set-cover", func(t *testing.T) []string {
			return []string{"app", "listing", "set-cover", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}, "", "Cover set"},
		{"add-screenshot", func(t *testing.T) []string {
			return []string{"app", "listing", "add-screenshot", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}, "", "Screenshot set"},
		{"rm-screenshot", func(*testing.T) []string {
			return []string{"app", "listing", "rm-screenshot", "alsc_1", "--slug", offsiteSlugA}
		}, "", "Screenshot removed"},
		{"reorder", func(*testing.T) []string {
			return []string{"app", "listing", "reorder", "alsc_2", "alsc_1", "--slug", offsiteSlugA}
		}, "", "Reordered 2 screenshots"},
		// A DRAFT listing has no revision to submit, so this refuses — and the
		// refusal quotes the status it read off the ref the fallback resolved.
		{"submit-revision", func(*testing.T) []string {
			return []string{"app", "listing", "submit-revision", "--slug", offsiteSlugA}
		}, "this listing is not live (status draft)", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fastScanPoll(t)
			fake := newOffsiteListingServer(t)
			listingEnv(t, fake.URL)

			out, _, err := run(t, tc.args(t)...)
			if err != nil && strings.Contains(err.Error(), "OFFSITE app") {
				t.Fatalf("civitai/cli#422 outcome 1: this subcommand must REACH the offsite listing, not refuse:\n%s", err)
			}
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				if !strings.Contains(out, tc.wantOut) {
					t.Errorf("missing %q in the output:\n%s", tc.wantOut, out)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %q, got success:\n%s", tc.wantErr, out)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want an error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// TestOffsiteRepairReachesEverySubcommandOnAnApprovedListing is the state
// PRODUCTION IS IN, and the shipped suite did not have it.
//
// 🔴 EVERY OFFSITE APP MEASURED ON civitai.com IS `approved` — `radio`, `comfy`,
// `cosmetic-studio`, `vitrine`, all four (2026-08-17). The sibling test above
// drives a DRAFT listing, where each subcommand writes the listing directly and
// `submit-revision` refuses. None of that is the code an offsite author runs: on
// an approved listing every mutating subcommand instead goes
// beginListingRevision → attach to the SHADOW → submitListingRevision, and
// `submit-revision` succeeds. A repair asserted only against `draft` is asserted
// against a state no real offsite app is in.
//
// 🔴 THE ROWS ASSERT THE REVISION LANGUAGE, NOT MERELY SUCCESS. "staged on a
// revision — pending moderator review" is the claim that separates the live path
// from the draft one; a command that resolved the offsite listing and then took
// the DRAFT branch would print `✓ Icon set` and exit 0, which is the same green
// for a world where a live listing was edited without review. The fake's
// assertListingTarget carries the other half: the attach must be addressed at
// the shadow, never at the live parent.
func TestOffsiteRepairReachesEverySubcommandOnAnApprovedListing(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(t *testing.T) []string
		// wantOut is the outcome, chosen from the LIVE arm's own wording.
		wantOut string
		// wantDraftWord must NOT appear: it is what the draft arm prints for the
		// same command, so its absence is what proves the live branch ran.
		wantNotOut string
		// wantRevisionCalls / wantSubmitCalls pin the revision handshake itself,
		// which no string in the output can distinguish from a lucky message.
		wantRevisionCalls int32
		wantSubmitCalls   int32
	}{
		{"status", func(*testing.T) []string {
			return []string{"app", "listing", "status", "--slug", offsiteSlugA}
		}, "approved", "", 0, 0},
		{"set-icon", func(t *testing.T) []string {
			return []string{"app", "listing", "set-icon", writePNG(t, 512, 512), "--slug", offsiteSlugA, "-y"}
		}, "Icon staged on a revision — pending moderator review (alpr_zzz_offsite).", "Icon set", 1, 1},
		{"set-cover", func(t *testing.T) []string {
			return []string{"app", "listing", "set-cover", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}, "Cover staged on a revision — pending moderator review (alpr_zzz_offsite).", "Cover set", 1, 1},
		{"add-screenshot", func(t *testing.T) []string {
			return []string{"app", "listing", "add-screenshot", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}, "Screenshot staged on a revision — pending moderator review (alpr_zzz_offsite).", "Screenshot set", 1, 1},
		{"rm-screenshot", func(*testing.T) []string {
			return []string{"app", "listing", "rm-screenshot", "alsc_1", "--slug", offsiteSlugA}
		}, "Screenshot removal staged on an open revision — not submitted for review yet.", "", 0, 0},
		{"reorder", func(*testing.T) []string {
			return []string{"app", "listing", "reorder", "alsc_2", "alsc_1", "--slug", offsiteSlugA}
		}, "New screenshot order staged on a revision — pending moderator review (alpr_zzz_offsite).", "Reordered 2 screenshots", 1, 1},
		// The mirror of the draft row: there the listing is not live so this
		// refuses; here it IS live, so it submits the open revision.
		{"submit-revision", func(*testing.T) []string {
			return []string{"app", "listing", "submit-revision", "--slug", offsiteSlugA}
		}, "Revision submitted — pending moderator review (alpr_zzz_offsite).", "not live (status", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fastScanPoll(t)
			fake := newOffsiteListingServerWithStatus(t, "approved")
			listingEnv(t, fake.URL)

			out, _, err := run(t, tc.args(t)...)
			if err != nil {
				if strings.Contains(err.Error(), "OFFSITE app") {
					t.Fatalf("civitai/cli#422 outcome 1: this subcommand must REACH the approved offsite listing, not refuse:\n%s", err)
				}
				t.Fatalf("expected success against an APPROVED offsite listing, got: %v\n%s", err, out)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Errorf("missing %q in the output:\n%s", tc.wantOut, out)
			}
			if tc.wantNotOut != "" && strings.Contains(out, tc.wantNotOut) {
				t.Errorf("this is the DRAFT arm's wording — the live listing was edited without opening a revision:\n%s", out)
			}
			if n := fake.revisionCalls.Load(); n != tc.wantRevisionCalls {
				t.Errorf("beginListingRevision ran %d times, want %d", n, tc.wantRevisionCalls)
			}
			if n := fake.submitRevCalls.Load(); n != tc.wantSubmitCalls {
				t.Errorf("submitListingRevision ran %d times, want %d", n, tc.wantSubmitCalls)
			}
			// PREMISE, as in the draft sibling: the by-slug fallback is what
			// resolved this, and a successful command never reaches the probe.
			if n := fake.listingCalls.Load(); n != 1 {
				t.Errorf("the by-slug fallback must have resolved in exactly one call, ran %d times", n)
			}
			if n := fake.appCalls.Load(); n != 0 {
				t.Errorf("PREMISE: a command that SUCCEEDED never reaches the diagnostic probe; it ran %d times", n)
			}
		})
	}
}

// TestFallbackFailuresKeepTheirOwnError is civitai/cli#422's OTHER gate, and it
// had no test at all.
//
// TestNonNotFoundSubmissionFailuresSkipTheSlugFallback covers the FIRST lookup:
// a non-404 there must not be retried by slug. This covers the SECOND: once the
// fallback HAS run, its own failure must not be swallowed.
//
// 🔴 THE BUG IT PINS SHIPPED AND LOOKED DELIBERATE. resolveListing wrote
// `if ref, slugErr := client.GetMyListingForApp(…); slugErr == nil`, discarding
// slugErr — so a 403, a 500 or a 503 from the by-slug arm fell through carrying
// the SUBMISSION's not-found, `errors.Is(err, ErrNotFound)` stayed true, and the
// command exited 4 printing a message whose three named causes did not include
// "the server errored". Measured before the fix: a mutant that RETURNED slugErr
// was killed by 15 tests, i.e. the swallow was pinned by the suite and read as
// intentional. Nothing observed the failure it hid.
//
// The rows are the classifications the published contract now names, and the
// 500 row is the one that matters most: it carries NO sentinel, so a swallow
// shows up as 4 while the honest answer is 1 — two codes that mean opposite
// things to a script.
func TestFallbackFailuresKeepTheirOwnError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		wantIs error
		// wantMsg is a fragment of the FALLBACK's own error, taken from
		// listingError's arm for that status — not from a shared prefix.
		wantMsg string
	}{
		// 🔴 THE FIXTURE MESSAGE IS A COHORT REFUSAL ON PURPOSE. It used to be
		// "you can only manage your own listings", which was incidental — any
		// 403 body exercises this row's actual subject (the fallback's own
		// error survives instead of inheriting the submission's not-found).
		// That string now has its OWN arm in listingError (the NOT_OWNED
		// ownership branch), so leaving it here would silently re-point this
		// row at that arm and couple an unrelated test to it. The cohort
		// message is the one the catch-all is genuinely true of, which is the
		// arm `wantMsg` below names. The not-owned routing is pinned by
		// appapi's TestListingForbiddenTakedownDoesNotBlameTheAccount.
		{"403 from the listings route", http.StatusForbidden,
			`{"error":{"json":{"message":"Apps authoring is not enabled"}}}`,
			civitai.ErrUnauthorized, "not permitted for your account (403)"},
		{"500 from the listings route", http.StatusInternalServerError,
			`{"error":{"json":{"message":"boom"}}}`, nil, ""},
		{"503 from the listings route", http.StatusServiceUnavailable,
			`{"error":{"json":{"message":"down"}}}`, civitai.ErrNetwork, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var listingCalls, appCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
					// The offsite shape: 200 + empty list, resolved to
					// ErrNotFound client-side. This is what makes the fallback
					// run — without it the row would be testing the sibling.
					_, _ = w.Write([]byte(`{"submissions":[]}`))
				case strings.Contains(r.URL.Path, "getMyListingForApp"):
					listingCalls.Add(1)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
					appCalls.Add(1)
					// Serve a REAL offsite body: if the probe ever runs, the
					// command produces the confident OFFSITE refusal, so the
					// assertion below fails LOUDLY rather than on a count alone.
					_, _ = w.Write([]byte(appDetailJSON(offsiteSlugA, "offsite", offsiteURLA)))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			listingEnv(t, srv.URL)

			_, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
			if err == nil {
				t.Fatal("a non-not-found failure of the by-slug fallback must stay an error")
			}
			// PREMISE: the fallback really ran. Without this the row is green
			// for a CLI that never asks, which is the regression outcome 1 undoes.
			if n := listingCalls.Load(); n != 1 {
				t.Fatalf("PREMISE: the by-slug fallback must have run exactly once, ran %d times", n)
			}
			if errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("a %d from the FALLBACK must not inherit the submission's not-found (exit 4): %v", tc.status, err)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("the fallback's own classification must survive, got %T: %v", err, err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("the user must see the fallback's own error %q, got:\n%s", tc.wantMsg, err)
			}
			if strings.Contains(err.Error(), "OFFSITE") {
				t.Errorf("a %d must not be rewritten into a claim about the app's kind:\n%s", tc.status, err)
			}
			if n := appCalls.Load(); n != 0 {
				t.Errorf("only a not-found may be probed; the store route ran %d times", n)
			}
		})
	}
}

// offsiteFake is a Civitai carrying civitai/civitai#3989, plus the call counters
// the premise assertions read.
type offsiteFake struct {
	*httptest.Server
	listingCalls     atomic.Int32
	appCalls         atomic.Int32
	revisionCalls    atomic.Int32
	submitRevCalls   atomic.Int32
	lastListingInput atomic.Value
}

// newOffsiteListingServer is the DRAFT offsite listing.
func newOffsiteListingServer(t *testing.T) *offsiteFake {
	t.Helper()
	return newOffsiteListingServerWithStatus(t, "draft")
}

// newOffsiteListingServerWithStatus answers as a Civitai carrying #3989 does: no
// block submission for the app, a listing in `status` resolvable BY SLUG, and
// every listing-keyed proc the subcommands need. The `/api/v1/apps/` arm FAILS
// the test rather than answering — reaching the kind probe means the command
// failed, and a fake that served it would hide that behind today's generic
// message.
//
// 🔴 `approved` IS THE STATE PRODUCTION IS IN, AND THE SHIPPED SUITE ONLY HAD
// `draft`. All four offsite apps measured on civitai.com are approved, and the
// two paths are not the same code: on a draft the subcommands write the listing
// directly, while on an approved one each runs ingest → beginListingRevision →
// attach-to-the-SHADOW → submitRevision, and `submit-revision` flips from
// refusing ("not live (status draft)") to succeeding. A suite that models only
// `draft` is structurally blind to every defect on the revision path — including
// attaching to the parent instead of the shadow, which would edit a LIVE listing
// without moderator review.
//
// 🔴 IT ASSERTS *WHICH* LISTING EACH PROC WAS ADDRESSED AT. That is what makes
// the approved fixture worth more than a second copy of the draft one: the
// parent and the shadow are different ids here, so "the command attached
// something" and "the command attached it to the revision" stop being the same
// green. See assertListingTarget.
func newOffsiteListingServerWithStatus(t *testing.T, status string) *offsiteFake {
	t.Helper()
	live := status == "approved"
	// The attach/reorder target: the shadow revision on a live listing, the
	// listing itself on a draft one.
	attachTarget := offsiteListingID
	if live {
		attachTarget = offsiteShadowID
	}
	var shadowID any
	if live {
		shadowID = offsiteShadowID
	}
	f := &offsiteFake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			_, _ = w.Write([]byte(`{"submissions":[]}`))
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			f.listingCalls.Add(1)
			f.lastListingInput.Store(r.URL.Query().Get("input"))
			if slug := trpcInputSlug(t, r.URL.Query().Get("input")); slug != offsiteSlugA {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"No listing found"}}}`))
				return
			}
			trpcData(w, listingRefBody(offsiteListingID, status))
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			// Always the PARENT: every caller passes ref.AppListingID, and the
			// server resolves the shadow itself. A command that passed the shadow
			// here would be reading the wrong row.
			assertListingTarget(t, r, offsiteListingID)
			trpcData(w, map[string]any{
				"parentId": offsiteListingID, "slug": "server-echo", "status": status,
				"hasPendingRevision": false, "shadowId": shadowID,
				"assets": map[string]any{
					"icon":  map[string]any{"imageId": 11, "url": "http://x/i.png"},
					"cover": map[string]any{"imageId": 12, "url": "http://x/c.png"},
					"screenshots": []any{
						map[string]any{"id": "alsc_1", "imageId": 5, "url": "http://x/1.png", "order": 0},
						map[string]any{"id": "alsc_2", "imageId": 6, "url": "http://x/2.png", "order": 1},
					},
				},
			})
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			trpcData(w, map[string]any{"imageId": 4242})
		case strings.Contains(r.URL.Path, "image-upload"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "uuid-1", "uploadURL": "http://" + r.Host + "/upload-sink",
			})
		case strings.Contains(r.URL.Path, "/upload-sink"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "persistAssetImage"):
			trpcData(w, map[string]any{"imageId": 91})
		case strings.Contains(r.URL.Path, "setIcon"):
			assertListingTarget(t, r, attachTarget)
			trpcData(w, map[string]any{"status": "attached", "iconId": 4242})
		case strings.Contains(r.URL.Path, "setCover"):
			assertListingTarget(t, r, attachTarget)
			trpcData(w, map[string]any{"status": "attached", "coverId": 91})
		case strings.Contains(r.URL.Path, "addScreenshot"):
			assertListingTarget(t, r, attachTarget)
			trpcData(w, map[string]any{"status": "attached", "id": "alsc_new", "order": 2})
		case strings.Contains(r.URL.Path, "reorderScreenshots"):
			assertListingTarget(t, r, attachTarget)
			trpcData(w, map[string]any{"ok": true})
		case strings.Contains(r.URL.Path, "removeScreenshot"):
			// Keyed by screenshotId alone — the server resolves the owning
			// listing from the row, which is civitai/cli#436's whole mechanism.
			// Asserted rather than assumed: listingKeyedProcs declares it
			// un-keyed, so this call is what makes that half of the ledger real.
			assertListingTarget(t, r, attachTarget)
			trpcData(w, map[string]any{"ok": true})
		case strings.Contains(r.URL.Path, "submitListingRevision"):
			f.submitRevCalls.Add(1)
			// Declared un-keyed (it carries the shadowId) — same reason.
			assertListingTarget(t, r, attachTarget)
			if !live {
				t.Errorf("a DRAFT listing has no revision to submit — submitListingRevision must not be called")
			}
			trpcData(w, map[string]any{"publishRequestId": "alpr_zzz_offsite"})
		case strings.Contains(r.URL.Path, "beginListingRevision"):
			f.revisionCalls.Add(1)
			if !live {
				t.Errorf("a DRAFT listing has no shadow revision — beginListingRevision must not be called")
				trpcData(w, map[string]any{"shadowId": "apl_shadow", "created": true})
				return
			}
			assertListingTarget(t, r, offsiteListingID)
			// created=false — the shadow already exists, which is the state
			// `submit-revision` requires (created=true means nothing was staged
			// and it refuses). The attach paths do not read this flag.
			trpcData(w, map[string]any{"shadowId": offsiteShadowID, "created": false})
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			f.appCalls.Add(1)
			t.Errorf("the kind probe ran, so a command that should have resolved did not: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// TestNonNotFoundSubmissionFailuresSkipTheSlugFallback — the fallback is gated on
// the not-found KIND, exactly as the diagnostic probe is. A 403 from the
// invite-gated submissions route, or a 5xx, says nothing about whether a listing
// exists; spending a second request on it would turn a real error into a guess,
// and a fallback that then RESOLVED would report success for a command whose
// authorization actually failed.
//
// The zero-call assertion is the load-bearing one: widening the gate to "the
// command failed" is the natural-looking edit this exists to catch, and nothing
// in the error text would reveal it.
func TestNonNotFoundSubmissionFailuresSkipTheSlugFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		wantIs error
	}{
		{"403 from the invite-gated submissions route", http.StatusForbidden, `{"error":"not an apps author"}`, civitai.ErrUnauthorized},
		{"500 from the submissions route", http.StatusInternalServerError, `{"error":"boom"}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var listingCalls, appCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "getMyListingForApp"):
					listingCalls.Add(1)
					// If the gate ever widens, RESOLVE — so the test fails by
					// reporting a bogus success rather than by a count alone.
					trpcData(w, listingRefBody(offsiteListingID, "draft"))
				case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
					appCalls.Add(1)
					_, _ = w.Write([]byte(appDetailJSON(offsiteSlugA, "offsite", offsiteURLA)))
				default:
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer srv.Close()
			listingEnv(t, srv.URL)

			_, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
			if err == nil {
				t.Fatal("a non-not-found submissions failure must stay an error")
			}
			if errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("a %d must not be reclassified as not-found: %v", tc.status, err)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("the original classification must survive, got %T: %v", err, err)
			}
			if strings.Contains(err.Error(), "OFFSITE") {
				t.Errorf("a %d must not be rewritten into a claim about the app's kind:\n%s", tc.status, err)
			}
			if n := listingCalls.Load(); n != 0 {
				t.Errorf("only a not-found may fall back to the slug selector; it ran %d times", n)
			}
			if n := appCalls.Load(); n != 0 {
				t.Errorf("only a not-found may be probed; the store route ran %d times", n)
			}
		})
	}
}

// TestOnsiteHappyPathMakesNoExtraCall is the cost constraint on outcome 1: an
// app that HAS a submission resolves in exactly the two requests it always did.
//
// TestSuccessfulCommandsMakeNoStoreProbe holds the same line for the diagnostic
// probe; this is its sibling for the by-slug fallback, and it is a different
// question — a fallback wired BEFORE the submission lookup, or one that ran
// unconditionally and discarded its answer, would leave that test green and this
// one red. The `appBlockId` assertion is what pins WHICH selector the single
// call used, so a fallback that simply replaced the onsite path is caught too.
func TestOnsiteHappyPathMakesNoExtraCall(t *testing.T) {
	var subCalls, listingCalls atomic.Int32
	var lastInput atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			subCalls.Add(1)
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			listingCalls.Add(1)
			lastInput.Store(r.URL.Query().Get("input"))
			trpcData(w, listingRefBody("listing_1", "draft"))
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId": "listing_1", "slug": "my-app", "status": "draft",
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
		t.Fatalf("PREMISE BROKEN — the onsite happy path must succeed: %v", err)
	}
	if n := subCalls.Load(); n != 1 {
		t.Errorf("the submission lookup must run exactly once, ran %d times", n)
	}
	if n := listingCalls.Load(); n != 1 {
		t.Errorf("a resolved submission must not also pay for the by-slug fallback; getMyListingForApp ran %d times", n)
	}
	in, _ := lastInput.Load().(string)
	if !strings.Contains(in, `"appBlockId":"block_1"`) {
		t.Errorf("the onsite path must still select by the submission's appBlockId; input was %q", in)
	}
}

// ---------------------------------------------------------------------------
// The new behaviour
// ---------------------------------------------------------------------------

// TestAppListingOffsiteRefusesPrecisely is what is LEFT of #422's `app listing`
// half once outcome 1 shipped: the OLD-SERVER case.
//
// 🔴 ITS PREMISE CHANGED AND THE TEST WAS RE-POINTED, NOT WEAKENED. It used to
// assert the refusal for *any* offsite app, because no CLI path existed. That
// premise is now false — TestOffsiteAppListingResolvesBySlug drives the same
// slug to SUCCESS against a server carrying civitai/civitai#3989. What this
// server models instead is a Civitai WITHOUT it (or an app with no listing row):
// the by-slug lookup 404s, both lookups have missed, and the refusal is the only
// thing left to say. The `listingCalls` assertion below is what makes that a
// real premise rather than a name — without it this passes for a CLI that never
// tries the fallback at all, which is the exact regression outcome 1 undoes.
//
// 🔴 THE RETIRED ABSOLUTE IS PINNED BY THE WHOLE STRING, NOT BY BANNING A
// SPELLING — AND THE SPELLED VERSION WAS WALKABLE. This test used to check only
// `!strings.Contains(msg, "cannot be addressed from this CLI")`. Measured:
// re-inserting the same false absolute as a SYNONYM — "can never be reached from
// this CLI" — SURVIVED the entire suite. That is the #367 / #382 / #393 class
// this file's own header names, arrived at from the prose side: a guard on WORDS
// is walked by REWORDING, so the guard has to be on the whole normalised string.
// A cosmetic reword now fails this test; that is the price of a machine-readable
// claim, and the fix is to update the literal AFTER re-reading the copy against
// the code, never to weaken the comparison back to a keyword.
func TestAppListingOffsiteRefusesPrecisely(t *testing.T) {
	ps := newAppProbeServer(t, bothOffsiteAndOnsite())

	_, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	if err == nil {
		t.Fatal("expected an error when BOTH lookups miss")
	}
	msg := err.Error()

	// The defect: the old message's next step.
	if strings.Contains(msg, "run `civitai app submit` first") {
		t.Errorf("the dead-end advice is the regression — `app submit` cannot succeed for an offsite app:\n%s", msg)
	}
	// The retired absolute, pinned STRUCTURALLY — see wantOffsiteListingRefusal.
	if got := normaliseMessage(msg); got != wantOffsiteListingRefusal {
		t.Errorf("the `app listing` refusal is pinned WHOLE, not by banned spellings (civitai/cli#367, #382, #393).\n"+
			"If you changed the copy on purpose, re-read it against the code and update the literal.\n want: %s\n got:  %s",
			wantOffsiteListingRefusal, got)
	}
	for _, want := range []string{
		"OFFSITE app",
		offsiteSlugA,
		offsiteURLA,
		"could not reach its store listing here",
		"`civitai app view " + offsiteSlugA + "`",
		"App-store listing UI on civitai.com",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got:\n%s", want, msg)
		}
	}
	// PREMISE: the fallback really ran and really missed. Without this the test
	// is green for a CLI that never asks.
	if n := ps.listingCalls.Load(); n != 1 {
		t.Errorf("the by-slug fallback must have been attempted exactly once before refusing, ran %d times", n)
	}
	// House rule: an error names a real next command (see
	// errors_name_next_command_test.go).
	if !strings.Contains(msg, "civitai app view") {
		t.Errorf("the refusal must name a next command that WORKS; got:\n%s", msg)
	}
	// AGENTS.md item 7 — asserted with errors.Is, because the sentinel carries
	// no visible text. The app exists but the submission this command resolves
	// through does not, so the code stays 4.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("the offsite refusal must stay not-found (exit 4), got %T: %v", err, err)
	}
	if n := ps.appCalls.Load(); n != 1 {
		t.Errorf("the store probe must run exactly once on the error path, ran %d times", n)
	}
}

// TestAppStatusOffsiteRefusesPrecisely is the other half, and the assertions
// below are what stops the two messages being collapsed into one: `app status`
// lists BLOCK SUBMISSIONS, an offsite app genuinely has none, and nothing about
// that command is about store media.
func TestAppStatusOffsiteRefusesPrecisely(t *testing.T) {
	newAppProbeServer(t, bothOffsiteAndOnsite())

	_, _, err := run(t, "app", "status", offsiteSlugB)
	if err == nil {
		t.Fatal("expected an error for an offsite app")
	}
	msg := err.Error()

	if strings.Contains(msg, "run `civitai app submit` first") {
		t.Errorf("the dead-end advice is the regression:\n%s", msg)
	}
	for _, want := range []string{
		"OFFSITE app",
		offsiteSlugB,
		offsiteURLB,
		"no block submissions",
		"`civitai app view " + offsiteSlugB + "`",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got:\n%s", want, msg)
		}
	}
	// `app status` says nothing about listing media — that is the listing
	// command's story, and telling it here would send the reader to a surface
	// this command has no business naming.
	if strings.Contains(msg, "App-store listing UI") {
		t.Errorf("`app status` must not talk about the listing UI:\n%s", msg)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("the offsite refusal must stay not-found (exit 4), got %T: %v", err, err)
	}
}

// TestOffsiteRefusalsDifferPerCommand pins the DISTINCTION itself. Both messages
// are produced for the same app, from the same probe result, and they must not
// converge: one reports a CLI gap over a listing that exists, the other reports
// the plain truth that there are no submissions. A future "let's share the
// string" refactor is exactly what this fails on.
func TestOffsiteRefusalsDifferPerCommand(t *testing.T) {
	d := &civitai.AppDetail{Slug: serverEchoSlug(offsiteSlugA), Kind: "offsite"}
	d.KindData.Kind = "offsite"
	d.KindData.ExternalURL = offsiteURLA

	listing := offsiteListingRefusal(offsiteSlugA, d)
	status := offsiteStatusRefusal(offsiteSlugA, d)
	if listing == status {
		t.Fatal("the two commands must not share one message — that difference IS #422 outcome 2")
	}
	if !strings.Contains(listing, "civitai app listing") || strings.Contains(listing, "no block submissions") {
		t.Errorf("the listing refusal must be about the LISTING:\n%s", listing)
	}
	if !strings.Contains(status, "no block submissions") || strings.Contains(status, "App-store listing UI") {
		t.Errorf("the status refusal must be about SUBMISSIONS:\n%s", status)
	}
}

// TestOffsiteRefusalRendersTheAppsOwnExternalURL is the fixture-trap control.
// Two offsite apps with two DIFFERENT external URLs must produce two different
// messages: if the rendered URL could only ever be the one value the fixture
// supplies, a mutant that hardcodes a literal would survive a green suite.
func TestOffsiteRefusalRendersTheAppsOwnExternalURL(t *testing.T) {
	newAppProbeServer(t, bothOffsiteAndOnsite())

	_, _, errA := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	_, _, errB := run(t, "app", "listing", "status", "--slug", offsiteSlugB)
	if errA == nil || errB == nil {
		t.Fatal("expected errors for both offsite apps")
	}
	a, b := errA.Error(), errB.Error()
	if !strings.Contains(a, offsiteURLA) || strings.Contains(a, offsiteURLB) {
		t.Errorf("app A's message must carry A's URL and only A's:\n%s", a)
	}
	if !strings.Contains(b, offsiteURLB) || strings.Contains(b, offsiteURLA) {
		t.Errorf("app B's message must carry B's URL and only B's:\n%s", b)
	}
}

// TestOffsiteRegisteredAtRejectsUnsafeServerText — the external URL is SERVER
// text spliced into the middle of a CLI error, and safeTerm deliberately keeps
// `\n` and `\t`. A value that could break the sentence across lines, or that is
// not an absolute http(s) URL, is dropped and the message simply says less.
//
// 🔴 THE UNICODE ROWS ARE THE FIX FOR A GUARD THAT WAS SPELLED, NOT STRUCTURAL.
// The first cut banned four ASCII bytes and its comment claimed "no whitespace
// at all"; measured against that shipped function, U+00A0, U+2028, U+2029,
// U+3000 and U+205F all RENDERED, and U+2028/U+2029 are line separators — the
// exact sentence-breaking hazard the four-byte list was written to stop.
// saferune cannot backstop it: it subtracts White_Space from its strip class on
// purpose. Each row below is a rune that USED to render.
//
// 🔴 THE TWO REACHABILITY ROWS (`ftp` and `javascript://…`) EXIST BECAUSE THE
// SCHEME CLAUSE WAS UNREACHED. `javascript:alert(1)` and `file:///etc/passwd`
// both parse to an EMPTY Host, so `u.Host == ""` killed them first and deleting
// `u.Scheme != "http" && u.Scheme != "https"` survived a green suite. These two
// carry a real host, so the scheme clause is the ONLY thing that can reject
// them — the "an earlier check always wins" mutant case, closed by construction.
//
// 🔴 THE ACCEPT ROWS ARE A MUTATION CONTROL, NOT DECORATION, AND THAT IS WHY
// THEY ARE SO CROWDED. This test once had exactly two `want: true` rows —
// `https://ok.example/x` and its http twin — which between them contain no
// digit, no `%`, no `?`, no `#`, no `@`, no `~`, no port and no uppercase
// letter. Measured against that table, EVERY one of these survived a fully green
// suite: dropping `%`, `?`, `@`, `#`, `~` or the digit `9` from the accepted set
// (each of which silently stops rendering every URL with percent-encoding, a
// query string or a digit in it), and — worse — replacing the allowlist
// wholesale with a DENYLIST of exactly the runes the reject rows below name,
// which is a verbatim regression to the spelled guard this function exists to
// replace. So the four accept rows partition the allowlist between them and are
// annotated with which runes each is the only witness for: an accept row's job
// is to go RED when a rune leaves the accepted set.
//
// 🔴 AND THE BACKTICK / `{` / `|` REJECT ROWS ARE WHAT A DENYLIST CANNOT PASS.
// All three parse cleanly with a real host and an https scheme (measured), so
// `isURIRune` is the ONLY thing that can reject them — the same reachability
// argument as the scheme rows above. The backtick is the load-bearing one: the
// refusal messages use backticks as code-span delimiters, so an off-by-one
// widening of `'a'..'z'` (whose predecessor IS the backtick) would let a server
// URL forge a span in a sentence the CLI wrote.
func TestOffsiteRegisteredAtRejectsUnsafeServerText(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		want      bool
	}{
		{"plain https", "https://ok.example/x", true},
		{"plain http", "http://ok.example/x", true},
		// The three rows above carry no digit and no punctuation beyond `:/.`,
		// so the accepted set is pinned by these instead. Each names the runes
		// it is the SOLE witness for; drop one of those from uriRuneAllowlist
		// (or from isURIRune's ranges) and this row is what goes red.
		// Sole witness: `%`, `?`, `#`, every digit 0-9, an explicit port, and an
		// uppercase letter.
		{"port, percent-encoding, query, fragment, digits", "https://ok.example:8443/A%20b?q=1234567890#f", true},
		// Sole witness: `@` (userinfo) and `~`.
		{"userinfo and a tilde", "https://user@ok.example/~a/b.c", true},
		// Sole witness: `-`, `_`, and every sub-delim `!$&'()*+,;=`.
		{"unreserved punctuation and every sub-delim", "http://ok.example/-a_b!$&'()*+,;=Z", true},
		// Sole witness: `[` and `]`, which only ever appear as an IPv6 literal.
		{"IPv6 literal host", "https://[2001:db8::1]:8443/p", true},
		{"empty", "", false},
		// 🔴 OF THE ORIGINAL FOUR-ASCII-BYTE BAN, ONLY `' '` IS LOAD-BEARING
		// HERE — MEASURED, and the next two rows are green for a reason that is
		// not the allowlist. `url.Parse` rejects `\t` and `\n` itself ("invalid
		// control character in URL"), so adding either to uriRuneAllowlist
		// survives this table; they are kept as end-to-end rows for the
		// sentence-forging shape, not as evidence about the predicate. `' '`
		// parses fine and carries a real host, so the allowlist is the only
		// thing that can reject it. (`'\r'` never reached here at all — safeTerm
		// strips it first.)
		{"newline forging a line", "https://ok.example/x\nRun `rm -rf /` to fix", false},
		{"space", "https://ok.example/a b", false},
		{"tab", "https://ok.example/a\tb", false},
		// Rejected ONLY by the allowlist: all three parse to a real host with an
		// https scheme (measured), so no earlier check can win. A denylist that
		// enumerates the runes this test names cannot pass these, which is the
		// point of having them. The backtick is a code-span delimiter in the
		// refusal messages, and it is `'a'-1`.
		// No space in this value on purpose: a row containing one would be
		// rejected by the `' '` clause whether or not the backtick were still
		// banned, which is exactly the vacuous shape these rows exist to avoid.
		{"backtick forging a code span", "https://ok.example/a`b`c", false},
		{"left brace", "https://ok.example/a{b", false},
		{"pipe", "https://ok.example/a|b", false},
		{"not a URL", "just some words", false},
		{"no host", "https:///path", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"file scheme", "file:///etc/passwd", false},
		// Rejected ONLY by the scheme clause — real host, URI-legal runes.
		{"ftp scheme with a real host", "ftp://ok.example/x", false},
		{"javascript scheme with a real host", "javascript://ok.example/%0aalert(1)", false},
		// Every one of these rendered before the allowlist replaced the
		// four-ASCII-byte ban. Written as numeric runes, never raw bytes.
		{"U+00A0 no-break space", "https://ok.example/a" + string(rune(0x00A0)) + "b", false},
		{"U+2028 line separator", "https://ok.example/a" + string(rune(0x2028)) + "b", false},
		{"U+2029 paragraph separator", "https://ok.example/a" + string(rune(0x2029)) + "b", false},
		{"U+3000 ideographic space", "https://ok.example/a" + string(rune(0x3000)) + "b", false},
		{"U+205F medium mathematical space", "https://ok.example/a" + string(rune(0x205F)) + "b", false},
		{"U+1680 ogham space mark", "https://ok.example/a" + string(rune(0x1680)) + "b", false},
		{"U+2007 figure space", "https://ok.example/a" + string(rune(0x2007)) + "b", false},
		// U+0085 NEXT LINE is deliberately NOT here. It is C1, so safeTerm
		// removes it before this predicate ever sees it and the URL renders
		// (without it) — a STRIP, not a rejection. Its row lives in
		// TestOffsiteRefusalSanitizesControlChars, which asserts that.
		// Measured, not assumed: asserting rejection here failed.
		// Not whitespace at all, so `unicode.IsSpace` would have kept it: a
		// full-width solidus that reads as a path separator it is not. The
		// allowlist is what makes this a non-question.
		{"U+FF0F fullwidth solidus", "https://ok.example" + string(rune(0xFF0F)) + "evil", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &civitai.AppDetail{}
			d.KindData.ExternalURL = tc.url
			got := offsiteRegisteredAt(d)
			if tc.want != (got != "") {
				t.Fatalf("offsiteRegisteredAt(%q) = %q, wanted rendered=%v", tc.url, got, tc.want)
			}
			if tc.want && !strings.Contains(got, tc.url) {
				t.Errorf("a rendered target must carry the URL: %q", got)
			}
		})
	}
	// The whole message survives a dropped URL — the refusal is still complete.
	d := &civitai.AppDetail{}
	d.KindData.ExternalURL = "not a url"
	msg := offsiteListingRefusal(offsiteSlugA, d)
	if !strings.Contains(msg, "OFFSITE app,") || !strings.Contains(msg, "civitai app view") {
		t.Errorf("with no printable target the message must still be whole:\n%s", msg)
	}
}

// TestOffsiteRefusalSanitizesControlChars is the STRIPPING half, and it did not
// exist until it was found missing: replacing `safeTerm(d.KindData.ExternalURL)`
// with the raw field in offsiteRegisteredAt left the whole `internal/cmd` suite
// green. TestOffsiteRegisteredAtRejectsUnsafeServerText only ever asserted
// REJECTION — that a hostile value produces nothing — which a mutant that drops
// the strip satisfies trivially, because dropping it makes MORE values reject.
//
// The property this pins is the opposite one: a value that is hostile ONLY in
// runes safeTerm removes must still render, WITHOUT them. That is what makes the
// call load-bearing, and it is the house pattern of the five
// …SanitizesControlChars tests in sanitize_render_test.go.
//
// The mutant it kills, concretely: with the raw field, U+202E / U+200B / ESC are
// not RFC 3986 runes, so isURIRune rejects the whole value and the ` (registered
// at …)` clause vanishes — every `wantRendered` assertion below fails.
func TestOffsiteRefusalSanitizesControlChars(t *testing.T) {
	// Built from numeric rune values, never typed as raw bytes — the same rule
	// sanitize_render_test.go's escRune/belRune/c1CSI follow, and the one
	// staticcheck's ST1018 enforces.
	const (
		rtlOverride = string(rune(0x202E)) // RIGHT-TO-LEFT OVERRIDE — reverses what follows
		zeroWidth   = string(rune(0x200B)) // ZERO WIDTH SPACE — invisible, splits a hostname
	)
	for _, tc := range []struct {
		name, url, wantRendered string
	}{
		{"bidi override in the path", "https://ok.example/a" + rtlOverride + "b", "https://ok.example/ab"},
		{"zero-width space in the host", "https://ok" + zeroWidth + ".example/x", "https://ok.example/x"},
		{"ANSI colour introducer", "https://ok.example/x" + escRune + "[31m", "https://ok.example/x[31m"},
		{"C1 CSI", "https://ok.example/y" + c1CSI + "[2K", "https://ok.example/y[2K"},
		{"BEL terminating an OSC", "https://ok.example/z" + belRune, "https://ok.example/z"},
		{"U+0085 next line", "https://ok.example/a" + string(rune(0x0085)) + "b", "https://ok.example/ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &civitai.AppDetail{}
			d.KindData.ExternalURL = tc.url
			got := offsiteRegisteredAt(d)
			want := " (registered at " + tc.wantRendered + ")"
			if got != want {
				t.Fatalf("offsiteRegisteredAt(%q) = %q, want %q — the hostile runes must be STRIPPED, "+
					"not made to reject the whole URL", tc.url, got, want)
			}
			assertNoControlBytes(t, "the offsite registered-at clause", got)
		})
	}

	// End to end, through the real command tree: the refusal a user actually
	// sees carries the stripped URL and no control bytes.
	fixtures := bothOffsiteAndOnsite()
	hostile := "https://ok.example/live" + rtlOverride + escRune + "[1A" + escRune + "[2K"
	fixtures[offsiteSlugA] = appDetailJSON(offsiteSlugA, "offsite", hostile)
	newAppProbeServer(t, fixtures)

	_, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	if err == nil {
		t.Fatal("expected an error for an offsite app")
	}
	msg := err.Error()
	assertNoControlBytes(t, "the offsite refusal", msg)
	if !strings.Contains(msg, "(registered at https://ok.example/live[1A[2K)") {
		t.Errorf("the refusal must render the app's URL with the control runes removed; got:\n%q", msg)
	}
}

// ---------------------------------------------------------------------------
// Everything that must keep TODAY's message
// ---------------------------------------------------------------------------

// TestOnsiteNeverSubmittedKeepsTodaysMessage is THE regression case. An onsite
// app that exists in the store but has never been submitted is the situation the
// existing message was written for (civitai/cli#363), and `civitai app submit`
// really is its next step.
//
// 🔴 LABEL IT HONESTLY: the MESSAGE assertion is an INVARIANT guard — it is
// green at `1b20b99` too, because the message it pins is the one that already
// shipped. What is red at base is the PREMISE below (`appCalls == 1`): before
// this change nothing probed the store at all. Both are worth having, and only
// the premise makes the message assertion mean "the probe ran and said onsite"
// rather than "no probe exists".
// 🔴 AND SINCE #422 OUTCOME 1 IT CARRIES A SECOND PREMISE, `wantListingCalls`.
// `app listing` now tries the by-slug selector before refusing, so this row is
// the case where THAT missed too; `app status` never resolves a listing at all
// and must stay at zero. Asserting the number per row is what stops the fallback
// silently leaking onto the status path.
func TestOnsiteNeverSubmittedKeepsTodaysMessage(t *testing.T) {
	for _, tc := range []struct {
		name             string
		args             []string
		wantListingCalls int32
	}{
		{"app listing status", []string{"app", "listing", "status", "--slug", onsiteSlug}, 1},
		{"app status <slug>", []string{"app", "status", onsiteSlug}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected the no-submission error")
			}
			if err.Error() != wantNoSubmissionMsg(onsiteSlug) {
				t.Errorf("an onsite app that was never submitted must keep TODAY's message.\nwant: %s\ngot:  %s",
					wantNoSubmissionMsg(onsiteSlug), err.Error())
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
			// PREMISE: the probe really ran and really said "onsite" — without
			// this the assertion above passes for a probe that never fired,
			// which is the same green for a completely different reason.
			if n := ps.appCalls.Load(); n != 1 {
				t.Errorf("the probe must have run exactly once (it is what decided onsite), ran %d times", n)
			}
			if n := ps.listingCalls.Load(); n != tc.wantListingCalls {
				t.Errorf("getMyListingForApp ran %d times, want %d — `app listing` falls back to the slug "+
					"selector before refusing, `app status` resolves no listing at all", n, tc.wantListingCalls)
			}
		})
	}
}

// TestUnknownSlugKeepsTodaysMessage — a slug that is no app at all. The store
// route 404s, nothing is established, and the message is unchanged. Same
// labelling as above: the message half is an invariant guard, the `appCalls`
// premise is the half that is red at base.
func TestUnknownSlugKeepsTodaysMessage(t *testing.T) {
	const unknown = "definitely-not-an-app-zzz"
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"app listing status", []string{"app", "listing", "status", "--slug", unknown}},
		{"app status <slug>", []string{"app", "status", unknown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected the no-submission error")
			}
			if err.Error() != wantNoSubmissionMsg(unknown) {
				t.Errorf("an unknown slug must keep TODAY's message.\nwant: %s\ngot:  %s",
					wantNoSubmissionMsg(unknown), err.Error())
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
			if n := ps.appCalls.Load(); n != 1 {
				t.Errorf("the probe must have run and 404ed, ran %d times", n)
			}
		})
	}
}

// TestProbeFailureKeepsTodaysMessage — every way the diagnostic can fail to
// answer collapses to the unchanged error. A network failure of a DIAGNOSTIC
// must never replace a real error with a confusing one.
func TestProbeFailureKeepsTodaysMessage(t *testing.T) {
	const slug = "probe-fails-zzz"
	for _, tc := range []struct {
		name string
		// serve answers GET /api/v1/apps/{slug}.
		serve http.HandlerFunc
	}{
		{"500 from the store route", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}},
		{"403 — not visible to this caller", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		}},
		{"garbage body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>not json</html>`))
		}},
		{"a kind this CLI does not recognise", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(appDetailJSON(slug, "marketplace-v2", "https://future.zzz/x")))
		}},
		{"no kind at all", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"slug":"` + slug + `","name":"x"}`))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
					tc.serve(w, r)
					return
				}
				// The by-slug arm 404s: this test is about the PROBE failing,
				// so the fallback must miss the way a real Civitai misses (no
				// listing row) rather than by handing getMyListingForApp the
				// submissions envelope, which is an undecodable body and a
				// different failure entirely. See emptySubmissionsServer.
				if strings.Contains(r.URL.Path, "getMyListingForApp") {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":{"json":{"message":"No listing found for this app"}}}`))
					return
				}
				_, _ = w.Write([]byte(`{"submissions":[]}`))
			}))
			defer srv.Close()
			listingEnv(t, srv.URL)

			_, _, err := run(t, "app", "listing", "status", "--slug", slug)
			if err == nil {
				t.Fatal("expected the no-submission error")
			}
			if err.Error() != wantNoSubmissionMsg(slug) {
				t.Errorf("a probe that could not answer must leave the message alone.\nwant: %s\ngot:  %s",
					wantNoSubmissionMsg(slug), err.Error())
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
		})
	}
}

// TestNonNotFoundErrorsAreNeverProbed — the enrichment is gated on the
// not-found KIND, not on "the command failed". A 403 from the invite-gated
// submissions route, a 5xx or a transport failure are different errors with
// different exit codes, and none of them is evidence about an app's kind: an
// offsite app whose submissions lookup 403s must still report the 403, on exit
// 3, and must not spend a request finding that out.
func TestNonNotFoundErrorsAreNeverProbed(t *testing.T) {
	var appCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
			appCalls.Add(1)
			_, _ = w.Write([]byte(appDetailJSON(offsiteSlugA, "offsite", offsiteURLA)))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"not an apps author"}`))
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"app listing status", []string{"app", "listing", "status", "--slug", offsiteSlugA}},
		{"app status <slug>", []string{"app", "status", offsiteSlugA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected the 403")
			}
			if strings.Contains(err.Error(), "OFFSITE") {
				t.Errorf("a 403 must not be rewritten into a claim about the app's kind:\n%s", err.Error())
			}
			if !errors.Is(err, civitai.ErrUnauthorized) {
				t.Errorf("a 403 must stay exit 3, got %T: %v", err, err)
			}
		})
	}
	if n := appCalls.Load(); n != 0 {
		t.Errorf("a non-not-found failure must not be probed at all; the store route ran %d times", n)
	}
}

// TestOffsiteProbeDoesNotHoldTheCommandOpen — the probe gets a deadline of its
// own, SHORTER than the read client's 30s budget. The command has already
// failed; a hung store route must not keep it alive for the full budget before
// printing the message it would have printed immediately.
//
// The elapsed assertion is what makes this more than a wiring check: removing
// the context.WithTimeout in offsiteApp leaves this test hanging on the client's
// own budget (30s per attempt) instead, which no server-side assertion can see.
func TestOffsiteProbeDoesNotHoldTheCommandOpen(t *testing.T) {
	const slug = "hangs-zzz"
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
			<-block
			return
		}
		// 🔴 THE BY-SLUG ARM MUST 404, NOT FALL THROUGH TO THE CATCH-ALL. This
		// handler used to answer EVERY path with the submissions envelope, so
		// getMyListingForApp received `{"submissions":[]}` and failed to DECODE
		// — a failure that is not a not-found. Once resolveListing stopped
		// swallowing the fallback's own error that surfaced as `unexpected
		// appListings.getMyListingForApp response`, which is the honest report
		// of a server answering nonsense but is not the server this test is
		// about. A 404 is what a Civitai with no listing row for the app really
		// says, and it is what keeps this test measuring the PROBE's deadline.
		if strings.Contains(r.URL.Path, "getMyListingForApp") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"No listing found for this app"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"submissions":[]}`))
	}))
	// 🔴 ORDER: httptest.Server.Close BLOCKS until every outstanding handler
	// returns, and this one is parked on <-block. Defers run LIFO, so the
	// release must be registered LAST to run FIRST — registering it as a
	// t.Cleanup instead deadlocks the whole package, because Cleanup runs after
	// the deferred Close.
	defer srv.Close()
	defer close(block)
	listingEnv(t, srv.URL)

	old := offsiteProbeTimeout
	offsiteProbeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { offsiteProbeTimeout = old })

	// 🔴 os.Stderr, NOT the cobra buffer. The read client's transient-retry
	// notices go to the process's real stderr, so the only way to see whether
	// the probe is silent is to hold the file it writes to. Without this, the
	// `client.Stderr = io.Discard` line in offsiteApp is unpinned and deleting
	// it leaves the suite green while `network error from Civitai, retrying
	// (2/4)…` prints ABOVE the real error on every probe timeout — measured.
	stderrFile := filepath.Join(t.TempDir(), "stderr.txt")
	f, ferr := os.Create(stderrFile)
	if ferr != nil {
		t.Fatalf("create stderr capture: %v", ferr)
	}
	realStderr := os.Stderr
	os.Stderr = f
	restore := func() { os.Stderr = realStderr; _ = f.Close() }
	defer restore()

	start := time.Now()
	_, _, err := run(t, "app", "listing", "status", "--slug", slug)
	elapsed := time.Since(start)
	restore()

	captured, rerr := os.ReadFile(stderrFile)
	if rerr != nil {
		t.Fatalf("read stderr capture: %v", rerr)
	}
	if strings.Contains(string(captured), "retrying") {
		t.Errorf("the diagnostic probe must be SILENT — its retry notices would sit above the real error:\n%s", captured)
	}

	if err == nil {
		t.Fatal("expected the no-submission error")
	}
	if err.Error() != wantNoSubmissionMsg(slug) {
		t.Errorf("a probe that timed out must leave the message alone.\nwant: %s\ngot:  %s",
			wantNoSubmissionMsg(slug), err.Error())
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("exit code must stay 4, got %T: %v", err, err)
	}
	// 5s is far below the read client's own 30s-per-attempt budget, and far
	// above the 50ms this should really take — a threshold that separates the
	// two without being a timing race.
	if elapsed > 5*time.Second {
		t.Errorf("the probe inherited the client's budget instead of its own: the command took %s", elapsed)
	}
}

// ---------------------------------------------------------------------------
// The probe costs nothing on a path that did not fail
// ---------------------------------------------------------------------------

// TestSuccessfulCommandsMakeNoStoreProbe is constraint 1 of #422 outcome 2: the
// diagnostic adds NO request to any successful command. Both surfaces are driven
// to a real success and the store route's call count is asserted to be zero.
func TestSuccessfulCommandsMakeNoStoreProbe(t *testing.T) {
	var appCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			appCalls.Add(1)
			_, _ = w.Write([]byte(appDetailJSON("my-app", "offsite", offsiteURLA)))
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft", "contentRating": "g", "hasPendingRevision": false})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId": "listing_1", "slug": "my-app", "status": "draft", "hasPendingRevision": false, "shadowId": nil,
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
		t.Fatalf("PREMISE BROKEN — the happy path must succeed: %v", err)
	}
	if _, _, err := run(t, "app", "status", "my-app"); err != nil {
		t.Fatalf("PREMISE BROKEN — the happy path must succeed: %v", err)
	}
	if n := appCalls.Load(); n != 0 {
		t.Errorf("a successful command must not probe the store route; it ran %d times", n)
	}
}

// TestAppStatusByIDDoesNotProbeTheStore — the `--id` selector names a publish
// request, not an app, so there is no slug to ask about. The precedence mirrors
// submissionsURL/submissionSubject: the id wins even when a slug is ALSO given,
// so neither spelling may probe.
func TestAppStatusByIDDoesNotProbeTheStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"--id alone", []string{"app", "status", "--id", "pubreq_01ZZZ"}},
		{"--id wins over a slug", []string{"app", "status", "--id", "pubreq_01ZZZ", offsiteSlugA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected a not-found error for an unknown publish request")
			}
			if strings.Contains(err.Error(), "OFFSITE") {
				t.Errorf("an `--id` lookup must not be answered with a claim about an app:\n%s", err.Error())
			}
			if n := ps.appCalls.Load(); n != 0 {
				t.Errorf("an `--id` lookup has no slug to probe; the store route ran %d times", n)
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
		})
	}
}

// TestOffsiteRefusalReachesEverySubcommand — resolveListing is the one funnel
// for `app listing`, so every subcommand that resolves a slug must produce the
// refusal, not just `status`. This is the reachability half of the mutation
// check: the branch is not reached only by the one command a single case drives.
//
// 🔴 SINCE #422 OUTCOME 1 THIS PINS THE NARROWED CASE — a server WITHOUT
// civitai/civitai#3989, modelled by newAppProbeServer's 404-ing tRPC arm. Its
// mirror image, the same seven subcommands against a server that HAS it, is
// TestOffsiteRepairReachesEverySubcommand. Both are needed: this one alone would
// be green for a CLI with no fallback, and that one alone would be green for a
// CLI that had deleted the refusal.
//
// 🔴 ALL SIX SUBCOMMANDS, NOT FOUR. The first cut drove `status`, `set-icon`,
// `set-cover` and `rm-screenshot` and left `add-screenshot` (runSetMedia, the
// same shared flow as set-icon/set-cover) and `reorder`
// (resolveListingRefFromFlags, the same helper as rm-screenshot) unexercised —
// so the claim "every subcommand" was two thirds measured. `app listing` has
// exactly these SEVEN since civitai/cli#436 added `submit-revision` (which
// resolves through resolveListingRefFromFlags, the same helper as
// rm-screenshot); if an eighth is added it belongs here.
func TestOffsiteRefusalReachesEverySubcommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(t *testing.T) []string
	}{
		{"status", func(*testing.T) []string {
			return []string{"app", "listing", "status", "--slug", offsiteSlugA}
		}},
		{"set-icon", func(t *testing.T) []string {
			return []string{"app", "listing", "set-icon", writePNG(t, 512, 512), "--slug", offsiteSlugA, "-y"}
		}},
		{"set-cover", func(t *testing.T) []string {
			return []string{"app", "listing", "set-cover", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}},
		{"add-screenshot", func(t *testing.T) []string {
			return []string{"app", "listing", "add-screenshot", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}},
		{"rm-screenshot", func(*testing.T) []string {
			return []string{"app", "listing", "rm-screenshot", "alsc_1", "--slug", offsiteSlugA}
		}},
		{"reorder", func(*testing.T) []string {
			return []string{"app", "listing", "reorder", "alsc_2", "alsc_1", "--slug", offsiteSlugA}
		}},
		{"submit-revision", func(*testing.T) []string {
			return []string{"app", "listing", "submit-revision", "--slug", offsiteSlugA}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args(t)...)
			if err == nil {
				t.Fatal("expected an error for an offsite app")
			}
			if !strings.Contains(err.Error(), "OFFSITE app") {
				t.Errorf("every `app listing` subcommand resolves through resolveListing and must refuse precisely:\n%s", err.Error())
			}
		})
	}
}
