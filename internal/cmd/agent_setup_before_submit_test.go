package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/manifest"
	"github.com/spf13/cobra"
)

// THE FIRST-REVIEW CHECKLIST POINTER (civitai-app-starters#573).
//
// Human review of agent-built apps sent the same feedback back on app after app,
// and the docs site now carries it as a checklist (`/apps/guide/first-review`,
// civitai-developer-docs#179). This CLI POINTS at it from two places and carries
// none of its contents — AGENTS.md item 36's "the CLI ships the address, the docs
// repo ships the contents":
//
//   - the managed AGENTS.md block's `### Before you submit` section, and
//   - `civitai app submit`'s confirmation step (confirmSubmit).
//
// Both read firstReviewChecklistURL; the template reaches it through the
// `firstReviewURL` func, so the block has no literal of its own to drift.
//
// An inline variant that repeated four checklist items in the block was
// considered and rejected — see claudedocs/decisions/36-agents-block-per-project.md.

// wantBeforeSubmitSection is the WHOLE section, whitespace-normalised.
//
// 🔴 PINNED AS A WHOLE STRING, NOT BY KEYWORDS. The artefact is prose, so a guard
// on a word ("first-review") is walkable by rewording the sentence around it into
// one that no longer tells the reader what to do. The URL is spelled out here
// rather than built from the constant, so the test also pins the spelling.
const wantBeforeSubmitSection = "### Before you submit " +
	"Run the first-review checklist before `civitai app submit` — it is the review feedback that came " +
	"back on app after app: https://developer.civitai.com/apps/guide/first-review.md"

// blockSectionOf returns the `### <heading>` section of a rendered block, from
// its heading up to the next `### ` heading or the END marker, trimmed. The bool
// is false when the heading is absent — a distinct answer from an empty section,
// so a block that LOST the section cannot compare equal to anything.
func blockSectionOf(block, heading string) (string, bool) {
	i := strings.Index(block, "\n### "+heading+"\n")
	if i < 0 {
		return "", false
	}
	rest := block[i+1:]
	if j := strings.Index(rest, agentsEndMarker); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.Index(rest[len("### "):], "\n### "); j >= 0 {
		rest = rest[:len("### ")+j]
	}
	return strings.TrimSpace(rest), true
}

// TestTheBlockCarriesTheBeforeYouSubmitSectionInEveryShape pins the block side.
//
// 🔴 EVERY SHAPE × BOTH SDK BRANCHES. The template branches on project kind and
// on `ElementsSDK`; this section sits outside every `{{ if }}`, and a mis-nested
// edit that moved it inside one is exactly what a single-shape check passes.
// allProjectShapesForTest renders no ElementsSDK shape, so both values are set
// here explicitly.
func TestTheBlockCarriesTheBeforeYouSubmitSectionInEveryShape(t *testing.T) {
	shapes := allProjectShapesForTest()
	if len(shapes) < 3 {
		t.Fatalf("CONTROL failure, not a finding: %d shape(s) to render", len(shapes))
	}
	rendered := 0
	for _, sdk := range []bool{false, true} {
		for _, shape := range shapes {
			shape.ElementsSDK = sdk
			block, err := agentsManagedBlock(shape)
			if err != nil {
				t.Fatalf("rendering kind %q (sdk=%t): %v", shape.Kind, sdk, err)
			}
			rendered++
			got, ok := blockSectionOf(block, "Before you submit")
			if !ok {
				t.Errorf("kind %q (%d script(s), wizard=%t, sdk=%t): the managed block has no "+
					"`### Before you submit` section — the agent is never pointed at the first-review "+
					"checklist before `civitai app submit`",
					shape.Kind, len(shape.Scripts), shape.HasSetupWizard, sdk)
				continue
			}
			if n := normaliseMessage(got); n != wantBeforeSubmitSection {
				t.Errorf("kind %q (sdk=%t): the `### Before you submit` section is not what is pinned.\n"+
					"--- want ---\n%s\n--- got ---\n%s\n"+
					"Edit internal/cmd/templates/agents-app.md and wantBeforeSubmitSection together.",
					shape.Kind, sdk, wantBeforeSubmitSection, n)
			}
		}
	}
	if rendered != 2*len(shapes) {
		t.Fatalf("CONTROL failure, not a finding: rendered %d block(s), want %d", rendered, 2*len(shapes))
	}
}

// TestTheBlockAndSubmitNameOneChecklistURL is the single-source guard: the
// template carries NO literal of the address (it must go through the template
// func), and the submit pointer is built from the same constant.
func TestTheBlockAndSubmitNameOneChecklistURL(t *testing.T) {
	if !strings.Contains(wantBeforeSubmitSection, firstReviewChecklistURL) {
		t.Errorf("the pinned block section does not carry firstReviewChecklistURL (%s) — the block and "+
			"`app submit` now name different pages", firstReviewChecklistURL)
	}
	if strings.Contains(agentsAppTemplate, "/apps/guide/first-review") {
		t.Error("internal/cmd/templates/agents-app.md spells the first-review address itself; use " +
			"`{{ firstReviewURL }}` so the block and `civitai app submit` cannot drift apart")
	}
	if !strings.Contains(agentsAppTemplate, "{{ firstReviewURL }}") {
		t.Error("CONTROL failure, not a finding: the template never calls firstReviewURL, so the literal " +
			"check above is about nothing")
	}
	if !strings.HasSuffix(firstReviewPointer, firstReviewChecklistURL) {
		t.Errorf("the submit pointer %q does not end in firstReviewChecklistURL", firstReviewPointer)
	}
}

