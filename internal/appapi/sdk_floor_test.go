package appapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TESTS FOR THE REAL GetSDKFloor CLIENT AND THE sdkFloorOverride CAP.

func floorClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != SDKFloorPath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("the floor read is public and must send no token, sent %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok-must-not-be-sent", "")
}

func TestGetSDKFloorParsesAValidReply(t *testing.T) {
	c := floorClient(t, http.StatusOK,
		`{"packages":[{"name":"@civitai/sdk","minimum":"0.41.7"},{"name":"@civitai/blocks-react","minimum":"0.93.2"}]}`)
	got, err := c.GetSDKFloor(context.Background())
	if err != nil {
		t.Fatalf("GetSDKFloor: %v", err)
	}
	want := []SDKFloor{{Name: "@civitai/sdk", Minimum: "0.41.7"}, {Name: "@civitai/blocks-react", Minimum: "0.93.2"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("floors = %#v, want %#v", got, want)
	}
}

// An explicit empty list is the server saying "no floor": not an error.
func TestGetSDKFloorExplicitEmptyListIsNoFloor(t *testing.T) {
	got, err := floorClient(t, http.StatusOK, `{"packages":[]}`).GetSDKFloor(context.Background())
	if err != nil {
		t.Fatalf("an explicit empty packages list is a valid reply: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("floors = %#v, want empty", got)
	}
}

// A 200 with no usable packages list must be an error, never an empty list:
// an empty list switches the guard off silently.
func TestGetSDKFloorMissingOrWrongShapePackagesIsAnError(t *testing.T) {
	for name, body := range map[string]string{
		"no packages key":    `{"floors":[{"name":"@civitai/sdk","minimum":"0.33.0"}]}`,
		"packages null":      `{"packages":null}`,
		"packages a string":  `{"packages":"@civitai/sdk"}`,
		"packages an object": `{"packages":{"name":"@civitai/sdk"}}`,
		"top level an array": `[{"name":"@civitai/sdk","minimum":"0.33.0"}]`,
		"empty object":       `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := floorClient(t, http.StatusOK, body).GetSDKFloor(context.Background())
			if err == nil {
				t.Fatalf("a reply with no packages list must be an error, got floors %#v", got)
			}
			if errors.Is(err, ErrSDKFloorNotPublished) {
				t.Errorf("a malformed reply is not a 404; got %v", err)
			}
		})
	}
}

func TestGetSDKFloor404IsNotPublished(t *testing.T) {
	_, err := floorClient(t, http.StatusNotFound, `{"message":"not found"}`).GetSDKFloor(context.Background())
	if !errors.Is(err, ErrSDKFloorNotPublished) {
		t.Fatalf("404 must be ErrSDKFloorNotPublished, got %v", err)
	}
}

func TestGetSDKFloor500IsAnError(t *testing.T) {
	_, err := floorClient(t, http.StatusInternalServerError, `oops`).GetSDKFloor(context.Background())
	if err == nil || errors.Is(err, ErrSDKFloorNotPublished) {
		t.Fatalf("500 must be a plain error, got %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("the error must name the status, got %q", err)
	}
}

// A 200 HTML page (a proxy's error page) is an error whose text does not
// carry the body.
func TestGetSDKFloorHTMLReplyIsARedactedError(t *testing.T) {
	const marker = "PROXY-SECRET-MARKER-7f3a"
	html := "<html><body>" + marker + "</body></html>"
	_, err := floorClient(t, http.StatusOK, html).GetSDKFloor(context.Background())
	if err == nil {
		t.Fatal("an HTML 200 must be an error")
	}
	if strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), "<html>") {
		t.Errorf("the error must not echo the body, got %q", err)
	}
	if !strings.Contains(err.Error(), "not shown") {
		t.Errorf("the error must say the body was withheld, got %q", err)
	}
}

// TestSubmitCapsTheSDKFloorOverrideToTheServerSchema: an 80-char pre-release,
// a 300-char package name and 11 entries — each past the server's cap
// (civitai/civitai#10593: 64 / 214 / 8) — reach the server inside it.
func TestSubmitCapsTheSDKFloorOverrideToTheServerSchema(t *testing.T) {
	long80 := "0.31.4-" + strings.Repeat("a", 73)
	if len(long80) != 80 {
		t.Fatalf("fixture length %d, want 80", len(long80))
	}
	longPkg := "@civitai/" + strings.Repeat("p", 291)
	override := []OutdatedSDK{{Package: longPkg, Installed: long80, Minimum: long80}}
	for i := 0; i < 10; i++ {
		override = append(override, OutdatedSDK{Package: "@x/p" + string(rune('a'+i)), Installed: "0.1.0", Minimum: "0.2.0"})
	}

	body := bodyOf(t, Provenance{OutdatedSDK: override})
	var sent []OutdatedSDK
	if err := json.Unmarshal(body["sdkFloorOverride"], &sent); err != nil {
		t.Fatalf("sdkFloorOverride: %v (raw %s)", err, body["sdkFloorOverride"])
	}
	if len(sent) != 8 {
		t.Errorf("sent %d entries, the server accepts at most 8", len(sent))
	}
	if want := long80[:64]; sent[0].Installed != want || sent[0].Minimum != want {
		t.Errorf("installed/minimum = %q / %q, want the first 64 chars %q", sent[0].Installed, sent[0].Minimum, want)
	}
	if want := longPkg[:214]; sent[0].Package != want {
		t.Errorf("package len %d, want the first 214 chars", len(sent[0].Package))
	}
	if sent[7].Package != "@x/pg" {
		t.Errorf("the cap must keep the FIRST 8 entries in order; entry 8 = %q", sent[7].Package)
	}
	// The caller's slice is not mutated.
	if override[0].Installed != long80 || len(override) != 11 {
		t.Error("capping must not modify the caller's override")
	}

	// SubmitBodySize must count the capped body, not the uncapped one.
	doc := map[string]any{
		"bundleBase64":     base64.StdEncoding.EncodeToString([]byte("ZIPDATA")),
		"sdkFloorOverride": sent,
	}
	want, _ := json.Marshal(doc)
	if got := SubmitBodySize(len("ZIPDATA"), Provenance{OutdatedSDK: override}); got != len(want) {
		t.Errorf("SubmitBodySize = %d, the capped body is %d bytes", got, len(want))
	}
}

// Short values pass through untouched (the cap is not a rewrite).
func TestSubmitSendsAnInCapOverrideVerbatim(t *testing.T) {
	in := []OutdatedSDK{{Package: "@civitai/sdk", Installed: "0.31.4-beta.2", Minimum: "0.33.0"}}
	body := bodyOf(t, Provenance{OutdatedSDK: in})
	var sent []OutdatedSDK
	if err := json.Unmarshal(body["sdkFloorOverride"], &sent); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sent, in) {
		t.Errorf("sent %#v, want %#v", sent, in)
	}
}

func TestTruncateBytesKeepsRuneBoundaries(t *testing.T) {
	// "é" is 2 bytes; cutting at 5 would split the third one.
	if got := truncateBytes("ééé", 5); got != "éé" {
		t.Errorf("truncateBytes = %q, want %q", got, "éé")
	}
	if got := truncateBytes("abc", 64); got != "abc" {
		t.Errorf("truncateBytes short = %q", got)
	}
}
