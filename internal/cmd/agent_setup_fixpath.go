package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/civitai/cli/internal/ui"
)

// `civitai agent-setup --fix-path` — putting this CLI's own directory on the
// PATH of every LATER shell, by editing the user's shell startup files.
//
// 🔴 THE DEFECT IT CLOSES IS SECOND-ORDER, AND THAT IS WHY IT IS HERE AND NOT IN
// A DOCS PAGE. On a machine where npm's global prefix is not writable, the
// documented remedy installs into a prefix the user owns and prepends it to PATH
// *for the installing shell*. `agent-setup` then writes an AGENTS.md whose every
// row starts with `civitai`, and every later agent session gets a NEW shell.
// Measured over 6 of 6 failing containers (complete enumeration, cli#663/#665):
// the binary sat at `~/.npm-global/bin/civitai` while both `zsh -lic 'civitai
// --version'` and `bash -lc 'command -v civitai'` answered `command not found`.
// On the two graded images NO directory on PATH is writable at all, so no install
// location can fix it — only a shell startup file can.
//
// 🔴 IT IS OPT-IN, AND THE DEFAULT RUN IS BYTE-IDENTICAL TO BEFORE. This command
// is the first thing a new user runs; a run that silently edits `~/.zshenv`
// because it thought it knew better is not a setup step, it is a surprise.
// TestWithoutFixPathNoShellProfileIsEverWritten pins that.
//
// 🔴 AND IT RUNS NO LOGIN SHELL, EVER. Two earlier drafts of this area tried to
// answer "is `civitai` reachable?" by executing a login shell from Go, and each
// was measured wrong in a way the previous fix introduced: a distro idempotence
// sentinel left set so the profile built no PATH at all; then
// `exec.ErrWaitDelay` returned WITH the shell exited 0 and the resolved path
// already on stdout, which the probe scored "not reachable" while discarding
// that answer. See claudedocs/decisions/36-agents-block-per-project.md for the
// full record. The shape of the bug was never any one inversion: it was that a
// PER-MACHINE signal's only consumer was a WRITE INTO A COMMITTED FILE.
//
// 🔴 SO THE SCOPE IS "NO LOGIN-SHELL PROBE FEEDING A COMMITTED FILE", NOT "NO
// PER-MACHINE READ ANYWHERE". Decision 36's rule governs what the `AGENTS.md`
// block may claim, and that is the only thing its evidence measured. It does not
// forbid, say, an `exec.LookPath("zsh")` — a PATH lookup is not a login-shell
// probe, and its consumer here would be a DOTFILE, not a file anyone commits.
// pathFixTargets declines that presence check for a different and stated reason
// (see its own comment); do not re-derive the decision from a prohibition that
// is not there.
//
// So this feature buys its correctness a different way: it is UNCONDITIONAL and
// IDEMPOTENT, and the conditional it emits is evaluated BY THE SHELL, on the
// user's machine, at startup. A redundant guarded block is harmless, which is
// exactly what makes the detection unnecessary — and nothing machine-specific is
// written into AGENTS.md (TestFixPathWritesNoMachinePathIntoAGENTSMD).
//
// 🔴 THE WINDOWS POSTURE, IN ONE PLACE, BECAUSE EVERY EARLIER DRAFT OF IT WAS
// STATED IN SEVERAL PLACES AND AT LEAST ONE OF THEM WAS ALWAYS WRONG.
// `--fix-path` REFUSES on `GOOS=windows`: the flag is still registered on every
// platform (so `--help` and the exit-code contract are uniform), but a run on
// windows gets a `blocked` row, `ok:false` and a non-zero exit, and writes no
// startup file. The project files — `AGENTS.md`, `CLAUDE.md`, the MCP config —
// land exactly as they do without the flag. pathFixTargets is where the refusal
// lives, keyed on the INJECTED `agentEnv.GOOS` rather than on `runtime.GOOS`, so
// the gate is reachable from a test on this host; see pathFixWindowsRefused.
//
// 🔴 THE GATE CHANGES NO BEHAVIOUR, AND THAT IS THE ARGUMENT FOR IT. The
// unconditional colon refusal below already refuses EVERY native-Windows run:
// `os.Executable()` there returns a fully-qualified path, `filepath.Abs` keeps it
// one, and `filepath.Dir` returns `VolumeName(path) + dir`, whose volume on a
// drive-letter path is two bytes ending in `:`
// ($GOROOT/src/internal/filepathlite/path.go `Dir`, and `volumeNameLen` in
// path_windows.go, which returns 2 when `path[1] == ':'`). So the colon is at
// index 1 of every resolved directory and `strings.IndexAny` finds it. What the
// gate replaces is therefore only the REASON: "your directory contains a colon,
// move or symlink this CLI somewhere else" is advice no Windows user can act on,
// because the drive letter is not a thing they can move away from.
//
// 🔴 AND GIT BASH / MSYS CANNOT BE RESCUED BY A CARVE-OUT, WHICH IS WHAT AN
// EARLIER DRAFT OF THIS AREA ASSUMED. MSYS is `GOOS=windows` running a POSIX bash
// that really does read `~/.bash_profile`, so the flag was left ungated to
// protect that population — but the colon refusal already refused all of it, so
// nothing was being protected. Making it work is not a gate: that bash splits
// PATH on `:` and needs the `/c/Users/…` form, so real support means translating
// a Windows path into an MSYS one. That is a separate feature, and it is not
// writable here without a Windows host to measure it on. WSL is unaffected — it
// is a Linux build, so `GOOS=linux`, and nothing below sees it.
//
// ⚠ WHAT A NATIVE-WINDOWS RUN ACTUALLY DOES AT RUNTIME IS STILL UNVERIFIED. No
// Windows host has been available to any round of this work, so every sentence
// here about native Windows — the colon's position included — is DERIVED from the
// build matrix and from the stdlib source named above, not from an executed run.
// Do not promote it to a measurement. The refusal itself IS measured, because it
// is keyed on an injected seam: see
// TestFixPathRefusesWindowsAndWritesNoStartupFile.

