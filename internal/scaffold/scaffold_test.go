package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRenderStatic(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "my-block")
	written, err := Render(Static, dest, Data{Slug: "my-block", Name: "My Block"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	want := []string{
		".gitignore",
		"README.md",
		"app.js",
		"block.manifest.json",
		// The vendored block -> host ready-ack emitter, copied verbatim from
		// internal/blockproto (see ready_ack_contract_test.go).
		"civitai-host.js",
		"index.html",
		"style.css",
	}
	got := relNames(t, dest, written)
	assertSameSet(t, want, got)

	// Manifest must contain the slug + name and NOT carry build fields.
	manifest := readFile(t, filepath.Join(dest, "block.manifest.json"))
	mustContain(t, manifest, `"blockId": "my-block"`)
	mustContain(t, manifest, `"name": "My Block"`)
	if contains(manifest, "buildCommand") {
		t.Error("static template manifest should not declare buildCommand")
	}
}

func TestRenderPageVite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "vite-block")
	written, err := Render(PageVite, dest, Data{Slug: "vite-block", Name: "Vite Block"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := relNames(t, dest, written)
	for _, expect := range []string{
		"block.manifest.json", "package.json", "vite.config.js", "index.html",
		"src/main.jsx", "src/App.jsx", "src/index.css", "README.md", ".gitignore",
	} {
		if !containsStr(got, expect) {
			t.Errorf("page-vite missing expected file %q (got %v)", expect, got)
		}
	}

	manifest := readFile(t, filepath.Join(dest, "block.manifest.json"))
	mustContain(t, manifest, `"buildCommand": "npm run build"`)
	mustContain(t, manifest, `"outputDir": "dist"`)

	// package.json name should use the slug, carry a postinstall next-step hint,
	// and stay valid JSON after rendering.
	pkg := readFile(t, filepath.Join(dest, "package.json"))
	mustContain(t, pkg, `"name": "vite-block"`)
	mustContain(t, pkg, `"postinstall"`)
	mustContain(t, pkg, "npm run dev")
	if !json.Valid([]byte(pkg)) {
		t.Errorf("page-vite package.json is not valid JSON:\n%s", pkg)
	}
}

func TestRenderPageMoney(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "money-block")
	written, err := Render(PageMoney, dest, Data{Slug: "money-block", Name: "Money Block"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := relNames(t, dest, written)
	for _, expect := range []string{
		"block.manifest.json", "package.json", "vite.config.ts", "tsconfig.json", "index.html",
		"src/main.tsx", "src/App.tsx", "src/Harness.tsx", "src/generation.ts",
		"src/models.ts", "src/nav.ts", "src/setup-dev-live.ts",
		"vite-plugin-civitai-setup.ts",
		"src/comfy.ts",
		"src/generation.test.ts", "src/models.test.ts", "src/nav.test.ts",
		"src/comfy.test.ts",
		"src/setup-dev-live.test.ts", "src/setup-plugin.test.ts",
		"src/App.pollretry.test.tsx", "src/index.css",
		"README.md", ".gitignore", ".env.development", ".env.production", ".env.example",
	} {
		if !containsStr(got, expect) {
			t.Errorf("page-money missing expected file %q (got %v)", expect, got)
		}
	}
	// The in-block catalog browser is gone — the HOST serves the picker now
	// (useCheckpointPicker / useResourcePicker), so catalog-api.ts + its test
	// must NOT be scaffolded.
	for _, gone := range []string{"src/catalog-api.ts", "src/catalog-api.test.ts"} {
		if containsStr(got, gone) {
			t.Errorf("page-money should NOT scaffold %q anymore (host serves the picker)", gone)
		}
	}

	// Money-path manifest: budgeted scope + per-gen budget + build fields.
	//
	// buzzBudgetPerGen is a SAFETY CEILING — the most a malicious or exploited
	// app could spend per generation — NOT a forecast of what a generation costs.
	// It must sit far ABOVE expected spend: an over-budget generation is refused
	// at a pre-submit gate (nothing runs, nothing is charged) but the app is then
	// broken for every user until a new manifest version ships AND is re-approved.
	// Headroom, by contrast, is never actually spent — the server re-prices and
	// hard-caps every generation at the real per-job ceiling.
	//
	// 300 = 10x the scaffold's worst case (30, the starter Comfy on Civitai
	// recipe's hard per-job ceiling; the txt2img sample is cheaper). The old 40
	// was worst-case+10, which read as an estimate and taught authors to size it
	// like one. Keep this in sync with PAGE_BUZZ_BUDGET in src/generation.ts.
	manifest := readFile(t, filepath.Join(dest, "block.manifest.json"))
	mustContain(t, manifest, `"ai:write:budgeted"`)
	mustContain(t, manifest, `"buzzBudgetPerGen": 300`)
	mustContain(t, manifest, `"buildCommand": "npm run build"`)
	mustContain(t, manifest, `"outputDir": "dist"`)
	// Server-owned fields must NOT be set.
	if contains(manifest, "iframe.src") || contains(manifest, `"src"`) {
		t.Error("page-money manifest must not declare iframe.src")
	}
	if contains(manifest, "trustTier") {
		t.Error("page-money manifest must not declare trustTier")
	}

	// package.json: SDK deps + the dev:harness script + slug name + a postinstall
	// next-step hint, all while staying valid JSON after rendering.
	pkg := readFile(t, filepath.Join(dest, "package.json"))
	mustContain(t, pkg, `"@civitai/blocks-react"`)
	// Per-account Buzz (useBuzzBalance / body.accountType / snapshot.spentAccountType)
	// needs @civitai/blocks-react@0.18.0+ (createMockHost answers GET_BUZZ_BALANCE
	// from the `buzzBalance` option AND stamps spentAccountType on succeeded
	// snapshots natively — so the scaffold no longer ships a mock balance shim) +
	// @civitai/app-sdk@0.14.0+. The pins track the CURRENT published minors — the
	// pins-vs-published guard (TestScaffoldPinsSatisfyPublished) fails CI if they
	// fall behind npm, so bump BOTH assertions here in lockstep with the .tmpl.
	mustContain(t, pkg, `"@civitai/blocks-react": "^0.39.0"`)
	mustContain(t, pkg, `"@civitai/app-sdk": "^0.31.0"`)
	mustContain(t, pkg, `"@civitai/app-sdk"`)
	mustContain(t, pkg, `"dev:harness"`)
	mustContain(t, pkg, `"build"`)
	mustContain(t, pkg, `"test"`)
	mustContain(t, pkg, `"name": "money-block"`)
	mustContain(t, pkg, `"postinstall"`)
	mustContain(t, pkg, "dev:harness")
	if !json.Valid([]byte(pkg)) {
		t.Errorf("page-money package.json is not valid JSON:\n%s", pkg)
	}

	// App.tsx must use the SDK, never raw postMessage('*').
	app := readFile(t, filepath.Join(dest, "src", "App.tsx"))
	mustContain(t, app, "useRequestConsent")
	mustContain(t, app, "useBuzzWorkflow")
	mustContain(t, app, "useBlockResize")
	// The model + LoRA controls drive the HOST's native pickers via the SDK hooks
	// (the in-block catalog browser is gone).
	mustContain(t, app, "useCheckpointPicker")
	mustContain(t, app, "useResourcePicker")
	if contains(app, "window.parent.postMessage") {
		t.Error("page-money App.tsx should not use raw window.parent.postMessage")
	}
	// The poll loop must retry transient transport errors (a throw), not turn
	// them into a terminal failure — guards the round-5 dogfood regression where
	// a poll blip marked a server-side SUCCESS as FAILED.
	mustContain(t, app, "MAX_TRANSIENT_ERRORS")
	mustContain(t, app, "consecutiveErrors")

	// The confirm-spend modal was removed: Generate is one click → consent →
	// spend (the button already shows the cost).
	mustNotContain(t, app, "Confirm generation")
	mustNotContain(t, app, "setConfirmOpen")
	// The Generate click runs the gate directly (no intermediate confirm state).
	mustContain(t, app, "proceed()")

	// The "About this block" modal + the "How this works" section/button were
	// removed (a clean, minimal scaffold). The status Badge near the title was
	// removed too — but its underlying consent/auth logic (granted/anon) stays.
	mustNotContain(t, app, "About this block")
	mustNotContain(t, app, "aboutOpen")
	mustNotContain(t, app, "How this works")
	mustNotContain(t, app, "How it works")
	mustNotContain(t, app, "signed out") // the removed status-Badge copy
	mustNotContain(t, app, "needs access")
	// The load-bearing consent/auth logic the removed Badge merely surfaced MUST
	// remain — it's what makes the first Generate request consent.
	mustContain(t, app, "hasBudgetedScope")
	mustContain(t, app, "requestConsent")

	// Model control: the page ships a curated default checkpoint + a "Change
	// model" button that opens the HOST's checkpoint picker (entity=none means no
	// host model context). The old hardcoded GEN_MODEL + the in-block <select> are
	// gone; the pick threads into the workflow body. A pick is DISCOVERY ONLY.
	mustNotContain(t, app, "GEN_MODEL")
	mustNotContain(t, app, "pm-model-select") // the old in-block <select> is gone
	mustNotContain(t, app, "fetchCatalog")    // the in-block catalog browser is gone
	mustContain(t, app, "DEFAULT_CHECKPOINT")
	mustContain(t, app, "pm-change-model")
	mustContain(t, app, "openCheckpointPicker(")
	mustContain(t, app, "checkpointFromPick")
	mustContain(t, app, "buildWorkflowBody(prompt, checkpoint, loras, account)")

	// Per-account Buzz: the viewer's 3-pool balance is read host-side via
	// useBuzzBalance, and an account picker threads the chosen pool into the body
	// (Auto = omit accountType = unchanged behavior). spentAccountType is surfaced
	// after a successful generation.
	mustContain(t, app, "useBuzzBalance")
	mustContain(t, app, "AccountPicker")
	mustContain(t, app, "spentAccountType")
	mustContain(t, app, "pm-account-") // the picker radios' testid prefix

	// ...but the scaffold renders NO balance readout of its own: an App runs
	// inside the Civitai chrome, which already shows the viewer's balance, and
	// agents copy whatever the default template does. The balance is read ONLY to
	// annotate which account can fund the generation (the AccountPicker above).
	// Guarding the component + style symbols too, so re-adding a readout under a
	// different testid still trips this.
	mustNotContain(t, app, "pm-balance")
	mustNotContain(t, app, "BalancePanel")
	mustNotContain(t, app, "PoolChip")
	mustNotContain(t, app, "POOL_COLORS")

	// LoRA selector (the main feature): the user adds up to MAX_LORAS LoRAs on top
	// of the checkpoint via the HOST resource picker (type=LORA), each with a
	// weight, threaded into the body as additionalResources.
	mustContain(t, app, "LoraSelector")
	mustContain(t, app, "openResourcePicker(")
	mustContain(t, app, "resourceType: 'LORA'")
	mustContain(t, app, "loraFromPick")
	mustContain(t, app, "pm-lora-add")
	mustContain(t, app, "pm-lora-weight")
	mustContain(t, app, "addLora")

	// Comfy on Civitai (customComfy) sample: a mode toggle swaps the body-builder to
	// buildComfyBody (a server-registered recipe). The recipe id is FIXED +
	// server-registered; comfy.ts never sends a graph, only { kind, recipe, params }.
	comfy := readFile(t, filepath.Join(dest, "src", "comfy.ts"))
	mustContain(t, comfy, "customComfy")
	mustContain(t, comfy, "starter-comfy-txt2img")
	mustContain(t, comfy, "buildComfyBody")
	mustContain(t, app, "buildComfyBody(prompt, account)")
	mustContain(t, app, "SegmentedControl")
	mustContain(t, app, "pm-comfy-beta")
	mustContain(t, app, "pm-gated")

	// generation.ts threads the chosen checkpoint into modelId/modelVersionId
	// instead of a hardcoded constant, AND emits the selected LoRAs as
	// additionalResources (only when non-empty).
	gen := readFile(t, filepath.Join(dest, "src", "generation.ts"))
	mustNotContain(t, gen, "GEN_MODEL")
	mustContain(t, gen, "checkpoint: CheckpointOption")

	// SEAM: the budget lives in TWO files — the manifest (authoritative; the
	// server reads it at mint) and PAGE_BUZZ_BUDGET here (the app's own mirror).
	// Nothing at runtime reconciles them, so a drifted mirror silently misinforms
	// the author. Pin the RELATIONSHIP, not just each value: parse the number the
	// manifest actually emitted and require the constant to equal it, so raising
	// one alone fails.
	budget := budgetFromManifest(t, manifest)
	if budget != 300 {
		t.Errorf("manifest buzzBudgetPerGen = %d, want 300 (safety ceiling, not an estimate)", budget)
	}
	mustContain(t, gen, fmt.Sprintf("export const PAGE_BUZZ_BUDGET = %d;", budget))
	mustContain(t, gen, "modelVersionId: checkpoint.versionId")
	mustContain(t, gen, "loras: readonly LoraOption[]")
	mustContain(t, gen, "body.additionalResources = loras.map")

	// models.ts ships the verified DEFAULT checkpoint id the app starts on (Public
	// + generation-covered + SFW), the pick→option mappers (from the SDK's
	// BlockCheckpointInfo / BlockResourceInfo), PLUS the LoRA types + the selection
	// helpers (cap + weight clamp). The curated CHECKPOINTS/LORAS browse lists are
	// gone (the host serves the picker).
	models := readFile(t, filepath.Join(dest, "src", "models.ts"))
	mustContain(t, models, "CheckpointOption")
	mustContain(t, models, "DEFAULT_CHECKPOINT")
	mustContain(t, models, "128078") // SD XL 1.0 — the default checkpoint version id
	mustContain(t, models, "checkpointFromPick")
	mustContain(t, models, "loraFromPick")
	mustContain(t, models, "LoraOption")
	mustContain(t, models, "MAX_LORAS")
	mustContain(t, models, "clampLoraWeight")
	mustContain(t, models, "LORA_STRENGTH_MAX")
	// The catalog-fetch surface is gone from models.ts (host serves the picker).
	mustNotContain(t, models, "fetchCatalog")
	mustNotContain(t, models, "mergeCheckpoints")

	// nav.ts: the dev:live host-nav pure logic (profile name + Buzz balance
	// parsers + display logic). The balance parser must be tolerant of the tRPC
	// envelope + safe on garbage.
	nav := readFile(t, filepath.Join(dest, "src", "nav.ts"))
	mustContain(t, nav, "parseBuzzBalance")
	mustContain(t, nav, "parseViewerName")
	mustContain(t, nav, "navDisplay")
	// The personal key is NEVER referenced in client code — only the proxy
	// (vite.config.ts) injects it server-side.
	mustNotContain(t, nav, "CIVITAI_HOST_KEY")
	mustNotContain(t, nav, "VITE_LIVE_BLOCK_TOKEN")
	// Dev-token expiry surfacing: pure helpers that name the dead-token state so
	// an expired (~4h) token stops masquerading as feature bugs.
	mustContain(t, nav, "decodeTokenExp")
	mustContain(t, nav, "isTokenExpired")
	mustContain(t, nav, "shouldPromptReMint")

	// The display name should land in the manifest + App heading.
	mustContain(t, manifest, `"name": "Money Block"`)
	mustContain(t, app, "Money Block")

	// The LiveUnavailable screen (shown by dev:live with no token). The PRIMARY
	// path is now one-click "Set up automatically" (the dev-only vite plugin mints
	// + writes the env + auto-restarts); the manual 4-step command wizard is kept
	// as a FALLBACK behind a disclosure. Live mode always spends real Buzz, so
	// it's credential-first.
	mainTSX := readFile(t, filepath.Join(dest, "src", "main.tsx"))
	// Built on the component pack, not bespoke HTML + inline styles.
	mustContain(t, mainTSX, "@civitai/blocks-react/ui")
	mustContain(t, mainTSX, "injectBlocksStyles")

	// AUTO-SETUP (the primary path): an input + a "Set up automatically" button
	// that POSTs the pasted key to the dev-only plugin endpoint; on ok it reloads
	// into live mode (vite restart). The button + the endpoint are present.
	mustContain(t, mainTSX, "Set up automatically")
	mustContain(t, mainTSX, "/__civitai/setup-dev-live")
	mustContain(t, mainTSX, "setUpAutomatically")
	mustContain(t, mainTSX, "Setting up…")
	mustContain(t, mainTSX, "Done — restarting…")
	mustContain(t, mainTSX, "pm-setup-auto")
	// The personal key only travels in the POST body to localhost — the client
	// never references the NON-VITE_ CIVITAI_HOST_KEY var.
	mustContain(t, mainTSX, "apiKey: trimmedKey")

	// MANUAL FALLBACK (demoted behind a disclosure — the knowledge is kept, not
	// deleted): the original progressive command wizard.
	mustContain(t, mainTSX, "Prefer to run the commands yourself?")
	mustContain(t, mainTSX, "function ManualSetupSteps")
	// Progressive wizard: a step counter that advances automatically.
	mustContain(t, mainTSX, "function WizardStep")
	mustContain(t, mainTSX, "setStep(")
	// Wizard AUTO-ADVANCE: step 1 advances on a non-empty key (no Next button),
	// and steps 2 + 3 advance on the copy-icon click of that step's command
	// (CopyButton's onCopied, wired only when it's the current step).
	mustContain(t, mainTSX, "if (step === 1 && trimmedKey.length > 0) setStep(2)")
	mustContain(t, mainTSX, "onCopied")
	mustContain(t, mainTSX, "step === 2 ? () => setStep(3)")
	mustContain(t, mainTSX, "step === 3 ? () => setStep(4)")
	// The old per-step "Next" buttons are gone (advance is automatic now).
	mustNotContain(t, mainTSX, ">Next<")
	mustNotContain(t, mainTSX, "onClick={() => setStep(2)}")
	// Step 1 deeplinks to the Add-API-Key modal, pre-filled with an APP-SPECIFIC
	// token name + the minimal AI Services scope; the link is masked as the URL
	// (NOT "personal API key"), and there's an input to paste the key.
	mustContain(t, mainTSX, "<a href={apiKeyUrl}")
	mustContain(t, mainTSX, "civitai.com/user/account")
	mustNotContain(t, mainTSX, "personal API key</a>")
	mustContain(t, mainTSX, "App Money Block dev token")
	mustContain(t, mainTSX, "user/account?addApiKey=1")
	mustContain(t, mainTSX, "scope=AIServices")
	mustContain(t, mainTSX, "TextInput")
	mustContain(t, mainTSX, "setApiKey(")
	// Step 2 authenticates WITH the pasted key (not a bare placeholder line).
	mustContain(t, mainTSX, "'civitai login --token ' + (trimmedKey")
	// Step 3: the dev-token mint with the real slug, straight to the env file.
	mustContain(t, mainTSX, "civitai app dev-token money-block")
	mustContain(t, mainTSX, ".env.development.local")
	mustNotContain(t, mainTSX, "civitai app submit")
	mustContain(t, mainTSX, "clipboard?.writeText")
	// The mock-host escape hatch is still offered for free local dev.
	mustContain(t, mainTSX, "dev:harness")
	// The sticky LIVE banner was replaced by a minimal HOST NAV (profile name +
	// Buzz balance) — but the LIVE safety pill must NOT be lost.
	mustContain(t, mainTSX, "function HostNav")
	mustContain(t, mainTSX, "LIVE · spends real Buzz")
	mustNotContain(t, mainTSX, "LIVE HOST · spends REAL Buzz") // the old banner copy is gone
	// The nav reads the profile name with the BLOCK token and the balance through
	// the same-origin proxy (NO Authorization header on the balance fetch — the
	// proxy injects the personal key server-side).
	mustContain(t, mainTSX, "/api/v1/blocks/me")
	mustContain(t, mainTSX, "buzz.getBuzzAccount")
	mustContain(t, mainTSX, "parseBuzzBalance")
	mustContain(t, mainTSX, "parseViewerName")
	// The personal key is NEVER referenced in client code (main.tsx). Only the
	// vite proxy injects it server-side.
	mustNotContain(t, mainTSX, "CIVITAI_HOST_KEY")
	// Expired-dev-token surfacing: the nav captures the /blocks/me HTTP status
	// (a 401 = server-rejected token) + computes promptReMint, and renders a
	// re-mint banner with the dev-token mint command instead of silently
	// degrading to the neutral 'viewer'.
	mustContain(t, mainTSX, "meStatus")
	mustContain(t, mainTSX, "promptReMint")
	mustContain(t, mainTSX, "isTokenExpired")
	mustContain(t, mainTSX, "shouldPromptReMint")
	mustContain(t, mainTSX, `data-testid="pm-nav-remint"`)
	mustContain(t, mainTSX, `data-harness-banner="token-expired"`)
	mustContain(t, mainTSX, "Dev token expired — re-mint to restore your session")
	mustContain(t, mainTSX, "civitai app dev-token money-block")

	// Harness.tsx: the mock chrome reads as a console/terminal strip. The MOCK
	// banner is exactly "MOCK HOST · no real Buzz spent" and the scenario panel
	// is labelled "mock scenarios".
	harness := readFile(t, filepath.Join(dest, "src", "Harness.tsx"))
	mustContain(t, harness, "MOCK HOST · no real Buzz spent")
	mustNotContain(t, harness, "no real compute")
	mustContain(t, harness, "mock scenarios")

	// vite.config.ts: the credential split. The personal key (CIVITAI_HOST_KEY) is
	// read NON-VITE-prefixed server-side via loadEnv(..., '') and injected as the
	// Authorization header ONLY on the dedicated balance route — never bundled to
	// the client. The balance route is declared before the catch-all `/api`.
	vite := readFile(t, filepath.Join(dest, "vite.config.ts"))
	mustContain(t, vite, "loadEnv")
	mustContain(t, vite, "CIVITAI_HOST_KEY")
	mustContain(t, vite, "/api/trpc/buzz.getBuzzAccount")
	mustContain(t, vite, "authorization: `Bearer ${hostKey}`")
	// The balance route key must come BEFORE the catch-all `/api` so it wins.
	if strings.Index(vite, "/api/trpc/buzz.getBuzzAccount") > strings.Index(vite, "'/api':") {
		t.Error("vite.config.ts: the balance route must be declared before the catch-all '/api'")
	}
	// The dev-only auto-setup plugin is imported + wired into plugins.
	mustContain(t, vite, "vite-plugin-civitai-setup")
	mustContain(t, vite, "civitaiSetupPlugin")
	mustContain(t, vite, "civitaiSetupPlugin()")

	// The auto-setup vite plugin (Node side of the dev:live "Set up automatically"
	// button). It is dev-only (apply: 'serve' — never in `vite build`), registers
	// the POST /__civitai/setup-dev-live route, and mints via the local manifest
	// (no CLI shell-out, no global-auth mutation). The pasted personal key it
	// writes is CIVITAI_HOST_KEY — still a NON-VITE_ var (never bundled).
	plugin := readFile(t, filepath.Join(dest, "vite-plugin-civitai-setup.ts"))
	mustContain(t, plugin, "apply: 'serve'") // dev-only — never in build
	mustContain(t, plugin, "/__civitai/setup-dev-live")
	mustContain(t, plugin, "runDevLiveSetup")
	mustContain(t, plugin, "CIVITAI_HOST_KEY")
	mustContain(t, plugin, ".env.development.local")
	// It calls the SAME no-row local-manifest mint as `civitai app dev-token`.
	mustContain(t, plugin, "block.manifest.json")
	// Never log the pasted key.
	mustNotContain(t, plugin, "console.log(apiKey")

	// The pure, node-testable core lives in src/setup-dev-live.ts (so vitest's
	// node project + tsconfig pick it up; the plugin is a thin shell over it).
	setup := readFile(t, filepath.Join(dest, "src", "setup-dev-live.ts"))
	mustContain(t, setup, "mergeEnvFile")
	mustContain(t, setup, "manifestToMintRequest")
	mustContain(t, setup, "mapMintError")
	mustContain(t, setup, "mintDevToken")
	mustContain(t, setup, "runDevLiveSetup")
	mustContain(t, setup, "/api/v1/blocks/dev-token")
	mustContain(t, setup, "VITE_LIVE_BLOCK_TOKEN")
	mustContain(t, setup, "CIVITAI_HOST_KEY")

	// env.example documents CIVITAI_HOST_KEY as a NON-VITE_, dev-only secret that
	// goes in .env.development.local.
	envExample := readFile(t, filepath.Join(dest, ".env.example"))
	mustContain(t, envExample, "CIVITAI_HOST_KEY")
	mustContain(t, envExample, ".env.development.local")
	// It must be documented as a NON-VITE_ var (no VITE_CIVITAI_HOST_KEY alias).
	mustNotContain(t, envExample, "VITE_CIVITAI_HOST_KEY")

	// README docs the live-mode Buzz-balance recipe (#30) — the only working path
	// is the tRPC procedure with a bearer key (no public REST buzz endpoint).
	readme := readFile(t, filepath.Join(dest, "README.md"))
	mustContain(t, readme, "buzz.getBuzzAccount")
	mustContain(t, readme, "no public REST")
	// README docs the dev:live host nav + the credential split (CIVITAI_HOST_KEY).
	mustContain(t, readme, "CIVITAI_HOST_KEY")
	mustContain(t, readme, ".env.development.local")
	// README docs the host-served pickers (the in-block catalog browser is gone).
	mustContain(t, readme, "useCheckpointPicker")
	mustContain(t, readme, "useResourcePicker")
	// README docs the submission lifecycle + the withdraw escape hatch (#29).
	mustContain(t, readme, "Submission lifecycle")
	mustContain(t, readme, "civitai app withdraw")
	mustContain(t, readme, "pending")

	// SEAM (writer 3 of 3): the README is the THIRD writer of the budget number,
	// and the ONLY one with a drift history — when the manifest moved 40 -> 300 it
	// was the README that kept saying 40, while the manifest <-> generation.ts pair
	// stayed in sync on its own. A guard over only that pair would have stayed
	// green straight through the failure it exists to catch, so assert the README
	// too, against the value the RENDERED manifest actually emitted (never a
	// literal — a literal just relocates the drift into the test).
	//
	// Scope of this assertion: it catches the README naming a budget the manifest
	// does not emit. It does NOT police how the README explains the number; the
	// recipe's own per-job ceiling (30) is a different quantity and legitimately
	// appears alongside it.
	assertBudgetDocumented(t, readme, budget)
}

func TestPageMoneyNeedsHarness(t *testing.T) {
	if !PageMoney.NeedsHarness() {
		t.Error("page-money should need the dev harness")
	}
	if Static.NeedsHarness() || PageVite.NeedsHarness() {
		t.Error("static/page-vite should not need the dev harness")
	}
}

func TestOutputNameMapsEnvFiles(t *testing.T) {
	cases := map[string]string{
		"env.development.tmpl": ".env.development",
		"env.production.tmpl":  ".env.production",
		"env.example.tmpl":     ".env.example",
	}
	for in, want := range cases {
		if got := outputName(in); got != want {
			t.Errorf("outputName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(Static, dir, Data{Slug: "abc", Name: "Abc"}); err == nil {
		t.Fatal("expected error rendering into a non-empty dir")
	}
}

func TestParseTemplate(t *testing.T) {
	if _, err := ParseTemplate("static"); err != nil {
		t.Errorf("static should parse: %v", err)
	}
	if _, err := ParseTemplate("page-vite"); err != nil {
		t.Errorf("page-vite should parse: %v", err)
	}
	if _, err := ParseTemplate("page-money"); err != nil {
		t.Errorf("page-money should parse: %v", err)
	}
	if _, err := ParseTemplate("nope"); err == nil {
		t.Error("unknown template should error")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My Cool Block": "my-cool-block",
		"weird__Name!!": "weird-name",
		"already-slug":  "already-slug",
	}
	for in, want := range cases {
		got, err := Slugify(in)
		if err != nil {
			t.Errorf("Slugify(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := Slugify("a"); err == nil {
		t.Error("too-short name should error")
	}
}

func TestValidateSlug(t *testing.T) {
	if err := ValidateSlug("good-slug"); err != nil {
		t.Errorf("good-slug rejected: %v", err)
	}
	for _, bad := range []string{"ab", "Bad", "1starts", "-leading", "trailing-", "has space"} {
		if err := ValidateSlug(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// --- helpers ---

func relNames(t *testing.T, dest string, written []string) []string {
	t.Helper()
	var out []string
	for _, w := range written {
		rel, err := filepath.Rel(dest, w)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func assertSameSet(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("file count = %d, want %d\n got=%v\nwant=%v", len(got), len(want), got, want)
	}
	for _, w := range want {
		if !containsStr(got, w) {
			t.Errorf("missing expected file %q (got %v)", w, got)
		}
	}
}

// assertBudgetDocumented requires the rendered README to name the budget the
// manifest emitted. Matched on a word boundary so "300" cannot be satisfied by a
// substring of an unrelated number (4000, 3000, 1300), and reported with the
// numbers the README DOES carry so a failure is diagnosable without re-running.
func assertBudgetDocumented(t *testing.T, readme string, budget int) {
	t.Helper()
	want := strconv.Itoa(budget)
	if regexp.MustCompile(`\b` + want + `\b`).MatchString(readme) {
		return
	}
	t.Errorf("README does not document the manifest's buzzBudgetPerGen (%d) — "+
		"the manifest and the README disagree about the budget. "+
		"Standalone integers found in the README: %v",
		budget, uniqueInts(readme))
}

// uniqueInts lists the distinct standalone integers in a document, so a budget
// mismatch names the stale value instead of just saying "not found".
func uniqueInts(s string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range regexp.MustCompile(`\b\d{2,}\b`).FindAllString(s, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// budgetFromManifest parses page.buzzBudgetPerGen out of a RENDERED manifest, so
// the seam guard compares the value the scaffold actually emitted rather than a
// string the test happens to spell the same way.
func budgetFromManifest(t *testing.T, manifest string) int {
	t.Helper()
	var m struct {
		Page struct {
			BuzzBudgetPerGen *int `json:"buzzBudgetPerGen"`
		} `json:"page"`
	}
	if err := json.Unmarshal([]byte(manifest), &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if m.Page.BuzzBudgetPerGen == nil {
		t.Fatal("manifest has no page.buzzBudgetPerGen")
	}
	return *m.Page.BuzzBudgetPerGen
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !contains(haystack, needle) {
		t.Errorf("expected to contain %q\n--- content ---\n%s", needle, haystack)
	}
}

func mustNotContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if contains(haystack, needle) {
		t.Errorf("expected NOT to contain %q\n--- content ---\n%s", needle, haystack)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsStr(set []string, s string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}
