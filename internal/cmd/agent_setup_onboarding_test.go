package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/scaffold"
	"github.com/civitai/cli/internal/ui"
)

// THE 110-MINUTE FALSE BLOCKER, PINNED.
//
// A real operator's App-build session (opencode `ses_f1b9a7d40ffeqlEuYZlMEN0U2L`,
// CLI 0.1.105) reached a SUBMITTED App Block in 45.6 minutes of agent work and
// took 2h17m wall-clock, because the operator spent 110 of those minutes looking
// for a credential nothing in the run needed. Two defects produced it, and both
// are guarded here.
//
//	FIX A  The scaffold ships a one-click wizard that answers "where does the dev
//	       token come from" — `vite-plugin-civitai-setup.ts` + `src/setup-dev-live.ts`
//	       + the "Set up automatically" button in `src/main.tsx` — and NO document
//	       an agent reads mentioned it. Measured in the generated project:
//	       `README.md` names `dev-token` 7 times and the wizard 0 times; the managed
//	       AGENTS.md block named `dev-token` 0 times and had no auth section at all.
//	       So the block now names the wizard, conditionally on the project shipping
//	       it (agent_setup_project.go, setupWizardMarker).
//
//	FIX B  `--check --json` emitted `{"name":"cli-version","ok":true,"detail":"0.1.105"}`
//	       — a presence check wearing a freshness check's name, four versions stale
//	       — and `{"name":"authenticated","ok":true,"detail":"… CIVITAI_TOKEN is NOT
//	       set … orchestration.civitai.com/mcp 401s until you export CIVITAI_TOKEN"}`,
//	       an `ok: true` over a state its own detail calls broken. The hosted setup
//	       prompt tells its reader `"ok" is the verdict — read that, not the
//	       individual rows`, so neither row could be acted on.
//
// 🔴 HOW THIS FILE WAS WATCHED FAIL AT THE MERGE-BASE (6c99704), stated exactly
// because "red at base" is otherwise unfalsifiable. It uses NO new production
// FUNCTION, only three new declarations — `setupWizardMarker`, `checkAgentToken`
// and `appTrackTokenNote` — so the base run was done by dropping a 3-line
// declarations-only shim into `internal/cmd` at that ref and running the file
// unmodified. Every assertion then failed on base BEHAVIOUR rather than on a
// missing symbol: the block carried no wizard paragraph, `cli-version` was
// `{"ok":true,"detail":"<version>"}` for every input including a build four
// releases behind, there was no `agent-token` row at all, and `authenticated` was
// green while its own detail said the variable was unset.
//
// 🔴 THAT PROPERTY COST A DESIGN CHANGE AND IS WORTH KEEPING. An earlier draft added
// a function-shaped network seam (`agentSetupLatestRelease`) and stubbed it in
// TestMain; with that, none of the freshness tests could even be COMPILED at the
// base, and a test nobody can run at the base proves nothing about the defect it
// claims to pin. The only seam used now is `latestReleaseURL`, which already existed
// for `version` and `upgrade`, reached through the `pointAtServer` helper that
// already existed too.

// ---------------------------------------------------------------------------
// FIX A — the managed block surfaces the auto-setup wizard
// ---------------------------------------------------------------------------

// wizardBlockClaims are the substrings the wizard paragraph must carry. They are
// the CLAIMS, not the wording around them: the button label a reader has to find
// on screen, the file the dev server writes, the two env keys the merge replaces,
// and the flag whose absence breaks the headless path.
//
// The endpoint path (`/__civitai/setup-dev-live`) is deliberately NOT here: the
// reader presses a button, and the managed block is paid for on every agent turn in
// the project — a line nobody acts on is not worth its tokens. That the marker file
// really is the one mounting that endpoint is asserted separately, against the
// plugin source.
//
// 🔴 EACH IS ALSO ASSERTED AGAINST THE ARTEFACT IT DESCRIBES, one test down. A
// substring list on its own is a guard on this file's memory of the scaffold; the
// relationship guard is what fails when the scaffold moves.
var wizardBlockClaims = []string{
	"Set up automatically",
	".env.development.local",
	"VITE_LIVE_BLOCK_TOKEN",
	"CIVITAI_HOST_KEY",
	"civitai app dev-token",
	"--spend",
	"--budget",
}

