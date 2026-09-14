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

- **`civitai/cli` `main` @ `426288f`** (was `7467c62`; `#601`, `#596`, `#603` landed since), clean,
  `make ci` rc=0 / 21 ok / 0 FAIL / golangci-lint 0 issues — measured on `main` itself.
  🔴 **This repo has a SECOND live handoff doc**, `claudedocs/handoff-agent-setup-onboarding.md`,
  maintained by a concurrent session (`#603`). It has its OWN ranked list, so **"rank 23 closes"
  in a commit subject may not mean this doc's rank 23** — #603's meant `cli#596`, this doc's is
  `cli#593`. The `claim-work` slug is derived from the DOC path, so the locks do not collide; only
  human readers do.
- **`civitai-developer-docs` `main` @ `cced3e2`**, drift sweep green.
- **No `clawgate-task:` field.** `resolve` exited **5** — 0 tasks for this session, with its
  positive control answering 2 links for a DIFFERENT session, so the board was reached and the
  token accepted. A wrong id also answers 200 with an empty array, so that zero is narrower than
  a clean bill of health. No field written.

### Merged this session — all verified by CONTENT, not ancestry

| what | sha |
|---|---|
| `cli#590` — download line-forgery (3 audit rounds) | `3b222c6` |
| `cli#591` — the 429 exit-code contract (4 rounds; merged by a CONCURRENT session at 02:51) | `2c6fc4a` |
| `cli#592` — handoff + 15 closed blocks demoted to `claudedocs/refs/` | `e39250b` |
| `cli#595` — the dispatched-vs-scheduled correction | `e9c51b1` |
| `cli#585` — submit-body ceiling + `--allow-oversize` (round 0 + round 1) | `61be65e` |
| `cli#598` — **restores a regression I shipped in `61be65e`** | `7467c62` |
| `devdocs#80` — the last two drifts (7 rounds) | `cced3e2` |

🔴 **`cli#591` was merged by another session while I was mid-work on it.** Its PR head froze at
`c355274`, so a merge-with-main I pushed afterwards (`02b0f50`) could never land, and CI reported
**0 checks** on it — which reads as "pending", not as "the PR is closed". `gh pr close` is what
finally said so. **Treat a PR object's head as a claim; the remote branch tip and `mergedAt` are
the facts.**

## Open investigations — live diagnosis state

### 🔴 RETRACTED: "#513's server-side coercion is NOT reproducible" — it IS, and it is root-caused

🔴 **This block RETRACTS the earlier block titled "The server-side coercion is NOT reproducible
on any reachable surface" AND the block above it that inverted the probe strategy. Both are
superseded. Do not act on either — in particular, the instruction to go find a test subject is
DISCHARGED; a test subject was found, 55 of them.** The earlier measurements were not fabricated;
they were real readings of the wrong thing, and the mechanism that made them wrong is the whole
lesson.

- **Symptom + exact repro (verified first-hand, not only by the agent):**
  ```bash
  curl -s -A '<a real browser UA>' \
    'https://civitai.com/api/v1/images?username=2428023993&limit=3' \
    | grep -oE '"username":[^,}]{0,25}'      # -> "username":2428023993   UNQUOTED
  ```
- **Observed (with values):**
  - `cf-cache-status: MISS` on that request — a genuine origin response, 3/3 items unquoted.
  - **The same user via the other code path is QUOTED:**
    `?imageId=1446527` → `"username":"2428023993"`.
  - **It is a numeric COERCION, not merely missing quotes.** `?imageId=622901` →
    `"username":"0222"`; `?username=0222&limit=4` → `"username":222`. Leading zeros are
    DESTROYED. And `?username=222&limit=4` returns **0 items** — the printed name round-trips
    to nothing.
  - Agent sweep: 98 all-digit usernames harvested, 55 with public images. On a cache-busted
    re-sweep, **51/51 UNQUOTED, 0 quoted**; 38 of 38 readings that had looked "quoted" flipped
    once the CDN cache key changed. The origin coerces **100%** of the time.
  - Prevalence in unfiltered listings is low — 797 `username` keys across 8 listing pages, 0
    unquoted — but that measures **rarity of all-digit accounts** (0/399 in the subset), not
    absence of the bug. It bites when a caller filters by such a user, which is exactly what
    Rochet2 did.
- 🔴 **WHY THE EARLIER PROBE READ EMPTY — two independent causes, and the second is the
  transferable one:** (1) it never reached an all-digit account that had public images; (2) the
  quoted readings it *did* get were **Cloudflare cache HITs**, not origin responses. A CDN with
  `s-maxage=300` served stale correct-looking copies. **A probe that does not report
  `cf-cache-status` is not measuring the origin**, and "I read the raw bytes" does not save you
  — the bytes were real, they were just the wrong server's.
- 🔴 **ROOT CAUSE — RETRACTED AND REPLACED 2026-09-14. IT IS NOT MEILISEARCH.** Every
  MEASUREMENT above holds; the ATTRIBUTION did not survive contact with the code. The index
  document's `user.username` **never reaches the response**: `getImagesFromFeedSearch` →
  `ImagesFeed.populatedQuery` builds `user` from the Postgres-backed `userData` **Redis cache** and
  spreads it AFTER the doc, overriding it. The coercion is in
  `event-engine-common/caches/base.ts` — a Redis hash stores only strings, so `createCache`
  serialised each field on write and **guessed the type back** on read:
  `item[key] = isNaN(Number(value)) ? value : Number(value)`. `Number('0222')` is `222`. Same
  observable, different mechanism. (It also read a stored `false` back as the truthy string
  `'false'`, `null` as `'null'`, and `''` as `0`.)
- 🔴 **THE OBSERVATION THAT SEPARATES THE TWO — and why the old attribution was UNFALSIFIED, not
  confirmed:** cache-busted, `cf-cache-status: MISS` on every row,
  `?username=0222&limit=11` → `"username":"0222"` (a Redis MISS returning the raw Postgres row),
  then `limit=12,13,…,16` → `"username":222` (Redis HIT, decoded). **A Meilisearch document cannot
  change between two requests seconds apart.** The original probe never varied anything that would
  make the two mechanisms disagree, so it read the symptom and attached the first plausible cause —
  the `via: code` tag on that bullet made a reading of ONE file look like a derivation. **`via:
  code` means "I read code", not "I read the code that runs."**
- **Ruled out:** that it was fixed server-side since 2026-08-30 — 51/51 fresh origin responses
  coerce, measured 2026-09-11. `via: measurement` · That other surfaces are affected —
  `/api/v1/models` (`creator.username`), `/api/v1/models/{id}`, `/api/v1/users?query=` and
  `/api/v1/creators` are all Prisma-backed and all returned QUOTED, cache-busted.
  `via: measurement` · That the CDN itself was coercing — the legacy path is quoted through the
  same CDN. `via: measurement`
