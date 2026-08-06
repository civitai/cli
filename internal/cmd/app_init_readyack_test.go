package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// The scaffold ships a handshake file the author must not delete, and whose
// SILENCE during local preview is correct behaviour. Both of those are
// counter-intuitive, so `app init` says them at the moment the choice is made —
// and, like every other caveat in that output ("Commit the lockfile", pinned in
// app_validate_lockfile_test.go), the words need a test or they can be dropped
// in a refactor with the whole suite green.
//
// The assertions are on MEANING, not on exact sentences: the emitter's real
// path, that it must be kept, and that local preview shows the author's UI
// only. Rewording is fine; silently dropping the guidance is not.
func TestInitTellsTheAuthorAboutTheHandshakeFile(t *testing.T) {
	for _, tc := range []struct {
		tmpl    scaffold.Template
		wantAck string
	}{
		{scaffold.Static, "civitai-host.js"},
		{scaffold.PageVite, "src/civitai-host.js"},
	} {
		t.Run(string(tc.tmpl), func(t *testing.T) {
			// Guard against this test rotting into a tautology if the template
			// stops shipping an emitter: the caveat is only expected because
			// ReadyAckPath() says there IS one.
			if got := tc.tmpl.ReadyAckPath(); got != tc.wantAck {
				t.Fatalf("%s ReadyAckPath() = %q, want %q — update this test with the template",
					tc.tmpl, got, tc.wantAck)
			}

			dest := filepath.Join(t.TempDir(), "handshake-block")
			stdout, _, err := run(t, "app", "init", "handshake-block", dest, "--template", string(tc.tmpl))
			if err != nil {
				t.Fatalf("app init --template %s: %v\n%s", tc.tmpl, err, stdout)
			}

			for _, want := range []string{
				tc.wantAck,            // names the actual file, at its actual path
				handshakeCaveatPhrase, // the same string the negative control below forbids
				"keep it",             // says not to delete it
				"BLOCK_INIT",          // names why it is quiet locally
				"UI only",             // sets the expectation for local preview
				"never reveal",        // says what happens without it
			} {
				if !strings.Contains(stdout, want) {
					t.Errorf("`app init --template %s` next steps must mention %q — an author who "+
						"deletes the handshake file, or reads its local silence as a bug, ships a "+
						"broken app (issue #206). Output was:\n%s", tc.tmpl, want, stdout)
				}
			}
		})
	}
}

// handshakeCaveatPhrase is the part of the caveat that does NOT depend on the
// template's ReadyAckPath(), so an empty path cannot erase it.
const handshakeCaveatPhrase = "performs the host handshake"

// The caveat must NOT appear for a template that ships no emitter — otherwise
// it is unconditional text rather than a fact about the scaffold, and the
// assertions above would pass no matter what the templates do.
//
// 🔴 It asserts on the PHRASE, not on the filename, and that distinction is the
// entire test. The first version looked for `civitai-host.js` and was VACUOUS:
// replacing the guard in printScaffoldResult with `if true` survived the whole
// suite, because page-money's ReadyAckPath() is "" — so the line printed with an
// EMPTY filename ("   performs the host handshake — keep it. …") and the
// "filename is absent" assertion was satisfied BY THE BUG it should have
// caught. Spelling an assertion on a value the failure mode erases can only
// ever pass. The filename check is kept below as a second, weaker signal.
func TestInitOmitsTheHandshakeCaveatForSDKTemplates(t *testing.T) {
	if scaffold.PageMoney.ReadyAckPath() != "" {
		t.Fatal("page-money now ships an emitter — this control is no longer valid, revisit it")
	}
	dest := filepath.Join(t.TempDir(), "money-block")
	stdout, _, err := run(t, "app", "init", "money-block", dest, "--template", "page-money")
	if err != nil {
		t.Fatalf("app init --template page-money: %v\n%s", err, stdout)
	}
	if strings.Contains(stdout, handshakeCaveatPhrase) {
		t.Errorf("page-money ships no vendored emitter (the SDK transport acks), so its next steps "+
			"must not carry the handshake caveat. Finding %q here means the guard in "+
			"printScaffoldResult stopped discriminating and the line is being printed with an "+
			"empty filename:\n%s", handshakeCaveatPhrase, stdout)
	}
	if strings.Contains(stdout, "civitai-host.js") {
		t.Errorf("page-money's next steps must not name a vendored emitter:\n%s", stdout)
	}
}
