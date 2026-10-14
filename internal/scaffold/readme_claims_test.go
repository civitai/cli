package scaffold

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Guards for the claims the page-money README makes to the author who reads it:
// that it sends them to the hosted hook reference EARLY, and — network-gated —
// that its `dev:harness` capability list is true of the published mock host.
//
// 🔴 THIS FILE REPLACED hooks_test.go, AND THE DIFFERENCE IS THE POINT. Until
// #685's index was retired, this repo shipped its own 36-row table of every
// `@civitai/blocks-react` hook: a Go table (`internal/scaffold/hooks.go`)
// rendered into the scaffolded README as `{{ .HookIndex }}`, with a
// bidirectional guard tying it to the published `.d.ts`. The table was correct
// and nobody read it. Measured across both graded deepseek dogfood trials: the
// scaffolded README was read ZERO times, while the hosted reference
// <https://developer.civitai.com/apps/reference/hooks.md> was fetched FOUR times
// in the post-release run. Two copies of one list is a drift surface; the hosted
// one is generated from the package itself and is therefore the copy that cannot
// go stale. So the index, its generator, and the four guards that pinned it are
// gone, and what survives here is the claim that still matters — that the reader
// is pointed at the canonical list before they start guessing.

// ---------------------------------------------------------------------------
// The pointer to the hosted reference
// ---------------------------------------------------------------------------

// hostedHookReference is the canonical, generated hook list. It is the ONE thing
// the scaffolded README has to say about hooks.
//
// 🔴 THE `.md` SPELLING IS LOAD-BEARING, not a stylistic choice: it is the
// plain-text rendering an agent can fetch and read, and the `.html` page is the
// one that cost a measured run its budget. `internal/cmd/templates/agents-app.md`
// spells the same URL for the same reason, and both spellings are reconciled by
// TestEveryDocsURLSpellingIsLedgered in internal/cmd.
const hostedHookReference = "https://developer.civitai.com/apps/reference/hooks.md"

// TestTheScaffoldedReadmeSendsTheReaderToTheHostedHookReference is the guard
// that replaces TestTheScaffoldedReadmeCarriesTheHookIndexWhereItIsRead.
//
// 🔴 THE MEASURED DEFECT IT INHERITS. A blind agent asked to "generate an image
// and then post it to Civitai" needed `useCreatePostFromApp`, which appeared in
// NO scaffolded file. It reverse-engineered the hook from the package's `.d.ts`
// at a cost of 706,371 carried tokens — 28.2% of the run — and wrote no code in
// 66 steps. The fix for that was never "ship a table"; it was "tell the reader
// where the list is". The table was one way to do that and it is not the way the
// measurements say agents take.
//
// 🔴 THE POSITION BOUND IS A *START* BOUND and it is the same 3 KB the retired
// index was held to, so the reader still meets the pointer before they have read
// enough prose to start guessing. There is no end-line bound any more: a link is
// two lines, not 36 rows, so the constraint the old guard had to trade against
// does not exist.
func TestTheScaffoldedReadmeSendsTheReaderToTheHostedHookReference(t *testing.T) {
	const maxStartByte = 3072
	readme := scaffoldedReadme(t)

	start := strings.Index(readme, hostedHookReference)
	if start < 0 {
		t.Fatalf("the scaffolded README does not name the hosted hook reference (%s) at all.\n"+
			"  The local 36-hook index was retired in favour of that URL, so this link is now the ONLY "+
			"thing in a scaffolded project that tells an author where the capability list lives — without "+
			"it the project ships with no route to it and no error.\n  README starts:\n%s",
			hostedHookReference, readme[:min(len(readme), 400)])
	}
	if start > maxStartByte {
		t.Errorf("the scaffolded README first names the hosted hook reference at byte %d, past the "+
			"%d-byte bound. The whole point is that a reader meets the capability list BEFORE it has read "+
			"enough prose to start guessing.", start, maxStartByte)
	}

	// The pointer is useless if the reader does not know a declared scope is not
	// a granted scope — the single most expensive surprise on this platform, and
	// the one thing the retired index's footnote carried that the hosted page
	// does not lead with. It survived the removal deliberately.
	for _, needle := range []string{"consent-gated", "useRequestConsent()"} {
		if !strings.Contains(readme, needle) {
			t.Errorf("the scaffolded README no longer mentions %q. `ai:write:budgeted` and "+
				"`posts:write:self` are dropped from the token until the viewer consents, so an author "+
				"gets a 403 while the manifest and the runtime both look correct.", needle)
		}
	}

	// 🔴 NEGATIVE CONTROL FOR THE REMOVAL ITSELF: no rendered README may carry a
	// hook TABLE any more. A table that came back would satisfy every assertion
	// above while re-creating the drift surface the removal exists to close.
	if m := hookTableRowRe.FindAllString(readme, -1); len(m) > 0 {
		t.Errorf("the scaffolded README carries %d hook-table row(s) (%q …). The local index was retired; "+
			"the hosted reference is canonical. A second copy in this repo goes stale against the package "+
			"silently.", len(m), m[0])
	}
}

