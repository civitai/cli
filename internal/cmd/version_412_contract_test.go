package cmd

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// THE #412 CROSS-COMMAND CONTRACT.
//
// 🔴 THIS IS THE TEST NEITHER PR HAD, AND ITS ABSENCE IS WHY THEY COULD DISAGREE.
// #412 shipped as two halves — `app status`'s drift warning (#413) and `app
// submit`'s monotonic refusal (#414) — written independently against the same
// route. Each half was green, mutation-swept and audit-clean IN ISOLATION. Each
// suite pinned its own command's behaviour and neither built the combined state,
// so nothing anywhere asserted the property both halves rest on and
// highestApprovedVersion's own doc comment states out loud:
//
//	the two commands have to name the same number or they contradict each other.
//
// They did not. The two open-coded copies of the predicate disagreed about
// pre-release ordering, slug normalisation and liveness, and the observable
// result was that with an approved `0.6.0-rc.1` beside an approved `0.5.2`,
// `app status` told the author their repo was behind `0.6.0-rc.1` while `app
// submit` accepted `0.5.3` without a word.
//
// So this file asserts a RELATIONSHIP rather than a component: for one set of
// rows, whatever version `app status` quotes as the highest approved one is the
// version `app submit` refuses against. It lives in neither command's test file
// on purpose — it belongs to the seam, which is what nobody owned.

// contractCase is one row set plus the local manifest version to run both
// commands with.
type contractCase struct {
	name  string
	rows  []driftRow
	local string
	// wantVersion is the version BOTH commands must name, or "" when neither may
	// name one. It is spelled out rather than derived from the fixture: a
	// derived expectation would re-implement the predicate under test and agree
	// with it however wrong it is.
	wantVersion string
}

// statusQuotesVersion runs the real `app status <slug>` against a server serving
// body, from a directory holding a manifest at local, and returns the version
// its drift line names ("" when it stayed silent).
func statusQuotesVersion(t *testing.T, body map[string]any, local string) string {
	t.Helper()
	var calls int32
	srv := driftServer(t, body, &calls, 0)
	writeDriftManifest(t, driftSlug, local)
	setupDriftEnv(t, srv.URL)

	_, errOut, err := run(t, "app", "status", driftSlug)
	if err != nil {
		t.Fatalf("app status: %v", err)
	}
	if !strings.Contains(errOut, driftWarnCore) {
		return ""
	}
	// 🔴 `\S+` then a literal '.', NOT `[^.\s]+` — the first draft of this line
	// used the latter and captured "0" out of "0.5.2.", so every case compared
	// "0" against a real version and the whole battery went red on its first
	// run. Recorded because it is the good outcome: an extractor that returns
	// the WRONG string is loud, while one that returns "" is silent and green.
	m := regexp.MustCompile(`which is (\S+)\.(?:\s|$)`).FindStringSubmatch(errOut)
	if m == nil {
		t.Fatalf("the drift warning printed but names no version — this test cannot compare what it "+
			"cannot read, and a silently-changed sentence must not read as agreement.\nstderr:\n%s", errOut)
	}
	return m[1]
}

