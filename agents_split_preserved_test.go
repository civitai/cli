package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The evidence split moved nine item BODIES out of AGENTS.md and left stubs
// behind. The whole value of the split rests on one claim: the move was
// VERBATIM. Every measured RSS table, every mutation matrix, every retraction
// and every enumerated residual is still there, byte for byte, in the file the
// pointer names.
//
// 🔴 THAT CLAIM MUST BE PROVED MECHANICALLY, NOT ASSERTED, because the failure it
// guards against is invisible by construction. A summarised paragraph reads
// fine. A dropped 🔴 residual reads fine. A reflowed table reads fine. Nothing
// about the resulting file looks wrong — the loss is only detectable against the
// text it came from, and the person best placed to notice is the person who did
// the moving and has already stopped looking.
//
// # THE CONTRACT: a digest per item, taken from the BASE commit
//
// splitItems pins, for each moved item, the sha256 of its NON-BLANK body lines
// as they stood at agentsSplitBase — joined with "\n" and newline-terminated.
// The digests were computed FROM `git show <base>:AGENTS.md`, not from the
// evidence files, so they are not a restatement of this change's own output.
// Anyone can re-derive them; TestSplitDigestsAreTheBaseCommitsText does exactly
// that whenever the base blob is reachable.
//
// Blank lines are excluded on purpose. They carry no content, and including them
// would make the guard fail on a trailing-whitespace tidy — a false failure at
// correct content, which is how a docs guard gets deleted.
//
// # WHY A DIGEST AND NOT A LIVE DIFF AGAINST `git show`
//
// `actions/checkout@v4` clones at depth 1, so in CI the base commit's blob is
// NOT in the object store and `git show <base>:AGENTS.md` fails. A guard that
// skipped there would be green in the only environment that gates a merge —
// which is worse than no guard, because it reads as coverage. So the digest is
// the always-on contract, and the git-backed cross-checks below run as
// reinforcement wherever the object is reachable (any full clone, i.e. every
// developer machine).
//
// # THE ONE DELIBERATE CONTENT CHANGE
//
// Item 3 carried a false REASON — that declining to parse `pnpm-lock.yaml`
// avoided "a new dependency, 'ask first'". `gopkg.in/yaml.v3 v3.0.1` is already a
// direct requirement in go.mod and `internal/config/config.go` imports it in
// production code, so the parser was never the cost. The rule and the behaviour
// are unchanged; the reason is corrected and the correction is written up in the
// repo's own retraction style.
//
// That delta is SPELLED OUT here — the exact line replaced, and the exact
// bracketing lines of the retraction paragraph — and reversed before digesting.
// It is not an allowance to edit item 3 freely: the reversal restores one known
// line and removes one known block, and any OTHER edit still breaks the digest.
// Both halves of the delta are asserted live, so a future revert of the
// correction fails with a message that says what it reverted rather than a bare
// hash mismatch.

// agentsSplitBase is the commit the nine bodies were moved from.
const agentsSplitBase = "c5c3817de72ac457cc41839c31b07f8bf1197dce"

type splitItem struct {
	num      int
	file     string
	nonBlank int    // pinned so a failure says "we found N lines, base had M"
	sha      string // sha256 of the base body's non-blank lines
}

// splitItems is the table. Adding a row means moving an item; the sha comes from
// `git show <base>:AGENTS.md`, never from the file you just wrote.
var splitItems = []splitItem{
	{num: 3, file: "claudedocs/decisions/03-lockfile-check.md", nonBlank: 116, sha: "88c413758efb22d2db23f849257c9f6ffd8175bb764c71078fd6c7eeadd9ec4d"},
	{num: 11, file: "claudedocs/decisions/11-vendored-ready-ack.md", nonBlank: 153, sha: "232d2649f45928825e60396d36c4c513a469112d48092ec45686cc2b9ba396bf"},
	{num: 18, file: "claudedocs/decisions/18-ready-ack-existing-apps.md", nonBlank: 130, sha: "83c49221352702ce161854c729e2663cc67d6ca120090d807f8c504db52baaf5"},
	{num: 20, file: "claudedocs/decisions/20-ready-ack-advisory-tiers.md", nonBlank: 389, sha: "82de3c61f2fa3b43516ec8112d5ce64e3026ace3a2a60aa2ca723d44cf6bb191"},
	{num: 21, file: "claudedocs/decisions/21-model-substitution.md", nonBlank: 114, sha: "60ee9ffc115dac34eff068cd270c2d6427c26f84c77bca28388aa60c51c5be67"},
	{num: 23, file: "claudedocs/decisions/23-finding-field-message.md", nonBlank: 165, sha: "89ad99bb3e9fec0e6a316045d7160362b6fad8f8e340fc20c022dcc91becc13a"},
	{num: 24, file: "claudedocs/decisions/24-errno-is-a-net-error.md", nonBlank: 327, sha: "baa658929002c871ee2cbf9332d7dcb9d78749fcab21e832b11a659803c42fd8"},
	{num: 26, file: "claudedocs/decisions/26-project-path-classification.md", nonBlank: 246, sha: "1747de6cf56e85a935367ec118b7e16aecee5fb100d1ebf545ca40576ebbcc11"},
	{num: 27, file: "claudedocs/decisions/27-blockid-derivation-refuses.md", nonBlank: 105, sha: "5edc575b345db5b459af8d0fc0bd6c826e8102a7588b61aa873092848394796a"},
}

