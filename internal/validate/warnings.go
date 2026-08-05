package validate

// warnings.go holds the NON-FATAL advisory tier of `civitai app validate`.
//
// These are real money-path / page footguns the JSON Schema and the ported
// semantic rules can't express as HARD errors (the manifest is technically
// well-formed and the server won't necessarily reject it), but which are almost
// always a mistake an author wants flagged before submit. They never fail
// validation unless the caller passes --strict.
//
// The budgeted-page-without-budget case is the headline one the page-money
// dogfood surfaced: a page declaring `ai:write:budgeted` but no
// `page.buzzBudgetPerGen` is minted with the platform's 10-Buzz fallback budget,
// which is below almost any real generation, so every submit is rejected
// insufficient-budget before it runs — a silent, confusing dead end.
//
// SIZING — `page.buzzBudgetPerGen` is a SAFETY CEILING, not a forecast. It caps
// what ONE generation the app requests may cost, so a bug or a compromised
// bundle can't drain the viewer's Buzz. The server re-prices every submit and
// charges the REAL price, so headroom is free; a budget sized to an ESTIMATE
// breaks the app outright (hard `insufficient buzz budget` reject, nothing
// charged, nothing delivered) the moment real cost drifts up. Advice here must
// therefore always push budgets UP, never down.
//
// DELIBERATELY NOT CHECKED: "this value looks small". The CLI cannot price a
// generation (that is a server-side whatIf against live model pricing), so any
// threshold would be arbitrary — and a manifest sized correctly for a cheap
// single-recipe app is byte-identical to one sized from a per-run estimate. A
// magnitude warning would also have to fire above this CLI's own page-money
// scaffold value (40), and `--strict` promotes warnings to hard failures.

const budgetedScope = "ai:write:budgeted"

// warningChecks runs the advisory rules over the decoded manifest map.
func warningChecks(generic any) []string {
	m, ok := generic.(map[string]any)
	if !ok {
		return nil
	}
	var warns []string

	page, hasPage := m["page"].(map[string]any)
	scopes := stringSet(m["scopes"])
	_, hasBudgeted := scopes[budgetedScope]
	_, hasTargets := m["targets"].([]any)

	// (1) Budgeted scope but no per-gen budget on the page. The page's tokens
	//     fall back to the platform's 10-Buzz default → every submit is rejected
	//     before it runs. The headline footgun the dogfood found.
	if hasBudgeted && hasPage {
		if _, hasBudget := page["buzzBudgetPerGen"]; !hasBudget {
			warns = append(warns,
				"scopes include \"ai:write:budgeted\" but page.buzzBudgetPerGen is not set — "+
					"tokens fall back to a 10 Buzz budget, below almost any real generation, so "+
					"every submit is rejected with insufficient budget before it runs. Set "+
					"page.buzzBudgetPerGen: it is a SAFETY CEILING on what one generation may "+
					"cost, not a cost estimate — size it several times your worst-case run "+
					"(headroom is free; you are charged the real price, and the server clamps "+
					"the budget at the per-gen cap)")
		}
	}

	// (2) The budgeted (money) scope on a NON-page app. Budgeted spend is a page
	//     affordance; a model-slot/static app requesting it almost certainly
	//     meant to be a page (declare a `page` field). Not a hard error (the
	//     server gates the spend), but worth surfacing.
	if hasBudgeted && !hasPage {
		warns = append(warns,
			"scopes include \"ai:write:budgeted\" but no \"page\" block is declared — "+
				"budgeted Buzz spend is a full-page (W10) affordance; declare a \"page\" field, "+
				"or drop the scope if this isn't a money-path page app")
	}

	// (3) A page app that declares neither model targets nor a money scope and no
	//     budget — likely an incomplete page. Purely informational: a page with
	//     a generation budget but no budgeted scope can never spend.
	if hasPage && !hasBudgeted {
		if _, hasBudget := page["buzzBudgetPerGen"]; hasBudget {
			warns = append(warns,
				"page.buzzBudgetPerGen is set but scopes do not include \"ai:write:budgeted\" — "+
					"the budget is inert; add the scope to enable budgeted generation, or remove the budget")
		}
	}

	_ = hasTargets // reserved for future target-related advisories.
	return warns
}

// stringSet decodes a JSON array of strings into a set, ignoring non-strings.
func stringSet(v any) map[string]struct{} {
	out := map[string]struct{}{}
	arr, ok := v.([]any)
	if !ok {
		return out
	}
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out[s] = struct{}{}
		}
	}
	return out
}
