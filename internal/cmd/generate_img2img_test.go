package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// imgOpts is a valid img2img invocation: a prompt, an ecosystem and whatever
// --image values the case needs. --no-wait keeps these cases about the graph and
// the spend gate rather than the poll/download half, which has its own tests.
func imgOpts(ecosystem string, images ...string) generateOpts {
	o := baseOpts()
	o.ecosystem = ecosystem
	o.images = images
	return o
}

// runImg drives the real runGenerate with injected seams and captured streams.
func runImg(t *testing.T, s *genSeams, o generateOpts) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	cmd, out, errb := genCmd("")
	return out, errb, runGenerate(cmd, s.deps(t), o)
}

// --- flag validation ---------------------------------------------------------

// 🔴 --image REQUIRES --ecosystem. Without it the server ignores the images and
// charges for a plain txt2img (measured against civitai.com), so this is a
// refusal and it must happen before ANY network call.
func TestGenerateImage_RequiresEcosystem(t *testing.T) {
	s := &genSeams{}
	o := baseOpts()
	o.images = []string{"./cat.png"}
	o.dryRun = true
	_, _, err := runImg(t, s, o)
	if err == nil {
		t.Fatal("--image without --ecosystem must be refused")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("errors.Is(err, ErrUsage) = false — exit code pinned by kind, not text")
	}
	for _, want := range []string{"--image requires --ecosystem", "charges you for it"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err, want)
		}
	}
	// 🔴 Nothing may have happened: no upload, no estimate, no submit.
	if s.uploadCalls != 0 || s.whatIfCalls != 0 || s.submitCalls != 0 {
		t.Errorf("upload=%d whatIf=%d submit=%d, want 0/0/0", s.uploadCalls, s.whatIfCalls, s.submitCalls)
	}
}

// 🔴 Over the global cap: refuse, and prove nothing was uploaded or submitted.
func TestGenerateImage_OverCapRefusesWithoutUploadingOrSubmitting(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i <= maxReferenceImages; i++ { // one MORE than the cap
		paths = append(paths, writeFixture(t, dir, string(rune('a'+i))+".png", pngBytes(t, 8, 8)))
	}

	s := &genSeams{}
	o := imgOpts("Qwen", paths...)
	o.assumeYes = true
	_, _, err := runImg(t, s, o)
	if err == nil {
		t.Fatal("more than the cap must be refused")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("errors.Is(err, ErrUsage) = false")
	}
	for _, want := range []string{"above the 7-image maximum", "nothing was uploaded and nothing was submitted", "DROPS the extras silently"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err, want)
		}
	}
	if s.uploadCalls != 0 {
		t.Errorf("🔴 an over-cap invocation uploaded %d image(s)", s.uploadCalls)
	}
	if s.submitCalls != 0 {
		t.Errorf("🔴 an over-cap invocation submitted %d time(s)", s.submitCalls)
	}
	if s.whatIfCalls != 0 {
		t.Errorf("an over-cap invocation priced %d time(s); it should stop at validation", s.whatIfCalls)
	}
}

// 🔴 POSITIVE CONTROL for the two zeros above: EXACTLY the cap is accepted, and
// drives both counters. It also pins the comparison as `>` not `>=` — an
// off-by-one would fail right here.
func TestGenerateImage_PositiveControl_UploadCounterMoves(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < maxReferenceImages; i++ { // EXACTLY the cap
		paths = append(paths, writeFixture(t, dir, string(rune('a'+i))+".png", pngBytes(t, 8, 8)))
	}

	s := &genSeams{}
	o := imgOpts("Qwen", paths...)
	o.assumeYes = true
	if _, _, err := runImg(t, s, o); err != nil {
		t.Fatalf("positive control FAILED: exactly %d images was refused: %v", maxReferenceImages, err)
	}
	if s.uploadCalls != maxReferenceImages {
		t.Fatalf("positive control FAILED: uploadCalls = %d, want %d — the zeros asserted above would be meaningless",
			s.uploadCalls, maxReferenceImages)
	}
	if s.submitCalls != 1 {
		t.Fatalf("positive control FAILED: submitCalls = %d, want 1", s.submitCalls)
	}
}

