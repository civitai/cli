package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/civitai/cli/pkg/civitai"
)

// Platform feedback — a developer (usually through their coding agent) telling
// CIVITAI about friction with the developer platform, and reading back whether
// it is known or fixed. It is the opposite lane from feedback.go, which is an
// app OWNER reading what the app's USERS wrote.
//
// 🔴 THIS FILE IS THE ONE PLACE THE WIRE CONTRACT LIVES — procedure names,
// field names, enums, bounds and the status→error mapping. It was written
// against a contract, NOT against a running server: the endpoint did not exist
// when this shipped (AGENTS.md item 41, "Not established"). So a late contract
// change is expected, and it must be a one-file change: the command layer names
// these types and constants and never restates a bound or a field name.
//
// Like the app-feedback procedures these are tRPC only — a non-batched GET with
// ?input={"json":{…}} for the query, a POST with a {"json":{…}} body for the
// mutation, success unwrapped from result.data.json.
const (
	PlatformFeedbackCreatePath    = "/api/trpc/platformFeedback.create"
	PlatformFeedbackGetStatusPath = "/api/trpc/platformFeedback.getStatus"
)

// PlatformFeedbackClientName is report.client.name: which client produced the
// report. The server's contract fixes it for this CLI.
const PlatformFeedbackClientName = "civitai-cli"

// PlatformFeedbackDocURLPrefix is what report.docUrl must start with.
const PlatformFeedbackDocURLPrefix = "https://developer.civitai.com/"

// The contract's bounds. 🔴 THE SERVER REJECTS AN OVERSIZE VALUE RATHER THAN
// CLIPPING IT, so every one of these is enforced on this side of the wire by
// PlatformFeedbackCreateInput.Validate, which CreatePlatformFeedback runs
// before it builds a request.
//
// Lengths are counted in UTF-16 code units (see utf16Len): the server's schema
// is zod, whose string `.max()` is JavaScript's `.length`.
const (
	PlatformFeedbackMessageMax      = 2000
	PlatformFeedbackRefMax          = 80
	PlatformFeedbackVersionMax      = 32
	PlatformFeedbackAgentMax        = 40
	PlatformFeedbackOSMax           = 16
	PlatformFeedbackArchMax         = 16
	PlatformFeedbackCommandMax      = 120
	PlatformFeedbackSDKMax          = 4
	PlatformFeedbackSDKNameMax      = 40
	PlatformFeedbackDocURLMax       = 300
	PlatformFeedbackErrorExcerptMax = 500
	PlatformFeedbackAppSlugMax      = 64
	// PlatformFeedbackStatusRefsMax is the most refs one getStatus call takes.
	PlatformFeedbackStatusRefsMax = 50
)

// PlatformFeedbackAbouts is report.about's enum: which part of the developer
// platform the report is about. Required.
var PlatformFeedbackAbouts = []string{"cli", "sdk", "docs", "review", "sandbox", "scopes", "mcp", "other"}

// PlatformFeedbackKinds is report.kind's enum.
var PlatformFeedbackKinds = []string{"bug", "confusing", "missing", "request"}

// PlatformFeedbackKindDefault is the kind sent when the caller chose none.
const PlatformFeedbackKindDefault = "bug"

// The status values the server reports for a piece of feedback.
const (
	PlatformFeedbackStatusReceived     = "received"
	PlatformFeedbackStatusAcknowledged = "acknowledged"
	PlatformFeedbackStatusFixed        = "fixed"
	PlatformFeedbackStatusAddressed    = "addressed"
	PlatformFeedbackStatusNotPlanned   = "not_planned"
)

// platformFeedbackRefRe is report.ref's pattern: a caller-chosen SITE KEY naming
// where in the platform the friction happened (e.g. `submit.server-refused`).
// It is not the reference the server hands back for a report.
var platformFeedbackRefRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)

// platformFeedbackStatusRefRe is the shape this CLI accepts for a
// SERVER-ISSUED report reference (e.g. `5123-3f9a01bc`) before it stores one,
// sends one back or prints one as a command to run. The contract gives an
// example, not a pattern, so this is a conservative superset of the example —
// alphanumerics and `.`, `_`, `-` — rather than a mirror of anything: its job is
// to keep whitespace, quotes and shell metacharacters out of a value that is
// echoed into a suggested command line.
var platformFeedbackStatusRefRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// platformFeedbackVersionRe is report.client.version's pattern.
var platformFeedbackVersionRe = regexp.MustCompile(`^[0-9A-Za-z.+-]+$`)