// TestTheManagedBlockNamesTheAutoSetupWizardOnlyWhereItExists is FIX A's guard,
// in both directions at once.
//
// 🔴 THE NEGATIVE HALF IS THE HALF THAT MAKES IT A CONDITIONAL. `page-vite` and
// `static` ship none of the wizard's three files, so a block that named the button
// in them would send that author hunting for a screen their project cannot render
// — the same defect shape item 36 records for npm scripts, which is why this is a
// directory read rather than a fixed paragraph.
func TestTheManagedBlockNamesTheAutoSetupWizardOnlyWhereItExists(t *testing.T) {
	t.Run("page-money ships the wizard and the block says so", func(t *testing.T) {
		dir := scaffoldInto(t, scaffold.PageMoney)

		// PREMISE, read rather than assumed: the marker file this feature keys on
		// really is in this scaffold. If page-money ever stops shipping it, this
		// test is measuring something else and must say so instead of passing.
		if _, err := os.Stat(filepath.Join(dir, setupWizardMarker)); err != nil {
			t.Fatalf("PREMISE BROKEN: the page-money scaffold no longer ships %s (%v), so there is no "+
				"wizard for the block to name and this guard is vacuous", setupWizardMarker, err)
		}

		block := setupInto(t, dir)
		for _, claim := range wizardBlockClaims {
			if !strings.Contains(block, claim) {
				t.Errorf("the managed block for a page-money project does not mention %q.\n\n"+
					"This project ships a one-click wizard that mints the live-dev token and writes "+
					"`.env.development.local` itself, and the measured cost of not saying so is 110 minutes "+
					"of an operator's time. Fix internal/cmd/templates/agents-app.md.\n\nblock:\n%s",
					claim, block)
			}
		}
	})

	for _, tmpl := range []scaffold.Template{scaffold.PageVite, scaffold.Static} {
		t.Run(string(tmpl)+" ships no wizard and the block stays silent", func(t *testing.T) {
			dir := scaffoldInto(t, tmpl)

			// PREMISE: this template really lacks the marker, or the negative below
			// is asserting the absence of text about a wizard that IS there.
			if _, err := os.Stat(filepath.Join(dir, setupWizardMarker)); err == nil {
				t.Fatalf("PREMISE BROKEN: the %s scaffold now ships %s, so this is no longer the "+
					"no-wizard case", tmpl, setupWizardMarker)
			}

			block := setupInto(t, dir)
			// Only the claims that are UNIQUELY the wizard's. `--spend` and
			// `civitai app dev-token` could legitimately appear in other prose one
			// day — a `static` project can mint a dev token too — while the button
			// label and the two env keys the wizard writes cannot.
			for _, claim := range []string{"Set up automatically", "VITE_LIVE_BLOCK_TOKEN", ".env.development.local"} {
				if strings.Contains(block, claim) {
					t.Errorf("the managed block for a %s project mentions %q, and that project ships no "+
						"wizard — the reader is being sent to a screen it cannot render.\n\nblock:\n%s",
						tmpl, claim, block)
				}
			}
		})
	}
}

// devLivePortRe pulls the port out of a `vite --host localhost --port NNNN`
// script body.
var devLivePortRe = regexp.MustCompile(`--port\s+(\d+)`)

// blockIntegerRe matches a WHOLE integer in the managed block — the tokeniser the
// budget assertion needs, because "250" contains "50" and a substring test over the
// two therefore cannot distinguish the default from the cap. A measured mutant
// survived exactly that.
var blockIntegerRe = regexp.MustCompile(`\d+`)

