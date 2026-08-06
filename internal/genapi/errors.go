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

// APIError is the structured form of a non-200 from an orchestrator generation
// procedure. It exists so a caller can discriminate on the SERVER's own message
// without re-parsing the formatted error string.
//
// 🔴 Read ServerMessage, never Error(), when classifying. Error() is the
// ACTIONABLE text this package builds, and for a 403 that text deliberately
// lists every cause a 403 can have ("a missing AI Services scope, insufficient
// Buzz, a muted account, or generation being disabled") — so a substring match
// against Error() matches ALL of them for EVERY 403 and classifies nothing.
// ServerMessage is only what the server said.
//
// Error() delegates unchanged, so the user-visible message and the
// civitai.TagStatus classification are byte-for-byte what they were before this
// type existed; it only adds an errors.As handle.
type APIError struct {
	// Proc is the orchestrator procedure that failed.
	Proc string
	// Status is the HTTP status the tRPC endpoint answered with. Note this is
	// derived server-side from the TRPCError CODE, not from whatever status an
	// upstream service used — see the mapping notes in generateError.
	Status int
	// ServerMessage is the server's own message, unwrapped from the tRPC error
	// envelope ({"error":{"json":{"message":…}}}). It is UNTRUSTED, server-origin
	// text: route it through the presentation layer's terminal sanitiser before
	// printing it.
	ServerMessage string

	err error
}

func (e *APIError) Error() string { return e.err.Error() }
func (e *APIError) Unwrap() error { return e.err }

// generateError turns a non-200 from an orchestrator generation procedure into
// an actionable error, classified by HTTP status via civitai.TagStatus so the
// process exit code is pinned by errors.Is and never by message text.
//
// 🔴 This layer classifies FAITHFULLY BY STATUS and nothing else. civitai.TagStatus
// maps BOTH 401 and 403 to ErrUnauthorized -> exit 3 ("login required / credential
// lacks scope"), and several generation failures are not that. Discriminating them
// needs the server's message text, which is a presentation concern — so the
// re-mapping lives at the surface that owns the exit-code contract
// (internal/cmd/generate.go, classifyGenerateError), reading APIError.ServerMessage.
// Do not move it here: this function's status->kind mapping is pinned by tests.
//
// Which failures actually need re-mapping was MEASURED against the platform
// source, and the design doc's §7 table is wrong about two of them. tRPC derives
// the HTTP status from the TRPCError CODE, so an upstream orchestrator 403 does
// NOT reach the CLI as a 403:
//
//   - insufficient Buzz: the orchestrator's 403 is caught and re-thrown as
//     `throwInsufficientFundsError` -> code BAD_REQUEST -> the CLI sees **400**
//     (civitai/civitai -> src/server/services/orchestrator/workflows.ts:385,
//     src/server/utils/errorHandling.ts:218). So it exits 2 today, not 3.
//   - generation disabled / memberOnly: thrown as BAD_REQUEST -> **400**
//     (orchestrator.router.ts:379-390). Also not 3 today.
//   - account muted, and onboarding incomplete: bare FORBIDDEN -> **403**
//     (trpc.ts:389-401, :423-435). THESE are the ones that read byte-identical
//     to a scope error and would tell a restricted user to re-run `civitai login`
//     forever.
//   - prompt flagged: BAD_REQUEST -> **400**, and must never be retried.
//   - unknown ecosystem: a bare Error -> **500**, so it has no sentinel (exit 1).
func generateError(proc string, status int, raw []byte) (err error) {
	msg := serverMessage(raw)
	defer func() {
		err = civitai.TagStatus(status, &APIError{Proc: proc, Status: status, ServerMessage: msg, err: err})
	}()
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
