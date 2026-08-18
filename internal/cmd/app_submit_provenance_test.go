package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// TESTS FOR THE PROVENANCE STAMP'S FACT-GATHERING HALF (issue #411).
//
// The refusal half (#415) is app_submit_dirty_guard_test.go; this file is about
// what checkWorkTreeClean now RETURNS, and about the two seams that value
// crosses on its way to the server:
//
//	git stdout → checkWorkTreeClean → appapi.Provenance → submitBody → HTTP body
//
// 🔴 A UNIT TEST ON EACH END PROVES NEITHER. appapi's own tests prove a bad
// value cannot leave the client; the fixtures here prove which value real git
// produces. The defect that would survive both is in the SEAM — a gathering step
// that hands over something the sender then dutifully drops (a stamp that never
// arrives), or one that hands over something the sender happily forwards (a 400
// that kills the submit). So the hostile-input test below drives the REAL
// gathering function into the REAL client against an httptest server, and reads
// the bytes that came out the far end.

// fullSHARe is the SERVER's rule, spelled out here rather than imported, so an
// assertion in this file is never a restatement of the code it is testing.
var fullSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// guardFacts runs the guard over a real repository and returns everything it
// answered: the provenance, the warnings, the verdict.
func guardFacts(t *testing.T, dir string, allowDirty bool) (appapi.Provenance, string, error) {
	t.Helper()
	var warn bytes.Buffer
	prov, err := checkWorkTreeClean(gitOutput, &warn, dir, dirtySlug, "0.6.1", allowDirty)
	return prov, warn.String(), err
}

func dirtyStr(d *bool) string {
	if d == nil {
		return "nil (UNKNOWN)"
	}
	return strconv.FormatBool(*d)
}

// TestProvenanceOnACleanTreeIsTheRealHeadSHA — the ordinary case, and the
// expectation is git's own answer rather than a literal, so a truncation, a
// different ref or a stale cache all show up.
func TestProvenanceOnACleanTreeIsTheRealHeadSHA(t *testing.T) {
	f := cleanAppFixture(t, "")
	want := f.gitIn(f.root, "rev-parse", "HEAD")

	prov, warn, err := guardFacts(t, f.root, false)
	if err != nil {
		t.Fatalf("a clean tree must proceed: %v", err)
	}
	if prov.Commit != want {
		t.Errorf("sourceCommit = %q, want the tree's real HEAD %q (warnings: %s)", prov.Commit, want, warn)
	}
	if !fullSHARe.MatchString(prov.Commit) {
		t.Errorf("sourceCommit %q is not the shape the server accepts — it would 400 the submit", prov.Commit)
	}
	if prov.Dirty == nil || *prov.Dirty {
		t.Errorf("a clean tree must claim sourceDirty=false (an ASSERTION of clean, not silence), got %s",
			dirtyStr(prov.Dirty))
	}
}

// TestProvenanceUnderAllowDirtyClaimsDirty is the row with the most diagnostic
// value, and the one the old --allow-dirty short-circuit made impossible.
//
// 🔴 IT IS ALSO A CONTRACT REGRESSION GUARD FOR #415: the same call must not
// refuse and must not print. Gathering facts is allowed to cost a subprocess; it
// is not allowed to change the verdict.
func TestProvenanceUnderAllowDirtyClaimsDirty(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.write("src/App.tsx", "uncommitted")
	want := f.gitIn(f.root, "rev-parse", "HEAD")

	// Control: the same tree really is refused without the flag.
	if _, _, err := guardFacts(t, f.root, false); err == nil {
		t.Fatal("control: this tree must be refused WITHOUT --allow-dirty, or the row below proves nothing")
	}

	prov, warn, err := guardFacts(t, f.root, true)
	if err != nil {
		t.Fatalf("--allow-dirty must never refuse: %v", err)
	}
	if warn != "" {
		t.Errorf("--allow-dirty must stay silent, got:\n%s", warn)
	}
	if prov.Commit != want {
		t.Errorf("sourceCommit = %q, want %q — a dirty bundle still has a commit it is dirty AGAINST, "+
			"and that is the value someone will need to explain it later", prov.Commit, want)
	}
	if prov.Dirty == nil || !*prov.Dirty {
		t.Errorf("--allow-dirty on a dirty tree must claim sourceDirty=true, got %s", dirtyStr(prov.Dirty))
	}
}

