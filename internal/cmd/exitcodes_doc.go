package cmd

import (
	"fmt"
	"strings"
)

// The exit-code contract is DOCUMENTED IN ONE PLACE — this file — and rendered
// into both surfaces that publish it: the root `civitai --help` section and the
// "Exit codes" table in README.md.
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
// NewRootCmd), and the README table is asserted byte-identical to what the same
// slice renders (TestREADMEExitCodeTableIsGenerated). Editing either text by
// hand now fails a test until this slice is edited instead, and editing this
// slice moves both. cmd/civitai's exit-code CONSTANTS are pinned against the
// same slice by TestExitCodeConstantsMatchDocs, so a code cannot be added to
// the binary without being documented, or documented without existing.

// ExitCodeDoc is the documented meaning of one process exit code.
//
// Summary and Notes are the SHARED text: they are rendered into `--help` (with
// markdown emphasis stripped) and into the README table verbatim. Extra is
// README-only prose — depth that does not earn its lines in terminal help.
// Nothing `--help` says can therefore contradict the README: it is a prefix of
// it, from one string.
type ExitCodeDoc struct {
	Code    int
	Summary string
	Notes   []string
	Extra   []string
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
var exitCodeDocs = []ExitCodeDoc{
	{
		Code:    0,
		Summary: "Success.",
	},
	{
		Code:    1,
		Summary: "Generic / unclassified error.",
		Notes: []string{
			"A **filesystem failure** lands here — a file that exists but cannot be read, an unwritable config directory, an I/O error. It is neither a mistake about the invocation (`2`) nor a transport failure (`5`), and there is no filesystem-specific code.",
			"A **validation verdict** lands here, and deliberately not on `2`: `civitai app validate` exits `1` when the manifest is invalid, and likewise when the directory you named is a real directory with no `block.manifest.json` at its root — you pointed at a real place, so the invocation was right and the project is wrong. (A path that does **not exist**, or that is not a directory, is the invocation being wrong, and exits `2`.) **When validation produces a result**, `civitai app validate --json` prints it in full and its `ok` field is the structured form of the same answer; a failure that produces no result at all — a project directory the CLI cannot **stat**, say, because it is unreadable or because a path component below it is not a directory — still exits `1` with **nothing on stdout**, so branch on the exit code before parsing.",
			"A resource that **exists but is not ready** lands here too, and deliberately not on `4`: `civitai app metrics <slug>` for an app whose submitted version is still in review exits `1`, because the slug is right and the app does exist — only its analytics do not exist yet, and the error names `civitai app status <slug>` as the next command. `4` stays reserved for a slug with no submissions at all, so the two remain separately actionable: fix the slug, versus wait for approval.",
		},
	},
	{
		Code:    2,
		Summary: "Usage error — a bad flag, a **missing required flag or argument** (e.g. `civitai app withdraw` with no publish-request id), a bad flag **value** (`--limit` out of range, a non-integer id, `--template nope`), or a request the API rejected as malformed (HTTP 400, e.g. a bad `--period`/`--sort` enum).",
		Notes: []string{
			"This does not depend on where the refusal happens: a mistake the CLI catches locally and one the server rejects both exit `2`.",
			"A local image the CLI refuses before uploading anything (`civitai app listing set-icon <file>`, `civitai generate --image`) exits `2` when the file is " + joinPhrases(imageUsageRefusals) + " — but a file that exists and cannot be **read** (permissions, an I/O error) is a filesystem failure rather than a mistake about the invocation, and exits `1`, not `2`.",
			"That split is not images-only and it is not flags-only — it holds for **a flag's value and a positional argument alike**, over the paths listed here: `civitai generate --input <file>` likewise exits `2` for a path that is not there or is a directory, and `1` when the file is there and the read fails.",
			"The project commands take a positional path and refuse it the same way: `civitai app validate <dir>` and `civitai app submit <dir>` exit `2` when the path does not exist **or is not a directory**, because both are mistakes about the invocation. A directory that **does** exist but holds no `block.manifest.json` is a validation verdict instead, and exits `1`.",
		},
		Extra: []string{
			"`app listing set-cover` and `app listing add-screenshot` take the same positional `<file>` and refuse it the same way. (The CLI has no `--file` image flag at all: the only `--file` is `civitai download --file`, which picks a file *inside* a model version.)",
			"**Paths outside that list are not covered, and mostly exit `1`.** `civitai app listing … --dir <missing>` exits `1` (it reports \"no `block.manifest.json` found in …\", the same way it does for a directory that is really there but holds no manifest), and so does `civitai app submit … --out <path under a directory that does not exist>`. Both are stated rather than promised: this is a ledger of the paths the split is published for, not a claim about every path in the CLI.",
			"A usage error emits **no JSON object**, in every mode. `civitai app validate /nope --json` therefore writes nothing to stdout and exits `2`; it used to print `{\"ok\": false, …}` and exit `1`, which reported a nonexistent path as a validation result. Scripts that parsed that object must branch on the exit code first.",
		},
	},
	{
		Code:    3,
		Summary: "Authentication/authorization — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403, or no token configured).",
		Extra: []string{
			"**`civitai generate` refines this**: several of its failures are *not* credential problems but would otherwise land here or on `2`, so they exit `1` instead and a script never loops on `civitai login`. A **muted account or incomplete onboarding** arrives as a bare `403` that is byte-identical to a missing scope; **out of Buzz** and **generation disabled** arrive as `400` (the upstream 403 is re-thrown server-side as a tRPC `BAD_REQUEST`), which would otherwise read as \"bad flags\". See [Generate](#exit-codes-specific-to-generate).",
		},
	},
	{
		Code:    4,
		Summary: "Not found — the requested resource does not exist.",
		Notes: []string{
			"Usually an HTTP 404, but not always: some lookups answer `200` with an empty result set instead (`civitai app status <slug>` for an unregistered slug, `civitai users get` for an unknown username), and those exit `4` too.",
		},
		Extra: []string{
			"The same question therefore exits the same way however the API happens to phrase the miss.",
		},
	},
	{
		Code:    5,
		Summary: "Network/transport failure or service unavailable — dial/timeout, or HTTP 502/503/504 after retries.",
		Notes: []string{
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

// shared returns the text published on BOTH surfaces: Summary plus Notes.
func (d ExitCodeDoc) shared() string {
	return strings.Join(append([]string{d.Summary}, d.Notes...), " ")
}

// readmeCell returns the full markdown for this code's README table cell.
func (d ExitCodeDoc) readmeCell() string {
	return strings.Join(append([]string{d.shared()}, d.Extra...), " ")
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
// a terminal. It is deliberately tiny: Summary and Notes are constrained to
// bold/code emphasis only (pinned by TestSharedExitCodeTextIsTerminalSafe), so
// there is nothing else to handle.
func plainify(s string) string {
	return strings.NewReplacer("**", "", "`", "").Replace(s)
}

const (
	// exitCodeHelpWidth is the total line width of the rendered help section.
	exitCodeHelpWidth = 79
	// exitCodeHelpIndent is the continuation indent; the "    N  " prefix is the
	// same width, so the code column and the text column both line up.
	exitCodeHelpIndent = "       "
)

// rootExitCodeHelp renders the exit-code section of the root command's Long
// help from exitCodeDocs. NewRootCmd calls this — the text is never written out
// by hand, which is what makes `--help` unable to drift from the README.
func rootExitCodeHelp() string {
	var b strings.Builder
	b.WriteString("Exit codes:\n\n")
	b.WriteString("  Every command returns a differentiated exit code so scripts can branch on the\n")
	b.WriteString("  KIND of failure without parsing stderr (the error message itself is unchanged):\n\n")
	for _, d := range exitCodeDocs {
		lines := wrapRunes(plainify(d.shared()), exitCodeHelpWidth-len(exitCodeHelpIndent))
		for i, line := range lines {
			if i == 0 {
				fmt.Fprintf(&b, "    %d  %s\n", d.Code, line)
				continue
			}
			b.WriteString(exitCodeHelpIndent + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// readmeExitCodeTable renders the README's "Exit codes" markdown table from
// exitCodeDocs. TestREADMEExitCodeTableIsGenerated asserts the file matches it
// byte for byte.
func readmeExitCodeTable() string {
	var b strings.Builder
	b.WriteString("| Code | Meaning |\n")
	b.WriteString("| --- | --- |\n")
	for _, d := range exitCodeDocs {
		fmt.Fprintf(&b, "| `%d` | %s |\n", d.Code, d.readmeCell())
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
