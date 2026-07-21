// Package civitai is the importable HTTP client SDK for the Civitai API.
//
// It exposes the read/download surface — the Reader (models, model versions,
// images, tags, creators, users, articles, collections) and Downloader
// interfaces plus the typed result models — so external modules can import
// "github.com/civitai/cli/pkg/civitai" and drive the public API directly. Build
// a client with New(baseURL, personalAPIKey) for a personal-key consumer, or
// NewWithSource(baseURL, src) to supply a custom TokenSource (e.g. an OAuth
// access token that refreshes on a 401).
//
// Auth is supplied via a TokenSource so a refreshable credential's token is
// refreshed transparently before a request and once on a 401; a personal API
// key is a static source with no refresh. The App Blocks developer surface
// (submission, dev-tunnel, OAuth device login) is intentionally NOT part of
// this SDK — it lives in the CLI-internal package and is not a public contract.
package civitai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TokenSource yields the Bearer token to send. Refresh, if supported, refreshes
// the token (e.g. after a 401) and returns the new one; a static personal-key
// source returns ErrNoRefresh.
type TokenSource interface {
	// Token returns the current bearer token, refreshing first if it is known
	// to be expired. An empty token means "unauthenticated".
	Token(ctx context.Context) (string, error)
	// Refresh forces a refresh (used on a 401) and returns the new token.
	// Sources that cannot refresh return ("", ErrNoRefresh).
	Refresh(ctx context.Context) (string, error)
}

// ErrNoRefresh is returned by a non-refreshable (personal-key) TokenSource.
var ErrNoRefresh = fmt.Errorf("token source cannot refresh")

// StaticToken is a TokenSource for a fixed token (personal API key). It never
// refreshes.
type StaticToken string

// Token returns the static token.
func (s StaticToken) Token(context.Context) (string, error) { return string(s), nil }

// Refresh always fails: a personal key has no refresh path.
func (s StaticToken) Refresh(context.Context) (string, error) { return "", ErrNoRefresh }

// defaultTimeout governs the fast, interactive read calls. Kept short so a hung
// connection surfaces quickly.
const defaultTimeout = 30 * time.Second

// Client is the default HTTP implementation of the read/download SDK.
type Client struct {
	BaseURL string
	Tokens  TokenSource
	HTTP    *http.Client
	// RetryBackoffBase overrides the base delay of the transient-failure retry
	// on read GETs (see retry.go). A nil pointer means "use defaultRetryBackoff";
	// tests set it to a pointer-to-0 for instant, sleepless retries.
	RetryBackoffBase *time.Duration
	// Stderr receives the one-line transient-retry notices on read GETs; nil
	// defaults to os.Stderr. Tests point it at a buffer to assert the notice.
	Stderr io.Writer
	// MaxResponseBody overrides the per-response body read cap (see
	// maxResponseBody) when > 0. It bounds memory against a pathological body;
	// a real civitai JSON page is far below the default. Tests set a small value
	// to exercise the over-cap guard without allocating 64 MiB.
	MaxResponseBody int64
	// AllowPrivateDownloadHosts disables the download SSRF guard — the https-only
	// requirement AND the internal-range dial block (loopback/link-local/private/
	// ULA) enforced by downloadHTTPClient. It defaults to FALSE (production-safe):
	// a server-supplied downloadUrl (or redirect) that is plain-http or resolves
	// to a non-public IP is refused. ONLY the download tests set it true, because
	// their httptest servers bind plain-http loopback (127.0.0.1), which the guard
	// would otherwise block. Never set it in production code.
	AllowPrivateDownloadHosts bool
}

// New builds a Client with sane defaults from a static token (personal API key
// or a one-shot access token). For refreshable credentials use NewWithSource.
func New(baseURL, token string) *Client {
	return NewWithSource(baseURL, StaticToken(token))
}

// NewWithSource builds a Client backed by a TokenSource (which may refresh).
func NewWithSource(baseURL string, src TokenSource) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Tokens:  src,
		HTTP:    &http.Client{Timeout: defaultTimeout},
	}
}
