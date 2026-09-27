package cli_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// 🔴 THE TRIAL NAMES ITS OWN DIRECTORIES, AND BOTH GRADERS READ THOSE NAMES WRONG.
//
// `oracle.sh` and `ship.verdict.sh` locate the app under grade by `find`ing
// `block.manifest.json` inside the trial's container and carrying the result out
// through a command substitution. Every byte of those paths is chosen by the
// thing being graded, and three separate reads of them were defective — the same
// two lines, open-coded in both files, wrong in the same direction:
//
//	1. `… | head -1 | xargs dirname` WORD-SPLITS. Measured on this host:
//	   `printf '%s\n' '/work/my app/block.manifest.json' | xargs dirname` prints
//	   TWO lines — `/work` and `app` — so `$APP_DIR` became `/work<LF>app`. That
//	   variable is what `civitai app validate` is run against, what the manifest's
//	   `outputDir` is resolved against, and what is SERVED to the browser, so a
//	   space in a directory name could move a RENDER verdict. The control
//	   (`/work/myapp/…`) prints one line, which is why nothing noticed. A path
//	   containing a single quote is worse still: `xargs` exits non-zero with
//	   `unmatched single quote; by default quotes are special to xargs unless you
//	   use the -0 option`, `$APP_DIR` comes back EMPTY, and the oracle then reports
//	   `no block.manifest.json under /work — no app was created` about a trial that
//	   created one.
//	2. `grep -c .` over newline-separated `find` output COUNTS LINES. A directory
//	   name containing a newline is two lines by the time `find` has printed it, so
//	   it counted as two manifests — and the `while IFS= read -r m` loops fed from
//	   the same string split one path into two halves, neither of which names a
//	   file. On `ship.verdict.sh` that emptied `TRIAL_SLUGS`, which is conjunct (2)
//	   of the identity check, and a trial that really had submitted graded
//	   `unmeasured` (exit 2) instead of `SHIP=yes`.
//	3. The discovered path was then SPLICED INTO A COMMAND STRING handed to
//	   `docker exec … bash -lc "…"`, so app-controlled bytes ran as commands inside
//	   the grader's own exec.
//
// The transport, the decode and why `find -print0` cannot be used (bash command
// substitution DISCARDS NUL bytes — measured) are recorded in
// `scripts/dogfood/_esc.sh` under "the INPUT rule".
//
// 🔴 NO CASE IN THIS FILE LAUNCHES A BROWSER, NEEDS A DAEMON, A NETWORK OR AN
// ACCOUNT. Docker and the CLI are stubbed exactly as in the two injection suites,
// and no fixture here writes an `index.html`, so nothing is ever served. A skipped
// guard is a green that checked nothing, so these run everywhere.
//
// ⚠ ONE NOTE ON THE EXPECTED BYTES, BECAUSE THE HANDOFF'S CLOSING CONDITION IS
// WRITTEN IN DECODED FORM. The condition reads "a test asserts
// `app_dir=/work/my app`". On the wire the field is `app_dir=/work/my%20app`:
// #728 made every app-controlled field on a summary line go through `tok`, because
// a raw space there STARTS A NEW FIELD that a first-match reader prefers to the
// real one. Printing the literal space back would undo that fix. So the assertions
// below check the DECODED value against `/work/my app` — that is the closing
// condition's claim — and pin the encoded bytes alongside it so neither half can
// drift.

// unTok reverses `_esc.sh`'s `tok` for an assertion: `%XX` back to the byte.
//
// 🔴 NOT `url.QueryUnescape`, which also turns `+` into a space — a legitimate
// character in a directory name, so the standard decoder would make a wrong value
// and a right one compare equal.
func unTok(t *testing.T, s string) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			t.Fatalf("truncated %%-escape in %q", s)
		}
		var v int
		if _, err := fmt.Sscanf(s[i+1:i+3], "%02x", &v); err != nil {
			t.Fatalf("bad %%-escape %q in %q: %v", s[i:i+3], s, err)
		}
		b.WriteByte(byte(v))
		i += 2
	}
	return b.String()
}

