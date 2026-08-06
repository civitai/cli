package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// accountAPIKeysURL is where a user mints a personal API key (the "API Keys"
// section of the Civitai account page). Single-sourced here for the
// `login --token` (no value) deeplink.
const accountAPIKeysURL = "https://civitai.com/user/account"

// spendCredentialRoutes names BOTH credentials that carry the AI-Services
// (Buzz-spend) scope.
//
// ONE RULE, ONE PLACE. This sentence used to be open-coded in `login`, `app
// create`, `generate` and the three `workflows` commands, each asserting that a
// personal API key was the ONLY route. Adding `login --scopes generate` made
// every copy false and they had to be corrected one at a time — which is exactly
// how several were missed. Commands in other packages (genapi, appapi) cannot
// import this constant, so their wording is corrected in place; keep it in step.
const spendCredentialRoutes = "`civitai login --scopes generate` (a browser login that opts into " +
	"generation), or a full-scope personal API key (`civitai login --token <key>`, created at " +
	accountAPIKeysURL + ")"

// tokenFlagNoValue is the sentinel NoOptDefVal for --token. pflag only allows a
// flag to be written with no argument (`login --token`) when its NoOptDefVal is
// non-empty; without it, `--token` alone errors "flag needs an argument". We set
// this sentinel so the no-value form parses, then detect it (or an explicitly
// empty value) and print where to mint a key instead of attempting a login.
const tokenFlagNoValue = "\x00civitai-token-no-value"

