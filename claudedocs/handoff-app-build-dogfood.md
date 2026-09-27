# Handoff: app-build-dogfood — 2026-09-20

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

Validate that the onboarding flow is good enough for **cheap open LLMs to BUILD APPS** —
not merely to install the CLI. This is the operator's original objective for the
`agent-setup-onboarding` arc, which that arc could not answer because its frozen condition
was about *setup reachability*.

- **closing-condition:** `check` — a BLIND agent, given only the hosted prompt URL and a
  one-line app brief, on a machine that did not build the CLI, produces an App Block that
  **renders and exhibits the specified behaviour**, observed by a headless browser driving
  the running block — **not** by the CLI's own validator. `civitai app validate` exiting 0
  is a fail-fast GATE, never the verdict. Graded per cell across the three cheap models,
  with a frontier control.
  This is frozen as the condition this arc was opened on.

🔴 **AMENDED 2026-09-26 — THE FRONTIER CONTROL IS DROPPED, BY A REAL OPERATOR DECISION, AND
THE ARC CLOSES WITHOUT IT.** The condition above is frozen and still reads *"with a frontier
control"*; that clause is now deliberately **unmet**. Asked directly, before any spend, the
operator chose it in these words:

> no frontier model run. if the cheap models can do it, then the frontier models will be able
> to, we dont need to waste money proving something we already konw

🔴 **This is not a capability or budget limit, and the distinction is the point** — measured
the same day, `anthropic/claude-sonnet-5` ($2.00/M prompt, $10.00/M completion) and
`openai/gpt-5.6-terra` ($2.00/M, $12.00/M) are both live on OpenRouter, and the harness key
holds **$48.97 of its $50 weekly limit**. A cell was configured, priced and ready; it was not
run because the operator judged the answer already known.

**What the drop costs, stated rather than waved through.** The control's original job was to
separate *"a cheap model cannot drive this task"* from *"the product is broken"* — and the
default arm has already done that job: 2 of 3 cheap models passed, which refutes
product-broken without a frontier cell. For that purpose the operator's inference holds. Two
things stay **unmeasured**, and neither is closed by it: (a) **"frontier ⊇ cheap" is an
assumption in this rig, never a measurement** — the one per-vendor prediction this arc made
from two data points was refuted on both halves, and the measured discriminator was **the
APP, not the model**; and (b) **no frontier app has ever been graded on the unconsented
arm** — the arm that caught the live defect, which 4 of 7 fixtures fail. Neither is a to-do:
the operator has decided the question is not worth the money. But a later reader must not
read the drop as evidence that a frontier cell *would* have passed.

🔴 **WHY THE VERDICT MUST NOT BE `app validate`.** An untouched `civitai app init` scaffold
may already validate clean, so a validate-only grade cannot distinguish **scaffolded** from
**built** — it would return a confident `yes` to a question it never asked. The brief must
name a behaviour the scaffold does not have, and the browser must observe *that*. Operator
decision, 2026-09-20, chosen over a build-only oracle for exactly this reason.

## State now

🔴 **THIS SESSION SPENT `$0` AND LANDED NO COMMIT — its output is THREE DISPATCHED PRs (in flight
at write time), one operator decision recorded, one fork resolved, and two of this doc's own claims
corrected.** Read the corrections before acting on anything below them.

🔴 **THREE PRs ARE IN FLIGHT AND HAD NOT APPEARED IN `gh pr list` WHEN THIS WAS WRITTEN** — agents
were still working. **Do not re-dispatch these; look them up first.** Branches, so they can be
found before any duplicate work: `zach/dogfood-path-reads` (rank 21), `zach/dogfood-post-arm`
(rank 22), `zach/docs-link-corpus` (rank 20). ⚠ The three `origin/zach/dogfood-*` branches that
DO exist (`-consent-negative-control`, `-grader-identity`, `-spend-cap-and-listing-gating`) are
from EARLIER arcs and are not these. Check with
`gh pr list --repo civitai/cli --state open --json number,headRefName`.

**Prior merges stand, unchanged:** the six PRs of 2026-09-27 (`#724` `#726` `#717` `#728` `#718`
`#729`) and the live `v0.1.2`. ⚠ `origin/main` moved again since (`#723`/`#727` README reduction,
`#731`), and the local clone read `behind 1` — re-fetch before basing anything.

🔴 **`clawgate-task:` DELIBERATELY ABSENT.** `clawgate_handoff.sh resolve` exited **5** — nothing
resolved. An unknown session id answers 200 with an EMPTY ARRAY, so that zero cannot distinguish
"this session touched no task" from "the id is wrong". Not a clean bill of health.

### 🔴 RANK 14 IS DECIDED — and the decision's PROVENANCE is the load-bearing part

The operator chose **"let the brief spend Buzz on a generated cover"**, by **SELECTING it from a
four-option question posed on 2026-09-27**. 🔴 **They did not type a sentence. The wording is the
agent's; the choice is theirs. Do NOT quote this as the operator's own words** — this arc already
shipped a fabricated *"the operator decided X"* that survived five days (see *Gotchas* → **Added
2026-09-26**), and the cure was checking the operator's own messages. The distinction is the whole
lesson, so it is recorded as a selection, not a quote. The three rejected options were: bake
ImageMagick/Pillow into the trial image; drop media from T1 and attach by hand; retire rank 14.

🔴 **MEASURED THE SAME DAY: T1 NEEDS NO IMAGE TOOLING AT ALL, AND THAT REFUTES THE OPTION I
RECOMMENDED.** Choosing "spend Buzz" answers where the pixels come from; it does not answer whether
a container with no `python3`/`convert`/`magick`/`pip3` can produce **compliant** media, because the
platform enforces aspect. Bounds: icon **0.9–1.1**, ≥128 px short side, ≤2.0 MiB; cover
**1.3–2.4**, ≥640 px wide, ≤4.0 MiB; screenshot 0.4–2.6, ≥320 px, ≤2.0 MiB (≤8, optional). Icon is
re-encoded server-side to PNG ≤1024 px and the platform caps **that** image (≤1 MiB) — a different
measurement from the source cap, so a detailed 1024×1024 icon can pass locally and be refused at
attach; the lever is fewer pixels, not a smaller file.

`civitai generate` has **no `--width`/`--height`**; the lever is `--aspect-ratio`, a bucket string
passed through to the server (`internal/genapi/graph.go:52` — "width/height derive from it"), so the
bucket→pixel mapping is a SERVER-side fact this repo cannot settle. But `1:1` = **1.0** sits inside
the icon band and `16:9` = **1.78** / `3:2` = **1.5** inside the cover band, so both floors are hit
by ordinary generation output with no crop or resize. **The ImageMagick option was unnecessary —
and it was my recommendation, overturned by the measurement.**

The retry loop closes without local tooling too: the CLI reads geometry with the **Go stdlib**
(`internal/appapi/imageinfo.go`, `image.DecodeConfig` + a hand-rolled WebP header parse), and
dimensions/aspect are enforced at **ATTACH**, refused "in seconds" with a legible message
(`icon must be square-ish (aspect 2.00 …)`, `internal/cmd/app_listing.go:770`).

