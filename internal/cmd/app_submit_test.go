package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// fakeSubmitter records the bytes it was handed.
type fakeSubmitter struct {
	got        []byte
	gotSlug    string
	gotVersion string
	result     *appapi.SubmitResult
	err        error
}

func (f *fakeSubmitter) SubmitVersion(_ context.Context, zip []byte, slug, version string) (*appapi.SubmitResult, error) {
	f.got = zip
	f.gotSlug = slug
	f.gotVersion = version
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestDoUploadHandsBytesToSubmitter(t *testing.T) {
	fs := &fakeSubmitter{result: &appapi.SubmitResult{
		PublishRequestID: "pr_9", Slug: "demo", Version: "0.1.0", Status: "pending",
	}}
	var out bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)

	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0", Name: "Demo"}
	if err := doUpload(c, fs, []byte("ZIPBYTES"), m, "https://civitai.com/"); err != nil {
		t.Fatalf("doUpload: %v", err)
	}
	if string(fs.got) != "ZIPBYTES" {
		t.Errorf("submitter got %q, want ZIPBYTES", fs.got)
	}
	if fs.gotSlug != "demo" || fs.gotVersion != "0.1.0" {
		t.Errorf("submitter got slug/version %q/%q, want demo/0.1.0", fs.gotSlug, fs.gotVersion)
	}
	if !strings.Contains(out.String(), "pr_9") || !strings.Contains(out.String(), "pending") {
		t.Errorf("output should report the publish request + status, got: %s", out.String())
	}
}

// TestDoUploadSuccessOutput pins the concise, actionable success output: it must
// surface the publish-request id, the per-app deploy URL, and the submissions
// link — and must NOT carry the old, unrelated dev:live-Buzz "Tip" or the
// doubled "pending — pending" phrasing.
func TestDoUploadSuccessOutput(t *testing.T) {
	fs := &fakeSubmitter{result: &appapi.SubmitResult{
		PublishRequestID: "pubreq_abc", Slug: "my-block", Version: "0.2.0", Status: "pending",
	}}
	var out bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)

	m := &manifest.Manifest{BlockID: "my-block", Version: "0.2.0", Name: "My Block"}
	// trailing slash on baseURL must be trimmed when composing the link.
	if err := doUpload(c, fs, []byte("ZIP"), m, "https://civitai.com/"); err != nil {
		t.Fatalf("doUpload: %v", err)
	}
	s := out.String()

	for _, want := range []string{
		"pubreq_abc",                              // the publish-request id
		"https://my-block.civit.ai",               // per-app deploy URL (from slug)
		"https://civitai.com/apps/my-submissions", // submissions link (no doubled slash)
		"civitai app status",                      // the CLI status command
	} {
		if !strings.Contains(s, want) {
			t.Errorf("success output missing %q, got:\n%s", want, s)
		}
	}

	for _, bad := range []string{
		"spends Buzz",       // the dropped dev:live Buzz tip
		"dev:live",          // ditto
		"pending — pending", // the old doubled phrasing
		"is now pending",    // the old redundant phrasing
	} {
		if strings.Contains(s, bad) {
			t.Errorf("success output should not contain %q, got:\n%s", bad, s)
		}
	}
}
