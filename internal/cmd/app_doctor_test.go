package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// `civitai app doctor` (civitai/civitai#4341's client half).
//
// Everything here runs against an httptest fake. Nothing in this file has been
// run against a real listing.
//
// 🔴 THE FIXTURE CONSTANTS ARE PAIRWISE DISTINCT AND DISTINCT FROM EVERY
// CONSTANT AN ASSERTION NAMES. A fixture that can only ever produce the value
// the assertion spells is invisible to a mutant that hardcodes that literal —
// so the slugs, the listing ids and the block ids below share no substring with
// each other or with any command name, and the two apps in the multi-app
// fixtures disagree on every field.

const (
	docSlugA    = "doctor-alpha"
	docSlugB    = "doctor-bravo"
	docListingA = "apl_ALPHA111"
	docListingB = "apl_BRAVO222"
	docBlockA   = "block_ALPHA333"
	// The DELISTED fixture. Distinct from every other slug and id, so an
	// assertion about the delisted arm cannot pass on a value copied from the
	// gating one.
	docSlugC    = "doctor-charlie"
	docListingC = "apl_CHARLIE333"
)

// listMinePath is the ONE route `app doctor` is allowed to speak. It is spelled
// here, in the test, rather than read from the production route var: a test that
// reads the value it is checking agrees with any change to it.
const listMinePath = "/api/trpc/appListings.listMine"

// doctorProblem builds one server-shaped problem row.
func doctorProblem(code, label, severity string) map[string]any {
	return map[string]any{"code": code, "label": label, "severity": severity}
}

// doctorRow builds one server-shaped listMine row.
func doctorRow(slug, listingID, status, role string, blockID *string, problems ...map[string]any) map[string]any {
	if problems == nil {
		problems = []map[string]any{}
	}
	return map[string]any{
		"appListingId": listingID,
		"slug":         slug,
		// 🔴 EVERY FIXTURE ROW CARRIES A KIND, because the real server always
		// sends one and a fake that omits it would encode the same blind spot
		// the code had. `offsite` is the default here so the existing
		// browser-editor assertions keep testing the arm they were written for;
		// the onsite arm has its own dedicated cases.
		"kind":       "offsite",
		"name":       strings.ToUpper(slug[:1]) + slug[1:] + " Display Name",
		"status":     status,
		"role":       role,
		"appBlockId": blockID,
		"problems":   problems,
	}
}

// doctorServer answers listMine with rows, and RECORDS every path it is asked
// for. The recording is what makes the request-ledger test possible; every other
// test gets it for free and fails loudly if the command starts making a second
// call.
type doctorServer struct {
	*httptest.Server
	mu    sync.Mutex
	paths []string
}

func (d *doctorServer) seen() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.paths))
	copy(out, d.paths)
	return out
}

func newDoctorServer(t *testing.T, rows ...map[string]any) *doctorServer {
	t.Helper()
	d := &doctorServer{}
	if rows == nil {
		rows = []map[string]any{}
	}
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.paths = append(d.paths, r.URL.Path)
		d.mu.Unlock()
		if r.URL.Path != listMinePath {
			// 🔴 A 500 rather than a t.Errorf alone: a command that made an
			// unexpected call must FAIL, not merely be noted, or a test can pass
			// while the CLI quietly reaches a proc that 403s in production.
			t.Errorf("unexpected request to %s — `app doctor` speaks only %s", r.URL.Path, listMinePath)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		trpcData(w, rows)
	}))
	t.Cleanup(d.Close)
	listingEnv(t, d.URL)
	return d
}

// ---------------------------------------------------------------------------
// The exit-code contract. Three arms, because two cannot tell "correct" from
// "always exits 0" — or from "always exits 1".
// ---------------------------------------------------------------------------

// TestDoctorExitsNonZeroOnABlockingProblem is the arm the whole command exists
// for: a release script's `civitai app doctor my-app || exit 1`.
func TestDoctorExitsNonZeroOnABlockingProblem(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err == nil {
		t.Fatalf("a blocking problem must exit non-zero; stdout was:\n%s", stdout)
	}
	if !errors.Is(err, ErrListingBlocked) {
		t.Errorf("error must be tagged ErrListingBlocked (that is what carries the exit code), got %T: %v", err, err)
	}
	if !strings.Contains(stdout, "missing-icon") {
		t.Errorf("the finding must be on stdout before the exit, got:\n%s", stdout)
	}
}

// TestDoctorExitsZeroOnACleanListing is the arm that keeps the one above from
// being satisfied by a command that always fails.
func TestDoctorExitsZeroOnACleanListing(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "approved", "owner", ptr(docBlockA)))
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("a listing with no problems must exit 0, got %v; stdout was:\n%s", err, stdout)
	}
	// 🔴 EXPLICITLY CLEAN, not a gap. An empty section is indistinguishable
	// from a read that failed and was swallowed.
	if !strings.Contains(stdout, "No problems") {
		t.Errorf("a clean listing must SAY it is complete, got:\n%s", stdout)
	}
}

