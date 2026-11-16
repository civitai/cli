package redact

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file holds the error-text side of redaction: what a CLI error may say
// about a response body from a route whose success payload IS a credential —
// OAuth tokens, a device_code, a dev token, a Forgejo push token, a presigned
// upload URL. Such a body is never shown, whatever its status and whether or
// not it decoded:
//
//   - a 2xx body that did not decode can still carry intact tokens beside the
//     one off-shape field (civitai/cli#800);
//   - a 2xx body that decoded but lacked the field the caller needed can carry
//     the credential under another key;
//   - a NON-2xx body without a recognised error field can be the success
//     payload under an unexpected status (a 201), or what a half-failed handler
//     had already built (a 500).
//
// What may be shown is a server error MESSAGE that ServerText accepts, the
// status, and the body's size.

// CredentialBodyError reports a body that did not yield what the caller needed,
// for a route whose success payload is a credential. It names the failure and
// the body's size, and never the body.
//
// 🔴 AN UNDECODABLE BODY IS NOT A BODY WITHOUT A CREDENTIAL. One off-shape field
// fails the whole json.Unmarshal while the tokens beside it are intact, and the
// refresh route has already rotated them — so echoing the body put the only
// copy of a live refresh token on stderr (civitai/cli#800, which introduced this
// helper in internal/appapi). decodeErr is safe to show: encoding/json's errors
// name a type, a field and at most one offending byte, never a string value. It
// is formatted with %v, not %w, so the error stays unclassified.
func CredentialBodyError(what string, raw []byte, decodeErr error) error {
	if decodeErr != nil {
		return fmt.Errorf("%s (%d bytes, not shown because it may carry credentials): %v", what, len(raw), decodeErr)
	}
	return fmt.Errorf("%s (%d bytes, not shown because it may carry credentials)", what, len(raw))
}

// WithheldBody describes a credential-route response body without showing it.
func WithheldBody(raw []byte) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "empty response body"
	}
	return fmt.Sprintf("%d-byte response body not shown because it may carry credentials", len(raw))
}

// maxServerText bounds a server message ServerText accepts. A real error
// message ("invalid_grant", "App not found", "Not authorized to upload") is a
// few dozen characters.
const maxServerText = 200

// ServerText reports whether s — a server's error MESSAGE field, or a body a
// route documents as bare error text — is short, single-line plain text, and
// returns it trimmed if so.
//
// It refuses: anything over maxServerText runes; invalid UTF-8; any control or
// format character (newlines, ESC, bidi overrides); text that opens like a JSON
// document; and text holding `://`, which every signed URL and every
// credential-in-userinfo clone URL carries.
//
// 🔴 THIS IS A SHAPE CHECK, NOT A SECRET DETECTOR. A bare token is short plain
// text and passes. It is applied only to a FIELD whose meaning is "error
// message" (an OAuth `error`/`error_description`, a tRPC `error.json.message`)
// or to a body the route documents as error text — never to a body the caller
// did not recognise, which is withheld whatever it looks like.
func ServerText(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxServerText {
		return "", false
	}
	if s[0] == '{' || s[0] == '[' || strings.Contains(s, "://") {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", false
		}
	}
	return s, true
}