- 🔴 **The merged `FlexString` fix (#532) does NOT repair this, and the distinction matters.**
  It makes the CLI decode a number cleanly, which is correct and still wanted. But `"0222"`
  arrives as `222`, so the CLI now prints a **wrong username that looks right**. A decode fix
  cannot recover information the wire already lost.
- **Next probe:** none for diagnosis. Fix is open as **`civitai/civitai#4839`** (pins the
  submodule) + **`civitai/event-engine-common#13`** (the real fix). 🔴 **Merge `#13` FIRST, then
  re-pin `#4839`** — it currently points at #13's branch commit. `/api/v1/blocks/images` is still
  **suspected, not measured**: it returns `401 Block token required`, and calls the same
  `runImageSearch` (`blocks/images.ts:198`), so it is covered by code identity only.

### 🔴 A TAB is a worse forgery vector than a newline, because tabwriter's delimiter IS the tab

- **Symptom + exact repro:** at #554 HEAD, `images search` (no `--meta`) with
  `username = "alice\tSDXL\t9x9\tNone\t0\t0\thttps://evil.example/steal"` emits a **fully
  aligned**, entirely attacker-controlled row, pushing the real values past column 80.
- **Observed (with values), verified first-hand not just by the auditor:**
  `safeTermSingle` (`safeterm.go:59-65`) replaces **only `"\n"`**; `saferune.go:219` reads
  `if r == '\n' || r == '\t'` — tab is **deliberately kept**. Survival through
  `safeTermSingle`: `\n`→space; `\r`, `\v`, `\f`, U+0085 **stripped**; **`\t`, U+2028,
  U+2029 SURVIVE**.
- 🔴 **The generalisable part: "reach column zero" was never the whole hazard.** In a
  `tabwriter`, a tab lets the attacker **inject a column**, so alignment — the thing that
  makes output look trustworthy — becomes the attack surface. Any replacement predicate must
  cover both.
- **Ruled out:** that this is new in #554 — the tab behaviour predates it. What is new is the
  CLAIM: `safeterm.go:49-55` justifies the helper by naming column-alignment and tabwriter
  row-forging, while guarding only `\n`. `via: code`

### #513's server half — the fix is OPEN, and the filed root cause was WRONG

- **State:** `civitai/civitai#4768` still carries the SUPERSEDED Meilisearch attribution in its
  body; the correction is a COMMENT on it (an issue body is what the next reader acts on, so
  this is the weaker placement — see the Gotcha on that). Fix PRs: `civitai#4839` +
  `event-engine-common#13`. `civitai/cli#513` deliberately stays OPEN; Rochet2 was told
  directly that `FlexString` does not make the value correct.

### `developer-docs#61` does NOT reproduce — but at the WRONG end of the reporter's own dimension

- **Symptom + exact repro:** reporter (giannamikaelova, 2026-08-24) got HTTP 500 from
  `POST https://orchestration.civitai.com/v2/consumer/workflows?whatif=true&allowMatureContent=false`
  with the official `klein4bBody`.
- **Observed (with values), 2026-09-11:** the byte-identical official payload returns
  **HTTP 200** with a valid quote — `{"transactions":{"list":[{"type":"debit","amount":500,
  "accountType":"blue"}],"insufficientBuzz":false},"status":"unassigned"}`. **Nothing was
  created or charged:** the returned id is absent from `civitai workflows list`, whose newest
  entry is 2026-09-10.
- 🔴 **This is NOT evidence the bug is fixed, and must not be recorded as such.** The reporter
  hypothesised the 500 might be the *insufficient-balance* path. The account used holds far
  more than the 500-Buzz quote and returned `insufficientBuzz: false` — the GOOD end of
  exactly that dimension. One reading there says nothing about the under-funded end.
- **Next probe:** a `whatif` for this payload from an account whose blue balance is **below
  500**. Asked the reporter for their balance at the time; that one detail likely settles it.
- **Method note:** the request URL was hard-guarded on `whatif=true` before sending —
  omitting it would SUBMIT a paid training job. Reuse that guard.


### Closed blocks live in `claudedocs/refs/external-issue-513-numeric-username.md`

15 investigation blocks were demoted there on 2026-09-13 when this doc hit its 65,536 B
ceiling: the `#513` supersession chain, the `#544`/`#545` review-latency chain, `#554`/`#564`'s
collision, `#552`'s closing condition, `#575` R1, and `devdocs#76`/`#77`'s reporting half. All are
CLOSED — each was either superseded by a block still below, or resolved by a merge in this doc's
SHA ledger. **Read them before re-deriving anything about those threads**, and treat every value
there as recall rather than live state.

### `developer-docs#76` — CLOSED, all seven drifts cleared. Block demoted 2026-09-14

Moved VERBATIM to `claudedocs/refs/external-issue-513-numeric-username.md` when this doc hit
its 65,536 B ceiling. Headline: sweep run `34765344965`, `conclusion=success`, **0 failing
steps / 15 executed**; the last two drifts were `check:pins` and `check:ds-pins`. Rank 9's
scheduled-run confirmation is the separate RESOLVED block below.

### 🔴 `cli-snapshot-refresh`'s GREEN means "skipped", not "unblocked" — and it WILL go red again
- as-of: 2026-09-13

- **Symptom + exact repro:** the workflow went green at run `34757726339` (2026-09-13 12:40) after
  two red runs. `gh api repos/.../actions/runs/<id>/jobs`.
- **Observed (with values):** `job decide: success`, **`job build-cli: skipped`, `job refresh:
  skipped`**. The `decide` log reads `committed snapshot: civitai v0.1.104 (116 ===CMD blocks)` and
  `✓ snapshot tag v0.1.104 matches the latest civitai/cli release v0.1.104`, plus a degraded
  `⊘ commits-behind unavailable (compare API unreachable) — tag verdict stands`. So the run was
  green **because there was nothing to re-capture**.
- **Ruled out:** *"rank 14 is fixed"* — **via: measurement**. The refresh path never executed. The
  org policy (*"GitHub Actions is not permitted to create or approve pull requests"*) is untouched
  and no `permissions:` block can grant it.
- **Leading hypothesis:** the next `civitai/cli` release (`v0.1.105`) makes the snapshot stale,
  `decide` stops skipping, and `refresh` hits the policy again.
- **Next probe:** after the next cli release, check whether `build-cli`/`refresh` are non-skipped.
  Filed as `devdocs#82` with exactly that closing condition — **a green run with `refresh: skipped`
  does not close it.**

### The `devdocs#80` audit ladder — CLOSED. Block demoted 2026-09-14

Moved VERBATIM to `claudedocs/refs/external-issue-513-numeric-username.md`. Headline: the page
was correct from round 2; rounds 3–7 found only scaffolding defects, and round 4's justified the
ladder — with the flag misspelled generator-side the page built with **0 `<pre>` elements**
while two checks returned rc=0. Durable lesson: **a guard's DESCRIPTION claiming more than its
implementation** was four of seven rounds' findings, and the fix shape that ended it was to stop
REPLACING assertions and start ACCUMULATING them. Residual: `devdocs#81`.

### 🔴 RESOLVED: the drift streak's end is now OBSERVED in a scheduled run, not inferred
- as-of: 2026-09-14

- **Symptom + exact repro:** rank 9's forcing function was stated in SCHEDULED runs ("red for 34
  consecutive scheduled runs, last green 2026-08-11"), and the green run that motivated closing it
  was a `workflow_dispatch` — the same workflow and job against the same tree, but not the
  observation the claim was made in terms of.
- **Observed (with values):** run **`34848144324`**, `event=schedule`, **2026-09-14T13:15 UTC**:
  `drift=success`, **0 failing steps and 15 steps EXECUTED**, `notify=success`. The step count is
  the load-bearing half — a run that SKIPPED its steps reports `success` too, which is exactly how
  `cli-snapshot-refresh` (rank 14) reads green while doing nothing. The prior scheduled run,
  `09-13T12:14`, was the last `failure`.
- **Ruled out:** *"a dispatched run closes a claim stated in scheduled runs"* — **via: measurement**.
  It did not; this run does. The gap was ~22 h and cost nothing but patience.
- **Next probe:** none. Closing condition met, in the terms it was written in.

### 🔴 RESOLVED: `cli#591` round-4 F2 — the ordering is pinned at BOTH the sentinel and the code
- as-of: 2026-09-14 · shipped in **`cli#601`** (`b727a83`)

- **Symptom + exact repro:** `exitcodes_doc.go`'s code-6 bullet publishes 🔴 *"THE HEADER IS
  CONSULTED BEFORE THE MESSAGE, so a cap-worded 429 that carries `Retry-After` exits `5`, NOT
  `2`."* Consult the message first in `pkg/civitai/retry.go`'s 429 branch (return terminal on cap
  wording, whatever the header) and run `go test ./...`.
