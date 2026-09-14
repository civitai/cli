package appapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestSubmitCeilingValueAndBoundary pins the two things every other test in this
// package is structurally blind to: the constant's VALUE, and the comparison's
// BOUNDARY.
//
// 🔴 WHY THEY WERE BLIND. Round 1 of #585's audit ran two mutations and both
// SURVIVED the whole suite:
//
//	MaxSubmitBodyBytes  10485760 -> 20971520      SURVIVED
//	len(body) > Max…    >        -> >=            SURVIVED
//
// The first survives because every fixture is DERIVED from the constant
// (`make([]byte, MaxSubmitBodyBytes)`, `(MaxSubmitBodyBytes/4)*3`), so doubling
// it moves the fixtures with it and nothing can see the change — the
// "fixture that can only ever produce the constant's own value" blind spot. The
// second survives because no fixture lands anywhere near the boundary.
//
// Both matter. The value is what three documentation surfaces quote literally,
// and the operator decides whether a body of EXACTLY the ceiling is refused.
func TestSubmitCeilingValueAndBoundary(t *testing.T) {
	// ── The VALUE, against a literal, not against itself ────────────────────
	//
	// 10 * 1024 * 1024 is Next.js's DEFAULT_BODY_CLONE_SIZE_LIMIT, which applies
	// because civitai's proxy matches /api/v1/:path* and sets no override. It is
	// written out here rather than as the constant so this assertion cannot be
	// satisfied by the thing it is checking.
	t.Run("the constant is the framework default, written independently", func(t *testing.T) {
		const nextDefaultBodyCloneSizeLimit = 10 * 1024 * 1024
		if MaxSubmitBodyBytes != nextDefaultBodyCloneSizeLimit {
			t.Errorf("MaxSubmitBodyBytes = %d, want %d (Next.js proxyClientMaxBodySize default).\n"+
				"If civitai raised the limit, this is the deliberate place to change it — and "+
				"claudedocs/decisions/31 and README both quote the number literally, so change those too. "+
				"If you changed it for any other reason, do not: the whole justification for vendoring it "+
				"at all is that it is SOURCED rather than chosen.",
				MaxSubmitBodyBytes, nextDefaultBodyCloneSizeLimit)
		}
	})

	// ── The DOCUMENTATION, so the number and its prose cannot diverge ───────
	//
	// 🔴 THE SURFACES QUOTE IT AS A LITERAL. Nothing else ties them to the
	// constant, so the code could move and the published contract stay behind —
	// which is exactly the multi-surface staleness AGENTS.md warns about.
	t.Run("every surface quoting the ceiling quotes the current one", func(t *testing.T) {
		root := repoRootForTest(t)
		want := strconv.Itoa(MaxSubmitBodyBytes)
		// A number of this shape in these files is the ceiling; there is no
		// other 8-digit literal starting 104 or 209 in them today.
		suspicious := regexp.MustCompile(`\b(?:10485760|20971520|10 ?MiB|10 ?MB)\b`)
		for _, rel := range []string{
			"README.md",
			"claudedocs/decisions/31-pkgzip-caps-are-not-a-server-mirror.md",
		} {
			b, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			body := string(b)
			if !strings.Contains(body, want) {
				t.Errorf("%s quotes no %s. It documents the submit-body ceiling, so if the constant moved, "+
					"this surface did not move with it.", rel, want)
				continue
			}
			for _, m := range suspicious.FindAllString(body, -1) {
				if m != want && !strings.HasPrefix(m, "10 ") {
					t.Errorf("%s still quotes %q alongside the current ceiling %s — one of them is stale.",
						rel, m, want)
				}
			}
		}
	})

	// ── The BOUNDARY, driven through the REAL guard ────────────────────────
	//
	// Next.js refuses on `bytesRead > bodySizeLimit`, so a body of EXACTLY the
	// ceiling is delivered intact and must NOT be refused here. `>=` would
	// silently reject a submit the server accepts.
	//
	// 🔴 THIS CALLS SubmitVersion. The first version of this subtest asserted
	// `tc.size > MaxSubmitBodyBytes` — a RE-IMPLEMENTATION of the predicate in
	// the test, which mutating production could not affect. Measured: the `>=`
	// mutant SURVIVED it. An expectation derived from the implementation is not
	// a test of the implementation; the only thing that works here is running it.
	//
	// 🔴 AND EVEN SO, `>` vs `>=` IS UNOBSERVABLE THROUGH THIS API — SAID PLAINLY
	// RATHER THAN LEFT AS FALSE COVERAGE. Measured: the envelope is 19 bytes and
	// base64 steps by 4, so body sizes go … 10485759, 10485763 — nothing lands on
	// 10485760 at all. There is no input that distinguishes the two operators, so
	// the `>=` mutant survives this subtest too, and that is a fact about the
	// arithmetic rather than a hole in the test. `>` is still the correct operator
	// (Next.js refuses on `bytesRead > bodySizeLimit`), it is simply not reachable
	// from here. What IS pinned below is the reachable boundary — the largest body
	// accepted and the next one refused — which catches a removed guard and a
	// changed constant. The envelope assertion guards the claim itself: if a
	// future envelope makes the ceiling reachable, this stops being true and you
	// should come back and pin the operator directly.
	t.Run("a body of exactly the ceiling is accepted; one byte more is refused", func(t *testing.T) {
		// Find the largest zip length whose body lands exactly on the ceiling or
		// just under it. base64 is 3-byte granular, so not every body size is
		// reachable — take the boundary pair that IS.
		zipAt := 0
		for n := 0; ; n++ {
			if SubmitBodySize(n, Provenance{}) > MaxSubmitBodyBytes {
				zipAt = n - 1
				break
			}
			if n > MaxSubmitBodyBytes {
				t.Fatal("CONTROL failure: never crossed the ceiling — SubmitBodySize is not growing")
			}
		}
		underBody := SubmitBodySize(zipAt, Provenance{})
		overBody := SubmitBodySize(zipAt+1, Provenance{})
		if !(underBody <= MaxSubmitBodyBytes && overBody > MaxSubmitBodyBytes) {
			t.Fatalf("CONTROL failure: the bracket is wrong — %d bytes of zip gives a %d-byte body and "+
				"%d gives %d, against a %d ceiling", zipAt, underBody, zipAt+1, overBody, MaxSubmitBodyBytes)
		}

		// The claim above, pinned. If this ever fails, a body CAN land exactly on
		// the ceiling and `>` vs `>=` became observable — pin the operator here.
		if underBody == MaxSubmitBodyBytes {
			t.Errorf("a body of exactly %d is now reachable (zip %d). The comment above says it is not, "+
				"and on that basis this test does NOT pin `>` vs `>=`. It can now: assert that this exact "+
				"size is ACCEPTED, which `>=` would refuse.", MaxSubmitBodyBytes, zipAt)
		}

		srv, hits := acceptingSubmitServer(t)
		defer srv.Close()
		c := NewWithSource(srv.URL, civitai.StaticToken("t"), "/api/blocks/submit-version")

		if _, err := c.SubmitVersion(context.Background(), make([]byte, zipAt), "slug", "1.0.0", Provenance{}); err != nil {
			t.Errorf("a %d-byte body — at or under the %d ceiling — was REFUSED: %v\n"+
				"The comparison must be `>`, not `>=`: Next.js refuses on `bytesRead > bodySizeLimit`, so a "+
				"body of exactly the ceiling is delivered intact, and refusing it rejects a submit the "+
				"server accepts.", underBody, MaxSubmitBodyBytes, err)
		}
		if *hits != 1 {
			t.Errorf("the at-ceiling case contacted the server %d time(s), want 1 — it must actually upload", *hits)
		}

		before := *hits
		_, err := c.SubmitVersion(context.Background(), make([]byte, zipAt+1), "slug", "1.0.0", Provenance{})
		if err == nil {
			t.Errorf("a %d-byte body — one step OVER the %d ceiling — was accepted; the guard is gone",
				overBody, MaxSubmitBodyBytes)
		} else if !errors.Is(err, ErrBundleTooLarge) {
			t.Errorf("the over-ceiling body was refused, but not by the size guard: %v", err)
		}
		if *hits != before {
			t.Errorf("the over-ceiling case contacted the server; the refusal must be a PREFLIGHT")
		}
	})
}

// acceptingSubmitServer returns a server that accepts any submit, plus a counter
// of how many requests reached it — the positive control for "did this actually
// upload", which a bare error check cannot answer.
func acceptingSubmitServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"1"}`)
	}))
	return srv, &hits
}

// repoRootForTest walks up from the test's working directory to the module root.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the module root — every assertion that needs it is vacuous")
	return ""
}