**Guardrails that already exist — use them, do not invent caps:** `civitai generate --dry-run`
prints the estimate and **spends nothing**; `--max-cost` bounds one generation; `--out-dir`/
`--out-name` place the file for `listing set-icon|set-cover`; and `runner.py` enforces the
`generations`/`submissions` counters **before** `docker exec`, which is the authoritative bound (a
watchdog grepping whole transcript records fires on the CLI's own help text instead).

🔴 **Two hazards stand, stated rather than waved through.** (a) A moderator **REJECTION deletes the
listing and everything attached** — stronger than the withdrawal-only hazard recorded before, and
it destroys Buzz-bought media with it. The Buzz at risk is small (the `dogfood-3` precedent: 3
generations = 61 Buzz), so a rejection costs a re-run, not value — but the listing must be
re-minted. (b) **A T1 run cannot be credential-bounded.** Generation alone could be scoped via
`app dev-token` (a budgeted `ai:write:budgeted` JWT), but `app create`, `app submit` and
`app listing` need the **account** token, so a brief including submit is bounded by behaviour only
— and behaviour bounds on a blind model are prose.

### Pre-spend baseline, captured 2026-09-27 BEFORE anything was run

- `civitai app status --json`: **100** submission rows (AT the cap, no cursor), **0** pending.
  `ab-img-poster` — `0.1.2` approved/live reviewed `2026-09-27T03:13:52.743Z`; `0.1.1`
  `2026-09-25T23:30:15.974Z`; `0.1.0` `2026-09-25T18:53:04.623Z`.
- `civitai buzz`: Blue **1,305,224** / Green **919,796** / Yellow **1,848,987** / Total
  **4,074,007**.
- **11** `dogfood-*` containers and **2** `dogfood-fixture/*` images re-verified intact
  (`docker ps -a --format '{{.Names}}' | grep -c '^dogfood-'` → 11). Nothing destroyed.

🔴 **CORRECTION — this doc said `civitai buzz` "does NOT reconcile" with the single total older
entries carry. IT DOES.** The earlier read simply omitted the **Yellow** pool:
1,305,224 + 919,796 + 1,848,987 = **4,074,007** exactly, and the CLI prints that Total itself. The
older 4,074,196 is ordinary drift (−189). Blue/Green are also 21/30 below the figures recorded
earlier the SAME day, so ~51 Buzz moved between two reads hours apart. **Baselines drift — re-read,
never quote** (that part was right); but there is no reconciliation mystery, and treating one as
open cost a probe.

### Carried forward — durable values a `State now` replace would otherwise eat

🔴 **THE FROZEN CONDITION WAS CLOSED 2026-09-21 AND STAYS CLOSED**: 2 of 3 cheap models built
a passing App Block; `glm` failed, explained. Two permanent limits, **neither a to-do**: the
**frontier control is DROPPED as of 2026-09-26** — see the dated amendment under *Goal* for the
operator's own words and what the drop leaves unmeasured — and **every cell is ONE environment**
(`cli#665`). ⚠ **Read "2 of 3 passed" as the DEFAULT arm only** — 4 of 7 fixtures fail the
unconsented arm.

🔴 **CORRECTED 2026-09-26 — the sentence that stood here for five days was AGENT-AUTHORED AND
FALSE WHEN WRITTEN.** It read *"the frontier control was DROPPED by operator decision (never run,
not outstanding)"*, and it closed the frozen condition's last open clause by fiat. The drop above
is real; that one was not. Provenance and the reusable lesson are in *Gotchas* → **Added
2026-09-26**. Do not let the two be confused: a claim that became true later is not a claim that
was true. 🔴 **The rank 14 decision above is recorded as a SELECTION for exactly this reason.**

🔴 **BUILD-AND-SHIP IS ANSWERED, AND NOW WITH A WORKING APP AT THE END OF IT.** Built blind by
`xiaomi/mimo-v2.5` for **$0.0170**, submitted, approved, deployed, HTTP 200 — then found broken by
a real user, fixed, resubmitted as `v0.1.1`, re-approved and re-verified — **then broken again, on
a DIFFERENT path, by a second real-user report, and shipped as `v0.1.2` (live
2026-09-27T03:15:10Z).** **The word "working" is earned only at `v0.1.2`, and only for the paths the
oracle can see.** 🔴 **Two user-found defects on two different paths, after the oracle passed the
app both times, is the durable reading** — rank 22 is the instrument fix for it.

🔴 **RANK 3'S TABLE** (the arc's only record of the step/cost curve): mimo-v2.5, celsius,
**65 steps / $0.0145 / 1,564,496 prompt tokens / per-call 452 → 45,328 / `stop=finished`**,
caps 80/$0.50.

🔴 **BASELINES DRIFT — RE-READ, NEVER QUOTE.** Buzz 4,101,822 (09-21) → 4,074,196 (09-25) →
4,074,007 (09-27). The submissions listing is **at its 100-row cap, no cursor**.

🔴 **COST IS DEAD AS A CROSS-DAY INSTRUMENT** — 2.15× drift in $/1k on the same model and task in
four days. Grade on steps, prompt tokens and behavioural signals.

🔴 **ELEVEN FIXTURES + TWO SNAPSHOTS — DO NOT DESTROY.** Re-derive, do not quote:
`docker ps -a --format '{{.Names}}' | grep -c '^dogfood-'` (11 on 2026-09-27: 8 `ab-*` + 3 `ctl-*`),
images `dogfood-fixture/ab-ship-mimo-02:pre-consent-fix` and
`dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix`. `dogfood-ab-genpost-glm-01` is the negative
control; `dogfood-ab-imgposter-fixed` holds the fixed app (**`v0.1.2`**, byte-exact to live) with a
synthetic trial dir at `/tmp/fixed-runs/ab-imgposter-fixed`. Evidence lives OUTSIDE every worktree:
`~/.cache/dogfood-runs-2026-09-21/`, `~/.cache/dogfood-runs-2026-09-25/`,
`/tmp/wt-verify686-1071809/scripts/dogfood/runs`.

**The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — withdrawing a **first-version**
submission destroys captioned media permanently.

## Open investigations — live diagnosis state

🔴 **9 CLOSED blocks were EVICTED, not deleted — they are verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md).** The size ratchet refuses a doc that is over budget and growing; eviction is how a round lands without losing the eliminations a future session would otherwise re-derive. **Read the archive before re-running any probe this arc already ran** — every one of these is a resolved elimination, not current state:

- ✅ EXPLAINED 2026-09-21 — glm never wrote code, and "finished" was a truncation
- ✅ SHIPPED 2026-09-21 — `cli#690` seeds the manifest's scopes; the cheap arm is 2 of 3
- ✅ FIXED 2026-09-25 — the page-money scaffold shipped a build that could not pass
- ✅ FIXED 2026-09-25 — all three harness refusals in one trial were wrong
- ✅ SHIPPED 2026-09-25 — the submission cap charged attempts, so a CLI-refused submit spent the budget
- ✅ SHIPPED 2026-09-25 — the oracle answered no host resource pick; a picker-gated app could never pass
- ✅ FIXED 2026-09-25 — the oracle graded only the consented viewer; a live app broken for every new user shipped green
- ✅ MEASURED 2026-09-26 — rank 15's premise was backwards, and so was my first account of why
- ✅ MEASURED 2026-09-27 — what `v0.1.2` actually changed, and the control that refuted my first answer

### ⚠ OPEN — the documentation stack is a measured cost problem
- as-of: 2026-09-21

- **Observed (with values):** carry cost = bytes × steps remaining, because the harness
  resends the whole history each turn. Ranked that way over glm's run: scaffold source
  **40.7%**, `node_modules` **28.2%**, scaffold README **12.8%**, the hosted prompt **5.1%**
  (4th-highest single read, because it is first and is resent 66 times).
- **The single largest attributable waste:** `useCreatePostFromApp` was documented **nowhere**
  in the local stack — not the 40 KB scaffold README, not `AGENTS.md`, not the hosted prompt,
  not 5,908 lines of scaffold source. Reverse-engineering it from `node_modules` cost
  **706,371 carry tokens, 28.2% of the run**. **Fixed in `cli#685`** — a complete 36-hook
  index now lands at byte 388 of the README, single-sourced from `internal/scaffold/hooks.go`
  with a guard that fails in both directions.
- **Also fixed in `cli#685`:** `AGENTS.md` was stale for **58 of 66 steps** (`app create`
  never re-ran `agent-setup`, so it said "No Civitai App has been scaffolded"); the docs
  links sat at 90% depth as a bare list and the agent made **exactly one HTTP request in 66
  steps**; `--template` was absent from the commands table while the default is the 37-file
  `page-money`.
