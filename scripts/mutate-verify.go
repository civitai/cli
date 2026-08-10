//go:build ignore

// mutate-verify re-classifies a gremlins mutation report by asking the COMPILER
// what gremlins cannot ask itself.
//
// WHY THIS EXISTS. gremlins decides a mutant's fate from `go test`'s EXIT CODE:
//
//	// gremlins@v0.6.0 internal/engine/executor.go
//	func getTestFailedStatus(exitCode int) mutator.Status {
//	    case 1: return mutator.Killed
//	    case 2: return mutator.NotViable
//	    default: return mutator.Lived
//	    }
//
// `go test` exits 1 for BOTH "a test failed" and "the package did not compile",
// so every non-compiling mutant is scored KILLED and gremlins' NOT VIABLE bucket
// is effectively dead code. Measured on internal/validate at 8ec3cb0: gremlins
// reported 146 KILLED / 0 NOT VIABLE, and 36 of those 146 do not compile —
// almost all ARITHMETIC_BASE `+`->`-` applied to STRING CONCATENATION, which
// this package is full of. That is 24.7% of the reported kills, inflating the
// headline efficacy from a true 82.7% to 86.4%.
//
// So: this tool re-applies each mutant gremlins called KILLED, runs `go build`,
// and demotes the ones that fail to compile to NOT VIABLE. A non-compiling
// mutant is evidence about neither the tests nor the code.
//
// ============================ WHAT THIS DOES NOT COVER ======================
//
//  1. IT IS NOT A GUARD AND CANNOT FAIL A BUILD. It reports; it gates nothing.
//     Nothing here belongs in CI until the signal-to-noise is agreed — see
//     claudedocs/mutation-testing-experiment.md.
//
//  2. IT ONLY MUTATES PRODUCTION CODE, SO IT CANNOT SEE A DEFECT THAT LIVES IN
//     TEST CODE. A `t.Skip` that skips a whole row, a `POSITIVE CONTROL` block
//     whose condition is `s == s`, an assertion whose expected path can never
//     match, a mutant killed by a BYSTANDER assertion in the same test — all of
//     these leave a green mutation report. Mutation testing answers "is this
//     line pinned by something?", never "is it pinned by the assertion that
//     claims to pin it?".
//
//  3. IT CANNOT SEE ANY MUTATION GREMLINS DOES NOT GENERATE. gremlins mutates
//     operators only: arithmetic, comparison boundaries, comparison negation,
//     &&/||, ++/--, break/continue, bitwise, assignment ops. It does NOT mutate
//     string literals, numeric CONSTANTS, function arguments, argument ORDER,
//     struct field selection, or a return value. Most of internal/validate is
//     message construction, and none of it is reachable by this tool.
//
//  4. NOT COVERED != SAFE. gremlins does not execute a mutant on a line no test
//     covers; those are reported as NOT COVERED and are a coverage question,
//     not a mutation result. There were 67 of them here.
//
//  5. SURVIVORS ARE A TRIAGE QUEUE, NOT A BUG LIST. Some are EQUIVALENT MUTANTS
//     (provably cannot change behaviour). "Equivalent" is a CLAIM — prove it
//     with an input that would discriminate, not with reasoning.
//
//  6. ONE PACKAGE PER RUN. gremlins' JSON carries a BASENAME (`finding.go`), not
//     a path, so mutants from two packages holding a same-named file cannot be
//     told apart. -pkg is therefore a single package directory.
//
//  7. THE COMPILE CHECK IS `go build`, NOT `go vet`. A mutant that compiles but
//     trips a vet check `go test` would run is still counted viable here.
//
// ============================================================================
//
// Usage: go run scripts/mutate-verify.go -pkg internal/validate -json out.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type mutation struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type fileReport struct {
	FileName  string     `json:"file_name"`
	Mutations []mutation `json:"mutations"`
}

type report struct {
	Files []fileReport `json:"files"`
}

// tables mirror gremlins' own token maps (internal/engine/tokenmutator.go). A
// mutant is re-created by rewriting the token at the reported BYTE column.
var tables = map[string]map[string]string{
	"ARITHMETIC_BASE":         {"+": "-", "-": "+", "*": "/", "/": "*", "%": "*"},
	"CONDITIONALS_BOUNDARY":   {">": ">=", "<": "<=", ">=": ">", "<=": "<"},
	"CONDITIONALS_NEGATION":   {"==": "!=", "!=": "==", "<=": ">", ">=": "<", "<": ">=", ">": "<="},
	"INVERT_LOGICAL":          {"&&": "||", "||": "&&"},
	"INCREMENT_DECREMENT":     {"++": "--", "--": "++"},
	"INVERT_LOOPCTRL":         {"break": "continue", "continue": "break"},
	"INVERT_ASSIGNMENTS":      {"+=": "-=", "-=": "+=", "*=": "/=", "/=": "*=", "%=": "*="},
	"REMOVE_SELF_ASSIGNMENTS": {"+=": "=", "-=": "=", "*=": "=", "/=": "=", "%=": "=", "&=": "=", "|=": "=", "^=": "="},
	"INVERT_BITWISE":          {"&": "|", "|": "&", "^": "&"},
	"INVERT_BWASSIGN":         {"&=": "|=", "|=": "&=", "^=": "&="},
	"INVERT_NEGATIVES":        {"-": ""},
}

