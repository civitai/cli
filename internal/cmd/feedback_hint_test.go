package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/internal/devtunnel"
	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// `civitai feedback` is ADVERTISED: listed in the root help, named in the
// managed AGENTS.md block and the README, and suggested at a small whitelist of
// platform-failure sites (feedback_hint.go). Every expectation below is a
// LITERAL written from what the CLI is meant to print — none is computed from
// the code under test.

// The hint line each whitelisted site prints, verbatim.
const (
	wantHintSubmitRefused = `Feedback: if this is the Civitai platform getting in your way, report it to Civitai: ` +
		`civitai feedback send --about review --ref submit.server-refused "app submit was refused by the server: <the error above>"`
	wantHintDoctorRollout = `Feedback: if this is the Civitai platform getting in your way, report it to Civitai: ` +
		`civitai feedback send --about cli --ref doctor.rollout-unreadable "app doctor could not read the App Blocks rollout answer: <the rollout lines above>"`
	wantHintTunnelDial = `Feedback: if this is the Civitai platform getting in your way, report it to Civitai: ` +
		`civitai feedback send --about cli --ref tunnel.dial-failed "app dev-tunnel could not open the reverse tunnel: <the error above>"`
	wantHintDecode = `Feedback: if this is the Civitai platform getting in your way, report it to Civitai: ` +
		`civitai feedback send --about cli --ref decode.unreadable-response "<the command you ran> got a response the CLI could not read: <the error above>"`
	wantAgentSetupFooter = "Feedback: when the Civitai platform itself gets in your way, report it with " +
		`civitai feedback send --about cli "<what you ran, what you expected, what happened>" ` +
		"(--about: cli, sdk, docs, review, sandbox, scopes, mcp, other); check on it later with " +
		"`civitai feedback status`. Not `civitai app feedback` — that is your app's user inbox."
)

// feedbackHintMarker is how a test asks "did ANY hint print": the prefix every
// hint line opens with, followed by the verb.
const feedbackHintMarker = "civitai feedback send"

// ---------------------------------------------------------------------------
// The ledger.
// ---------------------------------------------------------------------------

// TestEveryFeedbackHintSiteIsLedgered pins the WHITELIST as a set. Adding a
// site — or dropping one — is a decision, so it has to be made here too.
func TestEveryFeedbackHintSiteIsLedgered(t *testing.T) {
	want := []string{
		wantAgentSetupFooterCommand,
		wantHintDecode, wantHintDoctorRollout, wantHintSubmitRefused, wantHintTunnelDial,
	}
	var got []string
	for _, s := range feedbackHintSites {
		if s.ref == "" {
			got = append(got, s.command())
			continue
		}
		got = append(got, s.line())
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the hint whitelist changed.\n--- want ---\n%s\n--- got ---\n%s",
			strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
	for _, s := range feedbackHintSites {
		if !containsString(appapi.PlatformFeedbackAbouts, s.about) {
			t.Errorf("site %q suggests --about %q, which `feedback send` refuses", s.ref, s.about)
		}
		if s.ref != "" && !appapi.ValidPlatformFeedbackRef(s.ref) {
			t.Errorf("site key %q is not a valid --ref", s.ref)
		}
	}
}

// wantAgentSetupFooterCommand is the command the footer suggests.
const wantAgentSetupFooterCommand = `civitai feedback send --about cli "<what you ran, what you expected, what happened>"`

// splitHintCommand turns a suggested command into argv the way a POSIX shell
// would for the shapes a hint uses: bare words, and one double-quoted argument.
func splitHintCommand(t *testing.T, line string) []string {
	t.Helper()
	var args []string
	for rest := strings.TrimSpace(line); rest != ""; rest = strings.TrimSpace(rest) {
		if rest[0] == '"' {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				t.Fatalf("unterminated quote in %q", line)
			}
			args = append(args, rest[1:1+end])
			rest = rest[2+end:]
			continue
		}
		word, tail, _ := strings.Cut(rest, " ")
		args = append(args, word)
		rest = tail
	}
	return args
}