// TestDoctorExitsZeroOnAdvisoryOnly is the third arm, and the one that pins the
// severity split to the exit code rather than to "any problem at all".
func TestDoctorExitsZeroOnAdvisoryOnly(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "approved", "owner", ptr(docBlockA),
		doctorProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory"),
		doctorProblem("empty-tagline", "Missing tagline", "advisory"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("advisories alone must exit 0, got %v; stdout was:\n%s", err, stdout)
	}
	// The findings must still be REPORTED — exiting 0 is not the same as
	// staying quiet, and an author who never sees the advisory never acts on it.
	for _, want := range []string{"no-screenshots", "empty-tagline", "ADVISORY"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("advisory-only run must still print %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "BLOCKING") {
		t.Errorf("an advisory-only run must not print a BLOCKING heading, got:\n%s", stdout)
	}
}

// TestDoctorBlockingWinsOverAdvisoriesOnTheSameApp: a mixed app is blocked. The
// count in the summary is what a mutant that stops at the first finding, or that
// counts apps instead of problems, gets wrong.
func TestDoctorBlockingWinsOverAdvisoriesOnTheSameApp(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory"),
		doctorProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"),
		doctorProblem("empty-category", "Missing category", "advisory"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err == nil {
		t.Fatalf("a mixed app with one blocking problem must exit non-zero; stdout:\n%s", stdout)
	}
	// 1 blocking and 2 advisory — three DIFFERENT numbers (1, 2, 3 findings on
	// 1 app), so a summary that reports the wrong quantity cannot look right.
	if !strings.Contains(stdout, "1 app(s) checked — 1 blocking, 2 advisory.") {
		t.Errorf("summary line is wrong; got:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Every one of the eight codes, including the two that could not fire anywhere
// before civitai/civitai#4341 wired assetScans into listMine.
// ---------------------------------------------------------------------------

// allEightProblems is a fixture that emits every code `computeListingProblems`
// can produce, at the severity the server gives it.
//
// 🔴 IT INCLUDES `blocked-media` AND `scanning-media`. Those two were
// structurally unreachable on every pre-#4341 surface, so a fixture set without
// them leaves their handling — including the fact that one of them is BLOCKING —
// untested while this file reads green.
func allEightProblems() []map[string]any {
	return []map[string]any{
		doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
		doctorProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"),
		doctorProblem("blocked-media", "Replace the blocked cover before it can publish", "blocking"),
		doctorProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory"),
		doctorProblem("empty-description", "Missing description", "advisory"),
		doctorProblem("empty-tagline", "Missing tagline", "advisory"),
		doctorProblem("empty-category", "Missing category", "advisory"),
		doctorProblem("scanning-media", "Icon is still being scanned", "advisory"),
	}
}

// TestDoctorReportsAllEightCodesInTheRightGroup drives every code through the
// real command and asserts each lands under the heading its SERVER-GIVEN
// severity puts it under.
func TestDoctorReportsAllEightCodesInTheRightGroup(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil, allEightProblems()...))
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err == nil {
		t.Fatalf("three of the eight are blocking, so this must exit non-zero; stdout:\n%s", stdout)
	}
	got := decodeDoctorJSON(t, stdout)
	if len(got.Apps) != 1 {
		t.Fatalf("want 1 app, got %d", len(got.Apps))
	}
	wantBlocking := []string{"missing-icon", "missing-cover", "blocked-media"}
	wantAdvisory := []string{"no-screenshots", "empty-description", "empty-tagline", "empty-category", "scanning-media"}
	if diff := codesOf(got.Apps[0].Blocking); !equalStrings(diff, wantBlocking) {
		t.Errorf("blocking codes = %v, want %v", diff, wantBlocking)
	}
	if diff := codesOf(got.Apps[0].Advisory); !equalStrings(diff, wantAdvisory) {
		t.Errorf("advisory codes = %v, want %v", diff, wantAdvisory)
	}
	// 3 and 5 are different numbers from each other and from 8 and 1, so a
	// summary that counted the wrong thing cannot coincide.
	if got.Summary.Blocking != 3 || got.Summary.Advisory != 5 || got.Summary.Apps != 1 {
		t.Errorf("summary = %+v, want {apps:1 blocking:3 advisory:5}", got.Summary)
	}
	if got.OK {
		t.Error("ok must be false when anything is blocking")
	}
}

// TestDoctorPositiveControlOnTheFindingCount is the pair the rules ask for: a
// reassuring ZERO is indistinguishable from a probe wired to nothing, so the
// same command is driven against a payload that MUST produce eight findings and
// against one that must produce none, and BOTH numbers are reported.
func TestDoctorPositiveControlOnTheFindingCount(t *testing.T) {
	// Control arm: eight findings must be counted.
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil, allEightProblems()...))
	stdout, _, _ := run(t, "app", "doctor", "--json")
	control := decodeDoctorJSON(t, stdout)
	controlTotal := control.Summary.Blocking + control.Summary.Advisory
	if controlTotal != 8 {
		t.Fatalf("POSITIVE CONTROL FAILED: a payload carrying 8 problems produced %d findings. "+
			"Every zero this file reports is a fact about a probe wired to nothing until this passes.", controlTotal)
	}

	// Test arm: the same command, the same code path, a clean payload.
	srv2 := newDoctorServer(t, doctorRow(docSlugB, docListingB, "approved", "owner", ptr(docBlockA)))
	_ = srv2
	stdout2, _, err := run(t, "app", "doctor", "--json")
	if err != nil {
		t.Fatalf("clean arm must exit 0: %v", err)
	}
	under := decodeDoctorJSON(t, stdout2)
	underTotal := under.Summary.Blocking + under.Summary.Advisory
	if underTotal != 0 {
		t.Errorf("clean payload produced %d findings, want 0", underTotal)
	}
	t.Logf("positive-control pair: %d findings on the control, %d under test", controlTotal, underTotal)
}

// ---------------------------------------------------------------------------
// The `--json` contract. Pinned WHOLE, not by a handful of keys.
// ---------------------------------------------------------------------------

// TestDoctorJSONShapeIsPinnedWhole compares the ENTIRE marshalled payload
// against a literal, so a renamed field, a dropped field, a reordered struct or
// a changed null-vs-omitted decision is visible.
//
// 🔴 A FEW `Contains` CHECKS WOULD NOT DO THIS. `app doctor --json` is a
// published contract (README documents it); a substring assertion agrees with a
// payload that has quietly grown, lost or renamed a sibling key.
func TestDoctorJSONShapeIsPinnedWhole(t *testing.T) {
	// 🔴 THE DELISTED ROW IS SENT FIRST ON THE WIRE and must come LAST in the
	// payload. Feeding them already in the right order would make the ordering
	// assertion pass against code that does no ordering at all.
	newDoctorServer(t,
		doctorRow(docSlugC, docListingC, "removed", "owner", nil,
			doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")),
		doctorRow(docSlugA, docListingA, "draft", "owner", nil,
			doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
			doctorProblem("empty-tagline", "Missing tagline", "advisory"),
		),
		// 🔴 THIS ROW IS `onsite` ON PURPOSE. Every other fixture reaching this
		// pin was "offsite", so `Kind: r.Kind` -> `Kind: "offsite"` was a
		// HARDCODING mutant no test could see — the documented blind spot where
		// a fixture that can only ever produce the constant's own value cannot
		// observe a mutant that hardcodes the literal. M39 nulled the field and
		// died; hardcoding survived a fully green suite. Two distinct kinds in
		// the pinned payload is the mechanical control.
		doctorRowKind(docSlugB, docListingB, "approved", "onsite"),
	)
	base := envBaseURL(t)
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err == nil {
		t.Fatalf("the DRAFT app is blocked, so this must exit non-zero; stdout:\n%s", stdout)
	}
	// 🔴 `<file>` arrives as `\u003cfile\u003e`. That is the SHARED writeJSON
	// encoder's HTML escaping (json.Encoder does it by default), not this
	// command's doing, and it is pinned here rather than worked around: a
	// consumer decoding the payload gets `<file>` back, and changing the shared
	// encoder would silently change every other `--json` contract in the CLI.
	//
	// 🔴 THE NUMBERS ARE ALL DIFFERENT FROM EACH OTHER: 3 apps, 2 blocking,
	// 1 advisory, 1 gating, 1 delisted. `blocking` and `gating` differ, which is
	// the whole point of publishing both — a payload that emitted one twice
	// cannot pass.
	want := `{
  "ok": false,
  "apps": [
    {
      "slug": "` + docSlugA + `",
      "name": "Doctor-alpha Display Name",
      "appListingId": "` + docListingA + `",
      "appBlockId": null,
      "status": "draft",
      "role": "owner",
      "kind": "offsite",
      "delisted": false,
      "blocking": [
        {
          "code": "missing-icon",
          "label": "Missing icon (required before publishing)",
          "severity": "blocking",
          "fix": "civitai app listing set-icon \u003cfile\u003e --slug ` + docSlugA + `"
        }
      ],
      "advisory": [
        {
          "code": "empty-tagline",
          "label": "Missing tagline",
          "severity": "advisory",
          "fix": "edit the listing in the browser: ` + base + `/apps/listing/` + docListingA + `/edit"
        }
      ]
    },
    {
      "slug": "` + docSlugB + `",
      "name": "Doctor-bravo Display Name",
      "appListingId": "` + docListingB + `",
      "appBlockId": null,
      "status": "approved",
      "role": "owner",
      "kind": "onsite",
      "delisted": false,
      "blocking": [],
      "advisory": []
    },
    {
      "slug": "` + docSlugC + `",
      "name": "Doctor-charlie Display Name",
      "appListingId": "` + docListingC + `",
      "appBlockId": null,
      "status": "removed",
      "role": "owner",
      "kind": "offsite",
      "delisted": true,
      "blocking": [
        {
          "code": "missing-icon",
          "label": "Missing icon (required before publishing)",
          "severity": "blocking",
          "fix": "civitai app listing set-icon \u003cfile\u003e --slug ` + docSlugC + `"
        }
      ],
      "advisory": []
    }
  ],
  "summary": {
    "apps": 3,
    "blocking": 2,
    "advisory": 1,
    "gating": 1,
    "delisted": 1,
    "truncated": false
  }
}
`
	if stdout != want {
		t.Errorf("--json payload changed.\n--- got ---\n%s\n--- want ---\n%s", stdout, want)
	}
}

// TestDoctorJSONIsTheWholeOfStdout: `… --json | jq -e .` must always parse, so
// nothing else may share the stream.
func TestDoctorJSONIsTheWholeOfStdout(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"),
	))
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err == nil {
		t.Fatal("expected a non-zero verdict")
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("stdout is not valid JSON (%v); stdout was:\n%s", err, stdout)
	}
	if _, err := dec.Token(); err != io.EOF {
		t.Fatalf("stdout carries more than the JSON payload (next token err = %v); stdout was:\n%s", err, stdout)
	}
	// internal/ui/CONVENTION.md rule 1: machine-readable output carries no
	// styling. The human renderer's own glyphs are the cheapest tell.
	for _, glyph := range []string{"✓", "⚠", "\x1b["} {
		if strings.Contains(stdout, glyph) {
			t.Errorf("--json stdout carries the styling marker %q — machine output must be unstyled:\n%s", glyph, stdout)
		}
	}
}

// ---------------------------------------------------------------------------
// The fix advice: only routes that exist.
// ---------------------------------------------------------------------------

