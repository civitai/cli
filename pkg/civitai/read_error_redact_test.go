package civitai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// readErrNginx502 is a realistic edge 502 page, CRLF line endings and all, as
// nginx serves it in front of the API. 150 bytes.
const readErrNginx502 = "<html>\r\n<head><title>502 Bad Gateway</title></head>\r\n<body>\r\n" +
	"<center><h1>502 Bad Gateway</h1></center>\r\n<hr><center>nginx</center>\r\n</body>\r\n</html>\r\n"

// readErrAPIKey is a realistic 32-hex Civitai API key (civitai's
// generateKey(length = 32)), distinct from every other fixture in this package.
const readErrAPIKey = "7c2e91d04b8fa36e5d10c9f7a24b8e63"

// TestReadErrorFallbackRedactsTheDisplayedBody pins readError's fallback for a
// non-2xx body with no `error`/`message` field: what it DISPLAYS goes through
// redact.CredentialRouteText, so a body is shown only when it is single-line
// plain text with no token shape, and otherwise only by its size. It used to
// display the raw body, cut to 500 bytes with control characters stripped but
// nothing redacted — so an edge 502 HTML page was printed as markup and a
// plain-text body holding a 32-hex API key put the key on stderr. Same class as
// civitai/cli#959 (appapi) and #960 (genapi).
//
// Every readError arm that interpolates the body is driven. 400 is not: it never
// prints the fallback (badRequestDetail reads only JSON fields).
func TestReadErrorFallbackRedactsTheDisplayedBody(t *testing.T) {
	bodies := []struct{ label, raw, secret string }{
		{"edge 502 html", readErrNginx502, "502 Bad Gateway</title>"},
		{"plain text with a 32-hex key", "invalid API key " + readErrAPIKey + "\n", readErrAPIKey},
		{"json without a message field carrying a key", `{"ok":false,"key":"` + readErrAPIKey + `"}`, readErrAPIKey},
	}
	statuses := []int{
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	}
	for _, status := range statuses {
		for _, body := range bodies {
			t.Run(http.StatusText(status)+"/"+body.label, func(t *testing.T) {
				msg := readError(status, []byte(body.raw)).Error()
				if strings.Contains(msg, body.secret) {
					t.Errorf("the error carries the body's secret %q:\n  %.300q", body.secret, msg)
				}
				for _, tag := range []string{"<html", "<center", "</"} {
					if strings.Contains(msg, tag) {
						t.Errorf("the error carries HTML markup %q from the body:\n  %.300q", tag, msg)
					}
				}
				if !strings.Contains(msg, "response body not shown because it may carry credentials") {
					t.Errorf("the withheld body is not reported by its size:\n  %.300q", msg)
				}
			})
		}
	}

	// The whole text, pinned for one arm: the status and the body's size (150
	// bytes, counted independently of the code under test).
	if got, want := readError(http.StatusBadGateway, []byte(readErrNginx502)).Error(),
		"server returned 502: 150-byte response body not shown because it may carry credentials"; got != want {
		t.Errorf("readError(502, nginx html) =\n  %q\nwant\n  %q", got, want)
	}

	// The same path end to end: a 404 (terminal, not retried) through a real
	// Client and getInto, as `civitai models get` would see it.
	c := serveOnce(t, http.StatusNotFound, "invalid API key "+readErrAPIKey)
	_, _, err := c.GetModel(context.Background(), "1")
	if err == nil {
		t.Fatal("a 404 must be an error; without it this half asserts on nothing")
	}
	if strings.Contains(err.Error(), readErrAPIKey) {
		t.Errorf("GetModel's 404 error carries the body's API key:\n  %q", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("the 404 lost ErrNotFound: %v", err)
	}
}

// TestReadErrorRedactsATokenShapedMessageField pins readError's DISPLAY of an
// `error`/`message` field: a field holding a token shape (redact.HasTokenShape,
// the rules ServerText refuses on — a 32-hex Civitai API key, a 40+ run, a JWT)
// is reported by the body's size, like the no-field fallback. It used to be
// printed whole, so `{"error":"bad key <key>"}` put the key on stderr.
//
// The split-key case is the one a check on the RAW field misses: U+00AD (Cf)
// sits inside the key, so the raw text holds two runs of 12 and 20, and only
// after snippet's saferune strip does the display carry the joined 32-hex key.
// The check must be on what is shown.
func TestReadErrorRedactsATokenShapedMessageField(t *testing.T) {
	bodies := []struct{ label, raw, secret string }{
		{"error field with a 32-hex key", `{"error":"bad key ` + readErrAPIKey + `"}`, readErrAPIKey},
		{"message field with a 32-hex key", `{"message":"token rejected: ` + readErrAPIKey + `"}`, readErrAPIKey},
		{"error field with a key split by a soft hyphen", `{"error":"bad key 7c2e91d04b8f\u00ada36e5d10c9f7a24b8e63"}`, readErrAPIKey},
	}
	statuses := []int{
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusServiceUnavailable,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	}
	for _, status := range statuses {
		for _, body := range bodies {
			t.Run(http.StatusText(status)+"/"+body.label, func(t *testing.T) {
				msg := readError(status, []byte(body.raw)).Error()
				if strings.Contains(msg, body.secret) {
					t.Errorf("the error carries the field's key %q:\n  %.300q", body.secret, msg)
				}
				if !strings.Contains(msg, "response body not shown because it may carry credentials") {
					t.Errorf("the withheld field is not reported by the body's size:\n  %.300q", msg)
				}
			})
		}
	}

	// The whole text, pinned: the body sizes (52, 58 and 56 bytes) are counted
	// independently of the code under test. The 400 arm prints badRequestDetail,
	// which reads the same fields, so it is held to the same rule.
	for _, c := range []struct {
		status int
		raw    string
		want   string
	}{
		{http.StatusBadRequest, `{"message":"bad param ` + readErrAPIKey + `"}`,
			"invalid request parameter (400): 56-byte response body not shown because it may carry credentials"},
		{http.StatusInternalServerError, `{"error":"bad key ` + readErrAPIKey + `"}`,
			"server returned 500: 52-byte response body not shown because it may carry credentials"},
		{http.StatusNotFound, `{"error":"bad key 7c2e91d04b8f\u00ada36e5d10c9f7a24b8e63"}`,
			"not found (404): 58-byte response body not shown because it may carry credentials"},
	} {
		if got := readError(c.status, []byte(c.raw)).Error(); got != c.want {
			t.Errorf("readError(%d, %q) =\n  %q\nwant\n  %q", c.status, c.raw, got, c.want)
		}
	}

	// A key straddling snippet's 500-byte cut: it starts at byte 469 of the
	// field, so the cut leaves its first 31 characters, which no longer match
	// the 32-hex rule. The check must see the field before the cut. Driven on
	// the 400 arm (badRequestDetail) and on a non-400 arm.
	straddle := `{"message":"z` + strings.Repeat(" ", 468) + readErrAPIKey + `"}`
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound} {
		msg := readError(status, []byte(straddle)).Error()
		if strings.Contains(msg, readErrAPIKey[:24]) {
			t.Errorf("readError(%d) prints a prefix of a key that straddles the 500-byte cut:\n  %.300q", status, msg)
		}
		if !strings.Contains(msg, "response body not shown because it may carry credentials") {
			t.Errorf("readError(%d): the straddling key's field is not reported by the body's size:\n  %.300q", status, msg)
		}
	}

	// End to end: a 404 through a real Client, as `civitai models get` sees it.
	c := serveOnce(t, http.StatusNotFound, `{"error":"bad key `+readErrAPIKey+`"}`)
	_, _, err := c.GetModel(context.Background(), "1")
	if err == nil {
		t.Fatal("a 404 must be an error; without it this half asserts on nothing")
	}
	if strings.Contains(err.Error(), readErrAPIKey) {
		t.Errorf("GetModel's 404 error carries the field's API key:\n  %q", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("the 404 lost ErrNotFound: %v", err)
	}
}

// TestReadErrorShowsMessageFieldsAndPlainText is the POSITIVE CONTROL for the
// tests above: a body WITH an `error`/`message` field that holds no token shape
// still shows the server's words — including a nested (non-string) `error`
// value, which only the token-shape rules are applied to, not ServerText's
// JSON refusal — and a short single-line plain-text body is still shown. So the
// assertions above are not met by withholding everything.
func TestReadErrorShowsMessageFieldsAndPlainText(t *testing.T) {
	cases := []struct {
		status int
		raw    string
		want   string
	}{
		{http.StatusNotFound, `{"error":"No model with id 999"}`, "not found (404): No model with id 999"},
		{http.StatusBadGateway, `{"message":"upstream deploy in progress"}`, "server returned 502: upstream deploy in progress"},
		{http.StatusNotFound, `{"error":{"code":"not_found","path":"models/41"}}`, `not found (404): {"code":"not_found","path":"models/41"}`},
		{http.StatusServiceUnavailable, "Service Temporarily Unavailable\n",
			"service unavailable (503): Service Temporarily Unavailable — the search backend is briefly overloaded; retry shortly"},
		{http.StatusInternalServerError, "", "server returned 500: empty response body"},
	}
	for _, c := range cases {
		if got := readError(c.status, []byte(c.raw)).Error(); got != c.want {
			t.Errorf("readError(%d, %q) =\n  %q\nwant\n  %q", c.status, c.raw, got, c.want)
		}
	}
}

// TestDeepPagingCapStillReadsTheRawBody proves the 429 deep-paging check keeps
// classifying on the RAW body when the DISPLAYED text is withheld. The body is a
// non-JSON HTML page carrying the cap phrase: its display is reduced to a size,
// so a classifier that read the displayed text would find no phrase and leave
// it ErrRateLimited (exit 6).
//
// 🔴 INVARIANT GUARD, NOT REGRESSION COVERAGE: the pre-change code also
// classified on the raw body, so this is green before and after. It pins that
// the display change did not move the classifier. A throttle-worded twin is the
// control that the same shape is not reclassified regardless of its text.
func TestDeepPagingCapStillReadsTheRawBody(t *testing.T) {
	capPage := "<html>\r\n<body>\r\n<p>You've requested too many pages, please use cursors instead</p>\r\n</body>\r\n</html>\r\n"
	err := readError(http.StatusTooManyRequests, []byte(capPage))
	if !errors.Is(err, ErrBadRequest) || errors.Is(err, ErrRateLimited) {
		t.Errorf("a cap-worded 429 HTML body must be reclassified ErrBadRequest (exit 2), got %v", err)
	}
	if strings.Contains(err.Error(), "too many pages") || strings.Contains(err.Error(), "<p>") {
		t.Errorf("the 429 body was displayed, so this case does not prove the classifier reads the raw body:\n  %q", err)
	}
	if !strings.Contains(err.Error(), "for deep paging use --cursor instead of --page") {
		t.Errorf("the --cursor guidance is gone:\n  %q", err)
	}

	throttlePage := "<html>\r\n<body>\r\n<p>Rate limit exceeded, slow down</p>\r\n</body>\r\n</html>\r\n"
	err = readError(http.StatusTooManyRequests, []byte(throttlePage))
	if !errors.Is(err, ErrRateLimited) || errors.Is(err, ErrBadRequest) {
		t.Errorf("a throttle-worded 429 HTML body must stay ErrRateLimited (exit 6), got %v", err)
	}

	// The same for a FIELD the display withholds: a cap-worded message that also
	// carries a key is withheld, and still reclassified from the raw field.
	capField := `{"message":"You've requested too many pages, please use cursors instead (key ` + readErrAPIKey + `)"}`
	err = readError(http.StatusTooManyRequests, []byte(capField))
	if !errors.Is(err, ErrBadRequest) || errors.Is(err, ErrRateLimited) {
		t.Errorf("a cap-worded 429 field must be reclassified ErrBadRequest (exit 2), got %v", err)
	}
	if strings.Contains(err.Error(), "too many pages") || strings.Contains(err.Error(), readErrAPIKey) {
		t.Errorf("the 429 field was displayed, so this case does not prove the classifier reads the raw field:\n  %q", err)
	}
}

// TestRetryExhaustedErrorRedactsTheDisplayedBody pins retryExhaustedError's
// display: the body of the last failed attempt goes through
// redact.CredentialRouteText, as readError's fallback does. It used to be
// snippet(raw) — control characters stripped, nothing redacted — so a 502 HTML
// page that persisted through every retry was printed as markup, and a plain
// body holding a 32-hex key put the key on stderr.
func TestRetryExhaustedErrorRedactsTheDisplayedBody(t *testing.T) {
	const prefix = "Civitai returned HTTP %d after 4 attempts — the service is temporarily unavailable, try again shortly"

	// A realistic edge 502 served on EVERY attempt through a real Client.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(readErrNginx502))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = &bytes.Buffer{}
	_, _, err := c.GetModel(context.Background(), "1")
	if err == nil {
		t.Fatal("a persistent 502 must be an error; without it this half asserts on nothing")
	}
	if got := atomic.LoadInt32(&hits); got != readMaxAttempts {
		t.Fatalf("the 502 was served %d times, want %d: this is not the retry-exhausted path", got, readMaxAttempts)
	}
	if want := fmt.Sprintf(prefix, 502) + ": 150-byte response body not shown because it may carry credentials"; err.Error() != want {
		t.Errorf("persistent 502 error =\n  %q\nwant\n  %q", err.Error(), want)
	}
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("the exhausted 502 lost ErrNetwork: %v", err)
	}

	// A plain body with a 32-hex key (54 bytes, counted independently).
	keyBody := "upstream rejected key " + readErrAPIKey
	got := retryExhaustedError(http.StatusServiceUnavailable, 4, []byte(keyBody)).Error()
	if strings.Contains(got, readErrAPIKey) {
		t.Errorf("the exhausted 503 carries the body's API key:\n  %q", got)
	}
	if want := fmt.Sprintf(prefix, 503) + ": 54-byte response body not shown because it may carry credentials"; got != want {
		t.Errorf("retryExhaustedError(503, key body) =\n  %q\nwant\n  %q", got, want)
	}

	// POSITIVE CONTROLS: a short plain-text body and a plain error field stay
	// readable, and an empty body adds nothing (as before).
	for _, pc := range []struct {
		status int
		raw    string
		want   string
	}{
		{http.StatusServiceUnavailable, "Service Temporarily Unavailable\n", fmt.Sprintf(prefix, 503) + ": Service Temporarily Unavailable"},
		{http.StatusGatewayTimeout, `{"error":"upstream timed out"}`, fmt.Sprintf(prefix, 504) + ": upstream timed out"},
		{http.StatusBadGateway, "", fmt.Sprintf(prefix, 502)},
	} {
		if got := retryExhaustedError(pc.status, 4, []byte(pc.raw)).Error(); got != pc.want {
			t.Errorf("retryExhaustedError(%d, %q) =\n  %q\nwant\n  %q", pc.status, pc.raw, got, pc.want)
		}
	}
}
