package cmd

import "fmt"

// Shared help text for the PUBLIC READ API command group (models,
// model-versions, images, articles, collections, tags, creators, users).
//
// It is stated ONCE here and interpolated into the eight family bodies rather
// than retyped, for the same reason the `app listing` bodies compute their byte
// caps: a fact typed out eight times is a fact that will be wrong in seven
// places the first time it moves. What is derivable from the code is derived —
// notably every `--limit` bound, which is rendered from the SAME constant
// checkLimit refuses on.

// readAnonNote is the ONE statement of what credentials this group needs, for
// the eight GROUP bodies.
//
// 🔴 IT MUST STAY SCOPED TO THE PUBLIC READ ROUTES. The blanket claim "reads are
// anonymous — no login needed" used to sit in the README over a table that also
// listed `civitai app list` / `app view`, and it was FALSE for those two: the
// store endpoint keys the visible catalog off your identity, so an anonymous
// call is refused outright (see newAppListCmd's Long). PR #268 corrected the
// README by scoping the claim; this constant is that scoped claim, and the
// contrast is kept IN the sentence so the next person to copy it somewhere else
// carries the exception with it.
//
// What the CLI side of this claim rests on is checkable and is checked:
// TestReadAPICommandsRunAnonymously drives every leaf in this group with no
// token configured and asserts it succeeds and sends no Authorization header.
const readAnonNote = `No login is needed — these are public read routes (unlike ` + "`civitai app list`" + `
and ` + "`app view`" + `, which are refused without a token). The CLI still sends your
stored token when you have one; --anon forces an anonymous request.`

// readAnonShort is the leaf-level form. A leaf gets its own docs page, so it
// cannot rely on the group body being read first, but it also cannot afford
// three lines of the same paragraph.
const readAnonShort = `Works anonymously (no login needed); --anon forces an anonymous request even
when you are logged in.`

// readJSONNote states the --json contract once. It is a claim about stdout
// PURITY — the property the whole "Scripting with --json" README section rests
// on — and deliberately not a claim about BYTES.
//
// 🔴 IT MUST NOT SAY "UNCHANGED", AND AN EARLIER REVISION OF THIS CONSTANT DID.
// emitJSON (read.go) re-indents the body through json.Indent. Measured against a
// recording server, `models search --json` over a 113-byte compact body: 194
// bytes on stdout, not byte-identical. So the DOCUMENT is the API's and the
// BYTES are not — and a scripter who took "unchanged" literally would diff or
// hash CLI output against the API body and get a mismatch with nothing wrong.
// The README's "Scripting with --json" section is careful about exactly this: it
// promises "pure JSON — nothing else is written to stdout", never byte-identity.
//
// It also does NOT mention emitJSON's raw-control-byte repair
// (escapeJSONStringControlChars), and that omission is measured rather than an
// oversight: every read command reaches emitJSON only AFTER getInto has
// unmarshalled the same bytes into a typed struct, and encoding/json rejects a
// raw C0 byte inside a string literal there first. Measured: a body carrying a
// literal CR inside a description exits 1 with "unexpected response from
// /api/v1/models (status 200)" and an EMPTY stdout — emitJSON is never entered.
// The repair is real defence-in-depth for any future caller that passes raw
// bytes through without a typed decode; it is not a behaviour of this group, so
// it is not described as one.
const readJSONNote = `--json writes the API response to stdout and nothing else — notes and errors go
to stderr, so ` + "`… --json | jq -e .`" + ` always parses. The document is the API's,
the bytes are not: it is re-indented on the way out, so do not diff or hash it
against the wire.`

// limitRule renders one endpoint's --limit bounds.
//
// 🔴 IT IS COMPUTED FROM THE ENDPOINT'S OWN CEILING CONSTANT AND MUST STAY THAT
// WAY. The number in the help has to be the number checkLimit refuses above,
// and the six ceilings are not the same (100 for models/articles/collections,
// 200 for images/tags/creators) — so a hand-typed bound here is a second copy of
// a constant that reads fine, survives any test that only greps for "a limit
// being mentioned", and goes stale the day a ceiling moves.
// TestReadAPIHelpQuotesTheEnforcedLimit pins the coupling in both directions,
// and pins each ledgered ceiling against what the command ACTUALLY accepts and
// refuses, so the ledger cannot drift into agreeing with a stale help body.
func limitRule(max int) string {
	return fmt.Sprintf("--limit takes 1–%d; omit it for the server's default page size", max)
}

// limitFlagUsage renders the --limit FLAG DESCRIPTION from the same ceiling.
//
// 🔴 THE FLAG BLOCK IS A SECOND PUBLISHED SURFACE, AND IT WAS THE ONE LEFT
// HAND-TYPED. Six read commands bind --limit; four described their ceiling as a
// literal ("(1-100)", "(1-200)") and two stated no bound at all while their Long
// now derives one — so moving a ceiling produced a `--help` page that
// contradicted ITSELF, flag block versus body, with the suite green. The idiom
// already existed thirty lines away in apps.go, which renders this string from
// appsLimitMax; this is that idiom, shared.
// TestReadAPILimitFlagUsageIsDerived pins it.
func limitFlagUsage(max int) string {
	return fmt.Sprintf("results per page (1-%d)", max)
}

// deepPagingNote is the ONE statement of the API's page-offset cap, for the two
// commands that offer both paging modes.
//
// 🔴 STATE WHAT IS PINNED AND WHAT IS NOT. The two halves of this sentence have
// very different standing. That --page is the shallow mode and --cursor the deep
// one is a CLI-side fact and is asserted (TestReadAPIHelpDescribesItsOwnPagingShape
// requires each mode to be named with its role, which is what makes an inversion
// observable). The "1000" and the "429" are SERVER facts with no local
// counterpart anywhere in this repo — pkg/civitai's isDeepPagingCap matches the
// server's PHRASING, not the number — so nothing here can catch them going
// stale. Single-sourcing them at least means they cannot disagree between the
// two commands that quote them, and that is the whole of the guarantee.
// Measured against civitai.com in the README's terms as of 2026-08.
const deepPagingNote = `--page is shallow paging, --cursor is deep paging, and the next cursor is
printed under the results. The API caps page × limit at 1000 and answers 429
past it — the CLI reports that cap as a usage mistake, not a rate limit, so a
retry loop does not spin on it.`

// serverOwnedEnumNote is the honest statement about the ENUM-VALUED filters —
// --type, --sort, --period: the CLI does not model those value sets AT ALL.
//
// It is worded as a claim about the CLI, not about the server, because that is
// the only half this repo can verify: addIfSet copies the flag value into the
// query string unexamined, and the rejection arrives as the API's HTTP 400,
// which readError reports as a usage mistake. Saying "the valid values are X, Y,
// Z" here would be a vendored enum — the thing AGENTS.md item 13 forbids on the
// generate path for the same reason: it buys no correctness, goes stale, and
// starts refusing values the server has since added.
//
// 🔴 ITS ANTECEDENT MUST BE THE ENUM FLAGS ONLY, AND AN EARLIER REVISION PUT IT
// UNDER A LIST THAT ALSO NAMED --nsfw AND --query. That made it FALSE for two
// reasons: --nsfw is a bool cobra parses itself (measured:
// `articles search --nsfw=maybe` is refused client-side, exit 2, with ZERO
// requests made — the opposite of "passed through for the server to reject"),
// and --query/--username have no value set to be wrong about. Each caller now
// names the enum flags immediately above it.
const serverOwnedEnumNote = `The CLI does not check those value sets — it passes the value through and the
server rejects an unknown one with HTTP 400, reported as a usage mistake.`
