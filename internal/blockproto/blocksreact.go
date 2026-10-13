package blockproto

import (
	"regexp"
	"sort"
	"strings"
)

// BlocksReactPackage is the one acking package whose ack is CONDITIONAL on how
// the app uses it — see blocksReactMountConnecting.
const BlocksReactPackage = "@civitai/blocks-react"

// blocksReactMountConnecting is the set of `@civitai/blocks-react` exports that
// CONSTRUCT THE SDK TRANSPORT AT MOUNT, UNCONDITIONALLY — during render, or in
// an effect that runs on every mount — IN THE VERSIONS BEFORE THE FIX
// (civitai/civitai-app-starters#632, "every hook connects the transport at
// mount"). It is a VENDORED MIRROR of SDK source, in the same sense as
// ackingPackages above, and it is FROZEN: the SDK-mount check only runs for a
// lockfile-resolved version older than the first release carrying that fix
// (see internal/validate's blocksReactMountFixVersion), so later SDK changes
// never need to be re-derived into it.
//
// 🔴 WHY THIS EXISTS: IN THOSE VERSIONS, DEPENDING ON THE SDK IS NOT THE SAME AS
// ACKING. The SDK's `IframeTransport` sends BLOCK_HELLO and answers BLOCK_INIT
// with BLOCK_READY — but only once something calls `getTransport()`, which
// builds it lazily on first call (`src/transport/singleton.ts`). Before #632, a
// page app whose only SDK use was `useBuzzWorkflow` called `getTransport()`
// exclusively INSIDE the callbacks it returns (estimate/submit/poll/cancel), so
// nothing constructed the transport at mount, BLOCK_READY never went out, and
// the host showed "This app didn't load in time" after 10 s, retried twice and
// failed. Verified live on a production app that passed `civitai app validate`
// and `app submit`, because the dependency alone used to end the ready-ack
// check.
//
// DERIVED FROM SOURCE, NOT GUESSED: civitai/civitai-app-starters
// `packages/civitai-blocks-react/src`, @civitai/blocks-react 0.71.0 (origin/main
// 14bde2eb, 2026-10-12). Every VALUE export of the package's `.` and `./ui`
// entry points was classified by where it calls `getTransport()`, and every
// member below was re-read for an early return or guard in front of that call:
//
//	render — getTransport() directly, via the internal useTransportSnapshot
//	         (useSyncExternalStore), or in a useMemo initialiser:
//	  getTransport, useBlockContext, useBlockTheme, useBlockSettings,
//	  useBlockToken, useHostOrigin, useCivitaiRoute, useDomainMaturity,
//	  useDirectLoad, useAppStorage, useSharedStorage
//	render, via one of the above, called at the top of the component/hook:
//	  useEntitlements, useGenerationResources, useGoodPurchase, useTip,
//	  useTipAllowance, SfwGate (useDomainMaturity), BlockGate (useDirectLoad),
//	  TipButton (useTip)
//	mount effect — `useEffect(() => { refetch(); }, [refetch])`, or an effect
//	  body, that reaches getTransport() with no guard in front of it:
//	  useViewer, useBuzzBalance, useBuzzAccounts, useBuzzTransactions,
//	  useAppWorkflows, useDailyCompensation, useConsentUnavailable,
//	  useImageUpload
//	constructs a transport by hand:
//	  IframeTransport, BlockTransportDetector
//
// and, deliberately ABSENT:
//
//	callback-only — getTransport() is reached only inside a caller-invoked
//	  function: useBuzzWorkflow, useBatchEstimate, useBuzzPurchase,
//	  useBlockAnalytics, useCivitaiNavigate, useRequestSignIn, useRequestConsent,
//	  useCollectionFollow, useCreatePostFromApp, usePrepareTrainingDataset,
//	  useRunTraining, useRunConfirmedStep, useCheckpointPicker,
//	  useResourcePicker, useUploadImageBytes, usePublishGenerationOutputs,
//	  useGatedImages, useSaveImage, FollowButton
//	CONDITIONAL mount effect — connects at mount only on some inputs, and not
//	  in its no-op case (independently measured in civitai-app-starters#632):
//	  useWildcardPack (returns before the request for a non-positive id),
//	  useBlockResize (`if (!el) return;` before getTransport(), so nothing
//	  before its ref is attached), useWorkflowBatch (its watch loops start only
//	  for ids in the list, so nothing with an empty list)
//	never touches the transport: useBlockBreakpoint, useNestedDocument
//
// 🔴 A CONDITIONAL MEMBER IS A HOLE, NOT A HEDGE. An earlier revision listed the
// three conditional hooks "because a wrong member only silences" — but silence
// on the incident's own shape (an app whose only SDK use is one of them, in its
// no-op case) is exactly the false pass this check exists to remove. A name is
// in this set only if it connects on EVERY mount.
//
// Adding a name here is a claim about SDK RUNTIME code in a pre-fix version:
// read the hook's body, not its docs, and record the version.
var blocksReactMountConnecting = map[string]bool{
	// render
	"getTransport": true, "useBlockContext": true, "useBlockTheme": true,
	"useBlockSettings": true, "useBlockToken": true, "useHostOrigin": true,
	"useCivitaiRoute": true, "useDomainMaturity": true, "useDirectLoad": true,
	"useAppStorage": true, "useSharedStorage": true,
	// render, via one of the above
	"useEntitlements": true, "useGenerationResources": true, "useGoodPurchase": true,
	"useTip": true, "useTipAllowance": true, "SfwGate": true, "BlockGate": true,
	"TipButton": true,
	// unconditional mount effect
	"useViewer": true, "useBuzzBalance": true, "useBuzzAccounts": true,
	"useBuzzTransactions": true, "useAppWorkflows": true, "useDailyCompensation": true,
	"useConsentUnavailable": true, "useImageUpload": true,
	// constructs a transport by hand
	"IframeTransport": true, "BlockTransportDetector": true,
}

// BlocksReactMountConnecting returns the mount-connecting export names, sorted,
// for messages and tests.
func BlocksReactMountConnecting() []string {
	out := make([]string, 0, len(blocksReactMountConnecting))
	for name := range blocksReactMountConnecting {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// reMountConnecting matches any mount-connecting name as a WHOLE JavaScript
// identifier. The boundaries are identifier characters (`[A-Za-z0-9_$]`), not
// `\b`, because `$` is an identifier character `\b` does not know about.
// Member access (`SDK.useBlockContext`) is deliberately accepted: that is how a
// namespace import calls it.
var reMountConnecting = func() *regexp.Regexp {
	names := BlocksReactMountConnecting()
	for i, n := range names {
		names[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?:^|[^A-Za-z0-9_$])(?:` + strings.Join(names, "|") + `)(?:[^A-Za-z0-9_$]|$)`)
}()

// ReferencesMountConnectingAPI reports whether code — which the caller must
// already have COMMENT-STRIPPED (EntryGraphOptions.Inspect hands it over that
// way) — names any mount-connecting `@civitai/blocks-react` export as a whole
// identifier.
//
// It is a REFERENCE test, not a call test: an import with no call, or the name
// inside a string literal, satisfies it. Both are false negatives — the cheap
// direction — and requiring a call would warn at `import { useBlockContext as
// useCtx }`, a correct app.
func ReferencesMountConnectingAPI(code string) bool {
	return reMountConnecting.MatchString(code)
}
