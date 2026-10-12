package scaffold

// Pins-vs-published rot-guard.
//
// Every scaffolded App is born pinning the `@civitai/*` packages the templates
// declare. Because those pins use a caret (`^0.24.0`) and the SDK is pre-1.0,
// the caret LOCKS THE MINOR — `^0.18.0` will never install `0.24.0`. So a
// template whose pins fall behind npm silently ships apps that are born stale,
// peer-mismatched, and missing whatever the newer minors added. The
// `template-page-money` CI job (npm install → typecheck → build) does NOT catch
// this: `npm install` happily resolves the caret to the newest version it
// ADMITS, so a stale `^0.18.0` installs a green-but-old `0.18.x`.
//
// This guard closes that gap: it reads each template's `@civitai/*` pin and
// asserts the caret range still ADMITS the version npm currently publishes as
// `latest`. When it doesn't, it fails loud with the stale pin, the current
// published version, and the one-line fix.
//
// It is network-gated: it only runs when CIVITAI_CHECK_PUBLISHED_PINS=1 (a
// dedicated CI job sets it), so the default `go test ./...` stays offline. When
// enabled but npm is unreachable it SKIPS (never a false failure).
//
// The shared pin logic (CivitaiPinRe / FetchNpmLatest / CaretAdmits /
// ErrPkgNotFound, and TemplatePins / CheckPins — the walk and the verdict
// themselves) lives in pins.go — production code the bump-pins and pins-gate
// commands reuse so the guard, the bumper and the CI job agree by construction.
//
// 🔴 THIS TEST IS NO LONGER WHAT THE `pins-vs-published` CI JOB RUNS. That job
// runs `internal/scaffold/cmd/pins-gate`, which asks the same CheckPins question
// and then decides whether the answer should BLOCK the run it is in (a PR that
// did not touch a pin input is warned, not blocked). This test is still the
// bump bot's pre-PR check (`validate — pins now satisfy published`), and the
// two deliberately differ on ONE row: this test SKIPS when npm is unreachable,
// because a bot that withheld its PR over a blip would leave the pins stale;
// the gate reports unreachable as its own verdict instead of a skip.

import (
	"os"
	"testing"
)

func TestScaffoldPinsSatisfyPublished(t *testing.T) {
	if os.Getenv("CIVITAI_CHECK_PUBLISHED_PINS") != "1" {
		t.Skip("network guard; set CIVITAI_CHECK_PUBLISHED_PINS=1 to run (the bump-scaffold-pins workflow does)")
	}

	pins, err := TemplatePins()
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) == 0 {
		t.Fatal("no @civitai/* pins found in any template package.json.tmpl — the extractor is broken")
	}

	findings := CheckPins(pins, FetchNpmLatest)

	// NOT-FOUND FIRST, and it is a failure whatever else happened: the package
	// genuinely does not exist on npm — a renamed/unpublished/typo'd pin. That
	// is a REAL drift, not "can't tell right now", so fail loud (don't skip).
	// Once per package.
	reportedMissing := map[string]bool{}
	for _, f := range findings {
		if f.Status != PinNotFound || reportedMissing[f.Pkg] {
			continue
		}
		reportedMissing[f.Pkg] = true
		t.Errorf(
			"NONEXISTENT PACKAGE — a template pins a package npm does not publish.\n"+
				"  template: %s\n"+
				"  package:  %s   (not found on npm — renamed, unpublished, or a typo in the pin?)\n"+
				"  fix:      correct the @civitai/* package name in the template's package.json.tmpl",
			f.File, f.Pkg,
		)
	}

	// Unreachable NEXT, and it skips the rest: a transient/connectivity failure
	// (DNS/timeout/connection-refused, or a 5xx / 429 rate-limit) means we can't
	// tell right now, so skip rather than false-fail. It has to come before the
	// staleness verdicts — "npm blipped on one package" must not be reported as
	// a half-evaluated set of pins.
	for _, f := range findings {
		if f.Status == PinUnreachable {
			t.Skipf("npm unreachable for %s (%v) — skipping, not failing", f.Pkg, f.Err)
		}
	}

	for _, f := range findings {
		switch f.Status {
		case PinInvalid:
			t.Fatalf("%s: cannot evaluate pin %q against published %q: %v", f.File, f.Pin, f.Published, f.Err)
		case PinStale:
			t.Errorf(
				"STALE SCAFFOLD PIN — every app created from this template is born stale.\n"+
					"  template: %s\n"+
					"  package:  %s\n"+
					"  pinned:   %s   (does NOT admit the published version — pre-1.0 caret locks the minor)\n"+
					"  published:%s   (npmjs.org/%s latest)\n"+
					"  fix:      run `go run ./internal/scaffold/cmd/bump-pins` (or set the pin to \"^%s\" and update the matching assertion in scaffold_test.go)",
				f.File, f.Pkg, f.Pin, f.Published, f.Pkg, f.Published,
			)
		}
	}
}
