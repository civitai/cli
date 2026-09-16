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

	// Every check name the command can emit.
	//
	// 🔴 THE `mcp-*` NAMES ARE NOT CONSTANTS — they are bare literals on the
	// server table (agent_setup_mcp.go), so a list of the `check*` constants
	// alone holds FOUR of the six and the docstring above it reads as coverage
	// it does not have. Round 2 of #641 measured exactly that: a mutant
	// exempting `mcp-orch` from the verdict SURVIVED this guard while the
	// equivalent mutant on `agents-md` was killed. Derive the server rows from
	// the table so a third server is covered the moment it is added.
	all := []string{checkCLIVersion, checkAgentsMD, checkClaudeMD, checkAuthenticated}
	for _, s := range civitaiMCPServers {
		all = append(all, s.Check)
	}
	// Positive control on the derivation: the table must actually have
	// contributed, or this guard silently narrows back to the four constants.
	if len(all) <= 4 {
		t.Fatalf("derived only %d check name(s) — civitaiMCPServers contributed none, so "+
			"the `mcp-*` rows are unguarded again", len(all))
	}

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

	// 🔴 READ THE EXEMPTION SENTENCE, NOT THE SECTION. Round 2 of #641 killed
	// the previous version of this guard: it asked whether the section MENTIONS
	// each exempt name anywhere, and `mcp-orch` is mentioned several paragraphs
	// away (the JSON sample, the unreadable-config paragraph). So a mutant
	// exempting `mcp-orch` from the verdict SURVIVED — the name was present,
	// while the sentence that tells a `--json` consumer what to skip said
	// nothing about it. Presence of a word is not a statement about it.
	//
	// So: find the sentence that STATES the exemption, and compare the check
	// names inside THAT sentence against the set derived from the code.
	const claim = "`ok` is the AND of every check except"
	ci := strings.Index(sec, claim)
	if ci < 0 {
		t.Fatalf("PREMISE BROKEN: the agent-setup section no longer contains %q, so this "+
			"guard cannot find the claim it exists to check", claim)
	}
	sentence := sec[ci:]
	if end := strings.Index(sentence, ".**"); end >= 0 {
		sentence = sentence[:end]
	}

	named := map[string]bool{}
	for _, name := range all {
		if strings.Contains(sentence, "`"+name+"`") {
			named[name] = true
		}
	}
	for _, name := range exemptForOther {
		if !named[name] {
			t.Errorf("checkCountsTowardVerdict exempts %q from `ok`, and the README's "+
				"exemption sentence does not name it. A `--json` consumer computing its own "+
				"verdict from that sentence folds %q in and disagrees with the tool it wraps."+
				"\n  sentence: %q", name, name, strings.TrimSpace(sentence))
		}
	}
	for name := range named {
		if !contains(exemptForOther, name) {
			t.Errorf("the README's exemption sentence names %q as excluded from `ok`, but "+
				"checkCountsTowardVerdict COUNTS it. A consumer skipping that row misses a "+
				"real failure.\n  sentence: %q", name, strings.TrimSpace(sentence))
		}
	}
	// The agent-conditional exemption must carry its condition, or the sentence
	// claims claude users get it too.
	for _, name := range exemptForOther {
		if contains(exemptForClaude, name) {
			continue
		}
		if !strings.Contains(sentence, "other than `claude`") {
			t.Errorf("%q is exempt ONLY for agents other than claude, and the README states "+
				"the exemption without that condition — which claims claude users get it too",
				name)
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
	// 🔴 ASSERT THE SET, NOT A SPELLING. An earlier version scanned each row's
	// name for "header"/"authorization"; round 2 of #641 killed it with a row
	// named `mcp-auth` whose DETAIL was about the header — the hazard in a
	// different shape, which is what RULES.md means by a SPELLED guard. The
	// emitted set is enumerable, so enumerate it: any new row at all is red
	// here, and whoever adds one must decide what the README should say.
	want := map[string]bool{
		checkCLIVersion: true, checkAgentsMD: true, checkClaudeMD: true,
		checkAuthenticated: true,
	}
	for _, s := range civitaiMCPServers {
		want[s.Check] = true
	}
	for _, c := range payload.Checks {
		if !want[c.Name] {
			t.Errorf("`--check` emits an unledgered row %q. The agent-setup section tells "+
				"readers an absent Authorization header gets NO row of its own and that "+
				"`--check` will not tell them — if this new row reports on one, that "+
				"paragraph is now wrong. Either way, ledger it here.", c.Name)
		}
	}
	for name := range want {
		var seen bool
		for _, c := range payload.Checks {
			if c.Name == name {
				seen = true
				break
			}
		}
		if !seen {
			t.Errorf("`--check` no longer emits the %q row this file ledgers; the README "+
				"section still describes the row set as it was", name)
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
			// The README's rule for every OTHER row: absolute — which means
			// NON-EMPTY and absolute. 🔴 An earlier version of this guard read
			// `c.Path != "" && !filepath.IsAbs(...)`, which exempted the empty
			// string — the exact shape it exists to catch. Round 2 of #641
			// measured a mutant emptying every path: it produced
			// `"path": "", "action": "create"` in real output and this guard
			// PASSED.
			if !filepath.IsAbs(c.Path) {
				t.Errorf("row %q carries path %q, which is not absolute; the README says a "+
					"path in `--json` is always absolute whatever --dir you passed, and names "+
					"the `manual` row as the ONLY exception", c.Action, c.Path)
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
// 🔴 HOW THE TWO DIRECTIONS ARE ENFORCED, because they are enforced
// differently and an earlier docstring claimed both were mechanical:
//
//	forward  (`Long` carries what the pointer promises) — a HAND-MAINTAINED
//	         table. Adding a promise here is what proves `Long` carries it.
//	reverse  (the pointer promises nothing `Long` lacks) — mechanical, by
//	         pinning the WHOLE sentence. Any widening is red, so the forward
//	         table cannot be bypassed by editing prose alone.
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
		{"the merge rules", "merged into"},
		{"the JSONC re-encoding caveat", "JSONC"},
	} {
		// 🔴 CASE-INSENSITIVE, and the message says the fixture may be at fault.
		// These two rows match PROSE, not an identifier, so a legitimate reword
		// of `Long` turns them red — and the first draft asserted "MERGED INTO",
		// which `Long` does not spell that way. A failure here is "check, then
		// fix ONE of the two", never "the help text lost this topic".
		if !strings.Contains(strings.ToLower(long), strings.ToLower(tc.inLong)) {
			t.Errorf("the README tells readers `--help` carries %s, and the agent-setup Long "+
				"string no longer contains %q.\n\nTWO POSSIBILITIES, check before fixing: "+
				"(a) `Long` really dropped the topic — then drop it from the README pointer "+
				"too; or (b) `Long` merely reworded it — then this fixture is what is stale, "+
				"and the topic is still documented.", tc.promise, tc.inLong)
		}
	}

	// 🔴 THE REVERSE DIRECTION — the README must not promise a topic `Long` does
	// not cover — AND IT IS PINNED AS A WHOLE NORMALISED SENTENCE, not by
	// scanning for a word.
	//
	// Round 2 of #641 measured why: when this checked only for "symlink", a
	// mutant widening the pointer to "…and the Windsurf Devin CLI path in full"
	// SURVIVED the entire package suite — the same shape as the defect this
	// guard was written for, re-shippable with everything green. A word scan
	// cannot enumerate the topics nobody has thought of yet.
	//
	// RULES.md: "When the artifact under test IS prose, a guard on WORDS is
	// walkable by REWORDING — pin the WHOLE normalised string. A cosmetic reword
	// then fails the test — pay it, for a machine-readable claim." So widening
	// this sentence is RED BY CONSTRUCTION: whoever widens it must add the new
	// promise to the forward table above, which is what proves `Long` carries it.
	const wantPointer = "`civitai agent-setup --help` carries the per-vendor header spellings, the merge " +
		"rules and the JSONC re-encoding caveat in full."
	idx := strings.Index(sec, pointer)
	sentence := sec[idx:]
	if end := strings.Index(sentence, "\n\n"); end >= 0 {
		sentence = sentence[:end]
	}
	if got := strings.Join(strings.Fields(sentence), " "); got != wantPointer {
		t.Errorf("the `--help` pointer sentence changed.\n  got:  %q\n  want: %q\n\n"+
			"This guard pins the WHOLE sentence because a word scan let a widened pointer "+
			"ship green (round 2 of #641). If you added a promise, add a row to the table "+
			"above proving `Long` carries it, then update wantPointer. If you only reworded, "+
			"update wantPointer — that is the price of a machine-readable claim.", got, wantPointer)
	}
}