// applyAt rewrites the token at 1-based BYTE column col. Go token columns count
// bytes, and these files contain multi-byte em dashes, so a rune-indexed
// implementation silently edits the wrong offset.
func applyAt(line string, col int, mtype string) (string, string, string, bool) {
	tbl, ok := tables[mtype]
	if !ok {
		return "", "", "", false
	}
	toks := make([]string, 0, len(tbl))
	for t := range tbl {
		toks = append(toks, t)
	}
	// Longest first so ">=" is matched before ">".
	sort.Slice(toks, func(i, j int) bool { return len(toks[i]) > len(toks[j]) })

	b := []byte(line)
	i := col - 1
	if i < 0 || i > len(b) {
		return "", "", "", false
	}
	for _, t := range toks {
		if i+len(t) <= len(b) && string(b[i:i+len(t)]) == t {
			return string(b[:i]) + tbl[t] + string(b[i+len(t):]), t, tbl[t], true
		}
	}
	return "", "", "", false
}

type verdict struct {
	file, mtype, swap, err string
	line, col              int
}

func main() {
	pkg := flag.String("pkg", "", "package directory the report is about, e.g. internal/validate")
	jsonPath := flag.String("json", "", "gremlins -o JSON report")
	flag.Parse()
	if *pkg == "" || *jsonPath == "" {
		fmt.Fprintln(os.Stderr, "usage: mutate-verify -pkg <dir> -json <report.json>")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read report: %v\n", err)
		os.Exit(1)
	}
	var rep report
	if err := json.Unmarshal(raw, &rep); err != nil {
		fmt.Fprintf(os.Stderr, "parse report: %v\n", err)
		os.Exit(1)
	}

	var killed, lived, notCovered []verdict
	for _, f := range rep.Files {
		for _, m := range f.Mutations {
			v := verdict{file: f.FileName, line: m.Line, col: m.Column, mtype: m.Type}
			switch m.Status {
			case "KILLED":
				killed = append(killed, v)
			case "LIVED":
				lived = append(lived, v)
			case "NOT COVERED":
				notCovered = append(notCovered, v)
			}
		}
	}

	var nonViable, realKills, unapplied []verdict
	for _, v := range killed {
		path := filepath.Join(*pkg, v.file)
		orig, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
			os.Exit(1)
		}
		lines := strings.Split(string(orig), "\n")
		if v.line-1 < 0 || v.line-1 >= len(lines) {
			unapplied = append(unapplied, v)
			continue
		}
		newLine, from, to, ok := applyAt(lines[v.line-1], v.col, v.mtype)
		if !ok {
			unapplied = append(unapplied, v)
			continue
		}
		v.swap = from + "->" + to
		lines[v.line-1] = newLine
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		out, buildErr := exec.Command("go", "build", "./"+*pkg+"/").CombinedOutput()
		// Restore before doing anything else — a panic here would leave the
		// tree mutated, which is the one unrecoverable failure mode.
		if werr := os.WriteFile(path, orig, 0o644); werr != nil {
			fmt.Fprintf(os.Stderr, "FATAL: could not restore %s: %v\n", path, werr)
			os.Exit(1)
		}
		if buildErr != nil {
			ls := strings.Split(strings.TrimSpace(string(out)), "\n")
			v.err = ls[len(ls)-1]
			nonViable = append(nonViable, v)
		} else {
			realKills = append(realKills, v)
		}
	}

	fmt.Printf("package            : %s\n", *pkg)
	fmt.Printf("gremlins reported  : KILLED=%d LIVED=%d NOT_COVERED=%d NOT_VIABLE=0\n",
		len(killed), len(lived), len(notCovered))
	fmt.Printf("compiler says      : of %d 'kills', %d DO NOT COMPILE\n", len(killed), len(nonViable))
	fmt.Println()
	fmt.Printf("CORRECTED          : KILLED=%d LIVED=%d NOT_VIABLE=%d NOT_COVERED=%d",
		len(realKills), len(lived), len(nonViable), len(notCovered))
	if len(unapplied) > 0 {
		fmt.Printf(" UNVERIFIED=%d", len(unapplied))
	}
	fmt.Println()
	if n := len(realKills) + len(lived); n > 0 {
		fmt.Printf("corrected efficacy : %.1f%%  (gremlins claimed %.1f%%)\n",
			100*float64(len(realKills))/float64(n),
			100*float64(len(killed))/float64(len(killed)+len(lived)))
	}

	sortV := func(v []verdict) {
		sort.Slice(v, func(i, j int) bool {
			if v[i].file != v[j].file {
				return v[i].file < v[j].file
			}
			if v[i].line != v[j].line {
				return v[i].line < v[j].line
			}
			return v[i].col < v[j].col
		})
	}

	sortV(lived)
	fmt.Printf("\n=== SURVIVORS (%d) — triage these by hand ===\n", len(lived))
	for _, v := range lived {
		fmt.Printf("  %s:%d:%d  %s\n", v.file, v.line, v.col, v.mtype)
	}

	sortV(nonViable)
	fmt.Printf("\n=== NON-VIABLE (%d) — gremlins scored each of these KILLED ===\n", len(nonViable))
	for _, v := range nonViable {
		fmt.Printf("  %s:%d:%d  %s (%s)\n     %s\n", v.file, v.line, v.col, v.mtype, v.swap, v.err)
	}

	if len(unapplied) > 0 {
		sortV(unapplied)
		fmt.Printf("\n=== UNVERIFIED (%d) — this tool could not re-create the mutant ===\n", len(unapplied))
		fmt.Println("  (a gap in THIS tool, not a gremlins finding — its viability is unknown)")
		for _, v := range unapplied {
			fmt.Printf("  %s:%d:%d  %s\n", v.file, v.line, v.col, v.mtype)
		}
	}
}
