package cmd

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// verifiedAnalyticsPayload is the real shape observed from
// blocks.getMyAppAnalytics on civitai.com (HTTP 200, unwrapped
// .result.data.json). Tests pin the RENDERED values against it rather than
// against whatever the renderer happens to produce.
const verifiedAnalyticsPayload = `{
  "range": {"from":"2026-05-01T00:00:00.000Z","to":"2026-08-03T00:00:00.000Z","granularity":"week"},
  "notOwned": false,
  "installs": {"total":3,"active":2,"series":[]},
  "runs": {"count":20,"buzzSpent":65,"series":[{"bucket":"2026-06-15T00:00:00.000Z","value":17}]},
  "buzzPurchased": {"count":1,"buzzAmount":500,"grossCents":499},
  "engagement": {"apiCalls":26,"activeUsers":2,"errorRate":0,
    "topScopes":[{"scope":"ai:write:budgeted","count":20}],
    "topEndpoints":[{"endpoint":"/api/v1/blocks/me","count":4}]}
}`

// metricsRec records what the fake server saw, so tests can assert the request
// the CLI actually made (auth header, resolved slug, tRPC input JSON).
type metricsRec struct {
	auth        string
	blockIDQ    string
	trpcInput   string
	trpcCalls   int
	subsCalls   int
	trpcReached bool
}

