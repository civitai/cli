package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The `--json` contract. Pinned WHOLE, not by a handful of keys.
// ---------------------------------------------------------------------------

// TestWhoAmIJSONShapeIsPinnedWhole compares the ENTIRE marshalled payload
// against a literal, for every capability state the command can be in, so an
// ADDED key, a DROPPED key, a rename, or a changed null-vs-value decision is
// visible.
//
// 🔴 EVERY OTHER `whoami --json` TEST DECODES INTO A PARTIAL STRUCT, WHICH BY
// CONSTRUCTION CANNOT SEE EITHER FAILURE. `encoding/json` ignores unknown keys
// and zero-fills missing ones, so a partial-struct assertion agrees with a
// payload that has quietly grown or lost a sibling. Measured before this guard
// existed, against a full-package run with no `-run` filter: adding
// `"zzz_mutant": true` to the payload map SURVIVED, and deleting
// `"canSubmitApps"` SURVIVED. (The positive control — deleting `"canSpend"` —
// went red in TestWhoAmIJSONCredentialAndScopes, proving the harness could see
// a missing key at all, which is what makes the two survivors real findings
// rather than a broken runner.)
//
// 🔴 THE FIXTURES ARE A MATRIX, NOT ONE SAMPLE, AND THAT IS LOAD-BEARING.
// Three booleans over one payload cannot be pairwise distinct, so a single
// fixture leaves cross-assignment mutants alive (`"canSubmitApps":
// id.CanReadBuzz()` and friends). Across the cases below every pair disagrees
// somewhere: A has read=true/spend=false, C has read=false/spend=true, A has
// submit=true/spend=false, C has submit=true/read=false, and D has submit=null
// — which no bool-valued mutant can produce.
//
// 🔴 CASES F AND G EXIST BECAUSE A/B/C/D/E CANNOT SEE THE PROFILE FIELDS AT ALL.
// None of those five bodies carries `tier`/`status`/`isMember`/`subscriptions`,
// so all four pin as `null` in every one of them — a literal that a payload
// which never reads `id.Tier` and friends satisfies exactly as well. F and G
// carry them with values, and carry them PAIRWISE DISTINCT (silver/free,
// active/muted, true/false, ["yellow"]/[]) so a cross-assignment mutant
// (`"tier": id.Status`, `"isMember": id.Tier != nil`) disagrees in at least one
// case. F additionally carries `email`/`emailVerified` in the SERVER BODY and
// pins a payload WITHOUT them: that is the #377 privacy invariant, asserted on
// a body that really contains the PII rather than on one that never could.
func TestWhoAmIJSONShapeIsPinnedWhole(t *testing.T) {
	const (
		scopeUserRead        = 1 << 0
		scopeAIServicesWrite = 1 << 15
		scopeBuzzRead        = 1 << 16
		scopeAppBlocksSubmit = 1 << 25
	)
	cases := []struct {
		name string
		body string
		// want is a format string; %s is substituted with the test server's URL.
		want string
	}{{
		// ---- A: personal key + KNOWN mask ------------------------------------
		name: "personal key, known mask",
		body: fmt.Sprintf(`{"username":"alma","id":11,"tokenScope":%d,"subject":{"type":"apiKey","id":"k"}}`,
			scopeUserRead|scopeBuzzRead),
		want: `{
  "base_url": "%s",
  "canReadBalance": true,
  "canSpend": false,
  "canSubmitApps": true,
  "capabilities": {
    "can_read_buzz": true,
    "can_spend_buzz": false
  },
  "credentialType": "personal API key",
  "id": 11,
  "isMember": null,
  "scopes": [
    "UserRead",
    "BuzzRead"
  ],
  "scopesKnown": true,
  "status": null,
  "subscriptions": null,
  "tier": null,
  "username": "alma"
}
`,
	}, {
		// ---- B: personal key + ABSENT mask -----------------------------------
		// 🔴 canSubmitApps is TRUE here with scopesKnown FALSE. That pair is the
		// whole point of the tri-state: a personal key is not scope-gated for
		// submit, so the answer is KNOWN even though the mask is not.
		name: "personal key, absent mask",
		body: `{"username":"bertil","id":22,"subject":{"type":"apiKey","id":"k"}}`,
		want: `{
  "base_url": "%s",
  "canReadBalance": false,
  "canSpend": false,
  "canSubmitApps": true,
  "capabilities": {
    "can_read_buzz": false,
    "can_spend_buzz": false
  },
  "credentialType": "personal API key",
  "id": 22,
  "isMember": null,
  "scopes": null,
  "scopesKnown": false,
  "status": null,
  "subscriptions": null,
  "tier": null,
  "username": "bertil"
}
`,
	}, {
		// ---- C: OAuth + KNOWN mask -------------------------------------------
		name: "oauth, known mask",
		body: fmt.Sprintf(`{"username":"cesar","id":33,"tokenScope":%d,"subject":{"type":"oauth","id":"a"}}`,
			scopeAIServicesWrite|scopeAppBlocksSubmit),
		want: `{
  "base_url": "%s",
  "canReadBalance": false,
  "canSpend": true,
  "canSubmitApps": true,
  "capabilities": {
    "can_read_buzz": false,
    "can_spend_buzz": true
  },
  "credentialType": "OAuth login",
  "id": 33,
  "isMember": null,
  "scopes": [
    "AIServicesWrite",
    "AppBlocksSubmit"
  ],
  "scopesKnown": true,
  "status": null,
  "subscriptions": null,
  "tier": null,
  "username": "cesar"
}
`,
	}, {
		// ---- D: OAuth + ABSENT mask — THE REGRESSION -------------------------
		// 🔴 `"canSubmitApps": null`. Before this change the field was a plain
		// bool and this exact body emitted `false` — a false negative stated as
		// fact, since an OAuth credential's submit answer IS the mask bit and
		// the mask is absent. This literal is what fails on pre-change code.
		name: "oauth, absent mask",
		body: `{"username":"david","id":44,"subject":{"type":"oauth","id":"a"}}`,
		want: `{
  "base_url": "%s",
  "canReadBalance": false,
  "canSpend": false,
  "canSubmitApps": null,
  "capabilities": {
    "can_read_buzz": false,
    "can_spend_buzz": false
  },
  "credentialType": "OAuth login",
  "id": 44,
  "isMember": null,
  "scopes": null,
  "scopesKnown": false,
  "status": null,
  "subscriptions": null,
  "tier": null,
  "username": "david"
}
`,
	}, {
		// ---- E: no subject at all --------------------------------------------
		// The other unknowable state: we cannot tell whether the OAuth gate even
		// applies. Note scopesKnown is TRUE here while canSubmitApps is null, so
		// the two are demonstrably not the same discriminator.
		name: "no subject, known mask",
		body: `{"username":"elias","id":55,"tokenScope":0}`,
		want: `{
  "base_url": "%s",
  "canReadBalance": false,
  "canSpend": false,
  "canSubmitApps": null,
  "capabilities": {
    "can_read_buzz": false,
    "can_spend_buzz": false
  },
  "credentialType": "unknown",
  "id": 55,
  "isMember": null,
  "scopes": [],
  "scopesKnown": true,
  "status": null,
  "subscriptions": null,
  "tier": null,
  "username": "elias"
}
`,
	}, {
		// ---- F: the full account profile, PII INCLUDED IN THE BODY -----------
		// 🔴 THE SERVER BODY BELOW CARRIES `email` AND `emailVerified` AND THE
		// PINNED PAYLOAD DOES NOT. That is the #377 privacy invariant as a
		// whole-payload literal: modelling either field on appapi.Identity and
		// adding it to the map turns this literal red. A body without the PII
		// could not have seen that, which is why this one has it.
		name: "personal key, full profile (PII in the body)",
		body: fmt.Sprintf(`{"username":"frida","id":66,"tokenScope":%d,"subject":{"type":"apiKey","id":"k"},`+
			`"tier":"silver","status":"active","isMember":true,"subscriptions":["yellow"],`+
			`"email":"frida@example.test","emailVerified":true}`, scopeUserRead),
		want: `{
  "base_url": "%s",
  "canReadBalance": false,
  "canSpend": false,
  "canSubmitApps": true,
  "capabilities": {
    "can_read_buzz": false,
    "can_spend_buzz": false
  },
  "credentialType": "personal API key",
  "id": 66,
  "isMember": true,
  "scopes": [
    "UserRead"
  ],
  "scopesKnown": true,
  "status": "active",
  "subscriptions": [
    "yellow"
  ],
  "tier": "silver",
  "username": "frida"
}
`,
	}, {
		// ---- G: the profile's OTHER pole -------------------------------------
		// Every profile value disagrees with F's: free/silver, muted/active,
		// false/true, []/["yellow"]. A mutant that cross-wires two of the four,
		// or derives one from another's presence, agrees with F and dies here.
		// `subscriptions: []` also pins the empty-vs-null distinction the raw
		// passthrough preserves — the same nil-is-not-empty rule as `scopes`.
		name: "oauth, known mask, profile at the other pole",
		body: `{"username":"gustav","id":77,"tokenScope":0,"subject":{"type":"oauth","id":"a"},` +
			`"tier":"free","status":"muted","isMember":false,"subscriptions":[]}`,
		want: `{
  "base_url": "%s",
  "canReadBalance": false,
  "canSpend": false,
  "canSubmitApps": false,
  "capabilities": {
    "can_read_buzz": false,
    "can_spend_buzz": false
  },
  "credentialType": "OAuth login",
  "id": 77,
  "isMember": false,
  "scopes": [],
  "scopesKnown": true,
  "status": "muted",
  "subscriptions": [],
  "tier": "free",
  "username": "gustav"
}
`,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupWhoAmI(t, tc.body)
			stdout, _, err := run(t, "whoami", "--json")
			if err != nil {
				t.Fatalf("whoami --json: %v", err)
			}
			want := fmt.Sprintf(tc.want, srv.URL)
			if stdout != want {
				t.Errorf("--json payload changed.\n--- got ---\n%s\n--- want ---\n%s", stdout, want)
			}
		})
	}
}

