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

// `civitai app submit` reports the store listing's readiness the server returns
// with the submit (civitai/civitai#7311): when a moderator cannot approve the
// submission until the listing has an icon and a cover, the server's own next
// step is printed prominently, right under the success line; any other next step
// is printed as a plain note; and the listing problems are listed by code and
// label. An older server sends no `listing` and the output is unchanged.

const (
	readinessBlockedPrefix  = "Approval blocked: "
	readinessNotePrefix     = "Listing: "
	readinessProblemsHeader = "Listing problems reported by the server:"
	readinessBlockedStep    = "Add an icon and a cover on your listing's Media tab before a moderator can approve it."
)

// submitOutput runs doUpload with a fake submitter returning res and returns
// stdout.
func submitOutput(t *testing.T, res *appapi.SubmitResult) string {
	t.Helper()
	var out bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)
	c.SetErr(&bytes.Buffer{})
	m := &manifest.Manifest{BlockID: "demo", Version: "0.2.0", Name: "Demo"}
	if err := doUpload(c, &fakeSubmitter{result: res}, []byte("ZIP"), m, "https://civitai.com/", appapi.Provenance{}); err != nil {
		t.Fatalf("doUpload: %v", err)
	}
	return out.String()
}

func readinessResult(l *appapi.SubmitListingReadiness) *appapi.SubmitResult {
	return &appapi.SubmitResult{
		PublishRequestID: "pr_7", Slug: "demo", Version: "0.2.0", Status: "pending", Listing: l,
	}
}

func TestSubmitPrintsApprovalBlockedNextStepProminently(t *testing.T) {
	s := submitOutput(t, readinessResult(&appapi.SubmitListingReadiness{
		ApprovalBlocked: true,
		Missing:         []string{"icon", "cover"},
		NextStep:        readinessBlockedStep,
		Problems: []appapi.SubmitListingProblem{
			{Code: "missing-icon", Label: "Missing icon (required before publishing)", Severity: "blocking"},
			{Code: "missing-cover", Label: "Missing cover image (required before publishing)", Severity: "blocking"},
		},
	}))

	blocked := strings.Index(s, readinessBlockedPrefix+readinessBlockedStep)
	if blocked < 0 {
		t.Fatalf("output lacks %q, got:\n%s", readinessBlockedPrefix+readinessBlockedStep, s)
	}
	// Prominent = before the generic "What's next" block, not buried after it.
	if next := strings.Index(s, "What's next"); next < 0 || blocked > next {
		t.Errorf("the blocked next step (at %d) must print before \"What's next\" (at %d), got:\n%s", blocked, next, s)
	}
	// It is a warning line, not a plain note.
	if strings.Contains(s, readinessNotePrefix+readinessBlockedStep) {
		t.Errorf("a blocked next step must not ALSO print as a plain note, got:\n%s", s)
	}
	for _, want := range []string{
		readinessProblemsHeader,
		"missing-icon  Missing icon (required before publishing)",
		"missing-cover  Missing cover image (required before publishing)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q, got:\n%s", want, s)
		}
	}
}

// The server also sends approvalBlocked=false with a next step (an app with no
// store listing row yet). That step is printed, as a plain note, never as the
// approval-blocked warning.
func TestSubmitPrintsUnblockedNextStepAsPlainNote(t *testing.T) {
	const step = "Create your store listing so it can be reviewed with this version."
	s := submitOutput(t, readinessResult(&appapi.SubmitListingReadiness{
		ApprovalBlocked: false,
		NextStep:        step,
	}))
	if !strings.Contains(s, readinessNotePrefix+step) {
		t.Errorf("output lacks the plain note %q, got:\n%s", readinessNotePrefix+step, s)
	}
	if strings.Contains(s, readinessBlockedPrefix) {
		t.Errorf("approvalBlocked=false must not print %q, got:\n%s", readinessBlockedPrefix, s)
	}
	if strings.Contains(s, readinessProblemsHeader) {
		t.Errorf("no problems were sent, so no problems header, got:\n%s", s)
	}
}

func TestSubmitPrintsProblemsWithoutNextStep(t *testing.T) {
	s := submitOutput(t, readinessResult(&appapi.SubmitListingReadiness{
		Problems: []appapi.SubmitListingProblem{
			{Code: "no-screenshots", Label: "No screenshots (recommended)", Severity: "advisory"},
		},
	}))
	if !strings.Contains(s, readinessProblemsHeader) || !strings.Contains(s, "no-screenshots  No screenshots (recommended)") {
		t.Errorf("output lacks the problems list, got:\n%s", s)
	}
	for _, bad := range []string{readinessBlockedPrefix, readinessNotePrefix} {
		if strings.Contains(s, bad) {
			t.Errorf("no next step was sent, so %q must not print, got:\n%s", bad, s)
		}
	}
}

