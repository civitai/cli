package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression coverage for civitai/cli#963 and #964: the dry-run plan must
// predict what the real run does. #963 — it said "would download" (exit 0) for
// an --out whose parent directory the real run cannot create. #964 — it said
// "would download" for a target the real run skips as already present.

const parityBody = "parity-weights"

func parityServer(t *testing.T) *dlServer {
	t.Helper()
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: parityBody, withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	return d
}

// TestDownloadDryRunRefusesAnOutParentThatIsAFile is #963's first repro: a path
// component is a regular file. The dry run must fail with the SAME error the
// real run returns, print no "would download", and create nothing.
func TestDownloadDryRunRefusesAnOutParentThatIsAFile(t *testing.T) {
	for _, rel := range []string{"f/x.safetensors", "f/a/b/x.safetensors"} {
		t.Run(rel, func(t *testing.T) {
			d := parityServer(t)
			tmp := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmp, "f"), []byte("a file"), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(tmp, filepath.FromSlash(rel))

			plan, _, dryErr := run(t, "download", "128713", "--out", out, "--dry-run")
			if dryErr == nil {
				t.Fatalf("dry run exited 0 for an --out under a regular file:\n%s", plan)
			}
			if strings.Contains(plan, "status: would download") {
				t.Errorf("dry run still planned \"would download\":\n%s", plan)
			}
			if !strings.Contains(plan, "status: blocked") {
				t.Errorf("dry run should mark the file blocked:\n%s", plan)
			}

			_, _, realErr := run(t, "download", "128713", "--out", out)
			if realErr == nil {
				t.Fatal("control: the real run must fail here too")
			}
			if dryErr.Error() != realErr.Error() {
				t.Errorf("dry run error differs from the real run's:\n dry:  %v\n real: %v", dryErr, realErr)
			}
			wantSuffix := "mkdir " + filepath.Join(tmp, "f") + ": not a directory"
			if !strings.HasSuffix(dryErr.Error(), wantSuffix) {
				t.Errorf("dry run error = %q, want it to end %q", dryErr, wantSuffix)
			}
			if d.dlHits != 0 {
				t.Errorf("nothing may be transferred (dlHits=%d)", d.dlHits)
			}
		})
	}
}

// TestDownloadDryRunRefusesAnUnwritableParent is #963's second repro (a
// read-only location): the nearest existing directory is not writable. Skipped
// as root, who can write a 0555 directory — the real run would succeed too.
func TestDownloadDryRunRefusesAnUnwritableParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions; the real run would succeed")
	}
	d := parityServer(t)
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	out := filepath.Join(ro, "new", "x.safetensors")

	plan, _, dryErr := run(t, "download", "128713", "--out", out, "--dry-run")
	if dryErr == nil {
		t.Fatalf("dry run exited 0 for an --out under a read-only directory:\n%s", plan)
	}
	if strings.Contains(plan, "status: would download") {
		t.Errorf("dry run still planned \"would download\":\n%s", plan)
	}
	if !strings.Contains(plan, "status: blocked") {
		t.Errorf("dry run should mark the file blocked:\n%s", plan)
	}
	_, _, realErr := run(t, "download", "128713", "--out", out)
	if realErr == nil {
		t.Fatal("control: the real run must fail here too")
	}
	// Not byte-equality with the real run: here the dry run PREDICTS an errno
	// with access(2), and which one the kernel gives MkdirAll (EACCES, EROFS,
	// EPERM) varies by platform. What #963 asks for is the same failure, the
	// same error class and a message naming the directory.
	wantPrefix := "create output directory " + filepath.Dir(out) + ": mkdir " + filepath.Dir(out) + ": "
	for _, e := range []error{dryErr, realErr} {
		if !strings.HasPrefix(e.Error(), wantPrefix) {
			t.Errorf("error = %q, want it to start %q", e, wantPrefix)
		}
	}
	if d.dlHits != 0 {
		t.Errorf("nothing may be transferred (dlHits=%d)", d.dlHits)
	}
}

// TestDownloadDryRunStillPlansACreatableParent is #963's control: missing but
// creatable parents still plan "would download", exit 0, and are NOT created.
func TestDownloadDryRunStillPlansACreatableParent(t *testing.T) {
	parityServer(t)
	tmp := t.TempDir()
	out := filepath.Join(tmp, "new", "sub", "x.safetensors")

	plan, _, err := run(t, "download", "128713", "--out", out, "--dry-run")
	if err != nil {
		t.Fatalf("dry run with creatable parents: %v\n%s", err, plan)
	}
	if !strings.Contains(plan, "status: would download") {
		t.Errorf("want \"would download\":\n%s", plan)
	}
	if _, serr := os.Stat(filepath.Join(tmp, "new")); serr == nil {
		t.Error("the dry run created a directory")
	}
}

// TestDownloadDryRunPlansTheAlreadyPresentSkip is #964: the plan must say what
// the real run does with a target that already exists.
func TestDownloadDryRunPlansTheAlreadyPresentSkip(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		flags    []string
		want     string // the plan's status line
	}{
		{"matching file", parityBody, nil, "status: would skip — already present (SHA256 verified)"},
		{"mismatch with --no-verify", "junk", []string{"--no-verify"}, "status: would skip — already present (not hash-checked)"},
		// Controls: the real run downloads in both of these.
		{"matching file with --force", parityBody, []string{"--force"}, "status: would download"},
		{"mismatching file", "junk", nil, "status: would download"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := parityServer(t)
			target := filepath.Join(t.TempDir(), "e.safetensors")
			if err := os.WriteFile(target, []byte(tc.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"download", "128713", "--out", target, "--dry-run"}, tc.flags...)
			plan, _, err := run(t, args...)
			if err != nil {
				t.Fatalf("dry run: %v\n%s", err, plan)
			}
			if !strings.Contains(plan, tc.want) {
				t.Errorf("plan should contain %q:\n%s", tc.want, plan)
			}
			if got, _ := os.ReadFile(target); string(got) != tc.contents {
				t.Errorf("dry run changed the target: %q", got)
			}
			if d.dlHits != 0 {
				t.Errorf("dry run transferred (dlHits=%d)", d.dlHits)
			}

			// The real run must agree with the plan.
			realOut, _, rerr := run(t, append([]string{"download", "128713", "--out", target}, tc.flags...)...)
			if rerr != nil {
				t.Fatalf("real run: %v", rerr)
			}
			skipped := strings.Contains(realOut, "already present")
			if planSkip := strings.Contains(tc.want, "would skip"); planSkip != skipped {
				t.Errorf("plan said %q but the real run skipped=%v:\n%s", tc.want, skipped, realOut)
			}
		})
	}
}

// TestDownloadHelpNamesNoRedirectStatus is #962: the server answers 307, and
// the help said 302. The help now names no status code at all.
func TestDownloadHelpNamesNoRedirectStatus(t *testing.T) {
	long := newDownloadCmd().Long
	if strings.Contains(long, "302") {
		t.Error("download --help still names a 302 redirect; the server answers 307")
	}
	if !strings.Contains(long, "Transport: the Civitai download URL redirects to signed storage and the CLI\nfollows it.") {
		t.Error("download --help lost its Transport sentence")
	}
}
