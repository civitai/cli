package cmd

import (
	"encoding/json"
	"fmt"
	"io"
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
  "scopes": [
    "UserRead",
    "BuzzRead"
  ],
  "scopesKnown": true,
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
  "scopes": null,
  "scopesKnown": false,
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
  "scopes": [
    "AIServicesWrite",
    "AppBlocksSubmit"
  ],
  "scopesKnown": true,
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
  "scopes": null,
  "scopesKnown": false,
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
  "scopes": [],
  "scopesKnown": true,
  "username": "elias"
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
// The output is a hand-built projection of ten keys; the server demonstrably
// sends six more (`tier`, `status`, `isMember`, `subscriptions`, `email`,
// `emailVerified` — see the production capture in internal/appapi/api_test.go).
// Making it truly raw is NOT the fix: `email`/`emailVerified` are PII this
// command does not print.
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