func newLoginCmd() *cobra.Command {
	var tokenFlag string
	var noBrowser bool
	var scopeSets []string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Civitai",
		Long: `Authenticate the CLI with Civitai for authenticated commands (whoami,
app submit).

By default ` + "`civitai login`" + ` runs a browser-based device login: it prints a
URL and a code, you approve in your browser, and the CLI stores short-lived
OAuth tokens that refresh automatically.

Alternatively, pass --token to store a personal API key created at
https://civitai.com/user/account (API Keys). Passing --token with NO value prints
where to create that key and how to re-run (it does not log in). Either way the
credential is saved
to your config file (~/.config/civitai/config.yaml, owner-readable only). The
CIVITAI_TOKEN environment variable still overrides the stored credential.

SCOPES. By DEFAULT ` + "`civitai login`" + ` grants identity + Apps submit +
dev-tunnel, and deliberately NOT Buzz-spend — a plain login must never silently
hand the CLI authority to spend your Buzz. Opt in per named scope set with
--scopes (additive — you keep everything the default grants):

` + deviceScopeSetHelp() + `
So ` + "`civitai login --scopes generate`" + ` yields ONE credential that can both
submit apps and run ` + "`civitai generate`" + `. Without it, generation is refused
and ` + "`dev:live`" + ` cannot spend; the other way to get spend authority is a
full-scope personal API key (` + "`civitai login --token <key>`" + `, created at
https://civitai.com/user/account).

--scopes applies only to the browser device login; it is rejected with --token.

Switching accounts: running ` + "`civitai login`" + ` again overwrites the stored
credential with the new account — no separate logout needed. (Check the active
account with ` + "`civitai whoami`" + `.)`,
		Example: `  civitai login                    # browser device login (recommended); no Buzz-spend
  civitai login --scopes generate  # ALSO grant generation + Buzz SPEND (civitai generate)
  civitai login --no-browser       # device login without auto-opening a browser
  civitai login --token <token>    # store a personal API key instead
  civitai login --token            # no value: print where to create a personal key
  civitai login                    # run again to SWITCH the active account (overwrites the stored credential)`,
		// Setting --token's NoOptDefVal (so `login --token` needs no argument) makes
		// pflag parse the space-separated form `login --token <value>` as a single
		// positional arg. Accept exactly one positional ONLY when --token was given,
		// to preserve that form; login otherwise takes no positional args.
		//
		// Combined with SetInterspersed(false) below, this is position-aware: a
		// positional can ONLY be recovered as the token value when it FOLLOWS
		// --token. A positional BEFORE --token (e.g. `login foo --token`) halts
		// flag parsing, so --token is never marked Changed and this guard falls
		// through to NoArgs, rejecting the stray positional — closing the footgun
		// where `login foo --token` used to store `foo` as the token. See the
		// SetInterspersed call for the full rationale.
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("token") {
				if len(args) <= 1 {
					return nil
				}
				// 🔴 CREDENTIAL REDACTION. Reaching here means --token was given AND
				// there is more than one positional, so args[0] IS the token value
				// (position-aware recovery, above). cobra.NoArgs would render
				// `unknown command "<args[0]>" for "civitai login"` — i.e. print the
				// user's personal API key to stderr, where it lands in scrollback, CI
				// logs and pasted bug reports. Describe the SHAPE of the mistake and
				// echo nothing.
				return asUsageError(fmt.Errorf(
					"unexpected argument(s) after the `--token <key>` value (not echoed here: the first one " +
						"is your API key). Flag parsing stops at that value, so anything after it is a stray " +
						"argument. Put other flags BEFORE it (e.g. `civitai login --no-color --token <key>`), " +
						"or use `civitai login --token=<key>`. Nothing was stored"))
			}
			return cobra.NoArgs(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Resolve the token value across all three spellings:
			//   login --token=<value>   → tokenFlag holds <value>
			//   login --token <value>   → tokenFlag is the sentinel, <value> is args[0]
			//   login --token / --token= → no value (mint-deeplink form)
			tokenVal := tokenFlag
			noValueForm := tokenVal == tokenFlagNoValue
			if noValueForm {
				tokenVal = ""
				if len(args) == 1 {
					tokenVal = args[0]
				}
			}
			tokenVal = strings.TrimSpace(tokenVal)

			// --scopes is a DEVICE-FLOW concept: it is the `scope` parameter of the
			// device-authorization request. A personal API key's scopes are fixed
			// when it is minted in the web UI, so `--token <key> --scopes generate`
			// cannot do what it says. Reject it loudly rather than storing the key
			// and silently dropping the scope request.
			//
			// 🔴 THIS MUST PRECEDE THE bare-`--token` MINT-HELP RETURN BELOW. It used
			// to sit after it, which made the guard partial in exactly the two
			// spellings where --token carries NO value: `login --token --scopes
			// generate` and `login --scopes generate --token` both took the early
			// return, printed the mint help and exited 0 with --scopes silently
			// dropped — while the two value-carrying spellings were rejected. A guard
			// that fires for some spellings of the same mistake is worse than none,
			// because the exit-0 ones read as acceptance.
			if cmd.Flags().Changed("scopes") && cmd.Flags().Changed("token") {
				return asUsageError(fmt.Errorf(
					"--scopes applies only to the browser device login and cannot be combined with --token: " +
						"a personal API key's scopes are fixed when you create it at " + accountAPIKeysURL +
						". Run `civitai login --scopes generate` on its own, or drop --scopes"))
			}

			// `--token` with NO value: the user wants a personal key but hasn't got
			// one yet — point them at where to mint it rather than erroring or
			// falling through to the device flow.
			if cmd.Flags().Changed("token") && tokenVal == "" {
				printMintTokenHelp(cmd.OutOrStdout())
				return nil
			}

			// Resolve the requested scope BEFORE any network call so a typo'd set
			// name fails instantly instead of after a device-init round trip.
			scope, err := appapi.ResolveDeviceScope(scopeSets)
			if err != nil {
				return asUsageError(err)
			}

			// --token <value> keeps the personal-key path.
			if tokenVal != "" {
				return loginWithToken(cmd, cfg, tokenVal)
			}
			return loginWithDevice(cmd, cfg, noBrowser, scope, scopeSets)
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "store a personal API key instead of the browser device login (pass with no value to print where to create one)")
	// Allow `civitai login --token` with no argument; the sentinel is detected in RunE.
	cmd.Flags().Lookup("token").NoOptDefVal = tokenFlagNoValue
	// Non-interspersed parsing makes the positional-recovery of the space form
	// (`login --token <key>`) POSITION-AWARE, which closes the audit footgun.
	//
	// The problem: with NoOptDefVal set, `login --token <key>` and
	// `login <stray> --token` are otherwise INDISTINGUISHABLE after parsing —
	// both leave --token marked Changed with a single positional, so the stray
	// value would be stored as the token (overwriting a good credential with
	// garbage). NoOptDefVal is nonetheless required: it's what lets bare
	// `--token` mean "print the mint deeplink" AND what stops pflag from
	// swallowing a following flag as the value (so `login --token --bogus`
	// correctly errors "unknown flag" instead of storing "--bogus").
	//
	// With interspersed=false, pflag stops treating tokens as flags at the FIRST
	// positional. So a positional BEFORE --token (`login foo --token`) halts
	// parsing: --token is left as a positional, never marked Changed, and the
	// Args guard rejects the stray arg via NoArgs — nothing is stored. A
	// positional AFTER --token (`login --token key`) still parses as the one
	// allowed positional and is recovered as the value in RunE. This makes the
	// whole matrix unambiguous WITHOUT scanning raw os.Args. (Trade-off: global
	// flags must precede the space-form value, e.g. `login --no-color --token
	// key`, not `login --token key --no-color`; the `--token=key` form has no
	// such constraint. Combining --token with the device-only --no-browser is
	// nonsensical anyway.)
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "do not attempt to open a browser for device login")
	// --scopes takes a NAMED SET, never a raw bitmask: nobody should type bit
	// arithmetic to log in, and a named set is what lets the granted mask change
	// server-side without changing this interface.
	//
	// It is deliberately a value-taking flag with NO NoOptDefVal, which is what
	// keeps it clear of the --token footgun documented above. Because pflag
	// consumes the following argument as this flag's VALUE, `login --scopes
	// generate` produces ZERO positionals — so it can never be mistaken for the
	// `--token <value>` positional-recovery form, and the Args guard is untouched.
	// The SetInterspersed(false) trade-off still applies in the other direction:
	// in `login --token <key> --scopes generate` the positional <key> halts flag
	// parsing, so --scopes is left as a stray positional and the Args guard
	// rejects the whole invocation rather than silently ignoring it. Put --scopes
	// FIRST (or use `--token=<key>`).
	//
	// The cost of having no NoOptDefVal is the mirror image of --token's: pflag
	// DOES swallow a following flag as this flag's value, so `login --scopes
	// --no-browser` arrives at ResolveDeviceScope as the set name "--no-browser".
	// It fails safe (the login is refused, nothing stored) and ResolveDeviceScope
	// detects the leading dash and says the flag was swallowed rather than
	// reporting an unknown set. Giving --scopes a NoOptDefVal would fix the
	// swallow but reopen the positional ambiguity above, which is the worse bug.
	cmd.Flags().StringSliceVar(&scopeSets, "scopes", nil, fmt.Sprintf(
		"extra scope sets to request on a browser device login, additive on top of the default (valid: %s). "+
			"--scopes generate grants generation AND Buzz-SPEND authority; omit it and this login cannot spend your Buzz. "+
			"Not valid with --token",
		strings.Join(appapi.DeviceScopeSetNames(), ", ")))
	return cmd
}

