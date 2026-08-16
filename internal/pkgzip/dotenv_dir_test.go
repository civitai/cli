package pkgzip

import (
	"archive/zip"
	"bytes"
	"io"
	"sort"
	"strings"
	"testing"
)

// Issue #420: the dotenv and *.zip rules were keyed on FILE TYPE, exactly as
// `.git` was before #409 — and this side leaks credentials rather than layout.
//
// `.env.local`, `.env.d` and `artifact.zip` are ordinary DIRECTORY names.
// `isExcludedFile` judged base names of regular files only, and the walk's
// directory branch consulted `excludedDirs`, a fixed-name map holding none of
// them — so the walk descended and shipped every file inside.
//
// 🔴 THE FILE RULE DOES NOT COVER FOR THE DIRECTORY RULE, which is what makes
// this a leak rather than bloat. Inside `.env.d/` the conventional file names
// are `db.env`, `api.env`, `local.env` — base names that do NOT start with
// ".env", so `isExcludedFile` keeps every one of them. The directory is the only
// thing standing between those files and an uploaded bundle that is committed to
// Forgejo and deployed.
//
// The fixture plants BOTH SHAPES of every name it names. The shape that hid this
// class twice is a fixture that only ever plants the type its author had in mind:
// `.git` was only ever tested as a directory (#409), `.env.local` only ever as a
// file (#420). A guard that has never seen the other shape cannot report it.
func TestBuildExcludesDotenvShapedDirectories(t *testing.T) {
	const leak = "API_SECRET=leak-must-never-be-packaged"

	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/App.tsx", "export default 1")

	// The measured leak, verbatim from the issue.
	writeFile(t, dir, ".env.d/db.env", leak)
	writeFile(t, dir, ".env.local/value", leak)
	writeFile(t, dir, ".env/value", leak)
	// Deeper than the root, because the rule is any-depth.
	writeFile(t, dir, "src/.env.d/api.env", leak)
	// Build artifact shaped as a directory.
	writeFile(t, dir, "artifact.zip/payload.bin", "x")

	// 🔴 KEPT NAMES INSIDE EXCLUDED DIRECTORIES — asserted against Build, not
	// against the string predicate, because the two are a seam and only Build
	// decides what is in the zip.
	//
	// Measured: a guard that pins only IsExcludedPath is BLIND here. Patching
	// Build's walk to rescue keptEnvFiles names out of an excluded directory
	// before SkipDir — i.e. restoring the "wherever they sit" behaviour this
	// package retracted — leaves the FULL 20-package suite green while the zip
	// uploads `.env.d/.env.production`. The help and README would then promise
	// those paths are dropped while the bundle carries them, which is the
	// credential direction.
	//
	// One row per directory RULE, since each reaches SkipDir by a different
	// branch of isExcludedDir, and one nested row because the rule is about
	// every ancestor, not only the parent.
	writeFile(t, dir, ".env.d/.env.production", leak)           // dotenv-shaped dir
	writeFile(t, dir, "node_modules/.env.production", leak)     // fixed name
	writeFile(t, dir, "node_modules/pkg/.env.production", leak) // …at depth
	writeFile(t, dir, "dist/.env.example", leak)                // fixed name, other kept file
	writeFile(t, dir, "artifact.zip/.env.sample", leak)         // archive-shaped dir

	// 🔴 NEGATIVE CONTROLS — content whose name merely BEGINS with ".env" and is
	// NOT dotenv-shaped. These must SHIP. A rule widened to the bare ".env"
	// prefix (which is correct for FILES, see isExcludedFile) would delete each
	// of these whole subtrees from a submission with no error anywhere, which is
	// a far worse trade for a directory than for a file. If someone widens the
	// directory rule to match the file rule, these rows go red.
	writeFile(t, dir, ".environment/config.yaml", "keep me")
	writeFile(t, dir, ".envoy/bootstrap.yaml", "keep me")

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := append([]string{}, res.Files...)
	sort.Strings(got)

	want := []string{
		".environment/config.yaml",
		".envoy/bootstrap.yaml",
		"block.manifest.json",
		"src/App.tsx",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("bundle contents wrong\n got: %v\nwant: %v", got, want)
	}

	// The file list is the packager's own account of itself. Assert the SECRET
	// is absent from the produced BYTES too — a rule that listed the path
	// differently while still writing the content would pass the check above.
	if bytesContain(res.Zip, leak) {
		t.Fatal("the planted secret is present in the produced archive bytes " +
			"although no excluded path appears in the file list")
	}
}