// hookTableRowRe matches a markdown table row whose first cell is a `useX()`
// call — i.e. a row of the retired index, in whatever spelling.
var hookTableRowRe = regexp.MustCompile("(?m)^\\| `(use[A-Za-z0-9]+)\\(\\)` \\|")

// TestHookTableRowDetectorCanFail is the negative control for the detector
// above: fed a table row it must match, fed prose naming a hook it must not.
// Without it, "no table rows found" is indistinguishable from a dead regex.
func TestHookTableRowDetectorCanFail(t *testing.T) {
	if got := hookTableRowRe.FindAllString("A paragraph naming `useCreatePostFromApp()` inline.\n", -1); got != nil {
		t.Errorf("the detector matched prose, so it is not reading TABLE ROWS: %v", got)
	}
	if got := hookTableRowRe.FindAllString("| `useThing()` | a thing | — |\n", -1); len(got) != 1 {
		t.Errorf("the detector did not read a real table row: %v", got)
	}
}

// ---------------------------------------------------------------------------
// The dev:harness capability claim (network-gated)
// ---------------------------------------------------------------------------

// publishedHooksEnv gates the one network-touching guard here, following the
// existing `CIVITAI_CHECK_PUBLISHED_PINS` pattern: `make ci` must stay green
// offline, so this is a manual / scheduled check.
const publishedHooksEnv = "CIVITAI_CHECK_PUBLISHED_HOOKS"

// mockedMessageClaimRe reads the message types the README claims `dev:harness`
// mocks, out of the paragraph that makes the claim. It is anchored on the
// sentence so an unrelated `SHOUTY_TOKEN` elsewhere in the README cannot join
// the set being verified.
var (
	mockedClaimAnchor  = "The mock host answers the whole block→host protocol"
	mockedMessageReExp = regexp.MustCompile("`([A-Z][A-Z0-9_]{3,})`")
)

// TestTheMockHostClaimsAreTrueOfThePublishedMockHost checks the bonus claim.
//
// 🔴 WHY THE CLAIM EXISTS AT ALL. The README told an author what `dev:live` does
// NOT support and then stopped, so the measured dogfood run had no way to learn
// whether its ONE required capability — posting — could be exercised for free.
// It read `mockHost.js` and `mockHost.d.ts` to find out, at ~380,000 carried
// tokens. The answer is yes.
//
// 🔴 AND WHY IT IS CHECKED RATHER THAN ASSERTED. A list of capabilities in prose
// is a claim about a package that ships separately and changes without this
// repo: a mock host that drops a handler turns this paragraph into an
// instruction to try something that hangs. The published `createMockHost` is the
// authority, so the test reads it.
func TestTheMockHostClaimsAreTrueOfThePublishedMockHost(t *testing.T) {
	readme := scaffoldedReadme(t)
	i := strings.Index(readme, mockedClaimAnchor)
	if i < 0 {
		t.Fatalf("the scaffolded README no longer carries the `dev:harness` capability claim "+
			"(anchor %q). Either it was removed — drop this guard with it — or it was reworded, in which "+
			"case nothing has been verifying it since.", mockedClaimAnchor)
	}
	section := readme[i:]
	if j := strings.Index(section, "\n### "); j >= 0 {
		section = section[:j]
	}
	var claimed []string
	for _, m := range mockedMessageReExp.FindAllStringSubmatch(section, -1) {
		claimed = append(claimed, m[1])
	}
	if len(claimed) < 4 {
		t.Fatalf("CONTROL failure, not a finding: extracted %d message type(s) from the claim paragraph "+
			"(%v), want >= 4 — the extractor is reading the wrong text", len(claimed), claimed)
	}

	if os.Getenv(publishedHooksEnv) != "1" {
		t.Skipf("the README claims %d mocked message type(s) %v; set %s=1 to verify them against the "+
			"published createMockHost (network)", len(claimed), claimed, publishedHooksEnv)
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
	// Positive control: the mock host must mention a type we know it handles, or
	// every "not found" below is a fact about the fetch.
	if !strings.Contains(mock, "SUBMIT_WORKFLOW") {
		t.Fatalf("CONTROL failure, not a finding: %s@%s's mock host does not mention SUBMIT_WORKFLOW, "+
			"which is the money path it exists for — the fetched file is not the mock host", pkg, version)
	}
	for _, msg := range claimed {
		if !strings.Contains(mock, "'"+msg+"'") && !strings.Contains(mock, `"`+msg+`"`) {
			t.Errorf("the scaffolded README tells an author that `%s` works in `dev:harness`, and "+
				"%s@%s's createMockHost does not handle it. An unhandled REQUEST-style message does not "+
				"error — it HANGS to the SDK timeout, which reads as the author's own bug.",
				msg, pkg, version)
		}
	}
	t.Logf("verified %d claimed message type(s) against %s@%s", len(claimed), pkg, version)
}

// fetchPackageFile pulls one file out of a published npm tarball.
func fetchPackageFile(pkg, version, name string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	meta := fmt.Sprintf("%s/%s/%s", npmRegistryBase, pkg, version)
	resp, err := client.Get(meta)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", meta, resp.Status)
	}
	var doc struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	if doc.Dist.Tarball == "" {
		return "", fmt.Errorf("GET %s: no tarball url", meta)
	}
	tgz, err := client.Get(doc.Dist.Tarball)
	if err != nil {
		return "", err
	}
	defer func() { _ = tgz.Body.Close() }()
	if tgz.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", doc.Dist.Tarball, tgz.Status)
	}
	gz, err := gzip.NewReader(tgz.Body)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%s not found in %s", name, doc.Dist.Tarball)
		}
		if err != nil {
			return "", err
		}
		if h.Name != name {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 1<<20))
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
}

