// Command pins-gate is what the `pins-vs-published` CI job runs.
//
// It answers TWO questions and keeps them apart, because conflating them is what
// this command replaced:
//
//  1. Do the scaffold templates' `@civitai/*` pins still admit what npm
//     publishes as `latest`? (scaffold.CheckPins — the same code the guard test
//     and the bump bot use.)
//  2. If they do not, is that THIS RUN'S problem?
//
// # Why question 2 exists
//
// The job is a REQUIRED status check, and the thing it compares against is npm,
// which moves on its own schedule. So the old job — a bare
// `go test -run TestScaffoldPinsSatisfyPublished` — went red on every open PR
// the moment a `@civitai/*` minor was published, whatever the PR contained, and
// stayed red until the bump bot's PR merged AND the author pushed again. A
// docs-only PR was blocked by a version number on a registry.
//
// The rule now: staleness blocks the runs that could have CAUSED it or are
// responsible for FIXING it, and is reported everywhere else.
//
//   - A pull request whose diff touches no PIN INPUT (see pinInputs) cannot have
//     made a pin stale, and merging it cannot make one staler. Staleness there
//     is a warning annotation plus a job summary, and the job succeeds.
//   - A pull request that DOES touch a pin input gates exactly as before.
//   - Every other event — push to main, workflow_dispatch, schedule, anything
//     this command does not recognise — gates. `main` stays loud.
//
// # 🔴 It fails CLOSED, toward gating
//
// "This PR did not touch a pin input" is a claim, and the downgrade is only as
// good as the claim. Every way of NOT knowing resolves to gate: a missing or
// malformed PR head sha, a checkout that is not the PR's merge commit, a shallow
// clone without the base, a failing `git diff`, an empty diff. None of those is
// evidence that a PR is harmless.
//
// # The three reasons the job can be unhappy, which must not be collapsed
//
//	stale        npm moved past a pin. External. Downgraded on an untouched PR.
//	unreachable  npm could not be asked, after retries. NOTHING was checked.
//	             Downgraded on an untouched PR, but reported under its own title
//	             — never as "stale", and never as a pass. It FAILS a gating run:
//	             a run that gates is the only thing standing between a changed
//	             pin and main, and a green there would be a claim about nothing.
//	broken       a pin names a package npm does not publish, or cannot be
//	             compared at all, or the templates hold no pins. The TREE is
//	             wrong. Never downgraded: on a PR that touched no pin input this
//	             means either main is broken or the classifier is — and neither
//	             is something to wave through.
//
// Usage (the CI job):
//
//	go run ./internal/scaffold/cmd/pins-gate \
//	    -event "$GITHUB_EVENT_NAME" -pr-head "$PR_HEAD_SHA" -bot-pr "$BOT_PR_URL"
//
// Exit 0: nothing blocks this run (read the annotations — 0 is not "pins are
// current"). Exit 1: a pin verdict blocks this run. Exit 2: the gate itself
// could not do its job.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/civitai/cli/internal/scaffold"
)

// ─── what counts as a pin input ──────────────────────────────────────────────

// pinInput is one path rule. Exactly one of prefix/exact is set.
type pinInput struct {
	prefix string // matches any path under this directory (must end in "/")
	exact  string // matches this one path
	why    string
}

// pinInputs is everything the verdict of this command is a function of, other
// than npm. A pull request that changes none of these cannot have changed
// whether a pin is stale.
//
// 🔴 IT IS DERIVED FROM WHAT THE CHECK READS, NOT FROM WHERE PINS "LIVE", and
// TestPinInputsCoverTheBuildGraph holds it to that: the in-module packages this
// command is built from (per `go list -deps`) must be exactly the set named
// here, so a new import — or the template tree moving to another package — reds
// the suite until this table is taught about it. Embedded files need no row of
// their own: `go:embed` cannot reach outside the embedding package's directory,
// so a directory prefix covers every file a package can embed.
//
// Deliberately WIDE. `internal/scaffold/` also holds tests and unrelated
// scaffold code that cannot move a pin; they gate anyway. Carving the directory
// up file by file would make every new file in it a question, and the cost of
// being wrong in the narrow direction is a stale pin merged green.
var pinInputs = []pinInput{
	{prefix: "internal/scaffold/", why: "the embedded templates (`go:embed all:templates`), the pin logic in pins.go, and this command"},
	{prefix: "internal/blockproto/", why: "imported by package scaffold, so it is compiled into this command"},
	{exact: "go.mod", why: "the toolchain and module set this command is built with"},
	{exact: "go.sum", why: "the toolchain and module set this command is built with"},
	{exact: "go.work", why: "a workspace file would change which module sources this command is built from"},
	{exact: "go.work.sum", why: "a workspace file would change which module sources this command is built from"},
	{prefix: "vendor/", why: "a vendor tree would replace the module sources this command is built from"},
	{exact: ".github/workflows/ci.yml", why: "the job that runs this command, and the arguments it is given"},
}

