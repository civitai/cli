package cli_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// saferune_callers_ledger_test.go pins WHO asks internal/saferune its question,
// because two separate prose statements of that set went stale on one commit.
//
// 🔴 THE SET GREW AND BOTH SENTENCES DESCRIBING IT STAYED AT TWO.
// civitai/cli#526's follow-up made pkg/civitai's snippet the third non-test
// caller. AGENTS.md's Layout entry still read "`cmd`'s `safeTerm` and
// `genapi`'s `hasPrintableContent` BOTH call it", and saferune's own package doc
// still opened "IT IS A PACKAGE RATHER THAN A FUNCTION BECAUSE TWO PACKAGES ASK
// THE SAME QUESTION" and then named the two. Both survived a full green suite,
// a lint run and a round-2 audit of the very branch that falsified them, because
// nothing asserted on either.
//
// This is the assertion. It fails when the caller set GROWS (a fourth package
// now shares the class — a decision about the class, so both texts must say so)
// and when it SHRINKS (a caller re-derived the class locally, which is the
// two-tables-that-disagree defect #393 exists to prevent).
type saferuneCaller struct {
	// pkgPath is the import path suffix the caller lives under.
	pkgPath string
	// question is the identifier both prose statements must name — the function
	// that asks. Asserting the IDENTIFIER rather than a phrase is what keeps
	// this from being a guard on wording: the function can be renamed and the
	// texts must move with it, but the sentence around it is free.
	question string
	// why is the question that call site asks, for a reader of this ledger.
	why string
}

var saferuneCallers = []saferuneCaller{
	{
		pkgPath:  "internal/cmd",
		question: "safeTerm",
		why:      "may this rune reach the terminal — the HUMAN renderers",
	},
	{
		pkgPath:  "internal/genapi",
		question: "hasPrintableContent",
		why:      "would this string still say anything once a renderer stripped it",
	},
	{
		pkgPath: "pkg/civitai",
		// snippet is the FUNCTION that asks; read.go is where the import sits.
		question: "snippet",
		why: "what may an ERROR STRING carry — cmd/civitai/main.go prints " +
			"`Error: <err>` to stderr with no renderer in front of it",
	},
}

const saferuneImportPath = "github.com/civitai/cli/internal/saferune"

// countWords renders len(saferuneCallers) as the word the two prose statements
// must use, so the NUMBER moves with the set rather than being retyped. Growing
// the ledger past this table is a deliberate stop: a fifth caller is a decision
// about the class, not a bump.
var countWords = map[int]string{2: "TWO", 3: "THREE", 4: "FOUR"}