// TestTheWizardParagraphAgreesWithTheArtefactsItDescribes is the RELATIONSHIP
// guard, and it is the one that survives a reword.
//
// 🔴 A SUBSTRING LEDGER PINS THIS FILE'S MEMORY OF THE SCAFFOLD; THIS PINS THE
// SCAFFOLD. The paragraph makes four checkable claims about artefacts that live
// somewhere else — a dev URL, a button label, a refusal string, a numeric default
// — and every one of them can move without touching the template. Each is derived
// from its source here, so the block cannot keep describing a scaffold that has
// changed under it.
func TestTheWizardParagraphAgreesWithTheArtefactsItDescribes(t *testing.T) {
	dir := scaffoldInto(t, scaffold.PageMoney)
	block := setupInto(t, dir)

	t.Run("the dev URL names the port dev:live actually binds", func(t *testing.T) {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		raw := readFile(t, filepath.Join(dir, "package.json"))
		if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
			t.Fatalf("PREMISE BROKEN: the scaffolded package.json does not parse: %v", err)
		}
		script, ok := pkg.Scripts["dev:live"]
		if !ok {
			t.Fatalf("PREMISE BROKEN: the page-money scaffold defines no `dev:live` script, so the "+
				"paragraph's URL describes nothing.\nscripts: %v", pkg.Scripts)
		}
		m := devLivePortRe.FindStringSubmatch(script)
		if m == nil {
			t.Fatalf("PREMISE BROKEN: `dev:live` (%q) names no --port, so this guard cannot derive the "+
				"port the paragraph should quote", script)
		}
		port, err := strconv.Atoi(m[1])
		if err != nil || port <= 0 {
			t.Fatalf("PREMISE BROKEN: derived port %q from %q", m[1], script)
		}
		want := fmt.Sprintf("http://localhost:%d", port)
		if !strings.Contains(block, want) {
			t.Errorf("the block's wizard paragraph does not name %s, which is where `dev:live` "+
				"(%q) actually serves. A URL a reader cannot open is worse than no URL.\n\nblock:\n%s",
				want, script, block)
		}
		// NEGATIVE CONTROL on the derivation: a port the scaffold does NOT bind must
		// be absent, or the assertion above would pass on any URL-shaped string.
		if other := fmt.Sprintf("http://localhost:%d", port+1); strings.Contains(block, other) {
			t.Errorf("the block also names %s, which `dev:live` does not bind — the check above cannot "+
				"distinguish the right port from a neighbour", other)
		}
	})

	t.Run("the button label is the one the scaffold renders", func(t *testing.T) {
		const label = "Set up automatically"
		src := readFile(t, filepath.Join(dir, "src", "main.tsx"))
		if !strings.Contains(src, label) {
			t.Fatalf("PREMISE BROKEN: the scaffolded src/main.tsx does not render a %q button, so the "+
				"block is telling the reader to press something that is not there", label)
		}
		if !strings.Contains(block, label) {
			t.Errorf("the block does not name the %q button that src/main.tsx renders", label)
		}
	})

	t.Run("the endpoint is the one the plugin mounts", func(t *testing.T) {
		plugin := readFile(t, filepath.Join(dir, setupWizardMarker))
		const endpoint = "setup-dev-live"
		if !strings.Contains(plugin, endpoint) {
			t.Fatalf("PREMISE BROKEN: %s does not mention %q, so it is not the file that mounts the "+
				"wizard endpoint and setupWizardMarker is keyed on the wrong artefact",
				setupWizardMarker, endpoint)
		}
	})

	t.Run("the refusal string is the CLI's own", func(t *testing.T) {
		// 🔴 DERIVED FROM `budgetedScope`, NOT TYPED OUT. `spendFilteredNotice` is
		// the CLI's own warning for exactly this mistake and it says `npm run
		// dev:live` "will refuse to generate with \"block lacks <scope> scope\"".
		// Quoting the same string means an operator can grep what they saw; deriving
		// it means a scope rename cannot leave the block quoting a dead message.
		want := "block lacks " + budgetedScope + " scope"
		notice := spendFilteredNotice(ui.For(io.Discard), "demo-app")
		if !strings.Contains(notice, want) {
			t.Fatalf("PREMISE BROKEN: spendFilteredNotice no longer says %q, so the block is quoting a "+
				"refusal this CLI does not predict.\nnotice: %s", want, notice)
		}
		if !strings.Contains(block, want) {
			t.Errorf("the block does not quote %q — the string an operator who skips `--spend` will "+
				"actually see.\n\nblock:\n%s", want, block)
		}
	})

	t.Run("the budget default is the real one", func(t *testing.T) {
		// 🔴 WHOLE-NUMBER TOKENS, NOT `strings.Contains`. A mutation sweep SURVIVED
		// the substring form: with the default written as 50 and the cap as 250,
		// `strings.Contains(block, "50")` is satisfied by the "50" inside "250", so
		// changing the block's default from 50 to 40 passed a guard whose whole job
		// is to catch that. The assertion has to be over the numbers the block
		// actually states, so it is over a tokenised set.
		nums := map[string]bool{}
		for _, tok := range blockIntegerRe.FindAllString(block, -1) {
			nums[tok] = true
		}
		// Positive control on the tokeniser: the block certainly states SOME numbers,
		// and an empty set would make every membership test below pass or fail for
		// reasons unrelated to the budget.
		if len(nums) < 2 {
			t.Fatalf("the tokeniser found %d integer(s) in the block, so the membership tests below "+
				"are not about the numbers it states: %v", len(nums), nums)
		}
		for label, n := range map[string]int{
			"default": appapi.DevBuzzBudgetDefault,
			"minimum": appapi.DevBuzzBudgetMin,
			"cap":     appapi.DevBuzzBudgetCap,
		} {
			if !nums[strconv.Itoa(n)] {
				t.Errorf("the block does not state %d as the per-generation budget %s "+
					"(appapi.DevBuzzBudget* is the source). That number is the SERVER's, not the "+
					"manifest's, and getting it wrong is a refused generation.\n  integers in the "+
					"block: %v", n, label, nums)
			}
		}
		// MECHANICAL NEGATIVE CONTROL: a value the constant cannot equal must be
		// absent, so the membership test above is distinguishing values rather than
		// matching anything number-shaped.
		if off := strconv.Itoa(appapi.DevBuzzBudgetDefault + 1); nums[off] {
			t.Errorf("the block states %s, one more than the real default — the membership test "+
				"cannot tell the right number from a neighbour", off)
		}
	})
}

// ---------------------------------------------------------------------------
// FIX B1 — `cli-version` is a freshness check
// ---------------------------------------------------------------------------

