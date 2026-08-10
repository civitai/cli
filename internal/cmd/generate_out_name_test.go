package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// --out-name is a USER-SUPPLIED value on the path that decides where downloaded
// bytes land — the same path the SERVER-supplied workflow id already had to be
// bounded on. These tests exist to keep it bounded.
//
// 🔴 Every refusal below is paired with a POSITIVE CONTROL that a normal
// template writes where it should, through the same wiring. A refusal test on
// its own cannot tell "the guard works" from "--out-name never writes anything".

// --- rendering ---------------------------------------------------------------

// The default (empty) template must still be the historical scheme. If this
// drifts, every existing caller's filenames change silently.
func TestRenderOutName_EmptyTemplateIsTheHistoricalScheme(t *testing.T) {
	got, err := renderOutName("", "wf_123", 2, "https://x/o/a.jpeg")
	if err != nil {
		t.Fatalf("default template: %v", err)
	}
	if got != "wf_123-2.jpeg" {
		t.Errorf("default render = %q, want %q", got, "wf_123-2.jpeg")
	}
}

func TestRenderOutName_ExpandsTheThreePlaceholders(t *testing.T) {
	cases := []struct {
		tmpl, url, want string
	}{
		{"img-{n}{ext}", "https://x/o/a.jpeg", "img-7.jpeg"},
		{"{workflow}_{n}{ext}", "https://x/o/a.png", "wf_123_7.png"},
		{"literal{ext}", "https://x/o/a.JPEG", "literal.jpeg"},
		{"{n}-{n}{ext}", "https://x/o/a.jpeg", "7-7.jpeg"},
		{"no-placeholders-at-all", "https://x/o/a.jpeg", "no-placeholders-at-all"},
		// Braces that are not a placeholder are literal text.
		{"a}b{c-{n}{ext}", "https://x/o/a.jpeg", "a}b{c-7.jpeg"},
	}
	for _, tc := range cases {
		got, err := renderOutName(tc.tmpl, "wf_123", 7, tc.url)
		if err != nil {
			t.Errorf("renderOutName(%q): %v", tc.tmpl, err)
			continue
		}
		if got != tc.want {
			t.Errorf("renderOutName(%q) = %q, want %q", tc.tmpl, got, tc.want)
		}
	}
}

// 🔴 {ext} must render blobExtension's BOUNDED value, never raw URL text. The
// URL is server-supplied and its path is attacker-shaped in the worst case, so
// anything that is not a dot plus 1-8 alphanumerics has to collapse to .bin.
func TestRenderOutName_ExtIsTheBoundedExtension(t *testing.T) {
	cases := map[string]string{
		"https://x/o/a.jpeg?sig=deadbeef": "img.jpeg",
		"https://x/o/a.verylongextension": "img.bin",
		"https://x/o/a":                   "img.bin",
		"https://x/o/a.":                  "img.bin",
		"https://x/o/a.j-peg":             "img.bin",
		// 🔴 A percent-encoded traversal DECODES into u.Path, so the URL really
		// does reach blobExtension as ".../a.jpeg/../../.bashrc". The bound is
		// what contains it: path.Ext takes only the span after the last
		// separator, and safeExtension then requires a dot plus 1-8
		// alphanumerics — so what survives is the harmless literal ".bashrc",
		// inside --out-dir, and NOT the traversal that carried it.
		"https://x/o/a.jpeg%2F..%2F..%2F.bashrc": "img.bashrc",
		"://nonsense":                            "img.bin",
	}
	for url, want := range cases {
		got, err := renderOutName("img{ext}", "wf_123", 1, url)
		if err != nil {
			t.Errorf("renderOutName with url %q: %v", url, err)
			continue
		}
		if got != want {
			t.Errorf("{ext} for %q rendered %q, want %q", url, got, want)
		}
		if strings.ContainsRune(got, filepath.Separator) {
			t.Errorf("{ext} for %q leaked a separator into %q", url, got)
		}
	}
}

// An unknown placeholder must be named, not pasted through as literal text: a
// file called "wf_123{workflowid}.jpeg" is a mystery, an error is not.
func TestRenderOutName_UnknownPlaceholderIsRefused(t *testing.T) {
	for _, tmpl := range []string{"{workflowid}-{n}{ext}", "{N}{ext}", "{}", "{ workflow }{ext}", "{extension}"} {
		_, err := renderOutName(tmpl, "wf_123", 1, "https://x/o/a.jpeg")
		if err == nil {
			t.Errorf("renderOutName(%q) must be refused", tmpl)
			continue
		}
		if !strings.Contains(err.Error(), "unknown placeholder") {
			t.Errorf("renderOutName(%q) refusal must name the problem: %v", tmpl, err)
		}
		// Actionable: it has to say what IS supported.
		for _, want := range []string{"{workflow}", "{n}", "{ext}"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("renderOutName(%q) refusal does not list %s: %v", tmpl, want, err)
			}
		}
	}
}

