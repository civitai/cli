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
// The OFFSITE refusal (civitai/cli#422, outcome 2) — now `app status`'s alone,
// plus `app listing`'s NARROWED fallback (#422 outcome 1)
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
// 🔴 THE `app listing` HALF IS NO LONGER A REFUSAL — #422 OUTCOME 1 SHIPPED, AND
// THIS FILE IS NOW ITS FALLBACK RATHER THAN ITS ANSWER. The paragraph that stood
// here said `appListings.getMyListingForApp` "cannot do it: it was measured live
// to resolve ONLY by `appBlockId` … the slug selector 404s for every app, onsite
// positive controls included". That measurement was true of the server it was
// taken against and is now STALE: `civitai/civitai#3989` rescoped the slug arm
// from `{slug, kind:'onsite', appBlockId:null, status:'draft'}` to
// `{slug, revisionOfId:null}`, and it is DEPLOYED. Measured live 2026-08-17
// against https://civitai.com — `radio`, `comfy`, `cosmetic-studio` and
// `vitrine` (4/4 offsite, all approved) each resolved BY SLUG to their `apl_`
// listing, `gen-matrix` (onsite, approved) still resolved, and
// `definitely-not-an-app-zzz` still 404ed. So `resolveListing` (app_listing.go)
// now falls back to the slug selector when the submission lookup 404s, and every
// `app listing` subcommand reaches an offsite app.
//
// 🔴 WHAT SURVIVES, AND WHY THIS FILE DID NOT SIMPLY GO AWAY. Two things.
// (a) `app status <slug>` is UNCHANGED and its refusal is still TRUE — it reads
// the block-submission pipeline, an offsite app genuinely has no block
// submissions, and no listing selector repairs that. Do not collapse the two
// messages now that only one of them reports a gap; see offsiteStatusRefusal.
// (b) The `app listing` refusal NARROWED rather than vanished: it is what a
// caller gets when the by-slug lookup ALSO returns NOT-FOUND — a server that
// predates #3989 (older or self-hosted), or an app with no listing row.
// A listing the caller does not OWN is no longer in that list: `getMyListingForApp`
// answers NOT_OWNED → FORBIDDEN for it (`resolveListingRole`,
// `civitai/civitai → src/server/services/blocks/offsite-listing.service.ts:1772`,
// mapped at `app-listings.router.ts:637-654`), and resolveListing returns that 403
// with its own exit code instead of folding it into this message — so it reaches
// the refusal only on a server that predates #3989, where the same listing 404s.
// `offsiteListingRefusal` was reworded to match; it no longer claims the listing
// "cannot be addressed from this CLI", because in the normal case it now can. The
// upstream ask that produced #3989 was civitai/civitai#3984.
//
// 🔴 `appBlockId` IS NOT A KIND DISCRIMINATOR, AND THIS COMMENT USED TO SAY IT
// WAS. The sentence above read "an offsite app has no appBlockId — that is
// what `kind: offsite` MEANS" until 2026-08-16. That absolute is false. The
// server schema says so in as many words
// (`packages/civitai-db-schema/prisma/schema.full.prisma:2849-2853` in
// `civitai/civitai`): the 1:1 backing AppBlock is "Set for EVERY backfilled row
// — on-site AND the #2821 off-site rows (both come from an AppBlock). It is NOT
// a kind discriminator: discriminate on `kind`, never on appBlockId nullness.
// Only a natively-created off-site listing (no backing AppBlock) leaves it
// NULL." The `kind: offsite` + non-null `appBlockId` shape is measured at 0 rows
// in production (2026-08-11, `src/components/Apps/appListingEditorTabs.ts`), so
// it is EMPIRICALLY ABSENT TODAY, STRUCTURALLY POSSIBLE, and a backfilled class
// for it exists in the schema. Nothing about the refusal changes: #422's 4/4
// offsite / 7/7 onsite measurement stands, and a NATIVELY-created offsite app —
// which is what all four measured ones are — genuinely has no AppBlock. What
// changes is what may be INFERRED: never read appBlockId nullness as the kind,
// which is why offsiteApp below branches on `d.Kind` and nothing else.
//
// 🔴 "NO CLIENT PATH EXISTS" WAS RETRACTED IN PART BEFORE #3989 LANDED, AND THE
// COUNTEREXAMPLE IS IN THIS FILE. It is recorded because it is the reasoning that
// stopped the refusal being treated as proof — the WORKAROUND it describes was
// never built, and must still not be: the shipped repair is the one-hop slug
// selector above, not this two-hop route through the public catalog.
// `offsiteApp` below calls GET /api/v1/apps/{slug}, whose payload
// carries the `AppListing` row id — `civitai.AppDetail.ID`, an `apl_` ULID,
// measured non-null for an offsite app (`radio` → `apl_01KYNB77D490DM6YS0C5Z7KYT7`,
// 2026-08-16). Server-side, `getMyListingForEdit` and every listing-keyed asset
// proc gate on OWNERSHIP only — no kind check — so a client path plausibly exists
// for an APPROVED offsite app. It is UNVERIFIED (probing it mints a shadow
// revision on a live listing) and it is not a drop-in: that route is
// approved-only and scope-gated, so a draft or pending offsite listing is
// invisible to it. Read claudedocs/decisions/29-offsite-refusal.md BEFORE either
// restating the absolute or building on the workaround.
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
// 1. CLOSED 2026-08-17 (civitai/cli#427) — kept as the record of WHY the wording
//    is what it is, because the constraint that produced it has not gone away.
//    offsiteApp still performs no ownership check, and GET /api/v1/apps/{slug} is
//    still the PUBLIC catalog, so the app whose kind is read may belong to a
//    stranger. #422 outcome 1 closed the absolute — the sentence "an offsite
//    app's store listing cannot be addressed from this CLI" is deleted, and its
//    replacement names the stranger's case as the one refused 403 ELSEWHERE
//    rather than reported here. What remained false for a stranger was the advice
//    that followed: "Changing that media always works in the App-store listing UI
//    on civitai.com, where this app was registered" read as an instruction to a
//    person who could act on it, and they cannot. That clause now says the media
//    is MANAGED there by the account that registered the app — a fact about the
//    listing rather than a promise to the reader — so it is true whoever runs the
//    command. (`civitai app view <slug>`, the other command named, is a public
//    read and was already true for anyone.)
//    🔴 THE FIX IS A NARROWED CLAIM, NOT AN ACQUIRED SIGNAL. These two commands
//    still have no ownership signal on the error path, and the CLI cannot infer
//    one from the by-slug miss: `getMyListingForApp` refuses a listing you do not
//    own with NOT_OWNED (→ FORBIDDEN), which resolveListing returns AS a 403
//    rather than folding into this message at all — so the stranger case that
//    reaches here is the one where the store catalog knows the app and the
//    listings route does not answer for it. Anything added to this message that
//    presumes the reader owns the app reopens this residual.
//
// 2. THE ERROR PATH NOW COSTS THREE BOUNDED REQUESTS, WITH NO OFF SWITCH. Every
//    `no such submission` — the ordinary "I have not submitted yet" case, which
//    is far more common than an offsite app — pays the submissions lookup that
//    already failed, then resolveListing's by-slug fallback (30s, the client's
//    own per-request budget — see the 🟡 note on resolveListing for why that one
//    is deliberately NOT capped at a diagnostic's 5s), then this probe (5s)
//    before printing. It was ONE extra request when #422 outcome 2 shipped; the
//    fallback added the second, and this line said "one" for the length of that
//    PR. That is a COST, not a defect: it buys the diagnosis and the offsite
//    repair, it cannot make the command fail differently (every probe failure
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
// AGENTS.md item 29 for why. That item's body now lives in
// claudedocs/decisions/29-offsite-refusal.md; it was inline while #426 was its
// first appearance, because a first-appearance item has no base commit where its
// body already stood in AGENTS.md and so cannot carry the
// agents_split_preserved_test.go provenance row a split body needs. Once #426
// merged, that base existed. (An earlier draft of this line cited a decision
// file that did not yet exist; TestSourceCitationsOfDecisionFilesResolve fails on
// that class, and the citation above now resolves.)
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
// this file has to keep current.
//
// MEASURED, AND NOT WHAT THE FOUR-BYTE LIST BELIEVED ABOUT ITSELF: exactly ONE
// of those four bytes was ever load-bearing here. `'\r'` never reaches this
// function — safeTerm strips it first. `'\t'` and `'\n'` are rejected by
// `url.Parse` below ("invalid control character in URL"), so adding either to
// the allowlist changes nothing observable. Only `' '` parses cleanly into a
// real host, which makes the allowlist the only thing standing between a server
// string and a forged clause. Stated so the next reader does not infer, from
// three tab/newline/space rows in the test, that the predicate is what stops
// them — for two of the three it is not.
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