// releaseServer serves a GitHub-shaped `releases/latest` body carrying tag, and
// counts the requests it received so a test can prove the row actually consulted
// it rather than guessing.
func releaseServer(t *testing.T, tag string) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// pinVersion sets the running version for one test and restores it.
func pinVersion(t *testing.T, v string) {
	t.Helper()
	orig := version
	version = v
	t.Cleanup(func() { version = orig })
}

// checkRows runs `agent-setup --check --json` against a configured project and
// returns the decoded rows plus the process-level error.
func checkRows(t *testing.T, dir, agent string) ([]agentCheckJSON, bool, error) {
	t.Helper()
	stdout, stderr, err := run(t, "agent-setup", "--dir", dir, "--agent", agent, "--check", "--json")
	var payload agentSetupJSON
	if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
		t.Fatalf("`--check --json` stdout does not decode: %v\nstdout:\n%s\nstderr:\n%s", jerr, stdout, stderr)
	}
	// Positive control: a payload with no rows makes every lookup below vacuous.
	if len(payload.Checks) == 0 {
		t.Fatalf("`--check --json` returned no rows:\n%s", stdout)
	}
	return payload.Checks, payload.OK, err
}

func rowNamed(t *testing.T, rows []agentCheckJSON, name string) agentCheckJSON {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
	}
	t.Fatalf("no %q row in the payload; got %v", name, names)
	return agentCheckJSON{}
}

