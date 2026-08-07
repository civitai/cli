package cmd

// app_validate_wrap_test.go pins the OTHER half of issue #258: the ready-ack
// advisory is a ~2 kB paragraph, and `app validate` printed it as ONE
// 1938-character line.
//
// 🔴 THE TWO SURFACES HAVE OPPOSITE REQUIREMENTS, AND ONE FIX CANNOT SERVE BOTH
// FROM THE PRODUCER. `--json` needs the message to stay a single line — it is a
// string field a consumer reads — while the terminal needs it broken to a width
// the producer cannot know. So the layout lives at the printer
// (validate_print.go) and the message stays flat, which is the inverse of
// AGENTS.md item 23's rule for a finding's `Field`. Both directions are asserted
// here, because either alone is satisfied by a broken fix: wrap in the message
// and the text test passes while `--json` is corrupted; wrap nowhere and the
// `--json` test passes while the terminal is unreadable.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/scaffold"
)

// prefixScaffold renders `static` and deletes the emitter — the canonical #206
// project, and the one that produces the longest message this CLI emits.
func prefixScaffold(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "block")
	if _, err := scaffold.Render(scaffold.Static, dir, scaffold.Data{Slug: "wrap-block", Name: "Wrap Block"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, blockproto.ReadyAckFilename)); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestValidateJSONMessagesAreOneLine is the wire contract.
//
// 🔴 IT DECODES. Per AGENTS.md item 23, a `strings.Contains(out, …)` over raw
// `--json` stdout cannot tell a real newline inside a message from the `\n`
// escape `encoding/json` writes for one — the escape is what a naive text test
// sees, and it looks identical either way. So the payload is unmarshalled and
// the decoded Go string is checked.
func TestValidateJSONMessagesAreOneLine(t *testing.T) {
	dir := prefixScaffold(t)
	stdout, _, err := run(t, "app", "validate", dir, "--json")
	if err != nil {
		t.Fatalf("validate --json: %v\n%s", err, stdout)
	}
	var payload map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
		t.Fatalf("--json output is not valid JSON: %v\n%s", jerr, stdout)
	}

	checked := 0
	for _, key := range []string{"errors", "warnings"} {
		list, _ := payload[key].([]any)
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("%s[] element is not an object: %#v", key, item)
			}
			msg, ok := m["message"].(string)
			if !ok {
				t.Fatalf("%s[] element has no string message: %#v", m, m)
			}
			if strings.ContainsAny(msg, "\n\r") {
				t.Errorf("a %s message carries a line break — wrapping belongs at the printer, not in the "+
					"message, or every --json consumer gets a multi-line string field:\n%q", key, msg)
			}
			checked++
		}
	}
	// POSITIVE CONTROL. A zero here is indistinguishable from a project that
	// produced no findings at all, and this fixture must produce the ready-ack
	// advisory — the longest message the CLI emits, and the one that provoked
	// this test.
	if checked == 0 {
		t.Fatal("the fixture produced NO findings, so every assertion above is vacuous — a `static` scaffold " +
			"with its emitter deleted must warn")
	}
	if !strings.Contains(stdout, "BLOCK_READY") {
		t.Fatalf("the ready-ack advisory is absent from --json; this test is checking the wrong findings:\n%s", stdout)
	}
}

// TestValidateTextOutputIsWrapped is the terminal half.
func TestValidateTextOutputIsWrapped(t *testing.T) {
	dir := prefixScaffold(t)
	stdout, stderr, err := run(t, "app", "validate", dir)
	if err != nil {
		t.Fatalf("validate: %v\n%s\n%s", err, stdout, stderr)
	}

	longest, lines := 0, 0
	advisoryLines := 0
	inAdvisory := false
	for _, line := range strings.Split(strings.TrimRight(stderr, "\n"), "\n") {
		lines++
		if n := len([]rune(line)); n > longest {
			longest = n
		}
		if strings.HasPrefix(line, findingBullet) {
			inAdvisory = strings.Contains(line, "page")
		}
		if inAdvisory && strings.HasPrefix(line, "  ") {
			advisoryLines++
		}
	}
	if lines == 0 {
		t.Fatal("validate printed nothing to stderr — the fixture is not warning and this test is vacuous")
	}
	if longest > findingWrapWidth {
		t.Errorf("a stderr line is %d runes wide, over the %d-rune budget — the finding was not wrapped:\n%s",
			longest, findingWrapWidth, stderr)
	}
	// 🔴 The WIDTH assertion alone is satisfied by a build that prints nothing
	// long, so require the advisory to have actually been BROKEN UP. It is ~2 kB;
	// at this width that is tens of lines, and any single-digit count means the
	// message shrank rather than wrapped.
	if advisoryLines < 10 {
		t.Errorf("the ready-ack advisory occupies only %d line(s) — it is ~2 kB, so this is not a wrapped "+
			"paragraph:\n%s", advisoryLines, stderr)
	}

	// The content survives the layout: unwrapping reproduces the logical message,
	// including the issue-#258 gap report naming the reference that broke.
	flat := unwrapFinding(stderr)
	for _, want := range []string{
		"did NOT check that the file is loaded",
		`index.html <script src> "./civitai-host.js" points at civitai-host.js, which does not exist`,
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("the wrapped output no longer carries %q — layout must not lose content:\n%s", want, stderr)
		}
	}
}

// TestWrapRunesHangingIndent pins the shape at the function, where the input can
// be controlled: the continuation indent must be the same width as the bullet,
// or a wrapped finding stops reading as one list item.
func TestWrapRunesHangingIndent(t *testing.T) {
	if len(findingBullet) != len(findingIndent) {
		t.Fatalf("bullet %q and indent %q differ in width; the text column will not line up",
			findingBullet, findingIndent)
	}
	var b strings.Builder
	printFinding(&b, strings.Repeat("word ", 200))
	got := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(got) < 5 {
		t.Fatalf("1000 characters wrapped to %d line(s) at width %d", len(got), findingWrapWidth)
	}
	if !strings.HasPrefix(got[0], findingBullet) {
		t.Errorf("the first line does not carry the bullet: %q", got[0])
	}
	for i, line := range got[1:] {
		if !strings.HasPrefix(line, findingIndent) || strings.HasPrefix(line, findingBullet) {
			t.Errorf("continuation line %d is not hanging-indented: %q", i+1, line)
		}
		if len([]rune(line)) > findingWrapWidth {
			t.Errorf("continuation line %d is %d runes: %q", i+1, len([]rune(line)), line)
		}
	}

	// A short message stays on one line — wrapping must not shred every finding.
	var short strings.Builder
	printFinding(&short, "blockId: 'Bad_Id' does not match pattern")
	if n := strings.Count(short.String(), "\n"); n != 1 {
		t.Errorf("a short finding printed on %d lines, want 1:\n%s", n, short.String())
	}
}