// An older server (no `listing`) or one that could not read it (`null`) leaves
// Listing nil: none of the readiness lines appear and the submit still succeeds.
// TestSubmitPrintsApprovalBlockedNextStepProminently is this case's positive
// control — the same harness DOES print these strings when a listing arrives.
func TestSubmitWithoutListingPrintsNoReadiness(t *testing.T) {
	s := submitOutput(t, readinessResult(nil))
	for _, bad := range []string{readinessBlockedPrefix, readinessNotePrefix, readinessProblemsHeader} {
		if strings.Contains(s, bad) {
			t.Errorf("nil Listing printed %q, got:\n%s", bad, s)
		}
	}
	if !strings.Contains(s, "pr_7") {
		t.Errorf("success output lost the publish request id, got:\n%s", s)
	}
}

// TestSubmitListingReadinessCannotForgeALine: every readiness field is SERVER
// text, so it goes through the CLI's terminal sanitiser. An escape sequence or a
// newline in nextStep / a problem label must not reach the terminal as one, and
// a newline must not start a forged line of its own.
func TestSubmitListingReadinessCannotForgeALine(t *testing.T) {
	const forged = "✓ Approved and live"
	s := submitOutput(t, readinessResult(&appapi.SubmitListingReadiness{
		ApprovalBlocked: true,
		NextStep:        "Add art\x1b[2J\n" + forged,
		Problems: []appapi.SubmitListingProblem{
			{Code: "missing-icon\x1b[31m", Label: "Missing icon\r\n" + forged, Severity: "blocking\x1b[0m"},
		},
	}))
	if strings.Contains(s, "\x1b") {
		t.Errorf("server text put an ESC byte on the terminal, got %q", s)
	}
	if strings.Contains(s, "\r") {
		t.Errorf("server text put a CR on the terminal, got %q", s)
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), forged) {
			t.Errorf("server text forged a line of its own: %q\nfull output:\n%s", line, s)
		}
	}
	// Positive control: the text itself still arrives, sanitised, on its line.
	if !strings.Contains(s, readinessBlockedPrefix+"Add art") {
		t.Errorf("the sanitised next step did not print, got:\n%s", s)
	}
}

// The seam: the REAL appapi client decoding a real response body, handed to the
// real doUpload. Each half is tested alone above; this pins that the field the
// client decodes is the one the command prints.
func TestSubmitListingReadinessEndToEndThroughClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"publishRequestId":"pr_e2e","slug":"demo","version":"0.2.0","status":"pending",
			"listing":{"approvalBlocked":true,"missing":["cover"],
			"nextStep":"Add a cover on your listing's Media tab before a moderator can approve it.",
			"problems":[{"code":"missing-cover","label":"Missing cover image (required before publishing)","severity":"blocking"}]}}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)
	c.SetErr(&bytes.Buffer{})
	m := &manifest.Manifest{BlockID: "demo", Version: "0.2.0", Name: "Demo"}
	client := appapi.New(srv.URL, "tok", "")
	if err := doUpload(c, client, []byte("ZIP"), m, srv.URL, appapi.Provenance{}); err != nil {
		t.Fatalf("doUpload: %v", err)
	}
	s := out.String()
	for _, want := range []string{
		"pr_e2e",
		readinessBlockedPrefix + "Add a cover on your listing's Media tab before a moderator can approve it.",
		"missing-cover  Missing cover image (required before publishing)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q, got:\n%s", want, s)
		}
	}
}

// The fixed "needs an icon AND a cover" reminder predates the server's
// readiness block. When the response carries `listing`, the server's next step
// and problems already say what the listing lacks, so printing the fixed
// reminder as well shows two similar messages; it is kept only for a server
// that sends no `listing`.
const staticFloorReminder = "your listing needs an icon AND a cover before it can publish"

func TestSubmitWithListingOmitsStaticFloorReminder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		listing *appapi.SubmitListingReadiness
	}{
		{"approval blocked", &appapi.SubmitListingReadiness{
			ApprovalBlocked: true,
			Missing:         []string{"icon", "cover"},
			NextStep:        readinessBlockedStep,
		}},
		{"nothing to report", &appapi.SubmitListingReadiness{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := submitOutput(t, readinessResult(tc.listing))
			if strings.Contains(s, staticFloorReminder) {
				t.Errorf("a response with `listing` must not also print the fixed reminder %q, got:\n%s", staticFloorReminder, s)
			}
			if !strings.Contains(s, "pr_7") {
				t.Errorf("success output lost the publish request id, got:\n%s", s)
			}
		})
	}
}

// Positive control for the test above: the same harness prints the reminder
// when the server sends no `listing`.
func TestSubmitWithoutListingKeepsStaticFloorReminder(t *testing.T) {
	s := submitOutput(t, readinessResult(nil))
	if !strings.Contains(s, staticFloorReminder) {
		t.Errorf("a response without `listing` must keep the fixed reminder %q, got:\n%s", staticFloorReminder, s)
	}
}
