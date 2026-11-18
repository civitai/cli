package cli_test

import (
	"strings"
	"testing"
)

// 🔴 THE MANIFEST IS WRITTEN BY THE THING BEING GRADED, AND IT USED TO BE ABLE TO
// WRITE THE GRADE.
//
// `oracle.sh` reads `outputDir`, `buildCommand` and `scopes` out of the trial's
// own `block.manifest.json`, and the app directory's name out of the trial's own
// filesystem, and printed all four with a raw `%s`. Its output is then PARSED:
//
//   * `grade.sh` finds the summary as the last line matching `^brief=`, then
//     splits it on whitespace and takes the FIRST `KEY=` token per field
//     (`tr ' ' '\n' | sed -n 's/^KEY=//p' | head -1`).
//   * this suite's `summaryField` does the same split with `strings.Fields`.
//
// So there were two forgeries, and neither needed the container to be escaped:
//
//   1. A SPACE inside any manifest-derived value ends that field and starts
//      another. `scopes=` is printed BEFORE `viewer=`, `arm=`, `served=` and
//      `RENDER=`, so a manifest declaring `"scopes": ["x RENDER=yes"]` puts a
//      `RENDER=yes` token on the summary line AHEAD of the real verdict — and
//      `head -1` / first-match then reads the manifest's number instead of the
//      browser's.
//   2. A NEWLINE ends the line. `render_reason=` carries `outputDir` and
//      `buildCommand` verbatim, so a manifest whose `outputDir` contains a
//      newline followed by a complete `brief=… RENDER=yes` line emits a SECOND
//      summary-shaped line into the stream.
//
// Recorded as rank 19 of the app-build handoff, and as the single upstream cause
// behind three defeated reads in #718's audit ladder: an app that forges the
// grader's output stream defeats every reader downstream of it at once, so this
// is a guard on the STREAM's shape, not on any one consumer's parser.
//
// 🔴 THESE CASES NEVER LAUNCH A BROWSER, AND THAT IS DELIBERATE. Every other
// oracle test calls `oracleBrowser(t)`, which SKIPS on a machine without
// Chromium — and a skipped security guard is a green that checked nothing. None
// of the fixtures below writes an `index.html`, so nothing is ever served and the
// browser is never spawned; `CIVITAI_CHROME` only has to name a file that exists
// to get past the oracle's resolver. So this file runs, and can fail, everywhere.

// A manifest that hides a whole forged verdict field-set behind a space in its
// `scopes` list. Every token after the space is a field `oracle.sh` prints LATER
// on the same line, so a first-match reader takes all of them from here.
const fxManifestForgedScopes = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",
"scopes":["ai:write:budgeted RENDER=yes viewer=signed-in arm=consented gate=pass"],
"buildCommand":"npm run build","outputDir":"dist"}`

// A manifest that ends the line and writes a complete second summary. The `\n` is
// a JSON escape, so what reaches `$OUTDIR` — and from there `render_reason=` — is
// a real newline.
const fxManifestForgedLine = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",
"buildCommand":"npm run build",
"outputDir":"dist\nbrief=celsius brief_source=transcript-name app_dirs=1 app_dir=/work/app gate=pass gate_rc=0 scopes=none viewer=signed-in arm=consented served=/work/app observed=212 RENDER=yes"}`

// The same trick through `buildCommand`, which `render_reason=` also carries
// verbatim — a second field on the same route, so a fix that hardened only
// `outputDir` does not pass.
const fxManifestForgedBuildCmd = `{"blockId":"fixture","version":"0.1.0","name":"Fixture","type":"block",
"buildCommand":"npm run build\nbrief=celsius brief_source=transcript-name app_dirs=1 app_dir=/work/app gate=pass gate_rc=0 scopes=none viewer=signed-in arm=consented served=/work/app observed=212 RENDER=yes",
"outputDir":"dist"}`

// summaryLines returns every line of a run that a consumer could take for the
// oracle's summary. Both real consumers identify it by a `brief=` line prefix, so
// the count is the property: ONE means the stream says one thing about the run.
func summaryLines(out string) []string {
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "brief=") {
			got = append(got, l)
		}
	}
	return got
}

