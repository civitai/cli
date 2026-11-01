package scaffold

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Civitai host serves App previews on every Civitai domain. A scaffold that
// named only civitai.com shipped a dev server civitai.red could not frame and an
// allowlist that dropped civitai.red's BLOCK_INIT, so the preview stayed blank.
//
// The expected values are LITERALS, not the constant the templates render from:
// an expectation derived from the implementation passes whatever it says.
const (
	wantCivitaiOrigins = "https://civitai.com https://civitai.red https://civitai.green"
	wantDevAllowlist   = "http://localhost:5186,https://civitai.com,https://civitai.red,https://civitai.green"
	wantProdAllowlist  = "https://civitai.com,https://civitai.red,https://civitai.green,https://www.civitai.com"
	wantDevCSP         = "frame-ancestors 'self' https://civitai.com https://civitai.red https://civitai.green"
)

// envAssignments returns every non-comment value assigned to key in a dotenv file.
func envAssignments(t *testing.T, path, key string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if v, ok := strings.CutPrefix(line, key+"="); ok {
			out = append(out, v)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestScaffoldAllowlistsEveryCivitaiDomain pins the rendered allowlists whole.
func TestScaffoldAllowlistsEveryCivitaiDomain(t *testing.T) {
	const key = "VITE_BLOCK_ALLOWED_PARENT_ORIGINS"
	for _, tmpl := range devEmbedTemplates {
		t.Run(string(tmpl), func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Render(tmpl, dir, Data{Slug: "my-block", Name: "My Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
			}
			got := envAssignments(t, filepath.Join(dir, ".env.development"), key)
			if len(got) != 1 || got[0] != wantDevAllowlist {
				t.Errorf(".env.development %s = %q, want exactly one assignment %q", key, got, wantDevAllowlist)
			}
			if tmpl == PageMoney {
				got := envAssignments(t, filepath.Join(dir, ".env.production"), key)
				if len(got) != 1 || got[0] != wantProdAllowlist {
					t.Errorf(".env.production %s = %q, want exactly one assignment %q", key, got, wantProdAllowlist)
				}
			}
		})
	}
}

// TestScaffoldFramingAndAllowlistShareOneOriginList pins the RELATIONSHIP: the
// origins the dev server lets frame it are the origins the app accepts messages
// from (plus the local harness), in every Vite template. Each is also pinned to
// the literal, so the two cannot agree on a wrong list.
func TestScaffoldFramingAndAllowlistShareOneOriginList(t *testing.T) {
	for _, v := range renderViteTemplates(t) {
		t.Run(string(v.tmpl), func(t *testing.T) {
			c := extractDevEmbedConstants(t, v.read(t, v.devEmbed))
			if c.parentOrigins != wantCivitaiOrigins {
				t.Errorf("%s parent origins = %q, want %q", v.devEmbed, c.parentOrigins, wantCivitaiOrigins)
			}
			if c.csp != wantDevCSP {
				t.Errorf("%s Content-Security-Policy = %q, want %q", v.devEmbed, c.csp, wantDevCSP)
			}
			// One spelling of each origin in the dev-embed module: the shared list.
			for _, o := range strings.Fields(wantCivitaiOrigins) {
				if n := strings.Count(v.read(t, v.devEmbed), "'"+o+"'"); n != 1 {
					t.Errorf("%s spells '%s' %d times, want once (in the shared origin list)", v.devEmbed, o, n)
				}
			}
			envPath := filepath.Join(v.dir, ".env.development")
			if _, err := os.Stat(envPath); err != nil {
				return // page-vite has no SDK and no allowlist
			}
			got := envAssignments(t, envPath, "VITE_BLOCK_ALLOWED_PARENT_ORIGINS")
			if len(got) != 1 {
				t.Fatalf(".env.development has %d allowlist assignments, want 1", len(got))
			}
			if want := "http://localhost:5186," + strings.ReplaceAll(c.parentOrigins, " ", ","); got[0] != want {
				t.Errorf(".env.development allowlist %q is not the dev server's framing list %q plus the harness", got[0], want)
			}
		})
	}
}
