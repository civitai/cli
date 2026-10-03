# `--fix-path` writes the USER's shell startup files, opt-in, and detects NOTHING

`civitai agent-setup --fix-path` appends a marker-guarded block to the user's
shell startup files so that a **new** shell can resolve `civitai`. It is the only
thing in this CLI that writes outside the project directory, and the only thing
that writes a file the user's login shell executes.

Implementation: `internal/cmd/agent_setup_fixpath.go`. Guards:
`internal/cmd/agent_setup_fixpath_test.go` (unit and integration) and
`agent_setup_fix_path_shell_test.go` at the module root (the real binary, real
shells).

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

## The design, and the three bugs it is shaped to avoid

Decision 36 §Fourth records two abandoned drafts of a remedy in this area. Both
DETECTED the condition by running a login shell from Go and asking whether
`civitai` resolved, then wrote the binary's absolute path into the managed
`AGENTS.md` block when it did not. Each was measured wrong in a way the previous
fix introduced:

1. stripping `PATH` alone left NixOS's `__NIXOS_SET_ENVIRONMENT_DONE` idempotence
   sentinel set, so the profile built **no PATH at all** and a correctly
   installed CLI read as unreachable — on the maintainer's own platform;
2. stripping the sentinels fixed that, and a `cmd.WaitDelay` added for an
   unrelated hang introduced a third inversion: a profile that backgrounds a
   daemon returns `exec.ErrWaitDelay` **with the shell exited 0 and the resolved
   path already on stdout**, which the probe scored "not reachable" while
   discarding that answer (measured: 2 s deadline, 30.0 s elapsed, `err=nil`).

The accepted fix then was **deleting the probe**, because the shape of the bug was
never any one inversion: a per-machine signal's only consumer was a **write into
a committed file**. Decision 36's rule — *the `AGENTS.md` block may depend on the
PROJECT, never on the MACHINE* — stands unchanged here.

So this feature pays for its correctness differently:

- **It is unconditional.** It never asks whether `civitai` is already reachable.
- **The conditional it emits is evaluated by the SHELL, on the user's machine, at
  startup**, which is the only moment the answer is true or false:

  ```sh
  civitai_cli_dir='/path/to/bin'
  case ":$PATH:" in
    *":$civitai_cli_dir:"*) ;;
    *) PATH="$civitai_cli_dir:$PATH" ; export PATH ;;
  esac
  unset civitai_cli_dir
  ```

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

### 🔴 WHAT IT DOES NOT REACH

- **`bash -c`** — neither login nor interactive — reads **no** startup file at all
  unless `BASH_ENV` is set. No profile edit can fix that invocation.
- An **interactive non-login bash** reads `~/.bashrc` only. It is covered when
  `~/.profile` sources `.bashrc` (Debian and Ubuntu ship exactly that) and not
  otherwise. `~/.bashrc` is deliberately NOT written: the two probes the closing
  condition names are both satisfied without it, and every extra file is another
  line of someone's login path this CLI owns.
- A **`.zshrc` that ASSIGNS PATH wholesale** rather than prepending will discard
  what `.zshenv` added. Nothing written here can prevent that; the block being
  idempotent is what makes re-running after such an edit cheap.
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
| CONTROL | no mutation | GREEN |

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
- **Only this host's shells were exercised.** zsh and bash were both measured here
  (NixOS); `~/.zshenv` was observed to survive `/etc/zprofile` on this host, which
  is NOT a general claim — a distribution whose system `zprofile` assigns PATH
  wholesale would defeat it, and nothing detects that. The test SKIPS loudly when
  a shell is absent and never passes on an unmeasured probe.
- **macOS was not exercised.** `/etc/zprofile` there runs `path_helper`, which
  rebuilds PATH from `/etc/paths` and `/etc/paths.d` and appends surviving
  entries; the expectation is that the entry survives in a later position, which
  still resolves the name. Expectation, not measurement.
- **Windows is skipped, not supported.** The two probes are POSIX login shells,
  and no PowerShell profile is written.
