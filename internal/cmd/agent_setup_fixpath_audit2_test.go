package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// R-F1 — the PATH-separator refusal is a property of the EMITTED SHELL
// ---------------------------------------------------------------------------
//
// 🔴 READ THIS BEFORE QUOTING ANY TEST BELOW AS REGRESSION COVERAGE, BECAUSE ONLY
// ONE OF THEM CAN BE RED ON A POSIX HOST.
//
// Round 1 refused `strings.IndexAny(dir, "\n\r"+string(os.PathListSeparator))`.
// `os.PathListSeparator` is `':'` on every POSIX GOOS and `';'` on windows, so on
// linux/amd64 the pre-fix predicate and the post-fix predicate are the SAME THREE
// BYTES. The defect is therefore structurally invisible to any behavioural test
// compiled for this host: `:` was already refused here and `;` was already
// accepted here, before and after. Only a windows build can see it behave, and no
// Windows host was available — so the native-Windows runtime shape stays
// UNVERIFIED (see the posture note at the top of agent_setup_fixpath.go).
//
// So the halves are labelled for what they are:
//
//   - TestTheRefusedCharacterSetIsNotDerivedFromTheCompilingPlatform is the
//     REGRESSION guard, and the only one red at the pre-fix tree. It reads what
//     the predicate is DERIVED FROM, which is the only thing that changed here.
//   - TestTheRefusedCharacterSetIsExactlyNewlineReturnAndColon and
//     TestFixPathRefusesTheColonAndAcceptsTheSemicolonOnPOSIX are INVARIANT
//     guards on this host — green before and after. They are kept because they
//     pin the CONTRACT in behaviour rather than in spelling, which is what stops
//     the source scan above being walked around by a local variable; and because
//     the semicolon arm MEASURES the premise the fix rests on rather than
//     asserting it.
//
// 🔴 NOTHING IN THIS FILE REFERENCES A SYMBOL THE FIX INTRODUCES, DELIBERATELY. A
// regression test that only fails to BUILD on pre-change code has not been shown
// to fail; it has been shown not to compile. The whole file therefore compiles at
// the pre-fix tree, where the structural guard goes red on behaviour and the
// invariant guards stay green — which is the matrix, not a build error.

// fixPathSourceFile is the implementation this file reads. It is a path rather
// than a symbol for the reason stated above.
const fixPathSourceFile = "agent_setup_fixpath.go"

// fixPathImplementationCode returns the implementation with comment-only lines
// removed.
//
// 🔴 THE COMMENT STRIP IS LOAD-BEARING. The doc comment on the refusal constants
// NAMES `os.PathListSeparator` in order to record the retraction, and a scan that
// cannot tell a retraction from a use would make documenting the bug impossible —
// which is the shape that gets a correction deleted rather than kept.
func fixPathImplementationCode(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(fixPathSourceFile)
	if err != nil {
		t.Fatalf("reading %s: %v", fixPathSourceFile, err)
	}
	var code []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		code = append(code, line)
	}
	return strings.Join(code, "\n")
}

