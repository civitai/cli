package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// These three guards tie README claims about INSTALLING and CONFIGURING the CLI
// to the artifacts that decide them: `.goreleaser.yaml` (what the release
// actually publishes), `internal/cmd/upgrade.go` (what the self-updater asks
// for) and `strconv.ParseBool` (what the CIVITAI_* colour variables accept).
//
// All three defects they pin shipped for months in the reassuring direction —
// the README promised a platform or a value that does not work, and nothing was
// red. A drift guard is the only thing that notices, because none of these
// claims can be falsified by running the CLI on the machine CI runs on.

// readGoreleaserConfig returns the parsed `.goreleaser.yaml` as a top-level key
// map plus the raw bytes. It Fatals on a read or parse failure rather than
// returning an empty map: an empty map would make every "stanza absent" branch
// below fire for the wrong reason.
func readGoreleaserConfig(t *testing.T) (map[string]yaml.Node, []byte) {
	t.Helper()
	path := filepath.Join(repoRootDir(t), ".goreleaser.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(raw, &top); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	// POSITIVE CONTROL ON THE INSTRUMENT. A config that parsed to two keys would
	// answer "no `brews:` stanza" just as confidently as the real file does, and
	// the Homebrew guard below would then pass while reading nothing.
	if len(top) < 6 {
		t.Fatalf("CONTROL failure: .goreleaser.yaml parsed to only %d top-level key(s) (%v) — "+
			"this test is reading the wrong file or the wrong format, so every conclusion "+
			"about which stanzas it carries is unfounded", len(top), sortedYAMLKeys(top))
	}
	for _, want := range []string{"builds", "archives", "release"} {
		if _, ok := top[want]; !ok {
			t.Fatalf("CONTROL failure: .goreleaser.yaml has no top-level %q key (found %v) — "+
				"the parse is wrong, not the config", want, sortedYAMLKeys(top))
		}
	}
	return top, raw
}

func sortedYAMLKeys(m map[string]yaml.Node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readmeHomebrewHeading returns the README's single `### Homebrew …` heading
// text and its anchor slug.
func readmeHomebrewHeading(t *testing.T, md string) (text, slug string) {
	t.Helper()
	var found []string
	for s, h := range readmeHeadings(t, md) {
		if strings.HasPrefix(h, "Homebrew") {
			found = append(found, s+"\x00"+h)
		}
	}
	if len(found) != 1 {
		sort.Strings(found)
		t.Fatalf("expected exactly one README heading starting with \"Homebrew\", found %d (%v) — "+
			"the install section moved and this guard is reading the wrong document", len(found), found)
	}
	parts := strings.SplitN(found[0], "\x00", 2)
	return parts[1], parts[0]
}

// readmeRawSection returns the README section under the heading line whose text
// equals `heading`, bounded by the next same-or-higher-level heading — WITHOUT
// stripping fenced code.
//
// 🔴 It exists because readmeSectionByAnchor (which does strip fences) returns
// the EMPTY STRING for a subsection whose whole body is a code block, and the
// Homebrew subsection was exactly that before this change. A guard whose
// anti-vacuity floor is the first thing to fire on the very document it was
// written to reject reports a CONTROL failure instead of the defect — true, but
// it is not the claim, and the substantive assertion never ran.
func readmeRawSection(t *testing.T, md, heading string) string {
	t.Helper()
	lines := strings.Split(md, "\n")
	start, level := -1, 0
	for i, line := range lines {
		m := anyHeadingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[2] == heading {
			start, level = i+1, len(m[1])
			break
		}
	}
	if start < 0 {
		t.Fatalf("README.md has no heading with the exact text %q", heading)
	}
	for i := start; i < len(lines); i++ {
		if m := anyHeadingRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= level {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// homebrewMacOSOnlyClaim is the README's macOS-only refusal, pinned as a WHOLE
// normalised sentence rather than as keywords.
//
// 🔴 The artifact under test is PROSE, and a guard on individual words is
// walkable by rewording: "Homebrew" + "macOS" + "Linux" all appear in a sentence
// that re-advertises Linux just as readily as in one that refuses it. Pinning
// the whole string means a cosmetic reword fails this test — that cost is the
// point, because the claim then has a machine-readable definition.
const homebrewMacOSOnlyClaim = "🔴 **macOS only — there is no Linux Homebrew install.**"

// homebrewNoFormulaClaim is the README's statement of WHY, tied to the stanza
// this guard reads out of `.goreleaser.yaml`.
//
// 🔴 It NAMES THE FILE, and that is the correction, not decoration. An earlier
// wording credited the missing stanza to `civitai/homebrew-tap`; `brews:` and
// `homebrew_casks:` are goreleaser stanzas in THIS repo's `.goreleaser.yaml`,
// which is the file this guard parses, so a reader following the "why" was sent
// to the wrong repository to check it.
const homebrewNoFormulaClaim = "`.goreleaser.yaml` carries no `brews:` (formula) stanza at all"

// homebrewMacOSExclusivityIntro is the `## Install` intro's macOS-exclusivity
// clause, pinned whole and required in the FORWARD direction so that forbidding
// it in the reverse direction is a real assertion rather than a vacuous one.
const homebrewMacOSExclusivityIntro = "it is the only platform the tap covers"

// upgradeLongBrewMacOSScope is the SAME macOS-only fact as it has to appear in
// what `civitai upgrade --help` prints.
//
// 🔴 A THIRD SURFACE, pinned for AGENTS.md's reason: the command's own help
// text, the README section and the command-reference row each state this
// contract and each goes stale ALONE. This one did — the README was scoped to
// macOS while `upgrade --help` still told users on every platform, flat, that a
// Homebrew install delegates to `brew upgrade civitai/tap/civitai`.
const upgradeLongBrewMacOSScope = "The release publishes a Homebrew CASK, which is macOS-only, " +
	"so that delegation only leads anywhere on macOS."

// readUpgradeSource returns internal/cmd/upgrade.go. `Long` is a raw backquoted
// literal, so its prose appears verbatim in the file and flattenWS makes it
// matchable across the wrapping.
func readUpgradeSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRootDir(t), "internal", "cmd", "upgrade.go"))
	if err != nil {
		t.Fatalf("read internal/cmd/upgrade.go: %v", err)
	}
	// POSITIVE CONTROL: the delegation sentence the callers reason about has to
	// be in the file at all, or every verdict is about text that moved.
	if !strings.Contains(string(b), "brew upgrade civitai/tap/civitai") {
		t.Fatalf("CONTROL failure: internal/cmd/upgrade.go no longer mentions " +
			"`brew upgrade civitai/tap/civitai` — the Homebrew delegation moved, so a verdict about how " +
			"its help text scopes that delegation would be about nothing")
	}
	return string(b)
}

// upgradeLongText returns the `Long:` raw string literal from upgrade.go — the
// exact prose `civitai upgrade --help` prints. Extracted rather than printing
// the whole file, so a failure message shows the surface under test.
func upgradeLongText(t *testing.T, src string) string {
	t.Helper()
	i := strings.Index(src, "Long: `")
	if i < 0 {
		t.Fatalf("CONTROL failure: internal/cmd/upgrade.go has no `Long: ` + raw string literal — " +
			"the help text moved or changed form, so a verdict about its wording is about nothing")
	}
	rest := src[i+len("Long: `"):]
	j := strings.Index(rest, "`")
	if j <= 0 {
		t.Fatal("CONTROL failure: upgrade.go's Long literal is unterminated or empty")
	}
	return rest[:j]
}

// TestREADMEHomebrewSectionMatchesTheReleaseConfig ties the README's Homebrew
// section to what the release pipeline actually publishes.
//
// `.goreleaser.yaml` carries `homebrew_casks:` and NO `brews:` stanza, and a
// Homebrew *cask* is macOS-only BY DEFINITION — Homebrew implements casks on
// macOS only, so the artifact type alone settles it. (An earlier version of this
// comment cited tools/caskcheck/testdata/drill-broken-cask.rb as evidence that
// the rendered cask names darwin archives. That file is a hand-written FIRE-DRILL
// FIXTURE whose own header says it is "not the tap, not shipped to anyone", so it
// evidences nothing about goreleaser's output: right conclusion, wrong witness.)
// So there is no Linux Homebrew install — yet the README advertised
// `### Homebrew (macOS / Linux)` and told Linux readers it was "quickest" for
// them.
//
// 🔴 IT FAILS IN BOTH DIRECTIONS, AND BOTH DIRECTIONS COVER THE SAME THREE
// SURFACES. The forward branch used to demand three things (heading, Install
// intro, section refusal) while the reverse branch checked only that the refusal
// was gone — so adding a `brews:` stanza AND deleting only the refusal left the
// suite green with the heading still scoped `(macOS)` and the intro still saying
// Homebrew is macOS-only, which is the exact steering-Linux-users-away failure
// this guard's own summary claims to prevent.
//   - No `brews:` stanza (today): the heading must not name Linux, the Install
//     intro must carry the macOS-exclusivity clause and must not pitch Homebrew
//     at Linux, and the section must carry the refusal above plus its reason.
//   - A `brews:` stanza appears (a real Linux formula is published): the refusal,
//     its `.goreleaser.yaml` reason, the intro's exclusivity clause and a
//     macOS-scoped heading must ALL be gone. That is the silent direction — a
//     stale "Linux is not supported" claim on ANY of those surfaces reads as a
//     current rule and would send Linux users away from an install that works.
func TestREADMEHomebrewSectionMatchesTheReleaseConfig(t *testing.T) {
	top, _ := readGoreleaserConfig(t)
	_, hasBrews := top["brews"]
	_, hasCasks := top["homebrew_casks"]

	// CONTROL: if BOTH tap stanzas are gone the Homebrew section should not exist
	// at all, and neither branch below is the right question. Say so rather than
	// silently taking the "no brews" branch.
	if !hasCasks && !hasBrews {
		t.Fatalf("CONTROL failure: .goreleaser.yaml has neither `homebrew_casks:` nor `brews:` (keys: %v). "+
			"The CLI publishes no Homebrew artifact at all, so the README's Homebrew section documents "+
			"an install that does not exist — delete it, and this guard with it", sortedYAMLKeys(top))
	}

	md := readREADME(t)
	heading, slug := readmeHomebrewHeading(t, md)
	// The fence-stripping extractor is still called, purely so a BROKEN ANCHOR
	// fails here (it Fatals) rather than silently widening the text searched.
	_ = readmeSectionByAnchor(t, md, slug)
	body := readmeRawSection(t, md, heading)
	// POSITIVE CONTROL: the section must really be the Homebrew one. A body that
	// does not carry the install command is the wrong block, and every verdict
	// below — in either direction — would be about text this guard is not reading.
	if !strings.Contains(body, "brew install civitai/tap/civitai") {
		t.Fatalf("CONTROL failure: the README's `### %s` section does not contain "+
			"`brew install civitai/tap/civitai`:\n%s\n\nThe extractor is reading the wrong block", heading, body)
	}
	flatBody := flattenWS(body)

	// The Install intro is the OTHER place the claim lives — the two go stale
	// independently, which is how #371 shipped a two-of-three fix.
	install := readmeSectionByAnchor(t, md, "install")
	intro := install
	if i := strings.Index(intro, "\n### "); i >= 0 {
		intro = intro[:i]
	}
	if !strings.Contains(intro, "Homebrew") {
		t.Fatalf("CONTROL failure: the `## Install` intro does not mention Homebrew at all:\n%s\n"+
			"A 'does not pitch Homebrew at Linux' verdict from this text would mean "+
			"'wrong text read', not 'claim corrected'", intro)
	}

	flatIntro := flattenWS(intro)
	// The help text itself, not the whole file: a match anywhere in upgrade.go
	// would count a code comment as a user-facing claim.
	flatUpgradeLong := flattenWS(upgradeLongText(t, readUpgradeSource(t)))

	if hasBrews {
		// Reverse direction: a Linux formula now exists. Every surface the forward
		// direction pins has to be un-pinned, not just the refusal sentence.
		const because = ".goreleaser.yaml now carries a top-level `brews:` stanza — a Homebrew FORMULA, " +
			"which DOES install on Linux"
		if strings.Contains(flatBody, flattenWS(homebrewMacOSOnlyClaim)) {
			t.Errorf("%s — but README's `### %s` section still says %q.\n\n"+
				"Delete that refusal and re-advertise Linux. A stale 'not supported' paragraph is worse than "+
				"none: it reads as a current rule and steers Linux users away from an install that works.",
				because, heading, homebrewMacOSOnlyClaim)
		}
		if strings.Contains(flatBody, homebrewNoFormulaClaim) {
			t.Errorf("%s — but README's `### %s` section still states the old reason %q, which is now "+
				"FALSE about the very file this guard just parsed.\n\n"+
				"The reason is what makes the refusal checkable; leaving it behind gives a reader a "+
				"verifiable-looking claim that the config contradicts.", because, heading, homebrewNoFormulaClaim)
		}
		if strings.Contains(flatIntro, homebrewMacOSExclusivityIntro) {
			t.Errorf("%s — but the `## Install` intro still says %q:\n  %s\n\n"+
				"The intro and the Homebrew section are separate surfaces and go stale ALONE (AGENTS.md; "+
				"#371 shipped a two-of-three fix). A reader who never scrolls to the section is told here "+
				"that Homebrew is macOS-only.", because, homebrewMacOSExclusivityIntro, flatIntro)
		}
		if regexp.MustCompile(`(?i)mac\s*os|macos|darwin`).MatchString(heading) &&
			!regexp.MustCompile(`(?i)linux`).MatchString(heading) {
			t.Errorf("%s — but README's Homebrew heading is still `### %s`, which scopes the install to "+
				"macOS and never names Linux.\n\n"+
				"The heading is the first and most-linked surface (`#homebrew-macos` is referenced from "+
				"§Upgrading), so a Linux reader stops there. Widen it, or drop the platform from it.",
				because, heading)
		}
		if strings.Contains(flatUpgradeLong, flattenWS(upgradeLongBrewMacOSScope)) {
			t.Errorf("%s — but `civitai upgrade --help` still says %q.\n\n"+
				"Help text is the surface a user reads INSTEAD of the README. Delete the macOS scoping "+
				"there too.", because, upgradeLongBrewMacOSScope)
		}
		return
	}

	// Forward direction: cask only, so macOS only.
	if regexp.MustCompile(`(?i)linux`).MatchString(heading) {
		t.Errorf("README's Homebrew heading is `### %s`, but .goreleaser.yaml publishes a Homebrew CASK "+
			"(`homebrew_casks:`) and NO `brews:` formula stanza. Casks are macOS-only and the rendered cask "+
			"names darwin archives only, so `brew install civitai/tap/civitai` installs nothing on Linux. "+
			"Name only the platform the tap covers.", heading)
	}
	if !strings.Contains(flatBody, flattenWS(homebrewMacOSOnlyClaim)) {
		t.Errorf("README's `### %s` section does not carry the macOS-only refusal.\n\nwant (normalised): %s\n\ngot:\n%s\n\n"+
			".goreleaser.yaml has no `brews:` stanza, so there is no Linux Homebrew install to advertise — "+
			"the section must say so, and point Linux readers at npm / Nix / the prebuilt binary / go install.",
			heading, flattenWS(homebrewMacOSOnlyClaim), flatBody)
	}
	if !strings.Contains(flatBody, homebrewNoFormulaClaim) {
		t.Errorf("README's `### %s` section does not state WHY (%q). The reason is the thing this guard reads "+
			"out of .goreleaser.yaml; without it the refusal is an assertion a reader cannot check.",
			heading, homebrewNoFormulaClaim)
	}
	// The section must route Linux readers somewhere that works. Anchors are
	// separately proven to resolve by TestREADMEAnchorLinksResolve.
	for _, want := range []string{"#npm-node", "#nix-flake", "#prebuilt-binary", "#go-install-from-source-go-125"} {
		if !strings.Contains(body, want) {
			t.Errorf("README's `### %s` section refuses Linux but does not link the working alternative %q — "+
				"a refusal with no next step is where a reader stops", heading, want)
		}
	}
	if regexp.MustCompile(`(?i)homebrew[^.]*\blinux\b`).MatchString(flatIntro) {
		t.Errorf("the `## Install` intro still pitches Homebrew at Linux:\n%s\n\n"+
			"Only a cask is published; there is no Linux Homebrew install.", flatIntro)
	}
	// 🔴 The POSITIVE half of the reverse branch's ban. Without this, "the intro
	// must not claim macOS-exclusivity once a formula exists" is a check on a
	// string that need never have been there — the shape that reads as coverage
	// while providing none. Requiring it here makes the pair a real bidirectional
	// pin on the ONE sentence a reader meets before any heading.
	if !strings.Contains(flatIntro, homebrewMacOSExclusivityIntro) {
		t.Errorf("the `## Install` intro does not scope Homebrew to macOS.\n\nwant (normalised, verbatim): %s\n\ngot:\n%s\n\n"+
			".goreleaser.yaml has no `brews:` stanza, so `brew install civitai/tap/civitai` installs nothing "+
			"off macOS. The intro is where a reader chooses an install method, and it has to say so there — "+
			"not only in the section further down, which they may never reach.",
			homebrewMacOSExclusivityIntro, flatIntro)
	}
	// 🔴 THE THIRD SURFACE. `civitai upgrade --help` announces the same Homebrew
	// delegation and is what a user reads INSTEAD of the README.
	if !strings.Contains(flatUpgradeLong, flattenWS(upgradeLongBrewMacOSScope)) {
		t.Errorf("`civitai upgrade --help` announces the `brew upgrade civitai/tap/civitai` delegation "+
			"without scoping it to macOS.\n\nwant (normalised, verbatim, in upgrade.go's Long): %s\n\n"+
			".goreleaser.yaml publishes a CASK and no `brews:` formula, so that delegation resolves on "+
			"macOS only. An unscoped sentence in help text is the version most users see — the README "+
			"being right does not fix it.", upgradeLongBrewMacOSScope)
	}
}

// windowsUpgradeCaveat is the README's Windows refusal for `civitai upgrade`,
// pinned whole for the same reason as homebrewMacOSOnlyClaim.
const windowsUpgradeCaveat = "🔴 **`civitai upgrade` does not work on Windows — use the manual path.**"

// windowsUpgradeRowCaveat is the SAME claim as it has to appear in the
// command-reference table's `civitai upgrade` row — a separate published
// surface, pinned whole for the same reason.
const windowsUpgradeRowCaveat = "🔴 **Not on Windows**: the `.tar.gz` asset it looks for is never published " +
	"there, so it fails and replaces nothing — upgrade by hand."

// upgradeAssetNameRe finds the ONE release-asset name `runUpgrade` builds. The
// extension is captured, because the extension is the entire defect.
var upgradeAssetNameRe = regexp.MustCompile(`"civitai_%s_%s_%s\.([a-z.]+)"`)

// TestREADMEWindowsUpgradeCaveatMatchesTheReleaseFormats ties the README's
// Windows caveat to the SEAM between two files that are each individually
// correct: `.goreleaser.yaml` publishes Windows as a `.zip`
// (`format_overrides`), while `internal/cmd/upgrade.go` asks for a `.tar.gz`
// unconditionally. Neither file is wrong on its own; together they mean
// `civitai upgrade` can never succeed on Windows, and §Install advertises
// Windows binaries.
//
// 🔴 IT FAILS IN BOTH DIRECTIONS, and it compares the two derived names rather
// than grepping each file for a keyword — a keyword pair would stay green if one
// side changed to some third format.
//   - The names disagree (today): the README must carry the caveat.
//   - The names agree (someone gave upgrade.go a per-GOOS extension, or the
//     release stopped overriding Windows): the caveat must be REMOVED. That is
//     the silent direction — a stale "does not work on Windows" tells users to
//     do by hand what the command would now do for them.
//
// What it does NOT check: that a real Windows `civitai upgrade` produces that
// error. `runtime.GOOS` is fixed at compile time, so the only honest place to
// confirm the end-to-end symptom is a Windows machine.
func TestREADMEWindowsUpgradeCaveatMatchesTheReleaseFormats(t *testing.T) {
	_, raw := readGoreleaserConfig(t)

	var cfg struct {
		Archives []struct {
			ID              string `yaml:"id"`
			Formats         []string
			FormatOverrides []struct {
				Goos    string   `yaml:"goos"`
				Formats []string `yaml:"formats"`
			} `yaml:"format_overrides"`
		} `yaml:"archives"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml archives: %v", err)
	}
	if len(cfg.Archives) < 2 {
		t.Fatalf("CONTROL failure: parsed %d archive(s) from .goreleaser.yaml, want at least 2 "+
			"(the tar.gz archive and the raw binary) — the `archives:` key moved", len(cfg.Archives))
	}

	// The extension the RELEASE publishes for Windows, for the archive the
	// self-updater downloads (id `civitai`, the tar/zip one — not `civitai-raw`).
	winExt := ""
	found := false
	for _, a := range cfg.Archives {
		if a.ID != "civitai" {
			continue
		}
		found = true
		for _, fo := range a.FormatOverrides {
			if fo.Goos == "windows" && len(fo.Formats) > 0 {
				winExt = fo.Formats[0]
			}
		}
	}
	if !found {
		t.Fatalf("CONTROL failure: .goreleaser.yaml has no archive with id `civitai` — that is the archive " +
			"`civitai upgrade` downloads, so without it this guard is comparing nothing")
	}
	if winExt == "" {
		winExt = "tar.gz" // no override: Windows gets the default archive format.
	}

	// The extension the CLI ASKS FOR, read out of the production source.
	upgradeSrc := readUpgradeSource(t)
	m := upgradeAssetNameRe.FindAllStringSubmatch(upgradeSrc, -1)
	if len(m) != 1 {
		t.Fatalf("CONTROL failure: found %d asset-name template(s) matching %s in internal/cmd/upgrade.go, want exactly 1. "+
			"Either the name is now built some other way (in which case re-derive this comparison rather than "+
			"deleting it) or this regex has stopped matching and every verdict below is vacuous",
			len(m), upgradeAssetNameRe)
	}
	cliExt := m[0][1]

	// A per-GOOS branch would make the single template above an incomplete read.
	if regexp.MustCompile(`runtime\.GOOS\s*==\s*"windows"`).MatchString(upgradeSrc) {
		t.Fatalf("internal/cmd/upgrade.go now branches on runtime.GOOS == \"windows\", so the single asset-name " +
			"template this guard reads is no longer the whole story. Re-derive the comparison against the new code.")
	}

	md := readREADME(t)
	body := readmeSectionByAnchor(t, md, "upgrading")
	if len(strings.TrimSpace(body)) < 500 {
		t.Fatalf("CONTROL failure: the README's `## Upgrading` section is only %d byte(s) long — "+
			"the extractor is reading the wrong block", len(strings.TrimSpace(body)))
	}
	hasCaveat := strings.Contains(flattenWS(body), flattenWS(windowsUpgradeCaveat))

	// 🔴 THE SAME CLAIM ON A SECOND SURFACE. AGENTS.md: the command section, the
	// exit-code table and the Troubleshooting index each state the contract and
	// each goes stale ALONE — #371 shipped having updated two of three. The
	// `civitai upgrade` row sat unscoped for the whole life of the §Upgrading
	// caveat, so a reader of the command table was told the command self-updates
	// the binary, full stop.
	row := ""
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "| `civitai upgrade") {
			if row != "" {
				t.Fatalf("the command-reference table has two `civitai upgrade` rows; this guard would pin "+
					"whichever came first:\n%s\n%s", row, line)
			}
			row = line
		}
	}
	if row == "" {
		t.Fatal("CONTROL failure: the README has no `| `civitai upgrade` command-reference row — the row " +
			"scan is reading the wrong document, so its verdict below is about nothing")
	}
	hasRowCaveat := strings.Contains(flattenWS(row), flattenWS(windowsUpgradeRowCaveat))

	// 🔴 AND A THIRD: `civitai upgrade --help` itself. It is the surface a user
	// reaches without opening the README at all, and it carried the unscoped
	// claim for the entire life of the §Upgrading caveat. The expected sentence
	// is BUILT from the two extensions derived above rather than hardcoded, so
	// changing either side moves the expectation with it instead of leaving this
	// asserting a string nothing produces.
	longCaveat := fmt.Sprintf("This command asks for a .%s release asset, but Windows is published "+
		"as a .%s, so the lookup never matches", cliExt, winExt)
	flatUpgradeLong := flattenWS(upgradeLongText(t, upgradeSrc))
	hasLongCaveat := strings.Contains(flatUpgradeLong, longCaveat) &&
		strings.Contains(flatUpgradeLong, "Not on Windows.")

	if cliExt == winExt {
		if hasCaveat {
			t.Errorf("internal/cmd/upgrade.go now asks for a .%s asset and .goreleaser.yaml publishes Windows as "+
				".%s — they AGREE, so `civitai upgrade` can find its Windows asset. Delete the README caveat %q.\n\n"+
				"A stale refusal tells Windows users to do by hand what the command now does for them.",
				cliExt, winExt, windowsUpgradeCaveat)
		}
		if hasRowCaveat {
			t.Errorf("the extensions AGREE (.%s asked, .%s published) but the `civitai upgrade` command-reference "+
				"row still says %q. Delete it there too — the row and the §Upgrading section state the same "+
				"contract and go stale independently.", cliExt, winExt, windowsUpgradeRowCaveat)
		}
		if hasLongCaveat {
			t.Errorf("the extensions AGREE (.%s asked, .%s published) but `civitai upgrade --help` still "+
				"refuses Windows (%q). Delete it from upgrade.go's Long too — help text is the surface a "+
				"user reads without opening the README, so a stale refusal there tells Windows users to "+
				"do by hand what the command now does for them.", cliExt, winExt, longCaveat)
		}
		return
	}
	if !hasLongCaveat {
		t.Errorf("`civitai upgrade --help` does not scope its own contract to the platforms it holds "+
			"on.\n\nwant in internal/cmd/upgrade.go's Long (normalised): \"Not on Windows.\" … %q\n\n"+
			"got (normalised, upgrade.go's Long):\n%s\n\nThe command asks for a `.%s` asset that is never "+
			"published for Windows. AGENTS.md: the command surface, the README section and the "+
			"Troubleshooting index each state the contract and each goes stale ALONE — this one did.",
			longCaveat, flatUpgradeLong, cliExt)
	}
	if !hasRowCaveat {
		t.Errorf("the `civitai upgrade` command-reference row does not scope the claim to the platforms it "+
			"holds on:\n  %s\n\nwant (normalised): %s\n\n`civitai upgrade` asks for a `.%s` asset that is never "+
			"published for Windows, so the row's \"self-update this binary in place\" is false there. The row "+
			"and the §Upgrading section are separate surfaces and go stale alone.",
			flattenWS(row), flattenWS(windowsUpgradeRowCaveat), cliExt)
	}

	if !hasCaveat {
		t.Errorf("`civitai upgrade` asks for a `.%s` release asset (internal/cmd/upgrade.go) but .goreleaser.yaml "+
			"publishes Windows as `.%s` (archives[civitai].format_overrides, goos: windows). The asset lookup can "+
			"never match on Windows, and §Install advertises Windows binaries — yet the README's `## Upgrading` "+
			"section carries no caveat.\n\nwant (normalised): %s\n\ngot:\n%s",
			cliExt, winExt, flattenWS(windowsUpgradeCaveat), flattenWS(body))
		return
	}
	// The caveat must name the two facts it is derived from AND the way out.
	flat := flattenWS(body)
	for _, want := range []string{"." + cliExt, "." + winExt, "upgrade manually", "nothing is replaced"} {
		if !strings.Contains(strings.ToLower(flat), strings.ToLower(want)) {
			t.Errorf("the README's Windows upgrade caveat does not mention %q. It is derived from a `.%s` ask "+
				"against a `.%s` publish, and its whole value to a reader is the manual path plus the fact that "+
				"the failed lookup leaves the binary untouched.", want, cliExt, winExt)
		}
	}
}