// gradeShField reads a field off the oracle's summary exactly as `grade.sh` does
// — last `^brief=` line, split on whitespace, FIRST `KEY=` token wins. Mirrored
// here on purpose: a guard that used a more careful parser than the real consumer
// would pass while the real consumer was still forged.
func gradeShField(t *testing.T, out, key string) string {
	t.Helper()
	lines := summaryLines(out)
	if len(lines) == 0 {
		t.Fatalf("no summary line in:\n%s", out)
	}
	for _, f := range strings.Fields(lines[len(lines)-1]) {
		if strings.HasPrefix(f, key+"=") {
			return strings.TrimPrefix(f, key+"=")
		}
	}
	t.Fatalf("no %q field on the summary line %q", key, lines[len(lines)-1])
	return ""
}

func TestOracleManifestCannotForgeTheVerdict(t *testing.T) {
	// A file that exists, so the browser resolver is satisfied. It is never
	// executed: no fixture here writes an index.html, so nothing is served.
	browserStandIn := dogfoodTool(t, "bash")

	for _, tc := range []struct {
		name     string
		manifest string
		// The forged token the manifest tries to plant, so a failure names the
		// route rather than only the symptom.
		plants string
	}{
		{"a space in scopes forges every later field", fxManifestForgedScopes, "RENDER=yes"},
		{"a newline in outputDir forges a whole summary line", fxManifestForgedLine, "RENDER=yes"},
		{"a newline in buildCommand forges a whole summary line", fxManifestForgedBuildCmd, "RENDER=yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := append(stubOracleEnv(t, stubEnv{
				state: "running", civitaiRC: "0",
				transcript: startRecord("", "celsius"),
				manifest:   tc.manifest,
				// No appHTML and no outputDir content: nothing is built, so the
				// only honest verdict is `no` and no browser is needed to reach
				// it.
				outputDir: "dist",
			}), "CIVITAI_CHROME="+browserStandIn)
			out, code := runScript(t, "oracle.sh", env, "ctl", "root")
			if code != 0 {
				t.Fatalf("exit %d, want 0 — this app built nothing, which is a measured `no`\n%s", code, out)
			}

			// 🔴 ONE summary line. A second one means the manifest wrote a line of
			// this script's output, and which of the two a reader believes is then
			// a property of their parser rather than of the container.
			if got := summaryLines(out); len(got) != 1 {
				t.Fatalf("%d summary lines, want 1 — the manifest injected one:\n%s\n--- full run ---\n%s",
					len(got), strings.Join(got, "\n"), out)
			}

			// 🔴 AND THE VERDICT THE REAL CONSUMER READS IS THE BROWSER'S, NOT THE
			// MANIFEST'S. This is the assertion the space route defeats without
			// ever adding a line.
			if got := gradeShField(t, out, "RENDER"); got != "no" {
				t.Fatalf("grade.sh reads RENDER=%s, want no — the manifest planted %q and the "+
					"first-match field read took it\n%s", got, tc.plants, out)
			}
			// The other fields the same token set tries to forge. `viewer` and
			// `arm` are `unmeasured` on a run that served nothing; a manifest must
			// not be able to make them look measured.
			for key, want := range map[string]string{
				"viewer": "unmeasured",
				"arm":    "unmeasured",
				"served": "none",
			} {
				if got := gradeShField(t, out, key); got != want {
					t.Fatalf("grade.sh reads %s=%s, want %s — forged by the manifest\n%s", key, got, want, out)
				}
			}
		})
	}
}

