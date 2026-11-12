package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// gitCredentialCmdName is the command git runs as a credential helper. It is
// excluded from the update notice (update_notice.go): git invokes it on every
// fetch and push, and a "new version available" line on each one is noise in
// the middle of git's own output.
const gitCredentialCmdName = "git-credential"

// helperAppRe bounds what `app pull` may write into a `!`-prefixed
// credential.helper value. git runs that value through the SHELL, and the slug
// comes from the server, so a slug carrying a space, quote, `;` or `$` would be
// a command injected into every later git operation in the checkout. This is
// deliberately narrower than anything the platform issues (lowercase slugs, and
// `apb_…` ids), so it can only refuse, never widen.
var helperAppRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// credentialRequest is the subset of git's credential-helper input this helper
// reads (git-credential(1), "INPUT/OUTPUT FORMAT"). Unknown keys are ignored.
type credentialRequest struct {
	Protocol string
	Host     string
	Path     string
}

// readCredentialRequest parses git's `key=value` lines up to the first blank
// line or EOF. A line without `=` is a protocol error.
func readCredentialRequest(r io.Reader) (credentialRequest, error) {
	var req credentialRequest
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return req, fmt.Errorf("malformed credential request line (no '=')")
		}
		switch k {
		case "protocol":
			req.Protocol = v
		case "host":
			req.Host = v
		case "path":
			req.Path = v
		}
	}
	return req, sc.Err()
}

