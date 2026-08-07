package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// manifestBlockID / manifestName read what the scaffold actually WROTE. The
// manifest is the authority on the app's identity; the printed line is a claim
// ABOUT it, so the tests below assert both and never infer one from the other.
// (readBlockID already exists in app_dev_token_rename_test.go — reused here.)
func manifestBlockID(t *testing.T, dir string) string {
	t.Helper()
	got := readBlockID(t, dir)
	if got == "" {
		t.Fatalf("manifest in %s has no blockId", dir)
	}
	return got
}

func manifestName(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return m.Name
}

// TestScaffoldEchoesTheDerivedSlugWithADir is the half of issue #259 that makes
// the derivation bug INVISIBLE. `printScaffoldResult` took `slug` and never used
// it — a dead parameter — so no line of output named the blockId. With the
// default directory you can infer it from the directory name; with --dir the
// only copy is inside block.manifest.json.
//
// The assertion is deliberately not "the output contains the slug somewhere":
// the slug here (`ueber-app`) shares no substring with the display name or the
// directory, so a green cannot be earned by an unrelated line.
func TestScaffoldEchoesTheDerivedSlugWithADir(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "somewhere-else")

	stdout, _, err := run(t, "app", "create", "Uber App", "--slug", "ueber-app", "--dir", dest, "--template", "static")
	if err != nil {
		t.Fatalf("app create: %v\n%s", err, stdout)
	}
	if got := manifestBlockID(t, dest); got != "ueber-app" {
		t.Fatalf("manifest blockId = %q, want ueber-app", got)
	}
	if !strings.Contains(stdout, "ueber-app") {
		t.Errorf("output must echo the blockId (the app's permanent public id):\n%s", stdout)
	}
	if !strings.Contains(stdout, "blockId") {
		t.Errorf("output must LABEL the id, not just happen to contain the string:\n%s", stdout)
	}
	// The directory name is the one place the slug is normally readable; here it
	// is not, which is what makes this the load-bearing case.
	if strings.Contains(filepath.Base(dest), "ueber-app") {
		t.Fatal("fixture is broken: the --dir name must NOT contain the slug")
	}
}

// TestScaffoldEchoesTheSlugWithTheDefaultDir is the same claim for the default
// directory. It is a weaker observation on its own (the directory name is the
// slug), so it asserts the LABEL too.
func TestScaffoldEchoesTheSlugWithTheDefaultDir(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	stdout, _, err := run(t, "app", "create", "My Cool Block", "--template", "static")
	if err != nil {
		t.Fatalf("app create: %v\n%s", err, stdout)
	}
	if got := manifestBlockID(t, filepath.Join(tmp, "my-cool-block")); got != "my-cool-block" {
		t.Fatalf("manifest blockId = %q, want my-cool-block", got)
	}
	if !strings.Contains(stdout, "blockId: my-cool-block") {
		t.Errorf("output must echo the blockId even with the default dir:\n%s", stdout)
	}
}

// TestSlugFlagIsTheEscapeHatchForARefusedName is the reason the refusal in
// scaffold.Slugify is shippable at all: "Café App" no longer derives (it used to
// silently mint `caf-app`), and --slug is how the author says what they want
// instead.
func TestSlugFlagIsTheEscapeHatchForARefusedName(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "out")

	// Without --slug the name is refused, as a usage error (exit 2).
	if _, _, err := run(t, "app", "create", "Café App", "--dir", filepath.Join(tmp, "refused"), "--template", "static"); err == nil {
		t.Fatal("expected a non-slugifiable name to be refused")
	} else if !errors.Is(err, ErrUsage) {
		t.Errorf("a refused name is a usage error: errors.Is(err, ErrUsage) = false (%v)", err)
	}

	// With --slug it succeeds and the manifest carries the explicit slug.
	stdout, _, err := run(t, "app", "create", "Café App", "--slug", "cafe-app", "--dir", dest, "--template", "static")
	if err != nil {
		t.Fatalf("app create with --slug: %v\n%s", err, stdout)
	}
	if got := manifestBlockID(t, dest); got != "cafe-app" {
		t.Errorf("manifest blockId = %q, want cafe-app", got)
	}
	if got := manifestName(t, dest); got != "Café App" {
		t.Errorf("display name = %q, want the name the author typed (Café App)", got)
	}
	if !strings.Contains(stdout, "cafe-app") {
		t.Errorf("output must echo the explicit blockId too:\n%s", stdout)
	}
}

