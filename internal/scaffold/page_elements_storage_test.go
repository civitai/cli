package scaffold

import (
	"path/filepath"
	"strings"
	"testing"
)

// The page-elements README's sentence on the `@civitai/sdk` legacy stores.
//
// It said the legacy `app.storage.get/set/…` and `app.sharedStorage` stores
// "are deprecated, so start nothing new on them" — an availability claim, read
// as "existing calls keep working". Since 2026-11-10 civitai.com refuses every
// call to them with `legacy_store_removed` (the same contract the agent-setup
// managed block and the page-money README already state; civitai/cli#900).
//
// Pinned WHOLE and whitespace-normalised, because a guard on one word is
// walkable by rewording back into a sentence that implies the stores work.
const wantPageElementsLegacyStoreSentence = "Its legacy `app.storage.get/set/…` and " +
	"`app.sharedStorage` stores are shut down: civitai.com refuses every call to them " +
	"with `legacy_store_removed`, so use the document store."

// The sentence after it: why the template's no-token `initialize()` matters for
// storage. @civitai/sdk 0.32.0 routes a collection to the app-data service only
// under a block token (no-token `initialize()`), and only for the shapes the
// service serves. Pinned WHOLE for the same reason as the one above. It must
// not name tallies: counters, marks, `update` and document publish/unpublish
// were removed from App Storage (civitai #10391; the server answers 410
// `removed`), so they go nowhere.
const wantPageElementsDataServiceSentence = "Because `initialize()` is called with no token " +
	"(transport mode), `@civitai/sdk` 0.32.0 or later lets the platform serve this app's own " +
	"collections through its data service; a `where` filter, a sort other than by key, and an " +
	"import (`{ from }`) go to civitai.com regardless."

func TestPageElementsReadmeExplainsTheDataServiceRoute(t *testing.T) {
	dir := renderPageElements(t)
	readme := strings.Join(strings.Fields(readFile(t, filepath.Join(dir, "README.md"))), " ")
	if !strings.Contains(readme, wantPageElementsDataServiceSentence) {
		t.Errorf("page-elements README.md does not carry the data-service sentence WHOLE.\nwant: %s",
			wantPageElementsDataServiceSentence)
	}
	if strings.Contains(strings.ToLower(readme), "tallies") {
		t.Error("page-elements README.md still mentions tallies, which App Storage no longer has")
	}
}

func TestPageElementsReadmeSaysTheLegacyStoresAreShutDown(t *testing.T) {
	dir := renderPageElements(t)
	readme := strings.Join(strings.Fields(readFile(t, filepath.Join(dir, "README.md"))), " ")

	if !strings.Contains(readme, wantPageElementsLegacyStoreSentence) {
		t.Errorf("page-elements README.md does not carry the legacy-store sentence WHOLE.\nwant: %s",
			wantPageElementsLegacyStoreSentence)
	}
	for _, retired := range []string{"are deprecated, so start nothing new", "start nothing new on them"} {
		if strings.Contains(readme, retired) {
			t.Errorf("page-elements README.md still says %q about the retired legacy stores", retired)
		}
	}
}
