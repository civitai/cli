package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 🔴 README.md IS SHIPPED; AGENTS.md IS NOT.
//
// The npm package and the Homebrew cask carry the README and not the
// contributor docs, so a **relative** link to `AGENTS.md` resolves for exactly
// one audience — someone reading the file on github.com — and 404s for the
// user who installed the CLI. Seven such links existed, including the two a
// `generate` user is sent to for the per-ecosystem image-editing rationale and
// an `app listing` user for the media bounds.
//
// The rule is therefore: a README link into a contributor-only file must be an
// ABSOLUTE URL. The link text may stay `AGENTS.md`.
//
// 🔴 THE SET, AND WHY IT IS THREE. `CONTRIBUTING.md` was missing from the first
// version of this list, and the README linked it relatively FOUR LINES below one
// of the links this test had just fixed — an instance of the very bug, shipped
// beside the fix. It is in neither shipping artifact either: `npm/package.json`
// ships `["bin/","lib/","README.md","LICENSE"]` and `.goreleaser.yaml`'s archive
// carries `README.md` + `LICENSE`. Before adding a doc here, check it is
// genuinely NOT shipped; before adding a README link to a repo file, check it is.
//
// Scope, stated so a green is not over-read: this checks the LINK TARGET, not
// whether routing a user through a contributor doc was the right call in the
// first place. Where the fact itself belongs in the README, inline it.
var contributorOnlyDocs = []string{"AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md"}

// relativeMDLinkRe matches an inline markdown link target.
var relativeMDLinkRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// refDefMDLinkRe matches a reference-style link DEFINITION (`[label]: target`).
// It is a second shape entirely: the first version of this guard read only
// inline links, so `[agents]: AGENTS.md` plus `[agents]` in the prose would have
// rendered the same 404 and passed.
var refDefMDLinkRe = regexp.MustCompile(`(?m)^\[[^\]]+\]:[ \t]+(\S+)`)

// normaliseLinkTarget reduces a relative markdown target to the repo-root file
// name it resolves to, or "" if it is absolute.
//
// 🔴 THIS IS WHAT MAKES THE GUARD STRUCTURAL RATHER THAN SPELLED. Comparing the
// raw target to "AGENTS.md" accepts `./AGENTS.md` and `../AGENTS.md`, which are
// the same 404 written differently — a guard that can be walked around by
// respelling the hazard is the failure mode this repo has recorded before.
func normaliseLinkTarget(target string) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "#") ||
		strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "/") {
		return ""
	}
	// Drop a trailing #anchor / ?query, then any ./ and ../ prefixes.
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		target = target[:i]
	}
	target = strings.TrimPrefix(target, "<")
	target = strings.TrimSuffix(target, ">")
	for {
		switch {
		case strings.HasPrefix(target, "./"):
			target = target[2:]
		case strings.HasPrefix(target, "../"):
			target = target[3:]
		default:
			return target
		}
	}
}

// TestNormaliseLinkTargetSeesEverySpelling is the POSITIVE CONTROL for the
// normaliser: a matcher that silently stopped recognising `../` would make the
// guard below pass over the exact link it exists to reject, and nothing else in
// this file would notice. Every form here must reduce to the same file name, and
// the absolute form must reduce to nothing.
func TestNormaliseLinkTargetSeesEverySpelling(t *testing.T) {
	relative := []string{
		"AGENTS.md", "./AGENTS.md", "../AGENTS.md", "../../AGENTS.md",
		"AGENTS.md#layout", "./AGENTS.md#layout", "<AGENTS.md>",
	}
	for _, in := range relative {
		if got := normaliseLinkTarget(in); got != "AGENTS.md" {
			t.Errorf("normaliseLinkTarget(%q) = %q, want %q — this spelling of the hazard is invisible to the guard", in, got, "AGENTS.md")
		}
	}
	for _, in := range []string{
		"https://github.com/civitai/cli/blob/main/AGENTS.md",
		"http://example.com/AGENTS.md",
		"#anchor-only",
	} {
		if got := normaliseLinkTarget(in); got != "" {
			t.Errorf("normaliseLinkTarget(%q) = %q, want \"\" — an absolute or in-page link must not be flagged", in, got)
		}
	}
}

func TestREADMEDoesNotLinkContributorDocsRelatively(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	body := string(raw)

	links := relativeMDLinkRe.FindAllStringSubmatch(body, -1)
	// POSITIVE CONTROL: a regex that stopped matching would validate nothing
	// and report a serene pass over a README full of broken links.
	if len(links) < 20 {
		t.Fatalf("CONTROL failure: found only %d markdown links in README.md, want >= 20 — "+
			"the pattern %s has stopped matching, so every check below is vacuous", len(links), relativeMDLinkRe)
	}
	links = append(links, refDefMDLinkRe.FindAllStringSubmatch(body, -1)...)

	checkedTargets := 0
	for _, m := range links {
		target := normaliseLinkTarget(m[1])
		if target == "" {
			continue
		}
		checkedTargets++
		for _, doc := range contributorOnlyDocs {
			if target != doc {
				continue
			}
			t.Errorf("README.md links %s relatively (`%s`). That file is not in the npm package "+
				"or the Homebrew cask, so the link 404s for everyone who did not install from a git "+
				"checkout — use the absolute https://github.com/civitai/cli/blob/main/%s URL, or inline "+
				"the fact the reader was being sent for.", doc, m[0], doc)
		}
	}
	// POSITIVE CONTROL on the NORMALISER, not just the regex. Most of the
	// README's ~150 inline links are in-page `#anchor` targets, which
	// normaliseLinkTarget correctly discards — so the count that matters here is
	// the handful of real repo-relative file links (`examples/`, `LICENSE`,
	// `schema/…`). A normaliser that started returning "" for everything would
	// leave this at zero while the regex control above stayed happy.
	if checkedTargets < 3 {
		t.Fatalf("CONTROL failure: only %d relative FILE link target(s) survived normalisation, want >= 3. "+
			"The README has several (examples/, schema/, LICENSE); a lower count means normaliseLinkTarget is "+
			"discarding targets it should be checking, and every assertion above is vacuous", checkedTargets)
	}
	t.Logf("checked %d relative link target(s) against %d contributor-only doc(s)", checkedTargets, len(contributorOnlyDocs))
}

