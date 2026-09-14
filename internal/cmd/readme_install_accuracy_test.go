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
const homebrewNoFormulaClaim = "publishes no `brews:` (formula) stanza at all"

// TestREADMEHomebrewSectionMatchesTheReleaseConfig ties the README's Homebrew
// section to what the release pipeline actually publishes.
//
// `.goreleaser.yaml` carries `homebrew_casks:` and NO `brews:` stanza. A
// Homebrew *cask* is a macOS-only concept and the rendered cask names darwin
// archives only (see tools/caskcheck/testdata/drill-broken-cask.rb), so there is
// no Linux Homebrew install — yet the README advertised
// `### Homebrew (macOS / Linux)` and told Linux readers it was "quickest" for
// them.
//
// 🔴 IT FAILS IN BOTH DIRECTIONS.
//   - No `brews:` stanza (today): the heading must not name Linux, the Install
//     intro must not pitch Homebrew at Linux, and the section must carry the
//     refusal above.
//   - A `brews:` stanza appears (a real Linux formula is published): the refusal
//     must be REMOVED. That is the silent direction — a stale "Linux is not
//     supported" paragraph reads as a current rule and would send Linux users
//     away from an install that works.
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

	if hasBrews {
		// Reverse direction: a Linux formula now exists.
		if strings.Contains(flatBody, flattenWS(homebrewMacOSOnlyClaim)) {
			t.Errorf(".goreleaser.yaml now carries a top-level `brews:` stanza — a Homebrew FORMULA, which "+
				"does install on Linux — but README's `### %s` section still says %q.\n\n"+
				"Delete that refusal and re-advertise Linux. A stale 'not supported' paragraph is worse than "+
				"none: it reads as a current rule and steers Linux users away from an install that works.",
				heading, homebrewMacOSOnlyClaim)
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
	if regexp.MustCompile(`(?i)homebrew[^.]*\blinux\b`).MatchString(flattenWS(intro)) {
		t.Errorf("the `## Install` intro still pitches Homebrew at Linux:\n%s\n\n"+
			"Only a cask is published; there is no Linux Homebrew install.", flattenWS(intro))
	}
}

// windowsUpgradeCaveat is the README's Windows refusal for `civitai upgrade`,
// pinned whole for the same reason as homebrewMacOSOnlyClaim.
const windowsUpgradeCaveat = "🔴 **`civitai upgrade` does not work on Windows — use the manual path.**"

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
	upgradeSrc, err := os.ReadFile(filepath.Join(repoRootDir(t), "internal", "cmd", "upgrade.go"))
	if err != nil {
		t.Fatalf("read internal/cmd/upgrade.go: %v", err)
	}
	m := upgradeAssetNameRe.FindAllStringSubmatch(string(upgradeSrc), -1)
	if len(m) != 1 {
		t.Fatalf("CONTROL failure: found %d asset-name template(s) matching %s in internal/cmd/upgrade.go, want exactly 1. "+
			"Either the name is now built some other way (in which case re-derive this comparison rather than "+
			"deleting it) or this regex has stopped matching and every verdict below is vacuous",
			len(m), upgradeAssetNameRe)
	}
	cliExt := m[0][1]

	// A per-GOOS branch would make the single template above an incomplete read.
	if regexp.MustCompile(`runtime\.GOOS\s*==\s*"windows"`).MatchString(string(upgradeSrc)) {
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

	if cliExt == winExt {
		if hasCaveat {
			t.Errorf("internal/cmd/upgrade.go now asks for a .%s asset and .goreleaser.yaml publishes Windows as "+
				".%s — they AGREE, so `civitai upgrade` can find its Windows asset. Delete the README caveat %q.\n\n"+
				"A stale refusal tells Windows users to do by hand what the command now does for them.",
				cliExt, winExt, windowsUpgradeCaveat)
		}
		return
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
