package cmd

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// These guards tie README claims about INSTALLING and CONFIGURING the CLI to the
// artifacts that decide them: `.goreleaser.yaml` (what the release actually
// publishes), `internal/cmd/upgrade.go` (what the self-updater asks for) and
// `strconv.ParseBool` (what the CIVITAI_* colour variables accept).
//
// Both defects they pin shipped for months in the reassuring direction — the
// README promised a platform or a value that does not work, and nothing was red.
// A drift guard is the only thing that notices, because neither claim can be
// falsified by running the CLI on the machine CI runs on.
//
// A THIRD guard used to live here: it demanded the README carry a caveat saying
// `civitai upgrade` cannot work on Windows. That caveat is gone because the bug
// it described is FIXED (#613) — upgrade.go now derives the archive extension
// per GOOS. The durable claim is the extension PARITY between the two files, and
// it is pinned by TestUpgradeArchiveFormatsMatchTheReleaseConfig in
// upgrade_test.go, where it can drive the real code path instead of the prose.

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
// The guard has TWO legs. It had a third, over the README's `## Global flags`
// section; that section is now a pointer to the published guide, and the leg was
// RETIRED rather than re-pointed — the argument is recorded where it used to be,
// at the end of this function.
//  1. BEHAVIOURAL — the same viper binding root.go uses, exercised per value.
//  2. LEDGER — root.go must still bind and read those keys that way; a switch to
//     GetString/envSet semantics reddens this and forces the docs to be redone.
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
	// per-value loops below are iterating over an empty set.
	//
	// 🔴 IT NO LONGER GUARDS A DOCUMENT ASSERTION — IT IS THE LAST COUPLING TO THE
	// PUBLISHED LIST. This comment used to say "or the document assertions below are
	// checking an empty set"; leg 3 was the only document assertion and it was
	// deleted (see the note at the end of this function). So what this `!= 12` now
	// does is fail the build if Go's boolean grammar or boolValueCandidates ever
	// stops yielding exactly the twelve spellings the published output contract
	// enumerates — which is the only thing left in this repository tying that page's
	// number to the code. Do not weaken it to a floor: a control described as
	// protecting something that no longer exists is the shape that gets deleted as
	// vacuous, which is why it is re-stated here rather than left stale.
	if len(accepted) != 12 {
		t.Fatalf("CONTROL failure: strconv.ParseBool accepts %d of the %d candidate(s) (%v), want exactly 12. "+
			"The Go boolean grammar changed, or boolValueCandidates lost entries — re-derive the PUBLISHED list "+
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
				t.Errorf("%s=%q resolves to %v, want %v — the PUBLISHED value set is derived from "+
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
		t.Fatal("CIVITAI_NO_COLOR=yes now disables colour — the published output contract says it does nothing. Re-derive both.")
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
		t.Error("NO_COLOR=yes did not disable colour — the published output contract says any non-empty NO_COLOR does, " +
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
			"published output contract states it outright. If envSet grew a `v != \"0\"` clause (harmonising " +
			"it with envTrue, which legitimately has one for CLICOLOR_FORCE), that contract is wrong in two " +
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
			"the contrast the published output contract draws against NO_COLOR=0, where the same value DOES " +
			"disable. " +
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
			t.Errorf("internal/cmd/root.go no longer contains %s. The PUBLISHED value set is a claim "+
				"about GetBool (→ cast.ToBool → strconv.ParseBool); a different read (GetString, an envSet-style "+
				"presence test) accepts a DIFFERENT set of values and makes that prose wrong. Re-derive both.", want)
		}
	}

	// --- Leg 3 (the document) WAS HERE, AND IT WAS RETIRED RATHER THAN RE-POINTED.
	//
	// 🔴 IT WAS UNREACHABLE VIA BOTH TRIGGERS ITS OWN COMMENT CLAIMED. The comment
	// said it fires "when Go's boolean grammar or boolValueCandidates changes and
	// the published sentence has not been re-synced". Neither can reach it: both
	// inputs feed `accepted`, and the `len(accepted) != 12` CONTROL above is a
	// t.Fatalf ~150 lines earlier in this same function, so it always wins.
	// MEASURED: dropping "True" from boolValueCandidates dies at that control
	// ("accepts 11 of the 22 candidate(s) … want exactly 12") and leg 3's own
	// message never appears.
	//
	// Its ONLY reachable trigger was editing the vendored constant it read — which
	// is what the mutation that "proved" it worked actually mutated. That is
	// breakable, not reachable: RULES.md's unreachable-guards case, where a guard
	// dies to an earlier check while its own assertion never executes.
	//
	// 🔴 AND THE CONSTANT WAS NOT THE TRANSCRIPT ITS HEADER CLAIMED. Diffed
	// clause-by-clause against the raw page (curl + strip tags, per
	// claudedocs/readme-reduction-plan.md's "diff against the raw page, never a
	// summary of it"): two of its four clauses were the DELETED README's wording,
	// not the page's. The page says CIVITAI_NO_COLOR/CIVITAI_COLOR "are **bound
	// into the CLI's config layer** and read as booleans", and splits the
	// =yes/=1/=0 clause into three sentences ending "— unlike NO_COLOR=0, where the
	// same value disables it". So it asserted phrasing the publisher did not
	// choose — the very thing the old leg's own residual dropped
	// "not interchangeable" to avoid.
	//
	// Legs 1 and 2 pin the BEHAVIOUR (through the viper binding root.go uses) and
	// the MECHANISM (root.go must still BindEnv/GetBool those keys) against
	// internal/ui and root.go. They are untouched, and they are what made the
	// twelve-spelling claim true in the first place. The published page is
	// developer.civitai.com/site/guide/cli-output; nothing in this repository
	// asserts on its prose, and that is now stated rather than simulated.

}

// ---------------------------------------------------------------------------
// The two claims below shipped in this same README pass and were NOT pinned by
// the guards above: reverting either one restored published falsehood with the
// whole suite still green. Both are pinned as WHOLE NORMALISED STRINGS, for the
// reason homebrewMacOSOnlyClaim states — the artifact under test is prose, and a
// keyword guard is walkable by rewording.
// ---------------------------------------------------------------------------

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
	//
	// 🔴 THE BODY IS NOW ONE CALL, AND THAT IS THE POINT. This function and
	// readme_output_contract_test.go's assertsUnquotedClaim were two copies of the
	// same predicate over two byte-identical tables of quotation delimiters — the
	// exact duplication the second one's own comment warned about ("two tables of
	// quotation characters that disagree is how one of them starts admitting a
	// claim the other rejects") while shipping it. One rule, one place: the
	// mechanism lives there, mentionDelims lives here, and both callers get the
	// same answer by construction rather than by two maintainers agreeing.
	return assertsUnquotedClaim(flat, retractedFlagWinsClaim)
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
