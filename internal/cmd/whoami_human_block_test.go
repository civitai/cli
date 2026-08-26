package cmd

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// whoami_human_block_test.go pins the SHAPE of `civitai whoami`'s human
// rendering — both section headers, every row, the column they share, and the
// blank lines between them — for every credential state the command can be in.
//
// 🔴 A `strings.Contains` GUARD CANNOT SEE THE SHAPE. The section a row lands
// in, the order of the rows, whether a row printed at all, and above all the
// COLUMN the values line up in are exactly the properties a substring check is
// blind to: `Contains(out, "Submit Apps:              yes")` passes whether that
// row sits under `Credential:` or under `Capabilities:`, and whether the row
// above it was padded to a different width. The predecessor of this file was a
// want/notWant substring matrix; it is replaced rather than kept, because
// equality against the whole block subsumes every assertion it made.

// cantSpendGuidance is the block `whoami` appends when the credential cannot
// spend Buzz. It is part of the pinned rendering, not a separate concern: the
// #34 money-path dead end has to stay visible after the section split. This is
// the NON-OAuth ordering (personal key first), which is what a `CIVITAI_TOKEN`
// in the environment resolves to.
const cantSpendGuidance = "\n" +
	"⚠ This credential can't spend Buzz — `civitai generate` and money-path `dev:live`\n" +
	"generation both need the AI Services scope. To get it:\n" +
	"  civitai login --token <key>      # a FULL-SCOPE personal API key: create one at https://civitai.com/user/account\n" +
	"  civitai login --scopes generate  # or a browser login that opts into generation\n"

// scopeCaveat is the degraded-scope footnote. It sits INSIDE `Capabilities:`,
// BELOW the rows, because it explains why two capability rows are absent — it
// is a statement about the scope mask, not about the credential's identity, so
// it does not belong in the `Credential:` section.
const scopeCaveat = "  (token scope not reported by the server — Buzz capabilities unknown)\n"

// profileFields is the tail of a /api/v1/me body carrying the full account
// profile plus the two PII keys the CLI withholds. Bodies that end with it
// render IDENTICALLY to bodies without it — that is the property being pinned.
const profileFields = `"tier":"silver","status":"active","isMember":true,` +
	`"subscriptions":["yellow"],"email":"zach@example.test","emailVerified":true}`

// whoamiGoldenCase is one pinned state of the human surface.
type whoamiGoldenCase struct {
	name string
	body string
	args []string
	// oauth swaps the default CIVITAI_TOKEN setup for a stored OAuth config.
	//
	// 🔴 IT IS NOT COSMETIC: config.AuthKind() returns AuthKindToken for ANY
	// env-supplied token, so every case without this flag takes the `else`
	// half of the can't-spend guidance. The OAuth half was structurally
	// unreachable from this table — a render path no fixture executed.
	oauth bool
	// want is a format string; the single %s is the test server's URL.
	want string
}

// runWhoAmIGoldenCase drives one case and returns its two streams.
func runWhoAmIGoldenCase(t *testing.T, tc whoamiGoldenCase) (stdout, stderr, baseURL string) {
	t.Helper()
	srv := setupWhoAmI(t, tc.body)
	if tc.oauth {
		// setupWhoAmI already pointed XDG_CONFIG_HOME at a temp dir and set
		// CIVITAI_TOKEN; clear the token so the stored OAuth config is what
		// AuthKind() reads. CIVITAI_BASE_URL is untouched, so the server the
		// command talks to is still srv.
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("CIVITAI_TOKEN", "")
		writeOAuthConfig(t, dir)
	}
	out, errOut, err := run(t, append([]string{"whoami"}, tc.args...)...)
	if err != nil {
		t.Fatalf("whoami %v: %v", tc.args, err)
	}
	return out, errOut, srv.URL
}

