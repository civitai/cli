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
// with no further detail. Following the CLI's own advice therefore landed an
// author on a SECOND refusal, because the analytics proc carried no `.meta` and
// an un-annotated proc implicitly requires `TokenScope.Full`, which the scoped
// `civitai login` token is not.
//
// 🔴 THAT SECOND SENTENCE IS NO LONGER TRUE, and this test used to assert it.
// `civitai/civitai#3572` (`7c529f1eea`) annotated the proc
// `requiredScope: TokenScope.AppBlocksSubmit` so the CLI's login token could
// read its own analytics — verified against civitai/civitai `origin/main` at
// `src/server/routers/blocks.router.ts:5483`. The assertion that the message
// must contain "403" is therefore GONE from this test rather than merely
// unsatisfied: it was pinning a claim about the server that had been false for
// months, which is the shape a docs guard rots into.
//
// What the test still pins is the SHAPE of #260's fix, which is what was
// actually valuable: name the routes that WORK, name where to get one, and do
// it from one constant so the branches cannot drift apart again. It now demands
// BOTH routes, because both work and a message naming only one sends an author
// to mint a key they do not need.
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
		accountAPIKeysURL,   // where to actually mint one
		"Apps submit scope", // the bit that decides it
		"(`civitai login`)", // the OAuth route, which DOES work
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the no-token error must name %q so the advice is followable, got: %v", want, err)
		}
	}

	// 🔴 And it must NOT tell the reader the browser login is refused. That
	// sentence stood here for months after the server stopped refusing it, and a
	// reader who already had a login was sent to mint a key for nothing. Kept as
	// a PROHIBITION rather than deleted with the claim, so it cannot drift back.
	for _, banned := range []string{"refused with 403", "full-scope-refused"} {
		if strings.Contains(msg, banned) {
			t.Errorf("the no-token error still claims an OAuth login is refused (%q) — "+
				"civitai/civitai#3572 annotated the proc with AppBlocksSubmit and it is not: %v", banned, err)
		}
	}
}

// TestAppMetricsCredentialRouteIsNotTheSpendRoute is the discriminating check.
//
// login.go's spendCredentialRoutes names BOTH credentials because `generate`
// accepts either, and it is the obvious constant to reach for here — more so now
// that this command accepts both too. It is still the WRONG one, and the reason
// SURVIVED the #3572 correction above while its old phrasing did not: that
// constant offers `civitai login --scopes generate`, which opts a token into the
// AI-Services (Buzz-spend) bits and says NOTHING about the Apps submit bit this
// proc requires. Offering it would be the same "walk them into a second refusal"
// defect in a new spelling. Every message assertion above stays green under that
// substitution — both constants name a personal API key and the account URL — so
// the divergence is asserted directly.
func TestAppMetricsCredentialRouteIsNotTheSpendRoute(t *testing.T) {
	if appMetricsCredentialRoute == spendCredentialRoutes {
		t.Fatal("appMetricsCredentialRoute has been collapsed into spendCredentialRoutes — " +
			"that constant opts a login into the AI-Services scopes, which are not the Apps submit bit this proc needs")
	}
	if strings.Contains(appMetricsCredentialRoute, "--scopes generate") {
		t.Errorf("appMetricsCredentialRoute offers `login --scopes generate`, which grants the "+
			"AI-Services bits and not the Apps submit bit this proc requires: %q", appMetricsCredentialRoute)
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
