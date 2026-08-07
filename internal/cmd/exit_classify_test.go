package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// TestCheckLimitIsUsageTagged asserts an out-of-range --limit is classified as a
// USAGE error (exit 2), not the generic one. checkLimit is the single shared
// validator, so tagging it there classifies EVERY read subcommand at once.
func TestCheckLimitIsUsageTagged(t *testing.T) {
	err := checkLimit(999, 100)
	if err == nil {
		t.Fatal("expected an error for an out-of-range limit")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("out-of-range limit should be a usage error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "between 1 and 100") {
		t.Errorf("message should be preserved, got: %v", err)
	}
}

// TestModelsSearchLimitUsageTaggedEndToEnd drives the real command so the tag is
// verified through the full RunE path (the bad flag value must reach exit 2).
func TestModelsSearchLimitUsageTaggedEndToEnd(t *testing.T) {
	_, _, err := run(t, "models", "search", "--limit", "999")
	if err == nil {
		t.Fatal("expected an error for --limit 999")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--limit 999 should classify as usage, got: %v", err)
	}
}

// TestModelVersionsGetNonIntIsUsageTagged asserts a non-integer positional id is
// a usage error, not a generic one.
func TestModelVersionsGetNonIntIsUsageTagged(t *testing.T) {
	_, _, err := run(t, "model-versions", "get", "abc")
	if err == nil {
		t.Fatal("expected an error for a non-integer model-version id")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("non-integer id should classify as usage, got: %v", err)
	}
	if !strings.Contains(err.Error(), "must be an integer") {
		t.Errorf("message should be preserved, got: %v", err)
	}
}

// TestUsersGetMissingIsNotFoundTagged asserts a lookup that resolves to zero
// users is classified as NOT-FOUND (exit 4) even though the search endpoint
// returns an empty 200, matching the behaviour of a real HTTP 404.
func TestUsersGetMissingIsNotFoundTagged(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	})
	_, _, err := run(t, "users", "get", "zzz-no-such-user-zzz")
	if err == nil {
		t.Fatal("expected an error for a missing user")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("missing user should classify as not-found, got: %v", err)
	}
}

// TestUsersGetNoExactMatchIsNotFoundTagged asserts a name query that returns
// only fuzzy neighbours (no exact username) is also classified as not-found.
func TestUsersGetNoExactMatchIsNotFoundTagged(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":5,"username":"bob"},{"id":6,"username":"bobby"}]}`))
	})
	_, _, err := run(t, "users", "get", "bo")
	if err == nil {
		t.Fatal("expected an error for a no-exact-match user query")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("no-exact-match user should classify as not-found, got: %v", err)
	}
}