// civitaiCommandRef matches a `civitai …` command invocation inside prose,
// stopping at the first token that is not a lowercase command word — so
// `<file>`, `--slug` and a URL's `civitai.com` are all excluded.
var civitaiCommandRef = regexp.MustCompile(`civitai(?: [a-z][a-z0-9-]*)+`)

// TestDoctorFixAdviceNamesOnlyCommandsThatExist resolves every command named in
// every fix line against the REAL cobra tree.
//
// 🔴 IT PARSES RATHER THAN SPELLING THE COMMANDS OUT. A literal assertion
// ("the fix contains `civitai app listing set-icon`") agrees with any string,
// including one no CLI would accept — the same argument
// TestDriftRemedyCommandIsRunnableFromTheWarningsOwnDirectory rests on. And a
// fix naming a command that does not exist is worse than no fix: the author
// follows it, is refused, and concludes the tool is broken.
func TestDoctorFixAdviceNamesOnlyCommandsThatExist(t *testing.T) {
	root := NewRootCmd()

	// 🔴 THE NEGATIVE CONTROL COMES FIRST. "Every reference resolved" and "the
	// resolver says yes to everything" are the same output.
	if err := resolveCommandRef(root, "civitai app listing set-banner"); err == nil {
		t.Fatal("negative control: the resolver accepted a command that does not exist — " +
			"every positive below would be a fact about the resolver, not about the advice")
	}

	base := "https://example.invalid"
	editURL := base + fmt.Sprintf(listingEditPath, docListingA)
	seen := 0
	// 🔴 EVERY CODE, AND `blocked-media` ONCE PER SLOT. `problemWithCode` builds
	// an EMPTY label, so `blocked-media` always took the FALLBACK arm here — the
	// three per-slot arms, which is where the new commands live, were never
	// resolved against the command tree at all. A ledger that walks "all eight
	// codes" while silently skipping three branches is the narrower-than-its-
	// description shape.
	type probe struct {
		code  string
		label string
	}
	probes := []probe{}
	for _, code := range allEightCodes() {
		probes = append(probes, probe{code, ""})
	}
	for _, kind := range []string{"icon", "cover", "screenshot"} {
		probes = append(probes, probe{"blocked-media", "Replace the blocked " + kind + " before it can publish"})
	}
	for _, pr := range probes {
		code := pr.code
		fix := doctorRemedy(appapi.ListingProblem{Code: pr.code, Label: pr.label}, docSlugA, editURL, "offsite")
		if fix == "" {
			t.Errorf("code %q produced an EMPTY fix line — every finding must say what to do", code)
			continue
		}
		refs := civitaiCommandRef.FindAllString(fix, -1)
		for _, ref := range refs {
			seen++
			if err := resolveCommandRef(root, ref); err != nil {
				t.Errorf("code %q advises %q, which does not resolve in the command tree: %v", code, ref, err)
			}
		}
	}
	// Positive control on the SCAN: if the regex stopped matching (a reworded
	// fix line, a changed command spelling) every check above would silently
	// become vacuous.
	// 🔴 THE FLOOR IS THE REAL COUNT, NOT A SAFE-LOOKING SMALLER ONE. At 5
	// against 7 actual references the `blocked-media` arm could lose two
	// commands and this control would still pass — a positive control with two
	// references of slack is a control for a scan that has half stopped working.
	// The floor rises with the per-slot arms now being walked.
	if seen < 12 {
		t.Fatalf("the fix-line scan found only %d command references across the eight codes — "+
			"the regex is not reading what it thinks it is, so the resolutions above prove nothing", seen)
	}
	t.Logf("resolved %d command references across %d probes (8 codes + 3 blocked-media slots)", seen, len(probes))
}

// TestDoctorMediaFixesCarryASlugFlagThatExists: the printed commands pass
// `--slug`, which only works if those commands actually declare it. A fix that
// names a real command with a flag it does not have is refused at exit 2.
func TestDoctorMediaFixesCarryASlugFlagThatExists(t *testing.T) {
	root := NewRootCmd()
	for _, path := range [][]string{
		{"app", "listing", "set-icon"},
		{"app", "listing", "set-cover"},
		{"app", "listing", "add-screenshot"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd.Name() != path[len(path)-1] {
			t.Fatalf("%v does not resolve: %v", path, err)
		}
		if cmd.Flags().Lookup("slug") == nil {
			t.Errorf("`civitai %s` has no --slug flag, but `app doctor` prints it", strings.Join(path, " "))
		}
	}
	// Negative control on the flag lookup itself.
	cmd, _, _ := root.Find([]string{"app", "listing", "set-icon"})
	if cmd.Flags().Lookup("no-such-flag-here") != nil {
		t.Fatal("negative control: the flag lookup returns something for a flag that does not exist")
	}
}

// TestDoctorTextFixesPointAtTheListingEditorNotTheSlug pins the id space. The
// editor page is keyed by appListingId; building the URL from the slug would
// produce a well-formed URL that 404s, and nothing in the payload would look
// wrong.
func TestDoctorTextFixesPointAtTheListingEditorNotTheSlug(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("empty-description", "Missing description", "advisory"),
		doctorProblem("empty-tagline", "Missing tagline", "advisory"),
		doctorProblem("empty-category", "Missing category", "advisory"),
	))
	base := envBaseURL(t)
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err != nil {
		t.Fatalf("advisory-only must exit 0: %v", err)
	}
	got := decodeDoctorJSON(t, stdout)
	wantURL := base + "/apps/listing/" + docListingA + "/edit"
	for _, p := range got.Apps[0].Advisory {
		if !strings.Contains(p.Fix, wantURL) {
			t.Errorf("code %q fix = %q, want it to name %q", p.Code, p.Fix, wantURL)
		}
		// 🔴 The slug must NOT appear in the URL's position. docSlugA and
		// docListingA share no substring, so this cannot pass by coincidence.
		if strings.Contains(p.Fix, "/apps/listing/"+docSlugA+"/edit") {
			t.Errorf("code %q built the editor URL from the SLUG, which 404s: %q", p.Code, p.Fix)
		}
	}
}

// TestDoctorScanningMediaOffersNoAction: the one code with no remedy says so
// instead of naming a command that would restart the scan.
func TestDoctorScanningMediaOffersNoAction(t *testing.T) {
	fix := doctorRemedy(problemWithCode("scanning-media"), docSlugA, "https://example.invalid/x", "offsite")
	if !strings.Contains(fix, "nothing to do") {
		t.Errorf("scanning-media must say there is nothing to do, got %q", fix)
	}
	for _, banned := range []string{"set-icon", "set-cover", "add-screenshot"} {
		if strings.Contains(fix, banned) {
			t.Errorf("scanning-media must not advise %q — re-attaching restarts the scan; got %q", banned, fix)
		}
	}
}

// TestDoctorUnknownCodeStillReportsAndStillCounts: a NINTH server code is a
// server change, not a caller bug. It must still be printed, still be classified
// by the server's own severity, and still move the verdict.
func TestDoctorUnknownCodeStillReportsAndStillCounts(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("a-code-from-the-future", "Something new is wrong", "blocking"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err == nil {
		t.Fatalf("an unknown BLOCKING code must still block; stdout:\n%s", stdout)
	}
	for _, want := range []string{"a-code-from-the-future", "Something new is wrong", "no CLI route for this problem yet"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("unknown code must still be reported with %q, got:\n%s", want, stdout)
		}
	}
}

