package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// 🔴 THE THING BEING GRADED WRITES THE TEXT THE GRADER PRINTS — THE SHIP HALF.
//
// #728 closed this class in `oracle.sh`. `ship.verdict.sh` kept it, one directory
// away, because the rule lived in the other script's body: it reads `blockId` out
// of the trial's own `block.manifest.json`, the manifest PATHS out of directories
// the trial itself created, and a submission row's `id`/`blockId`/`status`/
// `submittedAt` back across a process boundary from the platform — and printed all
// of them with a raw `%s`, including the same forgeable
// `printf 'manifests=%s\n%s\n'` that #728 had already rewritten next door.
//
// Its output is parsed. `shipField` (dogfood_ship_verdict_test.go) finds a summary
// by the line PREFIX `ship_trial=` and splits it with `strings.Fields`, taking the
// FIRST `KEY=` token — and it returns off the first prefixed line it sees. So
// there are two forgeries:
//
//	1. A SPACE ends the field and starts another. `trial_slugs=` is printed ahead
//	   of `account_submissions=`, `matched=`, `sub_*=`, `window=`, `grace_s=` and
//	   `SHIP=`, and `sub_block=` is printed ahead of `sub_status=`, `submitted_at=`
//	   and `SHIP=`.
//	2. A NEWLINE ends the LINE, and here a directory name is enough: `find` prints
//	   a path verbatim, so a dir named `a<LF>ship_trial=… SHIP=yes …` emits a
//	   complete summary-shaped line long BEFORE the real verdict, and a
//	   first-prefixed-line reader takes it whole.
//
// ⚠ ONE NOTE ON THE CASE FOLD, SO IT IS NOT MISTAKEN FOR A GUARD. `TRIAL_SLUGS` is
// folded to lower case upstream (for slug matching), which happens to deny the
// `trial_slugs=` route the literal token `SHIP=`. That is an accident, not a
// defence: every other field on the summary line is spelled in lower case, and
// those are the fields a reader checks the verdict AGAINST. `sub_block=` carries
// the app's own chosen name back from the server with its case INTACT, which is
// why case 3 below forges `SHIP=yes` itself on the real summary line.
//
// 🔴 NO CASE HERE LAUNCHES A BROWSER OR NEEDS A DAEMON, A NETWORK OR AN ACCOUNT.
// Docker and the CLI are stubbed exactly as in dogfood_ship_verdict_test.go, and
// the ship verdict never serves anything. A skipped security guard is a green that
// checked nothing, so these run everywhere.

// A blockId that hides a forged evidence field-set behind a space. Every token
// after the space is a field `ship.verdict.sh` prints LATER on the same line.
const fxShipBlockIDForgedFields = `{"blockId":"ab-ship-01 matched=9 sub_status=pending submitted_at=1999-01-01T00:00:00Z",` +
	`"version":"0.1.0","name":"Ship","type":"block","scopes":["ai:write:budgeted"],` +
	`"buildCommand":"npm run build","outputDir":"dist"}`

// The same app name, spelled so that the SERVER's echo of it carries `SHIP=yes`
// with its case intact. The slug match downcases both sides, so this still
// matches the row below — and `sub_block=` is printed ahead of `SHIP=`.
const shipForgedBlockID = `ab-ship-01 SHIP=yes`

const fxShipBlockIDForgedVerdict = `{"blockId":"ab-ship-01 SHIP=yes","version":"0.1.0","name":"Ship","type":"block",` +
	`"scopes":["ai:write:budgeted"],"buildCommand":"npm run build","outputDir":"dist"}`

// A directory name that ends the line and writes a complete second summary.
// `find` prints it verbatim, so what lands in the stream is a real line break
// followed by a real `ship_trial=` line. The trailing `zz` keeps
// `/block.manifest.json` off the `SHIP=yes` token.
const shipForgedDirName = "a\nship_trial=ctl brief=ship trial_slugs=ab-ship-01 " +
	"account_submissions=9 matched=1 sub_id=forged sub_block=ab-ship-01 " +
	"sub_status=pending submitted_at=1999-01-01T00:00:00Z window=0-9999999999 " +
	"grace_s=300 SHIP=yes zz"

