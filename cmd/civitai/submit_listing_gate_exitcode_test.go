package main

// THE PROCESS EXIT STATUS OF `civitai app submit`'s LISTING-COMPLETENESS GATE,
// MEASURED END TO END (civitai/cli#762).
//
// 🔴 WHY THIS FILE EXISTS AT THE PROCESS LEVEL AND NOT ONLY IN `internal/cmd`.
// The whole defect class being closed is "a command that did not do the thing
// still exits 0". `civitai app submit` ALREADY has such a path — with no token it
// writes the .zip, prints `⚠ NOT SUBMITTED` and returns nil — so the refusal's
// value depends entirely on the number a caller's `||` branches on being
// DIFFERENT from that one. That number is produced by `exitCode` in this package
// from a sentinel returned by another, and the seam between them is owned by
// nobody: the sibling `doctor_e2e_exitcode_test.go` exists because a mutation
// that re-tagged the verdict moved the published code from 1 to 2 with the whole
// suite green.
//
// So this builds the real binary, drives it against a fake server, and reads the
// process status — and it measures the no-token path in the SAME table, because
// "the refusal exits non-zero" and "the no-token path still exits zero" are one
// claim about a pair of numbers, not two independent facts.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gateListMinePath = "/api/trpc/appListings.listMine"
const gateSubmissionsPath = "/api/v1/blocks/submissions"
const e2eGateSlug = "e2e-gate-app"

func e2eGateProblem(code, label, severity string) map[string]any {
	return map[string]any{"code": code, "label": label, "severity": severity}
}

func e2eGateRow(kind string, problems ...map[string]any) map[string]any {
	if problems == nil {
		problems = []map[string]any{}
	}
	return map[string]any{
		"appListingId": "apl_E2EGATE",
		"slug":         e2eGateSlug,
		"name":         "E2E Gate App",
		"status":       "draft",
		"role":         "owner",
		"appBlockId":   nil,
		"kind":         kind,
		"problems":     problems,
	}
}

