package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Issue #931: TestSetTokenPersists set XDG_CONFIG_HOME and nothing else, but
// Dir() is os.UserConfigDir(), which consults XDG_CONFIG_HOME on Linux ONLY —
// macOS reads $HOME/Library/Application Support and Windows %AppData%. So on a
// Mac the test overwrote the developer's real saved token with "saved-token"
// and passed. The same XDG-only pattern was open-coded in ~240 places across
// four packages.
//
// The fix is structural, not per-site: every package whose test binary can reach
// Dir() runs its tests under configtest.Run, which points HOME, USERPROFILE,
// APPDATA, LOCALAPPDATA and the XDG roots at a temp tree before any test starts.
//
// 🔴 ON LINUX CI THE ORIGINAL OVERWRITE IS NOT OBSERVABLE — XDG is honoured
// there, so the old tests were already isolated on the only OS CI runs. What
// these two tests pin is the MECHANISM, observably on every OS: the env each
// OS's UserConfigDir reads is test-owned (red at origin/main because APPDATA is
// unset and HOME is the launching user's), and every package that can reach
// Dir() installs it (red at origin/main because none did).

// TestEveryOSConfigDirIsTestOwned evaluates, from THIS process's environment,
// the base directory os.UserConfigDir would return on each OS — not only the one
// running — and requires every one of them to be inside the temp dir.
func TestEveryOSConfigDirIsTestOwned(t *testing.T) {
	tmp := realPath(t, os.TempDir())
	xdgOrHome := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(xdgOrHome) {
		xdgOrHome = join(os.Getenv("HOME"), ".config")
	}
	bases := map[string]string{
		"linux":   xdgOrHome,
		"darwin":  join(os.Getenv("HOME"), "Library", "Application Support"),
		"windows": os.Getenv("APPDATA"),
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		base := bases[goos]
		if base == "" {
			t.Errorf("%s: the variable its UserConfigDir reads is unset, so a test there resolves the "+
				"developer's real config dir (or errors)", goos)
			continue
		}
		if !within(realPath(t, base), tmp) {
			t.Errorf("%s: UserConfigDir would be %s, outside the test temp dir %s — a config test "+
				"there writes the developer's real token", goos, base, tmp)
		}
	}
	if d, err := Dir(); err != nil || !within(realPath(t, d), tmp) {
		t.Errorf("Dir() = %q (%v), want a path under %s", d, err, tmp)
	}
}

// configReachers is the asserted ledger of module packages whose test binary
// links internal/config (directly or transitively). It fails when the set GROWS
// (a new package can now write the real config — give it configtest.Run) and when
// it SHRINKS (the ledger is stale).
var configReachers = []string{
	"cmd/civitai",
	"internal/auth",
	"internal/cmd",
	"internal/config",
}

// TestEveryConfigReachingPackageIsolatesHome walks the module, builds the
// intra-module import graph from source (test files included), and requires
// every package that can reach internal/config to declare a TestMain that calls
// configtest.Run.
func TestEveryConfigReachingPackageIsolatesHome(t *testing.T) {
	root := moduleRoot(t)
	const modPrefix = "github.com/civitai/cli/"
	imports := map[string]map[string]bool{} // pkg dir -> imported pkg dirs
	testMainRunsConfigtest := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			n := d.Name()
			if path != root && (n == "testdata" || n == "node_modules" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil // not a Go source this module builds
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		rel = filepath.ToSlash(rel)
		if imports[rel] == nil {
			imports[rel] = map[string]bool{}
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, modPrefix) {
				imports[rel][strings.TrimPrefix(p, modPrefix)] = true
			}
		}
		if strings.HasSuffix(path, "_test.go") && callsConfigtestRunFromTestMain(f) {
			testMainRunsConfigtest[rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	// Positive control: the walk saw the package this test lives in.
	if _, ok := imports["internal/config"]; !ok || len(imports) < 10 {
		t.Fatalf("module walk found %d packages and not internal/config — the instrument is broken", len(imports))
	}

	reaches := map[string]bool{"internal/config": true}
	for changed := true; changed; {
		changed = false
		for pkg, deps := range imports {
			if reaches[pkg] {
				continue
			}
			for dep := range deps {
				if reaches[dep] {
					reaches[pkg], changed = true, true
					break
				}
			}
		}
	}
	var got []string
	for pkg := range reaches {
		got = append(got, pkg)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(configReachers, ",") {
		t.Errorf("packages whose tests can reach config.Dir():\n got: %v\nwant: %v\n"+
			"a new one must run its tests under configtest.Run (then add it here)", got, configReachers)
	}
	for _, pkg := range got {
		if !testMainRunsConfigtest[pkg] {
			t.Errorf("%s can reach config.Dir() but has no TestMain calling configtest.Run — on macOS "+
				"and Windows its tests read and write the developer's real config", pkg)
		}
	}
}

func callsConfigtestRunFromTestMain(f *ast.File) bool {
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "TestMain" || fn.Recv != nil || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "configtest" {
						found = true
					}
				}
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}
		dir = parent
	}
}

func join(base string, elem ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, elem...)...)
}

// realPath resolves symlinks on the longest existing prefix (macOS's /var ->
// /private/var), so a path is compared with the temp dir in one spelling.
func realPath(t *testing.T, p string) string {
	t.Helper()
	p = filepath.Clean(p)
	var tail []string
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(append([]string{r}, tail...)...)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(append([]string{p}, tail...)...)
		}
		tail = append([]string{filepath.Base(p)}, tail...)
		p = parent
	}
}

func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
