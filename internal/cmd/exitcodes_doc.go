package cmd

import (
	"fmt"
	"strings"
)

// The exit-code contract is DOCUMENTED IN ONE PLACE — this file — and rendered
// into both surfaces that publish it: the root `civitai --help` section and the
// "Exit codes" section in README.md.
//
// It is one place because it was two, and they drifted. PR #233 taught
// `app status <unknown-slug>` to exit 4 on an HTTP 200 and a missing required
// flag to exit 2, expanded the README for exactly those cases, and left the
// `--help` section — the place issue #224 named as where the contract lives —
// describing neither. A concurrent docs PR then edited the lines immediately
// above and below that block without touching it. Nothing failed, because
// nothing tied the two texts together.
//
// So: `--help` is GENERATED from exitCodeDocs (see rootExitCodeHelp, called by
// NewRootCmd), and BOTH README blocks — the table AND the per-code subsections
// — are asserted byte-identical to what the same slice renders
// (TestREADMEExitCodeTableIsGenerated, TestREADMEExitCodeSectionsAreGenerated).
// Editing either text by hand now fails a test until this slice is edited
// instead, and editing this slice moves both. cmd/civitai's exit-code CONSTANTS
// are pinned against the same slice by TestExitCodeConstantsMatchDocs, so a code
// cannot be added to the binary without being documented, or documented without
// existing.
//
// 🔴 SUMMARY VS DETAIL — WHAT `--help` DELIBERATELY NO LONGER CARRIES.
//
// Every code's text used to be published in full on both surfaces. That made
// the CONTENT right and the CONTAINER wrong: code 2's entry was one unbroken
// 2,525-character / 432-word / 14-sentence blob covering ~12 distinct topics
// (the enumerated image refusals, the flag-vs-positional split, the project-path
// classification, two residuals, the no-JSON-object rule), and code 1's was
// 1,490. In README it rendered as a wall inside a single markdown table cell; in
// an 80-column terminal it rendered as ~45 unbroken lines with no table, no
// links and no way to skim. A reference nobody can skim is a reference nobody
// reads, which is how the contract's own precision stops protecting anyone.
//
// So each code now carries a short `Summary` and a long `Detail`:
//
//   - `--help` renders `Summary` ONLY — seven skimmable lines, every code
//     present — plus one pointer line naming the README section by title.
//   - README renders the SAME `Summary` as its table (the index) and `Detail`
//     as a `### Exit code N` subsection below it, one bullet per enumerated
//     path and one per residual.
//
// THE TRADE, STATED RATHER THAN DISCOVERED: `--help` no longer carries the full
// ledger. An offline user — no network, no checkout — can learn from the
// terminal WHICH KIND of failure each code names, and cannot learn from the
// terminal which specific paths are covered, that `app listing --dir <missing>`
// and `app submit --out <under a missing dir>` are documented residuals that
// exit 1, or that a usage error emits no JSON object. For those they must open
// README.md's "Exit codes" section. That is why the pointer line names the
// section by title rather than linking a URL: it has to be findable in a local
// checkout, not only on github.com.
//
// What did NOT change: no exit code moved, nothing was paraphrased, and no
// clause was dropped. Every sentence the pre-split contract published is still
// published — asserted mechanically against a fixture of the pre-split text by
// TestEveryPreSplitClauseSurvives, because "I moved it all" is exactly the claim
// a reviewer cannot check by eye across 4 kB of prose.

// ExitCodeDoc is the documented meaning of one process exit code.
//
// Summary is the SHORT text, rendered into BOTH surfaces: `--help` (with
// markdown emphasis stripped) and the README table cell, verbatim. Detail is
// the long text, rendered into the README's per-code subsection ONLY — one
// bullet per entry. See the file header for why the terminal gets the summary.
//
// Nothing `--help` says can therefore contradict the README: the summary line
// it prints IS the README's table row, from one string.
type ExitCodeDoc struct {
	Code    int
	Summary string
	Detail  []string
}

