package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// The three bits `--scopes generate` adds, spelled out independently of the
// production constants so a typo in appblocks.go's scope table cannot make this
// file agree with itself. These are the values in civitai/civitai's
// token-scope.constants.ts.
const (
	litAIServicesRead  = 1 << 14 // 16384
	litAIServicesWrite = 1 << 15 // 32768
	litBuzzRead        = 1 << 16 // 65536
)

// litDefaultScope / litGenerateScope are the two literal masks this PR is about.
// 100777985 is what the civitai-cli OauthClient's allowedScopes must be widened
// to (civitai/civitai#3681) before `--scopes generate` can succeed in prod.
const (
	litDefaultScope  = "100663297"
	litGenerateScope = "100777985"
)

// TestDeviceScopeStringMatchesBits pins the string constant against the bit
// composition so the two encodings of the SAME fact cannot drift.
func TestDeviceScopeStringMatchesBits(t *testing.T) {
	got, err := strconv.Atoi(DeviceScope)
	if err != nil {
		t.Fatalf("DeviceScope %q is not a base-10 bitmask: %v", DeviceScope, err)
	}
	if got != deviceScopeBase {
		t.Errorf("DeviceScope=%s but deviceScopeBase=%d — the string and the bits disagree", DeviceScope, deviceScopeBase)
	}
	if DeviceScope != litDefaultScope {
		t.Errorf("DeviceScope = %q, want the literal default %q", DeviceScope, litDefaultScope)
	}
}

// TestResolveDeviceScopeDefault: no --scopes → the login request is bit-for-bit
// what it has always been. This is the load-bearing half of the feature: a plain
// `civitai login` must NOT silently gain Buzz-spend authority.
func TestResolveDeviceScopeDefault(t *testing.T) {
	for _, sets := range [][]string{nil, {}, {""}, {"  "}} {
		got, err := ResolveDeviceScope(sets)
		if err != nil {
			t.Fatalf("ResolveDeviceScope(%#v): %v", sets, err)
		}
		if got != litDefaultScope {
			t.Errorf("ResolveDeviceScope(%#v) = %q, want %q", sets, got, litDefaultScope)
		}
		n, _ := strconv.Atoi(got)
		if n&litAIServicesWrite != 0 {
			t.Errorf("ResolveDeviceScope(%#v) = %d carries AIServicesWrite (%d) — the DEFAULT login must not request Buzz-spend",
				sets, n, litAIServicesWrite)
		}
	}
}

// TestResolveDeviceScopeGenerate pins the literal widened mask.
func TestResolveDeviceScopeGenerate(t *testing.T) {
	got, err := ResolveDeviceScope([]string{ScopeSetGenerate})
	if err != nil {
		t.Fatalf("ResolveDeviceScope(generate): %v", err)
	}
	if got != litGenerateScope {
		t.Errorf("ResolveDeviceScope(generate) = %q, want %q", got, litGenerateScope)
	}
}

// TestGenerateScopeIsSupersetOfDefault pins the ADDITIVE property, which is the
// reason the flag ORs instead of replacing: a login yields ONE credential, so a
// user opting into generation must not lose app-submit / dev-tunnel ability.
func TestGenerateScopeIsSupersetOfDefault(t *testing.T) {
	base, err := ResolveDeviceScope(nil)
	if err != nil {
		t.Fatalf("default: %v", err)
	}
	gen, err := ResolveDeviceScope([]string{ScopeSetGenerate})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, _ := strconv.Atoi(base)
	g, _ := strconv.Atoi(gen)

	if g&b != b {
		t.Errorf("generate mask %d is NOT a superset of the default %d (missing bits %d)", g, b, b&^g)
	}
	if g == b {
		t.Fatalf("generate mask %d == default %d — --scopes generate would be a no-op", g, b)
	}
	// And the added bits are EXACTLY the three the server widening grants.
	wantAdded := litAIServicesRead | litAIServicesWrite | litBuzzRead
	if added := g &^ b; added != wantAdded {
		t.Errorf("generate adds %d, want exactly %d (AIServicesRead|AIServicesWrite|BuzzRead)", added, wantAdded)
	}
}

