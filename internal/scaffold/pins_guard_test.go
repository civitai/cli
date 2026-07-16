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

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// civitaiPinRe extracts `"@civitai/<pkg>": "<pin>"` pairs from a rendered or raw
// package.json(.tmpl). The pin lines carry no template placeholders, so a raw
// scan of the .tmpl is exact.
var civitaiPinRe = regexp.MustCompile(`"(@civitai/[a-z0-9-]+)"\s*:\s*"([^"]+)"`)

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
	for _, p := range pins {
		if _, ok := latest[p.pkg]; ok {
			continue
		}
		v, err := fetchNpmLatest(p.pkg)
		if err != nil {
			t.Skipf("npm unreachable for %s (%v) — skipping, not failing", p.pkg, err)
		}
		latest[p.pkg] = v
	}

	for _, p := range pins {
		published := latest[p.pkg]
		ok, err := caretAdmits(p.pin, published)
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
					"  fix:      set the pin to \"^%s\" and update the matching assertion in scaffold_test.go",
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
		for _, m := range civitaiPinRe.FindAllStringSubmatch(string(raw), -1) {
			out = append(out, civitaiPin{file: p, pkg: m[1], pin: m[2]})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking templates: %v", err)
	}
	return out
}

// fetchNpmLatest returns the `version` npm publishes as `latest` for pkg.
func fetchNpmLatest(pkg string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := "https://registry.npmjs.org/" + pkg + "/latest"
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Version == "" {
		return "", fmt.Errorf("GET %s: empty version", url)
	}
	return body.Version, nil
}

// caretAdmits reports whether a caret range `^X.Y.Z` admits the concrete
// version `published` (stable X.Y.Z), following npm's pre-1.0 semantics:
//   - ^X.Y.Z, X>0    → [X.Y.Z, (X+1).0.0)
//   - ^0.Y.Z, Y>0    → [0.Y.Z, 0.(Y+1).0)
//   - ^0.0.Z         → [0.0.Z, 0.0.(Z+1))
//
// A bare (caret-less) pin must equal the published version.
func caretAdmits(pin, published string) (bool, error) {
	pubMaj, pubMin, pubPat, err := parseSemver(published)
	if err != nil {
		return false, fmt.Errorf("published %q: %w", published, err)
	}

	caret := strings.HasPrefix(pin, "^")
	base := strings.TrimPrefix(pin, "^")
	lo0, lo1, lo2, err := parseSemver(base)
	if err != nil {
		return false, fmt.Errorf("pin %q: %w", pin, err)
	}

	if !caret {
		return lo0 == pubMaj && lo1 == pubMin && lo2 == pubPat, nil
	}

	// published must be >= lower bound.
	if cmpSemver(pubMaj, pubMin, pubPat, lo0, lo1, lo2) < 0 {
		return false, nil
	}
	// and < the caret's upper bound.
	var hi0, hi1, hi2 int
	switch {
	case lo0 > 0:
		hi0, hi1, hi2 = lo0+1, 0, 0
	case lo1 > 0:
		hi0, hi1, hi2 = 0, lo1+1, 0
	default:
		hi0, hi1, hi2 = 0, 0, lo2+1
	}
	return cmpSemver(pubMaj, pubMin, pubPat, hi0, hi1, hi2) < 0, nil
}

func parseSemver(v string) (int, int, int, error) {
	// Drop any prerelease/build metadata; `latest` is stable but be defensive.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("not X.Y.Z: %q", v)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("non-numeric component in %q: %w", v, err)
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], nil
}

func cmpSemver(a0, a1, a2, b0, b1, b2 int) int {
	if a0 != b0 {
		return a0 - b0
	}
	if a1 != b1 {
		return a1 - b1
	}
	return a2 - b2
}
