package cmd

import (
	"strings"
	"testing"
	"time"
)

// budgetRemintLines returns the suggested `civitai app dev-token …` commands in s
// that carry a --budget, which is to say the budget-shortfall notice's remedy and
// nothing else (spendFilteredNotice's own re-mint names no budget).
func budgetRemintLines(s string) []string {
	var out []string
	for _, l := range devTokenCommandLines(s) {
		if strings.Contains(l, "--budget ") {
			out = append(out, l)
		}
	}
	return out
}

// TestBudgetShortfallRemintCarriesTheSpendFlagItWasRunWith is end-to-end on
// purpose: the defect was in what the COMMAND prints after a real mint, and a
// test of the pure function alone would pass with the call site still handing it
// the wrong value.
//
// The measured failure: `civitai app dev-token <slug> --spend --env` on a manifest
// declaring a higher budget than the mint granted printed
//
//	civitai app dev-token <slug> --budget 100 --env >> .env.development.local
//
// which fixes the budget and drops --spend. Without --spend the CLI filters
// `ai:write:budgeted` out of the request, so following the hint exactly produces
// a token that cannot generate at all.
//
// The whole line is pinned rather than `Contains(line, "--spend")`: the remedy is
// a command somebody pastes, so its flags, their values and the redirect are all
// the claim. 120 is used because it is neither the server default (50) nor the cap
// (250), so a notice that hardcoded either figure would not pass.
func TestBudgetShortfallRemintCarriesTheSpendFlagItWasRunWith(t *testing.T) {
	jwt := func(t *testing.T) string {
		return makeJWTClaims(t, map[string]any{
			"scopes":     []string{"ai:write:budgeted"},
			"buzzBudget": 50,
			"exp":        time.Now().Add(4 * time.Hour).Unix(),
		})
	}

	t.Run("minted with --spend", func(t *testing.T) {
		writeManifestWithBudget(t, `{"buzzBudgetPerGen":120}`)
		_, errOut := devTokenStderr(t, jwt(t), "--spend", "--env")

		got := budgetRemintLines(errOut)
		const want = "civitai app dev-token my-block --spend --budget 120 --env >> .env.development.local"
		if len(got) != 1 || got[0] != want {
			t.Fatalf("after a --spend mint the budget re-mint must repeat --spend.\n got: %q\nwant: [%q]\nfull stderr:\n%s",
				got, want, errOut)
		}
	})

	// The mirror image, and the control that keeps the fix honest: a mint that was
	// NOT given --spend must not have one invented for it by this notice. The CLI
	// never asks for budgeted spend implicitly, and a suggested command is the
	// nearest thing to asking. (spendFilteredNotice, which names --spend as its
	// own explicit remedy, still prints here — it carries no --budget, so it is
	// outside what this helper collects.)
	t.Run("minted without --spend", func(t *testing.T) {
		writeManifestWithBudget(t, `{"buzzBudgetPerGen":120}`)
		_, errOut := devTokenStderr(t, jwt(t), "--env")

		got := budgetRemintLines(errOut)
		const want = "civitai app dev-token my-block --budget 120 --env >> .env.development.local"
		if len(got) != 1 || got[0] != want {
			t.Fatalf("a mint without --spend must get a budget re-mint without it.\n got: %q\nwant: [%q]\nfull stderr:\n%s",
				got, want, errOut)
		}
	})
}
