package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// maxDevTokenRenameAttempts bounds the auto-rename-on-collision retry loop: after
// this many generated alternatives still collide, dev-token gives up.
const maxDevTokenRenameAttempts = 5

// devTokenSuffixGen generates the random lowercase-alphanumeric suffix appended
// to a colliding slug. It is a package var so tests can inject a deterministic
// sequence.
var devTokenSuffixGen = func() string { return scaffold.RandomSuffix(5) }

// budgetedScope is the spend scope a dev token must carry for `npm run dev:live`
// to actually generate (spend Buzz). The mint clamps it against the BEARER's
// AIServicesWrite bit, so a DEFAULT `civitai login` (which omits that bit)
// yields a read-only token whose Generate button silently dead-ends in the live
// harness. We decode the minted JWT to detect that case and warn loudly.
//
// A `civitai login --scopes generate` token DOES carry AIServicesWrite, so that
// clamp no longer strips it. The server-side ceiling therefore stopped being the
// backstop it used to be, which is why the CLI must not REQUEST this scope
// unless asked to — see devTokenRequestScopes.
const budgetedScope = "ai:write:budgeted"

// selfReadScope is the ONE scope the mint force-grants unconditionally, AFTER
// every clamp (dev-scoped-mint.service.ts `clampDevScopes`: `granted.push(
// 'user:read:self')` past the narrowing + spend-ceiling steps). It is a
// self-bound read of the caller's OWN identity, so requesting it can never
// escalate anything — which is exactly what makes it usable as a sentinel below.
const selfReadScope = "user:read:self"

