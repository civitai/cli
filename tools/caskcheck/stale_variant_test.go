package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 🔴 A CHECK THAT READS A STALE COPY IS GREEN ABOUT A STATE THAT NO LONGER
// EXISTS, AND THAT IS INDISTINGUISHABLE FROM BEING RIGHT.
//
// Measured on 2026-08-10, minutes after v0.1.92 published and the cask bumped,
// with `git ls-remote` giving the tap's true HEAD as df4801b:
//
//	curl                 .../homebrew-tap/HEAD/Casks/civitai.rb -> version "0.1.92"
//	curl --compressed    (same URL)                              -> version "0.1.91"
//	net/http             (adds Accept-Encoding: gzip itself)     -> version "0.1.91"
//	go run ./tools/caskcheck                                     -> OK: cask version 0.1.91 …
//
// raw.githubusercontent.com answers `vary: Authorization,Accept-Encoding` behind
// Fastly, so that one URL is one cached object PER ENCODING PER EDGE NODE, and
// the gzip one was a release behind. The expensive direction is the first test
// below: if a cask push introduces a BROKEN version, a run reading the previous
// variant validates the PREVIOUS, WORKING version's archive URLs and exits 0 —
// green straight through the outage this command exists to catch.
//
// The fix is not to prefer a variant. Staleness is per edge node — in the same
// minute `cache-yyz4539` served the gzip variant a release behind on a HIT while
// `cache-yyz4528` MISSed and fetched 0.1.92 from the origin — so "read the
// identity variant" is a different lottery ticket, not a fix. The cask is read
// at a resolved commit instead, whose bytes are immutable.
//
// # What the mutation battery actually measured, including one claim it refuted
//
// Leaf `--- FAIL` lines counted from `go test -v`, never an exit code; each
// mutation checksum-gated so an edit that failed to apply is reported broken
// rather than read as a survivor. Base is 45 `--- PASS`, 0 `--- FAIL`.
//
//	M1  revert: resolve nothing, read the mutable ref            21
//	M2  half-fix: mutable ref + `Accept-Encoding: identity`      21
//	M3  degraded mode: fall back to the mutable ref on failure    4
//	M4  the shipped default reverted to a /HEAD/ URL              1
//	M5  parser takes the first 40-hex run (a shifted sha)         8
//	M6  the tap ref lookup starts carrying the credential         1
//	M7  the ref-advertisement instrument check removed            2
//	M8  the provenance line stops naming the verbatim mode        1
//	M0  NULL MUTANT (comment only)                                0 — survives
//
// 🔴 M1 AND M2 KILL THE **IDENTICAL** 21 LEAVES, so no test here distinguishes
// the do-nothing regression from the prefer-a-variant half-fix. An earlier
// revision of this comment claimed the test below separated them; it does not,
// and the separation was never needed — both are one defect, reading a mutable
// ref. Stating the sets are identical is the honest version, and it is what
// stops someone counting two mutants as two independent pieces of evidence.
//
// 🔴 THE `mutableHits() == 0` ASSERTIONS ARE REDUNDANCY, NOT THE COVERAGE.
// Re-measured with all three of them deleted: M1 and M2 still kill the same 21
// leaves. They are kept because they state the rule structurally and would
// catch a future "read both and compare" design that the behavioural
// assertions would let through — but they must not be counted as what makes
// these tests work.

// encodingSplitTap is the live host's behaviour in a fixture: one path serving
// DIFFERENT bodies per Accept-Encoding, and an immutable per-commit path that
// cannot. Every mutable-ref hit is counted, so a test can assert the resolver
// never touched it.
type encodingSplitTap struct {
	srv *httptest.Server
	rec *recorder
}

