package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/manifest"
)

// This file is the second half of the #256 guards, added after an audit found
// three ways the first half could go green while the contract was broken.
// project_dir_test.go pins the two exit-2 rows and the two verdict controls;
// everything here pins a clause that was STATED and unheld:
//
//  1. the gate runs AHEAD of --skip-validate (TestSubmitGateRunsBeforeSkipValidate),
//  2. the untagged stat arm stays untagged, on more than one row and more than
//     one surface (TestStatFailuresBelowTheGateStayUntagged),
//  3. `--json` publishes a result only when validation produced one
//     (TestValidateJSONOnlyEmitsAResultItActuallyProduced),
//  4. the paths the exit-code contract does NOT cover really are not covered
//     (TestUngatedPathFlagsAreNotUsageErrors).
//
// 🔴 EVERY CLASSIFICATION ASSERTION IS errors.Is, NEVER MESSAGE TEXT (AGENTS
// item 7): ErrUsage carries no visible wording of its own, so an assertion on
// the string says nothing about `echo $?`.

// euid0 reports whether this process can defeat mode bits, which makes every
// permission-denied fixture in this file unbuildable.
func euid0() bool { return os.Geteuid() == 0 }

// TestSubmitGateRunsBeforeSkipValidate pins the ORDERING clause in
// app_submit.go's own comment, which had no test at all.
//
// 🔴 Measured: moving `resolveProjectDir` inside the `if !skipValidate` block —
// a one-line move that reads like a tidy-up — left the ENTIRE suite green while
// reverting `app submit <nonexistent> --skip-validate` from exit 2 to exit 1.
// Only one test mentioned --skip-validate at all, and it used a VALID directory,
// so nothing observed the flag's interaction with the gate.
//
// The clause is not a nicety: `--skip-validate` waives our opinion of the
// MANIFEST. It cannot waive the question of whether the directory the user typed
// exists, because there is no manifest to have an opinion about.
func TestSubmitGateRunsBeforeSkipValidate(t *testing.T) {
	root := newProjectDirRoot(t)

	for _, tc := range []struct {
		name      string
		dir       string
		wantUsage bool
		wantErr   bool
		why       string
	}{
		{
			name:      "nonexistent path",
			dir:       filepath.Join(root, "does", "not", "exist"),
			wantUsage: true,
			wantErr:   true,
			why: "the regression the audit built: with the gate moved inside the !skipValidate block " +
				"this falls through to manifest.Load and comes back untagged, i.e. exit 1 — the exact " +
				"collapse issue #256 fixed, re-opened by a flag",
		},
		{
			name:      "regular file",
			dir:       filepath.Join(root, "notadir.txt"),
			wantUsage: true,
			wantErr:   true,
			why:       "same shape, second arm of the gate: a file is a mistake about the invocation, not a project",
		},
		{
			name:      "CONTROL: a real directory with no manifest",
			dir:       filepath.Join(root, "empty"),
			wantUsage: false,
			wantErr:   true,
			why: "the gate must let a real directory through even under --skip-validate. Without this row " +
				"a build that tagged EVERY --skip-validate failure would pass the two rows above",
		},
		{
			name:      "CONTROL: a valid project still packages",
			dir:       filepath.Join(root, "ok"),
			wantUsage: false,
			wantErr:   false,
			why:       "--skip-validate must still do the thing it exists for",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, err := run(t, "app", "submit", tc.dir,
				"--skip-validate", "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("app submit --skip-validate %s must succeed: %v\n%s", tc.name, err, stderr)
				}
				return
			}
			if err == nil {
				t.Fatalf("app submit --skip-validate must fail for %s (%s)", tc.name, tc.why)
			}
			if got := errors.Is(err, ErrUsage); got != tc.wantUsage {
				t.Errorf("app submit --skip-validate, %s: errors.Is(err, ErrUsage) = %v, want %v\nWhy it matters: %s\nerr: %v",
					tc.name, got, tc.wantUsage, tc.why, err)
			}
		})
	}
}

// statFailureCase is a path whose os.Stat fails with something that is NEITHER
// ENOENT nor "it is there and is not a directory" — the third arm of
// resolveProjectDir, which returns the error UNTAGGED so it exits 1.
type statFailureCase struct {
	name string
	// build returns the path to hand the gate, or "" to skip (with reason).
	build func(t *testing.T, root string) (path, skip string)
	why   string
}

