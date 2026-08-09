package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// listingMediaCmdRe matches a documented `civitai app listing set-icon|set-cover|
// add-screenshot <path>` invocation and captures the path argument.
//
// It is deliberately run over the RAW README, not the fenced-code-stripped copy
// the anchor guards use: every one of these invocations lives inside a ```bash or
// ```text block, so stripping fences would leave the guard matching nothing —
// which is indistinguishable from "the README is correct".
var listingMediaCmdRe = regexp.MustCompile(`civitai app listing (?:set-icon|set-cover|add-screenshot)\s+(\S+)`)

// TestREADMEListingMediaPathsExistInEveryScaffoldedProject is the seam guard
// between two surfaces that were each individually fine and broken together.
//
// The README's quickstart told authors to run
// `civitai app listing set-icon ./assets/icon.png` as step 7. No template
// created `assets/`. Measured on the binary built at 1eb4095, in a project
// freshly scaffolded by the CLI itself:
//
//	$ civitai app listing set-icon ./assets/icon.png
//	Error: no such file: ./assets/icon.png
//	exit=2
//
// Nothing was wrong with the command, and nothing was wrong with the scaffold.
// The defect lived in the relationship, so the guard has to pin the
// RELATIONSHIP: every relative path the README hands an author must have its
// parent directory present in a project the CLI actually scaffolds — for EVERY
// template, since an author picks one and the quickstart does not say which.
//
// 🔴 It asserts the DIRECTORY, never the file. The images themselves must not be
// scaffolded (a placeholder uploads cleanly to a public listing — see
// internal/scaffold/assets_dir_test.go and AGENTS.md item 25), so a guard
// demanding `assets/icon.png` would be a guard demanding the hazard.
func TestREADMEListingMediaPathsExistInEveryScaffoldedProject(t *testing.T) {
	matches := listingMediaCmdRe.FindAllStringSubmatch(readREADME(t), -1)

	// Positive control. A zero-match run is what a regex typo, a renamed
	// command or a wrong working directory all look like, and it would report a
	// serene pass over nothing.
	if len(matches) < 4 {
		t.Fatalf("found only %d `civitai app listing set-*` invocations in README.md, want >= 4 — "+
			"the extractor is reading the wrong text (pattern: %s)", len(matches), listingMediaCmdRe)
	}

	// Collect the distinct parent directories the README names. A path that is
	// obviously a stand-in (`<file>`, `$ICON`) is not a promise to the reader.
	dirs := map[string][]string{}
	for _, m := range matches {
		p := m[1]
		if strings.ContainsAny(p, "<>$*") || filepath.IsAbs(p) {
			continue
		}
		dir := filepath.Dir(filepath.Clean(p))
		if dir == "." {
			// The project root always exists; such a path makes no claim about
			// the scaffold. Recorded rather than silently dropped so the count
			// below stays honest.
			continue
		}
		dirs[dir] = append(dirs[dir], p)
	}
	if len(dirs) == 0 {
		t.Fatalf("every documented listing-media path resolved to the project root, so this guard checked nothing. "+
			"Either the README stopped naming a directory (in which case the quickstart no longer shows authors where artwork lives), "+
			"or the extractor is wrong. Paths seen: %v", matches)
	}

	var wantDirs []string
	for d := range dirs {
		wantDirs = append(wantDirs, d)
	}
	sort.Strings(wantDirs)

	for _, tmpl := range scaffold.AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "app")
			if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "app", Name: "App"}); err != nil {
				t.Fatalf("scaffold %s: %v", tmpl, err)
			}
			for _, dir := range wantDirs {
				info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(dir)))
				if err != nil || !info.IsDir() {
					t.Errorf("README documents %v, but template %q scaffolds no %q directory — "+
						"a reader who copies that command gets `Error: no such file` (exit 2). "+
						"Either scaffold the directory or stop naming the path.",
						dirs[dir], tmpl, dir)
				}
			}
		})
	}
}

