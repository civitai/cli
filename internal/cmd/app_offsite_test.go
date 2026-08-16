package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#422 outcome 2 — the OFFSITE refusal.
//
// The regression risk this file exists to hold down is NOT the new message. It
// is the old one: `app listing` / `app status <slug>` for an ONSITE app that has
// simply never been submitted must keep saying exactly what it says today, and
// so must every case where the diagnostic probe cannot answer. A probe whose own
// failure replaces a real error with a confident wrong one is worse than the
// error it replaced, so most of the cases below assert the UNCHANGED string.
//
// 🔴 THE FIXTURES ARE PAIRWISE DISTINCT ON EVERY FIELD THE ASSERTIONS READ, and
// that is deliberate (#422's own warning about fixtures that can only produce
// the expected value). The offsite rows differ from the onsite row in `kind`
// (the branch) AND in `kindData` (the rendered external URL), and the two
// offsite rows differ from EACH OTHER in that URL — so a mutant that hardcodes
// either the branch or the URL literal is killed rather than surviving green.
//
// 🔴 AND DISTINCT ON THE SLUG TOO, which the first cut missed. The store detail
// body echoes a `slug`, and the fixtures set it equal to the slug the caller
// typed — so swapping the message's `%s` from the caller's slug to the server's
// `d.Slug` SURVIVED a fully green suite. It is exactly the case a fixture whose
// two fields can only hold one value cannot see. serverEchoSlug now makes the
// server's echo a value the caller's slug is not even a SUBSTRING of, and
// TestOffsiteFixturesAreDistinctOnSlug is the control that keeps it that way.

const (
	// offsiteSlugA/B are two DIFFERENT offsite apps with two DIFFERENT external
	// targets. Neither URL appears as a literal anywhere in non-test source —
	// checked by TestOffsiteRefusalRendersTheAppsOwnExternalURL, which is the
	// control that the message really reads the probe's payload.
	offsiteSlugA = "radio-zzz"
	offsiteURLA  = "https://radio.zzz-fixture-a.test/launch"
	offsiteSlugB = "comfy-zzz"
	offsiteURLB  = "https://comfy.zzz-fixture-b.test/open"
	// onsiteSlug is an app that EXISTS and is onsite — the app-was-never-
	// submitted case, which must keep today's message.
	onsiteSlug = "gen-matrix-zzz"
)

// appProbeServer is a fake civitai.com that answers the submissions route with
// an EMPTY list (the #422 wall) and the store detail route from a per-slug
// table, counting how many times each is hit.
type appProbeServer struct {
	*httptest.Server
	// appCalls counts GET /api/v1/apps/{slug} — the diagnostic probe. On every
	// SUCCESSFUL command it must stay 0.
	appCalls atomic.Int32
	subCalls atomic.Int32
}

// serverEchoSlug is the `slug` the fake store route puts in the RESPONSE, and it
// is deliberately NOT the slug the caller asked about — nor a string that
// contains it. Every refusal must render the slug the USER typed (the one their
// next command will work with); a body echoing the same value made that
// unobservable, and the `d.Slug` mutant survived. The `zzz-srv-` prefix and the
// dropped `-zzz` suffix are what make containment impossible in both directions.
func serverEchoSlug(slug string) string {
	return "zzz-srv-" + strings.TrimSuffix(slug, "-zzz")
}

// appDetailJSON renders a store-detail body. The onsite and offsite shapes carry
// DIFFERENT kindData, mirroring the backend's discriminated union.
func appDetailJSON(slug, kind, externalURL string) string {
	echo := serverEchoSlug(slug)
	if kind == "onsite" {
		return fmt.Sprintf(`{"id":"apl_%s","slug":%q,"kind":"onsite","name":"Onsite %s",
			"iconUrl":"https://img.zzz/icon-onsite.png","coverUrl":"https://img.zzz/cover-onsite.png",
			"kindData":{"kind":"onsite","appBlockId":"apb_zzz_onsite","hasPage":true,"liveUrl":"https://%s.civit.ai/"}}`,
			slug, echo, slug, slug)
	}
	// externalUrl goes through jsonString (json.Marshal), NOT %q: Go's %q emits
	// `\x1b` and `\a` for ESC and BEL, which are not JSON escapes at all, so a
	// fixture carrying a control character would arrive as an unparseable body
	// and the probe would collapse to "could not answer" — a green that means
	// nothing. json.Marshal emits the spec-compliant \uXXXX form.
	return fmt.Sprintf(`{"id":"apl_%s","slug":%q,"kind":%q,"name":"Offsite %s",
		"iconUrl":"https://img.zzz/icon-%s.png","coverUrl":"https://img.zzz/cover-%s.png",
		"kindData":{"kind":%q,"subKind":"connect","externalUrl":%s,"connectClientId":"cc_zzz"}}`,
		slug, echo, kind, slug, slug, slug, kind, jsonString(externalURL))
}

