package cmd

import (
	"encoding/base64"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/ui"
)

// fixedNow is the clock every expiry assertion below is written against, so the
// rendered relative duration is deterministic rather than "whatever the test
// machine's clock said".
var fixedNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// TestDecodeDevTokenClaims: the decoder reads the three claims the CLI echoes,
// and reports ok=false (never panics, never errors) on anything malformed.
//
// The malformed rows are the fail-soft contract's foundation: every one of them
// is a real shape a token format change could produce, and each must degrade to
// "say nothing extra" rather than failing a mint that already succeeded.
func TestDecodeDevTokenClaims(t *testing.T) {
	budget := 250
	cases := []struct {
		name       string
		jwt        string
		wantOK     bool
		wantScopes []string
		wantBudget *int
		wantExp    int64
	}{
		{
			name:       "full payload",
			jwt:        makeJWTClaims(t, map[string]any{"scopes": []string{"ai:write:budgeted", "user:read:self"}, "buzzBudget": 250, "exp": 1790000000}),
			wantOK:     true,
			wantScopes: []string{"ai:write:budgeted", "user:read:self"},
			wantBudget: &budget,
			wantExp:    1790000000,
		},
		{
			// An absent buzzBudget must stay distinguishable from 0 — the pointer
			// is what lets grantedClaimsNotice say "not stated" instead of
			// inventing the server's default.
			name:       "no buzzBudget claim",
			jwt:        makeJWTClaims(t, map[string]any{"scopes": []string{"user:read:self"}}),
			wantOK:     true,
			wantScopes: []string{"user:read:self"},
			wantBudget: nil,
		},
		{"not a jwt", "garbage", false, nil, nil, 0},
		{"two segments only", "a.b", false, nil, nil, 0},
		{"bad base64 payload", "a.!!!.c", false, nil, nil, 0},
		{"payload is not json", "a." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".c", false, nil, nil, 0},
		{"empty string", "", false, nil, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeDevTokenClaims(tc.jwt)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if strings.Join(got.Scopes, ",") != strings.Join(tc.wantScopes, ",") {
				t.Errorf("scopes = %v, want %v", got.Scopes, tc.wantScopes)
			}
			switch {
			case tc.wantBudget == nil && got.BuzzBudget != nil:
				t.Errorf("buzzBudget = %d, want absent (nil)", *got.BuzzBudget)
			case tc.wantBudget != nil && got.BuzzBudget == nil:
				t.Errorf("buzzBudget absent, want %d", *tc.wantBudget)
			case tc.wantBudget != nil && *got.BuzzBudget != *tc.wantBudget:
				t.Errorf("buzzBudget = %d, want %d", *got.BuzzBudget, *tc.wantBudget)
			}
			if tc.wantExp != 0 && got.Exp != tc.wantExp {
				t.Errorf("exp = %d, want %d", got.Exp, tc.wantExp)
			}
		})
	}
}

// TestDecodeDevTokenClaimsStillBacksTokenCanSpend pins the SEAM: tokenCanSpend was
// reimplemented on top of decodeDevTokenClaims so there is one JWT decoder in the
// file. Its whole pre-existing contract must survive that, including every
// malformed input conservatively reading as "cannot spend".
func TestDecodeDevTokenClaimsStillBacksTokenCanSpend(t *testing.T) {
	cases := []struct {
		jwt  string
		want bool
	}{
		{makeJWTClaims(t, map[string]any{"scopes": []string{"ai:write:budgeted"}, "buzzBudget": 50}), true},
		{makeJWTClaims(t, map[string]any{"scopes": []string{"user:read:self"}, "buzzBudget": 50}), false},
		{makeJWTClaims(t, map[string]any{"buzzBudget": 50}), false},
		{"garbage", false},
	}
	for i, tc := range cases {
		if got := tokenCanSpend(tc.jwt); got != tc.want {
			t.Errorf("case %d: tokenCanSpend = %v, want %v", i, got, tc.want)
		}
	}
}

// TestGrantedClaimsNotice: the line names the scopes, the budget and the expiry —
// the three facts a developer previously had to hand-decode the JWT to learn.
func TestGrantedClaimsNotice(t *testing.T) {
	b50 := 50
	b0 := 0
	cases := []struct {
		name      string
		claims    devTokenClaims
		ok        bool
		wantHas   []string
		wantNotHa []string
	}{
		{
			name:   "scopes + budget + expiry",
			claims: devTokenClaims{Scopes: []string{"ai:write:budgeted", "user:read:self"}, BuzzBudget: &b50, Exp: fixedNow.Add(4 * time.Hour).Unix()},
			ok:     true,
			wantHas: []string{
				"scopes ai:write:budgeted, user:read:self",
				"buzz budget 50/generation",
				"2026-09-27T16:00:00Z",
				"in 4h",
			},
		},
		{
			// 🔴 The absent-budget case must NOT print the server's default (50):
			// stating a number the token does not carry is the exact
			// misinformation this line exists to remove.
			name:      "budget not stated",
			claims:    devTokenClaims{Scopes: []string{"user:read:self"}},
			ok:        true,
			wantHas:   []string{"buzz budget not stated in the token"},
			wantNotHa: []string{"50/generation", strconv.Itoa(appapi.DevBuzzBudgetDefault) + "/generation"},
		},
		{
			// An explicit 0 is a real grant and must read as one, not as "absent".
			name:      "budget explicitly zero",
			claims:    devTokenClaims{Scopes: []string{"user:read:self"}, BuzzBudget: &b0},
			ok:        true,
			wantHas:   []string{"buzz budget 0/generation"},
			wantNotHa: []string{"not stated"},
		},
		{
			name:    "already expired",
			claims:  devTokenClaims{Scopes: []string{"user:read:self"}, BuzzBudget: &b50, Exp: fixedNow.Add(-time.Hour).Unix()},
			ok:      true,
			wantHas: []string{"EXPIRED", "2026-09-27T11:00:00Z"},
		},
		{
			name:    "no scopes claim",
			claims:  devTokenClaims{BuzzBudget: &b50},
			ok:      true,
			wantHas: []string{"scopes: none stated"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := grantedClaimsNotice(tc.claims, tc.ok, fixedNow)
			if got == "" {
				t.Fatal("notice is empty")
			}
			if !strings.HasPrefix(got, "Granted: ") {
				t.Errorf("notice must lead with Granted:, got %q", got)
			}
			for _, w := range tc.wantHas {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q; got %q", w, got)
				}
			}
			for _, w := range tc.wantNotHa {
				if strings.Contains(got, w) {
					t.Errorf("must NOT contain %q; got %q", w, got)
				}
			}
		})
	}
}