// boolValueCandidates is the universe this guard classifies with
// strconv.ParseBool. It deliberately mixes the twelve spellings Go accepts with
// the ones users reach for from other tools (`yes`, `on`, `y`, `enabled`) and
// the NO_COLOR convention's own case (`""`), so the accepted/rejected split is
// derived here rather than copied from the README it is checking.
var boolValueCandidates = []string{
	"1", "t", "T", "TRUE", "true", "True",
	"0", "f", "F", "FALSE", "false", "False",
	"yes", "YES", "Yes", "no", "on", "off", "y", "n", "enabled", "2", "",
}

// TestREADMEColorEnvValuesMatchTheBooleanParser pins the exact set of values
// `CIVITAI_NO_COLOR` / `CIVITAI_COLOR` accept, because that set is NOT the one
// `NO_COLOR` / `CLICOLOR_FORCE` accept and the README used to present the pairs
// as interchangeable.
//
// `internal/cmd/root.go` binds the CIVITAI_* variables into viper and reads them
// with GetBool → cast.ToBool → strconv.ParseBool, so only twelve spellings mean
// anything and `CIVITAI_NO_COLOR=yes` silently parses to false and does nothing.
// `internal/ui` reads the standard variables with envSet/envTrue instead, where
// any non-empty value counts.
//
// The guard has three legs, because the claim spans three surfaces and a check
// on one of them would go stale against the other two:
//  1. BEHAVIOURAL — the same viper binding root.go uses, exercised per value.
//  2. LEDGER — root.go must still bind and read those keys that way; a switch to
//     GetString/envSet semantics reddens this and forces the README to be redone.
//  3. DOCUMENT — the README must name every accepted spelling and at least one
//     value that silently does nothing.
func TestREADMEColorEnvValuesMatchTheBooleanParser(t *testing.T) {
	var accepted, rejected []string
	for _, v := range boolValueCandidates {
		if _, err := strconv.ParseBool(v); err == nil {
			accepted = append(accepted, v)
		} else {
			rejected = append(rejected, v)
		}
	}
	// POSITIVE CONTROL on the derivation: both halves must be non-trivial, or the
	// document assertions below are checking an empty set.
	if len(accepted) != 12 {
		t.Fatalf("CONTROL failure: strconv.ParseBool accepts %d of the %d candidate(s) (%v), want exactly 12. "+
			"The Go boolean grammar changed, or boolValueCandidates lost entries — re-derive the README's list "+
			"rather than adjusting this number", len(accepted), len(boolValueCandidates), accepted)
	}
	if len(rejected) < 5 {
		t.Fatalf("CONTROL failure: only %d candidate value(s) are REJECTED by strconv.ParseBool (%v). "+
			"Without rejected values this guard cannot show the asymmetry it exists to document", len(rejected), rejected)
	}

	// --- Leg 1: behavioural, through the exact mechanism root.go uses. ---
	for _, envName := range []string{"CIVITAI_NO_COLOR", "CIVITAI_COLOR"} {
		key := map[string]string{"CIVITAI_NO_COLOR": "no_color", "CIVITAI_COLOR": "color"}[envName]
		for _, v := range boolValueCandidates {
			want, parseErr := strconv.ParseBool(v)
			if parseErr != nil {
				want = false
			}
			t.Setenv(envName, v)
			vp := viper.New()
			if err := vp.BindEnv(key, envName); err != nil {
				t.Fatalf("BindEnv(%q, %q): %v", key, envName, err)
			}
			if got := vp.GetBool(key); got != want {
				t.Errorf("%s=%q resolves to %v, want %v — the README's documented value set is derived from "+
					"strconv.ParseBool and this value does not follow it", envName, v, got, want)
			}
		}
	}
	// The single fact the README now states and used to contradict.
	t.Setenv("CIVITAI_NO_COLOR", "yes")
	vp := viper.New()
	if err := vp.BindEnv("no_color", "CIVITAI_NO_COLOR"); err != nil {
		t.Fatalf("BindEnv: %v", err)
	}
	if vp.GetBool("no_color") {
		t.Fatal("CIVITAI_NO_COLOR=yes now disables colour — the README says it does nothing. Re-derive both.")
	}

	// --- Leg 1b: the ASYMMETRY. `NO_COLOR=yes` really does disable colour. ---
	// Restore the shared ui state afterwards the way the other cmd tests do.
	t.Cleanup(func() { ui.Configure(ui.Options{Writer: io.Discard}) })
	t.Setenv("CIVITAI_NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "yes")
	t.Setenv("NO_COLOR", "")
	ui.Configure(ui.Options{Writer: io.Discard})
	if !ui.EnabledFor(io.Discard) {
		t.Error("CLICOLOR_FORCE=yes did not force colour on — internal/ui reads the standard variables with " +
			"envTrue (non-empty and not \"0\"), so `yes` counts there even though it does nothing for CIVITAI_COLOR. " +
			"That asymmetry is exactly what the README now documents.")
	}
	t.Setenv("NO_COLOR", "yes")
	ui.Configure(ui.Options{Writer: io.Discard})
	if ui.EnabledFor(io.Discard) {
		t.Error("NO_COLOR=yes did not disable colour — the README says any non-empty NO_COLOR does, " +
			"in contrast with CIVITAI_NO_COLOR=yes which does nothing")
	}

	// --- Leg 1c: `0`, the ONE value where the two contracts give OPPOSITE answers. ---
	//
	// 🔴 `yes` above is a value where the two rules AGREE in direction, so it
	// cannot see the divergence the README's two new sentences state. `0` is the
	// only value in boolValueCandidates where NO_COLOR (presence-only: it
	// disables) and CIVITAI_NO_COLOR (parsed boolean: a real false, colour left
	// alone) disagree, and it is the value the README names twice — "even
	// `NO_COLOR=0` disables colour" and "an explicit `CIVITAI_NO_COLOR=0` is a
	// real *false* that leaves colour alone (unlike `NO_COLOR=0`)".
	//
	// Mutation-measured: giving internal/ui's envSet the `v != "0"` clause that
	// envTrue carries — `return ok && v != "" && v != "0"`, a one-clause
	// "harmonise the two helpers" tidy-up — left the ENTIRE suite green (21 ok,
	// 0 FAIL) while both of those sentences became false.
	//
	// CLICOLOR_FORCE stays `yes` here ON PURPOSE. io.Discard is not a TTY, so
	// with no force-ON in play resolveMode's auto branch answers "colour off"
	// regardless of whether NO_COLOR was consulted at all, and this assertion
	// would pass vacuously under the mutant. The CLICOLOR_FORCE=yes check above
	// is the positive control that force-ON really is in effect.
	t.Setenv("NO_COLOR", "0")
	ui.Configure(ui.Options{Writer: io.Discard})
	if ui.EnabledFor(io.Discard) {
		t.Error("NO_COLOR=0 did not disable colour, with CLICOLOR_FORCE=yes forcing it on. internal/ui " +
			"reads NO_COLOR with envSet — PRESENT and non-empty, the VALUE is not parsed — so `0` " +
			"disables colour like any other non-empty value. That is the no-color.org contract and the " +
			"README states it outright. If envSet grew a `v != \"0\"` clause (harmonising it with " +
			"envTrue, which legitimately has one for CLICOLOR_FORCE), the README is now wrong in two " +
			"places: re-derive both, do not adjust this test.")
	}
	// The OTHER half of the same sentence, at the SAME value: CIVITAI_NO_COLOR is
	// a parsed boolean, so `0` is a real false and leaves colour alone. Asserted
	// here beside its counterpart rather than only inside the loop above, because
	// the published claim is the CONTRAST between the two, not either half.
	t.Setenv("CIVITAI_NO_COLOR", "0")
	vpZero := viper.New()
	if err := vpZero.BindEnv("no_color", "CIVITAI_NO_COLOR"); err != nil {
		t.Fatalf("BindEnv: %v", err)
	}
	if vpZero.GetBool("no_color") {
		t.Error("CIVITAI_NO_COLOR=0 now disables colour. It is read as a BOOLEAN (GetBool → cast.ToBool " +
			"→ strconv.ParseBool), so `0` is a real false that leaves colour alone — which is exactly " +
			"the contrast the README draws against NO_COLOR=0, where the same value DOES disable. " +
			"Re-derive both.")
	}
	t.Setenv("CIVITAI_NO_COLOR", "")

	// --- Leg 2: the ledger, so the mechanism cannot change under the prose. ---
	rootSrc, err := os.ReadFile(filepath.Join(repoRootDir(t), "internal", "cmd", "root.go"))
	if err != nil {
		t.Fatalf("read internal/cmd/root.go: %v", err)
	}
	for _, want := range []string{
		`BindEnv("no_color", "CIVITAI_NO_COLOR")`,
		`BindEnv("color", "CIVITAI_COLOR")`,
		`GetBool("no_color")`,
		`GetBool("color")`,
	} {
		if !strings.Contains(string(rootSrc), want) {
			t.Errorf("internal/cmd/root.go no longer contains %s. The README's documented value set is a claim "+
				"about GetBool (→ cast.ToBool → strconv.ParseBool); a different read (GetString, an envSet-style "+
				"presence test) accepts a DIFFERENT set of values and makes that prose wrong. Re-derive both.", want)
		}
	}

	// --- Leg 3: the document. ---
	md := readREADME(t)
	body := readmeSectionByAnchor(t, md, "global-flags")
	if len(strings.TrimSpace(body)) < 800 {
		t.Fatalf("CONTROL failure: the README's `## Global flags` section is only %d byte(s) long — "+
			"the extractor is reading the wrong block", len(strings.TrimSpace(body)))
	}
	flat := flattenWS(body)
	for _, v := range accepted {
		if !strings.Contains(flat, "`"+v+"`") {
			t.Errorf("the README's `## Global flags` section does not name the accepted value `%s` for "+
				"CIVITAI_NO_COLOR / CIVITAI_COLOR. strconv.ParseBool accepts exactly %v and the section has to "+
				"enumerate them, because every OTHER value parses as false and does nothing with no warning.",
				v, accepted)
		}
	}
	// It must also say, by example, that an unlisted value is inert — an
	// enumeration a reader does not know is EXHAUSTIVE is not one.
	if !strings.Contains(flat, "`yes`") {
		t.Errorf("the README's `## Global flags` section does not name `yes` as a value that does nothing. " +
			"strconv.ParseBool rejects it, so CIVITAI_NO_COLOR=yes silently leaves colour on — the single " +
			"symptom this documentation exists to prevent.")
	}
	for _, want := range []string{
		"CIVITAI_NO_COLOR=yes",
		"not interchangeable",
	} {
		if !strings.Contains(strings.ToLower(flat), strings.ToLower(want)) {
			t.Errorf("the README's `## Global flags` section does not state %q. The two pairs of variables "+
				"accept different value sets, and presenting them as equivalent is the defect being fixed.", want)
		}
	}
}

// TestREADMECommandReferencePointsAtTheReadCommandTable pins the pointer added
// to `## Command reference`, whose table covers the authoring and account
// commands only.
//
// 🔴 IT IS DERIVED FROM THE COBRA TREE, not from a hardcoded list, so a read
// command added to the CLI is covered the day it lands. For each root command
// that the reference table does NOT carry a row for, the section must name it —
// otherwise a reader who lands there is told neither that the table is partial
// nor where the rest is.
func TestREADMECommandReferencePointsAtTheReadCommandTable(t *testing.T) {
	md := readREADME(t)
	subjects := readmeCommandTableSubjects(t, md)
	if len(subjects) < 15 {
		t.Fatalf("CONTROL failure: extracted only %d command-reference rows — the table extractor is "+
			"reading the wrong block", len(subjects))
	}

	section := readmeSectionByAnchor(t, md, "command-reference")
	// The prose above the table, which is where the pointer has to be: a reader
	// must meet it before the table, not after 25 rows of it.
	preamble := section
	if i := strings.Index(preamble, "| Command | What it does |"); i >= 0 {
		preamble = preamble[:i]
	}
	// NOT a control — this is the defect. Before this guard landed the section
	// opened straight onto its table, so a reader was never told it is partial.
	if len(strings.TrimSpace(preamble)) < 50 {
		t.Errorf("the `## Command reference` section has only %d byte(s) of prose before its table, so it "+
			"carries no pointer at all. Its table covers the authoring and account commands only; the note "+
			"saying so has to sit ahead of the table it qualifies.", len(strings.TrimSpace(preamble)))
	}
	if !strings.Contains(preamble, "#browse-the-public-api") {
		t.Errorf("the `## Command reference` preamble does not link #browse-the-public-api. Its table does not " +
			"cover the public-API read commands, so a reader has to be told where they are — with a working " +
			"in-page anchor, not a bare mention.")
	}

	root := NewRootCmd()
	undocumented := 0
	for _, sub := range root.Commands() {
		name := sub.Name()
		if sub.Hidden || sub.IsAdditionalHelpTopicCommand() || name == "help" || name == "completion" {
			continue
		}
		inTable := false
		for _, s := range subjects {
			if strings.Contains(s, "civitai "+name) {
				inTable = true
				break
			}
		}
		if inTable {
			continue
		}
		undocumented++
		if !strings.Contains(preamble, "`"+name+"`") && !strings.Contains(preamble, "`civitai "+name+"`") {
			t.Errorf("`civitai %s` has no row in the command-reference table AND is not named in the pointer "+
				"above it. A reader in that section is told neither that the table is partial nor where %s is "+
				"documented — add it to the pointer, or give it a row.", name, name)
		}
	}
	// A tree walk that matched everything would report a serene pass: the table
	// really does omit the read commands, and that is the premise of the pointer.
	if undocumented < 5 {
		t.Fatalf("CONTROL failure: only %d root command(s) are missing from the command-reference table. "+
			"The pointer exists because the read commands are omitted; if that is no longer true the pointer "+
			"is stale and this guard is checking nothing", undocumented)
	}
}

// ---------------------------------------------------------------------------
// The two claims below shipped in this same README pass and were NOT pinned by
// the guards above: reverting either one restored published falsehood with the
// whole suite still green. Both are pinned as WHOLE NORMALISED STRINGS, for the
// reason homebrewMacOSOnlyClaim states — the artifact under test is prose, and a
// keyword guard is walkable by rewording.
// ---------------------------------------------------------------------------

// readmeCivitaiCommandTable is one markdown table whose header's first column is
// `Command` AND whose rows document `civitai …` invocations, recorded with the
// `##` section holding it and its whole header row — the table's SCHEMA.
//
// The `civitai` row requirement is what keeps `### Local dev loop`'s
// `| Command | Mode | What it does |` table (its rows are `npm run …`) out of
// the count: it is a table of npm scripts, not a table of CLI commands, and
// counting it would make "there are two command tables" true for the wrong
// reason.
type readmeCivitaiCommandTable struct {
	sectionSlug string
	section     string
	header      string
	rows        int
}

func readmeCivitaiCommandTables(t *testing.T, md string) []readmeCivitaiCommandTable {
	t.Helper()
	lines := strings.Split(stripFencedCode(md), "\n")
	var out []readmeCivitaiCommandTable
	section, slug := "", ""
	for i := 0; i < len(lines); i++ {
		if m := anyHeadingRe.FindStringSubmatch(lines[i]); m != nil {
			if len(m[1]) == 2 {
				section, slug = m[2], readmeAnchorSlug(m[2])
			}
			continue
		}
		if !strings.HasPrefix(lines[i], "| Command |") {
			continue
		}
		tbl := readmeCivitaiCommandTable{sectionSlug: slug, section: section, header: strings.TrimSpace(lines[i])}
		for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], "|"); j++ {
			cells := strings.Split(lines[j], "|")
			if len(cells) > 1 && strings.Contains(cells[1], "`civitai ") {
				tbl.rows++
			}
		}
		if tbl.rows > 0 {
			out = append(out, tbl)
		}
	}
	return out
}

