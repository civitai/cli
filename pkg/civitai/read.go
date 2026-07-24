package civitai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Reader is the read surface of the public Civitai REST API (`/api/v1/**`).
//
// These are all public GET endpoints: they accept the CLI's bearer token when
// one is configured (a personal API key or an OAuth device-login access token,
// refreshed transparently on a 401) but also work fully anonymously, since the
// public read routes do not enforce scope. Build the Client with an empty token
// (civitai.New(base, "")) to force anonymous requests.
type Reader interface {
	SearchModels(ctx context.Context, q url.Values) (*ModelSearchResult, error)
	GetModel(ctx context.Context, id string) (*ModelDetail, []byte, error)
	GetModelVersion(ctx context.Context, id string) (*ModelVersionDetail, []byte, error)
	GetModelVersionByHash(ctx context.Context, hash string) (*ModelVersionDetail, []byte, error)
	GetModelVersionsByHashes(ctx context.Context, hashes []string) ([]HashMatch, error)
	SearchImages(ctx context.Context, q url.Values) (*ImageSearchResult, error)
	SearchTags(ctx context.Context, q url.Values) (*TagSearchResult, error)
	SearchCreators(ctx context.Context, q url.Values) (*CreatorSearchResult, error)
	SearchUsers(ctx context.Context, q url.Values) (*UserSearchResult, error)
	SearchArticles(ctx context.Context, q url.Values) (*ArticleSearchResult, error)
	GetArticle(ctx context.Context, id string) (*ArticleDetail, []byte, error)
	SearchCollections(ctx context.Context, q url.Values) (*CollectionSearchResult, error)
	GetCollection(ctx context.Context, id string) (*CollectionDetail, []byte, error)
}

// Metadata is the pagination envelope the list endpoints return under
// `metadata`. Not every endpoint sets every field: the cursor-paged feeds
// (models/images) set nextCursor/nextPage (+ currentPage/pageSize when
// page-paged), while the tRPC-backed lists (tags/creators) set the classic
// totalItems/currentPage/pageSize/totalPages + nextPage/prevPage.
//
// nextCursor is left as RawMessage because the server emits it as a string, a
// number, or a date depending on the endpoint's sort key; CursorString renders
// it as a plain string for the user to feed back via --cursor.
type Metadata struct {
	NextCursor  json.RawMessage `json:"nextCursor,omitempty"`
	NextPage    string          `json:"nextPage,omitempty"`
	PrevPage    string          `json:"prevPage,omitempty"`
	CurrentPage *int            `json:"currentPage,omitempty"`
	PageSize    *int            `json:"pageSize,omitempty"`
	TotalItems  *int            `json:"totalItems,omitempty"`
	TotalPages  *int            `json:"totalPages,omitempty"`
}

// CursorString renders nextCursor as a bare string (unquoting a JSON string
// value) for display + re-use via --cursor. Returns "" when there is no cursor.
func (m Metadata) CursorString() string {
	raw := strings.TrimSpace(string(m.NextCursor))
	if raw == "" || raw == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.NextCursor, &s); err == nil {
		return s
	}
	return raw
}

// getRaw performs a public GET against path with the query q, returning the
// status code and raw body. The token (if any) is attached by authedDoHdr and a
// non-empty token is refreshed once on a 401; an empty token sends no
// Authorization header (anonymous). Transient failures (429/502/503/504 and
// transient network errors) are retried with bounded backoff via getWithRetry —
// this is an idempotent read, safe to replay. A non-2xx status is turned into a
// clear error by the caller via readError.
func (c *Client) getRaw(ctx context.Context, path string, q url.Values) (int, []byte, error) {
	full := c.BaseURL + path
	if enc := q.Encode(); enc != "" {
		full += "?" + enc
	}
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	}
	return c.getWithRetry(ctx, path, build)
}

