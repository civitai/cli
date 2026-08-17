package pkgzip

import (
	"sort"
	"strings"
	"testing"
)

// Issue #435, the half that is decidable: the packager matched dotenv files by
// PREFIX only, and every rule was case-SENSITIVE.
//
// #420 closed the dotenv-shaped DIRECTORY hole. It did nothing for the shape
// that tooling actually writes — `db.env`, `prod.env`, `local.env` — because
// those base names do not START with ".env", so no rule saw them at any depth.
// Measured on v0.1.97 through the released binary: `db.env` at the project
// root, `envs/prod.env`, `env/local.env` and `.env-backup/db.env` were all
// packaged into a bundle that is committed to Forgejo and deployed.
//
// 🔴 A SUFFIX RULE FOR FILES, NOT FOR DIRECTORIES, and the asymmetry is the
// same one #420 settled: for a FILE the cost of matching too much is one file
// the author renames, and the cost of matching too little is an uploaded
// credential — so it is aimed wide. A directory named `config.env` would take a
// whole subtree with it silently, so the directory rule stays narrow
// (isDotenvShaped) and is deliberately NOT given the suffix.
func TestBuildExcludesEnvSuffixFilesAnywhere(t *testing.T) {
	const leak = "API_SECRET=leak-must-never-be-packaged"

	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")

	// The measured rows from #435 — none of these start with ".env".
	writeFile(t, dir, "db.env", leak)              // project root
	writeFile(t, dir, "envs/prod.env", leak)       // the conventional plural dir
	writeFile(t, dir, "env/local.env", leak)       // and the singular
	writeFile(t, dir, ".env-backup/db.env", leak)  // inside a NOT-dotenv-shaped dir
	writeFile(t, dir, "src/deep/config.env", leak) // any depth

	// 🔴 CASE VARIANTS, THROUGH Build — not only through the predicates.
	// Measured: with these rows absent, a walk that consults isExcludedFile only
	// for already-lowercase names passes the FULL 20-package suite while
	// packaging `X.ZIP/`, `.ENV.LOCAL` and `DB.ENV`. Every case assertion lived
	// in predicate-level tables, and the predicates keep answering correctly
	// under that mutation — the defect is in the walk that calls them. The
	// function that produces the uploaded bytes has to see the case variants.
	writeFile(t, dir, "DB.ENV", leak)
	writeFile(t, dir, ".ENV.LOCAL", leak)
	writeFile(t, dir, "X.ZIP/a.txt", leak)

	// 🔴 A DOTTED STEM. Every other `*.env` row here has a single-dot name, so
	// none of them can see a rule narrowed to `HasSuffix(".env") && the stem has
	// no dot` — measured, that mutation passes the FULL suite while packaging
	// this file. `backup.db.env` is exactly what a dated or environment-scoped
	// dump is called.
	writeFile(t, dir, "backup.db.env", leak)

	// 🔴 NEGATIVE CONTROLS — names that merely contain or resemble "env" and
	// must SHIP. A suffix rule widened to a substring, or to a bare "env",
	// would take every one of these.
	writeFile(t, dir, "src/environment.ts", "keep me")
	writeFile(t, dir, "src/env.ts", "keep me")
	writeFile(t, dir, "docs/env.md", "keep me")
	// A FILE named `env`, under src/ because the fixture already needs `env/`
	// as a DIRECTORY at the root — one tree cannot hold both shapes of one name.
	writeFile(t, dir, "src/env", "keep me")
	writeFile(t, dir, "src/envelope.tsx", "keep me")
	// 🔴 THE SUFFIX/SUBSTRING BOUNDARY. Every control above lacks a literal
	// ".env" substring, so they cannot see the single most likely careless
	// widening: HasSuffix -> Contains. Measured — that mutation leaves the full
	// suite green and silently drops both of these from the bundle.
	writeFile(t, dir, "src/app.env.ts", "keep me")
	writeFile(t, dir, "docs/readme.env.md", "keep me")
	// 🔴 THE FILES-ONLY DECISION, PINNED. A DIRECTORY whose name ends in ".env"
	// keeps its contents: giving isExcludedDir this suffix would drop a whole
	// subtree on a name match, which is the silent loss the directory rule is
	// deliberately aimed away from. Without this row, moving the rule into
	// isExcludedDir passes every other assertion here.
	writeFile(t, dir, "config.env/settings.ts", "keep me")
	// The allow-list is unaffected: it is checked by NAME, and none of the
	// three kept names ends in ".env".
	writeFile(t, dir, ".env.example", "PLACEHOLDER=")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := append([]string{}, res.Files...)
	sort.Strings(got)

	want := []string{
		".env.example",
		"block.manifest.json",
		"config.env/settings.ts",
		"docs/env.md",
		"docs/readme.env.md",
		"src/App.tsx",
		"src/app.env.ts",
		"src/env",
		"src/env.ts",
		"src/envelope.tsx",
		"src/environment.ts",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("bundle contents wrong\n got: %v\nwant: %v", got, want)
	}
	if bytesContain(res.Zip, leak) {
		t.Fatal("a planted secret is present in the produced archive bytes although " +
			"no excluded path appears in the file list")
	}
}

