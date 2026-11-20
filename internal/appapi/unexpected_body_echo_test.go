package appapi

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// bodyEchoSentinel stands in for a credential in a body the CLI could not use.
const bodyEchoSentinel = "SENTINEL-UNPARSED-BODY-91c2"

// forgedBanner is the civitai/cli#620 payload: a newline, then cursor-up +
// erase-line, then a counterfeit status line with a tab in it.
const forgedBanner = "\n\x1b[1A\x1b[2K✓ Submitted — publish request pr_evil\tCharged 0 Buzz"

// assertBodyNotEchoed is the shared check: the failure is still REPORTED and
// names its route, the error carries no byte of the body's sentinel and no
// control or format character (so no ESC, newline or tab), and its size does not
// grow with the body's.
func assertBodyNotEchoed(t *testing.T, err error, wantText string) {
	t.Helper()
	if err == nil {
		t.Fatal("an unusable body returned no error — the failure was swallowed")
	}
	msg := err.Error()
	if !strings.Contains(msg, wantText) {
		t.Errorf("the error no longer names the route (want %q):\n  %q", wantText, msg)
	}
	if strings.Contains(msg, bodyEchoSentinel) {
		t.Errorf("the error text echoes the response body:\n  %.300q", msg)
	}
	for i, r := range msg {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			t.Errorf("control/format rune %U at byte %d of the error, so it can rewrite the terminal:\n  %.300q", r, i, msg)
			break
		}
	}
	if len(msg) > 400 {
		t.Errorf("the error is %d bytes; it must be bounded, not sized by the body:\n  %.200q…", len(msg), msg)
	}
}

// TestAppapiUnexpectedBodyIsNeverEchoed drives every internal/appapi site that
// reported a 200 body (or tRPC payload) it could not use by interpolating that
// body, plus the presigned-PUT failure that interpolated the storage server's
// error body. Each row's body carries a sentinel and the #620 ANSI forgery; one
// row is 200 KiB.
func TestAppapiUnexpectedBodyIsNeverEchoed(t *testing.T) {
	forged := "not json " + bodyEchoSentinel + forgedBanner
	huge := "not json " + bodyEchoSentinel + strings.Repeat("A", 200<<10)
	q, err := json.Marshal(bodyEchoSentinel + forgedBanner) // a JSON string carrying the forgery
	if err != nil {
		t.Fatal(err)
	}
	jsonForged := string(q)
	envelope := func(payload string) string { return `{"result":{"data":{"json":` + payload + `}}}` }
	ctx := context.Background()

	cases := []struct {
		name, body, wantText string
		call                 func(c *Client) error
	}{
		{"listing tRPC (decodeTRPCData): a 200 that is not JSON", forged,
			"unexpected appListings.listMine response",
			func(c *Client) error { _, err := c.ListMyListings(ctx); return err }},
		{"listing tRPC (decodeTRPCData): a payload that does not decode", envelope(jsonForged),
			"unexpected appListings.listMine payload",
			func(c *Client) error { _, err := c.ListMyListings(ctx); return err }},
		{"submit: a 200 that is not JSON", forged,
			"unexpected response (status 200)",
			func(c *Client) error {
				_, err := c.SubmitVersion(ctx, []byte("z"), "my-block", "0.1.0", Provenance{})
				return err
			}},
		{"buzz.getBuzzAccount: a null payload beside a credential",
			`{"result":{"data":{"json":null}},"token":` + jsonForged + `}`,
			"unexpected buzz.getBuzzAccount response",
			func(c *Client) error { _, err := c.GetBuzzAccount(ctx); return err }},
		{"submissions list: a 200 KiB body that is not JSON", huge,
			"unexpected /api/v1/blocks/submissions response",
			func(c *Client) error { _, err := c.ListSubmissions(ctx, "my-block"); return err }},
		{"submissions rows: a 200 that is not JSON", forged,
			"unexpected /api/v1/blocks/submissions response",
			func(c *Client) error { _, _, err := c.GetSubmissionRows(ctx, "", "my-block"); return err }},
		{"blocks.startDevTunnel: no result envelope", `{"data":` + jsonForged + `}`,
			"unexpected blocks.startDevTunnel response",
			func(c *Client) error {
				_, err := c.StartDevTunnel(ctx, "my-block", "ssh-ed25519 AAAA", nil, "")
				return err
			}},
		{"blocks.startDevTunnel: a session missing its host, beside a credential",
			envelope(`{"sessionId":"s1","tunnelToken":` + jsonForged + `}`),
			"blocks.startDevTunnel response missing host/sessionId",
			func(c *Client) error {
				_, err := c.StartDevTunnel(ctx, "my-block", "ssh-ed25519 AAAA", nil, "")
				return err
			}},
		{"blocks.stopDevTunnel: a 200 that is not JSON", forged,
			"unexpected blocks.stopDevTunnel response",
			func(c *Client) error { _, err := c.StopDevTunnel(ctx, "s1", ""); return err }},
		{"blocks.getMyAppAnalytics: a null payload beside a credential",
			`{"result":{"data":{"json":null}},"k":` + jsonForged + `}`,
			"unexpected blocks.getMyAppAnalytics response",
			func(c *Client) error { _, _, err := c.GetMyAppAnalytics(ctx, "ab_1", "", ""); return err }},
		{"blocks.getMyAppAnalytics: a payload that does not decode", envelope(jsonForged),
			"unexpected blocks.getMyAppAnalytics payload",
			func(c *Client) error { _, _, err := c.GetMyAppAnalytics(ctx, "ab_1", "", ""); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.body, bodyEchoSentinel) {
				t.Fatal("PREMISE BROKEN: the fixture body carries no sentinel")
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			assertBodyNotEchoed(t, tc.call(New(srv.URL, "tok", "")), tc.wantText)
		})
	}

	t.Run("presigned PUT: a storage error body carrying the signing material", func(t *testing.T) {
		body := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
			`<Error><Code>SignatureDoesNotMatch</Code>` +
			`<Message>The request signature we calculated does not match the signature you provided.</Message>` +
			`<AWSAccessKeyId>` + bodyEchoSentinel + `</AWSAccessKeyId>` +
			// A real StringToSign is multi-line; the newlines and tab must not
			// reach the error either.
			"<StringToSign>AWS4-HMAC-SHA256\n20261120T000000Z\n\tus-east-1/s3/aws4_request</StringToSign></Error>"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()
		err := New(srv.URL, "tok", "").putPresigned(ctx, srv.URL+"/bucket/key?X-Amz-Signature=x", []byte("PNG"))
		assertBodyNotEchoed(t, err, "image upload PUT failed (HTTP 403)")
		// The storage error CODE is what diagnoses a presigned-PUT failure (an
		// expired URL, a skewed clock), so withholding the body must keep it.
		if err != nil && !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
			t.Errorf("the storage error code was dropped with the body:\n  %q", err.Error())
		}
	})
}

