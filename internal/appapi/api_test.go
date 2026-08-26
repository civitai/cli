package appapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSubmitTimeoutIsLongAndScoped pins the F6 fix: the submit upload gets a
// substantially longer timeout than the fast, interactive calls, scoped to the
// submit client only. A short shared timeout previously produced false
// "context deadline exceeded" failures on submits that had already succeeded
// server-side, so the user's retry hit "you already have a pending submission".
func TestSubmitTimeoutIsLongAndScoped(t *testing.T) {
	if submitTimeout != 120*time.Second {
		t.Errorf("submitTimeout = %v, want 120s", submitTimeout)
	}
	if defaultTimeout != 30*time.Second {
		t.Errorf("defaultTimeout = %v, want 30s", defaultTimeout)
	}
	if submitTimeout <= defaultTimeout {
		t.Errorf("submit timeout (%v) must exceed the fast-call timeout (%v)", submitTimeout, defaultTimeout)
	}

	c := New("https://example.com", "tok", "")
	// The shared client stays on the short fast-call timeout...
	if c.HTTP.Timeout != defaultTimeout {
		t.Errorf("shared client timeout = %v, want %v (fast calls must not be lengthened)", c.HTTP.Timeout, defaultTimeout)
	}
	// ...while the submit client uses the long timeout.
	if got := c.submitClient().Timeout; got != submitTimeout {
		t.Errorf("submit client timeout = %v, want %v", got, submitTimeout)
	}
}

func TestSubmitVersionSendsBearerAndBase64(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		var body submitBody
		_ = json.Unmarshal(b, &body)
		gotBody = body.BundleBase64
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SubmitResult{
			PublishRequestID: "pr_1", Slug: "my-block", Version: "0.1.0", Status: "pending",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "/api/blocks/submit-version")
	res, err := c.SubmitVersion(context.Background(), []byte("ZIPDATA"), "my-block", "0.1.0", Provenance{})
	if err != nil {
		t.Fatalf("SubmitVersion: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	decoded, _ := base64.StdEncoding.DecodeString(gotBody)
	if string(decoded) != "ZIPDATA" {
		t.Errorf("decoded body = %q, want ZIPDATA", decoded)
	}
	if res.PublishRequestID != "pr_1" || res.Status != "pending" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestSubmitVersionSurfacesServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "bundle is missing required file: block.manifest.json"})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.SubmitVersion(context.Background(), []byte("x"), "my-block", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing required file") {
		t.Errorf("error %q should surface the server message", err)
	}
}

func TestWhoAmIRequiresToken(t *testing.T) {
	c := New("https://example.com", "", "")
	if _, err := c.WhoAmI(context.Background()); err == nil {
		t.Fatal("expected error with no token")
	}
}

func TestWhoAmIParsesIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "nope"})
			return
		}
		_ = json.NewEncoder(w).Encode(Identity{Username: "zach", ID: 42})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if id.Username != "zach" || id.ID != 42 {
		t.Errorf("identity = %+v", id)
	}
}

func TestWhoAmIParsesTokenScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A full-scope personal key: UserRead | AIServicesWrite | BuzzRead.
		_, _ = w.Write([]byte(`{"username":"zach","id":42,"tokenScope":98305}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if id.TokenScope == nil || *id.TokenScope != (ScopeUserRead|ScopeAIServicesWrite|ScopeBuzzRead) {
		t.Errorf("tokenScope = %v", id.TokenScope)
	}
	if !id.ScopeKnown() {
		t.Error("ScopeKnown should be true when tokenScope is present")
	}
	if !id.CanSpendBuzz() {
		t.Error("CanSpendBuzz should be true for a full-scope key")
	}
	if !id.CanReadBuzz() {
		t.Error("CanReadBuzz should be true for a full-scope key")
	}
}

// scopePtr is a test helper for building an Identity with a known scope mask.
func scopePtr(v int) *int { return &v }

func TestIdentityCapabilityBits(t *testing.T) {
	// An OAuth login token: UserRead | AppBlocksSubmit (bit 25) — no spend, no
	// balance-read.
	oauth := &Identity{TokenScope: scopePtr(ScopeUserRead | ScopeAppBlocksSubmit)}
	if oauth.CanSpendBuzz() {
		t.Error("OAuth token must not be able to spend Buzz")
	}
	if oauth.CanReadBuzz() {
		t.Error("OAuth token must not be able to read Buzz balance")
	}

	spendOnly := &Identity{TokenScope: scopePtr(ScopeAIServicesWrite)}
	if !spendOnly.CanSpendBuzz() || spendOnly.CanReadBuzz() {
		t.Errorf("AIServicesWrite-only: spend=%v read=%v", spendOnly.CanSpendBuzz(), spendOnly.CanReadBuzz())
	}

	readOnly := &Identity{TokenScope: scopePtr(ScopeBuzzRead)}
	if readOnly.CanSpendBuzz() || !readOnly.CanReadBuzz() {
		t.Errorf("BuzzRead-only: spend=%v read=%v", readOnly.CanSpendBuzz(), readOnly.CanReadBuzz())
	}
}

// TestIdentityScopeUnknown: an absent tokenScope must degrade to "unknown", not
// decode as "no capabilities".
func TestIdentityScopeUnknown(t *testing.T) {
	id := &Identity{Username: "zach", ID: 1} // no TokenScope
	if id.ScopeKnown() {
		t.Error("ScopeKnown should be false when tokenScope is absent")
	}
	if id.CanSpendBuzz() || id.CanReadBuzz() {
		t.Error("unknown scope must not report capabilities as true")
	}
	if id.DecodeScopes() != nil {
		t.Errorf("unknown scope should decode to nil, got %v", id.DecodeScopes())
	}
}

// TestCanSubmitAppsIsTriState pins the THREE states of the submit predicate at
// its source, so the rule lives in one place and both `whoami` surfaces read
// the same answer rather than re-deriving it.
//
// 🔴 THE nil CASES ARE THE REGRESSION. The predicate used to return a plain
// bool, so an OAuth credential with an ABSENT mask reported `false` — a false
// negative stated as fact, since for OAuth the mask bit IS the answer. Same for
// a credential with no `subject` at all: the CLI cannot tell whether the OAuth
// gate applies, so it cannot answer. Neither may ever render as "no".
func TestCanSubmitAppsIsTriState(t *testing.T) {
	oauthSubject := &Subject{Type: "oauth", ID: json.RawMessage(`"a"`)}
	keySubject := &Subject{Type: "apiKey", ID: json.RawMessage(`"k"`)}

	cases := []struct {
		name string
		id   *Identity
		want *bool // nil == unknowable
	}{
		{"oauth with the submit bit", &Identity{Subject: oauthSubject, TokenScope: scopePtr(ScopeUserRead | ScopeAppBlocksSubmit)}, boolPtr(true)},
		// Full scope EXCLUDES bit 25, so this is a real "no", not an unknown.
		{"oauth, full scope, no submit bit", &Identity{Subject: oauthSubject, TokenScope: scopePtr(ScopeFull)}, boolPtr(false)},
		{"oauth, mask absent", &Identity{Subject: oauthSubject}, nil},
		// A personal key is not scope-gated for submit, so the answer holds with
		// or without a mask — the pair below is what makes that visible.
		{"personal key, minimal mask", &Identity{Subject: keySubject, TokenScope: scopePtr(ScopeUserRead)}, boolPtr(true)},
		{"personal key, mask absent", &Identity{Subject: keySubject}, boolPtr(true)},
		{"no subject, mask present", &Identity{TokenScope: scopePtr(ScopeFull | ScopeAppBlocksSubmit)}, nil},
		{"no subject, mask absent", &Identity{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.id.CanSubmitApps()
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("CanSubmitApps = %v, want nil (unknowable) — a bool here is a claim the CLI cannot support", *got)
			case tc.want != nil && got == nil:
				t.Errorf("CanSubmitApps = nil, want %v — this state IS knowable and must not be withheld", *tc.want)
			case tc.want != nil && got != nil && *tc.want != *got:
				t.Errorf("CanSubmitApps = %v, want %v", *got, *tc.want)
			}
		})
	}
}

// TestDecodeScopesNilVsEmpty: `nil` and `[]` are DIFFERENT answers, because
// `whoami --json` emitted `"scopes": null` in two unrelated states — scope
// unreported, and a real credential with no bits set — which no consumer could
// tell apart. Go callers see len 0 either way; only the JSON encoding differs.
func TestDecodeScopesNilVsEmpty(t *testing.T) {
	unknown := (&Identity{}).DecodeScopes()
	if unknown != nil {
		t.Errorf("an ABSENT mask must decode to nil (JSON null), got %#v", unknown)
	}
	knownEmpty := (&Identity{TokenScope: scopePtr(0)}).DecodeScopes()
	if knownEmpty == nil {
		t.Error("a KNOWN mask of 0 must decode to a non-nil empty slice (JSON []), not nil — that is the whole distinction")
	}
	if len(knownEmpty) != 0 {
		t.Errorf("a zero mask decodes to no scope names, got %v", knownEmpty)
	}
}

// TestDecodeScopes exercises the bit-decode helper against representative masks.
func TestDecodeScopes(t *testing.T) {
	full := &Identity{TokenScope: scopePtr(ScopeFull)}
	names := full.DecodeScopes()
	// Full is bits 0..24 (25 bits) and EXCLUDES AppBlocksSubmit.
	if len(names) != 25 {
		t.Errorf("Full should decode to 25 scopes, got %d: %v", len(names), names)
	}
	if containsStr(names, "AppBlocksSubmit") {
		t.Errorf("Full must NOT include AppBlocksSubmit: %v", names)
	}
	for _, want := range []string{"UserRead", "AIServicesWrite", "BuzzRead", "VaultWrite"} {
		if !containsStr(names, want) {
			t.Errorf("Full should include %s: %v", want, names)
		}
	}

	fullSubmit := &Identity{TokenScope: scopePtr(ScopeFull | ScopeAppBlocksSubmit)}
	if !containsStr(fullSubmit.DecodeScopes(), "AppBlocksSubmit") {
		t.Errorf("Full|AppBlocksSubmit should include AppBlocksSubmit: %v", fullSubmit.DecodeScopes())
	}

	// A typical civitai-cli OAuth mask: UserRead + AppBlocksSubmit, no spend/read.
	oauth := &Identity{TokenScope: scopePtr(ScopeUserRead | ScopeAppBlocksSubmit)}
	got := oauth.DecodeScopes()
	if len(got) != 2 || got[0] != "UserRead" || got[1] != "AppBlocksSubmit" {
		t.Errorf("OAuth mask decode = %v, want [UserRead AppBlocksSubmit]", got)
	}
	if containsStr(got, "AIServicesWrite") || containsStr(got, "BuzzRead") {
		t.Errorf("OAuth mask must lack spend/read scopes: %v", got)
	}
}

// TestCredentialType maps subject.type to a human label.
func TestCredentialType(t *testing.T) {
	oauth := &Identity{Subject: &Subject{Type: "oauth", ID: json.RawMessage(`"1"`)}}
	if oauth.CredentialType() != "OAuth login" || !oauth.IsOAuth() {
		t.Errorf("oauth subject: type=%q isOAuth=%v", oauth.CredentialType(), oauth.IsOAuth())
	}
	key := &Identity{Subject: &Subject{Type: "apiKey", ID: json.RawMessage("96633526")}}
	if key.CredentialType() != "personal API key" || key.IsOAuth() {
		t.Errorf("apiKey subject: type=%q isOAuth=%v", key.CredentialType(), key.IsOAuth())
	}
	absent := &Identity{}
	if absent.CredentialType() != "unknown" || absent.IsOAuth() {
		t.Errorf("absent subject: type=%q isOAuth=%v", absent.CredentialType(), absent.IsOAuth())
	}
}

// liveMeBody is the EXACT GET /api/v1/me response that hard-failed `civitai
// whoami` in production: buzzLimit is now an array of window objects (was a bare
// number) and subject.id is a number (the struct typed it as a string). Both
// broke json.Unmarshal into the stale Identity struct. Kept verbatim as the
// regression fixture.
const liveMeBody = `{"id":8753561,"username":"zachlowdenzx","tier":"silver","status":"active","isMember":true,"subscriptions":["yellow"],"email":"zachlowden1@gmail.com","emailVerified":true,"tokenScope":33554431,"buzzLimit":[{"type":"sliding","limit":5000,"window":"day","unit":1}],"subject":{"type":"apiKey","id":96633526}}`

func meServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

// TestWhoAmIParsesLiveMeBody is the primary regression: the exact live /me body
// (array buzzLimit + numeric subject.id) must parse with NO error and surface
// the core identity — id 8753561, username zachlowdenzx, the apiKey subject.
func TestWhoAmIParsesLiveMeBody(t *testing.T) {
	srv := meServer(t, liveMeBody)
	defer srv.Close()

	id, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI on the live /me body must not error: %v", err)
	}
	if id.ID != 8753561 {
		t.Errorf("id = %d, want 8753561", id.ID)
	}
	if id.Username != "zachlowdenzx" {
		t.Errorf("username = %q, want zachlowdenzx", id.Username)
	}
	if id.Subject == nil || id.Subject.Type != "apiKey" {
		t.Fatalf("subject = %+v, want type apiKey", id.Subject)
	}
	if id.CredentialType() != "personal API key" || id.IsOAuth() {
		t.Errorf("credentialType = %q isOAuth=%v, want personal API key / false", id.CredentialType(), id.IsOAuth())
	}
	if id.TokenScope == nil || *id.TokenScope != 33554431 {
		t.Errorf("tokenScope = %v, want 33554431", id.TokenScope)
	}
	// Full-scope personal key can spend + read Buzz.
	if !id.CanSpendBuzz() || !id.CanReadBuzz() {
		t.Errorf("full-scope key should spend+read Buzz: spend=%v read=%v", id.CanSpendBuzz(), id.CanReadBuzz())
	}
	// buzzLimit is retained raw (the array), never decoded — but must not break parse.
	if len(id.BuzzLimit) == 0 || id.BuzzLimit[0] != '[' {
		t.Errorf("buzzLimit should be retained as the raw array, got %s", string(id.BuzzLimit))
	}
}

// TestWhoAmIParsesAccountProfile pins #377 option (b): the four modellable
// profile fields the live capture demonstrably carries must LAND, with their
// real values, off the very body that proves the server sends them.
//
// 🔴 THE FIXTURE IS THE REAL CAPTURE, AND THAT IS THE WHOLE METHOD. #377 was
// only findable because `liveMeBody` is a production response rather than a
// hand-written mirror of this struct — a mirror cannot, by construction, carry
// a field the struct is missing. Asserting against a body typed from the struct
// would re-create the blind spot that hid these four for months, so do not
// "simplify" this onto a local literal.
func TestWhoAmIParsesAccountProfile(t *testing.T) {
	srv := meServer(t, liveMeBody)
	defer srv.Close()

	id, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI on the live /me body must not error: %v", err)
	}
	if id.Tier == nil || *id.Tier != "silver" {
		t.Errorf("tier = %v, want silver", id.Tier)
	}
	if id.Status == nil || *id.Status != "active" {
		t.Errorf("status = %v, want active", id.Status)
	}
	if id.IsMember == nil || !*id.IsMember {
		t.Errorf("isMember = %v, want true", id.IsMember)
	}
	if len(id.Subscriptions) != 1 || id.Subscriptions[0] != "yellow" {
		t.Errorf("subscriptions = %v, want [yellow]", id.Subscriptions)
	}
}

