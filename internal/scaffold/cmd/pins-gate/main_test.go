package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/civitai/cli/internal/scaffold"
)

// ─── the two decision tables ─────────────────────────────────────────────────
//
// Every expectation below is a LITERAL. None is computed from pinInputs or from
// blocks(): a table derived from the thing it tests passes whatever that thing
// says.

func TestClassifyDecidesWhichRunsGate(t *testing.T) {
	someDiffErr := errors.New("git diff: exit status 128: fatal: bad object deadbeef")

	cases := []struct {
		name    string
		event   string
		changed []string
		diffErr error
		want    mode
		touched []string // literal, in order
	}{
		// ── pull requests that touch no pin input: the downgrade ──
		{name: "docs-only PR", event: "pull_request", changed: []string{"README.md", "claudedocs/handoff.md", "AGENTS.md"}, want: modeAdvise},
		{name: "internal/cmd-only PR", event: "pull_request", changed: []string{"internal/cmd/login.go", "internal/cmd/login_test.go"}, want: modeAdvise},
		{name: "another workflow file", event: "pull_request", changed: []string{".github/workflows/release.yml"}, want: modeAdvise},
		// The bump bot's workflow decides when pins get FIXED, not whether they
		// are stale: this command reads nothing from it.
		{name: "the bump bot's own workflow", event: "pull_request", changed: []string{".github/workflows/bump-scaffold-pins.yml"}, want: modeAdvise},

		// ── pull requests that touch a pin input: gate, as before ──
		{name: "template change", event: "pull_request", changed: []string{"internal/scaffold/templates/page-money/package.json.tmpl"}, want: modeGate,
			touched: []string{"internal/scaffold/templates/page-money/package.json.tmpl"}},
		{name: "a template file that is not a package.json", event: "pull_request", changed: []string{"internal/scaffold/templates/static/index.html.tmpl"}, want: modeGate,
			touched: []string{"internal/scaffold/templates/static/index.html.tmpl"}},
		{name: "pin-table change", event: "pull_request", changed: []string{"internal/scaffold/pins.go"}, want: modeGate,
			touched: []string{"internal/scaffold/pins.go"}},
		{name: "script change", event: "pull_request", changed: []string{"internal/scaffold/cmd/pins-gate/main.go"}, want: modeGate,
			touched: []string{"internal/scaffold/cmd/pins-gate/main.go"}},
		{name: "ci.yml change", event: "pull_request", changed: []string{".github/workflows/ci.yml"}, want: modeGate,
			touched: []string{".github/workflows/ci.yml"}},
		{name: "go.mod change", event: "pull_request", changed: []string{"go.mod"}, want: modeGate, touched: []string{"go.mod"}},
		{name: "go.sum change", event: "pull_request", changed: []string{"go.sum"}, want: modeGate, touched: []string{"go.sum"}},
		{name: "a package scaffold imports", event: "pull_request", changed: []string{"internal/blockproto/ready-ack.js"}, want: modeGate,
			touched: []string{"internal/blockproto/ready-ack.js"}},
		{name: "a vendor tree appears", event: "pull_request", changed: []string{"vendor/modules.txt"}, want: modeGate, touched: []string{"vendor/modules.txt"}},
		{name: "a workspace file appears", event: "pull_request", changed: []string{"go.work"}, want: modeGate, touched: []string{"go.work"}},
		{name: "one pin input among many harmless paths", event: "pull_request",
			changed: []string{"README.md", "internal/cmd/root.go", "internal/scaffold/scaffold_test.go", "main.go"}, want: modeGate,
			touched: []string{"internal/scaffold/scaffold_test.go"}},
		// "An unclassified path that could contain a pin": the one file kind a
		// pin is read from, somewhere no rule names.
		{name: "a package.json.tmpl outside the known tree", event: "pull_request", changed: []string{"internal/newtemplates/page-x/package.json.tmpl"}, want: modeGate,
			touched: []string{"internal/newtemplates/page-x/package.json.tmpl"}},

		// ── near misses: a prefix rule must not be a substring rule ──
		{name: "a sibling directory sharing the prefix's spelling", event: "pull_request", changed: []string{"internal/scaffolding/x.go"}, want: modeAdvise},
		{name: "the input path nested under another root", event: "pull_request", changed: []string{"docs/internal/scaffold/pins.go"}, want: modeAdvise},
		{name: "a file merely named like ci.yml", event: "pull_request", changed: []string{".github/workflows/ci.yml.md", "docs/ci.yml"}, want: modeAdvise},
		{name: "a root file merely named like go.mod", event: "pull_request", changed: []string{"examples/app/go.mod"}, want: modeAdvise},

		// ── not knowing is not the same as not touching: fail CLOSED ──
		{name: "diff error", event: "pull_request", diffErr: someDiffErr, want: modeGate},
		{name: "diff error even with a harmless-looking partial list", event: "pull_request", changed: []string{"README.md"}, diffErr: someDiffErr, want: modeGate},
		{name: "empty diff", event: "pull_request", changed: nil, want: modeGate},

		// ── every other event gates, whatever the diff says ──
		{name: "push to main, docs-only", event: "push", changed: []string{"README.md"}, want: modeGate},
		{name: "schedule", event: "schedule", want: modeGate},
		{name: "workflow_dispatch", event: "workflow_dispatch", want: modeGate},
		{name: "merge_group", event: "merge_group", changed: []string{"README.md"}, want: modeGate},
		// Its checkout is the BASE branch, so a diff there is not the PR's.
		{name: "pull_request_target", event: "pull_request_target", changed: []string{"README.md"}, want: modeGate},
		{name: "an event this command has never heard of", event: "issue_comment", changed: []string{"README.md"}, want: modeGate},
		{name: "no event name at all", event: "", changed: []string{"README.md"}, want: modeGate},
		{name: "event name differing only in case", event: "Pull_Request", changed: []string{"README.md"}, want: modeGate},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.event, c.changed, c.diffErr)
			if got.Mode != c.want {
				t.Fatalf("classify(%q, %q, %v) = mode %q, want %q.\nreason given: %s", c.event, c.changed, c.diffErr, got.Mode, c.want, got.Reason)
			}
			if !reflect.DeepEqual(got.Touched, c.touched) {
				t.Errorf("classify(%q, %q) named the touched pin inputs as %q, want %q — the report tells a reader WHY their run gates, so a wrong list is a wrong explanation", c.event, c.changed, got.Touched, c.touched)
			}
			if got.Reason == "" {
				t.Errorf("classify(%q, …) gave no reason. Every verdict is printed with one; an empty one is a run nobody can explain.", c.event)
			}
		})
	}
}