// e2eGateProject writes a minimal valid project whose blockId is e2eGateSlug and
// which carries NO tagline — the shape every app scaffolded before this change
// has.
func e2eGateProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	m := `{
  "$schema": "https://civitai.com/schemas/app-block/v1.json",
  "blockId": "` + e2eGateSlug + `",
  "version": "0.3.0",
  "name": "E2E Gate App",
  "type": "block",
  "scopes": [],
  "page": { "path": "/", "title": "E2E Gate App", "icon": "bolt" },
  "iframe": { "minHeight": 400, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts allow-forms" },
  "contentRating": "g",
  "minApiVersion": "1.0"
}`
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSubmitListingGateProcessExitStatusEndToEnd(t *testing.T) {
	bin := buildCLI(t)

	for _, tc := range []struct {
		name    string
		rows    []map[string]any
		args    []string
		noToken bool
		want    int
		wantWhy string
		// mustSay is asserted on STDERR (the refusal) or STDOUT (the banner).
		mustSayErr string
		mustSayOut string
	}{
		{
			name: "an incomplete listing refuses with the GENERIC code",
			rows: []map[string]any{e2eGateRow("onsite",
				e2eGateProblem("missing-icon", "Missing icon (required before publishing)", "blocking"))},
			args: []string{"app", "submit", "--yes"},
			want: exitGeneric,
			wantWhy: "a verdict about the PROJECT, the same class as a version regression or an invalid " +
				"manifest — not a usage mistake about the invocation",
			mustSayErr: "missing-icon",
		},
		{
			name: "the refusal keeps the `Error: ` prefix",
			rows: []map[string]any{e2eGateRow("onsite",
				e2eGateProblem("missing-cover", "Missing cover image (required before publishing)", "blocking"))},
			args: []string{"app", "submit", "--yes"},
			want: exitGeneric,
			wantWhy: "unlike `app doctor`, whose job IS to return a verdict, a refused submit is a failure " +
				"of what the caller asked for — errorLine's bare-render whitelist is one entry and this " +
				"is not it",
			mustSayErr: "Error: refusing to submit",
		},
		{
			name: "the escape hatch submits and exits 0",
			rows: []map[string]any{e2eGateRow("onsite",
				e2eGateProblem("missing-icon", "Missing icon (required before publishing)", "blocking"))},
			args:       []string{"app", "submit", "--yes", "--allow-incomplete-listing"},
			want:       exitOK,
			wantWhy:    "a human submitting early on purpose must have a way through",
			mustSayOut: "pending moderator review",
		},
		{
			name: "a permitted problem exits 0 and is still reported",
			rows: []map[string]any{e2eGateRow("offsite",
				e2eGateProblem("no-screenshots", "No screenshots (recommended, optional)", "advisory"))},
			args:       []string{"app", "submit", "--yes"},
			want:       exitOK,
			wantWhy:    "screenshots are documented recommended-and-optional; a gate on them buys junk media",
			mustSayErr: "no-screenshots",
		},
		{
			name:    "the no-token path STILL exits 0, and says NOT SUBMITTED",
			rows:    []map[string]any{e2eGateRow("onsite", e2eGateProblem("missing-icon", "Missing icon", "blocking"))},
			args:    []string{"app", "submit", "--yes"},
			noToken: true,
			want:    exitOK,
			wantWhy: "this pre-existing zero is WHY the refusal above may not be one: a script cannot tell " +
				"a refusal from it if both are 0",
			mustSayOut: "NOT SUBMITTED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := tc.rows
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasPrefix(r.URL.Path, gateSubmissionsPath):
					_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
				case strings.HasPrefix(r.URL.Path, gateListMinePath):
					_ = json.NewEncoder(w).Encode(map[string]any{
						"result": map[string]any{"data": map[string]any{"json": rows}},
					})
				default:
					_ = json.NewEncoder(w).Encode(map[string]any{
						"publishRequestId": "pubreq_e2e", "slug": e2eGateSlug,
						"version": "0.3.0", "status": "pending",
					})
				}
			}))
			defer srv.Close()

			// The bundle the no-token path writes lands in the process CWD, so it
			// is directed at a temp path rather than at the repo.
			args := append([]string{}, tc.args...)
			args = append(args, "-o", filepath.Join(t.TempDir(), "bundle.zip"), e2eGateProject(t))

			env := []string{
				"HOME=" + t.TempDir(),
				"XDG_CONFIG_HOME=" + t.TempDir(),
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_SUBMIT_PATH=/api/v1/blocks/submit-version",
				"CIVITAI_NO_UPDATE_CHECK=1",
				"NO_COLOR=1",
			}
			if !tc.noToken {
				env = append(env, "CIVITAI_TOKEN=tok-e2e-gate")
			}

			rc, stdout, stderr := runCLIAnyStatus(t, bin, env, args...)
			if rc != tc.want {
				t.Errorf("`civitai %s` exited %d, want %d — %s.\nstdout:\n%s\nstderr:\n%s",
					strings.Join(args, " "), rc, tc.want, tc.wantWhy, stdout, stderr)
			}
			if tc.mustSayErr != "" && !strings.Contains(stderr, tc.mustSayErr) {
				t.Errorf("stderr does not carry %q:\n%s", tc.mustSayErr, stderr)
			}
			if tc.mustSayOut != "" && !strings.Contains(stdout, tc.mustSayOut) {
				t.Errorf("stdout does not carry %q:\n%s", tc.mustSayOut, stdout)
			}
		})
	}

	// 🔴 NEGATIVE CONTROL ON THE HARNESS. Every number above comes from one
	// runner, so a runner that always reported the same status would make the
	// table agree with itself. A usage mistake must come back as 2 — a code no row
	// above expects, and the code this refusal deliberately is NOT.
	t.Run("control: the harness observes a DIFFERENT code", func(t *testing.T) {
		rc, _, stderr := runCLIAnyStatus(t, bin,
			[]string{"HOME=" + t.TempDir(), "NO_COLOR=1", "CIVITAI_NO_UPDATE_CHECK=1"},
			"app", "submit", "--no-such-flag")
		if rc != exitUsage {
			t.Errorf("a bad flag exited %d, want %d — the harness is not observing differentiated codes.\nstderr: %s",
				rc, exitUsage, stderr)
		}
	})
}
