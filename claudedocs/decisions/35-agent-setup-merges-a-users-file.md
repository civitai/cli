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
command writes back loses its comments, its trailing commas and its key order.
`jsonMergeDropsFormatting` detects it and the change `reason` states it in the
run's own output.

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
  survives untouched. That is the finding this section exists for. 🔴 **On the
  TOML side that was still false after round 1** — see round 2, defect 1.
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

---

# Round 2

Round 1's fixes were re-audited against the same standard — run the binary
against files real installs have — and five more defects came out of it. The
first is the one that matters most, because **it is the contract round 1
declared fixed**.

## 1. The TOML merge still deleted the user's auth keys

`mergeTOMLBlock` skipped a **fixed** set of owned keys — `{URLKey,
EnvBearerKey, HeadersKey}` — while `renderTOMLServer` emits `http_headers`
**never** (Codex's `EnvHeaderSyntax` is `""`, so `mcpAuthValue` returns `""`) and
`bearer_token_env_var` only **with** a token. So an owned key that was not
re-rendered was skipped with **nothing put back**:

```
BEFORE                                          AFTER            (rc 0, no warning)
[mcp_servers.civitai]                           [mcp_servers."civitai"]
url = "…/mcp"                                   url = "…/mcp"
# I added this by hand so it works:             # I added this by hand so it works:
http_headers = { Authorization = "Bearer …" }   startup_timeout_sec = 30
startup_timeout_sec = 30
```

`http_headers` is the **only** static-header key Codex documents — it is how a
Codex user authenticates by hand — and it was deleted while the comment
introducing it survived, annotating nothing. `--check` still said `ok: true`.

**The fix mirrors the JSON side exactly**: the owned set is now DERIVED from the
rendered lines, so "skip a key" and "write a key" are the same decision.
`mergeEntryKeys` overlays exactly the keys present in `ours`; `mergeTOMLBlock`
now does the same, spelled for lines.

🔴 **The residual, stated.** With a token configured, this writes
`bearer_token_env_var` and leaves a user's `http_headers` `Authorization` in
place — two sources for one header, in a shape a diff will look surprising in.
That is deliberate: item 34's rule is about what this command **writes**, not a
licence to delete what it **finds**, and deleting it silently is defect 1 all
over again.

🔴 **Why the round-1 guard could not see it, and this is the transferable part.**
`TestTOMLMergePreservesKeysOnOurOwnTable`'s docstring said "the same defect on
the Codex path". Its fixture carried `startup_timeout_sec` and
`tool_timeout_sec` — two keys the renderer emits under **no** condition. The
keys that were actually being deleted are the ones the renderer emits
**sometimes**. A fixture built only from never-written keys cannot distinguish
"skip what we re-render" from "skip a fixed list", so the guard read as coverage
and provided none. **Ask which of the states your fixture does not reach.**

## 2. The run described a header it had just preserved as absent

`mcpAuthReason` and `printAgentSetupAuthNote` both derived everything from
`hasToken` and never looked at the merged entry. Measured: run with a token, then
re-run in a shell without one — CI, a second machine, an expired login. The merge
correctly **keeps** `"Authorization": "Bearer ${env:CIVITAI_TOKEN}"`, and the run
then printed:

> No token is configured, so no Authorization header was written.
> ⚠ Access without one: … civitai-orchestration returns 401 until an
> Authorization header is present.

The first sentence is true of the **run** and misleading about the **file**. The
second is a claim about **what the registration reaches**, and it is simply wrong
about the file just written. `--json`'s `reason` said the same.

Both surfaces now read `mcpAuthCoverage`, computed from **the bytes the run will
write** — the one artefact `--dry-run` and the real run agree on by construction,
since `renderMCPConfig` performs no write. The sentence is built once
(`mcpPreservedAuthNote`) so the terminal and `--json` cannot disagree about a
file they are both describing.

🔴 **The residual, stated:** a credential in a shape neither the JSON nor the
TOML reader recognises is reported as **no credential** — the conservative
direction, which is exactly the message that shipped. Conservative is not
correct: a header this code cannot see is still described as absent.

## 3. A trailing comma was refused while every surface said JSONC was tolerated

`blankJSONComments` stripped comments and nothing else. Measured on
`{ "theme": "One Dark", }` as Zed's `settings.json`: **rc 1**, *"does not parse …
fix or move that file"* — this command's own stated failure mode, refusing while
blaming the user, on a file the editor authored and reads back happily.

Both parsers were **read**, not remembered:

- **Zed** — `crates/settings_json/src/settings_json.rs`'s
  `parse_json_with_comments` is `serde_json_lenient::Deserializer::from_str`.
  `serde_json_lenient` accepts `//` and `/* */` comments **and** trailing commas
  by default, no feature flag; that leniency is the crate's purpose.
- **VS Code** — `src/vs/base/common/jsonc.ts` strips both before `JSON.parse`:
  the scanner's fifth capture group is `(,\s*[}\]])`, and there is a
  `.replace(/,\s*([}\]])/g, '$1')` fallback besides.

🔴 **NEITHER EDITOR WAS RUN.** The claim rests on those two sources. And
**opencode's parser was not established at all** — its row rides on the same
`AllowsComments` gate by inference. What bounds that: this command writes
**strict JSON** back in every case, so tolerating an input form the agent would
have rejected can never produce a file the agent cannot read. The asymmetry
`AllowsComments`' own comment worries about does not exist in this direction.

`blankJSONTrailingCommas` blanks rather than deletes, like its sibling, so byte
offsets into the original stay valid. The refusals that must SURVIVE the
loosening are pinned separately (`TestATrailingCommaDoesNotMakeRubbishAcceptable`,
`TestAStrictAgentStaysStrictAboutTrailingCommas`): a doubled comma, a leading
comma, a truncated file, and a strict agent's plain-JSON config.

**The exception statement had to widen with it.** A trailing comma does not
survive the re-encode either, so every surface that framed the exception as
"comments and key order" now names all three forms — README, the command's
`Long`, the change `reason`, this file, `AGENTS.md`.

## 4. `--json` emitted zero bytes for three of four failure shapes

One flag, three contracts:

| trigger | `AGENTS.md` written | `--json` stdout | rc |
|---|---|---|---|
| plan-time refusal | yes | full payload, `ok:false`, `blocked` row | 1 |
| write-time refusal (a broken symlink) | yes | **0 bytes** | 1 |
| write-time failure (an unwritable dir) | yes | **0 bytes** | 1 |
| `--check --json` on an unparseable config | n/a | **0 bytes** | 1 |

Only the first was documented. A consumer facing the others cannot distinguish a
partial run from a usage error — and the fourth is what
`developer.civitai.com`'s hosted prompt reads.

- **The write path** now attempts each of the three files **independently** and
  turns a failure into that row's own `blocked` action, exactly as the MCP
  refusal already was. Independence is not decoration: the old code returned on
  the FIRST failure, so a payload emitted after one would have claimed actions
  for files nothing ever tried to write.
- **`--check`** turns an unreadable config into `mcp-site`/`mcp-orch` rows
  carrying the parse failure, which is what the code's own principle already said
  — *"COULD NOT LOOK is not NOT REGISTERED, and the details say which"* — and
  what the other two could-not-look cases already did.

## 5. `--dry-run` reported an action the real run refuses

`resolveWriteTarget`'s broken-symlink refusal lived in the **write** path, so
`--dry-run` reported `create`, `ok: true`, exit 0 for a destination the real run
refuses by name. The file header's claim that a dry run "cannot report a path or
an action the write path would not take" was true of the refusals that happened
to sit in the planner and blind to the one that did not.

Resolving the destination is a **pure read**, so it moved into `planMCPConfig`
where the claim can hold — which also turned the real run's bare error into a
`blocked` row (defect 4). `TestDryRunAndTheRealRunAgreeOnEveryAction` is the
**ledger** over the class rather than a second copy of the one case: for four
fixtures, the two runs' `changes` must agree on path **and action**, row for row.
Comparing paths alone is satisfied by `create` opposite `blocked`, which is the
defect.

🔴 **What the claim still cannot cover, stated rather than left to be found
again:** a failure that only the *act of writing* can produce — permissions, a
full disk, a race — is by definition invisible to a run that performs no write.
`--dry-run` reports it as `create`; the real run reports it as `blocked`. That is
the residual, and defect 4's independent-attempt fix is what keeps it from being
silent.

## Red-then-green

Every defect above was reproduced by running the binary on `c801ab8` — the tip of
the round-1 fix — then fixed and re-run on the same input.
`agent_setup_round2_test.go` is deliberately **black-box**: every assertion drives
the command through `run` and reads the file or the payload it produced, so the
same test source compiles and runs against the pre-fix tree. A guard referencing a
symbol the fix introduced cannot be watched red — it fails to compile, which is
not the same observation.

The controls in that file passed on `c801ab8` as well as after, which is what
makes them controls: a header-less run still names the server that 401s, broken
JSON is still refused, a strict agent still rejects a trailing comma, and the
plan-time refusal still emitted its payload.

**One guard was strengthened rather than added.**
`TestRepeatedRunsAreIdempotent` claimed "the per-key merge must not accumulate"
while starting from an **empty** project — so every run after the first merged
over this command's own output, which is the one input this file's own thesis
says cannot break a merge routine. It now starts from a file the user wrote,
carrying a key on OUR entry that the command never renders, and asserts that key
is still there after three runs. It was **green on `c801ab8`**: it is coverage
that was missing, not a defect that was found.


# Round 3

Round 2 was re-audited the same way, and the shape that came back is one shape:
**a guard's description was wider than its body**, four times over. That is what
let every defect below through a green suite, so it is stated first.

| the sentence | what the body actually did |
| --- | --- |
| the extractor's "every invocation" | one invocation |
| the TOML fixture "a key on OUR entry" | only keys the renderer never writes |
| `TestRepeatedRunsAreIdempotent`'s "the MERGE's idempotence" | started from an empty project |
| `TestDryRunAndTheRealRunAgreeOnEveryAction`'s "every action" | four fixtures, all varying only the MCP destination |

## 1. `--dry-run` lied about `AGENTS.md`, exactly as it had about the MCP config

Round 2 moved `checkWriteTargetResolvable` into `planMCPConfig` **and stopped**.
`writeProjectFile` → `writeFileAtomic` → `resolveWriteTarget` still reached the
refusal for the two instruction files only at **write** time, so the file
header's claim that a dry run "cannot report a path or an action the write path
would not take" was still false — one file over. Measured on `897c1cc`:

```
$ ln -s /nonexistent/agents.md proj/AGENTS.md
$ civitai agent-setup --agent claude --dir proj --dry-run --json
rc=0  ok=true   create AGENTS.md / create CLAUDE.md / create .mcp.json
$ civitai agent-setup --agent claude --dir proj --json
rc=1  ok=false  blocked AGENTS.md / create CLAUDE.md / create .mcp.json
```

`planAgentsMD` and `planClaudeMD` now classify their own destination, and every
plan-time refusal — for all three files — becomes that file's `blocked` row
rather than a bare `return err`. `TestDryRunAndTheRealRunAgreeOnEveryAction`
gained a fixture **per written file**, plus a `wantBlocked` field naming which
row each fixture must block: agreement alone is satisfied by two runs that are
both wrong.

## 2. A preserved value was reported as authenticating, in both directions

`mcpAuthCoverageOf` treated **any** non-empty `Authorization` as "carries a
credential", and `mcpPreservedAuthNote` ended "…and that is what they
authenticate with". Two measured cases:

1. **Zed with this CLI's own template still in place.** The next-step block tells
   a Zed user to paste `"headers": {"Authorization": "Bearer <your token>"}`.
   With that literal on disk the run called the entry authenticating and
   **dropped** the `needs an Authorization header — this one returns 401 without
   a credential` row that `c801ab8` printed. The same run then contradicted
   itself, because Zed's static `Caveat` still said the entries carry no
   Authorization header.
2. **Codex, no token.** A leftover `bearer_token_env_var = "CIVITAI_TOKEN"` made
   the run say "…and that is what they authenticate with" **from the `!hasToken`
   branch**, which is reached only when `CIVITAI_TOKEN` is unset in this process:
   the command had just established the reference resolves to nothing. It also
   called `bearer_token_env_var` an "Authorization header", which it is not —
   the file contains no header; Codex builds one from the variable name.

Round 2 stated its residual only in the **conservative** direction ("a shape this
code cannot see is described as absent"). This is the **opposite** direction and
it was unstated. Items 28 and 34 both say the command does not claim what it
cannot observe, so the classification is now three-valued (`mcpAuthKind`):

- `authManaged` — byte-for-byte what this command renders for that target. It
  may be described as *referencing* `CIVITAI_TOKEN`, and, when that variable is
  unset here, as resolving to nothing until the user exports it.
- `authOpaque` — present, not this command's, **unevaluable**. Every sentence
  about one says exactly that: *"…this command did not write that value and
  cannot tell whether it resolves to a credential."*
- `authNone` — nothing recognised. The conservative residual, unchanged.

The 401 row is no longer suppressed by a string the command cannot evaluate; each
kind gets its own honest row instead. Zed's `Caveat` now describes what this
command **writes** rather than what the file **contains** — a static sentence
cannot know the second.

🔴 **Both residuals now stated.** (a) conservative: an unrecognised shape reads
as absent. (b) optimistic, closed: material this code *can* see is not thereby a
working credential, which is why `authOpaque` exists.

## 3. `--json` still emitted zero bytes outside the enumerated four shapes

Round 2 closed four **instances** and asserted an **absolute** — "never as an
empty stdout". Measured on `897c1cc`, all rc 1 with 0 bytes on stdout:

```
mkdir proj/AGENTS.md ; … --json
mkdir proj/AGENTS.md ; … --check --json      # the surface developer.civitai.com reads
mkdir proj/AGENTS.md ; … --dry-run --json
env -i … --agent codex --json                # "neither $XDG_CONFIG_HOME nor $HOME are defined"
```

The class is now closed at the **one place stdout is written**
(`agentSetupEmitter`), not per instance: `agentSetupChecks` returns no error at
all — a file it cannot read is a failed row — and any error that still escapes
without a payload is emitted as the **third shape**, `track` / `agent` /
`ok: false` / `error`, with neither array. A consumer discriminates on which of
`checks` / `changes` / `error` is present.

🔴 **The one exception is STATED rather than absolute.** Exit `2` is a mistake
about the *invocation* — unknown `--agent`, a bad `--dir`, `--track api` — and
there is no run to describe, so it goes to stderr like every other command's
usage error. `TestAUsageErrorStillEmitsNoPayload` is the control that keeps
"close the class" from widening into it.

## 4. Write-independence had no guard, and the mutant survived the package

Inserting `if writeFailed { return }` at the top of `runAgentSetupWrite`'s
`attempt` — restoring abort-on-first-failure — **survived `go test
./internal/cmd`** at `897c1cc` (rc 0). Built and run against a broken-symlink
`AGENTS.md`, that mutant emitted `blocked / create / create` while only the
symlink existed on disk: a payload claiming `create` for two files nothing
attempted, precisely what §4 above says independence prevents.

`assertBlockedWritePayload` carried the right assertion; all three of its
fixtures failed the **last** write, so an abort-on-first mutant never skipped
anything. `TestAFirstWriteFailureDoesNotStopTheLaterOnes` fails the **first**
write, at write time rather than plan time — `AGENTS.md` symlinked to an existing
file inside a `0500` directory, so the destination resolves and the atomic write's
temp file cannot be created. With the mutant applied it is the **only** test in
the package that fails; without it, green.

Its docstring says it is a **mutation guard, not a regression test**: `897c1cc`
already has the behaviour, so it passes there. `assertBlockedWritePayload`'s
`wantWritten` is now a parameter for the same reason — its docstring said "the
instruction files written anyway, because they have nothing to do with the MCP
config", which describes nothing at all about a first-write-fails fixture.

## 5. The idempotency fixture repeated the blind class round 2 diagnosed

Round 2's rewritten `TestRepeatedRunsAreIdempotent` used
`startup_timeout_sec = 45` (TOML) and `theirEntryKey` (JSON) — keys the renderers
emit under **no** condition. §1 of round 2 names exactly that: *a fixture built
only from never-written keys cannot distinguish "skip what we re-render" from
"skip a fixed list"*. It passed on `c801ab8`: an invariant guard wearing a
regression guard's docstring.

The pre-file now also carries an **owned-but-not-re-rendered** key (each target's
`HeadersKey`), and the assertions name the keys instead of matching the digits
`"45"` anywhere in the file. Measured scope: with that key added the **codex**
subtest goes red on `c801ab8` and the six JSON subtests still pass — round 1's
JSON merge was already per-key, and `c801ab8`'s defect was in `mergeTOMLBlock`
alone. The JSON arms therefore remain invariant guards and say so.

## 6. The round-2 file's blanket "watched red" was false for 3 of its 10 tests

Replayed against the `c801ab8` payload, eight were red;
`TestARunWithoutATokenAndWithoutAHeaderStillSaysSo`,
`TestATrailingCommaDoesNotMakeRubbishAcceptable` and
`TestAStrictAgentStaysStrictAboutTrailingCommas` **passed**. They are deliberate
over-widening controls and they are correct — but RULES.md requires an invariant
guard be labelled as one and not counted as regression coverage. The file header
and each of the three now say so; the sentence made a maintainer count ten where
there are seven.

🔴 **The same shape was then found one file over, in `agent_setup_round1_test.go`.**
Its header says every test below was watched red on `6340db8`. Two of them have
since been **rewritten** — `TestTOMLMergePreservesKeysOnOurOwnTable` in round 2
and `TestRepeatedRunsAreIdempotent` in rounds 2 and 3 — and a blanket matrix does
not survive a rewrite of the body it was measured against. Six of the seven
per-agent subtests of the latter PASS on `c801ab8`. The header now says so and
points at each rewritten test's own, narrower matrix.

## 7. A README absolute falsified by `action: manual`

`README.md` said "there is no outcome where a step is skipped and the command
still exits `0`". But `civitai agent-setup --agent other --json` exits **0**,
`ok: true`, with the MCP row `action: manual` and nothing written; a user-scoped
agent with no resolvable home is the same. That is deliberate — there is no file
for this CLI to write, so the config is printed to paste — so the absolute is
replaced by naming the one exception, in the README and in `Long`. A step this
CLI *could* have taken and did not is always `blocked`, never `manual`.

## 8. `--dry-run`'s error claimed the instruction files were written

`"the instruction files were written; the MCP config was not"` was a pre-existing
sentence, but round 2's fix **routed a new case into it**: the broken-symlink dry
run used to exit 0 and now exits 1 with that claim attached to a run that wrote
nothing at all. `ok`, the exit code and the error text all derive from one
predicate now — is any row `blocked` — and the dry-run arm says what a dry run
actually did.

## Red-then-green

`agent_setup_round3_test.go` is black-box for the same reason the round-2 file
is, including the envelope assertions, which decode into a bare `map[string]any`
rather than naming a Go field the fix introduced. The file header enumerates
which of its tests are **regression**, which is a **mutation guard**, and which
are **over-widening controls** — the round-2 mistake, not repeated.

## 9. The sweep, and what it found in round 3's own guards

Because this is the defect's fifth appearance, all 45 top-level tests in the
three round files were swept for it — docstring claim against body — rather than
only the ones the audit named. Six more instances, three of them in guards
written *in this round*:

- **`agent_setup_round1_test.go`'s header is false in two ways.** Measured: copy
  the file into a worktree at `6340db8` and `go vet ./internal/cmd` stops at
  `srv.Anonymous undefined`; `mcpAnonymityNote`, `blankJSONComments`,
  `agentsMDBlockState`, `actionBlocked`, `PreferExisting` and `agentEnv.GOOS` are
  absent there too. A `_test.go` file is one compilation unit, so **no** test in
  it was watched red at `6340db8` in this form — what was measured is what each
  docstring says, that the BEHAVIOUR was reproduced against the pre-fix BINARY.
  That is a real observation and a weaker one; it is exactly the distinction the
  round-2 and round-3 files are black-box to preserve, and both of those do
  compile against their own base trees. Second, five tests in that file are
  self-described opposite-direction controls, i.e. invariant guards, and none was
  labelled.
- **`TestRepeatedRunsAreIdempotent`'s round-3 docstring — written in this round —
  was itself wider than its body.** It said `owned` is "in the merge's owned set
  and NOT re-rendered on this run" for all seven agents, while setting a token:
  `mcpEntry` writes `HeadersKey` whenever `EnvHeaderSyntax != ""` and a token is
  set, so the property held for **2 of 7**. The run is now parameterised on the
  token and `ownedIsReRendered` is derived from the two facts the renderer itself
  reads. Both codex arms are red on `c801ab8`; the JSON arms remain invariant
  guards and say so.
- **`TestAUsageErrorStillEmitsNoPayload` — written in this round — asserted only
  `err != nil` and an empty stdout**, never that the invocation is in the exit-2
  class. So an input that regressed *out* of that class into an ordinary runtime
  failure with zero bytes on stdout — the very defect §3 exists to prevent —
  would have passed here and been certified correct. It now asserts
  `errors.Is(err, ErrUsage)`.
- **`TestAFirstWriteFailureDoesNotStopTheLaterOnes` — written in this round — had
  its closing assertion behind an earlier full run.** `assertBlockedWritePayload`
  performs a complete `agent-setup` against the same dir, so the final
  `os.Stat(mcpPath)` passed on *that* run's artefact whatever the run it named
  did. The direct run now comes FIRST, with a premise that neither later file
  exists beforehand. Re-verified against the `attempt` mutant: still the only
  failing test in the package, now dying on both later-file assertions.
- **`TestJSONIsEmittedForEveryFailureShape` is named for a class and enumerates
  four instances.** Its docstring now says so and points at
  `TestJSONNeverExitsSilently` as the class-level guard.
- **`TestDryRunAndTheRealRunAgreeOnEveryAction`'s "the dry run wrote nothing"
  check sampled one of the three paths.** It now snapshots which paths a fixture
  left absent and asserts none of them appeared, with a premise that the set is
  non-empty.

Four more pre-existing round-1 guards were flagged and deliberately NOT changed
here, because they are outside this round's findings and each needs its own
measurement: `TestABrokenSymlinkIsRefusedByName` (asserts the message, not that
the dangling target was left unmaterialised — the thing its sentence is about),
`TestCodexHomeIsNotConsultedOnATargetThatDoesNotDeclareIt` (samples one
target × one root var for a general per-target claim, and discards `ok` from
`agentConfigPath` so the assertion can pass vacuously),
`TestJSONPathsAreAbsoluteUnderTheDefaultDir` (its `--check` half filters on both
the row name and `c.OK` with no counter, so it can assert nothing), and
`TestAGenuinelyMalformedConfigStillRefuses` (checks "the file is untouched" as
`Contains(got, "rubbish")` where its round-2 sibling uses `got != tc.src`).

# Round 4

Three findings. The common thread is not any one of them — it is that **each of
the previous three rounds fixed a false absolute by writing a narrower absolute
that was also false**, in the same commit as the fix and before anyone had
enumerated the cases:

| round | the sentence it shipped | what falsified it |
| --- | --- | --- |
| 2 | `--json` "never as an empty stdout" | three cases, found in round 3 |
| 3 | "three shapes and no silent one; the one exception is exit 2" | a fourth shape, found in round 4 |
| 3 | `--dry-run` reports "the same rows, the same `ok` and the same exit code … for **any** of the three files" | a test *inside the same PR* depends on the disagreement |

So the rule this round adopted, and the one to hold future rounds to: for a claim
about behaviour, either **make the code true and name the guard that enforces
it**, or **state the enumerated truth positively and say the enumeration may not
be exhaustive**. "There is no case where…" is the sentence that has now been
wrong three times.

## 1. `--json` had a fourth, silent shape: rc 1 with zero bytes

`resolveAgentSetupDir` returns three classifications (item 26): does-not-exist
and not-a-directory are `ErrUsage` (exit 2), and **any other stat failure is
returned untagged** (exit 1, item 24). Round 3 preserved the "exit 2 carries no
payload" exception by **constructing the emitter after that call** — which made
the untagged arm silent too. Measured at `c341be4`:

```
$ mkdir -p outer/proj && chmod 0000 outer
$ civitai agent-setup --agent claude --dir outer/proj --json
rc=1   stdout = 0 bytes   stderr: Error: stat …/outer/proj: permission denied
$ civitai agent-setup --agent claude --dir outer/proj --check --json
rc=1   stdout = 0 bytes
$ printf x > file; civitai agent-setup --agent claude --dir file/sub --json
rc=1   stdout = 0 bytes   stderr: Error: stat file/sub: not a directory
```

Five surfaces written in round 3 denied it, including the published `Long` and
the README, and one source comment asserted this branch "exits 2" when it
exits 1.

**Two fixes were available.** The rejected one: tag the untagged arm
`asUsageError` so it exits 2 and the stated exception becomes true. That would
have made the sentence true by moving the code to fit it, at the cost of calling
an *environment* condition a mistake about the invocation — a directory that is
momentarily unsearchable is a fact about the machine, and the command line
naming it may be exactly right. It also contradicts item 24 (errno failures take
the generic code) and item 26's published three-way branch, which are the two
places this repo already decided the question.

**The chosen fix** moves the decision from *position* to *the error*:
`RunE` builds the emitter first, has a **single return**, and hands every error
to `envelope`, which skips only `errors.Is(err, ErrUsage)`. Both halves are then
properties of one function rather than of a list somebody maintains — and the
one thing that can still break it is named at the place where it would happen
(`RunE`'s comment: do not add a second return).

## 2. `--dry-run` and the real run disagree on write-time failures

Measured with an existing `AGENTS.md` in a project directory at mode 0500:

```
--dry-run --json : ok=true   rc=0   append-managed-block / create / create
        --json   : ok=false  rc=1   blocked / blocked / blocked
```

The README and `Long` said the two runs report "the same rows, the same `ok` and
the same exit code … for **any** of the three files", while
`TestAFirstWriteFailureDoesNotStopTheLaterOnes` — in the same PR — fails
`PREMISE BROKEN` if they ever agree on a case like this one.

**The prose moved; the code did not, and neither did the test.** The rejected
alternative was teaching the planner to predict writability (an `access(2)`
probe, or a temp-file probe). Both replace a false absolute with a narrower one:
an advisory permission check is wrong under ACLs, read-only mounts and immutable
flags, and *no* pre-check covers a full disk. A dry run that performs no write
cannot observe a failure the write produces, and that is now what the documents
say — naming the **one** case actually reproduced (an unwritable project
directory) as measured, and a full disk and a read-only mount as the same shape
by construction but **not** measured. The list is stated as open.

`TestDryRunAndTheRealRunAgreeOnEveryAction` was renamed
`TestDryRunAndTheRealRunAgreeOnEveryPlanTimeOutcome`: all seven of its fixtures
vary something the **plan** can see, so that is the width of what it
establishes. `TestADryRunCannotSeeAFailureOnlyTheWriteCanProduce` (round-4 file)
now pins the divergence head-on, and goes red if a later change makes the
planner predict it — which is the correct outcome, because the sentence would
then need re-measuring rather than inheriting.

## 3. A fully blocked run still opened with "Configured …"

```
$ chmod 0500 proj && civitai agent-setup --agent claude --dir proj
Configured claude for Civitai App development in …/proj
  ✗ blocked …/AGENTS.md   ✗ blocked …/CLAUDE.md   ✗ blocked …/.mcp.json
```

The verb branched on `dryRun` and nothing else. `ok`, the exit code and the error
already read `blockedRowSummary`; the headline — the **first line the user
reads** — did not. This is round 3's own finding 8 one surface over, and round 3
*widened* which inputs reach it, because a plan-time refusal used to be a bare
`return err` that printed no report at all.

`agentSetupHeadline` now counts `blocked` rows — the same predicate `ok` uses, so
the two cannot disagree — and a `manual` row still does not move it, because a
manual step is a success with work left for the human (that is the published
exit-code contract, not an oversight).

## Red-then-green

| finding | red at `c341be4` | green after |
| --- | --- | --- |
| 1 | `TestJSONNeverExitsSilently`'s five new subtests, "no readable payload (unexpected end of JSON input)"; its six pre-existing subtests PASS at `c341be4` | all eleven pass |
| 2 | no test was red — the code is correct and the **prose** was wrong; `TestADryRunCannotSeeAFailureOnlyTheWriteCanProduce` is labelled a DOCUMENTATION GUARD and passes at `c341be4` by design | passes |
| 3 | `TestTheHeadlineReadsTheRowsRatherThanTheDryRunFlag`'s two blocked arms, both modes; its "nothing blocked" CONTROL arm PASSES at `c341be4` | all six pass |

The round-4 file labels each test's kind for the reason round 3's file gives.
The finding-1 case was added to `TestJSONNeverExitsSilently` rather than to a new
test, so the shape class stays in one place; it asserts `!errors.Is(err,
ErrUsage)` before asserting a payload, so it pins the classification decision as
well as the payload.

## What this round could NOT establish

- That `--json` emits a payload for **every** non-usage failure. What is
  established is that every error `RunE` returns passes through `envelope`, and
  `envelope` emits for everything that is not `ErrUsage`. That is a property of
  two functions, verified by reading them and by the eleven cases above — not a
  proof over all inputs. A second `return` in `RunE` would end it; a mutation
  inserting one that bypasses `envelope` for the `--dir` check DOES turn five
  subtests red, so the shape is defended for the listed inputs, but a second
  return covering a future step no input reaches would not be caught.
- That the write-only failure list (unwritable directory, full disk, read-only
  mount) is complete. Only the first was reproduced here.

## Mutation matrix for the round-4 guards

Each mutant was applied to `internal/cmd/agent_setup.go`, the named tests run
with `-count=1`, and the file restored from a `cp` aside — never from a stash,
never from `git checkout --`.

| mutant | killed by | with its own message? |
| --- | --- | --- |
| drop `errors.Is(err, ErrUsage)` from `envelope` | `TestAUsageErrorStillEmitsNoPayload` (the over-widening control) | yes — "put a payload on stdout for a usage error" |
| tag the untagged `os.Stat` failure `asUsageError` — i.e. **the rejected alternative fix** | `TestJSONNeverExitsSilently`, all five new subtests | yes — "was classified as a usage error … items 24 and 26" |
| `agentSetupHeadline`'s success arms taken unconditionally | `TestTheHeadlineReadsTheRowsRatherThanTheDryRunFlag`, both blocked arms × both modes | yes — "first line does not lead with …" and "opens by claiming it configured the agent" |
| a second `return` in `RunE` bypassing `envelope` for the `--dir` check | `TestJSONNeverExitsSilently`, five subtests | yes |

🔴 **One mutant "SURVIVED" first time and the survival was an artefact of the
harness, not of the code.** The `asUsageError` mutant was first applied with a
`perl -0pi -e 's/\Q…\E/…/'` whose `\Q…\E` quoted the embedded newlines
literally, so nothing was substituted and the suite passed against *unmutated*
source. The tell was that the `grep` verifying the edit printed nothing — only
visible if you look at it. Re-applied with a Python replace that asserts
`count(anchor) == 1` first, the mutant dies. Assert the edit landed before
reading the verdict.

---

## `--check`'s `mcp-*` rows: presence in a file, never reachability (#534, dogfood 2)

A blind dogfood probed both registered servers with a real MCP `initialize`, no
credential:

```
POST https://mcp.civitai.com/mcp            -> 200
POST https://orchestration.civitai.com/mcp  -> 401
```

and `civitai agent-setup --check --json` reported, for both:

```json
{"name": "mcp-site", "ok": true, "detail": "/home/u/proj/.mcp.json"},
{"name": "mcp-orch", "ok": true, "detail": "/home/u/proj/.mcp.json"}
```

`ok: true` beside a bare filename is indistinguishable from "this server works",
and one of the two rows was green against a `401`. The command's own PROSE was
already correct — the write path prints *"⚠ needs an Authorization header — this
one returns 401 without a credential"*, and the `authenticated` row already says
*"this row only checks that one is present"* — so what was narrow was the
machine-readable row, which is the surface `developer.civitai.com`'s hosted
prompt consumes.

### What was NOT done: a network probe

`--check` is offline and side-effect-free by construction — every call in
`agentSetupChecks` is a file read — and that property is worth more than a tick
that has to reach the network to be earned: it works on a plane, in CI, behind a
proxy, and it can never be why a setup run hangs. `ok` was also left alone: the
entry IS registered, which is what the row has always meant and what
`agentSetupVerdict` counts. **The sentence is what changed**, the same fix shape
the `authenticated` row got.

`mcpRegisteredDetail(path, srv)` (`internal/cmd/agent_setup_mcp.go`) renders it:

```
registered in <path> — this row read that file; it does not contact <url>[, which
returns 401 until an Authorization header is present]
```

The trailing clause is **derived from `mcpServer.Anonymous`**, the same field
`mcpAnonymityNote` reads, so it cannot be spelled per Check name and drift from
the table; a third server gets the right sentence the moment it is added.

It deliberately does not claim the entry AUTHENTICATES — whether a header
present in the file resolves to a credential is `mcpAuthCoverage`'s three-valued
question, not this row's.

### Guards

`internal/cmd/agent_setup_scaffolder_test.go`:

- `TestRegisteredMCPRowSaysWhatItInspected` — checks the premise (the two shipped
  servers still disagree on `Anonymous`, else the assertion is vacuous), then
  MUTATES `Anonymous` on a probe server in both directions and requires the
  sentence to move with it.
- `TestCheckJSONDoesNotReportOrchestrationAsReachable` — drives the real
  `--check --json`, asserts each `ok: true` row is not the bare config path, and
  that the two rows differ, because the two servers do.

Red on `origin/main` `cbcb992` with a shim reproducing that branch byte-for-byte
(`Detail: path`):

```
the mcp-orch row's ok:true detail mentions 401 = false, want true (Anonymous=false): "/home/u/proj/.mcp.json"
mcp-orch's ok:true detail is the bare config path — presence in a file is not reachability of …
both mcp rows carry an identical detail … while the servers disagree on anonymous access
```
