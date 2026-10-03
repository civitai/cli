# `--fix-path` writes the USER's shell startup files, opt-in, and detects NOTHING

`civitai agent-setup --fix-path` appends a marker-guarded block to the user's
shell startup files so that a **new** shell can resolve `civitai`. It is the only
thing in this CLI that writes outside the project directory, and the only thing
that writes a file the user's login shell executes.

Implementation: `internal/cmd/agent_setup_fixpath.go`. Guards:
`internal/cmd/agent_setup_fixpath_test.go` (unit and integration),
`internal/cmd/agent_setup_fixpath_audit2_test.go` (round 2's F1 guards), and at
the module root `agent_setup_fix_path_shell_test.go` (the real binary, real login
shells) plus `agent_setup_fix_path_multishell_test.go` (the portability table).

> ⚠ **PROPOSED, NOT DONE: THIS FILE IS NOW MOSTLY AN AUDIT LOG, AND THE DECISION
> IT RECORDS IS A MINORITY OF IT.** 338 → 458 → **657** lines (20,416 → 29,576 →
> **43,007** bytes) over two audit rounds, +45% each time, and every byte of that
> growth is this ladder's own record rather than anything about `--fix-path`. The
> sections a reader needs in order to *change the feature safely* are the top
> through WHAT IT DOES NOT REACH, plus the flag's contract — roughly the first
> 340 lines. The three per-round findings-and-mutants sections are **provenance**:
> they answer "was this verified, and how" and are read approximately never by
> someone editing the code. **Suggested split (a separate change — round 2
> deliberately did not perform it):** move `### Mutants…`, `### Round 1 of
> cli#777…` and `### Round 2 of cli#777…` into
> `claudedocs/decisions/39-agent-setup-fix-path-verification.md`, leave a
> one-line pointer, and keep the RETRACTIONS inline where they are — the
> retracted-claim notes are the part that stops a future draft re-deriving a dead
> sentence (F2 is on its **third** draft precisely because that history was
> legible). That lands the decision record near its original size without losing
> anything a reader of this file was actually looking for.

## The defect, measured

`civitai/cli#665`. On a machine where npm's global prefix is not writable,
`developer.civitai.com/agent-setup/prompt.md` §2 correctly tells the agent to
install into a prefix the user owns and prepend it to PATH:

```sh
npm install -g --prefix="$HOME/.npm-global" @civitai/cli
PATH="$HOME/.npm-global/bin:$PATH"
```

That `PATH=` line lasts for the installing shell. Nothing puts
`~/.npm-global/bin` on any login shell's PATH, so afterwards both of the commands
cli#665's closing condition names fail while the binary is right there:

| probe | result |
|---|---|
| `ls ~/.npm-global/bin` | `civitai` |
| `zsh -lic 'civitai --version'` | `command not found` |
| `bash -lc 'command -v civitai'` | (nothing) |

**8 of 8** trials on a non-writable prefix ended that way, across four models, so
it is not model-dependent; **6 of 6** (complete enumeration, measured 2026-09-19)
installed to `~/.npm-global` specifically rather than to the
`--prefix="$HOME/.local"` the prompt documents — which matters, because
`~/.local/bin` is on a **bash** login shell's PATH on the graded images via
`~/.profile`'s existence-conditional block, and `~/.npm-global/bin` is on
nobody's. And on the two graded images **no** directory on PATH is writable at
all, so there is no install location that fixes this; only a startup file can.

**The second-order cost is the one that justifies code rather than prose.** Step 1
of the setup writes an `AGENTS.md` telling every FUTURE agent session to run
`civitai …`, and every future session gets a new shell. So the durable artifact
the setup exists to produce names a binary the setup left unreachable. **0 of 8**
agents connected the failure to the `AGENTS.md` they had just written — the
agents relayed the PATH line (8/8) and 6/8 warned it would not persist, so they
were never the weak link.

## The design, and the bug class it is shaped to avoid

**Decision 36 §Fourth owns that record — read it there.** It holds the two
abandoned login-shell-probe drafts, each measured inversion, and why the accepted
fix was deleting the probe rather than repairing it again. The reusable part is
the shape, not any one inversion: **a per-machine signal whose only consumer was
a write into a committed file.**

That is also the limit of what it governs, and decision 36 now carries a scoping
clause saying so. Nothing here writes a per-machine fact into a committed file.

So this feature pays for its correctness differently:

- **It is unconditional.** It never asks whether `civitai` is already reachable.
- **The conditional it emits is evaluated by the SHELL, on the user's machine, at
  startup**, which is the only moment the answer is true or false:

  ```sh
  civitai_cli_dir='/path/to/bin'
  if [ -x "$civitai_cli_dir/civitai" ]; then
    case ":$PATH:" in
      *":$civitai_cli_dir:"*) ;;
      *) PATH="$civitai_cli_dir:$PATH" ; export PATH ;;
    esac
  fi
  unset civitai_cli_dir
  ```

  The `[ -x … ]` wrapper arrived in cli#777 round 1; see the stale-prefix entry
  under WHAT IT DOES NOT REACH for what it buys and what it does not. It is `-x`
  on the FILE rather than `-d` on the directory because a surviving directory with
  no `civitai` in it would buy a PATH entry that can never resolve the name — the
  same thing the base-name refusal rejects at write time.

- **A redundant block is harmless**, which is exactly what makes the detection
  unnecessary. Running `--fix-path` on a machine where the CLI was already
  reachable writes a block that does nothing, and costs one `case`.
- **Nothing machine-specific reaches `AGENTS.md`.**
  `internal/cmd/agent_setup_path_note_test.go` already forbids a fixed list of
  spellings in the rendered block; `TestFixPathWritesNoMachinePathIntoAGENTSMD`
  adds the RELATIONSHIP — whatever directory THIS run resolved must not appear in
  the `AGENTS.md` it wrote — which holds for a directory nobody has thought of
  yet.

### Where the directory comes from

`os.Executable()` + `filepath.EvalSymlinks`, and no shell.

🔴 **The symlink resolution is load-bearing, and the npm case is why.**
`@civitai/cli` is a wrapper: `<prefix>/bin/civitai` is a symlink to a Node
launcher (`npm/bin/civitai.js`) which execs the real Go binary out of
`<pkg>/lib/binaries/civitai`. `os.Executable()` inside the Go process therefore
reports the LATTER — and that is the right answer, because the file there is
itself named `civitai`, so putting its directory on PATH makes the name resolve
(bypassing the launcher rather than depending on it).

🔴 **A base name that is not `civitai` is REFUSED.** The whole claim is that a new
shell can resolve the word `civitai`; a directory holding `civitai-0.1.2` cannot
deliver that, so a block written for it would be a confident false fix in a file
the user then trusts. The refusal names what it found and exits non-zero.

🔴 **AND SO IS A DIRECTORY HOLDING A COLON.** That is the one character a PATH
entry structurally cannot contain: POSIX `sh` splits PATH on it, so the entry
does not become a bad directory, it becomes TWO directories that do not exist.
The refusal used to cover `\n` and `\r` only, and cli#777 round 1 measured the gap
end to end — a binary at `…/a:b/civitai` wrote both startup files, reported
`Wrote` for each, printed `Open a NEW shell`, emitted `ok: true` and exited 0,
while a fresh `bash -lc 'command -v civitai'` with that block sourced still
answered nothing.

🔴 **THE CHARACTER SET IS A PROPERTY OF THE EMITTED SHELL, NOT OF THE COMPILING
PLATFORM — AND ROUND 1 GOT THAT WRONG.** Round 1 refused
`"\n\r" + string(os.PathListSeparator)`. That constant is `';'` on windows
(`$GOROOT/src/os/path_windows.go`), so the predicate asked what the *compiling
OS* splits on when the only consumer is the *emitted block*, which is POSIX `sh`
and splits on `:` on every platform it runs on. It was wrong in **both**
directions:

| directory | pre-fix verdict on a windows build | correct verdict |
|---|---|---|
| `C:\Users\me\AppData\Roaming\npm\node_modules\@civitai\cli\lib\binaries` | **accepted** — both startup files written with `civitai_cli_dir='C:\Users\…'`, `ok:true`, exit 0, "Open a NEW shell" | refused |
| `/home/u/a;b` | **refused** | accepted — POSIX `sh` does not split on `;` |

The first row is the confident false fix this refusal exists to close, one
platform over: `\` is not a separator to the `sh` that reads the file, so the
block can never make the name resolve. Round 2 therefore refuses `:`
unconditionally (`pathEntryRefusedChars`), and **refusing every Windows-style
path is the honest outcome** — exactly what the base-name refusal above already
does for the same reason, and better than a file the user then trusts.

🔴 **THERE IS DELIBERATELY NO `runtime.GOOS != "windows"` GATE.** It was offered
in round 2's audit as the alternative and it is **unsafe**: Git Bash / MSYS is
`GOOS=windows` running a POSIX bash that genuinely reads `~/.bash_profile`, so
gating would break a real population of users for whom this feature works. The
flag stays registered on all platforms. `TestTheRefusedCharacterSetIsNotDerived`
`FromTheCompilingPlatform` pins both halves — no platform-derived character set,
and `runtime.GOOS` referenced exactly once in that file (the `civitai.exe` base
name).

⚠ **Only one of round 2's three guards here can be red on a POSIX host, and the
reason is worth keeping.** On linux/amd64 `os.PathListSeparator` already *is*
`':'`, so the pre-fix and post-fix predicates are the same three bytes and the
defect is structurally invisible to any behavioural test compiled here. The
regression guard is therefore the **structural** one (it reads what the predicate
is derived from); the two behavioural guards are **invariant** guards on this
host and regression coverage only on a windows build. Measured: structural guard
RED at `667c59c`, the two behavioural guards GREEN at `667c59c`, all three green
at round 2's HEAD.

### Which startup files, and why those

The two shells read **disjoint** file sets, which is the whole reason there are
two rows:

| shell family | file | why |
|---|---|---|
| zsh | `~/.zshenv` | read by **every** zsh — login or not, interactive or not. `.zprofile` and `.zshrc` would both miss `zsh -c`, which is how an agent harness most often spawns a command. zsh never reads `~/.profile`. |
| bash / sh login | the first **existing** of `~/.bash_profile`, `~/.bash_login`, `~/.profile` — and `~/.profile` when none exists | bash in login mode reads the first of those three that exists **and stops**. `~/.profile` is also what `sh`/`dash` login shells read, so it is preferred when nothing forces a bash-only name. |

🔴 **The bash row is the one that can be wrong while looking right.** Always
writing `~/.profile` — the obvious choice, and what a "write the portable file"
instinct produces — is silently ineffective on every machine that has a
`~/.bash_profile`, which is most macOS setups and anything touched by nvm, rbenv
or pyenv. `TestPathFixTargetsFollowTheShellsOwnPrecedence` is the discriminating
test; a mutant that drops the precedence loop is killed by it.

Every line of the block is POSIX `sh`, because the bash target can be
`~/.profile`, which `dash` also reads. `case` is the portable substring test;
`[[ ]]`, arrays and `${var/x/y}` are all out.

The directory goes through a variable and is **single-quoted**. Interpolating it
straight into the `case` pattern would make a `*` or `[` in the path a glob
metacharacter; inside `*":$civitai_cli_dir:"*` the expansion is quoted, so its
content matches literally whatever it holds. A mutant that double-quotes it is
killed by a `$HOME` in the fixture directory.

### `~/.zshenv` is written unconditionally, and presence-detection was DECLINED

`pathFixTargets` emits the zsh row whether or not zsh is installed. Gating it on
a presence check was raised in review and **deliberately declined**; the record
is here because an undocumented divergence reads as an oversight.

**The reasoning is asymmetric cost.** A machine can gain zsh *after* `agent-setup`
runs. A gated write would then leave this CLI unreachable from the shell the user
later installs — which is exactly the failure cli#665 is about, re-created by the
remedy. A stray `~/.zshenv` on a machine that never gets zsh is **inert**: nothing
reads it, and the block it holds is a `case` that no shell evaluates. The missing
file is not inert. So the unconditional write is the more robust of the two.

🔴 **AND IT IS NOT BECAUSE DECISION 36 FORBIDS A PRESENCE CHECK — IT DOES NOT.**
That reason was stated in an earlier draft of this area and it does not hold.
`exec.LookPath("zsh")` is a **PATH lookup, not a login-shell probe**: it runs no
shell, so not one of the inversion channels decision 36 measured (a distro
sentinel, a backgrounding profile, a shell the probe could not run) can reach it.
And its consumer here would be a **dotfile**, not a committed file, so the rule's
own subject — what the `AGENTS.md` block may claim — is not engaged either. The
divergence is a **judgement call about robustness, not a prohibition**, and it is
written down as one so that no later change defends it with a rule that is not
there. Decision 36's scoping clause records the same boundary from its side.

### 🔴 WHAT IT DOES NOT REACH

- **`bash -c`** — neither login nor interactive — reads **no** startup file at all
  unless `BASH_ENV` is set. No profile edit can fix that invocation.
- An **interactive non-login bash** reads `~/.bashrc` only, so it never **reads**
  the block — `~/.bashrc` is deliberately NOT written (the two probes the closing
  condition names are login shells, and every extra file is another line of
  someone's login path this CLI owns). 🔴 **BUT READING IS NOT THE ONLY WAY A
  SHELL GETS THE ENTRY, AND THIS IS THE THIRD DRAFT OF THIS BULLET. The
  mechanism, stated once, instead of a verdict:**

  > A shell does not have to **read** the edit — it **inherits** the resulting
  > PATH from any ancestor process that did. It misses the entry only when **no
  > ancestor in its chain** read a file carrying the block.

  That is how every GUI terminal, VS Code integrated terminal and `bash -ic`
  agent harness inside a desktop or ssh session normally *does* get it: something
  upstream was a login shell.

  **Measured on this host** — bash 5.3.15, Debian-shaped fixture (`~/.profile`
  carries the block and sources `~/.bashrc`; `~/.bashrc` carries nothing), under
  `env -i`, each arm controlled against the same fixture with the block REMOVED,
  where every arm is NOTFOUND while `~/.profile` is still demonstrably read:

  | probe | result |
  |---|---|
  | `bash -lc 'command -v civitai'` | RESOLVES |
  | `bash -ic …` from a non-login parent | NOTFOUND |
  | `bash -c …` from a non-login parent | NOTFOUND |
  | **`bash -lc "bash -ic 'command -v civitai'"`** | **RESOLVES** |
  | `bash -lc "bash -c 'command -v civitai'"` | RESOLVES |
  | `bash -ic "bash -ic 'command -v civitai'"` | NOTFOUND |

  ⚠ **THE TWO DEAD DRAFTS, NAMED SO A FOURTH IS NOT DERIVED.**
  **Draft 1** (shipped, retracted in cli#777 round 1) claimed the case was
  *covered* because Debian's `~/.profile` sources `~/.bashrc` — inverted: that
  makes a **login** bash read `.bashrc` and does nothing for a shell that never
  reads `.profile`. It was wrong in the direction that matters, because it read
  as coverage.
  **Draft 2** (the round-1 retraction, replaced in round 2) said *"nothing can
  make a shell that never reads `.profile` see a PATH edit in `.profile`"* —
  **also false**, and refuted by row 4 of the table above.
  Neither was a logic slip; both reached for a **confident universal**, which is
  the move to stop making here.
  🔴 **So no universal is claimed in either direction. The scope is the six
  probes listed, on one host, with one bash** — and rows 2 and 6 against row 4
  are why: the *same* `bash -ic` invocation lands on opposite sides depending only
  on its ancestry, so "is an interactive non-login bash reached?" has no
  context-free answer. "I could not establish a universal" is the finding.
- A **`.zshrc` that ASSIGNS PATH wholesale** rather than prepending will discard
  what `.zshenv` added. Nothing written here can prevent that; the block being
  idempotent is what makes re-running after such an edit cheap.
- **A LATER INSTALL INTO A DIFFERENT PREFIX IS ONLY HALF-REACHED.** The block pins
  ONE absolute directory and PREPENDS it. Within a single npm prefix that is
  correct — a re-install lands in the same directory, so updates work. Across
  prefixes it is not: after an nvm node switch or a `--prefix` change the global
  install lands elsewhere while the startup file still puts the OLD directory
  first, so `civitai --version` reports the stale build and `npm update -g` looks
  inert. ⚠ **The prepend-wins mechanism is certain from the block; how often a
  user changes prefix was NOT measured — treat the likelihood as reasoned, not
  measured.** cli#777 round 1 added `[ -x "$civitai_cli_dir/civitai" ]` around the
  `case`, which closes the UNINSTALL case completely and the moved-prefix case
  only when the old directory no longer holds the binary (an nvm version
  directory that is still on disk keeps winning). Re-running `--fix-path` from the
  new install is the full repair, and the block is marker-guarded so that is one
  command. Nothing detects the stale state for the user.
- 🔴 **A DIRECTORY THAT APPEARS AFTER THE SHELL STARTS — the `[ -x … ]` guard's
  converse, and round 1 recorded only the benefit.** The guard turns a LAZY check
  into an EAGER one. Before it, the directory was prepended unconditionally and
  `civitai` resolved whenever the file existed at **command-lookup** time; now the
  PATH entry exists only if the file was there at **shell-start** time. So a
  directory that materialises later — an autofs or NFS mount, a container volume,
  or simply the window during `npm install -g @civitai/cli` while a shell is
  sourcing — gets **no entry for that shell's whole lifetime**, where it
  previously began working the moment the file appeared. The trade is deliberate
  (it is what makes the pin self-expiring on uninstall and on a moved prefix) and
  the remedy is the same one command, but it is a real loss and it is now
  written down.
- **A startup file saved wholly in CRLF** is already degraded for a POSIX shell
  before this command touches it (every blank line in it is a CR-only line, which
  bash answers `$'\r': command not found` for). The no-clobber rule preserves
  that. What the replace must not do is ADD such a line out of the old block's own
  terminator — that was a real defect, fixed in round 1, guarded by
  `TestReplacingACRLFSavedBlockLeavesNoLoneCarriageReturn`.
- **`fish`, `csh`, `nushell`** and anything else: not written, not claimed.

## The flag's contract

- **Opt-in. Without it, no startup file is read or written, and the run is
  byte-identical to every release before this one.** This command is the first
  thing a new user runs; a run that silently edits `~/.zshenv` because it thought
  it knew better is not a setup step, it is a surprise.
  `TestWithoutFixPathNoShellProfileIsEverWritten` pins it (and is labelled an
  INVARIANT guard — the defect never violated it).
- **Marker-guarded and idempotent.** `# BEGIN civitai cli PATH (managed by:
  civitai agent-setup --fix-path)` … `# END civitai cli PATH`. Running twice
  leaves exactly one block. Markers are deliberately ASCII, unlike the
  `AGENTS.md` ones (which carry a load-bearing em dash): a shell startup file is
  sourced by `sh`/`dash` on machines whose locale may be anything.
- **A second block, or half a block, is REFUSED rather than repaired.** Two
  blocks means only the first is ever refreshed, so the other keeps exporting
  whatever directory it was written with; a lone marker means the file was
  hand-edited across the boundary and this command cannot tell which surrounding
  bytes were its own. These are someone's login files — guessing is not a repair.
- **Every file it touched is printed**, under a `PATH:` heading as well as in the
  `changes` rows. `prompt.md` step 3 requires the agent to relay what was
  written; a file the output does not name is a file the operator never hears
  about, in a directory they own. A silent write outside the project is a defect.
- **`--dry-run` prints the exact block and writes nothing.** It shares the one
  `if !dryRun` the other three files use — a per-file copy of that guard is how a
  dry run starts lying.
- **`--check --fix-path` is a USAGE ERROR (exit 2).** This is a decision, not an
  omission. `--check` writes nothing and its `--json` shape is a published
  contract with ledgered consumers
  (`internal/cmd/readme_agent_setup_claims_test.go`, and the hosted setup
  prompt). The two ways to honour `--fix-path` there are to write (which breaks
  "writes nothing") or to add a row reporting whether the block is present (which
  puts a MACHINE-dependent row into a contract whose consumers read `ok`, i.e.
  exactly the axis decision 36 abandoned). Both are worse than refusing, so the
  combination is refused by name and `--dry-run` is pointed at.
  `TestFixPathAddsNoRowToTheCheckContract` is the standing guard on the second
  half.
- **An unresolvable `HOME` is a `blocked` row with an empty `path`, not a
  `manual` one.** The user asked for a write in as many words, so a run that
  could not do it must not exit 0. (The `manual` convention — empty path, exit 0 —
  is for a step there was never a file for; this is not that.)

## Red/green and the mutation matrix

**Red at `origin/main` `8753fa7`, green at HEAD.** The regression test is
`TestAfterFixPathANewLoginShellResolvesTheCLI`
(`agent_setup_fix_path_shell_test.go`). It references **no symbol this change
adds**, so at `origin/main` it RUNS rather than failing to compile:

```
--- FAIL: TestAfterFixPathANewLoginShellResolvesTheCLI
    `civitai agent-setup --fix-path` failed: exit status 2
        Error: unknown flag: --fix-path
```

That mattered: the first draft of the guards lived only in `internal/cmd` and
their "red" at the base was a **build failure**, which is not a test shown to
fail. It also explains the file's location — only the real binary can answer what
`os.Executable()` reports for a real `civitai` process, since in a `go test`
process it reports `cmd.test`. The `internal/cmd` tests inject a seam over
`os.Executable` and cover everything a seam can answer.

Both probes were really executed on this host (`measured 2 of 2 probes: zsh -lic
civitai --version | bash -lc command -v civitai`), with a negative control first.

🔴 **"The probe fails" is the WRONG negative control, and it was tried first.** A
maintainer with the CLI installed system-wide gets a probe that SUCCEEDS before
the fix — observed, `civitai 0.1.111` from the developer's own install — which
reads as "the environment already works" and aborts a test whose subject is
unrelated. The control that holds either way is that the **built binary** is what
answers afterwards and did not answer before. For the same reason the build
stamps a unique `-X main.version=…` marker: `bash -lc 'command -v civitai'` prints
a PATH (so the directory is checkable) but `zsh -lic 'civitai --version'` RUNS the
binary and prints a version, which without a marker is satisfied by any `civitai`
on the machine.

### Mutants, each the narrowest expression that can be wrong

| # | mutation | verdict |
|---|---|---|
| M1 | the `case` arm's pattern can never match | KILLED — `put /opt/some/bin on PATH 2 time(s)` |
| M1b | the arm is PRESENT but prepends too (guard inert) | KILLED — `on PATH 2 time(s)` |
| M2 | `case begin < 0 && end < 0:` → `case true:` (always append) | KILLED — `BEGIN markers after two runs` |
| M3 | the bash precedence loop iterates nothing | KILLED — `bash login reads the first of` |
| M4 | the zsh target is `.zprofile`, not `.zshenv` | KILLED — `zsh never reads ~/.profile` |
| M5 | the base-name guard is disabled | KILLED — `would not make \`civitai\` resolve` |
| M6 | the directory is double-quoted, not single-quoted | KILLED — `PATH does not contain` |
| M7 | `if fixPath` → `if true` (the write is not opt-in) | KILLED — `a run WITHOUT --fix-path created …` |
| M8 | `--check --fix-path` is accepted | KILLED — `the refusal does not mention "--check"` (not by the `err == nil` arm: with the refusal removed the run falls through to the CHECK path and returns a *different* error, which is itself worth knowing) |
| M9 | `if !dryRun` → `if true` | KILLED — `--dry-run created` |
| M10 | the write closure always returns nil | KILLED — `created no startup file at all` |
| M11 | the footprint is never printed | KILLED — `the output is missing "PATH:"` |
| M12 | a duplicated block is accepted | KILLED — `contains 2 civitai PATH blocks` |
| M13 | an unresolvable `HOME` is accepted | KILLED — `accepted an unresolvable home` |
| M14 | the footprint's verb ignores a `blocked` row | KILLED — `says \`Wrote \` on a run where every startup file was refused` |
| M15 | the next-step line ignores a `blocked` row | KILLED — `tells the reader to open a new shell` |
| M16 | the atomic write replaces a symlink instead of following it | KILLED — `is no longer a symlink` |
| CONTROL | no mutation | GREEN |

### Round 1 of cli#777 — seven findings, and the mutants for each fix

| # | mutation | verdict |
|---|---|---|
| M17 | the footprint's `unchanged` arm returns a write verb | KILLED — `the footprint line for a "unchanged" row is "Wrote …", which does not carry the verb "unchanged"` (and, end to end, `the PATH footprint says "Wrote …" on a repeat run`) |
| M18 | the footprint's write arms return `unchanged` | KILLED — `the footprint line for a "create" row is "unchanged …", which does not carry the verb "Wrote"` |
| M19 | the PATH list separator is not refused | KILLED — `cliBinDirForPATH accepted "…/a:b"` |
| M20 | the block prepends with no `[ -x … ]` test | KILLED — `the block put … on PATH 1 time(s) while that directory holds no "civitai"` |
| M21 | the test is `-e`, not `-x` | KILLED — `the block prepended …, which holds a NON-executable "civitai"` |
| M22 | the CRLF tail is not trimmed | KILLED — `the merged file holds a line that is a lone carriage return` |
| M23 | the home directory is not made absolute | KILLED — `target "junkhome/.zshenv" is not absolute` |
| M24 | the AGENTS.md block does not name the flag | KILLED — `the "npm" block does not carry "civitai agent-setup --fix-path"` |
| CONTROL | no mutation | GREEN (22 of 22 in the `--fix-path` selection) |

🔴 **M20'S FIRST FORM DIED FOR THE WRONG REASON AND WAS REBUILT.** Deleting only
the `if` line left an orphan `fi`, so every shell rejected the block with a
SYNTAX error — the shell equivalent of a mutant that does not compile. Three
tests went red on `sourcing the block twice failed: exit status 2`, which says
nothing about the guard. The rebuilt mutant removes the `if`/`fi` pair and
re-indents, i.e. it is exactly the block as it shipped before round 1, and then
one test fails on the behaviour.

⚠ **Two fixture repairs were part of these fixes, and both were green-for-the-
wrong-reason risks.** `TestTheEmittedBlockDecidesAtShellStartupNotAtWriteTime`
and `TestPathFixBlockSurvivesShellMetacharactersInTheDirectory` sourced blocks
built for LITERAL paths (`/opt/some/bin`, `/opt/My Apps/bin`) that do not exist.
Against the `[ -x … ]` guard those blocks correctly do nothing, so both tests
would have asserted over a block that never ran. They now build real directories
under `t.TempDir()` — including the `'`, `*`, `[` and `$HOME` shapes — each
holding a real executable `civitai` (`fakeCLIBinDirAt`).

The red/green matrix for round 1, all at base `166ffd4`:

| finding | guard | red at 166ffd4 | green at HEAD |
|---|---|---|---|
| R1 verb | `TestThePathFootprintVerbIsReadOutOfEveryFilesOwnRow`, `TestASecondFixPathRunDoesNotClaimAWriteItDidNotMake` | yes — `"Wrote /home/u/.s4"`; and `Wrote <path>` on a repeat run whose mtime did not move | yes |
| R2 separator | `TestFixPathRefusesADirectoryHoldingThePATHSeparator` | yes — `cliBinDirForPATH accepted "…/a:b"` | yes |
| R4 stale prefix | `TestTheBlockAddsNothingWhenTheDirectoryNoLongerHoldsTheCLI` | yes — `on PATH 1 time(s) while that directory holds no "civitai"` | yes |
| R5 CRLF | `TestReplacingACRLFSavedBlockLeavesNoLoneCarriageReturn` | yes — lone-CR line, and a real `sh` printed `command not found` | yes |
| R6 absolute | `TestPathFixTargetsAreAbsoluteEvenWhenHOMEIsRelative` | yes — `"junkhome/.zshenv" is not absolute` | yes |
| R7 template | `TestEveryBlockTellsYouWhatToDoWhenTheCLIIsNotOnPATH` | yes — block does not carry the flag name | yes |
| R3 inverted claim | prose, swept (see below) | n/a — a sentence, not a behaviour | n/a |

R3 was corrected at both sites it was handed (this file and
`internal/cmd/agent_setup_fixpath.go`) and then SWEPT tree-wide over NORMALISED
text — comment leaders stripped, whitespace collapsed — because the claim wrapped
across `//` lines and no line-based regex can see it. Two differently-shaped
patterns were used (one on the MECHANISM, `profile … sources … bashrc` in either
order; one on the COVERAGE wording plus the distro names that were its evidence),
and both were first shown to HIT a positive-control file carrying the retracted
sentence in both shapes — including in the WRAPPED form, since a bare zero from a
line-based grep would have been worthless. Run over the BASE tree (`166ffd4`) it
found exactly the two sites, and nothing else; run over the fixed tree the only
hits are the three corrected retraction passages (this file, the Go comment, and
the README bullet).

⚠ **THE FILE COUNTS THIS PARAGRAPH USED TO CARRY ("745 files" / "746 files") RESTED
ON AN UNSTATED CORPUS DEFINITION AND ARE NOT REPRODUCIBLE.** Re-measured in round
2, the two refs count as follows, and no filter yields 745/746:

| corpus definition | `166ffd4` | `667c59c` | delta |
|---|---|---|---|
| every tracked path (`git ls-tree -r --name-only`) | 830 | 831 | **+1** |
| `.go .md .yml .yaml .json .sh .ts .js .txt` | 744 | 745 | **+1** |
| `.go .md` only | 651 | 652 | **+1** |

**So quote the DELTA, which is corpus-independent: exactly one file, and it is
`internal/cmd/agent_setup_fixpath_audit1_test.go`** — round 1's own new test file
(`git diff --name-status 166ffd4 667c59c | grep '^A'`). Any absolute total here
must name the filter that produced it, or it rots into a number nobody can check:
the original pair is off by one from the nearest plausible filter and it is no
longer possible to say which corpus it meant.

🔴 **P1 WAS TOO NARROW ON ITS FIRST RUN AND THE SWEEP ITSELF HAD TO BE WIDENED.**
It required whitespace between "sources" and the path, so a backtick-wrapped
`` sources `~/.bashrc` `` did not match — it missed the README bullet written
in this very change. Widening the separator class to include backticks and
quotes found it. The base-tree count above is from the WIDENED pattern, so the
"exactly two sites" claim is not resting on the narrow one.

M14 and M15 are **round 2**: an adversarial read of this change's own output found
that the footprint printed `Wrote <path>` for every path in the report, and told
the reader to open a new shell, on a run whose destinations were all REFUSED and
whose rows said so three lines above — the same "summary contradicting the report
it summarises" class `agentSetupHeadline` exists for. Both now read the rows
(`pathFixApplied`), and `TestAFullyRefusedFixPathRunNeverClaimsAWriteOrANewShell`
is the guard. A review fix resets the verification gate, so the two new guards
were mutated like the rest rather than shipped on the reviewer's authority.

🔴 **M1b SURVIVED on the first sweep, and that was a real defect in the
assertion, not in the code.** The obvious count —
`strings.Count(":"+path+":", ":"+dir+":")` — is **non-overlapping**, so two
ADJACENT duplicates (`dir:dir:rest`, exactly what a broken guard produces) share
the colon between them and count as ONE. The structural half of the same test
(`the emitted block does not contain …`) killed M1 but could not see M1b, because
M1b keeps the asserted literal and changes what the arm DOES. `countPathEntries`
replaced the expression; both mutants are now killed by the behavioural half.
That is the whole value of running the sweep: the guard read as coverage and
provided none for one shape.

### Round 2 of cli#777 — four findings

Base `667c59c`. F2, F3 and F4(b) are prose; F1 is code plus guards and F4(a)
commits the multi-shell harness.

| finding | what changed | guard | red at `667c59c` | green at HEAD |
|---|---|---|---|---|
| F1 predicate | refuse `:` unconditionally (`pathEntryRefusedChars`), drop `os.PathListSeparator` | `TestTheRefusedCharacterSetIsNotDerivedFromTheCompilingPlatform` | **yes** — `agent_setup_fixpath.go derives a character set from os.PathListSeparator in CODE` | yes |
| F1 contract | `:` refused, `;` accepted, `\n`/`\r` refused | `TestTheRefusedCharacterSetIsExactlyNewlineReturnAndColon`, `TestFixPathRefusesTheColonAndAcceptsTheSemicolonOnPOSIX` | **no — INVARIANT guards here** (see below) | yes |
| F1 prose | the Windows posture, stated once in the file header and cited from both passages | prose | n/a | n/a |
| F2 | the inheritance mechanism, at all three sites | prose + the six measured probes | n/a | n/a |
| F3 | the `[ -x … ]` guard's eager/lazy converse | prose | n/a | n/a |
| F4(a) | the multi-shell harness, committed with its control | `TestTheEmittedBlockIsPortableAcrossPOSIXShells` | n/a — new coverage, not a regression | yes (100 assertions, 5 shells) |
| F4(b) | the sweep's corpus figures replaced by a delta | prose | n/a | n/a |

🔴 **ONLY ONE F1 GUARD CAN BE RED ON A POSIX HOST, AND NOT SAYING SO WOULD BE THE
SAME CLASS OF DEFECT F1 IS ABOUT.** `os.PathListSeparator` already *is* `':'` on
linux/amd64, so the pre-fix and post-fix predicates are the **same three bytes**
here: `:` was already refused and `;` already accepted, before and after. No
behavioural test compiled for this host can see the defect. The regression guard
is therefore the **structural** one, which reads what the predicate is derived
from; the two behavioural guards are **invariant** guards on this host and
regression coverage only on a windows build. ⚠ The Windows consequences that
motivated the finding were **not reproduced** — no Windows host was available.

Every test in `internal/cmd/agent_setup_fixpath_audit2_test.go` deliberately
references **no symbol the fix introduces**, so the whole file compiles at
`667c59c` and the matrix above is a test shown to fail rather than a build error.

| # | mutation (narrowest expression) | verdict |
|---|---|---|
| M25 | the colon is dropped from `pathEntryRefusedChars` | KILLED — `cliBinDirForPATH accepted a directory holding ":"` (and round 1's `cliBinDirForPATH accepted "…/a:b"`) |
| M26 | a semicolon is ADDED to `pathEntryRefusedChars` | KILLED — `cliBinDirForPATH refused a directory holding ";"` |
| M27 | `pathEntrySplitChar` is the windows separator `";"` | KILLED — both arms above fire |
| M28 | the set is re-keyed to `string(os.PathListSeparator)` (the exact pre-fix derivation) | KILLED — `derives a character set from os.PathListSeparator in CODE` |
| M29 | a `runtime.GOOS != "windows"` gate is added to the refusal | KILLED — `references runtime.GOOS 2 time(s) in code, want exactly 1` |
| M30 | the block's `case` arm can never match (dedupe inert) | KILLED — `P2: … on PATH 2 time(s) after TWO sources` in **all 5** shells; **no P1/P3/P4 failure**, so the mutation is isolated and the block still parses |
| M31 | the `[ -x … ]` guard removed — `if` **and** `fi` together | KILLED — exactly **25** `P4: … while that directory holds no civitai` (5 shells × 5 shapes) and **zero** P1/P2/P3, which is what proves the uninstall arm is REACHABLE in every shell rather than merely asserted |
| CONTROL | no mutation | GREEN |

🔴 **M31 IS THE ONE THAT MATTERED, AND IT IS M20'S LESSON APPLIED RATHER THAN
RE-LEARNED.** Round 1's first M20 deleted only the `if` line, leaving an orphan
`fi`, so every shell rejected the block with a syntax error — a mutant that does
not compile. M31 removes the `if`/`fi` pair, so the block parses and exactly one
property moves. The 25-and-only-25 count is the isolation evidence: a mutant that
broke the block wholesale would fire P1 and P3 too.

## 🔴 WHAT IS NOT ESTABLISHED

- **This does not close cli#665.** Its closing condition is graded by
  `scripts/dogfood/grade.sh` against `df-node-user` or `df-ubuntu-apt`, from a
  blind model trial. Nothing here is that: the tests measure the same OBSERVABLE
  on this host, with the real binary and the real shells. A green suite is not a
  closed issue.
- **The flag is unreachable to the agents the issue is about until the hosted
  prompt names it.** `agent-setup/prompt.md` lives in
  `civitai/civitai-developer-docs`, and changing it before a CLI release ships
  this flag would tell agents to run a flag the published CLI does not have. That
  sequencing is deliberate and is somebody's next step, not this change's.
- **Only this host's shells were exercised** — five of them, against the block the
  REAL binary writes rather than a Go string literal. 🔴 **THE HARNESS IS NOW
  COMMITTED, AND THAT REPLACED A QUOTED TOTAL WITH A DERIVABLE ONE.**
  `agent_setup_fix_path_multishell_test.go` (module root) builds `./cmd/civitai`,
  copies it into a directory of each shape, runs `--fix-path` from there against a
  throwaway HOME, and extracts the managed block out of the `~/.zshenv` it wrote.
  It prints its own totals, so **read the run, do not quote this paragraph**:

  ```bash
  go test . -run TheEmittedBlockIsPortable -v                       # this host's shells
  nix-shell -p dash mksh busybox coreutils \
    --run 'go test . -run TheEmittedBlockIsPortable -v'             # all five
  ```

  ⚠ **Round 1 ran this matrix from an UNCOMMITTED harness and this bullet quoted
  "109 assertions, 0 failures". That figure was not re-derivable and did not
  follow from its own dimensions** — 5 shells × 5 directory shapes × 4 properties
  is **100**, 109 is prime, and nothing accounted for the other 9. The committed
  harness reports **100 assertions, 0 failures** over bash 5.3.15, dash 0.5.13.5,
  zsh 5.9.2, mksh 59c and busybox ash 1.37.0 — five directory shapes (plain,
  space, `'`, `*`/`[`, `$HOME`) × four properties: one source puts the directory
  on PATH once; two sources still leave one; `set -eu` is clean with **empty
  stderr**; and with the binary moved away the block adds nothing. On this host
  bare (no `nix-shell`) it reports **40 assertions over 2 shells, and names dash,
  mksh and busybox ash in a SKIPPED line** — a run that measured two shells
  cannot be read as a run that measured five.

  🔴 **THE UNGUARDED CONTROL BLOCK IS PART OF THE COMMITTED TEST, AND A CONTROL
  FAILURE IS A FAILURE, NOT A SKIP.** Before any verdict, each shell sources a
  plain unconditional prepend — no `case`, no `[ -x … ]` — which must score
  **1 / 2 / 1** on the three behavioural arms. It is the negative control for all
  three at once: a control that does not duplicate cannot show that the `case` arm
  is what prevents a duplicate, and a control that also adds nothing cannot show
  that `[ -x … ]` is what removes the entry — which is exactly how round 1's first
  harness scored the uninstall arm PASS for every shell it never ran.

  🔴 **AND THE CONTROL IMMEDIATELY EARNED ITS KEEP: mksh COULD NOT BE MEASURED AT
  ALL ON THE FIRST COMMITTED RUN.** `printf %s "$PATH"` is how every probe reads
  its answer, and it is a builtin in bash, dash, zsh and busybox ash but **not in
  mksh**, which execs `/usr/bin/printf`. On NixOS `/usr/bin` and `/bin` hold
  almost nothing, so the harness's deliberately minimal PATH left mksh with no
  `printf` and it exited 127 (`printf: inaccessible or not found`). That is a
  defect in the harness, not in the block — and it surfaced as the **control**
  failing, the one thing that cannot be mistaken for a finding. The harness now
  resolves `printf` and adds its directory. ⚠ **Consequence for the record: round
  1's claim to have measured mksh is NOT reproducible from anything committed, and
  round 2 could not confirm it.** Treat the mksh row as first measured here.

  **The harness was also shown to go RED on real defects, isolated to one arm**
  (round 2 mutants M30/M31 below), which is the positive control the arm count
  alone cannot give.

  `~/.zshenv` was observed to survive `/etc/zprofile` on this host, which is NOT a
  general claim — a distribution whose system `zprofile` assigns PATH wholesale
  would defeat it, and nothing detects that.
- **macOS was not exercised.** `/etc/zprofile` there runs `path_helper`, which
  rebuilds PATH from `/etc/paths` and `/etc/paths.d` and appends surviving
  entries; the expectation is that the entry survives in a later position, which
  still resolves the name. Expectation, not measurement.
- 🔴 **WINDOWS: THE TESTS SKIP IT, THE COMMAND DOES NOT REFUSE IT, AND THE
  RUNTIME SHAPE IS UNVERIFIED.** These are three different claims and an earlier
  draft of this line collapsed them into one. What is true: the two probes above
  are POSIX login shells and no PowerShell profile is written, so there is **no
  test coverage** on Windows. What does **not** follow — and what a Go comment
  cited *this very bullet* to assert until round 2 — is that a Windows run is
  "unsupported either way". The flag is registered on every platform, nothing
  gates it on GOOS, and `.goreleaser.yaml` builds windows amd64 and arm64: a
  Windows run reaches the code, writes `~/.zshenv` and a bash login file, and
  emits POSIX `sh`. It is useful there **to a POSIX shell** — Git Bash / MSYS is
  `GOOS=windows` running a bash that really does read `~/.bash_profile`, and WSL
  is a Linux build — and useless to `cmd.exe` or PowerShell, for which nothing is
  written and nothing is claimed. ⚠ **No Windows host was available to either the
  author or the auditor, so every sentence here about Windows is derived from the
  build matrix and from `os`/`runtime` constants, not from an executed run. Do
  not promote it to a measurement.**

### One property this feature INHERITS rather than states

A dotfiles symlink is the normal way `~/.zshenv` and `~/.profile` are managed —
more so than any MCP config. `writeFileAtomic` already resolves the destination
before renaming (its own comment records the measured defect for
`~/.codex/config.toml`: a rename onto the link destroys it and leaves a regular
file, orphaning the dotfiles copy at rc 0), and refuses a BROKEN link by name.
Reusing `writeProjectFile` is what buys that behaviour here.

🔴 **IT IS GUARDED ON THE HELPER, NOT ON THIS CALLER — and the first draft got
that wrong.** This change shipped a per-caller copy of the proof
(`TestFixPathFollowsASymlinkedStartupFileInsteadOfReplacingIt`), which was a
second copy of `TestASymlinkedConfigIsFollowed`: same fixture shape, same
`Lstat` assertion, nearly the same failure string. The property is identical for
every caller, so a copy per caller grows with the caller set and *still* cannot
report a caller that has NO coverage — `app_init.go`'s did not, and nothing said
so. Replaced by two guards that split the work:

- **behavioural**, on the seam itself —
  `TestWriteProjectFileFollowsASymlinkRatherThanReplacingIt` plus
  `TestWriteProjectFileRefusesABrokenSymlinkByName`
  (`internal/cmd/write_helper_symlink_test.go`). Mutant M16 is killed here now.
- **structural**, on the caller SET — `TestWriteHelperCallersAreLedgered`
  (`writefile_callers_ledger_test.go`) fails when a caller is ADDED or REMOVED,
  and makes every site state what covers it or record plainly that nothing does.

Neither replaces the other: a ledger type-checks past a wrong argument and never
watches a byte get written, which is why the behavioural half is not optional.
