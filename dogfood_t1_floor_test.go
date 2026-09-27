package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The PUBLISH-FLOOR half of the ship verdict — T1's third conjunct.
//
// T0 asks "was it submitted". T1 asks "was it submitted AND is its store listing
// publishable", where publishable means an ICON and a COVER are attached. That is
// server state on a different route from the submissions listing, so
// `ship.verdict.sh` makes a second read (`civitai app doctor --json`) with the same
// credential, in the same container, attributed by the same slug set.
//
// 🔴 THE PROPERTY THESE TESTS EXIST FOR, ABOVE THE VERDICT ITSELF: THE FLOOR READ
// MUST NOT BE ABLE TO DAMAGE THE SHIP VERDICT. Every `ship` cell already graded is
// entitled to its `SHIP=` whether or not this account also answers `app doctor`, so
// a floor read that fails yields `FLOOR=unmeasured` on the line and leaves the exit
// code and `SHIP=` untouched. A change that made the floor read a fourth way to
// exit 2 would silently convert working T0 cells into unmeasured ones —
// `TestT1FloorFailureCannotDamageTheShipVerdict` is the guard.
//
// Docker and the CLI are STUBBED exactly as in dogfood_ship_verdict_test.go, so
// none of this needs a daemon, a network, a credential or an account.

// ── fixtures ─────────────────────────────────────────────────────────────────

// doctorProblem mirrors one row of `civitai app doctor --json`'s
// `apps[].blocking` / `apps[].advisory` arrays.
type doctorProblem struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
	Fix      string `json:"fix"`
}

type doctorApp struct {
	Slug     string          `json:"slug"`
	Name     string          `json:"name"`
	Status   string          `json:"status"`
	Blocking []doctorProblem `json:"blocking"`
	Advisory []doctorProblem `json:"advisory"`
}

// The two codes the floor is read from, spelled here rather than derived. Pinned
// against the CLI's own constants by TestT1FloorCodesAreTheCodesTheCLIEmits — the
// positive control for a verdict that is otherwise an ABSENCE of these strings.
const (
	codeMissingIcon  = "missing-icon"
	codeMissingCover = "missing-cover"
	// Not graded on, and on the cell for a reason: it is the code almost every
	// trial listing carries, so seeing it proves the server's completeness
	// computation RAN on the row rather than the row simply being empty.
	codeNoScreenshots = "no-screenshots"
)

func dProblem(code, sev string) doctorProblem {
	return doctorProblem{Code: code, Label: "server sentence for " + code, Severity: sev, Fix: "a fix line"}
}

