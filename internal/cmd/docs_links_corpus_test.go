package cmd

// REPO-WIDE docs-URL liveness: every `developer.civitai.com` address this
// repository spells, not just the ones inside the `agent-setup` managed block.
//
// 🔴 WHY THIS FILE EXISTS, WHICH IS A MEASURED DEFECT AND NOT A GENERALISATION
// FOR ITS OWN SAKE. The path `/apps/responsive` on this host was a 404 shipped at
// FOUR scaffold template sites — `page-money`'s README and `App.tsx`,
// `page-vite`'s README and `index.css` — i.e. into every project `civitai app
// init` produced with either template. The correct address is
// `/apps/guide/responsive`, one dropped path segment away, and the page had never
// been anywhere else. Measured 2026-09-27: `/apps/responsive` 404,
// `/apps/guide/responsive` 200, and an impossible path on the same host 404, so
// the prober was not being blocked.
//
// (The dead path is written WITHOUT its scheme and host throughout this file, and
// so is every other dead address named in a comment here. That is not style — see
// the last 🔴 in this header. Spelling it whole would enrol it in this guard's own
// corpus and redden the run forever, from the file that defines the guard. It
// happened once while this file was being written, which is why the note exists.)
//
// It survived because BOTH of this repo's link guards were structurally unable to
// see it, for opposite reasons:
//
//	TestBlockDocsLinksResolve         — the right prober, the wrong corpus. Its
//	  (agent_setup_docs_test.go)        corpus is the `### Docs` section of the
//	                                    managed block; a scaffold template is not
//	                                    in the block. It was also wired to no job.
//	TestREADMEExternalURLsResolve     — wired (weekly, readme-links.yml), but its
//	  (readme_external_links_test.go)   corpus is `README.md` and nothing else.
//
// So the address rotted in the gap between "a careful prober nobody runs" and "a
// running prober that reads one file". This guard closes that gap by keeping the
// prober and widening the corpus to the whole tree, and
// `.github/workflows/docs-links.yml` runs it weekly.
//
// 🔴 IT DOES NOT REPLACE TestBlockDocsLinksResolve, AND MERGING THEM WOULD BE A
// REGRESSION. The block's URLs are written into somebody else's project and
// cannot be recalled, so their findings earn wording no other spelling does,
// naming the two files to edit. A generic "some link in the repo is dead" over a
// 48-address corpus is a worse message for that case, and this corpus is a
// superset, so the block's URLs are probed by both. Five duplicated requests a
// week is the whole cost of keeping the specific message.
//
// 🔴 WHY NOT FOLD THIS INTO TestREADMEExternalURLsResolve, WHICH ALREADY HAS THE
// CALLER. Because that prober is deliberately laxer, and the laxity is correct
// for its corpus and wrong for this one. It FOLLOWS REDIRECTS (it sets no
// CheckRedirect), so it scores a 301 as a clean 200 — exactly the failure this
// repo has measured on this host, where `/apps/guide` 301s to *cleartext* http;
// and it has no negative control, so it cannot tell a live page from a host
// answering OK to everything. Widening it would have meant either giving up those
// two properties for the docs corpus, or imposing them on ~50 unrelated
// github.com/npm links whose 3xx hops are nobody's defect. Sharing the CALLER
// instead of the prober gets the coverage without either trade.
//
// 🔴 READ THIS BEFORE WRITING A KNOWN-DEAD ADDRESS ANYWHERE IN THIS TREE. The
// corpus is the tree, so a dead URL spelled in a committed file — a sentinel, a
// fire-drill target, an example of what not to write — becomes a permanent
// finding against itself, and a permanently-red gate is worse than no gate. Two
// mechanisms exist for that and NEITHER is "add it to the skip list": reference
// an existing const instead of retyping the literal (as docsCorpusNotAPage does
// with mustNotResolve), or keep the host and the path in separate strings so the
// extractor never sees a whole address (as docsCorpusDrillURL does). Suppressing
// a dead address by URL is reserved for the one case in docsCorpusNotAPage.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docsSiteURLRe matches a docs-site address anywhere in a file: prose, a markdown
// link target, a code span, a Go string literal, a CSS comment, a shell heredoc.
//
// 🔴 IT IS AN ALLOW-LIST OF CHARACTERS, NOT A DENY-LIST, AND THAT IS A MEASURED
// CORRECTION RATHER THAN A PREFERENCE. The first version denied the punctuation
// seen next to a URL in this tree — whitespace, backtick, `*`, `)`, `<`, `>`,
// quotes, `,`, `;`, backslash — which is unbounded in the wrong direction: every
// byte nobody thought of is ACCEPTED. `scripts/dogfood/__pycache__/` holds a
// compiled copy of the dogfood runner, and Python marshals the source's string
// constants into it followed by bytecode, so the deny-list matched
// `…/agent-setup/prompt.md` and then ran on through `\xda\x05brief\xda\x06return`
// and ~200 bytes of opcodes. The probe reported it as a transport error (Go
// refused to parse the address), which the guard classes as UNREACHABLE — so a
// stray `.pyc` turned a clean pass into "PARTIAL RUN, the corpus was not fully
// verified", and only when the dogfood tests had run first. A gate whose colour
// depends on a generated file is a gate people learn to re-run.
//
// The allowed set is RFC 3986's unreserved and reserved characters MINUS the ones
// this tree writes immediately after an address — `,` `;` `'` `*` `(` `)` `[` `]`
// — following `absoluteURLRe` in readme_external_links_test.go, which took the
// same shape for the same reason. Every real spelling here survives it: `#`
// anchors, `.md` and `.txt` suffixes, `-` in page slugs, and the bare host with
// no path at all.
//
// Two spellings drove the exclusions and are worth naming, because a narrower
// version of this regex silently mis-probes them:
//
//	`<https://…/apps/guide/responsive>` — markdown autolinks. BOTH scaffold
//	  READMEs write the address this way, so `>` must not be part of a match or
//	  two of the four files the original defect shipped in are probed with a
//	  bracket welded on: a guaranteed 404 reported as a dead page.
//	`"https://developer.civitai.com/site/\n"` — a Go literal whose escape follows
//	  the address with no space. A backslash is outside the allow-list, so the
//	  match ends at `/site/`.
//
// The host is pinned rather than matching every URL because the general external
// link case is already owned by TestREADMEExternalURLsResolve, with the
// placeholder-host and not-a-page tables that case needs. The DOCS SITE is the
// thing this repo deletes its own prose in favour of, so a 404 there is content
// loss, and it needs no such tables: every address is meant to be a page.
var docsSiteURLRe = regexp.MustCompile(`https?://developer\.civitai\.com[A-Za-z0-9._~:/?#@!$&+=%-]*`)

