package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// TestCheckLimitIsUsageTagged asserts an out-of-range --limit is classified as a
// USAGE error (exit 2), not the generic one. checkLimit is the single shared
// validator, so tagging it there classifies EVERY read subcommand at once.
func TestCheckLimitIsUsageTagged(t *testing.T) {
	err := checkLimit(999, 100)
	if err == nil {
		t.Fatal("expected an error for an out-of-range limit")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("out-of-range limit should be a usage error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "between 1 and 100") {
		t.Errorf("message should be preserved, got: %v", err)
	}
}

// TestModelsSearchLimitUsageTaggedEndToEnd drives the real command so the tag is
// verified through the full RunE path (the bad flag value must reach exit 2).
func TestModelsSearchLimitUsageTaggedEndToEnd(t *testing.T) {
	_, _, err := run(t, "models", "search", "--limit", "999")
	if err == nil {
		t.Fatal("expected an error for --limit 999")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--limit 999 should classify as usage, got: %v", err)
	}
}

// TestModelVersionsGetNonIntIsUsageTagged asserts a non-integer positional id is
// a usage error, not a generic one.
func TestModelVersionsGetNonIntIsUsageTagged(t *testing.T) {
	_, _, err := run(t, "model-versions", "get", "abc")
	if err == nil {
		t.Fatal("expected an error for a non-integer model-version id")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("non-integer id should classify as usage, got: %v", err)
	}
	if !strings.Contains(err.Error(), "must be an integer") {
		t.Errorf("message should be preserved, got: %v", err)
	}
}

// TestUsersGetMissingIsNotFoundTagged asserts a lookup that resolves to zero
// users is classified as NOT-FOUND (exit 4) even though the search endpoint
// returns an empty 200, matching the behaviour of a real HTTP 404.
func TestUsersGetMissingIsNotFoundTagged(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	_, _, err := run(t, "users", "get", "zzz-no-such-user-zzz")
	if err == nil {
		t.Fatal("expected an error for a missing user")
	}
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("missing user should classify as not-found, got: %v", err)
	}
}

// TestUsersGetNoExactMatchIsNotFoundTagged asserts a name query that returns
// only fuzzy neighbours (no exact username) is also classified as not-found.
func TestUsersGetNoExactMatchIsNotFoundTagged(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":5,"username":"bob"},{"id":6,"username":"bobby"}]}`))
	})
	_, _, err := run(t, "users", "get", "bo")
	if err == nil {
		t.Fatal("expected an error for a no-exact-match user query")
	}
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("no-exact-match user should classify as not-found, got: %v", err)
	}
}

// TestImagesSearchBadEnumIsBadRequestTaggedAndClean asserts an invalid enum
// (rejected by the server with a 400 ZodError body) is (a) classified as a
// bad-request/usage error and (b) surfaced as a CONCISE message — never the raw
// ZodError JSON blob.
func TestImagesSearchBadEnumIsBadRequestTaggedAndClean(t *testing.T) {
	const zodBody = `{"error":{"name":"ZodError","message":"[\n  {\n    \"code\": \"invalid_value\",\n    \"path\": [\n      \"period\"\n    ],\n    \"message\": \"Invalid option: expected one of \\\"Day\\\"|\\\"Week\\\"|\\\"Month\\\"|\\\"Year\\\"|\\\"AllTime\\\"\"\n  }\n]"}}`
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(zodBody))
	})
	_, _, err := run(t, "images", "search", "--period", "Bogus")
	if err == nil {
		t.Fatal("expected an error for an invalid --period enum")
	}
	if !errors.Is(err, api.ErrBadRequest) {
		t.Errorf("400 should classify as bad-request (usage), got: %v", err)
	}
	if strings.Contains(err.Error(), "ZodError") {
		t.Errorf("the raw ZodError blob must not leak into the message: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid request parameter (400)") {
		t.Errorf("message should be the concise 400 form, got: %v", err)
	}
	// The concise message should name the offending field.
	if !strings.Contains(err.Error(), "period") {
		t.Errorf("concise message should name the invalid field, got: %v", err)
	}
}
