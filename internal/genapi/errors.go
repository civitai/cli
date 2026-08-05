package genapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// serverMessage extracts a tRPC error body's message
// ({"error":{"json":{"message":...}}}), falling back to a plain
// {"message"|"error": ...} body and then to the trimmed raw bytes.
func serverMessage(raw []byte) string {
	var trpc struct {
		Error struct {
			JSON struct {
				Message string `json:"message"`
			} `json:"json"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &trpc) == nil && trpc.Error.JSON.Message != "" {
		return trpc.Error.JSON.Message
	}
	var plain struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &plain) == nil {
		if plain.Message != "" {
			return plain.Message
		}
		if plain.Error != "" {
			return plain.Error
		}
	}
	return strings.TrimSpace(string(raw))
}

// generateError turns a non-200 from an orchestrator generation procedure into
// an actionable error, classified by HTTP status via civitai.TagStatus so the
// process exit code is pinned by errors.Is and never by message text.
//
// 🔴 TODO (design §7 — resolve in the command-surface phase, NOT here):
// civitai.TagStatus maps BOTH 401 and 403 to ErrUnauthorized -> exit 3, whose
// documented meaning is "login required / credential lacks scope". Three
// generation failures arrive as a 403 and are NOT that:
//
//   - insufficient Buzz             -> must not be exit 3; show cost vs balance
//   - generation globally disabled  -> must not be exit 3; not the user's fault
//   - account muted / onboarding    -> reads byte-identical to a scope error
//
// A script that runs out of Buzz would be told to re-run `civitai login`,
// forever. Discriminating them needs the server's message text, which is a
// presentation concern; this layer classifies FAITHFULLY by status and leaves
// the re-mapping (a new sentinel, or untagged) to the surface that owns the
// exit-code contract. Do not paper over it with a message-text match here.
//
// Also per §7: an unknown ecosystem is thrown as a bare Error server-side and
// arrives as a 500, not a 4xx. It has no sentinel today (exit 1); the surface
// phase should map it to usage.
func generateError(proc string, status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
	msg := serverMessage(raw)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("not authenticated for %s (401): %s — generation needs a personal API key with the AI Services scopes; create one at https://civitai.com/user/account, then run `civitai login --token <key>`", proc, msg)
	case http.StatusForbidden:
		return fmt.Errorf("refused by the server for %s (403): %s — this can be a missing AI Services scope, insufficient Buzz, a muted account, or generation being disabled; check your credential with `civitai whoami`", proc, msg)
	case http.StatusBadRequest:
		return fmt.Errorf("the server rejected the generation request (400): %s", msg)
	case http.StatusNotFound:
		return fmt.Errorf("no such generation procedure or resource (404): %s", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited (429): %s — back off before retrying", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("generation is unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d for %s: %s", status, proc, msg)
	}
}
