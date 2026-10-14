package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// Every expectation here is a LITERAL written down from the platform-feedback
// contract — a request body, a query input, a field name — and none is computed
// from the code under test. The server did not exist when this was written, so
// these literals ARE the contract as this CLI understood it: a later server
// that disagrees should make exactly these lines fail.

// pfCall is one request the fake received.
type pfCall struct {
	method, path, input, body, contentType, auth string
}

// pfServer is a fake Civitai that records every request and answers with the
// reply a test installs.
type pfServer struct {
	mu    sync.Mutex
	calls []pfCall
	reply func(c pfCall) (int, string)
}

func (s *pfServer) start(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c := pfCall{
			method: r.Method, path: r.URL.Path, input: r.URL.Query().Get("input"),
			body: string(raw), contentType: r.Header.Get("Content-Type"), auth: r.Header.Get("Authorization"),
		}
		s.mu.Lock()
		s.calls = append(s.calls, c)
		reply := s.reply
		s.mu.Unlock()
		status, body := http.StatusInternalServerError, `{}`
		if reply != nil {
			status, body = reply(c)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok-platform-feedback", "")
}

func (s *pfServer) seen() []pfCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pfCall(nil), s.calls...)
}

func pfEnvelope(payload string) string { return `{"result":{"data":{"json":` + payload + `}}}` }

func pfTRPCError(message string) string {
	b, _ := json.Marshal(map[string]any{"error": map[string]any{"json": map[string]any{"message": message}}})
	return string(b)
}

func intPtr(n int) *int { return &n }

// pfFullInput sets EVERY field of the contract, each to a distinct value, so the
// body literal below pins every wire name and no two fields can be swapped
// without the test noticing.
func pfFullInput() PlatformFeedbackCreateInput {
	return PlatformFeedbackCreateInput{
		Message: "submit was refused with a 500 and no message",
		Report: PlatformFeedbackReport{
			About:        "review",
			Kind:         "confusing",
			Ref:          "submit.server-refused",
			Client:       PlatformFeedbackClient{Name: "civitai-cli", Version: "0.1.96"},
			Agent:        "codex",
			OS:           "darwin",
			Arch:         "arm64",
			Command:      "civitai app submit --yes",
			ExitCode:     intPtr(1),
			HTTPStatus:   intPtr(500),
			SDK:          []PlatformFeedbackSDK{{Name: "@civitai/app-sdk", Version: "^0.69.0"}},
			DocURL:       "https://developer.civitai.com/apps/submit",
			ErrorExcerpt: "Error: server returned 500",
			AppSlug:      "tide-chart",
		},
	}
}

const pfFullBody = `{"json":{"message":"submit was refused with a 500 and no message","report":{` +
	`"about":"review","kind":"confusing","ref":"submit.server-refused",` +
	`"client":{"name":"civitai-cli","version":"0.1.96"},"agent":"codex","os":"darwin","arch":"arm64",` +
	`"command":"civitai app submit --yes","exitCode":1,"httpStatus":500,` +
	`"sdk":[{"name":"@civitai/app-sdk","version":"^0.69.0"}],` +
	`"docUrl":"https://developer.civitai.com/apps/submit","errorExcerpt":"Error: server returned 500","appSlug":"tide-chart"}}}`