// A whitespace-only template must be refused rather than silently becoming
// either a file named "   " or the default scheme the user was replacing.
func TestRenderOutName_EmptyRenderIsRefused(t *testing.T) {
	for _, tmpl := range []string{" ", "   ", "\t"} {
		_, err := renderOutName(tmpl, "wf_123", 1, "https://x/o/a.jpeg")
		if err == nil {
			t.Fatalf("renderOutName(%q) must be refused", tmpl)
		}
		if !strings.Contains(err.Error(), "empty file name") {
			t.Errorf("renderOutName(%q) = %v, want the empty-render refusal", tmpl, err)
		}
	}
}

// --- containment -------------------------------------------------------------

// outputTarget is the containment guard. It must REFUSE, not sanitise.
func TestOutputTarget_RefusesAnythingThatLeavesTheDirectory(t *testing.T) {
	dir := "/tmp/civitai-out"
	for _, name := range []string{
		"../../.bashrc",
		"a/b",
		"..",
		".",
		"",
		"/etc/passwd",
		"sub/../a.jpeg",
		"./a.jpeg",
	} {
		got, err := outputTarget(dir, name)
		if err == nil {
			t.Errorf("outputTarget(%q, %q) = %q, want a refusal", dir, name, got)
			continue
		}
		if !strings.Contains(err.Error(), "plain file name") {
			t.Errorf("outputTarget(%q, %q) refusal is not the containment one: %v", dir, name, err)
		}
	}

	// The "/" out-dir case: filepath.Join CLEANS, so a name of "." resolves to
	// "/" whose parent is also "/". Without the literal check the structural
	// comparison alone would accept it.
	if got, err := outputTarget("/", "."); err == nil {
		t.Errorf(`outputTarget("/", ".") = %q, want a refusal`, got)
	}

	// POSITIVE CONTROL: ordinary names resolve, so the refusals above are not
	// "outputTarget rejects everything".
	for _, name := range []string{"a.jpeg", "wf_123-1.jpeg", "cat 1.png", "-leading-dash.bin", ".hidden.jpeg"} {
		got, err := outputTarget(dir, name)
		if err != nil {
			t.Errorf("POSITIVE CONTROL FAILED: outputTarget(%q, %q): %v", dir, name, err)
			continue
		}
		if want := filepath.Join(dir, name); got != want {
			t.Errorf("outputTarget(%q, %q) = %q, want %q", dir, name, got, want)
		}
	}

	// Relative out-dirs must behave the same way.
	if _, err := outputTarget(".", "../escape.jpeg"); err == nil {
		t.Error(`outputTarget(".", "../escape.jpeg") must be refused`)
	}
	if got, err := outputTarget(".", "a.jpeg"); err != nil || got != "a.jpeg" {
		t.Errorf(`outputTarget(".", "a.jpeg") = %q, %v; want "a.jpeg", nil`, got, err)
	}
}

// --- the download loop -------------------------------------------------------

// POSITIVE CONTROL for every refusal below: a normal template writes real bytes
// under the names it says, inside --out-dir.
func TestDownloadOutputs_OutNameWritesTheNamedFiles(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()
	kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 3))

	paths, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""),
		io.Discard, io.Discard, "wf_123", kept, dir, "cat-{n}{ext}", false)
	if err != nil {
		t.Fatalf("downloadOutputs with --out-name: %v", err)
	}
	want := []string{"cat-1.jpeg", "cat-2.jpeg", "cat-3.jpeg"}
	if len(paths) != len(want) {
		t.Fatalf("wrote %d files, want %d", len(paths), len(want))
	}
	for i, p := range paths {
		if filepath.Dir(p) != dir {
			t.Errorf("file %d landed in %s, want %s", i, filepath.Dir(p), dir)
		}
		if filepath.Base(p) != want[i] {
			t.Errorf("file %d = %s, want %s", i, filepath.Base(p), want[i])
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s not on disk: %v", p, err)
			continue
		}
		if !strings.HasPrefix(string(b), "image-bytes") {
			t.Errorf("%s holds %q, want the served bytes", p, b)
		}
	}
	if len(seen) != 3 {
		t.Errorf("the blob host saw %d requests, want 3", len(seen))
	}
}

