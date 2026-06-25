package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// withParseableVersion pins the package `version` var to a known parseable
// value for the duration of the test (the notice only fires when current
// parses). Restores the original afterward.
func withParseableVersion(t *testing.T, v string) {
	t.Helper()
	orig := version
	version = v
	t.Cleanup(func() { version = orig })
}

// forceTTY makes stderrIsTerminal report the given value for the test.
func forceTTY(t *testing.T, isTTY bool) {
	t.Helper()
	orig := stderrIsTerminal
	stderrIsTerminal = func() bool { return isTTY }
	t.Cleanup(func() { stderrIsTerminal = orig })
}

// captureRefresh replaces requestRefresh with a recorder and returns a pointer
// to the call count. No process is forked.
func captureRefresh(t *testing.T) *int {
	t.Helper()
	orig := requestRefresh
	var n int
	requestRefresh = func() error { n++; return nil }
	t.Cleanup(func() { requestRefresh = orig })
	return &n
}

// useTempConfigDir points config.Dir() at a temp dir via XDG_CONFIG_HOME and
// clears CI/opt-out env so the gating doesn't accidentally suppress.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")
	for _, k := range ciEnvVars {
		t.Setenv(k, "")
	}
	return dir
}

func TestCacheIsFresh(t *testing.T) {
	now := time.Now()
	if cacheIsFresh(updateCache{}, now) {
		t.Error("empty cache (zero LastCheck) must not be fresh")
	}
	if !cacheIsFresh(updateCache{LastCheck: now.Add(-1 * time.Hour)}, now) {
		t.Error("1h-old check should be fresh")
	}
	if cacheIsFresh(updateCache{LastCheck: now.Add(-25 * time.Hour)}, now) {
		t.Error("25h-old check should be stale")
	}
}

func TestDecideNotice_FreshCacheNoRefresh(t *testing.T) {
	now := time.Now()
	c := updateCache{
		LastCheck:     now.Add(-1 * time.Hour), // fresh
		LatestVersion: "v0.1.12",
	}
	d := decideNotice(c, "v0.1.11", now)
	if d.refresh {
		t.Error("fresh cache must NOT request a refresh")
	}
	if !d.show {
		t.Error("behind + fresh + never-notified should show the notice")
	}
	if !strings.Contains(d.notice, "v0.1.12") || !strings.Contains(d.notice, "v0.1.11") {
		t.Errorf("notice should mention both versions: %q", d.notice)
	}
	if !strings.Contains(d.notice, "civitai upgrade") {
		t.Errorf("notice should mention 'civitai upgrade': %q", d.notice)
	}
}

func TestDecideNotice_StaleCacheRequestsRefresh(t *testing.T) {
	now := time.Now()
	c := updateCache{
		LastCheck:     now.Add(-48 * time.Hour), // stale
		LatestVersion: "v0.1.12",
	}
	d := decideNotice(c, "v0.1.11", now)
	if !d.refresh {
		t.Error("stale cache MUST request a refresh")
	}
	// Stale but with a usable cached value => still shows the (stale) notice.
	if !d.show {
		t.Error("stale cache with a usable cached value should still show the notice")
	}
}

func TestDecideNotice_EmptyCacheFirstRun(t *testing.T) {
	now := time.Now()
	d := decideNotice(updateCache{}, "v0.1.11", now)
	if !d.refresh {
		t.Error("empty cache should kick off the first refresh")
	}
	if d.show {
		t.Error("first run with no cached latest must NOT show a notice")
	}
}

func TestDecideNotice_UpToDateNoNotice(t *testing.T) {
	now := time.Now()
	c := updateCache{LastCheck: now, LatestVersion: "v0.1.11"}
	if d := decideNotice(c, "v0.1.11", now); d.show {
		t.Error("equal versions must not show a notice")
	}
	// Current ahead of latest (dev build): no notice.
	c2 := updateCache{LastCheck: now, LatestVersion: "v0.1.11"}
	if d := decideNotice(c2, "v0.2.0", now); d.show {
		t.Error("current ahead of latest must not show a notice")
	}
}

func TestDecideNotice_UnparseableCurrentNoNotice(t *testing.T) {
	now := time.Now()
	c := updateCache{LastCheck: now, LatestVersion: "v0.1.12"}
	if d := decideNotice(c, "dev", now); d.show {
		t.Error("unparseable current (dev) must not be nagged")
	}
}

func TestDecideNotice_UnparseableCachedLatestNoNotice(t *testing.T) {
	now := time.Now()
	c := updateCache{LastCheck: now, LatestVersion: "garbage"}
	if d := decideNotice(c, "v0.1.11", now); d.show {
		t.Error("unparseable cached latest must not show a notice")
	}
}

