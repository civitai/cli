package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/civitai/cli/internal/config"
)

// updateCacheName is the cache file (sibling of config.yaml) that backs the
// non-blocking daily update notice. It records when we last checked GitHub, the
// latest version we saw, and when we last actually showed the notice.
const updateCacheName = "update-check.json"

// updateCacheTTL is how often the background refresh is allowed to hit GitHub.
const updateCacheTTL = 24 * time.Hour

// updateNotifyTTL throttles how often the notice is shown — at most once a day
// even while the user stays behind, so it's a gentle nudge, not nagware.
const updateNotifyTTL = 24 * time.Hour

// updateCache is the on-disk JSON state for the cached update notice.
type updateCache struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version"`
	LastNotified  time.Time `json:"last_notified"`
}

// updateCachePath returns ~/.config/civitai/update-check.json (the same dir as
// config.yaml). Returns an error only if the config dir can't be resolved.
func updateCachePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, updateCacheName), nil
}

// readUpdateCache loads the cache. A missing or corrupt file is treated as an
// empty cache with no error — the notice path must never fail a command.
func readUpdateCache(path string) updateCache {
	b, err := os.ReadFile(path)
	if err != nil {
		return updateCache{}
	}
	var c updateCache
	if err := json.Unmarshal(b, &c); err != nil {
		return updateCache{} // corrupt => empty, fail-silent.
	}
	return c
}

// writeUpdateCache persists the cache atomically (0600 temp + rename) into the
// config dir. The dir is created if needed. Errors are returned to the caller,
// but the only caller (the detached refresh) ignores them — a failed write just
// means the next run refreshes again.
func writeUpdateCache(path string, c updateCache) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".update-check-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// cacheIsFresh reports whether the cached check is younger than the TTL, so no
// background refresh is needed. A zero LastCheck (empty cache) is never fresh.
func cacheIsFresh(c updateCache, now time.Time) bool {
	if c.LastCheck.IsZero() {
		return false
	}
	return now.Sub(c.LastCheck) < updateCacheTTL
}

// noticeDecision describes what the post-run hook should do, derived purely from
// the cache + current version + the clock (no I/O, so it's unit-testable).
type noticeDecision struct {
	// show is true when a one-line "newer version" notice should be printed.
	show bool
	// notice is the text to print (only meaningful when show is true).
	notice string
	// refresh is true when a background GitHub refresh should be kicked off.
	refresh bool
}

// decideNotice computes whether to show the notice and/or refresh the cache,
// from the cached state and the resolved current version. Pure function.
//
// Rules:
//   - refresh when the cache is stale (older than updateCacheTTL or empty).
//   - show the notice only when: the cached latest parses, current parses,
//     latest > current, AND we haven't notified within updateNotifyTTL.
func decideNotice(c updateCache, current string, now time.Time) noticeDecision {
	d := noticeDecision{refresh: !cacheIsFresh(c, now)}

	if c.LatestVersion == "" || !isParseableVersion(c.LatestVersion) {
		return d // no usable cached value yet (e.g. first run) => no notice.
	}
	// Only nag when we KNOW the current version and it's genuinely behind.
	if !isParseableVersion(current) {
		return d
	}
	if compareVersions(current, c.LatestVersion) != -1 {
		return d // up to date or ahead.
	}
	// Throttle: at most one notice per updateNotifyTTL.
	if !c.LastNotified.IsZero() && now.Sub(c.LastNotified) < updateNotifyTTL {
		return d
	}
	d.show = true
	d.notice = formatUpdateNotice(c.LatestVersion, current)
	return d
}

// formatUpdateNotice renders the single dim stderr line.
func formatUpdateNotice(latest, current string) string {
	return "A new version of civitai is available: " + latest +
		" (you have " + current + "). Run 'civitai upgrade' to update."
}