func TestBlocksIsTheWholeBehaviourMatrix(t *testing.T) {
	cases := []struct {
		m    mode
		s    scaffold.PinStatus
		want bool
	}{
		{modeAdvise, scaffold.PinOK, false},
		{modeAdvise, scaffold.PinStale, false},       // external: reported, not enforced
		{modeAdvise, scaffold.PinUnreachable, false}, // nothing learned: reported distinctly, not enforced
		{modeAdvise, scaffold.PinNotFound, true},     // the tree is wrong: never downgraded
		{modeAdvise, scaffold.PinInvalid, true},      // the tree is wrong: never downgraded
		{modeGate, scaffold.PinOK, false},
		{modeGate, scaffold.PinStale, true},
		{modeGate, scaffold.PinUnreachable, true}, // a gate that could not look must not report green
		{modeGate, scaffold.PinNotFound, true},
		{modeGate, scaffold.PinInvalid, true},
		// A status nobody taught this function about, and a mode nobody did:
		// both must land on "blocks".
		{modeAdvise, scaffold.PinStatus(99), true},
		{mode("something-new"), scaffold.PinStale, true},
		{mode(""), scaffold.PinUnreachable, true},
	}
	for _, c := range cases {
		if got := blocks(c.m, c.s); got != c.want {
			t.Errorf("blocks(mode=%q, %s) = %v, want %v", c.m, c.s, got, c.want)
		}
	}
}

// ─── the pin-input table against the build graph ─────────────────────────────

// TestPinInputsCoverTheBuildGraph is what makes pinInputs a DERIVED list rather
// than a remembered one. The verdict of this command is a function of the
// packages it is compiled from; `go list -deps` names them. The in-module set
// must equal the literal below — so it fails when the set GROWS (a new import
// whose directory nothing gates on) and when it SHRINKS (a row that guards a
// dependency that no longer exists) — and every member must be matched by a
// pinInputs rule.
func TestPinInputsCoverTheBuildGraph(t *testing.T) {
	const module = "github.com/civitai/cli"

	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	var got []string
	for _, line := range strings.Fields(string(out)) {
		if line == module || strings.HasPrefix(line, module+"/") {
			got = append(got, strings.TrimPrefix(strings.TrimPrefix(line, module), "/"))
		}
	}
	sort.Strings(got)

	want := []string{
		"internal/blockproto",
		"internal/scaffold",
		"internal/scaffold/cmd/pins-gate",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the in-module packages pins-gate is built from changed.\n  go list -deps: %q\n  ledger:        %q\n\n"+
			"Its verdict is a function of every package in that list, so each one's directory must be a pin input: a pull request "+
			"that edits one of them can change whether a pin is reported stale. Add (or remove) the matching row in pinInputs, then "+
			"update this ledger. Do not edit only the ledger.", got, want)
	}
	for _, dir := range got {
		// A file directly inside the package directory.
		if probe := dir + "/x.go"; !isPinInput(probe) {
			t.Errorf("%s is compiled into pins-gate, but no pinInputs rule matches %s — a pull request editing it would be classified as touching no pin input", dir, probe)
		}
	}

	// Positive control on the probe itself: a package that is NOT in the graph
	// must not match, or the loop above is satisfied by a rule that matches
	// everything.
	if isPinInput("internal/cmd/x.go") {
		t.Fatal("isPinInput matches internal/cmd/x.go, a package pins-gate does not import — the coverage loop above would pass for any table")
	}
}

// ─── reading the pull request's diff, against real git ───────────────────────

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-C", dir,
		"-c", "user.name=pins-gate-test", "-c", "user.email=pins-gate-test@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main", "-c", "protocol.file.allow=always",
	}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// baseTree is a stand-in for main: enough paths that a PR can touch a pin input
// or avoid one.
var baseTree = map[string]string{
	"README.md":                            "readme\n",
	"internal/cmd/root.go":                 "package cmd\n",
	"internal/scaffold/pins.go":            "package scaffold\n",
	"internal/scaffold/templates/a.tmpl":   "a\n",
	".github/workflows/ci.yml":             "name: CI\n",
	"internal/scaffold/cmd/pins-gate/m.go": "package main\n",
}