// deviceScopeSetHelp renders the named --scopes sets as an indented list for the
// command's long help. Single-sourced from the appapi registry so a new set
// shows up in `civitai login --help` without editing this file.
func deviceScopeSetHelp() string {
	var b strings.Builder
	for _, name := range appapi.DeviceScopeSetNames() {
		summary, _ := appapi.DeviceScopeSetSummary(name)
		fmt.Fprintf(&b, "  --scopes %s\n      %s\n", name, summary)
	}
	return b.String()
}

// printMintTokenHelp tells the user where to create a personal API key and how to
// re-run login with it. Printed when `civitai login --token` is passed with no
// value; it performs no network login.
func printMintTokenHelp(out io.Writer) {
	fmt.Fprintf(out, "To use a personal API key, create one at %s (API Keys), then run:\n\n", accountAPIKeysURL)
	fmt.Fprintf(out, "  %s\n\n", ui.Code("civitai login --token <key>"))
	fmt.Fprintf(out, "Or run %s for browser sign-in.\n", ui.Code("civitai login"))
}

// loginWithToken stores a personal API key (manual, no refresh).
func loginWithToken(cmd *cobra.Command, cfg *config.Config, tokenFlag string) error {
	token := strings.TrimSpace(tokenFlag)
	if token == "" {
		fmt.Fprint(cmd.OutOrStdout(), "Civitai API token: ")
		r := bufio.NewReader(cmd.InOrStdin())
		line, _ := r.ReadString('\n')
		token = strings.TrimSpace(line)
	}
	if token == "" {
		return fmt.Errorf("no token provided")
	}
	if err := cfg.SetToken(token); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("Token saved to %s", cfg.Path())))
	fmt.Fprintf(cmd.OutOrStdout(), "Verify with: %s\n", ui.Code("civitai whoami"))
	return nil
}

