package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// PRECISION of the dogfood run-cap classifier (scripts/dogfood/runner.py).
//
// 🔴 THE MEASUREMENT. Trial `ab-genpost-dsv4-02` (deepseek-v4-pro, 2026-09-25)
// produced THREE refusals and all three were wrong:
//
//	step  9  civitai app create civitai-image-generator --template static
//	         → correctly refused on `civitai-image-generator`, AND ALSO on
//	           `static`, the value of --template.
//	step 11  civitai app create ab-image-generator --template static
//	         → refused on `static` alone. The app name was already correct. The
//	           refusal said "Rename the app and retry", which named the wrong fix;
//	           the agent dropped `--template static` and fell back to the 320 KB
//	           page-money default, so a FALSE refusal silently changed which
//	           scaffold the experiment measured.
//	step 25  cat > block.manifest.json << 'ENDOFFILE' … ENDOFFILE
//	         → the fail-closed rule fired on a LINE OF THE JSON BEING WRITTEN:
//	           "posts:write:self": "Posts generated images to the user's Civitai
//	           profile…" names "Civitai" and contains "generate" inside
//	           "generated". The agent's next attempt used python3, absent from the
//	           image (rc=127), so one false refusal cost two steps.
//
// 🔴 AND THE CONSTRAINT THE FIX IS IN TENSION WITH. `6f9bf96` (#698) closed a
// real bypass: `positional()` dropped every token starting with `-`, so
// `civitai app create --slug=some-other-app` yielded no candidate, fell through
// to the "read every block.manifest.json under /work" branch, and was ACCEPTED
// while --slug pointed at a foreign listing.
// TestDogfoodAttachedSlugFlagDoesNotBypassThePrefixCap in dogfood_spend_cap_test.go
// pins that and must stay green — including its `--dir=sensei` structural arm.
//
// Both hold because the classifier now reads a LEDGER of which flags carry an app
// identity (`GATED_FLAGS` in runner.py) instead of treating every value as a slug:
// "slug"-role flag values are candidates in both spellings, "value"-role flags
// consume their next token so it is not mistaken for a positional, and an unknown
// flag defaults to consuming nothing — the over-refusing direction.
//
// Everything here runs offline through testdata/fake_trial.py: no Docker, no
// OpenRouter, no money, no account.

// ── the false refusal: a flag's value is not an app name ──────────────────────

// 🔴 THE REGRESSION TEST. Red on origin/main, where every one of these is
// REFUSED because the flag's value is slug-shaped and does not carry the trial's
// prefix.
//
// Each arm names a correctly-prefixed app (or none at all), so the ONLY thing
// that can refuse it is a flag value being read as an app name.
func TestDogfoodAFlagValueIsNotReadAsAnAppName(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, tc := range []struct{ name, command string }{
		// The two measured shapes, plus the attached and shorthand spellings —
		// cobra treats all three as identical and the classifier must too.
		{"the measured refusal", "civitai app create ab-image-generator --template static"},
		{"attached", "civitai app create ab-image-generator --template=static"},
		{"shorthand separated", "civitai app create ab-image-generator -t static"},
		// ⚠ NOT RED AT BASE, and counted separately: at origin/main `positional()`
		// dropped any token starting with `-` unless it contained `=`, so `-tstatic`
		// contributed nothing and this shape was already allowed. It is pinned so the
		// three shorthand spellings cannot diverge again.
		{"shorthand attached", "civitai app create ab-image-generator -tstatic"},
		{"shorthand with =", "civitai app create ab-image-generator -t=static"},
		// A template name that is itself hyphenated slug-shaped: the same defect,
		// and the value the brief most often asks for.
		{"page-vite", "civitai app init ab-thing --template page-vite"},
		// It was never --template-specific. Every value-taking flag on a gated
		// command had it.
		{"submit --out", "civitai app submit --out bundle"},
		{"submit -o", "civitai app submit -o bundle"},
		{"listing --caption", "civitai app listing add-screenshot ./s.png --caption wide-shot --slug=ab-thing"},
		{"listing --changelog", "civitai app listing submit-revision --changelog typo-fix --slug=ab-thing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrial(t, []string{tc.command}, "",
				"--credential-file", credPath, "--app-prefix", "ab-")
			got := stepVerdicts(t, readFile(t, tr.transcript))
			if len(got) != 1 || got[0] != "run" {
				t.Fatalf("%q was REFUSED (verdicts %v).\nNothing in it names an app outside the "+
					"prefix — the refusal is the harness reading a flag's VALUE as a slug. A false "+
					"refusal is not a safe default here: the measured agent believed one, dropped "+
					"`--template static`, and the trial measured a different scaffold than the brief "+
					"asked for.\n%s", tc.command, got, readFile(t, tr.transcript))
			}
		})
	}
}