// The managed-block markers for a SHELL file. They are shell comments, and they
// are the contract between one run and the next: everything between them is this
// command's to rewrite, everything outside them is the user's and is never
// touched.
//
// 🔴 DELIBERATELY ASCII, UNLIKE THE AGENTS.md MARKERS. Those carry an em dash
// (agent_setup_files.go explains why it is load-bearing there); a shell startup
// file is sourced by `sh`/`dash` on machines whose locale may be anything, so the
// marker that has to round-trip through a profile stays in the portable set.
const (
	pathFixBeginMarker = "# BEGIN civitai cli PATH (managed by: civitai agent-setup --fix-path)"
	pathFixEndMarker   = "# END civitai cli PATH"
)

// cliExecutable is the ONE seam over os.Executable, and it exists for the same
// reason agentEnv.Exists and agentEnv.GOOS do: the behaviour under test is
// "which directory does this CLI live in", and in a `go test` process the honest
// answer is the test binary's directory, which is named `cmd.test` and would
// make every end-to-end case here exercise the refusal rather than the feature.
//
// 🔴 IT IS NOT A DETECTION HOOK. It answers where THIS PROCESS's image is, from
// the kernel (`/proc/self/exe` on Linux), and never runs a shell. Do not grow it
// into something that asks whether the CLI is reachable — that is the per-machine
// probe decision 36 records as abandoned.
var cliExecutable = os.Executable

// cliBinaryBaseName is the name a shell has to resolve for any row in the
// managed block to run. Adding a directory to PATH only helps if the file in it
// is called this, which is why cliBinDirForPATH refuses anything else rather
// than writing a block that cannot work.
func cliBinaryBaseName() string {
	if runtime.GOOS == "windows" {
		return "civitai.exe"
	}
	return "civitai"
}

// pathEntrySplitChar is the character the EMITTED BLOCK's shell splits PATH on,
// and pathEntryRefusedChars is the set this command refuses a resolved directory
// for.
//
// 🔴 THE SET IS NOT A COMPLETENESS CLAIM, AND SAYING IT WAS is cli#777 round 3's
// F6. This doc comment used to call it "every character a resolved directory may
// not contain if that block is to work", and the user-facing message below used
// to say a newline "would split the assignment itself". Both are false, and they
// are false in the direction that matters — a stated reason a reader can check
// and find wrong is what gets a correct guard deleted. The directory is rendered
// by shellSingleQuote, so a newline or a carriage return sits INSIDE a
// single-quoted word and splits nothing. Measured in bash 5.3.15 against a
// fixture binary, on a PATH asserted to hold no `civitai` first, with a plain
// directory name as the positive control: `a<LF>b` → the block resolves and runs
// the CLI; `a<CR>b` → resolves and runs; `a:b` → NOTFOUND. Only the colon is a
// defect. `\n` and `\r` stay refused as PATHOLOGICAL rather than as
// unrepresentable — a login-file path holding either is far likelier to be a
// corrupted `HOME` than an intended directory, and this command writes a file the
// user's shell executes. ⚠ The previous round restated the false reason and
// WIDENED it to `\r`, inside a commit arguing that a false negative is a defect;
// this is the correction, not a third justification.
//
// 🔴 THESE ARE PROPERTIES OF THE EMITTED SHELL, NOT OF THE COMPILING PLATFORM —
// AND KEYING THEM TO THE PLATFORM WAS THE BUG. The block this command writes is
// POSIX `sh` (pathFixBlock's comment says so three times, and the bash target can
// be `~/.profile`, which `dash` reads). POSIX `sh` splits PATH on `:` on EVERY
// platform it runs on. So the only question that matters is what the emitted
// shell splits on, never what `runtime.GOOS` splits on.
//
// The refusal used `os.PathListSeparator` until cli#777 round 2. That constant is
// `';'` on windows ($GOROOT/src/os/path_windows.go), so the predicate asked the
// wrong platform and was wrong in BOTH directions:
//
//   - `C:\Users\me\AppData\Roaming\npm\node_modules\@civitai\cli\lib\binaries`
//     contains no `;`, so it was ACCEPTED on a Windows build: both startup files
//     were written with `civitai_cli_dir='C:\Users\…'`, `ok:true`, exit 0 and
//     "Open a NEW shell" — the confident false fix this refusal exists to close,
//     since `\` is not a separator to the `sh` that reads the file.
//   - `/home/u/a;b` contains a `;`, so it was REFUSED on a Windows build, for a
//     directory POSIX `sh` accepts without complaint.
//
// 🔴 REFUSING EVERY WINDOWS-STYLE PATH IS THE HONEST OUTCOME, AND IT IS WHAT THE
// BASE-NAME REFUSAL ABOVE ALREADY DOES FOR THE SAME REASON. A drive-letter colon
// now refuses, which is correct: `sh` would split `C:\…` into `C` and `\…`, so
// the block could never make the name resolve, and a refusal naming the reason
// beats a file the user then trusts.
//
// 🔴 AND THE REFUSAL IS NOT WHERE WINDOWS IS DECIDED — pathFixTargets IS. Round 2
// left this predicate as the only thing standing between a Windows run and a
// useless startup file, and recorded "there is deliberately no GOOS gate" here on
// the grounds that gating would break Git Bash / MSYS. That reasoning was wrong
// about its own code: the colon below refuses every Windows path including every
// MSYS one, so the ungated flag protected nobody. Round 3 made the refusal
// explicit at pathFixWindowsRefused, which changes no outcome and replaces an
// unactionable reason with the true one. This predicate stays keyed to the
// EMITTED shell on every platform — see the Windows posture note at the top of
// this file, including that the native-Windows runtime shape is still UNVERIFIED.
const (
	pathEntrySplitChar    = ":"
	pathEntryRefusedChars = "\n\r" + pathEntrySplitChar
)