func statFailureCases() []statFailureCase {
	return []statFailureCase{
		{
			name: "ENOTDIR below a regular file",
			build: func(t *testing.T, root string) (string, string) {
				return filepath.Join(root, "notadir.txt", "sub"), ""
			},
			why: "`app validate <regular-file>/x.json` is one of the six invocations measured in issue " +
				"#241, and exit 1 (generic/filesystem) is the answer that issue settled on",
		},
		{
			name: "EACCES on an unsearchable parent",
			build: func(t *testing.T, root string) (string, string) {
				if euid0() {
					return "", "running as root: mode bits do not deny this process"
				}
				parent := filepath.Join(root, "locked")
				if err := os.Mkdir(parent, 0o755); err != nil {
					t.Fatal(err)
				}
				child := filepath.Join(parent, "proj")
				if err := os.Mkdir(child, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(parent, 0o000); err != nil {
					t.Fatal(err)
				}
				// t.TempDir's own cleanup cannot walk a 000 directory.
				t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
				return child, ""
			},
			why: "a permissions failure on the way to the project root is a filesystem problem, not a " +
				"malformed invocation — and it is a SECOND shape, so a mutant that only survives on " +
				"ENOTDIR cannot hide behind one row",
		},
	}
}

// TestStatFailuresBelowTheGateStayUntagged is the F6 repair: the widening
// mutant — turning resolveProjectDir's untagged arm into `asUsageError` — used
// to redden EXACTLY ONE leaf subtest, and that subtest could `t.Skip` itself on
// a filesystem that resolves a path below a regular file. A battery resting on
// one skippable row is AGENTS item 24's recorded failure shape.
//
// So: two independent stat shapes × three surfaces (the helper, `app validate`,
// `app submit`), each row asserting its OWN PREMISE — an independent os.Stat
// must fail with something that is neither ENOENT nor a live non-directory —
// so a row whose fixture quietly stops reaching the untagged arm FAILS instead
// of continuing to look like coverage. Plus a count floor.
func TestStatFailuresBelowTheGateStayUntagged(t *testing.T) {
	var reached int

	for _, c := range statFailureCases() {
		t.Run(c.name, func(t *testing.T) {
			root := newProjectDirRoot(t)
			path, skip := c.build(t, root)
			if skip != "" {
				t.Skip(skip)
			}

			// PREMISE, asserted independently of the implementation: this
			// fixture must actually reach the third arm. If os.Stat succeeds,
			// or fails with ENOENT, the row is exercising a different branch
			// and its green says nothing about the untagged one.
			info, serr := os.Stat(path)
			if serr == nil {
				t.Skipf("this filesystem resolves %s (mode %v) — the untagged arm is unreachable here", path, info.Mode())
			}
			if os.IsNotExist(serr) {
				t.Fatalf("PREMISE BROKEN: %s stats as ENOENT (%v), so this row exercises the exit-2 arm, "+
					"not the untagged one. It would stay green under the widening mutant and is not evidence.", path, serr)
			}
			reached++

			t.Run("resolveProjectDir", func(t *testing.T) {
				err := resolveProjectDir(path)
				if err == nil {
					t.Fatal("the gate must surface the stat failure")
				}
				if errors.Is(err, ErrUsage) {
					t.Errorf("a stat failure that is neither ENOENT nor a non-directory must stay UNTAGGED (exit 1).\n"+
						"Why: %s\nerr: %v", c.why, err)
				}
			})

			for _, cmdName := range []string{"validate", "submit"} {
				t.Run("app "+cmdName, func(t *testing.T) {
					args := []string{"app", cmdName, path}
					if cmdName == "submit" {
						args = append(args, "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))
					}
					_, _, err := run(t, args...)
					if err == nil {
						t.Fatalf("app %s must fail for %s", cmdName, c.name)
					}
					if errors.Is(err, ErrUsage) {
						t.Errorf("app %s classified a filesystem stat failure as a USAGE error (exit 2).\n"+
							"Why that is wrong: %s\nerr: %v", cmdName, c.why, err)
					}
				})
			}
		})
	}

	// COUNT FLOOR. A silent skip on every row is indistinguishable from a
	// battery wired to nothing, which is precisely how the single-row version
	// of this guard could have gone quiet.
	if reached < 1 {
		t.Fatalf("no stat-failure fixture reached the untagged arm (%d of %d rows) — this battery is proving nothing",
			reached, len(statFailureCases()))
	}
	if testing.Short() {
		return
	}
	if reached < 2 {
		t.Errorf("only %d of %d stat-failure shapes ran; the whole point of this file is that the widening "+
			"mutant must not rest on a single row", reached, len(statFailureCases()))
	}
}

// TestProjectDirStatErrorDoesNotStutter is F5.
//
// os.Stat returns an *fs.PathError whose Error() ALREADY begins `stat <path>: `,
// so the `fmt.Errorf("stat %s: %w", dir, err)` wrapper printed the op and the
// path twice: measured on the PR tip,
// `Error: stat …/file.txt/x.json: stat …/file.txt/x.json: not a directory`,
// where the base binary printed one `stat`.
//
// The assertion COUNTS occurrences rather than matching a golden string — the
// defect is a duplicate, and a duplicate is a count.
func TestProjectDirStatErrorDoesNotStutter(t *testing.T) {
	root := newProjectDirRoot(t)
	path := filepath.Join(root, "notadir.txt", "sub")
	if _, err := os.Stat(path); err == nil || os.IsNotExist(err) {
		t.Skip("this filesystem does not produce ENOTDIR below a regular file")
	}

	err := resolveProjectDir(path)
	if err == nil {
		t.Fatal("the gate must surface the stat failure")
	}
	msg := err.Error()
	if n := strings.Count(msg, "stat "); n != 1 {
		t.Errorf("the message says %q %d time(s), want 1 — os.Stat's own *fs.PathError already carries it:\n%s",
			"stat ", n, msg)
	}
	if n := strings.Count(msg, path); n != 1 {
		t.Errorf("the message names %s %d time(s), want 1:\n%s", path, n, msg)
	}
	// It must still name the path the USER typed — the whole point of the gate
	// is that the base binary named the JOINED manifest path instead.
	if !strings.Contains(msg, path) {
		t.Errorf("the message must name the path the user typed (%s):\n%s", path, msg)
	}
}

// TestValidateJSONOnlyEmitsAResultItActuallyProduced is F2: the published code-1
// note used to promise `app validate --json` "prints the full result … so a
// script never has to read stderr", full stop.
//
// 🔴 Measured on the PR tip: `civitai app validate <unreadable-dir> --json`
// exits 1 with stdout EMPTY (0 bytes) and the error on stderr only, because
// validate.Dir returns an `error` rather than a Result for a non-ENOENT stat
// failure. The note is now scoped to "a project directory it could READ", and
// this test is the behaviour that scoping describes.
//
// The positive control is what makes the empty-stdout row mean anything: a
// --json mode that printed nothing ever would satisfy the first half alone.
func TestValidateJSONOnlyEmitsAResultItActuallyProduced(t *testing.T) {
	if euid0() {
		t.Skip("running as root: mode bits do not deny this process")
	}
	root := newProjectDirRoot(t)

	// A real, well-formed project whose manifest cannot be READ. The directory
	// itself stats fine, so it passes the gate; validate.Dir then fails on the
	// manifest and returns an error instead of a Result.
	unreadable := filepath.Join(root, "unreadable")
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStaticManifest(t, unreadable)
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	stdout, stderr, err := run(t, "app", "validate", unreadable, "--json")
	if err == nil {
		t.Fatal("an unreadable project must fail")
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("an unreadable project directory is a FILESYSTEM failure (exit 1), not a usage error (exit 2):\n%v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("validation produced no result, so --json must emit no object.\n"+
			"If this now prints one, the code-1 note in exitcodes_doc.go is under-scoped again — "+
			"re-widen it rather than deleting this test.\nstdout:\n%s", stdout)
	}
	if strings.TrimSpace(stderr) == "" && err.Error() == "" {
		t.Error("the failure has to be reported somewhere")
	}

	// POSITIVE CONTROL: for a directory it CAN read, the object is still there
	// and `ok` still carries the verdict — the half of the claim that survives.
	stdout, _, err = run(t, "app", "validate", filepath.Join(root, "empty"), "--json")
	if err == nil {
		t.Fatal("a directory with no manifest must still fail validation")
	}
	if errors.Is(err, ErrUsage) {
		t.Error("a directory with no manifest is a validation verdict, not a usage error")
	}
	if !strings.Contains(stdout, `"ok"`) {
		t.Errorf("--json must still emit the result object for a directory it could read:\n%s", stdout)
	}
}

// TestProjectDirRemediesMatchTheirArm pins WHICH remedy each exit-2 arm
// carries, which no errors.Is assertion can see: both arms tag ErrUsage, so
// they are indistinguishable downstream, and the pre-existing message test only
// requires the path the user typed to appear — true of both spellings.
//
// 🔴 Measured before this guard existed: exchanging the two format strings
// passed the ENTIRE suite, 0 failures. The result is advice that is exactly
// backwards — a missing path told to "pass the ROOT, not a file", a manifest
// path told to scaffold a project the author already has. AGENTS item 21(f)
// records the same class for the substitution reporter, and prescribes this
// shape: derive the expected text from the CONSTANT and require the other arm's
// to be ABSENT.
func TestProjectDirRemediesMatchTheirArm(t *testing.T) {
	root := newProjectDirRoot(t)
	missing := filepath.Join(root, "nope")
	file := filepath.Join(root, "notadir.txt")

	// The remedies must be non-empty and distinct, or every Contains below is
	// vacuous (`strings.Contains(x, "")` is always true) and the absence
	// assertions can never fire.
	noSuch := fmt.Sprintf(remedyNoSuchDir, missing)
	notDir := fmt.Sprintf(remedyNotADir, file, manifest.Filename)
	if strings.TrimSpace(noSuch) == "" || strings.TrimSpace(notDir) == "" {
		t.Fatalf("a remedy rendered empty — every assertion in this test would be vacuous\nnoSuch=%q notDir=%q", noSuch, notDir)
	}
	if noSuch == notDir {
		t.Fatal("the two remedies are identical — this guard cannot tell the arms apart, so a swap is undetectable")
	}

	for _, tc := range []struct {
		name       string
		dir        string
		want, deny string
		why        string
	}{
		{
			name: "nonexistent path gets the `app init` remedy",
			dir:  missing, want: noSuch, deny: notDir,
			why: "the path is not there, so there is no file to have pointed at — telling the user to " +
				"pass the ROOT rather than a file is advice about something that does not exist",
		},
		{
			name: "a regular file gets the project-ROOT remedy",
			dir:  file, want: notDir, deny: noSuch,
			why: "the user has a real project and pointed one level too deep (typically at the manifest) — " +
				"telling them to scaffold a new one with `app init` sends them to create what they already have",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := resolveProjectDir(tc.dir)
			if err == nil {
				t.Fatal("must be refused")
			}
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("premise: both arms must be usage errors, got %v", err)
			}
			if got := err.Error(); !strings.Contains(got, tc.want) {
				t.Errorf("wrong remedy for this arm.\nWhy it matters: %s\nwant it to contain: %s\ngot: %s", tc.why, tc.want, got)
			}
			if got := err.Error(); strings.Contains(got, tc.deny) {
				t.Errorf("this arm carries the OTHER arm's remedy — the two are swapped.\nWhy it matters: %s\ngot: %s", tc.why, got)
			}
		})
	}
}