// whoamiGoldenCases is every state the human surface is pinned in. The cases
// are numbered in their own comments; count them there rather than trusting a
// total written in prose — a sentence here said "six states" while the list
// held eight, which is what a count maintained in parallel with the thing it
// counts always does.
//
// 🔴 WHICH BODIES CARRY THE PROFILE, AND WHY — A GOLDEN ONLY CLOSES ADDITION ON
// THE REGIONS ITS FIXTURES RENDER. A profile row added to any render path ships
// to a real user, and a body WITHOUT profile fields cannot see it, because the
// `id.Tier != nil` guard never fires. So `profileFields` goes on enough bodies
// to cover every region. Which regions those are is not written down here on
// purpose: TestGoldenCarriesTheProfileOnEveryRenderPath DRIVES these cases and
// checks coverage against what they actually print, so the answer is measured
// rather than remembered.
//
// The history is the argument for that. Measured, one nil-guarded `Plan:`
// mutant per path: with the profile only in case 9, the Credential-section
// mutant was KILLED and the guidance, early-return and `--scopes` mutants all
// SURVIVED the whole `internal/cmd` package. Spreading the fixture killed those
// three — and a further round found ANOTHER path they had all missed, the OAuth
// half of the can't-spend guidance, which no case could reach at all because
// `config.AuthKind()` reads any env `CIVITAI_TOKEN` as a personal key. Twice in
// a row, a hand-written count of the render paths was wrong.
//
// 🔴 THIS REPLACED A BANNED-SUBSTRING LEDGER, AND THE TRADE IS DELIBERATE. That
// ledger caught a guidance-block row (it listed "silver") but lost to a
// paraphrase (`Member: yes` pays no banned word) and to a case change
// (`ACTIVE` evades a lowercase literal) — AGENTS item 28 records that shape
// losing three times. The golden loses to neither, and the fixture spread is
// what recovers the branch coverage the ledger had.
func whoamiGoldenCases() []whoamiGoldenCase {
	const (
		scopeUserRead        = 1 << 0
		scopeAIServicesWrite = 1 << 15
		scopeBuzzRead        = 1 << 16
	)
	return []whoamiGoldenCase{{
		// ---- 1: personal key + KNOWN mask ------------------------------------
		// The mask (33554431 = bits 0..24) is ScopeFull: reads and spends.
		name: "personal key, known mask",
		body: `{"id":8753561,"username":"zach","tokenScope":33554431,"subject":{"type":"apiKey","id":96633526}}`,
		want: "Logged in as zach (id 8753561) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     personal API key\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        yes\n" +
			"  Spend Buzz (AI Services): yes\n" +
			"  Submit Apps:              yes\n",
	}, {
		// ---- 2: personal key + ABSENT mask -----------------------------------
		// 🔴 `Submit Apps: yes` under an absent mask. A personal key is not
		// scope-gated for submit, so the answer is KNOWN even though the Buzz
		// rows are not — which is why the caveat names Buzz and not
		// "capabilities".
		name: "personal key, absent mask",
		body: `{"username":"zach","id":1,"subject":{"type":"apiKey","id":"k"}}`,
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     personal API key\n" +
			"\n" +
			"Capabilities:\n" +
			"  Submit Apps:              yes\n" +
			scopeCaveat,
	}, {
		// ---- 3: OAuth + KNOWN mask -------------------------------------------
		// UserRead only: no Buzz, and no AppBlocksSubmit bit — for an OAuth
		// credential that bit IS the submit answer, so this is a real `no`.
		name: "oauth, known mask",
		body: fmt.Sprintf(`{"username":"zach","id":1,"tokenScope":%d,"subject":{"type":"oauth","id":"a"}}`, scopeUserRead),
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     OAuth login\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        no\n" +
			"  Spend Buzz (AI Services): no\n" +
			"  Submit Apps:              no\n" +
			cantSpendGuidance,
	}, {
		// ---- 4: OAuth + ABSENT mask ------------------------------------------
		// 🔴 "unknown", never "no".
		// 🔴 THE PROFILE IS IN THIS BODY TO COVER THE EARLY-RETURN BRANCH.
		// See "which body carries the profile, and why" above the case list:
		// case 9's body never reaches the `!ScopeKnown()` return, so a profile
		// row added inside it renders for a real user and no golden sees it.
		name: "oauth, absent mask",
		body: `{"username":"zach","id":1,"subject":{"type":"oauth","id":"a"},` +
			profileFields,
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     OAuth login\n" +
			"\n" +
			"Capabilities:\n" +
			"  Submit Apps:              unknown\n" +
			scopeCaveat,
	}, {
		// ---- 5: no subject + ABSENT mask -------------------------------------
		// Both sections degrade at once, and the `Credential:` section still
		// prints — an "unknown" type is a rendered answer, not an omission.
		// 🔴 THE PROFILE IS IN THIS BODY FOR `Type: unknown`, a value no other
		// case produces. It was invisible while outputShape collapsed a padded
		// row's VALUE: `Type: unknown` and `Type: personal API key` shared a
		// shape, so this branch read as covered by case 9. Dropping that
		// collapse surfaced it immediately.
		name: "no subject, absent mask",
		body: `{"username":"zach","id":1,` + profileFields,
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     unknown\n" +
			"\n" +
			"Capabilities:\n" +
			"  Submit Apps:              unknown\n" +
			scopeCaveat,
	}, {
		// ---- 6: KNOWN mask == 0 ----------------------------------------------
		// The discriminator case: the mask WAS reported and had no bits, so the
		// Buzz rows are a measured `no` — no caveat, and the guidance fires.
		// 🔴 THE PROFILE IS IN THIS BODY TO COVER THE can't-spend GUIDANCE
		// BLOCK — the branch a profile row is most tempting to land in, since
		// it is the one that already talks about the account.
		name: "known mask is zero",
		body: `{"username":"zach","id":1,"tokenScope":0,"subject":{"type":"apiKey","id":"k"},` +
			profileFields,
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     personal API key\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        no\n" +
			"  Spend Buzz (AI Services): no\n" +
			"  Submit Apps:              yes\n" +
			cantSpendGuidance,
	}, {
		// ---- 7: --scopes, so the split is pinned with the scope list too -----
		// 🔴 THE PROFILE IS IN THIS BODY TO COVER THE `--scopes` BLOCK.
		name: "--scopes, known mask",
		body: fmt.Sprintf(`{"username":"zach","id":1,"tokenScope":%d,"subject":{"type":"apiKey","id":"k"},%s`,
			scopeUserRead|scopeAIServicesWrite|scopeBuzzRead, profileFields),
		args: []string{"--scopes"},
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     personal API key\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        yes\n" +
			"  Spend Buzz (AI Services): yes\n" +
			"  Submit Apps:              yes\n" +
			"\n" +
			"Scopes (3): UserRead, AIServicesWrite, BuzzRead\n",
	}, {
		// ---- 8: --scopes on a zero mask, plus the can't-spend guidance -------
		// 🔴 THE PROFILE IS IN THIS BODY FOR THE EMPTY-SCOPE-LIST BRANCH. It is
		// the same `Fprintf` as case 7's scope list (whoami.go:144) — the
		// `(none granted)` fork is an ASSIGNMENT feeding it, not a second print
		// — but it is a different rendered VALUE, and only this case produces
		// it. The previous ledger keyed on the substring `"Scopes ("`, which
		// matches both values, so this branch read as covered by case 7 and a
		// nil-guarded row printed between `Submit Apps:` and
		// `Scopes (0): (none granted)` survived the whole repo. That is why the
		// coverage unit is a rendered value, not a statement.
		name: "--scopes, zero mask",
		body: `{"username":"zach","id":1,"tokenScope":0,"subject":{"type":"apiKey","id":"k"},` +
			profileFields,
		args: []string{"--scopes"},
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     personal API key\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        no\n" +
			"  Spend Buzz (AI Services): no\n" +
			"  Submit Apps:              yes\n" +
			"\n" +
			"Scopes (0): (none granted)\n" +
			cantSpendGuidance,
	}, {
		// ---- 9: the account profile is present and RENDERS NOTHING -----------
		// 🔴 THIS CASE IS WHAT CLOSES ADDITION, AND EVERY OTHER CASE IS BLIND TO
		// IT. When this case was written it was the ONLY body carrying
		// `tier`/`status`/`isMember`/`subscriptions` — every other golden ran a
		// profile-less body, so a row rendered under `if id.Tier != nil` never
		// executed and the whole file stayed green while the human surface
		// turned into an account dump. (Cases 4-8 and 10 have since gained the
		// fixture too, one per render path; see whoamiGoldenCases.) Measured,
		// not reasoned: adding nil-guarded `Member:` and
		// `Account status:` rows to whoami.go left the WHOLE `internal/cmd`
		// package `ok`, and printed
		//     Type:                     personal API key
		//     Member:                   yes
		//     Account status:           ACTIVE
		// This body makes those rows render, so the golden below fails on them.
		//
		// It is a golden and not a banned-substring list on purpose. AGENTS item
		// 28 (claudedocs/decisions/28-no-claims-about-unobservable-spend.md)
		// records a phrase ledger losing THREE times — to a paraphrase paying no
		// banned word, and to a case-change — and names golden-output pinning as
		// the shape that closes ADDITION. `Member:` and `ACTIVE` are exactly
		// those two evasions, and this case kills both.
		//
		// The DECISION it pins: `whoami`'s human surface is a capability report,
		// not an account dump. #377 option (b) is scoped to `--json`.
		name: "full account profile present, human surface unchanged",
		body: `{"username":"zach","id":1,"tokenScope":33554431,"subject":{"type":"apiKey","id":"k"},` +
			profileFields,
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     personal API key\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        yes\n" +
			"  Spend Buzz (AI Services): yes\n" +
			"  Submit Apps:              yes\n",
	}, {
		// ---- 10: OAuth CONFIG + can't-spend ----------------------------------
		// 🔴 CASE 6 CANNOT REACH THIS BRANCH, AND NEITHER COULD ANY OTHER CASE.
		// The can't-spend guidance forks on config.AuthKind(), and every case
		// above runs under a CIVITAI_TOKEN — which AuthKind() classifies as a
		// personal key unconditionally. So the OAuth half of that fork was
		// structurally unreachable from this table: a nil-guarded profile row
		// added inside it SURVIVED the whole `internal/cmd` package while the
		// same mutant died on every other region.
		//
		// It is not an exotic state either — it is the headline case for the #34
		// money-path dead end this guidance exists to surface: a default
		// `civitai login` that cannot spend Buzz.
		name:  "oauth config, can't spend — the OAuth guidance fork",
		oauth: true,
		body: `{"username":"zach","id":1,"tokenScope":1,"subject":{"type":"oauth","id":"a"},` +
			profileFields,
		want: "Logged in as zach (id 1) at %s\n" +
			"\n" +
			"Credential:\n" +
			"  Type:                     OAuth login\n" +
			"\n" +
			"Capabilities:\n" +
			"  Read Buzz balance:        no\n" +
			"  Spend Buzz (AI Services): no\n" +
			"  Submit Apps:              no\n" +
			"\n" +
			"⚠ This credential can't spend Buzz — `civitai generate` and money-path `dev:live`\n" +
			"generation both need the AI Services scope. To get it:\n" +
			"  civitai login --scopes generate  # re-login, additively granting generation + Buzz spend\n" +
			"  civitai login --token <key>      # or a full-scope personal API key: https://civitai.com/user/account\n",
	}}
}