// PlatformFeedbackClient is report.client.
type PlatformFeedbackClient struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// PlatformFeedbackSDK is one entry of report.sdk.
type PlatformFeedbackSDK struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// PlatformFeedbackReport is the structured half of a report.
//
// Command, ExitCode and HTTPStatus are part of the contract and validated here,
// and NO flag of `civitai feedback send` sets them yet: they are kept so that
// the follow-up that attaches them from a failing command is not a wire change.
//
// 🔴 EVERY OPTIONAL FIELD IS ABSENT WHEN UNSET, NEVER A ZERO VALUE (AGENTS.md
// item 14). ExitCode and HTTPStatus are pointers for that reason: 0 is a real
// exit code, and `httpStatus: 0` is outside the server's 100–599 range and would
// be rejected. About, Kind and Client are always sent.
type PlatformFeedbackReport struct {
	About        string                 `json:"about"`
	Kind         string                 `json:"kind"`
	Ref          string                 `json:"ref,omitempty"`
	Client       PlatformFeedbackClient `json:"client"`
	Agent        string                 `json:"agent,omitempty"`
	OS           string                 `json:"os,omitempty"`
	Arch         string                 `json:"arch,omitempty"`
	Command      string                 `json:"command,omitempty"`
	ExitCode     *int                   `json:"exitCode,omitempty"`
	HTTPStatus   *int                   `json:"httpStatus,omitempty"`
	SDK          []PlatformFeedbackSDK  `json:"sdk,omitempty"`
	DocURL       string                 `json:"docUrl,omitempty"`
	ErrorExcerpt string                 `json:"errorExcerpt,omitempty"`
	AppSlug      string                 `json:"appSlug,omitempty"`
}

// PlatformFeedbackCreateInput is the platformFeedback.create input.
type PlatformFeedbackCreateInput struct {
	Message string                 `json:"message"`
	Report  PlatformFeedbackReport `json:"report"`
}

// PlatformFeedbackFixedIn names the release a fix shipped in.
type PlatformFeedbackFixedIn struct {
	Component string `json:"component"`
	Version   string `json:"version"`
}

// PlatformFeedbackCreateResult is the platformFeedback.create output.
//
// Duplicate is the SERVER's verdict that this report matches one it already
// holds; the CLI never decides that. Ref is the server-issued reference for the
// report (e.g. `5123-3f9a01bc`) — not the site key the caller sent as
// report.ref. Note is text Civitai wrote; it is data for a human to read and
// the command layer renders it as such.
type PlatformFeedbackCreateResult struct {
	Ref       string                   `json:"ref"`
	Duplicate bool                     `json:"duplicate"`
	Status    string                   `json:"status"`
	FixedIn   *PlatformFeedbackFixedIn `json:"fixedIn"`
	Note      *string                  `json:"note"`
	Reporters int                      `json:"reporters"`
	Reports   int                      `json:"reports"`
}

// PlatformFeedbackStatusItem is one element of the platformFeedback.getStatus
// output. Found is false for a ref the server does not hold (or will not show
// this caller); the remaining fields are then whatever the server sent, which
// the command layer does not render.
type PlatformFeedbackStatusItem struct {
	Ref         string                   `json:"ref"`
	Found       bool                     `json:"found"`
	Status      string                   `json:"status"`
	FixedIn     *PlatformFeedbackFixedIn `json:"fixedIn"`
	Note        *string                  `json:"note"`
	Reporters   int                      `json:"reporters"`
	FirstSeenAt *string                  `json:"firstSeenAt"`
	UpdatedAt   *string                  `json:"updatedAt"`
}

type platformFeedbackStatusInput struct {
	Refs []string `json:"refs"`
}

// utf16Len is the length JavaScript — and so the server's zod schema — sees.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if l := utf16.RuneLen(r); l > 0 {
			n += l
		} else {
			n++ // U+FFFD for an invalid rune: one unit
		}
	}
	return n
}

// ClipUTF16 returns the longest prefix of s that is at most max UTF-16 code
// units and ends on a rune boundary, and whether anything was cut.
func ClipUTF16(s string, max int) (string, bool) {
	n := 0
	for i, r := range s {
		l := utf16.RuneLen(r)
		if l < 1 {
			l = 1
		}
		if n+l > max {
			return s[:i], true
		}
		n += l
	}
	return s, false
}

