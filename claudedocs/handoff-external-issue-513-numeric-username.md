# Handoff: external-issue-513-numeric-username — 2026-09-09

## Run this first — the index, one command
```bash
cairn recall --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Clear the queue of **externally-reported** issues on `civitai/cli` — three open, none
answered — starting with #513, and fix the API-side root cause behind it.

## State now

- **Branch `main` @ `69724bf`.** The base clone is synced (`merge --ff-only`).
  🔴 **`main` moved SEVEN times during the session that wrote this** (#533, #534,
  #536, #537, #538, plus our #535 and #532). Treat any byte count or merge-state
  reading in this doc as a snapshot; re-measure before acting.
- **BOTH PRs of this effort are MERGED:**
  - **`civitai/cli#532` → `69724bf`** — `FlexString` in `pkg/civitai`, fixing #513.
    Shipped as **`fix(read)!:`** (source-breaking for direct importers).
  - **`civitai/cli#535` → `2c694f0`** — AGENTS.md headroom: evicted the wrong
    `newWhoAmICmd` snippet, fixed `CONTRIBUTING.md`'s phantom `internal/api`, and
    extended `layout_ledger_test.go` to cover `CONTRIBUTING.md`.
- **AGENTS.md is 30,028 B** vs `agentsMaxBytes` 30,500 — **472 B headroom**.
  Neither that constant nor `agentsMaxBytesCeiling` (30,600) was changed.
- **Recorded in the subsystem index:** NEW entry `cli/civitai` (revision
  `33097855f51bce77`) — the public read SDK had no entry, and the `cli/` scope now
  holds 6. Carries the `getInto` defect class, the breaking-change/`!` rule, and one
  `OPEN:` bullet for the unreproducible server coercion.