// metricsServer stands up an httptest server serving BOTH routes the command
// uses: GET /api/v1/blocks/submissions (slug → appBlockId) and the
// blocks.getMyAppAnalytics tRPC query. subsBody/trpcBody are raw response
// bodies; a zero status means 200.
func metricsServer(t *testing.T, subsBody string, subsStatus int, trpcBody string, trpcStatus int, rec *metricsRec) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rec != nil {
			rec.auth = r.Header.Get("Authorization")
		}
		switch r.URL.Path {
		case appapi.SubmissionsPath:
			if rec != nil {
				rec.subsCalls++
				rec.blockIDQ = r.URL.Query().Get("blockId")
			}
			if subsStatus != 0 && subsStatus != http.StatusOK {
				w.WriteHeader(subsStatus)
			}
			_, _ = w.Write([]byte(subsBody))
		case appapi.AppAnalyticsPath:
			if rec != nil {
				rec.trpcCalls++
				rec.trpcReached = true
				rec.trpcInput = r.URL.Query().Get("input")
			}
			if trpcStatus != 0 && trpcStatus != http.StatusOK {
				w.WriteHeader(trpcStatus)
			}
			_, _ = w.Write([]byte(trpcBody))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"unexpected path ` + r.URL.Path + `"}`))
		}
	}))
}

// submissionsBody is a one-row submissions list for slug, with the given
// appBlockId (pass nil for the never-approved, null case).
func submissionsBody(slug string, appBlockID *string) string {
	row := map[string]any{
		"id": "pubreq_1", "blockId": slug, "version": "0.1.0", "status": "approved",
		"submittedAt": "2026-06-20T08:00:00.000Z", "updatedAt": "2026-06-20T08:00:00.000Z",
		"createdAt": "2026-06-20T08:00:00.000Z", "appBlockId": appBlockID,
	}
	b, _ := json.Marshal(map[string]any{"submissions": []any{row}})
	return string(b)
}

// trpcEnvelope wraps a payload in the tRPC success envelope the server returns.
func trpcEnvelope(payload string) string {
	return `{"result":{"data":{"json":` + payload + `}}}`
}

func strPtr(s string) *string { return &s }

// setupMetricsEnv points the CLI at srv with a token configured.
func setupMetricsEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-metrics")
	t.Setenv("CIVITAI_BASE_URL", baseURL)
}

func TestAppMetricsRendersVerifiedPayload(t *testing.T) {
	var rec metricsRec
	srv := metricsServer(t,
		submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "my-block")
	if err != nil {
		t.Fatalf("app metrics: %v", err)
	}
	if rec.auth != "Bearer tok-metrics" {
		t.Errorf("auth header = %q", rec.auth)
	}
	if rec.blockIDQ != "my-block" {
		t.Errorf("submissions blockId query = %q, want my-block", rec.blockIDQ)
	}
	// The slug must be resolved to the appBlockId before the analytics call.
	if !strings.Contains(rec.trpcInput, `"appBlockId":"apb_123"`) {
		t.Errorf("tRPC input should carry the resolved appBlockId, got %q", rec.trpcInput)
	}
	for _, want := range []string{
		"my-block",
		"2026-05-01 00:00 UTC", "2026-08-03 00:00 UTC", "week", // requirement 1: the window is always printed
		"Installs", "Total", "3", "Active", "2",
		"Runs", "Count", "20", "Buzz spent", "65",
		"Buzz purchased", "Purchases", "1", "500", "$4.99",
		// errorRate is a 0–1 ratio server-side; an exact zero must read "0.0%",
		// never a bare "0" or a long float.
		"Engagement", "API calls", "26", "Active users", "Error rate", "0.0%",
		"Top scopes", "ai:write:budgeted",
		"Top endpoints", "/api/v1/blocks/me", "4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q:\n%s", want, out)
		}
	}
	// A non-zero engagement must NOT print the "flat engagement" caveat.
	if strings.Contains(out, "no scoped API surface") {
		t.Errorf("the flat-engagement note should not appear when apiCalls > 0:\n%s", out)
	}
}

func TestAppMetricsJSONPassthrough(t *testing.T) {
	srv := metricsServer(t,
		submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "my-block", "--json")
	if err != nil {
		t.Fatalf("app metrics --json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, out)
	}
	// The UNWRAPPED payload — not the tRPC envelope.
	if _, wrapped := got["result"]; wrapped {
		t.Errorf("--json should emit the unwrapped payload, got the tRPC envelope:\n%s", out)
	}
	runs, ok := got["runs"].(map[string]any)
	if !ok || runs["count"].(float64) != 20 || runs["buzzSpent"].(float64) != 65 {
		t.Errorf("--json payload lost the runs rollup:\n%s", out)
	}
	if got["notOwned"] != false {
		t.Errorf("--json payload should carry notOwned:\n%s", out)
	}
	// Human formatting must not leak into the scriptable path.
	if strings.Contains(out, "Buzz purchased") || strings.Contains(out, "UTC") {
		t.Errorf("--json emitted rendered text:\n%s", out)
	}
}

func TestAppMetricsNotOwnedRefusesDashboard(t *testing.T) {
	payload := `{"range":{"from":"2026-07-04T00:00:00.000Z","to":"2026-08-03T00:00:00.000Z","granularity":"day"},
	  "notOwned":true,
	  "installs":{"total":0,"active":0,"series":[]},
	  "runs":{"count":0,"buzzSpent":0,"series":[]},
	  "buzzPurchased":{"count":0,"buzzAmount":0,"grossCents":0},
	  "engagement":{"apiCalls":0,"activeUsers":0,"errorRate":0,"topScopes":[],"topEndpoints":[]}}`
	srv := metricsServer(t,
		submissionsBody("other-app", strPtr("apb_other")), http.StatusOK,
		trpcEnvelope(payload), http.StatusOK, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "other-app")
	if err == nil {
		t.Fatal("notOwned:true must not be reported as a successful read")
	}
	for _, want := range []string{"not owned", "civitai whoami", "civitai app status other-app"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("not-entitled error should contain %q, got: %v", want, err)
		}
	}
	// The whole point: a permission failure must not be dressed up as a
	// zeroed-but-real dashboard.
	for _, forbidden := range []string{"Installs", "Runs", "Buzz purchased", "Engagement", "Granularity"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("notOwned must not render the dashboard, but output contained %q:\n%s", forbidden, out)
		}
	}

	// --json is a raw passthrough, so a script still sees notOwned itself.
	jsonOut, _, err := run(t, "app", "metrics", "other-app", "--json")
	if err != nil {
		t.Fatalf("app metrics --json (notOwned): %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", err, jsonOut)
	}
	if got["notOwned"] != true {
		t.Errorf("--json should surface notOwned:true for scripts:\n%s", jsonOut)
	}
}

func TestAppMetricsGenuineZeroRendersWithWindow(t *testing.T) {
	payload := `{"range":{"from":"2026-07-04T00:00:00.000Z","to":"2026-08-03T00:00:00.000Z","granularity":"day"},
	  "notOwned":false,
	  "installs":{"total":0,"active":0,"series":[]},
	  "runs":{"count":0,"buzzSpent":0,"series":[]},
	  "buzzPurchased":{"count":0,"buzzAmount":0,"grossCents":0},
	  "engagement":{"apiCalls":0,"activeUsers":0,"errorRate":0,"topScopes":[],"topEndpoints":[]}}`
	srv := metricsServer(t,
		submissionsBody("quiet-app", strPtr("apb_quiet")), http.StatusOK,
		trpcEnvelope(payload), http.StatusOK, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "quiet-app")
	if err != nil {
		t.Fatalf("a genuine zero is a successful read: %v", err)
	}
	// A real zero renders — and the window it covers is printed with it, so the
	// zero is never ambiguous.
	for _, want := range []string{"quiet-app", "2026-07-04 00:00 UTC", "2026-08-03 00:00 UTC", "day",
		"Installs", "Runs", "Buzz purchased", "$0.00", "Engagement"} {
		if !strings.Contains(out, want) {
			t.Errorf("zero-result output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "no scoped API surface") {
		t.Errorf("a flat engagement should explain itself:\n%s", out)
	}
	if strings.Contains(out, "not owned") {
		t.Errorf("a genuine zero must not be reported as a permission problem:\n%s", out)
	}
}

func TestAppMetricsClampedRangeEchoesResponseNotFlags(t *testing.T) {
	var rec metricsRec
	// Requested 2020-01-01 → 2026-08-03; the server clamps to 366 days and says so.
	payload := `{"range":{"from":"2025-08-03T00:00:00.000Z","to":"2026-08-03T00:00:00.000Z","granularity":"week"},
	  "notOwned":false,
	  "installs":{"total":1,"active":1,"series":[]},
	  "runs":{"count":2,"buzzSpent":3,"series":[]},
	  "buzzPurchased":{"count":0,"buzzAmount":0,"grossCents":0},
	  "engagement":{"apiCalls":5,"activeUsers":1,"errorRate":0.25,"topScopes":[],"topEndpoints":[]}}`
	srv := metricsServer(t,
		submissionsBody("clamped", strPtr("apb_clamped")), http.StatusOK,
		trpcEnvelope(payload), http.StatusOK, &rec)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "clamped", "--from", "2020-01-01", "--to", "2026-08-03")
	if err != nil {
		t.Fatalf("app metrics --from/--to: %v", err)
	}
	// The request carried what the user asked for …
	if !strings.Contains(rec.trpcInput, `"from":"2020-01-01T00:00:00.000Z"`) {
		t.Errorf("requested from should be sent verbatim, got input %q", rec.trpcInput)
	}
	// … but the PRINTED window must follow the response, never the flags.
	if !strings.Contains(out, "2025-08-03 00:00 UTC") {
		t.Errorf("printed window must echo the server's clamped range:\n%s", out)
	}
	if strings.Contains(out, "2020-01-01") {
		t.Errorf("printed window must not echo the requested (unclamped) start:\n%s", out)
	}
	// errorRate is a ratio (errorCount/apiCalls) server-side, so 0.25 is 25%.
	if !strings.Contains(out, "25.0%") {
		t.Errorf("error rate 0.25 should render as 25.0%%:\n%s", out)
	}
	if strings.Contains(out, "0.25") {
		t.Errorf("the human view must not print the raw ratio:\n%s", out)
	}
}

func TestAppMetricsWindowFlagsAccepted(t *testing.T) {
	cases := []struct {
		name       string
		from, to   string
		wantInWire []string
	}{
		{"bare dates", "2026-05-01", "2026-08-03",
			[]string{`"from":"2026-05-01T00:00:00.000Z"`, `"to":"2026-08-03T00:00:00.000Z"`}},
		{"rfc3339", "2026-05-01T06:30:00Z", "2026-08-03T12:00:00Z",
			[]string{`"from":"2026-05-01T06:30:00.000Z"`, `"to":"2026-08-03T12:00:00.000Z"`}},
		{"rfc3339 with offset normalises to UTC", "2026-05-01T02:00:00+02:00", "",
			[]string{`"from":"2026-05-01T00:00:00.000Z"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec metricsRec
			srv := metricsServer(t,
				submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
				trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
			defer srv.Close()
			setupMetricsEnv(t, srv.URL)

			args := []string{"app", "metrics", "my-block", "--from", tc.from}
			if tc.to != "" {
				args = append(args, "--to", tc.to)
			}
			if _, _, err := run(t, args...); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			for _, want := range tc.wantInWire {
				if !strings.Contains(rec.trpcInput, want) {
					t.Errorf("wire input %q missing %q", rec.trpcInput, want)
				}
			}
			if tc.to == "" && strings.Contains(rec.trpcInput, `"to"`) {
				t.Errorf("an unset --to must be omitted from the wire so the server default applies: %q", rec.trpcInput)
			}
		})
	}
}

