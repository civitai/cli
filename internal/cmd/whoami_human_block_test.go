package cmd

import (
	"fmt"
	"regexp"
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

// TestWhoAmIHumanBlockIsPinnedWhole compares the ENTIRE stdout of the human
// surface against a literal, for every state in the numbered list below. The
// cases are numbered in their own comments; count them there rather than
// trusting a total written here — this sentence said "six states" while the
// list held eight, which is what a count maintained in parallel with the thing
// it counts always does.
//
// 🔴 WHICH BODY CARRIES THE PROFILE, AND WHY — A GOLDEN ONLY CLOSES ADDITION ON
// THE BRANCHES ITS FIXTURES EXECUTE. `whoami` renders through FOUR paths, and a
// profile row added to any of them ships to a real user. Measured, one mutant
// per path, each a nil-guarded `Plan:` row: with the profile only in case 9,
// the Credential-section mutant was KILLED and the guidance-block, early-return
// and `--scopes` mutants all SURVIVED the whole `internal/cmd` package. A body
// without profile fields cannot see them, because the `id.Tier != nil` guard
// never fires. So `profileFields` is now on one body per path:
//
//	Credential section          -> case 9
//	`!ScopeKnown()` early return -> case 4 (oauth, absent mask)
//	can't-spend guidance         -> case 6 (known mask is zero)
//	`--scopes` block             -> case 7 (--scopes, known mask)
//
// The loop also pins STDERR empty, because a golden on stdout alone is walked
// by writing the row to the other stream — which reaches the terminal just the
// same. Adding a render path to whoami.go means adding it to this list.
//
// 🔴 THIS REPLACED A BANNED-SUBSTRING LEDGER, AND THE TRADE IS DELIBERATE. The
// ledger caught the guidance-block row (it listed "silver") but lost to a
// paraphrase (`Member: yes` pays no banned word) and to a case change
// (`ACTIVE` evades the lowercase literal) — AGENTS item 28 records that shape
// losing three times. The golden loses to neither, and the fixture spread above
// is what recovers the branch coverage the ledger had.
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
func TestWhoAmIHumanBlockIsPinnedWhole(t *testing.T) {
	const (
		scopeUserRead        = 1 << 0
		scopeAIServicesWrite = 1 << 15
		scopeBuzzRead        = 1 << 16
	)
	cases := []struct {
		name string
		body string
		args []string
		// want is a format string; the single %s is the test server's URL.
		want string
	}{{
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
		name: "no subject, absent mask",
		body: `{"username":"zach","id":1}`,
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
		name: "--scopes, zero mask",
		body: `{"username":"zach","id":1,"tokenScope":0,"subject":{"type":"apiKey","id":"k"}}`,
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
		// IT. Bodies 1–8 carry no `tier`/`status`/`isMember`/`subscriptions`, so
		// a row rendered under `if id.Tier != nil` never executes in any of them
		// — every golden here stays green while the human surface turns into an
		// account dump. Measured, not reasoned: adding nil-guarded `Member:` and
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
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupWhoAmI(t, tc.body)
			stdout, stderr, err := run(t, append([]string{"whoami"}, tc.args...)...)
			if err != nil {
				t.Fatalf("whoami %v: %v", tc.args, err)
			}
			want := fmt.Sprintf(tc.want, srv.URL)
			if stdout != want {
				t.Errorf("the rendered block changed.\n--- got ---\n%s\n--- want ---\n%s", stdout, want)
			}
			// 🔴 STDERR IS PART OF THE PIN, BECAUSE A GOLDEN ON STDOUT ALONE IS
			// WALKABLE BY WRITING TO THE OTHER STREAM. Measured: a profile row
			// sent to ErrOrStderr renders in the user's terminal exactly like a
			// stdout row and left every case below green. A successful `whoami`
			// says nothing on stderr, so equality with "" is the whole contract.
			if stderr != "" {
				t.Errorf("a successful whoami must print nothing to stderr, got:\n%s", stderr)
			}
		})
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
