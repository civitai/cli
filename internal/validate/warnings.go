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
// `page.buzzBudgetPerGen` mints budget-less tokens, so every generation fails
// insufficient-budget at the orchestrator — a silent, confusing dead end.

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

	// (1) Budgeted scope but no per-gen budget on the page. A page mints
	//     budget-less ai:write:budgeted tokens → every spend fails at the
	//     orchestrator. The headline footgun the dogfood found.
	if hasBudgeted && hasPage {
		if _, hasBudget := page["buzzBudgetPerGen"]; !hasBudget {
			warns = append(warns,
				"scopes include \"ai:write:budgeted\" but page.buzzBudgetPerGen is not set — "+
					"a page mints budget-less tokens, so every generation will fail with "+
					"insufficient budget; set page.buzzBudgetPerGen (server-clamped to the per-gen cap)")
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
