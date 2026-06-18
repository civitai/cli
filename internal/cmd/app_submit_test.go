package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// fakeSubmitter records the bytes it was handed.
type fakeSubmitter struct {
	got    []byte
	result *api.SubmitResult
	err    error
}

func (f *fakeSubmitter) SubmitVersion(_ context.Context, zip []byte) (*api.SubmitResult, error) {
	f.got = zip
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestDoUploadHandsBytesToSubmitter(t *testing.T) {
	fs := &fakeSubmitter{result: &api.SubmitResult{
		PublishRequestID: "pr_9", Slug: "demo", Version: "0.1.0", Status: "pending",
	}}
	var out bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)

	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0", Name: "Demo"}
	if err := doUpload(c, fs, []byte("ZIPBYTES"), m); err != nil {
		t.Fatalf("doUpload: %v", err)
	}
	if string(fs.got) != "ZIPBYTES" {
		t.Errorf("submitter got %q, want ZIPBYTES", fs.got)
	}
	if !strings.Contains(out.String(), "pr_9") || !strings.Contains(out.String(), "pending") {
		t.Errorf("output should report the publish request + status, got: %s", out.String())
	}
}