// TestListingMediaShortsQuoteTheirOwnCap covers the surface #274's cap guard
// does not: `TestListingHelpQuotesTheEnforcedCaps` reads `c.Long` only, so a
// Short could quote a cap the command does not enforce and stay green.
//
// The Shorts carry a cap because they replaced "a square-ish image" / "a
// landscape hero image" (#270) — one-liners that stated a SHAPE the CLI does not
// check while omitting the format and size it does. Both are built by
// interpolating listingSourceRule(kind), so this asserts the interpolation picked
// the right kind, which is exactly the copy-paste #274 caught in the Long bodies.
//
// Scoped to icon and cover on purpose: their caps DIFFER (2.0 MB vs 4.0 MB), so
// "contains mine" and "does not contain the sibling's" are independent
// assertions. add-screenshot's Short carries no cap, and its cap string is
// byte-identical to the icon's today, so asserting on it would be #274's
// declared equivalent mutant — a guard that cannot fail.
func TestListingMediaShortsQuoteTheirOwnCap(t *testing.T) {
	root := NewRootCmd()
	listing, _, err := root.Find([]string{"app", "listing"})
	if err != nil {
		t.Fatalf("`app listing` is not in the command tree: %v", err)
	}

	cases := []struct {
		name    string
		kind    mediaKind
		foreign mediaKind
	}{
		{"set-icon", kindIcon, kindCover},
		{"set-cover", kindCover, kindIcon},
	}
	var checked int
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var short string
			for _, c := range listing.Commands() {
				if c.Name() == tc.name {
					short = c.Short
				}
			}
			if short == "" {
				t.Fatalf("`app listing %s` is not registered (or has no Short) — a rename must "+
					"update this test, not silently drop the row", tc.name)
			}
			checked++

			mine := humanBytes(int64(kindByteCap(tc.kind)))
			theirs := humanBytes(int64(kindByteCap(tc.foreign)))
			if mine == theirs {
				t.Fatalf("%s and %s render the same cap (%s), so this row cannot distinguish them — "+
					"pick a differently-capped sibling", tc.kind, tc.foreign, mine)
			}
			if !strings.Contains(short, mine) {
				t.Errorf("`app listing %s` Short (%q) does not quote the cap it enforces (%s) — "+
					"the one-liner in `app listing --help` is the only thing many readers see",
					tc.name, short, mine)
			}
			if strings.Contains(short, theirs) {
				t.Errorf("`app listing %s` Short (%q) quotes %s, the %s cap — "+
					"listingSourceRule was interpolated with the wrong kind",
					tc.name, short, theirs, tc.foreign)
			}
			if !strings.Contains(short, listingImageFormats) {
				t.Errorf("`app listing %s` Short (%q) does not state the accepted formats (%q)",
					tc.name, short, listingImageFormats)
			}
			// The regression itself: a bare shape adjective, stated as though the
			// CLI enforced it. Shape is the platform's and is attributed in Long.
			for _, banned := range []string{"square-ish", "landscape hero"} {
				if strings.Contains(short, banned) {
					t.Errorf("`app listing %s` Short (%q) is back to describing a SHAPE the CLI does "+
						"not check (%q). Shape is the platform's, validated at attach; the Short "+
						"states what the CLI enforces locally. See AGENTS.md item 25.",
						tc.name, short, banned)
				}
			}
		})
	}
	if checked != 2 {
		t.Fatalf("checked %d Shorts, want 2 — the command walk is wrong", checked)
	}
}

// readmeSectionSlice returns the README text of the section anchored at slug,
// raw (fences intact) so tables and sample output are visible to the caller.
func readmeSectionSlice(t *testing.T, md, heading string) string {
	t.Helper()
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatalf("README.md has no %q heading — the listing-media requirements have nowhere to live", strings.TrimSpace(heading))
	}
	body := md[i+len(heading):]
	// Bound at the next heading of the same or a higher level.
	for _, next := range []string{"\n## ", "\n### "} {
		if j := strings.Index(body, next); j >= 0 {
			body = body[:j]
		}
	}
	return body
}

// TestREADMEListingByteCapsMatchTheCLIConstants is a code↔prose drift guard.
//
// The dimension and aspect bounds in that section are PLATFORM constants the CLI
// deliberately does not vendor (AGENTS.md item 25), so nothing here can check
// them — they are guidance, and the server's rejection message is the authority.
// The BYTE CAPS are different: those are this CLI's own local gate, so the
// README quoting a number the binary does not enforce is a plain lie a test can
// catch. That is the half that is checkable, so it is the half that is checked.
func TestREADMEListingByteCapsMatchTheCLIConstants(t *testing.T) {
	section := readmeSectionSlice(t, readREADME(t), "\n### Listing media requirements\n")

	// Positive control on the extractor before reading anything out of it.
	if len(section) < 500 {
		t.Fatalf("the `Listing media requirements` section is only %d bytes — the section extractor is reading the wrong block:\n%s", len(section), section)
	}

	cases := []struct {
		kind string
		cap  int
	}{
		{"icon", maxIconBytes},
		{"cover", maxCoverBytes},
		{"screenshot", maxScreenshotBytes},
	}
	for _, tc := range cases {
		row := ""
		for _, line := range strings.Split(section, "\n") {
			if strings.HasPrefix(line, "| **"+tc.kind+"**") && strings.Contains(line, "MiB") {
				row = line
				break
			}
		}
		if row == "" {
			t.Errorf("no `| **%s** | … MiB …` row in the Listing media requirements section — "+
				"a reader cannot see the byte cap the CLI will reject their file with:\n%s", tc.kind, section)
			continue
		}
		want := fmt.Sprintf("%d MiB", tc.cap/(1024*1024))
		if !strings.Contains(row, want) {
			t.Errorf("README says %q for %s, but the CLI enforces %s (%d bytes). "+
				"The prose and the gate must agree — a reader sizing to the README should never be refused locally.",
				strings.TrimSpace(row), tc.kind, want, tc.cap)
		}
	}

	// The framing this section exists to establish: the dimension table is NOT a
	// local check. Losing that sentence is how the next reader concludes the CLI
	// is missing a validation and adds one.
	for _, want := range []string{"server-side", "platform"} {
		if !strings.Contains(section, want) {
			t.Errorf("the Listing media requirements section never says %q — the dimension bounds then read as rules the CLI enforces", want)
		}
	}
}