// TestProvenanceIsAbsentWhereNoFactExists walks every branch that cannot
// establish a commit. All of them must return the ZERO provenance: the wire
// spelling of UNKNOWN is an absent key, and a default here would be a claim
// nobody made.
//
// 🔴 THE FIRST ROW IS THE SCAFFOLD PATH THE ISSUE SAYS MUST NOT BREAK.
func TestProvenanceIsAbsentWhereNoFactExists(t *testing.T) {
	t.Run("a directory in no git repo", func(t *testing.T) {
		requireGit(t)
		gitFixtureEnv(t)
		dir := t.TempDir()
		prov, warn, err := guardFacts(t, dir, false)
		if err != nil {
			t.Fatalf("a scaffolded app with no repo must submit exactly as before: %v", err)
		}
		if warn != "" {
			t.Errorf("the no-repo path must stay silent, got:\n%s", warn)
		}
		assertNoProvenance(t, prov)
	})

	t.Run("no git binary at all", func(t *testing.T) {
		noGit := func(string, ...string) (string, error) { return "", errGitUnavailable }
		var w bytes.Buffer
		prov, err := checkWorkTreeClean(noGit, &w, "/some/dir", dirtySlug, "0.6.1", false)
		if err != nil {
			t.Fatalf("no git binary must degrade like no repo: %v", err)
		}
		assertNoProvenance(t, prov)
	})

	t.Run("an unborn HEAD — a repo with no commits", func(t *testing.T) {
		f := newGitFixture(t) // init only; nothing committed
		writeManifestVersion(t, f.root, dirtySlug, "0.6.1")
		// --allow-dirty, because an unborn HEAD's tree is untracked by
		// definition; the flag isolates the question to the COMMIT.
		prov, _, err := guardFacts(t, f.root, true)
		if err != nil {
			t.Fatalf("an unborn HEAD must not break a submit: %v", err)
		}
		if prov.Commit != "" {
			t.Errorf("sourceCommit = %q for a repo with no commits — there is no commit to claim", prov.Commit)
		}
	})

	t.Run("a bare repo / inside .git", func(t *testing.T) {
		stub := func(_ string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "rev-parse" {
				return "false\n", nil
			}
			return "", errors.New("nothing else should be asked")
		}
		var w bytes.Buffer
		prov, err := checkWorkTreeClean(stub, &w, "/some/dir", dirtySlug, "0.6.1", false)
		if err != nil {
			t.Fatalf("no working tree means nothing can be dirty: %v", err)
		}
		assertNoProvenance(t, prov)
	})
}

func assertNoProvenance(t *testing.T, prov appapi.Provenance) {
	t.Helper()
	if prov.Commit != "" {
		t.Errorf("sourceCommit = %q where no commit could be established; want \"\" so the key is ABSENT on the wire", prov.Commit)
	}
	if prov.Dirty != nil {
		t.Errorf("sourceDirty = %v where nothing was established; want nil.\n"+
			"false is an ASSERTION that a client looked and the tree was clean — it must never be a default.", *prov.Dirty)
	}
}