// getInto GETs path+q, and on a 2xx unmarshals the body into out (when non-nil)
// and returns the raw body (for --json). A non-2xx returns a readError.
func (c *Client) getInto(ctx context.Context, path string, q url.Values, out any) ([]byte, error) {
	status, raw, err := c.getRaw(ctx, path, q)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, readError(status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, fmt.Errorf("unexpected response from %s (status %d): %s", path, status, snippet(raw))
		}
	}
	return raw, nil
}

// readError turns a non-2xx read response into a clear, actionable error,
// surfacing the API's own error body ({"error": ...} or {"message": ...})
// rather than a Go struct dump.
func readError(status int, raw []byte) (err error) {
	// The classification sentinel attached to the returned error, without
	// changing its user-visible message. Normally derived purely from the HTTP
	// status (401/404/429/503…), but the 429 branch may override `kind` for a
	// deep-paging cap (see below) — a structural, non-retryable rejection that
	// masquerades as a 429.
	kind := statusKind(status)
	defer func() { err = tag(kind, err) }()
	msg := strings.TrimSpace(string(raw))
	var wrapped struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &wrapped) == nil {
		if len(wrapped.Error) > 0 {
			// error may be a string or a nested object (zod issue list).
			var s string
			if json.Unmarshal(wrapped.Error, &s) == nil && s != "" {
				msg = s
			} else {
				msg = strings.TrimSpace(string(wrapped.Error))
			}
		} else if wrapped.Message != "" {
			msg = wrapped.Message
		}
	}
	msg = snippet([]byte(msg))
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized (401): %s — your token may be expired; run `civitai login` (these endpoints also work without a token)", msg)
	case http.StatusBadRequest:
		// A 400 is the server rejecting a bad flag value / query parameter (e.g.
		// an invalid enum caught by zod). Surface a CONCISE, actionable message —
		// naming the offending field/value when the body is a ZodError — instead
		// of dumping the raw ZodError JSON blob at the user.
		if d := badRequestDetail(raw); d != "" {
			return fmt.Errorf("invalid request parameter (400): %s", d)
		}
		return errors.New("invalid request parameter (400)")
	case http.StatusNotFound:
		return fmt.Errorf("not found (404): %s", msg)
	case http.StatusTooManyRequests:
		// A genuine 429 is throttling: transient, safe to back off and retry.
		// But the API also returns 429 for a PERMANENT deep-paging cap — a
		// `--page`/`--limit` product past the fixed offset ceiling (page*limit >
		// 1000). That request is structurally doomed: retrying the same page
		// loops forever. Detect the cap by its message and reclassify it as a
		// usage error (ErrBadRequest → exit 2) so a scripter's generic 429
		// backoff-and-retry loop doesn't spin on it, while a real throttle 429
		// stays ErrRateLimited (exit 6). The visible message is unchanged.
		if isDeepPagingCap(msg) {
			kind = ErrBadRequest
		}
		return fmt.Errorf("rate limited (429): %s — for deep paging use --cursor instead of --page", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable (503): %s — the search backend is briefly overloaded; retry shortly", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
}

// isDeepPagingCap reports whether a 429 error message is really the API's
// PERMANENT deep-paging cap (page*limit past the fixed offset ceiling) rather
// than transient throttling. The server phrases the cap as e.g. "You've
// requested too many pages, please use cursors instead"; a genuine throttle
// 429 carries none of these phrases. The match is deliberately narrow so a real
// rate-limit 429 is never misclassified as a usage error.
func isDeepPagingCap(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "too many pages") ||
		strings.Contains(m, "use cursors") ||
		strings.Contains(m, "deep paging")
}