// TestWhoAmIJSONIsTheWholeOfStdout: `civitai whoami --json | jq -e .` must
// always parse, and nothing human may contaminate it (internal/ui/CONVENTION.md
// — `--json` never passes through `ui`). An INVARIANT GUARD: it pins behaviour
// this change did not alter, so it is not regression coverage for anything here.
func TestWhoAmIJSONIsTheWholeOfStdout(t *testing.T) {
	setupWhoAmI(t, `{"username":"frida","id":66,"tokenScope":0,"subject":{"type":"oauth","id":"a"}}`)
	stdout, _, err := run(t, "whoami", "--json")
	if err != nil {
		t.Fatalf("whoami --json: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("stdout is not valid JSON (%v):\n%s", err, stdout)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("stdout carries more than the payload:\n%s", stdout)
	}
	// 🔴 THIS LIST IS THE NEGATIVE CONTROL, AND IT GOES VACUOUS ON A RENAME.
	// Every entry must be a marker the HUMAN surface really emits today —
	// a header that no longer exists is a string `--json` can never carry, so
	// the guard would pass while testing nothing. `whoami` now prints TWO
	// section headers, so BOTH are listed; `whoami_human_block_test.go` pins
	// the rendering they come from, which is what keeps them real.
	for _, glyph := range []string{"Logged in as", "Credential:", "Capabilities:", "\x1b["} {
		if strings.Contains(stdout, glyph) {
			t.Errorf("--json stdout carries the human marker %q:\n%s", glyph, stdout)
		}
	}
}

// ---------------------------------------------------------------------------
// The two surfaces must agree about what is KNOWABLE.
// ---------------------------------------------------------------------------

// The human-surface half of this pairing — the same states driven through the
// rendered block, because the #492 defect was precisely that the two surfaces
// DISAGREED (the old code returned early on `!ScopeKnown()` and printed no
// Submit Apps row at all, while `--json` published `canSubmitApps: true` for
// the same body) — lives in whoami_human_block_test.go. It moved there when it
// was upgraded from a want/notWant substring matrix to whole-block equality,
// which subsumes every assertion the matrix made and additionally sees the
// section a row lands in and the column its value starts at.

// TestWhoAmICaveatIsScopedToBuzz: the degraded-scope caveat used to say
// "capabilities unknown", full stop. With the Submit Apps row now printing
// under that same caveat, the old wording contradicted the line right above it.
func TestWhoAmICaveatIsScopedToBuzz(t *testing.T) {
	setupWhoAmI(t, `{"username":"bertil","id":22,"subject":{"type":"apiKey","id":"k"}}`)
	stdout, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if strings.Contains(stdout, "— capabilities unknown)") {
		t.Errorf("the caveat must not claim EVERY capability is unknown while Submit Apps prints a known answer:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Buzz capabilities unknown") {
		t.Errorf("the caveat must name the capabilities it actually covers:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// #377 — the help must not call a curated projection "raw".
// ---------------------------------------------------------------------------

// TestWhoAmIJSONHelpDoesNotClaimRaw pins BOTH help surfaces, separately.
//
// 🔴 TWO SURFACES, EACH GOES STALE ALONE — the `Example` block and the `--json`
// flag's usage string are different strings in different places, and #377 found
// the same false claim in both. Asserting once, on the rendered `--help`, would
// pass with one of them still wrong if the other happened to shadow it.
//
// The output is a hand-built projection of fourteen keys. #377 option (b)
// landed four of the six the server sent and the CLI dropped (`tier`, `status`,
// `isMember`, `subscriptions`); the remaining two are `email`/`emailVerified`,
// which stay dropped BECAUSE they are PII this command does not print. So
// "raw" is still false, and now false in a sharper way: the only gap left
// between raw and curated IS the PII, so a reader who believes the word expects
// exactly the two fields the CLI is deliberately withholding. The production
// capture proving the server sends them is in internal/appapi/api_test.go.
func TestWhoAmIJSONHelpDoesNotClaimRaw(t *testing.T) {
	cmd := newWhoAmICmd()
	flag := cmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatal("whoami has no --json flag")
	}
	surfaces := map[string]string{
		"Example block":     cmd.Example,
		"--json flag usage": flag.Usage,
	}
	for name, text := range surfaces {
		if strings.Contains(strings.ToLower(text), "raw") {
			t.Errorf("%s still calls the curated projection raw (#377): %q", name, text)
		}
		if !strings.Contains(text, "curated") {
			t.Errorf("%s should say what the output IS — a curated identity object: %q", name, text)
		}
	}
}

// TestWhoAmIScopesEmptyListIsNotBlank: a KNOWN mask with no bits set rendered
// as `Scopes (0): ` with an empty trailing list, which reads as truncated
// output rather than as a measurement.
func TestWhoAmIScopesEmptyListIsNotBlank(t *testing.T) {
	setupWhoAmI(t, `{"username":"frida","id":66,"tokenScope":0,"subject":{"type":"apiKey","id":"k"}}`)
	stdout, _, err := run(t, "whoami", "--scopes")
	if err != nil {
		t.Fatalf("whoami --scopes: %v", err)
	}
	if strings.Contains(stdout, "Scopes (0): \n") {
		t.Errorf("a zero mask must not render as a blank trailing list:\n%q", stdout)
	}
	if !strings.Contains(stdout, "Scopes (0): (none granted)") {
		t.Errorf("a zero mask should say so explicitly:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// #377 option (b) — the profile fields, and the PII that stays behind.
// ---------------------------------------------------------------------------

// meBodyWithPII is a /api/v1/me body shaped like the production capture in
// internal/appapi/api_test.go: it carries the four profile fields the CLI now
// publishes AND the two it deliberately withholds.
const meBodyWithPII = `{"username":"ida","id":88,"tokenScope":1,"subject":{"type":"apiKey","id":"k"},` +
	`"tier":"silver","status":"active","isMember":true,"subscriptions":["yellow"],` +
	`"email":"ida@example.test","emailVerified":true}`

// TestWhoAmINeverPrintsEmail is the #377 privacy invariant asserted where a user
// would actually see it: on BOTH of the command's own output surfaces.
//
// 🔴 THE STRUCTURAL GUARD IN internal/appapi CANNOT SEE THIS AND THIS CANNOT SEE
// THAT. TestIdentityHasNoEmailField pins that the bytes die at the client
// boundary; this pins that neither rendered surface prints them. They fail to
// different changes — a raw-body passthrough added in whoami.go would leave the
// struct guard green — so both exist.
//
// The positive control matters as much as the assertion: an email address is
// exactly the string a command that never had it also fails to print, so a
// verdict over a body without PII would be vacuous.
func TestWhoAmINeverPrintsEmail(t *testing.T) {
	if !strings.Contains(meBodyWithPII, "ida@example.test") || !strings.Contains(meBodyWithPII, "emailVerified") {
		t.Fatal("positive control failed: the fixture body no longer carries the PII this guard is about")
	}
	for _, surface := range [][]string{{"whoami"}, {"whoami", "--json"}} {
		t.Run(strings.Join(surface, " "), func(t *testing.T) {
			setupWhoAmI(t, meBodyWithPII)
			stdout, stderr, err := run(t, surface...)
			if err != nil {
				t.Fatalf("%v: %v", surface, err)
			}
			for _, pii := range []string{"ida@example.test", "email", "emailVerified"} {
				if strings.Contains(stdout, pii) || strings.Contains(stderr, pii) {
					t.Errorf("`civitai %s` leaked %q — #377 withholds email/emailVerified on purpose:\n%s%s",
						strings.Join(surface, " "), pii, stdout, stderr)
				}
			}
			// Positive control on the run itself: a command that errored early,
			// or printed nothing, would also print no PII.
			if !strings.Contains(stdout, "ida") {
				t.Fatalf("the command printed nothing about this identity, so the PII verdict is vacuous:\n%s", stdout)
			}
		})
	}
}

// TestWhoAmIProfileFieldsReachTheHumanSurfaceNever pins a deliberate NON-change.
//
// #377 option (b) is scoped to `--json`. The human output is byte-locked to
// README.md by whoami_readme_block_test.go and is a capability report, not an
// account dump — so the four profile fields must not appear there. Without this,
// "add them to the human rows too" reads as an obvious follow-up rather than as
// a decision someone already made.
func TestWhoAmIProfileFieldsReachTheHumanSurfaceNever(t *testing.T) {
	setupWhoAmI(t, meBodyWithPII)
	stdout, _, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	for _, v := range []string{"silver", "active", "yellow", "isMember", "Tier", "tier"} {
		if strings.Contains(stdout, v) {
			t.Errorf("the human surface is a capability report, not an account dump — it printed %q:\n%s", v, stdout)
		}
	}
	// Positive control: the capability rows the surface IS for still print.
	if !strings.Contains(stdout, "Submit Apps:") {
		t.Fatalf("the capability rows are missing, so the verdict above is vacuous:\n%s", stdout)
	}
}

// readmeWhoAmIJSONBlockRe captures the ```json fence under `##### whoami --json`
// — identified by a key unique to that payload on the whole page.
var readmeWhoAmIJSONBlockRe = regexp.MustCompile("(?s)```json\\n(\\{[^`]*?\"credentialType\"[^`]*?\\})\\n```")

// TestREADMEWhoAmIJSONBlockHasTheRealKeys closes the OTHER whoami seam.
//
// 🔴 whoami_readme_block_test.go BYTE-LOCKS THE HUMAN BLOCKS AND IS BLIND TO
// THIS ONE. Its regex anchors on `Logged in as`, which the `--json` example does
// not contain, so the documented payload could drift arbitrarily far from the
// real one with every existing guard green — and #377 option (b) is precisely a
// change that adds keys to one and not the other.
//
// It pins the KEY SET rather than the bytes, deliberately: the README block is
// hand-formatted (`"capabilities"` inline on one line) and json.Encoder is not,
// so byte-equality would force a reformat that helps no reader. The key set is
// what a script author reads it for, and it is what goes stale.
func TestREADMEWhoAmIJSONBlockHasTheRealKeys(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	blocks := readmeWhoAmIJSONBlockRe.FindAllStringSubmatch(string(readme), -1)
	// Positive control on the extractor: a verdict over zero blocks is the
	// reassuring zero this repo keeps hitting.
	if len(blocks) != 1 {
		t.Fatalf("expected exactly 1 `whoami --json` example block in README.md, extracted %d "+
			"(pattern: %s) — the extractor is reading the wrong text", len(blocks), readmeWhoAmIJSONBlockRe)
	}
	var documented map[string]any
	if err := json.Unmarshal([]byte(blocks[0][1]), &documented); err != nil {
		t.Fatalf("README.md's `whoami --json` example is not valid JSON (%v):\n%s", err, blocks[0][1])
	}

	setupWhoAmI(t, meBodyWithPII)
	stdout, _, err := run(t, "whoami", "--json")
	if err != nil {
		t.Fatalf("whoami --json: %v", err)
	}
	var actual map[string]any
	if err := json.Unmarshal([]byte(stdout), &actual); err != nil {
		t.Fatalf("payload is not valid JSON (%v):\n%s", err, stdout)
	}
	got, want := slices.Sorted(maps.Keys(actual)), slices.Sorted(maps.Keys(documented))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("README.md's `whoami --json` example no longer lists the keys the command emits.\n"+
			"--- the command emits ---\n%v\n--- README.md documents ---\n%v", got, want)
	}
}