// docsSiteURLTrailing is punctuation that ends a SENTENCE, never an address.
// `.` is the one that matters — prose routinely ends a line on a URL — and it is
// safe to strip because no docs page here ends in a dot.
const docsSiteURLTrailing = ".:!?"

// docsCorpusNotAPage lists addresses that MUST NOT be fetched, with the reason.
//
// 🔴 ONE ENTRY, AND THE SHORTNESS IS DEFENDED RATHER THAN LUCKY. An exclusion
// table over a liveness guard is where the guard narrows until it covers
// nothing, so the licence is not "known-failing" but "this repo NEEDS this
// address dead, and another guard depends on it being so". A genuinely dead docs
// page never qualifies: fix the address or publish the page. The two other
// deliberately-dead addresses in this tree are kept out of the corpus by
// construction instead — see docsCorpusSkipPathPrefixes and docsCorpusDrillURL —
// because an exclusion cannot distinguish a sentinel from a regression, and both
// of those would have needed one.
//
// Note the KEY: `mustNotResolve`, the const, not a retyped literal. Spelling that
// address out here would put it in the corpus from this very file.
var docsCorpusNotAPage = map[string]string{
	mustNotResolve: "this prober's OWN negative control (agent_setup_docs_test.go). It is an impossible " +
		"path and must answer >= 400 or every verdict here is void, so probing it as corpus would report " +
		"the control working as a dead link",
}

