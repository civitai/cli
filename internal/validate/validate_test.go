package validate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// scaffoldRaw renders a template exactly as `civitai app create` leaves it — no
// lockfile, because the author's first `npm install` writes that.
func scaffoldRaw(t *testing.T, tmpl scaffold.Template) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "block")
	if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "good-block", Name: "Good Block"}); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return dir
}

// scaffoldGood renders a template in the state an author would actually SUBMIT:
// scaffolded, then installed so the lockfile the platform build installs
// strictly from is committed.
func scaffoldGood(t *testing.T, tmpl scaffold.Template) string {
	t.Helper()
	dir := scaffoldRaw(t, tmpl)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		// The npm templates declare `npm run build`; stand in for `npm install`.
		// The body has to be what an install WRITES, not `{}` — measured on npm
		// 11.17.0, `npm ci` over `{}` exits 1 on any project with a dependency
		// (the sync check: "Missing: <pkg> from lock file"), so a `{}` fixture
		// would assert that a build-breaking project validates clean (issue #255).
		if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(npmLockBody), 0o600); err != nil {
			t.Fatalf("write package-lock.json: %v", err)
		}
	}
	return dir
}

func TestValidateAcceptsStaticTemplate(t *testing.T) {
	res, err := Dir(scaffoldGood(t, scaffold.Static))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("static template should be valid, got: %v", res.Errors)
	}
}

func TestValidateAcceptsPageViteTemplate(t *testing.T) {
	res, err := Dir(scaffoldGood(t, scaffold.PageVite))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("page-vite template should be valid, got: %v", res.Errors)
	}
}

func TestValidateAcceptsPageMoneyTemplate(t *testing.T) {
	res, err := Dir(scaffoldGood(t, scaffold.PageMoney))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !res.OK() {
		t.Fatalf("page-money template should be valid, got: %v", res.Errors)
	}
	// The scaffolded page-money manifest declares a budget, so it must be
	// warning-free (no budgeted-without-budget advisory).
	if res.HasWarnings() {
		t.Errorf("page-money scaffold should be warning-free, got: %v", res.Warnings)
	}
}

// writeManifest writes a manifest-only project dir and validates it.
func validateManifest(t *testing.T, json string) Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	return res
}

func mustReject(t *testing.T, name, json, wantSubstr string) {
	t.Helper()
	res := validateManifest(t, json)
	if res.OK() {
		t.Fatalf("%s: expected rejection, got valid", name)
	}
	if wantSubstr != "" {
		found := false
		for _, e := range res.Errors {
			if strings.Contains(e.Message, wantSubstr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: errors %v did not contain %q", name, res.Errors, wantSubstr)
		}
	}
}

const baseGood = `{
  "blockId": "ok-block",
  "version": "0.1.0",
  "name": "OK",
  "contentRating": "g",
  "scopes": [],
  "page": {"path": "/", "title": "OK"},
  "iframe": {"minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms"}
}`

func TestValidateAcceptsMinimalGood(t *testing.T) {
	res := validateManifest(t, baseGood)
	if !res.OK() {
		t.Fatalf("baseGood should validate, got: %v", res.Errors)
	}
}

func TestValidateRejectsMissingRequired(t *testing.T) {
	mustReject(t, "missing blockId", `{
		"version": "0.1.0", "name": "X", "contentRating": "g", "scopes": []
	}`, "blockId")
}

func TestValidateRejectsBadScope(t *testing.T) {
	mustReject(t, "bad scope", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["NotAScope"]
	}`, "scopes")
}

// --- current-scope enum + scopeJustifications shape (Part 1: re-vendored
// schema). These scopes were MISSING from the pre-re-vendor stale schema, so
// these manifests would have failed the old vendored enum. ---

func TestValidateAcceptsCurrentStorageSharedReadScope(t *testing.T) {
	// apps:storage:shared:read is a CURRENT, non-sensitive scope absent from the
	// old stale enum. It must validate with no justification needed.
	mustAccept(t, "apps:storage:shared:read", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["apps:storage:shared:read"],
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

func TestValidateAcceptsSensitiveScopeWithJustification(t *testing.T) {
	// apps:storage:shared:write is a CURRENT, SENSITIVE scope. With a non-empty
	// scopeJustifications entry the manifest validates end-to-end — exercising
	// both the re-vendored enum AND the accepted scopeJustifications shape.
	mustAccept(t, "sensitive scope + justification", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["apps:storage:shared:write"],
		"scopeJustifications": {"apps:storage:shared:write": "persist the user's saved presets so they survive reloads"},
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

// TestValidateRejectsSensitiveScopeWithoutJustification is the Part 2 end-to-end
// mirror of the live backend enforcement: a declared sensitive scope with no
// justification is a HARD error (validate fails, non-zero exit) carrying the
// server's exact message — no server round-trip needed.
func TestValidateRejectsSensitiveScopeWithoutJustification(t *testing.T) {
	mustReject(t, "sensitive scope, no justification", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["ai:write:budgeted"],
		"page": {"path": "/", "title": "X", "buzzBudgetPerGen": 100},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`, "sensitive scopes require a justification — add a non-empty scopeJustifications entry for: ai:write:budgeted")
}

