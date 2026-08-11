package scaffold_test

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/pkgzip"
	"github.com/civitai/cli/internal/scaffold"
)

// 🔴 A DOTENV FILE THE PACKAGER UPLOADS MUST NOT BE THE PLACE ITS OWN SECRET
// GOES. This is the seam between two packages that were each individually
// correct: `internal/pkgzip` deliberately ALLOW-LISTS `.env.example` (a reviewer
// is meant to read it) and `internal/scaffold` writes that file's contents — so
// "where does the token go?" was answered by a template that had never been read
// against the packager. It answered "Paste it here", and `civitai app submit`
// then shipped the pasted token to the platform and to a human moderator
// reviewer (issue #380). The warning it carried — "never commit one" — is about
// git, which is the channel that was NOT the problem.
//
// 🔴 EVERY LEG IS ANSWERED BY THE REAL IMPLEMENTATION, NEVER BY A MODEL OF IT.
// "Is this file uploaded?" is `pkgzip.Build`'s own file list; "is it git-ignored?"
// is `git check-ignore` run against the rendered tree. Both were reimplementations
// once and both lied: a hand-rolled `path.Match` loop over the `.gitignore` lines
// silently ignored NEGATION, so appending `!.env.development.local` left this
// suite green while real git reported the secret's destination NOT IGNORED.
// A guard that reimplements the thing it is guarding tests the reimplementation.
//
// 🔴 WHAT IT IS NOT. It does not read the prose. A comment block that carries a
// correct destination annotation AND also invites a paste into the uploaded file
// passes here — item 28 records two banned-phrase ledgers losing to the next
// paraphrase, so no word list is attempted. What is computable is: (1) every key
// in an uploaded dotenv file is CLASSIFIED; (2) every secret one names a
// destination the packager EXCLUDES and `git check-ignore` IGNORES; (3) every
// secret one is declared EMPTY in the uploaded file — the single deterministic
// check that makes a real token in an uploaded file impossible rather than
// discouraged; and (4) no UNDOTTED `env.*` file reaches the package, since
// `pkgzip` recognises dotenv files by the leading dot alone.
// `TestUploadedDotenvIsTheReviewedCopy` closes the addition half.

// secretEnvKeys are the keys in the scaffolded dotenv files whose value is a
// live credential: a real one spends the author's money or reads their account.
// publicEnvKeys are the rest — build/config values that are meant to be read by
// a reviewer and are inlined into the client bundle anyway.
//
// The two sets are a BIDIRECTIONAL ledger against what the templates actually
// render: a key in neither fails, and a key in either that no template declares
// fails. That is the part that survives a change in shape — a NEW credential
// added to an uploaded dotenv file cannot reach the platform without someone
// classifying it here, and classifying it as secret arms every check below.
var (
	secretEnvKeys = map[string]string{
		"VITE_LIVE_BLOCK_TOKEN": "a dev block token — a real one SPENDS the author's Buzz",
		"CIVITAI_HOST_KEY":      "the author's personal API key — full account scope",
	}
	publicEnvKeys = map[string]struct{}{
		"VITE_BLOCK_ALLOWED_PARENT_ORIGINS": {}, // baked into the bundle by design
		"VITE_DEV_HARNESS":                  {}, // a dev-only on/off flag
		"VITE_HARNESS_MODE":                 {}, // "mock" | "live"
		"VITE_LIVE_HOST_ORIGIN":             {}, // a proxy target, not a credential
	}
)

// assignmentRE matches an uncommented `KEY=VALUE` declaration line, capturing
// both halves. Deliberately not restricted to SHOUTING_CASE: dotenv keys are
// uppercase by convention only, and a convention is not a rule the ledger below
// may rely on — a lower-case key would otherwise be invisible to every check in
// this file.
var assignmentRE = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

// destinationRE matches the destination annotation: a commented-out example
// assignment carrying the file the real value belongs in. It is the shape the
// scaffolded `CIVITAI_HOST_KEY` block already used —
//
//	#   CIVITAI_HOST_KEY=<your-personal-api-key>   # in .env.development.local only
//
// — promoted to a contract so the answer to "where does this go?" is machine
// readable and cannot be a deictic ("here"). The captured path is then checked
// against the packager and the scaffolded .gitignore, so this regexp decides
// only WHERE THE CLAIM IS, never whether the destination is safe.
func destinationRE(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^#\s*` + regexp.QuoteMeta(key) + `=.*#\s*in\s+(\S+)\s+only\s*$`)
}