// --image is a CONTENT flag, so it is mutually exclusive with --input, exactly
// like --checkpoint and --lora.
func TestGenerateImage_MutuallyExclusiveWithInput(t *testing.T) {
	cases := []struct {
		name   string
		images []string
		eco    string
		want   string
	}{
		{"--image", []string{"./a.png"}, "", "--image"},
		{"--ecosystem", nil, "Qwen", "--ecosystem"},
		{"both", []string{"./a.png"}, "Qwen", "--image, --ecosystem"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &genSeams{}
			o := baseOpts()
			o.prompt = "" // --input replaces the prompt argument
			o.inputPath = "g.json"
			o.images = tc.images
			o.ecosystem = tc.eco
			o.dryRun = true
			_, _, err := runImg(t, s, o)
			if err == nil {
				t.Fatal("--input combined with a content flag must be refused")
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("errors.Is(err, ErrUsage) = false")
			}
			if !strings.Contains(err.Error(), "--input cannot be combined with "+tc.want) {
				t.Errorf("message %q should name %q", err, tc.want)
			}
			if s.uploadCalls != 0 || s.whatIfCalls != 0 || s.submitCalls != 0 {
				t.Errorf("upload=%d whatIf=%d submit=%d, want 0/0/0", s.uploadCalls, s.whatIfCalls, s.submitCalls)
			}
		})
	}
}

// --- the graph that goes over the wire ---------------------------------------

// 🔴 The payload keeps workflow "txt2img". The server promotes it; sending an
// img2img value is a different (and worse) request. Asserted on the DECODED
// object, never a substring search.
func TestGenerateImage_GraphShape(t *testing.T) {
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 640, 360))

	s := &genSeams{uploadReplyURL: "https://orchestration.civitai.com/v2/consumer/blobs/UP.png"}
	o := imgOpts("Qwen", local)
	o.dryRun = true
	if _, _, err := runImg(t, s, o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}

	raw, err := json.Marshal(s.lastGraph)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if m["workflow"] != "txt2img" {
		t.Errorf("workflow = %v, want \"txt2img\" — the CLI must NEVER send an img2img workflow value; the server promotes it", m["workflow"])
	}
	if m["ecosystem"] != "Qwen" {
		t.Errorf("ecosystem = %v, want Qwen — without it the server does not promote the job", m["ecosystem"])
	}

	imgs, ok := m["images"].([]any)
	if !ok || len(imgs) != 1 {
		t.Fatalf("images = %#v, want one entry", m["images"])
	}
	entry, ok := imgs[0].(map[string]any)
	if !ok {
		t.Fatalf("images[0] = %#v, want an object", imgs[0])
	}
	// 🔴 Exactly {url,width,height} — verified against the server's imagesNode
	// output schema, where width/height are REQUIRED numbers.
	if entry["url"] != "https://orchestration.civitai.com/v2/consumer/blobs/UP.png" {
		t.Errorf("images[0].url = %v", entry["url"])
	}
	if entry["width"] != float64(640) || entry["height"] != float64(360) {
		t.Errorf("images[0] dimensions = %v x %v, want 640 x 360", entry["width"], entry["height"])
	}
	if len(entry) != 3 {
		t.Errorf("images[0] has %d keys (%v), want exactly url/width/height", len(entry), entry)
	}

	// 🔴 KEY ABSENCE on the decoded object (item 14): unset fields stay absent
	// even now that the graph carries images.
	for _, k := range []string{"steps", "cfgScale", "sampler", "seed", "quantity", "model", "resources", "negativePrompt", "aspectRatio"} {
		if _, present := m[k]; present {
			t.Errorf("key %q is present in the marshalled graph but was never set", k)
		}
	}
}