// TestResolveDeviceScopeUnknownSet: a typo must be a hard error naming the valid
// sets — never a silent fallback that logs the user in with fewer scopes than
// they asked for.
func TestResolveDeviceScopeUnknownSet(t *testing.T) {
	_, err := ResolveDeviceScope([]string{"generte"})
	if err == nil {
		t.Fatal("ResolveDeviceScope(generte) returned no error — a typo must not silently degrade to the default")
	}
	msg := err.Error()
	if !strings.Contains(msg, "generte") {
		t.Errorf("error should echo the bad name; got %q", msg)
	}
	for _, name := range DeviceScopeSetNames() {
		if !strings.Contains(msg, name) {
			t.Errorf("error should list the valid set %q; got %q", name, msg)
		}
	}
}

// TestResolveDeviceScopeUnknownAmongValid: one bad name poisons the whole
// request even when a valid one precedes it (fail closed, not partially).
func TestResolveDeviceScopeUnknownAmongValid(t *testing.T) {
	if _, err := ResolveDeviceScope([]string{ScopeSetGenerate, "nope"}); err == nil {
		t.Fatal("a valid set followed by an unknown one must still error")
	}
}

// TestResolveDeviceScopeNormalizes: case + surrounding whitespace are tolerated
// so `--scopes Generate` and `--scopes " generate"` behave.
func TestResolveDeviceScopeNormalizes(t *testing.T) {
	for _, in := range []string{"generate", "Generate", "GENERATE", " generate "} {
		got, err := ResolveDeviceScope([]string{in})
		if err != nil {
			t.Fatalf("ResolveDeviceScope(%q): %v", in, err)
		}
		if got != litGenerateScope {
			t.Errorf("ResolveDeviceScope(%q) = %q, want %q", in, got, litGenerateScope)
		}
	}
}

// TestResolveDeviceScopeRepeatIsIdempotent: OR-ing the same set twice is the
// same mask (a repeated --scopes must not corrupt the value).
func TestResolveDeviceScopeRepeatIsIdempotent(t *testing.T) {
	got, err := ResolveDeviceScope([]string{ScopeSetGenerate, ScopeSetGenerate})
	if err != nil {
		t.Fatalf("ResolveDeviceScope: %v", err)
	}
	if got != litGenerateScope {
		t.Errorf("repeated generate = %q, want %q", got, litGenerateScope)
	}
}

// TestDeviceScopeSetSummaryNamesTheConsequence: the summary is what `login
// --help` and the login output print. It must name the CONSEQUENCE (spend), not
// just bit names — that is the whole point of surfacing it at the point of use.
func TestDeviceScopeSetSummaryNamesTheConsequence(t *testing.T) {
	summary, ok := DeviceScopeSetSummary(ScopeSetGenerate)
	if !ok {
		t.Fatal("generate set has no summary")
	}
	if !strings.Contains(strings.ToUpper(summary), "SPEND") {
		t.Errorf("generate summary must say it grants Buzz SPEND authority; got %q", summary)
	}
	if _, ok := DeviceScopeSetSummary("nope"); ok {
		t.Error("DeviceScopeSetSummary reported an unknown set as known")
	}
}

// startDeviceServer stands up a device-init server that records the outgoing
// form and answers with `body` at `status`.
func startDeviceServer(t *testing.T, gotForm *url.Values, status int, body any) *httptest.Server {
	t.Helper()
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			writeDiscovery(w, base)
			return
		}
		if r.URL.Path != pathDeviceInit {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = r.ParseForm()
		*gotForm = r.PostForm
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	base = srv.URL
	return srv
}

// TestStartDeviceSendsResolvedScope asserts the OUTGOING FORM BODY, not the
// constant: the scope the client is configured with is what actually reaches the
// device endpoint. A test that only checks ResolveDeviceScope proves nothing
// about the wire.
func TestStartDeviceSendsResolvedScope(t *testing.T) {
	cases := []struct {
		name  string
		scope string // what we set on the client (empty = leave unset)
		want  string // what must appear in the form
	}{
		{"unset client scope falls back to the default", "", litDefaultScope},
		{"explicit default", litDefaultScope, litDefaultScope},
		{"generate", litGenerateScope, litGenerateScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var form url.Values
			srv := startDeviceServer(t, &form, http.StatusOK, DeviceAuth{
				DeviceCode: "dc", UserCode: "UC", VerificationURI: "https://x/y",
				ExpiresIn: 900, Interval: 5,
			})
			oc := NewOAuthClient(srv.URL)
			oc.Scope = tc.scope
			if _, err := oc.StartDevice(context.Background()); err != nil {
				t.Fatalf("StartDevice: %v", err)
			}
			if got := form.Get("scope"); got != tc.want {
				t.Errorf("device-init form scope = %q, want %q", got, tc.want)
			}
			if got := form.Get("client_id"); got != ClientID {
				t.Errorf("client_id = %q, want %q", got, ClientID)
			}
		})
	}
}

