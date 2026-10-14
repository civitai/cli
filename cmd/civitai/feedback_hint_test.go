package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestUnreadableResponsePrintsTheFeedbackHintAndKeepsExitOne is the decode
// site's SEAM test: a real 2xx reply the real SDK cannot decode, rendered by the
// real stderr path main() uses. pkg/civitai owns the tag; internal/cmd owns the
// line; this is where they meet, and where the exit code is published.
func TestUnreadableResponsePrintsTheFeedbackHintAndKeepsExitOne(t *testing.T) {
	const wantHint = `Feedback: if this is the Civitai platform getting in your way, report it to Civitai: ` +
		`civitai feedback send --about cli --ref decode.unreadable-response "<the command you ran> got a response the CLI could not read: <the error above>"`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>edge maintenance page</html>"))
	}))
	t.Cleanup(srv.Close)

	_, _, err := civitai.New(srv.URL, "").GetModelVersion(context.Background(), "44190")
	if err == nil {
		t.Fatal("an HTML 200 must fail to decode; without an error this test asserts on nothing")
	}
	var buf bytes.Buffer
	printFeedbackHint(&buf, err)
	if buf.String() != wantHint+"\n" {
		t.Errorf("stderr hint for an unreadable response:\n got %q\nwant %q", buf.String(), wantHint+"\n")
	}
	// The hint must not move the published code: an unreadable response was
	// untagged before it existed, so it exits 1.
	if got := exitCode(err); got != exitGeneric {
		t.Errorf("exit code = %d, want %d (exitGeneric) — the hint's tag changed the classification", got, exitGeneric)
	}
	if !strings.Contains(errorLine(err), "unexpected response from") {
		t.Errorf("the error line changed: %q", errorLine(err))
	}
}

// TestNoFeedbackHintForAnOrdinaryReadError: a 404 is the caller's answer, not
// platform friction — nothing is printed under it.
func TestNoFeedbackHintForAnOrdinaryReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no version 44190"}`))
	}))
	t.Cleanup(srv.Close)

	_, _, err := civitai.New(srv.URL, "").GetModelVersion(context.Background(), "44190")
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Fatalf("CONTROL failure: want a 404, got %v", err)
	}
	var buf bytes.Buffer
	printFeedbackHint(&buf, err)
	if buf.Len() != 0 {
		t.Errorf("a 404 printed a feedback hint: %q", buf.String())
	}
}
