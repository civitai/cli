package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `app listing set-text` THROUGH THE MATRIX DRIVER (civitai/cli#762, decision D3).
//
// 🔴 WHY IT HAD TO BE REACHABLE AT ALL. `runner.py` refuses
// `civitai app listing set-text` under a credential unless `--allow-listing-text`
// is passed, and that flag was reachable only by invoking runner.py directly. So
// the one instrument that drives real agents end to end could not reach the
// command that sets a listing DESCRIPTION — which makes it blind to exactly the
// defect `civitai app submit`'s listing-completeness gate closes. A trial leaving
// `empty-description` behind was measuring the harness, not the product.
//
// 🔴 AND WHY THE DEFAULT STAYS OFF. `set-text` rewrites a listing's public
// tagline/description/category IN PLACE on every listing status — no revision, no
// moderator review, public the moment it returns — and this CLI ships no command
// that restores the previous value. The account a credentialed matrix runs against
// owns real published listings.
//
// These drive the real `driver.sh` through `runStubbedDriver` (dogfood_brief_test.go),
// which puts a STUB `python3` first on PATH and reads back the argv one cell was
// launched with. So what is asserted is the flag the RUNNER would have received,
// not a grep of the script.

// credentialedEnv is the environment a credentialed matrix needs: driver.sh
// refuses one without a non-empty credential file and an app prefix, and refuses
// the default `t-` namespace outright.
func credentialedEnv(t *testing.T) []string {
	t.Helper()
	cred := filepath.Join(t.TempDir(), "cred.json")
	if err := os.WriteFile(cred, []byte(`{"token":"stub"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{
		"DOGFOOD_CREDENTIAL_FILE=" + cred,
		"DOGFOOD_APP_PREFIX=dogfoodzz-",
		"DOGFOOD_TRIAL_PREFIX=tz",
	}
}

// The REGRESSION arm: at origin/main `--allow-listing-text` is threaded nowhere,
// so a credentialed matrix cannot reach `set-text` however it is configured.
func TestDogfoodDriverPassesAllowListingTextWhenAsked(t *testing.T) {
	env := append(credentialedEnv(t), "DOGFOOD_ALLOW_LISTING_TEXT=1")
	argv, code, out := runStubbedDriver(t, append(env, oneCellEnv...))
	if code != 0 {
		t.Fatalf("driver.sh exited %d, want 0\n%s", code, out)
	}
	if !contains(argv, "--allow-listing-text") {
		t.Fatalf("DOGFOOD_ALLOW_LISTING_TEXT=1 did not reach the runner — a matrix trial still cannot "+
			"set a listing description: %v", argv)
	}
	// 🔴 THE OPT-IN IS LOUD. A write with no undo that arms silently is a write
	// nobody knows is armed, and the banner is the only thing an operator reading
	// the run's output can see it in.
	for _, want := range []string{"DOGFOOD_ALLOW_LISTING_TEXT=1", "set-text", "NO undo"} {
		if !strings.Contains(out, want) {
			t.Errorf("the arming banner does not mention %q — the opt-in is silent:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "dogfoodzz-") {
		t.Errorf("the banner does not name the app prefix, which is the only thing keeping the writes "+
			"off the account's own listings:\n%s", out)
	}
}

// The DEFAULT arm: an unset variable leaves runner.py's own refusal in place, and
// the banner must not appear — a warning printed on every run is a warning nobody
// reads on the run that matters.
func TestDogfoodDriverDefaultsToRefusingListingText(t *testing.T) {
	argv, code, out := runStubbedDriver(t, append(credentialedEnv(t), oneCellEnv...))
	if code != 0 {
		t.Fatalf("driver.sh exited %d, want 0\n%s", code, out)
	}
	if contains(argv, "--allow-listing-text") {
		t.Fatalf("the default matrix armed `set-text`: %v", argv)
	}
	if strings.Contains(out, "DOGFOOD_ALLOW_LISTING_TEXT=1") {
		t.Errorf("the arming banner printed on a run that did not arm it:\n%s", out)
	}
	// CONTROL: a cell really was launched, so the absence above is about the flag
	// rather than about a matrix that ran nothing.
	if len(argv) == 0 {
		t.Fatalf("no trial was launched, so this test observed nothing:\n%s", out)
	}
	if !contains(argv, "--credential-file") {
		t.Fatalf("the credentialed cell did not launch with a credential: %v", argv)
	}
}

// 🔴 THE AMBIGUOUS-VALUE ARM, AND IT IS THE ONE WITH TEETH. The obvious
// `[ -n "$X" ]` test arms on `0`, on `false` and on `no` — three spellings an
// operator reaching for OFF would plausibly type. For a write with no undo,
// guessing in either direction is wrong, so only the literal `1` arms it and
// anything else is a hard refusal before any cell starts.
func TestDogfoodDriverRefusesAnAmbiguousAllowListingText(t *testing.T) {
	for _, val := range []string{"0", "false", "no", "yes", "true", "TRUE", " 1"} {
		t.Run(val, func(t *testing.T) {
			env := append(credentialedEnv(t), "DOGFOOD_ALLOW_LISTING_TEXT="+val)
			argv, code, out := runStubbedDriver(t, append(env, oneCellEnv...))
			if code == 0 {
				t.Fatalf("driver.sh accepted DOGFOOD_ALLOW_LISTING_TEXT=%q instead of refusing it\n%s", val, out)
			}
			if len(argv) != 0 {
				t.Fatalf("driver.sh refused but still launched a trial: %v", argv)
			}
			if !strings.Contains(out, "DOGFOOD_ALLOW_LISTING_TEXT") {
				t.Errorf("the refusal does not name the variable it is about:\n%s", out)
			}
		})
	}
}

// An uncredentialed matrix reaches no account, so the flag changes nothing — but
// an operator who set it meant to arm something and is entitled to be told it did
// not. Not fatal, because the setup matrix is the one thing that must keep running
// under any configuration.
func TestDogfoodDriverSaysAllowListingTextIsInertWithoutACredential(t *testing.T) {
	argv, code, out := runStubbedDriver(t, append([]string{
		"DOGFOOD_ALLOW_LISTING_TEXT=1",
		"DOGFOOD_TRIAL_PREFIX=tz",
	}, oneCellEnv...))
	if code != 0 {
		t.Fatalf("an uncredentialed matrix must still run; exited %d\n%s", code, out)
	}
	if len(argv) == 0 {
		t.Fatalf("no trial was launched:\n%s", out)
	}
	if !strings.Contains(out, "inert without DOGFOOD_CREDENTIAL_FILE") {
		t.Errorf("driver.sh armed nothing and said nothing:\n%s", out)
	}
}

// contains is spelled here rather than reaching for a helper in another test file,
// so this file's assertions cannot be changed by an edit somewhere else.
func contains(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}
