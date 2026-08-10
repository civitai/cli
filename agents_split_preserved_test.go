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

// The evidence split has moved sixteen item BODIES out of AGENTS.md, over three
// waves, and left stubs behind. The whole value of the split rests on one
// claim: the move was VERBATIM. Every measured RSS table, every mutation
// matrix, every retraction and every enumerated residual is still there, byte
// for byte, in the file the pointer names.
//
// 🔴 THAT CLAIM MUST BE PROVED MECHANICALLY, NOT ASSERTED, because the failure it
// guards against is invisible by construction. A summarised paragraph reads
// fine. A dropped 🔴 residual reads fine. A reflowed table reads fine. Nothing
// about the resulting file looks wrong — the loss is only detectable against the
// text it came from, and the person best placed to notice is the person who did
// the moving and has already stopped looking.
//
// # THE CONTRACT: a digest per item, taken from THAT ITEM'S base commit
//
// splitItems pins, for each moved item, the sha256 of its NON-BLANK body lines
// as they stood at the commit it was moved FROM — joined with "\n" and
// newline-terminated. The digests were computed FROM `git show <base>:AGENTS.md`,
// not from the evidence files, so they are not a restatement of this change's
// own output. Anyone can re-derive them; TestSplitDigestsAreTheBaseCommitsText
// does exactly that whenever the base blob is reachable.
//
// # 🔴 THE BASE IS PER-ITEM, AND A SINGLE GLOBAL CONSTANT IS NOT AVAILABLE
//
// This started as one constant, `agentsSplitBase`, because there had been
// exactly one eviction wave (#290, nine items). The second wave (items 10 and
// 19) proved that was an accident of history rather than a design, in two
// independent ways:
//
//  1. THE BODIES BEING MOVED HAD CHANGED. #297's compression pass rewrote both
//     items in AGENTS.md — item 10 by 22 bytes, item 19 by 77 — so neither is
//     byte-identical to c5c3817 any more. (agents_size_test.go's own comment
//     asserted the opposite, "both are byte-identical to agentsSplitBase"; it
//     was written before that pass landed and was already false. Re-measure a
//     claim like that, do not inherit it.) Digesting them against c5c3817 would
//     have pinned text that is not what was moved.
//
//  2. THE OLD BASE CANNOT BE RE-POINTED FORWARD. The obvious alternative —
//     re-derive every digest from one NEWER commit — is impossible, not
//     merely inconvenient: after wave 1, AGENTS.md no longer CONTAINS the nine
//     moved bodies at any commit. `baseItemBody(c5c3817+1, 3)` returns item 3's
//     STUB. A body's base commit is therefore fixed forever at the moment it was
//     evicted, and different evictions have different moments. Per-item is the
//     only shape that can express that.
//
// So `base` is a field on the row, not a package constant, and the three named
// constants below exist only so a reader can see which wave a row belongs to.
// Wave 3 (items 9, 13, 17, 22 and 25) added the third and touched neither of the
// first two, which is the property the per-item field was introduced for. A
// fourth wave adds a fourth.
//
// 🔴 WHAT DOES NOT CHANGE: the guard still fails if ANY split body is altered,
// for all sixteen items. Splitting the base per row weakens nothing — each row is
// still compared against a specific immutable blob — and that is the property to
// re-verify by mutation whenever this table's shape is edited, because "I made
// the pinning more flexible" is exactly how a pin stops pinning.
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

// agentsSplitBase is the commit WAVE 1's nine bodies were moved from (#290).
const agentsSplitBase = "c5c3817de72ac457cc41839c31b07f8bf1197dce"

// agentsSplitBaseWave2 is the commit items 10 and 19 were moved from: the tip
// after #297's compression pass, which edited BOTH of them. It is deliberately
// not agentsSplitBase — see the 🔴 section of this file's doc comment.
const agentsSplitBaseWave2 = "5cb45f0ff54fe55f5249aade8b0cb9826ade3c58"