// 🔴 THE TRAVERSAL REFUSAL. A rendered name that would leave --out-dir must be
// refused with nothing written and nothing fetched — not sanitised into some
// other name, which would make the traversal invisible rather than impossible.
func TestDownloadOutputs_OutNameTraversalIsRefusedBeforeAnyTransfer(t *testing.T) {
	for _, tmpl := range []string{
		"../../.bashrc",
		"a/b",
		"..",
		".",
		"../{n}{ext}",
		"{workflow}/../../{n}{ext}",
		"/etc/cron.d/{n}",
		"sub/{workflow}-{n}{ext}",
	} {
		t.Run(tmpl, func(t *testing.T) {
			var seen []string
			srv := blobServer(t, &seen)
			// A nested out-dir, so an escape has somewhere real to land and the
			// sentinel below can prove it did not.
			root := t.TempDir()
			dir := filepath.Join(root, "out")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(root, ".bashrc")
			if err := os.WriteFile(sentinel, []byte("do not touch"), 0o600); err != nil {
				t.Fatal(err)
			}
			kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 2))

			_, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""),
				io.Discard, io.Discard, "wf_123", kept, dir, tmpl, false)
			if err == nil {
				t.Fatalf("--out-name %q must be refused", tmpl)
			}
			if !strings.Contains(err.Error(), "plain file name") {
				t.Errorf("the refusal must be the containment one, got: %v", err)
			}
			if !strings.Contains(err.Error(), "--out-name") {
				t.Errorf("the refusal must name --out-name so it is actionable: %v", err)
			}
			if len(seen) != 0 {
				t.Errorf("%d blob request(s) were made before the refusal — it must run before any byte moves", len(seen))
			}
			if b, _ := os.ReadFile(sentinel); string(b) != "do not touch" {
				t.Errorf("the file OUTSIDE --out-dir was written: %q", b)
			}
			if entries, _ := filepath.Glob(filepath.Join(root, "**")); len(entries) > 2 {
				t.Errorf("unexpected files created: %v", entries)
			}
			left, _ := filepath.Glob(filepath.Join(dir, "*"))
			if len(left) != 0 {
				t.Errorf("files were written into --out-dir despite the refusal: %v", left)
			}
		})
	}
}

// A template that renders the same name for two outputs would have the run
// overwrite its own results, with the presigned URLs already spent. It must be
// caught in the same before-any-bytes window as an on-disk collision — and
// --force must NOT wave it through, because there is no earlier version to
// replace, only the run's own other output.
func TestDownloadOutputs_DuplicateTargetsAreRefusedBeforeAnyTransfer(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "withoutForce"
		if force {
			name = "withForce"
		}
		t.Run(name, func(t *testing.T) {
			var seen []string
			srv := blobServer(t, &seen)
			dir := t.TempDir()
			kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 3))

			_, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""),
				io.Discard, io.Discard, "wf_123", kept, dir, "cat{ext}", force)
			if err == nil {
				t.Fatal("a template that names every output the same must be refused")
			}
			if !strings.Contains(err.Error(), "{n}") {
				t.Errorf("the refusal must name the fix ({n}): %v", err)
			}
			if len(seen) != 0 {
				t.Errorf("%d blob request(s) were made before the refusal — the presigned URLs must not be spent", len(seen))
			}
			left, _ := filepath.Glob(filepath.Join(dir, "*"))
			if len(left) != 0 {
				t.Errorf("files were written despite the refusal: %v", left)
			}
		})
	}
}

// The single-output case is the reason the duplicate check is not a static
// "template must contain {n}" rule: one output plus a fixed name is exactly the
// ergonomics this flag exists for.
func TestDownloadOutputs_OutNameWithoutIndexIsFineForOneOutput(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()
	kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 1))

	paths, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""),
		io.Discard, io.Discard, "wf_123", kept, dir, "cat{ext}", false)
	if err != nil {
		t.Fatalf("a single output with a fixed name must be allowed: %v", err)
	}
	if len(paths) != 1 || filepath.Base(paths[0]) != "cat.jpeg" {
		t.Fatalf("paths = %v, want [<dir>/cat.jpeg]", paths)
	}
}

// The existing-file collision check must still fire against TEMPLATED names —
// it is keyed on the resolved target, so a name the template produced is not a
// blind spot.
func TestDownloadOutputs_OutNameCollidesWithAnExistingFile(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()
	kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 2))

	existing := filepath.Join(dir, "cat-2.jpeg")
	if err := os.WriteFile(existing, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""),
		io.Discard, io.Discard, "wf_123", kept, dir, "cat-{n}{ext}", false)
	if err == nil {
		t.Fatal("a templated name colliding with an existing file must be refused")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal must name --force: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("the collision check ran after %d transfer(s)", len(seen))
	}
	if b, _ := os.ReadFile(existing); string(b) != "keep me" {
		t.Errorf("the existing file was modified: %q", b)
	}

	// POSITIVE CONTROL: --force takes the same invocation through.
	if _, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""),
		io.Discard, io.Discard, "wf_123", kept, dir, "cat-{n}{ext}", true); err != nil {
		t.Fatalf("--force with --out-name: %v", err)
	}
	if b, _ := os.ReadFile(existing); string(b) == "keep me" {
		t.Error("--force did not overwrite the templated target")
	}
}

