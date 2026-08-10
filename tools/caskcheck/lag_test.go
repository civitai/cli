package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file covers the two things added on top of the 2026-08-09 invariant: the
// LAG finding, and the CLASSIFICATION a caller needs in order to put different
// words in front of a human for "users are broken", "we could not ask" and "the
// pipeline did not move the cask".
//
// 🔴 The lag finding is the one most able to become a permanently red run, which
// this repo already knows trains everyone to click through. So the tests below
// weight the GREEN side deliberately: three separate ways of not being able to
// ask, and a boundary pinned from both sides, against one red case.

// lagWorld builds a checker whose clock is fixed, so a threshold is exercised
// without sleeping and a run in 2027 measures the same thing as a run today.
func lagWorld(t *testing.T, caskVersion string, published []string, latestTag string, publishedAgo, threshold time.Duration) (*world, *checker) {
	t.Helper()
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	w := newWorldAt(t, caskVersion, published, latestTag, base.Add(-publishedAgo))
	c := w.checker()
	c.lagThreshold = threshold
	c.now = func() time.Time { return base }
	return w, c
}

// TestACaskStillLaggingADayAfterThePublishIsAFinding is the new detector's
// proven failure. The state: v0.1.91 is PUBLISHED (so its archives are
// downloadable and nothing is broken for users), and the cask still says 0.1.90
// a day later — which can only mean the `release: published` job that is the one
// thing allowed to move the cask never moved it.
func TestACaskStillLaggingADayAfterThePublishIsAFinding(t *testing.T) {
	_, c := lagWorld(t, "0.1.90", []string{"0.1.90", "0.1.91"}, "v0.1.91", 25*time.Hour, 24*time.Hour)

	rep, err := run(t, c)
	if err == nil {
		t.Fatalf("a cask that never followed a release published 25h ago went unreported; report=%+v", rep)
	}
	if !errors.Is(err, errCaskLags) {
		t.Fatalf("want errCaskLags, got %v", err)
	}
	if got := classify(err); got != kindLagging {
		t.Errorf("classify = %q, want %q — a caller keys its wording on this", got, kindLagging)
	}

	// 🔴 A lag must never borrow the outage's words. `brew install` works fine
	// in this state, and telling an operator at 2am that every user is broken
	// when they are not is how the next real one gets ignored.
	if errors.Is(err, errArchivesNotDownloadable) {
		t.Error("a lagging cask was classified as the archives-404 outage; they are different severities and must not share a sentinel")
	}
	if strings.Contains(err.Error(), "failing for every user") {
		t.Errorf("the lag message claims users are broken, which is false here:\n%v", err)
	}
	// It has to say what to look at. Both versions and the workflow that owns
	// the push are the whole diagnosis.
	for _, want := range []string{"0.1.90", "v0.1.91", "release-homebrew.yml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the lag message does not mention %q, so it does not say what is wrong:\n%v", want, err)
		}
	}
}

// TestTheLagThresholdIsPinnedFromBOTHSides. A threshold asserted on one side
// only is satisfied by a check that fires always, or never.
func TestTheLagThresholdIsPinnedFromBOTHSides(t *testing.T) {
	const threshold = 24 * time.Hour

	t.Run("exactly at the threshold is GREEN", func(t *testing.T) {
		_, c := lagWorld(t, "0.1.90", []string{"0.1.90", "0.1.91"}, "v0.1.91", threshold, threshold)
		if _, err := run(t, c); err != nil {
			t.Fatalf("a cask lagging by exactly the grace window must be green: %v", err)
		}
	})

	t.Run("one second past the threshold is a FINDING", func(t *testing.T) {
		_, c := lagWorld(t, "0.1.90", []string{"0.1.90", "0.1.91"}, "v0.1.91", threshold+time.Second, threshold)
		_, err := run(t, c)
		if !errors.Is(err, errCaskLags) {
			t.Fatalf("one second past the window must be a finding, got %v", err)
		}
	})

	// The shipped default is the number the README-facing reasoning is written
	// about; an unpinned constant is a claim nothing holds. Measured basis for
	// 24h is in the package comment.
	if defaultLagThreshold != 24*time.Hour {
		t.Fatalf("defaultLagThreshold = %v, want 24h — the measurement behind that number is in the package comment; move both or neither", defaultLagThreshold)
	}
	// And a zero/negative flag value must not silently disable the check.
	c := &checker{}
	if c.lagAfter() != defaultLagThreshold {
		t.Errorf("an unset lag threshold resolved to %v, want the default — 0 must not read as 'never lags'", c.lagAfter())
	}
}