// TestCLIVersionRowIsFalseOnlyWhenStale enumerates every arm of the freshness
// check through the REAL command.
//
// 🔴 THE STALE ARM IS THE NEGATIVE CONTROL, AND IT IS THE WHOLE POINT. A freshness
// check nobody has watched go red is a claim about a command line: before this
// change the row was `{"ok":true,"detail":"<version>"}` for every input, which is
// green on a build four releases behind. So the first subtest pins a version BELOW
// what the endpoint publishes and requires `ok: false`, a detail naming the newer
// tag, and a non-zero exit.
//
// 🔴 AND THE FOUR REMAINING ARMS ARE WHAT STOPS THE FIX BEING "return false". Equal,
// ahead, unreachable and opted-out must ALL be `ok: true` — the fail-soft contract
// this repo has been burned on twice, once when `pins-vs-published` froze every open
// PR and again on npm 10's arborist crash. A gate that reddens because a network
// went away trains everyone to click through it.
func TestCLIVersionRowIsFalseOnlyWhenStale(t *testing.T) {
	t.Run("stale is ok:false and names the newer tag, WITHOUT failing the verdict", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: the setup run itself failed: %v", err)
		}
		srv, hits := releaseServer(t, "v9.9.9")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v0.1.105")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, ok, err := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if row.OK {
			t.Errorf("the `cli-version` row is ok:true on a build four releases behind the published "+
				"one. That is the measured defect: a presence check wearing a freshness check's "+
				"name.\n  detail: %s", row.Detail)
		}
		if !strings.Contains(row.Detail, "v9.9.9") {
			t.Errorf("the detail does not name the newer published tag, so a reader cannot tell what "+
				"to upgrade TO.\n  detail: %s", row.Detail)
		}
		if !strings.Contains(row.Detail, "civitai upgrade") {
			t.Errorf("the detail names no remedy — `civitai upgrade` is the one command that fixes "+
				"it.\n  detail: %s", row.Detail)
		}
		// 🔴 AND IT IS VERDICT-EXEMPT, LIKE THE TWO AUTH ROWS. A counted freshness row
		// on a repo that cuts releases days apart is red for a large share of ordinary
		// runs over a usually-harmless state — a permanently-red gate people learn to
		// click through — and because the hosted prompt instructs its reader to read
		// `ok` and NOT the rows, counting it does not produce an ignored warning, it
		// produces an agent that concludes the setup is broken and starts repairing
		// something that was fine.
		//
		// ⚠ THE COST OF THAT CHOICE IS ASSERTED HERE RATHER THAN LEFT IMPLICIT: the
		// two lines below are what make "a consumer reading only `ok` cannot see a
		// stale CLI" a pinned property of this command instead of a remark. Closing
		// that gap needs a change to the hosted prompt, not to this repo.
		if !ok {
			t.Error("`ok` is false with only `cli-version` failing. That makes the published verdict " +
				"red for every user a few days behind a release, which is the permanently-red gate " +
				"shape — and the hosted prompt reads `ok`, so it would report a BROKEN setup for one " +
				"that works.")
		}
		if err != nil {
			t.Errorf("`--check` exited non-zero with only the exempt `cli-version` row failing: %v", err)
		}
		// POSITIVE CONTROL: the row really did consult the endpoint. Without this a
		// row that is red for some unrelated reason passes every assertion above.
		if *hits == 0 {
			t.Error("the release endpoint was never requested, so the red verdict above is not about " +
				"freshness at all")
		}
	})

	// 🔴 THE ANTI-`return true` CONTROL FOR THE EXEMPTION ABOVE. "A stale row leaves
	// `ok` true" is individually satisfiable by a verdict that is always true, so the
	// same run must still go red on a row that DOES count. Without this pair the
	// exemption is indistinguishable from having broken the verdict.
	t.Run("a stale CLI beside a REAL failure still fails the verdict", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		// Deliberately NOT set up: `agents-md` is missing, which counts.
		srv, _ := releaseServer(t, "v9.9.9")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v0.1.105")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, ok, err := checkRows(t, dir, agentClaude)
		// PREMISE: both conditions really hold in this run.
		if rowNamed(t, rows, checkCLIVersion).OK {
			t.Fatal("PREMISE BROKEN: `cli-version` is green, so this is not the stale case")
		}
		if rowNamed(t, rows, checkAgentsMD).OK {
			t.Fatal("PREMISE BROKEN: `agents-md` is green, so there is no counting failure to observe")
		}
		if ok || err == nil {
			t.Errorf("a missing AGENTS.md no longer fails the verdict (ok=%v, err=%v) — exempting "+
				"`cli-version` has broken the verdict rather than narrowed it", ok, err)
		}
	})

	// The human surface has to agree with the verdict, and it is derived from the
	// SAME predicate — a stale row rendered like a failure tells a reader the run
	// failed on a row that deliberately cannot fail it.
	t.Run("the terminal report warns rather than errors, and names the gap", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		srv, _ := releaseServer(t, "v9.9.9")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v0.1.105")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		stdout, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude, "--check")
		if err != nil {
			t.Fatalf("`--check` exited non-zero on a stale-but-otherwise-complete setup: %v\n%s", err, stdout)
		}
		if !strings.Contains(stdout, "Setup is complete.") {
			t.Errorf("the report does not call a stale-but-complete setup complete, so the screen "+
				"disagrees with `ok`:\n%s", stdout)
		}
		// 🔴 THE RESIDUAL, PINNED AS TEXT. The exemption means a consumer reading only
		// `ok` cannot see a stale CLI. That is a real gap this PR does not close, and
		// an unstated gap is one the next reader has to rediscover.
		if !strings.Contains(stdout, "reads only `ok` will not see it") {
			t.Errorf("the report does not say that a consumer reading only `ok` misses this row. "+
				"The exemption is deliberate; leaving its cost unstated is not.\n%s", stdout)
		}
	})

	t.Run("equal is ok:true", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		srv, hits := releaseServer(t, "v1.2.3")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v1.2.3")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, _, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if !row.OK {
			t.Errorf("the row is red on a build that IS the latest release: %s", row.Detail)
		}
		if *hits == 0 {
			t.Error("the endpoint was never requested, so this green is not a comparison")
		}
	})

	t.Run("ahead of the latest release is ok:true", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		// The measured real-world case: GitHub's /releases/latest excludes drafts,
		// so a machine running a just-cut release compares as ahead.
		srv, _ := releaseServer(t, "v1.2.3")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v1.3.0")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, _, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if !row.OK {
			t.Errorf("a build AHEAD of the latest published release is not stale, and this row calls "+
				"it a failure: %s", row.Detail)
		}
	})

	t.Run("an unreachable endpoint is ok:true and says so", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		// A server that is closed before the run: a guaranteed transport failure at
		// a real address, with no timeout to wait out.
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		deadURL := dead.URL
		dead.Close()
		pointAtServer(t, deadURL)
		pinVersion(t, "v0.1.105")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, ok, err := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if !row.OK {
			t.Errorf("an unreachable release endpoint turned the row RED. This is the shape that froze "+
				"every open PR when `pins-vs-published` went live: a gate that reddens because a "+
				"network went away.\n  detail: %s", row.Detail)
		}
		if !strings.Contains(row.Detail, "could not check") {
			t.Errorf("the detail does not say the comparison did not happen, so a green row is "+
				"indistinguishable from a verified-current one.\n  detail: %s", row.Detail)
		}
		if !ok || err != nil {
			t.Errorf("an offline machine cannot clear `--check` (ok=%v, err=%v)", ok, err)
		}
	})

	t.Run("opted out is ok:true and names the control", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		// A LIVE server publishing a much newer tag: if the opt-out were ignored,
		// this arm would go red, so the green below is about the opt-out and not
		// about an absent endpoint.
		srv, hits := releaseServer(t, "v9.9.9")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v0.1.105")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")

		rows, ok, err := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if !row.OK || !ok || err != nil {
			t.Errorf("CIVITAI_NO_UPDATE_CHECK did not suppress the lookup (row ok=%v, verdict=%v, "+
				"err=%v)\n  detail: %s", row.OK, ok, err, row.Detail)
		}
		if *hits != 0 {
			t.Errorf("the endpoint was requested %d time(s) despite CIVITAI_NO_UPDATE_CHECK=1 — the "+
				"opt-out is documented and does not work", *hits)
		}
		if !strings.Contains(row.Detail, "CIVITAI_NO_UPDATE_CHECK") {
			t.Errorf("the detail does not say WHY it could not check, so a permanently-disabled "+
				"machine reads as an offline one.\n  detail: %s", row.Detail)
		}
	})

	t.Run("a dev build is ok:true and does not claim to be current", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		srv, _ := releaseServer(t, "v1.2.3")
		pointAtServer(t, srv.URL)
		pinVersion(t, "dev")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, _, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if !row.OK {
			t.Errorf("a `dev` build is not a stale release: %s", row.Detail)
		}
		if strings.Contains(row.Detail, "the latest published release") &&
			!strings.Contains(row.Detail, "unknown") {
			t.Errorf("the detail claims a `dev` build is current, which the comparison cannot "+
				"establish.\n  detail: %s", row.Detail)
		}
	})

	t.Run("an unparseable published tag cannot make the row red", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		srv, _ := releaseServer(t, "nightly-2026-09-27")
		pointAtServer(t, srv.URL)
		pinVersion(t, "v0.1.105")
		t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

		rows, ok, err := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkCLIVersion)
		if !row.OK || !ok || err != nil {
			t.Errorf("a tag this CLI cannot parse turned the row red. The endpoint is not ours to "+
				"control, and an unexpected tag format must not break a user's setup check.\n  detail: %s",
				row.Detail)
		}
	})
}