// TestOffsiteFixturesAreDistinctOnSlug is the control ON the fixtures. Every
// assertion that a refusal names the caller's slug is vacuous unless the server
// echoes a DIFFERENT one, and "different" is not enough — a `Contains` assertion
// passes for any echo the caller's slug is a substring of, which is what a naive
// `slug + "-server"` echo would have been. This fails before any of those
// assertions can pass for the wrong reason.
func TestOffsiteFixturesAreDistinctOnSlug(t *testing.T) {
	for _, slug := range []string{offsiteSlugA, offsiteSlugB, onsiteSlug} {
		echo := serverEchoSlug(slug)
		if echo == slug || strings.Contains(echo, slug) || strings.Contains(slug, echo) {
			t.Errorf("the server echo %q must not equal or contain the caller's slug %q (nor be contained by it) — "+
				"otherwise every `the message names the slug` assertion passes for a message rendering the server's value",
				echo, slug)
		}
	}
}

// newAppProbeServer wires the fake and the environment. apps maps a slug to the
// raw detail body the store route answers with; a slug that is absent 404s,
// which is the "no such app" case.
func newAppProbeServer(t *testing.T, apps map[string]string) *appProbeServer {
	t.Helper()
	ps := &appProbeServer{}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			ps.subCalls.Add(1)
			if r.URL.Query().Get("id") != "" {
				// The `--id` spelling 404s natively (there is no such publish
				// request), which is a different arm of submissionsError.
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"submissions":[]}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			ps.appCalls.Add(1)
			slug := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
			body, ok := apps[slug]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not found"}`))
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(ps.Close)
	listingEnv(t, ps.URL)
	return ps
}

// bothOffsiteAndOnsite is the fixture table every case below shares.
func bothOffsiteAndOnsite() map[string]string {
	return map[string]string{
		offsiteSlugA: appDetailJSON(offsiteSlugA, "offsite", offsiteURLA),
		offsiteSlugB: appDetailJSON(offsiteSlugB, "offsite", offsiteURLB),
		onsiteSlug:   appDetailJSON(onsiteSlug, "onsite", ""),
	}
}

// ---------------------------------------------------------------------------
// The new behaviour
// ---------------------------------------------------------------------------

// TestAppListingOffsiteRefusesPrecisely is #422's `app listing` half: the app
// exists, its store listing is serving an icon and a cover right now, and the
// CLI cannot address it. The message must say that and must NOT name
// `app submit`, which cannot succeed for an offsite app.
func TestAppListingOffsiteRefusesPrecisely(t *testing.T) {
	ps := newAppProbeServer(t, bothOffsiteAndOnsite())

	_, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	if err == nil {
		t.Fatal("expected an error for an offsite app")
	}
	msg := err.Error()

	// The defect: the old message's next step.
	if strings.Contains(msg, "run `civitai app submit` first") {
		t.Errorf("the dead-end advice is the regression — `app submit` cannot succeed for an offsite app:\n%s", msg)
	}
	for _, want := range []string{
		"OFFSITE app",
		offsiteSlugA,
		offsiteURLA,
		"cannot be addressed from this CLI",
		"`civitai app view " + offsiteSlugA + "`",
		"App-store listing UI on civitai.com",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got:\n%s", want, msg)
		}
	}
	// House rule: an error names a real next command (see
	// errors_name_next_command_test.go).
	if !strings.Contains(msg, "civitai app view") {
		t.Errorf("the refusal must name a next command that WORKS; got:\n%s", msg)
	}
	// AGENTS.md item 7 — asserted with errors.Is, because the sentinel carries
	// no visible text. The app exists but the submission this command resolves
	// through does not, so the code stays 4.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("the offsite refusal must stay not-found (exit 4), got %T: %v", err, err)
	}
	if n := ps.appCalls.Load(); n != 1 {
		t.Errorf("the store probe must run exactly once on the error path, ran %d times", n)
	}
}

// TestAppStatusOffsiteRefusesPrecisely is the other half, and the assertions
// below are what stops the two messages being collapsed into one: `app status`
// lists BLOCK SUBMISSIONS, an offsite app genuinely has none, and nothing about
// that command is about store media.
func TestAppStatusOffsiteRefusesPrecisely(t *testing.T) {
	newAppProbeServer(t, bothOffsiteAndOnsite())

	_, _, err := run(t, "app", "status", offsiteSlugB)
	if err == nil {
		t.Fatal("expected an error for an offsite app")
	}
	msg := err.Error()

	if strings.Contains(msg, "run `civitai app submit` first") {
		t.Errorf("the dead-end advice is the regression:\n%s", msg)
	}
	for _, want := range []string{
		"OFFSITE app",
		offsiteSlugB,
		offsiteURLB,
		"no block submissions",
		"`civitai app view " + offsiteSlugB + "`",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q; got:\n%s", want, msg)
		}
	}
	// `app status` says nothing about listing media — that is the listing
	// command's story, and telling it here would send the reader to a surface
	// this command has no business naming.
	if strings.Contains(msg, "App-store listing UI") {
		t.Errorf("`app status` must not talk about the listing UI:\n%s", msg)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("the offsite refusal must stay not-found (exit 4), got %T: %v", err, err)
	}
}