// TestImagesSearchBadEnumIsBadRequestTaggedAndClean asserts an invalid enum
// (rejected by the server with a 400 ZodError body) is (a) classified as a
// bad-request/usage error and (b) surfaced as a CONCISE message — never the raw
// ZodError JSON blob.
func TestImagesSearchBadEnumIsBadRequestTaggedAndClean(t *testing.T) {
	const zodBody = `{"error":{"name":"ZodError","message":"[\n  {\n    \"code\": \"invalid_value\",\n    \"path\": [\n      \"period\"\n    ],\n    \"message\": \"Invalid option: expected one of \\\"Day\\\"|\\\"Week\\\"|\\\"Month\\\"|\\\"Year\\\"|\\\"AllTime\\\"\"\n  }\n]"}}`
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(zodBody))
	})
	_, _, err := run(t, "images", "search", "--period", "Bogus")
	if err == nil {
		t.Fatal("expected an error for an invalid --period enum")
	}
	if !errors.Is(err, civitai.ErrBadRequest) {
		t.Errorf("400 should classify as bad-request (usage), got: %v", err)
	}
	if strings.Contains(err.Error(), "ZodError") {
		t.Errorf("the raw ZodError blob must not leak into the message: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid request parameter (400)") {
		t.Errorf("message should be the concise 400 form, got: %v", err)
	}
	// The concise message should name the offending field.
	if !strings.Contains(err.Error(), "period") {
		t.Errorf("concise message should name the invalid field, got: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Client-side (non-HTTP-derived) failures — issue #224.
//
// Every case above reaches its classification through an HTTP status. The set
// below covers the failures the CLI resolves ENTIRELY LOCALLY: an unknown slug
// the server answers 200-with-an-empty-list, and bad flag VALUES that never
// leave the process. They used to fall through to the generic exit 1 while the
// docs promised 4 / 2.
//
// Each asserts BOTH the sentinel (errors.Is — what the exit code is derived
// from) AND that the message is unchanged (Tag/asUsageError add no text), per
// AGENTS.md item 7. The errors.Is half is the load-bearing one: a message-only
// assertion stays green when the classification is stripped.
// ----------------------------------------------------------------------------

// emptySubmissionsServer answers every request 200 with an EMPTY submissions
// list — the shape the real route returns for a `?blockId=<unknown slug>`
// lookup (it does NOT 404) — and points the CLI at it with a token configured.
func emptySubmissionsServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"submissions":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
}

// TestAppStatusBySlugUnknownIsNotFoundTagged pins `app status <slug>` to exit 4.
// The `--id` spelling of the same question 404s and was always classified; the
// slug spelling resolves client-side off an empty 200 and was not, so one
// question answered two ways produced two different exit codes.
func TestAppStatusBySlugUnknownIsNotFoundTagged(t *testing.T) {
	emptySubmissionsServer(t)
	_, _, err := run(t, "app", "status", "no-such-app-xyz")
	if err == nil {
		t.Fatal("expected an error for an unknown app slug")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("`app status <slug>` on an unknown slug must classify as not-found (exit 4), got %T: %v", err, err)
	}
	if err.Error() != "no such submission" {
		t.Errorf("classification must not change the message, got %q", err.Error())
	}
}

// TestAppListingStatusUnknownSlugIsNotFoundTagged pins the second command that
// resolves a slug through the same empty-list path.
func TestAppListingStatusUnknownSlugIsNotFoundTagged(t *testing.T) {
	emptySubmissionsServer(t)
	_, _, err := run(t, "app", "listing", "status", "--slug", "no-such-app-xyz")
	if err == nil {
		t.Fatal("expected an error for an unknown app slug")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("`app listing status --slug` on an unknown slug must classify as not-found (exit 4), got %T: %v", err, err)
	}
	if err.Error() != "no such submission" {
		t.Errorf("classification must not change the message, got %q", err.Error())
	}
}

// TestAppInitBadFlagValuesAreUsageTagged pins the client-side scaffolder
// refusals to exit 2. A bad flag VALUE is the same class of mistake as a bad
// flag NAME, which Cobra's FlagErrorFunc already tags — these did not, so
// `--template nope` exited 1 while `--nope` exited 2.
//
// `app create` is a thin alias for `app init`, so the same RunE covers it.
// Every case fails BEFORE anything is written to disk (the parse/arg checks run
// ahead of scaffold.Render), so no output directory is created.
func TestAppInitBadFlagValuesAreUsageTagged(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "unknown --template value",
			args:    []string{"app", "init", "my-app", "--template", "nope", "--yes"},
			wantMsg: `unknown template "nope" (valid: static, page-vite, page-money)`,
		},
		{
			name:    "unknown --template value via the create alias",
			args:    []string{"app", "create", "my-app", "--template", "nope", "--yes"},
			wantMsg: `unknown template "nope" (valid: static, page-vite, page-money)`,
		},
		{
			name:    "no project name",
			args:    []string{"app", "init", "--yes"},
			wantMsg: "provide a project name: civitai app init <name>",
		},
		{
			name:    "positional dir conflicts with --dir",
			args:    []string{"app", "init", "my-app", "here", "--dir", "there", "--yes"},
			wantMsg: `conflicting output directory: positional "here" and --dir "there"`,
		},
		{
			name:    "unusable project name",
			args:    []string{"app", "init", "!!", "--yes"},
			wantMsg: `cannot derive a valid slug from "!!" (need ≥3 chars; use lowercase letters/numbers/hyphens)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("a bad flag/arg VALUE must classify as usage (exit 2), got %T: %v", err, err)
			}
			if err.Error() != tc.wantMsg {
				t.Errorf("classification must not change the message:\n got: %q\nwant: %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// TestMissingArgsAndBadFlagValuesAreUsageTagged sweeps the remaining commands
// that refuse an invocation entirely client-side: a missing required
// argument/flag, an out-of-range flag value, and a mutually-exclusive pair. Each
// one used to exit 1 while its own message names the flag to fix — and each has
// a sibling elsewhere in the tree (app_metrics' "an app slug is required",
// download's "--layout must be one of…") that was already tagged, so the same
// mistake exited 1 or 2 depending only on which command you typed.
//
// None of these reaches the network, so no server is needed; a token is
// configured because a few of the commands check it before parsing.
func TestMissingArgsAndBadFlagValuesAreUsageTagged(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "app dev-token without a slug",
			args:    []string{"app", "dev-token"},
			wantMsg: "an app slug is required — e.g. `civitai app dev-token my-block` (find it with `civitai app status`)",
		},
		{
			name: "app dev-tunnel without a blockId",
			args: []string{"app", "dev-tunnel"},
			wantMsg: fmt.Sprintf("a blockId is required — pass it (`civitai app dev-tunnel my-block`), or run from an App directory containing %s "+
				"(list your submitted apps with `civitai app status`)", manifest.Filename),
		},
		{
			name:    "app dev-tunnel out-of-range --port",
			args:    []string{"app", "dev-tunnel", "my-block", "--port", "70000"},
			wantMsg: "invalid --port 70000 (must be 1-65535)",
		},
		{
			name:    "app dev-tunnel non-positive --idle-timeout",
			args:    []string{"app", "dev-tunnel", "my-block", "--idle-timeout", "0s"},
			wantMsg: "--idle-timeout must be positive (got 0s)",
		},
		{
			name:    "app dev-tunnel negative --ready-timeout",
			args:    []string{"app", "dev-tunnel", "my-block", "--ready-timeout", "-1s"},
			wantMsg: "--ready-timeout must be >= 0 (0 = wait indefinitely until ready or Ctrl-C; got -1s)",
		},
		{
			// Cobra's REQUIRED-flag validator, not the command's own check.
			// Cobra runs it after Args and after the PreRun hooks, so it is
			// invisible to SetFlagErrorFunc — `--app` omitted exited 1 while the
			// unknown flag `--nope` exited 2. Message is cobra's, unchanged.
			name:    "app pull with --app omitted entirely (cobra's required-flag error)",
			args:    []string{"app", "pull", "."},
			wantMsg: `required flag(s) "app" not set`,
		},
		{
			// The command's OWN check, reachable once the flag is present but
			// blank (cobra counts it as set).
			name:    "app pull with a blank --app",
			args:    []string{"app", "pull", ".", "--app", "  "},
			wantMsg: "an app is required — pass --app <slug|appBlockId> (find the slug with `civitai app status`)",
		},
		{
			name:    "app withdraw with both id spellings",
			args:    []string{"app", "withdraw", "pubreq_1", "--id", "pubreq_2"},
			wantMsg: "pass the publish-request id as an argument OR via --id, not both",
		},
		{
			name:    "app withdraw without an id",
			args:    []string{"app", "withdraw"},
			wantMsg: "a publish-request id is required — pass it as an argument or with --id (find it via `civitai app status`)",
		},
		{
			name:    "app listing status on a manifest with no blockId",
			args:    []string{"app", "listing", "status"},
			wantMsg: "block.manifest.json has no blockId — pass --slug",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Run from a scratch directory so no block.manifest.json in the
			// repo's tree can satisfy a "run from your app directory" fallback.
			t.Chdir(t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok")
			if tc.name == "app listing status on a manifest with no blockId" {
				// This one needs a manifest that PARSES but declares no blockId,
				// which is the condition under test.
				if err := os.WriteFile(filepath.Join(".", manifest.Filename), []byte(`{"name":"x"}`), 0o600); err != nil {
					t.Fatalf("write manifest: %v", err)
				}
			}
			_, _, err := run(t, tc.args...)
			if err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("a missing/bad argument must classify as usage (exit 2), got %T: %v", err, err)
			}
			if err.Error() != tc.wantMsg {
				t.Errorf("classification must not change the message:\n got: %q\nwant: %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// TestFlagGroupViolationIsUsageTagged covers the OTHER validator
// enforceUsageExitCodes now runs. No command in the tree declares a flag group
// today, so without a synthetic command this line would be an unreachable guard
// — present, untested, and free to rot until the first MarkFlagsMutuallyExclusive
// lands and silently exits 1. The command is built and wired the same way
// NewRootCmd builds the real tree.
func TestFlagGroupViolationIsUsageTagged(t *testing.T) {
	newTree := func() *cobra.Command {
		root := &cobra.Command{Use: "root", SilenceUsage: true, SilenceErrors: true}
		child := &cobra.Command{
			Use:  "child",
			Args: cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error { return nil },
		}
		child.Flags().Bool("a", false, "")
		child.Flags().Bool("b", false, "")
		child.MarkFlagsMutuallyExclusive("a", "b")
		root.AddCommand(child)
		enforceUsageExitCodes(root)
		return root
	}

	// Positive control FIRST: a legal invocation must succeed, so a red below
	// cannot mean "this wiring rejects everything".
	ok := newTree()
	ok.SetArgs([]string{"child", "--a"})
	ok.SetOut(io.Discard)
	ok.SetErr(io.Discard)
	if err := ok.Execute(); err != nil {
		t.Fatalf("a legal invocation must succeed, got: %v", err)
	}

	bad := newTree()
	bad.SetArgs([]string{"child", "--a", "--b"})
	bad.SetOut(io.Discard)
	bad.SetErr(io.Discard)
	err := bad.Execute()
	if err == nil {
		t.Fatal("expected an error for two mutually exclusive flags")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("a flag-group violation must classify as usage (exit 2), got %T: %v", err, err)
	}
	// cobra's own wording, unchanged.
	if !strings.Contains(err.Error(), "were all set") {
		t.Errorf("cobra's message must be preserved, got %q", err.Error())
	}
}

// TestListingImageValidationIsUsageTagged pins `app listing set-icon/set-cover/
// add-screenshot`'s local image refusals to exit 2. loadAndValidateImage is a
// line-for-line twin of resolveLocalImage (generate --image), where every one of
// these conditions is already asUsageError-tagged — so the same bad path exited
// 2 through `generate` and 1 through `app listing`.
func TestListingImageValidationIsUsageTagged(t *testing.T) {
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty.png")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	notAnImage := filepath.Join(dir, "notanimage.png")
	if err := os.WriteFile(notAnImage, []byte("this is not an image at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(dir, "huge.png")
	if err := os.WriteFile(huge, make([]byte, kindByteCap(kindIcon)+1), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, file string }{
		{"no such file", filepath.Join(dir, "nope.png")},
		{"a directory", dir},
		{"an empty file", empty},
		{"over the per-kind size cap", huge},
		{"not a png/jpeg/webp", notAnImage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := loadAndValidateImage(kindIcon, tc.file)
			if err == nil {
				t.Fatalf("expected an error for %s", tc.file)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("a bad <file> argument must classify as usage (exit 2), got %T: %v", err, err)
			}
		})
	}

	// Positive control: a VALID image must not be rejected at all, so a green
	// sweep above cannot mean "this function fails everything".
	good := filepath.Join(dir, "good.png")
	if err := os.WriteFile(good, pngBytes(t, 64, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadAndValidateImage(kindIcon, good); err != nil {
		t.Fatalf("a valid png must be accepted, got: %v", err)
	}
}