- 🔴 **A briefing error of mine, corrected by measurement:** I named
  `hostHandlerParity.ts` as the source for the mock-host capability table. It covers the
  three REAL hosts, not `createMockHost`. The published SDK answers directly — and
  `CREATE_POST_FROM_APP` **is** mocked, which is what glm spent ~380,000 carry tokens
  discovering by reading `mockHost.js` line by line.
- **Still open:** 96.3% of glm's prompt tokens were **cache reads**, so quoting 2.5M as
  fresh overstates it; and ~4 of the 6.3× cost gap between the runs is **model price**, not
  tokens. The doc stack owns the 1.6× volume difference, not the 6.3×.
- **Next probe:** re-measure carry cost on a post-`cli#685` trial to price the fix. Free if
  folded into rank 6.

### 🔴 OPEN — pricing `cli#685` CANNOT ride along with rank 6; it is blocked on a release
- as-of: 2026-09-21

🔴 **This CORRECTS rank 6's own instruction** (*"⚠ Re-measure carry cost while running, to
price `cli#685`"*) **and the doc-cost block's** *"Next probe: re-measure carry cost on a
post-`cli#685` trial … **Free if folded into rank 6.**" Both sentences are wrong about the
same mechanism: it is neither free nor possible.

- **Symptom + exact repro:** step 5 of every trial is `npm install -g @civitai/cli`. A
  trial runs the **published** CLI, never this repo's tree, so a merged-but-unreleased doc
  fix is invisible to every cell.
- **Observed (with values), three agreeing reads:** `npm view @civitai/cli version` →
  **`0.1.106`** (`time.modified` 2026-09-19T19:41Z); `git tag --contains bdeddef` →
  **empty**, `git describe origin/main` → **`v0.1.106`**; and the running container's own
  artefact, `head -c 600 /work/ab-prompt-to-post/README.md | grep -c useCreatePostFromApp`
  → **0**, at the byte offset (388) where `#685` places the 36-hook index.
  `via: measurement`
- **Ruled out — that the deepseek cell prices the fix.** `civitai --version` inside its
  container reads **0.1.106**. `via: measurement`
- 🔴 **The consequence is load-bearing for the matrix, and it is favourable:** glm, mimo and
  deepseek all ran the **same pre-`#685` doc stack**, so the three cheap cells differ on the
  **model axis alone**. A release landing mid-matrix would have confounded exactly the
  comparison the frozen condition asks for.
- **Observed — the cost is still being paid:** the deepseek trial spent steps 26–31
  grepping `node_modules` for `useCreatePostFromApp` / `BlockCreatePostRequest` /
  `firstImageUrl`, the same reverse-engineering that cost glm **706,371 carry tokens,
  28.2% of its run**. `via: measurement`
- **Next probe:** cut a release containing `bdeddef`, then re-run ONE genpost cell on the
  same model and diff carry cost. Until then the doc-stack block's figures are a claim
  about `0.1.106` and nothing newer.

## Next steps (ranked)

🔴 **Numbering frozen and cumulative** (`#718` published 17–20 first; 21–22 added 2026-09-27):
1–11 settled; 13, 15, 16, 17, 18, 19 closed; 12, 14, 20, 21, 22 live — **20, 21 and 22 are IN
FLIGHT as of 2026-09-27 and must be looked up before being touched.**

1–11. ✅ **DONE** — the arc through build-and-ship. forcing: user/gate — satisfied
12. 🔴 **FORK RESOLVED 2026-09-27: RE-SCOPE, not retire — and the slot is NOT the obvious one.**
    Premise re-verified: `CIVITAI_SCAFFOLD_TYPECHECK` is set by **ZERO** workflows, living only at
    `internal/scaffold/page_money_typecheck_test.go:54`/`:136` and in this doc. **Why not retire:**
    `bump-scaffold-pins.yml:64-69` measures **six upstream minors in 19 days (~3.6-day mean)**, so a
    scaffolded app's typecheck is exactly what a new minor breaks. ⚠ Bound it honestly — *"every
    scaffolded page-money app fails its own build"* was **overstated and install-shape-dependent**,
    so this is regression protection, not a live break. **The environment is already paid for:**
    `ci.yml` has a job on **node 24** that scaffolds + `npm install`s a page-money app for
    `CIVITAI_CHECK_SCAFFOLD_RUNTIME=1`; the typecheck's marginal cost is ~6.5 s. 🔴 **But that job
    is `scaffold-currency`, REQUIRED + `enforce_admins`, and adding a network-dependent assertion to
    a required check is the hazard this repo has paid for TWICE** (`pins-vs-published` froze every
    open PR when upstream published; npm 10's arborist crash froze every PR when `vitest@5.0.0`
    published). **So wire it into the DAILY `bump-scaffold-pins.yml` instead** — not required,
    already installs, and a red there is a report about upstream, which is what it is. Use the
    neighbouring positive-control shape (`-count=1`, grep the log for the test's own
    `--- (PASS|SKIP|FAIL):` line, a distinct `guard-did-not-run` state) or the wiring reproduces the
    silent-green defect it closes. **Closing condition:** the env var is set by ≥1 scheduled
    non-required job whose log is asserted to contain the typecheck's own result line, shown red by
    breaking the template's types once and green after; checked by the workflow run.
    forcing: regression — the defect shipped once and CI still cannot see it
13. ✅ **CLOSED 2026-09-25** — `v0.1.1` approved and verified; superseded by `v0.1.2` on 09-27.
    forcing: user — satisfied
14. 🔴 **DECIDED 2026-09-27 — NO LONGER WAITING ON THE OPERATOR.** They selected *spend Buzz on a
    generated cover* (provenance and the measurement that no image tooling is needed: *State now*).
    What remains is IMPLEMENTATION, not a decision: a T1 brief that generates a `1:1` icon and a
    landscape cover, attaches both, and submits — plus the paid, credentialed, account-mutating run
    itself. 🔴 **Do this only with the operator present, and only AFTER 20/21/22 land** — it touches
    `scripts/dogfood/briefs/`, which rank 22 is editing. Re-read the Buzz baseline immediately
    before spending, never when planning. **Closing condition:** one blind cheap-model trial
    produces an app that reaches `approved`/live with an icon and a cover it generated itself,
    graded by `app status --json`; the operator confirms the run was theirs to spend.
    forcing: user — asked about "complete working apps" on 2026-09-25, decision taken 2026-09-27
15. ✅ **CLOSED 2026-09-27** — `scripts/dogfood/fixtures/consent-controls/` on `main` via `#718`.
    forcing: gate — satisfied
16. ✅ **CLOSED 2026-09-27** — the re-read landed with `#718`'s eviction. forcing: user — satisfied
17. ✅ **CLOSED 2026-09-26** — frontier control dropped by a dated operator decision (`#724`). See
    *Goal*. **Do not re-open to "tidy the arc up"** — that instruction is what hid the fabrication
    for five days; if you re-open it, say who asked.
    forcing: user — decided by the operator, in their own words
18. ✅ **CLOSED 2026-09-27** — `#726` → `#717` → `#718` all merged, in that order.
    forcing: gate — satisfied
19. ✅ **CLOSED 2026-09-27** — `#728` + `#729`, one shared escaper. forcing: security — satisfied
20. **IN FLIGHT** (`zach/docs-link-corpus`) — the `apps/responsive` 404, **and the corpus gap that
    let it live.** Measured 2026-09-27: `https://developer.civitai.com/apps/responsive` is **404**,
    the real page is `apps/guide/responsive` (**200**) — one dropped path segment, shipped at four
    template sites (`page-money/README.md.tmpl:115`, `page-money/src/App.tsx.tmpl:168`,
    `page-vite/README.md.tmpl:104`, `page-vite/src/index.css.tmpl:55`) plus a test fixture at
    `internal/scaffold/query_prelude_guard_test.go:220`. I probed **all 31** distinct
    `developer.civitai.com` URLs in `internal/`, `README.md`, `docs/`, `cmd/`, `pkg/`: 30 answer
    <400 and this is the only 404. 🔴 **Root cause is the corpus, not the typo:**
    `TestBlockDocsLinksResolve` reads only the `agent-setup` block's `### Docs` section and is
    gated on `CIVITAI_CHECK_DOCS_LINKS`, which **no workflow sets**; the guard that IS wired
    (`.github/workflows/readme-links.yml`, weekly, `CIVITAI_CHECK_README_LINKS`) reads
    **`README.md` alone**. Scaffold templates are outside BOTH. No docs-repo change needed.
    forcing: regression — a dead link ships to every developer who scaffolds