// 🔴 THE MESSAGE IS PART OF THE DEFECT, NOT COSMETICS. Red on origin/main, where
// the refusal always ends "Rename the app and retry." — which on step 11 was said
// about `static`. A remedy that names the wrong fix is worse than no remedy: it
// is actionable, and following it is what corrupted the measurement.
func TestDogfoodTheRefusalNamesTheOffendingTokensRole(t *testing.T) {
	credPath, _ := credentialFile(t)

	t.Run("a flag value", func(t *testing.T) {
		tr := runFakeTrial(t, []string{"civitai app create ab-thing --slug=some-other-app"}, "",
			"--credential-file", credPath, "--app-prefix", "ab-")
		body := readFile(t, tr.transcript)
		if got := stepVerdicts(t, body); len(got) != 1 || got[0] != "refused" {
			t.Fatalf("a foreign --slug was not refused (verdicts %v)\n%s", got, body)
		}
		if !strings.Contains(body, "--slug") {
			t.Errorf("the refusal does not name the flag that carried the foreign slug, so the trial "+
				"cannot tell WHICH token was rejected:\n%s", body)
		}
		if strings.Contains(body, "Rename the app") {
			t.Errorf("the refusal tells the trial to rename the app, and the app name is not what was "+
				"refused — `--slug` is. This is the sentence the measured agent acted on:\n%s", body)
		}
	})

	// The mirror image, and the discrimination that stops the fix from being "never
	// say rename": when the APP NAME really is the offender, that IS the remedy.
	//
	// ⚠ INVARIANT GUARD, NOT REGRESSION COVERAGE — it passes at origin/main, where
	// every refusal says this. It is here so the message fix cannot be satisfied by
	// deleting the remedy.
	t.Run("the app-name argument", func(t *testing.T) {
		tr := runFakeTrial(t, []string{"civitai app create some-other-app"}, "",
			"--credential-file", credPath, "--app-prefix", "ab-")
		body := readFile(t, tr.transcript)
		if got := stepVerdicts(t, body); len(got) != 1 || got[0] != "refused" {
			t.Fatalf("a foreign positional app name was not refused (verdicts %v)\n%s", got, body)
		}
		if !strings.Contains(body, "Rename the app and retry") {
			t.Errorf("the refusal no longer names the remedy for the case where the remedy really is a "+
				"rename:\n%s", body)
		}
	})
}

// runFakeTrialOneCommand feeds ONE command — which may span lines — through the
// offline trial.
//
// 🔴 IT EXISTS BECAUSE `runFakeTrial` SEPARATES COMMANDS BY NEWLINE, so a
// here-document handed to it arrives as four separate assistant turns and the
// classifier never sees the shape under test. Measured while writing this file: a
// heredoc arm reported verdicts [refused run run run] — four steps for one
// command. `FAKE_TOOL_COMMANDS_JSON` (testdata/fake_trial.py) is the JSON-array
// form that takes precedence; every existing caller is untouched.
func runFakeTrialOneCommand(t *testing.T, command string, extra ...string) fakeTrial {
	t.Helper()
	raw, err := json.Marshal([]string{command})
	if err != nil {
		t.Fatalf("encoding the command: %v", err)
	}
	tr := runFakeTrialEnv(t, []string{"FAKE_TOOL_COMMANDS_JSON=" + string(raw)}, nil, "", extra...)
	// CONTROL: the whole point of this helper is ONE step. If the harness split
	// it, every verdict assertion below is about a fragment.
	if got := stepVerdicts(t, readFile(t, tr.transcript)); len(got) != 1 {
		t.Fatalf("CONTROL failure, not a finding: the trial produced %d step(s) (%v) for one command, "+
			"want 1 — the command was split, so nothing was judged as written.\n%s",
			len(got), got, readFile(t, tr.transcript))
	}
	return tr
}

