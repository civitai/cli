package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

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
https://civitai.com/user/account (API Keys). Either way the credential is saved
to your config file (~/.config/civitai/config.yaml, owner-readable only). The
CIVITAI_TOKEN environment variable still overrides the stored credential.

Note: ` + "`civitai login`" + ` (OAuth) grants submit but NOT Buzz-spend. To run
` + "`dev:live`" + ` real generations, authenticate with a full-scope personal API key
(` + "`civitai login --token <key>`" + `, created at https://civitai.com/user/account).`,
		Example: `  civitai login                 # browser device login (recommended)
  civitai login --no-browser    # device login without auto-opening a browser
  civitai login --token <token> # store a personal API key instead`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// --token (or piped stdin to the prompt) keeps the personal-key path.
			if strings.TrimSpace(tokenFlag) != "" {
				return loginWithToken(cmd, cfg, tokenFlag)
			}
			return loginWithDevice(cmd, cfg, noBrowser)
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "store a personal API key instead of the browser device login")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "do not attempt to open a browser for device login")
	return cmd
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
	fmt.Fprintf(cmd.OutOrStdout(), "Token saved to %s\n", cfg.Path())
	fmt.Fprintln(cmd.OutOrStdout(), "Verify with: civitai whoami")
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

	// Tell the user what to do. Never print the device_code (secret).
	uri := da.VerificationURI
	if uri == "" {
		uri = cfg.BaseURL() + "/login/oauth/device"
	}
	fmt.Fprintln(out, "To authenticate, open this URL in your browser and enter the code:")
	fmt.Fprintf(out, "\n  URL:  %s\n  Code: %s\n\n", uri, da.UserCode)

	if !noBrowser && da.VerificationURIComplete != "" {
		if err := openBrowser(da.VerificationURIComplete); err == nil {
			fmt.Fprintln(out, "Opened your browser. Waiting for approval...")
		} else {
			fmt.Fprintln(out, "Waiting for approval...")
		}
	} else {
		fmt.Fprintln(out, "Waiting for approval...")
	}

	tr, err := oc.PollToken(ctx, da, nil)
	if err != nil {
		return err
	}

	expiry := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if err := cfg.SetOAuthTokens(tr.AccessToken, tr.RefreshToken, expiry, tr.Scope.String()); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nLogged in. Tokens saved to %s\n", cfg.Path())
	fmt.Fprintln(out, "Verify with: civitai whoami")
	return nil
}

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