// --- marketplace category (optional; enforced by the vendored schema enum,
// mirroring the server's BlockManifestValidator category check ~L312) ---

func TestValidateAcceptsValidCategory(t *testing.T) {
	// A manifest declaring a known marketplace category validates.
	mustAccept(t, "valid category", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "category": "generation",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

func TestValidateRejectsUnknownCategory(t *testing.T) {
	// An out-of-taxonomy category fails with a clear, field-keyed error.
	mustReject(t, "unknown category", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "category": "bananas",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`, "category")
}

func TestValidateAcceptsMissingCategory(t *testing.T) {
	// category is optional — a manifest omitting it validates (baseGood has none).
	res := validateManifest(t, baseGood)
	if !res.OK() {
		t.Fatalf("manifest without category should validate, got: %v", res.Errors)
	}
}

// --- store tagline (optional; enforced by the vendored schema's minLength /
// maxLength, mirroring the server's BlockManifestValidator tagline check) ---
//
// NOTE on a deliberate, SAFE asymmetry: the server checks the TRIMMED length
// while JSON Schema's maxLength counts the RAW string. So a 140-char tagline
// padded with trailing spaces is rejected here but accepted server-side — the
// CLI is the stricter side, which fails locally rather than surprising the
// author at submit. The schema is a shape hint; the server validator wins.

func TestValidateAcceptsTagline(t *testing.T) {
	mustAccept(t, "tagline", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": "The fastest way to remix a model",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

func TestValidateAcceptsTaglineAtCap(t *testing.T) {
	// Exactly 140 chars — the boundary must be inclusive.
	mustAccept(t, "tagline at cap", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": "`+strings.Repeat("a", 140)+`",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

func TestValidateRejectsOverLongTagline(t *testing.T) {
	// One char over the 140 cap fails with a clear, field-keyed error.
	mustReject(t, "over-long tagline", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": "`+strings.Repeat("a", 141)+`",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`, "tagline")
}

func TestValidateRejectsWhitespaceOnlyTagline(t *testing.T) {
	// The server TRIMS before measuring, so a whitespace-only tagline is a blank
	// one. The schema's `\S` pattern is what keeps this rejection in lockstep —
	// without it the schema would be MORE permissive than the server, which is
	// the divergence direction that actually hurts (green locally, rejected at
	// submit).
	mustReject(t, "whitespace-only tagline", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": "   ",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`, "tagline")
}

func TestValidateAcceptsPaddedTagline(t *testing.T) {
	// Padding AROUND real content is fine — the server trims it off.
	mustAccept(t, "padded tagline", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": "  A crisp one-liner  ",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

func TestValidateRejectsEmptyTagline(t *testing.T) {
	// An empty tagline is not "no tagline" — omit the field instead.
	mustReject(t, "empty tagline", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": "",
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`, "tagline")
}

func TestValidateRejectsNonStringTagline(t *testing.T) {
	mustReject(t, "non-string tagline", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "tagline": 42,
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`, "tagline")
}

func TestValidateAcceptsMissingTagline(t *testing.T) {
	// tagline is optional — a manifest omitting it validates (baseGood has none).
	res := validateManifest(t, baseGood)
	if !res.OK() {
		t.Fatalf("manifest without tagline should validate, got: %v", res.Errors)
	}
}

func TestValidateRejectsUnknownScopeJustificationKey(t *testing.T) {
	// End-to-end through the semantic path: an otherwise-valid manifest that
	// justifies a scope it does not declare must be rejected, matching the
	// server rule that justifications for un-requested scopes are rejected.
	mustReject(t, "unknown scopeJustifications key", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["user:read:self"],
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"},
		"scopeJustifications": {"buzz:read:self": "I would like buzz"}
	}`, `scopeJustifications key "buzz:read:self"`)
}

func TestValidateAcceptsDeclaredScopeJustification(t *testing.T) {
	// Control: justifying a scope that IS declared passes end-to-end.
	mustAccept(t, "declared scopeJustifications key", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": ["user:read:self"],
		"page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"},
		"scopeJustifications": {"user:read:self": "needed to greet the signed-in user"}
	}`)
}

func TestValidateRejectsBadContentRating(t *testing.T) {
	mustReject(t, "bad contentRating", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "banana", "scopes": []
	}`, "contentRating")
}

func TestValidateRejectsDevSetIframeSrc(t *testing.T) {
	mustReject(t, "dev-set iframe.src", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"src": "https://evil.example.com", "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts"}
	}`, "iframe.src is server-owned")
}

func TestValidateRejectsDevSetTrustTier(t *testing.T) {
	mustReject(t, "dev-set trustTier", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "trustTier": "internal"
	}`, "trustTier is server-owned")
}

func TestValidateRejectsNonIntegerBuzzBudget(t *testing.T) {
	mustReject(t, "non-integer buzzBudgetPerGen", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"page": {"path": "/", "title": "X", "buzzBudgetPerGen": 1.5}
	}`, "buzzBudgetPerGen")
}

func TestValidateRejectsNonPositiveBuzzBudget(t *testing.T) {
	mustReject(t, "non-positive buzzBudgetPerGen", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"page": {"path": "/", "title": "X", "buzzBudgetPerGen": 0}
	}`, "buzzBudgetPerGen")
}

func TestValidateRejectsBuildCommandWithoutOutputDir(t *testing.T) {
	mustReject(t, "buildCommand without outputDir", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "buildCommand": "npm run build"
	}`, "outputDir")
}

func TestValidateRejectsBadSemver(t *testing.T) {
	mustReject(t, "bad version", `{
		"blockId": "ok-block", "version": "v1", "name": "X",
		"contentRating": "g", "scopes": []
	}`, "version")
}

func TestValidateRejectsPagePathNotSlash(t *testing.T) {
	mustReject(t, "page.path no slash", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"page": {"path": "home", "title": "X"}
	}`, "path")
}

func TestValidateMissingManifest(t *testing.T) {
	res, err := Dir(t.TempDir())
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if res.OK() {
		t.Fatal("empty dir should fail validation")
	}
}

func mustAccept(t *testing.T, name, json string) {
	t.Helper()
	res := validateManifest(t, json)
	if !res.OK() {
		t.Fatalf("%s: expected valid, got errors: %v", name, res.Errors)
	}
}

// --- 🔴 Sandbox trust-tier allowlist (validator validateSandbox ~L175-206) ---

func TestValidateRejectsSandboxSameOriginWithScripts(t *testing.T) {
	// The marquee sandbox-escape combo. MUST be rejected (mutation-check target).
	mustReject(t, "allow-same-origin allow-scripts", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-same-origin allow-scripts"}
	}`, "allow-same-origin")
}

