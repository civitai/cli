package cmd

import (
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// The managed block's storage guidance (Gotchas section).
//
// It used to say "Shared storage has a REST path, and it is the one to prefer
// … the platform is consolidating on them". That stopped being true when the
// platform shipped the DOCUMENT STORE (`@civitai/sdk` 0.14.0
// `app.storage.collection`, the manifest's `storage` declaration, the
// `apps:storage` / `apps:storage:publish` scopes): every legacy store —
// `useAppStorage`, `useSharedStorage`, the SDK's `app.storage.get/set/…` and
// `app.sharedStorage`, and the `app-storage` / `shared-storage` REST routes — is
// deprecated in its favour (the SDK 0.14.0 README marks them so). An agent
// following the old bullet started new code on a deprecated store. Since
// 2026-11-10 those stores are shut down (civitai.com answers
// `legacy_store_removed`) and `@civitai/blocks-react` 0.81.0 removed the two
// hooks, so the bullet now says that instead of "they still work".
//
// This bullet is CONTENTS, not just an address, which AGENTS.md item 36 argues
// against. It REPLACES a contents bullet of the same kind rather than adding
// one, so it is not a new exception. Measured cost of the replacement: +48 bytes
// on every blocks-react-arm block and +32 on every `@civitai/sdk`-arm block
// (smallest shape 9,093 -> 9,141 B, +0.5%).

// wantStorageBulletHead and wantStorageBulletTail bracket the per-arm middle.
// Pinned WHOLE and whitespace-normalised: a guard on a word ("collection") is
// walkable by rewording into a sentence that still points at a legacy store.
const (
	wantStorageBulletHead = "**Store data in the document store, not the legacy stores.** Declare each " +
		"collection under the manifest's `storage` key, add the `apps:storage` scope (and the consent-gated " +
		"`apps:storage:publish` to create or edit shared documents), and call " +
		"`app.storage.collection<T>(name)` from `@civitai/sdk` 0.32.0 or later"
	// wantStorageBulletService follows the per-arm blocks-react clause. @civitai/sdk
	// 0.32.0 sends a collection to the platform's app-data service only when the
	// host's BLOCK_INIT carries `appData` AND the frame holds a block token —
	// i.e. `initialize()` called with no token. `initialize({ token })` never
	// does, and `where` / non-key sorts / imports stay on civitai.com (SDK
	// CHANGELOG 0.32.0). Counters, marks, `update` and document publish/unpublish
	// are not named: they were removed from App Storage (civitai #10391; the
	// server answers 410 `removed`), so they go nowhere.
	wantStorageBulletService = ", on the app that `initialize()` returns when called with no token " +
		"(transport mode). That lets the platform serve the app's own collections through its data " +
		"service; a `where` filter, a sort other than by key, and an import (`{ from }`) go to " +
		"civitai.com regardless"
	wantStorageBulletTail = "and the `/api/v1/blocks/app-storage/` and `/shared-storage/` routes are " +
		"shut down: civitai.com refuses every call with `legacy_store_removed`. Any storage scope alongside " +
		"`auth: \"oauth\"` is refused at submit. Guide: " + storageGuideURL
)

// wantStorageBullet is the whole bullet for one SDK arm. The blocks-react arm
// adds that its own package has no document-store hook and names the hooks;
// the `@civitai/sdk` arm names the SDK's legacy methods and never a hook
// (TestAgentsBlockPointsAtTheSDKTheProjectInstalled bans blocks-react names there).
func wantStorageBullet(elementsSDK bool) string {
	if elementsSDK {
		return wantStorageBulletHead + wantStorageBulletService + ". Test against `createFakeStorage` from " +
			"`@civitai/sdk/testing`. `app.storage.get/set/delete/list`, `app.sharedStorage` " + wantStorageBulletTail
	}
	return wantStorageBulletHead + " — `@civitai/blocks-react` has no document-store hook" +
		wantStorageBulletService + ". Test against " +
		"`createFakeStorage` from `@civitai/sdk/testing`. `useAppStorage()`, `useSharedStorage()` " +
		"(removed in `@civitai/blocks-react` 0.81.0) " + wantStorageBulletTail
}

// TestTheBlockPointsStorageAtTheDocumentStore pins the bullet for every project
// shape on BOTH SDK arms (the Gotchas section is unconditional, so a mis-nested
// `{{ if }}` that moved it into one branch must fail here), and bans the retired
// claim outright.
func TestTheBlockPointsStorageAtTheDocumentStore(t *testing.T) {
	shapes := allProjectShapesForTest()
	if len(shapes) < 3 {
		t.Fatalf("CONTROL failure, not a finding: %d shape(s) to render", len(shapes))
	}
	rendered := 0
	for _, shape := range shapes {
		for _, sdk := range []bool{false, true} {
			shape.ElementsSDK = sdk
			block, err := agentsManagedBlock(shape)
			if err != nil {
				t.Fatalf("render kind %q sdk=%t: %v", shape.Kind, sdk, err)
			}
			rendered++
			norm := normaliseProseForTest(block)
			if want := wantStorageBullet(sdk); !strings.Contains(norm, want) {
				t.Errorf("kind %q sdk=%t: the block does not carry the storage bullet WHOLE.\nwant: %s\nblock: %s",
					shape.Kind, sdk, want, norm)
			}
			for _, retired := range []string{"the one to prefer", "consolidating on them", "they still work",
				"`@civitai/sdk` 0.14.0 or later", "tallies"} {
				if strings.Contains(norm, retired) {
					t.Errorf("kind %q sdk=%t: the block still says %q about the legacy shared-storage routes",
						shape.Kind, sdk, retired)
				}
			}
		}
	}
	if rendered < 6 {
		t.Fatalf("CONTROL failure, not a finding: rendered %d block(s), want >= 6", rendered)
	}
}

// TestTheRealScaffoldsGetTheirArmsStorageBullet drives the two scaffolds that
// select each arm through detection, not a hand-set flag — the flag is only
// meaningful if detection sets it for the template that installs @civitai/sdk.
func TestTheRealScaffoldsGetTheirArmsStorageBullet(t *testing.T) {
	for _, tc := range []struct {
		tmpl        scaffold.Template
		elementsSDK bool
	}{{scaffold.PageElements, true}, {scaffold.PageMoney, false}} {
		block := normaliseProseForTest(setupInto(t, scaffoldInto(t, tc.tmpl)))
		if !strings.Contains(block, wantStorageBullet(tc.elementsSDK)) {
			t.Errorf("%s: the block does not carry its arm's storage bullet.\nwant: %s\nblock: %s",
				tc.tmpl, wantStorageBullet(tc.elementsSDK), block)
		}
		if strings.Contains(block, wantStorageBullet(!tc.elementsSDK)) {
			t.Errorf("%s: the block carries the OTHER arm's storage bullet", tc.tmpl)
		}
	}
}
