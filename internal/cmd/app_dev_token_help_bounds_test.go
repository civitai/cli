package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// helpSpaceRE collapses the help's hard line wraps so a sentence can be pinned
// whole regardless of where the column limit broke it.
var helpSpaceRE = regexp.MustCompile(`\s+`)

// helpIntegerRE matches a WHOLE integer, so "50" is not found inside "5000" or
// "250" — the substring form of this check is exactly what failed to notice the
// help going stale (see TestAppDevTokenBudgetHelpTeachesTheTrap).
var helpIntegerRE = regexp.MustCompile(`\b\d+\b`)

// TestAppDevTokenHelpStatesTheVendoredBudgetBounds is the regression test for
// the help teaching the dev-token budget bounds the server had already moved
// off: it said "1-250", "50 for a slug with no submitted app" and offered
// `--budget 250` as the max after the server aliased the dev bounds to the
// install-token ones (now appapi.DevBuzzBudgetCap / DevBuzzBudgetDefault).
//
// Every sentence that states a bound is pinned WHOLE, built from the appapi
// constants, so the help can neither keep a stale literal nor drift from the
// vendored value — and a reword of a sentence that makes a numeric claim has to
// come through here. Red against main's app_dev_token.go (literal 250/50) with
// the current appapi constants; green after.
func TestAppDevTokenHelpStatesTheVendoredBudgetBounds(t *testing.T) {
	out, _, err := run(t, "app", "dev-token", "--help")
	if err != nil {
		t.Fatalf("app dev-token --help: %v", err)
	}
	flat := helpSpaceRE.ReplaceAllString(out, " ")

	minB, capB, defB, inl := appapi.DevBuzzBudgetMin, appapi.DevBuzzBudgetCap, appapi.DevBuzzBudgetDefault, appapi.InlineMaxBuzz
	wantSentences := []string{
		// Long help.
		fmt.Sprintf("BUDGET (--budget, %d-%d Buzz). The token carries a per-generation Buzz budget.", minB, capB),
		fmt.Sprintf("Omit --budget and the server picks one — %d for a slug with no submitted app.", defB),
		fmt.Sprintf("Your LOCAL block.manifest.json page.buzzBudgetPerGen does NOT set it: until the app is submitted "+
			"there is no server-side manifest to read, so the %d is a flat default, not a reading of your file.", defB),
		`A generation is REFUSED outright when its Buzz ceiling exceeds that budget, with an ` +
			`"insufficient buzz budget: ... exceeds budget <M>" error.`,
		fmt.Sprintf("Raise --budget (up to %d) rather than editing the recipe.", capB),
		fmt.Sprintf("Give it the seconds the graph needs, but note the server caps an inline maxBuzz at %d — "+
			"so at most %ds of wall clock — whatever the token's budget: --budget %d does not lift it.", inl, inl, capB),
		// Example.
		fmt.Sprintf("civitai app dev-token my-block --budget %d # the most a dev token can carry", capB),
		// Flag help.
		fmt.Sprintf("per-generation Buzz budget the token may spend (%d-%d; omit to let the server decide — "+
			"%d for an unsubmitted app). Must clear your recipe's ceiling. It does not lift an inline customComfy "+
			"maxBuzz (also its step timeout in seconds), which the server caps at %d", minB, capB, defB, inl),
	}
	for _, want := range wantSentences {
		if !strings.Contains(flat, want) {
			t.Errorf("`app dev-token --help` does not state:\n  %q\n(the numbers come from appapi; "+
				"help that types them instead goes stale when the server moves them)\n\nhelp:\n%s", want, out)
		}
	}

	// MECHANICAL NEGATIVE CONTROL against the stale figures. The old default (50)
	// must not appear as a whole number at all, and 250 may appear only as the
	// inline maxBuzz cap — never as the --budget range or the max --budget.
	nums := map[string]bool{}
	for _, tok := range helpIntegerRE.FindAllString(out, -1) {
		nums[tok] = true
	}
	if len(nums) < 3 { // positive control: the tokeniser does see the help's numbers
		t.Fatalf("tokeniser found only %v in the help — the checks below would be vacuous", nums)
	}
	if defB != 50 && nums["50"] {
		t.Errorf("help still states 50, the retired dev-token default (now %d):\n%s", defB, out)
	}
	if capB != 250 {
		for _, stale := range []string{"1-250", "--budget 250", "50 for"} {
			if strings.Contains(flat, stale) {
				t.Errorf("help still contains the stale budget phrase %q (cap is now %d):\n%s", stale, capB, out)
			}
		}
	}
}