func TestAppMetricsNoWindowFlagsOmitsBothBounds(t *testing.T) {
	var rec metricsRec
	srv := metricsServer(t,
		submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	if _, _, err := run(t, "app", "metrics", "my-block"); err != nil {
		t.Fatalf("app metrics: %v", err)
	}
	if strings.Contains(rec.trpcInput, `"from"`) || strings.Contains(rec.trpcInput, `"to"`) {
		t.Errorf("with no flags both bounds must be absent (server 30-day default): %q", rec.trpcInput)
	}
}

func TestAppMetricsBadWindowIsUsageError(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantMsg  string
		wantWire bool
	}{
		{"garbage --from", []string{"--from", "last-tuesday"}, `invalid --from "last-tuesday"`, false},
		{"garbage --to", []string{"--to", "2026-13-45"}, `invalid --to "2026-13-45"`, false},
		{"from after to", []string{"--from", "2026-08-03", "--to", "2026-05-01"}, "is after --to", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec metricsRec
			srv := metricsServer(t,
				submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
				trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
			defer srv.Close()
			setupMetricsEnv(t, srv.URL)

			args := append([]string{"app", "metrics", "my-block"}, tc.args...)
			_, _, err := run(t, args...)
			if err == nil {
				t.Fatalf("%s: expected an error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("%s: error %q should contain %q", tc.name, err.Error(), tc.wantMsg)
			}
			// exit 2 (the entrypoint maps ErrUsage → exitUsage).
			if !errors.Is(err, ErrUsage) {
				t.Errorf("%s: a bad window is a usage error (exit 2), got %T: %v", tc.name, err, err)
			}
			// It is decided client-side: no request should have been made at all.
			if rec.subsCalls != 0 || rec.trpcCalls != 0 {
				t.Errorf("%s: a usage error must be caught before any network call (subs=%d trpc=%d)",
					tc.name, rec.subsCalls, rec.trpcCalls)
			}
		})
	}
}

func TestAppMetricsMissingTokenErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	_, _, err := run(t, "app", "metrics", "my-block")
	if err == nil {
		t.Fatal("expected error with no token")
	}
	if !strings.Contains(err.Error(), "no token") || !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("missing-token error should point to login: %v", err)
	}
}