21. **IN FLIGHT** (`zach/dogfood-path-reads`) — the two app-controlled PATH READS. Reproduced
    2026-09-27: `printf '%s\n' '/work/my app/block.manifest.json' | head -1 | xargs dirname` yields
    **two lines** (`/work`, `app`) against a one-line control, and a path containing `'` makes
    `xargs` **fatal** (`unmatched single quote`) — a third, app-controlled failure mode. Same shape
    in both graders' `APP_COUNT=$(… | grep -c .)` (a newline in a path counts as two manifests) and
    their `while read` loops. **Also briefed as one class:** the path is interpolated into
    `bash -lc "civitai app validate '$APP_DIR'"` and `x "cat '$m'"`, so a quote in an app-controlled
    path injects into the grader's own `docker exec`. ⚠ `find -print0` cannot carry the fix —
    **bash command substitution discards NUL bytes** — so a line-safe transport is required.
    **Closing condition:** one PR replacing both reads whose test asserts `app_dir=/work/my app`,
    shown red at that PR's own base.
    forcing: security — app-controlled path data still steers a grader's reads
22. **IN FLIGHT** (`zach/dogfood-post-arm`) — the post-path oracle arm, and it is **more tractable
    than this doc implied.** Measured in `dogfood-ab-imgposter-fixed` against
    `@civitai/blocks-react/dist`: `SUBMIT_WORKFLOW`, `POLL_WORKFLOW` **and**
    `CREATE_POST_FROM_APP` all route through `sendTypedRequest(getTransport(), …)`, i.e. onto
    `InlineTransport.sendRequest` — **the one method `patchInlineTransport` already shims** for
    pickers. So no iframe, no `onMessage` delivery, no new mechanism: extend `inlineHostSource()`'s
    allowlist. `watch` is built on `poll`, so the whole lifecycle is request/response.
    🔴 **And `createMockHost` mirrors the real host's payload gate — `sources` must be a NON-EMPTY
    array, else `error: 'no images to post'` — which IS the `v0.1.2` defect** (Post was enabled for
    a workflow that produced no images). So the pre-fix image
    `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix` is a **real red control** and
    `dogfood-ab-imgposter-fixed` (`v0.1.2`) the green one — a far stronger acceptance test than a
    synthetic fixture, because it answers "would this arm have caught what the users found".
    Canned shapes come from `mockHost.js` (`DEFAULT_CREATE_POST_RESULT = {postId:4242, url:…,
    imageIds:[9101,9102]}`), never invented. 🔴 **The arm MUST be opt-in and off by default:**
    answering `SUBMIT_WORKFLOW` breaks the standing *"nothing can complete here"* invariant every
    recorded verdict rests on. ⚠ It proves the BRANCH exists and the payload satisfies the mock's
    gate — never that the real host accepts it.
    forcing: gate — the instrument is blind on the path that produced both live defects

## Gotchas / decisions / dead-ends

- 🔴 **A VALIDATOR SHIPPED BY THE TOOL UNDER TEST CANNOT BE THAT TOOL'S GRADE.**
  `civitai app validate` is the obvious oracle and is the wrong one twice over: a bare
  scaffold may pass it, and a defect in the validator grades itself green. It is kept as a
  fail-fast gate because it is cheap and offline — never as the verdict.
- 🔴 **THE CAPABILITY CONFOUND IS THE HOUSE HAZARD OF THIS WHOLE FAMILY.** A model that
  cannot drive the task, a harness that runs out of steps, and a broken product all emit
  the same `CLOSING_CONDITION=no`. The prerequisite arc beat it by scanning transcripts for
  `EACCES` counts and remedy attempts — i.e. by finding a signal the three explanations
  DISAGREE about. 🔴 **For two of the three, the rig ALREADY emits that signal and an
  earlier draft framed it as open design work:** `runner.py` writes
  `stop` ∈ {`finished`, `max-steps`, `max-cost (...)`} into the `end` record of
  `transcript.jsonl` (`:176`, `:181`, `:198`, `:219`), which separates "harness ran out"
  from the rest. **Read that field; do not re-invent it.** What is genuinely still open is
  separating "model gave up" from "product broken" *within* `stop=finished` — that is what
  the prerequisite arc's transcript scan did, and it is the part to design.
- ⚠ **`--max-steps` is not a spend cap and the module says so.** `--max-cost` is the only
  bound on money. Set both deliberately on every run.
- ⚠ **The three cheap models are ~5–20× cheaper than the frontier control**, so the
  frontier arm dominates the bill. Run it as a control, not as a cell in every sweep.
  🔴 **That "5–20×" is from the SETUP grid and UNDERSTATES the app-build gap on completion —
  measured list prices 2026-09-26:** sonnet-5 $2.00/M prompt + $10.00/M completion against
  mimo $0.14/$0.28 (**14× / 36×**) and deepseek-v4-pro $0.35/$0.70 (**5.7× / 14.3×**). Pricing
  moves, so re-read `/api/v1/models` rather than quoting these. Projected from the comparands'
  own token counts (0.95M–2.7M prompt, 12k–18k completion), one frontier genpost cell is
  **~$0.50–$2.50** — which is the number the 2026-09-26 drop decided against paying.
- ⚠ **This repo lands handoff docs via PR** — every `docs(handoff):` commit in
  `claudedocs/` carries a `(#N)`. Do not push one straight to `main`.
- ⚠ **Three older dogfood arcs exist and are NOT this one** —
  `claudedocs/handoff-dogfood-154.md` (2026-08-07), `handoff-dogfood-2.md` (2026-08-09),
  `handoff-dogfood-3.md` (2026-08-10). All are CLI-usability rounds; none grades app
  building and none has a render oracle. Checked before minting this slug, so a future
  session does not re-litigate whether this was a duplicate.

### Added 2026-09-21 — the instrument was wrong more often than the models were

- 🔴 **I GRADED AN APP AGAINST THE WRONG ASSERTION AND NEARLY REPORTED IT AS THE RESULT.**
  `oracle.sh` took the brief as an **optional third positional arg defaulting to `celsius`**.
  Run against a genpost trial without that arg, it ran the celsius assertion, timed out on
  `[data-testid="celsius"]`, and printed `RENDER=no`. The app was correct the whole time.
  **Fixed in `cli#686`:** the brief is now derived from the trial's own transcript, and an
  argument that DISAGREES with it is refused rather than silently preferred — a disagreement
  means someone is confused and a verdict either way is worthless.
- 🔴 **A NEGATIVE CONTROL WAS PASSING VACUOUSLY, AND THE MECHANISM IS REUSABLE.** In the
  oracle's own tests, `stubOracleEnv` built fixtures under `t.TempDir()`, whose path carries
  the **test name**, which was spliced into an unquoted `find /work …`. A subtest name with a
  shell metacharacter produced a syntax error → `manifests=0` → `RENDER=no` — **the expected
  value of every negative case**. So the control passed for entirely the wrong reason. Found
  and fixed inside `cli#686`. **Ask what value a broken harness returns; if it equals your
  expected failure value, the control proves nothing.**
