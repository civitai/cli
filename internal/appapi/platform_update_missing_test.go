package appapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// --allow-unmerged-platform-update rides the submit body as
// `allowUnmergedPlatformUpdate: true`, and only when it was passed.
func TestSubmitSendsAllowUnmergedPlatformUpdateOnlyWhenSet(t *testing.T) {
	on := bodyOf(t, Provenance{AllowUnmergedPlatformUpdate: true})
	if got := string(on["allowUnmergedPlatformUpdate"]); got != "true" {
		t.Fatalf("allowUnmergedPlatformUpdate = %q, want true", got)
	}
	off := bodyOf(t, Provenance{})
	if raw, ok := off["allowUnmergedPlatformUpdate"]; ok {
		t.Fatalf("allowUnmergedPlatformUpdate sent without the flag: %s", raw)
	}
}

// SubmitBodySize counts the override: it is the exact size of the body sent.
func TestSubmitBodySizeCountsAllowUnmergedPlatformUpdate(t *testing.T) {
	want, _ := json.Marshal(map[string]any{
		"bundleBase64":                base64.StdEncoding.EncodeToString([]byte("ZIPDATA")),
		"allowUnmergedPlatformUpdate": true,
	})
	if got := SubmitBodySize(len("ZIPDATA"), Provenance{AllowUnmergedPlatformUpdate: true}); got != len(want) {
		t.Fatalf("SubmitBodySize = %d, the body with the override is %d bytes", got, len(want))
	}
	if SubmitBodySize(7, Provenance{AllowUnmergedPlatformUpdate: true}) == SubmitBodySize(7, Provenance{}) {
		t.Fatal("CONTROL: the override did not change the size")
	}
}

func refuseWith(t *testing.T, status int, body string) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "tok", "").SubmitVersion(context.Background(), []byte("ZIPDATA"), "my-block", "0.2.0", Provenance{})
	if err == nil {
		t.Fatal("SubmitVersion succeeded against a refusal")
	}
	return err
}

// The server's platform_update_missing refusal is recognised by its code; the
// listed updates are decoded leniently (by as an object or a string, paths as
// objects or strings), and the error keeps serverError's text and status tag.
func TestSubmitRecognisesPlatformUpdateMissing(t *testing.T) {
	body := `{"code":"platform_update_missing","message":"This bundle is missing 2 platform updates.",` +
		`"details":{"updates":[` +
		`{"id":"apu_1","reason":"SDK bump","by":{"username":"mod-jane"},"paths":[{"path":"package.json","operation":"update"}]},` +
		`{"id":"apu_2","reason":"Fix hook","by":"mod-kim","paths":["hooks/a.ts","hooks/b.ts"]}]}}`
	err := refuseWith(t, http.StatusConflict, body)
	var missing *PlatformUpdateMissingError
	if !errors.As(err, &missing) || !errors.Is(err, ErrPlatformUpdateMissing) {
		t.Fatalf("not recognised as platform_update_missing: %v", err)
	}
	var refused *SubmitRefusedError
	if !errors.As(err, &refused) || refused.Status != http.StatusConflict {
		t.Fatalf("not a SubmitRefusedError with the status: %v", err)
	}
	if got, want := err.Error(), "server returned 409: This bundle is missing 2 platform updates."; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	want := []MissingPlatformUpdate{
		{ID: "apu_1", Reason: "SDK bump", By: "mod-jane", Paths: []string{"package.json"}},
		{ID: "apu_2", Reason: "Fix hook", By: "mod-kim", Paths: []string{"hooks/a.ts", "hooks/b.ts"}},
	}
	if !reflect.DeepEqual(missing.Updates, want) {
		t.Fatalf("updates = %#v\nwant %#v", missing.Updates, want)
	}
}

// A 400 keeps its classification (exit code) through the new type.
func TestPlatformUpdateMissingKeepsTheStatusTag(t *testing.T) {
	err := refuseWith(t, http.StatusBadRequest, `{"code":"platform_update_missing","message":"missing"}`)
	if !errors.Is(err, ErrPlatformUpdateMissing) || !errors.Is(err, civitai.ErrBadRequest) {
		t.Fatalf("want ErrPlatformUpdateMissing and ErrBadRequest, got %v", err)
	}
}

// An unknown details shape still recognises the refusal and keeps the
// server's message; a different code (or none) is the ordinary refusal.
func TestPlatformUpdateMissingToleratesUnknownShapes(t *testing.T) {
	for name, body := range map[string]string{
		"no details":     `{"code":"platform_update_missing","message":"missing an update"}`,
		"details string": `{"code":"platform_update_missing","message":"missing an update","details":"see the app page"}`,
		"updates object": `{"code":"platform_update_missing","message":"missing an update","details":{"updates":{"id":"x"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := refuseWith(t, http.StatusConflict, body)
			var missing *PlatformUpdateMissingError
			if !errors.As(err, &missing) {
				t.Fatalf("not recognised: %v", err)
			}
			if missing.Updates != nil || err.Error() != "server returned 409: missing an update" {
				t.Fatalf("updates %#v, error %q", missing.Updates, err)
			}
		})
	}
	for name, body := range map[string]string{
		"other code": `{"code":"version_exists","message":"already exists"}`,
		"no code":    `{"message":"already exists"}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := refuseWith(t, http.StatusConflict, body)
			if errors.Is(err, ErrPlatformUpdateMissing) {
				t.Fatalf("recognised a refusal that is not platform_update_missing: %v", err)
			}
			if err.Error() != "server returned 409: already exists" {
				t.Fatalf("error = %q", err)
			}
		})
	}
}