// submitQuotesVersion runs the real refusal path over the SAME rows, decoded
// from the SAME body the server serves, and returns the version the refusal
// names ("" when it did not refuse).
//
// 🔴 THE ROWS COME FROM THE JSON, NOT FROM A PARALLEL []appapi.Submission
// LITERAL. Two hand-built fixtures are two chances to encode the same wrong
// shape twice — the failure mode where both sides agree because both are wrong.
// Round-tripping the served body through the real struct tags means the submit
// half sees exactly the bytes the status half's HTTP client saw.
func submitQuotesVersion(t *testing.T, body map[string]any, local string) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}
	var env struct {
		Submissions []appapi.Submission `json:"submissions"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding the fixture through appapi.Submission: %v", err)
	}
	if len(env.Submissions) == 0 {
		t.Fatal("the fixture decoded to zero rows — every case below would then agree vacuously")
	}
	_, gErr := guardRun(t, env.Submissions, local, false)
	if gErr == nil {
		return ""
	}
	m := regexp.MustCompile(`— ([^\s]+) is already approved`).FindStringSubmatch(gErr.Error())
	if m == nil {
		t.Fatalf("the guard refused but names no approved version:\n%s", gErr.Error())
	}
	return m[1]
}

// TestStatusAndSubmitNameTheSameVersion is the contract itself.
//
// Every case uses a local version STRICTLY BELOW the intended peak, because that
// is the one situation in which both commands are required to speak — `app
// status` warns BEHIND and `app submit` refuses — so a disagreement is
// observable as two different strings rather than as one command's silence.
// (The silent-vs-silent direction is the last case, and it is the one the
// pre-release policy changed.)
func TestStatusAndSubmitNameTheSameVersion(t *testing.T) {
	for _, tc := range []contractCase{
		{
			name: "the canonical anti-vacuity listing",
			rows: []driftRow{
				{"pubreq_07", "0.7.0", "withdrawn", "", "2026-08-03T10:00:00.000Z"},
				{"pubreq_06", "0.6.0", "pending", "building", "2026-08-02T10:00:00.000Z"},
				{"pubreq_05", "0.5.2", "approved", "live", "2026-08-01T10:00:00.000Z"},
				{"pubreq_02", "0.2.0", "rejected", "failed", "2026-07-02T10:00:00.000Z"},
			},
			local: "0.4.0", wantVersion: "0.5.2",
		},
		{
			// 🔴 THE FIXTURE THAT PRODUCED THE ORIGINAL CONTRADICTION. Under the
			// truncating comparator `app status` ranked the rc first and quoted
			// 0.6.0-rc.1; `app submit` declared it unorderable and refused
			// against 0.5.2. Two commands, same rows, different numbers.
			name: "an approved pre-release beside an approved release",
			rows: []driftRow{
				{"pubreq_rc", "0.6.0-rc.1", "approved", "live", "2026-08-06T10:00:00.000Z"},
				{"pubreq_05", "0.5.2", "approved", "deploying", "2026-08-01T10:00:00.000Z"},
			},
			local: "0.4.0", wantVersion: "0.5.2",
		},
		{
			// The newest APPROVED row is a lower rollback, so "newest approved"
			// and "highest approved" diverge. Both commands must take the max.
			name: "a newer approved rollback below the peak",
			rows: []driftRow{
				{"pubreq_rb", "0.4.9", "approved", "failed", "2026-08-11T12:00:00.000Z"},
				{"pubreq_05", "0.5.2", "approved", "live", "2026-08-01T08:00:00.000Z"},
			},
			local: "0.3.0", wantVersion: "0.5.2",
		},
		{
			// Semver, not string, ordering — across the 9/10 boundary, where a
			// string compare is wrong in both directions.
			name: "the 9/10 boundary",
			rows: []driftRow{
				{"pubreq_10", "0.10.0", "approved", "live", "2026-08-05T10:00:00.000Z"},
				{"pubreq_09", "0.9.0", "approved", "building", "2026-08-04T10:00:00.000Z"},
			},
			local: "0.9.0", wantVersion: "0.10.0",
		},
		{
			// Status normalisation: a server that shouts must not switch one
			// command off while the other keeps working.
			name: "a capitalised approved status",
			rows: []driftRow{
				{"pubreq_06", "0.6.0", "pending", "building", "2026-08-02T10:00:00.000Z"},
				{"pubreq_05", "0.5.2", " Approved ", "live", "2026-08-01T10:00:00.000Z"},
			},
			local: "0.4.0", wantVersion: "0.5.2",
		},
		{
			// 🔴 THE SILENT-VS-SILENT DIRECTION. Every approved row carries a
			// pre-release suffix, so neither command may name a version: `app
			// status` says nothing, `app submit` warns that it could not order
			// them and PROCEEDS (no refusal, hence no version). Agreement here
			// is agreement about an ABSENCE, which is exactly the case a
			// contract test has to include — the two halves previously
			// disagreed most sharply on precisely these rows.
			name: "nothing orderable is approved",
			rows: []driftRow{
				{"pubreq_rc2", "0.6.0-rc.2", "approved", "live", "2026-08-06T10:00:00.000Z"},
				{"pubreq_rc1", "0.5.9-rc.1", "approved", "building", "2026-08-05T10:00:00.000Z"},
			},
			local: "0.4.0", wantVersion: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := driftRows(t, tc.rows...)
			gotSubmit := submitQuotesVersion(t, body, tc.local)
			// statusQuotesVersion t.Chdir()s, so it runs last — nothing after it
			// may depend on the original working directory.
			gotStatus := statusQuotesVersion(t, body, tc.local)

			if gotStatus != gotSubmit {
				t.Errorf("THE TWO COMMANDS DISAGREE about the same rows: `app status` names %q, "+
					"`app submit` refuses against %q. One of them is telling the author to change something "+
					"the other will not accept — see the header of this file.", gotStatus, gotSubmit)
			}
			if gotStatus != tc.wantVersion {
				t.Errorf("both commands name %q, want %q — they agree, but on the wrong version, which "+
					"an equality check alone cannot catch", gotStatus, tc.wantVersion)
			}
		})
	}
}

