package genapi

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// bodyEchoSentinel stands in for a credential in a body the CLI could not parse.
const bodyEchoSentinel = "SENTINEL-UNPARSED-BODY-4d7b"

// forgedBanner is the measured civitai/cli#620 payload: a newline, then
// cursor-up + erase-line, then a counterfeit submit banner with a tab in it.
const forgedBanner = "\n\x1b[1A\x1b[2K✓ Generation submitted — workflow wf_evil\tCharged 0 Buzz"

// assertBodyNotEchoed is the shared check for every row below: the error names
// its route, carries no byte of the body's sentinel, holds no control character
// at all (so no ESC, newline or tab), and is bounded whatever the body's size.
func assertBodyNotEchoed(t *testing.T, err error, wantText string) {
	t.Helper()
	if err == nil {
		t.Fatal("an unparseable body returned no error — the failure was swallowed")
	}
	msg := err.Error()
	if !strings.Contains(msg, wantText) {
		t.Errorf("the error no longer names the route (want %q):\n  %q", wantText, msg)
	}
	if strings.Contains(msg, bodyEchoSentinel) {
		t.Errorf("the error text echoes the unparsed body:\n  %q", msg)
	}
	for i, r := range msg {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			t.Errorf("control/format rune %U at byte %d of the error, so it can rewrite the terminal:\n  %q", r, i, msg)
			break
		}
	}
	if len(msg) > 400 {
		t.Errorf("the error is %d bytes; it must be bounded, not sized by the body:\n  %.200q…", len(msg), msg)
	}
}

// TestUnexpectedBodyIsNeverEchoed is civitai/cli#620's table: every genapi site
// that reports a body or payload it could not parse, driven with a body that
// carries a sentinel, the measured ANSI forgery, and (one row) a 200 KiB body.
//
// The unwrapTRPC, whatIfFromGraph-payload and generateFromGraph-payload rows are
// the regressions: those sites interpolated string(raw)/string(payload). The
// getWorkflow, queryGeneratedImages and two blob rows were already routed
// through redact.CredentialBodyError and are INVARIANT GUARDS here, kept so the
// table names the whole class.
func TestUnexpectedBodyIsNeverEchoed(t *testing.T) {
	envelope := func(payload string) string { return `{"result":{"data":{"json":` + payload + `}}}` }
	huge := "not json " + bodyEchoSentinel + strings.Repeat("A", 200<<10)
	forgedJSON, err := json.Marshal(bodyEchoSentinel + forgedBanner) // a JSON string carrying the forgery
	if err != nil {
		t.Fatal(err)
	}
	jsonForged := string(forgedJSON)

	whatIf := func(c *Client) error {
		_, _, err := c.WhatIfFromGraph(context.Background(), Graph{Workflow: "txt2img"})
		return err
	}
	submit := func(c *Client) error {
		_, _, _, err := c.GenerateFromGraph(context.Background(), Graph{Workflow: "txt2img"}, SubmitOptions{})
		return err
	}
	getWF := func(c *Client) error { _, _, err := c.GetWorkflow(context.Background(), "wf_1"); return err }
	query := func(c *Client) error {
		_, _, err := c.QueryWorkflows(context.Background(), ListOptions{Take: 5})
		return err
	}

	cases := []struct {
		name, body, wantText string
		call                 func(c *Client) error
		envelopeOK           bool // the body is a well-formed envelope, so the PAYLOAD site is the one reached
	}{
		{"unwrapTRPC: a 200 that is not JSON, carrying the ANSI forgery", "not json " + bodyEchoSentinel + forgedBanner,
			"unexpected orchestrator.whatIfFromGraph response", whatIf, false},
		{"unwrapTRPC: a 200 whose payload is null beside a credential",
			`{"result":{"data":{"json":null}},"token":` + jsonForged + `}`,
			"unexpected orchestrator.getWorkflow response", getWF, false},
		{"unwrapTRPC: a 200 with no result envelope", `{"data":` + jsonForged + `}`,
			"unexpected orchestrator.queryGeneratedImages response", query, false},
		{"unwrapTRPC: a 200 KiB body that is not JSON", huge,
			"unexpected orchestrator.generateFromGraph response", submit, false},
		{"whatIfFromGraph: a payload that does not decode", envelope(`{"ready":` + jsonForged + `}`),
			"unexpected orchestrator.whatIfFromGraph payload", whatIf, true},
		{"generateFromGraph: a payload that does not decode", envelope(`{"id":7,"status":` + jsonForged + `}`),
			"unexpected orchestrator.generateFromGraph payload", submit, true},
		{"getWorkflow (invariant): a payload that does not decode", envelope(`{"id":7,"status":` + jsonForged + `}`),
			"unexpected orchestrator.getWorkflow payload", getWF, true},
		{"queryGeneratedImages (invariant): a payload that does not decode", envelope(`{"items":[{"id":7,"status":` + jsonForged + `}]}`),
			"unexpected orchestrator.queryGeneratedImages payload", query, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.body, bodyEchoSentinel) {
				t.Fatal("PREMISE BROKEN: the fixture body carries no sentinel")
			}
			if tc.envelopeOK {
				if _, err := unwrapTRPC("premise", []byte(tc.body)); err != nil {
					t.Fatalf("PREMISE BROKEN: the envelope does not unwrap, so the payload site is never reached: %v", err)
				}
			}
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			assertBodyNotEchoed(t, tc.call(c), tc.wantText)
		})
	}

	blobRows := []struct {
		name, presign, upload, wantText string
	}{
		{"getConsumerBlobUploadUrl (invariant): a 200 that is not JSON", "not json " + bodyEchoSentinel + forgedBanner, "",
			"unexpected getConsumerBlobUploadUrl response"},
		{"image-upload (invariant): a 200 that is not JSON", "", "not json " + bodyEchoSentinel + forgedBanner,
			"unexpected image-upload response"},
	}
	for _, tc := range blobRows {
		t.Run(tc.name, func(t *testing.T) {
			h := newBlobHarness(t)
			if tc.presign != "" {
				h.presignBody = tc.presign
			}
			if tc.upload != "" {
				h.uploadBody2 = tc.upload
			}
			_, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
			assertBodyNotEchoed(t, err, tc.wantText)
		})
	}
}

