// Shared @civitai/* scaffold-pin logic.
//
// The templates scaffolded by `civitai app create` pin the `@civitai/*` packages
// with a pre-1.0 caret (`^0.25.0`). Because the SDK is pre-1.0 the caret LOCKS
// THE MINOR (`^0.25.0` never installs `0.26.0`), so a pin that falls behind npm
// silently ships apps that are born stale. Two consumers keep those pins honest
// and MUST agree by construction, so the logic lives here (production code), not
// in a `_test.go`:
//
//   - the pins-vs-published rot-guard (pins_guard_test.go, network-gated by
//     CIVITAI_CHECK_PUBLISHED_PINS=1) — reads each template's literal pin and
//     asserts the caret still ADMITS npm's published `latest`.
//   - the bump-pins command (internal/scaffold/cmd/bump-pins) — rewrites the
//     literal pins to the desired value when they fall behind, so no human ever
//     hand-bumps them again.
//
// 🔴 The pins are kept as LITERAL `^X.Y.Z` strings in the raw `.tmpl` bytes — do
// NOT template them (e.g. `{{ .Pins.SDK }}`): the guard reads the literal from
// the RAW template via CivitaiPinRe, so templating the value would blind it. The
// automation rewrites the literals in place instead.
package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrPkgNotFound is returned by FetchNpmLatest when npm answers 404/410 — the
// package genuinely does not exist (vs a transient/connectivity failure). The
// guard hard-fails on this (a real drift/typo); the bumper skips it.
var ErrPkgNotFound = errors.New("package not found on npm")

// CivitaiPinRe extracts `"@civitai/<pkg>": "<pin>"` pairs from a rendered or raw
// package.json(.tmpl). The pin lines carry no template placeholders, so a raw
// scan of the .tmpl is exact.
var CivitaiPinRe = regexp.MustCompile(`"(@civitai/[a-z0-9-]+)"\s*:\s*"([^"]+)"`)

// npmRegistryBase is the registry root FetchNpmLatest queries. It is a package
// var (not a const) purely so tests can point it at an httptest server; nothing
// in production ever reassigns it.
var npmRegistryBase = "https://registry.npmjs.org"

// FetchNpmLatest returns the `version` npm publishes as `latest` for pkg.
//
// 404/410 = the package genuinely doesn't exist → a real drift/typo, returned as
// ErrPkgNotFound. Any other non-200 (5xx, 429 rate-limit, DNS/timeout) is a
// "can't tell right now" transient the caller skips on.
func FetchNpmLatest(pkg string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := npmRegistryBase + "/" + pkg + "/latest"
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return "", fmt.Errorf("GET %s: %s: %w", url, resp.Status, ErrPkgNotFound)
	}
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

// DesiredPin is the caret pin a template SHOULD carry for a published version.
// It pins the leftmost non-zero component so a scaffolded app tracks the current
// line while still admitting `published`, matching npm's pre-1.0 caret semantics
// (and CaretAdmits / the guard). For X>0 or Y>0 that's `^<major>.<minor>.0`
// (pins the minor); for a `0.0.Z` package the caret locks the PATCH, so it must
// be `^0.0.<patch>` — `^0.0.0` would admit ONLY 0.0.0, i.e. a pin the guard then
// rejects for any 0.0.Z>0. e.g. 0.25.3 → "^0.25.0", 1.4.0 → "^1.4.0", 0.0.7 → "^0.0.7".
func DesiredPin(published string) (string, error) {
	maj, min, patch, err := parseSemver(published)
	if err != nil {
		return "", fmt.Errorf("published %q: %w", published, err)
	}
	if maj == 0 && min == 0 {
		return fmt.Sprintf("^0.0.%d", patch), nil
	}
	return fmt.Sprintf("^%d.%d.0", maj, min), nil
}

// CaretAdmits reports whether a caret range `^X.Y.Z` admits the concrete version
// `published` (stable X.Y.Z), following npm's pre-1.0 semantics:
//   - ^X.Y.Z, X>0    → [X.Y.Z, (X+1).0.0)
//   - ^0.Y.Z, Y>0    → [0.Y.Z, 0.(Y+1).0)
//   - ^0.0.Z         → [0.0.Z, 0.0.(Z+1))
//
// A bare (caret-less) pin must equal the published version.
func CaretAdmits(pin, published string) (bool, error) {
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

// parseSemver parses a concrete X.Y.Z. Only caret (`^X.Y.Z`) and exact pins are
// supported (all the templates use); a `~`/`>=`/`x` operator will fail loud
// here, which is the right behavior — a red CI prompting a guard update.
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