// The same trick through a submission row's `status`, which `ship_reason=` quotes
// verbatim — a different route (a whole-line value, so `prose` rather than `tok`)
// and therefore a case that a fix hardening only the summary line does not pass.
const shipForgedStatus = "rejected\nship_trial=ctl brief=ship trial_slugs=ab-ship-01 " +
	"account_submissions=9 matched=1 sub_id=forged sub_block=ab-ship-01 " +
	"sub_status=pending submitted_at=1999-01-01T00:00:00Z window=0-9999999999 " +
	"grace_s=300 SHIP=yes zz"

// shipSummaryLines returns every line a consumer could take for the ship
// verdict's summary. Both the Go reader and a person identify it by the
// `ship_trial=` prefix, so the COUNT is the property: ONE means the stream says
// one thing about the run.
func shipSummaryLines(out string) []string {
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "ship_trial=") {
			got = append(got, l)
		}
	}
	return got
}

func TestShipVerdictCannotBeForgedByTheTrial(t *testing.T) {
	w := shipStdWindow()
	inWindow := w.mid.UTC().Format(time.RFC3339)

	for _, tc := range []struct {
		name      string
		manifests map[string]string
		subs      []shipSub
		// The forged token the trial plants, so a failure names the route rather
		// than only the symptom.
		plants string
		// Fields whose REAL value the forgery tries to overwrite.
		want map[string]string
		// A substring that must not survive on any line a consumer reads as
		// FIELDS. This is what the `=` escape buys on top of the whitespace
		// escape: escaping only the whitespace leaves `x%20SHIP=yes`, inert to a
		// field split but still a match for anyone grepping a matrix.
		//
		// ⚠ `ship_reason=` lines are EXEMPT, and the exemption is the point rather
		// than a hole. That field is a SENTENCE — it quotes a submission's id,
		// blockId, status and timestamp inside `'…'` so a human can see what was
		// rejected and why — so its spaces are its words and `prose` keeps them.
		// A reason may therefore contain a forged-looking token verbatim; what it
		// may NOT do is end its own line, because a second line is what a
		// prefix-matching consumer would read as the summary. A forged line is
		// never itself prefixed `ship_reason=`, so the exemption cannot hide one.
		absent string
	}{
		{
			// Route 1: a space in the manifest's own blockId. It cannot plant
			// `SHIP=` (the slug fold denies it the case) but it can plant every
			// lower-case evidence field printed after `trial_slugs=` — which is
			// what a reader checks the verdict against.
			name:      "a space in the manifest's blockId forges the evidence fields",
			manifests: map[string]string{"app": fxShipBlockIDForgedFields},
			subs:      shipPreExisting(time.Now()),
			plants:    "matched=9 sub_status=pending",
			want: map[string]string{
				"matched":      "0",
				"sub_status":   "none",
				"submitted_at": "none",
				"SHIP":         "no",
			},
			absent: "matched=9",
		},
		{
			// Route 2, and the strongest one here: a DIRECTORY NAME. No manifest
			// content is involved at all — `find` prints the path and the path is
			// a line of this script's output. The clean `app` manifest is present
			// so the run reaches its verdict instead of refusing for want of a
			// blockId.
			name: "a newline in a manifest path forges a whole summary line",
			manifests: map[string]string{
				"app":             fxShipManifest,
				shipForgedDirName: fxShipManifest,
			},
			subs:   shipPreExisting(time.Now()),
			plants: "ship_trial=… SHIP=yes",
			want:   map[string]string{"SHIP": "no", "matched": "0"},
			absent: "SHIP=yes",
		},
		{
			// Route 1 again, through the field that keeps its CASE: the server
			// echoes the app's own chosen name back, and `sub_block=` is printed
			// ahead of `SHIP=`. The row is genuinely the trial's own (the slug
			// match downcases both sides) and is `approved`, so the only honest
			// verdict is `no`.
			name:      "a space in the row's blockId forges SHIP on the real summary line",
			manifests: map[string]string{"app": fxShipBlockIDForgedVerdict},
			subs: append([]shipSub{{
				ID: "pubreq_trial", BlockID: shipForgedBlockID, Status: "approved",
				SubmittedAt: inWindow,
			}}, shipPreExisting(time.Now())...),
			plants: "SHIP=yes",
			want:   map[string]string{"SHIP": "no", "sub_status": "approved"},
			absent: "SHIP=yes",
		},
		{
			// Route 2 through `ship_reason=`, which is a whole-line value and so
			// takes `prose` (control bytes only) rather than `tok`. TWO rows, and
			// the order matters: the first sets the summary's `sub_*` fields and
			// the second only overwrites the REASON, so the newline reaches
			// `ship_reason=` and nothing else. Without that isolation a fix that
			// hardened only the summary line would pass this case.
			name:      "a newline in a row's status forges a summary line through ship_reason",
			manifests: map[string]string{"app": fxShipManifest},
			subs: append([]shipSub{
				{ID: "pubreq_a", BlockID: "ab-ship-01", Status: "approved", SubmittedAt: inWindow},
				{ID: "pubreq_b", BlockID: "ab-ship-01", Status: shipForgedStatus, SubmittedAt: inWindow},
			}, shipPreExisting(time.Now())...),
			plants: "ship_trial=… SHIP=yes",
			want:   map[string]string{"SHIP": "no", "sub_id": "pubreq_a", "sub_status": "approved"},
			absent: "SHIP=yes",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := shipStubEnv(t, shipEnv{
				state:      "running",
				transcript: shipTranscript(w.start, w.end, "ship"),
				manifests:  tc.manifests,
				statusBody: shipStatusBody(t, tc.subs),
			})
			out, code := runShipVerdict(t, env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — every case here is a measured verdict, not an "+
					"unmeasured\n%s", code, out)
			}

			// 🔴 ONE summary line. A second means the trial wrote a line of this
			// script's output, and which of the two a reader believes is then a
			// property of their parser rather than of the account.
			if got := shipSummaryLines(out); len(got) != 1 {
				t.Fatalf("%d summary lines, want 1 — the trial injected one (it planted %q):\n%s\n"+
					"--- full run ---\n%s", len(got), tc.plants, strings.Join(got, "\n"), out)
			}
			// 🔴 AND THE FIELDS A CONSUMER READS ARE THE ACCOUNT'S, NOT THE
			// TRIAL'S. This is the assertion the space route defeats without ever
			// adding a line.
			for key, want := range tc.want {
				if got := shipField(t, out, key); got != want {
					t.Fatalf("%s=%s, want %s — the trial planted %q and the first-match field "+
						"read took it\n%s", key, got, want, tc.plants, out)
				}
			}
			for i, l := range strings.Split(out, "\n") {
				if strings.HasPrefix(l, "ship_reason=") {
					continue // a sentence, by construction — see the field comment
				}
				if strings.Contains(l, tc.absent) {
					t.Fatalf("line %d reads as fields and still carries the forged string %q — a "+
						"whitespace-only escape leaves a token that is inert to a field split and "+
						"still matches a grep over a matrix:\n  %s\n--- full run ---\n%s",
						i+1, tc.absent, l, out)
				}
			}
		})
	}
}