// renderedProject renders one template and returns its directory plus the
// archive-relative paths `civitai app submit` would UPLOAD.
func renderedProject(t *testing.T, tmpl scaffold.Template) (string, map[string]struct{}) {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Render(tmpl, dir, scaffold.Data{Slug: "demo-block", Name: "Demo Block"}); err != nil {
		t.Fatalf("render %s: %v", tmpl, err)
	}
	res, err := pkgzip.Build(dir)
	if err != nil {
		t.Fatalf("package %s: %v", tmpl, err)
	}
	if len(res.Files) == 0 {
		t.Fatalf("CONTROL failure: packaging %s produced no files, so every check below is vacuous", tmpl)
	}
	uploaded := make(map[string]struct{}, len(res.Files))
	for _, f := range res.Files {
		uploaded[f] = struct{}{}
	}
	return dir, uploaded
}

// ignoreOracle answers "would git ignore this path in the rendered project?"
// using git itself.
//
// 🔴 THIS USED TO BE A `path.Match` LOOP OVER THE `.gitignore` LINES, AND THAT
// SHAPE CANNOT BE MADE CORRECT. `.gitignore` is not a glob list: it has negation
// (`!pattern` re-includes), anchoring (a leading or embedded `/`), directory-only
// patterns (`dir/`), `**`, and last-match-wins ordering. The reimplementation
// silently dropped all of it, so appending `!.env.development.local` to the
// scaffolded template left this test asserting "ignored" about a file real git
// reports as NOT ignored — the guard's git leg lying in the reassuring
// direction, about the destination it had just told authors to trust.
type ignoreOracle struct {
	dir string
	env []string
}

// newIgnoreOracle initialises a throwaway repository in the rendered project so
// `git check-ignore` has something to answer against. It runs with global and
// system config neutralised, so a developer's own `core.excludesFile` cannot
// change the verdict here or in CI.
func newIgnoreOracle(t *testing.T, dir string) *ignoreOracle {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is not on PATH (%v) — this guard's second leg is 'the destination is git-ignored', "+
			"and the only honest way to answer that is to ask git. It is not skipped: a skipped safety leg "+
			"is the vacuous green this whole file exists to avoid.", err)
	}
	// Drop the inherited GIT_* pointers before adding ours: a GIT_DIR or
	// GIT_WORK_TREE in the environment (a git hook, a wrapper script) would
	// redirect `git init` and `git check-ignore` at some OTHER repository, and
	// the answer would look perfectly normal.
	var env []string
	for _, kv := range os.Environ() {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES":
			continue
		}
		env = append(env, kv)
	}
	o := &ignoreOracle{
		dir: dir,
		env: append(env,
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			"HOME="+t.TempDir(),
			"XDG_CONFIG_HOME="+t.TempDir(),
		),
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = o.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init in the rendered project: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return o
}

// ignores reports git's verdict for name. Exit 0 = ignored, 1 = not ignored;
// anything else is a broken oracle and fails the test rather than resolving to
// a boolean nobody measured.
func (o *ignoreOracle) ignores(t *testing.T, name string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", "--no-index", "--", name)
	cmd.Dir = o.dir
	cmd.Env = o.env
	err := cmd.Run()
	if err == nil {
		return true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false
	}
	t.Fatalf("git check-ignore %q returned an unusable status (%v) — the oracle is broken, so neither "+
		"answer below may be believed", name, err)
	return false
}

// commentBlockFor returns the run of `#` lines immediately preceding the
// `key=` declaration — the documentation an author reads before filling it in.
func commentBlockFor(body, key string) (string, bool) {
	lines := strings.Split(body, "\n")
	decl := -1
	for i, l := range lines {
		if strings.HasPrefix(l, key+"=") {
			decl = i
			break
		}
	}
	if decl < 0 {
		return "", false
	}
	start := decl
	for start > 0 && strings.HasPrefix(lines[start-1], "#") {
		start--
	}
	return strings.Join(lines[start:decl], "\n"), true
}