// TestOffsiteRefusalsDifferPerCommand pins the DISTINCTION itself. Both messages
// are produced for the same app, from the same probe result, and they must not
// converge: one reports a CLI gap over a listing that exists, the other reports
// the plain truth that there are no submissions. A future "let's share the
// string" refactor is exactly what this fails on.
func TestOffsiteRefusalsDifferPerCommand(t *testing.T) {
	d := &civitai.AppDetail{Slug: serverEchoSlug(offsiteSlugA), Kind: "offsite"}
	d.KindData.Kind = "offsite"
	d.KindData.ExternalURL = offsiteURLA

	listing := offsiteListingRefusal(offsiteSlugA, d)
	status := offsiteStatusRefusal(offsiteSlugA, d)
	if listing == status {
		t.Fatal("the two commands must not share one message — that difference IS #422 outcome 2")
	}
	if !strings.Contains(listing, "civitai app listing") || strings.Contains(listing, "no block submissions") {
		t.Errorf("the listing refusal must be about the LISTING:\n%s", listing)
	}
	if !strings.Contains(status, "no block submissions") || strings.Contains(status, "App-store listing UI") {
		t.Errorf("the status refusal must be about SUBMISSIONS:\n%s", status)
	}
}

// TestOffsiteRefusalRendersTheAppsOwnExternalURL is the fixture-trap control.
// Two offsite apps with two DIFFERENT external URLs must produce two different
// messages: if the rendered URL could only ever be the one value the fixture
// supplies, a mutant that hardcodes a literal would survive a green suite.
func TestOffsiteRefusalRendersTheAppsOwnExternalURL(t *testing.T) {
	newAppProbeServer(t, bothOffsiteAndOnsite())

	_, _, errA := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	_, _, errB := run(t, "app", "listing", "status", "--slug", offsiteSlugB)
	if errA == nil || errB == nil {
		t.Fatal("expected errors for both offsite apps")
	}
	a, b := errA.Error(), errB.Error()
	if !strings.Contains(a, offsiteURLA) || strings.Contains(a, offsiteURLB) {
		t.Errorf("app A's message must carry A's URL and only A's:\n%s", a)
	}
	if !strings.Contains(b, offsiteURLB) || strings.Contains(b, offsiteURLA) {
		t.Errorf("app B's message must carry B's URL and only B's:\n%s", b)
	}
}

