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
// ErrPkgNotFound) lives in pins.go — production code the bump-pins command
// reuses so the guard and the bumper agree by construction.

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestScaffoldPinsSatisfyPublished(t *testing.T) {
	if os.Getenv("CIVITAI_CHECK_PUBLISHED_PINS") != "1" {
		t.Skip("network guard; set CIVITAI_CHECK_PUBLISHED_PINS=1 to run (CI does)")
	}

	pins := collectCivitaiPins(t)
	if len(pins) == 0 {
		t.Fatal("no @civitai/* pins found in any template package.json.tmpl — the extractor is broken")
	}

	// De-dupe the published lookups across templates.
	latest := map[string]string{}
	notFound := map[string]bool{} // packages npm returned 404/410 for
	for _, p := range pins {
		if _, ok := latest[p.pkg]; ok || notFound[p.pkg] {
			continue
		}
		v, err := FetchNpmLatest(p.pkg)
		switch {
		case errors.Is(err, ErrPkgNotFound):
			// The package genuinely does not exist on npm — a renamed/
			// unpublished/typo'd pin. This is a REAL drift, not "can't tell
			// right now", so fail loud (don't skip).
			notFound[p.pkg] = true
			t.Errorf(
				"NONEXISTENT PACKAGE — a template pins a package npm does not publish.\n"+
					"  template: %s\n"+
					"  package:  %s   (not found on npm — renamed, unpublished, or a typo in the pin?)\n"+
					"  fix:      correct the @civitai/* package name in the template's package.json.tmpl",
				p.file, p.pkg,
			)
		case err != nil:
			// Transient/connectivity failure (DNS/timeout/connection-refused,
			// or a 5xx / 429 rate-limit) — we can't tell right now, so skip
			// rather than false-fail.
			t.Skipf("npm unreachable for %s (%v) — skipping, not failing", p.pkg, err)
		default:
			latest[p.pkg] = v
		}
	}

	for _, p := range pins {
		if notFound[p.pkg] {
			continue // already reported as a nonexistent package
		}
		published := latest[p.pkg]
		ok, err := CaretAdmits(p.pin, published)
		if err != nil {
			t.Fatalf("%s: cannot evaluate pin %q against published %q: %v", p.file, p.pin, published, err)
		}
		if !ok {
			t.Errorf(
				"STALE SCAFFOLD PIN — every app created from this template is born stale.\n"+
					"  template: %s\n"+
					"  package:  %s\n"+
					"  pinned:   %s   (does NOT admit the published version — pre-1.0 caret locks the minor)\n"+
					"  published:%s   (npmjs.org/%s latest)\n"+
					"  fix:      run `go run ./internal/scaffold/cmd/bump-pins` (or set the pin to \"^%s\" and update the matching assertion in scaffold_test.go)",
				p.file, p.pkg, p.pin, published, p.pkg, published,
			)
		}
	}
}

type civitaiPin struct {
	file string // template-relative path of the package.json.tmpl
	pkg  string // e.g. @civitai/app-sdk
	pin  string // e.g. ^0.24.0
}

// collectCivitaiPins walks the embedded templates for every package.json.tmpl
// and extracts its @civitai/* pins.
func collectCivitaiPins(t *testing.T) []civitaiPin {
	t.Helper()
	var out []civitaiPin
	err := fs.WalkDir(templatesFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "package.json.tmpl" {
			return nil
		}
		raw, err := templatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range CivitaiPinRe.FindAllStringSubmatch(string(raw), -1) {
			out = append(out, civitaiPin{file: p, pkg: m[1], pin: m[2]})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking templates: %v", err)
	}
	return out
}
