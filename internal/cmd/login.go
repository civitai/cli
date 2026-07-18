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

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// accountAPIKeysURL is where a user mints a personal API key (the "API Keys"
// section of the Civitai account page). Single-sourced here for the
// `login --token` (no value) deeplink.
const accountAPIKeysURL = "https://civitai.com/user/account"

// tokenFlagNoValue is the sentinel NoOptDefVal for --token. pflag only allows a
// flag to be written with no argument (`login --token`) when its NoOptDefVal is
// non-empty; without it, `--token` alone errors "flag needs an argument". We set
// this sentinel so the no-value form parses, then detect it (or an explicitly
// empty value) and print where to mint a key instead of attempting a login.
const tokenFlagNoValue = "\x00civitai-token-no-value"

func newLoginCmd() *cobra.Command {
	var tokenFlag string
	var noBrowser bool

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

Note: ` + "`civitai login`" + ` (OAuth) grants submit but NOT Buzz-spend. To run
` + "`dev:live`" + ` real generations, authenticate with a full-scope personal API key
(` + "`civitai login --token <key>`" + `, created at https://civitai.com/user/account).

Switching accounts: running ` + "`civitai login`" + ` again overwrites the stored
credential with the new account — no separate logout needed. (Check the active
account with ` + "`civitai whoami`" + `.)`,
		Example: `  civitai login                 # browser device login (recommended)
  civitai login --no-browser    # device login without auto-opening a browser
  civitai login --token <token> # store a personal API key instead
  civitai login --token         # no value: print where to create a personal key
  civitai login                 # run again to SWITCH the active account (overwrites the stored credential)`,
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
			if cmd.Flags().Changed("token") && len(args) <= 1 {
				return nil
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

			// `--token` with NO value: the user wants a personal key but hasn't got
			// one yet — point them at where to mint it rather than erroring or
			// falling through to the device flow.
			if cmd.Flags().Changed("token") && tokenVal == "" {
				printMintTokenHelp(cmd.OutOrStdout())
				return nil
			}

			// --token <value> keeps the personal-key path.
			if tokenVal != "" {
				return loginWithToken(cmd, cfg, tokenVal)
			}
			return loginWithDevice(cmd, cfg, noBrowser)
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
	return cmd
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

// loginWithDevice runs the OAuth device-authorization grant.
func loginWithDevice(cmd *cobra.Command, cfg *config.Config, noBrowser bool) error {
	out := cmd.OutOrStdout()
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	oc := api.NewOAuthClient(cfg.BaseURL())
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

// deviceVerificationURI returns the BARE verification URI (no code prefilled),
// falling back to the base URL's device path when the server omits it.
func deviceVerificationURI(da *api.DeviceAuth, baseURL string) string {
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
func renderDeviceInstructions(out io.Writer, da *api.DeviceAuth, baseURL string) {
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