// TestIdentityHasNoEmailField is the #377 privacy invariant at its SOURCE.
//
// 🔴 IT IS A STRUCTURAL GUARD ON `appapi.Identity`, NOT ON `whoami --json`, AND
// THE TWO ARE DIFFERENT CLAIMS. The payload guard in internal/cmd pins one
// command's output; this one pins that the bytes are dropped at the CLIENT
// boundary, so no FUTURE surface can publish a user's email address by adding a
// map entry. `liveMeBody` really carries `email`/`emailVerified` — the positive
// control that this asserts a live omission rather than an impossible one.
func TestIdentityHasNoEmailField(t *testing.T) {
	if !strings.Contains(liveMeBody, `"email"`) || !strings.Contains(liveMeBody, `"emailVerified"`) {
		t.Fatalf("positive control failed: the live capture no longer carries the PII this guard is about, " +
			"so its verdict would be vacuous — re-point it at a body that does")
	}
	// Round-trip the capture through the struct: whatever Identity models is
	// what a `--json` surface can reach, and nothing else survives.
	var id Identity
	if err := json.Unmarshal([]byte(liveMeBody), &id); err != nil {
		t.Fatalf("the live capture must parse: %v", err)
	}
	back, err := json.Marshal(&id)
	if err != nil {
		t.Fatalf("marshal Identity: %v", err)
	}
	for _, pii := range []string{"email", "emailVerified", "@"} {
		if strings.Contains(string(back), pii) {
			t.Errorf("appapi.Identity now retains PII (%q) from /api/v1/me — #377 rejected modelling "+
				"email/emailVerified precisely so no --json surface can publish them:\n%s", pii, back)
		}
	}
	// Positive control on the round-trip itself: a zero of everything would also
	// contain no PII. Something the struct DOES model must have survived.
	if !strings.Contains(string(back), `"tier":"silver"`) {
		t.Fatalf("the round-trip carried nothing, so the PII verdict above is vacuous:\n%s", back)
	}
}