// badRequestDetail extracts a concise, human-readable detail from a 400 response
// body. The Civitai API rejects a bad query parameter with a zod validation
// error whose body is either `{"error":{"name":"ZodError","message":"<JSON
// array of issues>"}}` or `{"error":[<issues>]}`; a simpler handler may return
// `{"error":"..."}` or `{"message":"..."}`. It returns the first issue as
// `field — message` (or just the message when there is no field path), or the
// plain string body, or "" when nothing usable can be parsed.
func badRequestDetail(raw []byte) string {
	var wrapped struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &wrapped) != nil {
		return ""
	}
	if len(wrapped.Error) > 0 {
		// error as a ZodError object: {"name":"ZodError","message":"<issues JSON>"}
		var zerr struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		if json.Unmarshal(wrapped.Error, &zerr) == nil && zerr.Name == "ZodError" {
			if d := firstZodIssue([]byte(zerr.Message)); d != "" {
				return snippet([]byte(d))
			}
		}
		// error as a bare array of zod issues.
		if d := firstZodIssue(wrapped.Error); d != "" {
			return snippet([]byte(d))
		}
		// error as a plain string.
		var s string
		if json.Unmarshal(wrapped.Error, &s) == nil && s != "" {
			return snippet([]byte(s))
		}
	}
	if wrapped.Message != "" {
		return snippet([]byte(wrapped.Message))
	}
	return ""
}

// firstZodIssue parses a JSON array of zod issues and renders the first one that
// carries a message as `field — message` (or just the message when the issue
// has no path). Returns "" when arr is not a parseable non-empty issue array.
func firstZodIssue(arr []byte) string {
	var issues []struct {
		Path    []json.RawMessage `json:"path"`
		Message string            `json:"message"`
	}
	if json.Unmarshal(arr, &issues) != nil {
		return ""
	}
	for _, is := range issues {
		if strings.TrimSpace(is.Message) == "" {
			continue
		}
		if field := zodPath(is.Path); field != "" {
			return field + " — " + is.Message
		}
		return is.Message
	}
	return ""
}

// zodPath renders a zod issue's path (a list of string keys and/or numeric
// indices) as a dotted field name, e.g. ["sort"] -> "sort", ["items",0,"id"] ->
// "items.0.id". Each element is a JSON string or number; strings are unquoted.
func zodPath(path []json.RawMessage) string {
	parts := make([]string, 0, len(path))
	for _, p := range path {
		var s string
		if json.Unmarshal(p, &s) == nil {
			parts = append(parts, s)
			continue
		}
		parts = append(parts, strings.TrimSpace(string(p)))
	}
	return strings.Join(parts, ".")
}

// maxResponseBody bounds how many bytes of a single HTTP response body the
// client reads into memory. It is deliberately far above any real Civitai JSON
// page — a `models search --limit 100` page tops out at a few MB — so a genuine
// API response is NEVER truncated, while a pathological/runaway body still can't
// exhaust memory. The previous 1 MiB cap silently truncated any response over
// ~1 MiB mid-JSON (e.g. `models search --limit 20`, ~1.04 MiB), so json.Unmarshal
// failed with a misleading "unexpected response (status 200)".
const maxResponseBody = 64 << 20 // 64 MiB

// maxBody returns the effective per-response body cap: the Client override when
// set (tests use a small value), else maxResponseBody.
func (c *Client) maxBody() int64 {
	if c.MaxResponseBody > 0 {
		return c.MaxResponseBody
	}
	return maxResponseBody
}

// readBody reads an entire HTTP response body, bounded by the client's cap.
func (c *Client) readBody(body io.Reader) ([]byte, error) {
	return readResponseBody(body, c.maxBody())
}

// readResponseBody reads an entire HTTP response body, bounded by limit. It
// reads one byte past the cap so it can DETECT an over-limit body and return a
// clear error instead of silently handing truncated bytes to json.Unmarshal —
// a silent truncation is exactly what caused the 1 MiB-cap bug. Under the cap it
// returns the full body (never truncating a real response).
func readResponseBody(body io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return raw, err
	}
	if int64(len(raw)) > limit {
		return raw[:limit], fmt.Errorf("response body exceeded %d MiB limit", limit>>20)
	}
	return raw, nil
}

// snippet bounds an error/body string so a huge response can't flood the
// terminal.
func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	const max = 500
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
