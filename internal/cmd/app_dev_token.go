package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/scaffold"
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
// to actually generate (spend Buzz). The server strips it from an OAuth-minted
// token (the civitai-cli OAuth client lacks AI Services), so an OAuth login
// yields a read-only token whose Generate button silently dead-ends in the live
// harness. We decode the minted JWT to detect that case and warn loudly.
const budgetedScope = "ai:write:budgeted"

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
//     scope; the fix is the manifest, not the credential.
//   - credential can't spend + OAuth login → OAuth grants no Buzz-spend.
//   - credential can't spend + personal key → that key lacks AI Services.
//
// It's a pure function (canSpend from whoami, authKind from config) so the
// branch logic is unit-testable without a network round-trip.
func readOnlyTokenWarning(canSpend bool, authKind, slug string) string {
	const header = "\n⚠  This token is READ-ONLY (no `ai:write:budgeted` scope) — " +
		"`npm run dev:live` will NOT spend Buzz or generate; Generate dead-ends silently."
	remint := "     civitai app dev-token " + slug + " --env >> .env.development.local   # re-mint, then restart dev:live"

	switch {
	case canSpend:
		// The stored credential CAN spend, so the token is read-only because the
		// mint request didn't carry the scope — i.e. the manifest omits it.
		return header + "\n" +
			"   Cause: your credential CAN spend, but `ai:write:budgeted` wasn't requested — add it to `scopes` in block.manifest.json, then re-mint:\n" +
			remint
	case authKind == config.AuthKindOAuth:
		return header + "\n" +
			"   Cause: you're signed in with an OAuth login (`civitai login`), which can't spend Buzz. Use a full-scope personal API key:\n" +
			"     civitai login --token <key>      # create one at https://civitai.com/user/account\n" +
			remint
	default:
		return header + "\n" +
			"   Cause: your stored personal API key lacks the AI Services (Buzz-spend) scope. Create a full-scope key, then re-store it:\n" +
			"     civitai login --token <key>      # https://civitai.com/user/account\n" +
			remint
	}
}

func newAppDevTokenCmd() *cobra.Command {
	var envOut bool

	cmd := &cobra.Command{
		Use:   "dev-token <slug>",
		Short: "Mint a short-lived dev block token for `npm run dev:live`",
		Long: `Mint a short-lived dev block token so a scaffolded page-money app can run
"npm run dev:live" against the REAL Civitai backend.

Calls the invite-gated mint route (POST /api/v1/blocks/dev-token) with your
stored credential and prints the token (a ~4-hour RS256 JWT). Paste it into
VITE_LIVE_BLOCK_TOKEN in .env.development.local, then restart "npm run dev:live".

The minted token's CAPABILITIES depend on the credential you mint with:
  - REAL generation (spends real Buzz) needs a FULL-SCOPE PERSONAL API KEY
    (create one at https://civitai.com/user/account — it carries AI Services).
    Confirm yours can spend with "civitai whoami".
  - An OAuth login ("civitai login") mints a READ/IDENTITY-ONLY dev token (no
    spend) — dev:live shows your viewer + catalog/storage, but estimate →
    submit → generation will NOT spend.

Pre-GA the mint route is invite-only. You do NOT need to submit the app
first — for a brand-new slug with no app row yet, the token is minted from the
scopes in your local block.manifest.json (clamped server-side), so
"create → dev-token → dev:live" works directly. The token is short-lived —
never commit it; re-mint when it expires.`,
		Example: `  civitai app dev-token my-block               # print the token to stdout
  civitai app dev-token my-block --env         # print VITE_LIVE_BLOCK_TOKEN=<token>
  civitai app dev-token my-block --env >> .env.development.local`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)")
			}

			var slug string
			if len(args) == 1 {
				slug = strings.TrimSpace(args[0])
			}
			if slug == "" {
				return fmt.Errorf("an app slug is required — e.g. `civitai app dev-token my-block` (find it with `civitai app status`)")
			}

			// Read the LOCAL manifest scopes (current working directory — the
			// user runs this from the scaffolded project dir) so the server can
			// mint a token for a slug with no app row yet (no submit needed).
			// Degrade gracefully: a missing/malformed manifest sends no scopes
			// (read-only token), and a registered app's server-side scopes still
			// govern when none are sent.
			scopes := manifest.LoadScopes(".")

			client := api.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			errOut := cmd.ErrOrStderr()
			token, slug, err := mintDevTokenWithRename(context.Background(), client, errOut, ".", slug, scopes)
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
				fmt.Fprintln(errOut, readOnlyTokenWarning(canSpend, cfg.AuthKind(), slug))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&envOut, "env", false, "print VITE_LIVE_BLOCK_TOKEN=<token> (paste-ready into .env.development.local)")
	return cmd
}

// mintDevTokenWithRename mints a dev token for slug, and on the anti-shadow
// collision (api.ErrSlugRegisteredToOtherAccount — the slug is an approved app
// owned by another account) it appends a random suffix to the ORIGINAL slug,
// PERMANENTLY rewrites block.manifest.json's blockId, notifies on errOut, and
// retries — up to maxDevTokenRenameAttempts. It returns the token and the FINAL
// slug used (so callers reference the renamed slug in follow-up hints).
//
// Renaming only applies when a local manifest exists in dir (nothing to rewrite
// otherwise); any non-collision error, or a collision with no manifest, is
// surfaced verbatim without a rename.
func mintDevTokenWithRename(ctx context.Context, client *api.Client, errOut io.Writer, dir, slug string, scopes []string) (string, string, error) {
	original := slug
	for attempt := 0; ; attempt++ {
		token, err := client.MintDevToken(ctx, slug, scopes)
		if err == nil {
			return token, slug, nil
		}
		if !errors.Is(err, api.ErrSlugRegisteredToOtherAccount) {
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
		fmt.Fprintf(errOut,
			"Slug %q is registered to another account — renamed to %q for your app (%s updated).\n",
			slug, newSlug, manifest.Filename)
		slug = newSlug
	}
}