- **Observed (with values):** round 4's *21 ok / 0 FAIL* under that mutant was **independently
  reproduced** — mutant applied with both of #601's files reverted to `7467c62` → 21 ok / 0 FAIL.
  The gap was real. **Why nothing could see it:** `retry_test.go`'s two 429 neighbours each hold
  one half of the input fixed — the exhaustion case sends an EMPTY body, the terminal case sends
  no header. It takes both at once, which nothing sent. After #601: under the same mutant, **19 ok
  packages and exactly TWO `--- FAIL`s**, both new guards.
- 🔴 **Round 0 of the audit found the delivered guard was ONE HOP SHORT, and that is the durable
  part.** It pinned the SENTINEL (`errors.Is` ErrNetwork, not ErrBadRequest). The requirement of
  record and the published bullet are about an **EXIT CODE**, and nothing anywhere composed
  cap-body + `Retry-After` → `exitCode()`. The repo already owned the idiom one directory over:
  `cmd/civitai/read_error_stderr_test.go` says it in words for the adjacent 429 row — *"pkg/civitai
  owns the sentinel; this owns the number a script reads from `$?`."* **A classification test and
  an exit-code test are two claims; a published `rc` needs the second.**
- **Ruled out:** *"nothing local can pin this"* — **via: code**. The old `pinnedBy` read
  `"nothing local — the assumption is about the server"`, conflating the **ORDERING** (local,
  trivially observable, now pinned twice) with the **REACHABILITY** (whether the server ever
  attaches `Retry-After` to a cap 429 — genuinely unguardable, and the reason the case is
  published). Telling the next maintainer no guard is possible is the description-reads-as-coverage
  failure.
- **Also fixed, and the cheaper lesson:** the precedence was **described in three surfaces and
  argued in none**, so a maintainer reading the bullet call it *"the exact hazard the 2
  reclassification exists to prevent"* would reasonably invert it and hit a test that asserts a
  contract without saying why. `retry.go` now argues it — a `Retry-After` header is a STRUCTURED
  signal the server sent deliberately; `isDeepPagingCap` is a three-phrase substring match over
  prose its own doc comment calls *"deliberately narrow"*, which any proxy or copy-edit can produce
  or destroy — and names the four surfaces that move if the precedence ever changes.
- **Next probe:** none. Round 1 (the nine correctness axes) was **NOT** run on #601; it merged on
  round 0 plus my own verification. Test-and-comment-only and green on `main`, but that is a
  judgement, not an audit.

## Next steps (ranked)

🔴 Numbering stable — rank is half a `claim-work` slug's identity. 1–21 keep their meaning.

1. **DONE — `civitai/cli#526`.** forcing: none
2. **`developer-docs#61`** — awaiting the reporter's blue balance at the time of their 500. Still
   OPEN, 1 comment, untouched since 2026-09-12. Genuinely blocked on a third party.
   forcing: user — external reporter, waiting since 2026-09-11.
3. **`civitai/civitai#4768` — PICKED UP 2026-09-14, agent dispatched to fix + PR.** Still 0
   comments / untouched since 09-11 at dispatch time; the operator chose "dispatch to pick it up
   and PR" over merely routing it. Working in `/home/zach/workspace/civit/civitai-4768-username`
   off `civitai/civitai`, claim slug `civitai-4768-numeric-username`. 🔴 **The brief's load-bearing
   constraint: a serialization-boundary `String(...)` cast is NOT the fix** — `"0222"` is already
   `222` by the time the read path sees it, so a cast repeats #532's mistake on the server side.
   The PR must state which half it ships (stored representation + reindex/backfill vs defensive
   read) and use a BARE reference if partial. forcing: none — but see rank 25 for the outcome.
4. **DONE — stale branch deleted** (`12818a3`). forcing: none
5. **DONE — `#545` merged.** forcing: none
6. **DONE — `#554` closed by @xsvm.** forcing: none
7. **DONE — AGENTS.md item 37 corrected** via `#556`. forcing: none
8. **DONE — `#552` CLOSED.** forcing: none
9. **DONE — the drift streak's end is OBSERVED.** Scheduled run `34848144324` (2026-09-14T13:15
   UTC): `drift=success`, **0 failing steps / 15 executed**, `notify=success`. See the resolved
   investigation block. forcing: none
10. **DONE — vendored mirrors measured CLEAN.** forcing: none
11. **`home-manager switch`** so `devrc#1498`'s lesson is live. Operator's call — restarts
    collector/keylog/i3 on both hosts. forcing: none
12. **`civitai/cli#575` — R2, R3, R4, R6 remain, and NONE were touched this session.** The issue
    body carries a status table: R1 ✅ `#578`, R5 ✅ `#582`, R2/R3/R4 🟡 *"a trade-off that may be
    accepted in writing"*, R6 🟡 has engineering. **R2–R4 need a WRITTEN decision from a named
    reader, not code.** forcing: none
13. **DONE — `#77` verified live**, in both directions: the notify job also CLOSED `#76` when the
    sweep passed. forcing: none
14. **`cli-snapshot-refresh` is LATENT, not fixed** — filed as `developer-docs#82`. Its green runs
    mean `build-cli`/`refresh` SKIPPED; the bot-PR org policy is untouched and the next
    `civitai/cli` release re-triggers it. forcing: gate — a gate whose green means "did nothing".
15. **The `civitai-developer-docs` cairn scope is unreachable.** `cairn create` refuses
    `[not-found]`; the drafted `drift-sweep` entry is preserved in this doc's Gotchas.
    forcing: none
16. **DONE — `cli#591` merged** (`2c6fc4a`), by a concurrent session. 4 audit rounds.
    forcing: none
17. **DONE — `cli#590` merged** (`3b222c6`). 3 audit rounds. forcing: none
18. **DONE — `cli#585` merged** (`61be65e`). Round 0 questioned the requirement; round 1 found a
    🔴 behaviour bug. forcing: none
19. **`developer-docs#5`** — open since **2026-07-13**, MERGEABLE/CLEAN, 12 checks. Two months
    stale; needs a yes/no rather than work. forcing: none
20. **`developer-docs#81`** — a transitive `ERR_MODULE_NOT_FOUND` misrouted to the retirement
    message. Filed rather than fixed so round 7 stayed the ladder's last. forcing: none
21. **The `developer-docs#76` residuals that are NOT drift-checked, none started.**
    (a) `WorkflowStep.jobs` removed from the refreshed OpenAPI spec while
    `orchestration/guide/workflows.md:75,82-111` still documents it as a field AND a whole
    `## Jobs` section — **needs an authenticated live call** to tell "the spec stopped declaring
    it" from "the API stopped returning it"; do not delete a documented section on a spec diff
    alone. (b) the `/apps/installed` → `/apps/activity` prose rename. (c) `apps/guide/concepts.md`'s
    NSFW gating advice (`isSfwCeiling(maxBrowsingLevel)`) is now the LESS safe option.
    forcing: none
22. **DONE — `cli#601` merged** (`b727a83`). The ordering is pinned twice: the SENTINEL in
    `pkg/civitai/retry_test.go` and the EXIT CODE in `cmd/civitai/read_error_stderr_test.go`.
    Round 0 ran BEFORE the merge decision and changed what shipped (2 payload findings);
    round 1 did not run. forcing: none