func TestCreatePlatformFeedbackSendsTheContractBody(t *testing.T) {
	s := &pfServer{reply: func(pfCall) (int, string) {
		return http.StatusOK, pfEnvelope(`{"ref":"5123-3f9a01bc","duplicate":true,"status":"acknowledged",` +
			`"fixedIn":{"component":"civitai-cli","version":"0.1.97"},"note":"Known; a fix is in review.","reporters":14,"reports":22}`)
	}}
	c := s.start(t)

	res, err := c.CreatePlatformFeedback(context.Background(), pfFullInput())
	if err != nil {
		t.Fatalf("CreatePlatformFeedback: %v", err)
	}
	calls := s.seen()
	if len(calls) != 1 {
		t.Fatalf("want exactly 1 request, got %d", len(calls))
	}
	got := calls[0]
	if got.method != http.MethodPost || got.path != "/api/trpc/platformFeedback.create" {
		t.Errorf("request = %s %s, want POST /api/trpc/platformFeedback.create", got.method, got.path)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	if got.auth != "Bearer tok-platform-feedback" {
		t.Errorf("Authorization = %q, want the user's existing token as a bearer", got.auth)
	}
	if got.body != pfFullBody {
		t.Errorf("request body:\n got: %s\nwant: %s", got.body, pfFullBody)
	}

	if res.Ref != "5123-3f9a01bc" || !res.Duplicate || res.Status != "acknowledged" || res.Reporters != 14 || res.Reports != 22 {
		t.Errorf("decoded result = %+v", res)
	}
	if res.FixedIn == nil || res.FixedIn.Component != "civitai-cli" || res.FixedIn.Version != "0.1.97" {
		t.Errorf("fixedIn = %+v, want {civitai-cli 0.1.97}", res.FixedIn)
	}
	if res.Note == nil || *res.Note != "Known; a fix is in review." {
		t.Errorf("note = %v", res.Note)
	}
}

// TestCreatePlatformFeedbackOmitsUnsetOptionalFields: an unset optional field
// must be ABSENT, not a zero value the server's schema would reject
// (`httpStatus: 0`, `ref: ""`).
func TestCreatePlatformFeedbackOmitsUnsetOptionalFields(t *testing.T) {
	s := &pfServer{reply: func(pfCall) (int, string) {
		return http.StatusOK, pfEnvelope(`{"ref":"6001-0a1b2c3d","duplicate":false,"status":"received","fixedIn":null,"note":null,"reporters":1,"reports":1}`)
	}}
	c := s.start(t)
	in := PlatformFeedbackCreateInput{
		Message: "the docs page 404s",
		Report: PlatformFeedbackReport{
			About: "docs", Kind: "bug",
			Client: PlatformFeedbackClient{Name: "civitai-cli", Version: "dev"},
		},
	}
	res, err := c.CreatePlatformFeedback(context.Background(), in)
	if err != nil {
		t.Fatalf("CreatePlatformFeedback: %v", err)
	}
	const want = `{"json":{"message":"the docs page 404s","report":{"about":"docs","kind":"bug","client":{"name":"civitai-cli","version":"dev"}}}}`
	if got := s.seen()[0].body; got != want {
		t.Errorf("request body:\n got: %s\nwant: %s", got, want)
	}
	if res.FixedIn != nil || res.Note != nil || res.Duplicate {
		t.Errorf("null fixedIn/note must decode to nil and duplicate to false; got %+v", res)
	}
}

func TestGetPlatformFeedbackStatusSendsTheContractQuery(t *testing.T) {
	s := &pfServer{reply: func(pfCall) (int, string) {
		return http.StatusOK, pfEnvelope(`[` +
			`{"ref":"5123-3f9a01bc","found":true,"status":"fixed","fixedIn":{"component":"@civitai/app-sdk","version":"0.70.1"},` +
			`"note":null,"reporters":9,"firstSeenAt":"2026-10-01T08:00:00.000Z","updatedAt":"2026-10-11T16:30:00.000Z"},` +
			`{"ref":"7777-deadbeef","found":false,"status":null,"fixedIn":null,"note":null,"reporters":0,"firstSeenAt":null,"updatedAt":null}]`)
	}}
	c := s.start(t)

	items, err := c.GetPlatformFeedbackStatus(context.Background(), []string{"5123-3f9a01bc", "7777-deadbeef"})
	if err != nil {
		t.Fatalf("GetPlatformFeedbackStatus: %v", err)
	}
	calls := s.seen()
	if len(calls) != 1 {
		t.Fatalf("want exactly 1 request, got %d", len(calls))
	}
	if calls[0].method != http.MethodGet || calls[0].path != "/api/trpc/platformFeedback.getStatus" {
		t.Errorf("request = %s %s, want GET /api/trpc/platformFeedback.getStatus", calls[0].method, calls[0].path)
	}
	if want := `{"json":{"refs":["5123-3f9a01bc","7777-deadbeef"]}}`; calls[0].input != want {
		t.Errorf("?input= %s, want %s", calls[0].input, want)
	}
	if calls[0].body != "" {
		t.Errorf("a tRPC query carries no body, got %q", calls[0].body)
	}
	if len(items) != 2 {
		t.Fatalf("decoded %d item(s), want 2", len(items))
	}
	a, b := items[0], items[1]
	if !a.Found || a.Status != "fixed" || a.Reporters != 9 || a.FixedIn == nil || a.FixedIn.Version != "0.70.1" ||
		a.FirstSeenAt == nil || *a.FirstSeenAt != "2026-10-01T08:00:00.000Z" || a.UpdatedAt == nil || *a.UpdatedAt != "2026-10-11T16:30:00.000Z" {
		t.Errorf("first item = %+v", a)
	}
	if b.Found || b.Status != "" || b.FixedIn != nil || b.FirstSeenAt != nil || b.UpdatedAt != nil {
		t.Errorf("a not-found item must decode its nulls to zero values; got %+v", b)
	}
}

func TestGetPlatformFeedbackStatusBoundsTheRefCount(t *testing.T) {
	s := &pfServer{}
	c := s.start(t)
	for _, n := range []int{0, PlatformFeedbackStatusRefsMax + 1} {
		refs := make([]string, n)
		for i := range refs {
			refs[i] = "1000-0000000" + string(rune('a'+i%26))
		}
		_, err := c.GetPlatformFeedbackStatus(context.Background(), refs)
		var fe *PlatformFeedbackFieldError
		if !errors.As(err, &fe) || fe.Field != "refs" {
			t.Errorf("%d refs: err = %v, want a refs field error", n, err)
		}
	}
	if n := len(s.seen()); n != 0 {
		t.Errorf("an out-of-range ref count must be refused before the wire; the server saw %d request(s)", n)
	}
}

// TestPlatformFeedbackErrorClassification pins every arm of the status mapper
// by errors.Is against the classification sentinels (AGENTS.md item 7) — the
// thing cmd/civitai's exitCode reads — never by message text alone.
func TestPlatformFeedbackErrorClassification(t *testing.T) {
	kinds := map[string]error{
		"ErrUnauthorized": civitai.ErrUnauthorized,
		"ErrNotFound":     civitai.ErrNotFound,
		"ErrBadRequest":   civitai.ErrBadRequest,
		"ErrRateLimited":  civitai.ErrRateLimited,
		"ErrNetwork":      civitai.ErrNetwork,
	}
	cases := []struct {
		name    string
		status  int
		body    string
		kind    string // "" = must match NONE of the kinds (exit 1)
		off     bool   // must be a *PlatformFeedbackOffError
		wantMsg string
	}{
		{"401", http.StatusUnauthorized, pfTRPCError("UNAUTHORIZED"), "ErrUnauthorized", false, "not logged in (401)"},
		{"403 scope refusal", http.StatusForbidden,
			pfTRPCError("Your API key does not have the required scope for this action"),
			"ErrUnauthorized", false, "scope does not cover platform feedback (403)"},
		{"403 collection off", http.StatusForbidden,
			pfTRPCError("Feedback is not being collected here right now."),
			"", true, "Civitai is not collecting platform feedback right now"},
		{"403 anything else", http.StatusForbidden, pfTRPCError("Your account is restricted."),
			"ErrUnauthorized", false, "the server refused this platform-feedback request (403): Your account is restricted."},
		{"404 no such procedure", http.StatusNotFound,
			pfTRPCError(`No "mutation"-procedure on path "platformFeedback.create"`),
			"ErrNotFound", false, "this server does not offer platform feedback (404)"},
		{"429", http.StatusTooManyRequests, pfTRPCError("Too many requests"), "ErrRateLimited", false, "rate limited (429): Too many requests"},
		{"400", http.StatusBadRequest, pfTRPCError("report.errorExcerpt: too long"), "ErrBadRequest", false, "rejected this platform-feedback request (400)"},
		{"503", http.StatusServiceUnavailable, `{"message":"maintenance"}`, "ErrNetwork", false, "platform feedback unavailable (503): maintenance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := platformFeedbackError(tc.status, []byte(tc.body))
			if err == nil {
				t.Fatal("expected an error")
			}
			for name, kind := range kinds {
				if got, want := errors.Is(err, kind), name == tc.kind; got != want {
					t.Errorf("errors.Is(err, civitai.%s) = %v, want %v — that moves the exit code. err: %v", name, got, want, err)
				}
			}
			var off *PlatformFeedbackOffError
			if got := errors.As(err, &off); got != tc.off {
				t.Errorf("is a PlatformFeedbackOffError = %v, want %v. err: %v", got, tc.off, err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("message does not contain %q:\n%s", tc.wantMsg, err)
			}
		})
	}
}

// TestCollectionOffIsNarrow: only the server's own collection-off sentence makes
// a 403 "collection is off". Anything else keeps its 403 classification, and a
// SCOPE refusal is never re-read as collection-off even if both phrases appear.
func TestCollectionOffIsNarrow(t *testing.T) {
	var off *PlatformFeedbackOffError
	for _, msg := range []string{"", "Forbidden", "You do not have permission", "Feedback closed"} {
		if errors.As(platformFeedbackError(http.StatusForbidden, []byte(pfTRPCError(msg))), &off) {
			t.Errorf("a 403 saying %q was reported as collection-off", msg)
		}
	}
	both := "Your API key does not have the required scope for this action; feedback is not being collected"
	if errors.As(platformFeedbackError(http.StatusForbidden, []byte(pfTRPCError(both))), &off) {
		t.Error("a scope refusal was reported as collection-off")
	}
	// Positive control: the sentence the contract gives DOES take the arm, in
	// any letter case.
	if !errors.As(platformFeedbackError(http.StatusForbidden, []byte(pfTRPCError("FEEDBACK IS NOT BEING COLLECTED HERE RIGHT NOW."))), &off) {
		t.Fatal("the contract's collection-off sentence did not take the collection-off arm")
	}
	if off.ServerMsg != "FEEDBACK IS NOT BEING COLLECTED HERE RIGHT NOW." {
		t.Errorf("the server's words must be carried verbatim, got %q", off.ServerMsg)
	}
}

func TestPlatformFeedbackNullPayloadIsMalformedNotEmpty(t *testing.T) {
	s := &pfServer{reply: func(pfCall) (int, string) { return http.StatusOK, pfEnvelope(`null`) }}
	c := s.start(t)
	if _, err := c.CreatePlatformFeedback(context.Background(), pfFullInput()); err == nil ||
		!strings.Contains(err.Error(), "unexpected platformFeedback.create response") {
		t.Errorf("a null create payload must be an error naming the procedure, got %v", err)
	}
	if _, err := c.GetPlatformFeedbackStatus(context.Background(), []string{"5123-3f9a01bc"}); err == nil ||
		!strings.Contains(err.Error(), "unexpected platformFeedback.getStatus response") {
		t.Errorf("a null getStatus payload must be an error, not an empty list; got %v", err)
	}
}

// boundCase mutates a valid input so that exactly one field is at its bound
// (ok) or one past it (bad), and names the wire field Validate must blame.
type boundCase struct {
	field string
	set   func(in *PlatformFeedbackCreateInput, n int)
	max   int
}

func rep(n int) string { return strings.Repeat("k", n) }

var pfBoundCases = []boundCase{
	{"message", func(in *PlatformFeedbackCreateInput, n int) { in.Message = rep(n) }, PlatformFeedbackMessageMax},
	{"report.ref", func(in *PlatformFeedbackCreateInput, n int) { in.Report.Ref = rep(n) }, PlatformFeedbackRefMax},
	{"report.client.version", func(in *PlatformFeedbackCreateInput, n int) { in.Report.Client.Version = rep(n) }, PlatformFeedbackVersionMax},
	{"report.agent", func(in *PlatformFeedbackCreateInput, n int) { in.Report.Agent = rep(n) }, PlatformFeedbackAgentMax},
	{"report.os", func(in *PlatformFeedbackCreateInput, n int) { in.Report.OS = rep(n) }, PlatformFeedbackOSMax},
	{"report.arch", func(in *PlatformFeedbackCreateInput, n int) { in.Report.Arch = rep(n) }, PlatformFeedbackArchMax},
	{"report.command", func(in *PlatformFeedbackCreateInput, n int) { in.Report.Command = rep(n) }, PlatformFeedbackCommandMax},
	{"report.docUrl", func(in *PlatformFeedbackCreateInput, n int) {
		in.Report.DocURL = PlatformFeedbackDocURLPrefix + rep(n-len(PlatformFeedbackDocURLPrefix))
	}, PlatformFeedbackDocURLMax},
	{"report.errorExcerpt", func(in *PlatformFeedbackCreateInput, n int) { in.Report.ErrorExcerpt = rep(n) }, PlatformFeedbackErrorExcerptMax},
	{"report.appSlug", func(in *PlatformFeedbackCreateInput, n int) { in.Report.AppSlug = rep(n) }, PlatformFeedbackAppSlugMax},
	{"report.sdk[0].name", func(in *PlatformFeedbackCreateInput, n int) { in.Report.SDK[0].Name = rep(n) }, PlatformFeedbackSDKNameMax},
	{"report.sdk[0].version", func(in *PlatformFeedbackCreateInput, n int) { in.Report.SDK[0].Version = rep(n) }, PlatformFeedbackVersionMax},
}

// TestValidateEnforcesEveryBoundAtItsEdge: for each bounded field, the value AT
// the contract's maximum is accepted and the value one past it is refused with
// that field's name — and the refused one never reaches the server.
func TestValidateEnforcesEveryBoundAtItsEdge(t *testing.T) {
	// The literal maxima, restated from the contract so a constant edited in
	// platform_feedback.go cannot move the test with it.
	wantMax := map[string]int{
		"message": 2000, "report.ref": 80, "report.client.version": 32, "report.agent": 40,
		"report.os": 16, "report.arch": 16, "report.command": 120, "report.docUrl": 300,
		"report.errorExcerpt": 500, "report.appSlug": 64, "report.sdk[0].name": 40, "report.sdk[0].version": 32,
	}
	if len(pfBoundCases) != len(wantMax) {
		t.Fatalf("CONTROL failure: %d bound case(s) for %d contract bound(s)", len(pfBoundCases), len(wantMax))
	}
	s := &pfServer{reply: func(pfCall) (int, string) {
		return http.StatusOK, pfEnvelope(`{"ref":"1-a","duplicate":false,"status":"received","fixedIn":null,"note":null,"reporters":1,"reports":1}`)
	}}
	c := s.start(t)

	for _, bc := range pfBoundCases {
		t.Run(bc.field, func(t *testing.T) {
			if bc.max != wantMax[bc.field] {
				t.Fatalf("%s is bounded at %d in the code and %d in the contract", bc.field, bc.max, wantMax[bc.field])
			}
			ok := pfFullInput()
			bc.set(&ok, bc.max)
			if err := ok.Validate(); err != nil {
				t.Errorf("a %s exactly %d long must be accepted, got %v", bc.field, bc.max, err)
			}

			bad := pfFullInput()
			bc.set(&bad, bc.max+1)
			before := len(s.seen())
			_, err := c.CreatePlatformFeedback(context.Background(), bad)
			var fe *PlatformFeedbackFieldError
			if !errors.As(err, &fe) {
				t.Fatalf("a %s %d long must be refused with a field error, got %v", bc.field, bc.max+1, err)
			}
			if fe.Field != bc.field {
				t.Errorf("the refusal blames %q, want %q", fe.Field, bc.field)
			}
			if after := len(s.seen()); after != before {
				t.Errorf("the oversize %s reached the server (%d new request(s)) — it must be refused first", bc.field, after-before)
			}
		})
	}
	// Positive control on the fake: a valid input DOES reach it, so "zero new
	// requests" above is not a fact about a client that never sends.
	before := len(s.seen())
	if _, err := c.CreatePlatformFeedback(context.Background(), pfFullInput()); err != nil {
		t.Fatalf("control send: %v", err)
	}
	if got := len(s.seen()) - before; got != 1 {
		t.Fatalf("CONTROL failure: a valid input produced %d request(s), want 1", got)
	}
}

func TestValidateRefusesEveryOtherContractViolation(t *testing.T) {
	cases := []struct {
		name  string
		field string
		mut   func(in *PlatformFeedbackCreateInput)
	}{
		{"empty message", "message", func(in *PlatformFeedbackCreateInput) { in.Message = "" }},
		{"untrimmed message", "message", func(in *PlatformFeedbackCreateInput) { in.Message = " padded " }},
		{"about outside the enum", "report.about", func(in *PlatformFeedbackCreateInput) { in.Report.About = "billing" }},
		{"about missing", "report.about", func(in *PlatformFeedbackCreateInput) { in.Report.About = "" }},
		{"kind outside the enum", "report.kind", func(in *PlatformFeedbackCreateInput) { in.Report.Kind = "praise" }},
		{"ref with an upper-case letter", "report.ref", func(in *PlatformFeedbackCreateInput) { in.Report.Ref = "Submit.refused" }},
		{"ref starting with a dot", "report.ref", func(in *PlatformFeedbackCreateInput) { in.Report.Ref = ".hidden" }},
		{"ref with a space", "report.ref", func(in *PlatformFeedbackCreateInput) { in.Report.Ref = "submit refused" }},
		{"client name other than this CLI's", "report.client.name", func(in *PlatformFeedbackCreateInput) { in.Report.Client.Name = "curl" }},
		{"client version with a space", "report.client.version", func(in *PlatformFeedbackCreateInput) { in.Report.Client.Version = "0.1 beta" }},
		{"client version empty", "report.client.version", func(in *PlatformFeedbackCreateInput) { in.Report.Client.Version = "" }},
		{"docUrl on another host", "report.docUrl", func(in *PlatformFeedbackCreateInput) { in.Report.DocURL = "https://civitai.com/apps" }},
		{"docUrl over http", "report.docUrl", func(in *PlatformFeedbackCreateInput) { in.Report.DocURL = "http://developer.civitai.com/apps" }},
		{"exitCode below range", "report.exitCode", func(in *PlatformFeedbackCreateInput) { in.Report.ExitCode = intPtr(-1) }},
		{"exitCode above range", "report.exitCode", func(in *PlatformFeedbackCreateInput) { in.Report.ExitCode = intPtr(256) }},
		{"httpStatus below range", "report.httpStatus", func(in *PlatformFeedbackCreateInput) { in.Report.HTTPStatus = intPtr(99) }},
		{"httpStatus above range", "report.httpStatus", func(in *PlatformFeedbackCreateInput) { in.Report.HTTPStatus = intPtr(600) }},
		{"five sdk entries", "report.sdk", func(in *PlatformFeedbackCreateInput) {
			in.Report.SDK = []PlatformFeedbackSDK{{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}, {"e", "5"}}
		}},
		{"sdk entry with no version", "report.sdk[0].version", func(in *PlatformFeedbackCreateInput) { in.Report.SDK[0].Version = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := pfFullInput()
			tc.mut(&in)
			var fe *PlatformFeedbackFieldError
			if err := in.Validate(); !errors.As(err, &fe) || fe.Field != tc.field {
				t.Errorf("err = %v, want a field error on %q", err, tc.field)
			}
		})
	}
	// Controls: the unmutated input is valid, and so are the range edges and a
	// four-entry sdk list.
	in := pfFullInput()
	if err := in.Validate(); err != nil {
		t.Fatalf("CONTROL failure: the base input is invalid (%v), so every refusal above could be about it", err)
	}
	in.Report.ExitCode, in.Report.HTTPStatus = intPtr(0), intPtr(100)
	in.Report.SDK = []PlatformFeedbackSDK{{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}}
	if err := in.Validate(); err != nil {
		t.Errorf("exitCode 0, httpStatus 100 and four sdk entries are all in range, got %v", err)
	}
	in.Report.ExitCode, in.Report.HTTPStatus = intPtr(255), intPtr(599)
	if err := in.Validate(); err != nil {
		t.Errorf("exitCode 255 and httpStatus 599 are in range, got %v", err)
	}
}

// TestLengthsAreCountedTheWayTheServerCountsThem: the server's schema measures
// a string in UTF-16 code units, so an emoji is TWO. A rune count would accept a
// message the server then rejects.
func TestLengthsAreCountedTheWayTheServerCountsThem(t *testing.T) {
	in := pfFullInput()
	in.Report.OS = strings.Repeat("😀", 8) // 8 runes, 16 UTF-16 units: exactly the bound
	if err := in.Validate(); err != nil {
		t.Errorf("16 UTF-16 units must fit report.os, got %v", err)
	}
	in.Report.OS = strings.Repeat("😀", 8) + "x" // 9 runes, 17 units
	var fe *PlatformFeedbackFieldError
	if err := in.Validate(); !errors.As(err, &fe) || fe.Field != "report.os" {
		t.Errorf("17 UTF-16 units must not fit report.os (a rune count of 9 would let it through), got %v", err)
	}

	got, cut := ClipUTF16("ab😀cd", 3) // the emoji would straddle unit 3
	if got != "ab" || !cut {
		t.Errorf(`ClipUTF16("ab😀cd", 3) = %q, %v — want "ab", true (never half a surrogate pair)`, got, cut)
	}
	got, cut = ClipUTF16("ab😀cd", 4)
	if got != "ab😀" || !cut {
		t.Errorf(`ClipUTF16("ab😀cd", 4) = %q, %v — want "ab😀", true`, got, cut)
	}
	got, cut = ClipUTF16("ab😀cd", 6)
	if got != "ab😀cd" || cut {
		t.Errorf(`ClipUTF16 at the exact length = %q, %v — want the whole string, false`, got, cut)
	}
}

func TestRefShapes(t *testing.T) {
	for _, ok := range []string{"a", "submit.server-refused", "0", strings.Repeat("z", 80)} {
		if !ValidPlatformFeedbackRef(ok) {
			t.Errorf("site key %q must be accepted", ok)
		}
	}
	for _, bad := range []string{"", "-lead", "UPPER", "has space", "a/b", strings.Repeat("z", 81)} {
		if ValidPlatformFeedbackRef(bad) {
			t.Errorf("site key %q must be refused", bad)
		}
	}
	// A server-issued ref: the contract's own example, and nothing that could
	// break out of a suggested command line.
	for _, ok := range []string{"5123-3f9a01bc", "A1.b_c-9"} {
		if !ValidPlatformFeedbackStatusRef(ok) {
			t.Errorf("report ref %q must be accepted", ok)
		}
	}
	for _, bad := range []string{"", "5123 3f9a", "5123;rm", "$(id)", "5123\n3f9a", "`x`", strings.Repeat("9", 81)} {
		if ValidPlatformFeedbackStatusRef(bad) {
			t.Errorf("report ref %q must be refused", bad)
		}
	}
}

// --- audit round 1 -------------------------------------------------------------

// TestErrorsClaimOnlyWhatAStatusCodeCarries: a 400 does not say the limits
// changed, a 404 does not say the server is old, and neither procedure's error
// may tell the caller to re-run the OTHER verb — one mapper serves both.
func TestErrorsClaimOnlyWhatAStatusCodeCarries(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		banned []string
	}{
		{"400", http.StatusBadRequest, pfTRPCError("Invalid input"),
			[]string{"civitai upgrade", "changed", "probably", "feedback send"}},
		{"404", http.StatusNotFound, pfTRPCError(`No "query"-procedure on path "platformFeedback.getStatus"`),
			[]string{"predates", "older", "self-hosted", "yet", "feedback send"}},
		{"403 collection off", http.StatusForbidden, pfTRPCError("Feedback is not being collected here right now."),
			[]string{"feedback send", "keep the report"}},
		{"429", http.StatusTooManyRequests, pfTRPCError("Too many requests"), []string{"feedback send"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := platformFeedbackError(tc.status, []byte(tc.body)).Error()
			for _, b := range tc.banned {
				if strings.Contains(msg, b) {
					t.Errorf("the %s error says %q, which this status does not establish (or names the wrong verb):\n%s", tc.name, b, msg)
				}
			}
			// The server's own words always survive.
			var env struct {
				Error struct {
					JSON struct {
						Message string `json:"message"`
					} `json:"json"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(tc.body), &env)
			if !strings.Contains(msg, env.Error.JSON.Message) {
				t.Errorf("the server's message was dropped:\n%s", msg)
			}
		})
	}
}

// TestValidateRefusesWhatTheServerRejectsInFreeText: the server answers
// BAD_REQUEST for a NUL or an unpaired surrogate in any free-text field, so the
// last gate refuses both before the wire.
func TestValidateRefusesWhatTheServerRejectsInFreeText(t *testing.T) {
	fields := map[string]func(in *PlatformFeedbackCreateInput, v string){
		"message":             func(in *PlatformFeedbackCreateInput, v string) { in.Message = v },
		"report.errorExcerpt": func(in *PlatformFeedbackCreateInput, v string) { in.Report.ErrorExcerpt = v },
		"report.command":      func(in *PlatformFeedbackCreateInput, v string) { in.Report.Command = v },
		"report.agent":        func(in *PlatformFeedbackCreateInput, v string) { in.Report.Agent = v },
		"report.appSlug":      func(in *PlatformFeedbackCreateInput, v string) { in.Report.AppSlug = v },
	}
	for field, set := range fields {
		for name, bad := range map[string]string{"a NUL": "ab\x00cd", "an unpaired surrogate": "ab\xed\xa0\x80cd"} {
			in := pfFullInput()
			set(&in, bad)
			var fe *PlatformFeedbackFieldError
			if err := in.Validate(); !errors.As(err, &fe) || fe.Field != field {
				t.Errorf("%s with %s: err = %v, want a field error on %q", field, name, err, field)
			}
		}
	}
	if err := pfFullInput().Validate(); err != nil {
		t.Fatalf("CONTROL failure: the base input is invalid: %v", err)
	}
}
