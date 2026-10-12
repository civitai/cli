// Package plan splits the repo-root test package (`cli_test`) into the tier
// `build-test` runs and the browser tier the `dogfood-browser` shards run, and
// assigns the browser tier to shards.
//
// 🔴 ONE RULE, ONE PLACE. The CI shards (via ../main.go), `build-test`'s skip
// filter, and the root package's own ledger test
// (dogfood_browser_shards_test.go) all call this package. A second copy of the
// partition — a hand-written `-run` list in ci.yml, say — is exactly how a test
// ends up in NEITHER tier: green everywhere, run nowhere.
//
// WHAT DECIDES A TIER: the FILE a test is declared in (FileTiers), never its
// name. A name rule would put a new browser test in whichever tier its name
// happened to match; a file rule makes a new root test file fail the ledger
// until someone decides.
//
// WHAT DECIDES A SHARD: the measured duration of each browser test
// (Durations), packed longest-first onto the least-loaded shard, ties broken by
// name and then by shard index — so the same inputs always give the same plan.
// A browser test with no measured duration is REFUSED rather than given a
// guessed one: the ledger names it and says how to measure it. That is the
// whole procedure for a newly added test.
package plan

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tier is where a root test runs in CI.
type Tier string

const (
	// Always runs in `build-test`, on every PR.
	Always Tier = "always"
	// Browser runs in the `dogfood-browser` shards.
	Browser Tier = "browser"
)

// Shards is the number of `dogfood-browser` shards. ci.yml's matrix must list
// exactly 1..Shards; the root ledger test reads it to make sure.
//
// Chosen by measurement, not by core count: the same test binary as 4
// concurrent process-level shards finished in 81 s locally against 59 s for 8
// and 58 s for 12, and each extra shard costs a whole job's setup (checkout,
// setup-go, compiling the package, the browser smoke) in runner-seconds.
const Shards = 4

// FileTiers assigns every repo-root `*_test.go` to exactly one tier. A file
// missing from this map fails the plan, so a new root test file cannot be
// silently run nowhere — or silently run in both.
//
// The browser tier is the files whose tests drive a real headless browser
// through scripts/dogfood (they call oracleBrowser). Two files that DO touch
// the dogfood harness stay in Always on purpose:
//   - dogfood_t1_floor_test.go reads internal/cmd/app_doctor.go (its codes are
//     the CLI's), so a product change can break it;
//   - dogfood_submission_cap_seam_test.go builds ./cmd/civitai.
var FileTiers = map[string]Tier{
	"agent_setup_fix_path_multishell_test.go": Always,
	"agent_setup_fix_path_shell_test.go":      Always,
	"agents_evidence_test.go":                 Always,
	"agents_index_test.go":                    Always,
	"agents_prose_routing_test.go":            Always,
	"agents_size_test.go":                     Always,
	"agents_split_preserved_test.go":          Always,
	"agents_trigger_test.go":                  Always,
	"agents_xrefs_test.go":                    Always,
	"changelog_filter_ledger_test.go":         Always,
	"claude_md_import_only_test.go":           Always,
	"content_rating_literal_ledger_test.go":   Always,
	"dogfood_brief_test.go":                   Always,
	"dogfood_browser_relevance_test.go":       Always,
	"dogfood_browser_shards_test.go":          Always,
	"dogfood_cdp_launch_test.go":              Browser,
	"dogfood_classifier_precision_test.go":    Always,
	"dogfood_control_wait_test.go":            Always,
	"dogfood_credential_test.go":              Always,
	"dogfood_listing_text_test.go":            Always,
	"dogfood_oracle_consent_test.go":          Browser,
	"dogfood_oracle_injection_test.go":        Browser,
	"dogfood_oracle_picker_test.go":           Browser,
	"dogfood_oracle_post_test.go":             Browser,
	"dogfood_oracle_test.go":                  Browser,
	"dogfood_path_reads_test.go":              Always,
	"dogfood_ship_verdict_injection_test.go":  Always,
	"dogfood_ship_verdict_test.go":            Browser,
	"dogfood_spend_cap_test.go":               Always,
	"dogfood_stop_reason_test.go":             Always,
	"dogfood_submission_cap_seam_test.go":     Always,
	"dogfood_submission_cap_test.go":          Always,
	"dogfood_t1_floor_test.go":                Always,
	"examples_test.go":                        Always,
	"layout_ledger_test.go":                   Always,
	"neterr_ledger_test.go":                   Always,
	"release_page_state_test.go":              Always,
	"revendor_owed_signal_test.go":            Always,
	"saferune_arg_origins_test.go":            Always,
	"saferune_callers_ledger_test.go":         Always,
	"workflow_node_version_test.go":           Always,
	"writefile_callers_ledger_test.go":        Always,
}

