package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// `civitai app submit` prints the App Storage schema-migration notes an
// accepted submit returns (`storageMigrationWarnings`, civitai/civitai
// src/pages/api/v1/blocks/submit-version.ts). Before this, the client decoded
// no such field, so an author never saw one. The warning strings below are
// copied from the server's checkStorageMigrations.

// storageWarningUnusedOp is the op-covers-nothing arm.
const storageWarningUnusedOp = "storage.collections.cards.migrations[0] (default) covers no breaking change " +
	"between the previous approved version and this one, so it will not run — remove it from the manifest"

// storageWarningDropped is the collection-no-longer-declared arm.
const storageWarningDropped = "storage: collection drafts is no longer declared; its stored documents are kept " +
	"and become reachable again if it is re-declared"

// storageAcceptedBody is a 200 carrying two migration warnings.
const storageAcceptedBody = `{"publishRequestId":"pr_sw","slug":"demo","version":"0.2.0","status":"pending",` +
	`"listing":null,"storageMigrationWarnings":["` + storageWarningUnusedOp + `","` + storageWarningDropped + `"]}`

// storagePlainBody is a 200 with nothing to say: the server sends an empty list.
const storagePlainBody = `{"publishRequestId":"pr_plain","slug":"demo","version":"0.2.0","status":"pending",` +
	`"listing":null,"storageMigrationWarnings":[]}`

// storageWarningsHeaderText is the fixed part of the header; case (c) asserts
// it never appears.
const storageWarningsHeaderText = "storage migration note(s) from the server"

func TestPrintStorageMigrationWarnings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		warnings []string
		want     []string // substrings, in order; nil = no output at all
	}{
		{"nil prints nothing", nil, nil},
		{"empty prints nothing", []string{}, nil},
		{"one", []string{storageWarningDropped},
			[]string{"1 " + storageWarningsHeaderText, "  - storage: collection drafts is no longer declared"}},
		{"two, in the server's order", []string{storageWarningUnusedOp, storageWarningDropped},
			[]string{"2 " + storageWarningsHeaderText,
				"  - storage.collections.cards.migrations[0] (default) covers no breaking",
				"  - storage: collection drafts"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			printStorageMigrationWarnings(&b, tc.warnings)
			if tc.want == nil && b.String() != "" {
				t.Errorf("want no output, got:\n%s", b.String())
			}
			assertInOrder(t, b.String(), tc.want)
		})
	}
}

// assertInOrder: each substring of want must appear in s, each after the
// previous. It asserts nothing about what else s holds.
func assertInOrder(t *testing.T, s string, want []string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := strings.Index(s[at:], w)
		if i < 0 {
			t.Errorf("output lacks %q after offset %d, got:\n%s", w, at, s)
			return
		}
		at += i + len(w)
	}
}

// TestSubmitStorageMigrationTextCannotForgeALine: a warning is SERVER text. An
// ESC or a newline in one must not reach the terminal as such, nor start a
// forged line.
func TestSubmitStorageMigrationTextCannotForgeALine(t *testing.T) {
	const forged = "✓ Submitted and approved"
	var b bytes.Buffer
	printStorageMigrationWarnings(&b, []string{"note\x1b[2J\r\n" + forged})
	s := b.String()
	if strings.ContainsAny(s, "\x1b\r") {
		t.Errorf("server text put an ESC or CR on the terminal: %q", s)
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), forged) {
			t.Errorf("server text forged a line of its own: %q\nfull output:\n%s", line, s)
		}
	}
	// Positive control: the text still arrives, sanitised.
	if !strings.Contains(s, "note") {
		t.Errorf("the sanitised text did not print, got:\n%s", s)
	}
}

// The seam: the REAL appapi client decoding a real 200 body, handed to the real
// doUpload.
func TestSubmitStorageMigrationWarningsThroughClient(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		want   []string
		absent bool // the header must appear nowhere
	}{
		{name: "(b) 200 prints storageMigrationWarnings", body: storageAcceptedBody, want: []string{
			"pr_sw",
			"2 " + storageWarningsHeaderText,
			"  - storage.collections.cards.migrations[0] (default) covers no breaking",
			"  - storage: collection drafts is no longer declared",
			"What's next",
		}},
		// Invariant guard: green on pre-change code by design. Its positive
		// control is (b), in the same harness.
		{name: "(c) 200 without warnings prints nothing new", body: storagePlainBody,
			want: []string{"pr_plain", "What's next"}, absent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			var out, errb bytes.Buffer
			c := &cobra.Command{}
			c.SetOut(&out)
			c.SetErr(&errb)
			m := &manifest.Manifest{BlockID: "demo", Version: "0.2.0", Name: "Demo"}
			if err := doUpload(c, appapi.New(srv.URL, "tok", ""), []byte("ZIP"), m, srv.URL, appapi.Provenance{}); err != nil {
				t.Fatalf("doUpload: %v", err)
			}
			assertInOrder(t, out.String(), tc.want)
			if tc.absent && strings.Contains(out.String()+errb.String(), storageWarningsHeaderText) {
				t.Errorf("no warnings were sent, so %q must not print\nstdout:\n%s\nstderr:\n%s",
					storageWarningsHeaderText, out.String(), errb.String())
			}
		})
	}
}
