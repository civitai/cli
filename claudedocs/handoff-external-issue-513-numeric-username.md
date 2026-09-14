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

- **`civitai/cli` `main` @ `a6395ef`** (clean); **`civitai-developer-docs` `main` @ `cced3e2`** (clean,
  re-synced after the worktree merge).
- 🔴 **RANK 9 IS DISCHARGED. The `appblocks-drift` sweep is GREEN — 0 failing steps** at dispatched
  run `34765344965`, with **15 steps executed** as the positive control, so the zero is a reading
  and not a job that ran nothing. **`developer-docs#76` auto-CLOSED** by the notify job, which also
  demonstrates `#77`'s reporting fix works in the GREEN direction and not only the red one.
  Trajectory across the arc: **7 failing steps (09-13 00:00) → 2 (09-13 12:14) → 0 (09-13 15:2x)**.
  Last green scheduled run before today was **2026-08-11**; **34 consecutive red scheduled runs**
  ended.
- **`devdocs#80` MERGED** (`cced3e2`), verified by CONTENT not ancestry — a squash is never an
  ancestor, so `scripts/test-appblocks-hooks.mjs`, `scripts/lib/description-has-table.mjs` and the
  `blocks-react@0.49.0` / `app-sdk@0.39.0` pins were each confirmed present in `origin/main`.
- **`devdocs#81` and `#82` filed**, each with a mechanical closing condition (see ranks 20 and 14).
- **Three `civitai/cli` PRs are open, CLEAN and 13/13 green**, created today by CONCURRENT sessions:
  `#585`, `#590`, `#591`. All three are claimed (`audit-cli-585/590/591`) and under audit as of this
  writing. **They are the highest-value outstanding work in the arc** — finished code awaiting a
  merge decision, not new engineering.
- **No `clawgate-task:` field.** `resolve` exited **5**: 0 tasks for this session, with its positive
  control answering 7 links for a DIFFERENT session — so the board was reached and the token is
  accepted, but a wrong id also answers 200 with an empty array. That is a narrower claim than a
  clean bill of health, so no field was written.

### Merged this arc — verified by CONTENT

| what | sha |
|---|---|
| `cli#578` — #575 **R1** | `3457c5d` |
| `cli#582` — #575 **R5** | `095f4ac` |
| `devdocs#77` — the reporting half of `#76` | `17f2a44` |
| `devdocs#80` — the last two drifts, after a 7-round ladder | `cced3e2` |

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
- **Root cause (`via: code` + `via: measurement`):** `runImageSearch`
  (`image-search.service.ts:127`) forks — `imageId`, or `modelId` without `modelVersionId`, take
  the legacy Postgres path (`getAllImages`); everything else takes the **Meilisearch feed path**
  (`getImagesFromFeedSearch`). `image-search.service.ts:255` passes `image.user.username` through
  verbatim. The Meilisearch index has stored `user.username` with Meili's dynamic JSON typing, so
  an all-digit username is a **JSON number in the index document**. Path, not user, not
  cardinality, not the CDN.
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
- **Next probe:** none needed for diagnosis. The open work is outward: reply to Rochet2, and
  file against `civitai/civitai`. `via: code` (not measured): `/api/v1/blocks/images` shares
  `runImageSearch` per the comment at `index.ts:167`, so it is almost certainly affected too —
  worth one cache-busted check before asserting it in a bug report.

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

### #513's server half — FILED, and the CLI cannot do more

- **State:** `civitai/civitai#4768` carries the root cause (Meilisearch feed path stores
  `user.username` under dynamic JSON typing), the 8-row path-fork table, the 51/51 sweep and
  a closing condition. `civitai/cli#513` deliberately **stays open**; Rochet2 was told
  directly that `FlexString` does not make the value correct.
- **Ruled out:** that `/api/v1/blocks/images` could be confirmed — it returns
  `401 Block token required`, so it is labelled *suspected, not verified* in #4768 rather
  than asserted. `via: command`

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

### 🔴 RESOLVED: `developer-docs#76` — all seven drifts cleared, sweep green
- as-of: 2026-09-13

- **Symptom + exact repro:** the scheduled `appblocks-drift` sweep had been red for 34 consecutive
  scheduled runs. `gh workflow run appblocks-drift.yml --repo civitai/civitai-developer-docs`, then
  count failing steps in the `drift` job.