// Durations is each browser-tier test's measured wall time in seconds — the
// ONLY input to shard assignment. Its key set must equal the browser tier
// exactly: a browser test missing here, or a key naming no browser test, fails
// the plan.
//
// To add a test: run `go test -json -count=1 -run '^TestYourName$' .` with a
// browser available and copy the top-level `"Action":"pass"` Elapsed here. The
// numbers only need to be right RELATIVE to each other; they were measured in
// one serial run of the package (Go 1.25.14, local, load average ≈50–65 on 24
// cores), with the expected-timeout negative controls on their short wait.
var Durations = map[string]float64{
	"TestABlindMessageInstrumentReportsUnmeasuredNotNo":           9.9,
	"TestABlindPickerInstrumentReportsUnmeasuredNotNo":            7.7,
	"TestAConsentAskBuysNoSpendOnTheUnconsentedArm":               4.2,
	"TestANonPostSummaryLineIsByteIdentical":                      9.9,
	"TestAPickDoesNotBuyAScaffoldAPass":                           3.1,
	"TestCdpLaunchAsksForNoSessionBusServices":                    0.1,
	"TestCdpLaunchAttemptsAreTunable":                             0.9,
	"TestCdpLaunchIsBoundedAndStillFailsLoudly":                   1.7,
	"TestCdpLaunchKillsABrowserItGaveUpOn":                        1.4,
	"TestCdpLaunchRetriesOnceAndAnnouncesItEvenThoughItSucceeded": 1.2,
	"TestCdpLaunchSurvivesAnUnparseableSessionBus":                0.5,
	"TestEveryBrowserResolverAgreesOnTheSameOrder":                0.1,
	"TestGradeCarriesTheRenderResultOntoTheVerdictLine":           1.6,
	"TestGradeReportsUnmeasuredRatherThanFailing":                 0.6,
	"TestGradeVerdictLineCannotBeForgedByTheManifest":             0.4,
	"TestGradeWithoutABriefEmitsNoRenderFields":                   0.7,
	"TestOnlyTheGenerateClickCanEarnTheVerdict":                   2.7,
	"TestOracleAgreesWithTheAssertionOnWhichScopesWereSeeded":     3.7,
	"TestOracleAnswersAHostResourcePick":                          5.4,
	"TestOracleCarriesTheGenpostStatusSequence":                   1.4,
	"TestOracleCarriesTheObservedValueToTheCell":                  2.5,
	"TestOracleDerivesTheBriefFromTheTrial":                       0.6,
	"TestOracleGateNeverDecidesTheVerdict":                        8.2,
	"TestOracleGradesTheGenpostBrief":                             15.4,
	"TestOracleGradesTheShipBriefThroughTheGenpostAssertion":      20.2,
	"TestOracleInlineHostAnswersOnlyThePickerLedger":              0.1,
	"TestOracleLeavesLegitimateManifestValuesByteIdentical":       0.2,
	"TestOracleManifestCannotForgeTheVerdict":                     0.5,
	"TestOracleRefusesEveryRequestThatIsNotAPick":                 1.8,
	"TestOracleRefusesRatherThanGradeTheWrongBrief":               0.2,
	"TestOracleRefusesWhenNothingCanBeMeasured":                   0.1,
	"TestOracleReportsNoAppAsAVerdictNotAHarnessError":            0.1,
	"TestOracleReportsScopesWithoutDeciding":                      5.8,
	"TestOracleResolvesAPickWithThePublishedShape":                1.7,
	"TestOracleSaysTheAppWasNeverBuilt":                           0.3,
	"TestOracleSeedsTheBlocksDeclaredScopes":                      19.0,
	"TestOracleSeedsTheProductionViewerAndNoCredential":           4.1,
	"TestOracleSendsTheHostHandshake":                             7.4,
	"TestOracleServesANoBuildApp":                                 1.5,
	"TestOracleShowsTheBlockASignedInViewer":                      8.8,
	"TestServeBlockRefusesToEscapeItsRoot":                        0.3,
	"TestShipVerdictFailsEveryNegativeControl":                    1.1,
	"TestShipVerdictGradesARealSubmission":                        0.2,
	"TestShipVerdictGradesATrialThatNeverFinished":                0.1,
	"TestShipVerdictNeverMutatesTheAccount":                       0.1,
	"TestShipVerdictNormalisesTheSlugTheWayTheAPIDoes":            0.1,
	"TestShipVerdictRefusesWhenNothingCanBeMeasured":              0.3,
	"TestShipVerdictSeparatesTheTrialsAppFromTheAccountsOwn":      0.3,
	"TestTheMalformedUrlShapeIsOptInAndVisible":                   8.0,
	"TestTheMessageLedgerAnswersNothingAtAll":                     0.2,
	"TestThePostArmFailsAnAppThatCanNeverPost":                    2.6,
	"TestThePostArmGradesTheLinkTheBlockRendersFromTheResult":     27.7,
	"TestThePostArmRefusesAPayloadTheHostWouldRefuse":             5.3,
	"TestThePostArmRefusesAnIncoherentArmCombination":             2.2,
	"TestThePostArmSeesAPostControlWiredToNothing":                9.2,
	"TestThePostArmSeparatesTheGateFromTheWorkflowResult":         11.0,
	"TestThePostArmShimAnswersOnlyItsOwnLedger":                   0.2,
	"TestThePostArmStatesItsCeilingOnEveryCell":                   10.0,
	"TestThePostArmStillRefusesEveryRequestOutsideItsLedger":      5.1,
	"TestThePostArmsCeilingReachesTheGradeRow":                    6.8,
	"TestTheSameAppPassesConsentedAndFailsUnconsented":            6.2,
	"TestTheUnconsentedArmGrantsTheBlockNothing":                  4.8,
	"TestTheUnconsentedArmPassesAnAppThatAsks":                    10.1,
}