// TestUngatedPathFlagsAreNotUsageErrors is F3: the code-2 note claimed the
// missing-vs-unreadable split was the rule for "every local path the CLI is
// handed — a flag's value and a positional argument alike". That quantifier had
// a live counterexample INSIDE its own scope, measured identical on base and on
// this branch: `civitai app listing status --dir /does/not/exist` exits 1.
//
// The sentence is now scoped to the enumerated ledger and the residual is stated
// in the README. This test is what keeps the two in step: it FAILS if the
// residual closes, which is the moment the README paragraph becomes wrong.
//
// It is deliberately NOT an argument that exiting 1 here is right. It is a
// record that the published contract does not claim otherwise.
func TestUngatedPathFlagsAreNotUsageErrors(t *testing.T) {
	root := newProjectDirRoot(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "app listing status --dir <nonexistent>",
			args: []string{"app", "listing", "status", "--dir", filepath.Join(root, "does", "not", "exist")},
		},
		{
			name: "app listing status --dir <regular file>",
			args: []string{"app", "listing", "status", "--dir", filepath.Join(root, "notadir.txt")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("%s must fail", tc.name)
			}
			if errors.Is(err, ErrUsage) {
				t.Errorf("%s now exits 2. That may well be an improvement — but the code-2 Extra note in "+
					"exitcodes_doc.go currently tells scripts this path exits 1, and the README's ledger "+
					"paragraph says the split is published only for the paths it enumerates. Bring --dir "+
					"into the ledger (and into resolveProjectDir's set) in the same change, or this test is "+
					"the only thing that noticed the docs went stale.\nerr: %v", tc.name, err)
			}
		})
	}
}
