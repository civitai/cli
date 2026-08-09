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
// purity that the whole "Scripting with --json" README section rests on.
const readJSONNote = `--json prints the raw /api/v1 response on stdout, unchanged — notes and errors
go to stderr, so ` + "`… --json | jq -e .`" + ` always parses.`

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

// serverOwnedEnumNote is the honest statement about --sort / --period / --type /
// --base-model: the CLI does not model those value sets AT ALL.
//
// It is worded as a claim about the CLI, not about the server, because that is
// the only half this repo can verify: addIfSet copies the flag value into the
// query string unexamined, and the rejection arrives as the API's HTTP 400,
// which readError reports as a usage mistake. Saying "the valid values are X, Y,
// Z" here would be a vendored enum — the thing AGENTS.md item 13 forbids on the
// generate path for the same reason: it buys no correctness, goes stale, and
// starts refusing values the server has since added.
const serverOwnedEnumNote = `The CLI does not check these value sets — it passes them through and the server
rejects an unknown one with HTTP 400, which the CLI reports as a usage mistake.`