// onlySectionSlug renders the section set for the reverse branch's message. It
// is a message helper, not a predicate: the branch is decided by len(sections).
func onlySectionSlug(sections map[string]readmeCivitaiCommandTable) string {
	slugs := make([]string, 0, len(sections))
	for s := range sections {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	return strings.Join(slugs, ", ")
}

// readmeTOCEntry returns the `## Contents` list item linking `anchor`, including
// its wrapped continuation lines (a TOC entry with a qualifier spans two lines,
// and reading only the first would make the qualifier invisible to every
// assertion below — the exact shape of a guard that reads as coverage while
// providing none).
func readmeTOCEntry(t *testing.T, md, anchor string) string {
	t.Helper()
	toc := readmeSectionByAnchor(t, md, "contents")
	lines := strings.Split(toc, "\n")
	start := -1
	for i, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "- [") || !strings.Contains(l, "("+anchor+")") {
			continue
		}
		if start >= 0 {
			t.Fatalf("the `## Contents` list has two entries linking %s (lines %q and %q) — "+
				"this guard would pin whichever came first and silently ignore the other", anchor, lines[start], l)
		}
		start = i
	}
	if start < 0 {
		t.Fatalf("the `## Contents` list has no entry linking %s. Either the TOC lost the entry this guard "+
			"pins, or the section walk is reading the wrong text", anchor)
	}
	end := start + 1
	for end < len(lines) {
		s := strings.TrimSpace(lines[end])
		if s == "" || strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") || strings.HasPrefix(s, "**") {
			break
		}
		end++
	}
	return strings.Join(lines[start:end], "\n")
}

