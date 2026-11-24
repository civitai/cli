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

// `civitai app submit` prints the optional, non-blocking `advisories` an
// accepted submit may return. The field may not exist on the server yet, so the
// absent and malformed shapes must print nothing and leave the submit a
// success.

// submitAdvisoriesHeaderText is the fixed part of the header; the absent cases
// assert it never appears.
const submitAdvisoriesHeaderText = "suggestion(s) from the server (advisory"

const advisoryThemeMessage = "This app ships Civitai's stock look; consider giving it its own accent colour."
const advisoryThemeDocs = "https://docs.example.test/theming#own-look"

func TestPrintSubmitAdvisories(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []appapi.SubmitAdvisory
		want []string // substrings, in order; nil = no output at all
	}{
		{"nil prints nothing", nil, nil},
		{"empty prints nothing", []appapi.SubmitAdvisory{}, nil},
		{"with docs link", []appapi.SubmitAdvisory{{Code: "stock-theme", Message: advisoryThemeMessage, DocsURL: advisoryThemeDocs}},
			[]string{"1 " + submitAdvisoriesHeaderText, "  - This app ships Civitai's stock look;", "[stock-theme]",
				"    See: " + advisoryThemeDocs}},
		{"no code, no docs, in the server's order",
			[]appapi.SubmitAdvisory{{Message: "first note"}, {Code: "c2", Message: "second note"}},
			[]string{"2 " + submitAdvisoriesHeaderText, "  - first note\n", "  - second note [c2]"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			printSubmitAdvisories(&b, tc.in)
			if tc.want == nil && b.String() != "" {
				t.Errorf("want no output, got:\n%s", b.String())
			}
			assertInOrder(t, b.String(), tc.want)
			if tc.name == "no code, no docs, in the server's order" && strings.Contains(b.String(), "See:") {
				t.Errorf("no docsUrl was sent, so no See: line may print:\n%s", b.String())
			}
		})
	}
}

// TestSubmitAdvisoriesCannotForgeALine: every advisory field is SERVER text. An
// ESC, a CR or a newline in any of them must not reach the terminal as such, nor
// start a forged line.
func TestSubmitAdvisoriesCannotForgeALine(t *testing.T) {
	const forged = "✓ Submitted and approved"
	const evil = "\x1b[2J\r\n" + forged
	for _, tc := range []struct {
		name string
		adv  appapi.SubmitAdvisory
		ctl  string // the benign text that must still print
	}{
		{"message", appapi.SubmitAdvisory{Code: "c", Message: "msgnote" + evil}, "msgnote"},
		{"code", appapi.SubmitAdvisory{Code: "codenote" + evil, Message: "m"}, "codenote"},
		{"docsUrl", appapi.SubmitAdvisory{Code: "c", Message: "m", DocsURL: "docsnote" + evil}, "docsnote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			printSubmitAdvisories(&b, []appapi.SubmitAdvisory{tc.adv})
			s := b.String()
			if strings.ContainsAny(s, "\x1b\r") {
				t.Errorf("server text put an ESC or CR on the terminal: %q", s)
			}
			for _, line := range strings.Split(s, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), forged) {
					t.Errorf("server text forged a line of its own: %q\nfull output:\n%s", line, s)
				}
			}
			if !strings.Contains(s, tc.ctl) {
				t.Errorf("positive control: the sanitised %s did not print, got:\n%s", tc.name, s)
			}
		})
	}
}

// The seam: the REAL appapi client decoding a real 200 body, handed to the real
// doUpload. doUpload returning nil is the exit-code claim: advisories never fail
// a submit.
func TestSubmitAdvisoriesThroughClient(t *testing.T) {
	const head = `{"publishRequestId":"pr_adv","slug":"demo","version":"0.2.0","status":"pending","listing":null`
	for _, tc := range []struct {
		name   string
		body   string
		want   []string
		absent bool // the header must appear nowhere
	}{
		{name: "advisories print after the success line", body: head + `,"advisories":[{"code":"stock-theme","message":"` +
			advisoryThemeMessage + `","docsUrl":"` + advisoryThemeDocs + `"}]}`,
			want: []string{"pr_adv", "1 " + submitAdvisoriesHeaderText, "  - This app ships Civitai's stock look;",
				"    See: " + advisoryThemeDocs, "What's next"}},
		// Invariant guards: green on pre-change code by design (the client then
		// ignored the key). Their positive control is the case above.
		{name: "absent prints nothing", body: head + `}`, want: []string{"pr_adv", "What's next"}, absent: true},
		{name: "empty prints nothing", body: head + `,"advisories":[]}`, want: []string{"pr_adv", "What's next"}, absent: true},
		// Red on pre-change code only once the field is typed: a typed decode
		// would turn this ACCEPTED submit into an error.
		{name: "malformed is ignored and the submit still succeeds", body: head + `,"advisories":"oops"}`,
			want: []string{"pr_adv", "What's next"}, absent: true},
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
				t.Fatalf("doUpload: %v — an advisory must never fail an accepted submit", err)
			}
			assertInOrder(t, out.String(), tc.want)
			if tc.absent && strings.Contains(out.String()+errb.String(), submitAdvisoriesHeaderText) {
				t.Errorf("no advisories were usable, so %q must not print\nstdout:\n%s\nstderr:\n%s",
					submitAdvisoriesHeaderText, out.String(), errb.String())
			}
		})
	}
}
