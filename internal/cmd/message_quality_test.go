package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// message_quality_test.go drives the THREE issue-#260 message defects through
// the real command tree, because the unit guards in internal/{validate,manifest,
// scaffold} each prove a correct string is PRODUCED and none of them proves it
// REACHES the author. A helper that is right and unreferenced passes its own
// package's tests and changes nothing anyone sees.
//
// The `--json` half is here for the reason AGENTS.md item 23 gives: a
// strings.Contains over raw stdout is satisfied by the word appearing anywhere,
// so the payload is DECODED and the finding is read out of the structure.

func manifestDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

// TestValidatePatternFindingReachesTheAuthor — issue #260 item 7, first bullet.
// `blockId` is the highest-traffic case: the first field a new author gets
// wrong, and an identity that cannot be renamed afterwards.
func TestValidatePatternFindingReachesTheAuthor(t *testing.T) {
	dir := manifestDir(t, `{"blockId":"My First App!","name":"x","version":"1.0.0",`+
		`"contentRating":"g","scopes":[],"kind":"page",`+
		`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`)

	_, stderr, err := run(t, "app", "validate", dir)
	if err == nil {
		t.Fatal("PREMISE BROKEN: the fixture validated clean")
	}
	// The finding is wrapped to 79 columns by the printer (AGENTS.md item 20),
	// so assert on fragments that cannot straddle a wrap point rather than on
	// the whole sentence.
	for _, want := range []string{
		"blockId: 'My First App!' does not match pattern",
		"lowercase letters, digits and hyphens",
		`(example: "my-first-app")`,
	} {
		if !strings.Contains(unwrapped(stderr), want) {
			t.Errorf("`app validate` stderr does not carry %q:\n%s", want, stderr)
		}
	}
	// CONTROL: exit 1, not 2. A validation verdict is not a usage error, and
	// this is asserted with errors.Is because the sentinels carry no visible
	// text (AGENTS.md item 7).
	if errors.Is(err, ErrUsage) {
		t.Error("a validation failure must not be classified as a usage error")
	}
}

// TestValidateEnumFindingWordingIsUnchanged is the CONTROL for the test above,
// at the SAME surface. The enum messages are the standard the pattern gloss was
// written to match; a rewrite of the finding renderer that regressed them has
// to fail somewhere a user-visible assertion can see it.
func TestValidateEnumFindingWordingIsUnchanged(t *testing.T) {
	dir := manifestDir(t, `{"blockId":"ok-app","name":"x","version":"1.0.0",`+
		`"contentRating":"zz","scopes":[],"kind":"page",`+
		`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`)

	_, stderr, err := run(t, "app", "validate", dir)
	if err == nil {
		t.Fatal("PREMISE BROKEN: the fixture validated clean")
	}
	const want = "contentRating: value must be one of 'g', 'pg', 'pg13', 'r', 'x'"
	if !strings.Contains(unwrapped(stderr), want) {
		t.Errorf("the enum finding's wording changed\n  want: %s\n  got:\n%s", want, stderr)
	}
	// The gloss is for PATTERN findings only — an enum finding must not grow
	// one, or "the rule in English" becomes noise on messages that already say
	// it.
	if strings.Contains(stderr, "(example:") {
		t.Errorf("an enum finding gained a pattern gloss:\n%s", stderr)
	}
}

// TestValidateMalformedJSONReportsALocation — issue #260 item 7, second bullet.
//
// The multi-byte row is the one that matters: it is the same manifest as the
// ASCII row in CHARACTERS and longer in BYTES, so both must report the same
// column. See internal/manifest/jsonloc_test.go for why a single fixture cannot
// see the bug.
func TestValidateMalformedJSONReportsALocation(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{
			name: "trailing comma on its own line (CONTROL)",
			body: "{\n  \"name\": \"plain\",\n  \"blockId\": \"ok-app\",\n}\n",
			want: "line 4, column 1",
		},
		// 🔴 THE PAIR. `aaaaaaaa` and `Café — 🚀` are 8 runes each and 8 vs 14
		// bytes, and the defect sits AFTER them on the same line — so a
		// byte-counted column makes the two rows disagree.
		//
		// The first version of this pair put the multi-byte text on line 2 and
		// the defect on a bare `}` on line 4, where the column is 1 either way:
		// it read like a discriminator and observed nothing. Measured, the
		// byte-counted mutant reddened it ZERO times.
		{
			name: "ASCII before the syntax error, same line (CONTROL)",
			body: "{\n  \"blockId\": \"ok-app\",\n  \"name\": \"aaaaaaaa\":\n}\n",
			want: "line 3, column 21",
		},
		{
			name: "multi-byte UTF-8 before the syntax error, same line",
			body: "{\n  \"blockId\": \"ok-app\",\n  \"name\": \"Café — 🚀\":\n}\n",
			want: "line 3, column 21",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := manifestDir(t, tc.body)
			_, stderr, err := run(t, "app", "validate", dir)
			if err == nil {
				t.Fatal("PREMISE BROKEN: the malformed fixture validated clean")
			}
			flat := unwrapped(stderr)
			if !strings.Contains(flat, tc.want) {
				t.Errorf("no location in the refusal\n  want: %s\n  got:\n%s", tc.want, stderr)
			}
			// The decoder's own reason must survive alongside the location.
			if !strings.Contains(flat, "invalid character") {
				t.Errorf("the decoder's reason was lost:\n%s", stderr)
			}
		})
	}
}