// TestSaferuneCallersAreLedgered is round 3's finding 2.
func TestSaferuneCallersAreLedgered(t *testing.T) {
	files := goSourceFiles(t)
	// POSITIVE CONTROL on the walk itself, before any verdict: a walk that found
	// nothing reports "no unledgered callers", which is the reassuring zero.
	if len(files) < 100 {
		t.Fatalf("CONTROL failure, not a finding: walked only %d non-test .go files "+
			"in the module — the scan is not reading the tree", len(files))
	}

	fset := token.NewFileSet()
	importers := map[string]bool{}
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			if imp.Path == nil {
				continue
			}
			if strings.Trim(imp.Path.Value, `"`) != saferuneImportPath {
				continue
			}
			importers[filepath.ToSlash(filepath.Dir(path))] = true
		}
	}
	// POSITIVE CONTROL on the MATCH: an import-path typo, or a parser that read
	// no imports, yields an empty set that would pass a "grew?" check silently.
	if len(importers) == 0 {
		t.Fatalf("CONTROL failure, not a finding: no non-test file in the module "+
			"imports %s. The package has callers; the scan is broken.", saferuneImportPath)
	}

	var got []string
	for pkg := range importers {
		got = append(got, pkg)
	}
	sort.Strings(got)
	var want []string
	for _, c := range saferuneCallers {
		want = append(want, c.pkgPath)
	}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("packages importing internal/saferune are %v, ledgered as %v.\n"+
			"GREW: a fourth package now asks the class's question. That is a decision\n"+
			"      about the class — read saferune's package doc (the strip is\n"+
			"      unconditional and must never see what the USER typed), add an entry\n"+
			"      here, and update BOTH prose statements this test checks below.\n"+
			"SHRANK: a caller stopped importing it, which usually means it re-derived\n"+
			"      the class locally — the two-tables-that-disagree defect of #393.",
			got, want)
	}

	// The two prose statements. Each must name every ledgered question and the
	// count word derived from the ledger's own length.
	word, ok := countWords[len(saferuneCallers)]
	if !ok {
		t.Fatalf("CONTROL failure, not a finding: countWords has no word for %d "+
			"callers — extend it, and say in saferune's package doc why the class "+
			"is now shared that widely", len(saferuneCallers))
	}
	// 🔴 EACH TEXT IS NARROWED TO THE PASSAGE THAT MAKES THE CLAIM BEFORE IT IS
	// SEARCHED. The first cut of this test searched the WHOLE of AGENTS.md, and
	// a mutant that deleted `snippet` from the Layout entry SURVIVED: the file
	// also contains "what an error snippet prints" in item 38's trigger, so a
	// whole-file Contains was green about a sentence that no longer named the
	// caller. A guard on a document must be scoped to the sentence it guards, or
	// it is asserting on the document's vocabulary.
	pkgDoc := saferunePackageDoc(t)
	layout := agentsSaferuneEntry(t)
	for _, doc := range []struct {
		text     string
		what     string
		backtick bool
	}{
		{pkgDoc, "saferune's package doc", false},
		{layout, "AGENTS.md's Layout entry for internal/saferune", true},
	} {
		for _, c := range saferuneCallers {
			needle := c.question
			if doc.backtick {
				// In AGENTS.md an identifier is a code span. Requiring the
				// backticks is what stops the prose word "snippet" satisfying a
				// claim about the FUNCTION snippet.
				needle = "`" + c.question + "`"
			}
			if !strings.Contains(doc.text, needle) {
				t.Errorf("%s does not name %s (%s), which imports internal/saferune.\n"+
					"Both statements of this set went stale at once when it grew to %d; "+
					"this is what stops that happening silently again.\ngot:\n%s",
					doc.what, needle, c.pkgPath, len(saferuneCallers), doc.text)
			}
		}
	}
	// The COUNT is a separate claim from the membership and goes stale on its
	// own: saferune's package doc opens with it in words.
	if !strings.Contains(pkgDoc, word+" PACKAGES ASK THE SAME") {
		t.Errorf("saferune's package doc does not say %q — it must state the count "+
			"the ledger has (%d), and it said TWO for a full branch after the third "+
			"caller landed.", word+" PACKAGES ASK THE SAME", len(saferuneCallers))
	}
}

// saferunePackageDoc returns internal/saferune's package doc comment, parsed
// rather than grepped so "the package doc" means the package doc and not any
// comment that happens to be in the file.
func saferunePackageDoc(t *testing.T) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "internal/saferune/saferune.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot parse saferune.go: %v", err)
	}
	if f.Doc == nil {
		t.Fatalf("CONTROL failure, not a finding: internal/saferune/saferune.go has " +
			"no package doc comment — the checks below would be vacuous")
	}
	return f.Doc.Text()
}

// agentsSaferuneEntry returns just the `internal/saferune` bullet from
// AGENTS.md's Layout section: from the line opening it to the next top-level
// bullet.
func agentsSaferuneEntry(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read AGENTS.md: %v", err)
	}
	const opener = "- `internal/saferune`"
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, opener) {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("CONTROL failure, not a finding: AGENTS.md has no Layout bullet "+
			"starting %q — this test cannot see the sentence it guards", opener)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "- ") || strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	entry := strings.Join(lines[start:end], "\n")
	// POSITIVE CONTROL on the extraction: a one-line slice means the bullet
	// scanner is wrong, and every Contains below would fail for that reason
	// rather than for a finding.
	if end-start < 3 {
		t.Fatalf("CONTROL failure, not a finding: the internal/saferune bullet "+
			"extracted to %d lines:\n%s", end-start, entry)
	}
	return entry
}

// goSourceFiles returns every non-test .go file in the module.
func goSourceFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != "." && (strings.HasPrefix(name, ".") || name == "testdata" ||
				name == "node_modules" || name == "dist" || name == "bin") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	sort.Strings(out)
	return out
}