// agentsSplitBaseWave3 is the commit items 9, 13, 17, 22 and 25 were moved
// from. It is the third distinct base, which is the shape the per-item field
// exists for: each of the three is the tip at the moment ITS wave ran, and none
// of them can be re-pointed at either of the others.
const agentsSplitBaseWave3 = "6e31b4b29a0eb15c83e951a114db2345877d38bd"

type splitItem struct {
	num      int
	file     string
	base     string // the commit whose AGENTS.md still held this item's BODY
	nonBlank int    // pinned so a failure says "we found N lines, base had M"
	sha      string // sha256 of the base body's non-blank lines
}

// splitItems is the table. Adding a row means moving an item; the sha comes from
// `git show <base>:AGENTS.md`, never from the file you just wrote — and `base`
// is the commit YOU moved it from, which for a new wave is not the constant the
// rows above it use.
var splitItems = []splitItem{
	{num: 3, file: "claudedocs/decisions/03-lockfile-check.md", base: agentsSplitBase, nonBlank: 116, sha: "88c413758efb22d2db23f849257c9f6ffd8175bb764c71078fd6c7eeadd9ec4d"},
	{num: 9, file: "claudedocs/decisions/09-views-unavailable-discriminator.md", base: agentsSplitBaseWave3, nonBlank: 50, sha: "6bd481b9d65e560c4e02ce8aec2d7d14ce4b37f3e84c933562b9800374330624"},
	{num: 10, file: "claudedocs/decisions/10-dev-tunnel-embeddability.md", base: agentsSplitBaseWave2, nonBlank: 103, sha: "b2397598f7913fac286f2fb5ee1e1e1ebf85348e365a6e820799d4ff817cab3c"},
	{num: 13, file: "claudedocs/decisions/13-generation-graph-not-validated.md", base: agentsSplitBaseWave3, nonBlank: 42, sha: "0b875b35411e58cd61c66b16dd64fc20d4fb5d1f787d838912ed1d3fa2074df2"},
	{num: 17, file: "claudedocs/decisions/17-download-presigned-no-credential.md", base: agentsSplitBaseWave3, nonBlank: 34, sha: "18d1888d11bf5359fa25dcc1fe0d86418e9b9b349bb297d54876afd5d5d5ef21"},
	{num: 11, file: "claudedocs/decisions/11-vendored-ready-ack.md", base: agentsSplitBase, nonBlank: 153, sha: "232d2649f45928825e60396d36c4c513a469112d48092ec45686cc2b9ba396bf"},
	{num: 18, file: "claudedocs/decisions/18-ready-ack-existing-apps.md", base: agentsSplitBase, nonBlank: 130, sha: "83c49221352702ce161854c729e2663cc67d6ca120090d807f8c504db52baaf5"},
	{num: 19, file: "claudedocs/decisions/19-img2img-workflow-and-upload.md", base: agentsSplitBaseWave2, nonBlank: 103, sha: "0e1bd1bd9b862123d99e106167e6fc0bc5fca55445f3d34709e8115092fef1ef"},
	{num: 20, file: "claudedocs/decisions/20-ready-ack-advisory-tiers.md", base: agentsSplitBase, nonBlank: 389, sha: "82de3c61f2fa3b43516ec8112d5ce64e3026ace3a2a60aa2ca723d44cf6bb191"},
	{num: 21, file: "claudedocs/decisions/21-model-substitution.md", base: agentsSplitBase, nonBlank: 114, sha: "60ee9ffc115dac34eff068cd270c2d6427c26f84c77bca28388aa60c51c5be67"},
	{num: 22, file: "claudedocs/decisions/22-input-refuses-non-txt2img.md", base: agentsSplitBaseWave3, nonBlank: 44, sha: "25ddad01439c67ae7d3ef3362c095a0885db409f77b6d3ea62c1008a7c95ce1a"},
	{num: 23, file: "claudedocs/decisions/23-finding-field-message.md", base: agentsSplitBase, nonBlank: 165, sha: "89ad99bb3e9fec0e6a316045d7160362b6fad8f8e340fc20c022dcc91becc13a"},
	{num: 24, file: "claudedocs/decisions/24-errno-is-a-net-error.md", base: agentsSplitBase, nonBlank: 327, sha: "baa658929002c871ee2cbf9332d7dcb9d78749fcab21e832b11a659803c42fd8"},
	{num: 25, file: "claudedocs/decisions/25-listing-media-bounds.md", base: agentsSplitBaseWave3, nonBlank: 36, sha: "91069724894cd2b602034aa89cb61349096567f4e18a2ec40353146ac3dac249"},
	{num: 26, file: "claudedocs/decisions/26-project-path-classification.md", base: agentsSplitBase, nonBlank: 246, sha: "1747de6cf56e85a935367ec118b7e16aecee5fb100d1ebf545ca40576ebbcc11"},
	{num: 27, file: "claudedocs/decisions/27-blockid-derivation-refuses.md", base: agentsSplitBase, nonBlank: 105, sha: "5edc575b345db5b459af8d0fc0bd6c826e8102a7588b61aa873092848394796a"},
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
					it.file, it.num, got, len(nb), it.sha, it.nonBlank, it.base[:7], it.base)
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

