package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// THE FAKE-FIDELITY LEDGER for `appListings.getMyListingForApp`.
//
// 🔴 WHY THIS EXISTS. `appapi.ListingRef` decoded four keys while the server
// sends eight, and `app listing set-text`'s overwrite warning depended on one of
// the four it dropped — `shadowId`. The defect was real, shipped, and
// UNCATCHABLE, because every fake reply in this package omitted `shadowId` too.
// The fake encoded the same wrong assumption as the code, so a green suite meant
// nothing: no test could construct the state the warning existed for.
//
// A fake that agrees with the bug is worse than no fake. It converts "we have not
// tested this" into "we have tested this and it is fine", which is the one
// transformation a test suite must never perform.
//
// 🔴 THIS IS A ZERO-OFFENDER STRUCTURAL ASSERTION, NOT A LIST OF KNOWN FILES. A
// hand-maintained list covers the fakes somebody thought of, which is the same
// failure one level up. The rule is mechanical: any `trpcData` reply in this
// package that answers a `getMyListingForApp` request must carry `shadowId`. Six
// such literals existed across three files when this was written; all six were
// corrected in the same commit, so the assertion starts and stays at zero.
//
// 🔴 THE SCANNER IS A PURE FUNCTION AND HAS ITS OWN TEST, AND THAT IS NOT
// CEREMONY. The first version inlined the scan in the ledger, so a mutation that
// made the offender check `if false` SURVIVED the entire suite: with no
// offenders to find, a blinded scanner and a clean tree produce byte-identical
// output. The manual plant-and-restore that "verified" it was a fact about one
// afternoon, not a property CI holds. `scanListingRefFakes` is now exercised
// against synthetic sources that MUST yield an offender, so blinding it goes red
// on its own terms — see TestScanListingRefFakesCanSeeAnOffender.
//
// 🔴 WHAT IT CANNOT SEE, measured rather than waved at. It is a source scanner
// with a window, so: a reply built by a helper function rather than inline; a
// reply assembled across more than the window below; a fake in another package.
// It catches the shape that actually occurred — an inline map literal written
// next to the case that serves it — which is how all six arose.

// fakeReplyWindow bounds how far after a `getMyListingForApp` mention the
// scanner looks for the reply literal. Generous but finite; a literal further
// away is outside what this guard claims to cover, and the doc says so.
const fakeReplyWindow = 1200

var trpcDataLiteral = regexp.MustCompile(`(?s)trpcData\(w, map\[string\]any\{(.*?)\n\t*\}\)`)

// scanListingRefFakes reports how many getMyListingForApp reply literals a
// source carries, and how many of those omit `shadowId`.
//
// Pure and exported to the package's tests so the SCANNER itself is testable.
// A guard whose detection logic cannot be exercised is a guard nobody can prove
// works.
func scanListingRefFakes(src string) (candidates int, offending int) {
	for _, idx := range indexAll(src, "getMyListingForApp") {
		end := idx + len("getMyListingForApp")
		stop := end + fakeReplyWindow
		if stop > len(src) {
			stop = len(src)
		}
		m := trpcDataLiteral.FindStringSubmatch(src[end:stop])
		if m == nil || !strings.Contains(m[1], `"appListingId"`) {
			continue
		}
		candidates++
		if !strings.Contains(m[1], `"shadowId"`) {
			offending++
		}
	}
	return candidates, offending
}

// TestScanListingRefFakesCanSeeAnOffender is the POSITIVE CONTROL, as code.
//
// 🔴 IT IS THE REASON THE LEDGER BELOW MEANS ANYTHING. "Zero offenders" and "the
// scanner is blind" are the same output, and a mutation sweep proved the point:
// blinding the offender check survived a fully green suite until this existed.
func TestScanListingRefFakesCanSeeAnOffender(t *testing.T) {
	const bad = "" +
		"case strings.Contains(r.URL.Path, \"getMyListingForApp\"):\n" +
		"\ttrpcData(w, map[string]any{\n" +
		"\t\t\"appListingId\": \"apl_X\",\n" +
		"\t\t\"status\":       \"draft\",\n" +
		"\t})\n"
	const good = "" +
		"case strings.Contains(r.URL.Path, \"getMyListingForApp\"):\n" +
		"\ttrpcData(w, map[string]any{\n" +
		"\t\t\"appListingId\": \"apl_X\",\n" +
		"\t\t\"shadowId\":     nil,\n" +
		"\t})\n"

	if c, o := scanListingRefFakes(bad); c != 1 || o != 1 {
		t.Errorf("a reply OMITTING shadowId must be seen as 1 candidate / 1 offender, got %d/%d — "+
			"the ledger's zero would be a fact about a blind scanner", c, o)
	}
	if c, o := scanListingRefFakes(good); c != 1 || o != 0 {
		t.Errorf("a reply CARRYING shadowId must be 1 candidate / 0 offenders, got %d/%d — "+
			"a scanner that flags everything is as useless as one that flags nothing", c, o)
	}
	// And a source with no such reply must contribute nothing, or the candidate
	// floor below could be met by unrelated text.
	if c, o := scanListingRefFakes("func unrelated() {}\n"); c != 0 || o != 0 {
		t.Errorf("unrelated source must yield 0/0, got %d/%d", c, o)
	}
}

// TestGetMyListingForAppFakesCarryShadowID is the ledger.
func TestGetMyListingForAppFakesCarryShadowID(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	candidates := 0
	var offenders []string
	for _, f := range files {
		if f == "listing_ref_fake_fidelity_test.go" {
			continue // this file carries synthetic fixtures on purpose
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		c, o := scanListingRefFakes(string(raw))
		candidates += c
		if o > 0 {
			offenders = append(offenders, f)
		}
	}

	// Positive control on the POPULATION: a run that found no candidate replies
	// would report zero offenders and look identical to a clean tree.
	if candidates < 6 {
		t.Fatalf("the scan found only %d getMyListingForApp reply literals — it is not reading what it "+
			"thinks it is, so the zero below would be a fact about the regex rather than about the fakes. "+
			"If replies were legitimately removed, lower this floor in the same commit.", candidates)
	}
	if len(offenders) != 0 {
		t.Errorf("%d file(s) carry getMyListingForApp fake replies that omit `shadowId`: %v\n"+
			"The real server ALWAYS sends it, and `appapi.ListingRef` decodes it. A fake that omits it "+
			"cannot construct an open-but-unsubmitted revision — the exact state `app listing set-text`'s "+
			"overwrite warning exists for, and the reason that defect shipped uncatchable. Add "+
			"`\"shadowId\": nil` (or a real id) to each.", len(offenders), offenders)
	}
	t.Logf("scanned %d getMyListingForApp reply literals, %d offending files", candidates, len(offenders))
}

func indexAll(s, sub string) []int {
	var out []int
	for i := 0; ; {
		j := strings.Index(s[i:], sub)
		if j < 0 {
			return out
		}
		out = append(out, i+j)
		i += j + len(sub)
	}
}