// TestSlugFlagRejectsAnInvalidValue: --slug bypasses derivation, so it is the
// only thing standing between a typo and an invalid manifest. It goes through
// the same ValidateSlug the server contract is written from, and a bad VALUE is
// a usage error — pinned with errors.Is, never message text (AGENTS item 7).
func TestSlugFlagRejectsAnInvalidValue(t *testing.T) {
	for _, bad := range []string{
		"Cafe-App",              // uppercase
		"1st-app",               // must start with a letter
		"ab",                    // under the 3-char floor
		"cafe app",              // space
		"cafe-",                 // must end alphanumeric
		strings.Repeat("a", 41), // over the 40-char cap
	} {
		t.Run(bad, func(t *testing.T) {
			tmp := t.TempDir()
			_, _, err := run(t, "app", "create", "my-block", "--slug", bad, "--dir", filepath.Join(tmp, "out"), "--template", "static")
			if err == nil {
				t.Fatalf("--slug %q should be refused", bad)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("--slug %q must be a usage error: %v", bad, err)
			}
			if _, statErr := os.Stat(filepath.Join(tmp, "out")); statErr == nil {
				t.Errorf("--slug %q was refused but a project was still written", bad)
			}
		})
	}
}

// TestNameSlugAndDirAreFullyIndependent. The help text has promised name/dir
// independence for a while; --slug makes the third axis real. All three values
// are pairwise distinct AND share no substring, so no assertion here can be
// satisfied by the wrong one.
func TestNameSlugAndDirAreFullyIndependent(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "zzz-output-dir")

	stdout, _, err := run(t, "app", "create", "ignored-positional",
		"--name", "Widget Machine",
		"--slug", "qqq-block-id",
		"--dir", dest,
		"--template", "static")
	if err != nil {
		t.Fatalf("app create: %v\n%s", err, stdout)
	}
	if got := manifestBlockID(t, dest); got != "qqq-block-id" {
		t.Errorf("blockId = %q, want qqq-block-id", got)
	}
	if got := manifestName(t, dest); got != "Widget Machine" {
		t.Errorf("display name = %q, want Widget Machine", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "block.manifest.json")); err != nil {
		t.Errorf("project should be written to --dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "qqq-block-id")); err == nil {
		t.Error("--dir was given, so nothing may be written to ./<slug>")
	}
	for _, want := range []string{"qqq-block-id", "Widget Machine", "zzz-output-dir"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output should name all three of slug/display/dir; missing %q:\n%s", want, stdout)
		}
	}
}

// TestSlugFlagAloneNeedsNoName — the axes really are independent: --slug on its
// own is enough to scaffold, with the display name title-cased from it.
func TestSlugFlagAloneNeedsNoName(t *testing.T) {
	tmp := t.TempDir()
	chdir(t, tmp)

	stdout, _, err := run(t, "app", "create", "--slug", "solo-block", "--template", "static")
	if err != nil {
		t.Fatalf("app create --slug (no name): %v\n%s", err, stdout)
	}
	if got := manifestBlockID(t, filepath.Join(tmp, "solo-block")); got != "solo-block" {
		t.Errorf("blockId = %q, want solo-block", got)
	}
	if got := manifestName(t, filepath.Join(tmp, "solo-block")); got != "Solo Block" {
		t.Errorf("display name = %q, want Solo Block (title-cased from the slug)", got)
	}
}

// TestAsciiDerivationIsUnchanged is the CONTROL for the refusal: every name that
// derived a slug before must derive the byte-identical slug now, through the
// real command. These rows are expected to have been green before the change —
// they are an invariant guard, not regression coverage, and their job is to fail
// if the refusal predicate ever widens to swallow ordinary punctuation.
func TestAsciiDerivationIsUnchanged(t *testing.T) {
	cases := map[string]string{
		"My Cool Block":         "my-cool-block",
		"my-block":              "my-block",
		"  Spaced   Out  ":      "spaced-out",
		"Foo/Bar_Baz.Qux!":      "foo-bar-baz-qux",
		"Widget (v2) — Pro":     "widget-v2-pro", // an em dash is a separator, not content
		"a & b + c = d":         "a-b-c-d",
		"Trailing punctuation.": "trailing-punctuation",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			dest := filepath.Join(tmp, "out")
			stdout, _, err := run(t, "app", "create", name, "--dir", dest, "--template", "static")
			if err != nil {
				t.Fatalf("app create %q: %v\n%s", name, err, stdout)
			}
			if got := manifestBlockID(t, dest); got != want {
				t.Errorf("blockId for %q = %q, want %q", name, got, want)
			}
		})
	}
}