// cliBinDirForPATH resolves the directory a shell must have on PATH for
// `civitai` to resolve to THIS binary. No shell is run and no PATH is read.
//
// 🔴 THE SYMLINK RESOLUTION IS THE POINT, AND THE npm CASE IS WHY. `@civitai/cli`
// is a wrapper: `<prefix>/bin/civitai` is a symlink to a Node launcher, which
// execs the real Go binary out of `<pkg>/lib/binaries/civitai`. os.Executable()
// inside the Go process therefore reports the LATTER, and that is the right
// answer — the file there is itself named `civitai`, so putting its directory on
// PATH makes the name resolve, bypassing the launcher rather than depending on
// it. EvalSymlinks is what makes a Homebrew install (an opt/Cellar chain) resolve
// to the directory that actually holds the file.
//
// 🔴 A BASE NAME THAT IS NOT `civitai` IS REFUSED, NOT PAPERED OVER. The whole
// claim of this feature is that a NEW shell can resolve the word `civitai`; a
// directory holding `civitai-0.1.2` or `cmd.test` cannot deliver that, so writing
// a block for it would be a confident false fix in a file the user then trusts.
func cliBinDirForPATH() (string, error) {
	exe, err := cliExecutable()
	if err != nil {
		return "", fmt.Errorf("could not resolve this CLI's own location (%w) — "+
			"add the directory holding `civitai` to your shell profile by hand", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("could not resolve %s to a real file (%w) — "+
			"add the directory holding `civitai` to your shell profile by hand", exe, err)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("could not make %s absolute (%w)", resolved, err)
	}
	if base, want := filepath.Base(abs), cliBinaryBaseName(); base != want {
		return "", fmt.Errorf("this binary is installed as %q, not %q (%s) — "+
			"putting its directory on PATH would not make `civitai` resolve, so nothing was written; "+
			"rename or symlink it to %q and re-run `civitai agent-setup --fix-path`",
			base, want, abs, want)
	}
	dir := filepath.Dir(abs)
	if i := strings.IndexAny(dir, pathEntryRefusedChars); i >= 0 {
		return "", fmt.Errorf("this CLI's directory contains %q, so no PATH block was written: %q — "+
			"%q is what the POSIX `sh` this command emits splits PATH on, so an entry holding one would "+
			"become two directories that do not exist; a newline or a carriage return is refused as "+
			"pathological rather than as unrepresentable (the block single-quotes the directory, so "+
			"either one would survive, but a login file this command wrote is the wrong place to guess "+
			"that such a path was intended) — move or symlink this CLI into a directory whose path has "+
			"none of them, then re-run `civitai agent-setup --fix-path`",
			string(dir[i]), dir, pathEntrySplitChar)
	}
	return dir, nil
}

// shellSingleQuote renders s as a POSIX single-quoted word. A single quote is
// closed, backslash-escaped and reopened, which is the only escape POSIX
// single-quoting has — everything else, spaces and glob metacharacters included,
// is literal inside it.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// pathFixBlock renders the managed block for dir.
//
// 🔴 THE CONDITIONAL IS EVALUATED BY THE SHELL, AT STARTUP, ON THE USER'S
// MACHINE — NOT HERE. That is what replaces the login-shell probe decision 36
// abandoned: this command does not need to know whether dir is already on PATH,
// because the block asks at the only moment the answer is true or false. Running
// `--fix-path` on a machine where the CLI was already reachable therefore writes
// a block that does nothing, which is the correct outcome and costs one `case`.
//
// 🔴 EVERY LINE IS POSIX sh, DELIBERATELY. The bash target below can be
// `~/.profile`, which `sh`/`dash` login shells also read, so `[[ ]]`, arrays and
// `${var/x/y}` are all out. `case` is the portable substring test.
//
// 🔴 THE DIRECTORY GOES THROUGH A VARIABLE AND IS SINGLE-QUOTED. Interpolating it
// straight into the `case` pattern would make a `*` or `[` in the path a glob
// metacharacter; inside `*":$civitai_cli_dir:"*` the expansion is quoted, so its
// content is matched literally whatever it holds.
//
// 🔴 THE `[ -x … ]` GUARD IS WHAT STOPS THIS BLOCK SHADOWING A LATER INSTALL.
// The block pins ONE absolute directory and PREPENDS it. Within a single npm
// prefix that is correct — a re-install lands in the same directory, so updates
// work. Across prefixes it is not: after an nvm node switch or a `--prefix`
// change the global install lands somewhere else while this file still puts the
// OLD directory first, so `civitai --version` reports the stale build
// indefinitely and `npm update -g` looks inert. The prepend-wins mechanism is
// certain from the lines below; how often a user changes prefix was NOT measured,
// so the likelihood here is reasoned, not measured. The guard makes the pin
// self-expiring: the entry appears only while the file it was written for is
// still there, which is also the uninstall case.
//
// ⚠ AND THE CONVERSE, WHICH ROUND 1 DID NOT RECORD: THE GUARD TURNS A LAZY CHECK
// INTO AN EAGER ONE. An unconditional prepend resolved `civitai` whenever the
// file existed at COMMAND-LOOKUP time; the entry now exists only if the file was
// there at SHELL-START time. A directory that materialises later — an autofs/NFS
// mount, a container volume, or the window during `npm install -g @civitai/cli`
// while a shell is sourcing — therefore gets no entry for that shell's whole
// lifetime, where before it began working as soon as the file appeared. The trade
// is deliberate; it is listed under decision 39's WHAT IT DOES NOT REACH so the
// cost is recorded alongside the benefit.
//
// It is `-x` on the FILE, not `-d` on the directory, because a directory that
// survives an uninstall with no `civitai` in it buys a PATH entry that can never
// resolve the name — the same thing the base-name refusal rejects at write time.
// Every line stays POSIX `sh`: `[ ]` not `[[ ]]`, and the expansion is
// double-quoted so a space or a glob character in the path is literal. It is
// clean under `set -u` (the variable is assigned immediately above) and under
// `set -e` (a false `if` condition is not an error, and both `case` arms and the
// `unset` exit 0).
func pathFixBlock(dir string) string {
	q := shellSingleQuote(dir)
	return pathFixBeginMarker + "\n" +
		"# Added by the Civitai CLI so a NEW shell can run `civitai`.\n" +
		"# Re-run `civitai agent-setup --fix-path` to refresh this block, or delete\n" +
		"# everything from BEGIN to END to remove it. Lines outside the markers are yours.\n" +
		"civitai_cli_dir=" + q + "\n" +
		"if [ -x \"$civitai_cli_dir/" + cliBinaryBaseName() + "\" ]; then\n" +
		"  case \":$PATH:\" in\n" +
		"    *\":$civitai_cli_dir:\"*) ;;\n" +
		"    *) PATH=\"$civitai_cli_dir:$PATH\" ; export PATH ;;\n" +
		"  esac\n" +
		"fi\n" +
		"unset civitai_cli_dir\n" +
		pathFixEndMarker + "\n"
}

// mergePathFixBlock applies the same three-case rule mergeAgentsMD does, to a
// shell startup file: create, append, or replace BETWEEN the markers. Everything
// outside the markers is returned byte for byte.
//
// 🔴 A SECOND BLOCK IS REFUSED, AND A HALF BLOCK IS REFUSED. These are somebody's
// login files. Two blocks means only the first would ever be refreshed, so the
// other keeps exporting whatever PATH it was written with — and a lone marker
// means the file was hand-edited across the boundary and this command cannot tell
// which surrounding bytes were its own. Guessing in a file that decides what
// every future shell can run is not a repair.
func mergePathFixBlock(path, existing, block string) (string, fileAction, error) {
	begins := strings.Count(existing, pathFixBeginMarker)
	ends := strings.Count(existing, pathFixEndMarker)
	if begins > 1 || ends > 1 {
		return "", "", fmt.Errorf(
			"%s contains %d civitai PATH blocks (%d BEGIN, %d END) — only the first would ever be "+
				"refreshed, so the rest would keep exporting a stale directory; delete all but one "+
				"(everything from a BEGIN to its END is this command's to rewrite) and re-run "+
				"`civitai agent-setup --fix-path`", path, max(begins, ends), begins, ends)
	}
	if existing == "" {
		return block, actionCreate, nil
	}
	begin := strings.Index(existing, pathFixBeginMarker)
	end := strings.Index(existing, pathFixEndMarker)
	switch {
	case begin < 0 && end < 0:
		// Append, with one blank line in front and every existing byte kept.
		return strings.TrimRight(existing, "\n") + "\n\n" + block, actionAppend, nil
	case begin >= 0 && end > begin:
		head := existing[:begin]
		tail := existing[end+len(pathFixEndMarker):]
		// The marker line's own newline is inside `block`, so drop a single
		// leading newline from the tail to avoid growing a blank line per run.
		//
		// 🔴 CRLF FIRST, AND IT IS NOT COSMETIC. On a file saved with CRLF the
		// tail begins `"\r\n"`, so trimming only `"\n"` leaves the `"\r"` as a
		// line of its own — and a lone-CR line is a COMMAND to a shell: bash
		// answers `$'\r': command not found`, dash and zsh likewise, at every
		// shell start, forever, in a file this command wrote.
		//
		// 🔴 REACHABILITY IS A PROPERTY OF THE FILE, NOT OF THE PLATFORM — and
		// this comment used to cite the platform ("`.goreleaser.yaml` builds
		// windows amd64 and arm64, and nothing gates `--fix-path` on GOOS"), which
		// round 3's Windows refusal makes false. The guard is not weakened: what
		// has to be CRLF is the USER'S EXISTING startup file, and a POSIX user
		// acquires one of those from a dotfiles repo edited on Windows, a shared
		// mount, or any editor that saves CRLF — no Windows run of this CLI is
		// needed, and TestReplacingACRLFSavedBlockLeavesNoLoneCarriageReturn drives
		// exactly that case on this host. The arms are exclusive:
		// trimming both in sequence would eat a blank line the user wrote.
		if strings.HasPrefix(tail, "\r\n") {
			tail = strings.TrimPrefix(tail, "\r\n")
		} else {
			tail = strings.TrimPrefix(tail, "\n")
		}
		updated := head + block + tail
		if updated == existing {
			return existing, actionUnchanged, nil
		}
		return updated, actionReplace, nil
	}
	return "", "", fmt.Errorf(
		"%s contains only one half of the civitai PATH block (BEGIN present: %t, END present: %t, "+
			"in order: %t) — restore the missing marker line or delete the partial block, then re-run "+
			"`civitai agent-setup --fix-path`; this command will not guess where your lines end and its "+
			"own begin", path, begin >= 0, end >= 0, begin >= 0 && end > begin)
}

// pathFixTarget is one startup file this feature writes, and the reason it is
// that file rather than another.
type pathFixTarget struct {
	Path string
	Why  string
}

// pathFixWindowsRefused is the whole Windows posture as one predicate: on
// `GOOS=windows` there is no startup file for this command to write, and saying
// so is the refusal.
//
// 🔴 IT READS `env.GOOS`, NOT `runtime.GOOS`, AND THAT IS THE LOAD-BEARING PART.
// `agentEnv.GOOS` is this package's house seam for exactly this (its own doc
// comment records why: a per-OS path is otherwise untestable on one machine), and
// it is already a parameter of pathFixTargets and planPathFix. Keyed to the
// injected value, the refusal is REACHABLE from a test on a POSIX host —
// TestFixPathRefusesWindowsAndWritesNoStartupFile drives it in both directions.
// Keyed to `runtime.GOOS` it would be unreachable here and could only ever be
// asserted by reading the source for a spelling, which is the guard shape round 3
// deleted: a source scan for `runtime.GOOS` was measured to be walked around by a
// gate written `env.GOOS == "windows"` while catching the other spelling, i.e. it
// pinned a word and not the gate.
//
// 🔴 IT IS CHECKED BEFORE `HOME`, DELIBERATELY. A Windows run with no resolvable
// home has two true refusals available and only one of them is actionable; being
// told to set HOME invites a second run that then refuses for a different reason.
// The platform answer is the one that ends the matter.
//
// The returned error is the OUTER error of pathFixTargets, so the caller turns it
// into exactly one `blocked` row with an empty `path` — the same convention the
// unresolvable-HOME refusal uses, and for the same reason: the user asked for a
// write in as many words, so `ok:true` and exit 0 are the one pair of answers a
// script must not be given.
func pathFixWindowsRefused(env agentEnv) error {
	if env.GOOS != "windows" {
		return nil
	}
	return fmt.Errorf("`--fix-path` is not supported on Windows, so no startup file was written " +
		"(every other step ran): the only files it knows how to edit are POSIX shell startup files — " +
		"`~/.zshenv` and the first of `~/.bash_profile`/`~/.bash_login`/`~/.profile` a bash login shell " +
		"reads — and the block it puts in them is POSIX `sh`, which neither `cmd.exe` nor PowerShell " +
		"reads. A Windows directory could not be used in that block either: `sh` splits PATH on a " +
		"colon, and the drive letter puts one in every absolute Windows path. Git Bash and MSYS are " +
		"in the same position despite running a POSIX bash — that bash wants the `/c/Users/...` form, " +
		"and translating a path into it is not something this command does. Add the directory holding " +
		"`civitai.exe` to your PATH through the Windows environment-variable settings, or run " +
		"`civitai agent-setup --fix-path` inside WSL, where this CLI is a Linux build")
}

// pathFixTargets picks the startup files that make the two commands cli#665's
// closing condition names resolve the CLI.
//
// It refuses before it picks, in two cases and in this order: `GOOS=windows`
// (pathFixWindowsRefused — there is no POSIX startup file to write there), then
// an unresolvable `HOME`. Both are the OUTER error, so each becomes one `blocked`
// row with an empty `path` rather than a per-file refusal.
//
// 🔴 THE TWO SHELLS READ DISJOINT FILES, AND THAT IS THE WHOLE REASON THERE ARE
// TWO ROWS. zsh reads `.zshenv`, `.zprofile`, `.zshrc` and `.zlogin`, and NEVER
// `~/.profile`. bash in LOGIN mode reads the FIRST that exists of
// `~/.bash_profile`, `~/.bash_login`, `~/.profile`, and in that mode never reads
// `.bashrc`. So one file cannot satisfy both, and writing `~/.profile` when a
// `~/.bash_profile` exists satisfies NEITHER — bash stops at the first match.
//
//	zsh  -> ~/.zshenv            — read by EVERY zsh: login or not, interactive or
//	                               not. `.zprofile` would miss `zsh -c`, which is
//	                               how an agent harness most often spawns a
//	                               command, and `.zshrc` would miss it too.
//	bash -> the first EXISTING of ~/.bash_profile, ~/.bash_login, ~/.profile,
//	        and ~/.profile when none exists — because that is the file bash would
//	        then read. `~/.profile` is also what `sh`/`dash` login shells read, so
//	        it is preferred when nothing forces one of the bash-only names.
//
// 🔴 WHAT THIS DOES NOT REACH, STATED SO IT IS NOT ASSUMED — AND THE MECHANISM,
// NOT A VERDICT, BECAUSE TWO EARLIER DRAFTS OF THIS PASSAGE WERE BOTH WRONG.
// `~/.bashrc` is deliberately not written (both probes cli#665's closing
// condition names are login shells), so a bash that reads only `~/.bashrc` never
// READS the block. But reading is not the only way a shell gets the PATH entry:
//
//	A shell does not have to read the edit — it INHERITS the resulting PATH from
//	any ancestor process that did. It misses the entry only when NO ancestor in
//	its chain read a file carrying the block.
//
// That is why a GUI terminal, a VS Code integrated terminal and a `bash -ic`
// agent harness inside a desktop or ssh session normally DO get it: something
// upstream was a login shell. Measured on this host (bash 5.3.15, Debian-shaped
// fixture, `env -i`, each arm controlled against the same fixture with the block
// REMOVED — where all arms are NOTFOUND while `~/.profile` is still read):
//
//	bash -lc 'command -v civitai'                    RESOLVES
//	bash -ic …           from a non-login parent     NOTFOUND
//	bash -c  …           from a non-login parent     NOTFOUND
//	bash -lc "bash -ic 'command -v civitai'"         RESOLVES
//	bash -lc "bash -c  'command -v civitai'"         RESOLVES
//	bash -ic "bash -ic 'command -v civitai'"         NOTFOUND
//
// ⚠ THIS IS THE THIRD DRAFT OF THIS SENTENCE. Draft 1 claimed an interactive
// non-login bash was COVERED because Debian's `~/.profile` sources `~/.bashrc` —
// inverted (that makes a LOGIN bash read `.bashrc`; it does nothing for a shell
// that never reads `.profile`), and retracted in cli#777 round 1. Draft 2, the
// retraction itself, said "nothing can make a shell that never reads `.profile`
// see a PATH edit in `.profile`" — ALSO false, and refuted by row 4 above.
// Neither draft was a logic slip; both reached for a confident universal. So:
// the scope of what is claimed here is the six probes listed, on one host, with
// one bash. No universal is asserted in either direction, and the last two rows
// of the table are why — the same `bash -ic` invocation lands on opposite sides
// depending only on its ancestry.
//
// And a `.zshrc` that ASSIGNS PATH wholesale rather than prepending to it will
// discard what `.zshenv` added — nothing written here can prevent that, and the
// block being idempotent is what makes re-running after such an edit cheap.
//
// 🔴 THE zsh ROW IS WRITTEN UNCONDITIONALLY, AND THAT IS A DELIBERATE CHOICE,
// NOT AN OVERSIGHT. Gating it on whether zsh is installed was considered and
// DECLINED: a machine can gain zsh AFTER setup runs, and a gated write would
// then leave this CLI unreachable from the shell the user later installs — which
// is precisely the failure cli#665 is about. A stray `~/.zshenv` on a machine
// with no zsh is inert; the missing one is not. The cost is asymmetric, so the
// unconditional write wins.
//
// ⚠ AND NOT BECAUSE DECISION 36 FORBIDS A PRESENCE CHECK — it does not; see the
// scope note at the top of this file. `exec.LookPath("zsh")` is a PATH lookup
// rather than a login-shell probe, and its consumer would be a dotfile rather
// than a committed file, so neither half of that rule's evidence reaches it.
// This is a judgement call about robustness, and it is recorded as one so nobody
// "restores" a prohibition to justify it.
func pathFixTargets(env agentEnv) ([]pathFixTarget, error) {
	if err := pathFixWindowsRefused(env); err != nil {
		return nil, err
	}
	home := strings.TrimSpace(env.Home)
	if home == "" {
		return nil, fmt.Errorf("no home directory could be resolved, so there is no shell startup file to " +
			"write — set HOME and re-run `civitai agent-setup --fix-path`, or add the directory holding " +
			"`civitai` to your profile by hand")
	}
	// 🔴 ABSOLUTE, FOR THE REASON runAgentSetup MAKES `--dir` ABSOLUTE. README.md
	// publishes "a path in `--json` is always absolute, whatever `--dir` you
	// passed"; the project rows earn that with one `filepath.Abs` and these rows
	// joined `env.Home` raw, so a RELATIVE `HOME` (measured with `HOME=junkhome`:
	// rows reading `create junkhome/.zshenv`) both broke that contract and created
	// the files under the CLI's working directory instead of a home directory.
	// This runs AFTER the empty check on purpose: `Abs("")` is the working
	// directory, so doing it first would turn "no home" into a silent write into
	// the project.
	absHome, err := filepath.Abs(home)
	if err != nil {
		return nil, fmt.Errorf("could not make the home directory %q absolute (%w), so the startup files "+
			"to write cannot be named — add the directory holding `civitai` to your profile by hand", home, err)
	}
	home = absHome
	exists := env.Exists
	if exists == nil {
		exists = func(p string) bool { _, err := os.Stat(p); return err == nil }
	}

	targets := []pathFixTarget{{
		Path: filepath.Join(home, ".zshenv"),
		Why:  "read by every zsh (login or not, interactive or not)",
	}}

	bashLogin := filepath.Join(home, ".profile")
	why := "the file a bash login shell reads when none of .bash_profile/.bash_login exists; " +
		"sh and dash login shells read it too"
	for _, name := range []string{".bash_profile", ".bash_login"} {
		if p := filepath.Join(home, name); exists(p) {
			bashLogin = p
			why = "a bash login shell reads the first of .bash_profile/.bash_login/.profile that exists, " +
				"and stops there — so ~/.profile would never be read"
			break
		}
	}
	return append(targets, pathFixTarget{Path: bashLogin, Why: why}), nil
}

// pathFixPlan is one startup file's planned outcome, built the way every other
// row in this command is: plan first, write second, so `--dry-run` and the real
// run reach the same verdict on everything the plan decides.
type pathFixPlan struct {
	Path    string
	Content string
	Action  fileAction
	Reason  string
	Err     error
}

// planPathFix renders what `--fix-path` would do, writing nothing.
//
// The outer error is for the cases in which there is no row to build at all: no
// resolvable home. A failure to resolve THIS binary still yields one row per
// target, carrying the refusal, because the paths are known and naming them is
// more useful than a single message with no file attached.
//
// 🔴 IT RETURNS THE DIRECTORY IT USED, AND THE CALLER MUST NOT RE-DERIVE IT.
// cliBinDirForPATH reads the filesystem (EvalSymlinks), so a second call is a
// second measurement that can disagree with the first — and the caller's copy
// feeds the human report, which would then describe a block the rows were not
// built from.
func planPathFix(env agentEnv) ([]pathFixPlan, string, error) {
	targets, err := pathFixTargets(env)
	if err != nil {
		return nil, "", err
	}
	dir, dirErr := cliBinDirForPATH()
	plans := make([]pathFixPlan, 0, len(targets))
	for _, t := range targets {
		if dirErr != nil {
			plans = append(plans, pathFixPlan{Path: t.Path, Err: dirErr})
			continue
		}
		if terr := checkWriteTargetResolvable(t.Path); terr != nil {
			plans = append(plans, pathFixPlan{Path: t.Path, Err: terr})
			continue
		}
		raw, _, rerr := readIfExists(t.Path)
		if rerr != nil {
			plans = append(plans, pathFixPlan{Path: t.Path, Err: rerr})
			continue
		}
		content, action, merr := mergePathFixBlock(t.Path, string(raw), pathFixBlock(dir))
		if merr != nil {
			plans = append(plans, pathFixPlan{Path: t.Path, Err: merr})
			continue
		}
		plans = append(plans, pathFixPlan{
			Path:    t.Path,
			Content: content,
			Action:  action,
			Reason:  pathFixReason(action, dir) + " — " + t.Why,
		})
	}
	return plans, dir, nil
}

// pathFixReason words what happened to one file, from the action, so the human
// report, `--json` and `--dry-run` cannot describe the same run differently.
func pathFixReason(a fileAction, dir string) string {
	switch a {
	case actionCreate:
		return "creates it, putting " + dir + " on PATH at shell startup"
	case actionAppend:
		return "appends a marker-guarded PATH block for " + dir + "; every existing byte is kept"
	case actionReplace:
		return "the managed PATH block is refreshed for " + dir + "; everything outside the markers is untouched"
	default:
		return "the managed PATH block already points at " + dir
	}
}

// pathFixFootprintVerb words what happened to ONE startup file, from THAT file's
// own action. It is the whole implementation of the rule
// printPathFixFootprint's doc comment states, and it is a function so that the
// rule has one place rather than one arm.
//
// 🔴 `unchanged` IS A WRITE THAT DID NOT HAPPEN, AND IT GETS ITS OWN WORD. The
// plan is `unchanged` on every repeat run — the common case, since the block is
// idempotent on purpose — and the write closure returns without touching the
// file. `Wrote` there is a false claim about a file in the user's home directory,
// which is precisely what the footprint exists to report honestly.
//
// `dryRun` only decides the tense of the rows that WOULD be written: a dry run
// over an already-current file would still write nothing, so it stays
// `unchanged` rather than becoming `WOULD write`.
func pathFixFootprintVerb(action string, dryRun bool) string {
	switch action {
	case actionBlocked:
		return "REFUSED   "
	case string(actionUnchanged):
		return "unchanged "
	default:
		if dryRun {
			return "WOULD write"
		}
		return "Wrote     "
	}
}

// pathFixReport is what the human renderer needs to say what was written. It is
// a struct rather than four parameters because every field is meaningless unless
// Requested is true, and a renderer that has to check four flags is a renderer
// that will one day print one of them on a run that did not ask for it.
type pathFixReport struct {
	// Requested is `--fix-path`. Nothing in this struct is printed when it is
	// false, which is what keeps a default run byte-identical.
	Requested bool
	// Dir is the directory the block puts on PATH; "" when it could not be
	// resolved (the rows then carry the refusal).
	Dir string
	// Block is the exact text `--dry-run` prints instead of writing.
	Block string
	// Paths is every startup file the run wrote or would write, in row order.
	Paths []string
}

// pathFixApplied counts the startup-file rows that were NOT refused, read out of
// the rows themselves so the next-step block cannot claim a file the report shows
// as blocked. It is the same "ask the rows" rule `ok`, the exit code, the error
// and the headline already share (blockedRowSummary, agentSetupHeadline).
func pathFixApplied(changes []agentChangeJSON, pf pathFixReport) int {
	want := make(map[string]bool, len(pf.Paths))
	for _, p := range pf.Paths {
		want[p] = true
	}
	n := 0
	for _, c := range changes {
		if want[c.Path] && c.Action != actionBlocked {
			n++
		}
	}
	return n
}

// printPathFixFootprint names every startup file this run touched, and under
// `--dry-run` prints the exact block instead of writing it.
//
// 🔴 A SILENT WRITE OUTSIDE THE PROJECT IS A DEFECT, NOT A TIDINESS QUESTION.
// `developer.civitai.com/agent-setup/prompt.md` step 3 requires the agent to
// relay what was written to the operator; a file the output does not name is a
// file the operator never hears about, in a directory they own. The `changes`
// rows already carry each path — this block exists so the FOOTPRINT is one
// readable list rather than something a reader has to assemble from the rows.
//
// 🔴 IT PRINTS NOTHING WHEN THE FLAG WAS NOT PASSED. That is what keeps a default
// run byte-identical to every release before this one.
//
// 🔴 THE VERB PER FILE IS READ OUT OF THAT FILE'S ROW, NOT FROM `dryRun` ALONE.
// A first draft printed `Wrote <path>` for every path in the report, which on a
// run whose destination was refused claimed a write the rows immediately above it
// showed as `blocked` — the same "headline disagreeing with the report" defect
// agentSetupHeadline records, one surface over.
//
// 🔴 AND THAT SENTENCE WAS WIDER THAN THE CODE UNTIL cli#777 ROUND 1. It read the
// row for `blocked` and for nothing else, so the SECOND `--fix-path` run — plan
// `actionUnchanged`, write skipped by agent_setup.go's `plan.Action ==
// actionUnchanged`, file mtime unmoved — printed `Wrote <path>` three lines under
// a row reading `unchanged`. Mutant M14 repaired the `blocked` arm and nobody
// checked the others; the lie was unguarded in BOTH directions. pathFixFootprintVerb
// is now the one place a verb is chosen, and
// TestThePathFootprintVerbIsReadOutOfEveryFilesOwnRow asserts every action's.
func printPathFixFootprint(w io.Writer, st ui.Styler, changes []agentChangeJSON, pf pathFixReport, dryRun bool) {
	if !pf.Requested {
		return
	}
	fmt.Fprintln(w, "\nPATH:")
	if pf.Dir == "" {
		// 🔴 IT NAMES NO CAUSE, BECAUSE THERE IS MORE THAN ONE AND THIS LINE CANNOT
		// TELL THEM APART. It used to assert "this CLI's own directory could not be
		// resolved", which was the only reason `Dir` could be empty when it was
		// written. Round 3's Windows refusal is a second: there `Dir` is empty
		// because no directory was ever LOOKED FOR, and claiming a failed lookup
		// would be a false statement about what the run did — the same class as the
		// `Wrote <path>` footprint defect this file already records twice. The rows
		// carry the reason, and they are two lines up.
		fmt.Fprintf(w, "  %s\n", st.ErrorMsg("no PATH block was written — the rows above carry the reason"))
		return
	}
	action := map[string]string{}
	for _, c := range changes {
		action[c.Path] = c.Action
	}
	fmt.Fprintf(w, "  %s lives in %s. A marker-guarded block puts that directory on PATH at startup:\n",
		st.Bold("civitai"), pf.Dir)
	for _, p := range pf.Paths {
		switch verb := pathFixFootprintVerb(action[p], dryRun); action[p] {
		case actionBlocked:
			fmt.Fprintf(w, "  %s %s\n", st.ErrorMsg(verb), p)
		case string(actionUnchanged):
			fmt.Fprintf(w, "  %s %s\n", st.Dim(verb), p)
		default:
			fmt.Fprintf(w, "  %s %s\n", verb, p)
		}
	}
	if pathFixApplied(changes, pf) == 0 {
		fmt.Fprintf(w, "  %s\n", st.ErrorMsg("Nothing was written: every startup file above was refused — "+
			"the rows carry the reason, and PATH is unchanged."))
		return
	}
	if dryRun {
		fmt.Fprintf(w, "  %s\n", st.Dim("Nothing was written. The exact block, byte for byte:"))
		for _, line := range strings.Split(strings.TrimRight(pf.Block, "\n"), "\n") {
			fmt.Fprintf(w, "    %s\n", st.Code(line))
		}
		return
	}
	fmt.Fprintf(w, "  %s\n", st.Dim("Re-running is safe: the block is marker-guarded, so a second run replaces "+
		"it rather than adding another, and nothing outside the markers is touched."))
}
