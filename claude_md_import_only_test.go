package cli_test

import (
	"os"
	"strings"
	"testing"
)

// CLAUDE.md IS AN IMPORT SHIM. IT IS NOT A SECOND PLACE TO PUT GUIDANCE.
//
// AGENTS.md is the canonical agent doc because it is the cross-tool one: Cursor,
// Codex, Copilot and Gemini all read `AGENTS.md`, and none of them resolve
// Claude Code's `@file` import syntax. So a rule written into CLAUDE.md reaches
// exactly one tool, while reading — to the person who wrote it — as guidance the
// project has.
//
// 🔴 TWO DOCS DRIFT, AND THE ONE THAT IS NOT CANONICAL IS THE ONE THAT GOES
// STALE. That is not a prediction; it is what this repo measured before the
// files were collapsed. CLAUDE.md carried four bullets. One
// (`make ci` omits lint) was a THIRD copy of a rule AGENTS.md already states
// twice, so it had three places to rot in. Two more were load-bearing rules
// AGENTS.md did not state AT ALL — that `README.md` is the published user
// contract, and that "README" is three surfaces which go stale independently —
// so every agent that is not Claude Code shipped behaviour changes without them.
// The fourth ("AGENTS.md is the single source of truth; keep guidance there")
// was a rule the file itself was violating in the three bullets around it.
//
// Prose asking the next maintainer to keep CLAUDE.md empty is what failed the
// first time, so this is the deterministic version: the file's whole content is
// pinned, not merely its size or a keyword. A new bullet fails here by name, and
// the fix is to write it in AGENTS.md — where every tool reads it, and where
// agents_size_test.go's ceiling prices it.

// claudeMDImport is the ONE line CLAUDE.md may carry. It is the Claude-Code
// import that makes AGENTS.md load into a session; without it the collapse
// would have silently removed AGENTS.md from every Claude Code session, which
// is why its presence is a CONTROL below rather than just another line.
const claudeMDImport = "@AGENTS.md"

// claudeMDResidue returns every line that is neither blank nor the import —
// i.e. guidance CLAUDE.md has grown of its own.
//
// It is the ONE classifier: the live check and the negative-control corpus both
// call it, so narrowing what it reports fails the corpus by name instead of
// quietly widening what CLAUDE.md may contain.
func claudeMDResidue(body string) []string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		s := strings.TrimSpace(l)
		if s == "" || s == claudeMDImport {
			continue
		}
		out = append(out, s)
	}
	return out
}

// TestClaudeMDIsTheImportAndNothingElse is the guard nothing provided before:
// `grep CLAUDE.md *_test.go` returned zero hits, so the file could regrow a
// second copy of the project's guidance with the whole suite green.
func TestClaudeMDIsTheImportAndNothingElse(t *testing.T) {
	raw, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v (CONTROL failure — the file this guard measures was not found, so nothing below checked anything)", err)
	}
	body := string(raw)

	// POSITIVE CONTROL, and it is the load-bearing half. An EMPTY or deleted
	// CLAUDE.md trivially satisfies "carries no guidance of its own" while
	// having removed AGENTS.md from every Claude Code session — the reassuring
	// zero. So the import must be present as its own line before the residue
	// check below is allowed to mean anything.
	found := false
	for _, l := range strings.Split(body, "\n") {
		if strings.TrimSpace(l) == claudeMDImport {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("CONTROL failure, not a finding: CLAUDE.md does not contain the line %q.\n"+
			"That import is the only thing loading AGENTS.md into a Claude Code session. Without it this guard would report "+
			"a serene pass over a file that has stopped doing its one job — and every rule in AGENTS.md would silently "+
			"stop reaching Claude Code while still reaching Cursor, Codex, Copilot and Gemini.\nfile content:\n%s", claudeMDImport, body)
	}

	if residue := claudeMDResidue(body); len(residue) > 0 {
		t.Fatalf("CLAUDE.md carries %d line(s) of guidance of its own:\n  %s\n\n"+
			"🔴 PUT IT IN AGENTS.md INSTEAD. AGENTS.md is canonical because it is the CROSS-TOOL doc — Cursor, Codex, Copilot and "+
			"Gemini read it, and none of them resolve the `@file` import syntax, so a rule written here reaches Claude Code alone "+
			"while reading like a project rule.\n"+
			"And two docs drift: the one that is not canonical is the one that goes stale, because nobody edits it when the "+
			"behaviour changes. That is measured, not feared — the bullets this file used to carry were one THIRD copy of a rule "+
			"AGENTS.md already stated twice, plus two rules AGENTS.md did not state at all.\n"+
			"If the rule is worth keeping it is worth its bytes in AGENTS.md, where agents_size_test.go prices it and every tool "+
			"reads it. This file is the import and nothing else.",
			len(residue), strings.Join(residue, "\n  "))
	}
	t.Logf("CLAUDE.md is %d bytes and carries only the %q import", len(raw), claudeMDImport)
}