func TestValidateRejectsSandboxAllowSameOriginToken(t *testing.T) {
	// allow-same-origin alone is also outside the unverified allowlist.
	mustReject(t, "allow-same-origin token", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-same-origin"}
	}`, "not allowed for unverified")
}

func TestValidateRejectsSandboxTopNavigation(t *testing.T) {
	mustReject(t, "allow-top-navigation", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-top-navigation"}
	}`, "allow-top-navigation")
}

func TestValidateRejectsSandboxPopups(t *testing.T) {
	// allow-popups is allowed for verified/internal but NOT unverified.
	mustReject(t, "allow-popups", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-popups"}
	}`, "allow-popups")
}

func TestValidateAcceptsSandboxUnverifiedAllowlist(t *testing.T) {
	mustAccept(t, "allow-scripts allow-forms", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

// --- 🟡 page ⇒ iframe required (validator ~L504) ---

func TestValidateRejectsPageWithoutIframe(t *testing.T) {
	mustReject(t, "page without iframe", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"}
	}`, "must also declare an iframe block")
}

// --- 🟡 renderMode=iframe ⇒ iframe required (validator ~L377) ---

func TestValidateRejectsIframeModeWithoutIframe(t *testing.T) {
	// Default renderMode is iframe; omitting the iframe block must fail.
	mustReject(t, "default renderMode without iframe", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": []
	}`, "iframe block is required for renderMode=iframe")
}

// --- 🟡 iframe required sub-fields (validator ~L387-415) ---

func TestValidateRejectsIframeMissingMinHeight(t *testing.T) {
	mustReject(t, "iframe missing minHeight", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"resizable": true, "sandbox": "allow-scripts"}
	}`, "iframe.minHeight is required")
}

