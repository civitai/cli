package cmd

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// THE SHARED SLUG PREDICATE AT ITS THREE cmd-SIDE CALL SITES.
//
// One question — "is this submissions row the app I am talking about?" — was
// answered four ways: normalising at internal/cmd/approved_version.go (#414) and
// exact at the other three. The whole suite was green over a cased or padded
// blockId at ALL FOUR, including the one that already normalised: #414 shipped
// the predicate and its doc comment but no fixture that exercised it. These
// cases are the missing fixtures.
//
// 🔴 WHICH OF THESE ARE REGRESSION COVERAGE AND WHICH ARE INVARIANT GUARDS —
// stated, because the two are not worth the same, and measured rather than
// asserted. "Base" below means origin/main's call sites with appapi.SameSlug
// present but uncalled, so the only variable is the call site itself.
//
//	site                                    converted?          at base   kind
//	internal/cmd/apps.go ownedSubmission    yes (== -> SameSlug)  RED     regression
//	internal/cmd/app_status.go drift check  yes (!= -> SameSlug)  RED     regression
//	internal/cmd/approved_version.go        no, already SameSlug  GREEN   invariant
//
// 🔴 SITE 1's CASE IS GREEN AT BASE AND IS NOT REGRESSION COVERAGE. An earlier
// draft of this header claimed it was red at base; the base run refuted that,
// and the claim is corrected here rather than quietly dropped. Its value is
// different and worth having anyway: #414 shipped that normalisation with a doc
// comment and NO fixture, so reverting it to `==` left the whole suite green.
// The case below kills that mutant, with its own named reason. Coverage this PR
// adds for pre-existing behaviour — not a behaviour change at that site.

// slugRespellings are the spellings that name the SAME app as the canonical
// slug. Every one of them is invalid per schema/app-block.manifest.schema.json's
// `^[a-z][a-z0-9-]*[a-z0-9]$`, which is the reason accepting them cannot collide
// with a second registered app — the fold-and-trim is the identity map on the
// set of valid slugs.
func slugRespellings(canonical string) []string {
	return []string{
		strings.ToUpper(canonical),
		strings.ToUpper(canonical[:1]) + canonical[1:],
		" " + canonical,
		canonical + " ",
		" " + strings.ToUpper(canonical[:1]) + canonical[1:] + " ",
	}
}

// slugRejectsFor builds the four negative controls for a canonical slug: an
// unrelated app, an extension of it, a prefix of it, and the empty string. A
// widened predicate that passed every positive case above fails at least one of
// these, and each names the over-wide implementation it catches.
func slugRejectsFor(canonical string) []struct{ name, spelling, breaks string } {
	return []struct{ name, spelling, breaks string }{
		{"unrelated", "gen-matrix", "a different app entirely"},
		{"extension", canonical + "-2", "prefix matching would accept a DIFFERENT registered app"},
		{"prefix", canonical[:strings.Index(canonical, "-")], "suffix/Contains matching would accept a shorter slug"},
		{"empty", "", "no slug names an app; empty must never be a wildcard"},
	}
}

// ---------------------------------------------------------------------------
// Site 1: internal/cmd/approved_version.go — highestApprovedVersion
// ---------------------------------------------------------------------------

// TestHighestApprovedVersionMatchesARespelledBlockID pins #414's normalisation,
// which shipped with a doc comment and no fixture. Reverting that site to `==`
// on origin/main left the entire suite green (measured), so this is the killing
// case for that mutant.
//
// The assertion is on slugRows as well as on the version, because the two fail
// differently: slugRows is what tells "this app has rows, none approved yet"
// apart from "rows came back, none of them for this app", and a slug-blind read
// collapses both into the silent branch.
func TestHighestApprovedVersionMatchesARespelledBlockID(t *testing.T) {
	const canonical = "custom-generators"
	for _, spelling := range slugRespellings(canonical) {
		t.Run(spelling, func(t *testing.T) {
			peak := highestApprovedVersion(driftSubs(
				[3]string{spelling, "0.5.2", "approved"},
				[3]string{spelling, "0.4.0", "rejected"},
			), canonical)
			if !peak.found {
				t.Fatalf("highestApprovedVersion found nothing for rows spelled %q against slug %q — "+
					"those are the same app, and missing them makes the #412 guard a SILENT no-op",
					spelling, canonical)
			}
			if peak.version != "0.5.2" {
				t.Errorf("peak.version = %q, want 0.5.2", peak.version)
			}
			if peak.slugRows != 2 {
				t.Errorf("peak.slugRows = %d, want 2 — both rows are this app's, and slugRows is what "+
					"distinguishes 'nothing approved yet' from 'no rows for this app'", peak.slugRows)
			}
		})
	}
}

