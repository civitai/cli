# Handoff: civitai-generate — 2026-08-07

## Goal

Ship `civitai generate` — text-to-image and image-to-image from the CLI — safely, on a
path that **spends the user's real Buzz irreversibly**. Feature is shipped and merged;
what remains is follow-up hardening and one open platform issue.

## State now

- 🔴 **The base clone is a SHARED checkout — check `git branch --show-current` before
  assuming it is on `main`.** As of 2026-08-06 it sits on
  `docs/login-comment-scanner-reflow` with an uncommitted `internal/cmd/login.go`
  belonging to **another session**. Do not switch it, stash it, or `merge --ff-only`
  over it. Work in a worktree instead:
  `git worktree add <dir> -b <branch> origin/main` (there is no `.envrc` in this repo,
  so a worktree needs no direnv setup).
- **Untracked files in `claudedocs/`** (`img2img-anime-research.md`,
  `photo-to-anime-workflow.json`, `verify_workflow.py`) — **not mine, leave them.**

### DONE (merged to `main`)

| PR | Commit | What |
|---|---|---|
| #210 | — | `civitai generate` + `civitai workflows list\|get\|cancel` |
| #215 | `e800129` | `--image` img2img, credential-free presigned upload |
| #218 | `39c0683` | killed 8 surviving mutants, gated `workflows cancel`, 2 doc fixes |
| #221 | `ba3136c` | surface silent model substitutions (pre- and post-spend) |

Platform issues filed against `civitai/civitai`:
- **#3665 CLOSED/COMPLETED** — silent model substitution now reported on tRPC replies (PRs #3692, #3673).
- **#3681 CLOSED/COMPLETED** — `civitai-cli` OAuth client granted the AI-services scopes (PR #3699). CLI side shipped as #220.
- **#3667 OPEN** — prompt auditing is keyed by field name with no coverage guard. Preventive, no bypass known.

### IN FLIGHT

