package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// ---------------------------------------------------------------------------
// The OFFSITE refusal (civitai/cli#422, outcome 2)
// ---------------------------------------------------------------------------
//
// `app listing <sub>` and `app status <slug>` both resolve their target through
// GET /api/v1/blocks/submissions?blockId=<slug>. An OFFSITE app is a registered
// URL rather than a block bundle, so it has NO submissions, so both commands
// die in appapi's empty-list branch with
//
//	no such submission for app "radio" — run `civitai app submit` first; …
//
// which names a command that CANNOT succeed for such an app: `app submit`
// packages a block bundle and an offsite app has none. Measured on #422: 4/4
// offsite apps fail that way, 7/7 onsite apps succeed — `kind` predicts it
// exactly.
//
// 🔴 THIS IS THE REFUSAL, NOT THE REPAIR, AND THAT IS A MEASURED CHOICE RATHER
// THAN A SCOPE CUT. #422's preferred outcome was to make offsite apps reachable
// by resolving them by slug/app id instead. That is impossible client-side:
// `appListings.getMyListingForApp` was measured live to resolve ONLY by
// `appBlockId` (the slug selector 404s for every app, onsite positive controls
// included), and an offsite app has no appBlockId — that is what `kind: offsite`
// MEANS. Dropping the submission lookup just moves the 404 one call later.
// Reaching these apps needs a server-side selector; see #422's thread and #424.
// So do not "fix" this file by making it resolve something.
//
// 🔴 THE PROBE RUNS ONLY ON THE ERROR PATH, and that is load-bearing. A
// successful `app listing status` / `app status <slug>` must be byte-identical
// and cost exactly the requests it costs today — a diagnostic that runs
// unconditionally buys latency and a brand-new failure mode for advice nobody
// will read. Same argument, same shape, as explainAppViewNotFound's ownership
// probe in apps.go.
//
// ---------------------------------------------------------------------------
// RESIDUALS THIS FILE KNOWINGLY SHIPS WITH — stated so they are not rediscovered
// as findings, and deliberately NOT fixed here (civitai/cli#427).
// ---------------------------------------------------------------------------
//
// 1. THE REFUSAL ASSERTS THINGS THAT ARE FALSE FOR A SLUG THE CALLER DOES NOT
//    OWN. offsiteApp performs no ownership check, and GET /api/v1/apps/{slug} is
//    the PUBLIC catalog, so the app whose kind is read may belong to a stranger.
//    `civitai app listing` then says "an offsite app's store listing cannot be
//    addressed from this CLI" and points at "the App-store listing UI … where
//    this app was registered", both of which read as advice to a person who
//    could act on it. This predates the refusal — the message it replaced said
//    "run `civitai app submit` first", equally false for someone else's slug —
//    so it is a pre-existing wrongness this change carries forward rather than
//    one it introduced. Closing it needs an ownership signal these two commands
//    do not have on the error path.
//
// 2. THE PROBE IS AN EXTRA ROUND TRIP ON THE MOST COMMON ERROR PATH, WITH NO OFF
//    SWITCH. Every `no such submission` — the ordinary "I have not submitted yet"
//    case, which is far more common than an offsite app — now pays one bounded
//    (5s) request before printing. That is a COST, not a defect: it buys the
//    diagnosis, it cannot make the command fail differently (every probe failure
//    collapses to today's message, pinned), and no flag or env var disables it.
//    If that cost is ever measured to matter, the knob goes here.

// appKindOnsite / appKindOffsite are the two `kind` discriminators GET
// /api/v1/apps/{slug} returns (civitai.AppDetail.Kind). They are the store's
// enum, not the CLI's, so `app list --kind` builds its allowed set from them
// rather than re-spelling the same two words a second time.
const (
	appKindOnsite  = "onsite"
	appKindOffsite = "offsite"
)

