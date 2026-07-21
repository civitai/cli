package appapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// defaultTimeout governs the fast, interactive App-Blocks calls (whoami,
// submissions, dev-token). Kept short so a hung connection surfaces quickly.
const defaultTimeout = 30 * time.Second

// maxResponseBody bounds an App-Blocks response read (mirrors the SDK's read
// cap). A real civitai JSON body is far below this; the cap only guards against
// a pathological body.
const maxResponseBody = 64 << 20 // 64 MiB

// Client is the CLI-internal App Blocks HTTP client. It carries its own auth +
// submit plumbing (see appblocks.go) and shares only the read/download SDK's
// exported TokenSource contract and error-kind helpers.
type Client struct {
	BaseURL    string
	Tokens     civitai.TokenSource
	SubmitPath string // route for submit-version; CIVITAI_SUBMIT_PATH overrides
	HTTP       *http.Client
	// SubmitTimeout overrides the submit-upload timeout when non-zero; it
	// defaults to submitTimeout. Used by tests to exercise the timeout-recovery
	// path without a real slow upload.
	SubmitTimeout time.Duration
	// SubmitPollDelay overrides the inter-attempt delay of the post-timeout
	// recovery poll when set (>= 0 with the zero value meaning "use the
	// default"); tests set it to 0 to avoid sleeping.
	SubmitPollDelay *time.Duration
	// MaxResponseBody overrides the per-response body read cap (see
	// maxResponseBody) when > 0. Tests set a small value to exercise the over-cap
	// guard without allocating 64 MiB.
	MaxResponseBody int64
}

// New builds a Client with sane defaults from a static token (personal API key
// or a one-shot access token). For refreshable OAuth credentials use
// NewWithSource.
func New(baseURL, token, submitPath string) *Client {
	return NewWithSource(baseURL, civitai.StaticToken(token), submitPath)
}

// NewWithSource builds a Client backed by a TokenSource (which may refresh).
func NewWithSource(baseURL string, src civitai.TokenSource, submitPath string) *Client {
	if submitPath == "" {
		submitPath = DefaultSubmitPath
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Tokens:     src,
		SubmitPath: submitPath,
		HTTP:       &http.Client{Timeout: defaultTimeout},
	}
}

// maxBody returns the effective response-body cap for this client.
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
// clear error instead of silently handing truncated bytes to json.Unmarshal.
// Under the cap it returns the full body (never truncating a real response).
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