- **PR #237** — the two substitution follow-ups (handle-before-advisory ordering, and
  the SUPERSEDED annotation). `MERGEABLE/CLEAN`; all **12** CI checks `SUCCESS`.
  **Nothing else should touch `generate_substitution.go` until it lands.**
  ✅ **Gate INDEPENDENTLY re-verified** (not the author's self-report):
  - **2377 `--- PASS` / 0 `--- FAIL` / 4 `--- SKIP`**, no timeout panic — counted,
    never read off the exit code. The 4 skips are the documented by-design ones.
  - **10/10 mutants killed** on a *separately constructed* battery, weighted at the
    CALL SITES rather than the renderer internals — which is the hole AGENTS item
    21(g) records from the first round. Both classes named there are now covered:
    an operand swap (note prints `Requested` instead of `Applied`) and a call-site
    phase swap (`substitutionAtEstimate` → `substitutionAfterSubmit`) each died to a
    specific named test. Also killed: advisory-before-handle ordering, note always
    empty, match on `Applied`, note dropped from confirm only, dropped from
    `--dry-run` only, hardcoded tense, dropped `checkpointSet` guard, `--no-wait`
    JSON branch reporting non-terminal.
  - 🔴 **The battery carried a POSITIVE CONTROL**, because 10/10 KILLED is also what
    a harness wired to nothing prints: a real edit no test can see (a comment word)
    was scored **SURVIVED**, so the harness can distinguish. Each mutation also
    asserted it actually applied, so a no-op `perl` could not be scored as a survivor.
  - `gofmt -s -l .` 0 files **with a planted malformed file flagged** as its control;
    `go vet` clean; `golangci-lint run` **0 issues with a planted `ineffassign` +
    `unused` flagged** in `internal/cmd` as its control (the handoff's own warning
    that a lint verdict needs a known-bad first).

### Deploy/verify status — honest

- **Verified live against production, end-to-end:** txt2img (8 Buzz), img2img
  (45 Buzz), and a real model substitution (read-only, 0 Buzz). See "How to verify".
- **Total spend this work: 53 Buzz.** Green/yellow balances untouched throughout —
  only blue moved, and blue drifts on its own (observed ±63 with no action).
- Not released: no `v*` tag pushed.

## Open investigations — live diagnosis state

### 1. ~~Advisory name lookups delay the workflow id~~ — PREMISE WAS WRONG. RESOLVED in PR #237.

🔴 **Do not re-derive this. The hazard as originally written does not exist on `main`.**

- **What I claimed:** the post-spend substitution report resolves version ids to names
  via `ResolveModelVersion` → `getWithRetry` (≈2 min/id worst case) *before* the
  workflow id reaches the user.
- **What is actually true (verified at `ba3136c`):**
  `reportModelSubstitutions(errw io.Writer, subs []genapi.ModelSubstitution, phase substitutionPhase)`
  — no `context`, no resolver, **zero I/O**; it prints raw ids.
  `grep -cE "context\.|resolveVersion|ResolveModelVersion|http\." internal/cmd/generate_substitution.go` → **0**.
  `resolveVersion` is called only at `internal/cmd/generate.go:703,715`, inside
  `buildGenerateGraph` **pre-estimate**, for `--checkpoint`/`--lora` — the load-bearing
  lookup AGENTS item 13 requires, not an advisory one.
- **Where the false premise came from:** the finding was real, but against **PR #222** —
  a competing implementation of the same feature that *did* resolve names in the render
  path. #222 was closed as a duplicate; **#221 merged instead**, and I carried the
  finding across without re-reading the merged code. The `Ran: Google's Nano Banana — …`
  transcript quoted in the original brief is not producible by anything on `main`.
- **Still worth knowing:** the constants themselves are right — `readMaxAttempts = 4`
  (`pkg/civitai/retry.go:29`), `defaultTimeout = 30s` (`pkg/civitai/api.go:54`) plus
  exponential backoff. So *if* name enrichment is ever added to the report, it must be
  bounded.
- **What #237 did instead:** fixed the *ordering* so the hazard cannot appear —
  `emitSubmitHandle` emits workflow id + externalId from one call site across all four
  branches **before** `reportModelSubstitutions`, plus a signature ledger that fails to
  compile the moment the renderer grows a context or resolver argument.

### 2. Approval summary names the checkpoint that will NOT run

- **Symptom + exact repro:** with a substitution in play, the confirm summary and
  `--dry-run` quote print `Checkpoint: <requested>` — so the last model line before
  `Generate? [y/N]` is the wrong one.
- **Observed (live, verbatim):**
  ```
  ⚠ The server SUBSTITUTED a different checkpoint …
    Ran:        Google's Nano Banana — Nano Banana (Checkpoint, id 2154472)
  …
  Checkpoint:   DreamShaper — 8 (Checkpoint, id 128713)
  ```
- **Ruled out:** silently swapping in the applied id — the user asked for the
  requested one and must still see that their input was overridden.
- **RESOLVED in PR #237:** `substitutionCheckpointNote` appends
  `[SUPERSEDED — the server will run version N instead; see the warning above]` to the
  requested label at both pre-spend surfaces, computed at one call site, matched on
  `requested`, tense from `substitutionVerb(phase)` (reuses item 21's phase model —
  no parallel notion of tense). Annotate rather than swap, so the user still sees that
  their input was overridden.

### 3. #3667 — prompt-audit coverage guard (platform, OPEN) — ⚠️ NOW CONFIRMED, NOT PREVENTIVE

🔴 **THE "no known bypass" LINE THAT USED TO BE HERE WAS WRONG. Do not restore it.**
It read *"Ruled out — IMPORTANT, do not re-derive: there is no known bypass"*, and its
reasoning was that ecosystem graphs lacking a `prompt` node **compose** a shared one
(`flux-graph.ts:269` merges `promptGraph`). That is true of the image graphs and false
as a generalisation — checking that some graphs compose `promptGraph` says nothing
about the graphs that deliberately **opt out of the shared node names**. Two do.

- **Symptom:** the content audit is keyed by hardcoded field names — `prompt` +
  `negativePrompt` (`orchestration-new.service.ts:1460`) and, separately,
  `musicDescription` + `lyrics` (`:1510`). Nothing enforces that a new text-bearing
  graph node gets an audit block.
- **Observed:** `data` is rebuilt from **declared graph nodes only**, so the gate is
  necessarily name-based. No test in
  `src/server/services/orchestrator/__tests__/` asserts call-site coverage.
- 🔴 **CONFIRMED unaudited, verified by code path at `civitai@a7e0bcd668`:**
  - **Hunyuan3D declares NO `prompt` node** — only `hunyuanPrompt`
    (`hunyuan3d-graph.ts:71`), prefixed because the bare names *"collide with the
    standard image Controllers in `GenerationForm.tsx`"* (`:12`). `data.prompt` is
    absent → `:1460` false → `auditPromptServer` never called. The handler then maps
    it back: `prompt: hunyuanPrompt ? hunyuanPrompt : undefined`
    (`hunyuan3d-graph.handler.ts:58`). The generator's prompt is text nothing audited.
  - **PolyGen's `texturePrompt`** (600 chars, `polygen-graph.ts:162`) is covered by no
    audit block, reaches the orchestrator (`polygen.schema.ts:143`), and is rendered
    publicly (`pages/3d-models/[id]/[[...slug]].tsx:279`). On `img2model3d` it is the
    only text in the request — that workflow's `prompt` node is gated
    `when: ctx.workflow.startsWith('txt')` and is deleted from `data`.
- **Sweep basis (so the next reader does not redo it):** all **79** declared node keys
  across `src/shared/data-graph/generation/*.ts`, checking every node whose input is a
  bare `z.string()`. Complete free-text set: `prompt`/`negativePrompt` (audited
  `:1467`), `musicDescription`/`lyrics` (audited `:1510`), `hunyuanPrompt`,
  `texturePrompt`, `title` (ACE Audio, display-only per its own comment).
  ⚠️ A `.node('key'` grep on ONE line misses these — both are multi-line `.node(\n
  'name',` declarations. Use a `perl -0777` multiline match.
- 🔴 **What this still is NOT — keep the claim this size.** The CLI is not the vector:
  both fields are reachable from the first-party form (`GenerationForm.tsx:1528`,
  `:2014`) and from any direct tRPC caller holding a personal API key. The gate stops
  accidents, not adversaries. And **no live generation probe was run** — this is
  reachability by code path, not a demonstrated exploit.
- **Escalated:** comment on civitai/civitai#3667
  (`#issuecomment-5210701143`) with the evidence above, proposing the asserted ledger
  (text-bearing nodes vs audited fields, failing when the first set grows). Open
  question left to the triager: whether the ledger keys on the graph node name or on
  what the handler emits — Hunyuan3D is a case where those differ.
- **CLI side:** PR #240 rewrites the `inputWorkflow` gate comment and design-doc §10 q4
  from "SUSPECTED" to confirmed. No behaviour change; the gate was already correct.

## Next steps (ranked)

1. **`git merge --ff-only origin/main`** in the base clone (1 behind).
2. **Wait for / review the in-flight follow-up PR** (investigations 1 and 2). Verify its
   gate yourself — re-run its mutations, don't trust the report.
3. ~~**`--input` is txt2img-only on purpose.** Unblocking … is gated on #3667's
   answer.~~ 🔴 **DECIDED: keep the gate. Do not unblock.** #3667 is now confirmed
   rather than suspected (investigation 3), so the gate's own stated lift condition
   — *"lift only when the upstream question is answered"* — has been answered in the
   negative. Unblocking would make CLI traffic a second client to a confirmed
   unaudited path. Revisit when the platform ships the coverage guard, and **not** on
   the argument that the gap is reachable from the web anyway: that argument is true,
   and it is an argument for fixing the platform. Still the biggest capability unlock
   left — it is just not available yet.
4. **webp support for `--image`** — needs `golang.org/x/image`, a new dependency.
   AGENTS.md puts that behind maintainer approval. **Zach's call.**
5. `--steps` / `--cfg-scale` / `--sampler` — deliberately omitted; the server accepts
   `steps:0` (degenerate half-price job) and ignores a misspelled sampler, both
   silently. Add only with a warn-first layer.

## Gotchas / decisions / dead-ends

- 🔴 **`make ci` runs NEITHER gofmt NOR golangci-lint**, and `make lint` silently falls
  back to `go vet` when golangci-lint is absent (it is, on this host — use
  `nix-shell -p golangci-lint --run 'golangci-lint run'`). A red `lint` job shipped on
  #215 exactly this way. Validate a lint verdict against a *planted* known-bad first.
- 🔴 **Never read a test exit code.** Count `--- PASS`/`--- FAIL`/`--- SKIP` and grep
  for `panic: test timed out` — a hang prints `FAIL` with no `--- FAIL` line. Observed
  once: `ok` printed with exit code 1.
- 🔴 **zsh does not word-split unquoted parameters.** `for c in $LIST` loops **once**
  with the whole string and silently matches nothing — this produced a confident
  `0/5 checks green` for 7 minutes while all 12 were green. Use literal lists.
- **Four tests skip by design:** `TestScanDirFromEnv`, `TestScaffoldPinsSatisfyPublished`,
  `TestSubmissionsCapDriftAgainstLiveAPI`, `TestScaffoldedReadyAckActuallyFires`.
- **`main` moves fast** (6+ merges during this session). Expect `AGENTS.md` numbering
  collisions — the file's own rule is *the PR merging **second** renumbers **its own**
  new items*. Happened 3×; worked cleanly each time. Also check `README.md`'s generate
  command-table row: two PRs edited the **same line**, and taking either side would
  have silently reverted the other.
- **CI has been 3–5 hours backlogged** at times (verified: runs at 17:40–19:45 took
  3–5h; runs at 00:04 completed in minutes). `mergeable` also lags — it reported
  `CONFLICTING` right after a clean merge push. Re-poll before believing it.
- **Design doc:** `claudedocs/civitai-generate-design.md` has a **retraction appendix**
  listing every v1 claim that did not survive implementation. Read it before trusting
  any statement in the body.
- **Dead end — do NOT re-add:** a "did the img2img promotion fire?" detector. Both the
  `factors.images` check and the price-differential version are **wrong**:
  Flux1Kontext, NanoBanana and Seedream are genuinely edit-capable and price
  *identically* with and without images. Either would refuse valid img2img on the three
  most obvious edit ecosystems.
- **Duplicate-work lesson:** PR #222 was dispatched while #221 (same feature) was
  already open. **Run `gh pr list` before dispatching an agent.**

## How to verify

```bash
cd ~/workspace/civit/cli && git merge --ff-only origin/main
go build -o /tmp/civitai ./cmd/civitai

# 1. txt2img estimate — zero spend
/tmp/civitai generate "a cat" --dry-run            # expect ~8 Buzz + the no-cap caveat

# 2. img2img estimate — zero spend (needs --ecosystem or images are silently dropped)
/tmp/civitai generate "make it blue" --ecosystem Flux1Kontext --image ./some.jpg --dry-run

# 3. Model substitution, live, zero spend — a REAL id from the wrong ecosystem
/tmp/civitai generate "a cat" --ecosystem NanoBanana --checkpoint 128713 --dry-run
#    expect: substitution warning, requested 128713 -> applied 2154472, reason "unrecognized"
#    control: --checkpoint 2436219 (correct) -> 160 Buzz, NO substitution, NO warning

# 4. Spend gates still refuse in CI
/tmp/civitai generate "a cat" >/dev/null 2>&1; echo $?          # 1 = refused (non-TTY, no --yes)
/tmp/civitai workflows cancel 8753561-9999999999 >/dev/null 2>&1; echo $?  # 1 = refused

# 5. Full gate (counted, never the exit code)
go test ./... -count=1 -v > /tmp/t.txt 2>&1
grep -c -- '--- PASS' /tmp/t.txt; grep -c -- '--- FAIL' /tmp/t.txt; grep -c 'panic: test timed out' /tmp/t.txt
gofmt -s -l .   # must print nothing
nix-shell -p golangci-lint --run 'golangci-lint run'

# 6. Buzz spend check — green/yellow are the reliable signal; blue drifts on its own
/tmp/civitai buzz --json
```

**Balance reference at handoff:** green `1000000`, yellow `1849049` — unchanged for the
whole feature. If either moves, something spent.
