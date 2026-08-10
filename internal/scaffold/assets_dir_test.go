package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every template scaffolds an `assets/` directory for the store-listing media
// (`civitai app listing set-icon|set-cover|add-screenshot`), holding a README of
// the requirements and NOTHING ELSE.
//
// The directory exists because the README's quickstart names `./assets/icon.png`
// and no template used to create the parent — the one copy-pasteable command in
// that step exited 2 with `no such file` on a fresh scaffold (issue #270).
//
// 🔴 The EMPTINESS is the load-bearing half, and it is the half a future
// "helpful" commit will undo. A placeholder icon.png passes the CLI's format +
// byte check and the platform's aspect/dimension check and uploads cleanly — so
// it can reach a PUBLIC store listing as a stub graphic. A missing file fails
// loudly at `set-icon`, which is the moment the author can still fix it. See
// AGENTS.md item 25.
func TestEveryTemplateScaffoldsAssetsReadmeAndNoImages(t *testing.T) {
	// imageExts is the "no placeholder" ledger: an image the platform would
	// accept, plus the neighbours someone would reach for instead (svg/gif are
	// rejected by the listing MIME allowlist, but scaffolding one is the same
	// mistake). Extensions, not names, so `logo.png` is caught as well as
	// `icon.png`.
	imageExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
		".gif": true, ".svg": true, ".avif": true, ".bmp": true, ".ico": true,
	}

	// Positive control: a template list that resolved to nothing would make
	// every assertion below vacuous.
	if len(AllTemplates()) < 3 {
		t.Fatalf("AllTemplates() returned %d templates, want >= 3 — the walk below would prove almost nothing", len(AllTemplates()))
	}

	for _, tmpl := range AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "app")
			if _, err := Render(tmpl, dest, Data{Slug: "app", Name: "App"}); err != nil {
				t.Fatalf("Render: %v", err)
			}

			assetsDir := filepath.Join(dest, "assets")
			info, err := os.Stat(assetsDir)
			if err != nil || !info.IsDir() {
				t.Fatalf("template %q scaffolds no `assets/` directory (%v). "+
					"The README quickstart tells authors to save an icon at ./assets/icon.png; without the directory, "+
					"the documented `civitai app listing set-icon ./assets/icon.png` cannot work.", tmpl, err)
			}

			entries, err := os.ReadDir(assetsDir)
			if err != nil {
				t.Fatalf("read assets/: %v", err)
			}
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
				if imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
					t.Errorf("template %q scaffolds an IMAGE at assets/%s. Never ship a placeholder icon or cover: "+
						"it passes every format and byte check and uploads cleanly, so it can reach a public store listing as a stub. "+
						"A missing file fails loudly at `civitai app listing set-icon`, which is the point. See AGENTS.md item 25.",
						tmpl, e.Name())
				}
			}

			readme := filepath.Join(assetsDir, "README.md")
			b, err := os.ReadFile(readme)
			if err != nil {
				t.Fatalf("template %q has assets/ but no assets/README.md (entries: %v). "+
					"An empty directory tells an author nothing about what belongs in it.", tmpl, names)
			}
			body := string(b)

			// The requirement ledger. These are the platform's per-kind bounds
			// (civitai/civitai src/server/schema/blocks/app-listing.schema.ts,
			// validateListingImage) — an author who reads only this file must be
			// able to size all three images from it. Asserted per template so the
			// three copies cannot drift apart on the numbers while staying free to
			// differ in prose.
			for _, want := range []string{
				"set-icon", "set-cover", "add-screenshot",
				"0.9", "1.1", // icon aspect
				"1.3", "2.4", // cover aspect
				"0.4", "2.6", // screenshot aspect
				"128", "640", "320", // per-kind minimum dimension
				"2 MiB", "4 MiB", // the CLI's local byte caps
			} {
				if !strings.Contains(body, want) {
					t.Errorf("template %q assets/README.md does not mention %q — an author sizing their artwork from this file would be missing a bound", tmpl, want)
				}
			}
			// The framing, not just the numbers: these are checked server-side,
			// and the file must say so rather than reading as a local rule.
			if !strings.Contains(body, "platform") {
				t.Errorf("template %q assets/README.md never attributes the dimension rules to the platform; "+
					"unattributed numbers read as something the CLI enforces", tmpl)
			}
			if !strings.Contains(body, "no images") && !strings.Contains(body, "no placeholder") {
				t.Errorf("template %q assets/README.md does not explain that the directory ships with no images on purpose; "+
					"without that, the next contributor adds a placeholder", tmpl)
			}
		})
	}
}