// prCheckout builds what `actions/checkout` leaves for a pull_request event: a
// repo whose HEAD is the merge of a PR head into the base tip. It returns the
// repo and the PR head sha. mainMoved adds an unrelated commit to the base
// AFTER the PR branched, so the merge base and the base tip differ. fromFork
// makes the PR head arrive from a SEPARATE repository, the way a fork's does.
func prCheckout(t *testing.T, change func(dir string), mainMoved, fromFork bool) (repo, prHead string) {
	t.Helper()
	return prCheckoutIn(t, t.TempDir, change, mainMoved, fromFork)
}

func prCheckoutIn(t *testing.T, newDir func() string, change func(dir string), mainMoved, fromFork bool) (repo, prHead string) {
	t.Helper()
	repo = newDir()
	gitT(t, repo, "init", "-q")
	writeTree(t, repo, baseTree)
	gitT(t, repo, "add", "--", ".")
	gitT(t, repo, "commit", "-q", "-m", "base")

	work := repo
	if fromFork {
		work = newDir()
		gitT(t, work, "clone", "-q", "file://"+repo, ".")
	}
	gitT(t, work, "checkout", "-q", "-b", "pr")
	change(work)
	gitT(t, work, "add", "-A", "--", ".")
	gitT(t, work, "commit", "-q", "-m", "pr change")
	prHead = gitT(t, work, "rev-parse", "HEAD")

	if fromFork {
		gitT(t, repo, "fetch", "-q", "file://"+work, "pr")
	} else {
		gitT(t, repo, "checkout", "-q", "main")
	}
	if mainMoved {
		writeTree(t, repo, map[string]string{"internal/scaffold/pins.go": "package scaffold\n\n// moved on main, not by the PR\n"})
		gitT(t, repo, "add", "--", "internal/scaffold/pins.go")
		gitT(t, repo, "commit", "-q", "-m", "main moves")
	}
	gitT(t, repo, "merge", "-q", "--no-ff", "-m", "Merge pr into main", prHead)
	return repo, prHead
}

// The end-to-end cases below never write to a checkout, so the handful of
// distinct ones are built once and shared: each costs ~10 git invocations, and
// building one per row made this the slowest package in the module.
var shared = struct {
	sync.Mutex
	root string
	n    int
	byID map[string][2]string
}{byID: map[string][2]string{}}

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "pins-gate-fixtures-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	shared.root = root
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}

func sharedCheckout(t *testing.T, id string, change func(dir string), mainMoved, fromFork bool) (repo, prHead string) {
	t.Helper()
	shared.Lock()
	defer shared.Unlock()
	if v, ok := shared.byID[id]; ok {
		return v[0], v[1]
	}
	newDir := func() string {
		shared.n++
		d := filepath.Join(shared.root, fmt.Sprint(shared.n))
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	repo, prHead = prCheckoutIn(t, newDir, change, mainMoved, fromFork)
	shared.byID[id] = [2]string{repo, prHead}
	return repo, prHead
}

func edit(files map[string]string) func(string) {
	return func(dir string) {
		for rel, body := range files {
			p := filepath.Join(dir, rel)
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				panic(err)
			}
		}
	}
}

func TestChangedPathsReadsThePullRequestsOwnDiff(t *testing.T) {
	t.Run("lists exactly what the PR changed", func(t *testing.T) {
		repo, head := prCheckout(t, edit(map[string]string{"README.md": "changed\n", "docs/new.md": "new\n"}), false, false)
		got, err := changedPaths(repo, head)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"README.md", "docs/new.md"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("changedPaths = %q, want %q", got, want)
		}
	})

	// The base moved a PIN INPUT after the PR branched. That is main's change,
	// not the PR's, and must not be attributed to it — otherwise every PR
	// opened before a pin bump would gate until rebased, which is the symptom.
	t.Run("does not attribute the base's own commits to the PR", func(t *testing.T) {
		repo, head := prCheckout(t, edit(map[string]string{"README.md": "changed\n"}), true, false)
		got, err := changedPaths(repo, head)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"README.md"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("changedPaths = %q, want %q (internal/scaffold/pins.go moved on main, not in this PR)", got, want)
		}
	})

	// A file MOVED out of a pin input. With rename detection the diff would
	// name only the destination and the PR would read as untouched.
	t.Run("a rename lists the path it left as well as the one it reached", func(t *testing.T) {
		repo, head := prCheckout(t, func(dir string) {
			if err := os.Mkdir(filepath.Join(dir, "docs"), 0o755); err != nil {
				t.Fatal(err)
			}
			gitT(t, dir, "mv", "internal/scaffold/templates/a.tmpl", "docs/a.tmpl")
		}, false, false)
		got, err := changedPaths(repo, head)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"docs/a.tmpl", "internal/scaffold/templates/a.tmpl"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("changedPaths = %q, want %q", got, want)
		}
	})

	t.Run("a PR from a fork reads the same way", func(t *testing.T) {
		repo, head := prCheckout(t, edit(map[string]string{"internal/cmd/root.go": "package cmd // fork\n"}), false, true)
		got, err := changedPaths(repo, head)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"internal/cmd/root.go"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("changedPaths = %q, want %q", got, want)
		}
	})

	// A path with a newline in it must come back as ONE path. Without -z git
	// would quote it; split on newlines it would read as two harmless ones.
	t.Run("a path containing a newline stays one path", func(t *testing.T) {
		odd := "internal/scaffold/templates/odd\nname.tmpl"
		repo, head := prCheckout(t, edit(map[string]string{odd: "x\n"}), false, false)
		got, err := changedPaths(repo, head)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{odd}; !reflect.DeepEqual(got, want) {
			t.Fatalf("changedPaths = %q, want %q", got, want)
		}
	})
}

