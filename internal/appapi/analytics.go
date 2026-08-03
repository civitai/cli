package appapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/civitai/cli/pkg/civitai"
)

// AppAnalyticsPath is the owner-only tRPC query returning one App Block's
// analytics for the authenticated owner (civitai/civitai
// src/server/routers/blocks.router.ts -> blocks.getMyAppAnalytics). It is a
// non-batched tRPC GET: the input rides in ?input={"json":{...}} and the success
// envelope is {"result":{"data":{"json":{...}}}}, exactly like
// GetForgejoCloneInfo / GetBuzzAccount.
const AppAnalyticsPath = "/api/trpc/blocks.getMyAppAnalytics"

// AnalyticsRange is the window the SERVER actually served. It is NOT necessarily
// the window that was requested: the server defaults to the last 30 days when
// from/to are omitted and clamps a longer request to 366 days, so the caller must
// echo THIS range (never the flags) when rendering — otherwise a zero count is
// ambiguous about the period it covers.
type AnalyticsRange struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Granularity is the bucket size of the series: "day", or "week" once the
	// window exceeds ~60 days (the server switches on its own).
	Granularity string `json:"granularity"`
}

// AnalyticsPoint is one bucket of a time series. Value is float64 because the
// server's aggregates are not all integral (rates/averages can appear here).
type AnalyticsPoint struct {
	Bucket string  `json:"bucket"`
	Value  float64 `json:"value"`
}

// AnalyticsInstalls is the install-side rollup.
type AnalyticsInstalls struct {
	Total  int64            `json:"total"`
	Active int64            `json:"active"`
	Series []AnalyticsPoint `json:"series"`
}

// AnalyticsRuns is the run-side rollup: how often the app ran and how much Buzz
// those runs spent.
type AnalyticsRuns struct {
	Count     int64            `json:"count"`
	BuzzSpent int64            `json:"buzzSpent"`
	Series    []AnalyticsPoint `json:"series"`
}

// AnalyticsBuzzPurchased is the revenue rollup: Buzz bought through the app.
// GrossCents is USD cents.
type AnalyticsBuzzPurchased struct {
	Count      int64 `json:"count"`
	BuzzAmount int64 `json:"buzzAmount"`
	GrossCents int64 `json:"grossCents"`
}

// AnalyticsScopeCount is one row of the top-scopes breakdown.
type AnalyticsScopeCount struct {
	Scope string `json:"scope"`
	Count int64  `json:"count"`
}

// AnalyticsEndpointCount is one row of the top-endpoints breakdown.
type AnalyticsEndpointCount struct {
	Endpoint string `json:"endpoint"`
	Count    int64  `json:"count"`
}

// AnalyticsEngagement is the API-side rollup. IMPORTANT: it counts only
// AUTHENTICATED, scope-gated API calls made by the app — an app with no scoped
// API surface reads flat here while installs/revenue are non-zero. That is the
// data's shape, not a bug.
type AnalyticsEngagement struct {
	APICalls     int64                    `json:"apiCalls"`
	ActiveUsers  int64                    `json:"activeUsers"`
	ErrorRate    float64                  `json:"errorRate"`
	TopScopes    []AnalyticsScopeCount    `json:"topScopes"`
	TopEndpoints []AnalyticsEndpointCount `json:"topEndpoints"`
}

// AppAnalytics mirrors the blocks.getMyAppAnalytics result. Field names + JSON
// casing track the server EXACTLY.
type AppAnalytics struct {
	Range AnalyticsRange `json:"range"`
	// NotOwned is the ENTITLEMENT signal, and the reason the command layer must
	// branch before it renders: the proc sits behind the `appBlocksAuthor` feature
	// flag (moderators-only by default) and, when the caller does not match it or
	// does not own the app, answers HTTP 200 with every counter zeroed rather than
	// an error. A renderer that ignores NotOwned therefore prints a
	// plausible-looking empty dashboard for what is really a permission failure.
	NotOwned      bool                   `json:"notOwned"`
	Installs      AnalyticsInstalls      `json:"installs"`
	Runs          AnalyticsRuns          `json:"runs"`
	BuzzPurchased AnalyticsBuzzPurchased `json:"buzzPurchased"`
	Engagement    AnalyticsEngagement    `json:"engagement"`
}

