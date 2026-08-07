package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// serveURLRe finds the `https://<host>.civit.ai/` the scaffold echoes. The
// capture group is the whole thing this file exists to assert on.
var serveURLRe = regexp.MustCompile(`https://([^\s]+)\.civit\.ai/`)

// TestScaffoldServingURLIsBuiltFromTheSlugNotTheDisplayName pins the OPERAND of
// the echoed URL, which nothing did before.
//
// 🔴 SWAPPING `slug` FOR `display` IN THAT `Fprintf` SURVIVED THE WHOLE SUITE.
// Every existing assertion was a `strings.Contains(stdout, slug)`, and the FIRST
// `%s` on that line still carried the slug — so the mutant printed
// `https://Widget Machine.civit.ai/` as the app's "permanent public id" and no
// test could see it. The fixture keeps slug, display name and directory pairwise
// distinct AND substring-disjoint, so no assertion here can be satisfied by the
// wrong value.
func TestScaffoldServingURLIsBuiltFromTheSlugNotTheDisplayName(t *testing.T) {
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

	m := serveURLRe.FindAllStringSubmatch(stdout, -1)
	if len(m) != 1 {
		t.Fatalf("expected exactly one https://<id>.civit.ai/ in the output, got %d:\n%s", len(m), stdout)
	}
	if got := m[0][1]; got != "qqq-block-id" {
		t.Errorf("the serving URL must be built from the blockId, got host %q (want qqq-block-id):\n%s", got, stdout)
	}
	// The display name must not appear inside the URL at all — the mutation this
	// test exists for produces `https://Widget Machine.civit.ai/`.
	if strings.Contains(stdout, "https://Widget") {
		t.Errorf("the display name leaked into the serving URL:\n%s", stdout)
	}
}

// TestScaffoldDoesNotPromiseTheURLServesYet — issue #260, same class as the
// "validates clean" promise two lines up in the SAME output block.
//
// 🔴 THE URL IS GUARANTEED TO 404 AT THE MOMENT IT IS PRINTED. The subdomain is
// only programmed on approval + deploy (README: "Before approval,
// https://<blockId>.civit.ai/ 404s"), and `app status` already says so ("Not
// live yet — … only serves after the app is approved and deployed"). Printing it
// bare as the app's "permanent public id" walks a first-time author straight
// into a 404 they were told to expect to work. The qualifier must sit on the
// SAME LINE as the URL, or a later edit can move the URL out from under it.
func TestScaffoldDoesNotPromiseTheURLServesYet(t *testing.T) {
	tmp := t.TempDir()
	stdout, _, err := run(t, "app", "create", "my-block", "--dir", filepath.Join(tmp, "out"), "--template", "static")
	if err != nil {
		t.Fatalf("app create: %v\n%s", err, stdout)
	}

	var urlLine string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, ".civit.ai/") {
			urlLine = line
			break
		}
	}
	if urlLine == "" {
		t.Fatalf("expected the scaffold to echo a serving URL:\n%s", stdout)
	}
	// The wording deliberately matches `app status`'s "approved and deployed",
	// so the two surfaces tell the author the same thing (app_status.go).
	if !strings.Contains(urlLine, "approved and deployed") {
		t.Errorf("the serving URL must be qualified on its own line — it 404s until approval:\n%s", urlLine)
	}
	// And the blockId is still labelled, on a line of its own.
	if !strings.Contains(stdout, "blockId: my-block") {
		t.Errorf("output must still LABEL the blockId:\n%s", stdout)
	}
}

// promptRecorder captures what the scaffold asked the interactive form for.
type promptRecorder struct {
	calls           int
	askedName       bool
	defaultTemplate string
}

func stubPrompt(t *testing.T, rec *promptRecorder, reply scaffoldInputs) {
	t.Helper()
	origTTY, origPrompt := stdinIsTTY, scaffoldPromptFn
	t.Cleanup(func() { stdinIsTTY = origTTY; scaffoldPromptFn = origPrompt })
	stdinIsTTY = func() bool { return true }
	scaffoldPromptFn = func(_ *cobra.Command, defaultTemplate string, askName bool) (scaffoldInputs, error) {
		rec.calls++
		rec.askedName = askName
		rec.defaultTemplate = defaultTemplate
		return reply, nil
	}
}

