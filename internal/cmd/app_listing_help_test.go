package cmd

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// listingHelpNodes returns every command under `civitai app listing`, keyed by
// its leaf name, discovered by WALKING the real tree rather than from a
// hardcoded list.
//
// Walking matters: a hardcoded list turns a renamed or dropped command into a
// silently skipped row, and a skipped row is indistinguishable from a passing
// one. Every test below therefore also asserts a count, so a walk that found
// nothing cannot report a serene pass.
func listingHelpNodes(t *testing.T) map[string]*cobra.Command {
	t.Helper()
	var listing *cobra.Command
	var find func(c *cobra.Command)
	find = func(c *cobra.Command) {
		if c.Name() == "listing" && c.Parent() != nil && c.Parent().Name() == "app" {
			listing = c
			return
		}
		for _, s := range c.Commands() {
			find(s)
		}
	}
	find(NewRootCmd())
	if listing == nil {
		t.Fatal("could not find `civitai app listing` in the command tree")
	}
	out := map[string]*cobra.Command{"": listing}
	for _, s := range listing.Commands() {
		out[s.Name()] = s
	}
	return out
}

// TestListingHelpQuotesTheEnforcedCaps pins the coupling between what `--help`
// PROMISES about a source file and what loadAndValidateImage actually ENFORCES.
//
// 🔴 THE POINT IS THE DIRECTION OF DERIVATION. The expected strings here are
// computed from maxIconBytes/maxCoverBytes/maxScreenshotBytes through the same
// humanBytes the refusal message uses — so moving a cap constant without moving
// the help reddens this test whether the help body computes its number or has a
// stale literal typed into it. A test that merely asserted "the help mentions a
// size" would pass over both.
//
// The cross-kind assertions are separate and are not redundant with the
// per-kind ones: icon and screenshot share a cap (2 MiB), so "set-cover quotes
// 4.0 MB" is satisfied by a body that ALSO quotes the icon's — the shape a
// copy-paste between the two bodies produces.
func TestListingHelpQuotesTheEnforcedCaps(t *testing.T) {
	nodes := listingHelpNodes(t)

	cases := []struct {
		cmd     string
		kind    mediaKind
		notCaps []string // caps that must NOT appear in this body
	}{
		{"set-icon", kindIcon, []string{humanBytes(maxCoverBytes)}},
		{"set-cover", kindCover, []string{humanBytes(maxIconBytes)}},
		{"add-screenshot", kindScreenshot, []string{humanBytes(maxCoverBytes)}},
	}
	if len(cases) != 3 {
		t.Fatalf("expected 3 media commands under test, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			c, ok := nodes[tc.cmd]
			if !ok {
				t.Fatalf("`app listing %s` is not in the command tree — a rename must "+
					"update this test, not silently drop the row", tc.cmd)
			}
			want := humanBytes(int64(kindByteCap(tc.kind)))
			if !strings.Contains(c.Long, want) {
				t.Errorf("`app listing %s` Long does not quote the cap it enforces (%s).\n"+
					"The help must state the SAME number loadAndValidateImage refuses on.\nLong:\n%s",
					tc.cmd, want, c.Long)
			}
			if !strings.Contains(c.Long, listingImageFormats) {
				t.Errorf("`app listing %s` Long does not state the accepted formats (%q)",
					tc.cmd, listingImageFormats)
			}
			for _, bad := range tc.notCaps {
				if bad == want {
					continue // shared cap; nothing to distinguish
				}
				if strings.Contains(c.Long, bad) {
					t.Errorf("`app listing %s` Long quotes %s, which is another kind's cap — "+
						"this is what a copy-paste between two media bodies looks like", tc.cmd, bad)
				}
			}
		})
	}

	t.Run("group", func(t *testing.T) {
		parent := nodes[""]
		for _, want := range []string{
			humanBytes(maxIconBytes),
			humanBytes(maxCoverBytes),
			humanBytes(maxScreenshotBytes),
			listingImageFormats,
		} {
			if !strings.Contains(parent.Long, want) {
				t.Errorf("`app listing` group Long does not state %q — the group page is where a "+
					"reader lands first, so all three caps belong there", want)
			}
		}
	})
}

// TestListingHelpBodiesAreComplete is the pilot's own contract: every node in
// this group carries an authored Long AND an Example.
//
// Three of these commands (set-icon, set-cover, add-screenshot) shipped with
// NEITHER — cobra falls back to Short, so `--help` printed a single 41-character
// line and the docs generator, which parses that same help text, published a
// single line too. This test is what makes that state fail instead of pass.
//
// The floor is asserted so a walk that found nothing cannot read as clean.
func TestListingHelpBodiesAreComplete(t *testing.T) {
	nodes := listingHelpNodes(t)
	const wantNodes = 7 // the group + its six subcommands
	if len(nodes) != wantNodes {
		t.Fatalf("walked %d nodes under `app listing`, want %d — adding a command means "+
			"documenting it, so update this floor deliberately", len(nodes), wantNodes)
	}
	for name, c := range nodes {
		label := "app listing " + name
		t.Run(strings.TrimSpace(name), func(t *testing.T) {
			if strings.TrimSpace(c.Long) == "" {
				t.Errorf("%s has no Long — `--help` will print only its Short (%q), and the docs "+
					"generator publishes exactly what `--help` prints", label, c.Short)
			}
			if strings.TrimSpace(c.Example) == "" {
				t.Errorf("%s has no Example", label)
			}
			if strings.TrimSpace(c.Short) == "" {
				t.Errorf("%s has no Short", label)
			}
		})
	}
}

// helpBodyBudget is the ceiling on a single leaf's rendered Long.
//
// It exists because the failure mode of "move the README's prose into Long" is
// NOT missing content — it is a `--help` nobody reads. The number is a budget,
// not a measurement: 1400 characters is roughly a terminal screen before the
// Usage/Flags blocks are added, and every body in this group is comfortably
// under it. Raise it deliberately, with a reason, rather than to make a new
// paragraph fit.
const helpBodyBudget = 1400

func TestListingHelpStaysWithinTheBudget(t *testing.T) {
	nodes := listingHelpNodes(t)
	if len(nodes) == 0 {
		t.Fatal("no nodes walked")
	}
	var checked int
	for name, c := range nodes {
		body := strings.TrimSpace(c.Long)
		if body == "" {
			continue // TestListingHelpBodiesAreComplete owns that failure
		}
		checked++
		// 🔴 RUNES, NOT BYTES. These bodies are full of em-dashes (3 bytes, one
		// column), so a byte count reports a 79-column line as 81 and fails prose
		// that is fine. Measured: the first version of this guard reddened four
		// PRE-EXISTING bodies that render inside 80 columns.
		if n := utf8.RuneCountInString(body); n > helpBodyBudget {
			t.Errorf("`app listing %s` Long is %d chars, over the %d budget — "+
				"prose that does not fit a screen belongs in the guide, not in `--help`",
				name, n, helpBodyBudget)
		}
		for _, line := range strings.Split(body, "\n") {
			if n := utf8.RuneCountInString(line); n > 80 {
				t.Errorf("`app listing %s` Long has an %d-column line (>80) — cobra does not wrap "+
					"Long, so it will hard-wrap in a standard terminal:\n%s", name, n, line)
			}
		}
	}
	if checked < 7 {
		t.Fatalf("only %d bodies were measured, want 7 — a body that is empty is not a body "+
			"that is within budget", checked)
	}
}