// tocCommandReferenceEntry is the `## Contents` entry for `## Command
// reference`, pinned whole and already normalised (flattenWS form).
const tocCommandReferenceEntry = "- [Command reference](#command-reference) — one table of the authoring & " +
	"account commands (the public-API reads have their own)"

// tocCommandReferenceRetracted is the claim that shipped in that slot and was
// false: the section's table has never covered the public-API read commands.
const tocCommandReferenceRetracted = "every command, one table"

// TestREADMETOCCommandReferenceEntryNamesTheTableSplit pins the Contents entry
// for `## Command reference` against a fact derived from the document itself:
// there is MORE THAN ONE `civitai …` command table, and they sit in DIFFERENT
// `##` sections.
//
// 🔴 THE SECTION COUNT IS THE WHOLE DERIVATION, and the schema count is
// reporting detail only. The entry's claim is "the public-API reads have their
// own [table]", which is true exactly while a `civitai …` command table exists
// somewhere OTHER than `## Command reference` — a question about WHERE the
// tables are, not about what columns they carry. The first version of this guard
// gated the reverse branch on `len(sections) < 2 || len(schemas) < 2`, so
// collapsing the read table's header from `| Command | What it does | Notable
// flags |` to `| Command | What it does |` — pure formatting, both tables still
// present in two sections — fired the reverse branch and told the reader "the
// tables were merged … Re-derive it". Following that instruction deletes a
// qualifier that is still true and republishes the defect the entry was written
// to fix.
//
// 🔴 IT FAILS IN BOTH DIRECTIONS.
//   - The tables are split (today): the entry must not tell a reader the section
//     holds "every command, one table", and must carry the qualifier naming the
//     split. The TOC is the first thing a reader meets, so a wrong entry here
//     sends them to the wrong section before any other prose can correct it.
//   - The tables are merged into one (someone folded the read commands into the
//     reference table): the split qualifier must be REMOVED. That is the silent
//     direction — an entry promising a second table that no longer exists.
func TestREADMETOCCommandReferenceEntryNamesTheTableSplit(t *testing.T) {
	md := readREADME(t)
	tables := readmeCivitaiCommandTables(t, md)

	// POSITIVE CONTROL ON THE DERIVATION. An extractor that found nothing — a
	// changed header spelling, a fence-stripping bug — would take the "merged"
	// branch below and report a serene pass on a document it never read.
	if len(tables) == 0 {
		t.Fatalf("CONTROL failure: found no `| Command | … |` table with any `civitai …` row in README.md. " +
			"The table extractor is reading the wrong text, so every verdict below is about nothing")
	}
	sections := map[string]readmeCivitaiCommandTable{}
	schemas := map[string]bool{}
	for _, tb := range tables {
		sections[tb.sectionSlug] = tb
		schemas[tb.header] = true
	}
	if _, ok := sections["command-reference"]; !ok {
		var got []string
		for s := range sections {
			got = append(got, s)
		}
		sort.Strings(got)
		t.Fatalf("CONTROL failure: no `civitai …` command table was found under `## Command reference` "+
			"(sections with one: %v). The entry this guard pins describes that table; without it the "+
			"guard is checking a claim about a table it cannot see", got)
	}

	entry := readmeTOCEntry(t, md, "#command-reference")
	flat := flattenWS(entry)

	if len(sections) < 2 {
		// Reverse direction: every `civitai …` command table now lives in ONE
		// section, so there is no second table for the entry to point at.
		if strings.Contains(flat, "have their own") {
			t.Errorf("README.md now has %d `civitai …` command table(s), all inside the single `##` section "+
				"%q, but the `## Contents` entry still says the public-API reads \"have their own\":\n  %s\n\n"+
				"There is no longer a command table outside `## Command reference`, so the entry promises a "+
				"second table a reader cannot find. Re-derive it.\n\n"+
				"(If the tables are still in two sections and you are reading this, the derivation is wrong, "+
				"not the prose — do NOT delete the qualifier.)",
				len(tables), onlySectionSlug(sections), flat)
		}
		return
	}

	// Forward direction: the tables really are split.
	if regexp.MustCompile(`(?i)every command,?\s+one table`).MatchString(flat) {
		var where []string
		for slug, tb := range sections {
			where = append(where, slug+" "+tb.header)
		}
		sort.Strings(where)
		t.Errorf("the `## Contents` entry for `## Command reference` claims %q:\n  %s\n\n"+
			"It is false. README.md carries %d `civitai …` command tables, with %d different schemas, in "+
			"%d different sections:\n    %s\n\nThe Contents list is the first thing a reader meets; an entry "+
			"promising one complete table sends anyone looking for `civitai models` or `civitai download` to "+
			"a section that does not document them.",
			tocCommandReferenceRetracted, flat, len(tables), len(schemas), len(sections), strings.Join(where, "\n    "))
	}
	if flat != tocCommandReferenceEntry {
		t.Errorf("the `## Contents` entry for `## Command reference` is not the pinned statement.\n\n"+
			"want (normalised): %s\n\ngot  (normalised): %s\n\n"+
			"It is pinned WHOLE rather than by keyword because the artifact is prose: \"Command reference\", "+
			"\"table\" and \"commands\" all appear just as readily in an entry that re-claims completeness. "+
			"If the wording is genuinely being improved, change the constant and this message together — and "+
			"re-run the revert proof, which is what makes the new string a claim rather than a restatement.",
			tocCommandReferenceEntry, flat)
	}
}