23. **`cli#593`** — `closedLoopbackAddr` binds an ephemeral port, closes it, and assumes nothing
    rebinds it. Flaked once on `#590`; re-run passed. It guards two POSITIVE CONTROLS, and a
    control that flakes trains the reflex of dismissing the one assertion that proves the harness
    works. forcing: none
25. 🔴 **`civitai/civitai#4839` + `civitai/event-engine-common#13` are OPEN and MERGE ORDER
    MATTERS.** #4839 pins the submodule to #13's BRANCH commit: **merge #13 first, then re-pin
    #4839**. Neither has been audited — `/audit-pr` round 0 was offered and not run. Claim
    `civitai-4768-numeric-username` is RELEASED. **Not verified by anyone: nothing was exercised
    against a real Redis or a deploy** — the live probes measured the BUG, not the fix — and
    `pnpm typecheck` does not cover `apps/event-engine` (that tree has its own command).
    forcing: user — two open PRs on the main platform repo awaiting a merge decision.
26. **`cli#602` merged-tree re-run — DELIBERATELY SKIPPED, operator's call 2026-09-14.** It shares
    `internal/cmd/exitcodes_claims_test.go` with the merged `#601` and was 1 commit behind at the
    time. `gh pr view` said `MERGEABLE`/`CLEAN`, which is the TEXTUAL claim only. The mechanical
    check is posted as a comment on #602. Recorded as skipped, not absent — `#585` dropped
    `#591`'s rows on a resolution that conflicted nowhere. forcing: none
24. 🔴 **`claudedocs/handoff-index-store-claims-accuracy.md` is 7,091 B OVER its ceiling and is
    reddening a SHARED gate** — `test_no_handoff_doc_exceeds_its_budget` fails for everyone, and
    the next unrelated PR inherits it. It is in a `devrc-*` worktree, not this repo, and it GREW
    29 B between two readings minutes apart, so another session is actively editing it.
    **Not mine to edit; named because it is red on a gate this repo shares.**
    forcing: gate — a permanently-red gate trains everyone to click through.

## Gotchas / decisions / dead-ends

- 🔴 **`via: code` MEANS "I READ CODE", NOT "I READ THE CODE THAT RUNS" — and that tag is what let
  a wrong root cause sit in this doc for three days reading as derived.** #4768's Meilisearch
  attribution was tagged `via: code` + `via: measurement`. The measurements were all real; the
  attribution came from reading ONE file on the path and stopping at the first plausible mechanism.
  The actual coercion is two repos away, in a Redis cache decoder, and the value from the index is
  **overwritten before it is ever serialised**. **Before tagging a cause `via: code`, follow the
  value to the line that WRITES the response** — and name the rival mechanism, then find the
  request that makes the two disagree. Here that was trivial once asked: a Redis MISS and a Redis
  HIT seconds apart return different types, which no index document can do.
- 🔴 **A TIMED-OUT MUTATION BATTERY LEAVES THE MUTANT IN THE TREE, AND THE RESTORE LINE IS THE ONE
  THAT DIES.** A `make ci` + mutate + full-suite + restore chain hit the 2-minute tool timeout
  (`rc=143`, SIGTERM) after the measurement printed but before the `cp -a` restore ran. Every
  number I wanted was already on screen, so the run read as a success. **The mutant sat in
  `retry.go` until an explicit `grep` for it before committing.** Put the restore in its own call,
  or re-check for the mutant by name after ANY battery — a clean-looking transcript is not a clean
  tree, and `git status` shows the file as modified either way because the fix modified it too.
- 🔴 **A GREP FOR "THE OLD TEXT IS GONE" CANNOT SEE THE DIFFERENCE BETWEEN REMOVED AND QUOTED.**
  Verifying the `pinnedBy` correction landed, `grep -c 'nothing local — the assumption is about'`
  returned **1** on the merged `main` — and the hit was **my own retraction comment quoting the old
  text**. Read as "the fix did not land". When a fix RETRACTS a sentence by quoting it, grep for
  the live construct (`pinnedBy: "nothing local`), never the prose.
- 🔴 **`| head` ATE grep's EXIT STATUS AGAIN, IN THE SAME SESSION THAT RECORDED THE LESSON.** A
  closing-keyword audit on the merge commit printed nothing and its `|| echo "0 hits"` never fired,
  so the pipeline was silently indistinguishable from "no output because the command failed".
  Re-run as `hits=$(… | grep -c …)` **with a positive control on the same pattern** — a contrived
  `fixes #513` string returned 1, which is what makes the real 0 a reading.
- 🔴 **THE MERGE DROP-CHECK HAS A PREMISE, AND IT IS NOT ALWAYS TRUE.**
  `diff <(git show origin/main:<file>) <file>` reported **103 lines main has that we lack** on the
  handoff branch — which reads exactly like the `#585` regression. It was not: `main`'s copy was
  simply OLDER, because the three commits it had gained never touched that file, and the 103 lines
  were what this PR itself deliberately replaced. **The check only means "drop" when `main`
  actually changed that path** — establish that first (`git diff <merge-base> origin/main -- <path>`),
  or the alarm fires on every ordinary rewrite and trains you to ignore it.
- **Round-0 trial ledger, this session: `ran: 1 · changed the outcome: 1`.** It ran BEFORE the
  merge decision (the `audit-pr-nudge.py` routing working) and produced two payload findings, one
  of which — the sentinel-vs-exit-code seam — closed the requirement of record that the delivered
  guard had missed.

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

- 🔴 **`#544` and `#545` COLLIDE ACROSS TWO TRACKERS, and the older rows are the OTHER one.**
  Activity-telemetry `mention-detected` rows for `#544` and `#545` dated **2026-08-29 →
  2026-09-09** are **clawgate task ids**, not `civitai/cli` issues — the cli issues did not
  exist until 2026-09-10 16:10Z. This doc's own earlier text says "clawgate #544 (API-side
  coercion)" and means the clawgate one. A telemetry search for prior work on the cli issues
  will surface those rows and read as a confident "a previous session already handled this".
  **It did not.** Discriminate by DATE: anything before 2026-09-10 16:10Z is clawgate.