// TestWhoAmIHumanBlockIsPinnedWhole compares the ENTIRE stdout of the human
// surface against a literal, for every case in whoamiGoldenCases.
//
// 🔴 RED AT origin/main FOR EVERY CASE. Before this change the command printed
// ONE `Capabilities:` section whose first row was `Credential type:` — an
// identity attribute filed among three capability verdicts. These literals are
// what fails on pre-change code.
//
// 🔴 THE ROW ORDER IN THE DEGRADED CASES IS LOAD-BEARING. `Submit Apps` stays
// the LAST capability row and the caveat stays below it: the caveat is scoped
// to Buzz on purpose, and a caveat printed above a known `Submit Apps` answer
// would read as qualifying it (see TestWhoAmICaveatIsScopedToBuzz).
//
// 🔴 STDERR IS PART OF THE PIN, BECAUSE A GOLDEN ON STDOUT ALONE IS WALKABLE BY
// WRITING TO THE OTHER STREAM. Measured: a profile row sent to ErrOrStderr
// renders in the user's terminal exactly like a stdout row and left every case
// green. A successful `whoami` says nothing on stderr, so equality with "" is
// the whole contract.
func TestWhoAmIHumanBlockIsPinnedWhole(t *testing.T) {
	for _, tc := range whoamiGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, baseURL := runWhoAmIGoldenCase(t, tc)
			want := fmt.Sprintf(tc.want, baseURL)
			if stdout != want {
				t.Errorf("the rendered block changed.\n--- got ---\n%s\n--- want ---\n%s", stdout, want)
			}
			// See the 🔴 STDERR paragraph on this function's doc comment.
			if stderr != "" {
				t.Errorf("a successful whoami must print nothing to stderr, got:\n%s", stderr)
			}
		})
	}
}