// configPrecedenceClaim is the README's per-setting precedence block, pinned as
// one WHOLE normalised string. Every bullet is a separate contract an operator
// acts on, and three of the four are counter-intuitive, so the block is pinned
// entire rather than bullet by bullet: a reword that drops one bullet reads as a
// tidy-up and is a silent loss of the only place that behaviour is written down.
const configPrecedenceClaim = "🔴 **Precedence is decided per setting; no one rule covers all four flags.** " +
	"Only the first of them is a plain flag-beats-environment override: " +
	"- **`--tunnel-endpoint`** (`app dev-tunnel`) wins — `CIVITAI_DEV_TUNNEL_ENDPOINT` is read only when " +
	"the flag is empty. " +
	"- **`--token`** is no override at all: it is a `civitai login` flag that **writes** the key into the " +
	"config file, and `CIVITAI_TOKEN` then beats that file on every command — including the key " +
	"`login --token` just stored. For a one-off key, set `CIVITAI_TOKEN` for that invocation. " +
	"- **The colour flags** invert it — *off beats on* ([Global flags](#global-flags)), so `--color` " +
	"**loses** to a `NO_COLOR` / `CIVITAI_NO_COLOR` in the environment. " +
	"- **`--no-update-check`** is OR'd with `CIVITAI_NO_UPDATE_CHECK`: either one disables the check, and " +
	"no flag re-enables it once the variable is set."

