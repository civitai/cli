package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// withdrawDiscardsListing is the ONE sentence naming what a withdraw destroys,
// stated once so `--help`, the confirmation prompt and the line printed after
// the withdraw lands cannot drift apart. Three separate copies of a weaker claim
// are how issue #350 happened.
//
// 🔴 IT IS SERVER-SIDE AND THE CLI CANNOT OPT OUT — do not "fix" this by
// changing the request. Traced in `civitai/civitai`, not assumed:
//
//   - `POST /api/v1/blocks/withdraw` (src/pages/api/v1/blocks/withdraw.ts) takes
//     a body schema with EXACTLY ONE field, `publishRequestId` (:78-83). There is
//     no flag to preserve the listing, and the tRPC spelling
//     (`blocks.withdrawPublishRequest`) shares the same single-field schema and
//     the same service call, so neither route can avoid it.
//   - `withdrawRequest` calls `deleteOnsiteDraftListingForSlug`
//     (src/server/services/blocks/publish-request.service.ts:1543 → :1454-1459)
//     immediately after the status flip, as a deliberate application-level
//     `deleteMany` — NOT a Prisma cascade. `AppBlockPublishRequest` has no
//     relation to `AppListing` at all; the two are joined by the slug string.
//
// WHAT SURVIVES, STATED SO THE COPY DOES NOT OVERCLAIM. The `deleteMany`
// predicate is `{slug, kind:'onsite', appBlockId: null, status:'draft'}`, so it
// can only ever remove the PRE-APPROVAL DRAFT listing that `app submit` minted
// for a FIRST version. A subsequent-version withdraw on an app that is already
// approved matches nothing and leaves the live listing untouched. The screenshot
// rows cascade away with it; the underlying Image rows are `onDelete: SetNull`,
// so the uploaded images are detached rather than hard-deleted — but the
// listing's icon/cover slots and every screenshot CAPTION are gone, and a
// resubmit mints an empty draft rather than reusing anything.
const withdrawDiscardsListing = "withdrawing a FIRST-VERSION submission also DELETES that app's store listing server-side — " +
	"its icon, its cover and every screenshot with its caption. Resubmitting mints an EMPTY listing; the media does not come back"

// withdrawListingCaveat is the qualification that MUST accompany every place
// this CLI tells an author their listing media "carries forward".
//
// 🔴 IT IS ONE CONSTANT BECAUSE THREE COPIES OF THE UNQUALIFIED CLAIM IS
// LITERALLY THE BUG (#350). The README quickstart, `app listing --help` and
// `app submit`'s success output each said the media carries forward, each was
// true across approval and false across withdraw, and nothing distinguished the
// two for the reader — so the CLI reprinted the promise over a listing it had
// just helped destroy. A fourth surface that repeats the claim must repeat this
// with it; TestCarriesForwardClaimsAreQualifiedEverywhere is the ledger, and it
// covers the README too (which cannot import a Go constant, so the guard is what
// keeps the prose and the help in step).
const withdrawListingCaveat = "carry forward on APPROVAL only — withdrawing the submission, or a moderator rejecting it, deletes the listing and everything on it"

