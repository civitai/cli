package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestAppMetricsNoTokenNamesTheRouteThatWorks is the issue-#260 guard.
//
// The no-token branch used to return the house-standard "run `civitai login`"
// — which is the ONE credential this command cannot use. The analytics proc is
// full-scope, so an OAuth browser login is refused with 403 (item 5 in
// AGENTS.md, and TestAppMetricsForbiddenAsksForPersonalAPIKey pins that half).
// Following the CLI's own advice therefore landed an author on a SECOND
// refusal.
//
// The exit code is pinned with errors.Is and never from the text (AGENTS item
// 7): stripping the ErrUnauthorized tag leaves every character of this message
// intact and silently degrades exit 3 to exit 1. The MESSAGE assertions are
// separate, and they ask for the specific route — "some login advice" is
// satisfied by exactly the wording this test exists to reject.
func TestAppMetricsNoTokenNamesTheRouteThatWorks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")

	_, _, err := run(t, "app", "metrics", "my-block")
	if err == nil {
		t.Fatal("expected an error with no token configured")
	}

	// Exit 3. Asserted structurally.
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("a missing token must classify as ErrUnauthorized (exit 3), got %T: %v", err, err)
	}

	msg := err.Error()
	for _, want := range []string{
		"no token configured",
		"personal API key",
		"civitai login --token <key>",
		accountAPIKeysURL, // where to actually mint one
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the no-token error must name %q so the advice is followable, got: %v", want, err)
		}
	}

	// And it must SAY that the obvious route is refused — otherwise a reader who
	// already has a browser login has no reason to mint a key.
	if !strings.Contains(msg, "403") {
		t.Errorf("the no-token error should say a browser login is refused with 403, got: %v", err)
	}
}

// TestAppMetricsCredentialRouteIsNotTheSpendRoute is the discriminating check.
//
// login.go's spendCredentialRoutes names BOTH credentials because `generate`
// accepts either, and it is the obvious constant to reach for here. It is the
// WRONG one: `civitai login --scopes generate` does not make an OAuth token
// full-scope, so offering it would be the same "walk them into a second
// refusal" defect in a new spelling. Every message assertion above stays green
// under that substitution, so the divergence is asserted directly.
func TestAppMetricsCredentialRouteIsNotTheSpendRoute(t *testing.T) {
	if appMetricsCredentialRoute == spendCredentialRoutes {
		t.Fatal("appMetricsCredentialRoute has been collapsed into spendCredentialRoutes — " +
			"the analytics proc is full-scope and refuses an OAuth login however it was scoped")
	}
	if strings.Contains(appMetricsCredentialRoute, "--scopes generate") {
		t.Errorf("appMetricsCredentialRoute offers `login --scopes generate`, which cannot satisfy "+
			"a full-scope proc: %q", appMetricsCredentialRoute)
	}
	if !strings.Contains(appMetricsCredentialRoute, accountAPIKeysURL) {
		t.Errorf("appMetricsCredentialRoute does not say where to mint the key: %q", appMetricsCredentialRoute)
	}

	// The command's own --help must carry the same route, from the same
	// constant — two hand-copied answers are how the 403 branch and the
	// no-token branch drifted apart in the first place.
	long := newAppMetricsCmd().Long
	if !strings.Contains(long, appMetricsCredentialRoute) {
		t.Errorf("`app metrics --help` does not render appMetricsCredentialRoute:\n%s", long)
	}
}
