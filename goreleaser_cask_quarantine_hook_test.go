package cli_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The Homebrew cask must strip macOS quarantine from the installed binary, and
// must do it through `hooks.post.install_steps` (rendered as Homebrew's
// declarative `postflight_steps`), never the legacy `hooks.post.install`
// (rendered as a raw-Ruby `postflight do` block). See #875 / #904.
//
// # Why this is a test
//
// Both failure modes are silent in CI:
//
//   - Reverting to `hooks.post.install` still releases fine, but every
//     `brew info/install/upgrade` of the cask then prints Homebrew's
//     "Calling `postflight` is deprecated!" warning.
//   - Dropping the xattr step also releases fine, but the binary is neither
//     signed nor notarized, so Gatekeeper blocks it on first run.
//
// Neither produces an error until a user hits it after a release.
//
// # What is asserted, and how
//
// The YAML is parsed, so a comment that mentions the old key, or a reformat of
// the file, cannot trip or satisfy this. The `install_steps` value is a string
// of Homebrew's install-steps DSL; it is parsed into statements (an `on_macos`
// block and the `run` calls inside it, with their string-literal arguments)
// rather than substring-matched, so whitespace, quote style and line layout
// do not matter but the command, its arguments and its macOS scoping do.
//
// What this does NOT check: that goreleaser renders `install_steps` the way the
// comment in `.goreleaser.yaml` says, or that Homebrew executes it. Those are
// claims about goreleaser and Homebrew; the place to confirm them is the
// rendered `dist/homebrew/Casks/civitai.rb` of the next release.
func TestGoreleaserCaskQuarantineHookUsesInstallSteps(t *testing.T) {
	raw, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}

	var cfg struct {
		HomebrewCasks []struct {
			Name  string `yaml:"name"`
			Hooks struct {
				// Decoded as a generic map so ANY key under `post` is visible,
				// including ones this struct does not name.
				Post map[string]any `yaml:"post"`
			} `yaml:"hooks"`
		} `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}

	// Positive control on the read: if the top-level key moves, an empty slice
	// would make every assertion below vacuous.
	if len(cfg.HomebrewCasks) == 0 {
		t.Fatalf("homebrew_casks is empty or missing in .goreleaser.yaml — did the key move?")
	}

	for i, c := range cfg.HomebrewCasks {
		if _, ok := c.Hooks.Post["install"]; ok {
			t.Errorf("homebrew_casks[%d] (%s): hooks.post.install is set — that renders the "+
				"deprecated `postflight do` block; use hooks.post.install_steps (#875)", i, c.Name)
		}

		v, ok := c.Hooks.Post["install_steps"]
		if !ok {
			t.Errorf("homebrew_casks[%d] (%s): hooks.post.install_steps is missing — the cask "+
				"would not strip macOS quarantine from the unsigned binary (#875)", i, c.Name)
			continue
		}
		steps, ok := v.(string)
		if !ok {
			t.Errorf("homebrew_casks[%d] (%s): hooks.post.install_steps is %T, want a string", i, c.Name, v)
			continue
		}

		runs, err := parseCaskInstallSteps(steps)
		if err != nil {
			t.Errorf("homebrew_casks[%d] (%s): cannot parse hooks.post.install_steps: %v\n%s",
				i, c.Name, err, steps)
			continue
		}

		if !hasCaskQuarantineStrip(runs) {
			t.Errorf("homebrew_casks[%d] (%s): hooks.post.install_steps has no `run "+
				"\"/usr/bin/xattr\"` under `on_macos` with args -dr, com.apple.quarantine, "+
				"{{ .StagedPath }}/civitai — Gatekeeper would block the unsigned binary (#875)\nparsed runs: %+v",
				i, c.Name, runs)
		}
	}
}

// caskInstallRun is one `run "<cmd>", args: [...]` statement, with the platform
// blocks (`on_macos`, `on_linux`, ...) it is nested in.
type caskInstallRun struct {
	Scopes []string
	Cmd    string
	Args   []string
}

var (
	reCaskBlockOpen = regexp.MustCompile(`^(on_[a-z_]+)\s+do$`)
	reCaskRun       = regexp.MustCompile(`^run\s+(.+)$`)
	reCaskArgsKey   = regexp.MustCompile(`^args:\s*\[(.*)\]$`)
)

// parseCaskInstallSteps parses the small subset of Homebrew's install-steps DSL
// this config uses: `on_<platform> do ... end` blocks and `run "<cmd>"[, args:
// [<string>, ...]]` statements. Anything else is an error, so a statement this
// parser does not understand cannot silently be skipped.
func parseCaskInstallSteps(src string) ([]caskInstallRun, error) {
	var (
		runs  []caskInstallRun
		stack []string
	)
	for n, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			continue
		case line == "end":
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: unmatched `end`", n+1)
			}
			stack = stack[:len(stack)-1]
		case reCaskBlockOpen.MatchString(line):
			stack = append(stack, reCaskBlockOpen.FindStringSubmatch(line)[1])
		case reCaskRun.MatchString(line):
			r, err := parseCaskRun(reCaskRun.FindStringSubmatch(line)[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n+1, err)
			}
			r.Scopes = append([]string(nil), stack...)
			runs = append(runs, r)
		default:
			return nil, fmt.Errorf("line %d: unrecognised statement %q", n+1, line)
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unclosed block(s): %v", stack)
	}
	return runs, nil
}

// parseCaskRun parses `"<cmd>"[, args: [<string>, ...]]`.
func parseCaskRun(s string) (caskInstallRun, error) {
	cmd, rest, err := takeCaskString(s)
	if err != nil {
		return caskInstallRun{}, err
	}
	r := caskInstallRun{Cmd: cmd}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return r, nil
	}
	if !strings.HasPrefix(rest, ",") {
		return caskInstallRun{}, fmt.Errorf("expected `,` after command, got %q", rest)
	}
	m := reCaskArgsKey.FindStringSubmatch(strings.TrimSpace(rest[1:]))
	if m == nil {
		return caskInstallRun{}, fmt.Errorf("expected `args: [...]`, got %q", rest)
	}
	list := strings.TrimSpace(m[1])
	for list != "" {
		a, after, err := takeCaskString(list)
		if err != nil {
			return caskInstallRun{}, err
		}
		r.Args = append(r.Args, a)
		list = strings.TrimSpace(after)
		if list == "" {
			break
		}
		if !strings.HasPrefix(list, ",") {
			return caskInstallRun{}, fmt.Errorf("expected `,` between args, got %q", list)
		}
		list = strings.TrimSpace(list[1:])
	}
	return r, nil
}

// takeCaskString reads one leading Ruby string literal ("..." or '...', no
// escapes — none are used here) and returns its contents and the remainder.
func takeCaskString(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	if s == "" || (s[0] != '"' && s[0] != '\'') {
		return "", "", fmt.Errorf("expected a string literal, got %q", s)
	}
	q := s[0]
	end := strings.IndexByte(s[1:], q)
	if end < 0 {
		return "", "", fmt.Errorf("unterminated string literal %q", s)
	}
	return s[1 : 1+end], s[2+end:], nil
}

func hasCaskQuarantineStrip(runs []caskInstallRun) bool {
	for _, r := range runs {
		if r.Cmd != "/usr/bin/xattr" || !caskHookContains(r.Scopes, "on_macos") {
			continue
		}
		if caskHookContains(r.Args, "-dr") &&
			caskHookContains(r.Args, "com.apple.quarantine") &&
			caskHookContains(r.Args, "{{ .StagedPath }}/civitai") {
			return true
		}
	}
	return false
}

func caskHookContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
