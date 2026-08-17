package main

// THE PUBLISHED EXIT CODE FOR A `civitai app listing` LOOKUP THAT FAILS FOR A
// REASON THAT IS NOT "NOT FOUND".
//
// civitai/cli#422 outcome 1 gave `resolveListing` a SECOND lookup: when the block
// submission 404s it retries by slug through `appListings.getMyListingForApp`.
// That made the published contract two claims instead of one — either lookup can
// fail, and neither failure is a 404 — and the README said something false about
// both:
//
//	"A submissions failure that is not a 404 … keeps its own code (`3`, `5`)"
//
// `3, 5` is not the set. A plain `500` carries NO classification sentinel
// (pkg/civitai's statusKind returns nil for it), so it falls to the generic `1`;
// only 502/503/504 are ErrNetwork. And the FALLBACK's own failure was not covered
// at all: `if ref, slugErr := …; slugErr == nil` discarded slugErr, so a 403 or a
// 500 from the by-slug arm fell through carrying the SUBMISSION's not-found and
// exited `4` under a message naming causes that did not include "the server
// errored".
//
// internal/cmd's TestFallbackFailuresKeepTheirOwnError pins the CLASSIFICATION.
// This file pins what the README actually promises a script — the PROCESS EXIT
// STATUS — by building the real binary and reading what it returns, the same
// split (and the same buildCLI/runCLI harness) as generate_input_exitcode_test.go.
// The two are not the same claim: a classification can be right while `exitCode`
// routes it somewhere else, which is exactly what issue #241 was.
//
// MEASURED — see the `want` column of each row below; every number in
// exitCodeDocs' offsite bullets is one of these, and no number appears there that
// is not measured here.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// listingLookupStage is which of resolveListing's two lookups the row breaks.
type listingLookupStage int

const (
	// breakSubmissions fails the FIRST lookup (GET /api/v1/blocks/submissions).
	// The by-slug fallback must not run at all.
	breakSubmissions listingLookupStage = iota
	// breakFallback 404s the first lookup — the offsite shape — and fails the
	// SECOND (appListings.getMyListingForApp).
	breakFallback
)

// TestAppListingLookupFailureExitCodes measures the process exit status for every
// non-404 failure of either lookup.
//
// 🔴 THE ROWS ARE THE README'S NUMBERS, AND THE `500` ROWS ARE WHY THIS EXISTS.
// A reader of "keeps its own code (3, 5)" who branches `rc == 5` on a server
// outage gets `1` from a plain 500 and treats a transient failure as a permanent
// one. The 502/503 rows are the CONTROL that 5 is reachable at all, so a suite
// where everything collapsed to 1 could not pass.
//
// 🔴 EVERY ROW ALSO ASSERTS THE MESSAGE IS NOT THE OFFSITE REFUSAL. The exit code
// alone cannot see the swallow this test was written for: before the fix, the
// fallback's 403 exited 4 — a code the contract already used — so only the pair
// (code AND text) distinguishes "the listing is absent" from "the lookup failed".
func TestAppListingLookupFailureExitCodes(t *testing.T) {
	bin := buildCLI(t)

	for _, tc := range []struct {
		name  string
		stage listingLookupStage
		// status is what the broken route answers with.
		status int
		// want is the MEASURED process exit status.
		want int
		// wantStderr is a fragment of the error the user sees, chosen from the
		// arm that produced it rather than from a shared prefix.
		wantStderr string
	}{
		// --- the FIRST lookup (submissions) ---
		{"submissions 403", breakSubmissions, http.StatusForbidden, 3, "apps access required"},
		{"submissions 500", breakSubmissions, http.StatusInternalServerError, 1, "server returned 500"},
		{"submissions 502", breakSubmissions, http.StatusBadGateway, 5, "server returned 502"},
		{"submissions 503", breakSubmissions, http.StatusServiceUnavailable, 5, "apps is not enabled"},
		{"submissions 429", breakSubmissions, http.StatusTooManyRequests, 6, "rate limited"},

		// --- the SECOND lookup (the by-slug fallback) ---
		{"fallback 403", breakFallback, http.StatusForbidden, 3, "not permitted for your account"},
		{"fallback 500", breakFallback, http.StatusInternalServerError, 1, ""},
		{"fallback 502", breakFallback, http.StatusBadGateway, 5, ""},
		{"fallback 503", breakFallback, http.StatusServiceUnavailable, 5, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				subs := strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions")
				slug := strings.Contains(r.URL.Path, "getMyListingForApp")
				switch {
				case subs && tc.stage == breakSubmissions:
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprintf(w, `{"error":"broken with %d"}`, tc.status)
				case subs:
					// The offsite shape: a 200 with an empty list, which
					// GetSubmission resolves to ErrNotFound client-side. This is
					// what makes the fallback run at all.
					_, _ = w.Write([]byte(`{"submissions":[]}`))
				case slug:
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprintf(w, `{"error":{"json":{"message":"broken with %d"}}}`, tc.status)
				default:
					// The kind probe (GET /api/v1/apps/{slug}) must never be
					// reached: it runs only when BOTH lookups answered not-found.
					// Answering 418 rather than a plausible body means a row that
					// somehow got here cannot pass by accident.
					w.WriteHeader(http.StatusTeapot)
				}
			}))
			defer srv.Close()

			rc, stderr := runCLI(t, bin, []string{
				"XDG_CONFIG_HOME=" + filepath.Join(t.TempDir(), "config"),
				"CIVITAI_TOKEN=tok-1",
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_NO_UPDATE_CHECK=1",
			}, "app", "listing", "status", "--slug", "zzz-lookup-fixture")

			if rc != tc.want {
				t.Errorf("exit status %d, want %d\nstderr: %s", rc, tc.want, stderr)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr must carry the failing lookup's own words %q; got:\n%s", tc.wantStderr, stderr)
			}
			// The refusal is a claim about the app's KIND, reached only when both
			// lookups say not-found. None of these rows may produce it.
			if strings.Contains(stderr, "OFFSITE app") {
				t.Errorf("a %d must not be rewritten into a claim about the app's kind:\n%s", tc.status, stderr)
			}
			if rc == 4 {
				t.Errorf("exit 4 promises the resource does not exist; a %d says nothing of the sort\nstderr: %s", tc.status, stderr)
			}
		})
	}
}
