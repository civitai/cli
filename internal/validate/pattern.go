package validate

import (
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// pattern.go makes a `pattern` schema violation READ like the ported semantic
// checks instead of like a regex dump (issue #260, item 7).
//
// The base library message is honest but terse:
//
//	blockId: 'My First App!' does not match pattern '^[a-z][a-z0-9-]*[a-z0-9]$'
//
// while the enum violations in the SAME command already name the rule and the
// valid set:
//
//	contentRating: value must be one of 'g', 'pg', 'pg13', 'r', 'x'
//
// So a `pattern` finding now carries the rule in English AND a value that
// satisfies it. The regex STAYS in the message: it is the authority, it is
// greppable, and a reader who knows regex should not have to trust our prose.
// The gloss is appended, never substituted.
//
// 🔴 THE TABLE IS KEYED ON THE REGEX SOURCE, NOT ON THE FIELD PATH. Keying on
// the field would be a second, hand-maintained map of "which fields have a
// pattern", wrong the moment the schema moves one; and it would say nothing
// about a pattern reached through `$ref` or reused on two fields. The regex is
// the thing the finding is actually ABOUT, so it is the key, and one gloss
// covers every field the schema applies that pattern to.
//
// 🔴 IT FAILS SOFT. An unglossed pattern emits exactly the base message it
// emits today — terse, never wrong. That is what makes this safe to sit in
// front of a VENDORED mirror (AGENTS.md item 1): a schema update that adds a
// pattern degrades to the old output rather than to a stale English claim about
// a rule that changed.
//
// So the coverage of this table is a NOTIFICATION concern, not a correctness
// one, and `TestPatternRulesCoverTheVendoredSchema` is shaped accordingly. It is
// a TOTAL ledger: every pattern the vendored schema can surface must be either
// glossed here or recorded as an acknowledged debt in `pattern_gloss_owed.json`.
//
//   - A pattern the schema GAINS can be cleared mechanically, by recording the
//     debt (`go test ./internal/validate -run
//     TestPatternRulesCoverTheVendoredSchema -update-gloss-ledger -count=1`).
//     That is what lets `revendor-canonical-schema.yml` land a pattern-adding
//     canonical change unaided — it runs the suite on its own output before
//     opening a PR, so an unconditional failure here made the automation whose
//     job is to land that change unable to land it (#486 and #743). The cost was
//     not the stalled bot: `ci.yml`'s `schema-drift` job runs on every PR in this
//     repo with no path filter and stays red while the vendored mirror is stale,
//     so a blockage here reddens a check on unrelated human PRs until someone
//     writes a gloss. A human is still told: the debt row is in the bot's PR
//     diff, and the workflow keeps a `gloss-owed` issue open until it is cleared.
//     (⚠️ An earlier revision listed "#323, #486, #607, #695, #743" as five
//     monthly blockages of this guard. Retracted — #323 was a hand-dispatched
//     fire drill under a different title, #607 was the enum half alone, and #695
//     was an `internal/pkgzip` test that files under the same headline because
//     the bot's gate is `go test ./...`. The full table is above the `owed` step
//     in `.github/workflows/revendor-canonical-schema.yml`.)
//   - A pattern the schema DROPS while a gloss survives is NOT mechanically
//     clearable and still fails the suite. A row claiming to explain a rule that
//     no longer exists reads as coverage, and deleting prose is a human call.
//
// 🔴 A ROW IN `pattern_gloss_owed.json` IS A DEBT, NOT A DECISION. Clearing it
// means writing the gloss here — plus its ledger row in
// TestPatternGlossesAreTheRightWayRound and its tripping fixture in
// patternFixtures, both of which are total against this table.
//
// The four `not: {"pattern": …}` sub-schemas under `outputDir` are deliberately
// NOT in this table and cannot be: a failing `not` surfaces as `kind.Not`,
// whose message is the bare "not failed" and which carries no keyword path, no
// regex and no value at all — there is nothing to key a gloss on. `outputDir`
// is covered instead by buildCoherence's own ported messages. See the residual
// note in AGENTS.md.

// patternRule is the author-facing gloss for one schema `pattern`.
type patternRule struct {
	// rule states the constraint in English, in the imperative the other
	// findings use ("must be …"). It describes the REGEX, not the field, so it
	// stays true wherever the schema applies that pattern.
	rule string
	// example is a value that SATISFIES the pattern. It is a literal rather
	// than something derived, because the point is that an author can copy it.
	example string
}

// patternRules maps a schema `pattern` regex source to its gloss.
//
// Every entry is a claim about the regex it is keyed on. When you add one,
// check the example against the regex — TestPatternRuleExamplesSatisfyTheir
// Pattern compiles each key and requires the example to match, so a gloss
// cannot ship an example the schema would reject.
var patternRules = map[string]patternRule{
	// blockId — the highest-traffic case: the first field a new author gets
	// wrong, and a permanent public identity that cannot be renamed later.
	`^[a-z][a-z0-9-]*[a-z0-9]$`: {
		rule:    "must be lowercase letters, digits and hyphens only, starting with a letter and ending with a letter or digit",
		example: "my-first-app",
	},
	// goods[].id — the entitlement key. A DIFFERENT alphabet from blockId on
	// purpose, and the gloss has to say so or an author reads the two failures
	// as the same rule: underscores are legal here, and there is no
	// last-character restriction, because a good id is private to one manifest
	// rather than a public URL segment. What it shares with blockId is that it
	// is permanent — a purchase and an entitlement are keyed by it.
	`^[a-z0-9][a-z0-9_-]*$`: {
		rule:    "must be lowercase letters, digits, underscores and hyphens only, starting with a letter or digit",
		example: "extra-credits",
	},
	// version
	`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`: {
		rule:    "must be a semantic version — three dot-separated numbers, with an optional -prerelease suffix",
		example: "1.0.0",
	},
	// tagline
	`\S`: {
		rule:    "must contain at least one non-whitespace character",
		example: "A tiny image tool",
	},
	// repository — the optional public source-repository link. The regex is a
	// coarse SHAPE check, not the whole server-side rule, so the gloss claims
	// only what the regex itself enforces.
	`^https://(github\.com|gitlab\.com|codeberg\.org)/[^/]+/[^/]+/?$`: {
		rule:    "must be a repository root URL on github.com, gitlab.com or codeberg.org — exactly https://<host>/<owner>/<repo>, with no deeper path",
		example: "https://github.com/civitai/civitai",
	},
	// minApiVersion
	`^\d+(\.\d+)*$`: {
		rule:    "must be dot-separated numbers only",
		example: "1.2",
	},
	// buildCommand. buildCoherence emits its own, fuller message for this one
	// as well; the gloss keeps the schema finding self-contained for anyone
	// reading a single line out of --json.
	`^(?:(?:npm|pnpm|yarn) run [a-zA-Z0-9:_-]+|(?:npx )?vite build)$`: {
		rule:    "must be one of the allowlisted build invocations — \"npm run <script>\", \"pnpm run <script>\", \"yarn run <script>\", \"vite build\" or \"npx vite build\"",
		example: "npm run build",
	},
	// assetBundleUrl
	`^https://`: {
		rule:    "must be an https:// URL",
		example: "https://example.com/bundle.zip",
	},
	// page.path
	`^/`: {
		rule:    "must start with a \"/\"",
		example: "/",
	},
}

// patternAdvice returns the trailing gloss for a pattern violation, or "" when
// the pattern has none. The empty return is the fail-soft path described above
// and is a supported state, not a bug.
func patternAdvice(k *kind.Pattern) string {
	r, ok := patternRules[k.Want]
	if !ok {
		return ""
	}
	return fmt.Sprintf(" — %s (example: %q)", r.rule, r.example)
}
