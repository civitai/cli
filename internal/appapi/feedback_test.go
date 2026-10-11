package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// feedbackTestServer records the one request it receives and answers it.
type feedbackTestServer struct {
	method, path, input, body string
	hasInput                  bool
}

func (s *feedbackTestServer) start(t *testing.T, status int, reply string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.method, s.path, s.body = r.Method, r.URL.Path, string(raw)
		s.input = r.URL.Query().Get("input")
		_, s.hasInput = r.URL.Query()["input"]
		if status != http.StatusOK {
			w.WriteHeader(status)
		}
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok", "")
}

// TestFeedbackListInputOmitsEveryUnsetOptional asserts KEY ABSENCE on the
// decoded request (AGENTS.md item 14): the server rejects `cursor: 0`
// (.positive()), `limit: 0` (.min(1)) and `ownerStatus: ""` (not in the enum),
// so an unset field must not be sent at all.
func TestFeedbackListInputOmitsEveryUnsetOptional(t *testing.T) {
	var s feedbackTestServer
	c := s.start(t, http.StatusOK, `{"result":{"data":{"json":{"items":[]}}}}`)
	page, err := c.ListAppFeedback(context.Background(), FeedbackListInput{AppListingID: "apl_9"})
	if err != nil {
		t.Fatalf("ListAppFeedback: %v", err)
	}
	if len(page.Items) != 0 || page.NextCursor != nil {
		t.Errorf("an empty page with no cursor decoded as %+v", page)
	}
	if s.method != http.MethodGet || s.path != "/api/trpc/appFeedback.listForListing" {
		t.Errorf("want GET /api/trpc/appFeedback.listForListing, got %s %s", s.method, s.path)
	}
	var env struct {
		JSON map[string]any `json:"json"`
	}
	if err := json.Unmarshal([]byte(s.input), &env); err != nil {
		t.Fatalf("input is not JSON: %v — %q", err, s.input)
	}
	if env.JSON["appListingId"] != "apl_9" {
		t.Fatalf("CONTROL failure: decoded input lacks the listing id, so absence proves nothing: %s", s.input)
	}
	for _, k := range []string{"cursor", "limit", "ownerStatus"} {
		if v, ok := env.JSON[k]; ok {
			t.Errorf("unset %s was sent as %#v; it must be ABSENT: %s", k, v, s.input)
		}
	}
	if len(env.JSON) != 1 {
		t.Errorf("a zero-valued input must marshal to the listing id alone, got %d keys: %s", len(env.JSON), s.input)
	}
}

func TestFeedbackListInputSendsWhatIsSet(t *testing.T) {
	var s feedbackTestServer
	c := s.start(t, http.StatusOK, `{"result":{"data":{"json":{"items":[],"nextCursor":77}}}}`)
	page, err := c.ListAppFeedback(context.Background(), FeedbackListInput{AppListingID: "apl_9", Cursor: 120, Limit: 25, OwnerStatus: "wont_fix"})
	if err != nil {
		t.Fatalf("ListAppFeedback: %v", err)
	}
	if want := `{"json":{"appListingId":"apl_9","cursor":120,"limit":25,"ownerStatus":"wont_fix"}}`; s.input != want {
		t.Errorf("input\n got: %s\nwant: %s", s.input, want)
	}
	if page.NextCursor == nil || *page.NextCursor != 77 {
		t.Errorf("nextCursor 77 decoded as %v", page.NextCursor)
	}
}

// TestFeedbackNextCursorNullMeansNoMore: an explicit null and an absent key
// both mean the inbox is exhausted.
func TestFeedbackNextCursorNullMeansNoMore(t *testing.T) {
	var s feedbackTestServer
	c := s.start(t, http.StatusOK, `{"result":{"data":{"json":{"items":[],"nextCursor":null}}}}`)
	page, err := c.ListAppFeedback(context.Background(), FeedbackListInput{AppListingID: "apl_9"})
	if err != nil {
		t.Fatalf("ListAppFeedback: %v", err)
	}
	if page.NextCursor != nil {
		t.Errorf("a null nextCursor decoded as %d", *page.NextCursor)
	}
}

