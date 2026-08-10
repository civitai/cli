package cmd

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/civitai/cli/internal/appapi"
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
// 4.0 MiB" is satisfied by a body that ALSO quotes the icon's — the shape a
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

	// 🔴 THE GROUP ASSERTION MUST BE CAP-IN-CONTEXT, NOT CAP-ALONE.
	//
	// `humanBytes(maxIconBytes)` and `humanBytes(maxScreenshotBytes)` are the SAME
	// STRING today ("2.0 MiB"), so three bare Contains checks over three strings —
	// two of them byte-identical — enforce only TWO of the three caps. Measured:
	// deleting the icon cap from the group body, and deleting the screenshot cap
	// from it, BOTH survived a green suite, because the other one's identical
	// text satisfied the check. Pairing each cap with its label is what makes the
	// three assertions independent.
	t.Run("group", func(t *testing.T) {
		parent := nodes[""]
		for _, want := range []string{
			humanBytes(maxIconBytes) + " for an icon",
			humanBytes(maxCoverBytes) + " for a cover",
			humanBytes(maxScreenshotBytes) + " for a screenshot",
			listingImageFormats,
		} {
			if !strings.Contains(parent.Long, want) {
				t.Errorf("`app listing` group Long does not state %q — the group page is where a "+
					"reader lands first, so all three caps belong there, each next to the kind "+
					"it applies to", want)
			}
		}
	})
}

// TestListingHelpNamesTheKindItDescribes closes the other half of the shared-cap
// hole: a per-kind body asserted only to contain "2.0 MiB" is satisfied by the
// OTHER 2 MiB kind's sentence.
//
// Measured: swapping `set-cover`'s `listingSourceRule(kindCover)` to `kindIcon`
// SURVIVED the original guard. Requiring the body to name its own kind, and NOT
// to name a sibling that has a DIFFERENT cap, is what makes such a swap
// observable — the cap string alone cannot.
//
// 🔴 THE ICON <-> SCREENSHOT SWAP IS A DECLARED **EQUIVALENT MUTANT**, NOT A
// SURVIVING HOLE, AND THE DIFFERENCE MATTERS TO WHOEVER EDITS THIS NEXT.
// `humanBytes(maxIconBytes)` and `humanBytes(maxScreenshotBytes)` are both
// "2.0 MiB" today, so swapping `kindScreenshot` -> `kindIcon` produces a DIFFERENT
// BINARY and BYTE-IDENTICAL rendered help — measured, `cmp` on the two `--help`
// outputs, with the differing binaries as the negative control. There is nothing
// for any assertion to observe, which is why `foreign` deliberately names COVER
// (a differently-capped sibling) rather than the same-capped one: an assertion
// against a token that cannot vary is a guard that cannot fail.
// The protection against the swap MATTERING is elsewhere and was measured too —
// set maxScreenshotBytes to 3 MiB under that mutation and
// TestListingHelpQuotesTheEnforcedCaps reddens with "does not quote the cap it
// enforces (3.0 MiB)". So the swap is invisible exactly while it is harmless.
func TestListingHelpNamesTheKindItDescribes(t *testing.T) {
	nodes := listingHelpNodes(t)
	cases := []struct {
		cmd  string
		self string
		// A sibling whose cap DIFFERS, so naming it is unambiguous evidence of a
		// copy-paste rather than an incidental mention.
		foreign string
	}{
		{"set-icon", "ICON", "COVER"},
		{"set-cover", "COVER", "ICON"},
		{"add-screenshot", "SCREENSHOT", "COVER"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			c, ok := nodes[tc.cmd]
			if !ok {
				t.Fatalf("`app listing %s` is not in the command tree", tc.cmd)
			}
			if !strings.Contains(c.Long, tc.self) {
				t.Errorf("`app listing %s` Long never names %s — a body that does not say which "+
					"asset it is about cannot be distinguished from its sibling's", tc.cmd, tc.self)
			}
			if strings.Contains(c.Long, tc.foreign) {
				t.Errorf("`app listing %s` Long names %s, a different asset with a different cap — "+
					"this is what a copy-paste between two media bodies looks like", tc.cmd, tc.foreign)
			}
		})
	}
}

