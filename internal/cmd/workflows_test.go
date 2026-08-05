package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// wfGetDeps wires a scripted reply into runWorkflowsGet.
func wfGetDeps(reply string, err error) workflowsGetDeps {
	return workflowsGetDeps{getWorkflow: func(ctx context.Context, id string) (*genapi.Workflow, json.RawMessage, error) {
		if err != nil {
			return nil, nil, err
		}
		var wf genapi.Workflow
		if uerr := json.Unmarshal([]byte(reply), &wf); uerr != nil {
			return nil, nil, uerr
		}
		return &wf, json.RawMessage(reply), nil
	}}
}

const wfGetPayload = `{
  "id":"wf_123","status":"succeeded","createdAt":"2026-08-05T12:00:00Z","completedAt":"2026-08-05T12:00:30Z",
  "steps":[{"$type":"textToImage","name":"s","status":"succeeded",
    "metadata":{"params":{"seed":42},"output":{"c":{"hidden":true}}},
    "output":{"images":[
      {"id":"a","type":"image","available":true,"url":"https://blobs.example/a.jpeg","urlExpiresAt":"2026-08-05T13:00:00Z","nsfwLevel":"pg","width":512,"height":768},
      {"id":"b","type":"image","available":true,"url":"https://blobs.example/b.jpeg","blockedReason":"minor"},
      {"id":"c","type":"image","available":true,"url":"https://blobs.example/c.jpeg"}
    ]}}]}`

func TestWorkflowsGet_Found(t *testing.T) {
	c, out, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfGetPayload, nil), workflowsGetOpts{baseURL: "https://civitai.com"}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	stdout := out.String()
	for _, want := range []string{"wf_123", "succeeded", "512x768", "seed:    42", "https://blobs.example/a.jpeg", "2026-08-05T13:00:00Z"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	// One deliverable, two excluded — and the excluded ones must be REPORTED,
	// not silently dropped.
	if !strings.Contains(stdout, "Outputs (1 deliverable, 2 excluded)") {
		t.Errorf("the deliverable/excluded counts are missing:\n%s", stdout)
	}
	stderr := errb.String()
	for _, want := range []string{"moderation", "minor", "hidden", "expire"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	// A blocked/hidden output's URL must not be presented as a result.
	if strings.Contains(stdout, "blobs.example/b.jpeg") || strings.Contains(stdout, "blobs.example/c.jpeg") {
		t.Errorf("an excluded output's URL was listed as a result:\n%s", stdout)
	}
}

func TestWorkflowsGet_NotFound(t *testing.T) {
	apiErr := apiErrorWithStatus(t, http.StatusNotFound)
	c, out, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps("", apiErr), workflowsGetOpts{}, "wf_missing")
	if err == nil {
		t.Fatal("a 404 must return an error")
	}
	// The exit code is pinned by errors.Is, never by the message text.
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be printed on stdout for a 404: %q", out.String())
	}
}

// A 403 that is really a muted account must classify as ErrAccountRestricted, not
// as "log in again" — the same discrimination `generate` makes, via the same
// classifier.
func TestWorkflowsGet_RestrictedAccountIsNotAnAuthError(t *testing.T) {
	// Build the error from a REAL 403 reply carrying the muted-account wording,
	// so the classifier is exercised against the same *genapi.APIError
	// production produces.
	srv := trpcErrServer(t, http.StatusForbidden,
		"You cannot perform this action because your account has been restricted", "FORBIDDEN")
	_, _, apiErr := genapi.New(srv.URL, "tok").GetWorkflow(context.Background(), "wf_1")
	if apiErr == nil {
		t.Fatal("fixture: expected a 403 error")
	}

	c, _, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps("", apiErr), workflowsGetOpts{}, "wf_1")
	if !errors.Is(err, ErrAccountRestricted) {
		t.Fatalf("want ErrAccountRestricted, got %v", err)
	}
	if errors.Is(err, civitai.ErrUnauthorized) {
		t.Error("a restricted account must NOT read as an auth failure (exit 3) — `civitai login` will not help")
	}
}

func TestWorkflowsGet_JSONIsRawPassthrough(t *testing.T) {
	c, out, _ := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(wfGetPayload, nil), workflowsGetOpts{jsonOut: true}, "wf_123"); err != nil {
		t.Fatalf("--json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("--json stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if decoded["id"] != "wf_123" {
		t.Errorf("--json id = %v", decoded["id"])
	}
	// Raw passthrough: a field the CLI's struct never models must survive, so a
	// script can branch on the server payload rather than on this CLI's view.
	steps, ok := decoded["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("--json lost the steps array: %v", decoded["steps"])
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("--json must emit no ANSI styling")
	}
}

func TestWorkflowsGet_EmptyIDIsAUsageError(t *testing.T) {
	c, _, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps(wfGetPayload, nil), workflowsGetOpts{}, "   ")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage (exit 2), got %v", err)
	}
}

// An unfinished workflow says so instead of rendering "0 outputs" as a result.
func TestWorkflowsGet_UnfinishedWorkflowSaysSo(t *testing.T) {
	c, out, errb := genCmd("")
	payload := `{"id":"wf_9","status":"processing","createdAt":"2026-08-05T12:00:00Z","steps":[]}`
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{}, "wf_9"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if !strings.Contains(out.String(), "not finished") {
		t.Errorf("stdout should say the workflow has not finished:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "spends nothing") {
		t.Errorf("stderr should say re-checking is free:\n%s", errb.String())
	}
}

// The command group must reject an unknown subcommand as a usage error rather
// than printing help and exiting 0 (root.go's enforceUsageExitCodes only covers
// non-runnable parents — this pins that `workflows` stays one).
func TestWorkflowsGroup_UnknownSubcommandIsAUsageError(t *testing.T) {
	_, _, err := run(t, "workflows", "frobnicate")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("unknown subcommand: want ErrUsage (exit 2), got %v", err)
	}
}
