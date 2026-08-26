package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// whoami_readme_block_test.go closes the SEAM between two surfaces that are
// each separately tested and were never tested TOGETHER: the rendering in
// whoami.go, and the literal rendered block README.md shows a reader.
//
// 🔴 THE UNIT TESTS CANNOT SEE THIS DEFECT AND NEITHER CAN THE README GUARDS.
// whoami_human_block_test.go pins what the command emits; the README guards in
// readme_troubleshooting_test.go pin symptom strings, anchors and attribution.
// Neither ever builds the combined state "the fenced block in README.md equals
// what the binary prints", so a shape change can ship with the docs showing the
// old one — which is exactly what the two-section split would have done. The
// guard pins the RELATIONSHIP, so it fails in either direction: reword the
// command and the README goes stale; edit the README block and it stops
// describing the command.
//
// It is also why the README block is a WHOLE stdout and not an excerpt. An
// excerpt cannot be compared, and the elision is invisible: before this guard
// the block stopped just above the can't-spend guidance the same invocation
// really appends, and nothing said so.

// readmeWhoAmIBlockRe captures every fenced block on the page that opens with
// `whoami`'s pinned first line. Both of `whoami`'s documented states use the
// same fixture identity, which is what makes them findable as a set.
var readmeWhoAmIBlockRe = regexp.MustCompile("(?s)```\\n(Logged in as zach \\(id 1\\) at https://civitai\\.com\\n.*?)```")

// TestREADMEWhoAmIBlocksMatchTheRealOutput drives the command for each state
// README.md renders and compares the WHOLE of stdout against the fenced block,
// byte for byte (only the base URL is substituted, since the README shows the
// production host and the test drives an httptest server).
func TestREADMEWhoAmIBlocksMatchTheRealOutput(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	blocks := readmeWhoAmIBlockRe.FindAllStringSubmatch(string(readme), -1)

	// Positive control on the extractor. A reformatted fence, a renamed fixture
	// user or a regex typo all look identical to "everything matched", and a
	// verdict over zero blocks is the reassuring-zero this repo keeps hitting.
	if len(blocks) != 2 {
		t.Fatalf("expected exactly 2 rendered `civitai whoami` blocks in README.md, extracted %d — "+
			"the extractor is reading the wrong text (pattern: %s). Both states are documented: the "+
			"full-scope key and the absent-mask degradation.", len(blocks), readmeWhoAmIBlockRe)
	}

	cases := []struct {
		name  string
		body  string
		block int
	}{{
		// UserRead | AIServicesWrite | BuzzRead = 98305. Spend-capable, so no
		// guidance block is appended and stdout is exactly the two sections.
		name:  "full-scope personal key",
		body:  `{"username":"zach","id":1,"tokenScope":98305,"subject":{"type":"apiKey","id":"k"}}`,
		block: 0,
	}, {
		name:  "oauth, absent mask (the degraded block)",
		body:  `{"username":"zach","id":1,"subject":{"type":"oauth","id":"a"}}`,
		block: 1,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupWhoAmI(t, tc.body)
			stdout, _, err := run(t, "whoami")
			if err != nil {
				t.Fatalf("whoami: %v", err)
			}
			got := strings.ReplaceAll(stdout, srv.URL, "https://civitai.com")
			want := blocks[tc.block][1]
			if got != want {
				t.Errorf("README.md's rendered block %d no longer matches what `civitai whoami` prints.\n"+
					"--- the command prints ---\n%s\n--- README.md shows ---\n%s", tc.block, got, want)
			}
		})
	}
}