// docsCorpusSkipPathPrefixes are paths the walk does not read, beyond the
// directory set it shares with the spelling ledger.
//
// 🔴 `.github/workflows/` IS THE ONE PLACE IN THIS TREE WHERE A 404 IS THE
// SPECIFICATION. Both link guards are driven by a workflow that rehearses its own
// dead-link arm by planting an address that must not resolve —
// `readme-links.yml` has carried `zz-readme-links-fire-drill` since it was
// written, and `docs-links.yml` carries this guard's equivalent. Neither is a
// link anybody follows: nothing reads a URL out of a CI job. Reading them would
// make this guard permanently red for two properties working exactly as designed,
// and suppressing them by URL would leave the drill planting an address the
// prober skips — a rehearsal of nothing that reports green.
//
// The cost is real and bounded: a docs address written in a workflow comment is
// unchecked. That is acceptable because no reader follows it; it is NOT a
// precedent for skipping a directory that ships something.
var docsCorpusSkipPathPrefixes = []string{".github/workflows/"}

// docsCorpusDrillHost and docsCorpusDrillPath assemble the address
// `.github/workflows/docs-links.yml` appends to a scaffold template during a fire
// drill.
//
// 🔴 THEY ARE TWO STRINGS FOR ONE REASON: A WHOLE ADDRESS HERE WOULD PUT A KNOWN
// 404 INTO THIS GUARD'S OWN CORPUS, from the file that defines the guard, and the
// run would be red forever. Do not "tidy" this into one const — the host half
// ends in `/`, so what the extractor sees from this file is the docs root, which
// is live (measured 200 on 2026-09-27).
//
// 🔴 AND IT IS DELIBERATELY *NOT* IN docsCorpusNotAPage. A drill whose address
// this prober skips rehearses nothing and reports green, which is the same class
// of defect as a guard that cannot fail. It must be probed, found dead, and
// reported against the template the workflow planted it in.
const (
	docsCorpusDrillHost = "https://developer.civitai.com/"
	docsCorpusDrillPath = "zz-docs-links-fire-drill"
)

// docsCorpusDrillURL is the assembled drill address.
func docsCorpusDrillURL() string { return docsCorpusDrillHost + docsCorpusDrillPath }

// docsCorpusMinURLs is an anti-vacuity FLOOR, not a census.
//
// It answers one question — "is the walk still extracting addresses at all?" — and
// is deliberately slack, following readmeMinExternalURLs' hard-won reasoning in
// readme_external_links_test.go: set to the exact count, it fires on every honest
// link removal with a message telling you to lower the constant, which is a
// ratchet rather than an invariant. 48 distinct addresses over 792 files were
// extracted on 2026-09-27; those numbers are history, not a claim about now, and
// what actually defends this guard is docsCorpusMustCover below.
const docsCorpusMinURLs = 20

// docsCorpusMustCover is the COVERAGE control, and it is the one assertion in
// this file that the defect above could not have survived.
//
// 🔴 "0 DEAD LINKS" FROM A PROBER WIRED TO THE WRONG CORPUS IS INDISTINGUISHABLE
// FROM A CLEAN REPO. That is not a hypothetical here — it is the history of this
// file: TestBlockDocsLinksResolve reported clean for as long as
// `/apps/responsive` shipped, because the four files carrying it were outside
// what it read. The header claims "every address this repository spells", which
// is a claim about COVERAGE, so the coverage is asserted rather than described:
// each path below must appear among the files this walk attributed an address to.
//
// These four are the scaffold templates specifically. They are the shipped
// artefacts furthest from the managed block, written into a stranger's project,
// and the ones whose absence from a corpus is invisible.
var docsCorpusMustCover = []string{
	"internal/scaffold/templates/page-money/README.md.tmpl",
	"internal/scaffold/templates/page-money/src/App.tsx.tmpl",
	"internal/scaffold/templates/page-vite/README.md.tmpl",
	"internal/scaffold/templates/page-vite/src/index.css.tmpl",
}