// With no --image at all, the images key must be ABSENT — not an empty array,
// which the server would read as "an image is required" for an edit workflow.
func TestGenerateImage_AbsentWhenNoImageFlag(t *testing.T) {
	s := &genSeams{}
	o := baseOpts()
	o.dryRun = true
	if _, _, err := runImg(t, s, o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	raw, _ := json.Marshal(s.lastGraph)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := m["images"]; present {
		t.Errorf("images = %v, want the key ABSENT when no --image was passed", m["images"])
	}
	if _, present := m["ecosystem"]; present {
		t.Errorf("ecosystem = %v, want the key ABSENT when no --ecosystem was passed", m["ecosystem"])
	}
	if s.uploadCalls != 0 {
		t.Errorf("uploadCalls = %d, want 0", s.uploadCalls)
	}
}

// A remote URL is referenced verbatim; the CLI must not re-host it.
func TestGenerateImage_RemoteURLIsPassedThrough(t *testing.T) {
	calls, auth := 0, false
	fetch, url := remoteImageFetcher(t, 200, jpegBytes(t, 512, 512), &calls, &auth)

	s := &genSeams{downloadBlob: fetch}
	o := imgOpts("Seedream", url)
	o.dryRun = true
	if _, _, err := runImg(t, s, o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if len(s.lastGraph.Images) != 1 {
		t.Fatalf("got %d images, want 1", len(s.lastGraph.Images))
	}
	if s.lastGraph.Images[0].URL != url {
		t.Errorf("url = %q, want the caller's URL unchanged", s.lastGraph.Images[0].URL)
	}
	if s.uploadCalls != 0 {
		t.Errorf("a URL was uploaded %d time(s); it must be passed through", s.uploadCalls)
	}
}

// --- --dry-run ---------------------------------------------------------------

// 🔴 --dry-run DOES upload, and still never submits.
//
// The upload is required for the estimate to mean anything: the server prices
// the job it can actually see, and a graph whose images[] is missing prices a
// plain txt2img. Uploading spends no Buzz. The submit seam must stay untouched.
func TestGenerateImage_DryRunUploadsButNeverSubmits(t *testing.T) {
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 100, 200))

	s := &genSeams{}
	o := imgOpts("Qwen", local)
	o.dryRun = true
	if _, _, err := runImg(t, s, o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.uploadCalls != 1 {
		t.Errorf("uploadCalls = %d, want 1 — without the upload the estimate prices a job with no images", s.uploadCalls)
	}
	if s.whatIfCalls != 1 {
		t.Errorf("whatIfCalls = %d, want 1", s.whatIfCalls)
	}
	if s.submitCalls != 0 {
		t.Errorf("🔴 --dry-run submitted %d time(s)", s.submitCalls)
	}
	if len(s.lastGraph.Images) != 1 || s.lastGraph.Images[0].Width != 100 {
		t.Errorf("the priced graph did not carry the reference image: %+v", s.lastGraph.Images)
	}
}

// --print-input must emit a graph that --input would accept — which means the
// images it prints reference real uploaded blobs, not local paths.
func TestGenerateImage_PrintInputEmitsUploadedBlobURLs(t *testing.T) {
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 70, 80))

	s := &genSeams{uploadReplyURL: "https://orchestration.civitai.com/v2/consumer/blobs/PI.png"}
	o := imgOpts("Qwen", local)
	o.printInput = true
	out, _, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("--print-input did not emit valid JSON: %v (%s)", err, out.String())
	}
	imgs, ok := m["images"].([]any)
	if !ok || len(imgs) != 1 {
		t.Fatalf("images = %#v", m["images"])
	}
	entry := imgs[0].(map[string]any)
	if entry["url"] != "https://orchestration.civitai.com/v2/consumer/blobs/PI.png" {
		t.Errorf("--print-input printed %v; it must print the UPLOADED blob URL, or the round-trip through --input is broken", entry["url"])
	}
	if strings.Contains(out.String(), local) {
		t.Error("--print-input leaked the local file path into the graph")
	}
	// It still stops before the estimator and the submit.
	if s.whatIfCalls != 0 || s.submitCalls != 0 {
		t.Errorf("--print-input reached whatIf=%d submit=%d, want 0/0", s.whatIfCalls, s.submitCalls)
	}
}

// --- the count note ----------------------------------------------------------

func TestGenerateImage_MultipleImagesWarnAboutSilentTruncation(t *testing.T) {
	dir := t.TempDir()
	a := writeFixture(t, dir, "a.png", pngBytes(t, 10, 10))
	b := writeFixture(t, dir, "b.png", pngBytes(t, 20, 20))

	s := &genSeams{}
	o := imgOpts("Qwen", a, b)
	o.dryRun = true
	_, errBuf, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if !strings.Contains(errBuf.String(), "DROPS the extras silently") {
		t.Errorf("stderr should warn about silent truncation, got:\n%s", errBuf.String())
	}
	// The warning must not block: both images still went through.
	if len(s.lastGraph.Images) != 2 {
		t.Errorf("got %d images, want 2 — the note warns, it never truncates", len(s.lastGraph.Images))
	}
	if s.uploadCalls != 2 {
		t.Errorf("uploadCalls = %d, want 2", s.uploadCalls)
	}
}

// A single image gets no note — one image can never be truncated.
func TestGenerateImage_SingleImageDoesNotWarn(t *testing.T) {
	dir := t.TempDir()
	a := writeFixture(t, dir, "a.png", pngBytes(t, 10, 10))

	s := &genSeams{}
	o := imgOpts("Qwen", a)
	o.dryRun = true
	_, errBuf, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if strings.Contains(errBuf.String(), "DROPS the extras silently") {
		t.Errorf("a single image must not warn about truncation, got:\n%s", errBuf.String())
	}
}

// --- the workflow label on the spend surfaces --------------------------------
//
// 🔴 THESE ARE ABOUT WHAT IS PRINTED, NOT WHAT IS SENT. The wire value stays
// `txt2img` with --image (AGENTS.md item 19(a)) and TestGenerateImage_GraphShape
// is what pins that. What was wrong is that the two screens preceding an
// irreversible spend printed the bare word `txt2img` beside a warning saying the
// server may IGNORE the images — the most alarming line on the screen, with
// nothing explaining it.

