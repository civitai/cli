package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/auth"
	"github.com/civitai/cli/internal/config"
	"github.com/spf13/cobra"
)

// readOpts are the flags shared by every read subcommand.
type readOpts struct {
	json bool
	anon bool
}

// bindReadFlags attaches the shared --json / --anon flags to a read subcommand.
func bindReadFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "print the raw API JSON response (for scripting)")
	cmd.Flags().Bool("anon", false, "force an anonymous request (ignore any stored login token)")
}

// readFlags reads the shared --json / --anon flag values off cmd.
func readFlags(cmd *cobra.Command) *readOpts {
	j, _ := cmd.Flags().GetBool("json")
	a, _ := cmd.Flags().GetBool("anon")
	return &readOpts{json: j, anon: a}
}

// newReader builds an api.Client for the public read endpoints. It sends the
// stored login token when present (OAuth device-login or personal key, both
// refreshed transparently) unless --anon is set, and works fully anonymously
// when no token is configured — the public read routes don't require auth.
func newReader(o *readOpts) (*api.Client, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", err
	}
	if o.anon {
		return api.New(cfg.BaseURL(), "", ""), cfg.BaseURL(), nil
	}
	return api.NewWithSource(cfg.BaseURL(), auth.New(cfg), ""), cfg.BaseURL(), nil
}

// emitJSON pretty-prints the raw API body (falling back to the raw bytes if it
// is not valid JSON) and reports whether output was handled.
func emitJSON(cmd *cobra.Command, raw []byte) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		buf.Reset()
		buf.Write(raw)
	}
	fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(buf.String(), "\n"))
	return nil
}

// checkLimit validates a --limit against an endpoint's documented maximum. A
// zero limit means "server default" and is always allowed.
func checkLimit(limit, max int) error {
	if limit < 0 || limit > max {
		return fmt.Errorf("--limit must be between 1 and %d for this endpoint", max)
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
func printPageFooter(cmd *cobra.Command, cmdPath string, m api.Metadata) {
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
		fmt.Fprintf(out, "next page: %s --cursor %s\n", cmdPath, cursor)
	case m.CurrentPage != nil && (m.TotalPages == nil || *m.CurrentPage < *m.TotalPages):
		fmt.Fprintf(out, "next page: %s --page %d\n", cmdPath, *m.CurrentPage+1)
	}
}