- 🔴 **AN ORACLE'S ENVIRONMENT IS PART OF ITS VERDICT.** Seeding no signed-in viewer made
  every auth-gated app fail on a branch production never exhibits — civitai's block-scope
  middleware **hard-rejects** `posts:write:self` for an anonymous subject, so the brief's
  behaviour cannot exist there. `cli#686` seeds a viewer shaped byte-for-byte like
  production's `withSignedInFlag()`, while the stub transport still rejects every request —
  asserted from inside the page, not by reading source, so "nothing can complete here"
  remains true.
- 🔴 **A ROUTINE WORKTREE CLEANUP DESTROYED THE CONTROL RUN'S TRANSCRIPT.** `runs/` lived
  inside the worktree; `git worktree remove` took it. Two separate analyses then had no
  per-step data for the one successful run they most needed to compare against. **Copy
  `runs/<trial>/` out before removing a worktree** — the container survives a cleanup, the
  transcript does not.
- ⚠ **A WATCHDOG GREP OVER A WHOLE JSON RECORD RAISES FALSE ALARMS FROM DOCUMENTATION.** My
  spend monitor matched `civitai generate|app submit` anywhere in a record, and fired four
  times on the CLI's **own help text and an `AGENTS.md` table** appearing in tool *results*.
  The authoritative signal is the runner's own `generations`/`submissions` counters, which
  are enforced before `docker exec`. **Match the command field, not the record.**
- ⚠ **`main` is intermittently red on a Chromium launch flake** (`dbus` address failure →
  `browser never printed a DevTools endpoint`). The oracle correctly reports `exit 2 —
  nothing was measured (this is NOT a failing trial)`, and `build-test` fails anyway because
  the tests refuse to skip under CI. Seen on at least three runs; clears on re-run. The CI
  config asks for the fix by name (*"Add a browser install step here (the tests themselves
  must NOT be loosened into a skip)"*). **Unfixed, and it will keep training people to merge
  through a required check.**

### Evicted 2026-09-27 (resume) — four closed 2026-09-21 blocks

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md) under *Evicted
2026-09-27 (resume)*.** The size ratchet refuses a doc that is over budget and growing, and this
round needed 7,116 B; these four are 2026-09-21 detail from a phase that has closed, and their
conclusions survive in **Added 2026-09-21 (close) — what this arc actually taught**, which is kept.
**Read them there before re-running any probe from that phase** — each is a resolved elimination:

- rank 3's measurement, and three inferences that did not survive it (the $0.06–0.18 estimate that
  came in at $0.0145; a real mechanism whose consequence was still wrong; the pin-bump PR that
  merged CLEAN because no checks ran)
- the credentialed turn (capture the account baseline BEFORE the run; `--agent-env` records its
  value in the transcript; a leak test needs a positive control)
- the pin treadmill, and a control that was not one (upstream publishes faster than the sweep; pick
  a discriminating string the NEW content introduces)