// TestTheRefusedCharacterSetIsNotDerivedFromTheCompilingPlatform is the
// regression guard for cli#777 round 2's F1.
//
// 🔴 IT ASSERTS A DERIVATION, NOT A VALUE, AND THAT IS THE ONLY THING IT COULD
// ASSERT. The value is identical on this platform before and after the fix, so a
// test on the value cannot see the bug. What changed is WHERE the character set
// comes from: the platform's list separator — what the COMPILING OS splits on —
// versus the separator the emitted POSIX `sh` block splits on, which is the only
// consumer there is.
//
// The two measured consequences of asking the platform, both on a windows build:
// a real npm install path (`C:\Users\…\lib\binaries`) holds no `;`, so it was
// ACCEPTED and the command wrote both startup files, reported `ok:true` and
// exited 0 on a block that can never resolve the name; and `/home/u/a;b` holds a
// `;`, so it was REFUSED for a directory POSIX `sh` accepts.
//
// ⚠ A SPELLING GUARD IS WALKABLE, AND THIS ONE IS NO EXCEPTION — a
// platform-dependent set can be reintroduced through a local variable this scan
// cannot see. That is precisely why the two behavioural guards below exist. What
// this one buys is the thing they structurally cannot: a red at the pre-fix tree
// on the host the work was done on.
func TestTheRefusedCharacterSetIsNotDerivedFromTheCompilingPlatform(t *testing.T) {
	code := fixPathImplementationCode(t)

	for _, banned := range []string{"os.PathListSeparator", "filepath.ListSeparator"} {
		if !strings.Contains(code, banned) {
			continue
		}
		t.Errorf("%s derives a character set from %s in CODE (not in a comment).\n\n"+
			"The block this command writes is POSIX `sh`, which splits PATH on a colon on EVERY "+
			"platform it runs on. %s answers what the COMPILING OS splits on — `';'` on windows — so "+
			"a predicate built from it ACCEPTS a real Windows npm path (no `;` in "+
			"`C:\\Users\\…\\lib\\binaries`, so both startup files are written, `ok:true`, exit 0, on a "+
			"block that can never resolve the name) and REFUSES `/home/u/a;b`, which POSIX `sh` "+
			"accepts.\n\n"+
			"Refuse a colon unconditionally instead. And do NOT reach for a "+
			"`runtime.GOOS != \"windows\"` gate: Git Bash / MSYS is GOOS=windows running a POSIX bash "+
			"that really does read ~/.bash_profile, so gating would break a real population of users "+
			"for whom this feature works.", fixPathSourceFile, banned, banned)
	}

	// 🔴 AND THE GATE THE AUDITOR OFFERED AS AN ALTERNATIVE IS BANNED BY NAME, for
	// the Git Bash / MSYS reason above. `runtime.GOOS` has exactly one legitimate
	// use in this file — the binary's own base name really is platform-dependent
	// (`civitai.exe`) — so the count is pinned rather than the spelling.
	if n := strings.Count(code, "runtime.GOOS"); n != 1 {
		t.Errorf("%s references runtime.GOOS %d time(s) in code, want exactly 1 (cliBinaryBaseName's "+
			"`civitai.exe`).\n\nIf this is a new `runtime.GOOS != \"windows\"` gate on the refusal or on "+
			"the write: that was considered and REFUSED. Git Bash / MSYS reports GOOS=windows while "+
			"running a POSIX bash that genuinely reads ~/.bash_profile, so gating the feature on GOOS "+
			"breaks users it currently works for. The flag stays registered on all platforms.",
			fixPathSourceFile, n)
	}
}

