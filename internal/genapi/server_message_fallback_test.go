package genapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// nginx502 is a realistic edge 502 page, CRLF line endings and all, as nginx
// serves it in front of the tRPC host. 150 bytes.
const nginx502 = "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n" +
	"<center><h1>502 Bad Gateway</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

// fallbackAPIKey is a realistic 32-hex Civitai API key (civitai's
// generateKey(length = 32)), distinct from every other fixture here.
const fallbackAPIKey = "3b1f9e07c4a28d65f0e9a7b2c81d4e36"

// TestGenerateErrorFallbackNeverEchoesTheBody pins genapi's serverMessage
// fallback for a non-2xx body with no tRPC `error.json.message` and no top-level
// `message`/`error`: it goes through redact.CredentialRouteMessage, so the error
// shows the body's size and never the body. It used to fall back to the trimmed
// raw body, so an edge 502 HTML page was printed whole — CRLFs, markup and all,
// at whatever size it was — and a plain-text or JSON body holding an API key put
// the key on stderr. This is internal/appapi's civitai/cli#959 fix, mirrored for
// genapi's own copy of serverMessage.
//
// Every generateError arm is driven, because each one interpolates the message,
// and APIError.ServerMessage is checked too, because classifyGenerateError
// prints it.
func TestGenerateErrorFallbackNeverEchoesTheBody(t *testing.T) {
	htmlSentinel := strings.Replace(nginx502, "nginx", "nginx "+bodyEchoSentinel, 1)
	huge := htmlSentinel + strings.Repeat("<p>"+bodyEchoSentinel+"</p>\r\n", 200*1024/32)
	bodies := []struct{ label, raw, secret string }{
		{"edge 502 html", htmlSentinel, bodyEchoSentinel},
		{"200KiB html", huge, bodyEchoSentinel},
		{"plain text with a 32-hex key", "invalid API key " + fallbackAPIKey + "\n", fallbackAPIKey},
		{"json without a message field carrying a key", `{"ok":false,"key":"` + fallbackAPIKey + `"}`, fallbackAPIKey},
	}
	arms := []struct {
		proc   string
		status int
	}{
		{"generateFromGraph", http.StatusUnauthorized},
		{"generateFromGraph", http.StatusForbidden},
		{"generateFromGraph", http.StatusBadRequest},
		{"getWorkflow", http.StatusNotFound},
		{"generateFromGraph", http.StatusNotFound},
		{"generateFromGraph", http.StatusTooManyRequests},
		{"generateFromGraph", http.StatusServiceUnavailable},
		{"generateFromGraph", http.StatusBadGateway},
	}
	for _, arm := range arms {
		for _, body := range bodies {
			t.Run(arm.proc+"/"+http.StatusText(arm.status)+"/"+body.label, func(t *testing.T) {
				err := generateError(arm.proc, arm.status, []byte(body.raw))
				assertBodyNotEchoed(t, err, "response body not shown because it may carry credentials")
				msg := err.Error()
				if strings.Contains(msg, body.secret) {
					t.Errorf("the error carries the body's secret %q:\n  %.300q", body.secret, msg)
				}
				// The body's own tags, not any `<`: the 401 arm's copy says
				// `--token <key>` itself.
				for _, tag := range []string{"<html", "<center", "<p>", "</"} {
					if strings.Contains(msg, tag) {
						t.Errorf("the error carries HTML markup %q from the body:\n  %.300q", tag, msg)
					}
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("generateError lost its *APIError: %T", err)
				}
				if strings.Contains(apiErr.ServerMessage, body.secret) || strings.Contains(apiErr.ServerMessage, "<") {
					t.Errorf("APIError.ServerMessage carries the body:\n  %.300q", apiErr.ServerMessage)
				}
			})
		}
	}

	// The whole text, pinned for one arm: the status, the proc and the body's
	// size (150 bytes, counted independently of the code under test).
	if got, want := generateError("generateFromGraph", http.StatusBadGateway, []byte(nginx502)).Error(),
		"server returned 502 for generateFromGraph: 150-byte response body not shown because it may carry credentials"; got != want {
		t.Errorf("generateError(502, nginx html) =\n  %q\nwant\n  %q", got, want)
	}

	// Positive controls: a body WITH a message field still shows the server's
	// words — tRPC envelope and plain REST alike — so the assertions above are
	// not met by withholding everything.
	controls := []struct{ raw, want string }{
		{`{"error":{"json":{"message":"upstream deploy in progress"}}}`, "server returned 502 for generateFromGraph: upstream deploy in progress"},
		{`{"message":"orchestrator draining"}`, "server returned 502 for generateFromGraph: orchestrator draining"},
		{`{"error":"queue full"}`, "server returned 502 for generateFromGraph: queue full"},
	}
	for _, c := range controls {
		if got := generateError("generateFromGraph", http.StatusBadGateway, []byte(c.raw)).Error(); got != c.want {
			t.Errorf("control: generateError(502, %s) =\n  %q\nwant\n  %q", c.raw, got, c.want)
		}
	}
}
