package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#936 — the --dry-run plan must print each planned file's numeric
// id: the help promises "the ids are shown by --dry-run" so a user can
// --file <id> one of two same-named files. The ids are pairwise distinct and
// distinct from every other number in the plan (sizes, the version id), so a
// renderer that printed the wrong file's id, or any constant, cannot pass.
func TestDownloadDryRunPlanShowsFileIDs(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 54299, name: "first.safetensors", typ: "Model", primary: true, body: "aa", withSHA: true},
		{id: 55054, name: "second.safetensors", typ: "Model", body: "bbb", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	dir := t.TempDir()

	out, _, err := run(t, "download", "128713", "--all", "--out-dir", dir, "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --all should exit 0, got %v", err)
	}
	for _, want := range []string{
		"\nfirst.safetensors\n  id:     54299\n",
		"\nsecond.safetensors\n  id:     55054\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan should show the file id directly under its name; missing %q in:\n%s", want, out)
		}
	}
	if !dirIsEmpty(t, dir) || d.dlHits != 0 {
		t.Errorf("dry-run must write and fetch nothing (dlHits=%d)", d.dlHits)
	}
}

// civitai/cli#938 — the plan's auth line used to print "required" for every
// Civitai-hosted URL. That came from DownloadNeedsAuth, a HOST-TRUST check
// (will the CLI attach a token to this URL?), not the file's real requirement,
// and the version metadata the CLI fetches carries no per-file auth signal.
// So the plan must say what it actually knows: whether a token is sent, and
// that the server decides. It must also never present a Civitai download as
// "not required" (a gated file would then look anonymous).
func TestDownloadDryRunAuthLineSaysWhatIsKnown(t *testing.T) {
	cases := []struct {
		name  string
		flags []string
		want  string
	}{
		{"default (token sent if logged in)", nil,
			"  auth:   decided by the server — your login token is sent if you have one; a gated file 401s without it\n"},
		{"--anon (no token sent)", []string{"--anon"},
			"  auth:   decided by the server — --anon sends no token, so a gated file will 401\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newDLServer(t, false, []dlFile{
				{id: 7, name: "model.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
			})
			setupDownloadEnv(t, d, "")
			args := append([]string{"download", "128713", "--dry-run"}, tc.flags...)
			out, _, err := run(t, args...)
			if err != nil {
				t.Fatalf("dry-run should exit 0, got %v", err)
			}
			if strings.Contains(out, "auth:   required") || strings.Contains(out, "auth:   not required") {
				t.Errorf("plan must not claim a requirement the CLI cannot know:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("plan auth line: want %q in:\n%s", tc.want, out)
			}
		})
	}
}

// The off-domain branch (a URL the CLI never sends a token to) is reached via
// printDownloadPlan directly, since the loopback fixture always serves its
// download URLs from the trusted base host.
func TestDownloadPlanAuthLineOffDomainHost(t *testing.T) {
	var out strings.Builder
	f := civitai.ModelVersionFile{ID: 3, Name: "x.safetensors", DownloadURL: "https://storage.example.net/x"}
	if err := printDownloadPlan(&out, []civitai.ModelVersionFile{f}, &downloadOpts{}, "https://civitai.com"); err != nil {
		t.Fatalf("printDownloadPlan: %v", err)
	}
	const want = "  auth:   no token is sent to this host (not Civitai)\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("want %q in:\n%s", want, out.String())
	}
}