// ValidPlatformFeedbackRef reports whether s is acceptable as report.ref.
func ValidPlatformFeedbackRef(s string) bool { return platformFeedbackRefRe.MatchString(s) }

// ValidPlatformFeedbackStatusRef reports whether s has the shape of a
// server-issued report reference. See platformFeedbackStatusRefRe.
func ValidPlatformFeedbackStatusRef(s string) bool { return platformFeedbackStatusRefRe.MatchString(s) }

// ValidPlatformFeedbackVersion reports whether s is acceptable as
// report.client.version.
func ValidPlatformFeedbackVersion(s string) bool {
	return s != "" && utf16Len(s) <= PlatformFeedbackVersionMax && platformFeedbackVersionRe.MatchString(s)
}

// PlatformFeedbackFieldError is one field of a report that the server would
// reject. Typed so the command layer can turn it into a usage error naming the
// flag that carries the field, without parsing a message.
type PlatformFeedbackFieldError struct {
	// Field is the wire path, e.g. "message" or "report.docUrl".
	Field string
	// Problem says what is wrong with it, in words that do not name a flag.
	Problem string
}

func (e *PlatformFeedbackFieldError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Problem)
}

func fieldErr(field, format string, args ...any) error {
	return &PlatformFeedbackFieldError{Field: field, Problem: fmt.Sprintf(format, args...)}
}

func oneOf(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}

// Validate checks the input against every bound and enum of the contract, and
// returns the FIRST violation as a *PlatformFeedbackFieldError.
//
// It never repairs anything. Deciding whether an over-long value is clipped
// (the error excerpt), dropped (an auto-detected field) or refused (something
// the user typed) is the command layer's call, made before this runs; this is
// the last gate, and what it refuses is not sent.
func (in PlatformFeedbackCreateInput) Validate() error {
	if in.Message != strings.TrimSpace(in.Message) {
		return fieldErr("message", "must be trimmed")
	}
	if n := utf16Len(in.Message); n < 1 || n > PlatformFeedbackMessageMax {
		return fieldErr("message", "must be 1 to %d characters, got %d", PlatformFeedbackMessageMax, n)
	}
	r := in.Report
	// The server answers BAD_REQUEST for a NUL or an unpaired surrogate in any
	// free-text field. In a Go string the latter can only exist as invalid UTF-8.
	for _, c := range []struct{ field, value string }{
		{"message", in.Message},
		{"report.ref", r.Ref}, {"report.client.version", r.Client.Version},
		{"report.agent", r.Agent}, {"report.os", r.OS}, {"report.arch", r.Arch},
		{"report.command", r.Command}, {"report.docUrl", r.DocURL},
		{"report.errorExcerpt", r.ErrorExcerpt}, {"report.appSlug", r.AppSlug},
	} {
		if strings.IndexByte(c.value, 0) >= 0 || !utf8.ValidString(c.value) {
			return fieldErr(c.field, "must not contain a NUL or invalid text")
		}
	}
	if !oneOf(PlatformFeedbackAbouts, r.About) {
		return fieldErr("report.about", "must be one of %s, got %q", strings.Join(PlatformFeedbackAbouts, ", "), r.About)
	}
	if !oneOf(PlatformFeedbackKinds, r.Kind) {
		return fieldErr("report.kind", "must be one of %s, got %q", strings.Join(PlatformFeedbackKinds, ", "), r.Kind)
	}
	if r.Ref != "" && !ValidPlatformFeedbackRef(r.Ref) {
		return fieldErr("report.ref", "must be 1 to %d characters of a-z, 0-9, '.', '_' or '-', starting with a letter or digit",
			PlatformFeedbackRefMax)
	}
	if r.Client.Name != PlatformFeedbackClientName {
		return fieldErr("report.client.name", "must be %q, got %q", PlatformFeedbackClientName, r.Client.Name)
	}
	if !ValidPlatformFeedbackVersion(r.Client.Version) {
		return fieldErr("report.client.version", "must be 1 to %d characters of letters, digits, '.', '+' or '-'",
			PlatformFeedbackVersionMax)
	}
	for _, c := range []struct {
		field, value string
		max          int
	}{
		{"report.agent", r.Agent, PlatformFeedbackAgentMax},
		{"report.os", r.OS, PlatformFeedbackOSMax},
		{"report.arch", r.Arch, PlatformFeedbackArchMax},
		{"report.command", r.Command, PlatformFeedbackCommandMax},
		{"report.docUrl", r.DocURL, PlatformFeedbackDocURLMax},
		{"report.errorExcerpt", r.ErrorExcerpt, PlatformFeedbackErrorExcerptMax},
		{"report.appSlug", r.AppSlug, PlatformFeedbackAppSlugMax},
	} {
		if n := utf16Len(c.value); n > c.max {
			return fieldErr(c.field, "must be at most %d characters, got %d", c.max, n)
		}
	}
	if r.DocURL != "" && !strings.HasPrefix(r.DocURL, PlatformFeedbackDocURLPrefix) {
		return fieldErr("report.docUrl", "must start with %s", PlatformFeedbackDocURLPrefix)
	}
	if r.ExitCode != nil && (*r.ExitCode < 0 || *r.ExitCode > 255) {
		return fieldErr("report.exitCode", "must be 0 to 255, got %d", *r.ExitCode)
	}
	if r.HTTPStatus != nil && (*r.HTTPStatus < 100 || *r.HTTPStatus > 599) {
		return fieldErr("report.httpStatus", "must be 100 to 599, got %d", *r.HTTPStatus)
	}
	if len(r.SDK) > PlatformFeedbackSDKMax {
		return fieldErr("report.sdk", "must hold at most %d entries, got %d", PlatformFeedbackSDKMax, len(r.SDK))
	}
	for i, s := range r.SDK {
		if n := utf16Len(s.Name); n < 1 || n > PlatformFeedbackSDKNameMax {
			return fieldErr(fmt.Sprintf("report.sdk[%d].name", i), "must be 1 to %d characters, got %d", PlatformFeedbackSDKNameMax, n)
		}
		if n := utf16Len(s.Version); n < 1 || n > PlatformFeedbackVersionMax {
			return fieldErr(fmt.Sprintf("report.sdk[%d].version", i), "must be 1 to %d characters, got %d", PlatformFeedbackVersionMax, n)
		}
	}
	return nil
}

