package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// `civitai app submit` prints the two App Storage schema-migration fields the
// submit routes return (civitai/civitai src/pages/api/v1/blocks/submit-version.ts):
// `storageBreakingChanges` on a refused submit (400) and
// `storageMigrationWarnings` on an accepted one (200). Before this, the client
// decoded neither, so an author never saw a warning and saw a refusal only as
// one long message line.
//
// The fixtures below are the server's own shapes: a BreakingChange is
// {collection, pointer, keyword, old, new} with `old`/`new` the keyword's values
// (null when absent), and the warning text is copied from checkStorageMigrations.

// storageWarningUnusedOp is a real checkStorageMigrations warning string (the
// op-covers-nothing arm).
const storageWarningUnusedOp = "storage.collections.cards.migrations[0] (default) covers no breaking change " +
	"between the previous approved version and this one, so it will not run — remove it from the manifest"

// storageWarningDropped is the collection-no-longer-declared arm.
const storageWarningDropped = "storage: collection drafts is no longer declared; its stored documents are kept " +
	"and become reachable again if it is re-declared"

// storageRefusalBody is a 400 the v1 route answers for a StorageMigrationRefusal:
// two uncovered changes, a newly required field and a tightened maxLength.
const storageRefusalBody = `{"message":"storage: 2 breaking schema change(s) since the previous approved version ` +
	`can leave documents already stored unwritable, and no migration covers them: cards (the document root) ` +
	`required: [\"title\"] → [\"title\",\"status\"]; cards \"/title\" maxLength: 120 → 80. For each, declare an op ` +
	`under storage.collections.<name>.migrations","storageBreakingChanges":[` +
	`{"collection":"cards","pointer":"","keyword":"required","old":["title"],"new":["title","status"]},` +
	`{"collection":"cards","pointer":"/title","keyword":"maxLength","old":120,"new":80}]}`

// storageAcceptedBody is a 200 carrying two migration warnings.
const storageAcceptedBody = `{"publishRequestId":"pr_sw","slug":"demo","version":"0.2.0","status":"pending",` +
	`"listing":null,"storageMigrationWarnings":["` + storageWarningUnusedOp + `","` + storageWarningDropped + `"]}`

// storagePlainBody is a 200 with neither field's content: the server sends an
// empty warnings list when there is nothing to say.
const storagePlainBody = `{"publishRequestId":"pr_plain","slug":"demo","version":"0.2.0","status":"pending",` +
	`"listing":null,"storageMigrationWarnings":[]}`

// storageStrings is every line the two printers can contribute; case (c)
// asserts none of them appears.
var storageStrings = []string{
	"storage schema change(s) that no migration covers",
	"storage migration note(s) from the server",
	"Declare an op for each",
}

