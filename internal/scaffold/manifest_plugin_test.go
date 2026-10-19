package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
)

// Every Vite template that depends on the Civitai SDK registers
// `blockManifestPlugin()` from `@civitai/app-sdk/vite`.
//
// Why it is not optional (civitai/civitai-app-starters#710 / #711): in a
// dev-tunnel preview the host sends BLOCK_INIT `storageMode: 'local-preview'`
// and `@civitai/sdk` serves `app.storage.collection()` from an in-memory store
// built from the LOCAL manifest. The manifest's only route into the SDK is a
// global that this plugin injects on the Vite dev server — there is no
// `initialize()` option. Without the plugin every document-store call in a
// tunnel preview rejects `local_preview_no_manifest`, and nothing else in the
// scaffold would tell the author why.
//
// The subject set is DERIVED from the rendered tree (a Vite config plus a
// dependency on `@civitai/app-sdk` or `@civitai/sdk`), then compared with an
// expected ledger in both directions: a new SDK-backed template must register
// the plugin, and a template silently dropping out of the set (its config
// renamed, its SDK dependency removed) fails rather than shrinking coverage.
//
// page-vite is NOT a subject, deliberately: it is the SDK-free template
// (AGENTS.md item 11; TestRenderPageVite forbids any `@civitai/*` dependency
// there), so `@civitai/sdk`'s document store is unreachable from it and the
// plugin would have nothing to serve. The moment it gains either SDK package it
// becomes a subject here and must register the plugin.
func TestViteTemplatesRegisterBlockManifestPlugin(t *testing.T) {
	wantSubjects := []string{string(PageElements), string(PageMoney)}

	importRe := regexp.MustCompile(`import\s*\{[^}]*\bblockManifestPlugin\b[^}]*\}\s*from\s*['"]@civitai/app-sdk/vite['"]`)
	// The CALL inside the `plugins` array — an import that is never registered
	// is inert, and a call outside `plugins:` registers nothing.
	registeredRe := regexp.MustCompile(`plugins\s*:\s*\[[^\]]*\bblockManifestPlugin\s*\(\s*\)`)

	var subjects []string
	for _, tmpl := range AllTemplates() {
		dir := filepath.Join(t.TempDir(), "probe")
		if _, err := Render(tmpl, dir, Data{Slug: "manifest-plugin-probe", Name: "Manifest Plugin Probe"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}

		var cfgPath string
		for _, name := range []string{"vite.config.ts", "vite.config.js", "vite.config.mts", "vite.config.mjs"} {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				cfgPath = p
				break
			}
		}
		if cfgPath == "" {
			t.Logf("%s: no vite config — not a Vite template", tmpl)
			continue
		}

		deps, devDeps := packageDeps(t, dir)
		_, appSDK := deps["@civitai/app-sdk"]
		if _, ok := devDeps["@civitai/app-sdk"]; ok {
			appSDK = true
		}
		_, sdk := deps["@civitai/sdk"]
		if !appSDK && !sdk {
			t.Logf("%s: Vite template with no @civitai/app-sdk or @civitai/sdk dependency — SDK-free, not a subject", tmpl)
			continue
		}
		subjects = append(subjects, string(tmpl))

		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		src := blockproto.StripJSComments(string(raw))
		base := filepath.Base(cfgPath)
		if !importRe.MatchString(src) {
			t.Errorf("%s: %s does not import blockManifestPlugin from '@civitai/app-sdk/vite'.\n"+
				"  Without it, a dev-tunnel preview's document store (app.storage.collection) rejects\n"+
				"  local_preview_no_manifest — the plugin's dev-server global is the manifest's only route\n"+
				"  into @civitai/sdk (civitai/civitai-app-starters#711).", tmpl, base)
		}
		if !registeredRe.MatchString(src) {
			t.Errorf("%s: %s never calls blockManifestPlugin() inside `plugins: [...]` — an imported but\n"+
				"  unregistered plugin injects nothing.", tmpl, base)
		}
		if !appSDK {
			t.Errorf("%s: imports from @civitai/app-sdk/vite but package.json does not depend on @civitai/app-sdk", tmpl)
		}
		// The plugin compiles the canonical schema with Ajv at config time;
		// `ajv` is an OPTIONAL peer of @civitai/app-sdk, so npm does not install
		// it for you and the dev server dies on boot without it.
		if _, ok := devDeps["ajv"]; !ok {
			if _, ok := deps["ajv"]; !ok {
				t.Errorf("%s: registers blockManifestPlugin but package.json has no `ajv` (an optional peer the plugin needs at runtime)", tmpl)
			}
		}
	}

	sort.Strings(subjects)
	sort.Strings(wantSubjects)
	if !reflect.DeepEqual(subjects, wantSubjects) {
		t.Errorf("SDK-backed Vite template set = %v, want %v.\n"+
			"  Grew: register blockManifestPlugin() in the new template (the checks above say how) and add it here.\n"+
			"  Shrank: a template stopped being detected — check its vite config name and SDK dependency\n"+
			"  before removing it from this ledger.", subjects, wantSubjects)
	}
}

// packageDeps returns the rendered package.json's dependencies (dependencies
// ∪ peerDependencies) and devDependencies. Missing package.json → empty maps.
func packageDeps(t *testing.T, dir string) (deps, devDeps map[string]string) {
	t.Helper()
	deps, devDeps = map[string]string{}, map[string]string{}
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if os.IsNotExist(err) {
		return deps, devDeps
	}
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("package.json in %s is not valid JSON: %v", dir, err)
	}
	for k, v := range pkg.Dependencies {
		deps[k] = v
	}
	for k, v := range pkg.PeerDependencies {
		deps[k] = v
	}
	for k, v := range pkg.DevDependencies {
		devDeps[k] = v
	}
	return deps, devDeps
}