// --- the command layer: refused BEFORE anything is spent ---------------------

// 🔴 A broken --out-name is a LOCAL mistake, so it must be a usage error (exit
// 2) raised before the estimator and the submit are reached. Discovering it
// after the charge leaves the user with presigned URLs and no files.
//
// Item 7: the exit code is asserted with errors.Is, never on message text.
func TestGenerate_BadOutNameIsAUsageErrorAndSpendsNothing(t *testing.T) {
	withStdinTTY(t, false)
	cases := map[string]string{
		"traversal":           "../../.bashrc",
		"separator":           "a/b",
		"dotdot":              "..",
		"unknown placeholder": "{workflowid}-{n}{ext}",
		"whitespace only":     "   ",
		"absolute":            "/etc/passwd",
	}
	for name, tmpl := range cases {
		t.Run(name, func(t *testing.T) {
			var s genSeams
			// --no-wait, so that a REGRESSION here fails readably instead of
			// panicking. Measured: with the pre-spend check removed and a
			// full-wait fixture, the run sailed past validation into
			// pollWorkflow and nil-dereferenced an unwired seam — a kill, but a
			// panic that aborts the package rather than the assertion below
			// naming what actually broke. The spend gate is whatIf/submit, and
			// --no-wait reaches both.
			o := baseOpts()
			o.assumeYes = true
			o.outDir = t.TempDir()
			o.outName = tmpl

			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), o)
			if err == nil {
				t.Fatalf("--out-name %q must be refused", tmpl)
			}
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("--out-name %q must classify as ErrUsage (exit 2), got %T: %v", tmpl, err, err)
			}
			if s.whatIfCalls != 0 || s.submitCalls != 0 {
				t.Errorf("the refused template still reached the network: whatIf=%d submit=%d — it must be refused before anything is priced or spent",
					s.whatIfCalls, s.submitCalls)
			}
		})
	}
}

// POSITIVE CONTROL for the case above, end to end: a good template goes all the
// way through a real (faked-transport) run and the files land, named, in
// --out-dir. Without this the refusals prove only that --out-name never works.
func TestGenerate_OutNameHappyPathWritesNamedFiles(t *testing.T) {
	withStdinTTY(t, false)
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	wf := succeededWorkflow(srv.URL, 3)
	raw, _ := json.Marshal(wf)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, string(raw))
	s.downloadBlob = presignedFetcher(t, srv.URL, "tok")

	o := waitOpts(dir)
	o.outName = "kitten-{n}{ext}"

	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("generate with --out-name: %v", err)
	}

	entries, _ := filepath.Glob(filepath.Join(dir, "*"))
	sort.Strings(entries)
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, filepath.Base(e))
	}
	want := []string{"kitten-1.jpeg", "kitten-2.jpeg", "kitten-3.jpeg"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files written = %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "kitten-1.jpeg") {
		t.Errorf("stdout should name the saved files: %q", out.String())
	}
	if s.whatIfCalls != 1 || s.submitCalls != 1 {
		t.Errorf("CONTROL: the happy path must actually price and submit; whatIf=%d submit=%d", s.whatIfCalls, s.submitCalls)
	}
}

// The duplicate-name refusal AT THE COMMAND LAYER. It is the one --out-name
// failure that cannot be caught before the spend (how many outputs come back is
// the server's answer), so its error has to be the one that hands the user their
// results back rather than a bare failure.
func TestGenerate_OutNameDuplicateNamesFailAfterTheRunWithAReattachRoute(t *testing.T) {
	withStdinTTY(t, false)
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	wf := succeededWorkflow(srv.URL, 2)
	raw, _ := json.Marshal(wf)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, string(raw))
	s.downloadBlob = presignedFetcher(t, srv.URL, "tok")

	o := waitOpts(dir)
	o.outName = "kitten{ext}"

	c, _, errb := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if err == nil {
		t.Fatal("a template naming both outputs the same must not report success")
	}
	if !strings.Contains(err.Error(), "{n}") {
		t.Errorf("the refusal must name the fix: %v", err)
	}
	if !strings.Contains(err.Error(), "civitai workflows get") {
		t.Errorf("the refusal must name the re-attach route — the run was charged: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("%d blob(s) were fetched before the refusal, want 0", len(seen))
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(left) != 0 {
		t.Errorf("files were written despite the refusal: %v", left)
	}
	if !strings.Contains(errb.String(), "civitai workflows get") {
		t.Errorf("stderr should still carry the expiry/re-attach note:\n%s", errb.String())
	}
}
