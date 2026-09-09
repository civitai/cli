package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/config"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/ui"
)

// The regression guards for the SECOND blind dogfood's two findings:
//
//   - the managed block's "Scaffold a new app" row named `civitai app init`,
//     the no-build `static` default, while everything around it — its own
//     npm-script table, its lockfile rule, its `@civitai/*` pin rule — describes
//     a page-money project, and `civitai --help`'s "Get started" block names
//     `civitai app create`. Measured by the dogfood: `app init dogfood2-hello`
//     produced an 8-file static app with NO package.json; `app create
//     dogfood2-cmp` produced 37 files, page-money, with `dev:harness`,
//     `dev:tunnel` and `dev:live` all present.
//
//   - `--check --json` returned `ok: true` for `mcp-orch` with the config path
//     as its whole `detail`, against a server the dogfood measured returning
//     401 to an unauthenticated `initialize`.

// ---------------------------------------------------------------------------
// Defect 1 — the block's scaffolder
// ---------------------------------------------------------------------------

// blockScaffoldRowRe reads the "Scaffold a new app" row out of the SHIPPED
// document rather than out of a literal here. A guard that hardcodes the command
// it expects re-encodes the assumption that produced the bug; this one asks the
// template what it tells an author to run, and then goes and runs it.
var blockScaffoldRowRe = regexp.MustCompile("(?m)^\\|\\s*Scaffold a new app\\s*\\|\\s*`civitai ([^`]+)`")

// blockScaffoldCommand returns the `civitai …` command path the block's scaffold
// row names, with placeholder operands (`<name>`) and flags dropped.
func blockScaffoldCommand(doc string) []string {
	m := blockScaffoldRowRe.FindStringSubmatch(doc)
	if m == nil {
		return nil
	}
	var path []string
	for _, f := range strings.Fields(m[1]) {
		if strings.HasPrefix(f, "<") || strings.HasPrefix(f, "[") || strings.HasPrefix(f, "-") {
			continue
		}
		path = append(path, f)
	}
	return path
}

// TestTheBlocksScaffolderProducesAProjectItsOwnCommandsRunIn is THE regression
// guard for defect 1, and it is a RELATIONSHIP rather than a word check: it does
// not assert that the row says "create". It runs whatever the row says, exactly
// as the row spells it (no `--template`), and then requires that the block
// `civitai agent-setup` writes into the result can actually name commands that
// run there.
//
// 🔴 WATCHED FAIL ON origin/main (cbcb992), where the row read
// `civitai app init <name>`:
//
//	agent_setup_scaffolder_test.go:  the block's "Scaffold a new app" row names
//	`civitai app init`, whose default template produced a "no-build" project
//	with 0 recognised dev scripts …
//
// because `app init` with no flags is the no-build `static` template. That is
// the whole defect: the block's Local-development branch credits `civitai app
// create` with `dev:harness` / `dev` / `dev:live` / `dev:tunnel`, and the
// scaffolder the block named produces a project that defines none of them and
// has no package.json to define them in.
//
// 🔴 WHAT IT DOES NOT CLAIM. It says nothing about `app init`, which is a
// supported command with a documented purpose ("the same scaffolder with a
// no-build static default (back-compat alias)"). The claim is only that the
// command the block hands a NEW author must produce a project the block's own
// following sections describe.
func TestTheBlocksScaffolderProducesAProjectItsOwnCommandsRunIn(t *testing.T) {
	path := blockScaffoldCommand(agentsAppTemplate)
	if len(path) == 0 {
		t.Fatal("no `Scaffold a new app` row found in the embedded template — the extractor is reading the " +
			"wrong text, and every assertion below would be vacuous")
	}
	spelling := "civitai " + strings.Join(path, " ")

	// It has to be a real command before it can be a right one. (The walker in
	// agent_setup_test.go covers every command in the block; this is the same
	// check on the one command this test is about to execute.)
	c, _, err := NewRootCmd().Find(path)
	if err != nil || c.Name() != path[len(path)-1] {
		t.Fatalf("the block's scaffold row names `%s`, which does not resolve in the command tree (%v)", spelling, err)
	}

	// Run it the way the row spells it: a name, and NO --template. --dir keeps it
	// out of the working directory; --yes is only there because the row is meant
	// to be runnable in a script too.
	dir := filepath.Join(t.TempDir(), "scaffolded")
	args := append(append([]string{}, path...), "demo-app", "--dir", dir, "--yes")
	if out, errOut, err := run(t, args...); err != nil {
		t.Fatalf("`%s demo-app` failed: %v\n%s\n%s", spelling, err, out, errOut)
	}

	// 🔴 THE DISCRIMINATING ASSERTION. The block's Local-development section is
	// rendered from this directory, and its npm branch names `%s` as the
	// authority for what those script names mean. A scaffolder whose default
	// output has no package.json cannot be that authority.
	shape := detectProjectShape(dir)
	if shape.Kind != projectNPM || len(shape.Scripts) == 0 {
		t.Fatalf("the block's \"Scaffold a new app\" row names `%s`, whose default template produced a %q "+
			"project with %d recognised dev scripts — so an author who follows that row gets a project in "+
			"which the block's own Local-development table can name nothing, while the block credits that "+
			"same command with `dev:harness`/`dev`/`dev:live`/`dev:tunnel` and with the `@civitai/*` pins. "+
			"Name the scaffolder whose output the rest of the block describes.",
			spelling, shape.Kind, len(shape.Scripts))
	}

	// The "can run" half proper: every command the block prints for THIS project
	// must exist in it.
	block := setupInto(t, dir)
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "package.json"))), &pkg); err != nil {
		t.Fatalf("the scaffolded package.json does not parse: %v", err)
	}
	named := regexp.MustCompile("`npm run ([a-z0-9:._-]+)`").FindAllStringSubmatch(block, -1)
	if len(named) == 0 {
		t.Fatalf("the block written into a `%s` project names no `npm run` script at all — the assertion "+
			"below observes nothing:\n%s", spelling, block)
	}
	for _, m := range named {
		if _, ok := pkg.Scripts[m[1]]; !ok {
			t.Errorf("the block tells a `%s` project to run `npm run %s`, which that project does not define "+
				"(it defines %v)", spelling, m[1], scriptNames(pkg.Scripts))
		}
	}
}