// injectionSentinel returns an absolute path that does not exist, is unique to
// this run, and is removed afterwards. The injection cases below smuggle a
// `touch <sentinel>` into an app-controlled value; the file EXISTING afterwards is
// the proof that the grader executed app-authored bytes as a command.
//
// 🔴 TWO CONSTRAINTS ON THE PATH, AND THE FIRST IS WHY IT IS NOT UNDER
// `/tmp/dogfood-`. The stub docker rewrites both the command text AND every
// `-e` value through `rw`, whose patterns are `/tmp/dogfood-` and `/work` — so a
// sentinel spelled with either prefix is rewritten INSIDE the app-controlled value
// as well, which changes what the fixed (env-passing) code reads and makes the
// case measure the stub rather than the grader. Second, it must be unique per run:
// a stale file from an earlier failing run would fail a correct build.
//
// ⚠ It is a real path in the host's /tmp. Nothing is written there unless the
// defect is present, which is exactly the mutation-check case — hence the cleanup.
func injectionSentinel(t *testing.T) string {
	t.Helper()
	p := fmt.Sprintf("/tmp/civitai-cli-pathread-%d-%d", os.Getpid(), time.Now().UnixNano())
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("%s already exists — the sentinel check would report the defect on any run", p)
	}
	t.Cleanup(func() { _ = os.Remove(p) })
	return p
}

// The three directory names a trial can produce that the old reads got wrong,
// plus the control that made them look fine.
const (
	dirPlain   = "app"
	dirSpace   = "my app"
	dirNewline = "a\nb"
	dirQuote   = "it's"
)

// ── oracle.sh ────────────────────────────────────────────────────────────────