// ── the false refusal: a file being written is data, not a command ────────────

// 🔴 THE REGRESSION TEST for step 25, with the exact bytes that were refused.
// Red on origin/main: `segments()` splits on newlines, so each line of the JSON
// is classified, and the `posts:write:self` justification line trips the
// fail-closed rule ("Civitai" + "generated").
func TestDogfoodAHeredocBodyIsNotClassifiedAsCommandText(t *testing.T) {
	credPath, _ := credentialFile(t)
	const measured = `cat > /work/ab-image-generator/block.manifest.json << 'ENDOFFILE'
{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "ab-image-generator",
  "scopes": ["ai:write:budgeted", "posts:write:self"],
  "scopeJustifications": {
    "ai:write:budgeted": "Runs image generations on the user's behalf, spending from the per-generation Buzz budget the page grants.",
    "posts:write:self": "Posts generated images to the user's Civitai profile as public posts."
  },
  "buildCommand": "npm run build",
  "outputDir": "dist"
}
ENDOFFILE
echo "Manifest updated"`

	for _, tc := range []struct{ name, command string }{
		{"the measured write", measured},
		// The same hazard in the other heredoc spellings.
		{"unquoted delimiter", "cat > m.json << EOF\n\"x\": \"Posts generated images to Civitai\"\nEOF"},
		{"double-quoted delimiter", "cat > m.json << \"EOF\"\n\"x\": \"Posts generated images to Civitai\"\nEOF"},
		{"tab-stripped <<-", "cat > m.json <<- EOF\n\t\"x\": \"Posts generated images to Civitai\"\n\tEOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrialOneCommand(t, tc.command,
				"--credential-file", credPath, "--app-prefix", "ab-")
			got := stepVerdicts(t, readFile(t, tr.transcript))
			if got[0] != "run" {
				t.Fatalf("writing a file was REFUSED (verdicts %v).\nA here-document body is stdin for "+
					"the opener's command — the shell never executes it — so it is data. Classifying it "+
					"as command text refuses an app's own manifest for quoting the platform's name.\n%s",
					got, readFile(t, tr.transcript))
			}
		})
	}
}

// 🔴 THE DISCRIMINATING CONTROL, without which the fix above is satisfied by
// "stop classifying anything that follows a `<<`". A real invocation AFTER the
// heredoc terminator is command text and must still be judged.
//
// ⚠ INVARIANT GUARD, NOT REGRESSION COVERAGE: at origin/main these are refused
// too (for the body as well as the command). It exists so the heredoc fix cannot
// over-reach.
func TestDogfoodHeredocStrippingDoesNotHideARealInvocation(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, tc := range []struct{ name, command, wantIn string }{
		{
			"an eval after the terminator",
			"cat > m.json << 'EOF'\n{\"blockId\": \"ab-thing\"}\nEOF\neval \"civitai app submit\"",
			"not a form the harness can read",
		},
		{
			"a foreign slug after the terminator",
			"cat > m.json << 'EOF'\n{\"blockId\": \"ab-thing\"}\nEOF\ncivitai app listing status --slug=some-other-app",
			"some-other-app",
		},
		{
			// The opener LINE is command text and is kept, so a foreign --slug on
			// it is still caught even though the heredoc it opens is dropped.
			"a real invocation on the opener line itself",
			"civitai app listing status --slug=some-other-app --dir \"$(cat << 'EOF'\nhello\nEOF\n)\"",
			"some-other-app",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrialOneCommand(t, tc.command,
				"--credential-file", credPath, "--app-prefix", "ab-")
			body := readFile(t, tr.transcript)
			got := stepVerdicts(t, body)
			if got[0] != "refused" {
				t.Fatalf("%q reached the container (verdicts %v). Stripping here-document BODIES must "+
					"not stop the surrounding command text from being classified.\n%s",
					tc.command, got, body)
			}
			if !strings.Contains(body, tc.wantIn) {
				t.Errorf("refused for the wrong reason — the refusal does not contain %q:\n%s", tc.wantIn, body)
			}
		})
	}
}