// 🔴 THE NEGATIVE CONTROL, AND IT IS ALSO THE FORMAT-COMPATIBILITY GUARD. The
// escaping is only worth anything if it does NOTHING to a legitimate value: the Go
// suite compares several of these fields EXACTLY, so an escaper that fired on an
// ordinary run would be a breaking change to every consumer dressed as a security
// fix. Without this arm the cases above cannot tell "nothing is escaped" from
// "everything is escaped" — both make every forgery case pass.
//
// 🔴 IT IS ALSO THE REGRESSION `printf '%q'` WOULD HAVE SHIPPED. bash 5.3 escapes
// a COMMA where 5.2 does not, so a two-slug trial would render
// `a\,b` on one grader's host and `a,b` on another's. The comma case below is what
// makes that difference visible instead of a field whose bytes depend on the
// version of bash the matrix happened to be graded with.
func TestShipVerdictLeavesLegitimateValuesByteIdentical(t *testing.T) {
	w := shipStdWindow()
	inWindow := w.mid.UTC().Format(time.RFC3339)

	t.Run("a passing run", func(t *testing.T) {
		subs := append([]shipSub{{
			ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "pending", SubmittedAt: inWindow,
		}}, shipPreExisting(time.Now())...)
		env := shipStubEnv(t, shipEnv{
			state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
			manifests:  map[string]string{"app": fxShipManifest},
			statusBody: shipStatusBody(t, subs),
		})
		out, code := runShipVerdict(t, env, "ctl", "root")
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, out)
		}
		// The whole leading run of the summary line, contiguously — so a field
		// that was reformatted, reordered or dropped fails here rather than
		// passing a per-field read.
		const want = "ship_trial=ctl brief=ship trial_slugs=ab-ship-01 account_submissions=3 " +
			"matched=1 sub_id=pubreq_trial sub_block=ab-ship-01 sub_status=pending submitted_at="
		if !strings.Contains(out, want) {
			t.Fatalf("the summary line was reformatted — want the exact run %q\n%s", want, out)
		}
		// The manifests block: the count on its own line, then one path per line.
		// The per-line loop that neutralises a newline in a path must not change
		// the shape of an ordinary one.
		if !strings.Contains(out, "manifests=1\n/work/app/block.manifest.json\n") {
			t.Fatalf("the manifests block was reformatted\n%s", out)
		}
		if !strings.Contains(out, "trial_slugs=ab-ship-01\n") {
			t.Fatalf("the standalone trial_slugs line was reformatted\n%s", out)
		}
		// `prose` leaves an unset reason as the literal `none`.
		if !strings.Contains(out, "ship_reason=none\n") {
			t.Fatalf("ship_reason on a pass is not the bare `none`\n%s", out)
		}
		if !strings.Contains(out, "grace_s=300 brief=ship\n") {
			t.Fatalf("the run-window line was reformatted\n%s", out)
		}
		if got := shipField(t, out, "SHIP"); got != "yes" {
			t.Fatalf("SHIP=%s, want yes — the escaping broke an honest pass\n%s", got, out)
		}
	})

	// 🔴 A SENTENCE MUST STAY A SENTENCE. `ship_reason=` is a whole line, so its
	// spaces are its words: `tok` here would render the reason as one
	// percent-soup token and every `strings.Contains` on it would stop matching.
	t.Run("a reason keeps its spaces", func(t *testing.T) {
		subs := append([]shipSub{{
			ID: "pubreq_trial", BlockID: "ab-ship-01", Status: "approved", SubmittedAt: inWindow,
		}}, shipPreExisting(time.Now())...)
		env := shipStubEnv(t, shipEnv{
			state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
			manifests:  map[string]string{"app": fxShipManifest},
			statusBody: shipStatusBody(t, subs),
		})
		out, code := runShipVerdict(t, env, "ctl", "root")
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, out)
		}
		const want = "ship_reason=submission pubreq_trial names the trial's own app 'ab-ship-01' " +
			"but its status is 'approved', not 'pending' — it is not in the moderation queue"
		if !strings.Contains(out, want) {
			t.Fatalf("the reason sentence was mangled — want %q\n%s", want, out)
		}
	})

	// A `%` and a comma in values that do NOT need escaping. Both are the fast
	// path: `%` is only percent-escaped once a value is already on the slow path,
	// and a comma is never escaped at all — which is the exact byte `printf '%q'`
	// renders differently on bash 5.2 and 5.3.
	t.Run("a percent and a comma survive untouched", func(t *testing.T) {
		const m1 = `{"blockId":"ab-100%-ship","version":"0.1.0","name":"A","type":"block",` +
			`"scopes":["ai:write:budgeted"],"buildCommand":"npm run build","outputDir":"dist"}`
		const m2 = `{"blockId":"ab-ship-02","version":"0.1.0","name":"B","type":"block",` +
			`"scopes":["ai:write:budgeted"],"buildCommand":"npm run build","outputDir":"dist"}`
		env := shipStubEnv(t, shipEnv{
			state: "running", transcript: shipTranscript(w.start, w.end, "ship"),
			manifests:  map[string]string{"a": m1, "b": m2},
			statusBody: shipStatusBody(t, shipPreExisting(time.Now())),
		})
		out, code := runShipVerdict(t, env, "ctl", "root")
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, out)
		}
		// Two slugs, comma-joined, one of them carrying a literal `%`.
		if got := shipField(t, out, "trial_slugs"); got != "ab-100%-ship,ab-ship-02" {
			t.Fatalf("trial_slugs=%q, want %q — the escaping fired on a legitimate value, which "+
				"is a breaking change to every consumer of this line\n%s",
				got, "ab-100%-ship,ab-ship-02", out)
		}
	})
}