// TestEveryFeedbackHintCommandRuns drives each suggested command, AS PRINTED,
// through the real binary's `feedback send` under --dry-run. A hint naming a
// flag or a value the binary refuses is an instruction an agent follows into a
// usage error.
func TestEveryFeedbackHintCommandRuns(t *testing.T) {
	// CONTROL on the splitter: the quoted message must come back as ONE arg.
	if got := splitHintCommand(t, `civitai feedback send --about cli "a b c"`); len(got) != 6 || got[5] != "a b c" {
		t.Fatalf("CONTROL failure: splitter produced %q", got)
	}
	ran := 0
	for _, s := range feedbackHintSites {
		cmdline := s.command()
		t.Run(cmdline, func(t *testing.T) {
			f := &pfFake{t: t}
			f.serve()
			argv := splitHintCommand(t, cmdline)
			if argv[0] != "civitai" {
				t.Fatalf("a hint must start with `civitai`: %q", cmdline)
			}
			out, errOut, err := run(t, append(argv[1:], "--dry-run")...)
			if err != nil {
				t.Fatalf("`%s --dry-run` failed: %v\nstderr:\n%s", cmdline, err, errOut)
			}
			if !strings.Contains(out, `"about": "`+s.about+`"`) {
				t.Errorf("the dry run does not carry --about %s:\n%s", s.about, out)
			}
			if s.ref != "" && !strings.Contains(out, `"ref": "`+s.ref+`"`) {
				t.Errorf("the dry run does not carry --ref %s:\n%s", s.ref, out)
			}
			if n := len(f.seen()); n != 0 {
				t.Errorf("--dry-run made %d request(s)", n)
			}
			ran++
		})
	}
	if ran != len(feedbackHintSites) || ran < 5 {
		t.Fatalf("CONTROL failure: ran %d of %d hint commands", ran, len(feedbackHintSites))
	}
}

// ---------------------------------------------------------------------------
// Site 1 — `app submit`, refused by the server.
// ---------------------------------------------------------------------------

func TestSubmitHintFiresOnAPlatformRefusal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		// kind is the classification the error must STILL carry: the hint
		// must not move an exit code.
		kind error
	}{
		{"500", http.StatusInternalServerError, nil},
		{"502", http.StatusBadGateway, civitai.ErrNetwork},
		{"400 for a bundle local validate passed", http.StatusBadRequest, civitai.ErrBadRequest},
		{"413", http.StatusRequestEntityTooLarge, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newUploadRecorder(t, tc.status, `{"error":"refused"}`)
			client := appapi.New(rec.URL, "tok", "/api/blocks/submit-version")
			stderr, err := doUploadStderr(t, client)
			if err == nil {
				t.Fatalf("a %d must fail the upload", tc.status)
			}
			if rec.hits.Load() != 1 {
				t.Fatalf("CONTROL failure: the server saw %d request(s), want 1", rec.hits.Load())
			}
			if !strings.Contains(stderr, wantHintSubmitRefused+"\n") {
				t.Errorf("a %d from the submit route did not print the hint.\nwant line:\n%s\nstderr:\n%s",
					tc.status, wantHintSubmitRefused, stderr)
			}
			if strings.Count(stderr, feedbackHintMarker) != 1 {
				t.Errorf("want exactly ONE hint line, stderr:\n%s", stderr)
			}
			if tc.kind != nil && !errors.Is(err, tc.kind) {
				t.Errorf("the %d lost its classification %v: %v", tc.status, tc.kind, err)
			}
			if !strings.Contains(err.Error(), "refused") {
				t.Errorf("the server's words are no longer in the error: %v", err)
			}
		})
	}
}