// offsiteProbeTimeout bounds the diagnostic lookup this file makes.
//
// 🔴 A DEADLINE OF ITS OWN, SHORTER THAN THE CLIENT'S. The command has ALREADY
// failed by the time the probe runs; everything it can still do is improve the
// wording of an error the user is waiting on. The read client's own budget is
// sized for an ANSWER, and letting a diagnostic inherit it means a hung
// /api/v1/apps route holds a failed command open for the full budget before
// printing the message it would have printed immediately.
//
// A `var`, not a `const`, and only so a test can SHRINK it — nothing in the
// binary writes it. Pinned by TestOffsiteProbeDoesNotHoldTheCommandOpen, which
// shrinks it against a server that never answers and asserts BOTH the fallback
// message and that the command returned in a fraction of the client budget; a
// deadline is not observable from the server side of an httptest handler, so
// that elapsed-time assertion is the only thing separating a dedicated budget
// from an inherited one.
var offsiteProbeTimeout = 5 * time.Second

// offsiteApp answers ONE question — "is <slug> an app that EXISTS and whose kind
// is offsite?" — and it is the only place that question is answered. Both
// commands that need it call through here; open-coding a second `Kind ==
// "offsite"` at the other call site is how one of the two ends up wrong.
//
// nil means NOT ESTABLISHED, and it deliberately does not distinguish "onsite",
// "no such app", "the probe could not ask" (no token, network, timeout, 404,
// 5xx) or "a kind this CLI does not recognise". Every one of those must produce
// today's message unchanged: a diagnostic whose own failure REPLACES a real
// error with a confident wrong one is strictly worse than the terse error it
// replaced. Collapsing them here keeps that decision in one place.
//
// An unrecognised kind is in that list on purpose. The store's kind enum is the
// server's to grow, and a future third kind is a thing this CLI knows nothing
// about — asserting "this is an offsite app" about it would be a false claim,
// while the existing message is merely unhelpful.
func offsiteApp(ctx context.Context, slug string) *civitai.AppDetail {
	if strings.TrimSpace(slug) == "" {
		return nil
	}
	client, err := newLoginGatedReader()
	if err != nil {
		return nil
	}
	// 🔴 THE PROBE IS SILENT, INCLUDING ITS RETRIES. The read client prints
	// `network error from Civitai, retrying (n/4)…` to stderr, which is right
	// for a call the user is waiting on and wrong for a diagnostic they never
	// asked for: on a hung store route it puts retry noise ABOVE the real error,
	// making a command that failed for one reason look like it failed for
	// another. Measured — it printed on the probe-timeout path before this line.
	client.Stderr = io.Discard
	if ctx == nil {
		ctx = context.Background()
	}
	pctx, cancel := context.WithTimeout(ctx, offsiteProbeTimeout)
	defer cancel()
	d, _, err := client.GetApp(pctx, slug)
	if err != nil || d == nil {
		return nil
	}
	if d.Kind != appKindOffsite {
		return nil
	}
	return d
}

// offsiteRefusal renders the per-command replacement message for an offsite app.
// The two commands get DIFFERENT copy and that difference is the whole point of
// #422's outcome 2 — see offsiteListingRefusal / offsiteStatusRefusal.
type offsiteRefusal func(slug string, d *civitai.AppDetail) string

// explainOffsiteMiss upgrades the submissions route's "no such submission" to a
// precise refusal when the slug turns out to be an offsite app.
//
// It REPLACES the message rather than appending to it, unlike
// explainAppViewNotFound. That is the difference between the two situations:
// #223's advice adds context to a 404 that is TRUE and whose remedy is still
// "look at your submission", while here the sentence being replaced names
// `civitai app submit` as the next step and that step cannot exist for this app.
// Appending would leave the dead end on screen above the correction.
//
// The classification is re-attached, so the exit code is unchanged at 4 — see
// AGENTS.md item 29 for why. That item is deliberately INLINE in AGENTS.md and
// has no `claudedocs/decisions/29-*.md` body: a first-appearance item cannot
// carry the agents_split_preserved_test.go provenance row a split body needs.
// (An earlier draft of this line cited a decision file that never existed;
// TestSourceCitationsOfDecisionFilesResolve now fails on that class.)
func explainOffsiteMiss(ctx context.Context, slug string, err error, refuse offsiteRefusal) error {
	// Only the not-found kind. A 403 from the invite-gated submissions route, a
	// 5xx or a transport failure are different errors with different exit codes,
	// and none of them is evidence about the app's kind.
	if err == nil || !errors.Is(err, civitai.ErrNotFound) {
		return err
	}
	d := offsiteApp(ctx, slug)
	if d == nil {
		return err
	}
	return civitai.Tag(civitai.ErrNotFound, errors.New(refuse(slug, d)))
}