// The positive control for the assertion above: the same secret, in a file the
// packager is SUPPOSED to ship, must be findable in the archive bytes by the
// same search. Without this, `bytesContain` returning false proves only that the
// search does not work — the reassuring-zero shape.
//
// Deflate would defeat a naive substring search on compressed bytes, so this
// pins that the search actually reads the archive rather than the raw buffer.
func TestSecretSearchCanFindAKeptFile(t *testing.T) {
	const leak = "API_SECRET=leak-must-never-be-packaged"

	dir := t.TempDir()
	writeFile(t, dir, "block.manifest.json", `{"blockId":"x"}`)
	writeFile(t, dir, "src/config.ts", leak)

	res, err := Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !bytesContain(res.Zip, leak) {
		t.Fatal("the search cannot find a string in a file the packager DID ship — " +
			"so a negative result in TestBuildExcludesDotenvShapedDirectories " +
			"would prove nothing about the exclusion")
	}
}

// The predicate half of the seam. Build's walk and IsExcludedPath are separate
// implementations of one rule (see IsExcludedPath's 🔴 note), so a fix applied
// to the walk alone leaves the dirty-tree guard telling authors an uncommitted
// `.env.d/db.env` is bundle content.
func TestIsExcludedPathDotenvShapedDirectories(t *testing.T) {
	cases := []struct {
		path string
		want bool
		why  string
	}{
		// Dotenv-shaped directories, in git's porcelain spelling for an
		// untracked directory and as a path to a file inside one.
		{".env.d/", true, "dotenv-shaped directory"},
		{".env.d/db.env", true, "file inside a dotenv-shaped directory"},
		{".env.local/", true, "dotenv-shaped directory"},
		{".env.local/value", true, "file inside a dotenv-shaped directory"},
		{".env/", true, "the bare dotenv name as a directory"},
		{".env/value", true, "file inside it"},
		{"src/.env.d/api.env", true, "any depth"},
		{"artifact.zip/", true, "archive-shaped directory"},
		{"artifact.zip/payload.bin", true, "file inside it"},

		// 🔴 Near-miss names that must SHIP. `.environment` and `.envoy` start
		// with ".env" but are not dotenv-shaped: no `.` follows the prefix.
		{".environment/", false, "not dotenv-shaped — ordinary content"},
		{".environment/config.yaml", false, "not dotenv-shaped — ordinary content"},
		{".envoy/", false, "not dotenv-shaped — ordinary content"},
		{".envoy/bootstrap.yaml", false, "not dotenv-shaped — ordinary content"},

		// The FILE rule is unchanged and stays broader than the directory rule
		// on purpose — for a file the cost of being wrong is one renameable
		// file, for a directory it is a silently missing subtree.
		{".envrc", true, "file rule: the bare .env prefix"},
		{".env.example", false, "allow-listed FILE"},
		{".env.production", false, "allow-listed FILE"},

		// 🔴 The allow-list is a claim about FILES the server build READS.
		// `vite build` reads a dotenv FILE; a directory of that name is not
		// that, and is dotenv-shaped like any other, so it is excluded.
		{".env.production/", true, "allow-listed name, but as a DIRECTORY"},
		{".env.example/anything", true, "allow-listed name, but as a DIRECTORY"},
	}

	for _, c := range cases {
		if got := IsExcludedPath(c.path); got != c.want {
			t.Errorf("IsExcludedPath(%q) = %v, want %v — %s", c.path, got, c.want, c.why)
		}
	}
}