// PlatformFeedbackCreateBody is the exact request body CreatePlatformFeedback
// POSTs for in. It is exported for one caller: `civitai feedback send
// --dry-run`, which must show what WOULD be sent without a second marshaller
// that could drift from the real one.
func PlatformFeedbackCreateBody(in PlatformFeedbackCreateInput) ([]byte, error) {
	return json.Marshal(map[string]any{"json": in})
}

// CreatePlatformFeedback sends one report.
//
// 🔴 IT VALIDATES FIRST AND SENDS NOTHING THAT FAILS. See Validate.
func (c *Client) CreatePlatformFeedback(ctx context.Context, in PlatformFeedbackCreateInput) (*PlatformFeedbackCreateResult, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	body, err := PlatformFeedbackCreateBody(in)
	if err != nil {
		return nil, err
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+PlatformFeedbackCreatePath, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, platformFeedbackError(status, raw)
	}
	var out PlatformFeedbackCreateResult
	if err := decodeFeedbackData(raw, &out, trpcName(PlatformFeedbackCreatePath)); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPlatformFeedbackStatus reads the status of up to
// PlatformFeedbackStatusRefsMax reports by their server-issued refs.
func (c *Client) GetPlatformFeedbackStatus(ctx context.Context, refs []string) ([]PlatformFeedbackStatusItem, error) {
	if len(refs) < 1 || len(refs) > PlatformFeedbackStatusRefsMax {
		return nil, fieldErr("refs", "must hold 1 to %d refs, got %d", PlatformFeedbackStatusRefsMax, len(refs))
	}
	inputJSON, err := json.Marshal(map[string]any{"json": platformFeedbackStatusInput{Refs: refs}})
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + PlatformFeedbackGetStatusPath + "?" + q.Encode()
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, platformFeedbackError(status, raw)
	}
	out := []PlatformFeedbackStatusItem{}
	if err := decodeFeedbackData(raw, &out, trpcName(PlatformFeedbackGetStatusPath)); err != nil {
		return nil, err
	}
	return out, nil
}

// PlatformFeedbackOffError is the server saying it is not collecting platform
// feedback: the area's flag is off. Typed so the command layer can recognise it
// without reading a message, and so that it carries NO classification — see
// platformFeedbackError for why this 403 must not exit like a credential
// problem.
type PlatformFeedbackOffError struct {
	// ServerMsg is the server's own text.
	ServerMsg string
}

// PlatformFeedbackOffText is the stable lead of the collection-off error. A
// named constant so the tests quote it rather than paraphrase.
const PlatformFeedbackOffText = "Civitai is not collecting platform feedback right now"