// TestProvenanceKeepsDirtinessUnknownWhenStatusFails is the half-known row. The
// commit resolved; the question about the work tree failed. `false` here would
// be the `?? false` collapse: a claim of clean built out of a failure.
func TestProvenanceKeepsDirtinessUnknownWhenStatusFails(t *testing.T) {
	const sha = "3b1d90c07af4e5628ba1d7c9f0e24587ac6bd913"
	stub := func(_ string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD":
			return sha + "\n", nil
		case len(args) > 0 && args[0] == "rev-parse":
			return "true\n\n", nil
		}
		return "", errors.New("fatal: index file corrupt")
	}
	var w bytes.Buffer
	prov, err := checkWorkTreeClean(stub, &w, "/some/dir", dirtySlug, "0.6.1", false)
	if err != nil {
		t.Fatalf("a failed status read must not block the submit: %v", err)
	}
	if prov.Commit != sha {
		t.Errorf("sourceCommit = %q, want %q — the commit was established even though the status read was not", prov.Commit, sha)
	}
	if prov.Dirty != nil {
		t.Errorf("sourceDirty = %v after a FAILED status read; want nil (UNKNOWN). "+
			"A question this run could not ask has no answer, and the reassuring answer is the wrong default.", *prov.Dirty)
	}
	if !strings.Contains(w.String(), "could not check") {
		t.Errorf("the fail-open must still be announced, got:\n%s", w.String())
	}
}

// --- THE SEAM: git stdout → guard → client → HTTP body ---------------------

// submitThrough sends prov through the REAL appapi client to a recording server
// and returns the request body's raw fields.
func submitThrough(t *testing.T, prov appapi.Provenance) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"publishRequestId": "pubreq_x", "slug": dirtySlug, "version": "0.6.1", "status": "pending",
		})
	}))
	defer srv.Close()

	c := appapi.New(srv.URL, "tok", "")
	if _, err := c.SubmitVersion(context.Background(), []byte("ZIP"), dirtySlug, "0.6.1", prov); err != nil {
		t.Fatalf("SubmitVersion: %v", err)
	}
	if _, ok := got["bundleBase64"]; !ok {
		t.Fatal("CONTROL failure: the recorded body is not a submit body")
	}
	return got
}

// TestHostileGitOutputNeverReachesTheWire is the guard that matters most, and it
// is deliberately built at the SEAM rather than at either end.
//
// Every row is a string a real `git rev-parse HEAD` could hand this CLI when
// something is wrong — a stale git, a hook-mangled environment, a detached-HEAD
// message, an error printed to stdout. The assertion is not "the stamp is
// right"; it is that the request body carries either NO sourceCommit or one the
// server's `^[0-9a-f]{40}$` accepts. Anything else is a 400 on the whole upload:
// the provenance feature would have taken away a submit that used to work.
func TestHostileGitOutputNeverReachesTheWire(t *testing.T) {
	const good = "9f2c41ab7de305619c8bd4a0e7f13c25db806e4f"
	rows := []struct{ name, revParseHEAD string }{
		{"an empty answer", ""},
		{"whitespace only", "   \n"},
		{"an uppercase sha", strings.ToUpper(good)},
		{"an abbreviated sha", good[:7]},
		{"39 characters", good[:39]},
		{"41 characters", good + "0"},
		{"a sha with a trailing CRLF", good + "\r\n"},
		{"a detached-HEAD description", "HEAD detached at 9f2c41a"},
		{"the ref name instead of the sha", "HEAD"},
		{"an error on stdout", "fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree."},
		{"a sha with a NUL", good[:20] + "\x00" + good[21:]},
		{"two shas", good + " " + good},
		{"a value that would break out of the JSON string", `","sourceDirty":true,"evil":"`},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			// A repo that says it is clean, so the ONLY variable is HEAD's text.
			stub := func(_ string, args ...string) (string, error) {
				switch {
				case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD":
					return row.revParseHEAD, nil
				case len(args) > 0 && args[0] == "rev-parse":
					return "true\n\n", nil
				case len(args) > 0 && args[0] == "for-each-ref":
					return "origin/main\n", nil
				}
				return "", nil // an empty porcelain status: clean
			}
			var w bytes.Buffer
			prov, err := checkWorkTreeClean(stub, &w, "/some/dir", dirtySlug, "0.6.1", false)
			if err != nil {
				t.Fatalf("the tree is clean; the guard must not refuse: %v", err)
			}

			body := submitThrough(t, prov)
			raw, present := body["sourceCommit"]
			if !present {
				if _, d := body["sourceDirty"]; d {
					t.Errorf("no sourceCommit was sent, but sourceDirty was — that claims a work-tree "+
						"state for a commit the CLI could not name (git said %q)", row.revParseHEAD)
				}
				return
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				t.Fatalf("sourceCommit is not a JSON string: %s", raw)
			}
			if !fullSHARe.MatchString(s) {
				t.Errorf("git printed %q and the CLI put %q on the wire.\n"+
					"The server answers a malformed sourceCommit with a HARD 400 that fails the whole submit — "+
					"a provenance stamp must never turn a working submit into a failed one.", row.revParseHEAD, s)
			}
		})
	}

	// POSITIVE CONTROL on the whole chain: a good sha must actually arrive, or
	// every row above would pass against a chain that sends nothing at all.
	stub := func(_ string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD":
			return good + "\n", nil
		case len(args) > 0 && args[0] == "rev-parse":
			return "true\n\n", nil
		case len(args) > 0 && args[0] == "for-each-ref":
			return "origin/main\n", nil
		}
		return "", nil
	}
	var w bytes.Buffer
	prov, err := checkWorkTreeClean(stub, &w, "/some/dir", dirtySlug, "0.6.1", false)
	if err != nil {
		t.Fatalf("control: %v", err)
	}
	body := submitThrough(t, prov)
	if got := string(body["sourceCommit"]); got != `"`+good+`"` {
		t.Fatalf("CONTROL failure: a well-formed sha did not survive the chain (got %s). "+
			"Every row above would report the same 'nothing malformed was sent' with the feature deleted.", got)
	}
	if got := string(body["sourceDirty"]); got != "false" {
		t.Fatalf("CONTROL failure: a clean tree did not send an explicit false (got %q)", got)
	}
}

