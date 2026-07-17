package api

import (
	"context"
	"encoding/json"
	"fmt"
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
// (api.New(base, "", "")) to force anonymous requests.
type Reader interface {
	SearchModels(ctx context.Context, q url.Values) (*ModelSearchResult, error)
	GetModel(ctx context.Context, id string) (*ModelDetail, []byte, error)
	GetModelVersion(ctx context.Context, id string) (*ModelVersionDetail, []byte, error)
	GetModelVersionByHash(ctx context.Context, hash string) (*ModelVersionDetail, []byte, error)
	SearchImages(ctx context.Context, q url.Values) (*ImageSearchResult, error)
	SearchTags(ctx context.Context, q url.Values) (*TagSearchResult, error)
	SearchCreators(ctx context.Context, q url.Values) (*CreatorSearchResult, error)
	SearchUsers(ctx context.Context, q url.Values) (*UserSearchResult, error)
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
// status code and raw body. The token (if any) is attached by authedDo and a
// non-empty token is refreshed once on a 401; an empty token sends no
// Authorization header (anonymous). A non-2xx status is turned into a clear
// error by the caller via readError.
func (c *Client) getRaw(ctx context.Context, path string, q url.Values) (int, []byte, error) {
	full := c.BaseURL + path
	if enc := q.Encode(); enc != "" {
		full += "?" + enc
	}
	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	}
	return c.authedDo(ctx, build)
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
func readError(status int, raw []byte) error {
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
	case http.StatusNotFound:
		return fmt.Errorf("not found (404): %s", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited (429): %s — for deep paging use --cursor instead of --page", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("service unavailable (503): %s — the search backend is briefly overloaded; retry shortly", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, msg)
	}
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