// outputShape normalises one printed line by discarding the two things that
// vary with the HARNESS rather than with the code: the test server's URL and
// digit runs (the fixture's user id, and the scope count).
//
// 🔴 DO NOT NORMALISE ANYTHING ELSE. Every widening merges two lines into one
// shape, and a merge is exactly how the previous version of this guard went
// blind: it treated `Scopes (N): …` and `Scopes (0): (none granted)` as one
// region, and a nil-guarded row in the empty-list branch survived the whole
// repo. The tempting widenings are named so they can be refused by name:
//
//   - the SCOPE LIST ("it varies between fixtures") — collapsing it re-merges
//     the two `Scopes` renderings, which is the merge that blinded the previous
//     version. It does NOT resurrect the survivor today, and the reason is
//     worth knowing: case 8 now carries `profileFields`, so the GOLDEN catches
//     a row in that branch directly and the ledger is the second line of
//     defence, not the only one. Measured — the widening plus a planted row is
//     still red, at `TestWhoAmIHumanBlockIsPinnedWhole`. What the widening
//     costs is the ledger's ability to NOTICE if that fixture is ever removed,
//     which is how the hole opened the first time.
//   - a padded row's VALUE ("yes/no/unknown are one call site") — three sites,
//     not one: `yesNo`, `yesNoUnknown`, and `id.CredentialType()`. An earlier
//     version collapsed values, and that hid `Type: unknown` behind
//     `Type: personal API key` until the collapse was dropped.
//   - the GUIDANCE FORK wording — the OAuth and personal-key forks differ only
//     in their trailing comments, so converging that copy merges two branches
//     an earlier round paid to discover.
//
// When a legitimate new case produces a new shape, this test goes red. THE FIX
// IS A FIXTURE, NEVER A WIDER NORMALISATION: give that case `profileFields`.
//
// The rule is not merely measured, it is PINNED:
// TestOutputShapeDoesExactlyTheHarnessNormalisation asserts equality against an
// independent copy of it over every raw rendered line, so any widening at all —
// not just one that merges two of today's lines — fails.
func outputShape(line, baseURL string) string {
	s := strings.ReplaceAll(line, baseURL, "<url>")
	return digitRun.ReplaceAllString(s, "N")
}