func TestSubmitHintAbsentWhenTheRefusalIsTheCallers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"401", http.StatusUnauthorized},
		{"403 no Apps access", http.StatusForbidden},
		{"404", http.StatusNotFound},
		{"409 version exists", http.StatusConflict},
		{"429", http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newUploadRecorder(t, tc.status, `{"error":"refused"}`)
			client := appapi.New(rec.URL, "tok", "/api/blocks/submit-version")
			stderr, err := doUploadStderr(t, client)
			if err == nil || rec.hits.Load() != 1 {
				t.Fatalf("CONTROL failure: err=%v hits=%d — the refusal path was not exercised", err, rec.hits.Load())
			}
			if strings.Contains(stderr, feedbackHintMarker) {
				t.Errorf("a %d is the caller's to fix, not platform friction — no hint.\nstderr:\n%s", tc.status, stderr)
			}
		})
	}
	t.Run("nothing sent (the dial never connected)", func(t *testing.T) {
		rec := newUploadRecorder(t, http.StatusInternalServerError, `{}`)
		url := rec.URL
		rec.Close()
		stderr, err := doUploadStderr(t, appapi.New(url, "tok", "/api/blocks/submit-version"))
		if !errors.Is(err, appapi.ErrNothingSent) {
			t.Fatalf("CONTROL failure: want ErrNothingSent, got %v", err)
		}
		if strings.Contains(stderr, feedbackHintMarker) {
			t.Errorf("nothing reached the server — no hint.\nstderr:\n%s", stderr)
		}
	})
}

// TestSubmitHintAbsentOnSuccess: a submit the server accepts prints no hint,
// on either stream.
func TestSubmitHintAbsentOnSuccess(t *testing.T) {
	rec := newUploadRecorder(t, http.StatusOK, `{"publishRequestId":"pubreq_1","slug":"demo"}`)
	client := appapi.New(rec.URL, "tok", "/api/blocks/submit-version")
	var out, errBuf strings.Builder
	c := &cobra.Command{}
	c.SetOut(&out)
	c.SetErr(&errBuf)
	m := &manifest.Manifest{BlockID: "demo", Version: "0.1.0", Name: "Demo"}
	if err := doUpload(c, client, zipPayload, m, "https://civitai.com/", appapi.Provenance{}); err != nil {
		t.Fatalf("CONTROL failure: a 200 must succeed: %v", err)
	}
	if !strings.Contains(out.String(), "pubreq_1") {
		t.Fatalf("CONTROL failure: the success report is missing:\n%s", out.String())
	}
	if strings.Contains(out.String()+errBuf.String(), feedbackHintMarker) {
		t.Errorf("a successful submit printed a hint:\n%s\n%s", out.String(), errBuf.String())
	}
}

// ---------------------------------------------------------------------------
// Site 2 — `app doctor`, an unreadable rollout answer.
// ---------------------------------------------------------------------------

func TestDoctorHintFiresOnlyOnAnUnreadableRolloutAnswer(t *testing.T) {
	fired := 0
	for _, f := range rolloutOutcomeFixtures {
		for _, jsonOut := range []bool{false, true} {
			name := f.state
			if jsonOut {
				name += "/json"
			}
			t.Run(name, func(t *testing.T) {
				newDoctorServerRollout(t, f.resp.status, f.resp.body,
					doctorRow(docSlugA, docListingA, "draft", "owner", nil))
				args := []string{"app", "doctor"}
				if jsonOut {
					args = append(args, "--json")
				}
				stdout, stderr, err := run(t, args...)
				if err != nil {
					t.Fatalf("a clean listing must exit 0 whatever the rollout said: %v", err)
				}
				if strings.Contains(stdout, feedbackHintMarker) {
					t.Errorf("a hint reached STDOUT:\n%s", stdout)
				}
				if jsonOut {
					var payload map[string]any
					if jerr := json.Unmarshal([]byte(stdout), &payload); jerr != nil {
						t.Errorf("--json stdout is no longer one JSON document: %v\n%s", jerr, stdout)
					}
				}
				if f.state == string(appapi.RolloutUnknown) {
					fired++
					if !strings.Contains(stderr, wantHintDoctorRollout+"\n") {
						t.Errorf("an unreadable rollout answer did not print the hint on stderr.\nwant:\n%s\nstderr:\n%s",
							wantHintDoctorRollout, stderr)
					}
					return
				}
				if strings.Contains(stderr, feedbackHintMarker) {
					t.Errorf("rollout state %q is not platform friction — no hint.\nstderr:\n%s", f.state, stderr)
				}
			})
		}
	}
	if fired != 2 {
		t.Fatalf("CONTROL failure: the unknown-state fixture ran %d time(s), want 2 (human + json)", fired)
	}
}