// TestPresignedPutErrorText pins the whole text a presigned-PUT failure
// reports, so withholding the body cannot be met by dropping the diagnosis.
func TestPresignedPutErrorText(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusForbidden,
			`<Error><Code>RequestTimeTooSkewed</Code><Message>The difference between the request time and the current time is too large.</Message><RequestTime>x</RequestTime></Error>`,
			"image upload PUT failed (HTTP 403): RequestTimeTooSkewed: The difference between the request time and the current time is too large."},
		{http.StatusBadGateway, "<html>bad gateway " + bodyEchoSentinel + "</html>",
			"image upload PUT failed (HTTP 502): 52-byte response body not shown because it may carry credentials"},
		{http.StatusInternalServerError, "",
			"image upload PUT failed (HTTP 500): empty response body"},
		// The Message is refused by ServerText (it holds a signed URL), so only
		// the Code is shown.
		{http.StatusForbidden,
			`<Error><Code>AccessDenied</Code><Message>denied for https://bucket.example/k?X-Amz-Signature=` + bodyEchoSentinel + `</Message></Error>`,
			"image upload PUT failed (HTTP 403): AccessDenied"},
		// The Code itself is refused, so nothing from the body is shown.
		{http.StatusForbidden, `<Error><Code>see https://x.example/k</Code><Message>m</Message></Error>`,
			"image upload PUT failed (HTTP 403): 71-byte response body not shown because it may carry credentials"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		err := New(srv.URL, "tok", "").putPresigned(context.Background(), srv.URL+"/k", []byte("PNG"))
		srv.Close()
		if err == nil || err.Error() != tc.want {
			t.Errorf("HTTP %d %q:\n  got  %v\n  want %s", tc.status, tc.body, err, tc.want)
		}
	}
}

// rawBodyEchoLedger is every error-constructing call in internal/appapi's
// non-test sources that is handed a byte slice, as "file: func: format: arg",
// where arg is the argument expression holding the byte slice. The argument is
// part of the key so that putting string(raw) back where a sanitiser call was
// changes the entry even though the format string did not change.
//
// No entry shows a response body:
//   - WhoAmI hands the /me body to describeMeBody, which reports the keys present
//     and the offending JSON type, never a value (the body carries the email);
//   - decodeWebPInfo quotes a 4-byte chunk tag of a LOCAL image file with %q, so
//     it is bounded and escaped;
//   - putPresigned hands the storage error body to storageErrorMessage, and the
//     three OAuth failures hand theirs to oauthMsg: each shows only a code and
//     message that redact.ServerText accepts, else the body's size.
var rawBodyEchoLedger = []string{
	`appblocks.go: WhoAmI: "unexpected /api/v1/me response (%s) — the server sent a shape this CLI cannot read; try ` +
		"`civitai upgrade`" + `, and report it at https://github.com/civitai/cli/issues": describeMeBody(raw)`,
	`imageinfo.go: decodeWebPInfo: "unrecognized webp chunk %q": string(data[12:16])`,
	`listing.go: putPresigned: "image upload PUT failed (HTTP %d): %s": storageErrorMessage(raw)`,
	`oauth.go: Refresh: "token refresh failed (status %d): %s": oauthMsg(raw)`,
	`oauth.go: StartDevice: "device init failed (status %d): %s": oauthMsg(raw)`,
	`oauth.go: pollOnce: "device-token poll failed (status %d): %s": oauthMsg(raw)`,
}