// TestOffsiteRegisteredAtRejectsUnsafeServerText — the external URL is SERVER
// text spliced into the middle of a CLI error, and safeTerm deliberately keeps
// `\n` and `\t`. A value that could break the sentence across lines, or that is
// not an absolute http(s) URL, is dropped and the message simply says less.
//
// 🔴 THE UNICODE ROWS ARE THE FIX FOR A GUARD THAT WAS SPELLED, NOT STRUCTURAL.
// The first cut banned four ASCII bytes and its comment claimed "no whitespace
// at all"; measured against that shipped function, U+00A0, U+2028, U+2029,
// U+3000 and U+205F all RENDERED, and U+2028/U+2029 are line separators — the
// exact sentence-breaking hazard the four-byte list was written to stop.
// saferune cannot backstop it: it subtracts White_Space from its strip class on
// purpose. Each row below is a rune that USED to render.
//
// 🔴 THE TWO REACHABILITY ROWS (`ftp` and `javascript://…`) EXIST BECAUSE THE
// SCHEME CLAUSE WAS UNREACHED. `javascript:alert(1)` and `file:///etc/passwd`
// both parse to an EMPTY Host, so `u.Host == ""` killed them first and deleting
// `u.Scheme != "http" && u.Scheme != "https"` survived a green suite. These two
// carry a real host, so the scheme clause is the ONLY thing that can reject
// them — the "an earlier check always wins" mutant case, closed by construction.
//
// 🔴 THE ACCEPT ROWS ARE A MUTATION CONTROL, NOT DECORATION, AND THAT IS WHY
// THEY ARE SO CROWDED. This test once had exactly two `want: true` rows —
// `https://ok.example/x` and its http twin — which between them contain no
// digit, no `%`, no `?`, no `#`, no `@`, no `~`, no port and no uppercase
// letter. Measured against that table, EVERY one of these survived a fully green
// suite: dropping `%`, `?`, `@`, `#`, `~` or the digit `9` from the accepted set
// (each of which silently stops rendering every URL with percent-encoding, a
// query string or a digit in it), and — worse — replacing the allowlist
// wholesale with a DENYLIST of exactly the runes the reject rows below name,
// which is a verbatim regression to the spelled guard this function exists to
// replace. So the four accept rows partition the allowlist between them and are
// annotated with which runes each is the only witness for: an accept row's job
// is to go RED when a rune leaves the accepted set.
//
// 🔴 AND THE BACKTICK / `{` / `|` REJECT ROWS ARE WHAT A DENYLIST CANNOT PASS.
// All three parse cleanly with a real host and an https scheme (measured), so
// `isURIRune` is the ONLY thing that can reject them — the same reachability
// argument as the scheme rows above. The backtick is the load-bearing one: the
// refusal messages use backticks as code-span delimiters, so an off-by-one
// widening of `'a'..'z'` (whose predecessor IS the backtick) would let a server
// URL forge a span in a sentence the CLI wrote.
func TestOffsiteRegisteredAtRejectsUnsafeServerText(t *testing.T) {
	for _, tc := range []struct {
		name, url string
		want      bool
	}{
		{"plain https", "https://ok.example/x", true},
		{"plain http", "http://ok.example/x", true},
		// The three rows above carry no digit and no punctuation beyond `:/.`,
		// so the accepted set is pinned by these instead. Each names the runes
		// it is the SOLE witness for; drop one of those from uriRuneAllowlist
		// (or from isURIRune's ranges) and this row is what goes red.
		// Sole witness: `%`, `?`, `#`, every digit 0-9, an explicit port, and an
		// uppercase letter.
		{"port, percent-encoding, query, fragment, digits", "https://ok.example:8443/A%20b?q=1234567890#f", true},
		// Sole witness: `@` (userinfo) and `~`.
		{"userinfo and a tilde", "https://user@ok.example/~a/b.c", true},
		// Sole witness: `-`, `_`, and every sub-delim `!$&'()*+,;=`.
		{"unreserved punctuation and every sub-delim", "http://ok.example/-a_b!$&'()*+,;=Z", true},
		// Sole witness: `[` and `]`, which only ever appear as an IPv6 literal.
		{"IPv6 literal host", "https://[2001:db8::1]:8443/p", true},
		{"empty", "", false},
		// 🔴 OF THE ORIGINAL FOUR-ASCII-BYTE BAN, ONLY `' '` IS LOAD-BEARING
		// HERE — MEASURED, and the next two rows are green for a reason that is
		// not the allowlist. `url.Parse` rejects `\t` and `\n` itself ("invalid
		// control character in URL"), so adding either to uriRuneAllowlist
		// survives this table; they are kept as end-to-end rows for the
		// sentence-forging shape, not as evidence about the predicate. `' '`
		// parses fine and carries a real host, so the allowlist is the only
		// thing that can reject it. (`'\r'` never reached here at all — safeTerm
		// strips it first.)
		{"newline forging a line", "https://ok.example/x\nRun `rm -rf /` to fix", false},
		{"space", "https://ok.example/a b", false},
		{"tab", "https://ok.example/a\tb", false},
		// Rejected ONLY by the allowlist: all three parse to a real host with an
		// https scheme (measured), so no earlier check can win. A denylist that
		// enumerates the runes this test names cannot pass these, which is the
		// point of having them. The backtick is a code-span delimiter in the
		// refusal messages, and it is `'a'-1`.
		// No space in this value on purpose: a row containing one would be
		// rejected by the `' '` clause whether or not the backtick were still
		// banned, which is exactly the vacuous shape these rows exist to avoid.
		{"backtick forging a code span", "https://ok.example/a`b`c", false},
		{"left brace", "https://ok.example/a{b", false},
		{"pipe", "https://ok.example/a|b", false},
		{"not a URL", "just some words", false},
		{"no host", "https:///path", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"file scheme", "file:///etc/passwd", false},
		// Rejected ONLY by the scheme clause — real host, URI-legal runes.
		{"ftp scheme with a real host", "ftp://ok.example/x", false},
		{"javascript scheme with a real host", "javascript://ok.example/%0aalert(1)", false},
		// Every one of these rendered before the allowlist replaced the
		// four-ASCII-byte ban. Written as numeric runes, never raw bytes.
		{"U+00A0 no-break space", "https://ok.example/a" + string(rune(0x00A0)) + "b", false},
		{"U+2028 line separator", "https://ok.example/a" + string(rune(0x2028)) + "b", false},
		{"U+2029 paragraph separator", "https://ok.example/a" + string(rune(0x2029)) + "b", false},
		{"U+3000 ideographic space", "https://ok.example/a" + string(rune(0x3000)) + "b", false},
		{"U+205F medium mathematical space", "https://ok.example/a" + string(rune(0x205F)) + "b", false},
		{"U+1680 ogham space mark", "https://ok.example/a" + string(rune(0x1680)) + "b", false},
		{"U+2007 figure space", "https://ok.example/a" + string(rune(0x2007)) + "b", false},
		// U+0085 NEXT LINE is deliberately NOT here. It is C1, so safeTerm
		// removes it before this predicate ever sees it and the URL renders
		// (without it) — a STRIP, not a rejection. Its row lives in
		// TestOffsiteRefusalSanitizesControlChars, which asserts that.
		// Measured, not assumed: asserting rejection here failed.
		// Not whitespace at all, so `unicode.IsSpace` would have kept it: a
		// full-width solidus that reads as a path separator it is not. The
		// allowlist is what makes this a non-question.
		{"U+FF0F fullwidth solidus", "https://ok.example" + string(rune(0xFF0F)) + "evil", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &civitai.AppDetail{}
			d.KindData.ExternalURL = tc.url
			got := offsiteRegisteredAt(d)
			if tc.want != (got != "") {
				t.Fatalf("offsiteRegisteredAt(%q) = %q, wanted rendered=%v", tc.url, got, tc.want)
			}
			if tc.want && !strings.Contains(got, tc.url) {
				t.Errorf("a rendered target must carry the URL: %q", got)
			}
		})
	}
	// The whole message survives a dropped URL — the refusal is still complete.
	d := &civitai.AppDetail{}
	d.KindData.ExternalURL = "not a url"
	msg := offsiteListingRefusal(offsiteSlugA, d)
	if !strings.Contains(msg, "OFFSITE app,") || !strings.Contains(msg, "civitai app view") {
		t.Errorf("with no printable target the message must still be whole:\n%s", msg)
	}
}

