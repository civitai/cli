package scaffold

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The page-money scaffold's OWN BUILD has to pass.
//
// 🔴 THE MEASURED DEFECT. `package.json.tmpl` declared `@testing-library/react`,
// `@testing-library/jest-dom` and `@testing-library/user-event` and NOT
// `@testing-library/dom`. `@testing-library/react`'s types re-export `screen`,
// `fireEvent` and `waitFor` from `@testing-library/dom` (two
// `from '@testing-library/dom'` lines in its `types/index.d.ts`), so the four
// shipped `*.test.tsx` files import names TypeScript can only resolve through a
// package the scaffold never declared. The template's `build` script is
// `tsc -p tsconfig.json --noEmit && vite build`, which typechecks those files, so
// the build failed:
//
//	src/App.test.tsx(1,18): error TS2305: Module '"@testing-library/react"' has no exported member 'screen'.
//	src/e2e.test.tsx(1,10): error TS2305: ... no exported member 'fireEvent'.
//	src/responsive.test.tsx(1,26): error TS2305: ... no exported member 'waitFor'.
//
// Two blind dogfood trials hit it independently (`z-ai/glm-5.3-flash`
// 2026-09-21, `deepseek-v4-pro` 2026-09-25), the second losing ~26% of its run to
// it. Every failing file carried the scaffold's creation mtime, so they were
// shipped, not authored by the agent.
//
// 🔴 IT IS AN INSTALL-SHAPE-DEPENDENT DEFECT, AND SAYING SO IS THE HONEST
// VERSION. `@testing-library/dom` is a PEER dependency of
// `@testing-library/react`, and npm >= 7 auto-installs peers — so a plain
// `npm install` on npm 11.19.0 does land it and the scaffold DOES typecheck.
// Measured both ways on 2026-09-25. What does not land it:
//   - `npm install --legacy-peer-deps` (and `--force`), which is exactly where
//     the measured trial ended up: its plain `npm install` died with npm's own
//     `Cannot read properties of null (reading 'edgesOut')` arborist crash, and
//     the agent's next move was `--legacy-peer-deps || --force`;
//   - `pnpm install` and yarn classic, which do not auto-install peers;
//   - `npm ci` from a lockfile produced by any of the above.
//
// So the claim is NOT "every scaffolded app fails to build". It is: the scaffold
// depends on a package it does not declare, and on every install path that does
// not silently repair that for it, its own `npm run build` fails out of the box.
// Declaring the dependency removes the dependence on the install shape.

// typecheckEnv gates the real guard. It needs the network (a full `npm install`)
// and ~60s, so it follows the `CIVITAI_CHECK_PUBLISHED_PINS` pattern rather than
// running in `make ci`.
const typecheckEnv = "CIVITAI_SCAFFOLD_TYPECHECK"

// TestPageMoneyScaffoldTypechecksItsOwnShippedTests is THE guard for the defect.
//
// 🔴 IT RUNS THE THING, rather than asserting a string is present in
// package.json. A string-presence assertion passes while the build still breaks —
// it cannot see a wrong version range, a peer that moved, a test file that starts
// importing from a fourth package, or `tsconfig.json` changing which files are in
// the program. This renders the template, installs, and runs the template's own
// `tsc -p tsconfig.json --noEmit`.
//
// 🔴 THE INSTALL IS DELIBERATELY `--legacy-peer-deps`. That is the install shape
// the measured trial used, and it is the one that does NOT auto-install peers —
// i.e. the one that makes this test a question about what the TEMPLATE declares
// rather than about what the host's npm happens to repair. Under a plain
// `npm install` the pre-fix template typechecks clean on npm >= 7, so a plain
// install here would be a test that cannot go red for this defect.
//
// Matrix (measured 2026-09-25, npm 11.19.0 / node v24.20.0):
//   - origin/main: `tsc` exits 2 with 7 × TS2305 — RED.
//   - HEAD:        `tsc` exits 0, and `vitest run` is 168/168 — GREEN.
func TestPageMoneyScaffoldTypechecksItsOwnShippedTests(t *testing.T) {
	if os.Getenv(typecheckEnv) != "1" {
		t.Skipf("set %s=1 to render page-money, npm install --legacy-peer-deps and run the "+
			"template's own `tsc --noEmit` (network, ~60s). NOTHING about the scaffold's build was "+
			"verified by this run.", typecheckEnv)
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("%s=1 was set and npm is not on PATH: %v. A skip here would report a verified "+
			"build having run nothing.", typecheckEnv, err)
	}

	dir := renderPageMoney(t)

	// The install. `--legacy-peer-deps` is the point — see the doc comment.
	install := exec.Command(npm, "install", "--legacy-peer-deps", "--no-audit", "--no-fund")
	install.Dir = dir
	install.Env = append(os.Environ(), "npm_config_update_notifier=false")
	if out, err := runWithTimeout(install, 10*time.Minute); err != nil {
		t.Fatalf("npm install --legacy-peer-deps failed in the rendered scaffold: %v\n%s", err, tail(out, 4000))
	}

	// 🔴 POSITIVE CONTROL FOR THE INSTRUMENT, before the verdict is read. `tsc`
	// reports 0 errors over 0 files just as happily as over 17, so confirm the
	// program it builds actually contains the test files that carried the defect.
	list := exec.Command(filepath.Join(dir, "node_modules", ".bin", "tsc"),
		"-p", filepath.Join(dir, "tsconfig.json"), "--listFiles", "--noEmit")
	list.Dir = dir
	listOut, _ := runWithTimeout(list, 5*time.Minute) // a non-zero exit is the defect, not a control failure
	for _, must := range []string{"src/App.test.tsx", "src/e2e.test.tsx", "src/responsive.test.tsx"} {
		if !strings.Contains(listOut, filepath.FromSlash(must)) {
			t.Fatalf("CONTROL failure, not a finding: tsc's program does not include %s, so a clean "+
				"verdict below would be a fact about tsconfig.json's include globs and not about the "+
				"scaffold's tests.\n%s", must, tail(listOut, 3000))
		}
	}

	// The verdict.
	check := exec.Command(filepath.Join(dir, "node_modules", ".bin", "tsc"),
		"-p", filepath.Join(dir, "tsconfig.json"), "--noEmit")
	check.Dir = dir
	out, err := runWithTimeout(check, 5*time.Minute)
	if err != nil {
		t.Fatalf("the page-money scaffold does not typecheck, so its own `npm run build` "+
			"(`tsc -p tsconfig.json --noEmit && vite build`) fails out of the box: %v\n%s\n\n"+
			"Every dependency the shipped sources import types from must be DECLARED in "+
			"package.json.tmpl — a peer dependency of another package is not a declaration, and only "+
			"npm >= 7 quietly repairs it.", err, tail(out, 4000))
	}
	t.Logf("rendered page-money typechecks clean after `npm install --legacy-peer-deps`")
}