// TestRawBodyEchoSitesAreLedgered is the enumeration control for the class
// TestAppapiUnexpectedBodyIsNeverEchoed samples: it type-checks the package and
// lists every call written directly as fmt.Errorf / fmt.Sprintf / errors.New
// inside a func declaration, any of whose argument
// expressions holds a value of byte-slice type ([]byte, json.RawMessage, …) —
// whatever the variable is called and however it is wrapped (string(raw),
// strings.TrimSpace(string(raw)), a bare raw for %s). Two forms are not counted:
// the operand of len or cap, and an argument to a redact.* function, which is
// the sanctioned way to describe a body. The list must equal rawBodyEchoLedger
// exactly, so the test fails when a site is ADDED (route it through
// redact.CredentialBodyError, or ledger it with a reason) and when one is
// REMOVED (drop the entry).
//
// It does not see a function-valued alias (errf := fmt.Errorf), a body inside a
// package-level func literal, or an error struct literal holding the body.
//
// 🔴 IT FOLLOWS NO DATA FLOW. A body first copied into a string variable
// (msg := string(raw); fmt.Errorf("%s", msg)) is not seen, which is how
// serverMessage's old raw-body fallback reached serverError's messages unseen
// (it now goes through redact.CredentialRouteMessage; see
// TestServerMessageFallbackNeverEchoesTheBody).
func TestRawBodyEchoSitesAreLedgered(t *testing.T) {
	got := rawBodyEchoSites(t, ".")
	// Positive control: the scan must be able to see a site, or an empty set
	// would pass for the wrong reason.
	if len(got) == 0 {
		t.Fatal("the scan found no byte-slice error arguments at all — the scan is broken, not the code")
	}
	want := append([]string(nil), rawBodyEchoLedger...)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the set of error calls handed a byte slice changed — an HTTP body in an error must go through redact.CredentialBodyError:\n got:\n  %s\n want:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// rawBodyEchoSites type-checks the non-test Go files in dir and returns the
// sorted "file: func: format: arg" of every error-constructing call handed a byte
// slice (see TestRawBodyEchoSitesAreLedgered).
func rawBodyEchoSites(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	if _, err := conf.Check("appapi", fset, files, info); err != nil {
		t.Fatalf("type-checking the package for the scan: %v", err)
	}

	isByteSlice := func(e ast.Expr) bool {
		tv, ok := info.Types[e]
		if !ok || tv.Type == nil {
			return false
		}
		s, ok := tv.Type.Underlying().(*types.Slice)
		if !ok {
			return false
		}
		b, ok := s.Elem().Underlying().(*types.Basic)
		return ok && b.Kind() == types.Byte
	}
	calleeOf := func(call *ast.CallExpr) (pkg, name string) {
		var id *ast.Ident
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			id = fn
		case *ast.SelectorExpr:
			id = fn.Sel
		default:
			return "", ""
		}
		switch obj := info.Uses[id].(type) {
		case *types.Func:
			if obj.Pkg() != nil {
				return obj.Pkg().Path(), obj.Name()
			}
		case *types.Builtin:
			return "", obj.Name()
		}
		return "", ""
	}
	sinks := map[string]bool{"fmt.Errorf": true, "fmt.Sprintf": true, "errors.New": true}

	var got []string
	for _, f := range files {
		file := filepath.Base(fset.Position(f.Pos()).Filename)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				pkg, name := calleeOf(call)
				if !sinks[pkg+"."+name] {
					return true
				}
				var flagged []string
				for _, arg := range call.Args {
					found := false
					ast.Inspect(arg, func(m ast.Node) bool {
						if found {
							return false
						}
						if inner, ok := m.(*ast.CallExpr); ok {
							ipkg, iname := calleeOf(inner)
							if ipkg == "" && (iname == "len" || iname == "cap") {
								return false
							}
							if strings.HasSuffix(ipkg, "/internal/redact") {
								return false
							}
						}
						if e, ok := m.(ast.Expr); ok && isByteSlice(e) {
							found = true
							return false
						}
						return true
					})
					if found {
						flagged = append(flagged, types.ExprString(arg))
					}
				}
				if len(flagged) > 0 {
					format := "<non-constant>"
					if len(call.Args) > 0 {
						if tv := info.Types[call.Args[0]]; tv.Value != nil && tv.Value.Kind() == constant.String {
							format = strconv.Quote(constant.StringVal(tv.Value))
						}
					}
					got = append(got, file+": "+fd.Name.Name+": "+format+": "+strings.Join(flagged, ", "))
				}
				return true
			})
		}
	}
	sort.Strings(got)
	return got
}
