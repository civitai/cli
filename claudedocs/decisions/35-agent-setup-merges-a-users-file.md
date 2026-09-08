# `agent-setup` merges into a file the USER owns

**Item 35.** Code: `internal/cmd/agent_setup_mcp.go` (`blankJSONComments`,
`decodeAgentJSON`, `mergeEntryKeys`, `mergeTOMLBlock`, `parseTOMLBlocks`,
`resolveWriteTarget`, `writeMCPConfig`), `internal/cmd/agent_setup_detect.go`
(`agentRoot`, `PreferExisting`, `AllowsComments`, `agentConfigPath`),
`internal/cmd/agent_setup_files.go` (`agentsMDBlockState`),
`internal/cmd/agent_setup.go` (`runAgentSetupWrite`, `checkCountsTowardVerdict`).
Guards: `internal/cmd/agent_setup_round1_test.go`.

Item 34 is about the one thing this command must never PUT in those files. This
item is about everything else it does to them.

## The thesis

**Every file `agent-setup` writes already belongs to somebody, and most of them
were being damaged in ways that exited 0.**

The original design principle was right and is unchanged — *merge, never
overwrite; refuse rather than repair* — but it was implemented against an
imagined config file rather than a real one. Round 1's audit ran the binary
against files real installs actually have, and six of the seven cases below
produced either a silent deletion or a refusal that blamed the user for a valid
file. None of them failed a test, because every test used a config file this
repo had written itself.

🔴 **That is the lesson worth more than any single fix: a merge routine tested
only against its own output is tested against the one input that cannot break
it.** Fixtures here must come from a stock install — Zed's shipped
`settings.json`, a Codex `config.toml` with timeouts in it, a dotfiles symlink —
not from a previous run of this command.

## The seven measured defects

Each was reproduced by running `./bin/civitai agent-setup`, reading what it
wrote, and re-running after the fix.

### 1. A JSONC config aborted the entire run

Zed ships `~/.config/zed/settings.json` with a four-line `//` comment block.
`encoding/json` fails on the first byte:

```
Error: …/.config/zed/settings.json does not parse (invalid character '/' …)
       — fix or move that file
rc=1        and the project directory is EMPTY
```

**Two independent defects in one symptom**, and both are fixed:

- **JSONC is decoded.** `blankJSONComments` replaces `//` and `/* */` runs with
  spaces before the decode, for the agents whose own parsers accept comments
  (`AllowsComments`: Zed, VS Code, opencode). A strict decode stays strict for
  everyone else — tolerating comments in a file the agent itself rejects would
  let this command write a config that parses here and not there.
- **An MCP refusal degrades instead of aborting.** `planMCPConfig` ran before any
  write, so a config file this command was never going to touch took `AGENTS.md`
  and `CLAUDE.md` down with it. Those files have nothing to do with the MCP
  config. They are now written regardless, and the refusal is reported as its own
  `changes` row.

🔴 **THE COMMENTS ARE STILL LOST ON THE WAY OUT, AND THAT IS SAID RATHER THAN
HIDDEN.** The merge decodes into a Go map and re-encodes, so a JSONC file this
command writes back loses its comments and its key order. `jsonMergeDropsComments`
detects it and the change `reason` states it in the run's own output.

*Rejected: refusing every commented file.* That is what shipped, and it made
`--agent zed` fail for every real Zed install. Losing a comment while saying so
beats refusing while blaming the user — but only if it is actually said.

*Not built, and a genuine option for a later round:* `blankJSONComments`
preserves LENGTH deliberately (a newline inside a `/* */` run stays a newline),
so every byte offset into the blanked text still points at the same place in the
original. That is exactly what a comment-preserving textual splice would need. It
was not built here because the highest-risk path in this command is the one that
rewrites a user's config, and this round already changes it a great deal. **Do
not "simplify" the blanking to `strings.Replace`** — that removes the only
property that makes the splice possible.

### 2. The degraded run's contract

A partial run reports `ok: false`, a `changes` row with the action `blocked`
carrying the refusal, **and exits 1**. Degrading is not pretending.

This retires one invariant deliberately: a write run's payload used to imply
success ("a write run that fails returns an error and emits no payload at all").
The invariant that matters is preserved in the form scripts need — `ok: true`
still means every step happened — and the alternative was writing nothing at all.

### 3. Re-running deleted the header the tool told the user to add

The JSON merge assigned `section[srv.Name] = mcpEntry(…)` and the TOML merge
replaced the whole `[mcp_servers.<name>]` block. So every key the user added to
**our own** entries vanished on the next run — silently, at rc 0, with `--check`
still `ok: true`.

