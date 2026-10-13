package cli_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// THE NEGATIVE CONTROLS THAT ARE SUPPOSED TO TIME OUT, AND THE ONLY PLACE THEIR
// WAIT IS SHORTENED.
//
// A browser leaf whose expected verdict is `no` because the thing it waits for
// never appears — the input on a scaffold, the prompt for an anonymous viewer,
// the status after an inert Generate — earns that `no` by sitting out the whole
// in-page budget (`WAIT_MS` in scripts/dogfood/briefs/_cdp.mjs, 15 s). Measured
// on this package before this file existed: 8 leaves, 9 waits (the ship
// delegation test grades its block twice), ≈135 s of a fully serial ≈320 s
// package spent waiting for something the fixture was written never to render.
//
// 🔴 THE HAZARD A SHORT WAIT INTRODUCES, AND WHAT STANDS AGAINST IT. A control
// that reports "absent" is only evidence if the wait was long enough to have
// SEEN the thing had it been there. A budget below the time a real mount takes
// on a slow runner would let every control here pass while the oracle had
// stopped being able to tell present from absent. So:
//
//   - the short budget is chosen from the measured time the SUCCESSFUL waits
//     take (CIVITAI_ASSERT_WAIT_LOG, which build-test now publishes per run),
//     with margin over the worst observed under load — see the PR that added
//     this file for the distribution and the load it was taken at;
//   - each ledgered leaf's control was re-run at the short budget with the
//     missing thing made PRESENT, and the control's own expectation went red —
//     the mutation table is in the same PR;
//   - the default stays 15 s for every leaf NOT in this ledger, and the guard
//     below makes a positive case on the short budget a failure rather than a
//     faster flake.
//
// ⚠ This shortens nothing for an operator grading a real trial: the knob is set
// on the child environment of a ledgered leaf only, and _cdp.mjs's default is
// unchanged.
const (
	// The in-page wait a ledgered leaf gets when nobody raised the default.
	// Measured basis in the PR body. It is a FLOOR as well: see controlBudget.
	dogfoodControlWaitMS = 5000
	// _cdp.mjs's own default, pinned against that file by
	// TestControlWaitDefaultMatchesTheHarness so the two cannot drift.
	dogfoodDefaultWaitMS = 15000
	controlWaitEnvKey    = "CIVITAI_ASSERT_WAIT_MS"
	waitLogEnvKey        = "CIVITAI_ASSERT_WAIT_LOG"
)

// expectedTimeoutLeaves: leaf name (t.Name(), exactly as `go test -v` prints
// it) → what the fixture never renders. Keyed by the LEAF, never by a parent:
// every one of these sits beside a positive sibling that must keep the full
// default.
//
// Adding a leaf: run it with `CIVITAI_ASSERT_WAIT_LOG=/tmp/w.jsonl go test -run
// '<Parent>/<leaf>' .` and read /tmp/w.jsonl for an `"ok":false` row — NOT the
// test's output, which the post-arm leaf never prints a timeout into. If the
// wait it times out on is the absence the control exists to show, add it here.
// A new negative control that is NOT listed simply keeps the full 15 s: slower,
// still correct, and nothing fails. A LISTED leaf that stops timing out does
// fail (checkControlWait), and a listed name that matches no test fails
// statically, so an entry cannot go stale silently.
var expectedTimeoutLeaves = map[string]string{
	"TestOracleGateNeverDecidesTheVerdict/green_gate_over_a_scaffold":                               `[data-testid="celsius"] on an untouched scaffold`,
	"TestOracleSendsTheHostHandshake/without_it_(control)":                                          `[data-testid="celsius"] on a host-gated block with no handshake`,
	"TestOracleGradesTheGenpostBrief/Generate_does_nothing":                                         `a status change after clicking an inert Generate`,
	"TestOracleSeedsTheBlocksDeclaredScopes/the_same_app,_manifest_declaring_nothing_(control)":     `a status change after Generate on a consent-gated app shown no scopes`,
	"TestOracleSeedsTheBlocksDeclaredScopes/scopes_do_not_buy_an_untouched_scaffold_a_pass":         `[data-testid="prompt"] on an untouched scaffold`,
	"TestOracleShowsTheBlockASignedInViewer/anonymous_(the_control_arm)":                            `[data-testid="prompt"] on an auth-gated app shown an anonymous viewer`,
	"TestOracleGradesTheShipBriefThroughTheGenpostAssertion/a_block_with_none_of_the_brief's_hooks": `[data-testid="prompt"] on a block with none of the brief's test ids (graded twice: ship, then genpost)`,
	"TestThePostArmSeesAPostControlWiredToNothing":                                                  `a CREATE_POST_FROM_APP after clicking a Post control wired to nothing`,
}

