package cmd

import (
	"strings"
	"testing"
)

// The img2img disclosure must reach EVERY surface that precedes a spend
// decision. Audit finding on PR #215: it printed only on the interactive TTY
// path, so the two surfaces that most need it never showed it — `--yes` (which
// is MANDATORY non-interactively, i.e. every CI run) and `--dry-run` (the
// documented price-check-first workflow).
//
// Four mutants survived the whole suite before this file existed: deleting the
// Image loop, deleting the Ecosystem line, deleting the caveat, and emptying the
// label list.
const disclosureCaveat = "server IGNORES the images above"

func assertDisclosure(t *testing.T, surface, stderr, wantEco, wantImg string) {
	t.Helper()
	if !strings.Contains(stderr, "Ecosystem:") || !strings.Contains(stderr, wantEco) {
		t.Errorf("%s: ecosystem %q must be disclosed before the spend decision.\nstderr:\n%s", surface, wantEco, stderr)
	}
	if !strings.Contains(stderr, "Image:") || !strings.Contains(stderr, wantImg) {
		t.Errorf("%s: the image %q must be disclosed before the spend decision.\nstderr:\n%s", surface, wantImg, stderr)
	}
	if !strings.Contains(stderr, disclosureCaveat) {
		t.Errorf("%s: the silently-ignored-images caveat is the one thing the estimate cannot show; it must appear.\nstderr:\n%s", surface, stderr)
	}
}

// --yes skips the PROMPT, never the disclosure.
func TestGenerateImage_DisclosureOnYes(t *testing.T) {
	withStdinTTY(t, false)
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 70, 80))

	s := &genSeams{uploadReplyURL: "https://orchestration.civitai.com/v2/consumer/blobs/Y.png"}
	o := imgOpts("Qwen", local)
	o.assumeYes = true
	_, errBuf, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit reached %d times, want 1 — the run must actually have passed the spend gate", s.submitCalls)
	}
	assertDisclosure(t, "--yes", errBuf.String(), "Qwen", local)
}

// --dry-run is the documented price-check-first workflow: catching a
// silently-dropped image here is free.
func TestGenerateImage_DisclosureOnDryRun(t *testing.T) {
	withStdinTTY(t, false)
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 70, 80))

	s := &genSeams{uploadReplyURL: "https://orchestration.civitai.com/v2/consumer/blobs/D.png"}
	o := imgOpts("Flux1Kontext", local)
	o.dryRun = true
	out, errBuf, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.whatIfCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: whatIf reached %d times, want 1", s.whatIfCalls)
	}
	if s.submitCalls != 0 {
		t.Fatalf("--dry-run must never submit; submit reached %d times", s.submitCalls)
	}
	assertDisclosure(t, "--dry-run", errBuf.String(), "Flux1Kontext", local)
	// Stream discipline: the disclosure is stderr, so a --json stdout stays clean.
	if strings.Contains(out.String(), disclosureCaveat) {
		t.Errorf("the caveat must go to stderr, not stdout.\nstdout:\n%s", out.String())
	}
}

// The interactive path keeps it too (this is the surface that already worked —
// it is here so a refactor cannot trade one surface for another).
func TestGenerateImage_DisclosureOnTTYPrompt(t *testing.T) {
	withStdinTTY(t, true)
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 70, 80))

	s := &genSeams{uploadReplyURL: "https://orchestration.civitai.com/v2/consumer/blobs/T.png"}
	o := imgOpts("Qwen", local)
	cmd, _, errb := genCmd("y\n")
	if err := runGenerate(cmd, s.deps(t), o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	assertDisclosure(t, "TTY prompt", errb.String(), "Qwen", local)
}

// CONTRAST CONTROL: with no --image, the caveat must NOT appear. Without this a
// renderer that printed the caveat unconditionally would pass every test above,
// and the warning would become noise users learn to ignore.
func TestGenerate_NoImagesMeansNoImageCaveat(t *testing.T) {
	withStdinTTY(t, false)
	s := &genSeams{}
	o := baseOpts()
	o.assumeYes = true
	_, errBuf, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit reached %d times, want 1", s.submitCalls)
	}
	if strings.Contains(errBuf.String(), disclosureCaveat) {
		t.Errorf("a txt2img run must not print the image caveat.\nstderr:\n%s", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "Image:") {
		t.Errorf("a txt2img run must not print an Image line.\nstderr:\n%s", errBuf.String())
	}
}