// TestStatusAndSubmitContractHarnessCanObserveADisagreement is the POSITIVE
// CONTROL on the test above.
//
// 🔴 WITHOUT IT, TestStatusAndSubmitNameTheSameVersion IS INDISTINGUISHABLE FROM
// A HARNESS WIRED TO NOTHING. Its verdict is an equality between two strings
// this file extracts with two different regexes from two different streams; if
// either extractor silently stopped matching, both sides would come back "" and
// every case would agree — a green built entirely out of two failures. So: feed
// the SAME extractors an input on which the two commands genuinely differ, and
// require the comparison to come out UNEQUAL.
//
// The divergent input is a local version EQUAL to the peak. There the two
// commands are designed to differ, and the difference is deliberate product
// behaviour documented on versionDriftWarning: `app status` describes a REPO, and
// a repo sitting exactly at what it released is healthy, so it is silent; `app
// submit` judges an ACTION, and resubmitting the version that is already live is
// the accident #412 is about, so it refuses and names 0.5.2. Both extractors
// therefore have to run, and they have to return different answers.
//
// This is NOT a hole in the contract. The contract is "when both speak, they
// name the same version" — never "both always speak".
func TestStatusAndSubmitContractHarnessCanObserveADisagreement(t *testing.T) {
	rows := []driftRow{
		{"pubreq_06", "0.6.0", "pending", "building", "2026-08-02T10:00:00.000Z"},
		{"pubreq_05", "0.5.2", "approved", "live", "2026-08-01T10:00:00.000Z"},
	}

	// ARM 1 — can each extractor observe a version AT ALL? Both must come back
	// non-empty and correct, or a "" from either one downstream means "the
	// regex missed", not "the command was silent".
	t.Run("both extractors observe a version", func(t *testing.T) {
		body := driftRows(t, rows...)
		gotSubmit := submitQuotesVersion(t, body, "0.4.0")
		gotStatus := statusQuotesVersion(t, body, "0.4.0")
		if gotSubmit != "0.5.2" {
			t.Errorf("the SUBMIT extractor returned %q, want 0.5.2 — it is not observing the refusal, so every "+
				"agreement this file reports would be an agreement between two blanks", gotSubmit)
		}
		if gotStatus != "0.5.2" {
			t.Errorf("the STATUS extractor returned %q, want 0.5.2 — same hazard on the other side", gotStatus)
		}
	})

	// ARM 2 — can the COMPARISON come out unequal? An equality check that can
	// never be false certifies nothing. At a local version EQUAL to the peak the
	// two commands are designed to differ (see the note above), so the harness
	// must report exactly that.
	t.Run("the comparison can come out unequal", func(t *testing.T) {
		body := driftRows(t, rows...)
		gotSubmit := submitQuotesVersion(t, body, "0.5.2")
		gotStatus := statusQuotesVersion(t, body, "0.5.2")
		if gotSubmit != "0.5.2" {
			t.Fatalf("`app submit` must refuse an equal resubmit and name 0.5.2; got %q", gotSubmit)
		}
		if gotStatus != "" {
			t.Fatalf("`app status` must stay silent at EQUAL (see versionDriftWarning's three-way note); got %q", gotStatus)
		}
		if gotStatus == gotSubmit {
			t.Fatal("the harness reported agreement on an input where the two commands are designed to differ — " +
				"it cannot observe a disagreement, so it cannot certify one is absent")
		}
	})
}

// TestContractFixturesAreServedByTheSameBody guards the one structural
// assumption both helpers make: the body the httptest server encodes and the
// body decoded into []appapi.Submission are the same object, so a fixture change
// cannot drift between the two halves.
func TestContractFixturesAreServedByTheSameBody(t *testing.T) {
	body := driftRows(t,
		driftRow{"pubreq_05", "0.5.2", "approved", "live", "2026-08-01T10:00:00.000Z"},
	)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var env struct {
		Submissions []appapi.Submission `json:"submissions"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Submissions) != 1 {
		t.Fatalf("decoded %d rows, want 1", len(env.Submissions))
	}
	s := env.Submissions[0]
	if s.BlockID != driftSlug || s.Version != "0.5.2" || s.Status != "approved" {
		t.Errorf("the fixture decodes to %+v — the submit half is not seeing the row the status half is served", s)
	}
	// liveUrl is the field rowIsServing leads on; if driftRows stopped emitting
	// it, the submit half would quietly lose its live claim while the status
	// half, which never reads it, stayed green.
	if s.LiveURL == nil || *s.LiveURL == "" {
		t.Error("an approved+live fixture row decoded with no liveUrl — rowIsServing leads on that field, so " +
			"the refusal would silently drop its 'and live' claim")
	}
	var calls int32
	srv := driftServer(t, body, &calls, 0)
	if srv.URL == "" {
		t.Fatal("no server")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("the server counted a request before one was made")
	}
}