// doctorBody renders a payload the way the real `--json` does, including the
// `summary` block the grader reads `truncated` out of.
func doctorBody(t *testing.T, apps []doctorApp, truncated bool) string {
	t.Helper()
	blocking := 0
	advisory := 0
	for _, a := range apps {
		blocking += len(a.Blocking)
		advisory += len(a.Advisory)
	}
	raw, err := json.Marshal(map[string]any{
		"ok":   blocking == 0,
		"apps": apps,
		"summary": map[string]any{
			"apps": len(apps), "blocking": blocking, "advisory": advisory,
			"gating": blocking, "delisted": 0, "truncated": truncated,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The operator's real back catalogue, in miniature, on the LISTING route. Present
// in every case below — including the passing one — because a payload holding only
// the trial's own row would let a floor check with no identity conjunct pass every
// test here. This is the listing-route counterpart of shipPreExisting.
func doctorPreExisting() []doctorApp {
	return []doctorApp{
		// Complete: a real published app of the operator's. A floor check keyed on
		// "does ANY listing on this account have an icon and a cover" passes on this
		// row alone, which is the failure this fixture makes visible.
		{Slug: "panorama-360", Name: "Panorama", Status: "approved",
			Blocking: []doctorProblem{}, Advisory: []doctorProblem{}},
		// Incomplete, and it must not drag the trial's own row down either.
		{Slug: "custom-generators", Name: "Custom", Status: "draft",
			Blocking: []doctorProblem{dProblem(codeMissingCover, "blocking")},
			Advisory: []doctorProblem{dProblem(codeNoScreenshots, "advisory")}},
	}
}

// The trial's own listing, at the floor: no missing-icon, no missing-cover. It
// still carries the screenshots advisory, which is what a real T1 listing looks
// like — the brief does not ask for screenshots.
func doctorTrialAtFloor() doctorApp {
	return doctorApp{Slug: "ab-ship-01", Name: "Ship", Status: "draft",
		Blocking: []doctorProblem{},
		Advisory: []doctorProblem{dProblem(codeNoScreenshots, "advisory")}}
}

// shipFloorEnv is the standard T1 fixture: a trial that submitted inside its own
// window, plus whatever doctor payload the case is about.
func shipFloorEnv(t *testing.T, w shipWindow, apps []doctorApp, truncated bool, doctorRC string) []string {
	t.Helper()
	subs := append([]shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}, shipPreExisting(time.Now())...)
	return shipStubEnv(t, shipEnv{
		state: "running", transcript: shipTranscript(w.start, w.end, "t1"),
		manifests:  map[string]string{"app": fxShipManifest},
		statusBody: shipStatusBody(t, subs),
		doctorBody: doctorBody(t, apps, truncated),
		doctorRC:   doctorRC,
	})
}

// ── the verdict ──────────────────────────────────────────────────────────────

// T1 = SUBMITTED AND AT THE FLOOR. All of SHIP's three conjuncts hold and the
// trial's own listing carries both mandatory assets.
func TestT1FloorGradesAListingAtThePublishFloor(t *testing.T) {
	w := shipStdWindow()
	apps := append([]doctorApp{doctorTrialAtFloor()}, doctorPreExisting()...)
	out, code := runShipVerdict(t, shipFloorEnv(t, w, apps, false, "0"), "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	for k, want := range map[string]string{
		"SHIP":           "yes",
		"FLOOR":          "yes",
		"T1":             "yes",
		"floor_matched":  "1",
		"floor_slug":     "ab-ship-01",
		"floor_missing":  "none",
		"floor_listings": "3",
	} {
		if got := shipField(t, out, k); got != want {
			t.Fatalf("%s=%s, want %s\n%s", k, got, want, out)
		}
	}
	// 🔴 THE CODE LIST IS ON THE CELL, AND ON A PASS IT IS THE ONLY THING THAT
	// SEPARATES "the server said this listing is complete" FROM "the server said
	// nothing about this listing". FLOOR=yes is read as the ABSENCE of two codes, so
	// a row with an empty code list would produce the same verdict as a row the
	// completeness computation genuinely cleared. The screenshots advisory is what a
	// reader checks.
	if !strings.Contains(out, "floor_codes="+codeNoScreenshots+"\n") {
		t.Fatalf("floor_codes does not carry the codes the server DID emit for the trial's row — "+
			"without them a FLOOR=yes read off two absent codes cannot be distinguished from a row "+
			"the server never evaluated:\n%s", out)
	}
}

// 🔴 THE NEGATIVE CONTROLS. Each holds every other conjunct and removes exactly
// one part of the floor. Read them as a matrix: a `FLOOR=yes` on any of them means
// that part is no longer checked, and a T1 cell then reports a publishable app for
// a listing with an empty mandatory slot.
func TestT1FloorFailsEveryNegativeControl(t *testing.T) {
	w := shipStdWindow()
	trialRow := func(blocking ...doctorProblem) doctorApp {
		a := doctorTrialAtFloor()
		a.Blocking = blocking
		return a
	}
	for _, tc := range []struct {
		name        string
		apps        []doctorApp
		wantMatched string
		wantMissing string
		wantMsg     string
	}{
		{
			// The shape a `ship` trial produces today: submitted, no media at all.
			// This is the case whose REAL exit code is 1 with a full payload, which is
			// why the grader reads the payload and not the code.
			name:        "the trial's own listing has neither an icon nor a cover",
			apps:        append([]doctorApp{trialRow(dProblem(codeMissingIcon, "blocking"), dProblem(codeMissingCover, "blocking"))}, doctorPreExisting()...),
			wantMatched: "1",
			wantMissing: codeMissingIcon + "," + codeMissingCover,
			wantMsg:     "below the publish floor",
		},
		{
			// Half the floor. The icon band (aspect 0.9–1.1) and the cover band
			// (1.3–2.4) are disjoint, so one attach succeeding says nothing about the
			// other — an agent that generated one image and attached it twice fails
			// the second attach at the platform's aspect check.
			name:        "an icon but no cover",
			apps:        append([]doctorApp{trialRow(dProblem(codeMissingCover, "blocking"))}, doctorPreExisting()...),
			wantMatched: "1",
			wantMissing: codeMissingCover,
			wantMsg:     "below the publish floor",
		},
		{
			name:        "a cover but no icon",
			apps:        append([]doctorApp{trialRow(dProblem(codeMissingIcon, "blocking"))}, doctorPreExisting()...),
			wantMatched: "1",
			wantMissing: codeMissingIcon,
			wantMsg:     "below the publish floor",
		},
		{
			// 🔴 THE IDENTITY CONTROL, and the one the whole attribution exists for.
			// The account's OWN complete listing (`panorama-360`) is present and has
			// both assets; the trial's app has no listing at all. A floor check that
			// asked "does this account have a complete listing?" reads yes.
			name:        "the account has complete listings but none of them is the trial's",
			apps:        doctorPreExisting(),
			wantMatched: "0",
			wantMissing: "none",
			wantMsg:     "names any of the trial's own apps",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runShipVerdict(t, shipFloorEnv(t, w, tc.apps, false, "1"), "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — a below-floor listing is a VERDICT, and the doctor "+
					"read exiting 1 (which it does whenever anything is gating) must not make it "+
					"unmeasured\n%s", code, out)
			}
			if got := shipField(t, out, "FLOOR"); got != "no" {
				t.Fatalf("FLOOR=%s, want no — this control removes exactly one part of the floor, "+
					"so a `yes` means that part is no longer checked\n%s", got, out)
			}
			// 🔴 AND THE SHIP HALF IS UNTOUCHED. A floor failure is not a submission
			// failure; conflating them would report "it did not submit" about a trial
			// that plainly did.
			if got := shipField(t, out, "SHIP"); got != "yes" {
				t.Fatalf("SHIP=%s, want yes — the floor half moved the ship verdict\n%s", got, out)
			}
			if got := shipField(t, out, "T1"); got != "no" {
				t.Fatalf("T1=%s, want no\n%s", got, out)
			}
			if got := shipField(t, out, "floor_matched"); got != tc.wantMatched {
				t.Fatalf("floor_matched=%s, want %s\n%s", got, tc.wantMatched, out)
			}
			if got := shipField(t, out, "floor_missing"); got != tc.wantMissing {
				t.Fatalf("floor_missing=%s, want %s — a bare `no` cannot tell a half-met floor "+
					"from a listing that does not exist\n%s", got, tc.wantMissing, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("the floor reason does not say %q:\n%s", tc.wantMsg, out)
			}
		})
	}
}

// 🔴 THE DISCRIMINATING CONTROL FOR THE CONTROLS. A floor check that ALWAYS
// answered `no` satisfies every negative case above, and four green negative
// controls would then read as thorough coverage of a verdict that can never say
// yes. This pairs the passing arm with the identity arm on ONE doctor payload that
// differs only in the manifest's blockId, so the two verdicts cannot both be
// explained by anything else. Same role TestShipVerdictSeparatesTheTrialsAppFrom
// TheAccountsOwn plays for the ship half.
func TestT1FloorSeparatesTheTrialsListingFromTheAccountsOwn(t *testing.T) {
	w := shipStdWindow()
	apps := append([]doctorApp{doctorTrialAtFloor()}, doctorPreExisting()...)
	body := doctorBody(t, apps, false)
	subs := append([]shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}, shipPreExisting(time.Now())...)
	statusBody := shipStatusBody(t, subs)
	for _, tc := range []struct {
		name      string
		blockID   string
		wantFloor string
	}{
		{"the manifest names the listing that is at the floor", "ab-ship-01", "yes"},
		{"the manifest names a DIFFERENT app the trial built", "ab-ship-99", "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := `{"blockId":"` + tc.blockID + `","version":"0.1.0","name":"Ship","type":"block"}`
			env := shipStubEnv(t, shipEnv{
				state: "running", transcript: shipTranscript(w.start, w.end, "t1"),
				manifests: map[string]string{"app": m}, statusBody: statusBody,
				doctorBody: body,
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			if got := shipField(t, out, "FLOOR"); got != tc.wantFloor {
				t.Fatalf("FLOOR=%s, want %s (blockId %q against the same doctor payload)\n%s",
					got, tc.wantFloor, tc.blockID, out)
			}
		})
	}
}

// The slug the floor is attributed by is read out of the CONTAINER'S manifest and
// normalised the way `appapi.SameSlug` normalises — trimmed and case-folded — and
// it is compared against the LISTING's `slug`, which `resolveListingSlug` shows is
// the manifest's own blockId. `manifest.Load` is a bare json.Unmarshal with no
// schema validation, so a hand-edited `"blockId": " Ab-Ship-01 "` reaches this
// comparison and an exact byte compare would report a confident `FLOOR=no` for a
// listing that plainly is the trial's.
//
// ⚠ An INVARIANT guard for the normalisation itself (the ship half already pins
// it); it is regression coverage for the floor half, which had no such comparison
// before this change.
func TestT1FloorNormalisesTheSlugTheWayTheAPIDoes(t *testing.T) {
	w := shipStdWindow()
	apps := []doctorApp{doctorTrialAtFloor()}
	subs := []shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}
	env := shipStubEnv(t, shipEnv{
		state: "running", transcript: shipTranscript(w.start, w.end, "t1"),
		manifests:  map[string]string{"app": `{"blockId":"  Ab-Ship-01  ","version":"0.1.0"}`},
		statusBody: shipStatusBody(t, subs),
		doctorBody: doctorBody(t, apps, false),
	})
	out, code := runShipVerdict(t, env, "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := shipField(t, out, "FLOOR"); got != "yes" {
		t.Fatalf("FLOOR=%s, want yes — a padded, mixed-case blockId names the same listing\n%s", got, out)
	}
}

// ── the floor read must not be able to damage the ship verdict ───────────────

// 🔴 THE SINGLE MOST IMPORTANT PROPERTY OF THIS ADDITION. The floor check is a
// SECOND account read bolted onto a grader whose first read already works, and the
// failure mode of adding one is that its failures become the whole script's. Every
// arm here is reachable on an ordinary credentialed run — an older CLI in the
// image, a route that 500s, a payload shape that moved — and every one of them must
// leave `SHIP=yes`, exit 0, and say `FLOOR=unmeasured` rather than `FLOOR=no`.
//
// `unmeasured` is a THIRD STATE and is not `no`: "this account's listings could not
// be read" and "this trial did not finish its listing" are opposite answers, and a
// T1 matrix that folded them together would report models failing at a rung the
// harness never measured.
func TestT1FloorFailureCannotDamageTheShipVerdict(t *testing.T) {
	w := shipStdWindow()
	for _, tc := range []struct {
		name       string
		doctorBody string
		doctorRC   string
		wantMsg    string
	}{
		{
			// The `ship`-era shape: a CLI (or a stub) that answers the doctor call
			// with nothing at all.
			name: "the doctor call printed nothing", doctorBody: "", doctorRC: "0",
			wantMsg: "no `apps` array",
		},
		{
			name: "the doctor call failed outright", doctorBody: "", doctorRC: "1",
			wantMsg: "no `apps` array",
		},
		{
			// A route that answered, with something that is not this payload.
			name:       "the doctor call returned a payload with no apps array",
			doctorBody: `{"error":"nope"}`, doctorRC: "1",
			wantMsg: "no `apps` array",
		},
		{
			name:       "the doctor call returned unparseable bytes",
			doctorBody: "<html>504 Gateway Timeout</html>", doctorRC: "1",
			wantMsg: "no `apps` array",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subs := append([]shipSub{{
				ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
				SubmittedAt: w.mid.UTC().Format(time.RFC3339),
			}}, shipPreExisting(time.Now())...)
			env := shipStubEnv(t, shipEnv{
				state: "running", transcript: shipTranscript(w.start, w.end, "t1"),
				manifests:  map[string]string{"app": fxShipManifest},
				statusBody: shipStatusBody(t, subs),
				doctorBody: tc.doctorBody, doctorRC: tc.doctorRC,
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — a floor read that could not be taken must NOT become a "+
					"fourth way for this script to exit 2, or every already-graded ship cell "+
					"becomes unmeasured\n%s", code, out)
			}
			if got := shipField(t, out, "SHIP"); got != "yes" {
				t.Fatalf("SHIP=%s, want yes — the floor read damaged the T0 verdict\n%s", got, out)
			}
			if got := shipField(t, out, "FLOOR"); got != "unmeasured" {
				t.Fatalf("FLOOR=%s, want unmeasured — a read that did not happen is not evidence "+
					"of an unfinished listing\n%s", got, out)
			}
			if got := shipField(t, out, "T1"); got != "unmeasured" {
				t.Fatalf("T1=%s, want unmeasured — nothing false was established\n%s", got, out)
			}
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("the floor reason does not say %q:\n%s", tc.wantMsg, out)
			}
		})
	}
}