// TestOffsiteRefusalSanitizesControlChars is the STRIPPING half, and it did not
// exist until it was found missing: replacing `safeTerm(d.KindData.ExternalURL)`
// with the raw field in offsiteRegisteredAt left the whole `internal/cmd` suite
// green. TestOffsiteRegisteredAtRejectsUnsafeServerText only ever asserted
// REJECTION — that a hostile value produces nothing — which a mutant that drops
// the strip satisfies trivially, because dropping it makes MORE values reject.
//
// The property this pins is the opposite one: a value that is hostile ONLY in
// runes safeTerm removes must still render, WITHOUT them. That is what makes the
// call load-bearing, and it is the house pattern of the five
// …SanitizesControlChars tests in sanitize_render_test.go.
//
// The mutant it kills, concretely: with the raw field, U+202E / U+200B / ESC are
// not RFC 3986 runes, so isURIRune rejects the whole value and the ` (registered
// at …)` clause vanishes — every `wantRendered` assertion below fails.
func TestOffsiteRefusalSanitizesControlChars(t *testing.T) {
	// Built from numeric rune values, never typed as raw bytes — the same rule
	// sanitize_render_test.go's escRune/belRune/c1CSI follow, and the one
	// staticcheck's ST1018 enforces.
	const (
		rtlOverride = string(rune(0x202E)) // RIGHT-TO-LEFT OVERRIDE — reverses what follows
		zeroWidth   = string(rune(0x200B)) // ZERO WIDTH SPACE — invisible, splits a hostname
	)
	for _, tc := range []struct {
		name, url, wantRendered string
	}{
		{"bidi override in the path", "https://ok.example/a" + rtlOverride + "b", "https://ok.example/ab"},
		{"zero-width space in the host", "https://ok" + zeroWidth + ".example/x", "https://ok.example/x"},
		{"ANSI colour introducer", "https://ok.example/x" + escRune + "[31m", "https://ok.example/x[31m"},
		{"C1 CSI", "https://ok.example/y" + c1CSI + "[2K", "https://ok.example/y[2K"},
		{"BEL terminating an OSC", "https://ok.example/z" + belRune, "https://ok.example/z"},
		{"U+0085 next line", "https://ok.example/a" + string(rune(0x0085)) + "b", "https://ok.example/ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &civitai.AppDetail{}
			d.KindData.ExternalURL = tc.url
			got := offsiteRegisteredAt(d)
			want := " (registered at " + tc.wantRendered + ")"
			if got != want {
				t.Fatalf("offsiteRegisteredAt(%q) = %q, want %q — the hostile runes must be STRIPPED, "+
					"not made to reject the whole URL", tc.url, got, want)
			}
			assertNoControlBytes(t, "the offsite registered-at clause", got)
		})
	}

	// End to end, through the real command tree: the refusal a user actually
	// sees carries the stripped URL and no control bytes.
	fixtures := bothOffsiteAndOnsite()
	hostile := "https://ok.example/live" + rtlOverride + escRune + "[1A" + escRune + "[2K"
	fixtures[offsiteSlugA] = appDetailJSON(offsiteSlugA, "offsite", hostile)
	newAppProbeServer(t, fixtures)

	_, _, err := run(t, "app", "listing", "status", "--slug", offsiteSlugA)
	if err == nil {
		t.Fatal("expected an error for an offsite app")
	}
	msg := err.Error()
	assertNoControlBytes(t, "the offsite refusal", msg)
	if !strings.Contains(msg, "(registered at https://ok.example/live[1A[2K)") {
		t.Errorf("the refusal must render the app's URL with the control runes removed; got:\n%q", msg)
	}
}

