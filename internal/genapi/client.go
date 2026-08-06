// Package genapi is the CLI-internal client for the orchestrator generation
// graph (the tRPC `orchestrator.*` procedures behind `civitai generate`).
//
// It is deliberately NOT part of pkg/civitai: that package is the public
// read/download SDK (see pkg/civitai/api.go), and generation is a
// money-spending developer surface whose wire shape is not a public contract.
// It shares only the SDK's exported TokenSource contract, its error-kind
// helpers, and the existing public model-version read route.
//
// The package mirrors internal/appapi's shape on purpose — same TokenSource
// seam, same authedDo/doOnceWith transparent-401-refresh plumbing, same
// response-body cap — so there is one auth/transport pattern in the CLI rather
// than two.
package genapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// defaultTimeout governs the fast, interactive calls (whatIf cost estimation,
// model-version resolution). Kept short so a hung connection surfaces quickly.
const defaultTimeout = 30 * time.Second

// submitTimeout governs the generate submit specifically. The procedure does a
// synchronous orchestrator hop (validate graph -> build steps -> submitWorkflow,
// itself retried up to 3x server-side), so it can take well over the fast-call
// timeout. Scope this longer window to the submit only — do NOT lengthen the
// interactive calls.
const submitTimeout = 120 * time.Second

// maxResponseBody bounds a response read (mirrors the SDK's read cap). A real
// orchestrator JSON body is far below this; the cap only guards against a
// pathological body.
const maxResponseBody = 64 << 20 // 64 MiB

// VersionGetter is the seam for resolving a model-version id. It is satisfied
// by *civitai.Client — the generation client REUSES the existing public
// `GET /api/v1/model-versions/{id}` route rather than duplicating it, so the
// SSRF/retry/error-kind behaviour of the read SDK is inherited unchanged.
type VersionGetter interface {
	GetModelVersion(ctx context.Context, id string) (*civitai.ModelVersionDetail, []byte, error)
}

// Client is the CLI-internal orchestrator-generation HTTP client.
type Client struct {
	BaseURL string
	Tokens  civitai.TokenSource
	HTTP    *http.Client
	// Versions resolves a model-version id to its model TYPE + display name
	// (see versions.go). nil falls back to a client built from BaseURL/Tokens.
	Versions VersionGetter
	// SubmitTimeout overrides the submit timeout when non-zero; it defaults to
	// submitTimeout. Tests set it to avoid depending on the production window.
	SubmitTimeout time.Duration
	// MaxResponseBody overrides the per-response body read cap (see
	// maxResponseBody) when > 0. Tests set a small value to exercise the
	// over-cap guard without allocating 64 MiB.
	MaxResponseBody int64
}

// New builds a Client from a static token (personal API key). Generation is NOT
// personal-key-only: an OAuth login that opted in via `civitai login --scopes
// generate` carries the AI Services scopes the orchestrator procedures require.
// Only a DEFAULT `civitai login` lacks them. Prefer NewWithSource so an OAuth
// token can refresh; this constructor is for a fixed token (e.g. CIVITAI_TOKEN).
func New(baseURL, token string) *Client {
	return NewWithSource(baseURL, civitai.StaticToken(token))
}

// NewWithSource builds a Client backed by a TokenSource (which may refresh).
func NewWithSource(baseURL string, src civitai.TokenSource) *Client {
	base := strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL:  base,
		Tokens:   src,
		HTTP:     &http.Client{Timeout: defaultTimeout},
		Versions: civitai.NewWithSource(base, src),
	}
}

// maxBody returns the effective response-body cap for this client.
func (c *Client) maxBody() int64 {
	if c.MaxResponseBody > 0 {
		return c.MaxResponseBody
	}
	return maxResponseBody
}

// readBody reads an entire HTTP response body, bounded by the client's cap. It
// reads one byte past the cap so it can DETECT an over-limit body and return a
// clear error instead of silently handing truncated bytes to json.Unmarshal.
func (c *Client) readBody(body io.Reader) ([]byte, error) {
	limit := c.maxBody()
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return raw, err
	}
	if int64(len(raw)) > limit {
		return raw[:limit], fmt.Errorf("response body exceeded %d MiB limit", limit>>20)
	}
	return raw, nil
}

// submitClient returns an *http.Client for the generate submit: it mirrors the
// shared client's transport but uses the longer submitTimeout.
func (c *Client) submitClient() *http.Client {
	base := c.HTTP
	if base == nil {
		base = &http.Client{}
	}
	timeout := submitTimeout
	if c.SubmitTimeout > 0 {
		timeout = c.SubmitTimeout
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     base.Transport,
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
	}
}

// httpClient returns the shared client, tolerating a zero-value Client.
func (c *Client) httpClient() *http.Client {
	if c.HTTP == nil {
		return &http.Client{Timeout: defaultTimeout}
	}
	return c.HTTP
}

// token returns the bearer token, refusing an unauthenticated call up front
// with an ErrUnauthorized-tagged error (every orchestrator procedure is
// scope-gated, so an anonymous call can only 401).
func (c *Client) token(ctx context.Context) (string, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return "", err
	}
	if tok == "" {
		return "", civitai.Tag(civitai.ErrUnauthorized,
			fmt.Errorf("no token configured — run `civitai login --scopes generate` (a browser login that opts into generation) or `civitai login --token <personal API key>` (or set CIVITAI_TOKEN)"))
	}
	return tok, nil
}

// authedDo performs build()'s request with the current token, refreshing once
// and replaying on a 401 (a non-refreshable personal-key source returns
// ErrNoRefresh and the original 401 stands).
//
// 🔴 The replay re-sends the request body. That is safe for the calls in this
// package only because every mutation carries an unconditional `externalId`
// (see generate.go): the orchestrator dedupes on (userId, externalId), so a
// replayed submit returns the pre-existing workflow instead of charging twice.
// Do not add a mutation here that omits it.
func (c *Client) authedDo(ctx context.Context, build func() (*http.Request, error)) (int, []byte, error) {
	return c.authedDoWith(ctx, c.httpClient(), build)
}

// authedDoWith is authedDo against a specific *http.Client, so the slow submit
// can use a longer-timeout client without affecting the fast interactive calls.
func (c *Client) authedDoWith(ctx context.Context, httpClient *http.Client, build func() (*http.Request, error)) (int, []byte, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return 0, nil, err
	}
	status, raw, err := c.doOnceWith(httpClient, build, tok)
	if err != nil {
		return 0, nil, err
	}
	if status == http.StatusUnauthorized {
		newTok, rerr := c.Tokens.Refresh(ctx)
		if rerr == nil && newTok != "" {
			return c.doOnceWith(httpClient, build, newTok)
		}
	}
	return status, raw, nil
}

func (c *Client) doOnceWith(httpClient *http.Client, build func() (*http.Request, error), token string) (int, []byte, error) {
	req, err := build()
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := c.readBody(resp.Body)
	if err != nil {
		return resp.StatusCode, raw, err
	}
	return resp.StatusCode, raw, nil
}