// --- the one deliberate delta, item 3 ---------------------------------------

const (
	// item3OldLine is what stood at agentsSplitBase. It states a reason that is
	// false: yaml.v3 is already a direct dependency of this module.
	item3OldLine = `   needs a YAML parser (a new dependency, "ask first" below) and a yarn v1`
	// item3NewLine is what replaced it.
	item3NewLine = `   needs a YAML parser, and a yarn v1`
	// item3RetractionFirst / item3RetractionLast bracket the paragraph added in
	// the repo's retraction style. Bracketing by exact lines rather than by a
	// keyword keeps the reversal mechanical: it cannot swallow neighbouring text
	// even if someone writes "RETRACTED" elsewhere in the item.
	item3RetractionFirst = `   🔴 **"A NEW DEPENDENCY, 'ASK FIRST'" IS RETRACTED — THE REASON WAS FALSE,`
	item3RetractionLast  = `   change is documentation-only, and it is the copy to fix next.`
)

var evidenceBodyStartRe = regexp.MustCompile(`^([0-9]+)\. \*\*`)

// evidenceBody returns the verbatim item block out of an evidence file: from its
// `NN. **` heading to the end of the file, trailing blanks trimmed.
func evidenceBody(t *testing.T, path string, wantNum int) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v — the pointer in AGENTS.md names a file that could not be read", path, err)
	}
	lines := strings.Split(string(b), "\n")
	start := -1
	for i, l := range lines {
		if m := evidenceBodyStartRe.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			if n != wantNum {
				t.Fatalf("%s: body opens with item %d, want item %d — the file and its pointer disagree about which item this is", path, n, wantNum)
			}
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s: no `%d. **…` heading found, so there is no body to compare. "+
			"An evidence file must carry the item VERBATIM, heading included — a reworded or re-headed body is exactly the loss this guard exists to catch.",
			path, wantNum)
	}
	body := lines[start:]
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return body
}

// reverseKnownDelta maps an evidence body back to the text it was moved from.
// It is a no-op for every item but 3.
func reverseKnownDelta(t *testing.T, num int, body []string) []string {
	t.Helper()
	if num != 3 {
		return body
	}

	// Assert the delta is actually PRESENT before reversing it. Without this,
	// a revert of the correction would sail through: the reversal would find
	// nothing to undo, the digest would match the base, and the guard would
	// certify a body that had quietly lost the retraction.
	joined := strings.Join(body, "\n")
	if strings.Contains(joined, item3OldLine) {
		t.Errorf("item 3's evidence file still carries the RETRACTED line:\n  %s\n"+
			"That reason is false — `gopkg.in/yaml.v3 v3.0.1` is a direct requirement in go.mod and `internal/config/config.go` imports it "+
			"in production code. The rule and the behaviour are unchanged; only the reason was corrected.", item3OldLine)
	}
	if !strings.Contains(joined, item3NewLine) {
		t.Errorf("item 3's evidence file no longer carries the corrected line:\n  %s", item3NewLine)
	}

	var out []string
	inRetraction := false
	sawRetraction, closedRetraction, swapped := false, false, false
	for _, l := range body {
		switch {
		case l == item3RetractionFirst:
			inRetraction, sawRetraction = true, true
			continue
		case inRetraction && l == item3RetractionLast:
			inRetraction, closedRetraction = false, true
			continue
		case inRetraction:
			continue
		case l == item3NewLine:
			out = append(out, item3OldLine)
			swapped = true
			continue
		}
		out = append(out, l)
	}
	if !sawRetraction || !closedRetraction {
		t.Fatalf("item 3's retraction paragraph is missing or unterminated (first line found: %v, last line found: %v).\n"+
			"The correction is only half a change without it: the repo's house style is to state what was retracted and what was measured, "+
			"and the surviving copy of the same false claim in internal/validate/lockfile.go is named there.", sawRetraction, closedRetraction)
	}
	if !swapped {
		t.Fatalf("item 3's corrected line was not found, so the reversal to base text could not be performed")
	}
	return out
}