// ---------------------------------------------------------------------------
// The README does not restate the moving pins
// ---------------------------------------------------------------------------

// readmePinSpellingRe matches a version-pinned package spelling in prose —
// `@civitai/<pkg>@^X.Y`, `@civitai/<pkg>@X.Y` — the shape the README used to
// restate the template's pins in.
var readmePinSpellingRe = regexp.MustCompile(`@civitai/[a-z0-9-]+@[~^]?\d+\.\d+`)

// TestPageMoneyREADMESpellsNoCaretPin pins the RELATIONSHIP between the README
// and package.json.tmpl: the README must not repeat a version the bump bot moves.
//
// 🔴 THE DEFECT. The README said its per-account Buzz features "require
// `@civitai/app-sdk@^0.70.0` + `@civitai/blocks-react@^0.73.0`". Those were the
// template's CURRENT pins, not a requirement: `spentAccountType` first appears
// in app-sdk 0.14.0's typings (absent in 0.13.0) and `useBuzzBalance` in
// blocks-react 0.16.0's (absent in 0.15.0). The sentence was false the moment it
// was written and was rewritten by `bump-pins` on every bump to stay false in a
// new way. The README now names those fixed minimums, and `bump-pins` no longer
// treats it as a pin site.
//
// Two checks, because either alone is walkable: the pinned-package SPELLING
// (`@pkg@^X.Y`), and the current pin's MAJOR.MINOR of every package page-money
// pins (the exact X.Y.Z once a package is >= 1.0), matched on number
// boundaries (pinMentionRe), so "`@civitai/app-sdk` 0.70.0", `^0.70`, `0.70.x`
// and `v0.70` are all caught.
func TestPageMoneyREADMESpellsNoCaretPin(t *testing.T) {
	// Positive control: the regex matches the spelling that shipped.
	if !readmePinSpellingRe.MatchString("require `@civitai/app-sdk@^0.70.0` + `@civitai/blocks-react@^0.73.0`") {
		t.Fatalf("CONTROL failure, not a finding: readmePinSpellingRe (%s) does not match the spelling it exists to catch",
			readmePinSpellingRe)
	}

	readme := scaffoldedReadme(t)
	if m := readmePinSpellingRe.FindAllString(readme, -1); len(m) > 0 {
		t.Errorf("the scaffolded page-money README spells %d version-pinned package(s): %s.\n"+
			"A version here restates package.json's pin, which bump-pins moves and this README does not. "+
			"Name the MINIMUM version a feature needs (a fixed fact), or say \"the versions package.json pins\".",
			len(m), strings.Join(m, ", "))
	}

	pins, err := TemplatePins()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, p := range pins {
		if p.File != "templates/page-money/package.json.tmpl" {
			continue
		}
		checked++
		re, err := pinMentionRe(p.Pin)
		if err != nil {
			t.Fatalf("%s pin %q: %v", p.Pkg, p.Pin, err)
		}
		if m := re.FindString(readme); m != "" {
			t.Errorf("the scaffolded page-money README contains %q, which names the moving part of the version package.json.tmpl "+
				"currently pins %s to (%s). That number moves on every bump and the README does not; name the "+
				"feature's minimum instead.", strings.TrimSpace(m), p.Pkg, p.Pin)
		}
	}
	if checked < 2 {
		t.Fatalf("CONTROL failure, not a finding: %d page-money @civitai/* pin(s) found, want >= 2 "+
			"(app-sdk + blocks-react) — the version check above compared against nothing", checked)
	}
}

