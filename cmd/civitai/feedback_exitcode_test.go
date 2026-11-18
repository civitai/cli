package main

// THE PROCESS EXIT CODES OF `civitai feedback send`, MEASURED ON THE BUILT BINARY.
//
// internal/cmd pins each refusal's CLASSIFICATION by errors.Is. That is one
// claim; what a script reads is the process status, which only exitCode in this
// package produces. The two can disagree — the collection-off arm is the case
// this file exists for: it arrives as HTTP 403, and every other 403 in this CLI
// exits 3, so "untagged on purpose" is one stray TagStatus away from a script
// answering it with `civitai login`.
//
// The chain each row measures, end to end:
//
//	fake server status + message
//	    internal/appapi/platform_feedback.go  platformFeedbackError -> kind (or none)
//	    cmd/civitai/main.go                   exitCode              -> process status
//
// 🔴 THE SERVER DID NOT EXIST WHEN THIS WAS WRITTEN. The statuses and messages
// below are the written contract's, not strings read off a deployment; the
// fixtures are a pass-through control on the mapping, not a claim about what
// Civitai sends.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFeedbackSendExitCodes(t *testing.T) {
	bin := buildCLI(t)

	trpcErr := func(message string) string {
		b, _ := json.Marshal(map[string]any{"error": map[string]any{"json": map[string]any{"message": message}}})
		return string(b)
	}

	rows := []struct {
		name string
		// status and body are what platformFeedback.create answers with.
		status int
		body   string
		// want is the MEASURED process exit status.
		want       int
		wantStderr string
	}{
		{
			name:       "scope refusal",
			status:     http.StatusForbidden,
			body:       trpcErr("Your API key does not have the required scope for this action"),
			want:       3,
			wantStderr: "scope does not cover platform feedback (403)",
		},
		{
			name:       "rate limited",
			status:     http.StatusTooManyRequests,
			body:       trpcErr("Too many requests"),
			want:       6,
			wantStderr: "rate limited (429)",
		},
		{
			name:       "a server without the platform-feedback API",
			status:     http.StatusNotFound,
			body:       trpcErr(`No "mutation"-procedure on path "platformFeedback.create"`),
			want:       4,
			wantStderr: "this server does not offer platform feedback (404)",
		},
		{
			name:       "collection is off",
			status:     http.StatusForbidden,
			body:       trpcErr("Feedback is not being collected here right now."),
			want:       1,
			wantStderr: "Civitai is not collecting platform feedback right now",
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if r.URL.Path != "/api/trpc/platformFeedback.create" {
					http.NotFound(w, r)
					return
				}
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(row.status)
				_, _ = io.WriteString(w, row.body)
			}))
			defer srv.Close()

			env := []string{
				"XDG_CONFIG_HOME=" + t.TempDir(),
				"HOME=" + t.TempDir(),
				"CIVITAI_TOKEN=5d0c7a3e9b1f4682a4c6e8f0b2d49713",
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_NO_UPDATE_CHECK=1",
			}
			code, stderr := runCLI(t, bin, env, "feedback", "send", "--about", "cli", "the thing broke")
			if code != row.want {
				t.Errorf("exit status = %d, want %d\nstderr: %s", code, row.want, stderr)
			}
			if !strings.Contains(stderr, row.wantStderr) {
				t.Errorf("stderr does not say %q:\n%s", row.wantStderr, stderr)
			}
			// The refusal was the SERVER's: the request reached the route. Without
			// this a client-side failure with the same code would satisfy the row.
			if n := atomic.LoadInt32(&hits); n != 1 {
				t.Errorf("platformFeedback.create was called %d time(s), want exactly 1", n)
			}
		})
	}

	// Exit 0, for a NEW report and for a DUPLICATE alike. runCLI fatals on
	// success (its callers measure failures), so these run the binary directly.
	for name, payload := range map[string]string{
		"sent":      `{"ref":"6001-0a1b2c3d","duplicate":false,"status":"received","fixedIn":null,"note":null,"reporters":1,"reports":1}`,
		"duplicate": `{"ref":"5123-3f9a01bc","duplicate":true,"status":"acknowledged","fixedIn":null,"note":null,"reporters":14,"reports":22}`,
	} {
		t.Run(name+" exits 0", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, `{"result":{"data":{"json":`+payload+`}}}`)
			}))
			defer srv.Close()
			cmd := exec.Command(bin, "feedback", "send", "--about", "cli", "the thing broke")
			cmd.Env = []string{
				"XDG_CONFIG_HOME=" + t.TempDir(),
				"HOME=" + t.TempDir(),
				"CIVITAI_TOKEN=5d0c7a3e9b1f4682a4c6e8f0b2d49713",
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_NO_UPDATE_CHECK=1",
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("want exit 0, got %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "Feedback sent to Civitai") {
				t.Errorf("exit 0 without the success line:\n%s", stdout.String())
			}
		})
	}
}
