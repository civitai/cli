package cmd

import (
	"runtime/debug"
	"testing"
)

// stubBuildInfo swaps readBuildInfo for the duration of a test.
func stubBuildInfo(t *testing.T, info *debug.BuildInfo, ok bool) {
	t.Helper()
	orig := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) { return info, ok }
	t.Cleanup(func() { readBuildInfo = orig })
}

// resetBuildVars restores the package build vars after a test.
func resetBuildVars(t *testing.T) {
	t.Helper()
	origV, origC, origD := version, commit, date
	t.Cleanup(func() { version, commit, date = origV, origC, origD })
}

func TestSetBuildInfoFallsBackToBuildInfo(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "dev", "none", "unknown"

	stubBuildInfo(t, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.1"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "deadbeefcafe"},
			{Key: "vcs.time", Value: "2026-06-23T00:00:00Z"},
		},
	}, true)

	// Empty ldflag values => still dev defaults => fallback applies.
	SetBuildInfo("", "", "")

	if version != "v0.1.1" {
		t.Errorf("version: got %q want v0.1.1", version)
	}
	if commit != "deadbeefcafe" {
		t.Errorf("commit: got %q want deadbeefcafe", commit)
	}
	if date != "2026-06-23T00:00:00Z" {
		t.Errorf("date: got %q want 2026-06-23T00:00:00Z", date)
	}
}

func TestSetBuildInfoLdflagsAreAuthoritative(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "dev", "none", "unknown"

	stubBuildInfo(t, &debug.BuildInfo{
		Main: debug.Module{Version: "v9.9.9"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "frombuildinfo"},
			{Key: "vcs.time", Value: "2000-01-01T00:00:00Z"},
		},
	}, true)

	// Real release ldflag values must win over build info.
	SetBuildInfo("1.2.3", "abc123", "2026-01-01")

	if version != "1.2.3" {
		t.Errorf("version: got %q want 1.2.3 (ldflag authoritative)", version)
	}
	if commit != "abc123" {
		t.Errorf("commit: got %q want abc123 (ldflag authoritative)", commit)
	}
	if date != "2026-01-01" {
		t.Errorf("date: got %q want 2026-01-01 (ldflag authoritative)", date)
	}
}

func TestApplyBuildInfoFallbackPartial(t *testing.T) {
	resetBuildVars(t)
	// version stamped by ldflag, commit/date still defaults.
	version, commit, date = "1.0.0", "none", "unknown"

	stubBuildInfo(t, &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.1"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "rev123"},
			{Key: "vcs.time", Value: "2026-06-23T00:00:00Z"},
		},
	}, true)

	applyBuildInfoFallback()

	if version != "1.0.0" {
		t.Errorf("version: got %q want 1.0.0 (already stamped, not overridden)", version)
	}
	if commit != "rev123" {
		t.Errorf("commit: got %q want rev123 (filled from build info)", commit)
	}
	if date != "2026-06-23T00:00:00Z" {
		t.Errorf("date: got %q want build-info time", date)
	}
}

func TestApplyBuildInfoFallbackDevelVersionIgnored(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "dev", "none", "unknown"

	// `go build` (no module path version) yields "(devel)" — not useful, keep "dev".
	stubBuildInfo(t, &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: nil,
	}, true)

	applyBuildInfoFallback()

	if version != "dev" {
		t.Errorf("version: got %q want dev ((devel) should be ignored)", version)
	}
}

func TestApplyBuildInfoFallbackNoBuildInfo(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "dev", "none", "unknown"

	stubBuildInfo(t, nil, false)
	applyBuildInfoFallback()

	if version != "dev" || commit != "none" || date != "unknown" {
		t.Errorf("expected dev defaults preserved when build info unavailable: %s %s %s", version, commit, date)
	}
}