func TestValidateRejectsIframeMissingResizable(t *testing.T) {
	mustReject(t, "iframe missing resizable", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "sandbox": "allow-scripts"}
	}`, "iframe.resizable is required")
}

// --- 🟡 renderMode tier gate (validator ~L289) ---

func TestValidateRejectsInlineRenderMode(t *testing.T) {
	mustReject(t, "inline renderMode", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "renderMode": "inline",
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"}
	}`, "INLINE_REQUIRES_VERIFIED_TIER")
}

func TestValidateRejectsHybridRenderMode(t *testing.T) {
	mustReject(t, "hybrid renderMode", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [], "renderMode": "hybrid",
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"}
	}`, "INLINE_REQUIRES_VERIFIED_TIER")
}

// --- 🟡 targets[].slotId registry check (validator ~L426-460) ---

func TestValidateRejectsUnknownTargetSlot(t *testing.T) {
	mustReject(t, "unknown slot", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"},
		"targets": [{"slotId": "model.bogus_slot"}]
	}`, "is not a known slot")
}

func TestValidateRejectsPageSlotInTargets(t *testing.T) {
	mustReject(t, "page slot in targets", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"},
		"targets": [{"slotId": "app.page"}]
	}`, "is the page slot")
}

func TestValidateAcceptsKnownModelTargetSlot(t *testing.T) {
	mustAccept(t, "known model slot", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"},
		"targets": [{"slotId": "model.sidebar_top"}]
	}`)
}

// --- 🟡 false-INVALID fixes ---

func TestValidateAcceptsLongName(t *testing.T) {
	// The server only requires a non-empty name (no length cap). A 200-char
	// name that the server accepts must not be rejected by the CLI.
	long := strings.Repeat("a", 200)
	mustAccept(t, "200-char name", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "`+long+`",
		"contentRating": "g", "scopes": [], "page": {"path": "/", "title": "X"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts allow-forms"}
	}`)
}

func TestValidateRejectsOutputDirLeadingSlash(t *testing.T) {
	mustReject(t, "outputDir leading slash", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"},
		"buildCommand": "npm run build", "outputDir": "/etc/passwd"
	}`, "relative path")
}

func TestValidateRejectsOutputDirTraversal(t *testing.T) {
	mustReject(t, "outputDir traversal", `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"},
		"buildCommand": "npm run build", "outputDir": "../../escape"
	}`, "path traversal")
}

// --- 🔴 buildCommand allowlist (mirrors server BUILD_COMMAND_RE /
// BUILD_COMMAND_MAX_LENGTH; a friendly Go message on top of the schema pattern) ---

// buildManifest wraps a buildCommand + valid outputDir (buildCommand requires
// outputDir) in an otherwise-good manifest.
func buildManifest(buildCommand string) string {
	return `{
		"blockId": "ok-block", "version": "0.1.0", "name": "X",
		"contentRating": "g", "scopes": [],
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"},
		"buildCommand": ` + strconv.Quote(buildCommand) + `, "outputDir": "dist"
	}`
}

func TestValidateAcceptsAllowlistedBuildCommands(t *testing.T) {
	for _, bc := range []string{
		"npm run build",
		"pnpm run build",
		"yarn run test:e2e",
		"vite build",
		"npx vite build",
	} {
		mustAccept(t, "buildCommand "+bc, buildManifest(bc))
	}
}

func TestValidateRejectsNonAllowlistedBuildCommands(t *testing.T) {
	for _, bc := range []string{
		"make all",
		"npm run build && rm -rf /",
		"bash -c x",
		"vite build --watch",
	} {
		mustReject(t, "buildCommand "+bc, buildManifest(bc), "not an allowed build invocation")
	}
}

func TestValidateRejectsOverlongBuildCommand(t *testing.T) {
	// A valid-shaped but >128-char buildCommand (long script name) is rejected
	// with the length message, mirroring the server's max length.
	long := "npm run " + strings.Repeat("a", 130)
	mustReject(t, "overlong buildCommand", buildManifest(long), "too long")
}
