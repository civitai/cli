package scaffold

import (
	"fmt"
	"strings"
)

// The COMPLETE index of `@civitai/blocks-react` hooks that the page-money
// scaffold's README ships.
//
// 🔴 WHY IT EXISTS: A HOOK THAT IS DOCUMENTED NOWHERE LOCALLY IS A HOOK AN AGENT
// REVERSE-ENGINEERS FROM `.d.ts`. Measured on a blind dogfood run (one-line
// brief: "generate an image and post it to Civitai"): `useCreatePostFromApp` —
// the ONE hook that brief needed — appeared in no scaffolded README, in no
// AGENTS.md, and nowhere in the 5,908 lines of scaffolded `src/`. The agent
// found it by reading `node_modules/@civitai/blocks-react/dist/*.d.ts`, at a
// cost of 706,371 carried tokens — 28.2% of the whole run — and wrote no code in
// 66 steps. The old README named the SIX hooks the sample itself calls, which is
// a fine description of the SAMPLE and a terrible index of the SDK.
//
// 🔴 SO THE INDEX IS THE WHOLE EXPORT SURFACE, NOT THE SAMPLE'S SUBSET. The
// claim this table makes is "these are all of them", and
// TestHookIndexMatchesThePublishedSDK (network-gated) is what keeps it a checked
// claim: it resolves the published `@civitai/blocks-react` the page-money pin
// admits, extracts every `use*` VALUE export from its `dist/index.d.ts`, and
// fails when this table and that set differ in EITHER direction. A hook added
// upstream fails here until it is written down; a row for a hook that was
// removed fails too.
//
// 🔴 THE NAMES ARE DERIVED; THE CLAUSES ARE NOT, AND THAT IS DELIBERATE. The
// `.d.ts` carries no one-line summary a machine can lift without producing prose
// that reads like a type signature, so `Summary` is written by hand from the
// published reference (developer.civitai.com/apps/reference/hooks.md, itself
// generated from the same package) and the hook's own JSDoc. The DRIFT RISK is
// therefore confined to the clause: the guard above can tell you a row is
// missing or stale in its NAME, and cannot tell you a clause has stopped being
// true. Keep the clauses short enough that they only state what the hook is for.

// HookRow is one row of the index: the hook, what it is for, and the manifest
// scope it needs.
type HookRow struct {
	// Name is the exported identifier, without the call parens.
	Name string
	// Summary is one clause — what the hook is FOR. Not a signature.
	Summary string
	// Scope is the `block.manifest.json` scope the hook needs, or "" when the
	// SDK's own types name none.
	//
	// 🔴 "" MEANS "THE SDK NAMES NONE", NOT "PROVABLY NONE". Most of these hooks
	// are host-mediated — the host resolves the viewer from the block token and
	// performs the privileged call itself — so no scope is the normal answer.
	// But the column is only as good as what the published types say, and a few
	// hooks document no scope while plainly touching private data. The rendered table
	// says so in its own footnote rather than letting a blank read as a
	// guarantee.
	Scope string
}

