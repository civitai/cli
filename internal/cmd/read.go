package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// readOpts are the flags shared by every read subcommand.
type readOpts struct {
	json bool
	anon bool
}

// bindReadFlags attaches the shared --json / --anon flags to a read subcommand
// and installs a PreRunE guard that rejects a non-positive EXPLICIT --limit.
//
// The lower-bound check lives here (not in checkLimit) because it needs the
// flag's Changed state: every read command binds --limit with a default of 0
// and passes that value to checkLimit unconditionally, so at the checkLimit
// call site an explicit `--limit 0` and an unset limit (which means "use the
// server default page") are the SAME int and indistinguishable. Here we can
// see cmd.Flags().Changed("limit"), so we reject a caller who explicitly asked
// for a page size < 1 while leaving the unset default untouched. The upper
// bound (> endpoint max) stays in checkLimit, which knows the per-endpoint max.
func bindReadFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "print the raw API JSON response (for scripting)")
	cmd.Flags().Bool("anon", false, "force an anonymous request (ignore any stored login token)")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if f := c.Flags().Lookup("limit"); f != nil && f.Changed {
			if v, err := c.Flags().GetInt("limit"); err == nil && v < 1 {
				return asUsageError(fmt.Errorf(
					"--limit must be a positive integer (got %d); omit --limit to use the server default", v))
			}
		}
		if prev != nil {
			return prev(c, args)
		}
		return nil
	}
}

// readFlags reads the shared --json / --anon flag values off cmd.
func readFlags(cmd *cobra.Command) *readOpts {
	j, _ := cmd.Flags().GetBool("json")
	a, _ := cmd.Flags().GetBool("anon")
	return &readOpts{json: j, anon: a}
}

// allowPrivateDownloadHostsForTest, when true, makes newReader build download
// clients that skip the SSRF guard (https-only + internal-range dial block).
// It exists ONLY so the download command tests — whose httptest servers bind
// plain-http loopback (127.0.0.1) — can exercise the real download path. It is
// unexported and never set outside the cmd test suite; production builds leave
// it false so the guard is fully in force.
var allowPrivateDownloadHostsForTest bool

// newReader builds an civitai.Client for the public read endpoints. It sends the
// stored login token when present (OAuth device-login or personal key, both
// refreshed transparently) unless --anon is set, and works fully anonymously
// when no token is configured — the public read routes don't require auth.
func newReader(o *readOpts) (*civitai.Client, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", err
	}
	var client *civitai.Client
	if o.anon {
		client = civitai.New(cfg.BaseURL(), "")
	} else {
		client = civitai.NewWithSource(cfg.BaseURL(), auth.New(cfg))
	}
	client.AllowPrivateDownloadHosts = allowPrivateDownloadHostsForTest
	return client, cfg.BaseURL(), nil
}

// emitJSON pretty-prints the raw API body for scripting consumers. The whole
// promise of --json is output that jq / JSON.parse / json.loads can read, so it
// must be VALID JSON. The Civitai API intermittently returns bodies with raw,
// unescaped C0 control characters (e.g. a literal 0x0D/0x0A inside a prompt
// string) — which is invalid JSON — so before indenting we sanitize any such
// control bytes that appear inside string literals into their proper escapes.
// Already-valid input is passed through untouched (only json.Indent's
// indentation is applied, exactly as before). If the body is invalid for some
// reason sanitizing can't repair, we fall back to emitting the raw bytes.
func emitJSON(cmd *cobra.Command, raw []byte) error {
	src := raw
	if !json.Valid(src) {
		if fixed := civitai.EscapeJSONStringControlChars(src); json.Valid(fixed) {
			src = fixed
		}
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, src, "", "  "); err != nil {
		buf.Reset()
		buf.Write(src)
	}
	fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(buf.String(), "\n"))
	return nil
}

// nonModelFileMarker returns a short human tag naming a version's PRIMARY file
// type when it is NOT model weights (type != "Model") — e.g. a "Workflows"
// model's [Archive], a [Training Data] registration, or [Other]. It's purely
// informational: any file type downloads, so this only flags "this version's
// primary file isn't weights" without claiming it can't be downloaded. Returns
// "" for a normal weights version (or no files).
func nonModelFileMarker(files []civitai.ModelVersionFile) string {
	pf := civitai.PrimaryFile(files)
	if pf == nil || pf.IsModelWeights() {
		return ""
	}
	typ := strings.TrimSpace(pf.Type)
	if typ == "" {
		typ = "Other"
	}
	// 🔴 SERVER TEXT, SO IT GOES THROUGH THE GATE — civitai/cli#399 found this
	// one by driving the renderer rather than by reading it. `files[].type` is
	// uploader-supplied and this marker is INTERPOLATED INTO A HEADER LINE
	// (`printModelVersionDetail`'s first line, and a row of `printModelDetail`'s
	// version table), which is precisely where an escape sequence is worth the
	// most to an attacker. The same field is safeTerm'd in the files table a few
	// lines further down; here it was not, and nothing could see the difference
	// until a hostile fixture was fed through this function.
	//
	// 🔴 safeTermSingle, NOT safeTerm (civitai/cli#552). Both of this marker's
	// call sites are single-line: one is interpolated into a header LINE, the
	// other into a tabwriter CELL, and safeTerm keeps `\n` and `\t` — the one
	// that forges a row at column zero and the one that injects a column. The
	// marker is a bracketed type word; a line break inside it is never
	// legitimate.
	return "[" + safeTermSingle(typ) + "]"
}

