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
// a rule that changed. `TestPatternRulesCoverTheVendoredSchema` is the
// bidirectional ledger that keeps the two in step anyway — it fails when the
// schema grows a pattern with no gloss AND when a gloss names a pattern the
// schema no longer has.
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