// --- END TO END: the command, a real repo, a recording server ---------------

// submitRecorder is a server that answers both routes `app submit` touches and
// records the submit body plus its exact byte length.
type submitRecorder struct {
	body   map[string]json.RawMessage
	nbytes int
}

func newSubmitRecorder(t *testing.T) (*submitRecorder, *httptest.Server) {
	t.Helper()
	rec := &submitRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "submit-version") {
			var buf bytes.Buffer
			n, _ := buf.ReadFrom(r.Body)
			rec.nbytes = int(n)
			if err := json.Unmarshal(buf.Bytes(), &rec.body); err != nil {
				t.Errorf("decode submit body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"publishRequestId": "pubreq_e2e", "slug": dirtySlug, "version": "0.6.1", "status": "pending",
			})
			return
		}
		// The version guard's listing read.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []any{}})
	}))
	return rec, srv
}

func runSubmitIn(t *testing.T, dir, srvURL string, extra ...string) (string, string, error) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-prov")
	t.Setenv("CIVITAI_BASE_URL", srvURL)
	t.Setenv("CIVITAI_SUBMIT_PATH", "")
	return run(t, append([]string{"app", "submit", dir, "--yes"}, extra...)...)
}

// TestAppSubmitStampsTheCommitEndToEnd drives the whole command over a real git
// repository and reads what the server received. The three rows are the three
// answers the feature can give.
func TestAppSubmitStampsTheCommitEndToEnd(t *testing.T) {
	t.Run("a clean repo stamps the commit and an explicit false", func(t *testing.T) {
		f := cleanAppFixture(t, "")
		f.publishHEAD()
		want := f.gitIn(f.root, "rev-parse", "HEAD")

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL)
		if err != nil {
			t.Fatalf("submit: %v\n%s\n%s", err, stdout, stderr)
		}
		if got := string(rec.body["sourceCommit"]); got != `"`+want+`"` {
			t.Errorf("the server received sourceCommit=%s, want %q", got, want)
		}
		if got := string(rec.body["sourceDirty"]); got != "false" {
			t.Errorf("the server received sourceDirty=%q, want an explicit false", got)
		}
	})

	t.Run("--allow-dirty stamps the commit and true", func(t *testing.T) {
		f := cleanAppFixture(t, "")
		f.publishHEAD()
		f.write("src/App.tsx", "uncommitted")
		want := f.gitIn(f.root, "rev-parse", "HEAD")

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, f.root, srv.URL, "--allow-dirty")
		if err != nil {
			t.Fatalf("submit --allow-dirty: %v\n%s\n%s", err, stdout, stderr)
		}
		if got := string(rec.body["sourceCommit"]); got != `"`+want+`"` {
			t.Errorf("the server received sourceCommit=%s, want %q", got, want)
		}
		if got := string(rec.body["sourceDirty"]); got != "true" {
			t.Errorf("the server received sourceDirty=%q, want true — this is the case worth recording", got)
		}
	})

	// 🔴 THE REGRESSION ROW. A scaffolded app is a directory with no repo, and
	// that submit must be byte-identical to what it was before #411: no keys.
	t.Run("a directory in no repo sends no provenance at all", func(t *testing.T) {
		requireGit(t)
		gitFixtureEnv(t)
		dir := t.TempDir()
		writeManifestVersion(t, dir, dirtySlug, "0.6.1")

		rec, srv := newSubmitRecorder(t)
		defer srv.Close()
		stdout, stderr, err := runSubmitIn(t, dir, srv.URL)
		if err != nil {
			t.Fatalf("submit from a non-repo must still work: %v\n%s\n%s", err, stdout, stderr)
		}
		for _, k := range []string{"sourceCommit", "sourceDirty"} {
			if raw, ok := rec.body[k]; ok {
				t.Errorf("a directory in no git repo sent %s=%s; it must send NOTHING — "+
					"there is no fact here, and every scaffolded app starts this way", k, raw)
			}
		}
		if _, ok := rec.body["bundleBase64"]; !ok {
			t.Error("CONTROL failure: no bundle was uploaded, so the absence above is not evidence about provenance")
		}
	})
}