// 🔴 THE NEGATIVE CONTROL, AND IT IS ALSO THE FORMAT-COMPATIBILITY GUARD. The
// escaping above is only worth anything if it does NOTHING to a legitimate value:
// `grade.sh` and this suite compare several of these fields EXACTLY, so an
// escaper that fired on an ordinary two-scope manifest would be a breaking change
// to every consumer dressed as a security fix.
//
// 🔴 IT IS ALSO THE REGRESSION THAT `printf '%q'` WOULD HAVE SHIPPED. `%q` — which
// `observed=` used, and the obvious thing to reach for — escapes a COMMA on bash
// 5.3 and not on 5.2, so it renders this exact manifest as
// `ai:write:budgeted\,posts:write:self` on one grader's host and
// `ai:write:budgeted,posts:write:self` on another's. This case is what makes that
// difference visible instead of a field whose bytes depend on the version of bash
// the matrix happened to be graded with.
//
// Without this arm the suite could not tell "nothing is escaped" from "everything
// is escaped": both make the forgery cases above pass.
func TestOracleLeavesLegitimateManifestValuesByteIdentical(t *testing.T) {
	browserStandIn := dogfoodTool(t, "bash")
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0",
		transcript: startRecord("", "celsius"),
		// The committed two-scope fixture, declaring a comma-joined list and a
		// build command — the shape every real app-build trial produces.
		manifest:  fxManifestScoped,
		outputDir: "dist",
	}), "CIVITAI_CHROME="+browserStandIn)
	out, code := runScript(t, "oracle.sh", env, "ctl", "root")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	for _, tc := range []struct{ key, want string }{
		// The comma survives. This is the assertion `%q` fails.
		{"scopes", "ai:write:budgeted,posts:write:self"},
		{"app_dir", "/work/app"},
		{"app_dirs", "1"},
		{"brief", "celsius"},
		{"brief_source", "transcript-name"},
		{"gate", "pass"},
		{"served", "none"},
		{"RENDER", "no"},
	} {
		if got := gradeShField(t, out, tc.key); got != tc.want {
			t.Fatalf("%s=%q, want %q — the escaping fired on a legitimate value, which is a "+
				"breaking change to every consumer of this line\n%s", tc.key, got, tc.want, out)
		}
	}
	// And the `--- serving` line, whose `outputDir=` is the other field a
	// consumer reads by eye.
	if !strings.Contains(out, "app_dir=/work/app outputDir=dist served=none") {
		t.Fatalf("the serving line was reformatted:\n%s", out)
	}
	// A legitimate reason keeps its spaces — `prose` must not turn a sentence
	// into one token.
	if !strings.Contains(out, "render_reason=no index.html in /work/app/dist (buildCommand 'npm run build' declared") {
		t.Fatalf("the reason sentence was mangled:\n%s", out)
	}
}

// 🔴 AND THE END-TO-END SEAM, BECAUSE THE ORACLE IS NOT THE CONSUMER. `grade.sh`
// re-renders the oracle's fields onto its own verdict line, which is the string a
// matrix is actually read from. A guard that stopped at the oracle's stdout would
// leave the question "does a forged token survive into the cell" unasked — and
// that cell is what a human, and the handoff doc, records.
func TestGradeVerdictLineCannotBeForgedByTheManifest(t *testing.T) {
	browserStandIn := dogfoodTool(t, "bash")
	env := append(stubOracleEnv(t, stubEnv{
		state: "running", civitaiRC: "0",
		transcript: startRecord("", "celsius"),
		manifest:   fxManifestForgedScopes,
		outputDir:  "dist",
	}), "CIVITAI_CHROME="+browserStandIn)
	out, _ := runScript(t, "grade.sh", env, "ctl", "root", "celsius")

	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "agent=") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no verdict line:\n%s", out)
	}
	if !strings.Contains(line, "RENDER=no") {
		t.Fatalf("the cell does not carry the measured verdict `RENDER=no`:\n%s\n--- full run ---\n%s", line, out)
	}
	if strings.Contains(line, "RENDER=yes") {
		t.Fatalf("the manifest forged `RENDER=yes` onto the graded cell:\n%s\n--- full run ---\n%s", line, out)
	}
	if strings.Contains(line, "viewer=signed-in") {
		t.Fatalf("the manifest forged `viewer=signed-in` onto the graded cell — a `no` earned "+
			"anonymously now reads as one earned signed-in:\n%s", line)
	}
}