// TestBlockSectionOfCanFail is the extractor's negative and boundary control:
// absent is reported absent, a heading that merely STARTS with the name does not
// match, and the section stops at the next heading and at the END marker.
func TestBlockSectionOfCanFail(t *testing.T) {
	if _, ok := blockSectionOf("## Top\n\n### Gotchas\n\n- a\n"+agentsEndMarker, "Before you submit"); ok {
		t.Error("blockSectionOf reported a section that is not in the document")
	}
	if _, ok := blockSectionOf("x\n### Before you submit later\n", "Before you submit"); ok {
		t.Error("blockSectionOf matched a heading that only starts with the wanted name")
	}
	got, ok := blockSectionOf("x\n### Before you submit\n\nbody\n\n### Next\n\nother\n", "Before you submit")
	if !ok || got != "### Before you submit\n\nbody" {
		t.Errorf("did not stop at the next heading: ok=%t %q", ok, got)
	}
	got, ok = blockSectionOf("x\n### Before you submit\n\nbody\n"+agentsEndMarker+"\ntrailer", "Before you submit")
	if !ok || got != "### Before you submit\n\nbody" {
		t.Errorf("did not stop at the END marker: ok=%t %q", ok, got)
	}
}

// splitConfirmCmd is a command whose stdout and stderr are separate buffers —
// newConfirmCmd (app_submit_r2_test.go) merges them, which cannot tell the
// streams apart.
type splitConfirmCmd struct {
	cmd      *cobra.Command
	out, err *bytes.Buffer
}

func newSplitConfirmCmd(stdin string) splitConfirmCmd {
	c := splitConfirmCmd{cmd: &cobra.Command{}, out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	c.cmd.SetOut(c.out)
	c.cmd.SetErr(c.err)
	c.cmd.SetIn(strings.NewReader(stdin))
	return c
}

// wantFirstReviewPointer is the submit-side line, spelled out WHOLE.
const wantFirstReviewPointer = "Before you submit: run the first-review checklist — " +
	"https://developer.civitai.com/apps/guide/first-review.md"

// TestConfirmSubmitPointsAtTheFirstReviewChecklist pins the submit side, per
// branch of confirmSubmit, with stdout and stderr captured SEPARATELY — the
// claim is about which stream carries the line, so a merged buffer could not
// see it.
//
//   - interactive TTY: on stdout, with the rest of the prompt, BEFORE the
//     question it informs.
//   - --yes: on stderr only, so a script's captured stdout is unchanged.
//   - non-TTY refusal: unchanged — same error, nothing printed.
func TestConfirmSubmitPointsAtTheFirstReviewChecklist(t *testing.T) {
	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0"}

	t.Run("interactive", func(t *testing.T) {
		withStdinTTY(t, true)
		c := newSplitConfirmCmd("y\n")
		if err := confirmSubmit(c.cmd, m, "https://civitai.com", false); err != nil {
			t.Fatalf("TTY + y must proceed: %v", err)
		}
		out := c.out.String()
		p, q := strings.Index(out, wantFirstReviewPointer), strings.Index(out, "Submit for review? [y/N]")
		if q < 0 {
			t.Fatalf("CONTROL failure, not a finding: no prompt on stdout:\n%s", out)
		}
		if p < 0 || p > q {
			t.Errorf("the interactive prompt does not name the first-review checklist before asking "+
				"(pointer at %d, question at %d):\n%s", p, q, out)
		}
		if c.err.Len() != 0 {
			t.Errorf("interactive confirm wrote to stderr: %q", c.err.String())
		}
	})

	t.Run("--yes", func(t *testing.T) {
		withStdinTTY(t, false)
		c := newSplitConfirmCmd("")
		if err := confirmSubmit(c.cmd, m, "https://civitai.com", true); err != nil {
			t.Fatalf("--yes must proceed: %v", err)
		}
		if got := strings.TrimSpace(c.err.String()); got != wantFirstReviewPointer {
			t.Errorf("--yes stderr = %q, want exactly the pointer %q", got, wantFirstReviewPointer)
		}
		if c.out.Len() != 0 {
			t.Errorf("--yes wrote to stdout, which scripts capture: %q", c.out.String())
		}
	})

	t.Run("non-TTY refusal is unchanged", func(t *testing.T) {
		withStdinTTY(t, false)
		c := newSplitConfirmCmd("")
		err := confirmSubmit(c.cmd, m, "https://civitai.com", false)
		if err == nil || !strings.Contains(err.Error(), "refusing to submit without --yes") {
			t.Fatalf("non-TTY without --yes must still refuse, got %v", err)
		}
		if c.out.Len()+c.err.Len() != 0 {
			t.Errorf("the refusal path printed output: stdout=%q stderr=%q", c.out.String(), c.err.String())
		}
	})
}

// TestAppSubmitYesKeepsThePointerOffStdout drives the real command end to end:
// a successful `--yes` upload exits nil, carries the pointer on stderr, and its
// stdout still reports the publish request without it.
func TestAppSubmitYesKeepsThePointerOffStdout(t *testing.T) {
	tmp := t.TempDir()
	writeStaticManifest(t, tmp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publishRequestId": "pr_42", "slug": "demo-block", "version": "0.1.0", "status": "pending",
		})
	}))
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-xyz")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")

	stdout, stderr, err := run(t, "app", "submit", tmp, "--yes")
	if err != nil {
		t.Fatalf("submit --yes: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "pr_42") {
		t.Fatalf("CONTROL failure, not a finding: stdout does not report the publish request:\n%s", stdout)
	}
	if !strings.Contains(stderr, wantFirstReviewPointer) {
		t.Errorf("`app submit --yes` never pointed at the first-review checklist; stderr:\n%s", stderr)
	}
	if strings.Contains(stdout, "first-review") {
		t.Errorf("the pointer leaked onto stdout:\n%s", stdout)
	}
}