func TestAppMetricsUnauthorizedPointsAtLogin(t *testing.T) {
	srv := metricsServer(t,
		submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
		`{"error":{"json":{"message":"Invalid API key"}}}`, http.StatusUnauthorized, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	_, _, err := run(t, "app", "metrics", "my-block")
	if err == nil {
		t.Fatal("expected an error on 401")
	}
	if !strings.Contains(err.Error(), "not logged in (401)") || !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("401 should point at login, got: %v", err)
	}
	// exit 3 (root's taxonomy maps ErrUnauthorized → exitAuth). Pinned
	// separately from the message: an unclassified error keeps the same text
	// while silently degrading the exit code to 1.
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("a 401 must classify as ErrUnauthorized (exit 3), got %T: %v", err, err)
	}
}

func TestAppMetricsForbiddenAsksForPersonalAPIKey(t *testing.T) {
	srv := metricsServer(t,
		submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
		`{"error":{"json":{"message":"Your API key does not have the required scope for this action"}}}`,
		http.StatusForbidden, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	_, _, err := run(t, "app", "metrics", "my-block")
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	// The whole value of this branch is naming the fix, not echoing "forbidden".
	for _, want := range []string{"403", "personal API key", "civitai login --token", "OAuth"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("403 error should contain %q, got: %v", want, err)
		}
	}
	// exit 3 (root's taxonomy maps ErrUnauthorized → exitAuth). Dropping the
	// classification leaves every character of the message above intact, so
	// nothing but this assertion notices.
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("a 403 must classify as ErrUnauthorized (exit 3), got %T: %v", err, err)
	}
}