// controlWaitEnv returns the environment a browser leaf's child gets, the
// in-page budget that environment carries, and the wait log the child will
// write. `full` is os.Environ() plus the leaf's own entries, in that order
// (later wins, as in exec.Cmd.Env).
func controlWaitEnv(t *testing.T, full []string) ([]string, int, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "waits.jsonl")
	env, budget, err := resolveControlWait(t.Name(), full, log)
	if err != nil {
		t.Fatal(err)
	}
	return env, budget, log
}

// checkControlWait reads the wait log the child wrote. Errorf, not Fatalf: the
// leaf's own assertion still runs and reports, which matters when the cause is
// a positive case that genuinely failed.
//
// 🔴 IT READS THE HARNESS'S STRUCTURED LOG, NOT ITS OUTPUT. The first version of
// this matched `timed out after <n>ms` in the oracle's output, and it was
// measured blind to one of the eight ledgered leaves: the post arm CATCHES the
// timeout on its post wait (`try { … } catch { /* nothing was sent */ }`), so
// that leaf earns its `no` by timing out and prints no timeout line at all.
func checkControlWait(t *testing.T, budget int, log string) {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("read the wait log %s: %v", log, err)
		return
	}
	// Forward to an operator's own log, which this leaf's private one shadowed —
	// build-test publishes the distribution from it.
	if dst := os.Getenv(waitLogEnvKey); dst != "" && len(raw) > 0 {
		if f, err := os.OpenFile(dst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.Write(raw)
			_ = f.Close()
		}
	}
	rows, err := parseWaitLog(raw)
	if err != nil {
		t.Errorf("the wait log %s is not the harness's: %v", log, err)
		return
	}
	if err := controlWaitVerdict(t.Name(), rows, budget); err != nil {
		t.Error(err)
	}
}

// One line of _cdp.mjs's CIVITAI_ASSERT_WAIT_LOG.
type waitRow struct {
	What   string `json:"what"`
	OK     bool   `json:"ok"`
	MS     int    `json:"ms"`
	Budget int    `json:"budget"`
}