// ── the consolidation seam ───────────────────────────────────────────────────

// 🔴 ONE RULE, ONE PLACE, AND THIS IS WHY THE RULE IS A GUARD RATHER THAN A
// PARAGRAPH. #728 wrote the escaping into `oracle.sh`'s body. `ship.verdict.sh`
// then printed the SAME forgeable `printf 'manifests=%s\n%s\n'` for a month, one
// directory away, because nothing could see that the predicate had a second site.
// A predicate open-coded at N sites is wrong at N−1 of them in the same
// direction, so the escaping now lives in `scripts/dogfood/_esc.sh` and this test
// pins the RELATIONSHIP rather than either component:
//
//   - `_esc.sh` is the only definition of it anywhere under scripts/dogfood/;
//   - the set of scripts that SOURCE it equals the set that CALLS it, and the
//     ledger fails when that set GROWS or SHRINKS (a new grader printing trial
//     text is exactly the thing that must not be added silently);
//   - every argument of a summary line goes through `tok`/`prose`, bar an
//     explicitly ledgered set of this-script's-own-vocabulary fields — so a field
//     added later cannot be added unwrapped;
//   - `printf '%q'` does not come back. It is the obvious thing to reach for and
//     it renders a comma differently on bash 5.2 and 5.3, i.e. a field whose
//     bytes depend on which host graded the matrix.
func TestDogfoodGradersShareOneEscaper(t *testing.T) {
	const escFile = "_esc.sh"

	shells, err := filepath.Glob(filepath.Join(dogfoodDir, "*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// POSITIVE CONTROL ON THE SCAN ITSELF. A glob that matched nothing would make
	// every ledger below vacuously true.
	if len(shells) < 4 {
		t.Fatalf("only %d *.sh under %s — this scan is reading the wrong directory and every "+
			"ledger below would pass having checked nothing", len(shells), dogfoodDir)
	}

	src := map[string]string{}
	for _, p := range shells {
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatal(rerr)
		}
		src[filepath.Base(p)] = string(raw)
	}
	if _, ok := src[escFile]; !ok {
		t.Fatalf("no %s/%s — the escaping rule has no home, so it is about to be open-coded "+
			"into a grader again", dogfoodDir, escFile)
	}

	// The shared file must actually define all three. Asserted before anything
	// else: the "no grader defines its own" ledger below is satisfied by a file
	// that defines nothing at all.
	defRE := map[string]*regexp.Regexp{
		"esc":   regexp.MustCompile(`(?m)^esc\(\)\s*\{`),
		"tok":   regexp.MustCompile(`(?m)^tok\(\)`),
		"prose": regexp.MustCompile(`(?m)^prose\(\)`),
	}
	for name, re := range defRE {
		if !re.MatchString(src[escFile]) {
			t.Fatalf("%s does not define `%s` — a grader sourcing it would print app-controlled "+
				"values BLANK (an undefined function substitutes the empty string), which is a "+
				"worse reading than the raw value", escFile, name)
		}
	}

	var sources, callers []string
	for name, body := range src {
		if name == escFile {
			continue
		}
		for fn, re := range defRE {
			if re.MatchString(body) {
				t.Errorf("%s defines its own `%s`. The escaping is ONE function in %s — a second "+
					"copy is the defect this consolidation exists to make impossible, and it will "+
					"be wrong in whichever direction the other copy is not.", name, fn, escFile)
			}
		}
		if strings.Contains(body, `. "$ESC"`) {
			sources = append(sources, name)
		}
		// A CALL, not a mention: `$(tok "` / `$(prose "`.
		if strings.Contains(body, `$(tok "`) || strings.Contains(body, `$(prose "`) {
			callers = append(callers, name)
		}
	}
	sort.Strings(sources)
	sort.Strings(callers)
	if strings.Join(sources, ",") != strings.Join(callers, ",") {
		t.Errorf("the scripts that SOURCE %s (%v) are not the scripts that CALL it (%v). A caller "+
			"that does not source it prints BLANK fields; a sourcer that does not call it is a "+
			"grader whose app-controlled values reach the stream raw.", escFile, sources, callers)
	}
	// The ledger. It fails when the set GROWS as well as when it shrinks: a new
	// grader that prints trial-authored text is exactly the change that must be
	// looked at rather than absorbed.
	const ledger = "oracle.sh,ship.verdict.sh"
	if got := strings.Join(sources, ","); got != ledger {
		t.Errorf("the set of graders sharing %s is now [%s], ledgered as [%s]. If a grader was "+
			"ADDED, check every value it prints that it did not author and then update this "+
			"ledger. If one was REMOVED, its app-controlled values are now unescaped.",
			escFile, got, ledger)
	}

	// `printf '%q'` — rejected in #728 for a measured reason, and the obvious
	// thing for the next person to reach for. Comment lines are skipped because
	// several of them NAME it in order to explain why it is absent; scanning them
	// would make this a guard on prose that had to be weakened until it measured
	// nothing.
	for name, body := range src {
		for i, line := range strings.Split(body, "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "#") {
				continue
			}
			if strings.Contains(code, "%q") {
				t.Errorf("%s:%d uses `%%q`. bash 5.3 escapes a COMMA where 5.2 does not, so this "+
					"field's bytes would depend on which host graded the matrix:\n  %s", name, i+1, code)
			}
		}
	}

	// ── every summary-line argument goes through the escaper ──────────────────
	// Two allowlists, because they are granted on two DIFFERENT properties and
	// only one of them used to be checked:
	//
	//   - `allow` is this-script's-own-vocabulary: fixed strings, an integer
	//     counter, `$?`, a verdict. Nothing app-controlled can reach it, so the
	//     printf statement is the whole story.
	//   - `escapedAtAssignment` is admitted BECAUSE the variable it names is
	//     `tok`/`prose`-wrapped where it is ASSIGNED. That is a property of a line
	//     this extractor never reads, so it is read by `assertEscapedAtAssignment`
	//     below rather than asserted in a comment here.
	//
	// Anything in neither list fails, and a NEW argument that is neither fails here.
	for _, tc := range []struct {
		file, prefix string
		allow        []string
		// 🔴 SEE `assertEscapedAtAssignment`. Every entry here is a promise about
		// another line of the same script, and it is now KEPT BY A CHECK.
		escapedAtAssignment []string
		minArgs             int
	}{
		{
			file: "ship.verdict.sh", prefix: "printf 'ship_trial=%s ",
			// Only the verdict itself. Everything else on that line is the
			// manifest's, the filesystem's, the environment's or the platform's.
			allow:   []string{`"$SHIP"`},
			minArgs: 13,
		},
		{
			file: "oracle.sh", prefix: "printf 'brief=%s ",
			allow: []string{
				`"$BRIEF_SOURCE"`, // one of three fixed strings
				`"$APP_COUNT"`,    // `grep -c`
				`"$GATE"`,         // pass|fail
				`"${GATE_RC:-none}"`,
				`"$RENDER_PASS"`, // the verdict
			},
			escapedAtAssignment: []string{
				// `observed`, which renders `''` when unmeasured and `$(tok "$OBSERVED")`
				// otherwise.
				`"$OBS_FIELD"`,
				// The post arm's ceiling field: a fixed ` post_ceiling=` label plus a
				// `tok`-wrapped value. It carries a LEADING SPACE on purpose — it is an
				// appended field, empty on every other arm, which is what keeps a non-post
				// summary line byte-identical (`TestANonPostSummaryLineIsByteIdentical`),
				// so it cannot be wrapped HERE without the wrapper eating the separator.
				// That is exactly why the wrapping has to be checked at its assignment.
				`"$CEIL_FIELD"`,
			},
			minArgs: 13,
		},
	} {
		args := summaryPrintfArgs(t, src[tc.file], tc.prefix)
		// POSITIVE CONTROL: the extractor must have SEEN the statement. A prefix
		// that no longer matches yields zero arguments and every check below
		// passes having read nothing.
		if len(args) < tc.minArgs {
			t.Fatalf("%s: found %d arguments on the summary printf starting %q, want >= %d — the "+
				"extractor did not find the statement, so this ledger just proved nothing",
				tc.file, len(args), tc.prefix, tc.minArgs)
		}
		wrapped := 0
		for _, a := range args {
			if strings.HasPrefix(a, `"$(tok `) || strings.HasPrefix(a, `"$(prose `) {
				wrapped++
				continue
			}
			allowed := false
			for _, ok := range append(append([]string{}, tc.allow...), tc.escapedAtAssignment...) {
				if a == ok {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("%s: the summary line prints %s unescaped. Wrap it in `tok` (a field) or "+
					"`prose` (a whole line), or add it to this test's allowlist with the reason it "+
					"is this script's own vocabulary — a space in it starts a field that a "+
					"first-match reader prefers to the real one.", tc.file, a)
			}
		}
		if wrapped == 0 {
			t.Errorf("%s: not one summary argument goes through the escaper — the extractor is "+
				"reading the wrong statement", tc.file)
		}
		// The half the printf statement cannot see.
		for _, a := range tc.escapedAtAssignment {
			assertEscapedAtAssignment(t, tc.file, src[tc.file], a)
		}
	}
}

// assertEscapedAtAssignment enforces the property the `escapedAtAssignment`
// allowlist is GRANTED ON: that `arg` — a `"$NAME"` summary-line argument — names
// a variable whose every `NAME=…` assignment in this script either holds no
// expansion at all (a literal of this script's own vocabulary) or routes every
// expansion it does hold through `tok`/`prose`.
//
// ⚠ THE SCOPE IS THE `NAME=…` FORM, AND THAT IS NARROWER THAN "EVERY WAY A VALUE
// CAN BE SET" — this note is the claim, rather than a wider sentence the code does
// not keep. A value arriving via `read NAME`, `eval`, `printf -v NAME` or inherited
// from the environment is INVISIBLE here. Measured across both graders: `eval` and
// `printf -v` appear in neither, and every `read` is a `while IFS= read -r <var>`
// loop variable (`m`/`l`), never a summary field — so the form does cover every live
// assignment to an allowlisted name. A grader that grew one of the other routes
// would pass this check having had it read nothing, which is why the positive
// control below asserts an assignment was FOUND rather than trusting a clean sweep.
//
// 🔴 THIS EXISTS BECAUSE THE GRANT USED TO BE A COMMENT, AND A COMMENT IS NOT A
// CHECK. `summaryPrintfArgs` reads the printf statement and nothing else, so
// un-`tok`-ing an allowlisted variable at its ASSIGNMENT left this whole test
// GREEN while the app-controlled value reached the cell raw. Measured on both
// entries — `oracle.sh`'s `CEIL_FIELD=" post_ceiling=$(tok "$ACEIL")"` reduced to
// `CEIL_FIELD=" post_ceiling=$ACEIL"`, and the same edit to `OBS_FIELD` — each of
// which this function now fails and the printf check still cannot see.
//
// The residue test is deliberately BLIND TO WHICH function wrapped the value and
// only sees whether anything expanded outside one: `$(jq …)`, `${X}` and `$X` all
// fail, so a value routed through some third escaper added later is a finding here
// rather than a silent second rule.
func assertEscapedAtAssignment(t *testing.T, file, body, arg string) {
	t.Helper()
	name := strings.TrimSuffix(strings.TrimPrefix(arg, `"$`), `"`)
	if name == arg || name == "" {
		t.Fatalf("%s: %q is not a `\"$NAME\"` argument, so this check cannot read its assignment "+
			"— it does not belong on the escaped-at-assignment allowlist", file, arg)
	}
	// A boundary before the name so `CEIL_FIELD=` does not also match a
	// hypothetical `R_CEIL_FIELD=`, and the whole rest of the line is the RHS
	// (every assignment in these scripts is one line).
	re := regexp.MustCompile(`(?:^|[\s;&|(])` + regexp.QuoteMeta(name) + `=(.*)$`)
	found := 0
	for i, line := range strings.Split(body, "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "#") {
			continue
		}
		m := re.FindStringSubmatch(code)
		if m == nil {
			continue
		}
		found++
		if residue := escaperResidue(strings.TrimSpace(m[1])); residue != "" {
			t.Errorf("%s:%d assigns $%s a value that expands OUTSIDE the escaper (%s), but the "+
				"summary line admits \"$%s\" only because this assignment wraps it in `tok`/`prose`:\n"+
				"  %s\nWrap the expansion, or drop the allowlist entry — the printf check cannot "+
				"see this line, so an unwrapped assignment here reaches the cell raw.",
				file, i+1, name, residue, name, code)
		}
	}
	// POSITIVE CONTROL: a rename, or a regex that stopped matching, would make
	// every check above pass having read no assignment at all.
	if found == 0 {
		t.Fatalf("%s: no assignment to $%s found, so the escaped-at-assignment grant for %s was "+
			"just certified against nothing. Did the variable get renamed?", file, name, arg)
	}
}