// ---------------------------------------------------------------------------
// Everything that must keep TODAY's message
// ---------------------------------------------------------------------------

// TestOnsiteNeverSubmittedKeepsTodaysMessage is THE regression case. An onsite
// app that exists in the store but has never been submitted is the situation the
// existing message was written for (civitai/cli#363), and `civitai app submit`
// really is its next step.
//
// 🔴 LABEL IT HONESTLY: the MESSAGE assertion is an INVARIANT guard — it is
// green at `1b20b99` too, because the message it pins is the one that already
// shipped. What is red at base is the PREMISE below (`appCalls == 1`): before
// this change nothing probed the store at all. Both are worth having, and only
// the premise makes the message assertion mean "the probe ran and said onsite"
// rather than "no probe exists".
func TestOnsiteNeverSubmittedKeepsTodaysMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"app listing status", []string{"app", "listing", "status", "--slug", onsiteSlug}},
		{"app status <slug>", []string{"app", "status", onsiteSlug}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected the no-submission error")
			}
			if err.Error() != wantNoSubmissionMsg(onsiteSlug) {
				t.Errorf("an onsite app that was never submitted must keep TODAY's message.\nwant: %s\ngot:  %s",
					wantNoSubmissionMsg(onsiteSlug), err.Error())
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
			// PREMISE: the probe really ran and really said "onsite" — without
			// this the assertion above passes for a probe that never fired,
			// which is the same green for a completely different reason.
			if n := ps.appCalls.Load(); n != 1 {
				t.Errorf("the probe must have run exactly once (it is what decided onsite), ran %d times", n)
			}
		})
	}
}

// TestUnknownSlugKeepsTodaysMessage — a slug that is no app at all. The store
// route 404s, nothing is established, and the message is unchanged. Same
// labelling as above: the message half is an invariant guard, the `appCalls`
// premise is the half that is red at base.
func TestUnknownSlugKeepsTodaysMessage(t *testing.T) {
	const unknown = "definitely-not-an-app-zzz"
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"app listing status", []string{"app", "listing", "status", "--slug", unknown}},
		{"app status <slug>", []string{"app", "status", unknown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected the no-submission error")
			}
			if err.Error() != wantNoSubmissionMsg(unknown) {
				t.Errorf("an unknown slug must keep TODAY's message.\nwant: %s\ngot:  %s",
					wantNoSubmissionMsg(unknown), err.Error())
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
			if n := ps.appCalls.Load(); n != 1 {
				t.Errorf("the probe must have run and 404ed, ran %d times", n)
			}
		})
	}
}