// ── the narrowing must not reopen #698 ───────────────────────────────────────

// 🔴 THIS IS A NARROWING OF A SECURITY CHECK, so the flags that DO carry an app
// identity are pinned in BOTH spellings. `--slug` is the obvious one; `--from`
// ("fork from an existing published app slug"), `--name` (slugified into the
// blockId when --slug is absent) and `--dir` (on `app listing`, selects the
// directory whose block.manifest.json supplies the blockId) are the three that
// are less obvious and equally load-bearing.
//
// ⚠ INVARIANT GUARD, NOT REGRESSION COVERAGE. Every arm is already refused at
// origin/main — the old classifier collected every non-flag token and every
// `--opt=value` value, so it caught these by accident of being indiscriminate.
// That is exactly why they need pinning now: the fix removes the accident.
func TestDogfoodSlugBearingFlagsStillRefuseAForeignApp(t *testing.T) {
	credPath, _ := credentialFile(t)
	for _, flag := range []string{"--slug", "--from", "--name", "--dir"} {
		for _, spelling := range []string{flag + " some-other-app", flag + "=some-other-app"} {
			t.Run(spelling, func(t *testing.T) {
				cmd := "civitai app create ab-thing " + spelling
				tr := runFakeTrial(t, []string{cmd}, "",
					"--credential-file", credPath, "--app-prefix", "ab-")
				body := readFile(t, tr.transcript)
				got := stepVerdicts(t, body)
				if len(got) != 1 || got[0] != "refused" {
					t.Fatalf("%q reached the container (verdicts %v).\n%s carries an app identity, so "+
						"narrowing the classifier to a flag ALLOWLIST must keep it in the allowlist — "+
						"otherwise this is #698's bypass again, under a different flag name.\n%s",
						cmd, got, flag, body)
				}
				if !strings.Contains(body, "some-other-app") || !strings.Contains(body, "ab-") {
					t.Errorf("refused for the wrong reason — the refusal names neither the foreign slug "+
						"nor the prefix, so this is not the prefix cap firing:\n%s", body)
				}
			})
		}
	}
}

// ── the seam guard: the ledger against the CLI ───────────────────────────────

// gatedFlagSourceFiles are the `internal/cmd` files declaring flags on the
// commands `_prefix_ok` gates. `app_listing*.go` is globbed so a new listing
// subcommand file is picked up without editing this list.
var gatedFlagSourceGlobs = []string{
	"internal/cmd/app_create.go",
	"internal/cmd/app_init.go",
	"internal/cmd/app_submit.go",
	"internal/cmd/app_listing*.go",
}

// flagDeclRe reads a cobra flag declaration: the binder method, the flag name,
// and (for a `…VarP`) the shorthand. The method name is captured because
// `BoolVar`/`BoolVarP` are the ones that consume NOTHING, which is the fact the
// classifier's parse turns on.
var flagDeclRe = regexp.MustCompile(`(?:Flags|PersistentFlags)\(\)\.([A-Za-z]+VarP?)\(\s*[^,]*,\s*"([^"]*)"(?:\s*,\s*"([^"]*)")?`)