func TestChangedPathsRefusesWhenItCannotKnow(t *testing.T) {
	repo, head := prCheckout(t, edit(map[string]string{"README.md": "changed\n"}), false, false)

	// Positive control: this exact repo and head DO resolve, so each refusal
	// below is about the one thing that arm changes.
	if _, err := changedPaths(repo, head); err != nil {
		t.Fatalf("control: the unbroken checkout must resolve, got %v", err)
	}

	cases := []struct {
		name    string
		repo    func(t *testing.T) string
		head    string
		wantErr string // a substring of THIS refusal, not of any error
	}{
		{name: "missing head sha", repo: func(*testing.T) string { return repo }, head: "", wantErr: "no usable pull-request head sha"},
		{name: "abbreviated head sha", repo: func(*testing.T) string { return repo }, head: head[:12], wantErr: "no usable pull-request head sha"},
		{name: "a ref name instead of a sha", repo: func(*testing.T) string { return repo }, head: "HEAD^2", wantErr: "no usable pull-request head sha"},
		{name: "a sha that is not this merge's second parent", repo: func(*testing.T) string { return repo },
			head: strings.Repeat("a", 40), wantErr: "is not that pull request's merge commit"},
		// What a `push` checkout, or a PR checkout by head ref, looks like.
		{name: "HEAD is not a merge commit", repo: func(t *testing.T) string {
			d := t.TempDir()
			gitT(t, d, "clone", "-q", "file://"+repo, ".")
			gitT(t, d, "checkout", "-q", head)
			return d
		}, head: head, wantErr: "want the 2 of a pull-request merge commit"},
		// 🔴 The default `actions/checkout` depth. The merge commit is there
		// and its parents are not, which is the "missing base sha" case.
		{name: "depth-1 checkout hides the base", repo: func(t *testing.T) string {
			d := t.TempDir()
			gitT(t, d, "clone", "-q", "--depth", "1", "file://"+repo, ".")
			return d
		}, head: head, wantErr: "HEAD has 0 visible parent(s)"},
		{name: "not a git repository", repo: func(t *testing.T) string { return t.TempDir() }, head: head, wantErr: "git rev-list"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := changedPaths(c.repo(t), c.head)
			if err == nil {
				t.Fatalf("changedPaths returned %q with no error. Not knowing what a PR changed must be an ERROR — a nil error here is read as a real diff, and an empty or partial one downgrades a run that should gate.", got)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("refused for the wrong reason.\n  got:  %v\n  want: an error containing %q", err, c.wantErr)
			}
			if ctx := classify("pull_request", got, err); ctx.Mode != modeGate {
				t.Fatalf("classify turned this refusal into mode %q; it must gate", ctx.Mode)
			}
		})
	}

	// The depth the job actually asks for must be ENOUGH — otherwise the arm
	// above proves only that shallow clones fail, not that fetch-depth: 2 works.
	t.Run("depth-2 checkout is enough", func(t *testing.T) {
		d := t.TempDir()
		gitT(t, d, "clone", "-q", "--depth", "2", "file://"+repo, ".")
		got, err := changedPaths(d, head)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"README.md"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("changedPaths at depth 2 = %q, want %q", got, want)
		}
	})
}

// ─── retry ───────────────────────────────────────────────────────────────────

func TestWithRetry(t *testing.T) {
	transient := errors.New("GET …: 503 Service Unavailable")

	t.Run("a transient failure is retried until it answers", func(t *testing.T) {
		calls, slept := 0, 0
		f := withRetry(func(string) (string, error) {
			calls++
			if calls < 3 {
				return "", transient
			}
			return "1.2.3", nil
		}, 3, func(time.Duration) { slept++ })
		v, err := f("@civitai/x")
		if err != nil || v != "1.2.3" || calls != 3 || slept != 2 {
			t.Fatalf("got (%q, %v) after %d call(s) and %d sleep(s); want (\"1.2.3\", nil) after 3 calls and 2 sleeps", v, err, calls, slept)
		}
	})

	t.Run("it gives up, and says how hard it tried", func(t *testing.T) {
		calls := 0
		f := withRetry(func(string) (string, error) { calls++; return "", transient }, 3, func(time.Duration) {})
		_, err := f("@civitai/x")
		if calls != 3 || err == nil || !errors.Is(err, transient) || !strings.Contains(err.Error(), "after 3 attempts") {
			t.Fatalf("got %v after %d call(s); want the transient error wrapped with \"after 3 attempts\" after 3 calls", err, calls)
		}
	})

	// A 404 is an ANSWER. Retrying it would be slow; turning it into
	// "unreachable" would downgrade a broken pin on an untouched PR.
	t.Run("not-found is an answer and is neither retried nor reworded", func(t *testing.T) {
		calls := 0
		nf := fmt.Errorf("GET …: 404 Not Found: %w", scaffold.ErrPkgNotFound)
		f := withRetry(func(string) (string, error) { calls++; return "", nf }, 3, func(time.Duration) { t.Fatal("slept before a not-found") })
		_, err := f("@civitai/x")
		if calls != 1 || err != nf {
			t.Fatalf("got %v after %d call(s); want the not-found error itself after exactly 1 call", err, calls)
		}
	})
}