func TestDecideNotice_NotifyThrottle(t *testing.T) {
	now := time.Now()
	// Behind, but notified 1h ago => throttled (<=1/24h).
	c := updateCache{
		LastCheck:     now,
		LatestVersion: "v0.1.12",
		LastNotified:  now.Add(-1 * time.Hour),
	}
	if d := decideNotice(c, "v0.1.11", now); d.show {
		t.Error("notice within 24h of last_notified must be throttled")
	}
	// Notified 25h ago => allowed again.
	c.LastNotified = now.Add(-25 * time.Hour)
	if d := decideNotice(c, "v0.1.11", now); !d.show {
		t.Error("notice should be allowed once >24h since last_notified")
	}
}

func TestReadUpdateCache_CorruptIsEmptySilent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/update-check.json"
	if err := writeFileHelper(path, "{ not valid json"); err != nil {
		t.Fatal(err)
	}
	c := readUpdateCache(path) // must not panic/error
	if c.LatestVersion != "" || !c.LastCheck.IsZero() {
		t.Errorf("corrupt cache should read as empty, got %+v", c)
	}
}

func TestReadUpdateCache_MissingIsEmpty(t *testing.T) {
	c := readUpdateCache(t.TempDir() + "/does-not-exist.json")
	if c.LatestVersion != "" {
		t.Errorf("missing cache should read as empty, got %+v", c)
	}
}

func TestWriteReadUpdateCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/update-check.json"
	now := time.Now().Truncate(time.Second)
	in := updateCache{LastCheck: now, LatestVersion: "v0.1.12", LastNotified: now}
	if err := writeUpdateCache(path, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := readUpdateCache(path)
	if out.LatestVersion != in.LatestVersion || !out.LastCheck.Equal(in.LastCheck) {
		t.Errorf("round-trip mismatch: in=%+v out=%+v", in, out)
	}
}

// TestMaybeNotify_FreshCacheNoSpawnUsesCached: a fresh cache => no spawn, but
// the cached notice still prints to the provided (stderr) writer.
func TestMaybeNotify_FreshCacheNoSpawnUsesCached(t *testing.T) {
	useTempConfigDir(t)
	withParseableVersion(t, "v0.1.11")
	forceTTY(t, true)
	n := captureRefresh(t)

	path, _ := updateCachePath()
	now := time.Now()
	if err := writeUpdateCache(path, updateCache{LastCheck: now, LatestVersion: "v0.1.12"}); err != nil {
		t.Fatal(err)
	}

	var errb bytes.Buffer
	maybeNotifyUpdate(&errb, "whoami", false)

	if *n != 0 {
		t.Errorf("fresh cache should NOT spawn a refresh, got %d", *n)
	}
	if !strings.Contains(errb.String(), "v0.1.12") {
		t.Errorf("fresh cache should print the cached notice: %q", errb.String())
	}
}

// TestMaybeNotify_StaleCacheRequestsRefresh: a stale cache => a refresh is
// requested (asserted via the injected recorder; no fork).
func TestMaybeNotify_StaleCacheRequestsRefresh(t *testing.T) {
	useTempConfigDir(t)
	withParseableVersion(t, "v0.1.11")
	forceTTY(t, true)
	n := captureRefresh(t)

	path, _ := updateCachePath()
	if err := writeUpdateCache(path, updateCache{
		LastCheck:     time.Now().Add(-48 * time.Hour),
		LatestVersion: "v0.1.12",
	}); err != nil {
		t.Fatal(err)
	}

	var errb bytes.Buffer
	maybeNotifyUpdate(&errb, "whoami", false)

	if *n != 1 {
		t.Errorf("stale cache should request exactly 1 refresh, got %d", *n)
	}
}

// TestMaybeNotify_PersistsLastNotified proves the once-a-day throttle survives
// across invocations: the first call prints + records last_notified; the second
// (same day) stays silent.
func TestMaybeNotify_PersistsLastNotified(t *testing.T) {
	useTempConfigDir(t)
	withParseableVersion(t, "v0.1.11")
	forceTTY(t, true)
	captureRefresh(t)

	path, _ := updateCachePath()
	if err := writeUpdateCache(path, updateCache{LastCheck: time.Now(), LatestVersion: "v0.1.12"}); err != nil {
		t.Fatal(err)
	}

	var first, second bytes.Buffer
	maybeNotifyUpdate(&first, "whoami", false)
	maybeNotifyUpdate(&second, "whoami", false)

	if !strings.Contains(first.String(), "v0.1.12") {
		t.Errorf("first call should notify: %q", first.String())
	}
	if second.Len() != 0 {
		t.Errorf("second same-day call should be throttled silent, got: %q", second.String())
	}
}