func newEncodingSplitTap(t *testing.T, rec *recorder, pinned, identityVariant, gzipVariant, advertisement string) *encodingSplitTap {
	t.Helper()
	tap := &encodingSplitTap{rec: rec}
	tap.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rec.record(req)
		switch {
		case strings.Contains(req.URL.Path, "/info/refs"):
			rw.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte(advertisement))
		case strings.HasPrefix(req.URL.Path, "/"+tapCommitFixture+"/"):
			// Immutable by construction: one body, whatever the encoding.
			serveCask(rw, req, pinned)
		case strings.HasPrefix(req.URL.Path, "/HEAD/"):
			// Mutable, and sharded on Accept-Encoding exactly like the CDN.
			if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
				rw.Header().Set("Content-Encoding", "gzip")
				rw.WriteHeader(http.StatusOK)
				_, _ = rw.Write(gzipped(gzipVariant))
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte(identityVariant))
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(tap.srv.Close)
	return tap
}

func (t *encodingSplitTap) mutableHits() int { return t.rec.hitsForPathContaining("/HEAD/") }

// checkerFor wires a checker at this tap, taking the releases API and the
// archive host from an existing world.
func (t *encodingSplitTap) checkerFor(w *world) *checker {
	return &checker{
		caskURL:    t.srv.URL + "/" + caskCommitPlaceholder + "/Casks/civitai.rb",
		tapRefsURL: t.srv.URL + "/info/refs?service=git-upload-pack",
		latestURL:  w.api.URL + "/repos/civitai/cli/releases/latest",
		apiToken:   "fixture-token",
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// TestAStaleVariantCannotHideABrokenCask is the proven failure for this defect,
// and it is the OUTAGE shape rather than a cosmetic version mismatch.
//
// The tap's real HEAD names 0.1.92, whose archives are NOT downloadable — the
// 2026-08-09 state, the one thing this command exists to detect. The mutable
// ref still serves 0.1.91 to a gzip client, whose archives are all fine. A
// checker that reads the mutable ref therefore probes four URLs, gets four
// 200s, and exits 0 while `brew install` is broken for every user.
//
// The variants here reproduce the measured live split exactly — identity
// CURRENT, gzip a release behind — so this is the incident, not an invented
// shape. Note what that means for the VERSION assertion specifically: a fix
// that merely preferred the identity variant would satisfy it, because in this
// fixture the identity variant happens to be right. It is the next test that
// removes that safe harbour. (Both mutants die here anyway, on the recorded
// commit; see the file comment for why the kill sets are identical.)
func TestAStaleVariantCannotHideABrokenCask(t *testing.T) {
	// 0.1.91 is downloadable; 0.1.92 is the unpublished draft.
	w := newWorld(t, "0.1.92", []string{"0.1.90", "0.1.91"}, "v0.1.91")
	tap := newEncodingSplitTap(t, w.rec,
		caskFixture("0.1.92", w.releases.URL), // pinned: the truth, and it is broken
		caskFixture("0.1.92", w.releases.URL), // identity variant: current, as measured
		caskFixture("0.1.91", w.releases.URL), // gzip variant: a release behind
		refAdvertisement(tapCommitFixture),
	)

	rep, err := run(t, tap.checkerFor(w))
	if err == nil {
		t.Fatalf("the cask names 0.1.92, whose archives 404, and this run reported CLEAN.\n"+
			"That is the 2026-08-09 outage passing the detector built for it, because the bytes judged came from a\n"+
			"stale CDN variant naming the previous, working release. report=%+v", rep)
	}
	if !errors.Is(err, errArchivesNotDownloadable) {
		t.Fatalf("want the archives finding, got %v", err)
	}
	// The verdict has to be ABOUT the current cask. A run that went red for
	// some other reason while still reading 0.1.91 has not fixed anything.
	if rep == nil || rep.caskVersion != "0.1.92" {
		t.Fatalf("this run judged cask version %q; the tap's HEAD commit names 0.1.92, and any other value means the mutable ref was read", caskVersionOf(rep))
	}
	if tap.mutableHits() != 0 {
		t.Errorf("the mutable ref was requested %d time(s); it must not be read at all", tap.mutableHits())
	}
	if rep.caskCommit != tapCommitFixture {
		t.Errorf("report records commit %q, want %q — the verdict must say which snapshot it is about", rep.caskCommit, tapCommitFixture)
	}
}

// TestNeitherEncodingVariantOfTheMutableRefIsTrusted removes the safe harbour
// the test above leaves standing.
//
// 🔴 BOTH variants are deliberately WRONG here, and that is not an unrealistic
// fixture — it is the honest one. Which variant is stale is a property of the
// Fastly edge node that answers, measured to differ between two nodes in the
// same minute, so there is no variant a checker may trust. A fix that pinned
// `Accept-Encoding: identity` would read 0.1.90 here and go red on a healthy
// pipeline, which is the point: preferring a variant is not a fix, it is a
// different way to be wrong.
func TestNeitherEncodingVariantOfTheMutableRefIsTrusted(t *testing.T) {
	w := newWorld(t, "0.1.92", []string{"0.1.92"}, "v0.1.92")
	tap := newEncodingSplitTap(t, w.rec,
		caskFixture("0.1.92", w.releases.URL), // pinned: the truth
		caskFixture("0.1.90", w.releases.URL), // identity variant: stale one way
		caskFixture("0.1.91", w.releases.URL), // gzip variant: stale another way
		refAdvertisement(tapCommitFixture),
	)

	rep, err := run(t, tap.checkerFor(w))
	if err != nil {
		t.Fatalf("the tap's HEAD names 0.1.92 and 0.1.92 is downloadable, so this must be green: %v", err)
	}
	if rep.caskVersion != "0.1.92" {
		t.Fatalf("judged cask version %q, want 0.1.92 — %q is what the mutable ref serves, so this run read the cached copy",
			rep.caskVersion, rep.caskVersion)
	}
	if tap.mutableHits() != 0 {
		t.Errorf("the mutable ref was requested %d time(s); no encoding of it is evidence about the current cask", tap.mutableHits())
	}
	// POSITIVE CONTROL on the fixture itself: if the mutable path did not
	// actually serve two different bodies, this test would pass against a
	// checker that reads it, and would be proving nothing.
	idBody, gzBody := fetchVariants(t, tap.srv.URL+"/HEAD/Casks/civitai.rb")
	if idBody == gzBody {
		t.Fatal("PREMISE BROKEN: the fixture's mutable path served identical bodies to both encodings, so it does not mirror the CDN and this test cannot fail for the right reason")
	}
	for _, b := range []string{idBody, gzBody} {
		if strings.Contains(b, `version "0.1.92"`) {
			t.Fatal("PREMISE BROKEN: a mutable variant carries the CURRENT version, so reading it would pass and the test would be satisfied by the defect")
		}
	}
}

// fetchVariants asks one URL both ways, as the two CDN clients do.
func fetchVariants(t *testing.T, url string) (identity, gzip string) {
	t.Helper()
	get := func(ae string) string {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		if ae != "" {
			req.Header.Set("Accept-Encoding", ae)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		return string(b)
	}
	// "" lets Go's transport add gzip itself, which is the whole point.
	return get("identity"), get("")
}

func caskVersionOf(rep *report) string {
	if rep == nil {
		return "<no report>"
	}
	return rep.caskVersion
}

// TestTheShippedCaskURLIsPinnedToACommit pins the DEFAULT, because every other
// test in this package injects its own URLs and so says nothing about what a
// real run reads. Reverting the default to a mutable ref is the whole defect.
func TestTheShippedCaskURLIsPinnedToACommit(t *testing.T) {
	if !strings.Contains(defaultCaskURL, caskCommitPlaceholder) {
		t.Fatalf("defaultCaskURL = %q, which carries no %s placeholder — a real run would read a mutable ref, which measured a release behind on 2026-08-10",
			defaultCaskURL, caskCommitPlaceholder)
	}
	// A mutable ref segment must not survive alongside the placeholder.
	for _, mutable := range []string{"/HEAD/", "/main/", "/master/", "/refs/heads/"} {
		if strings.Contains(defaultCaskURL, mutable) {
			t.Errorf("defaultCaskURL = %q still names the mutable ref %q", defaultCaskURL, mutable)
		}
	}
	if !strings.HasPrefix(defaultTapRefsURL, "https://") || !strings.Contains(defaultTapRefsURL, "/info/refs") || !strings.Contains(defaultTapRefsURL, "service=git-upload-pack") {
		t.Errorf("defaultTapRefsURL = %q, want the git smart-HTTP ref advertisement — it is the one tap read that is served with `cache-control: no-cache` and behind no CDN", defaultTapRefsURL)
	}
	// Both must name the same repository, or a run pins the cask of one tap to
	// the HEAD of another and the whole guarantee is void.
	if !strings.Contains(defaultCaskURL, "civitai/homebrew-tap") || !strings.Contains(defaultTapRefsURL, "civitai/homebrew-tap") {
		t.Errorf("the cask URL (%q) and the ref URL (%q) must resolve the same repo", defaultCaskURL, defaultTapRefsURL)
	}
}

// TestAnUnresolvableTapIsLoudAndNeverFallsBack — "we could not resolve the tap"
// is the same state as "we could not read the cask": nothing was measured.
//
// 🔴 The fallback assertion is the important half. Reading the mutable ref when
// the lookup fails would be a degraded mode that reintroduces the defect on
// exactly the days something is wrong, and it would look like resilience.
func TestAnUnresolvableTapIsLoudAndNeverFallsBack(t *testing.T) {
	notAnAdvertisement := "<!DOCTYPE html><html><body>404 Not Found</body></html>"
	emptyRepo := func() string {
		line := "0000000000000000000000000000000000000000 capabilities^{}\x00multi_ack agent=git/github-fixture\n"
		return "001e# service=git-upload-pack\n0000" + fmt.Sprintf("%04x%s", len(line)+4, line) + "0000"
	}()

	for _, tc := range []struct {
		name string
		adv  string
		// refused points the checker at a dead port instead of the fixture.
		refused bool
		wantMsg string
	}{
		{name: "refs endpoint refused", refused: true, wantMsg: ""},
		{name: "response is not a ref advertisement", adv: notAnAdvertisement, wantMsg: "not a git ref advertisement"},
		{name: "advertisement names no HEAD", adv: emptyRepo, wantMsg: "names no HEAD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t, "0.1.92", []string{"0.1.92"}, "v0.1.92")
			tap := newEncodingSplitTap(t, w.rec,
				caskFixture("0.1.92", w.releases.URL),
				caskFixture("0.1.90", w.releases.URL),
				caskFixture("0.1.91", w.releases.URL),
				tc.adv,
			)
			c := tap.checkerFor(w)
			if tc.refused {
				c.tapRefsURL = "http://127.0.0.1:1/info/refs?service=git-upload-pack"
			}

			_, err := run(t, c)
			if err == nil {
				t.Fatal("a tap whose HEAD could not be resolved reported CLEAN — 'we could not ask' must never look like 'we asked and it was fine'")
			}
			if !errors.Is(err, errUnreachableTap) {
				t.Fatalf("want errUnreachableTap, got %v", err)
			}
			if got := classify(err); got != kindUnmeasurable {
				t.Errorf("classify = %q, want %q — a caller keys its wording on this", got, kindUnmeasurable)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("the message does not say what went wrong (want %q):\n%v", tc.wantMsg, err)
			}
			if tap.mutableHits() != 0 {
				t.Errorf("the run fell back to the mutable ref %d time(s) after failing to resolve HEAD; that degraded mode is the defect, wearing resilience as a hat", tap.mutableHits())
			}
		})
	}
}

// TestAVerbatimCaskURLSkipsTheRefLookup covers the fire drill in
// release-homebrew.yml, which points -cask-url at a fixture already pinned to a
// commit sha of THIS repo. Such a URL is immutable already; making it depend on
// resolving the tap would make the drill fail whenever the tap is unreachable,
// which is a state the drill is supposed to be able to rehearse.
func TestAVerbatimCaskURLSkipsTheRefLookup(t *testing.T) {
	w := newWorld(t, "0.1.92", []string{"0.1.92"}, "v0.1.92")
	c := w.checker()
	c.caskURL = w.tap.URL + "/some/pinned/fixture/civitai.rb" // no placeholder
	c.tapRefsURL = "http://127.0.0.1:1/info/refs"             // must never be reached

	rep, err := run(t, c)
	if err != nil {
		t.Fatalf("a verbatim -cask-url must be usable with no tap lookup at all: %v", err)
	}
	if n := w.rec.hitsForPathContaining("/info/refs"); n != 0 {
		t.Errorf("the ref advertisement was requested %d time(s) for a URL carrying no placeholder", n)
	}
	if rep.caskCommit != "" {
		t.Errorf("report records commit %q for a verbatim URL; there is no commit to record", rep.caskCommit)
	}
	// 🔴 It must SAY it was unpinned. A verbatim read carries none of the
	// freshness guarantee the pinned one does, so a run that took this path and
	// printed nothing about it would be the silent degrade.
	prov := readProvenance(rep)
	if !strings.Contains(prov, "verbatim") || !strings.Contains(prov, c.caskURL) {
		t.Errorf("provenance line = %q, want it to name the URL and say the read was unpinned", prov)
	}
	// And the pinned path must say the opposite, or "says which mode" is
	// satisfied by a constant.
	pinnedProv := readProvenance(&report{caskURL: "u", caskCommit: tapCommitFixture})
	if strings.Contains(pinnedProv, "verbatim") || !strings.Contains(pinnedProv, tapCommitFixture) {
		t.Errorf("pinned provenance line = %q, want it to name the commit and not claim a verbatim read", pinnedProv)
	}
}

// TestTheRefAdvertisementParserReadsTheRealShape.
//
// 🔴 The pkt-line LENGTH PREFIX is hex and runs straight into the sha
// (`00000159df4801…`), so an offset-based or greedy parser can read a shifted
// 40-character window and return a plausible wrong "commit" — which would then
// 404 at the raw host and read as an unreachable tap forever. Each row is a
// shape github.com really serves.
func TestTheRefAdvertisementParserReadsTheRealShape(t *testing.T) {
	const other = "0123456789abcdef0123456789abcdef01234567"

	pkt := func(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }

	for _, tc := range []struct {
		name string
		adv  string
		want string
	}{
		{
			name: "HEAD first, with capabilities",
			adv:  refAdvertisement(tapCommitFixture),
			want: tapCommitFixture,
		},
		{
			name: "a branch literally named HEAD must not be mistaken for it",
			adv: "001e# service=git-upload-pack\n0000" +
				pkt(tapCommitFixture+" HEAD\x00symref=HEAD:refs/heads/main\n") +
				pkt(other+" refs/heads/HEAD\n") + "0000",
			want: tapCommitFixture,
		},
		{
			name: "HEAD is not the first ref line",
			adv: "001e# service=git-upload-pack\n0000" +
				pkt(other+" refs/heads/topic\n") +
				pkt(tapCommitFixture+" HEAD\n") + "0000",
			want: tapCommitFixture,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				_, _ = rw.Write([]byte(tc.adv))
			}))
			t.Cleanup(srv.Close)
			c := &checker{tapRefsURL: srv.URL, client: &http.Client{Timeout: 5 * time.Second}}

			got, err := c.tapCommit(t.Context())
			if err != nil {
				t.Fatalf("tapCommit: %v", err)
			}
			if got != tc.want {
				t.Fatalf("resolved %q, want %q — a shifted window is a well-formed sha that names no commit", got, tc.want)
			}
		})
	}
}