func parseWaitLog(raw []byte) ([]waitRow, error) {
	var rows []waitRow
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var r waitRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("line %q: %w", line, err)
		}
		if r.What == "" || r.Budget == 0 {
			return nil, fmt.Errorf("line %q carries no `what` or no `budget`", line)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// resolveControlWait is controlWaitEnv without the *testing.T, so the guard can
// be driven by a table.
//
// 🔴 IT READS THE EFFECTIVE VALUE, NOT THE LEAF'S INTENT. The hazard is a
// positive case inheriting the short budget — from an `os.Setenv` a previous
// serial test leaked into the process, from an operator's shell, or from a
// hand-added env entry — and every one of those arrives through `full`, not
// through anything the leaf itself says.
func resolveControlWait(leaf string, full []string, log string) ([]string, int, error) {
	v := effectiveEnv(full, controlWaitEnvKey)
	full = append(full, waitLogEnvKey+"="+log)
	if _, ledgered := expectedTimeoutLeaves[leaf]; ledgered {
		budget := controlBudget(v)
		full = append(full, fmt.Sprintf("%s=%d", controlWaitEnvKey, budget))
		return full, budget, nil
	}
	if v == "" {
		return full, dogfoodDefaultWaitMS, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < dogfoodDefaultWaitMS {
		return nil, 0, fmt.Errorf("%s=%q reaches %s, which is NOT in expectedTimeoutLeaves "+
			"(dogfood_control_wait_test.go).\nA leaf outside that ledger must run on the full %d ms budget: "+
			"a positive case on a short wait turns a slow mount into a false `no`, and a negative one on a "+
			"wait nobody measured proves nothing about absence. If this came from the PROCESS environment, "+
			"an earlier test leaked it with os.Setenv — use t.Setenv, or better, add the leaf to the ledger",
			controlWaitEnvKey, v, leaf, dogfoodDefaultWaitMS)
	}
	return full, n, nil
}

// controlBudget is the wait a ledgered leaf gets, given the value of
// CIVITAI_ASSERT_WAIT_MS that would otherwise have reached it.
//
// 🔴 A RAISED DEFAULT RAISES THE CONTROL TOO. An operator who sets 30000 is
// saying "this machine mounts slowly"; forcing 5000 on the controls there is
// the exact hazard this file exists to rule out (a control reporting "absent"
// before a real mount could have finished), on the one machine known to be
// slow. So the control keeps the same 1:3 ratio to whatever the positive cases
// get, and never goes below the measured floor: max(5000, v/3).
//
// A value BELOW the default, or one that does not parse, does not shorten the
// control further — 5000 is the smallest budget anyone measured a margin for.
// (Such a value is refused outright for every unledgered leaf, so the run is
// already red; this only decides what the ledgered leaves do meanwhile.)
func controlBudget(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n/3 < dogfoodControlWaitMS {
		return dogfoodControlWaitMS
	}
	return n / 3
}

// controlWaitVerdict judges one oracle run's wait log against the ledger.
//
// ⚠ PER RUN, NOT PER LEAF: a ledgered leaf that calls the oracle twice (the
// ship delegation test grades one block as `ship` and then as `genpost`) must
// time out on EACH call. True of every entry today; a leaf where only one call
// times out would need splitting.
func controlWaitVerdict(leaf string, rows []waitRow, budget int) error {
	var timeouts []string
	for _, r := range rows {
		if r.Budget != budget {
			return fmt.Errorf("%s: the harness waited on a %d ms budget for %q, but this run set %d ms — "+
				"the budget is not reaching the assertion, so neither the ledger nor the guard on it "+
				"is measuring anything", leaf, r.Budget, r.What, budget)
		}
		if !r.OK {
			timeouts = append(timeouts, fmt.Sprintf("%q after %d ms", r.What, r.MS))
		}
	}
	if _, ledgered := expectedTimeoutLeaves[leaf]; ledgered {
		if len(timeouts) == 0 {
			return fmt.Errorf("%s is in expectedTimeoutLeaves but no in-page wait timed out (%d wait(s) "+
				"logged). Either the log is wired to nothing, or the control no longer earns its verdict by "+
				"timing out — in which case its entry is stale and must go", leaf, len(rows))
		}
	}
	// An UNLEDGERED leaf that times out is not judged here: a new negative
	// control on the full budget is slower, not wrong, and a positive case that
	// timed out fails its own assertion.
	return nil
}

func effectiveEnv(env []string, key string) string {
	v := ""
	for _, kv := range env {
		if k, val, ok := strings.Cut(kv, "="); ok && k == key {
			v = val
		}
	}
	return v
}

// ── the ledger's own guards (no browser) ────────────────────────────────────

var cdpWaitDefaultRe = regexp.MustCompile(`export const WAIT_MS = Number\(process\.env\.CIVITAI_ASSERT_WAIT_MS \|\| (\d+)\);`)

// The default this file treats as "unshortened" is _cdp.mjs's, read from it.
// And the log checkControlWait reads is one _cdp.mjs writes. Both halves are
// also checked at RUN time — a ledgered leaf with no timeout row fails — but
// only when a browser is present; this is the half that runs everywhere.
func TestControlWaitDefaultMatchesTheHarness(t *testing.T) {
	raw, err := os.ReadFile(cdpModule)
	if err != nil {
		t.Fatalf("read %s: %v", cdpModule, err)
	}
	m := cdpWaitDefaultRe.FindSubmatch(raw)
	if m == nil {
		t.Fatalf("%s no longer declares `export const WAIT_MS = Number(process.env.CIVITAI_ASSERT_WAIT_MS || <n>);` "+
			"— the knob the expected-timeout ledger sets was renamed or moved, so the ledger shortens nothing.", cdpModule)
	}
	if got, _ := strconv.Atoi(string(m[1])); got != dogfoodDefaultWaitMS {
		t.Fatalf("%s defaults WAIT_MS to %d, but dogfoodDefaultWaitMS is %d — the guard that keeps positive "+
			"cases on the full budget is comparing against the wrong number.", cdpModule, got, dogfoodDefaultWaitMS)
	}
	if !strings.Contains(string(raw), "process.env.CIVITAI_ASSERT_WAIT_LOG") {
		t.Fatalf("%s no longer reads CIVITAI_ASSERT_WAIT_LOG, which is the log checkControlWait judges "+
			"every browser leaf by. Re-point it before trusting either direction of that check.", cdpModule)
	}
}

// 🔴 A LEDGER ENTRY THAT NAMES NOTHING IS A GUARD THAT NEVER RUNS. A renamed
// subtest would leave its entry matching no leaf: the control falls back to the
// full budget (slower, still correct) and NOTHING at run time notices — an
// unledgered leaf that times out is deliberately not a failure. So each key is
// resolved statically: the parent must be a Test func declared in this package,
// and the subtest part must be the `go test` rewrite of a string literal in that
// func's body.
func TestEveryExpectedTimeoutLeafNamesARealTest(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no *_test.go files found (err=%v) — this guard checked nothing", err)
	}
	bodies := map[string]*ast.FuncDecl{}
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && strings.HasPrefix(fd.Name.Name, "Test") {
				bodies[fd.Name.Name] = fd
			}
		}
	}
	if len(expectedTimeoutLeaves) == 0 {
		t.Fatal("expectedTimeoutLeaves is empty — every check against it is vacuous")
	}
	var bad []string
	for leaf := range expectedTimeoutLeaves {
		parent, sub, hasSub := strings.Cut(leaf, "/")
		fd, ok := bodies[parent]
		if !ok {
			bad = append(bad, leaf+": no `func "+parent+"` in this package")
			continue
		}
		if !hasSub {
			continue
		}
		found := false
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if s, err := strconv.Unquote(bl.Value); err == nil && rewriteSubtestName(s) == sub {
					found = true
				}
			}
			return !found
		})
		if !found {
			bad = append(bad, leaf+": no string literal in "+parent+" rewrites to "+strconv.Quote(sub))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("expectedTimeoutLeaves names leaves that do not exist, so their control runs on the full "+
			"budget and nothing notices:\n  %s", strings.Join(bad, "\n  "))
	}
}