// strayPinFile is the basename of the one file kind a pin is read from. A file
// with this name OUTSIDE the known tree is "a path that could contain a pin"
// which nothing above classifies, so it gates.
const strayPinFile = "package.json.tmpl"

// touchedInputs returns the changed paths that are pin inputs, in input order.
func touchedInputs(changed []string) []string {
	var out []string
	for _, p := range changed {
		if isPinInput(p) {
			out = append(out, p)
		}
	}
	return out
}

func isPinInput(p string) bool {
	for _, in := range pinInputs {
		if in.exact != "" && p == in.exact {
			return true
		}
		if in.prefix != "" && strings.HasPrefix(p, in.prefix) {
			return true
		}
	}
	return path.Base(p) == strayPinFile
}

// ─── which runs gate ─────────────────────────────────────────────────────────

type mode string

const (
	// modeGate: a non-OK pin verdict fails the job. The pre-existing behaviour.
	modeGate mode = "gate"
	// modeAdvise: staleness and unreachability are reported, not enforced.
	modeAdvise mode = "advise"
)

// prEvent is the ONE event that can be downgraded. Everything else gates,
// including events this command has never heard of — and including
// `pull_request_target`, whose checkout is the BASE branch, so a diff taken
// there says nothing about the pull request.
const prEvent = "pull_request"

type runContext struct {
	Mode    mode
	Reason  string   // one sentence, safe to print
	Touched []string // the pin inputs the PR changed (gate-by-diff only)
	Changed int      // how many paths the PR changed (0 when unknown)
}

// classify decides whether this run gates. It is the single place that decision
// is made. changed/diffErr are only consulted for a pull_request event.
func classify(event string, changed []string, diffErr error) runContext {
	if event != prEvent {
		name := event
		if name == "" {
			name = "(no event name)"
		}
		return runContext{Mode: modeGate, Reason: fmt.Sprintf("event %q always gates — only a %s that touches no pin input is downgraded", name, prEvent)}
	}
	if diffErr != nil {
		return runContext{Mode: modeGate, Reason: fmt.Sprintf("could not determine what this pull request changed (%v) — an unknown diff is not an untouched one", diffErr)}
	}
	if len(changed) == 0 {
		return runContext{Mode: modeGate, Reason: "the pull request's diff came back empty — that is not evidence it touched nothing"}
	}
	if touched := touchedInputs(changed); len(touched) > 0 {
		return runContext{Mode: modeGate, Reason: "this pull request changes a pin input", Touched: touched, Changed: len(changed)}
	}
	return runContext{Mode: modeAdvise, Reason: fmt.Sprintf("this pull request changes %d path(s), none of them a pin input", len(changed)), Changed: len(changed)}
}

// blocks is the WHOLE behaviour matrix: does a pin in this state fail a run in
// this mode? Read it as a table — it is asserted as one.
func blocks(m mode, s scaffold.PinStatus) bool {
	switch s {
	case scaffold.PinOK:
		return false
	case scaffold.PinStale, scaffold.PinUnreachable:
		// The two EXTERNAL states. npm moved, or npm could not be asked; neither
		// is something a run that touched no pin input did.
		return m != modeAdvise
	default:
		// PinNotFound, PinInvalid, and any status added later: the tree is
		// wrong. Never downgraded, in any mode.
		return true
	}
}