// TestDogfoodGatedFlagLedgerCoversTheCLI is the seam guard for the narrowing.
//
// 🔴 WHY A SEAM GUARD AND NOT A COMMENT. The classifier now decides which tokens
// can name an app from a table in runner.py, and the authority for that table is
// the cobra source in `internal/cmd` — a different language, a different
// directory, no import between them. If the CLI gains a slug-bearing flag
// (`--app`, `--block`, `--app-slug`) and nobody adds it to `GATED_FLAGS`, the
// classifier reads it as a value-less boolean, collects nothing for it, and the
// app-prefix cap silently stops covering it. Nothing fails, nothing logs, and the
// next credentialed trial can point that flag anywhere on the account.
//
// So this derives the flag set from the source and requires the ledger to match
// it in BOTH directions. A hardcoded list on each side would be two copies of one
// assumption; this makes the CLI the authority and the ledger the claim.
//
// ⚠ NOT REGRESSION COVERAGE FOR ANY OF THE THREE DEFECTS, and deliberately not
// counted as such: `GATED_FLAGS` does not exist at origin/main, so at base this
// dies reporting the ledger is unreadable — a DIFFERENT failure from the one it
// tests. It was validated by MUTATION instead (see the PR body): adding a
// `StringVar(&x, "app-slug", …)` to `internal/cmd/app_create.go` turns it red
// naming `app-slug`, and deleting a row from `GATED_FLAGS` turns it red naming
// that row.
func TestDogfoodGatedFlagLedgerCoversTheCLI(t *testing.T) {
	// Tests in this package run with cwd = the repo root (see dogfoodDir).
	const root = "."

	// The ledger, read out of runner.py itself rather than restated here.
	ledger := gatedFlagLedger(t)
	if len(ledger) < 15 {
		t.Fatalf("CONTROL failure, not a finding: runner.py's GATED_FLAGS parsed to %d entry/entries. "+
			"The gated commands declare well over a dozen flags, so a number this low means the dump "+
			"below read the wrong thing", len(ledger))
	}

	// The authority: every flag the gated commands declare, plus root's
	// persistent flags (which every command accepts).
	type decl struct{ bool_, value bool }
	declared := map[string]*decl{}
	note := func(name string, isBool bool) {
		if name == "" {
			return
		}
		d := declared[name]
		if d == nil {
			d = &decl{}
			declared[name] = d
		}
		if isBool {
			d.bool_ = true
		} else {
			d.value = true
		}
	}

	files := map[string]bool{}
	for _, g := range gatedFlagSourceGlobs {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(g)))
		if err != nil {
			t.Fatalf("globbing %s: %v", g, err)
		}
		for _, m := range matches {
			if strings.HasSuffix(m, "_test.go") {
				continue
			}
			files[m] = true
		}
	}
	// root.go's PersistentFlags apply to the gated commands too.
	files[filepath.Join(root, "internal", "cmd", "root.go")] = true

	if len(files) < 5 {
		t.Fatalf("CONTROL failure, not a finding: resolved %d source file(s) for the gated commands, "+
			"want >= 5 — the globs are not matching, so `declared` would be empty and the ledger would "+
			"look fully covered", len(files))
	}

	persistentOnly := regexp.MustCompile(`PersistentFlags\(\)\.([A-Za-z]+VarP?)\(\s*[^,]*,\s*"([^"]*)"(?:\s*,\s*"([^"]*)")?`)
	for path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		re := flagDeclRe
		if filepath.Base(path) == "root.go" {
			// root declares flags for the ROOT command; only the persistent ones
			// reach `app …`.
			re = persistentOnly
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			isBool := strings.HasPrefix(m[1], "Bool")
			note(m[2], isBool)
			if strings.HasSuffix(m[1], "P") {
				note(m[3], isBool)
			}
		}
	}

	if len(declared) < 15 {
		t.Fatalf("CONTROL failure, not a finding: derived %d flag name(s) from %d file(s), want >= 15. "+
			"flagDeclRe is not matching the declarations, so every 'covered' verdict below is a fact "+
			"about the regex: %v", len(declared), len(files), sortedNames(declared))
	}

	// (1) The set GREW: a flag the CLI declares and the ledger does not classify.
	// This is the direction that can hide a bypass.
	for name, d := range declared {
		role, ok := ledger[name]
		if !ok {
			t.Errorf("the CLI declares `%s` on a gated command and runner.py's GATED_FLAGS does not "+
				"classify it.\n"+
				"  An unledgered flag is treated as a value-less boolean. If its value names an app, "+
				"the app-prefix cap silently stops seeing that app — #698's bypass under a new flag name.\n"+
				"  fix: add %q to GATED_FLAGS in scripts/dogfood/runner.py with role \"slug\" (its value "+
				"names or selects an app), \"value\" (it consumes a token that is not an app) or \"bool\".",
				name, name)
			continue
		}
		// (3) Role consistency with the binder type.
		switch {
		case d.value && !d.bool_ && role == "bool":
			t.Errorf("GATED_FLAGS classifies `%s` as \"bool\", but every declaration of it in the CLI "+
				"binds a VALUE. The classifier will read that value as a positional app name — the "+
				"false-refusal defect this ledger exists to prevent.", name)
		case d.bool_ && !d.value && role != "bool":
			t.Errorf("GATED_FLAGS classifies `%s` as %q, but every declaration of it in the CLI is a "+
				"Bool. The classifier will swallow the NEXT token as its value, so a real app name "+
				"after it is never checked.", name, role)
		}
		// (4) The direct slug-bearing signal: a flag whose NAME says slug must be
		// ledgered as one. This is what catches `--app-slug` / `--block-slug`
		// before anyone reasons about it.
		if strings.Contains(name, "slug") && role != "slug" {
			t.Errorf("the CLI declares `%s`, whose name says it carries a slug, and GATED_FLAGS "+
				"classifies it as %q. Its value must be a prefix-cap candidate.", name, role)
		}
	}

	// (2) The set SHRANK: a ledger row for a flag the CLI no longer declares. A
	// ledger nobody prunes becomes decoration, and a stale row is how a reviewer
	// concludes coverage exists.
	for name := range ledger {
		if declared[name] == nil {
			t.Errorf("runner.py's GATED_FLAGS classifies `%s`, which no gated command in internal/cmd "+
				"declares any more. Drop the row, or the ledger is describing a CLI that no longer "+
				"exists.", name)
		}
	}

	t.Logf("%d flag name(s) derived from %d source file(s); %d ledgered", len(declared), len(files), len(ledger))
}