// TestSetAppFeedbackStatusAlwaysSendsExpectedOwnerStatus: the key is REQUIRED
// by the server (`.nullable()`, not `.optional()`); nil travels as JSON null.
func TestSetAppFeedbackStatusAlwaysSendsExpectedOwnerStatus(t *testing.T) {
	ack := "acknowledged"
	for _, tc := range []struct {
		name     string
		expected *string
		want     string
	}{
		{"still new", nil, `{"json":{"id":31,"appListingId":"apl_9","ownerStatus":"resolved","expectedOwnerStatus":null}}`},
		{"was acknowledged", &ack, `{"json":{"id":31,"appListingId":"apl_9","ownerStatus":"resolved","expectedOwnerStatus":"acknowledged"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s feedbackTestServer
			c := s.start(t, http.StatusOK, `{"result":{"data":{"json":{"id":31,"ownerStatus":"resolved"}}}}`)
			if err := c.SetAppFeedbackStatus(context.Background(), "apl_9", 31, "resolved", tc.expected); err != nil {
				t.Fatalf("SetAppFeedbackStatus: %v", err)
			}
			if s.method != http.MethodPost || s.path != "/api/trpc/appFeedback.setOwnerStatus" {
				t.Errorf("want POST /api/trpc/appFeedback.setOwnerStatus, got %s %s", s.method, s.path)
			}
			if s.body != tc.want {
				t.Errorf("body\n got: %s\nwant: %s", s.body, tc.want)
			}
		})
	}
}

func TestFlagAppFeedbackBody(t *testing.T) {
	var s feedbackTestServer
	c := s.start(t, http.StatusOK, `{"result":{"data":{"json":{"id":31}}}}`)
	if err := c.FlagAppFeedback(context.Background(), "apl_9", 31); err != nil {
		t.Fatalf("FlagAppFeedback: %v", err)
	}
	if s.method != http.MethodPost || s.path != "/api/trpc/appFeedback.flagAbusive" {
		t.Errorf("want POST /api/trpc/appFeedback.flagAbusive, got %s %s", s.method, s.path)
	}
	if want := `{"json":{"id":31,"appListingId":"apl_9"}}`; s.body != want {
		t.Errorf("body\n got: %s\nwant: %s", s.body, want)
	}
}

// TestCountNewAppFeedbackSendsNoInput: the procedure declares no `.input()`,
// so no ?input= parameter is sent — not even `{"json":null}`.
func TestCountNewAppFeedbackSendsNoInput(t *testing.T) {
	var s feedbackTestServer
	c := s.start(t, http.StatusOK, `{"result":{"data":{"json":{"apl_9":4}}}}`)
	counts, err := c.CountNewAppFeedback(context.Background())
	if err != nil {
		t.Fatalf("CountNewAppFeedback: %v", err)
	}
	if s.hasInput {
		t.Errorf("countNewForMyListings takes no input; an input parameter was sent: %q", s.input)
	}
	if s.path != "/api/trpc/appFeedback.countNewForMyListings" || s.method != http.MethodGet {
		t.Errorf("want GET /api/trpc/appFeedback.countNewForMyListings, got %s %s", s.method, s.path)
	}
	if len(counts) != 1 || counts["apl_9"] != 4 {
		t.Errorf("counts = %v, want {apl_9: 4}", counts)
	}
}

// TestFeedbackNullPayloadIsMalformed: `null` decodes cleanly into a zero page
// or an empty map, which a caller would render as "no feedback".
func TestFeedbackNullPayloadIsMalformed(t *testing.T) {
	var s feedbackTestServer
	c := s.start(t, http.StatusOK, `{"result":{"data":{"json":null}}}`)
	if page, err := c.ListAppFeedback(context.Background(), FeedbackListInput{AppListingID: "apl_9"}); err == nil {
		t.Errorf("a null list payload must be an error, got %+v", page)
	}
	if counts, err := c.CountNewAppFeedback(context.Background()); err == nil {
		t.Errorf("a null count payload must be an error, got %v", counts)
	}
}

// TestFeedbackErrorClassification pins the EXIT-CODE half of every arm by
// errors.Is (AGENTS.md item 7), separately from the wording.
func TestFeedbackErrorClassification(t *testing.T) {
	body := func(msg string) []byte {
		b, _ := json.Marshal(map[string]any{"error": map[string]any{"json": map[string]any{"message": msg}}})
		return b
	}
	sentinels := map[string]error{
		"ErrUnauthorized": civitai.ErrUnauthorized, "ErrNotFound": civitai.ErrNotFound,
		"ErrBadRequest": civitai.ErrBadRequest, "ErrRateLimited": civitai.ErrRateLimited, "ErrNetwork": civitai.ErrNetwork,
	}
	for _, tc := range []struct {
		status int
		want   string // the one sentinel it must carry, or "" for none
	}{
		{http.StatusUnauthorized, "ErrUnauthorized"},
		{http.StatusForbidden, "ErrUnauthorized"},
		{http.StatusNotFound, "ErrNotFound"},
		{http.StatusBadRequest, "ErrBadRequest"},
		{http.StatusTooManyRequests, "ErrRateLimited"},
		{http.StatusServiceUnavailable, "ErrNetwork"},
		{http.StatusConflict, ""},
		{http.StatusInternalServerError, ""},
	} {
		err := feedbackError(tc.status, body("server words"), feedbackOpSetStatus)
		if err == nil {
			t.Fatalf("status %d: nil error", tc.status)
		}
		for name, s := range sentinels {
			if got := errors.Is(err, s); got != (name == tc.want) {
				t.Errorf("status %d: errors.Is(%s) = %t, want %t — %v", tc.status, name, got, name == tc.want, err)
			}
		}
	}
}

// TestFeedbackConflictIsTypedAndKnowsWhichWrite: the command layer builds its
// wording from this, so the type and the Flag bit are the contract.
func TestFeedbackConflictIsTypedAndKnowsWhichWrite(t *testing.T) {
	raw := []byte(`{"error":{"json":{"message":"This feedback has changed — refresh and try again."}}}`)
	for _, tc := range []struct {
		op       feedbackOp
		wantFlag bool
	}{{feedbackOpSetStatus, false}, {feedbackOpFlag, true}} {
		var conflict *FeedbackConflictError
		if err := feedbackError(http.StatusConflict, raw, tc.op); !errors.As(err, &conflict) {
			t.Fatalf("op %d: a 409 must be a *FeedbackConflictError, got %T %v", tc.op, err, err)
		}
		if conflict.Flag != tc.wantFlag {
			t.Errorf("op %d: Flag = %t, want %t", tc.op, conflict.Flag, tc.wantFlag)
		}
		if conflict.ServerMsg != "This feedback has changed — refresh and try again." {
			t.Errorf("op %d: ServerMsg = %q", tc.op, conflict.ServerMsg)
		}
	}
}

// TestFeedbackStatusSpellsNullAsNew: the filter vocabulary, on the way out.
func TestFeedbackStatusSpellsNullAsNew(t *testing.T) {
	var rows []Feedback
	if err := json.Unmarshal([]byte(`[{"id":1,"ownerStatus":null},{"id":2},{"id":3,"ownerStatus":"wont_fix"}]`), &rows); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"new", "new", "wont_fix"} {
		if got := rows[i].Status(); got != want {
			t.Errorf("row %d: Status() = %q, want %q", rows[i].ID, got, want)
		}
	}
}
