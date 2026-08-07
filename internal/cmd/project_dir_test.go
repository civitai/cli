package cmd

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #256. `civitai app validate /nope` reported a path that is not there as
// "block.manifest.json not found at project root /nope" and exited 1 — a
// validation FINDING about a directory nobody has. The published contract says a
// path that does not exist is a mistake about the invocation and exits 2, and
// `civitai generate --input /nope/x.json` has honoured that since #251.
//
// 🔴 EVERY ROW PINS THE CLASSIFICATION WITH errors.Is, NEVER MESSAGE TEXT
// (AGENTS item 7). The exit code is carried by the ErrUsage sentinel, which has
// no visible text of its own: `asUsageError` preserves the message byte for
// byte. So a test that asserts on wording says NOTHING about `echo $?` — that is
// the measured failure #251 recorded, where a one-token classification change
// moved a command's exit code through a fully green suite.
//
// The table is deliberately BOTH directions. The two exit-2 rows are the
// regression; the two control rows are what stop the obvious over-fix, which is
// to tag every path failure and make `app validate` report a genuinely broken
// project as a usage error. A build that tagged everything passes the first half
// and fails the second.

// projectDirCase is one path shape and the classification it must get.
type projectDirCase struct {
	name string
	// dir is built from the temp root the harness prepares.
	dir func(root string) string
	// wantUsage is the CLASSIFICATION under test: true means ErrUsage (exit 2).
	wantUsage bool
	// wantErr is whether the command must fail at all. A valid project fails
	// with neither.
	wantErr bool
	// why records what breaks if this row's answer changes.
	why string
}

// projectDirCases is shared by the `app validate` and `app submit` tables — the
// gate is ONE helper (resolveProjectDir), so the two commands must answer
// identically, and running the same rows through both is what makes that
// observable rather than assumed.
func projectDirCases() []projectDirCase {
	return []projectDirCase{
		{
			name:      "path does not exist",
			dir:       func(root string) string { return filepath.Join(root, "does", "not", "exist") },
			wantUsage: true,
			wantErr:   true,
			why: "the headline defect: it exited 1 and blamed a missing manifest, so a script could not " +
				"tell a typo'd path from an app that genuinely fails validation",
		},
		{
			name:      "path is a regular file",
			dir:       func(root string) string { return filepath.Join(root, "notadir.txt") },
			wantUsage: true,
			wantErr:   true,
			why: "it fell through to the raw syscall and printed `stat notadir.txt/block.manifest.json: " +
				"not a directory` — a path the user never named, on exit 1",
		},
		{
			name:      "CONTROL: a real directory with no manifest",
			dir:       func(root string) string { return filepath.Join(root, "empty") },
			wantUsage: false,
			wantErr:   true,
			why: "a validation VERDICT, not a usage error: the user pointed at a real place, so the " +
				"invocation was right and the project is wrong. Tagging this would make the fix " +
				"indistinguishable from `tag every path failure`",
		},
		{
			name:      "CONTROL: a valid project",
			dir:       func(root string) string { return filepath.Join(root, "ok") },
			wantUsage: false,
			wantErr:   false,
			why:       "the gate must not refuse the case both commands exist to serve",
		},
	}
}