// gatedFlagLedger dumps runner.py's GATED_FLAGS. Read from the module rather than
// restated here: a copy in this file would be a third place to keep in sync.
func gatedFlagLedger(t *testing.T) map[string]string {
	t.Helper()
	py := dogfoodPython(t)
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("r", sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
print(json.dumps(m.GATED_FLAGS))
`
	cmd := exec.Command(py, "-c", script, filepath.Join(dogfoodDir, "runner.py"))
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("reading GATED_FLAGS out of runner.py: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("GATED_FLAGS did not dump as a name->role map: %v\n%s", err, out)
	}
	return got
}

func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ── the parse, exercised directly ────────────────────────────────────────────

// 🔴 THE TABLE THAT PINS THE PARSE'S EDGES, run against `slug_candidates` itself
// rather than through a trial — a full trial per shape would be 40 subprocesses,
// and the edges (a bare `--`, a shorthand cluster, a flag at end-of-argv) are
// awkward to reach through a shell string.
//
// ⚠ NOT REGRESSION COVERAGE, AND NOT COUNTED AS SUCH. `slug_candidates` does not
// exist at origin/main, so at base this dies with an AttributeError — a DIFFERENT
// failure from the one it claims to test. The red-at-base evidence is the
// end-to-end tests above.
func TestDogfoodSlugCandidatesParse(t *testing.T) {
	py := dogfoodPython(t)
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("r", sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
cases = [
    # (argv, skip, want) — want is [[value, origin], …]
    (["app", "create", "ab-x", "--template", "static"], 2, [["ab-x", "argument"]]),
    (["app", "create", "ab-x", "--template=static"], 2, [["ab-x", "argument"]]),
    (["app", "create", "ab-x", "-t", "static"], 2, [["ab-x", "argument"]]),
    (["app", "create", "ab-x", "-tstatic"], 2, [["ab-x", "argument"]]),
    (["app", "create", "ab-x", "-t=static"], 2, [["ab-x", "argument"]]),
    # a bool shorthand cluster followed by a value-taking one
    (["app", "create", "ab-x", "-yt", "static"], 2, [["ab-x", "argument"]]),
    # slug flags, both spellings, are candidates
    (["app", "create", "--slug", "foo"], 2, [["foo", "--slug"]]),
    (["app", "create", "--slug=foo"], 2, [["foo", "--slug"]]),
    (["app", "create", "--from", "foo"], 2, [["foo", "--from"]]),
    (["app", "create", "--name", "foo"], 2, [["foo", "--name"]]),
    (["app", "listing", "status", "--dir=foo"], 3, [["foo", "--dir"]]),
    # an empty attached value contributes nothing
    (["app", "create", "--slug="], 2, []),
    # a slug flag at end-of-argv has no value to take
    (["app", "create", "--slug"], 2, []),
    # a bool flag does NOT consume the next token
    (["app", "create", "--yes", "ab-x"], 2, [["ab-x", "argument"]]),
    (["app", "submit", "--package-only", "ab-x"], 2, [["ab-x", "argument"]]),
    # an UNKNOWN flag is assumed to consume nothing: the next token stays a
    # positional candidate, which over-refuses rather than under-collecting.
    (["app", "create", "--brand-new-flag", "foo"], 2, [["foo", "argument"]]),
    (["app", "create", "--brand-new-flag=foo"], 2, []),
    # everything after a bare -- is positional
    (["app", "create", "--", "--slug", "foo"], 2, [["--slug", "argument"], ["foo", "argument"]]),
    # a lone dash is a positional, not a flag
    (["app", "create", "-"], 2, [["-", "argument"]]),
    # several candidates, in order
    (["app", "create", "ab-x", "--slug", "foo", "--template", "static", "bar"], 2,
     [["ab-x", "argument"], ["foo", "--slug"], ["bar", "argument"]]),
]
bad = []
for argv, skip, want in cases:
    got = [list(p) for p in m.slug_candidates(argv, skip)]
    if got != want:
        bad.append("slug_candidates(%r, %d) = %r, want %r" % (argv, skip, got, want))
print(json.dumps(bad))
`
	cmd := exec.Command(py, "-c", script, filepath.Join(dogfoodDir, "runner.py"))
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exercising slug_candidates: %v\n%s", err, out)
	}
	var bad []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &bad); err != nil {
		t.Fatalf("the slug_candidates table did not report: %v\n%s", err, out)
	}
	if len(bad) != 0 {
		t.Fatalf("slug_candidates disagrees with the contract:\n  %s", strings.Join(bad, "\n  "))
	}
}

