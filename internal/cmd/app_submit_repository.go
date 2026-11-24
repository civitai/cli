package cmd

// app_submit_repository.go — the nudge to declare a source repository.
//
// The manifest's optional `repository` key is the author's own public source
// link; the store shows it, and a reviewer can open it to check the code. Most
// apps do not declare one. `app submit` and `app doctor` now say so, and an
// interactive `app submit` offers to add the URL of the project's own git
// remote.
//
// 🔴 ADVISORY, NEVER A GATE. Nothing here changes an exit code on its own, and
// nothing prompts where nobody is at a terminal (--yes, CI, GitHub Actions, an
// OIDC submit, a stdin that is not a TTY — see submitIsUnattended).
//
// 🔴 THE OFFER STOPS THE SUBMIT WHEN IT EDITS THE MANIFEST. The edit leaves
// block.manifest.json uncommitted, and the submit claims HEAD as the commit the
// bundle was built from (decision 32). Shipping the edited manifest would claim
// a commit that does not hold it — so on "yes" the key is written, the author is
// told to commit, and this run ends before anything is packaged or uploaded.
//
// 🔴 "PUBLIC" IS NOT CHECKED, AND THE WORDING SAYS SO. Nothing here asks a forge
// whether the repository is public (that would need the user's credential). The
// remote is offered only when it normalises to https on one of the three hosts
// the platform accepts, with an `<owner>/<repo>` path; the author decides whether
// it is public, and the URL is the repository they declared.

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/ui"
	"github.com/spf13/cobra"
)

// repositoryHosts are the hosts the platform accepts for the manifest's
// `repository` (civitai/civitai REPOSITORY_HOST_ALLOWLIST). A remote on any
// other host is never offered.
var repositoryHosts = []string{"github.com", "gitlab.com", "codeberg.org"}

// repositorySegment is the shape of one `<owner>` or `<repo>` path segment the
// platform accepts. A remote that does not fit is not offered, so the offer can
// only ever write a value of the accepted shape. This narrows the OFFER only:
// it is not a local check on a `repository` the author typed (decision 33).
var repositorySegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// repositoryFromRemote turns a git remote URL into the https repository URL the
// manifest takes, or reports false when the remote is not one to offer.
//
// Accepted: `https://host/o/r[.git][/]`, `ssh://[user@]host/o/r[.git]` and the
// scp form `[user@]host:o/r[.git]`, on an allowed host (any case; emitted
// lower-case), with exactly two path segments. Refused: any other scheme
// (http, git, file), a local path, a port, a query or fragment, and an https
// remote carrying a user name or password — a credential in a remote is never
// echoed or written.
func repositoryFromRemote(remote string) (string, bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", false
	}
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", false
		}
		switch u.Scheme {
		case "https":
			if u.User != nil {
				return "", false
			}
		case "ssh":
			if u.User != nil {
				if _, hasPassword := u.User.Password(); hasPassword {
					return "", false
				}
			}
		default:
			return "", false
		}
		if u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like syntax. Git reads a remote as a local path when a slash comes
		// before the first colon, so that shape is refused here too.
		before, after, ok := strings.Cut(remote, ":")
		if !ok || strings.Contains(before, "/") {
			return "", false
		}
		if i := strings.LastIndex(before, "@"); i >= 0 {
			before = before[i+1:]
		}
		host, path = before, after
	}
	host = strings.ToLower(host)
	if !slices.Contains(repositoryHosts, host) {
		return "", false
	}
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) != 2 {
		return "", false
	}
	owner, repo := segs[0], segs[1]
	if strings.HasSuffix(strings.ToLower(repo), ".git") {
		repo = repo[:len(repo)-len(".git")]
	}
	if !repositorySegment.MatchString(owner) || !repositorySegment.MatchString(repo) {
		return "", false
	}
	return "https://" + host + "/" + owner + "/" + repo, true
}

// repositoryNudge is what the nudge knows about one project directory.
type repositoryNudge struct {
	// missing is true when the manifest declares no `repository` key at all.
	missing bool
	// suggested is the origin remote as a repository URL, or "" when there is
	// no origin or it is not one to offer.
	suggested string
}