// uriRuneAllowlist is every non-alphanumeric character RFC 3986 permits in a
// URI: unreserved punctuation (`-._~`), gen-delims (`:/?#[]@`), sub-delims
// (`!$&'()*+,;=`) and `%`, the pct-encoding introducer. Alphanumerics are
// handled by isURIRune's ranges.
const uriRuneAllowlist = "-._~:/?#[]@!$&'()*+,;=%"

// isURIRune reports whether r is a character RFC 3986 allows to appear in a URI.
func isURIRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune(uriRuneAllowlist, r)
}

// offsiteRegisteredAt renders " (registered at <url>)" for an offsite app's
// external target, or the empty string when there is nothing safe to print.
//
// 🔴 THE URL IS SERVER TEXT IN THE MIDDLE OF A CLI ERROR, and — because
// offsiteApp does no ownership check and GET /api/v1/apps/{slug} is the PUBLIC
// catalog — it is very often a THIRD PARTY's string. So it is not merely
// safeTerm'd. safeTerm deliberately KEEPS `\n` and `\t` (legitimate layout in
// the fields it was written for), which here would let a server string break the
// sentence across lines and forge whatever the next line said.
//
// 🔴 SO THE FILTER IS AN ALLOWLIST, NOT A LIST OF CHARACTERS SOMEONE REMEMBERED
// TO BAN, AND THAT REPLACES A GUARD THAT WAS SPELLED RATHER THAN STRUCTURAL.
// The first cut rejected exactly four ASCII bytes — ' ', '\t', '\n', '\r' —
// while claiming to accept "no whitespace at all". Measured against that shipped
// function, U+00A0 NBSP, U+2028 LINE SEPARATOR, U+2029 PARAGRAPH SEPARATOR,
// U+3000 IDEOGRAPHIC SPACE and U+205F MEDIUM MATHEMATICAL SPACE all RENDERED,
// and two of those are line separators. `internal/saferune` cannot be the
// backstop: it SUBTRACTS Unicode's White_Space property from its strip class on
// purpose, so every rune above reaches here intact. (civitai/cli#367, #382, #393
// are the same class — a guard satisfied by writing the hazard a different way.)
//
// Widening the ban to `unicode.IsSpace` would have closed those five and left
// the same shape of question open for every future non-space rune that renders
// as a gap or a confusable. The allowlist inverts the burden: the value is
// accepted only if EVERY rune is one RFC 3986 permits in a URI, which is a
// property of the grammar the value claims to belong to rather than a denylist
// this file has to keep current. `'\r'` needed no clause of its own either way —
// safeTerm strips it before this function sees it (measured), which the old
// four-byte list did not know.
//
// RESIDUAL, stated rather than hidden: this also drops URLs that are legitimate
// but not pure-ASCII — an IDN host or a UTF-8 path that was never
// percent-encoded. The failure direction is the safe one (the message says less
// and is still whole, pinned by TestOffsiteRegisteredAtRejectsUnsafeServerText),
// and a non-ASCII host is itself a homoglyph-spoofing surface in a terminal, so
// dropping it is not merely acceptable here.
//
// 🔴 NO wrapServerText, DELIBERATELY — the hazard it exists for is absent here.
// wrapServerText (workflows_list.go) hard-wraps server text that sits under a
// TABLE, where a soft-wrapped tail landing at column zero is indistinguishable
// from a real row. This string sits inside a multi-sentence error paragraph the
// CLI itself writes at well over 80 columns, so column zero already carries
// CLI-authored wrapped prose and there is no row shape to forge. Worse, it would
// hurt: a URL contains no spaces, so strings.Fields sees ONE token and
// hardSplitOverlong would chop a 400-character link into 75-rune pieces,
// destroying the one thing the reader wants to do with it — paste it.
func offsiteRegisteredAt(d *civitai.AppDetail) string {
	if d == nil {
		return ""
	}
	raw := safeTerm(d.KindData.ExternalURL)
	if raw == "" || strings.ContainsFunc(raw, func(r rune) bool { return !isURIRune(r) }) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return fmt.Sprintf(" (registered at %s)", raw)
}

