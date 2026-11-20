package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
// helper in internal/appapi). decodeErr is shown: encoding/json's errors name a
// type, a field and one quoted offending byte, never a string value — but an
// UnmarshalTypeError quotes a NUMBER value whole, so a 50,000-digit number made
// a 50 KB error. The decode error is therefore cut at decodeErrMax runes. It is
// formatted as text, not wrapped with %w, so the error stays unclassified.
func CredentialBodyError(what string, raw []byte, decodeErr error) error {
	if decodeErr != nil {
		return fmt.Errorf("%s (%d bytes, not shown because it may carry credentials): %s", what, len(raw), capRunes(decodeErr.Error(), decodeErrMax))
	}
	return fmt.Errorf("%s (%d bytes, not shown because it may carry credentials)", what, len(raw))
}

// decodeErrMax bounds the decode error CredentialBodyError shows. encoding/json's
// own messages run well under it; only a quoted number value reaches it.
const decodeErrMax = 200

// capRunes returns s cut to at most n runes, marking a cut with "…".
func capRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// WithheldBody describes a credential-route response body without showing it.
func WithheldBody(raw []byte) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "empty response body"
	}
	return fmt.Sprintf("%d-byte response body not shown because it may carry credentials", len(raw))
}

// ServerText reports whether s — a server's error MESSAGE field, or a body a
// route documents as bare error text — is single-line plain text, and returns it
// trimmed if so.
//
// It refuses: invalid UTF-8; any control or format character (newlines, ESC,
// bidi overrides); text that opens like a JSON document; and text holding `://`,
// which every signed URL and every credential-in-userinfo clone URL carries.
//
// There is NO length limit. A length cap refuses real messages, and a caller
// that branched on the text it got back lost the server's wording with them —
// the dev-token route's owned-but-not-deployed 404 is 179 runes plus the
// blockId, and a 200-rune cap made a long slug's 404 read as a slug collision.
// Branch on ErrorMessage, never on what this returns.
//
// In place of the cap it refuses three TOKEN SHAPES, wherever they sit in the
// text: a JWT (three dot-separated base64url segments opening `eyJ`, the third
// may be empty); a run of 40 or more base64/base64url characters that is all
// hex or mixes upper- and lower-case letters; and an UNBROKEN run of 40 or more
// ASCII letters and digits, whatever its case. The third is what refuses a
// single-case token (a lowercase or base32 key) while still letting a long
// slug, snake_case name or path through: those break into short pieces at `-`,
// `_`, `/` and `.` — the 30-char-slug 404 above is all such pieces — and no
// English word reaches 40 letters without a separator. ACCEPTED FALSE
// POSITIVE: a run of 40+ letters/digits with no separator is withheld even when
// it is not a token — notably an app slug of 40+ characters with no hyphen,
// which the blockId patterns allow. Such a message is reported by its size; a
// caller that classifies on the wording reads ErrorMessage, unfiltered, so the
// classification is unaffected.
//
// 🔴 THIS IS A SHAPE CHECK, NOT A SECRET DETECTOR. A short token, or a long
// single-case one broken up by `-`, `_` or `/`, is plain text and passes. It is applied only
// to a FIELD whose meaning is "error message" (an OAuth
// `error`/`error_description`, a tRPC `error.json.message`) or to a body the
// route documents as error text — never to a body the caller did not
// recognise, which is withheld whatever it looks like.
func ServerText(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) {
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
	if hasTokenShape(s) {
		return "", false
	}
	return s, true
}

// tokenRunMin is the shortest unbroken base64/base64url/hex run hasTokenShape
// treats as a possible token. 40 is a SHA-1 hex digest. It is NOT below every
// credential this CLI handles: a Civitai API key is 32 hex characters, so a bare
// API key inside server text passes this check.
const tokenRunMin = 40

var (
	jwtShape = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`)
	tokenRun = regexp.MustCompile(`[A-Za-z0-9_\-+/=]{` + strconv.Itoa(tokenRunMin) + `,}`)
	alnumRun = regexp.MustCompile(`[A-Za-z0-9]{` + strconv.Itoa(tokenRunMin) + `,}`)
	hexRun   = regexp.MustCompile(`^[0-9A-Fa-f]+$`)
)

// hasTokenShape reports whether s holds a JWT, an unbroken run of tokenRunMin+
// ASCII letters and digits, or a run of tokenRunMin+ token characters that is
// all hex or holds both an upper- and a lower-case letter. See ServerText for
// why a single-case run broken by separators passes.
func hasTokenShape(s string) bool {
	if jwtShape.MatchString(s) || alnumRun.MatchString(s) {
		return true
	}
	for _, run := range tokenRun.FindAllString(s, -1) {
		if hexRun.MatchString(run) {
			return true
		}
		if strings.ContainsAny(run, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") && strings.ContainsAny(run, "abcdefghijklmnopqrstuvwxyz") {
			return true
		}
	}
	return false
}

// ErrorMessage returns a JSON error body's MESSAGE field, unfiltered: a tRPC
// `error.json.message`, else a top-level string `message`, else a top-level
// string `error`; "" when the body has none.
//
// 🔴 IT IS FOR CLASSIFYING A FAILURE, NEVER FOR SHOWING ONE. A caller that tells
// two errors apart by the server's wording must read that wording HERE, before
// any display filter — reading it after ServerText meant a message the filter
// withheld lost its wording, and the dev-token 404 that says "no live
// deployment" was taken for a slug collision and renamed the developer's app.
// What may be shown is CredentialRouteMessage.
func ErrorMessage(raw []byte) string {
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
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	for _, key := range []string{"message", "error"} {
		var s string
		if json.Unmarshal(fields[key], &s) == nil && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// CredentialRouteMessage is what an error on a route whose success payload IS a
// credential (OAuth tokens, a dev token, a Forgejo push token, a presigned URL)
// may say about a non-2xx body: its ErrorMessage when ServerText accepts it, and
// otherwise only the body's size.
//
// 🔴 NEVER THE BODY. A body without a recognised message field can be the
// success payload under an unexpected status (a 201) or what a half-failed
// handler had built (a 500), so the old raw-body fallback printed
// `getMyForgejoCloneInfo failed (HTTP 201): {…"token":"…"}`.
func CredentialRouteMessage(raw []byte) string {
	if msg, ok := ServerText(ErrorMessage(raw)); ok {
		return msg
	}
	return WithheldBody(raw)
}

// CredentialRouteProblem is CredentialRouteMessage for a route that may answer
// in RFC 7807 problem-details form: a body with no message field shows its
// `title` and `detail`, each only when ServerText accepts it, as "title: detail".
// No other problem-details member is read — `type` and `instance` are URIs, and
// an `instance` can be a signed URL.
func CredentialRouteProblem(raw []byte) string {
	if ErrorMessage(raw) == "" {
		var p struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}
		if json.Unmarshal(raw, &p) == nil {
			var parts []string
			for _, f := range []string{p.Title, p.Detail} {
				if s, ok := ServerText(f); ok {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ": ")
			}
		}
	}
	return CredentialRouteMessage(raw)
}

// CredentialRouteText is CredentialRouteMessage for a route documented to
// answer some failures with a BARE TEXT body: a body with no message field is
// shown whole when ServerText accepts it. ServerText refuses JSON, so a JSON
// body without a message field is still withheld.
func CredentialRouteText(raw []byte) string {
	if ErrorMessage(raw) == "" {
		if msg, ok := ServerText(string(raw)); ok {
			return msg
		}
	}
	return CredentialRouteMessage(raw)
}