// pinMentionRe matches a mention of the part of a caret pin that MOVES on a
// bump, as a whole number (not preceded by a digit or `.`, not followed by a
// digit):
//
//   - major 0 (`^0.70.0`): the caret locks the MINOR, so every bump moves
//     major.minor. It matches `0.70`, `0.70.0`, `0.70.x`, `^0.70`, `v0.70` —
//     but not `10.70`, `1.0.70` or `0.701`. Matching only the full `X.Y.Z` is
//     walkable by any spelling that drops the patch.
//   - major >= 1 (`^1.0.0`): the caret locks only the major, and major.minor is
//     too common to own — the README says `SDXL 1.0`, which a `1.0` match
//     flags as soon as a package reaches 1.0.0, reddening the bump bot on
//     correct text. So it matches the exact `X.Y.Z` the pin names, still
//     number-bounded.
func pinMentionRe(pin string) (*regexp.Regexp, error) {
	parts := strings.Split(strings.TrimLeft(pin, "^~"), ".")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("not a X.Y[.Z] pin")
	}
	moving := parts[0] + "." + parts[1]
	if parts[0] != "0" {
		if len(parts) < 3 || parts[2] == "" {
			return nil, fmt.Errorf("a >= 1.0 pin needs X.Y.Z")
		}
		moving += "." + parts[2]
	}
	return regexp.Compile(`(?:^|[^0-9.])` + regexp.QuoteMeta(moving) + `(?:[^0-9]|$)`)
}

// TestPinMentionReCatchesEverySpellingOfTheMovingVersion is the matcher's own
// control: the spellings that walked the earlier exact-`X.Y.Z` check, and the
// README's legitimate fixed minimums, which must NOT collide.
func TestPinMentionReCatchesEverySpellingOfTheMovingVersion(t *testing.T) {
	re, err := pinMentionRe("^0.70.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"0.70.0", "^0.70", "0.70.x", "v0.70", "@civitai/app-sdk@^0.70.0", "needs 0.70 or later", "(0.70)"} {
		if !re.MatchString(s) {
			t.Errorf("pinMentionRe(^0.70.0) does NOT match %q — a README spelling the moving pin that way passes the guard", s)
		}
	}
	for _, s := range []string{"0.14.0", "0.16.0", "10.70", "1.0.70", "0.701", "0.7", "0.71.0"} {
		if re.MatchString(s) {
			t.Errorf("pinMentionRe(^0.70.0) matches %q, which is not the pinned version", s)
		}
	}
	if _, err := pinMentionRe("^0"); err == nil {
		t.Error("pinMentionRe accepted a pin with no minor")
	}
}

// TestPinMentionReIsQuietOnTheREADMEWhenAPackageReaches1x is the 1.0 boundary.
// `DesiredPin` turns a first 1.0.0 publish into `^1.0.0`, and the page-money
// README legitimately says `SDXL 1.0` / `1.0`. A major.minor match there fires on
// correct text, which reddens the bump bot's offline suite and leaves
// `pins-vs-published` red until someone edits this guard.
func TestPinMentionReIsQuietOnTheREADMEWhenAPackageReaches1x(t *testing.T) {
	readme := scaffoldedReadme(t)
	// Positive control: the README really does carry the colliding `1.0` text,
	// so a zero below is the matcher's answer, not an absent input.
	if !strings.Contains(readme, "1.0") {
		t.Fatal("CONTROL failure, not a finding: the rendered README no longer contains `1.0`; this case tests nothing")
	}
	re, err := pinMentionRe("^1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if m := re.FindAllString(readme, -1); len(m) != 0 {
		t.Errorf("a `^1.0.0` pin matches %d place(s) in the real README, all of which are correct text: %q", len(m), m)
	}
	for _, s := range []string{"1.0.0", "^1.0.0", "@civitai/app-sdk@^1.0.0"} {
		if !re.MatchString(s) {
			t.Errorf("pinMentionRe(^1.0.0) does NOT match %q — the exact pinned version must still be caught", s)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func renderPageMoney(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	if _, err := Render(PageMoney, dir, Data{Slug: "readme-claims-probe", Name: "Readme Claims Probe"}); err != nil {
		t.Fatalf("rendering page-money: %v", err)
	}
	return dir
}

func scaffoldedReadme(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(renderPageMoney(t), "README.md"))
	if err != nil {
		t.Fatalf("read the scaffolded README: %v", err)
	}
	return string(raw)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