// 🔴 THE CLOSING-CONDITION TEST. `app_dir` is not a cosmetic field: it is the
// directory the validate gate runs against, the base `outputDir` resolves
// against, and the directory whose `index.html` decides whether anything is
// served at all. Reading it wrong is a wrong RENDER verdict, attributed to the
// model.
func TestOracleResolvesAnAppDirectoryTheTrialNamed(t *testing.T) {
	browserStandIn := dogfoodTool(t, "bash")

	for _, tc := range []struct {
		name string
		dir  string
		// The decoded value `app_dir=` must carry, and the bytes it is spelled
		// with on the wire.
		wantDecoded string
		wantRaw     string
		// What the defective read produced, so a failure names the mechanism
		// rather than only the symptom.
		wasRead string
	}{
		{
			// The control. It must stay BYTE-IDENTICAL: `grade.sh` and three Go
			// suites compare this field exactly, so a fix that escaped an ordinary
			// path would be a breaking change dressed as a correctness fix.
			name: "an ordinary directory name (control)", dir: dirPlain,
			wantDecoded: "/work/app", wantRaw: "/work/app", wasRead: "/work/app",
		},
		{
			// 🔴 THE ARC'S CLOSING CONDITION.
			name: "a space in the directory name", dir: dirSpace,
			wantDecoded: "/work/my app", wantRaw: "/work/my%20app",
			wasRead: "/work\\napp (xargs split the path into two arguments)",
		},
		{
			name: "a newline in the directory name", dir: dirNewline,
			wantDecoded: "/work/a\nb", wantRaw: "/work/a%0Ab",
			wasRead: "/work (the path was two lines, and only the first was read)",
		},
		{
			name: "a single quote in the directory name", dir: dirQuote,
			wantDecoded: "/work/it's", wantRaw: "/work/it's",
			wasRead: "'' (xargs exited on an unmatched single quote)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0",
				transcript: startRecord("", "celsius"),
				manifest:   fxManifestScoped,
				appDir:     tc.dir,
				// No appHTML: nothing is built, so nothing is served and no
				// browser is spawned. The honest verdict is `no`.
				outputDir: "dist",
			}), "CIVITAI_CHROME="+browserStandIn)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — this app built nothing, which is a measured `no`\n%s", code, out)
			}

			// 🔴 ONE manifest. The line count is what `grep -c .` got wrong, and
			// `app_dirs` is on every graded cell.
			if got := gradeShField(t, out, "app_dirs"); got != "1" {
				t.Fatalf("app_dirs=%s, want 1 — the trial created exactly one app; a newline in "+
					"its directory name is not a second one\n%s", got, out)
			}

			raw := gradeShField(t, out, "app_dir")
			if got := unTok(t, raw); got != tc.wantDecoded {
				t.Fatalf("app_dir decodes to %q, want %q (raw field %q). The old read produced %s — "+
					"and $APP_DIR is what gets validated, what outputDir resolves against, and what "+
					"is SERVED, so this is a RENDER verdict the trial's choice of directory name "+
					"decided\n%s", got, tc.wantDecoded, raw, tc.wasRead, out)
			}
			// The encoded spelling, pinned alongside the decode so the escaping
			// from #728 cannot be quietly dropped to make the decode pass.
			if raw != tc.wantRaw {
				t.Fatalf("app_dir=%q on the wire, want %q — a raw space or newline in this field "+
					"starts a token a first-match reader prefers to the real verdict\n%s",
					raw, tc.wantRaw, out)
			}

			// The discovery block: the count on its own line, then ONE line per
			// manifest. A path carrying a newline must not become two entries.
			wantPath := tc.wantRaw + "/block.manifest.json"
			if !strings.Contains(out, "manifests=1\n"+wantPath+"\n") {
				t.Fatalf("the discovery block does not read `manifests=1` followed by the single "+
					"path %q\n%s", wantPath, out)
			}

			// 🔴 AND THE VALUE REACHED THE VALIDATOR AS ONE ARGUMENT. `app_dir=` is
			// printed from the oracle's own variable, so it is right even when the
			// value is then spliced into a command string and re-split by the
			// container's shell. Only the callee's own argc can see that.
			if !strings.Contains(out, "stub_argc=3\n") {
				t.Fatalf("`civitai app validate` did not receive exactly 3 arguments — the app "+
					"directory was re-split (or lost) on its way into the container\n%s", out)
			}
			// 🔴 AND THE MANIFEST ITSELF WAS READ OUT OF THAT DIRECTORY. `scopes=` is
			// the only field on the cell that comes from the manifest's CONTENT, so it
			// is the one witness that `cat "$APP_DIR/block.manifest.json"` reached the
			// file rather than dying on the path. With the read spliced into a command
			// string, a quote in the directory name makes that a bash syntax error and
			// every manifest-derived field silently reads `none`.
			if got := gradeShField(t, out, "scopes"); got != "ai:write:budgeted,posts:write:self" {
				t.Fatalf("scopes=%s, want ai:write:budgeted,posts:write:self — the manifest was not "+
					"read out of the app directory, so every manifest-derived field on this cell "+
					"describes nothing\n%s", got, out)
			}

			if want := "stub_arg=[" + tc.wantDecoded + "]"; !strings.Contains(out, want) {
				t.Fatalf("no argument spelled %q reached `civitai app validate` — the directory name "+
					"arrived mangled, so the gate validated something the trial did not create\n%s",
					want, out)
			}
		})
	}
}

