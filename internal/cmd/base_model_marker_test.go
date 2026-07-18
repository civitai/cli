package cmd

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestModelsSearchBaseModelRepeatedEncoding proves --base-model is repeatable and
// encodes as repeated `baseModels=` query keys (the array form the API's
// string|string[] union parses as `baseModel IN (...)`).
func TestModelsSearchBaseModelRepeatedEncoding(t *testing.T) {
	var got []string
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()["baseModels"]
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	if _, _, err := run(t, "models", "search", "--base-model", "Pony", "--base-model", "Illustrious"); err != nil {
		t.Fatalf("models search --base-model: %v", err)
	}
	if len(got) != 2 || got[0] != "Pony" || got[1] != "Illustrious" {
		t.Errorf("baseModels query = %v, want [Pony Illustrious] as repeated keys", got)
	}
}

func TestModelsSearchBaseModelSingle(t *testing.T) {
	var got []string
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()["baseModels"]
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	// A base model with a space (e.g. video checkpoints) must round-trip intact.
	if _, _, err := run(t, "models", "search", "--base-model", "Wan Video 2.2 T2V-A14B"); err != nil {
		t.Fatalf("models search --base-model: %v", err)
	}
	if len(got) != 1 || got[0] != "Wan Video 2.2 T2V-A14B" {
		t.Errorf("baseModels = %v, want the single spaced value", got)
	}
}

// TestModelVersionGetMarksNonWeightsType asserts the informational marker renders
// the ACTUAL file type in the human `model-versions get` output when the primary
// file is not weights — here an Archive (e.g. a "Workflows" model), tagged
// [Archive] rather than any "no downloadable weights" claim.
func TestModelVersionGetMarksNonWeightsType(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"modelId":2,"name":"clean-name","baseModel":"SD 1.5",
		  "files":[{"id":9,"name":"workflow.zip","type":"Archive","primary":true,"sizeKB":18000}]}`))
	})
	out, _, err := run(t, "model-versions", "get", "1")
	if err != nil {
		t.Fatalf("model-versions get: %v", err)
	}
	if !strings.Contains(out, "[Archive]") {
		t.Errorf("non-weights version should carry the accurate [Archive] marker: %s", out)
	}
	if strings.Contains(out, "no downloadable weights") {
		t.Errorf("marker must NOT falsely claim there are no downloadable weights: %s", out)
	}
}

// TestModelVersionGetMarksTrainingDataType asserts a Training Data primary file
// is tagged with its verbatim type.
func TestModelVersionGetMarksTrainingDataType(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"modelId":2,"name":"clean-name","baseModel":"SD 1.5",
		  "files":[{"id":9,"name":"training.zip","type":"Training Data","primary":true,"sizeKB":18000}]}`))
	})
	out, _, err := run(t, "model-versions", "get", "1")
	if err != nil {
		t.Fatalf("model-versions get: %v", err)
	}
	if !strings.Contains(out, "[Training Data]") {
		t.Errorf("training-data version should carry the [Training Data] marker: %s", out)
	}
}

// TestModelVersionGetNoMarkerForWeights asserts a normal weights version is NOT
// tagged.
func TestModelVersionGetNoMarkerForWeights(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"modelId":2,"name":"v","baseModel":"SD 1.5",
		  "files":[{"id":9,"name":"model.safetensors","type":"Model","primary":true,"sizeKB":2000000}]}`))
	})
	out, _, err := run(t, "model-versions", "get", "1")
	if err != nil {
		t.Fatalf("model-versions get: %v", err)
	}
	if strings.Contains(out, "[") {
		t.Errorf("a normal weights version must NOT be tagged with a type marker: %s", out)
	}
}

// TestModelGetMarksNonWeightsVersion asserts the marker also shows the accurate
// type in the `models get` version list.
func TestModelGetMarksNonWeightsVersion(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":2,"name":"M","type":"Checkpoint",
		  "modelVersions":[{"id":10,"name":"weights","baseModel":"SD 1.5","files":[{"id":1,"name":"m.safetensors","type":"Model","primary":true}]},
		                   {"id":11,"name":"onsite","baseModel":"SD 1.5","files":[{"id":2,"name":"t.zip","type":"Training Data","primary":true}]}]}`))
	})
	out, _, err := run(t, "models", "get", "2")
	if err != nil {
		t.Fatalf("models get: %v", err)
	}
	if !strings.Contains(out, "[Training Data]") {
		t.Errorf("models get should tag the non-weights version with its type: %s", out)
	}
}

// TestModelVersionGetJSONUnchanged asserts --json is a raw passthrough (no marker
// injected).
func TestModelVersionGetJSONUnchanged(t *testing.T) {
	const raw = `{"id":1,"modelId":2,"name":"clean","baseModel":"SD 1.5","files":[{"id":9,"name":"t.zip","type":"Training Data","primary":true}]}`
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(raw))
	})
	out, _, err := run(t, "model-versions", "get", "1", "--json")
	if err != nil {
		t.Fatalf("model-versions get --json: %v", err)
	}
	if strings.Contains(out, "[Training Data]") {
		t.Errorf("--json output must NOT carry the human marker: %s", out)
	}
	if !strings.Contains(out, `"Training Data"`) {
		t.Errorf("--json should pass the raw body through: %s", out)
	}
}

// TestDecideNoticeForcesRefreshWhenAheadOfCache proves the stale-cache guard: a
// cache pinning a "latest" OLDER than the running version (the draft-window
// artifact) forces a refresh even while the cache is otherwise fresh, and never
// shows a notice.
func TestDecideNoticeForcesRefreshWhenAheadOfCache(t *testing.T) {
	now := time.Now()
	c := updateCache{
		LastCheck:     now.Add(-1 * time.Hour), // fresh (< 24h)
		LatestVersion: "v0.1.61",               // stale: older than what we run
	}
	d := decideNotice(c, "v0.1.62", now)
	if d.show {
		t.Errorf("must not show a notice when ahead of the cached latest: %+v", d)
	}
	if !d.refresh {
		t.Error("a cache pinning an older 'latest' than the running version must force a refresh")
	}
}

// TestDecideNoticeNoNoticeWhenCurrentEqualsLatest guards the >= case.
func TestDecideNoticeNoNoticeWhenCurrentEqualsLatest(t *testing.T) {
	now := time.Now()
	c := updateCache{LastCheck: now, LatestVersion: "v0.1.62"}
	if d := decideNotice(c, "v0.1.62", now); d.show {
		t.Errorf("equal current/latest must not notify: %+v", d)
	}
}

// TestDecideNoticeShowsWhenBehind keeps the positive path covered.
func TestDecideNoticeShowsWhenBehind(t *testing.T) {
	now := time.Now()
	c := updateCache{LastCheck: now, LatestVersion: "v0.1.62"}
	d := decideNotice(c, "v0.1.60", now)
	if !d.show {
		t.Errorf("a genuinely behind version should notify: %+v", d)
	}
	if !strings.Contains(d.notice, "v0.1.62") {
		t.Errorf("notice should name the latest: %q", d.notice)
	}
}