// imageUsageRefusals is the LEDGER of local `<file>` image refusals that exit 2,
// and it is the single source for BOTH the sentence the docs render and the
// cases TestImageUsageRefusalLedger exercises against the real code. Adding a
// phrase here changes the published contract AND obliges the test to prove the
// code honours it; the test fails when the two sets disagree in either
// direction.
//
// 🔴 An unreadable-but-present file is deliberately NOT in this list. The
// README used to claim "an unreadable --file image" exits 2, which was wrong
// twice over: there is no `--file` image flag (the image is a positional
// `<file>`; the only `--file` in the CLI selects a file inside a model version
// for `civitai download`), and a permission-denied read does not exit 2. It
// exits 1, the generic code — see the note in loadAndValidateImage for why
// tagging it here is not the fix. (It exited **5** until issue #241: a
// syscall.Errno satisfies net.Error, so cmd/civitai's isNetworkErr matched
// every filesystem failure in the CLI and reported it as retryable transport.
// The fix is in that classifier, not in a per-call-site tag here.)
var imageUsageRefusals = []string{
	"missing",
	"empty",
	"a directory",
	"over the size cap",
	"not a PNG/JPEG/WebP",
}

// exitCodeDocs is the contract. Order is by code and must stay dense from 0.
//
// 🔴 EVERY Detail STRING BELOW IS VERBATIM PRE-SPLIT TEXT. The clauses in codes
// 1 and 2 in particular are the product of two published corrections — the
// flag-vs-positional sentence shipped once over-narrow ("every local path a FLAG
// names", which excluded the two commands that took the path positionally) and
// once over-broad ("every local path the CLI is HANDED", which had a live
// counterexample inside its own scope). Their precision is what those rounds
// bought. Do not compress them because a bullet reads long.
var exitCodeDocs = []ExitCodeDoc{
	{
		Code:    0,
		Summary: "Success.",
	},
	{
		Code:    1,
		Summary: "Generic / unclassified error. A **filesystem failure** lands here, and so does a **validation verdict** — an invalid manifest, or a real directory holding no manifest.",
		Detail: []string{
			"A **filesystem failure** lands here — a file that exists but cannot be read, an unwritable config directory, an I/O error. It is neither a mistake about the invocation (`2`) nor a transport failure (`5`), and there is no filesystem-specific code.",
			"A **validation verdict** lands here, and deliberately not on `2`: `civitai app validate` exits `1` when the manifest is invalid, and likewise when the directory you named is a real directory with no `block.manifest.json` at its root — you pointed at a real place, so the invocation was right and the project is wrong. (A path that does **not exist**, or that is not a directory, is the invocation being wrong, and exits `2`.)",
			"**When validation produces a result**, `civitai app validate --json` prints it in full and its `ok` field is the structured form of the same answer; a failure that produces no result at all — a project directory the CLI cannot **stat**, say, because it is unreadable or because a path component below it is not a directory — still exits `1` with **nothing on stdout**, so branch on the exit code before parsing. The full exit→stdout table is in [The `--json` result shape](#the---json-result-shape).",
			"A resource that **exists but is not ready** lands here too, and deliberately not on `4`: `civitai app metrics <slug>` for an app whose submitted version is still in review exits `1`, because the slug is right and the app does exist — only its analytics do not exist yet, and the error names `civitai app status <slug>` as the next command. `4` stays reserved for a slug with no submissions at all, so the two remain separately actionable: fix the slug, versus wait for approval.",
			// 🔴 ADDED, NOT MERGED INTO THE BULLET ABOVE. That bullet is verbatim
			// pre-split text and TestEveryPreSplitClauseSurvives requires both its
			// sentences to stay published word for word, so the correction has to
			// arrive beside them. "Wait for approval" was true for the only state
			// anyone had measured (pending) and false for the two the CLI can also
			// be handed, and `app metrics` printed "check where it is in review"
			// for all three until pullReviewAdvice became shared.
			// 🔴 ADDED for the monotonic-version guard (issue #412). It belongs on
			// 1 rather than 2 for the reason the validation-verdict bullet above
			// already gives: the invocation is right (every flag, argument and
			// path is well-formed) and the PROJECT is wrong relative to what is
			// published. Pinned by TestVersionRegressionExitsGeneric.
			"A **version regression** lands here for the same reason: `civitai app submit` refuses when the manifest version is not strictly **above the highest approved version** of that app, because approving an older (or identical) version replaces the newer live deployment. Nothing about the invocation is wrong, so it is a verdict about the project, not a `2`. `--allow-downgrade` is the deliberate-rollback escape hatch, and the guard is skipped entirely by `--package-only` or a run with no token — neither reaches the server.",
			// 🔴 ADDED for the dirty-work-tree guard (issue #411). Same code and
			// the same argument as the version regression above it: the
			// invocation is well-formed and the PROJECT is wrong — here, its
			// working tree against its own history. Pinned by
			// TestDirtyWorkTreeExitsGeneric.
			"A **dirty git work tree** lands here too: `civitai app submit` refuses while files that go into the bundle are uncommitted, because the bundle is packaged from what is on disk and approving one deploys code that exists in no commit. `--allow-dirty` submits the tree as it is. It **degrades rather than enforcing** — a directory that is not in a git repo, or a machine with no `git` on `PATH`, submits exactly as before (scaffolded apps have no repo, and that path must keep working), and a clean tree whose `HEAD` is on no remote **warns** instead of refusing. Like the version guard it is skipped by `--package-only` and by a run with no token.",
			// 🔴 ADDED for the provenance STAMP half of the same issue (#411).
			// It is here because the question a reader arrives with is "can this
			// new field fail my submit?", and the answer belongs beside the
			// guard's own bullet rather than only in the README section.
			"The **build-provenance stamp** those two guards now also collect (issue #411 — the commit `civitai app submit` reports and `civitai app status` shows) changes no exit code at all, in either direction. It is sent only when the CLI can establish a value in exactly the shape the server accepts (`^[0-9a-f]{40}$`); every branch that cannot — no repo, no `git` on `PATH`, a repo with no commits, or an answer it does not recognise — sends nothing and submits exactly as it did before. A submit that would have succeeded cannot fail because of it, and a missing stamp is never an error.",
			"**\"Wait for approval\" is the *pending* case only.** The same `1` covers an app whose latest submission was **rejected** or **withdrawn** — nothing is in review there, so `civitai app metrics <slug>` says so and names a new `civitai app submit` as the next step instead of a review to wait for. What separates `1` from `4` is unchanged: the slug is right and the app exists.",
		},
	},
	{
		Code:    2,
		Summary: "Usage error — a bad flag, a **missing required flag or argument**, a bad flag **value**, or a path that does not exist / is not a directory.",
		Detail: []string{
			"Usage error — a bad flag, a **missing required flag or argument** (e.g. `civitai app withdraw` with no publish-request id), a bad flag **value** (`--limit` out of range, a non-integer id, `--template nope`), or a request the API rejected as malformed (HTTP 400, e.g. a bad `--period`/`--sort` enum).",
			"This does not depend on where the refusal happens: a mistake the CLI catches locally and one the server rejects both exit `2`.",
			"A local image the CLI refuses before uploading anything (`civitai app listing set-icon <file>`, `civitai generate --image`) exits `2` when the file is " + joinPhrases(imageUsageRefusals) + " — but a file that exists and cannot be **read** (permissions, an I/O error) is a filesystem failure rather than a mistake about the invocation, and exits `1`, not `2`.",
			"That split is not images-only and it is not flags-only — it holds for **a flag's value and a positional argument alike**, over the paths listed here: `civitai generate --input <file>` likewise exits `2` for a path that is not there or is a directory, and `1` when the file is there and the read fails.",
			"The project commands take a positional path and refuse it the same way: `civitai app validate <dir>` and `civitai app submit <dir>` exit `2` when the path does not exist **or is not a directory**, because both are mistakes about the invocation. A directory that **does** exist but holds no `block.manifest.json` is a validation verdict instead, and exits `1`.",
			"`app listing set-cover` and `app listing add-screenshot` take the same positional `<file>` and refuse it the same way. (The CLI has no `--file` image flag at all: the only `--file` is `civitai download --file`, which picks a file *inside* a model version.)",
			"**Paths outside that list are not covered, and mostly exit `1`.** `civitai app listing … --dir <missing>` exits `1` (it reports \"no `block.manifest.json` found in …\", the same way it does for a directory that is really there but holds no manifest), and so does `civitai app submit … --out <path under a directory that does not exist>`. Both are stated rather than promised: this is a ledger of the paths the split is published for, not a claim about every path in the CLI.",
			"A usage error emits **no JSON object**, in every mode. `civitai app validate /nope --json` therefore writes nothing to stdout and exits `2`; it used to print `{\"ok\": false, …}` and exit `1`, which reported a nonexistent path as a validation result. Scripts that parsed that object must branch on the exit code first.",
		},
	},
	{
		Code:    3,
		Summary: "Not authorized — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403).",
		Detail: []string{
			"Authentication/authorization — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403, or no token configured).",
			"**`civitai generate` refines this**: several of its failures are *not* credential problems but would otherwise land here or on `2`, so they exit `1` instead and a script never loops on `civitai login`. A **muted account or incomplete onboarding** arrives as a bare `403` that is byte-identical to a missing scope; **out of Buzz** and **generation disabled** arrive as `400` (the upstream 403 is re-thrown server-side as a tRPC `BAD_REQUEST`), which would otherwise read as \"bad flags\". See [Generate](#exit-codes-specific-to-generate).",
		},
	},
	{
		Code:    4,
		Summary: "Not found — the requested resource does not exist.",
		Detail: []string{
			"Usually an HTTP 404, but not always: some lookups answer `200` with an empty result set instead (`civitai app status <slug>` for an unregistered slug, `civitai users get` for an unknown username), and those exit `4` too.",
			"The same question therefore exits the same way however the API happens to phrase the miss.",
			"**An app that EXISTS but is `offsite` exits `4` from `civitai app status <slug>`, and that is deliberate.** That command resolves through the app's block submission, which an offsite app never has, so the *resource it looks up* is genuinely absent even though the app is not. Only the message changes — where the CLI can tell, it says the app is offsite and names a next step that can work, instead of `civitai app submit`, which cannot. This is **not** the `has no approved App Block yet` case on `1` above, which exits `1` because the thing looked up (analytics) is expected to appear later; an offsite app's block submission never will.",
			"**`civitai app listing …` no longer exits `4` for an offsite app in the normal case** — it falls back to selecting the listing by slug and succeeds ([#422](https://github.com/civitai/cli/issues/422), needing `civitai/civitai#3989` server-side). It still exits `4` when that fallback *also* answers **not-found**: a Civitai without `#3989` (older or self-hosted), or an app with no listing row. A listing your account does not own is **not** in that set on a current server — the by-slug lookup resolves it and then refuses it `403`, so it exits `3`.",
			"**A lookup failure that is not a 404 keeps its own code, and that is not always `3` or `5`.** Neither of the two lookups is retried past a non-404 — a `403`, a `5xx` or a transport failure says nothing about whether the resource exists — so you get the failing lookup's own error and its own code. **Measured** end-to-end on both lookups (`cmd/civitai/app_listing_lookup_exitcode_test.go`): `401`/`403` → `3`, `429` → `6`, `502`/`503`/`504` and a transport failure → `5`, and a plain **`500` → `1`**, because that status carries no classification sentinel at all. Do not read \"the API failed\" as \"exit `5`\": the code to retry on is `5`, and a `500` is not it.",
			"**The offsite wording is best-effort, and the exit code is not.** Naming an app as offsite costs one extra lookup against the public store catalog (`GET /api/v1/apps/{slug}`), and that route answers only for a **published** store listing and is itself still behind a launch flag — until the catalog opens publicly you see an app there only as a moderator or app-dev-tester. So an offsite app whose listing is not published, a caller without that access, or any network/5xx failure of the lookup all keep the generic `no such submission … run civitai app submit first` message. **Exit `4` either way**, so a script branching on the code is unaffected; only the human-readable half degrades, and always in that direction.",
		},
	},
	{
		Code:    5,
		Summary: "Network/transport failure or service unavailable — the code to **retry** on.",
		Detail: []string{
			"Network/transport failure or service unavailable — dial/timeout, or HTTP 502/503/504 after retries.",
			"This is the code to **retry** on, so a **filesystem** failure never lands here however retryable its errno looks: a permissions or I/O problem does not fix itself, and a loop that sleeps and re-runs would never terminate. Those exit `1`.",
		},
	},
	{
		Code:    6,
		Summary: "Rate limited — throttled by the API (HTTP 429).",
	},
}