// TestPageMoneyDeclaresTheTestingLibraryDomDependency is the OFFLINE half.
//
// ⚠ IT IS THE WEAKER CHECK AND IS LABELLED AS ONE. It reads package.json and
// asserts a declaration and a compatible range. It CANNOT see a build break: it
// would pass if `tsconfig.json` stopped including the test files, if a test file
// started importing from a fourth `@testing-library/*` package, if the declared
// range were satisfiable but the resolved copy shipped different types, or if any
// unrelated type error appeared. The guard that can see those is
// TestPageMoneyScaffoldTypechecksItsOwnShippedTests above, and it is gated on
// CIVITAI_SCAFFOLD_TYPECHECK=1 — so `make ci` alone does NOT verify that the
// scaffold builds.
//
// What it does buy: it runs everywhere, offline, in milliseconds, and it goes red
// on the exact edit that caused the outage (deleting the dependency line).
//
// Measured: RED at origin/main (no `@testing-library/dom` key at all), GREEN at HEAD.
func TestPageMoneyDeclaresTheTestingLibraryDomDependency(t *testing.T) {
	const pkg = "@testing-library/dom"
	dir := renderPageMoney(t)
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatalf("read the rendered package.json: %v", err)
	}
	var doc struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the rendered page-money package.json is not valid JSON: %v\n%s", err, raw)
	}
	// Control: the parse must have SEEN the other testing-library packages, or a
	// missing key below is a fact about the unmarshal.
	if _, ok := doc.DevDependencies["@testing-library/react"]; !ok {
		t.Fatalf("CONTROL failure, not a finding: the rendered package.json's devDependencies do not "+
			"name @testing-library/react at all (%v) — the parse is not reading the block the assertion "+
			"below is about", doc.DevDependencies)
	}

	got, ok := doc.DevDependencies[pkg]
	if !ok {
		if _, dep := doc.Dependencies[pkg]; !dep {
			t.Fatalf("the rendered page-money package.json declares neither a dependency nor a "+
				"devDependency on %s.\n"+
				"  Its shipped test files import `screen`, `fireEvent` and `waitFor` from "+
				"`@testing-library/react`, whose types RE-EXPORT them from %s — so `tsc --noEmit`, which "+
				"the `build` script runs, fails with TS2305 on every install path that does not "+
				"auto-install peer dependencies (`--legacy-peer-deps`, `--force`, pnpm, yarn classic, "+
				"`npm ci` from any lockfile they produced).\n"+
				"  devDependencies: %v", pkg, pkg, doc.DevDependencies)
		}
		got = doc.Dependencies[pkg]
	}

	// `@testing-library/react@16.x` peers on `@testing-library/dom: ^10.0.0`,
	// `jest-dom@6.x` on `>=10 <11`, `user-event@14.x` on `>=7.21.4` — all three
	// agree on the 10 line, so the pin must be inside it. A `^9` or `^11` would
	// install and then break differently.
	if !strings.HasPrefix(got, "^10.") {
		t.Errorf("page-money pins %s at %q. @testing-library/react@16's peer range is `^10.0.0` and "+
			"jest-dom@6's is `>=10 <11`, so a pin outside the 10 line resolves to types the other "+
			"packages were not built against.", pkg, got)
	}
}

// runWithTimeout runs cmd, returning its combined output. A timeout is reported
// as an error rather than hanging the suite.
func runWithTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	var sb strings.Builder
	cmd.Stdout = &sb
	cmd.Stderr = &sb
	if err := cmd.Start(); err != nil {
		return sb.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return sb.String(), err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return sb.String(), os.ErrDeadlineExceeded
	}
}

// tail keeps a failure message readable without losing the end of a long log,
// which is where a compiler puts its verdict.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "[… truncated …]\n" + s[len(s)-n:]
}