// TestDoctorUnknownSeverityIsNotBlocking: an unrecognised severity is grouped
// with the advisories rather than failing every release.
func TestDoctorUnknownSeverityIsNotBlocking(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("missing-icon", "Missing icon (required before publishing)", "catastrophic"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("an unrecognised severity must not block a release, got %v; stdout:\n%s", err, stdout)
	}
	// It is grouped, not hidden: the code and its verbatim severity still print.
	if !strings.Contains(stdout, "missing-icon") {
		t.Errorf("an unrecognised severity must still be reported, got:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Selection, and the empty case.
// ---------------------------------------------------------------------------

// TestDoctorSlugSelectsOneApp: the positional argument narrows to one row, and
// the OTHER app must not appear.
func TestDoctorSlugSelectsOneApp(t *testing.T) {
	newDoctorServer(t,
		doctorRow(docSlugA, docListingA, "draft", "owner", nil,
			doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")),
		doctorRow(docSlugB, docListingB, "approved", "owner", ptr(docBlockA)),
	)
	stdout, _, err := run(t, "app", "doctor", docSlugB)
	if err != nil {
		t.Fatalf("the selected app is clean, so this must exit 0, got %v; stdout:\n%s", err, stdout)
	}
	if strings.Contains(stdout, docSlugA) || strings.Contains(stdout, "missing-icon") {
		t.Errorf("selecting %q must not report %q, got:\n%s", docSlugB, docSlugA, stdout)
	}
	if !strings.Contains(stdout, docSlugB) {
		t.Errorf("the selected app is missing from the report:\n%s", stdout)
	}
}

// TestDoctorSlugSelectionNormalises: the slug is TYPED, so a padded or mis-cased
// spelling reaches the filter routinely. It goes through appapi.SameSlug, the
// shared predicate, not an exact compare.
func TestDoctorSlugSelectionNormalises(t *testing.T) {
	for _, typed := range []string{docSlugA, strings.ToUpper(docSlugA), "  " + docSlugA + "  "} {
		t.Run(strings.TrimSpace(typed), func(t *testing.T) {
			newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil))
			stdout, _, err := run(t, "app", "doctor", typed)
			if err != nil {
				t.Fatalf("%q should resolve to %q, got %v; stdout:\n%s", typed, docSlugA, err, stdout)
			}
			if !strings.Contains(stdout, docSlugA) {
				t.Errorf("%q did not select the app:\n%s", typed, stdout)
			}
		})
	}
}

// TestDoctorUnknownSlugIsNotFoundNotAPass: "you own no app called that" and
// "that app is fine" are opposite answers, and a release gate must not confuse
// them. Exit 4, not 0.
func TestDoctorUnknownSlugIsNotFoundNotAPass(t *testing.T) {
	newDoctorServer(t,
		doctorRow(docSlugA, docListingA, "draft", "owner", nil),
		doctorRow(docSlugB, docListingB, "approved", "owner", ptr(docBlockA)),
	)
	stdout, _, err := run(t, "app", "doctor", "no-such-app-of-mine")
	if err == nil {
		t.Fatalf("an unknown slug must NOT read as a clean run; stdout:\n%s", stdout)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("an unknown slug must be ErrNotFound (exit 4), got %T: %v", err, err)
	}
	if errors.Is(err, ErrListingBlocked) {
		t.Error("an unknown slug is not a blocking-problem verdict — the two exit codes mean different things")
	}
	// It names what the caller CAN work on, so the mistake is fixable from the
	// message alone.
	for _, want := range []string{docSlugA, docSlugB} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must list the apps you can work on (missing %q): %v", want, err)
		}
	}
}