// Every rule was case-sensitive, so `x.ZIP/` and `.ENV.local/` packaged (#435,
// measured on v0.1.97). Case is not a meaningful distinction for either
// convention — nobody names content to differ from `.zip` only by case — so
// matching is now case-insensitive on both PATTERN rules and on the file-level
// dotenv prefix.
//
// 🔴 THE ALLOW-LIST STAYS EXACT, AND THAT IS DELIBERATE. `vite build` reads
// `.env.production` by its exact name; a file called `.ENV.PRODUCTION` is not
// read by anything, so keeping it would be a claim nothing supports. With a
// case-insensitive prefix and an exact allow-list it is DROPPED — the safe
// direction, and the author sees a missing file rather than an uploaded one.
//
// The FIXED-name maps (excludedDirs, vcsMetadataNames) are NOT case-folded
// here. `Build/` and `Dist/` are plausible content directory names in projects
// that capitalise, and dropping one is a silent subtree loss — the hazard #420
// spent four rounds tuning against. That is a separate decision with a
// different blast radius; it is not this change.
func TestExclusionRulesAreCaseInsensitive(t *testing.T) {
	dirCases := []struct {
		name string
		want bool
		why  string
	}{
		{"x.ZIP", true, "archive rule, upper"},
		{"x.Zip", true, "archive rule, mixed"},
		{"x.zip", true, "archive rule, lower (unchanged)"},
		{".ENV.local", true, "dotenv-shaped, upper"},
		{".Env.D", true, "dotenv-shaped, mixed"},
		{".ENV", true, "the bare dotenv name, upper"},
		{".ENVIRONMENT", false, "near-miss: no dot follows the prefix"},
		{".ENVOY", false, "near-miss"},
		{"SRC", false, "ordinary content"},
		// The fixed-name maps are deliberately NOT folded — see the doc above.
		{"NODE_MODULES", false, "fixed name, not case-folded (deliberate)"},
		{"Build", false, "fixed name, not case-folded (deliberate)"},
	}
	for _, c := range dirCases {
		if got := IsExcluded(c.name); got != c.want {
			t.Errorf("IsExcluded(%q) = %v, want %v — %s", c.name, got, c.want, c.why)
		}
	}

	fileCases := []struct {
		name string
		want bool
		why  string
	}{
		{"BUNDLE.ZIP", true, "archive rule, upper"},
		{".ENV.LOCAL", true, "dotenv prefix, upper"},
		{".Env", true, "the bare dotenv name, mixed"},
		{"DB.ENV", true, "the new suffix rule, upper"},
		{"Prod.Env", true, "the new suffix rule, mixed"},
		{".env.example", false, "allow-listed, exact"},
		{".env.production", false, "allow-listed, exact"},
		// 🔴 The allow-list is EXACT: a case variant is not the file vite reads,
		// so it falls to the catch-all and is dropped.
		{".ENV.PRODUCTION", true, "allow-list is exact — a case variant is dropped"},
		{".Env.Example", true, "allow-list is exact — a case variant is dropped"},
		{"environment.ts", false, "near-miss: does not end in .env"},
		{"env", false, "near-miss: no extension"},
		{"envelope.tsx", false, "near-miss"},
	}
	for _, c := range fileCases {
		if got := IsExcludedFile(c.name); got != c.want {
			t.Errorf("IsExcludedFile(%q) = %v, want %v — %s", c.name, got, c.want, c.why)
		}
	}
}

// The seam again: the dirty-tree guard reads IsExcludedPath, so a rule added to
// Build alone would tell an author an uncommitted `envs/prod.env` is bundle
// content. TestIsExcludedPathAgreesWithBuild covers the tree; these pin the
// specific paths #435 measured.
func TestIsExcludedPathCoversEnvSuffixAndCase(t *testing.T) {
	for _, p := range []string{
		"db.env",
		"envs/prod.env",
		"env/local.env",
		".env-backup/db.env",
		"src/deep/config.env",
		"x.ZIP/a.txt",
		".ENV.local/b.txt",
		".ENV.LOCAL",
	} {
		if !IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = false, want true — #435 measured this packaged", p)
		}
	}
	for _, p := range []string{
		"src/environment.ts",
		"src/env.ts",
		".env.example",
		".environment/config.yaml",
		".envoy/bootstrap.yaml",
	} {
		if IsExcludedPath(p) {
			t.Errorf("IsExcludedPath(%q) = true, want false — this is ordinary content", p)
		}
	}
}
