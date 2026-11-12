package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// Unit coverage for the git credential helper (`civitai git-credential`). The
// real-git protocol run — git itself invoking the helper — is in
// git_credential_git_test.go.

const credHost = "git.example.test"
const credRepoURL = "https://" + credHost + "/civitai-apps/my-block.git"

func credInfo() *appapi.ForgejoCloneInfo {
	return &appapi.ForgejoCloneInfo{
		Slug:            "my-block",
		ForgejoUsername: "dev-7",
		Token:           "tok-sha1",
		HTTPURL:         credRepoURL,
		CloneURL:        "https://dev-7:tok-sha1@" + credHost + "/civitai-apps/my-block.git",
	}
}

func okReq() credentialRequest {
	return credentialRequest{Protocol: "https", Host: credHost, Path: "civitai-apps/my-block.git"}
}

func TestReadCredentialRequestParsesGitInput(t *testing.T) {
	in := "protocol=https\nhost=" + credHost + "\npath=civitai-apps/my-block.git\nusername=ignored\nwwwauth[]=Basic realm=x\n\nprotocol=NOT-READ\n"
	req, err := readCredentialRequest(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if req != okReq() {
		t.Fatalf("req = %+v, want %+v (parsing must stop at the blank line)", req, okReq())
	}
	// CRLF input (git for Windows) parses the same.
	req, err = readCredentialRequest(strings.NewReader(strings.ReplaceAll(in, "\n", "\r\n")))
	if err != nil || req != okReq() {
		t.Fatalf("CRLF: req = %+v, err = %v", req, err)
	}
	if _, err := readCredentialRequest(strings.NewReader("protocol\n")); err == nil {
		t.Fatal("a line with no '=' is a protocol error")
	}
}

// TestMatchCredentialRequest is the helper's ONLY guard on which request gets
// the token. Each refusal row differs from the accepted row in exactly one
// field, so a mutant that drops one comparison turns exactly that row green.
func TestMatchCredentialRequest(t *testing.T) {
	type tc struct {
		name    string
		req     credentialRequest
		app     string
		mutate  func(*appapi.ForgejoCloneInfo)
		wantErr string // "" = accepted
	}
	cases := []tc{
		{name: "exact repository", req: okReq(), app: "my-block"},
		{name: "leading slash in path", req: credentialRequest{Protocol: "https", Host: credHost, Path: "/civitai-apps/my-block.git"}, app: "my-block"},
		{name: "host compared case-insensitively", req: credentialRequest{Protocol: "https", Host: strings.ToUpper(credHost), Path: "civitai-apps/my-block.git"}, app: "my-block"},
		{name: "wrong slug: server answered for another app", req: okReq(), app: "my-block",
			mutate: func(i *appapi.ForgejoCloneInfo) { i.Slug = "other-block" }, wantErr: `answered for app "other-block"`},
		{name: "wrong slug: git asked for another repository", req: credentialRequest{Protocol: "https", Host: credHost, Path: "civitai-apps/other-block.git"}, app: "my-block",
			wantErr: `git asked for repository "civitai-apps/other-block.git"`},
		{name: "another host", req: credentialRequest{Protocol: "https", Host: "evil.example.test", Path: "civitai-apps/my-block.git"}, app: "my-block",
			wantErr: "git asked for https://evil.example.test"},
		{name: "same host and port mismatch", req: credentialRequest{Protocol: "https", Host: credHost + ":8443", Path: "civitai-apps/my-block.git"}, app: "my-block",
			wantErr: "git asked for https://" + credHost + ":8443"},
		{name: "downgraded protocol", req: credentialRequest{Protocol: "http", Host: credHost, Path: "civitai-apps/my-block.git"}, app: "my-block",
			wantErr: "git asked for http://"},
		{name: "no path (useHttpPath removed)", req: credentialRequest{Protocol: "https", Host: credHost}, app: "my-block",
			wantErr: "did not send the repository path"},
		{name: "server URL carries a credential", req: okReq(), app: "my-block",
			mutate: func(i *appapi.ForgejoCloneInfo) { i.HTTPURL = i.CloneURL }, wantErr: "carries a credential"},
		{name: "server URL is plain http off-machine", req: credentialRequest{Protocol: "http", Host: credHost, Path: "civitai-apps/my-block.git"}, app: "my-block",
			mutate: func(i *appapi.ForgejoCloneInfo) { i.HTTPURL = "http://" + credHost + "/civitai-apps/my-block.git" }, wantErr: "plain http on a non-local host"},
		{name: "plain http on loopback (local platform)", req: credentialRequest{Protocol: "http", Host: "127.0.0.1:3000", Path: "civitai-apps/my-block.git"}, app: "my-block",
			mutate: func(i *appapi.ForgejoCloneInfo) { i.HTTPURL = "http://127.0.0.1:3000/civitai-apps/my-block.git" }},
		{name: "server URL not http(s)", req: okReq(), app: "my-block",
			mutate: func(i *appapi.ForgejoCloneInfo) { i.HTTPURL = "ext::sh -c id" }, wantErr: "no usable repository URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := credInfo()
			if c.mutate != nil {
				c.mutate(info)
			}
			err := matchCredentialRequest(c.req, c.app, info)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the request to be answered, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, c.wantErr)
			}
			if strings.Contains(err.Error(), "tok-sha1") {
				t.Fatalf("a refusal must never carry the token: %v", err)
			}
		})
	}
}

