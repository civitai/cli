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
	// A newline in a path cannot be expressed in a shell startup file at all: the
	// assignment below would be split across lines and the rest of the block would
	// be read as commands. Refuse rather than emit something unparseable.
	if strings.ContainsAny(dir, "\n\r") {
		return "", fmt.Errorf("this CLI's directory contains a newline, which cannot be written into a "+
			"shell startup file: %q", dir)
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
func pathFixBlock(dir string) string {
	q := shellSingleQuote(dir)
	return pathFixBeginMarker + "\n" +
		"# Added by the Civitai CLI so a NEW shell can run `civitai`.\n" +
		"# Re-run `civitai agent-setup --fix-path` to refresh this block, or delete\n" +
		"# everything from BEGIN to END to remove it. Lines outside the markers are yours.\n" +
		"civitai_cli_dir=" + q + "\n" +
		"case \":$PATH:\" in\n" +
		"  *\":$civitai_cli_dir:\"*) ;;\n" +
		"  *) PATH=\"$civitai_cli_dir:$PATH\" ; export PATH ;;\n" +
		"esac\n" +
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
		tail = strings.TrimPrefix(tail, "\n")
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

// pathFixTargets picks the startup files that make the two commands cli#665's
// closing condition names resolve the CLI.
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
// 🔴 WHAT THIS DOES NOT REACH, STATED SO IT IS NOT ASSUMED. `bash -c` (neither
// login nor interactive) reads NO startup file at all unless `BASH_ENV` is set,
// so no profile edit can fix that invocation. An interactive non-login bash reads
// `~/.bashrc` only; it is covered when `~/.profile` sources `.bashrc` (Debian and
// Ubuntu ship exactly that) and not otherwise. And a `.zshrc` that ASSIGNS PATH
// wholesale rather than prepending to it will discard what `.zshenv` added —
// nothing written here can prevent that, and the block being idempotent is what
// makes re-running after such an edit cheap.
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
	home := strings.TrimSpace(env.Home)
	if home == "" {
		return nil, fmt.Errorf("no home directory could be resolved, so there is no shell startup file to " +
			"write — set HOME and re-run `civitai agent-setup --fix-path`, or add the directory holding " +
			"`civitai` to your profile by hand")
	}
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
func printPathFixFootprint(w io.Writer, st ui.Styler, changes []agentChangeJSON, pf pathFixReport, dryRun bool) {
	if !pf.Requested {
		return
	}
	fmt.Fprintln(w, "\nPATH:")
	if pf.Dir == "" {
		fmt.Fprintf(w, "  %s\n", st.ErrorMsg("this CLI's own directory could not be resolved, so no PATH block "+
			"was written — the rows above carry the reason"))
		return
	}
	action := map[string]string{}
	for _, c := range changes {
		action[c.Path] = c.Action
	}
	verb := "Wrote"
	if dryRun {
		verb = "WOULD write"
	}
	fmt.Fprintf(w, "  %s lives in %s. A marker-guarded block puts that directory on PATH at startup:\n",
		st.Bold("civitai"), pf.Dir)
	for _, p := range pf.Paths {
		if action[p] == actionBlocked {
			fmt.Fprintf(w, "  %s %s\n", st.ErrorMsg("REFUSED   "), p)
			continue
		}
		fmt.Fprintf(w, "  %s %s\n", verb, p)
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