// TestBlockScaffoldRowExtractorCanFail is the NEGATIVE CONTROL for the extractor
// above. An extractor that returns a constant — or that silently returns nothing
// and lets a `len(path) == 0` guard be the only thing standing — would make the
// test above a fact about the extractor rather than about the template.
func TestBlockScaffoldRowExtractorCanFail(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"create", "| Task | Command |\n| Scaffold a new app | `civitai app create <name>` |\n", "app create"},
		{"init", "| Scaffold a new app | `civitai app init <name>` |\n", "app init"},
		{"flags dropped", "| Scaffold a new app | `civitai app create <name> --yes` |\n", "app create"},
		{"a command that does not exist", "| Scaffold a new app | `civitai app frobnicate <name>` |\n", "app frobnicate"},
		{"no such row", "| Check the manifest | `civitai app validate` |\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(blockScaffoldCommand(tc.doc), " "); got != tc.want {
				t.Errorf("blockScaffoldCommand(%q) = %q, want %q", tc.doc, got, tc.want)
			}
		})
	}
	// And the row it extracts must be resolvable-or-not on its own merits: the
	// control command must NOT resolve, or the resolution check in the guard
	// above accepts anything.
	c, _, err := NewRootCmd().Find(blockScaffoldCommand("| Scaffold a new app | `civitai app frobnicate` |\n"))
	if err == nil && c.Name() == "frobnicate" {
		t.Error("`civitai app frobnicate` resolved — the resolution check cannot reject a command that does not exist")
	}
}

// newAppRemedySurfaces is the ENUMERATED ledger of the places this CLI tells
// somebody who has NO app which command creates one.
//
// 🔴 IT IS AN ENUMERATION AND THE ENUMERATION IS OPEN. This is not a claim that
// no other string in the repo names a scaffolder — it cannot be, because plenty
// legitimately do: `civitai app --help` documents BOTH commands on purpose (that
// is where the alias is defined), the README's command table has a row for each,
// and `readyAckRemedy` names `civitai app init` deliberately, because it needs
// the `static` template's `civitai-host.js` and `app create`'s page-money
// default does not ship one. The claim is only that these SEVEN surfaces, each
// of which addresses a reader with no project, agree on one answer. A new
// surface of that kind has to be added here by hand.
func newAppRemedySurfaces(t *testing.T) map[string]string {
	t.Helper()
	root := NewRootCmd()
	appCmd, _, err := root.Find([]string{"app"})
	if err != nil {
		t.Fatalf("app command: %v", err)
	}
	_, manifestErr := manifest.Load(t.TempDir())
	if manifestErr == nil {
		t.Fatal("manifest.Load on an empty directory returned no error — its remedy cannot be measured")
	}
	return map[string]string{
		"civitai --help (Long: Get started)":       root.Long,
		"civitai --help (Example)":                 root.Example,
		"civitai app --help (Example)":             appCmd.Example,
		"agent-setup's managed block scaffold row": strings.Join(blockScaffoldCommand(agentsAppTemplate), " "),
		"agent-setup --dir remedy":                 agentSetupNoSuchDir,
		"project-path remedy (remedyNoSuchDir)":    remedyNoSuchDir,
		"manifest.Load remedy":                     manifestErr.Error(),
	}
}