// newProjectDirRoot builds the fixture tree every row above indexes into.
func newProjectDirRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "notadir.txt"), []byte("not a project"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(root, "ok")
	if err := os.Mkdir(ok, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStaticManifest(t, ok)
	if err := os.WriteFile(filepath.Join(ok, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestProjectDirExitCodes drives the REAL commands through NewRootCmd, which is
// the only way the assertion covers the wiring as well as the helper: a
// resolveProjectDir that is correct and never called would pass a unit test and
// fail every row here.
func TestProjectDirExitCodes(t *testing.T) {
	for _, cmdName := range []string{"validate", "submit"} {
		t.Run(cmdName, func(t *testing.T) {
			for _, c := range projectDirCases() {
				t.Run(c.name, func(t *testing.T) {
					root := newProjectDirRoot(t)
					dir := c.dir(root)

					args := []string{"app", cmdName, dir}
					if cmdName == "submit" {
						// --package-only never submits and needs no token, so
						// the valid-project row exercises the gate rather than
						// the network.
						args = append(args, "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))
					}
					_, stderr, err := run(t, args...)

					if !c.wantErr {
						if err != nil {
							t.Fatalf("app %s %s must succeed: %v\n%s", cmdName, c.name, err, stderr)
						}
						return
					}
					if err == nil {
						t.Fatalf("app %s must fail for %s (%s)", cmdName, c.name, c.why)
					}
					if got := errors.Is(err, ErrUsage); got != c.wantUsage {
						t.Errorf("app %s, %s: errors.Is(err, ErrUsage) = %v, want %v\nWhy it matters: %s\nerr: %v",
							cmdName, c.name, got, c.wantUsage, c.why, err)
					}
				})
			}
		})
	}
}

// TestProjectDirRefusalNamesThePathTheUserTyped is the message half, and it is
// deliberately a NEGATIVE assertion about one specific string rather than a
// positive one about wording.
//
// The old failure was not "an unhelpful message" — it was a message about a path
// the CLI ASSEMBLED. `validate.Dir` stats `<dir>/block.manifest.json`, so a
// regular file produced `stat README.md/block.manifest.json: not a directory`,
// naming a path that cannot exist and that the user never wrote. Asserting the
// joined form is ABSENT survives any rewording of the replacement; asserting the
// replacement's own text would not.
func TestProjectDirRefusalNamesThePathTheUserTyped(t *testing.T) {
	root := newProjectDirRoot(t)
	file := filepath.Join(root, "notadir.txt")
	joined := filepath.Join(file, "block.manifest.json")

	for _, cmdName := range []string{"validate", "submit"} {
		t.Run(cmdName, func(t *testing.T) {
			args := []string{"app", cmdName, file}
			if cmdName == "submit" {
				args = append(args, "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))
			}
			_, _, err := run(t, args...)
			if err == nil {
				t.Fatalf("app %s <regular file> must fail", cmdName)
			}
			if strings.Contains(err.Error(), joined) {
				t.Errorf("the error names a path the CLI assembled and the user never typed (%s):\n%v", joined, err)
			}
			if !strings.Contains(err.Error(), file) {
				t.Errorf("the error must name the path the user DID type (%s):\n%v", file, err)
			}
		})
	}
}

// TestValidateJSONEmitsNothingForARefusedPath pins the deliberate WIRE BREAK.
//
// `civitai app validate /nope --json` used to write
// {"ok":false,"dir":"/nope","errors":[…]} to stdout and exit 1 — i.e. it
// reported a path that does not exist as a validation RESULT, with a fabricated
// finding about a manifest nobody could have written. It now writes nothing and
// exits 2, matching the CLI-wide convention that a usage error emits no JSON
// object.
//
// The stdout assertion is the load-bearing half: the exit code alone is
// satisfied by a build that tags the error AND still prints the object, which
// would leave a parsing script reading a result for a path that is not there.
func TestValidateJSONEmitsNothingForARefusedPath(t *testing.T) {
	root := newProjectDirRoot(t)

	for _, tc := range []struct {
		name string
		dir  string
	}{
		{"missing path", filepath.Join(root, "nope")},
		{"regular file", filepath.Join(root, "notadir.txt")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := run(t, "app", "validate", tc.dir, "--json")
			if err == nil {
				t.Fatal("a refused path must still fail")
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("--json must not change the classification: errors.Is(err, ErrUsage) = false\nerr: %v", err)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Errorf("a usage error must emit no JSON object; stdout was:\n%s", stdout)
			}
		})
	}

	// POSITIVE CONTROL: --json still emits a real object for a path the gate
	// ACCEPTS. Without it, "stdout is empty" is equally satisfied by a --json
	// mode that never prints anything at all.
	stdout, _, err := run(t, "app", "validate", filepath.Join(root, "empty"), "--json")
	if err == nil {
		t.Fatal("a directory with no manifest must still fail validation")
	}
	if errors.Is(err, ErrUsage) {
		t.Error("a directory with no manifest is a validation verdict, not a usage error")
	}
	var payload map[string]any
	if uerr := json.Unmarshal([]byte(stdout), &payload); uerr != nil {
		t.Fatalf("--json must still emit the result object for a real directory: %v\nstdout: %s", uerr, stdout)
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Errorf("the result object should report ok:false: %s", stdout)
	}
}

// TestResolveProjectDirClassification exercises the helper directly, including
// the shapes the command-level table cannot reach portably.
func TestResolveProjectDirClassification(t *testing.T) {
	root := newProjectDirRoot(t)

	t.Run("nonexistent is a usage error", func(t *testing.T) {
		err := resolveProjectDir(filepath.Join(root, "nope"))
		if !errors.Is(err, ErrUsage) {
			t.Errorf("want ErrUsage, got %v", err)
		}
	})
	t.Run("regular file is a usage error", func(t *testing.T) {
		err := resolveProjectDir(filepath.Join(root, "notadir.txt"))
		if !errors.Is(err, ErrUsage) {
			t.Errorf("want ErrUsage, got %v", err)
		}
	})
	t.Run("a directory passes", func(t *testing.T) {
		if err := resolveProjectDir(filepath.Join(root, "empty")); err != nil {
			t.Errorf("a real directory must pass the gate, got %v", err)
		}
	})
	t.Run("CONTROL: the default `.` passes", func(t *testing.T) {
		// Both commands default dir to "." when given no argument. A gate that
		// refused it would break every bare `civitai app validate`.
		if err := resolveProjectDir("."); err != nil {
			t.Errorf("the default project dir must pass the gate, got %v", err)
		}
	})
	t.Run("CONTROL: ENOTDIR below a file is NOT a usage error", func(t *testing.T) {
		// `app validate <regular-file>/x.json` is one of the six invocations
		// measured in issue #241, and exit 1 (generic/filesystem) is the answer
		// that issue settled on. It must not silently become a 2 here.
		//
		// 🔴 This row ASSERTS ITS OWN PREMISE. It used to be the ONLY leaf
		// subtest the "tag every stat failure" widening reddened, and it carries
		// a t.Skip — so a fixture that quietly stopped reaching the untagged arm
		// would have taken the whole battery with it, silently. The wider,
		// multi-shape battery now lives in project_dir_gate_test.go; this row
		// stays because the helper-level assertion is cheap, but it no longer
		// stands alone and it no longer skips for the wrong reason.
		path := filepath.Join(root, "notadir.txt", "sub")
		if _, serr := os.Stat(path); serr == nil {
			t.Skip("this filesystem resolves a path below a regular file")
		} else if os.IsNotExist(serr) {
			t.Fatalf("PREMISE BROKEN: %s stats as ENOENT (%v) — this row would then be exercising the "+
				"exit-2 arm and would stay green under the widening mutant", path, serr)
		}
		err := resolveProjectDir(path)
		if err == nil {
			t.Fatal("the gate must surface the stat failure")
		}
		if errors.Is(err, ErrUsage) {
			t.Errorf("a stat failure that is neither ENOENT nor a non-directory must stay untagged (exit 1): %v", err)
		}
	})
}

// TestEveryValidateDirCallerGatesOnResolveProjectDir is the SEAM guard, and it
// is the reason the branch is one shared helper rather than two copies.
//
// AGENTS.md's "one rule, one place": a predicate open-coded at N sites is
// typically wrong at N−1 of them. This defect WAS that shape — `app submit` had
// the identical hole and nothing connected the two, exactly as the four copies
// of item 24's transport predicate were found one at a time. So the ledger is
// asserted structurally: the set of files calling `validate.Dir` must equal the
// set of files calling `resolveProjectDir`.
//
// 🔴 It fails when the set GROWS (a third command validates a user-named
// directory without the gate) AND when it SHRINKS (someone deletes a gate, which
// would otherwise leave this file green and the ledger a false map).
//
// It deliberately does NOT cover `validate.ManifestOnly`: `app init` self-checks
// a directory it just created, so there is no user-named path to classify.
func TestEveryValidateDirCallerGatesOnResolveProjectDir(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	validateDirFiles := map[string]bool{}
	gateFiles := map[string]bool{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				// validate.Dir(...) — a package-qualified call.
				pkg, ok := fn.X.(*ast.Ident)
				if ok && pkg.Name == "validate" && fn.Sel.Name == "Dir" {
					validateDirFiles[name] = true
				}
			case *ast.Ident:
				if fn.Name == "resolveProjectDir" {
					gateFiles[name] = true
				}
			}
			return true
		})
	}

	// Positive control: the scanner must have found the sites at all. A pass
	// built on two empty sets is a guard wired to nothing.
	if len(validateDirFiles) < 2 {
		t.Fatalf("found validate.Dir in %d file(s) (%v) — expected at least app_validate.go and app_submit.go; the scanner is looking at the wrong tree",
			len(validateDirFiles), keysOf(validateDirFiles))
	}

	for f := range validateDirFiles {
		if !gateFiles[f] {
			t.Errorf("%s calls validate.Dir on a user-named path but never calls resolveProjectDir.\n"+
				"That is issue #256 regenerated at a new call site: a path that does not exist would be "+
				"reported as a project without a manifest and exit 1 instead of 2.", f)
		}
	}
	for f := range gateFiles {
		if !validateDirFiles[f] {
			t.Errorf("%s calls resolveProjectDir but no longer calls validate.Dir — the ledger in this test "+
				"has gone stale. Either the gate moved (update this guard) or a validate.Dir call was "+
				"dropped (which is a behaviour change worth stating).", f)
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