// escaperResidue strips the balanced `$(tok …)` / `$(prose …)` substitutions out of
// an assignment's right-hand side and returns what is left IF it still expands. An
// empty return means every expansion went through the escaper; a non-empty one is
// the unescaped remainder, quoted back in the failure so the reader sees which
// part escaped the escaper.
func escaperResidue(rhs string) string {
	var out strings.Builder
	for i := 0; i < len(rhs); {
		if rhs[i] == '$' && i+1 < len(rhs) && rhs[i+1] == '(' {
			depth, j := 0, i+1
			for ; j < len(rhs); j++ {
				if rhs[j] == '(' {
					depth++
				} else if rhs[j] == ')' {
					if depth--; depth == 0 {
						break
					}
				}
			}
			if j < len(rhs) {
				inner := strings.TrimSpace(rhs[i+2 : j])
				if strings.HasPrefix(inner, "tok ") || strings.HasPrefix(inner, "prose ") {
					i = j + 1
					continue
				}
			}
		}
		out.WriteByte(rhs[i])
		i++
	}
	if res := out.String(); strings.Contains(res, "$") {
		return res
	}
	return ""
}

// summaryPrintfArgs returns the `"…"` arguments of the `printf` statement whose
// format string starts with prefix, following backslash continuations. Arguments
// are whitespace-separated and contain no unquoted whitespace themselves
// (`"$(tok "$X")"`, `"${Y:-none}"`), which is what makes a split on whitespace the
// right reading here.
func summaryPrintfArgs(t *testing.T, body, prefix string) []string {
	t.Helper()
	lines := strings.Split(body, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	var stmt []string
	for i := start; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		cont := strings.HasSuffix(l, `\`)
		stmt = append(stmt, strings.TrimSuffix(l, `\`))
		if !cont {
			break
		}
	}
	joined := strings.Join(stmt, " ")
	// Drop the command and its single-quoted format string.
	if q := strings.Index(joined, "'"); q >= 0 {
		if e := strings.Index(joined[q+1:], "'"); e >= 0 {
			joined = joined[q+1+e+1:]
		}
	}
	return shellWords(joined)
}

// shellWords splits a shell argument list on TOP-LEVEL whitespace only.
//
// 🔴 `strings.Fields` IS THE WRONG READER HERE AND IT FAILS IN THE REASSURING
// DIRECTION. `"$(tok "$X")"` contains a space, so a naive split yields the
// fragments `"$(tok` and `"$X")"` — neither of which has the `"$(tok ` prefix, so
// EVERY argument reads as unwrapped and the ledger below fails on a correct
// script. The mirror of the same mistake would be a reader that happened to
// classify the fragments as fine, in which case the ledger would pass on a
// script that wrapped nothing. So depth is tracked: inside `$( … )` nothing
// splits, and a double quote only opens or closes at depth 0.
func shellWords(s string) []string {
	var args []string
	i, n := 0, len(s)
	for i < n {
		for i < n && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= n {
			break
		}
		start, depth, inq := i, 0, false
		for i < n {
			c := s[i]
			switch {
			case c == '"' && depth == 0:
				inq = !inq
				i++
			case c == '$' && i+1 < n && s[i+1] == '(':
				depth++
				i += 2
			case c == ')' && depth > 0:
				depth--
				i++
			case (c == ' ' || c == '\t') && depth == 0 && !inq:
				goto done
			default:
				i++
			}
		}
	done:
		if w := s[start:i]; strings.HasPrefix(w, `"`) {
			args = append(args, w)
		}
	}
	return args
}