// TestTheNormalWindowRightAfterAPublishStaysGREEN pins the non-decision the
// original check made: between a tag push and the maintainer publishing, and for
// a day after, a cask naming the previous version is correct behaviour.
func TestTheNormalWindowRightAfterAPublishStaysGREEN(t *testing.T) {
	_, c := lagWorld(t, "0.1.90", []string{"0.1.90", "0.1.91"}, "v0.1.91", 90*time.Minute, 24*time.Hour)

	rep, err := run(t, c)
	if err != nil {
		t.Fatalf("a cask 90 minutes behind a publish is the normal path and must be green: %v", err)
	}
	if !rep.lagEvaluated {
		t.Error("the lag question was answered, so the report must say it was ASKED — otherwise green here is indistinguishable from green-by-not-looking")
	}
	if !strings.Contains(rep.lagWhy, "grace window") {
		t.Errorf("lagWhy = %q, want it to name the reason this is green", rep.lagWhy)
	}
}

// TestACaskMatchingTheLatestPublishedReleaseIsGREEN — the steady state.
func TestACaskMatchingTheLatestPublishedReleaseIsGREEN(t *testing.T) {
	_, c := lagWorld(t, "0.1.91", []string{"0.1.90", "0.1.91"}, "v0.1.91", 400*time.Hour, 24*time.Hour)

	rep, err := run(t, c)
	if err != nil {
		t.Fatalf("a cask naming the latest published release must be green however old that release is: %v", err)
	}
	if !rep.lagEvaluated {
		t.Error("want the lag question recorded as asked")
	}
}

// TestALagFindingNeedsAKnownPublishTime is the "we could not ask" half, and it
// covers the two ways the answer can be missing.
//
// 🔴 The second subtest is the one that matters. `published_at` is a POINTER in
// the payload struct precisely so an absent key stays absent: a value type would
// decode a missing key to the zero time, i.e. 1 January year 1, i.e. maximally
// stale — and the check would manufacture a lag finding out of a field the
// server never sent, on every run, forever.
func TestALagFindingNeedsAKnownPublishTime(t *testing.T) {
	t.Run("releases API unreadable", func(t *testing.T) {
		_, c := lagWorld(t, "0.1.90", []string{"0.1.90", "0.1.91"}, "v0.1.91", 400*time.Hour, 24*time.Hour)
		c.latestURL = "http://127.0.0.1:1/nope" // guaranteed refused

		rep, err := run(t, c)
		if err != nil {
			t.Fatalf("an unreadable releases API must not become a lag finding — the invariant does not depend on it: %v", err)
		}
		if rep.lagEvaluated {
			t.Error("the lag question was NOT answerable, so the report must not claim it was evaluated")
		}
		if !strings.Contains(rep.lagWhy, "not evaluated") {
			t.Errorf("lagWhy = %q — a run that could not ask has to say so, not print a line that reads as 'checked, fine'", rep.lagWhy)
		}
	})

	t.Run("payload carries no published_at", func(t *testing.T) {
		// newWorld (no publish time) plus a threshold of one nanosecond: if an
		// absent key decoded to the zero time, EVERY run would be a lag finding.
		w := newWorld(t, "0.1.90", []string{"0.1.90", "0.1.91"}, "v0.1.91")
		c := w.checker()
		c.lagThreshold = time.Nanosecond

		rep, err := run(t, c)
		if err != nil {
			t.Fatalf("a release payload with no publish time must not produce a lag finding: %v", err)
		}
		if rep.lagEvaluated {
			t.Error("no publish time means the question was not answerable")
		}
		if !strings.Contains(rep.lagWhy, "no publish time") {
			t.Errorf("lagWhy = %q, want it to name the missing field", rep.lagWhy)
		}
	})
}

// TestTheOutageOutranksTheLag. Both conditions hold in the 2026-08-09 state
// (the cask is ahead of the latest PUBLISHED release, and its archives 404), and
// only one of them is worth waking someone for.
func TestTheOutageOutranksTheLag(t *testing.T) {
	_, c := lagWorld(t, "0.1.91", []string{"0.1.90"}, "v0.1.90", 400*time.Hour, 24*time.Hour)

	_, err := run(t, c)
	if err == nil {
		t.Fatal("the outage state went unreported")
	}
	if !errors.Is(err, errArchivesNotDownloadable) {
		t.Fatalf("want the archives finding to win, got %v", err)
	}
	if errors.Is(err, errCaskLags) {
		t.Error("the outage was reported as a lag — the softer of two simultaneous findings must never be the one that surfaces")
	}
	if got := classify(err); got != kindBroken {
		t.Errorf("classify = %q, want %q", got, kindBroken)
	}
}