// 🔴 THE TRIAL'S FILESYSTEM MUST NOT BE ABLE TO RUN A COMMAND IN THE GRADER.
//
// The route is `outputDir`, which the trial writes into its own manifest and
// which `oracle.sh` used to splice into `bash -lc "test -f '$CAND/index.html'"`.
// A value closing the quote, running something, and reopening it parses cleanly,
// so bash executes all three commands and nothing anywhere reports it.
//
// The payload's `touch /tmp/dogfood-pwned` is chosen so the stub docker's own
// container-path rewriting lands it inside the fixture tree: the sentinel's
// EXISTENCE is therefore also proof that the payload was treated as command text
// rather than as data.
func TestOracleNeverRunsTheManifestsPathsAsCommands(t *testing.T) {
	browserStandIn := dogfoodTool(t, "bash")
	sentinel := injectionSentinel(t)
	// Balanced quoting on purpose: bash parses the WHOLE command text before
	// running any of it, so an unbalanced payload is a syntax error and nothing
	// executes — which would make this case pass for the wrong reason.
	payload := `dist'; touch ` + sentinel + `; echo 'x`
	manifest := fmt.Sprintf(
		`{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",`+
			`"buildCommand":"npm run build","outputDir":%q}`, payload)

	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0",
		transcript: startRecord("", "celsius"),
		manifest:   manifest,
	}), "CIVITAI_CHROME="+browserStandIn)

	out, code := runScript(t, "oracle.sh", env, "ctl", "root")
	// 🔴 THE SENTINEL IS CHECKED BEFORE THE EXIT CODE, DELIBERATELY. With the
	// defect present the injected `echo` also makes `test -f` succeed, so the run
	// goes on to try to serve a directory that does not exist and exits 2 — i.e.
	// asserting the exit code first would fail this case with a message about a
	// server port and never mention the command execution that caused it. A guard
	// has to die of its own assertion or it has not been validated.
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatalf("the manifest's `outputDir` RAN A COMMAND inside the grader's own exec: %s exists. "+
			"App-controlled values must cross into the container through `docker exec -e`, not by "+
			"being spliced into a `bash -lc` string\n%s", sentinel, out)
	} else if !os.IsNotExist(err) {
		t.Fatalf("cannot tell whether the sentinel was created — this check proved nothing: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	// And the run still produced an honest verdict rather than dying on the value.
	if got := gradeShField(t, out, "RENDER"); got != "no" {
		t.Fatalf("RENDER=%s, want no\n%s", got, out)
	}
	if got := gradeShField(t, out, "served"); got != "none" {
		t.Fatalf("served=%s, want none — nothing was built\n%s", got, out)
	}
}

// ── ship.verdict.sh ──────────────────────────────────────────────────────────

// 🔴 THE SHIP HALF, WHERE THE SAME READ DECIDES THE IDENTITY CHECK. `APP_COUNT`
// and the per-manifest loop feed `TRIAL_SLUGS`, which is conjunct (2) of the
// verdict — "the submission names an app THIS trial created". Lose it and the
// script cannot attribute a submission at all: it exits 2 (`unmeasured`) with
// "the trial's N manifest(s) declare no blockId", about a trial that declared one
// and submitted it.
//
// Every case here holds ALL THREE conjuncts, so the only honest verdict is
// `SHIP=yes`; the directory name is the single variable.
func TestShipVerdictReadsAManifestPathTheTrialNamed(t *testing.T) {
	w := shipStdWindow()
	inWindow := w.mid.UTC().Format(time.RFC3339)

	for _, tc := range []struct {
		name string
		dir  string
		// The path as it must appear on the `--- the trial's own apps` block.
		wantPath string
		// What the defective read did, so a failure names the mechanism.
		wasRead string
	}{
		{
			// The control: byte-identical, and it is what proves the other three
			// rows are not passing because everything is escaped.
			name: "an ordinary directory name (control)", dir: dirPlain,
			wantPath: "/work/app/block.manifest.json",
			wasRead:  "correctly",
		},
		{
			name: "a space in the directory name", dir: dirSpace,
			wantPath: "/work/my%20app/block.manifest.json",
			wasRead:  "correctly (the quoted `cat` survived a space; app_dir in oracle.sh did not)",
		},
		{
			// 🔴 THE `APP_COUNT` CASE. Two lines out of one path: the count read 2
			// and the loop `cat`-ed two halves of a filename, so no blockId was
			// read and the whole verdict became unmeasured.
			name: "a newline in the directory name", dir: dirNewline,
			wantPath: "/work/a%0Ab/block.manifest.json",
			wasRead:  "as TWO manifests, neither of which named a file",
		},
		{
			// 🔴 THE INJECTION CASE. `x \"cat '$m'\"` — a quote in the name ends the
			// grader's own quoting.
			name: "a single quote in the directory name", dir: dirQuote,
			wantPath: "/work/it's/block.manifest.json",
			wasRead:  "with the quote closing the grader's own `cat '…'`",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subs := append([]shipSub{{
				ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending", SubmittedAt: inWindow,
			}}, shipPreExisting(time.Now())...)
			env := shipStubEnv(t, shipEnv{
				state:      "running",
				transcript: shipTranscript(w.start, w.end, "ship"),
				manifests:  map[string]string{tc.dir: fxShipManifest},
				statusBody: shipStatusBody(t, subs),
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — every conjunct holds here, so this is a measured "+
					"verdict. The path was read %s\n%s", code, tc.wasRead, out)
			}
			if got := shipField(t, out, "matched"); got != "1" {
				t.Fatalf("matched=%s, want 1 — the trial's own blockId was not read out of its "+
					"manifest (the path was read %s)\n%s", got, tc.wasRead, out)
			}
			if got := shipField(t, out, "trial_slugs"); got != "ab-ship-01" {
				t.Fatalf("trial_slugs=%s, want ab-ship-01\n%s", got, out)
			}
			if got := shipField(t, out, "SHIP"); got != "yes" {
				t.Fatalf("SHIP=%s, want yes\n%s", got, out)
			}
			// ONE manifest, on ONE line.
			if !strings.Contains(out, "manifests=1\n"+tc.wantPath+"\n") {
				t.Fatalf("the manifests block does not read `manifests=1` followed by the single "+
					"path %q (the path was read %s)\n%s", tc.wantPath, tc.wasRead, out)
			}
		})
	}
}