// TestGrantedClaimsNoticeSilentOnUndecodable is the FAIL-SOFT contract at the
// pure-function level: an undecodable payload yields NO line at all rather than a
// line full of zero values. A wrong budget is worse than no budget.
func TestGrantedClaimsNoticeSilentOnUndecodable(t *testing.T) {
	if got := grantedClaimsNotice(devTokenClaims{}, false, fixedNow); got != "" {
		t.Errorf("an undecodable token must print nothing, got %q", got)
	}
}

// TestGrantedClaimsNoticeSanitizesScopes: the granted set derives from a request
// the LOCAL manifest can influence, so author-supplied text reaches this line.
// A crafted scope must not be able to drive the developer's terminal.
func TestGrantedClaimsNoticeSanitizesScopes(t *testing.T) {
	got := grantedClaimsNotice(devTokenClaims{Scopes: []string{"ai:write:budgeted\x1b[31mRED\x07"}}, true, fixedNow)
	for _, banned := range []string{"\x1b", "\x07"} {
		if strings.Contains(got, banned) {
			t.Errorf("control sequence %q survived into the display line: %q", banned, got)
		}
	}
	if !strings.Contains(got, "ai:write:budgeted") {
		t.Errorf("sanitizing must not eat the printable scope text: %q", got)
	}
}

// budgetFlagRE pulls the `--budget N` out of a rendered notice so the test can
// check the NUMBER rather than the sentence around it.
var budgetFlagRE = regexp.MustCompile(`--budget (\d+)`)

// TestBudgetShortfallNoticeRemedyIsAcceptable is the STRUCTURAL guard on this
// file's own house rule — "every command this block prints must leave the state it
// complains about".
//
// 🔴 It does not check the wording. It extracts the `--budget N` the notice tells
// the developer to run and feeds N back through validateDevTokenBudget, the very
// function the CLI would reject it with. The scaffold declares 300 against a cap
// of 250, so echoing the declared figure verbatim would print a command that
// exits 2 as a usage error — a guard on the sentence could not see that, and the
// obvious implementation gets it wrong.
func TestBudgetShortfallNoticeRemedyIsAcceptable(t *testing.T) {
	cases := []struct {
		name          string
		declared      int
		granted       int
		wantAsk       int
		wantCapNote   bool
		wantNoCapNote bool
	}{
		{name: "the scaffold's own 300 against the default 50", declared: 300, granted: 50, wantAsk: appapi.DevBuzzBudgetCap, wantCapNote: true},
		{name: "declared exactly at the cap", declared: 250, granted: 50, wantAsk: 250, wantNoCapNote: true},
		{name: "declared under the cap", declared: 120, granted: 50, wantAsk: 120, wantNoCapNote: true},
		{name: "declared one over the cap", declared: 251, granted: 50, wantAsk: appapi.DevBuzzBudgetCap, wantCapNote: true},
		{name: "far over the cap", declared: 100000, granted: 1, wantAsk: appapi.DevBuzzBudgetCap, wantCapNote: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := budgetShortfallNotice(ui.For(io.Discard), "my-block", tc.declared, tc.granted)

			m := budgetFlagRE.FindStringSubmatch(got)
			if m == nil {
				t.Fatalf("notice names no --budget remedy at all; got:\n%s", got)
			}
			ask, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("--budget %q is not an integer", m[1])
			}
			if ask != tc.wantAsk {
				t.Errorf("remedy is --budget %d, want %d", ask, tc.wantAsk)
			}
			// THE LOAD-BEARING ASSERTION: the command it printed must actually run.
			if err := validateDevTokenBudget(ask); err != nil {
				t.Errorf("the notice printed `--budget %d`, which the CLI itself REFUSES: %v\n"+
					"Every command this block prints must leave the state it complains about.", ask, err)
			}
			// It must state both real figures, or the developer cannot see the gap.
			for _, w := range []string{strconv.Itoa(tc.declared), strconv.Itoa(tc.granted), "page.buzzBudgetPerGen"} {
				if !strings.Contains(got, w) {
					t.Errorf("notice omits %q; got:\n%s", w, got)
				}
			}
			capNote := strings.Contains(got, "above the route's cap")
			if tc.wantCapNote && !capNote {
				t.Errorf("a declared figure over the cap must say so; got:\n%s", got)
			}
			if tc.wantNoCapNote && capNote {
				t.Errorf("a reachable declared figure must NOT claim it is over the cap; got:\n%s", got)
			}
		})
	}
}