var digitRun = regexp.MustCompile(`\d+`)

// harnessDigitRun is harnessNormalise's OWN copy of the digit rule.
//
// 🔴 IT DELIBERATELY DUPLICATES `digitRun`, AND MERGING THE TWO DEFEATS THE
// GUARD. An earlier version had harnessNormalise read `digitRun`, so widening
// that ONE shared line — e.g. to `\d+|yes|no|unknown` — changed both sides at
// once and the equality test passed while outputShape had gained a rule. The
// duplication is what makes the two definitions independent, which is the whole
// basis for comparing them.
var harnessDigitRun = regexp.MustCompile(`\d+`)

// harnessNormalise is the normalisation outputShape is ALLOWED to perform: the
// test server's URL and digit runs, and nothing else.
//
// 🔴 IT IS A FROZEN COPY, AND IT MUST NEVER GAIN A RULE. It exists so
// TestOutputShapeDoesExactlyTheHarnessNormalisation can compare outputShape
// against the intended behaviour rather than against itself. If you find
// yourself editing this to make a test pass, the test is telling you outputShape
// has widened — fix that instead.
func harnessNormalise(line, baseURL string) string {
	return harnessDigitRun.ReplaceAllString(strings.ReplaceAll(line, baseURL, "<url>"), "N")
}

// TestOutputShapeDoesExactlyTheHarnessNormalisation is the coverage, over EVERY
// RAW line the golden cases render.
//
// 🔴 IT ASSERTS EQUALITY, NOT INJECTIVITY, AND THE DIFFERENCE IS WHY THE
// PREVIOUS VERSION WAS BLIND. That version fed outputShape lines the harness had
// ALREADY normalised, then checked no two of them collapsed together. So any
// widening whose trigger referenced what the harness had erased — a digit run,
// or the base URL — never fired inside the test and passed silently. Measured:
// a `^Scopes \(\d+\): .*$` collapse, which is refused widening #1 in its most
// natural spelling, SURVIVED it. So did a whitespace-squashing collapse.
//
// Raw lines plus equality closes the whole class. outputShape must return
// exactly what an independent copy of the intended rule returns; any extra rule
// changes some line and fails here, whether or not it merges two of today's
// lines. Injectivity then follows rather than being separately asserted.
func TestOutputShapeDoesExactlyTheHarnessNormalisation(t *testing.T) {
	type sample struct{ line, baseURL string }
	var raw []sample
	seen := map[string]bool{}
	for _, tc := range whoamiGoldenCases() {
		stdout, _, baseURL := runWhoAmIGoldenCase(t, tc)
		for _, l := range strings.Split(stdout, "\n") {
			// 🔴 RAW, NOT NORMALISED — that is the fix. Dedupe on the normalised
			// form only to keep the sample small; what is COMPARED is `l`.
			if l == "" || seen[harnessNormalise(l, baseURL)] {
				continue
			}
			seen[harnessNormalise(l, baseURL)] = true
			raw = append(raw, sample{l, baseURL})
		}
	}
	// Positive control: a run producing nothing would satisfy the loop vacuously.
	if len(raw) < 10 {
		t.Fatalf("only %d distinct rendered lines across every golden case — the cases are not "+
			"exercising the command, so the verdict below would be vacuous", len(raw))
	}
	for _, s := range raw {
		if got, want := outputShape(s.line, s.baseURL), harnessNormalise(s.line, s.baseURL); got != want {
			t.Errorf("outputShape has gained a normalisation rule the harness does not have, so the "+
				"ledger can now merge two rendered lines and go blind.\n  line: %q\n  outputShape: %q"+
				"\n  allowed:     %q", s.line, got, want)
		}
	}
}