// TestSlugFlagDropsTheNameFieldButKeepsTheTemplatePrompt — the regression this
// PR introduced and this test closes.
//
// 🔴 `--slug` USED TO SUPPRESS THE WHOLE PROMPT, on the reasoning that it
// "supplies the one thing the prompt exists to collect". `runScaffoldForm`
// collects a name AND a TEMPLATE, so a TTY user running
// `civitai app create --slug my-app` silently got page-money with no template
// choice — a question they were asked before the flag existed. The mutant
// deleting `slugFlag == ""` from that guard survived with ZERO failures because
// nothing covered the suppression at all.
//
// The assertions are structural, not "some prompt happened": the prompt must be
// CALLED, it must be told NOT to ask for a name, and the template it returns
// must be the one the project is built from — which is only observable because
// the reply (page-vite) differs from `app create`'s default (page-money).
func TestSlugFlagDropsTheNameFieldButKeepsTheTemplatePrompt(t *testing.T) {
	var rec promptRecorder
	stubPrompt(t, &rec, scaffoldInputs{name: "", template: "page-vite"})

	tmp := t.TempDir()
	chdir(t, tmp)
	stdout, _, err := run(t, "app", "create", "--slug", "solo-block")
	if err != nil {
		t.Fatalf("app create --slug on a TTY: %v\n%s", err, stdout)
	}

	if rec.calls != 1 {
		t.Fatalf("--slug must NOT suppress the prompt: it was called %d time(s), want 1", rec.calls)
	}
	if rec.askedName {
		t.Error("--slug settles the identity, so the NAME field must be dropped from the form")
	}
	if rec.defaultTemplate != "page-money" {
		t.Errorf("the form should be pre-selected with the command's default template, got %q", rec.defaultTemplate)
	}
	// The prompted template is what got built — the whole point of still asking.
	if !strings.Contains(stdout, "(page-vite)") {
		t.Errorf("the template chosen at the prompt must be the one scaffolded:\n%s", stdout)
	}
	if got := readBlockID(t, filepath.Join(tmp, "solo-block")); got != "solo-block" {
		t.Errorf("blockId = %q, want solo-block", got)
	}
}

// TestNoSlugFlagStillAsksForTheName is the other direction, and it is what stops
// the test above from being satisfied by a form that never asks for a name at
// all. Without --slug there is no identity yet, so both fields must be collected.
func TestNoSlugFlagStillAsksForTheName(t *testing.T) {
	var rec promptRecorder
	stubPrompt(t, &rec, scaffoldInputs{name: "prompted-block", template: "static"})

	tmp := t.TempDir()
	chdir(t, tmp)
	if stdout, _, err := run(t, "app", "create"); err != nil {
		t.Fatalf("app create on a TTY: %v\n%s", err, stdout)
	}
	if rec.calls != 1 {
		t.Fatalf("the prompt must run when no name and no --slug are supplied (calls=%d)", rec.calls)
	}
	if !rec.askedName {
		t.Error("with no --slug the form still has to collect the name")
	}
}

// TestScaffoldRefusesAnInvalidUTF8Name — the manifest half of the silent-loss
// class #259 is about. `civitai app create $'caf\xe9 app'` used to derive the
// blockId `caf-app` with rc 0 AND write a block.manifest.json holding the raw
// 0xE9, which is not valid UTF-8 and which `validate.ManifestOnly` accepted.
// scaffold.Slugify now refuses the derivation; this pins the other route in,
// because --slug bypasses derivation entirely and the name still lands in the
// manifest verbatim.
//
// Classification is asserted with errors.Is, never message text (AGENTS item 7).
func TestScaffoldRefusesAnInvalidUTF8Name(t *testing.T) {
	const badName = "caf\xe9 app"
	if utf8.ValidString(badName) {
		t.Fatal("fixture is broken: badName must NOT be valid UTF-8")
	}

	t.Run("as the positional name, with --slug", func(t *testing.T) {
		tmp := t.TempDir()
		dest := filepath.Join(tmp, "out")
		_, _, err := run(t, "app", "create", badName, "--slug", "cafe-app", "--dir", dest, "--template", "static")
		if err == nil {
			t.Fatal("an invalid-UTF-8 display name must be refused")
		}
		if !errors.Is(err, ErrUsage) {
			t.Errorf("a bad name VALUE is a usage error: %v", err)
		}
		if _, statErr := os.Stat(dest); statErr == nil {
			t.Error("refused, but a project was still written")
		}
	})

	t.Run("as --name", func(t *testing.T) {
		tmp := t.TempDir()
		dest := filepath.Join(tmp, "out")
		_, _, err := run(t, "app", "create", "my-block", "--name", badName, "--dir", dest, "--template", "static")
		if err == nil {
			t.Fatal("an invalid-UTF-8 --name must be refused")
		}
		if !errors.Is(err, ErrUsage) {
			t.Errorf("a bad --name VALUE is a usage error: %v", err)
		}
	})

	t.Run("derivation refuses it too, with no --slug", func(t *testing.T) {
		tmp := t.TempDir()
		_, _, err := run(t, "app", "create", badName, "--dir", filepath.Join(tmp, "out"), "--template", "static")
		if err == nil {
			t.Fatal("invalid UTF-8 must not derive a slug")
		}
		if !errors.Is(err, ErrUsage) {
			t.Errorf("a bad name VALUE is a usage error: %v", err)
		}
	})

	// POSITIVE CONTROL: the same shape with VALID UTF-8 succeeds and writes a
	// manifest that is valid UTF-8 — so the rows above are evidence about the
	// bytes, not about a build that refuses everything.
	t.Run("a valid non-ASCII name still scaffolds with --slug", func(t *testing.T) {
		tmp := t.TempDir()
		dest := filepath.Join(tmp, "out")
		if _, _, err := run(t, "app", "create", "Café App", "--slug", "cafe-app", "--dir", dest, "--template", "static"); err != nil {
			t.Fatalf("a valid UTF-8 name must still work: %v", err)
		}
		b, readErr := os.ReadFile(filepath.Join(dest, "block.manifest.json"))
		if readErr != nil {
			t.Fatalf("read manifest: %v", readErr)
		}
		if !utf8.Valid(b) {
			t.Error("the written manifest must be valid UTF-8")
		}
	})
}