// 🔴 THE HEREDOC STRIPPER'S OWN TABLE, with a NEGATIVE CONTROL. A stripper that
// returned the empty string would satisfy every "must run" arm above; a stripper
// that returned its input unchanged would satisfy every "must be refused" arm.
// Only a table pins both.
//
// ⚠ Same caveat: `strip_heredoc_bodies` does not exist at base, so this dies with
// an AttributeError there rather than failing the claim it makes.
func TestDogfoodHeredocStripperTable(t *testing.T) {
	py := dogfoodPython(t)
	const script = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("r", sys.argv[1])
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
cases = [
    # the BODY and its terminator go (neither is command text); the opener line
    # and everything after the terminator stay
    ("cat > f << 'EOF'\nbody line\nEOF\necho after",
     "cat > f << 'EOF'\necho after"),
    ("cat > f << EOF\nbody\nEOF", "cat > f << EOF"),
    ('cat > f << "EOF"\nbody\nEOF', 'cat > f << "EOF"'),
    # <<- strips leading tabs from the terminator
    ("cat > f <<- EOF\n\tbody\n\tEOF\necho after",
     "cat > f <<- EOF\necho after"),
    # a herestring is NOT a heredoc: nothing may be dropped
    ('grep x <<< "civitai generate"\necho after',
     'grep x <<< "civitai generate"\necho after'),
    # no heredoc at all: byte-identical passthrough
    ("civitai app submit --yes\necho done", "civitai app submit --yes\necho done"),
    # unterminated: the body runs to EOF, exactly as bash treats it
    ("cat > f << EOF\neval \"civitai app submit\"", "cat > f << EOF"),
    # two heredocs on one line are consumed in order
    ("cmd << A << B\nbody a\nA\nbody b\nB\necho after",
     "cmd << A << B\necho after"),
]
bad = []
for src, want in cases:
    got = m.strip_heredoc_bodies(src)
    if got != want:
        bad.append("strip_heredoc_bodies(%r) = %r, want %r" % (src, got, want))
print(json.dumps(bad))
`
	cmd := exec.Command(py, "-c", script, filepath.Join(dogfoodDir, "runner.py"))
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exercising strip_heredoc_bodies: %v\n%s", err, out)
	}
	var bad []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &bad); err != nil {
		t.Fatalf("the heredoc-stripper table did not report: %v\n%s", err, out)
	}
	if len(bad) != 0 {
		t.Fatalf("strip_heredoc_bodies disagrees with the contract:\n  %s", strings.Join(bad, "\n  "))
	}
}