// ─── end to end, through run() ───────────────────────────────────────────────

// registry is a local stand-in for npm. It answers `GET /<pkg>/latest` from
// behave, and counts the requests it sees — the positive control that a case
// asserting "nothing blocked" actually asked something.
type registry struct {
	*httptest.Server
	hits int
}

func newRegistry(t *testing.T, behave func(pkg string) (status int, body string)) *registry {
	t.Helper()
	r := &registry{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits++
		pkg := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/"), "/latest")
		status, body := behave(pkg)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(r.Close)
	return r
}

// currentPins reads the pins the templates carry RIGHT NOW, so a fixture can be
// expressed relative to them ("exactly the pinned version", "one major above")
// and the cases below hold whatever the bump bot last wrote.
func currentPins(t *testing.T) map[string]string {
	t.Helper()
	pins, err := scaffold.TemplatePins()
	if err != nil || len(pins) == 0 {
		t.Fatalf("TemplatePins: %v (%d pins) — the fixtures below are relative to the real pins and mean nothing without them", err, len(pins))
	}
	out := map[string]string{}
	for _, p := range pins {
		out[p.Pkg] = strings.TrimPrefix(p.Pin, "^")
	}
	return out
}

func versionBody(v string) string { return `{"version":"` + v + `"}` }

// The registry fixtures. stale publishes a version no pre-existing pin can
// admit: 999.0.0 is outside every caret range a real pin could have.
func regFresh(pins map[string]string) func(string) (int, string) {
	return func(pkg string) (int, string) { return 200, versionBody(pins[pkg]) }
}
func regStale(string) (int, string)    { return 200, versionBody("999.0.0") }
func regDown(string) (int, string)     { return 503, "upstream connect error" }
func regNotFound(string) (int, string) { return 404, `{"error":"Not found"}` }
func regGarbage(string) (int, string)  { return 200, versionBody("latest-and-greatest") }

type gateRun struct {
	code    int
	stdout  string
	stderr  string
	summary string
	output  string
	hits    int
}

func runGate(t *testing.T, reg *registry, args ...string) gateRun {
	t.Helper()
	tmp := t.TempDir()
	sum, outF := filepath.Join(tmp, "summary.md"), filepath.Join(tmp, "output.txt")
	env := map[string]string{"GITHUB_STEP_SUMMARY": sum, "GITHUB_OUTPUT": outF}
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"-registry", reg.URL}, args...), func(k string) string { return env[k] }, &stdout, &stderr, func(time.Duration) {})
	s, _ := os.ReadFile(sum)
	o, _ := os.ReadFile(outF)
	return gateRun{code: code, stdout: stdout.String(), stderr: stderr.String(), summary: string(s), output: string(o), hits: reg.hits}
}

// lastLine is the verdict line the workflow's positive control requires.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// annotations returns the workflow-command lines of one level.
func annotations(stdout, level string) []string {
	var out []string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(l, "::"+level+" ") {
			out = append(out, l)
		}
	}
	return out
}