func TestAppMetricsUnknownSlugIsActionableNotFound(t *testing.T) {
	var rec metricsRec
	srv := metricsServer(t,
		`{"submissions":[]}`, http.StatusOK,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	_, _, err := run(t, "app", "metrics", "ghost-app")
	if err == nil {
		t.Fatal("expected an error for a slug with no submissions")
	}
	for _, want := range []string{`no submissions found for app "ghost-app"`, "civitai app status"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("unknown-slug error should contain %q, got: %v", want, err)
		}
	}
	// exit 4 (root's taxonomy maps ErrNotFound → exitNotFound). The message is
	// unchanged by dropping the tag, so only this pins the contract.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("an unknown slug must classify as ErrNotFound (exit 4), got %T: %v", err, err)
	}
	if rec.trpcReached {
		t.Error("an unresolvable slug must not reach the analytics query")
	}
}

// TestAppMetricsServer404ClassifiesNotFound covers the OTHER not-found path: the
// analytics query itself answering 404 after the slug resolved. That one is
// tagged by analyticsError's TagStatus deferral rather than an explicit Tag, so
// it needs its own pin.
func TestAppMetricsServer404ClassifiesNotFound(t *testing.T) {
	srv := metricsServer(t,
		submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
		`{"error":{"json":{"message":"No such app block"}}}`, http.StatusNotFound, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	_, _, err := run(t, "app", "metrics", "my-block")
	if err == nil {
		t.Fatal("expected an error on 404")
	}
	if !strings.Contains(err.Error(), "no such app for your account (404)") ||
		!strings.Contains(err.Error(), "civitai app status") {
		t.Errorf("404 should name the next command, got: %v", err)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("a 404 must classify as ErrNotFound (exit 4), got %T: %v", err, err)
	}
}

// TestAppMetricsErrorRateRendersAsPercent pins the unit of engagement.errorRate.
// The server computes it as errorCount/apiCalls — a 0–1 ratio — so the human
// view must rescale it while --json keeps passing the raw ratio through. The
// value here is the one observed live for playable-collections (5 errors in 245
// calls).
func TestAppMetricsErrorRateRendersAsPercent(t *testing.T) {
	payload := `{"range":{"from":"2026-05-01T00:00:00.000Z","to":"2026-08-03T00:00:00.000Z","granularity":"week"},
	  "notOwned":false,
	  "installs":{"total":0,"active":0,"series":[]},
	  "runs":{"count":20,"buzzSpent":65,"series":[]},
	  "buzzPurchased":{"count":0,"buzzAmount":0,"grossCents":0},
	  "engagement":{"apiCalls":245,"activeUsers":3,"errorRate":0.02040816326530612,
	    "topScopes":[{"scope":"collections:read:self","count":170}],
	    "topEndpoints":[{"endpoint":"/api/v1/blocks/collections/:id","count":113}]}}`
	srv := metricsServer(t,
		submissionsBody("playable-collections", strPtr("apb_pc")), http.StatusOK,
		trpcEnvelope(payload), http.StatusOK, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "playable-collections")
	if err != nil {
		t.Fatalf("app metrics: %v", err)
	}
	if !strings.Contains(out, "Error rate") || !strings.Contains(out, "2.0%") {
		t.Errorf("errorRate 0.02040816326530612 should render as 2.0%%:\n%s", out)
	}
	// The raw ratio is unreadable in a dashboard — it must not leak through.
	if strings.Contains(out, "0.02040816326530612") {
		t.Errorf("the human view must not print the raw ratio:\n%s", out)
	}

	// --json is a verbatim passthrough: the server's own ratio, not a percentage.
	jsonOut, _, err := run(t, "app", "metrics", "playable-collections", "--json")
	if err != nil {
		t.Fatalf("app metrics --json: %v", err)
	}
	if !strings.Contains(jsonOut, "0.02040816326530612") {
		t.Errorf("--json must pass the server's raw errorRate through:\n%s", jsonOut)
	}
	if strings.Contains(jsonOut, "%") {
		t.Errorf("--json must not rescale errorRate into a percentage:\n%s", jsonOut)
	}
}

func TestPctFromRatio(t *testing.T) {
	cases := []struct {
		name  string
		ratio float64
		want  string
	}{
		{"exact zero stays 0.0%", 0, "0.0%"},
		{"observed live rate", 0.02040816326530612, "2.0%"},
		{"quarter", 0.25, "25.0%"},
		{"half a percent", 0.005, "0.5%"},
		{"everything", 1, "100.0%"},
		// The sub-0.0005 band: one decimal place alone renders all of these
		// "0.0%", which is indistinguishable from a genuine zero.
		{"5000 calls, 2 errors", 0.0004, "<0.1%"},
		{"one in a hundred thousand", 1e-5, "<0.1%"},
		{"smallest representable positive", math.SmallestNonzeroFloat64, "<0.1%"},
		// Boundary, both sides. 0.0005 is the first ratio that rounds UP to
		// 0.1%, so the largest float64 strictly below it is the last member of
		// the small-but-nonzero band.
		{"just below the 0.0005 boundary", math.Nextafter(0.0005, 0), "<0.1%"},
		{"exactly the 0.0005 boundary", 0.0005, "0.1%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pctFromRatio(tc.ratio); got != tc.want {
				t.Errorf("pctFromRatio(%v) = %q, want %q", tc.ratio, got, tc.want)
			}
		})
	}
}