- **How the "no prior action" conclusion was actually established**, so it can be rechecked
  rather than re-derived: every `source IN ('claude','opencode','mentions','zsh')` row in
  `cwd LIKE '%civit/cli%'` since 2026-09-10 belongs to the agent-setup arc (#541, #542, #543,
  #546, #526, #530). Positive control on the same query shape: 6,464 `source='claude'` rows in
  the last 7 days, so the zero is a reading, not a broken filter.
- 🔴 **Do not use `IS NOT NULL` against a ClickHouse `JSONExtractString`** — it returns `''`
  for a missing key, never `NULL`, so the predicate selects the WHOLE TABLE. Test membership
  with `!= ''` and run the predicate-removed arm as a negative control. Hit this class while
  writing the queries above; it is documented in the `activity` skill and is easy to re-derive
  wrongly.
- **`ts` is a reserved-ish alias in these queries** — `SELECT toString(ts) ts` fails with
  `NO_COMMON_TYPE`. Alias it to something else (`when_`).
- **Round 0 of `/audit-pr` is ON TRIAL** and was run on #545 to generate trial data. Whoever
  collects its report must record the pair **`ran: R · changed the outcome: C`** on the PR —
  never `C` alone, because a bare zero cannot distinguish "it ran and was useless" (retire the
  section) from "nobody ever typed `--round 0`" (the trial never started).

- **The external-issue ratio, carried forward from the replaced status block so it is not lost:**
  measured 2026-09-11 via `gh issue list --state open --limit 100 --json number,title,author`,
  **21 open issues of which only 2 were NOT authored by ZacxDev** — #544 (`xsvm`, 2026-09-10) and
  #513 (`Rochet2`, 2026-08-30). The other 19 were self-filed. Both external ones are now
  progressed (#544 closed by #545; #513 diagnosed), so a re-measure today returns a different
  ratio — the **number is a snapshot; the shape is the durable part**: external reports are rare
  here and correspondingly easy to leave sitting, which is exactly what happened to #545 for 13h.
- 🔴 **A CDN CACHE HIT IS NOT A MEASUREMENT OF THE ORIGIN, and reading raw bytes does not save
  you.** The 2026-09-09 probe did everything the rules ask — raw bytes, no `jq`, a positive
  control — and still concluded the opposite of the truth, because Cloudflare (`s-maxage=300`)
  served correct-looking cached copies. **Log `cf-cache-status` on every request and require
  MISS before believing a negative**, or vary the cache key (here, `limit=1` → `limit=2`) and
  re-read. Generalises past HTTP: *any* cache between you and the thing under test makes a
  clean reading a reading of the cache.
- 🔴 **An "empty result" can be empty for TWO reasons at once.** Here the earlier probe had both
  a wrong subject (no all-digit account with public images) and a wrong observation surface (the
  CDN). Fixing only one would still have read empty and would have looked like confirmation.
  Name the rival mechanisms *before* concluding from an absence.
- 🔴 **A DECODE fix cannot recover information the WIRE already lost.** #532's `FlexString` is
  correct and stays, but it made the failure quieter, not absent: `"0222"` now decodes cleanly
  to `222` and prints a wrong username that looks right. When a fix is on the consumer side of a
  lossy producer, say explicitly which half it does not cover.
- **A squash merge is verified by CONTENT, never ancestry.** `#545` was confirmed by reading
  `origin/main:internal/cmd/images.go`; `git merge-base --is-ancestor` returns false after every
  squash, forever, and reads as "not merged".
- **The GitHub issue-creation hook requires a level-2 `## Closing condition` with content under
  it.** A "Proposed closing condition" heading at the wrong level is refused. The gate checks
  heading shape only — it cannot check that what you wrote is an observable end-state rather than
  a restatement of the fix.

- 🔴 **A COMPARISON AGAINST AN ABSENT OPERAND REPORTS "SAME", NOT "MISSING" — this bit me
  twice in one session and I reported the wrong thing to the operator both times.** I twice
  claimed the stale branch existed "locally AND on origin". The remote branch had been
  deleted; I was reading a stale remote-tracking ref. Worse, `git rev-list --count
  origin/main..origin/<gone>` and `git diff --stat origin/main origin/<gone>` both returned
  **EMPTY**, which reads exactly like "identical, safe to delete". **Prove the ref exists
  first** — `git show-ref --verify`, `git ls-remote --heads origin <branch>` — then compare.
  The previous handoff carried the same wrong claim, so it was wrong for two sessions.
- 🔴 **A `grep` for a phrase that spans a LINE WRAP returns a false zero.** Verifying
  `devrc#1498` landed, I grepped for `unique only within ITS OWN tracker` and got **0** on a
  file that contains it — the phrase straddles a newline. Pick a pattern that cannot wrap, and
  pair it with a positive control.
- 🔴 **Guard a money URL structurally, not by care.** Before probing #61 I gated the request
  on `case "$URL" in *"whatif=true"*)`. Without that flag the same POST SUBMITS a paid
  training job. Make the dangerous case unreachable rather than trusting yourself to type it.
- **An auto-generated issue's BODY is overwritten by the next run; COMMENTS persist.** #76
  says so explicitly. Put diagnosis in a comment.
- **Both #545 and #554 came from the same external contributor within two weeks, and BOTH sat
  unreviewed** (#545 for 13h; #554 until this arc). External PRs here are rare enough that
  nothing routes them to a human. That is the recurring failure, not any individual defect.

- 🔴 **TWO PARALLEL PRs ON ONE FILE: THE TEXTUAL CONFLICT IS THE HARMLESS HALF.** `#554` and
  `#564` were developed simultaneously against `cf5e4a8`, neither able to see the other, and
  both touch one map. `merge-tree` catches the textual clash — but the real damage is that one
  PR strengthens a guard while the other blinds it, which **no conflict marker can express**.
  Whenever two PRs touch one guard, ask *what each does to the guard's POWER*, not just whether
  the lines merge. Generalises past this pair: a clean merge means "no textual conflict", never
  "safe".
- 🔴 **`gh pr view --json mergeable` returns `UNKNOWN` until GitHub computes it lazily.**
  `UNKNOWN` is not an answer — re-poll until `MERGEABLE`/`CLEAN` rather than merging on it.
  Seen on `#554` again this session.
- **A PR's `updatedAt` moves when you COMMENT on it.** Both `#554` and `#564` show
  `2026-09-12T01:12` because of this session's own comments — do not read that as the author
  having pushed.
- **The ranked list here is now mostly DONE rows kept for numbering stability.** That is
  deliberate (rank is half a `claim-work` slug's identity) and is why `forcing: none` outnumbers
  the rest — it is not a queue of unforced work.

- 🔴 **A ZERO FROM A GUESSED PATTERN IS NOT A READING, AND I PRODUCED THREE IN A ROW.** Checking
  the vendored mirrors, `grep -oE "'[a-z]+:[a-z_.]+'"` returned **0** against a 410-line file of
  scope constants, and a schema probe returned "no scopes" for a file that plainly has them.
  Both were wrong patterns, not absences. **Inject a control into the comparison itself** — I
  added a fake scope to the upstream set and confirmed the diff reported it — and only then
  quote the zero.
- 🔴 **A TAB IS A COLUMN DELIMITER, SO "REACH COLUMN ZERO" WAS NEVER THE WHOLE HAZARD.**
  `saferune` keeps `\t` by design and `text/tabwriter` splits on it, so server text could
  *insert columns* and forge an ALIGNED row — more convincing than a newline forgery, because
  tabwriter does the aligning. Any guard on server text reaching a table must cover both.
- 🔴 **A GUARD'S FLOOR MUST BE STRICTLY BELOW THE SET IT GUARDS.** `minIndentCallSitesExpected`
  was 6 against 6 pinned sites and `Fatalf`'d first, so deleting a real call reported
  `CONTROL failure` — the file's own idiom for "the harness is broken" — and the SHRANK message
  never ran. That is how a floor gets lowered instead of a regression investigated.
- 🔴 **A MUTATION THAT DOES NOT APPLY REPORTS "SURVIVED".** Re-checking a guard, my replacement
  string did not match the source (`listReasonWrapWidth`, not a literal `79`), so the mutant was
  a no-op and read as a surviving defect. **Assert the target exists before mutating.**
- 🔴 **A SQUASH INHERITS COMMIT SUBJECTS, INCLUDING CLOSING KEYWORDS.** `#554`'s subject carried
  `(closes #552)`; merging `#569` would have auto-closed the issue holding all the deferred
  work. Merged with an explicit `--subject`/`--body`; verified **0** closing-keyword hits in the
  merge commit and `#552` still OPEN.
- **Amending an issue in a COMMENT is weaker than amending its BODY.** `#552`'s closing
  condition was corrected in a comment on 2026-09-11 while its body still described the narrow
  predicate and "8 fields". The body is what the next reader acts on; it was rewritten
  2026-09-12 with the original preserved in a `<details>`.
- **Blind audits paid for themselves twice.** #545's audit and #569's each refuted claims made
  by the work they reviewed — #569's refuted **three of my own**, including a comment asserting
  "the only thing that says so", a stale count, and a guard that checked an identifier's NAME
  rather than its value.

### THE ARC'S SHA LEDGER — deliberately filed HERE, under an APPEND heading

🔴 **This lived under `State now` (a REPLACE heading) and came within one confirm of being
deleted THREE times.** `handoff_doc.py`'s durable-line warning reads prose and does not see a
table, so it flagged 3 unrelated lines while 8 shas were silently in the drop set. It is a
FLOOR, not a guarantee — and the structural fix is to keep the record where nothing replaces it.

| what | sha / ref | state |
|---|---|---|
| `#545` (xsvm) images prompt indent | `feb330c` | MERGED |
| `#556` retraction of "not reproducible" | `d0d1805` | MERGED |
| `#551` handoff r2 | `5208a6b` | MERGED |
| `#565` handoff r3 | `d37b32a` | MERGED |
| `#567` handoff r4 | `f11f26e` | MERGED |
| `#570` handoff r5 | `c4ed077` | MERGED |
| `#569` tab vector + #564 reconciliation | `e3f2608` | MERGED |
| **`#573` tabwriter ledger + 20 renderers gated** | **`6f0a8d8`** | MERGED |
| `devrc#1498` mentions-id-collision lesson | `550de40` | MERGED — ⚠ NOT live until `home-manager switch` |
| **`#601` 429 ordering: sentinel + exit code** | **`b727a83`** | MERGED |
| `devdocs#5` creators totalItems lower bounds | `23c6d46` | MERGED 09-14, claim re-verified live first |
| `cli#575` R2/R3/R4 accepted in writing | comment `5671768753` | issue rests on R6 alone |
| `civitai/civitai#4768` (server-side #513) | new | FILED → agent dispatched 09-14 to fix + PR |
| stale branch `fix/flexstring-numeric-username` | — | DELETED, recovery sha `12818a3` |


- 🔴 **THE FIX ROUND'S OWN PROSE IS THE LIKELIEST NEXT FINDING, AND THIS ARC PROVED IT TWICE.**
  Round 2 of `#573` refuted **no shipped behaviour claim** — all ten fixes verified under
  mutation — yet every finding it returned was a false or incomplete SENTENCE written by the
  round that fixed round 1. Budget for it; do not read a clean behavioural result as a clean
  round.
- 🔴 **SWEEP THE SHAPE, NOT THE REPORTED SITES.** The audit named the false closed-set claim at
  two places. Grepping for the *shape* — a guard description asserting coverage its body does not
  provide — found a **third**, in residual 1, the line a reader hits first. That sweep is what
  made stopping legitimate rather than arbitrary.
- 🔴 **WHERE A GUARD HAS LOST ITS REASON, WRITE THAT IT HAS NONE.** `gatePreSanitised`'s comment
  had already been through one confident justification that was false. The fix is the retraction
  plus the enumerated residual, not a fresh rationale.
- 🔴 **A STALE LSP DIAGNOSTIC READS EXACTLY LIKE A BROKEN BUILD — twice in one session.**
  `sortedKeys redeclared` and `"regexp" imported and not used` both surfaced against PR heads
  that compiled clean; both were mid-edit worktree state. The second was especially plausible
  because `regexp` had been *my* import for a test the fix round deleted. **Build the actual
  pushed sha before reporting a compile error.**
- 🔴 **A REBASED BRANCH BREAKS THE NAIVE DELTA RANGE.** `#573` was rebased, so
  `<audited-tip>..<head>` would have spanned the rebase **and** an unrelated docs commit — the
  silent over-covering case. Check whether the audited tip is an ancestor; if not, find its
  rebased twin (`git diff <old> <new>` should show only main's own movement) and anchor there.
- **A CLOSING CONDITION THAT IS MET SHOULD CLOSE, even if you said otherwise an hour ago.** An
  open issue whose body describes fixed work is worse than a closed one with an accurate record.
  Reverse it in the open and say why.

- 🔴 **FOUR AUDIT LADDERS RAN THIS SESSION AND EVERY SINGLE FINDING WAS IN MY OWN EVIDENCE, NOT
  IN THE CODE.** #578: a coverage reduction whose opposite the PR body asserted. #582: three
  ledger rows saying SERVER about strings carrying the user's typed bytes. #77: a permission
  scope that was never needed, then five mutants surviving a fixture table that recorded requests
  and read none of them. The shipped behaviour was right early in all four; what took the rounds
  was making the CLAIMS about it true. Budget for that shape — it is not a sign the work is bad.
- 🔴 **TWICE IN TWO COMMITS I FIXED AN UNPINNED CLAIM AND SHIPPED THE FIX UNPINNED.** #77's 404
  retry arm, then the `firstStatus` message that fixed its message. Both survived the full suite
  on revert. **When a round's finding is "this claim has no fixture", check the FIX has one
  before committing** — the reflex to write the fix and move on is exactly what reproduces it.
- 🔴 **A FIXTURE THAT COUNTS CALLS IS NOT A FIXTURE THAT CHECKS REQUESTS.** #77's fake `fetch`
  took no arguments, so five mutants survived — including "never send the token" and "invert the
  ordering", the very property the table's failure text claimed to protect. Record the request
  (`{url, auth, signal}`) and assert the per-call ORDER, not the count. And then read every field
  you record: `url` was recorded and asserted by nothing for a whole round.
- 🔴 **A LEDGER THAT MISCOUNTS ITS OWN PAYLOAD BREAKS THE LADDER'S STOP GATE.** I reported "57
  executable payload lines" for a round whose true figure was 11 — 57 was the SCAFFOLDING count.
  The ladder stops on two consecutive ZERO-payload rounds, so an inflated number means the gate
  can never fire and the ladder runs on its own scaffolding forever. Classify hunk by hunk.
- 🔴 **A PERMISSION SCOPE THAT LOOKS NECESSARY MAY BE YOUR OWN HEADER.** #77 added `actions: read`
  because the jobs API answered 403 — but the repo is PUBLIC and answers **200 anonymously**; the
  403 came from unconditionally attaching `Bearer`. Measured with controls (bogus run id → 404).
  Token-first + anonymous retry removed the scope and made three doc claims true again untouched.
- 🔴 **THE INSTRUMENT FAILED FIVE TIMES THIS SESSION, EACH TIME IN THE REASSURING DIRECTION.**
  A mutation anchor matching 0 times printed SURVIVED; another matching 2 times did the same; a
  case-sensitive grep for a lowercase banner returned a false zero; `set -- $x` in zsh did not
  word-split so a gate measurement ran with an EMPTY range and reported 0; and `| head` ate a
  non-zero exit status. **Assert the anchor matched exactly once, and capture `rc` before piping.**
- 🔴 **`git checkout -- <file>` DISCARDS UNCOMMITTED WORK, AND I DID IT TWICE MID-MUTATION.** Both
  times it silently reverted an edit I had not committed. Use `cp -a <f> /tmp/bak` before a
  mutation battery and restore from the copy; commit before starting one.
- 🔴 **`gh issue view --json body | grep` IS A CLAIM ABOUT YOUR PATTERN.** Verifying #575's body
  edit, a grep for `CLOSED — #578` returned 0 because the real text is `**CLOSED** — #578`.
  Pair every such check with a positive control string you know is absent.
- **An issue's BODY is what the next reader acts on; comments are not.** #575 listed R1 as an
  open 🔴 for hours after #578 merged, and R5/R6 exist only as comments. Annotate the body in
  place — a status table at the top, headings amended rather than rewritten — so the original
  framing stays readable.
- **`audit-dispatch.py` takes `--repo owner/name`, not `owner/name#N`.** The positional argument
  is an int. For a cross-repo PR it prints an explicit `git -C … worktree add` recipe and tells
  you NOT to pass `isolation: "worktree"` — that flag worktrees the cwd's repo, which is wrong.
- **A round-0 claims block is REFUSED by design** (`round 0 has no fixes to claim`). Round 0's
  verdict goes in a PR comment as prose; the claims chain starts at the round that first fixes
  something.
- **Round-0 trial ledger is CLOSED** at `ran: 6 · changed the outcome: 3`; the fix was a TRIGGER
  (`audit-pr-nudge.py` routes it at `gh pr create`), not an edit to the section. Keep reporting
  the pair per PR — it is now the only signal for whether that trigger works. This session:
  `ran: 3 · changed the outcome: 3`.
- **A subagent self-disclosed exceeding its permitted writes** — a `git fetch` with no destination
  refspec in a shared clone. Checked: `for-each-ref 'refs/pull/*'` → 0, clone clean. The
  disclosure was accurate and the impact nil; worth knowing the disclosure habit works.

- 🔴 **THE devdocs `drift-sweep` INDEX ENTRY COULD NOT BE WRITTEN, AND THE CONTENT IS HERE SO IT
  IS NOT LOST.** `cairn create --scope civitai-developer-docs` refuses `[not-found]`; `cairn
  doctor` names the scope as existing only in the frozen pre-cutover mirror and absent from the
  store's answer to this token. 🔴 **Do NOT fall back to writing the local mirror** — entry files
  are `0444`, but `Edit` rewrites-and-renames and `Write` creates a fresh `0644` file, so both
  succeed and strand the content invisibly on one host; that is a measured 5-entry / 24-bullet
  loss. The durable facts that entry would have carried:
  - `.github/workflows/appblocks-drift.yml` — `drift` pins `contents: read`; `notify` is the one
    elevated job (`issues: write`) and runs **no `npm ci`**, so `scripts/drift-notify.mjs` may
    import node builtins ONLY.
  - **No `actions: read` is needed and adding one is the easy mistake.** The repo is PUBLIC, so
    `GET /actions/runs/<id>/jobs` answers **200 anonymously** (bogus run id → 404 as the control).
    A first cut attached `Bearer` unconditionally, got 403 from a scope-less token, and granted a
    scope to fix a header it was sending itself. Token-first, anonymous retry on 403 **or** 404.
  - **`check-example-apps` RUNS on every PR but is NOT a required context** on `main`
    (`test-cli, test-messages, test-bridge, typecheck-snippets, build-site, test-md-regions`).
    Say "runs on", never "gates".
- 🔴 **A COUNT I ASSERTED WENT STALE INSIDE ONE DAY, AND THE FIX IS WHAT REVEALED IT.** I wrote
  "six real drifts" in a PR body, a merge commit and this doc; the verification run shows
  **seven**. The seventh is the example-app-rot check — the one whose counters the broken body
  rendered — so its own failure was the least visible of all. Re-derive a count from the newest
  run before quoting it, and prefer "as of run `<id>`" to a bare number.

- 🔴 **`audit-dispatch.py` WITHOUT `--repo` READS THE CWD'S REPO, AND A SAME-NUMBERED PR IN THE
  WRONG REPO ANSWERS.** Dispatching a round for `civitai-developer-docs#80` from a `civitai/cli`
  checkout read **`civitai/cli#80`** — which exists — and reported *"no `audit-claims` block in any
  of the 0 comment(s) read"*. The zero looks exactly like "nobody posted a block". **Always pass
  `--repo owner/name` for a cross-repo PR**, and treat a surprising zero as a repo question first.
- 🔴 **AN `audit-claims` FENCE WITHOUT `round=`/`audited=` IN ITS *HEADER* SILENTLY WIDENS THE NEXT
  ROUND'S RANGE.** Metadata inside the fence BODY does not parse. The script then falls back to the
  newest block it CAN read — an older round's — and the "delta" spans two rounds of fixes while
  reading as a perfectly ordinary one. It is announced **on stderr only** (`the newest claims block
  says round=1, and you asked for round 3`) and nowhere in the brief. **Read the stderr of every
  assembly**, and emit the block with `--emit-claims --audited <tip>` rather than hand-writing the
  fence.
- 🔴 **A WORKFLOW THAT GOES GREEN BY *SKIPPING* IS NOT A FIXED WORKFLOW.** `cli-snapshot-refresh`
  reported `success` with `build-cli: skipped` and `refresh: skipped`. Reading only the run
  conclusion would have closed rank 14 on a run that never executed the path that was broken. **Read
  the per-job conclusions, and ask what a green run actually DID** — `success` over a skipped job
  and `success` over a completed one are the same word.
- 🔴 **A REPLACEMENT ASSERTION IS NOT AUTOMATICALLY A SUPERSET OF THE ONE IT REPLACES, AND NOTHING
  CHECKS THAT IT IS.** Measured twice in one ladder. The tell is a round that "improves" a guard by
  pointing it somewhere better: rounds 4 and 5 of `#80` each caught a mutant the other missed, and
  both commit messages read as upgrades. **When you re-point a guard, run the OLD assertion against
  the new mutant set too** — and prefer accumulating assertions over swapping them; they cost one
  line each.
- 🔴 **A MUTATION RESULT IS A CLAIM ABOUT THE MUTANT YOU *RAN*, NOT THE ONE YOU *DESCRIBED*.** A
  round-5 commit asserted a two-edit mutant was red; the number came from a run that had made
  **three** edits, and the two-edit version was green. Because the three-edit version genuinely was
  red, the claim read as verified for a full round. **Write the mutant down as a diff, not as prose,
  and re-apply it from that diff when you quote the result.**
- 🔴 **A PIPE EATS THE EXIT STATUS.** `bash clawgate_handoff.sh field <doc> | tail -2; echo rc=$?`
  printed `rc=0` — `tail`'s status — for a script that had exited **1** (no field present). Same
  class as the repo's own `gh pr checks` traps. **Capture first: `out=$(cmd 2>&1); rc=$?`.**
- **`claim-work` batch loops report a false failure.** A `for` loop ending in
  `[ $rc -ne 0 ] && echo …` returns 1 when the last claim SUCCEEDED, so the whole command exits 1
  while every claim printed `[0]`. Read the per-item codes, not the loop's.

- 🔴 **THE `civitai-developer-docs` CAIRN SCOPE IS STILL UNREACHABLE (re-probed 2026-09-14), SO
  THIS BULLET IS WHERE ITS INDEX ENTRY WOULD HAVE GONE.** `cairn append --scope
  civitai-developer-docs --ref apps` exits **6**, `the store REFUSED the write [not-found]`. That
  is a REAL reading, not a broken client: the `cli` scope accepted two appends minutes earlier
  from the same session (`appapi` rev `464fb632`, `safeterm` rev `ee06726f`), so the binary, the
  endpoint and the token all work. 🔴 **Do NOT fall back to editing the local mirror** — entries
  are `0444`, but `Edit` rewrites-and-renames and `Write` creates a fresh `0644` file, so both
  succeed and strand the content invisibly on one host. The durable facts that entry would carry:
  - **`descriptionHasTable` is stamped into `hooks.json` and branched on by `<HooksReference>`'s
    `<pre v-if>`.** The `.md` fallback region is a DEAD CHANNEL — the island declares no
    `<slot />` and discards it — so a fix applied there passes `check:md-regions` over a page that
    is still broken. `public/appblocks/` is gitignored, so the flag exists only in a BUILT tree
    and no assertion on the committed artifact can see it.
  - **`check-built-site.mjs` must assert a RELATIONSHIP, not a floor.** A `lines.length >= 10`
    floor red-lights a correctly rendered minimal table (5 lines) on a REQUIRED context; a
    `pre.length === flagged` equality reads `0 === 0` when the generator stops stamping the flag,
    which silently restores the original bug. Both were measured. Assert per-hook element
    placement plus `typeof h.descriptionHasTable === 'boolean'`.
  - **No `actions: read` is needed for `scripts/drift-notify.mjs`** — the repo is PUBLIC, so
    `GET /actions/runs/<id>/jobs` answers **200 anonymously** (a bogus run id → 404 is the
    control). Token-first, anonymous retry on 403 **or** 404.
  - **`check-example-apps` RUNS on every PR but is NOT a required context**; the required set is
    `test-cli, test-messages, test-bridge, typecheck-snippets, build-site, test-md-regions`, read
    from the branch-protection API rather than a workflow comment. Say "runs on", never "gates".
- 🔴 **A MERGE THAT RESOLVES CLEANLY STILL DROPS THE OTHER SIDE'S EDITS IN REGIONS THAT NEVER
  CONFLICTED, AND GIT SAYS NOTHING — IT FIRED THREE TIMES IN ONE SESSION AND SHIPPED ONCE.** The
  shape: `main` moves, you merge it into a branch, resolve the two real conflicts, take one side's
  copy of a big file, and re-apply the other side's rows **you can think of**. Everything you did
  not think of is gone silently. Caught twice mid-merge, shipped once: `cli#585`'s merge dropped
  `cli#591`'s two Troubleshooting rows onto `main`, putting back the exact 🔴 that PR existed to
  fix, with the generated section thirty lines above still saying the opposite. **The fix is
  mechanical and cheap: after resolving, `diff <(git show origin/main:<file>) <file>` and require
  the "lines main has that we lack" set to be EMPTY.** The one drop I caught on the second merge
  was caught by exactly that; I had not run it on the first. **Spot-checking a merge is sampling;
  the diff is the measurement.**
- 🔴 **I ASSERTED "NOTHING UNIQUE" ABOUT A BRANCH WHILE THE `git diff --stat` I HAD JUST PRINTED
  SHOWED FIVE LINES DIFFERENT — AND DELETED IT.** The object survived locally
  (`git cat-file -e <sha>`) and those five lines were exactly the regression fix. **Before deleting
  any branch, read the diff you printed, not the conclusion you had already reached.** A deleted
  remote branch is recoverable only while the local object lives.
- 🔴 **A GENERATED REGION CANNOT BE HAND-MERGED, AND THERE IS MORE THAN ONE OF THEM.** README's
  exit-code **sections** and its exit-code **table** are generated separately from `exitCodeDocs`;
  regenerating only the sections leaves `TestREADMEExitCodeTableIsGenerated` red. Resolve a
  conflict in either by REGENERATING from the merged source — a throwaway
  `func TestZZRegen` calling `readmeExitCodeSections()` / `readmeExitCodeTable()`, deleted in the
  same session — never by editing the generated text.
- 🔴 **A PR OBJECT'S HEAD IS A CLAIM; `mergedAt` AND THE REMOTE TIP ARE THE FACTS.** `cli#591` was
  merged by a concurrent session while I worked on it. The PR reported head `c355274` and
  `mergeable=UNKNOWN` for >12 minutes after a successful push whose tip was `02b0f50`, and
  `gh api .../commits/02b0f50/check-runs` returned **total=0** — which reads as "CI pending", not
  as "this PR is closed". `gh pr close` is what finally surfaced it. **On a surprising 0-checks,
  check `mergedAt` before waiting.**
- 🔴 **A TEST WHOSE EXPECTATION RE-IMPLEMENTS THE PREDICATE CANNOT SEE A MUTATION OF IT.** My
  first boundary test for `#585` asserted `size > MaxSubmitBodyBytes` *in the test*; the `>=`
  mutant duly survived. Driving the real `SubmitVersion` instead, it STILL survived — and the
  measurement explains why: the envelope is 19 bytes and base64 steps by 4, so body sizes go
  `10485759, 10485763`. **Nothing lands on the ceiling, so no input distinguishes `>` from `>=`.**
  That is now stated as unreachable rather than faked as covered, with a guard that fires if a
  future envelope makes it reachable (mutating the envelope by one byte turns it red).
- 🔴 **"IT REPRODUCES BOTH MEASURED POINTS" IS VACUOUS WHEN THE NUMBER IS INSIDE THE BRACKET
  THOSE POINTS DEFINE.** I wrote, in two published surfaces, that `MaxSubmitBodyBytes` "correctly
  predicts both of #423's measured submissions, which is the evidence that matters more than its
  provenance". 10485760 ÷ 4/3 is a 7,864,320-byte zip, INSIDE `(2.32 MB, 8.20 MB]` — every number
  in that bracket reproduces both observations, by definition. The agreement carries zero
  information. **Provenance was the whole argument; the corroboration was decoration.**
- **`audit-dispatch.py` needs `--repo` for a cross-repo PR** — without it, it reads the cwd's repo,
  and a same-numbered PR there answers. And an `audit-claims` fence missing `round=`/`audited=`
  **in its header** makes the next round silently anchor on an OLDER block, widening the "delta"
  across two rounds of fixes. Both warnings are **stderr-only**. Emit blocks with
  `--emit-claims --audited <tip>` rather than hand-writing the fence.
- **A grep's answer is a claim about the grep's view.** `grep "does NOT identify the deep-paging
  cap"` returned 0 for a comment that was present — the phrase wrapped across two comment lines.
  I nearly recorded a dropped edit that had never been dropped.
- **zsh does not word-split `$var`.** `set -- $combo` inside a `for` made `$1="ra cap"` and `$2=""`,
  so four "combinations" of a 429 fixture all ran as one, returning a uniform `rc=6` — twice, read
  past twice. **Validate a fixture server with `curl` and confirm both switches VARY before
  trusting any row of a matrix.**

## How to verify

```bash
# 1. main is green and self-consistent — the index and the generated section agree
cd /home/zach/workspace/civit/cli && git checkout main && git merge --ff-only origin/main
make ci                                  # rc=0
go test ./... -count=1 | grep -c '^FAIL'  # 0
grep -c "This one message has TWO exit codes" README.md          # 1  (index)
grep -c 'reaches \*\*`2` or `6`, never `5`\*\*' README.md         # 1  (generated section)
grep -c "Throttled; exit \`6\`\. For deep paging" README.md       # 0  (the regressed row is gone)

# 2. #585's escape hatch is real, and the refusal costs no upload
go test ./internal/cmd/ -run TestAppSubmitOversizeEndToEnd -count=1 -v
go test ./internal/appapi/ -run 'TestAllowOversizeBody|TestSubmitCeilingValue' -count=1 -v

# 3. rank 22 — BOTH guards must go red on the ordering mutant. Insert, in the 429
#    branch of pkg/civitai/retry.go's getWithRetry, ahead of the retryAfterDelay call:
#        if isDeepPagingCap(string(raw)) { return status, raw, nil }
#    Restore from a `cp -a` copy afterwards, and grep the mutant out by name to
#    confirm it is gone — a timed-out battery leaves it behind.
go test ./... -count=1 | grep -c '^--- FAIL'   # 2: the sentinel guard AND the rc guard
go test ./... -count=1 | grep -c '^ok '        # 19 (21 minus pkg/civitai and cmd/civitai)

# 4. rank 9 — CLOSED, but this is how it was closed; re-run reads the NEWEST scheduled run
rid=$(gh run list --repo civitai/civitai-developer-docs --workflow appblocks-drift.yml \
        --limit 5 --json databaseId,event -q '[.[]|select(.event=="schedule")][0].databaseId')
gh api "repos/civitai/civitai-developer-docs/actions/runs/$rid/jobs" \
  -q '[.jobs[]|select(.name=="drift")|.steps[]|select(.conclusion=="failure")]|length'  # 0
gh api "repos/civitai/civitai-developer-docs/actions/runs/$rid/jobs" \
  -q '[.jobs[]|select(.name=="drift")|.steps[]]|length'   # >0, or the green means "skipped"
```