// TestStartDeviceMapsInvalidScopeForWidenedRequest: Running `--scopes generate`
// against a server whose allowedScopes has NOT been widened yields
// `invalid_scope`; the user must get an actionable message, not a raw OAuth
// code. civitai.com production HAS been widened (civitai/civitai#3699), so this
// now covers a self-hosted/older auth server — and any future set whose bits
// land outside allowedScopes.
func TestStartDeviceMapsInvalidScopeForWidenedRequest(t *testing.T) {
	var form url.Values
	srv := startDeviceServer(t, &form, http.StatusBadRequest, map[string]string{"error": "invalid_scope"})

	oc := NewOAuthClient(srv.URL)
	oc.Scope = litGenerateScope
	_, err := oc.StartDevice(context.Background())
	if err == nil {
		t.Fatal("StartDevice should fail on invalid_scope")
	}
	var ise *InvalidScopeError
	if !errors.As(err, &ise) {
		t.Fatalf("error is %T, want *InvalidScopeError: %v", err, err)
	}
	if ise.Requested != litGenerateScope {
		t.Errorf("InvalidScopeError.Requested = %q, want %q", ise.Requested, litGenerateScope)
	}
	msg := err.Error()
	for _, want := range []string{
		"invalid_scope",
		"civitai login",                // the fallback that works today
		"does not permit them",         // says it's the SERVER that lacks the scopes
		litGenerateScope,               // what was requested
		litDefaultScope,                // what the default requests
		"--scopes " + ScopeSetGenerate, // what to retry once it's live
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("invalid_scope message missing %q; got:\n%s", want, msg)
		}
	}
}

// TestStartDeviceMapsInvalidScopeForDefaultRequest: the SAME code on a DEFAULT
// login means something different (the CLI is ahead of the server entirely), so
// it must not tell the user to "run plain civitai login" — they just did.
func TestStartDeviceMapsInvalidScopeForDefaultRequest(t *testing.T) {
	var form url.Values
	srv := startDeviceServer(t, &form, http.StatusBadRequest,
		map[string]string{"error": "invalid_scope", "error_description": "scope exceeds allowedScopes"})

	_, err := NewOAuthClient(srv.URL).StartDevice(context.Background())
	var ise *InvalidScopeError
	if !errors.As(err, &ise) {
		t.Fatalf("error is %T, want *InvalidScopeError: %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "scope exceeds allowedScopes") {
		t.Errorf("message should carry the server's error_description; got:\n%s", msg)
	}
	if !strings.Contains(msg, "--token") {
		t.Errorf("the default-login case should offer the personal-key fallback; got:\n%s", msg)
	}
	if strings.Contains(msg, "--scopes "+ScopeSetGenerate) {
		t.Errorf("a DEFAULT login must not be told to retry --scopes generate; got:\n%s", msg)
	}
}

// TestStartDeviceLeavesOtherErrorsAlone is the negative control on the mapping:
// a DIFFERENT non-200 must keep the generic message, so the invalid_scope branch
// is proven to be selective rather than swallowing every failure.
func TestStartDeviceLeavesOtherErrorsAlone(t *testing.T) {
	var form url.Values
	srv := startDeviceServer(t, &form, http.StatusBadRequest, map[string]string{"error": "invalid_client"})

	oc := NewOAuthClient(srv.URL)
	oc.Scope = litGenerateScope
	_, err := oc.StartDevice(context.Background())
	if err == nil {
		t.Fatal("StartDevice should fail")
	}
	var ise *InvalidScopeError
	if errors.As(err, &ise) {
		t.Fatalf("invalid_client was mis-mapped to InvalidScopeError: %v", err)
	}
	if !strings.Contains(err.Error(), "device init failed") {
		t.Errorf("want the generic device-init error; got %q", err.Error())
	}
}

// TestRequestedScopeFallback pins the zero-value contract: a client nobody
// configured requests the default.
func TestRequestedScopeFallback(t *testing.T) {
	for in, want := range map[string]string{
		"":               DeviceScope,
		"   ":            DeviceScope,
		litGenerateScope: litGenerateScope,
	} {
		if got := (&OAuthClient{Scope: in}).RequestedScope(); got != want {
			t.Errorf("RequestedScope(%q) = %q, want %q", in, got, want)
		}
	}
}