// offsiteListingRefusal is what `civitai app listing <sub>` says when BOTH of
// resolveListing's lookups answered NOT-FOUND.
//
// 🔴 IT NO LONGER SAYS THE LISTING "CANNOT BE ADDRESSED FROM THIS CLI", BECAUSE
// IN THE NORMAL CASE IT NOW CAN (civitai/cli#422 outcome 1). That sentence was
// this message's thesis until `civitai/civitai#3989` deployed; keeping it would
// be the file header's own hazard — a claim the code contradicts — asserted to
// the one person who cannot check it. What is left is a report about THIS
// invocation: the CLI could not reach the listing HERE, and the ways that happens
// are named so the reader can tell which one they are in.
//
// 🔴 "A LISTING THIS ACCOUNT DOES NOT OWN" IS NAMED AS THE CASE THAT DOES *NOT*
// LAND HERE, and that is the second claim this message had to stop making. It
// listed not-owned as one of three equal causes; on a server carrying #3989 that
// is false — the by-slug arm resolves the row and then refuses it 403 (NOT_OWNED
// → FORBIDDEN, see the file header for the upstream file:line), which
// resolveListing now returns as a 403 rather than swallowing. Not-owned reaches
// this message only on a server that predates #3989, where the narrower `where`
// clause excludes the row and it 404s like an absent one. Saying so is the
// difference between a reader who checks their account and one who does not.
//
// 🔴 THE TWO MESSAGES ARE NOT INTERCHANGEABLE AND MUST NOT BE COLLAPSED, and the
// asymmetry GREW rather than shrank. offsiteStatusRefusal reports a permanent
// truth (an offsite app has no block submissions, ever); this one reports a
// lookup that missed on this server, this account, this app. It still names the
// App-store listing UI on civitai.com and `civitai app view <slug>`, the one read
// the CLI can always do here.
//
// 🔴 IT NAMES THAT UI AS WHERE THE MEDIA IS MANAGED, NOT AS A SURFACE THAT WILL
// WORK FOR THE READER, and the difference is civitai/cli#427 residual 1. This
// clause read "Changing that media always works in the App-store listing UI on
// civitai.com, where this app was registered" until 2026-08-17 — an instruction
// addressed to someone who can act on it. offsiteApp does NO ownership check and
// GET /api/v1/apps/{slug} is the PUBLIC catalog, so the app may be a stranger's
// and that sentence was then a promise the reader cannot keep. The current
// wording states where the media lives and who administers it, which is true
// whoever is reading; `civitai app view <slug>`, the other command named, is a
// public read and was always true for anyone. The CLI still has no ownership
// signal on this path — the fix is to stop CLAIMING one, not to acquire one.
//
// It still refuses to name `civitai app submit`: that step cannot exist for an
// offsite app whichever of the three cases this is.
//
// Internal sentences carry full stops; the LAST one does not. AGENTS.md's
// "lowercase, no trailing punctuation" reads oddly against a multi-sentence
// message, so the tie-break is the two nearest precedents rather than the
// letter of the rule: appViewOwnedAdvice (apps.go) and wantNoSubmissionMsg are
// both multi-sentence, both punctuate internally and both end bare. Same here.
func offsiteListingRefusal(slug string, d *civitai.AppDetail) string {
	return fmt.Sprintf(
		"`%s` is an OFFSITE app%s, and this CLI could not reach its store listing here — "+
			"`civitai app listing` resolves a listing through the app's block submission, which an offsite app has none of, "+
			"and then by the slug alone; neither answered. `civitai app submit` is NOT the missing step: it packages a block "+
			"bundle, and an offsite app is a registered URL, so no submission it could create exists to be made.\n"+
			"The by-slug lookup reaches an offsite listing only on a Civitai carrying civitai/civitai#3989, so an older or "+
			"self-hosted server, or an app with no listing row at all, each land here; a listing this account does not own "+
			"is refused as a 403 by any server carrying #3989, and reaches this message only on one that does not.\n"+
			"Its store listing may be live all the same — run `civitai app view %s` to see the icon and cover it is serving. "+
			"That media is managed in the App-store listing UI on civitai.com by the account that registered the app; "+
			"that surface is authoritative whatever this CLI can reach",
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