// checkLimit validates a --limit against an endpoint's documented maximum. It
// is called with the raw flag value, where an unset limit is 0 and means "use
// the server default page"; that 0 is allowed here. An EXPLICIT non-positive
// --limit (e.g. `--limit 0`) is rejected earlier, in the bindReadFlags PreRunE,
// which unlike this function can see the flag's Changed state and so can tell an
// explicit 0 from an unset default. An out-of-range value is a client-side
// USAGE error (a bad flag value), so it is tagged with ErrUsage — every read
// subcommand that calls checkLimit thus exits with the usage exit code, not the
// generic one.
func checkLimit(limit, max int) error {
	if limit < 0 || limit > max {
		return asUsageError(fmt.Errorf("--limit must be between 1 and %d for this endpoint", max))
	}
	return nil
}

// addIfPositive sets key=v on q when v > 0.
func addIfPositive(q interface{ Set(string, string) }, key string, v int) {
	if v > 0 {
		q.Set(key, strconv.Itoa(v))
	}
}

// addPage validates an EXPLICIT --page against the endpoint's minimum and, when
// it passes, sends it — including a legitimate page 0 on an endpoint whose
// first page is 0. An omitted --page sends nothing (the server's first page).
//
// It needs the flag's Changed state for the same reason the --limit guard in
// bindReadFlags does: the flag's zero default and an explicit `--page 0` are the
// same int, so a value test alone (the old addIfPositive) silently dropped
// `--page 0` and `--page=-1` and answered with page one at exit 0
// (civitai/cli#946). Below the minimum is a usage error, raised before any
// request is made.
func addPage(cmd *cobra.Command, q interface{ Set(string, string) }, page, min int) error {
	if !cmd.Flags().Changed("page") {
		return nil
	}
	if page < min {
		return asUsageError(fmt.Errorf(
			"--page must be at least %d for this endpoint (got %d); omit --page for the first page", min, page))
	}
	q.Set("page", strconv.Itoa(page))
	return nil
}

// addIfSet sets key=v on q when v is non-empty.
func addIfSet(q interface{ Set(string, string) }, key, v string) {
	if v != "" {
		q.Set(key, v)
	}
}

// shellWord renders s as one POSIX shell word: bare when every byte is
// unambiguous to sh, single-quoted (via shellQuote) otherwise.
func shellWord(s string) string {
	if s == "" {
		return shellQuote(s)
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("@%+=:,./_-", r):
		default:
			return shellQuote(s)
		}
	}
	return s
}

// replayFlags renders the command's OWN flags that the user explicitly set, as
// shell words, so a next-page hint continues the same search (civitai/cli#942:
// the hint used to be the bare command path plus --cursor, which dropped
// --query/--limit/--nsfw/--anon and 400'd on a sort-keyed cursor).
//
// Only local flags are replayed — never inherited/global ones — and --cursor and
// --page are skipped because the hint supplies the next value of whichever one
// pages. Every flag is written as --name=value so a value that starts with `-`
// still binds to its flag; a repeatable flag is written once per value. Order is
// pflag's (lexicographic), not the user's — the API does not care.
func replayFlags(cmd *cobra.Command) string {
	var words []string
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Changed || f.Name == "cursor" || f.Name == "page" {
			return
		}
		if f.Value.Type() == "bool" {
			if f.Value.String() == "true" {
				words = append(words, "--"+f.Name)
			} else {
				words = append(words, "--"+f.Name+"="+f.Value.String())
			}
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			for _, v := range sv.GetSlice() {
				words = append(words, "--"+f.Name+"="+shellWord(v))
			}
			return
		}
		words = append(words, "--"+f.Name+"="+shellWord(f.Value.String()))
	})
	if len(words) == 0 {
		return ""
	}
	return " " + strings.Join(words, " ")
}

// printPageFooter prints a compact pagination footer + a hint for fetching the
// next page, when the endpoint returned enough to page. The hint is cmdPath, the
// flags the user explicitly passed (replayFlags), and the next --cursor/--page.
func printPageFooter(cmd *cobra.Command, cmdPath string, m civitai.Metadata) {
	out := cmd.OutOrStdout()
	var parts []string
	if m.TotalItems != nil {
		parts = append(parts, fmt.Sprintf("total items: %d", *m.TotalItems))
	}
	if m.CurrentPage != nil {
		p := fmt.Sprintf("page %d", *m.CurrentPage)
		if m.TotalPages != nil {
			p += fmt.Sprintf("/%d", *m.TotalPages)
		}
		parts = append(parts, p)
	}
	// The cursor is SERVER text printed twice to the terminal (the footer and
	// the paste-ready hint), so it goes through the gate first; single-line,
	// because a newline would split the hint into two commands.
	cursor := safeTermSingle(m.CursorString())
	if cursor != "" {
		parts = append(parts, "next cursor: "+cursor)
	}
	if len(parts) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s\n", strings.Join(parts, "  ·  "))
	switch {
	case cursor != "":
		// Cursors contain a space and a `|` (e.g. 2026-07-19 00:10:35.891|2788964),
		// so the value must be single-quoted or a pasted hint word-splits on the
		// space and pipes on the `|`. shellQuote (not a bare '%s') so a cursor
		// carrying a `'` cannot close the quote early.
		fmt.Fprintf(out, "next page: %s%s --cursor %s\n", cmdPath, replayFlags(cmd), shellQuote(cursor))
	case m.CurrentPage != nil && (m.TotalPages == nil || max(*m.CurrentPage, 1) < *m.TotalPages):
		// tags/creators accept page 0, which the server pages exactly like page 1
		// (it skips nothing) while reporting currentPage 0 — so "current + 1"
		// would hint --page 1 and hand back the same rows. Count page 0 as 1.
		fmt.Fprintf(out, "next page: %s%s --page %d\n", cmdPath, replayFlags(cmd), max(*m.CurrentPage, 1)+1)
	}
}