// SDKHooks is every `use*` value export of `@civitai/blocks-react`, grouped the
// way an author meets them.
//
// The ORDER is the document's; the SET is the SDK's. Grouping is by what the
// author is trying to do, because an index sorted alphabetically answers "how is
// it spelled" — which is the question you can already answer — instead of "what
// is there", which is the one that cost the dogfood run its budget.
var SDKHooks = []HookRow{
	// --- Context and plumbing: no scope, needed by every app. ---
	{"useBlockContext", "the host's `BLOCK_INIT` snapshot plus a `ready` gate — every other hook builds on it", ""},
	{"useBlockTheme", "the host's current site theme, kept live", ""},
	{"useBlockSettings", "the publisher + user settings the host forwarded (read-only in the iframe)", ""},
	{"useBlockToken", "the current block-scoped JWT, auto-refreshed, plus a `refresh()` for the 401 retry", ""},
	{"useHostOrigin", "the validated host origin to direct-fetch the App Blocks HTTP API against", ""},
	{"useBlockResize", "report your root element's height so the host sizes the iframe to fit", ""},
	{"useBlockBreakpoint", "your OWN box's width tier — slot width is not viewport width", ""},
	{"useDirectLoad", "detect an unembedded top-level load so you can show an \"Open on Civitai\" fallback", ""},
	{"useBlockAnalytics", "fire-and-forget event tracking into the host's analytics pipeline", ""},
	{"useCivitaiNavigate", "ask the host to navigate within civitai.com", ""},
	{"useRequestSignIn", "ask the host to open sign-in for an anonymous viewer", ""},
	{"useRequestConsent", "ask the host to open its consent UI for a scope the token is missing", ""},
	{"useConsentUnavailable", "the host's push saying a scope can NEVER be granted in this environment", ""},
	{"useDomainMaturity", "the maturity ceiling in force for this viewer (fail-closed SFW)", ""},

	// --- Generation and money. ---
	{"useBuzzWorkflow", "estimate → submit → poll a generation: the Buzz spend itself", "ai:write:budgeted"},
	{"useBuzzPurchase", "open the host's Buzz purchase modal — the insufficient-budget recovery path", ""},
	{"useBuzzBalance", "the viewer's spendable per-pool balance (`{ blue, green, yellow }`)", ""},
	{"useBuzzAccounts", "all-pool balances, including the creator payout pools", "buzz:read:self"},
	{"useBuzzTransactions", "the viewer's paged Buzz-transaction ledger", "buzz:read:self"},
	{"useDailyCompensation", "per-modelVersion generation compensation for one month", "buzz:read:self"},
	{"useViewer", "an authoritative self-read of the signed-in viewer (id, username, status, budget)", "user:read:self"},
	{"useAppWorkflows", "THIS app's own generator subqueue, newest-first, plus a fail-closed `cancel`", "ai:write:budgeted"},
	{"useTip", "send a Buzz tip from the viewer", "social:tip:self"},
	{"useTipAllowance", "the viewer's remaining daily tip allowance", "social:tip:self"},

	// --- Resources and media: the host owns the catalog and the bytes. ---
	{"useCheckpointPicker", "open the host's Checkpoint picker and persist the viewer's override", ""},
	{"useResourcePicker", "open the host's resource picker (Checkpoint or LORA) — the app never browses a catalog", ""},
	{"useGenerationResources", "rehydrate saved resources by version id without re-opening a picker", ""},
	{"useImageUpload", "host-mediated image upload — the iframe never handles the bytes", ""},
	{"useGatedImages", "per-viewer gated display data for a list of image ids", ""},
	{"useSaveImage", "ask the host to download an image for the viewer", ""},
	{"useWildcardPack", "import a wildcard pack's parsed prompt lists by model version", ""},

	// --- Publishing back to Civitai. ---
	{"useCreatePostFromApp", "create a Civitai POST from a generated image", "posts:write:self"},
	{"usePublishGenerationOutputs", "publish this app's own generation outputs as public `Image` rows", ""},
	{"useCollectionFollow", "bookmark (follow) a collection on the viewer's behalf", "collections:write:self"},

	// --- Storage. ---
	{"useAppStorage", "per-(install, viewer) private KV store, host-mediated", "apps:storage:read + apps:storage:write"},
	{"useSharedStorage", "app-wide append-only SHARED list with votes (every viewer sees the same rows)", "apps:storage:shared:read + apps:storage:shared:write"},
}

// consentGatedScopes are the scopes a manifest declaration does NOT grant: the
// token is minted WITHOUT them and the viewer has to consent before the host
// adds them. Naming them in the index is the point — "I declared it and still
// get a 403" is the single most expensive surprise on this platform, and the
// hooks that hit it are exactly the ones an author reaches for first.
var consentGatedScopes = map[string]bool{
	"ai:write:budgeted": true,
	"posts:write:self":  true,
}

// hookIndexHeading is the section's heading in the rendered README. Tests locate
// the block by it, so it is a const rather than a literal in the renderer.
const hookIndexHeading = "## Every hook the SDK exports"

// RenderHookIndex renders the index as markdown.
//
// 🔴 IT IS RENDERED INTO THE README AT SCAFFOLD TIME, NOT PASTED INTO THE
// TEMPLATE. `SDKHooks` is then the ONE place the list lives: there is no second
// copy in `README.md.tmpl` to fall behind it, and no generator to remember to
// run. The template says `{{ .HookIndex }}` and `Render` fills it in.
func RenderHookIndex() string {
	var b strings.Builder
	b.WriteString(hookIndexHeading + "\n\n")
	b.WriteString("This is the COMPLETE list of `@civitai/blocks-react` hooks — not just the ones\n")
	b.WriteString("this sample happens to call. If a capability is not here, it is not a hook:\n")
	b.WriteString("check the reference before concluding the platform cannot do it.\n\n")
	b.WriteString("| Hook | What it is for | Manifest scope |\n|---|---|---|\n")
	for _, h := range SDKHooks {
		scope := "—"
		if h.Scope != "" {
			scope = "`" + strings.ReplaceAll(h.Scope, " + ", "` + `") + "`"
			if consentGatedScopes[h.Scope] {
				scope += " (consent-gated)"
			}
		}
		fmt.Fprintf(&b, "| `%s()` | %s | %s |\n", h.Name, h.Summary, scope)
	}
	b.WriteString("\n**A declared scope is not a granted scope.** The consent-gated ones are dropped\n")
	b.WriteString("from the token until the viewer consents, so you get a 403 while the manifest\n")
	b.WriteString("and the runtime both look correct — call `useRequestConsent()` and retry.\n")
	b.WriteString("A `—` means the SDK's own types name no scope: those hooks are host-mediated\n")
	b.WriteString("(the host resolves the viewer from your block token). Confirm against\n")
	b.WriteString("<https://developer.civitai.com/apps/reference/hooks.md> before relying on it.\n")
	return b.String()
}

// HookNames returns the indexed hook names, for guards that compare the index
// against a set derived from somewhere else.
func HookNames() []string {
	out := make([]string, 0, len(SDKHooks))
	for _, h := range SDKHooks {
		out = append(out, h.Name)
	}
	return out
}