// TestDoctorNoListingsIsASentenceAndExitsZero: an empty result is a real answer.
// A blank screen is indistinguishable from a read that failed.
func TestDoctorNoListingsIsASentenceAndExitsZero(t *testing.T) {
	newDoctorServer(t)
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("no listings must exit 0, got %v", err)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("an empty run printed NOTHING — that is indistinguishable from a swallowed read")
	}
	if !strings.Contains(stdout, "No App listings to check") {
		t.Errorf("an empty run must say so, got:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// The request ledger — the D3 measurement.
// ---------------------------------------------------------------------------

// TestDoctorIssuesExactlyOneRequestAndItIsListMine pins the whole request set.
//
// 🔴 THIS IS WHAT DECIDES WHETHER `doctor` TRIPS THE `getAssetScanStatuses`
// OWNER FILTER. That proc filters `Image.userId = caller` for non-moderators, so
// an asset attached by a COLLABORATOR, or one surviving an ownership transfer,
// is silently omitted from its answer. `listMine`'s own scan batch
// (`loadListingAssetScansBatch`) applies no such filter, so the DIAGNOSIS is
// complete either way. The gap can only bite a command that goes on to ask
// `getAssetScanStatuses` which asset row is blocked — and this ledger is the
// proof that this command never does.
//
// A ledger rather than a "did not call X" check: a set assertion fails when the
// set GROWS as well as when it shrinks, so a later change that adds a second
// read has to come here and say so.
func TestDoctorIssuesExactlyOneRequestAndItIsListMine(t *testing.T) {
	srv := newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("blocked-media", "Replace the blocked icon before it can publish", "blocking"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err == nil {
		t.Fatalf("blocked-media is blocking; stdout:\n%s", stdout)
	}
	got := srv.seen()
	// Positive control: a zero-length ledger would make the assertion below
	// pass for a command that made no request at all.
	if len(got) == 0 {
		t.Fatal("the request ledger is EMPTY — the command made no request, so this test measures nothing")
	}
	if len(got) != 1 || got[0] != listMinePath {
		t.Errorf("`app doctor` issued %v.\nIt must issue EXACTLY [%s]. If a second read was added deliberately, "+
			"re-derive the getAssetScanStatuses owner-filter argument in this test's doc comment before widening it.",
			got, listMinePath)
	}
	// And the finding is still complete: the SERVER's label names the slot, so
	// the CLI never needed the per-asset read to be actionable.
	if !strings.Contains(stdout, "Replace the blocked icon") {
		t.Errorf("the blocked-media label (which is where the asset KIND lives) must survive into the report:\n%s", stdout)
	}
}

// TestDoctorSendsNoInputParameter: `appListings.listMine` declares no input
// schema, so the CLI sends no `?input=`. Pinned because the alternative
// (`?input={"json":null}`) is a value the server never has to accept, and the
// difference is invisible in every other test.
func TestDoctorSendsNoInputParameter(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw = r.URL.RawQuery
		trpcData(w, []map[string]any{})
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)
	if _, _, err := run(t, "app", "doctor"); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if raw != "" {
		t.Errorf("listMine was sent with a query string %q — it takes no input, so none should be sent", raw)
	}
}

// ---------------------------------------------------------------------------
// Help.
// ---------------------------------------------------------------------------

// TestDoctorHelpDocumentsTheExitCodes: the exit code is the command's product,
// and `--help` is the contract a reader has offline.
func TestDoctorHelpDocumentsTheExitCodes(t *testing.T) {
	out, _, err := run(t, "app", "doctor", "--help")
	if err != nil {
		t.Fatalf("app doctor --help: %v", err)
	}
	for _, want := range []string{
		"EXIT CODES",
		"1 when a blocking problem was found on a listing that can still\npublish",
		"0 otherwise",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("`app doctor --help` must document the exit codes (missing %q):\n%s", want, out)
		}
	}
	// 🔴 THE OLD WORDING IS BANNED, NOT MERELY REPLACED. `--help` used to say "1
	// when ANY blocking problem was found", which was true until delisted
	// listings stopped gating and is now an OVER-BROAD claim about the exit
	// code: a reader who believes it will read a 0 on a delisted app as a bug in
	// the tool. Keeping it as a prohibition is what stops a future edit
	// shortening the sentence straight back into the falsehood.
	if strings.Contains(out, "ANY blocking problem") {
		t.Errorf("`app doctor --help` claims ANY blocking problem exits 1 — delisted listings do not gate:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// doctorPayloadShape mirrors what `--json` emits. Hand-written rather than
// reusing the production type: a test that unmarshals into the very struct it
// is checking agrees with any renaming of a field.
type doctorPayloadShape struct {
	OK   bool `json:"ok"`
	Apps []struct {
		Slug         string  `json:"slug"`
		Name         string  `json:"name"`
		AppListingID string  `json:"appListingId"`
		AppBlockID   *string `json:"appBlockId"`
		Status       string  `json:"status"`
		Role         string  `json:"role"`
		Kind         string  `json:"kind"`
		Delisted     bool    `json:"delisted"`
		Blocking     []struct {
			Code     string `json:"code"`
			Label    string `json:"label"`
			Severity string `json:"severity"`
			Fix      string `json:"fix"`
		} `json:"blocking"`
		Advisory []struct {
			Code     string `json:"code"`
			Label    string `json:"label"`
			Severity string `json:"severity"`
			Fix      string `json:"fix"`
		} `json:"advisory"`
	} `json:"apps"`
	Summary struct {
		Apps      int  `json:"apps"`
		Blocking  int  `json:"blocking"`
		Advisory  int  `json:"advisory"`
		Gating    int  `json:"gating"`
		Delisted  int  `json:"delisted"`
		Truncated bool `json:"truncated"`
	} `json:"summary"`
}

func decodeDoctorJSON(t *testing.T, stdout string) doctorPayloadShape {
	t.Helper()
	var got doctorPayloadShape
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not the doctor payload (%v); stdout was:\n%s", err, stdout)
	}
	return got
}

// codesOf works over either problem array (they are structurally identical but
// are distinct anonymous types, so this takes the codes via a tiny interface of
// its own rather than a shared type).
func codesOf[T any](rows []T) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		b, _ := json.Marshal(r)
		var v struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(b, &v)
		out = append(out, v.Code)
	}
	return out
}

// allEightCodes is the code vocabulary, spelled out here rather than read from
// app_doctor.go's constants: a table derived from the thing it checks agrees
// with any change to it.
func allEightCodes() []string {
	return []string{
		"missing-icon", "missing-cover", "no-screenshots",
		"empty-description", "empty-tagline", "empty-category",
		"blocked-media", "scanning-media",
	}
}

// problemWithCode builds the minimal problem doctorRemedy reads: only the code
// selects a remedy, so the label and severity are deliberately left empty —
// a remedy that needed either would fail loudly here rather than quietly work.
func problemWithCode(code string) appapi.ListingProblem {
	return appapi.ListingProblem{Code: code}
}

// resolveCommandRef checks that a `civitai …` string names a real command, with
// no leftover arguments.
func resolveCommandRef(root *cobra.Command, ref string) error {
	parts := strings.Fields(ref)
	if len(parts) < 2 || parts[0] != "civitai" {
		return fmt.Errorf("%q is not a civitai command reference", ref)
	}
	args := parts[1:]
	cmd, rest, err := root.Find(args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("%q leaves unresolved arguments %v", ref, rest)
	}
	if cmd.Name() != args[len(args)-1] {
		return fmt.Errorf("%q resolved to %q, not to the command it names", ref, cmd.Name())
	}
	return nil
}

// envBaseURL is the base URL the current test env points the CLI at — the same
// value the editor URL is built from.
func envBaseURL(t *testing.T) string {
	t.Helper()
	v := os.Getenv("CIVITAI_BASE_URL")
	if v == "" {
		t.Fatal("CIVITAI_BASE_URL is unset — listingEnv did not run, so the URL assertions below would compare against nothing")
	}
	return strings.TrimRight(v, "/")
}

// ---------------------------------------------------------------------------
// Delisted listings: reported, but they do not set the exit code.
// ---------------------------------------------------------------------------

// TestDoctorDelistedListingsDoNotSetTheExitCode is the THREE-ARM table, and the
// third arm is the one that matters.
//
// 🔴 THE MIXED ARM IS WHAT A NAIVE FILTER GETS WRONG. An implementation that
// drops delisted rows from the payload entirely passes arms 1 and 2 and passes
// arm 3 too — but an implementation that computes the verdict from "are there
// any delisted apps" instead of "are there gating problems" passes arms 1 and 2
// and FAILS arm 3. Two arms cannot tell those apart.
//
// Measured motivation, not a hypothetical: on production 2026-08-24 this
// account held 21 listings with 11 blocking problems, TEN of them on `removed`
// apps. Before this rule the no-arg form exited 1 forever.
func TestDoctorDelistedListingsDoNotSetTheExitCode(t *testing.T) {
	blocked := func(slug, id, status string) map[string]any {
		return doctorRow(slug, id, status, "owner", nil,
			doctorProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"))
	}

	cases := []struct {
		name    string
		rows    []map[string]any
		wantErr bool
		why     string
		// wantGating / wantBlocking are asserted from --json in the same run, so
		// the arm pins WHY it exited that way and not merely that it did.
		wantBlocking int
		wantGating   int
	}{
		{
			name:         "a removed listing with blocking problems exits 0",
			rows:         []map[string]any{blocked(docSlugC, docListingC, "removed")},
			wantErr:      false,
			why:          "the publish floor is meaningless for a delisted app",
			wantBlocking: 1,
			wantGating:   0,
		},
		{
			name:         "a live listing with blocking problems exits 1",
			rows:         []map[string]any{blocked(docSlugA, docListingA, "draft")},
			wantErr:      true,
			why:          "this is the case the gate exists for",
			wantBlocking: 1,
			wantGating:   1,
		},
		{
			name: "BOTH present exits 1",
			rows: []map[string]any{
				blocked(docSlugC, docListingC, "removed"),
				blocked(docSlugA, docListingA, "draft"),
			},
			wantErr:      true,
			why:          "a delisted app must not MASK a real one — the naive filter's failure",
			wantBlocking: 2,
			wantGating:   1,
		},
		{
			name: "a removed listing next to a CLEAN live one still exits 0",
			rows: []map[string]any{
				blocked(docSlugC, docListingC, "removed"),
				doctorRow(docSlugB, docListingB, "approved", "owner", ptr(docBlockA)),
			},
			wantErr:      false,
			why:          "nothing publishable is blocked",
			wantBlocking: 1,
			wantGating:   0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newDoctorServer(t, tc.rows...)
			stdout, _, err := run(t, "app", "doctor", "--json")
			if tc.wantErr && err == nil {
				t.Fatalf("expected a non-zero verdict — %s; stdout:\n%s", tc.why, stdout)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected exit 0 — %s; got %v; stdout:\n%s", tc.why, err, stdout)
			}
			if tc.wantErr && !errors.Is(err, ErrListingBlocked) {
				t.Errorf("the verdict must be ErrListingBlocked, got %T: %v", err, err)
			}
			got := decodeDoctorJSON(t, stdout)
			if got.Summary.Blocking != tc.wantBlocking {
				t.Errorf("summary.blocking = %d, want %d — every blocking problem must still be COUNTED",
					got.Summary.Blocking, tc.wantBlocking)
			}
			if got.Summary.Gating != tc.wantGating {
				t.Errorf("summary.gating = %d, want %d — gating is what sets the exit code",
					got.Summary.Gating, tc.wantGating)
			}
			if got.OK != (tc.wantGating == 0) {
				t.Errorf("ok = %v with gating = %d — `ok` must follow gating, not blocking", got.OK, got.Summary.Gating)
			}
		})
	}
}

// TestDoctorDelistedRowsAreStillVisible: the verdict shrinks, the report does
// not. Hiding a population by default is how it stops existing for anyone.
func TestDoctorDelistedRowsAreStillVisible(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugC, docListingC, "removed", "owner", nil,
		doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
		doctorProblem("empty-tagline", "Missing tagline", "advisory"),
	))
	stdout, _, err := run(t, "app", "doctor")
	if err != nil {
		t.Fatalf("a delisted-only run must exit 0, got %v; stdout:\n%s", err, stdout)
	}
	for _, want := range []string{
		docSlugC,                   // the row itself
		"missing-icon",             // its blocking finding, not swallowed
		"empty-tagline",            // and its advisory
		"BLOCKING",                 // still under the honest heading
		"Delisted",                 // announced as a section
		"do NOT set the exit code", // and the rule stated where the reader meets it
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("a delisted app must still be fully reported (missing %q):\n%s", want, stdout)
		}
	}
	// The trailing explanation of the discrepancy.
	if !strings.Contains(stdout, "1 of them are delisted; 0 blocking problem(s) on publishable listings set the exit code.") {
		t.Errorf("the summary must explain why the exit code disagrees with the blocking count:\n%s", stdout)
	}
}

// TestDoctorDelistedRowsSortLast pins the ORDER, in the payload both renderings
// read from. A reader scanning the top of the report must meet the apps that
// can actually block a release first.
func TestDoctorDelistedRowsSortLast(t *testing.T) {
	// Sent delisted-FIRST on the wire, so passing requires real ordering work.
	newDoctorServer(t,
		doctorRow(docSlugC, docListingC, "removed", "owner", nil),
		doctorRow(docSlugA, docListingA, "draft", "owner", nil),
		doctorRow(docSlugB, docListingB, "approved", "owner", ptr(docBlockA)),
	)
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	got := decodeDoctorJSON(t, stdout)
	var order []string
	for _, a := range got.Apps {
		order = append(order, a.Slug)
	}
	want := []string{docSlugA, docSlugB, docSlugC}
	if !equalStrings(order, want) {
		t.Errorf("app order = %v, want %v (gating first, delisted last, stable within each group)", order, want)
	}
}

