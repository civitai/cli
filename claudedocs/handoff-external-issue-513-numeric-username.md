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

- **Branch `main` @ `46c928a`**, clean apart from the same untracked `node_modules/`
  (16K, NOT gitignored) the previous handoff flagged. Still a live `git add -A` footgun.
- **The repo freeze is CLEARED** — the previous handoff's blocking item. `#529` landed
  the pin bump to `^0.39.0` / `^0.49.0`; `CIVITAI_CHECK_PUBLISHED_PINS=1 go test
  ./internal/scaffold -run TestScaffoldPinsSatisfyPublished` passes against live npm,
  and CI + CodeQL are green on `main`. Verified 2026-09-09T00:50Z.
- **Work claimed:** `claim-work --release cli-513-numeric-username` when done.
- **No clawgate task recorded.** `clawgate_handoff.sh resolve` exited 5 — 0 tasks for
  this session. Its positive control resolved 1 link for another session, so the board
  IS reachable and the token IS accepted; but a wrong session id also answers 200 with
  an empty array, so this is NOT evidence that no task exists.

### IN FLIGHT — the #513 fix

- Subagent working in worktree `.claude/worktrees/agent-a98d33d792f41ee1c`, branch
  **`fix/flexstring-numeric-username`** (off `46c928a`). **Nothing committed or pushed
  yet** — `git ls-remote --heads origin | grep flex` is empty.
- Confirmed live from its LSP diagnostics: the field-type change fans out to **14 call
  sites** in `internal/cmd` that pass `.Username` to `safeTerm` / `strings.EqualFold`
  (`apps.go:334,367` · `articles.go:194,210` · `collections.go:199,210` ·
  `models.go:217,232` · `images.go:246,275` · `creators.go:107` ·
  `users.go:111,119,132,145`). Each needs an explicit `string(...)`. This is the
  breaking-change cost of `FlexString`, and it was anticipated in the brief.

### Decisions taken this session (settled by the operator, do not relitigate)

- **Approach:** a `FlexString` type in `pkg/civitai` with a custom `UnmarshalJSON`
  accepting a JSON string OR number, applied to the `Username` fields. NOT a lenient
  decode inside `getInto` — that would collide with external PR #526, which edits
  exactly that block.
- **Scope:** the **7 sites in `pkg/civitai` only**. The 3 in `internal/appapi`
  (`appblocks.go:172,993`, `forgejoUsername:1340`) are deliberately excluded — a
  different service, no evidence of the coercion there.
- **Numeric fidelity:** the bare-number branch must preserve the literal digits
  (`json.Number`), never route through `float64` — `2802169344506` would become
  `2.802169344506e+12`.
- **API side:** fix it too, tracked as its own clawgate task (drafted, see next steps).
  Scope settled as *username + pin the class with a v1 contract test*; fix at the
  **root cause** plus a boundary test; dispatchable, human-reviewed PR.

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

## Next steps (ranked)

1. **Review and land the CLI `FlexString` PR.** IN FLIGHT: branch
   `fix/flexstring-numeric-username` in `civitai/cli`, worktree
   `.claude/worktrees/agent-a98d33d792f41ee1c`. Touches `pkg/civitai/{apps,articles,
   collections,images,models,tags_creators_users}.go`, a new type file, and the 14
   `internal/cmd` call sites listed above. Demand the red-at-base/green-at-HEAD matrix
   and the mutation matrix before believing the agent's green.
   forcing: user — an external contributor has had no reply since 2026-08-30.
2. **Create the API-side clawgate task.** Body is fully drafted and was rendered for
   approval this session but **not yet POSTed** — it is in the transcript, not on disk.
   Repo `civitai/civitai`, directory `/home/zach/workspace/civit/civitai`, tags
   `bug,civitai,project:civitai` (all validated against `/api/tags`).
   forcing: user — same reporter, and the client fix does nothing for third-party clients.
3. **Reply on `civitai/cli#513`.** Rochet2 asked whether the API or the CLI needs the
   fix; the answer settled this session is *both*, and nobody has told them.
   forcing: user — 10 days of silence on a well-formed external report.
4. **`civitai/cli#526`** (xsvm, opened 2026-09-06). **Zero checks have ever run** — a
   fork PR needing maintainer approval to trigger workflows, which is why it reads
   `BLOCKED`. Approve the run, then review. Note the reporter asked "would you be open
   to a PR?" first and got no answer.
   forcing: user — external contributor, 3 days, no response.
5. **`civitai/civitai-developer-docs#61`** (giannamikaelova, 2026-08-24, **16 days**):
   Flux 2 Klein training `whatif` returns HTTP 500 for the payload the docs publish, with
   a working control (image whatif quotes 12 Buzz). Not fixable in either repo worked
   this session — route it to whoever owns `orchestration.civitai.com`.
   forcing: user — the oldest unanswered external report of the three.
6. **Delete or gitignore `node_modules/`** in the `civitai/cli` base clone.
   forcing: none
7. **Run the subsystem-index `--pr` window once the FlexString PR lands** — this
   session's index write is OWED, not declined on content. Both windows read empty
   (`--session` = `looked-at-nothing`, 0 paths under cwd; `--commit` =
   `no-commits-in-range`) because the work is in a subagent worktree with nothing
   committed. `pkg/civitai` has **no entry** in the `cli/` scope (its 5 entries are
   `scaffold`, `appapi`, `release`, `credscan`, `pkgzip`), so the PR window is what
   will finally nominate it.
   ```bash
   python3 /home/zach/workspace/devrc/scripts/lib/subsystem_touch.py \
     --repo /home/zach/workspace/civit/cli --pr <n> \
     --exclude claudedocs/handoff-external-issue-513-numeric-username.md
   ```
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

## How to verify

The CLI fix, once the branch lands (read the OUTPUT, not the exit code):

```bash
cd /home/zach/workspace/civit/cli
make ci                                   # tidy + vet + test + build
nix-shell -p golangci-lint --run "make lint"   # NOT covered by make ci
```

The regression itself — this must be RED at `origin/main` and GREEN at the fix:

```bash
go test ./pkg/civitai -run FlexString -count=1 -v
```

The API side, once its task is worked:

```bash
cd /home/zach/workspace/civit/civitai
npm run test:unit -- src/tests/api/v1     # assert a NON-ZERO test count
curl -s 'https://civitai.com/api/v1/images?username=2802169344506&limit=1' \
  | jq '.items[0].username | type'        # advisory only — the account may be renamed
```

🔴 Read `rc` directly rather than `$?` after a pipe — this repo has been misled by that
once already.