// TestPackagedLineReportsTheSizeOfTheBodyThatWasSENT is the #423 contract under
// #411's fields, measured end to end.
//
// 🔴 THIS IS WHY SubmitBodySize TAKES THE PROVENANCE. The number on the
// `Packaged …` line is published to users as the EXACT size a request-body limit
// applies to. A stamped submit is bigger than a bare one, so a size function
// that could not see the stamp would print a number that is no longer the one
// that left the machine — a small error in the quantity and a total one in the
// claim. The expectation here is the byte count the SERVER measured, not
// anything the CLI computed.
func TestPackagedLineReportsTheSizeOfTheBodyThatWasSENT(t *testing.T) {
	f := cleanAppFixture(t, "")
	f.publishHEAD()

	rec, srv := newSubmitRecorder(t)
	defer srv.Close()
	stdout, stderr, err := runSubmitIn(t, f.root, srv.URL)
	if err != nil {
		t.Fatalf("submit: %v\n%s\n%s", err, stdout, stderr)
	}
	m := packagedLineRe.FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("no `Packaged …` line in:\n%s", stdout)
	}
	reported := atoiOrFatal(t, m[4])
	if rec.nbytes == 0 {
		t.Fatal("CONTROL failure: the server measured a zero-byte request body")
	}
	if reported != rec.nbytes {
		t.Errorf("the CLI printed %d bytes as the submit-body size, but the server received %d.\n"+
			"The difference is the provenance stamp (%s): the printed number is documented as EXACT, "+
			"so it has to be computed from the body this run actually sends.",
			reported, rec.nbytes, provFields(rec.body))
	}
}

func provFields(body map[string]json.RawMessage) string {
	var parts []string
	for _, k := range []string{"sourceCommit", "sourceDirty"} {
		if raw, ok := body[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", k, raw))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}
