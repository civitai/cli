package devtunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clearParentOrigins guarantees the variable is ABSENT from the process
// environment for the duration of a test. Without this the suite's verdict would
// depend on the developer's shell — a value exported in the ambient environment
// would silently satisfy resolveViteEnv and turn every "warns when missing" case
// green for the wrong reason.
func clearParentOrigins(t *testing.T) {
	t.Helper()
	if v, ok := os.LookupEnv(ParentOriginsEnvVar); ok {
		t.Setenv(ParentOriginsEnvVar, v) // registers the restore
		_ = os.Unsetenv(ParentOriginsEnvVar)
	}
}

type appDirOpts struct {
	manifest bool
	pkg      string            // raw package.json; "" = none
	envFiles map[string]string // filename -> contents
}

func writeAppDir(t *testing.T, o appDirOpts) string {
	t.Helper()
	dir := t.TempDir()
	if o.manifest {
		if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(`{"blockId":"demo"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if o.pkg != "" {
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(o.pkg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range o.envFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const sdkPkg = `{"dependencies":{"@civitai/app-sdk":"0.30.0","react":"^18.3.1"}}`

// TestCheckParentOriginsGate pins the silence conditions. `dev-tunnel` accepts an
// explicit blockId and so runs from arbitrary directories; advice derived from an
// unrelated CWD would be worse than none.
func TestCheckParentOriginsGate(t *testing.T) {
	clearParentOrigins(t)

	tests := []struct {
		name      string
		opts      appDirOpts
		wantQuiet bool
	}{
		{name: "empty dir", opts: appDirOpts{}, wantQuiet: true},
		{name: "manifest but no package.json", opts: appDirOpts{manifest: true}, wantQuiet: true},
		{name: "package.json but no manifest", opts: appDirOpts{pkg: sdkPkg}, wantQuiet: true},
		{
			name:      "app without the SDK",
			opts:      appDirOpts{manifest: true, pkg: `{"dependencies":{"react":"^18.3.1"}}`},
			wantQuiet: true,
		},
		{
			name:      "malformed package.json",
			opts:      appDirOpts{manifest: true, pkg: `{not json`},
			wantQuiet: true,
		},
		{
			name: "SDK app is checked",
			opts: appDirOpts{manifest: true, pkg: sdkPkg},
		},
		{
			name: "SDK as a devDependency is checked",
			opts: appDirOpts{manifest: true, pkg: `{"devDependencies":{"@civitai/app-sdk":"0.30.0"}}`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckParentOrigins(writeAppDir(t, tc.opts))
			if tc.wantQuiet && len(got) != 0 {
				t.Fatalf("want silence, got %v", kinds(got))
			}
			if !tc.wantQuiet && !hasKind(got, FindingParentOrigins) {
				t.Fatalf("want a %q finding, got %v", FindingParentOrigins, kinds(got))
			}
		})
	}
}

func TestCheckParentOriginsValues(t *testing.T) {
	tests := []struct {
		name     string
		envFiles map[string]string
		wantWarn bool
	}{
		{name: "no env files at all", wantWarn: true},
		{
			name:     "scaffold default is accepted",
			envFiles: map[string]string{".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186,https://civitai.com\n"},
		},
		{
			name:     "prod parent alone is accepted",
			envFiles: map[string]string{".env": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n"},
		},
		{
			name:     "localhost only is flagged",
			envFiles: map[string]string{".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n"},
			wantWarn: true,
		},
		{
			name:     "empty value is flagged",
			envFiles: map[string]string{".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=\n"},
			wantWarn: true,
		},
		{
			name:     "www subdomain is not the parent origin",
			envFiles: map[string]string{".env": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://www.civitai.com\n"},
			wantWarn: true,
		},
		{
			name:     "quoted value is accepted",
			envFiles: map[string]string{".env": `VITE_BLOCK_ALLOWED_PARENT_ORIGINS="http://localhost:5186,https://civitai.com"` + "\n"},
		},
		{
			name:     "export prefix is accepted",
			envFiles: map[string]string{".env": "export VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n"},
		},
		{
			name:     "commented-out assignment does not count",
			envFiles: map[string]string{".env": "# VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n"},
			wantWarn: true,
		},
		// Vite precedence: .env.development overrides .env, and *.local overrides
		// its non-local sibling. Getting this backwards would warn at a correct
		// project (or stay silent on a broken one).
		{
			name: "mode file overrides base file (good wins)",
			envFiles: map[string]string{
				".env":             "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n",
				".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186,https://civitai.com\n",
			},
		},
		{
			name: "mode file overrides base file (bad wins)",
			envFiles: map[string]string{
				".env":             "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n",
				".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n",
			},
			wantWarn: true,
		},
		{
			name: "development.local overrides development",
			envFiles: map[string]string{
				".env.development":       "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n",
				".env.development.local": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n",
			},
		},
		{
			name: "other keys in the file are ignored",
			envFiles: map[string]string{
				".env.development": "VITE_OTHER=1\nVITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\nVITE_MORE=2\n",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearParentOrigins(t)
			dir := writeAppDir(t, appDirOpts{manifest: true, pkg: sdkPkg, envFiles: tc.envFiles})
			got := CheckParentOrigins(dir)
			if hasKind(got, FindingParentOrigins) != tc.wantWarn {
				t.Fatalf("want warn=%v, got %v", tc.wantWarn, kinds(got))
			}
		})
	}
}

// TestCheckParentOriginsProcessEnvWins pins the rule most likely to be coded
// backwards: dotenv does NOT overwrite a variable that already exists, so an
// exported shell value beats every file. Both directions are asserted — the
// "good overrides bad files" case alone would also pass if files were ignored
// entirely.
func TestCheckParentOriginsProcessEnvWins(t *testing.T) {
	t.Run("exported good value silences bad files", func(t *testing.T) {
		t.Setenv(ParentOriginsEnvVar, "https://civitai.com")
		dir := writeAppDir(t, appDirOpts{
			manifest: true, pkg: sdkPkg,
			envFiles: map[string]string{".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n"},
		})
		if got := CheckParentOrigins(dir); hasKind(got, FindingParentOrigins) {
			t.Fatalf("an exported value must win over .env files; got %v", kinds(got))
		}
	})

	t.Run("exported bad value overrides good files", func(t *testing.T) {
		t.Setenv(ParentOriginsEnvVar, "http://localhost:5186")
		dir := writeAppDir(t, appDirOpts{
			manifest: true, pkg: sdkPkg,
			envFiles: map[string]string{".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=https://civitai.com\n"},
		})
		if got := CheckParentOrigins(dir); !hasKind(got, FindingParentOrigins) {
			t.Fatalf("an exported value must win over .env files; got %v", kinds(got))
		}
	})
}

// TestCheckParentOriginsRemediationPreservesExisting: the suggested line must
// ADD the parent origin to what the author already has, not replace it — a
// remediation that drops the localhost entry silently breaks `npm run dev`.
func TestCheckParentOriginsRemediationPreservesExisting(t *testing.T) {
	clearParentOrigins(t)
	dir := writeAppDir(t, appDirOpts{
		manifest: true, pkg: sdkPkg,
		envFiles: map[string]string{".env.development": "VITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186\n"},
	})
	got := CheckParentOrigins(dir)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %v", kinds(got))
	}
	joined := strings.Join(got[0].Fix, "\n")
	if !strings.Contains(joined, "http://localhost:5186,"+ProdParentOrigin) {
		t.Fatalf("remediation must preserve the existing origins:\n%s", joined)
	}
}

func TestParseDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := strings.Join([]string{
		"# a comment",
		"",
		"PLAIN=value",
		"  SPACED  =  spaced value  ",
		`DQ="double quoted"`,
		`SQ='single quoted'`,
		"export EXPORTED=exported",
		"INLINE=value # trailing comment",
		`QUOTED_HASH="value # not a comment"`,
		"EMPTY=",
		"NOEQUALS",
		"=novalue",
	}, "\n")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("parseDotEnv: %v", err)
	}
	want := map[string]string{
		"PLAIN":       "value",
		"SPACED":      "spaced value",
		"DQ":          "double quoted",
		"SQ":          "single quoted",
		"EXPORTED":    "exported",
		"INLINE":      "value",
		"QUOTED_HASH": "value # not a comment",
		"EMPTY":       "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q", k, got[k], v)
		}
	}
	if _, ok := got["NOEQUALS"]; ok {
		t.Error("a line with no '=' must be skipped")
	}
	if _, ok := got[""]; ok {
		t.Error("an empty key must be skipped")
	}
	if len(got) != len(want) {
		t.Errorf("unexpected extra keys: got %v", got)
	}
}

func TestParseDotEnvMissingFile(t *testing.T) {
	if _, err := parseDotEnv(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

// TestParseDotEnvMatchesVite pins the dotenv semantics that were VERIFIED
// differentially against Vite's own loadEnv (26 fixtures, all matching). Four of
// these were wrong in the first implementation and every one produced a FALSE
// WARNING at a correctly configured project — the worst failure mode for
// advisory output, since it teaches authors to ignore it.
func TestParseDotEnvMatchesVite(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string // "\x00" = key absent
	}{
		{name: "plain", body: "K=https://civitai.com", want: "https://civitai.com"},
		{name: "export prefix", body: "export K=https://civitai.com", want: "https://civitai.com"},
		{name: "single quoted", body: "K='https://civitai.com'", want: "https://civitai.com"},
		{name: "double quoted", body: `K="https://civitai.com"`, want: "https://civitai.com"},
		// dotenv >=16 accepts backticks; treating them as literal produced a value
		// that never matched the parent origin.
		{name: "backtick quoted", body: "K=`https://civitai.com`", want: "https://civitai.com"},
		// An unquoted `#` starts a comment ANYWHERE, not only after a space.
		{name: "hash without a leading space", body: "K=https://civitai.com#x", want: "https://civitai.com"},
		{name: "hash after a space", body: "K=https://civitai.com # c", want: "https://civitai.com"},
		// Inside quotes a `#` is literal.
		{name: "hash inside double quotes", body: `K="https://civitai.com#f"`, want: "https://civitai.com#f"},
		{name: "hash inside single quotes", body: "K='https://civitai.com#f'", want: "https://civitai.com#f"},
		// Vite runs dotenv-expand over the parsed file.
		{name: "braced expansion", body: "BASE=https://civitai.com\nK=${BASE}", want: "https://civitai.com"},
		{name: "bare expansion", body: "BASE=https://civitai.com\nK=$BASE", want: "https://civitai.com"},
		{name: "partial expansion", body: "BASE=https://civitai.com\nK=${BASE},http://localhost:5186", want: "https://civitai.com,http://localhost:5186"},
		// An unresolvable reference expands to EMPTY, not to its own text.
		{name: "unresolved expansion", body: "K=${NOPE}", want: ""},
		// A colon is NOT a separator for Vite — honouring it would resolve a value
		// the app never sees, and stay silent on a project that is actually broken.
		{name: "colon is not a separator", body: "K: https://civitai.com", want: "\x00"},
		{name: "value containing a colon", body: "K=http://localhost:5186", want: "http://localhost:5186"},
		{name: "value with colon and comma", body: "K=http://localhost:5186,https://civitai.com", want: "http://localhost:5186,https://civitai.com"},
		{name: "full-line comment", body: "# K=https://civitai.com", want: "\x00"},
		{name: "no separator", body: "K", want: "\x00"},
		{name: "empty value", body: "K=", want: ""},
		{name: "spaces around the equals", body: "K   =   https://civitai.com   ", want: "https://civitai.com"},
		{name: "crlf line ending", body: "K=https://civitai.com\r\n", want: "https://civitai.com"},
		{name: "later line wins", body: "K=first\nK=https://civitai.com", want: "https://civitai.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, ".env")
			if err := os.WriteFile(p, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			vars, err := parseDotEnv(p)
			if err != nil {
				t.Fatalf("parseDotEnv: %v", err)
			}
			got, ok := vars["K"]
			if tc.want == "\x00" {
				if ok {
					t.Fatalf("K must be absent, got %q", got)
				}
				return
			}
			if !ok {
				t.Fatalf("K missing; want %q", tc.want)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// TestExpandDotEnvTerminates: a self-referential file must not spin.
func TestExpandDotEnvTerminates(t *testing.T) {
	done := make(chan map[string]string, 1)
	go func() { done <- expandDotEnv(map[string]string{"A": "${B}", "B": "${A}"}) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("expandDotEnv did not terminate on a reference cycle")
	}
}

// TestCheckParentOriginsExpandedValue drives the whole check through an expanded
// value, so the expansion is pinned at the level the user actually experiences.
func TestCheckParentOriginsExpandedValue(t *testing.T) {
	clearParentOrigins(t)
	dir := writeAppDir(t, appDirOpts{
		manifest: true, pkg: sdkPkg,
		envFiles: map[string]string{
			".env.development": "PARENT=https://civitai.com\nVITE_BLOCK_ALLOWED_PARENT_ORIGINS=http://localhost:5186,${PARENT}\n",
		},
	})
	if got := CheckParentOrigins(dir); len(got) != 0 {
		t.Fatalf("an expanded ${PARENT} must satisfy the check, got %v", kinds(got))
	}
}