// TestUnwrapTRPCErrorText pins the whole text unwrapTRPC reports, so the
// withholding cannot be met by dropping the diagnosis. Each fixture's length is
// distinct and is not a constant the code names.
func TestUnwrapTRPCErrorText(t *testing.T) {
	cases := []struct{ body, want string }{
		{"<html>gateway " + bodyEchoSentinel + "</html>",
			"unexpected orchestrator.whatIfFromGraph response (48 bytes, not shown because it may carry credentials): invalid character '<' looking for beginning of value"},
		{`{"result":{"data":{"json":null}},"k":"` + bodyEchoSentinel + `"}`,
			"unexpected orchestrator.whatIfFromGraph response (67 bytes, not shown because it may carry credentials): no result.data.json payload"},
		{`{"result":{}}`,
			"unexpected orchestrator.whatIfFromGraph response (13 bytes, not shown because it may carry credentials): no result.data.json payload"},
	}
	for _, tc := range cases {
		_, err := unwrapTRPC("orchestrator.whatIfFromGraph", []byte(tc.body))
		if err == nil || err.Error() != tc.want {
			t.Errorf("unwrapTRPC(%q):\n  got  %v\n  want %s", tc.body, err, tc.want)
		}
	}
}

// TestUnexpectedBodySitesAreLedgered is the enumeration control for
// civitai/cli#620: every call in internal/genapi's non-test sources whose
// arguments hold a string literal opening "unexpected " is one of the ledgered
// sites below AND is a redact.CredentialBodyError call. It fails when a site is
// added (decide how it reports the body, then ledger it), when one is removed,
// and when one is reached by any other function — fmt.Errorf with string(raw)
// is how the three regressions above were written.
func TestUnexpectedBodySitesAreLedgered(t *testing.T) {
	want := []string{
		"blobs.go: unexpected getConsumerBlobUploadUrl response",
		"blobs.go: unexpected image-upload response",
		"generate.go: unexpected ", // unwrapTRPC: "unexpected " + proc + " response"
		"generate.go: unexpected orchestrator.generateFromGraph payload",
		"generate.go: unexpected orchestrator.whatIfFromGraph payload",
		"status.go: unexpected orchestrator.getWorkflow payload",
		"workflows.go: unexpected orchestrator.queryGeneratedImages payload",
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var got []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			for _, arg := range call.Args {
				ast.Inspect(arg, func(m ast.Node) bool {
					lit, ok := m.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					s, err := strconv.Unquote(lit.Value)
					if err != nil || !strings.HasPrefix(s, "unexpected ") {
						return true
					}
					got = append(got, path+": "+s)
					callee := ""
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if x, ok := sel.X.(*ast.Ident); ok {
							callee = x.Name + "." + sel.Sel.Name
						}
					}
					if callee != "redact.CredentialBodyError" {
						t.Errorf("%s: %q is reported through %q, not redact.CredentialBodyError — an unparsed body must be withheld, not interpolated",
							fset.Position(lit.Pos()), s, callee)
					}
					return true
				})
			}
			return true
		})
	}
	sort.Strings(got)
	// Positive control: the scan must find the sites, or an empty set would pass
	// the callee check for the wrong reason.
	if len(got) == 0 {
		t.Fatal("the scan found no \"unexpected …\" literals — the scan is broken, not the code")
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the \"unexpected …\" sites changed:\n got:\n  %s\n want:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