// TestProbeFailureKeepsTodaysMessage — every way the diagnostic can fail to
// answer collapses to the unchanged error. A network failure of a DIAGNOSTIC
// must never replace a real error with a confusing one.
func TestProbeFailureKeepsTodaysMessage(t *testing.T) {
	const slug = "probe-fails-zzz"
	for _, tc := range []struct {
		name string
		// serve answers GET /api/v1/apps/{slug}.
		serve http.HandlerFunc
	}{
		{"500 from the store route", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}},
		{"403 — not visible to this caller", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		}},
		{"garbage body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html>not json</html>`))
		}},
		{"a kind this CLI does not recognise", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(appDetailJSON(slug, "marketplace-v2", "https://future.zzz/x")))
		}},
		{"no kind at all", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"slug":"` + slug + `","name":"x"}`))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
					tc.serve(w, r)
					return
				}
				_, _ = w.Write([]byte(`{"submissions":[]}`))
			}))
			defer srv.Close()
			listingEnv(t, srv.URL)

			_, _, err := run(t, "app", "listing", "status", "--slug", slug)
			if err == nil {
				t.Fatal("expected the no-submission error")
			}
			if err.Error() != wantNoSubmissionMsg(slug) {
				t.Errorf("a probe that could not answer must leave the message alone.\nwant: %s\ngot:  %s",
					wantNoSubmissionMsg(slug), err.Error())
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
		})
	}
}

// TestNonNotFoundErrorsAreNeverProbed — the enrichment is gated on the
// not-found KIND, not on "the command failed". A 403 from the invite-gated
// submissions route, a 5xx or a transport failure are different errors with
// different exit codes, and none of them is evidence about an app's kind: an
// offsite app whose submissions lookup 403s must still report the 403, on exit
// 3, and must not spend a request finding that out.
func TestNonNotFoundErrorsAreNeverProbed(t *testing.T) {
	var appCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
			appCalls.Add(1)
			_, _ = w.Write([]byte(appDetailJSON(offsiteSlugA, "offsite", offsiteURLA)))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"not an apps author"}`))
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"app listing status", []string{"app", "listing", "status", "--slug", offsiteSlugA}},
		{"app status <slug>", []string{"app", "status", offsiteSlugA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected the 403")
			}
			if strings.Contains(err.Error(), "OFFSITE") {
				t.Errorf("a 403 must not be rewritten into a claim about the app's kind:\n%s", err.Error())
			}
			if !errors.Is(err, civitai.ErrUnauthorized) {
				t.Errorf("a 403 must stay exit 3, got %T: %v", err, err)
			}
		})
	}
	if n := appCalls.Load(); n != 0 {
		t.Errorf("a non-not-found failure must not be probed at all; the store route ran %d times", n)
	}
}

// TestOffsiteProbeDoesNotHoldTheCommandOpen — the probe gets a deadline of its
// own, SHORTER than the read client's 30s budget. The command has already
// failed; a hung store route must not keep it alive for the full budget before
// printing the message it would have printed immediately.
//
// The elapsed assertion is what makes this more than a wiring check: removing
// the context.WithTimeout in offsiteApp leaves this test hanging on the client's
// own budget (30s per attempt) instead, which no server-side assertion can see.
func TestOffsiteProbeDoesNotHoldTheCommandOpen(t *testing.T) {
	const slug = "hangs-zzz"
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
			<-block
			return
		}
		_, _ = w.Write([]byte(`{"submissions":[]}`))
	}))
	// 🔴 ORDER: httptest.Server.Close BLOCKS until every outstanding handler
	// returns, and this one is parked on <-block. Defers run LIFO, so the
	// release must be registered LAST to run FIRST — registering it as a
	// t.Cleanup instead deadlocks the whole package, because Cleanup runs after
	// the deferred Close.
	defer srv.Close()
	defer close(block)
	listingEnv(t, srv.URL)

	old := offsiteProbeTimeout
	offsiteProbeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { offsiteProbeTimeout = old })

	// 🔴 os.Stderr, NOT the cobra buffer. The read client's transient-retry
	// notices go to the process's real stderr, so the only way to see whether
	// the probe is silent is to hold the file it writes to. Without this, the
	// `client.Stderr = io.Discard` line in offsiteApp is unpinned and deleting
	// it leaves the suite green while `network error from Civitai, retrying
	// (2/4)…` prints ABOVE the real error on every probe timeout — measured.
	stderrFile := filepath.Join(t.TempDir(), "stderr.txt")
	f, ferr := os.Create(stderrFile)
	if ferr != nil {
		t.Fatalf("create stderr capture: %v", ferr)
	}
	realStderr := os.Stderr
	os.Stderr = f
	restore := func() { os.Stderr = realStderr; _ = f.Close() }
	defer restore()

	start := time.Now()
	_, _, err := run(t, "app", "listing", "status", "--slug", slug)
	elapsed := time.Since(start)
	restore()

	captured, rerr := os.ReadFile(stderrFile)
	if rerr != nil {
		t.Fatalf("read stderr capture: %v", rerr)
	}
	if strings.Contains(string(captured), "retrying") {
		t.Errorf("the diagnostic probe must be SILENT — its retry notices would sit above the real error:\n%s", captured)
	}

	if err == nil {
		t.Fatal("expected the no-submission error")
	}
	if err.Error() != wantNoSubmissionMsg(slug) {
		t.Errorf("a probe that timed out must leave the message alone.\nwant: %s\ngot:  %s",
			wantNoSubmissionMsg(slug), err.Error())
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("exit code must stay 4, got %T: %v", err, err)
	}
	// 5s is far below the read client's own 30s-per-attempt budget, and far
	// above the 50ms this should really take — a threshold that separates the
	// two without being a timing race.
	if elapsed > 5*time.Second {
		t.Errorf("the probe inherited the client's budget instead of its own: the command took %s", elapsed)
	}
}

// ---------------------------------------------------------------------------
// The probe costs nothing on a path that did not fail
// ---------------------------------------------------------------------------