// TestPctFromRatioTinyRateIsNotZero is the discriminating test: it does not care
// what the small-but-nonzero rendering IS, only that a real error rate can never
// print the same string as no errors at all. Reverting to a bare
// FormatFloat(r*100, 'f', 1, 64) makes every case here fail.
func TestPctFromRatioTinyRateIsNotZero(t *testing.T) {
	zero := pctFromRatio(0)
	if zero != "0.0%" {
		t.Fatalf("an exact zero must render 0.0%%, got %q", zero)
	}
	// 5,000 API calls with 2 errors — a healthy, high-traffic app, i.e. exactly
	// the population that lands in this band.
	for _, r := range []float64{0.0004, 1e-5, math.Nextafter(0.0005, 0), math.SmallestNonzeroFloat64} {
		got := pctFromRatio(r)
		if got == zero {
			t.Errorf("pctFromRatio(%v) = %q — a nonzero error rate must not render identically to zero (%q)", r, got, zero)
		}
	}
}

// TestAppMetricsTinyErrorRateRendersDistinctly walks the same regression through
// the whole command: the dashboard must show the app HAS errors, while --json
// keeps passing the server's raw ratio through.
func TestAppMetricsTinyErrorRateRendersDistinctly(t *testing.T) {
	// 5,000 authenticated calls, 2 of them 4xx/5xx → errorRate 0.0004.
	payload := `{"range":{"from":"2026-05-01T00:00:00.000Z","to":"2026-08-03T00:00:00.000Z","granularity":"week"},
	  "notOwned":false,
	  "installs":{"total":0,"active":0,"series":[]},
	  "runs":{"count":0,"buzzSpent":0,"series":[]},
	  "buzzPurchased":{"count":0,"buzzAmount":0,"grossCents":0},
	  "engagement":{"apiCalls":5000,"activeUsers":40,"errorRate":0.0004,"topScopes":[],"topEndpoints":[]}}`
	srv := metricsServer(t,
		submissionsBody("busy-app", strPtr("apb_busy")), http.StatusOK,
		trpcEnvelope(payload), http.StatusOK, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	out, _, err := run(t, "app", "metrics", "busy-app")
	if err != nil {
		t.Fatalf("app metrics: %v", err)
	}
	if !strings.Contains(out, "Error rate") || !strings.Contains(out, "<0.1%") {
		t.Errorf("errorRate 0.0004 should render as <0.1%%:\n%s", out)
	}
	if strings.Contains(out, "Error rate    0.0%") || regexp.MustCompile(`Error rate\s+0\.0%`).MatchString(out) {
		t.Errorf("an app WITH errors must not read 0.0%%:\n%s", out)
	}

	jsonOut, _, err := run(t, "app", "metrics", "busy-app", "--json")
	if err != nil {
		t.Fatalf("app metrics --json: %v", err)
	}
	if !strings.Contains(jsonOut, "0.0004") {
		t.Errorf("--json must pass the server's raw errorRate through:\n%s", jsonOut)
	}
	if strings.Contains(jsonOut, "0.1%") || strings.Contains(jsonOut, "<") {
		t.Errorf("--json must not apply the human small-rate rendering:\n%s", jsonOut)
	}
}

func TestAppMetricsNoApprovedBlockYet(t *testing.T) {
	var rec metricsRec
	srv := metricsServer(t,
		submissionsBody("pending-app", nil), http.StatusOK,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	_, _, err := run(t, "app", "metrics", "pending-app")
	if err == nil {
		t.Fatal("expected an error when every submission has a null appBlockId")
	}
	// Must be the approval-specific message, NOT the generic not-found one.
	for _, want := range []string{"no approved App Block yet", "civitai app status pending-app"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("null-appBlockId error should contain %q, got: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "no submissions found") {
		t.Errorf("a null appBlockId is distinct from a missing app, got: %v", err)
	}
	if rec.trpcReached {
		t.Error("a null appBlockId must not reach the analytics query")
	}
}

func TestAppMetricsPicksNewestNonNullAppBlockID(t *testing.T) {
	var rec metricsRec
	// Newest-first list whose newest row is an un-approved (null) resubmission —
	// the resolver must fall through to the approved one rather than giving up.
	subs, _ := json.Marshal(map[string]any{"submissions": []any{
		map[string]any{"id": "pubreq_2", "blockId": "mixed", "version": "0.2.0", "status": "pending",
			"submittedAt": "2026-06-21T08:00:00.000Z", "appBlockId": nil},
		map[string]any{"id": "pubreq_1", "blockId": "mixed", "version": "0.1.0", "status": "approved",
			"submittedAt": "2026-06-20T08:00:00.000Z", "appBlockId": "apb_mixed"},
	}})
	srv := metricsServer(t, string(subs), http.StatusOK,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, &rec)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	if _, _, err := run(t, "app", "metrics", "mixed"); err != nil {
		t.Fatalf("app metrics (mixed submissions): %v", err)
	}
	if !strings.Contains(rec.trpcInput, `"appBlockId":"apb_mixed"`) {
		t.Errorf("resolver should use the approved row's appBlockId, got input %q", rec.trpcInput)
	}
}

func TestAppMetricsMalformedEnvelopeIsCleanError(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"not json", `<html>502 Bad Gateway</html>`},
		{"envelope without data", `{"result":{}}`},
		{"null payload", `{"result":{"data":{"json":null}}}`},
		{"payload of the wrong type", `{"result":{"data":{"json":"nope"}}}`},
		{"empty body", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := metricsServer(t,
				submissionsBody("my-block", strPtr("apb_123")), http.StatusOK,
				tc.body, http.StatusOK, nil)
			defer srv.Close()
			setupMetricsEnv(t, srv.URL)

			out, _, err := run(t, "app", "metrics", "my-block")
			if err == nil {
				t.Fatalf("%s: expected an error, got output:\n%s", tc.name, err)
			}
			if !strings.Contains(err.Error(), "getMyAppAnalytics") {
				t.Errorf("%s: error should name the failing call, got: %v", tc.name, err)
			}
			if strings.Contains(out, "Installs") {
				t.Errorf("%s: nothing should be rendered from a malformed response:\n%s", tc.name, out)
			}
		})
	}
}

