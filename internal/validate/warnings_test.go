package validate

import (
	"strings"
	"testing"
)

func hasWarningContaining(res Result, substr string) bool {
	for _, w := range res.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// A valid, well-formed budgeted page WITHOUT a per-gen budget — the headline
// footgun. Must warn, must NOT error, must exit OK (no --strict).
func TestWarnBudgetedPageWithoutBudget(t *testing.T) {
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "no-budget",
  "version": "0.1.0",
  "name": "No Budget",
  "type": "block",
  "scopes": ["ai:write:budgeted"],
  "page": { "path": "/", "title": "No Budget" },
  "iframe": { "minHeight": 600, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "buildCommand": "npm run build",
  "outputDir": "dist"
}`
	res := validateManifest(t, m)
	if !res.OK() {
		t.Fatalf("budgeted-page-without-budget should NOT be a hard error: %v", res.Errors)
	}
	if !hasWarningContaining(res, "page.buzzBudgetPerGen is not set") {
		t.Errorf("expected budgeted-without-budget warning, got: %v", res.Warnings)
	}
}

// A budgeted manifest with NO page block — money scope on a non-page app.
func TestWarnBudgetedScopeWithoutPage(t *testing.T) {
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "money-static",
  "version": "0.1.0",
  "name": "Money Static",
  "type": "block",
  "scopes": ["ai:write:budgeted"],
  "iframe": { "minHeight": 600, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g"
}`
	res := validateManifest(t, m)
	if !res.OK() {
		t.Fatalf("budgeted-without-page should NOT be a hard error: %v", res.Errors)
	}
	if !hasWarningContaining(res, "no \"page\" block is declared") {
		t.Errorf("expected budgeted-without-page warning, got: %v", res.Warnings)
	}
}

// A page with a budget but NO budgeted scope — the budget is inert.
func TestWarnBudgetWithoutScope(t *testing.T) {
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "inert-budget",
  "version": "0.1.0",
  "name": "Inert Budget",
  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "Inert Budget", "buzzBudgetPerGen": 10 },
  "iframe": { "minHeight": 600, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "buildCommand": "npm run build",
  "outputDir": "dist"
}`
	res := validateManifest(t, m)
	if !res.OK() {
		t.Fatalf("budget-without-scope should NOT be a hard error: %v", res.Errors)
	}
	if !hasWarningContaining(res, "the budget is inert") {
		t.Errorf("expected inert-budget warning, got: %v", res.Warnings)
	}
}

// A correctly-configured budgeted page (budget + scope) emits NO warnings.
func TestNoWarningsForBudgetedPageWithBudget(t *testing.T) {
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "good-money",
  "version": "0.1.0",
  "name": "Good Money",
  "type": "block",
  "scopes": ["ai:write:budgeted"],
  "page": { "path": "/", "title": "Good Money", "buzzBudgetPerGen": 10 },
  "iframe": { "minHeight": 600, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "buildCommand": "npm run build",
  "outputDir": "dist"
}`
	res := validateManifest(t, m)
	if !res.OK() {
		t.Fatalf("good money page should be valid: %v", res.Errors)
	}
	if res.HasWarnings() {
		t.Errorf("good money page should be warning-free, got: %v", res.Warnings)
	}
}

// A plain static app (no money scope, no page) emits no warnings.
func TestNoWarningsForStaticApp(t *testing.T) {
	const m = `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "plain-static",
  "version": "0.1.0",
  "name": "Plain Static",
  "type": "block",
  "scopes": ["models:read:self"],
  "iframe": { "minHeight": 200, "resizable": true, "sandbox": "allow-scripts" },
  "contentRating": "g"
}`
	res := validateManifest(t, m)
	if res.HasWarnings() {
		t.Errorf("plain static app should be warning-free, got: %v", res.Warnings)
	}
}