// TestOutputShapeDistinguishesEveryRenderedValue pins the collapses that would
// blind the ledger, as a direct property of outputShape.
//
// 🔴 THE PROPERTY IS PER-VALUE, NOT PER-STATEMENT, AND THE DIFFERENCE IS THE
// WHOLE POINT. An earlier version of this comment said each pair came from two
// DIFFERENT statements. Four of them do not: `Type:`, `Submit Apps:` and
// `Read Buzz balance:` each have exactly ONE `whoamiRow` call site, and
// `Scopes (%d)` exactly one `Fprintf` — the `(none granted)` fork is an
// assignment feeding it, not a second print. Those pairs are two VALUES from
// one statement.
//
// That wording was not merely imprecise, it was an invitation: a maintainer who
// believed it could reason "collapsing a padded row's value merges no two
// STATEMENTS, so it is safe" — which is refused widening #2 on outputShape,
// three lines above, and is exactly the collapse that hid `Type: unknown`.
//
// The property this enforces is the stronger one the ledger actually needs:
// every value that REACHES THE SCREEN keeps its own shape, so each one needs a
// profile-carrying fixture of its own. Pairs 2 and 3 are genuinely
// cross-statement (the two guidance forks); the rest are two values of one
// statement; pair 7 is cross-statement with a SHARED value word.
//
// 🔴 IT IS ALSO THE BACKSTOP FOR THE ONE CASE THE EQUALITY TEST CANNOT SEE:
// widening outputShape AND harnessNormalise together. Do not delete it on the
// grounds that the equality test covers everything — it does not cover that.
//
// The backstop is PARTIAL, and the bound is measured rather than assumed: it
// catches a both-widening that merges one of the pairs below (a padded-row value
// collapse and a trailing-`#` drop are each KILLED here and nowhere else), and
// it does NOT catch one that merges nothing listed — a both-sided whitespace
// squash survives every test in this file. That residual is latent rather than
// live: whitespace-squashing merges none of the lines whoami renders today, so
// it blinds nothing until some future line differs from another only by spacing.
// Widening a pair's coverage is the fix if that ever changes.
func TestOutputShapeDistinguishesEveryRenderedValue(t *testing.T) {
	const url = "http://127.0.0.1:1234"
	pairs := [][2]string{
		// Two VALUES of one `Fprintf` — the merge that blinded the previous
		// version. 🔴 NO LEADING NEWLINE: the ledger sees these AFTER
		// strings.Split, so a fixture written as the Fprintf's format string
		// ("\nScopes (%d): …") is not the line outputShape is ever handed. It
		// matters — with the "\n" present, a widening keyed on
		// strings.HasPrefix(s, "Scopes (") SURVIVED this test.
		{"Scopes (3): UserRead, AIServicesWrite, BuzzRead", "Scopes (0): (none granted)"},
		// Two STATEMENTS: the guidance forks, distinguished only by trailing comments.
		{"  civitai login --scopes generate  # re-login, additively granting generation + Buzz spend",
			"  civitai login --scopes generate  # or a browser login that opts into generation"},
		{"  civitai login --token <key>      # or a full-scope personal API key: https://civitai.com/user/account",
			"  civitai login --token <key>      # a FULL-SCOPE personal API key: create one at https://civitai.com/user/account"},
		// Two VALUES of one row, from three different value SOURCES:
		// id.CredentialType(), yesNo and yesNoUnknown respectively.
		{"  Type:                     personal API key", "  Type:                     OAuth login"},
		{"  Read Buzz balance:        yes", "  Read Buzz balance:        no"},
		{"  Submit Apps:              unknown", "  Submit Apps:              no"},
		// Cross-statement, and the same value word — a label-dropping collapse
		// merges these two even though nothing else does.
		{"  Type:                     unknown", "  Submit Apps:              unknown"},
	}
	for _, p := range pairs {
		if a, b := outputShape(p[0], url), outputShape(p[1], url); a == b {
			t.Errorf("outputShape merged two distinctly-rendered lines into one shape %q — the ledger "+
				"is now blind to an output row added alongside either of them.\n  a: %q\n  b: %q",
				a, p[0], p[1])
		}
	}
	// Positive control: the normalisations it IS supposed to do must still work,
	// or every pair above passes trivially on a function that changes nothing.
	if outputShape("Logged in as z (id 8753561) at "+url, url) != outputShape("Logged in as z (id 1) at "+url, url) {
		t.Error("outputShape must merge the harness-dependent id and base URL, or every legitimate " +
			"fixture difference reads as a new branch")
	}
}

