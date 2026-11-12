package cli_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// app_pull_invocation_test.go: every `civitai app pull` this repo tells a reader
// to RUN passes the app in --app.
//
// 🔴 THE DEFECT. `app pull` is the one `app` subcommand whose positional is the
// DIRECTORY and whose app is a flag (`civitai app pull [dir] --app <slug>`), so
// the shape every other subcommand uses, `civitai app pull <slug>`, is refused
// with exit 2. That wrong shape shipped twice in one release, in text written
// for a developer to copy: the `app feedback` detail line told the reader to
// run `civitai app pull <slug>`, and so did the README's feedback section. The
// command's own Args check names the mistake, but only to the person who has
// already typed it.
//
// WHAT IS SCANNED: README.md and every non-test .go and .md file outside
// claudedocs/ (history, quoted as it was) and testdata/. An invocation is the
// text after `civitai app pull` up to the next back-quote or newline. It must
// carry --app, or be empty (the command named in prose, as in "`civitai app
// pull` clones it"), or be exactly --help. The back-quote ends a Markdown span
// and a Go raw string; a Go double-quoted string does NOT end it, which is what
// lets the guard see `"civitai app pull " + slug`, the shape that shipped.
//
// WHAT IT DOES NOT CATCH: an invocation assembled so that the words
// `civitai app pull` never appear together in one string, or one wrapped across
// a newline. It is a regression guard on the shape that shipped.

var appPullInvocation = regexp.MustCompile("civitai app pull([^`\\n]*)")

// appPullInvocationOK reports whether the text after `civitai app pull` is a
// correct invocation, or no invocation at all.
func appPullInvocationOK(rest string) bool {
	r := strings.TrimSpace(rest)
	return r == "" || r == "--help" || strings.Contains(r, "--app")
}

func TestAppPullInvocationRule(t *testing.T) {
	for _, c := range []struct {
		line string
		ok   bool
	}{
		{"`civitai app pull` clones it", true},
		{"civitai app pull --app my-block                # clone into ./my-block", true},
		{"`civitai app pull . --app my-block`", true},
		{"📖 `civitai app pull --help`.", true},
		// The two shapes that shipped.
		{"the repo `civitai app pull <slug>` clones", false},
		{`item += ": ` + "`civitai app pull \" + slug + \"`" + `, then git checkout it"`, false},
		{"run civitai app pull my-block", false},
	} {
		m := appPullInvocation.FindStringSubmatch(c.line)
		if m == nil {
			t.Errorf("%q: the matcher found no invocation at all", c.line)
			continue
		}
		if got := appPullInvocationOK(m[1]); got != c.ok {
			t.Errorf("%q: ok=%v, want %v (rest %q)", c.line, got, c.ok, m[1])
		}
	}
}

func TestEveryAppPullInvocationPassesTheAppFlag(t *testing.T) {
	var bad []string
	withFlag := map[string]int{} // correct invocations, by file extension
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "testdata", "claudedocs":
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") ||
			!(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".md")) {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range appPullInvocation.FindAllStringSubmatch(line, -1) {
				if !appPullInvocationOK(m[1]) {
					bad = append(bad, filepath.ToSlash(path)+":"+strconv.Itoa(i+1)+": civitai app pull"+m[1])
				} else if strings.Contains(m[1], "--app") {
					withFlag[filepath.Ext(path)]++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	// POSITIVE CONTROL, per file type: the README and app pull's own Go help
	// both carry correct invocations, and the shipped defect was in a .go file,
	// so a walk that reached only the Markdown must not pass.
	for _, ext := range []string{".md", ".go"} {
		if withFlag[ext] == 0 {
			t.Errorf("found no correct `civitai app pull … --app` invocation in any %s file — the walk or the matcher is broken", ext)
		}
	}
	for _, b := range bad {
		t.Errorf("%s\n    `civitai app pull` takes the app in --app; the positional is the DIRECTORY: civitai app pull [dir] --app <slug>", b)
	}
}
