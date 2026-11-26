package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// THE OUTDATED-SDK GUARD (app_submit_sdk_floor_guard.go).
//
// Fixture versions differ from the published floors (0.33.0 / 0.87.0) in every
// part, and package.json's RANGE is chosen to disagree with the lockfile, so a
// guard that read the range — or hardcoded a floor — gives a different answer.

// platformFloors is what the server publishes today.
var platformFloors = []appapi.SDKFloor{
	{Name: "@civitai/sdk", Minimum: "0.33.0"},
	{Name: "@civitai/blocks-react", Minimum: "0.87.0"},
}

func floorsReader(floors []appapi.SDKFloor) sdkFloorReader {
	return func(context.Context) ([]appapi.SDKFloor, error) { return floors, nil }
}

// sdkProject writes package.json (with the given ranges) and a package-lock.json
// v3 installing the given versions. A "" version leaves the package out of the
// lockfile.
func sdkProject(t *testing.T, ranges map[string]string, installed map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	pkg, _ := json.Marshal(map[string]any{"name": "app", "dependencies": ranges})
	if err := os.WriteFile(filepath.Join(dir, "package.json"), pkg, 0o600); err != nil {
		t.Fatal(err)
	}
	packages := map[string]any{"": map[string]any{"dependencies": ranges}}
	for name, v := range installed {
		if v != "" {
			packages["node_modules/"+name] = map[string]any{"version": v}
		}
	}
	lock, _ := json.Marshal(map[string]any{"lockfileVersion": 3, "packages": packages})
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), lock, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runSDKGuard(t *testing.T, read sdkFloorReader, dir, buildCommand string, allow bool) ([]appapi.OutdatedSDK, string, error) {
	t.Helper()
	var warn bytes.Buffer
	got, err := checkSDKFloor(context.Background(), read, &warn, dir, buildCommand, allow)
	return got, warn.String(), err
}

// TestSDKFloorRefusesBelowTheMinimumNamingTheFlag pins the whole refusal: the
// package, installed version, minimum, the upgrade command and the flag.
func TestSDKFloorRefusesBelowTheMinimumNamingTheFlag(t *testing.T) {
	dir := sdkProject(t,
		map[string]string{"@civitai/sdk": "^0.33.0", "@civitai/blocks-react": "^0.87.0"},
		map[string]string{"@civitai/sdk": "0.31.4", "@civitai/blocks-react": "0.80.2"})

	got, warn, err := runSDKGuard(t, floorsReader(platformFloors), dir, "npm run build", false)
	if err == nil {
		t.Fatalf("an app installing @civitai/sdk 0.31.4 under a 0.33.0 minimum must be refused; got %v", got)
	}
	if !errors.Is(err, ErrOutdatedSDK) {
		t.Errorf("the refusal must carry ErrOutdatedSDK, got %#v", err)
	}
	want := "refusing to submit — package-lock.json installs SDK packages below the platform minimum:\n" +
		"  @civitai/sdk 0.31.4 (platform minimum 0.33.0)\n" +
		"  @civitai/blocks-react 0.80.2 (platform minimum 0.87.0)\n" +
		"Upgrade, commit the updated package-lock.json, and submit again:\n" +
		"  npm install @civitai/sdk@latest @civitai/blocks-react@latest\n" +
		"Or pass --allow-outdated-sdk to submit anyway; the moderator who reviews it is shown the versions and the override."
	if err.Error() != want {
		t.Errorf("refusal text:\n got: %q\nwant: %q", err.Error(), want)
	}
	if warn != "" {
		t.Errorf("a refusal prints nothing else, got:\n%s", warn)
	}
}

// TestSDKFloorReadsTheLockfileNotTheRange: package.json's range is BELOW the
// minimum, the installed version is above it — and it is the installed one
// that counts. The mirror image of the refusal fixture above, whose range was
// AT the minimum while the lockfile was below it.
func TestSDKFloorReadsTheLockfileNotTheRange(t *testing.T) {
	dir := sdkProject(t,
		map[string]string{"@civitai/sdk": "^0.30.0"},
		map[string]string{"@civitai/sdk": "0.34.1"})
	got, warn, err := runSDKGuard(t, floorsReader(platformFloors), dir, "", false)
	if err != nil || got != nil || warn != "" {
		t.Fatalf("0.34.1 installed is above 0.33.0 whatever the range says; got %v, %v, warn %q", got, err, warn)
	}
}