// TestValidateJSONCarriesTheLocationAndTheGloss reads the DECODED --json
// payload. Grouping findings by field in CI is what --json is for, so the
// location and the gloss have to be inside the `message` a consumer reads, not
// only in the human output.
func TestValidateJSONCarriesTheLocationAndTheGloss(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantField, wantIn string
	}{
		{
			name: "pattern gloss",
			body: `{"blockId":"My First App!","name":"x","version":"1.0.0",` +
				`"contentRating":"g","scopes":[],"kind":"page",` +
				`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`,
			wantField: "blockId",
			wantIn:    `(example: "my-first-app")`,
		},
		{
			name:      "json location",
			body:      "{\n  \"name\": \"Café — 🚀\",\n  \"blockId\": \"ok-app\",\n}\n",
			wantField: "(root)",
			wantIn:    "line 4, column 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := manifestDir(t, tc.body)
			stdout, _, err := run(t, "app", "validate", dir, "--json")
			if err == nil {
				t.Fatal("PREMISE BROKEN: the fixture validated clean")
			}
			var payload struct {
				OK     bool `json:"ok"`
				Errors []struct {
					Field   string `json:"field"`
					Message string `json:"message"`
				} `json:"errors"`
			}
			if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
				t.Fatalf("--json stdout is not valid JSON: %v\n%s", jerr, stdout)
			}
			if payload.OK {
				t.Fatal("PREMISE BROKEN: --json reported ok for a broken manifest")
			}
			var found bool
			for _, f := range payload.Errors {
				if f.Field == tc.wantField && strings.Contains(f.Message, tc.wantIn) {
					found = true
				}
				// AGENTS.md item 23: every finding carries a field.
				if f.Field == "" {
					t.Errorf("a finding came back with an empty field: %+v", f)
				}
				// The message is ONE line — the printer wraps, the payload
				// must not.
				if strings.Contains(f.Message, "\n") {
					t.Errorf("a --json message contains a newline: %q", f.Message)
				}
			}
			if !found {
				t.Errorf("no finding on %q containing %q\npayload: %+v", tc.wantField, tc.wantIn, payload.Errors)
			}
		})
	}
}

// TestAppCreateIntoNonEmptyDirNamesTheRemedy — issue #260 item 7, fourth
// bullet. The README's Troubleshooting section already had the remedy; the CLI
// did not print it.
//
// 🔴 BOTH SCAFFOLD COMMANDS ARE DRIVEN, not just `app create`. The exit-code
// half of this guard reddened exactly ONE leaf subtest when only `create` was
// covered — the "a battery rested on a single row" shape of AGENTS.md item 24,
// where deleting the row takes the guard with it. `app init` is the other user
// of the same refusal and is the command the README's Troubleshooting entry
// names.
func TestAppCreateIntoNonEmptyDirNamesTheRemedy(t *testing.T) {
	for _, verb := range []string{"create", "init"} {
		t.Run(verb, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("mine\n"), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}

			_, _, err := run(t, "app", verb, "dogx", "--dir", dir)
			if err == nil {
				t.Fatalf("PREMISE BROKEN: app %s overwrote a non-empty directory", verb)
			}
			msg := err.Error()
			if !strings.Contains(msg, "is not empty") {
				t.Errorf("the refusal lost its own reason:\n%s", msg)
			}
			// Derived from the constant, so a reword cannot leave this
			// asserting a stale copy.
			if !strings.Contains(msg, scaffold.NotEmptyRemedy) {
				t.Errorf("the refusal carries no remedy\n  want: %s\n  got:  %s", scaffold.NotEmptyRemedy, msg)
			}
			// The one clause the remedy exists to deliver, asserted at the
			// USER-VISIBLE surface as well as at the constant: without it the
			// next move on reading "refusing to overwrite" is to go hunting for
			// an override flag that does not exist.
			if !strings.Contains(msg, "no --force") {
				t.Errorf("the refusal does not say there is no --force:\n%s", msg)
			}
			// CONTROL — the published exit code does not move. A pre-existing
			// directory is not a mistake about the INVOCATION (the path is real
			// and is a directory), so this stays off exit 2. Asserted with
			// errors.Is, never with message text (AGENTS.md item 7).
			if errors.Is(err, ErrUsage) {
				t.Error("the non-empty-directory refusal must not become a usage error (exit 2)")
			}
			// CONTROL — the refusal is real: the directory is untouched.
			entries, rerr := os.ReadDir(dir)
			if rerr != nil {
				t.Fatalf("read back: %v", rerr)
			}
			if len(entries) != 1 || entries[0].Name() != "existing.txt" {
				t.Errorf("app %s modified a directory it refused: %v", verb, entries)
			}
		})
	}
}

// unwrapped collapses the printer's 79-column wrapping (AGENTS.md item 20) back
// into single-space-joined text, so a fragment assertion is about the MESSAGE
// rather than about where the layout happened to break.
func unwrapped(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