// ExitCodeDocs returns a copy of the exit-code contract, for callers outside
// this package (cmd/civitai pins its constants against it).
func ExitCodeDocs() []ExitCodeDoc {
	out := make([]ExitCodeDoc, len(exitCodeDocs))
	copy(out, exitCodeDocs)
	return out
}

// published returns EVERYTHING this code publishes anywhere: Summary plus every
// Detail bullet. It is the union the contract-claim ledger and the clause
// preservation guard assert against — neither cares which surface a sentence
// reaches, only that the contract still carries it.
func (d ExitCodeDoc) published() string {
	return strings.Join(append([]string{d.Summary}, d.Detail...), " ")
}

// joinPhrases renders a list as an Oxford-comma "a, b, or c" clause.
func joinPhrases(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " or " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", or " + items[len(items)-1]
}

// plainify strips the markdown emphasis the README needs from text destined for
// a terminal. It is deliberately tiny: Summary is constrained to bold/code
// emphasis only (pinned by TestSummaryTextIsTerminalSafe), so there is nothing
// else to handle. Detail never reaches a terminal and may carry links.
func plainify(s string) string {
	return strings.NewReplacer("**", "", "`", "").Replace(s)
}

const (
	// exitCodeHelpWidth is the total line width of the rendered help section.
	exitCodeHelpWidth = 79
	// exitCodeHelpIndent is the continuation indent; the "    N  " prefix is the
	// same width, so the code column and the text column both line up.
	exitCodeHelpIndent = "       "
	// exitCodeHelpLead is the one-line orientation above the code list.
	exitCodeHelpLead = "Branch on the KIND of failure without parsing stderr — the error message itself is unchanged."
	// exitCodeREADMESection is the README heading the pointer line names. It is
	// a TITLE, not a URL, because the reader it is for has a checkout and may
	// have no network. TestHelpPointsAtTheREADMESection asserts the heading
	// really exists in README.md, so the pointer cannot rot into a dead end.
	exitCodeREADMESection = "Exit codes"
	// exitCodeHelpPointer is the accepted cost of the summary/detail split,
	// printed where the reader meets its consequence. See the file header.
	exitCodeHelpPointer = "Full ledger — which paths each code covers, the exit→stdout table, and the two documented residuals: see the " + exitCodeREADMESection + " section of README.md."
)

