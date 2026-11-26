package main

// THE PROCESS EXIT STATUS OF `civitai app submit`'s OUTDATED-SDK GUARD,
// MEASURED END TO END. Modelled on submit_listing_gate_exitcode_test.go, for the
// same reason: the published code (exitcodes_doc.go: 1) is produced by
// exitCode in this package from a sentinel tagged in internal/cmd, and only the
// built binary observes the seam between them.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const e2eSDKFloorPath = "/api/v1/blocks/sdk-floor"

// e2eSDKProject is e2eGateProject plus a package.json and a package-lock.json
// installing @civitai/sdk 0.31.4 (the range says ^0.33.0, so a guard that read
// the range would not refuse).
func e2eSDKProject(t *testing.T) string {
	t.Helper()
	dir := e2eGateProject(t)
	pkg := `{"name":"e2e-sdk-app","dependencies":{"@civitai/sdk":"^0.33.0"}}`
	lock := `{"lockfileVersion":3,"packages":{"":{"dependencies":{"@civitai/sdk":"^0.33.0"}},` +
		`"node_modules/@civitai/sdk":{"version":"0.31.4"}}}`
	for name, body := range map[string]string{"package.json": pkg, "package-lock.json": lock} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSubmitSDKFloorProcessExitStatusEndToEnd(t *testing.T) {
	bin := buildCLI(t)

	for _, tc := range []struct {
		name        string
		floorStatus int
		args        []string
		noToken     bool
		want        int
		wantWhy     string
		mustSayErr  string
		mustSayOut  string
		wantSubmits int
	}{
		{
			name:        "an SDK below the published minimum refuses with the GENERIC code",
			floorStatus: http.StatusOK,
			args:        []string{"app", "submit", "--yes", "--skip-validate"},
			want:        exitGeneric,
			wantWhy:     "a verdict about the PROJECT, the same class as the listing gate and the version guard",
			mustSayErr:  "Error: refusing to submit — package-lock.json installs SDK packages below the platform minimum",
		},
		{
			name:        "--allow-outdated-sdk submits and exits 0",
			floorStatus: http.StatusOK,
			args:        []string{"app", "submit", "--yes", "--skip-validate", "--allow-outdated-sdk"},
			want:        exitOK,
			wantWhy:     "the override must have a way through",
			mustSayOut:  "pending moderator review",
			wantSubmits: 1,
		},
		{
			name:        "a server with no published minimum exits 0",
			floorStatus: http.StatusNotFound,
			args:        []string{"app", "submit", "--yes", "--skip-validate"},
			want:        exitOK,
			wantWhy:     "an older server is never a refusal",
			mustSayErr:  "publishes no SDK minimum",
			wantSubmits: 1,
		},
		{
			name:        "the no-token path STILL exits 0, and says NOT SUBMITTED",
			floorStatus: http.StatusOK,
			args:        []string{"app", "submit", "--yes", "--skip-validate"},
			noToken:     true,
			want:        exitOK,
			wantWhy:     "the pre-existing zero the refusal must differ from",
			mustSayOut:  "NOT SUBMITTED",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			submits := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == e2eSDKFloorPath:
					w.WriteHeader(tc.floorStatus)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"packages": []map[string]string{{"name": "@civitai/sdk", "minimum": "0.33.0"}},
					})
				case strings.HasPrefix(r.URL.Path, gateSubmissionsPath):
					_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
				case strings.HasPrefix(r.URL.Path, gateListMinePath):
					_ = json.NewEncoder(w).Encode(map[string]any{
						"result": map[string]any{"data": map[string]any{"json": []map[string]any{e2eGateRow("onsite")}}},
					})
				default:
					submits++
					_ = json.NewEncoder(w).Encode(map[string]any{
						"publishRequestId": "pubreq_e2e", "slug": e2eGateSlug,
						"version": "0.3.0", "status": "pending",
					})
				}
			}))
			defer srv.Close()

			args := append([]string{}, tc.args...)
			args = append(args, "-o", filepath.Join(t.TempDir(), "bundle.zip"), e2eSDKProject(t))
			env := []string{
				"HOME=" + t.TempDir(),
				"XDG_CONFIG_HOME=" + t.TempDir(),
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_SUBMIT_PATH=/api/v1/blocks/submit-version",
				"CIVITAI_NO_UPDATE_CHECK=1",
				"NO_COLOR=1",
			}
			if !tc.noToken {
				env = append(env, "CIVITAI_TOKEN=tok-e2e-sdk")
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
			if submits != tc.wantSubmits {
				t.Errorf("the submit route was hit %d time(s), want %d", submits, tc.wantSubmits)
			}
		})
	}
}