// TestTheRefusedCharacterSetIsExactlyNewlineReturnAndColon pins the SET by
// probing the function, so the source scan above cannot be satisfied by a file
// that simply stopped refusing anything.
//
// ⚠ INVARIANT GUARD ON A POSIX HOST — green before and after the fix, because
// `os.PathListSeparator` is already `':'` here. It is regression coverage on a
// windows build only.
func TestTheRefusedCharacterSetIsExactlyNewlineReturnAndColon(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted block is POSIX `sh`; this probe's windows behaviour was never measured")
	}
	// Every character worth asking about: the three that must be refused, and the
	// ones a platform-keyed predicate got wrong or that a path legitimately holds.
	for _, tc := range []struct {
		name string
		char string
		want bool // true = must be REFUSED
	}{
		{"newline splits the assignment", "\n", true},
		{"carriage return splits the assignment", "\r", true},
		{"colon splits the PATH entry", ":", true},
		{"semicolon is ordinary to POSIX sh", ";", false},
		{"space is ordinary (the block quotes it)", " ", false},
		{"backslash is ordinary to POSIX sh", `\`, false},
		{"asterisk is ordinary (the block quotes it)", "*", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A directory name cannot be created containing a path separator or NUL,
			// but the predicate is reachable without touching the filesystem: point
			// the seam at a path that does not exist and assert on WHICH error comes
			// back. EvalSymlinks runs first, so build the fixture for real wherever
			// the filesystem allows it, and fall back to a synthetic probe otherwise.
			base := t.TempDir()
			bad := filepath.Join(base, "a"+tc.char+"b")
			if err := os.MkdirAll(bad, 0o755); err != nil {
				t.Skipf("NOT A PASS: this filesystem will not hold a directory named %q, so %q was "+
					"not measured: %v", "a"+tc.char+"b", tc.char, err)
			}
			fakeCLIBinDirAt(t, bad)
			exe := filepath.Join(bad, cliBinaryBaseName())
			prev := cliExecutable
			cliExecutable = func() (string, error) { return exe, nil }
			t.Cleanup(func() { cliExecutable = prev })

			got, err := cliBinDirForPATH()
			switch {
			case tc.want && err == nil:
				t.Errorf("cliBinDirForPATH accepted a directory holding %q (%q). A PATH entry in the "+
					"emitted POSIX `sh` block cannot hold that character, so the block would report a "+
					"fix it did not make.", tc.char, got)
			case !tc.want && err != nil:
				t.Errorf("cliBinDirForPATH refused a directory holding %q (%q): %v\n\nPOSIX `sh` "+
					"handles that character in a PATH entry without complaint, so refusing it is a "+
					"false negative — the mirror image of accepting a Windows path.", tc.char, bad, err)
			}
		})
	}
}

// TestFixPathRefusesTheColonAndAcceptsTheSemicolonOnPOSIX drives the two
// characters the platform-keyed predicate got wrong, in opposite directions, and
// MEASURES the premise the fix rests on.
//
// ⚠ INVARIANT GUARD ON A POSIX HOST, for the reason the file header states.
//
// 🔴 THE SEMICOLON ARM IS THE VALUABLE HALF. The whole argument for a hardcoded
// colon is that POSIX `sh` does not split PATH on anything else, so a `;` in a
// directory name is ordinary. That is checked here with a real shell resolving a
// real executable out of a directory called `a;b` — the difference between a
// claim and a measurement.
func TestFixPathRefusesTheColonAndAcceptsTheSemicolonOnPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted block is POSIX `sh`; the refusal's windows behaviour was never measured " +
			"(no Windows host was available) and this test does not pretend to")
	}

	ok := filepath.Join(t.TempDir(), "a;b")
	if err := os.MkdirAll(ok, 0o755); err != nil {
		t.Skipf("NOT A PASS: this filesystem will not hold a directory named %q, so the acceptance "+
			"was not measured: %v", "a;b", err)
	}
	dir, marker := fakeCLIBinDirAt(t, ok)
	exe := filepath.Join(dir, cliBinaryBaseName())
	prev := cliExecutable
	cliExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { cliExecutable = prev })

	got, err := cliBinDirForPATH()
	if err != nil {
		t.Fatalf("cliBinDirForPATH refused %q: %v", dir, err)
	}
	if got != dir {
		t.Fatalf("cliBinDirForPATH returned %q, want %q", got, dir)
	}

	// POSITIVE CONTROL on the premise: a real shell must resolve the binary out of
	// that directory once the emitted block has been sourced. Without this the
	// acceptance above rests on the assertion that `;` is harmless, which is the
	// thing being claimed.
	sh, lerr := exec.LookPath("sh")
	if lerr != nil {
		t.Skipf("NOT A PASS: no `sh` on PATH, so the premise that POSIX `sh` accepts a semicolon in "+
			"a PATH entry was not measured: %v", lerr)
	}
	f := filepath.Join(t.TempDir(), "block.sh")
	if werr := os.WriteFile(f, []byte(pathFixBlock(dir)), 0o644); werr != nil {
		t.Fatal(werr)
	}
	script := ". " + shellSingleQuote(f) + "; command -v " + cliBinaryBaseName() +
		"; " + cliBinaryBaseName()
	out, cerr := exec.Command(sh, "-c", script).CombinedOutput()
	if cerr != nil {
		t.Fatalf("after sourcing the block for %q, `sh` could not resolve and run the CLI: %v\n%s\n\n"+
			"This is the premise the hardcoded colon rests on: POSIX `sh` splits PATH on a colon and "+
			"on nothing else, so a semicolon in a PATH entry is ordinary.", dir, cerr, out)
	}
	if !strings.Contains(string(out), dir) {
		t.Errorf("`command -v` did not resolve the CLI inside %q after sourcing the block.\n%s", dir, out)
	}
	if !strings.Contains(string(out), marker) {
		t.Errorf("the resolved `civitai` was not the fixture in %q (missing marker %q).\n%s",
			dir, marker, out)
	}
}
