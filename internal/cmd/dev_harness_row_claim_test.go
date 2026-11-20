package cmd

import "testing"

// TestDevHarnessRowDisclosesThePlaceholderFetch pins the `dev:harness` row that
// `civitai agent setup` writes into a project's agent docs. It restates the
// scaffolded README's Mock vs live row, which used to promise "no network"
// while the SDK mock host renders every result from placehold.co (issue #934;
// the README side is TestPageMoneyMockHarnessDocsDiscloseThePlaceholderFetch).
func TestDevHarnessRowDisclosesThePlaceholderFetch(t *testing.T) {
	const want = "Local dev against a MOCK host — synthetic replies, no real Buzz, no compute; " +
		"the mock result image is fetched from placehold.co"
	var got string
	n := 0
	for _, r := range knownDevScripts {
		if r.Script == "dev:harness" {
			got = r.Task
			n++
		}
	}
	if n != 1 {
		t.Fatalf("CONTROL failure: knownDevScripts has %d dev:harness row(s), want exactly 1", n)
	}
	if got != want {
		t.Errorf("dev:harness row:\n got: %s\nwant: %s", got, want)
	}
}