// 🔴 A CONJUNCT MEASURED FALSE SETTLES T1 EVEN WHEN THE OTHER IS UNMEASURED, and
// the asymmetry is deliberate: `SHIP=no` means the app is provably not in the
// queue, which settles T1 however the floor read went. Reporting `unmeasured`
// there would invite a re-run that cannot change the answer. `unmeasured` is
// reserved for the case where nothing false was established.
func TestT1IsTheConjunctionAndSaysSoOnEveryCombination(t *testing.T) {
	w := shipStdWindow()
	atFloor := append([]doctorApp{doctorTrialAtFloor()}, doctorPreExisting()...)
	belowFloor := func() []doctorApp {
		a := doctorTrialAtFloor()
		a.Blocking = []doctorProblem{dProblem(codeMissingIcon, "blocking")}
		return append([]doctorApp{a}, doctorPreExisting()...)
	}()
	for _, tc := range []struct {
		name string
		// submitted: the trial's own app has a pending in-window submission.
		submitted  bool
		apps       []doctorApp
		unreadable bool
		wantShip   string
		wantFloor  string
		wantT1     string
	}{
		{"submitted and at the floor", true, atFloor, false, "yes", "yes", "yes"},
		{"submitted, below the floor", true, belowFloor, false, "yes", "no", "no"},
		{"at the floor but never submitted", false, atFloor, false, "no", "yes", "no"},
		{"never submitted, floor unreadable", false, atFloor, true, "no", "unmeasured", "no"},
		{"submitted, floor unreadable", true, atFloor, true, "yes", "unmeasured", "unmeasured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subs := shipPreExisting(time.Now())
			if tc.submitted {
				subs = append([]shipSub{{
					ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
					SubmittedAt: w.mid.UTC().Format(time.RFC3339),
				}}, subs...)
			}
			body := doctorBody(t, tc.apps, false)
			if tc.unreadable {
				body = ""
			}
			env := shipStubEnv(t, shipEnv{
				state: "running", transcript: shipTranscript(w.start, w.end, "t1"),
				manifests:  map[string]string{"app": fxShipManifest},
				statusBody: shipStatusBody(t, subs),
				doctorBody: body,
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			for k, want := range map[string]string{
				"SHIP": tc.wantShip, "FLOOR": tc.wantFloor, "T1": tc.wantT1,
			} {
				if got := shipField(t, out, k); got != want {
					t.Fatalf("%s=%s, want %s (SHIP=%s FLOOR=%s)\n%s",
						k, got, want, tc.wantShip, tc.wantFloor, out)
				}
			}
		})
	}
}

// The doctor page is capped server-side with no cursor, exactly as the submissions
// listing is, so a `FLOOR=no` on a truncated page is weaker than it looks. The flag
// is REPORTED rather than refused on — `listMine` orders newest-first, so a fresh
// trial listing is in the page — but a reader has to be able to see it.
func TestT1FloorReportsWhetherTheDoctorPageWasTruncated(t *testing.T) {
	w := shipStdWindow()
	apps := append([]doctorApp{doctorTrialAtFloor()}, doctorPreExisting()...)
	for _, tc := range []struct {
		truncated bool
		want      string
	}{{false, "false"}, {true, "true"}} {
		out, code := runShipVerdict(t, shipFloorEnv(t, w, apps, tc.truncated, "0"), "ctl", "root")
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, out)
		}
		if got := shipField(t, out, "floor_truncated"); got != tc.want {
			t.Fatalf("floor_truncated=%s, want %s\n%s", got, tc.want, out)
		}
	}
	// A payload with no `summary.truncated` at all — an older CLI in the trial
	// image. 🔴 It must NOT read as `false`: that would assert the page was complete
	// on exactly the payload that cannot say so.
	env := shipStubEnv(t, shipEnv{
		state: "running", transcript: shipTranscript(w.start, w.end, "t1"),
		manifests:  map[string]string{"app": fxShipManifest},
		statusBody: shipStatusBody(t, shipPreExisting(time.Now())),
		doctorBody: `{"apps":[{"slug":"ab-ship-01","blocking":[],"advisory":[]}]}`,
	})
	out, code := runShipVerdict(t, env, "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := shipField(t, out, "floor_truncated"); got != "unknown" {
		t.Fatalf("floor_truncated=%s, want unknown — a payload with no `truncated` key must not "+
			"be reported as a complete page\n%s", got, out)
	}
}