// Plan is the partition of one package's top-level tests.
type Plan struct {
	// All is every top-level Test func declared in the root *_test.go files,
	// sorted. It is what `go test -list '.*' .` prints, minus the trailing
	// `ok` line; CI checks that equivalence against the real binary.
	All []string
	// Always is the tier `build-test` runs, sorted.
	Always []string
	// Shards[i] is shard i+1's tests, sorted.
	Shards [][]string
	// Load[i] is the summed measured duration of Shards[i], in seconds.
	Load []float64
}

// Build parses dir's `*_test.go` files (not recursively) and returns the plan
// for n shards.
func Build(dir string, n int) (*Plan, error) {
	if n < 1 {
		return nil, fmt.Errorf("shard count %d < 1", n)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no *_test.go files in %s — the plan would partition nothing", dir)
	}
	var problems []string
	fset := token.NewFileSet()
	seen := map[string]string{}
	var browser []string
	p := &Plan{}
	for _, f := range files {
		base := filepath.Base(f)
		tier, ok := FileTiers[base]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is not in plan.FileTiers — assign it to Always or Browser "+
				"(tools/dogfoodshards/plan/plan.go). Until then its tests would run in NEITHER tier.", base))
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !isTestFunc(fd.Name.Name) {
				continue
			}
			name := fd.Name.Name
			if prev, dup := seen[name]; dup {
				problems = append(problems, fmt.Sprintf("%s is declared in both %s and %s", name, prev, base))
			}
			seen[name] = base
			p.All = append(p.All, name)
			if tier == Browser {
				browser = append(browser, name)
			} else {
				p.Always = append(p.Always, name)
			}
		}
	}
	for base := range FileTiers {
		if _, err := os.Stat(filepath.Join(dir, base)); err != nil {
			problems = append(problems, fmt.Sprintf("plan.FileTiers names %s, which does not exist — remove it", base))
		}
	}
	inBrowser := map[string]bool{}
	for _, name := range browser {
		inBrowser[name] = true
		if _, ok := Durations[name]; !ok {
			problems = append(problems, fmt.Sprintf("browser-tier test %s (%s) has no entry in plan.Durations, so it "+
				"cannot be assigned a shard. Measure it: `go test -json -count=1 -run '^%s$' .` and copy the "+
				"top-level pass Elapsed into tools/dogfoodshards/plan/plan.go.", name, seen[name], name))
		}
	}
	for name := range Durations {
		if !inBrowser[name] {
			where := "is not declared in any root *_test.go"
			if f, ok := seen[name]; ok {
				where = "is declared in " + f + ", which is not a browser-tier file"
			}
			problems = append(problems, fmt.Sprintf("plan.Durations names %s, which %s — remove the entry", name, where))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("the root test package cannot be partitioned:\n  %s", strings.Join(problems, "\n  "))
	}

	// Longest-processing-time-first: deterministic, and within 4/3 of optimal.
	sort.Slice(browser, func(i, j int) bool {
		di, dj := Durations[browser[i]], Durations[browser[j]]
		if di != dj {
			return di > dj
		}
		return browser[i] < browser[j]
	})
	p.Shards = make([][]string, n)
	p.Load = make([]float64, n)
	for _, name := range browser {
		best := 0
		for i := 1; i < n; i++ {
			if p.Load[i] < p.Load[best] {
				best = i
			}
		}
		p.Shards[best] = append(p.Shards[best], name)
		p.Load[best] += Durations[name]
	}
	sort.Strings(p.All)
	sort.Strings(p.Always)
	for i := range p.Shards {
		sort.Strings(p.Shards[i])
		if len(p.Shards[i]) == 0 {
			return nil, fmt.Errorf("shard %d of %d would run no test — fewer browser tests than shards; "+
				"lower plan.Shards", i+1, n)
		}
	}
	return p, nil
}

// isTestFunc mirrors what `go test` collects as a top-level test: TestXxx where
// Xxx does not start with a lower-case letter. Benchmark/Fuzz/Example are not
// tests the tiers run; Build refuses a package that declares one (see List).
func isTestFunc(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	rest := name[len("Test"):]
	return rest == "" || !(rest[0] >= 'a' && rest[0] <= 'z')
}

// Regex returns the anchored alternation `^(A|B|…)$` for names, suitable for
// `go test -run` and `-skip`. Names are QuoteMeta'd. An empty list is an
// error: an empty alternation would match nothing (or, unanchored, everything).
func Regex(names []string) (string, error) {
	if len(names) == 0 {
		return "", fmt.Errorf("refusing to build a filter for zero tests")
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = regexp.QuoteMeta(n)
	}
	return "^(" + strings.Join(q, "|") + ")$", nil
}

// Browser returns every browser-tier test across all shards, sorted.
func (p *Plan) Browser() []string {
	var out []string
	for _, s := range p.Shards {
		out = append(out, s...)
	}
	sort.Strings(out)
	return out
}