// TestRedWhereItMustBeGreenWhereItMustBe is the original symptom, reproduced
// through the command's real entry point with a real git checkout and a local
// registry: npm has published past the pins. A PR that touched no pin input
// must come back GREEN with a warning; a PR that touched one, and a push to
// main, must come back RED.
//
// Every row is literal: the exit code, the verdict line, the annotation level
// and title, and the summary heading a human is told to look for.
func TestRedWhereItMustBeGreenWhereItMustBe(t *testing.T) {
	pins := currentPins(t)
	all, _ := scaffold.TemplatePins()
	nPins := len(all)

	untouched := edit(map[string]string{"README.md": "docs only\n", "internal/cmd/root.go": "package cmd // edited\n"})
	touched := edit(map[string]string{"README.md": "docs\n", "internal/scaffold/templates/a.tmpl": "edited template\n"})

	type ctxFn func(t *testing.T) []string
	pr := func(id string, change func(string), mainMoved, fork bool) ctxFn {
		return func(t *testing.T) []string {
			repo, head := sharedCheckout(t, id, change, mainMoved, fork)
			return []string{"-event", "pull_request", "-repo", repo, "-pr-head", head}
		}
	}
	event := func(name string) ctxFn {
		return func(t *testing.T) []string {
			// A real PR-shaped checkout of an UNTOUCHED change, so the only
			// thing making these rows gate is the event name.
			repo, head := sharedCheckout(t, "untouched", untouched, false, false)
			return []string{"-event", name, "-repo", repo, "-pr-head", head}
		}
	}
	contexts := map[string]ctxFn{
		"untouched PR":                   pr("untouched", untouched, false, false),
		"untouched PR, main moved a pin": pr("untouched-main-moved", untouched, true, false),
		"untouched PR from a fork":       pr("untouched-fork", untouched, false, true),
		"touched PR":                     pr("touched", touched, false, false),
		"touched PR from a fork":         pr("touched-fork", touched, false, true),
		"push to main":                   event("push"),
		"schedule":                       event("schedule"),
		"workflow_dispatch":              event("workflow_dispatch"),
		"untouched PR, head sha missing": func(t *testing.T) []string {
			r, _ := sharedCheckout(t, "untouched", untouched, false, false)
			return []string{"-event", "pull_request", "-repo", r}
		},
		"untouched PR, depth-1 checkout": func(t *testing.T) []string {
			r, head := sharedCheckout(t, "untouched", untouched, false, false)
			d := t.TempDir()
			gitT(t, d, "clone", "-q", "--depth", "1", "file://"+r, ".")
			return []string{"-event", "pull_request", "-repo", d, "-pr-head", head}
		},
	}

	registries := map[string]func(string) (int, string){
		"fresh": regFresh(pins), "stale": regStale, "down": regDown, "notfound": regNotFound, "garbage": regGarbage,
	}

	const (
		advise = "advise"
		gate   = "gate"
	)
	cases := []struct {
		ctx, reg string
		code     int
		mode     string
		verdict  string
		level    string // "" = no annotation at all
		title    string
		head     string
	}{
		// ── the symptom: npm published past the pins ──
		{"untouched PR", "stale", 0, advise, "stale", "warning", titleAdvisedStale, headAdvisedStale},
		{"untouched PR, main moved a pin", "stale", 0, advise, "stale", "warning", titleAdvisedStale, headAdvisedStale},
		{"untouched PR from a fork", "stale", 0, advise, "stale", "warning", titleAdvisedStale, headAdvisedStale},
		{"touched PR", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},
		{"touched PR from a fork", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},
		{"push to main", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},
		{"schedule", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},
		{"workflow_dispatch", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},
		// Not knowing what the PR changed gates, even though it changed nothing
		// that matters.
		{"untouched PR, head sha missing", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},
		{"untouched PR, depth-1 checkout", "stale", 1, gate, "stale", "error", titleBlockedStale, headBlockedStale},

		// ── the registry could not be asked: its own verdict, never "stale" ──
		{"untouched PR", "down", 0, advise, "unreachable", "warning", titleAdvisedUnreached, headAdvisedUnreached},
		{"untouched PR from a fork", "down", 0, advise, "unreachable", "warning", titleAdvisedUnreached, headAdvisedUnreached},
		{"touched PR", "down", 1, gate, "unreachable", "error", titleBlockedUnreached, headBlockedUnreached},
		{"push to main", "down", 1, gate, "unreachable", "error", titleBlockedUnreached, headBlockedUnreached},

		// ── the tree is wrong: never downgraded, in any context ──
		{"untouched PR", "notfound", 1, advise, "broken", "error", titleBroken, headBlockedBroken},
		{"touched PR", "notfound", 1, gate, "broken", "error", titleBroken, headBlockedBroken},
		{"push to main", "notfound", 1, gate, "broken", "error", titleBroken, headBlockedBroken},
		{"untouched PR", "garbage", 1, advise, "broken", "error", titleBroken, headBlockedBroken},
		{"push to main", "garbage", 1, gate, "broken", "error", titleBroken, headBlockedBroken},

		// ── nothing wrong: green everywhere, and silent ──
		{"untouched PR", "fresh", 0, advise, "ok", "", "", headOK},
		{"touched PR", "fresh", 0, gate, "ok", "", "", headOK},
		{"push to main", "fresh", 0, gate, "ok", "", "", headOK},
		{"untouched PR, depth-1 checkout", "fresh", 0, gate, "ok", "", "", headOK},
	}

	for _, c := range cases {
		t.Run(c.ctx+" / npm "+c.reg, func(t *testing.T) {
			mk, ok := contexts[c.ctx]
			if !ok {
				t.Fatalf("no such context %q", c.ctx)
			}
			beh, ok := registries[c.reg]
			if !ok {
				t.Fatalf("no such registry fixture %q", c.reg)
			}
			reg := newRegistry(t, beh)
			got := runGate(t, reg, mk(t)...)

			if got.hits == 0 {
				t.Fatalf("the local registry received no request — this case asserted nothing about a pin verdict.\nstdout:\n%s\nstderr:\n%s", got.stdout, got.stderr)
			}
			if got.code != c.code {
				t.Errorf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", got.code, c.code, got.stdout, got.stderr)
			}

			blocking := "0"
			if c.code != 0 {
				blocking = fmt.Sprint(nPins)
			}
			wantLast := fmt.Sprintf("pins-gate: mode=%s verdict=%s pins=%d blocking=%s exit=%d", c.mode, c.verdict, nPins, blocking, c.code)
			if l := lastLine(got.stdout); l != wantLast {
				t.Errorf("verdict line\n  got:  %s\n  want: %s", l, wantLast)
			}

			other := map[string]string{"warning": "error", "error": "warning", "": ""}[c.level]
			if c.level == "" {
				if n := len(annotations(got.stdout, "warning")) + len(annotations(got.stdout, "error")); n != 0 {
					t.Errorf("%d annotation(s) on a run with nothing to report:\n%s", n, got.stdout)
				}
			} else {
				mine := annotations(got.stdout, c.level)
				if len(mine) == 0 {
					t.Errorf("no ::%s annotation. A downgrade nobody is told about is a silent pass.\nstdout:\n%s", c.level, got.stdout)
				}
				for _, l := range mine {
					if !strings.HasPrefix(l, "::"+c.level+" title="+c.title+"::") {
						t.Errorf("::%s annotation carries the wrong title — the title is what tells \"stale\" from \"not checked\" from \"invalid\".\n  got:  %s\n  want title: %s", c.level, l, c.title)
					}
				}
				if theirs := annotations(got.stdout, other); len(theirs) != 0 {
					t.Errorf("unexpected ::%s annotation(s) on a run that should only emit ::%s:\n%s", other, c.level, strings.Join(theirs, "\n"))
				}
			}

			if !strings.HasPrefix(got.summary, c.head+"\n") {
				t.Errorf("job summary heading\n  got:  %s\n  want: %s", strings.SplitN(got.summary, "\n", 2)[0], c.head)
			}
			if want := fmt.Sprintf("mode=%s\nverdict=%s\n", c.mode, c.verdict); got.output != want {
				t.Errorf("step outputs = %q, want %q", got.output, want)
			}
		})
	}
}

