package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Tests for the missing-`repository` nudge (app_submit_repository.go). Every
// expectation is a literal: a URL, a whole advisory, the whole manifest.

const (
	nudgeRepoURL = "https://github.com/acme/my-app"

	// wantAdvisoryWithRemote / wantAdvisoryNoRemote are the whole advisory as a
	// whitespace-normalised stderr carries it.
	wantAdvisoryWithRemote = `⚠ Advisory (it does not block this submit): - No "repository" in block.manifest.json. ` +
		`A public source link helps reviewers check the code. Your git remote origin is ` +
		`https://github.com/acme/my-app. If that repository is public, add it to the manifest: ` +
		`"repository": "https://github.com/acme/my-app"`
	wantAdvisoryNoRemote = `⚠ Advisory (it does not block this submit): - No "repository" in block.manifest.json. ` +
		`A public source link helps reviewers check the code. If the source is public on github.com, ` +
		`gitlab.com or codeberg.org, add it to the manifest: "repository": "https://github.com/<owner>/<repo>"`

	offerPrompt = "Add repository https://github.com/acme/my-app to block.manifest.json? [y/N]"
)

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestRepositoryFromRemote pins which remotes are offered, and as what.
func TestRepositoryFromRemote(t *testing.T) {
	cases := []struct {
		remote string
		want   string // "" = not offered
	}{
		{"git@github.com:acme/my-app.git", nudgeRepoURL},
		{"git@github.com:acme/my-app", nudgeRepoURL},
		{"ssh://git@github.com/acme/my-app.git", nudgeRepoURL},
		{"https://github.com/acme/my-app.git", nudgeRepoURL},
		{"https://github.com/acme/my-app/", nudgeRepoURL},
		{"https://GitHub.COM/acme/my-app.GIT\n", nudgeRepoURL},
		{"git@gitlab.com:grp/proj.git", "https://gitlab.com/grp/proj"},
		{"https://codeberg.org/o/r", "https://codeberg.org/o/r"},
		// Not offered:
		{"", ""},
		{"/home/me/src/my-app", ""},
		{"../my-app.git", ""},
		{"./a:b/c", ""},
		{"file:///srv/git/acme/my-app.git", ""},
		{"http://github.com/acme/my-app", ""},
		{"git://github.com/acme/my-app.git", ""},
		{"https://example.com/acme/my-app", ""},
		{"git@bitbucket.org:acme/my-app.git", ""},
		{"https://github.com.evil.test/acme/my-app", ""},
		{"https://user:s3cret@github.com/acme/my-app.git", ""},
		{"https://x-access-token@github.com/acme/my-app", ""},
		{"ssh://git:pw@github.com/acme/my-app", ""},
		{"https://github.com:8443/acme/my-app", ""},
		{"ssh://git@github.com:22/acme/my-app", ""},
		{"https://github.com/acme", ""},
		{"https://github.com/acme/my-app/tree/main", ""},
		{"https://github.com/acme/my-app?x=1", ""},
		{"https://github.com/acme/my-app?", ""},
		{"https://github.com/acme/my-app#", ""},
		{"https://gitlab.com/grp/sub/proj", ""},
		{"https://github.com/acme/.git", ""},
		{"https://github.com/-acme/my-app", ""},
		{"https://github.com/acme/my%20app", ""},
		// git reads a slash before the first colon as a local path, even when
		// what follows an @ looks like an allowed host.
		{"dir/git@github.com:acme/my-app", ""},
	}
	for _, tc := range cases {
		got, ok := repositoryFromRemote(tc.remote)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("repositoryFromRemote(%q) = (%q, %v), want (%q, %v)", tc.remote, got, ok, tc.want, tc.want != "")
		}
	}
}