// Gating matrix: each suppression condition => no notice AND no spawn.
func TestMaybeNotify_GatingSuppresses(t *testing.T) {
	type setup func(t *testing.T)
	cases := []struct {
		name    string
		cmdName string
		noFlag  bool
		prep    setup
	}{
		{name: "CI env set", cmdName: "whoami", prep: func(t *testing.T) { t.Setenv("CI", "true") }},
		{name: "non-TTY", cmdName: "whoami", prep: func(t *testing.T) { forceTTY(t, false) }},
		{name: "opt-out flag", cmdName: "whoami", noFlag: true, prep: func(t *testing.T) {}},
		{name: "opt-out env", cmdName: "whoami", prep: func(t *testing.T) { t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1") }},
		{name: "excluded version", cmdName: "version", prep: func(t *testing.T) {}},
		{name: "excluded upgrade", cmdName: "upgrade", prep: func(t *testing.T) {}},
		{name: "excluded completion", cmdName: "completion", prep: func(t *testing.T) {}},
		{name: "excluded help", cmdName: "help", prep: func(t *testing.T) {}},
		{name: "internal __complete", cmdName: "__complete", prep: func(t *testing.T) {}},
		{name: "internal __update-check", cmdName: "__update-check", prep: func(t *testing.T) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useTempConfigDir(t)
			withParseableVersion(t, "v0.1.11")
			forceTTY(t, true) // default TTY; some cases override to false
			n := captureRefresh(t)
			tc.prep(t)

			// Behind, fresh cache => would notify if not gated.
			path, _ := updateCachePath()
			if err := writeUpdateCache(path, updateCache{LastCheck: time.Now(), LatestVersion: "v0.1.12"}); err != nil {
				t.Fatal(err)
			}

			var errb bytes.Buffer
			maybeNotifyUpdate(&errb, tc.cmdName, tc.noFlag)

			if errb.Len() != 0 {
				t.Errorf("%s: expected no notice, got %q", tc.name, errb.String())
			}
			if *n != 0 {
				t.Errorf("%s: expected no refresh spawn, got %d", tc.name, *n)
			}
		})
	}
}

// TestNoticeGoesToStderrNotStdout drives the full root command with a stale,
// behind cache + forced TTY and asserts the notice lands on STDERR, never
// STDOUT (so pipes/scripts are unaffected). It uses a SUCCESSFUL whoami (cobra
// only runs PersistentPostRun when the command's RunE succeeds — the notice is
// deliberately never appended to a failed command).
func TestNoticeGoesToStderrNotStdout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"username":"bob","id":99}`))
	}))
	t.Cleanup(srv.Close)

	useTempConfigDir(t)
	withParseableVersion(t, "v0.1.11")
	forceTTY(t, true)
	captureRefresh(t)
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)

	path, _ := updateCachePath()
	if err := writeUpdateCache(path, updateCache{LastCheck: time.Now(), LatestVersion: "v0.1.12"}); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := run(t, "whoami")
	if err != nil {
		t.Fatalf("whoami should succeed: %v", err)
	}

	if strings.Contains(out, "v0.1.12") {
		t.Errorf("update notice must NOT pollute stdout: %q", out)
	}
	if !strings.Contains(errOut, "v0.1.12") {
		t.Errorf("update notice should be on stderr: %q", errOut)
	}
}

func TestRefreshUpdateCache_WritesLatest(t *testing.T) {
	useTempConfigDir(t)
	srv, hits := newReleaseServer(t, "v0.1.12", 200)
	pointAtServer(t, srv.URL)

	refreshUpdateCache()

	if *hits != 1 {
		t.Errorf("expected 1 GitHub hit, got %d", *hits)
	}
	path, _ := updateCachePath()
	c := readUpdateCache(path)
	if c.LatestVersion != "v0.1.12" {
		t.Errorf("refresh should cache the latest version, got %q", c.LatestVersion)
	}
	if c.LastCheck.IsZero() {
		t.Error("refresh should stamp last_check")
	}
}

func TestRefreshUpdateCache_FailureStillStampsLastCheck(t *testing.T) {
	useTempConfigDir(t)
	srv, _ := newReleaseServer(t, "", 500)
	pointAtServer(t, srv.URL)

	refreshUpdateCache()

	path, _ := updateCachePath()
	c := readUpdateCache(path)
	if c.LastCheck.IsZero() {
		t.Error("a failed refresh should still stamp last_check to back off")
	}
	if c.LatestVersion != "" {
		t.Errorf("a failed refresh should not set a latest version, got %q", c.LatestVersion)
	}
}

func writeFileHelper(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