// gitEnv distinguishes the two reasons a base blob can fail to resolve, because
// they demand OPPOSITE answers and conflating them is what broke CI once.
//
// 🔴 A SHALLOW CLONE IS AN EXPECTED ABSENCE; AN UNRESOLVABLE SHA IN A FULL CLONE
// IS A BUG. `actions/checkout@v4` clones at depth 1, so NO base blob exists in
// CI and skipping is the honest answer — the digests are the contract and they
// need no git at all. But on a full clone the same empty string means the sha in
// the table is wrong, and answering THAT with a skip would retire the differ
// silently, which is the failure mode this whole file exists to prevent. So the
// environment is interrogated rather than inferred from the absence.
type gitEnv int

const (
	gitUsable      gitEnv = iota // full clone: base blobs must resolve
	gitShallow                   // depth-limited: base blobs are absent by construction
	gitUnavailable               // no git, or not a repository
)

var gitEnvOnce struct {
	done bool
	env  gitEnv
}

func detectGitEnv() gitEnv {
	if gitEnvOnce.done {
		return gitEnvOnce.env
	}
	gitEnvOnce.done = true
	out, err := exec.Command("git", "rev-parse", "--is-shallow-repository").Output()
	switch {
	case err != nil:
		gitEnvOnce.env = gitUnavailable
	case strings.TrimSpace(string(out)) == "true":
		gitEnvOnce.env = gitShallow
	default:
		gitEnvOnce.env = gitUsable
	}
	return gitEnvOnce.env
}

func (g gitEnv) String() string {
	switch g {
	case gitShallow:
		return "a shallow (depth-limited) clone"
	case gitUnavailable:
		return "an environment with no usable git"
	}
	return "a full clone"
}

// baseAgentsMD returns AGENTS.md at the given commit, or "" when the object is
// not reachable (a shallow CI clone, or no git at all). Results are cached: with
// a base per row, an uncached read would re-shell out once per item.
var baseAgentsMDCache = map[string]string{}

func baseAgentsMD(t *testing.T, ref string) string {
	t.Helper()
	if doc, ok := baseAgentsMDCache[ref]; ok {
		return doc
	}
	out, err := exec.Command("git", "show", ref+":AGENTS.md").Output()
	if err != nil {
		baseAgentsMDCache[ref] = ""
		return ""
	}
	baseAgentsMDCache[ref] = string(out)
	return string(out)
}

