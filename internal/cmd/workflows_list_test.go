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

// wfListDeps wires a scripted page into runWorkflowsList and records the options
// the command derived from its flags.
func wfListDeps(reply string, err error, seen *genapi.ListOptions, calls *int) workflowsListDeps {
	return workflowsListDeps{queryWorkflows: func(ctx context.Context, opts genapi.ListOptions) (*genapi.WorkflowPage, json.RawMessage, error) {
		if calls != nil {
			*calls++
		}
		if seen != nil {
			*seen = opts
		}
		if err != nil {
			return nil, nil, err
		}
		var page genapi.WorkflowPage
		if uerr := json.Unmarshal([]byte(reply), &page); uerr != nil {
			return nil, nil, uerr
		}
		return &page, json.RawMessage(reply), nil
	}}
}

const wfListPayload = `{
  "items":[
    {"id":"wf_1","status":"succeeded","createdAt":"2026-08-05T12:00:00Z","cost":{"base":8,"total":12},
     "steps":[{"$type":"textToImage","name":"s","status":"succeeded",
       "metadata":{"output":{"c":{"hidden":true}}},
       "output":[
         {"id":"a","available":true,"url":"https://blobs.example/a.jpeg"},
         {"id":"b","available":true,"url":"https://blobs.example/b.jpeg","blockedReason":"minor"},
         {"id":"c","available":true,"url":"https://blobs.example/c.jpeg"}
       ]}]},
    {"id":"wf_2","status":"processing","createdAt":"2026-08-05T12:05:00Z","steps":[]}
  ],
  "nextCursor":"cur_2",
  "serverOnlyField":"kept"}`

func TestWorkflowsList_Populated(t *testing.T) {
	c, out, errb := genCmd("")
	if err := runWorkflowsList(c, wfListDeps(wfListPayload, nil, nil, nil), workflowsListOpts{}); err != nil {
		t.Fatalf("workflows list: %v", err)
	}
	stdout := out.String()
	for _, want := range []string{"WORKFLOW ID", "wf_1", "succeeded", "wf_2", "processing", "12", "2026-08-05T12:00:00Z"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	// 3 outputs, of which one is blocked and one hidden — the row must report
	// BOTH numbers, or a workflow whose results were all filtered out reads as a
	// normal one.
	if !strings.Contains(stdout, "1/3") {
		t.Errorf("stdout does not report deliverable/total as 1/3:\n%s", stdout)
	}
	// The cursor is DATA a pipeline consumes, so it belongs on stdout.
	if !strings.Contains(stdout, "cur_2") {
		t.Errorf("the next cursor is missing from stdout:\n%s", stdout)
	}
	if !strings.Contains(errb.String(), "--cursor") {
		t.Errorf("stderr does not explain how to page:\n%s", errb.String())
	}
}

// An empty feed is an answer, not an error — but it must name which question
// came back empty, so a mis-typed --tag does not read as "you have never
// generated anything".
func TestWorkflowsList_Empty(t *testing.T) {
	cases := []struct {
		name string
		opts workflowsListOpts
		want string
	}{
		{"no workflows at all", workflowsListOpts{}, "No workflows yet"},
		{"exhausted cursor", workflowsListOpts{cursor: "cur_9"}, "No more workflows"},
		{"unmatched tag", workflowsListOpts{tags: []string{"nope"}}, "No workflows tagged nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, out, _ := genCmd("")
			if err := runWorkflowsList(c, wfListDeps(`{"items":[]}`, nil, nil, nil), tc.opts); err != nil {
				t.Fatalf("workflows list: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tc.want)
			}
		})
	}
}

func TestWorkflowsList_JSONIsRawPassthrough(t *testing.T) {
	c, out, _ := genCmd("")
	if err := runWorkflowsList(c, wfListDeps(wfListPayload, nil, nil, nil), workflowsListOpts{jsonOut: true}); err != nil {
		t.Fatalf("--json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("--json stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if decoded["nextCursor"] != "cur_2" {
		t.Errorf("--json nextCursor = %v", decoded["nextCursor"])
	}
	// A field the CLI's structs never model must survive, so a script can branch
	// on the server payload rather than on this CLI's view of it.
	if decoded["serverOnlyField"] != "kept" {
		t.Errorf("--json dropped a field the CLI does not model: %v", decoded)
	}
	// The human table must not leak into a machine-readable stdout.
	if strings.Contains(out.String(), "WORKFLOW ID") {
		t.Errorf("--json stdout carries the human table:\n%s", out.String())
	}
}

func TestWorkflowsList_PassesPagingOptionsThrough(t *testing.T) {
	var seen genapi.ListOptions
	c, _, _ := genCmd("")
	o := workflowsListOpts{limit: 5, limitSet: true, cursor: "cur_1", tags: []string{"a", "b"}}
	if err := runWorkflowsList(c, wfListDeps(`{"items":[]}`, nil, &seen, nil), o); err != nil {
		t.Fatalf("workflows list: %v", err)
	}
	if seen.Take != 5 || seen.Cursor != "cur_1" || len(seen.Tags) != 2 {
		t.Errorf("options reaching the seam = %+v, want take 5 / cursor cur_1 / 2 tags", seen)
	}
}

func TestWorkflowsList_RejectsNonPositiveLimit(t *testing.T) {
	calls := 0
	c, _, _ := genCmd("")
	err := runWorkflowsList(c, wfListDeps(`{"items":[]}`, nil, nil, &calls), workflowsListOpts{limit: 0, limitSet: true})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage (exit 2), got %v", err)
	}
	if calls != 0 {
		t.Errorf("a rejected --limit still hit the network (%d calls)", calls)
	}
	// Positive control: the same seam IS reached for a valid limit, so the zero
	// above is a fact about the guard rather than about the harness.
	if err := runWorkflowsList(c, wfListDeps(`{"items":[]}`, nil, nil, &calls), workflowsListOpts{limit: 5, limitSet: true}); err != nil {
		t.Fatalf("control list: %v", err)
	}
	if calls != 1 {
		t.Errorf("control: seam calls = %d, want 1", calls)
	}
}

func TestWorkflowsList_ErrorIsClassified(t *testing.T) {
	apiErr := apiErrorWithStatus(t, http.StatusForbidden)
	c, out, _ := genCmd("")
	err := runWorkflowsList(c, wfListDeps("", apiErr, nil, nil), workflowsListOpts{})
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("want civitai.ErrUnauthorized (exit 3), got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be printed on stdout for a failed list: %q", out.String())
	}
}

// A restricted account must classify through the same discriminator `generate`
// uses, not as "log in again".
func TestWorkflowsList_RestrictedAccountIsNotAnAuthError(t *testing.T) {
	srv := trpcErrServer(t, http.StatusForbidden,
		"You cannot perform this action because your account has been restricted", "FORBIDDEN")
	_, _, apiErr := genapi.New(srv.URL, "tok").QueryWorkflows(context.Background(), genapi.ListOptions{})
	if apiErr == nil {
		t.Fatal("fixture: expected a 403 error")
	}
	c, _, _ := genCmd("")
	err := runWorkflowsList(c, wfListDeps("", apiErr, nil, nil), workflowsListOpts{})
	if !errors.Is(err, ErrAccountRestricted) {
		t.Fatalf("want ErrAccountRestricted, got %v", err)
	}
	if errors.Is(err, civitai.ErrUnauthorized) {
		t.Error("a restricted account must NOT read as an auth failure — `civitai login` will not help")
	}
}