// AppAnalyticsReader reads the caller's own App Block analytics.
type AppAnalyticsReader interface {
	// GetMyAppAnalytics returns the analytics for appBlockID over [from, to].
	// Both bounds are optional RFC3339 strings; omitting BOTH takes the server's
	// 30-day default. It returns the decoded result AND the raw unwrapped payload
	// (the tRPC envelope's .result.data.json) so `--json` can passthrough every
	// server field, including ones this struct does not model yet.
	GetMyAppAnalytics(ctx context.Context, appBlockID, from, to string) (*AppAnalytics, json.RawMessage, error)
}

// analyticsInput mirrors the blocks.getMyAppAnalytics zod input. From/To are
// `omitempty` so an unset bound is genuinely absent from the wire (the server
// then applies its 30-day default) rather than being sent as an empty string,
// which the schema would reject.
type analyticsInput struct {
	AppBlockID string `json:"appBlockId"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
}

// GetMyAppAnalytics calls the owner-only blocks.getMyAppAnalytics tRPC query for
// one App Block. The OAuth access token is refreshed transparently on a 401; a
// non-200 is mapped to an actionable error by analyticsError.
func (c *Client) GetMyAppAnalytics(ctx context.Context, appBlockID, from, to string) (*AppAnalytics, json.RawMessage, error) {
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, nil, err
	}
	if tok == "" {
		return nil, nil, civitai.Tag(civitai.ErrUnauthorized, fmt.Errorf("no token configured — run `civitai login` first"))
	}
	inputJSON, err := json.Marshal(map[string]any{
		"json": analyticsInput{AppBlockID: appBlockID, From: from, To: to},
	})
	if err != nil {
		return nil, nil, err
	}
	q := url.Values{}
	q.Set("input", string(inputJSON))
	reqURL := c.BaseURL + AppAnalyticsPath + "?" + q.Encode()

	build := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	}
	status, raw, err := c.authedDo(ctx, build)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, analyticsError(status, raw)
	}
	var env struct {
		Result struct {
			Data struct {
				JSON json.RawMessage `json:"json"`
			} `json:"data"`
		} `json:"result"`
	}
	// A literal `null` payload unmarshals CLEANLY into the zero struct, which
	// would render as an all-zero dashboard over an empty window — the exact
	// silent-empty failure this command exists to prevent. Reject it as malformed
	// alongside a missing/undecodable envelope.
	if err := json.Unmarshal(raw, &env); err != nil ||
		len(env.Result.Data.JSON) == 0 ||
		string(bytes.TrimSpace(env.Result.Data.JSON)) == "null" {
		return nil, nil, fmt.Errorf("unexpected blocks.getMyAppAnalytics response: %s", string(raw))
	}
	var out AppAnalytics
	if err := json.Unmarshal(env.Result.Data.JSON, &out); err != nil {
		return nil, nil, fmt.Errorf("unexpected blocks.getMyAppAnalytics payload: %s", string(env.Result.Data.JSON))
	}
	return &out, env.Result.Data.JSON, nil
}

// analyticsError maps a non-200 from the analytics tRPC query to an actionable
// message. tRPC error bodies are {error:{json:{message,code,...}}}.
//
// The 403 is the DX case worth spelling out: the proc declares no requiredScope,
// so it defaults to FULL scope — an OAuth device-login token (`civitai login`)
// is refused where a personal API key succeeds.
func analyticsError(status int, raw []byte) (err error) {
	defer func() { err = civitai.TagStatus(status, err) }()
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
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("not logged in (401): %s — run `civitai login` (or set CIVITAI_TOKEN)", msg)
	case http.StatusForbidden:
		return fmt.Errorf("not permitted to read this app's analytics (403): %s — this query needs a full-scope personal API key; "+
			"an OAuth `civitai login` token can't read it. Create a key at https://civitai.com/user/account, then run "+
			"`civitai login --token <key>` (check which credential is active with `civitai whoami`)", msg)
	case http.StatusNotFound:
		return fmt.Errorf("no such app for your account (404): %s — list your apps with `civitai app status`", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("rate limited, try again shortly (429): %s", msg)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("apps analytics unavailable (503): %s", msg)
	default:
		return fmt.Errorf("server returned %d: %s", status, strings.TrimSpace(msg))
	}
}
