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
		if fixed := escapeJSONStringControlChars(src); json.Valid(fixed) {
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

// escapeJSONStringControlChars walks raw JSON bytes tracking in-string state
// (respecting \\ and \") and replaces any raw C0 control byte (0x00–0x1F) that
// appears INSIDE a string literal with its valid JSON escape (\b \f \n \r \t or
// \u00xx). Control bytes outside strings (structural whitespace) and everything
// already escaped are left byte-for-byte unchanged, so valid input round-trips
// identically. This does not attempt to repair other kinds of malformed JSON;
// callers should verify the result with json.Valid before relying on it.
func escapeJSONStringControlChars(raw []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(raw))
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case !inString:
			out.WriteByte(c)
			if c == '"' {
				inString = true
			}
		case escaped:
			// Previous byte was a backslash; this byte is the escape's payload
			// (", \\, /, b, f, n, r, t, or the u of a \uXXXX). Emit verbatim.
			out.WriteByte(c)
			escaped = false
		case c == '\\':
			out.WriteByte(c)
			escaped = true
		case c == '"':
			out.WriteByte(c)
			inString = false
		case c < 0x20:
			// Raw control character inside a string literal — invalid JSON.
			// Rewrite it as the shortest valid escape.
			switch c {
			case '\b':
				out.WriteString(`\b`)
			case '\f':
				out.WriteString(`\f`)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			default:
				fmt.Fprintf(&out, `\u%04x`, c)
			}
		default:
			out.WriteByte(c)
		}
	}
	return out.Bytes()
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
	return "[" + typ + "]"
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

// addIfSet sets key=v on q when v is non-empty.
func addIfSet(q interface{ Set(string, string) }, key, v string) {
	if v != "" {
		q.Set(key, v)
	}
}

// printPageFooter prints a compact pagination footer + a hint for fetching the
// next page, when the endpoint returned enough to page.
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
	cursor := m.CursorString()
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
		// space and pipes on the `|`.
		fmt.Fprintf(out, "next page: %s --cursor '%s'\n", cmdPath, cursor)
	case m.CurrentPage != nil && (m.TotalPages == nil || *m.CurrentPage < *m.TotalPages):
		fmt.Fprintf(out, "next page: %s --page %d\n", cmdPath, *m.CurrentPage+1)
	}
}