// TestWhoAmIProfileDriftDegradesTheWholeProfile covers what an unexpected shape
// anywhere in the account profile does.
//
// 🔴 A DRIFT IN ANY ONE PROFILE FIELD BLANKS ALL FOUR, AND THAT IS THE DESIGN.
// The strict parse is all-or-nothing, so an unexpected shape anywhere in the
// profile drops WhoAmI into parseCoreIdentity — which deliberately parses none
// of them. The result degrades to "the CLI does not have it" (null) rather than
// to a fabricated value, and the core identity whoami prints still surfaces.
//
// This replaced an earlier design in which Subscriptions was a json.RawMessage
// specifically so a drift in IT could not blank its siblings. Two measurements
// killed that: the raw field published whatever the server put in it straight
// to `--json` stdout (see
// TestWhoAmIJSONNeverPublishesUnmodelledSubscriptionContent in internal/cmd),
// and it did not buy the resilience anyway — as the subtests below show, a
// drift in tier, status OR isMember blanks the whole profile regardless, so
// the exemption only ever covered a drift in subscriptions itself.
//
// The parameterisation is the point: ONE drifted field per case, each a
// different one, so the test cannot pass because some other field happened to
// be the one that failed.
func TestWhoAmIProfileDriftDegradesTheWholeProfile(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"subscriptions is an object array", `{"id":7,"username":"carol","tokenScope":1,"subject":{"type":"apiKey","id":42},` +
			`"tier":"gold","status":"active","isMember":true,"subscriptions":[{"tier":"yellow"}]}`},
		{"tier is a number", `{"id":7,"username":"carol","tokenScope":1,"subject":{"type":"apiKey","id":42},` +
			`"tier":3,"status":"active","isMember":true,"subscriptions":["yellow"]}`},
		{"status is an object", `{"id":7,"username":"carol","tokenScope":1,"subject":{"type":"apiKey","id":42},` +
			`"tier":"gold","status":{"code":2},"isMember":true,"subscriptions":["yellow"]}`},
		{"isMember is a string", `{"id":7,"username":"carol","tokenScope":1,"subject":{"type":"apiKey","id":42},` +
			`"tier":"gold","status":"active","isMember":"true","subscriptions":["yellow"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := meServer(t, tc.body)
			defer srv.Close()

			id, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
			if err != nil {
				t.Fatalf("a profile shape drift must not fail WhoAmI: %v", err)
			}
			// The core identity is what the fallback exists to preserve.
			if id.ID != 7 || id.Username != "carol" {
				t.Errorf("core identity lost: id=%d user=%q", id.ID, id.Username)
			}
			if id.TokenScope == nil || *id.TokenScope != 1 {
				t.Errorf("tokenScope should survive the fallback: %v", id.TokenScope)
			}
			// 🔴 AND THE WHOLE PROFILE MUST BE NIL, NOT PARTIALLY POPULATED.
			// This is what enforces parseCoreIdentity's "the profile fields are
			// NOT parsed here on purpose" comment: re-listing them there makes
			// this assertion fail. A comment is a claim; this is the check.
			if id.Tier != nil || id.Status != nil || id.IsMember != nil || id.Subscriptions != nil {
				t.Errorf("the fallback must blank the WHOLE profile, not populate part of it: "+
					"tier=%v status=%v isMember=%v subscriptions=%v",
					id.Tier, id.Status, id.IsMember, id.Subscriptions)
			}
		})
	}
}

// TestDescribeMeBodyNeverLeaksValues pins the error path that used to echo the
// entire /api/v1/me body — email and all — into an error `main` writes to
// stderr, where a shell redirect or a CI log keeps it.
//
// 🔴 EVERY VALUE IN THE FIXTURE IS A DISTINCT PII-SHAPED MARKER, so the guard
// cannot pass because the body happened to be boring. Types and key names are
// what diagnose this failure (a peripheral field that drifted its JSON type),
// so withholding values costs nothing diagnostically — and the assertions below
// check that the diagnosis really is still there, not just that the PII is gone.
func TestDescribeMeBodyNeverLeaksValues(t *testing.T) {
	const body = `{"id":8753561,"username":"zachlowdenzx","email":"leak@example.test",` +
		`"emailVerified":true,"tier":"MARKER-TIER","status":"MARKER-STATUS",` +
		`"subscriptions":["MARKER-SUB"],"stripeCustomerId":"cus_MARKERSTRIPE",` +
		`"buzzLimit":[{"limit":5000}],"subject":{"type":"apiKey","id":96633526}}`
	got := describeMeBody([]byte(body))

	for _, leak := range []string{
		"leak@example.test", "MARKER-TIER", "MARKER-STATUS", "MARKER-SUB",
		"cus_MARKERSTRIPE", "zachlowdenzx", "8753561", "96633526", "5000",
		"stripeCustomerId", // an unrecognised key is COUNTED, never named
	} {
		if strings.Contains(got, leak) {
			t.Errorf("describeMeBody leaked %q from the body: %s", leak, got)
		}
	}
	// Positive controls: it must still DIAGNOSE, or "leaked nothing" is just a
	// description of an empty string.
	// "+3 unrecognised" is email, emailVerified and stripeCustomerId — the count
	// is the assertion that unallowlisted keys are COUNTED rather than named,
	// and it covers the two PII keys specifically.
	for _, want := range []string{"tier: string", "subscriptions: array", "buzzLimit: array", "+3 unrecognised"} {
		if !strings.Contains(got, want) {
			t.Errorf("describeMeBody must still report shape — missing %q in: %s", want, got)
		}
	}

	// 🔴 THE "no recognised keys" BRANCH IS THE THIRD BODY-ECHO SITE IN THIS
	// FUNCTION, and it was uncovered until this case: a mutant replacing it with
	// `desc = string(raw)` survived the whole battery. It is not reachable from
	// WhoAmI today (every key parseCoreIdentity can choke on is allowlisted, so
	// `named` is never empty there) — which is exactly why only a direct call
	// can pin it, and why leaving it unpinned bets on that staying true.
	if unnamed := describeMeBody([]byte(`{"email":"leak@example.test","stripeId":"cus_MARKER"}`)); //
	strings.Contains(unnamed, "leak@example.test") || strings.Contains(unnamed, "cus_MARKER") {
		t.Errorf("the no-recognised-keys branch echoed the body: %s", unnamed)
	} else if !strings.Contains(unnamed, "no recognised keys") || !strings.Contains(unnamed, "+2 unrecognised") {
		t.Errorf("the no-recognised-keys branch must still report a count, got: %s", unnamed)
	}

	// A JSON null under an allowlisted key must report as "null", not fall
	// through to "number" — deleting that case is otherwise invisible, and the
	// body is reachable (a bad `username` type with `tier:null` gets here).
	if k := describeMeBody([]byte(`{"tier":null,"isMember":false,"id":7}`)); !strings.Contains(k, "tier: null") {
		t.Errorf("a JSON null must be reported as null, got: %s", k)
	} else if !strings.Contains(k, "isMember: bool") || !strings.Contains(k, "id: number") {
		t.Errorf("bool and number must not collapse into one kind, got: %s", k)
	}

	// The `unknown > 0` boundary: with exactly ONE unrecognised key the clause
	// must still fire. Every other fixture here carries 3, 2 or 0, so `> 0`
	// mutated to `> 1` executed nowhere and SURVIVED — a guard green by never
	// reaching its own boundary.
	if one := describeMeBody([]byte(`{"id":1,"somethingNew":{"a":1}}`)); !strings.Contains(one, "+1 unrecognised") {
		t.Errorf("a single unrecognised key must still be reported, got: %s", one)
	}
	if none := describeMeBody([]byte(`{"id":1,"tier":"x"}`)); strings.Contains(none, "unrecognised") {
		t.Errorf("zero unrecognised keys must not print the clause at all, got: %s", none)
	}

	// The key list is sorted, so the message is deterministic. Go randomises map
	// iteration, so dropping the sort makes this error message differ run to run
	// — which nothing else here would notice.
	for range 8 {
		if k := describeMeBody([]byte(`{"tier":"x","status":"y","id":1,"username":"u"}`)); //
		k != "keys id: number, status: string, tier: string, username: string" {
			t.Fatalf("describeMeBody must be deterministic, got: %s", k)
		}
	}
}

// TestDescribeMeBodyNonObjectBodyLeaksNothingEither covers describeMeBody's
// OTHER branch.
//
// 🔴 THE OBJECT TEST ABOVE CANNOT REACH THIS CODE, AND A MUTATION SWEEP PROVED
// IT. Replacing the non-object return with `return string(raw)` — an outright
// body echo — SURVIVED the whole battery, because every fixture in the sibling
// tests is a valid JSON object and so takes the map-parse path. The branch was
// green by never executing, which is the unreachable-guard shape, not coverage.
//
// A non-object /me body carrying PII is not hypothetical: a bare JSON array or
// string reaches here with both parses defeated, and nothing stops the server
// (or a proxy, or an error page rendered as JSON) putting an address in it.
func TestDescribeMeBodyNonObjectBodyLeaksNothingEither(t *testing.T) {
	for _, body := range []string{
		`["leak@example.test","cus_MARKER"]`,
		`"leak@example.test"`,
		// Truncated mid-object: not parseable as an object at all.
		`{"id":1,"email":"leak@example.test"`,
	} {
		got := describeMeBody([]byte(body))
		for _, leak := range []string{"leak@example.test", "cus_MARKER"} {
			if strings.Contains(got, leak) {
				t.Errorf("describeMeBody leaked %q from a non-object body %q: %s", leak, body, got)
			}
		}
		// Positive control: it must still say something USEFUL, or "leaked
		// nothing" is just a description of the empty string.
		if !strings.Contains(got, "not a JSON object") {
			t.Errorf("a non-object body should be reported as such, got %q for %q", got, body)
		}
	}
}

// TestUnparseableMeErrorAdvisesUpgradeNotLogin pins the ADVICE, not just the
// absence of PII.
//
// 🔴 IT EXISTS BECAUSE THIS EXACT LINE WAS SILENTLY LOST ONCE. The wording was
// edited, the edit was discarded by a `git checkout HEAD --` restore run while
// it was still uncommitted, and the full suite stayed green through the whole
// round — nothing asserted on the string, so nothing could notice. An error
// message with no test is not a contract, it is a comment that happens to
// compile.
//
// The substance: `civitai login` cannot help here. The token was ACCEPTED (the
// branch is only reachable on a 200) and the response's SHAPE is what defeated
// both parses, so a fresh credential fails identically. Advice a user can
// follow to no effect is worse than no advice — it costs them a login and
// leaves them where they started.
func TestUnparseableMeErrorAdvisesUpgradeNotLogin(t *testing.T) {
	srv := meServer(t, `{"id":1,"username":{"first":"ida"}}`)
	defer srv.Close()

	_, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
	if err == nil {
		t.Fatal("a body that defeats both parses must error")
	}
	msg := err.Error()
	if strings.Contains(msg, "civitai login") {
		t.Errorf("the advice must not send the user to re-authenticate — the token was accepted "+
			"and the BODY SHAPE is the failure, so a fresh token fails identically: %s", msg)
	}
	if !strings.Contains(msg, "civitai upgrade") {
		t.Errorf("the error should name the one action that can help (a newer CLI): %s", msg)
	}
	if !strings.Contains(msg, "github.com/civitai/cli/issues") {
		t.Errorf("the error should name where to report an unreadable shape: %s", msg)
	}
}

// TestSubscriptionsTagKeepsNilAndEmptyDistinct pins the struct tag against
// `omitempty`, which omits nil and empty alike and so erases the distinction
// the field's own doc comment promises. Lost in the same discarded restore as
// the error wording above, and equally unnoticed — the tag had no assertion.
func TestSubscriptionsTagKeepsNilAndEmptyDistinct(t *testing.T) {
	absent, err := json.Marshal(&Identity{Username: "z", ID: 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	empty, err := json.Marshal(&Identity{Username: "z", ID: 1, Subscriptions: []string{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(absent), `"subscriptions":null`) {
		t.Errorf("an absent subscriptions must marshal to null, got %s", absent)
	}
	if !strings.Contains(string(empty), `"subscriptions":[]`) {
		t.Errorf("a reported-but-empty subscriptions must marshal to [], got %s", empty)
	}
	if string(absent) == string(empty) {
		t.Errorf("nil and empty must not serialise identically — that is the whole point of the tag:\n%s", absent)
	}
}

// TestWhoAmIUnparseableBodyErrorCarriesNoPII is the same guarantee at the seam:
// describeMeBody being clean proves nothing if WhoAmI does not use it.
func TestWhoAmIUnparseableBodyErrorCarriesNoPII(t *testing.T) {
	// `username` as an object defeats BOTH the strict parse and the core
	// fallback, which is the only way to reach this branch.
	const body = `{"id":1,"username":{"first":"ida"},"email":"leak@example.test","emailVerified":true}`
	srv := meServer(t, body)
	defer srv.Close()

	_, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
	if err == nil {
		t.Fatal("a body that defeats both parses must error")
	}
	if strings.Contains(err.Error(), "leak@example.test") || strings.Contains(err.Error(), "emailVerified") {
		t.Errorf("the unparseable-body error leaked PII from the response: %v", err)
	}
	// Positive control: the error must still be the actionable one, or this
	// passes for a command that failed somewhere else entirely.
	if !strings.Contains(err.Error(), "unexpected /api/v1/me response") {
		t.Fatalf("wrong error reached the assertion, so the verdict is vacuous: %v", err)
	}
}

// TestIdentityProfileFieldsAreTriState pins that an ABSENT profile field stays
// absent rather than zero-filling. A plain `string`/`bool` would publish
// `"tier": ""` and `"isMember": false` as if the server had said so — the same
// false-negative-stated-as-fact that made CanSubmitApps a pointer.
func TestIdentityProfileFieldsAreTriState(t *testing.T) {
	srv := meServer(t, `{"id":9,"username":"hedda","tokenScope":1,"subject":{"type":"apiKey","id":1}}`)
	defer srv.Close()

	id, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if id.Tier != nil || id.Status != nil || id.IsMember != nil || id.Subscriptions != nil {
		t.Errorf("an omitted profile must stay nil, got tier=%v status=%v isMember=%v subscriptions=%v",
			id.Tier, id.Status, id.IsMember, id.Subscriptions)
	}
}

// TestWhoAmIResilientToPeripheralDrift covers future non-essential field drift:
// an unknown extra field, and a peripheral field (buzzLimit) with an unexpected
// type must all still yield the core identity rather than hard-failing.
func TestWhoAmIResilientToPeripheralDrift(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "unknown extra field",
			body: `{"id":7,"username":"carol","tokenScope":1,"subject":{"type":"apiKey","id":42},"somethingNew":{"a":1},"tier":"gold"}`,
		},
		{
			name: "buzzLimit unexpected type (object)",
			body: `{"id":7,"username":"carol","tokenScope":1,"buzzLimit":{"limit":10},"subject":{"type":"oauth","id":"abc"}}`,
		},
		{
			name: "buzzLimit unexpected type (string)",
			body: `{"id":7,"username":"carol","tokenScope":1,"buzzLimit":"unlimited","subject":{"type":"apiKey","id":42}}`,
		},
		{
			name: "subject.id string (legacy shape) still fine",
			body: `{"id":7,"username":"carol","tokenScope":1,"subject":{"type":"oauth","id":"legacy-oauth-sub"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := meServer(t, tc.body)
			defer srv.Close()

			id, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
			if err != nil {
				t.Fatalf("WhoAmI must not hard-fail on peripheral drift: %v", err)
			}
			if id.ID != 7 || id.Username != "carol" {
				t.Errorf("core identity lost: id=%d user=%q", id.ID, id.Username)
			}
			if id.Subject == nil || id.Subject.Type == "" {
				t.Errorf("subject.type should survive: %+v", id.Subject)
			}
			if id.TokenScope == nil || *id.TokenScope != 1 {
				t.Errorf("tokenScope should survive: %v", id.TokenScope)
			}
		})
	}
}