// TestContributorOnlyDocsAreReallyNotShipped is the BIDIRECTIONAL LEDGER behind
// the list above, and it exists because the list is the guard's whole reach: a
// doc missing from it is a doc the check above cannot see, which is exactly how
// `CONTRIBUTING.md` was linked relatively four lines from a link the same change
// had just fixed.
//
// 🔴 IT MUST FAIL IN BOTH DIRECTIONS, AND ONLY ONE OF THEM IS OBVIOUS. Growing
// the list wrongly (a doc that IS shipped) is caught by reading the two
// manifests. SHRINKING it is the silent one: deleting `"CONTRIBUTING.md"` from
// the slice leaves every assertion above satisfied and the hazard reopened — a
// surviving mutant, measured. So the list is derived-and-compared rather than
// merely sanity-checked: every markdown file at the repo root that neither
// shipping manifest carries MUST be on it.
func TestContributorOnlyDocsAreReallyNotShipped(t *testing.T) {
	root := repoRootDir(t)
	shipping := map[string]string{
		"npm/package.json": "",
		".goreleaser.yaml": "",
	}
	for name := range shipping {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		shipping[name] = string(b)
	}
	// CONTROL: the manifests must actually name the file that IS shipped, or a
	// "not mentioned" verdict below means "wrong file read", not "not shipped".
	for name, body := range shipping {
		if !strings.Contains(body, "README.md") {
			t.Fatalf("CONTROL failure: %s does not mention README.md, which IS shipped — this test is "+
				"reading the wrong file, so every 'not shipped' conclusion below is unfounded", name)
		}
	}

	shipped := func(doc string) bool {
		for _, body := range shipping {
			if strings.Contains(body, doc) {
				return true
			}
		}
		return false
	}

	// Direction 1 — nothing on the list may actually be shipped.
	listed := map[string]bool{}
	for _, doc := range contributorOnlyDocs {
		listed[doc] = true
		if shipped(doc) {
			t.Errorf("a shipping manifest now carries %s, so a relative README link to it is no longer broken — "+
				"remove it from contributorOnlyDocs (and re-check the link) rather than leaving a stale rule", doc)
		}
	}

	// Direction 2 — every unshipped root markdown file must BE on the list.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read repo root: %v", err)
	}
	rootDocs := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		rootDocs++
		if shipped(name) || listed[name] {
			continue
		}
		t.Errorf("%s sits at the repo root, is in NEITHER shipping manifest, and is not in contributorOnlyDocs — "+
			"so a relative README link to it 404s for every installed user and no test would notice. Add it to the list.", name)
	}
	// CONTROL: a ReadDir that found no markdown at all (wrong root) would make
	// direction 2 vacuous while direction 1 stayed green.
	if rootDocs < 2 {
		t.Fatalf("CONTROL failure: only %d markdown file(s) found at %s — the repo root has several, so this "+
			"walk is looking in the wrong place and direction 2 checked nothing", rootDocs, root)
	}
}

// TestREADMEIconAspectDoesNotForbidASquareIcon — the aspect table said
// "square-ish, **not exactly square**" while the paragraph two lines below
// recommends starting from 512 × 512, which is exactly square. The bound is
// 0.9 ≤ aspect ≤ 1.1, so 1:1 is the CENTRE of the accepted range; the phrasing
// meant "it need not be exactly square" and read as a prohibition.
//
// This is a contradiction check, not a style one: the two claims cannot both be
// followed, and a reader who believes the table will avoid the size the README
// tells them to start from.
func TestREADMEIconAspectDoesNotForbidASquareIcon(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	body := string(raw)

	// CONTROL: the recommendation this would contradict must actually be there.
	if !strings.Contains(body, "**512 × 512**") {
		t.Fatal("CONTROL failure: the README no longer recommends a 512 × 512 icon, so there is no " +
			"contradiction to guard against — re-check whether this test still means anything")
	}
	if !strings.Contains(body, "0.9 – 1.1") {
		t.Fatal("CONTROL failure: the icon aspect bound (0.9 – 1.1) is not in the README, so the row " +
			"this test is about has moved or gone")
	}
	if strings.Contains(body, "not exactly square") {
		t.Error("the icon aspect row says \"not exactly square\", which reads as a prohibition on 1:1 — " +
			"but 1:1 is the centre of the 0.9–1.1 range, and the README itself recommends a 512 × 512 icon")
	}
}