// TestScaffoldHelpDoesNotPromiseAValidateCleanScaffold — issue #260 item 1.
// `app create --help` said the scaffold "validates clean"; a fresh page-money /
// page-vite project FAILS `civitai app validate` until `npm install` writes the
// lockfile the platform build installs from. The check is right; the promise was
// wrong.
func TestScaffoldHelpDoesNotPromiseAValidateCleanScaffold(t *testing.T) {
	help, _, err := run(t, "app", "create", "--help")
	if err != nil {
		t.Fatalf("app create --help: %v", err)
	}
	if strings.Contains(help, "validates clean") {
		t.Errorf("help must not claim the scaffold `validates clean` — it does not until `npm install` runs:\n%s", help)
	}
	// And it must say what the real precondition is, rather than going silent.
	for _, want := range []string{"civitai app validate", "npm install"} {
		if !strings.Contains(help, want) {
			t.Errorf("help should name the validate/install relationship; missing %q:\n%s", want, help)
		}
	}
}

// TestNextStepsSayValidateNeedsInstall — the same fact at the other surface the
// author actually reads: the numbered next-steps block printed right after the
// scaffold is written. The `static` template installs nothing and DOES validate
// clean, so it must NOT carry the caveat; that pair is what stops the assertion
// from passing on a line printed unconditionally.
func TestNextStepsSayValidateNeedsInstall(t *testing.T) {
	t.Run("page-money warns", func(t *testing.T) {
		tmp := t.TempDir()
		stdout, _, err := run(t, "app", "create", "my-block", "--dir", filepath.Join(tmp, "out"))
		if err != nil {
			t.Fatalf("app create: %v\n%s", err, stdout)
		}
		if !strings.Contains(stdout, "npm install") {
			t.Fatalf("expected an npm install step:\n%s", stdout)
		}
		if !strings.Contains(stdout, "civitai app validate` fails until you do") {
			t.Errorf("next steps must say validate fails until npm install has run:\n%s", stdout)
		}
	})
	t.Run("page-vite warns", func(t *testing.T) {
		tmp := t.TempDir()
		stdout, _, err := run(t, "app", "create", "my-block", "--dir", filepath.Join(tmp, "out"), "--template", "page-vite")
		if err != nil {
			t.Fatalf("app create: %v\n%s", err, stdout)
		}
		if !strings.Contains(stdout, "civitai app validate` fails until you do") {
			t.Errorf("next steps must say validate fails until npm install has run:\n%s", stdout)
		}
	})
	t.Run("static does not (it really does validate clean)", func(t *testing.T) {
		tmp := t.TempDir()
		stdout, _, err := run(t, "app", "create", "my-block", "--dir", filepath.Join(tmp, "out"), "--template", "static")
		if err != nil {
			t.Fatalf("app create: %v\n%s", err, stdout)
		}
		if strings.Contains(stdout, "fails until you do") {
			t.Errorf("static installs nothing and validates clean — it must not carry the caveat:\n%s", stdout)
		}
	})
}

// TestScaffoldFromErrorShipsNoEngineeringNote — issue #260 item 6. `--from` used
// to answer with a multi-line internal note ("TODO(server): expose a read
// endpoint …") that buried the one useful sentence. The context belongs in a
// source comment; the user gets an ordinary actionable CLI error.
func TestScaffoldFromErrorShipsNoEngineeringNote(t *testing.T) {
	for _, sub := range []string{"create", "init"} {
		t.Run(sub, func(t *testing.T) {
			tmp := t.TempDir()
			_, errOut, err := run(t, "app", sub, "forked-app", "--from", "some-published-app", "--dir", filepath.Join(tmp, "out"))
			if err == nil {
				t.Fatal("expected --from to fail")
			}
			blob := err.Error() + errOut
			for _, leaked := range []string{"TODO(", "TODO", "endpoint that returns"} {
				if strings.Contains(blob, leaked) {
					t.Errorf("user-facing error leaks an engineering note (%q):\n%s", leaked, blob)
				}
			}
			if !strings.Contains(blob, "--from is not available yet") {
				t.Errorf("error should say plainly that the flag is unavailable:\n%s", blob)
			}
			// Actionable: it names what to do instead, with the real command path.
			if !strings.Contains(blob, "civitai app "+sub+" <name>") {
				t.Errorf("error should name the command to run instead:\n%s", blob)
			}
			if strings.Count(blob, "\n") > 1 {
				t.Errorf("error should be a single line, not a multi-line note:\n%s", blob)
			}
			// Unchanged classification: an unavailable feature is not a
			// malformed invocation, so it stays generic (exit 1), not exit 2.
			if errors.Is(err, ErrUsage) {
				t.Error("--from is a real flag with a real value: it must not be tagged ErrUsage")
			}
		})
	}
}