func TestAppMetricsSubmissionsErrorSurfaces(t *testing.T) {
	srv := metricsServer(t,
		`{"message":"Apps are not enabled"}`, http.StatusServiceUnavailable,
		trpcEnvelope(verifiedAnalyticsPayload), http.StatusOK, nil)
	defer srv.Close()
	setupMetricsEnv(t, srv.URL)

	_, _, err := run(t, "app", "metrics", "my-block")
	if err == nil {
		t.Fatal("expected the resolution failure to surface")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("resolution error should surface the server message, got: %v", err)
	}
}

func TestAppHelpListsMetrics(t *testing.T) {
	out, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatalf("app --help: %v", err)
	}
	if !strings.Contains(out, "metrics") {
		t.Errorf("app help should list the metrics subcommand:\n%s", out)
	}
}

func TestAppMetricsHelpDocumentsTheCaveats(t *testing.T) {
	out, _, err := run(t, "app", "metrics", "--help")
	if err != nil {
		t.Fatalf("app metrics --help: %v", err)
	}
	lower := strings.ToLower(out)
	for _, want := range []string{"--from", "--to", "--json",
		"authenticated", "scope-gated", "personal api key",
		// --json deliberately fails OPEN on a not-entitled read (raw passthrough,
		// exit 0), so the help has to say scripts must branch on notOwned.
		"notowned"} {
		if !strings.Contains(lower, want) {
			t.Errorf("metrics help missing %q:\n%s", want, out)
		}
	}
}