func nonBlank(lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func digestLines(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n") + "\n"))
	return hex.EncodeToString(sum[:])
}

// TestSplitItemBodiesArePreservedVerbatim is the always-on contract: each
// evidence file's body, with item 3's recorded delta reversed, digests to
// exactly the text that stood in AGENTS.md at agentsSplitBase.
func TestSplitItemBodiesArePreservedVerbatim(t *testing.T) {
	if len(splitItems) < minEvidencePointers {
		t.Fatalf("CONTROL failure, not a finding: the split table holds %d row(s), want >= %d — a table this small proves almost nothing",
			len(splitItems), minEvidencePointers)
	}
	for _, it := range splitItems {
		t.Run(fmt.Sprintf("item_%d", it.num), func(t *testing.T) {
			body := reverseKnownDelta(t, it.num, evidenceBody(t, it.file, it.num))
			nb := nonBlank(body)
			if len(nb) != it.nonBlank {
				t.Errorf("%s: %d non-blank body line(s), base had %d (delta %+d)",
					it.file, len(nb), it.nonBlank, len(nb)-it.nonBlank)
			}
			if got := digestLines(nb); got != it.sha {
				t.Fatalf("%s: item %d's body no longer matches the text it was moved from.\n"+
					"  got  sha256 %s over %d non-blank line(s)\n"+
					"  want sha256 %s over %d non-blank line(s) (AGENTS.md at %s)\n\n"+
					"🔴 THE MOVE WAS VERBATIM AND MUST STAY VERBATIM. Every measurement, mutation matrix, retraction and residual in this body "+
					"was paid for by a round of work; a summary of it is not the same artefact. If you genuinely need to CHANGE this item, "+
					"change it — and re-pin the sha here in the same commit, so the edit is reviewable as an edit rather than lost as drift.\n"+
					"Re-derive the base text with:  git show %s:AGENTS.md",
					it.file, it.num, got, len(nb), it.sha, it.nonBlank, agentsSplitBase[:7], agentsSplitBase)
			}
		})
	}
}

// TestSplitTableCoversEveryEvidenceFile keeps this table and the ledger in
// agents_evidence_test.go from drifting apart. A body moved without a row here
// is a body with no preservation proof at all — the exact silence the whole file
// exists to end — and a row left behind after a file is deleted is a pin nothing
// enforces.
func TestSplitTableCoversEveryEvidenceFile(t *testing.T) {
	inTable := map[string]bool{}
	for _, it := range splitItems {
		if inTable[it.file] {
			t.Errorf("%s appears twice in splitItems", it.file)
		}
		inTable[it.file] = true
	}
	files := evidenceFilesOnDisk(t)
	for _, f := range files {
		if !inTable[f] {
			t.Errorf("%s has no row in splitItems, so nothing proves its body is the text it was moved from.\n"+
				"Add a row with the sha256 of its non-blank body lines at the commit you moved it from (see this file's header).", f)
		}
	}
	onDisk := map[string]bool{}
	for _, f := range files {
		onDisk[f] = true
	}
	for _, it := range splitItems {
		if !onDisk[it.file] {
			t.Errorf("splitItems pins %s, which is not in %s — a stale row looks like coverage and is not", it.file, evidenceDir)
		}
	}
}

// --- git-backed reinforcement ------------------------------------------------

// baseAgentsMD returns AGENTS.md at agentsSplitBase, or "" when the object is
// not reachable (a shallow CI clone, or no git at all).
func baseAgentsMD(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "show", agentsSplitBase+":AGENTS.md").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// baseItemBody slices one item's body out of a whole AGENTS.md.
func baseItemBody(t *testing.T, doc string, num int) []string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	start, end := -1, len(lines)
	for i, l := range lines {
		m := itemHeadingRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n == num {
			start = i
			continue
		}
		if start >= 0 && i > start {
			end = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("item %d not found in the base AGENTS.md", num)
	}
	// The last item runs to the closing paragraph rather than to another
	// heading; both spellings are handled by trimming trailing blanks after the
	// first line that leaves the list's indentation.
	body := lines[start:end]
	for i, l := range body {
		if i > 0 && l != "" && !strings.HasPrefix(l, " ") {
			body = body[:i]
			break
		}
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return body
}

// TestSplitDigestsAreTheBaseCommitsText re-derives every pinned digest from the
// base commit, so the constants above are provably the base text and not a
// restatement of this change's own output.
//
// 🔴 IT SKIPS IN A SHALLOW CLONE, AND THAT IS WHY IT IS NOT THE CONTRACT.
// `actions/checkout@v4` clones at depth 1, so the base blob does not exist in
// CI's object store. TestSplitItemBodiesArePreservedVerbatim is the guard that
// gates a merge; this one is the reason to believe its constants, and it runs on
// every developer machine.
func TestSplitDigestsAreTheBaseCommitsText(t *testing.T) {
	doc := baseAgentsMD(t)
	if doc == "" {
		t.Skipf("base blob %s:AGENTS.md is not reachable (shallow clone, or git unavailable) — "+
			"the pinned digests could not be re-derived here. TestSplitItemBodiesArePreservedVerbatim still holds them.",
			agentsSplitBase[:7])
	}
	// POSITIVE CONTROL for the slicer: the base file is the big one, and if the
	// checkout resolved to something else every comparison below is meaningless.
	if len(doc) < 150_000 {
		t.Fatalf("CONTROL failure: base AGENTS.md is %d bytes, expected the pre-split file (~186 kB). "+
			"agentsSplitBase does not name the commit these digests came from.", len(doc))
	}
	for _, it := range splitItems {
		t.Run(fmt.Sprintf("item_%d", it.num), func(t *testing.T) {
			nb := nonBlank(baseItemBody(t, doc, it.num))
			if len(nb) != it.nonBlank {
				t.Errorf("base item %d has %d non-blank line(s), table pins %d", it.num, len(nb), it.nonBlank)
			}
			if got := digestLines(nb); got != it.sha {
				t.Fatalf("the pinned sha for item %d does not match the base commit's own text:\n  base %s\n  pin  %s\n"+
					"The pin is wrong, not the evidence file. Re-derive it from `git show %s:AGENTS.md`.",
					it.num, got, it.sha, agentsSplitBase)
			}
		})
	}
}

// TestEveryBaseBodyLineSurvivedTheMove is the line-level differ. The digest above
// says PASS or FAIL; this says WHICH LINE went missing, which is the difference
// between a guard someone can act on and one they delete.
//
// Skips in a shallow clone for the same reason as its sibling.
func TestEveryBaseBodyLineSurvivedTheMove(t *testing.T) {
	doc := baseAgentsMD(t)
	if doc == "" {
		t.Skipf("base blob %s:AGENTS.md is not reachable — see TestSplitDigestsAreTheBaseCommitsText", agentsSplitBase[:7])
	}
	total := 0
	for _, it := range splitItems {
		t.Run(fmt.Sprintf("item_%d", it.num), func(t *testing.T) {
			have := map[string]int{}
			for _, l := range nonBlank(evidenceBody(t, it.file, it.num)) {
				have[l]++
			}
			var missing []string
			for _, l := range nonBlank(baseItemBody(t, doc, it.num)) {
				if it.num == 3 && l == item3OldLine {
					continue // the one recorded, reviewed delta
				}
				if have[l] == 0 {
					missing = append(missing, "  "+l)
				}
				total++
			}
			if len(missing) > 0 {
				show := missing
				if len(show) > 12 {
					show = append(show[:12:12], fmt.Sprintf("  … and %d more", len(missing)-12))
				}
				t.Fatalf("%d line(s) of item %d did not survive the move into %s:\n%s\n\n"+
					"The move is VERBATIM by contract. Nothing is summarised and nothing is dropped.",
					len(missing), it.num, it.file, strings.Join(show, "\n"))
			}
		})
	}
	// POSITIVE CONTROL. A differ that compared zero lines would report a serene
	// pass; requiring a floor makes "no missing lines" a claim about real work.
	if total == 0 {
		t.Fatal("CONTROL failure: the differ compared 0 lines, so its clean result says nothing")
	}
	t.Logf("checked %d non-blank base lines across %d moved items", total, len(splitItems))
}