// TestAgentSetupCheckReadsTheRealReleaseEndpoint is the SEAM-WIRING control.
//
// 🔴 TestMain POINTS `latestReleaseURL` AT A DEAD ADDRESS FOR THE WHOLE PACKAGE, so
// every other `--check` test exercises the fail-soft arm — which is what keeps the
// suite hermetic and is also exactly how a feature that was never wired up would
// look. This test closes that: it serves a tag from a live local server and requires
// the row to quote THAT tag, which is only possible if `resolveVersionFreshness`
// really does read `latestReleaseURL` through `fetchLatestRelease` on the `--check`
// path.
func TestAgentSetupCheckReadsTheRealReleaseEndpoint(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("PREMISE BROKEN: setup: %v", err)
	}
	const tag = "v4.5.6"
	srv, hits := releaseServer(t, tag)
	pointAtServer(t, srv.URL)
	pinVersion(t, tag)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

	rows, _, _ := checkRows(t, dir, agentClaude)
	row := rowNamed(t, rows, checkCLIVersion)
	if *hits != 1 {
		t.Errorf("the release endpoint was requested %d time(s), want exactly 1. `--check` must make "+
			"ONE bounded call, not zero (the feature is unwired) and not several (a per-row fetch)", *hits)
	}
	if !strings.Contains(row.Detail, "the latest published release") {
		t.Errorf("the row did not resolve the served tag %q — the production path is not reading "+
			"latestReleaseURL.\n  detail: %s", tag, row.Detail)
	}
}

// TestAWriteRunStillContactsNothing pins the OTHER half of the network decision.
//
// 🔴 THE LOOKUP IS ON `--check` ONLY, AND THAT IS A CHOICE WORTH GUARDING. A write
// run has no verdict to be stale about, and it is the first thing a new user runs;
// adding a round-trip there would slow the scaffolding path for no report. A live
// server is used rather than an absent one, so a request would be COUNTED rather
// than merely failing.
func TestAWriteRunStillContactsNothing(t *testing.T) {
	dir, _ := agentSetupProject(t)
	srv, hits := releaseServer(t, "v9.9.9")
	pointAtServer(t, srv.URL)
	pinVersion(t, "v0.1.105")
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if *hits != 0 {
		t.Errorf("a write run requested the release endpoint %d time(s); it must contact nothing", *hits)
	}
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude, "--dry-run"); err != nil {
		t.Fatalf("agent-setup --dry-run: %v", err)
	}
	if *hits != 0 {
		t.Errorf("a --dry-run requested the release endpoint %d time(s); it must contact nothing", *hits)
	}
}

// ---------------------------------------------------------------------------
// FIX B2 — the two auth rows, and the sentence that was missing
// ---------------------------------------------------------------------------