// TestAppMetricsJSONFlagRendersAsABoolean guards a real trap hit while writing
// the notOwned caveat: pflag's UnquoteUsage lifts the first BACK-QUOTED span out
// of a usage string and uses it as the flag's value name, so a usage string
// mentioning a `notOwned: true` payload renders the boolean as
// "--json notOwned: true" — i.e. the help tells you to pass an argument to a
// flag that takes none.
func TestAppMetricsJSONFlagRendersAsABoolean(t *testing.T) {
	out, _, err := run(t, "app", "metrics", "--help")
	if err != nil {
		t.Fatalf("app metrics --help: %v", err)
	}
	// A boolean flag's help line is "--json" then padding then the description.
	if !regexp.MustCompile(`--json\s{2,}emit`).MatchString(out) {
		t.Errorf("--json should render as a valueless boolean flag:\n%s", out)
	}
	if regexp.MustCompile(`--json\s+notOwned`).MatchString(out) {
		t.Errorf("--json picked up a value name from a back-quoted usage span:\n%s", out)
	}
}

func TestUSDFromCents(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{{0, "$0.00"}, {5, "$0.05"}, {65, "$0.65"}, {499, "$4.99"}, {100000, "$1000.00"}, {-250, "-$2.50"}}
	for _, tc := range cases {
		if got := usdFromCents(tc.cents); got != tc.want {
			t.Errorf("usdFromCents(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func TestUTCStamp(t *testing.T) {
	if got := utcStamp("2026-05-01T02:00:00+02:00"); got != "2026-05-01 00:00 UTC" {
		t.Errorf("utcStamp should normalise to UTC, got %q", got)
	}
	// Unparseable server data is shown, never hidden.
	if got := utcStamp("not-a-time"); got != "not-a-time" {
		t.Errorf("utcStamp should fall back to the raw value, got %q", got)
	}
	if got := utcStamp(""); got != "-" {
		t.Errorf("utcStamp(\"\") = %q, want %q", got, "-")
	}
}