// TestClaudeMDResidueClassifierCanGoRed is the NEGATIVE CONTROL. Without it,
// "CLAUDE.md has no residue" is satisfiable by a classifier that reports nothing
// — and the live file is, by construction, the one input on which a
// wired-to-nothing classifier and a correct one agree.
//
// The reject corpus is built from the ACTUAL bullets this file used to carry,
// not from a textbook fixture, because a scanner allowlisting its own canonical
// examples is how a clean scan gets believed.
func TestClaudeMDResidueClassifierCanGoRed(t *testing.T) {
	accept := []struct{ name, in string }{
		{"the collapsed file", "@AGENTS.md\n"},
		{"no trailing newline", "@AGENTS.md"},
		{"surrounded by blank lines", "\n\n@AGENTS.md\n\n"},
		{"CRLF line endings", "@AGENTS.md\r\n"},
		{"indented import", "  @AGENTS.md\n"},
	}
	for _, tc := range accept {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			if got := claudeMDResidue(tc.in); len(got) > 0 {
				t.Errorf("claudeMDResidue(%q) reported %v — a bare import is the one shape this file may have", tc.in, got)
			}
		})
	}

	reject := []struct{ name, in string }{
		{"a heading", "@AGENTS.md\n\n## Claude-specific\n"},
		{"the make-targets bullet that was a third copy",
			"@AGENTS.md\n\n- This is a single-binary Go project. Use the `make` targets — never reach for a\n  non-Go package manager.\n"},
		{"the self-referential single-source-of-truth bullet",
			"@AGENTS.md\n\n- `AGENTS.md` is the single source of truth for **agent guidance**; keep curated\n  guidance there, not here.\n"},
		{"the README-contract bullet that AGENTS.md did not have",
			"@AGENTS.md\n\n- The split: `AGENTS.md` holds **decisions and rationale**; `README.md` is the\n  **published user contract**.\n"},
		{"a lone sentence with no bullet or heading", "@AGENTS.md\n\nAlways run make lint before claiming done.\n"},
		{"an HTML comment, which is still content to maintain", "@AGENTS.md\n\n<!-- remember to run make lint -->\n"},
		{"a second import, which is a second doc to keep in step", "@AGENTS.md\n@README.md\n"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if got := claudeMDResidue(tc.in); len(got) == 0 {
				t.Errorf("claudeMDResidue(%q) reported nothing — this is guidance living outside the canonical doc and the guard cannot see it", tc.in)
			}
		})
	}

	// POSITIVE CONTROL for the corpus itself: both halves must be non-trivial,
	// or "the classifier agreed with every case" is a claim about an empty table.
	if len(accept) < 3 || len(reject) < 5 {
		t.Fatalf("CONTROL failure: the corpus is %d accept / %d reject case(s); a table this small cannot pin the classifier",
			len(accept), len(reject))
	}
}