// retractedFlagWinsClaim is the single sentence the block above replaced. It
// was published for months, it is wrong for three of the four flags it covered,
// and it is the one an operator acts on — so its ABSENCE is asserted separately
// from the new block's presence. (Restoring it elsewhere in the document would
// be just as wrong, so the check is over the whole README.)
const retractedFlagWinsClaim = "the flag wins over the environment"

// mentionDelims are the characters that, when they bracket an occurrence of
// retractedFlagWinsClaim, mark it as a MENTION of the claim rather than an
// ASSERTION of it.
var mentionDelims = map[rune]bool{'"': true, '“': true, '”': true, '\'': true, '`': true, '‘': true, '’': true}

// flagWinsRe matches retractedFlagWinsClaim case-insensitively. It is a regexp
// rather than a strings.Index over a lowercased copy so that the match indices
// address the ORIGINAL string — see readmeAssertsFlagWinsRule.
var flagWinsRe = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(retractedFlagWinsClaim))

// readmeAssertsFlagWinsRule reports whether `flat` — a flattenWS-normalised
// document — STATES the retracted rule, and returns the surrounding text of the
// first such occurrence.
//
// 🔴 TWO BUGS IN THE PREDICATE THIS REPLACES, pulling in OPPOSITE directions.
//
// Too LOOSE: it ran `strings.Contains` on the RAW README while every other
// whole-string pin in this file normalises first, so re-inserting "Where a
// setting also has a flag, the flag wins over the\nenvironment." — the published
// sentence, merely re-wrapped — restored the falsehood with the whole suite
// green. Measured. Hence flattenWS, and hence case-insensitivity: the same
// sentence opening a paragraph is capitalised and would have walked past too.
//
// Too STRICT: commit ef06edf legitimately wrote `"the flag wins over the
// environment" was wrong for three of the four flags it covered` — a correct
// NEGATION that has to quote the claim in order to retract it, and exactly the
// sentence a future editor might want in the document itself. A ban on the bare
// substring forbids the truth alongside the lie.
//
// 🔴 WHICH REPAIR, AND WHY. The brief offered two: pin the assertion (ban the
// retracted SENTENCE whole) or scope the ban to a non-negated context. Pinning
// the whole sentence was rejected: this file's own thesis is that a prose guard
// on a fixed string is walkable by rewording, and "the flag wins over the
// environment" is the CLAIM — republished in any other carrier sentence it is
// just as false, so a sentence-shaped pin would be narrower than the hazard.
//
// So the ban is scoped, and scoped STRUCTURALLY rather than by negation
// keywords: an occurrence bracketed by quotation delimiters is a mention (the
// document is talking ABOUT the sentence), anything else is an assertion (the
// document is telling the reader the rule). That is a property of two adjacent
// characters, not a keyword search over prose, and it is validated in both
// directions by TestReadmeAssertsFlagWinsRulePredicate — which is the point: the
// instrument gets its own negative and positive controls before its verdict is
// read. It does admit a contrived bypass (quoting the claim and then asserting
// it anyway); a reworded bare restatement, the thing that actually happens, is
// caught, and the alternative admitted a TRUE sentence being rejected.
func readmeAssertsFlagWinsRule(flat string) (bool, string) {
	// 🔴 The case-insensitive match is done with a regexp over `flat` ITSELF, not
	// by searching a strings.ToLower COPY. Measured: lowercasing the README
	// shifts byte offsets — Unicode has runes whose lowercase encodes to a
	// different number of bytes — so indices taken from the copy pointed a few
	// bytes off in the original, and the quotation delimiters this function reads
	// were simply the wrong characters. A correctly-quoted retraction was scored
	// as an assertion, which is the false-positive direction this whole exemption
	// exists to remove. Caught by the F4ctl control in the revert matrix, not by
	// the hand-written fixtures — they are pure ASCII and cannot drift.
	for _, loc := range flagWinsRe.FindAllStringIndex(flat, -1) {
		i, end := loc[0], loc[1]
		before, _ := utf8.DecodeLastRuneInString(flat[:i])
		after, _ := utf8.DecodeRuneInString(flat[end:])
		if mentionDelims[before] && mentionDelims[after] {
			continue // quoted: the document is retracting the claim, not making it.
		}
		lo := max(0, i-90)
		hi := min(len(flat), end+90)
		// The window is a byte slice of a normalised document, so it can land
		// mid-rune; the caller only ever prints it.
		return true, strings.ToValidUTF8(flat[lo:hi], "")
	}
	return false, ""
}