// 🔴 THE LEDGER FOR THE DIRECTORY RULE — the thing this package does for every
// other user-facing exclusion and that #420's first round shipped without.
//
// The README's dotenv section and the `app submit` help text both now tell an
// author which DIRECTORIES are dropped and, more importantly, which are NOT.
// Those sentences are a promise about behaviour, and prose cannot be trusted to
// track code: the golden-snapshot test next door fails when the prose is EDITED
// but cannot see prose and code DIVERGE, so widening isDotenvShaped left every
// README guard green while the documented `.envrc/` and `.env-backup/` silently
// started being deleted from submissions.
//
// This asserts the two against each other by BEHAVIOUR. The `kept` half is the
// load-bearing one: it is what goes red when someone widens the directory rule
// to the file rule's bare ".env" prefix, and the message points at the sentence
// that would become a lie.
func TestDirectoryRuleIsDocumentedForAuthors(t *testing.T) {
	// Dropped whole, at any depth.
	dropped := []string{".env", ".env.d", ".env.local", ".env.production", "artifact.zip"}
	// NOT dropped — ordinary content, or the documented residual.
	kept := []string{".envrc", ".env-backup", ".envs", ".environment", ".envoy"}

	section := dotenvSection(t)

	for _, n := range dropped {
		if !IsExcluded(n) {
			t.Errorf("directory %q ships, but DirectoryPatternSummary and the README "+
				"tell authors it is dropped whole", n)
		}
	}
	for _, n := range kept {
		if IsExcluded(n) {
			t.Errorf("directory %q is DROPPED, but DirectoryPatternSummary and the README "+
				"tell authors it ships — an author who read either would lose that subtree "+
				"from a submission with nothing printed to say so", n)
		}
	}

	// The residual is the half that can cost a credential, so the README must
	// name it rather than describing only what IS dropped. Scoped to the dotenv
	// SECTION, not the whole file: a mention anywhere in a 2,500-line README
	// would satisfy a whole-file search without the rule being documented where
	// an author reads about dotenv handling.
	for _, phrase := range []string{".env.d/", ".env-backup/"} {
		if !strings.Contains(section, phrase) {
			t.Errorf("the README's dotenv section does not mention %q — the directory "+
				"rule and its residual must both be documented where the dotenv rule is", phrase)
		}
	}

	// 🔴 EVERY ROW OF THE README'S "still uploaded" TABLE, PINNED TO BEHAVIOUR.
	// Prose that lists what is NOT protected is the most dangerous kind to let
	// rot: it rots in the safe-looking direction (code starts dropping a path
	// the doc says ships), and the author's loss is a silently missing file.
	// These paths are quoted from that table.
	for _, p := range []string{
		"db.env",             // root, base name does not start with ".env"
		"envs/prod.env",      // neither name is dotenv-shaped
		"env/local.env",      //
		".env-backup/db.env", // directory is not ".env."-dotted
		"x.ZIP/a.txt",        // the archive rule is case-sensitive
		".ENV.local/b.txt",   // the dotenv rule is case-sensitive
		// 🔴 The keptEnvFiles allow-list survives a directory the walk does not
		// drop, so an author who reads "files starting with .env are dropped" is
		// wrong about the one file most likely to hold a token. Added to the
		// README's table as its own row — the most surprising residual there is.
		".env-backup/.env.production",
	} {
		if IsExcludedPath(p) {
			t.Errorf("%q is EXCLUDED, but the README's residual table tells authors it is "+
				"still uploaded. Either the table is now wrong, or a rule widened without "+
				"its documentation — fix whichever moved", p)
		}
	}

	// 🔴 THE OTHER SIDE OF THE SAME SENTENCE. Both the help and
	// DirectoryPatternSummary now say a kept dotenv name is uploaded "in any
	// directory that is itself packaged" and NOT rescued from an excluded one.
	// The rows above pin the first half; without these, the sentence could be
	// widened back to the false universal it replaced — "wherever they sit" —
	// and stay green.
	//
	// Build's walk returns SkipDir before isExcludedFile is ever consulted, so
	// the directory decides. Measured: `.env.d/.env.production` is dropped even
	// though `.env.production` is on the allow-list.
	for _, p := range []string{
		".env.d/.env.production",       // the layout the help names two lines earlier
		"node_modules/.env.production", // any excluded directory, not just dotenv ones
		"dist/.env.example",
	} {
		if !IsExcludedPath(p) {
			t.Errorf("%q is PACKAGED, but the help and DirectoryPatternSummary both say a "+
				"kept name does not rescue its directory. A kept file inside a dropped "+
				"directory is content loss the author is never told about", p)
		}
	}
}