func TestRunGitCredentialGet(t *testing.T) {
	in := "protocol=https\nhost=" + credHost + "\npath=civitai-apps/my-block.git\n\n"
	t.Run("answers the matching request", func(t *testing.T) {
		var out bytes.Buffer
		err := runGitCredentialGet(context.Background(), strings.NewReader(in), &out, "my-block",
			func(ctx context.Context, app string) (*appapi.ForgejoCloneInfo, error) { return credInfo(), nil })
		if err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "username=dev-7\npassword=tok-sha1\n"; got != want {
			t.Fatalf("out = %q, want %q", got, want)
		}
	})
	refusals := []struct {
		name  string
		fetch credentialFetcher
		want  string
	}{
		{"expired civitai credential (401)", func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
			return nil, civitai.Tag(civitai.ErrUnauthorized, errors.New("not logged in (401)"))
		}, "not logged in"},
		{"app not approved yet", func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
			return &appapi.ForgejoCloneInfo{NotYetAvailable: true, Slug: "my-block", Message: "approve first"}, nil
		}, "not available yet: approve first"},
		{"server returned no credential", func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
			i := credInfo()
			i.Token = ""
			return i, nil
		}, "no git credential"},
		{"credential with a newline (protocol injection)", func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
			i := credInfo()
			i.Token = "tok\nusername=attacker"
			return i, nil
		}, "malformed git credential"},
		{"request for another repository", func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
			i := credInfo()
			i.HTTPURL = "https://" + credHost + "/civitai-apps/elsewhere.git"
			return i, nil
		}, "not answering"},
	}
	for _, r := range refusals {
		t.Run(r.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runGitCredentialGet(context.Background(), strings.NewReader(in), &out, "my-block", r.fetch)
			if err == nil || !strings.Contains(err.Error(), r.want) {
				t.Fatalf("err = %v, want %q", err, r.want)
			}
			// quit=1 and NOTHING else: no username/password line, so git stops
			// instead of prompting, and never receives a partial credential.
			if out.String() != "quit=1\n" {
				t.Fatalf("out = %q, want exactly quit=1", out.String())
			}
		})
	}
	t.Run("expired credential keeps its exit-code class", func(t *testing.T) {
		var out bytes.Buffer
		err := runGitCredentialGet(context.Background(), strings.NewReader(in), &out, "my-block",
			func(context.Context, string) (*appapi.ForgejoCloneInfo, error) {
				return nil, civitai.Tag(civitai.ErrUnauthorized, errors.New("not logged in (401)"))
			})
		if !errors.Is(err, civitai.ErrUnauthorized) {
			t.Fatalf("err = %v, want ErrUnauthorized (exit 3)", err)
		}
	})
}