// TestREADMEConfigPrecedenceIsPerSetting ties the `## Configuration` precedence
// block to the four independent mechanisms that decide it. Each leg reads the
// real production rule, so the prose cannot drift into agreeing with code that
// no longer behaves that way:
//
//   - `--tunnel-endpoint`: internal/cmd/app_dev_tunnel.go — flag first, env only
//     when the flag is empty. The one plain flag-beats-environment case.
//   - `--token`: internal/config/config.go binds `CIVITAI_TOKEN` over `keyToken`,
//     which is the SAME key `login.go`'s `cfg.SetToken(token)` writes. That seam
//     is the whole claim — neither file is surprising alone.
//   - colour: internal/ui/ui.go resolves force-OFF before force-ON, so `--color`
//     loses to `NO_COLOR`. Exercised through ui.Configure, not by reading source.
//   - update check: internal/cmd/update_check.go ORs the flag with the variable,
//     so no flag re-enables it. `noFlag` is a bool, so both values ARE the whole
//     input space and the "no flag re-enables it" claim is proven, not sampled.
func TestREADMEConfigPrecedenceIsPerSetting(t *testing.T) {
	root := repoRootDir(t)
	readSrc := func(parts ...string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(parts...), err)
		}
		return string(b)
	}

	// --- Leg 1: `--tunnel-endpoint` wins. ------------------------------------
	tunnelSrc := flattenWS(readSrc("internal", "cmd", "app_dev_tunnel.go"))
	const tunnelOrder = `ep := strings.TrimSpace(endpoint) if ep == "" { ep = strings.TrimSpace(os.Getenv(devTunnelEndpointEnv)) }`
	if !strings.Contains(tunnelSrc, tunnelOrder) {
		t.Errorf("internal/cmd/app_dev_tunnel.go no longer resolves the endpoint as `%s`.\n\n"+
			"The README's first bullet is the ONLY plain flag-beats-environment case in the table, and it is "+
			"that ordering. If the resolution changed, the bullet is now wrong — re-derive both.", tunnelOrder)
	}

	// --- Leg 2: `--token` writes the key `CIVITAI_TOKEN` then overrides. ------
	// 2a. The SEAM, as a ledger: the key login writes is the key the env binds.
	configSrc := readSrc("internal", "config", "config.go")
	for _, want := range []string{
		`v.BindEnv(keyToken, "CIVITAI_TOKEN")`,
		`func (c *Config) SetToken(token string) error {`,
		`c.v.Set(keyToken, token)`,
		`if t := c.v.GetString(keyToken); t != ""`,
	} {
		if !strings.Contains(configSrc, want) {
			t.Errorf("internal/config/config.go no longer contains %s. The README says `CIVITAI_TOKEN` beats "+
				"the config file \"including the key `login --token` just stored\" — a claim that holds only "+
				"while the env binds over the SAME key SetToken writes and Token() reads. Re-derive both.", want)
		}
	}
	if !strings.Contains(readSrc("internal", "cmd", "login.go"), "cfg.SetToken(token)") {
		t.Error("internal/cmd/login.go no longer calls cfg.SetToken(token), so `login --token` may no longer " +
			"write the key the README says CIVITAI_TOKEN overrides. Re-derive the bullet against the new writer.")
	}

	// 2b. BEHAVIOURAL, through the same viper mechanism config.Load builds: a
	// bound env var beats a value present in the config file.
	vp := viper.New()
	vp.SetConfigType("yaml")
	if err := vp.ReadConfig(strings.NewReader("token: from-login-flag\n")); err != nil {
		t.Fatalf("read in-memory config: %v", err)
	}
	if err := vp.BindEnv("token", "CIVITAI_TOKEN"); err != nil {
		t.Fatalf(`BindEnv("token", "CIVITAI_TOKEN"): %v`, err)
	}
	t.Setenv("CIVITAI_TOKEN", "placeholder-so-t-Setenv-restores-it")
	// POSITIVE CONTROL: with the variable unset the FILE value must win. Without
	// this, an env read that returned the file value for some unrelated reason
	// would look like the override working.
	os.Unsetenv("CIVITAI_TOKEN")
	if got := vp.GetString("token"); got != "from-login-flag" {
		t.Fatalf("CONTROL failure: with CIVITAI_TOKEN unset the config-file token resolves to %q, want "+
			"\"from-login-flag\" — the in-memory config did not load, so the override assertion below "+
			"would be comparing nothing", got)
	}
	t.Setenv("CIVITAI_TOKEN", "from-environment")
	if got := vp.GetString("token"); got != "from-environment" {
		t.Errorf("CIVITAI_TOKEN does not override the config file's `token` key (got %q). The README tells "+
			"operators to set CIVITAI_TOKEN for a one-off key precisely because it beats what "+
			"`login --token` stored; if that stopped being true the bullet is wrong.", got)
	}

	// --- Leg 3: the colour flags invert it — `--color` loses to NO_COLOR. -----
	t.Cleanup(func() { ui.Configure(ui.Options{Writer: io.Discard}) })
	t.Setenv("CLICOLOR_FORCE", "placeholder")
	os.Unsetenv("CLICOLOR_FORCE")
	t.Setenv("NO_COLOR", "placeholder")
	os.Unsetenv("NO_COLOR")
	// POSITIVE CONTROL: --color alone really does force colour on, so the
	// assertion below cannot pass because colour was off for some other reason.
	ui.Configure(ui.Options{ForceColor: true, Writer: io.Discard})
	if !ui.EnabledFor(io.Discard) {
		t.Fatalf("CONTROL failure: --color (ui.Options{ForceColor: true}) does not force colour on with no " +
			"NO_COLOR set. The \"--color loses to NO_COLOR\" assertion below would then pass for the wrong " +
			"reason — colour would be off either way")
	}
	t.Setenv("NO_COLOR", "1")
	ui.Configure(ui.Options{ForceColor: true, Writer: io.Discard})
	if ui.EnabledFor(io.Discard) {
		t.Error("--color (ui.Options{ForceColor: true}) now WINS over NO_COLOR in the environment. The " +
			"README's third bullet says it loses — off beats on. Re-derive both.")
	}

	// --- Leg 4: --no-update-check is OR'd with the variable. -----------------
	if !strings.Contains(flattenWS(readSrc("internal", "cmd", "update_check.go")),
		`return noFlag || os.Getenv("CIVITAI_NO_UPDATE_CHECK") != ""`) {
		t.Error(`internal/cmd/update_check.go no longer computes updateCheckDisabled as ` +
			`"noFlag || os.Getenv(\"CIVITAI_NO_UPDATE_CHECK\") != \"\"". The README's fourth bullet is that OR ` +
			`— and the "no flag re-enables it" half is a property of the OR specifically. Re-derive both.`)
	}
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "placeholder")
	os.Unsetenv("CIVITAI_NO_UPDATE_CHECK")
	// POSITIVE CONTROL: with the variable unset the flag decides, both ways.
	if updateCheckDisabled(false) {
		t.Fatal("CONTROL failure: the update check reports itself disabled with neither the flag nor " +
			"CIVITAI_NO_UPDATE_CHECK set — the env-set assertions below would pass without the variable " +
			"doing anything")
	}
	if !updateCheckDisabled(true) {
		t.Fatal("CONTROL failure: --no-update-check alone does not disable the update check")
	}
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	// `noFlag` is a bool, so these two cases are the ENTIRE input space: this is
	// a proof that no flag value re-enables the check, not a sample of one.
	for _, noFlag := range []bool{false, true} {
		if !updateCheckDisabled(noFlag) {
			t.Errorf("CIVITAI_NO_UPDATE_CHECK=1 with --no-update-check=%v leaves the update check ENABLED. "+
				"The README says either one disables it and no flag re-enables it.", noFlag)
		}
	}

	// --- Leg 5: the document. ------------------------------------------------
	md := readREADME(t)
	if asserted, where := readmeAssertsFlagWinsRule(flattenWS(md)); asserted {
		t.Errorf("README.md states the retracted claim %q, unquoted:\n  …%s…\n\n"+
			"It is true of `--tunnel-endpoint` and false of the other three flags in the same table: "+
			"`--token` is not a per-command override at all, `--color` LOSES to NO_COLOR, and "+
			"`--no-update-check` is OR'd with its variable. An operator who reads that sentence and sets "+
			"a flag to beat the environment gets the environment.\n\n"+
			"Quoting the claim in order to REFUTE it is allowed and is not what fired here — see "+
			"readmeAssertsFlagWinsRule.", retractedFlagWinsClaim, where)
	}
	body := readmeSectionByAnchor(t, md, "configuration")
	if len(strings.TrimSpace(body)) < 800 {
		t.Fatalf("CONTROL failure: the README's `## Configuration` section is only %d byte(s) long — "+
			"the extractor is reading the wrong block", len(strings.TrimSpace(body)))
	}
	if !strings.Contains(flattenWS(body), configPrecedenceClaim) {
		t.Errorf("the README's `## Configuration` section does not carry the per-setting precedence block.\n\n"+
			"want (normalised): %s\n\ngot:\n%s\n\n"+
			"It is pinned WHOLE, not by keyword: the words \"flag\", \"environment\" and every variable name "+
			"appear just as readily in a sentence that re-asserts a single blanket rule. Each bullet is a "+
			"separate contract an operator acts on and three of the four are counter-intuitive, so dropping "+
			"one is a silent loss of the only place that behaviour is written down. If the wording is being "+
			"improved, change the constant and re-run the revert proof.",
			configPrecedenceClaim, flattenWS(body))
	}
}