// isProbableTextFile reports whether a file's bytes are worth reading for
// addresses.
//
// 🔴 A COMPILED ARTEFACT IS NOT A SPELLING. Generated and compiled files embed
// their source's string constants, so a `.pyc`, a built binary or a cache entry
// can contain a docs address nobody wrote and nobody can edit — and attributing a
// finding to one sends the reader to a file they would have to regenerate rather
// than repair. Worse, those files come and go with whatever last ran, so reading
// them makes this guard's corpus depend on the state of the working tree.
// MEASURED: a `scripts/dogfood/__pycache__/` entry left behind by the dogfood
// tests turned a clean pass into a PARTIAL RUN skip, and only after those tests
// had run. A gate whose colour depends on a generated file is a gate people learn
// to re-run rather than read.
//
// The NUL test is git's own text/binary heuristic. It scans the WHOLE buffer
// rather than a prefix: these files are under the 2 MB cap the walk applies, so
// the second pass costs nothing, and a prefix test misses a NUL that appears late.
func isProbableTextFile(body []byte) bool {
	return bytes.IndexByte(body, 0) < 0
}

// extractDocsSiteURLs returns the distinct docs-site addresses spelled in one
// file's bytes, in the order first seen, with sentence punctuation trimmed. It
// returns nothing for bytes that are not text.
//
// It is a pure function so TestDocsSiteURLExtractorStopsAtTheAddress can feed it
// the exact byte sequences that have broken it, rather than reasoning about the
// regex by eye — which is how the deny-list version shipped.
func extractDocsSiteURLs(body []byte) []string {
	if !isProbableTextFile(body) {
		return nil
	}
	// Cheap reject before the regex: the overwhelming majority of files in this
	// tree do not mention the host at all.
	if !bytes.Contains(body, []byte("developer.civitai.com")) {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range docsSiteURLRe.FindAllString(string(body), -1) {
		u := strings.TrimRight(raw, docsSiteURLTrailing)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// TestDocsSiteURLExtractorStopsAtTheAddress pins the extractor against the exact
// byte sequences that have broken it or would.
//
// 🔴 EVERY CASE HERE IS A REAL SPELLING FROM THIS TREE, NOT AN INVENTED FIXTURE,
// and two of them are measured failures rather than anticipated ones. Without
// this, the extractor is verified only by "the corpus looked about the right
// size", which is exactly how it shipped matching 200 bytes of Python bytecode as
// part of a URL.
func TestDocsSiteURLExtractorStopsAtTheAddress(t *testing.T) {
	const host = "https://developer.civitai.com"
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{
			// MEASURED FAILURE. `scripts/dogfood/__pycache__/` holds a marshalled
			// copy of the runner's string constants followed by bytecode, and the
			// deny-list regex ran straight through it. The bytes after the address
			// are taken from the real file.
			name: "python bytecode after an address is not part of it",
			body: host + "/agent-setup/prompt.md\xda\x05brief\xda\x06returnc\x01\x02",
			want: []string{host + "/agent-setup/prompt.md"},
		},
		{
			// The same file, but with the NUL bytes a real `.pyc` carries: the
			// whole buffer is refused rather than parsed. This is the arm that
			// stops a finding being attributed to a generated file at all.
			name: "a buffer containing a NUL is not read",
			body: host + "/agent-setup/prompt.md\x00\x00\x00",
			want: nil,
		},
		{
			// Both scaffold READMEs write the responsive guide this way. If `>`
			// were matched, the address is probed with a bracket welded on — a
			// guaranteed 404, reported as a dead page, in the very files this
			// guard exists for.
			name: "markdown autolink brackets are not part of the address",
			body: "Full guide: <" + host + "/apps/guide/responsive>\n",
			want: []string{host + "/apps/guide/responsive"},
		},
		{
			// A real line in internal/cmd/agent_setup.go.
			name: "a Go escape immediately after an address ends it",
			body: `docs := "` + host + `/site/\n"`,
			want: []string{host + "/site/"},
		},
		{
			// page-vite/src/index.css.tmpl closes its comment right after the URL.
			name: "a CSS comment terminator is not part of the address",
			body: "   Guide: " + host + "/apps/guide/responsive */\n",
			want: []string{host + "/apps/guide/responsive"},
		},
		{
			name: "a sentence-ending period is trimmed but an anchor is kept",
			body: "See " + host + "/apps/guide/packaging#how-big-can-a-bundle-be.\n",
			want: []string{host + "/apps/guide/packaging#how-big-can-a-bundle-be"},
		},
		{
			name: "the bare host with no path is an address",
			body: "the docs live at " + host + "\n",
			want: []string{host},
		},
		{
			name: "a path-only mention with no scheme or host is not an address",
			body: "the page is at /apps/guide/responsive on developer.civitai.com\n",
			want: nil,
		},
		{
			name: "the same address twice in one file is reported once",
			body: host + "/llms.txt and again " + host + "/llms.txt\n",
			want: []string{host + "/llms.txt"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := extractDocsSiteURLs([]byte(tc.body))
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("extractDocsSiteURLs:\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}

	// POSITIVE CONTROL on the table itself: a table of cases that all expect
	// NOTHING would pass against an extractor wired to nothing. At least one case
	// must expect an address, and the count is asserted so a future edit cannot
	// quietly turn this into a set of negative cases.
	extracting := 0
	for _, body := range []string{
		host + "/llms.txt\n",
		"<" + host + "/apps/guide/responsive>\n",
	} {
		if len(extractDocsSiteURLs([]byte(body))) > 0 {
			extracting++
		}
	}
	if extracting != 2 {
		t.Fatalf("CONTROL failure, not a finding: %d of 2 plainly-valid addresses were extracted. "+
			"docsSiteURLRe matches nothing, so every 'want: nil' case above passes vacuously and the whole "+
			"corpus walk is reading an empty set", extracting)
	}
}

// collectDocsSiteURLs walks the repository and returns each distinct docs-site
// address with the sorted list of files spelling it, plus the number of files
// read. The file list is the whole reason this returns a map rather than a slice:
// a finding must say WHERE to edit, and "a docs link is dead" over 790 files is
// not an actionable message.
//
// The skip-dir set and the dated-record exemption are shared with
// TestEveryDocsURLSpellingIsLedgered rather than re-declared: two walks of one
// tree disagreeing about what counts is a bug generator, and `.claude` in
// particular is load-bearing — this repo's worktrees live there and each is a
// full copy of the tree, so a walk that entered it would attribute every address
// to a second path.
func collectDocsSiteURLs(t *testing.T) (map[string][]string, int) {
	t.Helper()
	root := repoRootDir(t)
	urls := map[string]map[string]bool{}
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && docsURLLedgerSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Size() > 2<<20 {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !isProbableTextFile(body) {
			return nil
		}
		scanned++
		// A dated record is EVIDENCE of what was true when it was written, not an
		// address anybody follows from a shipped artefact. Correcting one would
		// falsify the record, so a dead URL in one is not a finding. Same
		// exemption, same reason, as the spelling ledger's.
		if strings.HasPrefix(rel, docsURLLedgerExemptPrefix) {
			return nil
		}
		for _, prefix := range docsCorpusSkipPathPrefixes {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		for _, u := range extractDocsSiteURLs(body) {
			if urls[u] == nil {
				urls[u] = map[string]bool{}
			}
			urls[u][rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	out := make(map[string][]string, len(urls))
	for u, files := range urls {
		list := make([]string, 0, len(files))
		for f := range files {
			list = append(list, f)
		}
		sort.Strings(list)
		out[u] = list
	}
	return out, scanned
}

// sortedDocsCorpusURLs orders the corpus so every run probes and reports in the
// same order — a findings list that reshuffles between runs is one a reader
// cannot diff, and a probe order that reshuffles makes a rate-limit look random.
func sortedDocsCorpusURLs(corpus map[string][]string) []string {
	out := make([]string, 0, len(corpus))
	for u := range corpus {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// TestDocsURLCorpusIsExtractableAndCoversTheTemplates is the OFFLINE half, and it
// is the half that runs on every push under `make ci`.
//
// It asserts three things, and the last is the one the network prober cannot
// assert about itself: that the walk read a real tree, that the extractor still
// matches, and that the corpus actually includes the scaffold templates.
func TestDocsURLCorpusIsExtractableAndCoversTheTemplates(t *testing.T) {
	corpus, scanned := collectDocsSiteURLs(t)

	// Positive control on the WALK. A misrooted walk reads nothing, finds
	// nothing, and reports every assertion below satisfied. Same floor and same
	// reasoning as TestEveryDocsURLSpellingIsLedgered's.
	if scanned < 200 {
		t.Fatalf("CONTROL failure, not a finding: the walk read %d file(s) under %s, want >= 200. "+
			"This repository had 790 readable files on 2026-09-27, so a number this low means the walk is "+
			"rooted somewhere else or is skipping everything — not that the corpus is clean",
			scanned, repoRootDir(t))
	}

	// Positive control on the EXTRACTOR. A regex that stopped matching returns an
	// empty corpus, over which every assertion below passes.
	if len(corpus) < docsCorpusMinURLs {
		t.Fatalf("CONTROL failure, not a finding: extracted %d distinct docs address(es) from %d file(s), "+
			"want >= %d. Either docsSiteURLRe (%s) has stopped matching — in which case the liveness run is "+
			"probing nothing — or a great many links were removed. If they were removed on purpose, lower "+
			"docsCorpusMinURLs in the same commit and say why.",
			len(corpus), scanned, docsCorpusMinURLs, docsSiteURLRe)
	}

	// 🔴 THE COVERAGE CONTROL. See docsCorpusMustCover: this is the assertion the
	// shipped 404 could not have survived.
	covered := map[string]bool{}
	for _, files := range corpus {
		for _, f := range files {
			covered[f] = true
		}
	}
	for _, want := range docsCorpusMustCover {
		if covered[want] {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRootDir(t), filepath.FromSlash(want))); err != nil {
			t.Errorf("COVERAGE control: docsCorpusMustCover names %s, which cannot be read: %v. "+
				"Either the template moved — update the list — or the scaffold no longer ships it.", want, err)
			continue
		}
		t.Errorf("COVERAGE control failure, and it is NOT a finding about link health: %s exists but this "+
			"walk attributed NO docs address to it, so the liveness run says nothing whatever about what it "+
			"ships.\n"+
			"  This is the exact shape of the defect this guard was built for: a prober reporting '0 dead "+
			"links' over a corpus that excludes the files carrying the dead link.\n"+
			"  Check, in this order: (1) did the file legitimately stop spelling a docs address — then drop "+
			"it from docsCorpusMustCover and say so; (2) is docsSiteURLRe failing on how this file writes "+
			"the address (a new quote, bracket or escape around it); (3) is the walk skipping the directory.",
			want)
	}

	t.Logf("docs-address corpus: %d distinct address(es) over %d file(s) scanned; %d suppressed sentinel(s); "+
		"all %d template coverage target(s) present", len(corpus), scanned, len(docsCorpusNotAPage),
		len(docsCorpusMustCover))
}

// TestDocsCorpusDrillURLMatchesTheWorkflow keeps the fire drill honest.
//
// 🔴 A DRILL THAT PLANTS AN ADDRESS THIS GUARD DOES NOT PROBE REHEARSES NOTHING,
// AND IT FAILS GREEN — the workflow appends it, the prober never sees it, the run
// passes, and the reader concludes the dead-link path works. The drill's address
// lives in YAML and this guard's tables live in Go; nothing but this test stops
// them drifting apart.
//
// It asserts three things: the workflow exists at all (without it the prober is
// back to being a test nobody runs, which is the defect this whole change is
// about), the workflow plants the address this file names, and that address is
// NOT suppressed.
func TestDocsCorpusDrillURLMatchesTheWorkflow(t *testing.T) {
	rel := filepath.Join(".github", "workflows", "docs-links.yml")
	path := filepath.Join(repoRootDir(t), rel)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\nThis guard's only CI caller is missing, so TestEveryShippedDocsURLResolves "+
			"and TestBlockDocsLinksResolve are both behind an env var no job sets — a guard nobody runs "+
			"reads as coverage while providing none. Restore the workflow or delete the guards; do not "+
			"leave them in between.", rel, err)
	}
	if !bytes.Contains(body, []byte(docsCorpusDrillURL())) {
		t.Errorf("%s does not spell %s.\n"+
			"  The drill appends that address to a scaffold template in the runner's checkout so the "+
			"DEAD-DOCS-LINK arm is rehearsed against a real 404. If the workflow plants a different "+
			"address, the drill exercises nothing and still reports a successful rehearsal.",
			rel, docsCorpusDrillURL())
	}
	if why, suppressed := docsCorpusNotAPage[docsCorpusDrillURL()]; suppressed {
		t.Errorf("the drill address %s is in docsCorpusNotAPage (%q), so the fire drill plants something "+
			"this prober skips and the run goes GREEN. The drill's whole purpose is to be probed and found "+
			"dead — remove the suppression.", docsCorpusDrillURL(), why)
	}
}

// TestEveryShippedDocsURLResolves is the class guard: it probes every docs-site
// address in the repository, once each.
//
// It reuses TestBlockDocsLinksResolve's machinery deliberately and entirely — the
// same client (redirects NOT followed, because the spelling is the claim), the
// same negative control, the same four separable verdicts, the same rule that
// nothing in the probe loop can end the run, and the same ability to answer
// "cannot tell" rather than forcing a verdict. What differs is the corpus, and
// the wording of the findings, which name the files to edit.
func TestEveryShippedDocsURLResolves(t *testing.T) {
	if os.Getenv(docsLinkProbeEnv) != "1" {
		t.Skipf("network guard; set %s=1 to run (weekly in .github/workflows/docs-links.yml)", docsLinkProbeEnv)
	}

	corpus, scanned := collectDocsSiteURLs(t)
	if scanned < 200 || len(corpus) < docsCorpusMinURLs {
		t.Fatalf("CONTROL failure, not a finding: the walk read %d file(s) and extracted %d address(es) "+
			"(want >= 200 and >= %d). This run would be a serene pass over an unread tree — see "+
			"TestDocsURLCorpusIsExtractableAndCoversTheTemplates, which diagnoses this offline.",
			scanned, len(corpus), docsCorpusMinURLs)
	}

	client := newDocsProbeClient()
	control := runDocsProbeNegativeControl(t, client)

	// DEDUPE IS THE CORPUS ITSELF: the map is keyed by address, so N distinct
	// addresses are N requests however many files spell them. Without it
	// `/apps/guide/store-listing` alone would be fetched 11 times, and a guard
	// that hammers a docs host is a guard that rate-limits itself into permanent
	// CANNOT TELL.
	var probed []string
	suppressed := 0
	for _, u := range sortedDocsCorpusURLs(corpus) {
		if why, ok := docsCorpusNotAPage[u]; ok {
			suppressed++
			t.Logf("not probed: %s — %s", u, why)
			continue
		}
		probed = append(probed, u)
	}

	// Probe EVERY address first. Nothing in this loop can end the run: the
	// version of the sibling guard that diagnosed inside its loop abandoned the
	// whole run on the first transport error and reported a pass.
	results := make([]docsProbeResult, 0, len(probed))
	for _, u := range probed {
		results = append(results, probeDocsURL(client, u))
	}

	var (
		unreachable []docsProbeResult
		moved       []docsProbeResult
		gone        []docsProbeResult
		cannotTell  []docsProbeResult
		sawLive     bool
	)
	for _, r := range results {
		switch {
		case r.err != nil:
			unreachable = append(unreachable, r)
		case r.code < 300:
			sawLive = true
		case r.code < 400:
			moved = append(moved, r)
		case r.code == 404 || r.code == 410:
			gone = append(gone, r)
		default:
			cannotTell = append(cannotTell, r)
		}
	}
	checked := len(results) - len(unreachable)
	for _, r := range unreachable {
		t.Logf("%s -> transport error: %v (continuing; this run does NOT claim to have checked it)", r.url, r.err)
	}

	// Report the PAIR, never the reassuring zero alone: "0 gone" and "nothing was
	// fetched" are the same line otherwise.
	t.Logf("docs-corpus liveness: probed %d of %d address(es) over %d file(s) scanned "+
		"(%d sentinel(s) not probed); %d live, %d moved, %d gone, %d undiagnosable, %d unreachable",
		checked, len(probed), scanned, suppressed,
		checkedLive(results), len(moved), len(gone), len(cannotTell), len(unreachable))

	if checked == 0 {
		t.Skipf("probed 0 of %d address(es) — every request failed at the transport layer. That is a "+
			"finding about this machine's network, not about the docs site, so it skips rather than fails. "+
			"Nothing was verified.", len(probed))
	}

	for _, r := range gone {
		t.Errorf("DEAD DOCS LINK — this address answers %d and is spelled in %d file(s) here.\n"+
			"  url:   %s\n  files: %s\n"+
			"  fix:   correct the address in every file listed, or publish the page before merging. A "+
			"scaffold template and a compiled help string cannot be recalled once released, so a wrong "+
			"address in one outlives the fix. If the page moved, read the SPELLING carefully: this host "+
			"serves `/apps/guide/<page>`, and an `/apps/<page>` address that looks right 404s — that is "+
			"the defect this guard was built for.",
			r.code, len(corpus[r.url]), r.url, strings.Join(corpus[r.url], ", "))
	}
	for _, r := range moved {
		t.Errorf("MOVED DOCS LINK — the page answers, but NOT at the spelling this repository writes. Not a "+
			"dead link and not a live one: a browser follows the hop, and every file below is left naming an "+
			"address the site no longer serves directly.\n  url:      %s\n  status:   %d\n"+
			"  location: %s\n  files:    %s\n"+
			"  fix:      if the target is the intended canonical spelling, write THAT into the files listed. "+
			"Check the scheme before copying it: this host has been measured redirecting https to cleartext "+
			"http, and shipping a cleartext link is a downgrade, not a repair.",
			r.url, r.code, orNone(r.location), strings.Join(corpus[r.url], ", "))
	}
	if len(cannotTell) > 0 {
		var lines []string
		for _, r := range cannotTell {
			lines = append(lines, fmt.Sprintf("    %s -> %d  (%s)", r.url, r.code, strings.Join(corpus[r.url], ", ")))
		}
		msg := fmt.Sprintf("CANNOT TELL whether these addresses are dead — they answered a status a BLOCKED "+
			"or FAILING EDGE produces just as readily as a broken page (401/403/407/429/5xx):\n%s\n"+
			"  The negative control %s answered %d.\n"+
			"  🔴 This is NOT a dead-link finding. Do not edit any file on the strength of it. Re-run; if it "+
			"persists, fetch the address in a browser and check whether the host is refusing this probe's "+
			"User-Agent (%q) — developer.civitai.com is known to hard-block non-browser agents with "+
			"Cloudflare `error code: 1010`. A %d-address corpus is also enough to trip a rate limit, which "+
			"looks identical from here.",
			strings.Join(lines, "\n"), mustNotResolve, control.code, docsProbeUserAgent, len(probed))
		switch {
		case sawLive:
			// A live address in the same run IS the positive control: the prober
			// can see a live page, so a >= 400 elsewhere is a real anomaly worth
			// failing on — it is just not diagnosable as "gone".
			t.Errorf("%s\n  (At least one other address in this run answered < 300, so this prober is not "+
				"being blocked wholesale — which is what makes this worth failing on rather than skipping.)", msg)
		case !t.Failed():
			// No address answered < 300 anywhere. There is no positive control, so
			// this run cannot separate "the site is down or blocking" from "the
			// pages are gone", and a failure here would be a guess.
			t.Skipf("%s\n  NO address in this run answered < 300, so there is no positive control at all: "+
				"this run cannot distinguish a blocked prober from a dead docs site. Skipping rather than "+
				"guessing.", msg)
		default:
			t.Errorf("%s\n  (No address in this run answered < 300, so the dead-link finding(s) above are "+
				"reported WITHOUT a positive control — a site-wide outage or block is not excluded.)", msg)
		}
	}
	if len(gone) > 0 && !sawLive {
		t.Errorf("POSITIVE CONTROL ABSENT: %d address(es) are reported gone above and NOT ONE in this run "+
			"answered < 300. A prober that never saw a live page cannot distinguish 'these pages were "+
			"removed' from 'this whole host is answering 404 to me'. Confirm one of them in a browser "+
			"before editing any file.", len(gone))
	}
	if !t.Failed() && len(unreachable) > 0 {
		t.Skipf("PARTIAL RUN: %d of %d address(es) checked, %d unreachable at the transport layer. The ones "+
			"that WERE checked are fine — but this run did not verify the whole corpus, so it reports SKIP "+
			"rather than a pass that reads as 'all links verified'.", checked, len(probed), len(unreachable))
	}
}
