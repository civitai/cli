# Handoff: civitai-generate — 2026-08-07

## Goal

Ship `civitai generate` — text-to-image and image-to-image from the CLI — safely, on a
path that **spends the user's real Buzz irreversibly**. Feature is shipped and merged;
what remains is follow-up hardening and one open platform issue.

## State now

- **Branch:** `main` (base clone is **1 behind** `origin/main` — `git -C ~/workspace/civit/cli merge --ff-only origin/main` first).
- **Working tree clean** except three pre-existing untracked files in `claudedocs/`
  (`img2img-anime-research.md`, `photo-to-anime-workflow.json`, `verify_workflow.py`) —
  **not mine, leave them.**

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

- **Follow-up agent running** in worktree `.claude/worktrees/agent-af84d3a63080c18a3`,
  branched from `ba3136c`. Two fixes (details in "Open investigations"). It will open a
  PR. **Check for that PR before starting new work on `generate_substitution.go` /
  `generate.go`.**

### Deploy/verify status — honest

- **Verified live against production, end-to-end:** txt2img (8 Buzz), img2img
  (45 Buzz), and a real model substitution (read-only, 0 Buzz). See "How to verify".
- **Total spend this work: 53 Buzz.** Green/yellow balances untouched throughout —
  only blue moved, and blue drifts on its own (observed ±63 with no action).
- Not released: no `v*` tag pushed.

## Open investigations — live diagnosis state

### 1. Advisory name lookups can delay the workflow id of an already-charged job

- **Symptom + exact repro:** after a successful submit, `printModelSubstitutions`
  resolves substituted version ids to names *before* the workflow id reaches the user
  and before the pending record is written. The job is already charged; the id is the
  user's only handle on it.
- **Observed (values):** `ResolveModelVersion` → `getWithRetry`
  (`pkg/civitai/retry.go`, `readMaxAttempts = 4`) on a client with
  `defaultTimeout = 30s` (`pkg/civitai/api.go`). Worst case ≈2 min **per id**, two ids
  per record. Call site was `internal/cmd/generate.go` post-submit render, before
  `recordPendingWorkflowID`.
- **Ruled out:** not a parsing problem — `parseModelSubstitutions` is already lenient
  by design so a malformed advisory field cannot cost the user the handle
  (8 malformed shapes pinned, each asserting `res.ID` survives). The render path
  reintroduces the same exposure by a *different* mechanism.
- **Leading hypothesis:** bound the lookups (short total `context.WithTimeout`) or
  print the id first and enrich after. On timeout the report must still appear **with
  ids instead of names** — never silently drop it.
- **Next probe:** the in-flight agent is fixing this. If it failed, verify the
  constants first: `grep -n "readMaxAttempts\|defaultTimeout" pkg/civitai/retry.go pkg/civitai/api.go`.

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
- **Leading hypothesis:** annotate (`… (SUPERSEDED — see above)`) or show both. Reuse
  AGENTS item 21's phase model (pre-spend "will run" vs post-spend "ran"); do not
  invent a parallel notion of tense.
- **Next probe:** in-flight agent is fixing this too.

### 3. #3667 — prompt-audit coverage guard (platform, OPEN)

- **Symptom:** the content audit is keyed by hardcoded field names — `prompt` +
  `negativePrompt` (`orchestration-new.service.ts:1452`) and, separately,
  `musicDescription` + `lyrics` (`:1495`). Nothing enforces that a new text-bearing
  graph node gets an audit block.
- **Observed:** `data` is rebuilt from **declared graph nodes only**
  (`data-graph.ts` `_validate`, ~:735-765), so the gate is necessarily name-based.
  ACE Audio needing its own block is proof the pattern recurs. No test in
  `src/server/services/orchestrator/__tests__/` asserts call-site coverage.
- **Ruled out — IMPORTANT, do not re-derive:** there is **no known bypass**. An earlier
  read suggested several ecosystem graphs lack a `prompt` node; they **compose** a
  shared one (e.g. `flux-graph.ts:269` merges `promptGraph`). ACE Audio's fields are
  already covered at `:1495`. Filing this as a security hole would be an overclaim.
- **Leading hypothesis:** ask for an asserted ledger (text-bearing nodes vs audited
  fields) failing when the first set grows.
- **Next probe:** none needed from the CLI side; it is a platform ask awaiting triage.

## Next steps (ranked)

1. **`git merge --ff-only origin/main`** in the base clone (1 behind).
2. **Wait for / review the in-flight follow-up PR** (investigations 1 and 2). Verify its
   gate yourself — re-run its mutations, don't trust the report.
3. **`--input` is txt2img-only on purpose.** Unblocking video/audio/comfy (all 51
   ecosystems, free via `--input`) is gated on #3667's answer. That is the single
   biggest capability unlock left.
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