// TestSuccessfulCommandsMakeNoStoreProbe is constraint 1 of #422 outcome 2: the
// diagnostic adds NO request to any successful command. Both surfaces are driven
// to a real success and the store route's call count is asserted to be zero.
func TestSuccessfulCommandsMakeNoStoreProbe(t *testing.T) {
	var appCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
			appCalls.Add(1)
			_, _ = w.Write([]byte(appDetailJSON("my-app", "offsite", offsiteURLA)))
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft", "contentRating": "g", "hasPendingRevision": false})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId": "listing_1", "slug": "my-app", "status": "draft", "hasPendingRevision": false, "shadowId": nil,
				"assets": map[string]any{
					"icon":        map[string]any{"imageId": 11, "url": "http://x/i.png"},
					"cover":       map[string]any{"imageId": 12, "url": "http://x/c.png"},
					"screenshots": []any{},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	if _, _, err := run(t, "app", "listing", "status", "--slug", "my-app"); err != nil {
		t.Fatalf("PREMISE BROKEN — the happy path must succeed: %v", err)
	}
	if _, _, err := run(t, "app", "status", "my-app"); err != nil {
		t.Fatalf("PREMISE BROKEN — the happy path must succeed: %v", err)
	}
	if n := appCalls.Load(); n != 0 {
		t.Errorf("a successful command must not probe the store route; it ran %d times", n)
	}
}

// TestAppStatusByIDDoesNotProbeTheStore — the `--id` selector names a publish
// request, not an app, so there is no slug to ask about. The precedence mirrors
// submissionsURL/submissionSubject: the id wins even when a slug is ALSO given,
// so neither spelling may probe.
func TestAppStatusByIDDoesNotProbeTheStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"--id alone", []string{"app", "status", "--id", "pubreq_01ZZZ"}},
		{"--id wins over a slug", []string{"app", "status", "--id", "pubreq_01ZZZ", offsiteSlugA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatal("expected a not-found error for an unknown publish request")
			}
			if strings.Contains(err.Error(), "OFFSITE") {
				t.Errorf("an `--id` lookup must not be answered with a claim about an app:\n%s", err.Error())
			}
			if n := ps.appCalls.Load(); n != 0 {
				t.Errorf("an `--id` lookup has no slug to probe; the store route ran %d times", n)
			}
			if !errors.Is(err, civitai.ErrNotFound) {
				t.Errorf("exit code must stay 4, got %T: %v", err, err)
			}
		})
	}
}

// TestOffsiteRefusalReachesEverySubcommand — resolveListing is the one funnel
// for `app listing`, so every subcommand that resolves a slug must produce the
// refusal, not just `status`. This is the reachability half of the mutation
// check: the branch is not reached only by the one command a single case drives.
//
// 🔴 ALL SIX SUBCOMMANDS, NOT FOUR. The first cut drove `status`, `set-icon`,
// `set-cover` and `rm-screenshot` and left `add-screenshot` (runSetMedia, the
// same shared flow as set-icon/set-cover) and `reorder`
// (resolveListingRefFromFlags, the same helper as rm-screenshot) unexercised —
// so the claim "every subcommand" was two thirds measured. `app listing` has
// exactly these six; if a seventh is added it belongs here.
func TestOffsiteRefusalReachesEverySubcommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(t *testing.T) []string
	}{
		{"status", func(*testing.T) []string {
			return []string{"app", "listing", "status", "--slug", offsiteSlugA}
		}},
		{"set-icon", func(t *testing.T) []string {
			return []string{"app", "listing", "set-icon", writePNG(t, 512, 512), "--slug", offsiteSlugA, "-y"}
		}},
		{"set-cover", func(t *testing.T) []string {
			return []string{"app", "listing", "set-cover", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}},
		{"add-screenshot", func(t *testing.T) []string {
			return []string{"app", "listing", "add-screenshot", writePNG(t, 1600, 900), "--slug", offsiteSlugA, "-y"}
		}},
		{"rm-screenshot", func(*testing.T) []string {
			return []string{"app", "listing", "rm-screenshot", "alsc_1", "--slug", offsiteSlugA}
		}},
		{"reorder", func(*testing.T) []string {
			return []string{"app", "listing", "reorder", "alsc_2", "alsc_1", "--slug", offsiteSlugA}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newAppProbeServer(t, bothOffsiteAndOnsite())
			_, _, err := run(t, tc.args(t)...)
			if err == nil {
				t.Fatal("expected an error for an offsite app")
			}
			if !strings.Contains(err.Error(), "OFFSITE app") {
				t.Errorf("every `app listing` subcommand resolves through resolveListing and must refuse precisely:\n%s", err.Error())
			}
		})
	}
}