func TestUploadedDotenvNeverPointsASecretAtAnUploadedFile(t *testing.T) {
	// CONTROL, both directions: the packager's matcher must be able to answer
	// BOTH ways, or "the destination is excluded" is satisfied by a constant.
	if pkgzip.IsExcludedFile(".env.example") {
		t.Fatal("CONTROL failure: pkgzip excludes .env.example — nothing is uploaded, so this test proves nothing")
	}
	if !pkgzip.IsExcludedFile(".env.development.local") {
		t.Fatal("CONTROL failure: pkgzip uploads .env.development.local — the destination check cannot fail, so it checks nothing")
	}

	seenKeys := map[string]bool{}
	uploadedDotenvFiles := 0

	for _, tmpl := range scaffold.AllTemplates() {
		dir, uploaded := renderedProject(t, tmpl)
		oracle := newIgnoreOracle(t, dir)

		// CONTROL for the ignore oracle, both directions: it must be able to say
		// IGNORED and NOT IGNORED in this very tree, or every verdict below is a
		// constant. `.env.development.local` is the destination the templates
		// name; `.env.example` is the file they must never name.
		if !oracle.ignores(t, ".env.development.local") && tmpl == scaffold.PageMoney {
			t.Errorf("%s: git does NOT ignore .env.development.local in the scaffolded project — the file the "+
				"templates send secrets to is committable", tmpl)
		}
		if oracle.ignores(t, ".env.example") {
			t.Errorf("%s: CONTROL failure: git ignores .env.example in the scaffolded project, so the ignore "+
				"check below cannot discriminate", tmpl)
		}

		for rel := range uploaded {
			base := filepath.Base(rel)
			// 🔴 THE SEAM WITH `outputName`. `pkgzip` recognises a dotenv file by
			// its LEADING DOT, and the scaffold's embedded templates carry
			// undotted names (go:embed cannot embed a dot-leading file), so a
			// rewrite that misses one ships a dotenv file the packager never
			// classified — and it would slip past the `.env` filter just below
			// too, i.e. invisible to this entire test.
			if strings.HasPrefix(base, "env.") {
				t.Errorf("🔴 %s: %s is UPLOADED with an UNDOTTED name. `scaffold.outputName` must rewrite "+
					"`env.*` to `.env.*`: as %q it is not a dotenv file to `pkgzip.isExcludedFile`, so it ships "+
					"whatever it holds, and no check in this file would ever look at it.", tmpl, rel, base)
			}
			if !strings.HasPrefix(base, ".env") {
				continue
			}
			uploadedDotenvFiles++
			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("read uploaded %s: %v", rel, err)
			}
			body := string(b)

			for _, m := range assignmentRE.FindAllStringSubmatch(body, -1) {
				key, value := m[1], strings.TrimSpace(m[2])
				seenKeys[key] = true
				_, secret := secretEnvKeys[key]
				_, public := publicEnvKeys[key]
				if secret == public {
					where := "NEITHER ledger in this file"
					if secret {
						where = "BOTH ledgers in this file"
					}
					t.Errorf("🔴 %s/%s declares %s, which is in %s. Classify it: a key whose value is a live "+
						"credential goes in secretEnvKeys (and then its comment block must name a destination this "+
						"packager EXCLUDES), anything else in publicEnvKeys. This file is UPLOADED to the platform "+
						"and to a human moderator reviewer.", tmpl, rel, key, where)
					continue
				}
				if !secret {
					continue
				}

				// 🔴 THE ONE DETERMINISTIC CHECK: the declaration in an UPLOADED
				// file must be EMPTY. Everything else in this test is about where
				// the documentation points; this is about what the shipped bytes
				// hold, and it is the only assertion here that a real credential
				// in a real upload cannot survive. It is not implied by any of
				// them — a token pasted after the `=` with the destination
				// annotation left intact passed every other check in this file.
				if value != "" {
					t.Errorf("🔴 %s/%s declares %s=%s — a NON-EMPTY value for %s in a file `civitai app submit` "+
						"UPLOADS to the platform and to a human moderator reviewer. The scaffolded declaration must "+
						"be bare `%s=`; the real value belongs in the destination its comment names.",
						tmpl, rel, key, value, secretEnvKeys[key], key)
				}

				block, ok := commentBlockFor(body, key)
				if !ok || strings.TrimSpace(block) == "" {
					t.Errorf("🔴 %s/%s: %s (%s) has no documentation block at all", tmpl, rel, key, secretEnvKeys[key])
					continue
				}
				dm := destinationRE(key).FindStringSubmatch(block)
				if dm == nil {
					t.Errorf("🔴 %s/%s: %s (%s) does not name WHERE the real value goes.\n"+
						"This file is UPLOADED by `civitai app submit` — to the platform and to a human moderator "+
						"reviewer — so a value placed \"here\" leaks whatever git does. Add the destination "+
						"annotation the CIVITAI_HOST_KEY block uses, naming a file the packager excludes:\n"+
						"    #   %s=<placeholder>   # in .env.development.local only\n"+
						"Block read:\n%s", tmpl, rel, key, secretEnvKeys[key], key, block)
					continue
				}
				dest := strings.Trim(dm[1], "`'\"")
				if !pkgzip.IsExcludedFile(filepath.Base(dest)) {
					t.Errorf("🔴 %s/%s: %s is sent to %q, which `civitai app submit` UPLOADS. "+
						"The destination for a live credential must be a file the packager excludes.",
						tmpl, rel, key, dest)
				}
				// Corroboration, not an independent leg: a destination the
				// matcher excludes cannot be in this render's file list, so this
				// only fires when the two disagree — which would mean the
				// packager's name rule and its walk had come apart.
				if _, alsoUploaded := uploaded[dest]; alsoUploaded {
					t.Errorf("🔴 %s/%s: %s is sent to %q, which is IN THE PACKAGE this render produced",
						tmpl, rel, key, dest)
				}
				if !oracle.ignores(t, dest) {
					t.Errorf("🔴 %s/%s: %s is sent to %q, which `git check-ignore` says is NOT ignored in the "+
						"scaffolded project — the advice is safe against the upload and unsafe against a commit",
						tmpl, rel, key, dest)
				}
			}
		}
	}

	// Count floor. NOTE its reachability honestly: removing `.env.example` from
	// `keptEnvFiles` is caught EARLIER, by the matcher control at the top of this
	// function, so that mutation never gets here. What reaches this line is the
	// other way of emptying the set — the templates ceasing to render an uploaded
	// dotenv file at all (delete `env.example.tmpl` + `env.production.tmpl`).
	if uploadedDotenvFiles == 0 {
		t.Fatal("CONTROL failure: no template rendered a dotenv file that the packager uploads, so every " +
			"assertion above ran zero times. Either the templates stopped shipping .env.example (fine — delete " +
			"this test) or the render/package plumbing broke (not fine).")
	}

	// The ledger's other direction: a classified key no template declares is
	// stale, and stale entries are how a ledger stops describing the tree.
	var stale []string
	for k := range secretEnvKeys {
		if !seenKeys[k] {
			stale = append(stale, k+" (secret)")
		}
	}
	for k := range publicEnvKeys {
		if !seenKeys[k] {
			stale = append(stale, k+" (public)")
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("classified but declared by no UPLOADED dotenv file: %v — remove them, or the ledger is "+
			"describing a tree that no longer exists", stale)
	}
}