// civitai/cli#941 — a dry run whose target cannot be resolved (server filename
// ".") used to print "status: would download" and exit 0, while the real run
// refused with exit 1. The dry run must fail with the SAME error and the same
// classification as the real run, and must not say it would download.
func TestDownloadDryRunUnresolvedTargetFailsLikeRealRun(t *testing.T) {
	const wantMsg = `server returned an unusable filename "."; pass --out to set the output path`

	d := newDLServer(t, false, []dlFile{
		{id: 789, name: ".", typ: "Model", primary: true, body: "w"},
	})
	setupDownloadEnv(t, d, "")

	out, _, dryErr := run(t, "download", "--version", "123", "--dry-run")
	if dryErr == nil {
		t.Fatalf("dry run with an unresolved target must exit nonzero; plan was:\n%s", out)
	}
	if dryErr.Error() != wantMsg {
		t.Errorf("dry-run error:\n  got  %q\n  want %q", dryErr.Error(), wantMsg)
	}
	if strings.Contains(out, "would download") {
		t.Errorf("an unresolved target must not plan as would-download:\n%s", out)
	}
	if !strings.Contains(out, "  status: blocked — target unresolved\n") {
		t.Errorf("plan should mark the file blocked:\n%s", out)
	}

	_, _, realErr := run(t, "download", "--version", "123")
	if realErr == nil || realErr.Error() != wantMsg {
		t.Fatalf("CONTROL failure, not a finding: real run error = %v, want %q", realErr, wantMsg)
	}
	// Same exit-code class: both unclassified (exit 1), neither a usage error.
	for _, sentinel := range []error{ErrUsage, civitai.ErrNotFound, civitai.ErrUnauthorized, civitai.ErrNetwork} {
		if errors.Is(dryErr, sentinel) != errors.Is(realErr, sentinel) {
			t.Errorf("dry-run and real-run errors classify differently for %v", sentinel)
		}
	}
	if d.dlHits != 0 {
		t.Errorf("neither run may fetch the file (dlHits=%d)", d.dlHits)
	}
	ents, _ := os.ReadDir(".")
	if len(ents) != 0 {
		t.Errorf("no file may be created, found %d entries", len(ents))
	}
}

// #941, multi-file: with several blocked files the plan must still render
// EVERY file (a blocked file must not cut the plan short) and return the FIRST
// blocked file's error, not the last. The two blocked names differ ("." and
// "..") so first-vs-last is observable, and the good file comes after both so
// an early return would drop it.
func TestDownloadDryRunAllPlansEveryFileAndReturnsFirstBlockedError(t *testing.T) {
	const wantMsg = `server returned an unusable filename "."; pass --out to set the output path`

	d := newDLServer(t, false, []dlFile{
		{id: 801, name: ".", typ: "Model", primary: true, body: "a"},
		{id: 802, name: "..", typ: "Model", body: "bb"},
		{id: 803, name: "good.safetensors", typ: "Model", body: "ccc", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	dir := t.TempDir()

	out, _, err := run(t, "download", "--version", "123", "--all", "--out-dir", dir, "--dry-run")
	if err == nil {
		t.Fatalf("a plan with blocked files must exit nonzero; plan was:\n%s", out)
	}
	if err.Error() != wantMsg {
		t.Errorf("error must name the FIRST blocked file:\n  got  %q\n  want %q", err.Error(), wantMsg)
	}
	if got := strings.Count(out, "  status: blocked — target unresolved\n"); got != 2 {
		t.Errorf("both blocked files should be marked blocked, got %d:\n%s", got, out)
	}
	const goodBlock = "\ngood.safetensors\n  id:     803\n"
	i := strings.Index(out, goodBlock)
	if i < 0 {
		t.Fatalf("the good file after the blocked ones must still be planned:\n%s", out)
	}
	if !strings.Contains(out[i:], "  status: would download\n") {
		t.Errorf("the good file should plan as would-download:\n%s", out)
	}
	if !dirIsEmpty(t, dir) || d.dlHits != 0 {
		t.Errorf("dry-run must write and fetch nothing (dlHits=%d)", d.dlHits)
	}
}

// Control for #941: an explicit --out resolves the same unusable server name,
// so the dry run still exits 0 and plans the download.
func TestDownloadDryRunUnusableNameWithOutStillPlans(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 789, name: ".", typ: "Model", primary: true, body: "w"},
	})
	setupDownloadEnv(t, d, "")

	out, _, err := run(t, "download", "--version", "123", "--dry-run", "--out", "local.safetensors")
	if err != nil {
		t.Fatalf("dry run with --out must exit 0, got %v", err)
	}
	if !strings.Contains(out, "  target: local.safetensors\n") || !strings.Contains(out, "  status: would download\n") {
		t.Errorf("--out should resolve the target and plan the download:\n%s", out)
	}
	if _, e := os.Stat("local.safetensors"); e == nil {
		t.Error("dry-run must not create the --out target")
	}
}