// checkManifestRepository reads whether the manifest in dir declares
// `repository` and, when it does not, what the origin remote suggests. A
// manifest that cannot be read reports nothing missing: the command's own
// manifest handling owns that error.
func checkManifestRepository(git gitOutputFunc, dir string) repositoryNudge {
	has, err := manifest.HasKey(dir, "repository")
	if err != nil || has {
		return repositoryNudge{}
	}
	n := repositoryNudge{missing: true}
	if remote, err := git(dir, "config", "--get", "remote.origin.url"); err == nil {
		n.suggested, _ = repositoryFromRemote(remote)
	}
	return n
}

// repositoryAdvisoryText is the advisory's message. Kept apart from the
// printer so its exact words can be pinned.
func repositoryAdvisoryText(suggested string) string {
	const lead = `No "repository" in block.manifest.json. A public source link helps reviewers check the code. `
	if suggested != "" {
		return lead + "Your git remote origin is " + suggested + ". If that repository is public, add it to the manifest: " +
			`"repository": "` + suggested + `"`
	}
	return lead + "If the source is public on github.com, gitlab.com or codeberg.org, add it to the manifest: " +
		`"repository": "https://github.com/<owner>/<repo>"`
}

// printRepositoryAdvisory writes the advisory to w (stderr), or nothing when
// the manifest declares `repository`. note says what the advisory does not do.
func printRepositoryAdvisory(w io.Writer, n repositoryNudge, note string) {
	if !n.missing {
		return
	}
	fmt.Fprintln(w, ui.For(w).Warn("Advisory ("+note+"):"))
	printFinding(w, repositoryAdvisoryText(n.suggested))
}

// printDoctorRepositoryAdvisory is `app doctor`'s copy of the advisory. doctor
// grades server listings by slug and reads no project, so this looks only at a
// block.manifest.json in dir (the current directory), and only when the run is
// about that app: no slug, or a slug naming the manifest's blockId. It never
// writes — doctor is a pure read.
func printDoctorRepositoryAdvisory(w io.Writer, git gitOutputFunc, dir, slug string) {
	m, err := manifest.Load(dir)
	if err != nil {
		return
	}
	if slug != "" && !appapi.SameSlug(slug, m.BlockID) {
		return
	}
	printRepositoryAdvisory(w, checkManifestRepository(git, dir), "it does not change the exit code")
}

// errSubmitStoppedForRepository ends a submit whose manifest the offer just
// edited. See the file comment for why the run cannot continue.
var errSubmitStoppedForRepository = errors.New("submit stopped before uploading: block.manifest.json now has " +
	"uncommitted changes, and a submit ships the commit it claims. Commit block.manifest.json, then re-run " +
	"`civitai app submit`")

// offerRepository asks, on an attended submit, whether to add the suggested
// repository to the manifest. "No" (the default, and any answer but y/yes)
// returns nil and the submit continues. "Yes" writes the key and returns
// errSubmitStoppedForRepository; a failed write returns its error. Either way
// nothing has been packaged or sent.
//
// The caller decides whether anyone is there to ask (assumeYes,
// submitIsUnattended); this function only asks.
func offerRepository(cmd *cobra.Command, dir, suggested string) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Add repository %s to block.manifest.json? [y/N]: ", suggested)
	line := readPromptLine(cmd.InOrStdin())
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
	default:
		return nil
	}
	if err := manifest.SetRepository(dir, suggested); err != nil {
		return fmt.Errorf("could not add repository to block.manifest.json: %w", err)
	}
	fmt.Fprintln(out, ui.For(out).Success(`Added "repository": "`+suggested+`" to block.manifest.json — the repository you declared.`))
	return errSubmitStoppedForRepository
}

// readPromptLine reads one line from r WITHOUT read-ahead, so the confirmation
// prompt that follows can still read its own answer from the same stdin. A
// bufio.Reader here would swallow the next line on a piped stdin.
func readPromptLine(r io.Reader) string {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	return b.String()
}