// rootExitCodeHelp renders the exit-code section of the root command's Long
// help from exitCodeDocs. NewRootCmd calls this — the text is never written out
// by hand, which is what makes `--help` unable to drift from the README.
//
// It renders Summary only. Detail is README-only by design; the pointer line
// says so rather than leaving a terminal reader to assume this is everything.
func rootExitCodeHelp() string {
	var b strings.Builder
	b.WriteString("Exit codes:\n\n")
	for _, line := range wrapRunes(exitCodeHelpLead, exitCodeHelpWidth-2) {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("\n")
	for _, d := range exitCodeDocs {
		lines := wrapRunes(plainify(d.Summary), exitCodeHelpWidth-len(exitCodeHelpIndent))
		for i, line := range lines {
			if i == 0 {
				fmt.Fprintf(&b, "    %d  %s\n", d.Code, line)
				continue
			}
			b.WriteString(exitCodeHelpIndent + line + "\n")
		}
	}
	b.WriteString("\n")
	for _, line := range wrapRunes(plainify(exitCodeHelpPointer), exitCodeHelpWidth-2) {
		b.WriteString("  " + line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// exitCodeAnchor is the README heading for one code's Detail subsection, and the
// anchor the table row links to. Both are derived here so the link and the
// heading cannot disagree.
func exitCodeHeading(code int) string { return fmt.Sprintf("Exit code %d", code) }

func exitCodeAnchor(code int) string { return fmt.Sprintf("#exit-code-%d", code) }

// readmeExitCodeTable renders the README's "Exit codes" markdown table from
// exitCodeDocs. TestREADMEExitCodeTableIsGenerated asserts the file matches it
// byte for byte.
//
// The cell is the Summary — the table is the INDEX. A code carrying Detail gets
// a trailing link into its subsection, so the row a reader lands on names where
// the rest of its contract is.
func readmeExitCodeTable() string {
	var b strings.Builder
	b.WriteString("| Code | Meaning |\n")
	b.WriteString("| --- | --- |\n")
	for _, d := range exitCodeDocs {
		cell := d.Summary
		if len(d.Detail) > 0 {
			cell += fmt.Sprintf(" [Detail](%s)", exitCodeAnchor(d.Code))
		}
		fmt.Fprintf(&b, "| `%d` | %s |\n", d.Code, cell)
	}
	return strings.TrimRight(b.String(), "\n")
}

// readmeExitCodeSections renders the per-code `### Exit code N` subsections —
// the long half of the contract, one bullet per Detail entry.
// TestREADMEExitCodeSectionsAreGenerated asserts the file matches it byte for
// byte, so these are generated exactly as the table is; a code with no Detail
// gets no subsection and no link.
func readmeExitCodeSections() string {
	var b strings.Builder
	for _, d := range exitCodeDocs {
		if len(d.Detail) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", exitCodeHeading(d.Code))
		for _, item := range d.Detail {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// wrapRunes greedily wraps on spaces at width RUNES (not bytes — the summaries
// carry em dashes, and a byte-width wrap would break them short and unevenly).
func wrapRunes(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var (
		lines []string
		cur   string
		n     int
	)
	for _, w := range words {
		lw := len([]rune(w))
		if cur == "" {
			cur, n = w, lw
			continue
		}
		if n+1+lw > width {
			lines = append(lines, cur)
			cur, n = w, lw
			continue
		}
		cur += " " + w
		n += 1 + lw
	}
	return append(lines, cur)
}