// baseDocFor resolves one row's base document and applies the controls that make
// a comparison against it meaningful. It returns "" when the blob is unreachable
// (the caller then skips, per the 🔴 note on the git-backed tests).
//
// 🔴 THE OLD CONTROL WAS `len(doc) < 150_000` — "this is the big pre-split file"
// — AND IT CANNOT SURVIVE A SECOND WAVE. Wave 2's base is a POST-split commit of
// 67 kB, so that assertion would fail on a perfectly correct row. It is replaced
// by two controls that are true of any wave: the document must be a plausible
// AGENTS.md at all, and the sliced body must be a BODY rather than a stub — a
// base commit accidentally pointed AFTER its own eviction yields the 8-line stub,
// which would otherwise surface as an inscrutable digest mismatch instead of
// "you named the wrong commit".
func baseDocFor(t *testing.T, it splitItem) string {
	t.Helper()
	doc := baseAgentsMD(t, it.base)
	if doc == "" {
		if env := detectGitEnv(); env == gitUsable {
			t.Fatalf("item %d's base commit %s is NOT REACHABLE, and this is %s — so history IS available and the object still "+
				"did not resolve. That means the sha in splitItems is wrong, not that the environment is limited. "+
				"A base nobody can resolve is a pin nobody can re-derive; fix the sha rather than letting this degrade to a skip.",
				it.num, it.base, env)
		}
		return ""
	}
	if len(doc) < agentsMinBytes {
		t.Fatalf("CONTROL failure: AGENTS.md at %s is %d bytes, below the %d-byte floor. "+
			"That is not an AGENTS.md; item %d's base commit is wrong or the blob is truncated.",
			it.base[:7], len(doc), agentsMinBytes, it.num)
	}
	if evidencePathRe.MatchString(strings.Join(baseItemBody(t, doc, it.num), "\n")) {
		t.Fatalf("CONTROL failure: item %d's body at %s is already a STUB — it carries an `→ evidence:` pointer. "+
			"The base must be a commit whose AGENTS.md still held the BODY, i.e. the commit you moved it FROM. "+
			"A body's base is fixed at the moment it was evicted and can never be re-pointed forward.",
			it.num, it.base[:7])
	}
	return doc
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
// reachableBases counts how many rows' base blobs this environment can resolve.
// It is the ONE question both git-backed tests ask before comparing anything, so
// neither can reach a positive control on input the environment cannot supply.
func reachableBases(t *testing.T) int {
	t.Helper()
	n := 0
	for _, it := range splitItems {
		if baseAgentsMD(t, it.base) != "" {
			n++
		}
	}
	return n
}

func TestSplitDigestsAreTheBaseCommitsText(t *testing.T) {
	if reachableBases(t) == 0 {
		t.Skipf("none of the %d base blob(s) is reachable — this is %s. The pinned digests could not be re-derived here; "+
			"TestSplitItemBodiesArePreservedVerbatim still holds every one of them without git.",
			len(splitItems), detectGitEnv())
	}
	checked := 0
	for _, it := range splitItems {
		t.Run(fmt.Sprintf("item_%d", it.num), func(t *testing.T) {
			doc := baseDocFor(t, it)
			if doc == "" {
				t.Skipf("base blob %s:AGENTS.md is not reachable (shallow clone, or git unavailable) — "+
					"item %d's pinned digest could not be re-derived here. TestSplitItemBodiesArePreservedVerbatim still holds it.",
					it.base[:7], it.num)
			}
			checked++
			nb := nonBlank(baseItemBody(t, doc, it.num))
			if len(nb) != it.nonBlank {
				t.Errorf("base item %d has %d non-blank line(s), table pins %d", it.num, len(nb), it.nonBlank)
			}
			if got := digestLines(nb); got != it.sha {
				t.Fatalf("the pinned sha for item %d does not match its base commit's own text:\n  base %s\n  pin  %s\n"+
					"The pin is wrong, not the evidence file. Re-derive it from `git show %s:AGENTS.md`.",
					it.num, got, it.sha, it.base)
			}
		})
	}
	if checked > 0 {
		t.Logf("re-derived %d of %d pinned digest(s) from their own base commits", checked, len(splitItems))
	}
}

// TestSplitBasesAreWellFormed is the cheap always-on control on the new per-row
// field. It runs in a shallow clone where the git-backed tests skip, so a row
// added with an empty or abbreviated base fails somewhere rather than nowhere.
//
// It also asserts the bases are a SMALL set. Every row of one wave shares a
// commit; a table where each row carried its own would mean somebody re-derived
// digests against whatever HEAD happened to be, which is the drift this file
// exists to stop.
func TestSplitBasesAreWellFormed(t *testing.T) {
	fullSHA := regexp.MustCompile(`^[0-9a-f]{40}$`)
	waves := map[string]int{}
	for _, it := range splitItems {
		if !fullSHA.MatchString(it.base) {
			t.Errorf("item %d's base %q is not a full 40-character sha. An abbreviated ref is ambiguous forever; "+
				"a body's base commit is permanent and must be spelled in full", it.num, it.base)
			continue
		}
		waves[it.base]++
	}
	if len(waves) == 0 {
		t.Fatal("CONTROL failure: no bases parsed at all")
	}
	if len(waves) > len(splitItems)/2 {
		t.Errorf("%d distinct base commits across %d rows. Eviction happens in WAVES — every item moved in one change "+
			"shares its base — so a table this fragmented means digests were taken against a moving HEAD rather than "+
			"against the commit each body was moved from", len(waves), len(splitItems))
	}
}

// TestEveryBaseBodyLineSurvivedTheMove is the line-level differ. The digest above
// says PASS or FAIL; this says WHICH LINE went missing, which is the difference
// between a guard someone can act on and one they delete.
//
// Skips in a shallow clone for the same reason as its sibling.
func TestEveryBaseBodyLineSurvivedTheMove(t *testing.T) {
	// 🔴 THE SKIP IS DECIDED HERE, BEFORE ANY COMPARISON, AND THAT PLACEMENT IS
	// THE WHOLE FIX. When the per-item base landed, this skip moved INTO the
	// subtest — which reads like a tidy-up and is not one: in a shallow clone
	// every subtest then skipped, `total` stayed 0, and the zero-line CONTROL at
	// the bottom fired. CI went red with "the differ compared 0 lines", which is
	// the control doing its job on an environment it was never meant to judge.
	// The local `make ci` could not see it, because a full clone resolves every
	// blob — a true green about an environment CI does not have.
	//
	// So: if NOTHING resolves, this test has nothing to say and says so once, at
	// the top. The digests in TestSplitItemBodiesArePreservedVerbatim are
	// unaffected — they read the evidence files and need no git.
	if reachableBases(t) == 0 {
		t.Skipf("none of the %d base blob(s) is reachable — this is %s, where the line-naming differ CANNOT run. "+
			"Every body is still pinned by sha256 in TestSplitItemBodiesArePreservedVerbatim, which needs no git; "+
			"this test is reinforcement that names WHICH line went missing, and it runs on any full clone.",
			len(splitItems), detectGitEnv())
	}

	total := 0
	for _, it := range splitItems {
		t.Run(fmt.Sprintf("item_%d", it.num), func(t *testing.T) {
			doc := baseDocFor(t, it)
			if doc == "" {
				t.Skipf("base blob %s:AGENTS.md is not reachable — see TestSplitDigestsAreTheBaseCommitsText", it.base[:7])
			}
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
	//
	// It is reached ONLY when at least one base resolved (the guard at the top of
	// this function), so a 0 here means the comparison itself is broken — a
	// finding — rather than "the environment cannot supply the input", which is a
	// skip. Keeping those two apart is the entire lesson of this function.
	if total == 0 {
		t.Fatal("CONTROL failure: at least one base blob resolved, yet the differ compared 0 lines. " +
			"The slicer or the evidence reader is broken; this is NOT the shallow-clone case, which is skipped above.")
	}
	t.Logf("checked %d non-blank base lines across %d moved items", total, len(splitItems))
}
