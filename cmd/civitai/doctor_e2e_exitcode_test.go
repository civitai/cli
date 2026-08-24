package main

// THE PROCESS EXIT STATUS OF `civitai app doctor`, MEASURED END TO END.
//
// 🔴 WHY THIS FILE EXISTS, AND IT IS NOT REDUNDANT WITH
// doctor_exitcode_test.go. That file hands `exitCode` a HAND-BUILT error and
// checks the routing — which is a claim about the mapper, not about the command.
// A mutation battery caught the gap: tagging the REAL verdict with
// `civitai.ErrBadRequest` inside `runAppDoctor` moves the published exit code
// from 1 to 2, and the whole suite — that unit test included — stayed green,
// because nothing anywhere took the error the COMMAND actually returns and asked
// what code it produces.
//
// That is the "verified in isolation" shape: `internal/cmd` owns the verdict,
// `cmd/civitai` owns the mapping, both were tested, and the SEAM between them
// was owned by nobody. So this file builds the real binary, drives it against a
// fake `appListings.listMine`, and reads the process's own status — the number
// `civitai app doctor my-app || exit 1` actually branches on.
//
// It measures all FOUR arms. Three of them are the same number twice over (0),
// and that is the point: with only the blocking arm, an implementation that
// always exits 1 would pass.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// doctorListMinePath is spelled here rather than imported: this package cannot
// see internal/appapi's unexported route vars, and a literal is what makes the
// fake answer the route the binary really asks for.
const doctorListMinePath = "/api/trpc/appListings.listMine"

// e2eDoctorProblem / e2eDoctorRow build server-shaped fixtures. The slugs and
// ids are distinct from anything asserted, so no assertion can pass on a value
// copied from elsewhere.
func e2eDoctorProblem(code, label, severity string) map[string]any {
	return map[string]any{"code": code, "label": label, "severity": severity}
}

func e2eDoctorRow(problems ...map[string]any) map[string]any {
	if problems == nil {
		problems = []map[string]any{}
	}
	return map[string]any{
		"appListingId": "apl_E2E77",
		"slug":         "e2e-doctor-app",
		"name":         "E2E Doctor App",
		"status":       "draft",
		"role":         "owner",
		"appBlockId":   nil,
		"problems":     problems,
	}
}

// runCLIAnyStatus is runCLI's sibling for a command that may legitimately
// SUCCEED. runCLI t.Fatals on a zero status (it exists for failure paths), which
// is exactly the arm this file must be able to observe.
func runCLIAnyStatus(t *testing.T, bin string, env []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("could not run the binary: %v", err)
	}
	return ee.ExitCode(), stdout.String(), stderr.String()
}

func TestDoctorProcessExitStatusEndToEnd(t *testing.T) {
	bin := buildCLI(t)

	cases := []struct {
		name     string
		rows     []map[string]any
		args     []string
		want     int
		wantWhy  string
		mustSay  string
		mustExit string
	}{
		{
			name: "a blocking problem fails the build",
			rows: []map[string]any{e2eDoctorRow(
				e2eDoctorProblem("missing-icon", "Missing icon (required before publishing)", "blocking"))},
			args:    []string{"app", "doctor"},
			want:    exitGeneric,
			wantWhy: "a verdict about the LISTING, the same class as an invalid manifest — not a usage mistake",
			mustSay: "missing-icon",
		},
		{
			name: "blocked-media is blocking too",
			rows: []map[string]any{e2eDoctorRow(
				e2eDoctorProblem("blocked-media", "Replace the blocked icon before it can publish", "blocking"))},
			args:    []string{"app", "doctor"},
			want:    exitGeneric,
			wantWhy: "an asset the scan BLOCKED stops a publish exactly as a missing one does",
			mustSay: "blocked-media",
		},
		{
			name: "advisories alone do not fail the build",
			rows: []map[string]any{e2eDoctorRow(
				e2eDoctorProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory"),
				e2eDoctorProblem("scanning-media", "Icon is still being scanned", "advisory"))},
			args:    []string{"app", "doctor"},
			want:    0,
			wantWhy: "an advisory holds nothing up, so a release script must not stop on one",
			mustSay: "no-screenshots",
		},
		{
			name:    "a complete listing exits clean",
			rows:    []map[string]any{e2eDoctorRow()},
			args:    []string{"app", "doctor"},
			want:    0,
			wantWhy: "nothing is wrong",
			mustSay: "No problems",
		},
		{
			name: "--json uses the same codes",
			rows: []map[string]any{e2eDoctorRow(
				e2eDoctorProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"))},
			args:    []string{"app", "doctor", "--json"},
			want:    exitGeneric,
			wantWhy: "a script must be able to branch on the code before parsing the payload",
			mustSay: `"ok": false`,
		},
		{
			name:    "an unknown slug is NOT FOUND, not a pass",
			rows:    []map[string]any{e2eDoctorRow()},
			args:    []string{"app", "doctor", "not-an-app-of-mine"},
			want:    exitNotFound,
			wantWhy: "`you own no app called that` and `that app is fine` are opposite answers",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := tc.rows
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != doctorListMinePath {
					t.Errorf("unexpected request to %s", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"result": map[string]any{"data": map[string]any{"json": rows}},
				})
			}))
			defer srv.Close()

			env := []string{
				"HOME=" + t.TempDir(),
				"XDG_CONFIG_HOME=" + t.TempDir(),
				"CIVITAI_TOKEN=tok-e2e",
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_NO_UPDATE_CHECK=1",
				"NO_COLOR=1",
			}
			rc, stdout, stderr := runCLIAnyStatus(t, bin, env, tc.args...)
			if rc != tc.want {
				t.Errorf("`civitai %s` exited %d, want %d — %s.\nstdout:\n%s\nstderr:\n%s",
					strings.Join(tc.args, " "), rc, tc.want, tc.wantWhy, stdout, stderr)
			}
			if tc.mustSay != "" && !strings.Contains(stdout, tc.mustSay) {
				t.Errorf("stdout does not carry %q:\n%s", tc.mustSay, stdout)
			}
		})
	}

	// 🔴 NEGATIVE CONTROL ON THE HARNESS. Every number above is read from the
	// same runner, so a runner that always reported the same status would make
	// the whole table agree with itself. A usage mistake must come back as 2 —
	// a code no row above expects.
	t.Run("control: the harness observes a DIFFERENT code", func(t *testing.T) {
		rc, _, stderr := runCLIAnyStatus(t, buildCLI(t),
			[]string{"HOME=" + t.TempDir(), "NO_COLOR=1", "CIVITAI_NO_UPDATE_CHECK=1"},
			"app", "doctor", "--no-such-flag")
		if rc != exitUsage {
			t.Errorf("a bad flag exited %d, want %d — the harness is not observing differentiated codes.\nstderr: %s",
				rc, exitUsage, stderr)
		}
	})
}