// TestSDKFloorOverrideProceedsAndReturnsWhatItOverrode: --allow-outdated-sdk
// never refuses, and hands back the literal list the submit sends.
func TestSDKFloorOverrideProceedsAndReturnsWhatItOverrode(t *testing.T) {
	dir := sdkProject(t,
		map[string]string{"@civitai/sdk": "^0.31.0"},
		map[string]string{"@civitai/sdk": "0.31.4"})
	got, warn, err := runSDKGuard(t, floorsReader(platformFloors), dir, "", true)
	if err != nil {
		t.Fatalf("--allow-outdated-sdk must not refuse: %v", err)
	}
	want := []appapi.OutdatedSDK{{Package: "@civitai/sdk", Installed: "0.31.4", Minimum: "0.33.0"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("override = %#v, want %#v", got, want)
	}
	if !strings.Contains(warn, "submitting with --allow-outdated-sdk") ||
		!strings.Contains(warn, "@civitai/sdk 0.31.4 (platform minimum 0.33.0)") ||
		!strings.Contains(warn, "shown to the moderator who reviews it") {
		t.Errorf("the override must say what it overrode and who sees it, got:\n%s", warn)
	}
}

// TestSDKFloorSkipsWhenTheServerPublishesNone: an older server (404) gets a
// one-line note and never a refusal — on a project the guard WOULD refuse.
func TestSDKFloorSkipsWhenTheServerPublishesNone(t *testing.T) {
	dir := sdkProject(t,
		map[string]string{"@civitai/sdk": "^0.31.0"},
		map[string]string{"@civitai/sdk": "0.31.4"})
	notPublished := func(context.Context) ([]appapi.SDKFloor, error) { return nil, appapi.ErrSDKFloorNotPublished }

	got, warn, err := runSDKGuard(t, notPublished, dir, "", false)
	if err != nil || got != nil {
		t.Fatalf("no published minimum must never refuse; got %v, %v", got, err)
	}
	if want := "Note: this server publishes no SDK minimum, so the outdated-SDK check is skipped.\n"; warn != want {
		t.Errorf("note = %q, want %q", warn, want)
	}
}

// TestSDKFloorWarnsAndProceedsWhenTheReadFails: any other failure is a
// warning, not a refusal.
func TestSDKFloorWarnsAndProceedsWhenTheReadFails(t *testing.T) {
	dir := sdkProject(t, map[string]string{"@civitai/sdk": "^0.31.0"}, map[string]string{"@civitai/sdk": "0.31.4"})
	failing := func(context.Context) ([]appapi.SDKFloor, error) { return nil, errors.New("server returned 502") }
	got, warn, err := runSDKGuard(t, failing, dir, "", false)
	if err != nil || got != nil {
		t.Fatalf("a failed read must not refuse; got %v, %v", got, err)
	}
	if !strings.Contains(warn, "could not read the platform's SDK minimum (server returned 502)") {
		t.Errorf("the failed read must be announced, got %q", warn)
	}
}

// TestSDKFloorSkipsAnAppOnNeitherPackage: nothing to check, nothing printed.
func TestSDKFloorSkipsAnAppOnNeitherPackage(t *testing.T) {
	dir := sdkProject(t, map[string]string{"react": "^19.0.0"}, map[string]string{"react": "19.0.0"})
	got, warn, err := runSDKGuard(t, floorsReader(platformFloors), dir, "", false)
	if err != nil || got != nil || warn != "" {
		t.Fatalf("an app on neither package is not checked; got %v, %v, warn %q", got, err, warn)
	}
}

// TestSDKFloorRefusesBlocksReactAlone: @civitai/sdk is fine, blocks-react is
// not — only blocks-react is named.
func TestSDKFloorRefusesBlocksReactAlone(t *testing.T) {
	dir := sdkProject(t,
		map[string]string{"@civitai/sdk": "^0.35.0", "@civitai/blocks-react": "^0.80.0"},
		map[string]string{"@civitai/sdk": "0.35.2", "@civitai/blocks-react": "0.80.2"})
	_, _, err := runSDKGuard(t, floorsReader(platformFloors), dir, "", false)
	if !errors.Is(err, ErrOutdatedSDK) {
		t.Fatalf("blocks-react 0.80.2 under 0.87.0 must refuse, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "  @civitai/blocks-react 0.80.2 (platform minimum 0.87.0)\n") ||
		!strings.Contains(msg, "  npm install @civitai/blocks-react@latest\n") {
		t.Errorf("must name blocks-react and its upgrade, got:\n%s", msg)
	}
	if strings.Contains(msg, "@civitai/sdk ") {
		t.Errorf("@civitai/sdk 0.35.2 is above its minimum and must not be named, got:\n%s", msg)
	}
}

// TestSDKFloorJudgesTheServersMinimumNotAConstant: the same project refused
// under one published minimum passes under a lower one, and the reverse.
func TestSDKFloorJudgesTheServersMinimumNotAConstant(t *testing.T) {
	dir := sdkProject(t, map[string]string{"@civitai/sdk": "^0.31.0"}, map[string]string{"@civitai/sdk": "0.31.4"})
	lower := []appapi.SDKFloor{{Name: "@civitai/sdk", Minimum: "0.31.3"}}
	if _, _, err := runSDKGuard(t, floorsReader(lower), dir, "", false); err != nil {
		t.Errorf("0.31.4 is above a 0.31.3 minimum: %v", err)
	}
	higher := []appapi.SDKFloor{{Name: "@civitai/sdk", Minimum: "1.21.3"}}
	_, _, err := runSDKGuard(t, floorsReader(higher), dir, "", false)
	if err == nil || !strings.Contains(err.Error(), "@civitai/sdk 0.31.4 (platform minimum 1.21.3)") {
		t.Errorf("0.31.4 is below a 1.21.3 minimum and the message names the server's number, got %v", err)
	}
}

// TestSDKFloorPnpmLockfileNamesPnpmAdd: the lockfile and upgrade command follow
// the package manager the build uses.
func TestSDKFloorPnpmLockfileNamesPnpmAdd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"),
		[]byte(`{"dependencies":{"@civitai/sdk":"^0.33.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pnpm := "lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n      '@civitai/sdk':\n        specifier: ^0.33.0\n        version: 0.32.5\n"
	if err := os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(pnpm), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runSDKGuard(t, floorsReader(platformFloors), dir, "pnpm run build", false)
	if err == nil {
		t.Fatal("pnpm-lock.yaml installing 0.32.5 must refuse")
	}
	if !strings.Contains(err.Error(), "pnpm-lock.yaml installs SDK packages below") ||
		!strings.Contains(err.Error(), "  pnpm add @civitai/sdk@latest\n") {
		t.Errorf("must name the pnpm lockfile and `pnpm add`, got:\n%s", err)
	}
}

// TestSDKFloorWarnsWhenADeclaredPackageIsNotInTheLockfile: the app depends on
// the SDK but the lockfile cannot say which version — the check could not be
// made, so it says so rather than passing silently.
func TestSDKFloorWarnsWhenADeclaredPackageIsNotInTheLockfile(t *testing.T) {
	dir := sdkProject(t, map[string]string{"@civitai/sdk": "^0.31.0"}, map[string]string{"@civitai/sdk": ""})
	got, warn, err := runSDKGuard(t, floorsReader(platformFloors), dir, "", false)
	if err != nil || got != nil {
		t.Fatalf("an unreadable version is not a refusal; got %v, %v", got, err)
	}
	if !strings.Contains(warn, "cannot read the installed version of @civitai/sdk from package-lock.json") {
		t.Errorf("the unchecked package must be named, got %q", warn)
	}
}

// --- END TO END: the flag, the wiring, and what reaches the server ---

// sdkFloorServer answers the floor, the submissions and listing reads, and
// records the submit body. floorStatus 404 plays an older server.
func sdkFloorServer(t *testing.T, floorStatus int) (*httptest.Server, *[]byte) {
	t.Helper()
	var submitBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == appapi.SDKFloorPath:
			if floorStatus != http.StatusOK {
				w.WriteHeader(floorStatus)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"packages": platformFloors})
		case strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath):
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
		case strings.HasPrefix(r.URL.Path, listMineTestPath):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"data": map[string]any{"json": []any{}}},
			})
		case r.URL.Path == "/api/v1/blocks/submit-version":
			submitBody, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishRequestId": "pubreq_new", "slug": guardSlug, "version": "0.6.1", "status": "pending",
			})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &submitBody
}

func outdatedSDKApp(t *testing.T) string {
	t.Helper()
	dir := sdkProject(t,
		map[string]string{"@civitai/sdk": "^0.33.0"},
		map[string]string{"@civitai/sdk": "0.31.4"})
	writeManifestVersion(t, dir, guardSlug, "0.6.1")
	return dir
}

func TestAppSubmitRefusesAnOutdatedSDKAndNeverUploads(t *testing.T) {
	withStdinTTY(t, false)
	dir := outdatedSDKApp(t)
	srv, body := sdkFloorServer(t, http.StatusOK)
	withGuardEnv(t, srv)

	stdout, _, err := run(t, "app", "submit", dir, "--yes", "--skip-validate")
	if !errors.Is(err, ErrOutdatedSDK) {
		t.Fatalf("must refuse with ErrOutdatedSDK, got %v\nstdout:\n%s", err, stdout)
	}
	if !strings.Contains(err.Error(), "--allow-outdated-sdk") {
		t.Errorf("the refusal must name the override flag, got:\n%s", err)
	}
	if *body != nil {
		t.Error("the submit route was hit — the guard must refuse before uploading")
	}
	if strings.Contains(stdout, "Packaged ") {
		t.Errorf("the guard must run before packaging, got stdout:\n%s", stdout)
	}
}

func TestAppSubmitAllowOutdatedSDKSendsTheOverride(t *testing.T) {
	withStdinTTY(t, false)
	dir := outdatedSDKApp(t)
	srv, body := sdkFloorServer(t, http.StatusOK)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", dir, "--yes", "--skip-validate", "--allow-outdated-sdk")
	if err != nil {
		t.Fatalf("--allow-outdated-sdk must submit; got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if *body == nil {
		t.Fatal("--allow-outdated-sdk must reach the submit route")
	}
	var sent struct {
		SDKFloorOverride []map[string]string `json:"sdkFloorOverride"`
	}
	if err := json.Unmarshal(*body, &sent); err != nil {
		t.Fatalf("submit body: %v", err)
	}
	want := []map[string]string{{"package": "@civitai/sdk", "installed": "0.31.4", "minimum": "0.33.0"}}
	if !reflect.DeepEqual(sent.SDKFloorOverride, want) {
		t.Errorf("sdkFloorOverride = %#v, want %#v", sent.SDKFloorOverride, want)
	}
}

// TestAppSubmitWithoutAnOverrideSendsNoOverrideKey: an at-minimum app sends
// the body it sent before the field existed.
func TestAppSubmitWithoutAnOverrideSendsNoOverrideKey(t *testing.T) {
	withStdinTTY(t, false)
	dir := sdkProject(t, map[string]string{"@civitai/sdk": "^0.33.0"}, map[string]string{"@civitai/sdk": "0.33.0"})
	writeManifestVersion(t, dir, guardSlug, "0.6.1")
	srv, body := sdkFloorServer(t, http.StatusOK)
	withGuardEnv(t, srv)

	if _, stderr, err := run(t, "app", "submit", dir, "--yes", "--skip-validate"); err != nil {
		t.Fatalf("an at-minimum app must submit: %v\nstderr:\n%s", err, stderr)
	}
	if bytes.Contains(*body, []byte("sdkFloorOverride")) {
		t.Errorf("no override was made, so no key may be sent; body: %.200s", *body)
	}
}

// TestAppSubmitOnAServerWithNoFloorSkipsWithANote: the refused project above,
// against an older server, submits with the one-line note.
func TestAppSubmitOnAServerWithNoFloorSkipsWithANote(t *testing.T) {
	withStdinTTY(t, false)
	dir := outdatedSDKApp(t)
	srv, body := sdkFloorServer(t, http.StatusNotFound)
	withGuardEnv(t, srv)

	_, stderr, err := run(t, "app", "submit", dir, "--yes", "--skip-validate")
	if err != nil {
		t.Fatalf("no published minimum must never refuse: %v\nstderr:\n%s", err, stderr)
	}
	if *body == nil {
		t.Error("the submit must go ahead")
	}
	if !strings.Contains(stderr, "Note: this server publishes no SDK minimum, so the outdated-SDK check is skipped.") {
		t.Errorf("stderr must carry the one-line note, got:\n%s", stderr)
	}
}

// TestAppSubmitPackageOnlyNeverReadsTheFloor: --package-only never reaches the
// server, so it neither reads the minimum nor refuses on it.
func TestAppSubmitPackageOnlyNeverReadsTheFloor(t *testing.T) {
	dir := outdatedSDKApp(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	t.Cleanup(srv.Close)
	withGuardEnv(t, srv)

	out := filepath.Join(t.TempDir(), "bundle.zip")
	if _, stderr, err := run(t, "app", "submit", dir, "--package-only", "--skip-validate", "--out", out); err != nil {
		t.Fatalf("--package-only must not be refused by the SDK guard: %v\nstderr:\n%s", err, stderr)
	}
	if hits != 0 {
		t.Errorf("--package-only made %d request(s); it must make none", hits)
	}
}

func TestAppSubmitHasTheAllowOutdatedSDKFlag(t *testing.T) {
	c := newAppSubmitCmd()
	f := c.Flags().Lookup(strings.TrimPrefix(AllowOutdatedSDKFlag, "--"))
	if f == nil {
		t.Fatal("`app submit` must register --allow-outdated-sdk")
	}
	if f.DefValue != "false" {
		t.Errorf("--allow-outdated-sdk must default off, got %q", f.DefValue)
	}
	if !strings.Contains(c.Long, "--allow-outdated-sdk") {
		t.Error("`app submit --help` must document --allow-outdated-sdk")
	}
}
