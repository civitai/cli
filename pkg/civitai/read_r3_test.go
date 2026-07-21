package civitai

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestReadError429DeepPagingCapIsUsageError asserts that a 429 whose body is the
// API's PERMANENT deep-paging cap ("too many pages" / "use cursors" / "deep
// paging") is reclassified as a USAGE error (ErrBadRequest → exit 2) rather than
// throttling (ErrRateLimited → exit 6). A scripter's generic 429 backoff-and-
// retry loop would otherwise spin forever on a structurally-doomed page.
//
// The helpful message (including the --cursor guidance) must be preserved, and
// the error must NOT carry the rate-limited sentinel.
func TestReadError429DeepPagingCapIsUsageError(t *testing.T) {
	// Bodies the server actually returns for page*limit past the offset ceiling.
	bodies := []string{
		`{"error":"You've requested too many pages, please use cursors instead"}`,
		`{"message":"Too Many Pages — use cursors instead"}`,
		`{"error":"deep paging is not supported past 1000 results"}`,
	}
	for _, body := range bodies {
		err := readError(http.StatusTooManyRequests, []byte(body))
		if err == nil {
			t.Fatalf("body %q: expected an error", body)
		}
		if !errors.Is(err, ErrBadRequest) {
			t.Errorf("body %q: deep-paging cap 429 should be tagged ErrBadRequest (usage/exit-2), got: %v", body, err)
		}
		if errors.Is(err, ErrRateLimited) {
			t.Errorf("body %q: deep-paging cap 429 must NOT be tagged ErrRateLimited (would map to exit-6 retry), got: %v", body, err)
		}
		// The helpful message + --cursor guidance is preserved.
		if !strings.Contains(err.Error(), "rate limited (429)") ||
			!strings.Contains(err.Error(), "for deep paging use --cursor instead of --page") {
			t.Errorf("body %q: message not preserved, got: %q", body, err.Error())
		}
	}
}

// TestReadError429PlainThrottleStaysRateLimited asserts a GENUINE throttling
// 429 — one whose body carries NONE of the deep-paging-cap phrases — still maps
// to ErrRateLimited (exit 6), so real rate-limit backoff handling is unbroken.
func TestReadError429PlainThrottleStaysRateLimited(t *testing.T) {
	bodies := []string{
		`{"error":"Rate limit exceeded, slow down"}`,
		`{"message":"Too many requests"}`,
		`throttled`,
	}
	for _, body := range bodies {
		err := readError(http.StatusTooManyRequests, []byte(body))
		if err == nil {
			t.Fatalf("body %q: expected an error", body)
		}
		if !errors.Is(err, ErrRateLimited) {
			t.Errorf("body %q: plain throttle 429 should stay ErrRateLimited (exit-6), got: %v", body, err)
		}
		if errors.Is(err, ErrBadRequest) {
			t.Errorf("body %q: plain throttle 429 must NOT be reclassified as ErrBadRequest, got: %v", body, err)
		}
	}
}

// TestIsDeepPagingCap unit-tests the message matcher in isolation: it fires only
// on the cap phrases (case-insensitively) and never on generic throttle text.
func TestIsDeepPagingCap(t *testing.T) {
	hits := []string{
		"You've requested too many pages",
		"please USE CURSORS instead",
		"deep paging not supported",
	}
	for _, s := range hits {
		if !isDeepPagingCap(s) {
			t.Errorf("isDeepPagingCap(%q) = false, want true", s)
		}
	}
	misses := []string{
		"rate limit exceeded",
		"too many requests",
		"slow down",
		"",
	}
	for _, s := range misses {
		if isDeepPagingCap(s) {
			t.Errorf("isDeepPagingCap(%q) = true, want false", s)
		}
	}
}