// offsiteListingRefusal is what `civitai app listing <sub>` says.
//
// 🔴 THE TWO MESSAGES ARE NOT INTERCHANGEABLE AND MUST NOT BE COLLAPSED. This
// one describes a GAP: the app really does have a store listing, its icon and
// cover are serving publicly right now (#422 measured non-null iconUrl/coverUrl
// on all four offsite apps), and the CLI simply has no route that can address
// it. So it names the surface that IS authoritative — the App-store listing UI
// on the website — and is careful to promise no CLI path, because there is
// none. `civitai app view <slug>` is named as the next command because it is the
// one thing the CLI can genuinely do here: show the media that is live.
//
// Internal sentences carry full stops; the LAST one does not. AGENTS.md's
// "lowercase, no trailing punctuation" reads oddly against a multi-sentence
// message, so the tie-break is the two nearest precedents rather than the
// letter of the rule: appViewOwnedAdvice (apps.go) and wantNoSubmissionMsg are
// both multi-sentence, both punctuate internally and both end bare. Same here.
func offsiteListingRefusal(slug string, d *civitai.AppDetail) string {
	return fmt.Sprintf(
		"`%s` is an OFFSITE app%s, and an offsite app's store listing cannot be addressed from this CLI — "+
			"`civitai app listing` resolves a listing through the app's block submission, and an offsite app has none. "+
			"`civitai app submit` is NOT the missing step: it packages a block bundle, and an offsite app is a registered URL, "+
			"so no submission it could create exists to be made.\n"+
			"Its store listing is live all the same — run `civitai app view %s` to see the icon and cover it is serving right now. "+
			"Changing that media is only possible in the App-store listing UI on civitai.com, where this app was registered; "+
			"that surface is authoritative and the CLI has no equivalent for it",
		slug, offsiteRegisteredAt(d), slug)
}

// offsiteStatusRefusal is what `civitai app status <slug>` says.
//
// 🔴 THIS ONE IS NOT A GAP REPORT. `app status` lists BLOCK SUBMISSIONS, and an
// offsite app genuinely has none — there is no missing feature here, only a
// truth told badly by a message that blames the user for not having submitted.
// So it states the fact and points at `civitai app view <slug>`, which is what
// the CLI can actually show about an offsite app. It deliberately does NOT
// mention the store listing UI: nothing about this command is about media.
//
// Trailing punctuation: same rule as offsiteListingRefusal — internal stops,
// none on the last sentence.
func offsiteStatusRefusal(slug string, d *civitai.AppDetail) string {
	return fmt.Sprintf(
		"`%s` is an OFFSITE app%s, so it has no block submissions — `civitai app status` reads the block-submission "+
			"pipeline (submit → review → deploy), and an offsite app is a registered URL rather than a block bundle, "+
			"so there is nothing here to be pending, approved or deployed. Nothing is missing and `civitai app submit` "+
			"would not create one.\n"+
			"Run `civitai app view %s` for what the CLI can show about this app",
		slug, offsiteRegisteredAt(d), slug)
}