// TestHighestApprovedVersionStillRejectsOtherSlugs is the negative control at
// site 1: found=false AND slugRows=0, because a row for another app must not
// even be counted as this app's.
func TestHighestApprovedVersionStillRejectsOtherSlugs(t *testing.T) {
	const canonical = "custom-generators"
	for _, r := range slugRejectsFor(canonical) {
		t.Run(r.name, func(t *testing.T) {
			peak := highestApprovedVersion(driftSubs(
				[3]string{r.spelling, "0.9.9", "approved"},
			), canonical)
			if peak.found {
				t.Errorf("a row for blockId %q answered the highest-approved lookup for %q with %q (%s) — "+
					"`app submit` would refuse a perfectly good version against another app's number",
					r.spelling, canonical, peak.version, r.breaks)
			}
			if peak.slugRows != 0 {
				t.Errorf("peak.slugRows = %d for blockId %q, want 0 — counting another app's row as this "+
					"app's row silences the blockId-mismatch signal", peak.slugRows, r.spelling)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Site 2: internal/cmd/apps.go — ownedSubmission
// ---------------------------------------------------------------------------

// respelledOwnedBody is the owned-slug listing with the ROW spelled differently
// from the slug the caller asked for. The request still narrows by the exact
// slug (appViewServer asserts that), so this is precisely the defence-in-depth
// scenario: the CLI asked correctly and the server echoed a spelling it does not
// today emit.
func respelledOwnedBody(spelling string) string {
	return `{"submissions":[{
	  "id":"pubreq_02H","blockId":"` + spelling + `","version":"0.2.0","status":"pending",
	  "deployState":"building","submittedAt":"2026-07-29T10:00:00Z","updatedAt":"2026-07-29T10:00:00Z",
	  "createdAt":"2026-07-29T10:00:00Z","liveUrl":null}]}`
}

// TestAppViewOwnedAdviceMatchesARespelledBlockID is site 2's red-at-base case.
// With `==`, the row fell through to nil — and nil is deliberately
// indistinguishable from "not yours" and "could not ask", so `app view` printed
// the bare 404 and the author lost the advice with nothing on screen saying why.
func TestAppViewOwnedAdviceMatchesARespelledBlockID(t *testing.T) {
	for _, spelling := range slugRespellings("buzz-generator") {
		t.Run(spelling, func(t *testing.T) {
			probes := appViewServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(respelledOwnedBody(spelling)))
			})

			_, _, err := run(t, "app", "view", "buzz-generator")
			if err == nil {
				t.Fatal("expected a not-found error")
			}
			// The exit-code contract is untouched by which branch answered.
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("404 must still classify as ErrNotFound (rc=4), got %T: %v", err, err)
			}
			if n := atomic.LoadInt32(probes); n != 1 {
				t.Fatalf("the ownership probe ran %d times, want 1 — without it nothing here is about the row scan", n)
			}
			msg := err.Error()
			for _, want := range appViewAdviceMarkers {
				if !strings.Contains(msg, want) {
					t.Errorf("a row spelled %q is the SAME app as buzz-generator, so the owned-slug advice "+
						"must fire; missing %q. Dropping it is silent: the caller sees only the bare 404.\ngot: %v",
						spelling, want, err)
				}
			}
			// The advice must report THIS row's fields, not a default — the
			// positive control that the matched row is what was rendered.
			assertOwnedStatePair(t, msg, "pending", "building")
		})
	}
}

// TestAppViewOwnedAdviceStillRejectsOtherSlugs is site 2's negative control. A
// false positive here is the expensive direction: it claims a slug the caller
// does NOT own is "one of your own apps", and prints another author's submission
// status and deploy state as fact.
func TestAppViewOwnedAdviceStillRejectsOtherSlugs(t *testing.T) {
	for _, r := range slugRejectsFor("buzz-generator") {
		t.Run(r.name, func(t *testing.T) {
			probes := appViewServer(t, func(w http.ResponseWriter, r2 *http.Request) {
				_, _ = w.Write([]byte(respelledOwnedBody(r.spelling)))
			})

			_, _, err := run(t, "app", "view", "buzz-generator")
			if err == nil {
				t.Fatal("expected a not-found error")
			}
			if n := atomic.LoadInt32(probes); n != 1 {
				t.Fatalf("the probe must have RUN (positive control), ran %d times", n)
			}
			if strings.Contains(err.Error(), "one of your own apps") {
				t.Errorf("a row for blockId %q was read as ownership of buzz-generator (%s) — "+
					"that prints another author's status and deploy state as fact about this slug",
					r.spelling, r.breaks)
			}
			assertNoOwnedAdvice(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// Site 4: internal/cmd/app_status.go — warnLocalVersionDrift
// ---------------------------------------------------------------------------

// TestAppStatusDriftMatchesARespelledManifestBlockID is site 4's red-at-base
// case, and the one site where the unnormalised value does NOT have to come from
// the server: manifest.Load is a bare json.Unmarshal with no schema validation,
// so a hand-edited `"blockId": " Custom-Generators "` reaches the compare today.
//
// The failure it removes is silent — the drift warning simply does not print,
// which is indistinguishable from "nothing is approved yet".
func TestAppStatusDriftMatchesARespelledManifestBlockID(t *testing.T) {
	for _, spelling := range slugRespellings(driftSlug) {
		t.Run(spelling, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			writeDriftManifest(t, spelling, "0.4.0")
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			if n := atomic.LoadInt32(&calls); n != 1 {
				t.Fatalf("%d request(s), want 1 — the detail lookup's response IS the listing", n)
			}
			// The literal sentence, written by hand from the issue's numbers.
			if !strings.Contains(errOut, driftWarnLine) {
				t.Errorf("a manifest whose blockId is %q describes the SAME app as the row, so the drift "+
					"warning must print.\nwant line: %s\ngot stderr:\n%s", spelling, driftWarnLine, errOut)
			}
			// 🔴 THE WARNING NAMES THE SERVER'S SPELLING, NOT THE MANIFEST'S — a
			// normalising match must not become a route by which a hand-mangled
			// local value reaches the screen as if the server had said it.
			//
			// Asserted POSITIONALLY, on the two literal sentences that embed the
			// slug, rather than as `!Contains(errOut, spelling)`. That form is a
			// spelled guard and it fails here for a reason worth recording: a
			// space-padded spelling like " custom-generators" IS a substring of
			// "…version of custom-generators," so the check fired on correct
			// output. Pinning both slots catches every echo the check was aimed
			// at, and cannot fire on whitespace it did not cause.
			//
			// driftWarnLine (asserted above) is slot 1. This is slot 2:
			const remedy = "civitai app pull . --app " + driftSlug
			if !strings.Contains(errOut, remedy) {
				t.Errorf("the remedy must name the SERVER's slug: want %q.\nstderr:\n%s", remedy, errOut)
			}
			// And the case-differing spellings are observable directly, since no
			// correct rendering ever upper-cases the slug.
			if strings.Contains(errOut, strings.ToUpper(driftSlug[:1])+driftSlug[1:]) ||
				strings.Contains(errOut, strings.ToUpper(driftSlug)) {
				t.Errorf("the warning echoed a mis-cased slug; it must render s.BlockID.\nstderr:\n%s", errOut)
			}
			if strings.Contains(out, driftWarnCore) {
				t.Errorf("the warning belongs on stderr; stdout:\n%s", out)
			}
		})
	}
}

// TestAppStatusDriftStillSilentForOtherSlugs is site 4's negative control. A
// false positive prints "your repo is BEHIND" by comparing two DIFFERENT apps'
// versions — a fabricated regression that sends an author to re-pull released
// code for no reason.
//
// It extends TestAppStatusDriftSilentForADifferentApp (which covers
// `some-other-app` and a manifest with no blockId key at all) with the three
// shapes an over-wide predicate would fail: an extension, a prefix, and an
// explicitly EMPTY blockId — the last being the case the hand-written
// `m.BlockID == ""` clause used to carry and that SameSlug now owns.
func TestAppStatusDriftStillSilentForOtherSlugs(t *testing.T) {
	for _, r := range slugRejectsFor(driftSlug) {
		t.Run(r.name, func(t *testing.T) {
			var calls int32
			srv := driftServer(t, driftFixture(t), &calls, 0)
			writeDriftManifest(t, r.spelling, "0.4.0")
			setupDriftEnv(t, srv.URL)

			out, errOut, err := run(t, "app", "status", driftSlug)
			if err != nil {
				t.Fatalf("app status: %v", err)
			}
			if n := atomic.LoadInt32(&calls); n != 1 {
				t.Fatalf("%d request(s), want 1 — the detail lookup must have happened, or this "+
					"asserts silence about a command that never ran", n)
			}
			assertNoDriftWarning(t, "manifest blockId="+r.spelling+" ("+r.breaks+")", out, errOut)
		})
	}
}

// TestWarnLocalVersionDriftRejectsAnAllWhitespaceBlockID is the strictly-new
// behaviour, isolated. `m.BlockID == "" || m.BlockID != s.BlockID` DID reject an
// all-whitespace blockId, but only via the second clause — the explicit empty
// guard never saw it. SameSlug trims first, so the empty case now covers it, and
// that is asserted directly rather than left implied.
func TestWarnLocalVersionDriftRejectsAnAllWhitespaceBlockID(t *testing.T) {
	for _, spelling := range []string{"   ", "\t", " \n "} {
		t.Run(strings.TrimSpace("ws"+spelling), func(t *testing.T) {
			if appapi.SameSlug(spelling, driftSlug) {
				t.Errorf("SameSlug(%q, %q) = true — an all-whitespace blockId names no app", spelling, driftSlug)
			}
		})
	}
}