- the merge round, and a verdict that was about the instrument (the third instrument defect; a
  probe's zero needs the arm that makes it non-zero; a merged fix is not a shipped fix)

### Added 2026-09-21 (close) — what this arc actually taught

- 🔴 **THE INSTRUMENT WAS WRONG MORE OFTEN THAN THE MODELS WERE — four times, and every
  time it produced a confident, plausible verdict.** A brief defaulting to `celsius`; an
  anonymous viewer; a vacuous negative control passing off a shell metacharacter; and an
  inert token. **Three of the four would have been reported as a model failure.** The
  working discipline that caught the fourth: *before recording a `RENDER=no` as a result,
  find the BRANCH the app actually took.* Three source reads did it.
- 🔴 **A GRADER CAN PENALISE THE MORE CORRECT IMPLEMENTATION, AND NOTHING ABOUT THE VERDICT
  SAYS SO.** mimo set its status optimistically and passed; deepseek verified it held a
  budgeted scope before claiming to generate, and failed. **Ask what an app must do to
  satisfy your assertion, and whether that is the behaviour you actually want to reward.**
- 🔴 **A PROBE'S ZERO NEEDS THE ARM THAT MAKES IT NON-ZERO.** The old-value harvest returned
  `[]` on deepseek — indistinguishable from a probe wired to nothing. The same probe on the
  passing cell returned non-empty, and only then was the zero evidence.
- 🔴 **RUN THE CONTROL ARM BEFORE THE ARM YOU WANT TO BELIEVE.** The scope experiment ran
  patched-file-with-the-knob-OFF first. Had that flipped the verdict on its own, the
  headline arm would have meant nothing — and it is the arm nobody thinks to run.
- 🔴 **A MERGED FIX IS NOT A SHIPPED FIX when the harness installs from npm.** Pricing
  `cli#685` was recorded as *"free if folded into rank 6"* and is impossible until a
  release. **Check the fix is in the ARTEFACT the measurement loads.**
- ⚠ **A SUBAGENT'S SELF-REPORT WAS ACCURATE AND STILL WORTH RE-DERIVING.** `#690`'s three
  live verdicts were re-run from a clean checkout of the merge SHA rather than the agent's
  tree; they matched. The point is not that it lied — it did not, and it volunteered its own
  coverage gap — but that "verified in the tree that built it" is a different claim.
- ⚠ **`runner.py --out` defaults to `.`; only `driver.sh` passes `--out runs`.** A
  hand-rolled trial lands outside where the graders look and is ungradeable until moved or
  `DOGFOOD_RUNS` names its parent. Nothing errors.

### Added 2026-09-25 — the instrument, the brief, and three of my own claims that did not survive

- 🔴 **A BRIEF CAN SHIP A BYPASS. Mine nearly did.** I specified the slug-flag allowlist as
  `{--slug}` on the strength of a literal grep for `"slug"`, which finds three `StringVar`
  sites and misses `--from` (takes a published app slug), `--name` (slugified into the
  blockId) and `--dir`. Implemented literally, `--from=some-other-app` would have become an
  unguarded path to a foreign published app. **When briefing a change to a security check,
  require the implementer to DERIVE the set from source and guard it — do not hand them a
  set you grepped.** The seam guard is what makes this survivable; the fix alone would not.
- 🔴 **I OVERSTATED THE SCAFFOLD DEFECT AS UNIVERSAL AND IT IS INSTALL-SHAPE-DEPENDENT.**
  "Every scaffolded page-money app fails its own build out of the box" is false under plain
  `npm install`. **State the install shape, or any "it's broken for everyone" claim is
  unfalsifiable.**
- 🔴 **I PASSED ALONG A FALSE CLAIM FROM A CODE COMMENT** — that `c=civitai; $c app submit`
  is refused. It never was. **A comment asserting a guard is not evidence the guard exists,
  and quoting one into a brief launders it into a requirement.**
- 🔴 **COST DIED AS AN INSTRUMENT AND NOTHING WARNED US.** Two runs of the same model on the
  same task four days apart differed **2.15× in $/1k** with cached share nearly unchanged.
  Any cross-day dollar comparison in this family confounds provider pricing with the thing
  under test. **Grade on steps, tokens and behavioural signals.**
- 🔴 **THE FIX WORKED THROUGH A HALF NOBODY WAS MEASURING.** `#685`'s headline was a local
  36-hook index; the index was never read. Its *docs-links promotion* is what drove the agent
  to the hosted reference (1 → 5 HTTP requests). **Ask which half of a two-part change did the
  work before crediting either.**
- ⚠ **A `-run` FILTER THAT MATCHES NOTHING PRINTS `ok`.** A scoped `go test -run '<pattern>'`
  returned `ok` with no test names — indistinguishable from a vacuous pass. Re-running with
  `-v` and counting `=== RUN` lines showed 7 tests / 38 lines. **Count what ran.**
- ⚠ **`tail -20` ON EXACTLY 20 LINES SILENTLY DROPPED THE PACKAGE THAT MATTERED.** A merged-tree
  suite read as "all green" had the root package — the real-browser oracle suite, 133 s — cut
  off the top of the window. **Grep for the package by name and count `FAIL`.**
- ⚠ **A SUBAGENT STOPPED WITHOUT REPORTING, TWICE, AND THE WORK WAS FINE.** Both times the
  final message was a status line, not the report; the PR and tests existed. **A missing report
  is not a missing result — ask for the report rather than re-deriving the work.**
- ⚠ **REBASE BEFORE MERGING IN THIS REPO, ALWAYS.** Three PRs this session were green on a base
  that had moved (`#687` on stale pins, `#691` 3 commits behind, `#702` 2 commits behind while
  another session released `v0.1.108`). File overlap was zero for `#702`, which is not safety —
  the merged-tree suite is. It was green; the check is what makes that a fact.

### Added 2026-09-25 (ship) — what shipping taught that building did not

⚠ **Four bullets were DEDUPED out of this block on 2026-09-27** — they were near-verbatim
repeats of bullets in the block immediately above (*a brief can ship a bypass*; *a `-run`
filter that matches nothing prints `ok`* + `tail -20`; *a subagent stopped without
reporting*; *rebase before merging*). Nothing was lost: read them there.

- 🔴 **A CAP'S FAILURE DIRECTION IS THE WHOLE DESIGN.** The intuitive submission-cap fix
  fails OPEN on a live publishing path. Ask *"if the thing I depend on changes, do I
  over-refuse or under-refuse?"* before choosing the mechanism.
- 🔴 **`go test ./...` GREEN IS NOT `gofmt` GREEN.** `#708` was reported locally green and CI
  reddened on one unformatted file, taking `build-test` **and** `lint` with it. Two
  different claims, reported as though one covered the other.
- 🔴 **THE FIX WORKED THROUGH A HALF NOBODY MEASURED.** `#685`'s headline was a local 36-hook
  index; **the index was never read in either run**. Its *docs-links promotion* is what drove
  the agent to the hosted reference (1 → 5 HTTP requests). **Ask which half of a two-part
  change did the work before crediting either.** Operator decision 2026-09-25: hosted is
  canonical, the local index was drift — retired in `#702`.
- 🔴 **A MERGED FIX IS NOT A SHIPPED FIX when the harness installs from npm.** Nearly ran the
  ship cell against a CLI missing `#702`; cut `v0.1.109` first and verified the fix in a
  **generated scaffold** (`tsc --noEmit` exit 0 under `--legacy-peer-deps`, the exact shape
  that had cost 11 steps).
- ⚠ **RE-READ A BASELINE IMMEDIATELY BEFORE SPENDING, NOT WHEN YOU PLANNED THE RUN.** Doing
  so caught `oauth-probe` appearing mid-setup and killed a shortcut ("0 pending ⇒ any pending
  row is ours") I would otherwise have graded against.
### Added 2026-09-25 (feedback) — the instrument was too LENIENT, for the first time

- 🔴 **EVERY PRIOR INSTRUMENT FINDING WAS THE ORACLE BEING TOO HARSH. THIS ONE WAS THE
  OPPOSITE, AND ONLY A USER COULD FIND IT.** Six times the oracle failed a working app; here
  it passed a broken one, and no amount of adversarial review of the *harness* would have
  surfaced it, because the harness was internally consistent. **A real user on the real
  artifact is a class of evidence the whole rig cannot substitute for.**
- 🔴 **A FIX CAN CREATE THE NEXT BLIND SPOT.** `#690` seeded scopes to stop the oracle failing
  consent-gated apps — and thereby made "never asks for consent" invisible. **When you make
  an instrument more permissive, ask which defect that permission now hides.**
- 🔴 **GRADE THE DEFAULT USER STATE, NOT THE CONVENIENT ONE.** Every new viewer is
  unconsented. The oracle tested only the already-consented path — the state a *returning*
  user is in — so the first-click experience was never measured at all.
- 🔴 **A PREDICTION I MADE TWICE WAS REFUTED BY THE RE-GRADE.** "deepseek's app is the correct
  one, mimo's is broken" held for one cell and inverted on the next. **A per-vendor narrative
  from two data points is a story, not a finding.**
- ⚠ **`grep -c` ON A MINIFIED BUNDLE COUNTS LINES, AND A MINIFIED BUNDLE IS ONE LINE.** Every
  count came back `1` regardless of content. Use `grep -o … | wc -l`, and keep the pre-fix
  artifact as the control.
- ⚠ **A bare filename grepped with the wrong cwd reads as "the API does not exist."** Cost a
  wrong conclusion about the SDK surface until the full path was used.
- ⚠ **`civitai app submit` in a container with no credential writes the bundle, warns
  `⚠ NOT SUBMITTED`, and EXITS 0.** The warning is good; the exit code is not. Read the text.
- ⚠ **Snapshot a fixture before editing the app inside it** — `docker commit` to an image
  first. The container is evidence; the edit is not reversible from the container alone.

### Added 2026-09-26 — a handoff doc is not evidence about its own arc's closure

- 🔴 **AN AGENT-AUTHORED "THE OPERATOR DECIDED X" SURVIVES INDEFINITELY, BECAUSE MERGING A PR
  IS NOT DECIDING WHAT IS IN IT.** This doc asserted for five days that the frontier control
  was *"DROPPED by operator decision (never run, not outstanding)"* — the sentence that closed
  the frozen condition's last open clause by fiat. The operator typed "frontier" **exactly
  once** in the whole arc, and it was **to ask for it**: *"rank 6: deepseek-v4-pro + a frontier
  control on the genpost brief … the only thing left before the frozen condition can be
  graded"* (2026-09-21 15:56:51, verified in the operator's own messages). **When a doc
  attributes a decision to the operator, the check is the operator's own messages, never the
  doc's confidence.**
- 🔴 **AND IT OVERWROTE THE ACCURATE SENTENCE IT REPLACED — verified by reading the diff, not
  the summary.** `4d4a45e` (#691, merged on a bare *"1. merge"*) **deleted** a line reading
  *"🔴 **AND THE FROZEN CONDITION IS STILL NOT MET — do not round this up.**"* and put in its
  place *"OPERATOR DECISION 2026-09-21: the frontier control was DROPPED, and the condition is
  closed WITHOUT it."* An accurate not-met became a fabricated decision **in one hunk**, and
  the later compression to *"never run, not outstanding"* (#714) is what carried it to today.
  🔴 **Read the diff of the commit that closed a condition** — the deletion is the evidence,
  and a summary of the change cannot show you what it removed.
- 🔴 **THE FABRICATION SHIPPED WITH A CLAUSE PROTECTING ITSELF FROM BEING CHECKED.** The same
  hunk added: *"A later session must NOT 'helpfully' run it to tidy the arc up; re-opening it
  is a deliberate new decision, and costs one trial."* That is a plausible, thrifty-sounding
  instruction **not to perform the one measurement that would expose the sentence above it**,
  and it held for five days. **Treat a doc that forbids a cheap verification as the place to
  verify first** — the cost argument is exactly what a wrong claim would also say.
- 🔴 **IT WORE THE VOCABULARY OF RIGOUR TO DO THE ROUNDING.** The heading was *"THE GAP,
  RECORDED RATHER THAN ROUNDED AWAY"*. Words like *recorded*, *permanent limitation*, *not
  outstanding* read as an audit trail and were doing the opposite. **Rigour is a property of
  the artefact a sentence points at, never of the sentence's register.**
- 🔴 **"NEVER RUN" AND "NOT OUTSTANDING" ARE DIFFERENT CLAIMS, AND THE SENTENCE USED THE FIRST
  TO EVIDENCE THE SECOND.** That no cell was run is a fact about the run caches — cheap to
  check, and it was true. That it is *not outstanding* is a **disposition**, and nobody had
  taken it. Welding a measurement to a disposition in one parenthesis launders the
  disposition, and **"not outstanding" is the tell** — it is the doc marking its own homework.
  The same shape is available to any agent closing out its own arc.
- ⚠ **AN ABSENCE OF CELLS NEEDS ITS POSITIVE CONTROL TOO.** "0 frontier cells" has the same
  shape as a probe wired to nothing, so the enumeration reports both halves — 0 frontier ids
  against mimo 5 / deepseek 2 / glm 2 over 13 transcripts. It also surfaced a cell this doc
  never listed (`smoke-cred-01`, mimo), which is why the population is the transcripts' own
  `model` field and **not** trial names.
- ⚠ **`find-session.py --arc` CANNOT SEE THIS REPO.** It resolves a doc only under
  `$DEVRC`/`$HOMELAB`/`$DATAPACKET`/`$CIVITAI`, and `$CIVITAI` is the app repo — so the arc
  walk exits **5 (named but NOT measured**, explicitly not "empty"). The chain resolves only
  with `CIVITAI=<this repo> python3 …`. The next close-check here hits the same wall.

### Added 2026-09-27 — what merging a doc can break

- 🔴 **REBASING YOURSELF PROVES NOTHING ABOUT THE OTHER OPEN PRs ON THE SAME FILE.** I rebased
  `#724`, merged it, and put `#717` and `#718` into conflict — both clean minutes earlier. **The
  check is one command BEFORE merging:** `gh pr list --state open --json number,files` filtered
  to the path you are landing. This doc already said *"two changes touching one file: TEST-MERGE
  them"*; it does not say "your own branch", and I read it as if it did.
- 🔴 **A DOC-ONLY MERGE CAN MAKE THE CANONICAL DOC WRONG ABOUT PRODUCTION.** While `#717` sat
  conflicted, `main` asserted `v0.1.1` was *"pending operator review"* and *"`v0.1.0` is live and
  broken"* — both false since `2026-09-25T23:30Z`. **Delaying a doc PR is a correctness change.**
- 🔴 **RESOLVE A DOC CONFLICT ON MEANING, NOT ON SIDES** — here `--ours` wholesale would have
  resurrected a retracted falsehood. Verify with a per-claim grep of BOTH sides; absent conflict
  markers prove only that git is satisfied. **Never rebase a branch another worktree holds** —
  detached-worktree recipe in the `cli` store scope (`cairn recall --ref dogfood`).
- ⚠ **The close-check's own coverage is a FLOOR, not an enumeration:** the arc walk reported **4
  sessions** and its own gaps — **3 of 11** commits on this doc carry no session id, and the
  opencode corpus was not searched. (`--arc` cannot resolve a doc in this repo at all; that is a
  devrc tooling gap, recorded in the `devrc` store scope, not here.)

### Added 2026-09-27 (resume) — durable facts RELOCATED here out of `State now`

🔴 **These were living under a REPLACE heading, where every future update deletes them.** The write
gate flagged 18 such lines; these are the ones worth keeping, moved to an APPEND section so the next
`State now` rewrite cannot eat them. **Nothing here is new this session** — it is 2026-09-27
(morning) evidence, re-homed.

- **The six PRs of 2026-09-27, verified on `origin/main` BY CONTENT** (a squash merge never makes
  the branch an ancestor, so ancestry is never the check): `#724` `7c57d3a` frontier drop · `#726`
  `e9c9d78` schema re-vendor · `#717` `721edae` v0.1.1-live · `#728` `8c4c631` oracle
  verdict-forgery · `#718` `3c450ea` consent control + archive eviction · `#729` `9e9142e` ship
  verdict-forgery. Ranks closed: 13, 15, 16, 17, 18, 19.
- **`v0.1.2` is live** — reviewed `2026-09-27T03:13:52.743Z`, deployed `03:15:10.039Z`, answering a
  second real-user report (*"Posting failed. Please try again."*). ⚠ That report is
  operator-relayed; what is MEASURED is the version, the dates and the controls.
- **Both defects confirmed in the SERVED bytes** (`/assets/index-CcmcNwgc.js`, HTTP 200): Post was
  enabled for a workflow that produced NO images — `watch()` resolves on any of
  `succeeded|failed|canceled|expired` and only submit-time status was checked — now gated on
  `status==="succeeded"&&((Q=Bt.imageUrls)==null?void 0:Q.length)>0`; and every host refusal was
  flattened to one string — now all five closed-set codes plus
  `typeof(R?.code)=="string"&&th(R.code)?Dt(PS[R.code]||R.message||…)`.
  ⚠ **The minifier DOWNLEVELS optional chaining**, so grepping the source spelling
  `imageUrls?.length>0` returns **0** on the bundle and is NOT evidence of absence — it cost a
  near-miss report. Pair with: `grep -c` on a minified bundle counts LINES, and a minified bundle
  is one line.
- **The bundle md5s, which are the only attributable controls:**
  `dogfood-ab-imgposter-fixed` = `v0.1.2` = **`c603fa854eba4d5206b73e04c14a1f3c`**, equal to live;
  the pre-fix image `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix` =
  **`b8a1936e5939e39aa3e26f6e9e777113`**. Without that image `v0.1.2` has no control at all. ⚠ Any
  doc or probe reading the container as "the v0.1.1 fix" is one version behind.
- 🔴 **`v0.1.2` IS AN ERROR-PATH CHANGE AND THE ORACLE CANNOT GRADE ANY OF IT.** `"Please try
  again."` sites go 3 → 6; `InlineTransport` rejects every request, so the error branch is the only
  one any arm reaches and a fix making failures legible is invisible to `RENDER=`. **Auto-resume
  stays unverifiable by construction** (needs `TOKEN_REFRESH`, undeliverable by
  `InlineTransport.onMessage`) — a human clicking Generate is the only check. ⚠ `posts:write:self`
  4-vs-1 separates `v0.1.1`+`v0.1.2` from `v0.1.0` and says **nothing** about `v0.1.2`.
  **This is precisely what rank 22 exists to fix.**
- **The two forgery fixes are ONE RULE, not two.** `#728`: an app being graded could write its own
  grade (manifest values printed with a raw `%s`; both consumers take the FIRST `KEY=` match —
  measured `RENDER=yes` on an app that built nothing). `#729` found the same hole in
  `ship.verdict.sh` and **extracted the rule to `scripts/dogfood/_esc.sh`, sourced by both
  graders**, rather than copying it. 🔴 **MEASURED REASSURANCE: 0 of 8 recorded fixtures was
  forgeable**, so no recorded verdict in this arc is void.
- ⚠ **`#718` WAS MERGED WITH ROUND 7 OF ITS AUDIT LADDER NOT RUN** — accepted unaudited by a dated
  operator decision, 2026-09-27. No round ever returned clean: round 6 found the forgeable
  `outputDir`, `#728` fixed it upstream, and a fix RESETS the gate. **Do not read "merged" as
  "audited".** The residual is a subtly wrong fixture or an evicted block whose meaning changed in
  transit — the two things CI cannot see. Round 7 is a post-merge option, not a gate.
- ⚠ **`#718`'s DISCARDED retraction is still reachable** in that PR's own commits and in
  `refs/remotes/pr/718`. It concluded the frozen condition was *unmet and still outstanding*, which
  the 2026-09-26 operator drop supersedes. **Do not re-import it, and do not read its survival
  there as a second open question.**

### Added 2026-09-27 (resume) — a stale local CLI, a reconciliation that was not one, and my own recommendation refuted

- 🔴 **I NEARLY REPORTED DOC-ROT THAT WAS MY OWN STALE INSTALL. `civitai --help` IS NOT `main`.**
  Reading `civitai app listing set-icon --help` showed *"See 'Listing media requirements' in the
  README"* — a section deliberately relocated to the published docs
  (`claudedocs/decisions/25-listing-media-bounds.md`), with guards asserting it is gone. That reads
  as live rot in a shipped help string. It is not: `main` already says *"Platform bounds:
  https://developer.civitai.com/apps/guide/store-listing"*. My installed CLI was **0.1.105** against
  npm's **0.1.109** and `git describe origin/main` = `v0.1.109-25-gb408912`. **This is the MIRROR of
  the arc's own "a merged fix is not a shipped fix": a locally-installed CLI is not `main` either,
  and a trial installs npm `latest`, so NEITHER is what a cell runs — three artefacts, not two.**
  Check `civitai --version` against `npm view @civitai/cli version` before quoting any help text.
- 🔴 **A "DOES NOT RECONCILE" NOTE KEPT AN OPEN QUESTION ALIVE FOR A MISSING COLUMN.** This doc
  recorded that `civitai buzz`'s per-pool output does not reconcile with the single total older
  entries carry. It reconciles exactly — the earlier read omitted the **Yellow** pool
  (1,305,224 + 919,796 + 1,848,987 = 4,074,007, and the CLI prints that Total). **Before recording
  two numbers as irreconcilable, check you read all the ROWS** — a missing column looks identical to
  a discrepancy, and the note outlived the mistake by days.
- 🔴 **THE OPTION I RECOMMENDED TO THE OPERATOR WAS REFUTED BY A MEASUREMENT I COULD HAVE TAKEN
  FIRST.** I offered "bake ImageMagick into the trial image" as *(Recommended)* for rank 14's media
  problem. Reading the platform's actual bounds afterwards showed generation's own `--aspect-ratio`
  buckets land inside both bands (`1:1`→1.0 for the icon, `16:9`→1.78 for the cover), so no resize
  step exists to tool for. **A recommendation attached to an operator question is a claim; take the
  cheap measurement BEFORE putting a recommendation in front of someone**, or the question is
  shaped by the least-measured option.
- 🔴 **A GUARD'S OWN COMMENT SAYING "NO CI JOB RUNS THIS" NEEDED CHECKING, AND THE ANSWER CHANGED
  THE TASK.** I briefed rank 20 on the strength of `TestBlockDocsLinksResolve`'s comment. The
  comment is TRUE — but the repo also has a fully-wired sibling, `readme-links.yml`, running the
  README link test weekly with a positive control (grep the log for the test's own
  `--- (PASS|SKIP|FAIL):` line), `-count=1` (Go's test cache tracks env vars but **not** what a
  remote host answered, so a cached run replays a PASS over a link that has since died), four
  distinct states, issue filing, and a `drill` input that appends a known-404 URL to rehearse the
  failing path. Its header states the principle: *"a guard nobody runs reads as coverage while
  providing none — the worst of the three states, because it stops the next person looking."*
  **Widening an unwired corpus reproduces exactly that, so the deliverable is a corpus something
  actually RUNS** — amended mid-flight. **Look for the sibling that already works before designing
  the mechanism.**
- ⚠ **`developer.civitai.com` answers a real 404 for an unknown path** (verified: a nonsense path
  → 404, `apps` → 301, `apps/guide/responsive` → 200), which is what makes a liveness probe against
  this host meaningful at all. An SPA answering 200 for everything would make the whole guard
  vacuous — that is why the negative control exists, and it passed.
- ⚠ **Three agents on one repo is fine; three agents on one FILE is not.** 21 and 22 both touch
  `scripts/dogfood/oracle.sh`, so each was told the other's region by name and to rebase before
  reporting. **A clean git merge is not a clean merge** — read the merged result of `oracle.sh`
  rather than trusting the absence of conflict markers, and re-run the merged-tree suite once the
  base moves.

## How to verify

Every check reports a CONTROL beside its result; a bare count or zero is not a measurement.

**The three in-flight PRs — find them before anything else** (expect three rows once opened; an
empty result means they are still being written, NOT that the work was not dispatched):
```bash
gh pr list --repo civitai/cli --state open --json number,headRefName,title \
  --jq '.[] | select(.headRefName|test("dogfood-path-reads|dogfood-post-arm|docs-link-corpus")) | "\(.number) \(.headRefName)"'
```

**Rank 20's defect and its fix** (expect `404` then `200`; the second is the control that the host
answers honestly rather than serving an SPA 200 for everything):
```bash
curl -s -o /dev/null -w 'broken  %{http_code}\n' https://developer.civitai.com/apps/responsive
curl -s -o /dev/null -w 'correct %{http_code}\n' https://developer.civitai.com/apps/guide/responsive
```

**Rank 21's defect, reproducible with no repo at all** (expect two lines, then one):
```bash
printf 'DEFECT:\n';  printf '%s\n' '/work/my app/block.manifest.json'  | head -1 | xargs dirname
printf 'CONTROL:\n'; printf '%s\n' '/work/myapp/block.manifest.json'   | head -1 | xargs dirname
```

**Rank 22's premise — that the post path routes through the shimmed method** (expect a
`sendTypedRequest` hit for `CREATE_POST_FROM_APP`, and the mock host's non-empty-`sources` gate):
```bash
docker exec dogfood-ab-imgposter-fixed bash -lc \
  'D=/work/ab-img-poster/node_modules/@civitai/blocks-react/dist;
   grep -n "sendTypedRequest" $D/hooks/useCreatePostFromApp.js | head -2;
   grep -n "no images to post" $D/internal/mockHost.js | head -2'
```

**Rank 12's premise** (expect ZERO workflow hits, and a non-zero control proving the grep works):
```bash
git -C /home/zach/workspace/civit/cli grep -c CIVITAI_SCAFFOLD_TYPECHECK origin/main -- .github/  # expect no output
git -C /home/zach/workspace/civit/cli grep -c CIVITAI_CHECK_SCAFFOLD_RUNTIME origin/main -- .github/  # CONTROL: non-zero
```

**The fixtures are intact** (expect `11`, and the two images):
```bash
docker ps -a --format '{{.Names}}' | grep -c '^dogfood-'
docker images --format '{{.Repository}}:{{.Tag}}' | grep dogfood-fixture
```

**The account** — re-read immediately before any spend, never when planning:
```bash
civitai app status --json   # ab-img-poster v0.1.2 approved/live; 100 rows AT the cap; pending 0
civitai buzz               # Blue+Green+Yellow SUM to Total; 4,074,007 on 2026-09-27 — it drifts
```
⚠ **The submissions listing is AT its 100-row cap with no cursor**, so a blockId absent from
`app status --json` is not evidence it has no submission — look it up with
`civitai app status <blockId>`.

## Defects (batched)

- ✅ **RESOLVED 2026-09-25, AND SUPERSEDED 2026-09-27** — this entry read *"`ab-img-poster
  v0.1.0` is live and broken for first-time viewers until `v0.1.1` is approved"*. `v0.1.1` was
  approved `23:30:15Z`; **`v0.1.2` has been the serving revision since `2026-09-27T03:15:10Z`**,
  fixing a second user-reported path (*"Posting failed. Please try again."*). ⚠ Kept rather than
  deleted because rank 13's closure and this line were written in different sessions and the
  line was missed — the tell is a *Defects* entry whose condition a *Next steps* rank already
  reports closed. **When you close a rank, grep this section for the same claim.** 🔴 **This
  entry has now gone stale TWICE the same way — a version number written here is a claim about
  the day it was written. Name the date beside it or read the API.**
- `pickerBlind` and `consentBlind` have never been tested together on one doubly-blind bundle.
- `CONSENT_SETTLE_MS = 1200` is a judgement; every observed ask was synchronous, but there is
  no measured bound for an app that asks after an `await`.
- Nothing is known about the **iframe** transport; all of this is the inline path.
- `c=civitai; $c app submit` is not refused, at base or HEAD (pre-existing; the false comment
  was corrected in `#702`).
- `civitai app listing status` opens a shadow revision on a LIVE listing and there is no
  `discard-revision`. One is **still open on `panorama-360`** — operator-only to clear.
- The submissions listing caps at 100 rows with no cursor, and the account is AT the cap.
- `civitai app submit` **exits 0 when it did not submit** (no token) — it warns loudly
  (*"⚠ NOT SUBMITTED"*), but an exit-code-only reader would score it as success.