// The warning has to name the stale pin AND the fix, or "reported" is a colour.
func TestTheAdvisoryNamesTheStalePinAndTheBotPR(t *testing.T) {
	pins := currentPins(t)
	repo, head := sharedCheckout(t, "docs-only", edit(map[string]string{"README.md": "docs only\n"}), false, false)
	args := []string{"-event", "pull_request", "-repo", repo, "-pr-head", head}

	t.Run("with an open bot PR", func(t *testing.T) {
		const bot = "https://github.com/civitai/cli/pull/808"
		got := runGate(t, newRegistry(t, regStale), append(args, "-bot-pr", bot)...)
		if got.code != 0 {
			t.Fatalf("exit %d, want 0\n%s", got.code, got.stdout)
		}
		for pkg, ver := range pins {
			want := fmt.Sprintf("%s is pinned ^%s in templates/", pkg, ver)
			if !strings.Contains(got.stdout, want) {
				t.Errorf("no annotation names the stale pin %q:\n%s", want, got.stdout)
			}
			if row := fmt.Sprintf("| `%s` | `^%s` | `999.0.0` | stale | no |", pkg, ver); !strings.Contains(got.summary, row) {
				t.Errorf("the job summary has no row ending %q:\n%s", row, got.summary)
			}
		}
		if !strings.Contains(got.stdout, "does NOT admit npm's published 999.0.0") {
			t.Errorf("no annotation names the published version:\n%s", got.stdout)
		}
		for where, text := range map[string]string{"annotation": got.stdout, "summary": got.summary} {
			if !strings.Contains(text, "The fix is the bump bot's pull request: "+bot) {
				t.Errorf("the %s does not name the bot PR %s:\n%s", where, bot, text)
			}
		}
	})

	t.Run("with no open bot PR it says so, and says what to do", func(t *testing.T) {
		got := runGate(t, newRegistry(t, regStale), args...)
		want := "No open `bump-scaffold-pins` pull request was found. That workflow opens one on its next sweep; to fix it by hand run `go run ./internal/scaffold/cmd/bump-pins`."
		if !strings.Contains(got.summary, want) || !strings.Contains(got.stdout, want) {
			t.Errorf("want this sentence in both the annotation and the summary:\n  %s\nstdout:\n%s\nsummary:\n%s", want, got.stdout, got.summary)
		}
	})

	// The URL arrives from a `gh pr list` over branch names. Anything that is
	// not a plain pull-request URL is dropped, not printed.
	t.Run("a bot-pr value that is not a pull-request URL is not printed", func(t *testing.T) {
		for _, evil := range []string{
			"https://github.com/civitai/cli/pull/808\n::error title=x::owned",
			"https://evil.example/civitai/cli/pull/808",
			"javascript:alert(1)",
			"https://github.com/civitai/cli/pull/808?x=](https://evil.example)",
		} {
			got := runGate(t, newRegistry(t, regStale), append(args, "-bot-pr", evil)...)
			if strings.Contains(got.stdout+got.summary, "evil.example") || strings.Contains(got.stdout+got.summary, "javascript:") || strings.Contains(got.stdout+got.summary, "owned") {
				t.Errorf("an untrusted -bot-pr value reached the report: %q\nstdout:\n%s\nsummary:\n%s", evil, got.stdout, got.summary)
			}
			if !strings.Contains(got.summary, "No open `bump-scaffold-pins` pull request was found") {
				t.Errorf("-bot-pr %q was not treated as absent:\n%s", evil, got.summary)
			}
		}
	})
}

// A touched PR's report must say WHICH input it touched: "this run gates" with
// no reason is the complaint this command exists to end.
func TestAGatingReportSaysWhy(t *testing.T) {
	repo, head := prCheckout(t, edit(map[string]string{"internal/scaffold/templates/a.tmpl": "edited\n", "go.mod": "module x\n"}), false, false)
	got := runGate(t, newRegistry(t, regStale), "-event", "pull_request", "-repo", repo, "-pr-head", head)
	if got.code != 1 {
		t.Fatalf("exit %d, want 1\n%s", got.code, got.stdout)
	}
	for _, want := range []string{
		"pins-gate: mode=gate — this pull request changes a pin input",
		"pins-gate:   pin input changed: go.mod",
		"pins-gate:   pin input changed: internal/scaffold/templates/a.tmpl",
	} {
		if !strings.Contains(got.stdout, want+"\n") {
			t.Errorf("stdout is missing the line %q:\n%s", want, got.stdout)
		}
	}
	for _, want := range []string{"- `go.mod`\n", "- `internal/scaffold/templates/a.tmpl`\n", "**Mode: `gate`** — this pull request changes a pin input."} {
		if !strings.Contains(got.summary, want) {
			t.Errorf("summary is missing %q:\n%s", want, got.summary)
		}
	}
}

