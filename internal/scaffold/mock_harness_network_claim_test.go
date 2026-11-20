package scaffold

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 🔴 THE MOCK HARNESS IS NOT NETWORK-FREE, SO NOTHING MAY SAY IT IS (issue #934).
//
// page-money's `npm run dev:harness` mounts `<SdkHarness>` → the published
// `createMockHost` from `@civitai/blocks-react/testing`. Its default synthetic
// result is `https://placehold.co/512x512/1971c2/ffffff/png?text=MOCK%0A<id>`
// (`imagesFor` in blocks-react's internal/mockHost.ts), which App.tsx's
// `firstImageUrl` stores and renders as `<img alt="Generated result">`. So every
// successful mock generation makes one third-party request, and with that host
// blocked the run reads **Done** over a broken image. The template said
// "no real Buzz, no compute, no network" in three places. The URL lives in the
// SDK, not the template, so the fix is the description: synthetic, no real
// Buzz / compute, AND the result image is fetched from placehold.co.
//
// The README's mock row is pinned WHOLE (normalised), because a guard on one
// phrase is walked by the next paraphrase; the two code comments, which carry
// the same claim in shorter form, are held to "names placehold.co and does not
// promise no network".

// mockRowWant is the README's `dev:harness` row, whitespace-normalised.
const mockRowWant = "| `npm run dev:harness` | **mock** (default) | The SDK mock host. Synthetic " +
	"generation — **no real Buzz, no compute, no Civitai backend.** Not offline, though: each " +
	"result image is a placeholder fetched from the third-party host `placehold.co`, so if that " +
	"request is blocked the run still shows Done with a broken image. Safe to spam. |"

var noNetworkRe = regexp.MustCompile(`(?i)no\s+network`)

func normaliseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestPageMoneyMockHarnessDocsDiscloseThePlaceholderFetch(t *testing.T) {
	dir := renderPageMoney(t)
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read rendered %s: %v", rel, err)
		}
		return string(b)
	}

	readme := read("README.md")
	var row string
	for _, l := range strings.Split(readme, "\n") {
		if strings.HasPrefix(l, "| `npm run dev:harness` |") {
			if row != "" {
				t.Fatalf("CONTROL failure: more than one `dev:harness` table row in the README")
			}
			row = l
		}
	}
	if row == "" {
		t.Fatalf("CONTROL failure: the README has no `npm run dev:harness` table row — the Mock vs live " +
			"table moved, and nothing is checking what it tells the author")
	}
	if got := normaliseSpace(row); got != mockRowWant {
		t.Errorf("README Mock vs live row for dev:harness:\n got: %s\nwant: %s", got, mockRowWant)
	}

	// The shorter restatements of the same claim, located by the sentence that
	// introduces the mock mode in each file.
	for _, c := range []struct{ rel, anchor string }{
		{".env.example", "mock  -> the SDK MOCK host"},
		{"src/main.tsx", "VITE_HARNESS_MODE=mock"},
	} {
		body := read(c.rel)
		i := strings.Index(body, c.anchor)
		if i < 0 {
			t.Fatalf("CONTROL failure: %s no longer carries %q — the mock-mode description moved", c.rel, c.anchor)
		}
		// The description runs up to the next mode (`live`).
		desc := body[i:]
		if j := strings.Index(desc, "live"); j >= 0 {
			desc = desc[:j]
		}
		if noNetworkRe.MatchString(desc) {
			t.Errorf("%s still tells the author the mock harness makes no network requests: %q", c.rel, normaliseSpace(desc))
		}
		if !strings.Contains(desc, "placehold.co") {
			t.Errorf("%s's mock-mode description does not name the placehold.co result-image fetch: %q",
				c.rel, normaliseSpace(desc))
		}
	}

	// Negative control for the regexp: it must catch the shipped wording.
	if !noNetworkRe.MatchString("synthetic, no real Buzz, no compute, no\n  network.") {
		t.Fatal("CONTROL failure: noNetworkRe does not match a line-wrapped `no network`")
	}
}

// TestThePlaceholderFetchClaimIsTrueOfThePublishedMockHost keeps the disclosure
// above honest in the OTHER direction: if a future blocks-react stops fetching
// from placehold.co (for example, by switching to a `data:` URI), the docs now
// over-warn and should be revised. Network-gated like its neighbour
// TestTheMockHostClaimsAreTrueOfThePublishedMockHost, because `make ci` must
// stay green offline.
func TestThePlaceholderFetchClaimIsTrueOfThePublishedMockHost(t *testing.T) {
	if os.Getenv(publishedHooksEnv) != "1" {
		t.Skipf("set %s=1 to check that the published createMockHost still fetches placehold.co (network)",
			publishedHooksEnv)
	}
	const pkg = "@civitai/blocks-react"
	version, err := FetchNpmLatest(pkg)
	if err != nil {
		t.Skipf("could not resolve %s on npm (%v) — nothing was verified", pkg, err)
	}
	mock, err := fetchPackageFile(pkg, version, "package/dist/internal/mockHost.js")
	if err != nil {
		t.Skipf("could not read %s@%s's mock host (%v) — nothing was verified", pkg, version, err)
	}
	if !strings.Contains(mock, "SUBMIT_WORKFLOW") {
		t.Fatalf("CONTROL failure: %s@%s's mockHost.js does not mention SUBMIT_WORKFLOW — the fetched "+
			"file is not the mock host", pkg, version)
	}
	if !strings.Contains(mock, "https://placehold.co/") {
		t.Errorf("%s@%s's createMockHost no longer builds its result image from placehold.co. The "+
			"page-money docs (README Mock vs live, .env.example, src/main.tsx) tell authors it does — "+
			"revise them to match.", pkg, version)
	}
}