func newAppWithdrawCmd() *cobra.Command {
	var idFlag string
	var assumeYes bool

	cmd := &cobra.Command{
		Use:   "withdraw [pubreq-id]",
		Short: "Withdraw your own pending App submission",
		Long: `Withdraw your own pending App submission so you can resubmit a new bundle
for the same slug.

🔴 THIS IS NOT A FREE REPAIR — IT DESTROYS YOUR STORE LISTING. ` + withdrawDiscardsListing + `.
That is server-side and this command cannot opt out of it: the withdraw route
takes the publish-request id and nothing else. So attach your listing media
AFTER the submission you intend to keep, not before a withdraw — and if a
listing already has media you care about, copy the captions somewhere first.

The same discard happens when a MODERATOR REJECTS a first-version submission.
What "carries forward" is APPROVAL: media survives a moderator approving the
app, and does not survive the app leaving review any other way.

Calls the token-authenticated, self-scoped withdraw route
(POST /api/v1/blocks/withdraw) with your stored credential — you can only ever
withdraw your OWN submissions. Both a personal API key and an OAuth login
(civitai login) work; the OAuth token must carry the Apps submit scope
(the same gate the submit route uses).

Only a submission still in the 'pending' review state can be withdrawn; an
already-approved/rejected (or already-withdrawn) request cannot. Withdrawing is
idempotent WITH RESPECT TO THE SUBMISSION ONLY — withdrawing an already-withdrawn
request still succeeds, but the listing the FIRST withdraw deleted is already
gone and a second call does not restore it.

Pass the publish-request id as a positional argument or via --id (find it with
"civitai app status").

CONFIRMATION: because that deletion is irreversible, an interactive run asks
first. Pass --yes/-y to skip the prompt in a script; a non-interactive shell
without --yes REFUSES rather than deleting a listing silently.`,
		Example: `  civitai app withdraw pubreq_01H        # withdraw by publish-request id (asks first)
  civitai app withdraw --id pubreq_01H   # same, via the flag
  civitai app withdraw pubreq_01H --yes  # skip the prompt (scripts/CI)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.Token() == "" {
				return civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN)"))
			}

			// Both refusals below are about the INVOCATION (a mutually exclusive
			// pair, and a missing required id), so both are usage errors —
			// exit 2, like the bad-flag-name errors Cobra already tags.
			if idFlag != "" && len(args) == 1 {
				return asUsageError(fmt.Errorf("pass the publish-request id as an argument OR via --id, not both"))
			}
			id := idFlag
			if id == "" && len(args) == 1 {
				id = args[0]
			}
			id = strings.TrimSpace(id)
			if id == "" {
				return asUsageError(fmt.Errorf("a publish-request id is required — pass it as an argument or with --id (find it via `civitai app status`)"))
			}

			// 🔴 The gate runs BEFORE the client is even constructed, so a refusal
			// cannot have issued a request. "Nothing was destroyed" is then
			// structural rather than a claim — TestAppWithdrawNonTTYRefusalIssuesNoRequest
			// asserts the server saw zero hits.
			if err := confirmWithdraw(cmd, id, assumeYes); err != nil {
				return err
			}

			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			ctx := context.Background()

			if err := client.WithdrawRequest(ctx, id); err != nil {
				return err
			}
			out, errw := cmd.OutOrStdout(), cmd.ErrOrStderr()
			fmt.Fprintln(out, ui.Success(fmt.Sprintf("Withdrew %s", id)))
			// 🔴 Repeated at the point of use, not just in --help: a `--yes` run
			// never saw the prompt, and the line printed after the thing happened is
			// the one place the author is guaranteed to read. Issue #350 was found by
			// a user whose only signal was a bare "Withdrew …" and rc=0.
			fmt.Fprintln(errw, ui.For(errw).Warn("Its store listing went with it if this was a first-version submission — icon, cover and every screenshot."))
			fmt.Fprintln(errw, ui.For(errw).Dim("Check with `civitai app listing status`; after you resubmit, re-attach the media with `civitai app listing set-icon` / `set-cover` / `add-screenshot`."))
			return nil
		},
	}
	cmd.Flags().StringVar(&idFlag, "id", "", "the publish-request id to withdraw (pubreq_...)")
	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip the confirmation and withdraw (required in a non-interactive shell)")
	return cmd
}

// confirmWithdraw gates the withdraw mutation, mirroring confirmSubmit,
// confirmGenerate and confirmCancel.
//
// Withdraw was the one destructive App command that did NOT gate (issue #350).
// It reads as the routine repair path — `--help` called it idempotent and the
// README documents withdraw → fix → resubmit — while the server-side listing
// delete it triggers is irreversible and, at the time it fires, has been
// contradicted by three separate places in this CLI promising the media carries
// forward. A user withdrew to fix a version number and silently lost a
// hand-captioned screenshot set, four seconds after `listing status` said it was
// there.
//
//   - --yes/-y              → proceed without prompting.
//   - non-TTY without --yes → REFUSE (never hang, never destroy silently).
//   - interactive TTY       → say what is lost, prompt, proceed only on "y".
//
// Everything it prints goes to STDERR so stdout stays machine-clean.
//
// 🔴 The prompt DEFAULTS TO NO: the switch enumerates the ACCEPTING answers and
// everything else — including a bare Enter — aborts. Rewriting it to enumerate
// the refusing answers instead would turn `[y/N]` into `[Y/n]` and delete a
// listing on a stray keystroke.
//
// 🔴 IT DOES NOT LOOK THE LISTING UP FIRST, and that is deliberate. Naming the
// exact media count would need three more reads (submission → slug, slug →
// listing, listing → assets), each a new failure mode on a path whose whole job
// is to be reachable when something is already wrong, and it would put a copy of
// the server's `appBlockId: null, status:'draft'` predicate in the CLI to decide
// whether to warn at all. The residual, accepted: the prompt says WHAT is
// destroyed and under WHICH condition, but not how many screenshots you have.
func confirmWithdraw(cmd *cobra.Command, publishRequestID string, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if !stdinIsTTY() {
		return fmt.Errorf("refusing to withdraw without --yes in a non-interactive shell — %s. "+
			"Pass --yes to confirm, or `civitai app listing status` to see what %s would take with it",
			withdrawDiscardsListing, safeTerm(publishRequestID))
	}

	errw := cmd.ErrOrStderr()
	st := ui.For(errw)
	fmt.Fprintf(errw, "About to withdraw %s.\n", safeTerm(publishRequestID))
	fmt.Fprintln(errw, st.Warn("This is not just a resubmit: "+withdrawDiscardsListing+"."))
	fmt.Fprintln(errw, st.Dim("The media only survives a moderator APPROVING the app — not a withdraw, and not a rejection."))
	fmt.Fprintln(errw, st.Dim("Copy any captions you want to keep before answering; `civitai app listing status` prints them."))
	fmt.Fprint(errw, "Withdraw it? [y/N]: ")

	r := bufio.NewReader(cmd.InOrStdin())
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return errors.New("withdraw aborted")
	}
}