// runHelperCmd drives the real command (as git would) with stdin.
func runHelperCmd(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func TestGitCredentialCommand(t *testing.T) {
	info := map[string]any{
		"notYetAvailable": false, "slug": "my-block", "forgejoUsername": "dev-7", "token": "tok-sha1",
		"cloneUrl": "https://dev-7:tok-sha1@" + credHost + "/civitai-apps/my-block.git", "httpUrl": credRepoURL,
	}
	in := "protocol=https\nhost=" + credHost + "\npath=civitai-apps/my-block.git\n\n"

	t.Run("get answers through the stored civitai credential", func(t *testing.T) {
		var gotInput string
		srv := cloneInfoServer(t, info, http.StatusOK, &gotInput)
		defer srv.Close()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "tok-1")
		t.Setenv("CIVITAI_BASE_URL", srv.URL)
		out, _, err := runHelperCmd(t, in, "git-credential", "--app", "my-block", "get")
		if err != nil {
			t.Fatal(err)
		}
		if out != "username=dev-7\npassword=tok-sha1\n" {
			t.Fatalf("out = %q", out)
		}
		if !strings.Contains(gotInput, `"slug":"my-block"`) {
			t.Fatalf("the helper must ask for --app's credential: %q", gotInput)
		}
	})
	t.Run("no token configured", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "")
		out, _, err := runHelperCmd(t, in, "git-credential", "--app", "my-block", "get")
		if !errors.Is(err, civitai.ErrUnauthorized) || !strings.Contains(err.Error(), "civitai login") {
			t.Fatalf("err = %v, want ErrUnauthorized naming `civitai login`", err)
		}
		if out != "quit=1\n" {
			t.Fatalf("out = %q, want quit=1", out)
		}
	})
	t.Run("expired credential: the server's 401", func(t *testing.T) {
		srv := cloneInfoServer(t, nil, http.StatusUnauthorized, nil)
		defer srv.Close()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("CIVITAI_TOKEN", "tok-expired")
		t.Setenv("CIVITAI_BASE_URL", srv.URL)
		out, _, err := runHelperCmd(t, in, "git-credential", "--app", "my-block", "get")
		if !errors.Is(err, civitai.ErrUnauthorized) {
			t.Fatalf("err = %v, want ErrUnauthorized", err)
		}
		if out != "quit=1\n" {
			t.Fatalf("out = %q, want quit=1", out)
		}
	})
	for _, op := range []string{"store", "erase", "some-future-op"} {
		t.Run(op+" is a silent no-op that never calls the server", func(t *testing.T) {
			// A base URL that cannot answer: reaching it would fail the test.
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("CIVITAI_TOKEN", "tok-1")
			t.Setenv("CIVITAI_BASE_URL", "http://127.0.0.1:1")
			out, errOut, err := runHelperCmd(t, in+"username=dev-7\npassword=tok-sha1\n", "git-credential", "--app", "my-block", op)
			if err != nil || out != "" || errOut != "" {
				t.Fatalf("op %s: err=%v out=%q err=%q", op, err, out, errOut)
			}
		})
	}
	t.Run("an --app that could not have been installed is a usage error", func(t *testing.T) {
		_, _, err := runHelperCmd(t, in, "git-credential", "--app", "a;id", "get")
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("err = %v, want ErrUsage", err)
		}
	})
}

func TestCredentialHelperConfig(t *testing.T) {
	orig := helperExecutable
	t.Cleanup(func() { helperExecutable = orig })
	helperExecutable = func() string { return "civitai" }

	cfg, err := credentialHelperConfig("https://"+credHost+":8443/civitai-apps/my-block.git", "my-block")
	if err != nil {
		t.Fatal(err)
	}
	ctx := "credential.https://" + credHost + ":8443"
	want := [][2]string{
		{ctx + ".helper", ""},
		{ctx + ".helper", "!civitai git-credential --app my-block"},
		{ctx + ".useHttpPath", "true"},
	}
	if fmt.Sprint(cfg) != fmt.Sprint(want) {
		t.Fatalf("cfg = %q, want %q", cfg, want)
	}
	for _, bad := range []string{"", "a b", "a;b", "$(id)", "../x", "-x", strings.Repeat("a", 65)} {
		if _, err := credentialHelperConfig(credRepoURL, bad); err == nil {
			t.Errorf("slug %q must be refused", bad)
		}
	}
	if _, err := credentialHelperConfig("https://u:p@"+credHost+"/x.git", "my-block"); err == nil {
		t.Error("a repository URL carrying a credential must be refused")
	}
}

func TestShellQuoteSurvivesSh(t *testing.T) {
	if got, want := shellQuote(`/opt/it's here/civitai`), `'/opt/it'\''s here/civitai'`; got != want {
		t.Fatalf("shellQuote = %s, want %s", got, want)
	}
}

func TestStripURLCredential(t *testing.T) {
	got, err := stripURLCredential("https://dev-7:tok-sha1@" + credHost + "/civitai-apps/my-block.git")
	if err != nil || got != credRepoURL {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := stripURLCredential(credRepoURL); !errors.Is(err, errNoTokenInURL) {
		t.Fatalf("a URL without a credential: err = %v", err)
	}
}