func TestShownStorageValue(t *testing.T) {
	long := `"` + strings.Repeat("é", 100) + `"`
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"absent key", nil, "(none)"},
		{"json null", json.RawMessage(`null`), "(none)"},
		{"number", json.RawMessage(`120`), "120"},
		{"string", json.RawMessage(`"integer"`), `"integer"`},
		{"array is compacted like JSON.stringify", json.RawMessage(`[ "title" ,  "status" ]`), `["title","status"]`},
		{"cut at 80 runes, not bytes", json.RawMessage(long), `"` + strings.Repeat("é", 79) + "…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shownStorageValue(tc.raw); got != tc.want {
				t.Errorf("shownStorageValue(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDescribeStorageBreakingChange(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   appapi.StorageBreakingChange
		want string
	}{
		{
			"document root, newly required field",
			appapi.StorageBreakingChange{Collection: "cards", Pointer: "", Keyword: "required",
				Old: json.RawMessage(`["title"]`), New: json.RawMessage(`["title","status"]`)},
			`cards (the document root) required: ["title"] → ["title","status"]`,
		},
		{
			"nested pointer, tightened maxLength",
			appapi.StorageBreakingChange{Collection: "notes", Pointer: "/meta/title", Keyword: "maxLength",
				Old: json.RawMessage(`120`), New: json.RawMessage(`80`)},
			`notes "/meta/title" maxLength: 120 → 80`,
		},
		{
			"first schema: the old side is absent",
			appapi.StorageBreakingChange{Collection: "tags", Pointer: "/*", Keyword: "type",
				Old: json.RawMessage(`null`), New: json.RawMessage(`"string"`)},
			`tags "/*" type: (none) → "string"`,
		},
		{
			"field removed under additionalProperties false",
			appapi.StorageBreakingChange{Collection: "cards", Pointer: "/stars", Keyword: "removed",
				Old: json.RawMessage(`{"type":"integer"}`)},
			`cards "/stars" removed: {"type":"integer"} → (none)`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeStorageBreakingChange(tc.in); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestPrintStorageMigrationWarnings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		warnings []string
		want     []string // substrings, in order
	}{
		{"nil prints nothing", nil, nil},
		{"empty prints nothing", []string{}, nil},
		{"one", []string{storageWarningDropped},
			[]string{"1 storage migration note(s) from the server", "  - storage: collection drafts is no longer declared"}},
		{"two, in the server's order", []string{storageWarningUnusedOp, storageWarningDropped},
			[]string{"2 storage migration note(s) from the server",
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

func TestPrintStorageBreakingChanges(t *testing.T) {
	two := []appapi.StorageBreakingChange{
		{Collection: "cards", Keyword: "required", Old: json.RawMessage(`["title"]`), New: json.RawMessage(`["title","status"]`)},
		{Collection: "cards", Pointer: "/title", Keyword: "maxLength", Old: json.RawMessage(`120`), New: json.RawMessage(`80`)},
	}
	for _, tc := range []struct {
		name    string
		changes []appapi.StorageBreakingChange
		want    []string
	}{
		{"nil prints nothing", nil, nil},
		// A refusal over an invalid migration op alone carries an EMPTY list.
		{"empty prints nothing", []appapi.StorageBreakingChange{}, nil},
		{"two", two, []string{
			"The server refused this submit over 2 storage schema change(s) that no migration covers:",
			`  - cards (the document root) required: ["title"] → ["title","status"]`,
			`  - cards "/title" maxLength: 120 → 80`,
			"Declare an op for each under storage.collections.<name>.migrations",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			printStorageBreakingChanges(&b, tc.changes)
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

// TestSubmitStorageMigrationTextCannotForgeALine: every field printed here is
// SERVER text. An ESC or a newline in a warning, a collection, a pointer or a
// keyword must not reach the terminal as one, nor start a forged line.
func TestSubmitStorageMigrationTextCannotForgeALine(t *testing.T) {
	const forged = "✓ Submitted and approved"
	var b bytes.Buffer
	printStorageMigrationWarnings(&b, []string{"note\x1b[2J\n" + forged})
	printStorageBreakingChanges(&b, []appapi.StorageBreakingChange{{
		Collection: "cards\x1b[31m\n" + forged, Pointer: "/t\r\n" + forged, Keyword: "maxLength\x1b[0m",
		Old: json.RawMessage(`"a\u001b[2Jb"`), New: json.RawMessage(`1`),
	}})
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
	if !strings.Contains(s, "note") || !strings.Contains(s, "cards") {
		t.Errorf("the sanitised text did not print, got:\n%s", s)
	}
}

// submitAgainst drives the REAL appapi client against a server answering the
// submit with (status, body), through uploadAndReport — the seam the command
// uses — and returns stdout, stderr, the --json writer's bytes and the error.
func submitAgainst(t *testing.T, status int, body string, withJSON bool) (stdout, stderr, jsonOut string, err error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	var out, errb, jb bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)
	c.SetErr(&errb)
	m := &manifest.Manifest{BlockID: "demo", Version: "0.2.0", Name: "Demo"}
	// An untyped nil, never a nil *bytes.Buffer: that would be a NON-nil
	// io.Writer and take the --json branch.
	var jw io.Writer
	if withJSON {
		jw = &jb
	}
	err = uploadAndReport(c, appapi.New(srv.URL, "tok", ""), []byte("ZIP"), m, srv.URL, appapi.Provenance{}, jw)
	return out.String(), errb.String(), jb.String(), err
}

func TestSubmitStorageMigrationOutputThroughClient(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantStdout []string
		wantStderr []string
		// absent is checked against stdout AND stderr together.
		absent []string
	}{
		{
			name: "(a) 400 refusal lists storageBreakingChanges", status: http.StatusBadRequest,
			body: storageRefusalBody, wantErr: true,
			wantStderr: []string{
				"The server refused this submit over 2 storage schema change(s) that no migration covers:",
				`  - cards (the document root) required: ["title"] → ["title","status"]`,
				`  - cards "/title" maxLength: 120 → 80`,
				"Declare an op for each",
			},
		},
		{
			name: "(b) 200 prints storageMigrationWarnings", status: http.StatusOK, body: storageAcceptedBody,
			wantStdout: []string{
				"pr_sw",
				"2 storage migration note(s) from the server",
				"  - storage.collections.cards.migrations[0] (default) covers no breaking",
				"  - storage: collection drafts is no longer declared",
				"What's next",
			},
		},
		{
			name: "(c) 200 without either prints nothing new", status: http.StatusOK, body: storagePlainBody,
			wantStdout: []string{"pr_plain", "What's next"},
			absent:     storageStrings,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, _, err := submitAgainst(t, tc.status, tc.body, false)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v\nstdout:\n%s\nstderr:\n%s", err, tc.wantErr, stdout, stderr)
			}
			if tc.wantErr {
				var refused *appapi.SubmitRefusedError
				if !errors.As(err, &refused) || refused.Status != tc.status {
					t.Errorf("want a SubmitRefusedError with status %d, got %#v", tc.status, err)
				}
			}
			assertInOrder(t, stdout, tc.wantStdout)
			assertInOrder(t, stderr, tc.wantStderr)
			for _, bad := range tc.absent {
				if strings.Contains(stdout+stderr, bad) {
					t.Errorf("output must not contain %q\nstdout:\n%s\nstderr:\n%s", bad, stdout, stderr)
				}
			}
		})
	}
}

// TestSubmitJSONEmitsTheServerBodyVerbatim: under --json the response body
// reaches the JSON writer byte-for-byte (plus one newline) for a refusal, an
// accepted submit with warnings and one without — and the human report still
// prints to the command's Out, which RunE points at stderr under --json.
func TestSubmitJSONEmitsTheServerBodyVerbatim(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"(a) 400 refusal", http.StatusBadRequest, storageRefusalBody},
		{"(b) 200 with warnings", http.StatusOK, storageAcceptedBody},
		{"(c) 200 without either", http.StatusOK, storagePlainBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, j, _ := submitAgainst(t, tc.status, tc.body, true)
			if j != tc.body+"\n" {
				t.Errorf("--json output is not the server body verbatim\ngot:  %q\nwant: %q", j, tc.body+"\n")
			}
			if stdout+stderr == "" {
				t.Errorf("the human report vanished under --json")
			}
		})
	}
}

// A refusal whose body is not JSON (a proxy's HTML page) must not put that
// page on --json's stdout.
func TestSubmitJSONWritesNothingForANonJSONRefusal(t *testing.T) {
	_, _, j, err := submitAgainst(t, http.StatusBadGateway, "<html>bad gateway</html>", true)
	if err == nil {
		t.Fatal("a 502 must fail the submit")
	}
	if j != "" {
		t.Errorf("--json wrote a non-JSON body: %q", j)
	}
}

// The full command: --json moves every human line to stderr so stdout is the
// server body alone, and --json with --package-only is a usage error.
func TestAppSubmitJSONStdoutIsOnlyTheServerBody(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(storageAcceptedBody))
	}))
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-json")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "")

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes", "--json")
	if err != nil {
		t.Fatalf("submit --json: %v\nstderr:\n%s", err, stderr)
	}
	if stdout != storageAcceptedBody+"\n" {
		t.Errorf("stdout must be the server body alone\ngot:  %q\nwant: %q", stdout, storageAcceptedBody+"\n")
	}
	for _, want := range []string{"Packaged ", "pr_sw", "2 storage migration note(s) from the server"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q under --json, got:\n%s", want, stderr)
		}
	}
}

func TestAppSubmitJSONWithPackageOnlyIsAUsageError(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)
	_, _, err := run(t, "app", "submit", tmp, "--package-only", "--json")
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--json --package-only: want ErrUsage, got %v", err)
	}
}
