package civitai

import (
	"context"
	"errors"
	"net/http"
	"strings"
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

// TestReadErrorShowsMessageFieldsAndPlainText is the POSITIVE CONTROL for the
// test above: a body WITH an `error`/`message` field still shows the server's
// words, unfiltered by redact (the operator's call: message fields are shown
// as-is), and a short single-line plain-text body is still shown. So the
// assertions above are not met by withholding everything.
func TestReadErrorShowsMessageFieldsAndPlainText(t *testing.T) {
	cases := []struct {
		status int
		raw    string
		want   string
	}{
		{http.StatusNotFound, `{"error":"No model with id 999"}`, "not found (404): No model with id 999"},
		{http.StatusBadGateway, `{"message":"upstream deploy in progress"}`, "server returned 502: upstream deploy in progress"},
		// A message field is NOT run through redact, even when it holds a key
		// shape: that is the scope the operator chose, pinned so a change to it is
		// a visible decision rather than a side effect.
		{http.StatusInternalServerError, `{"error":"bad key ` + readErrAPIKey + `"}`, "server returned 500: bad key " + readErrAPIKey},
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
}