// isLoopbackHost reports whether host (optionally host:port) names this machine.
// Plain http is accepted only for such a host — a local development platform —
// so a token is never sent in the clear over a network.
func isLoopbackHost(hostport string) bool {
	h := hostport
	if sh, _, err := net.SplitHostPort(hostport); err == nil {
		h = sh
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// matchCredentialRequest decides whether a credential for `info` may be handed
// to the git request `req`. It returns nil only when the request is for exactly
// the repository the server issued the credential for: same scheme, same
// host:port, same path. Every refusal names what differed, never the token.
//
// 🔴 THE PATH IS REQUIRED. `app pull` sets credential.<origin>.useHttpPath so
// git sends it; without it this helper could not tell this app's repository from
// another one on the same host, and would answer for both.
func matchCredentialRequest(req credentialRequest, app string, info *appapi.ForgejoCloneInfo) error {
	if info.Slug != app {
		return fmt.Errorf("the server answered for app %q, not %q — refusing to hand its credential to git", info.Slug, app)
	}
	u, err := url.Parse(info.HTTPURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("the server returned no usable repository URL for %q", app)
	}
	if u.User != nil {
		return fmt.Errorf("the server's repository URL for %q carries a credential; refusing to use it", app)
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Host) {
		return fmt.Errorf("the repository URL for %q is plain http on a non-local host; refusing to send a credential over it", app)
	}
	if req.Protocol != u.Scheme || !strings.EqualFold(req.Host, u.Host) {
		return fmt.Errorf("git asked for %s://%s, but %q lives at %s://%s — not answering", req.Protocol, req.Host, app, u.Scheme, u.Host)
	}
	want := strings.TrimPrefix(u.Path, "/")
	got := strings.TrimPrefix(req.Path, "/")
	if got == "" {
		return fmt.Errorf("git did not send the repository path, so this helper cannot tell %q from another repository on %s — re-run `civitai app pull` in this checkout to restore credential.useHttpPath", app, u.Host)
	}
	if got != want {
		return fmt.Errorf("git asked for repository %q, but this checkout's helper is for %q (%s) — not answering", got, app, want)
	}
	return nil
}

// credentialFetcher is the seam the helper calls to obtain the credential. The
// real one is the same owner-only clone-info query `app pull` uses: the server
// stores one git credential per developer and returns it, so calling it per git
// operation does not mint anything.
type credentialFetcher func(ctx context.Context, app string) (*appapi.ForgejoCloneInfo, error)

// runGitCredentialGet answers one `get`: on success it writes username/password
// to out; on any failure it writes `quit=1` (so git stops instead of falling
// through to an interactive username prompt the user cannot answer) and returns
// the error for stderr.
func runGitCredentialGet(ctx context.Context, in io.Reader, out io.Writer, app string, fetch credentialFetcher) error {
	req, err := readCredentialRequest(in)
	if err == nil {
		err = answerCredential(ctx, req, app, fetch, out)
	}
	if err != nil {
		_, _ = io.WriteString(out, "quit=1\n")
	}
	return err
}

func answerCredential(ctx context.Context, req credentialRequest, app string, fetch credentialFetcher, out io.Writer) error {
	info, err := fetch(ctx, app)
	if err != nil {
		return err
	}
	if info.NotYetAvailable {
		return fmt.Errorf("git access for %q is not available yet: %s", app, strings.TrimSpace(info.Message))
	}
	if err := matchCredentialRequest(req, app, info); err != nil {
		return err
	}
	if info.ForgejoUsername == "" || info.Token == "" {
		return fmt.Errorf("the server returned no git credential for %q", app)
	}
	if strings.ContainsAny(info.ForgejoUsername+info.Token, "\n\r\x00") {
		return fmt.Errorf("the server returned a malformed git credential for %q", app)
	}
	_, err = fmt.Fprintf(out, "username=%s\npassword=%s\n", info.ForgejoUsername, info.Token)
	return err
}

func newGitCredentialCmd() *cobra.Command {
	var appFlag string
	cmd := &cobra.Command{
		Use:   gitCredentialCmdName + " <get|store|erase>",
		Short: "git credential helper for your app's repository (installed by `app pull`)",
		Long: `A git credential helper (see git-credential(1)) that answers for ONE app's
repository on the platform git host. ` + "`civitai app pull`" + ` installs it into the
checkout's own .git/config, so the remote URL carries no token and none is
written to disk.

On ` + "`get`" + ` it reads your stored Civitai credential (` + "`civitai login`" + ` or
CIVITAI_TOKEN), asks the platform for your git credential for --app, and prints
it to git — only when git's request is for exactly that repository (scheme,
host and path). Anything else is refused with ` + "`quit=1`" + `, so git stops rather
than prompting.

` + "`store`" + ` and ` + "`erase`" + ` are accepted and do nothing: nothing is cached. To
stop using the credential in a checkout, remove the helper there
(` + "`git config --local --unset-all credential.<url>.helper`" + `).

You do not normally run this yourself; git does.`,
		Example: `  # what app pull writes into .git/config:
  #   [credential "https://<git host>"]
  #       helper =
  #       helper = !civitai git-credential --app my-block
  #       useHttpPath = true
  printf 'protocol=https\nhost=<git host>\npath=civitai-apps/my-block.git\n\n' | civitai git-credential --app my-block get`,
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := strings.TrimSpace(appFlag)
			if !helperAppRe.MatchString(app) {
				return asUsageError(fmt.Errorf("--app must be the app slug the helper was installed for (re-run `civitai app pull` to reinstall it)"))
			}
			switch args[0] {
			case "store", "erase":
				// Drain the request: git writes it and may block on a full pipe.
				_, _ = io.Copy(io.Discard, cmd.InOrStdin())
				return nil
			case "get":
			default:
				// git-credential(1): a helper ignores operations it does not know.
				return nil
			}
			cfg, err := config.Load()
			if err == nil && cfg.Token() == "" {
				err = civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` (or set CIVITAI_TOKEN) so git can reach %q", app))
			}
			if err != nil {
				_, _ = io.WriteString(cmd.OutOrStdout(), "quit=1\n")
				return err
			}
			client := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
			return runGitCredentialGet(context.Background(), cmd.InOrStdin(), cmd.OutOrStdout(), app, client.GetForgejoCloneInfo)
		},
	}
	cmd.Flags().StringVar(&appFlag, "app", "", "the app slug whose repository this helper answers for (required)")
	_ = cmd.MarkFlagRequired("app")
	return cmd
}

// helperExecutable is how the installed helper names this binary. A bare
// `civitai` when that resolves on PATH to this same file, so an upgrade that
// moves the binary keeps working; otherwise the absolute path of the running
// executable (an `npx` or `./bin/civitai` run), shell-quoted because git runs a
// `!` helper through sh. A package var so tests can point it at the test binary.
var helperExecutable = func() string {
	self, err := os.Executable()
	if err != nil {
		return "civitai"
	}
	if onPath, lerr := exec.LookPath("civitai"); lerr == nil && sameFile(onPath, self) {
		return "civitai"
	}
	return shellQuote(self)
}

func sameFile(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}

// shellQuote single-quotes s for sh, closing and reopening the quote around
// each embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// credentialContext is the git config URL context the helper is scoped to:
// scheme://host[:port] of the repository URL. Scoping it there keeps the
// helper (and the reset of inherited helpers) from touching any other remote
// the developer adds to the checkout.
func credentialContext(httpURL string) (string, error) {
	u, err := url.Parse(httpURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("the server returned an unusable repository URL")
	}
	if u.User != nil {
		return "", fmt.Errorf("the server's repository URL carries a credential; refusing to write it to .git/config")
	}
	return u.Scheme + "://" + u.Host, nil
}

// credentialHelperConfig is the ordered list of (key, value) pairs `app pull`
// writes into the checkout. The EMPTY helper first is load-bearing: git appends
// helpers from every config level, and an empty value resets that list, so a
// user's global `store` helper never receives (and writes to disk) the token
// this helper hands out. Measured on git 2.55.0 with a global
// `helper = store --file=<f>`: with the reset, `git credential approve` leaves
// <f> absent; the same config without it writes the password into <f>.
func credentialHelperConfig(httpURL, app string) ([][2]string, error) {
	if !helperAppRe.MatchString(app) {
		return nil, fmt.Errorf("the server returned an app slug %q that cannot be written into a git credential helper safely", app)
	}
	ctxURL, err := credentialContext(httpURL)
	if err != nil {
		return nil, err
	}
	key := "credential." + ctxURL
	return [][2]string{
		{key + ".helper", ""},
		{key + ".helper", "!" + helperExecutable() + " " + gitCredentialCmdName + " --app " + app},
		{key + ".useHttpPath", "true"},
	}, nil
}

// errNoTokenInURL marks a remote URL that carries no credential.
var errNoTokenInURL = errors.New("no credential in URL")

// stripURLCredential returns rawURL without its userinfo, or errNoTokenInURL
// when it has none. Never returns or logs the credential itself.
func stripURLCredential(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return "", errNoTokenInURL
	}
	u.User = nil
	return u.String(), nil
}

// normalizeRepoURL is rawURL as net/url prints it, so a stripped remote URL and
// the server's repository URL compare equal when they name the same thing.
func normalizeRepoURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.String()
}