// TestSecretKeyDestinationRegexpCanFail is the negative control for the shape
// this guard depends on: an annotation-less block and one naming an uploaded
// file must both be REJECTED. Without it, a regexp that matched everything (or a
// destination check reading a constant) would let the whole test above pass
// while the hazard was back.
func TestSecretKeyDestinationRegexpCanFail(t *testing.T) {
	const key = "VITE_LIVE_BLOCK_TOKEN"
	cases := []struct {
		name     string
		block    string
		wantDest string // "" = the block must be rejected outright
	}{
		{
			name:  "the pre-#380 text: a paste instruction naming no file",
			block: "# LIVE mode only. A short-lived, scoped dev block token. Paste\n# it here to run `dev:live` against the real backend. ⚠️ never commit one.",
		},
		{
			name:  "prose naming the right file, but no annotation",
			block: "# put the real value in the git-ignored .env.development.local, never here",
		},
		{
			name:     "the annotation",
			block:    "# blah\n#   VITE_LIVE_BLOCK_TOKEN=<the minted token>   # in .env.development.local only",
			wantDest: ".env.development.local",
		},
		{
			name:     "an annotation pointing at an UPLOADED file",
			block:    "#   VITE_LIVE_BLOCK_TOKEN=<the minted token>   # in .env.example only",
			wantDest: ".env.example",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := destinationRE(key).FindStringSubmatch(tc.block)
			if tc.wantDest == "" {
				if m != nil {
					t.Fatalf("block was ACCEPTED with destination %q; it names no destination and must be rejected", m[1])
				}
				return
			}
			if m == nil {
				t.Fatalf("block was rejected; expected destination %q", tc.wantDest)
			}
			if m[1] != tc.wantDest {
				t.Fatalf("destination = %q, want %q", m[1], tc.wantDest)
			}
			// And the destination check must separate the two accepted cases.
			excluded := pkgzip.IsExcludedFile(m[1])
			if want := tc.wantDest != ".env.example"; excluded != want {
				t.Fatalf("pkgzip.IsExcludedFile(%q) = %v, want %v — the destination check does not discriminate",
					m[1], excluded, want)
			}
		})
	}
}