The worst instance is self-inflicted: item 34's no-interpolation branch **tells**
Zed users to add an `Authorization` header to the Civitai entries by hand. The
next run deleted it.

`mergeEntryKeys` (JSON) and `mergeTOMLBlock` (TOML) now overlay only the keys
this command owns. The headers object gets the same treatment one level deeper,
so an unrelated header survives a run that rewrites `Authorization`.

**The precedence rule, since "merge per key" does not say which side wins.** On
the Civitai entries only, this command OWNS `url`, the transport discriminator,
`enabled`, and — *when a token is configured* — `Authorization`. Everything else
on those entries is the user's and is preserved. Two consequences worth stating,
because both look surprising in a diff:

- With **no** token configured, a hand-added `Authorization` on our entry
  survives untouched. That is the finding this section exists for.
- With a token configured, a hand-added `Authorization` on our entry is
  **replaced** by the env-var reference. That is deliberate: it is our entry, the
  reference is what the command exists to write, and if what they wrote was a
  literal token, replacing it removes a credential from a file that gets
  committed. Their `X-Trace`, timeouts and everything else still survive.
- An `"headers": {}` the USER wrote is preserved as-is. Item 34 forbids this
  command from WRITING an empty headers object; it does not license deleting one
  out of somebody's file.

🔴 **The pre-existing guard could not see this.**
`TestAgentSetupDoesNotTouchAUserWrittenLiteralToken` is scoped to a DIFFERENT
server (`their-server`), and other people's servers were never the broken case.
A guard aimed one entry to the left of the defect passes forever.

### 4. The hand-rolled TOML parser refused valid TOML

Two shapes, both legal, both hard refusals telling the user their config was
broken:

| input | old error |
|---|---|
| `matrix = [`⏎`  [1, 2],`⏎`]` | `unterminated table header "[1, 2],"` |
| `[[profiles]]` twice | `table [profiles] is defined twice` |

The first is a header-shaped line inside a multi-line array; the second is an
array of tables, which is the *documented way to write a list* — both `[[…]]`
headers stripped to the same name.

Fixed by tracking **bracket depth** across lines (`tomlBracketDelta`, respecting
strings and comments), so a header is only recognised at depth 0; by recording
`tomlBlock.Array` and exempting `[[…]]` from the duplicate-table rule; and by
stripping a trailing `#` comment before the terminator check, so `[table] # mine`
is a header rather than an unterminated one.

🔴 **THE OLD RESIDUAL COMMENT WAS WRONG ABOUT THE EFFECT, WHICH IS WHY NOBODY
CHASED IT.** It predicted "a mis-placed block boundary rather than data loss".
The measured effect was a refusal. A residual stated more mildly than it behaves
is worse than an unstated one: it reads as already-assessed.

The refusals that must SURVIVE the loosening are pinned separately
(`TestTOMLParserStillRefusesBrokenDocuments`): a genuinely unterminated header,
and a real table defined twice.

### 5. The atomic write replaced a symlinked config with a regular file

`~/.codex/config.toml -> ~/dotfiles/codex.toml` came back as a **regular file**
at exit 0, with the dotfiles copy orphaned and untouched. Every later edit in the
dotfiles repo became invisible to Codex, and version control stopped tracking the
live file. Dotfile repositories are the normal way these files are managed, so
this is the common case.

`resolveWriteTarget` resolves the destination first and the write lands on the
real file, in the real file's directory so the rename stays on one filesystem. A
**broken** symlink is refused by name rather than materialised into a regular
file — the user pointed that path somewhere.

### 6. Config roots the vendor documents

`$HOME` is not always the root, and getting it wrong is silent: the servers land
in a file the agent never opens and `--check` reports "not registered" forever
with nothing to explain it.

- **`CODEX_HOME`** is Codex's documented config directory. It REPLACES
  `$HOME/.codex`, it does not add to it — `TestCodexHomeIsTheConfigRoot` asserts
  the default path is NOT written too, because a run that wrote both would keep
  the default "working" and hide that the override was ignored.
- **Zed on Windows** reads `%APPDATA%\Zed\settings.json`. The release
  cross-compiles windows/amd64 and windows/arm64, so this path ships to users on
  a build no maintainer runs. `agentEnv.GOOS` is injected rather than read from
  `runtime` precisely so it can be exercised from Linux, and the guard checks
  both directions — a Linux user with `APPDATA` set keeps `~/.config/zed`.