func (e *PlatformFeedbackOffError) Error() string {
	// Verb-neutral on purpose: one mapper serves `send` and `status`, and a
	// caller of either must not be told to re-run the other.
	return fmt.Sprintf("%s (403): %s — that is the server's answer, not a problem with your login; "+
		"try the same command again later", PlatformFeedbackOffText, e.ServerMsg)
}

// isCollectionOffMsg detects the server's collection-is-off refusal.
//
// It is the only wire signal separating that state from every other FORBIDDEN —
// tRPC gives them one code — so, exactly like isInsufficientScopeMsg, it keys on
// the stable core of the server's sentence ("Feedback is not being collected
// here right now.") rather than on the whole of it.
//
// 🔴 IT IS DELIBERATELY NARROW. A 403 that says neither this nor the scope
// sentence is NOT called "collection is off": the CLI cannot observe why it was
// refused, so it reports the refusal and the server's words and stops (item 28).
func isCollectionOffMsg(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not being collected") || strings.Contains(m, "not collecting")
}

// platformFeedbackError maps a non-200 from a platform-feedback procedure to an
// actionable error. The arms, and the exit code each one's classification
// produces (cmd/civitai/main.go):
//
//   - 401 — the shared not-logged-in remedy. Exit 3.
//   - 403 with the token-scope sentence — the credential's scope does not cover
//     the procedure. Exit 3.
//   - 403 with the collection-off sentence — *PlatformFeedbackOffError, and
//     🔴 UNTAGGED ON PURPOSE: the status is 403 but nothing about the caller's
//     credential is wrong, so it must not land on the code a script answers
//     with `civitai login`. Exit 1.
//   - any other 403 — refused for a reason this CLI cannot name. Exit 3, with
//     the server's words and no guess.
//   - 404 — tRPC's answer for a procedure the server does not have. Exit 4.
//     Neither procedure throws NOT_FOUND itself (an unknown ref is
//     `found: false` in a 200).
//   - 429 — rate limited. Exit 6.
//   - 400 — the server's schema rejected a request Validate accepted. Exit 2.
func platformFeedbackError(status int, raw []byte) (err error) {
	tagged := true
	defer func() {
		if tagged {
			err = civitai.TagStatus(status, err)
		}
	}()
	var env struct {
		Error struct {
			JSON struct {
				Message string `json:"message"`
			} `json:"json"`
		} `json:"error"`
	}
	msg := serverMessage(raw)
	if json.Unmarshal(raw, &env) == nil && env.Error.JSON.Message != "" {
		msg = env.Error.JSON.Message
	}
	msg = strings.TrimSpace(msg)
	switch status {
	case http.StatusUnauthorized:
		return unauthorizedError(msg)
	case http.StatusForbidden:
		if isInsufficientScopeMsg(msg) {
			return fmt.Errorf("this credential's scope does not cover platform feedback (403): %s — "+
				"it needs the Apps submit scope, which a default `civitai login` asks for: re-run `civitai login` if "+
				"yours predates that scope or was created with narrower --scopes. A full-scope personal API key also "+
				"works (create one at https://civitai.com/user/account and run `civitai login --token <key>`); "+
				"`civitai whoami` shows which credential is active", msg)
		}
		if isCollectionOffMsg(msg) {
			tagged = false
			return &PlatformFeedbackOffError{ServerMsg: msg}
		}
		return fmt.Errorf("the server refused this platform-feedback request (403): %s — this CLI cannot tell why "+
			"beyond the server's own words; `civitai whoami` shows which account and credential are active", msg)
	case http.StatusNotFound:
		// 🔴 A 404 SAYS THE PROCEDURE IS NOT THERE AND NOTHING ABOUT WHY. It is
		// not worded as a server that is too old, or one that will have it later:
		// the CLI shipped before any server had this API, so either guess would be
		// a claim about history or the future that the status does not carry.
		return fmt.Errorf("this server does not offer platform feedback (404): %s — "+
			"the server has no such API, and that is all this CLI can tell (CIVITAI_BASE_URL selects the server)", msg)
	case http.StatusBadRequest:
		// The CLI validated the request against the limits it knows and the
		// server still refused it. WHY is in the server's message or nowhere:
		// naming a cause here (limits changed, upgrade) would be a guess.
		return fmt.Errorf("the server rejected this platform-feedback request (400): %s — "+
			"it passed this CLI's own checks, so the server's message is all there is to go on", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited (429): %s — wait before running the same command again", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("platform feedback unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}