// The `testing` package's rewrite of a subtest name: whitespace becomes `_`,
// a non-printable rune is quoted.
func rewriteSubtestName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			b.WriteByte('_')
		case !strconv.IsPrint(r):
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// The two runtime guards, driven by a table and no browser.
func TestControlWaitGuardsReadTheEffectiveEnvironment(t *testing.T) {
	const leaf = "TestOracleGradesTheGenpostBrief/a_generate-then-post_app" // a POSITIVE sibling
	const ledgered = "TestOracleGradesTheGenpostBrief/Generate_does_nothing"
	if _, ok := expectedTimeoutLeaves[ledgered]; !ok {
		t.Fatalf("fixture leaf %q is no longer ledgered; re-point this table", ledgered)
	}
	short := fmt.Sprintf("%s=%d", controlWaitEnvKey, dogfoodControlWaitMS)
	for _, tc := range []struct {
		name    string
		leaf    string
		env     []string
		wantErr string
		want    int
	}{
		{"positive, unset: the default", leaf, []string{"PATH=/x"}, "", dogfoodDefaultWaitMS},
		{"positive, an operator's LONGER wait is allowed", leaf, []string{controlWaitEnvKey + "=30000"}, "", 30000},
		{"positive, set then cleared: the default", leaf, []string{short, controlWaitEnvKey + "="}, "", dogfoodDefaultWaitMS},
		{"positive, the short wait leaked into the process env", leaf, []string{short, "PATH=/x"}, "NOT in expectedTimeoutLeaves", 0},
		{"positive, garbage", leaf, []string{controlWaitEnvKey + "=soon"}, "NOT in expectedTimeoutLeaves", 0},
		{"ledgered, unset: the short budget", ledgered, []string{"PATH=/x"}, "", 5000},
		{"ledgered, the default spelled out: the short budget", ledgered, []string{controlWaitEnvKey + "=15000"}, "", 5000},
		{"ledgered, an operator's raised wait scales the control", ledgered, []string{controlWaitEnvKey + "=30000"}, "", 10000},
		// A third point on the line: 5000 and 10000 alone are also what a step
		// function (`>15000 → 10000`) and `n − 20000` produce.
		{"ledgered, and keeps scaling: a third of 45000", ledgered, []string{controlWaitEnvKey + "=45000"}, "", 15000},
		{"ledgered, a value below the floor never shortens the control", ledgered, []string{controlWaitEnvKey + "=1000"}, "", 5000},
		{"ledgered, garbage: the short budget", ledgered, []string{controlWaitEnvKey + "=soon"}, "", 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, got, err := resolveControlWait(tc.leaf, append([]string(nil), tc.env...), "/x/waits.jsonl")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err=%v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if got != tc.want {
				t.Fatalf("budget=%d, want %d", got, tc.want)
			}
			// The budget reported must be the budget the CHILD gets.
			eff := effectiveEnv(env, controlWaitEnvKey)
			if (eff == "" && got != dogfoodDefaultWaitMS) || (eff != "" && eff != strconv.Itoa(got)) {
				t.Fatalf("reported budget %d but the child environment carries %q", got, eff)
			}
		})
	}

	timeout := func(budget int) waitRow { return waitRow{What: "x", OK: false, MS: budget + 20, Budget: budget} }
	mounted := func(budget int) waitRow { return waitRow{What: "x", OK: true, MS: 300, Budget: budget} }
	for _, tc := range []struct {
		name, leaf string
		rows       []waitRow
		budget     int
		wantErr    string
	}{
		{"ledgered and timed out at the short wait", ledgered, []waitRow{mounted(dogfoodControlWaitMS), timeout(dogfoodControlWaitMS)}, dogfoodControlWaitMS, ""},
		{"ledgered but never timed out: stale entry", ledgered, []waitRow{mounted(dogfoodControlWaitMS)}, dogfoodControlWaitMS, "no in-page wait timed out"},
		{"ledgered but the log is empty: wired to nothing", ledgered, nil, dogfoodControlWaitMS, "no in-page wait timed out"},
		{"ledgered but the harness ignored the budget", ledgered, []waitRow{timeout(dogfoodDefaultWaitMS)}, dogfoodControlWaitMS, "is not reaching the assertion"},
		{"positive and never timed out", leaf, []waitRow{mounted(dogfoodDefaultWaitMS)}, dogfoodDefaultWaitMS, ""},
		{"unledgered and waited out the full budget: slower, not wrong", leaf, []waitRow{timeout(dogfoodDefaultWaitMS)}, dogfoodDefaultWaitMS, ""},
		{"positive but the harness used another budget", leaf, []waitRow{mounted(dogfoodControlWaitMS)}, dogfoodDefaultWaitMS, "is not reaching the assertion"},
	} {
		t.Run("verdict: "+tc.name, func(t *testing.T) {
			err := controlWaitVerdict(tc.leaf, tc.rows, tc.budget)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err=%v, want one containing %q", err, tc.wantErr)
			}
		})
	}

	// The parser refuses a log that is not the harness's, rather than reading
	// it as "no timeouts".
	for _, bad := range []string{"not json\n", `{"ok":false,"ms":1}` + "\n"} {
		if _, err := parseWaitLog([]byte(bad)); err == nil {
			t.Errorf("parseWaitLog accepted %q", bad)
		}
	}
	if rows, err := parseWaitLog([]byte(`{"what":"a","ok":false,"ms":5010,"budget":5000}` + "\n")); err != nil || len(rows) != 1 || rows[0].OK {
		t.Errorf("parseWaitLog on a real row: %v %v", rows, err)
	}
}
