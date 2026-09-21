package scaffold

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// Guards for the hook index (hooks.go): that it is a well-formed table, that a
// scaffolded README actually carries it where a reader arrives, that it covers
// every hook the scaffold's own code imports, and — network-gated — that it
// covers every hook the published SDK exports.

// ---------------------------------------------------------------------------
// Structure
// ---------------------------------------------------------------------------

// TestHookIndexRowsAreWellFormed is the cheap structural floor. Each clause is a
// separate defect that renders as a plausible-looking table.
func TestHookIndexRowsAreWellFormed(t *testing.T) {
	if len(SDKHooks) < 20 {
		t.Fatalf("CONTROL failure, not a finding: SDKHooks has %d row(s). The published SDK exported 36 "+
			"`use*` hooks when this index was written, so a number this low means the table was gutted, "+
			"not that the SDK shrank", len(SDKHooks))
	}
	seen := map[string]bool{}
	for _, h := range SDKHooks {
		if !strings.HasPrefix(h.Name, "use") {
			t.Errorf("%q is in the hook index and is not a hook name", h.Name)
		}
		if seen[h.Name] {
			t.Errorf("%s is listed twice — the index claims to be an enumeration", h.Name)
		}
		seen[h.Name] = true
		if strings.TrimSpace(h.Summary) == "" {
			t.Errorf("%s has no summary — a name with no clause tells a reader nothing they could not "+
				"have got from an import statement", h.Name)
		}
		// 🔴 A PIPE IN A CELL SILENTLY SPLITS THE ROW. It does not error, it does
		// not fail to render: it produces a table with one extra column whose
		// cells are shifted, which is how `Checkpoint | LORA` shipped a scope
		// column reading "the app never browses a catalog" in a first draft here.
		for _, field := range []struct{ what, val string }{
			{"Name", h.Name}, {"Summary", h.Summary}, {"Scope", h.Scope},
		} {
			if strings.Contains(field.val, "|") {
				t.Errorf("%s's %s contains a `|`, which splits the markdown row into an extra column: %q",
					h.Name, field.what, field.val)
			}
		}
	}
}

// TestEveryScopeInTheHookIndexIsARealScope ties the Scope column to the schema
// this CLI already embeds and validates manifests against.
//
// 🔴 THAT IS THE POINT: THE COLUMN IS AN INSTRUCTION TO EDIT `block.manifest.json`.
// A scope spelled `post:write:self` (singular) or `apps:shared:storage:write`
// (transposed) reads exactly like the right answer and fails server-side at
// submit, after the author has written the code. The schema's enum is the same
// list `civitai app validate` enforces, so a typo here cannot survive.
func TestEveryScopeInTheHookIndexIsARealScope(t *testing.T) {
	valid := manifestScopeEnum(t)
	if len(valid) < 5 {
		t.Fatalf("CONTROL failure, not a finding: the embedded manifest schema yielded %d scope(s) — "+
			"the enum could not be read, so every assertion below is against an empty set", len(valid))
	}
	for _, h := range SDKHooks {
		if h.Scope == "" {
			continue
		}
		for _, s := range strings.Split(h.Scope, " + ") {
			s = strings.TrimSpace(s)
			if !valid[s] {
				t.Errorf("%s names the scope %q, which is NOT in the manifest schema's scope enum "+
					"(%v). An author who copies it into block.manifest.json fails validation.",
					h.Name, s, sortedSet(valid))
			}
		}
	}
	// The consent-gated set is the other half of the column's claim, and it is
	// hand-maintained: a scope named there that no row uses is a rule pointing
	// at nothing.
	for scope := range consentGatedScopes {
		if !valid[scope] {
			t.Errorf("consentGatedScopes names %q, which is not a real scope", scope)
		}
	}
}

