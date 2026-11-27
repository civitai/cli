package validate

import (
	"os"
	"path/filepath"
	"testing"
)

// npmLock builds a lockfileVersion-3 package-lock.json resolving blocks-react
// to v at the root.
func npmLock(v string) string {
	return `{"name":"app","lockfileVersion":3,"requires":true,"packages":{` +
		`"":{"name":"app","dependencies":{"@civitai/blocks-react":"^0.71.0"}},` +
		`"node_modules/@civitai/blocks-react":{"version":"` + v + `"},` +
		`"node_modules/other/node_modules/@civitai/blocks-react":{"version":"9.9.9"}}}`
}

// TestLockedBlocksReactVersion pins the per-format resolution. Every row's
// version is distinct, so a reader that returned another row's or a nested
// copy's answer is caught.
func TestLockedBlocksReactVersion(t *testing.T) {
	cases := []struct {
		name, build, file, body, want string
		ok                            bool
	}{
		{"npm v3: the ROOT install, not a nested copy", "npm run build", "package-lock.json", npmLock("0.71.3"), "0.71.3", true},
		{"npm v1: dependencies map", "", "package-lock.json",
			`{"lockfileVersion":1,"dependencies":{"@civitai/blocks-react":{"version":"0.70.2"}}}`, "0.70.2", true},
		{"npm with a leading BOM", "npm run build", "package-lock.json", "\xef\xbb\xbf" + npmLock("0.71.4"), "0.71.4", true},
		{"npm: package absent", "npm run build", "package-lock.json",
			`{"lockfileVersion":3,"packages":{"node_modules/react":{"version":"19.0.0"}}}`, "", false},
		{"npm: not JSON", "npm run build", "package-lock.json", "garbage", "", false},
		{"pnpm v9: importers, peer suffix stripped", "pnpm run build", "pnpm-lock.yaml",
			"lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n      '@civitai/blocks-react':\n" +
				"        specifier: ^0.71.0\n        version: 0.71.5(react@19.0.0)\n", "0.71.5", true},
		{"pnpm v6 single project", "pnpm run build", "pnpm-lock.yaml",
			"lockfileVersion: '6.0'\ndependencies:\n  '@civitai/blocks-react':\n    specifier: ^0.69.0\n    version: 0.69.1\n", "0.69.1", true},
		{"pnpm v5: bare string, underscore peer suffix", "pnpm run build", "pnpm-lock.yaml",
			"lockfileVersion: 5.4\ndevDependencies:\n  '@civitai/blocks-react': 0.68.0_react@18.2.0\n", "0.68.0", true},
		{"yarn v1", "yarn run build", "yarn.lock",
			"# yarn lockfile v1\n\n\"@civitai/blocks-react@^0.71.0\":\n  version \"0.71.6\"\n  resolved \"x\"\n\nreact@^19:\n  version \"19.0.0\"\n",
			"0.71.6", true},
		{"yarn berry", "yarn run build", "yarn.lock",
			"__metadata:\n  version: 8\n\n\"@civitai/blocks-react@npm:^0.71.0\":\n  version: 0.71.7\n  resolution: \"x\"\n",
			"0.71.7", true},
		{"yarn: two distinct versions is unobservable", "yarn run build", "yarn.lock",
			"\"@civitai/blocks-react@^0.70.0\":\n  version \"0.70.0\"\n\n\"@civitai/blocks-react@^0.71.0\":\n  version \"0.71.0\"\n",
			"", false},
		{"yarn: a sibling package does not match", "yarn run build", "yarn.lock",
			"\"@civitai/blocks-react-native@^1.0.0\":\n  version \"1.0.0\"\n", "", false},
		{"the lockfile buildCommand selects is the one read", "pnpm run build", "package-lock.json", npmLock("0.71.0"), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, ok := lockedBlocksReactVersion(dir, tc.build)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("lockedBlocksReactVersion = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
	t.Run("no lockfile", func(t *testing.T) {
		if _, ok := lockedBlocksReactVersion(t.TempDir(), "npm run build"); ok {
			t.Fatal("a missing lockfile resolved a version")
		}
	})
}

// TestBlocksReactPreFixGate drives the gate at the REAL bound. Rows straddle
// it on every axis that can be wrong: patch, minor, major, numeric-vs-lexical,
// a pre-release on each side, and an unobservable lockfile.
//
// PRE-RELEASE DECISION, per SemVer precedence: 0.72.0-rc.1 sorts BELOW 0.72.0,
// so it is at or below the last measured pre-fix release and fires;
// 0.72.1-next.0 sorts ABOVE 0.72.0 — an unmeasured build that may already
// carry #632 — so it is silent (the cheap direction).
func TestBlocksReactPreFixGate(t *testing.T) {
	cases := []struct {
		locked string // "" = no lockfile
		want   bool
	}{
		{"0.71.0", true},         // what page-money's ^0.71.0 resolves to
		{"0.72.0", true},         // the bound itself: measured pre-fix
		{"0.72.0-rc.1", true},    // a pre-release BELOW the bound
		{"0.72.1-next.0", false}, // a pre-release ABOVE the bound: unmeasured
		{"0.72.1", false},        // the next patch: unmeasured
		{"0.73.0", false},        // a later minor
		{"1.0.0", false},         // a later major
		{"0.9.99", true},         // numeric, not lexical: 0.9 < 0.72
		{"0.100.0", false},       // numeric, not lexical: 0.100 > 0.72
		{"not-a-version", false}, // unparseable: unobservable, silent
		{"", false},              // no lockfile: unobservable, silent
	}
	for _, tc := range cases {
		dir := t.TempDir()
		if tc.locked != "" {
			if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(npmLock(tc.locked)), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got := blocksReactPreFix(dir, "npm run build", blocksReactLastPreFixVersion); got != tc.want {
			t.Errorf("locked %q against bound %s: preFix = %v, want %v", tc.locked, blocksReactLastPreFixVersion, got, tc.want)
		}
	}
	// An unreadable lockfile is unobservable too — a directory where the file
	// should be (regularFileExists refuses it), so this holds under root.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "package-lock.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if blocksReactPreFix(dir, "npm run build", blocksReactLastPreFixVersion) {
		t.Errorf("an unreadable lockfile opened the gate")
	}
	// A bound that is not a version must close the gate, not open it.
	if blocksReactPreFix(dir, "npm run build", "not-a-version") {
		t.Errorf("an unparseable bound opened the gate")
	}
}

// TestLastPreFixVersionValue pins the LITERAL. Every assertion above is written
// against the constant, so moving it would re-shape them all silently; this is
// the one that notices. Raising it is a claim that a newer release was MEASURED
// to lack civitai-app-starters#632 — change this test with that measurement.
func TestLastPreFixVersionValue(t *testing.T) {
	if blocksReactLastPreFixVersion != "0.72.0" {
		t.Fatalf("blocksReactLastPreFixVersion = %q, want \"0.72.0\" — see its doc comment before moving it",
			blocksReactLastPreFixVersion)
	}
}

// TestSDKMountFindingIsGatedOnTheLockedVersion runs the WHOLE check through
// Dir: the production shape (only useBuzzWorkflow) fires on a pre-fix lockfile
// and is silent above the bound and with no readable version.
func TestSDKMountFindingIsGatedOnTheLockedVersion(t *testing.T) {
	for _, tc := range []struct {
		locked string // "" = no lockfile
		want   string
	}{
		{"0.72.0", "sdk-unconnected"},
		{"0.71.0", "sdk-unconnected"},
		{"0.72.1", ""},
		{"0.73.0", ""},
		{"", ""},
	} {
		t.Run("locked "+tc.locked, func(t *testing.T) {
			extra := map[string]string{"package-lock.json": npmLock(tc.locked)}
			dir := sdkProject(t, lazyOnlyApp, extra)
			if tc.locked == "" {
				if err := os.Remove(filepath.Join(dir, "package-lock.json")); err != nil {
					t.Fatal(err)
				}
			}
			wantAckKind(t, dir, tc.want)
		})
	}
}

// TestSemverLess pins precedence on the edges the gate depends on.
func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.71.0", "0.72.1", true},
		{"0.72.1", "0.72.1", false},
		{"0.72.1-next.0", "0.72.1", true},
		{"0.72.1", "0.72.1-next.0", false},
		{"0.72.1-next.2", "0.72.1-next.10", true},
		{"0.72.1-alpha", "0.72.1-beta", true},
		{"0.72.1-1", "0.72.1-alpha", true},
		{"0.9.0", "0.72.0", true},
		{"0.72.1+build.5", "0.72.1", false},
	}
	for _, tc := range cases {
		a, ok1 := parseSemver(tc.a)
		b, ok2 := parseSemver(tc.b)
		if !ok1 || !ok2 {
			t.Fatalf("parse %q/%q failed", tc.a, tc.b)
		}
		if got := a.less(b); got != tc.want {
			t.Errorf("%s < %s = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
	for _, bad := range []string{"", "1.2", "1.2.3.4", "1.x.3", "1.2.3-", "-1.2.3"} {
		if _, ok := parseSemver(bad); ok {
			t.Errorf("parseSemver(%q) accepted a non-version", bad)
		}
	}
}

// pnpmTwoDocLock is a literal pnpm 11+ lockfile with TWO YAML documents: a
// leading one (pnpm's own version and config dependencies, which carries its
// own importers["."]) and then the project lockfile, which pnpm always writes
// LAST. leadDeps is spliced into the leading document's importers["."] so a
// case can put a decoy dependency there.
func pnpmTwoDocLock(leadDeps string) string {
	return "---\n" +
		"lockfileVersion: '9.0'\n" +
		"\n" +
		"importers:\n" +
		"\n" +
		"  .:\n" +
		"    configDependencies: {}\n" +
		leadDeps +
		"    packageManagerDependencies:\n" +
		"      pnpm:\n" +
		"        specifier: 12.3.4\n" +
		"        version: 12.3.4\n" +
		"\n" +
		"packages:\n" +
		"\n" +
		"  pnpm@12.3.4:\n" +
		"    resolution: {integrity: sha512-AAAA}\n" +
		"\n" +
		"snapshots:\n" +
		"\n" +
		"  pnpm@12.3.4: {}\n" +
		"---\n" +
		"lockfileVersion: '9.0'\n" +
		"\n" +
		"settings:\n" +
		"  autoInstallPeers: true\n" +
		"  excludeLinksFromLockfile: false\n" +
		"\n" +
		"importers:\n" +
		"\n" +
		"  .:\n" +
		"    dependencies:\n" +
		"      '@civitai/blocks-react':\n" +
		"        specifier: ^0.87.0\n" +
		"        version: 0.81.3(react@19.0.0)\n" +
		"      '@civitai/sdk':\n" +
		"        specifier: ^0.33.0\n" +
		"        version: 0.31.6\n"
}

// TestPnpmLockfileMultiDocument: a multi-document pnpm-lock.yaml is read from
// its LAST document (the project lockfile), as the server-side SDK floor check
// reads it. Reading only the first document found the leading pnpm-config
// document and silently resolved no SDK version. Single-document files read as
// before; an empty file resolves nothing; a non-mapping last document, or
// broken YAML in any document, is unreadable.
func TestPnpmLockfileMultiDocument(t *testing.T) {
	// A decoy the leading document cannot hold in a real pnpm lockfile: if a
	// reader takes the FIRST document, it answers these versions instead of
	// silence, and each is distinct from the project document's.
	decoy := "    dependencies:\n" +
		"      '@civitai/blocks-react':\n" +
		"        specifier: ^0.60.0\n" +
		"        version: 0.60.9\n" +
		"      '@civitai/sdk':\n" +
		"        specifier: ^0.20.0\n" +
		"        version: 0.20.7\n"
	single := "lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n" +
		"      '@civitai/blocks-react':\n        specifier: ^0.71.0\n        version: 0.71.8(react@19.0.0)\n" +
		"      '@civitai/sdk':\n        specifier: ^0.31.0\n        version: 0.31.2\n"
	cases := []struct {
		name, body          string
		wantBlocks, wantSDK string
		ok                  bool
	}{
		{"two documents: the project (last) document", pnpmTwoDocLock(""), "0.81.3", "0.31.6", true},
		{"two documents: a leading document holding the packages is not read", pnpmTwoDocLock(decoy), "0.81.3", "0.31.6", true},
		{"single document, no separator", single, "0.71.8", "0.31.2", true},
		{"single document behind a leading separator", "---\n" + single, "0.71.8", "0.31.2", true},
		{"two documents with a BOM", "\xef\xbb\xbf" + pnpmTwoDocLock(""), "0.81.3", "0.31.6", true},
		{"empty file", "", "", "", false},
		{"non-mapping last document is unreadable", single + "---\n- just\n- a list\n", "", "", false},
		{"scalar last document is unreadable", single + "---\nnot a lockfile\n", "", "", false},
		{"broken YAML in a later document is unreadable", single + "---\nimporters: [unclosed\n", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, pkg := range []struct{ name, want string }{
				{"@civitai/blocks-react", tc.wantBlocks},
				{"@civitai/sdk", tc.wantSDK},
			} {
				got, ok := LockedVersion(dir, "pnpm run build", pkg.name)
				if got != pkg.want || ok != tc.ok {
					t.Errorf("LockedVersion(%s) = (%q, %v), want (%q, %v)", pkg.name, got, ok, pkg.want, tc.ok)
				}
			}
		})
	}
}
