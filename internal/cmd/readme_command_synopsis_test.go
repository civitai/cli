package cmd

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The `## Command reference` table's first column is a SYNOPSIS, not a name:
// `civitai app submit [dir] [--yes] [--package-only] …`. Those bracketed flags
// are a published claim — a reader on github.com has no binary to run
// `--help` against, so the table IS the flag reference for them.
//
// 🔴 Until this file landed the synopses were pinned by NOTHING. The two guards
// that read the table key on the command NAME:
// TestREADMECommandTableDocumentsEveryAppSubcommand matches
// `civitai app <name>` and TestREADMECommandReferencePointsAtTheReadCommandTable
// matches `civitai <name>`, so every character after the name — 88 long-flag
// mentions across 25 rows — could be deleted, misspelled or invented with the
// whole suite green. Only two rows had any synopsis coverage at all: `upgrade`
// (a literal-prefix pin in upgrade_test.go) and `app listing` (a subcommand
// ledger in app_listing_test.go), and neither checks that a named flag EXISTS.
//
// There was no drift on the day this guard was written. It exists so there
// cannot be one silently tomorrow.

// readmeUnescapeCells splits one markdown table row into its cells on
// UNESCAPED `|` only, and unescapes `\|` back to the literal pipe a reader
// sees.
//
// This is not pedantry: the synopses use `|` as an alternation separator
// (`[--track app\|api]`, `[--template static\|page-vite\|page-money]`,
// `--app <slug\|appBlockId>`, and the whole `app listing` subcommand chain), so
// a plain strings.Split(line, "|") truncates those cells mid-flag — the
// `agent-setup` row would arrive cut off after "[--track app\" and five of its
// six flags would never be checked. A guard that silently sees
// a fragment of its subject is the shape this file exists to prevent.
func readmeUnescapeCells(row string) []string {
	var cells []string
	var b strings.Builder
	rs := []rune(row)
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '\\' && i+1 < len(rs) && rs[i+1] == '|':
			b.WriteRune('|')
			i++
		case rs[i] == '|':
			cells = append(cells, b.String())
			b.Reset()
		default:
			b.WriteRune(rs[i])
		}
	}
	cells = append(cells, b.String())
	return cells
}

// readmeCommandSynopses returns the first-column synopsis of every row of the
// `## Command reference` table, backticks stripped, in document order.
//
// Scoped to that one table (heading → the first `### ` after it) for the reason
// readmeCommandTableSubjects gives: `civitai app list` and friends also appear
// in prose and in shell fences, and a whole-file sweep would check strings that
// are not the published synopsis.
func readmeCommandSynopses(t *testing.T, md string) []string {
	t.Helper()
	const heading = "\n## Command reference\n"
	i := strings.Index(md, heading)
	if i < 0 {
		t.Fatal("README.md has no `## Command reference` heading")
	}
	body := md[i+len(heading):]
	if j := strings.Index(body, "\n### "); j >= 0 {
		body = body[:j]
	}

	var out []string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "| `civitai") {
			continue
		}
		cells := readmeUnescapeCells(line)
		if len(cells) < 3 {
			continue
		}
		s := strings.TrimSpace(cells[1])
		s = strings.Trim(s, "`")
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

// readmeSynopsisSegments splits one synopsis on the alternation pipes that sit
// at bracket depth zero, leaving the ones inside `[…]` / `<…>` alone.
//
// `civitai app listing status [--json]|set-text [--tagline <t>] …|set-source-repo
// <url>|--clear|…` is one row documenting ten subcommands; its `--tagline`
// belongs to `set-text` and its `--clear` to whichever subcommand precedes it.
// Splitting here is what lets each flag be checked against the command that
// really owns it, instead of against the group (where a check would pass for
// any flag existing anywhere under `app listing`).
//
// `[--track app|api]` and `--app <slug|appBlockId>` are alternations of VALUES,
// not of commands — they sit inside brackets and stay in one segment.
func readmeSynopsisSegments(s string) []string {
	var segs []string
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '[', '<':
			depth++
		case ']', '>':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				segs = append(segs, strings.TrimSpace(b.String()))
				b.Reset()
				continue
			}
		}
		b.WriteRune(r)
	}
	segs = append(segs, strings.TrimSpace(b.String()))
	return segs
}

// readmeSynopsisFlagRe matches a long flag as the synopsis spells it. Short
// flags are deliberately out of scope: the table writes them only as an aside
// (`-y`/`--yes`) and the long name is the published contract.
var readmeSynopsisFlagRe = regexp.MustCompile(`--[a-z0-9][a-z0-9-]*`)

// readmeCmdWordRe matches a token that could be a command name. `[dir]`,
// `<slug>`, `"<prompt>"` and `--json` all fail it, which is what stops the path
// walk at the end of the real command path.
var readmeCmdWordRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// resolveChildPath walks bare command words down from `from`, returning the
// deepest command reached and how many tokens it consumed. It stops at the
// first token that is not a child, so an argument placeholder ends the walk.
func resolveChildPath(from *cobra.Command, tokens []string) (*cobra.Command, int) {
	cur, n := from, 0
	for _, tok := range tokens {
		if !readmeCmdWordRe.MatchString(tok) {
			break
		}
		var next *cobra.Command
		for _, sub := range cur.Commands() {
			if sub.Name() == tok || sub.HasAlias(tok) {
				next = sub
				break
			}
		}
		if next == nil {
			break
		}
		cur, n = next, n+1
	}
	return cur, n
}