// normalizeDotenvProse reduces a dotenv file to the WORDS it tells an author,
// dropping the comment markers and the line wrapping. Both are layout, and
// pinning layout turns every reflow into a re-approval nobody reads.
func normalizeDotenvProse(body string) string {
	var words []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#")
		words = append(words, strings.Fields(line)...)
	}
	return strings.Join(words, " ")
}

// updateUploadedDotenvGolden re-approves the pinned copy below.
var updateUploadedDotenvGolden = flag.Bool("update", false, "rewrite the pinned uploaded-dotenv golden")

const uploadedDotenvGoldenPath = "testdata/uploaded_dotenv.golden.txt"

// 🔴 TestUploadedDotenvIsTheReviewedCopy PINS THE WHOLE TEXT OF EVERY DOTENV
// FILE THAT LEAVES THE AUTHOR'S MACHINE, because the guard above is a check on
// the DESTINATION ANNOTATION and cannot see prose added around it. Item 28
// records the same conclusion reached twice on the money copy: a banned-phrase
// ledger loses to the next paraphrase, and what closes ADDITION is a golden.
//
// These files are read by a human moderator reviewer, so a diff here is a
// review, not a chore:
//
//	go test ./internal/scaffold -run TestUploadedDotenvIsTheReviewedCopy -update
//
// READ the diff, and ask it of the VALUES as well as the prose: "does any
// sentence invite a live credential into a file `civitai app submit` uploads,
// warn only about git when the upload is the wider channel, or has a
// declaration acquired a value?" That last question is not left to a reader's
// diligence — `TestUploadedDotenvNeverPointsASecretAtAnUploadedFile` fails on a
// non-empty secret declaration — but the earlier phrasing of this instruction
// asked only about SENTENCES, and a bare `KEY=<token>` is not a sentence.
//
// Comparison strips comment markers and collapses whitespace, so a pure re-wrap
// passes — that carries no claim. (Measured: without the marker strip a rewrap
// DID fail, because the `#` moves between words; the first version of this
// comment said re-wraps passed and was wrong until the strip was added.)
//
// Residual: it pins only the files the packager UPLOADS. Prose in
// `README.md.tmpl` (also uploaded) is covered by neither guard here.
func TestUploadedDotenvIsTheReviewedCopy(t *testing.T) {
	var b strings.Builder
	files := 0
	for _, tmpl := range scaffold.AllTemplates() {
		dir, uploaded := renderedProject(t, tmpl)
		var rels []string
		for rel := range uploaded {
			if strings.HasPrefix(filepath.Base(rel), ".env") {
				rels = append(rels, rel)
			}
		}
		sort.Strings(rels)
		for _, rel := range rels {
			raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("read uploaded %s: %v", rel, err)
			}
			fmt.Fprintf(&b, "%s %s :: %s\n", tmpl, rel, normalizeDotenvProse(string(raw)))
			files++
		}
	}
	if files == 0 {
		t.Fatal("CONTROL failure: no uploaded dotenv file was rendered, so an empty golden would match forever")
	}
	got := b.String()

	if *updateUploadedDotenvGolden {
		if err := os.MkdirAll(filepath.Dir(uploadedDotenvGoldenPath), 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(uploadedDotenvGoldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", uploadedDotenvGoldenPath, err)
		}
		t.Logf("updated %s — READ THE DIFF", uploadedDotenvGoldenPath)
		return
	}
	want, err := os.ReadFile(uploadedDotenvGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v\n\nRe-approve deliberately with -update and review the file it writes.", uploadedDotenvGoldenPath, err)
	}
	if string(want) != got {
		t.Errorf("the text of a dotenv file `civitai app submit` UPLOADS changed.\n\n  want: %s\n\n  got:  %s\n\n"+
			"🔴 These files reach the platform and a human moderator reviewer. Re-approve with `-update` only "+
			"after checking that no new sentence invites a live credential into one of them.",
			strings.TrimSpace(string(want)), strings.TrimSpace(got))
	}
}