// TestDoctorUnknownStatusStillGates: only an explicit `removed` is excluded.
// "We could not tell that it is delisted" is not "it is delisted", and the two
// directions are not symmetric — wrongly excluding means a real blocking problem
// silently stops failing the build.
func TestDoctorUnknownStatusStillGates(t *testing.T) {
	for _, status := range []string{"draft", "pending", "approved", "rejected", "a-status-from-the-future", ""} {
		t.Run("status="+status, func(t *testing.T) {
			newDoctorServer(t, doctorRow(docSlugA, docListingA, status, "owner", nil,
				doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
			stdout, _, err := run(t, "app", "doctor")
			if err == nil {
				t.Fatalf("status %q is not `removed`, so a blocking problem must still gate; stdout:\n%s", status, stdout)
			}
		})
	}
	// POSITIVE CONTROL on the loop: the ONE status that is excluded must really
	// be excluded, or every assertion above is a fact about a predicate that
	// never returns false.
	t.Run("control: removed IS excluded", func(t *testing.T) {
		newDoctorServer(t, doctorRow(docSlugA, docListingA, "removed", "owner", nil,
			doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
		if _, _, err := run(t, "app", "doctor"); err != nil {
			t.Fatalf("control: `removed` must be excluded, got %v — the predicate never returns false", err)
		}
	})
}

// TestDoctorDelistedStatusMatchIsNormalised: the status is a server string and
// the CLI compares it. Case and padding must not silently turn a delisted
// listing back into a gating one.
func TestDoctorDelistedStatusMatchIsNormalised(t *testing.T) {
	for _, spelling := range []string{"removed", "REMOVED", "Removed", " removed "} {
		t.Run(strings.TrimSpace(spelling), func(t *testing.T) {
			newDoctorServer(t, doctorRow(docSlugA, docListingA, spelling, "owner", nil,
				doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
			stdout, _, err := run(t, "app", "doctor")
			if err != nil {
				t.Errorf("status %q must read as delisted, got %v; stdout:\n%s", spelling, err, stdout)
			}
		})
	}
}

// TestDoctorHelpDocumentsTheDelistedRule: the exit code is the command's
// product, and a rule that changes it belongs in the contract a reader has
// offline.
func TestDoctorHelpDocumentsTheDelistedRule(t *testing.T) {
	out, _, err := run(t, "app", "doctor", "--help")
	if err != nil {
		t.Fatalf("app doctor --help: %v", err)
	}
	for _, want := range []string{"DELISTED LISTINGS", "'removed'", "do NOT set the exit code"} {
		if !strings.Contains(out, want) {
			t.Errorf("`app doctor --help` must document the delisted rule (missing %q):\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------------
// The three TEXT codes are KIND-AWARE. An onsite app's copy is manifest-governed.
// ---------------------------------------------------------------------------

// textCodes are the three problems whose fix route depends on the listing kind.
func textCodes() []map[string]any {
	return []map[string]any{
		doctorProblem("empty-description", "Missing description", "advisory"),
		doctorProblem("empty-tagline", "Missing tagline", "advisory"),
		doctorProblem("empty-category", "Missing category", "advisory"),
	}
}

// doctorRowKind is doctorRow with the listing KIND under the caller's control.
func doctorRowKind(slug, listingID, status, kind string, problems ...map[string]any) map[string]any {
	r := doctorRow(slug, listingID, status, "owner", nil, problems...)
	r["kind"] = kind
	return r
}

// TestDoctorOnsiteTextRemedyNamesTheManifest is the correctness fix.
//
// 🔴 THE BROWSER-EDITOR ADVICE IS WRONG FOR AN ONSITE APP, and wrong in the
// silent direction. `(3b-sync)` in
// `<civitai>/src/server/services/blocks/publish-request.service.ts:2742-2800`
// overwrites name/tagline/description/category from the manifest on EVERY
// subsequent-version moderator approve, scoped `kind: 'onsite'` — its own
// comment says these fields "have NO author surface other than the manifest".
// So an onsite author who followed the old advice made an edit the platform
// reverted at the next approve, and `doctor` reported the same problem again
// with nothing explaining why.
func TestDoctorOnsiteTextRemedyNamesTheManifest(t *testing.T) {
	newDoctorServer(t, doctorRowKind(docSlugA, docListingA, "approved", "onsite", textCodes()...))
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err != nil {
		t.Fatalf("advisory-only must exit 0: %v", err)
	}
	got := decodeDoctorJSON(t, stdout)
	if len(got.Apps) != 1 || len(got.Apps[0].Advisory) != 3 {
		t.Fatalf("want 1 app with 3 advisories, got %+v", got.Apps)
	}
	for _, p := range got.Apps[0].Advisory {
		if !strings.Contains(p.Fix, "block.manifest.json") {
			t.Errorf("onsite %q must be fixed in the manifest, got %q", p.Code, p.Fix)
		}
		if !strings.Contains(p.Fix, "civitai app submit") {
			t.Errorf("onsite %q must name the resubmit step, got %q", p.Code, p.Fix)
		}
		// 🔴 THE WRONG ROUTE MUST BE ABSENT, not merely outranked. A fix line
		// that names BOTH still sends the author to the browser.
		if strings.Contains(p.Fix, "/apps/listing/") {
			t.Errorf("onsite %q still points at the browser editor, whose edit `(3b-sync)` reverts: %q", p.Code, p.Fix)
		}
	}
}

// TestDoctorOffsiteTextRemedyStillPointsAtTheEditor is the other arm, and
// without it the test above is equally true of a build that names the manifest
// for everyone — which would be wrong: an off-site listing's copy is
// AUTHOR-supplied through the submit wizard, not manifest-governed, and such an
// app has no block.manifest.json at all.
func TestDoctorOffsiteTextRemedyStillPointsAtTheEditor(t *testing.T) {
	newDoctorServer(t, doctorRowKind(docSlugB, docListingB, "approved", "offsite", textCodes()...))
	base := envBaseURL(t)
	stdout, _, err := run(t, "app", "doctor", "--json")
	if err != nil {
		t.Fatalf("advisory-only must exit 0: %v", err)
	}
	got := decodeDoctorJSON(t, stdout)
	for _, p := range got.Apps[0].Advisory {
		if !strings.Contains(p.Fix, base+"/apps/listing/"+docListingB+"/edit") {
			t.Errorf("offsite %q must point at the listing editor, got %q", p.Code, p.Fix)
		}
		if strings.Contains(p.Fix, "block.manifest.json") {
			t.Errorf("offsite %q names a manifest an off-site app does not have: %q", p.Code, p.Fix)
		}
	}
}

// TestDoctorMediaRemediesAreKindIndependent: only the THREE TEXT codes branch.
// The media procs are listing-keyed and seat-aware and work for both kinds, so
// a kind gate on them would refuse a route that works.
func TestDoctorMediaRemediesAreKindIndependent(t *testing.T) {
	media := []map[string]any{
		doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"),
		doctorProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory"),
	}
	fixes := map[string][]string{}
	for _, kind := range []string{"onsite", "offsite"} {
		newDoctorServer(t, doctorRowKind(docSlugA, docListingA, "draft", kind, media...))
		stdout, _, _ := run(t, "app", "doctor", "--json")
		got := decodeDoctorJSON(t, stdout)
		for _, p := range append(got.Apps[0].Blocking, got.Apps[0].Advisory...) {
			fixes[kind] = append(fixes[kind], p.Code+"="+p.Fix)
		}
	}
	if !equalStrings(fixes["onsite"], fixes["offsite"]) {
		t.Errorf("media remedies differ by kind, but the asset procs work for both:\n onsite=%v\noffsite=%v",
			fixes["onsite"], fixes["offsite"])
	}
	// Positive control: the comparison saw a non-empty population.
	if len(fixes["onsite"]) != 2 {
		t.Fatalf("collected %d media remedies, want 2 — the comparison above is vacuous", len(fixes["onsite"]))
	}
}

// TestDoctorUnknownKindTakesTheRecoverableArm: an absent or unrecognised kind
// must NOT take the manifest arm. Naming a manifest an app may not have is
// confusing but self-correcting; the reverse — sending an onsite author to the
// browser — is the silent failure this fix exists to remove, so the DEFAULT is
// the recoverable mistake.
func TestDoctorUnknownKindTakesTheRecoverableArm(t *testing.T) {
	for _, kind := range []string{"", "a-kind-from-the-future"} {
		t.Run("kind="+kind, func(t *testing.T) {
			newDoctorServer(t, doctorRowKind(docSlugA, docListingA, "draft", kind,
				doctorProblem("empty-tagline", "Missing tagline", "advisory")))
			stdout, _, _ := run(t, "app", "doctor", "--json")
			got := decodeDoctorJSON(t, stdout)
			fix := got.Apps[0].Advisory[0].Fix
			if strings.Contains(fix, "block.manifest.json") {
				t.Errorf("kind %q must not take the manifest arm: %q", kind, fix)
			}
		})
	}
	// POSITIVE CONTROL: `onsite` really does take it, or the negatives above are
	// a fact about a predicate that never returns true.
	newDoctorServer(t, doctorRowKind(docSlugA, docListingA, "draft", "onsite",
		doctorProblem("empty-tagline", "Missing tagline", "advisory")))
	stdout, _, _ := run(t, "app", "doctor", "--json")
	if fix := decodeDoctorJSON(t, stdout).Apps[0].Advisory[0].Fix; !strings.Contains(fix, "block.manifest.json") {
		t.Fatalf("control: `onsite` did not take the manifest arm (%q) — the negatives prove nothing", fix)
	}
}

// TestDoctorKindMatchIsNormalised: `kind` is a server string the CLI compares,
// and a spelling change must not silently re-route the advice.
func TestDoctorKindMatchIsNormalised(t *testing.T) {
	for _, spelling := range []string{"onsite", "ONSITE", "Onsite", " onsite "} {
		t.Run(strings.TrimSpace(spelling), func(t *testing.T) {
			newDoctorServer(t, doctorRowKind(docSlugA, docListingA, "draft", spelling,
				doctorProblem("empty-category", "Missing category", "advisory")))
			stdout, _, _ := run(t, "app", "doctor", "--json")
			if fix := decodeDoctorJSON(t, stdout).Apps[0].Advisory[0].Fix; !strings.Contains(fix, "block.manifest.json") {
				t.Errorf("kind %q must read as onsite, got fix %q", spelling, fix)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// blocked-media: the three slots do NOT share a remedy.
// ---------------------------------------------------------------------------

// TestBlockedMediaRemedyIsSufficientPerSlot is the correctness fix, and the
// reason the previous guard could not catch it is worth stating:
// TestDoctorFixAdviceNamesOnlyCommandsThatExist asserts the named commands
// RESOLVE IN THE COBRA TREE. Resolving is not sufficiency. `add-screenshot` is a
// real command that does not fix a blocked screenshot.
//
// 🔴 `add-screenshot` APPENDS. `assertAssetsScanClean`
// (`<civitai>/src/server/services/blocks/app-listing-assets.service.ts:857-894`)
// refuses go-live while ANY attached screenshot is Blocked, so adding a second
// image leaves the listing exactly as blocked and the author out of ideas. Only
// `rm-screenshot` clears it — and its `alsc_` id is not in `listMine`, so the
// read that prints it has to be named too.
func TestBlockedMediaRemedyIsSufficientPerSlot(t *testing.T) {
	cases := []struct {
		label      string
		mustSay    []string
		mustNotSay []string
	}{
		{
			// set-icon OVERWRITES the slot, dereferencing the blocked Image.
			label:      "Replace the blocked icon before it can publish",
			mustSay:    []string{"set-icon"},
			mustNotSay: []string{"add-screenshot", "rm-screenshot", "set-cover"},
		},
		{
			label:      "Replace the blocked cover before it can publish",
			mustSay:    []string{"set-cover"},
			mustNotSay: []string{"add-screenshot", "rm-screenshot", "set-icon"},
		},
		{
			// 🔴 THREE CLAUSES, AND `submit-revision` IS THE ONE THAT WAS
			// UNASSERTED. A mutation sweep caught it: deleting the
			// submit-revision clause left this test green, because it only
			// demanded the removal and the read. On an APPROVED listing
			// `rm-screenshot` lands in the open shadow and is deliberately NOT
			// submitted, and `listMine` reads the PARENT — so without that
			// clause the author removes the asset, `doctor` keeps reporting
			// `blocked-media`, keeps exiting 1, and they have nothing left to
			// try. Asserting two of three clauses is asserting an insufficient
			// remedy.
			label:      "Replace the blocked screenshot before it can publish",
			mustSay:    []string{"rm-screenshot", "civitai app listing status", "submit-revision"},
			mustNotSay: []string{"add-screenshot"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			fix := doctorRemedy(
				appapi.ListingProblem{Code: "blocked-media", Label: tc.label, Severity: "blocking"},
				docSlugA, "https://example.invalid/x", "offsite")
			for _, w := range tc.mustSay {
				if !strings.Contains(fix, w) {
					t.Errorf("missing %q in: %s", w, fix)
				}
			}
			for _, w := range tc.mustNotSay {
				if strings.Contains(fix, w) {
					t.Errorf("must not advise %q — it does not clear this slot: %s", w, fix)
				}
			}
		})
	}
}

// TestBlockedMediaUnknownLabelKeepsTheRemoval: a re-worded label must degrade to
// an arm that is still SUFFICIENT. Losing precision is acceptable; losing the
// removal is the defect returning.
func TestBlockedMediaUnknownLabelKeepsTheRemoval(t *testing.T) {
	fix := doctorRemedy(
		appapi.ListingProblem{Code: "blocked-media", Label: "this asset was rejected by the scanner", Severity: "blocking"},
		docSlugA, "https://example.invalid/x", "offsite")
	for _, want := range []string{"rm-screenshot", "civitai app listing status", "submit-revision"} {
		if !strings.Contains(fix, want) {
			t.Errorf("the fallback arm must still name %q — losing precision is acceptable, losing "+
				"SUFFICIENCY is the defect returning: %s", want, fix)
		}
	}
	if strings.Contains(fix, "add-screenshot") {
		t.Errorf("the fallback must not advise appending, which never clears a blocked row: %s", fix)
	}
}

// TestBlockedMediaKindExtractionIsWordBased, with a negative control so the
// helper is not merely returning the first thing it sees.
func TestBlockedMediaKindExtractionIsWordBased(t *testing.T) {
	for label, want := range map[string]string{
		"Replace the blocked icon before it can publish":       "icon",
		"Replace the blocked COVER before it can publish":      "cover",
		"replace the blocked Screenshot before it can publish": "screenshot",
		"something else entirely":                              "",
	} {
		if got := blockedMediaKind(label); got != want {
			t.Errorf("blockedMediaKind(%q) = %q, want %q", label, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// severity normalisation, and the page cap.
// ---------------------------------------------------------------------------

// TestDoctorSeverityIsNormalised: a cased severity must not silently become an
// advisory. Latent today (the server's union is lowercase), and fixed because
// unknown STATUS gated loudly while unknown SEVERITY failed quietly.
func TestDoctorSeverityIsNormalised(t *testing.T) {
	for _, spelling := range []string{"blocking", "BLOCKING", "Blocking", " blocking "} {
		t.Run(strings.TrimSpace(spelling), func(t *testing.T) {
			newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
				doctorProblem("missing-icon", "Missing icon (required before publishing)", spelling)))
			stdout, _, err := run(t, "app", "doctor", "--json")
			if err == nil {
				t.Fatalf("severity %q must gate; stdout:\n%s", spelling, stdout)
			}
			got := decodeDoctorJSON(t, stdout)
			if len(got.Apps[0].Blocking) != 1 || got.Summary.Gating != 1 || got.OK {
				t.Errorf("severity %q filed wrong: blocking=%d gating=%d ok=%v — a payload that files a "+
					"'blocking' finding under advisory[] while echoing its severity is a contradiction the CLI made",
					spelling, len(got.Apps[0].Blocking), got.Summary.Gating, got.OK)
			}
		})
	}
	// POSITIVE CONTROL: an unrecognised WORD is still advisory. Normalisation
	// makes casing safe; it cannot make a new vocabulary word meaningful.
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("missing-icon", "Missing icon", "catastrophic")))
	if _, _, err := run(t, "app", "doctor"); err != nil {
		t.Fatalf("control: an unrecognised severity word must NOT gate, got %v", err)
	}
}

// bulkRows builds n clean listings, plus (optionally) a blocking one at the END
// so a by-slug lookup has to reach past the head of the page.
func bulkRows(n int, tailSlug string) []map[string]any {
	rows := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, doctorRow(fmt.Sprintf("bulk-app-%03d", i), fmt.Sprintf("apl_BULK%03d", i),
			"approved", "owner", nil))
	}
	if tailSlug != "" && n > 0 {
		rows[n-1] = doctorRow(tailSlug, "apl_TAIL", "draft", "owner", nil,
			doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"))
	}
	return rows
}

// TestDoctorTruncationBoundary is the Cap-1 / Cap / Cap+1 table the sibling
// guard carries and this one lacked.
//
// 🔴 A SINGLE AT-CAP CASE CANNOT TELL `>=` FROM `>`, and the off-by-one is the
// likely drift: the commonest truncated read is EXACTLY at the cap, because that
// is what a clamp produces.
func TestDoctorTruncationBoundary(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want bool
	}{
		{appapi.ListMineCap - 1, false},
		{appapi.ListMineCap, true},
		{appapi.ListMineCap + 1, true},
	} {
		t.Run(fmt.Sprintf("n=%d", tc.n), func(t *testing.T) {
			newDoctorServer(t, bulkRows(tc.n, "")...)
			stdout, stderr, err := run(t, "app", "doctor", "--json")
			if err != nil {
				t.Fatalf("all clean, so exit 0: %v", err)
			}
			if got := decodeDoctorJSON(t, stdout).Summary.Truncated; got != tc.want {
				t.Errorf("n=%d truncated=%v, want %v", tc.n, got, tc.want)
			}
			if strings.Contains(stderr, "caps this read") != tc.want {
				t.Errorf("n=%d caveat presence = %v, want %v", tc.n, !tc.want, tc.want)
			}
		})
	}
}

// TestDoctorTruncationIsReportedOnTheBySlugPathToo is the defect itself.
//
// 🔴 `truncated` USED TO BE COMPUTED FROM THE FILTERED SLICE, so a by-slug run
// saw len(rows)==1 and reported false — on the exact path the caveat used to
// tell the reader to use. The flag is a property of the SERVER'S PAGE.
// `listMine` takes no input (TestDoctorSendsNoInputParameter), so every by-slug
// run is a capped read filtered client-side.
func TestDoctorTruncationIsReportedOnTheBySlugPathToo(t *testing.T) {
	newDoctorServer(t, bulkRows(appapi.ListMineCap, "tail-app")...)
	stdout, stderr, err := run(t, "app", "doctor", "tail-app", "--json")
	if err == nil {
		t.Fatalf("tail-app is blocked, so this must exit non-zero; stdout:\n%s", stdout)
	}
	got := decodeDoctorJSON(t, stdout)
	if len(got.Apps) != 1 {
		t.Fatalf("by-slug must select exactly one app, got %d", len(got.Apps))
	}
	if !got.Summary.Truncated {
		t.Errorf("a by-slug run reads the SAME capped page — truncated must be true, not false-by-construction")
	}
	if !strings.Contains(stderr, "caps this read") {
		t.Errorf("the caveat must print on the by-slug path:\n%s", stderr)
	}
	// 🔴 The caveat must NOT quote the FILTERED count. It used to interpolate
	// Summary.Apps, which is 1 here, producing "the server returned 1 listings"
	// about a 200-row page.
	if strings.Contains(stderr, "returned 1 listings") {
		t.Errorf("the caveat quotes the filtered count, not the cap:\n%s", stderr)
	}
	// And it must not send the reader to the path it was blind on.
	if strings.Contains(stderr, "Check a specific app with") {
		t.Errorf("the caveat still directs the reader at the by-slug path as a remedy:\n%s", stderr)
	}
}

// TestDoctorNotFoundOnACappedPageIsNotConclusive.
//
// 🔴 THE WORST ARM. A slug the caller REALLY OWNS, sitting beyond the cap,
// resolves to "no listing of yours is called that" — a confidently wrong answer
// about their own app. That is the same silent-pass class this caveat exists to
// close, relocated from exit 0 to exit 4, so the refusal has to say it is not
// conclusive.
func TestDoctorNotFoundOnACappedPageIsNotConclusive(t *testing.T) {
	newDoctorServer(t, bulkRows(appapi.ListMineCap, "")...)
	_, _, err := run(t, "app", "doctor", "an-app-past-the-cap")
	if err == nil {
		t.Fatal("expected a not-found")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("still exit 4, got %T: %v", err, err)
	}
	for _, want := range []string{"CAPS this read", "not conclusive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("a not-found off a capped page must say it is uncertain (missing %q): %v", want, err)
		}
	}
	// NEGATIVE CONTROL: below the cap the answer IS conclusive and must not be
	// hedged, or the hedge becomes noise on every miss.
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil))
	_, _, err2 := run(t, "app", "doctor", "no-such-app-of-mine")
	if err2 == nil {
		t.Fatal("control: expected a not-found")
	}
	if strings.Contains(err2.Error(), "not conclusive") {
		t.Errorf("below the cap the answer is conclusive and must not be hedged: %v", err2)
	}
}

// TestDoctorDoesNotCryTruncationBelowTheCap is the negative control: a caveat
// that always prints is one nobody reads.
func TestDoctorDoesNotCryTruncationBelowTheCap(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "approved", "owner", nil))
	stdout, stderr, err := run(t, "app", "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if decodeDoctorJSON(t, stdout).Summary.Truncated {
		t.Error("one listing is not a truncated page")
	}
	if strings.Contains(stderr, "caps this read") {
		t.Errorf("the cap caveat must not print below the cap:\n%s", stderr)
	}
}

// TestListMineCapMatchesTheServer is the drift guard the sibling constant
// carries and this one lacked. It cannot reach the server, so it pins the value
// and names where to re-read it — a stale cap silently changes what "truncated"
// means in both directions.
func TestListMineCapMatchesTheServer(t *testing.T) {
	const want = 200
	if appapi.ListMineCap != want {
		t.Errorf("ListMineCap = %d, want %d. Re-read `MY_APP_LISTINGS_LIMIT` in "+
			"<civitai>/src/server/services/blocks/app-access.service.ts and change this pin deliberately: "+
			"a cap that is too HIGH never warns on a truncated page, and one that is too LOW warns on every "+
			"complete one.", appapi.ListMineCap, want)
	}
}

// TestDoctorVerdictSentinelIsDistinguishable is what remains here of a test that
// used to claim more than it could see.
//
// 🔴 THE OLD TEST WAS NAMED `…DoesNotReadAsAToolFailure` AND ASSERTED ON
// `err.Error()` — the string as it exists BEFORE `cmd/civitai/main.go` prepends
// "Error: ". It was therefore structurally incapable of observing the prefix it
// was named for, and it passed while the prefix was still being printed. The
// property is a PROCESS-level one and is now measured where it happens, in
// cmd/civitai's TestDoctorVerdictPrintsNoErrorPrefix, which reads the real
// binary's stderr.
//
// What stays in-package is the one thing this level CAN establish: the verdict
// is reachable and carries the sentinel that main.go branches on. Without that,
// the process-level test could pass for the wrong reason.
func TestDoctorVerdictSentinelIsDistinguishable(t *testing.T) {
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil,
		doctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking")))
	_, _, err := run(t, "app", "doctor")
	if err == nil {
		t.Fatal("expected the blocking verdict")
	}
	if !errors.Is(err, ErrListingBlocked) {
		t.Fatalf("the verdict must carry ErrListingBlocked — main.go's errorLine branches on it: %v", err)
	}
	if !strings.Contains(err.Error(), "not ready to publish") {
		t.Errorf("the verdict should describe the LISTING, got: %v", err)
	}
	// NEGATIVE CONTROL: an ordinary failure must NOT carry the sentinel, or
	// main.go would strip the prefix from real errors too.
	newDoctorServer(t, doctorRow(docSlugA, docListingA, "draft", "owner", nil))
	if _, _, err := run(t, "app", "doctor", "no-such-app-of-mine"); err == nil {
		t.Fatal("control: expected a not-found error")
	} else if errors.Is(err, ErrListingBlocked) {
		t.Error("control: a not-found must NOT carry the verdict sentinel — it is a real failure")
	}
}