**Not done, and stated rather than quietly skipped:** Zed's `XDG_CONFIG_HOME`
handling on Linux was not added. It is plausible and it was not verified, and an
unverified path in this table is the mistake item 34 already records.

### 7. A detection marker that was never a write target

`opencode.jsonc` DETECTS opencode (it is in `agentMarkers`) and was never
something this command would write to. A project carrying only that file was
detected correctly and then had a SECOND file, `opencode.json`, created beside
it — with the servers in the one the user does not maintain.

`PreferExisting` makes an existing alternative spelling win over the default
name. `TestEveryDetectionMarkerIsReachableAsAWriteTarget` is the **ledger** over
the class rather than a second copy of the one case: any marker naming a config
file must be either the target's `Parts` or one of its `PreferExisting`
spellings.

## The `--check` verdict: a gate nobody can clear is a gate everyone ignores

This is the third instance of one shape, and it is now a single predicate,
`checkCountsTowardVerdict`, that both the verdict and the failed-count consult —
so `ok: true` beside "1 check(s) failed" is unreachable.

| row | counts? | why not |
|---|---|---|
| `authenticated` | never | setup stops before auth on purpose (item 34) |
| an absent `Authorization` header | never | the correct state for Zed (item 34) |
| `claude-md` | only for `--agent claude` | **new** |

`CLAUDE.md` exists only because Claude Code does not read `AGENTS.md`. Every
other agent reads `AGENTS.md` directly and the shim is inert for it. Measured: a
Cursor user who deletes the file they were never going to use got `ok: false`,
exit 1, with every other row green — and `developer.civitai.com`'s hosted prompt
reports that as a failed setup.

**The row STAYS.** It is true, and a user who later opens Claude Code in that
directory wants it. What moved is whether it fails the verdict, and the detail
says so, because a red row whose exemption is invisible reads as a bug in the
verdict.

## The managed-block states

`agentsMDBlockState` classifies `AGENTS.md`'s markers into four states, and both
`--check` and the merge read THAT — so a state one refuses cannot be a state the
other calls green. Three of the four have DIFFERENT remedies, which is the whole
reason they are distinct:

- **`blockDuplicated`** (more than one BEGIN, or more than one END) — the replace
  case spans the FIRST begin to the FIRST end, so a second block is never
  refreshed. It keeps instructing the agent with whatever the template said when
  it was written, forever, while `--check` reported `ok: true` because *a* block
  was found. **A stale instruction file that reports healthy is the worst of the
  four states**, and it is reachable by an ordinary copy-paste or a bad merge
  resolution.
- **`blockPartial`, markers out of order** — with END above BEGIN both markers ARE
  present, so the shared refusal read *"BEGIN present: true, END present: true —
  restore both marker lines"* and sent the reader after a marker sitting right in
  front of them. It now says the markers are in the wrong order and to swap them.
- **`blockPartial`, one marker missing** — unchanged, minus the plural: "restore
  the missing marker line".
- **`blockAbsent` / `blockPresent`** — append and replace, as before.

## 0600 on the MCP config, 0644 on the instruction files

Two files created side by side with different modes looks like an oversight. It
is a decision, and the reason on the MCP side had gone stale.

The comment used to justify 0600 with "the entry carries a bearer token whenever
one is configured". **Item 34 made that false** — no entry carries a credential.
The mode stays for a reason that is still true: this file is exactly where item
34's no-interpolation branch tells a Zed user to paste a literal token by hand,
and where an authenticated user's own header will end up. Creating it
world-readable would hand them a 0644 home for a credential they were invited to
add. The strict mode is about what the file is FOR, not about what this command
puts in it.

`AGENTS.md` and `CLAUDE.md` are 0644 because they are instruction files: meant to
be committed, read by the whole team and by CI, incapable of holding a
credential, and a 0600 `AGENTS.md` would break a checkout shared between
accounts. **The split is credential-adjacency, not scope.**

## `--json` paths are absolute

With the default `--dir .` the payload's `path` fields came out relative
(`AGENTS.md`, `.mcp.json`) while the README's documented example shows absolute
ones. A path a consumer cannot resolve without also knowing the CLI's working
directory is not a path. `filepath.Abs` is applied once, to `env.Dir`, from which
every path in the payload is built.

## Red-then-green

Every defect above was reproduced by running the binary on the pre-fix tree
(`6340db8`), fixed, and re-run. The guards were then **mutation-tested**: each
fix was reverted individually — 18 mutants — and every one was killed by the
guard named for it, with a green baseline as the positive control. The matrix is
in the PR that introduced this item.