// ---------------------------------------------------------------------------
// Site 3 — `app dev-tunnel`, the reverse tunnel would not open.
// ---------------------------------------------------------------------------

func TestTunnelHintFiresWhenTheDialFails(t *testing.T) {
	apiStub := &fakeTunnelAPI{startResult: sampleSession()}
	deps := baseDeps(t, apiStub, &fakeDialer{err: errors.New("connection refused")}, newFakeTimer(), make(chan os.Signal))
	err := runTunnelSession(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "open reverse tunnel") {
		t.Fatalf("CONTROL failure: want the dial error, got %v", err)
	}
	stderr := deps.errw.(*strings.Builder).String()
	if !strings.Contains(stderr, wantHintTunnelDial+"\n") {
		t.Errorf("a failed tunnel dial did not print the hint.\nwant:\n%s\nstderr:\n%s", wantHintTunnelDial, stderr)
	}
	if strings.Contains(deps.out.(*strings.Builder).String(), feedbackHintMarker) {
		t.Error("the hint reached stdout")
	}
}

func TestTunnelHintAbsentOnTheCallersFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(d *tunnelSessionDeps, api *fakeTunnelAPI)
	}{
		{"no local dev server", func(d *tunnelSessionDeps, _ *fakeTunnelAPI) {
			d.probeLocal = func(string, int) error { return errors.New("no local dev server is listening") }
		}},
		{"keygen failed", func(d *tunnelSessionDeps, _ *fakeTunnelAPI) {
			d.keygen = func() (*devtunnel.EphemeralKey, error) { return nil, errors.New("no entropy") }
		}},
		{"mint refused 403 (wrong account)", func(_ *tunnelSessionDeps, api *fakeTunnelAPI) {
			api.startResult = nil
			api.startErr = &appapi.DevTunnelForbiddenError{}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiStub := &fakeTunnelAPI{startResult: sampleSession()}
			deps := baseDeps(t, apiStub, &fakeDialer{tunnel: newFakeTunnel()}, newFakeTimer(), make(chan os.Signal))
			tc.setup(&deps, apiStub)
			if err := runTunnelSession(context.Background(), deps); err == nil {
				t.Fatal("CONTROL failure: the case did not fail")
			}
			if s := deps.errw.(*strings.Builder).String(); strings.Contains(s, feedbackHintMarker) {
				t.Errorf("%s is not platform friction — no hint.\nstderr:\n%s", tc.name, s)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Site 4 — an unreadable 2xx response, from any command (cmd/civitai prints
// it; FeedbackHint decides).
// ---------------------------------------------------------------------------

func TestFeedbackHintForAnUnreadableResponse(t *testing.T) {
	unreadable := civitai.Tag(civitai.ErrUnreadableResponse, errors.New("unexpected response from /x (status 200): <html>"))
	if got := FeedbackHint(unreadable); got != wantHintDecode {
		t.Errorf("FeedbackHint(unreadable response):\n got %q\nwant %q", got, wantHintDecode)
	}
	for _, err := range []error{
		civitai.Tag(civitai.ErrNotFound, errors.New("no such model")),
		civitai.Tag(civitai.ErrUnauthorized, errors.New("no token")),
		asUsageError(errors.New("bad flag")),
		errors.New("plain"),
		nil,
	} {
		if got := FeedbackHint(err); got != "" {
			t.Errorf("FeedbackHint(%v) = %q, want none", err, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Site 5 — the agent-setup footer.
// ---------------------------------------------------------------------------

func TestAgentSetupFooterNamesThePlatformFeedbackChannel(t *testing.T) {
	dir, _ := agentSetupProject(t)
	out, _, err := run(t, "--no-color", "agent-setup", "--dir", dir, "--agent", "claude")
	if err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	if !strings.Contains(out, "\nNext:\n") {
		t.Fatalf("CONTROL failure: no Next: block in the human report:\n%s", out)
	}
	if !strings.Contains(out, "\n"+wantAgentSetupFooter+"\n") {
		t.Errorf("the agent-setup footer does not carry the feedback line.\nwant:\n%s\nstdout:\n%s", wantAgentSetupFooter, out)
	}
	if strings.Index(out, wantAgentSetupFooter) < strings.Index(out, "\nNext:\n") {
		t.Error("the feedback line must come after the Next: block")
	}

	jsonOut, _, err := run(t, "agent-setup", "--dir", dir, "--agent", "claude", "--json")
	if err != nil {
		t.Fatalf("agent-setup --json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("--json stdout is not one JSON document: %v\n%s", err, jsonOut)
	}
	if strings.Contains(jsonOut, feedbackHintMarker) {
		t.Errorf("the footer reached the --json payload:\n%s", jsonOut)
	}
}

// ---------------------------------------------------------------------------
// The help listing and both Shorts.
// ---------------------------------------------------------------------------

// The two Shorts, verbatim. Each states its DIRECTION, because `civitai --help`
// and `civitai app --help` print nothing else about either.
const (
	wantFeedbackShort    = "Report Civitai developer-platform problems TO Civitai (not your app's user feedback)"
	wantAppFeedbackShort = "Your app's inbox: read what YOUR APP'S USERS wrote (to report a platform problem: civitai feedback)"
)

func TestFeedbackShortsStateTheirLane(t *testing.T) {
	root := NewRootCmd()
	var verb, appVerb *cobra.Command
	for _, c := range root.Commands() {
		switch c.Name() {
		case "feedback":
			verb = c
		case "app":
			for _, sub := range c.Commands() {
				if sub.Name() == "feedback" {
					appVerb = sub
				}
			}
		}
	}
	if verb == nil || appVerb == nil {
		t.Fatalf("CONTROL failure: feedback=%v app feedback=%v", verb != nil, appVerb != nil)
	}
	if verb.Short != wantFeedbackShort {
		t.Errorf("`civitai feedback` Short:\n got %q\nwant %q", verb.Short, wantFeedbackShort)
	}
	if appVerb.Short != wantAppFeedbackShort {
		t.Errorf("`civitai app feedback` Short:\n got %q\nwant %q", appVerb.Short, wantAppFeedbackShort)
	}
}

func TestRootHelpListsTheFeedbackVerb(t *testing.T) {
	help := renderRootHelp(t)
	if !strings.Contains(help, "\n  whoami ") {
		t.Fatalf("CONTROL failure: the rendered root help lists no `whoami` row:\n%s", help)
	}
	row := ""
	for _, line := range strings.Split(help, "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "feedback" {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("`civitai --help` does not list `feedback`:\n%s", help)
	}
	if !strings.HasSuffix(strings.TrimSpace(row), wantFeedbackShort) {
		t.Errorf("the `feedback` row does not carry its Short: %q", row)
	}

	appHelp, _, err := run(t, "app", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(appHelp, wantAppFeedbackShort) {
		t.Errorf("`civitai app --help` does not carry the app feedback Short:\n%s", appHelp)
	}
}

// TestPlatformFeedbackVerbIsAdvertised replaces the shipped
// TestPlatformFeedbackVerbIsUnadvertised: every surface that test kept the verb
// OFF now carries it.
func TestPlatformFeedbackVerbIsAdvertised(t *testing.T) {
	names := func(src string) bool {
		return strings.Contains(strings.ReplaceAll(src, "civitai app feedback", ""), "civitai feedback")
	}
	if !names("run `civitai feedback send`") || names("run `civitai app feedback my-app`") {
		t.Fatal("CONTROL failure: the matcher cannot tell `civitai feedback` from `civitai app feedback`")
	}
	for _, c := range NewRootCmd().Commands() {
		if c.Name() == "feedback" && c.Hidden {
			t.Error("the root `feedback` command is still Hidden")
		}
	}
	if !names(readREADME(t)) {
		t.Error("README.md does not document `civitai feedback`")
	}
	tmpl, err := os.ReadFile(filepath.Join("templates", "agents-app.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !names(string(tmpl)) {
		t.Error("the managed AGENTS.md template does not name `civitai feedback`")
	}
}

// ---------------------------------------------------------------------------
// The managed AGENTS.md section.
// ---------------------------------------------------------------------------

// wantPlatformProblemSection is the WHOLE section, whitespace-normalised —
// pinned whole for the reason wantFeedbackTriageSection gives.
const wantPlatformProblemSection = "### Reporting a platform problem " +
	"When the Civitai platform itself gets in your way (CLI, SDK, docs, review, sandbox, scopes, MCP), " +
	"report it: `civitai feedback send --about <area> \"<what happened>\"`, then " +
	"`civitai feedback status <ref>`. It is NOT `civitai app feedback` (your app's user inbox)."

func TestTheBlockCarriesThePlatformProblemSectionInEveryShape(t *testing.T) {
	shapes := allProjectShapesForTest()
	if len(shapes) < 3 {
		t.Fatalf("CONTROL failure: %d shape(s) to render", len(shapes))
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
			got, ok := blockSectionOf(block, "Reporting a platform problem")
			if !ok {
				t.Errorf("kind %q (sdk=%t): no `### Reporting a platform problem` section", shape.Kind, sdk)
				continue
			}
			if n := normaliseMessage(got); n != wantPlatformProblemSection {
				t.Errorf("kind %q (sdk=%t): the section is not what is pinned.\n--- want ---\n%s\n--- got ---\n%s",
					shape.Kind, sdk, wantPlatformProblemSection, n)
			}
			t.Logf("kind %q scripts=%d wizard=%t sdk=%t: block is %d bytes, the section %d",
				shape.Kind, len(shape.Scripts), shape.HasSetupWizard, sdk, len(block), len(got))
		}
	}
	if rendered != 2*len(shapes) {
		t.Fatalf("CONTROL failure: rendered %d block(s), want %d", rendered, 2*len(shapes))
	}
}

// TestThePlatformProblemSectionsCommandsRun drives both commands the section
// names, with its placeholders filled, against a fake server.
func TestThePlatformProblemSectionsCommandsRun(t *testing.T) {
	for _, spelled := range []string{
		"`civitai feedback send --about <area> \"<what happened>\"`",
		"`civitai feedback status <ref>`",
	} {
		if !strings.Contains(wantPlatformProblemSection, spelled) {
			t.Fatalf("CONTROL failure: the pinned section does not spell %s", spelled)
		}
	}
	for _, area := range appapi.PlatformFeedbackAbouts {
		f := &pfFake{t: t}
		f.serve()
		if _, errOut, err := run(t, "feedback", "send", "--about", area, "<what happened>", "--dry-run"); err != nil {
			t.Errorf("`civitai feedback send --about %s \"<what happened>\" --dry-run`: %v\n%s", area, err, errOut)
		}
	}
	f := &pfFake{t: t, status: func(string) (int, string) { return http.StatusOK, trpcEnvelope(pfStatusAll) }}
	f.serve()
	out, _, err := run(t, "--no-color", "feedback", "status", "8001-received")
	if err != nil {
		t.Fatalf("`civitai feedback status <ref>`: %v", err)
	}
	if !strings.Contains(out, "8001-received") || !strings.Contains(out, "received") {
		t.Errorf("`feedback status <ref>` did not report the ref:\n%s", out)
	}
}