// TestListingImageFormatsMatchesTheDecoder is the drift check for the FOURTH
// thing this file states about images.
//
// 🔴 `listingImageFormats` is a prose MIRROR of what appapi.DecodeImageInfo will
// actually decode, and the caps guard above cannot see it: that assertion
// compares the constant against a Long built by interpolating the SAME constant,
// so it is a constant compared with itself and can never fail. Measured, both on
// a green suite: narrowing the constant to "png or jpeg" (the help then
// UNDERSTATES — webp is accepted) survived, and widening it to
// "png, jpeg or avif" (the help then LIES — avif is refused) survived.
//
// Unlike this repo's server-side mirrors there is a local counterpart to check
// against, so the drift is cheap to pin: decode a real header of each claimed
// format and refuse one that is not claimed.
func TestListingImageFormatsMatchesTheDecoder(t *testing.T) {
	accepted := map[string][]byte{
		"png":  minimalPNG(t),
		"jpeg": minimalJPEG(t),
		"webp": minimalWebP(t),
	}
	for name, data := range accepted {
		t.Run("accepted/"+name, func(t *testing.T) {
			if _, err := appapi.DecodeImageInfo(data); err != nil {
				t.Fatalf("listingImageFormats claims %s is accepted, but DecodeImageInfo refused it: %v",
					name, err)
			}
			if !strings.Contains(listingImageFormats, name) {
				t.Errorf("DecodeImageInfo accepts %s but listingImageFormats (%q) does not name it — "+
					"the help UNDERSTATES what the CLI will take", name, listingImageFormats)
			}
		})
	}
	// Negative control: a format the constant must NOT claim. Without this the
	// test above is satisfied by a constant listing every format under the sun.
	t.Run("refused/gif", func(t *testing.T) {
		if _, err := appapi.DecodeImageInfo(minimalGIF(t)); err == nil {
			t.Fatal("DecodeImageInfo accepted a GIF — this test's premise is stale, not the constant")
		}
		if strings.Contains(listingImageFormats, "gif") {
			t.Errorf("listingImageFormats (%q) claims gif, which DecodeImageInfo refuses — "+
				"the help LIES about what it will take", listingImageFormats)
		}
	})
}

// Fixtures for the drift check above. They are REAL encoded images (or, for
// WebP, a real RIFF/VP8X container header), not magic-byte stubs — the point is
// to exercise the decoder the CLI actually calls, and a stub would only prove
// that a byte comparison works.
func minimalPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func minimalJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

// minimalWebP is a VP8X (extended) container header — the sub-format that
// carries the canvas size directly, so a 30-byte header is a complete answer to
// DecodeImageInfo without encoding pixel data the stdlib cannot write.
func minimalWebP(*testing.T) []byte {
	b := make([]byte, 30)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	copy(b[12:16], "VP8X")
	b[24], b[27] = 1, 1 // (width-1, height-1) little-endian -> 2x2
	return b
}

// minimalGIF is hand-built, and 🔴 MUST NOT be replaced with `gif.Encode`.
//
// Importing `image/gif` anywhere in this package runs its init() and REGISTERS
// the GIF decoder process-wide, which changes how `civitai generate --image`
// refuses a GIF: it stops failing via `image.ErrFormat` and starts failing via
// the allowlist branch. That is not hypothetical — the first version of this
// file did import `image/gif` and reddened three unrelated tests, including
// TestSupportedImageFormats_DecoderRegistrationIsInLockstep, whose whole job is
// to catch exactly this ("the unreachability argument for the allowlist branch
// depends on this being the ErrFormat arm"). A test fixture that mutates global
// decoder state is a test that changes the thing it is measuring.
//
// These 13 bytes are a valid GIF87a header (magic + logical screen descriptor),
// which is all any header-only decoder would read.
func minimalGIF(*testing.T) []byte {
	b := []byte("GIF87a")
	b = append(b, 2, 0, 2, 0) // width=2, height=2 (little-endian uint16)
	return append(b, 0x00, 0x00, 0x00)
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
		// that is fine.
		//
		// Measured on THIS tree: a byte count reddens 4 lines across 4 bodies
		// (`app listing`, set-icon, set-cover, add-screenshot) that all render
		// inside 80 columns. An earlier version of this comment called those four
		// bodies PRE-EXISTING; that was wrong — three of them had no Long at all
		// before this file was written. The 3 genuinely pre-existing over-80
		// COLUMN lines (two in the group body, one in `status`) are a separate
		// set, and are rewrapped rather than excused.
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