var scaffolderRe = regexp.MustCompile(`\bapp (create|init)\b`)

// TestEveryNewAppRemedyNamesOneScaffolder is the cross-surface half of defect 1.
//
// 🔴 THE DEFECT WAS A DISAGREEMENT, NOT A TYPO. On origin/main this ledger held
// three surfaces saying `create` (both root-help blocks, `app --help`'s example
// and `agentSetupNoSuchDir`) and three saying `init` (the managed block,
// `remedyNoSuchDir`, `manifest.Load`) — so a new author was told two different
// things by one binary, and the two commands have DIFFERENT defaults. `app
// --help` settles which is meant: "civitai app create is the friendly,
// batteries-included scaffolder … civitai app init is the same scaffolder with a
// no-build static default (back-compat alias)".
//
// The test does not name the winner. It requires the union to be a SINGLE verb,
// so a future change that moves them all together passes and one that moves half
// of them fails.
func TestEveryNewAppRemedyNamesOneScaffolder(t *testing.T) {
	named := map[string][]string{}
	for surface, text := range newAppRemedySurfaces(t) {
		hits := scaffolderRe.FindAllStringSubmatch(text, -1)
		// POSITIVE CONTROL, per surface: a surface that stopped naming a
		// scaffolder at all would otherwise silently drop out of the union and
		// leave the remaining ones trivially in agreement.
		if len(hits) == 0 {
			t.Errorf("%s names no `app create`/`app init` at all — either it stopped telling a new author "+
				"how to scaffold, or this ledger is reading the wrong text:\n%s", surface, text)
			continue
		}
		for _, h := range hits {
			named[h[1]] = appendOnce(named[h[1]], surface)
		}
	}
	if len(named) != 1 {
		var report []string
		for verb, surfaces := range named {
			sort.Strings(surfaces)
			report = append(report, "`civitai app "+verb+"`: "+strings.Join(surfaces, ", "))
		}
		sort.Strings(report)
		t.Errorf("the surfaces that tell a new author how to scaffold do not agree — a new author is told "+
			"%d different commands by one binary, and they default to DIFFERENT templates:\n  %s",
			len(named), strings.Join(report, "\n  "))
	}
}

