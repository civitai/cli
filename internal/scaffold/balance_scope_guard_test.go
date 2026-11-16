package scaffold

// Structural guard (#886): a template whose app code CALLS `useBuzzBalance`
// must declare `buzz:read:self` in its block.manifest.json.
//
// The host answers the hook's GET_BUZZ_BALANCE through civitai's
// `blocks.getMyBuzzBalance`, which requires `buzz:read:self` on the block
// token. A template that calls the hook without declaring the scope gets a
// refused read on the real platform while every mock-host test stays green
// (the SDK mock answers the read whatever the manifest says). page-money
// shipped exactly that, with a comment saying the scope was not needed.
//
// It pins the RELATIONSHIP (uses the hook ⇒ declares the scope) over every
// template, so a new template that adopts the hook is covered without being
// named here.

import (
	"encoding/json"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// useBuzzBalanceCall matches a CALL of the hook, not a mention in prose.
var useBuzzBalanceCall = regexp.MustCompile(`\buseBuzzBalance\s*\(`)

// templatesCallingBalanceHook returns, per template directory under root, the
// first non-test source file that calls useBuzzBalance (line comments and
// block-comment lines stripped). Test files are skipped: they mock the hook.
func templatesCallingBalanceHook(t *testing.T, fsys fs.FS, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.Contains(name, ".test.") {
			return nil
		}
		if !strings.HasSuffix(name, ".ts.tmpl") && !strings.HasSuffix(name, ".tsx.tmpl") &&
			!strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".tsx") {
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		tmpl, _, _ := strings.Cut(rel, "/")
		if _, seen := out[tmpl]; seen {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
				continue
			}
			if code, _, _ := strings.Cut(line, "//"); useBuzzBalanceCall.MatchString(code) {
				out[tmpl] = p
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}

// manifestScopes parses a template's block.manifest.json(.tmpl). The template
// actions sit inside JSON strings, so the file parses as JSON unrendered.
func manifestScopes(t *testing.T, fsys fs.FS, dir string) ([]string, bool) {
	t.Helper()
	for _, name := range []string{"block.manifest.json.tmpl", "block.manifest.json"} {
		b, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			continue
		}
		var m struct {
			Scopes []string `json:"scopes"`
		}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s/%s does not parse as JSON: %v", dir, name, err)
		}
		return m.Scopes, true
	}
	return nil, false
}

func balanceScopeViolations(t *testing.T, fsys fs.FS, root string) (users map[string]string, violations []string) {
	t.Helper()
	users = templatesCallingBalanceHook(t, fsys, root)
	for tmpl, file := range users {
		scopes, ok := manifestScopes(t, fsys, path.Join(root, tmpl))
		if !ok {
			violations = append(violations, tmpl+": calls useBuzzBalance ("+file+") but ships no block.manifest.json")
			continue
		}
		declared := false
		for _, s := range scopes {
			if s == "buzz:read:self" {
				declared = true
			}
		}
		if !declared {
			violations = append(violations, tmpl+": calls useBuzzBalance ("+file+") but its manifest scopes "+
				strings.Join(scopes, ",")+" do not declare buzz:read:self")
		}
	}
	return users, violations
}

func TestTemplatesCallingUseBuzzBalanceDeclareBuzzReadSelf(t *testing.T) {
	users, violations := balanceScopeViolations(t, templatesFS, "templates")
	// POSITIVE CONTROL: a zero-violation verdict over a tree where no template
	// calls the hook would be a statement about nothing. page-money calls it.
	if _, ok := users["page-money"]; !ok {
		t.Fatalf("the scan found no useBuzzBalance call in page-money (found: %v) — the matcher is not seeing the template, so a clean verdict would be vacuous", users)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

// The guard's own controls, against synthetic trees: it must go RED for a
// hook call without the scope, stay quiet for a template that only MENTIONS the
// hook in a comment or a test, and accept the declared scope.
func TestBalanceScopeGuardControls(t *testing.T) {
	call := "import { useBuzzBalance } from '@civitai/blocks-react';\nconst { balance } = useBuzzBalance();\n"
	cases := []struct {
		name     string
		files    map[string]string
		wantUser bool
		wantBad  int
	}{
		{"call without the scope is a violation", map[string]string{
			"t/x/src/App.tsx.tmpl":         call,
			"t/x/block.manifest.json.tmpl": `{"blockId":"{{ .Slug }}","scopes":["ai:write:budgeted"]}`,
		}, true, 1},
		{"call with the scope is clean", map[string]string{
			"t/x/src/App.tsx.tmpl":         call,
			"t/x/block.manifest.json.tmpl": `{"scopes":["ai:write:budgeted","buzz:read:self"]}`,
		}, true, 0},
		{"call with no manifest at all is a violation", map[string]string{
			"t/x/src/App.tsx.tmpl": call,
		}, true, 1},
		{"a comment mention is not a use", map[string]string{
			"t/x/src/App.tsx.tmpl":         "// useBuzzBalance() is not used here\n * useBuzzBalance() in a doc block\n",
			"t/x/block.manifest.json.tmpl": `{"scopes":[]}`,
		}, false, 0},
		{"a test file's mock is not a use", map[string]string{
			"t/x/src/App.test.tsx.tmpl":    "useBuzzBalance: () => ({ balance: null }),\nuseBuzzBalance();\n",
			"t/x/block.manifest.json.tmpl": `{"scopes":[]}`,
		}, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{}
			for name, body := range tc.files {
				fsys[name] = &fstest.MapFile{Data: []byte(body)}
			}
			users, bad := balanceScopeViolations(t, fsys, "t")
			if _, got := users["x"]; got != tc.wantUser {
				t.Errorf("uses hook = %v, want %v", got, tc.wantUser)
			}
			if len(bad) != tc.wantBad {
				t.Errorf("violations = %v, want %d", bad, tc.wantBad)
			}
		})
	}
}
