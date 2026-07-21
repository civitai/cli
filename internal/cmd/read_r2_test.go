package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// TestEmitJSONEscapesRawControlChars feeds emitJSON a body containing a raw,
// unescaped C0 control character (0x0D / CR) inside a string literal — which is
// invalid JSON, exactly what the Civitai API intermittently returns — and
// asserts the emitted output is valid JSON that decodes back to the intended
// value. This is the --json scripting-safety guarantee.
func TestEmitJSONEscapesRawControlChars(t *testing.T) {
	// {"p":"a\rb"} but with a RAW carriage return byte, not the two-char \r.
	raw := []byte("{\"p\":\"a\rb\"}")
	if json.Valid(raw) {
		t.Fatal("test fixture should be invalid JSON (raw CR inside a string)")
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := emitJSON(cmd, raw); err != nil {
		t.Fatalf("emitJSON returned error: %v", err)
	}

	got := out.Bytes()
	if !json.Valid(got) {
		t.Fatalf("emitJSON output is not valid JSON:\n%q", got)
	}
	var decoded struct {
		P string `json:"p"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("emitJSON output did not decode: %v\n%q", err, got)
	}
	if decoded.P != "a\rb" {
		t.Fatalf("decoded value = %q, want %q", decoded.P, "a\rb")
	}
}

// TestEmitJSONEscapesAllC0ControlChars covers the full 0x00–0x1F range inside a
// string, ensuring each raw control byte is escaped (via a named escape or
// \u00xx) rather than emitted raw.
func TestEmitJSONEscapesAllC0ControlChars(t *testing.T) {
	var body bytes.Buffer
	body.WriteString(`{"p":"`)
	for c := byte(0); c < 0x20; c++ {
		body.WriteByte(c) // raw control byte inside the string literal
	}
	body.WriteString(`x"}`)
	if json.Valid(body.Bytes()) {
		t.Fatal("fixture should be invalid JSON")
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := emitJSON(cmd, body.Bytes()); err != nil {
		t.Fatalf("emitJSON error: %v", err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("output not valid JSON:\n%q", out.Bytes())
	}
}

// TestEmitJSONValidInputUnchanged asserts already-valid JSON is only indented,
// with its string contents byte-identical (no spurious escaping).
func TestEmitJSONValidInputUnchanged(t *testing.T) {
	raw := []byte(`{"a":1,"b":"hello \"world\"","c":[1,2,3]}`)
	if !json.Valid(raw) {
		t.Fatal("fixture should be valid JSON")
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := emitJSON(cmd, raw); err != nil {
		t.Fatalf("emitJSON error: %v", err)
	}

	// Round-trip: indented output must re-marshal to the same canonical form.
	var want, got any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("valid input became unreadable: %v\n%q", err, out.Bytes())
	}
	wantB, _ := json.Marshal(want)
	gotB, _ := json.Marshal(got)
	if !bytes.Equal(wantB, gotB) {
		t.Fatalf("valid input altered: got %s want %s", gotB, wantB)
	}
	if !strings.Contains(out.String(), "\n") {
		t.Fatal("expected indented (multi-line) output for valid input")
	}
}

// TestEscapeJSONStringControlCharsLeavesStructuralWhitespace ensures control
// bytes OUTSIDE string literals (newlines/tabs between tokens, which are valid
// JSON whitespace) are not molested.
func TestEscapeJSONStringControlCharsLeavesStructuralWhitespace(t *testing.T) {
	raw := []byte("{\n\t\"a\": 1\n}")
	got := escapeJSONStringControlChars(raw)
	if !bytes.Equal(got, raw) {
		t.Fatalf("structural whitespace altered:\n got %q\nwant %q", got, raw)
	}
}

// TestCheckLimitExplicitZeroRejected drives the bindReadFlags PreRunE guard:
// an explicit `--limit 0` must be a usage error (exit code 2), while an UNSET
// limit must be allowed through to the command body (server default page).
func TestCheckLimitExplicitZeroRejected(t *testing.T) {
	// newCmd mirrors how a real read subcommand is wired: it binds --limit with
	// a 0 default, calls bindReadFlags (which installs the PreRunE guard), and in
	// RunE calls checkLimit with the endpoint max — exactly the images/models
	// search shape.
	newCmd := func() *cobra.Command {
		var limit int
		c := &cobra.Command{
			Use:           "search",
			SilenceUsage:  true,
			SilenceErrors: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return checkLimit(limit, 200)
			},
		}
		c.Flags().IntVar(&limit, "limit", 0, "results per page")
		bindReadFlags(c)
		return c
	}

	t.Run("explicit zero is a usage error", func(t *testing.T) {
		c := newCmd()
		c.SetArgs([]string{"--limit", "0"})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		err := c.Execute()
		if err == nil {
			t.Fatal("expected an error for --limit 0")
		}
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("--limit 0 error is not a usage error: %v", err)
		}
	})

	t.Run("explicit negative is a usage error", func(t *testing.T) {
		c := newCmd()
		c.SetArgs([]string{"--limit", "-5"})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		err := c.Execute()
		if err == nil || !errors.Is(err, ErrUsage) {
			t.Fatalf("--limit -5 should be a usage error, got: %v", err)
		}
	})

	t.Run("unset limit is allowed (server default)", func(t *testing.T) {
		c := newCmd()
		c.SetArgs([]string{})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		if err := c.Execute(); err != nil {
			t.Fatalf("unset --limit should be allowed, got: %v", err)
		}
	})

	t.Run("explicit in-range limit is allowed", func(t *testing.T) {
		c := newCmd()
		c.SetArgs([]string{"--limit", "20"})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		if err := c.Execute(); err != nil {
			t.Fatalf("--limit 20 should be allowed, got: %v", err)
		}
	})

	t.Run("explicit over-max limit is a usage error", func(t *testing.T) {
		c := newCmd()
		c.SetArgs([]string{"--limit", "9999"})
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&out)
		err := c.Execute()
		if err == nil || !errors.Is(err, ErrUsage) {
			t.Fatalf("--limit 9999 should be a usage error, got: %v", err)
		}
	})
}

// TestPrintPageFooterQuotesCursor asserts the copy-paste "next page" hint wraps
// the cursor in single quotes, so a cursor containing a space and a `|` pastes
// as one shell argument instead of word-splitting / piping.
func TestPrintPageFooterQuotesCursor(t *testing.T) {
	cursor := "2026-07-19 00:10:35.891|2788964"
	nc, _ := json.Marshal(cursor) // JSON string value, as the API returns it
	m := civitai.Metadata{NextCursor: json.RawMessage(nc)}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	printPageFooter(cmd, "civitai images search", m)

	s := out.String()
	if !strings.Contains(s, "--cursor '"+cursor+"'") {
		t.Fatalf("footer did not single-quote the cursor:\n%s", s)
	}
}