// A mixed result must not let the milder state hide the worse one, in either
// direction: one package unreachable and another stale.
func TestMixedFindingsAreEachReported(t *testing.T) {
	pins := currentPins(t)
	if len(pins) < 2 {
		t.Skipf("needs two distinct pinned packages, the templates pin %d", len(pins))
	}
	var pkgs []string
	for p := range pins {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	downPkg := pkgs[0]
	beh := func(pkg string) (int, string) {
		if pkg == downPkg {
			return 503, "down"
		}
		return 200, versionBody("999.0.0")
	}
	repo, head := sharedCheckout(t, "docs-only", edit(map[string]string{"README.md": "docs only\n"}), false, false)
	got := runGate(t, newRegistry(t, beh), "-event", "pull_request", "-repo", repo, "-pr-head", head)

	if got.code != 0 {
		t.Fatalf("exit %d, want 0 (both states are external on an untouched PR)\n%s", got.code, got.stdout)
	}
	var sawStale, sawUnreached bool
	for _, l := range annotations(got.stdout, "warning") {
		sawStale = sawStale || strings.HasPrefix(l, "::warning title="+titleAdvisedStale+"::")
		sawUnreached = sawUnreached || (strings.HasPrefix(l, "::warning title="+titleAdvisedUnreached+"::") && strings.Contains(l, downPkg))
	}
	if !sawStale || !sawUnreached {
		t.Errorf("want one warning per state (stale=%v, unreachable-naming-%s=%v):\n%s", sawStale, downPkg, sawUnreached, got.stdout)
	}
	// The retry is real: the unreachable package was asked fetchAttempts times.
	if !strings.Contains(got.stdout, fmt.Sprintf("after %d attempts", fetchAttempts)) {
		t.Errorf("the unreachable annotation does not say how hard it tried:\n%s", got.stdout)
	}
}

// What the registry sends back is untrusted, and it lands inside a workflow
// command. A version string must not be able to start a second annotation.
func TestARegistryReplyCannotForgeAnAnnotation(t *testing.T) {
	// parseSemver drops everything from the first `-`, so this parses as 999.0.0
	// (stale) while carrying a newline and a forged command in its tail.
	evil := `999.0.0-x\n::error title=forged::owned\n::warning::also`
	beh := func(string) (int, string) { return 200, `{"version":"` + evil + `"}` }
	repo, head := sharedCheckout(t, "docs-only", edit(map[string]string{"README.md": "docs only\n"}), false, false)
	got := runGate(t, newRegistry(t, beh), "-event", "pull_request", "-repo", repo, "-pr-head", head)

	if got.code != 0 || !strings.Contains(lastLine(got.stdout), " verdict=stale ") || !strings.HasSuffix(lastLine(got.stdout), " blocking=0 exit=0") {
		t.Fatalf("control: the forged version must still classify as stale on an untouched PR.\nexit %d\n%s", got.code, got.stdout)
	}
	if n := len(annotations(got.stdout, "error")); n != 0 {
		t.Errorf("the registry's reply produced %d ::error line(s):\n%s", n, got.stdout)
	}
	for _, l := range strings.Split(got.stdout, "\n") {
		if strings.HasPrefix(l, "::") && !strings.HasPrefix(l, "::warning title="+titleAdvisedStale+"::") {
			t.Errorf("a workflow-command line this command did not mean to write: %q", l)
		}
	}
	if strings.Contains(got.summary, "\n::") {
		t.Errorf("the forged text reached the summary on its own line:\n%s", got.summary)
	}
}

// Exit 2 — the gate could not do its job — must not be reachable as a green.
func TestUsageErrorsFail(t *testing.T) {
	reg := newRegistry(t, regStale)
	for _, args := range [][]string{
		{"-no-such-flag"},
		{"-event", "pull_request", "stray-positional"},
	} {
		var out, errb bytes.Buffer
		if code := run(append([]string{"-registry", reg.URL}, args...), func(string) string { return "" }, &out, &errb, func(time.Duration) {}); code != exitBroken {
			t.Errorf("run(%q) = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, exitBroken, out.String(), errb.String())
		}
	}
}

// The run's REASON reaches a gating annotation verbatim, and a diff error's
// reason carries git's stderr, which can be several lines. Every free-text
// field of a workflow command must be escaped, not only the registry's.
func TestAMultiLineReasonCannotStartASecondAnnotation(t *testing.T) {
	ctx := classify("pull_request", nil, errors.New("git diff: exit status 128: fatal: one\n::error title=forged::owned\nthree"))
	findings := []scaffold.PinFinding{{
		TemplatePin: scaffold.TemplatePin{File: "templates/x/package.json.tmpl", Pkg: "@civitai/x", Pin: "^0.1.0"},
		Published:   "0.2.0", Status: scaffold.PinStale,
	}}
	rep := buildReport(ctx, findings, "")
	lines := strings.Split(strings.TrimRight(rep.log, "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "::error title="+titleBlockedStale+"::") {
		t.Fatalf("want exactly one ::error line, got %d:\n%s", len(lines), rep.log)
	}
	if !strings.Contains(lines[0], "fatal: one%0A::error title=forged::owned%0Athree") {
		t.Errorf("the reason's newlines were not escaped as %%0A:\n%s", lines[0])
	}
}