// devTokenRequestScopes composes the `scopes` array POSTed to the dev-token mint
// route from the local manifest's scopes and the --spend affirmation.
//
// 🔴 THE CLI NEVER REQUESTS BUDGETED SPEND IMPLICITLY. That is the whole rule:
//
//   - --spend NOT given (default): return the manifest scopes with
//     `ai:write:budgeted` FILTERED OUT. Everything else goes through untouched.
//   - --spend given: UNION `ai:write:budgeted` in, so the request explicitly
//     asks for spend instead of relying on the server inferring it from the
//     bearer.
//
// Why the filter had to exist: the page-money scaffold declares
// `ai:write:budgeted` in block.manifest.json, so passing the manifest through
// verbatim REQUESTED spend on every default mint. The server's clamp only
// stripped it while no OAuth credential could carry AIServicesWrite; once the
// civitai-cli client was widened (allowedScopes 100777985), `civitai login
// --scopes generate` + `civitai app dev-token <slug>` silently produced a
// real-Buzz-spending dev token. Filtering makes the default request say what the
// developer actually asked for.
//
// 🔴 THE SENTINEL IS LOAD-BEARING, NOT DECORATION. `scopes` is `omitempty` on
// the wire and the server only narrows when `requestedScopes.length > 0`
// (clampDevScopes). So for a manifest that declares budgeted spend and NOTHING
// ELSE — which is precisely the default scaffold — filtering to an empty slice
// would drop the key entirely, and on an APPROVED or PENDING app the server
// would then fall back to that app's own scope snapshot and re-grant spend. The
// fix would be inert exactly where it matters most. Substituting the
// force-granted `user:read:self` keeps the narrowing expressible while adding no
// capability the token would not have had anyway.
//
// 🔴 WHAT THIS FUNCTION DOES NOT DO. With NO local manifest the CLI sends no
// `scopes` field, and that case is not expressible through this list at all:
// `scopes` is an intersection allow-list whose only sentinel is "absent ⇒ server
// decides", so "everything you would grant me, minus spend" cannot be written —
// enumerating a grant set the CLI does not know would silently strip
// storage/collections/etc. That gap is why the request ALSO carries a separate
// spend-intent field (devTokenBody.RequestBudgetedSpend, civitai/civitai#3703
// step 1), which says the same thing without enumerating anything.
//
// 🔴 THE TWO MECHANISMS ARE COMPLEMENTARY AND MUST AGREE. The narrowing here is
// what works against a server that has not flipped its `?? true` default; the
// field is what works after. Both are derived from the SAME `spend` bool in
// newAppDevTokenCmd, and the invariant they owe each other is a biconditional:
// the body's requestBudgetedSpend is true if and only if the scopes list
// contains `ai:write:budgeted` (an absent list contains nothing). Never one
// asking for spend while the other strips it.
//
// Do not describe this function as making a `--scopes generate` login unable to
// spend.
func devTokenRequestScopes(manifestScopes []string, spend bool) []string {
	if spend {
		for _, s := range manifestScopes {
			if s == budgetedScope {
				return manifestScopes // already declared — --spend is a no-op
			}
		}
		// Fresh slice: never append into the caller's backing array.
		out := make([]string, 0, len(manifestScopes)+1)
		out = append(out, manifestScopes...)
		return append(out, budgetedScope)
	}
	// No declared scopes → send nothing, preserving nil-ness so the key is
	// OMITTED. This is the inexpressible case above, not an oversight.
	if len(manifestScopes) == 0 {
		return manifestScopes
	}
	out := make([]string, 0, len(manifestScopes))
	for _, s := range manifestScopes {
		if s == budgetedScope {
			continue // EVERY occurrence, not just the first
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{selfReadScope}
	}
	return out
}

// spendFilteredFromManifest reports whether the local manifest DECLARED
// budgetedScope while --spend was absent — i.e. devTokenRequestScopes just
// dropped it from the request. That is a deliberate behaviour change with a
// user-visible consequence (dev:live will not generate), so it is announced
// rather than left to surface as a 403 later.
func spendFilteredFromManifest(manifestScopes []string, spend bool) bool {
	if spend {
		return false
	}
	for _, s := range manifestScopes {
		if s == budgetedScope {
			return true
		}
	}
	return false
}

// spendFilteredNotice is the stderr notice for spendFilteredFromManifest.
//
// 🔴 CLAIM DISCIPLINE — the wording is load-bearing. What this closes is the
// CLI requesting budgeted spend IMPLICITLY. It does NOT make a
// `civitai login --scopes generate` credential unable to spend in dev:live: with
// no local manifest the CLI sends no `scopes` field at all and the server
// resolves spend from the bearer, which is not expressible client-side (see
// devTokenRequestScopes). That closure is server-side — civitai/civitai#3703.
// Never phrase this as "your login cannot spend".
func spendFilteredNotice(sty ui.Styler, slug string) string {
	return sty.Warn(manifest.Filename+" declares "+budgetedScope+", but --spend was not passed — "+
		"this token does NOT request budgeted spend.") + "\n" +
		"   The CLI no longer requests budgeted spend implicitly, so `npm run dev:live` will refuse to\n" +
		"   generate with \"block lacks " + budgetedScope + " scope\". Re-mint with --spend to allow REAL Buzz:\n" +
		"     civitai app dev-token " + slug + " --spend --env >> .env.development.local"
}

// spendNarrowsToBudgetedOnly reports whether --spend turns an OTHERWISE-EMPTY
// scopes request into the single-element `["ai:write:budgeted"]`. That case is a
// genuine narrowing side effect — the request goes from "server decides" to an
// intersection against ONE scope — so the user is told rather than left to
// discover it when app storage stops working.
func spendNarrowsToBudgetedOnly(manifestScopes []string, spend bool) bool {
	return spend && len(manifestScopes) == 0
}

// spendNarrowingNotice is the stderr warning for spendNarrowsToBudgetedOnly.
func spendNarrowingNotice(sty ui.Styler) string {
	return sty.Warn("--spend with no local "+manifest.Filename+" scopes: this request narrows the token to "+
		budgetedScope+" ALONE.") + "\n" +
		"   Run it from your App project dir (or declare every scope your app needs in " + manifest.Filename +
		") so storage/collections scopes are requested too."
}

// tokenCanSpend reports whether the minted dev-token JWT carries the budgeted
// spend scope. It decodes the JWT payload segment WITHOUT verifying the
// signature (that's the server's job on every API call) — we only need the
// claims to give an early, actionable warning. Any malformed input returns
// false conservatively (the worst case is an extra warning, never a missed
// spend). Mirrors the SDK live host's decodeBlockTokenPayload.
func tokenCanSpend(jwt string) bool {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	for _, s := range claims.Scopes {
		if s == budgetedScope {
			return true
		}
	}
	return false
}

// readOnlyTokenWarning builds the stderr warning shown when a minted dev token
// can't spend. It names the ACTUAL cause instead of presuming OAuth:
//   - credential CAN spend (per whoami) → the request never asked for the
//     scope; the fix is --spend (or the manifest), not the credential.
//   - credential can't spend + OAuth login → this login did not opt into
//     generation; `civitai login --scopes generate` (or a full-scope personal
//     key) is the fix.
//   - credential can't spend + personal key → that key lacks AI Services.
//
// It's a pure function (canSpend from whoami, authKind from config) so the
// branch logic is unit-testable without a network round-trip. The header glyph +
// color are resolved against sty's writer (the command's stderr) so color
// follows that stream independently of stdout.
func readOnlyTokenWarning(sty ui.Styler, canSpend bool, authKind, slug string) string {
	header := "\n" + sty.Warn("This token is READ-ONLY (no `ai:write:budgeted` scope) — "+
		"`npm run dev:live` will NOT spend Buzz or generate; Generate dead-ends silently.")
	remint := "     civitai app dev-token " + slug + " --env >> .env.development.local   # re-mint, then restart dev:live"

	switch {
	case canSpend:
		// The stored credential CAN spend, so the token is read-only because the
		// mint request didn't carry the scope — pass --spend, or declare it in the
		// manifest so every mint asks for it.
		return header + "\n" +
			"   Cause: your credential CAN spend, but `ai:write:budgeted` wasn't requested — re-mint with `--spend` (or add it to `scopes` in block.manifest.json):\n" +
			"     civitai app dev-token " + slug + " --spend --env >> .env.development.local\n" +
			remint
	case authKind == config.AuthKindOAuth:
		return header + "\n" +
			"   Cause: you're signed in with an OAuth login that did NOT opt into generation, so it can't spend Buzz. Re-login asking for it:\n" +
			"     civitai login --scopes generate  # grants generation + Buzz spend\n" +
			"     civitai login --token <key>      # or a full-scope personal API key: https://civitai.com/user/account\n" +
			remint
	default:
		return header + "\n" +
			"   Cause: your stored personal API key lacks the AI Services (Buzz-spend) scope. Create a full-scope key, then re-store it:\n" +
			"     civitai login --token <key>      # https://civitai.com/user/account\n" +
			remint
	}
}

// validateDevTokenBudget range-checks a --budget value against the server's real
// accepted range, returning a usage error naming the bound it broke.
//
// It is a pure function so both bounds are testable without a round trip, and it
// exists at all because the two bounds fail differently on the server: under the
// minimum the route answers 400 (so sending it only wastes a call), while OVER
// the cap the route succeeds and silently clamps — the developer asks for 400,
// receives a 250-budget token, and nothing anywhere says so. Catching that here
// is the only place it can be caught.
func validateDevTokenBudget(budget int) error {
	switch {
	case budget < appapi.DevBuzzBudgetMin:
		return fmt.Errorf(
			"--budget %d is not a positive Buzz amount — pass an integer between %d and %d, or omit --budget to take the server's default (%d)",
			budget, appapi.DevBuzzBudgetMin, appapi.DevBuzzBudgetCap, appapi.DevBuzzBudgetDefault)
	case budget > appapi.DevBuzzBudgetCap:
		return fmt.Errorf(
			"--budget %d is above the server's dev-token cap of %d Buzz — the server would clamp it to %d WITHOUT saying so, "+
				"leaving you a token that does not carry the budget you asked for. Pass --budget %d or lower",
			budget, appapi.DevBuzzBudgetCap, appapi.DevBuzzBudgetCap, appapi.DevBuzzBudgetCap)
	default:
		return nil
	}
}

func newAppDevTokenCmd() *cobra.Command {
	var envOut bool
	var budget int
	var spend bool

	cmd := &cobra.Command{
		Use:   "dev-token <slug>",
		Short: "Mint a short-lived dev block token for `npm run dev:live`",
		Long: `Mint a short-lived dev block token so a scaffolded page-money app can run
"npm run dev:live" against the REAL Civitai backend.

Calls the invite-gated mint route (POST /api/v1/blocks/dev-token) with your
stored credential and prints the token (a ~4-hour RS256 JWT). Paste it into
VITE_LIVE_BLOCK_TOKEN in .env.development.local, then restart "npm run dev:live".

The minted token's CAPABILITIES depend on the credential you mint with:
  - REAL generation (spends real Buzz) needs a credential carrying AI Services:
    a FULL-SCOPE PERSONAL API KEY (create one at https://civitai.com/user/account)
    or an OAuth login that opted in with "civitai login --scopes generate".
    Confirm yours can spend with "civitai whoami".
  - A DEFAULT OAuth login ("civitai login", no --scopes) mints a
    READ/IDENTITY-ONLY dev token (no spend) — dev:live shows your viewer +
    catalog/storage, but estimate → submit → generation will NOT spend.

--spend is the explicit affirmation that this token may spend real Buzz. It does
two things to the request: it adds ai:write:budgeted to the scopes REQUESTED from
the mint route, and it sets the request's spend-intent field (requestBudgetedSpend)
to true. The server still clamps against your credential, so --spend cannot grant
what your credential lacks.

WITHOUT --spend the CLI never requests budgeted spend implicitly: if your
block.manifest.json declares ai:write:budgeted it is FILTERED OUT of the request
(the command tells you when that happens). The scaffolded money app declares it,
so a live run that used to generate now needs --spend — otherwise dev:live
refuses with "block lacks ai:write:budgeted scope". Every other manifest scope is
requested unchanged. With no local manifest the CLI sends no scopes at all —
independently of that, EVERY mint now states its spend intent explicitly, since
requestBudgetedSpend is always present on the request and is true only with
--spend.

Pre-GA the mint route is invite-only. You do NOT need to submit the app
first — for a brand-new slug with no app row yet, the token is minted from the
scopes in your local block.manifest.json (clamped server-side), so
"create → dev-token → dev:live" works directly. The token is short-lived —
never commit it; re-mint when it expires.

BUDGET (--budget, 1-250 Buzz). The token carries a per-generation Buzz budget.
Omit --budget and the server picks one — 50 for a slug with no submitted app.
Your LOCAL block.manifest.json page.buzzBudgetPerGen does NOT raise it: until
the app is submitted there is no server-side manifest to read, so the 50 is a
flat default, not a clamp of your file. --budget is the only way to move it.

A generation is REFUSED outright when the recipe's Buzz ceiling exceeds that
budget, so a shipped recipe with a ceiling of 90 dead-ends on the default:

  insufficient buzz budget: recipe ceiling 90 exceeds budget 50

Raise it (--budget 250) rather than editing the recipe.

The trap: for an inline customComfy graph your maxBuzz is BOTH the Buzz ceiling
and the step timeout in SECONDS. An over-thrifty budget therefore does not fail
as a billing error — the step runs out of wall clock and comes back "expired",
which reads like a broken graph. Budget for the seconds the graph needs.`,
		Example: `  civitai app dev-token my-block               # print the token to stdout
  civitai app dev-token my-block --spend       # also REQUEST ai:write:budgeted (real Buzz)
  civitai app dev-token my-block --budget 250  # max budget (and 250s of customComfy wall clock)
  civitai app dev-token my-block --env         # print VITE_LIVE_BLOCK_TOKEN=<token>
  civitai app dev-token my-block --env >> .env.development.local`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			var slug string
			if len(args) == 1 {
				slug = strings.TrimSpace(args[0])
			}
			if slug == "" {
				return fmt.Errorf("an app slug is required — e.g. `civitai app dev-token my-block` (find it with `civitai app status`)")
			}

			// Distinguish "not set" from "set to zero": only a CHANGED flag
			// becomes a request field. Defaulting the variable and always
			// sending it would permanently shadow the server's own budget
			// resolution, which is the behaviour every omitted-flag invocation
			// depends on today.
			var buzzBudget *int
			if cmd.Flags().Changed("budget") {
				if err := validateDevTokenBudget(budget); err != nil {
					return asUsageError(err)
				}
				buzzBudget = &budget
			}

			// Read the LOCAL manifest scopes (current working directory — the
			// user runs this from the scaffolded project dir) so the server can
			// mint a token for a slug with no app row yet (no submit needed).
			// Degrade gracefully: a missing/malformed manifest sends no scopes
			// (read-only token), and a registered app's server-side scopes still
			// govern when none are sent.
			manifestScopes := manifest.LoadScopes(".")

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			errOut := cmd.ErrOrStderr()

			// --spend is the EXPLICIT affirmation that this token may spend real
			// Buzz, and it drives BOTH halves of the request:
			//
			//   - the `scopes` narrowing (devTokenRequestScopes), which works
			//     against a server that has not flipped its default, and
			//   - the `requestBudgetedSpend` field passed to MintDevToken below,
			//     which states the intent outright.
			//
			// 🔴 ONE VARIABLE FEEDS BOTH, and that is the point: the two must
			// never disagree (a body asking for spend while the scopes strip it,
			// or the reverse). Do not compute either from anything but `spend` —
			// TestAppDevTokenSpendIntentAndScopesAgree pins the relationship on
			// the wire across the whole manifest × flag matrix.
			scopes := devTokenRequestScopes(manifestScopes, spend)
			if spendNarrowsToBudgetedOnly(manifestScopes, spend) {
				fmt.Fprintln(errOut, spendNarrowingNotice(ui.For(errOut)))
			}
			// Announce the FILTER before the mint, so the developer learns why
			// dev:live won't generate here rather than from the server's 403.
			if spendFilteredFromManifest(manifestScopes, spend) {
				fmt.Fprintln(errOut, spendFilteredNotice(ui.For(errOut), slug))
			}
			token, slug, err := mintDevTokenWithRename(context.Background(), client, errOut, ".", slug, scopes, buzzBudget, spend)
			if err != nil {
				return err
			}

			// Token to stdout (pipeable); the paste hint to stderr so it never
			// pollutes a `--env >> .env.development.local` redirect.
			out := cmd.OutOrStdout()
			if envOut {
				fmt.Fprintf(out, "VITE_LIVE_BLOCK_TOKEN=%s\n", token)
			} else {
				fmt.Fprintln(out, token)
			}
			fmt.Fprintln(errOut,
				"Paste into VITE_LIVE_BLOCK_TOKEN in .env.development.local, then restart `npm run dev:live`. "+
					"Short-lived (~4h); never commit it.")

			// Catch the #1 dev:live dead-end EARLY: a read-only token makes
			// `dev:live` Generate silently do nothing (the live host can't grant
			// consent for a scope the token lacks). Warn at the source — the
			// mint — not after a wasted click, and name the ACTUAL cause: a
			// read-only token can come from an OAuth login, a personal key
			// lacking AI Services, OR a manifest that never asked for the spend
			// scope. Disambiguate with whoami (the credential's real capability).
			if !tokenCanSpend(token) {
				canSpend := false
				if id, whoErr := client.WhoAmI(context.Background()); whoErr == nil {
					canSpend = id.CanSpendBuzz()
				}
				fmt.Fprintln(errOut, readOnlyTokenWarning(ui.For(errOut), canSpend, cfg.AuthKind(), slug))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&envOut, "env", false, "print VITE_LIVE_BLOCK_TOKEN=<token> (paste-ready into .env.development.local)")
	cmd.Flags().BoolVar(&spend, "spend", false,
		"explicitly REQUEST the "+budgetedScope+" scope so npm run dev:live can spend REAL Buzz. "+
			"Omit it and that scope is FILTERED OUT of the request and the request's spend-intent "+
			"field states false — the CLI never asks for budgeted spend implicitly, even when your "+
			manifest.Filename+" declares it")
	cmd.Flags().IntVar(&budget, "budget", 0, fmt.Sprintf(
		"per-generation Buzz budget the token may spend (%d-%d; omit to let the server decide — %d for an unsubmitted app). Must clear your recipe's ceiling; for inline customComfy it is ALSO the step timeout in seconds",
		appapi.DevBuzzBudgetMin, appapi.DevBuzzBudgetCap, appapi.DevBuzzBudgetDefault))
	return cmd
}

// mintDevTokenWithRename mints a dev token for slug, and on the anti-shadow
// collision (appapi.ErrSlugRegisteredToOtherAccount — the slug is an approved app
// owned by another account) it appends a random suffix to the ORIGINAL slug,
// PERMANENTLY rewrites block.manifest.json's blockId, notifies on errOut, and
// retries — up to maxDevTokenRenameAttempts. It returns the token and the FINAL
// slug used (so callers reference the renamed slug in follow-up hints).
//
// Renaming only applies when a local manifest exists in dir (nothing to rewrite
// otherwise); any non-collision error, or a collision with no manifest, is
// surfaced verbatim without a rename.
//
// buzzBudget and requestBudgetedSpend are carried through every retry unchanged
// (nil = budget not requested; the spend intent is always stated): a rename
// changes which slug is minted for, never what the developer asked the token to
// be able to spend.
func mintDevTokenWithRename(ctx context.Context, client *appapi.Client, errOut io.Writer, dir, slug string, scopes []string, buzzBudget *int, requestBudgetedSpend bool) (string, string, error) {
	original := slug
	for attempt := 0; ; attempt++ {
		token, err := client.MintDevToken(ctx, slug, scopes, buzzBudget, requestBudgetedSpend)
		if err == nil {
			return token, slug, nil
		}
		if !errors.Is(err, appapi.ErrSlugRegisteredToOtherAccount) {
			return "", slug, err
		}
		// Auto-rename only makes sense when there's a local manifest to rewrite.
		if _, lerr := manifest.Load(dir); lerr != nil {
			return "", slug, err
		}
		if attempt >= maxDevTokenRenameAttempts {
			return "", slug, fmt.Errorf(
				"slug %q and %d generated alternatives are all registered to other accounts — choose a different blockId in %s",
				original, maxDevTokenRenameAttempts, manifest.Filename)
		}
		newSlug, gerr := scaffold.SuffixSlug(original, devTokenSuffixGen())
		if gerr != nil {
			return "", slug, fmt.Errorf("could not generate an alternative slug for %q: %w", original, gerr)
		}
		if serr := manifest.SetBlockID(dir, newSlug); serr != nil {
			return "", slug, fmt.Errorf("could not update %s with the renamed slug %q: %w", manifest.Filename, newSlug, serr)
		}
		fmt.Fprintln(errOut, ui.For(errOut).Warn(fmt.Sprintf(
			"Slug %q is registered to another account — renamed to %q for your app (%s updated).",
			slug, newSlug, manifest.Filename)))
		slug = newSlug
	}
}