// ─── what the pull request changed ───────────────────────────────────────────

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// changedPaths lists the paths a pull request changes, read off the merge commit
// `actions/checkout` leaves at HEAD for a pull_request event.
//
// HEAD there is GitHub's test-merge of the PR into its base: first parent the
// base tip, second parent the PR head. `diff HEAD^1 HEAD` is therefore exactly
// "what merging this PR would change" — which is the question, and it needs no
// API call, no token, and behaves identically for a PR from a fork.
//
// Every assumption in that sentence is CHECKED rather than trusted, and each
// failure is an error (which classify turns into gate):
//
//   - prHead must be a full sha — an empty expansion is how a renamed context
//     field would arrive;
//   - HEAD must have exactly two parents — a depth-1 checkout has none visible,
//     and a non-PR checkout is not a merge;
//   - the second parent must BE prHead — otherwise HEAD is some other merge and
//     its diff describes something else.
//
// `--no-renames` so a file MOVED out of a pin input still lists its old path.
func changedPaths(repo, prHead string) ([]string, error) {
	if !shaRe.MatchString(prHead) {
		return nil, fmt.Errorf("no usable pull-request head sha (got %q)", prHead)
	}
	out, err := git(repo, "rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		return nil, err
	}
	f := strings.Fields(string(out))
	if len(f) != 3 {
		return nil, fmt.Errorf("HEAD has %d visible parent(s), want the 2 of a pull-request merge commit (a depth-1 checkout hides them; the job needs fetch-depth: 2)", max(len(f)-1, 0))
	}
	base, second := f[1], f[2]
	if second != prHead {
		return nil, fmt.Errorf("HEAD's second parent is %s, not the pull-request head %s — this checkout is not that pull request's merge commit", second, prHead)
	}
	out, err = git(repo, "diff", "--name-only", "-z", "--no-renames", base, "HEAD")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	return paths, nil
}

func git(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ─── asking npm ──────────────────────────────────────────────────────────────

// fetchAttempts bounds how hard "unreachable" is tried before it is believed.
// It matters more than it used to: unreachable FAILS a gating run, so a single
// dropped connection must not be enough to red main.
const fetchAttempts = 3

// withRetry retries a transient registry failure. A not-found is an ANSWER, not
// a failure to get one, and is returned at once.
func withRetry(fetch func(string) (string, error), attempts int, sleep func(time.Duration)) func(string) (string, error) {
	return func(pkg string) (string, error) {
		var v string
		var err error
		for i := 1; i <= attempts; i++ {
			v, err = fetch(pkg)
			if err == nil || errors.Is(err, scaffold.ErrPkgNotFound) {
				return v, err
			}
			if i < attempts {
				sleep(time.Duration(i) * 2 * time.Second)
			}
		}
		return v, fmt.Errorf("after %d attempts: %w", attempts, err)
	}
}

// ─── entry point ─────────────────────────────────────────────────────────────

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr, time.Sleep))
}

const (
	exitOK      = 0
	exitBlocked = 1
	exitBroken  = 2
)

func run(args []string, getenv func(string) string, stdout, stderr io.Writer, sleep func(time.Duration)) int {
	fs := flag.NewFlagSet("pins-gate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	event := fs.String("event", "", "the triggering event name ($GITHUB_EVENT_NAME). Anything but pull_request gates.")
	prHead := fs.String("pr-head", "", "the pull request's head sha (github.event.pull_request.head.sha). Empty or wrong gates.")
	repo := fs.String("repo", ".", "the checked-out repository to read the pull request's diff from")
	registry := fs.String("registry", "https://registry.npmjs.org", "npm registry root")
	botPR := fs.String("bot-pr", "", "URL of the open bump-scaffold-pins pull request, if there is one (named in the report)")
	if err := fs.Parse(args); err != nil {
		return exitBroken
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "pins-gate: unexpected argument(s): %q\n", fs.Args())
		return exitBroken
	}

	// Which runs gate. The diff is only taken for the one event it can matter to.
	var changed []string
	var diffErr error
	if *event == prEvent {
		changed, diffErr = changedPaths(*repo, *prHead)
	}
	ctx := classify(*event, changed, diffErr)
	fmt.Fprintf(stdout, "pins-gate: mode=%s — %s\n", ctx.Mode, oneLine(ctx.Reason))
	for _, p := range ctx.Touched {
		fmt.Fprintf(stdout, "pins-gate:   pin input changed: %s\n", oneLine(p))
	}

	pins, err := scaffold.TemplatePins()
	if err == nil && len(pins) == 0 {
		err = errors.New("no @civitai/* pins found in any template package.json.tmpl — the extractor is broken, and a clean verdict over zero pins would be a statement about nothing")
	}
	if err != nil {
		fmt.Fprintf(stdout, "::error title=pins-vs-published: the check could not run::%s\n", cmdData(err.Error()))
		fmt.Fprintf(stdout, "pins-gate: mode=%s verdict=broken pins=0 blocking=0 exit=%d\n", ctx.Mode, exitBroken)
		return exitBroken
	}

	base := *registry
	fetch := withRetry(func(pkg string) (string, error) { return scaffold.FetchNpmLatestFrom(base, pkg) }, fetchAttempts, sleep)
	findings := scaffold.CheckPins(pins, fetch)

	rep := buildReport(ctx, findings, validBotPR(*botPR))
	io.WriteString(stdout, rep.log)

	if p := getenv("GITHUB_STEP_SUMMARY"); p != "" {
		if err := appendFile(p, rep.summary); err != nil {
			fmt.Fprintf(stderr, "pins-gate: could not write the job summary: %v\n", err)
		}
	}
	if p := getenv("GITHUB_OUTPUT"); p != "" {
		if err := appendFile(p, fmt.Sprintf("mode=%s\nverdict=%s\n", ctx.Mode, rep.verdict)); err != nil {
			fmt.Fprintf(stderr, "pins-gate: could not write the step outputs: %v\n", err)
		}
	}

	code := exitOK
	if rep.blocking > 0 {
		code = exitBlocked
	}
	// The LAST line, and the one the workflow's positive control requires: a
	// run that exits 0 without printing it did not reach a verdict.
	fmt.Fprintf(stdout, "pins-gate: mode=%s verdict=%s pins=%d blocking=%d exit=%d\n", ctx.Mode, rep.verdict, len(findings), rep.blocking, code)
	return code
}