// ── the render half, through the shared delegation ───────────────────────────

// 🔴 THE DELEGATION IS A CLAIM AND A STRUCTURAL GREP CANNOT CHECK IT.
// `TestT1BriefAndAssertionAgree` asserts that `t1.assert.mjs` names
// `genpost.assert.mjs` and `_delegate.mjs` — and that type-checks past a delegation
// that swallows stdout, drops `argv[3]` (the scope list), or turns exit 2 into exit
// 1. Worse, it cannot see whether `oracle.sh` RESOLVES the `t1` brief name to that
// file at all. So this drives the real oracle and a real browser under BOTH brief
// names over one fixture set and requires the verdicts to be identical.
//
// Two fixtures rather than the four
// `TestOracleGradesTheShipBriefThroughTheGenpostAssertion` uses: the four-fixture
// sweep already covers the DELEGATE, which `t1` and `ship` share, and two arms are
// what proves this brief name can go both ways. One arm would be satisfied by a
// delegation wired to a constant.
//
// Requires a browser, and FAILS rather than skips under $CI for the same reason
// every other oracle test does: a skip and a pass read identically in the log a
// merge gate is read from.
func TestOracleGradesTheT1BriefThroughTheGenpostAssertion(t *testing.T) {
	browser := oracleBrowser(t)
	for _, tc := range []struct {
		name       string
		html       string
		wantRender string
	}{
		{"a generate-then-post app", fxGenpost, "yes"},
		{"generate only, no Post control", fxGenpostNoPost, "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := func(brief string) (render, reason, observed string) {
				t.Helper()
				env := append(stubOracleEnv(t, stubEnv{
					state: "running", civitaiRC: "0",
					manifest: fxManifestScoped, outputDir: "dist", appHTML: tc.html,
				}), "CIVITAI_CHROME="+browser)
				out, code := runScript(t, "oracle.sh", env, "ctl", "root", brief)
				if code != 0 {
					t.Fatalf("oracle.sh %s exited %d, want 0\n%s", brief, code, out)
				}
				for _, line := range strings.Split(out, "\n") {
					if strings.HasPrefix(line, "render_reason=") {
						reason = strings.TrimPrefix(line, "render_reason=")
					}
				}
				return summaryField(t, out, "RENDER"), reason, summaryField(t, out, "observed")
			}
			t1R, t1Reason, t1Obs := read("t1")
			gpR, gpReason, gpObs := read("genpost")
			if t1R != tc.wantRender {
				t.Fatalf("t1 RENDER=%s, want %s", t1R, tc.wantRender)
			}
			// 🔴 THE EQUIVALENCE, not just the verdict. `observed` and the reason are
			// what a reader uses to tell a near-miss from nothing at all, and a
			// delegation that lost either would still agree on the boolean.
			if t1R != gpR || t1Reason != gpReason || t1Obs != gpObs {
				t.Fatalf("t1.assert.mjs and genpost.assert.mjs disagree on the same block — the "+
					"delegation is not transparent:\n  t1:      RENDER=%s observed=%s reason=%q\n"+
					"  genpost: RENDER=%s observed=%s reason=%q",
					t1R, t1Obs, t1Reason, gpR, gpObs, gpReason)
			}
		})
	}
}

