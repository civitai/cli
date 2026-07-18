package cmd

import (
	"net/http"
	"strings"
	"testing"
)

const articleWithBody = `{"id":42,"title":"ComfyUI Basics","nsfwLevel":1,
  "publishedAt":"2026-07-01T12:00:00.000Z","user":{"id":3,"username":"guide-author"},
  "tags":[{"id":5,"name":"comfyui"}],
  "stats":{"viewCountAllTime":100,"likeCountAllTime":9,"favoriteCountAllTime":4,"commentCountAllTime":1,"collectedCountAllTime":2},
  "content":"<h1>Overview</h1><p>Use <strong>ComfyUI</strong> &amp; a good VAE.</p><ul><li>step one</li><li>step two</li></ul><p>Read the <a href=\"https://civitai.com/x\">docs</a>.</p>"}`

// TestArticlesGetContentRendersBody proves `articles get --content` renders the
// HTML body to readable text below the metadata.
func TestArticlesGetContentRendersBody(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(articleWithBody))
	})
	out, _, err := run(t, "articles", "get", "42", "--content")
	if err != nil {
		t.Fatalf("articles get --content: %v", err)
	}
	// Metadata still present.
	if !strings.Contains(out, "ComfyUI Basics (id 42)") {
		t.Errorf("metadata header missing:\n%s", out)
	}
	// Rendered body.
	for _, want := range []string{"# Overview", "**ComfyUI**", "& a good VAE", "- step one", "- step two", "[docs](https://civitai.com/x)"} {
		if !strings.Contains(out, want) {
			t.Errorf("content missing %q:\n%s", want, out)
		}
	}
	// No raw HTML tags or undecoded entities leaked.
	for _, bad := range []string{"<h1>", "<strong>", "<li>", "&amp;"} {
		if strings.Contains(out, bad) {
			t.Errorf("raw markup %q leaked:\n%s", bad, out)
		}
	}
}

// TestArticlesGetDefaultOmitsContent proves the default (no --content) output is
// unchanged — metadata only, no body.
func TestArticlesGetDefaultOmitsContent(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(articleWithBody))
	})
	out, _, err := run(t, "articles", "get", "42")
	if err != nil {
		t.Fatalf("articles get: %v", err)
	}
	if strings.Contains(out, "─── content ───") || strings.Contains(out, "# Overview") {
		t.Errorf("default output should NOT render the body:\n%s", out)
	}
}

// TestArticlesGetJSONWinsOverContent proves --json returns the raw body untouched
// even with --content, and that the raw HTML is preserved.
func TestArticlesGetJSONWinsOverContent(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(articleWithBody))
	})
	out, _, err := run(t, "articles", "get", "42", "--content", "--json")
	if err != nil {
		t.Fatalf("articles get --content --json: %v", err)
	}
	// Raw HTML content is present (not rendered to markdown).
	if !strings.Contains(out, `<h1>Overview</h1>`) {
		t.Errorf("--json should emit the raw HTML content:\n%s", out)
	}
	// It should NOT contain the rendered content header.
	if strings.Contains(out, "─── content ───") {
		t.Errorf("--json must not render the content section:\n%s", out)
	}
}