// TestGenerateImage_DryRunWorkflowLineExplainsThePromotion — the --dry-run
// quote's Workflow row.
func TestGenerateImage_DryRunWorkflowLineExplainsThePromotion(t *testing.T) {
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 100, 200))

	s := &genSeams{}
	o := imgOpts("Flux1Kontext", local)
	o.dryRun = true
	out, _, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	line := lineContaining(out.String(), "Workflow:")
	if line == "" {
		t.Fatalf("CONTROL failure: the quote printed no Workflow row, so there is nothing to annotate:\n%s", out.String())
	}
	if !strings.Contains(line, generateWorkflow) {
		t.Errorf("the row must still show the WIRE value %q — item 19(a) depends on it staying visible: %q",
			generateWorkflow, line)
	}
	if !strings.Contains(line, imagePromotionNote) {
		t.Errorf("`Workflow: %s` with --image set is unexplained on the screen a user approves a charge from.\n got: %q\nwant it to carry: %q",
			generateWorkflow, line, imagePromotionNote)
	}
}

// TestGenerate_WorkflowLineIsUnannotatedWithoutImages is the other direction,
// and it is the one that keeps the annotation honest: a plain text-to-image job
// really is txt2img, and telling that user about an image-editing promotion
// would be noise at best and a false claim at worst.
func TestGenerate_WorkflowLineIsUnannotatedWithoutImages(t *testing.T) {
	var s genSeams
	o := baseOpts()
	o.dryRun = true
	cmd, out, _ := genCmd("")
	if err := runGenerate(cmd, s.deps(t), o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	line := lineContaining(out.String(), "Workflow:")
	if line == "" {
		t.Fatalf("CONTROL failure: no Workflow row:\n%s", out.String())
	}
	if strings.Contains(line, "image editing") || strings.Contains(line, imagePromotionNote) {
		t.Errorf("a job with no --image must not be told about an image-editing promotion: %q", line)
	}
}

// TestGenerateImage_ConfirmationWorkflowLineExplainsThePromotion — the
// interactive confirmation is the OTHER screen naming the workflow, and it is
// the one immediately in front of the irreversible spend. It declines the
// prompt, so nothing is submitted.
func TestGenerateImage_ConfirmationWorkflowLineExplainsThePromotion(t *testing.T) {
	withStdinTTY(t, true)
	dir := t.TempDir()
	local := writeFixture(t, dir, "cat.png", pngBytes(t, 100, 200))

	s := &genSeams{}
	o := imgOpts("Flux1Kontext", local)
	cmd, _, errb := genCmd("n\n")
	if err := runGenerate(cmd, s.deps(t), o); err == nil {
		t.Fatal("declining the prompt must not report success")
	}
	if s.submitCalls != 0 {
		t.Fatalf("🔴 the declined run submitted %d time(s)", s.submitCalls)
	}
	line := lineContaining(errb.String(), "About to generate with")
	if line == "" {
		t.Fatalf("CONTROL failure: the confirmation printed no workflow line:\n%s", errb.String())
	}
	if !strings.Contains(line, generateWorkflow) {
		t.Errorf("the confirmation must still name the WIRE value %q: %q", generateWorkflow, line)
	}
	if !strings.Contains(line, imagePromotionNote) {
		t.Errorf("the confirmation names the workflow with no explanation of why it is %q with --image set: %q",
			generateWorkflow, line)
	}
}

// TestWorkflowLabelCallSiteLedger is item 28's second guard shape applied to
// this constant: an asserted, BIDIRECTIONAL ledger of the surfaces that name
// the workflow. It fails when the set GROWS (a new spend surface prints the raw
// constant and skips the explanation) and when it SHRINKS (a surface stops
// using the helper and hand-rolls the label again).
func TestWorkflowLabelCallSiteLedger(t *testing.T) {
	src, err := os.ReadFile("generate.go")
	if err != nil {
		t.Fatalf("read generate.go: %v", err)
	}
	body := string(src)

	// Every surface that renders the workflow name goes through workflowLabel.
	if got, want := strings.Count(body, "workflowLabel(built)"), 2; got != want {
		t.Errorf("workflowLabel is called at %d site(s), want %d (the --dry-run quote and the interactive confirmation). "+
			"A new site is a new screen that must explain the promotion; a lost one is a screen that stopped.", got, want)
	}
	// And no surface interpolates the bare constant into user-facing text any
	// more. `generateWorkflow` still appears — in the graph builder, in help
	// text and inside workflowLabel itself — so the assertion is on the
	// FORMATTING verbs the two surfaces used.
	for _, banned := range []string{
		`"Workflow:\t%s\n", generateWorkflow`,
		`"About to generate with %s at %s.\n", generateWorkflow`,
	} {
		if strings.Contains(body, banned) {
			t.Errorf("a spend surface still prints the bare wire value: %s", banned)
		}
	}
}

// lineContaining returns the first line of s holding sub, or "".
func lineContaining(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}