// ── the vocabulary the absence is read against ───────────────────────────────

// 🔴 THE POSITIVE CONTROL FOR A VERDICT MADE OF AN ABSENCE. `FLOOR=yes` is "the
// server did not emit `missing-icon` and did not emit `missing-cover` for this
// listing", so if either code is ever respelled the grader reads a complete floor
// for an empty one — silently, in the reassuring direction. The codes are not the
// grader's invention: they are the strings `internal/cmd/app_doctor.go` declares and
// routes a fix for, so this pins the grader's two literals TO THAT DECLARATION.
//
// It is a structural check because the behavioural route cannot see it: the stub
// answers whatever payload a test writes, so a grader keyed on a code the server no
// longer sends still passes every case above.
func TestT1FloorCodesAreTheCodesTheCLIEmits(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("internal", "cmd", "app_doctor.go"))
	if err != nil {
		t.Fatal(err)
	}
	doctor := string(raw)
	// The CLI's own declaration of each code, as a Go constant. A respelling moves
	// this line and the ledger goes red here rather than in a live T1 grade.
	//
	// ⚠ Matched with a regexp, not `strings.Contains`: gofmt ALIGNS the `=` of a
	// const block, so the literal `problemMissingIcon = "missing-icon"` is not the
	// bytes on disk (`problemMissingIcon      = "…"`) and a Contains form fails on a
	// correct file. Anchored to the line start so a mention inside a comment or a
	// longer identifier is not a match.
	for name, code := range map[string]string{
		"problemMissingIcon":   codeMissingIcon,
		"problemMissingCover":  codeMissingCover,
		"problemNoScreenshots": codeNoScreenshots,
	} {
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s+=\s+"` + regexp.QuoteMeta(code) + `"`)
		if !re.MatchString(doctor) {
			t.Errorf("internal/cmd/app_doctor.go no longer declares %s = %q — the floor verdict is "+
				"read as the ABSENCE of that code, so a respelling makes it report a complete floor "+
				"for an empty one", name, code)
		}
	}
	vraw, err := os.ReadFile(filepath.Join(dogfoodDir, "ship.verdict.sh"))
	if err != nil {
		t.Fatal(err)
	}
	verdict := string(vraw)
	// And the grader keys on exactly those two, in a line that RUNS.
	for _, code := range []string{codeMissingIcon, codeMissingCover} {
		found := false
		for _, line := range strings.Split(verdict, "\n") {
			code2 := strings.TrimSpace(line)
			if strings.HasPrefix(code2, "#") {
				continue
			}
			if strings.Contains(code2, `"`+code+`"`) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ship.verdict.sh does not select on %q outside a comment — the floor half is "+
				"not reading the code it is documented to read", code)
		}
	}
	// 🔴 AND IT MUST NOT READ THE MUTATING ROUTE. `civitai app listing status` is the
	// obvious command for "what does the floor still need" and it is the one that must
	// never appear: on a live listing it opens a shadow revision this CLI cannot
	// close. TestShipVerdictNeverMutatesTheAccount owns that ledger; this is the
	// positive half — the floor read is the pure one, and it has to be present at all.
	if !strings.Contains(verdict, "civitai app doctor --json") {
		t.Fatal("ship.verdict.sh does not run `civitai app doctor --json` — the floor half has no " +
			"read, and every check above just proved nothing")
	}
}