- **Observed (with values):** run `34765344965` — `conclusion=success`, **0 failing steps, 15 steps
  executed**. Issue `#76` state `CLOSED`, `updated=2026-09-13T15:21`, closed by the `notify` job
  (`job notify: success`). The last two drifts were `check:pins` (step *"Generation-bridge pin drift
  — pinned SDK devDeps vs npm latest"*) and `check:ds-pins` (step *"Design-system pin drift — docs
  CDN literals vs the declared pin"*); the step→script mapping was read out of the workflow, not
  assumed from the step names.
- **Ruled out:** *"the branch passing the two checks proves the sweep will go green"* — **via:
  measurement**. The branch result is a claim about the branch; the sweep was re-run against merged
  `main` before this was called done.
- **Next probe:** none. Closing condition met.

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

### The `devdocs#80` audit ladder — 7 rounds, and what it actually found
- as-of: 2026-09-13

- **Symptom + exact repro:** a hook description carrying a GFM table flattened into one
  1853-codepoint run of literal pipes on `apps/reference/hooks.html`.
- **Observed (with values):** the shipped page was **correct from round 2 onward**. Rounds 3–7 found
  **only scaffolding defects**. The one that justifies the whole ladder is round 4's: with the flag
  misspelled generator-side, the page built with **0 `<pre>` elements** — the original bug, fully
  restored — while `check:built-site` **rc=0** and `test:appblocks:hooks` **rc=0**. Round 7 was
  clean and the ladder stopped.
- **Ruled out:** *"a replacement assertion is at least as strong as the one it replaces"* — **via:
  measurement**. Twice, a replacement silently dropped coverage: rounds 4 and 5 were a **trade**,
  each catching a mutant the other missed. Nothing checks this.
- **Leading hypothesis (the arc's durable lesson):** the recurring defect was **a guard's
  DESCRIPTION claiming more than its implementation** — four of seven rounds' findings. The fix
  shape that ended it was to **stop replacing assertions and start accumulating** them: every level,
  every shape, one line each.
- **Next probe:** none for #80. The residual is `devdocs#81`.

## Next steps (ranked)

🔴 Numbering stable — rank is half a `claim-work` slug's identity. 1–15 keep their meaning.

1. **DONE — `civitai/cli#526`.** forcing: none
2. **`developer-docs#61`** — awaiting the reporter's blue balance at the time of their 500.
   Claim held. forcing: user — external reporter, waiting since 2026-09-11.
3. **DONE — #513 diagnosed; `civitai/civitai#4768` filed.** Still **0 comments** as of
   2026-09-13; needs routing to a platform owner, which is outward-facing and unasked-for so far.
   forcing: none
4. **DONE — stale branch deleted** (`12818a3`). forcing: none
5. **DONE — `#545` merged.** forcing: none
6. **DONE — `#554` closed by @xsvm.** forcing: none
7. **DONE — AGENTS.md item 37 corrected** via `#556`. forcing: none
8. **DONE — `#552` CLOSED.** forcing: none
9. **DONE — `developer-docs#76` CLOSED and the sweep is GREEN** (0 failing steps, run
   `34765344965`). All seven drifts cleared; `#80` took the last two. **Still unstarted and now
   tracked at rank 21**, not here: the `/apps/installed` → `/apps/activity` prose rename and the
   `WorkflowStep.jobs` spec residual. forcing: none
10. **DONE — vendored mirrors measured CLEAN.** forcing: none
11. **`home-manager switch`** so `devrc#1498`'s lesson is live. Operator's call — restarts
    collector/keylog/i3 on both hosts. forcing: none
12. **`civitai/cli#575` — R2, R3, R4, R6 remain.** R2–R4 need a WRITTEN maintainer decision, not
    code. **R6** is the one with engineering in it. forcing: none
13. **DONE — `#77` verified live**, and re-confirmed this session in the GREEN direction: the notify
    job closed `#76` when the sweep passed. forcing: none
14. **RECLASSIFIED — `cli-snapshot-refresh` is LATENT, not fixed.** Its green runs mean the snapshot
    matched and the work was SKIPPED; the bot-PR org policy is untouched and the next `civitai/cli`
    release re-triggers it. Filed as **`developer-docs#82`** with a closing condition that a
    `refresh: skipped` run cannot satisfy. forcing: gate — a gate whose green means "did nothing"
    trains everyone to ignore it.
15. **The `civitai-developer-docs` cairn scope is unreachable.** `cairn create` refuses
    `[not-found]`; the drafted `drift-sweep` entry is preserved in this doc's Gotchas.
    forcing: none
16. **`civitai/cli#591`** — *docs(exitcodes): publish that a 429 can exit 2*. CLEAN, 13/13 green,
    **no audit round had run**. First full audit dispatched (round 0 + nine axes), with the
    published-contract surfaces (README table, command section, Troubleshooting index, the
    GENERATED `internal/cmd/exitcodes_doc.go`) named as the completeness question — AGENTS.md
    records that #371 updated two of three. Claim `audit-cli-591`. **Merge when the round is clean.**
    forcing: none
17. **`civitai/cli#590`** — *fix(download): stop a server-supplied file name forging lines*. CLEAN,
    13/13 green, **round 1 already posted**, so a DELTA round 2 was dispatched over
    `208676c..67cad760`. The claims most worth attacking are its self-reported mutation counts
    (claim 10: "reddens four named arms") and claim 11, which **states a residual rather than fixing
    it** — an unbounded `p.name` soft-wrapping to strand forged text with no `\n` and no `\t`.
    Claim `audit-cli-590`. forcing: security — a server-supplied string forging terminal lines,
    including a fake "SHA256 verified".
18. **`civitai/cli#585`** — *fix(app submit): refuse a bundle the server cannot receive*. CLEAN,
    13/13 green. **ROUND 0 ONLY** was dispatched, deliberately: AGENTS.md **item 31** says the
    packager's caps are a DELIBERATE NON-MIRROR of the server and that #423 *"bracketed but could
    not pin"* the server ceiling. The round-0 question is whether this PR vendors a ceiling item 31
    forbids, and **what number it refuses at and where that number came from** — a local refusal at
    a guessed threshold rejects bundles the server would have accepted, on a publishing path.
    Claim `audit-cli-585`. forcing: none
19. **`developer-docs#5`** — *docs(creators): totalItems/totalPages are lower bounds on ?query=*.
    Open since **2026-07-13**, MERGEABLE/CLEAN, 12 checks. Two months stale; needs a yes/no rather
    than work. forcing: none
20. **`developer-docs#81`** — a transitive `ERR_MODULE_NOT_FOUND` from `check-cli-install-parity.mjs`
    is misrouted to `test-appblocks-hooks.mjs`'s *retirement* message. **Filed rather than fixed on
    purpose**, so round 7 stayed the ladder's last. Not a regression; the `underlying error:` line
    names the real cause. One line if anyone touches the file anyway. forcing: none
21. **The `developer-docs#76` residuals that are NOT drift-checked.** (a) `WorkflowStep.jobs` was
    removed from the refreshed OpenAPI spec while `orchestration/guide/workflows.md:75,82-111` still
    documents it as a field and a whole `## Jobs` section — **needs an authenticated live call** to
    tell "the spec stopped declaring it" from "the API stopped returning it"; do not delete a
    documented section on a spec diff alone. (b) the `/apps/installed` → `/apps/activity` prose
    rename. (c) `apps/guide/concepts.md`'s NSFW gating advice (`isSfwCeiling(maxBrowsingLevel)`) is
    now the LESS safe option and the page is re-stamped to 0.49.0. forcing: none

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
- **Round-0 trial ledger, cumulative: `ran: 1 · changed the outcome: 0`.** Two more runs
  needed before the retire/promote decision. Recorded here, not on external contributors' PRs.
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
- **Round-0 trial ledger, cumulative: `ran: 1 · changed the outcome: 0`.** Two more runs before
  the retire/promote decision.
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
| `civitai/civitai#4768` (server-side #513) | new | FILED, 0 comments |
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
- **Round-0 trial ledger, cumulative: `ran: 3 · changed the outcome: 1`.** The one that changed
  it produced four deletion candidates and a dropped requirement, none of which the nine
  correctness axes would have asked about. One more run before the retire/promote decision.

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

## How to verify

```bash
# 1. the drift sweep is green, and the zero is a READING (positive control: >0 steps executed)
rid=$(gh run list --repo civitai/civitai-developer-docs --workflow appblocks-drift.yml \
        --limit 1 --json databaseId -q '.[0].databaseId')
gh api "repos/civitai/civitai-developer-docs/actions/runs/$rid/jobs" \
  -q '[.jobs[]|select(.name=="drift")|.steps[]|select(.conclusion=="failure")]|length'   # -> 0
gh api "repos/civitai/civitai-developer-docs/actions/runs/$rid/jobs" \
  -q '[.jobs[]|select(.name=="drift")|.steps[]]|length'                                   # -> 15, not 0

# 2. #76 is closed; #80 landed by CONTENT (a squash is never an ancestor)
gh issue view 76 --repo civitai/civitai-developer-docs --json state -q .state             # -> CLOSED
git -C /home/zach/workspace/civit/civitai-developer-docs fetch -q origin
for f in scripts/test-appblocks-hooks.mjs scripts/lib/description-has-table.mjs; do
  git -C /home/zach/workspace/civit/civitai-developer-docs cat-file -e "origin/main:$f" \
    && echo "present: $f"
done

# 3. the new predicate battery actually runs in CI, and is not merely present
gh api repos/civitai/civitai-developer-docs/actions/runs/<a build-site run id>/jobs \
  -q '.jobs[].steps[] | select(.name|test("predicate")) | "\(.conclusion)  \(.name)"'

# 4. rank 14 is LATENT, not fixed — a green run here must be read per-job
gh api repos/civitai/civitai-developer-docs/actions/runs/34757726339/jobs \
  -q '.jobs[] | "\(.name): \(.conclusion)"'        # decide=success, build-cli+refresh=SKIPPED
```
