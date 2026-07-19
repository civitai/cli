package cmd

import (
	"net/http"
	"strings"
	"testing"
)

// TestByHashEmitsRunnableDownloadCommand proves fix 3: `model-versions by-hash`
// output includes a ready-to-run `civitai download --version <id>` command (not
// only the raw API download URL a user would otherwise hand-reconstruct and trip
// the ambiguity gate with). It uses --version so the suggested command is immune
// to the model-id/version-id ambiguity stop.
func TestByHashEmitsRunnableDownloadCommand(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/by-hash/") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":128713,"modelId":4384,"name":"v8","baseModel":"SD 1.5",
		  "downloadUrl":"https://civitai.com/api/download/models/128713",
		  "files":[{"id":9,"name":"m.safetensors","type":"Model","primary":true,"sizeKB":2000000}]}`))
	})

	out, _, err := run(t, "model-versions", "by-hash", "5D8D26E2A6")
	if err != nil {
		t.Fatalf("by-hash: %v", err)
	}
	// The runnable command names the resolved version via --version (gate-immune).
	if !strings.Contains(out, "download --version 128713") {
		t.Errorf("by-hash output should include a runnable `download --version 128713` command:\n%s", out)
	}
	// The raw download URL is still shown (kept as useful).
	if !strings.Contains(out, "https://civitai.com/api/download/models/128713") {
		t.Errorf("by-hash output should still show the raw download URL:\n%s", out)
	}
}

// TestByHashDownloadHintUsesVersionNotPositional proves the suggested command uses
// the --version flag (never a bare positional id), the whole point of fix 3: a
// bare `civitai download <id>` can trip the ambiguity stop, but `--version <id>`
// never does.
func TestByHashDownloadHintUsesVersionNotPositional(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":277680,"modelId":4384,"name":"v","baseModel":"SDXL",
		  "files":[{"id":9,"name":"m.safetensors","type":"Model","primary":true,"sizeKB":2000000}]}`))
	})

	out, _, err := run(t, "model-versions", "by-hash", "ABC123")
	if err != nil {
		t.Fatalf("by-hash: %v", err)
	}
	if !strings.Contains(out, "--version 277680") {
		t.Errorf("hint must use --version:\n%s", out)
	}
	// The bare `civitai download 277680` (no --version) would be the gate-tripping
	// form; it must not be what we suggest.
	if strings.Contains(out, "civitai download 277680 ") || strings.Contains(out, "civitai download 277680\n") {
		t.Errorf("hint must NOT suggest the bare-positional form that can trip the gate:\n%s", out)
	}
}