// writeCLIConfigToken puts a credential in THIS CLI's own store and nowhere else:
// `~/.config/civitai/config.yaml` under `home`, with CIVITAI_TOKEN left unset by
// the caller.
//
// 🔴 THAT COMBINATION IS THE MEASURED STATE, AND IT IS THE ONLY ONE IN WHICH THE
// DEFECT IS REACHABLE. `config.Load` merges the file with CIVITAI_TOKEN, so
// `t.Setenv("CIVITAI_TOKEN", …)` — what every neighbouring test uses — satisfies
// `cfg.Token() != ""` AND `tokenIsExported(env)`, i.e. the arm where both auth rows
// are legitimately green. A test built that way cannot see the contradiction; it
// has to be the file alone.
func writeCLIConfigToken(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, ".config", "civitai", "config.yaml")
	writeFile(t, path, "access_token: at-fixture\nrefresh_token: rt-fixture\nauth_kind: oauth\n")
	// PREMISE, asserted rather than assumed: the config this process will read must
	// be the one just written. XDG_CONFIG_HOME is set by agentSetupProject, and a
	// mismatch here would leave every caller exercising the no-credential arm under
	// the has-credential arm's name.
	if got := os.Getenv("XDG_CONFIG_HOME"); got != filepath.Join(home, ".config") {
		t.Fatalf("PREMISE BROKEN: XDG_CONFIG_HOME is %q, so %s is not the config this run reads",
			got, path)
	}
}

// TestNoGreenRowSaysTheTokenIsUnset is FIX B2's structural guard.
//
// 🔴 IT ASSERTS THE STATE, NOT A ROW NAME. The measured defect was
// `{"name":"authenticated","ok":true,"detail":"… but CIVITAI_TOKEN is NOT set in
// this environment — … orchestration.civitai.com/mcp 401s until you export
// CIVITAI_TOKEN"}`: a verdict contradicting its own text. Pinning it as "the
// `authenticated` row must not say that" would be walkable by moving the sentence
// to any other row, which is precisely how a guard gets satisfied by a rename. So
// the rule is over EVERY row: no row may report `ok: true` while its own detail
// says the variable is not set.
//
// 🔴 THE FIXTURE IS THE MEASURED STATE, NOT A CONVENIENT ONE: a token in this CLI's
// config and CIVITAI_TOKEN absent from the environment. That is what the operator's
// machine looked like, and it is the only state in which the contradiction is
// reachable — with the variable exported both rows are legitimately green.
func TestNoGreenRowSaysTheTokenIsUnset(t *testing.T) {
	dir, home := agentSetupProject(t)
	// `civitai login`'s store, without the export: cfg.Token() is non-empty and
	// tokenIsExported(env) is false.
	writeCLIConfigToken(t, home)
	t.Setenv(tokenEnvVar, "")
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("PREMISE BROKEN: setup: %v", err)
	}

	rows, ok, err := checkRows(t, dir, agentClaude)

	// PREMISE: this really is the has-a-token-but-unexported state. Without it the
	// scan below passes on a payload where nothing mentions the variable at all.
	auth := rowNamed(t, rows, checkAuthenticated)
	if !auth.OK {
		t.Fatalf("PREMISE BROKEN: `authenticated` is red, so this run has no stored credential and "+
			"the contradiction under test is unreachable.\n  detail: %s", auth.Detail)
	}

	needle := tokenEnvVar + " is NOT set"
	var sawIt bool
	for _, r := range rows {
		if !strings.Contains(r.Detail, needle) {
			continue
		}
		sawIt = true
		if r.OK {
			t.Errorf("row %q reports ok:true while its own detail says %q. A consumer reading the "+
				"verdict — which developer.civitai.com's prompt is instructed to do — is told this "+
				"setup is finished.\n  detail: %s", r.Name, needle, r.Detail)
		}
	}
	// POSITIVE CONTROL: some row must actually say it, or the loop above never ran
	// and its clean verdict is a fact about an empty iteration.
	if !sawIt {
		t.Errorf("no row mentions %q on a machine where it genuinely is not set — the unexported "+
			"token is now invisible, which is a different defect with the same symptom", needle)
	}

	// And the split must not have made a correct, unexported setup fail: the agent
	// half is `authenticated`'s sibling, exempt for the same reason.
	if !ok || err != nil {
		t.Errorf("a complete setup awaiting only an export now fails `--check` (ok=%v, err=%v). "+
			"That is the exit-1-forever shape decision record 35 removed.", ok, err)
	}
}

// TestTheAgentTokenRowTracksWhetherAReferenceWasWritten pins the condition the
// row branches on.
//
// 🔴 IT IS `false` ONLY WHERE A REFERENCE EXISTS TO RESOLVE. For Zed this command
// writes no environment reference at all, so an unset variable breaks nothing it
// produced — a red row there would be permanently red for a correct setup, the
// gate-nobody-can-clear shape this file's neighbours already record twice. The
// condition is read from `agentTargets`, so a vendor that gains an env spelling is
// covered the moment the table changes.
func TestTheAgentTokenRowTracksWhetherAReferenceWasWritten(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		target := agentTargets[agent]
		writesReference := target.EnvHeaderSyntax != "" || target.EnvBearerKey != ""
		t.Run(agent, func(t *testing.T) {
			dir, home := agentSetupProject(t)
			writeCLIConfigToken(t, home)
			t.Setenv(tokenEnvVar, "")
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("PREMISE BROKEN: setup for %s: %v", agent, err)
			}
			rows, _, _ := checkRows(t, dir, agent)
			row := rowNamed(t, rows, checkAgentToken)
			if writesReference && row.OK {
				t.Errorf("%s's MCP entries reference %s and it is unset, yet the row is green: %s",
					agent, tokenEnvVar, row.Detail)
			}
			if !writesReference && !row.OK {
				t.Errorf("%s gets no %s reference written (it documents no environment spelling), so "+
					"an unset variable breaks nothing — a red row here is permanently red for a "+
					"correct setup: %s", agent, tokenEnvVar, row.Detail)
			}
		})
	}

	t.Run("exported turns the row green", func(t *testing.T) {
		dir, _ := agentSetupProject(t)
		t.Setenv(tokenEnvVar, "civitai-test-token")
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		rows, _, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkAgentToken)
		if !row.OK {
			t.Errorf("%s IS exported and the row is still red — the check cannot see the only state "+
				"that satisfies it: %s", tokenEnvVar, row.Detail)
		}
	})
}

