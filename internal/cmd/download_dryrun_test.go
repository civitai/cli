package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirIsEmpty reports whether dir contains no entries.
func dirIsEmpty(t *testing.T, dir string) bool {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	return len(ents) == 0
}

func TestDownloadDryRunPrintsPlanWritesNothing(t *testing.T) {
	const body = "PRIMARY-WEIGHTS"
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: body, withSHA: true},
	})
	setupDownloadEnv(t, d, "")

	out, _, err := run(t, "download", "128713", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run should exit 0, got %v", err)
	}
	for _, want := range []string{"model.safetensors", "size:", "sha256:", sha256hex(body), "target:", "auth:", "would download"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output missing %q:\n%s", want, out)
		}
	}
	// Nothing on disk — no final file, no .part.
	if _, e := os.Stat("model.safetensors"); e == nil {
		t.Error("dry-run must not write the file")
	}
	if _, e := os.Stat("model.safetensors.part"); e == nil {
		t.Error("dry-run must not create a .part file")
	}
	if d.dlHits != 0 {
		t.Errorf("dry-run must not fetch the file (dlHits=%d)", d.dlHits)
	}
}

func TestDownloadDryRunAllPlansEveryFileNoRefusal(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
		{id: 2, name: "training.zip", typ: "Training Data", body: "trainingdata"},
	})
	setupDownloadEnv(t, d, "")
	dir := t.TempDir()

	out, _, err := run(t, "download", "128713", "--all", "--out-dir", dir, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --all should exit 0, got %v", err)
	}
	if !strings.Contains(out, "model.safetensors") || !strings.Contains(out, "training.zip") {
		t.Errorf("plan should list every selected file:\n%s", out)
	}
	// The non-weights file is no longer refused — every file plans as downloadable.
	if strings.Contains(out, "WOULD BE REFUSED") {
		t.Errorf("no file should be refused in the plan anymore:\n%s", out)
	}
	if strings.Count(out, "would download") != 2 {
		t.Errorf("both files should plan as would-download:\n%s", out)
	}
	if !dirIsEmpty(t, dir) {
		t.Error("dry-run --all must write nothing to the out-dir")
	}
	if d.dlHits != 0 {
		t.Errorf("dry-run must not fetch anything (dlHits=%d)", d.dlHits)
	}
}

func TestDownloadDryRunModel(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
	})
	setupDownloadEnv(t, d, "")

	out, _, err := run(t, "download", "--model", "4384", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --model: %v", err)
	}
	if !strings.Contains(out, "m.safetensors") || !strings.Contains(out, "would download") {
		t.Errorf("--model dry-run should plan the resolved version's file:\n%s", out)
	}
	if _, e := os.Stat("m.safetensors"); e == nil {
		t.Error("dry-run --model must not write the file")
	}
	if d.dlHits != 0 {
		t.Errorf("dry-run --model must not fetch (dlHits=%d)", d.dlHits)
	}
}

func TestDownloadDryRunArchivePlansDownloadNoRefusal(t *testing.T) {
	// Repro (a), dry-run form: a single non-weights primary — a "Workflows" model's
	// Archive — plans as a normal download with no refusal line, and writes nothing.
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "workflow.zip", typ: "Archive", primary: true, body: "workflowbytes"},
	})
	setupDownloadEnv(t, d, "")

	out, _, err := run(t, "download", "3083777", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run of an Archive file should not error, got %v", err)
	}
	if strings.Contains(out, "WOULD BE REFUSED") {
		t.Errorf("an Archive file must NOT be refused in the plan anymore:\n%s", out)
	}
	if !strings.Contains(out, "would download") {
		t.Errorf("the Archive file should plan as would-download:\n%s", out)
	}
	if _, e := os.Stat("workflow.zip"); e == nil {
		t.Error("dry-run must not write the file")
	}
}

func TestDownloadDryRunWithOutPath(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	target := filepath.Join(t.TempDir(), "custom.safetensors")

	out, _, err := run(t, "download", "128713", "--out", target, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --out: %v", err)
	}
	if !strings.Contains(out, target) {
		t.Errorf("plan target should reflect --out %q:\n%s", target, out)
	}
	if _, e := os.Stat(target); e == nil {
		t.Error("dry-run --out must not create the target")
	}
}