- **No `clawgate-task:` recorded, deliberately.** `clawgate_handoff.sh resolve`
  exited **6**: one task linked (#544) with `role=created` and none `worked`. Filing
  a task is not doing its work, so this doc belongs to none of them.
- Both claims released: `cli-513-numeric-username`, `cli-agents-md-headroom`.

### Answered externally

- **`civitai/cli#513`** — Rochet2 answered after 10 days of silence, and the issue
  **reopened**: the merge auto-closed it via a closing keyword in #532's body,
  seconds after the comment said it would stay open. The server half is unresolved,
  so open is correct.
- **clawgate #544** carries the merged-commit cross-link and the live-probe
  correction to its own premise.

## Open investigations — live diagnosis state

### `/api/v1/*` serializes an all-digit username as a JSON number

- **Symptom + exact repro:** the public API returns `"username": 2802169344506` —
  unquoted — for users whose username is all digits (e.g.
  https://civitai.com/user/2802169344506). Any client typing the field `string` fails
  to decode a **200**. Reported by Rochet2 as `civitai/cli#513` on 2026-08-30.
  ```bash
  curl -s 'https://civitai.com/api/v1/images?username=2802169344506&limit=1' \
    | jq '.items[0].username | type'    # want "string"
  ```
- **Observed (with values):**
  - `civitai/cli` fails at `pkg/civitai/read.go:92`, in `getInto`'s
    `json.Unmarshal(raw, out)`, and surfaces
    `unexpected response from /api/v1/images (status 200): <snippet>` with exit 1.
  - `$CIVITAI/src/shared/zod/username.schema.ts` is
    `z.string().regex(/^[A-Za-z0-9_]*$/).trim()` — an all-digit username is
    **explicitly legal**, so this is valid data, not bad data.
  - That schema is **input-only**: `src/pages/api/v1/images/index.ts:56` uses it for the
    `?username=` filter (`usernameSchema.optional()`), not for the response.
  - The response value flows from the DB via `src/server/services/image.service.ts`.
  - `--json` on the CLI side is **unaffected**: `emitJSON`
    (`internal/cmd/read.go:95`) re-indents the **raw server bytes** via `json.Indent`
    and never marshals the struct, so a numeric username stays numeric in `--json`
    whatever the Go type is. `via: code`
- **Ruled out:** that the CLI's `--json` contract changes as a result of the Go fix —
  `emitJSON` operates on raw bytes, confirmed by reading it. `via: code` · That the
  username is invalid/rejectable data — the zod regex permits all-digit names.
  `via: code` · That this collides with external PR #526 — that PR's files are
  `internal/cmd/read.go`, `pkg/civitai/read.go`, `pkg/civitai/read_test.go`, while the
  `FlexString` change lives in the struct files plus a new file; zero file overlap.
  `via: measurement` (`gh pr list --json files`).
- **Leading hypothesis:** something between the Postgres read and `res.json` coerces
  the string. A `text` column passed to `res.json` stays a string, so a JSON number
  implies an intermediate step — a cache round-trip, a search-index document, a
  superjson transform or an implicit cast. **NOT diagnosed.** `via: assumed`
- **Next probe:** find the coercing step before writing any fix. Read the value's type
  at each boundary between the service and the response — that is what discriminates
  the candidates; do not adopt the cache theory without it.
  ```bash
  cd /home/zach/workspace/civit/civitai
  npm run test:unit -- src/tests/api/v1
  ```

### 🔴 SUPERSEDED — "the API serializes an all-digit username as a JSON number"

**The block below this one, from 2026-09-08, is RETIRED. Do not run its "Next
probe".** It instructs you to find the coercing step between the DB read and
`res.json`. That framing assumed the coercion is currently observable. It is not.

### The server-side coercion is NOT reproducible on any reachable surface

- **Symptom + exact repro:** as reported in #513 — `"username": 2802169344506`
  unquoted, breaking a typed decode of a **200**.
- **Observed (with values), measured 2026-09-09, RAW bytes grepped (never
  `jq`-parsed — `jq` hides the quoting):**
  - `GET /api/v1/users?query=2802169344506` → 200, 1 item, `"username":"2802169344506"`
    — **quoted**.
  - Six queries (`1234`, `999`, `2802`, `0000`, `12345678`, `2802169344506`) over
    `/api/v1/users` returned **every** all-digit username quoted, including 13-digit
    `280204751375`, `280220194698`, `2802217989326`. **Zero** unquoted forms.
  - **Positive control:** `/api/v1/images?username=civitai` → 200, 1 item,
    `"username":"civitai"`. The images query path works, so the zeros are readings.
  - All **7** all-digit accounts found return `items=0` on `/api/v1/images`.
- **Ruled out:** that the images query path is broken, which would have made the
  zeros meaningless — the positive control returns an item. `via: command` · That
  the CLI `--json` contract is affected by the Go field type — `emitJSON`
  (`internal/cmd/read.go`) re-indents RAW server bytes. `via: code`
- **Leading hypothesis:** UNDECIDED, and deliberately so. Three candidates an empty
  result **cannot** separate: (a) fixed server-side since 2026-08-30; (b) specific to
  the embedded-user serializer (`image.service.ts`) and still live but unobservable;
  (c) conditional on a cache/index path the probe did not hit. `via: measurement`
- **Next probe:** NOT "find the coercing step" — first get a test subject. Find or
  seed an all-digit-username account **with a public image**, then re-probe
  `/api/v1/images`. Failing that, Rochet2's answer on #513 is the lead.
  ```bash
  curl -s 'https://civitai.com/api/v1/images?username=<all-digit-user>&limit=1' \
    | grep -oE '"username":[^,}]{0,25}'    # RAW bytes; jq hides the quoting
  ```

## Next steps (ranked)

1. **`civitai/cli#526`** (xsvm, opened 2026-09-06). **Zero checks have ever run** —
   a fork PR needing maintainer approval to trigger workflows, which is why it reads
   `BLOCKED`. Approve the run, then review. They asked "would you be open to a PR?"
   first and got no answer. Fixes #525, the OTHER half of the `getInto` defect class.
   forcing: user — external contributor, no response since 2026-09-06.
2. **`civitai/civitai-developer-docs#61`** (giannamikaelova, 2026-08-24). Flux 2
   Klein training `whatif` returns HTTP 500 for the payload the docs publish, with a
   working control. Not fixable in either repo worked here — route it to whoever owns
   `orchestration.civitai.com`.
   forcing: user — the oldest unanswered external report, now 17 days.
3. **clawgate #544** (API-side coercion) — BLOCKED pending Rochet2's repro on #513.
   Its premise is flagged as not-currently-reproducible; closing it as
   not-reproducible is an explicitly valid completion.
   forcing: none
4. **Delete the stale local branch `fix/flexstring-numeric-username`** — it sits at
   `12818a3` in worktree `.claude/worktrees/agent-a98d33d792f41ee1c`, which holds it
   repo-globally. It does NOT reflect the merged PR; anyone reusing that worktree
   gets pre-rebase code.
   forcing: none

## Gotchas / decisions / dead-ends

- 🔴 **`--json` is NOT the surface to reason about here.** Every read command's `--json`
  emits the raw server bytes; only the typed SDK and the human/table path see the Go
  field type. A session that assumes `--json` shape changes will design the wrong fix
  and write the wrong README note.
- 🔴 **`FlexString` is a breaking change for external importers of `pkg/civitai`.**
  Comparisons against untyped string constants still compile; passing the field to a
  `func(string)` does not — which is exactly the 14 call sites above. Anyone "tidying"
  the type back to `string` reintroduces #513, which is why the brief requires a
  numbered decision item (36) in `AGENTS.md` + `claudedocs/decisions/`.
- **The two external CLI issues are one defect class.** #513 (numeric username) and
  #525 (raw C0 control chars) both end at `getInto`'s strict `json.Unmarshal` rejecting
  a real 200 and reporting `unexpected response (status 200)`. Different trigger, same
  seam, same unactionable message. Worth deciding whether that error string should name
  the decode failure rather than the response.
- **`clawgatectl task ls --summary --status open` returned 147 tasks and no duplicate**
  for this work (checked by title grep for username/api/v1/coerce). `cairn search
  'username coercion' --scope homelab-talos` was NO MATCH across 36 entries.
- **Task-authoring tags are hard-validated** — one invalid tag is a 400 that fails the
  whole create. The three chosen were checked against `GET /api/tags` before drafting.

- 🔴 **`git rerere` is ENABLED repo-globally in `civitai/cli`** (`rerere.enabled=true`,
  cache in the shared `.git`, 37 entries). It auto-applied this session's AGENTS.md
  renumber conflict on a later rebase, printing only `Resolved 'AGENTS.md' using
  previous resolution`. It was correct — but it is a machine replaying an edit nobody
  reviewed, and **rerere cannot rename files**, so it left AGENTS.md pointing at
  `37-numeric-username.md` while the doc was still `36-…` on disk. **Read any
  rerere-resolved hunk before trusting it, and check for the half it cannot do.**
- 🔴 **A `!` in the COMMIT SUBJECT is not enough — put it in the PR TITLE too.** The
  repo's `squash_merge_commit_title` is `COMMIT_OR_PR_TITLE`, so a title lacking the
  `!` can drop the marker at merge and the break reaches release notes as an ordinary
  fix. That is the #267 scar (`claudedocs/handoff-dogfood-2.md`). Verified present in
  `69724bf`'s subject after merge.
- 🔴 **`fixes #N` on a PR that fixes HALF of N still closes N.** #532 auto-closed
  #513 on merge although only the client half shipped — seconds after a comment told
  the reporter it would stay open. Reopened. Use a bare reference, not a closing
  keyword, when a PR closes only part of an issue.
- **An always-loaded file is NOT a `SKILL.md`, and `prune-skill`'s budget does not
  govern it.** `skill-audit.py` marks `cli` `[ungoverned]` and its "cut ~17,954 B"
  verdict binds nothing — AGENTS.md answers to `agents_size_test.go`. The deterministic
  scan also found **no** evictable history, dated lessons or >500 B lines: what paid
  was ONE wrong 691 B block, found by a staleness pass, not by pruning. **Correctness
  is the lever on an always-loaded file; a pass over one may legitimately end LARGER.**
- **AGENTS.md carries FIVE item guards, not four** — `agents_size_test.go`,
  `agents_split_preserved_test.go` (pins moved text VERBATIM),
  `agents_evidence_test.go` (bidirectional pointer↔file set equality),
  `agents_trigger_test.go`, `agents_index_test.go` (every item needs an index clause
  in the section preamble), plus `agents_xrefs_test.go` and `layout_ledger_test.go`.
  A new item therefore costs a trigger block **and** an index clause.
- **Do not extrapolate one item's byte cost from another's.** An audit derived item
  37's cost from #533's item (325 B) and concluded #532 would land 42 B over the
  CEILING. #532's actual item measured **201 B** and landed 82 B under. Measure the
  diff, never a sibling.
- **`users get <all-digits>` is an ID lookup, not a username query.**
  `internal/cmd/users.go` routes any `strconv.Atoi`-parsable argument to `?ids=`, so
  a user whose *username* is all digits is unreachable by name through that command —
  before and after #532. The decode fix lands via `images search`, `models search`,
  `creators list`, and `users get <name>`'s candidate list.

## How to verify

The merged fix, from a clean checkout:

```bash
cd /home/zach/workspace/civit/cli && git fetch origin && git merge --ff-only origin/main
go test ./pkg/civitai -run FlexString -count=1 -v     # the type's own unit + mutation fixtures
go test . -run 'TestContributing|TestLayout' -count=1 # the ledger #535 extended
make ci && nix-shell -p golangci-lint --run "make lint"
```

The byte budget (re-measure; do not inherit these numbers):

```bash
git show origin/main:AGENTS.md | wc -c                       # 30,028 at 69724bf
grep -nE 'agentsMaxBytes(Ceiling)? +=' agents_size_test.go   # 30,500 / 30,600
```

Whether the server still coerces — **read the RAW bytes**, since `jq` renders a
quoted and unquoted value identically:

```bash
curl -s 'https://civitai.com/api/v1/users?query=2802169344506' \
  | grep -oE '"username":[^,}]{0,25}'
```

🔴 Read `rc` directly rather than `$?` after a pipe.