// nudgeFixture is a committed app in a real repo with origin set to remote and
// HEAD published (so the dirty guard needs no network). declareRepo adds a
// `repository` key to the committed manifest.
func nudgeFixture(t *testing.T, remote string, declareRepo bool) *gitFixture {
	t.Helper()
	f := newGitFixture(t)
	writeManifestVersion(t, f.root, dirtySlug, "0.6.1")
	if declareRepo {
		p := filepath.Join(f.root, "block.manifest.json")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		b = []byte(strings.Replace(string(b), `"minApiVersion": "1.0"`,
			`"minApiVersion": "1.0",`+"\n  "+`"repository": "https://github.com/acme/my-app"`, 1))
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.commit("app", "block.manifest.json", "index.html")
	if remote != "" {
		f.git("remote", "add", "origin", remote)
	}
	f.publishHEAD()
	return f
}

// submitEnv points the CLI at a server that counts every request it gets.
func submitEnv(t *testing.T) *atomic.Int64 {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"publishRequestId":"pubreq_mock","slug":"custom-generators","version":"0.6.1","status":"pending"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-nudge")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "/api/blocks/submit-version")
	return &hits
}

func readNudgeManifest(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Missing repository + a GitHub ssh remote: the advisory names the normalised
// https URL. --package-only, so this is the path every submit takes.
func TestSubmitAdvisoryNamesTheNormalisedRemote(t *testing.T) {
	clearUnattendedSignals(t)
	f := nudgeFixture(t, "git@github.com:acme/my-app.git", false)
	_, stderr, err := run(t, "app", "submit", f.root, "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))
	if err != nil {
		t.Fatalf("an advisory must not fail the submit: %v\n%s", err, stderr)
	}
	if !strings.Contains(squash(stderr), wantAdvisoryWithRemote) {
		t.Errorf("advisory missing or changed.\nwant: %s\ngot:  %s", wantAdvisoryWithRemote, squash(stderr))
	}
}

// A non-allowlisted host and a local-path remote get the generic advisory,
// with no URL from the remote in it.
func TestSubmitAdvisoryDoesNotOfferAnUnacceptedRemote(t *testing.T) {
	for _, remote := range []string{"https://example.com/acme/my-app.git", "/srv/git/my-app.git", ""} {
		t.Run(remote, func(t *testing.T) {
			clearUnattendedSignals(t)
			f := nudgeFixture(t, remote, false)
			_, stderr, err := run(t, "app", "submit", f.root, "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))
			if err != nil {
				t.Fatalf("submit: %v\n%s", err, stderr)
			}
			if !strings.Contains(squash(stderr), wantAdvisoryNoRemote) {
				t.Errorf("want the generic advisory.\nwant: %s\ngot:  %s", wantAdvisoryNoRemote, squash(stderr))
			}
			if strings.Contains(stderr, "my-app") {
				t.Errorf("a remote that is not offered must not be echoed, got:\n%s", stderr)
			}
		})
	}
}

// Repository declared: no advisory. The control is the first test above, on
// the same fixture minus the key.
func TestSubmitNoAdvisoryWhenRepositoryDeclared(t *testing.T) {
	clearUnattendedSignals(t)
	f := nudgeFixture(t, "git@github.com:acme/my-app.git", true)
	_, stderr, err := run(t, "app", "submit", f.root, "--package-only", "--out", filepath.Join(t.TempDir(), "b.zip"))
	if err != nil {
		t.Fatalf("submit: %v\n%s", err, stderr)
	}
	if strings.Contains(stderr, `No "repository"`) || strings.Contains(stderr, "Advisory") {
		t.Errorf("a manifest that declares repository must get no advisory, got:\n%s", stderr)
	}
}

// TTY + "y": the key is written after blockId, nothing else in the manifest
// moves, and the submit STOPS before packaging, prompting or contacting the
// server.
func TestSubmitOfferYesWritesTheKeyAndStops(t *testing.T) {
	clearUnattendedSignals(t)
	withStdinTTY(t, true)
	f := nudgeFixture(t, "git@github.com:acme/my-app.git", false)
	hits := submitEnv(t)
	before := readNudgeManifest(t, f.root)

	stdout, stderr, err := runWithStdin(t, "y\n", "app", "submit", f.root)
	if !errors.Is(err, errSubmitStoppedForRepository) {
		t.Fatalf("want errSubmitStoppedForRepository, got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, offerPrompt) {
		t.Errorf("the offer was not shown, stdout:\n%s", stdout)
	}
	want := strings.Replace(before, `"blockId": "custom-generators",`,
		`"blockId": "custom-generators",`+"\n  "+`"repository": "https://github.com/acme/my-app",`, 1)
	if want == before {
		t.Fatal("CONTROL: the expected manifest equals the original — the fixture no longer has the blockId line")
	}
	if got := readNudgeManifest(t, f.root); got != want {
		t.Errorf("manifest after yes:\n%s\nwant:\n%s", got, want)
	}
	for _, s := range []string{"Packaged", "Submit for review?"} {
		if strings.Contains(stdout, s) {
			t.Errorf("the run must stop before %q, stdout:\n%s", s, stdout)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the run must not contact the server after editing the manifest, got %d request(s)", n)
	}
}

// TTY + default answer: the manifest is untouched and the submit carries on —
// its own confirmation reads the NEXT line of the same stdin ("y"), so the
// upload happens. That second answer only arrives if the offer read exactly one
// line.
func TestSubmitOfferDefaultNoContinues(t *testing.T) {
	clearUnattendedSignals(t)
	withStdinTTY(t, true)
	f := nudgeFixture(t, "git@github.com:acme/my-app.git", false)
	hits := submitEnv(t)
	before := readNudgeManifest(t, f.root)

	stdout, stderr, err := runWithStdin(t, "\ny\n", "app", "submit", f.root)
	if err != nil {
		t.Fatalf("No must let the submit continue and the confirmation's yes must upload, got %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, offerPrompt) || !strings.Contains(stdout, "Submit for review? [y/N]") {
		t.Errorf("want the offer, then the confirmation, stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "pubreq_mock") {
		t.Errorf("want the submit to have gone through, stdout:\n%s", stdout)
	}
	if got := readNudgeManifest(t, f.root); got != before {
		t.Errorf("No must leave the manifest untouched, got:\n%s", got)
	}
	if n := hits.Load(); n == 0 {
		t.Error("the confirmed submit never reached the server")
	}
}

// No prompt where nobody can answer it: --yes, GITHUB_ACTIONS, generic CI, an
// OIDC token, a non-TTY stdin. Each row would otherwise be the case above.
func TestSubmitOfferNeverPromptsUnattended(t *testing.T) {
	cases := []struct {
		name string
		tty  bool
		env  map[string]string
		args []string
	}{
		{"--yes on a TTY", true, nil, []string{"--yes"}},
		{"GITHUB_ACTIONS", true, map[string]string{"GITHUB_ACTIONS": "true"}, nil},
		{"CI", true, map[string]string{"CI": "1"}, nil},
		{"OIDC token", true, map[string]string{GitHubOIDCTokenEnv: "eyJ.fake.jwt"}, nil},
		{"non-TTY", false, nil, nil},
		{"non-TTY --yes", false, nil, []string{"--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearUnattendedSignals(t)
			f := nudgeFixture(t, "git@github.com:acme/my-app.git", false)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			withStdinTTY(t, tc.tty)
			submitEnv(t)
			before := readNudgeManifest(t, f.root)

			args := append([]string{"app", "submit", f.root}, tc.args...)
			stdout, stderr, _ := runWithStdin(t, "y\nn\n", args...)
			if strings.Contains(stdout, "Add repository") {
				t.Errorf("must not prompt, stdout:\n%s", stdout)
			}
			if got := readNudgeManifest(t, f.root); got != before {
				t.Errorf("must not edit the manifest, got:\n%s", got)
			}
			if !strings.Contains(squash(stderr), wantAdvisoryWithRemote) {
				t.Errorf("the advisory still prints unattended.\nwant: %s\ngot:  %s", wantAdvisoryWithRemote, squash(stderr))
			}
		})
	}
}

// doctor prints the advisory for the app in the current directory, and not for
// a different slug.
func TestDoctorRepositoryAdvisory(t *testing.T) {
	clearUnattendedSignals(t)
	f := nudgeFixture(t, "git@github.com:acme/my-app.git", false)
	wantDoctor := strings.Replace(wantAdvisoryWithRemote, "it does not block this submit", "it does not change the exit code", 1)

	var b strings.Builder
	printDoctorRepositoryAdvisory(&b, gitOutput, f.root, "")
	if squash(b.String()) != wantDoctor {
		t.Errorf("no slug:\nwant: %s\ngot:  %s", wantDoctor, squash(b.String()))
	}
	b.Reset()
	printDoctorRepositoryAdvisory(&b, gitOutput, f.root, "Custom-Generators")
	if squash(b.String()) != wantDoctor {
		t.Errorf("own slug:\nwant: %s\ngot:  %s", wantDoctor, squash(b.String()))
	}
	b.Reset()
	printDoctorRepositoryAdvisory(&b, gitOutput, f.root, "some-other-app")
	if b.String() != "" {
		t.Errorf("another app's slug must print nothing, got:\n%s", b.String())
	}
	b.Reset()
	printDoctorRepositoryAdvisory(&b, gitOutput, t.TempDir(), "")
	if b.String() != "" {
		t.Errorf("a directory with no manifest must print nothing, got:\n%s", b.String())
	}
}