func appendFile(p, s string) error {
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(s); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ─── the report ──────────────────────────────────────────────────────────────

type report struct {
	verdict  string // ok | stale | unreachable | broken — the WORST state present
	blocking int    // findings that fail this run
	log      string // stdout: one workflow-command annotation per non-OK package
	summary  string // markdown for $GITHUB_STEP_SUMMARY
}

// Summary headings. They are exported to the tests verbatim and quoted in the PR
// that introduced this command as "the text to look for", so a reword is a
// contract change, not a tidy-up.
const (
	headOK                = "### pins-vs-published: every pin admits npm's published latest"
	headAdvisedStale      = "### pins-vs-published: STALE PIN — reported, NOT blocking this pull request"
	headAdvisedUnreached  = "### pins-vs-published: NOT CHECKED — npm was unreachable (not blocking this pull request)"
	headBlockedStale      = "### pins-vs-published: FAILED — a scaffold pin is stale against npm"
	headBlockedUnreached  = "### pins-vs-published: FAILED — npm was unreachable, so the pins were NOT checked"
	headBlockedBroken     = "### pins-vs-published: FAILED — a scaffold pin is invalid"
	bumpWorkflowName      = "bump-scaffold-pins"
	bumpCommand           = "go run ./internal/scaffold/cmd/bump-pins"
	titleAdvisedStale     = "pins-vs-published: stale pin (reported, not blocking this pull request)"
	titleAdvisedUnreached = "pins-vs-published: NOT CHECKED, npm unreachable (not blocking this pull request)"
	titleBlockedStale     = "pins-vs-published: stale pin"
	titleBlockedUnreached = "pins-vs-published: NOT CHECKED, npm unreachable"
	titleBroken           = "pins-vs-published: invalid pin"
)

func buildReport(ctx runContext, findings []scaffold.PinFinding, botPR string) report {
	var log, rows strings.Builder
	var nStale, nUnreached, nBroken, blocking int

	fix := "No open `" + bumpWorkflowName + "` pull request was found. That workflow opens one on its next sweep; to fix it by hand run `" + bumpCommand + "`."
	if botPR != "" {
		fix = "The fix is the bump bot's pull request: " + botPR
	}

	for _, f := range findings {
		blocked := blocks(ctx.Mode, f.Status)
		if blocked {
			blocking++
		}
		pkg, pin, pub, file := safeToken(f.Pkg), safeToken(f.Pin), safeToken(f.Published), safeToken(f.File)
		fmt.Fprintf(&rows, "| `%s` | `%s` | `%s` | `%s` | %s | %s |\n", file, pkg, pin, orDash(pub), f.Status, yesNo(blocked))

		var title, msg string
		switch f.Status {
		case scaffold.PinOK:
			fmt.Fprintf(&log, "pins-gate:   ok          %s pinned %s admits published %s (%s)\n", pkg, pin, pub, file)
			continue
		case scaffold.PinStale:
			nStale++
			msg = fmt.Sprintf("%s is pinned %s in %s, which does NOT admit npm's published %s (a pre-1.0 caret locks the minor).", pkg, pin, file, pub)
			if blocked {
				title = titleBlockedStale
				msg += " This run gates: " + ctx.Reason + ". " + fix
			} else {
				title = titleAdvisedStale
				msg += " This pull request did not cause it and is not blocked by it. " + fix
			}
		case scaffold.PinUnreachable:
			nUnreached++
			msg = fmt.Sprintf("npm could not be asked about %s (%v), so the pin %s in %s was NOT checked. This is not a verdict about the pin.", pkg, f.Err, pin, file)
			if blocked {
				title = titleBlockedUnreached
				msg += " This run gates (" + ctx.Reason + "), and a gate that could not look must not report green. Re-run the job."
			} else {
				title = titleAdvisedUnreached
				msg += " This pull request touched no pin input, so it is not blocked — but read this as \"unknown\", not as \"fine\"."
			}
		default:
			nBroken++
			title = titleBroken
			msg = fmt.Sprintf("%s pinned %s in %s is %s (%v). This is a defect in the templates, not npm moving, so it is never downgraded.", pkg, pin, file, f.Status, f.Err)
		}
		level := "warning"
		if blocked {
			level = "error"
		}
		fmt.Fprintf(&log, "::%s title=%s::%s\n", level, title, cmdData(msg))
	}

	r := report{blocking: blocking, log: log.String()}
	var head, lede string
	switch {
	case nBroken > 0:
		r.verdict, head = "broken", headBlockedBroken
		lede = "A template pins a package npm does not publish, or a pin that cannot be compared. Only an edit to the templates fixes this; it blocks every run, whatever the run touched."
	case nStale > 0 && ctx.Mode == modeAdvise:
		r.verdict, head = "stale", headAdvisedStale
		lede = "npm published a version a scaffold pin does not admit. **This pull request did not cause that and is not blocked by it** — the job is green on purpose. " + fix
	case nStale > 0:
		r.verdict, head = "stale", headBlockedStale
		lede = "npm published a version a scaffold pin does not admit, and this run gates on it. " + fix
	case nUnreached > 0 && ctx.Mode == modeAdvise:
		r.verdict, head = "unreachable", headAdvisedUnreached
		lede = "The registry could not be asked, so **nothing about the pins was verified by this run**. This pull request touched no pin input, so it is not blocked; the green here means \"unknown\", not \"current\"."
	case nUnreached > 0:
		r.verdict, head = "unreachable", headBlockedUnreached
		lede = "The registry could not be asked after " + fmt.Sprint(fetchAttempts) + " attempts, and this run gates — so it fails rather than report a green that checked nothing. Re-run the job."
	default:
		r.verdict, head = "ok", headOK
		lede = fmt.Sprintf("%d pin(s) checked.", len(findings))
	}

	var s strings.Builder
	s.WriteString(head + "\n\n" + lede + "\n\n")
	fmt.Fprintf(&s, "**Mode: `%s`** — %s.\n\n", ctx.Mode, mdText(ctx.Reason))
	if len(ctx.Touched) > 0 {
		s.WriteString("Pin inputs this pull request changes:\n\n")
		for i, p := range ctx.Touched {
			if i == 20 {
				fmt.Fprintf(&s, "- …and %d more\n", len(ctx.Touched)-i)
				break
			}
			fmt.Fprintf(&s, "- `%s`\n", safeToken(p))
		}
		s.WriteString("\n")
	}
	s.WriteString("| template | package | pinned | npm latest | state | blocks this run |\n|---|---|---|---|---|---|\n")
	s.WriteString(rows.String())
	s.WriteString("\n")
	r.summary = s.String()
	return r
}

func yesNo(b bool) string {
	if b {
		return "**yes**"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// ─── untrusted text ──────────────────────────────────────────────────────────
//
// A version string comes from the registry and an error string quotes its reply;
// both end up inside a workflow command and a markdown table. Neither may be
// able to start a second annotation or break out of a cell.

var botPRRe = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/pull/[0-9]+$`)

// validBotPR returns u only if it is a plain GitHub pull-request URL. It comes
// from a `gh pr list` over branch NAMES, so it is treated as untrusted.
func validBotPR(u string) string {
	u = strings.TrimSpace(u)
	if botPRRe.MatchString(u) {
		return u
	}
	return ""
}

// safeToken reduces a package name / pin / version / path to the characters such
// a thing is made of. Anything else becomes `?`, visibly.
func safeToken(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case strings.ContainsRune("@/^~.+-_=<>* ", r):
			return r
		}
		return '?'
	}, s)
}

// cmdData escapes a workflow-command message the way the runner documents, so a
// newline in it cannot begin a new command.
func cmdData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// mdText flattens free text for a markdown paragraph.
func mdText(s string) string {
	return strings.NewReplacer("`", "'", "|", "/", "<", "‹", ">", "›").Replace(oneLine(s))
}