// TestWhoAmIRejectsNonObjectBody confirms a genuinely malformed body (not a JSON
// object) still surfaces the actionable "unexpected /api/v1/me response" error
// rather than silently succeeding.
func TestWhoAmIRejectsNonObjectBody(t *testing.T) {
	srv := meServer(t, `[1,2,3]`)
	defer srv.Close()

	_, err := New(srv.URL, "tok", "").WhoAmI(context.Background())
	if err == nil {
		t.Fatal("a non-object /me body should still error")
	}
	if !strings.Contains(err.Error(), "unexpected /api/v1/me response") {
		t.Errorf("error should be the actionable parse error, got: %v", err)
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestBuzzAccountTotal checks the Total helper.
func TestBuzzAccountTotal(t *testing.T) {
	a := &BuzzAccount{Blue: 5, Green: 7, Yellow: 4242}
	if a.Total() != 4254 {
		t.Errorf("Total = %d, want 4254", a.Total())
	}
}

func TestGetBuzzAccountParsesBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != BuzzAccountPath {
			t.Errorf("path = %q, want %q", r.URL.Path, BuzzAccountPath)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"result":{"data":{"json":{"blue":10,"green":20,"yellow":1234}}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	acct, err := c.GetBuzzAccount(context.Background())
	if err != nil {
		t.Fatalf("GetBuzzAccount: %v", err)
	}
	if acct.Yellow != 1234 || acct.Blue != 10 || acct.Green != 20 {
		t.Errorf("balance = %+v", acct)
	}
}

func TestGetBuzzAccount403ReturnsScopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{
				"message": "Your API key does not have the required scope for this action",
			}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.GetBuzzAccount(context.Background())
	if !errors.Is(err, ErrBuzzScope) {
		t.Errorf("expected ErrBuzzScope, got %v", err)
	}
}

func TestGetBuzzAccountRequiresToken(t *testing.T) {
	c := New("https://example.com", "", "")
	if _, err := c.GetBuzzAccount(context.Background()); err == nil {
		t.Fatal("expected error with no token")
	}
}
