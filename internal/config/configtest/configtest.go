// Package configtest isolates a test binary from the developer's real per-user
// directories. It is imported ONLY from TestMain functions; nothing in the
// shipped binary links it.
//
// 🔴 WHY XDG_CONFIG_HOME ALONE IS NOT ISOLATION (issue #931). config.Dir() is
// os.UserConfigDir(), which reads XDG_CONFIG_HOME on Linux only: macOS uses
// $HOME/Library/Application Support and Windows %AppData%. Hundreds of tests set
// XDG_CONFIG_HOME and nothing else, so on a Mac they read — and SetToken
// overwrote — the developer's real saved credential, while passing. Fixing each
// site would leave the next one written the same way; pointing every per-user
// root at a temp tree once, before any test runs, closes the class for the
// package. (Since #965 a test binary's Dir() also honours an absolute
// XDG_CONFIG_HOME on every OS; this isolation still covers the tests that
// leave it unset and the built binaries some tests run.) Tests that set XDG_CONFIG_HOME / HOME themselves still win (t.Setenv
// overrides this for the duration of that test).
//
// 🔴 THE GO TOOLCHAIN'S OWN DIRECTORIES ARE PINNED FIRST. Some of these packages
// build the CLI with `go build` from a test. GOCACHE, GOMODCACHE, GOPATH and
// GOENV can all default to paths under HOME, so moving HOME without pinning them
// would hand that build a cold cache, an empty module cache (a network fetch)
// and none of the developer's `go env -w` settings. The pinned values are the
// EFFECTIVE ones, read with `go env -json` before HOME moves — so a value the
// developer set with `go env -w` is exported unchanged rather than replaced by
// the HOME-derived default (an exported variable beats the GOENV file). If `go`
// cannot be run, nothing is pinned and the variables are left as they were.
package configtest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run isolates the per-user directories, runs the tests, removes the temp tree
// and returns the exit code for os.Exit.
func Run(m *testing.M) int {
	root, err := os.MkdirTemp("", "civitai-configtest-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "configtest:", err)
		return 1
	}
	defer os.RemoveAll(root)
	if err := Isolate(root); err != nil {
		fmt.Fprintln(os.Stderr, "configtest:", err)
		return 1
	}
	return m.Run()
}

// homeVars are the variables os.UserHomeDir, os.UserConfigDir and
// os.UserCacheDir read on linux, darwin and windows, each mapped to its own
// directory under the root so a path cannot be satisfied by the wrong one.
var homeVars = []struct{ name, sub string }{
	{"HOME", "home"},                 // unix home; darwin config + cache
	{"USERPROFILE", "home"},          // windows home
	{"XDG_CONFIG_HOME", "config"},    // linux config
	{"XDG_CACHE_HOME", "cache"},      // linux cache
	{"APPDATA", "appdata"},           // windows config
	{"LOCALAPPDATA", "localappdata"}, // windows cache
}

// Isolate pins the Go toolchain directories to their current values, then
// points every per-user root at a directory under root.
func Isolate(root string) error {
	pinGoDirs()
	for _, v := range homeVars {
		dir := filepath.Join(root, v.sub)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Setenv(v.name, dir); err != nil {
			return err
		}
	}
	return nil
}

func pinGoDirs() {
	out, err := exec.Command("go", "env", "-json", "GOENV", "GOPATH", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		return
	}
	var eff map[string]string
	if json.Unmarshal(out, &eff) != nil {
		return
	}
	for k, v := range eff {
		// GOENV=off (or any other non-path) is exported as-is: it is the
		// effective setting, and re-exporting it changes nothing.
		if v != "" {
			_ = os.Setenv(k, v)
		}
	}
}