// TestGoldenCarriesTheProfileOnEveryRenderPath is the LEDGER behind the fixture
// spread, and it exists because that spread is load-bearing prose.
//
// 🔴 THE FIXTURES ARE THE GUARD, AND NOTHING GUARDED THE FIXTURES. Deleting
// `profileFields` from a case — leaving every `want` untouched — SURVIVED the
// whole `internal/cmd` package: the goldens still matched, because a body
// without profile fields never fires the `id.Tier != nil` row a mutant would
// add. So a tidy-up of a fixture silently reopens the gap the spread closes.
//
// 🔴 THERE IS NO LIST OF RENDER PATHS HERE, AND THAT IS THE POINT — THREE
// CONSECUTIVE VERSIONS OF THIS GUARD GOT THE LIST WRONG. First it read case
// names out of this file's own source (matching the literal in its own
// assertion, matching indented comments, and coupled to gofmt's key alignment).
// Then it observed output through a hand-kept map of named regions — better,
// but the map was STILL a restatement of the code, and still incomplete: its
// `--scopes` marker was `"Scopes ("`, which matches both `Scopes (N): …` and
// `Scopes (0): (none granted)`, so the empty-list branch read as covered by a
// case that never rendered it. A nil-guarded row there survived the whole repo.
//
// Every one of those failures was the same mistake — a human enumerating the
// branches. So this enumerates nothing. It derives the branch set from what the
// cases actually PRINT: the union of line shapes produced by profile-carrying
// bodies must equal the union produced by ALL bodies. Any line only a
// profile-less case can produce is, by construction, a place an
// `if id.Tier != nil` row could hide — whether or not anyone thought of it.
//
// A new branch in whoami.go therefore needs no edit here at all; it needs a
// golden case, and if that case has no profile the test names the exact line.
func TestGoldenCarriesTheProfileOnEveryRenderPath(t *testing.T) {
	all, withProfile := map[string]string{}, map[string]bool{}

	for _, tc := range whoamiGoldenCases() {
		// 🔴 NOT SUBTESTS. A t.Fatalf inside one would skip its contribution to
		// the sets below, and the outer verdict would then blame a "stale
		// marker" for what is really a broken fixture — a wrong diagnosis
		// printed after the right one.
		stdout, _, baseURL := runWhoAmIGoldenCase(t, tc)
		// The proxy must match the guard a mutant would write. `id.Tier != nil`
		// needs `tier` to have PARSED, so a body is profile-carrying only when
		// it embeds the fixture whole — checking one key would count a body with
		// `"tier":null`, or a drifted body that degrades every field to nil.
		carries := strings.Contains(tc.body, profileFields)
		for _, line := range strings.Split(stdout, "\n") {
			if line == "" {
				continue
			}
			shape := outputShape(line, baseURL)
			if _, ok := all[shape]; !ok {
				all[shape] = tc.name
			}
			if carries {
				withProfile[shape] = true
			}
		}
	}

	// Positive control against a VACUOUS pass: a run that printed nothing, or
	// fixtures that all collapsed to one shape, satisfies the subset check
	// trivially. The floor is deliberately loose — a tight number would be a
	// hand-maintained count, which is the thing this design exists to avoid —
	// so read it as "the command produced output", NOT as a completeness check.
	// Completeness comes from the mutation sweep recorded on whoamiGoldenCases,
	// re-run when the code moves — not from this floor.
	if len(all) < 10 {
		t.Fatalf("only %d distinct output shapes across every golden case — the cases are not "+
			"exercising the command, so the coverage verdict below would be vacuous", len(all))
	}

	for _, shape := range slices.Sorted(maps.Keys(all)) {
		if !withProfile[shape] {
			// `all[shape]` is the FIRST case that rendered this line, not the
			// only one — several may. Any one of them gaining profileFields
			// fixes it, so naming one is enough to act on.
			t.Errorf("the line %q is rendered by NO golden case whose body carries the account "+
				"profile (first case that renders it: %q). An output row added at that statement "+
				"behind `if id.Tier != nil` would ship with every golden green.\n"+
				"THE FIX IS A FIXTURE, NOT A WIDER NORMALISATION: add profileFields to a case that "+
				"reaches it. See the 🔴 block on outputShape for why widening reopens a closed hole.",
				shape, all[shape])
		}
	}
}