// cobraHasFlag reports whether `c` would accept `--name`, counting the flags it
// inherits from its parents (the root's `--color` / `--no-color` are real on
// every command, and a synopsis is entitled to name one).
func cobraHasFlag(c *cobra.Command, name string) bool {
	return c.Flags().Lookup(name) != nil ||
		c.PersistentFlags().Lookup(name) != nil ||
		c.InheritedFlags().Lookup(name) != nil
}

// cobraFlagNames renders a command's accepted long flags for a failure message,
// so a maintainer who broke a synopsis can see the real spelling without
// opening this test or the command's source.
func cobraFlagNames(c *cobra.Command) string {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, "--"+n)
		}
	}
	c.Flags().VisitAll(func(f *pflag.Flag) { add(f.Name) })
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) { add(f.Name) })
	sort.Strings(names)
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, " ")
}

// TestREADMECommandSynopsesNameRealFlags drives the LIVE Cobra tree — not a
// regex over the command sources — and asserts that every long flag the
// `## Command reference` table's first column names really exists on the
// command it is written under.
//
// Directionality: this is a one-way guard on purpose. It fails when the README
// names a flag the CLI does not have (a published falsehood a reader acts on),
// not when the CLI grows a flag the table omits — the table is a reference, not
// an exhaustive `--help` dump, and `app init`'s row abbreviates deliberately
// (`[--yes] [...]`).
func TestREADMECommandSynopsesNameRealFlags(t *testing.T) {
	root := NewRootCmd()
	synopses := readmeCommandSynopses(t, readREADME(t))

	rows, resolved, checks := len(synopses), 0, 0

	for _, syn := range synopses {
		segs := readmeSynopsisSegments(syn)

		// Segment 0 carries the `civitai …` path; later segments are sibling
		// subcommands of it (`…|set-text …`) or a bare alternative flag
		// (`set-source-repo <url>|--clear`), which belongs to the segment
		// before it.
		if !strings.HasPrefix(segs[0], "civitai ") && segs[0] != "civitai" {
			t.Errorf("command-reference row %q does not start with `civitai `. Every row in that table is a "+
				"`civitai …` synopsis; if that changed, this guard is reading the wrong block.", syn)
			continue
		}

		var prev *cobra.Command
		for si, seg := range segs {
			var cmd *cobra.Command
			switch {
			case si == 0:
				c, n := resolveChildPath(root, strings.Fields(strings.TrimPrefix(seg, "civitai")))
				if n == 0 && c == root {
					t.Errorf("command-reference row %q names no command that exists in the Cobra tree. "+
						"Either the row documents a command that was removed or renamed, or its first "+
						"token is misspelled.", syn)
					continue
				}
				cmd = c
				resolved++
			default:
				// A later segment either names a sibling subcommand of the
				// previous one, or is a bare flag alternative belonging to it.
				toks := strings.Fields(seg)
				if len(toks) > 0 && prev != nil && prev.Parent() != nil {
					if c, n := resolveChildPath(prev.Parent(), toks); n > 0 {
						cmd = c
						resolved++
					}
				}
				if cmd == nil {
					cmd = prev
				}
			}
			if cmd == nil {
				continue
			}
			prev = cmd

			for _, flag := range readmeSynopsisFlagRe.FindAllString(seg, -1) {
				name := strings.TrimPrefix(flag, "--")
				checks++
				if cobraHasFlag(cmd, name) {
					continue
				}
				t.Errorf("README `## Command reference` row\n    %s\nnames %s on `%s`, "+
					"but that command has no such flag.\n    %s accepts: %s\n"+
					"A synopsis is the flag reference for a reader who has not installed the binary — "+
					"fix the README, or restore the flag.",
					syn, flag, cmdFullPath(cmd), cmdFullPath(cmd), cobraFlagNames(cmd))
			}
		}
	}

	// 🔴 POSITIVE CONTROL. Every assertion above lives inside these loops, so a
	// parser that extracts nothing — a renamed heading, a changed table header,
	// a row shape this splitter does not understand — makes the whole test pass
	// while checking not one flag. These three floors are what make the green
	// above mean something; measured 25 rows / 33 command resolutions / 89 flag
	// checks on the day this landed, so each floor sits well below today's
	// value and far above zero.
	t.Logf("command-reference synopses: %d rows, %d command resolutions, %d flag checks", rows, resolved, checks)
	if rows < 20 {
		t.Fatalf("CONTROL failure: parsed %d command-reference rows (want >= 20). The assertions above are "+
			"vacuous at this count — the table extractor is reading the wrong block", rows)
	}
	if resolved < 25 {
		t.Fatalf("CONTROL failure: resolved %d command paths against the Cobra tree (want >= 25). The "+
			"assertions above are vacuous at this count — the synopsis parser is not finding command paths", resolved)
	}
	if checks < 60 {
		t.Fatalf("CONTROL failure: checked %d long flags (want >= 60). The assertions above are vacuous at "+
			"this count — the flag extractor is not seeing the bracketed synopses", checks)
	}
}

// cmdFullPath renders `civitai app listing set-text` for a failure message.
func cmdFullPath(c *cobra.Command) string {
	var parts []string
	for x := c; x != nil; x = x.Parent() {
		parts = append([]string{x.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}