func appendOnce(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

// ---------------------------------------------------------------------------
// Defect 2 — what an `ok: true` mcp-* row claims
// ---------------------------------------------------------------------------

// TestRegisteredMCPRowSaysWhatItInspected pins the sentence an `ok: true`
// `mcp-*` row carries, and pins the 401 clause to `mcpServer.Anonymous` rather
// than to a Check name.
//
// 🔴 THE PREMISE IS CHECKED, NOT ASSUMED. If the two shipped servers ever agree
// on Anonymous, the per-server assertion below cannot discriminate and this test
// says so instead of passing.
func TestRegisteredMCPRowSaysWhatItInspected(t *testing.T) {
	const path = "/home/u/proj/.mcp.json"

	anon, auth := anonymousServerNames(), authRequiredServerNames()
	if len(anon) == 0 || len(auth) == 0 {
		t.Fatalf("PREMISE BROKEN: the shipped servers no longer disagree on anonymous access (anonymous=%v, "+
			"auth-required=%v) — the discriminating assertion below is vacuous", anon, auth)
	}

	for _, srv := range civitaiMCPServers {
		detail := mcpRegisteredDetail(path, srv)
		// It has to say WHAT it inspected: the file, by name.
		if !strings.Contains(detail, path) {
			t.Errorf("the %s row's ok:true detail does not name the file it read: %q", srv.Check, detail)
		}
		// And that inspecting the file is ALL it did.
		if !strings.Contains(detail, "does not contact "+srv.URL) {
			t.Errorf("the %s row's ok:true detail does not say it did not contact %s — `ok: true` then reads "+
				"as a reachability claim: %q", srv.Check, srv.URL, detail)
		}
		if got, want := strings.Contains(detail, "401"), !srv.Anonymous; got != want {
			t.Errorf("the %s row's ok:true detail mentions 401 = %v, want %v (Anonymous=%v): %q",
				srv.Check, got, want, srv.Anonymous, detail)
		}
	}

	// MUTATION: the 401 clause is DERIVED from the field, not spelled per row.
	// A probe server flipped in both directions must move the sentence.
	probe := mcpServer{Name: "probe", URL: "https://probe.invalid/mcp", Check: "mcp-probe", Anonymous: true}
	if strings.Contains(mcpRegisteredDetail(path, probe), "401") {
		t.Error("an anonymous server's ok:true detail claims a 401 — the clause is not derived from Anonymous")
	}
	probe.Anonymous = false
	if !strings.Contains(mcpRegisteredDetail(path, probe), "401") {
		t.Error("a non-anonymous server's ok:true detail omits the 401 — the clause is not derived from Anonymous")
	}
}

// TestCheckJSONDoesNotReportOrchestrationAsReachable is the same finding at the
// surface a consumer actually reads: `civitai agent-setup --check --json`.
//
// 🔴 MEASURED BY THE DOGFOOD, no credential, POST `initialize`:
// `mcp.civitai.com` → 200, `orchestration.civitai.com` → 401. Before this fix
// both rows read `{"ok": true, "detail": "<path>"}`, so the green row for the
// 401 server was indistinguishable from the green row for the 200 one.
//
// 🔴 IT ASSERTS THE DIFFERENCE, NOT A KEYWORD. The two rows must not be
// interchangeable, because the two servers are not.
func TestCheckJSONDoesNotReportOrchestrationAsReachable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "block.manifest.json"), "{}\n")
	setupInto(t, dir) // writes AGENTS.md, CLAUDE.md and .mcp.json; also sets the env

	out, errOut, err := run(t, "agent-setup", "--check", "--json", "--dir", dir, "--agent", "claude")
	if err != nil {
		t.Fatalf("agent-setup --check --json: %v\n%s\n%s", err, out, errOut)
	}
	var payload struct {
		Checks []struct {
			Name   string `json:"name"`
			OK     bool   `json:"ok"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("--check --json did not emit an object: %v\n%s", err, out)
	}

	details := map[string]string{}
	for _, c := range payload.Checks {
		for _, srv := range civitaiMCPServers {
			if c.Name != srv.Check {
				continue
			}
			// POSITIVE CONTROL: the row has to be the green one, or this test is
			// asserting the wording of a failure row.
			if !c.OK {
				t.Fatalf("%s is not ok:true after a successful setup (%q) — this test is not exercising the "+
					"green row it is about", c.Name, c.Detail)
			}
			details[srv.Check] = c.Detail
		}
	}
	if len(details) != len(civitaiMCPServers) {
		t.Fatalf("found %d of %d mcp rows in --check --json: %v", len(details), len(civitaiMCPServers), details)
	}
	for _, srv := range civitaiMCPServers {
		if got, want := details[srv.Check], mcpRegisteredDetail(agentConfigPathForTest(t, dir), srv); got != want {
			t.Errorf("%s detail = %q, want %q", srv.Check, got, want)
		}
		if strings.TrimSpace(details[srv.Check]) == agentConfigPathForTest(t, dir) {
			t.Errorf("%s's ok:true detail is the bare config path — presence in a file is not reachability of "+
				"%s, and a consumer reading `ok: true` beside a filename cannot tell the two apart",
				srv.Check, srv.URL)
		}
	}
	// The two servers differ, so their rows must differ.
	if len(civitaiMCPServers) > 1 && details[civitaiMCPServers[0].Check] == details[civitaiMCPServers[1].Check] {
		t.Errorf("both mcp rows carry an identical detail (%q) while the servers disagree on anonymous access",
			details[civitaiMCPServers[0].Check])
	}
}

// agentConfigPathForTest resolves the config path the check rows name.
func agentConfigPathForTest(t *testing.T, dir string) string {
	t.Helper()
	path, ok := agentConfigPath(agentEnv{Dir: dir}, agentClaude)
	if !ok {
		t.Fatalf("could not resolve claude's config path under %s", dir)
	}
	return path
}

// ---------------------------------------------------------------------------
// The `<your token>` placeholder had no source
// ---------------------------------------------------------------------------

// TestEveryCredentialPlaceholderNamesItsSource is the guard for the third
// dogfood finding: `agent-setup` printed `export CIVITAI_TOKEN=<your token>` and
// nothing anywhere in the CLI said where that value comes from. The dogfood went
// looking for a command that prints the stored credential and found none — which
// is correct (nothing may print it) and is precisely why the placeholder had to
// name its own source instead.
//
// 🔴 IT IS A RELATIONSHIP, NOT A KEYWORD: wherever the output asks the user to
// type a credential, the SAME output must name where to get one. It is asserted
// over BOTH surfaces that print a placeholder — the interpolating agents' export
// step and the manual `Authorization` block Zed gets — because a fix applied to
// one of the two is how the other keeps the dead end.
func TestEveryCredentialPlaceholderNamesItsSource(t *testing.T) {
	// PREMISE: the two agents below really do take the two different branches.
	if agentTargets[agentClaude].EnvHeaderSyntax == "" || agentTargets[agentZed].EnvHeaderSyntax != "" {
		t.Fatalf("PREMISE BROKEN: claude (%q) and zed (%q) no longer straddle the interpolation branch",
			agentTargets[agentClaude].EnvHeaderSyntax, agentTargets[agentZed].EnvHeaderSyntax)
	}
	for _, agent := range []string{agentClaude, agentZed} {
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			t.Setenv("CIVITAI_TOKEN", credFixtureToken)
			out, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent)
			if err != nil {
				t.Fatalf("agent-setup --agent %s: %v\n%s", agent, err, out)
			}
			// POSITIVE CONTROL: this surface must actually print a placeholder,
			// or the assertion below is about a branch that did not run.
			if !strings.Contains(out, tokenPlaceholder) {
				t.Fatalf("the %s run prints no credential placeholder, so this test asserts nothing about "+
					"it:\n%s", agent, out)
			}
			if !strings.Contains(out, accountAPIKeysURL) {
				t.Errorf("the %s run tells the user to type %s and never names where one comes from — no "+
					"command prints the stored credential, so a placeholder without a source is a dead "+
					"end:\n%s", agent, tokenPlaceholder, out)
			}
			// The sourceless spelling must be gone from every surface, not just
			// the one that was reported.
			if strings.Contains(out, "<your token>") {
				t.Errorf("the %s run still prints the sourceless `<your token>` placeholder:\n%s", agent, out)
			}
			// 🔴 AND THE CREDENTIAL INVARIANT STILL HOLDS: naming the SOURCE must
			// never become surfacing the VALUE.
			if strings.Contains(out, credFixtureToken) {
				t.Errorf("the %s run printed the configured token itself:\n%s", agent, out)
			}
		})
	}
}

// TestTokenSourceNoteSaysAnExportedTokenIsNotRefreshed pins the second half of
// the finding, which is the half that fails LATER.
//
// 🔴 THE CLAIM IS READ OFF internal/config, NOT REMEMBERED. `Config.Token`
// prefers the env-bound `token` key over the stored `access_token`, and
// `Config.AuthKind` returns AuthKindToken — "a personal API key (no refresh)" —
// for ANY env-supplied value. So exporting the token `civitai login` stored
// shadows the refreshable pair and hard-fails at expiry. This test asserts that
// property directly, so the sentence cannot outlive the behaviour it describes.
func TestTokenSourceNoteSaysAnExportedTokenIsNotRefreshed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	t.Setenv("CIVITAI_TOKEN", credFixtureToken)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if got := cfg.AuthKind(); got != config.AuthKindToken {
		t.Fatalf("an env-supplied CIVITAI_TOKEN classifies as %q, want %q — the printed note claims an "+
			"exported value is never refreshed, and that claim rests on this", got, config.AuthKindToken)
	}

	var buf bytes.Buffer
	printTokenSourceNote(&buf, ui.For(&buf), "  ")
	note := buf.String()
	for _, want := range []string{accountAPIKeysURL, "never refreshed", "civitai login"} {
		if !strings.Contains(note, want) {
			t.Errorf("the token-source note does not mention %q:\n%s", want, note)
		}
	}
}