// labelledRow matches one `  <Label>:<pad><value>` line of either section. It
// deliberately requires a capital first letter so it skips the caveat (`  (…`)
// and the guidance lines (`  civitai …`), neither of which is a padded row.
var labelledRow = regexp.MustCompile(`^ {2}([A-Z][^:]*:)( +)(\S.*)$`)

// TestWhoAmIValueColumnIsSharedAcrossSections asserts that the `Credential:`
// section's row and the `Capabilities:` section's rows put their VALUES in one
// column — the property the split put at risk, because `Type:` is 20 bytes
// shorter than the labels it used to share a block with and its padding had to
// be recomputed rather than copied.
//
// 🔴 RED AT origin/main: pre-change output has no `Credential:` section at all,
// so the cross-section comparison this makes cannot be satisfied there. It is
// therefore regression coverage for the split, not only an alignment invariant.
//
// It is also the alignment mutation detector: change `whoamiLabelColumn`, or
// hand-pad any single row, and this fails naming the offending row and column
// — a failure mode `strings.Contains` cannot produce.
func TestWhoAmIValueColumnIsSharedAcrossSections(t *testing.T) {
	setupWhoAmI(t, `{"username":"zach","id":1,"tokenScope":33554431,"subject":{"type":"apiKey","id":"k"}}`)
	stdout, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}

	type row struct {
		section string
		line    string
		col     int
	}
	var rows []row
	section := ""
	for _, line := range strings.Split(stdout, "\n") {
		switch strings.TrimSpace(line) {
		case "Credential:":
			section = "Credential"
			continue
		case "Capabilities:":
			section = "Capabilities"
			continue
		}
		m := labelledRow.FindStringSubmatch(line)
		if m == nil || section == "" {
			continue
		}
		rows = append(rows, row{section: section, line: line, col: len(m[1]) + len(m[2]) + 2})
	}

	// Positive control: the parse must have SEEN both sections, or an equal-
	// columns verdict over zero rows would be a vacuous pass.
	seen := map[string]int{}
	for _, r := range rows {
		seen[r.section]++
	}
	if seen["Credential"] != 1 {
		t.Fatalf("expected exactly 1 padded row under `Credential:`, parsed %d — the two-section split is not being rendered:\n%s",
			seen["Credential"], stdout)
	}
	if seen["Capabilities"] != 3 {
		t.Fatalf("expected 3 padded rows under `Capabilities:`, parsed %d:\n%s", seen["Capabilities"], stdout)
	}

	want := rows[0].col
	for _, r := range rows {
		if r.col != want {
			t.Errorf("value column drifted: %s row %q starts its value at column %d, but %q starts at %d — "+
				"both sections must share ONE column (whoamiLabelColumn)",
				r.section, r.line, r.col, rows[0].line, want)
		}
	}
	// And pin the column itself, so widening one label silently is also caught.
	if want != 28 {
		t.Errorf("the shared value column moved to %d (was 28); every row in both sections shifted:\n%s", want, stdout)
	}
}

// TestWhoAmICredentialTypeIsNotUnderCapabilities is the structural half of the
// split: the credential type must be reachable ONLY through the `Credential:`
// header. A `Contains` on either string cannot tell the two sections apart, so
// this asserts the RELATIONSHIP — which header the row falls under.
//
// 🔴 RED AT origin/main, where the row is literally the first line of the
// `Capabilities:` block.
func TestWhoAmICredentialTypeIsNotUnderCapabilities(t *testing.T) {
	setupWhoAmI(t, `{"username":"zach","id":1,"tokenScope":33554431,"subject":{"type":"apiKey","id":"k"}}`)
	stdout, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	cred := strings.Index(stdout, "\nCredential:\n")
	caps := strings.Index(stdout, "\nCapabilities:\n")
	if cred < 0 || caps < 0 {
		t.Fatalf("both section headers must be present (Credential:=%d Capabilities:=%d):\n%s", cred, caps, stdout)
	}
	if cred > caps {
		t.Errorf("`Credential:` must come first — it is the identity the capabilities are about:\n%s", stdout)
	}
	typeRow := strings.Index(stdout, "  Type:")
	if typeRow < 0 {
		t.Fatalf("no `Type:` row:\n%s", stdout)
	}
	if !(typeRow > cred && typeRow < caps) {
		t.Errorf("the `Type:` row must sit BETWEEN `Credential:` and `Capabilities:` — the credential type is an "+
			"identity attribute, not a capability verdict:\n%s", stdout)
	}
	// The old label must be gone: leaving it would keep the conflation
	// readable even with the header split.
	if strings.Contains(stdout, "Credential type:") {
		t.Errorf("the old `Credential type:` label survived the split:\n%s", stdout)
	}
}