// loginWithDevice runs the OAuth device-authorization grant. scope is the
// already-resolved device scope (ResolveDeviceScope); scopeSets are the raw
// --scopes names, used only to tell the user what extra authority they just
// asked for.
func loginWithDevice(cmd *cobra.Command, cfg *config.Config, noBrowser bool, scope string, scopeSets []string) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Surface the consequence at the point of use: the approval screen names
	// scopes, but the terminal is where the user decided. Printed BEFORE the
	// browser opens so it is on screen while they approve.
	printRequestedScopeSets(out, scopeSets)

	oc := appapi.NewOAuthClient(cfg.BaseURL())
	oc.Scope = scope
	da, err := oc.StartDevice(ctx)
	if err != nil {
		return err
	}

	// Primary action: auto-open the code-prefilled URL. Only when that FAILS (or
	// --no-browser / no complete URL) do we print the full manual instructions.
	// Never print the device_code (secret).
	opened := false
	if !noBrowser && da.VerificationURIComplete != "" {
		if err := browserOpener(da.VerificationURIComplete); err == nil {
			opened = true
		}
	}
	if opened {
		// The browser is already open on the prefilled URL — print a single
		// "or open ..." fallback (the BARE url + the code, not the complete URL).
		fmt.Fprintf(out, "Opened your browser to approve. Or open %s and enter code %s\n",
			deviceVerificationURI(da, cfg.BaseURL()), da.UserCode)
	} else {
		renderDeviceInstructions(out, da, cfg.BaseURL())
	}
	fmt.Fprintln(out, "Waiting for approval...")

	tr, err := oc.PollToken(ctx, da, nil)
	if err != nil {
		return err
	}

	expiry := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if err := cfg.SetOAuthTokens(tr.AccessToken, tr.RefreshToken, expiry, tr.Scope.String()); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%s\n", ui.Success(fmt.Sprintf("Logged in. Tokens saved to %s", cfg.Path())))
	fmt.Fprintf(out, "Verify with: %s\n", ui.Code("civitai whoami"))
	return nil
}

// printRequestedScopeSets tells the user which extra scope sets this login is
// asking for, and what each one authorizes. It prints NOTHING for a default
// login (no --scopes), so existing output is unchanged. Unknown names never
// reach here — ResolveDeviceScope rejects them before any network call.
//
// DEDUPED to match ResolveDeviceScope: the mask is an OR, so `--scopes
// generate,generate` (or a repeated --scopes flag) requests exactly the same
// authority as one mention. Printing the Buzz-spend warning twice for one grant
// misrepresents what is being asked for.
func printRequestedScopeSets(out io.Writer, scopeSets []string) {
	seen := make(map[string]bool, len(scopeSets))
	for _, raw := range scopeSets {
		name := strings.ToLower(strings.TrimSpace(raw))
		summary, ok := appapi.DeviceScopeSetSummary(raw)
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		fmt.Fprintf(out, "%s\n", ui.Warn(fmt.Sprintf(
			"Requesting the %q scope set: %s.", name, summary)))
	}
}

// deviceVerificationURI returns the BARE verification URI (no code prefilled),
// falling back to the base URL's device path when the server omits it.
func deviceVerificationURI(da *appapi.DeviceAuth, baseURL string) string {
	if da.VerificationURI != "" {
		return da.VerificationURI
	}
	return baseURL + "/login/oauth/device"
}

// renderDeviceInstructions prints the MANUAL device-flow instructions used when
// the browser was not opened (--no-browser, headless, or openBrowser failed). It
// prints a SINGLE actionable form — the code-prefilled complete URL when the
// server supplies one, otherwise the bare URL + code — rather than both URLs up
// front. Never prints the device_code (secret).
func renderDeviceInstructions(out io.Writer, da *appapi.DeviceAuth, baseURL string) {
	if da.VerificationURIComplete != "" {
		fmt.Fprintln(out, "To authenticate, open this URL (your code is pre-filled):")
		fmt.Fprintf(out, "\n  %s\n\n", da.VerificationURIComplete)
		return
	}
	fmt.Fprintln(out, "To authenticate, open this URL in your browser and enter the code:")
	fmt.Fprintf(out, "\n  URL:  %s\n  Code: %s\n\n", deviceVerificationURI(da, baseURL), da.UserCode)
}

// browserOpener opens a URL in the default browser. It is a package var so tests
// can simulate open success/failure without spawning a real browser.
var browserOpener = openBrowser

// openBrowser best-effort opens url in the default browser. A failure is
// non-fatal (the user can use the printed URL).
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		name = "xdg-open"
		args = []string{url}
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	return exec.Command(path, args...).Start()
}