// TestClassifyIsKeyedOnSentinelsAndIsAClosedSet.
//
// Every branch is an errors.Is on a sentinel, never a match on message text —
// a text match is a guard that a reworded message walks straight past. The
// default arm is a NAMED state rather than a fallback into some other kind, so
// a failure path added later cannot silently inherit another kind's wording.
func TestClassifyIsKeyedOnSentinelsAndIsAClosedSet(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"green", nil, kindGreen},
		{"an error carrying no sentinel", errors.New("x"), kindUnclassified},
		{"archives 404 (sentinel)", errors.Join(errArchivesNotDownloadable, errors.New("detail")), kindBroken},
		{"wrapped archives 404", errors.New("outer: " + errArchivesNotDownloadable.Error()), kindUnclassified},
		{"lagging", errors.Join(errCaskLags, errors.New("detail")), kindLagging},
		{"unreachable tap", errors.Join(errUnreachableTap, errors.New("detail")), kindUnmeasurable},
		{"nothing checked", errors.Join(errNothingChecked, errors.New("detail")), kindUnmeasurable},
		{"unreadable cask", errors.Join(errUnreadableCask, errors.New("detail")), kindUnmeasurable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.err); got != tc.want {
				t.Errorf("classify(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}

	// 🔴 "wrapped archives 404" above is the anti-spelling control: an error
	// whose TEXT is the sentinel's but which does not wrap it must NOT classify
	// as broken. If classify is ever rewritten as strings.Contains, that row
	// goes red.

	// Every kind must be non-empty and pairwise distinct, or the assertions
	// above are satisfiable by two kinds collapsing into one.
	all := []string{kindGreen, kindBroken, kindLagging, kindUnmeasurable, kindUnclassified}
	seen := map[string]bool{}
	for _, k := range all {
		if k == "" {
			t.Fatal("a kind constant is empty — every comparison against it is then trivially satisfied")
		}
		if seen[k] {
			t.Fatalf("kind %q is duplicated; two verdicts would be indistinguishable to a caller", k)
		}
		seen[k] = true
	}
}

// TestTheKindFileCarriesTheVerdict. The kind does NOT ride the exit code, for a
// measured reason recorded in the package comment: on go1.25.12 `go run`
// collapses every non-zero exit to 1. So the file is the carrier, and a caller
// that reads no file has learned "this did not reach its own exit path".
func TestTheKindFileCarriesTheVerdict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kind")
	if err := writeKind(path, kindLagging); err != nil {
		t.Fatalf("writeKind: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != kindLagging {
		t.Errorf("kind file holds %q, want %q", got, kindLagging)
	}

	// An unset path is a no-op, not an error: the command has to stay usable by
	// hand.
	if err := writeKind("", kindGreen); err != nil {
		t.Errorf("an empty kind-file path must be a no-op, got %v", err)
	}

	// Positive control on the writer itself — an unwritable path must report,
	// not swallow. Without this, "writeKind returned nil" is equally consistent
	// with a writer that never writes anything.
	if err := writeKind(filepath.Join(t.TempDir(), "no-such-dir", "kind"), kindGreen); err == nil {
		t.Error("writeKind reported success for a path it cannot have written")
	}
}

// TestEveryKindIsHandledByTheWorkflowThatReadsIt is a cross-file seam guard.
//
// The kinds are produced here and consumed in release-homebrew.yml, and neither
// file's tests can see the other. A kind added here and not handled there
// reaches an operator as whatever the workflow's fallback says, which is exactly
// the "the notification layer swallowed the distinction" failure this pair
// exists to prevent.
//
// Its limits, stated rather than implied: it checks the FORWARD direction only
// (every Go kind appears in the workflow), and appearing is not the same as
// being branched on. The behavioural half is the drill — see the workflow's
// `drill` input.
func TestEveryKindIsHandledByTheWorkflowThatReadsIt(t *testing.T) {
	const wf = "../../.github/workflows/release-homebrew.yml"
	b, err := os.ReadFile(wf)
	if err != nil {
		t.Fatalf("could not read %s: %v", wf, err)
	}
	// Positive control: a truncated or empty read would make every Contains
	// below fail, but an EMPTY file would make this test's failure look like a
	// missing kind rather than a missing file.
	if len(b) < 2000 {
		t.Fatalf("PREMISE BROKEN: %s is only %d bytes — this guard is reading the wrong thing", wf, len(b))
	}
	body := string(b)
	for _, k := range []string{kindGreen, kindBroken, kindLagging, kindUnmeasurable, kindUnclassified} {
		if !strings.Contains(body, k) {
			t.Errorf("kind %q is produced by caskcheck and never named in %s — an operator would get the fallback wording for it", k, wf)
		}
	}
}

// TestProbeOKIsNarrow keeps the one predicate the whole invariant rests on
// honest. A 404 is the outage; a 403 (rate limit) or a 5xx is not a downloadable
// archive either, and none of them may read as ok.
func TestProbeOKIsNarrow(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError, http.StatusFound, http.StatusUnauthorized} {
		if (probe{status: code}).ok() {
			t.Errorf("HTTP %d read as a downloadable archive", code)
		}
	}
	for _, code := range []int{http.StatusOK, http.StatusPartialContent} {
		if !(probe{status: code}).ok() {
			t.Errorf("HTTP %d must be ok, or the green path is unreachable", code)
		}
	}
}
