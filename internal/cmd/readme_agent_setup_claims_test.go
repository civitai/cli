package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file pins the contract claims in `## Set up your coding agent
// (agent-setup)`, and it exists because a measured mutation showed that NONE of
// them were pinned: deleting all 200-odd body lines of that section — keeping
// only the five `###` headings — left `go test ./...` fully green. The headings
// are guarded (readme_nav_test.go resolves their anchors); every sentence under
// them was not.
//
// That gap is the same one #639 closed for `## Submit & auth`, and the same
// reason #635 needed four audit rounds: each round shipped a false claim into
// the published user contract with the suite green, and the next round found it
// by READING rather than by running anything. Round 1 of #641 then found three
// more in this section, all measured against the binary:
//
//	F1  the README pointed readers at `civitai agent-setup --help` for the
//	    "JSONC/symlink handling" — `Long` says nothing whatever about symlinks.
//	F2  it said an absent Authorization header is "reported" by `--check`.
//	    A real run emits six rows and none of them is about a header.
//	F3  it said paths in `--json` are "always absolute". A `manual` row
//	    carries an empty `path`.
//
// 🔴 A PROSE PIN ALONE WOULD BE THE WRONG GUARD — the same judgement
// readme_submit_entry_block_test.go records. A string pin catches drift in the
// prose and goes STALE the moment the code legitimately changes, still
// asserting a sentence about behaviour that has moved. So each guard below pins
// the RELATIONSHIP between the sentence and the thing it describes, and fails
// when that relationship changes in EITHER direction.

func readmeAgentSetupSection(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	const start = "## Set up your coding agent"
	i := strings.Index(string(raw), start)
	if i < 0 {
		t.Fatalf("PREMISE BROKEN: README.md has no %q heading — this file is "+
			"guarding a section that no longer exists", start)
	}
	rest := string(raw)[i+len(start):]
	j := strings.Index(rest, "\n## ")
	if j < 0 {
		t.Fatalf("PREMISE BROKEN: no `## ` heading follows %q, so the section "+
			"has no end and this guard would read the rest of the file", start)
	}
	sec := rest[:j]
	// A positive control on the extractor itself: a section this short means the
	// slice is wrong, and every Contains() below would pass vacuously.
	if len(sec) < 2000 {
		t.Fatalf("extracted only %d bytes for the agent-setup section — the "+
			"extractor is reading the wrong text, so the assertions below are "+
			"vacuous", len(sec))
	}
	return sec
}

// TestREADMEVerdictExemptionsAreLedgeredAgainstTheCode pins F2's neighbour: the
// sentence naming which rows are EXCLUDED from `--check`'s verdict.
//
// 🔴 THIS IS A LEDGER, NOT A COUNT. It derives the exempt set from
// checkCountsTowardVerdict itself, so adding a third exemption without saying so
// in the README is red, and so is removing one while the README still names it.
// A count would let add-one/remove-one swap an exemption silently.
func TestREADMEVerdictExemptionsAreLedgeredAgainstTheCode(t *testing.T) {
	sec := readmeAgentSetupSection(t)

	// Every check name the command can emit, with what the README must say about
	// each. Derived from the constants, not retyped, so a new check name that is
	// silently exempted cannot pass unnoticed.
	all := []string{checkCLIVersion, checkAgentsMD, checkClaudeMD, checkAuthenticated}

	var exemptForOther, exemptForClaude []string
	for _, name := range all {
		if !checkCountsTowardVerdict(name, "cursor") {
			exemptForOther = append(exemptForOther, name)
		}
		if !checkCountsTowardVerdict(name, agentClaude) {
			exemptForClaude = append(exemptForClaude, name)
		}
	}

	// PREMISE, asserted rather than assumed: if these ever coincide, the
	// README's "for any agent other than claude" qualifier is describing a
	// distinction the code no longer makes, and this test is measuring nothing.
	if len(exemptForOther) == len(exemptForClaude) {
		t.Fatalf("PREMISE BROKEN: the exempt set no longer depends on the agent "+
			"(claude=%v, other=%v). The README sentence carries an agent "+
			"qualifier that the code does not — rewrite both.", exemptForClaude, exemptForOther)
	}

	for _, name := range exemptForClaude {
		if !strings.Contains(sec, "`"+name+"`") {
			t.Errorf("checkCountsTowardVerdict exempts %q from the verdict for EVERY agent, "+
				"and the agent-setup section never names it. A `--json` consumer folding that "+
				"row into its own pass/fail gets a false failure.", name)
		}
	}
	// The agent-conditional exemption must be named WITH its condition.
	for _, name := range exemptForOther {
		if contains(exemptForClaude, name) {
			continue
		}
		if !strings.Contains(sec, "`"+name+"`") {
			t.Errorf("checkCountsTowardVerdict exempts %q for a non-claude agent and the "+
				"README never names it", name)
			continue
		}
		if !strings.Contains(sec, "other than `claude`") {
			t.Errorf("%q is exempt ONLY for agents other than claude, and the README states "+
				"the exemption without that condition — which claims claude users get it too", name)
		}
	}
}

