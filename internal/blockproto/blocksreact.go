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
// CONSTRUCT THE SDK TRANSPORT AT MOUNT — during render, or in an effect that
// runs on mount. It is a VENDORED MIRROR of SDK source, in the same sense as
// ackingPackages above.
//
// 🔴 WHY THIS EXISTS: DEPENDING ON THE SDK IS NOT THE SAME AS ACKING. The SDK's
// `IframeTransport` sends BLOCK_HELLO and answers BLOCK_INIT with BLOCK_READY —
// but only once something calls `getTransport()`, which builds it lazily on
// first call (`src/transport/singleton.ts`). A page app whose only SDK use is
// `useBuzzWorkflow` calls `getTransport()` exclusively INSIDE the callbacks it
// returns (estimate/submit/poll/cancel), so nothing constructs the transport at
// mount, BLOCK_READY never goes out, and the host shows "This app didn't load
// in time" after 10 s, retries twice and fails. Verified live on a production
// app that passed `civitai app validate` and `app submit`, because the
// dependency alone used to end the ready-ack check.
//
// DERIVED FROM SOURCE, NOT GUESSED: civitai/civitai-app-starters
// `packages/civitai-blocks-react/src`, @civitai/blocks-react 0.71.0 (origin/main
// 14bde2eb, 2026-10-12). Every VALUE export of the package's `.` and `./ui`
// entry points was classified by where it calls `getTransport()`:
//
//	render — getTransport() directly, via the internal useTransportSnapshot
//	         (useSyncExternalStore), or in a useMemo initialiser:
//	  getTransport, useBlockContext, useBlockTheme, useBlockSettings,
//	  useBlockToken, useHostOrigin, useCivitaiRoute, useDomainMaturity,
//	  useDirectLoad, useAppStorage, useSharedStorage
//	render, via one of the above:
//	  useEntitlements, useGenerationResources, useGoodPurchase, useTip,
//	  useTipAllowance, SfwGate (useDomainMaturity), BlockGate (useDirectLoad),
//	  TipButton (useTip)
//	mount effect — a useEffect that calls getTransport() (some conditionally,
//	  see below):
//	  useViewer, useBuzzBalance, useBuzzAccounts, useBuzzTransactions,
//	  useAppWorkflows, useDailyCompensation, useWildcardPack,
//	  useConsentUnavailable, useImageUpload, useBlockResize, useWorkflowBatch
//	constructs a transport by hand:
//	  IframeTransport, BlockTransportDetector
//
// and, deliberately ABSENT because they reach getTransport() ONLY inside a
// caller-invoked callback: useBuzzWorkflow, useBatchEstimate, useBuzzPurchase,
// useBlockAnalytics, useCivitaiNavigate, useRequestSignIn, useRequestConsent,
// useCollectionFollow, useCreatePostFromApp, usePrepareTrainingDataset,
// useRunTraining, useRunConfirmedStep, useCheckpointPicker, useResourcePicker,
// useUploadImageBytes, usePublishGenerationOutputs, useGatedImages,
// useSaveImage, FollowButton; and useBlockBreakpoint / useNestedDocument, which
// never touch the transport at all.
//
// 🔴 THE SET ERRS GENEROUS, AND THAT IS THE CHEAP DIRECTION. A name wrongly IN
// the set can only SILENCE the finding (a false negative); a name wrongly OUT
// of it warns at a correct app, the expensive direction this whole check is
// built to avoid. So the conditional mount effects are IN: useWildcardPack
// skips the request for a non-positive id, useBlockResize only once its ref is
// attached, useWorkflowBatch only with ids to watch — an app relying on one of
// those alone may still never connect, and goes unflagged.
//
// Residuals, stated: an older SDK whose hook connected LAZILY where 0.71.0's
// connects at mount is silenced by this set (false negative). A newer SDK that
// connects eagerly at import would make the finding a false warning on apps
// using only lazy hooks — re-derive this set (and the check's gate) when that
// ships. Adding a name here is a claim about SDK RUNTIME code: read the hook's
// body, not its docs, and record the version.
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
	// mount effect
	"useViewer": true, "useBuzzBalance": true, "useBuzzAccounts": true,
	"useBuzzTransactions": true, "useAppWorkflows": true, "useDailyCompensation": true,
	"useWildcardPack": true, "useConsentUnavailable": true, "useImageUpload": true,
	"useBlockResize": true, "useWorkflowBatch": true,
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