// TestReadmeAssertsFlagWinsRulePredicate validates the INSTRUMENT before its
// verdict above is read. readmeAssertsFlagWinsRule is the only place in this
// file where a ban on published prose is SCOPED rather than absolute, so "it
// returned false on today's README" is indistinguishable from "it can never
// return true" until both controls have been watched to work.
//
// Negative control: can it go red at all? Every `wantAsserted: true` case is a
// document it MUST reject, including the exact re-wrap that walked past the
// predicate this replaces (measured: suite green, claim republished).
//
// Positive control on the exemption: can a true sentence survive? The quoted
// cases are the shape commit ef06edf wrote — the claim mentioned in order to be
// retracted — which the old substring ban would have rejected.
func TestReadmeAssertsFlagWinsRulePredicate(t *testing.T) {
	cases := []struct {
		name         string
		doc          string
		wantAsserted bool
	}{
		{
			name:         "the published sentence, verbatim",
			doc:          "Where a setting also has a flag — `--token`, `--tunnel-endpoint`, and the colour and update-check flags — the flag wins over the environment. See",
			wantAsserted: true,
		},
		{
			name: "the same sentence re-wrapped — the measured walk-past",
			// flattenWS is what the caller applies; this is its input shape.
			doc:          flattenWS("Where a setting also has a flag, the flag wins over the\nenvironment.\n\nEverything in this table"),
			wantAsserted: true,
		},
		{
			name:         "capitalised at the start of a sentence",
			doc:          "The flag wins over the environment, always.",
			wantAsserted: true,
		},
		{
			name:         "quoted and refuted — ef06edf's own wording",
			doc:          `"the flag wins over the environment" was wrong for three of the four flags it covered`,
			wantAsserted: false,
		},
		{
			name:         "quoted inside a negation",
			doc:          `There is no single "the flag wins over the environment" rule; precedence is per setting.`,
			wantAsserted: false,
		},
		{
			name:         "mentioned in typographic quotes",
			doc:          "The README used to say “the flag wins over the environment”, which it no longer does.",
			wantAsserted: false,
		},
		{
			name:         "mentioned in backticks",
			doc:          "The retracted `the flag wins over the environment` rule is gone.",
			wantAsserted: false,
		},
		{
			name:         "absent entirely — today's README",
			doc:          "Precedence is decided per setting; no one rule covers all four flags.",
			wantAsserted: false,
		},
		{
			// \U0001f534 REGRESSION FIXTURE, and every ASCII case above is blind to
			// it. The first implementation searched a strings.ToLower COPY and used
			// the indices it got back to read delimiters out of the ORIGINAL.
			// U+212A KELVIN SIGN lowercases to "k" \u2014 3 bytes to 1 \u2014 so any such
			// rune earlier in the document slides every later offset, and the
			// quotation marks the exemption depends on are then read as some other
			// character entirely. A correct retraction scored as an assertion.
			// Measured on the real README, by the seam control below.
			name:         "quoted, behind a rune whose lowercase is SHORTER (index drift)",
			doc:          "Temperature 300\u212a. There is no single \"the flag wins over the environment\" rule.",
			wantAsserted: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, where := readmeAssertsFlagWinsRule(tc.doc)
			if got != tc.wantAsserted {
				t.Fatalf("readmeAssertsFlagWinsRule(%q) = %v, want %v (context: %q).\n\n"+
					"This predicate decides whether a published falsehood is allowed to stand. If the "+
					"expectation here is being changed, say which of the two failure directions "+
					"(too loose: a reworded restatement slips through; too strict: a correct retraction is "+
					"rejected) the change accepts.", tc.doc, got, tc.wantAsserted, where)
			}
			if got && where == "" {
				t.Error("the predicate reported an assertion but returned no context — the failure message " +
					"above it would name no location, which is how a guard gets deleted instead of obeyed")
			}
		})
	}

	// 🔴 SEAM CONTROL. Every case above is a hand-written fixture; none of them
	// proves the predicate is wired to the DOCUMENT. Feed it today's real README
	// with the retracted sentence spliced back in, exactly as the mutation did,
	// and it must fire.
	md := flattenWS(readREADME(t))
	if asserted, _ := readmeAssertsFlagWinsRule(md); asserted {
		t.Fatal("CONTROL failure: today's README already asserts the retracted claim, so the splice " +
			"below cannot show anything — fix the README first")
	}
	spliced := md + " Where a setting also has a flag, the flag wins over the environment."
	if asserted, where := readmeAssertsFlagWinsRule(spliced); !asserted {
		t.Errorf("the retracted sentence spliced into the REAL README was not detected (context: %q). "+
			"The fixtures above pass, so the predicate works on strings it was written against and not "+
			"on the artifact it guards.", where)
	}
	// \U0001f534 AND THE OTHER DIRECTION, on the same artifact. A retraction that
	// QUOTES the claim must survive being spliced into the real document, not
	// only into an ASCII fixture \u2014 this is the case that caught the index-drift
	// bug the fixture above now pins, and it is the difference between a guard
	// that forbids a lie and one that also forbids the truth.
	quoted := md + ` There is no single "the flag wins over the environment" rule.`
	if asserted, where := readmeAssertsFlagWinsRule(quoted); asserted {
		t.Errorf("a QUOTED retraction spliced into the REAL README was scored as an assertion "+
			"(context: %q). The ASCII fixtures above accept it, so the predicate disagrees with itself "+
			"depending on what precedes the match \u2014 which is an indexing bug, not a policy.", where)
	}
}
