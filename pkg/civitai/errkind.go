package civitai

import (
	"errors"
	"net/http"
)

// Classification sentinels for the API's failure kinds. They carry NO
// user-facing text of their own — they are ATTACHED to the existing,
// message-bearing errors via tag/tagStatus so that errors.Is(err, ErrX)
// reports the FAILURE KIND while err.Error() stays byte-for-byte unchanged.
//
// Consumers (e.g. the CLI's process-exit-code mapping in cmd/civitai) branch on
// these; nothing here alters what a human sees printed on stderr.
var (
	// ErrUnauthorized marks an authentication/authorization failure — a missing,
	// expired, or invalid credential, or a credential lacking the needed scope
	// (HTTP 401/403, and the pre-flight "no token configured" guards).
	ErrUnauthorized = errors.New("authentication required")
	// ErrNotFound marks a requested resource that does not exist (HTTP 404).
	ErrNotFound = errors.New("not found")
	// ErrBadRequest marks a request the server rejected as malformed — an invalid
	// flag value or query parameter (HTTP 400, e.g. a bad enum caught by the
	// server's zod validation). It is a USAGE-class failure (the invocation is
	// wrong), so the entrypoint maps it to the same exit code as a client-side
	// usage error.
	ErrBadRequest = errors.New("bad request")
	// ErrRateLimited marks a throttled request (HTTP 429).
	ErrRateLimited = errors.New("rate limited")
	// ErrNetwork marks a transport/service-availability failure — the gateway/
	// overload family (HTTP 502/503/504) and exhausted transient retries.
	ErrNetwork = errors.New("network or service unavailable")
)

// taggedError attaches a classification sentinel (tag) to an underlying,
// message-bearing error WITHOUT changing the user-visible message: Error()
// delegates to the wrapped error, while Unwrap() exposes BOTH the original
// error chain AND the tag, so errors.Is matches either.
type taggedError struct {
	err error // original, message-bearing error (its text is preserved)
	tag error // classification sentinel (contributes no visible text)
}

func (t *taggedError) Error() string   { return t.err.Error() }
func (t *taggedError) Unwrap() []error { return []error{t.err, t.tag} }

// tag attaches classification sentinel k to err while preserving err's message.
// It is a no-op (returns err unchanged) when either err or k is nil.
func tag(k, err error) error {
	if err == nil || k == nil {
		return err
	}
	return &taggedError{err: err, tag: k}
}

// Tag attaches classification sentinel k to err while preserving err's exact
// message, so errors.Is(result, k) is true but result.Error() == err.Error().
// Exported for callers in sibling packages (e.g. internal/auth) that produce an
// error of a known kind. A nil err or k is returned unchanged.
func Tag(k, err error) error { return tag(k, err) }

// TagStatus attaches the classification sentinel for an HTTP status to err
// while preserving err's exact message (statuses with no specific kind leave
// err unchanged). Exported so command-layer status→error mappers in sibling
// packages classify identically to the api client's own mappers.
func TagStatus(status int, err error) error { return tagStatus(status, err) }

// statusKind maps an HTTP status to its classification sentinel, or nil when the
// status has no specific kind (a generic/unknown failure).
func statusKind(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusBadRequest:
		return ErrBadRequest
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrNetwork
	default:
		return nil
	}
}

// tagStatus attaches the classification sentinel for status to err (message
// preserved). Statuses with no specific kind leave err unchanged.
func tagStatus(status int, err error) error {
	return tag(statusKind(status), err)
}