// manifestScopeEnum reads the scope vocabulary out of the embedded canonical
// manifest schema — the same bytes `civitai app validate` evaluates.
func manifestScopeEnum(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "schema", "app-block.manifest.schema.json"))
	if err != nil {
		t.Fatalf("read the embedded manifest schema: %v", err)
	}
	var doc struct {
		Properties struct {
			Scopes struct {
				Items struct {
					Enum []string `json:"enum"`
				} `json:"items"`
			} `json:"scopes"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse the embedded manifest schema: %v", err)
	}
	out := map[string]bool{}
	for _, s := range doc.Properties.Scopes.Items.Enum {
		out[s] = true
	}
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Where it lands in the rendered README
// ---------------------------------------------------------------------------

// hookIndexRowRe pulls the hook NAMES back out of a rendered table, so the
// assertions below read the document rather than the Go slice they came from.
var hookIndexRowRe = regexp.MustCompile("(?m)^\\| `(use[A-Za-z0-9]+)\\(\\)` \\|")

// TestTheScaffoldedReadmeCarriesTheHookIndexWhereItIsRead is the regression
// guard for the measured defect.
//
// 🔴 WHAT WAS MEASURED. A blind agent asked to "generate an image and then post
// it to Civitai" needed `useCreatePostFromApp`, which appeared in NO scaffolded
// file: not the README, not AGENTS.md, not the 5,908 lines of scaffolded `src/`.
// It reverse-engineered the hook from the package's `.d.ts` at a cost of 706,371
// carried tokens — 28.2% of the run — and wrote no code in 66 steps.
//
// 🔴 THE POSITION BOUND IS A *START* BOUND, AND THE DIFFERENCE IS NOT PEDANTRY.
// 36 rows cannot fit in 3 KB and still say what each hook is for, so requiring
// the whole table inside 3 KB would force the table to be useless in order to
// be early. What the measurement supports is that the reader arrives at it: the
// dogfooding agent read the first ~200 lines of the README before it paged, so
// the index must BEGIN inside the first 3 KB and be COMPLETE inside those 200
// lines. Both numbers are asserted; neither is a taste judgement.
func TestTheScaffoldedReadmeCarriesTheHookIndexWhereItIsRead(t *testing.T) {
	const (
		maxStartByte = 3072
		maxEndLine   = 200
	)
	readme := scaffoldedReadme(t)
	block := RenderHookIndex()

	start := strings.Index(readme, block)
	if start < 0 {
		t.Fatalf("the scaffolded README does not contain the hook index at all. `Render` passes it in as "+
			"`{{ .HookIndex }}`; a template that stopped referencing it renders a README with no index and "+
			"no error. README starts:\n%s", readme[:min(len(readme), 400)])
	}
	if start > maxStartByte {
		t.Errorf("the hook index starts at byte %d of the scaffolded README, past the %d-byte bound. "+
			"The whole point is that a reader meets the SDK's capability list BEFORE it has read enough "+
			"prose to start guessing.", start, maxStartByte)
	}
	endLine := strings.Count(readme[:start+len(block)], "\n") + 1
	if endLine > maxEndLine {
		t.Errorf("the hook index ends on line %d of the scaffolded README, past line %d — which is roughly "+
			"as far as the measured agent read before it paged. An index a reader never reaches is the "+
			"defect this guard exists for.", endLine, maxEndLine)
	}

	// The index must be the FULL export set as rendered, not just as declared:
	// read the names back out of the markdown.
	var rendered []string
	for _, m := range hookIndexRowRe.FindAllStringSubmatch(block, -1) {
		rendered = append(rendered, m[1])
	}
	if diff := setDiff(rendered, HookNames()); diff != "" {
		t.Errorf("the RENDERED table and SDKHooks disagree: %s. The renderer is dropping or mangling rows, "+
			"so the table is not the enumeration it says it is.", diff)
	}
}

// TestHookIndexRowExtractorCanFail is the negative control for the extractor
// above: fed a table it must report rows, fed prose it must report none.
func TestHookIndexRowExtractorCanFail(t *testing.T) {
	if got := hookIndexRowRe.FindAllStringSubmatch("A paragraph naming `useCreatePostFromApp()` inline.\n", -1); got != nil {
		t.Errorf("the row extractor matched prose, so it is not reading TABLE ROWS: %v", got)
	}
	got := hookIndexRowRe.FindAllStringSubmatch("| `useThing()` | a thing | — |\n", -1)
	if len(got) != 1 || got[0][1] != "useThing" {
		t.Errorf("the row extractor did not read a real row: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Coverage of what the scaffold itself uses
// ---------------------------------------------------------------------------

// sdkImportRe finds an import of `@civitai/blocks-react` (root or a subpath) and
// captures its named-import clause.
var sdkImportRe = regexp.MustCompile(`(?s)import\s*\{([^}]*)\}\s*from\s*['"]@civitai/blocks-react[^'"]*['"]`)

// TestEveryHookTheScaffoldImportsIsInTheIndex is the offline coverage guard, and
// it is a RELATIONSHIP rather than a restatement: it does not compare the index
// to a list written in this file, it compares it to what the generated app's own
// source actually imports from the SDK.
//
// The index is allowed to be WIDER than the sample — that is its entire purpose.
// It may not be NARROWER, because a hook the sample calls and the index omits is
// a hook a reader meets in the code with no entry to look it up by.
func TestEveryHookTheScaffoldImportsIsInTheIndex(t *testing.T) {
	dir := renderPageMoney(t)
	indexed := map[string]bool{}
	for _, n := range HookNames() {
		indexed[n] = true
	}

	imported := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch filepath.Ext(path) {
		case ".ts", ".tsx", ".js", ".jsx":
		default:
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range sdkImportRe.FindAllStringSubmatch(string(raw), -1) {
			for _, spec := range strings.Split(m[1], ",") {
				// `useX as y` / `type X` — take the imported name. A trailing
				// comma yields an empty spec, which has no fields at all.
				fields := strings.Fields(spec)
				if len(fields) == 0 {
					continue
				}
				if name := fields[0]; strings.HasPrefix(name, "use") {
					imported[name] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the rendered scaffold: %v", err)
	}

	// Positive control: an extractor that matches nothing would pass this test
	// silently, which is the exact shape of a guard wired to nothing.
	if len(imported) < 3 {
		t.Fatalf("CONTROL failure, not a finding: found %d SDK hook import(s) in the rendered page-money "+
			"scaffold (%v), want >= 3. The import extractor is not reading the source — the assertion "+
			"below would pass over an empty set", len(imported), sortedSet(imported))
	}
	for name := range imported {
		if !indexed[name] {
			t.Errorf("the page-money scaffold imports %s from @civitai/blocks-react, and the README's hook "+
				"index does not list it. A reader who meets it in src/ has nothing to look it up by — "+
				"which is the whole defect. Add a row to SDKHooks.", name)
		}
	}
	t.Logf("%d hook(s) imported by the scaffold, %d indexed", len(imported), len(indexed))
}

// ---------------------------------------------------------------------------
// Coverage of what the SDK publishes (network-gated)
// ---------------------------------------------------------------------------

// publishedHooksEnv gates the one network-touching guard here, following the
// existing `CIVITAI_CHECK_PUBLISHED_PINS` pattern: `make ci` must stay green
// offline, so this is a manual / scheduled check and saying so is the honest
// version of "the index is complete".
const publishedHooksEnv = "CIVITAI_CHECK_PUBLISHED_HOOKS"

// dtsHookExportRe captures the `use*` VALUE exports of a `.d.ts` barrel. `export
// type { … }` is deliberately not matched: a type is not a hook.
var dtsHookExportRe = regexp.MustCompile(`(?m)^export \{([^}]*)\}`)

// TestHookIndexMatchesThePublishedSDK is what makes "these are ALL of them" a
// checked claim rather than a remembered one.
//
// It resolves the published `@civitai/blocks-react`, extracts every `use*` value
// export from its `dist/index.d.ts` barrel, and fails in BOTH directions: a hook
// published upstream and missing here (the index has gone stale and is lying
// about being complete), and a row here for a hook that no longer exists (an
// author is being pointed at an import that will not resolve).
func TestHookIndexMatchesThePublishedSDK(t *testing.T) {
	if os.Getenv(publishedHooksEnv) != "1" {
		t.Skipf("network guard; set %s=1 to run (no CI job does — this is a manual/scheduled check)", publishedHooksEnv)
	}
	const pkg = "@civitai/blocks-react"
	version, err := FetchNpmLatest(pkg)
	if err != nil {
		t.Skipf("could not resolve %s on npm (%v) — that is a fact about this machine's network, not "+
			"about the index; nothing was verified", pkg, err)
	}
	barrel, err := fetchPackageFile(pkg, version, "package/dist/index.d.ts")
	if err != nil {
		t.Skipf("could not read %s@%s's dist/index.d.ts (%v) — nothing was verified", pkg, version, err)
	}

	var published []string
	for _, m := range dtsHookExportRe.FindAllStringSubmatch(barrel, -1) {
		for _, spec := range strings.Split(m[1], ",") {
			name := strings.TrimSpace(spec)
			if strings.HasPrefix(name, "use") {
				published = append(published, name)
			}
		}
	}
	// Positive control: a barrel whose format changed yields nothing, and an
	// empty published set would report the index perfectly complete.
	if len(published) < 20 {
		t.Fatalf("CONTROL failure, not a finding: extracted %d `use*` export(s) from %s@%s's barrel (%v). "+
			"The parser has stopped matching — this says nothing about the index.",
			len(published), pkg, version, published)
	}

	if diff := setDiff(HookNames(), published); diff != "" {
		t.Errorf("the README hook index and %s@%s disagree: %s\n"+
			"  The index claims to be the COMPLETE export list. Add or remove rows in "+
			"internal/scaffold/hooks.go — a name alone is not enough, each row owes a clause and a scope.",
			pkg, version, diff)
	}
	t.Logf("%s@%s exports %d hook(s); the index carries %d", pkg, version, len(published), len(SDKHooks))
}

// mockedMessageClaimRe reads the message types the README claims `dev:harness`
// mocks, out of the paragraph that makes the claim. It is anchored on the
// sentence so an unrelated `SHOUTY_TOKEN` elsewhere in the README cannot join
// the set being verified.
var (
	mockedClaimAnchor  = "The mock host answers the whole block→host protocol"
	mockedMessageReExp = regexp.MustCompile("`([A-Z][A-Z0-9_]{3,})`")
)

// TestTheMockHostClaimsAreTrueOfThePublishedMockHost checks the bonus claim.
//
// 🔴 WHY THE CLAIM EXISTS AT ALL. The README told an author what `dev:live` does
// NOT support and then stopped, so the measured dogfood run had no way to learn
// whether its ONE required capability — posting — could be exercised for free.
// It read `mockHost.js` and `mockHost.d.ts` to find out, at ~380,000 carried
// tokens. The answer is yes.
//
// 🔴 AND WHY IT IS CHECKED RATHER THAN ASSERTED. A list of capabilities in prose
// is a claim about a package that ships separately and changes without this
// repo: a mock host that drops a handler turns this paragraph into an
// instruction to try something that hangs. The published `createMockHost` is the
// authority, so the test reads it.
func TestTheMockHostClaimsAreTrueOfThePublishedMockHost(t *testing.T) {
	readme := scaffoldedReadme(t)
	i := strings.Index(readme, mockedClaimAnchor)
	if i < 0 {
		t.Fatalf("the scaffolded README no longer carries the `dev:harness` capability claim "+
			"(anchor %q). Either it was removed — drop this guard with it — or it was reworded, in which "+
			"case nothing has been verifying it since.", mockedClaimAnchor)
	}
	section := readme[i:]
	if j := strings.Index(section, "\n### "); j >= 0 {
		section = section[:j]
	}
	var claimed []string
	for _, m := range mockedMessageReExp.FindAllStringSubmatch(section, -1) {
		claimed = append(claimed, m[1])
	}
	if len(claimed) < 4 {
		t.Fatalf("CONTROL failure, not a finding: extracted %d message type(s) from the claim paragraph "+
			"(%v), want >= 4 — the extractor is reading the wrong text", len(claimed), claimed)
	}

	if os.Getenv(publishedHooksEnv) != "1" {
		t.Skipf("the README claims %d mocked message type(s) %v; set %s=1 to verify them against the "+
			"published createMockHost (network)", len(claimed), claimed, publishedHooksEnv)
	}
	const pkg = "@civitai/blocks-react"
	version, err := FetchNpmLatest(pkg)
	if err != nil {
		t.Skipf("could not resolve %s on npm (%v) — nothing was verified", pkg, err)
	}
	mock, err := fetchPackageFile(pkg, version, "package/dist/internal/mockHost.js")
	if err != nil {
		t.Skipf("could not read %s@%s's mock host (%v) — nothing was verified", pkg, version, err)
	}
	// Positive control: the mock host must mention a type we know it handles, or
	// every "not found" below is a fact about the fetch.
	if !strings.Contains(mock, "SUBMIT_WORKFLOW") {
		t.Fatalf("CONTROL failure, not a finding: %s@%s's mock host does not mention SUBMIT_WORKFLOW, "+
			"which is the money path it exists for — the fetched file is not the mock host", pkg, version)
	}
	for _, msg := range claimed {
		if !strings.Contains(mock, "'"+msg+"'") && !strings.Contains(mock, `"`+msg+`"`) {
			t.Errorf("the scaffolded README tells an author that `%s` works in `dev:harness`, and "+
				"%s@%s's createMockHost does not handle it. An unhandled REQUEST-style message does not "+
				"error — it HANGS to the SDK timeout, which reads as the author's own bug.",
				msg, pkg, version)
		}
	}
	t.Logf("verified %d claimed message type(s) against %s@%s", len(claimed), pkg, version)
}

// fetchPackageFile pulls one file out of a published npm tarball.
func fetchPackageFile(pkg, version, name string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	meta := fmt.Sprintf("%s/%s/%s", npmRegistryBase, pkg, version)
	resp, err := client.Get(meta)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", meta, resp.Status)
	}
	var doc struct {
		Dist struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	if doc.Dist.Tarball == "" {
		return "", fmt.Errorf("GET %s: no tarball url", meta)
	}
	tgz, err := client.Get(doc.Dist.Tarball)
	if err != nil {
		return "", err
	}
	defer func() { _ = tgz.Body.Close() }()
	if tgz.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", doc.Dist.Tarball, tgz.Status)
	}
	gz, err := gzip.NewReader(tgz.Body)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%s not found in %s", name, doc.Dist.Tarball)
		}
		if err != nil {
			return "", err
		}
		if h.Name != name {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, 1<<20))
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// setDiff renders the symmetric difference of two name sets, or "" when they
// agree. Both directions are reported because both are real defects and they
// have different fixes.
func setDiff(got, want []string) string {
	in := func(xs []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			m[x] = true
		}
		return m
	}
	g, w := in(got), in(want)
	var missing, extra []string
	for x := range w {
		if !g[x] {
			missing = append(missing, x)
		}
	}
	for x := range g {
		if !w[x] {
			extra = append(extra, x)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	switch {
	case len(missing) == 0 && len(extra) == 0:
		return ""
	default:
		return fmt.Sprintf("missing %v; unexpected %v", missing, extra)
	}
}

func renderPageMoney(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	if _, err := Render(PageMoney, dir, Data{Slug: "hook-index-probe", Name: "Hook Index Probe"}); err != nil {
		t.Fatalf("rendering page-money: %v", err)
	}
	return dir
}

func scaffoldedReadme(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(renderPageMoney(t), "README.md"))
	if err != nil {
		t.Fatalf("read the scaffolded README: %v", err)
	}
	return string(raw)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
