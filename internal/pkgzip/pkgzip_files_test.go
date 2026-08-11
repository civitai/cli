package pkgzip

import (
	"sort"
	"strings"
	"testing"
)

// fakeToken is a sentinel a secret-bearing dotenv would carry. We assert it
// never appears in the produced file list (and the file that holds it is gone).
const fakeToken = "VITE_LIVE_BLOCK_TOKEN=secret-do-not-leak"

// TestBuildExcludesEnvAndZip is the headline security + data-integrity guard:
// a project tree with secret-bearing dotenv files and a stray output zip must
// produce a package that includes the source + .env.example and excludes the
// dev-local/secret dotenv files and the .zip.
func TestBuildExcludesEnvAndZip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "src/App.tsx", "export default 1")

	// SECRET-bearing / dev-local — must be excluded.
	writeFile(t, dir, ".env.development", fakeToken+"\n")
	writeFile(t, dir, ".env.local", "VITE_HARNESS_MODE=live\n")
	writeFile(t, dir, ".env", "VITE_LIVE_BLOCK_TOKEN=root-secret\n")
	writeFile(t, dir, ".env.development.local", "VITE_LIVE_BLOCK_TOKEN=local-override\n")

	// Template — must be KEPT. Allow-listed BY NAME, not because anything read
	// it: this fixture's own body is the `VITE_LIVE_BLOCK_TOKEN=` line the
	// page-money template invites an author to fill in.
	writeFile(t, dir, ".env.example", "VITE_LIVE_BLOCK_TOKEN=\n")

	// Stray build artifact from a prior --package-only run — must be excluded.
	writeFile(t, dir, "prev-package.zip", "PK\x03\x04junk")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got := namesInZip(t, res.Zip)
	sort.Strings(got)
	want := []string{".env.example", "block.manifest.json", "package.json", "src/App.tsx"}
	if len(got) != len(want) {
		t.Fatalf("zip contents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("zip[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}

	// Explicit per-file assertions.
	excluded := []string{".env", ".env.local", ".env.development", ".env.development.local", "prev-package.zip"}
	for _, bad := range got {
		for _, e := range excluded {
			if bad == e {
				t.Errorf("%q should have been excluded", bad)
			}
		}
	}
	included := map[string]bool{}
	for _, g := range got {
		included[g] = true
	}
	if !included[".env.example"] {
		t.Error(".env.example (allow-listed BY NAME) should be INCLUDED")
	}
	if !included["src/App.tsx"] {
		t.Error("source file should be INCLUDED")
	}
}

// TestResultFilesHaveNoSecretEnv asserts no secret-bearing .env* path appears in
// the exported Result.Files list (what the CLI prints / uploads).
func TestResultFilesHaveNoSecretEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, ".env.development", fakeToken+"\n")
	writeFile(t, dir, ".env.local", "x")
	writeFile(t, dir, ".env", "x")
	writeFile(t, dir, "src/main.tsx", "export default 1")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, f := range res.Files {
		switch f {
		case ".env", ".env.local", ".env.development":
			t.Errorf("secret-bearing dotenv %q leaked into Result.Files: %v", f, res.Files)
		}
		if strings.Contains(f, fakeToken) {
			t.Errorf("token sentinel leaked into a file path: %q", f)
		}
	}
}

// TestBuildRecursiveZipReproduce reproduces the dogfound recursive re-bundle:
// build once, leave the output zip in the dir, build again — the second package
// must NOT contain the first zip, and the file count must be stable.
func TestBuildRecursiveZipReproduce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "src/main.tsx", "export default 1")

	first, err := Build(dir)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	firstCount := len(first.Files)

	// Simulate `--package-only`: drop the produced zip into the project dir.
	writeFile(t, dir, "my-block.zip", string(first.Zip))

	second, err := Build(dir)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}

	for _, f := range second.Files {
		if strings.HasSuffix(f, ".zip") {
			t.Errorf("second package recursively re-bundled a zip: %q (all: %v)", f, second.Files)
		}
	}
	if len(second.Files) != firstCount {
		t.Errorf("file count not stable across re-package: first=%d second=%d (%v)", firstCount, len(second.Files), second.Files)
	}
}

// TestIsExcludedFileRule pins the exact dotenv keep/drop decision so the
// build-safety judgment is regression-tested.
func TestIsExcludedFileRule(t *testing.T) {
	drop := []string{
		"prev-package.zip", "a/b.zip",
		".env", ".env.local", ".env.development", ".env.test",
		".env.development.local", ".env.production.local", ".env.staging",
		// 🔴 UNDOTTED `.env`-prefixed names. Every row above is `.env.`-dotted,
		// so this table agreed with a matcher keyed on `.env.` and with one keyed
		// on `.env` — it could not tell them apart, and the narrower one uploaded
		// `.envrc` (direnv; routinely holds exported credentials) to the platform
		// and to a human moderator reviewer.
		".envrc", ".env-local", ".envprod", ".environment",
	}
	for _, n := range drop {
		// IsExcludedFile matches on base name; strip dir for the path-y cases.
		base := n
		if i := strings.LastIndex(n, "/"); i >= 0 {
			base = n[i+1:]
		}
		if !IsExcludedFile(base) {
			t.Errorf("%q should be excluded", base)
		}
	}
	keep := []string{
		".env.example", ".env.sample", ".env.production",
		"src/App.tsx", "package.json", "block.manifest.json", "vite.config.ts",
		"zipper.ts", // contains "zip" but is not a .zip
		// The other side of the widened prefix. Two directions are pinned here:
		// the leading DOT is still load bearing (a source file merely named
		// after the environment ships), and the prefix stops at ".env" — the
		// dotfiles a Vite project actually carries must survive it.
		"envrc", "env.ts", "environment.ts", "src/env.d.ts",
		".eslintrc.json", ".editorconfig", ".gitignore", ".npmrc", ".prettierrc",
	}
	for _, n := range keep {
		base := n
		if i := strings.LastIndex(n, "/"); i >= 0 {
			base = n[i+1:]
		}
		if IsExcludedFile(base) {
			t.Errorf("%q should NOT be excluded", base)
		}
	}
}

// TestExcludedFilePatternsInJoin makes sure the CLI's printed exclusion list
// surfaces the new file patterns (so users know .env*/*.zip are dropped).
func TestExcludedFilePatternsInJoin(t *testing.T) {
	pats := ExcludedFilePatterns()
	if len(pats) == 0 {
		t.Fatal("ExcludedFilePatterns is empty")
	}
	joined := JoinExcluded()
	for _, p := range []string{"*.zip", ".env"} {
		if !strings.Contains(joined, p) {
			t.Errorf("JoinExcluded() = %q, missing file pattern %q", joined, p)
		}
	}
	// Dir names must still be present too.
	if !strings.Contains(joined, "node_modules") {
		t.Errorf("JoinExcluded() lost the dir names: %q", joined)
	}
}