// TestTheAppTrackNeedsNoTokenSentenceReachesBothSurfaces is the 110 minutes,
// stated as a test.
//
// 🔴 IT MUST LAND WHERE THE READER IS STANDING, NOT IN A DOC THEY WOULD HAVE TO GO
// FIND. `prompt.md` §5 requires an agent to relay the `Next:` block VERBATIM and
// forbids it from explaining the authentication itself, so a caveat that is not in
// that block is not said to the operator at all — which is how "export
// CIVITAI_TOKEN" read as a prerequisite for a task that never needed it. So the
// sentence is required in the `Next:` block AND in the `--check --json` payload,
// which is what a script and the hosted prompt read.
//
// 🔴 PINNED AS A WHOLE DERIVED STRING. `appTrackTokenNote` is one constant and both
// surfaces print it, so this cannot pass on two surfaces that say different things,
// and a reword has to keep it in both places.
func TestTheAppTrackNeedsNoTokenSentenceReachesBothSurfaces(t *testing.T) {
	// The claim the note has to make, independent of its wording: that the App
	// commands do not need the variable. Checked first, so a note reworded into
	// something true-but-silent fails here rather than passing the two Contains
	// below.
	for _, must := range []string{tokenEnvVar, "civitai app submit", "MCP"} {
		if !strings.Contains(appTrackTokenNote, must) {
			t.Fatalf("appTrackTokenNote no longer mentions %q, so it cannot be the sentence that "+
				"separates the App track from the MCP servers.\n  note: %s", must, appTrackTokenNote)
		}
	}

	t.Run("the Next: block", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		writeCLIConfigToken(t, home)
		t.Setenv(tokenEnvVar, "")
		stdout, stderr, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude)
		if err != nil {
			t.Fatalf("agent-setup: %v\n%s\n%s", err, stdout, stderr)
		}
		// PREMISE: this run really did print the export step, or the assertion below
		// is about a block that has nothing to qualify.
		if !strings.Contains(stdout, "export "+tokenEnvVar) {
			t.Fatalf("PREMISE BROKEN: the run printed no `export %s` step, so there is no instruction "+
				"for the note to qualify:\n%s", tokenEnvVar, stdout)
		}
		if !strings.Contains(stdout, appTrackTokenNote) {
			t.Errorf("the next-step block tells the operator to export %s and never says the App track "+
				"does not need it. That omission cost a measured 110 minutes: the operator went looking "+
				"for a credential, found no command that prints one (correctly), and came back two hours "+
				"later — after an agent had already scaffolded, built, tested and SUBMITTED the App "+
				"without it.\n\nwant to find: %s\n\ngot:\n%s", tokenEnvVar, appTrackTokenNote, stdout)
		}
		// And it must come BEFORE the mint instruction: that note starts a task, so
		// a caveat under it is read after the reader has left to do it.
		if i, j := strings.Index(stdout, appTrackTokenNote), strings.Index(stdout, accountAPIKeysURL); i >= 0 && j >= 0 && i > j {
			t.Errorf("the note appears AFTER the \"mint one at %s\" line. The reader leaves to create "+
				"a key at that point, so the qualifier arrives too late to prevent the detour.",
				accountAPIKeysURL)
		}
	})

	t.Run("the --check payload", func(t *testing.T) {
		dir, home := agentSetupProject(t)
		writeCLIConfigToken(t, home)
		t.Setenv(tokenEnvVar, "")
		if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
			t.Fatalf("PREMISE BROKEN: setup: %v", err)
		}
		rows, _, _ := checkRows(t, dir, agentClaude)
		row := rowNamed(t, rows, checkAgentToken)
		if !strings.Contains(row.Detail, appTrackTokenNote) {
			t.Errorf("the `%s` row's detail does not carry the note, so a `--json` consumer and the "+
				"hosted prompt see the demand without its scope.\n  detail: %s", checkAgentToken, row.Detail)
		}
	})
}