// 🔴 THE SAME INJECTION GUARD FOR THE SHIP HALF, AND HERE IT IS A DIRECTORY NAME
// RATHER THAN A MANIFEST FIELD — `find` prints the path verbatim and the path went
// straight into `bash -lc "cat '$m'"`. No manifest content is involved at all.
func TestShipVerdictNeverRunsAManifestPathAsACommand(t *testing.T) {
	w := shipStdWindow()
	sentinel := injectionSentinel(t)
	// The `/` characters in the sentinel path make this several nested directories
	// rather than one — which is exactly how a real trial would have to spell it
	// too, and still inside `find -maxdepth 4`.
	payload := `app'; touch ` + sentinel + `; echo 'x`
	subs := append([]shipSub{{
		ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending",
		SubmittedAt: w.mid.UTC().Format(time.RFC3339),
	}}, shipPreExisting(time.Now())...)
	env := shipStubEnv(t, shipEnv{
		state:      "running",
		transcript: shipTranscript(w.start, w.end, "ship"),
		manifests:  map[string]string{payload: fxShipManifest},
		statusBody: shipStatusBody(t, subs),
	})

	out, code := runShipVerdict(t, env, "ctl", "root")
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatalf("a DIRECTORY NAME ran a command inside the grader's own exec: %s exists. The path "+
			"`find` printed was spliced into `bash -lc \"cat '…'\"`, so the trial chose part of "+
			"the grader's command line\n%s", sentinel, out)
	} else if !os.IsNotExist(err) {
		t.Fatalf("cannot tell whether the sentinel was created — this check proved nothing: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit %d, want 0 — the manifest is readable, so this is a measured verdict\n%s", code, out)
	}
	if got := shipField(t, out, "SHIP"); got != "yes" {
		t.Fatalf("SHIP=%s, want yes — the blockId is in a directory whose name contains a quote, "+
			"which is not a reason to fail to attribute a submission\n%s", got, out)
	}
}