// TestCheckEmitsNoRowAboutAnAbsentHeader pins F2 directly, in both directions:
// the README now says an absent Authorization header gets "no row of its own",
// and this fails if one ever appears (making the README stale) — the direction a
// prose pin cannot see.
func TestCheckEmitsNoRowAboutAnAbsentHeader(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "zed"); err != nil {
		t.Fatalf("agent-setup --agent zed: %v", err)
	}
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "zed", "--check", "--json")
	if err != nil {
		t.Fatalf("--check --json: %v", err)
	}
	var payload struct {
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode --check --json: %v\n%s", err, out)
	}
	// Positive control: the run must have produced rows at all, or the loop
	// below is vacuous and would pass with the check machinery unwired.
	if len(payload.Checks) < 4 {
		t.Fatalf("only %d check row(s) — the payload is not what this test reads, so "+
			"the assertion below proves nothing\n%s", len(payload.Checks), out)
	}
	// PREMISE: zed is the documented no-header agent, so this really is the
	// absent-header case rather than a config that has one after all.
	if got := agentTargets[agentZed].EnvHeaderSyntax; got != "" {
		t.Fatalf("PREMISE BROKEN: zed now documents a header syntax (%q), so this run is "+
			"not the absent-header case the README describes", got)
	}
	for _, c := range payload.Checks {
		if c.Name == checkAuthenticated {
			continue
		}
		low := strings.ToLower(c.Name)
		if strings.Contains(low, "header") || strings.Contains(low, "authorization") {
			t.Errorf("`--check` now emits row %q about the Authorization header. The "+
				"agent-setup section tells readers there is NO row of its own for it and "+
				"to read the config file instead — update that paragraph.", c.Name)
		}
	}
}

// TestManualRowCarriesAnEmptyPath pins F3: the exception the README states to
// its own "a path in --json is always absolute" claim. If a manual row ever
// gains a real path, the README's exception becomes a lie in the other
// direction, and this fails.
func TestManualRowCarriesAnEmptyPath(t *testing.T) {
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "other", "--json")
	if err != nil {
		t.Fatalf("agent-setup --agent other --json: %v", err)
	}
	var payload struct {
		Changes []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode --json: %v\n%s", err, out)
	}
	var seenManual bool
	for _, c := range payload.Changes {
		if c.Action != actionManual {
			// The README's rule for every OTHER row: absolute.
			if c.Path != "" && !filepath.IsAbs(c.Path) {
				t.Errorf("row %q carries a non-absolute path %q; the README says a path in "+
					"`--json` is always absolute whatever --dir you passed", c.Action, c.Path)
			}
			continue
		}
		seenManual = true
		if c.Path != "" {
			t.Errorf("a `manual` row now carries path %q. The README states the manual row "+
				"as the ONE exception to absolute paths and says it is empty — that "+
				"exception is now wrong.", c.Path)
		}
	}
	if !seenManual {
		t.Fatalf("`--agent other` produced no `manual` row, so this test asserted nothing "+
			"about the exception it exists to pin\n%s", out)
	}
}

// TestTheHelpPointerIsHonoured pins F1 — the defect that shipped. The README
// tells the reader `--help` carries certain material; that is a CROSS-SURFACE
// claim, and nothing else in the tree checks it. `Long` can lose any of these
// without a single test noticing, leaving the README pointing at content that
// is not there.
//
// 🔴 The topics are the ones the README's pointer sentence NAMES. If you change
// that sentence, change this list — and if a topic leaves `Long`, take it out of
// the sentence rather than out of this test.
func TestTheHelpPointerIsHonoured(t *testing.T) {
	sec := readmeAgentSetupSection(t)
	const pointer = "`civitai agent-setup --help` carries"
	if !strings.Contains(sec, pointer) {
		t.Fatalf("PREMISE BROKEN: the agent-setup section no longer points readers at "+
			"`--help` (%q missing), so this guard is checking a claim nobody makes", pointer)
	}

	long := newAgentSetupCmd().Long
	if len(long) < 500 {
		t.Fatalf("agent-setup Long is only %d bytes — this test is not reading the help "+
			"text and every assertion below is vacuous", len(long))
	}

	// Each row: what the README's pointer promises, and a string `Long` must
	// carry for that promise to hold.
	for _, tc := range []struct {
		promise string
		inLong  string
	}{
		{"per-vendor header spellings", "${env:CIVITAI_TOKEN}"},
		{"per-vendor header spellings", "bearer_token_env_var"},
		{"the merge rules", "MERGED into, preserving every other server"},
		{"the JSONC re-encoding caveat", "JSONC"},
	} {
		if !strings.Contains(long, tc.inLong) {
			t.Errorf("the README tells readers `--help` carries %s, but the agent-setup "+
				"Long string does not contain %q. The pointer sends them to content that "+
				"is not there.", tc.promise, tc.inLong)
		}
	}

	// 🔴 THE REVERSE DIRECTION, and it is the one that actually failed. The
	// README must NOT promise a topic `Long` does not cover. Symlinks are the
	// measured case: the pointer claimed "JSONC/symlink handling" while `Long`
	// never mentions a symlink.
	if strings.Contains(long, "symlink") || strings.Contains(long, "Symlink") {
		return // Long gained it; the promise below is now safe to make.
	}
	idx := strings.Index(sec, pointer)
	sentence := sec[idx:]
	if end := strings.Index(sentence, "\n\n"); end >= 0 {
		sentence = sentence[:end]
	}
	if strings.Contains(strings.ToLower(sentence), "symlink") {
		t.Errorf("the README's `--help` pointer promises symlink handling, but the "+
			"agent-setup Long string never mentions a symlink. Say it in `Long`, or drop "+
			"it from the pointer — the symlink rule is stated in the README itself.\n"+
			"pointer sentence: %q", strings.TrimSpace(sentence))
	}
}