// 🔴 THE SUMMARY IS PROSE THE CLI PRINTS, SO PIN THE WHOLE NORMALISED STRING.
//
// The behavioural ledger above cannot see a REWORDING. Measured on the round
// that added it: rewriting the tail from "are NOT dropped … is uploaded" to
// "are ALSO dropped whole … is safely removed from the bundle for you" keeps
// every token the ledger greps for, so the whole suite stayed green — while the
// string the CLI prints had become false in the CREDENTIAL direction, which is
// exactly the direction DirectoryPatternSummary's own doc calls the costly one.
// A guard on WORDS is walkable by choosing different words.
//
// Comparison is whitespace-collapsed, so re-wrapping the source to fit the help
// text costs nothing. Any other edit fails here and must be re-read against the
// code before it is re-approved — which is the whole point.
func TestDirectoryPatternSummaryIsTheReviewedCopy(t *testing.T) {
	// 🔴 The tail was reworded once already, in the credential direction, and the
	// old wording is why: it said a file inside `.env-backup/` "is uploaded
	// unless its own name starts with .env", which is FALSE for the three
	// keptEnvFiles names. Measured — `.env-backup/.env.production` is packaged
	// WITH its contents, while `.env-backup/.env.local` is dropped. An author
	// reading the old sentence believed the wrong one was protected.
	const want = "a DIRECTORY named .env or .env.<anything> (e.g. .env.d/, .env.local/, " +
		".env.production/), or ending in .zip, is dropped whole at any depth. Directories " +
		"whose names merely start with .env — .envrc/, .env-backup/, .envs/ — are NOT " +
		"dropped, and a secret-bearing file inside one (db.env, prod.env) is uploaded " +
		"unless the FILE rule below drops it. That rule KEEPS .env.example, .env.sample " +
		"and .env.production BY NAME — so .env-backup/.env.production is uploaded too — " +
		"but only where every directory above it is itself packaged: a kept name anywhere " +
		"under .env.d/ or node_modules/ goes with that directory."

	got := strings.Join(strings.Fields(DirectoryPatternSummary()), " ")
	if got == "" {
		t.Fatal("CONTROL failure: the summary normalised to nothing, so an empty golden " +
			"would match it forever")
	}
	if got != want {
		t.Errorf("DirectoryPatternSummary changed.\n\n want: %s\n\n got:  %s\n\n"+
			"🔴 This string is printed by `civitai app submit --help` and tells an author "+
			"which directories are NOT protected. Re-approve only after checking the new "+
			"wording is true of isExcludedDir — run the roster in "+
			"TestDirectoryRuleIsDocumentedForAuthors against it.", want, got)
	}
}

// bytesContain reports whether s appears in the CONTENT of any file stored in
// the zip. Searching the raw archive buffer would be defeated by Deflate, so
// this decompresses each entry — see TestSecretSearchCanFindAKeptFile for the
// control that proves the search works at all.
func bytesContain(zipBytes []byte, s string) bool {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return false
	}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err == nil && strings.Contains(string(b), s) {
			return true
		}
	}
	return false
}
