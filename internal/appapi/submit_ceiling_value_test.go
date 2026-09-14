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
		//
		// 🔴 UNIT-SPELLED FORMS ARE DELIBERATELY NOT POLICED, AND THE LIST NO
		// LONGER PRETENDS THEY ARE. It used to read
		// `10485760|20971520|10 ?MiB|10 ?MB` and then exempt matches with
		// `!strings.HasPrefix(m, "10 ")` — which disagreed with itself about one
		// number written two ways: the SPACED forms could never be reported while
		// the UNSPACED `10MiB` could. Neither half was reachable coverage.
		//
		// They are dropped rather than repaired because a MiB spelling here is
		// AMBIGUOUS, not merely unchecked. Enumerated over both files at this
		// revision: `10 MiB` appears in each, and in both it is pkgzip's per-FILE
		// cap, not this ceiling — alongside `2 MiB`, `50 MiB`, `200 MiB`, `32 MB`
		// and others, none of which are the submit body limit. A rule that
		// reported them would be wrong, and one that exempted them by prefix only
		// looks like a rule. The byte literal is the unambiguous spelling and is
		// what these surfaces quote; that is what this checks.
		suspicious := regexp.MustCompile(`\b(?:10485760|20971520)\b`)
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
				if m != want {
					t.Errorf("%s still quotes %q alongside the current ceiling %s — one of them is stale.",
						rel, m, want)
				}
			}
		}
	})

	// ── The BOUNDARY, driven through the REAL guard ────────────────────────
	//
	// 🔴 THIS CALLS SubmitVersion. The first version of this subtest asserted
	// `tc.size > MaxSubmitBodyBytes` — a RE-IMPLEMENTATION of the predicate in
	// the test, which mutating production could not affect. Measured: the `>=`
	// mutant SURVIVED it. An expectation derived from the implementation is not
	// a test of the implementation; the only thing that works here is running it.
	//
	// 🔴 THIS SUBTEST IS ZERO-PROVENANCE, AND THAT MAKES IT BLIND TO THE OPERATOR.
	// With Provenance{} the envelope is 19 bytes and base64 steps by 4, so body
	// sizes go … 10485759, 10485763 — nothing lands on 10485760, and `>` vs `>=`
	// is genuinely unobservable FROM HERE. What it pins is the reachable pair
	// either side of the ceiling, which catches a removed guard and a changed
	// constant.
	//
	// ⚠ AN EARLIER VERSION OF THIS COMMENT GENERALISED THAT INTO "there is no
	// input that distinguishes the two operators", AND THAT WAS FALSE. It
	// measured one provenance and spoke for all of them. The envelope depends on
	// the provenance, and commit + Dirty=true gives 96 bytes — ≡ 0 (mod 4) — so
	// the ceiling IS reachable, at a zip of 7,864,246 bytes, on exactly the
	// invocation `--allow-dirty` produces. The operator is pinned by
	// TestSubmitBodyExactlyAtCeilingIsRefused below, not here.
	t.Run("a body just under the ceiling is accepted; one step more is refused", func(t *testing.T) {
		// Find the largest zip length whose body stays strictly under the ceiling.
		// base64 is 3-byte granular, so not every body size is reachable — take
		// the boundary pair that IS.
		zipAt := 0
		for n := 0; ; n++ {
			if SubmitBodySize(n, Provenance{}) >= MaxSubmitBodyBytes {
				zipAt = n - 1
				break
			}
			if n > MaxSubmitBodyBytes {
				t.Fatal("CONTROL failure: never crossed the ceiling — SubmitBodySize is not growing")
			}
		}
		underBody := SubmitBodySize(zipAt, Provenance{})
		overBody := SubmitBodySize(zipAt+1, Provenance{})
		if !(underBody < MaxSubmitBodyBytes && overBody > MaxSubmitBodyBytes) {
			t.Fatalf("CONTROL failure: the bracket is wrong — %d bytes of zip gives a %d-byte body and "+
				"%d gives %d, against a %d ceiling", zipAt, underBody, zipAt+1, overBody, MaxSubmitBodyBytes)
		}

		// CONTROL on this subtest's own stated scope: with zero provenance the
		// ceiling must be UNREACHABLE, which is why this subtest cannot see the
		// operator. If it ever becomes reachable here, this subtest's comment is
		// wrong and it should pin the operator directly like the test below does.
		if overBody == MaxSubmitBodyBytes || underBody == MaxSubmitBodyBytes {
			t.Errorf("a zero-provenance body of exactly %d is now reachable (zip %d/%d). This subtest's "+
				"comment says it is not, and on that basis it does not pin `>` vs `>=`.",
				MaxSubmitBodyBytes, zipAt, zipAt+1)
		}

		srv, hits := acceptingSubmitServer(t)
		defer srv.Close()
		c := NewWithSource(srv.URL, civitai.StaticToken("t"), "/api/blocks/submit-version")

		if _, err := c.SubmitVersion(context.Background(), make([]byte, zipAt), "slug", "1.0.0", Provenance{}); err != nil {
			t.Errorf("a %d-byte body — strictly UNDER the %d ceiling — was REFUSED: %v\n"+
				"Only a body AT or over the ceiling may be refused; refusing one below it rejects a submit "+
				"nothing suggests the server would.", underBody, MaxSubmitBodyBytes, err)
		}
		if *hits != 1 {
			t.Errorf("the under-ceiling case contacted the server %d time(s), want 1 — it must actually upload", *hits)
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

// dirtyProvenance is the provenance `civitai app submit --allow-dirty` produces on
// a dirty tree: a real 40-hex commit plus Dirty=true. It is the ONLY one of the
// four shapes the CLI can send whose JSON envelope length is ≡ 0 (mod 4), which is
// what makes a body of exactly MaxSubmitBodyBytes reachable.
func dirtyProvenance() Provenance {
	dirty := true
	return Provenance{Commit: "0123456789abcdef0123456789abcdef01234567", Dirty: &dirty}
}

// TestSubmitBodyExactlyAtCeilingIsRefused pins `>=` against `>` at the ONE input
// that can distinguish them, driven through the real SubmitVersion.
//
// 🔴 WHY THIS EXISTS. #585 shipped with `len(body) > MaxSubmitBodyBytes` and a
// sibling subtest asserting in prose that the operator was unobservable — "there
// is no input that distinguishes the two operators". That generalised a
// measurement taken at Provenance{} to every provenance, and it is false. base64
// output is always a multiple of 4, so the body can land on the ceiling only when
// the envelope length is too. Measured through SubmitBodySize:
//
//	provenance                envelope  mod 4  zipLen hitting the ceiling exactly
//	none                            19      3  unreachable
//	commit only                     77      1  unreachable
//	commit + Dirty=false            97      1  unreachable
//	commit + Dirty=true             96      0  7864246
//
// So `civitai app submit --allow-dirty` on a 7,864,246-byte zip produced a body of
// exactly 10485760 and `>` let the whole thing upload. The `>` mutant SURVIVED the
// entire suite before this test; it must not now.
//
// ⚠ WHAT THIS TEST DOES NOT CLAIM. It does not assert the server rejects exactly
// the cap. There is an unresolved contradiction — Next.js's source reads
// `bytesRead > bodySizeLimit` (which would accept it), while an end-to-end
// measurement read 413 at exactly 10485760 — and neither can be re-measured from a
// unit test. This pins the CLI's chosen side of an ASYMMETRY: refusing one byte
// early costs an author a documented flag, accepting one byte too many costs the
// full upload and an error naming nothing about size.
func TestSubmitBodyExactlyAtCeilingIsRefused(t *testing.T) {
	prov := dirtyProvenance()

	// CONTROL: the fixture must actually sit ON the boundary, or every assertion
	// below is about an ordinary over-ceiling body and the operator is untested.
	const zipAtCeiling = 7864246
	if got := SubmitBodySize(zipAtCeiling, prov); got != MaxSubmitBodyBytes {
		t.Fatalf("CONTROL failure, not a finding: a %d-byte zip with --allow-dirty provenance gives a "+
			"%d-byte body, not exactly %d. The envelope arithmetic moved, so this test no longer sits on "+
			"the boundary and cannot distinguish `>` from `>=`. Re-derive the zip length: it is the n for "+
			"which SubmitBodySize(n, dirtyProvenance()) == MaxSubmitBodyBytes.",
			zipAtCeiling, got, MaxSubmitBodyBytes)
	}
	// CONTROL: and the provenance must be the one that survives sanitisation, or
	// the envelope silently collapses to the zero-provenance 19 bytes.
	if commit, dirty := prov.sanitised(); commit == "" || dirty == nil || !*dirty {
		t.Fatalf("CONTROL failure, not a finding: dirtyProvenance() does not survive sanitised() "+
			"(commit=%q dirty=%v), so the body carries the zero-provenance envelope and the boundary "+
			"is unreachable", commit, dirty)
	}

	srv, hits := acceptingSubmitServer(t)
	defer srv.Close()
	c := NewWithSource(srv.URL, civitai.StaticToken("t"), "/api/blocks/submit-version")

	_, err := c.SubmitVersion(context.Background(), make([]byte, zipAtCeiling), "slug", "1.0.0", prov)
	if err == nil {
		t.Fatalf("a submit body of EXACTLY %d bytes was uploaded. The guard must refuse at the ceiling, "+
			"not one byte above it: the comparison is `>=`, and `>` lets this exact body through. This is "+
			"reachable in production — it is what `civitai app submit --allow-dirty` sends for a "+
			"%d-byte zip.", MaxSubmitBodyBytes, zipAtCeiling)
	}
	if !errors.Is(err, ErrBundleTooLarge) {
		t.Fatalf("the at-ceiling body was refused, but not by the size guard — so this test would stay "+
			"green with the guard deleted: %v", err)
	}
	// The refusal must be a PREFLIGHT, which is the entire point of the guard.
	if *hits != 0 {
		t.Errorf("the at-ceiling case contacted the server %d time(s); it must refuse before uploading", *hits)
	}
	// The message must report the real body size, not the zip.
	if !strings.Contains(err.Error(), strconv.Itoa(MaxSubmitBodyBytes)) {
		t.Errorf("the refusal names neither the body size nor the ceiling (both are %d here):\n  %v",
			MaxSubmitBodyBytes, err)
	}

	// 🔴 NEGATIVE CONTROL, and it is the half that makes the assertion above mean
	// `>=` rather than "refuses everything". One base64 quantum BELOW the ceiling
	// must still upload — `>=` accepts it and a guard that had drifted to `>=-4`
	// or to an unconditional refusal would not.
	belowZip := zipAtCeiling - 3
	belowBody := SubmitBodySize(belowZip, prov)
	if belowBody >= MaxSubmitBodyBytes {
		t.Fatalf("CONTROL failure, not a finding: the below-ceiling fixture is %d bytes, not under %d",
			belowBody, MaxSubmitBodyBytes)
	}
	if _, err := c.SubmitVersion(context.Background(), make([]byte, belowZip), "slug", "1.0.0", prov); err != nil {
		t.Errorf("a %d-byte body — under the %d ceiling — was refused: %v\n"+
			"The guard must be `>=` on the ceiling, not a blanket refusal.", belowBody, MaxSubmitBodyBytes, err)
	}
	if *hits != 1 {
		t.Errorf("the under-ceiling case reached the server %d time(s), want 1 — it must actually upload", *hits)
	}
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
